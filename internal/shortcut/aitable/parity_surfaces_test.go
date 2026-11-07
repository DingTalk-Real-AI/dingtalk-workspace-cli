// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"reflect"
	"strings"
	"testing"
)

func parityStep(value any) upsertByKeyStep {
	b, _ := json.Marshal(value)
	return upsertByKeyStep{text: string(b)}
}
func TestCrossPlatformCoverageParityAppWriteReadbackAndWrongIdentity(t *testing.T) {
	for _, bad := range []bool{false, true} {
		name := "new"
		if bad {
			name = "unchanged"
		}
		c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"pageId": "p"}), parityStep(map[string]any{"pageId": "p", "pageName": name})}}
		out, err := runAITableCompositeCLI(t, c, "+app-page-create", "--base-id", "b", "--name", "new", "--yes")
		if (err != nil) != bad || len(c.calls) != 2 {
			t.Fatal(out, err, c.calls)
		}
		if bad && out != "" {
			t.Fatal("published success after mismatched readback", out)
		}
	}
	for _, cmd := range []string{"+app-page-create", "+app-block-create"} {
		c := &upsertByKeyCaller{}
		args := []string{"--base-id", "b", "--name", "new"}
		if cmd == "+app-block-create" {
			args = append(args, "--page-id", "p", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"x":0,"y":0,"w":10,"h":4}`)
		}
		_, err := runAITableCompositeCLI(t, c, cmd, args...)
		if err == nil || len(c.calls) != 0 {
			t.Fatal("unconfirmed write", cmd, err, c.calls)
		}
	}
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"widgets": []any{}, "pageId": "other"})}}
	if _, err := runAITableCompositeCLI(t, c, "+app-block-list", "--base-id", "b", "--page-id", "p"); err == nil {
		t.Fatal("accepted wrong parent")
	}
	c = &upsertByKeyCaller{}
	if _, err := runAITableCompositeCLI(t, c, "+app-block-update", "--base-id", "b", "--page-id", "p", "--widget-id", "w", "--yes"); err == nil || len(c.calls) > 0 {
		t.Fatal("empty update", err)
	}
}
func TestCrossPlatformCoverageParityAppDeleteRequiresIndependentAbsence(t *testing.T) {
	for _, present := range []bool{false, true} {
		list := []any{}
		if present {
			list = append(list, map[string]any{"pageId": "p"})
		}
		c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"deletedPageId": "p"}), parityStep(map[string]any{"pages": list})}}
		out, err := runAITableCompositeCLI(t, c, "+app-page-delete", "--base-id", "b", "--page-id", "p", "--yes")
		if (err != nil) != present || len(c.calls) != 2 {
			t.Fatal(out, err, c.calls)
		}
	}
}
func TestCrossPlatformCoverageParityStatsMustContainEveryRequestedBusinessValue(t *testing.T) {
	stats := []any{map[string]any{"fieldId": "f", "statsType": "SUM"}}
	valid := map[string]any{"results": []any{map[string]any{"results": []any{map[string]any{"fieldId": "f", "statsType": "SUM", "value": "3.00"}}}}}
	if err := validateParityStats(valid, stats, "scalar"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{}`, `{"results":[]}`, `{"results":[{}]}`, `{"results":[{"results":[{"fieldId":"other","statsType":"SUM","value":"3"}]}]}`, `{"results":[{"results":[{"fieldId":"f","statsType":"SUM"}]}]}`} {
		var m map[string]any
		json.Unmarshal([]byte(raw), &m)
		if err := validateParityStats(m, stats, "scalar"); err == nil {
			t.Fatal("accepted metadata without requested value", raw)
		}
	}
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(valid)}}
	out, err := runAITableCompositeCLI(t, c, "+data-query", "--base-id", "b", "--table-id", "t", "--dsl", `{"stats":[{"fieldId":"f","statsType":"SUM"}]}`)
	if err != nil || !strings.Contains(out, "3.00") {
		t.Fatal(out, err)
	}
	c = &upsertByKeyCaller{}
	_, err = runAITableCompositeCLI(t, c, "+data-query", "--base-id", "b", "--table-id", "t", "--dsl", `{"stats":[{"fieldId":"f","statsType":"SUM"}],"limit":5}`)
	if err == nil || len(c.calls) > 0 {
		t.Fatal("silently allowed partial aggregation")
	}
}
func TestCrossPlatformCoverageParityWorkflowFullListFiltersAfterPaging(t *testing.T) {
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"workflows": []any{map[string]any{"workflowId": "a", "status": "RUNNING"}}, "hasMore": true}), parityStep(map[string]any{"workflows": []any{map[string]any{"workflowId": "b", "status": "STOP"}}, "hasMore": false})}}
	out, err := runAITableCompositeCLI(t, c, "+workflow-list", "--base-id", "b", "--all", "--limit", "1", "--status", "disabled")
	if err != nil || !strings.Contains(out, `"workflowId": "b"`) || strings.Contains(out, `"workflowId": "a"`) || len(c.calls) != 2 || c.calls[1].args["offset"] != 1 {
		t.Fatal(out, err, c.calls)
	}
	c = &upsertByKeyCaller{}
	_, err = runAITableCompositeCLI(t, c, "+workflow-list", "--base-id", "b", "--status", "enabled")
	if err == nil || len(c.calls) != 0 {
		t.Fatal("filtered one page")
	}
}
func TestCrossPlatformCoverageParityExactAttachmentSelectionRetainsSameNamedSibling(t *testing.T) {
	existing := []map[string]any{{"name": "same.txt", "resourceId": "a", "fileToken": "ta"}, {"name": "same.txt", "resourceId": "b", "fileToken": "tb"}}
	p, e := planAttachmentRemovalIDs(existing, []string{"a"})
	if e != nil || p.removed != 1 || len(p.remaining) != 1 || p.remaining[0]["resourceId"] != "b" {
		t.Fatal(p, e)
	}
	if e = verifyAttachmentRemoval(existing[1:], p, ""); e != nil {
		t.Fatal(e)
	}

	if _, e := planAttachmentRemovalIDs(existing, []string{"a", "missing"}); e == nil {
		t.Fatal("partial selector silently accepted")
	}
}
func TestCrossPlatformCoverageParityFullReadRejectsDuplicateOrMissingIDs(t *testing.T) {
	for _, tc := range []struct {
		steps []upsertByKeyStep
		ids   string
	}{
		{[]upsertByKeyStep{parityStep(map[string]any{"records": []any{map[string]any{"recordId": "a"}}, "hasMore": true, "nextCursor": "n"}), parityStep(map[string]any{"records": []any{map[string]any{"recordId": "a"}}, "hasMore": false})}, ""},
		{[]upsertByKeyStep{parityStep(map[string]any{"records": []any{map[string]any{"recordId": "a"}}, "hasMore": false})}, "a,b"},
	} {
		c := &upsertByKeyCaller{steps: tc.steps}
		args := []string{"--base-id", "b", "--table-id", "t", "--all"}
		if tc.ids != "" {
			args = append(args, "--record-ids", tc.ids)
		}
		out, err := runAITableCompositeCLI(t, c, "+record-query", args...)
		if err == nil || out != "" {
			t.Fatal("false complete", out, err)
		}
	}
}
func TestCrossPlatformCoverageParityFormGetCannotChooseFirstForm(t *testing.T) {
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"forms": []any{map[string]any{"viewId": "wrong", "name": "other"}}})}}
	out, err := runAITableCompositeCLI(t, c, "+form-get", "--base-id", "b", "--table-id", "t", "--view-id", "wanted")
	if err == nil || out != "" || len(c.calls) != 1 {
		t.Fatal(out, err, c.calls)
	}
}

