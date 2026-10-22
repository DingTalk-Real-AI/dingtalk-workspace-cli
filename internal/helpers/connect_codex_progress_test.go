// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageCodexPublicProgressIsolation(t *testing.T) {
	var got []agentProgress
	p := codexPublicProgress{emit: func(v agentProgress) { got = append(got, v) }}
	send := func(method, value string) { p.handle(method, json.RawMessage(value), "thread") }
	send("item/completed", `{"threadId":"other","item":{"id":"x","type":"agentMessage","phase":"commentary","text":"wrong thread"}}`)
	send("item/completed", `{"threadId":"thread","item":{"id":"r","type":"reasoning","text":"private reasoning"}}`)
	send("item/reasoning/textDelta", `{"threadId":"thread","delta":"private reasoning"}`)
	send("item/completed", `{`)
	send("item/agentMessage/delta", `{"threadId":"thread","itemId":"unknown","delta":"private"}`)
	send("item/completed", `{"threadId":"thread","item":{"id":"unknown-phase","type":"agentMessage","text":"private"}}`)
	send("item/completed", `{"threadId":"thread","item":{"id":"p","type":"agentMessage","phase":"commentary","text":"公开进度"}}`)
	send("item/started", `{"threadId":"thread","item":{"id":"t","type":"mcpToolCall","arguments":{"token":"secret"},"tool":"private-tool"}}`)
	send("item/completed", `{"threadId":"thread","item":{"id":"t","type":"mcpToolCall","result":{"secret":"private-result"}}}`)
	send("item/started", `{"threadId":"thread","item":{"id":"a","type":"agentMessage","phase":"final_answer","text":""}}`)
	send("item/agentMessage/delta", `{"threadId":"thread","itemId":"a","delta":"完整段落\n\n半"}`)
	send("item/agentMessage/delta", `{"threadId":"thread","itemId":"a","delta":"句话"}`)
	send("item/completed", `{"threadId":"thread","item":{"id":"a","type":"agentMessage","phase":"final_answer","text":"完整段落\n\n完整答案"}}`)
	if len(got) != 5 || got[0].Kind != "progress" || got[1].Kind != "tool" || !got[2].Done || got[3].Text != "完整段落" || got[4].Text != "完整段落\n\n完整答案" {
		t.Fatalf("unexpected public events: %+v", got)
	}
	raw, _ := json.Marshal(got)
	for _, forbidden := range []string{"secret", "private", "半句话", "wrong thread"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("private/incomplete content leaked")
		}
	}
}

func TestCrossPlatformCoverageCodexProgressRuntimeIntegration(t *testing.T) {
	for _, resumed := range []bool{false, true} {
		t.Run(fmt.Sprint("resumed=", resumed), func(t *testing.T) {
			var emitted []agentProgress
			w := &bufferWriteCloser{}
			c := unitCodexClient(w,
				codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)},
				codexRPCMessage{ID: codexIntPtr(2), Result: json.RawMessage(`{"thread":{"id":"thread"}}`)},
				codexRPCMessage{Method: "item/completed", Params: json.RawMessage(`{"threadId":"thread","item":{"id":"p","type":"agentMessage","phase":"commentary","text":"公开进度"}}`)},
				codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":{"status":"completed","items":[{"type":"agentMessage","text":"最终答案"}]}}`)},
			)
			testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) { return c, nil })
			f := &codexAppServerForwarder{bin: "codex", sessions: newCodexThreadSessions("")}
			if resumed {
				f.sessions.setThreadID(codexManagedConversationKey("conversation"), "thread")
			}
			answer, err := f.forwardProgress(context.Background(), "conversation", "question", func(p agentProgress) { emitted = append(emitted, p) })
			if err != nil || answer != "最终答案" || len(emitted) != 1 {
				t.Fatalf("reply=%s progress=%v err=%v", answer, emitted, err)
			}
			if !strings.Contains(w.String(), "不要主动读写文件或执行命令") && !resumed {
				t.Fatal("missing delivery ownership or expanded model permissions")
			}
			if strings.Contains(f.threadParams("")["developerInstructions"].(string), "由接入方接管") {
				t.Fatal("card instruction leaked to legacy robot")
			}
			for _, line := range strings.Split(strings.TrimSpace(w.String()), "\n") {
				var request struct {
					Method string `json:"method"`
					Params struct {
						Input                 []map[string]string `json:"input"`
						DeveloperInstructions string              `json:"developerInstructions"`
					} `json:"params"`
				}
				if err := json.Unmarshal([]byte(line), &request); err != nil {
					t.Fatal(err)
				}
				if request.Method == "turn/start" && (len(request.Params.Input) != 1 || request.Params.Input[0]["text"] != "question") {
					t.Fatalf("turn added repeated delivery context or changed user input: %s", line)
				}
				if request.Method == "thread/start" && !strings.Contains(request.Params.DeveloperInstructions, codexManagedReplyContext) {
					t.Fatal("new managed thread lacks delivery instructions")
				}
				if request.Method == "thread/resume" && request.Params.DeveloperInstructions != "" {
					t.Fatal("resumed thread repeated persistent instructions")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageCodexProgressInterruptedIsNotSuccess(t *testing.T) {
	ctx := context.WithValue(context.Background(), agentProgressKey{}, func(agentProgress) {})
	c := unitCodexClient(&bufferWriteCloser{}, codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":{"status":"interrupted","items":[{"type":"agentMessage","text":"partial"}]}}`)})
	if _, err := c.runTurn(ctx, "thread", "question", nil, nil); err == nil {
		t.Fatal("interrupted turn was reported as success")
	}
}

func TestCrossPlatformCoverageCodexProgressRequiresCompletedConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name string
		turn string
	}{
		{"missing_status", `{"items":[{"type":"agentMessage","text":"已有最终正文"}]}`},
		{"in_progress", `{"status":"inProgress","items":[{"type":"agentMessage","text":"已有最终正文"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), agentProgressKey{}, func(agentProgress) {})
			c := unitCodexClient(&bufferWriteCloser{},
				codexRPCMessage{Method: "item/agentMessage/delta", Params: json.RawMessage(`{"threadId":"thread","delta":"已有流式正文"}`)},
				codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":` + tc.turn + `}`)},
			)
			var delta string
			answer, err := c.runTurn(ctx, "thread", "question", nil, func(text string) { delta = text })
			if delta != "已有流式正文" {
				t.Fatalf("流式正文未进入真实消息处理路径：%q", delta)
			}
			if err == nil || err.Error() != "agent turn did not complete" || answer != "" {
				t.Fatalf("缺少 completed 确认时不得将已有正文作为成功结果：answer=%q err=%v", answer, err)
			}
		})
	}
}

