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

// Package aitable provides declarative shortcuts for the DingTalk AI 表格
// (aitable) service: Base / table / field / record / view / form / dashboard /
// chart / workflow / advanced-permission / section management. Each shortcut maps
// 1:1 onto an MCP tool declared in internal/helpers/aitable.go.
//
// Tool routing: most tools live on the "aitable" MCP server; a subset of helper
// tools live on the "aitable-helper" server. Each shortcut sets Product to the
// server that owns its tool so rt.CallMCP dispatches correctly.
package aitable

import (
	"encoding/json"
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// serverMain is the primary aitable MCP server id.
const serverMain = "aitable"

// serverHelper is the aitable-helper MCP server id (hosts a subset of tools).
const serverHelper = "aitable-helper"

// parseJSONAny parses an arbitrary JSON string (object or array) into any.
func parseJSONAny(flag, s string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("--%s JSON 解析失败: %w", flag, err)
	}
	return v, nil
}

// parseJSONObject parses a JSON string into a map[string]any, erroring on arrays.
func parseJSONObject(flag, s string) (map[string]any, error) {
	v, err := parseJSONAny(flag, s)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("--%s 必须是 JSON 对象，got %T", flag, v)
	}
	return m, nil
}

// ─────────────────────────────────────────────────────────────
// base: Base 管理（server: aitable）
// ─────────────────────────────────────────────────────────────

// BaseList 获取 AI 表格列表（list_bases）。
var BaseList = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+base-list",
	Product:     serverMain,
	Description: "获取当前用户可访问的 AI 表格 Base 列表（最近访问，支持游标分页）",
	Intent:      "当你不知道具体 baseId、想先浏览自己最近用过或可访问的 AI 表格清单以便定位目标时使用；支持游标分页，返回 Base 列表及其 baseId。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量，默认 10，最大 10"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，首次不传"},
	},
	Tips: []string{`dws aitable +base-list`, `dws aitable +base-list --limit 5 --cursor NEXT`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Changed("limit") {
			params["limit"] = rt.Int("limit")
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData(serverMain, "list_bases", params)
		if err != nil {
			return err
		}
		bases := baseListProject(data)
		return rt.Output(map[string]any{"count": len(bases), "bases": bases})
	},
}

// baseListProject reshapes list_bases / search_bases responses into a clean
// {baseId, baseName} list — clean output projection. Both the list
// container and the per-item field names are probed defensively across
// candidate keys, so an empty/unknown shape yields an empty list rather than a
// crash or fabricated data.
func baseListProject(data map[string]any) []map[string]any {
	raw := baseListResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := baseListFirst(m, "baseId", "base_id", "id"); ok {
			row["baseId"] = v
		}
		if v, ok := baseListFirst(m, "baseName", "base_name", "name", "title"); ok {
			row["baseName"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// baseListResolveList locates the list payload inside the response, tolerating a
// bare top-level array container or nesting one level under a common envelope.
func baseListResolveList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, key := range []string{"bases", "result", "data", "list", "items", "records"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"bases", "list", "items", "result", "data", "records"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// baseListFirst returns the first present candidate key's value.
func baseListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// BaseSearch 按名称关键词搜索 AI 表格（search_bases）。
var BaseSearch = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+base-search",
	Product:     serverMain,
	Description: "按名称关键词搜索 AI 表格 Base",
	Intent:      "当你知道某个 AI 表格的名字或部分关键词、想直接定位到它并拿到 baseId 时使用；输入名称关键词，返回匹配的 Base 列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "Base 名称关键词", Required: true},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，首次不传"},
	},
	Tips: []string{`dws aitable +base-search --query "项目管理"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"query": rt.Str("query")}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData(serverMain, "search_bases", params)
		if err != nil {
			return err
		}
		bases := baseListProject(data)
		return rt.Output(map[string]any{"count": len(bases), "bases": bases})
	},
}

// BaseGet 获取 AI 表格信息（get_base）。
var BaseGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+base-get",
	Product:     serverMain,
	Description: "获取指定 Base 的目录信息（tables / dashboards summary）",
	Intent:      "当你已有 baseId、需要了解这个表格里有哪些数据表和仪表盘（拿到 tableId/dashboardId）以便进一步操作时使用；返回 Base 的目录结构概要。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
	},
	Tips: []string{`dws aitable +base-get --base-id BASE_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_base", map[string]any{"baseId": rt.Str("base-id")})
	},
}

// BaseGetPrimaryDocID 获取记录主键文档 ID（get_base_primary_doc_id）。
var BaseGetPrimaryDocID = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+base-get-primary-doc-id",
	Product:     serverMain,
	Description: "根据 baseId/tableId/recordId 获取主键文档的 dentryUuid",
	Intent:      "当某条记录的主键列是文档类型、你需要拿到该主键文档的 dentryUuid 以便打开或引用该文档时使用；输入 base/table/record，返回 dentryUuid。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "record-id", Type: shortcut.FlagString, Desc: "记录 ID", Required: true},
	},
	Tips: []string{`dws aitable +base-get-primary-doc-id --base-id B --table-id T --record-id R`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_base_primary_doc_id", map[string]any{
			"baseId":   rt.Str("base-id"),
			"tableId":  rt.Str("table-id"),
			"recordId": rt.Str("record-id"),
		})
	},
}

// BaseCreate 创建 AI 表格（create_base）。
// BaseUpdate 更新 AI 表格（update_base）。
var BaseUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+base-update",
	Product:     serverMain,
	Description: "更新 Base 名称（可选备注）",
	Intent:      "当你要给已有 AI 表格改名或修改备注说明时使用；会实际修改指定 Base 的名称/描述。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新名称，1-50 字符", Required: true},
		{Name: "desc", Type: shortcut.FlagString, Desc: "备注文本（可选）"},
	},
	Tips: []string{`dws aitable +base-update --base-id BASE_ID --name "新名称"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":      rt.Str("base-id"),
			"newBaseName": rt.Str("name"),
		}
		if rt.Changed("desc") {
			params["description"] = rt.Str("desc")
		}
		return rt.CallMCP("update_base", params)
	},
}

// BaseDelete 删除 AI 表格（delete_base）。
var BaseDelete = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+base-delete",
	Product:     serverMain,
	Description: "删除指定 Base（不可逆）",
	Intent:      "当你确认要彻底删除某个 AI 表格时使用；会不可逆地删除整个 Base 及其所有数据表和记录，操作前务必核对 baseId。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "待删除 Base ID", Required: true},
		{Name: "reason", Type: shortcut.FlagString, Desc: "删除原因（可选）"},
	},
	Tips: []string{`dws aitable +base-delete --base-id BASE_ID --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"baseId": rt.Str("base-id")}
		if rt.Changed("reason") {
			params["reason"] = rt.Str("reason")
		}
		return rt.CallMCP("delete_base", params)
	},
}

// BaseCopy 复制 AI 表格（copy_base）。
var BaseCopy = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+base-copy",
	Product:     serverMain,
	Description: "复制 AI 表格到指定目录（可仅复制结构）",
	Intent:      "当你想基于现有表格快速复刻一份（如做模板或备份，可选仅复制结构不含数据）时使用；会在目标文件夹实际创建一个副本 Base。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "源 Base ID", Required: true},
		{Name: "target-folder-id", Type: shortcut.FlagString, Desc: "目标文件夹 dentryUuid", Required: true},
		{Name: "only-struct", Type: shortcut.FlagBool, Desc: "仅复制结构（不含数据），默认 false"},
	},
	Tips: []string{`dws aitable +base-copy --base-id BASE_ID --target-folder-id FOLDER_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("copy_base", map[string]any{
			"baseId":         rt.Str("base-id"),
			"targetFolderId": rt.Str("target-folder-id"),
			"onlyCopyMeta":   rt.Bool("only-struct"),
		})
	},
}

// ─────────────────────────────────────────────────────────────
// table: 数据表管理（server: aitable）
// ─────────────────────────────────────────────────────────────

// TableGet 获取数据表（get_tables）。
var TableGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+table-get",
	Product:     serverMain,
	Description: "批量获取指定数据表的表级信息、字段目录与视图目录",
	Intent:      "当你已进入某个 Base、需要了解其中某些数据表有哪些字段（拿 fieldId）、有哪些视图（拿 viewId）以便读写数据时使用；批量返回表信息、字段目录和视图目录。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-ids", Type: shortcut.FlagStringSlice, Desc: "Table ID 列表，逗号分隔，单次最多 10 个（可选）"},
	},
	Tips: []string{`dws aitable +table-get --base-id BASE_ID`, `dws aitable +table-get --base-id B --table-ids tbl1,tbl2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"baseId": rt.Str("base-id")}
		if rt.Changed("table-ids") {
			params["tableIds"] = rt.StrSlice("table-ids")
		}
		return rt.CallMCP("get_tables", params)
	},
}

