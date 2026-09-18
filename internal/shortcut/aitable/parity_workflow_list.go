// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
	"strings"
)

func executeWorkflowList(rt *shortcut.RuntimeContext) error {
	limit := 20
	if rt.Changed("limit") {
		limit = rt.Int("limit")
	}
	offset := rt.Int("offset")
	if limit < 1 || limit > 100 || offset < 0 {
		return fmt.Errorf("workflow limit must be 1-100 and offset nonnegative")
	}
	all := rt.Bool("all")
	if rt.Changed("status") && !all {
		return fmt.Errorf("--status requires --all so filtering covers the complete workflow set")
	}
	pageLimit := rt.Int("page-limit")
	if pageLimit < 1 || pageLimit > 1000 {
		return fmt.Errorf("--page-limit must be 1-1000")
	}
	rows := []map[string]any{}
	seen := map[string]bool{}
	scanned := 0
	for page := 0; page < pageLimit; page++ {
		raw, err := rt.CallMCPData(serverHelper, "list_workflows", map[string]any{"baseId": rt.Str("base-id"), "limit": limit, "offset": offset})
		if err != nil {
			return err
		}
		list, err := workflowListProject(raw)
		if err != nil {
			return err
		}
		_, more, known := aitabletarget.Pagination(raw)
		if !known {
			more = len(list) == limit
		}
		for _, row := range list {
			id, ok := row["workflowId"].(string)
			if !ok || id == "" || seen[id] {
				return fmt.Errorf("workflow list has missing/duplicate ID; completeness is unknown")
			}
			seen[id] = true
			scanned++
			if rt.Changed("status") {
				enabled, known := workflowEnabled(row["status"])
				if !known {
					return fmt.Errorf("workflow status is unknown; cannot filter truthfully")
				}
				if enabled != (rt.Str("status") == "enabled") {
					continue
				}
			}
			rows = append(rows, row)
		}
		offset += len(list)
		if !all || !more {
			out := map[string]any{"count": len(rows), "workflows": rows, "hasMore": more}
			if more {
				out["nextOffset"] = offset
			}
			if all {
				out["complete"] = true
				out["scannedCount"] = scanned
			}
			return rt.Output(out)
		}
		if len(list) == 0 {
			return fmt.Errorf("workflow list has more data but offset cannot advance")
		}
	}
	return fmt.Errorf("workflow list reached --page-limit; no complete result was published")
}
func workflowEnabled(value any) (bool, bool) {
	if b, ok := value.(bool); ok {
		return b, true
	}
	if s, ok := value.(string); ok {
		switch strings.ToLower(s) {
		case "enabled", "enable", "true", "running", "active":
			return true, true
		case "disabled", "disable", "false", "stopped", "stop", "inactive":
			return false, true
		}
	}
	return false, false
}
