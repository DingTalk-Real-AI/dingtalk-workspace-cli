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

// Package doc declares the high-fidelity `dws doc +<command>` shortcuts.
// Tool names and parameter keys are lifted verbatim from
// internal/helpers/doc.go (the single source of truth for DingTalk MCP tools).
package doc

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const (
	productDoc     = "doc"
	productComment = "doc-comment"
)

// ── 文档浏览 / 读取 ──────────────────────────────────────────

var Search = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+search",
	Product:     productDoc,
	Description: "按关键词搜索有权限的文档 (不传则返回最近访问)",
	Intent:      "当你只记得文档的标题或主题词、需要先定位到某篇钉钉文档拿到它的 nodeId/URL 以便后续阅读或编辑时使用；可按关键词、扩展名、创建/访问时间、创建者等条件过滤，不传关键词则返回最近访问的文档，返回匹配的文档列表。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_search",
			CanonicalPath:  "doc.shortcut_search",
			CLIPath:        "doc +search",
			PrimaryCLIPath: "doc +search",
		},
		Description: "按关键词搜索有权限的文档 (不传则返回最近访问)",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按关键词搜索有权限的文档 (不传则返回最近访问)",
			UseWhen:      []string{"当你只记得文档的标题或主题词、需要先定位到某篇钉钉文档拿到它的 nodeId/URL 以便后续阅读或编辑时使用；可按关键词、扩展名、创建/访问时间、创建者等条件过滤，不传关键词则返回最近访问的文档，返回匹配的文档列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws doc +search --query \"会议纪要\"",
				"dws doc +search --extensions pdf,docx",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词，不传返回最近访问的文档"},
		{Name: "extensions", Type: shortcut.FlagStringSlice, Desc: "按文件扩展名过滤 (如 adoc,axls,pdf)"},
		{Name: "created-from", Type: shortcut.FlagInt, Desc: "创建时间起始 (毫秒时间戳)"},
		{Name: "created-to", Type: shortcut.FlagInt, Desc: "创建时间截止 (毫秒时间戳)"},
		{Name: "visited-from", Type: shortcut.FlagInt, Desc: "访问时间起始 (毫秒时间戳)"},
		{Name: "visited-to", Type: shortcut.FlagInt, Desc: "访问时间截止 (毫秒时间戳)"},
		{Name: "creator-uids", Type: shortcut.FlagStringSlice, Desc: "按创建者用户 ID 过滤"},
		{Name: "editor-uids", Type: shortcut.FlagStringSlice, Desc: "按编辑者用户 ID 过滤"},
		{Name: "mentioned-uids", Type: shortcut.FlagStringSlice, Desc: "按 @提及的用户 ID 过滤"},
		{Name: "workspace-ids", Type: shortcut.FlagStringSlice, Desc: "按知识库 ID 过滤"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量 (默认 10，最大 30)"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标 (上次结果的 nextPageToken)"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "有界读取全部后续页"},
		{Name: "max-pages", Type: shortcut.FlagInt, Default: "20", Desc: "--page-all 最大页数"},
		{Name: "max-items", Type: shortcut.FlagInt, Default: "500", Desc: "最多返回文档数"},
	},
	Tips: []string{`dws doc +search --query "会议纪要"`, `dws doc +search --extensions pdf,docx`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if v := rt.Str("query"); v != "" {
			params["keyword"] = v
		}
		if rt.Changed("extensions") {
			params["extensions"] = rt.StrSlice("extensions")
		}
		if rt.Changed("created-from") {
			params["createdTimeFrom"] = rt.Int("created-from")
		}
		if rt.Changed("created-to") {
			params["createdTimeTo"] = rt.Int("created-to")
		}
		if rt.Changed("visited-from") {
			params["visitedTimeFrom"] = rt.Int("visited-from")
		}
		if rt.Changed("visited-to") {
			params["visitedTimeTo"] = rt.Int("visited-to")
		}
		if rt.Changed("creator-uids") {
			params["creatorUserIds"] = rt.StrSlice("creator-uids")
		}
		if rt.Changed("editor-uids") {
			params["editorUserIds"] = rt.StrSlice("editor-uids")
		}
		if rt.Changed("mentioned-uids") {
			params["mentionedUserIds"] = rt.StrSlice("mentioned-uids")
		}
		if rt.Changed("workspace-ids") {
			params["workspaceIds"] = rt.StrSlice("workspace-ids")
		}
		pageSize := rt.Int("limit")
		if pageSize == 0 {
			pageSize = 10
		}
		result, err := collectDocPages(rt, "search_documents", "documents", params, searchDocsProject, docPageOptions{
			PageAll: rt.Bool("page-all"), PageSize: pageSize, MaxPages: rt.Int("max-pages"), MaxItems: rt.Int("max-items"), Cursor: rt.Str("cursor"),
		})
		if err != nil {
			return err
		}
		return rt.Output(result)
	},
}