// TableCreate 创建数据表（create_table）。
// TableUpdate 更新数据表（update_table）。
var TableUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+table-update",
	Product:     serverMain,
	Description: "更新数据表名称 / 备注 / 行命名规则",
	Intent:      "当你要给数据表改名、改备注，或调整行的命名规则（如按 task/project 命名）时使用；会实际更新表级属性。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新表名（可选）"},
		{Name: "description", Type: shortcut.FlagString, Desc: "备注说明（可选）"},
		{Name: "record-name-key", Type: shortcut.FlagString, Desc: "行命名规则枚举键，如 task/project（可选）"},
	},
	Tips: []string{`dws aitable +table-update --base-id B --table-id T --name "新表名"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
		}
		if rt.Changed("name") {
			params["newTableName"] = rt.Str("name")
		}
		if rt.Changed("description") {
			params["description"] = rt.Str("description")
		}
		if rt.Changed("record-name-key") {
			params["recordNameKey"] = rt.Str("record-name-key")
		}
		return rt.CallMCP("update_table", params)
	},
}

// TableDelete 删除数据表（delete_table）。
var TableDelete = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+table-delete",
	Product:     serverMain,
	Description: "删除指定数据表（不可逆）",
	Intent:      "当你确认要删除整张数据表（连同其所有记录和视图）时使用；不可逆，操作前请核对 tableId。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "待删除 Table ID", Required: true},
		{Name: "reason", Type: shortcut.FlagString, Desc: "删除原因（可选）"},
	},
	Tips: []string{`dws aitable +table-delete --base-id B --table-id T --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
		}
		if rt.Changed("reason") {
			params["reason"] = rt.Str("reason")
		}
		return rt.CallMCP("delete_table", params)
	},
}

// ─────────────────────────────────────────────────────────────
// field: 字段管理（server: aitable）
// ─────────────────────────────────────────────────────────────

// FieldGet 获取字段详情（get_fields）。
var FieldGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+field-get",
	Product:     serverMain,
	Description: "批量获取字段详情（含类型相关完整配置）",
	Intent:      "当你需要查看字段的完整类型配置（如单选选项、关联表设置、AI 配置）以便正确写入数据或改配置时使用；批量返回字段详情。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "field-ids", Type: shortcut.FlagStringSlice, Desc: "字段 ID 列表，逗号分隔，单次最多 10 个（可选）"},
	},
	Tips: []string{`dws aitable +field-get --base-id B --table-id T`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
		}
		if rt.Changed("field-ids") {
			params["fieldIds"] = rt.StrSlice("field-ids")
		}
		return rt.CallMCP("get_fields", params)
	},
}

// FieldCreate 创建字段（create_fields）。
// FieldUpdate 更新字段（update_field）。
var FieldUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+field-update",
	Product:     serverMain,
	Description: "更新字段名称 / 配置 / AI 配置（类型不可改）",
	Intent:      "当你要改字段名，或调整字段配置/AI 配置（注意字段类型本身不可改）时使用；会实际更新指定字段。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "field-id", Type: shortcut.FlagString, Desc: "Field ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新字段名（可选）"},
		{Name: "config", Type: shortcut.FlagString, Desc: "字段配置 JSON（可选）"},
		{Name: "ai-config", Type: shortcut.FlagString, Desc: "AI 配置 JSON（可选）"},
	},
	Tips: []string{`dws aitable +field-update --base-id B --table-id T --field-id F --name "新名"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"fieldId": rt.Str("field-id"),
		}
		if rt.Changed("name") {
			params["newFieldName"] = rt.Str("name")
		}
		if rt.Changed("config") {
			cfg, err := parseJSONObject("config", rt.Str("config"))
			if err != nil {
				return err
			}
			params["config"] = cfg
		}
		if rt.Changed("ai-config") {
			cfg, err := parseJSONObject("ai-config", rt.Str("ai-config"))
			if err != nil {
				return err
			}
			params["aiConfig"] = cfg
		}
		return rt.CallMCP("update_field", params)
	},
}

// FieldSearchOptions 搜索单选/多选字段选项（search_field_options）。
// FieldDelete 删除字段（delete_field）。
var FieldDelete = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+field-delete",
	Product:     serverMain,
	Description: "删除指定字段（不可逆）",
	Intent:      "当你确认要删除某个字段（连同该列在所有记录里的数据）时使用；不可逆，操作前请核对 fieldId。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "field-id", Type: shortcut.FlagString, Desc: "待删除字段 ID", Required: true},
	},
	Tips: []string{`dws aitable +field-delete --base-id B --table-id T --field-id F --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("delete_field", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"fieldId": rt.Str("field-id"),
		})
	},
}

// ─────────────────────────────────────────────────────────────
// record: 记录管理（server: aitable / aitable-helper）
// ─────────────────────────────────────────────────────────────

// RecordQuery 获取行记录（query_records）。
var RecordQuery = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-query",
	Product:     serverMain,
	Description: "查询表格记录（按 ID 取 / 条件筛选 / 关键词 / 分页）",
	Intent:      "当你要读取表格里的行数据——按 recordId 精确取、按结构化条件筛选、按关键词全文搜索或分页遍历时使用；返回匹配记录及其单元格值。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "record-ids", Type: shortcut.FlagStringSlice, Desc: "记录 ID 列表，单次最多 100（可选）"},
		{Name: "field-ids", Type: shortcut.FlagStringSlice, Desc: "返回字段 ID 列表（可选）"},
		{Name: "filters", Type: shortcut.FlagString, Desc: "结构化过滤条件 JSON（可选）"},
		{Name: "sort", Type: shortcut.FlagString, Desc: "排序条件 JSON 数组（可选）"},
		{Name: "query", Type: shortcut.FlagString, Desc: "全文关键词（可选）"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "单次最大记录数，默认 100（可选）"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标（可选）"},
	},
	Tips: []string{`dws aitable +record-query --base-id B --table-id T --query "关键词" --limit 50`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
		}
		if rt.Changed("record-ids") {
			params["recordIds"] = rt.StrSlice("record-ids")
		}
		if rt.Changed("field-ids") {
			params["fieldIds"] = rt.StrSlice("field-ids")
		}
		if rt.Changed("filters") {
			f, err := parseJSONAny("filters", rt.Str("filters"))
			if err != nil {
				return err
			}
			params["filters"] = f
		}
		if rt.Changed("sort") {
			s, err := parseJSONAny("sort", rt.Str("sort"))
			if err != nil {
				return err
			}
			params["sort"] = s
		}
		if rt.Changed("query") {
			params["keyword"] = rt.Str("query")
		}
		if rt.Changed("limit") {
			params["limit"] = rt.Int("limit")
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		return rt.CallMCP("query_records", params)
	},
}

// RecordCreate 新增记录（create_records）。
// RecordUpdate 更新记录（update_records）。
var RecordUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-update",
	Product:     serverMain,
	Description: "批量更新记录（每条含 recordId 与 cells），单次最多 100 条",
	Intent:      "当你要批量修改已有行的字段值（如把一批任务标为已完成，每条须带 recordId）时使用；会实际覆盖对应单元格，单次最多 100 条。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "records", Type: shortcut.FlagString, Desc: "记录 JSON 数组，如 '[{\"recordId\":\"rec\",\"cells\":{...}}]'", Required: true},
	},
	Tips: []string{`dws aitable +record-update --base-id B --table-id T --records '[{"recordId":"rec","cells":{"fldStatusId":"已完成"}}]'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		recs, err := parseJSONAny("records", rt.Str("records"))
		if err != nil {
			return err
		}
		return rt.CallMCP("update_records", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"records": recs,
		})
	},
}

// RecordDelete 删除行记录（delete_records）。
var RecordDelete = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-delete",
	Product:     serverMain,
	Description: "批量删除记录（不可逆），单次最多 100 条",
	Intent:      "当你确认要批量删除若干行记录时使用；不可逆，按 recordId 列表删除，单次最多 100 条。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "record-ids", Type: shortcut.FlagStringSlice, Desc: "待删除记录 ID 列表，逗号分隔", Required: true},
	},
	Tips: []string{`dws aitable +record-delete --base-id B --table-id T --record-ids rec1,rec2 --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("delete_records", map[string]any{
			"baseId":    rt.Str("base-id"),
			"tableId":   rt.Str("table-id"),
			"recordIds": rt.StrSlice("record-ids"),
		})
	},
}

// RecordQueryEmpty 查询空行（query_empty_records，server: aitable-helper）。
var RecordQueryEmpty = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-query-empty",
	Product:     serverHelper,
	Description: "扫描并过滤出完全没填用户字段的空行",
	Intent:      "当你想清理表格、需要先找出那些所有用户字段都为空的空行时使用；扫描并返回空行列表，支持分页预算。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "单次扫描预算，范围 [1,100]（可选）"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标（可选）"},
	},
	Tips: []string{`dws aitable +record-query-empty --base-id B --table-id T`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
		}
		if rt.Changed("limit") {
			params["limit"] = rt.Int("limit")
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		return rt.CallMCP("query_empty_records", params)
	},
}

// RecordHistoryList 查询记录变更历史（query_record_history，server: aitable-helper）。
var RecordHistoryList = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-history-list",
	Product:     serverHelper,
	Description: "按 recordId 查询单条记录的变更历史",
	Intent:      "当你要追溯某条记录曾被谁在何时改过哪些字段时使用；按 recordId 分页返回该行的变更历史。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "record-id", Type: shortcut.FlagString, Desc: "记录 ID", Required: true},
		{Name: "offset", Type: shortcut.FlagInt, Desc: "分页偏移量，默认 0（可选）"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量，默认 20，最大 50（可选）"},
	},
	Tips: []string{`dws aitable +record-history-list --base-id B --table-id T --record-id R`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":   rt.Str("base-id"),
			"tableId":  rt.Str("table-id"),
			"recordId": rt.Str("record-id"),
		}
		if rt.Changed("offset") {
			params["offset"] = rt.Int("offset")
		}
		if rt.Changed("limit") {
			params["limit"] = rt.Int("limit")
		}
		return rt.CallMCP("query_record_history", params)
	},
}

