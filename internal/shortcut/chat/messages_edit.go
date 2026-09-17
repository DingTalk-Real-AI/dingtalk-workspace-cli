// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package chat

import (
	"encoding/json"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const editMessageConstraint = "Markdown 正文非空；content 仅含 text/title，title 只用于 text 模式；@ 占位符须与显式成员一致"

func newMessagesEdit() shortcut.Shortcut {
	s := shortcut.Shortcut{
		OutputRollout: output.RolloutUnifiedActive,
		Service:       "chat", Command: "+messages-edit", Product: "im",
		Description: "编辑当前用户自己的 Markdown 消息（不支持 Bot 普通消息编辑）",
		Intent:      "已知会话和消息 ID，需要修改个人已发送消息的正文、标题或 @ 时使用；先核实源消息 ID 与会话归属，写权限由下游校验。与 Lark 同名但仅个人 Markdown 子集；Bot text/post 编辑尚无等价合同。原 chat message edit 保留。",
		Risk:        shortcut.RiskWrite, Safety: reviewedChatShortcutSafety(shortcut.RiskWrite),
		Flags: []shortcut.Flag{
			{Name: "group", Type: shortcut.FlagString, Desc: "消息所在会话 openConversationId", Aliases: []string{"conversation-id", "chat-id"}},
			{Name: "message-id", Type: shortcut.FlagString, Required: true, Desc: "本人已发送的 openMessageId"},
			{Name: "as", Type: shortcut.FlagString, Default: "user", Enum: []string{"user"}, Aliases: []string{"identity"}, Desc: "仅 user；不支持 Lark Bot text/post 编辑"},
			{Name: "text", Type: shortcut.FlagString, Desc: "新的 Markdown 正文" + "；" + editMessageConstraint, Aliases: []string{"markdown"}, Input: []string{"file", "stdin"}},
			{Name: "title", Type: shortcut.FlagString, Desc: "新标题；仅 --text 模式，省略则从正文生成" + "；" + editMessageConstraint},
			{Name: "content", Type: shortcut.FlagString, Desc: "完整 Markdown JSON，包含 text，可选 title" + "；" + editMessageConstraint, Input: []string{"file", "stdin"}},
			{Name: "at-open-dingtalk-ids", Type: shortcut.FlagStringSlice, Desc: "@成员 openDingTalkId" + "；" + editMessageConstraint},
			{Name: "at-all", Type: shortcut.FlagBool, Desc: "@所有人" + "；" + editMessageConstraint},
		},
		Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"text", "content", "title", "at-open-dingtalk-ids", "at-all"}, Description: editMessageConstraint}, {Kind: shortcut.ConstraintExactlyOne, Flags: []string{"text", "content"}}},
		Tips:        []string{"dws chat +messages-edit --group <openConversationId> --message-id <openMessageId> --text \"更新后的内容\""},
		Validate:    func(rt *shortcut.RuntimeContext) error { _, err := editMessageContent(rt); return err },
		Execute: func(rt *shortcut.RuntimeContext) error {
			content, err := editMessageContent(rt)
			if err != nil {
				return err
			}
			message, err := exactChatMessage(rt, rt.StrFirst("group", "conversation-id", "chat-id"), rt.Str("message-id"))
			if err != nil {
				return err
			}
			args := map[string]any{"openConversationId": shortcutString(message, "openConversationId", "conversationId", "openCid"), "openMessageId": rt.Str("message-id"), "content": content}
			if rt.Bool("at-all") {
				args["atAll"] = true
			}
			if ids := uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids")); len(ids) > 0 {
				args["atOpenDingTalkIds"] = ids
			}
			if rt.DryRun() {
				return rt.Output(map[string]any{"dry_run": true, "executed": false, "tool": "im/edit_message", "arguments": args})
			}
			result, err := rt.CallMCPWriteData("im", "edit_message", args)
			if err != nil {
				return err
			}
			if replyResponseValue(result, "success") != true {
				started := true
				return output.StoreResult(rt.Command().Context(), output.Failure(&output.ErrorInfo{Type: "api", Subtype: "projection_unknown", Message: "编辑请求已返回，但缺少明确成功证据；请读取目标消息核实", ExecutionStarted: &started, Details: map[string]any{"conversationId": shortcutString(message, "openConversationId", "conversationId", "openCid"), "messageId": rt.Str("message-id"), "result": result}}))
			}
			return rt.Output(map[string]any{"identity": "user", "conversationId": shortcutString(message, "openConversationId", "conversationId", "openCid"), "messageId": rt.Str("message-id"), "result": result, "verification": "not_performed"})
		},
	}
	s.Contract = reviewedChatShortcutContract(s)
	s.Contract.Result = &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","description":"个人消息编辑的 API 回执或无写入预览；不表示已独立读回验证","properties":{"identity":{"type":"string","description":"编辑身份 user"},"conversationId":{"type":"string","description":"核实的会话 ID"},"messageId":{"type":"string","description":"核实的消息 ID"},"result":{"type":"object","description":"下游编辑回执","additionalProperties":true},"verification":{"type":"string","description":"是否进行了独立写后读回"},"arguments":{"type":"object","description":"dry-run 的精确请求参数","additionalProperties":true}},"additionalProperties":true}`)}
	return s
}

func editMessageContent(rt *shortcut.RuntimeContext) (string, error) {
	ids := uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids"))
	if err := validateExplicitOpenIDs("--at-open-dingtalk-ids", ids); err != nil {
		return "", err
	}
	body, title := rt.StrFirst("text", "markdown"), rt.Str("title")
	if raw := rt.Str("content"); raw != "" {
		if title != "" {
			return "", apperrors.NewValidation("--title 仅用于 --text 模式")
		}
		var value map[string]string
		if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil || value["text"] == "" {
			return "", apperrors.NewValidation("--content 必须是含非空 text 的 Markdown JSON 对象")
		}
		for key := range value {
			if key != "text" && key != "title" {
				return "", apperrors.NewValidation("--content 仅接受 text/title；不支持 post/附件区")
			}
		}
		body, title = value["text"], value["title"]
	} else if title == "" {
		title = shortcutMessageTitle(body)
	}
	if body == "" {
		return "", apperrors.NewValidation("编辑正文不能为空")
	}
	if err := validateCurrentUserMentionConsistency(body, ids, rt.Bool("at-all")); err != nil {
		return "", err
	}
	body = helpers.NormalizeMessageMentions(body, ids, rt.Bool("at-all"), true)
	value, _ := json.Marshal(map[string]string{"text": body, "title": title})
	return string(value), nil
}

func init() { shortcut.Register(newMessagesEdit()) }
