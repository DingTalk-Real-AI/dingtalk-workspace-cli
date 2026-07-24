// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"os"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type wukongWeeklySyncCall struct {
	server string
	tool   string
	args   map[string]any
}

type wukongWeeklySyncCaller struct {
	calls     []wukongWeeklySyncCall
	responses []string
	dryRun    bool
	index     int
}

func (c *wukongWeeklySyncCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, wukongWeeklySyncCall{server: server, tool: tool, args: args})
	response := `{}`
	if c.index < len(c.responses) {
		response = c.responses[c.index]
		c.index++
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: response}}}, nil
}

func (*wukongWeeklySyncCaller) Format() string { return "json" }
func (c *wukongWeeklySyncCaller) DryRun() bool { return c.dryRun }
func (*wukongWeeklySyncCaller) Fields() string { return "" }
func (*wukongWeeklySyncCaller) JQ() string     { return "" }

func executeWukongWeeklySyncCommand(
	t *testing.T,
	product string,
	caller *wukongWeeklySyncCaller,
	build func() *cobra.Command,
	args ...string,
) (string, string, error) {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	defer func() {
		deps = previousDeps
		os.Args = previousArgs
	}()

	os.Args = []string{"dws", product}
	InitDeps(caller)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = &stderr

	root := build()
	root.PersistentFlags().Bool("dry-run", false, "preview without executing")
	root.PersistentFlags().Bool("yes", false, "confirm execution")
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func requireWukongWeeklySyncCall(t *testing.T, caller *wukongWeeklySyncCaller, want wukongWeeklySyncCall) {
	t.Helper()
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func requireWukongWeeklySyncNoCalls(t *testing.T, caller *wukongWeeklySyncCaller) {
	t.Helper()
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want none", caller.calls)
	}
}

func requireWukongWeeklySyncConfirmation(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("unconfirmed destructive command unexpectedly succeeded")
	}
	var appErr *apperrors.Error
	if !stderrors.As(err, &appErr) {
		t.Fatalf("error = %T %v, want structured validation error", err, err)
	}
	if appErr.Category != apperrors.CategoryValidation || appErr.Reason != "confirmation_required" {
		t.Fatalf("error = %#v, want validation/confirmation_required", appErr)
	}
}

func TestCrossPlatformCoverageWukongWeeklyChatCategoryQueries(t *testing.T) {
	caller := &wukongWeeklySyncCaller{}
	_, _, err := executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand, "category", "list-by-conv")
	if err == nil || !strings.Contains(err.Error(), "flag --group is required") {
		t.Fatalf("missing group error = %v", err)
	}
	requireWukongWeeklySyncNoCalls(t, caller)

	root := newChatCommand()
	batchInfo, _, findErr := root.Find([]string{"category", "batch-info"})
	if findErr != nil {
		t.Fatal(findErr)
	}
	if err := batchInfo.RunE(batchInfo, nil); err == nil || !strings.Contains(err.Error(), "--category-ids") {
		t.Fatalf("direct missing category IDs error = %v", err)
	}

	caller = &wukongWeeklySyncCaller{}
	_, _, err = executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand,
		"category", "list-by-conv", "--conversation-id", "cid-1")
	if err != nil {
		t.Fatalf("list-by-conv returned error: %v", err)
	}
	requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
		server: "im",
		tool:   "list_conv_categories_by_conv",
		args:   map[string]any{"openConversationId": "cid-1"},
	})

	for _, tagIDs := range []string{"", "123,bad", ", ,"} {
		caller = &wukongWeeklySyncCaller{}
		args := []string{"category", "batch-info"}
		if tagIDs != "" {
			args = append(args, "--category-ids", tagIDs)
		}
		_, _, err = executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand, args...)
		if err == nil {
			t.Fatalf("batch-info category IDs %q unexpectedly succeeded", tagIDs)
		}
		requireWukongWeeklySyncNoCalls(t, caller)
	}

	caller = &wukongWeeklySyncCaller{}
	_, _, err = executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand,
		"category", "batch-info", "--category-ids", "123, 456")
	if err != nil {
		t.Fatalf("batch-info returned error: %v", err)
	}
	requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
		server: "im",
		tool:   "get_conv_categories_info",
		args:   map[string]any{"categoryIds": []int64{123, 456}},
	})
}

