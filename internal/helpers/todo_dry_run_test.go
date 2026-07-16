// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type todoAttachmentDryRunCaller struct {
	dryRunChecks int
	calls        int
}

func (c *todoAttachmentDryRunCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	c.calls++
	return &edition.ToolResult{}, nil
}

func (*todoAttachmentDryRunCaller) Format() string { return "json" }
func (c *todoAttachmentDryRunCaller) DryRun() bool {
	c.dryRunChecks++
	return true
}
func (*todoAttachmentDryRunCaller) Fields() string { return "" }
func (*todoAttachmentDryRunCaller) JQ() string     { return "" }

func TestTodoRemoveAttachmentDryRunSkipsConfirmationAndRemoteCall(t *testing.T) {
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	caller := &todoAttachmentDryRunCaller{}
	InitDeps(caller)

	cmd := newTodoCommand()
	cmd.PersistentFlags().Bool("dry-run", false, "preview only")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{
		"--dry-run", "task", "remove-attachment",
		"--task-id", "task-1", "--attachment-id", "attachment-1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("todo task remove-attachment dry-run failed: %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("todo task remove-attachment dry-run made %d remote call(s)", caller.calls)
	}
	if caller.dryRunChecks == 0 {
		t.Fatal("todo task remove-attachment never entered the audited dry-run output path")
	}
}
