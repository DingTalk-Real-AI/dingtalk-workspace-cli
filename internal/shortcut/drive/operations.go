// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package drive

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var Inspect = shortcut.Shortcut{
	Service: "drive", Command: "+inspect", Product: "drive",
	Description: "聚合检查节点元数据及可选统计、公开状态和封面",
	Intent:      "已知节点 ID，需要一次确认名称、类型、路径，并可附带统计、公开状态或封面时使用。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: driveContract(
		"+inspect", "聚合检查节点元数据及可选统计、公开状态和封面",
		"已知节点 ID，需要一次确认名称、类型、路径，并可附带统计、公开状态或封面时使用。",
		[]string{"读取在线文档正文使用 doc +fetch；浏览目录使用 drive +list"},
		[]string{`dws drive +inspect --node <dentryUuid>`, `dws drive +inspect --node <dentryUuid> --include-stats --include-publish`},
		driveObjectResult("Drive 节点聚合检查结果"), nil,
		contract.ParamDecl{Name: "node", Property: "fileId"},
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "节点 ID", Required: true},
		{Name: "space-id", Type: shortcut.FlagString, Desc: "钉盘空间 ID"},
		{Name: "include-stats", Type: shortcut.FlagBool, Desc: "附带阅读、编辑、下载等统计"},
		{Name: "include-publish", Type: shortcut.FlagBool, Desc: "附带互联网公开状态"},
		{Name: "include-cover", Type: shortcut.FlagBool, Desc: "附带封面或缩略图地址"},
	},
	Tips: []string{`dws drive +inspect --node <dentryUuid>`, `dws drive +inspect --node <dentryUuid> --include-stats --include-publish`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		node := rt.Str("node")
		params := map[string]any{"fileId": node}
		if rt.Str("space-id") != "" {
			params["spaceId"] = rt.Str("space-id")
		}
		data, err := rt.CallMCPData("drive", "get_file_info", params)
		if err != nil {
			return err
		}
		info, err := requireDriveObject(data, "drive/get_file_info")
		if err != nil {
			return err
		}
		result := map[string]any{"file": info}
		steps := []map[string]any{{"tool": "get_file_info", "status": "success"}}
		reads := []struct {
			flag, key, tool string
			params          map[string]any
		}{
			{"include-stats", "stats", "get_node_stats", map[string]any{"nodeId": node}},
			{"include-publish", "publish", "get_file_publish_status", map[string]any{"fileId": node}},
			{"include-cover", "cover", "get_cover", map[string]any{"nodeId": node}},
		}
		failures := []map[string]any{}
		for _, read := range reads {
			if !rt.Bool(read.flag) {
				continue
			}
			value, callErr := rt.CallMCPReadData("drive", read.tool, read.params)
			if callErr == nil {
				if read.tool == "get_node_stats" {
					value, callErr = requireDriveStats(value, "drive/"+read.tool)
				} else {
					value, callErr = requireDriveObject(value, "drive/"+read.tool)
				}
			}
			if callErr != nil {
				steps = append(steps, map[string]any{"tool": read.tool, "status": "failed"})
				failures = append(failures, map[string]any{"tool": read.tool, "error": callErr.Error()})
				continue
			}
			result[read.key] = value
			steps = append(steps, map[string]any{"tool": read.tool, "status": "success"})
		}
		if len(failures) > 0 {
			return apperrors.NewAPI("Drive 聚合检查只完成了部分读取",
				apperrors.WithOperation("drive.inspect"),
				apperrors.WithReason("drive_inspect_partial"),
				apperrors.WithFailureStage("optional_reads"),
				apperrors.WithExecutionStarted(false),
				apperrors.WithRetryable(true),
				apperrors.WithDetails(map[string]any{"status": "partial_success", "complete": false, "data": result, "steps": steps, "failures": failures}),
			)
		}
		return rt.Output(map[string]any{"status": "success", "complete": true, "data": result, "steps": steps})
	},
}