// searchDocsProject reshapes the raw search_documents response into a clean
// document list ({nodeId, name, docType, url, creatorId, modifiedTime}) —
// clean output projection. Both the list container and per-item
// field names are probed defensively across candidate keys so response-shape
// drift yields an empty/partial list rather than a crash or fabricated data.
func searchDocsProject(data map[string]any) []map[string]any {
	raw := docResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := docFirst(m, "nodeId", "node_id", "id", "docId", "doc_id"); ok {
			row["nodeId"] = v
		}
		if v, ok := docFirst(m, "name", "title", "docName", "fileName"); ok {
			row["name"] = v
		}
		if v, ok := docFirst(m, "docType", "doc_type", "type", "extension", "fileType"); ok {
			row["docType"] = v
		}
		if v, ok := docFirst(m, "url", "docUrl", "nodeUrl", "webUrl"); ok {
			row["url"] = v
		}
		if v, ok := docFirst(m, "creatorId", "creatorUserId", "creator_user_id", "creator"); ok {
			row["creatorId"] = v
		}
		if v, ok := docFirst(m, "modifiedTime", "gmtModified", "visitedTime", "updateTime", "modifyTime"); ok {
			row["modifiedTime"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// docResolveList locates the list payload inside a doc-service response,
// tolerating a bare top-level array or nesting under common envelope keys, and
// optionally one level deeper inside a result/data container.
func docResolveList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, key := range []string{"nodes", "documents", "list", "items", "result", "data", "records"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"nodes", "documents", "list", "items", "records", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// docFirst returns the first present candidate key's value.
func docFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

var List = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+list",
	Product:     productDoc,
	Description: "列出文件夹或知识库下的直接子节点",
	Intent:      "当你已知某个文档文件夹或知识库的 ID、想浏览它下面直接包含的文档与子文件夹（不递归深层）以便逐层导航时使用；输入 folder 或 workspace，返回该层级的子节点列表。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_list",
			CanonicalPath:  "doc.shortcut_list",
			CLIPath:        "doc +list",
			PrimaryCLIPath: "doc +list",
		},
		Description: "列出文件夹或知识库下的直接子节点",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出文件夹或知识库下的直接子节点",
			UseWhen:      []string{"当你已知某个文档文件夹或知识库的 ID、想浏览它下面直接包含的文档与子文件夹（不递归深层）以便逐层导航时使用；输入 folder 或 workspace，返回该层级的子节点列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws doc +list --folder DOC_FOLDER_NODE_ID",
				"dws doc +list --workspace WS_ID --limit 20",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "folder", Type: shortcut.FlagString, Desc: "文档文件夹 nodeId 或 alidocs 文件夹 URL"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "知识库 ID"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量 (默认 50，最大 50)"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标 (上次结果的 nextPageToken)"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "有界读取全部后续页"},
		{Name: "max-pages", Type: shortcut.FlagInt, Default: "20", Desc: "--page-all 最大页数"},
		{Name: "max-items", Type: shortcut.FlagInt, Default: "500", Desc: "最多返回节点数"},
	},
	Tips: []string{`dws doc +list --folder DOC_FOLDER_NODE_ID`, `dws doc +list --workspace WS_ID --limit 20`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Changed("folder") {
			params["folderId"] = rt.Str("folder")
		}
		if rt.Changed("workspace") {
			params["workspaceId"] = rt.Str("workspace")
		}
		pageSize := rt.Int("limit")
		if pageSize == 0 {
			pageSize = 50
		}
		result, err := collectDocPages(rt, "list_nodes", "nodes", params, listNodesProject, docPageOptions{
			PageAll: rt.Bool("page-all"), PageSize: pageSize, MaxPages: rt.Int("max-pages"), MaxItems: rt.Int("max-items"), Cursor: rt.Str("cursor"),
		})
		if err != nil {
			return err
		}
		return rt.Output(result)
	},
}

// listNodesProject reshapes the raw list_nodes response into a clean child-node
// list ({nodeId, name, nodeType, url}) — clean output projection.
// The list container and per-item field names are probed defensively via the
// shared docResolveList/docFirst helpers, so an unknown shape yields an empty
// list rather than a crash or fabricated data.
func listNodesProject(data map[string]any) []map[string]any {
	raw := docResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := docFirst(m, "nodeId", "node_id", "id", "docId", "doc_id"); ok {
			row["nodeId"] = v
		}
		if v, ok := docFirst(m, "name", "title", "nodeName", "fileName"); ok {
			row["name"] = v
		}
		if v, ok := docFirst(m, "nodeType", "node_type", "docType", "type", "extension"); ok {
			row["nodeType"] = v
		}
		if v, ok := docFirst(m, "url", "nodeUrl", "docUrl", "webUrl"); ok {
			row["url"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// ── 文档创建 / 更新 ──────────────────────────────────────────

var Copy = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+copy",
	Product:     productDoc,
	Description: "复制文档/文件到指定文件夹或知识库",
	Intent:      "当你想保留原件、在另一个文件夹或知识库里生成一份文档/文件副本（例如以某篇文档为模板另存）时使用；输入源 node 与目标 folder/workspace，会实际创建一个副本。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_copy",
			CanonicalPath:  "doc.shortcut_copy",
			CLIPath:        "doc +copy",
			PrimaryCLIPath: "doc +copy",
		},
		Description: "复制文档/文件到指定文件夹或知识库",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "复制文档/文件到指定文件夹或知识库",
			UseWhen:      []string{"当你想保留原件、在另一个文件夹或知识库里生成一份文档/文件副本（例如以某篇文档为模板另存）时使用；输入源 node 与目标 folder/workspace，会实际创建一个副本。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws doc +copy --node DOC_ID --folder TARGET_FOLDER_NODE_ID"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档/文件 ID 或 URL", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文档文件夹 nodeId 或 alidocs 文件夹 URL"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "目标知识库 ID"},
	},
	Tips: []string{`dws doc +copy --node DOC_ID --folder TARGET_FOLDER_NODE_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node")}
		if rt.Changed("folder") {
			params["targetFolderId"] = rt.Str("folder")
		}
		if rt.Changed("workspace") {
			params["workspaceId"] = rt.Str("workspace")
		}
		return rt.CallMCP("copy_document", params)
	},
}

var Move = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+move",
	Product:     productDoc,
	Description: "移动文档/文件到指定文件夹或知识库",
	Intent:      "当你要整理文档归属、把某篇文档/文件从当前位置挪到另一个文件夹或知识库（原位置不再保留）时使用；输入 node 与目标 folder/workspace，会实际改变文件的存放位置。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_move",
			CanonicalPath:  "doc.shortcut_move",
			CLIPath:        "doc +move",
			PrimaryCLIPath: "doc +move",
		},
		Description: "移动文档/文件到指定文件夹或知识库",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "移动文档/文件到指定文件夹或知识库",
			UseWhen:      []string{"当你要整理文档归属、把某篇文档/文件从当前位置挪到另一个文件夹或知识库（原位置不再保留）时使用；输入 node 与目标 folder/workspace，会实际改变文件的存放位置。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws doc +move --node DOC_ID --folder TARGET_FOLDER_NODE_ID"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档/文件 ID 或 URL", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文档文件夹 nodeId 或 alidocs 文件夹 URL"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "目标知识库 ID"},
	},
	Tips: []string{`dws doc +move --node DOC_ID --folder TARGET_FOLDER_NODE_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node")}
		if rt.Changed("folder") {
			params["targetFolderId"] = rt.Str("folder")
		}
		if rt.Changed("workspace") {
			params["workspaceId"] = rt.Str("workspace")
		}
		return rt.CallMCP("move_document", params)
	},
}

// ── 文件 / 文件夹 ────────────────────────────────────────────

// ── 块级编辑 ─────────────────────────────────────────────────

// ── 文档附件 ─────────────────────────────────────────────────

// ── 文档评论 (server: doc-comment) ───────────────────────────

var CommentList = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+comment-list",
	Product:     productComment,
	Description: "查询文档评论列表",
	Intent:      "当你想查看某篇文档上已有的评论、了解有哪些反馈或待处理意见（可按全文/划词、已解决/未解决过滤）时使用；输入 node，可用 --limit/--cursor 分页，返回评论列表及其 commentKey 以便后续回复。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_comment_list",
			CanonicalPath:  "doc.shortcut_comment_list",
			CLIPath:        "doc +comment-list",
			PrimaryCLIPath: "doc +comment-list",
		},
		Description: "查询文档评论列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询文档评论列表",
			UseWhen:      []string{"当你想查看某篇文档上已有的评论、了解有哪些反馈或待处理意见（可按全文/划词、已解决/未解决过滤）时使用；输入 node，可用 --limit/--cursor 分页，返回评论列表及其 commentKey 以便后续回复。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws doc +comment-list --node DOC_ID --limit 20",
				"dws doc +comment-list --node DOC_ID --limit 20 --cursor NEXT_TOKEN",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量 (默认 50，最大 50)", Aliases: []string{"page-size"}},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标 (上一页返回的 nextToken)"},
		{Name: "type", Type: shortcut.FlagString, Desc: "评论类型: global (全文) / inline (划词)", Enum: []string{"global", "inline"}},
		{Name: "resolve-status", Type: shortcut.FlagString, Desc: "解决状态: resolved / unresolved", Enum: []string{"resolved", "unresolved"}},
	},
	Tips: []string{`dws doc +comment-list --node DOC_ID --limit 20`, `dws doc +comment-list --node DOC_ID --limit 20 --cursor NEXT_TOKEN`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node")}
		if rt.Changed("limit") || rt.Changed("page-size") {
			params["pageSize"] = rt.IntFirst("limit", "page-size")
		}
		if v := rt.Str("cursor"); v != "" {
			params["nextToken"] = v
		}
		if v := rt.Str("type"); v != "" {
			params["commentType"] = v
		}
		if v := rt.Str("resolve-status"); v != "" {
			params["resolveStatus"] = v
		}
		return rt.CallMCP("list_comments", params)
	},
}

