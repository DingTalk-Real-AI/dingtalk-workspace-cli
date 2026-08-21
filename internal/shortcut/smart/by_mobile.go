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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ByMobile: find a person by phone number and return their full profile in one
// step.
//
// Steps: find one candidate through keyword search → fetch the same userId's
// full detail → require the returned orgUserMobile to exactly match the
// normalized request. A unique keyword candidate alone is never mobile proof.
//
//	dws contact +by-mobile --mobile 13800138000
var ByMobile = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+by-mobile",
	Product:     "contact",
	Description: "按手机号查询某人的完整资料（自动解析 userId 后取详情）",
	Intent: "当你只知道对方手机号、想一步拿到其完整资料（部门、职位、联系方式、是否管理员等）而不想先按手机号搜出 userId 再单独查详情时使用；" +
		"内部先用手机号在通讯录里查出对应的 userId，若没有人绑定该手机号会明确报错，再用该 userId 取完整详情。这是纯只读操作，不会修改任何数据。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "contact",
			Name:           "shortcut_by_mobile",
			CanonicalPath:  "contact.shortcut_by_mobile",
			CLIPath:        "contact +by-mobile",
			PrimaryCLIPath: "contact +by-mobile",
		},
		Description: "按手机号查询某人的完整资料（自动解析 userId 后取详情）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按手机号查询某人的完整资料（自动解析 userId 后取详情）",
			UseWhen:      []string{"当你只知道对方手机号、想一步拿到其完整资料（部门、职位、联系方式、是否管理员等）而不想先按手机号搜出 userId 再单独查详情时使用；内部先用手机号在通讯录里查出对应的 userId，若没有人绑定该手机号会明确报错，再用该 userId 取完整详情。这是纯只读操作，不会修改任何数据。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws contact +by-mobile --mobile 13800138000"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "mobile", Type: shortcut.FlagString, Desc: "手机号；--mobile 必须是至少 6 位数字的手机号，可包含国家码、空格、连字符或括号", Required: true},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"mobile"}, Description: "--mobile 必须是至少 6 位数字的手机号，可包含国家码、空格、连字符或括号"}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return validateContactSmartMobile(rt, "contact/by-mobile", "mobile")
	},
	Tips: []string{`dws contact +by-mobile --mobile 13800138000`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if err := rt.RequireAll("mobile"); err != nil {
			return err
		}
		mobile := rt.Str("mobile")

		// Step 1 — resolve the mobile through the keyword endpoint. Unlike
		// search_user_by_mobile, this endpoint publishes an explicit result
		// array for both the known match and the guaranteed zero-match case.
		_, profile, err := strictResolveContactUserByMobile(rt, mobile)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"profile": profile})
	},
}

func init() {
	finalizeContactSmart(&ByMobile)
	shortcut.Register(ByMobile)
}
