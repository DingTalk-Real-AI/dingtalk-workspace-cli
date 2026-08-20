// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package todo

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type todoPageSizeCaller struct {
	calls int
}

func (c *todoPageSizeCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	c.calls++
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":true,"result":{"todoCards":[]}}`}}}, nil
}

func (*todoPageSizeCaller) Format() string { return "json" }
func (*todoPageSizeCaller) DryRun() bool   { return false }
func (*todoPageSizeCaller) Fields() string { return "" }
func (*todoPageSizeCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageGetMyTasksRejectsUnsupportedPageSize(t *testing.T) {
	caller := &todoPageSizeCaller{}
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.AddCommand(shortcut.Commands()...)
	root.SetArgs([]string{"todo", "+get-my-tasks", "--size", "21"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--size") {
		t.Fatalf("size 21 error = %v, want local --size validation", err)
	}
	if caller.calls != 0 {
		t.Fatalf("size 21 made %d remote calls, want 0", caller.calls)
	}
}
