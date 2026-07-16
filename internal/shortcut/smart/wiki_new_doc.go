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
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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
	Service:     "wiki",
	Command:     "+wiki-new-doc",
	Product:     "wiki",
	Description: "在指定名称的知识库下新建一个文档节点（自动按空间名解析 workspaceId）",
	Intent: "当你只知道知识库（知识空间）的名字、想直接在它下面新建一篇文档，却不想先搜索空间、复制 workspaceId 再建节点时使用；" +
		"内部先按空间名搜索知识库，若唯一命中则拿到它的 workspaceId，再在该库根目录下创建一个在线文档节点。" +
		"如果这个名字没有匹配到任何知识库，或匹配到多个，会报错让你用更精确的名字，绝不乱猜。" +
		"这会真实创建一个新的文档节点。",
	Risk: shortcut.RiskWrite,
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
		created, err := rt.CallMCPWriteData("doc", "create_file", map[string]any{
			"workspaceId": workspaceID,
			"name":        title,
			"type":        "adoc",
		})
		if err != nil {
			return err
		}
		// Surface the created doc (id/url in the response) instead of returning
		// silently — previously the caller got no confirmation of the new doc.
		return rt.Output(map[string]any{
			"created": true,
			"space":   spaceName,
			"title":   title,
			"result":  created,
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
	spaces := wikiNewDocExtractSpaces(data)
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
func wikiNewDocExtractSpaces(data map[string]any) []wikiSpaceCandidate {
	if data == nil {
		return nil
	}
	for _, key := range []string{"result", "data", "list", "wikiSpaces", "spaces", "items", "records"} {
		switch v := data[key].(type) {
		case []any:
			return wikiNewDocToCandidates(v)
		case map[string]any:
			for _, k2 := range []string{"list", "wikiSpaces", "spaces", "items", "records", "result"} {
				if arr, ok := v[k2].([]any); ok {
					return wikiNewDocToCandidates(arr)
				}
			}
		}
	}
	return nil
}

func wikiNewDocToCandidates(arr []any) []wikiSpaceCandidate {
	out := make([]wikiSpaceCandidate, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
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
		if id == "" && name == "" {
			continue
		}
		out = append(out, wikiSpaceCandidate{id: id, name: name})
	}
	return out
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
