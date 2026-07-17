package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func executeTodoEdge(t *testing.T, caller *scriptedToolCaller, args ...string) error {
	t.Helper()
	previous := deps
	previousArgs := os.Args
	os.Args = append([]string{"dws", "todo"}, os.Args[1:]...)
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	defer func() {
		deps = previous
		os.Args = previousArgs
	}()

	cmd := newTodoCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageTodoCreateAndListCommandEdges(t *testing.T) {
	validDate := "2026-03-10T18:00:00+08:00"
	errorCases := [][]string{
		{"task", "create", "--title", "x", "--executors", "u", "--remind-at", validDate},
		{"task", "create", "--title", "x", "--executors", "u", "--due", "bad"},
		{"task", "create-sub", "--title", "x", "--executors", "u", "--parent-id", "abc"},
		{"task", "create-sub", "--title", "x", "--executors", "u", "--parent-id", "1", "--remind-at", validDate},
		{"task", "create-sub", "--title", "x", "--executors", "u", "--parent-id", "1", "--due", "bad"},
		{"task", "list", "--role-types", "invalid"},
		{"task", "list", "--plan-finish-date-start", "bad"},
		{"task", "list", "--plan-finish-date-end", "bad"},
	}
	for _, args := range errorCases {
		if err := executeTodoEdge(t, &scriptedToolCaller{}, args...); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}

	validCases := [][]string{
		{"task", "create", "--title", "x", "--executors", "u1,u2", "--due", validDate, "--priority", "40", "--recurrence", "daily"},
		{"task", "create", "--subject", "x", "--executors", "u", "--priority", "bad"},
		{"task", "create-sub", "--content", "x", "--executors", "u", "--parent-id", "1", "--due", validDate, "--priority", "40", "--recurrence", "daily"},
		{"task", "create-sub", "--title", "x", "--executors", "u", "--parent-id", "1", "--priority", "bad"},
		{"task", "list", "--size", "bad"},
		{"task", "list", "--status", "true", "--priority", "40,bad", "--role-types", "creator,executor", "--plan-finish-date-start", validDate, "--plan-finish-date-end", validDate},
	}
	for _, args := range validCases {
		if err := executeTodoEdge(t, &scriptedToolCaller{}, args...); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
	}
	if err := executeTodoEdge(t, &scriptedToolCaller{dry: true}, "task", "list", "--size", "21"); err != nil {
		t.Fatalf("dry auto-page: %v", err)
	}
}

func TestCrossPlatformCoverageTodoSimpleCommandValidationAndSuccessEdges(t *testing.T) {
	errorCases := [][]string{
		{"task", "create"},
		{"task", "create", "--title", "x"},
		{"task", "create-sub"},
		{"task", "create-sub", "--title", "x"},
		{"task", "list-sub"},
		{"task", "update"},
		{"task", "done"},
		{"task", "get"},
		{"task", "delete"},
		{"task", "add-executor"},
		{"task", "remove-executor"},
		{"task", "add-participant"},
		{"task", "remove-participant"},
		{"task", "add-reminder"},
		{"task", "reset-reminder"},
		{"task", "add-attachment"},
		{"task", "list-attachment"},
		{"task", "remove-attachment"},
		{"comment", "add"},
		{"comment", "list"},
		{"comment", "delete"},
	}
	for _, args := range errorCases {
		if err := executeTodoEdge(t, &scriptedToolCaller{}, args...); err == nil {
			t.Fatalf("expected validation error for %v", args)
		}
	}

	validCases := []struct {
		args  []string
		steps []scriptedToolStep
	}{
		{args: []string{"task", "list-sub", "--task-id", "1"}},
		{
			args: []string{"task", "done", "--task-id", "1", "--status", "true"},
			steps: []scriptedToolStep{
				{text: `{"result":{"todoDetailModel":{"taskId":"1"}}}`},
				{text: `{}`},
			},
		},
		{args: []string{"task", "add-executor", "--task-id", "1", "--executors", "u1,u2"}},
		{args: []string{"task", "remove-executor", "--task-id", "1", "--executors", "u1,u2"}},
		{args: []string{"task", "add-participant", "--task-id", "1", "--participants", "u1,u2"}},
		{args: []string{"task", "remove-participant", "--task-id", "1", "--participants", "u1,u2"}},
		{
			args: []string{"task", "list-attachment", "--task-id", "1"},
			steps: []scriptedToolStep{
				{text: `{"result":{"todoDetailModel":{"taskId":"1"}}}`},
				{text: `{}`},
			},
		},
		{args: []string{"comment", "add", "--task-id", "1", "--content", "hello"}},
		{args: []string{"comment", "list", "--task-id", "1"}},
	}
	for _, tc := range validCases {
		if err := executeTodoEdge(t, &scriptedToolCaller{steps: tc.steps}, tc.args...); err != nil {
			t.Fatalf("execute %v: %v", tc.args, err)
		}
	}
}

func TestCrossPlatformCoverageTodoUpdateReminderAndDetailEdges(t *testing.T) {
	validDate := "2026-03-10T18:00:00+08:00"
	errorCases := [][]string{
		{"task", "update", "--task-id", "1", "--remind-at", validDate},
		{"task", "update", "--task-id", "1", "--due", "bad"},
		{"task", "add-reminder", "--task-id", "1", "--base-time", "customTime", "--reminder-time-stamp", "bad"},
		{"task", "reset-reminder", "--task-id", "1", "--reminder-rules", `[{"baseTime":"customTime","reminderTimeStamp":"bad"}]`},
	}
	for _, args := range errorCases {
		if err := executeTodoEdge(t, &scriptedToolCaller{}, args...); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
	validCases := [][]string{
		{"task", "update", "--task-id", "1", "--title", "new", "--due", validDate, "--priority", "40", "--done", "true"},
		{"task", "update", "--task-id", "1", "--priority", "bad"},
		{"task", "add-reminder", "--task-id", "1", "--base-time", "dueTime", "--due-date-offset", "-10"},
		{"task", "add-reminder", "--task-id", "1", "--base-time", "customTime", "--reminder-time-stamp", validDate},
		{"task", "reset-reminder", "--task-id", "1", "--reminder-rules", `not-json`},
		{"task", "reset-reminder", "--task-id", "1", "--reminder-rules", `[1,{"baseTime":"dueTime"},{"baseTime":"customTime","reminderTimeStamp":"` + validDate + `"}]`},
	}
	for _, args := range validCases {
		if err := executeTodoEdge(t, &scriptedToolCaller{}, args...); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
	}

	details := []scriptedToolStep{
		{text: ""},
		{text: "not-json"},
		{text: `{"result":{"todoDetailModel":{"detailUrl":{"appUrl":"https://app.example/a b","pcUrl":"https://pc.example/a b"}}}}`},
	}
	for _, step := range details {
		if err := executeTodoEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{step}}, "task", "get", "--task-id", "1"); err != nil {
			t.Fatalf("detail %q: %v", step.text, err)
		}
	}
	boom := errors.New("boom")
	if err := executeTodoEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: boom}}}, "task", "get", "--task-id", "1"); err == nil {
		t.Fatal("detail tool error returned nil")
	}
}

