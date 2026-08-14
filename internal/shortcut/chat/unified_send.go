// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

const messagesSendFileUploadTimeout = 10 * time.Minute

const (
	messagesSendMaxBotGroups     = 100
	messagesSendMaxGroupFileSize = 1 << 20
)

var messagesSendReadGroupFile = os.ReadFile

// MessagesSend is the identity-aware common sending entry point. The current
// user branch reuses the native message leaf's reviewed file-upload flow and
// existing-mediaId image path. Bot and webhook remain text/Markdown-only
// because their lower transports do not expose equivalent media contracts.
var MessagesSend = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-send",
	Product:     "chat",
	Description: "按身份和目标统一发送消息，Bot 多群返回逐目标 ledger",
	Intent:      "当你需要文件、复杂 @、幂等，或选择 current-user、bot、webhook 身份发送消息时使用；current-user 可直接传稳定 ID，也可用 --user-query/--chat-query 在 CLI 内唯一解析自然目标，dry-run 与真实执行使用同一解析链。Bot 可用 --groups/--groups-file 向最多 100 个稳定群 ID 发送文本或 Markdown，去重后返回 im.batch-write.v1 逐目标 ledger；webhook 目标由 token 所在群决定。文件上传和已有 mediaId 图片仅 current-user 支持，bot/webhook 不支持富媒体。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_messages_send",
			CanonicalPath:  "chat.shortcut_messages_send",
			CLIPath:        "chat +messages-send",
			PrimaryCLIPath: "chat +messages-send",
		},
		Description: "按身份和目标统一发送消息，Bot 多群返回逐目标 ledger",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed composite send adapter: it selects current-user, bot, or webhook transport; current-user additionally supports live-compatible contact search with exact userId matching, mediaId images, and the native init/upload/commit local-file flow.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按身份和目标统一发送消息，Bot 多群返回逐目标 ledger",
			UseWhen:      []string{"当你需要文件、复杂 @、幂等，或选择 current-user、bot、webhook 身份发送消息时使用；current-user 可直接传稳定 ID，也可用 --user-query/--chat-query 在 CLI 内唯一解析自然目标，dry-run 与真实执行使用同一解析链。Bot 可用 --groups/--groups-file 向最多 100 个稳定群 ID 发送文本或 Markdown，去重后返回 im.batch-write.v1 逐目标 ledger；webhook 目标由 token 所在群决定。文件上传和已有 mediaId 图片仅 current-user 支持，bot/webhook 不支持富媒体。"},
			AvoidWhen:    []string{"需要 bot/webhook 发送媒体、卡片或 thread 回复时不要假设等价支持；改用真实存在的专用下层命令，缺少下层能力时停止"},
			Examples: []string{
				"dws chat +messages-send --as user --chat-id <openConversationId> --markdown \"## 周报\" --idempotency-key <key>",
				"dws chat +messages-send --as user --user <userId> --msg-type file --file ./report.pdf",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "identity", Type: shortcut.FlagString, Default: "user", Enum: []string{"user", "bot", "webhook"}, Desc: "发送身份；目标、凭据和幂等参数受发送身份能力矩阵约束"},
		{Name: "as", Type: shortcut.FlagString, Enum: []string{"user", "bot", "webhook"}, Desc: "--identity 的 lark-cli 对齐别名；受发送身份能力矩阵约束"},
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId（user/bot 群聊）；受发送身份能力矩阵约束"},
		{Name: "chat-id", Type: shortcut.FlagString, Desc: "--group 的 lark-cli 对齐别名；受发送身份能力矩阵约束"},
		{Name: "groups", Type: shortcut.FlagStringSlice, Desc: "多个群 openConversationId（仅 bot；受发送身份能力矩阵约束，逐群返回 typed ledger，最多 100 个）"},
		{Name: "groups-file", Type: shortcut.FlagString, Desc: "工作目录内相对文本文件（仅 bot；受发送身份能力矩阵约束），每行或逗号分隔一个群 openConversationId"},
		{Name: "chat-query", Type: shortcut.FlagString, Desc: "按群名解析唯一群聊（仅 user 的高级发送场景）；受发送身份能力矩阵约束"},
		{Name: "user", Type: shortcut.FlagString, Desc: "单聊接收者 userId（user；包括 --dry-run 也会先通过通讯录搜索精确匹配 openDingTalkId）；受发送身份能力矩阵约束"},
		{Name: "user-query", Type: shortcut.FlagString, Desc: "按姓名解析唯一 openDingTalkId（仅 user 的高级发送场景）；受发送身份能力矩阵约束"},
		{Name: "open-dingtalk-id", Type: shortcut.FlagString, Desc: "单聊接收者 openDingTalkId（user）；受发送身份能力矩阵约束"},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "批量单聊接收者 userId（bot）；受发送身份能力矩阵约束"},
		{Name: "open-dingtalk-ids", Type: shortcut.FlagStringSlice, Desc: "批量单聊接收者 openDingTalkId（bot）；受发送身份能力矩阵约束"},
		// Keep required_when out of Schema: validateMessagesSend already enforces
		// bot/webhook credentials, and publishing RequiredWhen breaks merge-base
		// schema-compatibility (null → expression).
		{Name: "robot-code", Type: shortcut.FlagString, Desc: "机器人 Code（identity=bot 时使用）；受发送身份能力矩阵约束"},
		{Name: "webhook-token", Type: shortcut.FlagString, Desc: "自定义机器人 Webhook token（identity=webhook 时使用）；受发送身份能力矩阵约束"},
		{Name: "title", Type: shortcut.FlagString, Desc: "消息标题（不传则从正文生成）"},
		{Name: "text", Type: shortcut.FlagString, Desc: "纯文本正文"},
		{Name: "markdown", Type: shortcut.FlagString, Desc: "Markdown 正文"},
		{Name: "msg-type", Type: shortcut.FlagString, Enum: []string{"text", "markdown", "image", "file", "audio", "video"}, Desc: "内容类型；省略时根据正文、--media-id 或 --file 自动推断"},
		{Name: "media-id", Type: shortcut.FlagString, Desc: "已有图片 mediaId（仅 user 的 image）"},
		{Name: "file", Type: shortcut.FlagString, Desc: "工作目录内安全相对文件路径（仅 user 的 file/audio/video）"},
		{Name: "file-path", Type: shortcut.FlagString, Desc: "--file 的兼容别名"},
		{Name: "uuid", Type: shortcut.FlagString, Desc: "幂等键（仅 user）；受发送身份能力矩阵约束"},
		{Name: "idempotency-key", Type: shortcut.FlagString, Desc: "--uuid 的 lark-cli 对齐别名（仅 user）；受发送身份能力矩阵约束"},
		{Name: "at-open-dingtalk-ids", Type: shortcut.FlagStringSlice, Desc: "@ 的 openDingTalkId（user/bot 群聊）"},
		{Name: "at-user-ids", Type: shortcut.FlagStringSlice, Desc: "@ 的 userId（bot/webhook）"},
		{Name: "at-mobiles", Type: shortcut.FlagStringSlice, Desc: "@ 的手机号（webhook）"},
		{Name: "at-all", Type: shortcut.FlagBool, Desc: "@所有人"},
		shortcut.AIMessageTagFlag(),
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"text", "markdown", "media-id", "file", "file-path"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"text", "markdown", "media-id", "file", "file-path"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"identity", "as"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"group", "chat-id"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"user", "open-dingtalk-id"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"uuid", "idempotency-key"}},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"identity", "as", "group", "chat-id", "groups", "groups-file", "chat-query", "user", "user-query", "open-dingtalk-id", "users", "open-dingtalk-ids", "robot-code", "webhook-token", "uuid", "idempotency-key"},
			Description: "目标、凭据和幂等参数受发送身份能力矩阵约束：user 必须指定一个群聊或单聊目标；bot 必须指定 robot-code 和一类目标，多群最多 100 个并逐项返回 ledger；webhook 必须指定 webhook-token；幂等键仅 user 支持",
		},
	},
	Tips: []string{
		`dws chat +messages-send --as user --chat-id <openConversationId> --markdown "## 周报" --idempotency-key <key>`,
		`dws chat +messages-send --as user --user <userId> --msg-type file --file ./report.pdf --idempotency-key <key>`,
		`dws chat +messages-send --as bot --robot-code <robotCode> --groups <openConversationId1>,<openConversationId2> --text "请提交周报"`,
	},
	Validate: validateMessagesSend,
	Execute:  executeMessagesSend,
}