const commentCreateTargetConstraint = "selection 与 block-id/start/end 高级通道互斥；block-id/start/end 必须一起提供；selection 必须唯一匹配"

var CommentCreate = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+comment-create",
	Product:     productComment,
	Description: "创建全文评论，或按 selection 创建划词评论",
	Intent:      "当用户要对整篇文档留言，或针对文档中唯一匹配的一段文字创建精确划词评论时使用；已知 block/start/end 时也可直接走高级通道。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: docContract("+comment-create", "创建全文评论，或按 selection 创建划词评论",
		"当用户要对整篇文档留言，或针对文档中唯一匹配的一段文字创建精确划词评论时使用；已知 block/start/end 时也可直接走高级通道。",
		[]string{`dws doc +comment-create --node <DOC_ID> --content "请补充数据来源"`, `dws doc +comment-create --node <DOC_ID> --selection "计划下周发布" --content "请确认日期"`},
		contract.ParamDecl{Name: "node", Property: "nodeId"},
		contract.ParamDecl{Name: "content", Property: "content"},
		contract.ParamDecl{Name: "selection", Property: "selection"},
		contract.ParamDecl{Name: "block-id", Property: "blockId"},
		contract.ParamDecl{Name: "start", Property: "start"},
		contract.ParamDecl{Name: "end", Property: "end"},
		contract.ParamDecl{Name: "selected-text", Property: "selectedText"},
		contract.ParamDecl{Name: "mention", Property: "mentionedUserIds"}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "评论文字内容", Required: true},
		{Name: "selection", Type: shortcut.FlagString, Desc: "完整文字或 前缀...后缀；" + commentCreateTargetConstraint},
		{Name: "block-id", Type: shortcut.FlagString, Desc: "高级通道 block ID；" + commentCreateTargetConstraint},
		{Name: "start", Type: shortcut.FlagInt, Desc: "块内 UTF-16 起始偏移；" + commentCreateTargetConstraint},
		{Name: "end", Type: shortcut.FlagInt, Desc: "块内 UTF-16 结束偏移；" + commentCreateTargetConstraint},
		{Name: "selected-text", Type: shortcut.FlagString, Desc: "可选引用原文；CLI 会从 block 回读并交叉校验"},
		{Name: "mention", Type: shortcut.FlagStringSlice, Desc: "被 @ 的用户 uid，多个值用逗号分隔；不要传 JSON 数组"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"selection", "block-id", "start", "end"}, Description: commentCreateTargetConstraint}},
	Tips:        []string{`dws doc +comment-create --node <DOC_ID> --content "请补充数据来源"`, `dws doc +comment-create --node <DOC_ID> --selection "计划下周发布" --content "请确认日期"`},
	Validate:    validateCommentCreate,
	Execute:     executeCommentCreate,
}

