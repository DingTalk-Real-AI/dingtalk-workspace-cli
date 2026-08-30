// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package hrbrain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type hrbrainCoverageCaller struct{ calls int }

func (caller *hrbrainCoverageCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	caller.calls++
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":true,"result":[]}`}}}, nil
}
func (*hrbrainCoverageCaller) Format() string { return "json" }
func (*hrbrainCoverageCaller) DryRun() bool   { return false }
func (*hrbrainCoverageCaller) Fields() string { return "" }
func (*hrbrainCoverageCaller) JQ() string     { return "" }

type hrbrainParityCaller struct {
	calls    int
	server   string
	tool     string
	params   map[string]any
	response string
	err      error
}

func (caller *hrbrainParityCaller) CallTool(_ context.Context, server, tool string, params map[string]any) (*edition.ToolResult, error) {
	caller.calls++
	caller.server = server
	caller.tool = tool
	caller.params = params
	if caller.err != nil {
		return nil, caller.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: caller.response}}}, nil
}
func (*hrbrainParityCaller) Format() string { return "json" }
func (*hrbrainParityCaller) DryRun() bool   { return false }
func (*hrbrainParityCaller) Fields() string { return "" }
func (*hrbrainParityCaller) JQ() string     { return "" }

func runHRbrainCoverage(t *testing.T, declaration shortcut.Shortcut, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	_, err := runHRbrainCoverageOutput(t, declaration, caller, args...)
	return err
}

