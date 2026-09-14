// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package drive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var uploadDriveFile = helpers.UploadDriveFileData

var uploadDocSpaceFile = helpers.UploadDocSpaceFileData

var driveRestoreWait = time.Sleep

var (
	driveGetwd        = os.Getwd
	driveEvalSymlinks = filepath.EvalSymlinks
	driveRel          = filepath.Rel
	driveStat         = os.Stat
)

var RecycleList = shortcut.Shortcut{
	Service: "drive", Command: "+recycle-list", Product: "drive",
	Description: "严格分页列出钉盘回收站",
	Intent:      "查找可恢复的回收项并获取 recycleItemId 时使用。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: driveContract(
		"+recycle-list", "严格分页列出钉盘回收站",
		"查找可恢复的回收项并获取 recycleItemId 时使用。",
		[]string{"恢复前必须先确认具体回收项；普通目录浏览使用 drive +list"},
		[]string{`dws drive +recycle-list --limit 20`},
		driveCollectionResult("items", "严格校验的回收站条目页"), driveCursorPagination(),
		contract.ParamDecl{Name: "cursor", Property: "nextCursor"},
	),
	Flags: []shortcut.Flag{
		{Name: "space-id", Type: shortcut.FlagString, Desc: "钉盘空间 ID"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"},
	},
	Tips: []string{`dws drive +recycle-list --limit 20`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"maxResults": rt.Int("limit")}
		if rt.Str("space-id") != "" {
			params["spaceId"] = rt.Str("space-id")
		}
		if rt.Str("cursor") != "" {
			params["nextCursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("drive", "list_recycle_items", params)
		if err != nil {
			return err
		}
		items, page, err := requireDriveCollection(data, "drive/list_recycle_items", "recycleItems")
		if err != nil {
			return err
		}
		rows := projectDriveRows(items, map[string][]string{
			"recycleItemId": {"recycleItemId", "id"},
			"originalName":  {"originalName", "name", "fileName", "title"},
			"originalPath":  {"originalPath", "path"},
			"type":          {"type", "fileType", "nodeType", "contentType"},
			"deleteTime":    {"operatorTime", "deleteTime", "deletedTime", "recycleTime"},
			"fileSize":      {"fileSize", "size"},
		})
		out := map[string]any{"count": len(rows), "items": rows}
		addDrivePagination(out, page)
		return rt.Output(out)
	},
}

var RecycleRestore = shortcut.Shortcut{
	Service: "drive", Command: "+recycle-restore", Product: "drive",
	Description: "恢复已确认的回收站条目并读回节点",
	Intent:      "已经通过 drive +recycle-list 确认回收项，并明确要求恢复时使用。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: driveContract(
		"+recycle-restore", "恢复已确认的回收站条目并读回节点",
		"已经通过 drive +recycle-list 确认回收项，并明确要求恢复时使用。",
		[]string{"尚未确认回收项时先列表；不要把原节点 ID 当 recycleItemId"},
		[]string{`dws drive +recycle-restore --id <recycleItemId>`},
		driveObjectResult("恢复并读回验证后的节点"), nil,
		contract.ParamDecl{Name: "id", Property: "recycleItemId"},
	),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "回收项 ID", Required: true},
	},
	Tips: []string{`dws drive +recycle-restore --id <recycleItemId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		recycleItem, err := findDriveRecycleItem(rt, rt.Str("id"))
		if err != nil {
			return err
		}
		written, err := rt.CallMCPWriteDataStrict("drive", "restore_recycle_item", map[string]any{"recycleItemId": rt.Str("id")})
		if err != nil {
			return err
		}
		if _, err := requireDriveWrite(written, "drive/restore_recycle_item"); err != nil {
			return err
		}
		nodeID := nestedString(written, "fileId", "nodeId", "dentryUuid", "id")
		if nodeID == "" {
			nodeID, err = findRestoredDriveNode(rt, recycleItem)
			if err != nil {
				return err
			}
		}
		verified, err := rt.CallMCPData("drive", "get_file_info", map[string]any{"fileId": nodeID})
		if err != nil {
			return err
		}
		verified, err = requireDriveObject(verified, "drive/get_file_info")
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"success": true, "nodeId": nodeID, "file": verified})
	},
}

func findDriveRecycleItem(rt *shortcut.RuntimeContext, recycleItemID string) (map[string]any, error) {
	params := map[string]any{"maxResults": 50}
	for pageNumber := 0; pageNumber < 20; pageNumber++ {
		data, err := rt.CallMCPData("drive", "list_recycle_items", params)
		if err != nil {
			return nil, err
		}
		items, page, err := requireDriveCollection(data, "drive/list_recycle_items", "recycleItems")
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			entry := item.(map[string]any)
			if nestedString(entry, "recycleItemId", "id") == recycleItemID {
				return entry, nil
			}
		}
		nextCursor := nestedString(page, "nextCursor", "nextToken", "nextPageToken")
		hasMore, hasMorePresent := boolField(page, "hasMore")
		if nextCursor == "" || (hasMorePresent && !hasMore) {
			break
		}
		params["nextCursor"] = nextCursor
	}
	return nil, driveResponseError("drive/list_recycle_items", "recycle_item_not_found", "回收站中没有找到指定 recycleItemId；未执行恢复")
}

func findRestoredDriveNode(rt *shortcut.RuntimeContext, recycleItem map[string]any) (string, error) {
	name := nestedString(recycleItem, "originalName", "name", "fileName", "title")
	if name == "" {
		return "", driveResponseError("drive/restore_recycle_item", "missing_restored_identity", "恢复响应没有节点 ID，且恢复前回收项没有名称，无法安全读回终态")
	}
	searchName := name
	if extension := filepath.Ext(name); extension != "" {
		searchName = strings.TrimSuffix(name, extension)
	}
	originalPath := nestedString(recycleItem, "originalPath", "path")
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			driveRestoreWait(750 * time.Millisecond)
		}
		if originalPath != "" {
			if nodeID, found, err := findRestoredDriveNodeAtOriginalPath(rt, originalPath, name); err != nil {
				return "", err
			} else if found {
				return nodeID, nil
			}
		}
		data, err := rt.CallMCPData("drive", "search_files", map[string]any{"keyword": searchName, "pageSize": 30})
		if err != nil {
			return "", err
		}
		items, _, err := requireDriveCollection(data, "drive/search_files", "items", "files", "dentries", "entries", "nodes", "list")
		if err != nil {
			return "", err
		}
		candidates := make([]string, 0, 1)
		for _, item := range items {
			entry := item.(map[string]any)
			entryName := nestedString(entry, "name", "fileName", "dentryName", "title")
			entryExtension := strings.TrimPrefix(nestedString(entry, "extension", "fileExtension", "ext"), ".")
			fullEntryName := entryName
			if entryExtension != "" && filepath.Ext(entryName) == "" {
				fullEntryName += "." + entryExtension
			}
			if entryName != name && fullEntryName != name && entryName != searchName {
				continue
			}
			if originalPath != "" {
				path := nestedString(entry, "path", "originalPath")
				if path != "" && path != originalPath {
					continue
				}
			}
			if id := nestedString(entry, "fileId", "dentryUuid", "nodeId", "id"); id != "" {
				candidates = append(candidates, id)
			}
		}
		if len(candidates) == 1 {
			return candidates[0], nil
		}
		if len(candidates) > 1 {
			return "", driveResponseError("drive/restore_recycle_item", "restored_node_ambiguous", "服务端已接受恢复，但按原名称找到多个节点，无法唯一确认恢复终态；请用 +list 核对")
		}
	}
	return "", driveResponseError("drive/restore_recycle_item", "restored_node_not_found", "服务端已接受恢复，但没有返回节点 ID，且在有界等待后仍按原名称搜索不到恢复后的节点；远端效果未知，请先用 +list 确认")
}

func findRestoredDriveNodeAtOriginalPath(rt *shortcut.RuntimeContext, originalPath, name string) (string, bool, error) {
	parentName := filepath.Base(filepath.Dir(originalPath))
	if parentName == "." || parentName == string(filepath.Separator) || parentName == "" {
		return "", false, nil
	}
	data, err := rt.CallMCPData("drive", "search_files", map[string]any{"keyword": parentName, "pageSize": 30})
	if err != nil {
		return "", false, err
	}
	items, _, err := requireDriveCollection(data, "drive/search_files", "items", "files", "dentries", "entries", "nodes", "list")
	if err != nil {
		return "", false, err
	}
	parentIDs := make([]string, 0, 1)
	for _, item := range items {
		entry := item.(map[string]any)
		if nestedString(entry, "name", "fileName", "dentryName", "title") != parentName {
			continue
		}
		if id := nestedString(entry, "fileId", "dentryUuid", "nodeId", "id"); id != "" {
			parentIDs = append(parentIDs, id)
		}
	}
	if len(parentIDs) != 1 {
		return "", false, nil
	}
	data, err = rt.CallMCPData("drive", "list_files", map[string]any{"parentId": parentIDs[0], "maxResults": 50})
	if err != nil {
		return "", false, err
	}
	items, _, err = requireDriveCollection(data, "drive/list_files", "items", "files", "dentries", "entries", "nodes", "list")
	if err != nil {
		return "", false, err
	}
	var nodeID string
	for _, item := range items {
		entry := item.(map[string]any)
		if nestedString(entry, "name", "fileName", "dentryName", "title") != name {
			continue
		}
		if nodeID != "" {
			return "", false, driveResponseError("drive/list_files", "restored_node_ambiguous", "恢复后的原目录中存在多个同名节点，无法唯一确认终态")
		}
		nodeID = nestedString(entry, "fileId", "dentryUuid", "nodeId", "id")
	}
	return nodeID, nodeID != "", nil
}

var StarList = shortcut.Shortcut{
	Service: "drive", Command: "+star-list", Product: "drive",
	Description: "严格分页列出当前用户收藏",
	Intent:      "浏览当前用户收藏并获取后续可操作节点 ID 时使用。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: driveContract(
		"+star-list", "严格分页列出当前用户收藏",
		"浏览当前用户收藏并获取后续可操作节点 ID 时使用。",
		[]string{"最近访问使用 drive +recent；目录浏览使用 drive +list"},
		[]string{`dws drive +star-list --limit 20`},
		driveCollectionResult("items", "严格校验的收藏条目页"), driveCursorPagination(),
		contract.ParamDecl{Name: "cursor", Property: "cursor"},
	),
	Flags: []shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"},
		{Name: "content-types", Type: shortcut.FlagStringSlice, Desc: "内容类型过滤"},
	},
	Tips: []string{`dws drive +star-list --limit 20`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"limit": rt.Int("limit")}
		if rt.Str("cursor") != "" {
			params["cursor"] = rt.Str("cursor")
		}
		if values := rt.StrSlice("content-types"); len(values) > 0 {
			params["contentTypes"] = values
		}
		data, err := rt.CallMCPData("drive", "get_star_list", params)
		if err != nil {
			return err
		}
		items, page, err := requireDriveCollection(data, "drive/get_star_list", "starList")
		if err != nil {
			return err
		}
		rows := projectDriveRows(items, map[string][]string{
			"nodeId":     {"nodeId", "fileId", "dentryUuid", "id"},
			"name":       {"name", "fileName", "title"},
			"type":       {"type", "contentType", "nodeType"},
			"createTime": {"createTime", "starTime"},
		})
		out := map[string]any{"count": len(rows), "items": rows}
		addDrivePagination(out, page)
		return rt.Output(out)
	},
}

var StarAdd = starMutationShortcut("+star-add", "收藏指定节点", "mark_star", "将指定文件或文档加入当前用户收藏。")
var StarRemove = starMutationShortcut("+star-remove", "取消收藏指定节点", "unmark_star", "将指定文件或文档从当前用户收藏移除。")

func starMutationShortcut(command, description, tool, useWhen string) shortcut.Shortcut {
	return shortcut.Shortcut{
		Service: "drive", Command: command, Product: "drive", Description: description, Intent: useWhen,
		Risk: shortcut.RiskWrite, Safety: contract.SafetySpec{Effect: "write", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: driveContract(command, description, useWhen,
			[]string{"查看收藏状态和列表使用 drive +star-list"},
			[]string{fmt.Sprintf("dws drive %s --node <dentryUuid>", command)},
			driveObjectResult(description+"的终态证据"), nil, contract.ParamDecl{Name: "node", Property: "nodeId"}),
		Flags: []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Desc: "节点 ID", Required: true}},
		Tips:  []string{fmt.Sprintf("dws drive %s --node <dentryUuid>", command)},
		Execute: func(rt *shortcut.RuntimeContext) error {
			written, err := rt.CallMCPWriteDataStrict("drive", tool, map[string]any{"nodeId": rt.Str("node")})
			if err != nil {
				return err
			}
			written, err = requireDriveWrite(written, "drive/"+tool)
			if err != nil {
				return err
			}
			return rt.Output(map[string]any{"success": true, "nodeId": rt.Str("node"), "result": written})
		},
	}
}

var PublishGet = shortcut.Shortcut{
	Service: "drive", Command: "+publish-get", Product: "drive",
	Description: "查询文件互联网公开状态",
	Intent:      "查询文件当前互联网公开状态和权限时使用。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: driveContract(
		"+publish-get", "查询文件互联网公开状态", "查询文件当前互联网公开状态和权限时使用。",
		[]string{"开启或关闭公开分别使用 drive +publish-set / +publish-unset"},
		[]string{`dws drive +publish-get --node <dentryUuid>`}, driveObjectResult("互联网公开状态"), nil,
		contract.ParamDecl{Name: "node", Property: "fileId"},
	),
	Flags: []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Desc: "文件 ID", Required: true}},
	Tips:  []string{`dws drive +publish-get --node <dentryUuid>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("drive", "get_file_publish_status", map[string]any{"fileId": rt.Str("node")})
		if err != nil {
			return err
		}
		data, err = requireDriveObject(data, "drive/get_file_publish_status")
		if err != nil {
			return err
		}
		return rt.Output(data)
	},
}

