// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type guardedMutationCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type guardedMutationCaller struct {
	calls  []guardedMutationCall
	dryRun bool
}

func (c *guardedMutationCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, guardedMutationCall{productID: productID, toolName: toolName, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*guardedMutationCaller) Format() string { return "json" }
func (c *guardedMutationCaller) DryRun() bool { return c.dryRun }
func (*guardedMutationCaller) Fields() string { return "" }
func (*guardedMutationCaller) JQ() string     { return "" }

func executeGuardedMutationCommand(t *testing.T, caller *guardedMutationCaller, build func() *cobra.Command, args ...string) error {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	InitDeps(caller)
	deps.Out.w = io.Discard
	root := build()
	if root.PersistentFlags().Lookup("yes") == nil {
		root.PersistentFlags().Bool("yes", false, "confirm high-risk operation")
	}
	if root.PersistentFlags().Lookup("dry-run") == nil {
		root.PersistentFlags().Bool("dry-run", false, "preview without executing")
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	// Closed stdin makes ConfirmSafety return typed confirmation_required
	// instead of hanging on an interactive prompt — but do not clobber a
	// fixture that already SetIn("no\n") / similar decline input.
	if root.InOrStdin() == os.Stdin {
		root.SetIn(strings.NewReader(""))
	}
	root.SetArgs(args)
	return root.Execute()
}

func requireTypedConfirmationError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("command without --yes unexpectedly succeeded")
	}
	var appErr *apperrors.Error
	if !stderrors.As(err, &appErr) || appErr.Category != apperrors.CategoryValidation {
		t.Fatalf("confirmation error = %T %v, want typed validation error", err, err)
	}
	if appErr.Reason != "confirmation_required" {
		t.Fatalf("confirmation reason = %q, want confirmation_required", appErr.Reason)
	}
}

func TestChatChmodRequiresConfirmationBeforeRemoteCall(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"chmod", "chat.message:send",
		"--agentCode", "wukong",
		"--grant-type", "timed",
		"--ttl", "24h",
		"--conversation-id", "cidX",
	)
	requireTypedConfirmationError(t, err)
	if len(caller.calls) != 0 {
		t.Fatalf("expected 0 MCP calls before --yes, got %d: %+v", len(caller.calls), caller.calls)
	}
}

func TestChatChmodCallsRemoteWithExactArgsWhenConfirmed(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"chmod", "chat.message:send",
		"--agentCode", "wukong",
		"--grant-type", "timed",
		"--ttl", "24h",
		"--conversation-id", "cidX",
		"--yes",
	)
	if err != nil {
		t.Fatalf("chat chmod with --yes returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("expected exactly 1 MCP call, got %d: %+v", len(caller.calls), caller.calls)
	}
	call := caller.calls[0]
	if call.productID != "im" || call.toolName != "chat_permission_grant" {
		t.Fatalf("tool call = %s/%s, want im/chat_permission_grant", call.productID, call.toolName)
	}
	wantArgs := map[string]any{
		"agentCode":   "wukong",
		"scope":       "chat.message:send",
		"grantType":   "timed",
		"ttl":         "24h",
		"grantParams": `{"conversationId":"cidX","openCid":"cidX","openConversationId":"cidX"}`,
	}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("tool args = %#v, want %#v", call.args, wantArgs)
	}
}

type guardedStepCaller struct {
	calls  []guardedMutationCall
	steps  []string
	index  int
	dryRun bool
}

func (c *guardedStepCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, guardedMutationCall{productID: productID, toolName: toolName, args: args})
	text := `{}`
	if c.index < len(c.steps) {
		text = c.steps[c.index]
	}
	c.index++
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*guardedStepCaller) Format() string { return "json" }
func (c *guardedStepCaller) DryRun() bool { return c.dryRun }
func (*guardedStepCaller) Fields() string { return "" }
func (*guardedStepCaller) JQ() string     { return "" }

func executeGuardedMailMutationCommand(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	testseam.Protect(t, &os.Args)
	os.Args = append([]string{"dws", "mail"}, args...)
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	InitDeps(caller)
	deps.Out.w = io.Discard
	root := newMailCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	if root.InOrStdin() == os.Stdin {
		root.SetIn(strings.NewReader(""))
	}
	root.SetArgs(args)
	return root.Execute()
}