// RecordShareURL 批量获取记录分享链接（get_record_share_url，server: aitable-helper）。
var RecordShareURL = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-share-url",
	Product:     serverHelper,
	Description: "按 recordId 批量获取记录分享链接，单次最多 20 条",
	Intent:      "当你要把某几条记录以链接形式分享给他人（可带视图上下文）时使用；按 recordId 批量返回分享链接，单次最多 20 条。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "record-ids", Type: shortcut.FlagStringSlice, Desc: "记录 ID 列表，单次最多 20", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "视图 ID，生成带视图上下文的链接（可选）"},
	},
	Tips: []string{`dws aitable +record-share-url --base-id B --table-id T --record-ids rec1,rec2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":    rt.Str("base-id"),
			"tableId":   rt.Str("table-id"),
			"recordIds": rt.StrSlice("record-ids"),
		}
		if rt.Changed("view-id") {
			params["viewId"] = rt.Str("view-id")
		}
		return rt.CallMCP("get_record_share_url", params)
	},
}

// RecordUpsert 批量创建或更新记录（record_upsert，server: aitable-helper）。
var RecordUpsert = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-upsert",
	Product:     serverHelper,
	Description: "按 records 是否带 recordId 自动拆分 create / update",
	Intent:      "当你有一批数据、其中部分是新增部分是更新、不想自己区分时使用；按记录是否带 recordId 自动拆分为创建或更新并实际写入，单次最多 100 条。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "records", Type: shortcut.FlagString, Desc: "记录 JSON 数组，单次最多 100 条", Required: true},
	},
	Tips: []string{`dws aitable +record-upsert --base-id B --table-id T --records '[{"cells":{...}}]'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		recs, err := parseJSONAny("records", rt.Str("records"))
		if err != nil {
			return err
		}
		return rt.CallMCP("record_upsert", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"records": recs,
		})
	},
}

// RecordPrimaryDocGet 查询记录主键文档（get_primary_doc，server: aitable-helper）。
var RecordPrimaryDocGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-primary-doc-get",
	Product:     serverHelper,
	Description: "查询记录关联的主键文档 nodeId",
	Intent:      "当某记录已关联主键文档、你需要拿到该文档的 nodeId 以便打开或编辑时使用；返回主键文档 nodeId。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "record-id", Type: shortcut.FlagString, Desc: "记录 ID", Required: true},
	},
	Tips: []string{`dws aitable +record-primary-doc-get --base-id B --table-id T --record-id R`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_primary_doc", map[string]any{
			"baseId":   rt.Str("base-id"),
			"tableId":  rt.Str("table-id"),
			"recordId": rt.Str("record-id"),
		})
	},
}

// RecordPrimaryDocCreate 为记录创建主键文档（create_primary_doc，server: aitable-helper）。
var RecordPrimaryDocCreate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-primary-doc-create",
	Product:     serverHelper,
	Description: "为记录创建主键文档（幂等），fieldId 须为 primaryDoc 类型",
	Intent:      "当某记录的主键文档列还没有对应文档、你要为它新建一个时使用；幂等操作，fieldId 须为 primaryDoc 类型，会实际生成主键文档。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "field-id", Type: shortcut.FlagString, Desc: "主键字段 ID（primaryDoc 类型）", Required: true},
		{Name: "record-id", Type: shortcut.FlagString, Desc: "记录 ID", Required: true},
	},
	Tips: []string{`dws aitable +record-primary-doc-create --base-id B --table-id T --field-id F --record-id R`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("create_primary_doc", map[string]any{
			"baseId":   rt.Str("base-id"),
			"tableId":  rt.Str("table-id"),
			"fieldId":  rt.Str("field-id"),
			"recordId": rt.Str("record-id"),
		})
	},
}

// ─────────────────────────────────────────────────────────────
// template / attachment（server: aitable）
// ─────────────────────────────────────────────────────────────

// TemplateSearch 搜索模板（search_templates）。
var TemplateSearch = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+template-search",
	Product:     serverMain,
	Description: "按名称关键词搜索 AI 表格模板",
	Intent:      "当你要新建表格并想套用现成模板、需要先按关键词找模板（不传关键词则返回热门）时使用；返回模板列表及其模板 ID。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "模板名称关键词（可选，不传返回热门）"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量，默认 10，最大 30（可选）"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标（可选）"},
	},
	Tips: []string{`dws aitable +template-search --query "项目管理"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Changed("query") {
			params["query"] = rt.Str("query")
		}
		if rt.Changed("limit") {
			params["limit"] = rt.Int("limit")
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData(serverMain, "search_templates", params)
		if err != nil {
			return err
		}
		templates := templateSearchProject(data)
		return rt.Output(map[string]any{"count": len(templates), "templates": templates})
	},
}

// templateSearchProject reshapes the raw search_templates response into a clean
// {templateId, templateName} list — clean output projection. Both
// the list container and the per-item field names are probed defensively across
// candidate keys, so an empty/unknown shape yields an empty list rather than a
// crash or fabricated data.
func templateSearchProject(data map[string]any) []map[string]any {
	raw := templateSearchResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := templateSearchFirst(m, "templateId", "template_id", "id"); ok {
			row["templateId"] = v
		}
		if v, ok := templateSearchFirst(m, "templateName", "template_name", "name", "title"); ok {
			row["templateName"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// templateSearchResolveList locates the list payload inside the response,
// tolerating a bare top-level array container or nesting one level under a
// common envelope key.
func templateSearchResolveList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, key := range []string{"templates", "result", "data", "list", "items"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"templates", "list", "items", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// templateSearchFirst returns the first present candidate key's value.
func templateSearchFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// AttachmentUpload 准备附件上传（prepare_attachment_upload）。
var AttachmentUpload = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+attachment-upload",
	Product:     serverMain,
	Description: "为 attachment 字段申请 OSS 直传地址（uploadUrl / fileToken）",
	Intent:      "当你要往 attachment（附件）字段上传文件、需要先申请 OSS 直传地址时使用；返回 uploadUrl 和 fileToken，供你直传文件后再写入记录。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "file-name", Type: shortcut.FlagString, Desc: "文件名（含扩展名）", Required: true},
		{Name: "size", Type: shortcut.FlagInt, Desc: "文件大小（字节），须 > 0", Required: true},
		{Name: "mime-type", Type: shortcut.FlagString, Desc: "MIME type，如 image/png（可选）"},
	},
	Tips: []string{`dws aitable +attachment-upload --base-id B --file-name report.xlsx --size 204800`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":   rt.Str("base-id"),
			"fileName": rt.Str("file-name"),
			"size":     rt.Int("size"),
		}
		if rt.Changed("mime-type") {
			params["mimeType"] = rt.Str("mime-type")
		}
		return rt.CallMCP("prepare_attachment_upload", params)
	},
}

// ─────────────────────────────────────────────────────────────
// view: 视图管理（server: aitable / aitable-helper）
// ─────────────────────────────────────────────────────────────

// ViewGet 获取视图详情（get_views）。
var ViewGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-get",
	Product:     serverMain,
	Description: "获取视图完整信息（列顺序、筛选、排序、分组等）",
	Intent:      "当你要了解某个视图当前的列顺序、筛选、排序、分组等完整配置以便复用或修改时使用；批量返回视图详情。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-ids", Type: shortcut.FlagStringSlice, Desc: "View ID 列表，单次最多 10 个（可选）"},
	},
	Tips: []string{`dws aitable +view-get --base-id B --table-id T`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
		}
		if rt.Changed("view-ids") {
			params["viewIds"] = rt.StrSlice("view-ids")
		}
		data, err := rt.CallMCPData(serverMain, "get_views", params)
		if err != nil {
			return err
		}
		views := viewGetProject(data)
		return rt.Output(map[string]any{"count": len(views), "views": views})
	},
}

// viewGetProject reshapes the raw get_views response into a clean
// {viewId, viewName, viewType} list — clean output projection. Both
// the list container and the per-item field names are probed defensively across
// candidate keys, so an empty/unknown shape yields an empty list rather than a
// crash or fabricated data.
func viewGetProject(data map[string]any) []map[string]any {
	raw := viewGetResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := viewGetFirst(m, "viewId", "view_id", "id"); ok {
			row["viewId"] = v
		}
		if v, ok := viewGetFirst(m, "viewName", "view_name", "name", "title"); ok {
			row["viewName"] = v
		}
		if v, ok := viewGetFirst(m, "viewType", "view_type", "type"); ok {
			row["viewType"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// viewGetResolveList locates the list payload inside the response, tolerating a
// bare top-level array container or nesting one level under a common envelope
// key.
func viewGetResolveList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, key := range []string{"views", "result", "data", "list", "items"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"views", "list", "items", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// viewGetFirst returns the first present candidate key's value.
func viewGetFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// ViewCreate 创建视图（create_view）。
// ViewUpdate 更新视图（update_view）。
var ViewUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-update",
	Product:     serverMain,
	Description: "更新视图名称 / 描述 / 配置（visibleFieldIds、filter、sort、group 等）",
	Intent:      "当你要调整视图的展示——改可见列、筛选条件、排序、分组或改名时使用；会实际更新视图配置。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "View ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新视图名（可选）"},
		{Name: "desc", Type: shortcut.FlagString, Desc: "视图描述 JSON（可选）"},
		{Name: "config", Type: shortcut.FlagString, Desc: "视图配置更新项 JSON（可选）"},
	},
	Tips: []string{`dws aitable +view-update --base-id B --table-id T --view-id V --config '{"visibleFieldIds":["fld1"]}'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
		}
		if rt.Changed("name") {
			params["newViewName"] = rt.Str("name")
		}
		if rt.Changed("desc") {
			d, err := parseJSONObject("desc", rt.Str("desc"))
			if err != nil {
				return err
			}
			params["viewDescription"] = d
		}
		if rt.Changed("config") {
			c, err := parseJSONObject("config", rt.Str("config"))
			if err != nil {
				return err
			}
			params["config"] = c
		}
		return rt.CallMCP("update_view", params)
	},
}

