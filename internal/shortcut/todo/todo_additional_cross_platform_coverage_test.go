// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package todo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

const todoCoverageTime = "2026-08-17T10:00:00+08:00"

type todoCoverageCaller struct {
	responses map[string][]string
	history   []string
	arguments []map[string]any
}

func (caller *todoCoverageCaller) CallTool(_ context.Context, _, tool string, arguments map[string]any) (*edition.ToolResult, error) {
	caller.history = append(caller.history, tool)
	caller.arguments = append(caller.arguments, arguments)
	queue := caller.responses[tool]
	if len(queue) == 0 {
		return nil, errors.New("missing fake response for " + tool)
	}
	caller.responses[tool] = queue[1:]
	if queue[0] == "__ERROR__" {
		return nil, errors.New("injected todo failure")
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: queue[0]}}}, nil
}

func (caller *todoCoverageCaller) CallReadTool(ctx context.Context, product, tool string, arguments map[string]any) (*edition.ToolResult, error) {
	return caller.CallTool(ctx, product, tool, arguments)
}

func (*todoCoverageCaller) Format() string { return "json" }
func (*todoCoverageCaller) DryRun() bool   { return false }
func (*todoCoverageCaller) Fields() string { return "" }
func (*todoCoverageCaller) JQ() string     { return "" }

func runTodoCoverage(t *testing.T, declaration shortcut.Shortcut, caller *todoCoverageCaller, args ...string) error {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	service := &cobra.Command{Use: "todo"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"todo", declaration.Command}, args...))
	return root.Execute()
}

func runTodoUnifiedCoverage(t *testing.T, declaration shortcut.Shortcut, caller *todoCoverageCaller, args ...string) map[string]any {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	service := &cobra.Command{Use: "todo"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"todo", declaration.Command}, args...))
	executed, err := root.ExecuteC()
	if err != nil {
		t.Fatalf("execute %s: %v", declaration.Command, err)
	}
	if code, emitted, emitErr := output.EmitStoredResult(executed); emitErr != nil || !emitted || code != 0 {
		t.Fatalf("emit %s: code=%d emitted=%v err=%v", declaration.Command, code, emitted, emitErr)
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s output: %v\n%s", declaration.Command, err, stdout.String())
	}
	return envelope.Data
}

func normalizedTodoArguments(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		t.Fatal(err)
	}
	return normalized
}

func todoRuntimeForTest(t *testing.T, declaration shortcut.Shortcut, values map[string]string) *shortcut.RuntimeContext {
	t.Helper()
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return shortcut.RuntimeContextForTest(cmd, declaration)
}

func requireTodoExecutionStartedError(t *testing.T, err error, wantReason string) {
	t.Helper()
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if typed.Reason != wantReason || typed.ExecutionStarted == nil || !*typed.ExecutionStarted ||
		!typed.RetryableSet || typed.Retryable || typed.Cause == nil {
		t.Fatalf("post-write failure = %#v", typed)
	}
}

