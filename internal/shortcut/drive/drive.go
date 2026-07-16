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

// Package drive declares high-fidelity shortcuts for the DingTalk drive (钉盘)
// service: file/folder listing, metadata, download link, folder creation,
// upload credentials, search, recycle bin, internet publish and the document
// space proxy operations (copy/move/rename/permission/recent). Tool names and
// parameters mirror internal/helpers/drive.go exactly. Tools that live on the
// "doc" MCP server (per callMCPToolOnServer("doc", ...)) set Product: "doc".
package drive

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"

// ── 钉盘文件（drive MCP server）────────────────────────────────

// List → list_files
var List = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+list",
	Product:     "drive",
	Description: "列出钉盘文件/文件夹",
	Intent:      "当你想浏览钉盘某个空间或文件夹下有哪些文件和子文件夹、需要拿到文件的 dentryUuid 以便后续下载/移动/删除时使用；可指定 space-id、folder 逐层进入，支持分页和按创建/修改时间、名称排序，返回文件列表（含 ID、名称、类型等）。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "space-id", Type: shortcut.FlagString, Desc: "钉盘空间 ID (纯数字)，不传则使用「我的文件」"},
		{Name: "folder", Type: shortcut.FlagString, Desc: "父节点 ID (dentryUuid)，不传则列出空间根目录"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页返回数量 (默认 20，最大 50)"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，首次不传"},
		{Name: "order-by", Type: shortcut.FlagString, Desc: "排序字段: createTime|modifyTime|name"},
		{Name: "order", Type: shortcut.FlagString, Desc: "排序方向: asc|desc (默认 desc)"},
		{Name: "thumbnail", Type: shortcut.FlagBool, Desc: "是否返回缩略图信息"},
	},
	Tips: []string{
		`dws drive +list --limit 20`,
		`dws drive +list --folder <dentryUuid> --order-by name --order asc`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"maxResults": rt.Int("limit")}
		if rt.Changed("space-id") {
			params["spaceId"] = rt.Str("space-id")
		}
		if rt.Changed("folder") {
			params["parentId"] = rt.Str("folder")
		}
		if rt.Changed("cursor") {
			params["nextToken"] = rt.Str("cursor")
		}
		if rt.Changed("order-by") {
			params["orderBy"] = rt.Str("order-by")
		}
		if rt.Changed("order") {
			params["order"] = rt.Str("order")
		}
		if rt.Bool("thumbnail") {
			params["withThumbnail"] = true
		}
		data, err := rt.CallMCPData("drive", "list_files", params)
		if err != nil {
			return err
		}
		files := listFilesProject(data)
		return rt.Output(map[string]any{"count": len(files), "files": files})
	},
}

// listFilesProject reshapes the raw list_files response into a clean,
// stable 钉盘 file/folder list ({name,type,dentryId,fileSize}) — the
// output-projection fidelity applied to every list shortcut. Both the list
// container and each field are probed defensively across candidate keys, so a
// missing container or unknown alias simply yields an empty list rather than a
// fabricated value.
func listFilesProject(data map[string]any) []map[string]any {
	raw := listFilesContainer(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		listFilesPick(row, m, "name", "name", "fileName", "dentryName", "title")
		listFilesPick(row, m, "type", "type", "dentryType", "fileType", "spaceType")
		listFilesPick(row, m, "dentryId", "dentryId", "dentryUuid", "id", "fileId", "nodeId")
		listFilesPick(row, m, "fileSize", "fileSize", "size", "byteSize", "length")
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// listFilesContainer locates the file list array inside the response by trying
// the common container keys emitted across drive backends; the payload itself
// may also already be the array.
func listFilesContainer(data map[string]any) []any {
	for _, k := range []string{"result", "data", "list", "items", "files", "dentries", "entries", "nodes"} {
		if v, ok := data[k].([]any); ok {
			return v
		}
		// The container may be nested one level (e.g. {"data":{"list":[...]}}).
		if inner, ok := data[k].(map[string]any); ok {
			for _, ik := range []string{"list", "items", "files", "dentries", "entries", "nodes", "result"} {
				if v, ok := inner[ik].([]any); ok {
					return v
				}
			}
		}
	}
	return nil
}

// listFilesPick copies the first matching alias from src into dst under the
// canonical key, leaving dst untouched when no alias is present.
func listFilesPick(dst, src map[string]any, canonical string, aliases ...string) {
	for _, a := range aliases {
		if v, ok := src[a]; ok {
			dst[canonical] = v
			return
		}
	}
}

// Info → get_file_info
var Info = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+info",
	Product:     "drive",
	Description: "获取钉盘文件/文件夹元数据",
	Intent:      "当你已经知道某个节点的 dentryUuid、想查看它的详细信息（名称、大小、类型、创建/修改时间、所属空间等）而不是仅列表概览时使用；输入 node（节点 ID），返回该单个文件或文件夹的完整元数据。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "节点 ID (dentryUuid)", Required: true},
		{Name: "space-id", Type: shortcut.FlagString, Desc: "节点所属空间 ID"},
	},
	Tips: []string{`dws drive +info --node <dentryUuid>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"fileId": rt.Str("node")}
		if rt.Changed("space-id") {
			params["spaceId"] = rt.Str("space-id")
		}
		return rt.CallMCP("get_file_info", params)
	},
}

// Download → download_file (返回下载链接与签名请求头)
var Download = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+download",
	Product:     "drive",
	Description: "获取钉盘文件下载链接",
	Intent:      "当你需要把钉盘里某个文件下载到本地或转给他人时使用；输入文件的 dentryUuid，返回带签名的临时下载 URL 和请求头，用它去真正拉取文件内容（本命令本身只取链接、不落盘）。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文件 ID (dentryUuid)", Required: true},
		{Name: "space-id", Type: shortcut.FlagString, Desc: "文件所属空间 ID"},
	},
	Tips: []string{`dws drive +download --node <dentryUuid>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"fileId": rt.Str("node")}
		if rt.Changed("space-id") {
			params["spaceId"] = rt.Str("space-id")
		}
		return rt.CallMCP("download_file", params)
	},
}