var PublishSet = publishMutationShortcut("+publish-set", true)
var PublishUnset = publishMutationShortcut("+publish-unset", false)

func publishMutationShortcut(command string, published bool) shortcut.Shortcut {
	description := "开启文件互联网公开发布"
	useWhen := "用户明确同意任何持链接者可访问，并已确认公开权限时使用。"
	if !published {
		description = "关闭文件互联网公开发布"
		useWhen = "用户明确要求让现有互联网公开链接失效时使用。"
	}
	flags := []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Desc: "文件 ID", Required: true}}
	if published {
		flags = append(flags, shortcut.Flag{Name: "permission", Type: shortcut.FlagString, Default: "DOWNLOADER", Desc: "公开权限", Enum: []string{"READER", "DOWNLOADER", "EDITOR"}})
	}
	return shortcut.Shortcut{
		Service: "drive", Command: command, Product: "drive", Description: description, Intent: useWhen,
		Risk: shortcut.RiskHighWrite, Safety: contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "unknown"},
		Contract: driveContract(command, description, useWhen,
			[]string{"只查询状态使用 drive +publish-get；企业内部协作者权限使用 doc +access-grant"},
			[]string{fmt.Sprintf("dws drive %s --node <dentryUuid>", command)},
			driveObjectResult(description+"并读回验证的状态"), nil, contract.ParamDecl{Name: "node", Property: "fileId"}),
		Flags: flags,
		Tips:  []string{fmt.Sprintf("dws drive %s --node <dentryUuid>", command)},
		Execute: func(rt *shortcut.RuntimeContext) error {
			params := map[string]any{"fileId": rt.Str("node"), "published": published}
			if published {
				params["publishPermission"] = rt.Str("permission")
			}
			written, err := rt.CallMCPWriteDataStrict("drive", "set_file_publish", params)
			if err != nil {
				return err
			}
			if _, err := requireDriveWrite(written, "drive/set_file_publish"); err != nil {
				return err
			}
			verified, err := rt.CallMCPData("drive", "get_file_publish_status", map[string]any{"fileId": rt.Str("node")})
			if err != nil {
				return err
			}
			verified, err = requireDriveObject(verified, "drive/get_file_publish_status")
			if err != nil {
				return err
			}
			actual, ok := boolField(verified, "published", "isPublished", "publish")
			if !ok || actual != published {
				return driveResponseError("drive/set_file_publish", "readback_mismatch", "公开状态写入后读回不一致")
			}
			return rt.Output(map[string]any{"success": true, "nodeId": rt.Str("node"), "publish": verified})
		},
	}
}

