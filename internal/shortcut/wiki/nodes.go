// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package wiki

import (
	"fmt"

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
	nodes := projectWikiRows(items, nodeAliases())
	out := map[string]any{"count": len(nodes), "nodes": nodes}
	addWikiPagination(out, page)
	return rt.Output(out)
})

func nodeAliases() map[string][]string {
	return map[string][]string{"nodeId": {"nodeId", "id", "dentryUuid", "fileId"}, "name": {"name", "title", "nodeName", "fileName"}, "type": {"type", "nodeType", "docType", "fileType"}, "contentType": {"contentType"}, "folderId": {"folderId", "parentId"}, "workspaceId": {"workspaceId", "spaceId"}, "url": {"docUrl", "url", "webUrl"}}
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
	return rt.Output(object)
})

var NodeSearch = readShortcut("+node-search", "严格搜索知识库节点", "在指定知识库内按关键词和扩展名搜索节点；零命中必须来自显式 documents:[]。", "nodes", "dws wiki +node-search --workspace <workspaceId> --query \"方案\" --format json", []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "知识库 ID"}, {Name: "query", Type: shortcut.FlagString, Required: true, Desc: "关键词"}, {Name: "extensions", Type: shortcut.FlagStringSlice, Desc: "扩展名过滤"}, {Name: "limit", Type: shortcut.FlagInt, Default: "10", Desc: "每页数量 1-30"}, {Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceIds"}, {Name: "query", Property: "keyword"}, {Name: "extensions", Property: "extensions"}, {Name: "limit", Property: "pageSize"}, {Name: "cursor", Property: "pageToken"}}, func(rt *shortcut.RuntimeContext) error {
	params := map[string]any{"workspaceIds": []string{rt.Str("workspace")}, "keyword": rt.Str("query"), "pageSize": rt.Int("limit")}
	if rt.Changed("extensions") {
		params["extensions"] = rt.StrSlice("extensions")
	}
	if rt.Changed("cursor") {
		params["pageToken"] = rt.Str("cursor")
	}
	data, err := rt.CallMCPData("doc", "search_documents", params)
	if err != nil {
		return err
	}
	items, page, err := requireWikiCollection(data, "doc/search_documents", "documents", "docs", "nodes", "items", "list")
	if err != nil {
		return err
	}
	nodes := projectWikiRows(items, nodeAliases())
	out := map[string]any{"count": len(nodes), "nodes": nodes}
	addWikiPagination(out, page)
	return rt.Output(out)
})

var NodeCreate = writeShortcut("+node-create", "创建知识库节点并读回验证", "在知识库根目录或文件夹中创建文档、表格、白板、脑图或文件夹；取得 nodeId 并读回后才成功。", "dws wiki +node-create --workspace <workspaceId> --name \"新文档\" --format json", shortcut.RiskWrite, wikiWriteSafety(false), []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "知识库 ID"}, {Name: "name", Type: shortcut.FlagString, Required: true, Desc: "节点名称"}, {Name: "type", Type: shortcut.FlagString, Default: "adoc", Desc: "节点类型", Enum: []string{"adoc", "axls", "able", "appt", "adraw", "amind", "folder"}}, {Name: "folder", Type: shortcut.FlagString, Desc: "父文件夹 ID"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}, {Name: "name", Property: "name"}, {Name: "type", Property: "type"}, {Name: "folder", Property: "folderId"}}, func(rt *shortcut.RuntimeContext) error {
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
	if firstWikiString(verified, "nodeId", "id", "fileId") != id {
		return wikiResponseError("doc/create_file", "readback_id_mismatch", "创建后读回节点 ID 不一致")
	}
	return rt.Output(map[string]any{"success": true, "nodeId": id, "node": verified})
})

