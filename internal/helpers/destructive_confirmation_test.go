// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	stderrors "errors"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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
			"operations": []any{
				map[string]any{"toolName": "clear_range", "input": map[string]any{"sheetId": "Sheet1", "range": "A1:B3", "type": "all"}},
				map[string]any{"toolName": "clear_range", "input": map[string]any{"sheetId": "Sheet2", "range": "C1:D5", "type": "all"}},
			},
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
			"operations": []any{
				map[string]any{"toolName": "clear_range", "input": map[string]any{"sheetId": "Sheet1", "range": "A1:B3", "type": "content"}},
				map[string]any{"toolName": "delete_dimension", "input": map[string]any{"sheetId": "Sheet1", "dimension": "ROWS", "startIndex": 2, "count": 3}},
			},
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