var CommentReply = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+comment-reply",
	Product:     productComment,
	Description: "回复文档中的一条评论",
	Intent:      "当你要针对某条已有评论进行回复、参与讨论或用表情贴图回应时使用；先从评论列表拿到 comment-key，再输入 node、comment-key 与 content（--emoji 则作为表情回复），会实际发布一条回复。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_comment_reply",
			CanonicalPath:  "doc.shortcut_comment_reply",
			CLIPath:        "doc +comment-reply",
			PrimaryCLIPath: "doc +comment-reply",
		},
		Description: "回复文档中的一条评论",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "回复文档中的一条评论",
			UseWhen:      []string{"当你要针对某条已有评论进行回复、参与讨论或用表情贴图回应时使用；先从评论列表拿到 comment-key，再输入 node、comment-key 与 content（--emoji 则作为表情回复），会实际发布一条回复。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws doc +comment-reply --node DOC_ID --comment-key COMMENT_KEY --content \"同意\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "回复文字内容 (表情回复时填表情名称)", Required: true},
		{Name: "comment-key", Type: shortcut.FlagString, Desc: "被回复评论的 commentKey (从 list/create 获取)", Required: true},
		{Name: "emoji", Type: shortcut.FlagBool, Desc: "作为表情贴图回复"},
		{Name: "mention", Type: shortcut.FlagStringSlice, Desc: "被 @ 的用户 uid，多个值用逗号分隔；不要传 JSON 数组"},
	},
	Tips: []string{`dws doc +comment-reply --node DOC_ID --comment-key COMMENT_KEY --content "同意"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"nodeId":          rt.Str("node"),
			"content":         rt.Str("content"),
			"replyCommentKey": rt.Str("comment-key"),
		}
		if rt.Bool("emoji") {
			params["emoji"] = true
		}
		if rt.Changed("mention") {
			mentions, err := normalizeMentionUserIDs(rt.StrSlice("mention"))
			if err != nil {
				return err
			}
			params["mentionedUserIds"] = mentions
		}
		return rt.CallMCP("reply_comment", params)
	},
}

var CommentCreateInline = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+comment-create-inline",
	Product:     productComment,
	Description: "兼容入口：按 block/start/end 创建划词评论",
	Intent:      "仅兼容既有调用；新任务统一使用 +comment-create 的 selection 或 block/start/end 通道。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "评论文字内容 (纯文本)", Required: true},
		{Name: "block-id", Type: shortcut.FlagString, Desc: "评论标记所在的块 ID (通过 +block-list 获取)", Required: true},
		{Name: "start", Type: shortcut.FlagInt, Desc: "块内文本起始字符偏移量 (从 0 开始)", Required: true},
		{Name: "end", Type: shortcut.FlagInt, Desc: "块内文本结束字符偏移量 (须大于 start)", Required: true},
		{Name: "selected-text", Type: shortcut.FlagString, Desc: "选中文本内容 (展示引用原文)"},
		{Name: "mention", Type: shortcut.FlagStringSlice, Desc: "被 @ 的用户 uid，多个值用逗号分隔；不要传 JSON 数组"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"block-id", "start", "end"}, Description: "block-id/start/end 必须一起提供，CLI 回读并校验 selectedText"}},
	Tips:        []string{`dws doc +comment-create --node DOC_ID --block-id BLOCK_ID --start 0 --end 10 --content "这里需要修改"`},
	Validate:    validateCommentCreate,
	Execute:     executeCommentCreate,
}

// ── 协作权限 ─────────────────────────────────────────────────

// ── 导出 ─────────────────────────────────────────────────────

var ExportSubmit = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+export-submit",
	Product:     productDoc,
	Description: "提交在线文档导出任务 (docx/markdown/pdf)，返回 jobId",
	Intent:      "仅当用户明确要求手工接管异步导出 job，且不需要当前命令下载文件时使用；返回 jobId 后只能用 +export-get 恢复。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_export_submit",
			CanonicalPath:  "doc.shortcut_export_submit",
			CLIPath:        "doc +export-submit",
			PrimaryCLIPath: "doc +export-submit",
		},
		Description: "提交在线文档导出任务 (docx/markdown/pdf)，返回 jobId",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "提交在线文档导出任务 (docx/markdown/pdf)，返回 jobId",
			UseWhen:      []string{"仅当用户明确要求手工接管异步导出 job，且不需要当前命令下载文件时使用；返回 jobId 后只能用 +export-get 恢复。"},
			AvoidWhen:    []string{"正常导出和保存本地文件使用 doc +export；不要手工编排 submit/get，也不要用它绕过 +export 的安全下载"},
			Examples:     []string{"dws doc +export-submit --node DOC_ID --export-format markdown"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "要导出的文档 ID 或 URL", Required: true},
		{Name: "export-format", Type: shortcut.FlagString, Desc: "导出格式；必须显式指定", Required: true, Enum: []string{"docx", "markdown", "pdf"}},
	},
	Tips: []string{`dws doc +export-submit --node DOC_ID --export-format markdown`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		result, err := rt.CallMCPWriteData(productDoc, "submit_export_job", map[string]any{
			"nodeId":       rt.Str("node"),
			"exportFormat": rt.Str("export-format"),
		})
		if err != nil {
			return docUnknownWriteError("doc.export_submit", "submit_export_job", rt.Str("node"), err)
		}
		return rt.Output(docEnvelope("doc.export_submit", map[string]any{"nodeId": rt.Str("node"), "result": result}, map[string]any{"name": "submit_export_job", "status": "success"}))
	},
}

var ExportGet = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+export-get",
	Product:     productDoc,
	Description: "根据 jobId 查询文档导出任务结果",
	Intent:      "当 +export 已返回 jobId 但轮询、中断或下载失败时使用；复用同一 job 查询，给 output 时通过 CLI 安全下载，禁止重新提交或 curl 临时链接。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_export_get",
			CanonicalPath:  "doc.shortcut_export_get",
			CLIPath:        "doc +export-get",
			PrimaryCLIPath: "doc +export-get",
		},
		Description: "根据 jobId 查询文档导出任务结果",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "根据 jobId 查询文档导出任务结果",
			UseWhen:      []string{"当 +export 已返回 jobId 但轮询、中断或下载失败时使用；复用同一 job 查询，给 output 时通过 CLI 安全下载，禁止重新提交或 curl 临时链接。"},
			AvoidWhen:    []string{"尚未提交导出或没有真实 jobId 时不要使用；正常首次导出使用 doc +export"},
			Examples:     []string{"dws doc +export-get --job-id JOB_ID", "dws doc +export-get --job-id JOB_ID --output ./exports/"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "job-id", Type: shortcut.FlagString, Desc: "导出任务 ID", Required: true},
		{Name: "output", Shorthand: "o", Type: shortcut.FlagString, Desc: "可选：任务完成后安全下载到工作目录内相对路径"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Str("output") == "" {
			return nil
		}
		return localio.ValidateOutput(rt.Str("output"))
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"output"}, Description: "提供 --output 时必须是工作目录内相对路径；默认 no-clobber"}},
	Tips:        []string{`dws doc +export-get --job-id JOB_ID`, `dws doc +export-get --job-id JOB_ID --output ./exports/`},
	Execute:     executeExportGet,
}

// ── 历史版本 (server: doc) ───────────────────────────────────

var VersionSave = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+version-save",
	Product:     productDoc,
	Description: "手动保存文档版本快照",
	Intent:      "当你在做重大改动前后、想手动打一个可回滚的版本存档点时使用；输入 node，会实际为该文档保存一个当前内容的历史版本快照。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_version_save",
			CanonicalPath:  "doc.shortcut_version_save",
			CLIPath:        "doc +version-save",
			PrimaryCLIPath: "doc +version-save",
		},
		Description: "手动保存文档版本快照",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "手动保存文档版本快照",
			UseWhen:      []string{"当你在做重大改动前后、想手动打一个可回滚的版本存档点时使用；输入 node，会实际为该文档保存一个当前内容的历史版本快照。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws doc +version-save --node DOC_ID"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
	},
	Tips: []string{`dws doc +version-save --node DOC_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("save_doc_version", map[string]any{"nodeId": rt.Str("node")})
	},
}

