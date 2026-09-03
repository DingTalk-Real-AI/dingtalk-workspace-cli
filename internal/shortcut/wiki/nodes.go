// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package wiki

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var NodeList = readShortcut("+node-list", "严格分页列出知识库节点", "浏览知识库根目录或指定文件夹；只有显式 nodes:[] 才表示空目录，并完整保留 nextCursor/hasMore。", "nodes", "dws wiki +node-list --workspace <workspaceId> --format json", []shortcut.Flag{
	{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "知识库 ID"}, {Name: "folder", Type: shortcut.FlagString, Desc: "父节点 ID"}, {Name: "limit", Type: shortcut.FlagInt, Default: "50", Desc: "每页数量 1-50"}, {Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标", Aliases: []string{"page-token"}, AliasesVisible: true},
}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}, {Name: "folder", Property: "folderId"}, {Name: "limit", Property: "pageSize"}, {Name: "cursor", Property: "pageToken"}}, func(rt *shortcut.RuntimeContext) error {
	items, page, err := collectWikiPages(rt, "wiki/list_nodes", rt.Int("limit"), []string{"nodes", "items", "list"}, func(cursor string, size int) (map[string]any, error) {
		params := map[string]any{"workspaceId": rt.Str("workspace"), "pageSize": size}
		if rt.Changed("folder") {
			params["folderId"] = rt.Str("folder")
		}
		if cursor != "" {
			params["pageToken"] = cursor
		}
		return rt.CallMCPData("doc", "list_nodes", params)
	})
	if err != nil {
		return err
	}
	nodes := projectWikiRowsPreservingSource(items, nodeAliases())
	out := map[string]any{"count": len(nodes), "nodes": nodes}
	addWikiPagination(out, page)
	return rt.Output(out)
})

func nodeAliases() map[string][]string {
	return map[string][]string{
		"nodeId":         {"nodeId", "id", "dentryUuid", "fileId"},
		"name":           {"name", "title", "nodeName", "fileName"},
		"type":           {"type", "nodeType", "docType", "fileType"},
		"extension":      {"extension", "fileExtension", "docType"},
		"contentType":    {"contentType"},
		"hasChildren":    {"hasChildren"},
		"folderId":       {"folderId", "parentFolderId", "parentId"},
		"parentFolderId": {"parentFolderId", "folderId", "parentId"},
		"workspaceId":    {"workspaceId", "spaceId"},
		"url":            {"docUrl", "url", "webUrl"},
	}
}

func projectWikiNode(data map[string]any) map[string]any {
	rows := projectWikiRowsPreservingSource([]any{data}, nodeAliases())
	return rows[0]
}

func wikiNodeType(data map[string]any) string {
	if extension := firstWikiString(data, "extension", "fileExtension", "docType"); extension != "" {
		return strings.ToLower(strings.TrimPrefix(extension, "."))
	}
	return strings.ToLower(firstWikiString(data, "type", "nodeType", "fileType", "contentType"))
}

func requireWikiNodeIdentity(data map[string]any, operation, expectedID string) error {
	actualID := firstWikiString(data, "nodeId", "id", "fileId", "dentryUuid")
	if actualID == "" {
		return wikiResponseError(operation, "readback_missing_id", "读回节点缺少 nodeId")
	}
	if expectedID != "" && actualID != expectedID {
		return wikiResponseError(operation, "readback_id_mismatch", "读回节点 ID 与预期不一致")
	}
	return nil
}

func requireWikiNodeTarget(data map[string]any, operation, workspaceID, folderID string) error {
	actualWorkspace := firstWikiString(data, "workspaceId", "spaceId")
	if actualWorkspace == "" {
		return wikiResponseError(operation, "readback_missing_workspace", "读回节点缺少 workspaceId，无法验证目标空间")
	}
	if actualWorkspace != workspaceID {
		return wikiResponseError(operation, "workspace_readback_mismatch", "读回节点的 workspaceId 与请求目标不一致")
	}
	if folderID != "" && firstWikiString(data, "folderId", "parentFolderId", "parentId") != folderID {
		return wikiResponseError(operation, "folder_readback_mismatch", "读回节点的父文件夹与请求目标不一致")
	}
	return nil
}

