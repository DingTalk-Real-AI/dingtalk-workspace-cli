// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package helpers

import (
	"context"
	"fmt"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"strings"
)

// AITableQueryWithView compiles an exact view's filter/sort into query_records
// arguments. Reads use explicit query options in preference to view defaults;
// writes intersect the view with the explicit selector and never widen scope.
// A view ID is never forwarded as an unsupported query_records parameter.
func AITableQueryWithView(ctx context.Context, params map[string]any, viewID string, restrict bool) (map[string]any, error) {
	if strings.TrimSpace(viewID) == "" {
		return nil, viewQueryError("--view-id 不能为空")
	}
	if _, ok := params["recordIds"]; ok {
		return nil, viewQueryError("--record-ids 与 --view-id 不能组合；按 ID 查询会忽略服务端筛选")
	}
	raw, err := CallMCPToolDataOnServer(ctx, "aitable", "get_views", map[string]any{"baseId": params["baseId"], "tableId": params["tableId"], "viewIds": []string{viewID}})
	if err != nil {
		return nil, err
	}
	body, ok := raw.(map[string]any)
	if !ok {
		return nil, viewQueryError("get_views 返回非对象")
	}
	for i := 0; i < 8; i++ {
		if _, ok := body["views"]; ok {
			break
		}
		v, ok := body["data"].(map[string]any)
		if !ok {
			v, ok = body["result"].(map[string]any)
		}
		if !ok {
			break
		}
		body = v
	}
	views, ok := body["views"].([]any)
	if !ok {
		return nil, viewQueryError("get_views 未返回明确视图列表")
	}
	var selected map[string]any
	for _, v := range views {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, viewQueryError("get_views 包含非对象项")
		}
		id, ok := m["viewId"].(string)
		if !ok || id == "" {
			return nil, viewQueryError("get_views 项缺少 viewId")
		}
		if id != viewID {
			continue
		}
		if selected != nil {
			return nil, viewQueryError("get_views 重复返回目标 viewId")
		}
		selected = m
	}
	if selected == nil {
		return nil, viewQueryError("get_views 未返回请求的准确 viewId")
	}
	for _, key := range []string{"baseId", "tableId"} {
		if v, present := selected[key]; present && v != params[key] {
			return nil, viewQueryError("视图所属 " + key + " 与请求不一致")
		}
	}
	// Only documented record-view types can supply filter/sort semantics.
	// A form or future view type must not silently become a full-table scope.
	kind, _ := selected["viewType"].(string)
	switch kind {
	case "Grid", "Kanban", "Gantt", "Calendar", "Gallery":
	default:
		return nil, viewQueryError("该视图类型不能安全转换为记录查询范围；请明确提供记录筛选条件而不传 view-id")
	}
	out := map[string]any{}
	for k, v := range params {
		if k != "viewId" {
			out[k] = v
		}
	}
	if _, explicit := out["filters"]; restrict || !explicit {
		f, present := selected["filter"]
		if !present {
			return nil, viewQueryError("视图缺少 filter，无法证明查询范围")
		}
		vf, err := compileAITableViewFilter(f)
		if err != nil {
			return nil, err
		}
		if restrict && vf != nil && out["filters"] != nil {
			existing, ok := out["filters"].(map[string]any)
			if !ok {
				return nil, viewQueryError("记录 filters 类型非法")
			}
			if !strings.EqualFold(fmt.Sprint(vf["operator"]), "and") || !strings.EqualFold(fmt.Sprint(existing["operator"]), "and") {
				return nil, viewQueryError("带 OR 的视图与额外筛选不能无损合并；请先创建只含目标条件的视图")
			}
			left, lok := vf["operands"].([]any)
			right, rok := existing["operands"].([]any)
			if !lok || !rok {
				return nil, viewQueryError("筛选 operands 类型非法")
			}
			operands := append(append([]any{}, left...), right...)
			out["filters"] = map[string]any{"operator": "and", "operands": operands}
		} else if vf != nil {
			out["filters"] = vf
		}
	}
	if _, explicit := out["sort"]; !explicit && !restrict {
		s, present := selected["sort"]
		if !present {
			return nil, viewQueryError("视图缺少 sort，不能推断排序")
		}
		sort, err := compileAITableViewSort(s)
		if err != nil {
			return nil, err
		}
		if len(sort) > 0 {
			out["sort"] = sort
		}
	}
	return out, nil
}

