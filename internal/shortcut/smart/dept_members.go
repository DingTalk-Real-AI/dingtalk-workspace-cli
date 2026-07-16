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
	"fmt"
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// DeptMembers: list the members of a department, by NAME, in one command.
//
// Steps: search the directory for the department by keyword
// (search_dept_by_keyword) → defensively parse the unique deptId out of the
// response → print that department's direct members
// (get_dept_members_by_deptId). Replaces the manual dance of
// `contact dept search --query <名称>` → copy deptId →
// `contact dept list-members --depts <deptId>`.
//
// Disambiguation: if the name matches zero or several departments, it does NOT
// guess — it errors and lists the candidates (name + deptId) so the caller can
// re-run against a specific department.
//
// Scope note: get_dept_members_by_deptId returns only the direct members of the
// resolved department, it does NOT recurse into sub-departments.
//
//	dws contact +dept-members --dept 技术部
var DeptMembers = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+dept-members",
	Product:     "contact",
	Description: "按部门名列出部门成员（自动解析 deptId）",
	Intent: "当你只知道某个部门的名称、想知道该部门里都有哪些成员时使用；" +
		"内部先按部门名搜索出唯一的 deptId（匹配到 0 个或多个时不猜测，" +
		"而是列出候选部门名与 deptId 让你选），再打印该部门的直接成员列表" +
		"（仅本部门，不递归下级部门）。只读，不做任何修改。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "dept", Type: shortcut.FlagString, Desc: "部门名/关键词", Required: true},
	},
	Tips: []string{`dws contact +dept-members --dept 技术部`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if err := rt.RequireAll("dept"); err != nil {
			return err
		}
		keyword := strings.TrimSpace(rt.Str("dept"))

		// Step 1 — search the directory for the department by keyword.
		data, err := rt.CallMCPData("contact", "search_dept_by_keyword", map[string]any{
			"query": keyword,
		})
		if err != nil {
			return err
		}

		// Step 2 — defensively pull the matching departments out of the
		// response and require exactly one, otherwise disambiguate.
		depts := deptMembersExtractDepts(data)
		switch {
		case len(depts) == 0:
			return apperrors.NewValidation(fmt.Sprintf(
				"没找到名字里包含 %q 的部门；换个更准确的部门名再试。", keyword))
		case len(depts) > 1:
			return apperrors.NewValidation(fmt.Sprintf(
				"%q 匹配到 %d 个部门：%s。请用更精确的部门名，"+
					"或直接用 dws contact dept list-members --depts <deptId>。",
				keyword, len(depts), strings.Join(deptMembersLabels(depts), "、")))
		}

		// Step 3 — list that department's direct members.
		// get_dept_members_by_deptId expects deptIds as a string list.
		return rt.CallMCP("get_dept_members_by_deptId", map[string]any{
			"deptIds": []string{strconv.FormatInt(depts[0].id, 10)},
		})
	},
}

// deptMembersDept is the minimal identity parsed out of a department search hit.
type deptMembersDept struct {
	id   int64
	name string
}

// deptMembersExtractDepts walks the search_dept_by_keyword response and returns
// the departments it can find. The real-machine payload may wrap the list under
// a few different keys ({"result": [...]}, {"data": [...]}, or a bare list), so
// probe several candidate shapes before giving up. Each hit needs a numeric
// deptId; the deptName is best-effort for the disambiguation message.
func deptMembersExtractDepts(data map[string]any) []deptMembersDept {
	var out []deptMembersDept
	seen := map[int64]bool{}
	for _, item := range deptMembersCandidateItems(data) {
		id, ok := deptMembersToInt64(deptMembersFirst(item, "deptId", "dept_id", "id"))
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		name, _ := deptMembersFirst(item, "deptName", "dept_name", "name").(string)
		// search_dept_by_keyword wraps the matched keyword in <red>…</red>
		// highlight markup; strip it so the disambiguation message is clean.
		out = append(out, deptMembersDept{id: id, name: stripHighlightTags(name)})
	}
	return out
}

// deptMembersCandidateItems flattens the several shapes the dept-search payload
// may take into a flat list of department-object maps to probe.
func deptMembersCandidateItems(data map[string]any) []map[string]any {
	var out []map[string]any
	if data == nil {
		return out
	}
	appendList := func(v any) {
		if list, ok := v.([]any); ok {
			for _, item := range list {
				if m, ok := item.(map[string]any); ok {
					out = append(out, m)
				}
			}
		}
	}
	for _, key := range []string{"result", "data", "deptList", "departments", "list", "items"} {
		appendList(data[key])
	}
	// The object itself may already be a single department record.
	if _, ok := deptMembersToInt64(deptMembersFirst(data, "deptId", "dept_id", "id")); ok {
		out = append(out, data)
	}
	return out
}

// deptMembersFirst returns the first non-nil value among the given keys.
func deptMembersFirst(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

// deptMembersToInt64 coerces a JSON-decoded numeric/string deptId to int64.
func deptMembersToInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, true
		}
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64); err == nil {
			return i, true
		}
	}
	return 0, false
}

// deptMembersLabels renders "<deptName>(<deptId>)" candidates for the
// disambiguation message.
func deptMembersLabels(depts []deptMembersDept) []string {
	out := make([]string, 0, len(depts))
	for _, d := range depts {
		name := d.name
		if name == "" {
			name = "(未命名)"
		}
		out = append(out, fmt.Sprintf("%s(%d)", name, d.id))
	}
	return out
}

func init() {
	shortcut.Register(DeptMembers)
}
