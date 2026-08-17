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

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var driveDownload = localio.Download

// ── 钉盘文件（drive MCP server）────────────────────────────────

// List → list_files
var List = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+list",
	Product:     "drive",
	Description: "严格分页列出钉盘文件和文件夹",
	Intent:      "浏览钉盘根目录或已知文件夹时使用；服务端明确空数组才表示空目录，缺字段、坏元素或空响应都会失败。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: driveContract(
		"+list", "严格分页列出钉盘文件和文件夹",
		"浏览钉盘根目录或已知文件夹时使用；服务端明确空数组才表示空目录，缺字段、坏元素或空响应都会失败。",
		[]string{"按关键词定位文件改用 drive +search；查看单个节点详情改用 drive +inspect"},
		[]string{`dws drive +list --limit 20`, `dws drive +list --folder <dentryUuid> --limit 20`},
		driveCollectionResult("files", "严格校验并投影的钉盘目录页"), driveCursorPagination(),
		contract.ParamDecl{Name: "space-id", Property: "spaceId"},
		contract.ParamDecl{Name: "folder", Property: "parentId"},
		contract.ParamDecl{Name: "cursor", Property: "nextToken"},
	),
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
		items, page, err := requireDriveCollection(data, "drive/list_files", "items", "files", "dentries", "entries", "nodes", "list")
		if err != nil {
			return err
		}
		files := projectDriveRows(items, map[string][]string{
			"name":     {"name", "fileName", "dentryName", "title"},
			"type":     {"type", "dentryType", "fileType", "spaceType"},
			"nodeId":   {"fileId", "dentryUuid", "nodeId", "id"},
			"dentryId": {"dentryId"},
			"fileSize": {"fileSize", "size", "byteSize", "length"},
		})
		out := map[string]any{"count": len(files), "files": files}
		addDrivePagination(out, page)
		return rt.Output(out)
	},
}

// Info → get_file_info
var Info = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+info",
	Product:     "drive",
	Description: "获取钉盘文件/文件夹元数据",
	Intent:      "当你已经知道某个节点的 dentryUuid、想查看它的详细信息（名称、大小、类型、创建/修改时间、所属空间等）而不是仅列表概览时使用；输入 node（节点 ID），返回该单个文件或文件夹的完整元数据。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "drive",
			Name:           "shortcut_info",
			CanonicalPath:  "drive.shortcut_info",
			CLIPath:        "drive +info",
			PrimaryCLIPath: "drive +info",
		},
		Description: "获取钉盘文件/文件夹元数据",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "获取钉盘文件/文件夹元数据",
			UseWhen:      []string{"当你已经知道某个节点的 dentryUuid、想查看它的详细信息（名称、大小、类型、创建/修改时间、所属空间等）而不是仅列表概览时使用；输入 node（节点 ID），返回该单个文件或文件夹的完整元数据。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws drive +info --node <dentryUuid>"},
		},
	},
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
		data, err := rt.CallMCPData("drive", "get_file_info", params)
		if err != nil {
			return err
		}
		result, err := requireDriveObject(data, "drive/get_file_info")
		if err != nil {
			return err
		}
		return rt.Output(result)
	},
}

