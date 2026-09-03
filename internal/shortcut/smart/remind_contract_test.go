// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package smart

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestRemindShortcutDoesNotAdvertiseDueTimeAsReminder(t *testing.T) {
	if strings.Contains(Remind.Description, "提醒时间") || strings.Contains(Remind.Intent, "截止/提醒") {
		t.Fatalf("shortcut contract still conflates dueTime with a reminder: %#v", Remind)
	}
	for _, flag := range Remind.Flags {
		if flag.Name == "at" && !strings.Contains(flag.Desc, "不是提醒时间") {
			t.Fatalf("--at description = %q, want explicit dueTime boundary", flag.Desc)
		}
	}
}

func TestRemindShortcutWritesAtOnlyAsDueTime(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"todo", "+remind",
		"--task", "交周报",
		"--at", "2026-03-10T18:00:00+08:00",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute +remind: %v", err)
	}
	if len(fake.calls) != 3 {
		t.Fatalf("tool calls = %#v, want profile read, todo create and detail verification", fake.calls)
	}
	call := fake.calls[1]
	if call.product != "todo" || call.tool != "create_personal_todo" {
		t.Fatalf("create call = %s/%s, want todo/create_personal_todo", call.product, call.tool)
	}
	request, ok := call.args["PersonalTodoCreateVO"].(map[string]any)
	if !ok {
		t.Fatalf("PersonalTodoCreateVO = %#v, want object", call.args["PersonalTodoCreateVO"])
	}
	if got := request["dueTime"]; got != int64(1773136800000) {
		t.Fatalf("dueTime = %#v, want 1773136800000", got)
	}
	if _, exists := request["reminderRules"]; exists {
		t.Fatalf("unexpected reminderRules in %#v", request)
	}
	if got := fake.calls[2].tool; got != "get_todo_detail" {
		t.Fatalf("verification call = %q, want get_todo_detail", got)
	}
}

func TestRemindShortcutRejectsInvalidAtBeforeTodoCreate(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"todo", "+remind", "--task", "交周报", "--at", "tomorrow", "--yes"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--at 时间格式无效") {
		t.Fatalf("error = %v, want invalid --at validation", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("tool calls = %#v, want local validation before any remote read", fake.calls)
	}
}

func TestTodoCreateShortcutsPreserveEpochZeroDueTime(t *testing.T) {
	const epoch = "1970-01-01T00:00:00Z"
	tests := []struct {
		name         string
		flag         string
		dryArgs      []string
		liveArgs     []string
		drySteps     map[string][]calendarSmartTestStep
		wantDryCalls int
	}{
		{
			name:         "assign due",
			flag:         "due",
			dryArgs:      []string{"todo", "+assign", "--to", "张三", "--task", "交周报", "--due", epoch, "--dry-run", "--yes"},
			liveArgs:     []string{"todo", "+assign", "--to", "张三", "--task", "交周报", "--due", epoch, "--yes"},
			drySteps:     map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}},
			wantDryCalls: 1,
		},
		{
			name:     "remind at",
			flag:     "at",
			dryArgs:  []string{"todo", "+remind", "--task", "交周报", "--at", epoch, "--dry-run", "--yes"},
			liveArgs: []string{"todo", "+remind", "--task", "交周报", "--at", epoch, "--yes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previewCaller := &calendarSmartTestCaller{steps: tt.drySteps}
			preview, _, err := runCalendarSmartCLI(t, previewCaller, tt.dryArgs...)
			if err != nil {
				t.Fatalf("dry-run with --%s: %v", tt.flag, err)
			}
			if got, exists := preview["dueTime"]; !exists || got != float64(0) {
				t.Fatalf("dry-run dueTime = %#v, exists=%v, want numeric zero", got, exists)
			}
			if got := len(previewCaller.calls); got != tt.wantDryCalls {
				t.Fatalf("dry-run tool calls = %#v", previewCaller.calls)
			}
			if previewCaller.counts["todo/create_personal_todo"] != 0 {
				t.Fatalf("dry-run attempted a write: %#v", previewCaller.calls)
			}

			fake := &platformCoverageCaller{}
			if _, _, err := runCalendarSmartCLI(t, fake, tt.liveArgs...); err != nil {
				t.Fatalf("live request with --%s: %v", tt.flag, err)
			}
			if len(fake.calls) != 3 {
				t.Fatalf("tool calls = %#v, want resolver, create, and verification", fake.calls)
			}
			request, ok := fake.calls[1].args["PersonalTodoCreateVO"].(map[string]any)
			if !ok {
				t.Fatalf("PersonalTodoCreateVO = %#v, want object", fake.calls[1].args["PersonalTodoCreateVO"])
			}
			if got, exists := request["dueTime"]; !exists || got != int64(0) {
				t.Fatalf("create dueTime = %#v, exists=%v, want int64 zero", got, exists)
			}
		})
	}
}