var NodeCopy = writeShortcut("+node-copy", "复制知识库节点并读回验证", "复制现有在线节点到目标知识库/文件夹；高风险确认后要求新 nodeId 并读取副本元数据。", "dws wiki +node-copy --workspace <workspaceId> --node <nodeId> --format json", shortcut.RiskHighWrite, contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "non_idempotent"}, []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "目标知识库 ID"}, {Name: "node", Type: shortcut.FlagString, Required: true, Desc: "源节点 ID"}, {Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 ID"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}, {Name: "node", Property: "nodeId"}, {Name: "folder", Property: "targetFolderId"}}, func(rt *shortcut.RuntimeContext) error {
	params := map[string]any{"workspaceId": rt.Str("workspace"), "nodeId": rt.Str("node")}
	if rt.Changed("folder") {
		params["targetFolderId"] = rt.Str("folder")
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "doc/copy_document", "arguments": params})
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
	verified, err := rt.CallMCPData("doc", "get_document_info", map[string]any{"nodeId": id})
	if err != nil {
		return err
	}
	verified, err = requireWikiObject(verified, "doc/get_document_info")
	if err != nil {
		return err
	}
	if firstWikiString(verified, "nodeId", "id", "fileId") != id {
		return wikiResponseError("doc/copy_document", "readback_id_mismatch", "复制后读回节点 ID 不一致")
	}
	return rt.Output(map[string]any{"success": true, "sourceNodeId": rt.Str("node"), "nodeId": id, "copy": verified})
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
	beforeWorkspace := firstWikiString(preflight, "workspaceId", "spaceId")
	params := map[string]any{"nodeId": rt.Str("node")}
	if !toDrive {
		params["workspaceId"] = rt.Str("workspace")
	}
	if rt.Changed("folder") {
		params["targetFolderId"] = rt.Str("folder")
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "doc/move_document", "arguments": params, "target": preflight})
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
	if toDrive {
		if beforeWorkspace == "" || afterWorkspace == "" || beforeWorkspace == afterWorkspace {
			return wikiResponseError("doc/move_document", "drive_move_not_verified", "移动到我的文档后 workspace 未发生可验证变化")
		}
	} else if afterWorkspace != rt.Str("workspace") {
		return wikiResponseError("doc/move_document", "workspace_readback_mismatch", "移动后读回的目标知识库不一致")
	}
	if rt.Changed("folder") && firstWikiString(verified, "folderId", "parentId") != rt.Str("folder") {
		return wikiResponseError("doc/move_document", "folder_readback_mismatch", "移动后读回的目标文件夹不一致")
	}
	return rt.Output(map[string]any{"success": true, "nodeId": rt.Str("node"), "node": verified})
}

var Move = writeShortcut("+move", "移动节点到知识库并读回验证", "将 Wiki 节点或我的文档在线节点移动到目标知识库/文件夹；同一入口覆盖库内移动与在线文档入库场景。", "dws wiki +move --workspace <workspaceId> --node <nodeId> --format json", shortcut.RiskWrite, wikiWriteSafety(true), []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "目标知识库 ID"}, {Name: "node", Type: shortcut.FlagString, Required: true, Desc: "节点 ID"}, {Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 ID"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}, {Name: "node", Property: "nodeId"}, {Name: "folder", Property: "targetFolderId"}}, func(rt *shortcut.RuntimeContext) error { return executeMove(rt, false) })

var MoveToDrive = writeShortcut("+move-to-drive", "移动 Wiki 节点到我的文档", "将 Wiki 在线节点同步移动到我的文档或指定文件夹，并以元数据读回证明任务完成。", "dws wiki +move-to-drive --node <nodeId> --format json", shortcut.RiskWrite, wikiWriteSafety(true), []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Required: true, Desc: "Wiki 节点 ID"}, {Name: "folder", Type: shortcut.FlagString, Desc: "我的文档目标文件夹 ID"}}, []contract.ParamDecl{{Name: "node", Property: "nodeId"}, {Name: "folder", Property: "targetFolderId"}}, func(rt *shortcut.RuntimeContext) error { return executeMove(rt, true) })

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
	feeds := projectWikiRows(items, map[string][]string{"id": {"id", "feedId"}, "type": {"type", "feedType", "action"}, "time": {"time", "createTime"}, "name": {"name", "title", "fileName"}, "nodeId": {"nodeId", "fileId"}})
	out := map[string]any{"count": len(feeds), "feeds": feeds}
	addWikiPagination(out, page)
	return rt.Output(out)
})

func init() {
	Move.Aliases = []string{"+node-move"}
	for _, item := range []*shortcut.Shortcut{&NodeList, &FeedList} {
		enableWikiAutoPage(item)
	}
	for _, item := range []*shortcut.Shortcut{&NodeList, &NodeSearch, &FeedList} {
		item.Contract.Pagination = wikiCursorPagination()
	}
	shortcut.Register(NodeList, NodeGet, NodeSearch, NodeCreate, NodeCopy, Move, MoveToDrive, NodeDelete, FeedList)
	_ = fmt.Sprintf
	_ = output.RolloutUnifiedActive
}
