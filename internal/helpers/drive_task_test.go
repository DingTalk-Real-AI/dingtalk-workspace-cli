package helpers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// ── NormalizeStatus 枚举映射 ──

func TestCrossPlatformCoverageNormalizeStatusMapping(t *testing.T) {
	cases := []struct {
		raw  string
		want TaskStatus
	}{
		{"PENDING", TaskStatusPending},
		{"QUEUED", TaskStatusPending},
		{"PROCESSING", TaskStatusProcessing},
		{"RUNNING", TaskStatusProcessing},
		{"IN_PROGRESS", TaskStatusProcessing},
		{"SUCCESS", TaskStatusSuccess},
		{"SUCCEED", TaskStatusSuccess},
		{"SUCCEEDED", TaskStatusSuccess},
		{"DONE", TaskStatusSuccess},
		{"FINISHED", TaskStatusSuccess},
		{"COMPLETE", TaskStatusSuccess},
		{"COMPLETED", TaskStatusSuccess},
		{"FAILED", TaskStatusFailed},
		{"FAILURE", TaskStatusFailed},
		{"ERROR", TaskStatusFailed},
		{"PARTIAL_FAILED", TaskStatusPartialFailed},
		{"PARTIALLY_FAILED", TaskStatusPartialFailed},
		{"TIMEOUT", TaskStatusTimeout},
		// Empty or unrecognized input is conservatively mapped to PROCESSING.
		{"", TaskStatusProcessing},
		{"  running  ", TaskStatusProcessing},
		{"MYSTERY", TaskStatusProcessing},
	}
	for _, tc := range cases {
		if got := NormalizeStatus(tc.raw); got != tc.want {
			t.Errorf("NormalizeStatus(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// ── checkTaskBusinessError：success=false 业务错误分支 ──

func TestCrossPlatformCoverageCheckTaskBusinessError(t *testing.T) {
	if err := checkTaskBusinessError(map[string]any{"success": false, "message": "boom"}); err == nil || err.Error() != "boom" {
		t.Fatalf("business error = %v, want boom", err)
	}
	if err := checkTaskBusinessError(map[string]any{"success": false}); err == nil || err.Error() != "MCP 工具调用返回业务错误" {
		t.Fatalf("default business error = %v", err)
	}
	if err := checkTaskBusinessError(map[string]any{"success": true}); err != nil {
		t.Fatalf("success=true error = %v", err)
	}
	if err := checkTaskBusinessError(map[string]any{"message": "no success flag"}); err != nil {
		t.Fatalf("missing success flag error = %v", err)
	}
	if err := checkTaskBusinessError(map[string]any{"success": "false"}); err != nil {
		t.Fatalf("non-bool success error = %v", err)
	}
}

// ── QueryTask：成功 / result 包装 / 调用错误 / 解析错误 / 业务错误 ──

func TestCrossPlatformCoverageQueryTaskOutcomes(t *testing.T) {
	t.Run("success flat payload", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"status":"SUCCESS","resultUrl":"https://x.test/f.docx","resultName":"f.docx","message":"ok","createTime":"2026-08-21 08:00:00"}`}}}
		installScriptedCaller(t, caller)
		result, err := QueryTask(context.Background(), "t1", "export")
		if err != nil {
			t.Fatal(err)
		}
		want := &TaskResult{
			ID: "t1", Type: "export", Status: TaskStatusSuccess,
			ResultURL: "https://x.test/f.docx", ResultName: "f.docx",
			Message: "ok", CreateTime: "2026-08-21 08:00:00",
		}
		if *result != *want {
			t.Fatalf("result = %+v, want %+v", *result, *want)
		}
		if caller.server != "drive" || caller.tool != "query_task" {
			t.Fatalf("routed to %s/%s, want drive/query_task", caller.server, caller.tool)
		}
		if caller.args["taskId"] != "t1" || caller.args["taskType"] != "export" {
			t.Fatalf("query args = %v", caller.args)
		}
	})

	t.Run("result wrapper unwraps", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"status":"QUEUED"}}`}}})
		result, err := QueryTask(context.Background(), "t2", "copy")
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != TaskStatusPending {
			t.Fatalf("status = %q, want PENDING", result.Status)
		}
	})

	t.Run("call error", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("network down")}}})
		_, err := QueryTask(context.Background(), "t3", "move")
		if err == nil || !strings.Contains(err.Error(), "查询任务失败") {
			t.Fatalf("error = %v, want 查询任务失败", err)
		}
	})

	t.Run("parse error", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{`}}})
		_, err := QueryTask(context.Background(), "t4", "import")
		if err == nil || !strings.Contains(err.Error(), "解析 query_task 响应失败") {
			t.Fatalf("error = %v, want 解析 query_task 响应失败", err)
		}
	})

	t.Run("business error nested in result wrapper", func(t *testing.T) {
		// Top-level success=false is rejected by parseMCPToolTextResult before
		// QueryTask sees it; the inner business error is what checkTaskBusinessError handles.
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"success":false,"message":"task not found"}}`}}})
		_, err := QueryTask(context.Background(), "t5", "export")
		if err == nil || err.Error() != "task not found" {
			t.Fatalf("error = %v, want task not found", err)
		}
	})
}

