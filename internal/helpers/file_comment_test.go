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
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type fileCommentTestCall struct {
	server string
	tool   string
	args   map[string]any
}

type fileCommentTestCaller struct {
	calls     []fileCommentTestCall
	responses []string
	err       error
	dryRun    bool
}

func (c *fileCommentTestCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for key, value := range args {
		copied[key] = value
	}
	c.calls = append(c.calls, fileCommentTestCall{server: server, tool: tool, args: copied})
	if c.err != nil {
		return nil, c.err
	}
	response := `{"result":{"fileId":"file-1","comment":{"commentId":"1","content":"ok","anchor":{"version":"v1","surface":"file","scope":"whole"}}}}`
	if len(c.responses) > 0 {
		response = c.responses[0]
		c.responses = c.responses[1:]
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: response}}}, nil
}

func (c *fileCommentTestCaller) Format() string { return "json" }
func (c *fileCommentTestCaller) DryRun() bool   { return c.dryRun }
func (c *fileCommentTestCaller) Fields() string { return "" }
func (c *fileCommentTestCaller) JQ() string     { return "" }

func executeFileCommentCommand(t *testing.T, caller *fileCommentTestCaller, stdin string, args ...string) ([]byte, error) {
	t.Helper()
	testseam.Protect(t, &deps)
	InitDeps(caller)
	var stdout bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = io.Discard

	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetIn(strings.NewReader(stdin))
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	drive := &cobra.Command{Use: "drive"}
	drive.AddCommand(newDriveFileCommentCmd())
	root.AddCommand(drive)
	var setOutput func(*cobra.Command)
	setOutput = func(command *cobra.Command) {
		command.SetOut(&stdout)
		command.SetErr(io.Discard)
		for _, child := range command.Commands() {
			setOutput(child)
		}
	}
	setOutput(root)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.Bytes(), err
}

func decodeFileCommentTestOutput(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode output %q: %v", raw, err)
	}
	return payload
}

func TestDriveFileCommentListMapsFiltersAndProjects(t *testing.T) {
	caller := &fileCommentTestCaller{responses: []string{`{
		"result": {
			"fileId": "resolved-file",
			"total": 2,
			"count": 2,
			"hasMore": false,
			"nextToken": null,
			"items": [
				{
					"commentId": "101",
					"parentCommentId": null,
					"content": "whole",
					"creatorId": "user-1",
					"creatorName": "Alice",
					"creatorAvatar": "https://avatar/1",
					"createdAt": 1785920000000,
					"updatedAt": 1785920000100,
					"commentCustomType": "common",
					"anchor": {"version":"v1","surface":"file","scope":"whole"}
				},
				{
					"commentId": "102",
					"content": "partial",
					"commentCustomType": "highlight",
					"options": {"page":"1"},
					"anchor": {"version":"v1","surface":"file","scope":"partial","selector":{"kind":"legacy-highlight"}}
				}
			]
		}
	}`}}

	out, err := executeFileCommentCommand(t, caller, "",
		"drive", "comment", "list",
		"--node", "https://alidocs.dingtalk.com/i/drive/file",
		"--space-id", "123", "--limit", "20", "--scope", "whole",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	call := caller.calls[0]
	if call.server != fileCommentServer || call.tool != listFileCommentsTool {
		t.Fatalf("target = %s/%s", call.server, call.tool)
	}
	wantArgs := map[string]any{
		"fileId":     "https://alidocs.dingtalk.com/i/drive/file",
		"spaceId":    "123",
		"maxResults": 20,
	}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", call.args, wantArgs)
	}

	payload := decodeFileCommentTestOutput(t, out)
	if payload["nodeId"] != "resolved-file" || payload["total"] != float64(2) ||
		payload["count"] != float64(1) || payload["complete"] != true || payload["scope"] != "whole" {
		t.Fatalf("list output = %#v", payload)
	}
	if payload["nextCursor"] != nil || payload["hasMore"] != false {
		t.Fatalf("pagination output = %#v", payload)
	}
	comments := payload["comments"].([]any)
	comment := comments[0].(map[string]any)
	if comment["commentId"] != "101" || comment["customType"] != "common" {
		t.Fatalf("comment projection = %#v", comment)
	}
	creator := comment["creator"].(map[string]any)
	if creator["userId"] != "user-1" || creator["name"] != "Alice" || creator["avatar"] != "https://avatar/1" {
		t.Fatalf("creator projection = %#v", creator)
	}
	if _, leaked := comment["creatorId"]; leaked {
		t.Fatalf("raw creator fields leaked: %#v", comment)
	}
}

