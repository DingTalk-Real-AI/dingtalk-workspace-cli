// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chat

import (
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
			"dry_run":        true,
			"executed":       false,
			"preview_kind":   "plan",
			"tool":           tool,
			"actionCount":    len(actions),
			"failedCount":    0,
			"actions":        actions,
			"requestedCount": len(items),
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
	return rt.Output(map[string]any{
		"ok":             len(failures) == 0,
		"partial":        len(succeeded) > 0 && len(failures) > 0,
		"requestedCount": len(items),
		"succeededCount": len(succeeded),
		"failedCount":    len(failures),
		"succeeded":      succeeded,
		"failures":       failures,
	})
}
