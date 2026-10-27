// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoveragePlanPersistenceStopsDispatch(t *testing.T) {
	for failureAt := 1; failureAt <= 5; failureAt++ {
		t.Run(string(rune('0'+failureAt)), func(t *testing.T) {
			plan, path := setupBuildPlan(t)
			calls := 0
			original := aitableTaskWriteJSON
			testseam.Swap(t, &aitableTaskWriteJSON, func(p string, b []byte) error {
				calls++
				if calls == failureAt {
					return errors.New("durable storage unavailable")
				}
				return original(p, b)
			})
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"baseId":"b","dashboardId":"d"}}`}, {text: `{"data":{"baseId":"b","dashboardId":"d","dashboardName":"看板"}}`}}}
			if _, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", path, "--yes"); err == nil {
				t.Fatal("checkpoint failure reported success")
			}
			wantCalls := 0
			if failureAt >= 3 {
				wantCalls = 1
			}
			if failureAt >= 4 {
				wantCalls = 2
			}
			if len(caller.calls) != wantCalls {
				t.Fatalf("failure at %d dispatched %d calls, want %d", failureAt, len(caller.calls), wantCalls)
			}
		})
	}
	for failureAt := 1; failureAt <= 2; failureAt++ {
		t.Run("path"+string(rune('0'+failureAt)), func(t *testing.T) {
			plan, path := setupBuildPlan(t)
			calls := 0
			original := aitableTaskAbsPath
			testseam.Swap(t, &aitableTaskAbsPath, func(p string) (string, error) {
				calls++
				if calls == failureAt {
					return "", errors.New("path unavailable")
				}
				return original(p)
			})
			caller := &upsertByKeyCaller{}
			if _, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", path, "--yes"); err == nil || len(caller.calls) != 0 {
				t.Fatal("path error dispatched mutation")
			}
		})
	}
}
func TestCrossPlatformCoveragePlanLocalPreconditions(t *testing.T) {
	for _, name := range []string{"bad plan", "bad duration", "identity unavailable", "overwrite plan", "parent file", "lock unavailable", "cancelled", "verify planned"} {
		t.Run(name, func(t *testing.T) {
			plan, path := setupBuildPlan(t)
			args := []string{"--plan", plan, "--checkpoint", path, "--yes"}
			command := "+build-plan"
			ctx := context.Background()
			switch name {
			case "bad plan":
				args[1] = filepath.Join(t.TempDir(), "missing")
			case "bad duration":
				args = append(args, "--timeout", "0s")
			case "identity unavailable":
				t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			case "overwrite plan":
				args[3] = plan
			case "parent file":
				args[3] = filepath.Join(plan, "task")
			case "lock unavailable":
				if err := os.Mkdir(path+".lock", 0700); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "verify planned":
				rt := boundaryRuntime(t, ChartsCreate, nil, nil, &upsertByKeyCaller{})
				identity, err := buildPlanIdentity(rt)
				if err != nil {
					t.Fatal(err)
				}
				p, d, err := readBuildPlan(plan)
				if err != nil {
					t.Fatal(err)
				}
				state := buildPlanState{Version: 1, RunID: "r", Identity: identity, Digest: d, BaseID: p.BaseID, Steps: []buildPlanStepState{{ID: "board", Status: "planned"}}}
				if err = saveBuildPlanState(path, &state); err != nil {
					t.Fatal(err)
				}
				command = "+build-plan-verify"
			}
			caller := &upsertByKeyCaller{}
			out, err := runAITableCompositeCLIContext(t, ctx, caller, command, args...)
			if name == "verify planned" {
				if err != nil {
					t.Fatalf("%s %v", out, err)
				}
			} else if err == nil {
				t.Fatal("invalid precondition succeeded")
			}
			if len(caller.calls) != 0 {
				t.Fatal("local failure sent remote call")
			}
		})
	}
}
func TestCrossPlatformCoverageAIWaitPersistenceFailures(t *testing.T) {
	for _, name := range []string{"path", "initial save", "final save", "restore"} {
		t.Run(name, func(t *testing.T) {
			_, path := setupBuildPlan(t)
			target := `[{"taskId":"TASK","fieldId":"F","recordIds":["R"]}]`
			args := []string{"--base-id", "B", "--table-id", "T", "--tasks", target, "--checkpoint", path, "--timeout", "0s"}
			caller := &upsertByKeyCaller{callFn: func(_ int, _, _ string, args map[string]any) (string, error) {
				return aiChunkReceipt(args["tasks"].([]map[string]any)[0], true), nil
			}}
			if name == "path" {
				testseam.Swap(t, &aitableTaskAbsPath, func(string) (string, error) { return "", errors.New("path unavailable") })
			}
			if name == "initial save" || name == "final save" {
				original := aitableTaskWriteJSON
				n := 0
				testseam.Swap(t, &aitableTaskWriteJSON, func(p string, b []byte) error {
					n++
					if name == "initial save" || n == 3 {
						return errors.New("disk unavailable")
					}
					return original(p, b)
				})
			}
			if name == "restore" {
				rt := boundaryRuntime(t, FieldWaitAI, nil, nil, &upsertByKeyCaller{})
				identity, err := buildPlanIdentity(rt)
				if err != nil {
					t.Fatal(err)
				}
				writeBoundaryJSON(t, path, aiWaitCheckpoint{Version: 1, Kind: "ai_wait", Identity: identity, BaseID: "B", TableID: "T", Targets: boundaryTargets(), Progress: aiTaskEvidence{Body: map[string]any{"baseId": "wrong"}}})
				args = append(args, "--resume")
			}
			if _, err := runAITableCompositeCLI(t, caller, "+field-wait-ai", args...); err == nil {
				t.Fatal("persistence failure succeeded")
			}
			if name != "final save" && len(caller.calls) != 0 {
				t.Fatal("failed precondition queried")
			}
		})
	}
}

func TestCrossPlatformCoveragePlanIdentityFailureBeforeCheckpoint(t *testing.T) {
	plan, path := setupBuildPlan(t)
	if err := os.WriteFile(filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "profiles.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	caller := &upsertByKeyCaller{}
	if _, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", path, "--yes"); err == nil || len(caller.calls) != 0 {
		t.Fatal("invalid identity reached writer")
	}
}
func TestCrossPlatformCoveragePlanUnknownUpdatesReadOriginalTargets(t *testing.T) {
	for _, tool := range []string{"update_view", "update_records"} {
		t.Run(tool, func(t *testing.T) {
			plan, path := setupBuildPlan(t)
			step := buildPlanStep{ID: "update", Tool: tool}
			read := `{"data":{"baseId":"b","tableId":"t","views":[{"viewId":"v","name":"new"}]}}`
			if tool == "update_view" {
				step.Arguments = boundaryObject(t, `{"baseId":"b","tableId":"t","viewId":"v","newViewName":"new"}`)
				step.Verify = buildPlanReadback{Tool: "get_views", Arguments: boundaryObject(t, `{"baseId":"b","tableId":"t"}`), Expected: boundaryObject(t, `{"data":{"views":[{"viewId":"v","name":"new"}]}}`)}
			} else {
				step.Arguments = boundaryObject(t, `{"baseId":"b","tableId":"t","records":[{"recordId":"R","cells":{"F":"v"}}]}`)
				step.Verify = buildPlanReadback{Tool: "query_records", Arguments: boundaryObject(t, `{"baseId":"b","tableId":"t","recordIds":["R"]}`), Expected: boundaryObject(t, `{"data":{"records":[{"recordId":"R","cells":{"F":"v"}}]}}`)}
				read = `{"data":{"baseId":"b","tableId":"t","records":[{"recordId":"R","cells":{"F":"v"}}]}}`
			}
			writeBoundaryJSON(t, plan, buildPlan{Version: 1, BaseID: "b", Steps: []buildPlanStep{step}})
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: errors.New("response lost")}, {text: read}}}
			if _, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", path, "--yes"); err != nil || len(caller.calls) != 2 {
				t.Fatalf("original target not independently read: %v %#v", err, caller.calls)
			}
		})
	}
	plan, path := setupBuildPlan(t)
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"dashboardId":"d"}}`}, {text: `{"data":{"baseId":"b","dashboardId":"d","dashboardName":"wrong"}}`}}}
	if _, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", path, "--yes"); err == nil {
		t.Fatal("mismatched assertion verified")
	}
}

