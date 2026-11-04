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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func employeeToolProcessForwarder(t *testing.T, channel string) forwarder {
	t.Helper()
	t.Setenv("DWS_EMPLOYEE_TOOL_PROCESS", "1")
	argv := []string{os.Args[0], "-test.run=^TestEmployeeToolProgressProcess$", "--"}
	if channel == "qoder" || channel == "qoderwork" {
		testseam.Swap(t, &qoderExecCommand, func(_ string, args ...string) *exec.Cmd {
			return exec.Command(argv[0], append(argv[1:], args...)...)
		})
		f := &qoderStreamForwarder{name: channel, bin: os.Args[0], timeout: 5 * time.Second}
		t.Cleanup(func() { _ = f.close() })
		return f
	}
	return &execForwarder{name: channel, argv: argv, streamArgv: argv, parser: "cc", timeout: 5 * time.Second}
}

func TestCrossPlatformCoverageEmployeeOtherAgentToolProgress(t *testing.T) {
	for _, channel := range []string{"claudecode", "codebuddy", "workbuddy", "qoder", "qoderwork"} {
		t.Run(channel, func(t *testing.T) {
			f := employeeToolProcessForwarder(t, channel)
			var tools []agentProgress
			ctx := context.WithValue(context.Background(), agentProgressKey{}, func(p agentProgress) {
				if p.Kind == "tool" {
					tools = append(tools, p)
				}
			})
			answer, err := forwardEmployeeTurn(ctx, f, "conversation", "执行测试")
			if err != nil || answer != "测试完成" {
				t.Fatalf("最终回答错误：%q %v", answer, err)
			}
			if len(tools) != 4 {
				t.Fatalf("缺少工具开始/结果事件，或输入块结束被误当执行完成：%+v", tools)
			}
			for i, want := range []struct{ label, status string }{{"命令执行", "running"}, {"文件读取", "running"}, {"命令执行", "completed"}, {"文件读取", "failed"}} {
				if tools[i].Text != want.label || tools[i].Status != want.status || tools[i].Done != (i >= 2) {
					t.Fatalf("工具状态错误：%+v", tools)
				}
			}
			if tools[0].ID != tools[2].ID || tools[1].ID != tools[3].ID {
				t.Fatal("工具状态没有更新同一执行项")
			}
			raw, _ := json.Marshal(tools)
			if strings.Contains(string(raw), "private_") {
				t.Fatal("工具输入、输出或内部推理进入公开进度")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeToolAttachments(t *testing.T) {
	for _, channel := range []string{"claudecode", "codebuddy", "workbuddy", "qoder", "qoderwork"} {
		t.Run(channel, func(t *testing.T) {
			f := employeeToolProcessForwarder(t, channel)
			// 使用原生路径并包含空格，避免把 Unix 分隔符误当作权限契约。
			attachmentDir := filepath.Join(t.TempDir(), "attachment files")
			attachmentPath := filepath.Join(attachmentDir, "image.png")
			calls := 0
			testseam.Swap(t, &connectStreamCommandContext, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				calls++
				values := func(flag string) []string {
					var found []string
					for i := 0; i+1 < len(args); i++ {
						if args[i] == flag {
							found = append(found, args[i+1])
						}
					}
					return found
				}
				if channel == "qoder" || channel == "qoderwork" {
					if !slices.Equal(values("--attachment"), []string{attachmentPath}) || !slices.Equal(values("--output-format"), []string{"stream-json"}) || !slices.Equal(values("--tools"), []string{""}) {
						t.Error("原生附件或原权限参数丢失")
					}
				} else if !slices.Equal(values("--allowedTools"), []string{"Read"}) || !slices.Equal(values("--add-dir"), []string{attachmentDir}) {
					t.Error("附件目录的精确只读权限丢失")
				}
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeToolProgressProcess$", "--")
			})
			var tools []agentProgress
			ctx := context.WithValue(context.Background(), agentProgressKey{}, func(p agentProgress) {
				if p.Kind == "tool" {
					tools = append(tools, p)
				}
			})
			answer, err := forwardEmployeeTurn(ctx, f, "conversation", "读取图片", connectMediaAttachment{LocalPath: attachmentPath})
			if err != nil || answer != "测试完成" || calls != 1 || len(tools) != 4 {
				t.Fatalf("附件与工具状态未共存：%q %v calls=%d tools=%+v", answer, err, calls, tools)
			}
			// Integrations may invoke the progress interface directly instead of
			// forwardEmployeeTurn's native attachment path. Preserve both the
			// attachment permissions above and tool events on that entry point.
			calls, tools = 0, nil
			answer, err = f.(progressForwarder).forwardProgress(context.Background(), "conversation", "读取图片", func(p agentProgress) {
				if p.Kind == "tool" {
					tools = append(tools, p)
				}
			}, connectMediaAttachment{LocalPath: attachmentPath})
			if err != nil || answer != "测试完成" || calls != 1 || len(tools) != 4 {
				t.Fatalf("直接进度接口丢失附件或工具状态：%q %v calls=%d tools=%+v", answer, err, calls, tools)
			}
		})
	}
}

func TestCrossPlatformCoverageAgentToolPrivacyAndLifecycle(t *testing.T) {
	for name, label := range map[string]string{"mcp__private_server": "MCP 工具调用", "Bash": "命令执行", "Read": "文件读取", "Edit": "文件修改", "Grep": "文件搜索", "WebFetch": "网页检索", "Task": "协作 Agent", "private_tool_name": "工具调用"} {
		if got := publicAgentToolLabel(name); got != label {
			t.Fatalf("公开类型错误：%q", got)
		}
	}
	var events []agentProgress
	p := agentToolProgress{emit: func(p agentProgress) { events = append(events, p) }}
	p.handleJSON(`not-json`)
	p.handleJSON(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"historical","is_error":false}]}}`)
	p.update("", "Bash", "running")
	p.update(strings.Repeat("x", 257), "Bash", "running")
	if len(events) != 0 {
		t.Fatal("无效或历史工具进入了卡片")
	}
	for i := 0; i < 130; i++ {
		p.update(fmt.Sprint(i), "private_name", "running")
	}
	if len(p.items) != 128 || len(events) != 128 {
		t.Fatal("工具事件记忆没有限制容量")
	}
	p.handleJSON(`{"type":"result","subtype":"error_during_execution"}`)
	p.finish("ended")
	if len(events) != 256 || events[255].Status != "failed" {
		t.Fatal("失败回合未正确收尾，或重复完成已结束工具")
	}
	var disabled agentToolProgress
	disabled.handleJSON(`{"type":"assistant"}`)
	disabled.update("id", "Bash", "running")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if agentToolEndStatus(ctx, nil) != "cancelled" || agentToolEndStatus(context.Background(), fmt.Errorf("failure")) != "failed" || agentToolEndStatus(context.Background(), nil) != "ended" {
		t.Fatal("取消、失败和仅确认结束混淆")
	}
}

func TestCrossPlatformCoverageAgentNilProgressKeepsTextMode(t *testing.T) {
	for _, channel := range []string{"claudecode", "qoder"} {
		t.Run(channel, func(t *testing.T) {
			f := employeeToolProcessForwarder(t, channel)
			if channel == "claudecode" {
				t.Setenv("DWS_EMPLOYEE_TOOL_PLAIN", "1")
			}
			answer, err := f.(progressForwarder).forwardProgress(context.Background(), "conv", "消息", nil)
			if err != nil || answer != "测试完成" {
				t.Fatalf("无进度接收器时普通回复改变：%q %v", answer, err)
			}
		})
	}
}

func TestCrossPlatformCoverageAgentOversizedCLIProgress(t *testing.T) {
	f := employeeToolProcessForwarder(t, "claudecode").(*execForwarder)
	f.timeout = 5 * time.Second
	t.Setenv("DWS_EMPLOYEE_TOOL_OVERSIZE", "1")
	var tools []agentProgress
	ctx := context.WithValue(context.Background(), agentProgressKey{}, func(p agentProgress) {
		if p.Kind == "tool" {
			tools = append(tools, p)
		}
	})
	_, err := forwardEmployeeTurn(ctx, f, "conv", "消息")
	if err == nil || !strings.Contains(err.Error(), "token too long") || len(tools) != 2 || tools[1].Status != "failed" || !tools[1].Done {
		t.Fatalf("超大 CLI 帧中断后仍错误显示执行中/成功：errType=%T tools=%+v", err, tools)
	}
}

// 仅显式选择 Qoder 时调用真实模型；只开放 Read，读取本轮隔离目录中的验收文件。
func TestEmployeeLiveQoderToolProtocol(t *testing.T) {
	if os.Getenv("DWS_EMPLOYEE_LIVE_QODER_TOOL") != "1" {
		t.Skip("set DWS_EMPLOYEE_LIVE_QODER_TOOL=1 for the authorized read-only native tool test")
	}
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "probe.txt"), []byte("A2UI_TOOL_PROTOCOL_OK_20261020"), 0600); err != nil {
		t.Fatal(err)
	}
	bin, err := exec.LookPath("qodercli")
	if err != nil {
		t.Fatal("未找到本机 Qoder CLI")
	}
	argv := []string{bin, "--print", "--output-format", "stream-json", "--tools", "Read", "--allowed-tools", "Read", "--permission-mode", "default", "--system-prompt", "", "--setting-sources", "", "--strict-mcp-config", "-p"}
	f := &execForwarder{name: "qoder", argv: argv, streamArgv: argv, parser: "qoder", workDir: work, timeout: 90 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	var tools []agentProgress
	ctx = context.WithValue(ctx, agentProgressKey{}, func(p agentProgress) {
		if p.Kind == "tool" {
			tools = append(tools, p)
		}
	})
	answer, err := forwardEmployeeTurn(ctx, f, "isolated-read-only-tool", "请使用 Read 工具读取当前目录的 probe.txt，只回复文件内的内容。不要执行命令、写文件或访问其他文件。")
	if err != nil || !strings.Contains(answer, "A2UI_TOOL_PROTOCOL_OK_20261020") {
		t.Fatalf("真实只读工具验收失败：errType=%T answerChars=%d toolEvents=%d", err, len(answer), len(tools))
	}
	if len(tools) != 2 || tools[0].Text != "文件读取" || tools[0].Status != "running" || tools[1].Status != "completed" || tools[0].ID != tools[1].ID {
		t.Fatalf("真实工具生命周期未完整进入卡片进度：%+v", tools)
	}
	t.Logf("真实 Qoder 只读工具通过：toolEvents=%d runningThenCompleted=true", len(tools))
}

func TestCrossPlatformCoverageEmployeeOtherAgentToolCard(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.cfg.ReplyMode, r.cfg.Binding.Channel = "a2ui", "claudecode"
	r.fwd = employeeToolProcessForwarder(t, "claudecode")
	barrier := filepath.Join(t.TempDir(), "running-visible")
	t.Setenv("DWS_EMPLOYEE_TOOL_BARRIER", barrier)
	view := newEmployeeCardTestSurface()
	var frames []map[string]any
	creates, previews, finishes := 0, 0, 0
	testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
		frames = append(frames, messages...)
		view.apply(messages)
		if args[0] == "send-a2ui-card" {
			creates++
			return map[string]any{"bizId": "tool-card"}, nil
		}
		if args[2] != "tool-card" {
			t.Error("工具状态未沿用原卡片")
		}
		// 递归读取实际面板的组件树，覆盖时间线及嵌套工具分组。
		var visibleText func(string) string
		visibleText = func(id string) string {
			component := view.components[id]
			text, _ := component["text"].(string)
			children, _ := component["children"].([]string)
			for _, child := range children {
				text += visibleText(child)
			}
			return text
		}
		text := visibleText(employeeCardPanelID(args[4] != "FINISH"))
		if strings.Contains(text, "private_") {
			t.Error("工具原始信息进入展开区")
		}
		if args[4] == "INPUTTING" {
			previews++
			if previews == 1 && !strings.Contains(text, "命令执行 · 执行中") {
				t.Errorf("执行中状态没有在工具结束前显示：%q", text)
			}
			if err := os.WriteFile(barrier, []byte("confirmed"), 0600); err != nil {
				t.Error(err)
			}
		} else if args[4] == "FINISH" {
			finishes++
			if !strings.Contains(text, "命令执行 · 已完成") || !strings.Contains(text, "文件读取 · 执行失败") || view.text("answer") != "测试完成" {
				t.Errorf("工具结果或最终正文丢失：%q", text)
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
	if creates != 1 || previews < 1 || finishes != 1 {
		t.Fatalf("卡片没有形成占位、执行中、最终状态的序列：%d/%d/%d", creates, previews, finishes)
	}
	if path := os.Getenv("DWS_EMPLOYEE_TOOL_CARD_SAMPLE"); path != "" {
		raw, _ := json.Marshal(frames)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// 子进程只输出 Agent 的公开协议，不调用模型或执行工具。
func TestEmployeeToolProgressProcess(t *testing.T) {
	if os.Getenv("DWS_EMPLOYEE_TOOL_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)
	run := func() {
		fmt.Println(`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool-1","name":"Bash","input":{"command":"private_command"}}}}`)
		if os.Getenv("DWS_EMPLOYEE_TOOL_OVERSIZE") == "1" {
			fmt.Print(strings.Repeat("x", 4*1024*1024+1024))
			return
		}
		fmt.Println(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"private_args"}}}`)
		fmt.Println(`{"type":"stream_event","event":{"type":"content_block_stop","index":0}}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"private_reasoning"},{"type":"tool_use","id":"tool-1","name":"Bash","input":{"command":"private_command"}},{"type":"tool_use","id":"tool-2","name":"Read","input":{"file_path":"private_path"}}]}}`)
		if barrier := os.Getenv("DWS_EMPLOYEE_TOOL_BARRIER"); barrier != "" {
			deadline := time.Now().Add(4 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(barrier); err == nil {
					break
				}
				time.Sleep(time.Millisecond)
			}
		}
		fmt.Println(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tool-1","content":"private_output","is_error":false},{"type":"tool_result","tool_use_id":"tool-2","content":"private_error","is_error":true}]}}`)
		fmt.Println(`{"type":"result","subtype":"success","result":"测试完成"}`)
	}
	if !strings.Contains(strings.Join(os.Args, " "), "--input-format") {
		if os.Getenv("DWS_EMPLOYEE_TOOL_PLAIN") == "1" {
			fmt.Print("测试完成")
			return
		}
		run()
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request map[string]any
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			continue
		}
		switch request["type"] {
		case "control_request":
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": request["request_id"], "response": map[string]any{}}})
		case "user":
			run()
		}
	}
}
