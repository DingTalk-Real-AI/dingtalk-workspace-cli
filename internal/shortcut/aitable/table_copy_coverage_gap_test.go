// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func copyFields(count int, prefix string) []any {
	fields := make([]any, 0, count)
	for index := 0; index < count; index++ {
		fields = append(fields, map[string]any{
			"fieldId":   fmt.Sprintf("%s%d", prefix, index),
			"fieldName": fmt.Sprintf("F%d", index),
			"type":      "text",
		})
	}
	return fields
}

func TestCrossPlatformCoverageTableCopyValidationAndDryRunE2E(t *testing.T) {
	t.Run("invalid maximum", func(t *testing.T) {
		out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+table-copy",
			"--source-base-id", "b1", "--source-table-id", "t1", "--target-base-id", "b2", "--new-name", "copy", "--max-records", "0", "--yes")
		if err == nil || out != "" {
			t.Fatalf("invalid maximum = output:%q err:%v", out, err)
		}
	})

	t.Run("source read error", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: errors.New("source fields failed")}}}
		out, err := runAITableCompositeCLI(t, caller, "+table-copy",
			"--source-base-id", "b1", "--source-table-id", "t1", "--target-base-id", "b2", "--new-name", "copy", "--yes")
		if err == nil || out != "" {
			t.Fatalf("source read = output:%q err:%v", out, err)
		}
	})

	t.Run("source fields collection missing", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+table-copy",
			"--source-base-id", "b1", "--source-table-id", "t1", "--target-base-id", "b2", "--new-name", "copy", "--yes")
		if err == nil || out != "" {
			t.Fatalf("source fields collection = output:%q err:%v", out, err)
		}
	})

	t.Run("invalid source field", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"fields":[{"fieldName":"F","type":"text"}]}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+table-copy",
			"--source-base-id", "b1", "--source-table-id", "t1", "--target-base-id", "b2", "--new-name", "copy", "--yes")
		if err == nil || out != "" {
			t.Fatalf("invalid source field = output:%q err:%v", out, err)
		}
	})

	t.Run("dry run skips unsupported and preserves config", func(t *testing.T) {
		payload := map[string]any{"fields": []any{
			map[string]any{"fieldId": "skip", "fieldName": "Formula", "type": "FORMULA"},
			map[string]any{"fieldId": "keep", "fieldName": "Text", "type": "text", "config": map[string]any{"x": 1}, "aiConfig": map[string]any{"y": 2}},
		}}
		caller := &upsertByKeyCaller{dryRun: true, steps: []upsertByKeyStep{{text: mustJSONText(t, payload)}}}
		out, err := runAITableCompositeCLI(t, caller, "+table-copy",
			"--source-base-id", "b1", "--source-table-id", "t1", "--target-base-id", "b2", "--new-name", "copy", "--dry-run", "--yes")
		if err != nil || !strings.Contains(out, `"status": "planned"`) || !strings.Contains(out, "skipped field") {
			t.Fatalf("dry run = output:%q err:%v", out, err)
		}
	})

	t.Run("source records query error", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"fields":[]}`}, {err: errors.New("records failed")}}}
		out, err := runAITableCompositeCLI(t, caller, "+table-copy",
			"--source-base-id", "b1", "--source-table-id", "t1", "--target-base-id", "b2", "--new-name", "copy", "--include-records", "--yes")
		if err == nil || out != "" {
			t.Fatalf("source records = output:%q err:%v", out, err)
		}
	})
}

func TestCrossPlatformCoverageTableCopySchemaFailureStagesE2E(t *testing.T) {
	source := mustJSONText(t, map[string]any{"fields": copyFields(1, "s")})
	cases := []struct {
		name  string
		steps []upsertByKeyStep
	}{
		{name: "target fields error", steps: []upsertByKeyStep{{text: source}, {text: `{"tableId":"target"}`}, {err: errors.New("target fields failed")}}},
		{name: "target fields mismatch", steps: []upsertByKeyStep{{text: source}, {text: `{"tableId":"target"}`}, {text: `{"fields":[]}`}}},
		{name: "target mapping duplicate", steps: []upsertByKeyStep{
			{text: source}, {text: `{"tableId":"target"}`},
			{text: `{"fields":[{"fieldId":"t1","fieldName":"F0","type":"text"},{"fieldId":"t2","fieldName":"F0","type":"text"}]}`},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: tc.steps}, "+table-copy",
				"--source-base-id", "b1", "--source-table-id", "t1", "--target-base-id", "b2", "--new-name", "copy", "--yes")
			if err == nil || out != "" {
				t.Fatalf("schema failure = output:%q err:%v", out, err)
			}
		})
	}
}

func TestCrossPlatformCoverageTableCopyFieldChunkErrorRecoveredE2E(t *testing.T) {
	sourceFields := copyFields(16, "s")
	targetFields := copyFields(16, "t")
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: mustJSONText(t, map[string]any{"fields": sourceFields})},
		{text: `{"tableId":"target"}`},
		{err: errors.New("create fields reply failed")},
		{text: mustJSONText(t, map[string]any{"fields": targetFields})},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+table-copy",
		"--source-base-id", "b1", "--source-table-id", "t1", "--target-base-id", "b2", "--new-name", "copy", "--yes")
	if err != nil || !strings.Contains(out, "create_fields offset") || !strings.Contains(out, `"fieldCount": 16`) {
		t.Fatalf("field chunk recovery = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageTableCopyRecordFailureStagesE2E(t *testing.T) {
	source := mustJSONText(t, map[string]any{"fields": []any{sourceFieldFixture()}})
	target := mustJSONText(t, map[string]any{"fields": []any{targetFieldFixture()}})
	cases := []struct {
		name   string
		record string
		after  []upsertByKeyStep
	}{
		{name: "source record missing cells", record: `{"records":[{"recordId":"source"}]}`},
		{name: "created IDs read-back error", record: `{"records":[{"recordId":"source","cells":{"sf1":"v"}}]}`, after: []upsertByKeyStep{
			{text: `{"createdRecords":[{"recordId":"target-record"}]}`}, {err: errors.New("target record read failed")},
		}},
		{name: "created records mismatch", record: `{"records":[{"recordId":"source","cells":{"sf1":"v"}}]}`, after: []upsertByKeyStep{
			{text: `{"createdRecords":[{"recordId":"target-record"}]}`}, {text: `{"records":[{"recordId":"target-record","cells":{"tf1":"wrong"}}]}`},
		}},
		{name: "fallback records list", record: `{"records":[{"recordId":"source","cells":{"sf1":"v"}}]}`, after: []upsertByKeyStep{
			{text: `{"records":[{"recordId":"target-record"}]}`}, {text: `{"records":[{"recordId":"target-record","cells":{"tf1":"v"}}]}`},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			steps := []upsertByKeyStep{{text: source}, {text: tc.record}, {text: `{"tableId":"target"}`}, {text: target}}
			steps = append(steps, tc.after...)
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: steps}, "+table-copy",
				"--source-base-id", "b1", "--source-table-id", "t1", "--target-base-id", "b2", "--new-name", "copy", "--include-records", "--yes")
			if tc.name == "fallback records list" {
				if err != nil || !strings.Contains(out, `"recordCount": 1`) {
					t.Fatalf("fallback success = output:%q err:%v", out, err)
				}
				return
			}
			if err == nil || out != "" {
				t.Fatalf("record failure = output:%q err:%v", out, err)
			}
		})
	}
}

func TestCrossPlatformCoverageTableCopyPureHelpers(t *testing.T) {
	if _, err := mapCopiedFieldIDs(nil, []map[string]any{{"fieldId": "a", "fieldName": "same"}, {"fieldId": "b", "fieldName": "same"}}); err == nil {
		t.Fatal("duplicate target field must fail")
	}
	if _, err := mapCopiedFieldIDs([]map[string]any{{"fieldId": "s", "fieldName": "missing"}}, nil); err == nil {
		t.Fatal("missing target field must fail")
	}
	if normalizedFieldType("FoRmUlA") != "formula" || normalizedFieldType("custom") != "custom" {
		t.Fatal("field type normalization mismatch")
	}
}
