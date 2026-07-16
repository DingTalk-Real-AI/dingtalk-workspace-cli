// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"io"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type minutesListScopeCaller struct {
	calls     int
	serverID  string
	toolName  string
	arguments map[string]any
}

func (c *minutesListScopeCaller) CallTool(_ context.Context, serverID, toolName string, arguments map[string]any) (*edition.ToolResult, error) {
	c.calls++
	c.serverID = serverID
	c.toolName = toolName
	c.arguments = arguments
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*minutesListScopeCaller) Format() string { return "json" }
func (*minutesListScopeCaller) DryRun() bool   { return false }
func (*minutesListScopeCaller) Fields() string { return "" }
func (*minutesListScopeCaller) JQ() string     { return "" }

func TestMinutesListLeavesDeliverTheirFixedRPCScope(t *testing.T) {
	for _, test := range []struct {
		path      string
		wantScope string
	}{
		{path: "mine", wantScope: "created"},
		{path: "shared", wantScope: "shared"},
		{path: "all", wantScope: "noLimit"},
	} {
		t.Run(test.path, func(t *testing.T) {
			previousDeps := deps
			t.Cleanup(func() { deps = previousDeps })
			caller := &minutesListScopeCaller{}
			InitDeps(caller)
			deps.Out.w = io.Discard
			deps.Out.errW = io.Discard

			command := newMinutesCommand()
			command.SilenceErrors = true
			command.SilenceUsage = true
			command.SetArgs([]string{"list", test.path})
			if err := command.Execute(); err != nil {
				t.Fatalf("minutes list %s: %v", test.path, err)
			}
			if caller.calls != 1 {
				t.Fatalf("minutes list %s calls = %d, want 1", test.path, caller.calls)
			}
			if caller.serverID != "minutes" || caller.toolName != "list_by_keyword_and_time_range" {
				t.Fatalf("minutes list %s routed to %s/%s", test.path, caller.serverID, caller.toolName)
			}
			if got := caller.arguments["belongingConditionId"]; got != test.wantScope {
				t.Fatalf("minutes list %s belongingConditionId = %#v, want %q", test.path, got, test.wantScope)
			}
		})
	}
}
