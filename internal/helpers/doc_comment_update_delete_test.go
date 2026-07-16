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
	"os"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type docCommentMutationCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type docCommentMutationCaller struct {
	calls []docCommentMutationCall
}

func (c *docCommentMutationCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, docCommentMutationCall{productID: productID, toolName: toolName, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*docCommentMutationCaller) Format() string { return "json" }
func (*docCommentMutationCaller) DryRun() bool   { return false }
func (*docCommentMutationCaller) Fields() string { return "" }
func (*docCommentMutationCaller) JQ() string     { return "" }

func executeDocCommentMutationCommand(t *testing.T, caller *docCommentMutationCaller, processArgs []string, args ...string) error {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	InitDeps(caller)
	deps.Out.w = io.Discard
	os.Args = processArgs
	cmd := newDocCommand()
	cmd.PersistentFlags().Bool("yes", false, "skip confirmation")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestDocCommentUpdateDeleteCommandsRegistered(t *testing.T) {
	root := newDocCommand()
	cases := []struct {
		path  string
		flags []string
	}{
		{path: "update", flags: []string{"node", "url", "id", "node-id", "doc-id", "file-id", "comment-key", "content", "mention"}},
		{path: "delete", flags: []string{"node", "url", "id", "node-id", "doc-id", "file-id", "comment-key"}},
	}
	for _, tc := range cases {
		cmd, remaining, err := root.Find([]string{"comment", tc.path})
		if err != nil || len(remaining) != 0 {
			t.Fatalf("dws doc comment %s not registered: cmd=%v remaining=%v err=%v", tc.path, cmd, remaining, err)
		}
		for _, flag := range tc.flags {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("dws doc comment %s missing flag --%s", tc.path, flag)
			}
		}
	}
}

func TestDocCommentUpdateMapsOpenToolArguments(t *testing.T) {
	caller := &docCommentMutationCaller{}
	err := executeDocCommentMutationCommand(t, caller, []string{"dws", "doc"},
		"comment", "update", "--file-id", "doc-1", "--comment-key", "comment-1",
		"--content", "updated", "--mention", "uid-1, ,uid-2")
	if err != nil {
		t.Fatalf("doc comment update returned error: %v", err)
	}
	want := docCommentMutationCall{
		productID: "doc-comment",
		toolName:  "update_comment",
		args: map[string]any{
			"nodeId":           "doc-1",
			"commentKey":       "comment-1",
			"content":          "updated",
			"mentionedUserIds": []string{"uid-1", "uid-2"},
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestDocCommentUpdateOmitsMentionWhenUnset(t *testing.T) {
	caller := &docCommentMutationCaller{}
	err := executeDocCommentMutationCommand(t, caller, []string{"dws", "doc"},
		"comment", "update", "--node", "doc-1", "--comment-key", "comment-1", "--content", "updated")
	if err != nil {
		t.Fatalf("doc comment update returned error: %v", err)
	}
	want := map[string]any{"nodeId": "doc-1", "commentKey": "comment-1", "content": "updated"}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("calls = %#v, want args %#v", caller.calls, want)
	}
}

func TestDocCommentMutationsRejectMissingRequiredFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "update node", args: []string{"comment", "update", "--comment-key", "comment-1", "--content", "updated"}},
		{name: "update comment key", args: []string{"comment", "update", "--node", "doc-1", "--content", "updated"}},
		{name: "update content", args: []string{"comment", "update", "--node", "doc-1", "--comment-key", "comment-1"}},
		{name: "delete node", args: []string{"comment", "delete", "--comment-key", "comment-1"}},
		{name: "delete comment key", args: []string{"comment", "delete", "--node", "doc-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &docCommentMutationCaller{}
			err := executeDocCommentMutationCommand(t, caller, []string{"dws", "doc", "--yes"}, tt.args...)
			if err == nil {
				t.Fatal("command with missing required flag returned nil error")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("remote calls = %d, want 0", len(caller.calls))
			}
		})
	}
}

func TestDocCommentDeleteMapsArgumentsAfterYesConfirmation(t *testing.T) {
	caller := &docCommentMutationCaller{}
	err := executeDocCommentMutationCommand(t, caller, []string{"dws", "doc"},
		"comment", "delete", "--id", "doc-1", "--comment-key", "comment-1", "--yes")
	if err != nil {
		t.Fatalf("doc comment delete returned error: %v", err)
	}
	want := docCommentMutationCall{
		productID: "doc-comment",
		toolName:  "delete_comment",
		args:      map[string]any{"nodeId": "doc-1", "commentKey": "comment-1"},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestDocCommentDeleteCancellationSkipsRemoteCall(t *testing.T) {
	previousStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := writer.WriteString("no\n"); err != nil {
		t.Fatalf("write confirmation: %v", err)
	}
	_ = writer.Close()
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = previousStdin
		_ = reader.Close()
	})

	caller := &docCommentMutationCaller{}
	err = executeDocCommentMutationCommand(t, caller, []string{"dws", "doc"},
		"comment", "delete", "--node", "doc-1", "--comment-key", "comment-1")
	if err != nil {
		t.Fatalf("cancelled doc comment delete returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote calls = %d, want 0 after cancellation", len(caller.calls))
	}
}
