// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package report

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type reportCoverageCall struct {
	tool string
	args map[string]any
}

type reportCoverageCaller struct {
	responses map[string][]string
	history   []reportCoverageCall
}

func (caller *reportCoverageCaller) CallTool(_ context.Context, _, tool string, args map[string]any) (*edition.ToolResult, error) {
	caller.history = append(caller.history, reportCoverageCall{tool: tool, args: args})
	queue := caller.responses[tool]
	if len(queue) == 0 {
		return nil, errors.New("missing Report fake response for " + tool)
	}
	caller.responses[tool] = queue[1:]
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: queue[0]}}}, nil
}

func (*reportCoverageCaller) Format() string { return "json" }
func (*reportCoverageCaller) DryRun() bool   { return false }
func (*reportCoverageCaller) Fields() string { return "" }
func (*reportCoverageCaller) JQ() string     { return "" }

func runReportCoverage(t *testing.T, declaration shortcut.Shortcut, caller *reportCoverageCaller, args ...string) (*cobra.Command, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd, cmd.Execute()
}

func directReportRuntime(t *testing.T, declaration shortcut.Shortcut, caller *reportCoverageCaller, args ...string) *shortcut.RuntimeContext {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Flags().Parse(args); err != nil {
		t.Fatal(err)
	}
	return shortcut.RuntimeContextForTest(cmd, declaration)
}

func TestCrossPlatformCoverageReportContractsAreStrictTypedAndUnified(t *testing.T) {
	declarations := []shortcut.Shortcut{InboxList, OutboxList, TemplateSearch, ReportLatest}
	for _, declaration := range declarations {
		if declaration.Contract.Empty() || declaration.Contract.Result == nil {
			t.Errorf("%s lacks Contract/Result", declaration.Command)
		}
		if declaration.Safety.Effect != "read" || declaration.Safety.Confirmation != "not_required" {
			t.Errorf("%s safety=%+v", declaration.Command, declaration.Safety)
		}
		if declaration.OutputRollout != output.RolloutUnifiedActive {
			t.Errorf("%s rollout=%q", declaration.Command, declaration.OutputRollout)
		}
		if declaration.Contract.Interface == nil || declaration.Contract.Interface.Availability != "available" {
			t.Errorf("%s interface=%+v", declaration.Command, declaration.Contract.Interface)
		}
	}
	if InboxList.Contract.Pagination == nil || OutboxList.Contract.Pagination == nil {
		t.Fatal("Report list shortcuts must publish cursor pagination")
	}
	if TemplateSearch.Contract.Pagination != nil || ReportLatest.Contract.Pagination != nil {
		t.Fatal("non-paginated Report shortcuts published pagination")
	}
}

