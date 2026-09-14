// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageRecordBulkPatchPaginatesChunksAndVerifiesE2E(t *testing.T) {
	records := updateFixtureRecords(0, 101, "旧")
	firstPage := map[string]any{"records": records[:100], "hasMore": true, "nextCursor": "next"}
	secondPage := map[string]any{"records": records[100:], "hasMore": false}
	patchedFirst := updateFixtureRecords(0, 100, "新")
	patchedSecond := updateFixtureRecords(100, 1, "新")
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: mustJSONText(t, firstPage)},
		{text: mustJSONText(t, secondPage)},
		{text: `{"updatedCount":100}`},
		{text: recordListJSON(t, patchedFirst)},
		{text: `{"updatedCount":1}`},
		{text: recordListJSON(t, patchedSecond)},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+record-bulk-patch",
		"--base-id", "base", "--table-id", "table", "--all", "--patch", `{"fldStatus":"新"}`, "--yes")
	if err != nil {
		t.Fatalf("bulk patch error = %v", err)
	}
	for _, want := range []string{`"verifiedCount": 101`, `"processedCount": 101`, `"batchCount": 2`} {
		if !strings.Contains(out, want) {
			t.Fatalf("bulk patch output missing %s: %s", want, out)
		}
	}
	if len(caller.calls) != 6 || caller.calls[1].args["cursor"] != "next" || caller.calls[2].tool != "update_records" {
		t.Fatalf("bulk patch calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageRecordBulkPatchPaginatesNestedCursorShapeE2E(t *testing.T) {
	records := updateFixtureRecords(0, 2, "旧")
	patched := updateFixtureRecords(0, 2, "新")
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: mustJSONText(t, map[string]any{"data": map[string]any{"records": records[:1], "hasMore": true, "cursor": "legacy-next"}})},
		{text: mustJSONText(t, map[string]any{"data": map[string]any{"records": records[1:], "hasMore": false}})},
		{text: `{"updatedCount":2}`},
		{text: recordListJSON(t, patched)},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+record-bulk-patch",
		"--base-id", "base", "--table-id", "table", "--all", "--patch", `{"fldStatus":"新"}`, "--yes")
	if err != nil || !strings.Contains(out, `"verifiedCount": 2`) {
		t.Fatalf("nested cursor bulk patch = output:%q err:%v", out, err)
	}
	if len(caller.calls) != 4 || caller.calls[1].args["cursor"] != "legacy-next" || caller.calls[2].tool != "update_records" {
		t.Fatalf("nested cursor calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageRecordBulkPatchSelectionBoundsAndEmptyE2E(t *testing.T) {
	t.Run("explicit no matches is success", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"records":[]}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+record-bulk-patch",
			"--base-id", "base", "--table-id", "table", "--query", "none", "--patch", `{"fld":"x"}`, "--yes")
		if err != nil || len(caller.calls) != 1 || !strings.Contains(out, `"status": "verified_no_matches"`) {
			t.Fatalf("empty patch = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("missing selector stops before reads", func(t *testing.T) {
		caller := &upsertByKeyCaller{}
		out, err := runAITableCompositeCLI(t, caller, "+record-bulk-patch",
			"--base-id", "base", "--table-id", "table", "--patch", `{"fld":"x"}`, "--yes")
		if err == nil || out != "" || len(caller.calls) != 0 {
			t.Fatalf("missing selector = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("max matches stops before writes", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: recordListJSON(t, updateFixtureRecords(0, 2, "旧"))}}}
		out, err := runAITableCompositeCLI(t, caller, "+record-bulk-patch",
			"--base-id", "base", "--table-id", "table", "--all", "--patch", `{"fld":"x"}`, "--max-matches", "1", "--yes")
		if err == nil || out != "" || len(caller.calls) != 1 {
			t.Fatalf("bounded patch = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})
}

func mustJSONText(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func sourceFieldFixture() map[string]any {
	return map[string]any{"fieldId": "sf1", "fieldName": "状态", "type": "text"}
}

func sourcePrimaryDocFixture() map[string]any {
	return map[string]any{"fieldId": "sp0", "fieldName": "主文档", "type": "primaryDoc"}
}

func targetFieldFixture() map[string]any {
	return map[string]any{"fieldId": "tf1", "fieldName": "状态", "type": "text"}
}

func targetPrimaryDocFixture() map[string]any {
	return map[string]any{"fieldId": "tp0", "fieldName": "主文档", "type": "primaryDoc"}
}

func TestCrossPlatformCoverageTableCopyStructureAndRecordsE2E(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: mustJSONText(t, map[string]any{"fields": []any{sourcePrimaryDocFixture(), sourceFieldFixture()}})},
		{text: `{"records":[{"recordId":"sr1","cells":{"sf1":"完成"}}]}`},
		{text: `{"data":{"tableId":"target-table"}}`},
		{text: mustJSONText(t, map[string]any{"fields": []any{targetPrimaryDocFixture(), targetFieldFixture()}})},
		{text: `{"data":{"newRecordIds":["tr1"]}}`},
		{text: `{"records":[{"recordId":"tr1","cells":{"tf1":"完成"}}]}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+table-copy",
		"--source-base-id", "source-base", "--source-table-id", "source-table",
		"--target-base-id", "target-base", "--new-name", "任务副本", "--include-records", "--yes")
	if err != nil {
		t.Fatalf("table copy error = %v", err)
	}
	for _, want := range []string{`"targetTableId": "target-table"`, `"fieldCount": 2`, `"recordCount": 1`, `"status": "verified"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("table copy output missing %s: %s", want, out)
		}
	}
	if len(caller.calls) != 6 || caller.calls[4].tool != "create_records" {
		t.Fatalf("table copy calls = %#v", caller.calls)
	}
	written := caller.calls[4].args["records"].([]any)[0].(map[string]any)["cells"].(map[string]any)
	if written["tf1"] != "完成" || written["sf1"] != nil {
		t.Fatalf("mapped record cells = %#v", written)
	}
}

func TestCrossPlatformCoverageTableCopyRetriesEventuallyConsistentRecordReadbackE2E(t *testing.T) {
	waits := make([]time.Duration, 0)
	testseam.Swap(t, &tableCopyRecordReadbackWait, func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	})
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: mustJSONText(t, map[string]any{"fields": []any{sourcePrimaryDocFixture(), sourceFieldFixture()}})},
		{text: `{"records":[{"recordId":"sr1","cells":{"sf1":"完成"}}]}`},
		{text: `{"data":{"tableId":"target-table"}}`},
		{text: mustJSONText(t, map[string]any{"fields": []any{targetPrimaryDocFixture(), targetFieldFixture()}})},
		{text: `{"data":{"newRecordIds":["tr1"]}}`},
		{text: `{"records":[]}`},
		{text: `{"records":[{"recordId":"tr1","cells":{"tf1":"完成"}}]}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+table-copy",
		"--source-base-id", "source-base", "--source-table-id", "source-table",
		"--target-base-id", "target-base", "--new-name", "任务副本", "--include-records", "--yes")
	if err != nil || !strings.Contains(out, `"status": "verified"`) || !strings.Contains(out, `"recordCount": 1`) {
		t.Fatalf("eventual record read-back = output:%q err:%v", out, err)
	}
	if len(waits) != 1 || waits[0] != tableCopyRecordReadbackDelays[0] {
		t.Fatalf("read-back waits = %v", waits)
	}
	createCalls := 0
	for _, call := range caller.calls {
		if call.tool == "create_records" {
			createCalls++
		}
	}
	if createCalls != 1 {
		t.Fatalf("create_records calls = %d, want 1", createCalls)
	}
}

func TestCrossPlatformCoverageTableCopyReadbackWaitFailureStopsSafelyE2E(t *testing.T) {
	waitErr := errors.New("read-back wait cancelled")
	testseam.Swap(t, &tableCopyRecordReadbackWait, func(context.Context, time.Duration) error {
		return waitErr
	})
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: mustJSONText(t, map[string]any{"fields": []any{sourceFieldFixture()}})},
		{text: `{"records":[{"recordId":"sr1","cells":{"sf1":"完成"}}]}`},
		{text: `{"data":{"tableId":"target-table"}}`},
		{text: mustJSONText(t, map[string]any{"fields": []any{targetFieldFixture()}})},
		{text: `{"data":{"newRecordIds":["tr1"]}}`},
		{text: `{"records":[]}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+table-copy",
		"--source-base-id", "source-base", "--source-table-id", "source-table",
		"--target-base-id", "target-base", "--new-name", "任务副本", "--include-records", "--yes")
	if err == nil || out != "" || !errors.Is(err, waitErr) {
		t.Fatalf("read-back wait failure = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageTableCopyReadbackExhaustionPreservesKnownCreatedIDsE2E(t *testing.T) {
	waits := make([]time.Duration, 0)
	testseam.Swap(t, &tableCopyRecordReadbackWait, func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	})
	steps := []upsertByKeyStep{
		{text: mustJSONText(t, map[string]any{"fields": []any{sourceFieldFixture()}})},
		{text: `{"records":[{"recordId":"sr1","cells":{"sf1":"完成"}}]}`},
		{text: `{"data":{"tableId":"target-table"}}`},
		{text: mustJSONText(t, map[string]any{"fields": []any{targetFieldFixture()}})},
		{text: `{"data":{"newRecordIds":["tr1"]}}`},
	}
	for range len(tableCopyRecordReadbackDelays) + 1 {
		steps = append(steps, upsertByKeyStep{text: `{"records":[]}`})
	}
	caller := &upsertByKeyCaller{steps: steps}
	out, err := runAITableCompositeCLI(t, caller, "+table-copy",
		"--source-base-id", "source-base", "--source-table-id", "source-table",
		"--target-base-id", "target-base", "--new-name", "任务副本", "--include-records", "--yes")
	if err == nil || out != "" {
		t.Fatalf("exhausted record read-back = output:%q err:%v", out, err)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "aitable_composite_partial_success" || typed.Retryable {
		t.Fatalf("exhausted record read-back error = %#v", err)
	}
	result, ok := typed.Details["result"].(compositeResult)
	if !ok {
		t.Fatalf("composite result details = %#v", typed.Details)
	}
	checkpointIDs, _ := result.Checkpoint["createdRecordIds"].([]string)
	if len(checkpointIDs) != 1 || checkpointIDs[0] != "tr1" || !strings.Contains(result.Checkpoint["nextStep"].(string), "do not rerun create_records") {
		t.Fatalf("safe recovery checkpoint = %#v", result.Checkpoint)
	}
	knownIDs, _ := result.KnownEffects[len(result.KnownEffects)-1]["recordIds"].([]string)
	if len(knownIDs) != 1 || knownIDs[0] != "tr1" {
		t.Fatalf("known created IDs = %#v", result.KnownEffects)
	}
	if len(waits) != len(tableCopyRecordReadbackDelays) {
		t.Fatalf("read-back waits = %v", waits)
	}
	createCalls := 0
	for _, call := range caller.calls {
		if call.tool == "create_records" {
			createCalls++
		}
	}
	if createCalls != 1 {
		t.Fatalf("create_records calls = %d, want 1", createCalls)
	}
}

func TestCrossPlatformCoverageTableCopyReadbackWaitHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tableCopyRecordReadbackWait(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait error = %v", err)
	}
	if err := tableCopyRecordReadbackWait(context.Background(), 0); err != nil {
		t.Fatalf("zero wait error = %v", err)
	}
}

func TestCrossPlatformCoverageTableCopyUnknownResponsesAreNotSuccessE2E(t *testing.T) {
	t.Run("missing target table id", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: mustJSONText(t, map[string]any{"fields": []any{sourceFieldFixture()}})},
			{text: `{}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+table-copy",
			"--source-base-id", "b1", "--source-table-id", "t1", "--target-base-id", "b2", "--new-name", "copy", "--yes")
		if err == nil || out != "" {
			t.Fatalf("missing target id = output:%q err:%v", out, err)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "aitable_composite_unknown" || typed.Retryable {
			t.Fatalf("missing target id error = %#v", err)
		}
	})

	t.Run("created records without ids", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: mustJSONText(t, map[string]any{"fields": []any{sourceFieldFixture()}})},
			{text: `{"records":[{"recordId":"sr1","cells":{"sf1":"完成"}}]}`},
			{text: `{"tableId":"target"}`},
			{text: mustJSONText(t, map[string]any{"fields": []any{targetFieldFixture()}})},
			{text: `{"data":{"newRecordIds":[]}}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+table-copy",
			"--source-base-id", "b1", "--source-table-id", "t1", "--target-base-id", "b2", "--new-name", "copy", "--include-records", "--yes")
		if err == nil || out != "" {
			t.Fatalf("missing record ids = output:%q err:%v", out, err)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "aitable_composite_partial_success" {
			t.Fatalf("missing record ids error = %#v", err)
		}
	})
}
