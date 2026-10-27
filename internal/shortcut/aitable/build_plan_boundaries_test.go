// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func planBoundaryFixture(t *testing.T) buildPlan {
	t.Helper()
	path, _ := setupBuildPlan(t)
	plan, _, err := readBuildPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestCrossPlatformCoveragePlanValidationRejectsUnsafeBindingsBeforeWrites(t *testing.T) {
	cases := []struct {
		name   string
		change func(*buildPlan)
	}{
		{"duplicate step", func(p *buildPlan) { p.Steps = append(p.Steps, p.Steps[0]) }},
		{"invalid id", func(p *buildPlan) { p.Steps[0].ID = "../bad" }},
		{"unknown dependency", func(p *buildPlan) { p.Steps[0].DependsOn = []string{"missing"} }},
		{"cross base", func(p *buildPlan) { p.Steps[0].Verify.Arguments["baseId"] = "other" }},
		{"no data assertion", func(p *buildPlan) { p.Steps[0].Verify.Expected = map[string]any{"status": "success"} }},
		{"unbound write ref", func(p *buildPlan) { p.Steps[0].Arguments["config"] = map[string]any{"$ref": "missing.config"} }},
		{"unbound read ref", func(p *buildPlan) { p.Steps[0].Verify.Arguments["dashboardId"] = map[string]any{"$ref": "missing.id"} }},
		{"unbound assertion ref", func(p *buildPlan) {
			p.Steps[0].Verify.Expected["data"] = map[string]any{"id": map[string]any{"$ref": "missing.id"}}
		}},
		{"field bindings on nonrecord", func(p *buildPlan) { p.Steps[0].FieldBindings = map[string]any{"title": "F"} }},
		{"template on nondrawing", func(p *buildPlan) { p.Steps[0].Template = "base" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := planBoundaryFixture(t)
			tc.change(&p)
			raw, _ := json.Marshal(p)
			path := filepath.Join(t.TempDir(), "plan.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := readBuildPlan(path); err == nil {
				t.Fatal("unsafe plan accepted")
			}
		})
	}
	for _, raw := range []string{`{"version":1,"unknown":true}`, strings.Repeat(" ", (8<<20)+1), `{}` + `{}`} {
		path := filepath.Join(t.TempDir(), "invalid.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readBuildPlan(path); err == nil {
			t.Fatal("malformed or oversized plan accepted")
		}
	}
	for _, path := range []string{filepath.Join(t.TempDir(), "missing"), t.TempDir()} {
		if _, _, err := readBuildPlan(path); err == nil {
			t.Fatal("unreadable plan accepted")
		}
	}
}

func TestCrossPlatformCoveragePlanTemplatesExpandWithoutChangingSource(t *testing.T) {
	template := map[string]any{"chartType": "AI_ANALYZE", "style": map[string]any{"color": "red", "font": 1}}
	cases := []struct {
		params  any
		missing bool
		wantErr bool
	}{
		{params: "wrong", wantErr: true}, {params: map[string]any{}, missing: true, wantErr: true},
		{params: map[string]any{"chartType": "LINE"}, wantErr: true},
		{params: map[string]any{"style": map[string]any{"font": 2}}},
	}
	for _, tc := range cases {
		p := planBoundaryFixture(t)
		p.Steps[0].Tool = "create_chart"
		p.Steps[0].Template = "chart"
		p.Steps[0].Arguments = map[string]any{"baseId": "b", "dashboardId": "D", "config": tc.params, "layout": map[string]any{"x": 0, "y": 0, "w": 6, "h": 4}}
		p.Steps[0].Verify.Tool = "get_chart"
		p.Templates = map[string]map[string]any{"chart": template}
		if tc.missing {
			p.Templates = nil
		}
		raw, _ := json.Marshal(p)
		path := filepath.Join(t.TempDir(), "plan.json")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		got, _, err := readBuildPlan(path)
		if (err != nil) != tc.wantErr {
			t.Fatalf("expansion error=%v expected=%v", err, tc.wantErr)
		}
		if err == nil && got.Steps[0].Arguments["config"].(map[string]any)["style"].(map[string]any)["color"] != "red" {
			t.Fatal("template lost defaults")
		}
	}
}

