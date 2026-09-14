// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package wiki

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const wikiCompositeReason = "Reviewed Wiki Shortcut composite: the executable CLI owns strict response validation, pagination projection, optional multi-step orchestration, read-back verification, and confirmation; no single MCP interface represents the complete command contract."

const wikiSpaceSearchCompositeReason = "Reviewed Wiki space-search compatibility adapter: the published workflow properties query/limit remain stable while execution translates them to search_wikiSpaces.keyword/pageSize; redirecting an existing non-empty property requires a versioned Schema migration."

func wikiContract(command, description, useWhen string, avoidWhen, examples []string, result *contract.ResultSpec, pagination *contract.PaginationSpec, params ...contract.ParamDecl) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	cliPath := "wiki " + command
	return corecmd.ContractDecl{
		Description: description, Parameters: params, Result: result, Pagination: pagination,
		Interface: &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: wikiCompositeReason},
		Selection: contract.SelectionSpec{AgentSummary: description, UseWhen: []string{useWhen}, AvoidWhen: avoidWhen, Examples: examples},
		Identity:  contract.ToolIdentitySpec{ProductID: "wiki", Name: name, CanonicalPath: "wiki." + name, CLIPath: cliPath, PrimaryCLIPath: cliPath},
	}
}

func wikiWithInterfaceReason(declared shortcut.Shortcut, reason string) shortcut.Shortcut {
	declared.Contract.Interface.Reason = reason
	return declared
}

func wikiObjectResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess}, DataSchema: json.RawMessage(fmt.Sprintf(`{"type":"object","description":%q,"additionalProperties":true}`, description))}
}

func wikiCollectionResult(collection, description string) *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess}, DataSchema: json.RawMessage(fmt.Sprintf(
		`{"type":"object","description":%q,"properties":{"count":{"type":"integer","description":"有效结果数量"},%q:{"type":"array","description":%q,"items":{"type":"object","description":"Wiki 业务条目","additionalProperties":true}},"nextCursor":{"type":"string","description":"下一页游标"},"hasMore":{"type":"boolean","description":"服务端是否仍有下一页"}},"required":["count",%q],"additionalProperties":true}`,
		description, collection, description, collection))}
}

func wikiSpaceListResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema: json.RawMessage(`{"type":"object","description":"带请求范围和分页完成证据的知识库列表","properties":{"count":{"type":"integer","description":"有效空间数量"},"requestedType":{"type":"string","description":"本次服务端查询使用的知识库类型范围","enum":["orgWikiSpace","myWikiSpace"]},"spaces":{"type":"array","description":"请求类型范围内的知识库条目","items":{"type":"object","description":"Wiki 空间条目","properties":{"workspaceId":{"type":"string","description":"知识库 workspaceId"},"name":{"type":"string","description":"知识库名称"},"spaceType":{"type":"string","description":"服务端实际返回的空间类型"}},"additionalProperties":true}},"nextCursor":{"type":"string","description":"下一页游标"},"hasMore":{"type":"boolean","description":"服务端是否仍有下一页"},"autoPageComplete":{"type":"boolean","description":"自动分页是否已取完当前类型范围"},"pagesFetched":{"type":"integer","description":"本次累计请求页数"}},"required":["count","requestedType","spaces"],"additionalProperties":true}`),
	}
}

func wikiSpaceCreateResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePartialFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"创建、读回并验证空间类型的知识库","properties":{"success":{"type":"boolean","description":"创建与读回是否成功"},"workspaceId":{"type":"string","description":"新知识库 workspaceId"},"space":{"type":"object","description":"创建后读回的知识库元数据","additionalProperties":true},"spaceType":{"type":"string","description":"经服务端详情或类型范围列表验证的知识库类型","enum":["orgWikiSpace","myWikiSpace"]},"spaceTypeVerified":{"type":"boolean","description":"空间类型是否具有服务端可复核证据"},"spaceTypeEvidence":{"type":"string","description":"空间类型证据来源"}},"required":["success","workspaceId","space","spaceType","spaceTypeVerified","spaceTypeEvidence"],"additionalProperties":true}`),
	}
}

func wikiNodeCollectionResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema: json.RawMessage(`{"type":"object","description":"带类型、层级和分页证据的 Wiki 节点集合","properties":{"count":{"type":"integer","description":"有效节点数量"},"nodes":{"type":"array","description":"服务端返回的 Wiki 节点","items":{"type":"object","description":"带稳定节点身份和层级字段的 Wiki 节点","properties":{"nodeId":{"type":"string","description":"稳定节点 ID"},"name":{"type":"string","description":"节点名称"},"type":{"type":"string","description":"服务端节点种类"},"extension":{"type":"string","description":"文件或在线文档扩展类型"},"contentType":{"type":"string","description":"服务端内容类型"},"hasChildren":{"type":"boolean","description":"服务端是否报告存在子节点"},"workspaceId":{"type":"string","description":"节点所属 workspaceId"},"folderId":{"type":"string","description":"服务端父文件夹 ID"},"parentFolderId":{"type":"string","description":"规范化父文件夹 ID"},"url":{"type":"string","description":"节点访问地址"}},"additionalProperties":true}},"nextCursor":{"type":"string","description":"下一页游标"},"hasMore":{"type":"boolean","description":"服务端是否仍有下一页"},"autoPageComplete":{"type":"boolean","description":"自动分页是否已完成"},"pagesFetched":{"type":"integer","description":"本次累计请求页数"}},"required":["count","nodes"],"additionalProperties":true}`),
	}
}

func wikiNodeCreateResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema: json.RawMessage(`{"type":"object","description":"创建后读回验证的 Wiki 节点","properties":{"success":{"type":"boolean","description":"节点是否通过创建后验证"},"nodeId":{"type":"string","description":"新节点 ID"},"workspaceId":{"type":"string","description":"已验证的目标 workspaceId"},"requestedType":{"type":"string","description":"请求创建的节点类型"},"node":{"type":"object","description":"创建后读回的节点元数据","additionalProperties":true}},"required":["success","nodeId","workspaceId","requestedType","node"],"additionalProperties":true}`),
	}
}

func wikiNodeCopyResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema: json.RawMessage(`{"type":"object","description":"源节点预检与副本读回组成的复制证据","properties":{"success":{"type":"boolean","description":"复制是否通过完整验证"},"sourceNodeId":{"type":"string","description":"源节点 ID"},"nodeId":{"type":"string","description":"新副本节点 ID"},"sourceWorkspaceId":{"type":"string","description":"源节点所属 workspaceId"},"targetWorkspaceId":{"type":"string","description":"已验证的副本 workspaceId"},"targetFolderId":{"type":"string","description":"副本读回的父文件夹 ID"},"source":{"type":"object","description":"复制前源节点元数据","additionalProperties":true},"copy":{"type":"object","description":"复制后副本元数据","additionalProperties":true}},"required":["success","sourceNodeId","nodeId","targetWorkspaceId","source","copy"],"additionalProperties":true}`),
	}
}

func wikiNodeMoveResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema: json.RawMessage(`{"type":"object","description":"移动前后读回和目标域验证证据","properties":{"success":{"type":"boolean","description":"移动是否通过完整验证"},"nodeId":{"type":"string","description":"被移动节点 ID"},"targetDomain":{"type":"string","description":"已请求并验证的目标域"},"sourceWorkspaceId":{"type":"string","description":"移动前 workspaceId"},"targetWorkspaceId":{"type":"string","description":"移动后 workspaceId"},"targetFolderId":{"type":"string","description":"移动后父文件夹 ID"},"source":{"type":"object","description":"移动前节点元数据","additionalProperties":true},"target":{"type":"object","description":"移动后节点元数据","additionalProperties":true},"node":{"type":"object","description":"兼容字段，等同 target","additionalProperties":true},"targetSpace":{"type":"object","description":"我的文档目标空间的服务端证据","additionalProperties":true}},"required":["success","nodeId","targetDomain","sourceWorkspaceId","targetWorkspaceId","source","target","node"],"additionalProperties":true}`),
	}
}

func wikiCursorPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "cursor", MetaPath: contract.PaginationMetaPath, EndpointExhaustedPath: contract.PaginationExhaustedPath, NextTokenPath: contract.PaginationNextTokenPath}
}

func requireWikiResponse(data map[string]any, operation string) (map[string]any, error) {
	if len(data) == 0 {
		return nil, wikiResponseError(operation, "empty_tool_response", "服务返回空响应，无法证明操作成功")
	}
	if value, present := data["success"]; present {
		success, ok := value.(bool)
		if !ok {
			return nil, wikiResponseError(operation, "malformed_success", "响应 success 字段不是布尔值")
		}
		if !success {
			message := firstWikiString(data, "errorMsg", "message", "error")
			if message == "" {
				message = "服务明确返回 success=false"
			}
			return nil, wikiResponseError(operation, "remote_failure", message)
		}
	}
	return data, nil
}

func requireWikiWrite(data map[string]any, operation string) (map[string]any, error) {
	data, err := requireWikiResponse(data, operation)
	if err != nil {
		return nil, err
	}
	if success, ok := data["success"].(bool); !ok || !success {
		return nil, wikiResponseError(operation, "missing_terminal_success", "写操作响应没有 success=true 终态证据")
	}
	return data, nil
}

func requireWikiObject(data map[string]any, operation string) (map[string]any, error) {
	data, err := requireWikiResponse(data, operation)
	if err != nil {
		return nil, err
	}
	for _, wrapper := range []string{"result", "data"} {
		if value, present := data[wrapper]; present {
			object, ok := value.(map[string]any)
			if !ok || len(object) == 0 {
				return nil, wikiResponseError(operation, "malformed_object", "响应业务对象缺失或畸形")
			}
			return object, nil
		}
	}
	if len(data) == 1 {
		return nil, wikiResponseError(operation, "missing_business_result", "响应没有可验证的业务对象")
	}
	return data, nil
}

func requireWikiCollection(data map[string]any, operation string, keys ...string) ([]any, map[string]any, error) {
	data, err := requireWikiResponse(data, operation)
	if err != nil {
		return nil, nil, err
	}
	containers := []map[string]any{data}
	for _, wrapper := range []string{"result", "data"} {
		if value, present := data[wrapper]; present {
			inner, ok := value.(map[string]any)
			if !ok {
				return nil, nil, wikiResponseError(operation, "malformed_envelope", fmt.Sprintf("响应 %s 字段不是对象", wrapper))
			}
			containers = append(containers, inner)
		}
	}
	for _, container := range containers {
		for _, key := range keys {
			value, present := container[key]
			if !present {
				continue
			}
			items, ok := value.([]any)
			if !ok {
				return nil, nil, wikiResponseError(operation, "malformed_collection", fmt.Sprintf("响应 %s 字段不是数组", key))
			}
			for index, item := range items {
				if _, ok := item.(map[string]any); !ok {
					return nil, nil, wikiResponseError(operation, "malformed_collection_item", fmt.Sprintf("响应 %s[%d] 不是对象", key, index))
				}
			}
			return items, container, nil
		}
	}
	return nil, nil, wikiResponseError(operation, "missing_collection", "响应缺少声明的业务数组；不能把缺字段或内部错误投影成空结果")
}

func projectWikiRows(items []any, aliases map[string][]string) []map[string]any {
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		source := item.(map[string]any)
		row := make(map[string]any)
		for canonical, candidates := range aliases {
			for _, candidate := range candidates {
				if value, ok := source[candidate]; ok && value != nil {
					row[canonical] = value
					break
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func projectWikiRowsPreservingSource(items []any, aliases map[string][]string) []map[string]any {
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		source := item.(map[string]any)
		row := make(map[string]any, len(source)+len(aliases))
		for key, value := range source {
			row[key] = value
		}
		for canonical, candidates := range aliases {
			if value, ok := row[canonical]; ok && value != nil {
				continue
			}
			for _, candidate := range candidates {
				if value, ok := source[candidate]; ok && value != nil {
					row[canonical] = value
					break
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func addWikiPagination(out, page map[string]any) {
	for _, pair := range [][2]string{{"nextCursor", "nextCursor"}, {"nextToken", "nextCursor"}, {"nextPageToken", "nextCursor"}, {"pageToken", "nextCursor"}, {"hasMore", "hasMore"}, {"truncated", "truncated"}, {"totalCount", "totalCount"}, {"autoPageComplete", "autoPageComplete"}, {"autoPageStopReason", "autoPageStopReason"}, {"pagesFetched", "pagesFetched"}} {
		if value, ok := page[pair[0]]; ok && value != nil {
			out[pair[1]] = value
		}
	}
}

func firstWikiString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nestedWikiString(data map[string]any, keys ...string) string {
	if value := firstWikiString(data, keys...); value != "" {
		return value
	}
	for _, wrapper := range []string{"result", "data"} {
		if inner, ok := data[wrapper].(map[string]any); ok {
			if value := firstWikiString(inner, keys...); value != "" {
				return value
			}
		}
	}
	return ""
}

func verifyWikiSpaceType(rt *shortcut.RuntimeContext, workspaceID string, space map[string]any) (string, string, error) {
	directTypes := make(map[string]string, 1)
	for _, key := range []string{"spaceType", "wikiSpaceType", "type"} {
		value := firstWikiString(space, key)
		if value == "orgWikiSpace" || value == "myWikiSpace" {
			directTypes[value] = "get_wikiSpace." + key
		}
	}
	if len(directTypes) == 1 {
		for spaceType, evidence := range directTypes {
			return spaceType, evidence, nil
		}
	}
	if len(directTypes) > 1 {
		return "", "", wikiResponseError("wiki/get_wikiSpace", "space_type_conflict", "空间详情同时返回互相冲突的组织和个人知识库类型")
	}

	matches := make([]string, 0, 1)
	for _, spaceType := range []string{"orgWikiSpace", "myWikiSpace"} {
		found, err := wikiSpaceExistsInType(rt, workspaceID, spaceType)
		if err != nil {
			return "", "", err
		}
		if found {
			matches = append(matches, spaceType)
		}
	}
	if len(matches) == 1 {
		return matches[0], "list_wikiSpaces." + matches[0] + ".workspaceId_match", nil
	}
	if len(matches) > 1 {
		return "", "", wikiResponseError("wiki/list_wikiSpaces", "space_type_conflict", "同一 workspaceId 同时出现在组织和个人知识库范围，无法确定空间类型")
	}
	return "", "", wikiResponseError("wiki/list_wikiSpaces", "space_type_not_found", "已完整查询组织和个人知识库范围，但都未找到新建 workspaceId")
}

func wikiSpaceExistsInType(rt *shortcut.RuntimeContext, workspaceID, spaceType string) (bool, error) {
	const pageLimit = 20
	cursor := ""
	seen := map[string]struct{}{}
	for pageNumber := 1; pageNumber <= pageLimit; pageNumber++ {
		params := map[string]any{"wikiSpaceType": spaceType, "pageSize": 50}
		if cursor != "" {
			params["pageToken"] = cursor
		}
		data, err := rt.CallMCPData("wiki", "list_wikiSpaces", params)
		if err != nil {
			return false, err
		}
		items, page, err := requireWikiCollection(data, "wiki/list_wikiSpaces", "wikiSpaces", "spaces")
		if err != nil {
			return false, err
		}
		for _, item := range items {
			candidate := item.(map[string]any)
			if firstWikiString(candidate, "workspaceId", "spaceId", "id") == workspaceID {
				return true, nil
			}
		}
		hasMore, present := page["hasMore"]
		if !present {
			return false, wikiResponseError("wiki/list_wikiSpaces", "missing_has_more", "空间类型验证要求每页响应提供 hasMore 布尔值")
		}
		more, ok := hasMore.(bool)
		if !ok {
			return false, wikiResponseError("wiki/list_wikiSpaces", "malformed_has_more", "空间类型验证响应的 hasMore 不是布尔值")
		}
		if !more {
			return false, nil
		}
		next := firstWikiString(page, "nextCursor", "nextToken", "nextPageToken", "pageToken")
		if next == "" {
			return false, wikiResponseError("wiki/list_wikiSpaces", "missing_next_cursor", "空间类型验证仍有下一页但响应缺少游标")
		}
		if next == cursor {
			return false, wikiResponseError("wiki/list_wikiSpaces", "stalled_cursor", "空间类型验证的下一页游标未变化")
		}
		if _, exists := seen[next]; exists {
			return false, wikiResponseError("wiki/list_wikiSpaces", "cyclic_cursor", "空间类型验证的分页游标形成循环")
		}
		seen[next] = struct{}{}
		cursor = next
	}
	return false, wikiResponseError("wiki/list_wikiSpaces", "page_limit_reached", "空间类型验证超过 20 页安全上限")
}

func wikiSpaceTypeVerificationError(workspaceID string, space map[string]any, cause error) error {
	return apperrors.NewAPI(
		"知识库已经创建并读回，但空间类型验证未完成；为避免重复创建，请使用 workspaceId 继续核查",
		apperrors.WithOperation("wiki/create_wikiSpace"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("verify_space_type"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithReason("space_type_unverified"),
		apperrors.WithActions("不要重复创建知识库", "使用 workspaceId 分别查询 orgWikiSpace 和 myWikiSpace 范围"),
		apperrors.WithDetails(map[string]any{
			"status":              "partial_success",
			"workspaceId":         workspaceID,
			"space":               space,
			"spaceTypeVerified":   false,
			"spaceTypeEvidence":   "unverified",
			"verificationFailure": cause.Error(),
		}),
		apperrors.WithCause(cause),
	)
}

func wikiStringInt(rt *shortcut.RuntimeContext, name string, fallback, min, max int) (int, error) {
	raw := rt.Str(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("--%s 必须是 %d-%d 之间的整数", name, min, max)
	}
	return value, nil
}

func wikiStringSliceFirst(rt *shortcut.RuntimeContext, primary string, aliases ...string) []string {
	if rt.Changed(primary) {
		return rt.StrSlice(primary)
	}
	for _, alias := range aliases {
		if rt.Changed(alias) {
			values, _ := rt.Command().Flags().GetStringSlice(alias)
			return values
		}
	}
	return rt.StrSlice(primary)
}

func wikiResponseError(operation, reason, message string) error {
	return apperrors.NewAPI(message, apperrors.WithOperation(operation), apperrors.WithOrigin("mcp"), apperrors.WithFailureStage("response_validation"), apperrors.WithRetryable(false), apperrors.WithReason(reason))
}

func wikiPageLimitError(operation string, pagesFetched int, items []any, nextCursor string) error {
	return apperrors.NewAPI(
		"达到 --page-limit 时服务端仍有下一页；已保留当前结果和 nextCursor，可从该游标继续读取",
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithReason("page_limit_reached"),
		apperrors.WithActions("使用 details.nextCursor 继续读取", "提高 --page-limit 后重新执行"),
		apperrors.WithDetails(map[string]any{
			"status":       "partial_success",
			"complete":     false,
			"pagesFetched": pagesFetched,
			"itemsFetched": len(items),
			"items":        items,
			"nextCursor":   nextCursor,
		}),
	)
}

func wikiReadSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
}
func wikiWriteSafety(confirm bool) contract.SafetySpec {
	confirmation := "not_required"
	if confirm {
		confirmation = "user_required"
	}
	return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: confirmation, Idempotency: "unknown"}
}
func wikiDeleteSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required", Idempotency: "unknown"}
}

func wikiAutoPageFlags() []shortcut.Flag {
	const evidence = "--max-items/--page-delay 仅与 --page-all 一起使用；值必须大于等于 0"
	return append([]shortcut.Flag{{Name: "page-all", Type: shortcut.FlagBool, Desc: "自动沿游标取完所有页。" + evidence}, {Name: "page-limit", Type: shortcut.FlagInt, Default: "20", Desc: "自动翻页最多请求页数。" + evidence}}, shortcut.AutoPageControlFlags()...)
}

func enableWikiAutoPage(item *shortcut.Shortcut) {
	item.Flags = append(item.Flags, wikiAutoPageFlags()...)
	item.Constraints = append(item.Constraints, shortcut.AutoPageControlConstraints()...)
	item.Validate = func(rt *shortcut.RuntimeContext) error {
		if err := shortcut.ValidateAutoPageControls(rt); err != nil {
			return err
		}
		if rt.Bool("page-all") && rt.Int("page-limit") < 1 {
			return fmt.Errorf("--page-limit 必须大于 0")
		}
		return nil
	}
}

type wikiPageFetcher func(cursor string, pageSize int) (map[string]any, error)

func collectWikiPages(rt *shortcut.RuntimeContext, operation string, pageSize int, keys []string, fetch wikiPageFetcher) ([]any, map[string]any, error) {
	if !rt.Bool("page-all") {
		data, err := fetch(rt.StrFirst("cursor", "page-token"), pageSize)
		if err != nil {
			return nil, nil, err
		}
		return requireWikiCollection(data, operation, keys...)
	}
	all := make([]any, 0)
	cursor := rt.StrFirst("cursor", "page-token")
	seenCursors := make(map[string]struct{})
	if cursor != "" {
		seenCursors[cursor] = struct{}{}
	}
	lastPage := map[string]any{}
	for pageNumber := 1; pageNumber <= rt.Int("page-limit"); pageNumber++ {
		requestSize := shortcut.AutoPageRequestSize(rt, pageSize, len(all))
		data, err := fetch(cursor, requestSize)
		if err != nil {
			return nil, nil, err
		}
		items, page, err := requireWikiCollection(data, operation, keys...)
		if err != nil {
			return nil, nil, err
		}
		maxItems := rt.Int("max-items")
		if maxItems > 0 {
			remaining := maxItems - len(all)
			if len(items) > remaining {
				items = items[:remaining]
			}
		}
		all = append(all, items...)
		lastPage = page
		if maxItems > 0 && len(all) >= maxItems {
			lastPage["autoPageComplete"] = false
			lastPage["autoPageStopReason"] = "max_items"
			lastPage["pagesFetched"] = pageNumber
			return all, lastPage, nil
		}
		hasMore, present := page["hasMore"]
		if !present {
			return nil, nil, wikiResponseError(operation, "missing_has_more", "--page-all 要求每页响应提供 hasMore 布尔值")
		}
		more, ok := hasMore.(bool)
		if !ok {
			return nil, nil, wikiResponseError(operation, "malformed_has_more", "分页响应 hasMore 不是布尔值")
		}
		if !more {
			lastPage["autoPageComplete"] = true
			lastPage["pagesFetched"] = pageNumber
			return all, lastPage, nil
		}
		next := firstWikiString(page, "nextCursor", "nextToken", "nextPageToken", "pageToken")
		if next == "" {
			return nil, nil, wikiResponseError(operation, "missing_next_cursor", "hasMore=true 但响应缺少下一页游标")
		}
		if next == cursor {
			return nil, nil, wikiResponseError(operation, "stalled_cursor", "下一页游标未变化，已停止以避免死循环")
		}
		if _, seen := seenCursors[next]; seen {
			return nil, nil, wikiResponseError(operation, "cyclic_cursor", "下一页游标形成循环，已停止以避免重复请求")
		}
		if pageNumber == rt.Int("page-limit") {
			return nil, nil, wikiPageLimitError(operation, pageNumber, all, next)
		}
		seenCursors[next] = struct{}{}
		cursor = next
		if err := shortcut.WaitAutoPageDelay(rt); err != nil {
			return nil, nil, err
		}
	}
	return nil, nil, wikiResponseError(operation, "page_limit_reached", "达到 --page-limit 时服务端仍有下一页；提高页数上限或使用返回游标续传")
}
