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

func TestCrossPlatformCoverageRecordUpsertMixedAttachmentRecovery(t *testing.T) {
	const token = "123e4567-e89b-42d3-a456-426614174000"
	for _, code := range []string{"CREATE_RECORDS_OUTCOME_UNKNOWN", "DUPLICATE_CLIENT_TOKEN", "ATTACHMENT_WRITE_UNCONFIRMED"} {
		for _, echoed := range []string{token, "different"} {
			cause := apperrors.NewAPI("Creation or attachments require reconciliation", apperrors.WithReason(code),
				apperrors.WithRetryable(false), apperrors.WithDetails(map[string]any{
					"clientToken": echoed, "reconcileTool": "get_record_write_result",
					"attachmentVerification": "unconfirmed", "recordIds": []string{"u1"},
					"fieldIds": []string{"attachment"}, "updatedRecordIds": []string{"u1"},
					"writeOutcome": "OUTCOME_UNKNOWN", "mutationSubmitted": true,
					"readback": "mismatch", "mismatchedFieldId": "F",
				}))
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: cause}}}
			_, err := runAITableCompositeCLI(t, caller, "+record-upsert", "--base-id", "b", "--table-id", "t",
				"--records", `[{"cells":{"f":"v"}},{"recordId":"u1","cells":{"f":"updated"}}]`, "--client-token", token, "--yes")
			var typed *apperrors.Error
			if !errors.As(err, &typed) || !errors.Is(err, cause) || typed.Retryable || len(caller.calls) != 1 {
				t.Fatalf("write replayed or original error lost: error=%#v calls=%#v", err, caller.calls)
			}
			result := typed.Details["result"].(compositeResult)
			if echoed != token {
				if result.Checkpoint["clientToken"] != token || result.Verification["attachmentVerification"] != nil ||
					result.Verification["writeOutcome"] != nil || result.Verification["mutationSubmitted"] != nil ||
					result.Verification["readback"] != nil || result.Verification["mismatchedFieldId"] != nil || len(result.KnownEffects) != 0 {
					t.Fatalf("mismatched receipt was promoted to known effects: %#v", result)
				}
				continue
			}
			if result.Status != "unknown" || result.Checkpoint["clientToken"] != token ||
				result.Verification["attachmentVerification"] != "unconfirmed" ||
				result.Verification["readback"] != "mismatch" || result.Verification["mismatchedFieldId"] != "F" ||
				!reflect.DeepEqual(result.Verification["recordIds"], []string{"u1"}) ||
				!reflect.DeepEqual(result.Verification["fieldIds"], []string{"attachment"}) ||
				!strings.Contains(result.Checkpoint["updateReadbackCommand"].(string), "--record-ids u1") {
				t.Fatalf("mixed recovery instructions lost: %#v", result)
			}
		}
	}
}

func TestCrossPlatformCoverageRecordBatchSingleRecoverySurvivesTypedErrors(t *testing.T) {
	const token = "123e4567-e89b-42d3-a456-426614174000"
	for _, command := range []string{"+record-upsert", "+record-update"} {
		for _, code := range []string{"ATTACHMENT_PREPARATION_UNAVAILABLE", "UPDATE_RECORDS_RESULT_UNKNOWN", "CREATE_RECORDS_OUTCOME_UNKNOWN", "DUPLICATE_CLIENT_TOKEN"} {
			t.Run(command+"/"+code, func(t *testing.T) {
				details := map[string]any{"writeOutcome": "OUTCOME_UNKNOWN", "mutationSubmitted": true,
					"createdRecordIds": []string{"created"}, "updatedRecordIds": []string{"updated"}}
				records := `[{"recordId":"updated","cells":{"f":"changed"}}]`
				if command == "+record-upsert" {
					records = `[{"cells":{"f":"new"}},{"recordId":"updated","cells":{"f":"changed"}}]`
					details["clientToken"], details["reconcileTool"] = token, "get_record_write_result"
				}
				wire, err := json.Marshal(map[string]any{"status": "error", "error": map[string]any{
					"type": "SYSTEM_ERROR", "code": code, "details": details}})
				if err != nil {
					t.Fatal(err)
				}
				cause := recordMutationErrorForTest(t, command, string(wire))
				caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: cause}}}
				args := []string{"--base-id", "b", "--table-id", "t", "--records", records, "--yes"}
				if command == "+record-upsert" {
					args = append(args, "--client-token", token)
				}
				_, err = runAITableCompositeCLI(t, caller, command, args...)
				var typed *apperrors.Error
				if !errors.As(err, &typed) || !errors.Is(err, cause) || typed.Retryable || len(caller.calls) != 1 {
					t.Fatalf("recovery was replaced or followed by another call: err=%#v calls=%#v", err, caller.calls)
				}
				result := typed.Details["result"].(compositeResult)
				if result.Status != "unknown" || result.FailedCount != 0 || result.CompletedCount != 0 ||
					result.Verification["writeOutcome"] != "OUTCOME_UNKNOWN" || result.Verification["mutationSubmitted"] != true ||
					!strings.Contains(result.Checkpoint["updateReadbackCommand"].(string), "--record-ids updated") {
					t.Fatalf("recovery stage or original targets lost: %#v", result)
				}
				if command == "+record-upsert" {
					if result.Checkpoint["clientToken"] != token || !strings.Contains(result.NextCommand, "+record-write-result") || len(result.KnownEffects) != 2 {
						t.Fatalf("creation recovery lost: %#v", result)
					}
				} else if result.Checkpoint["clientToken"] != nil || !strings.Contains(result.NextCommand, "+record-query") {
					t.Fatalf("update-only recovery acquired a creation token: %#v", result)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRecordBatchUnsubmittedTokenRetainsRejection(t *testing.T) {
	for _, stage := range []string{"NOT_ATTEMPTED", "REJECTED"} {
		for _, command := range []string{"+record-update", "+record-upsert"} {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: fmt.Sprintf(`{"status":"error","error":{"type":"INPUT_ERROR","code":"ATTACHMENT_PREPARATION_UNAVAILABLE","retryable":false,"details":{"writeOutcome":%q,"mutationSubmitted":false,"clientToken":"original","reconcileTool":"get_record_write_result"}}}`, stage)}}}
			_, err := runAITableCompositeCLI(t, caller, command, "--base-id", "b", "--table-id", "t", "--records", `[{"recordId":"r","cells":{"f":"changed"}}]`, "--yes")
			var typed *apperrors.Error
			if !errors.As(err, &typed) || len(caller.calls) != 1 {
				t.Fatalf("definite rejection must stop without reads or writes: %v", err)
			}
			result := typed.Details["result"].(compositeResult)
			if result.Status != "failed" || result.FailedCount != 1 || result.CompletedCount != 0 || result.NextCommand != "" || result.Verification != nil {
				t.Fatalf("unsubmitted batch was turned into pending reconciliation: %#v", result)
			}
		}
	}
}