func requireWikiCreatedNode(data map[string]any, operation, expectedID, workspaceID, folderID, name, nodeType string) error {
	if err := requireWikiNodeIdentity(data, operation, expectedID); err != nil {
		return err
	}
	if err := requireWikiNodeTarget(data, operation, workspaceID, folderID); err != nil {
		return err
	}
	if firstWikiString(data, "name", "title", "nodeName", "fileName") != name {
		return wikiResponseError(operation, "readback_name_mismatch", "创建后读回的节点名称与请求不一致")
	}
	if actualType := wikiNodeType(data); actualType == "" || actualType != strings.ToLower(nodeType) {
		return wikiResponseError(operation, "readback_type_mismatch", "创建后读回的节点类型与请求不一致")
	}
	return nil
}

func findMyDocumentsSpace(rt *shortcut.RuntimeContext, workspaceID string) (map[string]any, error) {
	cursor := ""
	seen := map[string]struct{}{}
	for page := 1; page <= 20; page++ {
		params := map[string]any{"wikiSpaceType": "myWikiSpace", "pageSize": 50}
		if cursor != "" {
			params["pageToken"] = cursor
		}
		data, err := rt.CallMCPData("wiki", "list_wikiSpaces", params)
		if err != nil {
			return nil, err
		}
		items, pageData, err := requireWikiCollection(data, "wiki/list_wikiSpaces", "wikiSpaces", "spaces")
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			space := item.(map[string]any)
			if firstWikiString(space, "workspaceId", "spaceId", "id") == workspaceID {
				return space, nil
			}
		}
		more, ok := pageData["hasMore"].(bool)
		if !ok || !more {
			return nil, wikiResponseError("wiki/list_wikiSpaces", "my_documents_workspace_not_found", "移动后 workspace 不在当前账号的我的文档空间列表中")
		}
		next := firstWikiString(pageData, "nextCursor", "nextToken", "nextPageToken", "pageToken")
		if next == "" {
			return nil, wikiResponseError("wiki/list_wikiSpaces", "missing_next_cursor", "我的文档空间列表仍有下一页但缺少游标")
		}
		if _, exists := seen[next]; exists {
			return nil, wikiResponseError("wiki/list_wikiSpaces", "cyclic_cursor", "我的文档空间列表游标形成循环")
		}
		seen[next] = struct{}{}
		cursor = next
	}
	return nil, wikiResponseError("wiki/list_wikiSpaces", "page_limit_reached", "我的文档空间列表超过安全分页上限")
}

var NodeGet = readShortcut("+node-get", "获取知识库节点详情", "已知节点 ID 或在线文档 URL 时读取元数据，并在节点信息之外统一返回文档/文件属性。", "", "dws wiki +node-get --node <nodeId> --format json", []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Required: true, Desc: "节点 ID 或 URL"}}, []contract.ParamDecl{{Name: "node", Property: "nodeId"}}, func(rt *shortcut.RuntimeContext) error {
	data, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": rt.Str("node")})
	if err != nil {
		return err
	}
	object, err := requireWikiObject(data, "doc/get_document_info")
	if err != nil {
		return err
	}
	if firstWikiString(object, "nodeId", "id", "fileId") == "" {
		return wikiResponseError("doc/get_document_info", "missing_node_id", "节点详情缺少 nodeId")
	}
	return rt.Output(projectWikiNode(object))
})

