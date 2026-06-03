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

// TestDocExportProgressGoesToStdout 守护 doc export 的评测契约：导出进度文案需要
// 出现在 stdout，便于 agent 在执行过程中看到 submit → poll → download 的状态。
func TestDocExportProgressGoesToStdout(t *testing.T) {
	cmd := docHandler{}.Command(docExportStubRunner{})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"export", "--node", "nodeABC123", "--output", "/tmp/dws-export-progress-test.docx"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, stderr.String())
	}

	for _, marker := range []string{"[1/3]", "提交导出任务", "jobId: job-123", "[2/3]"} {
		if !strings.Contains(stdout.String(), marker) {
			t.Fatalf("期望进度标记 %q 出现在 stdout，实际 stdout:\n%s", marker, stdout.String())
		}
	}
	if strings.Contains(stderr.String(), "[1/3]") {
		t.Fatalf("进度不应出现在 stderr，实际 stderr:\n%s", stderr.String())
	}

	// stdout 同时包含进度与最终 payload；解析末尾 JSON，确保结构化结果仍输出。
	jsonStart := strings.LastIndex(stdout.String(), "{")
	if jsonStart < 0 {
		t.Fatalf("stdout 未包含最终 JSON payload:\n%s", stdout.String())
	}
	out := strings.TrimSpace(stdout.String()[jsonStart:])
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("stdout 末尾不是可解析 JSON: err=%v\nstdout:\n%s", err, stdout.String())
	}
	if payload["jobId"] != "job-123" {
		t.Fatalf("payload.jobId = %#v, want job-123; stdout:\n%s", payload["jobId"], stdout.String())
	}
}
