// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupBuildPlan(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")
	if err := os.MkdirAll(authDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DWS_CONFIG_DIR", authDir)
	if err := os.WriteFile(filepath.Join(authDir, "profiles.json"), []byte(`{"version":3,"currentProfile":"corp:user","profiles":[{"name":"test","corpId":"corp","userId":"user","status":"active"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(dir, "plan.json")
	checkpoint := filepath.Join(dir, "task.json")
	raw := `{"version":1,"baseId":"b","steps":[{"id":"board","tool":"create_dashboard","arguments":{"baseId":"b","config":{"name":"看板"}},"verify":{"tool":"get_dashboard","arguments":{"baseId":"b","dashboardId":{"$ref":"board.dashboardId"}},"expected":{"data":{"baseId":"b","dashboardId":{"$ref":"board.dashboardId"},"dashboardName":"看板"}}}}]}`
	if err := os.WriteFile(plan, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	return plan, checkpoint
}

func TestCrossPlatformCoverageBuildPlanResumesByReadbackWithoutRecreating(t *testing.T) {
	plan, checkpoint := setupBuildPlan(t)
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"status":"success","data":{"baseId":"b","dashboardId":"d"}}`}, {err: errors.New("read unavailable")}}}
	_, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", checkpoint, "--yes")
	if err == nil {
		t.Fatal("failed readback claimed complete")
	}
	raw, err := os.ReadFile(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	var saved buildPlanState
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Steps[0].Resources["dashboardId"] != "d" || saved.Steps[0].Status != "unknown" || len(saved.Calls) != 2 {
		t.Fatalf("lost checkpoint: %s", raw)
	}
	info, _ := os.Stat(checkpoint)
	if info.Mode().Perm() != 0600 {
		t.Fatal("checkpoint not private")
	}
	read := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"status":"success","data":{"baseId":"b","dashboardId":"d","dashboardName":"看板"}}`}}}
	out, err := runAITableCompositeCLI(t, read, "+build-plan", "--plan", plan, "--checkpoint", checkpoint, "--resume", "--yes")
	if err != nil || !strings.Contains(out, `"status": "verified"`) {
		t.Fatalf("resume: %s %v", out, err)
	}
	if len(read.calls) != 1 || read.calls[0].tool != "get_dashboard" {
		t.Fatalf("resume rewrote: %#v", read.calls)
	}
	verify := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"status":"success","data":{"baseId":"b","dashboardId":"d","dashboardName":"看板"}}`}}}
	if _, err = runAITableCompositeCLI(t, verify, "+build-plan-verify", "--plan", plan, "--checkpoint", checkpoint); err != nil {
		t.Fatal(err)
	}
	if _, err = runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+build-plan", "--plan", plan, "--checkpoint", checkpoint, "--yes"); err == nil {
		t.Fatal("overwrote original task")
	}
}

func TestCrossPlatformCoverageBuildPlanUnknownWriteCannotBeReplayed(t *testing.T) {
	plan, checkpoint := setupBuildPlan(t)
	write := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: errors.New("delivery unknown")}}}
	if _, err := runAITableCompositeCLI(t, write, "+build-plan", "--plan", plan, "--checkpoint", checkpoint, "--yes"); err == nil {
		t.Fatal("lost response succeeded")
	}
	resume := &upsertByKeyCaller{}
	if _, err := runAITableCompositeCLI(t, resume, "+build-plan", "--plan", plan, "--checkpoint", checkpoint, "--resume", "--yes"); err == nil || len(resume.calls) != 0 {
		t.Fatal("unknown create was replayed or verified without ID")
	}
	changed, _ := os.ReadFile(plan)
	os.WriteFile(plan, []byte(strings.Replace(string(changed), "看板", "新计划", 1)), 0600)
	if _, err := runAITableCompositeCLI(t, resume, "+build-plan-verify", "--plan", plan, "--checkpoint", checkpoint); err == nil {
		t.Fatal("reused mismatched digest")
	}
}

func TestCrossPlatformCoverageBuildPlanPreflightsScopeAndReferences(t *testing.T) {
	plan, _ := setupBuildPlan(t)
	raw, _ := os.ReadFile(plan)
	for _, value := range []string{strings.Replace(string(raw), `"create_dashboard"`, `"delete_dashboard"`, 1), strings.Replace(string(raw), `"baseId":"b"`, `"baseId":"other"`, 1), strings.Replace(string(raw), `"board.dashboardId"`, `"unknown.dashboardId"`, 1), strings.Replace(string(raw), `"version":1`, `"version":2`, 1), string(raw) + `{}`, `{"version":1,"baseId":"b","steps":[]}`} {
		os.WriteFile(plan, []byte(value), 0600)
		if _, _, err := readBuildPlan(plan); err == nil {
			t.Fatalf("bad plan accepted: %s", value)
		}
	}
}

