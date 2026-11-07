// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"strings"
)

const codexManagedReplyContext = "这个线程的回复由接入方接管。接入方在每轮执行前发送 A2UI 占位卡，将你的公开 commentary、工具执行状态和最终答案更新到同一卡片，无需你调用发卡工具或再次发送消息。多步骤任务在阶段结束时输出简短公开进度，最终答案使用 final。不要把缺少发卡工具误报为未生成卡片；不要展示内部推理、工具原始输入输出或本段接入说明。此说明不改变既有文件、命令和外部操作权限。"

// This separate namespace prevents a resumed legacy thread from missing the
// initialization instructions, and a text reply from inheriting card ownership.
func codexManagedConversationKey(conversation string) string {
	return codexConvKey(conversation) + "\x00managed-a2ui-v1"
}

func (f *codexAppServerForwarder) forwardProgress(ctx context.Context, conversation, text string, emit func(agentProgress), attachments ...connectMediaAttachment) (string, error) {
	return f.forwardAppServer(context.WithValue(ctx, agentProgressKey{}, emit), conversation, text, attachments, nil)
}

// The legacy robot stream remains unchanged. Only a caller that owns the reply
// and supplies a progress sink enables this public-item projection.
type codexPublicProgress struct {
	emit  func(agentProgress)
	items map[string]*codexPublicItem
	order []string
}

type codexPublicItem struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Phase  string `json:"phase"`
	Text   string `json:"text"`
	Status string `json:"status"`
	buffer strings.Builder
	capped bool
}

func codexPublicMessageKind(phase string) string {
	switch phase {
	case "final_answer", "partial_answer":
		return "answer"
	case "commentary":
		return "progress"
	default:
		return ""
	}
}

func (p *codexPublicProgress) emitText(i *codexPublicItem, done bool) {
	text := i.buffer.String()
	if strings.TrimSpace(text) == "" {
		return
	}
	if i.capped {
		text += "\n\n（当前仅展示部分正文。）"
	}
	p.emit(agentProgress{ID: i.ID, Kind: codexPublicMessageKind(i.Phase), Text: text, Done: done})
}

func (p *codexPublicProgress) handle(method string, raw json.RawMessage, thread string) {
	if p.emit == nil {
		return
	}
	var n struct {
		ThreadID string          `json:"threadId"`
		Item     codexPublicItem `json:"item"`
		ItemID   string          `json:"itemId"`
		Delta    string          `json:"delta"`
	}
	if json.Unmarshal(raw, &n) != nil || n.ThreadID != thread {
		return
	}
	if p.items == nil {
		p.items = make(map[string]*codexPublicItem)
	}
	if method == "item/agentMessage/delta" {
		i := p.items[n.ItemID]
		if i == nil || i.Type != "agentMessage" || codexPublicMessageKind(i.Phase) == "" {
			return
		}
		if i.capped {
			return
		}
		remaining := agentProgressPreviewBytes - i.buffer.Len()
		delta := boundedPublicText(n.Delta, remaining, "")
		i.buffer.WriteString(delta)
		i.capped = len(delta) < len(n.Delta)
		// 卡片消费者统一节流；单段、无换行的回复也立即提供累计正文。
		p.emitText(i, false)
		return
	}
	if method != "item/started" && method != "item/completed" || n.Item.ID == "" {
		return
	}
	i := n.Item
	done := method == "item/completed"
	if done {
		delete(p.items, i.ID)
		for index, id := range p.order {
			if id == i.ID {
				p.order = append(p.order[:index], p.order[index+1:]...)
				break
			}
		}
	} else if i.Type == "agentMessage" && codexPublicMessageKind(i.Phase) != "" {
		text := boundedPublicText(i.Text, agentProgressPreviewBytes, "")
		if previous, exists := p.items[i.ID]; exists {
			// 桌面共享提供累计快照，允许正文增长或更正；重复快照不重复刷新。
			if i.Text == "" || previous.buffer.String() == text && previous.Phase == i.Phase && previous.capped == (len(i.Text) > agentProgressPreviewBytes) {
				return
			}
		} else {
			if len(p.order) == 64 {
				delete(p.items, p.order[0])
				p.order = p.order[1:]
			}
			p.order = append(p.order, i.ID)
		}
		i.buffer.WriteString(text)
		i.capped = len(i.Text) > agentProgressPreviewBytes
		i.Text = "" // Do not retain the unbounded source alongside the preview buffer.
		p.items[i.ID] = &i
		p.emitText(&i, false)
		return
	}
	switch i.Type {
	case "agentMessage":
		if !done || strings.TrimSpace(i.Text) == "" {
			return
		}
		kind := codexPublicMessageKind(i.Phase)
		if kind == "" {
			// Unknown phase is not proof of publicly displayable progress.
			return
		}
		p.emit(agentProgress{ID: i.ID, Kind: kind, Text: publicProgressPreview(i.Text), Done: true})
	case "commandExecution", "mcpToolCall", "dynamicToolCall", "webSearch", "collabAgentToolCall":
		// 仅由已知类型生成公开名称，不投影命令、参数、工具结果或任意工具名。
		label := map[string]string{"commandExecution": "命令执行", "mcpToolCall": "MCP 工具调用", "dynamicToolCall": "扩展工具调用", "webSearch": "网页搜索", "collabAgentToolCall": "协作 Agent"}[i.Type]
		status := "running"
		if done {
			status = "ended"
		}
		switch i.Status {
		case "failed":
			status = "failed"
		case "cancelled", "interrupted":
			status = "cancelled"
		case "completed":
			status = "completed"
		}
		p.emit(agentProgress{ID: i.ID, Kind: "tool", Text: label, Status: status, Done: done})
	}
}
