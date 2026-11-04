// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// 显式启用后使用真实 app-server 和无工具回合，验证跨进程恢复及人工权限变化。
func TestEmployeeCodexPermissionInheritanceLive(t *testing.T) {
	if os.Getenv("DWS_CODEX_PERMISSION_LIVE") != "1" {
		t.Skip("set DWS_CODEX_PERMISSION_LIVE=1 to run real local Codex turns")
	}
	bin, err := exec.LookPath("codex")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	// 模拟本机默认完全访问；只改本测试子进程 argv，不改用户配置。
	testseam.Swap(t, &codexExecCommandContext, func(ctx context.Context, bin string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, bin, append(args, "-c", `sandbox_mode="danger-full-access"`)...)
	})
	work := t.TempDir()
	open := func() *codexAppServerClient {
		c, err := newCodexAppServerClient(ctx, bin, nil, work)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(c.close)
		if err := c.initialize(ctx); err != nil {
			t.Fatal(err)
		}
		return c
	}
	call := func(c *codexAppServerClient, method string, params map[string]any) json.RawMessage {
		id := c.requestID()
		if err := c.send(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		raw, err := c.waitResponse(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	c := open()
	thread, err := c.startThread(ctx, map[string]any{"cwd": work, "sandbox": "workspace-write", "approvalPolicy": "on-request", "approvalsReviewer": "user"})
	if err != nil {
		t.Fatal(err)
	}
	const prompt = "这是本地权限继承测试。不要调用任何工具，不要读写文件，不要创建子任务，只回复 C9_P_LOCAL_OK。"
	if _, err := c.runTurn(ctx, thread, prompt, nil, nil); err != nil {
		t.Fatal(err)
	}
	c.close()
	f := &codexAppServerForwarder{bin: bin, workDir: work, sessions: newCodexThreadSessions(""), developerInstructions: "仅执行用户明确授权的本地测试。"}
	f.sessions.setThreadID("test", thread)
	for _, tc := range []struct{ mode, policy, reviewer string }{
		{"workspaceWrite", "on-request", "user"},
		{"dangerFullAccess", "never", "user"},
		{"readOnly", "on-request", "user"},
		{"workspaceWrite", "on-request", "auto_review"},
	} {
		c = open()
		params, err := c.employeeResumeParams(ctx, thread)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.resumeThread(ctx, params); err != nil {
			t.Fatal(err)
		}
		call(c, "thread/settings/update", map[string]any{"threadId": thread, "sandboxPolicy": map[string]string{"type": tc.mode}, "approvalPolicy": tc.policy, "approvalsReviewer": tc.reviewer})
		if _, err := c.runTurn(ctx, thread, prompt, nil, nil); err != nil {
			t.Fatal(err)
		}
		c.close()
		answer, err := f.forward(ctx, "test", prompt)
		if err != nil || !strings.Contains(answer, "C9_P_LOCAL_OK") {
			t.Fatalf("真实续接失败: %q %v", answer, err)
		}
		c = open()
		params, err = c.employeeResumeParams(ctx, thread)
		if err != nil {
			t.Fatal(err)
		}
		raw := call(c, "thread/resume", params)
		var actual struct {
			Approval string                `json:"approvalPolicy"`
			Reviewer string                `json:"approvalsReviewer"`
			Sandbox  struct{ Type string } `json:"sandbox"`
		}
		if json.Unmarshal(raw, &actual) != nil || actual.Sandbox.Type != tc.mode || actual.Approval != tc.policy || actual.Reviewer != tc.reviewer {
			t.Fatalf("恢复权限改变: %+v, expected %+v", actual, tc)
		}
		c.close()
		t.Logf("thread=%s sandbox=%s approval=%s reviewer=%s preserved", thread, tc.mode, tc.policy, tc.reviewer)
	}
}
