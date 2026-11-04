// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageCodexSingleParagraphStreaming(t *testing.T) {
	for _, phase := range []string{"final_answer", "partial_answer", "commentary"} {
		t.Run(phase, func(t *testing.T) {
			var got []agentProgress
			p := codexPublicProgress{emit: func(v agentProgress) { got = append(got, v) }}
			start, _ := json.Marshal(map[string]any{"threadId": "t", "item": map[string]string{"id": "a", "type": "agentMessage", "phase": phase}})
			p.handle("item/started", start, "t")
			p.handle("item/agentMessage/delta", json.RawMessage(`{"threadId":"t","itemId":"a","delta":"先显示"}`), "t")
			p.handle("item/agentMessage/delta", json.RawMessage(`{"threadId":"t","itemId":"a","delta":"这一句"}`), "t")
			if len(got) != 2 || got[0].Text != "先显示" || got[1].Text != "先显示这一句" || got[1].Done {
				t.Fatalf("单段正文必须在完成前逐步显示：%+v", got)
			}
		})
	}
}

// 在真实卡片消费者处等待中间帧，防止只有最终快照的测试误报流式通过。
func TestCrossPlatformCoverageEmployeeCodexStreamingFrames(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	r.ctx = ctx
	surface := newEmployeeCardTestSurface()
	frames := make(chan string, 8)
	var history []string
	testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
		surface.apply(messages)
		for _, message := range messages {
			b, _ := json.Marshal(message)
			history = append(history, string(b))
		}
		if args[0] == "send-a2ui-card" {
			return map[string]any{"bizId": "one-card"}, nil
		}
		if args[2] != "one-card" {
			t.Error("流式更新另建了卡片")
		}
		if args[4] == "INPUTTING" {
			frames <- surface.text("answer")
		}
		return map[string]any{"flowStatus": args[4]}, nil
	})
	f := &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
		p := codexPublicProgress{emit: emit}
		p.handle("item/started", json.RawMessage(`{"threadId":"t","item":{"id":"a","type":"agentMessage","phase":"final_answer"}}`), "t")
		for _, delta := range []string{"流式第一句", "，接着第二句"} {
			b, _ := json.Marshal(map[string]string{"threadId": "t", "itemId": "a", "delta": delta})
			p.handle("item/agentMessage/delta", b, "t")
			select {
			case actual := <-frames:
				if !strings.HasSuffix(actual, delta) {
					t.Errorf("中间正文没有更新：%q", actual)
				}
			case <-ctx.Done():
				t.Error("回合完成前没有收到流式卡片")
				return "", ctx.Err()
			}
		}
		p.handle("item/completed", json.RawMessage(`{"threadId":"t","item":{"id":"tool","type":"webSearch","status":"completed"}}`), "t")
		return "流式第一句，接着第二句。完整答案", nil
	}}
	record := employeeTaskRecord{}
	if err := r.processCard(e, &record, f); err != nil || record.Status != "card_finished" || surface.text("answer") != "流式第一句，接着第二句。完整答案" {
		t.Fatalf("终态未正常收尾：%+v %v", record, err)
	}
	all := "[" + strings.Join(history, ",") + "]"
	if !strings.Contains(all, "网页搜索 · 已完成") || !strings.Contains(all, `"defaultExpanded":true`) || !strings.Contains(all, `"defaultExpanded":false`) {
		t.Fatal("展开内容或客户端展开状态错误")
	}
	protocol, err := aicardProtocol()
	if err != nil {
		t.Fatal(err)
	}
	if report, err := protocol.Lint([]byte(all), false, false); err != nil || !report.Valid {
		t.Fatalf("流式批次不符合当前卡片协议：%+v %v", report, err)
	}
}

func TestCrossPlatformCoverageCodexToolStates(t *testing.T) {
	for _, status := range []string{"inProgress", "completed", "failed", "cancelled", "interrupted", "unknown"} {
		for _, kind := range []string{"commandExecution", "mcpToolCall", "dynamicToolCall", "webSearch", "collabAgentToolCall"} {
			view := employeeCardView{}
			p := codexPublicProgress{emit: view.accept}
			b, _ := json.Marshal(map[string]any{"threadId": "t", "item": map[string]string{"id": "tool", "type": kind, "status": status}})
			p.handle("item/started", b, "t")
			p.handle("item/completed", b, "t")
			b, _ = json.Marshal(view.messages("s", "done", false))
			want := map[string]string{"inProgress": "已结束", "completed": "已完成", "failed": "执行失败", "cancelled": "已取消", "interrupted": "已取消", "unknown": "已结束"}[status]
			if !strings.Contains(string(b), want) || len(view.items) != 1 {
				t.Fatalf("工具生命周期没有按同一 ID 更新：%s", b)
			}
		}
	}
}

func TestCrossPlatformCoverageCodexSnapshotStreaming(t *testing.T) {
	var got []agentProgress
	p := codexPublicProgress{emit: func(v agentProgress) { got = append(got, v) }}
	for _, text := range []string{"共享", "共享", "共享会话正文", "共享会话更正"} {
		b, _ := json.Marshal(map[string]any{"threadId": "t", "item": map[string]string{"id": "a", "type": "agentMessage", "phase": "final_answer", "text": text}})
		p.handle("item/started", b, "t")
	}
	if len(got) != 3 || got[0].Text != "共享" || got[1].Text != "共享会话正文" || got[2].Text != "共享会话更正" {
		t.Fatalf("共享快照必须显示新增和更正的正文，并去重：%+v", got)
	}
}

func TestCrossPlatformCoverageCodexToolCardDetails(t *testing.T) {
	view := employeeCardView{}
	p := codexPublicProgress{emit: view.accept}
	p.handle("item/started", json.RawMessage(`{"threadId":"t","item":{"id":"tool","type":"commandExecution","status":"inProgress","command":"PRIVATE_COMMAND","aggregatedOutput":"PRIVATE_OUTPUT"}}`), "t")
	p.handle("item/completed", json.RawMessage(`{"threadId":"t","item":{"id":"tool","type":"commandExecution","status":"failed","command":"PRIVATE_COMMAND","aggregatedOutput":"PRIVATE_OUTPUT"}}`), "t")
	b, _ := json.Marshal(view.messages("surface", "处理完成", false))
	if !strings.Contains(string(b), "命令执行") || !strings.Contains(string(b), "执行失败") || strings.Contains(string(b), "PRIVATE") {
		t.Fatalf("展开区必须保留公开工具类型及失败状态：%s", b)
	}
}
