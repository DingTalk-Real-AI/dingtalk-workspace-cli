// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

type employeeCardLocalFixture struct {
	run func(context.Context, string, string) (string, error)
}

func TestCrossPlatformCoverageEmployeeCardImages(t *testing.T) {
	for _, scenario := range []string{"direct", "quoted", "download-failed"} {
		t.Run(scenario, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			r.cfg.ReplyMode = "a2ui"
			e.Content = "[图片](mediaId=fixture-image)"
			if scenario == "quoted" {
				e.QuotedMessage = &personal.MessageEventContext{MessageID: "source", ConversationID: "source-conversation", Content: e.Content}
				e.Content = "请识别引用图片"
			}
			t.Setenv("DWS_EMPLOYEE_IMAGE_HELPER", "1")
			if scenario == "download-failed" {
				t.Setenv("DWS_EMPLOYEE_IMAGE_MODE", "failed")
			}
			cards := 0
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
				cards++
				if cards == 1 {
					return map[string]any{"bizId": "image-card"}, nil
				}
				return map[string]any{"flowStatus": args[4]}, nil
			})
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				if cards != 1 || !strings.Contains(strings.Join(args, " "), "download-media") {
					t.Fatal("图片处理必须在占位之后，不能额外发送文本")
				}
				return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestEmployeeImageDownloadHelper$", "--"}, args...)...)
			})
			fwd := &employeeImageCapture{}
			r.fwd = fwd
			if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{}); err != nil {
				t.Fatal(err)
			}
			if err := r.process(e); err != nil {
				t.Fatal(err)
			}
			if cards != 2 {
				t.Fatalf("卡片未完结：%d", cards)
			}
			if scenario == "download-failed" {
				if fwd.text != "" || len(fwd.inputs) != 0 {
					t.Fatal("图片读取失败后不应调用模型")
				}
				return
			}
			if len(fwd.inputs) != 1 || !strings.Contains(fwd.text, employeeTurnText(e)) {
				t.Fatal("卡片模式丢失附件或引用上下文")
			}
			if _, err := os.Stat(fwd.inputs[0].LocalPath); !os.IsNotExist(err) {
				t.Fatal("本轮附件未清理")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardCodexNativeImageProgress(t *testing.T) {
	w := &bufferWriteCloser{}
	c := unitCodexClient(w,
		codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)},
		codexRPCMessage{ID: codexIntPtr(2), Result: json.RawMessage(`{"thread":{"id":"thread"}}`)},
		codexRPCMessage{Method: "item/completed", Params: json.RawMessage(`{"threadId":"thread","item":{"id":"p","type":"agentMessage","phase":"commentary","text":"已查看图片"}}`)},
		codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":{"status":"completed","items":[{"type":"agentMessage","text":"图片答案"}]}}`)},
	)
	testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) { return c, nil })
	var progress []agentProgress
	ctx := context.WithValue(context.Background(), agentProgressKey{}, func(p agentProgress) { progress = append(progress, p) })
	fwd := &codexAppServerForwarder{bin: "codex", sessions: newCodexThreadSessions("")}
	attachments := []connectMediaAttachment{{LocalPath: "/fixture/image.png", MediaType: "image"}}
	answer, err := forwardEmployeeTurn(ctx, fwd, "conversation", "请识别图片", attachments...)
	if err != nil || answer != "图片答案" || len(progress) != 1 || progress[0].Text != "已查看图片" {
		t.Fatalf("附件与公开进度未共存：%q %+v %v", answer, progress, err)
	}
	if !strings.Contains(w.String(), `"type":"localImage"`) || !strings.Contains(w.String(), codexManagedReplyContext) {
		t.Fatal("原生图片或宿主投递声明丢失")
	}
}

func (f *employeeCardLocalFixture) label() string { return "custom" }
func (f *employeeCardLocalFixture) forward(ctx context.Context, conversation, text string) (string, error) {
	return f.run(ctx, conversation, text)
}