func TestDriveFileCommentListKeepsEmptyCommentsAsArray(t *testing.T) {
	caller := &fileCommentTestCaller{responses: []string{`{
		"fileId":"file-1","total":0,"count":0,"hasMore":false,"nextToken":null,"items":[]
	}`}}

	out, err := executeFileCommentCommand(t, caller, "",
		"drive", "comment", "list", "--node", "file-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeFileCommentTestOutput(t, out)
	comments, ok := payload["comments"].([]any)
	if !ok || len(comments) != 0 {
		t.Fatalf("comments = %#v, want an empty JSON array", payload["comments"])
	}
	if payload["total"] != float64(0) || payload["count"] != float64(0) ||
		payload["hasMore"] != false || payload["complete"] != true || payload["nextCursor"] != nil {
		t.Fatalf("empty list output = %#v", payload)
	}
}

func TestDriveFileCommentListAllAggregatesPages(t *testing.T) {
	caller := &fileCommentTestCaller{responses: []string{
		`{"fileId":"file-1","total":3,"count":2,"hasMore":true,"nextToken":"2","items":[
			{"commentId":"1","anchor":{"scope":"whole"}},
			{"commentId":"2","anchor":{"scope":"partial"}}
		]}`,
		`{"fileId":"file-1","total":3,"count":1,"hasMore":false,"nextToken":null,"items":[
			{"commentId":"3","anchor":{"scope":"partial"}}
		]}`,
	}}

	out, err := executeFileCommentCommand(t, caller, "",
		"drive", "comment", "list", "--node", "file-1", "--all", "--scope", "partial",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if caller.calls[0].args["maxResults"] != fileCommentMaxPageSize {
		t.Fatalf("first args = %#v", caller.calls[0].args)
	}
	if _, ok := caller.calls[0].args["nextToken"]; ok {
		t.Fatalf("first page unexpectedly has cursor: %#v", caller.calls[0].args)
	}
	if caller.calls[1].args["nextToken"] != "2" {
		t.Fatalf("second args = %#v", caller.calls[1].args)
	}
	for _, call := range caller.calls {
		if _, ok := call.args["all"]; ok {
			t.Fatalf("local --all leaked to MCP: %#v", call.args)
		}
		if _, ok := call.args["scope"]; ok {
			t.Fatalf("local --scope leaked to MCP: %#v", call.args)
		}
	}

	payload := decodeFileCommentTestOutput(t, out)
	if payload["total"] != float64(3) || payload["count"] != float64(2) ||
		payload["hasMore"] != false || payload["complete"] != true || payload["nextCursor"] != nil {
		t.Fatalf("aggregate output = %#v", payload)
	}
	comments := payload["comments"].([]any)
	if comments[0].(map[string]any)["commentId"] != "2" || comments[1].(map[string]any)["commentId"] != "3" {
		t.Fatalf("scope-filtered comments = %#v", comments)
	}
}

func TestDriveFileCommentListRejectsPaginationAnomalies(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		response string
		message  string
	}{
		{
			name:     "missing cursor",
			args:     []string{"drive", "comment", "list", "--node", "file-1", "--all"},
			response: `{"fileId":"file-1","total":1,"count":1,"hasMore":true,"nextToken":null,"items":[]}`,
			message:  "nextCursor 为空",
		},
		{
			name:     "stalled cursor",
			args:     []string{"drive", "comment", "list", "--node", "file-1", "--cursor", "2"},
			response: `{"fileId":"file-1","total":2,"count":1,"hasMore":true,"nextToken":"2","items":[]}`,
			message:  "游标未前进",
		},
		{
			name:     "invalid server cursor",
			args:     []string{"drive", "comment", "list", "--node", "file-1"},
			response: `{"fileId":"file-1","total":2,"count":1,"hasMore":true,"nextToken":"next","items":[]}`,
			message:  "不是非负数字游标",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &fileCommentTestCaller{responses: []string{tt.response}}
			_, err := executeFileCommentCommand(t, caller, "", tt.args...)
			var cliErr *CLIError
			if !errors.As(err, &cliErr) || cliErr.Code != CodeContentTruncated || !strings.Contains(cliErr.Message, tt.message) {
				t.Fatalf("error = %#v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v", caller.calls)
			}
		})
	}
}

func TestDriveFileCommentListValidatesLocalParameters(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"all with cursor", []string{"--all", "--cursor", "1"}},
		{"all with limit", []string{"--all", "--limit", "20"}},
		{"zero limit", []string{"--limit", "0"}},
		{"large limit", []string{"--page-size", "201"}},
		{"nonnumeric cursor", []string{"--cursor", "next"}},
		{"overflow cursor", []string{"--cursor", "999999999999999999999999999999"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &fileCommentTestCaller{}
			args := []string{"drive", "comment", "list", "--node", "file-1"}
			args = append(args, tt.args...)
			if _, err := executeFileCommentCommand(t, caller, "", args...); err == nil {
				t.Fatal("invalid arguments unexpectedly succeeded")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid arguments reached MCP: %#v", caller.calls)
			}
		})
	}
}