func TestCrossPlatformCoverageTodoCommonStrictBranches(t *testing.T) {
	for _, response := range []map[string]any{{}, {"success": "yes"}, {"success": false}} {
		if _, err := requireTodoResponse(response, "todo/test"); err == nil {
			t.Fatalf("bad response accepted: %#v", response)
		}
	}
	if _, err := requireTodoResponse(map[string]any{"result": map[string]any{}}, "todo/test"); err != nil {
		t.Fatalf("response without optional success rejected: %v", err)
	}
	for _, response := range []map[string]any{{}, {"result": map[string]any{}}, {"success": "yes"}, {"success": false}} {
		if err := requireTodoWriteReceipt(response, "todo/test"); err == nil {
			t.Fatalf("bad receipt accepted: %#v", response)
		}
	}
	if err := requireTodoWriteReceipt(map[string]any{"success": true}, "todo/test"); err != nil {
		t.Fatal(err)
	}

	for _, response := range []map[string]any{
		{"success": false},
		{"success": true, "result": map[string]any{"items": map[string]any{}}},
		{"success": true, "result": map[string]any{}},
		{"success": true, "result": map[string]any{"items": []any{"bad"}}},
	} {
		if _, err := requireTodoCollection(response, "todo/test", "items"); err == nil {
			t.Fatalf("bad collection accepted: %#v", response)
		}
	}
	items, err := requireTodoCollection(map[string]any{"success": true, "result": []any{map[string]any{"id": "x"}}}, "todo/test", "items")
	if err != nil || len(items) != 1 {
		t.Fatalf("direct wrapper collection=%#v %v", items, err)
	}

	if _, err := findTodoHasMore(map[string]any{}, 5); err == nil {
		t.Fatal("deep pagination accepted")
	}
	for _, response := range []map[string]any{
		{"hasMore": "bad"},
		{"result": []any{}},
		{},
	} {
		if _, err := todoHasMore(response); err == nil {
			t.Fatalf("bad pagination accepted: %#v", response)
		}
	}
	if more, err := todoHasMore(map[string]any{"result": map[string]any{"data": map[string]any{"hasMore": true}}}); err != nil || !more {
		t.Fatalf("nested pagination=%v %v", more, err)
	}

	if raw, found, err := findTodoCollection(map[string]any{}, []string{"items"}, 5); err != nil || found || raw != nil {
		t.Fatalf("deep collection=%#v %v %v", raw, found, err)
	}
	for _, response := range []map[string]any{
		{"items": map[string]any{}},
		{"result": "bad"},
	} {
		if _, _, err := findTodoCollection(response, []string{"items"}, 0); err == nil {
			t.Fatalf("bad collection wrapper accepted: %#v", response)
		}
	}
	if raw, found, err := findTodoCollection(map[string]any{"data": []any{}}, []string{"items"}, 0); err != nil || !found || raw == nil {
		t.Fatalf("direct array wrapper=%#v %v %v", raw, found, err)
	}

	for _, response := range []map[string]any{
		{"success": false},
		{"success": true, "result": map[string]any{"todoDetailModel": []any{}}},
		{"success": true, "result": map[string]any{}},
		{"success": true, "result": map[string]any{"todoDetailModel": map[string]any{"subject": "no id"}}},
	} {
		if _, err := requireTodoDetail(response, "todo/test", ""); err == nil {
			t.Fatalf("bad detail accepted: %#v", response)
		}
	}
	for _, response := range []map[string]any{
		{"success": true, "result": map[string]any{"taskId": "task-1"}},
		{"success": true, "taskId": "task-1"},
	} {
		if detail, err := requireTodoDetail(response, "todo/test", "task-1"); err != nil || todoStableString(detail, "taskId") != "task-1" {
			t.Fatalf("fallback detail=%#v %v", detail, err)
		}
	}
	if object, found, err := findTodoObject(map[string]any{}, []string{"todo"}, 5); err != nil || found || object != nil {
		t.Fatalf("deep object=%#v %v %v", object, found, err)
	}
	for _, response := range []map[string]any{{"todo": []any{}}, {"result": []any{}}} {
		if _, _, err := findTodoObject(response, []string{"todo"}, 0); err == nil {
			t.Fatalf("bad object wrapper accepted: %#v", response)
		}
	}

	if got := findTodoStableString(map[string]any{}, []string{"id"}, 6); got != "" {
		t.Fatalf("deep stable string=%q", got)
	}
	for _, tc := range []struct {
		value any
		want  string
	}{
		{" id ", "id"},
		{json.Number("12"), "12"},
		{float64(13), "13"},
		{float64(13.5), ""},
		{14, "14"},
		{int64(15), "15"},
	} {
		if got := todoStableString(map[string]any{"id": tc.value}, "id"); got != tc.want {
			t.Fatalf("stable %#v=%q want %q", tc.value, got, tc.want)
		}
	}
	if got := findTodoStableString(map[string]any{"result": map[string]any{"data": map[string]any{"id": "nested"}}}, []string{"id"}, 0); got != "nested" {
		t.Fatalf("nested stable string=%q", got)
	}
}

func TestCrossPlatformCoverageTodoProjectionBranches(t *testing.T) {
	for name, project := range map[string]func(map[string]any) error{
		"sub":        func(data map[string]any) error { _, err := listSubProjectStrict(data); return err },
		"attachment": func(data map[string]any) error { _, err := listAttachmentProjectStrict(data); return err },
		"comment":    func(data map[string]any) error { _, err := listCommentProjectStrict(data); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := project(map[string]any{"success": true, "result": map[string]any{"list": []any{map[string]any{"name": "missing id"}}, "hasMore": false}}); err == nil {
				t.Fatal("item without stable id accepted")
			}
		})
	}
	subs, err := listSubProjectStrict(map[string]any{"success": true, "result": map[string]any{"list": []any{map[string]any{"id": "sub-1", "title": "title", "deadline": 1}}}})
	if err != nil || len(subs) != 1 || subs[0]["subject"] != "title" || subs[0]["dueTime"] != 1 {
		t.Fatalf("sub projection=%#v %v", subs, err)
	}
	attachments, err := listAttachmentProjectStrict(map[string]any{"success": true, "result": map[string]any{"items": []any{map[string]any{"id": "a-1", "name": "file", "size": 1, "type": "text"}}}})
	if err != nil || len(attachments) != 1 {
		t.Fatalf("attachment projection=%#v %v", attachments, err)
	}
	comments, err := listCommentProjectStrict(map[string]any{"success": true, "result": map[string]any{"comments": []any{map[string]any{"id": "c-1", "text": "body", "userId": "u", "createdTime": 1}}}})
	if err != nil || len(comments) != 1 {
		t.Fatalf("comment projection=%#v %v", comments, err)
	}

	_ = listSubProject(map[string]any{"result": []any{"bad", map[string]any{}, map[string]any{"title": "x", "id": "s", "deadline": 1}}})
	_ = listAttachmentProject(map[string]any{"data": []any{"bad", map[string]any{}, map[string]any{"id": "a", "name": "n", "size": 1, "type": "t"}}})
	_ = listCommentProject(map[string]any{"result": []any{"bad", map[string]any{}, map[string]any{"id": "c", "text": "x", "creator": "u", "gmtCreate": 1}}})
	if listSubFirst(map[string]any{}, "x") != nil || listAttachmentFirst(map[string]any{}, "x") != nil || listCommentFirst(map[string]any{}, "x") != nil {
		t.Fatal("missing projection fields were invented")
	}
}