func TestCrossPlatformCoverageReportListResponseMatrix(t *testing.T) {
	valid := map[string]any{
		"success": true,
		"result": map[string]any{
			"report_list": []any{map[string]any{"reportId": "report-1", "templateName": "fixture", "createTime": float64(10)}},
		},
		"hasMore": false,
	}
	entries, page, err := reportProjectEntries(valid, "report/get_received_report_list")
	if err != nil || len(entries) != 1 || page.HasMore || entries[0]["createTime"] != int64(10) {
		t.Fatalf("valid projection entries=%#v page=%+v err=%v", entries, page, err)
	}
	empty := map[string]any{"success": true, "result": map[string]any{"report_list": []any{}}, "hasMore": false, "cursor": nil}
	if entries, page, err := reportProjectEntries(empty, "report/get_received_report_list"); err != nil || len(entries) != 0 || page.HasMore {
		t.Fatalf("terminal empty entries=%#v page=%+v err=%v", entries, page, err)
	}
	terminalReceipt := map[string]any{"success": true, "result": map[string]any{"report_list": []any{map[string]any{"reportId": "report-1"}}}, "hasMore": false, "nextCursor": float64(1)}
	if entries, page, err := reportProjectEntries(terminalReceipt, "report/get_received_report_list"); err != nil || len(entries) != 1 || page.HasMore || page.Next != "" {
		t.Fatalf("terminal cursor receipt entries=%#v page=%+v err=%v", entries, page, err)
	}
	fixtures := map[string]map[string]any{
		"empty response":     map[string]any{},
		"missing success":    map[string]any{"result": map[string]any{"report_list": []any{}}, "hasMore": false},
		"wrong success":      map[string]any{"success": "true", "result": map[string]any{"report_list": []any{}}, "hasMore": false},
		"remote failure":     map[string]any{"success": false, "result": map[string]any{"report_list": []any{}}, "hasMore": false},
		"missing result":     map[string]any{"success": true, "hasMore": false},
		"wrong result":       map[string]any{"success": true, "result": "bad", "hasMore": false},
		"missing collection": map[string]any{"success": true, "result": map[string]any{}, "hasMore": false},
		"wrong collection":   map[string]any{"success": true, "result": map[string]any{"report_list": map[string]any{}}, "hasMore": false},
		"bad item":           map[string]any{"success": true, "result": map[string]any{"report_list": []any{"bad"}}, "hasMore": false},
		"empty item":         map[string]any{"success": true, "result": map[string]any{"report_list": []any{map[string]any{}}}, "hasMore": false},
		"missing identity":   map[string]any{"success": true, "result": map[string]any{"report_list": []any{map[string]any{"createTime": float64(1)}}}, "hasMore": false},
		"wrong identity":     map[string]any{"success": true, "result": map[string]any{"report_list": []any{map[string]any{"reportId": float64(1)}}}, "hasMore": false},
		"duplicate identity": map[string]any{"success": true, "result": map[string]any{"report_list": []any{map[string]any{"reportId": "same"}, map[string]any{"reportId": "same"}}}, "hasMore": false},
		"wrong optional":     map[string]any{"success": true, "result": map[string]any{"report_list": []any{map[string]any{"reportId": "report-1", "createTime": "1"}}}, "hasMore": false},
		"missing pagination": map[string]any{"success": true, "result": map[string]any{"report_list": []any{map[string]any{"reportId": "report-1"}}}},
		"wrong pagination":   map[string]any{"success": true, "result": map[string]any{"report_list": []any{}}, "hasMore": "false"},
		"empty continuation": map[string]any{"success": true, "result": map[string]any{"report_list": []any{}}, "hasMore": true, "nextCursor": float64(2)},
		"missing continuation": map[string]any{
			"success": true, "result": map[string]any{"report_list": []any{map[string]any{"reportId": "report-1"}}}, "hasMore": true,
		},
		"wrong continuation": map[string]any{
			"success": true, "result": map[string]any{"report_list": []any{map[string]any{"reportId": "report-1"}}}, "hasMore": true, "nextCursor": "2",
		},
		"wrong terminal cursor": map[string]any{
			"success": true, "result": map[string]any{"report_list": []any{}}, "hasMore": false, "nextCursor": "2",
		},
		"conflicting has more": map[string]any{
			"success": true, "result": map[string]any{"report_list": []any{}, "hasMore": true}, "hasMore": false,
		},
		"conflicting continuation": map[string]any{
			"success": true, "result": map[string]any{"report_list": []any{map[string]any{"reportId": "report-1"}}, "cursor": float64(3)}, "hasMore": true, "nextCursor": float64(2),
		},
	}
	for name, fixture := range fixtures {
		if projected, _, projectErr := reportProjectEntries(fixture, "report/get_received_report_list"); projectErr == nil {
			t.Errorf("%s returned success: %#v", name, projected)
		}
	}
	if err := reportValidateContinuation(reportPageEvidence{HasMore: true, Next: "2"}, 2, "report/list"); err == nil {
		t.Fatal("stalled continuation returned success")
	}
}

