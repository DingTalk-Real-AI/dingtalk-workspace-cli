// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func parityFormShortcuts() []shortcut.Shortcut {
	var out []shortcut.Shortcut
	for _, op := range []string{"create", "get", "submit", "questions-remove"} {
		command := "+form-" + op
		desc := map[string]string{"create": "创建表单并按新 viewId 核对名称", "get": "按准确 viewId 获取表单及可见题目", "submit": "提交已分享表单并按返回 rowId 核对写入值", "questions-remove": "只隐藏表单题目并核对表字段仍存在，不删除整列"}[op]
		flags := []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true}}
		ex := "dws aitable " + command + " --base-id B --table-id T"
		if op == "create" {
			flags = append(flags, shortcut.Flag{Name: "name", Type: shortcut.FlagString, Desc: "表单名称", Required: true})
			ex += " --name 调研"
		} else {
			flags = append(flags, shortcut.Flag{Name: "view-id", Type: shortcut.FlagString, Desc: "准确表单 View ID", Required: true})
			ex += " --view-id V"
		}
		if op == "submit" {
			flags = append(flags, shortcut.Flag{Name: "value", Type: shortcut.FlagString, Desc: "fieldId 到值的 JSON 对象；提交已开启分享的表单，不代替匿名 token 路线", Required: true})
			ex += ` --value '{"fldTitle":"已填写"}'`
		}
		if op == "questions-remove" {
			flags = append(flags, shortcut.Flag{Name: "field-id", Type: shortcut.FlagString, Desc: "仅从当前表单隐藏的字段 ID", Required: true})
			ex += " --field-id F"
		}
		risk := shortcut.RiskWrite
		safety := contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent"}
		if op == "get" {
			risk = shortcut.RiskRead
			safety = contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
		}
		if op == "questions-remove" {
			safety.Idempotency = "idempotent"
		}
		s := shortcut.Shortcut{Service: "aitable", Command: command, Product: serverMain, Description: desc, Intent: desc + "；保留旧 form 原子命令。", Risk: risk, Safety: safety, Flags: flags, Contract: aitableCompositeContractWithResult(command, desc, desc+"时", "删除整个表字段用 +field-delete；分享 token 或条件题目不由此入口代替", ex, parityCompositeResultSpec())}
		s.Execute = func(rt *shortcut.RuntimeContext) error { return executeParityForm(rt, op) }
		out = append(out, withAITableParityAliases(s))
	}
	return out
}

