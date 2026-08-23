// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package drive

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var VersionHistory = shortcut.Shortcut{
	Service: "drive", Command: "+version-history", Product: "drive",
	Description: "严格分页列出普通文件历史版本",
	Intent:      "普通文件需要浏览历史版本并获取明确版本号时使用。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: driveContract(
		"+version-history", "严格分页列出普通文件历史版本",
		"普通文件需要浏览历史版本并获取明确版本号时使用。",
		[]string{"在线文档版本使用 doc +version-list；在线表格版本使用 sheet version"},
		[]string{`dws drive +version-history --node <dentryUuid> --limit 20`},
		driveCollectionResult("versions", "严格校验的普通文件历史版本页"), driveCursorPagination(),
		contract.ParamDecl{Name: "node", Property: "nodeId"},
		contract.ParamDecl{Name: "cursor", Property: "nextCursor"},
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "普通文件节点 ID", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"},
	},
	Tips: []string{`dws drive +version-history --node <dentryUuid> --limit 20`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		page, err := versionPage(rt, rt.Str("node"), rt.Int("limit"), rt.Str("cursor"))
		if err != nil {
			return err
		}
		return rt.Output(page)
	},
}

var VersionGet = shortcut.Shortcut{
	Service: "drive", Command: "+version-get", Product: "drive",
	Description: "按版本号精确读取普通文件版本元数据",
	Intent:      "已知普通文件版本号，需要确认该版本存在并读取其元数据时使用。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: driveContract(
		"+version-get", "按版本号精确读取普通文件版本元数据",
		"已知普通文件版本号，需要确认该版本存在并读取其元数据时使用。",
		[]string{"尚未确定版本号时使用 drive +version-history；需要版本字节使用 drive +version-download"},
		[]string{`dws drive +version-get --node <dentryUuid> --version 3`},
		driveObjectResult("精确匹配的普通文件版本元数据"), nil,
		contract.ParamDecl{Name: "node", Property: "nodeId"},
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "普通文件节点 ID", Required: true},
		{Name: "version", Type: shortcut.FlagInt, Desc: "版本号；--version 必须为正整数", Required: true},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"version"}, Description: "--version 必须为正整数"}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("version") <= 0 {
			return fmt.Errorf("--version 必须为正整数")
		}
		return nil
	},
	Tips: []string{`dws drive +version-get --node <dentryUuid> --version 3`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		version, err := findVersion(rt, rt.Str("node"), rt.Int("version"))
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"nodeId": rt.Str("node"), "version": version})
	},
}

var VersionDownload = shortcut.Shortcut{
	Service: "drive", Command: "+version-download", Product: "drive",
	Description: "安全下载普通文件指定历史版本",
	Intent:      "已确认普通文件版本号，需要安全落盘并验证实际字节时使用。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: driveContract(
		"+version-download", "安全下载普通文件指定历史版本",
		"已确认普通文件版本号，需要安全落盘并验证实际字节时使用。",
		[]string{"下载最新版本使用 drive +download；在线文档版本使用 doc +version-list/+export"},
		[]string{`dws drive +version-download --node <dentryUuid> --version 3 --output downloads/report-v3.pdf`},
		driveObjectResult("已验证的历史版本本地产物"), nil,
		contract.ParamDecl{Name: "node", Property: "nodeId"},
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "普通文件节点 ID", Required: true},
		{Name: "version", Type: shortcut.FlagInt, Desc: "版本号；--version 必须为正整数", Required: true},
		{Name: "output", Type: shortcut.FlagString, Desc: "工作目录内相对输出路径", Required: true},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"version"}, Description: "--version 必须为正整数"}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("version") <= 0 {
			return fmt.Errorf("--version 必须为正整数")
		}
		return nil
	},
	Tips: []string{`dws drive +version-download --node <dentryUuid> --version 3 --output downloads/report-v3.pdf`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if _, err := findVersion(rt, rt.Str("node"), rt.Int("version")); err != nil {
			return err
		}
		data, err := rt.CallMCPData("drive", "download_file_version", map[string]any{"nodeId": rt.Str("node"), "version": rt.Int("version")})
		if err != nil {
			return err
		}
		payload, err := requireDriveObject(data, "drive/download_file_version")
		if err != nil {
			return err
		}
		url, name, headers, err := driveDownloadPayload(payload, "drive/download_file_version")
		if err != nil {
			return err
		}
		cwd, err := driveGetwd()
		if err != nil {
			return err
		}
		artifact, err := driveDownload(rt.Command().Context(), url, localio.DownloadOptions{BaseDir: cwd, Output: rt.Str("output"), PreferredName: name, Headers: headers})
		if err != nil {
			return err
		}
		if artifact.SizeBytes <= 0 {
			return driveResponseError("drive/download_file_version", "empty_download_artifact", "历史版本下载产物为 0 字节")
		}
		return rt.Output(map[string]any{"success": true, "nodeId": rt.Str("node"), "version": rt.Int("version"), "savedPath": artifact.RelativePath, "sizeBytes": artifact.SizeBytes})
	},
}

