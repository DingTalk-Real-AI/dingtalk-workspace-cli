// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// validateAIPlanArguments requires a fixed subset, never an implicit whole-column
// refresh. References are permitted only during static plan preflight.
func validateAIPlanArguments(args map[string]any, allowReferences bool) error {
	if ref, ok := args["tableId"].(map[string]any); !(allowReferences && ok && ref["$ref"] != nil) {
		if table, ok := args["tableId"].(string); !ok || table == "" {
			return fmt.Errorf("AI 计划 tableId 必须绑定真实 ID")
		}
	}
	for key, limit := range map[string]int{"fieldIds": 10, "recordIds": 500} {
		if ref, ok := args[key].(map[string]any); allowReferences && ok && ref["$ref"] != nil {
			continue
		}
		values, ok := args[key].([]any)
		if !ok || len(values) == 0 || len(values) > limit {
			return fmt.Errorf("AI 计划 %s 必须为 1～%d 个固定 ID", key, limit)
		}
		seen := map[string]bool{}
		for _, value := range values {
			if ref, ok := value.(map[string]any); allowReferences && ok && ref["$ref"] != nil {
				continue
			}
			id, ok := value.(string)
			if !ok || id == "" || seen[id] {
				return fmt.Errorf("AI 计划 %s 必须为不同的非空 ID", key)
			}
			seen[id] = true
		}
	}
	return nil
}

// aiPlanTargets consumes only this submission's submitted receipts. Missing IDs,
// conflicts and unknown delivery stop the plan; resume never submits them again.
func aiPlanTargets(current *buildPlanStepState) ([]map[string]any, error) {
	if err := validateAIPlanArguments(current.Arguments, false); err != nil {
		return nil, err
	}
	tasks, found := findNamedObjectList(current.Resources, "tasks")
	fields, _ := current.Arguments["fieldIds"].([]any)
	if !found || len(tasks) != len(fields) || len(fields) == 0 {
		return nil, fmt.Errorf("AI 提交缺少完整逐字段回执；保留原提交，不重发")
	}
	byField := map[string]map[string]any{}
	for _, task := range tasks {
		field := stringValue(task, "fieldId")
		if byField[field] != nil || field == "" {
			return nil, fmt.Errorf("AI 提交字段身份缺失或重复")
		}
		byField[field] = task
	}
	bound := []map[string]any{}
	for _, field := range fields {
		task := byField[field.(string)]
		if task == nil || task["status"] != "submitted" || stringValue(task, "taskId") == "" || !declaredValueMatches(task["recordIds"], current.Arguments["recordIds"]) || !declaredValueMatches(current.Arguments["recordIds"], task["recordIds"]) {
			return nil, fmt.Errorf("AI 字段没有本轮已受理的固定任务证据；不重新触发")
		}
		bound = append(bound, map[string]any{"taskId": task["taskId"], "fieldId": field, "recordIds": task["recordIds"]})
	}
	// Reuse the public target validator after projection, including its limits.
	raw, _ := json.Marshal(bound)
	return parseAITaskTargets(string(raw))
}

// waitAIPlanStep persists partial evidence under the parent plan's identity and
// lock. The current provider cannot prove readability, so dependents stay pending.
func waitAIPlanStep(rt *shortcut.RuntimeContext, state *buildPlanState, current *buildPlanStepState, persist func() error) error {
	if err := validateAIPlanArguments(current.Arguments, false); err != nil {
		return err
	}
	targets, err := aiPlanTargets(current)
	if err != nil {
		return err
	}
	base, table := state.BaseID, stringValue(current.Arguments, "tableId")
	progress := newAITaskPoll(base, table, targets)
	if current.AIProgress != nil {
		progress, err = restoreAITaskPoll(base, table, targets, *current.AIProgress)
		if err != nil {
			return err
		}
	}
	current.Status = "pending"
	state.Status = "pending"
	save := func() error {
		evidence := progress.evidence()
		current.AIProgress = &evidence
		return persist()
	}
	if err := save(); err != nil {
		return err
	}
	_, timedOut, err := pollAITasks(rt, base, table, targets, progress, false, &state.Metrics, save)
	if timedOut {
		return nil
	}
	if err != nil {
		return err
	}
	// validateAITaskStatusReceipt accepts only resultReadableVerified=false.
	// Keep the plan pending even when terminal/version evidence is complete.
	return nil
}