func validateMessagesSend(rt *shortcut.RuntimeContext) error {
	identity := messagesSendIdentity(rt)
	group := rt.StrFirst("chat-id", "group")
	botGroups, botGroupsErr := messagesSendBotGroups(rt)
	if botGroupsErr != nil {
		return botGroupsErr
	}
	chatQuery := rt.Str("chat-query")
	userID := rt.Str("user")
	userQuery := rt.Str("user-query")
	openID := rt.Str("open-dingtalk-id")
	users := uniqueShortcutStrings(rt.StrSlice("users"))
	openIDs := uniqueShortcutStrings(rt.StrSlice("open-dingtalk-ids"))
	atOpenIDs := uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids"))
	atUserIDs := uniqueShortcutStrings(rt.StrSlice("at-user-ids"))
	atMobiles := uniqueShortcutStrings(rt.StrSlice("at-mobiles"))
	if openID != "" {
		if err := targetresolver.ValidateExplicitOpenDingTalkID("--open-dingtalk-id", openID); err != nil {
			return err
		}
	}
	if err := validateExplicitOpenIDs("--open-dingtalk-ids", openIDs); err != nil {
		return err
	}
	if err := validateExplicitOpenIDs("--at-open-dingtalk-ids", atOpenIDs); err != nil {
		return err
	}
	contentType, err := messagesSendContentType(rt)
	if err != nil {
		return err
	}
	if contentType == "text" && rt.Str("markdown") != "" {
		return apperrors.NewValidation("--msg-type text 必须与 --text 一起使用")
	}
	if contentType == "markdown" && rt.Str("text") != "" {
		return apperrors.NewValidation("--msg-type markdown 必须与 --markdown 一起使用")
	}
	switch identity {
	case "user":
		targetCount := nonEmptyStringCount(group, chatQuery, userID, userQuery, openID)
		if targetCount != 1 {
			return apperrors.NewValidation("--identity user 时 --group/--chat-id、--chat-query、--user、--user-query、--open-dingtalk-id 必须且只能指定一个")
		}
		if len(users) > 0 || len(openIDs) > 0 || len(botGroups) > 0 || rt.Str("robot-code") != "" || rt.Str("webhook-token") != "" {
			return apperrors.NewValidation("--identity user 不接受 bot/webhook 凭据或批量目标")
		}
		if len(atUserIDs) > 0 || len(atMobiles) > 0 {
			return apperrors.NewValidation("--identity user 只接受 --at-open-dingtalk-ids")
		}
		if (userID != "" || userQuery != "" || openID != "") && (len(atOpenIDs) > 0 || rt.Bool("at-all")) {
			return apperrors.NewValidation("user 单聊不接受 @ 参数；@ 只适用于群聊")
		}
		if contentType != "text" && contentType != "markdown" && (len(atOpenIDs) > 0 || rt.Bool("at-all")) {
			return apperrors.NewValidation("user image/file/audio/video 当前不接受 @ 参数")
		}
	case "bot":
		if chatQuery != "" || userQuery != "" {
			return apperrors.NewValidation("--identity bot 当前不接受 --chat-query 或 --user-query；请传真实群 ID 或批量用户 ID")
		}
		if rt.Str("robot-code") == "" {
			return apperrors.NewValidation("--identity bot 必须指定 --robot-code")
		}
		hasDirect := len(users)+len(openIDs) > 0
		hasGroup := group != "" || len(botGroups) > 0
		if hasGroup == hasDirect {
			return apperrors.NewValidation("--identity bot 时单群/多群与批量单聊目标必须且只能指定一类")
		}
		if group != "" && len(botGroups) > 0 {
			return apperrors.NewValidation("--identity bot 时 --group/--chat-id 与 --groups/--groups-file 不能同时使用")
		}
		if userID != "" || openID != "" || rt.Str("webhook-token") != "" {
			return apperrors.NewValidation("--identity bot 不接受 --user、--open-dingtalk-id 或 --webhook-token")
		}
		if len(atMobiles) > 0 {
			return apperrors.NewValidation("--identity bot 不接受 --at-mobiles")
		}
		if hasDirect && (len(atOpenIDs) > 0 || len(atUserIDs) > 0) {
			return apperrors.NewValidation("bot 批量单聊不接受指定 @对象；@对象只适用于 bot 群聊")
		}
		if messagesSendIdempotencyKey(rt) != "" {
			return apperrors.NewValidation("--uuid 当前仅 user 身份的下层支持")
		}
		if !messageIdentitySupportsContent(identity, contentType) {
			return apperrors.NewValidation("--identity bot 当前下层只支持 text/markdown")
		}
	case "webhook":
		if chatQuery != "" || userQuery != "" {
			return apperrors.NewValidation("--identity webhook 的目标由 token 所在群决定，不接受 --chat-query 或 --user-query")
		}
		if rt.Str("webhook-token") == "" {
			return apperrors.NewValidation("--identity webhook 必须指定 --webhook-token")
		}
		if group != "" || len(botGroups) > 0 || userID != "" || openID != "" || len(users) > 0 || len(openIDs) > 0 || rt.Str("robot-code") != "" {
			return apperrors.NewValidation("--identity webhook 的目标由 token 所在群决定，不接受其他目标或 bot Code")
		}
		if len(atOpenIDs) > 0 {
			return apperrors.NewValidation("--identity webhook 不接受 --at-open-dingtalk-ids")
		}
		if messagesSendIdempotencyKey(rt) != "" {
			return apperrors.NewValidation("--uuid 当前仅 user 身份的下层支持")
		}
		if !messageIdentitySupportsContent(identity, contentType) {
			return apperrors.NewValidation("--identity webhook 当前下层只支持 text/markdown")
		}
	}
	return nil
}

