// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageRecordObjectAndIDValidation(t *testing.T) {
	tooMany := make([]any, maxCompositeRecordRun+1)
	for index := range tooMany {
		tooMany[index] = map[string]any{"cells": map[string]any{"f": index}}
	}
	cases := []struct {
		name      string
		raw       string
		requireID bool
	}{
		{name: "invalid JSON", raw: `{`},
		{name: "not array", raw: `{}`},
		{name: "empty array", raw: `[]`},
		{name: "too many", raw: mustJSONText(t, tooMany)},
		{name: "non-object", raw: `[1]`},
		{name: "missing cells", raw: `[{"recordId":"r"}]`},
		{name: "empty cells", raw: `[{"recordId":"r","cells":{}}]`},
		{name: "missing record id", raw: `[{"cells":{"f":1}}]`, requireID: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if records, err := parseRecordObjects(tc.raw, tc.requireID); err == nil || records != nil {
				t.Fatalf("parseRecordObjects = %#v, %v", records, err)
			}
		})
	}
	if ids, err := parseRecordIDs([]string{" ", "b", "a", "a"}); err != nil || strings.Join(ids, ",") != "a,b" {
		t.Fatalf("parseRecordIDs = %#v, %v", ids, err)
	}
	tooManyIDs := make([]string, maxCompositeRecordRun+1)
	for index := range tooManyIDs {
		tooManyIDs[index] = strconv.Itoa(index)
	}
	if ids, err := parseRecordIDs(tooManyIDs); err == nil || ids != nil {
		t.Fatalf("too many IDs = %#v, %v", ids, err)
	}
}