var NodeSearch = readShortcut("+node-search", "严格分页搜索知识库节点", "在指定知识库内按关键词和扩展名搜索节点；需要完整结果时用 --page-all，零命中必须来自显式 documents:[]。", "nodes", "dws wiki +node-search --workspace <workspaceId> --query \"方案\" --format json", []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "知识库 ID"}, {Name: "query", Type: shortcut.FlagString, Required: true, Desc: "关键词"}, {Name: "extensions", Type: shortcut.FlagStringSlice, Desc: "扩展名过滤"}, {Name: "limit", Type: shortcut.FlagInt, Default: "10", Desc: "每页数量 1-30"}, {Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceIds"}, {Name: "query", Property: "keyword"}, {Name: "extensions", Property: "extensions"}, {Name: "limit", Property: "pageSize"}, {Name: "cursor", Property: "pageToken"}}, func(rt *shortcut.RuntimeContext) error {
	pageSize := rt.Int("limit")
	if pageSize < 1 || pageSize > 30 {
		return fmt.Errorf("--limit 必须是 1-30 之间的整数")
	}
	items, page, err := collectWikiPages(rt, "doc/search_documents", pageSize, []string{"documents", "docs", "nodes", "items", "list"}, func(cursor string, size int) (map[string]any, error) {
		params := map[string]any{"workspaceIds": []string{rt.Str("workspace")}, "keyword": rt.Str("query"), "pageSize": size}
		if rt.Changed("extensions") {
			params["extensions"] = rt.StrSlice("extensions")
		}
		if cursor != "" {
			params["pageToken"] = cursor
		}
		return rt.CallMCPData("doc", "search_documents", params)
	})
	if err != nil {
		return err
	}
	nodes := projectWikiRowsPreservingSource(items, nodeAliases())
	out := map[string]any{"count": len(nodes), "nodes": nodes}
	addWikiPagination(out, page)
	return rt.Output(out)
})

var NodeCreate = writeShortcut("+node-create", "创建知识库节点并严格读回验证", "在知识库根目录或文件夹中创建文档、表格、白板、脑图或文件夹；只有新 nodeId、workspace、名称、类型和显式父文件夹读回一致才成功。", "dws wiki +node-create --workspace <workspaceId> --name \"新文档\" --format json", shortcut.RiskWrite, wikiWriteSafety(false), []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "知识库 ID"}, {Name: "name", Type: shortcut.FlagString, Required: true, Desc: "节点名称"}, {Name: "type", Type: shortcut.FlagString, Default: "adoc", Desc: "节点类型", Enum: []string{"adoc", "axls", "able", "appt", "adraw", "amind", "folder"}}, {Name: "folder", Type: shortcut.FlagString, Desc: "父文件夹 ID"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}, {Name: "name", Property: "name"}, {Name: "type", Property: "type"}, {Name: "folder", Property: "folderId"}}, func(rt *shortcut.RuntimeContext) error {
	params := map[string]any{"workspaceId": rt.Str("workspace"), "name": rt.Str("name"), "type": rt.Str("type")}
	if rt.Changed("folder") {
		params["folderId"] = rt.Str("folder")
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "doc/create_file", "arguments": params})
	}
	written, err := rt.CallMCPWriteDataStrict("doc", "create_file", params)
	if err != nil {
		return err
	}
	written, err = requireWikiWrite(written, "doc/create_file")
	if err != nil {
		return err
	}
	id := nestedWikiString(written, "nodeId", "fileId", "id")
	if id == "" {
		return wikiResponseError("doc/create_file", "missing_created_id", "创建响应没有 nodeId；远端效果未知")
	}
	verified, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": id})
	if err != nil {
		return err
	}
	verified, err = requireWikiObject(verified, "doc/get_document_info")
	if err != nil {
		return err
	}
	if err := requireWikiCreatedNode(verified, "doc/create_file", id, rt.Str("workspace"), rt.Str("folder"), rt.Str("name"), rt.Str("type")); err != nil {
		return err
	}
	verified = projectWikiNode(verified)
	return rt.Output(map[string]any{"success": true, "nodeId": id, "workspaceId": rt.Str("workspace"), "requestedType": rt.Str("type"), "node": verified})
})