func executeMessagesSend(rt *shortcut.RuntimeContext) error {
	identity := messagesSendIdentity(rt)
	body := messagesSendBody(rt)
	title := rt.Str("title")
	if title == "" {
		title = shortcutMessageTitle(body)
	}
	switch identity {
	case "user":
		contentType, err := messagesSendContentType(rt)
		if err != nil {
			return err
		}
		group, openID, err := messagesSendUserTarget(rt)
		if err != nil {
			return err
		}
		if contentType == "image" {
			content, _ := json.Marshal(map[string]string{"mediaId": rt.Str("media-id")})
			params := rt.AddAIMessageTag(map[string]any{
				"msgType": "image",
				"content": string(content),
			})
			addMessagesSendUserTarget(params, group, openID)
			if value := messagesSendIdempotencyKey(rt); value != "" {
				params["uuid"] = value
			}
			return executeUnifiedMessageWrite(rt, "chat", "send_personal_message", params)
		}
		if contentType == "file" || contentType == "audio" || contentType == "video" {
			return executeMessagesSendUserFile(rt, group, openID, contentType)
		}
		params := resolvedUserMarkdownParams(rt, ResolvedUserMessageTarget{
			GroupID:        group,
			OpenDingTalkID: openID,
		}, title, body, uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids")), rt.Bool("at-all"), messagesSendIdempotencyKey(rt))
		return executeUnifiedMessageWrite(rt, "chat", "send_personal_message", params)
	case "bot":
		body = helpers.NormalizeMessageMentions(
			body,
			append(
				uniqueShortcutStrings(rt.StrSlice("at-user-ids")),
				uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids"))...,
			),
			rt.Bool("at-all"),
			false,
		)
		params := map[string]any{
			"robotCode": rt.Str("robot-code"),
			"title":     title,
			"markdown":  body,
		}
		if group := rt.StrFirst("chat-id", "group"); group != "" {
			params["openConversationId"] = group
			if values := uniqueShortcutStrings(rt.StrSlice("at-user-ids")); len(values) > 0 {
				params["atUserIds"] = values
			}
			if values := uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids")); len(values) > 0 {
				params["atOpendingtalkIds"] = values
			}
			if rt.Bool("at-all") {
				params["isAtAll"] = "true"
			}
			return executeUnifiedMessageWrite(rt, "bot", "send_robot_group_message", params)
		}
		groups, err := messagesSendBotGroups(rt)
		if err != nil {
			return err
		}
		if len(groups) > 0 {
			items := make([]shortcutBatchWrite, 0, len(groups))
			for _, group := range groups {
				arguments := make(map[string]any, len(params)+1)
				for key, value := range params {
					arguments[key] = value
				}
				arguments["openConversationId"] = group
				if values := uniqueShortcutStrings(rt.StrSlice("at-user-ids")); len(values) > 0 {
					arguments["atUserIds"] = values
				}
				if values := uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids")); len(values) > 0 {
					arguments["atOpendingtalkIds"] = values
				}
				if rt.Bool("at-all") {
					arguments["isAtAll"] = "true"
				}
				items = append(items, shortcutBatchWrite{target: group, arguments: arguments})
			}
			return executeShortcutBatchWrite(rt, "bot", "send_robot_group_message", items)
		}
		if values := uniqueShortcutStrings(rt.StrSlice("users")); len(values) > 0 {
			params["userIds"] = values
		}
		if values := uniqueShortcutStrings(rt.StrSlice("open-dingtalk-ids")); len(values) > 0 {
			params["openDingtalkIds"] = values
		}
		if rt.Bool("at-all") {
			params["isAtAll"] = "true"
		}
		return executeUnifiedMessageWrite(rt, "bot", "batch_send_robot_msg_to_users", params)
	case "webhook":
		body = helpers.NormalizeMessageMentions(
			body,
			append(
				uniqueShortcutStrings(rt.StrSlice("at-user-ids")),
				uniqueShortcutStrings(rt.StrSlice("at-mobiles"))...,
			),
			rt.Bool("at-all"),
			false,
		)
		params := map[string]any{
			"robotToken": rt.Str("webhook-token"),
			"title":      title,
			"text":       body,
		}
		if values := uniqueShortcutStrings(rt.StrSlice("at-user-ids")); len(values) > 0 {
			params["atUserIds"] = values
		}
		if values := uniqueShortcutStrings(rt.StrSlice("at-mobiles")); len(values) > 0 {
			params["atMobiles"] = values
		}
		if rt.Bool("at-all") {
			params["isAtAll"] = true
		}
		return executeUnifiedMessageWrite(rt, "bot", "send_message_by_custom_robot", params)
	default:
		return fmt.Errorf("unsupported identity %q", identity)
	}
}

