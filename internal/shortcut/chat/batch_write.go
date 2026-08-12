// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chat

import (
	"fmt"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

type shortcutBatchWrite struct {
	target    string
	arguments map[string]any
}

// executeShortcutBatchWrite gives batch-style IM mutations one shared result
// contract. Dry-run publishes executable per-item actions without calling a
// write tool. Real execution preserves input order and records every failure
// instead of aborting the remaining items.
func executeShortcutBatchWrite(rt *shortcut.RuntimeContext, product, tool string, items []shortcutBatchWrite) error {
	if rt.DryRun() {
		actions := make([]map[string]any, 0, len(items))
		for _, item := range items {
			actions = append(actions, map[string]any{
				"target":    item.target,
				"tool":      tool,
				"arguments": item.arguments,
			})
		}
		return rt.Output(map[string]any{
			"contractVersion": "im.batch-write.v1",
			"dry_run":         true,
			"executed":        false,
			"preview_kind":    "plan",
			"tool":            tool,
			"actionCount":     len(actions),
			"failedCount":     0,
			"actions":         actions,
			"requestedCount":  len(items),
		})
	}

	succeeded := make([]map[string]any, 0, len(items))
	failures := make([]map[string]any, 0)
	for _, item := range items {
		data, err := rt.CallMCPWriteData(product, tool, item.arguments)
		if err != nil {
			failures = append(failures, map[string]any{
				"target": item.target,
				"error":  err.Error(),
			})
			continue
		}
		entry := map[string]any{"target": item.target}
		if len(data) > 0 {
			entry["result"] = data
		}
		succeeded = append(succeeded, entry)
	}
	result := map[string]any{
		"contractVersion": "im.batch-write.v1",
		"ok":              len(failures) == 0,
		"partial":         len(succeeded) > 0 && len(failures) > 0,
		"requestedCount":  len(items),
		"succeededCount":  len(succeeded),
		"failedCount":     len(failures),
		"succeeded":       succeeded,
		"failures":        failures,
	}
	if err := rt.Output(result); err != nil {
		return err
	}
	if len(failures) > 0 {
		return apperrors.NewAPI(
			fmt.Sprintf("批量执行 %s 失败：%d/%d 个目标未完成", tool, len(failures), len(items)),
			apperrors.WithOperation(product+"/"+tool),
			apperrors.WithReason("batch_write_failed"),
			apperrors.WithExecutionStarted(true),
			apperrors.WithRetryable(false),
			apperrors.WithDetails(map[string]any{
				"requestedCount": len(items),
				"succeededCount": len(succeeded),
				"failedCount":    len(failures),
				"partial":        len(succeeded) > 0,
			}),
		)
	}
	return nil
}
