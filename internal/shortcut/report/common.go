// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package report

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const reportCompositeReason = "Reviewed Report Shortcut composite: the executable CLI owns strict business-success validation, exact collection paths, stable report/template identities, truthful pagination, local filtering, output projection, and multi-step detail verification."

type reportPageEvidence struct {
	HasMore bool
	Next    string
}

func reportReadSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
}

func reportCollectionResult(collection, description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","description":"当前页严格验证的日志数量"},%q:{"type":"array","description":%q,"items":{"type":"object","description":"具有稳定 reportId 的日志摘要","properties":{"reportId":{"type":"string","description":"稳定日志 ID"},"templateName":{"type":"string","description":"日志模板名称"},"creatorName":{"type":"string","description":"日志创建人显示名"},"creatorUserId":{"type":"string","description":"日志创建人稳定用户 ID"},"createTime":{"type":"integer","description":"日志创建时间毫秒值"},"modifiedTime":{"type":"integer","description":"日志修改时间毫秒值"}},"required":["reportId"],"additionalProperties":false}},"complete":{"type":"boolean","description":"服务端分页证据是否证明当前查询已结束"}},"required":["count",%q,"complete"],"additionalProperties":false}`,
			description, collection, description, collection,
		)),
		SensitivePaths: []string{collection + ".templateName", collection + ".creatorName", collection + ".creatorUserId"},
	}
}

func reportTemplateSearchResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema:     json.RawMessage(`{"type":"object","description":"严格验证的日志模板搜索结果","properties":{"count":{"type":"integer","description":"匹配模板数量"},"templates":{"type":"array","description":"名称匹配且具有稳定 ID 的模板","items":{"type":"object","description":"一条日志模板摘要","properties":{"templateId":{"type":"string","description":"稳定模板 ID"},"name":{"type":"string","description":"模板名称"},"lastModifiedTime":{"type":"integer","description":"模板最后修改时间毫秒值"}},"required":["templateId","name"],"additionalProperties":false}}},"required":["count","templates"],"additionalProperties":false}`),
		SensitivePaths: []string{"templates.name"},
	}
}

func reportLatestResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema:     json.RawMessage(`{"type":"object","description":"严格验证的最近日志详情","properties":{"report":{"type":"object","description":"稳定身份匹配的日志详情","properties":{"reportId":{"type":"string","description":"稳定日志 ID"},"title":{"type":"string","description":"日志标题"},"templateName":{"type":"string","description":"日志模板名称"},"creatorName":{"type":"string","description":"日志创建人显示名"},"createTime":{"type":"integer","description":"日志创建时间毫秒值"},"modifiedTime":{"type":"integer","description":"日志修改时间毫秒值"},"fields":{"type":"array","description":"严格验证的日志字段","items":{"type":"object","description":"日志字段值","properties":{"key":{"type":"string","description":"字段名称"},"value":{"type":"string","description":"字段内容"},"sort":{"type":"integer","description":"字段排序值"},"type":{"type":"integer","description":"字段类型码"}},"required":["key","value","sort","type"],"additionalProperties":false}}},"required":["reportId","fields"],"additionalProperties":false}},"required":["report"],"additionalProperties":false}`),
		SensitivePaths: []string{"report.title", "report.templateName", "report.creatorName", "report.fields.value"},
	}
}

func reportPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:                  contract.PaginationKindCursor,
		CursorParameter:       "cursor",
		MetaPath:              contract.PaginationMetaPath,
		EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath:         contract.PaginationNextTokenPath,
	}
}

func reportContract(command, description, intent string, result *contract.ResultSpec, pagination *contract.PaginationSpec, params []contract.ParamDecl, examples ...string) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	cliPath := "report " + command
	return corecmd.ContractDecl{
		Description: description,
		Result:      result,
		Pagination:  pagination,
		Parameters:  params,
		Identity: contract.ToolIdentitySpec{
			ProductID: "report", Name: name, CanonicalPath: "report." + name,
			CLIPath: cliPath, PrimaryCLIPath: cliPath,
		},
		Interface: &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: reportCompositeReason},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{intent},
			AvoidWhen:    []string{"钉钉在线文档使用 doc，待办使用 todo，OA 审批使用 oa；缺少稳定日志或模板身份时不要猜测"},
			Examples:     examples,
		},
	}
}

