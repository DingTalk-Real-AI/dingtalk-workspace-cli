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

// FindFile: search 钉盘 files by name keyword and project the essentials.
//
// This is a one-step convenience wrapper over the drive MCP tool search_files.
// It mirrors helpers driveSearchCmd exactly for the "仅搜钉盘文件" path:
//   - keyword      ← --query (the free-text file-name search term; MCP arg "keyword")
//   - searchTarget ← "file"  (restrict to 钉盘 files/folders, no doc-space aggregation)
//
// The raw search response is then reduced locally to a compact list of
// {name, type, dentryId, fileSize} per hit and emitted via rt.Output, so it
// honours the root --format/--jq/--fields projection flags. Field parsing is
// defensive: both the container (result/data/items/files/nodes/list) and each
// item's fields (multiple candidate keys) are probed leniently.
//
// Read-only: it never mutates anything, it only searches and projects locally.
//
//	dws drive +find-file --query 季度汇报
var FindFile = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+find-file",
	Product:     "drive",
	Description: "按名称关键词搜索钉盘文件并投影关键字段（只读）",
	Intent: "当你只记得钉盘文件的名字（或其中一部分），想快速按文件名关键词找到它、拿到它的 dentryId 以便后续下载/查看，" +
		"却不想手动翻目录或写复杂过滤条件时使用；内部调用钉盘的 search_files 工具，把 --query 作为文件名关键词(keyword) " +
		"并限定搜索范围为钉盘文件(searchTarget=file)，再在本地把每条命中结果精简为「文件名、类型、dentryId、大小」四个字段后打印。" +
		"这是纯只读操作，只做搜索与本地投影，不会创建、移动或删除任何文件；未命中时返回空列表。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "文件名关键词（必填）", Required: true},
	},
	Tips: []string{
		`dws drive +find-file --query 季度汇报`,
		`dws drive +find-file --query 合同`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		keyword := strings.TrimSpace(rt.Str("query"))

		// keyword / searchTarget mirror helpers driveSearchCmd (search_files),
		// restricted to 钉盘 files only.
		data, err := rt.CallMCPData("drive", "search_files", map[string]any{
			"keyword":      keyword,
			"searchTarget": "file",
		})
		if err != nil {
			return err
		}

		items := shortcutFindFileItems(data)
		files := make([]map[string]any, 0, len(items))
		for _, m := range items {
			files = append(files, map[string]any{
				"name":     shortcutFindFileStr(m, "name", "fileName", "title", "dentryName"),
				"type":     shortcutFindFileStr(m, "type", "dentryType", "extension", "fileType"),
				"dentryId": shortcutFindFileStr(m, "dentryId", "dentryUuid", "fileId", "nodeId", "id"),
				"fileSize": shortcutFindFileSize(m),
			})
		}

		return rt.Output(map[string]any{"files": files})
	},
}

// shortcutFindFileItems locates the list of hit records inside the search_files
// response, tolerating an optional result/data wrapper and several common list
// key names. Returns a slice of map records (non-map entries are skipped).
func shortcutFindFileItems(data map[string]any) []map[string]any {
	container := data
	for _, wrap := range []string{"result", "data"} {
		if inner, ok := container[wrap].(map[string]any); ok {
			container = inner
		}
	}
	for _, key := range []string{"items", "files", "nodes", "list", "dentries", "results", "records"} {
		if arr, ok := container[key].([]any); ok {
			return shortcutFindFileToMaps(arr)
		}
	}
	// Fallback: the container itself might be the array under a differently
	// named key — scan for the first []any value.
	for _, v := range container {
		if arr, ok := v.([]any); ok {
			return shortcutFindFileToMaps(arr)
		}
	}
	return nil
}

func shortcutFindFileToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// shortcutFindFileStr returns the first non-empty string value among the given
// candidate keys.
func shortcutFindFileStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				return s
			}
		}
	}
	return ""
}

// shortcutFindFileSize reads a hit's file size, tolerating numeric (float64) and
// string JSON encodings across the common size key names. Returns nil when
// absent (e.g. folders), so the field is simply omitted-as-null in output.
func shortcutFindFileSize(m map[string]any) any {
	for _, k := range []string{"fileSize", "size"} {
		switch v := m[k].(type) {
		case float64:
			return int64(v)
		case int64:
			return v
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		}
	}
	return nil
}

func init() {
	shortcut.Register(FindFile)
}
