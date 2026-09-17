// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package chat

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/url"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

var uploadBotMessageFile = helpers.UploadConversationLocalFile

func validateSendExtensions(rt *shortcut.RuntimeContext, identity, kind string) error {
	if kind == "a2ui" {
		if identity != "user" {
			return apperrors.NewValidation("A2UI 仅支持 user")
		}
		if messagesSendIdempotencyKey(rt) != "" {
			return apperrors.NewValidation("A2UI requestId 是链路追踪，不支持 --uuid/--idempotency-key")
		}
		if _, err := helpers.ParseChatA2UIMessages(rt.Str("a2ui-messages")); err != nil {
			return err
		}
	} else if rt.Str("card-summary") != "" || rt.Str("biz-card-id") != "" || rt.Str("request-id") != "" {
		return apperrors.NewValidation("卡片参数仅用于 --a2ui-messages")
	}
	if rt.Str("image-url") != "" {
		u, e := url.Parse(rt.Str("image-url"))
		if identity != "bot" || e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return apperrors.NewValidation("--image-url 仅用于 bot，必须是无凭据的 HTTP(S) URL")
		}
	}
	if rt.Str("media-id") != "" && identity != "user" {
		return apperrors.NewValidation("--media-id 仅支持 user；Bot 图片使用 --image-url")
	}
	if rt.Command().Flags().Changed("expires-seconds") && kind != "share-chat" {
		return apperrors.NewValidation("--expires-seconds 仅用于 --share-chat-id")
	}
	if rt.Int("expires-seconds") < 0 {
		return apperrors.NewValidation("--expires-seconds 不能为负数")
	}
	if kind == "profile" || kind == "share-chat" {
		if identity != "user" {
			return apperrors.NewValidation("名片/群邀请分享仅支持 user")
		}
		if kind == "profile" {
			if err := targetresolver.ValidateExplicitOpenDingTalkID("--contact-id", rt.Str("contact-id")); err != nil {
				return err
			}
		}
	}
	if identity == "bot" && (kind == "image" || kind == "file") {
		if len(rt.StrSlice("open-dingtalk-ids")) > 0 {
			return apperrors.NewValidation("Bot 媒体单聊请使用 --users；当前发布的 MCP 媒体合同仅声明 userIds")
		}
		if rt.Bool("at-all") || len(rt.StrSlice("at-open-dingtalk-ids"))+len(rt.StrSlice("at-user-ids")) > 0 {
			return apperrors.NewValidation("Bot 图片/文件不接受 @ 参数")
		}
		if kind == "file" {
			groups, err := messagesSendBotGroups(rt)
			if err != nil {
				return err
			}
			count := len(groups) + nonEmptyStringCount(rt.StrFirst("chat-id", "group")) + len(uniqueShortcutStrings(rt.StrSlice("users"))) + len(uniqueShortcutStrings(rt.StrSlice("open-dingtalk-ids")))
			if count != 1 {
				return apperrors.NewValidation("Bot 本地文件上传绑定会话，必须且只能指定一个群或单聊接收者")
			}
			safe, err := apperrors.SafeInputPath(rt.StrFirst("file", "file-path"))
			if err != nil {
				return err
			}
			if _, err = helpers.BuildConversationLocalFileMeta(safe, "", ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func executeMessagesSendUserShare(rt *shortcut.RuntimeContext, group, openID, kind string) error {
	if kind == "a2ui" {
		messages, err := helpers.ParseChatA2UIMessages(rt.Str("a2ui-messages"))
		if err != nil {
			return err
		}
		requestID, cardID := rt.Str("request-id"), rt.Str("biz-card-id")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		if cardID == "" {
			cardID = uuid.NewString()
		}
		summary := rt.Str("card-summary")
		if summary == "" {
			summary = strings.Join(messages, "\n")
		}
		args := map[string]any{"requestId": requestID, "bizCardId": cardID, "a2uiMessages": messages, "summary": summary, "protocolVersion": "1.0", "flowStatus": "PROCESSING"}
		addMessagesSendUserTarget(args, group, openID)
		return executeUnifiedMessageWrite(rt, "im", "create_and_send_a2ui_card", args)
	}
	if kind == "profile" {
		content, _ := json.Marshal(map[string]string{"openDingTalkId": rt.Str("contact-id")})
		args := rt.AddAIMessageTag(map[string]any{"msgType": "profile", "content": string(content)})
		addMessagesSendUserTarget(args, group, openID)
		if key := messagesSendIdempotencyKey(rt); key != "" {
			args["uuid"] = key
		}
		return executeUnifiedMessageWrite(rt, "chat", "send_personal_message", args)
	}
	args := map[string]any{"sourceOpenConversationId": rt.Str("share-chat-id"), "expiresSeconds": rt.Int("expires-seconds")}
	if group != "" {
		args["targetOpenConversationId"] = group
	} else {
		args["receiverOpenDingTalkId"] = openID
	}
	if key := messagesSendIdempotencyKey(rt); key != "" {
		args["uuid"] = key
	}
	return executeUnifiedMessageWrite(rt, "im", "share_group_invite_url", args)
}

func executeMessagesSendBotMedia(rt *shortcut.RuntimeContext, kind string) error {
	groups, err := messagesSendBotGroups(rt)
	if err != nil {
		return err
	}
	group := rt.StrFirst("chat-id", "group")
	if group != "" {
		groups = []string{group}
	}
	args := map[string]any{"robotCode": rt.Str("robot-code")}
	key := "sampleImageMsg"
	if kind == "image" {
		args["photoURL"] = rt.Str("image-url")
	} else {
		key = "sampleDingtalkDriveFile"
		safe, err := apperrors.SafeInputPath(rt.StrFirst("file", "file-path"))
		if err != nil {
			return err
		}
		meta, err := helpers.BuildConversationLocalFileMeta(safe, "", "")
		if err != nil {
			return err
		}
		target := map[string]any{}
		if len(groups) == 1 {
			target["openConversationId"] = groups[0]
		} else if ids := uniqueShortcutStrings(rt.StrSlice("users")); len(ids) == 1 {
			target["userId"] = ids[0]
		} else {
			target["openDingTalkId"] = uniqueShortcutStrings(rt.StrSlice("open-dingtalk-ids"))[0]
		}
		if rt.DryRun() {
			return rt.Output(map[string]any{"dry_run": true, "executed": false, "identity": "bot", "target": target, "file": map[string]any{"path": rt.StrFirst("file", "file-path"), "name": meta.FileName, "sizeBytes": meta.FileSize}, "steps": []string{"init_conversation_file_upload", "HTTP upload", "commit_conversation_file_upload", "send robot file"}})
		}
		ctx, cancel := context.WithTimeout(rt.Command().Context(), messagesSendFileUploadTimeout)
		defer cancel()
		commit, err := uploadBotMessageFile(ctx, target, meta, "")
		if err != nil {
			return err
		}
		link, err := helpers.ParseConversationFileDownloadURL(commit)
		if err != nil {
			return err
		}
		args["fileUrl"] = link
	}
	if len(groups) > 0 {
		args["msgKey"] = key
		items := make([]shortcutBatchWrite, 0, len(groups))
		for _, id := range groups {
			params := make(map[string]any, len(args)+1)
			for k, v := range args {
				params[k] = v
			}
			params["openConversationId"] = id
			items = append(items, shortcutBatchWrite{target: id, arguments: params})
		}
		return executeShortcutBatchWrite(rt, "bot", "send_robot_group_message", items)
	}
	args["msgType"] = key
	if ids := uniqueShortcutStrings(rt.StrSlice("users")); len(ids) > 0 {
		args["userIds"] = ids
	}
	if ids := uniqueShortcutStrings(rt.StrSlice("open-dingtalk-ids")); len(ids) > 0 {
		args["openDingtalkIds"] = ids
	}
	return executeUnifiedMessageWrite(rt, "bot", "batch_send_robot_msg_to_users", args)
}