var CreateFolder = shortcut.Shortcut{
	Service: "drive", Command: "+create-folder", Product: "drive",
	Description: "创建钉盘文件夹并读回验证",
	Intent:      "在钉盘中创建普通文件夹并需要拿到经过读回验证的 fileId 时使用。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
	Contract: driveContract(
		"+create-folder", "创建钉盘文件夹并读回验证",
		"在钉盘中创建普通文件夹并需要拿到经过读回验证的 fileId 时使用。",
		[]string{"知识库目录使用 wiki node create；创建在线文档使用 doc +create"},
		[]string{`dws drive +create-folder --name "项目资料"`, `dws drive +create-folder --name "子目录" --folder <dentryUuid>`},
		driveObjectResult("创建并验证后的文件夹"), nil,
		contract.ParamDecl{Name: "folder", Property: "parentId"},
	),
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "文件夹名称", Required: true},
		{Name: "space-id", Type: shortcut.FlagString, Desc: "钉盘空间 ID"},
		{Name: "folder", Type: shortcut.FlagString, Desc: "父文件夹 ID"},
	},
	Tips: []string{`dws drive +create-folder --name "项目资料"`, `dws drive +create-folder --name "子目录" --folder <dentryUuid>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"name": rt.Str("name")}
		if rt.Str("space-id") != "" {
			params["spaceId"] = rt.Str("space-id")
		}
		if rt.Str("folder") != "" {
			params["parentId"] = rt.Str("folder")
		}
		created, err := rt.CallMCPWriteDataStrict("drive", "create_folder", params)
		if err != nil {
			return err
		}
		created, err = requireDriveWrite(created, "drive/create_folder")
		if err != nil {
			return err
		}
		nodeID := nestedString(created, "fileId", "dentryUuid", "nodeId", "id")
		if nodeID == "" {
			return driveResponseError("drive/create_folder", "missing_created_id", "创建响应没有文件夹 fileId；远端效果未知")
		}
		verified, err := rt.CallMCPData("drive", "get_file_info", map[string]any{"fileId": nodeID})
		if err != nil {
			return err
		}
		verified, err = requireDriveObject(verified, "drive/get_file_info")
		if err != nil {
			return err
		}
		if name := firstString(verified, "name", "fileName"); name != rt.Str("name") {
			return driveCommittedWriteMismatch(
				"drive/create_folder",
				"readback_mismatch",
				fmt.Sprintf("创建后读回名称 %q 与请求 %q 不一致", name, rt.Str("name")),
				nodeID,
				rt.Str("name"),
				name,
				map[string]any{
					"resourceType":  "folder",
					"nodeId":        nodeID,
					"requestedName": rt.Str("name"),
					"observedName":  name,
					"ownership":     "owned",
					"readback":      verified,
				},
			)
		}
		return rt.Output(map[string]any{"success": true, "nodeId": nodeID, "folder": verified})
	},
}

var CreateShortcut = shortcut.Shortcut{
	Service: "drive", Command: "+create-shortcut", Product: "drive",
	Description: "为已有节点创建快捷方式并验证新节点",
	Intent:      "给已有文件或文档创建快捷入口，并需要区分 shortcut 与独立副本时使用。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "non_idempotent"},
	Contract: driveContract(
		"+create-shortcut", "为已有节点创建快捷方式并验证新节点",
		"给已有文件或文档创建快捷入口，并需要区分 shortcut 与独立副本时使用。",
		[]string{"需要独立副本使用 drive +copy；需要迁移原节点使用 drive +move"},
		[]string{`dws drive +create-shortcut --node <SOURCE_NODE> --folder <TARGET_FOLDER>`},
		driveObjectResult("创建并验证后的快捷方式节点"), nil,
		contract.ParamDecl{Name: "node", Property: "nodeId"},
		contract.ParamDecl{Name: "folder", Property: "targetFolderId"},
		contract.ParamDecl{Name: "workspace", Property: "workspaceId"},
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "源节点 ID", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 ID"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "目标知识库 ID"},
	},
	Tips: []string{`dws drive +create-shortcut --node <SOURCE_NODE> --folder <TARGET_FOLDER>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node")}
		if rt.Str("folder") != "" {
			params["targetFolderId"] = rt.Str("folder")
		}
		if rt.Str("workspace") != "" {
			params["workspaceId"] = rt.Str("workspace")
		}
		created, err := rt.CallMCPWriteDataStrict("drive", "create_shortcut", params)
		if err != nil {
			return err
		}
		created, err = requireDriveWrite(created, "drive/create_shortcut")
		if err != nil {
			return err
		}
		nodeID := nestedString(created, "fileId", "dentryUuid", "nodeId", "id")
		if nodeID == "" {
			return driveResponseError("drive/create_shortcut", "missing_created_id", "创建快捷方式未返回新节点 ID")
		}
		verified, err := rt.CallMCPData("drive", "get_file_info", map[string]any{"fileId": nodeID})
		if err != nil {
			return err
		}
		verified, err = requireDriveObject(verified, "drive/get_file_info")
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"success": true, "nodeId": nodeID, "shortcut": verified})
	},
}

