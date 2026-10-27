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

func pendingReviewReceipt(t *testing.T, target map[string]any) string {
	t.Helper()
	var response map[string]any
	if err := json.Unmarshal([]byte(aiChunkReceipt(target, true)), &response); err != nil {
		t.Fatal(err)
	}
	body := parityResponseObject(response)
	tasks, _ := findNamedObjectList(body, "tasks")
	rows, _ := findNamedObjectList(tasks[0], "rows")
	for _, row := range rows {
		row["state"], row["terminal"] = "writing", false
		delete(row, "resultVersion")
	}
	tasks[0]["terminalVerified"], body["terminalVerified"], body["resultVersionAvailable"] = false, false, false
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCrossPlatformCoverageAIWaitCheckpointRotatesAllPendingTargets(t *testing.T) {
	for _, acrossFields := range []bool{false, true} {
		t.Run(fmt.Sprintf("fields=%v", acrossFields), func(t *testing.T) {
			_, path := setupBuildPlan(t)
			targets := boundaryTargets()
			if acrossFields {
				targets = append(targets, map[string]any{"taskId": "TASK2", "fieldId": "F2", "recordIds": []any{"Z"}})
			} else {
				ids := make([]any, 51)
				for i := range ids {
					ids[i] = fmt.Sprintf("R%d", i)
				}
				targets[0]["recordIds"] = ids
			}
			raw, _ := json.Marshal(targets)
			args := []string{"--base-id", "B", "--table-id", "T", "--tasks", string(raw), "--checkpoint", path, "--timeout", "0s"}
			for invocation := 0; invocation < 4; invocation++ {
				caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
					if tool != "get_ai_field_task_status" {
						t.Fatal("AI write was replayed", tool)
					}
					batch := args["tasks"].([]map[string]any)[0]
					wantField, wantRecord := "F", "R0"
					if acrossFields {
						wantRecord = "R"
					}
					if invocation%2 == 1 {
						if acrossFields {
							wantField, wantRecord = "F2", "Z"
						} else {
							wantRecord = "R50"
						}
					}
					ids := batch["recordIds"].([]any)
					if batch["fieldId"] != wantField || ids[0] != wantRecord || len(ids) > 50 {
						t.Fatalf("invocation=%d starved pending target: %#v", invocation, batch)
					}
					return pendingReviewReceipt(t, batch), nil
				}}
				callArgs := append([]string(nil), args...)
				if invocation > 0 {
					callArgs = append(callArgs, "--resume")
				}
				out, err := runAITableCompositeCLI(t, caller, "+field-wait-ai", callArgs...)
				if err != nil || len(caller.calls) != 1 || !strings.Contains(out, `"outcome": "pending"`) {
					t.Fatalf("out=%s err=%v calls=%d", out, err, len(caller.calls))
				}
			}
		})
	}
}

func TestCrossPlatformCoverageBatchRequiresAffirmativeItemReceipt(t *testing.T) {
	for _, item := range []string{
		`{"index":0,"chartId":"C","success":true}`,
		`{"index":0,"chartId":"C","status":"accepted"}`,
		`{"index":0,"chartId":"C","status":"new_state","success":true}`,
		`{"index":0,"chartId":"C","status":"accepted","success":"true"}`,
	} {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`},
			{text: `{"data":{"baseId":"B","dashboardId":"D","results":[` + item + `]}}`},
			{text: `{"data":{"baseId":"B","dashboardId":"D","chartId":"C","config":{"chartType":"AI_ANALYZE"},"layout":{"x":0,"y":0,"w":12,"h":4}}}`},
		}}
		if _, err := runAITableCompositeCLI(t, caller, "+charts-create", "--base-id", "B", "--dashboard-id", "D", "--charts", `[{"config":{"chartType":"AI_ANALYZE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`, "--server-batch", "--yes"); err == nil || len(caller.calls) != 2 {
			t.Fatalf("incomplete acceptance became verified: item=%s err=%v calls=%d", item, err, len(caller.calls))
		}
	}
}

func TestCrossPlatformCoverageBatchKeepsProviderVerificationSeparateFromClientReadback(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`},
		{text: `{"data":{"baseId":"B","dashboardId":"D","results":[{"index":0,"chartId":"C","success":true,"status":"verified"}]}}`},
		{err: errors.New("read unavailable")},
	}}
	_, err := runAITableCompositeCLI(t, caller, "+charts-create", "--base-id", "B", "--dashboard-id", "D", "--charts", `[{"config":{"chartType":"AI_ANALYZE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`, "--server-batch", "--yes")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || len(caller.calls) != 3 {
		t.Fatal(err)
	}
	result := typed.Details["result"].(map[string]any)
	items := result["items"].([]map[string]any)
	if items[0]["status"] != "accepted" || result["verifiedCount"] != 0 {
		t.Fatalf("client claimed verification: %#v", result)
	}
	receipts, _ := findNamedObjectList(result["receipt"].(map[string]any), "results")
	if receipts[0]["status"] != "verified" {
		t.Fatal("provider receipt was mutated")
	}
}
