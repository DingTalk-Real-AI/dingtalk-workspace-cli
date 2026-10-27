// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageAIPlanPreservesLegacyReadError(t *testing.T) {
	plan, path := aiPlanFixture(t)
	original := &helpers.CLIError{Code: helpers.CodeAuthPermission, Message: "access denied", Suggestion: "check original account", ServerCode: "FORBIDDEN"}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"status":"success","data":{"tasks":[{"fieldId":"F","taskId":"TASK","status":"submitted","recordIds":["R"]}]}}`},
		{err: original},
	}}
	_, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", path, "--yes")
	var preserved *helpers.CLIError
	if !errors.As(err, &preserved) || preserved.Code != original.Code || preserved.ServerCode != original.ServerCode || preserved.Suggestion != original.Suggestion || preserved.Details["result"] == nil {
		t.Fatalf("legacy permission error lost its classification/progress: %#v %v", preserved, err)
	}
	if len(caller.calls) != 2 || caller.calls[1].tool != "get_ai_field_task_status" {
		t.Fatalf("read failure replayed submission: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageAIWaitCancellationKeepsPriorCheckpointRows(t *testing.T) {
	_, path := setupBuildPlan(t)
	ids := make([]any, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("R%d", i)
	}
	target := map[string]any{"taskId": "TASK", "fieldId": "F", "recordIds": ids}
	raw, _ := json.Marshal([]any{target})
	args := []string{"--base-id", "B", "--table-id", "T", "--tasks", string(raw), "--checkpoint", path}
	first := &upsertByKeyCaller{callFn: func(index int, _, tool string, params map[string]any) (string, error) {
		if index > 0 {
			return "", context.Canceled
		}
		return aiChunkReceipt(params["tasks"].([]map[string]any)[0], true), nil
	}}
	if _, err := runAITableCompositeCLI(t, first, "+field-wait-ai", args...); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation identity lost: %v", err)
	}
	resume := &upsertByKeyCaller{callFn: func(_ int, _, tool string, params map[string]any) (string, error) {
		chunk := params["tasks"].([]map[string]any)[0]
		if tool != "get_ai_field_task_status" || len(chunk["recordIds"].([]any)) != 1 || chunk["recordIds"].([]any)[0] != "R50" {
			t.Fatalf("lost prior proof or replayed: %s %#v", tool, chunk)
		}
		return aiChunkReceipt(chunk, true), nil
	}}
	if _, err := runAITableCompositeCLI(t, resume, "+field-wait-ai", append(args, "--resume")...); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageTaskEvidenceFilesFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.json")
	for _, raw := range []string{
		`null`, `{`, `{"kind":"other"}`, `{"kind":"ai_wait","version":1}`, `{"version":1,"runId":"R"}`,
		`{"version":1,"runId":"R","identity":"I","planDigest":"D","baseId":"B","steps":[{"id":"x","status":"verified"}],"metrics":{"remoteCalls":-1}}`,
		`{"version":1,"runId":"R","identity":"I","planDigest":"D","baseId":"B","steps":[{"id":"x","status":"verified"}],"metrics":{"remoteCalls":1,"readCalls":2}}`,
	} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadAitableTaskReport(path, ""); err == nil {
			t.Fatalf("malformed evidence accepted: %s", raw)
		}
	}
	malformed := []map[string]any{{"taskId": "TASK", "fieldId": "F", "recordIds": nil}}
	if _, err := restoreAITaskPoll("B", "T", malformed, aiTaskEvidence{}); err == nil {
		t.Fatal("malformed targets did not stop restoration")
	}
	for _, args := range []map[string]any{
		{"tableId": "T", "fieldIds": []any{false}, "recordIds": []any{"R"}},
		{"tableId": "T", "fieldIds": []any{"F"}, "recordIds": []any{"R", "R"}},
	} {
		if _, err := aiPlanTargets(&buildPlanStepState{Arguments: args}); err == nil {
			t.Fatal("corrupted AI arguments accepted")
		}
	}
}

func TestCrossPlatformCoverageAIWaitCheckpointResumesOriginalTargets(t *testing.T) {
	_, path := setupBuildPlan(t)
	ids := make([]any, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("R%d", i)
	}
	target := map[string]any{"taskId": "TASK", "fieldId": "F", "recordIds": ids}
	raw, _ := json.Marshal([]any{target})
	args := []string{"--base-id", "B", "--table-id", "T", "--tasks", string(raw), "--timeout", "0s", "--checkpoint", path}
	first := &upsertByKeyCaller{callFn: func(_ int, _, tool string, params map[string]any) (string, error) {
		if tool != "get_ai_field_task_status" {
			t.Fatal(tool)
		}
		return aiChunkReceipt(params["tasks"].([]map[string]any)[0], false), nil
	}}
	if out, err := runAITableCompositeCLI(t, first, "+field-wait-ai", args...); err != nil || !strings.Contains(out, "--resume") {
		t.Fatalf("first wait: %s %v", out, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("checkpoint not private")
	}
	resume := &upsertByKeyCaller{callFn: func(_ int, _, tool string, params map[string]any) (string, error) {
		chunk := params["tasks"].([]map[string]any)[0]
		if tool != "get_ai_field_task_status" || len(chunk["recordIds"].([]any)) != 1 || chunk["recordIds"].([]any)[0] != "R50" {
			t.Fatalf("queried old rows or replayed: %s %#v", tool, chunk)
		}
		return aiChunkReceipt(chunk, true), nil
	}}
	out, err := runAITableCompositeCLI(t, resume, "+field-wait-ai", append(args, "--resume")...)
	if err != nil || !strings.Contains(out, `"terminalVerified": true`) || !strings.Contains(out, `"resultReadableVerified": false`) {
		t.Fatalf("resume: %s %v", out, err)
	}
	var saved aiWaitCheckpoint
	if err := readAitableTaskFile(path, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Metrics.Invocations != 2 || saved.Metrics.RemoteCalls != 2 || saved.Metrics.WriteCalls != 0 {
		t.Fatalf("metrics: %#v", saved.Metrics)
	}
	if _, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+field-wait-ai", args...); err == nil {
		t.Fatal("overwrote checkpoint")
	}
	changed := append([]string(nil), args...)
	changed[5] = strings.Replace(string(raw), "TASK", "OTHER", 1)
	if _, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+field-wait-ai", append(changed, "--resume")...); err == nil {
		t.Fatal("resumed different task")
	}
	report, err := loadAitableTaskReport(path, "")
	if err != nil || report.Status != "terminal_results_unverified" || report.ModelMetrics != nil {
		t.Fatalf("report invented readability/model metrics: %#v %v", report, err)
	}
}

func aiPlanFixture(t *testing.T) (string, string) {
	t.Helper()
	plan, path := setupBuildPlan(t)
	raw := `{"version":1,"baseId":"B","steps":[{"id":"run","tool":"run_ai_field","arguments":{"baseId":"B","tableId":"T","fieldIds":["F"],"recordIds":["R"]},"verify":{"tool":"get_ai_field_task_status","arguments":{"baseId":"B","tableId":"T","tasks":{"$ref":"run.taskTargets"}},"expected":{"data":{"terminalVerified":true,"resultReadableVerified":true}}}}]}`
	if err := os.WriteFile(plan, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	return plan, path
}

func TestCrossPlatformCoverageAIPlanWaitPersistsWithoutResubmitting(t *testing.T) {
	plan, path := aiPlanFixture(t)
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"status":"success","data":{"tasks":[{"fieldId":"F","taskId":"TASK","status":"submitted","recordIds":["R"]}]}}`},
		{text: aiChunkReceipt(map[string]any{"taskId": "TASK", "fieldId": "F", "recordIds": []any{"R"}}, true)},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", path, "--yes")
	if err != nil || !strings.Contains(out, `"outcome": "pending"`) || len(caller.calls) != 2 {
		t.Fatalf("plan: %s %v %#v", out, err, caller.calls)
	}
	var saved buildPlanState
	if err := readAitableTaskFile(path, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Steps[0].AIProgress == nil || saved.Metrics.WriteCalls != 1 || saved.Metrics.ReadCalls != 1 || saved.Steps[0].Status != "pending" {
		t.Fatalf("lost AI evidence: %#v", saved)
	}
	resume := &upsertByKeyCaller{}
	out, err = runAITableCompositeCLI(t, resume, "+build-plan", "--plan", plan, "--checkpoint", path, "--resume", "--yes")
	if err != nil || len(resume.calls) != 0 || !strings.Contains(out, `"resultReadableVerified": false`) {
		t.Fatalf("replayed AI or claimed readable: %s %v %#v", out, err, resume.calls)
	}
}

