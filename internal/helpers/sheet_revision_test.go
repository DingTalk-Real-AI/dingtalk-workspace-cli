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
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type sheetRevisionTestCall struct {
	server string
	tool   string
	args   map[string]any
}

type sheetRevisionTestCaller struct {
	responses map[string]string
	calls     []sheetRevisionTestCall
	dryRun    bool
}

func (c *sheetRevisionTestCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	c.calls = append(c.calls, sheetRevisionTestCall{server: server, tool: tool, args: cloned})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.responses[tool]}}}, nil
}

func (*sheetRevisionTestCaller) Format() string { return "json" }
func (c *sheetRevisionTestCaller) DryRun() bool { return c.dryRun }
func (*sheetRevisionTestCaller) Fields() string { return "" }
func (*sheetRevisionTestCaller) JQ() string     { return "" }

func executeSheetRevisionCommand(t *testing.T, caller *sheetRevisionTestCaller, command *cobra.Command, args ...string) (string, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	InitDeps(caller)
	var stdout bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = io.Discard
	command = prepareUnifiedTestCommand(command)
	command.SetOut(&stdout)
	command.SetErr(io.Discard)
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetArgs(args)
	err := command.Execute()
	return stdout.String(), err
}

func assertSheetRevisionJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(got), &envelope); err != nil {
		t.Fatalf("decode command output: %v\noutput: %s", err, got)
	}
	if envelope["ok"] != true || envelope["outcome"] != "success" {
		t.Fatalf("command envelope = %#v, want success", envelope)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode expected JSON: %v", err)
	}
	if !reflect.DeepEqual(envelope["data"], wantValue) {
		t.Fatalf("command data = %#v, want raw MCP business JSON %#v", envelope["data"], wantValue)
	}
}

func encodeSheetChangesetTransport(t *testing.T, logical string) string {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(logical))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		t.Fatalf("decode logical changeset fixture: %v", err)
	}
	return encodeSheetChangesetTransportObject(t, object)
}

func encodeSheetChangesetTransportObject(t *testing.T, object map[string]any) string {
	t.Helper()
	changesets, ok := object["changesets"]
	if !ok {
		t.Fatal("logical changeset fixture missing changesets")
	}
	encodedChangesets, err := json.Marshal(changesets)
	if err != nil {
		t.Fatalf("encode changesets fixture: %v", err)
	}
	delete(object, "changesets")
	object["changesetsJson"] = string(encodedChangesets)
	encoded, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("encode transport fixture: %v", err)
	}
	return string(encoded)
}

func decodeSheetChangesetLogicalObject(t *testing.T, logical string) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(logical))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		t.Fatalf("decode logical changeset fixture: %v", err)
	}
	return object
}

const sheetChangesetAuditLogicalResponse = `{
	"success":true,
	"logId":"trace-audit",
	"schemaVersion":2,
	"changeSemantics":"FORWARD_ONLY",
	"latestRevision":12,
	"startRevision":10,
	"endRevision":12,
	"summary":{
		"changeCount":3,
		"completeChangeCount":1,
		"partialChangeCount":1,
		"unsupportedChangeCount":1,
		"containsStateReset":true,
		"containsIncompleteChanges":true,
		"affectedSheets":[
			{"sheetId":"sheet-a","sheetName":"Alpha","ranges":["C3"]},
			{"sheetId":"sheet-b","sheetName":"Beta","ranges":["A1","B2"]}
		]
	},
	"changesets":[
		{
			"revision":11,
			"createTime":"2026-08-21T10:00:00+08:00",
			"isSelfEdit":true,
			"eventType":"EDIT",
			"detailsStatus":"UNAVAILABLE",
			"changes":[
				{
					"type":"RANGE_CONTENT_SET",
					"targets":[
						{"scope":"RANGE","sheetId":"sheet-b","sheetName":"Beta","a1Range":"B2","role":"AFFECTED"},
						{"scope":"RANGE","sheetId":"sheet-b","sheetName":"Beta","a1Range":"A1","role":"DESTINATION"},
						{"scope":"WORKBOOK","role":"AFFECTED"}
					],
					"details":{},
					"detailsStatus":"COMPLETE",
					"omissions":[]
				},
				{
					"type":"RANGE_STYLE_SET",
					"targets":[{"scope":"RANGE","sheetId":"sheet-a","sheetName":"Alpha","a1Range":"C3","role":"AFFECTED"}],
					"details":{},
					"detailsStatus":"PARTIAL",
					"omissions":[{"code":"DETAILS_NOT_FULLY_INTERPRETED"}]
				},
				{
					"type":"UNSUPPORTED_CHANGE",
					"targets":[{"scope":"RANGE","sheetId":"sheet-source","a1Range":"D4","role":"SOURCE"}],
					"details":{},
					"detailsStatus":"UNAVAILABLE",
					"omissions":[{"code":"UNKNOWN_ACTION"}]
				}
			]
		},
		{
			"revision":12,
			"createTime":"2026-08-21T10:01:00+08:00",
			"isSelfEdit":false,
			"eventType":"STATE_RESET",
			"detailsStatus":"COMPLETE",
			"reset":{"type":"OVERWRITE","targetStatus":"NOT_APPLICABLE"},
			"changes":[]
		}
	]
}`

func TestCrossPlatformCoverageSheetRevisionGetPassesNodeURLAndPreservesJSONResult(t *testing.T) {
	const response = `{"success":true,"logId":"trace-revision","revision":142,"futureField":{"kept":true,"largeInteger":9007199254740993}}`
	caller := &sheetRevisionTestCaller{responses: map[string]string{sheetRevisionGetRemoteTool: response}}
	nodeURL := "https://alidocs.dingtalk.com/spreadsheetv2/key/path?sheet=one"

	got, err := executeSheetRevisionCommand(t, caller, newSheetRevisionGetCmd(), "--node", nodeURL)
	if err != nil {
		t.Fatalf("revision-get returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want one call", caller.calls)
	}
	call := caller.calls[0]
	if call.server != "sheet" || call.tool != sheetRevisionGetRemoteTool {
		t.Fatalf("dispatch = %s/%s, want sheet/%s", call.server, call.tool, sheetRevisionGetRemoteTool)
	}
	if !reflect.DeepEqual(call.args, map[string]any{"nodeId": nodeURL}) {
		t.Fatalf("args = %#v, want node URL passed through", call.args)
	}
	assertSheetRevisionJSONEqual(t, got, response)
	if !strings.Contains(got, "9007199254740993") {
		t.Fatalf("large JSON integer lost precision: %s", got)
	}
}

