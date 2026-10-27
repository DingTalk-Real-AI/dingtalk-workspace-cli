// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBoundaryJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestCrossPlatformCoverageTaskReportBoundaries(t *testing.T) {
	dir := t.TempDir()
	path, base, model, baseModel := filepath.Join(dir, "current"), filepath.Join(dir, "base"), filepath.Join(dir, "model"), filepath.Join(dir, "base-model")
	state := buildPlanState{Version: 1, RunID: "run", Identity: "identity", Digest: "digest", BaseID: "B", Status: "verified", InvocationComplete: true, Steps: []buildPlanStepState{{ID: "step", Status: "verified"}}}
	writeBoundaryJSON(t, path, state)
	writeBoundaryJSON(t, base, state)
	n := int64(5)
	metrics := aitableModelMetrics{Model: "model", Environment: "env", InputDigest: "digest", ElapsedMS: &n, OutputBytes: &n, Interactions: &n, ReworkCount: &n}
	writeBoundaryJSON(t, model, metrics)
	writeBoundaryJSON(t, baseModel, metrics)
	caller := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, caller, "+task-report", "--checkpoint", path, "--baseline", base, "--model-metrics", model, "--baseline-model-metrics", baseModel)
	if err != nil || !strings.Contains(out, `"comparable": true`) || !strings.Contains(out, `"interactionsDelta": 0`) || len(caller.calls) != 0 {
		t.Fatalf("offline report: %s %v", out, err)
	}
	for _, args := range [][]string{
		{"--checkpoint", path, "--baseline-model-metrics", model},
		{"--checkpoint", filepath.Join(dir, "missing")},
		{"--checkpoint", path, "--baseline", filepath.Join(dir, "missing")},
	} {
		if _, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+task-report", args...); err == nil {
			t.Fatal("invalid report succeeded")
		}
	}
	for _, value := range []any{
		map[string]any{"kind": "ai_wait", "version": "bad"},
		map[string]any{"version": "bad"},
		buildPlanState{Version: 1, RunID: "r", Identity: "i", Digest: "d", BaseID: "B"},
	} {
		writeBoundaryJSON(t, path, value)
		if _, err := loadAitableTaskReport(path, ""); err == nil {
			t.Fatal("invalid evidence accepted")
		}
	}
	state.Steps[0].Status = "started"
	writeBoundaryJSON(t, path, state)
	report, err := loadAitableTaskReport(path, "")
	if err != nil || report.MetricsComplete {
		t.Fatalf("unsettled write statistics: %#v %v", report, err)
	}
	legacy := map[string]any{"version": 1, "runId": "r", "identity": "i", "planDigest": "d", "baseId": "B", "status": "verified", "steps": []any{map[string]any{"id": "step", "status": "verified"}}, "calls": []any{map[string]any{"phase": "write", "elapsedMs": 4, "responseBytes": 8}, map[string]any{"phase": "verify", "elapsedMs": 6, "responseBytes": 10}}}
	writeBoundaryJSON(t, path, legacy)
	report, err = loadAitableTaskReport(path, "")
	if err != nil || report.MetricsComplete || report.Metrics.RemoteCalls != 2 || report.Metrics.WriteCalls != 1 || report.Metrics.ReadCalls != 1 {
		t.Fatalf("legacy metrics: %#v %v", report, err)
	}
	writeBoundaryJSON(t, path, state)
	if _, err := loadAitableTaskReport(path, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing model accepted")
	}
	for _, m := range []aitableModelMetrics{{Model: "model", Environment: "env", InputDigest: "wrong"}, {Model: "model", Environment: "env", InputDigest: "digest", ElapsedMS: func() *int64 { x := int64(-1); return &x }()}} {
		writeBoundaryJSON(t, model, m)
		if _, err := loadAitableTaskReport(path, model); err == nil {
			t.Fatal("invalid model metrics accepted")
		}
	}
	for _, tc := range []struct {
		path string
		raw  []byte
	}{
		{path: dir}, {path: path, raw: []byte(strings.Repeat(" ", (16<<20)+1))},
	} {
		if tc.raw != nil {
			if err := os.WriteFile(tc.path, tc.raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := readAitableTaskFile(tc.path, &map[string]any{}); err == nil {
			t.Fatal("invalid task file accepted")
		}
	}
	targets := boundaryTargets()
	progress := newAITaskPoll("B", "T", targets)
	checkpoint := aiWaitCheckpoint{Version: 1, Kind: "ai_wait", Identity: "i", BaseID: "B", TableID: "T", Targets: targets, Progress: progress.evidence()}
	checkpoint.Progress.Body = map[string]any{"baseId": "wrong"}
	writeBoundaryJSON(t, path, checkpoint)
	if _, err := loadAitableTaskReport(path, ""); err == nil {
		t.Fatal("corrupted AI progress accepted")
	}
	body := boundaryStatus(t, targets, true)
	ts, _ := findNamedObjectList(body, "tasks")
	rs, _ := findNamedObjectList(ts[0], "rows")
	rs[0]["state"] = "failed"
	delete(rs[0], "resultVersion")
	body["resultVersionAvailable"] = false
	checkpoint.Progress.Body = body
	writeBoundaryJSON(t, path, checkpoint)
	report, err = loadAitableTaskReport(path, "")
	if err != nil || report.Status != "failed" {
		t.Fatalf("terminal failure: %#v %v", report, err)
	}
}
