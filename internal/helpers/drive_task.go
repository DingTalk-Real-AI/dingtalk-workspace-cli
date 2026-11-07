package helpers

// drive_task.go 提供统一异步任务查询（query_task）与 copy/move 自动轮询能力。
//
// 同步自闭源 MR 28427926（drive task get 统一异步任务查询）与
// MR 28769810（copy/move 异步自动轮询 + 四类型任务查询统一收敛）。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// TaskStatus represents the normalized lifecycle status of an async task.
type TaskStatus string

const (
	TaskStatusPending       TaskStatus = "PENDING"
	TaskStatusProcessing    TaskStatus = "PROCESSING"
	TaskStatusSuccess       TaskStatus = "SUCCESS"
	TaskStatusFailed        TaskStatus = "FAILED"
	TaskStatusPartialFailed TaskStatus = "PARTIAL_FAILED"
	TaskStatusTimeout       TaskStatus = "TIMEOUT"
)

// TaskResult is the unified, flattened representation of an async task query
// response. JSON field names follow camelCase convention per the DWS CLI
// output specification.
type TaskResult struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Status     TaskStatus `json:"status"`
	ResultURL  string     `json:"resultUrl,omitempty"`
	ResultName string     `json:"resultName,omitempty"`
	Message    string     `json:"message,omitempty"`
	CreateTime string     `json:"createTime,omitempty"`
}

// NormalizeStatus maps a raw status string from an MCP response to a
// canonical TaskStatus value.
//
// Mapping rules:
//   - PENDING, QUEUED                     → PENDING
//   - PROCESSING, RUNNING, IN_PROGRESS    → PROCESSING
//   - SUCCESS, SUCCEED, SUCCEEDED, DONE,
//     FINISHED, COMPLETE, COMPLETED       → SUCCESS
//   - FAILED, FAILURE, ERROR              → FAILED
//   - PARTIAL_FAILED, PARTIALLY_FAILED    → PARTIAL_FAILED
//   - TIMEOUT                             → TIMEOUT
//   - empty or unknown                    → PROCESSING (conservative)
func NormalizeStatus(raw string) TaskStatus {
	s := strings.ToUpper(strings.TrimSpace(raw))
	switch s {
	case "PENDING", "QUEUED":
		return TaskStatusPending
	case "PROCESSING", "RUNNING", "IN_PROGRESS":
		return TaskStatusProcessing
	case "SUCCESS", "SUCCEED", "SUCCEEDED", "DONE", "FINISHED", "COMPLETE", "COMPLETED":
		return TaskStatusSuccess
	case "FAILED", "FAILURE", "ERROR":
		return TaskStatusFailed
	case "PARTIAL_FAILED", "PARTIALLY_FAILED":
		return TaskStatusPartialFailed
	case "TIMEOUT":
		return TaskStatusTimeout
	default:
		// Empty or unrecognized: conservatively assume the task is still
		// in progress rather than marking it as failed.
		return TaskStatusProcessing
	}
}

// unwrapTaskResult extracts the inner "result" object from an MCP response if
// the top-level JSON contains a "result" wrapper. Returns the original map
// when no wrapper is present.
func unwrapTaskResult(data map[string]any) map[string]any {
	if result, ok := data["result"].(map[string]any); ok {
		return result
	}
	return data
}

