// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCrossPlatformCoverageDashboardGetNullableNavigationRuntime(t *testing.T) {
	var schema any
	if err := json.Unmarshal(DashboardGet.Contract.Result.DataSchema, &schema); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("https://dws.test/dashboard-get.json", schema); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile("https://dws.test/dashboard-get.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, navigation := range []string{"", `,"url":null,"links":null`, `,"url":"","links":[]`} {
		response := `{"status":"success","data":{"dashboardId":"d","meta":null` + navigation + `}}`
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: response}}}
		out, err := runAITableCompositeCLI(t, caller, "+dashboard-get", "--base-id", "b", "--dashboard-id", "d")
		if err != nil {
			t.Fatal(err)
		}
		var actual, want map[string]any
		if err := json.Unmarshal([]byte(out), &actual); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(response), &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, want) || len(caller.calls) != 1 || caller.calls[0].tool != "get_dashboard" {
			t.Fatalf("get navigation changed: %s calls=%#v", out, caller.calls)
		}
		if err := compiled.Validate(actual); err != nil {
			t.Fatalf("runtime output violates Result: %v\n%s", err, out)
		}
		for key, value := range map[string]any{"url": true, "links": map[string]any{}} {
			invalid := maps.Clone(actual)
			data := maps.Clone(actual["data"].(map[string]any))
			data[key] = value
			invalid["data"] = data
			if compiled.Validate(invalid) == nil {
				t.Fatalf("invalid %s accepted", key)
			}
		}
	}
}

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
	if caller.calls[1].args["isAppMode"] != false {
		t.Fatalf("ordinary grid context was not forwarded: %#v", caller.calls[1].args)
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
	if caller.calls[1].args["isAppMode"] != true {
		t.Fatalf("read-only grid context was not forwarded: %#v", caller.calls[1].args)
	}
}
