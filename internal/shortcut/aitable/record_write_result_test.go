// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCrossPlatformCoverageRecordWriteResultForwardsOriginalToken(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"baseId":"b","tableId":"t","clientToken":"123e4567-e89b-42d3-a456-426614174000","state":"applied","recordIds":["rec-1"]}}`}}}
	out, err := runAITableCompositeCLI(t, caller, "+record-write-result",
		"--base-id", "b", "--table-id", "t",
		"--client-token", "123e4567-e89b-42d3-a456-426614174000")
	if err != nil {
		t.Fatalf("write result error = %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want one read", caller.calls)
	}
	call := caller.calls[0]
	if call.product != serverMain || call.tool != "get_record_write_result" {
		t.Fatalf("call = %#v, want %s/get_record_write_result", call, serverMain)
	}
	if got := call.args["clientToken"]; got != "123e4567-e89b-42d3-a456-426614174000" {
		t.Fatalf("clientToken = %#v", got)
	}
	var envelope struct {
		OK      bool           `json:"ok"`
		Outcome string         `json:"outcome"`
		Data    map[string]any `json:"data"`
	}
	if json.Unmarshal([]byte(out), &envelope) != nil || !envelope.OK || envelope.Outcome != "success" || envelope.Data["state"] != "applied" || envelope.Data["data"] != nil {
		t.Fatalf("output = %s", out)
	}
}

