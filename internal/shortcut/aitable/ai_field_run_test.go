// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const (
	aiFieldFixture  = `{"data":{"fields":[{"fieldId":"field-1","aiConfig":{"outputType":"text","prompt":[{"type":"text","value":"summarize"},{"type":"fieldRef","fieldId":"source-field"}]}}]}}`
	aiRecordFixture = `{"data":{"records":[{"recordId":"record-1"},{"recordId":"record-2"}]}}`
	aiRunFixture    = `{"data":{"tasks":[{"fieldId":"field-1","status":"submitted","taskId":"task-1","total":2}],"documentUrl":"https://alidocs.dingtalk.com/i/nodes/result"}}`
)

func runAIFieldFixture(t *testing.T, steps []upsertByKeyStep, args ...string) (string, error, *upsertByKeyCaller) {
	t.Helper()
	caller := &upsertByKeyCaller{steps: steps}
	out, err := runAITableCompositeCLI(t, caller, "+ai-field-run", args...)
	return out, err, caller
}

func aiFieldArgs(extra ...string) []string {
	args := []string{"--base-id", "base-1", "--table-id", "table-1", "--field-id", "field-1", "--record-ids", "record-1,record-2"}
	return append(args, extra...)
}

func TestCrossPlatformCoverageAIFieldRunRequiresConfirmationBeforeAnyMCP(t *testing.T) {
	out, err, caller := runAIFieldFixture(t, nil, aiFieldArgs()...)
	var typed *apperrors.Error
	if err == nil || out != "" || !errors.As(err, &typed) || typed.Reason != "confirmation_required" {
		t.Fatalf("confirmation = output:%q err:%#v", out, err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("unconfirmed run made %d MCP calls", len(caller.calls))
	}
}

func TestCrossPlatformCoverageAIFieldRunPayloadAndPendingReceipt(t *testing.T) {
	args := []string{"--base-id", "base-1", "--table-id", "table-1", "--field-id", " field-1 ", "--record-ids", " record-1 , record-1 , record-2 ", "--yes"}
	out, err, caller := runAIFieldFixture(t, []upsertByKeyStep{{text: aiFieldFixture}, {text: aiRecordFixture}, {text: aiRunFixture}}, args...)
	if err != nil {
		t.Fatalf("ai field run error = %v", err)
	}
	if len(caller.calls) != 3 || caller.calls[0].tool != "get_fields" || caller.calls[1].tool != "query_records" || caller.calls[2].tool != "run_ai_field" {
		t.Fatalf("ai field call sequence = %#v", caller.calls)
	}
	runArgs := caller.calls[2].args
	fieldIDs, _ := runArgs["fieldIds"].([]string)
	recordIDs, _ := runArgs["recordIds"].([]string)
	if len(fieldIDs) != 1 || fieldIDs[0] != "field-1" || len(recordIDs) != 2 || recordIDs[0] != "record-1" || recordIDs[1] != "record-2" {
		t.Fatalf("run_ai_field payload = %#v", runArgs)
	}
	for _, callIndex := range []int{0, 1} {
		preflightFieldIDs, _ := caller.calls[callIndex].args["fieldIds"].([]string)
		if len(preflightFieldIDs) != 1 || preflightFieldIDs[0] != "field-1" {
			t.Fatalf("preflight %d fieldIds = %#v", callIndex, preflightFieldIDs)
		}
	}
	queryRecordIDs, _ := caller.calls[1].args["recordIds"].([]string)
	if len(queryRecordIDs) != 2 || queryRecordIDs[0] != "record-1" || queryRecordIDs[1] != "record-2" {
		t.Fatalf("query_records recordIds = %#v", queryRecordIDs)
	}
	for _, want := range []string{`"outcome": "pending"`, `"scope": "selected_records"`, `"taskId": "task-1"`, `"documentUrl": "https://alidocs.dingtalk.com/i/nodes/result"`, `"verified": false`, `"completionStatus": "unknown"`, `"next_command": "dws aitable +record-query`} {
		if !strings.Contains(out, want) {
			t.Fatalf("pending output missing %s: %s", want, out)
		}
	}
	if strings.Contains(out, `"verified": true`) || strings.Contains(out, `"completionStatus": "completed"`) {
		t.Fatalf("pending output claimed completion: %s", out)
	}
}

func TestCrossPlatformCoverageAIFieldRunRejectsInvalidLocalScopeBeforeMCP(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "blank field", args: []string{"--base-id", "base", "--table-id", "table", "--field-id", " ", "--record-ids", "record", "--yes"}},
		{name: "blank base", args: []string{"--base-id", " ", "--table-id", "table", "--field-id", "field", "--record-ids", "record", "--yes"}},
		{name: "blank table", args: []string{"--base-id", "base", "--table-id", " ", "--field-id", "field", "--record-ids", "record", "--yes"}},
		{name: "blank records", args: []string{"--base-id", "base", "--table-id", "table", "--field-id", "field", "--record-ids", " ", "--yes"}},
		{name: "empty record member", args: []string{"--base-id", "base", "--table-id", "table", "--field-id", "field", "--record-ids", "record, ", "--yes"}},
		{name: "too many", args: []string{"--base-id", "base", "--table-id", "table", "--field-id", "field", "--record-ids", strings.Join(numberedIDs("record", 101), ","), "--yes"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			out, err, caller := runAIFieldFixture(t, nil, test.args...)
			if err == nil || out != "" || len(caller.calls) != 0 {
				t.Fatalf("local validation = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAIFieldRunPreflightRejectsIdentityDriftBeforeRun(t *testing.T) {
	fieldCases := []string{
		`{"data":{}}`,
		`{"data":{"fields":{}}}`,
		`{"data":{"fields":[]}}`,
		`{"data":{"fields":[{"fieldId":"field-1","aiConfig":{"outputType":"text","prompt":[{"type":"fieldRef","fieldId":"source"}]}},{"fieldId":"field-2","aiConfig":{"outputType":"text","prompt":[{"type":"fieldRef","fieldId":"source"}]}}]}}`,
		`{"data":{"fields":[{"fieldId":"other","aiConfig":{"outputType":"text","prompt":[{"type":"fieldRef","fieldId":"source"}]}}]}}`,
		`{"data":{"fields":[{"fieldId":"field-1"}]}}`,
		`{"data":{"fields":["bad"]}}`,
	}
	for index, response := range fieldCases {
		t.Run("field-"+string(rune('a'+index)), func(t *testing.T) {
			out, err, caller := runAIFieldFixture(t, []upsertByKeyStep{{text: response}}, aiFieldArgs("--yes")...)
			if err == nil || out != "" || len(caller.calls) != 1 || caller.calls[0].tool != "get_fields" {
				t.Fatalf("field preflight = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}

	recordCases := []string{
		`{"data":{}}`,
		`{"data":{"records":{}}}`,
		`{"data":{"records":[{"recordId":"record-1"}]}}`,
		`{"data":{"records":[{"recordId":"record-1"},{"recordId":"other"}]}}`,
		`{"data":{"records":[{"recordId":"record-1"},{"recordId":"record-1"}]}}`,
		`{"data":{"records":[{"recordId":"record-1"},"bad"]}}`,
	}
	for index, response := range recordCases {
		t.Run("record-"+string(rune('a'+index)), func(t *testing.T) {
			out, err, caller := runAIFieldFixture(t, []upsertByKeyStep{{text: aiFieldFixture}, {text: response}}, aiFieldArgs("--yes")...)
			if err == nil || out != "" || len(caller.calls) != 2 || caller.calls[1].tool != "query_records" {
				t.Fatalf("record preflight = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAIFieldRunRejectsMalformedNonEmptyAIConfig(t *testing.T) {
	configs := []string{
		`{"outputType":"unsupported","prompt":[{"type":"fieldRef","fieldId":"source"}]}`,
		`{"outputType":"text","prompt":"bad"}`,
		`{"outputType":"text","prompt":[]}`,
		`{"outputType":"text","prompt":[{"type":"text","value":"only text"}]}`,
		`{"outputType":"text","prompt":[{"type":"fieldRef","fieldId":" "}]}`,
		`{"outputType":"text","prompt":["bad"]}`,
	}
	for index, config := range configs {
		t.Run(fmt.Sprintf("config-%d", index), func(t *testing.T) {
			response := `{"data":{"fields":[{"fieldId":"field-1","aiConfig":` + config + `}]}}`
			out, err, caller := runAIFieldFixture(t, []upsertByKeyStep{{text: response}}, aiFieldArgs("--yes")...)
			if err == nil || out != "" || len(caller.calls) != 1 || caller.calls[0].tool != "get_fields" {
				t.Fatalf("malformed aiConfig = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAIFieldRunRejectsConflictingPreflightEnvelopes(t *testing.T) {
	fieldConflict := `{"fields":[{"fieldId":"field-1","aiConfig":{"outputType":"text","prompt":[{"type":"fieldRef","fieldId":"source"}]}}],"data":{"fields":[{"fieldId":"field-1","aiConfig":{"outputType":"text","prompt":[{"type":"fieldRef","fieldId":"source"}]}}]}}`
	out, err, caller := runAIFieldFixture(t, []upsertByKeyStep{{text: fieldConflict}}, aiFieldArgs("--yes")...)
	if err == nil || out != "" || len(caller.calls) != 1 {
		t.Fatalf("field envelope conflict = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
	fieldConflict = `{"data":{"fields":[{"fieldId":"field-1","aiConfig":{"outputType":"text","prompt":[{"type":"fieldRef","fieldId":"source"}]}}]},"result":{"fields":[{"fieldId":"field-1","aiConfig":{"outputType":"text","prompt":[{"type":"fieldRef","fieldId":"source"}]}}]}}`
	out, err, caller = runAIFieldFixture(t, []upsertByKeyStep{{text: fieldConflict}}, aiFieldArgs("--yes")...)
	if err == nil || out != "" || len(caller.calls) != 1 {
		t.Fatalf("field data/result conflict = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
	recordConflict := `{"records":[{"recordId":"record-1"},{"recordId":"record-2"}],"data":{"records":[{"recordId":"record-1"},{"recordId":"record-2"}]}}`
	out, err, caller = runAIFieldFixture(t, []upsertByKeyStep{{text: aiFieldFixture}, {text: recordConflict}}, aiFieldArgs("--yes")...)
	if err == nil || out != "" || len(caller.calls) != 2 {
		t.Fatalf("record envelope conflict = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageAIFieldRunDryRunStopsBeforeWrite(t *testing.T) {
	caller := &upsertByKeyCaller{dryRun: true, steps: []upsertByKeyStep{{text: aiFieldFixture}, {text: aiRecordFixture}}}
	out, err := runAITableCompositeCLI(t, caller, "+ai-field-run", aiFieldArgs("--dry-run")...)
	if err != nil {
		t.Fatalf("dry-run = output:%q err:%v", out, err)
	}
	if len(caller.calls) != 2 || caller.calls[0].tool != "get_fields" || caller.calls[1].tool != "query_records" {
		t.Fatalf("dry-run calls = %#v", caller.calls)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("decode dry-run output %q: %v", out, err)
	}
	data, _ := envelope["data"].(map[string]any)
	preflight, _ := data["preflight"].(map[string]any)
	arguments, _ := data["arguments"].(map[string]any)
	if envelope["ok"] != true || envelope["outcome"] != "success" || envelope["dry_run"] != true ||
		data["scope"] != "selected_records" || data["preview_kind"] != "plan" || data["tool"] != "run_ai_field" ||
		data["requestedRecordCount"] != float64(2) || data["executed"] != false || data["verified"] != false ||
		preflight["fieldVerified"] != true || preflight["recordsVerified"] != true {
		t.Fatalf("dry-run preview = %#v", envelope)
	}
	fieldIDs, _ := arguments["fieldIds"].([]any)
	recordIDs, _ := arguments["recordIds"].([]any)
	if arguments["baseId"] != "base-1" || arguments["tableId"] != "table-1" ||
		!reflect.DeepEqual(fieldIDs, []any{"field-1"}) || !reflect.DeepEqual(recordIDs, []any{"record-1", "record-2"}) {
		t.Fatalf("dry-run arguments = %#v", arguments)
	}
	if strings.Contains(out, `"pending"`) || strings.Contains(out, `"submitted"`) {
		t.Fatalf("dry-run manufactured acceptance: %s", out)
	}
	for _, forbidden := range []string{"taskId", "completionStatus", "documentUrl"} {
		if _, exists := data[forbidden]; exists {
			t.Fatalf("dry-run preview manufactured %s: %#v", forbidden, data)
		}
	}
}

func TestCrossPlatformCoverageAIFieldRunDryRunBadPreflightMakesZeroWrites(t *testing.T) {
	tests := []struct {
		name      string
		steps     []upsertByKeyStep
		wantCalls int
	}{
		{
			name:      "non AI field",
			steps:     []upsertByKeyStep{{text: `{"data":{"fields":[{"fieldId":"field-1"}]}}`}},
			wantCalls: 1,
		},
		{
			name:      "bad AI config",
			steps:     []upsertByKeyStep{{text: `{"data":{"fields":[{"fieldId":"field-1","aiConfig":{"outputType":"text","prompt":[{"type":"text","value":"no field ref"}]}}]}}`}},
			wantCalls: 1,
		},
		{
			name:      "missing record",
			steps:     []upsertByKeyStep{{text: aiFieldFixture}, {text: `{"data":{"records":[{"recordId":"record-1"}]}}`}},
			wantCalls: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{dryRun: true, steps: test.steps}
			out, err := runAITableCompositeCLI(t, caller, "+ai-field-run", aiFieldArgs("--dry-run")...)
			if err == nil || out != "" || len(caller.calls) != test.wantCalls {
				t.Fatalf("bad preflight = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
			for _, call := range caller.calls {
				if call.tool == "run_ai_field" {
					t.Fatalf("bad preflight reached write tool: %#v", caller.calls)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAIFieldRunRejectsMalformedRunReceipt(t *testing.T) {
	responses := []string{
		`{"tasks":[{"fieldId":"field-1","status":"submitted","taskId":"task","total":2}]}`,
		`{"data":"bad","tasks":[{"fieldId":"field-1","status":"submitted","taskId":"task","total":2}]}`,
		`{"data":{"tasks":[]}}`,
		`{"data":{"tasks":[{"fieldId":"other","status":"submitted","taskId":"task","total":2}]}}`,
		`{"data":{"tasks":[{"fieldId":"field-1","status":"finished","taskId":"task","total":2}]}}`,
		`{"data":{"tasks":[{"fieldId":"field-1","status":"submitted","taskId":"","total":2}]}}`,
		`{"data":{"tasks":[{"fieldId":"field-1","status":"submitted","taskId":"task","total":1}]}}`,
		`{"data":{"tasks":[{"fieldId":"field-1","status":"submitted","taskId":"task","total":2.5}]}}`,
		`{"data":{"tasks":[{"fieldId":"field-1","status":"submitted","taskId":"task","total":2}],"documentUrl":"http://example.test/result"}}`,
	}
	for index, response := range responses {
		t.Run("receipt-"+string(rune('a'+index)), func(t *testing.T) {
			out, err, caller := runAIFieldFixture(t, []upsertByKeyStep{{text: aiFieldFixture}, {text: aiRecordFixture}, {text: response}}, aiFieldArgs("--yes")...)
			if err == nil || out != "" || len(caller.calls) != 3 || caller.calls[2].tool != "run_ai_field" {
				t.Fatalf("receipt validation = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}

func numberedIDs(prefix string, count int) []string {
	out := make([]string, count)
	for index := range out {
		out[index] = prefix + fmt.Sprint(index)
	}
	return out
}