func TestCrossPlatformCoverageWukongWeeklyChatMessageEditValidation(t *testing.T) {
	root := newChatCommand()
	edit, _, findErr := root.Find([]string{"message", "edit"})
	if findErr != nil {
		t.Fatal(findErr)
	}
	if err := edit.Flags().Set("group", "cid-direct"); err != nil {
		t.Fatal(err)
	}
	if err := edit.RunE(edit, nil); err == nil || !strings.Contains(err.Error(), "msg-id") {
		t.Fatalf("direct missing message error = %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing conversation",
			args:    []string{"message", "edit", "--msg-id", "msg-1", "--text", "body"},
			wantErr: "conversation-id",
		},
		{
			name:    "missing message",
			args:    []string{"message", "edit", "--group", "cid-1", "--text", "body"},
			wantErr: "msg-id",
		},
		{
			name:    "missing content",
			args:    []string{"message", "edit", "--group", "cid-1", "--msg-id", "msg-1"},
			wantErr: "--text or --content",
		},
		{
			name: "mutually exclusive content",
			args: []string{
				"message", "edit", "--group", "cid-1", "--msg-id", "msg-1",
				"--text", "body", "--content", `{"title":"t","text":"b"}`,
			},
			wantErr: "mutually exclusive",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &wukongWeeklySyncCaller{}
			_, _, err := executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
			requireWukongWeeklySyncNoCalls(t, caller)
		})
	}
}

func TestCrossPlatformCoverageWukongWeeklyChatMessageEditMappings(t *testing.T) {
	rawContent := `{"title":"raw title","text":"raw body"}`
	caller := &wukongWeeklySyncCaller{}
	_, _, err := executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand,
		"message", "edit",
		"--chat", "cid-raw",
		"--msg-id", "msg-raw",
		"--content", rawContent,
	)
	if err != nil {
		t.Fatalf("raw content edit returned error: %v", err)
	}
	requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
		server: "im",
		tool:   "edit_message",
		args: map[string]any{
			"openConversationId": "cid-raw",
			"openMessageId":      "msg-raw",
			"content":            rawContent,
		},
	})

	caller = &wukongWeeklySyncCaller{}
	_, _, err = executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand,
		"message", "edit",
		"--group", "cid-text",
		"--msg-id", "msg-text",
		"--text", "hello @D1",
		"--at-all",
		"--at-open-dingtalk-ids", "D1,D2",
	)
	if err != nil {
		t.Fatalf("text edit returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want one edit_message call", caller.calls)
	}
	call := caller.calls[0]
	if call.server != "im" || call.tool != "edit_message" ||
		call.args["openConversationId"] != "cid-text" ||
		call.args["openMessageId"] != "msg-text" ||
		call.args["atAll"] != true ||
		!reflect.DeepEqual(call.args["atOpenDingTalkIds"], []string{"D1", "D2"}) {
		t.Fatalf("edit_message identity args = %#v", call)
	}
	var generatedContent map[string]string
	if err := json.Unmarshal([]byte(call.args["content"].(string)), &generatedContent); err != nil {
		t.Fatalf("generated content is invalid JSON: %v", err)
	}
	wantGenerated := map[string]string{
		"title": "hello @D1",
		"text":  "<@all> hello <@D1>",
	}
	if !reflect.DeepEqual(generatedContent, wantGenerated) {
		t.Fatalf("generated content = %#v, want %#v", generatedContent, wantGenerated)
	}

	caller = &wukongWeeklySyncCaller{}
	_, _, err = executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand,
		"message", "edit",
		"--id", "cid-title",
		"--msg-id", "msg-title",
		"--title", "Reviewed title",
		"--text", "<@all> body",
		"--at-all",
	)
	if err != nil {
		t.Fatalf("titled edit returned error: %v", err)
	}
	var titledContent map[string]string
	if err := json.Unmarshal([]byte(caller.calls[0].args["content"].(string)), &titledContent); err != nil {
		t.Fatalf("titled content is invalid JSON: %v", err)
	}
	if titledContent["title"] != "Reviewed title" || titledContent["text"] != "<@all> body" {
		t.Fatalf("titled content = %#v", titledContent)
	}
}

