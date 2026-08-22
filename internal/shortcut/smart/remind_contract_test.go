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
