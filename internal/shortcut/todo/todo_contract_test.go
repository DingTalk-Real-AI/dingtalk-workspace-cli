// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package todo

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
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
		dryRun  bool
	}{
		{"create", Create.OutputRollout, Create.Contract.Result != nil, Create.Safety.Confirmation, Create.Contract.DryRun != nil},
		{"update", Update.OutputRollout, Update.Contract.Result != nil, Update.Safety.Confirmation, Update.Contract.DryRun != nil},
		{"complete", Complete.OutputRollout, Complete.Contract.Result != nil, Complete.Safety.Confirmation, Complete.Contract.DryRun != nil},
		{"reopen", Reopen.OutputRollout, Reopen.Contract.Result != nil, Reopen.Safety.Confirmation, Reopen.Contract.DryRun != nil},
		{"search", Search.OutputRollout, Search.Contract.Result != nil, Search.Safety.Confirmation, Search.Contract.DryRun != nil},
		{"comment", Comment.OutputRollout, Comment.Contract.Result != nil, Comment.Safety.Confirmation, Comment.Contract.DryRun != nil},
		{"upload-attachment", UploadAttachment.OutputRollout, UploadAttachment.Contract.Result != nil, UploadAttachment.Safety.Confirmation, UploadAttachment.Contract.DryRun != nil},
		{"reminder", Reminder.OutputRollout, Reminder.Contract.Result != nil, Reminder.Safety.Confirmation, Reminder.Contract.DryRun != nil},
		{"get-my-tasks", GetMyTasks.OutputRollout, GetMyTasks.Contract.Result != nil, GetMyTasks.Safety.Confirmation, GetMyTasks.Contract.DryRun != nil},
		{"list-sub", ListSub.OutputRollout, ListSub.Contract.Result != nil, ListSub.Safety.Confirmation, ListSub.Contract.DryRun != nil},
		{"get", Get.OutputRollout, Get.Contract.Result != nil, Get.Safety.Confirmation, Get.Contract.DryRun != nil},
		{"list-attachment", ListAttachment.OutputRollout, ListAttachment.Contract.Result != nil, ListAttachment.Safety.Confirmation, ListAttachment.Contract.DryRun != nil},
		{"list-comment", ListComment.OutputRollout, ListComment.Contract.Result != nil, ListComment.Safety.Confirmation, ListComment.Contract.DryRun != nil},
	} {
		if item.rollout != output.RolloutUnifiedActive || !item.result || item.safety == "" {
			t.Errorf("%s contract incomplete: rollout=%q result=%v safety=%q", item.name, item.rollout, item.result, item.safety)
		}
		wantDryRun := item.safety == "user_required" && item.name != "upload-attachment"
		if item.dryRun != wantDryRun {
			t.Errorf("%s dry-run declaration=%v, want %v", item.name, item.dryRun, wantDryRun)
		}
	}
	if text := string(Reminder.Contract.Result.DataSchema); !strings.Contains(text, `"verified"`) || !strings.Contains(Reminder.Intent, "verified=false") {
		t.Fatalf("reminder must publish terminal-only verification boundary: %s / %s", text, Reminder.Intent)
	}
}

func TestTodoRuntimeRelationshipsArePublishedAsConstraints(t *testing.T) {
	if len(Update.Constraints) != 1 || Update.Constraints[0].Kind != shortcut.ConstraintAtLeastOne ||
		strings.Join(Update.Constraints[0].Flags, ",") != "title,due,priority" {
		t.Fatalf("update constraints = %#v", Update.Constraints)
	}
	if GetMyTasks.Validate == nil || len(GetMyTasks.Constraints) != 1 ||
		GetMyTasks.Constraints[0].Kind != shortcut.ConstraintCustom ||
		strings.Join(GetMyTasks.Constraints[0].Flags, ",") != "all,max-pages" {
		t.Fatalf("get-my-tasks constraints = %#v validate=%v", GetMyTasks.Constraints, GetMyTasks.Validate != nil)
	}
	if Reminder.Validate == nil || len(Reminder.Constraints) != 2 ||
		Reminder.Constraints[0].Kind != shortcut.ConstraintExactlyOne ||
		Reminder.Constraints[1].Kind != shortcut.ConstraintCustom {
		t.Fatalf("reminder constraints = %#v validate=%v", Reminder.Constraints, Reminder.Validate != nil)
	}
}

func TestCrossPlatformCoverageTodoWriteReceiptMarksExecutionStarted(t *testing.T) {
	err := requireTodoWriteReceipt(map[string]any{}, "todo/create_personal_todo")
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if typed.ExecutionStarted == nil || !*typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable {
		t.Fatalf("write failure safety = started %v retryable_set %v retryable %v", typed.ExecutionStarted, typed.RetryableSet, typed.Retryable)
	}
}

func TestCrossPlatformCoverageTodoWriteVerificationPreservesReason(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cause      error
		wantReason string
		wantOrigin string
		wantType   apperrors.Category
	}{
		{"api", todoResponseError("todo/get_todo_detail", "missing_detail", "missing detail"), "missing_detail", "mcp", apperrors.CategoryAPI},
		{"plain", errors.New("read timeout"), "write_verification_failed", "mcp", apperrors.CategoryAPI},
		{"auth", apperrors.NewAuth("expired", apperrors.WithReason("auth_expired"), apperrors.WithOrigin("gateway")), "auth_expired", "gateway", apperrors.CategoryAuth},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := todoWriteVerificationError("todo/create_personal_todo", tc.cause)
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error type = %T, want *errors.Error", err)
			}
			if typed.Category != tc.wantType || typed.Reason != tc.wantReason || typed.Origin != tc.wantOrigin ||
				typed.FailureStage != "write_verification" ||
				typed.ExecutionStarted == nil || !*typed.ExecutionStarted ||
				!typed.RetryableSet || typed.Retryable || !errors.Is(err, tc.cause) {
				t.Fatalf("verification failure = %#v, cause preserved=%v", typed, errors.Is(err, tc.cause))
			}
		})
	}
}
