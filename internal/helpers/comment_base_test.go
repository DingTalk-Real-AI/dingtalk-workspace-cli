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
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type commentRepliesCaller struct {
	responses []string
	calls     []docCommentMutationCall
	dryRun    bool
}

func (c *commentRepliesCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for key, value := range args {
		copied[key] = value
	}
	c.calls = append(c.calls, docCommentMutationCall{productID: productID, toolName: toolName, args: copied})
	response := `{"result":{"scope":"DIRECT","replyList":[],"complete":true,"scannedCount":0,"stopReason":"END"}}`
	if index := len(c.calls) - 1; index < len(c.responses) {
		response = c.responses[index]
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: response}}}, nil
}

func (*commentRepliesCaller) Format() string { return "json" }
func (c *commentRepliesCaller) DryRun() bool { return c.dryRun }
func (*commentRepliesCaller) Fields() string { return "" }
func (*commentRepliesCaller) JQ() string     { return "" }

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
	switch surface {
	case "sheet":
		cmd = newSheetCommand()
	case "drive":
		cmd = newDriveCommand()
	default:
		cmd = newDocCommand()
	}
	cmd.PersistentFlags().Bool("yes", false, "skip confirmation")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

func executeCommentRepliesCommand(t *testing.T, caller *commentRepliesCaller, surface string, args ...string) (map[string]any, error) {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	defer func() {
		deps = previousDeps
		os.Args = previousArgs
	}()

	InitDeps(caller)
	deps.Out.w = io.Discard
	os.Args = []string{"dws", surface}
	var cmd *cobra.Command
	switch surface {
	case "sheet":
		cmd = newSheetCommand()
	case "drive":
		cmd = newDriveCommand()
	default:
		cmd = newDocCommand()
	}
	cmd.PersistentFlags().Bool("yes", false, "skip confirmation")
	cmd.PersistentFlags().String("format", "json", "output format")
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(ctx)
	if stdout.Len() == 0 {
		return nil, err
	}
	var payload map[string]any
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("stdout is not JSON: %q: %v", stdout.String(), decodeErr)
	}
	return payload, err
}

func TestCrossPlatformCoverageCommentBaseCommandsRegisteredForDocSheetAndDrive(t *testing.T) {
	for _, surface := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "doc", cmd: newDocCommand()},
		{name: "sheet", cmd: newSheetCommand()},
		{name: "drive", cmd: newDriveCommand()},
	} {
		for _, leaf := range []string{"batch-query", "resolve", "restore", "react-reply", "list-replies"} {
			cmd, remaining, err := surface.cmd.Find([]string{"comment", leaf})
			if err != nil || len(remaining) != 0 {
				t.Fatalf("dws %s comment %s not registered: cmd=%v remaining=%v err=%v",
					surface.name, leaf, cmd, remaining, err)
			}
		}
	}
}