func executeUnifiedMessageWrite(rt *shortcut.RuntimeContext, product, tool string, arguments map[string]any) error {
	if rt.DryRun() {
		return rt.Output(map[string]any{
			"dry_run":      true,
			"executed":     false,
			"preview_kind": "plan",
			"tool":         tool,
			"actionCount":  1,
			"failedCount":  0,
			"actions": []map[string]any{{
				"identity":  messagesSendIdentity(rt),
				"tool":      tool,
				"arguments": arguments,
			}},
		})
	}
	data, err := rt.CallMCPWriteData(product, tool, arguments)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"ok":       true,
		"identity": messagesSendIdentity(rt),
		"tool":     tool,
		"result":   data,
	}
	if messagesSendIdentity(rt) == "user" && tool == "send_personal_message" {
		payload["sendReceipt"] = chatmsg.ProjectMessageSendReceipt(data)
	}
	return rt.Output(payload)
}

func messagesSendIdentity(rt *shortcut.RuntimeContext) string {
	return rt.StrFirst("as", "identity")
}

func messagesSendBody(rt *shortcut.RuntimeContext) string {
	return rt.StrFirst("markdown", "text")
}

func messagesSendContentType(rt *shortcut.RuntimeContext) (string, error) {
	contentType := strings.ToLower(rt.Str("msg-type"))
	switch {
	case rt.Str("media-id") != "":
		if contentType != "" && contentType != "image" {
			return "", apperrors.NewValidation("--media-id 只支持 --msg-type image")
		}
		return "image", nil
	case rt.StrFirst("file", "file-path") != "":
		if contentType == "" {
			return "file", nil
		}
		if contentType != "file" && contentType != "audio" && contentType != "video" {
			return "", apperrors.NewValidation("--file 只支持 --msg-type file/audio/video")
		}
		return contentType, nil
	default:
		if contentType == "" {
			if rt.Str("markdown") != "" {
				return "markdown", nil
			}
			return "text", nil
		}
		if contentType != "text" && contentType != "markdown" {
			return "", apperrors.NewValidation(fmt.Sprintf("--msg-type %s 需要匹配的 --media-id 或 --file", contentType))
		}
		return contentType, nil
	}
}

