// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// 运行真实适配器及子进程协议，验证正文在最终返回之前进入通用卡片进度接口。
func TestCrossPlatformCoverageEmployeeOtherAgentStreaming(t *testing.T) {
	for _, channel := range []string{"claudecode", "codebuddy", "workbuddy", "qoder", "qoderwork"} {
		for _, card := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/card=%t", channel, card), func(t *testing.T) {
				t.Setenv("DWS_EMPLOYEE_STREAM_PROCESS", "1")
				argv := []string{os.Args[0], "-test.run=^TestEmployeeOtherAgentStreamProcess$", "--"}
				var fwd forwarder
				if channel == "qoder" || channel == "qoderwork" {
					testseam.Swap(t, &qoderExecCommand, func(_ string, args ...string) *exec.Cmd {
						return exec.Command(argv[0], append(argv[1:], args...)...)
					})
					qoder := &qoderStreamForwarder{name: channel, bin: os.Args[0], timeout: 5 * time.Second}
					t.Cleanup(func() { _ = qoder.close() })
					fwd = qoder
				} else {
					fwd = &execForwarder{name: channel, argv: argv, streamArgv: append(append([]string{}, argv...), "--stream-fixture"), parser: "cc", timeout: 5 * time.Second}
				}
				var progress []agentProgress
				ctx := context.Background()
				returned := false
				if card {
					ctx = context.WithValue(ctx, agentProgressKey{}, func(p agentProgress) {
						if returned {
							t.Error("最终答案返回后仍然发送增量")
						}
						progress = append(progress, p)
					})
				}
				answer, err := forwardEmployeeTurn(ctx, fwd, "test-conversation", "只输出测试正文")
				returned = true
				if err != nil || answer != "最终完整答案" {
					t.Fatalf("适配器最终结果错误：%q %v", answer, err)
				}
				if !card {
					if len(progress) != 0 {
						t.Fatal("text 模式发送了卡片进度")
					}
					return
				}
				if len(progress) != 2 || progress[0].Kind != "answer" || progress[1].Kind != "answer" || !strings.HasPrefix(progress[0].Text, "第一段") || !strings.Contains(progress[1].Text, "第二段") {
					t.Fatalf("流式正文没有进入 A2UI：%+v", progress)
				}
				for _, p := range progress {
					if strings.Contains(p.Text, "private_reasoning") || strings.Contains(p.Text, "private_tool_input") {
						t.Fatal("内部推理或工具参数进入了卡片")
					}
				}
			})
		}
	}
}

// 只提供正文流式接口的渠道也必须在最终回复前展示预览。
func TestCrossPlatformCoverageEmployeeStreamingOnlyForwarder(t *testing.T) {
	fwd := &streamingConnectorForwarder{basicConnectorForwarder: &basicConnectorForwarder{reply: "最终答案"}}
	var progress []agentProgress
	ctx := context.WithValue(context.Background(), agentProgressKey{}, func(p agentProgress) { progress = append(progress, p) })
	answer, err := forwardEmployeeTurn(ctx, fwd, "conversation", "请回复")
	if err != nil || answer != "最终答案" || !fwd.streamed {
		t.Fatalf("正文流式渠道未调用流式入口：%q %v", answer, err)
	}
	if len(progress) != 1 || progress[0].Kind != "answer" || progress[0].Text != "partial" {
		t.Fatalf("正文预览没有进入卡片：%+v", progress)
	}
}