// ViewDuplicate 复制视图（duplicate_view，server: aitable-helper）。
var ViewDuplicate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-duplicate",
	Product:     serverHelper,
	Description: "复制视图，生成配置相同的新视图",
	Intent:      "当你想基于某个已配置好的视图快速再造一个相同配置的视图时使用；会实际复制生成新视图。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "源 View ID", Required: true},
		{Name: "new-name", Type: shortcut.FlagString, Desc: "新视图名（可选）"},
	},
	Tips: []string{`dws aitable +view-duplicate --base-id B --table-id T --view-id V`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":       rt.Str("base-id"),
			"tableId":      rt.Str("table-id"),
			"sourceViewId": rt.Str("view-id"),
		}
		if rt.Changed("new-name") {
			params["newViewName"] = rt.Str("new-name")
		}
		return rt.CallMCP("duplicate_view", params)
	},
}

// ViewDelete 删除视图（delete_view）。
var ViewDelete = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-delete",
	Product:     serverMain,
	Description: "删除指定视图（不可逆）",
	Intent:      "当你确认要删除某个视图时使用；不可逆，仅删除该展示视图、不影响底层记录数据。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "待删除 View ID", Required: true},
	},
	Tips: []string{`dws aitable +view-delete --base-id B --table-id T --view-id V --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("delete_view", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
		})
	},
}

// ViewGetLock 获取视图锁定状态（get_view_lock_status，server: aitable-helper）。
var ViewGetLock = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-get-lock",
	Product:     serverHelper,
	Description: "获取视图锁定状态",
	Intent:      "当你想确认某视图是否已被锁定（以防他人误改其配置）时使用；返回该视图的锁定状态。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "View ID", Required: true},
	},
	Tips: []string{`dws aitable +view-get-lock --base-id B --table-id T --view-id V`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_view_lock_status", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
		})
	},
}

// ViewLock 锁定/解锁视图（lock_or_unlock_view，server: aitable-helper）。
var ViewLock = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-lock",
	Product:     serverHelper,
	Description: "锁定视图（默认）或解锁（--off）",
	Intent:      "当你要锁定视图以防止他人修改其配置、或反过来解锁时使用；默认锁定，加 --off 解锁，会实际改变视图锁定状态。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "View ID", Required: true},
		{Name: "off", Type: shortcut.FlagBool, Desc: "传入则解锁（unlock），默认锁定（lock）"},
	},
	Tips: []string{`dws aitable +view-lock --base-id B --table-id T --view-id V`, `dws aitable +view-lock --base-id B --table-id T --view-id V --off`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		action := "lock"
		if rt.Bool("off") {
			action = "unlock"
		}
		return rt.CallMCP("lock_or_unlock_view", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
			"action":  action,
		})
	},
}

// ViewGetFrozenCols 获取视图冻结列数（get_frozen_columns_of_view，server: aitable-helper）。
var ViewGetFrozenCols = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-get-frozen-cols",
	Product:     serverHelper,
	Description: "获取视图当前冻结的左侧列数",
	Intent:      "当你想知道某视图当前冻结了左侧几列时使用；返回冻结列数。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "View ID", Required: true},
	},
	Tips: []string{`dws aitable +view-get-frozen-cols --base-id B --table-id T --view-id V`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_frozen_columns_of_view", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
		})
	},
}

// ViewSetFrozenCols 设置视图冻结列数（set_frozen_columns_of_view，server: aitable-helper）。
var ViewSetFrozenCols = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-set-frozen-cols",
	Product:     serverHelper,
	Description: "设置视图冻结列数（0 表示取消冻结）",
	Intent:      "当你要冻结视图左侧若干列、以便横向滚动时保持这些列可见（0 表示取消冻结）时使用；会实际修改视图冻结列数。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "View ID", Required: true},
		{Name: "count", Type: shortcut.FlagInt, Desc: "冻结列数，须 >= 0", Required: true},
	},
	Tips: []string{`dws aitable +view-set-frozen-cols --base-id B --table-id T --view-id V --count 1`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("set_frozen_columns_of_view", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
			"count":   rt.Int("count"),
		})
	},
}

// ViewGetRowHeight 获取视图行高（get_cell_height_of_view，server: aitable-helper）。
var ViewGetRowHeight = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-get-row-height",
	Product:     serverHelper,
	Description: "获取视图单元格行高（像素）",
	Intent:      "当你想知道某视图当前的行高档位时使用；返回单元格行高的像素值。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "View ID", Required: true},
	},
	Tips: []string{`dws aitable +view-get-row-height --base-id B --table-id T --view-id V`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_cell_height_of_view", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
		})
	},
}

// ViewSetRowHeight 设置视图行高（set_cell_height_of_view，server: aitable-helper）。
var ViewSetRowHeight = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-set-row-height",
	Product:     serverHelper,
	Description: "设置视图单元格行高（像素，合法档位 32/56/88/128）",
	Intent:      "当你要调整视图行高、让内容显示更宽松或更紧凑（合法档位 32/56/88/128）时使用；会实际修改视图行高。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "View ID", Required: true},
		{Name: "cell-height", Type: shortcut.FlagInt, Desc: "单元格高度（像素），须 > 0", Required: true},
	},
	Tips: []string{`dws aitable +view-set-row-height --base-id B --table-id T --view-id V --cell-height 56`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("set_cell_height_of_view", map[string]any{
			"baseId":     rt.Str("base-id"),
			"tableId":    rt.Str("table-id"),
			"viewId":     rt.Str("view-id"),
			"cellHeight": rt.Int("cell-height"),
		})
	},
}

// ViewSetFillColorRule 更新视图数据高亮规则（set_view_fill_color_rule，server: aitable）。
var ViewSetFillColorRule = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-set-fill-color-rule",
	Product:     serverMain,
	Description: "全量覆盖 Grid 视图的条件填色规则（传 '[]' 清空）",
	Intent:      "当你要为 Grid 视图设置条件填色（按规则给符合条件的行/单元格上色），或传空数组清空所有填色规则时使用；会全量覆盖该视图的填色规则。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "View ID", Required: true},
		{Name: "json", Type: shortcut.FlagString, Desc: "conditionalFormats JSON 数组", Required: true},
	},
	Tips: []string{`dws aitable +view-set-fill-color-rule --base-id B --table-id T --view-id V --json '[]'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		arr, err := parseJSONAny("json", rt.Str("json"))
		if err != nil {
			return err
		}
		return rt.CallMCP("set_view_fill_color_rule", map[string]any{
			"baseId":             rt.Str("base-id"),
			"tableId":            rt.Str("table-id"),
			"viewId":             rt.Str("view-id"),
			"conditionalFormats": arr,
		})
	},
}

// ─────────────────────────────────────────────────────────────
// form: 表单管理（server: aitable / aitable-helper）
// ─────────────────────────────────────────────────────────────

// FormCreate 创建表单视图（create_view, viewType=FormDesigner，server: aitable）。
// FormList 列出表单视图（list_form_views，server: aitable-helper）。
var FormList = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+form-list",
	Product:     serverHelper,
	Description: "列出指定数据表下的所有表单视图",
	Intent:      "当你要查看某数据表下已有哪些收集表单时使用；返回该表的全部表单视图列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
	},
	Tips: []string{`dws aitable +form-list --base-id B --table-id T`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData(serverHelper, "list_form_views", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
		})
		if err != nil {
			return err
		}
		forms := formListProject(data)
		return rt.Output(map[string]any{"count": len(forms), "forms": forms})
	},
}

// formListProject reshapes the raw list_form_views response into a clean form
// view list ({viewId, viewName, viewType}) — clean output projection.
// Both the list container and the per-item field names are probed
// defensively across candidate keys, so an empty/unknown shape yields an empty
// list rather than a crash or fabricated data.
func formListProject(data map[string]any) []map[string]any {
	raw := formListResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := formListFirst(m, "viewId", "view_id", "id"); ok {
			row["viewId"] = v
		}
		if v, ok := formListFirst(m, "viewName", "view_name", "name", "title"); ok {
			row["viewName"] = v
		}
		if v, ok := formListFirst(m, "viewType", "view_type", "type"); ok {
			row["viewType"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// formListResolveList locates the list payload inside the response, tolerating a
// bare top-level array container or nesting one level under a common envelope.
func formListResolveList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, key := range []string{"forms", "views", "formViews", "result", "data", "list", "items"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"forms", "views", "formViews", "list", "items", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// formListFirst returns the first present candidate key's value.
func formListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// FormDelete 删除表单（delete_form_view，server: aitable-helper）。
var FormDelete = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+form-delete",
	Product:     serverHelper,
	Description: "删除指定表单视图（不可逆）",
	Intent:      "当你确认要删除某个收集表单时使用；不可逆，仅删除该表单视图。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "表单 View ID", Required: true},
	},
	Tips: []string{`dws aitable +form-delete --base-id B --table-id T --view-id V --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("delete_form_view", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
		})
	},
}

// FormUpdate 更新表单配置（update_form_info，server: aitable-helper）。
var FormUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+form-update",
	Product:     serverHelper,
	Description: "更新表单标题 / 描述",
	Intent:      "当你要修改表单对外展示的标题或说明文案时使用；会实际更新表单的标题/描述。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "表单 View ID", Required: true},
		{Name: "title", Type: shortcut.FlagString, Desc: "表单标题（可选）"},
		{Name: "description", Type: shortcut.FlagString, Desc: "表单描述（可选）"},
	},
	Tips: []string{`dws aitable +form-update --base-id B --table-id T --view-id V --title "员工信息收集"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
		}
		if rt.Changed("title") {
			params["title"] = rt.Str("title")
		}
		if rt.Changed("description") {
			params["description"] = rt.Str("description")
		}
		return rt.CallMCP("update_form_info", params)
	},
}

// FormFieldList 列出表单字段（list_form_fields，server: aitable-helper）。
var FormFieldList = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+form-field-list",
	Product:     serverHelper,
	Description: "列出表单视图当前可见的字段及其配置",
	Intent:      "当你要查看某表单当前放出了哪些字段供填写及其是否必填等配置时使用；返回表单可见字段列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "表单 View ID", Required: true},
	},
	Tips: []string{`dws aitable +form-field-list --base-id B --table-id T --view-id V`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("list_form_fields", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
		})
	},
}