func TestCrossPlatformCoverageRecordBatchEntryValidationE2E(t *testing.T) {
	for _, command := range []string{"+record-update", "+record-upsert"} {
		t.Run(command, func(t *testing.T) {
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, command,
				"--base-id", "base", "--table-id", "table", "--records", `{`, "--yes")
			if err == nil || out != "" {
				t.Fatalf("invalid records = output:%q err:%v", out, err)
			}
		})
	}
	out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+record-delete",
		"--base-id", "base", "--table-id", "table", "--record-ids", ` `, "--yes")
	if err == nil || out != "" {
		t.Fatalf("invalid record IDs = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageRecordUpdateAcceptsSelectReadBackProjectionE2E(t *testing.T) {
	records := []map[string]any{{
		"recordId": "r1",
		"cells":    map[string]any{"status": "跟进中"},
	}}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"updatedCount":1}`},
		{text: `{"records":[{"recordId":"r1","cells":{"status":{"id":"opt-1","name":"跟进中"}}}]}`},
	}}
	out, err := runRecordBatchCLI(t, caller, "+record-update", records)
	if err != nil || !strings.Contains(out, `"status": "verified"`) || len(caller.calls) != 2 {
		t.Fatalf("select update read-back = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageRecordDeleteDryRunAndDualFailureE2E(t *testing.T) {
	caller := &upsertByKeyCaller{dryRun: true}
	out, err := runRecordDeleteCLI(t, caller, []string{"r1"}, "--dry-run")
	if err != nil || len(caller.calls) != 0 || !strings.Contains(out, `"status": "planned"`) {
		t.Fatalf("delete dry run = output:%q err:%v calls:%#v", out, err, caller.calls)
	}

	caller = &upsertByKeyCaller{steps: []upsertByKeyStep{{err: errors.New("write failed")}, {err: errors.New("verify failed")}}}
	out, err = runRecordDeleteCLI(t, caller, []string{"r1"})
	if err == nil || out != "" {
		t.Fatalf("delete dual failure = output:%q err:%v", out, err)
	}

	out, err = runRecordDeleteCLI(t, &upsertByKeyCaller{}, []string{"", " "})
	if err == nil || out != "" {
		t.Fatalf("delete parsed empty IDs = output:%q err:%v", out, err)
	}

	out, err = runRecordDeleteCLI(t, &upsertByKeyCaller{}, recordIDFixtures(maxCompositeRecordRun+1))
	if err == nil || out != "" {
		t.Fatalf("delete excessive IDs = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageRecordPrimaryDocPreflightAndNormalizationE2E(t *testing.T) {
	t.Run("record not found is classified", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"fields":[{"fieldId":"primary","type":"primaryDoc"}]}`},
			{text: `{"records":[]}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+record-primary-doc-get", "--base-id", "base", "--table-id", "table", "--record-id", "missing")
		var typed *apperrors.Error
		if err == nil || out != "" || !errors.As(err, &typed) || typed.Reason != "RESOURCE_NOT_FOUND" {
			t.Fatalf("missing primary-doc record = output:%q err:%#v", out, err)
		}
	})

	t.Run("table without primary doc field is normalized", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"fields":[{"fieldId":"text","type":"text"}]}`},
			{text: `{"records":[{"recordId":"r1","cells":{"text":"x"}}]}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+record-primary-doc-get", "--base-id", "base", "--table-id", "table", "--record-id", "r1")
		if err != nil || !strings.Contains(out, `"status": "no_primary_doc_field"`) || !strings.Contains(out, `"exists": false`) || len(caller.calls) != 2 {
			t.Fatalf("no primary field = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("empty primary doc cell is unassociated", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"fields":[{"fieldId":"primary","type":"primaryDoc"}]}`},
			{text: `{"records":[{"recordId":"r1","cells":{}}]}`},
			{text: `{"data":{"nodeId":null}}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+record-primary-doc-get", "--base-id", "base", "--table-id", "table", "--record-id", "r1")
		if err != nil || !strings.Contains(out, `"status": "unassociated"`) || !strings.Contains(out, `"exists": false`) || len(caller.calls) != 3 || caller.calls[2].tool != "get_primary_doc" {
			t.Fatalf("unassociated primary doc = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("known helper no-record error is unassociated", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"fields":[{"fieldId":"primary","type":"primaryDoc"}]}`},
			{text: `{"records":[{"recordId":"r1","cells":{}}]}`},
			{err: errors.New("no record")},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+record-primary-doc-get", "--base-id", "base", "--table-id", "table", "--record-id", "r1")
		if err != nil || !strings.Contains(out, `"status": "unassociated"`) || !strings.Contains(out, `"exists": false`) || len(caller.calls) != 3 {
			t.Fatalf("no-record primary doc = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("associated doc is resolved through helper", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"fields":[{"fieldId":"primary","type":"primaryDoc"}]}`},
			{text: `{"records":[{"recordId":"r1","cells":{"primary":{"associated":true}}}]}`},
			{text: `{"data":{"nodeId":"node-1"}}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+record-primary-doc-get", "--base-id", "base", "--table-id", "table", "--record-id", "r1")
		if err != nil || !strings.Contains(out, `"nodeId": "node-1"`) || !strings.Contains(out, `"exists": true`) || len(caller.calls) != 3 || caller.calls[2].tool != "get_primary_doc" {
			t.Fatalf("associated primary doc = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})
}

func TestCrossPlatformCoverageRecordPrimaryDocFailureAndShapeEdgesE2E(t *testing.T) {
	cases := []struct {
		name  string
		steps []upsertByKeyStep
	}{
		{name: "fields transport", steps: []upsertByKeyStep{{err: context.Canceled}}},
		{name: "missing fields collection", steps: []upsertByKeyStep{{text: `{}`}}},
		{name: "primary field missing id", steps: []upsertByKeyStep{{text: `{"fields":[{"type":"primaryDoc"}]}`}}},
		{name: "record query error", steps: []upsertByKeyStep{{text: `{"fields":[]}`}, {err: context.DeadlineExceeded}}},
		{name: "wrong record identity", steps: []upsertByKeyStep{{text: `{"fields":[]}`}, {text: `{"records":[{"recordId":"other"}]}`}}},
		{name: "helper unknown error", steps: []upsertByKeyStep{{text: `{"fields":[{"fieldId":"p","type":"primaryDoc"}]}`}, {text: `{"records":[{"recordId":"r1"}]}`}, {err: errors.New("permission denied")}}},
		{name: "helper missing node", steps: []upsertByKeyStep{{text: `{"fields":[{"fieldId":"p","type":"primaryDoc"}]}`}, {text: `{"records":[{"recordId":"r1"}]}`}, {text: `{"data":{"message":"ok"}}`}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: tc.steps}, "+record-primary-doc-get", "--base-id", "base", "--table-id", "table", "--record-id", "r1")
			if err == nil || out != "" {
				t.Fatalf("primary doc failure = output:%q err:%v", out, err)
			}
		})
	}

	for _, value := range []any{
		map[string]any{"exists": false},
		map[string]any{"dentryUuid": ""},
		map[string]any{"status": "NO_RECORD"},
		map[string]any{"data": map[string]any{"result": map[string]any{"status": "unassociated"}}},
	} {
		if !knownPrimaryDocUnassociatedData(value) {
			t.Errorf("known unassociated shape not recognized: %#v", value)
		}
	}
	if knownPrimaryDocUnassociatedData([]any{"unrelated"}) || knownPrimaryDocUnassociatedData(nil) {
		t.Fatal("unrelated shapes must not be unassociated")
	}
	for _, value := range []any{
		map[string]any{"metadata": map[string]any{"exists": false}},
		map[string]any{"items": []any{map[string]any{"nodeId": nil}}},
	} {
		if knownPrimaryDocUnassociatedData(value) {
			t.Errorf("unrelated nested marker must not be unassociated: %#v", value)
		}
	}
	if got := primaryDocNodeID(map[string]any{"metadata": map[string]any{"nodeId": "wrong"}}); got != "" {
		t.Fatalf("unrelated nested nodeId = %q, want empty", got)
	}
	if got := primaryDocNodeID(map[string]any{"data": map[string]any{"response": map[string]any{"dentryUuid": " node-1 "}}}); got != "node-1" {
		t.Fatalf("known envelope nodeId = %q, want node-1", got)
	}
}

func TestCrossPlatformCoverageRecordDeleteRejectsMalformedPreflightRecordsE2E(t *testing.T) {
	for _, tc := range []struct {
		name    string
		records string
	}{
		{name: "missing id", records: `[{"cells":{}}]`},
		{name: "unexpected id", records: `[{"recordId":"other"}]`},
		{name: "duplicate id", records: `[{"recordId":"r1"},{"recordId":"r1"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"records":` + tc.records + `}`}}}
			out, err := runRecordDeleteCLI(t, caller, []string{"r1"})
			if err == nil || out != "" || len(caller.calls) != 1 {
				t.Fatalf("delete malformed preflight = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordBatchVerificationEdgesE2E(t *testing.T) {
	record := []map[string]any{{"recordId": "r1", "cells": map[string]any{"f": "new"}}}

	t.Run("read-back query error", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"updatedCount":1}`}, {err: errors.New("read failed")}}}
		out, err := runRecordBatchCLI(t, caller, "+record-update", record)
		if err == nil || out != "" {
			t.Fatalf("query error = output:%q err:%v", out, err)
		}
	})

	t.Run("read-back cells mismatch", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"updatedCount":1}`},
			{text: `{"records":[{"recordId":"r1","cells":{"f":"old"}}]}`},
		}}
		out, err := runRecordBatchCLI(t, caller, "+record-update", record)
		if err == nil || out != "" {
			t.Fatalf("cell mismatch = output:%q err:%v", out, err)
		}
	})

	t.Run("upsert create read error", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"createdRecords":[{"recordId":"new"}]}`},
			{err: errors.New("created read failed")},
		}}
		out, err := runRecordBatchCLI(t, caller, "+record-upsert", []map[string]any{{"cells": map[string]any{"f": "new"}}})
		if err == nil || out != "" {
			t.Fatalf("create read error = output:%q err:%v", out, err)
		}
	})

	t.Run("upsert create cells mismatch", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"createdRecords":[{"recordId":"new"}]}`},
			{text: `{"records":[{"recordId":"new","cells":{"f":"old"}}]}`},
		}}
		out, err := runRecordBatchCLI(t, caller, "+record-upsert", []map[string]any{{"cells": map[string]any{"f": "new"}}})
		if err == nil || out != "" {
			t.Fatalf("create mismatch = output:%q err:%v", out, err)
		}
	})

	t.Run("upsert update read error", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"createdRecords":[],"updatedCount":1}`},
			{err: errors.New("updated read failed")},
		}}
		out, err := runRecordBatchCLI(t, caller, "+record-upsert", record)
		if err == nil || out != "" {
			t.Fatalf("update read error = output:%q err:%v", out, err)
		}
	})

	t.Run("incomplete update read", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"updatedCount":1}`},
			{text: `{"records":[{"recordId":"r1","cells":{"f":"new"}}],"hasMore":true}`},
		}}
		out, err := runRecordBatchCLI(t, caller, "+record-update", record)
		if err == nil || out != "" {
			t.Fatalf("incomplete read = output:%q err:%v", out, err)
		}
	})
}

func TestCrossPlatformCoverageRecordBatchPureHelpers(t *testing.T) {
	if err := matchCreatedCells([]map[string]any{{"cells": map[string]any{"f": 1}}}, nil); err == nil {
		t.Fatal("created result count mismatch must fail")
	}
	if err := matchCreatedCells(
		[]map[string]any{{"cells": map[string]any{"f": 1}}},
		[]map[string]any{{"recordId": "r", "cells": map[string]any{"f": 2}}},
	); err == nil {
		t.Fatal("unmatched created cells must fail")
	}
	ids := createdRecordIDs(map[string]any{"createdIds": []any{" r1 ", "r1", map[string]any{"record_id": "r2"}}})
	if strings.Join(ids, ",") != "r1,r2" {
		t.Fatalf("created IDs = %#v", ids)
	}
	if got := stringSlice("bad"); got != nil {
		t.Fatalf("stringSlice = %#v", got)
	}
	if minInt(1, 2) != 1 || minInt(2, 1) != 1 {
		t.Fatal("minInt branch mismatch")
	}
}

func TestCrossPlatformCoverageRecordBulkPatchValidationAndSelectorsE2E(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "invalid patch", args: []string{"--all", "--patch", `{`}},
		{name: "empty patch", args: []string{"--all", "--patch", `{}`}},
		{name: "false all", args: []string{"--all=false", "--patch", `{"f":1}`}},
		{name: "invalid max", args: []string{"--all", "--patch", `{"f":1}`, "--max-matches", "0"}},
		{name: "invalid filters", args: []string{"--filters", `{`, "--patch", `{"f":1}`}},
		{name: "null filters", args: []string{"--filters", `null`, "--patch", `{"f":1}`}},
		{name: "empty filters object", args: []string{"--filters", `{}`, "--patch", `{"f":1}`}},
		{name: "empty filters operands", args: []string{"--filters", `{"operator":"and","operands":[]}`, "--patch", `{"f":1}`}},
		{name: "missing filters operator", args: []string{"--filters", `{"operands":[{"operator":"eq","operands":["fld","x"]}]}`, "--patch", `{"f":1}`}},
		{name: "blank child operator", args: []string{"--filters", `{"operator":"and","operands":[{"operator":" ","operands":["fld","x"]}]}`, "--patch", `{"f":1}`}},
		{name: "null filters operands", args: []string{"--filters", `{"operator":"and","operands":null}`, "--patch", `{"f":1}`}},
		{name: "non condition operand", args: []string{"--filters", `{"operator":"and","operands":[null]}`, "--patch", `{"f":1}`}},
		{name: "root comparison", args: []string{"--filters", `{"operator":"eq","operands":["fld","x"]}`, "--patch", `{"f":1}`}},
		{name: "blank filter field", args: []string{"--filters", `{"operator":"and","operands":[{"operator":"eq","operands":[" ","x"]}]}`, "--patch", `{"f":1}`}},
		{name: "unknown filter operator", args: []string{"--filters", `{"operator":"and","operands":[{"operator":"unknown","operands":["fld","x"]}]}`, "--patch", `{"f":1}`}},
		{name: "missing comparison value", args: []string{"--filters", `{"operator":"and","operands":[{"operator":"eq","operands":["fld"]}]}`, "--patch", `{"f":1}`}},
		{name: "blank query", args: []string{"--query", ` `, "--patch", `{"f":1}`}},
		{name: "empty record IDs", args: []string{"--record-ids", ` `, "--patch", `{"f":1}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"--base-id", "base", "--table-id", "table"}
			args = append(args, tc.args...)
			args = append(args, "--yes")
			caller := &upsertByKeyCaller{}
			out, err := runAITableCompositeCLI(t, caller, "+record-bulk-patch", args...)
			if err == nil || out != "" || len(caller.calls) != 0 {
				t.Fatalf("validation = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}

	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"records":[]}`}}}
	out, err := runAITableCompositeCLI(t, caller, "+record-bulk-patch",
		"--base-id", "base", "--table-id", "table", "--filters", `{"operator":"and","operands":[{"operator":"or","operands":[{"operator":"eq","operands":["fld","x"]}]}]}`, "--query", "text",
		"--record-ids", "r2,r1", "--view-id", "view", "--patch", `{"f":1}`, "--yes")
	if err != nil || !strings.Contains(out, `"matchedCount": 0`) {
		t.Fatalf("selectors = output:%q err:%v", out, err)
	}
	for _, key := range []string{"filters", "keyword", "recordIds", "viewId"} {
		if _, ok := caller.calls[0].args[key]; !ok {
			t.Fatalf("query args missing %s: %#v", key, caller.calls[0].args)
		}
	}

	caller = &upsertByKeyCaller{dryRun: true, steps: []upsertByKeyStep{{text: `{"records":[]}`}}}
	out, err = runAITableCompositeCLI(t, caller, "+record-bulk-patch",
		"--base-id", "base", "--table-id", "table", "--query", "none", "--patch", `{"f":1}`, "--dry-run", "--yes")
	if err != nil || !strings.Contains(out, `"status": "planned"`) {
		t.Fatalf("empty dry run = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageRecordBulkPatchQueryFailuresE2E(t *testing.T) {
	cases := []struct {
		name  string
		steps []upsertByKeyStep
	}{
		{name: "query error", steps: []upsertByKeyStep{{err: errors.New("query failed")}}},
		{name: "missing records", steps: []upsertByKeyStep{{text: `{}`}}},
		{name: "more without cursor", steps: []upsertByKeyStep{{text: `{"records":[],"hasMore":true}`}}},
		{name: "cursor cycle", steps: []upsertByKeyStep{
			{text: `{"records":[],"hasMore":true,"nextCursor":"same"}`},
			{text: `{"records":[],"hasMore":true,"nextCursor":"same"}`},
		}},
		{name: "matched record missing id", steps: []upsertByKeyStep{{text: `{"records":[{"cells":{"f":1}}]}`}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: tc.steps}, "+record-bulk-patch",
				"--base-id", "base", "--table-id", "table", "--all", "--patch", `{"f":1}`, "--yes")
			if err == nil || out != "" {
				t.Fatalf("query failure = output:%q err:%v", out, err)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordQueryPageSafetyBoundE2E(t *testing.T) {
	caller := &upsertByKeyCaller{callFn: func(index int, _, tool string, _ map[string]any) (string, error) {
		if tool != "query_records" {
			return "", fmt.Errorf("unexpected tool %s", tool)
		}
		return fmt.Sprintf(`{"records":[],"hasMore":true,"nextCursor":"cursor-%d"}`, index), nil
	}}
	out, err := runAITableCompositeCLI(t, caller, "+record-bulk-patch",
		"--base-id", "base", "--table-id", "table", "--all", "--patch", `{"f":1}`, "--yes")
	if err == nil || out != "" || len(caller.calls) != 10000 {
		t.Fatalf("page safety = output:%q err:%v calls:%d", out, err, len(caller.calls))
	}
}

func TestCrossPlatformCoverageRecordUpsertInputAndQueryFailuresE2E(t *testing.T) {
	baseArgs := []string{"--base-id", "base", "--table-id", "table", "--key-field-id", "key", "--cells", `{"value":1}`, "--yes"}
	cases := []struct {
		name  string
		key   []string
		cells string
		steps []upsertByKeyStep
	}{
		{name: "empty string key", key: []string{"--key-value", " "}},
		{name: "invalid JSON key", key: []string{"--key-value-json", `{`}},
		{name: "null JSON key", key: []string{"--key-value-json", `null`}},
		{name: "object JSON key", key: []string{"--key-value-json", `{}`}},
		{name: "invalid cells", key: []string{"--key-value", "v"}, cells: `{`},
		{name: "conflicting key", key: []string{"--key-value", "v"}, cells: `{"key":"other"}`},
		{name: "query error", key: []string{"--key-value", "v"}, steps: []upsertByKeyStep{{err: errors.New("query failed")}}},
		{name: "missing records", key: []string{"--key-value", "v"}, steps: []upsertByKeyStep{{text: `{}`}}},
		{name: "incomplete unique query", key: []string{"--key-value", "v"}, steps: []upsertByKeyStep{{text: `{"records":[],"hasMore":true}`}}},
		{name: "record missing id", key: []string{"--key-value", "v"}, steps: []upsertByKeyStep{{text: `{"records":[{"cells":{"key":"v"}}]}`}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{}, baseArgs...)
			args = append(args, tc.key...)
			if tc.cells != "" {
				for index := range args {
					if args[index] == "--cells" {
						args[index+1] = tc.cells
					}
				}
			}
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: tc.steps}, "+record-upsert-by-key", args...)
			if err == nil || out != "" {
				t.Fatalf("upsert failure = output:%q err:%v", out, err)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordUpsertJSONScalarSuccessE2E(t *testing.T) {
	caller := &upsertByKeyCaller{dryRun: true, steps: []upsertByKeyStep{{text: `{"records":[]}`}}}
	out, err := runAITableCompositeCLI(t, caller, "+record-upsert-by-key",
		"--base-id", "base", "--table-id", "table", "--key-field-id", "key", "--key-value-json", `true`,
		"--cells", `{"value":1}`, "--dry-run", "--yes")
	if err != nil || !strings.Contains(out, `"keyValue": true`) {
		t.Fatalf("JSON scalar = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageRecordUpsertRejectsTrailingJSONBeforeMCPE2E(t *testing.T) {
	for name, value := range map[string]string{
		"trailing garbage":    `1 garbage`,
		"second JSON value":   `1 2`,
		"second string value": `true "extra"`,
	} {
		t.Run(name, func(t *testing.T) {
			caller := &upsertByKeyCaller{}
			out, err := runAITableCompositeCLI(t, caller, "+record-upsert-by-key",
				"--base-id", "base", "--table-id", "table", "--key-field-id", "key", "--key-value-json", value,
				"--cells", `{"value":1}`, "--yes")
			if err == nil || out != "" || len(caller.calls) != 0 {
				t.Fatalf("trailing JSON = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordShapeHelpers(t *testing.T) {
	if records, ok := findRecords(nil); ok || records != nil {
		t.Fatalf("nil records = %#v, %v", records, ok)
	}
	if _, ok := findRecords(map[string]any{"records": "bad"}); ok {
		t.Fatal("non-array records must fail")
	}
	if _, ok := findRecords(map[string]any{"records": []any{"bad"}}); ok {
		t.Fatal("non-object record must fail")
	}
	if records, ok := findRecords(map[string]any{"result": map[string]any{"records": []any{map[string]any{"id": "r"}}}}); !ok || recordID(records[0]) != "r" {
		t.Fatalf("nested records = %#v, %v", records, ok)
	}
	if responseHasMore(nil) || !responseHasMore(map[string]any{"cursor": "next"}) || !responseHasMore(map[string]any{"pagination": map[string]any{"hasMore": true}}) {
		t.Fatal("responseHasMore shape mismatch")
	}
	if err := verifyRecordCells(map[string]any{"recordId": "r"}, map[string]any{"f": 1}); err == nil {
		t.Fatal("missing cells must fail")
	}
	selectRecord := map[string]any{"recordId": "r", "cells": map[string]any{
		"singleByName": map[string]any{"id": "opt-1", "name": "跟进中"},
		"singleByID":   map[string]any{"id": "opt-2", "name": "已完成"},
		"multi": []any{
			map[string]any{"id": "opt-a", "name": "A"},
			map[string]any{"id": "opt-b", "name": "B"},
		},
	}}
	if err := verifyRecordCells(selectRecord, map[string]any{
		"singleByName": "跟进中",
		"singleByID":   "opt-2",
		"multi":        []any{"B", "A"},
	}); err != nil {
		t.Fatalf("select semantic read-back must match: %v", err)
	}
	if err := verifyRecordCells(selectRecord, map[string]any{"singleByName": "未开始"}); err == nil {
		t.Fatal("different select option must fail")
	}
	if responseCursor(nil) != "" || responseCursor(map[string]any{"data": map[string]any{"next_cursor": " next "}}) != "next" ||
		responseCursor(map[string]any{"result": map[string]any{"cursor": " legacy "}}) != "legacy" {
		t.Fatal("responseCursor shape mismatch")
	}
}