func TestDriveFileCommentNumericNodeRequiresSpaceIDForListAndCreateAliases(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "list id alias",
			args: []string{"drive", "comment", "list", "--id", "231773999335"},
		},
		{
			name: "create file-id alias",
			args: []string{"drive", "comment", "create", "--file-id", "231773999335", "--content", "test", "--yes"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &fileCommentTestCaller{}
			_, err := executeFileCommentCommand(t, caller, "", tt.args...)
			var cliErr *CLIError
			if !errors.As(err, &cliErr) || cliErr.Code != CodeInvalidParam || !strings.Contains(cliErr.Message, "--space-id") {
				t.Fatalf("error = %#v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("numeric node without space-id reached MCP: %#v", caller.calls)
			}
		})
	}

	caller := &fileCommentTestCaller{responses: []string{`{
		"fileId":"resolved-file","total":0,"count":0,"hasMore":false,"nextToken":null,"items":[]
	}`}}
	if _, err := executeFileCommentCommand(t, caller, "",
		"drive", "comment", "list", "--url", "231773999335", "--space-id", "2402756201",
	); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].args["fileId"] != "231773999335" || caller.calls[0].args["spaceId"] != "2402756201" {
		t.Fatalf("numeric node with space-id args = %#v", caller.calls)
	}
	if allASCIIDigits("") {
		t.Fatal("empty string unexpectedly accepted as numeric")
	}
}

func TestDriveFileCommentCreateValidatesMapsAndProjects(t *testing.T) {
	validContent := strings.Repeat("a", fileCommentMaxContentLength)
	caller := &fileCommentTestCaller{responses: []string{`{"result":{"fileId":"resolved-file","comment":{
		"commentId":"912345","parentCommentId":null,"content":"created","creatorId":"user-1",
		"createdAt":1785920000000,"commentCustomType":"common",
		"anchor":{"version":"v1","surface":"file","scope":"whole","selector":null}
	}}}`}}
	out, err := executeFileCommentCommand(t, caller, "",
		"drive", "comment", "create", "--node", "file-1", "--space-id", "123",
		"--content", validContent, "--yes",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].server != fileCommentServer || caller.calls[0].tool != createFileCommentTool {
		t.Fatalf("calls = %#v", caller.calls)
	}
	wantArgs := map[string]any{"fileId": "file-1", "spaceId": "123", "content": validContent}
	if !reflect.DeepEqual(caller.calls[0].args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", caller.calls[0].args, wantArgs)
	}
	payload := decodeFileCommentTestOutput(t, out)
	if payload["nodeId"] != "resolved-file" || payload["commentId"] != "912345" ||
		payload["content"] != "created" || payload["customType"] != "common" {
		t.Fatalf("create output = %#v", payload)
	}
	if _, nested := payload["comment"]; nested {
		t.Fatalf("create output was not flattened: %#v", payload)
	}

	for _, tc := range []struct {
		name    string
		content string
	}{
		{"blank", "  \t"},
		{"ascii over limit", strings.Repeat("a", fileCommentMaxContentLength+1)},
		{"utf16 over limit", strings.Repeat("😀", 1050)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalidCaller := &fileCommentTestCaller{}
			_, err := executeFileCommentCommand(t, invalidCaller, "",
				"drive", "comment", "create", "--node", "file-1", "--content", tc.content, "--yes",
			)
			if err == nil {
				t.Fatal("invalid content unexpectedly succeeded")
			}
			if len(invalidCaller.calls) != 0 {
				t.Fatalf("invalid content reached MCP: %#v", invalidCaller.calls)
			}
		})
	}
}