func boolField(data map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		if value, ok := data[key].(bool); ok {
			return value, true
		}
	}
	return false, false
}

var Upload = shortcut.Shortcut{
	Service: "drive", Command: "+upload", Product: "drive",
	Description: "从工作目录上传普通文件到钉盘或文档空间并读回验证",
	Intent:      "把工作目录内普通文件上传到钉盘，或用 --workspace 上传为知识库/文档空间中的独立文件节点，并验证远端节点 ID、名称和目标空间；服务端提供大小时同时校验大小。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: driveContract(
		"+upload", "从工作目录上传普通文件到钉盘或文档空间并读回验证",
		"把工作目录内普通文件上传到钉盘，或用 --workspace 上传为知识库/文档空间中的独立文件节点，并验证远端节点 ID、名称和目标空间；服务端提供大小时同时校验大小。",
		[]string{"在线文档导入转换使用 doc +import；作为在线文档正文附件使用 doc +media-insert；--space-id 与 --workspace 属于不同目标域，不可同时使用；--mime-type 仅适用于钉盘上传，不能与 --workspace 同时使用；覆盖已有文件必须显式 --node"},
		[]string{`dws drive +upload --file report.pdf`, `dws drive +upload --file notes.txt --workspace <workspaceId>`},
		driveObjectResult("上传并读回验证后的远端文件"), nil,
		contract.ParamDecl{Name: "workspace", Property: "workspaceId"},
		contract.ParamDecl{Name: "folder", Property: "parentId"},
		contract.ParamDecl{Name: "node", Property: "overwriteFileId"},
	),
	Flags: []shortcut.Flag{
		{Name: "file", Type: shortcut.FlagString, Desc: "工作目录内的相对文件路径", Required: true},
		{Name: "file-name", Type: shortcut.FlagString, Desc: "远端显示名称，默认使用本地文件名"},
		{Name: "mime-type", Type: shortcut.FlagString, Desc: "钉盘上传的 MIME 类型；不能与 --workspace 同时使用"},
		{Name: "space-id", Type: shortcut.FlagString, Desc: "钉盘空间 ID"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "知识库或文档空间 workspaceId；与 --space-id、--mime-type 互斥"},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标域内的父文件夹 ID"},
		{Name: "node", Type: shortcut.FlagString, Desc: "覆盖目标文件 ID"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"folder", "node"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"space-id", "workspace"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"workspace", "mime-type"}},
	},
	Tips: []string{`dws drive +upload --file report.pdf`, `dws drive +upload --file notes.txt --workspace <workspaceId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		path, info, err := resolveDriveUploadInput(rt.Str("file"))
		if err != nil {
			return err
		}
		name := rt.Str("file-name")
		if name == "" {
			name = info.Name()
		}
		workspaceID := rt.Str("workspace")
		if workspaceID != "" && filepath.Ext(name) == "" {
			name += filepath.Ext(path)
		}
		if rt.DryRun() {
			operation := "drive.upload"
			if workspaceID != "" {
				operation = "doc.upload_file"
			}
			preview := map[string]any{"dry_run": true, "executed": false, "operation": operation, "file": rt.Str("file"), "fileName": name, "sizeBytes": info.Size()}
			for key, value := range map[string]string{"spaceId": rt.Str("space-id"), "workspaceId": workspaceID, "folderId": rt.Str("folder"), "nodeId": rt.Str("node")} {
				if value != "" {
					preview[key] = value
				}
			}
			if mimeType := rt.Str("mime-type"); mimeType != "" {
				preview["mimeType"] = mimeType
			}
			return rt.Output(preview)
		}
		operation := "drive/commit_upload"
		var committed map[string]any
		if workspaceID != "" {
			operation = "doc/commit_uploaded_file"
			committed, err = uploadDocSpaceFile(rt.Command().Context(), helpers.DocSpaceUploadRequest{
				FilePath: path, FileName: name, FileSize: info.Size(), WorkspaceID: workspaceID, FolderID: rt.Str("folder"), OverwriteNode: rt.Str("node"),
			})
		} else {
			committed, err = uploadDriveFile(rt.Command().Context(), helpers.DriveUploadRequest{
				FilePath: path, FileName: name, FileSize: info.Size(), SpaceID: rt.Str("space-id"), ParentID: rt.Str("folder"), OverwriteFile: rt.Str("node"), MIMEType: rt.Str("mime-type"),
			})
		}
		if err != nil {
			return err
		}
		if workspaceID != "" {
			// commit_uploaded_file returns a business receipt identified by a
			// flat or result-wrapped node ID; success=true is not part of its
			// stable contract. Preserve explicit failures, then let the ID and
			// read-back checks below prove the remote write.
			committed, err = requireDriveResponse(committed, operation)
		} else {
			committed, err = requireDriveWrite(committed, operation)
		}
		if err != nil {
			return err
		}
		nodeID := rt.Str("node")
		if nodeID == "" {
			nodeID = nestedString(committed, "fileId", "dentryUuid", "nodeId", "id")
		}
		if nodeID == "" {
			return driveResponseError(operation, "missing_created_id", "上传提交没有返回文件 ID；远端效果未知")
		}
		readServer, readTool := "drive", "get_file_info"
		readArgs := map[string]any{"fileId": nodeID}
		if workspaceID != "" {
			readServer, readTool = "doc", "get_document_info"
			readArgs = map[string]any{"nodeId": nodeID}
		}
		verified, err := rt.CallMCPData(readServer, readTool, readArgs)
		if err != nil {
			return err
		}
		verified, err = requireDriveObject(verified, readServer+"/"+readTool)
		if err != nil {
			return err
		}
		remoteID := firstString(verified, "fileId", "dentryUuid", "nodeId", "id")
		if remoteID == "" {
			return driveResponseError(operation, "readback_missing_id", "上传后读回缺少文件 ID；无法证明读回的是已提交文件")
		}
		if remoteID != nodeID {
			return driveResponseError(operation, "readback_id_mismatch", fmt.Sprintf("上传后读回文件 ID %q 与提交 ID %q 不一致", remoteID, nodeID))
		}
		if workspaceID != "" {
			remoteWorkspaceID := firstString(verified, "workspaceId", "spaceId")
			if remoteWorkspaceID == "" {
				return driveResponseError(operation, "readback_missing_workspace", "上传后读回缺少 workspaceId；无法证明文件进入了请求的知识库或文档空间")
			}
			if remoteWorkspaceID != workspaceID {
				return driveResponseError(operation, "readback_workspace_mismatch", fmt.Sprintf("上传后读回 workspaceId %q 与请求 %q 不一致", remoteWorkspaceID, workspaceID))
			}
		}
		if remoteName := firstString(verified, "name", "fileName"); !driveReadbackNameMatches(verified, name) {
			return driveCommittedWriteMismatch(
				operation,
				"readback_mismatch",
				fmt.Sprintf("上传后读回名称 %q 与请求 %q 不一致", remoteName, name),
				nodeID,
				name,
				remoteName,
				verified,
			)
		}
		remoteSize, hasRemoteSize := firstInt64(verified, "fileSize", "size", "byteSize", "length")
		if !hasRemoteSize && workspaceID == "" {
			return driveResponseError(operation, "readback_missing_size", "上传后读回缺少有效文件大小；无法证明远端文件完整")
		}
		if hasRemoteSize && remoteSize != info.Size() {
			return driveResponseError(operation, "readback_size_mismatch", fmt.Sprintf("上传后读回大小 %d 与本地文件大小 %d 不一致", remoteSize, info.Size()))
		}
		out := map[string]any{"success": true, "nodeId": nodeID, "sizeBytes": info.Size(), "file": verified}
		if spaceID := rt.Str("space-id"); spaceID != "" {
			out["spaceId"] = spaceID
		}
		if workspaceID != "" {
			out["workspaceId"] = workspaceID
		}
		return rt.Output(out)
	},
}

func resolveDriveUploadInput(raw string) (string, os.FileInfo, error) {
	if strings.TrimSpace(raw) == "" || filepath.IsAbs(raw) {
		return "", nil, fmt.Errorf("--file 只接受工作目录内的相对文件路径")
	}
	cwd, err := driveGetwd()
	if err != nil {
		return "", nil, err
	}
	base, err := driveEvalSymlinks(cwd)
	if err != nil {
		return "", nil, err
	}
	path, err := driveEvalSymlinks(filepath.Join(base, filepath.Clean(raw)))
	if err != nil {
		return "", nil, fmt.Errorf("读取上传文件失败: %w", err)
	}
	rel, err := driveRel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return "", nil, fmt.Errorf("--file 不能逃逸工作目录")
	}
	info, err := driveStat(path)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() || info.Size() <= 0 {
		return "", nil, fmt.Errorf("--file 必须是非空普通文件")
	}
	return path, info, nil
}