func runHRbrainCoverageOutput(t *testing.T, declaration shortcut.Shortcut, caller edition.ToolCaller, args ...string) (string, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	service := &cobra.Command{Use: "hrbrain"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.SetArgs(append([]string{"hrbrain", declaration.Command}, args...))
	executed, err := root.ExecuteC()
	if err == nil {
		_, _, err = output.EmitStoredResult(executed)
	}
	return stdout.String(), err
}

func TestCrossPlatformCoverageHRbrainDeclarationsStayUnavailableAndTyped(t *testing.T) {
	declarations := []shortcut.Shortcut{
		ListPools, GetPool, ListPoolEmployees, ProfileMetadata, QueryProfile, ProfileLabels,
		ProfileCareer, ProfilePerformance, SearchEmployees, SearchEmployeesStructured, SearchFields,
	}
	if len(declarations) != 11 {
		t.Fatalf("declarations = %d", len(declarations))
	}
	for _, declaration := range declarations {
		if !declaration.Hidden || declaration.Availability != shortcut.AvailabilityUnavailable {
			t.Errorf("%s must remain hidden/unavailable", declaration.Command)
		}
		if declaration.OutputRollout != output.RolloutUnifiedActive || declaration.Contract.Empty() || declaration.Contract.Result == nil {
			t.Errorf("%s lacks unified Contract/Result", declaration.Command)
		}
		if strings.TrimSpace(declaration.Safety.Effect) == "" || declaration.Contract.Interface == nil || declaration.Contract.Interface.Availability != "unavailable" {
			t.Errorf("%s lacks unavailable interface and Safety", declaration.Command)
		}
	}
}

func TestCrossPlatformCoverageHRbrainListPoolsAcceptsReviewedLiveContentPage(t *testing.T) {
	caller := &hrbrainParityCaller{response: `{"success":true,"result":null,"content":{"currentPage":1,"pageSize":20,"pools":[],"totalCount":0}}`}
	stdout, err := runHRbrainCoverageOutput(t, ListPools, caller)
	if err != nil {
		t.Fatalf("list-pools live content page failed: %v", err)
	}
	if caller.calls != 1 || caller.server != "hrbrain" || caller.tool != "list_talent_pools" {
		t.Fatalf("list-pools atomic route = calls:%d server:%q tool:%q", caller.calls, caller.server, caller.tool)
	}
	wantParams := map[string]any{"currentPage": 1, "pageSize": 20}
	if !reflect.DeepEqual(caller.params, wantParams) {
		t.Fatalf("list-pools atomic params = %#v, want %#v", caller.params, wantParams)
	}
	var envelope struct {
		OK      bool   `json:"ok"`
		Outcome string `json:"outcome"`
		Data    struct {
			Count       int           `json:"count"`
			Items       []interface{} `json:"items"`
			CurrentPage int           `json:"currentPage"`
			PageSize    int           `json:"pageSize"`
			TotalCount  int           `json:"totalCount"`
			Complete    bool          `json:"complete"`
		} `json:"data"`
		Meta struct {
			Pagination struct {
				EndpointExhausted bool   `json:"endpoint_exhausted"`
				NextToken         string `json:"next_token"`
			} `json:"pagination"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode list-pools terminal receipt %q: %v", stdout, err)
	}
	if !envelope.OK || envelope.Outcome != "success" || envelope.Data.Count != 0 || len(envelope.Data.Items) != 0 ||
		envelope.Data.CurrentPage != 1 || envelope.Data.PageSize != 20 || envelope.Data.TotalCount != 0 || !envelope.Data.Complete ||
		!envelope.Meta.Pagination.EndpointExhausted || envelope.Meta.Pagination.NextToken != "" {
		t.Fatalf("list-pools terminal receipt = %#v", envelope)
	}
}

func TestCrossPlatformCoverageHRbrainBlockersAreClassifiedWithoutShortcutDefects(t *testing.T) {
	commands := []string{
		ListPools.Command, GetPool.Command, ListPoolEmployees.Command, ProfileMetadata.Command,
		QueryProfile.Command, ProfileLabels.Command, ProfileCareer.Command, ProfilePerformance.Command,
		SearchEmployees.Command, SearchEmployeesStructured.Command, SearchFields.Command,
	}
	counts := map[string]int{}
	for _, command := range commands {
		reason := hrbrainUnavailableReason(command)
		if strings.Contains(reason, "shortcut_defect") || !strings.Contains(reason, "classified=") {
			t.Fatalf("%s has unreviewed blocker reason %q", command, reason)
		}
		switch {
		case strings.Contains(reason, "classified="+hrbrainBlockerAdapterBusiness):
			counts[hrbrainBlockerAdapterBusiness]++
		case strings.Contains(reason, "classified="+hrbrainBlockerTenantFixture):
			counts[hrbrainBlockerTenantFixture]++
		default:
			t.Fatalf("%s has unknown blocker classification %q", command, reason)
		}
	}
	if counts[hrbrainBlockerAdapterBusiness] != 8 || counts[hrbrainBlockerTenantFixture] != 3 {
		t.Fatalf("blocker counts = %#v", counts)
	}
}

func TestCrossPlatformCoverageHRbrainExactParameterProjectionForAllElevenShortcuts(t *testing.T) {
	pageResponse := `{"success":true,"result":null,"content":{"pools":[{"poolCode":"pool"}],"currentPage":1,"pageSize":20,"totalCount":1}}`
	employeePageResponse := `{"success":true,"result":{"items":[{"workNo":"worker"}],"currentPage":1,"pageSize":20,"totalCount":1,"hasMore":false}}`
	queryItems := []any{map[string]any{"modelCode": "model", "fields": []any{"field"}}}
	structuredFields := []any{map[string]any{"label": "field", "value": "field"}}
	cases := []struct {
		name        string
		declaration shortcut.Shortcut
		args        []string
		tool        string
		params      map[string]any
		response    string
	}{
		{name: "list pools", declaration: ListPools, tool: "list_talent_pools", params: map[string]any{"currentPage": 1, "pageSize": 20}, response: pageResponse},
		{name: "get pool", declaration: GetPool, args: []string{"--pool-code", "pool"}, tool: "get_talent_pool_detail", params: map[string]any{"poolCode": "pool"}, response: `{"success":true,"result":{"poolCode":"pool"}}`},
		{name: "list pool employees", declaration: ListPoolEmployees, args: []string{"--pool-code", "pool"}, tool: "list_pool_employees", params: map[string]any{"poolCode": "pool", "currentPage": 1, "pageSize": 20}, response: employeePageResponse},
		{name: "profile metadata", declaration: ProfileMetadata, args: []string{"--work-no", "worker"}, tool: "get_profile_metadata", params: map[string]any{"workNo": "worker"}, response: `{"success":true,"result":[{"modelCode":"model"}]}`},
		{name: "query profile", declaration: QueryProfile, args: []string{"--work-no", "worker", "--data-queries", `[{"modelCode":"model","fields":["field"]}]`}, tool: "query_profile_data", params: map[string]any{"workNo": "worker", "dataQueries": queryItems}, response: `{"success":true,"result":[{"modelCode":"model"}]}`},
		{name: "profile labels", declaration: ProfileLabels, args: []string{"--staff-ids", "worker"}, tool: "get_profile_label", params: map[string]any{"staffIds": []string{"worker"}}, response: `{"success":true,"result":[{"workNo":"worker"}]}`},
		{name: "profile career", declaration: ProfileCareer, args: []string{"--work-no", "worker"}, tool: "get_employee_career", params: map[string]any{"workNo": "worker"}, response: `{"success":true,"result":[{"careerId":"career"}]}`},
		{name: "profile performance", declaration: ProfilePerformance, args: []string{"--work-no", "worker"}, tool: "get_employee_performance", params: map[string]any{"workNo": "worker"}, response: `{"success":true,"result":[{"performanceId":"performance"}]}`},
		{name: "search employees", declaration: SearchEmployees, args: []string{"--keyword", "worker"}, tool: "search_employees", params: map[string]any{"keyword": "worker", "currentPage": 1, "pageSize": 20}, response: employeePageResponse},
		{name: "search employees structured", declaration: SearchEmployeesStructured, args: []string{"--origin-json", `{"rules":[{"field":"field"}]}`, "--fields", `[{"label":"field","value":"field"}]`}, tool: "search_employees_structured", params: map[string]any{"originJson": `{"rules":[{"field":"field"}]}`, "fields": structuredFields, "currentPage": 1, "pageSize": 20}, response: employeePageResponse},
		{name: "search fields", declaration: SearchFields, tool: "get_search_fields", params: map[string]any{}, response: `{"success":true,"result":[{"fieldCode":"field"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &hrbrainParityCaller{response: tc.response}
			if err := runHRbrainCoverage(t, tc.declaration, caller, tc.args...); err != nil {
				t.Fatalf("exact Shortcut failed against valid synthetic response: %v", err)
			}
			if caller.calls != 1 || caller.server != "hrbrain" || caller.tool != tc.tool {
				t.Fatalf("remote route = calls:%d server:%q tool:%q, want 1/hrbrain/%s", caller.calls, caller.server, caller.tool, tc.tool)
			}
			if !reflect.DeepEqual(caller.params, tc.params) {
				t.Fatalf("params = %#v, want %#v", caller.params, tc.params)
			}
		})
	}
}

func TestCrossPlatformCoverageHRbrainRejectsNullWrongCollectionsAndBadItems(t *testing.T) {
	brokenCollections := []map[string]any{
		{},
		{"success": false, "result": []any{}},
		{"success": true, "result": nil},
		{"success": true, "result": map[string]any{}},
		{"success": true, "result": []any{"bad"}},
		{"success": true, "result": []any{map[string]any{"name": "missing-id"}}},
		{"success": true, "result": []any{map[string]any{"poolCode": "same"}, map[string]any{"poolCode": "same"}}},
	}
	for index, payload := range brokenCollections {
		if got, err := hrbrainRequireCollectionResult(payload, "hrbrain/test", "poolCode"); err == nil {
			t.Errorf("broken collection %d accepted: %#v", index, got)
		}
	}
	explicitEmpty := map[string]any{"success": true, "result": []any{}}
	if got, err := hrbrainRequireCollectionResult(explicitEmpty, "hrbrain/test", "poolCode"); err != nil || len(got) != 0 {
		t.Fatalf("explicit empty collection: got=%#v err=%v", got, err)
	}
}

func TestCrossPlatformCoverageHRbrainPaginationFailsClosed(t *testing.T) {
	valid := map[string]any{"success": true, "result": map[string]any{
		"items":       []any{map[string]any{"poolCode": "pool-1"}},
		"currentPage": float64(1), "pageSize": float64(1), "totalCount": float64(2), "hasMore": true,
	}}
	items, page, err := hrbrainProjectPage(valid, "hrbrain/test", 1, 1, "poolCode")
	if err != nil || len(items) != 1 || !page.HasMore {
		t.Fatalf("valid page: items=%#v page=%+v err=%v", items, page, err)
	}
	broken := []map[string]any{
		{"success": true, "result": map[string]any{"items": []any{}}},
		{"success": true, "result": map[string]any{"items": "bad", "currentPage": float64(1), "pageSize": float64(20), "totalCount": float64(0), "hasMore": false}},
		{"success": true, "result": map[string]any{"items": []any{}, "currentPage": float64(2), "pageSize": float64(20), "totalCount": float64(0), "hasMore": false}},
		{"success": true, "result": map[string]any{"items": []any{}, "currentPage": float64(1), "pageSize": float64(20), "totalCount": float64(21), "hasMore": false}},
		{"success": true, "result": map[string]any{"items": []any{}, "currentPage": float64(1), "pageSize": float64(20), "totalCount": float64(20), "hasMore": true}},
		{"success": true, "result": map[string]any{"items": []any{}, "currentPage": float64(1), "pageSize": float64(20), "totalCount": float64(0), "hasMore": "false"}},
	}
	for index, payload := range broken {
		if got, evidence, err := hrbrainProjectPage(payload, "hrbrain/test", 1, 20, "poolCode"); err == nil {
			t.Errorf("broken page %d accepted: items=%#v evidence=%+v", index, got, evidence)
		}
	}

	liveContent := map[string]any{"success": true, "result": nil, "content": map[string]any{
		"pools": []any{}, "currentPage": float64(1), "pageSize": float64(20), "totalCount": float64(0),
	}}
	items, page, err = hrbrainProjectPoolsPage(liveContent, "hrbrain/list_talent_pools", 1, 20)
	if err != nil || len(items) != 0 || page.HasMore || page.TotalCount != 0 {
		t.Fatalf("live content page: items=%#v page=%+v err=%v", items, page, err)
	}
	items, page, err = hrbrainProjectPoolsPage(valid, "hrbrain/list_talent_pools", 1, 1)
	if err != nil || len(items) != 1 || !page.HasMore {
		t.Fatalf("legacy result page: items=%#v page=%+v err=%v", items, page, err)
	}
	brokenContent := []map[string]any{
		{"result": nil, "content": map[string]any{"pools": []any{}}},
		{"success": false, "result": nil, "content": map[string]any{"pools": []any{}}},
		{"success": true, "result": nil},
		{"success": true, "result": nil, "content": map[string]any{"currentPage": float64(1), "pageSize": float64(20), "totalCount": float64(0)}},
		{"success": true, "result": nil, "content": map[string]any{"pools": "bad", "currentPage": float64(1), "pageSize": float64(20), "totalCount": float64(0)}},
		{"success": true, "result": nil, "content": map[string]any{"pools": []any{map[string]any{}}, "currentPage": float64(1), "pageSize": float64(20), "totalCount": float64(1)}},
		{"success": true, "result": nil, "content": map[string]any{"pools": []any{}, "currentPage": float64(2), "pageSize": float64(20), "totalCount": float64(0)}},
		{"success": true, "result": nil, "content": map[string]any{"pools": []any{}, "currentPage": float64(1), "pageSize": float64(20), "totalCount": float64(-1)}},
	}
	for index, payload := range brokenContent {
		if got, evidence, err := hrbrainProjectPoolsPage(payload, "hrbrain/list_talent_pools", 1, 20); err == nil {
			t.Errorf("broken live content %d accepted: items=%#v evidence=%+v", index, got, evidence)
		}
	}
}

func TestCrossPlatformCoverageHRbrainStructuredInputsRejectBadShapes(t *testing.T) {
	for _, raw := range []string{"", "{}", "[]", "[1]", "[{}]"} {
		if _, err := hrbrainJSONArray(raw, "--items"); err == nil {
			t.Errorf("bad array accepted: %q", raw)
		}
	}
	for _, raw := range []string{"", "[]", "{}"} {
		if _, err := hrbrainJSONObject(raw, "--object"); err == nil {
			t.Errorf("bad object accepted: %q", raw)
		}
	}
}

func TestCrossPlatformCoverageHRbrainInvalidInputMakesZeroRemoteCalls(t *testing.T) {
	for name, tc := range map[string]struct {
		declaration shortcut.Shortcut
		args        []string
	}{
		"unfiltered employee search": {declaration: SearchEmployees},
		"invalid page":               {declaration: ListPools, args: []string{"--page", "0"}},
		"invalid page size":          {declaration: ListPoolEmployees, args: []string{"--pool-code", "fixture", "--page-size", "101"}},
		"bad query json":             {declaration: QueryProfile, args: []string{"--work-no", "fixture", "--data-queries", `{}`}},
		"bad structured json":        {declaration: SearchEmployeesStructured, args: []string{"--origin-json", `{}`, "--fields", `[]`}},
	} {
		t.Run(name, func(t *testing.T) {
			caller := &hrbrainCoverageCaller{}
			if err := runHRbrainCoverage(t, tc.declaration, caller, tc.args...); err == nil {
				t.Fatal("invalid input unexpectedly succeeded")
			}
			if caller.calls != 0 {
				t.Fatalf("remote calls before validation = %d, want 0", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageHRbrainExecutionFailuresAndContinuation(t *testing.T) {
	for name, tc := range map[string]struct {
		declaration shortcut.Shortcut
		args        []string
		response    string
		err         error
	}{
		"object transport":     {declaration: GetPool, args: []string{"--pool-code", "pool"}, err: errors.New("transport")},
		"object malformed":     {declaration: GetPool, args: []string{"--pool-code", "pool"}, response: `{"success":true,"result":null}`},
		"object missing id":    {declaration: GetPool, args: []string{"--pool-code", "pool"}, response: `{"success":true,"result":{"name":"pool"}}`},
		"object wrong id":      {declaration: GetPool, args: []string{"--pool-code", "pool"}, response: `{"success":true,"result":{"poolCode":"other"}}`},
		"collection transport": {declaration: ProfileLabels, args: []string{"--staff-ids", "worker"}, err: errors.New("transport")},
		"collection malformed": {declaration: ProfileLabels, args: []string{"--staff-ids", "worker"}, response: `{"success":true,"result":null}`},
		"pools transport":      {declaration: ListPools, err: errors.New("transport")},
		"page transport":       {declaration: SearchEmployees, args: []string{"--keyword", "worker"}, err: errors.New("transport")},
		"page malformed":       {declaration: SearchEmployees, args: []string{"--keyword", "worker"}, response: `{"success":true,"result":null}`},
	} {
		t.Run(name, func(t *testing.T) {
			caller := &hrbrainParityCaller{response: tc.response, err: tc.err}
			if err := runHRbrainCoverage(t, tc.declaration, caller, tc.args...); err == nil {
				t.Fatal("invalid remote result succeeded")
			}
			if caller.calls != 1 {
				t.Fatalf("calls = %d", caller.calls)
			}
		})
	}

	continuing := &hrbrainParityCaller{response: `{"success":true,"result":{"items":[{"workNo":"worker"}],"currentPage":1,"pageSize":1,"totalCount":2,"hasMore":true}}`}
	if err := runHRbrainCoverage(t, SearchEmployees, continuing, "--keyword", "worker", "--page-size", "1"); err != nil {
		t.Fatal(err)
	}
	if continuing.calls != 1 {
		t.Fatalf("continuing calls = %d", continuing.calls)
	}

	optionalPool := &hrbrainParityCaller{response: `{"success":true,"result":null,"content":{"pools":[{"poolCode":"pool"}],"currentPage":1,"pageSize":20,"totalCount":1}}`}
	if err := runHRbrainCoverage(t, ListPools, optionalPool,
		"--keyword", "pool", "--pool-type", "type", "--creator", "creator", "--labels", "one,two"); err != nil {
		t.Fatal(err)
	}
	if labels, ok := optionalPool.params["labels"].([]string); !ok || len(labels) != 2 {
		t.Fatalf("labels = %#v", optionalPool.params["labels"])
	}

	labelsCaller := &hrbrainParityCaller{response: `{"success":true,"result":[{"workNo":"worker"}]}`}
	if err := runHRbrainCoverage(t, ProfileLabels, labelsCaller, "--staff-ids", "worker", "--all-label"); err != nil {
		t.Fatal(err)
	}
	if labelsCaller.params["allLabel"] != true {
		t.Fatalf("allLabel = %#v", labelsCaller.params["allLabel"])
	}

	structured := &hrbrainParityCaller{response: `{"success":true,"result":{"items":[{"workNo":"worker"}],"currentPage":1,"pageSize":20,"totalCount":1,"hasMore":false}}`}
	if err := runHRbrainCoverage(t, SearchEmployeesStructured, structured,
		"--origin-json", `{"rules":[{"field":"field"}]}`, "--fields", `[{"label":"field","value":"field"}]`, "--order-by", "field"); err != nil {
		t.Fatal(err)
	}
	if order, ok := structured.params["orderByClauses"].([]string); !ok || len(order) != 1 {
		t.Fatalf("orderByClauses = %#v", structured.params["orderByClauses"])
	}
}

func TestCrossPlatformCoverageHRbrainRemainingValidationAndScalarEdges(t *testing.T) {
	t.Run("unknown blocker", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("unknown blocker classification did not panic")
			}
		}()
		_ = hrbrainUnavailableReason("+unknown")
	})

	profileLabelsCommand := corecmd.New(shortcut.FromShortcut(ProfileLabels))
	if err := profileLabelsCommand.Flags().Set("staff-ids", ","); err != nil {
		t.Fatal(err)
	}
	if err := ProfileLabels.Validate(shortcut.RuntimeContextForTest(profileLabelsCommand, ProfileLabels)); err == nil {
		t.Fatal("blank staff IDs succeeded")
	}
	badPage := &hrbrainCoverageCaller{}
	if err := runHRbrainCoverage(t, SearchEmployeesStructured, badPage,
		"--origin-json", `{"rule":"value"}`, "--fields", `[{"field":"value"}]`, "--page", "0"); err == nil {
		t.Fatal("structured search invalid page succeeded")
	}
	if badPage.calls != 0 {
		t.Fatalf("structured invalid page remote calls = %d", badPage.calls)
	}

	if got := hrbrainIdentity(map[string]any{"id": float64(7)}, "id"); got != "7" {
		t.Fatalf("numeric identity = %q", got)
	}
	for _, value := range []any{math.NaN(), math.Inf(1), 1.5} {
		if got := hrbrainIdentity(map[string]any{"id": value}, "id"); got != "" {
			t.Fatalf("invalid identity = %q", got)
		}
	}
	if got, ok := hrbrainInteger(7); !ok || got != 7 {
		t.Fatalf("integer = %d, %v", got, ok)
	}
	for _, value := range []any{math.NaN(), math.Inf(1), 1.5, float64(math.MaxInt) * 2, "7"} {
		if _, ok := hrbrainInteger(value); ok {
			t.Fatalf("invalid integer accepted: %#v", value)
		}
	}

	t.Run("marshal panic", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("mustJSON did not panic for unsupported value")
			}
		}()
		_ = mustJSON(make(chan int))
	})
}

func TestCrossPlatformCoverageHRbrainPageSizeAndTotalContradictions(t *testing.T) {
	missing := map[string]any{"success": true, "result": map[string]any{}}
	if _, _, err := hrbrainProjectPage(missing, "hrbrain/test", 1, 1, "workNo"); err == nil {
		t.Fatal("missing page collection succeeded")
	}
	badItem := map[string]any{"success": true, "result": map[string]any{
		"items": []any{"bad"}, "currentPage": float64(1), "pageSize": float64(1), "totalCount": float64(1), "hasMore": false,
	}}
	if _, _, err := hrbrainProjectPage(badItem, "hrbrain/test", 1, 1, "workNo"); err == nil {
		t.Fatal("bad page item succeeded")
	}
	tooMany := map[string]any{"success": true, "result": map[string]any{
		"items":       []any{map[string]any{"workNo": "one"}, map[string]any{"workNo": "two"}},
		"currentPage": float64(1), "pageSize": float64(1), "totalCount": float64(2), "hasMore": true,
	}}
	if _, _, err := hrbrainProjectPage(tooMany, "hrbrain/test", 1, 1, "workNo"); err == nil {
		t.Fatal("page larger than pageSize succeeded")
	}
	badTotal := map[string]any{"success": true, "result": map[string]any{
		"items":       []any{map[string]any{"workNo": "one"}},
		"currentPage": float64(1), "pageSize": float64(1), "totalCount": float64(0), "hasMore": false,
	}}
	if _, _, err := hrbrainProjectPage(badTotal, "hrbrain/test", 1, 1, "workNo"); err == nil {
		t.Fatal("total below item count succeeded")
	}
}
