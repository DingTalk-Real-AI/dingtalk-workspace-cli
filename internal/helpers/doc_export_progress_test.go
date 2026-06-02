package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

// docExportStubRunner 驱动一次「提交→查询命中 SUCCESS（无 downloadUrl）」的导出流程，
// 走到 writeCommandPayload 但不触发实际下载（无网络）。
type docExportStubRunner struct{}

func (docExportStubRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	switch inv.Tool {
	case "submit_export_job":
		return executor.Result{Response: map[string]any{"jobId": "job-123"}}, nil
	case "query_export_job":
		// SUCCESS 但不带 downloadUrl → 跳过 asynctask.Download，仍输出结构化 payload
		return executor.Result{Response: map[string]any{"status": "SUCCESS"}}, nil
	default:
		return executor.Result{}, nil
	}
}

// TestDocExportProgressGoesToStderrNotStdout 守护 #388 引入的回归：导出进度文案一度
// 被改到 stdout，与 writeCommandPayload 的 JSON payload 混流，破坏 agent / MCP / `| jq`
// 的解析。约定（与 aitable export 一致）：进度 → stderr，结构化 payload → stdout。
func TestDocExportProgressGoesToStderrNotStdout(t *testing.T) {
	cmd := docHandler{}.Command(docExportStubRunner{})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"export", "--node", "nodeABC123", "--output", "/tmp/dws-export-progress-test.docx"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, stderr.String())
	}

	// stdout 必须是干净、可解析的 JSON —— 这是 agent / MCP / jq 的契约
	out := strings.TrimSpace(stdout.String())
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("stdout 不是干净 JSON（进度泄漏到了 stdout?）: err=%v\nstdout:\n%s", err, stdout.String())
	}
	if payload["jobId"] != "job-123" {
		t.Fatalf("payload.jobId = %#v, want job-123; stdout:\n%s", payload["jobId"], stdout.String())
	}

	// 进度标记绝不允许出现在 stdout
	for _, marker := range []string{"[1/3]", "提交导出任务", "[2/3]"} {
		if strings.Contains(stdout.String(), marker) {
			t.Fatalf("进度标记 %q 泄漏到 stdout:\n%s", marker, stdout.String())
		}
	}

	// 进度应当落在 stderr
	if !strings.Contains(stderr.String(), "[1/3]") {
		t.Fatalf("期望进度出现在 stderr，实际 stderr:\n%s", stderr.String())
	}
}