func TestCrossPlatformCoveragePlanPreflightValidatesRecordAndFieldShapes(t *testing.T) {
	cases := []struct {
		tool string
		args map[string]any
	}{
		{"create_table", map[string]any{"fields": []any{}}},
		{"create_fields", map[string]any{"tableId": "T", "fields": "bad"}},
		{"create_fields", map[string]any{"tableId": "T", "fields": []any{}}},
		{"create_fields", map[string]any{"tableId": "T", "fields": []any{map[string]any{}}}},
		{"create_chart", map[string]any{"dashboardId": "D", "config": map[string]any{}, "layout": map[string]any{}}},
		{"create_dashboard", map[string]any{"config": map[string]any{}}},
		{"create_records", map[string]any{"tableId": "T", "records": "bad"}},
		{"create_records", map[string]any{"tableId": "T", "records": []any{1}}},
		{"create_records", map[string]any{"tableId": "T", "records": []any{map[string]any{"cells": 1}}}},
		{"update_records", map[string]any{"tableId": "T", "records": []any{map[string]any{"cells": map[string]any{}}}}},
		{"run_ai_field", map[string]any{"tableId": "T", "fieldIds": []any{}, "recordIds": []any{}}},
		{"run_ai_field", map[string]any{"baseId": "b", "tableId": "T", "fieldIds": []any{"F"}, "recordIds": []any{"R"}}},
	}
	for _, tc := range cases {
		step := buildPlanStep{ID: "s", Tool: tc.tool, Arguments: tc.args, Verify: buildPlanReadback{Expected: map[string]any{"data": map[string]any{}}}}
		if err := preflightPlanStep(step); err == nil {
			t.Fatalf("unsafe %s accepted: %#v", tc.tool, tc.args)
		}
	}
	if err := validatePlanReferences([]any{map[string]any{"$ref": "missing.id"}}, nil); err == nil {
		t.Fatal("unbound array reference accepted")
	}
	for _, binding := range []map[string]any{{"a": 1}, {"a": ""}, {"a": " F"}, {"a": "F", "b": "F"}, {" ": "F"}, {"a": map[string]any{"$ref": "missing.id"}}} {
		if _, err := resolvePlanFieldBindings(binding, nil); err == nil {
			t.Fatalf("ambiguous field binding accepted: %#v", binding)
		}
	}
	bindings, err := resolvePlanFieldBindings(map[string]any{"title": "F"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, records := range []any{1, []any{1}, []any{map[string]any{"cells": 1}}, []any{map[string]any{"cells": map[string]any{"other": 1}}}} {
		if _, err := bindPlanRecordCells(records, bindings); err == nil {
			t.Fatalf("invalid record binding accepted: %#v", records)
		}
	}
	got, err := bindPlanRecordCells([]any{map[string]any{"recordId": "R", "cells": map[string]any{"title": "hello"}}}, bindings)
	if err != nil || got[0].(map[string]any)["cells"].(map[string]any)["F"] != "hello" {
		t.Fatalf("field binding failed: %#v %v", got, err)
	}
}

func TestCrossPlatformCoveragePlanCheckpointRejectsCorruptionAndKeepsOriginalOnMarshalFailure(t *testing.T) {
	p := planBoundaryFixture(t)
	state := buildPlanState{Version: 1, Digest: "digest", Identity: "identity", BaseID: "b", RunID: "run", Steps: []buildPlanStepState{{ID: p.Steps[0].ID, Status: "verified"}}}
	path := filepath.Join(t.TempDir(), "state.json")
	if _, err := loadBuildPlanState(path, p, "digest", "identity"); err == nil {
		t.Fatal("missing state accepted")
	}
	for _, raw := range []string{`{`, `{}`, `{"version":1,"planDigest":"digest","identity":"identity","baseId":"b","runId":"run","steps":[{"id":"wrong","status":"verified"}]}`, `{"version":1,"planDigest":"digest","identity":"identity","baseId":"b","runId":"run","steps":[{"id":"board","status":"future"}]}`} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadBuildPlanState(path, p, "digest", "identity"); err == nil {
			t.Fatalf("corrupt state accepted: %s", raw)
		}
	}
	if err := saveBuildPlanState(path, &state); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	state.Steps[0].Receipt = map[string]any{"number": math.NaN()}
	if err := saveBuildPlanState(path, &state); err == nil {
		t.Fatal("invalid state saved")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed save corrupted prior evidence")
	}
	legacy := `{"version":1,"planDigest":"digest","identity":"identity","baseId":"b","runId":"run","steps":[{"id":"board","status":"verified"}],"calls":[{"phase":"write","elapsedMs":2,"responseBytes":3},{"phase":"verify","elapsedMs":4,"responseBytes":5}]}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := loadBuildPlanState(path, p, "digest", "identity")
	if err != nil || !restored.MetricsPartial || restored.Metrics.RemoteCalls != 2 || restored.Metrics.WriteCalls != 1 || restored.Metrics.ReadCalls != 1 || restored.Metrics.ResponseBytes != 8 {
		t.Fatalf("legacy evidence changed: %#v %v", restored, err)
	}
}