func TestCrossPlatformCoveragePlanDryRunHasNoSideEffects(t *testing.T) {
	plan, path := setupBuildPlan(t)
	caller := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", path, "--dry-run")
	if err != nil || len(caller.calls) != 0 {
		t.Fatalf("dry-run dispatched: %s %v", out, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("dry-run persisted a checkpoint")
	}
}

func TestCrossPlatformCoveragePlanPrewriteRefusalCannotBecomeVerified(t *testing.T) {
	for _, tool := range []string{"update_view", "update_records"} {
		t.Run(tool, func(t *testing.T) {
			plan, path := setupBuildPlan(t)
			step := buildPlanStep{ID: "update", Tool: tool}
			if tool == "update_view" {
				step.Arguments = boundaryObject(t, `{"baseId":"b","tableId":"t","viewId":"v","newViewName":"already matches"}`)
				step.Verify = buildPlanReadback{Tool: "get_views", Arguments: boundaryObject(t, `{"baseId":"b","tableId":"t"}`), Expected: boundaryObject(t, `{"data":{"views":[{"viewId":"v","name":"already matches"}]}}`)}
			} else {
				step.Arguments = boundaryObject(t, `{"baseId":"b","tableId":"t","records":[{"recordId":"R","cells":{"F":"already matches"}}]}`)
				step.Verify = buildPlanReadback{Tool: "query_records", Arguments: boundaryObject(t, `{"baseId":"b","tableId":"t"}`), Expected: boundaryObject(t, `{"data":{"records":[{"recordId":"R","cells":{"F":"already matches"}}]}}`)}
			}
			writeBoundaryJSON(t, plan, buildPlan{Version: 1, BaseID: "b", Steps: []buildPlanStep{step}})
			refusal := apperrors.NewAPI("edit denied", apperrors.WithReason("DASHBOARD_WRITE_EDIT_REQUIRED"), apperrors.WithExecutionStarted(false), apperrors.WithRetryable(false))
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: refusal}}}
			_, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", path, "--yes")
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "DASHBOARD_WRITE_EDIT_REQUIRED" || typed.ExecutionStarted == nil || *typed.ExecutionStarted || len(caller.calls) != 1 {
				t.Fatalf("refusal lost: %#v %v calls=%#v", typed, err, caller.calls)
			}
			resume := &upsertByKeyCaller{}
			if _, err := runAITableCompositeCLI(t, resume, "+build-plan-verify", "--plan", plan, "--checkpoint", path); err == nil || len(resume.calls) != 0 {
				t.Fatal("refused write resumed as successful update")
			}
		})
	}
}
