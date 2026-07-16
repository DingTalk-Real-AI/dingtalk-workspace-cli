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

// Package wiki declares high-fidelity shortcuts for the DingTalk wiki
// (knowledge base) service: space management, member management and node
// management. Tool names and parameters mirror internal/helpers/wiki.go.
package wiki

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"

// ── space (知识库) ────────────────────────────────────────────

// SpaceCreate → create_wikiSpace
// SpaceGet → get_wikiSpace
// SpaceList → list_wikiSpaces
var SpaceList = shortcut.Shortcut{
	Service:     "wiki",
	Command:     "+space-list",
	Product:     "wiki",
	Description: "列出组织 / 个人知识库",
	Intent:      "当你想浏览自己有权限访问的知识库、拿到目标知识库的 workspaceId 却不确定具体名称时使用；可按类型（组织知识库或我的知识库）分页列出，返回知识库列表，是定位知识库的常用入口。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "type", Type: shortcut.FlagString, Default: "orgWikiSpace", Desc: "知识库类型: orgWikiSpace(默认) / myWikiSpace", Enum: []string{"orgWikiSpace", "myWikiSpace"}},
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页数量 1-50 (默认 20)"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标 (首页留空)"},
	},
	Tips: []string{
		`dws wiki +space-list`,
		`dws wiki +space-list --type myWikiSpace`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Changed("type") {
			params["wikiSpaceType"] = rt.Str("type")
		}
		if rt.Changed("limit") {
			params["pageSize"] = rt.Str("limit")
		}
		if rt.Changed("cursor") {
			params["pageToken"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("wiki", "list_wikiSpaces", params)
		if err != nil {
			return err
		}
		spaces := spaceListProject(data)
		return rt.Output(map[string]any{"count": len(spaces), "spaces": spaces})
	},
}

// spaceListProject reshapes list_wikiSpaces into a clean space list
// ({workspaceId, name, description, createTime}) — output-projection fidelity
// for clean output. The list container and per-item field names are probed defensively
// across candidate keys, so an unrecognized shape yields an empty list.
func spaceListProject(data map[string]any) []map[string]any {
	raw := wikiSpaceRawList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v := wikiSpaceFirst(m, "workspaceId", "workspace_id", "spaceId", "space_id", "id"); v != nil {
			row["workspaceId"] = v
		}
		if v := wikiSpaceFirst(m, "name", "title", "spaceName"); v != nil {
			row["name"] = v
		}
		if v := wikiSpaceFirst(m, "description", "desc"); v != nil {
			row["description"] = v
		}
		if v := wikiSpaceFirst(m, "createTime", "create_time", "gmtCreate", "createdAt"); v != nil {
			row["createTime"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// wikiSpaceRawList locates the space array across candidate container keys,
// tolerating a nested {result|data:{list|items|spaces}} wrapper.
func wikiSpaceRawList(data map[string]any) []any {
	for _, k := range []string{"result", "data", "list", "items", "spaces", "workspaces"} {
		if arr, ok := data[k].([]any); ok {
			return arr
		}
		if inner, ok := data[k].(map[string]any); ok {
			for _, ik := range []string{"list", "items", "spaces", "workspaces", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return nil
}

// wikiSpaceFirst returns the first present value among candidate keys.
func wikiSpaceFirst(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// SpaceSearch → search_wikiSpaces
var SpaceSearch = shortcut.Shortcut{
	Service:     "wiki",
	Command:     "+space-search",
	Product:     "wiki",
	Description: "搜索知识库",
	Intent:      "当你只记得知识库名称的部分关键词、想快速按名称定位某个知识库时使用；输入关键词返回匹配的知识库列表，比逐页 +space-list 更快找到目标 workspaceId。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词", Required: true},
		{Name: "limit", Type: shortcut.FlagString, Desc: "返回数量 1-20 (默认 10)"},
	},
	Tips: []string{`dws wiki +space-search --query "产品文档"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"keyword": rt.Str("query")}
		if rt.Changed("limit") {
			params["pageSize"] = rt.Str("limit")
		}
		data, err := rt.CallMCPData("wiki", "search_wikiSpaces", params)
		if err != nil {
			return err
		}
		spaces := spaceListProject(data)
		return rt.Output(map[string]any{"count": len(spaces), "spaces": spaces})
	},
}

// SpaceDelete → delete_wikiSpace
// ── member (知识库成员) ───────────────────────────────────────

// MemberAdd → add_member
// MemberUpdate → update_member
// MemberList → list_member
// MemberRemove → remove_member
// ── node (知识库节点，路由到 doc MCP server) ──────────────────

// NodeList → list_nodes (doc)
var NodeList = shortcut.Shortcut{
	Service:     "wiki",
	Command:     "+node-list",
	Product:     "doc",
	Description: "列出知识库节点",
	Intent:      "当你要浏览某个知识库的目录结构、查看某文件夹下有哪些文档/子文件夹并拿到它们的 nodeId 时使用；输入 workspace（可选父节点 folder），分页返回该层级的节点列表，是逐层进入知识库定位文档的常用方式。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "workspace", Type: shortcut.FlagString, Desc: "知识库 ID", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "父节点 nodeId (不传则列出根目录)"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量 (默认 50，最大 50)"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"},
	},
	Tips: []string{`dws wiki +node-list --workspace <workspaceId> --folder <parentNodeId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"workspaceId": rt.Str("workspace")}
		if rt.Changed("folder") {
			params["folderId"] = rt.Str("folder")
		}
		if rt.Changed("limit") {
			params["pageSize"] = rt.Int("limit")
		}
		if rt.Changed("cursor") {
			params["pageToken"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("wiki", "list_nodes", params)
		if err != nil {
			return err
		}
		nodes := nodeListProject(data)
		return rt.Output(map[string]any{"count": len(nodes), "nodes": nodes})
	},
}

// nodeListProject reshapes list_nodes into a clean node list (name/nodeId/type)
// — clean output projection. Container and field keys are probed
// defensively across candidate aliases; an unrecognized shape yields an empty list.
func nodeListProject(data map[string]any) []map[string]any {
	raw := nodeListRawList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v := nodeListFirst(m, "name", "title", "nodeName"); v != nil {
			row["name"] = v
		}
		if v := nodeListFirst(m, "nodeId", "node_id", "id", "uuid", "dentryUuid"); v != nil {
			row["nodeId"] = v
		}
		if v := nodeListFirst(m, "type", "nodeType", "docType", "fileType"); v != nil {
			row["type"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// nodeListRawList locates the node array across candidate container keys,
// tolerating a nested {result|data:{list|items|nodes}} wrapper.
func nodeListRawList(data map[string]any) []any {
	for _, k := range []string{"result", "data", "list", "items", "nodes"} {
		if arr, ok := data[k].([]any); ok {
			return arr
		}
		if inner, ok := data[k].(map[string]any); ok {
			for _, ik := range []string{"list", "items", "nodes", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return nil
}

// nodeListFirst returns the first present value among candidate keys.
func nodeListFirst(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// NodeCreate → create_file (doc)
// NodeCopy → copy_document (doc)
var NodeCopy = shortcut.Shortcut{
	Service:     "wiki",
	Command:     "+node-copy",
	Product:     "doc",
	Description: "复制知识库节点",
	Intent:      "当你想基于已有文档/文件夹快速生成一份副本（如用模板起草新文档、留档备份）时使用；指定源 node 和目标 folder，会实际在知识库中复制出一个新节点，原节点保持不变。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "workspace", Type: shortcut.FlagString, Desc: "知识库 ID", Required: true},
		{Name: "node", Type: shortcut.FlagString, Desc: "源节点 ID", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 nodeId (不传则复制到根目录)"},
	},
	Tips: []string{`dws wiki +node-copy --workspace <workspaceId> --node <nodeId> --folder <targetFolderId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"nodeId":      rt.Str("node"),
			"workspaceId": rt.Str("workspace"),
		}
		if rt.Changed("folder") {
			params["targetFolderId"] = rt.Str("folder")
		}
		return rt.CallMCP("copy_document", params)
	},
}

// NodeMove → move_document (doc)
var NodeMove = shortcut.Shortcut{
	Service:     "wiki",
	Command:     "+node-move",
	Product:     "doc",
	Description: "移动知识库节点",
	Intent:      "当你要重新整理知识库目录、把某个文档或文件夹从当前位置挪到另一个文件夹（或根目录）下时使用；指定源 node 和目标 folder，会实际改变该节点在知识库中的所属位置。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "workspace", Type: shortcut.FlagString, Desc: "知识库 ID", Required: true},
		{Name: "node", Type: shortcut.FlagString, Desc: "源节点 ID", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 nodeId (不传则移动到根目录)"},
	},
	Tips: []string{`dws wiki +node-move --workspace <workspaceId> --node <nodeId> --folder <targetFolderId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"nodeId":      rt.Str("node"),
			"workspaceId": rt.Str("workspace"),
		}
		if rt.Changed("folder") {
			params["targetFolderId"] = rt.Str("folder")
		}
		return rt.CallMCP("move_document", params)
	},
}

// NodeDelete → delete_document (doc)
// NodeSearch → search_documents (doc)
func init() {
	shortcut.Register(
		SpaceList,
		SpaceSearch,
		NodeList,
		NodeCopy,
		NodeMove,
	)
}
