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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
)

// ResolveBase: resolve a 多维表 Base by name keyword into a single baseId.
//
// This is the Base-level analogue of "resolve a user by name". It searches
// Bases by name and disambiguates:
//   - search Bases via search_bases (mirrors helpers base search, MCP arg
//     "query" ← --name);
//   - project each candidate to {baseId, name} — field parsing is defensive
//     (multiple candidate keys);
//   - exactly one match → return {resolved:true, baseId, name};
//     multiple matches → return {resolved:false, count, candidates} and let
//     the caller pick (never guesses);
//     zero matches → report a validation error instead of an empty raw dump.
//
// Read-only: it only searches and reshapes, never mutates any Base.
//
//	dws aitable +resolve-base --name 项目管理
var ResolveBase = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+resolve-base",
	Product:     "aitable",
	Description: "按名称搜索多维表 Base 并解析出唯一 baseId（只读）",
	Intent: "当你只知道某个多维表 Base 的名称、想把它解析成可直接用于后续工具的 baseId 时使用；" +
		"内部完整分页搜索并优先做大小写不敏感的精确名称匹配，只有显式 --fuzzy 才允许关键词包含匹配。" +
		"0 个或多个候选都会以结构化错误失败并返回候选，绝不替你猜选。" +
		"这是纯只读操作，只做搜索与本地投影，不会修改任何 Base。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_resolve_base",
			CanonicalPath:  "aitable.shortcut_resolve_base",
			CLIPath:        "aitable +resolve-base",
			PrimaryCLIPath: "aitable +resolve-base",
		},
		Description: "按名称搜索多维表 Base 并解析出唯一 baseId（只读）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按名称搜索多维表 Base 并解析出唯一 baseId（只读）",
			UseWhen:      []string{"当你只知道某个多维表 Base 的名称、想把它解析成可直接用于后续工具的 baseId 时使用；内部完整分页搜索并优先做大小写不敏感的精确名称匹配，只有显式 --fuzzy 才允许关键词包含匹配。0 个或多个候选都会以结构化错误失败并返回候选，绝不替你猜选。这是纯只读操作，只做搜索与本地投影，不会修改任何 Base。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws aitable +resolve-base --name 项目管理"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "要解析的 Base 名称", Required: true},
		{Name: "fuzzy", Type: shortcut.FlagBool, Default: "false", Desc: "精确名称无结果时允许包含匹配"},
	},
	Tips: []string{
		`dws aitable +resolve-base --name 项目管理`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		resolution, err := aitabletarget.ResolveBaseName(rt, rt.Str("name"), rt.Bool("fuzzy"))
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{
			"resolved":  true,
			"status":    resolution.Status,
			"matchType": resolution.MatchType,
			"baseId":    resolution.Selected.ID,
			"name":      resolution.Selected.Name,
		})
	},
}

// resolveBaseItems defensively unwraps the list of Base candidates from the
// search_bases response, tolerating the common container keys the gateway may
// use.
func resolveBaseItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	for _, key := range []string{"result", "data", "list", "items", "bases", "records"} {
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
			if inner := resolveBaseItems(nested); len(inner) > 0 {
				return inner
			}
		}
	}
	return nil
}

// resolveBaseID reads a Base's identifier, tolerating the common id keys.
func resolveBaseID(b map[string]any) string {
	for _, key := range []string{"baseId", "base_id", "id"} {
		if s, ok := b[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// resolveBaseName reads a Base's display name, tolerating the common name keys.
func resolveBaseName(b map[string]any) string {
	for _, key := range []string{"name", "baseName", "title"} {
		if s, ok := b[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func init() {
	shortcut.Register(ResolveBase)
}