func TestCrossPlatformCoverageAIPlanLostSubmissionCannotReplay(t *testing.T) {
	plan, path := aiPlanFixture(t)
	first := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: errors.New("delivery unknown")}}}
	if _, err := runAITableCompositeCLI(t, first, "+build-plan", "--plan", plan, "--checkpoint", path, "--yes"); err == nil {
		t.Fatal("unknown accepted")
	}
	resume := &upsertByKeyCaller{}
	if _, err := runAITableCompositeCLI(t, resume, "+build-plan", "--plan", plan, "--checkpoint", path, "--resume", "--yes"); err == nil || len(resume.calls) != 0 {
		t.Fatalf("replayed unknown task: %v %#v", err, resume.calls)
	}
	data, _ := os.ReadFile(plan)
	for _, invalid := range []string{strings.Replace(string(data), `"recordIds":["R"]`, `"recordIds":[]`, 1), strings.Replace(string(data), `"resultReadableVerified":true`, `"resultReadableVerified":false`, 1)} {
		os.WriteFile(plan, []byte(invalid), 0600)
		if _, _, err := readBuildPlan(plan); err == nil {
			t.Fatal("unfixed targets or weak proof accepted")
		}
	}
}

func TestCrossPlatformCoverageBuildPlanChecksRealGridBeforeCreate(t *testing.T) {
	plan, path := setupBuildPlan(t)
	raw := `{"version":1,"baseId":"B","steps":[{"id":"chart","tool":"create_chart","arguments":{"baseId":"B","dashboardId":"D","config":{"chartType":"AI_ANALYZE"},"layout":{"x":12,"y":0,"w":1,"h":4}},"verify":{"tool":"get_chart","arguments":{"baseId":"B","dashboardId":"D","chartId":{"$ref":"chart.chartId"}},"expected":{"data":{"chartId":{"$ref":"chart.chartId"}}}}}]}`
	os.WriteFile(plan, []byte(raw), 0600)
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"status":"success","data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":"2","schemaVersionTypeVerified":true},"layout":[]}}`}}}
	if _, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", path, "--yes"); err == nil || len(caller.calls) != 1 || caller.calls[0].tool != "get_dashboard" {
		t.Fatalf("invalid grid reached mutation: %v %#v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageTaskReportIsOfflineAndRejectsFalseComparisons(t *testing.T) {
	dir := t.TempDir()
	path, baseline := filepath.Join(dir, "task.json"), filepath.Join(dir, "baseline.json")
	state := buildPlanState{Version: 1, Digest: strings.Repeat("a", 64), Identity: "identity", BaseID: "B", RunID: "current", InvocationComplete: true, Status: "verified", ElapsedMS: 10, Steps: []buildPlanStepState{{ID: "one", Status: "verified"}}, Metrics: aitableTaskMetrics{Invocations: 1, RemoteCalls: 1, ReadCalls: 1}}
	saveBuildPlanState(path, &state)
	state.RunID, state.ElapsedMS = "baseline", 20
	saveBuildPlanState(baseline, &state)
	caller := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, caller, "+task-report", "--checkpoint", path, "--baseline", baseline)
	if err != nil || len(caller.calls) != 0 || !strings.Contains(out, `"comparable": false`) {
		t.Fatalf("invented model comparability: %s %v", out, err)
	}
	model := filepath.Join(dir, "model.json")
	data, _ := json.Marshal(map[string]any{"model": "test-model", "environment": "isolated-fixture", "inputDigest": state.Digest, "interactions": 2})
	os.WriteFile(model, data, 0600)
	out, err = runAITableCompositeCLI(t, caller, "+task-report", "--checkpoint", path, "--baseline", baseline, "--model-metrics", model, "--baseline-model-metrics", model)
	if err != nil || !strings.Contains(out, `"elapsedDeltaMs": -10`) || !strings.Contains(out, `"comparable": true`) {
		t.Fatalf("explicit comparable inputs: %s %v", out, err)
	}
	state.Status = "pending"
	saveBuildPlanState(baseline, &state)
	out, err = runAITableCompositeCLI(t, caller, "+task-report", "--checkpoint", path, "--baseline", baseline, "--model-metrics", model, "--baseline-model-metrics", model)
	if err != nil || strings.Contains(out, `"elapsedDeltaMs"`) {
		t.Fatalf("pending became performance proof: %s %v", out, err)
	}
}
