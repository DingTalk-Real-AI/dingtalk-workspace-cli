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

type mailTemplateUpdateCaller struct {
	dryRun       bool
	dryRunChecks int
	calls        int
	arguments    map[string]any
}

func (c *mailTemplateUpdateCaller) CallTool(_ context.Context, _, _ string, arguments map[string]any) (*edition.ToolResult, error) {
	c.calls++
	c.arguments = arguments
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*mailTemplateUpdateCaller) Format() string { return "json" }
func (c *mailTemplateUpdateCaller) DryRun() bool {
	c.dryRunChecks++
	return c.dryRun
}
func (*mailTemplateUpdateCaller) Fields() string { return "" }
func (*mailTemplateUpdateCaller) JQ() string     { return "" }

func executeMailTemplateUpdate(t *testing.T, caller *mailTemplateUpdateCaller, arguments ...string) error {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard

	cmd := newMailCommand()
	cmd.PersistentFlags().Bool("dry-run", false, "preview only")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(arguments)
	return cmd.Execute()
}

func TestMailTemplateUpdateRejectsNoOpBeforeToolCaller(t *testing.T) {
	for _, test := range []struct {
		name   string
		dryRun bool
		args   []string
	}{
		{
			name: "real execution",
			args: []string{"template", "update", "--email", "user@example.com", "--id", "template-1"},
		},
		{
			name:   "dry run",
			dryRun: true,
			args:   []string{"--dry-run", "template", "update", "--email", "user@example.com", "--id", "template-1"},
		},
		{
			name: "blank actual payload",
			args: []string{"template", "update", "--email", "user@example.com", "--id", "template-1", "--subject", "  ", "--to", " , "},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &mailTemplateUpdateCaller{dryRun: test.dryRun}
			err := executeMailTemplateUpdate(t, caller, test.args...)
			if err == nil || !strings.Contains(err.Error(), "至少需要指定一个更新字段") {
				t.Fatalf("mail template update error = %v", err)
			}
			if caller.calls != 0 || caller.dryRunChecks != 0 {
				t.Fatalf("no-op reached ToolCaller: calls=%d dry_run_checks=%d", caller.calls, caller.dryRunChecks)
			}
		})
	}
}

func TestMailTemplateUpdateAcceptsEveryRuntimeUpdateField(t *testing.T) {
	for _, test := range []struct {
		name      string
		flag      string
		value     string
		wantKey   string
		wantValue any
	}{
		{name: "from", flag: "--from", value: "sender@example.com", wantKey: "from", wantValue: "sender@example.com"},
		{name: "subject", flag: "--subject", value: "updated subject", wantKey: "subject", wantValue: "updated subject"},
		{name: "public content", flag: "--content", value: "updated body", wantKey: "body", wantValue: "updated body"},
		{name: "hidden body alias", flag: "--body", value: "updated body", wantKey: "body", wantValue: "updated body"},
		{name: "name", flag: "--name", value: "updated name", wantKey: "name", wantValue: "updated name"},
		{name: "to", flag: "--to", value: "first@example.com, second@example.com", wantKey: "toRecipients", wantValue: []string{"first@example.com", "second@example.com"}},
		{name: "cc", flag: "--cc", value: "copy@example.com", wantKey: "ccRecipients", wantValue: []string{"copy@example.com"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &mailTemplateUpdateCaller{}
			err := executeMailTemplateUpdate(t, caller,
				"template", "update", "--email", "user@example.com", "--id", "template-1",
				test.flag, test.value,
			)
			if err != nil {
				t.Fatalf("mail template update error = %v", err)
			}
			if caller.calls != 1 {
				t.Fatalf("ToolCaller calls = %d, want 1", caller.calls)
			}
			if got := caller.arguments[test.wantKey]; !reflect.DeepEqual(got, test.wantValue) {
				t.Fatalf("RPC %s = %#v, want %#v", test.wantKey, got, test.wantValue)
			}
		})
	}
}

func TestMailTemplateUpdateValidDryRunReachesPreviewOnly(t *testing.T) {
	caller := &mailTemplateUpdateCaller{dryRun: true}
	err := executeMailTemplateUpdate(t, caller,
		"--dry-run", "template", "update", "--email", "user@example.com", "--id", "template-1",
		"--subject", "updated subject",
	)
	if err != nil {
		t.Fatalf("mail template update dry-run error = %v", err)
	}
	if caller.calls != 0 || caller.dryRunChecks == 0 {
		t.Fatalf("dry-run execution: calls=%d dry_run_checks=%d", caller.calls, caller.dryRunChecks)
	}
}
