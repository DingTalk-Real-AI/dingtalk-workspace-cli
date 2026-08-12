package helpers

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type depthArgsRecordingCaller struct {
	steps []scriptedToolStep
	index int
	calls []map[string]any
}

func (c *depthArgsRecordingCaller) CallTool(_ context.Context, _, _ string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, args)
	index := c.index
	if index >= len(c.steps) {
		index = len(c.steps) - 1
	}
	c.index++
	return textToolResult(c.steps[index].text), nil
}

func (*depthArgsRecordingCaller) Format() string { return "json" }
func (*depthArgsRecordingCaller) DryRun() bool   { return false }
func (*depthArgsRecordingCaller) Fields() string { return "" }
func (*depthArgsRecordingCaller) JQ() string     { return "" }

func executeDriveCommand(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newDriveCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageDriveListVersionsMode(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"list", "--versions", "--node", "node-1", "--limit", "5", "--cursor", "cur-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	call := caller.calls[0]
	if call.productID != "drive" || call.toolName != "list_file_versions" {
		t.Fatalf("call = %#v", call)
	}
	if call.args["nodeId"] != "node-1" || call.args["maxResults"] != 5 || call.args["nextCursor"] != "cur-1" {
		t.Fatalf("args = %#v", call.args)
	}
}

func TestCrossPlatformCoverageDriveListVersionsModeRequiresNode(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand, "list", "--versions")
	if err == nil {
		t.Fatal("versions without --node returned nil")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveListWorkspaceDepthRoutesDocBFS(t *testing.T) {
	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"nodes":[{"nodeId":"n1","name":"doc1","nodeType":"doc"}],"hasMore":false}`},
	}}
	err := executeDriveCommand(t, caller,
		"list", "--workspace", "ws-1", "--depth", "2", "--node", "folder-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want 1", caller.calls)
	}
	args := caller.calls[0]
	if args["workspaceId"] != "ws-1" || args["folderId"] != "folder-1" || args["pageSize"] != float64(docDepthPageSize) {
		t.Fatalf("doc BFS args = %#v", args)
	}
}

func TestCrossPlatformCoverageDriveListWorkspaceDepthRejectsNumericFolder(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"list", "--workspace", "ws-1", "--depth", "2", "--node", "12345")
	if err == nil {
		t.Fatal("numeric doc folder returned nil")
	}
}

func TestCrossPlatformCoverageDriveListPanDepthBuildsBaseArgs(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"f1","name":"a.txt","type":"FILE"}]}`},
	}}
	err := executeDriveCommand(t, caller,
		"list", "--depth", "2", "--space-id", "sp-1", "--order-by", "name", "--order", "asc", "--thumbnail", "--folder", "root-folder")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want 1", caller.calls)
	}
	args := caller.calls[0]
	if args["spaceId"] != "sp-1" || args["orderBy"] != "name" || args["order"] != "asc" ||
		args["withThumbnail"] != true || args["parentId"] != "root-folder" || args["maxResults"] != float64(driveDepthPageSize) {
		t.Fatalf("pan BFS args = %#v", args)
	}
}

func TestCrossPlatformCoverageDriveListPanDepthRejectsNumericFolder(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"list", "--depth", "2", "--folder", "12345")
	if err == nil {
		t.Fatal("numeric drive folder returned nil")
	}
}

func TestCrossPlatformCoverageDriveTransferOwnerDryRun(t *testing.T) {
	caller := &guardedMutationCaller{dryRun: true}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"permission", "transfer-owner", "--node", "node-1", "--new-owner", "user-1", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveTransferOwnerYesRequiresRecursive(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"permission", "transfer-owner", "--node", "node-1", "--new-owner", "user-1", "--reserve-role", "EDITOR", "--yes")
	if err == nil || !strings.Contains(err.Error(), "--recursive is required") {
		t.Fatalf("err = %v, want --recursive required", err)
	}
}

func TestCrossPlatformCoverageDriveTransferOwnerWorkspaceTarget(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"permission", "transfer-owner", "--workspace", "ws-1", "--new-owner", "user-1", "--reserve-role", "EDITOR", "--recursive", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	call := caller.calls[0]
	if call.productID != "doc" || call.toolName != "transfer_owner" {
		t.Fatalf("call = %#v", call)
	}
	if call.args["workspaceId"] != "ws-1" || call.args["newOwnerId"] != "user-1" ||
		call.args["reserveOldOwnerRole"] != "EDITOR" || call.args["recursiveChange"] != true {
		t.Fatalf("args = %#v", call.args)
	}
	if _, ok := call.args["nodeId"]; ok {
		t.Fatalf("unexpected nodeId in %#v", call.args)
	}
}

func TestCrossPlatformCoverageDriveTransferOwnerDeclined(t *testing.T) {
	caller := &guardedMutationCaller{}
	root := newDriveCommand()
	root.SetIn(strings.NewReader("no\n"))
	err := executeGuardedMutationCommand(t, caller, func() *cobra.Command { return root },
		"permission", "transfer-owner", "--node", "node-1", "--new-owner", "user-1")
	if err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
		t.Fatalf("declined transfer error = %v, want 用户取消了操作", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("declined calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveCoverRequiresNode(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand, "cover")
	if err == nil {
		t.Fatal("cover without --node returned nil")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveRevertDeclined(t *testing.T) {
	caller := &guardedMutationCaller{}
	root := newDriveCommand()
	root.SetIn(strings.NewReader("no\n"))
	err := executeGuardedMutationCommand(t, caller, func() *cobra.Command { return root },
		"revert", "--node", "node-1", "--version", "3")
	if err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
		t.Fatalf("declined revert error = %v, want 用户取消了操作", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("declined revert calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveTransferOwnerRejectsBothTargets(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"permission", "transfer-owner", "--node", "node-1", "--workspace", "ws-1", "--new-owner", "user-1", "--yes")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("err = %v, want mutual exclusion", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want none", caller.calls)
	}
}
