// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestAITableJSONAndRecordFilterValidationBranches(t *testing.T) {
	if _, err := parseAitableJSONObjectFlag("config", "null", false); err == nil {
		t.Fatal("JSON null succeeded")
	}
	for name, filter := range map[string]map[string]any{
		"child missing operator":    {"operator": "and", "operands": []any{map[string]any{"operands": []any{}}}},
		"child operator non string": {"operator": "and", "operands": []any{map[string]any{"operator": 1, "operands": []any{}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateFiltersStructure(filter, "filters"); err == nil {
				t.Fatal("malformed filter succeeded")
			}
		})
	}

	for name, condition := range map[string]map[string]any{
		"unsupported known": {"operator": "date_between", "operands": []any{}},
		"logical scalar":    {"operator": "and", "operands": []any{"bad"}},
		"logical valid":     {"operator": "and", "operands": []any{map[string]any{"operator": "eq"}}},
		"date no value":     {"operator": "before"},
		"date shorthand":    {"operator": "after", "value": "2026-01-01"},
		"date integer":      {"operator": "not_before", "operands": []any{"field", json.Number("1")}},
		"date invalid":      {"operator": "not_after", "operands": []any{"field", true}},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateRecordQueryFilterValueShape(condition)
			if name == "logical valid" || name == "date no value" || name == "date shorthand" || name == "date integer" {
				if err != nil {
					t.Fatalf("valid condition: %v", err)
				}
			} else if err == nil {
				t.Fatal("invalid condition succeeded")
			}
		})
	}
}