func TestDriveFileCommentCreatePublishesAndEnforcesConfirmation(t *testing.T) {
	group := newDriveFileCommentCmd()
	create, remaining, err := group.Find([]string{"create"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("find create: remaining=%v err=%v", remaining, err)
	}
	final, ok := contractfinal.RuntimeContractFinal(create)
	if !ok || final.Safety == nil {
		t.Fatal("create command is missing ContractFinal Safety")
	}
	if safety := *final.Safety; safety.Effect != "write" || safety.Risk != "medium" ||
		safety.Confirmation != "user_required" || safety.Idempotency != "unknown" {
		t.Fatalf("create safety = %#v", safety)
	}

	caller := &fileCommentTestCaller{}
	_, err = executeFileCommentCommand(t, caller, "",
		"drive", "comment", "create", "--node", "file-1", "--content", "需要确认",
	)
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "confirmation_required" {
		t.Fatalf("closed-stdin error = %#v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("unconfirmed create reached MCP: %#v", caller.calls)
	}
}

func TestDriveFileCommentRejectsMalformedResponseInsteadOfReturningEmpty(t *testing.T) {
	caller := &fileCommentTestCaller{responses: []string{`{"result":{"fileId":"file-1","total":0,"hasMore":false}}`}}
	_, err := executeFileCommentCommand(t, caller, "", "drive", "comment", "list", "--node", "file-1")
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != CodeMCPToolError || !strings.Contains(cliErr.Message, "缺少 items") {
		t.Fatalf("error = %#v", err)
	}
}

func TestDriveFileCommentDryRunAndCallErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		tool string
	}{
		{
			name: "list dry run",
			args: []string{"drive", "comment", "list", "--node", "file-1"},
			tool: listFileCommentsTool,
		},
		{
			name: "create dry run",
			args: []string{"drive", "comment", "create", "--node", "file-1", "--content", "test", "--yes"},
			tool: createFileCommentTool,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller := &fileCommentTestCaller{dryRun: true}
			out, err := executeFileCommentCommand(t, caller, "", tt.args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 0 || !strings.Contains(string(out), `"tool": "`+tt.tool+`"`) {
				t.Fatalf("calls = %#v, output = %s", caller.calls, out)
			}
		})
	}

	sentinel := errors.New("mcp unavailable")
	for _, tt := range []struct {
		name string
		args []string
	}{
		{
			name: "list call error",
			args: []string{"drive", "comment", "list", "--node", "file-1", "--all"},
		},
		{
			name: "create call error",
			args: []string{"drive", "comment", "create", "--node", "file-1", "--content", "test", "--yes"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller := &fileCommentTestCaller{err: sentinel}
			_, err := executeFileCommentCommand(t, caller, "", tt.args...)
			if !errors.Is(err, sentinel) {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestDriveFileCommentListResponseValidationBranches(t *testing.T) {
	tests := []struct {
		name     string
		response string
		message  string
	}{
		{name: "empty", response: "", message: "返回为空"},
		{name: "invalid json", response: "{", message: "不是有效 JSON"},
		{name: "missing file id", response: `{}`, message: "缺少 fileId"},
		{name: "missing total", response: `{"fileId":"file-1","hasMore":false,"items":[]}`, message: "缺少 total"},
		{name: "missing has more", response: `{"fileId":"file-1","total":0,"items":[]}`, message: "缺少布尔字段 hasMore"},
		{name: "next token type", response: `{"fileId":"file-1","total":1,"hasMore":true,"nextToken":2,"items":[]}`, message: "nextToken 不是字符串"},
		{name: "items type", response: `{"fileId":"file-1","total":0,"hasMore":false,"items":{}}`, message: "items 不是数组"},
		{name: "item type", response: `{"fileId":"file-1","total":1,"hasMore":false,"items":[1]}`, message: "items[0] 不是对象"},
		{name: "missing comment id", response: `{"fileId":"file-1","total":1,"hasMore":false,"items":[{"anchor":{}}]}`, message: "缺少 commentId"},
		{name: "missing anchor", response: `{"fileId":"file-1","total":1,"hasMore":false,"items":[{"commentId":"1"}]}`, message: "缺少 anchor 对象"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &fileCommentTestCaller{responses: []string{tt.response}}
			_, err := executeFileCommentCommand(t, caller, "", "drive", "comment", "list", "--node", "file-1")
			var cliErr *CLIError
			if !errors.As(err, &cliErr) || cliErr.Code != CodeMCPToolError || !strings.Contains(cliErr.Message, tt.message) {
				t.Fatalf("error = %#v", err)
			}
		})
	}

	caller := &fileCommentTestCaller{responses: []string{`{
		"data":{"fileId":"file-1","total":0,"hasMore":false,"nextToken":null,"items":null}
	}`}}
	out, err := executeFileCommentCommand(t, caller, "", "drive", "comment", "list", "--node", "file-1")
	if err != nil {
		t.Fatal(err)
	}
	if comments := decodeFileCommentTestOutput(t, out)["comments"].([]any); len(comments) != 0 {
		t.Fatalf("comments = %#v", comments)
	}
}

func TestDriveFileCommentCreateResponseValidationBranches(t *testing.T) {
	tests := []struct {
		name     string
		response string
		message  string
	}{
		{name: "invalid json", response: "{", message: "不是有效 JSON"},
		{name: "missing file id", response: `{"comment":{"commentId":"1","anchor":{}}}`, message: "缺少 fileId"},
		{name: "missing comment", response: `{"fileId":"file-1"}`, message: "缺少 comment 对象"},
		{name: "missing comment id", response: `{"fileId":"file-1","comment":{"anchor":{}}}`, message: "缺少 commentId"},
		{name: "missing anchor", response: `{"fileId":"file-1","comment":{"commentId":"1"}}`, message: "缺少 anchor 对象"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &fileCommentTestCaller{responses: []string{tt.response}}
			_, err := executeFileCommentCommand(t, caller, "",
				"drive", "comment", "create", "--node", "file-1", "--content", "test", "--yes",
			)
			var cliErr *CLIError
			if !errors.As(err, &cliErr) || cliErr.Code != CodeMCPToolError || !strings.Contains(cliErr.Message, tt.message) {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestDriveFileCommentPaginationCycleLimitAndPartialPage(t *testing.T) {
	cycleCaller := &fileCommentTestCaller{responses: []string{
		`{"fileId":"file-1","total":0,"hasMore":true,"nextToken":"2","items":[]}`,
		`{"fileId":"file-1","total":0,"hasMore":true,"nextToken":"3","items":[]}`,
		`{"fileId":"file-1","total":0,"hasMore":true,"nextToken":"2","items":[]}`,
	}}
	_, err := executeFileCommentCommand(t, cycleCaller, "", "drive", "comment", "list", "--node", "file-1", "--all")
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != CodeContentTruncated || !strings.Contains(cliErr.Message, "发生循环") {
		t.Fatalf("cycle error = %#v", err)
	}

	limitResponses := make([]string, 0, fileCommentMaxAutoPages)
	for index := 0; index < fileCommentMaxAutoPages; index++ {
		limitResponses = append(limitResponses, fmt.Sprintf(
			`{"fileId":"file-1","total":0,"hasMore":true,"nextToken":"%d","items":[]}`,
			index+1,
		))
	}
	limitCaller := &fileCommentTestCaller{responses: limitResponses}
	_, err = executeFileCommentCommand(t, limitCaller, "", "drive", "comment", "list", "--node", "file-1", "--all")
	if !errors.As(err, &cliErr) || cliErr.Code != CodeContentTruncated || !strings.Contains(cliErr.Message, "10 页上限") {
		t.Fatalf("page limit error = %#v", err)
	}

	partialCaller := &fileCommentTestCaller{responses: []string{`{
		"fileId":"file-1","total":1,"hasMore":true,"nextToken":"2",
		"items":[{"commentId":"1","anchor":{"scope":"whole"}}]
	}`}}
	out, err := executeFileCommentCommand(t, partialCaller, "", "drive", "comment", "list", "--node", "file-1")
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeFileCommentTestOutput(t, out)
	if payload["nextCursor"] != "2" || payload["complete"] != false || payload["hasMore"] != true {
		t.Fatalf("partial page = %#v", payload)
	}

	overflowCaller := &fileCommentTestCaller{responses: []string{`{
		"fileId":"file-1","total":0,"hasMore":true,
		"nextToken":"999999999999999999999999999999","items":[]
	}`}}
	_, err = executeFileCommentCommand(t, overflowCaller, "", "drive", "comment", "list", "--node", "file-1")
	if !errors.As(err, &cliErr) || cliErr.Code != CodeContentTruncated || !strings.Contains(cliErr.Message, "超出 64 位整数范围") {
		t.Fatalf("overflow cursor error = %#v", err)
	}
}

func TestDriveFileCommentInternalDefaults(t *testing.T) {
	caller := &fileCommentTestCaller{responses: []string{`{
		"fileId":"file-1","total":0,"hasMore":false,"nextToken":null,"items":[]
	}`}}
	testseam.Protect(t, &deps)
	InitDeps(caller)
	var stdout bytes.Buffer
	cmd := &cobra.Command{Use: "list"}
	cmd.SetOut(&stdout)
	if err := runFileCommentList(cmd, listFileCommentsTool, map[string]any{"fileId": "file-1", "maxResults": 200}); err != nil {
		t.Fatal(err)
	}
	payload := decodeFileCommentTestOutput(t, stdout.Bytes())
	if payload["scope"] != "all" {
		t.Fatalf("scope = %#v", payload["scope"])
	}
	if comments := fileCommentListPayload(fileCommentPage{}, "all")["comments"].([]map[string]any); len(comments) != 0 {
		t.Fatalf("nil comments projection = %#v", comments)
	}
}
