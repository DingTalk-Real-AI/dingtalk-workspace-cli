// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageURLResolveObjectVerification(t *testing.T) {
	cases := []struct {
		query, body, tool, server, kind, id string
		args                                map[string]any
	}{
		{"", `{"baseId":"b","tables":[]}`, "get_base", serverMain, "base", "b", map[string]any{"baseId": "b"}},
		{"sheetId=s", `{"tables":[{"tableId":"s"}]}`, "get_tables", serverMain, "table", "s", map[string]any{"baseId": "b", "tableIds": []string{"s"}}},
		{"sheetId=s&viewId=v", `{"views":[{"viewId":"v"}]}`, "get_views", serverMain, "view", "v", map[string]any{"baseId": "b", "tableId": "s", "viewIds": []string{"v"}}},
		{"entrance=form&sheetId=s&viewId=v", `{"views":[{"viewId":"v","type":"FormDesigner"}]}`, "get_views", serverMain, "form", "v", map[string]any{"baseId": "b", "tableId": "s", "viewIds": []string{"v"}}},
		{"sheetId=s&fieldId=f", `{"fields":[{"fieldId":"f","fieldName":"字段"}]}`, "get_fields", serverMain, "field", "f", map[string]any{"baseId": "b", "tableId": "s", "fieldIds": []string{"f"}}},
		{"sheetId=s&fieldId=f", `{"baseId":"b","result":{"tableId":"s","data":{"fields":[{"fieldId":"f"}]}}}`, "get_fields", serverMain, "field", "f", map[string]any{"baseId": "b", "tableId": "s", "fieldIds": []string{"f"}}},
		{"record=s__r", `{"records":[{"recordId":"r"}]}`, "query_records", serverMain, "record", "r", map[string]any{"baseId": "b", "tableId": "s", "recordIds": []string{"r"}}},
		{"entrance=apps&appId=a&pageId=p", `{"pageId":"p","appId":"a"}`, "get_app_page", serverMain, "app_page", "p", map[string]any{"baseId": "b", "pageId": "p"}},
		{"entrance=automation&flowId=f&autoOpenAutomation=true", `{"flowId":"f","flowSchema":{}}`, "get_workflow", serverHelper, "workflow", "f", map[string]any{"baseId": "b", "workflowId": "f"}},
		{"flowId=123", `{"flowId":123}`, "get_workflow", serverHelper, "workflow", "123", map[string]any{"baseId": "b", "workflowId": "123"}},
		{"applicationId=d", `{"items":[{"nodeId":"d","nodeType":"dashboard"}],"total":1}`, "list_nsheet_nodes", serverHelper, "dashboard", "d", map[string]any{"baseId": "b"}},
		{"applicationId=doc", `{"items":[{"nodeId":"doc","nodeType":"document"}],"total":1}`, "list_nsheet_nodes", serverHelper, "document", "doc", map[string]any{"baseId": "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			caller := &urlResolveE2ECaller{results: []*edition.ToolResult{urlResult(`{"data":` + tc.body + `}`)}}
			out, err := runURLResolveCLI(t, caller, "--url", "https://alidocs.dingtalk.com/i/nodes/b?"+tc.query, "--verify")
			if err != nil {
				t.Fatal(err, out)
			}
			if len(caller.calls) != 1 || caller.calls[0].product != tc.server || caller.calls[0].tool != tc.tool || !reflect.DeepEqual(caller.calls[0].args, tc.args) {
				t.Fatalf("calls=%#v", caller.calls)
			}
			for _, want := range []string{`"verified": true`, `"entityType": "` + tc.kind + `"`, `"id": "` + tc.id + `"`} {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %s: %s", want, out)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageURLResolveVerifiesOptionalView(t *testing.T) {
	for _, query := range []string{"sheetId=s&fieldId=f&viewId=v", "record=s_v_r"} {
		for _, validView := range []bool{true, false} {
			t.Run(query+"/"+map[bool]string{true: "exists", false: "missing"}[validView], func(t *testing.T) {
				body := `{"fields":[{"fieldId":"f"}]}`
				if strings.HasPrefix(query, "record") {
					body = `{"records":[{"recordId":"r"}]}`
				}
				view := `{"views":[]}`
				if validView {
					view = `{"views":[{"viewId":"v","tableId":"s"}]}`
				}
				caller := &urlResolveE2ECaller{results: []*edition.ToolResult{urlResult(body), urlResult(view)}}
				out, err := runURLResolveCLI(t, caller, "--url", "https://alidocs.dingtalk.com/i/nodes/b?"+query, "--verify")
				if (err == nil) != validView || len(caller.calls) != 2 || caller.calls[1].tool != "get_views" {
					t.Fatal(out, err, caller.calls)
				}
				if validView && !strings.Contains(out, "+get_views") {
					t.Fatal(out)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageURLResolveRejectsUnprovenObjects(t *testing.T) {
	cases := []struct{ query, body string }{
		{"sheetId=s&fieldId=f", `{"fields":[]}`},
		{"sheetId=s&fieldId=f", `{"fields":[{"fieldId":"f","tableId":"other"}]}`},
		{"sheetId=s&fieldId=f", `{"base_id":"other","fields":[{"fieldId":"f"}]}`},
		{"sheetId=s&fieldId=f", `{"fields":[{"fieldId":"f","table_id":"other"}]}`},
		{"sheetId=s&fieldId=f", `{"baseId":"other","data":{"fields":[{"fieldId":"f"}]}}`},
		{"sheetId=s&fieldId=f", `{"tableId":"other","result":{"fields":[{"fieldId":"f"}]}}`},
		{"sheetId=s&fieldId=f", `{"data":{"sheetId":"other","result":{"fields":[{"fieldId":"f"}]}}}`},
		{"sheetId=s&viewId=v", `{"request":{"viewId":"v"},"views":[]}`},
		{"sheetId=s&viewId=v", `{"views":[{"viewId":"v","id":"other"}]}`},
		{"sheetId=s&viewId=v", `{"views":[{"viewId":"v"},{"viewId":"v"}]}`},
		{"sheetId=s&viewId=v", `{"views":"bad"}`},
		{"sheetId=s&viewId=v", `{"views":["bad"]}`},
		{"entrance=form&sheetId=s&viewId=v", `{"views":[{"viewId":"v","type":"Grid"}]}`},
		{"entrance=form&sheetId=s&viewId=v", `{"views":[{"viewId":"v"}]}`},
		{"appId=a&pageId=p", `{"pageId":"p","appId":"other"}`},
		{"appId=a&pageId=p", `{"pageId":"p"}`},
		{"appId=a&pageId=p", `{"appId":"other","data":{"pageId":"p","appId":"a"}}`},
		{"flowId=1.5", `{"flowId":1.5}`},
		{"flowId=f", `{"request":{"flowId":"f"}}`},
		{"flowId=9007199254740993", `{"flowId":9007199254740993}`},
		{"applicationId=d", `{"dashboards":[{"dashboardId":"other"}],"request":{"documentId":"d"}}`},
		{"applicationId=d", `{"dashboards":[{"dashboardId":"d"}],"documents":[{"documentId":"d"}]}`},
		{"applicationId=d", `{"dashboards":[{"dashboardId":"d"},{"dashboardId":"d"}],"documents":[{"documentId":"d"}]}`},
		{"applicationId=d", `{"dashboards":"bad","documents":[{"documentId":"d"}]}`},
		{"applicationId=d", `{"dashboards":[true],"documents":[{"documentId":"d"}]}`},
		{"applicationId=d", `{"baseId":"other","documents":[{"documentId":"d"}]}`},
		{"applicationId=d", `{"items":[{"nodeId":"d","nodeType":"document"},{"nodeId":"d","nodeType":"dashboard"}]}`},
		{"applicationId=d", `{"items":[{"nodeId":"d","nodeType":"document"},{"nodeId":"d","nodeType":"document"}]}`},
		{"applicationId=d", `{"items":[{"nodeId":"d","nodeType":"section"}]}`},
		{"applicationId=d", `{"items":[{"nodeId":"d"}]}`},
		{"applicationId=d", `{"items":[{"nodeId":"d","nodeType":1}]}`},
		{"applicationId=d", `{"items":[{"nodeId":"other","nodeType":"document"}]}`},
		{"applicationId=d", `{"items":[{"nodeId":"d","nodeType":"document","baseId":"other"}]}`},
		{"applicationId=d", `{"baseId":"other","items":[{"nodeId":"d","nodeType":"document"}]}`},
		{"applicationId=d", `{"items":{},"nodeId":"d","nodeType":"document"}`},
		{"applicationId=d", `{"items":[null]}`},
		{"applicationId=d", `{"items":[],"request":{"nodeId":"d","nodeType":"document"}}`},
		{"applicationId=d", `{"nodeId":"d","nodeType":"document"}`},
		{"", `{"baseId":"other","tables":[]}`},
		{"", `{"request":{"baseId":"b"}}`},
		{"", `{"tables":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.query+tc.body, func(t *testing.T) {
			caller := &urlResolveE2ECaller{results: []*edition.ToolResult{urlResult(tc.body)}}
			out, err := runURLResolveCLI(t, caller, "--url", "https://alidocs.dingtalk.com/i/nodes/b?"+tc.query, "--verify")
			var typed *apperrors.Error
			if err == nil || out != "" || !errors.As(err, &typed) || typed.Reason != "target_verification_failed" {
				t.Fatal(out, err)
			}
		})
	}
	caller := &urlResolveE2ECaller{}
	out, err := runURLResolveCLI(t, caller, "--url", "https://alidocs.dingtalk.com/i/nodes/b?entrance=apps&appId=a", "--verify")
	var typed *apperrors.Error
	if err == nil || out != "" || len(caller.calls) != 0 || !errors.As(err, &typed) || typed.Reason != "target_verification_unsupported" {
		t.Fatal(out, err, caller.calls)
	}
	caller = &urlResolveE2ECaller{errors: []error{errors.New("read denied")}}
	if _, err := runURLResolveCLI(t, caller, "--url", "https://alidocs.dingtalk.com/i/nodes/b?fieldId=f&sheetId=s", "--verify"); err == nil {
		t.Fatal("read error was swallowed")
	}
}

func TestCrossPlatformCoverageURLVerificationEvidenceBoundaries(t *testing.T) {
	if _, err := verifyURLTarget(nil, aitabletarget.Target{Kind: "future_object"}); err == nil {
		t.Fatal("unknown object kind fell back to Base verification")
	}
	for _, value := range []any{"123", json.Number("123"), float64(123)} {
		if !urlObjectIDMatches(map[string]any{"flowId": value}, "123", "flowId") {
			t.Fatal(value)
		}
	}
	for _, value := range []any{true, float64(-1), float64(9007199254740992), nil} {
		if urlObjectIDMatches(map[string]any{"id": value}, "123", "id") {
			t.Fatal(value)
		}
	}
	for _, tc := range []struct {
		value    any
		expected string
	}{
		{float64(1.5), "1.5"},
		{math.NaN(), "NaN"},
		{math.Inf(1), "+Inf"},
		{json.Number("1.5"), "1.5"},
		{json.Number("1e3"), "1e3"},
		{json.Number("-1"), "-1"},
		{json.Number("01"), "01"},
		{json.Number("18446744073709551616"), "18446744073709551616"},
	} {
		if urlObjectIDMatches(map[string]any{"flowId": tc.value}, tc.expected, "flowId") {
			t.Fatalf("non-integer or non-canonical numeric identity accepted: %#v", tc.value)
		}
	}
	if !urlObjectIDMatches(map[string]any{"flowId": json.Number("9007199254740993")}, "9007199254740993", "flowId") {
		t.Fatal("exact integer identity lost precision")
	}
	deep := map[string]any{"id": "b"}
	for range 5 {
		deep = map[string]any{"data": deep}
	}
	if urlResponseBody(deep, aitabletarget.Target{BaseID: "b"}) != nil {
		t.Fatal("unbounded envelope accepted")
	}
	if body := urlResponseBody(map[string]any{"result": map[string]any{"baseId": "b"}}, aitabletarget.Target{BaseID: "b"}); body["baseId"] != "b" {
		t.Fatal(body)
	}
	if urlFindObject(map[string]any{"config": map[string]any{"id": "x"}}, "", "x", "id") != nil {
		t.Fatal("configuration became identity evidence")
	}
}
