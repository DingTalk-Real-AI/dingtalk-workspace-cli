// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableExportCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type aitableExportCaller struct {
	calls []aitableExportCall
}

func (c *aitableExportCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, aitableExportCall{productID: productID, toolName: toolName, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*aitableExportCaller) Format() string { return "json" }
func (*aitableExportCaller) DryRun() bool   { return false }
func (*aitableExportCaller) Fields() string { return "" }
func (*aitableExportCaller) JQ() string     { return "" }

func runAitableExportCommand(t *testing.T, args ...string) (*aitableExportCaller, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	caller := &aitableExportCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard

	cmd := newAitableCommand()
	cmd.PersistentFlags().String("format", "json", "output format")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(append([]string{"export", "data"}, args...))
	return caller, cmd.Execute()
}

func TestAitableExportUsesDedicatedBusinessFormatWithJSONOutput(t *testing.T) {
	caller, err := runAitableExportCommand(t,
		"--base-id", "base-smoke",
		"--scope", "table",
		"--table-id", "table-smoke",
		"--export-format", "excel",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("aitable export data returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.toolName != "export_data" {
		t.Fatalf("tool call = %s/%s, want export_data", call.productID, call.toolName)
	}
	want := map[string]any{
		"baseId":  "base-smoke",
		"scope":   "table",
		"format":  "excel",
		"tableId": "table-smoke",
	}
	if !reflect.DeepEqual(call.args, want) {
		t.Fatalf("tool args = %#v, want %#v", call.args, want)
	}
}

func TestAitableExportNormalizesBusinessFormat(t *testing.T) {
	caller, err := runAitableExportCommand(t,
		"--base-id", "base-smoke",
		"--scope", "all",
		"--export-format", " EXCEL ",
	)
	if err != nil {
		t.Fatalf("aitable export data returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(caller.calls))
	}
	if got := caller.calls[0].args["format"]; got != "excel" {
		t.Fatalf("tool format = %#v, want excel", got)
	}
}

func TestAitableExportTaskPollingDoesNotRequireCreationFlags(t *testing.T) {
	caller, err := runAitableExportCommand(t,
		"--base-id", "base-smoke",
		"--task-id", "task-smoke",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("aitable export polling returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(caller.calls))
	}
	want := map[string]any{"baseId": "base-smoke", "taskId": "task-smoke"}
	if !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("tool args = %#v, want %#v", caller.calls[0].args, want)
	}
}

func TestAitableExportAcceptsLegacyBusinessFormat(t *testing.T) {
	caller, err := runAitableExportCommand(t,
		"--base-id", "base-smoke",
		"--scope", "all",
		"--format", "excel",
	)
	if err != nil {
		t.Fatalf("legacy aitable export returned error: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].args["format"] != "excel" {
		t.Fatalf("legacy tool calls = %#v, want one call with format=excel", caller.calls)
	}
}

func TestAitableExportValidatesBranchSpecificInputs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "table scope requires table id",
			args:    []string{"--base-id", "base-smoke", "--scope", "table", "--export-format", "excel"},
			wantErr: "--table-id",
		},
		{
			name:    "task polling rejects creation flags",
			args:    []string{"--base-id", "base-smoke", "--task-id", "task-smoke", "--scope", "all", "--format", "json"},
			wantErr: "mutually exclusive",
		},
		{
			name:    "all scope rejects table id",
			args:    []string{"--base-id", "base-smoke", "--scope", "all", "--table-id", "table-smoke", "--export-format", "excel"},
			wantErr: "--scope=all does not accept",
		},
		{
			name:    "all scope rejects view id",
			args:    []string{"--base-id", "base-smoke", "--scope", "all", "--view-id", "view-smoke", "--export-format", "excel"},
			wantErr: "--scope=all does not accept",
		},
		{
			name:    "table scope rejects view id",
			args:    []string{"--base-id", "base-smoke", "--scope", "table", "--table-id", "table-smoke", "--view-id", "view-smoke", "--export-format", "excel"},
			wantErr: "--scope=table does not accept --view-id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller, err := runAitableExportCommand(t, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool call count = %d, want 0", len(caller.calls))
			}
		})
	}
}
