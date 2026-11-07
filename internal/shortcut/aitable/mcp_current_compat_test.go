// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// aiChunkReceipt models current MCP's precise row evidence, including mixed
// provider capability and incomplete-read flags without inventing success.
func aiChunkReceipt(target map[string]any, complete bool) string {
	rows := []any{}
	for _, id := range target["recordIds"].([]any) {
		rows = append(rows, map[string]any{"taskId": target["taskId"], "fieldId": target["fieldId"], "recordId": id,
			"state": "succeeded", "terminal": true, "resultVersion": 7})
	}
	task := map[string]any{"taskId": target["taskId"], "fieldId": target["fieldId"], "recordIds": target["recordIds"], "rows": rows, "terminalVerified": true}
	raw, _ := json.Marshal(map[string]any{"status": "success", "data": map[string]any{"baseId": "B", "tableId": "T", "tasks": []any{task},
		"terminalVerified": true, "successTerminalVerifiable": false, "resultVersionAvailable": true, "resultReadableVerified": false,
		"queryComplete": complete, "queryRetryable": !complete, "queryBudgetExhausted": !complete}})
	return string(raw)
}

// A checkpoint round trip must retain an incomplete budget-limited read, while
// complete row evidence still clears the aggregate exhaustion flag.
func TestCrossPlatformCoverageAIWaitRestoresQueryBudgetEvidence(t *testing.T) {
	target := map[string]any{"taskId": "TASK", "fieldId": "F", "recordIds": []any{"R"}}
	for _, complete := range []bool{false, true} {
		poll := newAITaskPoll("B", "T", []map[string]any{target})
		var wire map[string]any
		if err := json.Unmarshal([]byte(aiChunkReceipt(target, complete)), &wire); err != nil {
			t.Fatal(err)
		}
		poll.merge(wire["data"].(map[string]any))
		restored, err := restoreAITaskPoll("B", "T", []map[string]any{target}, poll.evidence())
		if err != nil || restored.body["queryBudgetExhausted"] != !complete {
			t.Fatalf("lost budget evidence: complete=%v restored=%+v err=%v", complete, restored, err)
		}
	}
}

func TestCrossPlatformCoverageAIWaitQueries500TargetsOnceWithoutLosingMixedEvidence(t *testing.T) {
	ids := make([]any, 500)
	for i := range ids {
		ids[i] = fmt.Sprintf("R%d", i)
	}
	target := map[string]any{"taskId": "TASK", "fieldId": "F", "recordIds": ids}
	raw, _ := json.Marshal([]any{target})
	seen := map[string]bool{}
	caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
		if tool != "get_ai_field_task_status" {
			t.Fatal("AI mutation/replay", tool)
		}
		tasks := args["tasks"].([]map[string]any)
		if len(tasks) != 1 || len(tasks[0]["recordIds"].([]any)) > 50 {
			t.Fatal("unbounded query", tasks)
		}
		for _, id := range tasks[0]["recordIds"].([]any) {
			if seen[id.(string)] {
				t.Fatal("requeried terminal row", id)
			}
			seen[id.(string)] = true
		}
		return aiChunkReceipt(tasks[0], true), nil
	}}
	out, err := runAITableCompositeCLI(t, caller, "+field-wait-ai", "--base-id", "B", "--table-id", "T", "--tasks", string(raw), "--timeout", "2s")
	if err != nil || len(seen) != 500 || len(caller.calls) != 10 || !strings.Contains(out, `"terminalVerified": true`) || !strings.Contains(out, `"outcome": "pending"`) || !strings.Contains(out, `"R499"`) {
		t.Fatalf("lost tail or rejected mixed provider evidence: calls=%d rows=%d err=%v out=%s", len(caller.calls), len(seen), err, out)
	}
}

func TestCrossPlatformCoverageAIWaitRetainsPriorRowsWhenLaterChunkFails(t *testing.T) {
	ids := make([]any, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("R%d", i)
	}
	raw, _ := json.Marshal([]any{map[string]any{"taskId": "TASK", "fieldId": "F", "recordIds": ids}})
	cause := apperrors.NewAuth("Read permission unavailable", apperrors.WithReason("permission_dependency"), apperrors.WithRetryable(true))
	caller := &upsertByKeyCaller{callFn: func(i int, _, _ string, args map[string]any) (string, error) {
		if i == 1 {
			return "", cause
		}
		return aiChunkReceipt(args["tasks"].([]map[string]any)[0], true), nil
	}}
	_, err := runAITableCompositeCLI(t, caller, "+field-wait-ai", "--base-id", "B", "--table-id", "T", "--tasks", string(raw), "--timeout", "2s")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryAuth || !typed.Retryable || len(caller.calls) != 2 {
		t.Fatalf("changed classification or retried: %v", err)
	}
	result := typed.Details["result"].(map[string]any)
	tasks, _ := findNamedObjectList(result, "tasks")
	rows, _ := findNamedObjectList(tasks[0], "rows")
	if len(rows) != 51 || rows[49]["state"] != "succeeded" || rows[50]["state"] != "unknown" || result["queryComplete"] != false {
		t.Fatalf("lost prior rows: %#v", result)
	}
}