func TestCrossPlatformCoverageCommentListRepliesProjectsTwoPages(t *testing.T) {
	caller := &commentRepliesCaller{responses: []string{
		`{"result":{"scope":"DIRECT","topicId":"global","commentKey":"root-1","replyList":[{"commentKey":"reply-1","replyToCommentKey":"root-1","content":"first","creatorId":"u-1","createTime":100,"updateTime":101,"isEmoji":false}],"complete":false,"nextPageToken":"cursor-2","scannedCount":300,"stopReason":"SCAN_LIMIT"}}`,
		`{"result":{"scope":"DIRECT","topicId":"global","commentKey":"root-1","replyList":[{"commentKey":"reply-2","replyToCommentKey":"root-1","content":"鼓掌","isEmoji":true}],"complete":true,"scannedCount":17,"stopReason":"END"}}`,
	}}

	first, err := executeCommentRepliesCommand(t, caller, "doc",
		"comment", "list-replies", "--node", "doc-1", "--topic-id", "global",
		"--comment-key", "root-1", "--page-size", "1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := executeCommentRepliesCommand(t, caller, "sheet",
		"comment", "list-replies", "--node", "sheet-1", "--topic-id", "global",
		"--comment-key", "root-1", "--page-token", "cursor-2", "--page-size", "1")
	if err != nil {
		t.Fatal(err)
	}

	wantCalls := []docCommentMutationCall{
		{productID: commentServer, toolName: commentListRepliesTool, args: map[string]any{
			"nodeId": "doc-1", "topicId": "global", "commentKey": "root-1", "pageSize": 1,
		}},
		{productID: commentServer, toolName: commentListRepliesTool, args: map[string]any{
			"nodeId": "sheet-1", "topicId": "global", "commentKey": "root-1", "pageSize": 1, "pageToken": "cursor-2",
		}},
	}
	if !reflect.DeepEqual(caller.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, wantCalls)
	}

	firstData := first["data"].(map[string]any)
	if firstData["scope"] != "DIRECT" || firstData["complete"] != false ||
		firstData["scannedCount"] != float64(300) || firstData["stopReason"] != "SCAN_LIMIT" {
		t.Fatalf("first data = %#v", firstData)
	}
	firstPagination := first["meta"].(map[string]any)["pagination"].(map[string]any)
	if firstPagination["endpoint_exhausted"] != false || firstPagination["next_token"] != "cursor-2" {
		t.Fatalf("first pagination = %#v", firstPagination)
	}

	secondData := second["data"].(map[string]any)
	secondReplies := secondData["replies"].([]any)
	if len(secondReplies) != 1 || secondReplies[0].(map[string]any)["isEmoji"] != true {
		t.Fatalf("second replies = %#v", secondReplies)
	}
	secondPagination := second["meta"].(map[string]any)["pagination"].(map[string]any)
	if secondPagination["endpoint_exhausted"] != true {
		t.Fatalf("second pagination = %#v", secondPagination)
	}
}

func TestCrossPlatformCoverageCommentListRepliesDryRunAndNullReplyList(t *testing.T) {
	dryRunCaller := &commentRepliesCaller{dryRun: true}
	dryRun, err := executeCommentRepliesCommand(t, dryRunCaller, "doc",
		"comment", "list-replies", "--node", "doc-1", "--topic-id", "global",
		"--comment-key", "root-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(dryRunCaller.calls) != 0 {
		t.Fatalf("dry-run reached MCP: %#v", dryRunCaller.calls)
	}
	dryRunData := dryRun["data"].(map[string]any)
	if dryRunData["scope"] != "DIRECT" || dryRunData["complete"] != true ||
		dryRunData["scannedCount"] != float64(0) {
		t.Fatalf("dry-run data = %#v", dryRunData)
	}

	nullCaller := &commentRepliesCaller{responses: []string{
		`{"result":{"scope":"DIRECT","replyList":null,"complete":true,"scannedCount":0,"stopReason":"END"}}`,
	}}
	nullList, err := executeCommentRepliesCommand(t, nullCaller, "sheet",
		"comment", "list-replies", "--node", "sheet-1", "--topic-id", "global",
		"--comment-key", "root-1")
	if err != nil {
		t.Fatal(err)
	}
	nullData := nullList["data"].(map[string]any)
	if replies, ok := nullData["replies"].([]any); !ok || len(replies) != 0 {
		t.Fatalf("null replyList projection = %#v", nullData["replies"])
	}
}

