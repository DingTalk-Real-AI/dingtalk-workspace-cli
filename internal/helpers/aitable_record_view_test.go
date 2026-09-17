// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"reflect"
	"strings"
	"testing"
)

type parityViewCaller struct {
	payload string
	calls   int
}

func (c *parityViewCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls++
	if server != "aitable" || tool != "get_views" {
		panic("unexpected call")
	}
	return textToolResult(c.payload), nil
}
func (*parityViewCaller) Format() string { return "json" }
func (*parityViewCaller) DryRun() bool   { return false }
func (*parityViewCaller) Fields() string { return "" }
func (*parityViewCaller) JQ() string     { return "" }
func TestCrossPlatformCoverageAITableRecordViewIdentityAndFilter(t *testing.T) {
	c := &parityViewCaller{payload: `{"data":{"views":[{"viewId":"unrelated"},{"viewId":"v","viewType":"Grid","baseId":"b","tableId":"t","filter":{"operator":"and","operands":[{"operator":"eq","operands":["f","A"]}]},"sort":[{"fieldId":"n","direction":"desc"}]}]}}`}
	InitDepsForTest(t, c)
	p := map[string]any{"baseId": "b", "tableId": "t"}
	got, err := AITableQueryWithView(context.Background(), p, "v", false)
	if err != nil || c.calls != 1 || got["filters"] == nil || got["sort"] == nil {
		t.Fatal(got, err)
	}
	if _, ok := got["viewId"]; ok {
		t.Fatal("unsupported viewId forwarded")
	}
	if len(p) != 2 {
		t.Fatal("mutated input")
	}
	explicit := map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "gt", "operands": []any{"n", float64(10)}}}}
	p["filters"] = explicit
	got, err = AITableQueryWithView(context.Background(), p, "v", false)
	if err != nil || !reflect.DeepEqual(got["filters"], explicit) {
		t.Fatal(got, err)
	}
	got, err = AITableQueryWithView(context.Background(), p, "v", true)
	if err != nil || len(got["filters"].(map[string]any)["operands"].([]any)) != 2 {
		t.Fatal(got, err)
	}
	p["recordIds"] = []string{"r"}
	before := c.calls
	if _, err = AITableQueryWithView(context.Background(), p, "v", true); err == nil || c.calls != before {
		t.Fatal("ID selector combined unsafely")
	}
}
func TestCrossPlatformCoverageAITableRecordViewRejectsUnknown(t *testing.T) {
	for _, payload := range []string{`null`, `{"success":true}`, `{"views":{}}`, `{"views":[null]}`, `{"views":[{}]}`, `{"views":[{"viewId":"other"}]}`, `{"views":[{"viewId":"v"},{"viewId":"v"}]}`, `{"views":[{"viewId":"v","baseId":"wrong"}]}`, `{"views":[{"viewId":"v"}]}`, `{"views":[{"viewId":"v","filter":[]}]}`} {
		c := &parityViewCaller{payload: payload}
		InitDepsForTest(t, c)
		if _, err := AITableQueryWithView(context.Background(), map[string]any{"baseId": "b", "tableId": "t"}, "v", false); err == nil {
			t.Fatal(payload)
		}
	}
}
func TestCrossPlatformCoverageAITableRecordViewFilterShapes(t *testing.T) {
	for _, raw := range []string{`[]`, `{"operator":"and","operands":[]}`, `[{"operator":"eq","operands":["f","x"]}]`, `[{"operator":"and","operands":[{"operator":"exist","operands":["f"]}]}]`, `{"operator":"or","operands":[{"operator":"date_eq","operands":["f",{"type":"exact","timestamp":123}]}]}`} {
		var v any
		_ = json.Unmarshal([]byte(raw), &v)
		if _, err := compileAITableViewFilter(v); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{`null`, `{}`, `{"operator":"and","operands":null}`, `{"operator":"and","operands":[null]}`, `{"operator":"and","operands":[{"operator":"bogus","operands":[]}]}`, `{"operator":"and","operands":[{"operator":"eq","operands":["f"]}]}`, `{"operator":"and","operands":[{"operator":"eq","operands":[null,"x"]}]}`, `{"operator":"and","operands":[{"operator":"date_eq","operands":["f",{"type":"relative","period":"day","offset":0}]}]}`} {
		var v any
		_ = json.Unmarshal([]byte(raw), &v)
		if _, err := compileAITableViewFilter(v); err == nil {
			t.Fatal(raw)
		}
	}
}

func TestCrossPlatformCoverageAITableRecordViewNeverTreatsFormAsAllRows(t *testing.T) {
	for _, kind := range []string{"FormDesigner", "unknown", ""} {
		b, _ := json.Marshal(map[string]any{"views": []any{map[string]any{"viewId": "v", "viewType": kind, "filter": []any{}, "sort": []any{}}}})
		c := &parityViewCaller{payload: string(b)}
		InitDepsForTest(t, c)
		if _, err := AITableQueryWithView(context.Background(), map[string]any{"baseId": "b", "tableId": "t"}, "v", true); err == nil || c.calls != 1 {
			t.Fatal(kind, err, c.calls)
		}
	}
}

func TestCrossPlatformCoverageAITableViewSortValidation(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{`[{"fieldId":"f"}]`, "ASC"}, {`[{"fieldId":"f","desc":false}]`, "ASC"}, {`[{"fieldId":"f","desc":true}]`, "DESC"}, {`[{"fieldId":"f","direction":"desc"}]`, "DESC"},
	} {
		var v any
		_ = json.Unmarshal([]byte(tc.raw), &v)
		got, err := compileAITableViewSort(v)
		if err != nil || len(got) != 1 || got[0].(map[string]any)["direction"] != tc.want {
			t.Fatal(tc, got, err)
		}
	}
	for _, raw := range []string{`null`, `[null]`, `[{}]`, `[{"fieldId":"f","direction":1}]`, `[{"fieldId":"f","direction":"sideways"}]`, `[{"fieldId":"f","desc":"yes"}]`} {
		var v any
		_ = json.Unmarshal([]byte(raw), &v)
		if _, err := compileAITableViewSort(v); err == nil {
			t.Fatal(raw)
		}
	}
}
func TestCrossPlatformCoverageAITableViewScopeFailures(t *testing.T) {
	for _, tc := range []struct {
		view     string
		params   map[string]any
		restrict bool
	}{
		{`{"viewId":"v","viewType":"Grid","sort":[]}`, nil, false},
		{`{"viewId":"v","viewType":"Grid","filter":[]}`, nil, false},
		{`{"viewId":"v","viewType":"Grid","filter":{},"sort":[]}`, nil, false},
		{`{"viewId":"v","viewType":"Grid","filter":[],"sort":null}`, nil, false},
		{`{"viewId":"v","viewType":"Grid","filter":[{"operator":"eq","operands":["f","a"]}],"sort":[]}`, map[string]any{"filters": "bad"}, true},
		{`{"viewId":"v","viewType":"Grid","filter":[{"operator":"eq","operands":["f","a"]}],"sort":[]}`, map[string]any{"filters": map[string]any{"operator": "or", "operands": []any{}}}, true},
		{`{"viewId":"v","viewType":"Grid","filter":[{"operator":"eq","operands":["f","a"]}],"sort":[]}`, map[string]any{"filters": map[string]any{"operator": "and", "operands": nil}}, true},
	} {
		c := &parityViewCaller{payload: `{"result":{"views":[` + tc.view + `]}}`}
		InitDepsForTest(t, c)
		if _, err := AITableQueryWithView(context.Background(), tc.params, "v", tc.restrict); err == nil {
			t.Fatal(tc.view, tc.params)
		}
	}
	c := &parityViewCaller{}
	InitDepsForTest(t, c)
	if _, err := AITableQueryWithView(context.Background(), nil, " ", false); err == nil || c.calls != 0 {
		t.Fatal(err, c.calls)
	}
	var multi any
	_ = json.Unmarshal([]byte(`[{"operator":"exist","operands":["f"]},{"operator":"un_exist","operands":["g"]}]`), &multi)
	if got, err := compileAITableViewFilter(multi); err != nil || len(got["operands"].([]any)) != 2 {
		t.Fatal(got, err)
	}
}

func TestCrossPlatformCoverageAITableViewReadFailureAndAtomicCompleteness(t *testing.T) {
	c := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{err: fmt.Errorf("offline")}}}
	InitDepsForTest(t, c)
	if _, err := AITableQueryWithView(context.Background(), nil, "v", false); err == nil {
		t.Fatal("ignored MCP error")
	}
	for _, raw := range []string{`{"records":[null],"hasMore":false}`, `{"records":[{}],"hasMore":false}`, `{"records":[{"recordId":"r"},{"recordId":"r"}],"hasMore":false}`} {
		c := &recordQueryE2ECaller{steps: []recordQueryE2EStep{recordQueryTextStep(raw)}}
		if out, err := runRecordQueryCLI(t, c); err == nil || strings.Contains(out, `"complete": true`) {
			t.Fatal(out, err)
		}
	}
	c = &recordQueryE2ECaller{dryRun: true}
	out, err := runRecordQueryCLI(t, c, "--view-id", "v")
	if err != nil || len(c.calls) != 0 || !strings.Contains(out, "compile filter/sort") {
		t.Fatal(out, err)
	}
}
