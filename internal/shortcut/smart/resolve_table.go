// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package smart

import (
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ResolveTable: resolve a 数据表 (table) inside one Base by name keyword into a
// single tableId.
//
// This is the table-level analogue of "resolve a Base by name". Because there is
// no server tool that searches tables by name, it lists every table in the Base
// via get_tables (baseId ← --base, verbatim from list_tables / helpers
// tableGetCmd) and then matches --name locally:
//   - project each table to {tableId, name} — field parsing is defensive
//     (multiple candidate keys);
//   - filter locally by a case-insensitive substring match on name;
//   - exactly one match → return {resolved:true, tableId, name, base};
//     multiple matches → return {resolved:false, count, candidates} and let the
//     caller pick (never guesses);
//     zero matches → report a validation error instead of an empty raw dump.
//
// Read-only: it only lists and reshapes, never mutates any table.
//
//	dws aitable +resolve-table --base B --name 任务
var ResolveTable = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+resolve-table",
	Product:     "aitable",
	Description: "在某个多维表 Base 内按名称解析出唯一的数据表 tableId（只读）",
	Intent: "当你已经知道某个多维表 Base 的 baseId、又只记得里面某张数据表(table)的名称或名称关键词、" +
		"想把它解析成可直接用于后续工具的 tableId 时使用；" +
		"内部先用 get_tables（只传 baseId）列出该 Base 下的全部数据表，再在本地把每张表投影成 tableId、name，" +
		"并按 --name 关键词做大小写不敏感的包含匹配来筛选候选。" +
		"如果只命中一张表就直接返回它的 tableId；如果命中多张则列出全部候选让你消歧，绝不替你瞎猜；如果一张都没命中则提示未找到。" +
		"这是纯只读操作，只做列举、本地匹配与投影，不会创建、修改或删除任何数据表。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base", Type: shortcut.FlagString, Desc: "Base ID（要在其内解析数据表的多维表）", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "要匹配的数据表名称关键词（必填）", Required: true},
	},
	Tips: []string{
		`dws aitable +resolve-table --base B --name 任务`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// baseId mirrors list_tables / helpers tableGetCmd (get_tables). Omitting
		// tableIds asks the server for every table in the base.
		data, err := rt.CallMCPData("aitable", "get_tables", map[string]any{
			"baseId": rt.Str("base"),
		})
		if err != nil {
			return err
		}

		// Project every table to {tableId, name}, defensively unwrapping the list.
		items := resolveTableItems(data)
		all := make([]map[string]any, 0, len(items))
		for _, t := range items {
			all = append(all, map[string]any{
				"tableId": resolveTableID(t),
				"name":    resolveTableName(t),
			})
		}

		// Filter locally by case-insensitive substring match on name.
		needle := strings.ToLower(rt.Str("name"))
		candidates := make([]map[string]any, 0, len(all))
		for _, c := range all {
			name, _ := c["name"].(string)
			if strings.Contains(strings.ToLower(name), needle) {
				candidates = append(candidates, c)
			}
		}

		switch len(candidates) {
		case 0:
			return apperrors.NewValidation("Base 内没有名称包含 " + rt.Str("name") + " 的数据表")
		case 1:
			return rt.Output(map[string]any{
				"resolved": true,
				"tableId":  candidates[0]["tableId"],
				"name":     candidates[0]["name"],
				"base":     rt.Str("base"),
			})
		default:
			return rt.Output(map[string]any{
				"resolved":   false,
				"count":      len(candidates),
				"candidates": candidates,
			})
		}
	},
}

// resolveTableItems defensively unwraps the list of tables from a get_tables
// response, tolerating the common container keys the gateway may use.
func resolveTableItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	for _, key := range []string{"result", "data", "list", "items", "tables", "records"} {
		raw, ok := data[key]
		if !ok {
			continue
		}
		if list, ok := raw.([]any); ok {
			out := make([]map[string]any, 0, len(list))
			for _, e := range list {
				if m, ok := e.(map[string]any); ok {
					out = append(out, m)
				}
			}
			return out
		}
		// Nested container, e.g. {"data":{"list":[...]}}.
		if nested, ok := raw.(map[string]any); ok {
			if inner := resolveTableItems(nested); len(inner) > 0 {
				return inner
			}
		}
	}
	return nil
}

// resolveTableID reads a table's identifier, tolerating the common id keys.
func resolveTableID(t map[string]any) string {
	for _, key := range []string{"tableId", "table_id", "id"} {
		if s := resolveTableString(t[key]); s != "" {
			return s
		}
	}
	return ""
}

// resolveTableName reads a table's display name, tolerating the common name keys.
func resolveTableName(t map[string]any) string {
	for _, key := range []string{"name", "tableName", "table_name", "title"} {
		if s := resolveTableString(t[key]); s != "" {
			return s
		}
	}
	return ""
}

// resolveTableString coerces a scalar JSON value to a trimmed string, returning
// "" for nil / non-scalar / empty values.
func resolveTableString(v any) string {
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func init() {
	shortcut.Register(ResolveTable)
}
