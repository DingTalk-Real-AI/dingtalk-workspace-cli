// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

func validateReplyExtensions(rt *shortcut.RuntimeContext) error {
	if rt.Bool("create-thread") && (!rt.Bool("reply-in-thread") || rt.StrFirst("identity", "as") == "bot" || rt.Str("thread-id") != "") {
		return apperrors.NewValidation("--create-thread 仅配合个人reply-in-thread，不接受显式thread-id")
	}
	ids := uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids"))
	if err := validateExplicitOpenIDs("--at-open-dingtalk-ids", ids); err != nil {
		return err
	}
	if rt.Str("open-dingtalk-id") != "" && (rt.Bool("at-all") || len(ids) > 0) {
		return apperrors.NewValidation("@ 参数仅支持群回复，不支持单聊")
	}
	body := rt.StrFirst("text", "markdown", "content")
	if strings.TrimSpace(body) == "" {
		return apperrors.NewValidation("回复正文不能为空")
	}
	declared := map[string]bool{}
	for _, id := range ids {
		declared[id] = true
	}
	for _, id := range currentUserMentionBodyIDs(body) {
		if !declared[id] {
			return apperrors.NewValidation("正文 @成员占位符必须通过 --at-open-dingtalk-ids 声明")
		}
	}
	if containsCurrentUserMentionToken(body, "all") && !rt.Bool("at-all") {
		return apperrors.NewValidation("正文 <@all> 必须同时指定 --at-all")
	}

	if rt.StrFirst("identity", "as") == "bot" {
		if rt.Str("robot-code") == "" {
			return apperrors.NewValidation("Bot 回复必须指定 --robot-code")
		}
		if rt.Bool("reply-in-thread") || rt.Str("open-dingtalk-id") != "" || rt.StrFirst("uuid", "idempotency-key") != "" {
			return apperrors.NewValidation("Bot 仅支持群引用回复，不支持 Thread、单聊引用或幂等键")
		}
	} else if rt.Str("robot-code") != "" {
		return apperrors.NewValidation("--robot-code 仅支持 --as bot")
	}
	if rt.Str("thread-id") != "" && !rt.Bool("reply-in-thread") {
		return apperrors.NewValidation("--thread-id 仅用于 --reply-in-thread")
	}
	if rt.Bool("reply-in-thread") && rt.Str("open-dingtalk-id") != "" {
		return apperrors.NewValidation("Thread 追加与单聊引用互斥")
	}
	if rt.Bool("reply-in-thread") && rt.Str("ref-sender") != "" {
		return apperrors.NewValidation("Thread 追加不使用 --ref-sender")
	}
	if id := rt.Str("open-dingtalk-id"); id != "" {
		return targetresolver.ValidateExplicitOpenDingTalkID("--open-dingtalk-id", id)
	}
	return nil
}

// All reply modes require exact source identity. An omitted conversation may
// be resolved from the message; a missing response identity is never a match.
func exactChatMessage(rt *shortcut.RuntimeContext, conversationID, messageID string) (map[string]any, error) {
	data, err := rt.CallMCPData("im", "list_messages_by_ids", map[string]any{"openMsgIds": []string{messageID}})
	if err != nil {
		return nil, err
	}
	var found map[string]any
	for _, message := range shortcutMessageMaps(data) {
		if fmt.Sprint(chatmsg.MessageID(message)) != messageID {
			continue
		}
		cid := shortcutString(message, "openConversationId", "openconversationId", "conversationId", "openCid")
		if cid == "" || (conversationID != "" && cid != conversationID) {
			return nil, apperrors.NewValidation("源消息会话与 --group/--conversation-id 不一致或下游未提供会话身份")
		}
		if found != nil {
			return nil, apperrors.NewValidation("消息 ID 返回重复记录，无法确定唯一源消息")
		}
		found = message
	}
	if found == nil {
		return nil, apperrors.NewValidation("未找到精确匹配的源消息 ID；未执行写入")
	}
	return found, nil
}

type replyTarget struct {
	message        map[string]any
	conversationID string
	sender         string
}

func resolveReplyTarget(rt *shortcut.RuntimeContext) (replyTarget, error) {
	message, err := exactChatMessage(rt, replyConversationID(rt), replyMessageID(rt))
	if err != nil {
		return replyTarget{}, err
	}
	sender := findMessageSenderOpenDingTalkID(message)
	if sender == "" && !rt.Bool("reply-in-thread") {
		return replyTarget{}, apperrors.NewValidation("源消息缺少发送者 openDingTalkId，未执行回复")
	}
	if rt.Str("ref-sender") != "" {
		explicit, err := resolveReplySender(rt)
		if err != nil {
			return replyTarget{}, err
		}
		if explicit != sender {
			return replyTarget{}, apperrors.NewValidation("--ref-sender 与源消息发送者不一致")
		}
	}
	return replyTarget{message: message, conversationID: shortcutString(message, "openConversationId", "openconversationId", "conversationId", "openCid"), sender: sender}, nil
}