var NodeCopy = writeShortcut("+node-copy", "复制知识库节点并验证独立副本", "复制现有在线节点到目标知识库/文件夹；先读源节点，再要求新 nodeId 与源 ID 不同，并验证副本 workspace、folder 和类型。", "dws wiki +node-copy --workspace <workspaceId> --node <nodeId> --format json", shortcut.RiskHighWrite, contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "non_idempotent"}, []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "目标知识库 ID"}, {Name: "node", Type: shortcut.FlagString, Required: true, Desc: "源节点 ID"}, {Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 ID"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}, {Name: "node", Property: "nodeId"}, {Name: "folder", Property: "targetFolderId"}}, func(rt *shortcut.RuntimeContext) error {
	params := map[string]any{"workspaceId": rt.Str("workspace"), "nodeId": rt.Str("node")}
	if rt.Changed("folder") {
		params["targetFolderId"] = rt.Str("folder")
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "doc/copy_document", "arguments": params})
	}
	source, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": rt.Str("node")})
	if err != nil {
		return err
	}
	source, err = requireWikiObject(source, "doc/get_document_info")
	if err != nil {
		return err
	}
	if err := requireWikiNodeIdentity(source, "doc/copy_document", rt.Str("node")); err != nil {
		return err
	}
	if firstWikiString(source, "workspaceId", "spaceId") == "" {
		return wikiResponseError("doc/copy_document", "source_missing_workspace", "源节点读回缺少 workspaceId，无法报告复制前位置")
	}
	written, err := rt.CallMCPWriteDataStrict("doc", "copy_document", params)
	if err != nil {
		return err
	}
	written, err = requireWikiWrite(written, "doc/copy_document")
	if err != nil {
		return err
	}
	id := nestedWikiString(written, "nodeId", "fileId", "id")
	if id == "" {
		return wikiResponseError("doc/copy_document", "missing_created_id", "复制响应没有新 nodeId；远端效果未知")
	}
	if id == rt.Str("node") {
		return wikiResponseError("doc/copy_document", "copy_reused_source_id", "复制响应复用了源 nodeId，无法证明生成了独立副本")
	}
	verified, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": id})
	if err != nil {
		return err
	}
	verified, err = requireWikiObject(verified, "doc/get_document_info")
	if err != nil {
		return err
	}
	if err := requireWikiNodeIdentity(verified, "doc/copy_document", id); err != nil {
		return err
	}
	if err := requireWikiNodeTarget(verified, "doc/copy_document", rt.Str("workspace"), rt.Str("folder")); err != nil {
		return err
	}
	sourceType, copyType := wikiNodeType(source), wikiNodeType(verified)
	if sourceType != "" && copyType != "" && sourceType != copyType {
		return wikiResponseError("doc/copy_document", "copy_type_mismatch", "副本类型与源节点不一致")
	}
	source = projectWikiNode(source)
	verified = projectWikiNode(verified)
	out := map[string]any{
		"success": true, "sourceNodeId": rt.Str("node"), "nodeId": id,
		"sourceWorkspaceId": firstWikiString(source, "workspaceId", "spaceId"),
		"targetWorkspaceId": firstWikiString(verified, "workspaceId", "spaceId"),
		"source":            source, "copy": verified,
	}
	if folderID := firstWikiString(verified, "folderId", "parentFolderId", "parentId"); folderID != "" {
		out["targetFolderId"] = folderID
	}
	return rt.Output(out)
})