func viewQueryError(message string) error {
	return apperrors.NewValidation(message, apperrors.WithReason("aitable_view_query_unsupported"), apperrors.WithExecutionStarted(false))
}

func compileAITableViewFilter(raw any) (map[string]any, error) {
	if list, ok := raw.([]any); ok {
		if len(list) == 0 {
			return nil, nil
		}
		if len(list) == 1 {
			if m, ok := list[0].(map[string]any); ok && (m["operator"] == "and" || m["operator"] == "or") {
				raw = m
			} else {
				raw = map[string]any{"operator": "and", "operands": list}
			}
		} else {
			raw = map[string]any{"operator": "and", "operands": list}
		}
	}
	root, ok := raw.(map[string]any)
	if !ok {
		return nil, viewQueryError("视图 filter 不是对象或数组")
	}
	op, ok := root["operator"].(string)
	op = strings.ToLower(op)
	if !ok || (op != "and" && op != "or") {
		return nil, viewQueryError("视图 filter 根只支持 and/or")
	}
	operands, ok := root["operands"].([]any)
	if !ok {
		return nil, viewQueryError("视图 filter 缺少 operands 数组")
	}
	if len(operands) == 0 {
		return nil, nil
	}
	children := make([]any, 0, len(operands))
	allowed := map[string]bool{"eq": true, "ne": true, "exist": true, "un_exist": true, "lt": true, "gt": true, "lte": true, "gte": true, "contain": true, "exclusive": true, "all_of": true, "any_of": true, "none_of": true, "date_eq": true, "before": true, "after": true, "not_before": true, "not_after": true}
	for _, v := range operands {
		leaf, ok := v.(map[string]any)
		if !ok {
			return nil, viewQueryError("视图 filter 子项不是对象")
		}
		operator, ok := leaf["operator"].(string)
		operator = strings.ToLower(operator)
		if !ok || !allowed[operator] {
			return nil, viewQueryError("不支持转换视图操作符 " + operator)
		}
		values, ok := leaf["operands"].([]any)
		n := 2
		if operator == "exist" || operator == "un_exist" {
			n = 1
		}
		if !ok || len(values) != n {
			return nil, viewQueryError("视图条件操作数不完整")
		}
		field, ok := values[0].(string)
		if !ok || strings.TrimSpace(field) == "" {
			return nil, viewQueryError("视图条件缺少字段 ID")
		}
		values = append([]any(nil), values...)
		if n == 2 && strings.Contains(operator, "date") || n == 2 && (operator == "before" || operator == "after" || operator == "not_before" || operator == "not_after") {
			if scheme, ok := values[1].(map[string]any); ok {
				timestamp, number := scheme["timestamp"].(float64)
				if scheme["type"] != "exact" || !number {
					return nil, viewQueryError("相对日期视图暂不能无损转换为记录查询")
				}
				values[1] = timestamp
			}
		}
		children = append(children, map[string]any{"operator": operator, "operands": values})
	}
	return map[string]any{"operator": op, "operands": children}, nil
}

func compileAITableViewSort(raw any) ([]any, error) {
	list, ok := raw.([]any)
	if !ok {
		return nil, viewQueryError("视图 sort 不是数组")
	}
	out := make([]any, 0, len(list))
	for _, v := range list {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, viewQueryError("视图 sort 子项不是对象")
		}
		id, ok := m["fieldId"].(string)
		if !ok || id == "" {
			return nil, viewQueryError("视图排序缺少字段 ID")
		}
		direction := "ASC"
		if d, present := m["direction"]; present {
			s, ok := d.(string)
			if !ok {
				return nil, viewQueryError("视图排序方向非法")
			}
			direction = strings.ToUpper(s)
		} else if d, present := m["desc"]; present {
			b, ok := d.(bool)
			if !ok {
				return nil, viewQueryError("视图排序 desc 不是布尔值")
			}
			if b {
				direction = "DESC"
			}
		}
		if direction != "ASC" && direction != "DESC" {
			return nil, viewQueryError("视图排序方向只支持 ASC/DESC")
		}
		out = append(out, map[string]any{"fieldId": id, "direction": direction})
	}
	return out, nil
}