func TestEmployeeOtherAgentStreamProcess(t *testing.T) {
	if os.Getenv("DWS_EMPLOYEE_STREAM_PROCESS") != "1" {
		return
	}
	defer os.Exit(0) // 子进程只输出 CLI 协议，不附加 Go 测试的 PASS 文本。
	args := strings.Join(os.Args, " ")
	if strings.Contains(args, "--input-format") {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var request map[string]any
			if json.Unmarshal(scanner.Bytes(), &request) != nil {
				continue
			}
			if request["type"] == "control_request" {
				_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": request["request_id"], "response": map[string]any{"agents": []any{}}}})
				continue
			}
			if request["type"] == "user" {
				for _, text := range []string{"第一段", "第二段"} {
					_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "assistant", "message": map[string]any{"content": []map[string]string{{"type": "text", "text": text}}}})
				}
				fmt.Println(`{"type":"result","subtype":"success","result":"最终完整答案"}`)
			}
		}
		return
	}
	if strings.Contains(args, "--stream-fixture") {
		fmt.Println(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"第一段"}}}`)
		if barrier := os.Getenv("DWS_EMPLOYEE_STREAM_BARRIER"); barrier != "" {
			deadline := time.Now().Add(4 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(barrier); err == nil {
					break
				}
				time.Sleep(time.Millisecond)
			}
		}
		fmt.Println(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"private_reasoning"}}}`)
		fmt.Println(`{"type":"stream_event","event":{"type":"content_block_start","content_block":{"type":"tool_use","input":"private_tool_input"}}}`)
		fmt.Println(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"第二段"}}}`)
		fmt.Println(`{"type":"result","result":"最终完整答案"}`)
		return
	}
	fmt.Print("最终完整答案")
}

func TestCrossPlatformCoverageEmployeeOtherAgentIntermediateCard(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.cfg.ReplyMode = "a2ui"
	r.cfg.Binding.Channel = "claudecode"
	barrier := filepath.Join(t.TempDir(), "first-card-confirmed")
	t.Setenv("DWS_EMPLOYEE_STREAM_PROCESS", "1")
	t.Setenv("DWS_EMPLOYEE_STREAM_BARRIER", barrier)
	argv := []string{os.Args[0], "-test.run=^TestEmployeeOtherAgentStreamProcess$", "--"}
	r.fwd = &execForwarder{name: "claudecode", argv: argv, streamArgv: append(append([]string{}, argv...), "--stream-fixture"), parser: "cc", timeout: 5 * time.Second}
	view := newEmployeeCardTestSurface()
	creates, partials, finishes := 0, 0, 0
	testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
		view.apply(messages)
		if args[0] == "send-a2ui-card" {
			creates++
			return map[string]any{"bizId": "other-agent-card"}, nil
		}
		if args[2] != "other-agent-card" {
			t.Error("流式更新没有沿用原卡片")
		}
		if args[4] == "INPUTTING" {
			partials++
			if partials == 1 && view.text("answer") != "第一段" {
				t.Errorf("最终答案返回前的首段没有展示：%q", view.text("answer"))
			}
			if err := os.WriteFile(barrier, []byte("confirmed"), 0o600); err != nil {
				t.Error(err)
			}
		} else if args[4] == "FINISH" {
			finishes++
			if view.text("answer") != "最终完整答案" {
				t.Error("最终正文没有替换流式预览")
			}
		}
		return map[string]any{"flowStatus": args[4]}, nil
	})
	if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := r.process(e); err != nil {
		t.Fatal(err)
	}
	if creates != 1 || partials < 1 || finishes != 1 {
		t.Fatalf("占位、流式与终态未形成同卡序列：%d/%d/%d", creates, partials, finishes)
	}
}

// 显式选择本机 Agent 才调用真实模型；不绑定员工或发送钉钉消息。
func TestEmployeeLiveOtherAgentStreaming(t *testing.T) {
	channels := os.Getenv("DWS_EMPLOYEE_LIVE_STREAM_CHANNELS")
	if channels == "" {
		t.Skip("set DWS_EMPLOYEE_LIVE_STREAM_CHANNELS to opt in to real models")
	}
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DWS_CONNECT_NO_INSTALL", "1")
	for _, channel := range strings.Split(channels, ",") {
		t.Run(channel, func(t *testing.T) {
			cfg := digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{Channel: channel, DWSProfile: "live-stream:isolated-employee"}, Options: connectAgentOptions{WorkDir: t.TempDir(), Timeout: 90 * time.Second}}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
			defer cancel()
			fwd, err := digitalEmployeeNewForwarder(ctx, cfg)
			if err != nil {
				t.Fatalf("真实 Agent 初始化失败：%T", err)
			}
			if closer, ok := fwd.(forwarderCloser); ok {
				defer closer.close()
			}
			if err := prepareEmployeeForwarder(ctx, fwd); err != nil {
				t.Fatalf("真实 Agent 协议预检失败：%T", err)
			}
			frames := 0
			ctx = context.WithValue(ctx, agentProgressKey{}, func(p agentProgress) {
				if p.Kind == "answer" && strings.TrimSpace(p.Text) != "" {
					frames++
				}
			})
			answer, err := forwardEmployeeTurn(ctx, fwd, "isolated-stream", "这是纯文本流式验收。请不要调用任何工具，不要读写文件或执行命令。请用约三百个汉字介绍春夏秋冬，末尾写上校验数字731824。")
			if err != nil || !strings.Contains(answer, "731824") {
				// 仅记录固定分类及 HTTP 状态，不输出模型供应商的原始错误或凭据。
				backendError := strings.HasPrefix(answer, "AI 后端调用失败")
				status := "unknown"
				for _, code := range []string{"400", "401", "402", "403", "404", "422", "429", "500", "502", "503"} {
					if strings.Contains(answer, "API Error: "+code) {
						status = code
					}
				}
				t.Fatalf("真实模型回答失败：errType=%T chars=%d frames=%d backendError=%t httpStatus=%s", err, len(answer), frames, backendError, status)
			}
			if frames == 0 {
				t.Fatal("真实模型未提供公开流式正文")
			}
			t.Logf("真实模型流式验收通过：channel=%s frames=%d answerChars=%d", channel, frames, len([]rune(answer)))
		})
	}
}
