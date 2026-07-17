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
	"encoding/json"
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ResolveDept: resolve a department by name keyword into a single deptId.
//
// This is the department-level analogue of "resolve a user by name". It searches
// departments by name and disambiguates:
//   - search departments via search_dept_by_keyword (mirrors helpers contact
//     dept search, MCP arg "query" ← --name);
//   - project each candidate to {deptId, name} — field parsing is defensive
//     (multiple candidate keys);
//   - exactly one match → return {resolved:true, deptId, name};
//     multiple matches → return {resolved:false, count, candidates} and let
//     the caller pick (never guesses);
//     zero matches → report a validation error instead of an empty raw dump.
//
// Read-only: it only searches and reshapes, never mutates anything. Unlike
// +dept-members it stops at the deptId and does NOT list members.
//
//	dws contact +resolve-dept --name 技术部
var ResolveDept = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+resolve-dept",
	Product:     "contact",
	Description: "按名称搜索部门并解析出唯一 deptId（只读）",
	Intent: "当你只知道某个部门的名称（或名称里的关键词）、想把它解析成可直接用于后续工具的 deptId 时使用；" +
		"内部按 --name 关键词调用 search_dept_by_keyword 搜索部门，再在本地投影出每个候选的 deptId 和 name。" +
		"如果只命中一个部门就直接返回它的 deptId；如果命中多个则列出全部候选让你消歧，绝不替你瞎猜；如果一个都没命中则提示未找到。" +
		"这是纯只读操作，只做搜索与本地投影，不会修改任何部门，也不会列出部门成员。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "要搜索的部门名称关键词（必填）", Required: true},
	},
	Tips: []string{
		`dws contact +resolve-dept --name 技术部`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Search departments by name. tool "search_dept_by_keyword" + arg
		// "query" are taken verbatim from helpers contact dept search
		// (callMCPTool → server contact).
		data, err := rt.CallMCPData("contact", "search_dept_by_keyword", map[string]any{
			"query": rt.Str("name"),
		})
		if err != nil {
			return err
		}

		// Project candidates to {deptId, name}, defensively unwrapping the list.
		items := resolveDeptItems(data)
		candidates := make([]map[string]any, 0, len(items))
		for _, d := range items {
			candidates = append(candidates, map[string]any{
				"deptId": resolveDeptID(d),
				"name":   resolveDeptName(d),
			})
		}

		switch len(candidates) {
		case 0:
			return apperrors.NewValidation("没有找到名称包含 " + rt.Str("name") + " 的部门")
		case 1:
			return rt.Output(map[string]any{
				"resolved": true,
				"deptId":   candidates[0]["deptId"],
				"name":     candidates[0]["name"],
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

// resolveDeptItems defensively unwraps the list of department candidates from
// the search_dept_by_keyword response, tolerating the common container keys the
// gateway may use.
func resolveDeptItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	// NB: the real search_dept_by_keyword response wraps candidates in "deptList"
	// (verified against the live gateway); keep it first among the probes.
	for _, key := range []string{"deptList", "result", "data", "list", "items", "departments", "depts", "records"} {
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
			if inner := resolveDeptItems(nested); len(inner) > 0 {
				return inner
			}
		}
	}
	return nil
}

// resolveDeptID reads a department's identifier, tolerating the common id keys.
// DingTalk deptIds are numeric, so the value arrives as a JSON number (float64)
// rather than a string — coerce it instead of dropping it.
func resolveDeptID(d map[string]any) string {
	for _, key := range []string{"deptId", "dept_id", "departmentId", "id"} {
		switch v := d[key].(type) {
		case string:
			if v != "" {
				return v
			}
		case float64:
			return strconv.FormatInt(int64(v), 10)
		case json.Number:
			return v.String()
		}
	}
	return ""
}

// resolveDeptName reads a department's display name, tolerating the common name
// keys. search_dept_by_keyword wraps the matched substring in <red>…</red>
// highlight markup — strip it so the projected name is clean.
func resolveDeptName(d map[string]any) string {
	for _, key := range []string{"name", "deptName", "departmentName", "title"} {
		if s, ok := d[key].(string); ok && s != "" {
			return stripHighlightTags(s)
		}
	}
	return ""
}

// stripHighlightTags removes the <red>/</red> (and similar simple) highlight
// tags the DingTalk search gateway injects around matched keywords.
func stripHighlightTags(s string) string {
	for _, tag := range []string{"<red>", "</red>", "<b>", "</b>", "<em>", "</em>"} {
		s = strings.ReplaceAll(s, tag, "")
	}
	return s
}

func init() {
	shortcut.Register(ResolveDept)
}
