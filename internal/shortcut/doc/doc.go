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
		if rt.Changed("limit") {
			params["pageSize"] = rt.Int("limit")
		}
		if rt.Changed("cursor") {
			params["pageToken"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData(productDoc, "search_documents", params)
		if err != nil {
			return err
		}
		docs := searchDocsProject(data)
		return rt.Output(map[string]any{"count": len(docs), "documents": docs})
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
	Flags: []shortcut.Flag{
		{Name: "folder", Type: shortcut.FlagString, Desc: "文档文件夹 nodeId 或 alidocs 文件夹 URL"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "知识库 ID"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量 (默认 50，最大 50)"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标 (上次结果的 nextPageToken)"},
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
		if rt.Changed("limit") {
			params["pageSize"] = rt.Int("limit")
		}
		if rt.Changed("cursor") {
			params["pageToken"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData(productDoc, "list_nodes", params)
		if err != nil {
			return err
		}
		nodes := listNodesProject(data)
		return rt.Output(map[string]any{"count": len(nodes), "nodes": nodes})
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
	Intent:      "当你想查看某篇文档上已有的评论、了解有哪些反馈或待处理意见（可按全文/划词、已解决/未解决过滤）时使用；输入 node，返回评论列表及其 commentKey 以便后续回复。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量 (默认 50，最大 50)"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标 (上一页返回的 nextToken)"},
		{Name: "type", Type: shortcut.FlagString, Desc: "评论类型: global (全文) / inline (划词)", Enum: []string{"global", "inline"}},
		{Name: "resolve-status", Type: shortcut.FlagString, Desc: "解决状态: resolved / unresolved", Enum: []string{"resolved", "unresolved"}},
	},
	Tips: []string{`dws doc +comment-list --node DOC_ID`, `dws doc +comment-list --node DOC_ID --type inline --resolve-status unresolved`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node")}
		if rt.Changed("limit") {
			params["pageSize"] = rt.Int("limit")
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

var CommentCreate = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+comment-create",
	Product:     productComment,
	Description: "在文档上创建一条评论",
	Intent:      "当你想对整篇文档留一条全文评论、给出反馈或 @ 相关同事时使用；输入 node 与评论 content（可带 mention），会实际在文档上发布一条新评论。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "评论文字内容 (纯文本)", Required: true},
		{Name: "mention", Type: shortcut.FlagStringSlice, Desc: "被 @ 的用户 uid 列表"},
	},
	Tips: []string{`dws doc +comment-create --node DOC_ID --content "这里需要修改"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"nodeId":  rt.Str("node"),
			"content": rt.Str("content"),
		}
		if rt.Changed("mention") {
			params["mentionedUserIds"] = rt.StrSlice("mention")
		}
		return rt.CallMCP("create_comment", params)
	},
}

var CommentReply = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+comment-reply",
	Product:     productComment,
	Description: "回复文档中的一条评论",
	Intent:      "当你要针对某条已有评论进行回复、参与讨论或用表情贴图回应时使用；先从评论列表拿到 comment-key，再输入 node、comment-key 与 content（--emoji 则作为表情回复），会实际发布一条回复。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "回复文字内容 (表情回复时填表情名称)", Required: true},
		{Name: "comment-key", Type: shortcut.FlagString, Desc: "被回复评论的 commentKey (从 list/create 获取)", Required: true},
		{Name: "emoji", Type: shortcut.FlagBool, Desc: "作为表情贴图回复"},
		{Name: "mention", Type: shortcut.FlagStringSlice, Desc: "被 @ 的用户 uid 列表"},
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
			params["mentionedUserIds"] = rt.StrSlice("mention")
		}
		return rt.CallMCP("reply_comment", params)
	},
}

var CommentCreateInline = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+comment-create-inline",
	Product:     productComment,
	Description: "在文档选中文本区域上创建划词评论",
	Intent:      "当你想针对文档里某段具体文字（而非整篇）留评论、做精确批注时使用；需先用 +block-list 定位块，再输入 node、block-id 及该块内的 start/end 字符偏移量，会实际在选中文本上创建划词评论。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "评论文字内容 (纯文本)", Required: true},
		{Name: "block-id", Type: shortcut.FlagString, Desc: "评论标记所在的块 ID (通过 +block-list 获取)", Required: true},
		{Name: "start", Type: shortcut.FlagInt, Desc: "块内文本起始字符偏移量 (从 0 开始)", Required: true},
		{Name: "end", Type: shortcut.FlagInt, Desc: "块内文本结束字符偏移量 (须大于 start)", Required: true},
		{Name: "selected-text", Type: shortcut.FlagString, Desc: "选中文本内容 (展示引用原文)"},
		{Name: "mention", Type: shortcut.FlagStringSlice, Desc: "被 @ 的用户 uid 列表"},
	},
	Tips: []string{`dws doc +comment-create-inline --node DOC_ID --block-id BLOCK_ID --start 0 --end 10 --content "这里需要修改"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"nodeId":  rt.Str("node"),
			"content": rt.Str("content"),
			"blockId": rt.Str("block-id"),
			"start":   rt.Int("start"),
			"end":     rt.Int("end"),
		}
		if v := rt.Str("selected-text"); v != "" {
			params["selectedText"] = v
		}
		if rt.Changed("mention") {
			params["mentionedUserIds"] = rt.StrSlice("mention")
		}
		return rt.CallMCP("create_inline_comment", params)
	},
}

// ── 协作权限 ─────────────────────────────────────────────────

// ── 导出 ─────────────────────────────────────────────────────

var ExportSubmit = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+export-submit",
	Product:     productDoc,
	Description: "提交在线文档导出任务 (docx/markdown/pdf)，返回 jobId",
	Intent:      "当你想把在线文档导出成 docx/markdown/pdf 文件（例如离线保存或外发）时使用；这是异步任务的第一步，输入 node 与 export-format 提交导出，返回 jobId，随后用 +export-get 轮询结果。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "要导出的文档 ID 或 URL", Required: true},
		{Name: "export-format", Type: shortcut.FlagString, Default: "docx", Desc: "导出格式", Enum: []string{"docx", "markdown", "pdf"}},
	},
	Tips: []string{`dws doc +export-submit --node DOC_ID --export-format markdown`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		format := rt.Str("export-format")
		if format == "" {
			format = "docx"
		}
		return rt.CallMCP("submit_export_job", map[string]any{
			"nodeId":       rt.Str("node"),
			"exportFormat": format,
		})
	},
}

var ExportGet = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+export-get",
	Product:     productDoc,
	Description: "根据 jobId 查询文档导出任务结果",
	Intent:      "当你已用 +export-submit 提交了导出任务、想查询它是否完成并拿到导出文件的下载链接时使用；输入上一步返回的 job-id，返回任务状态与结果。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "job-id", Type: shortcut.FlagString, Desc: "导出任务 ID", Required: true},
	},
	Tips: []string{`dws doc +export-get --job-id JOB_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("query_export_job", map[string]any{"jobId": rt.Str("job-id")})
	},
}

// ── 历史版本 (server: doc) ───────────────────────────────────

var VersionSave = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+version-save",
	Product:     productDoc,
	Description: "手动保存文档版本快照",
	Intent:      "当你在做重大改动前后、想手动打一个可回滚的版本存档点时使用；输入 node，会实际为该文档保存一个当前内容的历史版本快照。",
	Risk:        shortcut.RiskWrite,
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
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词", Required: true},
		{Name: "source", Type: shortcut.FlagString, Desc: "模板来源: MY / PUBLIC (默认 MY)", Enum: []string{"MY", "PUBLIC"}},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "返回数量上限"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"},
	},
	Tips: []string{`dws doc +template-search --query "周报"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"searchName": rt.Str("query")}
		if v := rt.Str("source"); v != "" {
			params["templateSource"] = v
		}
		if rt.Changed("limit") {
			params["maxResults"] = rt.Int("limit")
		}
		if v := rt.Str("cursor"); v != "" {
			params["nextCursor"] = v
		}
		return rt.CallMCP("search_doc_templates", params)
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
		VersionSave,
		VersionList,
		VersionRevert,
		TemplateList,
		TemplateSearch,
		TemplateApply,
	)
}
