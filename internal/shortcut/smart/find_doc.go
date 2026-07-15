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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// FindDoc: search cloud documents by keyword and project the essentials.
//
// This is a one-step convenience wrapper over the doc MCP tool search_documents.
// It mirrors the helpers doc search path exactly:
//   - keyword  ← --query (the free-text search term; MCP arg "keyword", verbatim)
//   - pageSize ← --limit (optional; MCP arg "pageSize", verbatim)
//
// The raw search response is then reduced locally to a compact list of
// {title, url, type, token} per hit and emitted via rt.Output, so it honours
// the root --format/--jq/--fields projection flags. Field parsing is
// defensive: both the container (result/data/list/items/documents/docs) and
// each item's fields (multiple candidate keys) are probed leniently.
//
// Read-only: it never mutates anything, it only searches and projects locally.
//
//	dws doc +find-doc --query 季度汇报
var FindDoc = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+find-doc",
	Product:     "doc",
	Description: "按关键词搜索云文档并投影关键字段（只读）",
	Intent: "当你只记得云文档标题或内容里的某个关键词，想快速按关键词找到匹配的文档、拿到它的标题、URL、类型和 token 以便后续查看或编辑，" +
		"却不想拿到一大坨原始字段时使用；内部调用云文档的 search_documents 工具，把 --query 作为搜索关键词(keyword)，" +
		"可选地用 --limit 限制返回条数(pageSize)，再在本地把每条命中结果精简为「标题、URL、类型、token」四个字段后打印。" +
		"这是纯只读操作，只做搜索与本地投影，不会创建、修改或删除任何文档；未命中时提示「没搜到文档」。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "按关键词搜索云文档（必填）", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "限制返回的文档条数（可选）"},
	},
	Tips: []string{
		`dws doc +find-doc --query 季度汇报`,
		`dws doc +find-doc --query 合同 --limit 10`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// keyword / pageSize mirror the helpers doc search_documents path verbatim.
		params := map[string]any{
			"keyword": rt.Str("query"),
		}
		if rt.Changed("limit") {
			if n := rt.Int("limit"); n > 0 {
				params["pageSize"] = n
			}
		}

		data, err := rt.CallMCPData("doc", "search_documents", params)
		if err != nil {
			return err
		}

		items := shortcutFindDocItems(data)
		results := make([]map[string]any, 0, len(items))
		for _, m := range items {
			results = append(results, map[string]any{
				"title": shortcutFindDocStr(m, "title", "name", "docName", "subject"),
				"url":   shortcutFindDocStr(m, "url", "docUrl", "link", "webUrl"),
				"type":  shortcutFindDocStr(m, "docType", "type", "dentryType", "fileType"),
				"token": shortcutFindDocStr(m, "docId", "token", "nodeId", "dentryId", "id"),
			})
		}

		if len(results) == 0 {
			return apperrors.NewValidation("没搜到文档")
		}

		return rt.Output(map[string]any{"documents": results, "count": len(results)})
	},
}

// shortcutFindDocItems locates the list of hit records inside the
// search_documents response, tolerating an optional result/data wrapper and
// several common list key names. Returns a slice of map records (non-map
// entries are skipped).
func shortcutFindDocItems(data map[string]any) []map[string]any {
	container := data
	for _, wrap := range []string{"result", "data"} {
		if inner, ok := container[wrap].(map[string]any); ok {
			container = inner
		}
	}
	for _, key := range []string{"documents", "docs", "items", "list", "results", "records", "nodes"} {
		if arr, ok := container[key].([]any); ok {
			return shortcutFindDocToMaps(arr)
		}
	}
	// Fallback: the container itself might hold the array under a differently
	// named key — scan for the first []any value.
	for _, v := range container {
		if arr, ok := v.([]any); ok {
			return shortcutFindDocToMaps(arr)
		}
	}
	return nil
}

func shortcutFindDocToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// shortcutFindDocStr returns the first non-empty string value among the given
// candidate keys.
func shortcutFindDocStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				return s
			}
		}
	}
	return ""
}

func init() {
	shortcut.Register(FindDoc)
}
