// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageParityDashboardCreationReadback(t *testing.T) {
	for _, chart := range []bool{false, true} {
		for _, bad := range []string{"", "id", "config", "read"} {
			args := []string{"--base-id", "b", "--yes"}
			cmd := "+dashboard-create"
			cfg := map[string]any{"name": "Name", "filters": []any{}}
			c := &upsertByKeyCaller{}
			if !chart {
				args = append(args, "--name", "Name", "--config", mustJSONText(t, cfg))
				actual := map[string]any{"dashboardId": "d", "dashboardName": "Name", "filters": []any{}}
				if bad == "id" {
					actual["dashboardId"] = "other"
				}
				if bad == "config" {
					actual["filters"] = []any{"changed"}
				}
				c.steps = []upsertByKeyStep{parityStep(map[string]any{"dashboardId": "d"}), parityStep(actual)}
			} else {
				cmd = "+chart-create"
				cfg = map[string]any{"chartType": "AI_ANALYZE", "name": "Name"}
				layout := map[string]any{"x": 0, "y": 0, "w": 6, "h": 4}
				args = append(args, "--dashboard-id", "d", "--config", mustJSONText(t, cfg), "--layout", mustJSONText(t, layout))
				snap := map[string]any{"baseId": "b", "dashboardId": "d", "meta": map[string]any{"schemaVersion": 1, "schemaVersionTypeVerified": true}}
				actual := map[string]any{"chartId": "c", "dashboardId": "d", "config": cfg, "layout": layout}
				if bad == "id" {
					actual["chartId"] = "other"
				}
				if bad == "config" {
					actual["layout"] = map[string]any{"x": 0, "y": 0, "w": 1, "h": 1}
				}
				c.steps = []upsertByKeyStep{parityStep(map[string]any{"data": snap}), parityStep(map[string]any{"chartId": "c"}), parityStep(actual)}
			}
			if bad == "read" {
				c.steps[len(c.steps)-1] = upsertByKeyStep{err: fmt.Errorf("read unavailable")}
			}
			out, err := runAITableCompositeCLI(t, c, cmd, args...)
			if (err != nil) != (bad != "") {
				t.Fatal(chart, bad, out, err)
			}
			if bad == "" && !strings.Contains(out, `"status": "verified"`) {
				t.Fatal(out)
			}
		}
	}
	for _, tc := range []struct {
		cmd  string
		args []string
	}{
		{"+dashboard-create", []string{"--name", " ", "--config", "{}"}},
		{"+dashboard-create", []string{"--name", "Name", "--config", "[]"}},
		{"+chart-create", []string{"--dashboard-id", "d", "--config", "{}", "--layout", "{}"}},
		{"+chart-create", []string{"--dashboard-id", "d", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", "not-json"}},
	} {
		c := &upsertByKeyCaller{}
		_, err := runAITableCompositeCLI(t, c, tc.cmd, append([]string{"--base-id", "b", "--yes"}, tc.args...)...)
		if err == nil || len(c.calls) > 0 {
			t.Fatal("invalid create reached network", tc, err, c.calls)
		}
	}
}
func TestCrossPlatformCoverageParityDashboardListIdentity(t *testing.T) {
	for _, raw := range []string{`{"baseId":"b","dashboards":[{"dashboardId":"d"}]}`, `{"baseId":"other","dashboards":[]}`, `{"baseId":"b"}`, `{"baseId":"b","dashboards":[{}]}`, `{"baseId":"b","dashboards":[{"dashboardId":"d"},{"dashboardId":"d"}]}`} {
		c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: raw}}}
		out, err := runAITableCompositeCLI(t, c, "+dashboard-list", "--base-id", "b")
		valid := strings.Contains(raw, `[{"dashboardId":"d"}]`)
		if (err == nil) != valid {
			t.Fatal(out, err, raw)
		}
	}
}
func TestCrossPlatformCoverageParityStatsInputAndGroupedContract(t *testing.T) {
	valid := `{"stats":[{"fieldId":"f","statsType":"SUM"}]}`
	changes := map[string]any{"stats": []any{true}, "group": "bad", "filters": "bad", "sort": "bad", "dataVersion": 123, "keyword": " "}
	invalid := []string{"null", "{}", `{"stats":[]}`, `{"stats":[{}]}`, `{"stats":[{"fieldId":"f","statsType":"sum"}]}`, `{"stats":[{"fieldId":"f","statsType":"SUM"},{"fieldId":"f","statsType":"MAX"}]}`}
	for key, value := range changes {
		var d map[string]any
		json.Unmarshal([]byte(valid), &d)
		d[key] = value
		invalid = append(invalid, mustJSONText(t, d))
	}
	for _, entry := range []map[string]any{{"group": []any{true}}, {"sort": []any{true}}, {"sort": []any{map[string]any{"fieldId": "f", "direction": "bad"}}}, {"group": []any{map[string]any{"fieldId": "g"}}, "keyword": "word"}} {
		var d map[string]any
		json.Unmarshal([]byte(valid), &d)
		for k, v := range entry {
			d[k] = v
		}
		invalid = append(invalid, mustJSONText(t, d))
	}
	for _, raw := range invalid {
		c := &upsertByKeyCaller{}
		out, err := runAITableCompositeCLI(t, c, "+data-query", "--base-id", "b", "--table-id", "t", "--dsl", raw)
		if err == nil || out != "" || len(c.calls) > 0 {
			t.Fatal(raw, out, err, c.calls)
		}
	}
	grouped := map[string]any{"results": []any{map[string]any{"groupKeys": []any{map[string]any{"fieldId": "g", "value": "A"}}, "fieldStatsMap": map[string]any{"f": map[string]any{"action": "SUM", "value": "10.0"}}}}}
	for _, group := range []bool{false, true} {
		response := map[string]any{"data": map[string]any{"results": []any{map[string]any{"results": []any{map[string]any{"fieldId": "f", "statsType": "SUM", "value": "10.0"}}}}}}
		dsl := map[string]any{"stats": []any{map[string]any{"fieldId": "f", "statsType": "SUM"}}, "dataVersion": "123", "sort": []any{map[string]any{"fieldId": "f", "direction": "DESC"}}, "filters": map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "eq", "operands": []any{"g", "A"}}}}}
		if group {
			dsl["group"] = []any{map[string]any{"fieldId": "g"}}
			response = grouped
		} else {
			dsl["group"] = []any{}
			dsl["keyword"] = "A"
		}
		c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(response)}}
		out, err := runAITableCompositeCLI(t, c, "+data-query", "--base-id", "b", "--table-id", "t", "--dsl", mustJSONText(t, dsl))
		if err != nil || !strings.Contains(out, "10.0") {
			t.Fatal(out, err)
		}
		if group && c.calls[0].tool != "query_stats" || !group && c.calls[0].tool != "query_records_stats" {
			t.Fatal(c.calls)
		}
	}
	stats := []any{map[string]any{"fieldId": "f", "statsType": "SUM"}}
	for _, raw := range []string{`{"results":[1]}`, `{"results":[{}]}`, `{"results":[{"groupKeys":[{}]}]}`, `{"results":[{"groupKeys":[{"fieldId":"g"}]}]}`, `{"results":[{"groupKeys":[{"fieldId":"g","value":1}]}]}`, `{"results":[{"groupKeys":[{"fieldId":"g","value":1}],"fieldStatsMap":{"f":{"action":"AVG","value":1}}}]}`, `{"results":[{"groupKeys":[{"fieldId":"g","value":1}],"fieldStatsMap":{"f":{"action":"SUM"}}}]}`} {
		var m map[string]any
		json.Unmarshal([]byte(raw), &m)
		if e := validateParityStats(m, stats, "grouped"); e == nil {
			t.Fatal("invalid grouped success", raw)
		}
	}
	if e := validateParityStats(map[string]any{"results": []any{}}, stats, "grouped"); e != nil {
		t.Fatal("empty groups legitimate", e)
	}
}