// No group, reconciliation tool or accepted IDs may be needed to preserve a
// reviewed write stage. In particular, PARTIALLY_COMPLETED is not a rejection.
func TestCrossPlatformCoverageRecordBatchUpdateOnlyRecoveryStages(t *testing.T) {
	for _, command := range []string{"+record-update", "+record-upsert"} {
		for _, code := range []string{"ATTACHMENT_PREPARATION_UNAVAILABLE", "CREATE_RECORDS_OUTCOME_UNKNOWN", "UPDATE_RECORDS_RESULT_UNKNOWN", "DUPLICATE_CLIENT_TOKEN"} {
			for _, stage := range []string{"OUTCOME_UNKNOWN", "COMMITTED_RESULT_UNAVAILABLE", "PARTIALLY_COMPLETED"} {
				for _, representation := range []string{"wire", "typed", "legacy"} {
					t.Run(command+"/"+code+"/"+stage+"/"+representation, func(t *testing.T) {
						raw := fmt.Sprintf(`{"status":"error","error":{"type":"SYSTEM_ERROR","code":%q,"retryable":false,"details":{"writeOutcome":%q,"mutationSubmitted":true}}}`, code, stage)
						step := upsertByKeyStep{text: raw}
						if representation == "typed" {
							step = upsertByKeyStep{err: recordMutationErrorForTest(t, command, raw)}
						} else if representation == "legacy" {
							step = upsertByKeyStep{err: &helpers.CLIError{Code: helpers.CodeMCPToolError, Message: raw}}
						}
						caller := &upsertByKeyCaller{steps: []upsertByKeyStep{step, {err: fmt.Errorf("read-back unavailable")}}}
						records := make([]map[string]any, 101)
						for i := range records {
							records[i] = map[string]any{"recordId": fmt.Sprintf("r%d", i), "cells": map[string]any{"f": "v"}}
						}
						_, err := runRecordBatchCLI(t, caller, command, records)
						var typed *apperrors.Error
						if !errors.As(err, &typed) {
							t.Fatalf("missing structured error: %v", err)
						}
						result := typed.Details["result"].(compositeResult)
						if apperrors.ExitCode(err) != 1 || typed.Retryable || result.Retryable || result.Status != "unknown" || result.FailedCount != 0 || result.CompletedCount != 0 || len(caller.calls) != 1 {
							t.Fatalf("write recovery changed: status=%s retryable=%v failed=%d completed=%d calls=%d", result.Status, typed.Retryable, result.FailedCount, result.CompletedCount, len(caller.calls))
						}
						if result.Verification["writeOutcome"] != stage || result.Verification["mutationSubmitted"] != true || result.Checkpoint["clientToken"] != nil ||
							!strings.Contains(result.NextCommand, "+record-query") || !strings.Contains(result.NextCommand, "r99") || len(typed.Actions) != 1 || !strings.Contains(strings.Join(result.Warnings, " "), "禁止重放") {
							t.Fatalf("original stage/IDs/read-only recovery lost: %#v", result)
						}
					})
				}
			}
		}
	}
}