func exactForm(rt *shortcut.RuntimeContext, args map[string]any, id string) (map[string]any, error) {
	raw, err := rt.CallMCPData(serverHelper, "list_form_views", map[string]any{"baseId": args["baseId"], "tableId": args["tableId"]})
	if err != nil {
		return nil, err
	}
	list, err := formListResolveList(raw)
	if err != nil {
		return nil, err
	}
	var selected map[string]any
	seen := map[string]bool{}
	for _, v := range list {
		m, ok := v.(map[string]any)
		key := stringValue(m, "viewId", "view_id", "id")
		if !ok || key == "" || seen[key] {
			return nil, fmt.Errorf("form directory lacks unique identities")
		}
		seen[key] = true
		if key == id {
			selected = m
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("requested form viewId is absent from its table")
	}
	return selected, nil
}
func executeParityForm(rt *shortcut.RuntimeContext, op string) error {
	args := map[string]any{"baseId": rt.Str("base-id"), "tableId": rt.Str("table-id")}
	id := rt.Str("view-id")
	if id != "" {
		args["viewId"] = id
	}
	var values map[string]any
	if op == "submit" {
		var err error
		values, err = parseJSONObject("value", rt.Str("value"))
		if err != nil {
			return err
		}
		if len(values) == 0 {
			return apperrors.NewValidation("--value 不能为空对象")
		}
		b, _ := json.Marshal(values)
		args["value"] = string(b)
	}
	if op == "create" {
		if rt.Str("name") == "" {
			return apperrors.NewValidation("--name 不能为空")
		}
		args["name"] = rt.Str("name")
	}
	if op == "questions-remove" {
		args["fieldId"] = rt.Str("field-id")
		args["hidden"] = true
	}
	result := newCompositeResult("form_" + op)
	result.Resolved = map[string]any{"baseId": args["baseId"], "tableId": args["tableId"]}
	if id != "" {
		result.Resolved["viewId"] = id
	}
	tool := map[string]string{"create": "create_form_view", "get": "list_form_views", "submit": "submit_form", "questions-remove": "update_form_field_hidden"}[op]
	result.Plan = []compositeStep{{Index: 1, Name: op, Tool: tool, Status: "planned", Arguments: args}}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}
	if op != "create" {
		if _, err := exactForm(rt, args, id); err != nil {
			return err
		}
	}
	if op == "questions-remove" {
		if err := verifyFormFieldExists(rt, args); err != nil {
			return err
		}
	}
	if op != "get" {
		server := serverMain
		if op == "questions-remove" {
			server = serverHelper
		}
		raw, err := rt.CallMCPWriteDataStrict(server, tool, args)
		newID := findStringByKeys(raw, "viewId")
		if op == "submit" {
			newID = findStringByKeys(raw, "rowId")
		}
		if newID != "" {
			key := "viewId"
			if op == "submit" {
				key = "recordId"
			}
			result.Resolved[key] = newID
			result.KnownEffects = []map[string]any{{key: newID, "tool": tool}}
		}
		if err != nil {
			return compositeError(result, err, false)
		}
		if op == "submit" {
			if newID == "" {
				return compositeError(result, fmt.Errorf("submit response lacks rowId"), false)
			}
			rows, e := queryRecordsByIDs(rt, rt.Str("base-id"), rt.Str("table-id"), []string{newID})
			if e != nil {
				return compositeError(result, e, false)
			}
			if len(rows) != 1 {
				return compositeError(result, fmt.Errorf("submitted row is not readable by its exact ID"), false)
			}
			if e = newRecordFieldTypeResolver(rt, rt.Str("base-id"), rt.Str("table-id")).verify(rows[0], values); e != nil {
				return compositeError(result, e, false)
			}
			result.Result = map[string]any{"recordId": newID, "record": rows[0]}
			result.Verification = map[string]any{"status": "verified"}
			result.CompletedCount = 1
			return rt.Output(result)
		}
		if op == "create" {
			if newID == "" {
				return compositeError(result, fmt.Errorf("create_form_view lacks viewId"), false)
			}
			id = newID
			args["viewId"] = id
		}
	}
	form, err := exactForm(rt, args, id)
	if err != nil {
		return compositeError(result, err, false)
	}
	if op == "create" && stringValue(form, "name", "viewName", "title") != rt.Str("name") {
		return compositeError(result, fmt.Errorf("created form name mismatch"), false)
	}
	fields, err := rt.CallMCPData(serverHelper, "list_form_fields", map[string]any{"baseId": args["baseId"], "tableId": args["tableId"], "viewId": id})
	if err != nil {
		return compositeError(result, err, false)
	}
	list, ok := findNamedObjectList(fields, "fields", "items")
	if !ok {
		return compositeError(result, fmt.Errorf("form visible-field list missing"), false)
	}
	if op == "questions-remove" {
		for _, f := range list {
			fid := stringValue(f, "fieldId")
			if fid == "" {
				return compositeError(result, fmt.Errorf("visible field lacks ID"), false)
			}
			if fid == rt.Str("field-id") {
				return compositeError(result, fmt.Errorf("question remains visible"), false)
			}
		}
		if err = verifyFormFieldExists(rt, args); err != nil {
			return compositeError(result, err, false)
		}
	}
	result.Result = map[string]any{"form": form, "visibleFields": fields}
	result.Verification = map[string]any{"status": "verified"}
	result.CompletedCount = 1
	return rt.Output(result)
}
func verifyFormFieldExists(rt *shortcut.RuntimeContext, args map[string]any) error {
	raw, err := rt.CallMCPData(serverMain, "get_fields", map[string]any{"baseId": args["baseId"], "tableId": args["tableId"]})
	if err != nil {
		return err
	}
	list, ok := findNamedObjectList(raw, "fields")
	if !ok {
		return fmt.Errorf("field directory missing")
	}
	matches := 0
	for _, f := range list {
		if stringValue(f, "fieldId") == args["fieldId"] {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("question's table field is absent or ambiguous")
	}
	return nil
}
func init() { shortcut.Register(parityFormShortcuts()...) }