func TestCrossPlatformCoverageSheetChangesetGetAcceptsZeroStartAndOmitsOptionalEnd(t *testing.T) {
	const logicalResponse = `{
		"success":true,
		"logId":"trace-changeset",
		"schemaVersion":2,
		"changeSemantics":"FORWARD_ONLY",
		"latestRevision":1,
		"startRevision":0,
		"endRevision":1,
		"summary":{
			"changeCount":1,
			"completeChangeCount":1,
			"partialChangeCount":0,
			"unsupportedChangeCount":0,
			"containsStateReset":false,
			"containsIncompleteChanges":false,
			"affectedSheets":[{"sheetId":"sheet-id","sheetName":"项目","ranges":["C2"]}]
		},
		"changesets":[{
			"revision":1,
			"createTime":"2026-08-18T10:00:00+08:00",
			"isSelfEdit":true,
			"eventType":"EDIT",
			"detailsStatus":"COMPLETE",
			"changes":[{
				"type":"RANGE_CONTENT_SET",
				"targets":[{"scope":"RANGE","sheetId":"sheet-id","sheetName":"项目","sheetNameSource":"AT_CHANGE","a1Range":"C2","role":"AFFECTED"}],
				"details":{"cell":{"value":{"kind":"STRING","stringValue":"完成"}},"futureSafeField":{"x":1}},
				"detailsStatus":"COMPLETE",
				"omissions":[]
			}]
		}]
	}`
	response := encodeSheetChangesetTransport(t, logicalResponse)
	caller := &sheetRevisionTestCaller{responses: map[string]string{sheetChangesetGetRemoteTool: response}}

	got, err := executeSheetRevisionCommand(t, caller, newSheetChangesetGetCmd(),
		"--node", "node-1", "--start-revision", "0")
	if err != nil {
		t.Fatalf("changeset-get returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want one call", caller.calls)
	}
	call := caller.calls[0]
	if call.server != "sheet" || call.tool != sheetChangesetGetRemoteTool {
		t.Fatalf("dispatch = %s/%s, want sheet/%s", call.server, call.tool, sheetChangesetGetRemoteTool)
	}
	if gotStart, exists := call.args["startRevision"]; !exists || gotStart != int64(0) {
		t.Fatalf("startRevision = %#v (exists=%v), want int64(0)", gotStart, exists)
	}
	if _, exists := call.args["endRevision"]; exists {
		t.Fatalf("optional endRevision unexpectedly sent: %#v", call.args)
	}
	if call.args["nodeId"] != "node-1" {
		t.Fatalf("nodeId = %#v", call.args["nodeId"])
	}
	assertSheetRevisionJSONEqual(t, got, logicalResponse)
	if strings.Contains(got, "changesetsJson") {
		t.Fatalf("transport field leaked into Agent output: %s", got)
	}
}

func TestCrossPlatformCoverageSheetChangesetGetSendsNumericEndRevision(t *testing.T) {
	caller := &sheetRevisionTestCaller{responses: map[string]string{
		sheetChangesetGetRemoteTool: encodeSheetChangesetTransport(t, `{"success":true,"logId":"trace-changeset","schemaVersion":2,"changeSemantics":"FORWARD_ONLY","latestRevision":121,"startRevision":120,"endRevision":121,"summary":{"changeCount":0,"completeChangeCount":0,"partialChangeCount":0,"unsupportedChangeCount":0,"containsStateReset":false,"containsIncompleteChanges":false,"affectedSheets":[]},"changesets":[{"revision":121,"createTime":"2026-08-21T10:00:00+08:00","isSelfEdit":true,"eventType":"EDIT","detailsStatus":"COMPLETE","changes":[]}]}`),
	}}
	_, err := executeSheetRevisionCommand(t, caller, newSheetChangesetGetCmd(),
		"--node", "node-1", "--start-revision", "120", "--end-revision", "121")
	if err != nil {
		t.Fatalf("changeset-get returned error: %v", err)
	}
	want := map[string]any{"nodeId": "node-1", "startRevision": int64(120), "endRevision": int64(121)}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("calls = %#v, want args %#v", caller.calls, want)
	}
}

func TestCrossPlatformCoverageSheetChangesetGetAcceptsConsistentIntervalAndSummary(t *testing.T) {
	caller := &sheetRevisionTestCaller{responses: map[string]string{
		sheetChangesetGetRemoteTool: encodeSheetChangesetTransport(t, sheetChangesetAuditLogicalResponse),
	}}
	got, err := executeSheetRevisionCommand(t, caller, newSheetChangesetGetCmd(),
		"--node", "node-1", "--start-revision", "10", "--end-revision", "12")
	if err != nil {
		t.Fatalf("consistent changeset response returned error: %v", err)
	}
	assertSheetRevisionJSONEqual(t, got, sheetChangesetAuditLogicalResponse)
}