func TestCrossPlatformCoverageParityAttachmentDownloadIdentity(t *testing.T) {
	items := []map[string]any{{"resourceId": "a", "filename": "same"}, {"resourceId": "b", "filename": "same"}}
	item, err := exactAttachmentForDownload(items, "b")
	if err != nil || item["resourceId"] != "b" {
		t.Fatal(item, err)
	}
	if _, err = exactAttachmentForDownload(items, "missing"); err == nil {
		t.Fatal("picked first attachment")
	}
	if _, err = exactAttachmentForDownload(append(items, items[0]), "a"); err == nil {
		t.Fatal("ambiguous resource accepted")
	}
}
func TestCrossPlatformCoverageParityAIRequiresExactField(t *testing.T) {
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"fields": []any{map[string]any{"fieldId": "other", "type": "text"}}})}}
	_, err := runAITableCompositeCLI(t, c, "+field-run-ai", "--base-id", "b", "--table-id", "t", "--field-ids", "f", "--yes")
	if err == nil || len(c.calls) != 1 {
		t.Fatal("ran missing field", err, c.calls)
	}
	c = &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"fields": []any{map[string]any{"fieldId": "f", "aiConfig": map[string]any{"prompt": "test", "outputType": "text"}}}}), parityStep(map[string]any{"success": true, "message": "some jobs already running"})}}
	out, err := runAITableCompositeCLI(t, c, "+field-run-ai", "--base-id", "b", "--table-id", "t", "--field-ids", "f", "--record-ids", "r", "--yes")
	var typed *apperrors.Error
	if out != "" || !errors.As(err, &typed) || typed.Reason != "aitable_ai_submission_unverified" {
		t.Fatal(out, err)
	}
	result := typed.Details["result"].(map[string]any)
	if result["completed"] != false || result["status"] != "submitted_unverified" || result["receipt"] == nil {
		t.Fatal(result)
	}
}
func TestCrossPlatformCoverageParityAIFieldTypeCompatibility(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		fail bool
	}{
		{`[{"fieldName":"AI","type":"text","aiConfig":{"prompt":[{"type":"text","value":"test"}],"outputType":"text"}}]`, false},
		{`[{"fieldName":"AI","type":"number","aiConfig":{"prompt":[{"type":"text","value":"test"}],"outputType":"text"}}]`, true},
		{`[{"fieldName":"AI","type":"text","aiConfig":{}}]`, true},
	} {
		_, err := parseBootstrapFields(tc.raw)
		if (err != nil) != tc.fail {
			t.Fatal(tc.raw, err)
		}
	}
}

