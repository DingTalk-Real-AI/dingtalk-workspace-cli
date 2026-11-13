// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func boundaryRuntime(t *testing.T, s shortcut.Shortcut, values map[string]string, bools map[string]bool, caller *upsertByKeyCaller) *shortcut.RuntimeContext {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	cmd := &cobra.Command{Use: "test"}
	for k, v := range values {
		cmd.Flags().String(k, v, "")
	}
	for k, v := range bools {
		cmd.Flags().Bool(k, v, "")
	}
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	return shortcut.RuntimeContextForTest(cmd, s)
}
func boundaryTargets() []map[string]any {
	return []map[string]any{{"taskId": "TASK", "fieldId": "F", "recordIds": []any{"R"}}}
}
func boundaryStatus(t *testing.T, targets []map[string]any, terminal bool) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(aiChunkReceipt(targets[0], terminal)), &raw); err != nil {
		t.Fatal(err)
	}
	return parityResponseObject(raw)
}
func TestCrossPlatformCoverageAITaskReceiptBoundaries(t *testing.T) {
	targets := boundaryTargets()
	for _, mutate := range []func(map[string]any){
		func(b map[string]any) { b["queryComplete"] = true; b["queryRetryable"] = true },
		func(b map[string]any) { b["resultVersionAvailable"] = "true" },
		func(b map[string]any) { tasks, _ := findNamedObjectList(b, "tasks"); tasks[0]["taskId"] = "other" },
		func(b map[string]any) { tasks, _ := findNamedObjectList(b, "tasks"); tasks[0]["rows"] = []any{} },
		func(b map[string]any) {
			tasks, _ := findNamedObjectList(b, "tasks")
			rows, _ := findNamedObjectList(tasks[0], "rows")
			rows[0]["state"] = "new_enum"
		},
		func(b map[string]any) {
			tasks, _ := findNamedObjectList(b, "tasks")
			rows, _ := findNamedObjectList(tasks[0], "rows")
			rows[0]["terminal"] = false
		},
		func(b map[string]any) {
			tasks, _ := findNamedObjectList(b, "tasks")
			tasks[0]["terminalVerified"] = false
		},
		func(b map[string]any) { b["terminalVerified"] = false },
	} {
		b := boundaryStatus(t, targets, true)
		mutate(b)
		if err := validateAITaskStatusReceipt(b, targets); err == nil {
			t.Fatalf("invalid receipt accepted: %#v", b)
		}
	}
	for _, original := range []error{errors.New("network"), apperrors.NewAPI("typed", apperrors.WithDetails(map[string]any{"safe": true})), &helpers.CLIError{Code: helpers.CodeAuthPermission, Details: map[string]any{"safe": true}}} {
		err := aiTaskReadFailure(original, map[string]any{"prior": true})
		if err == nil {
			t.Fatal("lost failure")
		}
	}
}
func TestCrossPlatformCoverageAIWaitValidation(t *testing.T) {
	for _, tc := range []struct {
		tasks, budget string
		dry           bool
	}{
		{tasks: `[]`, budget: "0s"}, {tasks: `[{"taskId":"TASK","fieldId":"F","recordIds":["R"]}]`, budget: "bad"}, {tasks: `[]`, budget: "0s", dry: true},
	} {
		caller := &upsertByKeyCaller{}
		rt := boundaryRuntime(t, FieldWaitAI, map[string]string{"tasks": tc.tasks, "timeout": tc.budget}, map[string]bool{"dry-run": tc.dry}, caller)
		if err := executeFieldWaitAI(rt); err == nil || len(caller.calls) != 0 {
			t.Fatal("invalid wait queried")
		}
	}
}
func TestCrossPlatformCoverageAIPollerStopsAndPersists(t *testing.T) {
	targets := boundaryTargets()
	boom := errors.New("disk unavailable")
	for _, name := range []string{"already failed", "already terminal", "cancelled", "wrong target", "invalid rows", "save fails", "terminal failure", "backoff save fails", "backoff cancelled"} {
		t.Run(name, func(t *testing.T) {
			caller := &upsertByKeyCaller{callFn: func(_ int, _, _ string, args map[string]any) (string, error) {
				text := aiChunkReceipt(args["tasks"].([]map[string]any)[0], name == "terminal failure")
				if name == "wrong target" {
					return `{"baseId":"other"}`, nil
				}
				if name == "invalid rows" {
					return `{"baseId":"B","tableId":"T"}`, nil
				}
				if name == "terminal failure" {
					var b map[string]any
					_ = json.Unmarshal([]byte(text), &b)
					body := parityResponseObject(b)
					ts, _ := findNamedObjectList(body, "tasks")
					rs, _ := findNamedObjectList(ts[0], "rows")
					rs[0]["state"] = "failed"
					delete(rs[0], "resultVersion")
					body["resultVersionAvailable"] = false
					raw, _ := json.Marshal(b)
					return string(raw), nil
				}
				if name == "backoff save fails" || name == "backoff cancelled" {
					var raw map[string]any
					_ = json.Unmarshal([]byte(text), &raw)
					body := parityResponseObject(raw)
					tasks, _ := findNamedObjectList(body, "tasks")
					rows, _ := findNamedObjectList(tasks[0], "rows")
					rows[0]["state"], rows[0]["terminal"] = "running", false
					delete(rows[0], "resultVersion")
					tasks[0]["terminalVerified"], body["terminalVerified"], body["resultVersionAvailable"] = false, false, false
					encoded, _ := json.Marshal(raw)
					return string(encoded), nil
				}
				return text, nil
			}}
			rt := boundaryRuntime(t, FieldWaitAI, nil, nil, caller)
			p := newAITaskPoll("B", "T", targets)
			if name == "already failed" {
				p.body = boundaryStatus(t, targets, true)
				ts, _ := findNamedObjectList(p.body, "tasks")
				rs, _ := findNamedObjectList(ts[0], "rows")
				rs[0]["state"] = "failed"
			}
			if name == "already terminal" {
				p.body["terminalVerified"] = true
			}
			if name == "cancelled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				rt.Command().SetContext(ctx)
			}
			saves := 0
			save := func() error {
				saves++
				if name == "save fails" || (name == "backoff save fails" && saves == 2) {
					return boom
				}
				return nil
			}
			testseam.Swap(t, &bootstrapReadbackWait, func(context.Context, time.Duration) error {
				if name == "backoff cancelled" {
					return context.Canceled
				}
				return nil
			})
			_, _, err := pollAITasks(rt, "B", "T", targets, p, false, &aitableTaskMetrics{}, save)
			if name == "already terminal" {
				if err != nil || len(caller.calls) != 0 {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("lost poll failure")
			}
			if len(caller.calls) > 1 {
				t.Fatal("failure retried instead of preserving evidence")
			}
		})
	}
}
func TestCrossPlatformCoverageAIPlanArgumentAndEvidenceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		args    map[string]any
		wantErr bool
	}{
		{args: map[string]any{"tableId": false}, wantErr: true},
		{args: map[string]any{"tableId": "T", "fieldIds": []any{}, "recordIds": []any{"R"}}, wantErr: true},
		{args: map[string]any{"tableId": map[string]any{"$ref": "table.tableId"}, "fieldIds": map[string]any{"$ref": "field.ids"}, "recordIds": []any{map[string]any{"$ref": "row.id"}}}},
	} {
		if err := validateAIPlanArguments(tc.args, true); (err != nil) != tc.wantErr {
			t.Fatalf("AI arguments %#v: %v", tc.args, err)
		}
	}
	valid := map[string]any{"tableId": "T", "fieldIds": []any{"F"}, "recordIds": []any{"R"}}
	for _, tasks := range []any{
		[]any{map[string]any{"fieldId": "", "taskId": "TASK"}},
		[]any{map[string]any{"fieldId": "F", "taskId": "TASK", "status": "unknown", "recordIds": []any{"R"}}},
	} {
		if _, err := aiPlanTargets(&buildPlanStepState{Arguments: valid, Resources: map[string]any{"tasks": tasks}}); err == nil {
			t.Fatal("invalid task accepted")
		}
	}
	rt := boundaryRuntime(t, FieldWaitAI, nil, nil, &upsertByKeyCaller{})
	if err := waitAIPlanStep(rt, &buildPlanState{}, &buildPlanStepState{}, func() error { return nil }); err == nil {
		t.Fatal("invalid arguments waited")
	}
	if err := waitAIPlanStep(rt, &buildPlanState{}, &buildPlanStepState{Arguments: valid}, func() error { return nil }); err == nil {
		t.Fatal("missing receipt waited")
	}
	current := &buildPlanStepState{Arguments: valid, Resources: map[string]any{"tasks": []any{map[string]any{"fieldId": "F", "taskId": "TASK", "status": "submitted", "recordIds": []any{"R"}}}}}
	if err := waitAIPlanStep(rt, &buildPlanState{BaseID: "B"}, current, func() error { return errors.New("disk") }); err == nil {
		t.Fatal("failed persistence waited")
	}
	current.AIProgress = &aiTaskEvidence{Body: map[string]any{"baseId": "wrong"}}
	if err := waitAIPlanStep(rt, &buildPlanState{BaseID: "B"}, current, func() error { return nil }); err == nil {
		t.Fatal("invalid checkpoint waited")
	}
}
func TestCrossPlatformCoverageAICheckpointFileBoundaries(t *testing.T) {
	targets := boundaryTargets()
	if _, err := restoreAITaskPoll("B", "T", []map[string]any{{"bad": make(chan int)}}, aiTaskEvidence{}); err == nil {
		t.Fatal("unserializable targets restored")
	}
	if _, err := restoreAITaskPoll("B", "T", targets, aiTaskEvidence{Body: map[string]any{"baseId": "B", "tableId": "T"}}); err == nil {
		t.Fatal("invalid rows restored")
	}
	_, checkpoint := setupBuildPlan(t)
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		resume bool
	}{
		{resume: true}, {path: filepath.Join(blocked, "task")}, {path: filepath.Join(t.TempDir(), "missing"), resume: true},
	} {
		rt := boundaryRuntime(t, FieldWaitAI, map[string]string{"base-id": "B", "table-id": "T", "checkpoint": tc.path}, map[string]bool{"resume": tc.resume}, &upsertByKeyCaller{})
		if _, _, _, err := openAIWaitCheckpoint(rt, targets); err == nil {
			t.Fatal("invalid checkpoint opened")
		}
	}
	rt := boundaryRuntime(t, FieldWaitAI, map[string]string{"base-id": "B", "table-id": "T", "checkpoint": checkpoint}, nil, &upsertByKeyCaller{})
	state, save, closeFile, err := openAIWaitCheckpoint(rt, targets)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFile()
	state.Progress.Body = map[string]any{"bad": make(chan int)}
	if err := save(); err == nil {
		t.Fatal("invalid evidence saved")
	}
	if _, _, _, err := openAIWaitCheckpoint(rt, targets); err == nil {
		t.Fatal("concurrent checkpoint lock acquired")
	}
}