func TestCrossPlatformCoverageSheetChangesetGetRejectsIntervalAndSummaryDrift(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "wrong response start", want: "与请求值 10 不一致",
			args:   []string{"--node", "node-1", "--start-revision", "10", "--end-revision", "12"},
			mutate: func(object map[string]any) { object["startRevision"] = json.Number("9") },
		},
		{
			name: "wrong explicit response end", want: "与请求值 12 不一致",
			args:   []string{"--node", "node-1", "--start-revision", "10", "--end-revision", "12"},
			mutate: func(object map[string]any) { object["endRevision"] = json.Number("11") },
		},
		{
			name: "omitted end is not latest", want: "必须等于 $.latestRevision",
			args:   []string{"--node", "node-1", "--start-revision", "10"},
			mutate: func(object map[string]any) { object["endRevision"] = json.Number("11") },
		},
		{
			name: "response span too large", want: "响应区间超过 20 个 revision",
			args: []string{"--node", "node-1", "--start-revision", "10"},
			mutate: func(object map[string]any) {
				object["endRevision"] = json.Number("31")
				object["latestRevision"] = json.Number("31")
			},
		},
		{
			name: "missing revision", want: "无法完整覆盖 (10,12]",
			args: []string{"--node", "node-1", "--start-revision", "10", "--end-revision", "12"},
			mutate: func(object map[string]any) {
				changesets := object["changesets"].([]any)
				object["changesets"] = changesets[1:]
			},
		},
		{
			name: "out of order revisions", want: "期望连续 revision 11",
			args: []string{"--node", "node-1", "--start-revision", "10", "--end-revision", "12"},
			mutate: func(object map[string]any) {
				changesets := object["changesets"].([]any)
				object["changesets"] = []any{changesets[1], changesets[0]}
			},
		},
		{
			name: "fractional changeset revision", want: "必须是非负整数",
			args: []string{"--node", "node-1", "--start-revision", "10", "--end-revision", "12"},
			mutate: func(object map[string]any) {
				object["changesets"].([]any)[0].(map[string]any)["revision"] = json.Number("11.5")
			},
		},
		{
			name: "fractional summary count", want: "$.summary.changeCount 必须是非负整数",
			args: []string{"--node", "node-1", "--start-revision", "10", "--end-revision", "12"},
			mutate: func(object map[string]any) {
				object["summary"].(map[string]any)["changeCount"] = json.Number("3.5")
			},
		},
		{
			name: "forged summary count", want: "复算结果不一致",
			args: []string{"--node", "node-1", "--start-revision", "10", "--end-revision", "12"},
			mutate: func(object map[string]any) {
				object["summary"].(map[string]any)["changeCount"] = json.Number("2")
			},
		},
		{
			name: "forged incomplete flag", want: "复算结果不一致",
			args: []string{"--node", "node-1", "--start-revision", "10", "--end-revision", "12"},
			mutate: func(object map[string]any) {
				object["summary"].(map[string]any)["containsIncompleteChanges"] = false
			},
		},
		{
			name: "forged affected sheets", want: "复算结果不一致",
			args: []string{"--node", "node-1", "--start-revision", "10", "--end-revision", "12"},
			mutate: func(object map[string]any) {
				object["summary"].(map[string]any)["affectedSheets"] = []any{}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			object := decodeSheetChangesetLogicalObject(t, sheetChangesetAuditLogicalResponse)
			test.mutate(object)
			caller := &sheetRevisionTestCaller{responses: map[string]string{
				sheetChangesetGetRemoteTool: encodeSheetChangesetTransportObject(t, object),
			}}
			got, err := executeSheetRevisionCommand(t, caller, newSheetChangesetGetCmd(), test.args...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
			var apiErr *apperrors.Error
			if !errors.As(err, &apiErr) || apiErr.Reason != "invalid_tool_response" ||
				apiErr.Operation != "sheet/"+sheetChangesetGetRemoteTool || apiErr.FailureStage != "response_validation" {
				t.Fatalf("audit drift error = %#v", err)
			}
			if strings.TrimSpace(got) != "" {
				t.Fatalf("audit drift emitted a success envelope: %s", got)
			}
		})
	}
}

func TestCrossPlatformCoverageDecodeSheetChangesetTransportAcceptsLegacyArray(t *testing.T) {
	const response = `{"success":true,"logId":"trace-legacy","schemaVersion":2,"changeSemantics":"FORWARD_ONLY","latestRevision":9007199254740993,"startRevision":9007199254740992,"endRevision":9007199254740993,"summary":{"changeCount":0,"completeChangeCount":0,"partialChangeCount":0,"unsupportedChangeCount":0,"containsStateReset":false,"containsIncompleteChanges":false,"affectedSheets":[]},"changesets":[{"revision":9007199254740993,"createTime":"2026-08-20T10:00:00+08:00","isSelfEdit":true,"eventType":"EDIT","detailsStatus":"COMPLETE","changes":[]}]}`
	decoded, err := decodeSheetRevisionResult(sheetChangesetGetRemoteTool, map[string]any{
		"startRevision": int64(9007199254740992),
		"endRevision":   int64(9007199254740993),
	}, response)
	if err != nil {
		t.Fatalf("decode legacy array: %v", err)
	}
	object := decoded.(map[string]any)
	changesets := object["changesets"].([]any)
	revision := changesets[0].(map[string]any)["revision"].(json.Number)
	if revision.String() != "9007199254740993" {
		t.Fatalf("legacy revision = %s, want exact integer", revision)
	}
}