func executeMove(rt *shortcut.RuntimeContext, toDrive bool) error {
	preflight, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": rt.Str("node")})
	if err != nil {
		return err
	}
	preflight, err = requireWikiObject(preflight, "doc/get_document_info")
	if err != nil {
		return err
	}
	if err := requireWikiNodeIdentity(preflight, "doc/move_document", rt.Str("node")); err != nil {
		return err
	}
	beforeWorkspace := firstWikiString(preflight, "workspaceId", "spaceId")
	if beforeWorkspace == "" {
		return wikiResponseError("doc/move_document", "source_missing_workspace", "移动前节点缺少 workspaceId，无法验证位置变化")
	}
	if toDrive && rt.Changed("workspace") && beforeWorkspace != rt.Str("workspace") {
		return wikiResponseError("doc/move_document", "source_workspace_mismatch", "移动前节点不属于 --workspace 指定的来源知识库")
	}
	params := map[string]any{"nodeId": rt.Str("node")}
	if !toDrive {
		params["workspaceId"] = rt.Str("workspace")
	}
	if rt.Changed("folder") {
		params["targetFolderId"] = rt.Str("folder")
	}
	if rt.DryRun() {
		requestedTarget := map[string]any{"domain": "wiki", "workspaceId": rt.Str("workspace")}
		if toDrive {
			requestedTarget = map[string]any{"domain": "my_documents"}
		}
		if rt.Changed("folder") {
			requestedTarget["folderId"] = rt.Str("folder")
		}
		return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "doc/move_document", "arguments": params, "source": projectWikiNode(preflight), "requestedTarget": requestedTarget})
	}
	written, err := rt.CallMCPWriteDataStrict("doc", "move_document", params)
	if err != nil {
		return err
	}
	if _, err = requireWikiWrite(written, "doc/move_document"); err != nil {
		return err
	}
	verified, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": rt.Str("node")})
	if err != nil {
		return err
	}
	verified, err = requireWikiObject(verified, "doc/get_document_info")
	if err != nil {
		return err
	}
	if firstWikiString(verified, "nodeId", "id", "fileId") != rt.Str("node") {
		return wikiResponseError("doc/move_document", "readback_id_mismatch", "移动后读回节点 ID 不一致")
	}
	afterWorkspace := firstWikiString(verified, "workspaceId", "spaceId")
	var targetSpace map[string]any
	if toDrive {
		if beforeWorkspace == "" || afterWorkspace == "" || beforeWorkspace == afterWorkspace {
			return wikiResponseError("doc/move_document", "drive_move_not_verified", "移动到我的文档后 workspace 未发生可验证变化")
		}
		targetSpace, err = findMyDocumentsSpace(rt, afterWorkspace)
		if err != nil {
			return err
		}
	} else if afterWorkspace != rt.Str("workspace") {
		return wikiResponseError("doc/move_document", "workspace_readback_mismatch", "移动后读回的目标知识库不一致")
	}
	if rt.Changed("folder") && firstWikiString(verified, "folderId", "parentFolderId", "parentId") != rt.Str("folder") {
		return wikiResponseError("doc/move_document", "folder_readback_mismatch", "移动后读回的目标文件夹不一致")
	}
	preflight = projectWikiNode(preflight)
	verified = projectWikiNode(verified)
	targetDomain := "wiki"
	if toDrive {
		targetDomain = "my_documents"
	}
	out := map[string]any{
		"success": true, "nodeId": rt.Str("node"), "targetDomain": targetDomain,
		"sourceWorkspaceId": beforeWorkspace, "targetWorkspaceId": afterWorkspace,
		"source": preflight, "target": verified, "node": verified,
	}
	if folderID := firstWikiString(verified, "folderId", "parentFolderId", "parentId"); folderID != "" {
		out["targetFolderId"] = folderID
	}
	if targetSpace != nil {
		out["targetSpace"] = targetSpace
	}
	return rt.Output(out)
}

var Move = writeShortcut("+move", "移动节点到知识库并读回验证", "将 Wiki 节点或我的文档在线节点移动到目标知识库/文件夹；同一入口覆盖库内移动与在线文档入库场景。", "dws wiki +move --workspace <workspaceId> --node <nodeId> --format json", shortcut.RiskWrite, wikiWriteSafety(true), []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "目标知识库 ID"}, {Name: "node", Type: shortcut.FlagString, Required: true, Desc: "节点 ID"}, {Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 ID"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}, {Name: "node", Property: "nodeId"}, {Name: "folder", Property: "targetFolderId"}}, func(rt *shortcut.RuntimeContext) error { return executeMove(rt, false) })

var MoveToDrive = writeShortcut("+move-to-drive", "移动 Wiki 节点到我的文档并验证目标域", "将 Wiki 在线节点移动到我的文档根目录或指定文件夹；可用 --workspace 断言来源知识库，返回移动前后元数据，并确认目标 workspace 属于 myWikiSpace 范围。", "dws wiki +move-to-drive --node <nodeId> --workspace <sourceWorkspaceId> --format json", shortcut.RiskWrite, wikiWriteSafety(true), []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Required: true, Desc: "Wiki 节点 ID"}, {Name: "workspace", Type: shortcut.FlagString, Desc: "可选的来源知识库 ID；移动前必须与节点归属一致"}, {Name: "folder", Type: shortcut.FlagString, Desc: "我的文档目标文件夹 ID"}}, []contract.ParamDecl{{Name: "node", Property: "nodeId"}, {Name: "workspace", Property: "sourceWorkspaceId"}, {Name: "folder", Property: "targetFolderId"}}, func(rt *shortcut.RuntimeContext) error { return executeMove(rt, true) })

