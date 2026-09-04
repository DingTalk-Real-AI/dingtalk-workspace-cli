// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableAppCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type aitableAppCaller struct {
	calls    []aitableAppCall
	response string
	err      error
}

func (c *aitableAppCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, aitableAppCall{productID: productID, toolName: toolName, args: args})
	if c.err != nil {
		return nil, c.err
	}
	response := c.response
	if response == "" {
		response = `{"status":"success","data":{}}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{
		Type: "text",
		Text: response,
	}}}, nil
}

func (*aitableAppCaller) Format() string { return "json" }
func (*aitableAppCaller) DryRun() bool   { return false }
func (*aitableAppCaller) Fields() string { return "" }
func (*aitableAppCaller) JQ() string     { return "" }

func runAitableAppCommand(t *testing.T, args ...string) (*aitableAppCaller, error) {
	t.Helper()
	caller := &aitableAppCaller{}
	return caller, runAitableAppCommandWithCaller(t, caller, args...)
}

func runAitableAppCommandWithCaller(t *testing.T, caller *aitableAppCaller, args ...string) error {
	t.Helper()
	testseam.Protect(t, &os.Args)

	InitDepsForTest(t, caller)
	deps.Out.w = io.Discard
	os.Args = append([]string{"dws", "aitable"}, args...)

	cmd := newAitableCommand()
	cmd.PersistentFlags().String("format", "json", "output format")
	cmd.PersistentFlags().Bool("yes", false, "skip confirmation")
	cmd.PersistentFlags().Bool("dry-run", false, "preview only")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.SetArgs(args)
	cmd.SetIn(strings.NewReader(""))
	return cmd.Execute()
}

func TestCrossPlatformCoverageAitableAppModeMapsAllTools(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantTool string
		wantArgs map[string]any
	}{
		{
			name: "app get", args: []string{"app", "get", "--base-id", "base-get"}, wantTool: "get_app",
			wantArgs: map[string]any{"baseId": "base-get"},
		},
		{
			name: "app update", args: []string{"app", "update", "--base", "base-update", "--name", " Sales App ", "--appearance", "dark", "--icon", `{"type":"emoji","id":"rocket"}`, "--nav-theme-type", "dark", "--navigation-layout", "sidebar", "--theme-type", "default-blue"}, wantTool: "update_app",
			wantArgs: map[string]any{
				"baseId": "base-update", "name": "Sales App", "appearance": "dark",
				"icon":         map[string]any{"type": "emoji", "id": "rocket"},
				"navThemeType": "dark", "navigationLayout": "sidebar", "themeType": "default-blue",
			},
		},
		{
			name: "page create", args: []string{"app", "page", "create", "--base-id", "base-page-create", "--name", "Overview", "--before-page-id", "page-before", "--icon", `{"type":"emoji","id":"chart"}`, "--background", `{"type":"color","value":"#FFFFFF"}`}, wantTool: "create_app_page",
			wantArgs: map[string]any{
				"baseId": "base-page-create", "name": "Overview", "beforePageId": "page-before",
				"icon":       map[string]any{"type": "emoji", "id": "chart"},
				"background": map[string]any{"type": "color", "value": "#FFFFFF"},
			},
		},
		{
			name: "page get", args: []string{"app", "page", "get", "--base-id", "base-page-get", "--page-id", "page-get"}, wantTool: "get_app_page",
			wantArgs: map[string]any{"baseId": "base-page-get", "pageId": "page-get"},
		},
		{
			name: "page list", args: []string{"app", "page", "list", "--base-id", "base-page-list"}, wantTool: "list_app_pages",
			wantArgs: map[string]any{"baseId": "base-page-list"},
		},
		{
			name: "page update explicit false", args: []string{"app", "page", "update", "--base-id", "base-page-update", "--page-id", "page-update", "--hidden-menu=false"}, wantTool: "update_app_page",
			wantArgs: map[string]any{"baseId": "base-page-update", "pageId": "page-update", "hiddenMenu": false},
		},
		{
			name: "page move to end", args: []string{"app", "page", "move", "--base-id", "base-page-move", "--page-id", "page-move"}, wantTool: "move_app_page",
			wantArgs: map[string]any{"baseId": "base-page-move", "pageId": "page-move"},
		},
		{
			name: "page delete", args: []string{"app", "page", "delete", "--base-id", "base-page-delete", "--page-id", "page-delete", "--yes"}, wantTool: "delete_app_page",
			wantArgs: map[string]any{"baseId": "base-page-delete", "pageId": "page-delete"},
		},
		{
			name: "widget create", args: []string{"app", "widget", "create", "--base-id", "base-widget-create", "--page-id", "page-widget-create", "--name", "Insight", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"x":0,"y":1,"w":48,"h":8,"parentId":"group-1"}`}, wantTool: "create_app_widget",
			wantArgs: map[string]any{
				"baseId": "base-widget-create", "pageId": "page-widget-create", "name": "Insight",
				"config": map[string]any{"chartType": "AI_ANALYZE"},
				"layout": map[string]any{"x": json.Number("0"), "y": json.Number("1"), "w": json.Number("48"), "h": json.Number("8"), "parentId": "group-1"},
			},
		},
		{
			name: "widget get", args: []string{"app", "widget", "get", "--base-id", "base-widget-get", "--page-id", "page-widget-get", "--widget-id", "widget-get"}, wantTool: "get_app_widget",
			wantArgs: map[string]any{"baseId": "base-widget-get", "pageId": "page-widget-get", "widgetId": "widget-get"},
		},
		{
			name: "widget list", args: []string{"app", "widget", "list", "--base-id", "base-widget-list", "--page-id", "page-widget-list"}, wantTool: "list_page_widgets",
			wantArgs: map[string]any{"baseId": "base-widget-list", "pageId": "page-widget-list"},
		},
		{
			name: "widget update", args: []string{"app", "widget", "update", "--base-id", "base-widget-update", "--page-id", "page-widget-update", "--widget-id", "widget-update", "--config", `{"chartType":"AI_ANALYZE","title":"new"}`, "--layout", `{"x":2,"y":3,"w":24,"h":6}`}, wantTool: "update_app_widget",
			wantArgs: map[string]any{
				"baseId": "base-widget-update", "pageId": "page-widget-update", "widgetId": "widget-update",
				"config": map[string]any{"chartType": "AI_ANALYZE", "title": "new"},
				"layout": map[string]any{"x": json.Number("2"), "y": json.Number("3"), "w": json.Number("24"), "h": json.Number("6")},
			},
		},
		{
			name: "widget delete", args: []string{"app", "widget", "delete", "--base-id", "base-widget-delete", "--page-id", "page-widget-delete", "--widget-id", "widget-delete", "--yes"}, wantTool: "delete_app_widget",
			wantArgs: map[string]any{"baseId": "base-widget-delete", "pageId": "page-widget-delete", "widgetId": "widget-delete"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller, err := runAitableAppCommand(t, tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("tool call count = %d, want 1", len(caller.calls))
			}
			call := caller.calls[0]
			if call.productID != "aitable" || call.toolName != tc.wantTool {
				t.Fatalf("tool call = %s/%s, want aitable/%s", call.productID, call.toolName, tc.wantTool)
			}
			if !reflect.DeepEqual(call.args, tc.wantArgs) {
				t.Fatalf("tool args = %#v, want %#v", call.args, tc.wantArgs)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableAppModeRejectsInvalidInputBeforeMCP(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing base", args: []string{"app", "get"}, want: "base-id"},
		{name: "app update needs change", args: []string{"app", "update", "--base-id", "base"}, want: "至少指定"},
		{name: "page update needs change", args: []string{"app", "page", "update", "--base-id", "base", "--page-id", "page"}, want: "至少指定"},
		{name: "widget update needs change", args: []string{"app", "widget", "update", "--base-id", "base", "--page-id", "page", "--widget-id", "widget"}, want: "至少指定"},
		{name: "icon must be object", args: []string{"app", "update", "--base-id", "base", "--icon", `[]`}, want: "JSON 对象"},
		{name: "icon requires type", args: []string{"app", "update", "--base-id", "base", "--icon", `{}`}, want: "icon.type"},
		{name: "icon type is string", args: []string{"app", "update", "--base-id", "base", "--icon", `{"type":1}`}, want: "icon.type"},
		{name: "icon id is string", args: []string{"app", "update", "--base-id", "base", "--icon", `{"type":"emoji","id":1}`}, want: "icon.id"},
		{name: "background requires type", args: []string{"app", "page", "create", "--base-id", "base", "--name", "page", "--background", `{"value":"#fff"}`}, want: "background.type"},
		{name: "config requires chart type", args: []string{"app", "widget", "create", "--base-id", "base", "--page-id", "page", "--config", `{}`, "--layout", `{"x":0,"y":0,"w":48,"h":8}`}, want: "config.chartType"},
		{name: "config chart type is non-empty", args: []string{"app", "widget", "create", "--base-id", "base", "--page-id", "page", "--config", `{"chartType":" "}`, "--layout", `{"x":0,"y":0,"w":48,"h":8}`}, want: "config.chartType"},
		{name: "layout requires dimensions", args: []string{"app", "widget", "create", "--base-id", "base", "--page-id", "page", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"x":0}`}, want: "layout.y"},
		{name: "layout values are numeric", args: []string{"app", "widget", "create", "--base-id", "base", "--page-id", "page", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"x":"0","y":0,"w":48,"h":8}`}, want: "layout.x"},
		{name: "layout parent is string", args: []string{"app", "widget", "create", "--base-id", "base", "--page-id", "page", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"x":0,"y":0,"w":48,"h":8,"parentId":1}`}, want: "layout.parentId"},
		{name: "page delete needs confirmation", args: []string{"app", "page", "delete", "--base-id", "base", "--page-id", "page"}, want: "用户确认"},
		{name: "widget delete needs confirmation", args: []string{"app", "widget", "delete", "--base-id", "base", "--page-id", "page", "--widget-id", "widget"}, want: "用户确认"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller, err := runAitableAppCommand(t, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid input reached MCP: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableAppModeContracts(t *testing.T) {
	root := newAitableCommand()
	tests := []struct {
		path         string
		canonical    string
		rpc          string
		effect       string
		confirmation string
		idempotency  string
		resultKeys   []string
	}{
		{path: "aitable app get", canonical: "aitable.app_get", rpc: "get_app", effect: "write", confirmation: "not_required", idempotency: "idempotent", resultKeys: []string{"created", "app"}},
		{path: "aitable app update", canonical: "aitable.app_update", rpc: "update_app", effect: "write", confirmation: "not_required", idempotency: "idempotent", resultKeys: []string{"name", "pages"}},
		{path: "aitable app page create", canonical: "aitable.app_page_create", rpc: "create_app_page", effect: "write", confirmation: "not_required", idempotency: "non_idempotent", resultKeys: []string{"pageId", "pageName", "layout", "widgets"}},
		{path: "aitable app page get", canonical: "aitable.app_page_get", rpc: "get_app_page", effect: "read", confirmation: "not_required", idempotency: "idempotent", resultKeys: []string{"pageId", "pageName", "layout", "widgets"}},
		{path: "aitable app page list", canonical: "aitable.app_page_list", rpc: "list_app_pages", effect: "write", confirmation: "not_required", idempotency: "idempotent", resultKeys: []string{"pages", "total"}},
		{path: "aitable app page update", canonical: "aitable.app_page_update", rpc: "update_app_page", effect: "write", confirmation: "not_required", idempotency: "idempotent", resultKeys: []string{"pageId", "pageName", "layout", "widgets"}},
		{path: "aitable app page move", canonical: "aitable.app_page_move", rpc: "move_app_page", effect: "write", confirmation: "not_required", idempotency: "idempotent", resultKeys: []string{"name", "pages"}},
		{path: "aitable app page delete", canonical: "aitable.app_page_delete", rpc: "delete_app_page", effect: "destructive", confirmation: "user_required", idempotency: "unknown", resultKeys: []string{"deletedPageId", "deletedWidgetCount"}},
		{path: "aitable app widget create", canonical: "aitable.app_widget_create", rpc: "create_app_widget", effect: "write", confirmation: "not_required", idempotency: "non_idempotent", resultKeys: []string{"widgetId", "widgetName", "widgetType", "layout"}},
		{path: "aitable app widget get", canonical: "aitable.app_widget_get", rpc: "get_app_widget", effect: "read", confirmation: "not_required", idempotency: "idempotent", resultKeys: []string{"widgetId", "widgetName", "widgetType", "layout"}},
		{path: "aitable app widget list", canonical: "aitable.app_widget_list", rpc: "list_page_widgets", effect: "read", confirmation: "not_required", idempotency: "idempotent", resultKeys: []string{"pageId", "total", "widgets"}},
		{path: "aitable app widget update", canonical: "aitable.app_widget_update", rpc: "update_app_widget", effect: "write", confirmation: "not_required", idempotency: "idempotent", resultKeys: []string{"widgetId", "widgetName", "widgetType", "layout"}},
		{path: "aitable app widget delete", canonical: "aitable.app_widget_delete", rpc: "delete_app_widget", effect: "destructive", confirmation: "user_required", idempotency: "unknown", resultKeys: []string{"deletedWidgetId"}},
	}

	for _, tc := range tests {
		leaf := findCLIPath(root, tc.path)
		if leaf == nil {
			t.Fatalf("path %q does not resolve", tc.path)
		}
		payload, ok := contractfinal.RuntimeContractFinal(leaf)
		if !ok || payload.Identity == nil || payload.Interface == nil || payload.Interface.Ref == nil || payload.Safety == nil || payload.Result == nil {
			t.Fatalf("path %q has incomplete ContractFinal: %#v", tc.path, payload)
		}
		if payload.Identity.CanonicalPath != tc.canonical || payload.Interface.Ref.ProductID != "aitable" || payload.Interface.Ref.RPCName != tc.rpc {
			t.Fatalf("path %q identity/interface = %#v/%#v", tc.path, payload.Identity, payload.Interface)
		}
		if payload.Safety.Effect != tc.effect || payload.Safety.Confirmation != tc.confirmation || payload.Safety.Idempotency != tc.idempotency {
			t.Fatalf("path %q safety = %#v", tc.path, payload.Safety)
		}
		if got := output.CommandRollout(leaf); got != output.RolloutUnifiedActive {
			t.Fatalf("path %q output rollout = %s, want %s", tc.path, got, output.RolloutUnifiedActive)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(payload.Result.DataSchema, &schema); err != nil {
			t.Fatalf("path %q result schema: %v", tc.path, err)
		}
		for _, key := range tc.resultKeys {
			if _, ok := schema.Properties[key]; !ok {
				t.Fatalf("path %q result schema is missing %q: %s", tc.path, key, payload.Result.DataSchema)
			}
		}
	}
}

func TestCrossPlatformCoverageAitableAppModeDoesNotRetryToolErrors(t *testing.T) {
	wantErr := errors.New(`tool failed with {"retryable":true}`)
	caller := &aitableAppCaller{err: wantErr}
	err := runAitableAppCommandWithCaller(t, caller, "app", "page", "list", "--base-id", "base")
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want exactly 1", len(caller.calls))
	}
}

func TestCrossPlatformCoverageAitableAppModeRejectsInvalidToolResults(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{name: "malformed", response: `{`, want: "解析 get_app 返回失败"},
		{name: "non object envelope", response: `[]`, want: "返回值不是 JSON 对象"},
		{name: "missing data", response: `{"status":"success"}`, want: "返回值缺少 data"},
		{name: "non object data", response: `{"status":"success","data":[]}`, want: "data 不是 JSON 对象"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableAppCaller{response: tc.response}
			err := runAitableAppCommandWithCaller(t, caller, "app", "get", "--base-id", "base")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("tool call count = %d, want 1", len(caller.calls))
			}
		})
	}
}
