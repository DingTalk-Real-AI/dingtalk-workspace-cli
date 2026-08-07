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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
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
	Intent: "当你已经知道某个多维表 Base 的 baseId、又只记得里面某张数据表(table)的名称、" +
		"想把它解析成可直接用于后续工具的 tableId 时使用；" +
		"内部先列出全部数据表并优先做大小写不敏感的精确名称匹配，只有显式 --fuzzy 才允许包含匹配。" +
		"0 个或多个候选都会以结构化错误失败并返回候选，绝不替你猜选。" +
		"这是纯只读操作，只做列举、本地匹配与投影，不会创建、修改或删除任何数据表。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_resolve_table",
			CanonicalPath:  "aitable.shortcut_resolve_table",
			CLIPath:        "aitable +resolve-table",
			PrimaryCLIPath: "aitable +resolve-table",
		},
		Description: "在某个多维表 Base 内按名称解析出唯一的数据表 tableId（只读）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "在某个多维表 Base 内按名称解析出唯一的数据表 tableId（只读）",
			UseWhen:      []string{"当你已经知道某个多维表 Base 的 baseId、又只记得里面某张数据表(table)的名称、想把它解析成可直接用于后续工具的 tableId 时使用；内部先列出全部数据表并优先做大小写不敏感的精确名称匹配，只有显式 --fuzzy 才允许包含匹配。0 个或多个候选都会以结构化错误失败并返回候选，绝不替你猜选。这是纯只读操作，只做列举、本地匹配与投影，不会创建、修改或删除任何数据表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws aitable +resolve-table --base B --name 任务"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "base", Type: shortcut.FlagString, Desc: "Base ID（要在其内解析数据表的多维表）", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "要解析的数据表名称", Required: true},
		{Name: "fuzzy", Type: shortcut.FlagBool, Default: "false", Desc: "精确名称无结果时允许包含匹配"},
	},
	Tips: []string{
		`dws aitable +resolve-table --base B --name 任务`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		resolution, err := aitabletarget.ResolveTableName(rt, rt.Str("base"), rt.Str("name"), rt.Bool("fuzzy"))
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{
			"resolved":  true,
			"status":    resolution.Status,
			"matchType": resolution.MatchType,
			"tableId":   resolution.Selected.ID,
			"name":      resolution.Selected.Name,
			"base":      rt.Str("base"),
		})
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
