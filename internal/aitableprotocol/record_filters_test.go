// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageRecordFiltersEquivalentSpellings(t *testing.T) {
	for _, value := range []string{`5`, `true`, `null`, `"x"`, `["opt1"]`, `[{"userId":"u"}]`, `{"nested":1}`} {
		a := `{"operator":"and","operands":[{"operator":"eq","operands":["f",` + value + `]}]}`
		b := `{"operator":" AND ","operands":[{"operator":"or","operands":[{"fieldId":"f","operator":"eq","value":` + value + `}]}]}`
		left, err := ParseRecordFilters(a)
		if err != nil {
			t.Fatal(err)
		}
		right, err := ParseRecordFilters(b)
		if err != nil {
			t.Fatal(err)
		}
		child := right["operands"].([]any)[0].(map[string]any)["operands"].([]any)[0]
		if !reflect.DeepEqual(left["operands"].([]any)[0], child) {
			t.Fatalf("normalization changed JSON value %s: %#v / %#v", value, left, right)
		}
	}
	for _, op := range strings.Split(RecordFilterOperators, ", ") {
		value := `,"value":1`
		if op == "exist" || op == "un_exist" {
			value = ""
		}
		if _, err := ParseRecordFilters(`{"operator":"and","operands":[{"fieldId":"f","operator":"` + op + `"` + value + `}]}`); err != nil {
			t.Fatalf("supported operator %s: %v", op, err)
		}
	}
	if _, err := ParseRecordFilters(`{"operator":"or","operands":[{"operator":"before","operands":["f","2026-09-23"]}]}`); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageRecordFiltersErrors(t *testing.T) {
	cases := []struct{ raw, want string }{
		{`{"operator":"and","operands":[{"operator":"eq","operands":["f",1],"not":true}]}`, "operands[0].not"},
		{`{`, "valid JSON"}, {`null`, "JSON object"}, {`[]`, "JSON object"},
		{`{}`, "operator"}, {`{"operator":3}`, "operator"}, {`{"operator":" "}`, "operator"},
		{`{"operator":"eq","operands":["f",1]}`, "at root"},
		{`{"operator":"and","fieldId":"f","operands":[{}]}`, "cannot contain fieldId"},
		{`{"operator":"and","value":1,"operands":[{}]}`, ".value"},
		{`{"operator":"and","operands":[]}`, "non-empty array"},
		{`{"operator":"and","operands":"bad"}`, "non-empty array"},
		{`{"operator":"and","operands":[null]}`, "operands[0]"},
		{`{"operator":"and","operands":[{"operator":"isGreater","operands":["f",1]}]}`, "unsupported filter operator"},
		{`{"operator":"and","operands":[{"operator":"eq","fieldId":"f","operands":["f",1]}]}`, "do not mix"},
		{`{"operator":"and","operands":[{"operator":"eq","value":1,"operands":["f",1]}]}`, ".value"},
		{`{"operator":"and","operands":[{"operator":"eq","fieldId":"f"}]}`, "exactly 2"},
		{`{"operator":"and","operands":[{"operator":"exist","fieldId":"f","value":null}]}`, "exactly 1"},
		{`{"operator":"and","operands":[{"operator":"gt","operands":["f",1,2]}]}`, "exactly 2"},
		{`{"operator":"and","operands":[{"operator":"gt","operands":[" ",1]}]}`, "non-empty fieldId"},
		{`{"operator":"and","operands":[{"operator":"gt","operands":[1,1]}]}`, "non-empty fieldId"},
	}
	for _, tc := range cases {
		if _, err := ParseRecordFilters(tc.raw); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: %v, want %q", tc.raw, err, tc.want)
		}
	}
	for _, value := range []any{" ", true, nil, math.NaN(), math.Inf(1), 1.5, map[string]any{"type": "relative"}} {
		_, err := normalizeRecordFilter(map[string]any{"operator": "date_eq", "operands": []any{"f", value}}, "filters.operands[2]", false)
		if err == nil || !strings.Contains(err.Error(), "filters.operands[2]") || !strings.Contains(err.Error(), "relative/exact") {
			t.Fatalf("date value %#v: %v", value, err)
		}
	}
}
