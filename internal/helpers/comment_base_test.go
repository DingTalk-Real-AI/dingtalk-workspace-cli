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

	"github.com/spf13/cobra"
)

func executeCommentBaseCommand(t *testing.T, caller *docCommentMutationCaller, surface string, args ...string) error {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	InitDeps(caller)
	deps.Out.w = io.Discard
	os.Args = []string{"dws", surface}
	var cmd *cobra.Command
	if surface == "sheet" {
		cmd = newSheetCommand()
	} else {
		cmd = newDocCommand()
	}
	cmd.PersistentFlags().Bool("yes", false, "skip confirmation")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageCommentBaseCommandsRegisteredForDocAndSheet(t *testing.T) {
	for _, surface := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "doc", cmd: newDocCommand()},
		{name: "sheet", cmd: newSheetCommand()},
	} {
		for _, leaf := range []string{"batch-query", "resolve", "restore", "react-reply"} {
			cmd, remaining, err := surface.cmd.Find([]string{"comment", leaf})
			if err != nil || len(remaining) != 0 {
				t.Fatalf("dws %s comment %s not registered: cmd=%v remaining=%v err=%v",
					surface.name, leaf, cmd, remaining, err)
			}
		}
	}
}

func TestCrossPlatformCoverageDocCommentBatchQueryMapsStructuredRefs(t *testing.T) {
	caller := &docCommentMutationCaller{}
	err := executeCommentBaseCommand(t, caller, "doc",
		"comment", "batch-query", "--node", "doc-1",
		"--comment-ref", "global:comment-1", "--comment-ref", "topic-2:comment-2")
	if err != nil {
		t.Fatal(err)
	}
	want := docCommentMutationCall{
		productID: commentServer,
		toolName:  "batch_query_comments",
		args: map[string]any{
			"nodeId": "doc-1",
			"comments": []map[string]any{
				{"topicId": "global", "commentKey": "comment-1"},
				{"topicId": "topic-2", "commentKey": "comment-2"},
			},
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestCrossPlatformCoverageSheetCommentResolveAndRestoreUseSharedRPCs(t *testing.T) {
	caller := &docCommentMutationCaller{}
	if err := executeCommentBaseCommand(t, caller, "sheet",
		"comment", "resolve", "--node", "sheet-1", "--comment-key", "comment-1"); err != nil {
		t.Fatal(err)
	}
	if err := executeCommentBaseCommand(t, caller, "sheet",
		"comment", "restore", "--node", "sheet-1", "--comment-key", "comment-1"); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if caller.calls[0].toolName != "resolve_comment" || caller.calls[1].toolName != "restore_comment" {
		t.Fatalf("tools = %q, %q", caller.calls[0].toolName, caller.calls[1].toolName)
	}
	wantArgs := map[string]any{"nodeId": "sheet-1", "commentKey": "comment-1"}
	if !reflect.DeepEqual(caller.calls[0].args, wantArgs) || !reflect.DeepEqual(caller.calls[1].args, wantArgs) {
		t.Fatalf("calls = %#v, want args %#v", caller.calls, wantArgs)
	}
}

func TestCrossPlatformCoverageCommentReactReplyForcesEmojiTrue(t *testing.T) {
	caller := &docCommentMutationCaller{}
	err := executeCommentBaseCommand(t, caller, "doc",
		"comment", "react-reply", "--file-id", "doc-1",
		"--comment-key", "comment-1", "--reaction", "鼓掌")
	if err != nil {
		t.Fatal(err)
	}
	want := docCommentMutationCall{
		productID: commentServer,
		toolName:  "reply_comment",
		args: map[string]any{
			"nodeId":          "doc-1",
			"replyCommentKey": "comment-1",
			"content":         "鼓掌",
			"emoji":           true,
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestCrossPlatformCoverageCommentReactReplyGuidesDingTalkEmojiNames(t *testing.T) {
	for _, surface := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "doc", cmd: newDocCommand()},
		{name: "sheet", cmd: newSheetCommand()},
	} {
		cmd, remaining, err := surface.cmd.Find([]string{"comment", "react-reply"})
		if err != nil || len(remaining) != 0 {
			t.Fatalf("dws %s comment react-reply lookup: remaining=%v err=%v", surface.name, remaining, err)
		}
		reaction := cmd.Flags().Lookup("reaction")
		if reaction == nil || !strings.Contains(reaction.Usage, "不是 Unicode Emoji") || !strings.Contains(reaction.Usage, "😄=憨笑") {
			t.Fatalf("dws %s reaction guidance = %#v", surface.name, reaction)
		}
		if !strings.Contains(cmd.Long, "不要直接传") || !strings.Contains(cmd.Example, `--reaction "憨笑"`) {
			t.Fatalf("dws %s react-reply help missing DingTalk emoji-name guidance", surface.name)
		}
	}
}

func TestCrossPlatformCoverageParseCommentRefsRejectsInvalidShapeAndLimit(t *testing.T) {
	if _, err := parseCommentRefs(nil); err == nil {
		t.Fatal("missing refs returned nil error")
	}
	if _, err := parseCommentRefs([]string{"missing-separator"}); err == nil {
		t.Fatal("invalid ref returned nil error")
	}
	tooMany := make([]string, 101)
	for index := range tooMany {
		tooMany[index] = "global:comment"
	}
	if _, err := parseCommentRefs(tooMany); err == nil {
		t.Fatal("101 refs returned nil error")
	}
}
