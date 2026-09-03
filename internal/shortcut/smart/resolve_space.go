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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
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
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "wiki",
	Command:       "+resolve-space",
	Product:       "wiki",
	Description:   "按名称搜索知识空间并解析出唯一 spaceId（只读）",
	Intent: "当你只知道某个知识空间（wiki space）的名称（或名称里的关键词）、想把它解析成可直接用于后续工具的 spaceId 时使用；" +
		"内部按 --name 关键词调用 search_wikiSpaces 搜索知识空间，再在本地投影出每个候选的 spaceId 和 name。" +
		"如果只命中一个知识空间就直接返回它的 spaceId；如果命中多个则列出全部候选让你消歧，绝不替你瞎猜；如果一个都没命中则提示未找到。" +
		"这是纯只读操作，只做搜索与本地投影，不会修改任何知识空间。",
	Risk:   shortcut.RiskRead,
	Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: corecmd.ContractDecl{
		Description: "按名称搜索知识空间并解析出唯一 spaceId（只读）",
		Result:      &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess}, DataSchema: json.RawMessage(`{"type":"object","description":"知识库名称解析结果","properties":{"resolved":{"type":"boolean","description":"是否唯一解析"},"spaceId":{"type":"string","description":"唯一知识库 ID"},"name":{"type":"string","description":"唯一知识库名称"},"count":{"type":"integer","description":"候选数量"},"candidates":{"type":"array","description":"需要消歧的候选知识库","items":{"type":"object","description":"知识库候选","additionalProperties":true}}},"required":["resolved"],"additionalProperties":true}`)},
		Interface:   &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: "Reviewed Wiki resolver: the executable CLI strictly validates search results and refuses to guess when multiple spaces match."},
		Selection:   contract.SelectionSpec{AgentSummary: "按名称搜索知识空间并解析出唯一 spaceId（只读）", UseWhen: []string{"当你只知道某个知识空间（wiki space）的名称（或名称里的关键词）、想把它解析成可直接用于后续工具的 spaceId 时使用；内部按 --name 关键词调用 search_wikiSpaces 搜索知识空间，再在本地投影出每个候选的 spaceId 和 name。如果只命中一个知识空间就直接返回它的 spaceId；如果命中多个则列出全部候选让你消歧，绝不替你瞎猜；如果一个都没命中则提示未找到。这是纯只读操作，只做搜索与本地投影，不会修改任何知识空间。"}, AvoidWhen: []string{"只想浏览所有匹配项用 wiki +space-search；已知 workspaceId 时无需解析"}, Examples: []string{`dws wiki +resolve-space --name "产品文档"`}},
		Identity:    contract.ToolIdentitySpec{ProductID: "wiki", Name: "shortcut_resolve_space", CanonicalPath: "wiki.shortcut_resolve_space", CLIPath: "wiki +resolve-space", PrimaryCLIPath: "wiki +resolve-space"},
		Parameters:  []contract.ParamDecl{{Name: "name", Property: "keyword"}},
	},
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
		items, err := resolveSpaceItemsStrict(data)
		if err != nil {
			return err
		}
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
	for _, key := range []string{"result", "data", "list", "items", "wikiSpaces", "spaces", "records"} {
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
	for _, key := range []string{"workspaceId", "spaceId", "space_id", "id"} {
		if v, ok := s[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func resolveSpaceItemsStrict(data map[string]any) ([]map[string]any, error) {
	if len(data) == 0 {
		return nil, apperrors.NewAPI("search_wikiSpaces 返回空响应，不能当作零命中", apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("empty_tool_response"))
	}
	if success, present := data["success"]; present {
		value, ok := success.(bool)
		if !ok || !value {
			return nil, apperrors.NewAPI("search_wikiSpaces 未成功", apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("remote_failure"))
		}
	}
	containers := []map[string]any{data}
	for _, wrapper := range []string{"result", "data"} {
		if raw, present := data[wrapper]; present {
			inner, ok := raw.(map[string]any)
			if !ok {
				return nil, apperrors.NewAPI("search_wikiSpaces 响应包装不是对象", apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("malformed_envelope"))
			}
			containers = append(containers, inner)
		}
	}
	for _, container := range containers {
		for _, key := range []string{"wikiSpaces", "spaces", "items", "list", "records"} {
			raw, present := container[key]
			if !present {
				continue
			}
			list, ok := raw.([]any)
			if !ok {
				return nil, apperrors.NewAPI("search_wikiSpaces 业务集合不是数组", apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("malformed_collection"))
			}
			out := make([]map[string]any, 0, len(list))
			for index, item := range list {
				object, ok := item.(map[string]any)
				if !ok {
					return nil, apperrors.NewAPI(fmt.Sprintf("search_wikiSpaces 第 %d 项不是对象", index), apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("malformed_collection_item"))
				}
				if resolveSpaceID(object) == "" || resolveSpaceName(object) == "" {
					return nil, apperrors.NewAPI(fmt.Sprintf("search_wikiSpaces 第 %d 项缺少名称或 workspaceId", index), apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("malformed_collection_item"))
				}
				out = append(out, object)
			}
			return out, nil
		}
	}
	return nil, apperrors.NewAPI("search_wikiSpaces 缺少 wikiSpaces 数组，不能投影为空", apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("missing_collection"))
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
