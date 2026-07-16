// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type mailDryRunCaller struct {
	dryRun       bool
	dryRunChecks int
	calls        int
}

func (c *mailDryRunCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	c.calls++
	return &edition.ToolResult{}, nil
}

func (*mailDryRunCaller) Format() string { return "json" }
func (c *mailDryRunCaller) DryRun() bool {
	c.dryRunChecks++
	return c.dryRun
}
func (*mailDryRunCaller) Fields() string { return "" }
func (*mailDryRunCaller) JQ() string     { return "" }

func executeMailThreadDelete(t *testing.T, caller *mailDryRunCaller, args ...string) (string, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	var output bytes.Buffer
	deps.Out.w = &output
	deps.Out.errW = &output

	cmd := newMailCommand()
	cmd.PersistentFlags().Bool("dry-run", false, "preview only")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return output.String(), cmd.Execute()
}

func TestMailThreadDeleteDryRunDoesNotRequireYesOrCallRemote(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "single",
			args: []string{"--dry-run", "thread", "trash", "--email", "user@example.com", "--id", "conversation-1"},
		},
		{
			name: "batch",
			args: []string{"--dry-run", "thread", "batch-trash", "--email", "user@example.com", "--ids", "conversation-1,conversation-2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailDryRunCaller{dryRun: true}
			_, err := executeMailThreadDelete(t, caller, tc.args...)
			if err != nil {
				t.Fatalf("mail thread %s dry-run failed: %v", tc.name, err)
			}
			if caller.calls != 0 {
				t.Fatalf("mail thread %s dry-run made %d remote call(s)", tc.name, caller.calls)
			}
			if caller.dryRunChecks == 0 {
				t.Fatalf("mail thread %s never entered the audited dry-run output path", tc.name)
			}
		})
	}
}

func TestMailThreadDeleteYesFalseDoesNotConfirmRealWrite(t *testing.T) {
	caller := &mailDryRunCaller{}
	_, err := executeMailThreadDelete(t, caller,
		"thread", "trash", "--email", "user@example.com", "--id", "conversation-1", "--yes=false")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("mail thread trash --yes=false error = %v, want confirmation error", err)
	}
	if caller.calls != 0 {
		t.Fatalf("mail thread trash --yes=false made %d remote call(s)", caller.calls)
	}
}
