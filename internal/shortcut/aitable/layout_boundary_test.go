// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChartSizeDiagnosticsAreReadOnlyAndVersioned(t *testing.T) {
	for _, tc := range []struct {
		name, version, charts string
		app                   bool
		checked, issues       int
		columns               int
	}{
		{name: "numeric V2", columns: 48, version: "2", charts: `[{"chartId":"C","chartType":"BAR"}]`, checked: 1, issues: 1},
		{name: "string V1", columns: 12, version: `"2"`, charts: `[{"chartId":"C","chartType":"BAR"}]`, checked: 1},
		{name: "confirmed App V2", columns: 48, version: "1", charts: `[{"chartId":"C","chartType":"BAR"}]`, app: true, checked: 1, issues: 1},
		{name: "old response without types", columns: 48, version: "2", charts: `[]`},
		{name: "unknown type", columns: 48, version: "2", charts: `[{"chartId":"C","chartType":"RADAR"}]`},
		{name: "ambiguous identity", columns: 48, version: "2", charts: `[{"chartId":"C","chartType":"BAR"},{"chartId":"C","chartType":"LINE"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			receipt := fmt.Sprintf(`{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":%s,"schemaVersionTypeVerified":true},"layout":[{"i":"C","x":0,"y":0,"w":4,"h":8}],"charts":%s}}`, tc.version, tc.charts)
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: receipt}}}
			args := []string{"--base-id", "B", "--dashboard-id", "D"}
			if tc.app {
				args = append(args, "--is-app-mode=true")
			}
			out, err := runAITableCompositeCLI(t, caller, "+dashboard-diagnose", args...)
			if err != nil || len(caller.calls) != 1 || caller.calls[0].tool != "get_dashboard" {
				t.Fatalf("diagnosis failed or wrote: %s %v %#v", out, err, caller.calls)
			}
			var envelope map[string]any
			if err := json.Unmarshal([]byte(out), &envelope); err != nil {
				t.Fatal(err)
			}
			data := envelope["data"].(map[string]any)
			if data["totalColumns"] != float64(tc.columns) || data["geometryVerified"] != true || data["pageReadableVerified"] != false || len(data["sizeIssues"].([]any)) != tc.issues || len(data["sizeCheckedChartIds"].([]any)) != tc.checked {
				t.Fatalf("geometry, type coverage or page semantics lost: %s", out)
			}
		})
	}
}

func TestCrossPlatformCoverageVOCSixSizeDiagnostics(t *testing.T) {
	types := []string{"PIE", "BAR", "BAR", "PIE", "BAR", "LINE"}
	for _, height := range []int{8, 15} {
		layouts, charts := []map[string]any{}, []map[string]any{}
		for i, kind := range types {
			id := fmt.Sprintf("VOC-%d", i)
			layouts = append(layouts, map[string]any{"i": id, "x": i % 3 * 16, "y": i / 3 * height, "w": 16, "h": height})
			charts = append(charts, map[string]any{"chartId": id, "chartType": kind})
		}
		raw, err := json.Marshal(map[string]any{"data": map[string]any{"baseId": "B", "dashboardId": "D", "meta": map[string]any{"schemaVersion": 2, "schemaVersionTypeVerified": true}, "layout": layouts, "charts": charts}})
		if err != nil {
			t.Fatal(err)
		}
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: string(raw)}}}
		out, err := runAITableCompositeCLI(t, caller, "+dashboard-diagnose", "--base-id", "B", "--dashboard-id", "D")
		if err != nil || len(caller.calls) != 1 || caller.calls[0].tool != "get_dashboard" {
			t.Fatalf("unexpected calls: %v %#v", err, caller.calls)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatal(err)
		}
		data := envelope["data"].(map[string]any)
		want := 0
		if height == 8 {
			want = 6
		}
		if len(data["sizeIssues"].([]any)) != want || len(data["sizeCheckedChartIds"].([]any)) != 6 || data["geometryVerified"] != true || data["pageReadableVerified"] != false {
			t.Fatalf("VOC height %d: %s", height, out)
		}
	}
}

