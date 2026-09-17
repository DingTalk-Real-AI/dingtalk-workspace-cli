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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/minutesdata"
)

// MinutesSearch: search MY minutes (听记) by keyword and print a projected list.
//
// Steps:
//  1. list my minutes via list_by_keyword_and_time_range with
//     belongingConditionId="created", maxResults=20 and keyword=--query,
//     mirroring helpers.callListByKeywordRange;
//  2. project each entry to {title, createTime, taskUuid} — field parsing is
//     defensive (multiple candidate keys) — and print via rt.Output so it
//     honours --format/--jq/--fields;
//  3. if nothing matched, report "没搜到妙记" instead of an empty raw dump.
//
// Read-only: it only lists and reshapes, never mutates any minute.
//
//	dws minutes +minutes-search --query 周会
var MinutesSearch = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+minutes-search",
	Product:     "minutes",
	Description: "按关键词搜索我的妙记并投影列表",
	Intent: "当你想按关键词快速找回自己创建的会议听记（妙记），只需要看到匹配到的标题、创建时间和 taskUuid 列表、而不想拿到一大坨原始字段时使用；" +
		"内部按 --query 关键词列出你创建的听记（最多 20 条），再在本地投影出每条的标题、创建时间和 taskUuid。" +
		"这是纯只读操作，只做搜索与本地投影，不会修改任何听记；若没有匹配的听记则提示「没搜到妙记」。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "minutes",
			Name:           "shortcut_minutes_search",
			CanonicalPath:  "minutes.shortcut_minutes_search",
			CLIPath:        "minutes +minutes-search",
			PrimaryCLIPath: "minutes +minutes-search",
		},
		Description: "按关键词搜索我的妙记并投影列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按关键词搜索我的妙记并投影列表",
			UseWhen:      []string{"当你想按关键词快速找回自己创建的会议听记（妙记），只需要看到匹配到的标题、创建时间和 taskUuid 列表、而不想拿到一大坨原始字段时使用；内部按 --query 关键词列出你创建的听记（最多 20 条），再在本地投影出每条的标题、创建时间和 taskUuid。这是纯只读操作，只做搜索与本地投影，不会修改任何听记；若没有匹配的听记则提示「没搜到妙记」。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws minutes +minutes-search --query 周会"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "按关键词搜索听记（必填）", Required: true},
	},
	Tips: []string{
		`dws minutes +minutes-search --query 周会`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — search my minutes. belongingConditionId / maxResults / keyword
		// mirror helpers.callListByKeywordRange.
		data, err := rt.CallMCPData("minutes", "list_by_keyword_and_time_range", map[string]any{
			"belongingConditionId": "created",
			"maxResults":           float64(20),
			"keyword":              rt.Str("query"),
		})
		if err != nil {
			return err
		}

		// Step 2 — validate and project matched entries. An explicit [] is a
		// successful empty search; an unknown/malformed shape is a contract error.
		page, err := minutesdata.ParseListPage(data)
		if err != nil {
			return err
		}
		results, err := minutesdata.ProjectList(page)
		if err != nil {
			return err
		}

		// Step 3 — empty result guard.
		return rt.Output(map[string]any{"count": len(results), "minutes": results})
	},
}

func init() {
	// Keep the historical Schema identity as a hidden compatibility
	// leaf. +search is the only public discovery route, but removing this
	// independently shipped ToolSpec would break existing Schema consumers.
	shortcut.Register(finalizeMinutesSmartShortcut(MinutesSearch))
}