var VersionRevert = shortcut.Shortcut{
	Service: "drive", Command: "+version-revert", Product: "drive",
	Description: "预检并回滚普通文件到指定历史版本",
	Intent:      "用户明确选定普通文件历史版本并要求回滚时使用。",
	Risk:        shortcut.RiskHighWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "non_idempotent"},
	Contract: driveContract(
		"+version-revert", "预检并回滚普通文件到指定历史版本",
		"用户明确选定普通文件历史版本并要求回滚时使用。",
		[]string{"尚未确认版本号时先 drive +version-history；在线文档使用 doc +version-revert"},
		[]string{`dws drive +version-revert --node <dentryUuid> --version 3`},
		driveObjectResult("版本回滚终态及节点读回"), nil,
		contract.ParamDecl{Name: "node", Property: "nodeId"},
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "普通文件节点 ID", Required: true},
		{Name: "version", Type: shortcut.FlagInt, Desc: "版本号；--version 必须为正整数", Required: true},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"version"}, Description: "--version 必须为正整数"}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("version") <= 0 {
			return fmt.Errorf("--version 必须为正整数")
		}
		return nil
	},
	Tips: []string{`dws drive +version-revert --node <dentryUuid> --version 3`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if _, err := findVersion(rt, rt.Str("node"), rt.Int("version")); err != nil {
			return err
		}
		written, err := rt.CallMCPWriteDataStrict("drive", "revert_file_version", map[string]any{"nodeId": rt.Str("node"), "version": rt.Int("version")})
		if err != nil {
			return err
		}
		if _, err := requireDriveWrite(written, "drive/revert_file_version"); err != nil {
			return err
		}
		verified, err := rt.CallMCPData("drive", "get_file_info", map[string]any{"fileId": rt.Str("node")})
		if err != nil {
			return err
		}
		verified, err = requireDriveObject(verified, "drive/get_file_info")
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"success": true, "nodeId": rt.Str("node"), "revertedTo": rt.Int("version"), "file": verified})
	},
}

func versionPage(rt *shortcut.RuntimeContext, nodeID string, limit int, cursor string) (map[string]any, error) {
	params := map[string]any{"nodeId": nodeID, "maxResults": limit}
	if cursor != "" {
		params["nextCursor"] = cursor
	}
	data, err := rt.CallMCPData("drive", "list_file_versions", params)
	if err != nil {
		return nil, err
	}
	items, page, err := requireDriveCollection(data, "drive/list_file_versions", "versions", "versionList", "items")
	if err != nil {
		return nil, err
	}
	versions := projectDriveRows(items, map[string][]string{
		"version":    {"version", "versionNumber", "versionNum"},
		"createTime": {"createTime", "createdTime"},
		"creatorId":  {"creatorId", "creatorUserId", "operatorId"},
		"fileSize":   {"fileSize", "size"},
		"name":       {"name", "fileName"},
	})
	out := map[string]any{"count": len(versions), "versions": versions}
	addDrivePagination(out, page)
	return out, nil
}

func findVersion(rt *shortcut.RuntimeContext, nodeID string, target int) (map[string]any, error) {
	const maxPages = 20
	cursor := ""
	seenCursors := map[string]bool{}
	for pageNumber := 1; pageNumber <= maxPages; pageNumber++ {
		page, err := versionPage(rt, nodeID, 50, cursor)
		if err != nil {
			return nil, err
		}
		for _, version := range page["versions"].([]map[string]any) {
			if number, ok := versionNumber(version); ok && number == target {
				return version, nil
			}
		}

		nextCursor := strings.TrimSpace(nestedString(page, "nextCursor", "nextToken", "nextPageToken"))
		hasMore, hasMoreKnown := boolField(page, "hasMore")
		if hasMoreKnown && !hasMore {
			return nil, driveResponseError("drive/list_file_versions", "version_not_found", fmt.Sprintf("完整版本历史中不存在版本 %d", target))
		}
		if nextCursor == "" {
			if hasMoreKnown && hasMore {
				return nil, versionPaginationError("missing_next_cursor", pageNumber, cursor)
			}
			return nil, driveResponseError("drive/list_file_versions", "version_not_found", fmt.Sprintf("完整版本历史中不存在版本 %d", target))
		}
		if seenCursors[nextCursor] {
			return nil, versionPaginationError("stalled_cursor", pageNumber, nextCursor)
		}
		seenCursors[nextCursor] = true
		cursor = nextCursor
	}
	return nil, versionPaginationError("max_pages", maxPages, cursor)
}

func versionPaginationError(reason string, page int, cursor string) error {
	return driveResponseError(
		"drive/list_file_versions",
		"version_pagination_"+reason,
		fmt.Sprintf("版本预检无法证明分页已经完整，已停止操作（page=%d, cursor=%q）", page, cursor),
	)
}

func versionNumber(version map[string]any) (int, bool) {
	switch value := version["version"].(type) {
	case float64:
		return int(value), value == float64(int(value))
	case int:
		return value, true
	case string:
		var number int
		if _, err := fmt.Sscanf(value, "%d", &number); err == nil {
			return number, true
		}
	}
	return 0, false
}

func driveDownloadPayload(payload map[string]any, operation string) (string, string, map[string]string, error) {
	url := firstString(payload, "downloadUrl", "resourceUrl", "url")
	if url == "" {
		if resources, ok := payload["resourceUrls"].([]any); ok && len(resources) > 0 {
			if first, ok := resources[0].(map[string]any); ok {
				url = firstString(first, "url", "downloadUrl", "resourceUrl")
				payload = first
			}
		}
	}
	if url == "" {
		return "", "", nil, driveResponseError(operation, "missing_download_url", "下载响应没有有效 URL")
	}
	headers := map[string]string{}
	if raw, ok := payload["headers"].(map[string]any); ok {
		for key, value := range raw {
			if text, ok := value.(string); ok {
				headers[key] = text
			}
		}
	}
	return url, firstString(payload, "fileName", "name"), headers, nil
}