func TestCrossPlatformCoverageParityFieldCreatePartitionsAndChecksReceipts(t *testing.T) {
	names := []any{}
	for i := 0; i < 16; i++ {
		names = append(names, map[string]any{"fieldName": fmt.Sprintf("F%d", i), "type": "text"})
	}
	fields := map[string]map[string]any{}
	batches := []int{}
	c := &upsertByKeyCaller{callFn: func(index int, _, tool string, args map[string]any) (string, error) {
		if tool == "get_fields" {
			out := []any{}
			if ids, ok := args["fieldIds"].([]string); ok {
				if len(ids) > 10 {
					t.Fatal("oversized field read")
				}
				for _, id := range ids {
					out = append(out, fields[id])
				}
			}
			return mustJSONText(t, map[string]any{"fields": out}), nil
		}
		if tool != "create_fields" {
			return "", fmt.Errorf("unexpected tool %s", tool)
		}
		rows := args["fields"].([]any)
		batches = append(batches, len(rows))
		result := []any{}
		for _, v := range rows {
			f := v.(map[string]any)
			id := fmt.Sprintf("id%d", len(fields))
			fields[id] = map[string]any{"fieldId": id, "fieldName": f["fieldName"], "type": "text"}
			result = append(result, map[string]any{"fieldId": id, "fieldName": f["fieldName"], "success": true})
		}
		return mustJSONText(t, map[string]any{"data": map[string]any{"results": result}}), nil
	}}
	out, err := runAITableCompositeCLI(t, c, "+field-create", "--base-id", "b", "--table-id", "t", "--fields", mustJSONText(t, names), "--yes")
	if err != nil || !strings.Contains(out, `"completedCount": 16`) || len(batches) != 2 || batches[0] != 15 || batches[1] != 1 {
		t.Fatal(out, err, batches)
	}
	ids, e := parityCreatedFieldIDs(map[string]any{"results": []any{map[string]any{"fieldName": "a", "success": true, "fieldId": "id"}, map[string]any{"fieldName": "b", "success": false, "errorMessage": "invalid type"}}}, []any{map[string]any{"fieldName": "a"}, map[string]any{"fieldName": "b"}})
	if e == nil || !strings.Contains(e.Error(), "invalid type") || len(ids) != 1 {
		t.Fatal(ids, e)
	}
}
func TestCrossPlatformCoverageParityFieldResumeNeverCreates(t *testing.T) {
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"fields": []any{map[string]any{"fieldId": "f", "fieldName": "Title", "type": "text"}}})}}
	out, err := runAITableCompositeCLI(t, c, "+field-create", "--base-id", "b", "--table-id", "t", "--fields", `[{"fieldName":"Title","type":"text"}]`, "--resume-field-ids", "f", "--yes")
	if err != nil || len(c.calls) != 1 || c.calls[0].tool != "get_fields" || !strings.Contains(out, `"createdThisRun": false`) {
		t.Fatal(out, err, c.calls)
	}
}
func TestCrossPlatformCoverageParityStrictCopyStopsBeforeCreating(t *testing.T) {
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"fields": []any{map[string]any{"fieldId": "f", "fieldName": "Computed", "type": "formula"}, map[string]any{"fieldId": "name", "fieldName": "Name", "type": "text"}}})}}
	out, err := runAITableCompositeCLI(t, c, "+table-copy", "--source-base-id", "a", "--source-table-id", "t", "--target-base-id", "b", "--new-name", "copy", "--strict-fields", "--yes")
	if err == nil || out != "" || len(c.calls) != 1 {
		t.Fatal("created incomplete table", out, err, c.calls)
	}
}