// FormFieldUpdate 更新表单字段（update_form_field，server: aitable-helper）。
var FormFieldUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+form-field-update",
	Product:     serverHelper,
	Description: "更新表单字段的必填状态或描述",
	Intent:      "当你要把表单里某个字段设为必填/非必填，或补充其填写说明时使用；会实际更新该表单字段的配置。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "表单 View ID", Required: true},
		{Name: "field-id", Type: shortcut.FlagString, Desc: "Field ID", Required: true},
		{Name: "required", Type: shortcut.FlagBool, Desc: "是否必填（可选）"},
		{Name: "field-description", Type: shortcut.FlagString, Desc: "字段描述（可选）"},
	},
	Tips: []string{`dws aitable +form-field-update --base-id B --table-id T --view-id V --field-id F --required true`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
			"fieldId": rt.Str("field-id"),
		}
		if rt.Changed("required") {
			params["required"] = rt.Bool("required")
		}
		if rt.Changed("field-description") {
			params["fieldDescription"] = rt.Str("field-description")
		}
		return rt.CallMCP("update_form_field", params)
	},
}

// FormFieldHide 切换表单字段隐藏（update_form_field_hidden，server: aitable-helper）。
var FormFieldHide = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+form-field-hide",
	Product:     serverHelper,
	Description: "切换表单字段的隐藏/显示状态",
	Intent:      "当你要在表单里隐藏或重新显示某个字段时使用；会实际切换该字段在表单中的显示状态。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "表单 View ID", Required: true},
		{Name: "field-id", Type: shortcut.FlagString, Desc: "Field ID", Required: true},
		{Name: "hidden", Type: shortcut.FlagBool, Desc: "true 隐藏 / false 显示", Required: true},
	},
	Tips: []string{`dws aitable +form-field-hide --base-id B --table-id T --view-id V --field-id F --hidden true`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_form_field_hidden", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
			"fieldId": rt.Str("field-id"),
			"hidden":  rt.Bool("hidden"),
		})
	},
}

// FormShareGet 获取表单分享配置（get_share_form_config，server: aitable-helper）。
var FormShareGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+form-share-get",
	Product:     serverHelper,
	Description: "读取视图当前的分享表单配置",
	Intent:      "当你要查看某视图的表单分享是否已开启及其分享配置时使用；返回当前的分享表单配置。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "View ID", Required: true},
	},
	Tips: []string{`dws aitable +form-share-get --base-id B --table-id T --view-id V`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_share_form_config", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
		})
	},
}

// FormShareUpdate 开启/关闭分享表单（update_share_form，server: aitable-helper）。
var FormShareUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+form-share-update",
	Product:     serverHelper,
	Description: "开启或关闭指定视图的分享表单",
	Intent:      "当你要对外开启或关闭某表单的分享（生成或停用可对外填写的链接）时使用；会实际改变表单分享开关。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "View ID", Required: true},
		{Name: "enabled", Type: shortcut.FlagString, Desc: "开启/关闭分享", Required: true, Enum: []string{"true", "false"}},
	},
	Tips: []string{`dws aitable +form-share-update --base-id B --table-id T --view-id V --enabled true`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// helper 透传 enabled 的字符串值，保持一致。
		return rt.CallMCP("update_share_form", map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
			"viewId":  rt.Str("view-id"),
			"enabled": rt.Str("enabled"),
		})
	},
}

// ─────────────────────────────────────────────────────────────
// workflow: 自动化工作流（server: aitable-helper）
// ─────────────────────────────────────────────────────────────

// WorkflowEnable 启用工作流（enable_workflow）。
var WorkflowEnable = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+workflow-enable",
	Product:     serverHelper,
	Description: "启用指定 Base 中的自动化工作流",
	Intent:      "当你要启用某个已配置好的自动化工作流、让它重新生效时使用；会实际把该 workflow 置为启用。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "workflow-id", Type: shortcut.FlagString, Desc: "Workflow ID", Required: true},
	},
	Tips: []string{`dws aitable +workflow-enable --base-id B --workflow-id W`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("enable_workflow", map[string]any{
			"baseId":     rt.Str("base-id"),
			"workflowId": rt.Str("workflow-id"),
		})
	},
}

// WorkflowDisable 禁用工作流（disable_workflow）。
var WorkflowDisable = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+workflow-disable",
	Product:     serverHelper,
	Description: "禁用指定 Base 中的自动化工作流（影响业务自动化）",
	Intent:      "当你要停用某个自动化工作流时使用；会实际停用该 workflow，可能中断依赖它的业务自动化，请谨慎确认。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "workflow-id", Type: shortcut.FlagString, Desc: "Workflow ID", Required: true},
	},
	Tips: []string{`dws aitable +workflow-disable --base-id B --workflow-id W --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("disable_workflow", map[string]any{
			"baseId":     rt.Str("base-id"),
			"workflowId": rt.Str("workflow-id"),
		})
	},
}

// WorkflowGet 获取工作流详情（get_workflow）。
var WorkflowGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+workflow-get",
	Product:     serverHelper,
	Description: "获取单个自动化工作流的详细信息",
	Intent:      "当你要查看某个自动化工作流的触发条件与执行动作等详情时使用；返回单个 workflow 的完整信息。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "workflow-id", Type: shortcut.FlagString, Desc: "Workflow ID", Required: true},
	},
	Tips: []string{`dws aitable +workflow-get --base-id B --workflow-id W`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_workflow", map[string]any{
			"baseId":     rt.Str("base-id"),
			"workflowId": rt.Str("workflow-id"),
		})
	},
}

// WorkflowList 列出工作流（list_workflows）。
var WorkflowList = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+workflow-list",
	Product:     serverHelper,
	Description: "列出指定 Base 中的自动化工作流（分页）",
	Intent:      "当你想了解某 Base 里配置了哪些自动化工作流及其开关状态时使用；分页返回 workflow 列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量，默认 20，最大 100（可选）"},
		{Name: "offset", Type: shortcut.FlagInt, Desc: "分页偏移量，默认 0（可选）"},
	},
	Tips: []string{`dws aitable +workflow-list --base-id B`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"baseId": rt.Str("base-id")}
		if rt.Changed("limit") {
			params["limit"] = rt.Int("limit")
		}
		if rt.Changed("offset") {
			params["offset"] = rt.Int("offset")
		}
		data, err := rt.CallMCPData(serverHelper, "list_workflows", params)
		if err != nil {
			return err
		}
		workflows := workflowListProject(data)
		return rt.Output(map[string]any{"count": len(workflows), "workflows": workflows})
	},
}

// workflowListProject reshapes the raw list_workflows response into a clean
// workflow list ({workflowId, name, status/enabled}) — output-projection
// clean output projection. Both the list container and the per-item field names are
// probed defensively across candidate keys, so an empty/unknown shape yields an
// empty list rather than a crash or fabricated data.
func workflowListProject(data map[string]any) []map[string]any {
	raw := workflowListResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := workflowListFirst(m, "workflowId", "workflow_id", "id"); ok {
			row["workflowId"] = v
		}
		if v, ok := workflowListFirst(m, "name", "workflowName", "title"); ok {
			row["name"] = v
		}
		if v, ok := workflowListFirst(m, "status", "enabled", "isEnabled", "state"); ok {
			row["status"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// workflowListResolveList locates the list payload inside the response,
// tolerating a bare top-level array container or nesting one level under a
// common envelope.
func workflowListResolveList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, key := range []string{"workflows", "result", "data", "list", "items"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"workflows", "list", "items", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// workflowListFirst returns the first present candidate key's value.
func workflowListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// ─────────────────────────────────────────────────────────────
// dashboard: 仪表盘管理（server: aitable / aitable-helper）
// ─────────────────────────────────────────────────────────────

// DashboardConfigExample 获取仪表盘配置示例（get_dashboard_config_example）。
var DashboardConfigExample = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+dashboard-config-example",
	Product:     serverMain,
	Description: "获取 dashboard config 的结构示例",
	Intent:      "当你准备创建或更新仪表盘、需要先了解 dashboard config 的字段结构长什么样时使用；返回一份配置结构示例供参考。",
	Risk:        shortcut.RiskRead,
	Tips:        []string{`dws aitable +dashboard-config-example`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_dashboard_config_example", map[string]any{})
	},
}

// DashboardGet 获取仪表盘信息（get_dashboard）。
var DashboardGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+dashboard-get",
	Product:     serverMain,
	Description: "获取指定 dashboard 的详细信息（含 charts summary）",
	Intent:      "当你要查看某仪表盘的配置详情及它包含哪些图表（拿 chartId）时使用；返回 dashboard 信息与 charts 概要。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true},
	},
	Tips: []string{`dws aitable +dashboard-get --base-id B --dashboard-id D`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_dashboard", map[string]any{
			"baseId":      rt.Str("base-id"),
			"dashboardId": rt.Str("dashboard-id"),
		})
	},
}

// DashboardCreate 创建仪表盘（create_dashboard）。
// DashboardUpdate 更新仪表盘（update_dashboard）。
var DashboardUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+dashboard-update",
	Product:     serverMain,
	Description: "更新指定 dashboard 的配置",
	Intent:      "当你要修改仪表盘的名称或整体配置时使用；会实际更新指定 dashboard 的配置。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true},
		{Name: "config", Type: shortcut.FlagString, Desc: "dashboard 配置 JSON（可选，与 --name 二选一）"},
		{Name: "name", Type: shortcut.FlagString, Desc: "dashboard 名称（可选，与 --config 二选一）"},
	},
	Tips: []string{`dws aitable +dashboard-update --base-id B --dashboard-id D --name "新名称"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		cfg := map[string]any{}
		if rt.Changed("config") {
			m, err := parseJSONObject("config", rt.Str("config"))
			if err != nil {
				return err
			}
			cfg = m
		}
		if rt.Changed("name") {
			cfg["name"] = rt.Str("name")
		}
		if len(cfg) == 0 {
			return fmt.Errorf("必须指定 --config 或 --name")
		}
		return rt.CallMCP("update_dashboard", map[string]any{
			"baseId":      rt.Str("base-id"),
			"dashboardId": rt.Str("dashboard-id"),
			"config":      cfg,
		})
	},
}

