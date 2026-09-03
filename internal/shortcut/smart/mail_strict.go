// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func smartMailError(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

func smartMailValidatePageSize(rt *shortcut.RuntimeContext, flag string, required bool) error {
	if !required && !rt.Changed(flag) {
		return nil
	}
	value := rt.Int(flag)
	if value < 1 || value > 100 {
		return apperrors.NewValidation(fmt.Sprintf("--%s 必须在 1-100 之间", flag))
	}
	return nil
}

func smartMailStringPageSize(rt *shortcut.RuntimeContext, flag, fallback string) (string, error) {
	if !rt.Changed(flag) {
		return fallback, nil
	}
	value, err := strconv.Atoi(rt.Str(flag))
	if err != nil || value < 1 || value > 100 {
		return "", apperrors.NewValidation(fmt.Sprintf("--%s 必须是 1-100 之间的整数", flag))
	}
	return strconv.Itoa(value), nil
}

func smartMailValidateStringPageSize(rt *shortcut.RuntimeContext, flag string) error {
	_, err := smartMailStringPageSize(rt, flag, "")
	return err
}

func smartMailValidateRequiredText(rt *shortcut.RuntimeContext, flag string) error {
	if strings.TrimSpace(rt.Str(flag)) == "" {
		return apperrors.NewValidation(fmt.Sprintf("--%s 不能为空", flag))
	}
	return nil
}

func smartMailSuccess(data map[string]any, operation string) error {
	if len(data) == 0 {
		return smartMailError(operation, "empty_tool_response", "服务返回空响应，无法证明结果确实为空")
	}
	switch value := data["success"].(type) {
	case bool:
		if value {
			return nil
		}
	case string:
		if value == "true" {
			return nil
		}
	}
	if _, present := data["success"]; !present {
		return smartMailError(operation, "missing_success", "响应缺少 success 业务状态")
	}
	return smartMailError(operation, "remote_failure", "服务未明确返回成功状态")
}

func smartMailLookup(data map[string]any, path string) (any, bool) {
	var current any = data
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func smartMailCollection(data map[string]any, operation, path string) ([]map[string]any, error) {
	if err := smartMailSuccess(data, operation); err != nil {
		return nil, err
	}
	value, present := smartMailLookup(data, path)
	if !present {
		return nil, smartMailError(operation, "missing_collection", fmt.Sprintf("成功响应缺少 %s 数组；不能把未知响应结构当作空结果", path))
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, smartMailError(operation, "malformed_collection", fmt.Sprintf("响应 %s 应为数组，实际为 %T", path, value))
	}
	items := make([]map[string]any, 0, len(raw))
	for index, item := range raw {
		object, ok := item.(map[string]any)
		if !ok || len(object) == 0 {
			return nil, smartMailError(operation, "malformed_item", fmt.Sprintf("响应 %s[%d] 不是非空对象", path, index))
		}
		items = append(items, object)
	}
	return items, nil
}

func smartMailRows(data map[string]any, operation, path string, identityKeys ...string) ([]map[string]any, error) {
	items, err := smartMailCollection(data, operation, path)
	if err != nil {
		return nil, err
	}
	for index, item := range items {
		if searchMailFirstString(item, identityKeys...) == "" {
			return nil, smartMailError(operation, "missing_item_identity", fmt.Sprintf("结果第 %d 项缺少稳定 ID", index))
		}
	}
	return items, nil
}

// smartMailSearchRows handles the one reviewed Mail-specific zero-hit wire
// shape observed from search_emails: total="0", terminal nextCursor="$", and
// exactly one placeholder whose only fields are null recipient metadata. It is
// deliberately narrower than a general "drop bad item" rule.
func smartMailSearchRows(data map[string]any, operation string) ([]map[string]any, error) {
	items, err := smartMailCollection(data, operation, "messages")
	if err != nil {
		return nil, err
	}
	total, totalOK := smartMailInt(data["total"])
	next, nextOK := data["nextCursor"].(string)
	if totalOK && total == 0 && nextOK && (next == "" || next == "$") {
		if len(items) == 0 {
			return items, nil
		}
		if len(items) == 1 && smartMailEmptySearchSentinel(items[0]) {
			return []map[string]any{}, nil
		}
		return nil, smartMailError(operation, "conflicting_empty_result", "total=0 的搜索响应包含真实或未审核的消息对象")
	}
	if totalOK && total < 0 {
		return nil, smartMailError(operation, "invalid_total", "邮件搜索 total 不能为负数")
	}
	for index, item := range items {
		if searchMailFirstString(item, "id") == "" {
			return nil, smartMailError(operation, "missing_item_identity", fmt.Sprintf("结果第 %d 项缺少稳定 ID", index))
		}
	}
	return items, nil
}

func smartMailEmptySearchSentinel(item map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	allowed := map[string]bool{"toRecipients": true, "ccRecipients": true, "bccRecipients": true, "replyTo": true, "tags": true}
	for key, value := range item {
		if !allowed[key] || value != nil {
			return false
		}
	}
	return true
}

func smartMailInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case float64:
		if typed != float64(int(typed)) {
			return 0, false
		}
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(typed)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func smartMailPage(data map[string]any, operation, prefix, currentCursor string) (bool, string, error) {
	container := data
	if prefix != "" {
		value, present := smartMailLookup(data, prefix)
		if !present {
			return false, "", smartMailError(operation, "missing_pagination", "响应缺少分页对象")
		}
		var ok bool
		container, ok = value.(map[string]any)
		if !ok {
			return false, "", smartMailError(operation, "malformed_pagination", "分页对象类型错误")
		}
	}
	raw, present := container["nextCursor"]
	next, ok := raw.(string)
	if !present || !ok {
		return false, "", smartMailError(operation, "missing_pagination", "响应缺少字符串 nextCursor")
	}
	if rawMore, present := container["hasMore"]; present {
		hasMore, ok := smartMailBool(rawMore)
		if !ok {
			return false, "", smartMailError(operation, "malformed_pagination", "hasMore 应为布尔值")
		}
		terminal := strings.TrimSpace(next) == "" || strings.TrimSpace(next) == "$"
		if hasMore == terminal {
			return false, "", smartMailError(operation, "conflicting_pagination", "hasMore 与 nextCursor 互相矛盾")
		}
		if !hasMore {
			return true, "", nil
		}
		if strings.TrimSpace(currentCursor) != "" && strings.TrimSpace(next) == strings.TrimSpace(currentCursor) {
			return false, "", smartMailError(operation, "stalled_pagination", "服务端返回了与当前请求相同的 nextCursor")
		}
		return false, next, nil
	}
	if strings.TrimSpace(next) == "" || strings.TrimSpace(next) == "$" {
		return true, "", nil
	}
	if strings.TrimSpace(currentCursor) != "" && strings.TrimSpace(next) == strings.TrimSpace(currentCursor) {
		return false, "", smartMailError(operation, "stalled_pagination", "服务端返回了与当前请求相同的 nextCursor")
	}
	return false, next, nil
}

func smartMailBool(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		if typed == "true" {
			return true, true
		}
		if typed == "false" {
			return false, true
		}
	}
	return false, false
}