func TestCrossPlatformCoverageCommentListRepliesRejectsMalformedResponses(t *testing.T) {
	tests := []struct {
		name     string
		response string
		extra    []string
		message  string
	}{
		{
			name:     "missing complete",
			response: `{"scope":"DIRECT","replyList":[],"scannedCount":0,"stopReason":"END"}`,
			message:  "complete",
		},
		{
			name:     "missing reply list",
			response: `{"scope":"DIRECT","complete":true,"scannedCount":0,"stopReason":"END"}`,
			message:  "replyList",
		},
		{
			name:     "reply list is not array",
			response: `{"scope":"DIRECT","replyList":{},"complete":true,"scannedCount":0,"stopReason":"END"}`,
			message:  "回复列表不是数组",
		},
		{
			name:     "reply item is not object",
			response: `{"scope":"DIRECT","replyList":[1],"complete":true,"scannedCount":0,"stopReason":"END"}`,
			message:  "不是对象",
		},
		{
			name:     "reply missing comment key",
			response: `{"scope":"DIRECT","replyList":[{"replyToCommentKey":"root-1","isEmoji":false}],"complete":true,"scannedCount":0,"stopReason":"END"}`,
			message:  "commentKey",
		},
		{
			name:     "reply missing parent key",
			response: `{"scope":"DIRECT","replyList":[{"commentKey":"reply-1","isEmoji":false}],"complete":true,"scannedCount":0,"stopReason":"END"}`,
			message:  "replyToCommentKey",
		},
		{
			name:     "reply missing emoji marker",
			response: `{"scope":"DIRECT","replyList":[{"commentKey":"reply-1","replyToCommentKey":"root-1"}],"complete":true,"scannedCount":0,"stopReason":"END"}`,
			message:  "isEmoji",
		},
		{
			name:     "next token is not string",
			response: `{"scope":"DIRECT","replyList":[],"complete":false,"nextPageToken":1,"scannedCount":0,"stopReason":"PAGE_SIZE"}`,
			message:  "nextPageToken",
		},
		{
			name:     "missing next token",
			response: `{"scope":"DIRECT","replyList":[],"complete":false,"scannedCount":0,"stopReason":"PAGE_SIZE"}`,
			message:  "没有 nextPageToken",
		},
		{
			name:     "cursor does not advance",
			response: `{"scope":"DIRECT","replyList":[],"complete":false,"nextPageToken":"same","scannedCount":0,"stopReason":"PAGE_SIZE"}`,
			extra:    []string{"--page-token", "same"},
			message:  "游标未前进",
		},
		{
			name:     "negative scanned count",
			response: `{"scope":"DIRECT","replyList":[],"complete":true,"scannedCount":-1,"stopReason":"END"}`,
			message:  "scannedCount",
		},
		{
			name:     "invalid stop reason",
			response: `{"scope":"DIRECT","replyList":[],"complete":true,"scannedCount":0,"stopReason":"BAD"}`,
			message:  "stopReason",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &commentRepliesCaller{responses: []string{tc.response}}
			args := []string{
				"comment", "list-replies", "--node", "doc-1", "--topic-id", "global",
				"--comment-key", "root-1",
			}
			args = append(args, tc.extra...)
			payload, err := executeCommentRepliesCommand(t, caller, "doc", args...)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error = %v, want message containing %q; payload=%#v", err, tc.message, payload)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("MCP calls = %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageCommentListRepliesPublishesSchemaContract(t *testing.T) {
	for _, surface := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "doc", cmd: newDocCommand()},
		{name: "sheet", cmd: newSheetCommand()},
		{name: "drive", cmd: newDriveCommand()},
	} {
		leaf, remaining, err := surface.cmd.Find([]string{"comment", "list-replies"})
		if err != nil || len(remaining) != 0 {
			t.Fatalf("%s list-replies lookup: remaining=%v err=%v", surface.name, remaining, err)
		}
		final, ok := contractfinal.RuntimeContractFinal(leaf)
		if !ok || final.Identity == nil || final.Interface == nil || final.Result == nil || final.Pagination == nil {
			t.Fatalf("%s list-replies incomplete ContractFinal: %#v", surface.name, final)
		}
		if final.Identity.CanonicalPath != surface.name+".list_replies" ||
			final.Interface.Ref == nil || final.Interface.Ref.ProductID != commentServer || final.Interface.Ref.RPCName != commentListRepliesTool {
			t.Fatalf("%s identity/interface = %#v", surface.name, final)
		}
		if final.Pagination.Kind != contract.PaginationKindCursor || final.Pagination.CursorParameter != "page-token" ||
			final.Pagination.MetaPath != contract.PaginationMetaPath {
			t.Fatalf("%s pagination = %#v", surface.name, final.Pagination)
		}
		if output.CommandRollout(leaf) != output.RolloutUnifiedActive {
			t.Fatalf("%s rollout = %s", surface.name, output.CommandRollout(leaf))
		}
	}
}