// Download → download_file (返回下载链接与签名请求头)
var Download = shortcut.Shortcut{
	Service:     "drive",
	Command:     "+download",
	Product:     "drive",
	Description: "安全下载钉盘文件到工作目录",
	Intent:      "下载普通钉盘文件并要求验证本地字节产物时使用；不是只返回临时 URL。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: driveContract(
		"+download", "安全下载钉盘文件到工作目录",
		"下载普通钉盘文件并要求验证本地字节产物时使用；不是只返回临时 URL。",
		[]string{"在线文档导出为 docx/pdf 使用 doc +export；只查元数据使用 drive +inspect"},
		[]string{`dws drive +download --node <dentryUuid> --output downloads/report.pdf`},
		driveObjectResult("已验证的本地下载产物"), nil,
		contract.ParamDecl{Name: "node", Property: "fileId"},
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文件 ID (dentryUuid)", Required: true},
		{Name: "space-id", Type: shortcut.FlagString, Desc: "文件所属空间 ID"},
		{Name: "output", Shorthand: "o", Type: shortcut.FlagString, Desc: "工作目录内的相对输出路径", Required: true},
	},
	Tips: []string{`dws drive +download --node <dentryUuid> --output downloads/report.pdf`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"fileId": rt.Str("node")}
		if rt.Changed("space-id") {
			params["spaceId"] = rt.Str("space-id")
		}
		data, err := rt.CallMCPData("drive", "download_file", params)
		if err != nil {
			return err
		}
		payload, err := requireDriveObject(data, "drive/download_file")
		if err != nil {
			return err
		}
		url, preferredName, headers, err := driveDownloadPayload(payload, "drive/download_file")
		if err != nil {
			return err
		}
		cwd, err := driveGetwd()
		if err != nil {
			return err
		}
		artifact, err := driveDownload(rt.Command().Context(), url, localio.DownloadOptions{
			BaseDir: cwd, Output: rt.Str("output"), PreferredName: preferredName, Headers: headers,
		})
		if err != nil {
			return err
		}
		if artifact.SizeBytes <= 0 {
			return driveResponseError("drive/download_file", "empty_download_artifact", "下载完成但本地产物为 0 字节")
		}
		return rt.Output(map[string]any{"success": true, "nodeId": rt.Str("node"), "savedPath": artifact.RelativePath, "sizeBytes": artifact.SizeBytes})
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
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "drive",
			Name:           "shortcut_search",
			CanonicalPath:  "drive.shortcut_search",
			CLIPath:        "drive +search",
			PrimaryCLIPath: "drive +search",
		},
		Description: "搜索钉盘文件",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "搜索钉盘文件",
			UseWhen:      []string{"当你只记得文件名或内容关键词、不知道它在哪个目录时用它全局检索钉盘文件；输入 query，可按文件类型、扩展名、创建者、创建/修改时间范围过滤，返回匹配文件及其 ID，便于再做下载或整理。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws drive +search --query \"季度汇报\"",
				"dws drive +search --query \"合同\" --target file --extensions pdf,docx",
			},
		},
	},
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
		items, page, err := requireDriveCollection(data, "drive/search_files", "items", "files", "dentries", "entries", "nodes", "list")
		if err != nil {
			return err
		}
		files := projectDriveRows(items, map[string][]string{
			"name":      {"name", "fileName", "dentryName", "title"},
			"type":      {"type", "dentryType", "fileType", "spaceType"},
			"nodeId":    {"fileId", "dentryUuid", "nodeId", "id"},
			"dentryId":  {"dentryId"},
			"fileSize":  {"fileSize", "size", "byteSize", "length"},
			"creatorId": {"creatorId", "creatorUserId", "creator", "creatorUid"},
		})
		out := map[string]any{"count": len(files), "files": files}
		addDrivePagination(out, page)
		return rt.Output(out)
	},
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
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "drive",
			Name:           "shortcut_search_docs",
			CanonicalPath:  "drive.shortcut_search_docs",
			CLIPath:        "drive +search-docs",
			PrimaryCLIPath: "drive +search-docs",
		},
		Description: "搜索文档空间文档",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "搜索文档空间文档",
			UseWhen:      []string{"当你只记得文档标题或关键词、想在文档空间/知识库中检索在线文档（区别于 +search 检索钉盘文件）时使用；输入 query 关键词，返回匹配的文档及其节点信息。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws drive +search-docs --query \"季度汇报\""},
		},
	},
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
		items, page, err := requireDriveCollection(data, "doc/search_documents", "documents", "docs", "nodes", "items", "list")
		if err != nil {
			return err
		}
		docs := projectDriveRows(items, map[string][]string{
			"name":   {"name", "title", "docName", "nodeName", "fileName"},
			"nodeId": {"nodeId", "id", "docId", "dentryUuid", "fileId"},
			"type":   {"type", "docType", "nodeType", "fileType"},
			"url":    {"url", "docUrl", "link", "webUrl"},
		})
		out := map[string]any{"count": len(docs), "docs": docs}
		addDrivePagination(out, page)
		return rt.Output(out)
	},
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
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "drive",
			Name:           "shortcut_copy",
			CanonicalPath:  "drive.shortcut_copy",
			CLIPath:        "drive +copy",
			PrimaryCLIPath: "drive +copy",
		},
		Description: "复制文件/文档到指定位置",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "复制文件/文档到指定位置",
			UseWhen:      []string{"当你想保留原件、把某个文件/文档拷贝一份到指定文件夹或知识库时使用；输入源节点 node 及目标 folder/workspace，会实际生成一个副本，原文件位置不变。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws drive +copy --node <nodeId> --folder <targetFolderId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档/文件 ID", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 nodeId"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "目标知识库 ID"},
	},
	Tips: []string{`dws drive +copy --node <nodeId> --folder <targetFolderId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		preflight, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": rt.Str("node")})
		if err != nil {
			return err
		}
		preflight, err = requireDriveObject(preflight, "doc/get_document_info")
		if err != nil {
			return err
		}
		if !isOnlineDriveObject(preflight) {
			return driveResponseError("doc/copy_document", "ordinary_file_copy_unsupported", "普通钉盘文件当前没有独立副本接口；doc/copy_document 会生成 .dlink 快捷方式。需要快捷入口请用 +create-shortcut；需要独立副本请先 +download 再 +upload")
		}
		params := map[string]any{"nodeId": rt.Str("node")}
		if rt.Changed("folder") {
			params["targetFolderId"] = rt.Str("folder")
		}
		if rt.Changed("workspace") {
			params["workspaceId"] = rt.Str("workspace")
		}
		written, err := rt.CallMCPWriteDataStrict("doc", "copy_document", params)
		if err != nil {
			return err
		}
		written, err = requireDriveWrite(written, "doc/copy_document")
		if err != nil {
			return err
		}
		createdID := nestedString(written, "nodeId", "fileId", "dentryUuid", "id")
		if createdID == "" {
			return driveResponseError("doc/copy_document", "missing_created_id", "复制响应没有新节点 ID；远端效果未知")
		}
		verified, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": createdID})
		if err != nil {
			return err
		}
		verified, err = requireDriveObject(verified, "doc/get_document_info")
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"success": true, "sourceNodeId": rt.Str("node"), "nodeId": createdID, "copy": verified})
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
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "drive",
			Name:           "shortcut_move",
			CanonicalPath:  "drive.shortcut_move",
			CLIPath:        "drive +move",
			PrimaryCLIPath: "drive +move",
		},
		Description: "移动文件/文档到指定位置",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "移动文件/文档到指定位置",
			UseWhen:      []string{"当你要把某个文件/文档从当前位置转移到另一个文件夹或知识库（整理归档、调整目录结构）时使用；输入源节点 node 及目标 folder/workspace，会实际改变文件所在位置，原位置不再保留该文件。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws drive +move --node <nodeId> --folder <targetFolderId>"},
		},
	},
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
		written, err := rt.CallMCPWriteDataStrict("doc", "move_document", params)
		if err != nil {
			return err
		}
		written, err = requireDriveWrite(written, "doc/move_document")
		if err != nil {
			return err
		}
		verified, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": rt.Str("node")})
		if err != nil {
			return err
		}
		verified, err = requireDriveObject(verified, "doc/get_document_info")
		if err != nil {
			return err
		}
		remoteID := firstString(verified, "nodeId", "fileId", "dentryUuid", "id")
		if remoteID == "" {
			return driveResponseError("doc/move_document", "readback_missing_id", "移动后读回缺少节点 ID；无法证明读回的是已移动节点")
		}
		if remoteID != rt.Str("node") {
			return driveResponseError("doc/move_document", "readback_id_mismatch", fmt.Sprintf("移动后读回节点 %q 与请求节点 %q 不一致", remoteID, rt.Str("node")))
		}
		if rt.Changed("folder") {
			remoteFolder := firstString(verified, "folderId", "targetFolderId", "parentId")
			if remoteFolder == "" {
				return driveResponseError("doc/move_document", "readback_missing_folder", "移动后读回缺少目标文件夹 ID；无法证明移动已到达请求位置")
			}
			if remoteFolder != rt.Str("folder") {
				return driveResponseError("doc/move_document", "readback_folder_mismatch", fmt.Sprintf("移动后读回文件夹 %q 与请求 %q 不一致", remoteFolder, rt.Str("folder")))
			}
		}
		if rt.Changed("workspace") {
			remoteWorkspace := firstString(verified, "workspaceId", "spaceId")
			if remoteWorkspace == "" {
				return driveResponseError("doc/move_document", "readback_missing_workspace", "移动后读回缺少目标知识库 ID；无法证明移动已到达请求位置")
			}
			if remoteWorkspace != rt.Str("workspace") {
				return driveResponseError("doc/move_document", "readback_workspace_mismatch", fmt.Sprintf("移动后读回知识库 %q 与请求 %q 不一致", remoteWorkspace, rt.Str("workspace")))
			}
		}
		return rt.Output(map[string]any{"success": true, "nodeId": rt.Str("node"), "file": verified})
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
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "drive",
			Name:           "shortcut_recent",
			CanonicalPath:  "drive.shortcut_recent",
			CLIPath:        "drive +recent",
			PrimaryCLIPath: "drive +recent",
		},
		Description: "获取最近访问/编辑的文档列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "获取最近访问/编辑的文档列表",
			UseWhen:      []string{"当你想快速找回「我最近看过/改过的那个文档」而不记得它放在哪时使用；可按操作类型（最近访问/最近编辑）和创建人（全部/我创建/他人创建）过滤，返回近期文档列表及其节点信息。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws drive +recent",
				"dws drive +recent --operate-type 1 --creator-type 1",
			},
		},
	},
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
		items, page, err := requireDriveCollection(data, "doc/get_recent_list", "recentItems")
		if err != nil {
			return err
		}
		rows := projectDriveRows(items, map[string][]string{
			"name":        {"name"},
			"nodeType":    {"nodeType"},
			"contentType": {"contentType"},
			"accessTime":  {"accessTime"},
			"docUrl":      {"docUrl"},
			"nodeId":      {"nodeId"},
		})
		out := map[string]any{"count": len(rows), "items": rows}
		addDrivePagination(out, page)
		return rt.Output(out)
	},
}

func init() {
	Copy.Description = "复制在线文档到指定位置并读回验证"
	Copy.Intent = "复制钉钉在线文档到新位置并保留原件时使用；先验证对象类型，普通文件不伪装成复制成功。"
	Copy.Contract = driveContract(
		"+copy", Copy.Description,
		"复制钉钉在线文档到新位置并保留原件时使用；先验证对象类型，普通文件不伪装成复制成功。",
		[]string{"普通钉盘文件独立复制当前无等价接口：需要快捷入口用 +create-shortcut，需要独立字节副本用 +download 后 +upload；移动原件用 +move"},
		[]string{`dws drive +copy --node <ONLINE_DOC_ID> --folder <TARGET_FOLDER_ID>`},
		driveObjectResult("在线文档复制并读回后的新节点"), nil,
		// Preserve the historical public Schema properties. Execute translates
		// these CLI concepts to nodeId/targetFolderId/workspaceId for the RPC.
		contract.ParamDecl{Name: "node", Property: "node"},
		contract.ParamDecl{Name: "folder", Property: "folder"},
		contract.ParamDecl{Name: "workspace", Property: "workspace"},
	)
	Copy.Tips = []string{`dws drive +copy --node <ONLINE_DOC_ID> --folder <TARGET_FOLDER_ID>`}
	Info.Contract.Result = driveObjectResult("钉盘节点元数据")
	Search.Contract.Result = driveCollectionResult("files", "严格校验并投影的钉盘搜索结果页")
	Search.Contract.Pagination = driveCursorPagination()
	SearchDocs.Contract.Result = driveCollectionResult("docs", "兼容入口的在线文档搜索结果页")
	Copy.Contract.Result = driveObjectResult("复制操作的终态证据")
	Move.Contract.Result = driveObjectResult("移动操作的终态证据")
	Recent.Contract.Result = driveCollectionResult("items", "严格校验的最近访问或编辑结果页")
	Recent.Contract.Pagination = driveCursorPagination()
	for _, declaration := range []*shortcut.Shortcut{
		&List, &Info, &Download, &Search, &SearchDocs, &Copy, &Move, &Recent,
		&Inspect, &Upload, &CreateFolder, &CreateShortcut, &Rename, &Delete, &Stats,
		&RecycleList, &RecycleRestore, &StarList, &StarAdd, &StarRemove,
		&PublishGet, &PublishSet, &PublishUnset, &Cover,
		&VersionHistory, &VersionGet, &VersionDownload, &VersionRevert,
	} {
		declaration.OutputRollout = output.RolloutUnifiedActive
	}
	shortcut.Register(
		List,
		Info,
		Download,
		Search,
		SearchDocs,
		Copy,
		Move,
		Recent,
		Inspect,
		Upload,
		CreateFolder,
		CreateShortcut,
		Rename,
		Delete,
		Stats,
		RecycleList,
		RecycleRestore,
		StarList,
		StarAdd,
		StarRemove,
		PublishGet,
		PublishSet,
		PublishUnset,
		Cover,
		VersionHistory,
		VersionGet,
		VersionDownload,
		VersionRevert,
	)
}

func isOnlineDriveObject(info map[string]any) bool {
	extension := strings.ToLower(firstString(info, "extension", "fileExtension", "ext"))
	switch extension {
	case "adoc", "axls", "able", "amind", "adraw":
		return true
	}
	contentType := strings.ToUpper(firstString(info, "contentType", "docType"))
	return contentType == "DOC" || contentType == "SHEET" || contentType == "TABLE" || contentType == "MIND" || contentType == "DRAW"
}
