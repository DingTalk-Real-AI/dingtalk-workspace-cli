// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"strings"
)

// App and page semantics stay Base-scoped. No alias pretends a Lark app token
// is a DingTalk Base ID, and getters that initialize an App keep a write gate.
type parityAppOperation struct {
	command, tool, description, idFlag, idKey, readTool, collection string
	write, create                                                   bool
	flags                                                           []shortcut.Flag
}

func parityAppShortcuts() []shortcut.Shortcut {
	str := func(n, d string, required bool) shortcut.Flag {
		return shortcut.Flag{Name: n, Type: shortcut.FlagString, Desc: d, Required: required}
	}
	page := str("page-id", "应用模式 Page ID（仪表盘页面）", true)
	name := str("name", "显示名称", true)
	widget := str("widget-id", "Widget ID", true)
	ops := []parityAppOperation{
		{command: "+app-get", tool: "get_app", description: "获取 Base 唯一应用模式 App；不存在时自动创建，需确认", write: true},
		{command: "+app-page-list", tool: "list_app_pages", description: "列出 Base 应用页面；不存在 App 时会创建默认 App，需确认", collection: "pages", idKey: "pageId", write: true},
		{command: "+app-page-get", tool: "get_app_page", description: "读取准确应用页面及组件摘要", idFlag: "page-id", idKey: "pageId", flags: []shortcut.Flag{page}},
		{command: "+app-page-create", tool: "create_app_page", description: "新建应用仪表盘页面并按返回 ID 读回名称", idKey: "pageId", readTool: "get_app_page", write: true, create: true, flags: []shortcut.Flag{name}},
		{command: "+app-page-update", tool: "update_app_page", description: "重命名应用页面并按准确 ID 读回", idFlag: "page-id", idKey: "pageId", readTool: "get_app_page", write: true, flags: []shortcut.Flag{page, name}},
		{command: "+app-page-delete", tool: "delete_app_page", description: "删除应用页面并核对完整页面目录中已不存在", idFlag: "page-id", idKey: "deletedPageId", write: true, flags: []shortcut.Flag{page}},
		{command: "+app-block-delete", tool: "delete_app_widget", description: "删除应用 Widget 并核对其所属页面列表", idFlag: "widget-id", idKey: "deletedWidgetId", write: true, flags: []shortcut.Flag{page, widget}},

		{command: "+app-block-list", tool: "list_page_widgets", description: "列出准确应用页面中的全部 Widget", idFlag: "page-id", idKey: "widgetId", collection: "widgets", flags: []shortcut.Flag{page}},
		{command: "+app-block-get", tool: "get_app_widget", description: "读取准确应用 Widget 配置与布局", idFlag: "widget-id", idKey: "widgetId", flags: []shortcut.Flag{page, widget}},
		{command: "+app-block-create", tool: "create_app_widget", description: "创建应用 Widget 并独立核对配置和 48 列布局", idKey: "widgetId", readTool: "get_app_widget", write: true, create: true, flags: []shortcut.Flag{page, str("name", "组件名称", false), str("config", "含 chartType 的完整 Widget 配置 JSON", true), str("layout", "48 列根布局 JSON（x/y/w/h）", true)}},
		{command: "+app-block-update", tool: "update_app_widget", description: "更新应用 Widget 并独立核对名称、配置或布局", idFlag: "widget-id", idKey: "widgetId", readTool: "get_app_widget", write: true, flags: []shortcut.Flag{page, widget, str("name", "新名称", false), str("config", "完整 Widget 配置 JSON，省略保留", false), str("layout", "完整 48 列根布局 JSON，省略保留", false)}},
	}
	out := make([]shortcut.Shortcut, 0, len(ops))
	for _, op := range ops {
		risk, effect, confirmation := shortcut.RiskRead, "read", "not_required"
		safetyRisk := "low"
		if op.write {
			risk, effect, confirmation = shortcut.RiskWrite, "write", "user_required"
			safetyRisk = "medium"
			if op.command == "+app-page-delete" || op.command == "+app-block-delete" {
				risk, effect = shortcut.RiskHighWrite, "destructive"
				safetyRisk = "high"
			}
		}
		flags := append([]shortcut.Flag{str("base-id", "所属 Base ID；不是 Lark app-token", true)}, op.flags...)
		example := "dws aitable " + op.command + " --base-id B"
		for _, f := range op.flags {
			if f.Required {
				v := "VALUE"
				if f.Name == "config" {
					v = `'{"chartType":"AI_ANALYZE"}'`
				}
				if f.Name == "layout" {
					v = `'{"x":0,"y":0,"w":48,"h":8}'`
				}
				example += " --" + f.Name + " " + v
			}
		}
		s := shortcut.Shortcut{Service: "aitable", Command: op.command, Product: serverMain, Description: op.description, Intent: op.description + "；仅支持 Base 应用模式仪表盘页面/Widget，旧原子入口保留。", Risk: risk, Safety: contract.SafetySpec{Effect: effect, Risk: safetyRisk, Confirmation: confirmation, Idempotency: "unknown"}, Contract: aitableCompositeContractWithResult(op.command, op.description, op.description+"时", "独立 Workspace BaseApp 或 AI Page JSON 不使用这些入口；其他属性仍可用 aitable app 原子命令", example, parityCompositeResultSpec()), Flags: flags}
		s.Execute = func(rt *shortcut.RuntimeContext) error { return executeParityApp(rt, op) }
		out = append(out, withAITableParityAliases(s))
	}
	return out
}
func parityResponseObject(raw map[string]any) map[string]any {
	if v, ok := raw["data"].(map[string]any); ok {
		return v
	}
	return raw
}
func executeParityApp(rt *shortcut.RuntimeContext, op parityAppOperation) error {
	args := map[string]any{"baseId": rt.Str("base-id")}
	for _, f := range op.flags {
		if !rt.Changed(f.Name) {
			continue
		}
		key := map[string]string{"page-id": "pageId", "widget-id": "widgetId"}[f.Name]
		if key == "" {
			key = f.Name
		}
		if f.Name == "config" || f.Name == "layout" {
			m, err := parseJSONObject(f.Name, rt.Str(f.Name))
			if err != nil {
				return err
			}
			if err = aitableprotocol.ValidateDashboardPersistentMetadata(f.Name, m); err != nil {
				return apperrors.NewValidation(err.Error())
			}
			if f.Name == "layout" {
				if err = aitableprotocol.ValidateRootChartLayout(m, 48); err != nil {
					return apperrors.NewValidation(err.Error())
				}
			} else if stringValue(m, "chartType") == "" {
				return apperrors.NewValidation("config.chartType 必填")
			}
			args[key] = m
		} else {
			v := rt.Str(f.Name)
			if v == "" {
				return apperrors.NewValidation("--" + f.Name + " 不能为空")
			}
			args[key] = v
		}
	}
	if op.command == "+app-block-update" && len(args) == 3 {
		return apperrors.NewValidation("必须提供 name/config/layout 至少一项")
	}
	result := newCompositeResult(strings.ReplaceAll(op.command[1:], "-", "_"))
	result.Resolved = map[string]any{"baseId": rt.Str("base-id")}
	for _, k := range []string{"pageId", "widgetId"} {
		if v, ok := args[k]; ok {
			result.Resolved[k] = v
		}
	}
	result.Plan = []compositeStep{{Index: 1, Name: op.description, Tool: op.tool, Status: "planned", Arguments: args}}
	if rt.DryRun() {
		result.Executed = false
		result.Status = "planned"
		return rt.Output(result)
	}
	if op.command == "+app-block-delete" {
		before, e := rt.CallMCPData(serverMain, "get_app_widget", map[string]any{"baseId": args["baseId"], "pageId": args["pageId"], "widgetId": args["widgetId"]})
		if e != nil {
			return e
		}
		b := parityResponseObject(before)
		if b["widgetId"] != args["widgetId"] || b["pageId"] != args["pageId"] {
			return fmt.Errorf("widget deletion preflight identity mismatch")
		}
	}
	var raw map[string]any
	var err error
	if op.write {
		raw, err = rt.CallMCPWriteDataStrict(serverMain, op.tool, args)
	} else {
		raw, err = rt.CallMCPData(serverMain, op.tool, args)
	}
	body := parityResponseObject(raw)
	if id := stringValue(body, op.idKey); id != "" && op.write {
		result.Resolved[op.idKey] = id
		result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": op.tool, "returnedId": id, "status": "receipt_only"})
	}
	if err != nil {
		return compositeError(result, err, false)
	}
	if op.command == "+app-get" {
		app, ok := body["app"].(map[string]any)
		created, cok := body["created"].(bool)
		id := stringValue(app, "appId")
		if !ok || !cok || id == "" {
			return compositeError(result, fmt.Errorf("get_app lacks created/app.appId"), false)
		}
		result.Resolved["appId"] = id
		if created {
			result.KnownEffects = append(result.KnownEffects, map[string]any{"appId": id, "created": true})
			again, e := rt.CallMCPData(serverMain, "get_app", args)
			a := parityResponseObject(again)
			r, _ := a["app"].(map[string]any)
			if e != nil {
				return compositeError(result, e, false)
			}
			if stringValue(r, "appId") != id || a["created"] != false {
				return compositeError(result, fmt.Errorf("created App could not be independently verified"), false)
			}
		}
	} else if op.collection != "" {
		list, ok := body[op.collection].([]any)
		if !ok {
			return compositeError(result, fmt.Errorf("response lacks %s collection", op.collection), false)
		}
		seen := map[string]bool{}
		for _, raw := range list {
			m, ok := raw.(map[string]any)
			id := stringValue(m, op.idKey)
			if !ok || id == "" || seen[id] {
				return compositeError(result, fmt.Errorf("collection has missing/duplicate identity"), false)
			}
			seen[id] = true
		}
		if op.collection == "widgets" && body["pageId"] != args["pageId"] {
			return compositeError(result, fmt.Errorf("widget collection page mismatch"), false)
		}
	} else {
		id := stringValue(body, op.idKey)
		if id == "" || (!op.create && id != rt.Str(op.idFlag)) {
			return compositeError(result, fmt.Errorf("response object ID mismatch"), false)
		}
		if op.command == "+app-page-delete" || op.command == "+app-block-delete" {
			tool, key, itemKey := "list_app_pages", "pages", "pageId"
			params := map[string]any{"baseId": args["baseId"]}
			if op.command == "+app-block-delete" {
				tool, key, itemKey = "list_page_widgets", "widgets", "widgetId"
				params["pageId"] = args["pageId"]
			}
			r, e := rt.CallMCPData(serverMain, tool, params)
			if e != nil {
				return compositeError(result, e, false)
			}
			list, ok := parityResponseObject(r)[key].([]any)
			if !ok {
				return compositeError(result, fmt.Errorf("delete verification lacks collection"), false)
			}
			for _, raw := range list {
				m, ok := raw.(map[string]any)
				if !ok || stringValue(m, itemKey) == "" {
					return compositeError(result, fmt.Errorf("delete verification lacks item identity"), false)
				}
				if stringValue(m, itemKey) == id {
					return compositeError(result, fmt.Errorf("deleted object is still present"), false)
				}
			}
		}

		if op.readTool != "" {
			readArgs := map[string]any{"baseId": args["baseId"], op.idKey: id}
			if v, ok := args["pageId"]; ok {
				readArgs["pageId"] = v
			}
			again, e := rt.CallMCPData(serverMain, op.readTool, readArgs)
			actual := parityResponseObject(again)
			if e != nil {
				return compositeError(result, e, false)
			}
			if stringValue(actual, op.idKey) != id {
				return compositeError(result, fmt.Errorf("mutation readback ID mismatch"), false)
			}
			for _, key := range []string{"name", "config", "layout"} {
				if expected, ok := args[key]; ok {
					readKey := key
					if key == "name" {
						readKey = "pageName"
						if op.idKey == "widgetId" {
							readKey = "widgetName"
						}
					}
					if key == "name" {
						if actual[readKey] != expected {
							return compositeError(result, fmt.Errorf("readback name mismatch"), false)
						}
					} else {
						want := expected.(map[string]any)
						got, ok := actual[readKey].(map[string]any)
						if !ok || !mapContains(got, want) {
							return compositeError(result, fmt.Errorf("readback %s mismatch", key), false)
						}
					}
				}
			}
			body = actual
		}
		if op.idKey == "widgetId" && body["pageId"] != args["pageId"] {
			return compositeError(result, fmt.Errorf("widget belongs to a different page"), false)
		}
	}
	result.CompletedCount = 1
	result.Verification = map[string]any{"status": "verified"}
	result.Result = body
	return rt.Output(result)
}
func init() { shortcut.Register(parityAppShortcuts()...) }
