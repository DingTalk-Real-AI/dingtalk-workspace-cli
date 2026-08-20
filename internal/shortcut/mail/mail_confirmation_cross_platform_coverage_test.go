// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package mail

import (
	"context"
	"io"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type mailConfirmationCaller struct{ calls int }

func (caller *mailConfirmationCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	caller.calls++
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":true}`}}}, nil
}

func (*mailConfirmationCaller) Format() string { return "json" }
func (*mailConfirmationCaller) DryRun() bool   { return false }
func (*mailConfirmationCaller) Fields() string { return "" }
func (*mailConfirmationCaller) JQ() string     { return "" }

func runUnconfirmedMailWrite(t *testing.T, caller *mailConfirmationCaller, args ...string) error {
	t.Helper()
	helpers.InitDeps(caller)
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.AddCommand(shortcut.Commands()...)
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageMailConfirmationPrecedesRemoteCall(t *testing.T) {
	cases := [][]string{
		{"mail", "+draft-create", "--from", "sender@example.invalid", "--subject", "fixture"},
		{"mail", "+draft-edit", "--from", "sender@example.invalid", "--id", "draft-id", "--subject", "fixture"},
		{"mail", "+template-create", "--email", "sender@example.invalid", "--name", "fixture", "--subject", "fixture", "--body", "fixture"},
		{"mail", "+template-update", "--email", "sender@example.invalid", "--id", "template-id", "--subject", "fixture"},
	}
	for _, args := range cases {
		caller := &mailConfirmationCaller{}
		if err := runUnconfirmedMailWrite(t, caller, args...); err == nil {
			t.Fatalf("unconfirmed %v unexpectedly succeeded", args)
		}
		if caller.calls != 0 {
			t.Fatalf("unconfirmed %v reached MCP %d times", args, caller.calls)
		}
	}
}