func TestCrossPlatformCoverageWukongWeeklyChatUpdateNickClearSemantics(t *testing.T) {
	root := newChatCommand()
	updateNick, _, findErr := root.Find([]string{"group", "update-nick"})
	if findErr != nil {
		t.Fatal(findErr)
	}
	if err := updateNick.RunE(updateNick, nil); err == nil || !strings.Contains(err.Error(), "--group") {
		t.Fatalf("direct missing group error = %v", err)
	}

	caller := &wukongWeeklySyncCaller{}
	_, _, err := executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand,
		"group", "update-nick")
	if err == nil || !strings.Contains(err.Error(), "group") {
		t.Fatalf("missing group error = %v", err)
	}
	requireWukongWeeklySyncNoCalls(t, caller)

	for _, test := range []struct {
		name string
		args []string
		nick string
	}{
		{name: "set", args: []string{"--nick", "项目昵称"}, nick: "项目昵称"},
		{name: "clear", nick: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &wukongWeeklySyncCaller{}
			args := append([]string{"group", "update-nick", "--group", "cid-1"}, test.args...)
			_, _, err := executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand, args...)
			if err != nil {
				t.Fatalf("update-nick returned error: %v", err)
			}
			requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
				server: "im",
				tool:   "update_group_nick",
				args: map[string]any{
					"openConversationId": "cid-1",
					"nick":               test.nick,
				},
			})
		})
	}
}

func TestCrossPlatformCoverageWukongWeeklyChatUpgradeValidationAndSafety(t *testing.T) {
	root := newChatCommand()
	upgrade, _, findErr := root.Find([]string{"group", "upgrade-to-external"})
	if findErr != nil {
		t.Fatal(findErr)
	}
	if err := upgrade.RunE(upgrade, nil); err == nil || !strings.Contains(err.Error(), "--group") {
		t.Fatalf("direct missing group error = %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing group",
			args:    []string{"group", "upgrade-to-external", "--yes"},
			wantErr: "group",
		},
		{
			name:    "invalid JSON",
			args:    []string{"group", "upgrade-to-external", "--group", "cid-1", "--extension", `{`, "--yes"},
			wantErr: "JSON object",
		},
		{
			name:    "null extension",
			args:    []string{"group", "upgrade-to-external", "--group", "cid-1", "--extension", `null`, "--yes"},
			wantErr: "JSON object",
		},
		{
			name:    "non-string extension value",
			args:    []string{"group", "upgrade-to-external", "--group", "cid-1", "--extension", `{"retries":1}`, "--yes"},
			wantErr: "must be a string",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &wukongWeeklySyncCaller{}
			_, _, err := executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
			requireWukongWeeklySyncNoCalls(t, caller)
		})
	}

	caller := &wukongWeeklySyncCaller{}
	_, _, err := executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand,
		"group", "upgrade-to-external", "--group", "cid-1")
	requireWukongWeeklySyncConfirmation(t, err)
	requireWukongWeeklySyncNoCalls(t, caller)

	caller = &wukongWeeklySyncCaller{dryRun: true}
	stdout, _, err := executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand,
		"group", "upgrade-to-external",
		"--group", "cid-dry",
		"--extension", `{"source":"dws"}`,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("dry-run upgrade returned error: %v", err)
	}
	requireWukongWeeklySyncNoCalls(t, caller)
	if !strings.Contains(stdout, `"tool": "upgrade_group_to_external"`) ||
		!strings.Contains(stdout, `"source": "dws"`) {
		t.Fatalf("dry-run output = %q", stdout)
	}

	caller = &wukongWeeklySyncCaller{}
	_, _, err = executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand,
		"group", "upgrade-to-external", "--group", "cid-confirmed", "--yes")
	if err != nil {
		t.Fatalf("confirmed upgrade returned error: %v", err)
	}
	requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
		server: "im",
		tool:   "upgrade_group_to_external",
		args:   map[string]any{"openConversationId": "cid-confirmed"},
	})
}