func TestCrossPlatformCoverageParityAppLifecycle(t *testing.T) {
	page := map[string]any{"pageId": "p", "pageName": "Name"}
	cfg := map[string]any{"chartType": "AI_ANALYZE"}
	layout := map[string]any{"x": float64(0), "y": float64(0), "w": float64(12), "h": float64(4)}
	widget := map[string]any{"pageId": "p", "widgetId": "w", "widgetName": "Name", "config": cfg, "layout": layout}
	appCreated := false
	pageDeleted, widgetDeleted := false, false
	caller := func() *upsertByKeyCaller {
		return &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
			var out any
			switch tool {
			case "get_app":
				out = map[string]any{"app": map[string]any{"appId": "app"}, "created": !appCreated}
				appCreated = true
			case "list_app_pages":
				items := []any{}
				if !pageDeleted {
					items = append(items, page)
				}
				out = map[string]any{"pages": items}
			case "get_app_page":
				out = page
			case "create_app_page", "update_app_page":
				out = map[string]any{"pageId": "p"}
			case "list_page_widgets":
				items := []any{}
				if !widgetDeleted {
					items = append(items, widget)
				}
				out = map[string]any{"pageId": "p", "widgets": items}
			case "get_app_widget":
				out = widget
			case "create_app_widget", "update_app_widget":
				out = map[string]any{"widgetId": "w"}
			case "delete_app_widget":
				widgetDeleted = true
				out = map[string]any{"deletedWidgetId": "w"}
			case "delete_app_page":
				pageDeleted = true
				out = map[string]any{"deletedPageId": "p"}
			default:
				return "", fmt.Errorf("unexpected tool %s", tool)
			}
			return mustJSONText(t, out), nil
		}}
	}
	cases := []struct {
		cmd  string
		args []string
	}{
		{"+app-get", nil}, {"+app-page-list", nil}, {"+app-page-get", []string{"--page-id", "p"}},
		{"+app-page-update", []string{"--page-id", "p", "--name", "Name"}},
		{"+app-block-list", []string{"--page-id", "p"}}, {"+app-block-get", []string{"--page-id", "p", "--widget-id", "w"}},
		{"+app-block-create", []string{"--page-id", "p", "--name", "Name", "--config", mustJSONText(t, cfg), "--layout", mustJSONText(t, layout)}},
		{"+app-block-update", []string{"--page-id", "p", "--widget-id", "w", "--name", "Name", "--config", mustJSONText(t, cfg), "--layout", mustJSONText(t, layout)}},
		{"+app-block-delete", []string{"--page-id", "p", "--widget-id", "w"}},
		{"+app-page-delete", []string{"--page-id", "p"}},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			c := caller()
			out, err := runAITableCompositeCLI(t, c, tc.cmd, append([]string{"--base-id", "b", "--yes"}, tc.args...)...)
			if err != nil || !strings.Contains(out, `"status": "verified"`) {
				t.Fatal(tc.cmd, out, err)
			}
			if len(c.calls) == 0 {
				t.Fatal("no real dispatch")
			}
		})
	}
}
func TestCrossPlatformCoverageParityFormLifecycle(t *testing.T) {
	hidden := false
	factory := func() *upsertByKeyCaller {
		return &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
			var r any
			switch tool {
			case "create_form_view":
				r = map[string]any{"viewId": "v"}
			case "list_form_views":
				r = map[string]any{"forms": []any{map[string]any{"viewId": "other", "name": "Other"}, map[string]any{"viewId": "v", "name": "Name"}}}
			case "get_fields":
				r = map[string]any{"fields": []any{map[string]any{"fieldId": "f", "fieldName": "Text", "type": "text"}}}
			case "list_form_fields":
				f := []any{}
				if !hidden {
					f = append(f, map[string]any{"fieldId": "f"})
				}
				r = map[string]any{"fields": f}
			case "update_form_field_hidden":
				if args["fieldId"] != "f" || args["hidden"] != true {
					t.Fatal("incorrect hide target", args)
				}
				hidden = true
				r = map[string]any{"success": true}
			case "submit_form":
				if args["value"] != `{"f":"value"}` {
					t.Fatal("value encoding", args)
				}
				r = map[string]any{"rowId": "r"}
			case "query_records":
				r = map[string]any{"records": []any{map[string]any{"recordId": "r", "cells": map[string]any{"f": "value"}}}}
			default:
				return "", fmt.Errorf("unexpected tool %s", tool)
			}
			return mustJSONText(t, r), nil
		}}
	}
	for _, tc := range []struct {
		cmd  string
		args []string
	}{
		{"+form-create", []string{"--name", "Name"}}, {"+form-get", []string{"--view-id", "v"}}, {"+form-submit", []string{"--view-id", "v", "--value", `{"f":"value"}`}}, {"+form-questions-remove", []string{"--view-id", "v", "--field-id", "f"}},
	} {
		c := factory()
		out, err := runAITableCompositeCLI(t, c, tc.cmd, append([]string{"--base-id", "b", "--table-id", "t", "--yes"}, tc.args...)...)
		if err != nil || !strings.Contains(out, `"status": "verified"`) {
			t.Fatal(tc.cmd, out, err)
		}
		for _, call := range c.calls {
			if call.tool == "delete_field" {
				t.Fatal("removed a table column")
			}
		}
	}
}