var VersionList = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+version-list",
	Product:     productDoc,
	Description: "查看文档历史版本列表",
	Intent:      "当你想查看某篇文档有哪些历史版本、以便挑一个版本号用于回滚时使用；输入 node，返回历史版本列表及其版本号。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_version_list",
			CanonicalPath:  "doc.shortcut_version_list",
			CLIPath:        "doc +version-list",
			PrimaryCLIPath: "doc +version-list",
		},
		Description: "查看文档历史版本列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查看文档历史版本列表",
			UseWhen:      []string{"当你想查看某篇文档有哪些历史版本、以便挑一个版本号用于回滚时使用；输入 node，返回历史版本列表及其版本号。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws doc +version-list --node DOC_ID"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "返回版本数量上限"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"},
	},
	Tips: []string{`dws doc +version-list --node DOC_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node")}
		if rt.Changed("limit") {
			params["maxResults"] = rt.Int("limit")
		}
		if v := rt.Str("cursor"); v != "" {
			params["nextCursor"] = v
		}
		return rt.CallMCP("list_doc_versions", params)
	},
}

var VersionRevert = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+version-revert",
	Product:     productDoc,
	Description: "回滚文档到指定历史版本",
	Intent:      "当文档被误改、你想把它整体恢复到某个历史版本时使用；先用 +version-list 找到目标版本号，再输入 node 与 version，会实际把文档内容覆盖回该版本，属于高风险写操作，需谨慎确认。",
	Risk:        shortcut.RiskHighWrite,
	Safety: contract.SafetySpec{
		Effect: "destructive", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_version_revert",
			CanonicalPath:  "doc.shortcut_version_revert",
			CLIPath:        "doc +version-revert",
			PrimaryCLIPath: "doc +version-revert",
		},
		Description: "回滚文档到指定历史版本",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "回滚文档到指定历史版本",
			UseWhen:      []string{"当文档被误改、你想把它整体恢复到某个历史版本时使用；先用 +version-list 找到目标版本号，再输入 node 与 version，会实际把文档内容覆盖回该版本，属于高风险写操作，需谨慎确认。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws doc +version-revert --node DOC_ID --version 3"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "version", Type: shortcut.FlagInt, Desc: "目标版本号 (从 +version-list 获取)", Required: true},
	},
	Tips: []string{`dws doc +version-revert --node DOC_ID --version 3 --yes`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("revert_doc_version", map[string]any{
			"nodeId":  rt.Str("node"),
			"version": rt.Int("version"),
		})
	},
}

// ── 模板 (server: doc) ───────────────────────────────────────

var TemplateList = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+template-list",
	Product:     productDoc,
	Description: "获取文档模板列表",
	Intent:      "当你想基于模板新建文档、需要先浏览可用的模板（自己的 MY 或公共 PUBLIC）并拿到 templateId 时使用；返回模板列表，随后可配合 +template-apply 套用。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_template_list",
			CanonicalPath:  "doc.shortcut_template_list",
			CLIPath:        "doc +template-list",
			PrimaryCLIPath: "doc +template-list",
		},
		Description: "获取文档模板列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "获取文档模板列表",
			UseWhen:      []string{"当你想基于模板新建文档、需要先浏览可用的模板（自己的 MY 或公共 PUBLIC）并拿到 templateId 时使用；返回模板列表，随后可配合 +template-apply 套用。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws doc +template-list --source PUBLIC"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "source", Type: shortcut.FlagString, Desc: "模板来源: MY / PUBLIC (默认 MY)", Enum: []string{"MY", "PUBLIC"}},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "返回数量上限"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"},
	},
	Tips: []string{`dws doc +template-list --source PUBLIC`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if v := rt.Str("source"); v != "" {
			params["templateSource"] = v
		}
		if rt.Changed("limit") {
			params["maxResults"] = rt.Int("limit")
		}
		if v := rt.Str("cursor"); v != "" {
			params["nextCursor"] = v
		}
		return rt.CallMCP("list_doc_templates", params)
	},
}

var TemplateSearch = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+template-search",
	Product:     productDoc,
	Description: "根据关键词搜索文档模板",
	Intent:      "当模板较多、你想按关键词（如“周报”“合同”）快速找到合适的模板并拿到 templateId 时使用；输入 query，返回匹配的模板列表，随后可配合 +template-apply 套用。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           "shortcut_template_search",
			CanonicalPath:  "doc.shortcut_template_search",
			CLIPath:        "doc +template-search",
			PrimaryCLIPath: "doc +template-search",
		},
		Description: "根据关键词搜索文档模板",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "根据关键词搜索文档模板",
			UseWhen:      []string{"当模板较多、你想按关键词（如“周报”“合同”）快速找到合适的模板并拿到 templateId 时使用；输入 query，返回匹配的模板列表，随后可配合 +template-apply 套用。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws doc +template-search --query \"周报\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词", Required: true},
		{Name: "source", Type: shortcut.FlagString, Desc: "模板来源: MY / PUBLIC (默认 MY)", Enum: []string{"MY", "PUBLIC"}},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "返回数量上限"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"},
	},
	Tips: []string{`dws doc +template-search --query "周报"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		pageSize := rt.Int("limit")
		if pageSize <= 0 {
			pageSize = 50
		}
		params := map[string]any{"searchName": rt.Str("query"), "maxResults": pageSize}
		if v := rt.Str("source"); v != "" {
			params["templateSource"] = v
		}
		if v := rt.Str("cursor"); v != "" {
			params["nextCursor"] = v
		}
		found, err := rt.CallMCPData(productDoc, "search_doc_templates", params)
		if err != nil {
			return err
		}
		candidates := collectTemplateCandidates(found)
		hasMore, hasMoreKnown, nextCursor := docPageState(found)
		nextCursor = strings.TrimSpace(nextCursor)
		complete := hasMoreKnown && !hasMore
		if !hasMoreKnown && nextCursor == "" && len(candidates) < pageSize {
			complete = true
		}
		globalComplete := complete && rt.Str("cursor") == ""
		status := "selection_required"
		if globalComplete && len(candidates) == 0 {
			status = "not_found"
		} else if globalComplete && len(candidates) == 1 {
			status = "resolved"
		}
		selectedTemplateID := ""
		if status == "resolved" {
			selectedTemplateID = candidates[0]["templateId"].(string)
		}
		nextAction := map[string]string{"resolved": "create_once", "not_found": "stop", "selection_required": "ask_user"}[status]
		if !complete {
			nextAction = "continue_search"
		}
		return rt.Output(docEnvelope("doc.template_search", map[string]any{
			"query":      rt.Str("query"),
			"source":     rt.Str("source"),
			"count":      len(candidates),
			"candidates": candidates,
			"complete":   complete,
			"hasMore":    !complete,
			"nextCursor": nextCursor,
			"selection": map[string]any{
				"status":     status,
				"templateId": selectedTemplateID,
				"nextAction": nextAction,
			},
		}))
	},
}