func TestCrossPlatformCoverageDecodeSheetChangesetTransportRejectsInvalidJSONStrings(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "not string", raw: `{"success":true,"changesetsJson":[]}`},
		{name: "empty", raw: `{"success":true,"changesetsJson":""}`},
		{name: "not json", raw: `{"success":true,"changesetsJson":"not-json"}`},
		{name: "object root", raw: `{"success":true,"changesetsJson":"{}"}`},
		{name: "multiple values", raw: `{"success":true,"changesetsJson":"[] []"}`},
		{name: "missing", raw: `{"success":true}`},
		{name: "non object response", raw: `[]`},
		{name: "legacy changesets not array", raw: `{"success":true,"changesets":{}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeSheetRevisionResult(sheetChangesetGetRemoteTool,
				map[string]any{"startRevision": int64(0)}, test.raw); err == nil {
				t.Fatalf("decode %s unexpectedly succeeded", test.raw)
			}
		})
	}
}

func TestCrossPlatformCoverageDecodeSheetRevisionProtocolFailuresHaveMCPResponseMetadata(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		response  string
		wantLogID string
	}{
		{
			name: "malformed outer json", tool: sheetRevisionGetRemoteTool,
			response: `{"success":true`,
		},
		{
			name: "malformed changesets json", tool: sheetChangesetGetRemoteTool,
			response:  `{"success":true,"logId":"trace-malformed-changeset","changesetsJson":"not-json"}`,
			wantLogID: "trace-malformed-changeset",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeSheetRevisionResult(test.tool, map[string]any{"startRevision": int64(0)}, test.response)
			var apiErr *apperrors.Error
			if !errors.As(err, &apiErr) || apiErr.Origin != "mcp" ||
				apiErr.FailureStage != "response_validation" || apiErr.Reason != "invalid_tool_response" ||
				apiErr.Operation != "sheet/"+test.tool || !apiErr.RetryableSet || apiErr.Retryable {
				t.Fatalf("protocol failure error = %#v", err)
			}
			if test.wantLogID != "" && !strings.Contains(err.Error(), "logId="+test.wantLogID) {
				t.Fatalf("protocol failure error = %v, want backend logId", err)
			}
		})
	}
}

func TestCrossPlatformCoverageDecodeSheetChangesetTransportRejectsBusinessFailure(t *testing.T) {
	const response = `{"success":false,"logId":"trace","errorCode":"UPSTREAM","errorMsg":"failed"}`
	_, err := decodeSheetRevisionResult(sheetChangesetGetRemoteTool,
		map[string]any{"startRevision": int64(0)}, response)
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != CodeMCPToolError ||
		cliErr.Operation != "sheet/"+sheetChangesetGetRemoteTool {
		t.Fatalf("decode failure error = %#v, want sheet MCP business failure", err)
	}
	if !strings.Contains(cliErr.Message, `"errorCode":"UPSTREAM"`) {
		t.Fatalf("business failure message = %q, want backend error payload", cliErr.Message)
	}
}

func TestCrossPlatformCoverageSheetRevisionCommandsRejectInvalidResponsesWithoutSuccessEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		response string
		build    func() *cobra.Command
		args     []string
	}{
		{
			name: "revision empty response", tool: sheetRevisionGetRemoteTool, response: "  \n ",
			build: newSheetRevisionGetCmd, args: []string{"--node", "node-1"},
		},
		{
			name: "changeset empty response", tool: sheetChangesetGetRemoteTool, response: "",
			build: newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "revision non-object response", tool: sheetRevisionGetRemoteTool, response: `[]`,
			build: newSheetRevisionGetCmd, args: []string{"--node", "node-1"},
		},
		{
			name: "changeset missing success", tool: sheetChangesetGetRemoteTool, response: `{"changesetsJson":"[]"}`,
			build: newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "revision business failure", tool: sheetRevisionGetRemoteTool,
			response: `{"success":false,"errorCode":"UPSTREAM","errorMsg":"failed"}`,
			build:    newSheetRevisionGetCmd, args: []string{"--node", "node-1"},
		},
		{
			name: "changeset business failure", tool: sheetChangesetGetRemoteTool,
			response: `{"success":false,"errorCode":"UPSTREAM","errorMsg":"failed"}`,
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &sheetRevisionTestCaller{responses: map[string]string{test.tool: test.response}}
			got, err := executeSheetRevisionCommand(t, caller, test.build(), test.args...)
			if err == nil {
				t.Fatalf("command unexpectedly succeeded with output %q", got)
			}
			if strings.TrimSpace(got) != "" {
				t.Fatalf("failure path emitted a success envelope: %s", got)
			}
		})
	}
}

func TestCrossPlatformCoverageSheetRevisionCommandsRejectPublishedResultContractDrift(t *testing.T) {
	validSummary := `"summary":{"changeCount":0,"completeChangeCount":0,"partialChangeCount":0,"unsupportedChangeCount":0,"containsStateReset":false,"containsIncompleteChanges":false,"affectedSheets":[]}`
	validChangesetPrefix := `"success":true,"logId":"trace-contract","schemaVersion":2,"changeSemantics":"FORWARD_ONLY","latestRevision":1,"startRevision":0,"endRevision":1,`
	validChangesets := `"changesets":[]`

	tests := []struct {
		name     string
		tool     string
		response string
		build    func() *cobra.Command
		args     []string
	}{
		{
			name: "revision missing required field", tool: sheetRevisionGetRemoteTool,
			response: `{"success":true,"logId":"trace-contract"}`,
			build:    newSheetRevisionGetCmd, args: []string{"--node", "node-1"},
		},
		{
			name: "revision wrong field type", tool: sheetRevisionGetRemoteTool,
			response: `{"success":true,"logId":"trace-contract","revision":"1"}`,
			build:    newSheetRevisionGetCmd, args: []string{"--node", "node-1"},
		},
		{
			name: "revision empty log id", tool: sheetRevisionGetRemoteTool,
			response: `{"success":true,"logId":" ","revision":1}`,
			build:    newSheetRevisionGetCmd, args: []string{"--node", "node-1"},
		},
		{
			name: "revision negative integer", tool: sheetRevisionGetRemoteTool,
			response: `{"success":true,"logId":"trace-contract","revision":-1}`,
			build:    newSheetRevisionGetCmd, args: []string{"--node", "node-1"},
		},
		{
			name: "changeset missing summary", tool: sheetChangesetGetRemoteTool,
			response: encodeSheetChangesetTransport(t, `{`+validChangesetPrefix+validChangesets+`}`),
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "changeset wrong revision type", tool: sheetChangesetGetRemoteTool,
			response: encodeSheetChangesetTransport(t, `{"success":true,"logId":"trace-contract","schemaVersion":2,"changeSemantics":"FORWARD_ONLY","latestRevision":"1","startRevision":0,"endRevision":1,`+validSummary+`,`+validChangesets+`}`),
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "changeset wrong schema version", tool: sheetChangesetGetRemoteTool,
			response: encodeSheetChangesetTransport(t, `{"success":true,"logId":"trace-contract","schemaVersion":3,"changeSemantics":"FORWARD_ONLY","latestRevision":1,"startRevision":0,"endRevision":1,`+validSummary+`,`+validChangesets+`}`),
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "changeset wrong semantics", tool: sheetChangesetGetRemoteTool,
			response: encodeSheetChangesetTransport(t, `{"success":true,"logId":"trace-contract","schemaVersion":2,"changeSemantics":"BIDIRECTIONAL","latestRevision":1,"startRevision":0,"endRevision":1,`+validSummary+`,`+validChangesets+`}`),
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "changeset malformed summary", tool: sheetChangesetGetRemoteTool,
			response: encodeSheetChangesetTransport(t, `{`+validChangesetPrefix+`"summary":{"changeCount":0},`+validChangesets+`}`),
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "changeset negative latest revision", tool: sheetChangesetGetRemoteTool,
			response: encodeSheetChangesetTransport(t, `{"success":true,"logId":"trace-contract","schemaVersion":2,"changeSemantics":"FORWARD_ONLY","latestRevision":-1,"startRevision":0,"endRevision":0,`+validSummary+`,`+validChangesets+`}`),
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "changeset negative start revision", tool: sheetChangesetGetRemoteTool,
			response: encodeSheetChangesetTransport(t, `{"success":true,"logId":"trace-contract","schemaVersion":2,"changeSemantics":"FORWARD_ONLY","latestRevision":1,"startRevision":-1,"endRevision":0,`+validSummary+`,`+validChangesets+`}`),
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "changeset negative end revision", tool: sheetChangesetGetRemoteTool,
			response: encodeSheetChangesetTransport(t, `{"success":true,"logId":"trace-contract","schemaVersion":2,"changeSemantics":"FORWARD_ONLY","latestRevision":1,"startRevision":0,"endRevision":-1,`+validSummary+`,`+validChangesets+`}`),
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "changeset start after end", tool: sheetChangesetGetRemoteTool,
			response: encodeSheetChangesetTransport(t, `{"success":true,"logId":"trace-contract","schemaVersion":2,"changeSemantics":"FORWARD_ONLY","latestRevision":2,"startRevision":2,"endRevision":1,`+validSummary+`,`+validChangesets+`}`),
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "changeset end after latest", tool: sheetChangesetGetRemoteTool,
			response: encodeSheetChangesetTransport(t, `{"success":true,"logId":"trace-contract","schemaVersion":2,"changeSemantics":"FORWARD_ONLY","latestRevision":1,"startRevision":0,"endRevision":2,`+validSummary+`,`+validChangesets+`}`),
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
		{
			name: "changeset item missing required field", tool: sheetChangesetGetRemoteTool,
			response: encodeSheetChangesetTransport(t, `{`+validChangesetPrefix+validSummary+`,"changesets":[{"revision":1,"isSelfEdit":true,"eventType":"EDIT","detailsStatus":"COMPLETE","changes":[]}]}`),
			build:    newSheetChangesetGetCmd, args: []string{"--node", "node-1", "--start-revision", "0"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &sheetRevisionTestCaller{responses: map[string]string{test.tool: test.response}}
			got, err := executeSheetRevisionCommand(t, caller, test.build(), test.args...)
			if err == nil {
				t.Fatalf("command unexpectedly accepted contract drift with output %q", got)
			}
			var apiErr *apperrors.Error
			if !errors.As(err, &apiErr) || apiErr.Reason != "invalid_tool_response" ||
				apiErr.Operation != "sheet/"+test.tool || apiErr.FailureStage != "response_validation" {
				t.Fatalf("contract drift error = %#v", err)
			}
			if strings.TrimSpace(got) != "" {
				t.Fatalf("contract drift emitted a success envelope: %s", got)
			}
		})
	}
}

func TestCrossPlatformCoverageValidateSheetPublishedResultCoversDefensiveFailures(t *testing.T) {
	if err := validateSheetPublishedResult("unknown-tool", map[string]any{}); err == nil {
		t.Fatal("unknown tool unexpectedly accepted")
	}

	cache := &sheetPublishedResultSchemaCache{}
	if err := validateSheetPublishedResultWithSchema(sheetRevisionGetRemoteTool, map[string]any{},
		json.RawMessage(`{`), cache); err == nil {
		t.Fatal("invalid published schema unexpectedly accepted")
	}

	if _, err := sheetNonNegativeInteger(int64(1)); err == nil {
		t.Fatal("non-json number unexpectedly accepted")
	}
}

func TestCrossPlatformCoverageDecodeSheetRevisionEmptyResponseHasStructuredRetryableError(t *testing.T) {
	_, err := decodeSheetRevisionResult(sheetRevisionGetRemoteTool, nil, " \n ")
	var apiErr *apperrors.Error
	if !errors.As(err, &apiErr) || apiErr.Reason != "empty_tool_response" ||
		apiErr.Operation != "sheet/"+sheetRevisionGetRemoteTool ||
		apiErr.FailureStage != "response_validation" || !apiErr.RetryableSet || !apiErr.Retryable {
		t.Fatalf("empty response error = %#v", err)
	}
}

func TestCrossPlatformCoverageNormalizeSheetChangesetTransportRejectsOversizedString(t *testing.T) {
	data := map[string]any{
		"success":        true,
		"changesetsJson": strings.Repeat("x", sheetChangesetJSONMaxBytes+1),
	}
	if err := normalizeSheetChangesetTransport(data); err == nil || !strings.Contains(err.Error(), "超过") {
		t.Fatalf("oversized changesetsJson error = %v", err)
	}
}

func TestCrossPlatformCoverageSheetChangesetGetDryRunPreviewsWithoutCallingMCP(t *testing.T) {
	caller := &sheetRevisionTestCaller{dryRun: true, responses: map[string]string{}}
	got, err := executeSheetRevisionCommand(t, caller, newSheetChangesetGetCmd(),
		"--node", "node-1", "--start-revision", "0", "--dry-run")
	if err != nil {
		t.Fatalf("changeset-get dry-run returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run dispatched MCP calls: %#v", caller.calls)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(got), &envelope); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, got)
	}
	if envelope["dry_run"] != true || envelope["outcome"] != "success" {
		t.Fatalf("dry-run envelope = %#v", envelope)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok || data["executed"] != false || data["tool"] != sheetChangesetGetRemoteTool {
		t.Fatalf("dry-run data = %#v", envelope["data"])
	}
	arguments, ok := data["arguments"].(map[string]any)
	if !ok || arguments["startRevision"] != float64(0) || arguments["nodeId"] != "node-1" {
		t.Fatalf("dry-run arguments = %#v", data["arguments"])
	}
}

func TestCrossPlatformCoverageSheetChangesetGetRejectsInvalidRangesBeforeDispatch(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing node", args: []string{"--start-revision", "0"}, want: "--node"},
		{name: "missing start", args: []string{"--node", "node-1"}, want: "--start-revision"},
		{name: "non integer start", args: []string{"--node", "node-1", "--start-revision", "1.5"}, want: "--start-revision 必须是 64 位整数"},
		{name: "non integer end", args: []string{"--node", "node-1", "--start-revision", "0", "--end-revision", "1.5"}, want: "--end-revision 必须是 64 位整数"},
		{name: "negative start", args: []string{"--node", "node-1", "--start-revision", "-1"}, want: "--start-revision 必须是非负整数"},
		{name: "negative end", args: []string{"--node", "node-1", "--start-revision", "0", "--end-revision", "-1"}, want: "--end-revision 必须是非负整数"},
		{name: "end before start", args: []string{"--node", "node-1", "--start-revision", "9", "--end-revision", "8"}, want: "必须大于或等于"},
		{name: "span too large", args: []string{"--node", "node-1", "--start-revision", "9", "--end-revision", "30"}, want: "单次最多查询 20 个 revision"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &sheetRevisionTestCaller{responses: map[string]string{}}
			_, err := executeSheetRevisionCommand(t, caller, newSheetChangesetGetCmd(), test.args...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid input dispatched calls: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageSheetRevisionCommandsPublishHelpAndReviewedContracts(t *testing.T) {
	root := newSheetCommand()
	for _, name := range []string{"revision-get", "changeset-get"} {
		command, _, err := root.Find([]string{name})
		if err != nil || command == nil || command.Name() != name {
			t.Fatalf("sheet %s lookup = command %#v, err %v", name, command, err)
		}
		if command.Flags().Lookup("sheet-id") != nil {
			t.Fatalf("sheet %s unexpectedly accepts --sheet-id", name)
		}
		if !strings.Contains(command.Long, "工作簿级") {
			t.Fatalf("sheet %s Long = %q, want workbook-level scope", name, command.Long)
		}

		final, ok := contractfinal.RuntimeContractFinal(command)
		if !ok {
			t.Fatalf("sheet %s missing ContractFinal", name)
		}
		if final.Safety == nil || final.Safety.Effect != "read" || final.Safety.Confirmation != "not_required" {
			t.Fatalf("sheet %s safety = %#v", name, final.Safety)
		}
		if final.Interface == nil || final.Interface.Ref == nil || final.Interface.Ref.ProductID != "sheet" {
			t.Fatalf("sheet %s interface = %#v", name, final.Interface)
		}
		if final.Result == nil || len(final.Result.DataSchema) == 0 {
			t.Fatalf("sheet %s result = %#v", name, final.Result)
		}
		if final.DryRun == nil || final.DryRun.PreviewKind != contract.DryRunPreviewRequest {
			t.Fatalf("sheet %s dry-run = %#v, want request preview", name, final.DryRun)
		}
		if output.CommandRollout(command) != output.RolloutUnifiedActive {
			t.Fatalf("sheet %s rollout = %s, want unified_active so Result is Agent-visible", name, output.CommandRollout(command))
		}
		if final.Selection == nil || len(final.Selection.UseWhen) == 0 || len(final.Selection.AvoidWhen) == 0 {
			t.Fatalf("sheet %s selection = %#v", name, final.Selection)
		}
	}

	changeset, _, _ := root.Find([]string{"changeset-get"})
	if !strings.Contains(changeset.Long, "(startRevision, endRevision]") ||
		!strings.Contains(changeset.Long, "data.changesets") ||
		!strings.Contains(changeset.Long, "JSON 数组") ||
		!strings.Contains(changeset.Long, "只解码服务端传输格式") ||
		!strings.Contains(changeset.Long, "前向") || !strings.Contains(changeset.Long, "old/current") ||
		!strings.Contains(changeset.Long, "最终状态必须另行回读") {
		t.Fatalf("changeset help does not explain interval/decoded output: %q", changeset.Long)
	}
	final, _ := contractfinal.RuntimeContractFinal(changeset)
	if len(final.Parameters) != 3 || final.Parameters[1].InterfaceType != "number" || final.Parameters[2].InterfaceType != "number" {
		t.Fatalf("changeset parameters = %#v, want numeric revision interface types", final.Parameters)
	}
}

func TestCrossPlatformCoverageSheetRevisionResultSchemasUseMCPNumberAndChangesetV2(t *testing.T) {
	var revisionSchema map[string]any
	if err := json.Unmarshal(sheetRevisionResult.DataSchema, &revisionSchema); err != nil {
		t.Fatalf("decode revision Result schema: %v", err)
	}
	revisionProperties := mustSheetSchemaMap(t, revisionSchema, "properties")
	for _, name := range []string{"success", "logId", "revision"} {
		mustSheetSchemaMap(t, revisionProperties, name)
	}
	for _, name := range []string{"success", "logId", "revision"} {
		assertSheetSchemaRequired(t, revisionSchema, name)
	}
	if got := mustSheetSchemaMap(t, revisionProperties, "revision")["type"]; got != "number" {
		t.Fatalf("revision schema type = %#v, want MCP number", got)
	}

	var changesetSchema map[string]any
	if err := json.Unmarshal(sheetChangesetResult.DataSchema, &changesetSchema); err != nil {
		t.Fatalf("decode changeset Result schema: %v", err)
	}
	properties := mustSheetSchemaMap(t, changesetSchema, "properties")
	for _, name := range []string{"success", "logId", "schemaVersion", "changeSemantics", "latestRevision", "startRevision", "endRevision", "summary", "changesets"} {
		mustSheetSchemaMap(t, properties, name)
	}
	for _, name := range []string{"success", "logId", "schemaVersion", "changeSemantics", "latestRevision", "startRevision", "endRevision", "summary", "changesets"} {
		assertSheetSchemaRequired(t, changesetSchema, name)
	}
	for _, name := range []string{"schemaVersion", "latestRevision", "startRevision", "endRevision"} {
		if got := mustSheetSchemaMap(t, properties, name)["type"]; got != "number" {
			t.Fatalf("changeset %s schema type = %#v, want MCP number", name, got)
		}
	}

	changesetItem := mustSheetSchemaArrayItem(t, mustSheetSchemaMap(t, properties, "changesets"))
	changesetProperties := mustSheetSchemaMap(t, changesetItem, "properties")
	for _, name := range []string{"revision", "createTime", "isSelfEdit", "eventType", "detailsStatus", "reset", "changes"} {
		mustSheetSchemaMap(t, changesetProperties, name)
	}
	assertSheetSchemaNotRequired(t, changesetItem, "reset")
	if got := mustSheetSchemaMap(t, changesetProperties, "reset")["type"]; got != "object" {
		t.Fatalf("changeset reset schema type = %#v, want optional object", got)
	}
	changeItem := mustSheetSchemaArrayItem(t, mustSheetSchemaMap(t, changesetProperties, "changes"))
	changeProperties := mustSheetSchemaMap(t, changeItem, "properties")
	for _, name := range []string{"type", "targets", "details", "detailsStatus", "omissions"} {
		mustSheetSchemaMap(t, changeProperties, name)
		assertSheetSchemaRequired(t, changeItem, name)
	}
	assertSheetSchemaEnum(t, mustSheetSchemaMap(t, changeProperties, "type"), 40, "UNSUPPORTED_CHANGE")
	detailsProperties := mustSheetSchemaMap(t, mustSheetSchemaMap(t, changeProperties, "details"), "properties")
	assertSheetSchemaEnum(t, mustSheetSchemaMap(t, detailsProperties, "fillMode"), 5, "predict")
	if got := mustSheetSchemaMap(t, detailsProperties, "copyStyle")["type"]; got != "boolean" {
		t.Fatalf("copyStyle schema type = %#v, want boolean", got)
	}
	assertSheetSchemaEnum(t, mustSheetSchemaMap(t, detailsProperties, "styleMode"), 4, "cell")
	assertSheetSchemaEnum(t, mustSheetSchemaMap(t, detailsProperties, "pasteMode"), 4, "sheet")
	assertSheetSchemaEnum(t, mustSheetSchemaMap(t, detailsProperties, "iterateMode"), 2, "flex")
	if got := mustSheetSchemaMap(t, detailsProperties, "isCut")["type"]; got != "boolean" {
		t.Fatalf("isCut schema type = %#v, want boolean", got)
	}
	assertSheetSchemaEnum(t, mustSheetSchemaMap(t,
		mustSheetSchemaMap(t, detailsProperties, "includedParts"), "items"), 18, "CONTENT")
	assertSheetSchemaEnum(t, mustSheetSchemaMap(t,
		mustSheetSchemaMap(t, detailsProperties, "clearParts"), "items"), 14, "CELL_TYPES")
	assertSheetSchemaEnum(t, mustSheetSchemaMap(t,
		mustSheetSchemaMap(t, detailsProperties, "preservedCellTypes"), "items"), 2, "SELECT")
	for _, name := range []string{"cell", "cellType", "tag", "border", "style", "properties", "changes"} {
		objectProperties := mustSheetSchemaMap(t, mustSheetSchemaMap(t, detailsProperties, name), "properties")
		mustSheetSchemaMap(t, objectProperties, "cleared")
	}
	cellProperties := mustSheetSchemaMap(t, mustSheetSchemaMap(t, detailsProperties, "cell"), "properties")
	if got := mustSheetSchemaMap(t, cellProperties, "formula")["type"]; got != "string" {
		t.Fatalf("cell formula schema type = %#v, want string", got)
	}
	mustSheetSchemaMap(t, cellProperties, "formulaCleared")
	assertSheetTypedValueSchema(t, mustSheetSchemaMap(t, cellProperties, "value"), false)
	cellLink := mustSheetSchemaMap(t, cellProperties, "link")
	for _, name := range []string{"cleared", "type", "sheetId", "a1Range", "absolute"} {
		mustSheetSchemaMap(t, mustSheetSchemaMap(t, cellLink, "properties"), name)
	}
	if required, exists := cellLink["required"]; exists {
		t.Fatalf("cell.link schema required = %#v, want clear marker to be valid without range fields", required)
	}
	relativeChangeItem := mustSheetSchemaArrayItem(t, mustSheetSchemaMap(t, detailsProperties, "relativeChanges"))
	relativeChangeProperties := mustSheetSchemaMap(t, relativeChangeItem, "properties")
	if got := mustSheetSchemaMap(t, relativeChangeProperties, "offset")["type"]; got != "number" {
		t.Fatalf("relativeChanges offset schema type = %#v, want MCP number", got)
	}
	relativeDimensionProperties := mustSheetSchemaMap(t, mustSheetSchemaMap(t, relativeChangeProperties, "properties"), "properties")
	for _, name := range []string{"cleared", "size", "customSize", "hidden", "sticky", "cellType"} {
		mustSheetSchemaMap(t, relativeDimensionProperties, name)
	}
	for _, name := range []string{"size", "customSize", "hidden", "sticky"} {
		assertSheetSchemaTypeIncludes(t, mustSheetSchemaMap(t, relativeDimensionProperties, name), "null")
	}
	if required, exists := relativeChangeItem["required"]; exists {
		t.Fatalf("relativeChanges item required = %#v, want refs.sheet item valid without offsets", required)
	}
	mustSheetSchemaMap(t, relativeChangeProperties, "styleCleared")
	mustSheetSchemaMap(t, mustSheetSchemaMap(t, relativeChangeProperties, "tag"), "properties")
	relativeCellProperties := mustSheetSchemaMap(t, mustSheetSchemaMap(t, relativeChangeProperties, "cell"), "properties")
	if got := mustSheetSchemaMap(t, relativeCellProperties, "formula")["type"]; got != "string" {
		t.Fatalf("relative cell formula schema type = %#v, want string", got)
	}
	mustSheetSchemaMap(t, relativeCellProperties, "formulaCleared")
	assertSheetTypedValueSchema(t, mustSheetSchemaMap(t, relativeCellProperties, "value"), false)
	relativeCellLink := mustSheetSchemaMap(t, relativeCellProperties, "link")
	for _, name := range []string{"cleared", "type", "sheetId", "a1Range", "absolute"} {
		mustSheetSchemaMap(t, mustSheetSchemaMap(t, relativeCellLink, "properties"), name)
	}
	contentPatternProperties := mustSheetSchemaMap(t, mustSheetSchemaMap(t, detailsProperties, "contentPattern"), "properties")
	contentPatternCell := mustSheetSchemaArrayItem(t, mustSheetSchemaMap(t, contentPatternProperties, "cells"))
	contentPatternCellProperties := mustSheetSchemaMap(t, contentPatternCell, "properties")
	mustSheetSchemaMap(t, contentPatternCellProperties, "cleared")
	if got := mustSheetSchemaMap(t, contentPatternCellProperties, "formula")["type"]; got != "string" {
		t.Fatalf("content pattern formula schema type = %#v, want string", got)
	}
	mustSheetSchemaMap(t, contentPatternCellProperties, "formulaCleared")
	assertSheetTypedValueSchema(t, mustSheetSchemaMap(t, contentPatternCellProperties, "value"), false)
	contentPatternLink := mustSheetSchemaMap(t, contentPatternCellProperties, "link")
	for _, name := range []string{"cleared", "type", "sheetId", "a1Range", "absolute"} {
		mustSheetSchemaMap(t, mustSheetSchemaMap(t, contentPatternLink, "properties"), name)
	}
	if required, exists := contentPatternLink["required"]; exists {
		t.Fatalf("contentPattern cell link schema required = %#v, want clear marker to be valid", required)
	}
	styles := mustSheetSchemaMap(t, detailsProperties, "styles")
	if got := styles["type"]; got != "array" {
		t.Fatalf("styles schema type = %#v, want named entry array", got)
	}
	styleEntry := mustSheetSchemaArrayItem(t, styles)
	styleEntryProperties := mustSheetSchemaMap(t, styleEntry, "properties")
	mustSheetSchemaMap(t, styleEntryProperties, "styleId")
	mustSheetSchemaMap(t, styleEntryProperties, "style")
	assertSheetSchemaRequired(t, styleEntry, "styleId")
	assertSheetSchemaRequired(t, styleEntry, "style")
	dataValidationProperties := mustSheetSchemaMap(t, mustSheetSchemaMap(t, detailsProperties, "dataValidation"), "properties")
	if got := mustSheetSchemaMap(t, dataValidationProperties, "templateId")["type"]; got != "number" {
		t.Fatalf("dataValidation templateId schema type = %#v, want MCP number", got)
	}
	if got := mustSheetSchemaMap(t, dataValidationProperties, "options")["type"]; got != "array" {
		t.Fatalf("dataValidation options schema type = %#v, want dropdown array", got)
	}
	settingsProperties := mustSheetSchemaMap(t, mustSheetSchemaMap(t, dataValidationProperties, "settings"), "properties")
	for _, name := range []string{"allowBlank", "errorStyle", "showInputMessage", "showErrorMessage", "prompt", "error", "requiredInRow", "showDropDown", "columnType", "promptTitle", "errorTitle", "imeMode"} {
		assertSheetSchemaTypeIncludes(t, mustSheetSchemaMap(t, settingsProperties, name), "null")
	}
	criteriaProperties := mustSheetSchemaMap(t, mustSheetSchemaMap(t, dataValidationProperties, "criteria"), "properties")
	mustSheetSchemaMap(t, criteriaProperties, "operator")
	assertSheetTypedValueSchema(t, mustSheetSchemaMap(t, criteriaProperties, "value1"), true)
	assertSheetTypedValueSchema(t, mustSheetSchemaMap(t, criteriaProperties, "value2"), true)
	mustSheetSchemaMap(t, criteriaProperties, "formula")
	settingChangesProperties := mustSheetSchemaMap(t, mustSheetSchemaMap(t, detailsProperties, "changes"), "properties")
	settingMode := mustSheetSchemaMap(t, settingChangesProperties, "mode")
	assertSheetSchemaEnum(t, settingMode, 4, "default")
	if got := settingMode["type"]; got != "string" {
		t.Fatalf("setting mode schema type = %#v, want string", got)
	}
	for _, name := range []string{"cleared", "iterate", "iterateCount", "iterateDelta", "enableDynamicArray", "date1904", "image"} {
		mustSheetSchemaMap(t, settingChangesProperties, name)
	}
	for _, name := range []string{"iterate", "iterateCount", "iterateDelta", "enableDynamicArray", "date1904"} {
		assertSheetSchemaTypeIncludes(t, mustSheetSchemaMap(t, settingChangesProperties, name), "null")
	}
	settingImageProperties := mustSheetSchemaMap(t, mustSheetSchemaMap(t, settingChangesProperties, "image"), "properties")
	mustSheetSchemaMap(t, settingImageProperties, "cleared")
	assertSheetSchemaTypeIncludes(t, mustSheetSchemaMap(t, settingImageProperties, "opacity"), "null")
	omissionItem := mustSheetSchemaArrayItem(t, mustSheetSchemaMap(t, changeProperties, "omissions"))
	omissionCode := mustSheetSchemaMap(t, mustSheetSchemaMap(t, omissionItem, "properties"), "code")
	assertSheetSchemaEnum(t, omissionCode, 13, "SOURCE_RANGE_UNRESOLVED")
	targetItem := mustSheetSchemaArrayItem(t, mustSheetSchemaMap(t, changeProperties, "targets"))
	targetProperties := mustSheetSchemaMap(t, targetItem, "properties")
	for _, name := range []string{"scope", "sheetId", "sheetName", "sheetNameSource", "a1Range", "role"} {
		mustSheetSchemaMap(t, targetProperties, name)
	}

	for _, forbidden := range []string{
		`"actions"`, `"operationKind"`, `"operationVersion"`, `"action"`, `"payload"`,
		`"understood"`, `"payloadOmitted"`, `"sourceOperationIndex"`, `"pluginVersion"`,
		`"targetId"`, `"targetType"`, `"changeKind"`, `"sourceDeltaType"`, `"resetType"`,
		`"resetTarget"`, `"targetAvailable"`, `"warnings"`, `"changesetsJson"`,
	} {
		if bytes.Contains(sheetChangesetResult.DataSchema, []byte(forbidden)) {
			t.Fatalf("changeset Result schema still exposes forbidden V1 key %s", forbidden)
		}
	}
}

func assertSheetTypedValueSchema(t *testing.T, schema map[string]any, allowFormula bool) {
	t.Helper()
	if got := schema["type"]; got != "object" {
		t.Fatalf("typed value schema type = %#v, want object", got)
	}
	properties := mustSheetSchemaMap(t, schema, "properties")
	for _, name := range []string{"kind", "stringValue", "numberValue", "booleanValue"} {
		mustSheetSchemaMap(t, properties, name)
	}
	if allowFormula {
		mustSheetSchemaMap(t, properties, "formula")
	} else {
		mustSheetSchemaMap(t, properties, "values")
	}
	assertSheetSchemaRequired(t, schema, "kind")
}

func mustSheetSchemaMap(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("schema key %q = %#v, want object", key, parent[key])
	}
	return value
}

func mustSheetSchemaArrayItem(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	return mustSheetSchemaMap(t, schema, "items")
}

func assertSheetSchemaRequired(t *testing.T, schema map[string]any, name string) {
	t.Helper()
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema required = %#v, want array containing %q", schema["required"], name)
	}
	for _, value := range required {
		if value == name {
			return
		}
	}
	t.Fatalf("schema required = %#v, missing %q", required, name)
}

func assertSheetSchemaNotRequired(t *testing.T, schema map[string]any, name string) {
	t.Helper()
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema required = %#v, want array without %q", schema["required"], name)
	}
	for _, value := range required {
		if value == name {
			t.Fatalf("schema required = %#v, unexpectedly contains optional %q", required, name)
		}
	}
}

func assertSheetSchemaEnum(t *testing.T, schema map[string]any, wantCount int, wantValue string) {
	t.Helper()
	values, ok := schema["enum"].([]any)
	if !ok || len(values) != wantCount {
		t.Fatalf("schema enum = %#v, want %d values including %q", schema["enum"], wantCount, wantValue)
	}
	for _, value := range values {
		if value == wantValue {
			return
		}
	}
	t.Fatalf("schema enum = %#v, missing %q", values, wantValue)
}

func assertSheetSchemaTypeIncludes(t *testing.T, schema map[string]any, want string) {
	t.Helper()
	types, ok := schema["type"].([]any)
	if !ok {
		t.Fatalf("schema type = %#v, want array containing %q", schema["type"], want)
	}
	for _, value := range types {
		if value == want {
			return
		}
	}
	t.Fatalf("schema type = %#v, missing %q", types, want)
}
