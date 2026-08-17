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
		cursor = next
		if err := shortcut.WaitAutoPageDelay(rt); err != nil {
			return nil, nil, err
		}
	}
	return nil, nil, wikiResponseError(operation, "page_limit_reached", "达到 --page-limit 时服务端仍有下一页；提高页数上限或使用返回游标续传")
}