func TestCrossPlatformCoverageBuildPlanReconcilesLostRecordReceiptWithoutRewriting(t *testing.T) {
	plan, checkpoint := setupBuildPlan(t)
	value := `{"version":1,"baseId":"b","steps":[{"id":"rows","tool":"create_records","arguments":{"baseId":"b","tableId":"t","records":[{"cells":{"F":"v"}}]},"verify":{"tool":"query_records","arguments":{"baseId":"b","tableId":"t","recordIds":{"$ref":"rows.newRecordIds"}},"expected":{"data":{"records":[{"cells":{"F":"v"}}]}}}}]}`
	if err := os.WriteFile(plan, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	first := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: errors.New("delivery unknown")}}}
	if _, err := runAITableCompositeCLI(t, first, "+build-plan", "--plan", plan, "--checkpoint", checkpoint, "--yes"); err == nil {
		t.Fatal("unknown write completed")
	}
	var saved buildPlanState
	raw, _ := os.ReadFile(checkpoint)
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	token := stringValue(saved.Steps[0].Arguments, "clientToken")
	if token == "" {
		t.Fatal("original clientToken lost")
	}
	for _, body := range []string{
		fmt.Sprintf(`{"data":{"baseId":"b","tableId":"t","clientToken":%q,"state":"unknown","recordIds":[]}}`, token),
		`{"data":{"baseId":"b","tableId":"t","clientToken":"wrong","state":"applied","recordIds":["r"]}}`,
	} {
		read := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: body}}}
		if _, err := runAITableCompositeCLI(t, read, "+build-plan-verify", "--plan", plan, "--checkpoint", checkpoint); err == nil || len(read.calls) != 1 || read.calls[0].tool != "get_record_write_result" {
			t.Fatalf("unsafe reconciliation: %v %#v", err, read.calls)
		}
	}
	read := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: fmt.Sprintf(`{"data":{"baseId":"b","tableId":"t","clientToken":%q,"state":"applied","recordIds":["r"]}}`, token)}, {text: `{"data":{"records":[{"recordId":"r","cells":{"F":"v"}}]}}`}}}
	out, err := runAITableCompositeCLI(t, read, "+build-plan", "--plan", plan, "--checkpoint", checkpoint, "--resume", "--yes")
	if err != nil || !strings.Contains(out, `"status": "verified"`) {
		t.Fatalf("reconciliation: %s %v", out, err)
	}
	if len(read.calls) != 2 || read.calls[0].tool != "get_record_write_result" || read.calls[1].tool != "query_records" || read.calls[0].args["clientToken"] != token {
		t.Fatalf("replayed or retargeted: %#v", read.calls)
	}
}

func TestCrossPlatformCoverageBuildPlanRejectsChangedGatewayBeforeAnyCalls(t *testing.T) {
	plan, checkpoint := setupBuildPlan(t)
	first := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: errors.New("delivery unknown")}}}
	if _, err := runAITableCompositeCLI(t, first, "+build-plan", "--plan", plan, "--checkpoint", checkpoint, "--yes"); err == nil {
		t.Fatal("unknown write completed")
	}
	t.Setenv("DINGTALK_AITABLE_MCP_URL", "https://other.test/mcp")
	read := &upsertByKeyCaller{}
	if _, err := runAITableCompositeCLI(t, read, "+build-plan-verify", "--plan", plan, "--checkpoint", checkpoint); err == nil || len(read.calls) != 0 {
		t.Fatal("reused checkpoint across gateway context")
	}
}

