// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageBaseSearchSelectionKeepsAITableObjectContext(t *testing.T) {
	selection := BaseSearch.Contract.Selection
	useWhen := strings.Join(selection.UseWhen, " ")
	if !strings.Contains(useWhen, "当前或上一阶段对象是 Base") || !strings.Contains(useWhen, "关键词像姓名") {
		t.Fatalf("base search UseWhen does not preserve typed Base context: %#v", selection.UseWhen)
	}
}

func TestCrossPlatformCoverageRecordFiltersCanonicalValidation(t *testing.T) {
	valid := `{"operator":"or","operands":[{"fieldId":"status","operator":"eq","value":"待联系"},{"operator":"contain","operands":["name","科技"]}]}`
	normalized, err := parseRecordFilters(valid)
	if err != nil {
		t.Fatal(err)
	}
	root := normalized.(map[string]any)
	children := root["operands"].([]any)
	if root["operator"] != "or" || children[0].(map[string]any)["operator"] != "eq" {
		t.Fatalf("normalized = %#v", normalized)
	}
	for _, invalid := range []string{
		`{"or":[{"fieldId":"status","operator":"is","value":"待联系"}]}`,
		`{"operator":"or","operands":[{"fieldId":"status","operator":"is","value":"待联系"}]}`,
		`{"operator":"eq","operands":["status","待联系"]}`,
	} {
		if _, err := parseRecordFilters(invalid); err == nil {
			t.Fatalf("invalid filter succeeded: %s", invalid)
		}
	}
}

func TestCrossPlatformCoverageRecordPaginationExplicitFalseWins(t *testing.T) {
	if responseHasMore(map[string]any{"hasMore": false, "nextCursor": "residual"}) {
		t.Fatal("explicit hasMore=false must beat residual cursor")
	}
	if responseHasMore(map[string]any{"nextCursor": "outer", "data": map[string]any{"hasMore": false}}) {
		t.Fatal("nested explicit hasMore=false must beat outer residual cursor")
	}
	if !responseHasMore(map[string]any{"hasMore": true, "nextCursor": "next"}) {
		t.Fatal("hasMore=true must continue")
	}
}

func TestCrossPlatformCoverageRecordReadBackUsesSelectSemantics(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected any
		want     bool
	}{
		{name: "single select name and expanded option", actual: map[string]any{"id": "opt-1", "name": "跟进中"}, expected: "跟进中", want: true},
		{name: "single select id and expanded option", actual: map[string]any{"id": "opt-1", "name": "跟进中"}, expected: "opt-1", want: true},
		{name: "multiple select ignores option order", actual: []any{map[string]any{"id": "b", "name": "B"}, map[string]any{"id": "a", "name": "A"}}, expected: []string{"A", "B"}, want: true},
		{name: "multiple select accepts mixed ids and names", actual: []any{map[string]any{"id": "b", "name": "B"}, map[string]any{"id": "a", "name": "A"}}, expected: []string{"a", "B"}, want: true},
		{name: "number and decimal readback string", actual: "90000.00", expected: 90000, want: true},
		{name: "json number and currency string", actual: json.Number("12.50"), expected: "12.5", want: true},
		{name: "two strings remain text", actual: "001", expected: "1", want: false},
		{name: "different option name", actual: map[string]any{"id": "opt-1", "name": "已完成"}, expected: "跟进中", want: false},
		{name: "arbitrary object remains strict", actual: map[string]any{"id": "opt-1", "name": "跟进中", "color": "blue"}, expected: "跟进中", want: false},
		{name: "single and multiple are not interchangeable", actual: []any{map[string]any{"id": "opt-1", "name": "跟进中"}}, expected: "跟进中", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := recordCellEquivalent(tc.actual, tc.expected); got != tc.want {
				t.Fatalf("recordCellEquivalent(%#v, %#v) = %v, want %v", tc.actual, tc.expected, got, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordUpdateAcceptsSemanticReadBack(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"updatedCount":1}}`},
		{text: `{"data":{"records":[{"recordId":"record-1","cells":{"fldStatus":{"id":"option-1","name":"跟进中"}}}]}}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+record-update",
		"--base-id", "base", "--table-id", "table",
		"--records", `[{"recordId":"record-1","cells":{"fldStatus":"跟进中"}}]`, "--yes")
	if err != nil || !strings.Contains(out, `"status": "verified"`) {
		t.Fatalf("semantic record update = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
	if len(caller.calls) != 2 || caller.calls[0].tool != "update_records" || caller.calls[1].tool != "query_records" {
		t.Fatalf("semantic record update calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageChartWidgetsExampleReturnsOneStandardJSONConfig(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"status":"success","data":{"HISTOGRAM":{"chartType":"HISTOGRAM","sheet":"sheet_001"},"PIE":{"chartType":"PIE","sheet":"sheet_001"}}}`}}}
	out, err := runAITableCompositeCLI(t, caller, "+chart-widgets-example", "--chart-type", "HISTOGRAM")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"chartType": "HISTOGRAM"`, `"config": {`, `"w": 12`, `"h": 6`} {
		if !strings.Contains(out, want) {
			t.Fatalf("chart output missing %s: %s", want, out)
		}
	}
	if strings.Contains(out, `"PIE"`) || len(caller.calls) != 1 || caller.calls[0].tool != "get_dashboard_widgets_example" {
		t.Fatalf("chart output/calls were not projected: out=%s calls=%#v", out, caller.calls)
	}
}

func TestCrossPlatformCoveragePrimaryDocAbsentIsSuccessfulEmptyRead(t *testing.T) {
	absent := errors.New(`[MCP_TOOL_ERROR] {"data":{},"error":{"code":"-1","message":"no record","retryable":true,"type":"SYSTEM_ERROR"}}`)
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"fields":[{"fieldId":"primary","type":"primaryDoc"}]}`},
		{text: `{"records":[{"recordId":"record","cells":{}}]}`},
		{err: absent},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+record-primary-doc-get", "--base-id", "base", "--table-id", "table", "--record-id", "record")
	if err != nil || len(caller.calls) != 3 || caller.calls[2].tool != "get_primary_doc" || !strings.Contains(out, `"exists": false`) || !strings.Contains(out, `"nodeId": null`) {
		t.Fatalf("output=%q err=%v calls=%#v", out, err, caller.calls)
	}
}