func TestCrossPlatformCoverageParityViewFilterSingletonEquivalence(t *testing.T) {
	leaf := map[string]any{"operator": "eq", "operands": []any{"f", "B"}}
	got := canonicalParityViewFilter(map[string]any{"operator": "and", "operands": []any{leaf}})
	want := canonicalParityViewFilter([]any{leaf})
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got, want)
	}
	or := map[string]any{"operator": "or", "operands": []any{leaf, map[string]any{"operator": "eq", "operands": []any{"f", "A"}}}}
	and := map[string]any{"operator": "and", "operands": or["operands"]}
	if reflect.DeepEqual(canonicalParityViewFilter(or), canonicalParityViewFilter(and)) {
		t.Fatal("OR widened into AND")
	}
}

func TestCrossPlatformCoverageParityExactReadDistinguishesMissingAndUnqueried(t *testing.T) {
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"records": []any{map[string]any{"recordId": "a"}}, "hasMore": false})}}
	out, err := runAITableCompositeCLI(t, c, "+record-query", "--base-id", "b", "--table-id", "t", "--record-ids", "a,missing,not-requested", "--limit", "2")
	if err != nil {
		t.Fatal(out, err)
	}
	var wire struct {
		Data struct {
			Complete  bool     `json:"complete"`
			Missing   []string `json:"missingRecordIds"`
			Unqueried []string `json:"unqueriedRecordIds"`
		} `json:"data"`
	}
	if e := json.Unmarshal([]byte(out), &wire); e != nil {
		t.Fatal(e)
	}
	if wire.Data.Complete || !reflect.DeepEqual(wire.Data.Missing, []string{"missing"}) || !reflect.DeepEqual(wire.Data.Unqueried, []string{"not-requested"}) {
		t.Fatal(out)
	}
}