func TestCrossPlatformCoverageLayoutReadBoundaries(t *testing.T) {
	layout := `{"i":"C","x":0,"y":0,"w":12,"h":4}`
	dashboard := `{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true},"layout":[` + layout + `,{"i":"child","parentId":"P"}]}}`
	cases := []struct {
		name, command, receipt string
		args                   []string
		callErr                error
		wantErr                bool
	}{
		{name: "diagnose with design and region", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--expected-layout", "[" + layout + "]", "--regions", `[{"id":"r","x":0,"y":0,"w":12,"h":4}]`}},
		{name: "diagnose dependency", command: "+dashboard-diagnose", callErr: errors.New("read timeout"), wantErr: true},
		{name: "diagnose wrong target", command: "+dashboard-diagnose", receipt: `{"baseId":"other"}`, wantErr: true},
		{name: "diagnose missing layout", command: "+dashboard-diagnose", receipt: `{"data":{"baseId":"B","dashboardId":"D","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`, wantErr: true},
		{name: "bad expected JSON", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--expected-layout", "{}"}, wantErr: true},
		{name: "bad expected shape", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--expected-layout", `[{"i":"C"}]`}, wantErr: true},
		{name: "bad region JSON", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--regions", "{}"}, wantErr: true},
		{name: "bad region geometry", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--regions", `[{"id":"r","x":0,"y":0,"w":0,"h":4}]`}, wantErr: true},
		{name: "bad region field", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--regions", `[{"id":"r","x":0,"y":0,"w":1,"h":1,"unexpected":true}]`}, wantErr: true},
		{name: "bad region identity", command: "+dashboard-diagnose", receipt: dashboard, args: []string{"--regions", `[{"id":"","x":0,"y":0,"w":1,"h":1}]`}, wantErr: true},
		{name: "preview dry", command: "+dashboard-preview", args: []string{"--dry-run"}, wantErr: true},
		{name: "preview dependency", command: "+dashboard-preview", callErr: errors.New("timeout"), wantErr: true},
		{name: "preview mismatch", command: "+dashboard-preview", receipt: `{}`, wantErr: true},
		{name: "preview no before", command: "+dashboard-preview", receipt: `{"baseId":"B","dashboardId":"D","preview":true,"layoutFingerprint":"fp","layout":[]}`, wantErr: true},
		{name: "preview no after", command: "+dashboard-preview", receipt: `{"baseId":"B","dashboardId":"D","preview":true,"layoutFingerprint":"fp","beforeLayout":[]}`, wantErr: true},
		{name: "preview valid", command: "+dashboard-preview", receipt: `{"baseId":"B","dashboardId":"D","preview":true,"layoutFingerprint":"fp","beforeLayout":[],"layout":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: tc.receipt, err: tc.callErr}}}
			args := append([]string{"--base-id", "B", "--dashboard-id", "D"}, tc.args...)
			out, err := runAITableCompositeCLI(t, caller, tc.command, args...)
			if (err != nil) != tc.wantErr {
				t.Fatalf("out=%s err=%v", out, err)
			}
			if !tc.wantErr && tc.command == "+dashboard-diagnose" && (!strings.Contains(out, `"excludedChildItems": 1`) || !strings.Contains(out, `"designVerified": true`) || !strings.Contains(out, `"regionCoverageVerified": true`)) {
				t.Fatalf("lost design evidence: %s", out)
			}
		})
	}
}

func TestCrossPlatformCoverageReadBatchFailures(t *testing.T) {
	for _, tc := range []struct {
		name, requests, width string
		dry                   bool
		readErr               error
	}{
		{name: "dry", requests: `[{"tool":"get_base","arguments":{"baseId":"B"}}]`, width: "1", dry: true},
		{name: "malformed", requests: "{}", width: "1"},
		{name: "bad width", requests: `[{"tool":"get_base","arguments":{"baseId":"B"}}]`, width: "5"},
		{name: "unsafe tool", requests: `[{"tool":"create_dashboard","arguments":{"baseId":"B"}}]`, width: "1"},
		{name: "partial failure", requests: `[{"tool":"get_base","arguments":{"baseId":"B"}}]`, width: "1", readErr: errors.New("timeout")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: tc.readErr}}}
			args := []string{"--base-id", "B", "--requests", tc.requests, "--concurrency", tc.width}
			if tc.dry {
				args = append(args, "--dry-run")
			}
			if _, err := runAITableCompositeCLI(t, caller, "+read-batch", args...); err == nil {
				t.Fatal("unsafe request succeeded")
			}
		})
	}
}