func TestCrossPlatformCoverageAITaskOptionalFlagsAndDeadline(t *testing.T) {
	body := boundaryStatus(t, boundaryTargets(), true)
	body["queryRetryable"] = "true"
	if err := validateAITaskStatusReceipt(body, boundaryTargets()); err == nil {
		t.Fatal("nonboolean query flag accepted")
	}
	rt := boundaryRuntime(t, FieldWaitAI, nil, nil, &upsertByKeyCaller{})
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	rt.Command().SetContext(ctx)
	current := &buildPlanStepState{Arguments: boundaryObject(t, `{"tableId":"T","fieldIds":["F"],"recordIds":["R"]}`), Resources: boundaryObject(t, `{"tasks":[{"taskId":"TASK","fieldId":"F","recordIds":["R"],"status":"submitted"}]}`)}
	if err := waitAIPlanStep(rt, &buildPlanState{BaseID: "B"}, current, func() error { return nil }); err != nil || current.Status != "pending" {
		t.Fatalf("deadline lost checkpoint: %v", err)
	}
	_, path := setupBuildPlan(t)
	if err := os.WriteFile(filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "profiles.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	rt = boundaryRuntime(t, FieldWaitAI, map[string]string{"base-id": "B", "table-id": "T", "checkpoint": path}, nil, &upsertByKeyCaller{})
	if _, _, _, err := openAIWaitCheckpoint(rt, boundaryTargets()); err == nil {
		t.Fatal("unreadable identity accepted")
	}
}