func TestCrossPlatformCoverageParityGroupedEmptySentinelIsNarrow(t *testing.T) {
	body := map[string]any{"dataVersion": float64(89), "results": []any{map[string]any{"fieldStatsMap": map[string]any{}, "groupKeys": []any{}}}}
	raw := map[string]any{"success": true, "status": "success", "data": body}
	if !explicitEmptyGroupedStats(raw, body) {
		t.Fatal("reviewed empty sentinel rejected")
	}
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(raw)}}
	out, e := runAITableCompositeCLI(t, c, "+data-query", "--base-id", "b", "--table-id", "t", "--dsl", `{"stats":[{"fieldId":"f","statsType":"SUM"}],"group":[{"fieldId":"g"}]}`)
	if e != nil || !strings.Contains(out, `"results": []`) {
		t.Fatal(out, e)
	}
	for _, bad := range []map[string]any{{}, {"dataVersion": -1, "results": body["results"]}, {"dataVersion": 89, "results": []any{}}, {"dataVersion": 89, "results": []any{true}}, {"dataVersion": 89, "results": []any{map[string]any{"groupKeys": []any{}}}}} {
		if explicitEmptyGroupedStats(raw, bad) {
			t.Fatal("ambiguous response called empty", bad)
		}
	}
	if explicitEmptyGroupedStats(map[string]any{"success": false}, body) {
		t.Fatal("failed request called empty")
	}
}