// DashboardDelete 删除仪表盘（delete_dashboard）。
var DashboardDelete = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+dashboard-delete",
	Product:     serverMain,
	Description: "删除指定 dashboard（级联删除其 chart，不可逆）",
	Intent:      "当你确认要删除某个仪表盘时使用；会级联删除其下所有图表，不可逆。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true},
		{Name: "reason", Type: shortcut.FlagString, Desc: "删除原因（可选）"},
	},
	Tips: []string{`dws aitable +dashboard-delete --base-id B --dashboard-id D --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":      rt.Str("base-id"),
			"dashboardId": rt.Str("dashboard-id"),
		}
		if rt.Changed("reason") {
			params["reason"] = rt.Str("reason")
		}
		return rt.CallMCP("delete_dashboard", params)
	},
}

// DashboardArrange 自动重排仪表盘图表布局（align_dashboard，server: aitable-helper）。
var DashboardArrange = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+dashboard-arrange",
	Product:     serverHelper,
	Description: "对指定仪表盘做服务端智能布局重排",
	Intent:      "当仪表盘里的图表排布凌乱、你想让系统自动重新排版对齐时使用；会实际调整该仪表盘的图表布局。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true},
	},
	Tips: []string{`dws aitable +dashboard-arrange --base-id B --dashboard-id D`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("align_dashboard", map[string]any{
			"baseId":      rt.Str("base-id"),
			"dashboardId": rt.Str("dashboard-id"),
		})
	},
}

// DashboardShareGet 获取仪表盘分享配置（get_dashboard_share）。
var DashboardShareGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+dashboard-share-get",
	Product:     serverMain,
	Description: "查询 dashboard 的分享配置",
	Intent:      "当你要查看某仪表盘是否已开启对外分享及其分享方式时使用；返回 dashboard 的分享配置。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true},
	},
	Tips: []string{`dws aitable +dashboard-share-get --base-id B --dashboard-id D`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_dashboard_share", map[string]any{
			"baseId":      rt.Str("base-id"),
			"dashboardId": rt.Str("dashboard-id"),
		})
	},
}

// DashboardShareUpdate 更新仪表盘分享配置（update_dashboard_share）。
var DashboardShareUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+dashboard-share-update",
	Product:     serverMain,
	Description: "开启/关闭 dashboard 分享并可设置分享类型",
	Intent:      "当你要对外开启或关闭仪表盘分享（可选公开 PUBLIC 或仅组织内 ORG）时使用；会实际改变 dashboard 的分享设置。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true},
		{Name: "enabled", Type: shortcut.FlagBool, Desc: "是否开启分享", Required: true},
		{Name: "share-type", Type: shortcut.FlagString, Desc: "分享类型（仅开启时生效）", Enum: []string{"PUBLIC", "ORG"}},
		{Name: "allow-back-to-doc", Type: shortcut.FlagBool, Desc: "是否允许回到文档（可选）"},
	},
	Tips: []string{`dws aitable +dashboard-share-update --base-id B --dashboard-id D --enabled true --share-type PUBLIC`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":      rt.Str("base-id"),
			"dashboardId": rt.Str("dashboard-id"),
			"enabled":     rt.Bool("enabled"),
		}
		if rt.Changed("share-type") {
			params["shareType"] = rt.Str("share-type")
		}
		if rt.Changed("allow-back-to-doc") {
			params["allowBackToDoc"] = rt.Bool("allow-back-to-doc")
		}
		return rt.CallMCP("update_dashboard_share", params)
	},
}

// ─────────────────────────────────────────────────────────────
// chart: 图表管理（server: aitable）
// ─────────────────────────────────────────────────────────────

// ChartWidgetsExample 获取图表配置示例（get_dashboard_widgets_example）。
var ChartWidgetsExample = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+chart-widgets-example",
	Product:     serverMain,
	Description: "获取所有图表类型的 widget config 示例",
	Intent:      "当你准备创建或修改图表、需要先参考各类图表 widget config 的示例结构时使用；返回所有图表类型的配置示例。",
	Risk:        shortcut.RiskRead,
	Tips:        []string{`dws aitable +chart-widgets-example`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_dashboard_widgets_example", map[string]any{})
	},
}

// ChartGet 获取图表信息（get_chart）。
var ChartGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+chart-get",
	Product:     serverMain,
	Description: "获取指定 chart 的详细信息",
	Intent:      "当你要查看某个图表的配置详情（统计维度、样式等）时使用；返回指定 chart 的详细信息。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true},
		{Name: "chart-id", Type: shortcut.FlagString, Desc: "Chart ID", Required: true},
	},
	Tips: []string{`dws aitable +chart-get --base-id B --dashboard-id D --chart-id C`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_chart", map[string]any{
			"baseId":      rt.Str("base-id"),
			"dashboardId": rt.Str("dashboard-id"),
			"chartId":     rt.Str("chart-id"),
		})
	},
}

// ChartCreate 创建图表（create_chart）。
// ChartUpdate 更新图表（update_chart）。
var ChartUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+chart-update",
	Product:     serverMain,
	Description: "更新指定 chart 的配置或布局（--config 必填）",
	Intent:      "当你要修改某图表的配置（如改名、换统计维度）或调整其在仪表盘上的布局时使用；会实际更新 chart，config 必填。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true},
		{Name: "chart-id", Type: shortcut.FlagString, Desc: "Chart ID", Required: true},
		{Name: "config", Type: shortcut.FlagString, Desc: "图表配置 JSON（至少含 chartName）", Required: true},
		{Name: "layout", Type: shortcut.FlagString, Desc: "布局 JSON（可选）"},
	},
	Tips: []string{`dws aitable +chart-update --base-id B --dashboard-id D --chart-id C --config '{"chartName":"柱图"}'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		cfg, err := parseJSONObject("config", rt.Str("config"))
		if err != nil {
			return err
		}
		params := map[string]any{
			"baseId":      rt.Str("base-id"),
			"dashboardId": rt.Str("dashboard-id"),
			"chartId":     rt.Str("chart-id"),
			"config":      cfg,
		}
		if rt.Changed("layout") {
			layout, err := parseJSONObject("layout", rt.Str("layout"))
			if err != nil {
				return err
			}
			params["layout"] = layout
		}
		return rt.CallMCP("update_chart", params)
	},
}

