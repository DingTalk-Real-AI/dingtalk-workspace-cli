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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Org: look up the department a person belongs to, by NAME, in one command.
//
// Steps: resolve the person's name → userId → fetch their org detail
// (get_user_info_by_user_ids) → parse the primary deptId out of
// orgEmployeeModel.depts → print that department's detail
// (get_dept_info_by_dept_id). Replaces the manual dance of
// `contact user search` → copy userId → `contact user get --ids <id>` →
// copy deptId → `contact dept get-info --dept <deptId>`.
//
//	dws contact +org --name 张三
var Org = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+org",
	Product:     "contact",
	Description: "按姓名查某人所在部门的详情（自动解析 userId 与 deptId）",
	Intent: "当你只知道某位同事的姓名、想知道 TA 所在部门（部门ID、名称、人数）时使用；" +
		"内部先按姓名解析出唯一 userId，再取 TA 的组织信息拿到主部门 deptId，" +
		"最后打印该部门的详情。只读，不做任何修改。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "contact",
			Name:           "shortcut_org",
			CanonicalPath:  "contact.shortcut_org",
			CLIPath:        "contact +org",
			PrimaryCLIPath: "contact +org",
		},
		Description: "按姓名查某人所在部门的详情（自动解析 userId 与 deptId）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按姓名查某人所在部门的详情（自动解析 userId 与 deptId）",
			UseWhen:      []string{"当你只知道某位同事的姓名、想知道 TA 所在部门（部门ID、名称、人数）时使用；内部先按姓名解析出唯一 userId，再取 TA 的组织信息拿到主部门 deptId，最后打印该部门的详情。只读，不做任何修改。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws contact +org --name 张三"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "同事姓名/花名", Required: true},
	},
	Tips: []string{`dws contact +org --name 张三`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — resolve the person's name to a unique userId.
		user, err := strictResolveContactUser(rt, rt.Str("name"))
		if err != nil {
			return err
		}

		// Step 2 — fetch that user's org-management detail; it carries the
		// department list under orgEmployeeModel.depts.
		data, err := rt.CallMCPData("contact", "get_user_info_by_user_ids", map[string]any{
			"user_id_list": []string{user.userID},
		})
		if err != nil {
			return err
		}

		// Step 3 — defensively pull the primary deptId out of the response,
		// then print that department's detail.
		profile, err := strictUserDetail(data, user.userID, "contact/get_user_info_by_user_ids")
		if err != nil {
			return err
		}
		deptID, err := strictPrimaryDeptID(profile, "contact/get_user_info_by_user_ids")
		if err != nil {
			return err
		}
		data, err = rt.CallMCPData("contact", "get_dept_info_by_dept_id", map[string]any{
			"deptId": deptID,
		})
		if err != nil {
			return err
		}
		dept, err := strictDeptDetail(data, deptID, "contact/get_dept_info_by_dept_id")
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"department": dept})
	},
}

// shortcutOrgExtractDeptID walks the get_user_info_by_user_ids response and
// returns the first department id it can find, converted to int64 (the type
// get_dept_info_by_dept_id expects). The real-machine payload nests the user
// object differently depending on the transport wrapper, so try several
// candidate shapes before giving up.
func shortcutOrgExtractDeptID(data map[string]any) (int64, bool) {
	for _, user := range shortcutOrgCandidateUsers(data) {
		model, ok := user["orgEmployeeModel"].(map[string]any)
		if !ok {
			continue
		}
		depts, ok := model["depts"].([]any)
		if !ok {
			continue
		}
		for _, d := range depts {
			dm, ok := d.(map[string]any)
			if !ok {
				continue
			}
			if id, ok := shortcutOrgToInt64(dm["deptId"]); ok {
				return id, true
			}
		}
	}
	return 0, false
}

// shortcutOrgCandidateUsers flattens the several shapes the user-detail payload
// may take (top-level object, {"result": [...]}, {"data": ...}) into a list of
// user-object maps to probe.
func shortcutOrgCandidateUsers(data map[string]any) []map[string]any {
	var out []map[string]any
	if data == nil {
		return out
	}
	// The object itself may already be the user record.
	out = append(out, data)
	for _, key := range []string{"result", "data", "userList", "users"} {
		switch v := data[key].(type) {
		case map[string]any:
			out = append(out, v)
		case []any:
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					out = append(out, m)
				}
			}
		}
	}
	return out
}

// shortcutOrgToInt64 coerces a JSON-decoded numeric/string deptId to int64.
func shortcutOrgToInt64(v any) (int64, bool) {
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
		if i, err := strconv.ParseInt(n, 10, 64); err == nil {
			return i, true
		}
	}
	return 0, false
}

func init() {
	finalizeContactSmart(&Org)
	shortcut.Register(Org)
}
