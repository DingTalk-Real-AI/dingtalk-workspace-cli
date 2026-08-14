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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// WikiNewDoc: create a new document node inside a knowledge space identified BY
// NAME, in one command.
//
// Steps: search knowledge spaces by the given name (search_wikiSpaces) → resolve
// exactly one space to its workspaceId (0 or >1 matches → a clear disambiguation
// error, never a guess) → create an online document node under that space's root
// (create_file on the doc MCP server, mirroring `dws wiki node create`).
// Replaces the manual dance of `dws wiki space search --query <name>` (copy the
// workspaceId) → `dws wiki node create --workspace <id> --name <title>`.
//
//	dws wiki +wiki-new-doc --space "产品文档库" --title "需求评审纪要"
var WikiNewDoc = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "wiki",
	Command:       "+wiki-new-doc",
	Product:       "wiki",
	Description:   "在指定名称的知识库下新建一个文档节点（自动按空间名解析 workspaceId）",
	Intent: "当你只知道知识库（知识空间）的名字、想直接在它下面新建一篇文档，却不想先搜索空间、复制 workspaceId 再建节点时使用；" +
		"内部先按空间名搜索知识库，若唯一命中则拿到它的 workspaceId，再在该库根目录下创建一个在线文档节点。" +
		"如果这个名字没有匹配到任何知识库，或匹配到多个，会报错让你用更精确的名字，绝不乱猜。" +
		"这会真实创建一个新的文档节点。",
	Risk:   shortcut.RiskWrite,
	Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "non_idempotent"},
	Contract: corecmd.ContractDecl{
		Description: "在指定名称的知识库下新建一个文档节点（自动按空间名解析 workspaceId）",
		Result:      &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess}, DataSchema: json.RawMessage(`{"type":"object","description":"已验证的新建 Wiki 文档","properties":{"success":{"type":"boolean","description":"是否成功"},"nodeId":{"type":"string","description":"新文档节点 ID"},"space":{"type":"string","description":"请求的知识库名称"},"title":{"type":"string","description":"请求的文档标题"},"document":{"type":"object","description":"读回的文档元数据","additionalProperties":true}},"required":["success","nodeId","space","title","document"],"additionalProperties":true}`)},
		Interface:   &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: "Reviewed Wiki smart Shortcut: the executable CLI strictly resolves one exact space, creates a document, and verifies it through a metadata read-back."},
		Selection:   contract.SelectionSpec{AgentSummary: "在指定名称的知识库下新建一个文档节点（自动按空间名解析 workspaceId）", UseWhen: []string{"当你只知道知识库（知识空间）的名字、想直接在它下面新建一篇文档，却不想先搜索空间、复制 workspaceId 再建节点时使用；内部先按空间名搜索知识库，若唯一命中则拿到它的 workspaceId，再在该库根目录下创建一个在线文档节点。如果这个名字没有匹配到任何知识库，或匹配到多个，会报错让你用更精确的名字，绝不乱猜。这会真实创建一个新的文档节点。"}, AvoidWhen: []string{"已知 workspaceId 时用 wiki +node-create；空间名不唯一时先用 wiki +space-search"}, Examples: []string{`dws wiki +wiki-new-doc --space "产品文档库" --title "需求评审纪要"`}},
		Identity:    contract.ToolIdentitySpec{ProductID: "wiki", Name: "shortcut_wiki_new_doc", CanonicalPath: "wiki.shortcut_wiki_new_doc", CLIPath: "wiki +wiki-new-doc", PrimaryCLIPath: "wiki +wiki-new-doc"},
		Parameters:  []contract.ParamDecl{{Name: "space", Property: "keyword"}, {Name: "title", Property: "name"}},
	},
	Flags: []shortcut.Flag{
		{Name: "space", Type: shortcut.FlagString, Desc: "知识库（知识空间）名称", Required: true},
		{Name: "title", Type: shortcut.FlagString, Desc: "新建文档的标题", Required: true},
	},
	Tips: []string{
		`dws wiki +wiki-new-doc --space "产品文档库" --title "需求评审纪要"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		spaceName := strings.TrimSpace(rt.Str("space"))
		title := strings.TrimSpace(rt.Str("title"))
		if spaceName == "" {
			return apperrors.NewValidation("--space 不能为空")
		}
		if title == "" {
			return apperrors.NewValidation("--title 不能为空")
		}

		// Step 1 — search knowledge spaces by name. keyword param mirrors the
		// helper's `wiki space search` call site (search_wikiSpaces).
		data, err := rt.CallMCPData("wiki", "search_wikiSpaces", map[string]any{
			"keyword": spaceName,
		})
		if err != nil {
			return err
		}

		// Step 2 — resolve exactly one space to its workspaceId; refuse to guess
		// on 0 or multiple exact-name matches.
		workspaceID, err := wikiNewDocResolveSpaceID(data, spaceName)
		if err != nil {
			return err
		}

		// Step 3 — create the document node under the space root. workspaceId /
		// name / type params copied verbatim from the helper's `wiki node create`
		// call site (create_file lives on the doc MCP server, so route there
		// explicitly rather than this shortcut's own product; adoc = 在线文档).
		// Guard the create explicitly: the search above is a read and already ran.
		if rt.DryRun() {
			return rt.Output(map[string]any{
				"dryRun":        true,
				"space":         spaceName,
				"title":         title,
				"wouldCreateIn": workspaceID,
			})
		}
		created, err := rt.CallMCPWriteDataStrict("doc", "create_file", map[string]any{
			"workspaceId": workspaceID,
			"name":        title,
			"type":        "adoc",
		})
		if err != nil {
			return err
		}
		if success, ok := created["success"].(bool); !ok || !success {
			return apperrors.NewAPI("create_file 未返回 success=true，无法证明文档已创建", apperrors.WithOperation("doc/create_file"), apperrors.WithReason("missing_terminal_success"))
		}
		nodeID := wikiNewDocFirstString(created, "nodeId", "fileId", "id")
		if nodeID == "" {
			return apperrors.NewAPI("create_file 未返回 nodeId，远端效果未知", apperrors.WithOperation("doc/create_file"), apperrors.WithReason("missing_created_id"))
		}
		verified, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": nodeID})
		if err != nil {
			return err
		}
		if success, present := verified["success"]; present {
			value, ok := success.(bool)
			if !ok || !value {
				return apperrors.NewAPI("新建文档读回未成功", apperrors.WithOperation("doc/get_document_info"), apperrors.WithReason("readback_failed"))
			}
		}
		if wikiNewDocFirstString(verified, "nodeId", "fileId", "id") != nodeID {
			return apperrors.NewAPI("新建文档读回 nodeId 不一致", apperrors.WithOperation("doc/get_document_info"), apperrors.WithReason("readback_id_mismatch"))
		}
		return rt.Output(map[string]any{
			"success": true, "nodeId": nodeID, "space": spaceName, "title": title, "document": verified,
		})
	},
}

// wikiSpaceCandidate is the minimal {id, name} a space needs after searching.
type wikiSpaceCandidate struct {
	id   string
	name string
}

// wikiNewDocResolveSpaceID pulls the single space whose name matches spaceName
// out of a search_wikiSpaces response and returns its workspaceId. It errors
// clearly when nothing matches or when the name is ambiguous, never guessing.
func wikiNewDocResolveSpaceID(data map[string]any, spaceName string) (string, error) {
	spaces, err := wikiNewDocExtractSpaces(data)
	if err != nil {
		return "", err
	}
	if len(spaces) == 0 {
		return "", apperrors.NewValidation(fmt.Sprintf(
			"没找到名为 %q 的知识库；换个更完整/精确的空间名再试。", spaceName))
	}

	// Prefer exact (case-insensitive) name matches to disambiguate a keyword
	// search that may return partial hits.
	var exact []wikiSpaceCandidate
	for _, s := range spaces {
		if strings.EqualFold(strings.TrimSpace(s.name), spaceName) {
			exact = append(exact, s)
		}
	}

	candidates := exact
	if len(candidates) == 0 {
		// No exact match: fall back to whatever the search returned so we can
		// give a precise disambiguation message instead of a blind pick.
		candidates = spaces
	}

	switch {
	case len(candidates) == 1:
		if candidates[0].id == "" {
			return "", apperrors.NewValidation(fmt.Sprintf(
				"匹配到知识库 %q，但返回结果里没有可用的 workspaceId。", candidates[0].name))
		}
		return candidates[0].id, nil
	default:
		return "", apperrors.NewValidation(fmt.Sprintf(
			"%q 匹配到 %d 个知识库：%s。请用更精确的空间名再试。",
			spaceName, len(candidates), strings.Join(wikiNewDocLabels(candidates), "、")))
	}
}

// wikiNewDocExtractSpaces flattens the several shapes a search_wikiSpaces
// response may take into a list of {id, name} candidates. The gateway wraps the
// list under one of several common container keys, so probe them defensively.
func wikiNewDocExtractSpaces(data map[string]any) ([]wikiSpaceCandidate, error) {
	if data == nil {
		return nil, apperrors.NewAPI("search_wikiSpaces 返回空响应，不能当作零命中", apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("empty_tool_response"))
	}
	if success, present := data["success"]; present {
		value, ok := success.(bool)
		if !ok || !value {
			return nil, apperrors.NewAPI("search_wikiSpaces 未成功", apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("remote_failure"))
		}
	}
	for _, key := range []string{"result", "data", "list", "wikiSpaces", "spaces", "items", "records"} {
		switch v := data[key].(type) {
		case []any:
			return wikiNewDocToCandidates(v)
		case map[string]any:
			for _, k2 := range []string{"list", "wikiSpaces", "spaces", "items", "records", "result"} {
				if raw, present := v[k2]; present {
					arr, ok := raw.([]any)
					if !ok {
						return nil, apperrors.NewAPI("search_wikiSpaces 业务集合不是数组", apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("malformed_collection"))
					}
					return wikiNewDocToCandidates(arr)
				}
			}
		}
	}
	return nil, apperrors.NewAPI("search_wikiSpaces 缺少 wikiSpaces 数组，不能投影为空", apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("missing_collection"))
}

func wikiNewDocToCandidates(arr []any) ([]wikiSpaceCandidate, error) {
	out := make([]wikiSpaceCandidate, 0, len(arr))
	for index, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, apperrors.NewAPI(fmt.Sprintf("search_wikiSpaces 结果第 %d 项不是对象", index), apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("malformed_collection_item"))
		}
		id := ""
		for _, k := range []string{"workspaceId", "spaceId", "id"} {
			if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
				id = s
				break
			}
		}
		name := ""
		for _, k := range []string{"name", "spaceName", "title"} {
			if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
				name = s
				break
			}
		}
		if id == "" || name == "" {
			return nil, apperrors.NewAPI(fmt.Sprintf("search_wikiSpaces 结果第 %d 项缺少名称或 workspaceId", index), apperrors.WithOperation("wiki/search_wikiSpaces"), apperrors.WithReason("malformed_collection_item"))
		}
		out = append(out, wikiSpaceCandidate{id: id, name: name})
	}
	return out, nil
}

func wikiNewDocFirstString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	for _, wrapper := range []string{"result", "data"} {
		if inner, ok := data[wrapper].(map[string]any); ok {
			for _, key := range keys {
				if value, ok := inner[key].(string); ok && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return ""
}

func wikiNewDocLabels(spaces []wikiSpaceCandidate) []string {
	out := make([]string, 0, len(spaces))
	for _, s := range spaces {
		out = append(out, fmt.Sprintf("%s(%s)", s.name, s.id))
	}
	return out
}

func init() {
	shortcut.Register(WikiNewDoc)
}
