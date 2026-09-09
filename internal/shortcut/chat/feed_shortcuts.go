// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"encoding/json"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// These are fixed-action adapters, not Cobra aliases of a toggle: invoking
// remove must never inherit the toggle's default top=true behavior.
func newFeedShortcut(top bool) shortcut.Shortcut {
	name, description := "+feed-shortcut-create", "置顶指定会话（固定设置置顶；不支持 head/tail 顺序）"
	if !top {
		name, description = "+feed-shortcut-remove", "取消指定会话置顶（固定取消，不需要 --off）"
	}
	s := shortcut.Shortcut{
		OutputRollout: output.RolloutUnifiedActive,
		Service:       "chat", Command: name, Product: "im", Description: description,
		Intent: description + "；仅个人会话，支持 1–10 个稳定会话 ID，逐项返回成功和失败；原 +conversation-set-top 仍保留。",
		Risk:   shortcut.RiskWrite, Safety: reviewedChatShortcutSafety(shortcut.RiskWrite),
		Flags: []shortcut.Flag{
			{Name: "conversation-id", Type: shortcut.FlagString, Desc: "单个会话 openConversationId；会话 ID 去重后必须为 1-10 个"},
			{Name: "conversation-ids", Type: shortcut.FlagStringSlice, Desc: "多个会话 openConversationId；会话 ID 去重后必须为 1-10 个", Aliases: []string{"chat-id", "chat-ids"}},
		},
		Constraints: ConversationSetTop.Constraints,
		Validate:    ConversationSetTop.Validate,
		Tips:        []string{"dws chat " + name + " --conversation-id <openConversationId>"},
		Execute:     func(rt *shortcut.RuntimeContext) error { return executeFixedFeedShortcut(rt, top) },
	}
	s.Contract = reviewedChatShortcutContract(s)
	s.Contract.Result = &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePartialFailure, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"固定置顶动作的逐会话回执或预览；异常目标可能未知，需要读回确认","properties":{"total":{"type":"integer","description":"去重后的会话数量"},"succeeded":{"type":"array","description":"API确认的成功会话及回执","items":{"type":"object","properties":{"id":{"type":"string","description":"会话ID"},"result":{"type":"object","description":"下游回执","additionalProperties":true}}}},"failed":{"type":"array","description":"明确失败的会话","items":{"type":"object"}},"unknown":{"type":"array","description":"写入终态无法确认的会话，禁止盲目重试","items":{"type":"object","properties":{"id":{"type":"string","description":"会话ID"},"reason":{"type":"string","description":"未确认的原因"}}}},"actions":{"type":"array","description":"dry-run 精确计划","items":{"type":"object"}}},"additionalProperties":true}`),
	}
	return s
}

func init() { shortcut.Register(newFeedShortcut(true), newFeedShortcut(false)) }

var newFeedPartialData = output.NewPartialData

func executeFixedFeedShortcut(rt *shortcut.RuntimeContext, top bool) error {
	ids := conversationSetTopIDs(rt)
	if rt.DryRun() {
		actions := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			actions = append(actions, map[string]any{"tool": "im/set_top_conversation", "arguments": map[string]any{"openConversationId": id, "top": top}})
		}
		return rt.Output(map[string]any{"total": len(ids), "executed": false, "actions": actions})
	}
	succeeded := make([]any, 0, len(ids))
	unknown := make([]output.PartialUnknownEntry, 0)
	for _, id := range ids {
		data, err := rt.CallMCPWriteData("im", "set_top_conversation", map[string]any{"openConversationId": id, "top": top})
		if err != nil {
			unknown = append(unknown, output.PartialUnknownEntry{ID: id, Reason: err.Error()})
			continue
		}
		if replyResponseValue(data, "success") != true {
			unknown = append(unknown, output.PartialUnknownEntry{ID: id, Reason: "下游未返回明确成功证据，请查询置顶列表核实"})
			continue
		}
		succeeded = append(succeeded, map[string]any{"id": id, "result": data})
	}
	if len(unknown) == 0 {
		return rt.Output(map[string]any{"total": len(ids), "succeeded": succeeded, "failed": []any{}, "unknown": []any{}})
	}
	if len(succeeded) > 0 {
		partial, err := newFeedPartialData(len(ids), succeeded, nil, unknown)
		if err != nil {
			return err
		}
		return output.StoreResult(rt.Command().Context(), output.Partial(partial))
	}
	started := true
	return output.StoreResult(rt.Command().Context(), output.Failure(&output.ErrorInfo{Type: "api", Subtype: "feed_write_unconfirmed", Message: "所有会话置顶动作均未得到成功确认，请先查询实际状态", ExecutionStarted: &started, Details: map[string]any{"total": len(ids), "unknown": unknown}, Hint: "使用 dws chat +feed-shortcut-list 核实状态后，仅重试需要的目标"}))
}