func TestCrossPlatformCoverageReportTemplateResponseMatrix(t *testing.T) {
	valid := map[string]any{"success": true, "items": []any{
		map[string]any{"report_template_id": "template-1", "report_template_name": "Fixture Weekly", "last_modified_time": float64(2)},
		map[string]any{"report_template_id": "template-2", "report_template_name": "Fixture Daily"},
	}}
	templates, err := reportProjectTemplates(valid, "report/get_available_report_templates")
	if err != nil || len(templates) != 2 || templates[0]["lastModifiedTime"] != int64(2) {
		t.Fatalf("valid templates=%#v err=%v", templates, err)
	}
	for name, fixture := range map[string]map[string]any{
		"empty response":     {},
		"missing success":    {"items": []any{}},
		"wrong success":      {"success": float64(1), "items": []any{}},
		"missing collection": {"success": true},
		"wrong collection":   {"success": true, "items": map[string]any{}},
		"bad item":           {"success": true, "items": []any{"bad"}},
		"missing id":         {"success": true, "items": []any{map[string]any{"report_template_name": "fixture"}}},
		"missing name":       {"success": true, "items": []any{map[string]any{"report_template_id": "template-1"}}},
		"wrong name":         {"success": true, "items": []any{map[string]any{"report_template_id": "template-1", "report_template_name": float64(1)}}},
		"duplicate id":       {"success": true, "items": []any{map[string]any{"report_template_id": "same", "report_template_name": "one"}, map[string]any{"report_template_id": "same", "report_template_name": "two"}}},
		"wrong modified":     {"success": true, "items": []any{map[string]any{"report_template_id": "template-1", "report_template_name": "fixture", "last_modified_time": "2"}}},
	} {
		if projected, projectErr := reportProjectTemplates(fixture, "report/get_available_report_templates"); projectErr == nil {
			t.Errorf("%s returned success: %#v", name, projected)
		}
	}
	if projected, err := reportProjectTemplates(map[string]any{"success": true, "items": []any{}}, "report/templates"); err != nil || len(projected) != 0 {
		t.Fatalf("explicit empty template collection=%#v err=%v", projected, err)
	}
}

func TestCrossPlatformCoverageReportDetailResponseMatrix(t *testing.T) {
	valid := map[string]any{"success": true, "result": map[string]any{
		"report_Id": "report-2", "report_name": "fixture", "createTime": float64(2),
		"report_content": []any{map[string]any{"key": "field", "value": "value", "sort": float64(1), "type": float64(2)}},
	}}
	detail, err := reportProjectDetail(valid, "report/get_report_entry_details", "report-2")
	if err != nil || detail["reportId"] != "report-2" || len(detail["fields"].([]map[string]any)) != 1 {
		t.Fatalf("valid detail=%#v err=%v", detail, err)
	}
	for name, fixture := range map[string]map[string]any{
		"empty response":     {},
		"missing result":     {"success": true},
		"empty result":       {"success": true, "result": map[string]any{}},
		"missing id":         {"success": true, "result": map[string]any{"report_content": []any{}}},
		"identity mismatch":  {"success": true, "result": map[string]any{"report_Id": "other", "report_content": []any{}}},
		"missing collection": {"success": true, "result": map[string]any{"report_Id": "report-2"}},
		"wrong collection":   {"success": true, "result": map[string]any{"report_Id": "report-2", "report_content": map[string]any{}}},
		"bad field":          {"success": true, "result": map[string]any{"report_Id": "report-2", "report_content": []any{"bad"}}},
		"missing field key":  {"success": true, "result": map[string]any{"report_Id": "report-2", "report_content": []any{map[string]any{"value": "value", "sort": float64(1), "type": float64(2)}}}},
		"wrong field type":   {"success": true, "result": map[string]any{"report_Id": "report-2", "report_content": []any{map[string]any{"key": "field", "value": "value", "sort": "1", "type": float64(2)}}}},
	} {
		if projected, projectErr := reportProjectDetail(fixture, "report/get_report_entry_details", "report-2"); projectErr == nil {
			t.Errorf("%s returned success: %#v", name, projected)
		}
	}
}