func TestCrossPlatformCoverageParityViewUpdateUsesTypedFilterAndReadsExactID(t *testing.T) {
	for _, bad := range []string{"", "filter", "name", "desc", "missing", "parent"} {
		leaf := map[string]any{"operator": "eq", "operands": []any{"f", "B"}}
		view := map[string]any{"baseId": "b", "tableId": "t", "viewId": "v", "viewType": "Grid", "viewName": "Name", "viewDescription": map[string]any{"text": "Description"}, "config": map[string]any{"filter": map[string]any{"operator": "and", "operands": []any{leaf}}}}
		if bad == "filter" {
			view["filter"] = map[string]any{"operator": "eq", "operands": []any{"f", "A"}}
		}
		if bad == "name" {
			view["viewName"] = "Other"
		}
		if bad == "desc" {
			view["viewDescription"] = map[string]any{"text": "Other"}
		}
		if bad == "missing" {
			view["viewId"] = "other"
		}
		if bad == "parent" {
			view["tableId"] = "other"
		}
		c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{"fields": []any{map[string]any{"fieldId": "f", "type": "text"}}}), parityStep(map[string]any{"views": []any{map[string]any{"viewId": "v"}}}), parityStep(map[string]any{"success": true}), parityStep(map[string]any{"views": []any{view}})}}
		out, err := runAITableCompositeCLI(t, c, "+view-update", "--base-id", "b", "--table-id", "t", "--view-id", "v", "--name", "Name", "--desc", `{"text":"Description"}`, "--config", mustJSONText(t, map[string]any{"filter": []any{leaf}}), "--yes")
		if (err != nil) != (bad != "") {
			t.Fatal(bad, out, err, c.calls)
		}
		if len(c.calls) != 4 {
			t.Fatal("write/read protocol", c.calls)
		}
	}
}

func TestCrossPlatformCoverageParityExactAllRespectsRequestedPageSize(t *testing.T) {
	c := &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
		ids := args["recordIds"].([]string)
		if len(ids) != 1 || args["limit"] != 1 || tool != "query_records" {
			t.Fatal("ignored page size", args)
		}
		return mustJSONText(t, map[string]any{"records": []any{map[string]any{"recordId": ids[0]}}}), nil
	}}
	out, err := runAITableCompositeCLI(t, c, "+record-query", "--base-id", "b", "--table-id", "t", "--record-ids", "a,b,c", "--all", "--limit", "1")
	if err != nil || len(c.calls) != 3 || !strings.Contains(out, `"size": 3`) {
		t.Fatal(out, err, c.calls)
	}
}

func TestCrossPlatformCoverageParityFieldResumePublishesCompleteIDs(t *testing.T) {
	old := map[string]any{"fieldId": "old", "fieldName": "Old", "type": "text"}
	fresh := map[string]any{"fieldId": "new", "fieldName": "New", "type": "text"}
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{
		parityStep(map[string]any{"fields": []any{old}}), parityStep(map[string]any{"fields": []any{old}}),
		parityStep(map[string]any{"results": []any{map[string]any{"fieldId": "new", "fieldName": "New", "success": true}}}),
		parityStep(map[string]any{"fields": []any{fresh}}),
	}}
	out, err := runAITableCompositeCLI(t, c, "+field-create", "--base-id", "b", "--table-id", "t", "--fields", `[{"fieldName":"Old","type":"text"},{"fieldName":"New","type":"text"}]`, "--resume-field-ids", "old", "--yes")
	if err != nil {
		t.Fatal(out, err)
	}
	var body map[string]any
	if err = json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatal(err)
	}
	ids := body["resolved"].(map[string]any)["fieldIds"].([]any)
	if !reflect.DeepEqual(ids, []any{"old", "new"}) || body["completedCount"] != float64(2) || body["verification"].(map[string]any)["newFields"] != float64(1) {
		t.Fatal(body)
	}
	if len(c.calls) != 4 || c.calls[2].tool != "create_fields" || len(c.calls[2].args["fields"].([]any)) != 1 {
		t.Fatal(c.calls)
	}
}