func reportResponseError(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

func reportRequireSuccess(data map[string]any, operation string) error {
	if len(data) == 0 {
		return reportResponseError(operation, "empty_tool_response", "服务返回空响应，无法证明 Report 调用成功或结果确实为空")
	}
	raw, present := data["success"]
	if !present {
		return reportResponseError(operation, "missing_success", "Report 响应缺少 success 业务状态")
	}
	success, ok := raw.(bool)
	if !ok {
		return reportResponseError(operation, "invalid_success_type", fmt.Sprintf("Report success 应为布尔值，实际为 %T", raw))
	}
	if success {
		return nil
	}
	message := "Report 服务明确返回失败"
	for _, key := range []string{"errorMsg", "errorMessage", "message"} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			message = strings.TrimSpace(value)
			break
		}
	}
	return reportResponseError(operation, "remote_failure", message)
}

func reportObjectCollection(value any, operation, path string) ([]map[string]any, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, reportResponseError(operation, "malformed_collection", fmt.Sprintf("Report %s 应为数组，实际为 %T", path, value))
	}
	items := make([]map[string]any, 0, len(raw))
	for index, item := range raw {
		object, ok := item.(map[string]any)
		if !ok || len(object) == 0 {
			return nil, reportResponseError(operation, "malformed_item", fmt.Sprintf("Report %s[%d] 不是非空对象", path, index))
		}
		items = append(items, object)
	}
	return items, nil
}

func reportListCollection(data map[string]any, operation string) ([]map[string]any, map[string]any, error) {
	if err := reportRequireSuccess(data, operation); err != nil {
		return nil, nil, err
	}
	result, present := data["result"]
	if !present {
		return nil, nil, reportResponseError(operation, "missing_collection", "Report 成功响应缺少 result 日志集合")
	}
	if _, ok := result.([]any); ok {
		items, err := reportObjectCollection(result, operation, "result")
		return items, data, err
	}
	container, ok := result.(map[string]any)
	if !ok || container == nil {
		return nil, nil, reportResponseError(operation, "malformed_collection", fmt.Sprintf("Report result 应为数组或带 report_list 的对象，实际为 %T", result))
	}
	raw, present := container["report_list"]
	if !present {
		return nil, nil, reportResponseError(operation, "missing_collection", "Report 成功响应缺少 result.report_list 数组")
	}
	items, err := reportObjectCollection(raw, operation, "result.report_list")
	return items, container, err
}

func reportRequiredString(item map[string]any, operation string, index int, keys ...string) (string, error) {
	var selected string
	for _, key := range keys {
		raw, present := item[key]
		if !present || raw == nil {
			continue
		}
		value, ok := raw.(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" {
			return "", reportResponseError(operation, "malformed_item", fmt.Sprintf("Report 结果第 %d 项的 %s 必须是非空字符串", index, key))
		}
		if selected != "" && selected != value {
			return "", reportResponseError(operation, "conflicting_item_identity", fmt.Sprintf("Report 结果第 %d 项的身份字段冲突", index))
		}
		selected = value
	}
	if selected == "" {
		return "", reportResponseError(operation, "missing_item_identity", fmt.Sprintf("Report 结果第 %d 项缺少稳定身份", index))
	}
	return selected, nil
}

func reportOptionalString(item map[string]any, operation string, index int, keys ...string) (string, error) {
	for _, key := range keys {
		raw, present := item[key]
		if !present || raw == nil {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return "", reportResponseError(operation, "malformed_item", fmt.Sprintf("Report 结果第 %d 项的 %s 应为字符串，实际为 %T", index, key, raw))
		}
		return strings.TrimSpace(value), nil
	}
	return "", nil
}

