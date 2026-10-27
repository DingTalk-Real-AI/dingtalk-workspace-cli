// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// pollAITasks is shared by standalone waits and persisted build plans. save runs
// after each read/backoff, including failed reads; it never triggers an AI task.
func pollAITasks(rt *shortcut.RuntimeContext, base, table string, tasks []map[string]any,
	progress *aiTaskPoll, once bool, metrics *aitableTaskMetrics, save func() error) (int, bool, error) {
	ctx := rt.Command().Context()
	polls := 0
	for {
		if err := aiTaskTerminalFailure(progress.body); err != nil {
			return polls, false, err
		}
		if progress.body["terminalVerified"] == true {
			return polls, false, nil
		}
		for _, batch := range progress.batches(tasks) {
			if ctx.Err() != nil {
				return polls, ctx.Err() == context.DeadlineExceeded, ctx.Err()
			}
			polls++
			started := time.Now()
			raw, callErr := rt.CallMCPData(serverMain, "get_ai_field_task_status", map[string]any{"baseId": base, "tableId": table, "tasks": batch})
			metrics.recordCall(false, started, raw)
			if callErr == nil {
				body := parityResponseObject(raw)
				if body["baseId"] != base || body["tableId"] != table {
					callErr = apperrors.NewAPI("AI 状态查询目标不一致", apperrors.WithRetryable(false))
				} else if err := validateAITaskStatusReceipt(body, batch); err != nil {
					callErr = apperrors.NewAPI(err.Error(), apperrors.WithRetryable(false))
				} else {
					progress.merge(body)
				}
			}
			if err := save(); err != nil {
				return polls, false, err
			}
			if callErr != nil {
				return polls, ctx.Err() == context.DeadlineExceeded && errors.Is(callErr, context.DeadlineExceeded), aiTaskReadFailure(callErr, progress.body)
			}
			if err := aiTaskTerminalFailure(progress.body); err != nil {
				return polls, false, err
			}
			if progress.body["terminalVerified"] == true || once {
				return polls, false, nil
			}
		}
		started := time.Now()
		err := bootstrapReadbackWait(ctx, min(time.Duration(polls)*250*time.Millisecond, 2*time.Second))
		metrics.BackoffMS += time.Since(started).Milliseconds()
		if saveErr := save(); saveErr != nil {
			return polls, false, saveErr
		}
		if err != nil {
			return polls, err == context.DeadlineExceeded, err
		}
	}
}

func aiTaskTerminalFailure(body map[string]any) error {
	tasks, _ := findNamedObjectList(body, "tasks")
	for _, task := range tasks {
		rows, _ := findNamedObjectList(task, "rows")
		for _, row := range rows {
			switch row["state"] {
			case "failed", "timed_out", "cancelled":
				return apperrors.NewAPI("本轮 AI 任务存在失败、超时或取消项，保留任务 ID 与原始行状态", apperrors.WithReason("aitable_ai_task_terminal_failure"), apperrors.WithRetryable(false), apperrors.WithDetails(map[string]any{"result": body}))
			}
		}
	}
	return nil
}
