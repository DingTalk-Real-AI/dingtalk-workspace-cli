// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"os"
	"testing"
)

func saveBoundaryPlan(t *testing.T, planPath, path string, plan buildPlan, states []buildPlanStepState) {
	t.Helper()
	writeBoundaryJSON(t, planPath, plan)
	parsed, digest, err := readBuildPlan(planPath)
	if err != nil {
		t.Fatal(err)
	}
	rt := boundaryRuntime(t, ChartsCreate, nil, nil, &upsertByKeyCaller{})
	identity, err := buildPlanIdentity(rt)
	if err != nil {
		t.Fatal(err)
	}
	state := buildPlanState{Version: 1, RunID: "run", Digest: digest, Identity: identity, BaseID: parsed.BaseID, Steps: states}
	if err := saveBuildPlanState(path, &state); err != nil {
		t.Fatal(err)
	}
}
func TestCrossPlatformCoveragePlanBoundReferencesAndAliases(t *testing.T) {
	for _, name := range []string{"write missing reference", "write invalid alias", "write missing alias", "verify missing reference", "expected missing reference", "verify invalid alias", "expected unknown alias", "wrong table", "AI invalid binding", "chart preflight"} {
		t.Run(name, func(t *testing.T) {
			planPath, path := setupBuildPlan(t)
			p, _, err := readBuildPlan(planPath)
			if err != nil {
				t.Fatal(err)
			}
			args := boundaryObject(t, `{"baseId":"b","tableId":"t","records":[{"recordId":"R","cells":{"alias":"v"}}]}`)
			verify := buildPlanReadback{Tool: "query_records", Arguments: boundaryObject(t, `{"baseId":"b","tableId":"t"}`), Expected: boundaryObject(t, `{"data":{"records":[{"cells":{"alias":"v"}}]}}`)}
			step := buildPlanStep{ID: "rows", DependsOn: []string{"board"}, Tool: "update_records", Arguments: args, Verify: verify, FieldBindings: map[string]any{"alias": map[string]any{"$ref": "board.field"}}}
			resources := map[string]any{"dashboardId": "d", "field": "F"}
			status := "planned"
			switch name {
			case "write missing reference":
				args["tableId"] = map[string]any{"$ref": "board.missing"}
			case "write invalid alias":
				resources["field"] = false
			case "write missing alias":
				args["records"] = []any{map[string]any{"recordId": "R", "cells": map[string]any{"unknown": "v"}}}
			case "verify missing reference":
				status = "accepted"
				step.Verify.Arguments["tableId"] = map[string]any{"$ref": "board.missing"}
			case "expected missing reference":
				status = "accepted"
				step.Verify.Expected["data"].(map[string]any)["x"] = map[string]any{"$ref": "board.missing"}
			case "verify invalid alias":
				status = "accepted"
				resources["field"] = false
			case "expected unknown alias":
				status = "accepted"
				step.Verify.Expected["data"].(map[string]any)["records"] = []any{map[string]any{"cells": map[string]any{"unknown": "v"}}}
			case "wrong table":
				status = "accepted"
				step.Verify.Arguments["tableId"] = "other"
			case "AI invalid binding":
				step.Tool = "run_ai_field"
				step.FieldBindings = nil
				step.Arguments = boundaryObject(t, `{"baseId":"b","tableId":"t","fieldIds":{"$ref":"board.field"},"recordIds":["R"]}`)
				step.Verify = buildPlanReadback{Tool: "get_ai_field_task_status", Arguments: boundaryObject(t, `{"baseId":"b","tableId":"t","tasks":{"$ref":"rows.taskTargets"}}`), Expected: boundaryObject(t, `{"data":{"terminalVerified":true,"resultReadableVerified":true}}`)}
			case "chart preflight":
				step.Tool = "create_chart"
				step.FieldBindings = nil
				step.Arguments = boundaryObject(t, `{"baseId":"b","dashboardId":"d","config":{"chartType":"AI_ANALYZE"},"layout":{"x":0,"y":0,"w":12,"h":4}}`)
				step.Verify = buildPlanReadback{Tool: "get_chart", Arguments: boundaryObject(t, `{"baseId":"b","dashboardId":"d","chartId":{"$ref":"rows.chartId"}}`), Expected: boundaryObject(t, `{"data":{"baseId":"b"}}`)}
			}
			p.Steps = append(p.Steps, step)
			saveBoundaryPlan(t, planPath, path, p, []buildPlanStepState{{ID: "board", Status: "verified", Arguments: p.Steps[0].Arguments, Resources: resources}, {ID: "rows", Status: status, Arguments: args, Resources: map[string]any{"tableId": "t"}}})
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"baseId":"b","dashboardId":"d","dashboardName":"看板"}}`}, {text: `{}`}}}
			if _, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", planPath, "--checkpoint", path, "--resume", "--yes"); err == nil {
				t.Fatal("invalid bound plan succeeded")
			}
			for _, call := range caller.calls {
				if call.tool == "update_records" || call.tool == "run_ai_field" || call.tool == "create_chart" {
					t.Fatal("invalid bound values reached writer")
				}
			}
		})
	}
}
func TestCrossPlatformCoveragePlanResumeEvidenceFailures(t *testing.T) {
	for _, name := range []string{"missing resources", "read binding", "expected binding", "AI targets", "AI query binding", "AI wrong target", "AI wait failure"} {
		t.Run(name, func(t *testing.T) {
			planPath, path := setupBuildPlan(t)
			p, _, _ := readBuildPlan(planPath)
			current := buildPlanStepState{ID: "board", Status: "accepted", Arguments: p.Steps[0].Arguments, Resources: map[string]any{"dashboardId": "d"}}
			switch name {
			case "missing resources":
				current.Resources = nil
			case "read binding":
				current.Resources = map[string]any{}
			case "expected binding":
				p.Steps[0].Verify.Arguments["dashboardId"] = "d"
				p.Steps[0].Verify.Expected["data"].(map[string]any)["x"] = map[string]any{"$ref": "board.missing"}
			default:
				planPath, path = aiPlanFixture(t)
				p, _, _ = readBuildPlan(planPath)
				current.ID = "run"
				current.Arguments = p.Steps[0].Arguments
				current.Resources = boundaryObject(t, `{"tasks":[{"fieldId":"F","taskId":"TASK","status":"submitted","recordIds":["R"]}]}`)
				if name == "AI targets" {
					current.Resources = map[string]any{"unexpected": true}
				}
				if name == "AI query binding" {
					p.Steps[0].Verify.Arguments["tasks"] = map[string]any{"$ref": "run.missing"}
				}
				if name == "AI wrong target" {
					p.Steps[0].Verify.Arguments["tableId"] = "other"
				}
			}
			saveBoundaryPlan(t, planPath, path, p, []buildPlanStepState{current})
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{}`}}}
			if _, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", planPath, "--checkpoint", path, "--resume", "--yes"); err == nil {
				t.Fatal("invalid persisted evidence accepted")
			}
			if len(caller.calls) > 1 {
				t.Fatal("resumed mutation replayed")
			}
		})
	}
}
func TestCrossPlatformCoveragePlanReferencePreflightAndIdentity(t *testing.T) {
	planPath, _ := setupBuildPlan(t)
	p, _, _ := readBuildPlan(planPath)
	p.Steps[0].Tool = "create_records"
	p.Steps[0].Arguments = boundaryObject(t, `{"baseId":"b","tableId":"t","records":[{"cells":{"F":"v"}}]}`)
	p.Steps[0].Verify = buildPlanReadback{Tool: "query_records", Arguments: map[string]any{"baseId": "b"}, Expected: boundaryObject(t, `{"data":{"records":[]}}`)}
	p.Steps[0].FieldBindings = map[string]any{"alias": map[string]any{"$ref": "missing.field"}}
	writeBoundaryJSON(t, planPath, p)
	if _, _, err := readBuildPlan(planPath); err == nil {
		t.Fatal("unbound field alias accepted")
	}
	p.Steps[0].FieldBindings = nil
	p.Steps[0].Arguments = map[string]any{"baseId": "b"}
	writeBoundaryJSON(t, planPath, p)
	if _, _, err := readBuildPlan(planPath); err == nil {
		t.Fatal("invalid preflight accepted")
	}
	// Invalid metadata is an identity read failure, distinct from no logged-in user.
	if err := os.WriteFile(os.Getenv("DWS_CONFIG_DIR")+"/profiles.json", []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	rt := boundaryRuntime(t, ChartsCreate, nil, nil, &upsertByKeyCaller{})
	if _, err := buildPlanIdentity(rt); err == nil {
		t.Fatal("corrupt profile accepted")
	}
	// All plan tests persist only public receipt data; JSON encoding stays explicit.
	if _, err := json.Marshal(p); err != nil {
		t.Fatal(err)
	}
}
