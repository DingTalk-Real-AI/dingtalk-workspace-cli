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
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
)

func executeDriveCommentCommand(t *testing.T, caller *docCommentMutationCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	InitDeps(caller)
	deps.Out.w = io.Discard
	os.Args = []string{"dws", "drive"}
	cmd := newDriveCommand()
	if cmd.PersistentFlags().Lookup("yes") == nil {
		cmd.PersistentFlags().Bool("yes", false, "skip confirmation")
	}
	cmd.SetIn(strings.NewReader(""))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageDriveCommentRegistersLegacyAndAllTenNewCommands(t *testing.T) {
	group := newDriveFileCommentCmd()
	type wantCommand struct {
		identity string
		rpc      string
		legacy   bool
	}
	want := map[string]wantCommand{
		"list":         {identity: "list_file_comments", legacy: true},
		"create":       {identity: "create_file_comment", legacy: true},
		"list-v2":      {identity: "list_comments", rpc: "list_comments"},
		"create-v2":    {identity: "create_comment", rpc: "create_comment"},
		"reply":        {identity: "reply_comment", rpc: "reply_comment"},
		"update":       {identity: "update_comment", rpc: "update_comment"},
		"delete":       {identity: "delete_comment", rpc: "delete_comment"},
		"batch-query":  {identity: "batch_query_comments", rpc: "batch_query_comments"},
		"list-replies": {identity: "list_replies", rpc: "list_replies"},
		"resolve":      {identity: "resolve_comment", rpc: "resolve_comment"},
		"restore":      {identity: "restore_comment", rpc: "restore_comment"},
		"react-reply":  {identity: "react_reply", rpc: "reply_comment"},
	}
	if len(group.Commands()) != len(want) {
		t.Fatalf("drive comment command count = %d, want %d", len(group.Commands()), len(want))
	}
	for leaf, expected := range want {
		cmd, remaining, err := group.Find([]string{leaf})
		if err != nil || len(remaining) != 0 {
			t.Fatalf("drive comment %s lookup: cmd=%v remaining=%v err=%v", leaf, cmd, remaining, err)
		}
		final, ok := contractfinal.RuntimeContractFinal(cmd)
		if !ok || final.Identity == nil || final.Interface == nil {
			t.Fatalf("drive comment %s incomplete ContractFinal: %#v", leaf, final)
		}
		if final.Identity.ProductID != "drive" || final.Identity.Name != expected.identity ||
			final.Identity.PrimaryCLIPath != "drive comment "+leaf {
			t.Fatalf("drive comment %s identity = %#v", leaf, final.Identity)
		}
		if expected.legacy {
			if final.Interface.Mode != "composite" || final.Interface.Ref != nil || cmd.Deprecated == "" {
				t.Fatalf("drive comment %s historical contract: interface=%#v deprecated=%q", leaf, final.Interface, cmd.Deprecated)
			}
			continue
		}
		if final.Interface.Ref == nil || final.Interface.Ref.ProductID != commentServer ||
			final.Interface.Ref.RPCName != expected.rpc {
			t.Fatalf("drive comment %s interface = %#v, want %s/%s", leaf, final.Interface.Ref, commentServer, expected.rpc)
		}
		if leaf == "list-replies" || leaf == "batch-query" {
			if final.Selection == nil ||
				!strings.Contains(strings.Join(final.Selection.AvoidWhen, "\n"), "drive comment list-v2") {
				t.Fatalf("drive comment %s selection does not route commentKey lookup through list-v2: %#v", leaf, final.Selection)
			}
		}
	}
}

func TestCrossPlatformCoverageDriveCommentFirstFiveUseSharedNewCommentRPCs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		tool string
		want map[string]any
	}{
		{
			name: "list",
			args: []string{"comment", "list-v2", "--node", "file-1", "--limit", "20", "--cursor", "opaque-2", "--resolve-status", "unresolved"},
			tool: "list_comments",
			want: map[string]any{
				"nodeId": "file-1", "pageSize": 20, "nextToken": "opaque-2",
				"resolveStatus": "unresolved", "commentType": driveCommentGlobalTopic,
			},
		},
		{
			name: "create",
			args: []string{"comment", "create-v2", "--node", "file-1", "--content", "new root", "--yes"},
			tool: "create_comment",
			want: map[string]any{"nodeId": "file-1", "content": "new root"},
		},
		{
			name: "reply",
			args: []string{"comment", "reply", "--node", "file-1", "--comment-key", "comment-1", "--content", "reply"},
			tool: "reply_comment",
			want: map[string]any{
				"nodeId": "file-1", "replyCommentKey": "comment-1", "content": "reply", "emoji": false,
			},
		},
		{
			name: "update",
			args: []string{"comment", "update", "--node", "file-1", "--comment-key", "comment-1", "--content", "updated"},
			tool: "update_comment",
			want: map[string]any{"nodeId": "file-1", "commentKey": "comment-1", "content": "updated"},
		},
		{
			name: "delete",
			args: []string{"comment", "delete", "--node", "file-1", "--comment-key", "comment-1", "--yes"},
			tool: "delete_comment",
			want: map[string]any{"nodeId": "file-1", "commentKey": "comment-1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCommentMutationCaller{}
			if err := executeDriveCommentCommand(t, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v", caller.calls)
			}
			call := caller.calls[0]
			if call.productID != commentServer || call.toolName != tc.tool || !reflect.DeepEqual(call.args, tc.want) {
				t.Fatalf("call = %#v, want server=%q tool=%q args=%#v", call, commentServer, tc.tool, tc.want)
			}
			if call.toolName == "list_file_comments" || call.toolName == "create_file_comment" {
				t.Fatalf("new Drive command called legacy tool: %#v", call)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveCommentV2UsesExplicitPageLimit(t *testing.T) {
	group := newDriveFileCommentCmd()
	listCmd, _, _ := group.Find([]string{"list-v2"})
	if listCmd.Flags().Lookup("all") != nil || listCmd.Flags().Lookup("scope") != nil ||
		listCmd.Flags().Lookup("space-id") != nil {
		t.Fatalf("new list exposes legacy-only flags")
	}

	caller := &docCommentMutationCaller{}
	if err := executeDriveCommentCommand(t, caller,
		"comment", "list-v2", "--node", "file-1", "--limit", "51"); err == nil ||
		!strings.Contains(err.Error(), "1-50") {
		t.Fatalf("oversized v2 limit error = %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("oversized v2 limit reached MCP: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveCommentDeleteRequiresConfirmation(t *testing.T) {
	caller := &docCommentMutationCaller{}
	err := executeDriveCommentCommand(t, caller,
		"comment", "delete", "--node", "file-1", "--comment-key", "comment-1")
	if err == nil || !strings.Contains(err.Error(), "用户确认") {
		t.Fatalf("delete without confirmation error = %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("delete reached MCP before confirmation: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveCommentCreateRequiresExplicitConfirmation(t *testing.T) {
	unconfirmed := &docCommentMutationCaller{}
	err := executeDriveCommentCommand(t, unconfirmed,
		"comment", "create-v2", "--node", "file-1", "--content", "new root")
	if err == nil || !strings.Contains(err.Error(), "用户确认") {
		t.Fatalf("create without confirmation error = %v", err)
	}
	if len(unconfirmed.calls) != 0 {
		t.Fatalf("create reached MCP before confirmation: %#v", unconfirmed.calls)
	}

	confirmed := &docCommentMutationCaller{}
	if err := executeDriveCommentCommand(t, confirmed,
		"comment", "create-v2", "--node", "file-1", "--content", "new root", "--yes"); err != nil {
		t.Fatal(err)
	}
	want := docCommentMutationCall{
		productID: commentServer,
		toolName:  "create_comment",
		args:      map[string]any{"nodeId": "file-1", "content": "new root"},
	}
	if len(confirmed.calls) != 1 || !reflect.DeepEqual(confirmed.calls[0], want) {
		t.Fatalf("confirmed create calls = %#v, want %#v", confirmed.calls, want)
	}
}

func TestCrossPlatformCoverageDriveCommentRejectsBlankV2Content(t *testing.T) {
	caller := &docCommentMutationCaller{}
	err := executeDriveCommentCommand(t, caller,
		"comment", "create-v2", "--node", "file-1", "--content", " \t ", "--yes")
	if err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("blank v2 content error = %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("blank v2 content reached MCP: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveLegacyDeprecatedHelpStaysOnStdout(t *testing.T) {
	caller := &fileCommentTestCaller{}
	stdout, err := executeFileCommentCommand(t, caller, "", "drive", "comment", "list", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stdout), "--all") || strings.Contains(string(stdout), "is deprecated") {
		t.Fatalf("legacy help stdout = %q", stdout)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("legacy help reached MCP: %#v", caller.calls)
	}
}