func wukongWeeklyTodoCardsResponse(t *testing.T, count, offset int) string {
	t.Helper()
	cards := make([]map[string]any, count)
	for i := range cards {
		cards[i] = map[string]any{"id": offset + i}
	}
	raw, err := json.Marshal(map[string]any{
		"result": map[string]any{"todoCards": cards},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCrossPlatformCoverageWukongWeeklyTodoQueryAllMappings(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		tool string
	}{
		{
			name: "current organization",
			args: []string{"task", "list", "--page", "2", "--size", "20"},
			tool: "get_user_todos_in_current_org",
		},
		{
			name: "all organizations",
			args: []string{"task", "list", "--page", "2", "--size", "20", "--query-all"},
			tool: "get_user_todos",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &wukongWeeklySyncCaller{}
			_, _, err := executeWukongWeeklySyncCommand(t, "todo", caller, newTodoCommand, test.args...)
			if err != nil {
				t.Fatalf("todo list returned error: %v", err)
			}
			requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
				server: "todo",
				tool:   test.tool,
				args: map[string]any{
					"pageNum":  "2",
					"pageSize": "20",
					"roleTypes": []string{
						"executor",
					},
				},
			})
		})
	}

	caller := &wukongWeeklySyncCaller{
		responses: []string{
			wukongWeeklyTodoCardsResponse(t, 20, 0),
			wukongWeeklyTodoCardsResponse(t, 5, 20),
		},
	}
	stdout, _, err := executeWukongWeeklySyncCommand(t, "todo", caller, newTodoCommand,
		"task", "list", "--size", "21", "--query-all")
	if err != nil {
		t.Fatalf("query-all auto-page returned error: %v", err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("auto-page calls = %#v, want two", caller.calls)
	}
	for index, call := range caller.calls {
		if call.server != "todo" || call.tool != "get_user_todos" ||
			call.args["pageNum"] != []string{"1", "2"}[index] ||
			call.args["pageSize"] != "20" {
			t.Fatalf("auto-page call %d = %#v", index, call)
		}
	}
	var output struct {
		Result struct {
			TodoCards []any `json:"todoCards"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("auto-page output is invalid JSON: %v\n%s", err, stdout)
	}
	if len(output.Result.TodoCards) != 21 {
		t.Fatalf("auto-page returned %d cards, want 21", len(output.Result.TodoCards))
	}

	for _, test := range []struct {
		name     string
		queryAll bool
		tool     string
	}{
		{name: "current organization dry-run", tool: "get_user_todos_in_current_org"},
		{name: "all organizations dry-run", queryAll: true, tool: "get_user_todos"},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &wukongWeeklySyncCaller{dryRun: true}
			args := []string{"task", "list", "--size", "41", "--dry-run"}
			if test.queryAll {
				args = append(args, "--query-all")
			}
			stdout, _, err := executeWukongWeeklySyncCommand(t, "todo", caller, newTodoCommand, args...)
			if err != nil {
				t.Fatalf("auto-page dry-run returned error: %v", err)
			}
			requireWukongWeeklySyncNoCalls(t, caller)
			if !strings.Contains(stdout, test.tool) {
				t.Fatalf("auto-page dry-run output = %q, want tool %s", stdout, test.tool)
			}
		})
	}
}

func TestCrossPlatformCoverageWukongWeeklyTodoTagValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "add missing codes",
			args:    []string{"tag", "add", "--task-id", "task-1"},
			wantErr: "--tag-codes",
		},
		{
			name:    "add blank codes",
			args:    []string{"tag", "add", "--task-id", "task-1", "--tag-codes", ", ,"},
			wantErr: "non-empty code",
		},
		{
			name:    "delete missing codes",
			args:    []string{"tag", "delete"},
			wantErr: "--tag-codes",
		},
		{
			name:    "delete blank codes",
			args:    []string{"tag", "delete", "--tag-codes", ", ,"},
			wantErr: "non-empty code",
		},
		{
			name:    "update missing tags",
			args:    []string{"tag", "update"},
			wantErr: "--user-tags",
		},
		{
			name:    "update invalid JSON",
			args:    []string{"tag", "update", "--user-tags", `[`},
			wantErr: "合法的 JSON 数组",
		},
		{
			name:    "update null",
			args:    []string{"tag", "update", "--user-tags", `null`},
			wantErr: "must be a JSON array",
		},
		{
			name:    "create missing name",
			args:    []string{"tag", "create"},
			wantErr: "--name",
		},
		{
			name:    "create blank name",
			args:    []string{"tag", "create", "--name", "   "},
			wantErr: "must not be blank",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &wukongWeeklySyncCaller{}
			_, _, err := executeWukongWeeklySyncCommand(t, "todo", caller, newTodoCommand, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
			requireWukongWeeklySyncNoCalls(t, caller)
		})
	}

	caller := &wukongWeeklySyncCaller{}
	_, _, err := executeWukongWeeklySyncCommand(t, "todo", caller, newTodoCommand,
		"tag", "delete", "--tag-codes", "code1")
	requireWukongWeeklySyncConfirmation(t, err)
	requireWukongWeeklySyncNoCalls(t, caller)
}

func TestCrossPlatformCoverageWukongWeeklyTodoTagMappingsAndSafety(t *testing.T) {
	caller := &wukongWeeklySyncCaller{}
	_, _, err := executeWukongWeeklySyncCommand(t, "todo", caller, newTodoCommand,
		"tag", "add", "--task-id", "task-1", "--tag-codes", "code1, code2")
	if err != nil {
		t.Fatalf("tag add returned error: %v", err)
	}
	requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
		server: "todo",
		tool:   "tag_todo",
		args: map[string]any{
			"TodoTagRequest": map[string]any{
				"taskId":   "task-1",
				"tagCodes": []string{"code1", "code2"},
			},
		},
	})

	caller = &wukongWeeklySyncCaller{dryRun: true}
	stdout, _, err := executeWukongWeeklySyncCommand(t, "todo", caller, newTodoCommand,
		"tag", "delete", "--tag-codes", "code1,code2", "--dry-run")
	if err != nil {
		t.Fatalf("tag delete dry-run returned error: %v", err)
	}
	requireWukongWeeklySyncNoCalls(t, caller)
	if !strings.Contains(stdout, `"tool": "delete_todo_tag"`) ||
		!strings.Contains(stdout, `"code1"`) {
		t.Fatalf("tag delete dry-run output = %q", stdout)
	}

	caller = &wukongWeeklySyncCaller{}
	_, _, err = executeWukongWeeklySyncCommand(t, "todo", caller, newTodoCommand,
		"tag", "delete", "--tag-codes", "code1,code2", "--yes")
	if err != nil {
		t.Fatalf("confirmed tag delete returned error: %v", err)
	}
	requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
		server: "todo",
		tool:   "delete_todo_tag",
		args: map[string]any{
			"UserTagDeleteRequest": map[string]any{
				"tagCodes": []string{"code1", "code2"},
			},
		},
	})

	caller = &wukongWeeklySyncCaller{}
	userTagsJSON := `[{"code":"code1","name":"新名称"}]`
	_, _, err = executeWukongWeeklySyncCommand(t, "todo", caller, newTodoCommand,
		"tag", "update", "--user-tags", userTagsJSON)
	if err != nil {
		t.Fatalf("tag update returned error: %v", err)
	}
	requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
		server: "todo",
		tool:   "update_todo_tag",
		args: map[string]any{
			"UserTagAddRequest": map[string]any{
				"userTags": []any{map[string]any{"code": "code1", "name": "新名称"}},
			},
		},
	})

	caller = &wukongWeeklySyncCaller{}
	_, _, err = executeWukongWeeklySyncCommand(t, "todo", caller, newTodoCommand, "tag", "list")
	if err != nil {
		t.Fatalf("tag list returned error: %v", err)
	}
	requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
		server: "todo",
		tool:   "list_todo_tags",
		args:   map[string]any{},
	})

	caller = &wukongWeeklySyncCaller{}
	_, _, err = executeWukongWeeklySyncCommand(t, "todo", caller, newTodoCommand,
		"tag", "create", "--name", "  项目标签  ")
	if err != nil {
		t.Fatalf("tag create returned error: %v", err)
	}
	requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
		server: "todo",
		tool:   "create_todo_tag",
		args: map[string]any{
			"UserTagAddRequest": map[string]any{
				"userTags": []map[string]any{{"name": "项目标签"}},
			},
		},
	})
}