func messagesSendUserTarget(rt *shortcut.RuntimeContext) (group, openID string, err error) {
	group = rt.StrFirst("chat-id", "group")
	openID = rt.Str("open-dingtalk-id")
	if openID != "" || group != "" {
		return group, openID, nil
	}
	if query := rt.Str("chat-query"); query != "" {
		resolved, resolveErr := targetresolver.ResolveChat(rt, query)
		if resolveErr != nil {
			return "", "", resolveErr
		}
		return resolved.Selected.OpenConversationID, "", nil
	}
	if query := rt.Str("user-query"); query != "" {
		resolved, resolveErr := targetresolver.ResolveUser(rt, query, targetresolver.IdentityOpenDingTalkID)
		if resolveErr != nil {
			return "", "", resolveErr
		}
		return "", resolved.Selected.OpenDingTalkID, nil
	}
	openID, err = resolveUserOpenDingTalkID(rt, rt.Str("user"))
	if err != nil {
		return "", "", err
	}
	return "", openID, nil
}

// ResolvedUserMessageTarget is the stable target accepted by the shared user
// send engine after natural-name resolution has completed.
type ResolvedUserMessageTarget struct {
	GroupID        string
	OpenDingTalkID string
}

// ExecuteResolvedUserMarkdown lets narrow semantic shortcuts such as +dm and
// +send-to-group reuse the same target/content/AI-tag parameter builder while
// preserving their existing raw lower-response output contract.
func ExecuteResolvedUserMarkdown(
	rt *shortcut.RuntimeContext,
	target ResolvedUserMessageTarget,
	text string,
) error {
	params := resolvedUserMarkdownParams(
		rt,
		target,
		text,
		text,
		nil,
		false,
		"",
	)
	return rt.CallMCP("send_personal_message", params)
}

