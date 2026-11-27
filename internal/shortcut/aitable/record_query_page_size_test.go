// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

// scanRecordQueryResponse models both a current 100-row service and a service
// returning shorter pages, without relaxing the legacy exact-ID/write fixture.
func scanRecordQueryResponse(t *testing.T, records []map[string]any, args map[string]any, serviceCap int) string {
	t.Helper()
	limit, ok := args["limit"].(int)
	if !ok || limit < 1 || limit > 100 {
		t.Fatalf("scan request limit = %#v, want 1..100", args["limit"])
	}
	offset := 0
	if cursor, _ := args["cursor"].(string); cursor != "" {
		if _, err := fmt.Sscanf(cursor, "offset-%d", &offset); err != nil {
			t.Fatal(err)
		}
	}
	end := minInt(offset+minInt(limit, serviceCap), len(records))
	data := map[string]any{
		"records": records[offset:end], "totalCount": len(records), "hasMore": end < len(records),
	}
	if end < len(records) {
		data["nextCursor"] = fmt.Sprintf("offset-%d", end)
	}
	raw, err := json.Marshal(map[string]any{"status": "success", "data": data})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func assertRecordQueryRows(t *testing.T, result map[string]any, expected []map[string]any) {
	t.Helper()
	actual, found := findRecords(result)
	if !found || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("query changed record IDs, order or values: got %#v, want %#v", actual, expected)
	}
}

