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

// ResolveSpace: resolve a wiki 知识空间 by name keyword into a single spaceId.
//
// This is the wiki-space-level analogue of "resolve a user by name". It
// searches knowledge spaces by name and disambiguates:
//   - search spaces via search_wikiSpaces (mirrors helpers wiki space search,
//     MCP arg "keyword" ← --name);
//   - project each candidate to {spaceId, name} — field parsing is defensive
//     (multiple candidate keys);
//   - exactly one match → return {resolved:true, spaceId, name};
//     multiple matches → return {resolved:false, count, candidates} and let
//     the caller pick (never guesses);
//     zero matches → report a validation error instead of an empty raw dump.
//
// Read-only: it only searches and reshapes, never mutates any space.
//
//	dws wiki +resolve-space --name 产品文档
var ResolveSpace = shortcut.Shortcut{
	Service:     "wiki",
	Command:     "+resolve-space",
	Product:     "wiki",
	Description: "按名称搜索知识空间并解析出唯一 spaceId（只读）",
	Intent: "当你只知道某个知识空间（wiki space）的名称（或名称里的关键词）、想把它解析成可直接用于后续工具的 spaceId 时使用；" +
		"内部按 --name 关键词调用 search_wikiSpaces 搜索知识空间，再在本地投影出每个候选的 spaceId 和 name。" +
		"如果只命中一个知识空间就直接返回它的 spaceId；如果命中多个则列出全部候选让你消歧，绝不替你瞎猜；如果一个都没命中则提示未找到。" +
		"这是纯只读操作，只做搜索与本地投影，不会修改任何知识空间。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "要搜索的知识空间名称关键词（必填）", Required: true},
	},
	Tips: []string{
		`dws wiki +resolve-space --name 产品文档`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Search spaces by name. tool "search_wikiSpaces" + arg "keyword" are
		// taken verbatim from helpers wiki space search (callMCPTool → server
		// wiki).
		data, err := rt.CallMCPData("wiki", "search_wikiSpaces", map[string]any{
			"keyword": rt.Str("name"),
		})
		if err != nil {
			return err
		}

		// Project candidates to {spaceId, name}, defensively unwrapping the list.
		items := resolveSpaceItems(data)
		candidates := make([]map[string]any, 0, len(items))
		for _, s := range items {
			candidates = append(candidates, map[string]any{
				"spaceId": resolveSpaceID(s),
				"name":    resolveSpaceName(s),
			})
		}

		switch len(candidates) {
		case 0:
			return apperrors.NewValidation("没有找到名称包含 " + rt.Str("name") + " 的知识空间")
		case 1:
			return rt.Output(map[string]any{
				"resolved": true,
				"spaceId":  candidates[0]["spaceId"],
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

// resolveSpaceItems defensively unwraps the list of space candidates from the
// search_wikiSpaces response, tolerating the common container keys the gateway
// may use.
func resolveSpaceItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	for _, key := range []string{"result", "data", "list", "items", "spaces", "records"} {
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
			if inner := resolveSpaceItems(nested); len(inner) > 0 {
				return inner
			}
		}
	}
	return nil
}

// resolveSpaceID reads a space's identifier, tolerating the common id keys.
func resolveSpaceID(s map[string]any) string {
	for _, key := range []string{"spaceId", "space_id", "id"} {
		if v, ok := s[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// resolveSpaceName reads a space's display name, tolerating the common name keys.
func resolveSpaceName(s map[string]any) string {
	for _, key := range []string{"name", "spaceName", "title"} {
		if v, ok := s[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func init() {
	shortcut.Register(ResolveSpace)
}