func resolvedUserMarkdownParams(
	rt *shortcut.RuntimeContext,
	target ResolvedUserMessageTarget,
	title, body string,
	atOpenIDs []string,
	atAll bool,
	idempotencyKey string,
) map[string]any {
	if target.GroupID != "" {
		body = helpers.NormalizeMessageMentions(body, atOpenIDs, atAll, true)
	}
	content, _ := json.Marshal(map[string]string{"title": title, "text": body})
	params := rt.AddAIMessageTag(map[string]any{
		"msgType": "markdown",
		"content": string(content),
	})
	addMessagesSendUserTarget(params, target.GroupID, target.OpenDingTalkID)
	if target.GroupID != "" {
		if len(atOpenIDs) > 0 {
			params["atOpenDingTalkIds"] = atOpenIDs
		}
		if atAll {
			params["atAll"] = true
		}
	}
	if idempotencyKey != "" {
		params["uuid"] = idempotencyKey
	}
	return params
}

func resolveUserOpenDingTalkID(rt *shortcut.RuntimeContext, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	data, err := rt.CallMCPData("contact", "search_contact_by_key_word", map[string]any{
		"keyword": userID,
	})
	if err != nil {
		return "", fmt.Errorf("通过通讯录搜索把 userId %q 解析为 openDingTalkId 失败: %w", userID, err)
	}

	exactRows := 0
	openIDs := make([]string, 0, 1)
	for _, user := range shortcutMapSlice(data["result"]) {
		if shortcutString(user, "userId", "userID") != userID {
			continue
		}
		exactRows++
		if openID := shortcutString(user, "openDingTalkId", "openDingtalkId"); openID != "" {
			openIDs = appendUniqueShortcutString(openIDs, openID)
		}
	}
	switch {
	case len(openIDs) == 1:
		return openIDs[0], nil
	case len(openIDs) > 1:
		return "", apperrors.NewValidation(fmt.Sprintf(
			"通讯录中 userId %q 精确匹配到多个不同的 openDingTalkId，无法安全选择",
			userID,
		))
	case exactRows > 0:
		return "", apperrors.NewValidation(fmt.Sprintf(
			"通讯录已精确匹配 userId %q，但结果未返回 openDingTalkId",
			userID,
		))
	default:
		return "", apperrors.NewValidation(fmt.Sprintf(
			"通讯录搜索结果中没有精确匹配的 userId %q",
			userID,
		))
	}
}

