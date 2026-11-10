// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageCreationDefaultsKeepOldAndNewReadbackCompatible(t *testing.T) {
	layout := map[string]any{"x": float64(0), "y": float64(0), "w": float64(16), "h": float64(8)}
	for _, kind := range []string{"BAR", "LINE", "HISTOGRAM", "PIE"} {
		for _, explicit := range []bool{false, true} {
			cfg := map[string]any{"chartType": kind, "sheet": "actual-table"}
			if explicit {
				cfg["label"], cfg["xAxisShow"], cfg["colors"] = false, false, "CUSTOM_PALETTE"
			}
			for _, newMCP := range []bool{false, true} {
				actual := map[string]any{}
				for key, value := range cfg {
					actual[key] = value
				}
				if newMCP {
					actual["legend"] = "top"
					if !explicit {
						actual["label"], actual["xAxisShow"], actual["colors"] = true, true, "COLOR_PALETTE_1"
					}
				}
				if !verifyChartConfiguration(map[string]any{"config": actual, "layout": layout}, cfg, layout) {
					t.Fatalf("%s explicit=%t newMCP=%t: additive display keys broke readback", kind, explicit, newMCP)
				}
				if explicit {
					actual["label"] = true
					if verifyChartConfiguration(map[string]any{"config": actual, "layout": layout}, cfg, layout) {
						t.Fatal("overwritten explicit visibility verified")
					}
				}
			}
		}
	}
}