func TestCrossPlatformCoverageChartBatchKeepsCandidatesFromErrorAndPreflightIsNotUnknown(t *testing.T) {
	const charts = `[{"config":{"chartType":"LINE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`
	for _, preflight := range []bool{false, true} {
		response := `{"status":"error","data":{"baseId":"B","dashboardId":"D","results":[{"index":0,"chartId":"C","status":"unknown","success":false}]},"error":{"code":"HSF_FAILED","message":"Delivery unknown","retryable":false,"details":{"deliveryStatus":"unknown"}}}`
		if preflight {
			response = `{"status":"error","error":{"code":"DASHBOARD_WRITE_PERMISSION_UNAVAILABLE","message":"No write submitted","retryable":true,"details":{"mutationSubmitted":false,"stage":"document_edit"}}}`
		}
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`}, {text: response}}}
		_, err := runAITableCompositeCLI(t, caller, "+charts-create", "--base-id", "B", "--dashboard-id", "D", "--charts", charts, "--server-batch", "--yes")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || len(caller.calls) != 2 {
			t.Fatalf("lost error or replayed: %v", err)
		}
		if preflight {
			if typed.ExecutionStarted == nil || *typed.ExecutionStarted || !typed.Retryable {
				t.Fatalf("preflight became unknown: %#v", typed)
			}
		} else {
			encoded, _ := json.Marshal(typed.Details)
			if !strings.Contains(string(encoded), `"chartId":"C"`) || typed.Retryable {
				t.Fatalf("lost candidate: %s", encoded)
			}
		}
	}
}

func TestCrossPlatformCoverageAIWaitPartialZeroBudgetKeepsAllTargetsAndAdvancesContinuation(t *testing.T) {
	ids := make([]any, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("R%d", i)
	}
	raw, _ := json.Marshal([]any{map[string]any{"taskId": "TASK", "fieldId": "F", "recordIds": ids}, map[string]any{"taskId": "TASK2", "fieldId": "F2", "recordIds": []any{"Z"}}})
	caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
		batch := args["tasks"].([]map[string]any)
		var body map[string]any
		if err := json.Unmarshal([]byte(aiChunkReceipt(batch[0], false)), &body); err != nil {
			t.Fatal(err)
		}
		data := body["data"].(map[string]any)
		tasks, _ := findNamedObjectList(data, "tasks")
		rows, _ := findNamedObjectList(tasks[0], "rows")
		for _, row := range rows {
			row["state"], row["terminal"] = "unknown", false
			delete(row, "resultVersion")
		}
		tasks[0]["terminalVerified"], data["terminalVerified"], data["resultVersionAvailable"] = false, false, false
		encoded, _ := json.Marshal(body)
		return string(encoded), nil
	}}
	out, err := runAITableCompositeCLI(t, caller, "+field-wait-ai", "--base-id", "B", "--table-id", "T", "--tasks", string(raw), "--timeout", "0s")
	if err != nil || len(caller.calls) != 1 {
		t.Fatalf("0s should query one bounded chunk: %s %v", out, err)
	}
	var wire map[string]any
	if err = json.Unmarshal([]byte(out), &wire); err != nil {
		t.Fatal(err)
	}
	next := findStringByKeys(wire, "next_command")
	if next == "" || strings.Index(next, `F2`) > strings.Index(next, `"fieldId":"F"`) || strings.Index(next, `R50`) > strings.Index(next, `R0`) {
		t.Fatalf("continuation does not advance untouched field/tail: %s", next)
	}
	data := wire["data"].(map[string]any)
	tasks, _ := findNamedObjectList(data, "tasks")
	rows, _ := findNamedObjectList(tasks[0], "rows")
	if len(rows) != 51 || data["queryComplete"] != false || data["queryBudgetExhausted"] != true || data["terminalVerified"] != false {
		t.Fatalf("lost original unknown targets: %s", out)
	}
}
