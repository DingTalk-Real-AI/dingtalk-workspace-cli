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
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type driveParityCall struct {
	server string
	tool   string
	args   map[string]any
}

type driveParityCaller struct {
	calls  []driveParityCall
	dryRun bool
}

func (c *driveParityCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, driveParityCall{server: server, tool: tool, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*driveParityCaller) Format() string { return "json" }
func (c *driveParityCaller) DryRun() bool { return c.dryRun }
func (*driveParityCaller) Fields() string { return "" }
func (*driveParityCaller) JQ() string     { return "" }

func executeDriveParityCommand(t *testing.T, caller *driveParityCaller, stdin string, args ...string) (string, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	InitDeps(caller)
	var output bytes.Buffer
	deps.Out.w = &output
	deps.Out.errW = &output

	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolP("yes", "y", false, "skip confirmation")
	root.PersistentFlags().Bool("dry-run", false, "preview")
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&output)
	root.SetErr(&output)
	root.AddCommand(newDriveCommand())
	root.SetArgs(append([]string{"drive"}, args...))
	err := root.Execute()
	return output.String(), err
}

func TestDrivePermissionAndStarCommandsRegistered(t *testing.T) {
	root := newDriveCommand()
	paths := []string{
		"permission transfer-owner",
		"permission apply-info",
		"permission apply",
		"star add",
		"star remove",
		"star list",
	}
	for _, path := range paths {
		command, remaining, err := root.Find(strings.Fields(path))
		if err != nil || len(remaining) != 0 || command == nil {
			t.Errorf("drive %s not registered: command=%v remaining=%v err=%v", path, command, remaining, err)
		}
	}
}

func TestDriveTransferOwnerMapsExplicitSafeArguments(t *testing.T) {
	caller := &driveParityCaller{}
	_, err := executeDriveParityCommand(t, caller, "",
		"permission", "transfer-owner",
		"--node", "node-1",
		"--new-owner", "user-2",
		"--reserve-role", "editor",
		"--recursive=false",
		"--yes",
	)
	if err != nil {
		t.Fatalf("transfer-owner returned error: %v", err)
	}
	want := driveParityCall{
		server: "doc",
		tool:   "transfer_owner",
		args: map[string]any{
			"nodeId":              "node-1",
			"newOwnerId":          "user-2",
			"reserveOldOwnerRole": "EDITOR",
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestDriveTransferOwnerInteractiveAnswersShareOneReader(t *testing.T) {
	caller := &driveParityCaller{}
	_, err := executeDriveParityCommand(t, caller, "2\ny\nyes\n",
		"permission", "transfer-owner",
		"--node", "node-1",
		"--new-owner", "user-2",
	)
	if err != nil {
		t.Fatalf("interactive transfer-owner returned error: %v", err)
	}
	want := driveParityCall{
		server: "doc",
		tool:   "transfer_owner",
		args: map[string]any{
			"nodeId":              "node-1",
			"newOwnerId":          "user-2",
			"reserveOldOwnerRole": "EDITOR",
			"recursiveChange":     true,
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestDriveTransferOwnerYesRequiresExplicitRiskChoices(t *testing.T) {
	caller := &driveParityCaller{}
	_, err := executeDriveParityCommand(t, caller, "",
		"permission", "transfer-owner", "--node", "node-1", "--new-owner", "user-2", "--yes")
	if err == nil || !strings.Contains(err.Error(), "--reserve-role") {
		t.Fatalf("error = %v, want missing --reserve-role", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote calls = %#v, want none", caller.calls)
	}
}

func TestDriveTransferOwnerDryRunDoesNotPromptOrCall(t *testing.T) {
	caller := &driveParityCaller{dryRun: true}
	output, err := executeDriveParityCommand(t, caller, "",
		"permission", "transfer-owner", "--workspace", "workspace-1", "--new-owner", "user-2", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote calls = %#v, want none", caller.calls)
	}
	if !strings.Contains(output, "workspace-1") || !strings.Contains(output, "user-2") {
		t.Fatalf("dry-run output = %q", output)
	}
}

func TestDrivePermissionApplyMapsValidatedArguments(t *testing.T) {
	caller := &driveParityCaller{}
	_, err := executeDriveParityCommand(t, caller, "",
		"permission", "apply",
		"--node", "node-1",
		"--role", "reader",
		"--users", "approver-1,approver-2",
		"--notify-mode", "single_chat",
		"--reason", "need access",
		"--yes",
	)
	if err != nil {
		t.Fatalf("permission apply returned error: %v", err)
	}
	want := driveParityCall{
		server: "drive",
		tool:   "apply_permission",
		args: map[string]any{
			"nodeId":     "node-1",
			"roleId":     "READER",
			"receivers":  []string{"approver-1", "approver-2"},
			"notifyMode": "SINGLE_CHAT",
			"reason":     "need access",
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestDrivePermissionApplyRejectsUnsupportedRoleBeforeCall(t *testing.T) {
	caller := &driveParityCaller{}
	_, err := executeDriveParityCommand(t, caller, "",
		"permission", "apply", "--node", "node-1", "--role", "OWNER", "--users", "approver-1", "--yes")
	if err == nil || !strings.Contains(err.Error(), "--role") {
		t.Fatalf("error = %v, want invalid role", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote calls = %#v, want none", caller.calls)
	}
}

func TestDriveStarCommandsMapArguments(t *testing.T) {
	caller := &driveParityCaller{}
	if _, err := executeDriveParityCommand(t, caller, "", "star", "add", "--node", "node-1"); err != nil {
		t.Fatalf("star add: %v", err)
	}
	if _, err := executeDriveParityCommand(t, caller, "", "star", "remove", "--node", "node-1"); err != nil {
		t.Fatalf("star remove: %v", err)
	}
	if _, err := executeDriveParityCommand(t, caller, "", "star", "list", "--limit", "10", "--sort", "DESC", "--resource-types", "dentry", "--content-types", "doc,sheet"); err != nil {
		t.Fatalf("star list: %v", err)
	}
	want := []driveParityCall{
		{server: "drive", tool: "mark_star", args: map[string]any{"nodeId": "node-1"}},
		{server: "drive", tool: "unmark_star", args: map[string]any{"nodeId": "node-1"}},
		{server: "drive", tool: "get_star_list", args: map[string]any{
			"limit":                10,
			"sortType":             "desc",
			"supportResourceTypes": []string{"DENTRY"},
			"contentTypes":         []string{"doc", "sheet"},
		}},
	}
	if !reflect.DeepEqual(caller.calls, want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}
