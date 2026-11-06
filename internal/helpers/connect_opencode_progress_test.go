// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCrossPlatformCoverageOpencodePublicToolEvents(t *testing.T) {
	for _, scenario := range []string{"success", "failure", "stream-unavailable", "stream-connect-error", "stream-closes", "cancelled", "request-fails"} {
		t.Run(scenario, func(t *testing.T) {
			events := make(chan string, 16)
			running := make(chan struct{}, 1)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if user, pass, ok := r.BasicAuth(); !ok || user != "employee" || pass != "fixture-password" {
					t.Error("事件流与模型请求必须沿用同一认证")
				}
				if r.URL.Path == "/event" {
					if scenario == "stream-connect-error" {
						connection, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						_ = connection.Close()
						return
					}
					if scenario == "stream-unavailable" {
						http.NotFound(w, r)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					for {
						select {
						case event := <-events:
							fmt.Fprintf(w, "data: %s\n\n", event)
							w.(http.Flusher).Flush()
							if scenario == "stream-closes" {
								return
							}
						case <-r.Context().Done():
							return
						}
					}
				}
				if r.URL.Path != "/session/ses_current/message" || r.Method != http.MethodPost {
					t.Errorf("不应查询历史或其他会话：%s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				posts.Add(1)
				var body struct {
					MessageID string           `json:"messageID"`
					Parts     []map[string]any `json:"parts"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if !strings.HasPrefix(body.MessageID, "msg_") || len(body.Parts) != 2 || body.Parts[1]["type"] != "file" {
					t.Error("本轮标识或真实附件丢失")
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				message := func(id, parent, session string) string {
					data, _ := json.Marshal(map[string]any{"type": "message.updated", "properties": map[string]any{"info": map[string]string{"id": id, "parentID": parent, "sessionID": session, "role": "assistant"}}})
					return string(data)
				}
				part := func(message, session, status string) string {
					data, _ := json.Marshal(map[string]any{"type": "message.part.updated", "properties": map[string]any{"part": map[string]any{"id": "prt_tool", "type": "tool", "messageID": message, "sessionID": session, "tool": "bash", "state": map[string]any{"status": status, "input": map[string]string{"command": "private_input"}, "output": "private_output"}}}})
					return string(data)
				}
				if scenario != "stream-unavailable" && scenario != "stream-connect-error" {
					events <- message("old", "old-parent", "ses_current")
					events <- part("old", "ses_current", "running")
					events <- message("foreign", body.MessageID, "ses_foreign")
					events <- part("foreign", "ses_foreign", "running")
					events <- message("assistant-current", body.MessageID, "ses_current")
					events <- part("assistant-current", "ses_current", "pending")
					events <- part("assistant-current", "ses_current", "running")
					events <- part("assistant-current", "ses_current", "running")
					if scenario != "stream-closes" {
						select {
						case <-running:
						case <-r.Context().Done():
							return
						}
					}
				}
				if scenario == "cancelled" {
					cancel()
					<-r.Context().Done()
					return
				}
				if scenario == "request-fails" {
					http.Error(w, "private_error", http.StatusBadRequest)
					return
				}
				status := "completed"
				if scenario == "failure" {
					status = "error"
				}
				fmt.Fprintf(w, `{"parts":[{"id":"prt_tool","type":"tool","messageID":"assistant-current","sessionID":"ses_current","tool":"bash","state":{"status":%q,"output":"private_output"}},{"type":"text","text":"最终答案"}]}`, status)
			}))
			defer server.Close()
			var tools []agentProgress
			ctx = context.WithValue(ctx, agentProgressKey{}, func(p agentProgress) {
				if p.Kind == "tool" {
					tools = append(tools, p)
					if p.Status == "running" {
						select {
						case running <- struct{}{}:
						default:
						}
					}
				}
			})
			c := &opencodeHTTPClient{baseURL: server.URL, username: "employee", password: "fixture-password", httpClient: server.Client()}
			answer, err := c.sendMessageWithAttachments(ctx, "ses_current", "读取本轮附件", "", []connectMediaAttachment{{LocalPath: "/fixture/image.png"}})
			if scenario == "cancelled" || scenario == "request-fails" {
				if err == nil || len(tools) != 2 || tools[1].Status != map[string]string{"cancelled": "cancelled", "request-fails": "failed"}[scenario] {
					t.Fatalf("执行失败未正确收尾：%v %+v", err, tools)
				}
			} else {
				if err != nil || answer != "最终答案" || len(tools) < 1 {
					t.Fatalf("工具事件或最终答案丢失：%q %v %+v", answer, err, tools)
				}
				want := "completed"
				if scenario == "failure" {
					want = "failed"
				}
				if tools[len(tools)-1].Status != want || len(tools) > 2 {
					t.Fatalf("混入历史/其他会话事件，或重复工具项：%+v", tools)
				}
			}
			if posts.Load() != 1 {
				t.Fatalf("请求被重复提交：%d", posts.Load())
			}
			raw, _ := json.Marshal(tools)
			if strings.Contains(string(raw), "private_") {
				t.Fatal("原始工具数据进入卡片")
			}
		})
	}
}

func TestCrossPlatformCoverageOpencodeProgressPrivacyAndBounds(t *testing.T) {
	var events []agentProgress
	p := opencodePublicProgress{session: "ses_current", parent: "msg_current", tools: agentToolProgress{emit: func(p agentProgress) { events = append(events, p) }}, assistants: make(map[string]bool), text: make(map[string]string)}
	p.handle(`not-json`)
	p.handle(`{"type":"message.updated","properties":{"info":{"id":"assistant","sessionID":"ses_current","parentID":"msg_current","role":"assistant"}}}`)
	part := func(id, kind, text string) {
		raw, _ := json.Marshal(map[string]any{"type": "message.part.updated", "properties": map[string]any{"part": map[string]string{"id": id, "messageID": "assistant", "sessionID": "ses_current", "type": kind, "text": text}}})
		p.handle(string(raw))
	}
	part("reasoning", "reasoning", "private_reasoning")
	for i := 0; i < 65; i++ {
		text := "正文"
		if i == 0 {
			text = strings.Repeat("x", agentProgressPreviewBytes+20)
		}
		part(fmt.Sprint(i), "text", text)
	}
	if len(p.text) != 64 || len(events) != 64 || len(events[63].Text) > agentProgressPreviewBytes+100 {
		t.Fatal("公开正文的数量或长度没有限制")
	}
	part("0", "text", "更正")
	if !strings.HasPrefix(events[len(events)-1].Text, "更正正文") {
		t.Fatal("累计正文快照被重复拼接，或更正丢失")
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "private_") {
		t.Fatal("内部推理混入公开正文")
	}
}

func TestCrossPlatformCoverageOpencodeProgressForwarder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/global/health":
			fmt.Fprint(w, `{"healthy":true}`)
		case "/session":
			fmt.Fprint(w, `{"id":"ses_current"}`)
		case "/event":
			http.NotFound(w, r)
		case "/session/ses_current/message":
			fmt.Fprint(w, `{"parts":[{"id":"prt_tool","messageID":"assistant","type":"tool","tool":"read","state":{"status":"completed"}},{"type":"text","text":"正文"}]}`)
		}
	}))
	defer server.Close()
	f := &opencodeForwarder{timeout: time.Second, server: &opencodeServer{baseURL: server.URL, httpClient: server.Client()}}
	var events []agentProgress
	answer, err := f.forwardProgress(context.Background(), "conv", "消息", func(p agentProgress) { events = append(events, p) })
	if err != nil || answer != "正文" || len(events) != 1 || events[0].Status != "completed" {
		t.Fatalf("普通员工进度入口未传递本轮工具结果：%q %v %+v", answer, err, events)
	}
	c := &opencodeHTTPClient{baseURL: "\x00", httpClient: server.Client()}
	if err := c.messageWithProgress(context.Background(), "ses", map[string]any{}, &opencodeMessageResponse{}, func(agentProgress) {}); err == nil {
		t.Fatal("无效事件地址没有报错")
	}
}

func TestCrossPlatformCoverageOpencodeSSEBoundaries(t *testing.T) {
	input := ": keepalive\n\ndata: {\ndata: \"type\":\"message.updated\"}\n\ndata: " + strings.Repeat("x", 256*1024) + "\n\ndata: {\"type\":\"last\"}\n\n"
	events := make(chan string, 3)
	readOpencodeProgressEvents(context.Background(), strings.NewReader(input), events)
	if len(events) != 2 || !json.Valid([]byte(<-events)) || !json.Valid([]byte(<-events)) {
		t.Fatal("SSE 多行或超大事件边界处理错误")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	readOpencodeProgressEvents(ctx, io.LimitReader(strings.NewReader("data: {}\n\n"), 20), make(chan string))
}