func TestCrossPlatformCoverageBuildPlanVerifiesViewPublicProjection(t *testing.T) {
	state := buildPlanState{BaseID: "b"}
	step := buildPlanStep{Tool: "update_view"}
	current := buildPlanStepState{Arguments: map[string]any{"baseId": "b", "tableId": "t", "viewId": "v", "newViewName": "new", "config": map[string]any{"sort": []any{map[string]any{"fieldId": "F", "desc": true}}, "group": []any{}}}}
	raw := map[string]any{"data": map[string]any{"views": []any{map[string]any{"viewId": "v", "viewName": "new", "viewType": "Grid", "sort": []any{map[string]any{"fieldId": "F", "desc": true}}, "group": []any{}}}}}
	if err := verifyBuildPlanIntent(nil, &state, step, &current, raw); err != nil {
		t.Fatal(err)
	}
	view := raw["data"].(map[string]any)["views"].([]any)[0].(map[string]any)
	view["sort"] = []any{}
	if err := verifyBuildPlanIntent(nil, &state, step, &current, raw); err == nil {
		t.Fatal("view summary replaced full configuration")
	}
}

func TestCrossPlatformCoverageBuildPlanCellAliasesBindRealFieldsAndRejectAmbiguity(t *testing.T) {
	resources := map[string]map[string]any{"table": {"fieldsByName": map[string]any{"金额": "F"}}}
	bindings, err := resolvePlanFieldBindings(map[string]any{"amount": map[string]any{"$ref": "table.fieldsByName.金额"}}, resources)
	if err != nil {
		t.Fatal(err)
	}
	rows := []any{map[string]any{"cells": map[string]any{"amount": float64(100)}}}
	bound, err := bindPlanRecordCells(rows, bindings)
	if err != nil || bound[0].(map[string]any)["cells"].(map[string]any)["F"] != float64(100) {
		t.Fatalf("%#v %v", bound, err)
	}
	if _, changed := rows[0].(map[string]any)["cells"].(map[string]any)["F"]; changed {
		t.Fatal("mutated original plan")
	}
	if _, err := resolvePlanFieldBindings(map[string]any{"a": "F", "b": "F"}, resources); err == nil {
		t.Fatal("ambiguous field binding accepted")
	}
	if _, err := bindPlanRecordCells(rows, map[string]string{"other": "F"}); err == nil {
		t.Fatal("unknown cell alias accepted")
	}
}

func TestCrossPlatformCoverageChartBatchUsesOneSubmissionAndFullConfigReadback(t *testing.T) {
	charts := `[{"config":{"name":"趋势","chartType":"LINE","sheet":"t"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"baseId":"b","dashboardId":"d","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`}, {text: `{"data":{"baseId":"b","dashboardId":"d","results":[{"index":0,"chartId":"c","status":"accepted","success":true}]}}`}, {text: `{"data":{"baseId":"b","dashboardId":"d","chartId":"c","chartName":"趋势","config":{"chartType":"LINE","sheet":"t"},"layout":{"x":0,"y":0,"w":12,"h":4}}}`}}}
	out, err := runAITableCompositeCLI(t, caller, "+charts-create", "--base-id", "b", "--dashboard-id", "d", "--charts", charts, "--server-batch", "--yes")
	if err != nil || !strings.Contains(out, `"verifiedCount": 1`) {
		t.Fatalf("%s %v", out, err)
	}
	if len(caller.calls) != 3 || caller.calls[1].tool != "create_charts" {
		t.Fatal(caller.calls)
	}
	if verifyChartConfiguration(map[string]any{"config": map[string]any{"chartType": "LINE"}, "layout": map[string]any{}}, map[string]any{"sheet": "t"}, nil) {
		t.Fatal("summary accepted as full config")
	}
}

func TestCrossPlatformCoverageAIWaitStopsAtTerminalWithResultsPending(t *testing.T) {
	tasks := `[{"taskId":"task","fieldId":"f","recordIds":["r"]}]`
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"baseId":"b","tableId":"t","tasks":[{"taskId":"task","fieldId":"f","recordIds":["r"],"rows":[{"taskId":"task","fieldId":"f","recordId":"r","state":"succeeded","terminal":true,"resultVersion":7}],"terminalVerified":true}],"terminalVerified":true,"resultReadableVerified":false,"resultVersionAvailable":true}}`}}}
	out, err := runAITableCompositeCLI(t, caller, "+field-wait-ai", "--base-id", "b", "--table-id", "t", "--tasks", tasks, "--timeout", "0s")
	if err != nil || !strings.Contains(out, `"outcome": "pending"`) || len(caller.calls) != 1 {
		t.Fatalf("%s %v", out, err)
	}
	for _, value := range []string{`[]`, `[{"taskId":"task","fieldId":"f","recordIds":[]}]`, `[{"taskId":"task","fieldId":"f","recordIds":["r","r"]}]`} {
		if _, err := parseAITaskTargets(value); err == nil {
			t.Fatal("invalid task targets accepted")
		}
	}
}
