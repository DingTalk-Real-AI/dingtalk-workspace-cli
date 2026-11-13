// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

var _ progressForwarder = (*opencodeForwarder)(nil)

func (f *opencodeForwarder) forwardProgress(ctx context.Context, conversation, text string, emit func(agentProgress), attachments ...connectMediaAttachment) (string, error) {
	return f.forwardWithAttachments(context.WithValue(ctx, agentProgressKey{}, emit), conversation, text, attachments)
}

type opencodeProgressPart struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionID"`
	MessageID string `json:"messageID"`
	Type      string `json:"type"`
	Tool      string `json:"tool"`
	Text      string `json:"text"`
	State     struct {
		Status string `json:"status"`
	} `json:"state"`
}

type opencodePublicProgress struct {
	session, parent string
	tools           agentToolProgress
	assistants      map[string]bool
	text            map[string]string
	textOrder       []string
}

func (p *opencodePublicProgress) tool(part opencodeProgressPart) {
	if part.Type != "tool" || part.ID == "" {
		return
	}
	status := part.State.Status
	if status == "error" {
		status = "failed"
	}
	if status == "running" || status == "completed" || status == "failed" {
		p.tools.update(part.MessageID+":"+part.ID, part.Tool, status)
	}
}

func (p *opencodePublicProgress) handle(raw string) {
	var event struct {
		Type       string `json:"type"`
		Properties struct {
			Info struct {
				ID        string `json:"id"`
				SessionID string `json:"sessionID"`
				ParentID  string `json:"parentID"`
				Role      string `json:"role"`
			} `json:"info"`
			Part opencodeProgressPart `json:"part"`
		} `json:"properties"`
	}
	if json.Unmarshal([]byte(raw), &event) != nil {
		return
	}
	if event.Type == "message.updated" {
		i := event.Properties.Info
		if i.SessionID == p.session && i.ParentID == p.parent && i.Role == "assistant" && i.ID != "" && len(i.ID) <= 256 && len(p.assistants) < 64 {
			p.assistants[i.ID] = true
		}
		return
	}
	part := event.Properties.Part
	if event.Type != "message.part.updated" || part.SessionID != p.session || !p.assistants[part.MessageID] {
		return
	}
	p.tool(part)
	if part.Type != "text" || part.ID == "" || len(part.ID) > 256 {
		return
	}
	id := part.MessageID + ":" + part.ID
	if _, exists := p.text[id]; !exists {
		if len(p.textOrder) == 64 {
			return
		}
		p.textOrder = append(p.textOrder, id)
	}
	p.text[id] = publicProgressPreview(part.Text)
	var answer strings.Builder
	for _, key := range p.textOrder {
		if answer.Len() >= agentProgressPreviewBytes {
			break
		}
		answer.WriteString(p.text[key])
	}
	p.tools.emit(agentProgress{Kind: "answer", Text: publicProgressPreview(answer.String())})
}

// 订阅成功后才提交本轮请求；观察失败不重放请求，也不改变原执行权限。
func (c *opencodeHTTPClient) messageWithProgress(ctx context.Context, session string, body map[string]any, out *opencodeMessageResponse, emit func(agentProgress)) (turnErr error) {
	// 沿用原生 ID 的时间前缀，保留服务端按消息 ID 排序的语义。
	timestamp := (uint64(time.Now().UnixMilli()) << 12) & 0xffffffffffff
	parent := fmt.Sprintf("msg_%012x%s", timestamp, strings.ReplaceAll(uuid.NewString(), "-", "")[:14])
	body["messageID"] = parent
	p := opencodePublicProgress{session: session, parent: parent, tools: agentToolProgress{emit: emit}, assistants: make(map[string]bool), text: make(map[string]string)}
	defer func() {
		// 同步响应只属于本次提交，也可补齐事件流缺失的终态。
		if turnErr == nil {
			for _, part := range out.Parts {
				safe := opencodeProgressPart{ID: part.ID, MessageID: part.MessageID, Type: part.Type, Tool: part.Tool}
				safe.State.Status = part.State.Status
				p.tool(safe)
			}
		}
		p.tools.finish(agentToolEndStatus(ctx, turnErr))
	}()
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, strings.TrimRight(c.baseURL, "/")+"/event", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	// 只限制等待响应头的时间，已建立的事件流沿用本轮截止时间。
	timer := time.AfterFunc(5*time.Second, cancel)
	resp, streamErr := c.httpClient.Do(req)
	timer.Stop()
	if streamErr != nil {
		return c.doJSON(ctx, http.MethodPost, "/session/"+session+"/message", body, out)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return c.doJSON(ctx, http.MethodPost, "/session/"+session+"/message", body, out)
	}
	events := make(chan string, 16)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		readOpencodeProgressEvents(streamCtx, resp.Body, events)
	}()
	defer func() {
		cancel()
		_ = resp.Body.Close()
		<-readerDone
	}()
	result := make(chan error, 1)
	go func() { result <- c.doJSON(ctx, http.MethodPost, "/session/"+session+"/message", body, out) }()
	for {
		select {
		// 事件读取结束由 readerDone 收尾；主循环继续等待原 HTTP 请求，不重发。
		case raw := <-events:
			p.handle(raw)
		case turnErr = <-result:
			return turnErr
		}
	}
}

func readOpencodeProgressEvents(ctx context.Context, reader io.Reader, events chan<- string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	var data strings.Builder
	oversized := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if !oversized && data.Len() > 0 {
				select {
				case events <- data.String():
				case <-ctx.Done():
					return
				}
			}
			data.Reset()
			oversized = false
		} else if strings.HasPrefix(line, "data:") && !oversized {
			if data.Len()+len(line) > 256*1024 {
				oversized = true
				data.Reset()
				continue
			}
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
}