func TestCrossPlatformCoverageRecordUpsertPreservesUnknownUpdateGroup(t *testing.T) {
	const token = "123e4567-e89b-42d3-a456-426614174000"
	// A caller may pass a token to an update-only batch, but that cannot create
	// a creation-reconciliation obligation or replace the original ID read-back.
	for _, scenario := range []struct{ mixed, withToken bool }{{false, false}, {false, true}, {true, true}} {
		mixed := scenario.mixed
		groups := map[string]any{
			"create": map[string]any{"requested": false},
			"update": map[string]any{"requested": true, "requestedRecordIds": []any{"u1"}, "acceptedRecordIds": []any{},
				"error": map[string]any{"code": "UPDATE_RECORDS_RESULT_UNKNOWN", "details": map[string]any{
					"writeOutcome": "OUTCOME_UNKNOWN", "mutationSubmitted": true,
				}}},
		}
		details := map[string]any{"groupOutcomes": groups}
		code := "UPDATE_RECORDS_RESULT_UNKNOWN"
		records := `[{"recordId":"u1","cells":{"f":"updated"}}]`
		if mixed {
			code = "ATTACHMENT_WRITE_UNCONFIRMED"
			details["attachmentVerification"], details["clientToken"] = "unconfirmed", token
			details["createdRecordIds"] = []any{"created"}
			groups["create"] = map[string]any{"requested": true, "acceptedRecordIds": []any{"created"},
				"error": map[string]any{"code": code, "details": map[string]any{"attachmentVerification": "unconfirmed", "clientToken": token}}}
			records = `[{"cells":{"f":"created"}},{"recordId":"u1","cells":{"f":"updated"}}]`
		}
		raw, _ := json.Marshal(map[string]any{"status": "error", "error": map[string]any{"code": code, "details": details}})
		cause := recordMutationErrorForTest(t, "+record-upsert", string(raw))
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: cause}}}
		args := []string{"+record-upsert", "--base-id", "b", "--table-id", "t", "--records", records, "--yes"}
		if scenario.withToken {
			args = append(args, "--client-token", token)
		}
		_, err := runAITableCompositeCLI(t, caller, args[0], args[1:]...)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || !errors.Is(err, cause) || typed.Retryable || len(caller.calls) != 1 {
			t.Fatalf("uncertain update advanced or replayed: mixed=%v error=%#v calls=%#v", mixed, err, caller.calls)
		}
		result := typed.Details["result"].(compositeResult)
		projected, ok := result.Verification["groupOutcomes"].(map[string]any)
		if !ok {
			t.Fatalf("final composite receipt lost recovery groups: mixed=%v result=%#v", mixed, result)
		}
		update := projected["update"].(map[string]any)
		stage := update["error"].(map[string]any)["details"].(map[string]any)
		if result.Status != "unknown" || result.CompletedCount != 0 || stage["writeOutcome"] != "OUTCOME_UNKNOWN" ||
			!reflect.DeepEqual(update["requestedRecordIds"], []string{"u1"}) || !reflect.DeepEqual(update["acceptedRecordIds"], []string{}) ||
			!strings.Contains(result.Checkpoint["updateReadbackCommand"].(string), "--record-ids u1") {
			t.Fatalf("update reconciliation target/stage lost: %#v", result)
		}
		if !mixed && (!strings.Contains(result.NextCommand, "+record-query") || result.Checkpoint["clientToken"] != nil) {
			t.Fatalf("update-only recovery invented a creation token or omitted its read-only action: %#v", result)
		}
		for _, effect := range result.KnownEffects {
			if effect["group"] == "update" {
				t.Fatalf("unknown requested updates were promoted to accepted: %#v", result.KnownEffects)
			}
		}
	}
}

func recordMutationErrorForTest(t *testing.T, command, raw string) error {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	tool := "record_upsert"
	if command == "+record-update" {
		tool = "update_records"
	}
	return helpers.ClassifyAitableRecordMutationError("aitable", tool, body)
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

// A malformed typed recovery receipt preserves the stop-write obligation without
// inventing known IDs or retaining partially decoded targets.
func TestCrossPlatformCoverageMalformedTypedAttachmentRecovery(t *testing.T) {
	for _, ids := range []any{[]any{"record", 1}, make(chan int)} {
		err := apperrors.NewAPI("unconfirmed", apperrors.WithReason("ATTACHMENT_WRITE_UNCONFIRMED"),
			apperrors.WithDetails(map[string]any{"attachmentVerification": "unconfirmed", "recordIds": ids}))
		result := decodeRecordWriteFailure(err)
		if result.Error.Code != "ATTACHMENT_WRITE_UNCONFIRMED" || !result.needsReconciliation() ||
			len(result.Error.Details.AttachmentRecordIDs) != 0 || result.Error.Details.AttachmentVerification != "" {
			t.Fatalf("malformed receipt produced recovery metadata: %#v", result)
		}
	}
}
