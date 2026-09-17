// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package agoal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type agoalCoverageCall struct {
	tool   string
	params map[string]any
}

type agoalCoverageCaller struct {
	responses map[string][]map[string]any
	calls     []agoalCoverageCall
}

func (c *agoalCoverageCaller) CallTool(_ context.Context, _, tool string, params map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, agoalCoverageCall{tool: tool, params: params})
	queue := c.responses[tool]
	if len(queue) == 0 {
		return nil, errors.New("unexpected agoal coverage call: " + tool)
	}
	payload := queue[0]
	c.responses[tool] = queue[1:]
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(encoded)}}}, nil
}

func (*agoalCoverageCaller) Format() string { return "json" }
func (*agoalCoverageCaller) DryRun() bool   { return false }
func (*agoalCoverageCaller) Fields() string { return "" }
func (*agoalCoverageCaller) JQ() string     { return "" }

func runAgoalCoverage(t *testing.T, declaration shortcut.Shortcut, caller *agoalCoverageCaller, args ...string) error {
	t.Helper()
	helpers.InitDeps(caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	service := &cobra.Command{Use: productAgoal}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(append([]string{productAgoal, declaration.Command}, args...))
	return root.Execute()
}

func agoalRegistered(command string) (shortcut.Shortcut, bool) {
	for _, item := range shortcut.All() {
		if item.Service == productAgoal && item.Command == command {
			return item, true
		}
	}
	return shortcut.Shortcut{}, false
}

func TestCrossPlatformCoverageAgoalStrictResponseValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		data map[string]any
	}{
		{name: "empty", data: nil},
		{name: "missing success", data: map[string]any{"content": []any{}}},
		{name: "wrong success", data: map[string]any{"success": "true", "content": []any{}}},
		{name: "explicit failure", data: map[string]any{"success": false, "content": []any{}}},
		{name: "conflicting error", data: map[string]any{"success": true, "errorCode": "DENIED", "content": []any{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := requireAgoalSuccess(tc.data, "agoal/test"); err == nil {
				t.Fatal("malformed response was accepted")
			}
		})
	}

	for _, tc := range []struct {
		name  string
		value any
	}{
		{name: "missing collection", value: nil},
		{name: "wrong collection", value: map[string]any{}},
		{name: "bad element", value: []any{"bad"}},
		{name: "missing id", value: []any{map[string]any{"title": "template"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := requireAgoalList(map[string]any{"success": true}, "agoal/test", tc.value, "id"); err == nil {
				t.Fatal("malformed collection was accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageAgoalPublicReadsUseExactToolsAndShapes(t *testing.T) {
	t.Run("report nonempty", func(t *testing.T) {
		caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"list_report_statistics": {{"success": true, "content": []any{map[string]any{"templateId": "template"}}}},
		}}
		if err := runAgoalCoverage(t, ReportStatisticsList, caller, "--keyword", "weekly"); err != nil {
			t.Fatal(err)
		}
		want := []agoalCoverageCall{{tool: "list_report_statistics", params: map[string]any{"keyword": "weekly"}}}
		if !reflect.DeepEqual(caller.calls, want) {
			t.Fatalf("calls = %#v, want %#v", caller.calls, want)
		}
	})

	t.Run("report legal zero", func(t *testing.T) {
		caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"list_report_statistics": {{"success": true, "content": []any{}}},
		}}
		if err := runAgoalCoverage(t, ReportStatisticsList, caller, "--keyword", "guaranteed-zero"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("template nonempty page", func(t *testing.T) {
		caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"list_obj_template": {{"success": true, "content": map[string]any{
				"page": 2, "pageSize": 5, "totalCount": 6,
				"result": []any{map[string]any{"id": "template"}},
			}}},
		}}
		if err := runAgoalCoverage(t, ObjectTemplateList, caller, "--keyword", "target", "--page", "2", "--page-size", "5"); err != nil {
			t.Fatal(err)
		}
		want := []agoalCoverageCall{{tool: "list_obj_template", params: map[string]any{"keyword": "target", "page": 2, "pageSize": 5}}}
		if !reflect.DeepEqual(caller.calls, want) {
			t.Fatalf("calls = %#v, want %#v", caller.calls, want)
		}
	})

	t.Run("template rejects false pagination", func(t *testing.T) {
		caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"list_obj_template": {{"success": true, "content": map[string]any{
				"page": 1, "pageSize": 20, "totalCount": 0,
				"result": []any{map[string]any{"id": "impossible"}},
			}}},
		}}
		if err := runAgoalCoverage(t, ObjectTemplateList, caller); err == nil {
			t.Fatal("impossible pagination total was accepted")
		}
	})
}

func TestCrossPlatformCoverageAgoalResidualReads(t *testing.T) {
	t.Run("contract fields known and guaranteed zero", func(t *testing.T) {
		items := []map[string]any{{"id": "field-1", "code": "revenue", "title": "Revenue"}}
		if got := filterAgoalItems(items, "revenue", "id", "code", "title"); len(got) != 1 {
			t.Fatalf("known field projection = %#v", got)
		}
		if got := filterAgoalItems(items, "guaranteed-zero", "id", "code", "title"); len(got) != 0 {
			t.Fatalf("zero field projection = %#v", got)
		}
		caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"list_op_contract_fields": {{"success": true, "content": []any{items[0]}}},
		}}
		if err := runAgoalCoverage(t, ContractFields, caller, "--keyword", "revenue"); err != nil {
			t.Fatal(err)
		}
		want := []agoalCoverageCall{{tool: "list_op_contract_fields", params: map[string]any{}}}
		if !reflect.DeepEqual(caller.calls, want) {
			t.Fatalf("calls = %#v, want %#v", caller.calls, want)
		}
	})

	t.Run("user rules exact filter", func(t *testing.T) {
		caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"get_user_rules": {{"success": true, "content": map[string]any{
				"rules": []any{map[string]any{"id": "rule-1", "ruleName": "Rule"}},
			}}},
		}}
		if err := runAgoalCoverage(t, UserRules, caller, "--user-id", "user", "--rule-id", "rule-1"); err != nil {
			t.Fatal(err)
		}
		want := []agoalCoverageCall{{tool: "get_user_rules", params: map[string]any{"dingUserId": "user"}}}
		if !reflect.DeepEqual(caller.calls, want) {
			t.Fatalf("calls = %#v, want %#v", caller.calls, want)
		}
		zero := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"get_user_rules": {{"success": true, "content": map[string]any{
				"rules": []any{map[string]any{"id": "rule-1"}},
			}}},
		}}
		if err := runAgoalCoverage(t, UserRules, zero, "--rule-id", "guaranteed-zero"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("report submission page and privacy identity", func(t *testing.T) {
		caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"get_submit_detail": {{"success": true, "content": map[string]any{
				"page": 1, "pageSize": 20, "totalCount": 1,
				"result": []any{map[string]any{"reportId": nil, "user": map[string]any{"dingUserId": "user"}}},
			}}},
		}}
		if err := runAgoalCoverage(t, ReportSubmitDetail,
			caller, "--template-id", "template", "--submit-state", "NOT_SUBMITTED", "--page", "1", "--page-size", "20"); err != nil {
			t.Fatal(err)
		}
		want := []agoalCoverageCall{{tool: "get_submit_detail", params: map[string]any{
			"templateId": "template", "submitState": "NOT_SUBMITTED", "page": 1, "pageSize": 20,
		}}}
		if !reflect.DeepEqual(caller.calls, want) {
			t.Fatalf("calls = %#v, want %#v", caller.calls, want)
		}
	})

	t.Run("report keyword scans then filters locally", func(t *testing.T) {
		items := []map[string]any{
			{"reportId": "report-1", "user": map[string]any{"dingUserId": "user-1", "name": "Known Person"}},
			{"reportId": "report-2", "user": map[string]any{"dingUserId": "user-2", "name": "Another Person"}},
		}
		if got := filterAgoalSubmissions(items, "known"); len(got) != 1 {
			t.Fatalf("known submission projection = %#v", got)
		}
		if got := filterAgoalSubmissions(items, "guaranteed-zero"); len(got) != 0 {
			t.Fatalf("zero submission projection = %#v", got)
		}
		caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"get_submit_detail": {{"success": true, "content": map[string]any{
				"page": 1, "pageSize": agoalSubmissionReadPageSize, "totalCount": 2,
				"result": []any{items[0], items[1]},
			}}},
		}}
		if err := runAgoalCoverage(t, ReportSubmitDetail,
			caller, "--template-id", "template", "--submit-state", "NOT_SUBMITTED", "--keyword", "guaranteed-zero", "--page", "1", "--page-size", "20"); err != nil {
			t.Fatal(err)
		}
		want := []agoalCoverageCall{{tool: "get_submit_detail", params: map[string]any{
			"templateId": "template", "submitState": "NOT_SUBMITTED", "page": 1, "pageSize": agoalSubmissionReadPageSize,
		}}}
		if !reflect.DeepEqual(caller.calls, want) {
			t.Fatalf("calls = %#v, want %#v", caller.calls, want)
		}
	})

	t.Run("report keyword scan rejects duplicate pages", func(t *testing.T) {
		item := map[string]any{"reportId": "report-1", "user": map[string]any{"id": "user-1", "name": "Known"}}
		caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"get_submit_detail": {
				{"success": true, "content": map[string]any{"page": 1, "pageSize": agoalSubmissionReadPageSize, "totalCount": 2, "result": []any{item}}},
				{"success": true, "content": map[string]any{"page": 2, "pageSize": agoalSubmissionReadPageSize, "totalCount": 2, "result": []any{item}}},
			},
		}}
		if err := runAgoalCoverage(t, ReportSubmitDetail,
			caller, "--template-id", "template", "--submit-state", "NOT_SUBMITTED", "--keyword", "known"); err == nil {
			t.Fatal("duplicate submission pagination was accepted")
		}
	})

	t.Run("report keyword scan rejects changing total", func(t *testing.T) {
		caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"get_submit_detail": {
				{"success": true, "content": map[string]any{"page": 1, "pageSize": agoalSubmissionReadPageSize, "totalCount": 2, "result": []any{map[string]any{"reportId": "report-1", "user": map[string]any{"id": "user-1", "name": "Known"}}}}},
				{"success": true, "content": map[string]any{"page": 2, "pageSize": agoalSubmissionReadPageSize, "totalCount": 3, "result": []any{map[string]any{"reportId": "report-2", "user": map[string]any{"id": "user-2", "name": "Known"}}}}},
			},
		}}
		if err := runAgoalCoverage(t, ReportSubmitDetail,
			caller, "--template-id", "template", "--submit-state", "NOT_SUBMITTED", "--keyword", "known"); err == nil {
			t.Fatal("changing pagination total was accepted")
		}
	})

	t.Run("report keyword scan rejects stalled page", func(t *testing.T) {
		caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
			"get_submit_detail": {
				{"success": true, "content": map[string]any{"page": 1, "pageSize": agoalSubmissionReadPageSize, "totalCount": 2, "result": []any{map[string]any{"reportId": "report-1", "user": map[string]any{"id": "user-1", "name": "Known"}}}}},
				{"success": true, "content": map[string]any{"page": 2, "pageSize": agoalSubmissionReadPageSize, "totalCount": 2, "result": []any{}}},
			},
		}}
		if err := runAgoalCoverage(t, ReportSubmitDetail,
			caller, "--template-id", "template", "--submit-state", "NOT_SUBMITTED", "--keyword", "known"); err == nil {
			t.Fatal("stalled pagination was accepted")
		}
	})

	t.Run("report projection drops unreviewed personnel fields", func(t *testing.T) {
		items, err := requireAgoalSubmissionList([]any{map[string]any{
			"reportId": "report", "unreviewed": "drop",
			"user": map[string]any{"id": "user", "name": "Known", "email": "drop"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if _, found := items[0]["unreviewed"]; found {
			t.Fatal("unreviewed submission field was projected")
		}
		user := items[0]["user"].(map[string]any)
		if _, found := user["email"]; found {
			t.Fatal("unreviewed personnel field was projected")
		}
	})

	for _, tc := range []struct {
		name    string
		content map[string]any
	}{
		{name: "missing stable user", content: map[string]any{"page": 1, "pageSize": 20, "totalCount": 1, "result": []any{map[string]any{"user": map[string]any{"name": "x"}}}}},
		{name: "page mismatch", content: map[string]any{"page": 2, "pageSize": 20, "totalCount": 0, "result": []any{}}},
		{name: "impossible total", content: map[string]any{"page": 1, "pageSize": 20, "totalCount": 0, "result": []any{map[string]any{"user": map[string]any{"id": "user"}}}}},
	} {
		t.Run("report rejects "+tc.name, func(t *testing.T) {
			caller := &agoalCoverageCaller{responses: map[string][]map[string]any{
				"get_submit_detail": {{"success": true, "content": tc.content}},
			}}
			if err := runAgoalCoverage(t, ReportSubmitDetail,
				caller, "--template-id", "template", "--submit-state", "ON_TIME", "--page", "1", "--page-size", "20"); err == nil {
				t.Fatal("malformed submission page was accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageAgoalInvalidInputMakesZeroCalls(t *testing.T) {
	for _, args := range [][]string{{"--page", "0"}, {"--page-size", "0"}, {"--page-size", "101"}} {
		caller := &agoalCoverageCaller{}
		err := runAgoalCoverage(t, ObjectTemplateList, caller, args...)
		var typed *apperrors.Error
		if !errors.As(err, &typed) {
			t.Fatalf("invalid args %v error = %#v", args, err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("invalid args %v called MCP: %#v", args, caller.calls)
		}
	}
	for _, args := range [][]string{
		{"--template-id", "template", "--submit-state", "ON_TIME", "--page", "0"},
		{"--template-id", "template", "--submit-state", "ON_TIME", "--page-size", "101"},
		{"--template-id", "template", "--submit-state", "ON_TIME", "--query-date", "not-a-date"},
	} {
		caller := &agoalCoverageCaller{}
		if err := runAgoalCoverage(t, ReportSubmitDetail, caller, args...); err == nil {
			t.Fatalf("invalid submission args %v were accepted", args)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("invalid submission args %v called MCP: %#v", args, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageAgoalReviewedInventory(t *testing.T) {
	total := 0
	public := 0
	unavailable := 0
	for _, item := range shortcut.All() {
		if item.Service != productAgoal {
			continue
		}
		total++
		if item.Availability == shortcut.AvailabilityUnavailable {
			unavailable++
			if !item.Hidden || shortcut.InPublicCatalog(item.Service, item.Command) {
				t.Errorf("unavailable %s must remain hidden/non-public", item.Command)
			}
			continue
		}
		if shortcut.InPublicCatalog(item.Service, item.Command) {
			public++
			if item.Hidden || item.Availability != shortcut.AvailabilityAvailable {
				t.Errorf("public %s visibility/availability = %v/%q", item.Command, item.Hidden, item.Availability)
			}
			if item.OutputRollout != output.RolloutUnifiedActive {
				t.Errorf("public %s rollout = %q", item.Command, item.OutputRollout)
			}
			if item.Contract.Result == nil || item.Contract.Interface == nil || strings.TrimSpace(item.Contract.Description) == "" {
				t.Errorf("public %s has incomplete Contract/Result", item.Command)
			}
			if item.Safety.Effect == "" || item.Safety.Risk == "" || item.Safety.Confirmation == "" || item.Safety.Idempotency == "" {
				t.Errorf("public %s has incomplete Safety", item.Command)
			}
		}
	}
	if total != 16 || public != 5 || unavailable != 11 {
		t.Fatalf("agoal inventory total/public/unavailable = %d/%d/%d, want 16/5/11", total, public, unavailable)
	}
}

func TestCrossPlatformCoverageAgoalUnavailableCommandsMakeZeroCalls(t *testing.T) {
	for _, spec := range unavailableAgoalSpecs {
		declaration, found := agoalRegistered(spec.command)
		if !found {
			t.Fatalf("registered unavailable command missing: %s", spec.command)
		}
		args := make([]string, 0)
		for _, flag := range declaration.Flags {
			if !flag.Required {
				continue
			}
			args = append(args, "--"+flag.Name)
			switch flag.Name {
			case "scope-type":
				args = append(args, "PERSONAL")
			case "tracking-period-type":
				args = append(args, "MONTHLY")
			case "submit-state":
				args = append(args, "ON_TIME")
			default:
				if flag.Type != shortcut.FlagBool {
					args = append(args, "fixture")
				}
			}
		}
		caller := &agoalCoverageCaller{}
		err := runAgoalCoverage(t, declaration, caller, args...)
		if err == nil || !strings.Contains(err.Error(), spec.reason) {
			t.Errorf("%s unavailable error = %v", spec.command, err)
		}
		if len(caller.calls) != 0 {
			t.Errorf("%s unavailable command called MCP: %#v", spec.command, caller.calls)
		}
	}
}
