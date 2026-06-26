// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package compat

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/spf13/cobra"
)

// findLeaf returns the first leaf command with the given Use anywhere under
// root (depth-first), or nil.
func findLeaf(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
		if got := findLeaf(c, name); got != nil {
			return got
		}
	}
	return nil
}

func toolSet(names ...string) map[string]struct{} {
	s := make(map[string]struct{}, len(names))
	for _, n := range names {
		s[n] = struct{}{}
	}
	return s
}

// attendanceLike builds one server with a real tool and a phantom tool, the
// exact shape of the production drift (e.g. attendance: only a handful of the
// declared overrides map to deployed tools).
func attendanceLike() []market.ServerDescriptor {
	return []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-attendance",
			CLI: market.CLIOverlay{
				ID:      "attendance",
				Command: "attendance",
				ToolOverrides: map[string]market.CLIToolOverride{
					"get_attendance_summary": {CLIName: "summary"},  // real
					"get_overtime_rule":      {CLIName: "overtime"}, // phantom
				},
			},
		},
	}
}

// TestPhantomGuard_HidesWhenToolSetKnown is the core behaviour: when the live
// tool set is known and non-empty, a leaf whose backing tool is absent is
// hidden from --help while the real leaf stays visible.
func TestPhantomGuard_HidesWhenToolSetKnown(t *testing.T) {
	t.Parallel()

	existing := map[string]map[string]struct{}{
		"attendance": toolSet("get_attendance_summary"), // overtime is NOT deployed
	}
	cmds := BuildDynamicCommands(attendanceLike(), executor.EchoRunner{}, nil, existing)

	summary := findLeaf(cmds[0], "summary")
	overtime := findLeaf(cmds[0], "overtime")
	if summary == nil || overtime == nil {
		t.Fatalf("both leaves should still be registered (invocable); summary=%v overtime=%v", summary, overtime)
	}
	if summary.Hidden {
		t.Error("real command 'summary' must stay visible in --help")
	}
	if !overtime.Hidden {
		t.Error("phantom command 'overtime' must be hidden from --help")
	}
}

// TestPhantomGuard_ColdCacheKeepsEverything is the safety rail that the prior
// (source-blind) plan got wrong: with no tool set available (nil map), the
// guard must do nothing — never blank the command tree on a cold cache.
func TestPhantomGuard_ColdCacheKeepsEverything(t *testing.T) {
	t.Parallel()

	cmds := BuildDynamicCommands(attendanceLike(), executor.EchoRunner{}, nil, nil)
	for _, name := range []string{"summary", "overtime"} {
		leaf := findLeaf(cmds[0], name)
		if leaf == nil {
			t.Fatalf("%q should be registered", name)
		}
		if leaf.Hidden {
			t.Errorf("cold cache (nil existingTools) must not hide %q", name)
		}
	}
}

// TestPhantomGuard_EmptyOrAbsentSetKeepsEverything: an empty set for a server,
// or a server missing from the map entirely, both mean "unknown" — keep all.
func TestPhantomGuard_EmptyOrAbsentSetKeepsEverything(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		existing map[string]map[string]struct{}
	}{
		{"empty set for server", map[string]map[string]struct{}{"attendance": {}}},
		{"server absent from map", map[string]map[string]struct{}{"someother": toolSet("x")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmds := BuildDynamicCommands(attendanceLike(), executor.EchoRunner{}, nil, tc.existing)
			for _, name := range []string{"summary", "overtime"} {
				leaf := findLeaf(cmds[0], name)
				if leaf == nil {
					t.Fatalf("%q should be registered", name)
				}
				if leaf.Hidden {
					t.Errorf("%s: must not hide %q when tool set is unknown", tc.name, name)
				}
			}
		})
	}
}

// TestPhantomGuard_ServerOverrideRoutesToTargetSet: a leaf with serverOverride
// must be checked against the TARGET server's tool set, not the host's. This is
// what prevents false-flagging legit cross-server routes (contact→hrmregister,
// doc→doc-comment).
func TestPhantomGuard_ServerOverrideRoutesToTargetSet(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-contact",
			CLI: market.CLIOverlay{
				ID:      "contact",
				Command: "contact",
				ToolOverrides: map[string]market.CLIToolOverride{
					// routed to hrmregister; the tool lives there, not in contact
					"get_roster": {CLIName: "roster", ServerOverride: "hrmregister"},
				},
			},
		},
	}
	// contact's own set is empty of get_roster, but hrmregister has it.
	existing := map[string]map[string]struct{}{
		"contact":     toolSet("search_user"),
		"hrmregister": toolSet("get_roster"),
	}
	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil, existing)
	roster := findLeaf(cmds[0], "roster")
	if roster == nil {
		t.Fatal("roster leaf should be registered")
	}
	if roster.Hidden {
		t.Error("serverOverride leaf must resolve against the target server's set and stay visible")
	}
}

// TestPhantomGuard_EmptyGroupCollapses: a group all of whose overrides are
// hidden:true (so none of its leaves are built) must itself be hidden from
// help, while a group keeping at least one visible leaf stays. This runs
// regardless of the tools-cache oracle (envelope hidden:true is cache-
// independent), so existingTools is nil here.
func TestPhantomGuard_EmptyGroupCollapses(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-attendance",
			CLI: market.CLIOverlay{
				ID:      "attendance",
				Command: "attendance",
				Groups: map[string]market.CLIGroupDef{
					"vacation": {Description: "假期管理"}, // all leaves hidden -> collapse
					"record":   {Description: "考勤记录"}, // keeps a visible leaf
				},
				ToolOverrides: map[string]market.CLIToolOverride{
					"get_leave_types":            {CLIName: "types", Group: "vacation", Hidden: true},
					"get_leave_balance_quota":    {CLIName: "balance", Group: "vacation", Hidden: true},
					"get_user_attendance_record": {CLIName: "get", Group: "record"},
				},
			},
		},
	}
	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil, nil)

	vacation := findGroup(cmds[0], "vacation")
	record := findGroup(cmds[0], "record")
	if vacation == nil || record == nil {
		t.Fatalf("both groups should exist as commands; vacation=%v record=%v", vacation, record)
	}
	if !vacation.Hidden {
		t.Error("group 'vacation' with only hidden leaves must collapse (be hidden)")
	}
	if record.Hidden {
		t.Error("group 'record' with a visible leaf must stay visible")
	}
}

// findGroup returns a direct child of root with the given name (groups attach
// directly under the product root).
func findGroup(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// TestPhantomGuard_PipelineLeafNeverHidden: pipeline leaves orchestrate multiple
// tools and have no single backing toolName, so the guard must skip them even
// when the override key is not a deployed tool.
func TestPhantomGuard_PipelineLeafNeverHidden(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{
		{
			Endpoint: "https://endpoint-im",
			CLI: market.CLIOverlay{
				ID:      "im",
				Command: "im",
				ToolOverrides: map[string]market.CLIToolOverride{
					"download_media": {
						CLIName: "download-media",
						Pipeline: []market.PipelineStep{
							{Tool: "get_resource_download_url"},
						},
					},
				},
			},
		},
	}
	// download_media itself is not a deployed tool name, but the pipeline is.
	existing := map[string]map[string]struct{}{
		"im": toolSet("get_resource_download_url"),
	}
	cmds := BuildDynamicCommands(servers, executor.EchoRunner{}, nil, existing)
	dl := findLeaf(cmds[0], "download-media")
	if dl == nil {
		t.Fatal("download-media leaf should be registered")
	}
	if dl.Hidden {
		t.Error("pipeline leaf must never be hidden by the tool-existence guard")
	}
}
