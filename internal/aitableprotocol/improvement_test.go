// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageJSONCProjectionPreservesStrings(t *testing.T) {
	raw := `{ /* template */ "LINE":{"chartType":"LINE","url":"https://example.test/a//b?c=/*literal*/","quote":"a\\\"//b",}, // next
"PIE":{"chartType":"PIE"}, }`
	got, err := ProjectChartTemplates(raw, []string{"LINE"})
	if err != nil {
		t.Fatal(err)
	}
	config := got["LINE"].(map[string]any)
	if len(got) != 1 || config["url"] != "https://example.test/a//b?c=/*literal*/" {
		t.Fatalf("projection changed strings: %#v", got)
	}
	for _, value := range []string{`{"LINE":`, `{"LINE":{}}`, `null`, `[]`, `{/* unterminated`, `{"values":[,]}`, `{"values":{,}}`, strings.Repeat(" ", 8<<20+1)} {
		if _, err := ParseJSONC(value); err == nil && value != `{"LINE":{}}` {
			t.Fatalf("accepted invalid %q", value[:min(30, len(value))])
		}
	}
	for _, types := range [][]string{nil, {"unknown"}, {"LINE", "LINE"}} {
		if _, err := ProjectChartTemplates(raw, types); err == nil {
			t.Fatal("accepted missing/duplicate type")
		}
	}
}

func TestCrossPlatformCoverageStructuralResourceBindings(t *testing.T) {
	resources := map[string]map[string]any{"table": {"tableId": "t", "recordIds": []any{"r"}}}
	value := map[string]any{"sheet": map[string]any{"$ref": "table.tableId"}, "literal": "${table.tableId}", "nested": []any{map[string]any{"$ref": "table.recordIds"}}}
	got, err := BindReferences(value, resources)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["sheet"] != "t" || !reflect.DeepEqual(value["literal"], got.(map[string]any)["literal"]) {
		t.Fatal(got)
	}
	for _, v := range []any{map[string]any{"$ref": "missing.tableId"}, map[string]any{"$ref": "table"}, map[string]any{"$ref": 3}, map[string]any{"$ref": "table.tableId", "other": 1}} {
		if _, err := BindReferences(v, resources); err == nil {
			t.Fatal("invalid reference accepted")
		}
	}
}

func TestCrossPlatformCoverageKnownChartMinimumSizes(t *testing.T) {
	for _, columns := range []int{12, 48} {
		for _, kind := range []string{"BAR", "LINE", "HISTOGRAM", "PIE", "SUMMARY"} {
			minW, minH := 3, 4
			if kind == "SUMMARY" {
				minW = 2
			}
			if columns == 48 {
				minW *= 4
				minH *= 3
				if kind == "SUMMARY" {
					minH = 8
				}
			}
			charts := []map[string]any{{"chartId": "C", "chartType": kind}}
			for _, dimension := range []string{"w", "h", "pass"} {
				item := map[string]any{"i": "C", "x": 0, "y": 0, "w": minW, "h": minH}
				if dimension != "pass" {
					item[dimension] = item[dimension].(int) - 1
				}
				before := map[string]any{}
				for key, value := range item {
					before[key] = value
				}
				issues, checked := DiagnoseChartMinimumSizes([]map[string]any{item}, charts, columns)
				if len(checked) != 1 || checked[0] != "C" || (len(issues) == 0) != (dimension == "pass") || !reflect.DeepEqual(item, before) {
					t.Fatalf("%s/%d/%s: issues=%#v checked=%#v item=%#v", kind, columns, dimension, issues, checked, item)
				}
				if len(issues) != 0 && issues[0].Code != "below_type_minimum" {
					t.Fatal(issues)
				}
			}
		}
	}
	item := map[string]any{"i": "C", "x": 0, "y": 0, "w": 1, "h": 1}
	for _, charts := range [][]map[string]any{nil, {{"chartType": "BAR"}}, {{"chartId": 42, "chartType": "BAR"}}, {{"chartId": "C", "chartType": "RADAR"}}, {{"chartId": "other", "chartType": "BAR"}}, {{"chartId": "C", "chartType": "BAR"}, {"chartId": "C", "chartType": "LINE"}}} {
		if issues, checked := DiagnoseChartMinimumSizes([]map[string]any{item}, charts, 48); len(issues) != 0 || len(checked) != 0 {
			t.Fatalf("unverified chart inspected: %#v %#v", issues, checked)
		}
	}
	charts := []map[string]any{{"chartId": "C", "chartType": "BAR"}}
	for _, items := range [][]map[string]any{{item, item}, {{"i": "C", "parentId": "tab", "x": 0, "y": 0, "w": 1, "h": 1}}, {{"i": "C", "x": 0, "y": 0, "w": 0, "h": 1}}} {
		if issues, checked := DiagnoseChartMinimumSizes(items, charts, 48); len(issues) != 0 || len(checked) != 0 {
			t.Fatalf("invalid or child geometry inspected: %#v %#v", issues, checked)
		}
	}
	if issues, checked := DiagnoseChartMinimumSizes([]map[string]any{item}, charts, 24); len(issues) != 0 || len(checked) != 0 {
		t.Fatal("unverified grid accepted")
	}
}

func TestCrossPlatformCoverageRootLayoutDiagnostics(t *testing.T) {
	items := []map[string]any{{"i": "a", "x": 0, "y": 0, "w": 8, "h": 4}, {"i": "b", "x": 8, "y": 0, "w": 12, "h": 8}, {"i": "c", "x": 0, "y": 4, "w": 8, "h": 4}}
	if issues := DiagnoseRootLayout(items, 48); len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, item := range []map[string]any{{"i": "a", "x": 47, "y": 0, "w": 2, "h": 4}, {"i": "b", "x": 0, "y": 0, "w": 1, "h": 1, "minW": 8}, {"i": "b", "x": 0, "y": 0, "w": 8, "h": 4, "minH": "wrong"}, {"i": "child", "x": 0, "y": 0, "w": 8, "h": 4, "parentId": "tab"}, {"x": 0, "y": 0, "w": 8, "h": 4}, {"i": "a", "x": 0, "y": 0, "w": 8, "h": 4}} {
		if issues := DiagnoseRootLayout(append(items[:1:1], item), 48); len(issues) == 0 {
			t.Fatalf("missed %#v", item)
		}
	}
}