// ChartDelete 删除图表（delete_chart）。
var ChartDelete = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+chart-delete",
	Product:     serverMain,
	Description: "删除指定 chart 及其布局项（不可逆）",
	Intent:      "当你确认要删除某个图表时使用；会连同其在仪表盘上的布局项一并移除，不可逆。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true},
		{Name: "chart-id", Type: shortcut.FlagString, Desc: "Chart ID", Required: true},
		{Name: "reason", Type: shortcut.FlagString, Desc: "删除原因（可选）"},
	},
	Tips: []string{`dws aitable +chart-delete --base-id B --dashboard-id D --chart-id C --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":      rt.Str("base-id"),
			"dashboardId": rt.Str("dashboard-id"),
			"chartId":     rt.Str("chart-id"),
		}
		if rt.Changed("reason") {
			params["reason"] = rt.Str("reason")
		}
		return rt.CallMCP("delete_chart", params)
	},
}

// ChartShareGet 获取图表分享配置（get_chart_share）。
var ChartShareGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+chart-share-get",
	Product:     serverMain,
	Description: "查询 chart 的分享配置",
	Intent:      "当你要查看某图表是否已开启对外分享及其分享方式时使用；返回 chart 的分享配置。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true},
		{Name: "chart-id", Type: shortcut.FlagString, Desc: "Chart ID", Required: true},
	},
	Tips: []string{`dws aitable +chart-share-get --base-id B --dashboard-id D --chart-id C`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_chart_share", map[string]any{
			"baseId":      rt.Str("base-id"),
			"dashboardId": rt.Str("dashboard-id"),
			"chartId":     rt.Str("chart-id"),
		})
	},
}

// ChartShareUpdate 更新图表分享配置（update_chart_share）。
var ChartShareUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+chart-share-update",
	Product:     serverMain,
	Description: "开启/关闭 chart 分享并可设置分享类型",
	Intent:      "当你要对外开启或关闭单个图表的分享（可选公开 PUBLIC 或仅组织内 ORG）时使用；会实际改变 chart 的分享设置。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true},
		{Name: "chart-id", Type: shortcut.FlagString, Desc: "Chart ID", Required: true},
		{Name: "enabled", Type: shortcut.FlagBool, Desc: "是否开启分享", Required: true},
		{Name: "share-type", Type: shortcut.FlagString, Desc: "分享类型（仅开启时生效）", Enum: []string{"PUBLIC", "ORG"}},
		{Name: "allow-back-to-doc", Type: shortcut.FlagBool, Desc: "是否允许回到文档（可选）"},
	},
	Tips: []string{`dws aitable +chart-share-update --base-id B --dashboard-id D --chart-id C --enabled true --share-type ORG`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":      rt.Str("base-id"),
			"dashboardId": rt.Str("dashboard-id"),
			"chartId":     rt.Str("chart-id"),
			"enabled":     rt.Bool("enabled"),
		}
		if rt.Changed("share-type") {
			params["shareType"] = rt.Str("share-type")
		}
		if rt.Changed("allow-back-to-doc") {
			params["allowBackToDoc"] = rt.Bool("allow-back-to-doc")
		}
		return rt.CallMCP("update_chart_share", params)
	},
}

// ─────────────────────────────────────────────────────────────
// export / import（server: aitable）
// ─────────────────────────────────────────────────────────────

// ExportData 导出数据（export_data）。
var ExportData = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+export-data",
	Product:     serverMain,
	Description: "导出 AI 表格数据（创建导出任务或按 taskId 续等）",
	Intent:      "当你要把 AI 表格数据导出为 Excel/附件（可选整个 Base、某张表或某个视图），或用已有 taskId 继续等待之前导出任务完成时使用；返回导出任务状态与下载结果。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "task-id", Type: shortcut.FlagString, Desc: "已有导出任务 ID（传入则续等，不重新创建）"},
		{Name: "scope", Type: shortcut.FlagString, Desc: "导出范围", Enum: []string{"all", "table", "view"}},
		{Name: "format", Type: shortcut.FlagString, Desc: "导出格式",
			Enum: []string{"excel", "attachment", "excel_and_attachment", "excel_with_inline_images"}},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID（scope=table/view 时）"},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "View ID（scope=view 时）"},
		{Name: "timeout-ms", Type: shortcut.FlagInt, Desc: "同步等待超时（毫秒，可选）"},
	},
	Tips: []string{`dws aitable +export-data --base-id B --scope all --format excel`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"baseId": rt.Str("base-id")}
		if rt.Changed("task-id") {
			params["taskId"] = rt.Str("task-id")
		} else {
			if !rt.Changed("scope") || !rt.Changed("format") {
				return fmt.Errorf("需指定 --task-id，或同时指定 --scope 和 --format")
			}
			params["scope"] = rt.Str("scope")
			params["format"] = rt.Str("format")
		}
		if rt.Changed("table-id") {
			params["tableId"] = rt.Str("table-id")
		}
		if rt.Changed("view-id") {
			params["viewId"] = rt.Str("view-id")
		}
		if rt.Changed("timeout-ms") {
			params["timeoutMs"] = rt.Int("timeout-ms")
		}
		return rt.CallMCP("export_data", params)
	},
}

// ImportUpload 准备导入文件上传（prepare_import_upload）。
var ImportUpload = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+import-upload",
	Product:     serverMain,
	Description: "为导入任务申请 OSS 直传地址（uploadUrl / importId）",
	Intent:      "当你要把本地文件（如 Excel）导入 AI 表格、需要先申请上传地址时使用；返回 uploadUrl 和 importId，供直传文件后再调用导入。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "file-name", Type: shortcut.FlagString, Desc: "文件名（含扩展名）", Required: true},
		{Name: "file-size", Type: shortcut.FlagInt, Desc: "文件大小（字节，必须大于 0）", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"file-size"},
			Description: "--file-size 必须是大于 0 的整数（实际文件大小，单位字节）",
		},
	},
	Tips: []string{`dws aitable +import-upload --base-id B --file-name data.xlsx --file-size 204800`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("file-size") <= 0 {
			return fmt.Errorf("flag --file-size is required and must be a positive integer")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":   rt.Str("base-id"),
			"fileName": rt.Str("file-name"),
			"fileSize": rt.Int("file-size"),
		}
		return rt.CallMCP("prepare_import_upload", params)
	},
}

// ImportData 导入数据（import_data）。
var ImportData = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+import-data",
	Product:     serverMain,
	Description: "将已上传文件导入 AI 表格（新建表或追加到已有表）",
	Intent:      "当文件已上传、你要真正把它导入成新表或追加到已有表（可设表头行、指定源 Sheet、做字段映射）时使用；会实际写入数据。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "import-id", Type: shortcut.FlagString, Desc: "import upload 返回的 importId", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "追加导入的目标 Table ID（可选）"},
		{Name: "timeout", Type: shortcut.FlagInt, Desc: "等待超时（可选）"},
		{Name: "header-row", Type: shortcut.FlagInt, Desc: "表头行号（可选）"},
		{Name: "src-sheet-name", Type: shortcut.FlagString, Desc: "源 Sheet 名（可选）"},
		{Name: "field-mapping", Type: shortcut.FlagString, Desc: "字段映射 JSON 对象，key=目标字段名 value=源列名（可选）"},
	},
	Tips: []string{`dws aitable +import-data --import-id IMPORT_ID`, `dws aitable +import-data --import-id IMPORT_ID --table-id T`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"importId": rt.Str("import-id")}
		if rt.Changed("table-id") {
			params["tableId"] = rt.Str("table-id")
		}
		if rt.Changed("timeout") {
			params["timeout"] = rt.Int("timeout")
		}
		if rt.Changed("header-row") {
			params["headerRow"] = rt.Int("header-row")
		}
		if rt.Changed("src-sheet-name") {
			params["srcSheetName"] = rt.Str("src-sheet-name")
		}
		if rt.Changed("field-mapping") {
			m, err := parseJSONObject("field-mapping", rt.Str("field-mapping"))
			if err != nil {
				return err
			}
			params["fieldMapping"] = m
		}
		return rt.CallMCP("import_data", params)
	},
}

// ─────────────────────────────────────────────────────────────
// advperm / role: 高级权限与自定义角色（server: aitable-helper）
// ─────────────────────────────────────────────────────────────

// AdvpermEnable 开启高级权限总开关（set_advanced_permission, enabled=true）。
var AdvpermEnable = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+advperm-enable",
	Product:     serverHelper,
	Description: "开启指定 Base 的高级权限总开关",
	Intent:      "当你要为某 Base 打开高级权限总开关、以便后续配置自定义角色和精细权限时使用；会实际开启该 Base 的高级权限。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
	},
	Tips: []string{`dws aitable +advperm-enable --base-id B`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("set_advanced_permission", map[string]any{
			"baseId":  rt.Str("base-id"),
			"enabled": true,
		})
	},
}

// AdvpermDisable 关闭高级权限总开关（set_advanced_permission, enabled=false）。
var AdvpermDisable = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+advperm-disable",
	Product:     serverHelper,
	Description: "关闭指定 Base 的高级权限总开关（所有自定义角色失效）",
	Intent:      "当你要关闭某 Base 的高级权限总开关时使用；会使该 Base 下所有自定义角色失效、影响成员访问权限，请谨慎确认。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
	},
	Tips: []string{`dws aitable +advperm-disable --base-id B --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("set_advanced_permission", map[string]any{
			"baseId":  rt.Str("base-id"),
			"enabled": false,
		})
	},
}

// RoleList 列出 Base 下所有角色（list_roles）。
var RoleList = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+role-list",
	Product:     serverHelper,
	Description: "列出指定 Base 下的全部角色",
	Intent:      "当你要查看某 Base 下配置了哪些角色（拿 roleId）时使用；返回全部角色列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
	},
	Tips: []string{`dws aitable +role-list --base-id B`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("list_roles", map[string]any{"baseId": rt.Str("base-id")})
	},
}

// RoleGet 获取单个角色配置（get_role）。
var RoleGet = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+role-get",
	Product:     serverHelper,
	Description: "获取单个角色的完整配置",
	Intent:      "当你要查看某个角色的完整权限配置（含各表/字段的授权级别）时使用；返回单个角色的详情。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "role-id", Type: shortcut.FlagString, Desc: "Role ID", Required: true},
	},
	Tips: []string{`dws aitable +role-get --base-id B --role-id R`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_role", map[string]any{
			"baseId": rt.Str("base-id"),
			"roleId": rt.Str("role-id"),
		})
	},
}

// RoleCreate 创建自定义角色（create_role）。
var RoleCreate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+role-create",
	Product:     serverHelper,
	Description: "在指定 Base 下创建自定义角色",
	Intent:      "当你要在已开启高级权限的 Base 下新建一个自定义角色并设定其对各表的读写权限时使用；会实际创建角色。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "角色名称", Required: true},
		{Name: "role-type", Type: shortcut.FlagString, Desc: "角色类型（可选）"},
		{Name: "flow-type", Type: shortcut.FlagString, Desc: "流转类型（可选）"},
		{Name: "sub-roles", Type: shortcut.FlagString, Desc: "子角色 JSON 数组（可选）"},
	},
	Tips: []string{`dws aitable +role-create --base-id B --name "市场可读" --sub-roles '[{"targetId":"tbl","targetType":"sheet","authLevel":"read"}]'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId": rt.Str("base-id"),
			"name":   rt.Str("name"),
		}
		if rt.Changed("role-type") {
			params["roleType"] = rt.Str("role-type")
		}
		if rt.Changed("flow-type") {
			params["flowType"] = rt.Str("flow-type")
		}
		if rt.Changed("sub-roles") {
			sr, err := parseJSONAny("sub-roles", rt.Str("sub-roles"))
			if err != nil {
				return err
			}
			params["subRoles"] = sr
		}
		return rt.CallMCP("create_role", params)
	},
}

// RoleUpdate 增量更新自定义角色（patch_role）。
var RoleUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+role-update",
	Product:     serverHelper,
	Description: "按 PATCH 语义增量更新自定义角色",
	Intent:      "当你要按需增量调整某自定义角色的名称或子权限（PATCH 语义，只改传入项）时使用；会实际更新该角色。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "role-id", Type: shortcut.FlagString, Desc: "Role ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新角色名（可选）"},
		{Name: "role-type", Type: shortcut.FlagString, Desc: "角色类型（可选）"},
		{Name: "flow-type", Type: shortcut.FlagString, Desc: "流转类型（可选）"},
		{Name: "sub-roles", Type: shortcut.FlagString, Desc: "子角色 JSON 数组（可选）"},
	},
	Tips: []string{`dws aitable +role-update --base-id B --role-id R --name "新名字"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId": rt.Str("base-id"),
			"roleId": rt.Str("role-id"),
		}
		if rt.Changed("name") {
			params["name"] = rt.Str("name")
		}
		if rt.Changed("role-type") {
			params["roleType"] = rt.Str("role-type")
		}
		if rt.Changed("flow-type") {
			params["flowType"] = rt.Str("flow-type")
		}
		if rt.Changed("sub-roles") {
			sr, err := parseJSONAny("sub-roles", rt.Str("sub-roles"))
			if err != nil {
				return err
			}
			params["subRoles"] = sr
		}
		return rt.CallMCP("patch_role", params)
	},
}

