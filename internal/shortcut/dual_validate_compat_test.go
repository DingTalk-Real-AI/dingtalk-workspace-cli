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
	err    error
}

func (c *dualValidateCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.text}}}, nil
}

func TestFrameworkDualValidateBackendAndShadowErrors(t *testing.T) {
	caller := &dualValidateCaller{format: "json", text: `{"id":"x"}`, err: context.DeadlineExceeded}
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("format", "json", "")
	cmd := mount(Shortcut{Service: "sample", Command: "+compat", OutputRollout: output.RolloutDualValidate, Execute: func(rt *RuntimeContext) error { return rt.CallMCP("get", nil) }})
	root.AddCommand(cmd)
	root.SetArgs([]string{cmd.Name()})
	if err := root.Execute(); err == nil {
		t.Fatal("backend error swallowed")
	}

	original := validateShadowResult
	t.Cleanup(func() { validateShadowResult = original })
	validateShadowResult = func(output.CommandResult) error { return context.Canceled }
	direct := &cobra.Command{Use: "+direct"}
	direct.SetContext(context.Background())
	output.SetCommandRollout(direct, output.RolloutDualValidate)
	if err := RuntimeContextForTest(direct, Shortcut{Service: "sample", Command: "+direct"}).Output(map[string]any{"id": "x"}); err == nil {
		t.Fatal("Output shadow validation error swallowed")
	}
	caller.err = nil
	root.SetArgs([]string{cmd.Name()})
	if err := root.Execute(); err == nil {
		t.Fatal("shadow validation error swallowed")
	}
	caller.dryRun = true
	root.SetArgs([]string{cmd.Name(), "--dry-run"})
	root.PersistentFlags().Bool("dry-run", false, "")
	if err := root.Execute(); err == nil {
		t.Fatal("dry-run shadow validation error swallowed")
	}
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

func TestDualValidatePreservesLegacyJSONEscapingAndIndentation(t *testing.T) {
	caller := &dualValidateCaller{format: "json", text: `{"url":"https://example.test/?a=1&b=2","tag":"<x>"}`}
	got, _ := runDualValidateShortcut(t, caller, "--format", "json")
	want := "{\n  \"tag\": \"\\u003cx\\u003e\",\n  \"url\": \"https://example.test/?a=1\\u0026b=2\"\n}\n"
	if got != want {
		t.Fatalf("json output = %q, want legacy bytes %q", got, want)
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
	if !strings.Contains(stdout, "Tool") || !strings.Contains(stdout, "get_sample") || !strings.Contains(stdout, "Arguments") || stderr != "" {
		t.Fatalf("dry-run output lost legacy presentation: stdout=%q stderr=%q", stdout, stderr)
	}
	if caller.calls != 0 {
		t.Fatalf("dry-run business calls = %d, want 0", caller.calls)
	}
}
