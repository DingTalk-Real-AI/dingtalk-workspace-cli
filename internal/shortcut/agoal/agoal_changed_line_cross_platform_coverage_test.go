// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package agoal

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAgoalScalarAndProjectionEdges(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  int
		ok    bool
	}{
		{name: "int", value: 3, want: 3, ok: true},
		{name: "negative int", value: -1, want: -1},
		{name: "int64", value: int64(4), want: 4, ok: true},
		{name: "negative int64", value: int64(-1)},
		{name: "float", value: float64(5), want: 5, ok: true},
		{name: "negative float", value: float64(-1)},
		{name: "fraction", value: 1.5},
		{name: "huge float", value: math.MaxFloat64},
		{name: "json number", value: json.Number("6"), want: 6, ok: true},
		{name: "bad json number", value: json.Number("1.5")},
		{name: "negative json number", value: json.Number("-1")},
		{name: "wrong type", value: "7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := agoalNonNegativeInt(tc.value)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("agoalNonNegativeInt(%#v) = (%d,%v), want (%d,%v)", tc.value, got, ok, tc.want, tc.ok)
			}
		})
	}

	items := []map[string]any{{"id": "one", "name": "Known"}}
	if got := filterAgoalItems(items, "", "name"); len(got) != 1 {
		t.Fatalf("empty item filter = %#v", got)
	}
	if got := filterAgoalSubmissions(items, ""); len(got) != 1 {
		t.Fatalf("empty submission filter = %#v", got)
	}
	if got := agoalSubmissionIdentity(map[string]any{"user": map[string]any{"id": "user"}}); got != "user:user" {
		t.Fatalf("fallback identity = %q", got)
	}

	for _, bad := range []any{
		nil,
		[]any{"bad"},
		[]any{map[string]any{"user": "bad"}},
		[]any{map[string]any{"user": map[string]any{"name": "missing identity"}}},
	} {
		if _, err := requireAgoalSubmissionList(bad); err == nil {
			t.Fatalf("malformed submission list accepted: %#v", bad)
		}
	}
}

func TestCrossPlatformCoverageAgoalDateNormalizationEdges(t *testing.T) {
	if got, err := normalizeAgoalQueryDate(" 2026-08-24 "); err != nil || got != "2026-08-24" {
		t.Fatalf("date normalization = %q, %v", got, err)
	}
	if got, err := normalizeAgoalQueryDate("2026-08-24T01:00:00Z"); err != nil || got != "2026-08-24" {
		t.Fatalf("RFC3339 normalization = %q, %v", got, err)
	}
	testseam.Swap(t, &agoalLoadLocation, func(string) (*time.Location, error) {
		return nil, errors.New("fixture location failure")
	})
	if got, err := normalizeAgoalQueryDate("2026-08-24T18:00:00Z"); err != nil || got != "2026-08-25" {
		t.Fatalf("fallback timezone normalization = %q, %v", got, err)
	}
}

