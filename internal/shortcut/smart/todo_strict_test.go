// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func TestCrossPlatformCoverageTodoSmartPagerRequiresHasMoreAndStableItems(t *testing.T) {
	valid := map[string]any{"success": true, "result": map[string]any{"todoCards": []any{}, "hasMore": false, "page": float64(1), "size": float64(20)}}
	items, hasMore, err := shortcutTodoCardsStrict(valid)
	if err != nil || hasMore || len(items) != 0 {
		t.Fatalf("valid empty page = %#v, %v, %v", items, hasMore, err)
	}
	for name, response := range map[string]map[string]any{
		"missing cards":   {"success": true, "result": map[string]any{"hasMore": false}},
		"missing hasMore": {"success": true, "result": map[string]any{"todoCards": []any{}}},
		"bad element":     {"success": true, "result": map[string]any{"todoCards": []any{"bad"}, "hasMore": false}},
		"missing id":      {"success": true, "result": map[string]any{"todoCards": []any{map[string]any{"subject": "x"}}, "hasMore": false}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := shortcutTodoCardsStrict(response); err == nil {
				t.Fatalf("malformed response accepted: %#v", response)
			}
		})
	}
}

func TestAllShortcutsTodoSmartContractsAndRelatedAlias(t *testing.T) {
	if RelatedTasks.Command != "+get-related-tasks" || len(RelatedTasks.Aliases) != 1 || RelatedTasks.Aliases[0] != "+related-tasks" {
		t.Fatalf("related command/alias = %q / %#v", RelatedTasks.Command, RelatedTasks.Aliases)
	}
	for _, item := range []struct {
		name    string
		rollout output.RolloutState
		result  bool
		dryRun  bool
	}{
		{"related", RelatedTasks.OutputRollout, RelatedTasks.Contract.Result != nil, RelatedTasks.Contract.DryRun != nil},
		{"due-today", DueToday.OutputRollout, DueToday.Contract.Result != nil, DueToday.Contract.DryRun != nil},
		{"assign", Assign.OutputRollout, Assign.Contract.Result != nil, Assign.Contract.DryRun != nil},
		{"assign-multi", AssignMulti.OutputRollout, AssignMulti.Contract.Result != nil, AssignMulti.Contract.DryRun != nil},
		{"created", CreatedTodos.OutputRollout, CreatedTodos.Contract.Result != nil, CreatedTodos.Contract.DryRun != nil},
		{"overdue", Overdue.OutputRollout, Overdue.Contract.Result != nil, Overdue.Contract.DryRun != nil},
		{"todo-done", TodoDone.OutputRollout, TodoDone.Contract.Result != nil, TodoDone.Contract.DryRun != nil},
		{"remind", Remind.OutputRollout, Remind.Contract.Result != nil, Remind.Contract.DryRun != nil},
	} {
		if item.rollout != output.RolloutUnifiedActive || !item.result {
			t.Errorf("%s missing unified Result contract", item.name)
		}
		wantDryRun := item.name == "assign" || item.name == "assign-multi" || item.name == "todo-done" || item.name == "remind"
		if item.dryRun != wantDryRun {
			t.Errorf("%s dry-run declaration=%v, want %v", item.name, item.dryRun, wantDryRun)
		}
	}
	if !strings.Contains(Assign.Intent, "创建") {
		t.Fatal("legacy +assign semantics must remain create-and-assign")
	}
}