func TestCrossPlatformCoverageCodexManagedThreadResetAndCancelledFrames(t *testing.T) {
	f := &codexAppServerForwarder{sessions: newCodexThreadSessions("")}
	f.sessions.setThreadID("conv", "legacy")
	f.sessions.setThreadID(codexManagedConversationKey("conv"), "managed")
	f.resetSession("conv")
	if f.sessions.threadID("conv") != "" || f.sessions.threadID(codexManagedConversationKey("conv")) != "" {
		t.Fatal("reset left managed history")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := unitCodexClient(&bufferWriteCloser{}, codexRPCMessage{Method: "turn/completed"})
	if _, err := c.next(ctx); err != context.Canceled {
		t.Fatal("buffered frame ignored turn cancellation")
	}
}

func TestCrossPlatformCoverageCodexProgressBoundedBuffers(t *testing.T) {
	var got []agentProgress
	p := codexPublicProgress{emit: func(v agentProgress) { got = append(got, v) }}
	for i := 0; i < 4096; i++ {
		raw := json.RawMessage(fmt.Sprintf(`{"threadId":"thread","item":{"id":"t%d","type":"mcpToolCall"}}`, i))
		p.handle("item/started", raw, "thread")
		p.handle("item/completed", raw, "thread")
	}
	if len(p.items) != 0 || len(p.order) != 0 {
		t.Fatal("completed tools retained")
	}
	got = nil
	for i := 0; i < 65; i++ {
		p.handle("item/started", json.RawMessage(fmt.Sprintf(`{"threadId":"thread","item":{"id":"a%d","type":"agentMessage","phase":"final_answer"}}`, i)), "thread")
	}
	if len(p.items) != 64 || len(p.order) != 64 || p.items["a0"] != nil {
		t.Fatal("active public items unbounded")
	}
	for _, delta := range []string{"first\n", "\nsecond", "\n\n"} {
		raw, _ := json.Marshal(map[string]any{"threadId": "thread", "itemId": "a64", "delta": delta})
		p.handle("item/agentMessage/delta", raw, "thread")
	}
	if len(got) != 2 || got[0].Text != "first" || got[1].Text != "first\n\nsecond" {
		t.Fatalf("split paragraph delimiter lost: %+v", got)
	}
	raw, _ := json.Marshal(map[string]any{"threadId": "thread", "itemId": "a64", "delta": strings.Repeat("字", 50000)})
	p.handle("item/agentMessage/delta", raw, "thread")
	p.handle("item/agentMessage/delta", raw, "thread")
	item := p.items["a64"]
	if !item.capped || item.buffer.Len() > agentProgressPreviewBytes || item.Text != "" {
		t.Fatal("full answer retained in progress state")
	}
	completed, _ := json.Marshal(map[string]any{"threadId": "thread", "item": map[string]any{"id": "a64", "type": "agentMessage", "phase": "final_answer", "text": strings.Repeat("字", 50000)}})
	p.handle("item/completed", completed, "thread")
	if len(p.items) != 63 || len(got[len(got)-1].Text) > agentProgressPreviewBytes+100 || !strings.Contains(got[len(got)-1].Text, "当前仅展示部分正文") {
		t.Fatal("completion leaked state or silently truncated preview")
	}
}

func TestCrossPlatformCoverageCodexProgressFinalAnswerIsNotPreview(t *testing.T) {
	answer := strings.Repeat("完整结果", 20000)
	completed, _ := json.Marshal(map[string]any{"threadId": "thread", "turn": map[string]any{"status": "completed", "items": []map[string]any{{"type": "agentMessage", "text": answer}}}})
	c := unitCodexClient(&bufferWriteCloser{}, codexRPCMessage{Method: "turn/completed", Params: completed})
	ctx := context.WithValue(context.Background(), agentProgressKey{}, func(agentProgress) {})
	actual, err := c.runTurn(ctx, "thread", "question", nil, nil)
	if err != nil || actual != answer {
		t.Fatalf("final answer truncated: length=%d err=%v", len(actual), err)
	}
}