// checkTaskBusinessError inspects a parsed MCP response for a business-level
// failure (success == false). Returns an error with the response message
// if found, nil otherwise.
func checkTaskBusinessError(data map[string]any) error {
	if success, ok := data["success"].(bool); ok && !success {
		msg, _ := data["message"].(string)
		if msg == "" {
			msg = "MCP 工具调用返回业务错误"
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// queryTaskCallTool routes the unified task query tool query_task to the
// drive (dingpan) MCP server. All QueryTask callers use it so automatic
// routing (resolveProductID) cannot resolve to the wrong server under
// doc/sheet command contexts.
func queryTaskCallTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	return callMCPToolReturnTextOnServer(ctx, "drive", toolName, args)
}

// QueryTask queries the status of an async task by task ID and type via the
// unified "query_task" MCP tool and normalizes the response into a TaskResult.
// taskType must be one of: export, import, copy, move (lowercase).
func QueryTask(ctx context.Context, taskID, taskType string) (*TaskResult, error) {
	text, err := queryTaskCallTool(ctx, "query_task", map[string]any{"taskId": taskID, "taskType": taskType})
	if err != nil {
		return nil, fmt.Errorf("查询任务失败: %w", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil, fmt.Errorf("解析 query_task 响应失败: %w", err)
	}

	data = unwrapTaskResult(data)
	if err := checkTaskBusinessError(data); err != nil {
		return nil, err
	}

	status, _ := data["status"].(string)
	resultURL, _ := data["resultUrl"].(string)
	resultName, _ := data["resultName"].(string)
	message, _ := data["message"].(string)
	createTime, _ := data["createTime"].(string)

	return &TaskResult{
		ID:         taskID,
		Type:       taskType,
		Status:     NormalizeStatus(status),
		ResultURL:  resultURL,
		ResultName: resultName,
		Message:    message,
		CreateTime: createTime,
	}, nil
}

// printTaskProgress writes progress hints to stderr so stdout carries only
// result data (JSON purity). The format matches Formatter.PrintInfo.
func printTaskProgress(msg string) {
	fmt.Fprintf(os.Stderr, "[INFO] %s\n", msg)
}

// taskPollInterval returns the progressive backoff polling interval
// (aligned with the lippi-doc-solution server recommendation):
//
//	attempts 1-5: 2s; 6-10: 5s; 11-20: 10s; 21-30: 15s
func taskPollInterval(attempt int) time.Duration {
	switch {
	case attempt <= 5:
		return 2 * time.Second
	case attempt <= 10:
		return 5 * time.Second
	case attempt <= 20:
		return 10 * time.Second
	default:
		return 15 * time.Second
	}
}

// pollCopyMoveTask polls a copy/move async task (query_task, drive endpoint)
// until a terminal state or the polling cap is reached.
//
// Terminal handling:
//   - SUCCESS        → return result
//   - PARTIAL_FAILED → return result (terminal, caller decides handling)
//   - FAILED/TIMEOUT → return error (with taskId and message)
//   - PENDING/PROCESSING/unknown → keep polling
func pollCopyMoveTask(ctx context.Context, taskType, taskID string) (*TaskResult, error) {
	const maxPolls = 30

	for attempt := 1; attempt <= maxPolls; attempt++ {
		interval := taskPollInterval(attempt)
		printTaskProgress(fmt.Sprintf("    第 %d/%d 次查询，等待 %v ...", attempt, maxPolls, interval))

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("任务轮询被取消 (taskId=%s): %w", taskID, ctx.Err())
		case <-helperAfter(interval):
		}

		result, err := QueryTask(ctx, taskID, taskType)
		if err != nil {
			return nil, fmt.Errorf("查询任务失败 (taskId=%s): %w", taskID, err)
		}

		switch result.Status {
		case TaskStatusSuccess:
			return result, nil
		case TaskStatusPartialFailed:
			// Partial failure is terminal: return the result and let the
			// caller decide how to surface it.
			return result, nil
		case TaskStatusFailed:
			if result.Message != "" {
				return nil, fmt.Errorf("任务失败 (taskId=%s): %s", taskID, result.Message)
			}
			return nil, fmt.Errorf("任务失败 (taskId=%s)", taskID)
		case TaskStatusTimeout:
			if result.Message != "" {
				return nil, fmt.Errorf("任务超时 (taskId=%s): %s", taskID, result.Message)
			}
			return nil, fmt.Errorf("任务超时 (taskId=%s)", taskID)
		default:
			// PENDING / PROCESSING / unknown: keep polling.
			continue
		}
	}

	return nil, fmt.Errorf("任务仍在处理中 (taskId=%s)，请稍后使用 dws drive task get --type %s --id %s 手动查询", taskID, taskType, taskID)
}

// driveTaskPartialFailure 在 copy/move 异步任务 PARTIAL_FAILED 终态时返回：
// 结构化 TaskResult 已打印到 stdout（含失败明细），这里只负责以 exit=1 退出并向
// stderr 输出一行简短说明（与 drive pull/push 的 drivePartialFailure 模式一致）。
type driveTaskPartialFailure struct{ taskType, taskID string }

func (e *driveTaskPartialFailure) Error() string {
	return fmt.Sprintf("drive %s: task %s partially failed", e.taskType, e.taskID)
}
func (e *driveTaskPartialFailure) RawStderr() string { return e.Error() }
func (e *driveTaskPartialFailure) ExitCode() int     { return 1 }

// runNodeTransferWithAsyncPoll executes copy_document/move_document and
// handles the async scenario automatically:
//   - synchronous completion (no taskId): passthrough, byte-identical to the
//     callMCPTool printing path
//   - async (taskId returned): announce then poll query_task, print the
//     normalized TaskResult JSON on completion; PARTIAL_FAILED still prints
//     the TaskResult JSON but returns driveTaskPartialFailure so the CLI
//     exits 1 (partial success is not success, same as drive pull/push)
//
// The submit call is routed to the doc server (copy_document/move_document
// are registered there); the polling query is routed to the drive server
// (query_task is registered on dingpan).
// ctx is expected to come from cmd.Context() so Ctrl-C / parent timeouts abort
// both the submit call and the polling loop.
func runNodeTransferWithAsyncPoll(ctx context.Context, mcpToolName string, toolArgs map[string]any) error {
	text, err := callMCPToolReturnTextOnServer(ctx, "doc", mcpToolName, toolArgs)
	if err != nil {
		return err
	}

	// Parse the response to extract the async task identifier: a non-empty
	// taskId means async (metadata wins); isAsync is a compatibility fallback.
	taskID := ""
	var body map[string]any
	if json.Unmarshal([]byte(text), &body) == nil {
		data := body
		if inner, ok := body["result"].(map[string]any); ok {
			data = inner
		}
		if v, ok := data["taskId"].(string); ok {
			taskID = v
		}
	}

	// Async: poll query_task until a terminal state.
	if taskID != "" {
		taskType := "copy"
		if mcpToolName == "move_document" {
			taskType = "move"
		}
		printTaskProgress(fmt.Sprintf("检测到异步任务 (taskId=%s)，自动轮询中...", taskID))
		result, pollErr := pollCopyMoveTask(ctx, taskType, taskID)
		if pollErr != nil {
			return pollErr
		}
		switch result.Status {
		case TaskStatusPartialFailed:
			printTaskProgress(fmt.Sprintf("部分任务失败，可使用 dws drive task get --type %s --id %s 查看明细", taskType, taskID))
		default:
			if taskType == "move" {
				printTaskProgress("移动完成")
			} else {
				printTaskProgress("复制完成")
			}
		}
		if printErr := deps.Out.PrintJSON(result); printErr != nil {
			return printErr
		}
		if result.Status == TaskStatusPartialFailed {
			return &driveTaskPartialFailure{taskType: taskType, taskID: taskID}
		}
		return nil
	}

	// Synchronous: mirror the callMCPTool success output path.
	if deps.Caller.Format() == "json" {
		var parsed any
		if err := json.Unmarshal([]byte(text), &parsed); err == nil {
			return deps.Out.PrintJSON(parsed)
		}
	}
	deps.Out.PrintRaw(text)
	return nil
}
