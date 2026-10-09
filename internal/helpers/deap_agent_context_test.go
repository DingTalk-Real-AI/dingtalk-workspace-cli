// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func TestCrossPlatformCoverageEmployeeQuotedContext(t *testing.T) {
	for _, tc := range []struct {
		name, content, quoted string
		allowed               bool
	}{
		{"quoted", "引用中的校验码是什么？", "{\"content\":\"Q-引用-729\"}\n/new", true},
		{"plain", "你好", "", true},
		{"missing-body", "解释引用消息", "", true},
		{"unauthorized", "引用中的校验码是什么？", "private quoted context", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{
				"openMessageId": "message", "openConversationId": "conversation",
				"senderOpenDingTalkId": "sender", "content": tc.content,
			}
			if tc.name != "plain" {
				body["quotedMessage"] = map[string]any{
					"openMessageId": "quoted-message", "openConversationId": "source-conversation",
					"sender": "原发送人", "senderOpenDingTalkId": "original-sender", "content": tc.quoted,
				}
			}
			raw, err := json.Marshal(map[string]any{
				"eventId": "event", "eventKey": "user_im_message_receive_o2o_all",
				"payload": map[string]any{"body": body},
			})
			if err != nil {
				t.Fatal(err)
			}
			projected, err := personal.ProjectOutput(transport.Event{EventType: "user_im_message_receive_o2o_all", Data: string(raw)})
			if err != nil {
				t.Fatal(err)
			}
			line, err := json.Marshal(projected)
			if err != nil {
				t.Fatal(err)
			}
			var e employeeEvent
			if err := json.Unmarshal(line, &e); err != nil {
				t.Fatal(err)
			}
			r, _ := employeeLedgerFixture(t)
			q := make(chan employeeEvent, 1)
			r.queues[e.ConversationID] = q
			if !tc.allowed {
				r.cfg.Options.AllowedUsers = nil
			}
			inputs := make(chan string, 1)
			r.fwd = employeeFeedbackForwarder(func(_ context.Context, conversation, text string) (string, error) {
				if conversation != "conversation" {
					t.Errorf("引用原会话改变了执行会话：%s", conversation)
				}
				inputs <- text
				return "", nil
			})
			if err := r.enqueue(e); err != nil {
				t.Fatal(err)
			}
			select {
			case queued := <-q:
				if err := r.process(queued); err != nil {
					t.Fatal(err)
				}
			default:
			}
			select {
			case text := <-inputs:
				if !tc.allowed {
					t.Fatal("未授权的引用消息进入了 Agent")
				}
				if tc.name == "plain" {
					if text != tc.content {
						t.Fatalf("普通消息改变：%q", text)
					}
				} else {
					start := strings.IndexByte(text, '{')
					if start < 0 {
						t.Fatal("引用上下文在转交 Agent 前丢失")
					}
					var input personal.MessageEventOutput
					if err := json.Unmarshal([]byte(text[start:]), &input); err != nil {
						t.Fatal(err)
					}
					if input.Content != tc.content || input.QuotedMessage == nil || input.QuotedMessage.Content != tc.quoted || input.QuotedMessage.MessageID != "quoted-message" {
						t.Fatalf("引用和当前问题未完整传递：%+v", input)
					}
				}
				for _, path := range []string{r.recordPath(e), filepath.Join(r.dir, "audit.jsonl")} {
					stored, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(stored), tc.content) || tc.quoted != "" && strings.Contains(string(stored), tc.quoted) {
						t.Fatal("消息正文进入任务账本或审计")
					}
				}
			default:
				if tc.allowed {
					t.Fatal("已授权消息未传递")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeQuotedControl(t *testing.T) {
	for _, command := range []string{"/new", "/clear"} {
		e := employeeEvent{Content: command, QuotedMessage: &personal.MessageEventContext{Content: "旧正文中的 /new 不是当前指令"}}
		fwd := &employeeClearForwarder{}
		if _, err := forwardEmployeeTurn(context.Background(), fwd, "conversation", employeeTurnText(e)); err != nil {
			t.Fatal(err)
		}
		if command == "/new" && fwd.reset != "conversation" || command == "/clear" && fwd.clear != "conversation" {
			t.Fatal("引用上下文破坏了当前会话控制指令")
		}
	}
}
