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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// FindRecord: search records in a given aitable (多维表) by a full-text keyword.
//
// This is a one-step convenience wrapper over query_records. It mirrors
// helpers RecordQuery exactly:
//   - baseId  ← --base
//   - tableId ← --table
//   - keyword ← --query (the free-text search term; MCP arg is "keyword")
//
// When --query is omitted it simply returns the first page of records for the
// table (no filter). It is a read-only operation and never mutates data.
//
//	dws aitable +find-record --base B --table T
//	dws aitable +find-record --base B --table T --query 张三
var FindRecord = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+find-record",
	Product:     "aitable",
	Description: "在指定多维表里按关键词查记录（只读）",
	Intent: "当你已经知道某个多维表的 baseId 和 tableId、想按一个关键词快速找出匹配的行记录，" +
		"却不想手写结构化过滤条件时使用；内部直接调用 query_records，把 --query 作为全文关键词(keyword)在该表里检索并打印匹配记录。" +
		"不传 --query 时则返回该表的前若干条记录。这是只读操作，不会修改任何数据。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base", Type: shortcut.FlagString, Desc: "Base ID（多维表所属 base）", Required: true},
		{Name: "table", Type: shortcut.FlagString, Desc: "Table ID（要检索的数据表）", Required: true},
		{Name: "query", Type: shortcut.FlagString, Desc: "全文关键词（可选，不填则取前若干条）", Required: false},
	},
	Tips: []string{
		`dws aitable +find-record --base B --table T`,
		`dws aitable +find-record --base B --table T --query 张三`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// baseId / tableId / keyword mirror helpers RecordQuery (query_records).
		params := map[string]any{
			"baseId":  rt.Str("base"),
			"tableId": rt.Str("table"),
		}
		if kw := strings.TrimSpace(rt.Str("query")); kw != "" {
			params["keyword"] = kw
		}
		return rt.CallMCP("query_records", params)
	},
}

func init() {
	shortcut.Register(FindRecord)
}
