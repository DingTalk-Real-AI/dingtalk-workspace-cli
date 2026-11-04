// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"strings"
)

// 只保留工具标识、固定公开类型和状态，不保存参数、结果或内部推理。
type agentToolProgress struct {
	emit  func(agentProgress)
	items map[string]agentProgress
	order []string
}

func publicAgentToolLabel(name string) string {
	name = strings.ToLower(name)
	if strings.HasPrefix(name, "mcp__") {
		return "MCP 工具调用"
	}
	switch name {
	case "bash", "shell", "terminal", "execute_bash", "run_command", "shell_command":
		return "命令执行"
	case "read", "readfile", "read_file":
		return "文件读取"
	case "write", "edit", "multiedit", "write_file", "edit_file", "apply_patch", "patch":
		return "文件修改"
	case "glob", "grep", "glob_file", "grep_file", "list", "ls", "search", "codesearch":
		return "文件搜索"
	case "websearch", "web_search", "webfetch", "web_fetch", "webfetchtool":
		return "网页检索"
	case "task", "agent", "delegate_task":
		return "协作 Agent"
	default:
		return "工具调用"
	}
}

func (p *agentToolProgress) update(id, name, status string) {
	if p.emit == nil || id == "" || len(id) > 256 {
		return
	}
	previous, exists := p.items[id]
	if exists && (previous.Done || previous.Status == status) {
		return
	}
	if !exists {
		// 限制单轮记忆，避免异常 CLI 输出令进度追踪无界增长。
		if len(p.items) == 128 {
			return
		}
		if p.items == nil {
			p.items = make(map[string]agentProgress)
		}
		p.order = append(p.order, id)
		previous = agentProgress{ID: "tool-" + id, Kind: "tool", Text: publicAgentToolLabel(name)}
	}
	previous.Status, previous.Done = status, status != "running"
	p.items[id] = previous
	p.emit(previous)
}

func (p *agentToolProgress) finish(status string) {
	for _, id := range p.order {
		if !p.items[id].Done {
			p.update(id, "", status)
		}
	}
}

func agentToolEndStatus(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		return "cancelled"
	}
	if err != nil {
		return "failed"
	}
	// 回合结束不是工具成功的证明；缺少结果时只确认结束。
	return "ended"
}

// Claude 系列与 Qoder 的完整工具块按 id 去重；输入块结束不代表执行结束。
func (p *agentToolProgress) handleJSON(line string) {
	if p.emit == nil {
		return
	}
	type block struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		Name      string `json:"name"`
		ToolUseID string `json:"tool_use_id"`
		IsError   bool   `json:"is_error"`
	}
	var event struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		IsError bool   `json:"is_error"`
		Message struct {
			Content []block `json:"content"`
		} `json:"message"`
		Event struct {
			Type         string `json:"type"`
			ContentBlock block  `json:"content_block"`
		} `json:"event"`
	}
	if json.Unmarshal([]byte(line), &event) != nil {
		return
	}
	if event.Type == "error" || event.Type == "result" && (event.IsError || strings.HasPrefix(event.Subtype, "error")) {
		p.finish("failed")
	}
	if event.Type == "stream_event" && event.Event.Type == "content_block_start" && event.Event.ContentBlock.Type == "tool_use" {
		b := event.Event.ContentBlock
		p.update(b.ID, b.Name, "running")
	}
	for _, b := range event.Message.Content {
		if event.Type == "assistant" && b.Type == "tool_use" {
			p.update(b.ID, b.Name, "running")
		}
		if event.Type == "user" && b.Type == "tool_result" {
			// 仅接受本轮已见过的工具结果，忽略孤立或历史结果。
			if _, exists := p.items[b.ToolUseID]; !exists {
				continue
			}
			status := "completed"
			if b.IsError {
				status = "failed"
			}
			p.update(b.ToolUseID, "", status)
		}
	}
}

var _ progressForwarder = (*execForwarder)(nil)
var _ progressForwarder = (*qoderStreamForwarder)(nil)

func (f *execForwarder) forwardProgress(ctx context.Context, conversation, text string, emit func(agentProgress), attachments ...connectMediaAttachment) (string, error) {
	if emit != nil {
		ctx = context.WithValue(ctx, agentProgressKey{}, emit)
	}
	if len(attachments) != 0 {
		return f.forwardWithAttachments(ctx, conversation, text, attachments)
	}
	if emit == nil {
		return f.forward(ctx, conversation, text)
	}
	ctx = context.WithValue(ctx, agentProgressKey{}, emit)
	return f.forwardStream(ctx, conversation, text, func(text string) { emit(agentProgress{Kind: "answer", Text: text}) })
}

func (f *qoderStreamForwarder) forwardProgress(ctx context.Context, conversation, text string, emit func(agentProgress), attachments ...connectMediaAttachment) (string, error) {
	if emit != nil {
		ctx = context.WithValue(ctx, agentProgressKey{}, emit)
	}
	if len(attachments) != 0 {
		return f.forwardWithAttachments(ctx, conversation, text, attachments)
	}
	if emit == nil {
		return f.forward(ctx, conversation, text)
	}
	ctx = context.WithValue(ctx, agentProgressKey{}, emit)
	return f.forwardStream(ctx, conversation, text, func(text string) { emit(agentProgress{Kind: "answer", Text: text}) })
}