func TestCrossPlatformCoverageRecordWriteResultUnknownAndDryRun(t *testing.T) {
	var resultSchema any
	if err := json.Unmarshal(RecordWriteResult.Contract.Result.DataSchema, &resultSchema); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("https://dws.test/write-result.json", resultSchema); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile("https://dws.test/write-result.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, recordIDs := range []string{`,"recordIds":[]`, ``} {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"status":"success","data":{"baseId":"b","tableId":"t","clientToken":"123e4567-e89b-42d3-a456-426614174000","state":"unknown"` + recordIDs + `}}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+record-write-result", "--base-id", "b", "--table-id", "t", "--client-token", "123e4567-e89b-42d3-a456-426614174000")
		var envelope struct {
			OK      bool           `json:"ok"`
			Outcome string         `json:"outcome"`
			Data    map[string]any `json:"data"`
			Meta    struct {
				Operation struct{ ID, State, NextCommand string } `json:"operation"`
			} `json:"meta"`
		}
		if err != nil || json.Unmarshal([]byte(out), &envelope) != nil || !envelope.OK || envelope.Outcome != "pending" || envelope.Data["state"] != "unknown" || len(caller.calls) != 1 {
			t.Fatalf("unknown must remain pending after one read: out=%s err=%v calls=%#v", out, err, caller.calls)
		}
		if err := compiled.Validate(envelope.Data); err != nil {
			t.Fatalf("pending result violates declared schema: %v", err)
		}
		if envelope.Data["baseId"] != "b" || envelope.Data["tableId"] != "t" || envelope.Data["clientToken"] != "123e4567-e89b-42d3-a456-426614174000" {
			t.Fatalf("original identity lost: %s", out)
		}
		var wire map[string]any
		_ = json.Unmarshal([]byte(out), &wire)
		operation := wire["meta"].(map[string]any)["operation"].(map[string]any)
		if operation["id"] != "123e4567-e89b-42d3-a456-426614174000" || operation["state"] != "unknown" || operation["next_command"] != "dws aitable +record-write-result --base-id b --table-id t --client-token 123e4567-e89b-42d3-a456-426614174000" {
			t.Fatalf("unsafe recovery operation: %#v", operation)
		}
	}
	caller := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, caller, "+record-write-result", "--base-id", "b", "--table-id", "t", "--client-token", "123e4567-e89b-42d3-a456-426614174000", "--dry-run")
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal([]byte(out), &envelope) != nil || err != nil || len(caller.calls) != 0 || envelope.Data["executed"] != false || envelope.Data["tool"] != "get_record_write_result" || len(envelope.Data) != 3 {
		t.Fatalf("preview = %s, %v, %#v", out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageRecordWriteResultRejectsUntrustedReplies(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"base mismatch", "baseId", "other"}, {"table mismatch", "tableId", "other"}, {"token mismatch", "clientToken", "other"},
		{"state type", "state", true}, {"state unknown", "state", "unexpected"},
		{"unknown with IDs", "state", "unknown"}, {"applied empty", "recordIds", []string{}},
		{"IDs missing", "recordIds", nil}, {"IDs type", "recordIds", "r1"},
		{"ID type", "recordIds", []any{3}}, {"ID blank", "recordIds", []string{" "}},
		{"IDs duplicate", "recordIds", []string{"r1", "r1"}},
		{"IDs too many", "recordIds", make([]string, 101)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := map[string]any{"baseId": "b", "tableId": "t", "clientToken": "123e4567-e89b-42d3-a456-426614174000", "state": "applied", "recordIds": []string{"r1"}}
			data[tc.key] = tc.value
			raw, _ := json.Marshal(map[string]any{"data": data})
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: string(raw)}}}
			out, err := runAITableCompositeCLI(t, caller, "+record-write-result", "--base-id", "b", "--table-id", "t", "--client-token", "123e4567-e89b-42d3-a456-426614174000")
			var typed *apperrors.Error
			if out != "" || !errors.As(err, &typed) || typed.Reason != "invalid_record_write_result" || typed.Retryable || len(caller.calls) != 1 {
				t.Fatalf("out=%s err=%#v calls=%#v", out, err, caller.calls)
			}
		})
	}
	for _, step := range []upsertByKeyStep{{text: `{"data":null}`}, {text: `{}`}, {err: errors.New("read transport unavailable")}} {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{step}}
		out, err := runAITableCompositeCLI(t, caller, "+record-write-result", "--base-id", "b", "--table-id", "t", "--client-token", "123e4567-e89b-42d3-a456-426614174000")
		if err == nil || out != "" || len(caller.calls) != 1 {
			t.Fatalf("out=%s err=%v calls=%#v", out, err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageRecordWriteResultRejectsInvalidTokenBeforeCall(t *testing.T) {
	caller := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, caller, "+record-write-result",
		"--base-id", "b", "--table-id", "t", "--client-token", "not-a-uuid")
	if err == nil || !strings.Contains(err.Error(), "client-token 必须是合法的 UUID v4") {
		t.Fatalf("err = %v, want UUID validation", err)
	}
	if out != "" || len(caller.calls) != 0 {
		t.Fatalf("invalid token dispatched: out=%q calls=%#v", out, caller.calls)
	}
}

func TestCrossPlatformCoverageRecordUpsertShortcutForwardsClientToken(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"createdRecordIds":["rec-1"]}}`},
		{text: `{"data":{"records":[{"recordId":"rec-1","cells":{"f":"v"}}]}}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+record-upsert",
		"--base-id", "b", "--table-id", "t",
		"--records", `[{"cells":{"f":"v"}}]`,
		"--client-token", "123e4567-e89b-42d3-a456-426614174000", "--yes")
	if err != nil {
		t.Fatalf("upsert error = %v", err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %#v, want write and read", caller.calls)
	}
	if got := caller.calls[0].args["clientToken"]; got != "123e4567-e89b-42d3-a456-426614174000" {
		t.Fatalf("upsert clientToken = %#v", got)
	}
	if !strings.Contains(out, `"verifiedCount": 1`) {
		t.Fatalf("output = %s", out)
	}
}