// RoleDelete 删除自定义角色（delete_role）。
var RoleDelete = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+role-delete",
	Product:     serverHelper,
	Description: "删除 Base 下指定的自定义角色（不可逆）",
	Intent:      "当你确认要删除某个自定义角色时使用；不可逆，删除后被授予该角色的成员将失去对应权限。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "role-id", Type: shortcut.FlagString, Desc: "Role ID（数字 long 字符串）", Required: true},
	},
	Tips: []string{`dws aitable +role-delete --base-id B --role-id R --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("delete_role", map[string]any{
			"baseId": rt.Str("base-id"),
			"roleId": rt.Str("role-id"),
		})
	},
}

// ─────────────────────────────────────────────────────────────
// section: 文件夹与节点管理（server: aitable-helper）
// ─────────────────────────────────────────────────────────────

// SectionCreate 创建文件夹（create_section）。
var SectionCreate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+section-create",
	Product:     serverHelper,
	Description: "在指定 Base 下创建文件夹（组织 table / dashboard）",
	Intent:      "当你要在 Base 内新建文件夹来归类数据表和仪表盘时使用；会实际创建文件夹，可指定父文件夹和插入位置。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "文件夹名称", Required: true},
		{Name: "parent-section-id", Type: shortcut.FlagString, Desc: "父文件夹 ID，空表示根目录（可选）"},
		{Name: "index", Type: shortcut.FlagInt, Default: "-1", Desc: "0-based 位置，不传追加到末尾（可选）"},
	},
	Tips: []string{`dws aitable +section-create --base-id B --name 我的文件夹`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId": rt.Str("base-id"),
			"name":   rt.Str("name"),
		}
		if rt.Changed("parent-section-id") {
			params["parentSectionId"] = rt.Str("parent-section-id")
		}
		if idx := rt.Int("index"); idx >= 0 {
			params["index"] = idx
		}
		return rt.CallMCP("create_section", params)
	},
}

// SectionRename 重命名文件夹（rename_section）。
var SectionRename = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+section-rename",
	Product:     serverHelper,
	Description: "重命名指定文件夹",
	Intent:      "当你要给 Base 内某个文件夹改名时使用；会实际重命名该 section。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "section-id", Type: shortcut.FlagString, Desc: "Section ID", Required: true},
		{Name: "new-name", Type: shortcut.FlagString, Desc: "新名称", Required: true},
	},
	Tips: []string{`dws aitable +section-rename --base-id B --section-id S --new-name 新名称`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("rename_section", map[string]any{
			"baseId":    rt.Str("base-id"),
			"sectionId": rt.Str("section-id"),
			"newName":   rt.Str("new-name"),
		})
	},
}

// SectionDelete 删除文件夹（delete_section）。
var SectionDelete = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+section-delete",
	Product:     serverHelper,
	Description: "删除指定文件夹（不可逆）",
	Intent:      "当你确认要删除 Base 内某个文件夹时使用；不可逆，操作前请确认其内节点的处置。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "section-id", Type: shortcut.FlagString, Desc: "Section ID", Required: true},
	},
	Tips: []string{`dws aitable +section-delete --base-id B --section-id S --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("delete_section", map[string]any{
			"baseId":    rt.Str("base-id"),
			"sectionId": rt.Str("section-id"),
		})
	},
}

// SectionReorder 调整文件夹顺序（reorder_section）。
var SectionReorder = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+section-reorder",
	Product:     serverHelper,
	Description: "在当前父文件夹下调整文件夹的展示顺序",
	Intent:      "当你要在同一父文件夹下调整某文件夹的排列先后顺序时使用；会实际把它移动到指定的 0-based 位置。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "section-id", Type: shortcut.FlagString, Desc: "Section ID", Required: true},
		{Name: "target-index", Type: shortcut.FlagInt, Desc: "0-based 目标位置", Required: true},
	},
	Tips: []string{`dws aitable +section-reorder --base-id B --section-id S --target-index 0`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("reorder_section", map[string]any{
			"baseId":      rt.Str("base-id"),
			"sectionId":   rt.Str("section-id"),
			"targetIndex": rt.Int("target-index"),
		})
	},
}

// SectionListEmpty 列出空文件夹（list_empty_sections）。
var SectionListEmpty = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+section-list-empty",
	Product:     serverHelper,
	Description: "列出指定 Base 下所有没有子节点的空文件夹",
	Intent:      "当你想清理 Base、需要先找出所有没有任何子节点的空文件夹时使用；返回空文件夹列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
	},
	Tips: []string{`dws aitable +section-list-empty --base-id B`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("list_empty_sections", map[string]any{"baseId": rt.Str("base-id")})
	},
}

// SectionListNodes 列出全部节点（list_nsheet_nodes）。
var SectionListNodes = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+section-list-nodes",
	Product:     serverHelper,
	Description: "列出指定 Base 当前版本下的全部 nsheet 节点",
	Intent:      "当你要总览某 Base 当前版本下的全部节点（表、仪表盘、文件夹等）目录结构时使用；返回所有 nsheet 节点。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
	},
	Tips: []string{`dws aitable +section-list-nodes --base-id B`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("list_nsheet_nodes", map[string]any{"baseId": rt.Str("base-id")})
	},
}

// SectionMoveNode 移动节点（move_nsheet_node）。
var SectionMoveNode = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+section-move-node",
	Product:     serverHelper,
	Description: "把任意 nsheet 节点移动到目标文件夹下（可选调整位置）",
	Intent:      "当你要把某张表/仪表盘/文件夹移动到另一个文件夹下、或移到 Base 根目录（可选调整位置）时使用；会实际改变节点在目录树中的位置。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "node-id", Type: shortcut.FlagString, Desc: "待移动节点 ID", Required: true},
		{Name: "new-parent-section-id", Type: shortcut.FlagString, Desc: "目标父文件夹 ID，空字符串表示 Base 根目录"},
		{Name: "target-index", Type: shortcut.FlagInt, Default: "-1", Desc: "0-based 全局下标（可选）"},
	},
	Tips: []string{`dws aitable +section-move-node --base-id B --node-id N --new-parent-section-id S`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":             rt.Str("base-id"),
			"nodeId":             rt.Str("node-id"),
			"newParentSectionId": rt.Str("new-parent-section-id"),
		}
		if idx := rt.Int("target-index"); idx >= 0 {
			params["targetIndex"] = idx
		}
		return rt.CallMCP("move_nsheet_node", params)
	},
}

func init() {
	shortcut.Register(
		BaseList,
		BaseSearch,
		BaseGet,
		BaseGetPrimaryDocID,
		BaseUpdate,
		BaseDelete,
		BaseCopy,
		TableGet,
		TableUpdate,
		TableDelete,
		FieldGet,
		FieldUpdate,
		FieldDelete,
		RecordQuery,
		RecordUpdate,
		RecordDelete,
		RecordQueryEmpty,
		RecordHistoryList,
		RecordShareURL,
		RecordUpsert,
		RecordPrimaryDocGet,
		RecordPrimaryDocCreate,
		TemplateSearch,
		AttachmentUpload,
		ViewGet,
		ViewUpdate,
		ViewDuplicate,
		ViewDelete,
		ViewGetLock,
		ViewLock,
		ViewGetFrozenCols,
		ViewSetFrozenCols,
		ViewGetRowHeight,
		ViewSetRowHeight,
		ViewSetFillColorRule,
		FormList,
		FormDelete,
		FormUpdate,
		FormFieldList,
		FormFieldUpdate,
		FormFieldHide,
		FormShareGet,
		FormShareUpdate,
		WorkflowEnable,
		WorkflowDisable,
		WorkflowGet,
		WorkflowList,
		DashboardConfigExample,
		DashboardGet,
		DashboardUpdate,
		DashboardDelete,
		DashboardArrange,
		DashboardShareGet,
		DashboardShareUpdate,
		ChartWidgetsExample,
		ChartGet,
		ChartUpdate,
		ChartDelete,
		ChartShareGet,
		ChartShareUpdate,
		ExportData,
		ImportUpload,
		ImportData,
		AdvpermEnable,
		AdvpermDisable,
		RoleList,
		RoleGet,
		RoleCreate,
		RoleUpdate,
		RoleDelete,
		SectionCreate,
		SectionRename,
		SectionDelete,
		SectionReorder,
		SectionListEmpty,
		SectionListNodes,
		SectionMoveNode,
	)
}