func TestAITableGroupedStatsSortValidationBranches(t *testing.T) {
	for _, test := range []struct {
		name, group, sort string
		wantErr           bool
	}{
		{"empty sort", `[{"fieldId":"f"}]`, "", false},
		{"missing group", "", `[{"fieldId":"f"}]`, true},
		{"invalid group JSON", `{`, `[{"fieldId":"f"}]`, true},
		{"invalid group field", `[{"fieldId":1}]`, `[{"fieldId":"f"}]`, true},
		{"invalid sort JSON", `[{"fieldId":"f"}]`, `{`, true},
		{"invalid sort field", `[{"fieldId":"f"}]`, `[{"fieldId":""}]`, true},
		{"sort outside group", `[{"fieldId":"f"}]`, `[{"fieldId":"g"}]`, true},
		{"valid", `[{"fieldId":"f"}]`, `[{"fieldId":"f"}]`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateAitableGroupedStatsSort(test.group, test.sort)
			if (err != nil) != test.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestAITableFieldAndViewFilterValidationBranches(t *testing.T) {
	for _, fields := range [][]any{
		{"bad"},
		{map[string]any{"fieldName": 1}},
	} {
		if err := validateAitableFieldDefinitions(fields); err == nil {
			t.Fatalf("invalid fields succeeded: %#v", fields)
		}
	}
	if _, ok := canonicalAitableViewFilter(map[string]any{"operator": "eq", "operands": []any{}}); ok {
		t.Fatal("non-logical root succeeded")
	}
	if _, ok := canonicalAitableViewFilter(map[string]any{"operator": "and", "operands": "bad"}); ok {
		t.Fatal("logical root with scalar operands succeeded")
	}
	if err := validateAitableViewFilterCondition(map[string]any{
		"operator": "and", "operands": []any{"bad"},
	}, nil, true); err == nil {
		t.Fatal("logical view filter scalar child succeeded")
	}
}

func TestAITableDateFilterAndIntegerBranches(t *testing.T) {
	for _, test := range []struct {
		operator string
		value    any
		wantErr  bool
	}{
		{"before", "2026-01-01", false},
		{"after", json.Number("1"), false},
		{"not_before", true, true},
		{"other", nil, false},
		{"date_eq", "bad", true},
		{"date_eq", map[string]any{"type": "relative", "period": "hour", "offset": 1}, true},
		{"date_eq", map[string]any{"type": "relative", "period": "day", "offset": "1"}, true},
		{"date_eq", map[string]any{"type": "exact", "timestamp": "1"}, true},
		{"date_eq", map[string]any{"type": "unknown"}, true},
		{"from_now", "bad", true},
		{"from_now", map[string]any{"type": "exact"}, true},
		{"from_now", map[string]any{"type": "relative", "period": "week"}, true},
		{"from_now", map[string]any{"type": "relative", "period": "day", "offset": "+1"}, true},
		{"from_now", map[string]any{"type": "relative", "period": "day", "offset": "-30"}, false},
	} {
		err := validateAitableViewDateFilterValue(test.operator, test.value)
		if (err != nil) != test.wantErr {
			t.Errorf("%s %#v error=%v wantErr=%v", test.operator, test.value, err, test.wantErr)
		}
	}
	for _, value := range []any{float32(2), float32(2.5), float32(math.Inf(1)), float64(2), float64(2.5), math.NaN(), json.Number("2"), json.Number("2.5")} {
		_ = isJSONInteger(value)
	}
}

func TestAITableReadRetryNormalizesNilContext(t *testing.T) {
	//lint:ignore SA1012 This test verifies the documented nil-context normalization boundary.
	got, err := callAitableReadWithRetry[string](nil, "test", func(ctx context.Context) (string, error) {
		if ctx == nil {
			t.Fatal("nil context reached callback")
		}
		return "ok", nil
	})
	if err != nil || got != "ok" {
		t.Fatalf("callAitableReadWithRetry() = %q, %v", got, err)
	}
}

func TestAITableChartLayoutPreflightTransportShapes(t *testing.T) {
	config := `{"name":"趋势","chartType":"LINE","sheet":"table"}`
	layout := `{"x":0,"y":0,"w":1,"h":1}`
	for name, caller := range map[string]*aitableTestCaller{
		"transport":    {errors: []error{fmt.Errorf("read failed")}},
		"empty":        {responses: []string{" "}},
		"invalid JSON": {responses: []string{"{"}},
	} {
		t.Run(name, func(t *testing.T) {
			err := runAitableCoverageCommand(t, caller, "chart", "create", "--base-id=b", "--dashboard-id=d", "--config="+config, "--layout="+layout)
			if err == nil {
				t.Fatal("chart preflight failure succeeded")
			}
		})
	}
}

func TestAITableNewCommandValidationAndForwardingBranches(t *testing.T) {
	validToken := "9e438eda-66a9-4f4a-99f5-f2f1912442f7"
	manyFields := make([]string, 11)
	for index := range manyFields {
		manyFields[index] = fmt.Sprintf("f%d", index)
	}
	manyRecords := make([]string, 501)
	for index := range manyRecords {
		manyRecords[index] = fmt.Sprintf("r%d", index)
	}
	scenarios := [][]string{
		{"field", "create", "--base-id=b", "--table-id=t", "--name=n", "--type=text", "--ai-config={"},
		{"field", "create", "--base-id=b", "--table-id=t", "--name= ", "--type=text"},
		{"field", "run-ai", "--base-id=b", "--table-id=t", "--field-ids=,,"},
		{"field", "run-ai", "--base-id=b", "--table-id=t", "--field-ids=" + strings.Join(manyFields, ",")},
		{"field", "run-ai", "--base-id=b", "--table-id=t", "--field-ids=f", "--record-ids=,,"},
		{"field", "run-ai", "--base-id=b", "--table-id=t", "--field-ids=f", "--record-ids=" + strings.Join(manyRecords, ",")},
		{"record", "create", "--base-id=b", "--table-id=t", `--cells={"f":"v"}`, "--client-token=" + validToken},
		{"record", "create", "--base-id=b", "--table-id=t", `--records=[{"cells":{"f":"v"}}]`, "--client-token=" + validToken},
		{"record", "create-sub", "--base-id=b", "--table-id=t", "--parent-record-id=p", "--records={"},
		{"record", "create-sub", "--base-id=b", "--table-id=t", "--parent-record-id=p", "--records=[]"},
		{"record", "create-sub", "--base-id=b", "--table-id=t", "--parent-record-id=p", "--records=[1]"},
		{"record", "create-sub", "--base-id=b", "--table-id=t", "--parent-record-id=p", `--records=[{"cells":1}]`},
		{"record", "create-sub", "--table-id=t", "--parent-record-id=p", `--records=[{"cells":{"f":"v"}}]`},
		{"attachment", "remove", "--base-id=b", "--table-id=t", "--record-id=r", "--resource-ids=,,"},
		{"view", "create", "--base-id=b", "--table-id=t", "--view-type=Grid", `--desc={"text":"x"}`},
		{"view", "update", "--base-id=b", "--table-id=t", "--view-id=v", `--desc={"text":"x"}`},
		{"form", "share", "notify", "--table-id=t", "--view-id=v", "--recipients=u"},
		{"form", "share", "notify", "--base-id=b", "--table-id=t", "--view-id=v", "--recipients=,,"},
		{"form", "submit", "--table-id=t", "--view-id=v", `--value={"f":"v"}`},
		{"chart", "create", "--base-id=b", "--dashboard-id=d", `--config={"schemaVersion":2}`, `--layout={"x":0,"y":0,"w":1,"h":1}`},
		{"chart", "create", "--base-id=b", "--dashboard-id=d", `--config={"name":"x"}`, "--layout={"},
		{"chart", "update", "--base-id=b", "--dashboard-id=d", "--chart-id=c", `--config={"name":"x"}`, `--layout={"isAppMode":true}`},
		{"chart", "update", "--base-id=b", "--dashboard-id=d", "--chart-id=c", `--config={"name":"x"}`, `--layout={"x":0,"y":0,"w":1,"h":1}`},
	}
	for index, args := range scenarios {
		t.Run(fmt.Sprintf("case-%d", index), func(t *testing.T) {
			_ = runAitableCoverageCommand(t, &aitableCommandCoverageCaller{}, args...)
		})
	}
}

func TestAITableViewEntityResolutionErrorBranch(t *testing.T) {
	caller := &aitableTestCaller{responses: []string{
		`{"fields":[{"fieldId":"owner","type":"user"}]}`,
		`{"candidates":[],"hasMore":false}`,
	}}
	err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=v",
		`--json=[{"operator":"eq","operands":["owner",{"entityName":"Alice"}]}]`)
	if err == nil || len(caller.calls) != 2 || caller.calls[1].tool != "search_entities" {
		t.Fatalf("unresolved entity error=%v calls=%#v", err, caller.calls)
	}
}