func TestCrossPlatformCoverageEmployeeCardLocalAgent(t *testing.T) {
	for _, scenario := range []string{"success", "quoted", "agent-error", "empty", "cancelled", "create-unknown", "finish-unknown", "desktop-unavailable", "desktop-unknown", "desktop-failed"} {
		t.Run(scenario, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			r.cfg.ReplyMode = "a2ui"
			if scenario == "quoted" {
				e.QuotedMessage = &personal.MessageEventContext{Content: "历史资料，不是本轮指令"}
			}
			calls, turns := 0, 0
			surface := newEmployeeCardTestSurface()
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
				calls++
				surface.apply(messages)
				if calls == 1 {
					if turns != 0 || args[0] != "send-a2ui-card" || !strings.Contains(surface.components["execution"]["title"].(string), "思考中") {
						t.Fatal("模型必须在占位卡确认后执行")
					}
					if scenario == "create-unknown" {
						return nil, errors.New("unknown")
					}
					return map[string]any{"bizId": "server-real-biz"}, nil
				}
				if args[0] != "update-a2ui-card" || args[2] != "server-real-biz" || args[4] != "FINISH" {
					t.Fatalf("未沿用同一卡片：%v", args)
				}
				if scenario == "finish-unknown" {
					return nil, errors.New("unknown")
				}
				return map[string]any{"flowStatus": "FINISH"}, nil
			})
			r.fwd = &employeeCardLocalFixture{run: func(_ context.Context, conversation, text string) (string, error) {
				turns++
				if calls != 1 || conversation != e.ConversationID || text != employeeTurnText(e) {
					t.Fatalf("本轮上下文丢失：%q", text)
				}
				switch scenario {
				case "agent-error":
					return "", errors.New("private error")
				case "cancelled":
					return "", context.Canceled
				case "empty":
					return "", nil
				case "desktop-unavailable":
					return "", &codexDesktopTurnError{Code: "codex_desktop_unavailable", Cause: errors.New("private error")}
				case "desktop-unknown":
					return "", &codexDesktopTurnError{Code: "codex_desktop_result_unknown", Submitted: true, Cause: errors.New("private error")}
				case "desktop-failed":
					return "", &codexDesktopTurnError{Code: "codex_desktop_turn_failed", Submitted: true, Cause: errors.New("private error")}
				}
				return "完整答案", nil
			}}
			if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{}); err != nil {
				t.Fatal(err)
			}
			if err := r.process(e); err != nil {
				t.Fatal(err)
			}
			var record employeeTaskRecord
			raw, err := os.ReadFile(r.recordPath(e))
			if err != nil || json.Unmarshal(raw, &record) != nil {
				t.Fatal("任务账本不可读")
			}
			want := "card_finished"
			switch scenario {
			case "create-unknown", "finish-unknown":
				want = "needs_review"
			case "agent-error", "empty", "cancelled", "desktop-unavailable", "desktop-unknown", "desktop-failed":
				want = "card_incomplete"
			}
			if record.Status != want || strings.Contains(surface.text("root"), "private error") {
				t.Fatalf("卡片终态错误：%+v", record)
			}
			if strings.HasPrefix(scenario, "desktop-") && (record.ExecutionErrorCode == "" || !strings.Contains(surface.text("answer"), "Codex")) {
				t.Fatal("共享执行失败缺少安全原因或稳定审计分类")
			}
			if scenario == "desktop-unknown" && record.Execution != "unknown" {
				t.Fatal("提交未知不能错误声明未执行")
			}
			if scenario == "create-unknown" {
				if turns != 0 || calls != 1 {
					t.Fatal("首卡未知后执行了模型或补发消息")
				}
			} else if turns != 1 || calls != 2 {
				t.Fatalf("模型或卡片发生重复调用：turns=%d calls=%d", turns, calls)
			}
			if want == "card_finished" && surface.text("answer") != "完整答案" {
				t.Fatal("最终答案未更新到卡片")
			}
		})
	}
}