func reportInteger(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed > math.MaxInt64 || typed < math.MinInt64 {
			return 0, false
		}
		return int64(typed), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func reportOptionalInteger(item map[string]any, operation string, index int, keys ...string) (int64, bool, error) {
	for _, key := range keys {
		raw, present := item[key]
		if !present || raw == nil {
			continue
		}
		value, ok := reportInteger(raw)
		if !ok {
			return 0, false, reportResponseError(operation, "malformed_item", fmt.Sprintf("Report 结果第 %d 项的 %s 应为整数", index, key))
		}
		return value, true, nil
	}
	return 0, false, nil
}

func reportProjectEntries(data map[string]any, operation string) ([]map[string]any, reportPageEvidence, error) {
	items, pageContainer, err := reportListCollection(data, operation)
	if err != nil {
		return nil, reportPageEvidence{}, err
	}
	entries := make([]map[string]any, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		id, identityErr := reportRequiredString(item, operation, index, "reportId", "report_id", "report_Id")
		if identityErr != nil {
			return nil, reportPageEvidence{}, identityErr
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, reportPageEvidence{}, reportResponseError(operation, "duplicate_item_identity", "当前 Report 页包含重复 reportId")
		}
		seen[id] = struct{}{}
		projected := map[string]any{"reportId": id}
		for target, aliases := range map[string][]string{
			"templateName":  {"templateName", "template_name", "reportTemplateName", "report_template_name"},
			"creatorName":   {"creatorName", "creator_name", "creatorUserName", "senderName", "authorName"},
			"creatorUserId": {"creatorUserId", "creator_user_id", "creatorId", "senderUserId", "userId"},
		} {
			value, valueErr := reportOptionalString(item, operation, index, aliases...)
			if valueErr != nil {
				return nil, reportPageEvidence{}, valueErr
			}
			if value != "" {
				projected[target] = value
			}
		}
		for target, aliases := range map[string][]string{
			"createTime":   {"createTime", "create_time", "gmtCreate", "sendTime"},
			"modifiedTime": {"modifiedTime", "modified_time", "gmtModified", "modifyTime"},
		} {
			value, present, valueErr := reportOptionalInteger(item, operation, index, aliases...)
			if valueErr != nil {
				return nil, reportPageEvidence{}, valueErr
			}
			if present {
				projected[target] = value
			}
		}
		entries = append(entries, projected)
	}
	page, err := reportPage(data, pageContainer, operation, len(entries))
	if err != nil {
		return nil, reportPageEvidence{}, err
	}
	return entries, page, nil
}

func reportPage(data, result map[string]any, operation string, count int) (reportPageEvidence, error) {
	raw, present := data["hasMore"]
	if nested, exists := result["hasMore"]; exists {
		if present && !sameReportScalar(raw, nested) {
			return reportPageEvidence{}, reportResponseError(operation, "conflicting_pagination", "Report 顶层与 result.hasMore 冲突")
		}
		raw, present = nested, true
	}
	if !present {
		return reportPageEvidence{}, reportResponseError(operation, "missing_pagination", "Report 列表缺少 hasMore，不能把当前页宣称为完整结果")
	}
	hasMore, ok := raw.(bool)
	if !ok {
		return reportPageEvidence{}, reportResponseError(operation, "malformed_pagination", "Report hasMore 应为布尔值")
	}
	page := reportPageEvidence{HasMore: hasMore}
	next, nextPresent, err := reportNextCursor(data, result, operation)
	if err != nil {
		return reportPageEvidence{}, err
	}
	if !hasMore {
		// The Report service echoes an integer cursor on terminal pages. It is a
		// page receipt, not a continuation: hasMore=false is authoritative and
		// the unified projection intentionally omits next_token. Parsing above
		// still rejects wrong types and conflicting cursor fields.
		return page, nil
	}
	if count == 0 {
		return reportPageEvidence{}, reportResponseError(operation, "empty_page_with_continuation", "Report 空页仍声明 hasMore=true")
	}
	if !nextPresent || next <= 0 {
		return reportPageEvidence{}, reportResponseError(operation, "missing_next_cursor", "hasMore=true 时必须返回正整数 continuation cursor")
	}
	page.Next = strconv.FormatInt(next, 10)
	return page, nil
}

func reportNextCursor(data, result map[string]any, operation string) (int64, bool, error) {
	var selected int64
	found := false
	for _, object := range []map[string]any{data, result} {
		for _, key := range []string{"nextCursor", "cursor"} {
			raw, present := object[key]
			if !present || raw == nil {
				continue
			}
			value, ok := reportInteger(raw)
			if !ok {
				return 0, false, reportResponseError(operation, "malformed_pagination", fmt.Sprintf("Report %s 应为整数", key))
			}
			if found && selected != value {
				return 0, false, reportResponseError(operation, "conflicting_pagination", "Report continuation cursor 字段冲突")
			}
			selected, found = value, true
		}
	}
	return selected, found, nil
}

func sameReportScalar(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func reportProjectTemplates(data map[string]any, operation string) ([]map[string]any, error) {
	if err := reportRequireSuccess(data, operation); err != nil {
		return nil, err
	}
	raw, present := data["items"]
	if !present {
		return nil, reportResponseError(operation, "missing_collection", "Report 模板响应缺少 items 数组")
	}
	items, err := reportObjectCollection(raw, operation, "items")
	if err != nil {
		return nil, err
	}
	templates := make([]map[string]any, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		id, identityErr := reportRequiredString(item, operation, index, "report_template_id")
		if identityErr != nil {
			return nil, identityErr
		}
		name, nameErr := reportRequiredString(item, operation, index, "report_template_name")
		if nameErr != nil {
			return nil, nameErr
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, reportResponseError(operation, "duplicate_item_identity", "Report 模板响应包含重复 templateId")
		}
		seen[id] = struct{}{}
		projected := map[string]any{"templateId": id, "name": name}
		modified, present, modifiedErr := reportOptionalInteger(item, operation, index, "last_modified_time")
		if modifiedErr != nil {
			return nil, modifiedErr
		}
		if present {
			projected["lastModifiedTime"] = modified
		}
		templates = append(templates, projected)
	}
	return templates, nil
}

func reportProjectDetail(data map[string]any, operation, expectedID string) (map[string]any, error) {
	if err := reportRequireSuccess(data, operation); err != nil {
		return nil, err
	}
	result, ok := data["result"].(map[string]any)
	if !ok || len(result) == 0 {
		return nil, reportResponseError(operation, "missing_result", "Report 详情成功响应缺少非空 result 对象")
	}
	id, err := reportRequiredString(result, operation, 0, "report_Id", "reportId", "report_id")
	if err != nil {
		return nil, err
	}
	if id != expectedID {
		return nil, reportResponseError(operation, "identity_mismatch", "Report 详情 reportId 与请求目标不一致")
	}
	projected := map[string]any{"reportId": id}
	for target, aliases := range map[string][]string{
		"title":        {"report_name", "reportName", "title"},
		"templateName": {"report_template_name", "templateName"},
		"creatorName":  {"creatorName", "senderName"},
	} {
		value, valueErr := reportOptionalString(result, operation, 0, aliases...)
		if valueErr != nil {
			return nil, valueErr
		}
		if value != "" {
			projected[target] = value
		}
	}
	for target, aliases := range map[string][]string{
		"createTime": {"createTime", "create_time"}, "modifiedTime": {"modifiedTime", "modified_time"},
	} {
		value, present, valueErr := reportOptionalInteger(result, operation, 0, aliases...)
		if valueErr != nil {
			return nil, valueErr
		}
		if present {
			projected[target] = value
		}
	}
	rawFields, present := result["report_content"]
	if !present {
		return nil, reportResponseError(operation, "missing_collection", "Report 详情缺少 result.report_content 数组")
	}
	fields, err := reportObjectCollection(rawFields, operation, "result.report_content")
	if err != nil {
		return nil, err
	}
	projectedFields := make([]map[string]any, 0, len(fields))
	for index, field := range fields {
		key, keyErr := reportRequiredString(field, operation, index, "key")
		if keyErr != nil {
			return nil, keyErr
		}
		rawValue, valuePresent := field["value"]
		value, valueOK := rawValue.(string)
		if !valuePresent || !valueOK {
			return nil, reportResponseError(operation, "malformed_item", fmt.Sprintf("Report 详情字段第 %d 项的 value 必须是字符串", index))
		}
		sortValue, sortOK := reportInteger(field["sort"])
		typeValue, typeOK := reportInteger(field["type"])
		if !sortOK || !typeOK {
			return nil, reportResponseError(operation, "malformed_item", fmt.Sprintf("Report 详情字段第 %d 项的 sort/type 必须是整数", index))
		}
		projectedFields = append(projectedFields, map[string]any{"key": key, "value": value, "sort": sortValue, "type": typeValue})
	}
	projected["fields"] = projectedFields
	return projected, nil
}

func outputReportPage(rt *shortcut.RuntimeContext, collection string, entries []map[string]any, page reportPageEvidence) error {
	payload := map[string]any{"count": len(entries), collection: entries, "complete": !page.HasMore}
	if !output.UsesUnifiedResult(rt.Command()) {
		if page.HasMore {
			payload["nextCursor"] = page.Next
		}
		return rt.Output(payload)
	}
	pagination, err := output.NewPagination(!page.HasMore, page.Next)
	if err != nil {
		return reportResponseError("report/pagination", "invalid_pagination", err.Error())
	}
	meta := &output.Meta{Count: output.NewCount(len(entries)), Pagination: pagination}
	return output.StoreResult(rt.Command().Context(), output.Success(payload, output.WithMeta(meta)))
}