func TestMailMessageShareToChatRequiresConfirmationBeforeToolCall(t *testing.T) {
	baseArgs := []string{
		"message", "share-to-chat",
		"--email", "user@company.com",
		"--id", "msg-1",
		"--users", "uid1",
	}
	caller := &guardedMutationCaller{}
	err := executeGuardedMailMutationCommand(t, caller, baseArgs...)
	requireTypedConfirmationError(t, err)
	if len(caller.calls) != 0 {
		t.Fatalf("tool calls = %#v, want none before confirmation", caller.calls)
	}

	stepCaller := &guardedStepCaller{steps: []string{
		`{"result":{"sign":"sig","riskMessage":"careful"}}`,
		`{"ok":true}`,
	}}
	err = executeGuardedMailMutationCommand(t, stepCaller, append(append([]string(nil), baseArgs...), "--yes")...)
	if err != nil {
		t.Fatalf("confirmed share with sign returned error: %v", err)
	}
	if len(stepCaller.calls) != 2 {
		t.Fatalf("tool calls = %d, want 2 for sign retry", len(stepCaller.calls))
	}
	if stepCaller.calls[0].toolName != "share_message_to_chat" || stepCaller.calls[1].toolName != "share_message_to_chat" {
		t.Fatalf("tool calls = %#v, want share_message_to_chat twice", stepCaller.calls)
	}
	if sign, _ := stepCaller.calls[1].args["sign"].(string); sign != "sig" {
		t.Fatalf("second call sign = %q, want sig", sign)
	}

	directCaller := &guardedStepCaller{steps: []string{`{"ok":true}`}}
	err = executeGuardedMailMutationCommand(t, directCaller, append(append([]string(nil), baseArgs...), "--yes")...)
	if err != nil {
		t.Fatalf("confirmed direct success returned error: %v", err)
	}
	if len(directCaller.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1 for direct success", len(directCaller.calls))
	}
	if directCaller.calls[0].toolName != "share_message_to_chat" {
		t.Fatalf("tool call = %#v, want share_message_to_chat", directCaller.calls[0])
	}

	dryRunCaller := &guardedMutationCaller{dryRun: true}
	err = executeGuardedMailMutationCommand(t, dryRunCaller, baseArgs...)
	if err != nil {
		t.Fatalf("dry-run without --yes returned error: %v", err)
	}
	if len(dryRunCaller.calls) != 0 {
		t.Fatalf("dry-run tool calls = %#v, want none", dryRunCaller.calls)
	}
}

func TestChatDismissGroupRequiresConfirmationBeforeToolCall(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "dismiss", "--group", "conversation-1")
	requireTypedConfirmationError(t, err)
	if len(caller.calls) != 0 {
		t.Fatalf("tool calls = %#v, want none before confirmation", caller.calls)
	}

	caller = &guardedMutationCaller{}
	err = executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "dismiss", "--group", "conversation-1", "--yes")
	if err != nil {
		t.Fatalf("confirmed dismiss returned error: %v", err)
	}
	want := guardedMutationCall{
		productID: "im",
		toolName:  "dismiss_group",
		args:      map[string]any{"openConversationId": "conversation-1"},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("tool calls = %#v, want %#v", caller.calls, want)
	}
}

func TestSheetClearRangeRequiresConfirmationBeforeToolCall(t *testing.T) {
	args := []string{
		"range", "clear",
		"--node", "node-1",
		"--sheet-id", "sheet-1",
		"--range", "A1:B3",
		"--type", "all",
	}
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newSheetCommand, args...)
	requireTypedConfirmationError(t, err)
	if len(caller.calls) != 0 {
		t.Fatalf("tool calls = %#v, want none before confirmation", caller.calls)
	}

	caller = &guardedMutationCaller{}
	err = executeGuardedMutationCommand(t, caller, newSheetCommand, append(args, "--yes")...)
	if err != nil {
		t.Fatalf("confirmed clear returned error: %v", err)
	}
	want := guardedMutationCall{
		productID: "",
		toolName:  "clear_range",
		args: map[string]any{
			"nodeId":  "node-1",
			"sheetId": "sheet-1",
			"range":   "A1:B3",
			"type":    "all",
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("tool calls = %#v, want %#v", caller.calls, want)
	}

	caller = &guardedMutationCaller{dryRun: true}
	err = executeGuardedMutationCommand(t, caller, newSheetCommand, append(args, "--dry-run")...)
	if err != nil {
		t.Fatalf("clear dry-run without --yes returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run tool calls = %#v, want none", caller.calls)
	}
}