// CreateFolder → create_folder
// UploadInfo → get_upload_info (获取 OSS 上传凭证)
// Commit → commit_upload (OSS 上传完成后提交入库)
// ListSpaces → list_spaces
// Search → search_files
var Search = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+search",
	Product:     "drive",
	Description: "搜索钉盘文件",
	Intent:      "当你只记得文件名或内容关键词、不知道它在哪个目录时用它全局检索钉盘文件；输入 query，可按文件类型、扩展名、创建者、创建/修改时间范围过滤，返回匹配文件及其 ID，便于再做下载或整理。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词", Required: true},
		{Name: "target", Type: shortcut.FlagString, Desc: "搜索范围: file(钉盘文件) / space(钉盘团队空间)", Enum: []string{"file", "space"}},
		{Name: "file-types", Type: shortcut.FlagStringSlice, Desc: "按文件内容类型过滤: alidoc,document,image,video,audio,archive"},
		{Name: "extensions", Type: shortcut.FlagStringSlice, Desc: "按文件扩展名过滤，不含点号 (如 pdf,docx)"},
		{Name: "creator-uids", Type: shortcut.FlagStringSlice, Desc: "按创建者用户 ID 过滤"},
		{Name: "created-from", Type: shortcut.FlagInt, Desc: "创建时间起始 (毫秒时间戳，含)"},
		{Name: "created-to", Type: shortcut.FlagInt, Desc: "创建时间截止 (毫秒时间戳，含)"},
		{Name: "modified-from", Type: shortcut.FlagInt, Desc: "修改时间起始 (毫秒时间戳，含)"},
		{Name: "modified-to", Type: shortcut.FlagInt, Desc: "修改时间截止 (毫秒时间戳，含)"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页返回数量 (默认 10，最大 30)"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，从上次返回的 nextCursor 获取"},
	},
	Tips: []string{
		`dws drive +search --query "季度汇报"`,
		`dws drive +search --query "合同" --target file --extensions pdf,docx`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"keyword": rt.Str("query")}
		if rt.Changed("target") {
			params["searchTarget"] = rt.Str("target")
		}
		if rt.Changed("file-types") {
			params["fileTypes"] = rt.StrSlice("file-types")
		}
		if rt.Changed("extensions") {
			params["extensions"] = rt.StrSlice("extensions")
		}
		if rt.Changed("creator-uids") {
			params["creatorUserIds"] = rt.StrSlice("creator-uids")
		}
		if rt.Changed("created-from") {
			params["createdTimeFrom"] = rt.Int("created-from")
		}
		if rt.Changed("created-to") {
			params["createdTimeTo"] = rt.Int("created-to")
		}
		if rt.Changed("modified-from") {
			params["modifiedTimeFrom"] = rt.Int("modified-from")
		}
		if rt.Changed("modified-to") {
			params["modifiedTimeTo"] = rt.Int("modified-to")
		}
		if rt.Changed("limit") {
			params["pageSize"] = rt.Int("limit")
		}
		if rt.Changed("cursor") {
			params["pageToken"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("drive", "search_files", params)
		if err != nil {
			return err
		}
		files := searchFilesProject(data)
		return rt.Output(map[string]any{"count": len(files), "files": files})
	},
}

