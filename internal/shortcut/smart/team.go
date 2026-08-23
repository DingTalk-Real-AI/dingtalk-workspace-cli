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
	"fmt"
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Team: list the members of the department a person belongs to, by NAME.
//
// Steps: resolve the person's name → userId → fetch their org detail
// (get_user_info_by_user_ids) → defensively parse the primary deptId out of
// orgEmployeeModel.depts → print that department's direct members
// (get_dept_members_by_deptId). Replaces the manual dance of
// `contact user search` → copy userId → `contact user get --ids <id>` →
// copy deptId → `contact dept list-members --depts <deptId>`.
//
// Scope note: get_dept_members_by_deptId returns only the direct members of
// the resolved department, it does NOT recurse into sub-departments.
//
//	dws contact +team --name 张三
var Team = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+team",
	Product:     "contact",
	Description: "按姓名列出某人所在部门的成员（自动解析 userId 与 deptId）",
	Intent: "当你只知道某位同事的姓名、想知道 TA 所在部门里都有哪些成员时使用；" +
		"内部先按姓名解析出唯一 userId，再取 TA 的组织信息拿到主部门 deptId，" +
		"最后打印该部门的直接成员列表（仅本部门，不递归下级部门）。只读，不做任何修改。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "contact",
			Name:           "shortcut_team",
			CanonicalPath:  "contact.shortcut_team",
			CLIPath:        "contact +team",
			PrimaryCLIPath: "contact +team",
		},
		Description: "按姓名列出某人所在部门的成员（自动解析 userId 与 deptId）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按姓名列出某人所在部门的成员（自动解析 userId 与 deptId）",
			UseWhen:      []string{"当你只知道某位同事的姓名、想知道 TA 所在部门里都有哪些成员时使用；内部先按姓名解析出唯一 userId，再取 TA 的组织信息拿到主部门 deptId，最后打印该部门的直接成员列表（仅本部门，不递归下级部门）。只读，不做任何修改。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws contact +team --name 张三"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "同事姓名/花名", Required: true},
	},
	Tips: []string{`dws contact +team --name 张三`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — resolve the person's name to a unique userId.
		user, err := resolveUser(rt, rt.Str("name"))
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

		// Step 3 — defensively pull the primary deptId out of the response
		// (reuses the same extractor as +org), then list that department's
		// direct members.
		deptID, ok := shortcutOrgExtractDeptID(data)
		if !ok {
			return apperrors.NewValidation(fmt.Sprintf(
				"没能从 %s(%s) 的组织信息里解析出所在部门 deptId；"+
					"TA 可能没有归属部门，或返回结构与预期不符。",
				user.name, user.userID))
		}

		// get_dept_members_by_deptId expects deptIds as a string list.
		return rt.CallMCP("get_dept_members_by_deptId", map[string]any{
			"deptIds": []string{strconv.FormatInt(deptID, 10)},
		})
	},
}

func init() {
	shortcut.Register(Team)
}