func TestSheetBatchClearRequiresConfirmationBeforeToolCall(t *testing.T) {
	args := []string{
		"range", "batch-clear",
		"--node", "node-1",
		"--ranges", `["Sheet1!A1:B3","Sheet2!C1:D5"]`,
		"--type", "all",
	}
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newSheetCommand, args...)
	requireTypedConfirmationError(t, err)
	if len(caller.calls) != 0 {
		t.Fatalf("tool calls = %#v, want none before confirmation", caller.calls)
	}

	caller = &guardedMutationCaller{}
	err = executeGuardedMutationCommand(t, caller, newSheetCommand, append(args, "--yes")...)
	if err != nil {
		t.Fatalf("confirmed batch-clear returned error: %v", err)
	}
	want := guardedMutationCall{
		productID: "",
		toolName:  "batch_update",
		args: map[string]any{
			"nodeId": "node-1",
			"operationsJson": mustBatchOperationsJSON(t, []any{
				map[string]any{"toolName": "clear_range", "input": map[string]any{"sheetId": "Sheet1", "range": "A1:B3", "type": "all"}},
				map[string]any{"toolName": "clear_range", "input": map[string]any{"sheetId": "Sheet2", "range": "C1:D5", "type": "all"}},
			}),
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("tool calls = %#v, want %#v", caller.calls, want)
	}

	caller = &guardedMutationCaller{dryRun: true}
	err = executeGuardedMutationCommand(t, caller, newSheetCommand, append(args, "--dry-run")...)
	if err != nil {
		t.Fatalf("batch-clear dry-run without --yes returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run tool calls = %#v, want none", caller.calls)
	}
}

func TestSheetBatchUpdateRequiresConfirmationBeforeToolCall(t *testing.T) {
	operations := `[{"toolName":"range clear","input":{"sheet-id":"Sheet1","range":"A1:B3","type":"content"}},{"toolName":"delete-dimension","input":{"sheet-id":"Sheet1","dimension":"ROWS","position":2,"length":3}}]`
	args := []string{"batch-update", "--node", "node-1", "--operations", operations}
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newSheetCommand, args...)
	requireTypedConfirmationError(t, err)
	if len(caller.calls) != 0 {
		t.Fatalf("tool calls = %#v, want none before confirmation", caller.calls)
	}

	caller = &guardedMutationCaller{}
	err = executeGuardedMutationCommand(t, caller, newSheetCommand, append(args, "--yes")...)
	if err != nil {
		t.Fatalf("confirmed batch-update returned error: %v", err)
	}
	want := guardedMutationCall{
		productID: "",
		toolName:  "batch_update",
		args: map[string]any{
			"nodeId": "node-1",
			"operationsJson": mustBatchOperationsJSON(t, []any{
				map[string]any{"toolName": "clear_range", "input": map[string]any{"sheetId": "Sheet1", "range": "A1:B3", "type": "content"}},
				map[string]any{"toolName": "delete_dimension", "input": map[string]any{"sheetId": "Sheet1", "dimension": "ROWS", "startIndex": 1, "count": 3}},
			}),
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("tool calls = %#v, want %#v", caller.calls, want)
	}

	caller = &guardedMutationCaller{dryRun: true}
	err = executeGuardedMutationCommand(t, caller, newSheetCommand, append(args, "--dry-run")...)
	if err != nil {
		t.Fatalf("batch-update dry-run without --yes returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run tool calls = %#v, want none", caller.calls)
	}
}