func smartMailResult(collection, description string) *contract.ResultSpec {
	identity := smartMailIdentitySchema(collection)
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		SensitivePaths: smartMailSensitivePaths(collection),
		DataSchema: json.RawMessage(fmt.Sprintf(`{"type":"object","description":%q,"properties":{"count":{"type":"integer","minimum":0,"description":"当前响应中的有效记录数量"},%q:{"type":"array","description":%q,"items":{"type":"object","description":"严格校验后的邮件记录",%s,"additionalProperties":true}}},"required":["count",%q],"additionalProperties":false}`,
			description, collection, description, identity, collection)),
	}
}

func smartMailSensitivePaths(collection string) []string {
	switch collection {
	case "messages":
		return []string{"messages.subject", "messages.from", "messages.date"}
	case "mails":
		return []string{"mails.subject", "mails.from", "mails.date"}
	case "users":
		return []string{"users.name", "users.nickname", "users.email", "users.employeeNo", "users.jobTitle", "users.workLocation"}
	default:
		panic("unsupported smart Mail sensitive paths: " + collection)
	}
}

func smartMailIdentitySchema(collection string) string {
	switch collection {
	case "messages":
		return `"properties":{"messageId":{"type":"string","minLength":1,"description":"稳定邮件 ID"}},"required":["messageId"]`
	case "mails":
		return `"properties":{"threadId":{"type":"string","minLength":1,"description":"稳定邮件会话 ID"}},"required":["threadId"]`
	case "users":
		return `"properties":{"id":{"type":["string","number"],"description":"稳定邮箱用户 ID"},"email":{"type":"string","minLength":1,"description":"稳定邮箱地址身份"}},"anyOf":[{"required":["id"]},{"required":["email"]}]`
	default:
		panic("unsupported smart Mail Result identity: " + collection)
	}
}

func smartMailCursorPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:                  contract.PaginationKindCursor,
		CursorParameter:       "cursor",
		MetaPath:              contract.PaginationMetaPath,
		EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath:         contract.PaginationNextTokenPath,
	}
}

func hardenSmartMail(declaration *shortcut.Shortcut, collection, description string) {
	declaration.OutputRollout = output.RolloutUnifiedActive
	declaration.Contract.Result = smartMailResult(collection, description)
	declaration.Contract.Pagination = smartMailCursorPagination()
	declaration.Contract.Interface = &contract.InterfaceSpec{
		Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable,
		Reason: "Reviewed Mail Shortcut composite: strict success, collection, stable-ID and pagination validation are owned by the executable CLI.",
	}
}

func markSmartMailCompatibilityOnly(declaration *shortcut.Shortcut) {
	declaration.OutputRollout = output.RolloutLegacyOnly
	declaration.Contract.Result = nil
	declaration.Contract.Pagination = nil
	declaration.Contract.Interface = &contract.InterfaceSpec{
		Mode:         contract.InterfaceModeComposite,
		Availability: contract.InterfaceAvailable,
		Reason:       "Historical executable Schema compatibility: this command remains callable, but the reviewed Shortcut catalog keeps it non-public until its live-data proof gap is closed.",
	}
}

func smartMailPayload(collection string, items []map[string]any, complete bool, next string) map[string]any {
	payload := map[string]any{"count": len(items), collection: items, "complete": complete}
	if !complete {
		payload["nextCursor"] = next
	}
	return payload
}

func smartMailOutputPage(rt *shortcut.RuntimeContext, collection string, items []map[string]any, complete bool, next string) error {
	payload := smartMailPayload(collection, items, complete, next)
	if !output.UsesUnifiedResult(rt.Command()) {
		return rt.Output(payload)
	}
	pagination, err := output.NewPagination(complete, next)
	if err != nil {
		return smartMailError("mail/pagination", "invalid_pagination", err.Error())
	}
	business := map[string]any{"count": len(items), collection: items}
	meta := &output.Meta{Pagination: pagination, Count: output.NewCount(len(items))}
	return output.StoreResult(rt.Command().Context(), output.Success(business, output.WithMeta(meta)))
}
