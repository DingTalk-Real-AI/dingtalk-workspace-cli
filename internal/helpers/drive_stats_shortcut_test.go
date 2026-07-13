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

package helpers

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type driveStatsShortcutCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type driveStatsShortcutCaller struct {
	calls []driveStatsShortcutCall
}

func (c *driveStatsShortcutCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, driveStatsShortcutCall{productID: productID, toolName: toolName, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*driveStatsShortcutCaller) Format() string { return "json" }
func (*driveStatsShortcutCaller) DryRun() bool   { return false }
func (*driveStatsShortcutCaller) Fields() string { return "" }
func (*driveStatsShortcutCaller) JQ() string     { return "" }

func executeDriveStatsShortcutCommand(t *testing.T, caller *driveStatsShortcutCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	InitDeps(caller)
	deps.Out.w = io.Discard
	cmd := newDriveCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestDriveStatsShortcutCommandsRegistered(t *testing.T) {
	root := newDriveCommand()
	cases := []struct {
		path  string
		flags []string
	}{
		{path: "stats", flags: []string{"node", "url", "id", "node-id", "doc-id", "file-id"}},
		{path: "shortcut", flags: []string{"node", "url", "id", "node-id", "doc-id", "file-id", "folder", "workspace"}},
	}
	for _, tc := range cases {
		cmd, remaining, err := root.Find([]string{tc.path})
		if err != nil || len(remaining) != 0 {
			t.Fatalf("dws drive %s not registered: cmd=%v remaining=%v err=%v", tc.path, cmd, remaining, err)
		}
		for _, flag := range tc.flags {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("dws drive %s missing flag --%s", tc.path, flag)
			}
		}
	}
}

func TestDriveStatsMapsOpenToolArguments(t *testing.T) {
	caller := &driveStatsShortcutCaller{}
	if err := executeDriveStatsShortcutCommand(t, caller, "stats", "--url", "https://alidocs.dingtalk.com/i/nodes/node-1"); err != nil {
		t.Fatalf("drive stats returned error: %v", err)
	}
	want := driveStatsShortcutCall{
		productID: "drive",
		toolName:  "get_node_stats",
		args:      map[string]any{"nodeId": "https://alidocs.dingtalk.com/i/nodes/node-1"},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestDriveShortcutMapsOptionalTargetArguments(t *testing.T) {
	caller := &driveStatsShortcutCaller{}
	if err := executeDriveStatsShortcutCommand(t, caller,
		"shortcut", "--file-id", "source-1", "--folder", "folder-1", "--workspace", "workspace-1"); err != nil {
		t.Fatalf("drive shortcut returned error: %v", err)
	}
	want := driveStatsShortcutCall{
		productID: "drive",
		toolName:  "create_shortcut",
		args: map[string]any{
			"nodeId":         "source-1",
			"targetFolderId": "folder-1",
			"workspaceId":    "workspace-1",
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestDriveShortcutOmitsUnsetOptionalArguments(t *testing.T) {
	caller := &driveStatsShortcutCaller{}
	if err := executeDriveStatsShortcutCommand(t, caller, "shortcut", "--node", "source-1"); err != nil {
		t.Fatalf("drive shortcut returned error: %v", err)
	}
	want := map[string]any{"nodeId": "source-1"}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("calls = %#v, want args %#v", caller.calls, want)
	}
}

func TestDriveStatsShortcutRejectMissingNode(t *testing.T) {
	for _, path := range []string{"stats", "shortcut"} {
		t.Run(path, func(t *testing.T) {
			caller := &driveStatsShortcutCaller{}
			if err := executeDriveStatsShortcutCommand(t, caller, path); err == nil {
				t.Fatalf("drive %s without --node returned nil error", path)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("drive %s remote calls = %d, want 0", path, len(caller.calls))
			}
		})
	}
}

func TestDriveShortcutRejectsNumericDocFolder(t *testing.T) {
	caller := &driveStatsShortcutCaller{}
	err := executeDriveStatsShortcutCommand(t, caller, "shortcut", "--node", "source-1", "--folder", "123456")
	if err == nil {
		t.Fatal("drive shortcut accepted a pure numeric doc folder")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote calls = %d, want 0", len(caller.calls))
	}
}
