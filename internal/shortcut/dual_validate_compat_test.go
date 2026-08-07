// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package shortcut

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type dualValidateCaller struct {
	format string
	dryRun bool
	calls  int
	text   string
}

func (c *dualValidateCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	c.calls++
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.text}}}, nil
}

func (c *dualValidateCaller) Format() string { return c.format }
func (c *dualValidateCaller) DryRun() bool   { return c.dryRun }
func (c *dualValidateCaller) Fields() string { return "" }
func (c *dualValidateCaller) JQ() string     { return "" }

func runDualValidateShortcut(t *testing.T, caller *dualValidateCaller, args ...string) (string, string) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	var stdout, stderr bytes.Buffer
	helpers.GetFormatter().SetWriters(&stdout, &stderr)

	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Bool("dry-run", false, "")
	cmd := mount(Shortcut{
		Service:       "sample",
		Command:       "+compat",
		OutputRollout: output.RolloutDualValidate,
		Execute: func(rt *RuntimeContext) error {
			return rt.CallMCP("get_sample", map[string]any{"id": "1"})
		},
	})
	cmd.SetOut(&stdout)
	root.AddCommand(cmd)
	root.SetArgs(append([]string{cmd.Name()}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	return stdout.String(), stderr.String()
}

func TestDualValidatePreservesLegacyRawBytesWithOneBusinessCall(t *testing.T) {
	caller := &dualValidateCaller{format: "raw", text: `{"url":"https://example.test/?a=1&b=2"}`}
	got, _ := runDualValidateShortcut(t, caller, "--format", "raw")
	if want := caller.text + "\n"; got != want {
		t.Fatalf("raw output = %q, want legacy bytes %q", got, want)
	}
	if caller.calls != 1 {
		t.Fatalf("business calls = %d, want 1", caller.calls)
	}
}

func TestDualValidatePreservesLegacyPlainTextFormat(t *testing.T) {
	caller := &dualValidateCaller{format: "table", text: "legacy plain text"}
	got, _ := runDualValidateShortcut(t, caller, "--format", "table")
	if got != "legacy plain text\n" {
		t.Fatalf("table/plain output = %q", got)
	}
	if caller.calls != 1 {
		t.Fatalf("business calls = %d, want 1", caller.calls)
	}
}

func TestDualValidateDoesNotJSONQuoteLegacyPlainText(t *testing.T) {
	caller := &dualValidateCaller{format: "json", text: "legacy plain text"}
	got, _ := runDualValidateShortcut(t, caller, "--format", "json")
	if got != "legacy plain text\n" {
		t.Fatalf("json/plain output = %q", got)
	}
	if caller.calls != 1 {
		t.Fatalf("business calls = %d, want 1", caller.calls)
	}
}

func TestDualValidatePreservesLegacyDryRunWithoutBusinessCall(t *testing.T) {
	caller := &dualValidateCaller{format: "table", dryRun: true}
	stdout, stderr := runDualValidateShortcut(t, caller, "--format", "table", "--dry-run")
	if stdout != "" || !strings.Contains(stderr, "Tool") || !strings.Contains(stderr, "get_sample") || !strings.Contains(stderr, "Arguments") {
		t.Fatalf("dry-run output lost legacy presentation: stdout=%q stderr=%q", stdout, stderr)
	}
	if caller.calls != 0 {
		t.Fatalf("dry-run business calls = %d, want 0", caller.calls)
	}
}