func TestCrossPlatformCoverageChartBatchInputAndReadback(t *testing.T) {
	for _, raw := range []string{"{", "[]", `[{"config":{"chartType":"LINE"}}]`, `[{"config":{"chartType":"line"},"layout":{}}]`, `[{"config":{"chartType":"LINE","schemaVersion":2},"layout":{}}]`, `[{"config":{"chartType":"LINE"},"layout":{"x":0,"y":0,"w":0,"h":4}}]`} {
		if _, err := parseChartBatch(raw); err == nil {
			t.Fatalf("invalid charts accepted: %s", raw)
		}
	}
	layout := map[string]any{"x": float64(0), "y": float64(0), "w": float64(1), "h": float64(1)}
	for _, body := range []map[string]any{
		{"widget": map[string]any{"name": "expected", "config": map[string]any{"chartType": "LINE"}}, "layout": layout},
		{"config": map[string]any{"name": "expected", "chartType": "LINE"}, "layout": layout},
	} {
		if !verifyChartConfiguration(body, map[string]any{"name": "expected", "chartType": "LINE"}, layout) {
			t.Fatal("valid full widget rejected")
		}
	}
	if verifyChartConfiguration(map[string]any{"config": map[string]any{"chartType": "PIE"}, "layout": layout}, map[string]any{"chartType": "LINE"}, layout) {
		t.Fatal("wrong config verified")
	}
	if verifyChartConfiguration(map[string]any{"name": "wrong", "layout": layout}, map[string]any{"name": "expected"}, layout) {
		t.Fatal("wrong name verified")
	}
	charts := `[{"config":{"chartType":"AI_ANALYZE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`
	dashboard := `{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`
	for _, tc := range []struct {
		name, read, write, readback string
		readErr                     error
		serial                      bool
	}{
		{name: "prepare failed", readErr: errors.New("timeout")},
		{name: "invalid dashboard", read: `{}`},
		{name: "target mismatch", read: dashboard, write: `{"baseId":"X","dashboardId":"D","results":[]}`},
		{name: "missing entries", read: dashboard, write: `{"baseId":"B","dashboardId":"D"}`},
		{name: "bad index", read: dashboard, write: `{"baseId":"B","dashboardId":"D","results":[{"index":3}]}`},
		{name: "failed row", read: dashboard, write: `{"baseId":"B","dashboardId":"D","results":[{"index":0,"status":"failed","success":false}]}`},
		{name: "missing serial ID", read: dashboard, write: `{"data":{"baseId":"B","dashboardId":"D"}}`, serial: true},
		{name: "readback mismatch", read: dashboard, write: `{"data":{"chartId":"C"}}`, readback: `{}`, serial: true},
		{name: "readback unavailable", read: dashboard, write: `{"data":{"chartId":"C"}}`, serial: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: tc.read, err: tc.readErr}, {text: tc.write}, {text: tc.readback, err: func() error {
				if tc.name == "readback unavailable" {
					return errors.New("read unavailable")
				}
				return nil
			}()}}}
			flag := "--server-batch=true"
			if tc.serial {
				flag = "--server-batch=false"
			}
			if _, err := runAITableCompositeCLI(t, caller, "+charts-create", "--base-id", "B", "--dashboard-id", "D", "--charts", charts, flag, "--yes"); err == nil {
				t.Fatal("incomplete chart evidence verified")
			}
			if len(caller.calls) > 3 {
				t.Fatal("write replayed")
			}
		})
	}
	// A valid generic bound must still be rejected for a narrower actual grid.
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":1,"schemaVersionTypeVerified":true}}}`}}}
	if _, err := runAITableCompositeCLI(t, caller, "+charts-create", "--base-id", "B", "--dashboard-id", "D", "--charts", `[{"config":{"chartType":"AI_ANALYZE"},"layout":{"x":40,"y":0,"w":8,"h":4}}]`, "--yes"); err == nil || len(caller.calls) != 1 {
		t.Fatal("actual grid ignored")
	}
}
func TestCrossPlatformCoverageBoundPlanChartGrid(t *testing.T) {
	dashboard := `{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`
	for _, tc := range []struct {
		name     string
		args     map[string]any
		response string
		callErr  error
		wantErr  bool
	}{
		{name: "no target", args: map[string]any{}, wantErr: true},
		{name: "invalid mode", args: map[string]any{"dashboardId": "D", "isAppMode": "true"}, wantErr: true},
		{name: "read error", args: map[string]any{"dashboardId": "D"}, callErr: errors.New("timeout"), wantErr: true},
		{name: "wrong dashboard", args: map[string]any{"dashboardId": "D"}, response: `{}`, wantErr: true},
		{name: "no layout", args: map[string]any{"dashboardId": "D"}, response: dashboard, wantErr: true},
		{name: "out of grid", args: map[string]any{"dashboardId": "D", "layout": map[string]any{"x": float64(40), "y": float64(0), "w": float64(8), "h": float64(4)}}, response: dashboard, wantErr: true},
		{name: "valid app", args: map[string]any{"dashboardId": "D", "isAppMode": true, "layout": map[string]any{"x": float64(40), "y": float64(0), "w": float64(8), "h": float64(4)}}, response: dashboard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: tc.response, err: tc.callErr}}}
			rt := boundaryRuntime(t, ChartsCreate, nil, nil, caller)
			if tc.name == "out of grid" {
				caller.steps[0].text = `{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":1,"schemaVersionTypeVerified":true}}}`
			}
			err := preflightBoundPlanChart(rt, &buildPlanState{BaseID: "B"}, "chart", tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatal(err)
			}
		})
	}
}
func TestCrossPlatformCoverageTemplateReadFailures(t *testing.T) {
	for _, tc := range []struct {
		name, receipt string
		server        bool
		err           error
	}{
		{name: "typed dependency", server: true, err: errors.New("timeout")},
		{name: "typed missing type", server: true, receipt: `{"data":{"PIE":{"chartType":"PIE"}}}`},
		{name: "typed invalid type", server: true, receipt: `{"data":{"LINE":{"chartType":"PIE"}}}`},
		{name: "typed invalid configuration", server: true, receipt: `{"data":{"LINE":[]}}`},
		{name: "legacy dependency", err: errors.New("timeout")},
		{name: "legacy missing text", receipt: `{"data":{}}`},
		{name: "legacy malformed text", receipt: `{"data":"{"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: tc.receipt, err: tc.err}}}
			choice := "--server-filter=false"
			if tc.server {
				choice = "--server-filter=true"
			}
			if out, err := runAITableCompositeCLI(t, caller, "+chart-templates", "--chart-types", "LINE", choice); err == nil || len(caller.calls) != 1 || out != "" {
				t.Fatalf("invalid template read succeeded, refetched or leaked partial output: out=%s err=%v", out, err)
			}
		})
	}
}

func TestCrossPlatformCoverageTemplateDocumentationFailuresKeepTemplates(t *testing.T) {
	for _, documentation := range []string{
		`null`, `"upstream content must not be logged"`, `[]`, `{}`,
		`{"byType":{"LINE":"raw","PIE":"raw"}}`,
		`{"common":" ","byType":{"LINE":"raw","PIE":"raw"}}`,
		`{"common":1,"byType":{"LINE":"raw","PIE":"raw"}}`,
		`{"common":"common","byType":null}`,
		`{"common":"common","byType":{"LINE":"raw"}}`,
		`{"common":"common","byType":{"LINE":"raw","PIE":{}}}`,
	} {
		t.Run(documentation, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"LINE":{"chartType":"LINE"},"PIE":{"chartType":"PIE"},"documentation":` + documentation + `}}`}}}
			out, diagnostic, err := runAITableCompositeCLIWithDiagnostics(t, context.Background(), caller, "+chart-templates", "--chart-types", "LINE,PIE", "--server-filter=true")
			if err != nil || len(caller.calls) != 1 || caller.calls[0].tool != "get_dashboard_widget_templates" {
				t.Fatalf("templates failed or refetched: out=%s err=%v calls=%#v", out, err, caller.calls)
			}
			var envelope map[string]any
			if err := json.Unmarshal([]byte(out), &envelope); err != nil {
				t.Fatalf("diagnostic polluted JSON: %v out=%s", err, out)
			}
			data := envelope["data"].(map[string]any)
			templates := data["templates"].(map[string]any)
			if envelope["ok"] != true || envelope["outcome"] != "success" || len(data) != 1 || len(templates) != 2 || templates["LINE"].(map[string]any)["chartType"] != "LINE" || templates["PIE"].(map[string]any)["chartType"] != "PIE" {
				t.Fatalf("incomplete documentation leaked or valid templates lost: %#v", envelope)
			}
			if !strings.Contains(diagnostic, "说明已整体丢弃，已保留合法 templates") || strings.Count(diagnostic, "\n") != 1 || strings.Contains(diagnostic, "upstream content must not be logged") {
				t.Fatalf("missing or unsafe diagnostic: %s", diagnostic)
			}
		})
	}
}

func TestCrossPlatformCoverageChartCreationDryRunAndMalformedInput(t *testing.T) {
	valid := `[{"config":{"chartType":"AI_ANALYZE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`
	for _, raw := range []string{"{", valid} {
		caller := &upsertByKeyCaller{}
		out, err := runAITableCompositeCLI(t, caller, "+charts-create", "--base-id", "B", "--dashboard-id", "D", "--charts", raw, "--dry-run")
		if raw == valid && err != nil {
			t.Fatalf("%s %v", out, err)
		}
		if raw != valid && err == nil {
			t.Fatal("malformed batch accepted")
		}
		if len(caller.calls) != 0 {
			t.Fatal("dry-run wrote")
		}
	}

}