func TestCrossPlatformCoverageTodoAttachmentCommandEdges(t *testing.T) {
	file := filepath.Join(t.TempDir(), "attachment.txt")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := executeTodoEdge(t, &scriptedToolCaller{dry: true}, "task", "add-attachment", "--task-id", "1", "--file-path", file); err != nil {
		t.Fatalf("dry attachment: %v", err)
	}
	if err := executeTodoEdge(t, &scriptedToolCaller{}, "task", "add-attachment", "--task-id", "1", "--file-path", file+".missing"); err == nil {
		t.Fatal("missing attachment returned nil")
	}

	originalPut := httpPutFile
	defer func() { httpPutFile = originalPut }()
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
	credential := `{"resourceUrl":"https://upload.invalid","uploadKey":"key"}`
	for _, tc := range []struct {
		name  string
		steps []scriptedToolStep
		ok    bool
	}{
		{"upload-error", []scriptedToolStep{{err: errors.New("upload")}}, false},
		{"commit-parse-error", []scriptedToolStep{{text: credential}, {text: `{}`}}, false},
		{"success", []scriptedToolStep{{text: credential}, {text: `{"dentryId":1,"spaceId":2}`}, {text: `{}`}}, true},
	} {
		err := executeTodoEdge(t, &scriptedToolCaller{steps: tc.steps}, "task", "add-attachment", "--task-id", "1", "--file-path", file)
		if tc.ok && err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%s returned nil", tc.name)
		}
	}
}

func TestCrossPlatformCoverageTodoDeleteCancellationAndConfirmationEdges(t *testing.T) {
	originalArgs, originalStdin := os.Args, os.Stdin
	defer func() {
		os.Args = originalArgs
		os.Stdin = originalStdin
	}()

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeEnd.WriteString("no\nno\nno\n"); err != nil {
		t.Fatal(err)
	}
	_ = writeEnd.Close()
	os.Stdin = readEnd
	os.Args = []string{"dws"}
	for _, args := range [][]string{
		{"task", "delete", "--task-id", "1"},
		{"task", "remove-attachment", "--task-id", "1", "--attachment-id", "2"},
		{"comment", "delete", "--task-id", "1", "--comment-id", "2"},
	} {
		if err := executeTodoEdge(t, &scriptedToolCaller{}, args...); err != nil {
			t.Fatalf("cancel %v: %v", args, err)
		}
	}
	_ = readEnd.Close()

	os.Args = []string{"dws", "--yes"}
	for _, args := range [][]string{
		{"task", "delete", "--task-id", "1"},
		{"task", "remove-attachment", "--task-id", "1", "--attachment-id", "2"},
		{"comment", "delete", "--task-id", "1", "--comment-id", "2"},
	} {
		if err := executeTodoEdge(t, &scriptedToolCaller{}, args...); err != nil {
			t.Fatalf("confirm %v: %v", args, err)
		}
	}
}

func TestCrossPlatformCoverageTodoMD5AndEmptyListEdges(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := todoFileMD5Hex
	todoFileMD5Hex = func(string) (string, error) { return "", errors.New("md5") }
	defer func() { todoFileMD5Hex = original }()
	if _, err := buildTodoLocalFileMeta(file, "", ""); err == nil {
		t.Fatal("md5 error returned nil")
	}
	if got := parseStringList(", ,"); got != nil {
		t.Fatalf("empty list=%v", got)
	}
}