func TestCrossPlatformCoverageRecordQueryScanPageBudgetAndContinuation(t *testing.T) {
	records := updateFixtureRecords(0, 120, "中文-🙂")
	for index, record := range records {
		cells := record["cells"].(map[string]any)
		if index%2 == 0 {
			cells["fldOptional"] = nil
		}
		cells["fldLinks"] = []any{map[string]any{"linkedRecordId": fmt.Sprintf("linked-%d", index)}}
		cells["fldNumber"] = float64(index)
	}
	for _, tc := range []struct {
		name, cursor string
		limit        int
		serviceCap   int
		wantLimits   []int
		start, end   int
		wantCursor   string
	}{
		{"small", "", 1, 100, []int{1}, 0, 1, "offset-1"},
		{"twenty", "", 20, 100, []int{20}, 0, 20, "offset-20"},
		{"over_old_boundary", "", 21, 100, []int{21}, 0, 21, "offset-21"},
		{"hundred", "", 100, 100, []int{100}, 0, 100, "offset-100"},
		{"continuation", "offset-100", 100, 100, []int{100}, 100, 120, ""},
		{"short_service_pages", "", 100, 20, []int{100, 80, 60, 40, 20}, 0, 100, "offset-100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{}
			caller.callFn = func(_ int, product, tool string, args map[string]any) (string, error) {
				if product != serverMain || tool != "query_records" || args["recordIds"] != nil {
					t.Fatalf("unexpected scan target: %s/%s %#v", product, tool, args)
				}
				return scanRecordQueryResponse(t, records, args, tc.serviceCap), nil
			}
			extra := []string{}
			if tc.cursor != "" {
				extra = append(extra, "--cursor", tc.cursor)
			}
			result, err := runRecordQueryShortcutCLI(t, caller, tc.limit, extra...)
			if err != nil {
				t.Fatal(err)
			}
			assertRecordQueryRows(t, result, records[tc.start:tc.end])
			if responseCursor(result) != tc.wantCursor || responseHasMore(result) != (tc.wantCursor != "") {
				t.Fatalf("incorrect continuation: %#v", result)
			}
			limits := []int{}
			for _, call := range caller.calls {
				limits = append(limits, call.args["limit"].(int))
			}
			if !reflect.DeepEqual(limits, tc.wantLimits) {
				t.Fatalf("request limits = %v, want %v", limits, tc.wantLimits)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordQueryAllUsesScanPagesAndPreservesBound(t *testing.T) {
	for _, tc := range []struct {
		limit, cap, max int
		wantCalls       int
		fail            bool
	}{
		{100, 100, 120, 2, false},
		{25, 100, 120, 5, false},
		{100, 20, 120, 6, false},
		{100, 100, 100, 2, true},
	} {
		t.Run(fmt.Sprintf("limit_%d_cap_%d_max_%d", tc.limit, tc.cap, tc.max), func(t *testing.T) {
			records := updateFixtureRecords(0, 120, "完成")
			caller := &upsertByKeyCaller{}
			caller.callFn = func(_ int, _, tool string, args map[string]any) (string, error) {
				if tool != "query_records" || args["limit"] != tc.limit {
					t.Fatalf("all-query changed page size or target: %s %#v", tool, args)
				}
				return scanRecordQueryResponse(t, records, args, tc.cap), nil
			}
			result, err := runRecordQueryShortcutCLI(t, caller, tc.limit, "--all", "--max-records", fmt.Sprint(tc.max))
			if len(caller.calls) != tc.wantCalls || (err != nil) != tc.fail {
				t.Fatalf("calls=%d, result=%#v, err=%v", len(caller.calls), result, err)
			}
			if tc.fail {
				if result != nil || !strings.Contains(err.Error(), "allowed 100 records") {
					t.Fatalf("overflow emitted a complete result: %#v %v", result, err)
				}
				return
			}
			assertRecordQueryRows(t, result, records)
		})
	}
}

func TestCrossPlatformCoverageRecordQueryExactIDsRetainTwentyRowChunks(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(fmt.Sprintf("all_%t", all), func(t *testing.T) {
			records := updateFixtureRecords(0, 100, "完成")
			byID := map[string]map[string]any{}
			for _, record := range records {
				byID[recordID(record)] = record
			}
			caller := &upsertByKeyCaller{}
			caller.callFn = func(_ int, _, tool string, args map[string]any) (string, error) {
				ids := args["recordIds"].([]string)
				if tool != "query_records" || len(ids) != 20 || args["limit"] != 20 || args["cursor"] != nil {
					t.Fatalf("exact-ID compatibility policy changed: %s %#v", tool, args)
				}
				selected := []map[string]any{}
				for _, id := range ids {
					selected = append(selected, byID[id])
				}
				return pagedRecordQueryResponse(t, selected, args), nil
			}
			extra := []string{"--record-ids", strings.Join(recordIDs(records), ",")}
			if all {
				extra = append(extra, "--all")
			}
			result, err := runRecordQueryShortcutCLI(t, caller, 100, extra...)
			if err != nil || len(caller.calls) != 5 {
				t.Fatalf("exact-ID query: result=%#v err=%v calls=%d", result, err, len(caller.calls))
			}
			assertRecordQueryRows(t, result, records)
		})
	}
}

func TestCrossPlatformCoverageRecordQuerySharedHelpersRetainTwentyRowPages(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(fmt.Sprintf("all_%t", all), func(t *testing.T) {
			records := updateFixtureRecords(0, 100, "完成")
			caller := &upsertByKeyCaller{}
			caller.callFn = func(_ int, _, _ string, args map[string]any) (string, error) {
				if args["limit"] != 20 {
					t.Fatalf("shared composite policy changed: %#v", args)
				}
				return pagedRecordQueryResponse(t, records, args), nil
			}
			helpers.InitDepsForTest(t, caller)
			rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: "query"}, RecordQuery)
			var actual []map[string]any
			var err error
			if all {
				actual, err = queryAllRecords(rt, map[string]any{}, 100)
			} else {
				var window recordQueryWindow
				window, err = queryRecordWindow(rt, map[string]any{}, 100)
				actual = window.Records
			}
			if err != nil || !reflect.DeepEqual(actual, records) || len(caller.calls) != 5 {
				t.Fatalf("shared helper: rows=%d calls=%d err=%v", len(actual), len(caller.calls), err)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordQueryLargePageFailuresEmitNoPartialSuccess(t *testing.T) {
	first := updateFixtureRecords(0, 100, "完成")
	raw, _ := json.Marshal(map[string]any{"data": map[string]any{"records": first, "nextCursor": "next"}})
	for name, response := range map[string]string{
		"missing_records":  `{"status":"success","data":{"nextCursor":"later"}}`,
		"missing_cursor":   `{"data":{"records":[],"hasMore":true}}`,
		"cursor_cycle":     `{"data":{"records":[],"nextCursor":"next"}}`,
		"duplicate_id":     "",
		"snapshot_changed": `{"status":"error","error":{"code":"CURSOR_SNAPSHOT_CHANGED","message":"changed"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if name == "duplicate_id" {
				duplicate, _ := json.Marshal(map[string]any{"data": map[string]any{"records": first[:1]}})
				response = string(duplicate)
			}
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: string(raw)}, {text: response}}}
			result, err := runRecordQueryShortcutCLI(t, caller, 100, "--all")
			if err == nil || result != nil || len(caller.calls) != 2 {
				t.Fatalf("unsafe partial success: result=%#v err=%v calls=%d", result, err, len(caller.calls))
			}
		})
	}
}