func replyMentionBody(rt *shortcut.RuntimeContext, user bool) string {
	return helpers.PrepareChatReplyMentions(rt.StrFirst("text", "markdown", "content"), uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids")), rt.Bool("at-all"), user)
}

func addReplyMentionParams(rt *shortcut.RuntimeContext, args map[string]any, bot bool) {
	ids := uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids"))
	if bot {
		if len(ids) > 0 {
			args["atOpendingtalkIds"] = ids
		}
		if rt.Bool("at-all") {
			args["isAtAll"] = "true"
		}
	} else {
		if len(ids) > 0 {
			args["atOpenDingTalkIds"] = ids
		}
		if rt.Bool("at-all") {
			args["atAll"] = true
		}
	}
}

func executeReplyExtensions(rt *shortcut.RuntimeContext, target replyTarget) error {
	message := target.message
	thread := shortcutString(message, "openConvThreadId", "threadId", "topicId")
	if rt.Bool("reply-in-thread") {
		promoted := false
		if (thread == "" || thread == target.conversationID) && rt.Bool("create-thread") {
			promoteArgs := map[string]any{"openConversationId": target.conversationID, "openMessageId": replyMessageID(rt)}
			if rt.DryRun() {
				return rt.Output(map[string]any{"dryRun": true, "willSend": false, "steps": []any{map[string]any{"tool": "im/convert_message_to_thread", "arguments": promoteArgs}, map[string]any{"tool": "chat/send_personal_message", "target": "new Thread", "content": replyMentionBody(rt, true)}}})
			}
			data, err := rt.CallMCPWriteData("im", "convert_message_to_thread", promoteArgs)
			if err != nil {
				return err
			}
			value := replyResponseValue(data, "openConvThreadId")
			thread, _ = value.(string)
			if thread == "" || thread == target.conversationID {
				return apperrors.NewAPI("Thread转换已请求但结果未知；请核对源消息后再重试", apperrors.WithDetails(map[string]any{"sourceMessageId": replyMessageID(rt)}))
			}
			promoted = true
		}

		if thread == "" || thread == target.conversationID {
			return apperrors.NewValidation("源消息未提供可确认的 Thread 子会话 openConvThreadId；不能用父群代替")
		}
		if want := rt.Str("thread-id"); want != "" && want != thread {
			return apperrors.NewValidation("--thread-id 与源消息所属 Thread 不一致")
		}
		body := replyMentionBody(rt, true)
		args := resolvedUserMarkdownParams(rt, ResolvedUserMessageTarget{GroupID: thread}, shortcutMessageTitle(body), body, uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids")), rt.Bool("at-all"), rt.StrFirst("idempotency-key", "uuid"))
		err := executeReplyTransport(rt, "chat", "send_personal_message", args, thread, "thread")
		if err != nil && promoted {
			return apperrors.NewAPI("Thread已转换，但回复未确认成功；核实发送状态后恢复", apperrors.WithDetails(map[string]any{"threadId": thread, "sourceMessageId": replyMessageID(rt), "cause": err.Error()}))
		}
		return err
	}
	sender := target.sender
	body := rt.StrFirst("text", "markdown", "content")
	if rt.StrFirst("identity", "as") == "bot" {
		if thread != "" {
			return apperrors.NewValidation("Bot 引用暂不支持 Thread 消息，请使用个人 --reply-in-thread")
		}
		if err := helpers.ValidateChatQuoteReply(rt.Command(), target.conversationID, replyMessageID(rt)); err != nil {
			return err
		}
		args := map[string]any{"robotCode": rt.Str("robot-code"), "openConversationId": target.conversationID, "msgKey": "sampleMarkdownDX", "title": shortcutMessageTitle(body), "markdown": replyMentionBody(rt, false), "referenceOpenMessageId": replyMessageID(rt), "srcMsgSendOpenDingTalkId": sender}
		addReplyMentionParams(rt, args, true)
		return executeReplyTransport(rt, "bot", "send_robot_group_message", args, target.conversationID, "quote")
	}
	// Verify the direct recipient maps to the supplied conversation before writing.
	peer := rt.Str("open-dingtalk-id")
	info, err := rt.CallMCPData("chat", "get_conversation_info", map[string]any{"openDingTalkId": peer})
	if err != nil {
		return err
	}
	scope := info
	if result, ok := scope["result"].(map[string]any); ok {
		scope = result
	}
	if conversation, ok := scope["conversationInfo"].(map[string]any); ok {
		scope = conversation
	}
	cid := replyResponseValue(scope, "openConversationId", "openCid", "conversationId")
	if fmt.Sprint(cid) != target.conversationID {
		return apperrors.NewValidation("单聊接收者与源消息会话不匹配，未执行回复")
	}
	content, _ := json.Marshal(map[string]string{"referenceOpenMessageId": replyMessageID(rt), "srcMsgSendOpenDingTalkId": sender, "replyMsgType": "text", "content": body})
	args := rt.AddAIMessageTag(map[string]any{"receiverOpenDingTalkId": peer, "msgType": "reply", "content": string(content)})
	if key := rt.StrFirst("idempotency-key", "uuid"); key != "" {
		args["uuid"] = key
	}
	return executeReplyTransport(rt, "chat", "send_personal_message", args, target.conversationID, "quote")
}

func executeReplyTransport(rt *shortcut.RuntimeContext, product, tool string, args map[string]any, target, mode string) error {
	if rt.DryRun() {
		return rt.Output(map[string]any{"dryRun": true, "willSend": false, "transport": product + "/" + tool, "arguments": args, "mode": mode, "conversationId": target})
	}
	data, err := rt.CallMCPWriteData(product, tool, args)
	if err != nil {
		return err
	}
	return rt.Output(map[string]any{"identity": rt.StrFirst("identity", "as"), "mode": mode, "conversationId": target, "sourceMessageId": replyMessageID(rt), "result": data, "sendReceipt": chatmsg.ProjectMessageSendReceipt(data)})
}
