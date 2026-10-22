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
	ID      string `json:"id"`
	Type    string `json:"type"`
	Phase   string `json:"phase"`
	Text    string `json:"text"`
	buffer  strings.Builder
	lastEnd int
	capped  bool
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
		if i == nil || i.Type != "agentMessage" || (i.Phase != "final_answer" && i.Phase != "partial_answer") {
			return
		}
		if i.capped {
			return
		}
		previous := i.buffer.Len()
		remaining := agentProgressPreviewBytes - previous
		delta := boundedPublicText(n.Delta, remaining, "")
		i.buffer.WriteString(delta)
		i.capped = len(delta) < len(n.Delta)
		text := i.buffer.String()
		// Search only newly received bytes plus the possible split delimiter.
		from := max(0, previous-1)
		if end := strings.LastIndex(text[from:], "\n\n"); end >= 0 {
			end += from
			if end > i.lastEnd {
				i.lastEnd = end
				p.emit(agentProgress{ID: i.ID, Kind: "answer", Text: text[:end]})
			}
		}
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
	} else if i.Type == "agentMessage" && (i.Phase == "final_answer" || i.Phase == "partial_answer") {
		if _, exists := p.items[i.ID]; exists {
			return
		}
		if len(p.order) == 64 {
			delete(p.items, p.order[0])
			p.order = p.order[1:]
		}
		p.order = append(p.order, i.ID)
		i.buffer.WriteString(boundedPublicText(i.Text, agentProgressPreviewBytes, ""))
		i.capped = len(i.Text) > agentProgressPreviewBytes
		i.Text = "" // Do not retain the unbounded source alongside the preview buffer.
		p.items[i.ID] = &i
	}
	switch i.Type {
	case "agentMessage":
		if !done || strings.TrimSpace(i.Text) == "" {
			return
		}
		kind := "progress"
		if i.Phase == "final_answer" || i.Phase == "partial_answer" {
			kind = "answer"
		} else if i.Phase != "commentary" {
			// Unknown phase is not proof of publicly displayable progress.
			return
		}
		p.emit(agentProgress{ID: i.ID, Kind: kind, Text: publicProgressPreview(i.Text), Done: true})
	case "commandExecution", "mcpToolCall", "dynamicToolCall", "webSearch", "collabAgentToolCall":
		p.emit(agentProgress{ID: i.ID, Kind: "tool", Done: done})
	}
}