func TestSheetConfirmationGuardCoversEveryProtectedLeaf(t *testing.T) {
	tests := []struct {
		path string
		args []string
	}{
		{
			path: "sheet batch-update",
			args: []string{"batch-update", "--node", "node-1", "--operations", `[{"toolName":"range clear","input":{"sheet-id":"Sheet1","range":"A1:B3"}}]`},
		},
		{
			path: "sheet chart delete",
			args: []string{"chart", "delete", "--node", "node-1", "--sheet-id", "sheet-1", "--chart-id", "chart-1"},
		},
		{
			path: "sheet range clear",
			args: []string{"range", "clear", "--node", "node-1", "--sheet-id", "sheet-1", "--range", "A1:B3"},
		},
		{
			path: "sheet cond-format delete",
			args: []string{"cond-format", "delete", "--node", "node-1", "--sheet-id", "sheet-1", "--rule-id", "rule-1"},
		},
		{
			path: "sheet delete-dimension",
			args: []string{"delete-dimension", "--node", "node-1", "--sheet-id", "sheet-1", "--dimension", "ROWS", "--position", "1", "--length", "1"},
		},
		{
			path: "sheet delete-dropdown",
			args: []string{"delete-dropdown", "--node", "node-1", "--sheet-id", "sheet-1", "--range", "A1:A2"},
		},
		{
			path: "sheet filter delete",
			args: []string{"filter", "delete", "--node", "node-1", "--sheet-id", "sheet-1"},
		},
		{
			path: "sheet filter-view delete",
			args: []string{"filter-view", "delete", "--node", "node-1", "--sheet-id", "sheet-1", "--filter-view-id", "view-1"},
		},
		{
			path: "sheet delete-float-image",
			args: []string{"delete-float-image", "--node", "node-1", "--sheet-id", "sheet-1", "--float-image-id", "image-1"},
		},
		{
			path: "sheet pivot-table delete",
			args: []string{"pivot-table", "delete", "--node", "node-1", "--sheet-id", "sheet-1", "--pivot-table-id", "pivot-1"},
		},
		{
			path: "sheet delete-sheet",
			args: []string{"delete-sheet", "--node", "node-1", "--sheet-id", "sheet-1"},
		},
		{
			path: "sheet filter-view delete-criteria",
			args: []string{"filter-view", "delete-criteria", "--node", "node-1", "--sheet-id", "sheet-1", "--filter-view-id", "view-1", "--column", "0"},
		},
		{
			path: "sheet range batch-clear",
			args: []string{"range", "batch-clear", "--node", "node-1", "--ranges", `["Sheet1!A1:B3"]`},
		},
		{
			path: "sheet range move-to",
			args: []string{"range", "move-to", "--node", "node-1", "--sheet-id", "sheet-1", "--source-range", "A1:B3", "--target-range", "D1"},
		},
		{
			path: "sheet comment delete",
			args: []string{"comment", "delete", "--node", "node-1", "--comment-key", "ck-1"},
		},
		{
			path: "sheet version revert",
			args: []string{"version", "revert", "--node", "node-1", "--version", "2"},
		},
	}

	// A newly protected command must add a runnable case here. This keeps the
	// runtime assertion coupled to the structural wrapper instead of a stale
	// hand-maintained subset.
	root := newSheetCommand()
	guardedPaths := make(map[string]bool)
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if HasSheetMutationConfirmationGuard(command) {
			guardedPaths[command.CommandPath()] = true
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
	testPaths := make(map[string]bool, len(tests))
	for _, test := range tests {
		testPaths[test.path] = true
	}
	var missingCases, staleCases []string
	for path := range guardedPaths {
		if !testPaths[path] {
			missingCases = append(missingCases, path)
		}
	}
	for path := range testPaths {
		if !guardedPaths[path] {
			staleCases = append(staleCases, path)
		}
	}
	if len(missingCases) != 0 || len(staleCases) != 0 {
		sort.Strings(missingCases)
		sort.Strings(staleCases)
		t.Fatalf("Sheet confirmation runtime cases differ from protected leaves: missing_cases=%v stale_cases=%v", missingCases, staleCases)
	}

	for _, test := range tests {
		test := test
		t.Run(strings.TrimPrefix(test.path, "sheet "), func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newSheetCommand, test.args...)
			requireTypedConfirmationError(t, err)
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls before confirmation = %#v, want none", caller.calls)
			}

			caller = &guardedMutationCaller{dryRun: true}
			dryRunArgs := append(append([]string(nil), test.args...), "--dry-run")
			if err := executeGuardedMutationCommand(t, caller, newSheetCommand, dryRunArgs...); err != nil {
				t.Fatalf("dry-run without --yes returned error: %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("dry-run tool calls = %#v, want none", caller.calls)
			}
		})
	}
}