func TestCrossPlatformCoverageReportExactShortcutsProjectUnifiedData(t *testing.T) {
	templateCaller := &reportCoverageCaller{responses: map[string][]string{
		"get_available_report_templates": {`{"success":true,"items":[{"report_template_id":"template-1","report_template_name":"Fixture Weekly","last_modified_time":2},{"report_template_id":"template-2","report_template_name":"Fixture Daily"}]}`},
	}}
	cmd, err := runReportCoverage(t, TemplateSearch, templateCaller, "--query", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	if code, emitted, emitErr := output.EmitStoredResult(cmd); emitErr != nil || !emitted || code != 0 {
		t.Fatalf("emit=(%d,%t,%v)", code, emitted, emitErr)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	if data["count"] != float64(1) || len(data["templates"].([]any)) != 1 {
		t.Fatalf("template search data=%#v", data)
	}
	if _, leaked := data["items"]; leaked {
		t.Fatalf("template search leaked raw collection: %#v", data)
	}

	inboxCaller := &reportCoverageCaller{responses: map[string][]string{
		"get_received_report_list": {`{"success":true,"result":{"report_list":[{"reportId":"report-1","createTime":1}]},"hasMore":false,"cursor":null}`},
	}}
	cmd, err = runReportCoverage(t, InboxList, inboxCaller,
		"--start", "2026-07-01T00:00:00+08:00", "--end", "2026-07-02T00:00:00+08:00")
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	cmd.SetOut(&stdout)
	if _, emitted, emitErr := output.EmitStoredResult(cmd); emitErr != nil || !emitted {
		t.Fatalf("inbox emit=(%t,%v)", emitted, emitErr)
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data = envelope["data"].(map[string]any)
	if data["count"] != float64(1) || data["complete"] != true {
		t.Fatalf("inbox data=%#v", data)
	}
}

func TestCrossPlatformCoverageReportLatestRequiresCompleteOrderedListAndExactReadback(t *testing.T) {
	missingOrder := &reportCoverageCaller{responses: map[string][]string{
		"get_send_report_list": {`{"success":true,"result":{"report_list":[{"reportId":"report-1"}]},"hasMore":false}`},
	}}
	if _, err := runReportCoverage(t, ReportLatest, missingOrder); err == nil || len(missingOrder.history) != 1 {
		t.Fatalf("missing order err=%v history=%v", err, missingOrder.history)
	}

	incomplete := &reportCoverageCaller{responses: map[string][]string{
		"get_send_report_list": {`{"success":true,"result":{"report_list":[{"reportId":"report-1","createTime":1}]},"hasMore":true,"nextCursor":20}`},
	}}
	if _, err := runReportCoverage(t, ReportLatest, incomplete); err == nil || len(incomplete.history) != 1 {
		t.Fatalf("incomplete err=%v history=%v", err, incomplete.history)
	}

	tied := &reportCoverageCaller{responses: map[string][]string{
		"get_send_report_list": {`{"success":true,"result":{"report_list":[{"reportId":"report-1","createTime":2},{"reportId":"report-2","createTime":2}]},"hasMore":false}`},
	}}
	if _, err := runReportCoverage(t, ReportLatest, tied); err == nil || len(tied.history) != 1 {
		t.Fatalf("tied latest err=%v history=%v", err, tied.history)
	}

	caller := &reportCoverageCaller{responses: map[string][]string{
		"get_send_report_list":     {`{"success":true,"result":{"report_list":[{"reportId":"report-1","createTime":1},{"reportId":"report-2","createTime":2}]},"hasMore":false}`},
		"get_report_entry_details": {`{"success":true,"result":{"report_Id":"report-2","report_name":"fixture","createTime":2,"report_content":[{"key":"field","value":"value","sort":1,"type":2}]}}`},
	}}
	cmd, err := runReportCoverage(t, ReportLatest, caller,
		"--start", "2026-07-01T00:00:00+08:00", "--end", "2026-07-20T00:00:00+08:00")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.history) != 2 || caller.history[0].tool != "get_send_report_list" || caller.history[1].tool != "get_report_entry_details" || caller.history[1].args["report_id"] != "report-2" {
		t.Fatalf("exact call history=%#v", caller.history)
	}
	if caller.history[0].args["startTime"] != int64(1782835200000) || caller.history[0].args["endTime"] != int64(1784476800000) {
		t.Fatalf("explicit range args=%#v", caller.history[0].args)
	}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	if _, emitted, emitErr := output.EmitStoredResult(cmd); emitErr != nil || !emitted {
		t.Fatalf("latest emit=(%t,%v)", emitted, emitErr)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	projected := envelope["data"].(map[string]any)["report"].(map[string]any)
	_, leaked := projected["success"]
	if projected["reportId"] != "report-2" || leaked {
		t.Fatalf("latest projection=%s", stdout.String())
	}
}

func TestCrossPlatformCoverageReportValidationRejectsInvalidRangesBeforeMCP(t *testing.T) {
	tests := []struct {
		declaration shortcut.Shortcut
		args        []string
	}{
		{InboxList, []string{"--start", "2026-07-02T00:00:00+08:00", "--end", "2026-07-01T00:00:00+08:00"}},
		{InboxList, []string{"--start", "2026-07-01T00:00:00+08:00", "--end", "2026-07-02T00:00:00+08:00", "--size", "21"}},
		{OutboxList, []string{"--modified-start", "2026-07-01T00:00:00+08:00"}},
		{ReportLatest, []string{"--start", "2026-07-01T00:00:00+08:00"}},
		{ReportLatest, []string{"--start", "2026-07-22T00:00:00+08:00", "--end", "2026-07-01T00:00:00+08:00"}},
	}
	for _, test := range tests {
		caller := &reportCoverageCaller{responses: map[string][]string{}}
		if _, err := runReportCoverage(t, test.declaration, caller, test.args...); err == nil || len(caller.history) != 0 {
			t.Errorf("%s args=%v err=%v calls=%v", test.declaration.Command, test.args, err, caller.history)
		}
	}
}

func TestCrossPlatformCoverageReportPrimitiveStrictnessAndProjectionBranches(t *testing.T) {
	operation := "report/coverage"
	if err := reportRequireSuccess(map[string]any{"success": false, "errorMessage": " rejected "}, operation); err == nil {
		t.Fatal("remote failure with message returned success")
	}
	arrayEntries, container, err := reportListCollection(map[string]any{
		"success": true,
		"result":  []any{map[string]any{"report_id": "report-array"}},
		"hasMore": false,
	}, operation)
	if err != nil || len(arrayEntries) != 1 || container["hasMore"] != false {
		t.Fatalf("array collection entries=%#v container=%#v err=%v", arrayEntries, container, err)
	}
	if _, err := reportRequiredString(map[string]any{"reportId": "one", "report_id": "two"}, operation, 0, "reportId", "report_id"); err == nil {
		t.Fatal("conflicting aliases returned success")
	}
	if _, err := reportOptionalString(map[string]any{"name": 1}, operation, 0, "name"); err == nil {
		t.Fatal("non-string optional value returned success")
	}

	integerCases := []struct {
		value any
		want  int64
		ok    bool
	}{
		{float64(7), 7, true}, {math.NaN(), 0, false}, {math.Inf(1), 0, false},
		{1.5, 0, false}, {int(8), 8, true}, {int64(9), 9, true},
		{json.Number("10"), 10, true}, {json.Number("bad"), 0, false}, {"11", 0, false},
	}
	for _, test := range integerCases {
		got, ok := reportInteger(test.value)
		if got != test.want || ok != test.ok {
			t.Errorf("reportInteger(%#v)=(%d,%t), want (%d,%t)", test.value, got, ok, test.want, test.ok)
		}
	}

	fullEntry := map[string]any{
		"success": true,
		"result": map[string]any{
			"hasMore":    true,
			"nextCursor": json.Number("2"),
			"report_list": []any{map[string]any{
				"report_id": "report-full", "report_template_name": "Weekly",
				"senderName": "sender", "senderUserId": "user", "gmtCreate": int(12), "modifyTime": int64(13),
			}},
		},
	}
	entries, page, err := reportProjectEntries(fullEntry, operation)
	if err != nil || len(entries) != 1 || !page.HasMore || page.Next != "2" || entries[0]["modifiedTime"] != int64(13) {
		t.Fatalf("full entry entries=%#v page=%+v err=%v", entries, page, err)
	}
	if err := reportValidateContinuation(page, 1, operation); err != nil {
		t.Fatalf("advancing continuation: %v", err)
	}
	if err := reportValidateContinuation(reportPageEvidence{}, 0, operation); err != nil {
		t.Fatalf("terminal continuation: %v", err)
	}
	if err := reportValidateContinuation(reportPageEvidence{HasMore: true, Next: "bad"}, 0, operation); err == nil {
		t.Fatal("malformed continuation returned success")
	}
	badOptional := map[string]any{
		"success": true, "hasMore": false,
		"result": map[string]any{"report_list": []any{map[string]any{"reportId": "report-1", "senderName": 1}}},
	}
	if _, _, err := reportProjectEntries(badOptional, operation); err == nil {
		t.Fatal("bad optional entry field returned success")
	}

	completeDetail := map[string]any{"success": true, "result": map[string]any{
		"reportId": "report-detail", "reportName": "Title", "templateName": "Template", "senderName": "Creator",
		"create_time": int(4), "modified_time": json.Number("5"),
		"report_content": []any{map[string]any{"key": "field", "value": "", "sort": int(1), "type": int64(2)}},
	}}
	detail, err := reportProjectDetail(completeDetail, operation, "report-detail")
	if err != nil || detail["modifiedTime"] != int64(5) || len(detail["fields"].([]map[string]any)) != 1 {
		t.Fatalf("complete detail=%#v err=%v", detail, err)
	}
	for name, fixture := range map[string]map[string]any{
		"bad optional string":  {"success": true, "result": map[string]any{"reportId": "report-detail", "reportName": 1, "report_content": []any{}}},
		"bad optional integer": {"success": true, "result": map[string]any{"reportId": "report-detail", "create_time": "4", "report_content": []any{}}},
		"missing field value":  {"success": true, "result": map[string]any{"reportId": "report-detail", "report_content": []any{map[string]any{"key": "field", "sort": 1, "type": 2}}}},
	} {
		if projected, projectErr := reportProjectDetail(fixture, operation, "report-detail"); projectErr == nil {
			t.Errorf("%s returned success: %#v", name, projected)
		}
	}
}

func TestCrossPlatformCoverageReportValidationHelpersCoverAllContracts(t *testing.T) {
	if _, err := reportParseISOMillis("start", "not-a-time"); err == nil {
		t.Fatal("invalid ISO timestamp returned success")
	}
	if err := reportValidatePage(-1, 20); err == nil {
		t.Fatal("negative cursor returned success")
	}
	if err := reportValidatePage(0, 0); err == nil {
		t.Fatal("zero page size returned success")
	}
	if err := reportValidatePage(0, 20); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, start, end string
		maximumDays      int
	}{
		{"bad start", "bad", "2026-07-02T00:00:00+08:00", 20},
		{"bad end", "2026-07-01T00:00:00+08:00", "bad", 20},
		{"too wide", "2026-07-01T00:00:00+08:00", "2026-08-01T00:00:00+08:00", 20},
	} {
		if _, _, err := reportValidateRange("start", test.start, "end", test.end, test.maximumDays); err == nil {
			t.Errorf("%s returned success", test.name)
		}
	}
	if _, _, err := reportValidateRange("start", "2026-07-01T00:00:00+08:00", "end", "2026-08-01T00:00:00+08:00", 0); err != nil {
		t.Fatalf("unbounded valid range: %v", err)
	}
}

func TestCrossPlatformCoverageReportExecutorsSuccessErrorsAndZeroCalls(t *testing.T) {
	start := "2026-07-01T00:00:00+08:00"
	end := "2026-07-02T00:00:00+08:00"
	validList := `{"success":true,"result":{"report_list":[{"reportId":"report-1","createTime":1}]} ,"hasMore":false}`

	inbox := &reportCoverageCaller{responses: map[string][]string{"get_received_report_list": {validList}}}
	if _, err := runReportCoverage(t, InboxList, inbox, "--start", start, "--end", end, "--sender-user-ids", "user-1,user-2"); err != nil {
		t.Fatal(err)
	}
	if len(inbox.history) != 1 || len(inbox.history[0].args["senderUserIds"].([]string)) != 2 {
		t.Fatalf("inbox args/history=%#v", inbox.history)
	}
	for _, test := range []struct {
		name   string
		caller *reportCoverageCaller
		args   []string
	}{
		{"call error", &reportCoverageCaller{responses: map[string][]string{}}, []string{"--start", start, "--end", end}},
		{"projection error", &reportCoverageCaller{responses: map[string][]string{"get_received_report_list": {`{"success":true}`}}}, []string{"--start", start, "--end", end}},
		{"stalled cursor", &reportCoverageCaller{responses: map[string][]string{"get_received_report_list": {`{"success":true,"result":{"report_list":[{"reportId":"report-1"}]},"hasMore":true,"nextCursor":2}`}}}, []string{"--start", start, "--end", end, "--cursor", "2"}},
	} {
		if _, err := runReportCoverage(t, InboxList, test.caller, test.args...); err == nil {
			t.Errorf("inbox %s returned success", test.name)
		}
	}
	directInbox := directReportRuntime(t, InboxList, &reportCoverageCaller{responses: map[string][]string{}}, "--start", "bad", "--end", end)
	if err := InboxList.Execute(directInbox); err == nil {
		t.Fatal("direct inbox invalid range returned success")
	}

	outboxAll := &reportCoverageCaller{responses: map[string][]string{"get_send_report_list": {`{"success":true,"result":{"report_list":[{"reportId":"report-2","createTime":2}]},"hasMore":true,"nextCursor":3}`}}}
	if _, err := runReportCoverage(t, OutboxList, outboxAll,
		"--cursor", "2", "--start", start, "--end", end,
		"--modified-start", start, "--modified-end", end, "--template-name", " Weekly "); err != nil {
		t.Fatal(err)
	}
	if len(outboxAll.history) != 1 || outboxAll.history[0].args["report_template_name"] != "Weekly" || outboxAll.history[0].args["modifiedStartTime"] == nil {
		t.Fatalf("outbox args/history=%#v", outboxAll.history)
	}
	defaultOutbox := &reportCoverageCaller{responses: map[string][]string{"get_send_report_list": {validList}}}
	if _, err := runReportCoverage(t, OutboxList, defaultOutbox); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		caller *reportCoverageCaller
		args   []string
	}{
		{"call error", &reportCoverageCaller{responses: map[string][]string{}}, nil},
		{"projection error", &reportCoverageCaller{responses: map[string][]string{"get_send_report_list": {`{"success":true}`}}}, nil},
		{"stalled cursor", &reportCoverageCaller{responses: map[string][]string{"get_send_report_list": {`{"success":true,"result":{"report_list":[{"reportId":"report-1"}]},"hasMore":true,"nextCursor":2}`}}}, []string{"--cursor", "2"}},
	} {
		if _, err := runReportCoverage(t, OutboxList, test.caller, test.args...); err == nil {
			t.Errorf("outbox %s returned success", test.name)
		}
	}
	directOutbox := directReportRuntime(t, OutboxList, &reportCoverageCaller{responses: map[string][]string{}}, "--start", "bad", "--end", end)
	if err := OutboxList.Execute(directOutbox); err == nil {
		t.Fatal("direct outbox bad creation range returned success")
	}
	directOutbox = directReportRuntime(t, OutboxList, &reportCoverageCaller{responses: map[string][]string{}},
		"--modified-start", "bad", "--modified-end", end)
	if err := OutboxList.Execute(directOutbox); err == nil {
		t.Fatal("direct outbox bad modified range returned success")
	}

	for _, test := range []struct {
		name        string
		declaration shortcut.Shortcut
		args        []string
	}{
		{"outbox invalid page", OutboxList, []string{"--size", "0"}},
		{"outbox invalid creation range", OutboxList, []string{"--start", "bad", "--end", end}},
		{"outbox valid modified range", OutboxList, []string{"--modified-start", start, "--modified-end", end}},
		{"template empty query", TemplateSearch, []string{"--query", "   "}},
	} {
		caller := &reportCoverageCaller{responses: map[string][]string{}}
		_, err := runReportCoverage(t, test.declaration, caller, test.args...)
		if test.name == "outbox valid modified range" {
			if err == nil || len(caller.history) != 1 {
				t.Errorf("%s err=%v history=%#v", test.name, err, caller.history)
			}
			continue
		}
		if err == nil || len(caller.history) != 0 {
			t.Errorf("%s err=%v history=%#v", test.name, err, caller.history)
		}
	}

	for name, caller := range map[string]*reportCoverageCaller{
		"call error":       {responses: map[string][]string{}},
		"projection error": {responses: map[string][]string{"get_available_report_templates": {`{"success":true}`}}},
	} {
		if _, err := runReportCoverage(t, TemplateSearch, caller, "--query", "weekly"); err == nil {
			t.Errorf("template %s returned success", name)
		}
	}
	directTemplate := directReportRuntime(t, TemplateSearch, &reportCoverageCaller{responses: map[string][]string{}}, "--query", "   ")
	if err := TemplateSearch.Validate(directTemplate); err == nil {
		t.Fatal("direct template whitespace query returned success")
	}
	legacyTemplate := TemplateSearch
	legacyTemplate.OutputRollout = output.RolloutLegacyOnly
	legacyTemplateCaller := &reportCoverageCaller{responses: map[string][]string{"get_available_report_templates": {`{"success":true,"items":[]}`}}}
	if _, err := runReportCoverage(t, legacyTemplate, legacyTemplateCaller, "--query", "zero"); err != nil {
		t.Fatalf("legacy template: %v", err)
	}
}

func TestCrossPlatformCoverageReportLatestExecutorRemainingBranches(t *testing.T) {
	validList := `{"success":true,"result":{"report_list":[{"reportId":"report-1","createTime":1}]},"hasMore":false}`
	validDetail := `{"success":true,"result":{"report_Id":"report-1","report_content":[]}}`
	defaultCaller := &reportCoverageCaller{responses: map[string][]string{
		"get_send_report_list": {validList}, "get_report_entry_details": {validDetail},
	}}
	if _, err := runReportCoverage(t, ReportLatest, defaultCaller, "--keyword", " Weekly "); err != nil {
		t.Fatal(err)
	}
	if defaultCaller.history[0].args["report_template_name"] != "Weekly" {
		t.Fatalf("latest keyword args=%#v", defaultCaller.history[0].args)
	}

	for name, caller := range map[string]*reportCoverageCaller{
		"list call error": {responses: map[string][]string{}},
		"list projection error": {responses: map[string][]string{
			"get_send_report_list": {`{"success":true}`},
		}},
		"empty candidates": {responses: map[string][]string{
			"get_send_report_list": {`{"success":true,"result":{"report_list":[]},"hasMore":false}`},
		}},
		"detail call error": {responses: map[string][]string{
			"get_send_report_list": {validList},
		}},
		"detail projection error": {responses: map[string][]string{
			"get_send_report_list": {validList}, "get_report_entry_details": {`{"success":true}`},
		}},
	} {
		if _, err := runReportCoverage(t, ReportLatest, caller); err == nil {
			t.Errorf("latest %s returned success", name)
		}
	}
	directLatest := directReportRuntime(t, ReportLatest, &reportCoverageCaller{responses: map[string][]string{}},
		"--start", "bad", "--end", "2026-07-02T00:00:00+08:00")
	if err := ReportLatest.Execute(directLatest); err == nil {
		t.Fatal("direct latest invalid range returned success")
	}
	if _, err := reportLatestEntryID([]map[string]any{{"createTime": int64(1), "reportId": 1}}, "report/latest"); err == nil {
		t.Fatal("non-string latest report identity returned success")
	}
}

func TestCrossPlatformCoverageReportOutputLegacyAndInvalidPagination(t *testing.T) {
	command := &cobra.Command{Use: "report-page"}
	command.SetOut(io.Discard)
	command.SetContext(context.Background())
	declaration := shortcut.Shortcut{Service: "report", Product: "report"}
	rt := shortcut.RuntimeContextForTest(command, declaration)
	output.SetCommandRollout(command, output.RolloutLegacyOnly)
	if err := outputReportPage(rt, "reports", []map[string]any{{"reportId": "report-1"}}, reportPageEvidence{HasMore: true, Next: "2"}); err != nil {
		t.Fatalf("legacy report page: %v", err)
	}

	ctx, _ := output.WithResultStore(context.Background())
	command.SetContext(ctx)
	output.SetCommandRollout(command, output.RolloutUnifiedActive)
	if err := outputReportPage(rt, "reports", nil, reportPageEvidence{HasMore: true}); err == nil {
		t.Fatal("unified page without continuation returned success")
	}
}