func TestCrossPlatformCoverageParityGroupedEmptyLiveEnvelope(t *testing.T) {
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"summary":"Stats queried","data":{"dataVersion":89,"results":[{"fieldStatsMap":{},"groupKeys":[]}]},"meta":{}}`}}}
	out, err := runAITableCompositeCLI(t, c, "+data-query", "--base-id", "b", "--table-id", "t", "--dsl", `{"stats":[{"fieldId":"f","statsType":"SUM"}],"group":[{"fieldId":"g"}]}`)
	if err != nil || !strings.Contains(out, `"results": []`) {
		t.Fatal(out, err)
	}
	for _, raw := range []string{`{"success":false,"data":{"dataVersion":89,"results":[{"fieldStatsMap":{},"groupKeys":[]}]}}`, `{"status":"error","data":{"dataVersion":89,"results":[{"fieldStatsMap":{},"groupKeys":[]}]}}`, `{"summary":"Stats queried","data":{"dataVersion":89}}`} {
		c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: raw}}}
		out, err := runAITableCompositeCLI(t, c, "+data-query", "--base-id", "b", "--table-id", "t", "--dsl", `{"stats":[{"fieldId":"f","statsType":"SUM"}],"group":[{"fieldId":"g"}]}`)
		if err == nil || out != "" {
			t.Fatal("false empty", out, err)
		}
	}
}

func TestCrossPlatformCoverageParityDashboardFailureBoundaries(t *testing.T) {
	fail := upsertByKeyStep{err: fmt.Errorf("offline")}
	chartArgs := []string{"--dashboard-id", "d", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"x":0,"y":0,"w":6,"h":4}`}
	snap := upsertByKeyStep{text: `{"data":{"baseId":"b","dashboardId":"d","meta":{"schemaVersion":1,"schemaVersionTypeVerified":true}}}`}
	for _, tc := range []struct {
		cmd   string
		args  []string
		steps []upsertByKeyStep
	}{
		{"+dashboard-list", nil, []upsertByKeyStep{fail}},
		{"+dashboard-create", []string{"--name", "N", "--config", `{"isAppMode":true}`}, nil},
		{"+chart-create", []string{"--dashboard-id", "d", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"schemaVersion":2}`}, nil},
		{"+chart-create", []string{"--dashboard-id", "d", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"x":0,"y":0,"w":49,"h":4}`}, nil},
		{"+chart-create", chartArgs, []upsertByKeyStep{fail}},
		{"+chart-create", chartArgs, []upsertByKeyStep{{text: `{}`}}},
		{"+chart-create", []string{"--dashboard-id", "d", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"x":0,"y":0,"w":20,"h":4}`}, []upsertByKeyStep{snap}},
		{"+dashboard-create", []string{"--name", "N"}, []upsertByKeyStep{fail}},
		{"+dashboard-create", []string{"--name", "N"}, []upsertByKeyStep{{text: `{"success":true}`}}},
		{"+dashboard-create", []string{"--name", "N"}, []upsertByKeyStep{{text: `{"dashboardId":"d"}`}, {text: `{"dashboardId":"d","dashboardName":"Wrong"}`}}},
		{"+chart-create", chartArgs, []upsertByKeyStep{snap, {text: `{"chartId":"c"}`}, {text: `{"chartId":"c","dashboardId":"wrong"}`}}},
	} {
		c := &upsertByKeyCaller{steps: tc.steps}
		out, err := runAITableCompositeCLI(t, c, tc.cmd, append([]string{"--base-id", "b", "--yes"}, tc.args...)...)
		if err == nil || out != "" || len(c.calls) != len(tc.steps) {
			t.Fatal(tc.cmd, out, err, len(c.calls), len(tc.steps))
		}
	}
	c := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, c, "+dashboard-create", "--base-id", "b", "--name", "N", "--dry-run")
	if err != nil || len(c.calls) != 0 || !strings.Contains(out, `"executed": false`) {
		t.Fatal(out, err)
	}
}
func TestCrossPlatformCoverageParityStatsReadErrorAndPreview(t *testing.T) {
	dsl := `{"stats":[{"fieldId":"f","statsType":"SUM"}]}`
	for _, step := range []upsertByKeyStep{{err: fmt.Errorf("offline")}, {text: `{}`}, {text: `{"results":[1]}`}} {
		c := &upsertByKeyCaller{steps: []upsertByKeyStep{step}}
		out, err := runAITableCompositeCLI(t, c, "+data-query", "--base-id", "b", "--table-id", "t", "--dsl", dsl)
		if err == nil || out != "" {
			t.Fatal(out, err)
		}
	}
	c := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, c, "+data-query", "--base-id", "b", "--table-id", "t", "--dsl", dsl, "--dry-run")
	if err != nil || len(c.calls) != 0 || !strings.Contains(out, `"executed": false`) {
		t.Fatal(out, err)
	}
	if explicitEmptyGroupedStats(map[string]any{"status": "error"}, map[string]any{}) {
		t.Fatal("error considered empty")
	}
}