var Rename = shortcut.Shortcut{
	Service: "drive", Command: "+rename", Product: "doc",
	Description: "重命名文件或文件夹并读回验证",
	Intent:      "改变已有节点名称并要求验证最终名称时使用。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: driveContract(
		"+rename", "重命名文件或文件夹并读回验证",
		"改变已有节点名称并要求验证最终名称时使用。",
		[]string{"改变位置使用 drive +move；替换文件内容使用 drive +upload --node"},
		[]string{`dws drive +rename --node <dentryUuid> --name "新名称"`},
		driveObjectResult("重命名后的节点元数据"), nil,
		contract.ParamDecl{Name: "node", Property: "nodeId"},
		contract.ParamDecl{Name: "name", Property: "newName"},
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "节点 ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新名称", Required: true},
	},
	Tips: []string{`dws drive +rename --node <dentryUuid> --name "新名称"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		preflight, err := rt.CallMCPData("drive", "get_file_info", map[string]any{"fileId": rt.Str("node")})
		if err != nil {
			return err
		}
		preflight, err = requireDriveObject(preflight, "drive/get_file_info")
		if err != nil {
			return err
		}
		requestName, expectedNames := normalizedDriveRename(rt.Str("name"), preflight)
		written, err := rt.CallMCPWriteDataStrict("doc", "rename_document", map[string]any{"nodeId": rt.Str("node"), "newName": requestName})
		if err != nil {
			return err
		}
		if _, err := requireDriveWrite(written, "doc/rename_document"); err != nil {
			return err
		}
		verifiedService := "drive"
		verifiedTool := "get_file_info"
		verifiedParams := map[string]any{"fileId": rt.Str("node")}
		verifiedOperation := "drive/get_file_info"
		if isOnlineDriveObject(preflight) {
			verifiedService = "doc"
			verifiedTool = "get_document_info"
			verifiedParams = map[string]any{"nodeId": rt.Str("node")}
			verifiedOperation = "doc/get_document_info"
		}
		verified, err := rt.CallMCPData(verifiedService, verifiedTool, verifiedParams)
		if err != nil {
			return err
		}
		verified, err = requireDriveObject(verified, verifiedOperation)
		if err != nil {
			return err
		}
		name := firstString(verified, "name", "fileName", "title", "documentName")
		if name == "" || !expectedNames[name] {
			return driveResponseError("doc/rename_document", "readback_mismatch", fmt.Sprintf("重命名读回名称 %q 与请求 %q 不一致", name, rt.Str("name")))
		}
		return rt.Output(map[string]any{"success": true, "nodeId": rt.Str("node"), "file": verified})
	},
}

func normalizedDriveRename(name string, info map[string]any) (string, map[string]bool) {
	name = strings.TrimSpace(name)
	expected := map[string]bool{name: true}
	nodeType := strings.ToLower(firstString(info, "type", "nodeType", "fileType"))
	if nodeType == "folder" || nodeType == "dir" || nodeType == "directory" {
		return name, expected
	}
	extension := strings.TrimLeft(strings.ToLower(firstString(info, "extension", "fileExtension", "ext")), ".")
	if extension == "" {
		return name, expected
	}
	suffix := "." + extension
	if len(name) > len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix) {
		return name[:len(name)-len(suffix)], expected
	}
	expected[name+suffix] = true
	return name, expected
}

var Delete = shortcut.Shortcut{
	Service: "drive", Command: "+delete", Product: "doc",
	Description: "将已确认节点移入回收站",
	Intent:      "用户明确要求删除已确认钉盘节点，并理解它会进入回收站时使用。",
	Risk:        shortcut.RiskHighWrite,
	Safety:      contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: driveContract(
		"+delete", "将已确认节点移入回收站",
		"用户明确要求删除已确认钉盘节点，并理解它会进入回收站时使用。",
		[]string{"只是调整位置使用 drive +move；目标不明确时先 drive +inspect"},
		[]string{`dws drive +delete --node <dentryUuid>`},
		driveObjectResult("删除到回收站的终态证据"), nil,
		contract.ParamDecl{Name: "node", Property: "nodeId"},
	),
	Flags: []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Desc: "节点 ID", Required: true}},
	Tips:  []string{`dws drive +delete --node <dentryUuid>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		written, err := rt.CallMCPWriteDataStrict("doc", "delete_document", map[string]any{"nodeId": rt.Str("node")})
		if err != nil {
			return err
		}
		written, err = requireDriveWrite(written, "doc/delete_document")
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"success": true, "nodeId": rt.Str("node"), "result": written})
	},
}

var Stats = driveReadObjectShortcut(
	"+stats", "读取节点访问和协作统计", "get_node_stats", "nodeId",
	"用户要查看指定节点阅读、编辑、评论、点赞、预览或下载统计时使用。",
	[]string{"只要文件名称和类型使用 drive +inspect"},
)

var Cover = driveReadObjectShortcut(
	"+cover", "读取节点封面或缩略图地址", "get_cover", "nodeId",
	"用户需要节点封面、首图或缩略图 URL 时使用。",
	[]string{"这是封面资源，不等于 Lark 服务端多格式预览转换"},
)

func driveReadObjectShortcut(command, description, tool, property, useWhen string, avoidWhen []string) shortcut.Shortcut {
	return shortcut.Shortcut{
		Service: "drive", Command: command, Product: "drive", Description: description, Intent: useWhen,
		Risk: shortcut.RiskRead, Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: driveContract(command, description, useWhen, avoidWhen,
			[]string{fmt.Sprintf("dws drive %s --node <dentryUuid>", command)},
			driveObjectResult(description), nil, contract.ParamDecl{Name: "node", Property: property}),
		Flags: []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Desc: "节点 ID", Required: true}},
		Tips:  []string{fmt.Sprintf("dws drive %s --node <dentryUuid>", command)},
		Execute: func(rt *shortcut.RuntimeContext) error {
			data, err := rt.CallMCPData("drive", tool, map[string]any{property: rt.Str("node")})
			if err != nil {
				return err
			}
			data, err = requireDriveObject(data, "drive/"+tool)
			if err != nil {
				return err
			}
			return rt.Output(data)
		},
	}
}
