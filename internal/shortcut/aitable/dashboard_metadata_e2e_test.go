// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageDashboardGetPreservesSchemaVersionTypeEvidenceE2E(t *testing.T) {
	tests := []struct {
		name          string
		schemaVersion string
	}{
		{name: "number", schemaVersion: `2`},
		{name: "string", schemaVersion: `"2"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := `{"status":"success","data":{"dashboardId":"dashboard","meta":{"schemaVersion":` + test.schemaVersion + `,"schemaVersionTypeVerified":true},"charts":[]}}`
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: response}}}
			out, err := runAITableCompositeCLI(t, caller, "+dashboard-get", "--base-id", "base", "--dashboard-id", "dashboard")
			if err != nil {
				t.Fatalf("dashboard get error = %v", err)
			}
			for _, want := range []string{`"schemaVersion": ` + test.schemaVersion, `"schemaVersionTypeVerified": true`} {
				if !strings.Contains(out, want) {
					t.Fatalf("dashboard output missing %s: %s", want, out)
				}
			}
			if len(caller.calls) != 1 || caller.calls[0].tool != "get_dashboard" {
				t.Fatalf("dashboard calls = %#v", caller.calls)
			}
		})
	}
	if DashboardGet.Contract.Result == nil || !strings.Contains(string(DashboardGet.Contract.Result.DataSchema), "schemaVersionTypeVerified") {
		t.Fatalf("dashboard shortcut result contract is missing schemaVersion type evidence: %#v", DashboardGet.Contract.Result)
	}
}

func TestCrossPlatformCoverageChartUpdateShortcutEnforcesDashboardGrid(t *testing.T) {
	config := `{"name":"趋势","chartType":"LINE","sheet":"table"}`
	layout := `{"x":0,"y":0,"w":48,"h":12,"parentId":"root-responsive-layout"}`
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"status":"success","data":{"baseId":"base","dashboardId":"dashboard","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`},
		{text: `{"status":"success","data":{"chartId":"chart"}}`},
	}}
	_, err := runAITableCompositeCLI(t, caller, "+chart-update",
		"--base-id", "base", "--dashboard-id", "dashboard", "--chart-id", "chart",
		"--config", config, "--layout", layout, "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 || caller.calls[0].tool != "get_dashboard" || caller.calls[1].tool != "update_chart" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if _, exists := caller.calls[1].args["isAppMode"]; exists {
		t.Fatalf("read-only application context leaked into update_chart: %#v", caller.calls[1].args)
	}
}

func TestCrossPlatformCoverageChartUpdateShortcutFailsClosedBeforeWrite(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{
		text: `{"status":"success","data":{"baseId":"base","dashboardId":"dashboard","meta":{"schemaVersion":"2","schemaVersionTypeVerified":true}}}`,
	}}}
	_, err := runAITableCompositeCLI(t, caller, "+chart-update",
		"--base-id", "base", "--dashboard-id", "dashboard", "--chart-id", "chart",
		"--config", `{"name":"趋势","chartType":"LINE","sheet":"table"}`,
		"--layout", `{"x":0,"y":0,"w":13,"h":4}`, "--yes")
	if err == nil || !strings.Contains(err.Error(), "totalColumns=12") {
		t.Fatalf("error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "get_dashboard" {
		t.Fatalf("write must not run: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageChartUpdateShortcutUsesReadOnlyAppMode(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"status":"success","data":{"baseId":"base","dashboardId":"dashboard"}}`},
		{text: `{"status":"success","data":{"chartId":"chart"}}`},
	}}
	_, err := runAITableCompositeCLI(t, caller, "+chart-update",
		"--base-id", "base", "--dashboard-id", "dashboard", "--chart-id", "chart",
		"--config", `{"name":"趋势","chartType":"LINE","sheet":"table"}`,
		"--layout", `{"x":0,"y":0,"w":48,"h":12}`, "--is-app-mode=true", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 || caller.calls[1].tool != "update_chart" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if _, exists := caller.calls[1].args["isAppMode"]; exists {
		t.Fatalf("isAppMode leaked into update_chart: %#v", caller.calls[1].args)
	}
}