func TestCrossPlatformCoverageTodoLifecycleCore(t *testing.T) {
	if _, err := parseTodoMillis("due", "bad"); err == nil {
		t.Fatal("bad due time accepted")
	}
	if millis, err := parseTodoMillis("due", todoCoverageTime); err != nil || millis == 0 {
		t.Fatalf("due millis=%d %v", millis, err)
	}
	if got := nonEmptyStrings([]string{" a ", "", " b "}); strings.Join(got, ",") != "a,b" {
		t.Fatalf("non-empty strings=%#v", got)
	}
	if err := runTodoCoverage(t, UploadAttachment, &todoCoverageCaller{responses: map[string][]string{}}, "--task-id", "t", "--file-path", "file", "--yes"); err == nil {
		t.Fatal("unavailable upload shortcut executed")
	}
	if got := strconv.Itoa(todoPageSize); got != "20" {
		t.Fatal(got)
	}
}

func TestCrossPlatformCoverageTodoDryRunPreviewsExactWriteArguments(t *testing.T) {
	tests := []struct {
		name        string
		declaration shortcut.Shortcut
		tool        string
		args        []string
		responses   map[string][]string
	}{
		{
			name: "create", declaration: Create, tool: "create_personal_todo",
			args: []string{"--title", "x", "--executors", "u1,u2", "--due", todoCoverageTime, "--priority", "40"},
			responses: map[string][]string{
				"create_personal_todo": {`{"success":true,"result":{"taskId":"task-1"}}`},
				"get_todo_detail":      {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","subject":"x"}}}`},
			},
		},
		{
			name: "update", declaration: Update, tool: "update_todo_task",
			args: []string{"--task-id", "task-1", "--title", "x", "--due", todoCoverageTime, "--priority", "40"},
			responses: map[string][]string{
				"update_todo_task": {`{"success":true}`},
				"get_todo_detail":  {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","subject":"x","dueTime":1786932000000,"priority":40}}}`},
			},
		},
		{
			name: "complete", declaration: Complete, tool: "update_todo_done_status",
			args: []string{"--task-id", "task-1"},
			responses: map[string][]string{
				"get_todo_detail":         {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":false}}}`, `{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":true}}}`},
				"update_todo_done_status": {`{"success":true}`},
			},
		},
		{
			name: "reopen", declaration: Reopen, tool: "update_todo_done_status",
			args: []string{"--task-id", "task-1"},
			responses: map[string][]string{
				"get_todo_detail":         {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":true}}}`, `{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":false}}}`},
				"update_todo_done_status": {`{"success":true}`},
			},
		},
		{
			name: "comment", declaration: Comment, tool: "add_todo_comment",
			args: []string{"--task-id", "task-1", "--content", "body"},
			responses: map[string][]string{
				"list_todo_comment": {`{"success":true,"result":{"comments":[],"hasMore":false}}`, `{"success":true,"result":{"comments":[{"commentId":"new","content":"body"}],"hasMore":false}}`},
				"add_todo_comment":  {`{"success":true}`},
			},
		},
		{
			name: "reminder-clear", declaration: Reminder, tool: "reset_todo_reminder",
			args:      []string{"--task-id", "task-1", "--clear"},
			responses: map[string][]string{"reset_todo_reminder": {`{"success":true}`}},
		},
		{
			name: "reminder-due", declaration: Reminder, tool: "add_todo_reminder",
			args:      []string{"--task-id", "task-1", "--base-time", "dueTime", "--due-date-offset", "-30"},
			responses: map[string][]string{"add_todo_reminder": {`{"success":true}`}},
		},
		{
			name: "reminder-custom", declaration: Reminder, tool: "add_todo_reminder",
			args:      []string{"--task-id", "task-1", "--base-time", "customTime", "--at", todoCoverageTime},
			responses: map[string][]string{"add_todo_reminder": {`{"success":true}`}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			previewCaller := &todoCoverageCaller{responses: map[string][]string{}}
			preview := runTodoUnifiedCoverage(t, tc.declaration, previewCaller, append(append([]string{}, tc.args...), "--dry-run")...)
			if len(previewCaller.history) != 0 {
				t.Fatalf("dry-run made remote calls: %#v", previewCaller.history)
			}
			if preview["tool"] != tc.tool {
				t.Fatalf("preview tool=%#v want %q", preview["tool"], tc.tool)
			}
			previewParams, ok := preview["params"].(map[string]any)
			if !ok {
				t.Fatalf("preview params=%#v", preview["params"])
			}

			confirmedCaller := &todoCoverageCaller{responses: tc.responses}
			confirmedArgs := append(append([]string{}, tc.args...), "--yes")
			if err := runTodoCoverage(t, tc.declaration, confirmedCaller, confirmedArgs...); err != nil {
				t.Fatal(err)
			}
			writeIndex := -1
			for index, tool := range confirmedCaller.history {
				if tool == tc.tool {
					writeIndex = index
					break
				}
			}
			if writeIndex < 0 {
				t.Fatalf("confirmed calls=%#v, missing %s", confirmedCaller.history, tc.tool)
			}
			actual := normalizedTodoArguments(t, confirmedCaller.arguments[writeIndex])
			if !mapsEqualJSON(previewParams, actual) {
				t.Fatalf("preview params=%#v\nactual params=%#v", previewParams, actual)
			}
		})
	}
}

