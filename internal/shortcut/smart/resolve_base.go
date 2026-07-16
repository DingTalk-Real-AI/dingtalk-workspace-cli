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
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
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
	Intent: "当你只知道某个多维表 Base 的名称（或名称里的关键词）、想把它解析成可直接用于后续工具的 baseId 时使用；" +
		"内部按 --name 关键词调用 search_bases 搜索 Base，再在本地投影出每个候选的 baseId 和 name。" +
		"如果只命中一个 Base 就直接返回它的 baseId；如果命中多个则列出全部候选让你消歧，绝不替你瞎猜；如果一个都没命中则提示未找到。" +
		"这是纯只读操作，只做搜索与本地投影，不会修改任何 Base。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "要搜索的 Base 名称关键词（必填）", Required: true},
	},
	Tips: []string{
		`dws aitable +resolve-base --name 项目管理`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Search Bases by name. tool "search_bases" + arg "query" are taken
		// verbatim from helpers base search (callAitableTool → server aitable).
		data, err := rt.CallMCPData("aitable", "search_bases", map[string]any{
			"query": rt.Str("name"),
		})
		if err != nil {
			return err
		}

		// Project candidates to {baseId, name}, defensively unwrapping the list.
		items := resolveBaseItems(data)
		candidates := make([]map[string]any, 0, len(items))
		for _, b := range items {
			candidates = append(candidates, map[string]any{
				"baseId": resolveBaseID(b),
				"name":   resolveBaseName(b),
			})
		}

		switch len(candidates) {
		case 0:
			return apperrors.NewValidation("没有找到名称包含 " + rt.Str("name") + " 的 Base")
		case 1:
			return rt.Output(map[string]any{
				"resolved": true,
				"baseId":   candidates[0]["baseId"],
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
