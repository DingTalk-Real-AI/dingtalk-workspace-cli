// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeCodexToolPolicy(t *testing.T) {
	clearChannelEnv(t)
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DWS_CONNECT_NO_INSTALL", "1")
	bin := t.TempDir()
	if err := writeExecStub(bin, "codex"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	for _, yolo := range []bool{false, true} {
		cfg := digitalEmployeeAdapterConfig{
			Binding: digitalEmployeeBinding{Channel: "codex", DWSProfile: "corp:employee"},
			Options: connectAgentOptions{Memory: true, WorkDir: t.TempDir(), Yolo: yolo},
		}
		fwd, err := digitalEmployeeNewForwarder(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		cf := fwd.(*codexAppServerForwarder)
		for _, thread := range []string{"", "existing-thread"} {
			params := cf.threadParams(thread)
			instructions := params["developerInstructions"].(string)
			if strings.Contains(instructions, "不得访问其它文件") || strings.Contains(instructions, "仅当用户消息明确附带了本地附件路径时") {
				t.Error("数字员工继承了机器人仅分析附件的限制，用户明确请求也无法调用本地技能")
			}
			if !strings.Contains(instructions, "技能") || !strings.Contains(instructions, "用户明确") {
				t.Error("缺少数字员工按用户授权使用本地技能的指令")
			}
			wantSandbox := "read-only"
			if yolo {
				wantSandbox = "workspace-write"
			}
			if params["sandbox"] != wantSandbox || params["approvalPolicy"] != "never" || params["cwd"] != cfg.Options.WorkDir || cf.sessions == nil {
				t.Fatal("工具策略改变了沙箱、工作目录或会话复用配置")
			}
		}
		// 每轮新建 app-server 进程并不等于新建会话；重建 forwarder 后也要恢复同一 thread。
		var captures []*bufferWriteCloser
		testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) {
			buf := &bufferWriteCloser{}
			captures = append(captures, buf)
			return unitCodexClient(buf,
				codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)},
				codexRPCMessage{ID: codexIntPtr(2), Result: json.RawMessage(`{"thread":{"id":"employee-thread"}}`)},
				codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"employee-thread","turn":{"status":"completed","items":[{"type":"agentMessage","text":"done"}]}}`)},
			), nil
		})
		cf.resetSession("conversation")
		for turn := 0; turn < 2; turn++ {
			if turn == 1 {
				fwd, err = digitalEmployeeNewForwarder(context.Background(), cfg)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := fwd.forward(context.Background(), "conversation", "请使用本地技能"); err != nil {
				t.Fatal(err)
			}
		}
		for i, capture := range captures {
			for _, line := range strings.Split(strings.TrimSpace(capture.String()), "\n") {
				var request struct {
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if err := json.Unmarshal([]byte(line), &request); err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(request.Method, "thread/") {
					want := "thread/start"
					if i == 1 {
						want = "thread/resume"
						if request.Params["threadId"] != "employee-thread" {
							t.Fatal("重建 forwarder 后会话上下文丢失")
						}
					}
					if request.Method != want || request.Params["developerInstructions"] == codexRobotDeveloperInstructions {
						t.Fatal("真实 RPC 的指令或会话恢复策略错误")
					}
				}
				if request.Method == "turn/start" {
					contexts, ok := request.Params["additionalContext"].(map[string]any)
					if !ok {
						t.Fatal("旧 thread 的历史指令不会被 resume 参数替换，缺少本轮应用策略")
					}
					policy, ok := contexts["dws.digital_employee_policy"].(map[string]any)
					if !ok || policy["kind"] != "application" || policy["value"] != codexEmployeeDeveloperInstructions {
						t.Fatal("员工策略没有通过应用上下文传给本轮模型")
					}
				}
			}
		}
	}
	robot, err := newLocalAgentForwarder("codex", "robot", connectAgentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if robot.(*codexAppServerForwarder).threadParams("")["developerInstructions"] != codexRobotDeveloperInstructions {
		t.Fatal("数字员工策略影响了机器人链路")
	}
}

func TestCrossPlatformCoverageEmployeeCustomSessionOwnership(t *testing.T) {
	clearChannelEnv(t)
	fwd, err := newLocalAgentForwarder("custom", "employee", connectAgentOptions{Command: "codex exec", Memory: true})
	if err != nil {
		t.Fatal(err)
	}
	f := fwd.(*execForwarder)
	for _, conversation := range []string{"conversation", "conversation", "other-conversation"} {
		if f.sessions != nil || !reflect.DeepEqual(f.commandArgs(f.argv, conversation, "question"), []string{"exec", "question"}) {
			t.Fatal("custom 模式被注入了未知的会话参数")
		}
	}
}

func TestCrossPlatformCoverageEmployeeCodexResumePreservesContext(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	f := newCodexAppServerForwarder("fixture", nil, 0, connectAgentOptions{Memory: true}, "employee").(*codexAppServerForwarder)
	f.developerInstructions = codexEmployeeDeveloperInstructions
	f.sessions.setThreadID("conversation", "locked-thread")
	testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) {
		return unitCodexClient(&bufferWriteCloser{},
			codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)},
			codexRPCMessage{ID: codexIntPtr(2), Error: &codexRPCError{Code: -32600, Message: "thread already has an active writer"}},
			codexRPCMessage{ID: codexIntPtr(3), Result: json.RawMessage(`{"thread":{"id":"replacement-thread"}}`)},
			codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"replacement-thread","turn":{"status":"completed","items":[{"type":"agentMessage","text":"done"}]}}`)},
		), nil
	})
	if _, err := f.forward(context.Background(), "conversation", "继续之前的任务"); !errors.Is(err, errCodexEmployeeResumeFailed) {
		t.Fatal("恢复失败后静默新建了会话，或缺少可识别的恢复失败结果", err)
	}
	if f.sessions.threadID("conversation") != "locked-thread" {
		t.Fatal("会话恢复失败清除了已有 thread 映射")
	}
	// 只有用户明确发起新会话才清除映射。
	f.resetSession("conversation")
	if f.sessions.threadID("conversation") != "" {
		t.Fatal("明确的新会话指令未生效")
	}
}