var NodeDelete = writeShortcut("+node-delete", "删除知识库节点", "明确确认后将节点移入回收站；先读取目标，且删除响应必须提供 success=true 终态证据。", "dws wiki +node-delete --workspace <workspaceId> --node <nodeId> --format json", shortcut.RiskHighWrite, wikiDeleteSafety(), []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "知识库 ID，用于确认影响范围"}, {Name: "node", Type: shortcut.FlagString, Required: true, Desc: "节点 ID"}}, []contract.ParamDecl{{Name: "node", Property: "nodeId"}}, func(rt *shortcut.RuntimeContext) error {
	preflight, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": rt.Str("node")})
	if err != nil {
		return err
	}
	preflight, err = requireWikiObject(preflight, "doc/get_document_info")
	if err != nil {
		return err
	}
	if workspace := firstWikiString(preflight, "workspaceId", "spaceId"); workspace != "" && workspace != rt.Str("workspace") {
		return wikiResponseError("doc/delete_document", "workspace_preflight_mismatch", "节点不属于请求确认的知识库")
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "doc/delete_document", "target": preflight})
	}
	written, err := rt.CallMCPWriteDataStrict("doc", "delete_document", map[string]any{"nodeId": rt.Str("node")})
	if err != nil {
		return err
	}
	if _, err = requireWikiWrite(written, "doc/delete_document"); err != nil {
		return err
	}
	return rt.Output(map[string]any{"success": true, "nodeId": rt.Str("node"), "deleted": true})
})

var FeedList = readShortcut("+feed-list", "严格分页列出知识库动态", "查看谁在何时创建、更新或评论了知识库内容；严格验证 feeds 数组并保留游标。", "feeds", "dws wiki +feed-list --workspace <workspaceId> --format json", []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "知识库 ID 或 URL"}, {Name: "limit", Type: shortcut.FlagInt, Default: "10", Desc: "每页数量 1-20"}, {Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"}, {Name: "exclude-file", Type: shortcut.FlagBool, Desc: "排除普通文件动态"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}, {Name: "limit", Property: "maxResults"}, {Name: "cursor", Property: "nextToken"}, {Name: "exclude-file", Property: "excludeFile"}}, func(rt *shortcut.RuntimeContext) error {
	items, page, err := collectWikiPages(rt, "wiki/list_workspace_feeds", rt.Int("limit"), []string{"feeds", "items", "list"}, func(cursor string, size int) (map[string]any, error) {
		params := map[string]any{"workspaceId": rt.Str("workspace"), "maxResults": size}
		if cursor != "" {
			params["nextToken"] = cursor
		}
		if rt.Changed("exclude-file") {
			params["excludeFile"] = rt.Bool("exclude-file")
		}
		return rt.CallMCPData("wiki", "list_workspace_feeds", params)
	})
	if err != nil {
		return err
	}
	feeds := projectWikiRowsPreservingSource(items, map[string][]string{"id": {"id", "feedId"}, "type": {"type", "feedType", "action"}, "time": {"time", "createTime"}, "name": {"name", "title", "fileName"}, "nodeId": {"nodeId", "fileId"}})
	out := map[string]any{"count": len(feeds), "feeds": feeds}
	addWikiPagination(out, page)
	return rt.Output(out)
})

func init() {
	Move.Aliases = []string{"+node-move"}
	for _, item := range []*shortcut.Shortcut{&NodeList, &NodeSearch, &FeedList} {
		enableWikiAutoPage(item)
	}
	for _, item := range []*shortcut.Shortcut{&NodeList, &NodeSearch, &FeedList} {
		item.Contract.Pagination = wikiCursorPagination()
	}
	NodeList.Contract.Result = wikiNodeCollectionResult()
	NodeSearch.Contract.Result = wikiNodeCollectionResult()
	NodeCreate.Contract.Result = wikiNodeCreateResult()
	NodeCopy.Contract.Result = wikiNodeCopyResult()
	Move.Contract.Result = wikiNodeMoveResult()
	MoveToDrive.Contract.Result = wikiNodeMoveResult()
	shortcut.Register(NodeList, NodeGet, NodeSearch, NodeCreate, NodeCopy, Move, MoveToDrive, NodeDelete, FeedList)
	_ = fmt.Sprintf
	_ = output.RolloutUnifiedActive
}