func mapsEqualJSON(left, right map[string]any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func TestCrossPlatformCoverageTodoCreateAndUpdate(t *testing.T) {
	for name, values := range map[string]map[string]string{
		"create-required": {"title": "", "executors": ""},
		"create-due":      {"title": "x", "executors": "u", "due": "bad"},
		"create-priority": {"title": "x", "executors": "u", "priority": "11"},
		"update-none":     {"task-id": "task-1"},
		"update-title":    {"task-id": "task-1", "title": ""},
		"update-due":      {"task-id": "task-1", "due": "bad"},
		"update-priority": {"task-id": "task-1", "priority": "11"},
	} {
		t.Run(name, func(t *testing.T) {
			declaration, execute := Create, createTodo
			if strings.HasPrefix(name, "update-") {
				declaration, execute = Update, updateTodo
			}
			if err := execute(todoRuntimeForTest(t, declaration, values)); err == nil {
				t.Fatal("invalid lifecycle values accepted")
			}
		})
	}
	if err := runTodoCoverage(t, Create, &todoCoverageCaller{responses: map[string][]string{}}, "--title", "x", "--executors", "u", "--due", todoCoverageTime, "--priority", "40", "--dry-run", "--yes"); err != nil {
		t.Fatalf("create dry-run: %v", err)
	}
	if err := runTodoCoverage(t, Update, &todoCoverageCaller{responses: map[string][]string{}}, "--task-id", "task-1", "--title", "x", "--due", todoCoverageTime, "--priority", "40", "--dry-run", "--yes"); err != nil {
		t.Fatalf("update dry-run: %v", err)
	}

	createCases := map[string]map[string][]string{
		"call":       {"create_personal_todo": {"__ERROR__"}},
		"receipt":    {"create_personal_todo": {`{"result":{}}`}},
		"id":         {"create_personal_todo": {`{"success":true,"result":{}}`}},
		"read-call":  {"create_personal_todo": {`{"success":true,"result":{"taskId":"task-1"}}`}, "get_todo_detail": {"__ERROR__"}},
		"read-shape": {"create_personal_todo": {`{"success":true,"result":{"taskId":"task-1"}}`}, "get_todo_detail": {`{"success":true,"result":{}}`}},
		"mismatch":   {"create_personal_todo": {`{"success":true,"result":{"taskId":"task-1"}}`}, "get_todo_detail": {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","subject":"wrong"}}}`}},
	}
	for name, responses := range createCases {
		t.Run("create-"+name, func(t *testing.T) {
			if err := runTodoCoverage(t, Create, &todoCoverageCaller{responses: responses}, "--title", "x", "--executors", "u", "--yes"); err == nil {
				t.Fatal("bad create path accepted")
			}
		})
	}
	createSuccess := &todoCoverageCaller{responses: map[string][]string{
		"create_personal_todo": {`{"success":true,"result":{"taskId":"task-1"}}`},
		"get_todo_detail":      {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","subject":"x"}}}`},
	}}
	if err := runTodoCoverage(t, Create, createSuccess, "--title", "x", "--executors", "u", "--yes"); err != nil {
		t.Fatal(err)
	}
	createReadFailure := &todoCoverageCaller{responses: map[string][]string{
		"create_personal_todo": {`{"success":true,"result":{"taskId":"task-1"}}`},
		"get_todo_detail":      {`{"success":true,"result":{}}`},
	}}
	err := runTodoCoverage(t, Create, createReadFailure, "--title", "x", "--executors", "u", "--yes")
	requireTodoExecutionStartedError(t, err, "missing_detail")

	updateCases := map[string]map[string][]string{
		"call":       {"update_todo_task": {"__ERROR__"}},
		"receipt":    {"update_todo_task": {`{"result":{}}`}},
		"read-call":  {"update_todo_task": {`{"success":true}`}, "get_todo_detail": {"__ERROR__"}},
		"read-shape": {"update_todo_task": {`{"success":true}`}, "get_todo_detail": {`{"success":true,"result":{}}`}},
		"mismatch":   {"update_todo_task": {`{"success":true}`}, "get_todo_detail": {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","subject":"wrong"}}}`}},
	}
	for name, responses := range updateCases {
		t.Run("update-"+name, func(t *testing.T) {
			if err := runTodoCoverage(t, Update, &todoCoverageCaller{responses: responses}, "--task-id", "task-1", "--title", "x", "--yes"); err == nil {
				t.Fatal("bad update path accepted")
			}
		})
	}
	updateSuccess := &todoCoverageCaller{responses: map[string][]string{
		"update_todo_task": {`{"success":true}`},
		"get_todo_detail":  {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","subject":"x","priority":40,"dueTime":1786932000000}}}`},
	}}
	if err := runTodoCoverage(t, Update, updateSuccess, "--task-id", "task-1", "--title", "x", "--priority", "40", "--due", todoCoverageTime, "--yes"); err != nil {
		t.Fatal(err)
	}
	if todoUpdateFieldMatches("dueTime", float64(1786932000000), int64(1786932000001)) {
		t.Fatal("different dueTime milliseconds matched")
	}
	for _, tc := range []struct {
		value any
		want  int64
		ok    bool
	}{
		{value: int(40), want: 40, ok: true},
		{value: int64(1786932000000), want: 1786932000000, ok: true},
		{value: float64(40), want: 40, ok: true},
		{value: json.Number("40"), want: 40, ok: true},
		{value: " 40 ", want: 40, ok: true},
		{value: float64(math.MinInt64), want: math.MinInt64, ok: true},
		{value: 1786932000000.5},
		{value: math.NaN()},
		{value: math.Inf(1)},
		{value: math.Inf(-1)},
		{value: float64(math.MaxInt64)},
		{value: 1e300},
		{value: -1e300},
		{value: json.Number("not-a-number")},
		{value: "not-a-number"},
		{value: true},
	} {
		got, ok := todoExactInteger(tc.value)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("todoExactInteger(%#v) = %d/%v, want %d/%v", tc.value, got, ok, tc.want, tc.ok)
		}
	}
	if todoUpdateFieldMatches("unknown", 1, 1) {
		t.Fatal("unknown update field matched")
	}
}

func TestCrossPlatformCoverageTodoDoneBranches(t *testing.T) {
	if err := runTodoCoverage(t, Complete, &todoCoverageCaller{responses: map[string][]string{}}, "--task-id", "task-1", "--dry-run", "--yes"); err != nil {
		t.Fatal(err)
	}
	for name, responses := range map[string]map[string][]string{
		"before-call":   {"get_todo_detail": {"__ERROR__"}},
		"before-shape":  {"get_todo_detail": {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1"}}}`}},
		"write-call":    {"get_todo_detail": {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":false}}}`}, "update_todo_done_status": {"__ERROR__"}},
		"receipt":       {"get_todo_detail": {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":false}}}`}, "update_todo_done_status": {`{"result":{}}`}},
		"read-call":     {"get_todo_detail": {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":false}}}`, "__ERROR__"}, "update_todo_done_status": {`{"success":true}`}},
		"read-mismatch": {"get_todo_detail": {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":false}}}`, `{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":false}}}`}, "update_todo_done_status": {`{"success":true}`}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := runTodoCoverage(t, Complete, &todoCoverageCaller{responses: responses}, "--task-id", "task-1", "--yes"); err == nil {
				t.Fatal("bad done path accepted")
			}
		})
	}
	already := &todoCoverageCaller{responses: map[string][]string{"get_todo_detail": {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":true}}}`}}}
	if err := runTodoCoverage(t, Complete, already, "--task-id", "task-1", "--yes"); err != nil {
		t.Fatal(err)
	}
	success := &todoCoverageCaller{responses: map[string][]string{
		"get_todo_detail":         {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":false}}}`, `{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":true}}}`},
		"update_todo_done_status": {`{"success":true}`},
	}}
	if err := runTodoCoverage(t, Complete, success, "--task-id", "task-1", "--yes"); err != nil {
		t.Fatal(err)
	}
	if reopen := doneShortcut("+reopen-test", false); !strings.Contains(reopen.Description, "重新") {
		t.Fatalf("reopen description=%q", reopen.Description)
	}
}

func TestCrossPlatformCoverageTodoSearchAndPagination(t *testing.T) {
	searchNoQuery := Search
	for index := range searchNoQuery.Flags {
		if searchNoQuery.Flags[index].Name == "query" {
			searchNoQuery.Flags[index].Required = false
		}
	}
	if err := Search.Execute(todoRuntimeForTest(t, searchNoQuery, map[string]string{})); err == nil {
		t.Fatal("empty search accepted")
	}
	if err := runTodoCoverage(t, Search, &todoCoverageCaller{responses: map[string][]string{}}, "--query", "x", "--max-pages", "0"); err == nil {
		t.Fatal("bad max-pages accepted")
	}
	for name, response := range map[string]string{
		"call":       "__ERROR__",
		"collection": `{"success":true,"result":{"hasMore":false}}`,
		"pagination": `{"success":true,"result":{"todoCards":[]}}`,
		"subject":    `{"success":true,"result":{"todoCards":[{"taskId":"task-1"}],"hasMore":false}}`,
	} {
		t.Run(name, func(t *testing.T) {
			caller := &todoCoverageCaller{responses: map[string][]string{"get_user_todos_in_current_org": {response}}}
			if err := runTodoCoverage(t, Search, caller, "--query", "x", "--status", "false", "--max-pages", "1"); err == nil {
				t.Fatal("bad search page accepted")
			}
		})
	}
	limit := &todoCoverageCaller{responses: map[string][]string{"get_user_todos_in_current_org": {`{"success":true,"result":{"todoCards":[],"hasMore":true}}`}}}
	if err := runTodoCoverage(t, Search, limit, "--query", "x", "--max-pages", "1"); err == nil {
		t.Fatal("incomplete search accepted")
	}
	success := &todoCoverageCaller{responses: map[string][]string{"get_user_todos_in_current_org": {
		`{"success":true,"result":{"todoCards":[{"taskId":"task-1","subject":"Needle"}],"hasMore":true}}`,
		`{"success":true,"result":{"todoCards":[{"taskId":"task-2","subject":"other"}],"hasMore":false}}`,
	}}}
	if err := runTodoCoverage(t, Search, success, "--query", "needle", "--max-pages", "2"); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageTodoCommentAndReminder(t *testing.T) {
	commentNoContent := Comment
	for index := range commentNoContent.Flags {
		if commentNoContent.Flags[index].Name == "content" {
			commentNoContent.Flags[index].Required = false
		}
	}
	if err := Comment.Execute(todoRuntimeForTest(t, commentNoContent, map[string]string{"task-id": "task-1"})); err == nil {
		t.Fatal("empty comment accepted")
	}
	if err := runTodoCoverage(t, Comment, &todoCoverageCaller{responses: map[string][]string{}}, "--task-id", "task-1", "--content", "body", "--dry-run", "--yes"); err != nil {
		t.Fatal(err)
	}
	emptyPage := `{"success":true,"result":{"comments":[],"hasMore":false}}`
	for name, responses := range map[string]map[string][]string{
		"before-call":       {"list_todo_comment": {"__ERROR__"}},
		"before-shape":      {"list_todo_comment": {`{"success":true,"result":{}}`}},
		"before-pagination": {"list_todo_comment": {`{"success":true,"result":{"comments":[]}}`}},
		"write-call":        {"list_todo_comment": {emptyPage}, "add_todo_comment": {"__ERROR__"}},
		"receipt":           {"list_todo_comment": {emptyPage}, "add_todo_comment": {`{"result":{}}`}},
		"after-call":        {"list_todo_comment": {emptyPage, "__ERROR__"}, "add_todo_comment": {`{"success":true}`}},
		"ambiguous":         {"list_todo_comment": {emptyPage, emptyPage}, "add_todo_comment": {`{"success":true}`}},
	} {
		t.Run("comment-"+name, func(t *testing.T) {
			if err := runTodoCoverage(t, Comment, &todoCoverageCaller{responses: responses}, "--task-id", "task-1", "--content", "body", "--yes"); err == nil {
				t.Fatal("bad comment path accepted")
			}
		})
	}
	commentSuccess := &todoCoverageCaller{responses: map[string][]string{
		"list_todo_comment": {`{"success":true,"result":{"comments":[{"commentId":"old","content":"old"}],"hasMore":false}}`, `{"success":true,"result":{"comments":[{"commentId":"old","content":"old"},{"commentId":"new","content":"body"}],"hasMore":false}}`},
		"add_todo_comment":  {`{"success":true,"result":{}}`},
	}}
	if err := runTodoCoverage(t, Comment, commentSuccess, "--task-id", "task-1", "--content", "body", "--yes"); err != nil {
		t.Fatal(err)
	}
	commentReadFailure := &todoCoverageCaller{responses: map[string][]string{
		"list_todo_comment": {emptyPage, `{"success":true,"result":{}}`},
		"add_todo_comment":  {`{"success":true}`},
	}}
	err := runTodoCoverage(t, Comment, commentReadFailure, "--task-id", "task-1", "--content", "body", "--yes")
	requireTodoExecutionStartedError(t, err, "missing_collection")

	for name, args := range map[string][]string{
		"none":         {"--task-id", "task-1", "--yes"},
		"both":         {"--task-id", "task-1", "--clear", "--base-time", "dueTime", "--yes"},
		"due-offset":   {"--task-id", "task-1", "--base-time", "dueTime", "--yes"},
		"custom-at":    {"--task-id", "task-1", "--base-time", "customTime", "--yes"},
		"custom-value": {"--task-id", "task-1", "--base-time", "customTime", "--at", "bad", "--yes"},
	} {
		t.Run("reminder-"+name, func(t *testing.T) {
			if err := runTodoCoverage(t, Reminder, &todoCoverageCaller{responses: map[string][]string{}}, args...); err == nil {
				t.Fatal("bad reminder arguments accepted")
			}
		})
	}
	if err := Reminder.Execute(todoRuntimeForTest(t, Reminder, map[string]string{"task-id": "task-1"})); err == nil {
		t.Fatal("direct reminder execution without an action accepted")
	}
	for _, tc := range []struct {
		name       string
		args       []string
		requestKey string
		ignoredKey string
	}{
		{
			name:       "clear-offset",
			args:       []string{"--task-id", "task-1", "--clear", "--due-date-offset", "-30", "--dry-run", "--yes"},
			requestKey: "todoReminderUpdateRequest", ignoredKey: "dueDateOffset",
		},
		{
			name:       "due-at",
			args:       []string{"--task-id", "task-1", "--base-time", "dueTime", "--due-date-offset", "-30", "--at", todoCoverageTime, "--dry-run", "--yes"},
			requestKey: "todoReminderAddRequest", ignoredKey: "reminderTimeStamp",
		},
		{
			name:       "custom-offset",
			args:       []string{"--task-id", "task-1", "--base-time", "customTime", "--at", todoCoverageTime, "--due-date-offset", "-30", "--dry-run", "--yes"},
			requestKey: "todoReminderAddRequest", ignoredKey: "dueDateOffset",
		},
	} {
		t.Run("reminder-compatible-"+tc.name, func(t *testing.T) {
			preview := runTodoUnifiedCoverage(t, Reminder, &todoCoverageCaller{responses: map[string][]string{}}, tc.args...)
			params, ok := preview["params"].(map[string]any)
			if !ok {
				t.Fatalf("preview params = %#v", preview["params"])
			}
			request, ok := params[tc.requestKey].(map[string]any)
			if !ok {
				t.Fatalf("preview request %s = %#v", tc.requestKey, params[tc.requestKey])
			}
			if _, exists := request[tc.ignoredKey]; exists {
				t.Fatalf("historically ignored %s reached request: %#v", tc.ignoredKey, request)
			}
		})
	}
	for name, args := range map[string][]string{
		"clear": {"--task-id", "task-1", "--clear", "--dry-run", "--yes"},
		"due":   {"--task-id", "task-1", "--base-time", "dueTime", "--due-date-offset", "-30", "--dry-run", "--yes"},
		"at":    {"--task-id", "task-1", "--base-time", "customTime", "--at", todoCoverageTime, "--dry-run", "--yes"},
	} {
		t.Run("reminder-dry-"+name, func(t *testing.T) {
			if err := runTodoCoverage(t, Reminder, &todoCoverageCaller{responses: map[string][]string{}}, args...); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, tc := range []struct {
		name string
		tool string
		args []string
	}{
		{"clear", "reset_todo_reminder", []string{"--task-id", "task-1", "--clear", "--yes"}},
		{"due", "add_todo_reminder", []string{"--task-id", "task-1", "--base-time", "dueTime", "--due-date-offset", "-30", "--yes"}},
		{"at", "add_todo_reminder", []string{"--task-id", "task-1", "--base-time", "customTime", "--at", todoCoverageTime, "--yes"}},
	} {
		t.Run("reminder-success-"+tc.name, func(t *testing.T) {
			caller := &todoCoverageCaller{responses: map[string][]string{tc.tool: {`{"success":true}`}}}
			if err := runTodoCoverage(t, Reminder, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
		})
	}
	for name, response := range map[string]string{"call": "__ERROR__", "receipt": `{"result":{}}`} {
		t.Run("reminder-remote-"+name, func(t *testing.T) {
			caller := &todoCoverageCaller{responses: map[string][]string{"reset_todo_reminder": {response}}}
			if err := runTodoCoverage(t, Reminder, caller, "--task-id", "task-1", "--clear", "--yes"); err == nil {
				t.Fatal("bad reminder response accepted")
			}
		})
	}
	pages := make([]string, todoMaxPages)
	for index := range pages {
		pages[index] = `{"success":true,"result":{"comments":[],"hasMore":true}}`
	}
	capCaller := &todoCoverageCaller{responses: map[string][]string{"list_todo_comment": pages}}
	helpers.InitDepsForTest(t, capCaller)
	if _, err := listAllTodoComments(todoRuntimeForTest(t, Comment, map[string]string{"task-id": "task-1", "content": "x"}), "task-1"); err == nil {
		t.Fatal("comment pagination cap accepted incomplete data")
	}
}

func TestCrossPlatformCoverageTodoVerificationHelpers(t *testing.T) {
	type createdCase struct {
		name      string
		response  map[string]any
		read      string
		expected  string
		wantError bool
	}
	for _, tc := range []createdCase{
		{"receipt", map[string]any{}, "", "x", true},
		{"id", map[string]any{"success": true}, "", "x", true},
		{"read", map[string]any{"success": true, "taskId": "task-1"}, "__ERROR__", "x", true},
		{"mismatch", map[string]any{"success": true, "taskId": "task-1"}, `{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","subject":"wrong"}}}`, "x", true},
		{"success", map[string]any{"success": true, "taskId": "task-1"}, `{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","subject":"x"}}}`, "x", false},
	} {
		t.Run("created-"+tc.name, func(t *testing.T) {
			caller := &todoCoverageCaller{responses: map[string][]string{}}
			if tc.read != "" {
				caller.responses["get_todo_detail"] = []string{tc.read}
			}
			helpers.InitDepsForTest(t, caller)
			_, _, err := VerifyCreatedTodo(todoRuntimeForTest(t, Get, map[string]string{"task-id": "task-1"}), tc.response, "todo/create", tc.expected)
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v wantError=%v", err, tc.wantError)
			}
		})
	}
	for name, fixture := range map[string]struct {
		response map[string]any
		read     string
		wantErr  bool
	}{
		"receipt":  {map[string]any{}, "", true},
		"read":     {map[string]any{"success": true}, "__ERROR__", true},
		"mismatch": {map[string]any{"success": true}, `{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":false}}}`, true},
		"success":  {map[string]any{"success": true}, `{"success":true,"result":{"todoDetailModel":{"taskId":"task-1","isDone":true}}}`, false},
	} {
		t.Run("done-"+name, func(t *testing.T) {
			caller := &todoCoverageCaller{responses: map[string][]string{}}
			if fixture.read != "" {
				caller.responses["get_todo_detail"] = []string{fixture.read}
			}
			helpers.InitDepsForTest(t, caller)
			err := VerifyDoneStatus(todoRuntimeForTest(t, Get, map[string]string{"task-id": "task-1"}), fixture.response, "task-1", true)
			if (err != nil) != fixture.wantErr {
				t.Fatalf("error=%v wantError=%v", err, fixture.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageTodoGetMyTasksAndListLeaves(t *testing.T) {
	for name, args := range map[string][]string{
		"page":      {"--page", "0"},
		"size":      {"--size", "21"},
		"priority":  {"--priority", "11"},
		"role":      {"--role-types", "owner"},
		"max":       {"--all", "--max-pages", "0"},
		"max-alone": {"--max-pages", "1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := runTodoCoverage(t, GetMyTasks, &todoCoverageCaller{responses: map[string][]string{}}, args...); err == nil {
				t.Fatal("bad list arguments accepted")
			}
		})
	}
	if err := GetMyTasks.Execute(todoRuntimeForTest(t, GetMyTasks, map[string]string{"max-pages": "1"})); err == nil {
		t.Fatal("direct list execution with --max-pages but without --all accepted")
	}
	for name, response := range map[string]string{
		"call":       "__ERROR__",
		"collection": `{"success":true,"result":{"hasMore":false}}`,
		"pagination": `{"success":true,"result":{"todoCards":[]}}`,
	} {
		t.Run("list-"+name, func(t *testing.T) {
			caller := &todoCoverageCaller{responses: map[string][]string{"get_user_todos_in_current_org": {response}}}
			if err := runTodoCoverage(t, GetMyTasks, caller); err == nil {
				t.Fatal("bad list response accepted")
			}
		})
	}
	normal := &todoCoverageCaller{responses: map[string][]string{"get_user_todos_in_current_org": {`{"success":true,"result":{"todoCards":[{"taskId":"task-1","subject":"x"}],"hasMore":true}}`}}}
	if err := runTodoCoverage(t, GetMyTasks, normal, "--status", "false", "--priority", "40,30", "--role-types", "creator,participant", "--plan-finish-start", "1", "--plan-finish-end", "2"); err != nil {
		t.Fatal(err)
	}
	allFailure := &todoCoverageCaller{responses: map[string][]string{"get_user_todos_in_current_org": {"__ERROR__"}}}
	if err := runTodoCoverage(t, GetMyTasks, allFailure, "--all", "--max-pages", "1"); err == nil {
		t.Fatal("all-pages transport failure accepted")
	}
	allSuccess := &todoCoverageCaller{responses: map[string][]string{"get_user_todos_in_current_org": {`{"success":true,"result":{"todoCards":[],"hasMore":false}}`}}}
	if err := runTodoCoverage(t, GetMyTasks, allSuccess, "--all", "--max-pages", "1", "--role-types", ""); err != nil {
		t.Fatal(err)
	}

	getFailure := &todoCoverageCaller{responses: map[string][]string{"get_todo_detail": {"__ERROR__"}}}
	if err := runTodoCoverage(t, Get, getFailure, "--task-id", "task-1"); err == nil {
		t.Fatal("get transport failure accepted")
	}
	getSuccess := &todoCoverageCaller{responses: map[string][]string{"get_todo_detail": {`{"success":true,"result":{"todoDetailModel":{"taskId":"task-1"}}}`}}}
	if err := runTodoCoverage(t, Get, getSuccess, "--task-id", "task-1"); err != nil {
		t.Fatal(err)
	}

	for name, args := range map[string][]string{"page": {"--task-id", "task-1", "--page", "0"}, "size": {"--task-id", "task-1", "--size", "21"}} {
		t.Run("comments-"+name, func(t *testing.T) {
			if err := runTodoCoverage(t, ListComment, &todoCoverageCaller{responses: map[string][]string{}}, args...); err == nil {
				t.Fatal("bad comment list args accepted")
			}
		})
	}
	for name, response := range map[string]string{"call": "__ERROR__", "shape": `{"success":true,"result":{}}`} {
		t.Run("comments-"+name, func(t *testing.T) {
			caller := &todoCoverageCaller{responses: map[string][]string{"list_todo_comment": {response}}}
			if err := runTodoCoverage(t, ListComment, caller, "--task-id", "task-1"); err == nil {
				t.Fatal("bad comment list response accepted")
			}
		})
	}
	comments := &todoCoverageCaller{responses: map[string][]string{"list_todo_comment": {`{"success":true,"result":{"comments":[{"commentId":"c-1","content":"x"}]}}`}}}
	if err := runTodoCoverage(t, ListComment, comments, "--task-id", "task-1", "--page", "2", "--size", "10"); err != nil {
		t.Fatal(err)
	}
}