// ── taskPollInterval：渐进退避档位 ──

func TestCrossPlatformCoverageTaskPollInterval(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 2 * time.Second},
		{5, 2 * time.Second},
		{6, 5 * time.Second},
		{10, 5 * time.Second},
		{11, 10 * time.Second},
		{20, 10 * time.Second},
		{21, 15 * time.Second},
		{30, 15 * time.Second},
	}
	for _, tc := range cases {
		if got := taskPollInterval(tc.attempt); got != tc.want {
			t.Errorf("taskPollInterval(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

// ── pollCopyMoveTask：取消 / 各终态 / 查询失败 / 轮询上限 ──

func TestCrossPlatformCoveragePollCopyMoveTaskTerminalStates(t *testing.T) {
	installImmediateTiming(t)
	// The cancelled-context probe may randomly take the helperAfter branch of
	// the select (both channels are ready), which then calls QueryTask; a
	// caller must therefore be installed before the probe.
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"status":"PROCESSING"}`}}})

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pollCopyMoveTask(cancelled, "copy", "t1"); err == nil || !strings.Contains(err.Error(), "任务轮询被取消") {
		t.Fatalf("cancelled error = %v", err)
	}

	steps := []struct {
		name  string
		steps []scriptedToolStep
		check func(t *testing.T, result *TaskResult, err error)
	}{
		{"success", []scriptedToolStep{{text: `{"status":"SUCCESS"}`}},
			func(t *testing.T, r *TaskResult, err error) {
				if err != nil || r == nil || r.Status != TaskStatusSuccess {
					t.Fatalf("result=%v err=%v", r, err)
				}
			}},
		{"success after processing", []scriptedToolStep{{text: `{"status":"PROCESSING"}`}, {text: `{"status":"SUCCESS"}`}},
			func(t *testing.T, r *TaskResult, err error) {
				if err != nil || r == nil || r.Status != TaskStatusSuccess {
					t.Fatalf("result=%v err=%v", r, err)
				}
			}},
		{"partial failed is terminal", []scriptedToolStep{{text: `{"status":"PARTIAL_FAILED"}`}},
			func(t *testing.T, r *TaskResult, err error) {
				if err != nil || r == nil || r.Status != TaskStatusPartialFailed {
					t.Fatalf("result=%v err=%v", r, err)
				}
			}},
		{"failed with message", []scriptedToolStep{{text: `{"status":"FAILED","message":"permission denied"}`}},
			func(t *testing.T, r *TaskResult, err error) {
				if err == nil || !strings.Contains(err.Error(), "任务失败 (taskId=t1): permission denied") {
					t.Fatalf("error = %v", err)
				}
			}},
		{"failed without message", []scriptedToolStep{{text: `{"status":"FAILED"}`}},
			func(t *testing.T, r *TaskResult, err error) {
				if err == nil || !strings.Contains(err.Error(), "任务失败 (taskId=t1)") {
					t.Fatalf("error = %v", err)
				}
			}},
		{"timeout with message", []scriptedToolStep{{text: `{"status":"TIMEOUT","message":"slow"}`}},
			func(t *testing.T, r *TaskResult, err error) {
				if err == nil || !strings.Contains(err.Error(), "任务超时 (taskId=t1): slow") {
					t.Fatalf("error = %v", err)
				}
			}},
		{"timeout without message", []scriptedToolStep{{text: `{"status":"TIMEOUT"}`}},
			func(t *testing.T, r *TaskResult, err error) {
				if err == nil || !strings.Contains(err.Error(), "任务超时 (taskId=t1)") {
					t.Fatalf("error = %v", err)
				}
			}},
		{"query error aborts polling", []scriptedToolStep{{err: errors.New("mcp unavailable")}},
			func(t *testing.T, r *TaskResult, err error) {
				if err == nil || !strings.Contains(err.Error(), "查询任务失败 (taskId=t1)") {
					t.Fatalf("error = %v", err)
				}
			}},
		{"poll cap keeps processing", []scriptedToolStep{{text: `{"status":"PENDING"}`}},
			func(t *testing.T, r *TaskResult, err error) {
				if err == nil || !strings.Contains(err.Error(), "任务仍在处理中 (taskId=t1)") {
					t.Fatalf("error = %v", err)
				}
			}},
	}
	for _, tc := range steps {
		t.Run(tc.name, func(t *testing.T) {
			installScriptedCaller(t, &scriptedToolCaller{steps: tc.steps})
			result, err := pollCopyMoveTask(context.Background(), "copy", "t1")
			tc.check(t, result, err)
		})
	}
}

// ── runNodeTransferWithAsyncPoll：ctx 取消传播 / 异步终态 / 同步直通 ──

func TestCrossPlatformCoverageRunNodeTransferWithAsyncPoll(t *testing.T) {
	installImmediateTiming(t)

	t.Run("submit error propagates", func(t *testing.T) {
		boom := errors.New("submit failed")
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: boom}}})
		if err := runNodeTransferWithAsyncPoll(context.Background(), "copy_document", map[string]any{"nodeId": "n1"}); !errors.Is(err, boom) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("cancelled context aborts before polling", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"taskId":"t9"}`}}})
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		err := runNodeTransferWithAsyncPoll(cancelled, "copy_document", map[string]any{"nodeId": "n1"})
		if err == nil || !strings.Contains(err.Error(), "任务轮询被取消") {
			t.Fatalf("error = %v, want 任务轮询被取消", err)
		}
	})

	t.Run("async copy polls drive server and prints result", func(t *testing.T) {
		caller := &scriptedToolCaller{
			format: "json",
			steps: []scriptedToolStep{
				{text: `{"taskId":"t10"}`},                      // submit on doc server
				{text: `{"status":"SUCCESS","message":"done"}`}, // query_task on drive server
			},
		}
		installScriptedCaller(t, caller)
		if err := runNodeTransferWithAsyncPoll(context.Background(), "copy_document", map[string]any{"nodeId": "n1"}); err != nil {
			t.Fatal(err)
		}
		if len(caller.serverLog) != 2 || caller.serverLog[0] != "doc" || caller.serverLog[1] != "drive" {
			t.Fatalf("server log = %v, want [doc drive]", caller.serverLog)
		}
	})

	t.Run("async move announces completion", func(t *testing.T) {
		caller := &scriptedToolCaller{
			format: "json",
			steps: []scriptedToolStep{
				{text: `{"taskId":"t11"}`},
				{text: `{"status":"SUCCESS"}`},
			},
		}
		installScriptedCaller(t, caller)
		if err := runNodeTransferWithAsyncPoll(context.Background(), "move_document", map[string]any{"nodeId": "n1"}); err != nil {
			t.Fatal(err)
		}
		if len(caller.serverLog) != 2 || caller.toolLog[0] != "move_document" {
			t.Fatalf("calls = %v", caller.toolLog)
		}
	})

	t.Run("async partial failure hints manual query", func(t *testing.T) {
		caller := &scriptedToolCaller{
			format: "json",
			steps: []scriptedToolStep{
				{text: `{"taskId":"t12"}`},
				{text: `{"status":"PARTIAL_FAILED"}`},
			},
		}
		installScriptedCaller(t, caller)
		err := runNodeTransferWithAsyncPoll(context.Background(), "copy_document", map[string]any{"nodeId": "n1"})
		// PARTIAL_FAILED：结构化 TaskResult 仍打印到 stdout，但命令必须以
		// 非零退出码结束（errors.As 提取分型，参照 drive pull 的断言写法）。
		var pf *driveTaskPartialFailure
		if !errors.As(err, &pf) {
			t.Fatalf("expected *driveTaskPartialFailure, got %T %v", err, err)
		}
		if pf.taskType != "copy" || pf.taskID != "t12" {
			t.Errorf("partial failure = %#v, want copy/t12", pf)
		}
		if pf.ExitCode() != 1 || pf.RawStderr() == "" {
			t.Errorf("exit code = %d, stderr = %q, want exit 1 with non-empty stderr", pf.ExitCode(), pf.RawStderr())
		}
		if len(caller.serverLog) != 2 {
			t.Fatalf("calls = %v", caller.serverLog)
		}
	})

	// stdout 写失败（如管道破裂）：PrintJSON 的写错误必须上抛，不能被吞掉
	//（与 drive pull/push/sync 的 printFailurePropagates 契约一致）。
	t.Run("async print failure propagates", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{
			format: "json",
			steps: []scriptedToolStep{
				{text: `{"taskId":"t13"}`},
				{text: `{"status":"SUCCESS"}`},
			},
		})
		deps.Out.w = failingWriter{}
		if err := runNodeTransferWithAsyncPoll(context.Background(), "copy_document", map[string]any{"nodeId": "n1"}); err == nil {
			t.Fatal("expected the PrintJSON writer failure to propagate")
		}
	})

	t.Run("sync json passthrough", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"nodeId":"n2"}`}}})
		if err := runNodeTransferWithAsyncPoll(context.Background(), "copy_document", map[string]any{"nodeId": "n2"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("sync non-json passthrough prints raw text", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{format: "table", steps: []scriptedToolStep{{text: "plain result"}}})
		if err := runNodeTransferWithAsyncPoll(context.Background(), "move_document", map[string]any{"nodeId": "n3"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("sync json with invalid body falls back to raw", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: "not json"}}})
		if err := runNodeTransferWithAsyncPoll(context.Background(), "copy_document", map[string]any{"nodeId": "n4"}); err != nil {
			t.Fatal(err)
		}
	})
}

// ── drive task get 命令级：参数校验 / 类型校验 / dry-run / 查询链路 ──

func TestCrossPlatformCoverageDriveTaskGetCommand(t *testing.T) {
	t.Run("missing required flags", func(t *testing.T) {
		if err := executeDriveEdge(t, &scriptedToolCaller{}, "task", "get"); err == nil {
			t.Fatal("missing flags returned nil")
		}
	})

	t.Run("unsupported task type", func(t *testing.T) {
		err := executeDriveEdge(t, &scriptedToolCaller{}, "task", "get", "--type", "sync", "--id", "t1")
		if err == nil || !strings.Contains(err.Error(), "不支持的任务类型") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("dry run prints preview", func(t *testing.T) {
		err := executeDriveEdge(t, &scriptedToolCaller{dry: true}, "task", "get", "--type", "export", "--id", "t1", "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("dry run rejects unsupported task type before preview", func(t *testing.T) {
		caller := &scriptedToolCaller{dry: true}
		err := executeDriveEdge(t, caller, "task", "get", "--type", "sync", "--id", "t1", "--dry-run")
		if err == nil || !strings.Contains(err.Error(), "不支持的任务类型") {
			t.Fatalf("error = %v", err)
		}
		if caller.calls != 0 {
			t.Fatalf("expected zero tool calls, got %d", caller.calls)
		}
	})

	t.Run("query success prints task result", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"status":"SUCCESS","resultUrl":"https://x.test/f.docx"}`}}}
		if err := executeDriveEdge(t, caller, "task", "get", "--type", "export", "--id", "t1", "--format", "json"); err != nil {
			t.Fatal(err)
		}
		if caller.server != "drive" || caller.tool != "query_task" {
			t.Fatalf("routed to %s/%s", caller.server, caller.tool)
		}
	})

	t.Run("query error surfaces", func(t *testing.T) {
		err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("boom")}}}, "task", "get", "--type", "copy", "--id", "t2")
		if err == nil || !strings.Contains(err.Error(), "查询任务失败") {
			t.Fatalf("error = %v", err)
		}
	})
}