func TestCrossPlatformCoverageDriveCommentListRepliesInjectsGlobalTopic(t *testing.T) {
	caller := &commentRepliesCaller{responses: []string{
		`{"result":{"scope":"DIRECT","topicId":"global","commentKey":"root-1","replyList":[],"complete":true,"scannedCount":0,"stopReason":"END"}}`,
	}}
	payload, err := executeCommentRepliesCommand(t, caller, "drive",
		"comment", "list-replies", "--node", "file-1",
		"--comment-key", "root-1", "--page-size", "20")
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := map[string]any{
		"nodeId": "file-1", "topicId": driveCommentGlobalTopic,
		"commentKey": "root-1", "pageSize": 20,
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != commentListRepliesTool ||
		!reflect.DeepEqual(caller.calls[0].args, wantArgs) {
		t.Fatalf("calls = %#v, want args %#v", caller.calls, wantArgs)
	}
	data := payload["data"].(map[string]any)
	if data["topicId"] != driveCommentGlobalTopic {
		t.Fatalf("Drive list-replies topicId = %#v", data["topicId"])
	}
	leaf, _, _ := newDriveCommand().Find([]string{"comment", "list-replies"})
	if leaf.Flags().Lookup("topic-id") != nil {
		t.Fatal("Drive list-replies unexpectedly exposes --topic-id")
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

func TestCrossPlatformCoverageDriveCommentBatchQueryInjectsGlobalTopic(t *testing.T) {
	caller := &docCommentMutationCaller{}
	err := executeCommentBaseCommand(t, caller, "drive",
		"comment", "batch-query", "--node", "file-1",
		"--comment-key", "comment-1", "--comment-key", "comment-2")
	if err != nil {
		t.Fatal(err)
	}
	want := docCommentMutationCall{
		productID: commentServer,
		toolName:  "batch_query_comments",
		args: map[string]any{
			"nodeId": "file-1",
			"comments": []map[string]any{
				{"topicId": driveCommentGlobalTopic, "commentKey": "comment-1"},
				{"topicId": driveCommentGlobalTopic, "commentKey": "comment-2"},
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

func TestCrossPlatformCoverageCommentReactionsRejectUnsupportedValuesBeforeRPC(t *testing.T) {
	for _, surface := range []string{"doc", "sheet"} {
		for _, test := range []struct {
			name string
			args []string
		}{
			{name: "react unicode", args: []string{"comment", "react-reply", "--node", "node-1", "--comment-key", "comment-1", "--reaction", "😄"}},
			{name: "react garbage", args: []string{"comment", "react-reply", "--node", "node-1", "--comment-key", "comment-1", "--reaction", "乱码"}},
			{name: "reply unicode", args: []string{"comment", "reply", "--node", "node-1", "--comment-key", "comment-1", "--content", "👏", "--emoji"}},
		} {
			t.Run(surface+"/"+test.name, func(t *testing.T) {
				caller := &docCommentMutationCaller{}
				if err := executeCommentBaseCommand(t, caller, surface, test.args...); err == nil {
					t.Fatal("unsupported reaction accepted")
				}
				if len(caller.calls) != 0 {
					t.Fatalf("unsupported reaction reached RPC: %#v", caller.calls)
				}
			})
		}
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

func TestCrossPlatformCoverageParseDriveCommentKeysRejectsEmptyInput(t *testing.T) {
	if _, err := parseDriveCommentKeys(nil); err == nil {
		t.Fatal("missing Drive comment keys returned nil error")
	}
	if _, err := parseDriveCommentKeys([]string{"  "}); err == nil {
		t.Fatal("blank Drive comment key returned nil error")
	}
}