// executeGuardedPermissionRemoveCommand mirrors executeGuardedMutationCommand
// but additionally rewrites os.Args so resolveProductID routes doc/wiki
// auto-routed tools to their real MCP server (doc → doc, wiki → wiki).
func executeGuardedPermissionRemoveCommand(t *testing.T, caller *guardedMutationCaller, build func() *cobra.Command, args ...string) error {
	t.Helper()
	testseam.Protect(t, &os.Args)
	root := build()
	os.Args = append([]string{"dws", root.Name()}, args...)
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	InitDeps(caller)
	deps.Out.w = io.Discard
	if root.PersistentFlags().Lookup("yes") == nil {
		root.PersistentFlags().Bool("yes", false, "confirm high-risk operation")
	}
	if root.PersistentFlags().Lookup("dry-run") == nil {
		root.PersistentFlags().Bool("dry-run", false, "preview without executing")
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	if root.InOrStdin() == os.Stdin {
		root.SetIn(strings.NewReader(""))
	}
	root.SetArgs(args)
	return root.Execute()
}

// TestPermissionMemberRemoveRequiresConfirmationBeforeToolCall pins the
// destructive confirmation gate on the three batch remove entry points
// (drive/doc permission remove, wiki member remove). Removing up to 30
// USER/DEPT/CONVERSATION/TAG members in one call can revoke access for whole
// departments, chats, or role groups, so every invocation — new --members
// format and legacy --users alike — must confirm first: without --yes the
// command fails with the typed confirmation_required error and performs zero
// MCP calls; with --yes it dispatches exactly one call with the complete,
// precise tool arguments; --dry-run previews without any call.
func TestPermissionMemberRemoveRequiresConfirmationBeforeToolCall(t *testing.T) {
	cases := []struct {
		name     string
		build    func() *cobra.Command
		args     []string
		product  string
		tool     string
		wantArgs map[string]any
	}{
		{
			name:    "drive permission remove --members",
			build:   newDriveCommand,
			args:    []string{"permission", "remove", "--node", "n1", "--members", `[{"type":"CONVERSATION","id":"cid1"},{"type":"TAG","id":"t1","corpId":"c1"}]`},
			product: "doc",
			tool:    "remove_permission",
			wantArgs: map[string]any{
				"nodeId": "n1",
				"members": []map[string]any{
					{"type": "CONVERSATION", "id": "cid1"},
					{"type": "TAG", "id": "t1", "corpId": "c1"},
				},
			},
		},
		{
			name:    "drive permission remove --users",
			build:   newDriveCommand,
			args:    []string{"permission", "remove", "--node", "n1", "--users", "uid1,uid2"},
			product: "doc",
			tool:    "remove_permission",
			wantArgs: map[string]any{
				"nodeId":  "n1",
				"userIds": []string{"uid1", "uid2"},
			},
		},
		{
			name:    "doc permission remove --members",
			build:   newDocCommand,
			args:    []string{"permission", "remove", "--node", "n1", "--members", `[{"type":"USER","id":"u1","corpId":"c1"}]`},
			product: "doc",
			tool:    "remove_permission",
			wantArgs: map[string]any{
				"nodeId":  "n1",
				"members": []map[string]any{{"type": "USER", "id": "u1", "corpId": "c1"}},
			},
		},
		{
			name:    "wiki member remove --members",
			build:   newWikiCommand,
			args:    []string{"member", "remove", "--workspace", "ws1", "--members", `[{"type":"DEPT","id":"d1","corpId":"c1"}]`},
			product: "wiki",
			tool:    "remove_member",
			wantArgs: map[string]any{
				"workspaceId": "ws1",
				"members":     []map[string]any{{"type": "DEPT", "id": "d1", "corpId": "c1"}},
			},
		},
		{
			name:    "wiki member remove --users",
			build:   newWikiCommand,
			args:    []string{"member", "remove", "--workspace", "ws1", "--users", "uid1"},
			product: "wiki",
			tool:    "remove_member",
			wantArgs: map[string]any{
				"workspaceId": "ws1",
				"userIds":     []string{"uid1"},
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// 1) 未确认：typed confirmation_required 错误 + 零 MCP 调用。
			caller := &guardedMutationCaller{}
			err := executeGuardedPermissionRemoveCommand(t, caller, tc.build, tc.args...)
			requireTypedConfirmationError(t, err)
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls before confirmation = %#v, want none", caller.calls)
			}

			// 2) 明确确认（--yes）：恰好一次调用，参数完整且精确。
			confirmed := &guardedMutationCaller{}
			if err := executeGuardedPermissionRemoveCommand(t, confirmed, tc.build, append(append([]string(nil), tc.args...), "--yes")...); err != nil {
				t.Fatalf("confirmed remove returned error: %v", err)
			}
			if len(confirmed.calls) != 1 {
				t.Fatalf("tool calls = %d, want exactly 1: %+v", len(confirmed.calls), confirmed.calls)
			}
			call := confirmed.calls[0]
			if call.productID != tc.product || call.toolName != tc.tool {
				t.Fatalf("tool call = %s/%s, want %s/%s", call.productID, call.toolName, tc.product, tc.tool)
			}
			if !reflect.DeepEqual(call.args, tc.wantArgs) {
				t.Fatalf("tool args = %#v, want %#v", call.args, tc.wantArgs)
			}

			// 3) --dry-run 预览：不触发任何 MCP 调用。
			dryRun := &guardedMutationCaller{dryRun: true}
			if err := executeGuardedPermissionRemoveCommand(t, dryRun, tc.build, append(append([]string(nil), tc.args...), "--dry-run")...); err != nil {
				t.Fatalf("dry-run remove returned error: %v", err)
			}
			if len(dryRun.calls) != 0 {
				t.Fatalf("dry-run tool calls = %#v, want none", dryRun.calls)
			}
		})
	}
}