var TemplateApply = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+template-apply",
	Product:     productDoc,
	Description: "使用指定模板创建新文档",
	Intent:      "当你已选定某个模板、想据此快速生成一篇带预设结构的新文档时使用；输入 template-id（可选 name/folder/workspace），会实际按模板创建一篇新文档并返回其 ID。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "template-id", Type: shortcut.FlagString, Desc: "模板 ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新文档名称 (可选)"},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 ID (可选)"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "知识库 ID (可选)"},
	},
	Tips: []string{`dws doc +template-apply --template-id TPL_ID --name "我的周报"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"templateId": rt.Str("template-id")}
		if v := rt.Str("name"); v != "" {
			params["name"] = v
		}
		if rt.Changed("folder") {
			params["folderId"] = rt.Str("folder")
		}
		if rt.Changed("workspace") {
			params["workspaceId"] = rt.Str("workspace")
		}
		return rt.CallMCP("apply_doc_template", params)
	},
}

func init() {
	// Expert/recovery leaves remain callable without entering Agent discovery.
	CommentCreateInline.Contract = corecmd.ContractDecl{}
	TemplateApply.Contract = corecmd.ContractDecl{}
	canonicalizeHistoryShortcuts()
	shortcut.Register(
		Search,
		List,
		Copy,
		Move,
		CommentList,
		CommentCreate,
		CommentReply,
		CommentCreateInline,
		ExportSubmit,
		ExportGet,
		compatHistorySave,
		compatHistoryList,
		compatHistoryRevert,
		VersionSave,
		VersionList,
		VersionRevert,
		TemplateList,
		TemplateSearch,
		TemplateApply,
	)
}