func TestCrossPlatformCoverageRecordUpsertShortcutRejectsTokenAcrossBatches(t *testing.T) {
	records := make([]string, recordBatchSize+1)
	for index := range records {
		records[index] = `{"cells":{"f":"v"}}`
	}
	caller := &upsertByKeyCaller{}
	_, err := runAITableCompositeCLI(t, caller, "+record-upsert",
		"--base-id", "b", "--table-id", "t",
		"--records", "["+strings.Join(records, ",")+"]",
		"--client-token", "123e4567-e89b-42d3-a456-426614174000", "--yes")
	if err == nil || !strings.Contains(err.Error(), "仅支持单批最多 100 条") {
		t.Fatalf("err = %v, want multi-batch token validation", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("validation dispatched calls: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageRecordUpsertShortcutRejectsInvalidToken(t *testing.T) {
	caller := &upsertByKeyCaller{}
	_, err := runAITableCompositeCLI(t, caller, "+record-upsert", "--base-id", "b", "--table-id", "t", "--records", `[{"cells":{"f":"v"}}]`, "--client-token", "invalid", "--yes")
	if err == nil || !strings.Contains(err.Error(), "UUID v4") || len(caller.calls) != 0 {
		t.Fatalf("err=%v calls=%#v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageRecordUpsertReconciliationPreservesGroups(t *testing.T) {
	const token = "123e4567-e89b-42d3-a456-426614174000"
	for _, code := range []string{"DUPLICATE_CLIENT_TOKEN", "CREATE_RECORDS_OUTCOME_UNKNOWN", "DOWNSTREAM_UNAVAILABLE"} {
		for _, appError := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/app=%v", code, appError), func(t *testing.T) {
				raw := fmt.Sprintf(`{"status":"error","error":{"type":"USER_ERROR","code":%q,"retryable":false,"details":{"clientToken":%q,"reconcileTool":"get_record_write_result","createdRecordIds":["new1"],"updatedRecordIds":["u1"]}}}`, code, token)
				var cause error = &helpers.CLIError{Code: helpers.CodeMCPToolError, Message: raw}
				if appError {
					cause = apperrors.NewAPI(raw, apperrors.WithReason("business_error"))
				}
				caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: cause}}}
				_, err := runAITableCompositeCLI(t, caller, "+record-upsert", "--base-id", "b", "--table-id", "t", "--records", `[{"cells":{"f":"v"}},{"recordId":"u1","cells":{"f":"updated"}}]`, "--client-token", token, "--yes")
				var typed *apperrors.Error
				if !errors.As(err, &typed) || !errors.Is(err, cause) || typed.Retryable || len(caller.calls) != 1 {
					t.Fatalf("err=%#v calls=%#v", err, caller.calls)
				}
				result := typed.Details["result"].(compositeResult)
				if result.Status != "unknown" || result.FailedCount != 0 || result.CompletedCount != 0 || len(result.KnownEffects) != 2 || result.KnownEffects[0]["group"] != "create" || result.KnownEffects[1]["group"] != "update" {
					t.Fatalf("result=%#v", result)
				}
				if result.Checkpoint["clientToken"] != token || !strings.Contains(result.NextCommand, "+record-write-result") || !strings.Contains(result.NextCommand, token) || !strings.Contains(result.Checkpoint["updateReadbackCommand"].(string), "--record-ids u1") || len(typed.Actions) != 1 {
					t.Fatalf("missing read-only recovery: %#v", result)
				}
				if isRecordWriteInputRejection(cause) {
					t.Fatal("reconciliation error classified as rejected input")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRecordUpsertReconciliationKeepsOriginalToken(t *testing.T) {
	const token = "123e4567-e89b-42d3-a456-426614174000"
	for _, tc := range []struct{ original, echoed string }{
		{"", token}, {token, "different"}, {"", ""}, {"", "invalid"},
	} {
		details := map[string]any{"clientToken": tc.echoed}
		if tc.echoed != "" {
			details["createdRecordIds"], details["updatedRecordIds"] = []string{"untrusted-created"}, []string{"untrusted-updated"}
		}
		encodedDetails, _ := json.Marshal(details)
		raw := fmt.Sprintf(`{"status":"error","error":{"type":"USER_ERROR","code":"DUPLICATE_CLIENT_TOKEN","retryable":false,"details":%s}}`, encodedDetails)
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: raw}}}
		_, err := runAITableCompositeCLI(t, caller, "+record-upsert", "--base-id", "b", "--table-id", "t", "--records", `[{"cells":{"f":"v"}}]`, "--client-token", tc.original, "--yes")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Retryable || len(caller.calls) != 1 {
			t.Fatalf("err=%#v calls=%#v", err, caller.calls)
		}
		result := typed.Details["result"].(compositeResult)
		if result.Status != "unknown" || result.FailedCount != 0 || len(result.KnownEffects) != 0 || result.Checkpoint["updateReadbackCommand"] != nil {
			t.Fatalf("result=%#v", result)
		}
		sent, _ := caller.calls[0].args["clientToken"].(string)
		if sent == "" || aitableprotocol.ValidateClientToken(sent) != nil || (tc.original != "" && sent != tc.original) {
			t.Fatalf("write did not carry the original valid token: %#v", caller.calls[0])
		}
		if result.Checkpoint["clientToken"] != sent || !strings.Contains(result.NextCommand, sent) {
			t.Fatalf("recovery replaced the sent token with an untrusted echo: %#v", result)
		}
	}
	for _, raw := range []string{`not-json`, `{"status":"success"}`, `{"status":"error","error":{"type":"SYSTEM_ERROR"}}`} {
		if decodeRecordWriteFailure(&helpers.CLIError{Code: helpers.CodeMCPToolError, Message: raw}).needsReconciliation() {
			t.Fatalf("false recovery for %s", raw)
		}
	}
	protocolErr := apperrors.NewAPI(`{"status":"error","error":{"code":"DUPLICATE_CLIENT_TOKEN"}}`, apperrors.WithReason("mcp_tool_error"))
	if !decodeRecordWriteFailure(protocolErr).needsReconciliation() {
		t.Fatal("MCP protocol error lost reconciliation semantics")
	}
}

func TestCrossPlatformCoverageRecordUpsertReconciliationKeepsPriorBatch(t *testing.T) {
	records := make([]map[string]any, 101)
	for index := 0; index < 100; index++ {
		records[index] = map[string]any{"recordId": fmt.Sprintf("r%d", index), "cells": map[string]any{"f": "v"}}
	}
	records[100] = map[string]any{"cells": map[string]any{"f": "new"}}
	steps := []upsertByKeyStep{{text: `{"data":{"updatedRecordIds":[]}}`}}
	for index := 0; index < 100; index += 20 {
		raw, _ := json.Marshal(map[string]any{"records": records[index : index+20]})
		steps = append(steps, upsertByKeyStep{text: string(raw)})
	}
	steps = append(steps, upsertByKeyStep{text: `{"status":"error","error":{"type":"SYSTEM_ERROR","code":"CREATE_RECORDS_OUTCOME_UNKNOWN","retryable":false,"details":{"clientToken":"123e4567-e89b-42d3-a456-426614174000"}}}`})
	caller := &upsertByKeyCaller{steps: steps}
	_, err := runRecordBatchCLI(t, caller, "+record-upsert", records)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Retryable || len(caller.calls) != 7 {
		t.Fatalf("err=%#v calls=%#v", err, caller.calls)
	}
	result := typed.Details["result"].(compositeResult)
	if result.Status != "unknown" || result.CompletedCount != 100 || result.FailedCount != 0 || len(result.KnownEffects) != 1 || result.Checkpoint["nextOffset"] != 100 {
		t.Fatalf("result=%#v", result)
	}
}

// Invalid unknown receipts must fail before they can be interpreted as pending.
func TestCrossPlatformCoverageRecordWriteResultUnknownRejectsMalformedIdentityAndIDs(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"base", "baseId", "other"}, {"table", "tableId", "other"}, {"token", "clientToken", "other"},
		{"null IDs", "recordIds", nil}, {"string IDs", "recordIds", "r1"},
		{"partial IDs", "recordIds", []string{"r1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"baseId": "b", "tableId": "t", "clientToken": "123e4567-e89b-42d3-a456-426614174000", "state": "unknown"}
			body[tc.key] = tc.value
			raw, err := json.Marshal(map[string]any{"data": body})
			if err != nil {
				t.Fatal(err)
			}
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: string(raw)}}}
			out, err := runAITableCompositeCLI(t, caller, "+record-write-result", "--base-id", "b", "--table-id", "t", "--client-token", "123e4567-e89b-42d3-a456-426614174000")
			var typed *apperrors.Error
			if out != "" || !errors.As(err, &typed) || typed.Reason != "invalid_record_write_result" || typed.Retryable || len(caller.calls) != 1 {
				t.Fatalf("invalid receipt accepted: out=%s err=%v calls=%#v", out, err, caller.calls)
			}
		})
	}
}