func TestCrossPlatformCoverageAgoalLeafFailureWiring(t *testing.T) {
	tests := []struct {
		name        string
		declaration shortcut.Shortcut
		args        []string
		tool        string
		response    map[string]any
	}{
		{name: "report transport", declaration: ReportStatisticsList, tool: "list_report_statistics"},
		{name: "report failure", declaration: ReportStatisticsList, tool: "list_report_statistics", response: map[string]any{"success": false}},
		{name: "report conflicting error", declaration: ReportStatisticsList, tool: "list_report_statistics", response: map[string]any{"success": true, "errorCode": "DENIED", "content": []any{}}},
		{name: "report missing success", declaration: ReportStatisticsList, tool: "list_report_statistics", response: map[string]any{"content": []any{}}},
		{name: "report malformed items", declaration: ReportStatisticsList, tool: "list_report_statistics", response: map[string]any{"success": true, "content": []any{"bad"}}},
		{name: "template transport", declaration: ObjectTemplateList, tool: "list_obj_template"},
		{name: "template failure", declaration: ObjectTemplateList, tool: "list_obj_template", response: map[string]any{"success": false}},
		{name: "template conflicting error", declaration: ObjectTemplateList, tool: "list_obj_template", response: map[string]any{"success": true, "errorCode": "DENIED", "content": map[string]any{}}},
		{name: "template missing success", declaration: ObjectTemplateList, tool: "list_obj_template", response: map[string]any{"content": map[string]any{}}},
		{name: "template content", declaration: ObjectTemplateList, tool: "list_obj_template", response: map[string]any{"success": true, "content": []any{}}},
		{name: "template items", declaration: ObjectTemplateList, tool: "list_obj_template", response: map[string]any{"success": true, "content": map[string]any{"result": "bad", "page": 1, "pageSize": 20, "totalCount": 0}}},
		{name: "template pagination", declaration: ObjectTemplateList, tool: "list_obj_template", response: map[string]any{"success": true, "content": map[string]any{"result": []any{}, "page": "1", "pageSize": 20, "totalCount": 0}}},
		{name: "template page mismatch", declaration: ObjectTemplateList, tool: "list_obj_template", response: map[string]any{"success": true, "content": map[string]any{"result": []any{}, "page": 2, "pageSize": 20, "totalCount": 0}}},
		{name: "fields transport", declaration: ContractFields, tool: "list_op_contract_fields"},
		{name: "fields failure", declaration: ContractFields, tool: "list_op_contract_fields", response: map[string]any{"success": false}},
		{name: "fields conflicting error", declaration: ContractFields, tool: "list_op_contract_fields", response: map[string]any{"success": true, "errorCode": "DENIED", "content": []any{}}},
		{name: "fields missing success", declaration: ContractFields, tool: "list_op_contract_fields", response: map[string]any{"content": []any{}}},
		{name: "fields items", declaration: ContractFields, tool: "list_op_contract_fields", response: map[string]any{"success": true, "content": []any{"bad"}}},
		{name: "rules transport", declaration: UserRules, tool: "get_user_rules"},
		{name: "rules failure", declaration: UserRules, tool: "get_user_rules", response: map[string]any{"success": false}},
		{name: "rules conflicting error", declaration: UserRules, tool: "get_user_rules", response: map[string]any{"success": true, "errorCode": "DENIED", "content": map[string]any{}}},
		{name: "rules missing success", declaration: UserRules, tool: "get_user_rules", response: map[string]any{"content": map[string]any{}}},
		{name: "rules content", declaration: UserRules, tool: "get_user_rules", response: map[string]any{"success": true, "content": []any{}}},
		{name: "rules items", declaration: UserRules, tool: "get_user_rules", response: map[string]any{"success": true, "content": map[string]any{"rules": []any{"bad"}}}},
		{name: "submission transport", declaration: ReportSubmitDetail, args: []string{"--template-id", "template", "--submit-state", "ON_TIME"}, tool: "get_submit_detail"},
		{name: "submission failure", declaration: ReportSubmitDetail, args: []string{"--template-id", "template", "--submit-state", "ON_TIME"}, tool: "get_submit_detail", response: map[string]any{"success": false}},
		{name: "submission conflicting error", declaration: ReportSubmitDetail, args: []string{"--template-id", "template", "--submit-state", "ON_TIME"}, tool: "get_submit_detail", response: map[string]any{"success": true, "errorCode": "DENIED", "content": map[string]any{}}},
		{name: "submission missing success", declaration: ReportSubmitDetail, args: []string{"--template-id", "template", "--submit-state", "ON_TIME"}, tool: "get_submit_detail", response: map[string]any{"content": map[string]any{}}},
		{name: "submission content", declaration: ReportSubmitDetail, args: []string{"--template-id", "template", "--submit-state", "ON_TIME"}, tool: "get_submit_detail", response: map[string]any{"success": true, "content": []any{}}},
		{name: "submission items", declaration: ReportSubmitDetail, args: []string{"--template-id", "template", "--submit-state", "ON_TIME"}, tool: "get_submit_detail", response: map[string]any{"success": true, "content": map[string]any{"result": "bad", "page": 1, "pageSize": 20, "totalCount": 0}}},
		{name: "submission pagination", declaration: ReportSubmitDetail, args: []string{"--template-id", "template", "--submit-state", "ON_TIME"}, tool: "get_submit_detail", response: map[string]any{"success": true, "content": map[string]any{"result": []any{}, "page": "1", "pageSize": 20, "totalCount": 0}}},
		{name: "submission size mismatch", declaration: ReportSubmitDetail, args: []string{"--template-id", "template", "--submit-state", "ON_TIME"}, tool: "get_submit_detail", response: map[string]any{"success": true, "content": map[string]any{"result": []any{}, "page": 1, "pageSize": 21, "totalCount": 0}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &agoalCoverageCaller{}
			if tc.response != nil {
				caller.responses = map[string][]map[string]any{tc.tool: {tc.response}}
			}
			if err := runAgoalCoverage(t, tc.declaration, caller, tc.args...); err == nil {
				t.Fatal("failure fixture was accepted")
			}
		})
	}

	caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
		"get_user_rules": {{"success": true, "content": map[string]any{"rules": []any{map[string]any{"id": "rule"}}}}},
	}}
	if err := runAgoalCoverage(t, UserRules, caller); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageAgoalBoundedPaginationAndDateExecution(t *testing.T) {
	responses := make([]map[string]any, 0, agoalSubmissionMaxPages)
	for page := 1; page <= agoalSubmissionMaxPages; page++ {
		responses = append(responses, map[string]any{"success": true, "content": map[string]any{
			"page": page, "pageSize": agoalSubmissionReadPageSize, "totalCount": agoalSubmissionMaxPages + 1,
			"result": []any{map[string]any{"reportId": "report-" + json.Number(string(rune('A'+page-1))).String(), "user": map[string]any{"id": "user"}}},
		}})
	}
	caller := &agoalCoverageCaller{responses: map[string][]map[string]any{"get_submit_detail": responses}}
	if err := runAgoalCoverage(t, ReportSubmitDetail, caller,
		"--template-id", "template", "--submit-state", "ON_TIME", "--keyword", "known"); err == nil {
		t.Fatal("bounded pagination limit was accepted as complete")
	}

	scanFailure := &agoalCoverageCaller{responses: map[string][]map[string]any{
		"get_submit_detail": {{"content": map[string]any{}}},
	}}
	if err := runAgoalCoverage(t, ReportSubmitDetail, scanFailure,
		"--template-id", "template", "--submit-state", "ON_TIME", "--keyword", "known"); err == nil {
		t.Fatal("scan page validation failure was accepted")
	}

	dateCaller := &agoalCoverageCaller{responses: map[string][]map[string]any{
		"get_submit_detail": {{"success": true, "content": map[string]any{
			"page": 1, "pageSize": 20, "totalCount": 0, "result": []any{},
		}}},
	}}
	if err := runAgoalCoverage(t, ReportSubmitDetail, dateCaller,
		"--template-id", "template", "--submit-state", "ON_TIME", "--query-date", "2026-08-24T01:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if got := dateCaller.calls[0].params["queryDate"]; got != "2026-08-24" {
		t.Fatalf("queryDate = %#v", got)
	}

	laterPage := &agoalCoverageCaller{responses: map[string][]map[string]any{
		"get_submit_detail": {{"success": true, "content": map[string]any{
			"page": 1, "pageSize": agoalSubmissionReadPageSize, "totalCount": 1,
			"result": []any{map[string]any{"reportId": "report", "user": map[string]any{"id": "user", "name": "Different"}}},
		}}},
	}}
	if err := runAgoalCoverage(t, ReportSubmitDetail, laterPage,
		"--template-id", "template", "--submit-state", "ON_TIME", "--keyword", "missing", "--page", "2"); err != nil {
		t.Fatal(err)
	}
	maximumPage := &agoalCoverageCaller{responses: map[string][]map[string]any{
		"get_submit_detail": {{"success": true, "content": map[string]any{
			"page": 1, "pageSize": agoalSubmissionReadPageSize, "totalCount": 1,
			"result": []any{map[string]any{"reportId": "report", "user": map[string]any{"id": "user", "name": "Different"}}},
		}}},
	}}
	if err := runAgoalCoverage(t, ReportSubmitDetail, maximumPage,
		"--template-id", "template", "--submit-state", "ON_TIME", "--keyword", "missing",
		"--page", strconv.Itoa(int(^uint(0)>>1))); err != nil {
		t.Fatalf("maximum page must project an empty local page without overflowing: %v", err)
	}
}