func executeMessagesSendUserFile(
	rt *shortcut.RuntimeContext,
	group, openID, requestedType string,
) error {
	rawPath := rt.StrFirst("file", "file-path")
	safePath, err := apperrors.SafeInputPath(rawPath)
	if err != nil {
		return err
	}
	meta, err := helpers.BuildConversationLocalFileMeta(safePath, "", "")
	if err != nil {
		return err
	}
	targetArgs := map[string]any{}
	addMessagesSendUserTarget(targetArgs, group, openID)
	idempotencyKey := messagesSendIdempotencyKey(rt)
	if rt.DryRun() {
		return rt.Output(map[string]any{
			"dry_run":      true,
			"executed":     false,
			"preview_kind": "plan",
			"actionCount":  2,
			"failedCount":  0,
			"actions": []map[string]any{
				{
					"identity": "user",
					"tool":     "init/commit_conversation_file_upload",
					"file": map[string]any{
						"path":      rawPath,
						"name":      meta.FileName,
						"sizeBytes": meta.FileSize,
					},
				},
				{
					"identity":             "user",
					"tool":                 "send_personal_message",
					"requestedMessageType": requestedType,
					"effectiveMessageType": "file",
					"target":               targetArgs,
				},
			},
		})
	}
	uploadContext, cancelUpload := context.WithTimeout(
		rt.Command().Context(), messagesSendFileUploadTimeout)
	defer cancelUpload()
	commitText, err := helpers.UploadConversationLocalFile(
		uploadContext, targetArgs, meta, idempotencyKey)
	if err != nil {
		return err
	}
	dentryID, spaceID, err := helpers.ParseConversationFileSendIDs(commitText)
	if err != nil {
		return err
	}
	content, _ := helpers.BuildConversationFileContent(dentryID, spaceID, meta)
	params := rt.AddAIMessageTag(map[string]any{
		"msgType": "file",
		"content": content,
	})
	addMessagesSendUserTarget(params, group, openID)
	if idempotencyKey != "" {
		params["uuid"] = idempotencyKey
	}
	data, err := rt.CallMCPWriteData("chat", "send_personal_message", params)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"ok":                   true,
		"identity":             "user",
		"tool":                 "send_personal_message",
		"requestedMessageType": requestedType,
		"effectiveMessageType": "file",
		"file": map[string]any{
			"path":      rawPath,
			"name":      meta.FileName,
			"sizeBytes": meta.FileSize,
		},
		"result": data,
	}
	payload["sendReceipt"] = chatmsg.ProjectMessageSendReceipt(data)
	return rt.Output(payload)
}

func addMessagesSendUserTarget(params map[string]any, group, openID string) {
	if group != "" {
		params["openConversationId"] = group
		return
	}
	params["receiverOpenDingTalkId"] = openID
}

func nonEmptyStringCount(values ...string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func messagesSendIdempotencyKey(rt *shortcut.RuntimeContext) string {
	return rt.StrFirst("idempotency-key", "uuid")
}

func messagesSendBotGroups(rt *shortcut.RuntimeContext) ([]string, error) {
	if rt.Changed("groups") && rt.Changed("groups-file") {
		return nil, apperrors.NewValidation("--groups 与 --groups-file 不能同时指定")
	}
	groups := uniqueShortcutStrings(rt.StrSlice("groups"))
	if path := rt.Str("groups-file"); path != "" {
		safePath, err := apperrors.SafeInputPath(path)
		if err != nil {
			return nil, fmt.Errorf("校验 --groups-file 失败: %w", err)
		}
		info, err := os.Stat(safePath)
		if err != nil {
			return nil, fmt.Errorf("读取 --groups-file 失败: %w", err)
		}
		if !info.Mode().IsRegular() {
			return nil, apperrors.NewValidation("--groups-file 必须是普通文本文件")
		}
		if info.Size() > messagesSendMaxGroupFileSize {
			return nil, apperrors.NewValidation("--groups-file 不能超过 1 MiB")
		}
		raw, err := messagesSendReadGroupFile(safePath)
		if err != nil {
			return nil, fmt.Errorf("读取 --groups-file 失败: %w", err)
		}
		values := make([]string, 0)
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			values = append(values, strings.Split(line, ",")...)
		}
		groups = uniqueShortcutStrings(values)
	}
	if len(groups) > messagesSendMaxBotGroups {
		return nil, apperrors.NewValidation(fmt.Sprintf(
			"bot 多群发送最多支持 %d 个群，当前 %d 个",
			messagesSendMaxBotGroups, len(groups),
		))
	}
	if (rt.Changed("groups") || rt.Changed("groups-file")) && len(groups) == 0 {
		return nil, apperrors.NewValidation("bot 多群发送至少需要一个 openConversationId")
	}
	return groups, nil
}

func shortcutMessageTitle(text string) string {
	text = strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if utf8.RuneCountInString(text) <= 40 {
		return text
	}
	runes := []rune(text)
	return string(runes[:40])
}

func init() {
	shortcut.Register(withReviewedChatShortcutContracts(MessagesSend)...)
}