// searchFilesProject reshapes the raw search_files response into a clean,
// stable 钉盘 file list ({name,type,dentryId,fileSize,creatorId}) — the
// output-projection fidelity applied to every list/search shortcut. Both the
// list container and each field are probed defensively across candidate keys,
// so a missing container or unknown alias yields an empty list rather than a
// fabricated value.
func searchFilesProject(data map[string]any) []map[string]any {
	raw := searchFilesContainer(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		searchFilesPick(row, m, "name", "name", "fileName", "dentryName", "title")
		searchFilesPick(row, m, "type", "type", "dentryType", "fileType", "spaceType")
		searchFilesPick(row, m, "dentryId", "dentryId", "dentryUuid", "id", "fileId", "nodeId")
		searchFilesPick(row, m, "fileSize", "fileSize", "size", "byteSize", "length")
		searchFilesPick(row, m, "creatorId", "creatorId", "creatorUserId", "creator", "creatorUid")
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// searchFilesContainer locates the file list array inside the response by
// trying the common container keys emitted across drive backends; the payload
// itself may also already wrap the array one level deeper.
func searchFilesContainer(data map[string]any) []any {
	for _, k := range []string{"result", "data", "list", "items", "files", "dentries", "entries", "nodes"} {
		if v, ok := data[k].([]any); ok {
			return v
		}
		if inner, ok := data[k].(map[string]any); ok {
			for _, ik := range []string{"list", "items", "files", "dentries", "entries", "nodes", "result"} {
				if v, ok := inner[ik].([]any); ok {
					return v
				}
			}
		}
	}
	return nil
}

// searchFilesPick copies the first matching alias from src into dst under the
// canonical key, leaving dst untouched when no alias is present.
func searchFilesPick(dst, src map[string]any, canonical string, aliases ...string) {
	for _, a := range aliases {
		if v, ok := src[a]; ok {
			dst[canonical] = v
			return
		}
	}
}

// RecycleList → list_recycle_items
// RecycleRestore → restore_recycle_item
// PublishSet → set_file_publish (published=true)
// PublishUnset → set_file_publish (published=false)
// PublishStatus → get_file_publish_status
// ── 文档空间代理（doc MCP server）─────────────────────────────

// ListDocs → list_nodes (doc)
// SearchDocs → search_documents (doc)
var SearchDocs = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+search-docs",
	Product:     "doc",
	Description: "搜索文档空间文档",
	Intent:      "当你只记得文档标题或关键词、想在文档空间/知识库中检索在线文档（区别于 +search 检索钉盘文件）时使用；输入 query 关键词，返回匹配的文档及其节点信息。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量"},
	},
	Tips: []string{`dws drive +search-docs --query "季度汇报"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"keyword": rt.Str("query")}
		if rt.Changed("limit") {
			params["pageSize"] = rt.Int("limit")
		}
		data, err := rt.CallMCPData("doc", "search_documents", params)
		if err != nil {
			return err
		}
		docs := searchDocsProject(data)
		return rt.Output(map[string]any{"count": len(docs), "docs": docs})
	},
}

// searchDocsProject reshapes the raw search_documents response into a clean,
// stable document list ({name,nodeId,type,url}) — the output-projection
// fidelity applied to every list/search shortcut. Both the list container and
// each field are probed defensively across candidate keys, so a missing
// container or unknown alias yields an empty list rather than fabricated data.
func searchDocsProject(data map[string]any) []map[string]any {
	raw := searchDocsContainer(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		searchDocsPick(row, m, "name", "name", "title", "docName", "nodeName", "fileName")
		searchDocsPick(row, m, "nodeId", "nodeId", "id", "docId", "dentryUuid", "fileId")
		searchDocsPick(row, m, "type", "type", "docType", "nodeType", "fileType")
		searchDocsPick(row, m, "url", "url", "docUrl", "link", "webUrl")
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// searchDocsContainer locates the document list array inside the response by
// trying the common container keys; the payload may also wrap the array one
// level deeper under a common envelope.
func searchDocsContainer(data map[string]any) []any {
	for _, k := range []string{"result", "data", "list", "items", "documents", "docs", "nodes"} {
		if v, ok := data[k].([]any); ok {
			return v
		}
		if inner, ok := data[k].(map[string]any); ok {
			for _, ik := range []string{"list", "items", "documents", "docs", "nodes", "result"} {
				if v, ok := inner[ik].([]any); ok {
					return v
				}
			}
		}
	}
	return nil
}

// searchDocsPick copies the first matching alias from src into dst under the
// canonical key, leaving dst untouched when no alias is present.
func searchDocsPick(dst, src map[string]any, canonical string, aliases ...string) {
	for _, a := range aliases {
		if v, ok := src[a]; ok {
			dst[canonical] = v
			return
		}
	}
}

// Delete → delete_document (doc)
// Copy → copy_document (doc)
var Copy = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+copy",
	Product:     "doc",
	Description: "复制文件/文档到指定位置",
	Intent:      "当你想保留原件、把某个文件/文档拷贝一份到指定文件夹或知识库时使用；输入源节点 node 及目标 folder/workspace，会实际生成一个副本，原文件位置不变。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档/文件 ID", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 nodeId"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "目标知识库 ID"},
	},
	Tips: []string{`dws drive +copy --node <nodeId> --folder <targetFolderId>`},
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

// Move → move_document (doc)
var Move = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+move",
	Product:     "doc",
	Description: "移动文件/文档到指定位置",
	Intent:      "当你要把某个文件/文档从当前位置转移到另一个文件夹或知识库（整理归档、调整目录结构）时使用；输入源节点 node 及目标 folder/workspace，会实际改变文件所在位置，原位置不再保留该文件。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档/文件 ID", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 nodeId"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "目标知识库 ID"},
	},
	Tips: []string{`dws drive +move --node <nodeId> --folder <targetFolderId>`},
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

// Rename → rename_document (doc)
// PermissionAdd → add_permission (doc)
// PermissionUpdate → update_permission (doc)
// PermissionList → list_permission (doc)
// PermissionRemove → remove_permission (doc)
// Recent → get_recent_list (doc)
var Recent = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+recent",
	Product:     "doc",
	Description: "获取最近访问/编辑的文档列表",
	Intent:      "当你想快速找回「我最近看过/改过的那个文档」而不记得它放在哪时使用；可按操作类型（最近访问/最近编辑）和创建人（全部/我创建/他人创建）过滤，返回近期文档列表及其节点信息。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "operate-type", Type: shortcut.FlagInt, Desc: "操作类型: 0=最近访问(默认), 1=最近编辑"},
		{Name: "creator-type", Type: shortcut.FlagInt, Desc: "创建人过滤: 0=全部, 1=我创建, 2=他人创建"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量 (默认 20，最大 20)"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标 (从上次结果的 nextCursor 获取)"},
	},
	Tips: []string{
		`dws drive +recent`,
		`dws drive +recent --operate-type 1 --creator-type 1`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Changed("operate-type") {
			params["operateTypes"] = []int{rt.Int("operate-type")}
		}
		if rt.Changed("creator-type") {
			params["creatorType"] = rt.Int("creator-type")
		}
		if rt.Changed("limit") {
			params["maxResults"] = rt.Int("limit")
		}
		if rt.Changed("cursor") {
			params["nextToken"] = rt.Str("cursor")
		}
		// Project the verbose raw response (logId + per-item giant docUrl noise)
		// down to a clean {count, items:[…], nextCursor, hasMore}.
		data, err := rt.CallMCPData("doc", "get_recent_list", params)
		if err != nil {
			return err
		}
		return rt.Output(recentListProject(data))
	},
}

// recentListProject reshapes a get_recent_list response into a clean paginated
// document list, dropping transport noise (logId) while keeping the pagination
// cursor.
func recentListProject(data map[string]any) map[string]any {
	items := []map[string]any{}
	raw, _ := data["recentItems"].([]any)
	for _, it := range raw {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, map[string]any{
			"name":        m["name"],
			"nodeType":    m["nodeType"],
			"contentType": m["contentType"],
			"accessTime":  m["accessTime"],
			"docUrl":      m["docUrl"],
			"nodeId":      m["nodeId"],
		})
	}
	out := map[string]any{"count": len(items), "items": items}
	if nc, ok := data["nextCursor"]; ok && nc != nil {
		out["nextCursor"] = nc
	}
	if hm, ok := data["hasMore"]; ok {
		out["hasMore"] = hm
	}
	return out
}

func init() {
	shortcut.Register(
		List,
		Info,
		Download,
		Search,
		SearchDocs,
		Copy,
		Move,
		Recent,
	)
}
