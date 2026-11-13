// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// Whole-object refs differ from nested refs: binding may succeed but produce a
// scalar/array/null. Test the actual CLI, durable receipt and repeated resume.
func TestCrossPlatformCoveragePlanWholeObjectReferencesRejectNonObjects(t *testing.T) {
	for _, shape := range []struct {
		name  string
		value any
	}{{"string", "F"}, {"number", 7}, {"bool", false}, {"null", nil}, {"array", []any{"F"}}} {
		for _, stage := range []string{"expected data after acceptance", "bindings before write", "bindings after acceptance"} {
			t.Run(shape.name+"/"+stage, func(t *testing.T) {
				planPath, checkpoint := setupBuildPlan(t)
				plan, _, err := readBuildPlan(planPath)
				if err != nil {
					t.Fatal(err)
				}
				args := boundaryObject(t, `{"baseId":"b","tableId":"t","records":[{"recordId":"R","cells":{"alias":"v"}}]}`)
				step := buildPlanStep{ID: "rows", DependsOn: []string{"board"}, Tool: "update_records",
					Arguments: args, FieldBindings: map[string]any{"alias": "F"},
					Verify: buildPlanReadback{Tool: "query_records", Arguments: map[string]any{"baseId": "b", "tableId": "t"},
						Expected: boundaryObject(t, `{"data":{"records":[{"cells":{"alias":"v"}}]}}`)}}
				status, wantMessage, wantWrites := "planned", "fieldBindings", 0
				if stage == "expected data after acceptance" {
					step.Verify.Expected["data"] = map[string]any{"$ref": "board.bound"}
					wantMessage, wantWrites = "verify.expected.data", 1
				} else {
					step.FieldBindings = map[string]any{"$ref": "board.bound"}
					if stage == "bindings after acceptance" {
						status = "accepted"
					}
				}
				plan.Steps = append(plan.Steps, step)
				board := map[string]any{"dashboardId": "d", "bound": shape.value}
				receipt := map[string]any{"data": map[string]any{"tableId": "t"}}
				saveBoundaryPlan(t, planPath, checkpoint, plan, []buildPlanStepState{
					{ID: "board", Status: "verified", Arguments: plan.Steps[0].Arguments, Resources: board},
					{ID: "rows", Status: status, Arguments: args, Receipt: receipt, Resources: map[string]any{"tableId": "t"}},
				})
				writes := 0
				caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, _ map[string]any) (string, error) {
					switch tool {
					case "get_dashboard":
						return `{"data":{"baseId":"b","dashboardId":"d","dashboardName":"看板"}}`, nil
					case "update_records":
						writes++
						return `{"data":{"tableId":"t"}}`, nil
					default:
						t.Fatalf("invalid reference reached read/write tool: %s", tool)
						return "", nil
					}
				}}
				for attempt := 0; attempt < 2; attempt++ {
					_, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", planPath, "--checkpoint", checkpoint, "--resume", "--yes")
					var classified *apperrors.Error
					wantCategory := apperrors.CategoryValidation
					// A null reference is rejected by the binder before type validation;
					// retain its existing API classification and recoverable evidence.
					if shape.value == nil {
						wantCategory = apperrors.CategoryAPI
					}
					if !errors.As(err, &classified) || classified.Category != wantCategory ||
						(shape.value != nil && !strings.Contains(err.Error(), wantMessage)) {
						t.Fatalf("want validation, got %T %v", err, err)
					}
					if classified.Details["checkpoint"] != checkpoint || classified.Details["result"] == nil {
						t.Fatalf("lost recoverable checkpoint details: %#v", classified)
					}
					data, err := os.ReadFile(checkpoint)
					if err != nil {
						t.Fatal(err)
					}
					var saved buildPlanState
					if err = json.Unmarshal(data, &saved); err != nil {
						t.Fatal(err)
					}
					wantStatus := "planned"
					if stage != "bindings before write" {
						wantStatus = "accepted"
					}
					if writes != wantWrites || saved.Steps[1].Status != wantStatus || saved.Steps[1].Receipt == nil {
						t.Fatalf("receipt/status changed or write replayed: writes=%d state=%+v", writes, saved.Steps[1])
					}
				}
			})
		}
	}
}
