// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageRecordSnapshotProjections(t *testing.T) {
	cases := []struct {
		name, kind, want, got string
		equal                 bool
	}{
		{"local minutes", "date", `"2026-03-15 09:00"`, `"2026-03-15T09:00:00+08:00"`, true},
		{"legacy offset minutes", "date", `"2026-03-15T09:00+08:00"`, `"2026-03-15T09:00:00+08:00"`, true},
		{"milliseconds", "date", `1773536400000`, `"2026-03-15T09:00:00+08:00"`, true},
		{"millisecond precision", "date", `1773536459123`, `"2026-03-15T09:00:00+08:00"`, true},
		{"offset precision", "date", `"2026-03-15T01:00:59.123Z"`, `"2026-03-15T09:00:00+08:00"`, true},
		{"epoch", "date", `0`, `"1970-01-01T08:00:00+08:00"`, true},
		{"small millis", "date", `86400000`, `"1970-01-02T08:00:00+08:00"`, true},
		{"negative millis", "date", `-1`, `"1970-01-01T07:59:00+08:00"`, true},
		{"single id", "singleSelect", `{"id":"a"}`, `{"id":"a","name":"A"}`, true},
		{"single renamed", "singleSelect", `{"id":"a","name":"Old A"}`, `{"id":"a","name":"A"}`, true},
		{"multi ids reordered", "multipleSelect", `[{"id":"a"},{"id":"b"}]`, `[{"id":"b","name":"B"},{"id":"a","name":"A"}]`, true},
		{"URL string", "url", `"https://example.com"`, `{"text":"https://example.com","link":"https://example.com"}`, true},
		{"wrong second", "date", `1773536459123`, `"2026-03-15T09:00:01+08:00"`, false},
		{"fractional millis", "date", `1773536400000.5`, `"2026-03-15T09:00:00+08:00"`, false},
		{"wrong date", "date", `"2026-03-15"`, `"2026-03-16T00:00:00+08:00"`, false},
		{"wrong ID", "singleSelect", `{"id":"a","name":"A"}`, `{"id":"b","name":"A"}`, false},
		{"lost ID", "singleSelect", `{"id":"a","name":"A"}`, `"A"`, false},
		{"unknown requested key", "singleSelect", `{"id":"a","other":"requested"}`, `{"id":"a","name":"A"}`, false},
		{"non-select object", "text", `{"id":"a"}`, `{"id":"a","name":"A"}`, false},
		{"wrong URL", "url", `"https://example.com"`, `{"text":"https://example.com","link":"https://other.example.com"}`, false},
		{"wrong URL label", "url", `"https://example.com"`, `{"text":"Other","link":"https://example.com"}`, false},
		{"non-URL object", "text", `"https://example.com"`, `{"text":"https://example.com","link":"https://example.com"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got, want any
			if err := json.Unmarshal([]byte(tc.got), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if equal := recordCellValueEqual(got, want, tc.kind); equal != tc.equal {
				t.Fatalf("typed equal=%v, want %v", equal, tc.equal)
			}
			resolver := resolvedRecordFieldTypeResolver([]map[string]any{{"fieldId": "f", "type": tc.kind}})
			err := resolver.verify(map[string]any{"cells": map[string]any{"f": got}}, map[string]any{"f": want})
			if (err == nil) != tc.equal {
				t.Fatalf("resolver error=%v, want equal=%v", err, tc.equal)
			}
			if !tc.equal {
				return
			}
			// Exercise the real CLI resolver and its get_fields trigger, not just
			// a supplied field type. A successful MCP write must stay successful.
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
				{text: `{"status":"success","data":{"recordIds":["r1"]}}`},
				{text: mustJSONText(t, map[string]any{"records": []any{map[string]any{"recordId": "r1", "cells": map[string]any{"f": got}}}})},
				{text: mustJSONText(t, map[string]any{"fields": []any{map[string]any{"fieldId": "f", "type": tc.kind}}})},
			}}
			out, err := runRecordBatchCLI(t, caller, "+record-update", []map[string]any{{"recordId": "r1", "cells": map[string]any{"f": want}}})
			if err != nil || !strings.Contains(out, `"status": "verified"`) || len(caller.calls) != 3 {
				t.Fatalf("output=%s error=%v calls=%v", out, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordDateProjectionBoundaries(t *testing.T) {
	for _, value := range []string{"0001-01-01T00:00:00+08:00", "9999-12-31T23:59:00+08:00"} {
		date, ok := recordDateWriteTime(value)
		if !ok || !recordDateValueEqual(value, json.Number(strconv.FormatInt(date.UnixMilli(), 10))) {
			t.Fatalf("valid boundary rejected: %s", value)
		}
	}
	for _, value := range []any{json.Number("-62135625600001"), json.Number("253402272000000"), json.Number("9223372036854775808"), "0000-01-01", "bad", true, nil} {
		if _, ok := recordDateWriteTime(value); ok {
			t.Fatalf("invalid boundary accepted: %v", value)
		}
	}
}
