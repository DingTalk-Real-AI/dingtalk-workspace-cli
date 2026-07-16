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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ListTables: list every data table (数据表) inside one multi-dimensional
// table (base) in a single step.
//
// This is a read-only convenience wrapper over get_tables. It mirrors helpers
// tableGetCmd exactly: it calls the "aitable" server tool "get_tables" with a
// single argument, baseId ← --base (no tableIds → the server returns all tables
// in the base). The response's table list is then defensively projected down to
// {tableId, tableName} and printed via rt.Output so it honours --format/--jq/
// --fields. When no recognisable table list is found it falls back to printing
// the raw payload.
//
//	dws aitable +list-tables --base B
var ListTables = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+list-tables",
	Product:     "aitable",
	Description: "列出某个多维表(base)里的所有数据表（只读，投影 tableId/tableName）",
	Intent: "当你已经知道某个多维表(base)的 baseId、想一步看清这个 base 下都有哪些数据表(table)、" +
		"拿到它们的 tableId 和 tableName 以便后续查记录或改结构，却不想手动翻 base get 的完整目录时使用；" +
		"内部直接调用 get_tables，只传 baseId（不带 tableIds，因此返回该 base 下的全部数据表），" +
		"再在本地把每张表投影成 tableId、tableName 两个关键字段打印出来。" +
		"这是纯只读操作，只做列举与本地投影，不会创建、修改或删除任何表。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base", Type: shortcut.FlagString, Desc: "Base ID（要列出数据表的多维表）", Required: true},
	},
	Tips: []string{
		`dws aitable +list-tables --base B`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// baseId mirrors helpers tableGetCmd (get_tables). Omitting tableIds
		// asks the server for every table in the base.
		data, err := rt.CallMCPData("aitable", "get_tables", map[string]any{
			"baseId": rt.Str("base"),
		})
		if err != nil {
			return err
		}

		// Project each table down to {tableId, tableName}; fall back to the raw
		// payload when we cannot locate a recognisable table list.
		items := listTablesItems(data)
		if len(items) == 0 {
			return rt.Output(data)
		}
		results := make([]map[string]any, 0, len(items))
		for _, t := range items {
			results = append(results, map[string]any{
				"tableId":   listTablesID(t),
				"tableName": listTablesName(t),
			})
		}
		return rt.Output(map[string]any{"tables": results})
	},
}

// listTablesItems locates the tables list inside a get_tables response, probing
// common container keys at the top level and nested under "data"/"result".
// Returns nil when no list is found.
func listTablesItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	keys := []string{"tables", "list", "items", "records", "data", "result"}
	for _, key := range keys {
		if arr, ok := data[key].([]any); ok {
			return listTablesToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range []string{"tables", "list", "items", "records"} {
				if arr, ok := inner[k2].([]any); ok {
					return listTablesToMaps(arr)
				}
			}
		}
	}
	return nil
}

func listTablesToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// listTablesID reads a table's id, tolerating the common id key variants.
func listTablesID(t map[string]any) any {
	for _, key := range []string{"tableId", "table_id", "id"} {
		if v := listTablesString(t[key]); v != "" {
			return v
		}
	}
	return nil
}

// listTablesName reads a table's display name, tolerating the common name key
// variants.
func listTablesName(t map[string]any) any {
	for _, key := range []string{"tableName", "table_name", "name", "title"} {
		if v := listTablesString(t[key]); v != "" {
			return v
		}
	}
	return nil
}

// listTablesString coerces a scalar JSON value to a trimmed string, returning ""
// for nil / non-scalar / empty values.
func listTablesString(v any) string {
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
	shortcut.Register(ListTables)
}
