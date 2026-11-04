// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// 在真实转发入口验证创建与恢复，而不是只检查构造参数的辅助函数。
func TestCrossPlatformCoverageEmployeePermissionInheritance(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(jsonScalar(existing), func(t *testing.T) {
			writer := &bufferWriteCloser{}
			settings := codexPermissionFixture(t)
			read := json.RawMessage(`{"thread":{"id":"thread"}}`)
			if existing {
				read = codexPermissionReadFixture(t, "thread", settings)
			}
			client := unitCodexClient(writer,
				codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)},
				codexRPCMessage{ID: codexIntPtr(2), Result: read},
				codexRPCMessage{ID: codexIntPtr(3), Result: codexPermissionResumeFixture(t, "thread", settings)},
				codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":{"status":"completed","items":[{"type":"agentMessage","text":"done"}]}}`)},
			)
			testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) { return client, nil })
			f := &codexAppServerForwarder{developerInstructions: "employee", sessions: newCodexThreadSessions(""), yolo: true}
			if existing {
				f.sessions.setThreadID("conversation", "thread")
			}
			ctx := context.WithValue(context.Background(), codexApprovalKey{}, codexApprovalHandler(func(context.Context, string, json.RawMessage) bool { return false }))
			if _, err := f.forwardAppServer(ctx, "conversation", "hello", nil, nil); err != nil {
				t.Fatal(err)
			}
			for _, request := range decodeCodexRequests(t, writer.String()) {
				params := request.Params
				if request.Method == "thread/start" {
					if params["approvalPolicy"] != "on-request" || params["approvalsReviewer"] != "auto_review" || params["sandbox"] != "workspace-write" {
						t.Errorf("新会话没有默认使用帮我批准: %v", params)
					}
				}
				if request.Method == "thread/resume" {
					if params["approvalsReviewer"] != "user" || params["cwd"] != settings.Cwd || params["approvalPolicy"] != "on-request" || params["permissions"] == nil {
						t.Fatal("恢复时未取用最新人工权限", params)
					}
				}
				if request.Method == "turn/start" {
					for _, key := range []string{"approvalPolicy", "approvalsReviewer", "sandbox", "sandboxPolicy", "permissions"} {
						if _, exists := params[key]; exists {
							t.Errorf("%s 覆盖了会话人工设置的 %s", request.Method, key)
						}
					}
				}
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeInitialPermissionSelection(t *testing.T) {
	for _, mode := range []string{"", "ask", "bypass"} {
		t.Run(mode, func(t *testing.T) {
			clearChannelEnv(t)
			cmd := newDeapConnectCommand()
			_ = cmd.Flags().Set("channel", "codex")
			if mode != "" {
				_ = cmd.Flags().Set("agent-permission-mode", mode)
			}
			opts, err := digitalEmployeeOptions(cmd)
			if err != nil || opts.CodexInitialPermissionMode != mode {
				t.Fatalf("初始权限模式丢失: %q %v", opts.CodexInitialPermissionMode, err)
			}
			f := &codexAppServerForwarder{developerInstructions: "employee", initialPermissionMode: mode, model: "selected-model"}
			params := f.threadParams("")
			wantPolicy, wantReviewer := "on-request", "auto_review"
			if mode != "" {
				wantReviewer = "user"
			}
			if mode == "bypass" {
				wantPolicy = "never"
			}
			if params["approvalPolicy"] != wantPolicy || params["approvalsReviewer"] != wantReviewer || params["sandbox"] != "workspace-write" || params["model"] != "selected-model" {
				t.Fatal("显式选择与新会话默认值混淆")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeePermissionFlagAndEnvironmentPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		flags, env map[string]string
		want       string
	}{
		{"environment", nil, map[string]string{"DWS_AGENT_PERMISSION_MODE": "ask"}, "ask"},
		{"approval-flag", map[string]string{"agent-approval-mode": "ask"}, nil, "ask"},
		{"approval-environment", nil, map[string]string{"DWS_AGENT_APPROVAL_MODE": "yolo"}, "bypass"},
		{"explicit-yolo", map[string]string{"yolo": "true"}, nil, "bypass"},
		{"flag-wins", map[string]string{"agent-permission-mode": "ask", "agent-approval-mode": "yolo"}, map[string]string{"DWS_AGENT_PERMISSION_MODE": "bypass"}, "ask"},
		{"permission-env-wins", map[string]string{"agent-approval-mode": "ask"}, map[string]string{"DWS_AGENT_PERMISSION_MODE": "bypass"}, "bypass"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearChannelEnv(t)
			for _, key := range []string{"DWS_AGENT_PERMISSION_MODE", "DWS_AGENT_APPROVAL_MODE"} {
				t.Setenv(key, tc.env[key])
			}
			cmd := newDeapConnectCommand()
			_ = cmd.Flags().Set("channel", "codex")
			for key, value := range tc.flags {
				_ = cmd.Flags().Set(key, value)
			}
			opts, err := digitalEmployeeOptions(cmd)
			if err != nil || opts.CodexInitialPermissionMode != tc.want {
				t.Fatalf("初始权限优先级错误: %q %v", opts.CodexInitialPermissionMode, err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeWritableRootsInvalidatePendingApproval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := newCodexDesktopApprovals(ctx)
	defer a.close()
	snapshot := codexDesktopSnapshot{LatestThreadSettings: json.RawMessage(`{"approvalPolicy":"on-request","approvalsReviewer":"user","sandboxPolicy":{"type":"workspaceWrite"}}`), RuntimeWorkspaceRoots: []string{"/work/a", "/work/b"}}
	a.syncPermissions(snapshot)
	wait, stop := context.WithCancel(ctx)
	a.waits["request"] = &codexDesktopApprovalWait{cancel: stop, result: make(chan bool, 1)}
	snapshot.RuntimeWorkspaceRoots = []string{"/work/a"}
	a.syncPermissions(snapshot)
	if wait.Err() == nil || !a.waits["request"].invalidated {
		t.Fatal("缩小可写目录后，旧审批未失效")
	}
	if employeeApprovalSource(nil) != "" {
		t.Fatal("没有入站事件时不得伪造审批来源")
	}
}

func TestCrossPlatformCoverageEmployeePendingApprovalPermissionChanges(t *testing.T) {
	for _, sandbox := range []string{"readOnly", "dangerFullAccess"} {
		t.Run(sandbox, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a := newCodexDesktopApprovals(ctx)
			defer a.close()
			snapshot := codexDesktopSnapshot{CurrentPermissions: json.RawMessage(`{"approvalPolicy":"on-request","approvalsReviewer":"user","sandboxPolicy":{"type":"workspaceWrite"}}`)}
			a.syncPermissions(snapshot)
			waitCtx, stop := context.WithCancel(ctx)
			a.waits["old"] = &codexDesktopApprovalWait{cancel: stop, result: make(chan bool, 1)}
			// 与权限无关的设置不应取消卡片。
			snapshot.CurrentPermissions = json.RawMessage(`{"approvalPolicy":"on-request","approvalsReviewer":"user","sandboxPolicy":{"type":"workspaceWrite"},"model":"other-model"}`)
			a.syncPermissions(snapshot)
			if waitCtx.Err() != nil {
				t.Fatal("模型变动误使审批失效")
			}
			snapshot.CurrentPermissions = json.RawMessage(`{"approvalPolicy":"on-request","approvalsReviewer":"user","sandboxPolicy":{"type":"` + sandbox + `"}}`)
			a.syncPermissions(snapshot)
			if waitCtx.Err() == nil || !a.waits["old"].invalidated {
				t.Fatal("放宽或收紧权限后旧审批仍可放行")
			}
		})
	}
	// 不同钉钉会话的审批来源分别保存在各自的 context，不能串到另一同事。
	r := &employeeRuntime{ctx: context.Background(), cfg: digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{Channel: "codex"}}}
	for _, sender := range []string{"alice", "bob"} {
		ctx := r.turnContext(employeeEvent{SenderID: sender, SenderName: "同事\n姓名", ConversationID: sender + "-conversation", MessageID: sender + "-message"})
		origin := ctx.Value(employeeApprovalOriginKey{}).(*employeeApprovalOrigin)
		if origin.SenderID != sender || !strings.Contains(employeeApprovalSource(origin), sender+"-conversation") || strings.Contains(employeeApprovalSource(origin), "同事\n姓名") {
			t.Fatal("审批来源发生串会话或换行伪装")
		}
	}
}

func TestCrossPlatformCoverageEmployeeOldApprovalCannotGrantNewPermissions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, codexApprovalKey{}, codexApprovalHandler(func(context.Context, string, json.RawMessage) bool { return true }))
	a := newCodexDesktopApprovals(ctx)
	defer a.close()
	a.syncPermissions(codexDesktopSnapshot{CurrentPermissions: json.RawMessage(`{"approvalPolicy":"on-request","sandboxPolicy":{"type":"workspaceWrite"}}`)})
	r := codexDesktopApproval{ID: json.RawMessage(`"old"`), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"threadId":"thread","turnId":"turn","itemId":"item","command":"echo harmless"}`)}
	wait, stop := context.WithCancel(ctx)
	w := &codexDesktopApprovalWait{request: r, cancel: stop, result: make(chan bool, 1)}
	w.result <- true // 模拟权限变化前已到达的批准点击。
	a.waits[`"old"`] = w
	a.syncPermissions(codexDesktopSnapshot{CurrentPermissions: json.RawMessage(`{"approvalPolicy":"on-request","sandboxPolicy":{"type":"readOnly"}}`)})
	if wait.Err() == nil {
		t.Fatal("旧卡仍有效")
	}
	x, y := net.Pipe()
	defer x.Close()
	defer y.Close()
	_ = x.SetDeadline(time.Now().Add(time.Second))
	_ = y.SetDeadline(time.Now().Add(time.Second))
	c := &codexDesktopClient{conn: x, client: "follower", owner: "owner", thread: "thread", frames: make(chan codexDesktopFrame, 4)}
	go c.read(ctx)
	response := make(chan map[string]any, 1)
	go func() {
		var h [4]byte
		if _, err := io.ReadFull(y, h[:]); err != nil {
			return
		}
		b := make([]byte, binary.LittleEndian.Uint32(h[:]))
		if _, err := io.ReadFull(y, b); err != nil {
			return
		}
		var m codexDesktopMessage
		_ = json.Unmarshal(b, &m)
		response <- m.Params.(map[string]any)
		server := &codexDesktopClient{conn: y}
		_ = server.send(codexDesktopMessage{Type: "response", RequestID: m.RequestID, Method: m.Method, HandledByClientID: "owner", ResultType: "success", Result: json.RawMessage(`{"ok":true}`)})
	}()
	if err := a.update(ctx, c, []codexDesktopApproval{r}, "turn"); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-response:
		if p["decision"] != "decline" {
			t.Fatal("旧点击越过了新权限")
		}
	case <-ctx.Done():
		t.Fatal("未拒绝旧请求")
	}
}

type employeePermissionRPC struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func decodeCodexRequests(t *testing.T, data string) []employeePermissionRPC {
	t.Helper()
	var requests []employeePermissionRPC
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		var request employeePermissionRPC
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
	}
	return requests
}
