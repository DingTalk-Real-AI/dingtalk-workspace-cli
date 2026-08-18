// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package todo

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func TestCrossPlatformCoverageTodoStrictCollectionsDistinguishEmptyFromMalformed(t *testing.T) {
	empty := map[string]any{"success": true, "result": map[string]any{"todoCards": []any{}, "hasMore": false, "page": float64(1), "size": float64(20)}}
	items, err := getMyTasksProjectStrict(empty)
	if err != nil || len(items) != 0 {
		t.Fatalf("explicit empty todoCards = %#v, %v; want legal empty", items, err)
	}
	if _, err := getMyTasksProjectStrict(map[string]any{"success": true, "result": map[string]any{"hasMore": false}}); err == nil || !strings.Contains(err.Error(), "列表容器") {
		t.Fatalf("missing collection error = %v", err)
	}
	if _, err := getMyTasksProjectStrict(map[string]any{"success": true, "result": map[string]any{"todoCards": []any{"bad"}, "hasMore": false}}); err == nil || !strings.Contains(err.Error(), "不是对象") {
		t.Fatalf("malformed item error = %v", err)
	}
	if _, err := getMyTasksProjectStrict(map[string]any{"success": true, "result": map[string]any{"todoCards": []any{map[string]any{"subject": "missing id"}}, "hasMore": false}}); err == nil || !strings.Contains(err.Error(), "taskId") {
		t.Fatalf("missing taskId error = %v", err)
	}
}

func TestCrossPlatformCoverageTodoDetailBindsRequestedTaskID(t *testing.T) {
	response := map[string]any{"success": true, "result": map[string]any{"todoDetailModel": map[string]any{"taskId": "task-a", "subject": "A"}}}
	detail, err := requireTodoDetail(response, "todo/get_todo_detail", "task-a")
	if err != nil || detail["subject"] != "A" {
		t.Fatalf("detail = %#v, %v", detail, err)
	}
	if _, err := requireTodoDetail(response, "todo/get_todo_detail", "task-b"); err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("identity mismatch error = %v", err)
	}
	if _, err := requireTodoDetail(map[string]any{"success": true, "result": map[string]any{}}, "todo/get_todo_detail", "task-a"); err == nil {
		t.Fatal("missing todoDetailModel unexpectedly accepted")
	}
}

func TestAllShortcutsTodoLifecycleContractsAreComplete(t *testing.T) {
	for _, item := range []struct {
		name    string
		rollout output.RolloutState
		result  bool
		safety  string
	}{
		{"create", Create.OutputRollout, Create.Contract.Result != nil, Create.Safety.Confirmation},
		{"update", Update.OutputRollout, Update.Contract.Result != nil, Update.Safety.Confirmation},
		{"complete", Complete.OutputRollout, Complete.Contract.Result != nil, Complete.Safety.Confirmation},
		{"reopen", Reopen.OutputRollout, Reopen.Contract.Result != nil, Reopen.Safety.Confirmation},
		{"search", Search.OutputRollout, Search.Contract.Result != nil, Search.Safety.Confirmation},
		{"comment", Comment.OutputRollout, Comment.Contract.Result != nil, Comment.Safety.Confirmation},
		{"upload-attachment", UploadAttachment.OutputRollout, UploadAttachment.Contract.Result != nil, UploadAttachment.Safety.Confirmation},
		{"reminder", Reminder.OutputRollout, Reminder.Contract.Result != nil, Reminder.Safety.Confirmation},
		{"get-my-tasks", GetMyTasks.OutputRollout, GetMyTasks.Contract.Result != nil, GetMyTasks.Safety.Confirmation},
		{"list-sub", ListSub.OutputRollout, ListSub.Contract.Result != nil, ListSub.Safety.Confirmation},
		{"get", Get.OutputRollout, Get.Contract.Result != nil, Get.Safety.Confirmation},
		{"list-attachment", ListAttachment.OutputRollout, ListAttachment.Contract.Result != nil, ListAttachment.Safety.Confirmation},
		{"list-comment", ListComment.OutputRollout, ListComment.Contract.Result != nil, ListComment.Safety.Confirmation},
	} {
		if item.rollout != output.RolloutUnifiedActive || !item.result || item.safety == "" {
			t.Errorf("%s contract incomplete: rollout=%q result=%v safety=%q", item.name, item.rollout, item.result, item.safety)
		}
	}
	if text := string(Reminder.Contract.Result.DataSchema); !strings.Contains(text, `"verified"`) || !strings.Contains(Reminder.Intent, "verified=false") {
		t.Fatalf("reminder must publish terminal-only verification boundary: %s / %s", text, Reminder.Intent)
	}
}
