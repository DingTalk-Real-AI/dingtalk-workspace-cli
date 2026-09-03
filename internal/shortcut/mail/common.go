// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package mail

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const mailCompositeReason = "Reviewed Mail Shortcut composite: the executable CLI owns strict response validation, pagination evidence, stable-identity checks, output projection, and confirmation; no single MCP interface represents the complete command contract."

const mailCompatibilityInterfaceReason = "Historical executable Schema compatibility: this command remains callable, but the reviewed Shortcut catalog keeps it non-public until its live-data or cleanup proof gap is closed."

func mailCollectionResult(collection, description string) *contract.ResultSpec {
	identity := mailCollectionIdentitySchema(collection)
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		SensitivePaths: mailCollectionSensitivePaths(collection),
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","minimum":0,"description":"当前响应中的有效记录数量"},%q:{"type":"array","description":%q,"items":{"type":"object","description":"严格校验后的邮件业务记录",%s,"additionalProperties":true}}},"required":["count",%q],"additionalProperties":false}`,
			description, collection, description, identity, collection,
		)),
	}
}

func mailCollectionSensitivePaths(collection string) []string {
	switch collection {
	case "threads":
		return []string{"threads.subject", "threads.lastUpdated"}
	case "folders":
		return []string{"folders.name"}
	case "tags":
		return []string{"tags.name"}
	case "users":
		return []string{"users.name", "users.email", "users.employeeNo"}
	case "templates":
		return []string{"templates.name", "templates.subject"}
	case "contacts":
		return []string{"contacts.contactEmail", "contacts.displayName"}
	case "messages":
		return []string{"messages.subject", "messages.body", "messages.markdownBody", "messages.from", "messages.toRecipients", "messages.ccRecipients", "messages.bccRecipients", "messages.attachments", "messages.receivedDateTime", "messages.sentDateTime"}
	default:
		panic("unsupported Mail collection sensitive paths: " + collection)
	}
}

func mailCollectionIdentitySchema(collection string) string {
	switch collection {
	case "threads":
		return `"properties":{"conversationId":{"type":"string","minLength":1,"description":"稳定邮件会话 ID"}},"required":["conversationId"]`
	case "folders", "tags", "templates", "contacts", "messages":
		return `"properties":{"id":{"type":"string","minLength":1,"description":"稳定邮件业务对象 ID"}},"required":["id"]`
	case "users":
		return `"properties":{"userId":{"type":["string","number"],"description":"稳定邮箱用户 ID"},"email":{"type":"string","minLength":1,"description":"稳定邮箱地址身份"}},"anyOf":[{"required":["userId"]},{"required":["email"]}]`
	default:
		panic("unsupported Mail collection Result identity: " + collection)
	}
}

func mailCursorPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:                  contract.PaginationKindCursor,
		CursorParameter:       "cursor",
		MetaPath:              contract.PaginationMetaPath,
		EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath:         contract.PaginationNextTokenPath,
	}
}

func mailObjectResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"value":{"type":"object","description":"严格校验且身份匹配的邮件业务对象","properties":{"id":{"type":"string","minLength":1,"description":"稳定邮件业务对象 ID"}},"required":["id"],"additionalProperties":true}},"required":["value"],"additionalProperties":false}`,
			description,
		)),
		SensitivePaths: []string{"value.body", "value.markdownBody", "value.subject", "value.summary", "value.name", "value.email", "value.contactEmail", "value.displayName", "value.from", "value.toRecipients", "value.ccRecipients", "value.bccRecipients", "value.senders", "value.attachments", "value.receivedDateTime", "value.sentDateTime"},
	}
}

func mailReadSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
}

func mailValidatePageSize(rt *shortcut.RuntimeContext, flag string, required bool) error {
	if !required && !rt.Changed(flag) {
		return nil
	}
	value := rt.Int(flag)
	if value < 1 || value > 100 {
		return apperrors.NewValidation(fmt.Sprintf("--%s 必须在 1-100 之间", flag))
	}
	return nil
}

func mailStringPageSize(rt *shortcut.RuntimeContext, flag string, required bool) (string, error) {
	if !required && !rt.Changed(flag) {
		return "", nil
	}
	value, err := strconv.Atoi(rt.Str(flag))
	if err != nil || value < 1 || value > 100 {
		return "", apperrors.NewValidation(fmt.Sprintf("--%s 必须是 1-100 之间的整数", flag))
	}
	return strconv.Itoa(value), nil
}

func mailValidateStringPageSize(rt *shortcut.RuntimeContext, flag string, required bool) error {
	_, err := mailStringPageSize(rt, flag, required)
	return err
}

func mailValidateRequiredText(rt *shortcut.RuntimeContext, flag string) error {
	if strings.TrimSpace(rt.Str(flag)) == "" {
		return apperrors.NewValidation(fmt.Sprintf("--%s 不能为空", flag))
	}
	return nil
}

func mailValidateThreadList(rt *shortcut.RuntimeContext) error {
	if err := mailValidatePageSize(rt, "limit", true); err != nil {
		return err
	}
	parsed := make(map[string]time.Time, 2)
	for _, flag := range []string{"start", "end"} {
		if !rt.Changed(flag) {
			continue
		}
		value, err := time.Parse(time.RFC3339, strings.TrimSpace(rt.Str(flag)))
		if err != nil {
			return apperrors.NewValidation(fmt.Sprintf("--%s 必须是 RFC3339 时间", flag))
		}
		_, offset := value.Zone()
		if offset != 0 {
			return apperrors.NewValidation(fmt.Sprintf("--%s 必须使用 UTC 时区", flag))
		}
		parsed[flag] = value
	}
	start, hasStart := parsed["start"]
	end, hasEnd := parsed["end"]
	if hasStart && hasEnd && end.Before(start) {
		return apperrors.NewValidation("--end 不能早于 --start")
	}
	return nil
}

func mailWriteSafety(idempotency string) contract.SafetySpec {
	return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: idempotency}
}

func mailReadContract(command, description, intent string, result *contract.ResultSpec, flags []contract.ParamDecl, examples ...string) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	cliPath := "mail " + command
	return corecmd.ContractDecl{
		Description: description,
		Result:      result,
		Identity: contract.ToolIdentitySpec{
			ProductID: "mail", Name: name, CanonicalPath: "mail." + name,
			CLIPath: cliPath, PrimaryCLIPath: cliPath,
		},
		Interface: &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: mailCompositeReason},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{intent},
			AvoidWhen:    []string{"只需摘要列表时使用 mail +triage 或 +recent-mail；需要原始响应时改用对应原子命令"},
			Examples:     examples,
		},
		Parameters: flags,
	}
}

func mailWriteContract(command, description, intent string, flags []contract.ParamDecl, examples ...string) corecmd.ContractDecl {
	contractDecl := mailReadContract(command, description, intent, mailObjectResult(description), flags, examples...)
	contractDecl.Selection.AvoidWhen = []string{"用户尚未确认写入内容或目标时不要执行；只需读取时使用对应只读 shortcut"}
	return contractDecl
}

func mailResponseError(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

// mailRequireSuccess accepts only the two success encodings observed from the
// Mail backend: boolean true and the exact string "true". Missing, false, or
// malformed status values fail closed.
func mailRequireSuccess(data map[string]any, operation string) error {
	if len(data) == 0 {
		return mailResponseError(operation, "empty_tool_response", "服务返回空响应，无法证明调用成功或结果确实为空")
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
		return mailResponseError(operation, "missing_success", "响应缺少 success 业务状态")
	}
	message := mailFirstString(data, "errorMessage", "errorMsg", "message", "error")
	if message == "" {
		message = "服务未明确返回成功状态"
	}
	return mailResponseError(operation, "remote_failure", message)
}

func mailRequireCollection(data map[string]any, operation, path string) ([]map[string]any, error) {
	if err := mailRequireSuccess(data, operation); err != nil {
		return nil, err
	}
	value, present := mailLookup(data, path)
	if !present {
		return nil, mailResponseError(operation, "missing_collection", fmt.Sprintf("成功响应缺少 %s 数组；不能把未知响应结构当作空结果", path))
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, mailResponseError(operation, "malformed_collection", fmt.Sprintf("响应 %s 应为数组，实际为 %T", path, value))
	}
	items := make([]map[string]any, 0, len(raw))
	for index, item := range raw {
		object, ok := item.(map[string]any)
		if !ok || len(object) == 0 {
			return nil, mailResponseError(operation, "malformed_item", fmt.Sprintf("响应 %s[%d] 不是非空对象", path, index))
		}
		items = append(items, object)
	}
	return items, nil
}

func mailRequireObject(data map[string]any, operation, path string) (map[string]any, error) {
	if err := mailRequireSuccess(data, operation); err != nil {
		return nil, err
	}
	value, present := mailLookup(data, path)
	if !present || value == nil {
		return nil, mailResponseError(operation, "missing_result", fmt.Sprintf("成功响应缺少非空 %s 对象", path))
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) == 0 {
		return nil, mailResponseError(operation, "malformed_result", fmt.Sprintf("响应 %s 应为非空对象，实际为 %T", path, value))
	}
	return object, nil
}

func mailRequireIdentity(object map[string]any, operation, expected string, keys ...string) error {
	actual := mailFirstString(object, keys...)
	if actual == "" {
		return mailResponseError(operation, "missing_stable_id", "业务对象缺少稳定 ID")
	}
	if expected != "" && actual != expected {
		return mailResponseError(operation, "identity_mismatch", "业务对象 ID 与请求目标不一致")
	}
	return nil
}

func mailValidateRows(items []map[string]any, operation string, identityKeys ...string) error {
	allowNumericID := false
	for _, key := range identityKeys {
		if key == "email" {
			allowNumericID = true
			break
		}
	}
	for index, item := range items {
		valid := mailFirstString(item, identityKeys...) != ""
		if !valid && allowNumericID {
			valid = mailNumericIdentity(item["id"])
		}
		if !valid {
			return mailResponseError(operation, "missing_item_identity", fmt.Sprintf("结果第 %d 项缺少稳定 ID", index))
		}
	}
	return nil
}

func mailNumericIdentity(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return true
	default:
		return false
	}
}

func mailProjectValue(operation, outputName string, value any) (any, bool, error) {
	if value == nil {
		return nil, false, nil
	}
	switch outputName {
	case "email":
		text, ok := value.(string)
		if !ok {
			return nil, false, mailResponseError(operation, "malformed_item_identity", "邮箱用户 email 必须是字符串")
		}
		text = strings.TrimSpace(text)
		return text, text != "", nil
	case "userId":
		if text, ok := value.(string); ok {
			text = strings.TrimSpace(text)
			return text, text != "", nil
		}
		if !mailNumericIdentity(value) {
			return nil, false, mailResponseError(operation, "malformed_item_identity", "邮箱用户 userId 必须是字符串或数字")
		}
		return value, true, nil
	default:
		if text, ok := value.(string); ok {
			if strings.TrimSpace(text) == "" {
				return nil, false, nil
			}
			return text, true, nil
		}
		return value, true, nil
	}
}

func mailProjectCollection(data map[string]any, operation, path string, identityKeys []string, fields map[string][]string) ([]map[string]any, error) {
	items, err := mailRequireCollection(data, operation, path)
	if err != nil {
		return nil, err
	}
	if err := mailValidateRows(items, operation, identityKeys...); err != nil {
		return nil, err
	}
	projected := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row := make(map[string]any, len(fields))
		for outputName, candidates := range fields {
			for _, candidate := range candidates {
				if value, present := item[candidate]; present && value != nil {
					projectedValue, include, err := mailProjectValue(operation, outputName, value)
					if err != nil {
						return nil, err
					}
					if !include {
						continue
					}
					row[outputName] = projectedValue
					break
				}
			}
		}
		projected = append(projected, row)
	}
	return projected, nil
}

func mailPage(data map[string]any, operation, prefix, currentCursor string) (bool, string, error) {
	container := data
	if prefix != "" {
		value, present := mailLookup(data, prefix)
		if !present {
			return false, "", mailResponseError(operation, "missing_pagination", "分页响应缺少已审核的分页对象")
		}
		var ok bool
		container, ok = value.(map[string]any)
		if !ok {
			return false, "", mailResponseError(operation, "malformed_pagination", "分页容器不是对象")
		}
	}
	nextValue, nextPresent := container["nextCursor"]
	next, nextOK := nextValue.(string)
	if !nextPresent || !nextOK {
		return false, "", mailResponseError(operation, "missing_pagination", "响应缺少字符串 nextCursor，无法证明结果是否完整")
	}
	if raw, present := container["hasMore"]; present {
		hasMore, ok := mailBool(raw)
		if !ok {
			return false, "", mailResponseError(operation, "malformed_pagination", "hasMore 应为布尔值")
		}
		terminal := strings.TrimSpace(next) == "" || strings.TrimSpace(next) == "$"
		if hasMore && terminal {
			return false, "", mailResponseError(operation, "missing_next_cursor", "hasMore=true 但 nextCursor 为空")
		}
		if !hasMore && !terminal {
			return false, "", mailResponseError(operation, "conflicting_pagination", "hasMore=false 但 nextCursor 非空")
		}
		if !hasMore {
			return true, "", nil
		}
		if strings.TrimSpace(currentCursor) != "" && strings.TrimSpace(next) == strings.TrimSpace(currentCursor) {
			return false, "", mailResponseError(operation, "stalled_pagination", "服务端返回了与当前请求相同的 nextCursor")
		}
		return false, next, nil
	}
	if strings.TrimSpace(next) == "" || strings.TrimSpace(next) == "$" {
		return true, "", nil
	}
	if strings.TrimSpace(currentCursor) != "" && strings.TrimSpace(next) == strings.TrimSpace(currentCursor) {
		return false, "", mailResponseError(operation, "stalled_pagination", "服务端返回了与当前请求相同的 nextCursor")
	}
	return false, next, nil
}

func mailBool(value any) (bool, bool) {
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

func mailLookup(object map[string]any, path string) (any, bool) {
	var current any = object
	for _, segment := range strings.Split(path, ".") {
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapped[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func mailFirstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mailCollectionPayload(collection string, items []map[string]any, complete bool, next string) map[string]any {
	payload := map[string]any{"count": len(items), collection: items, "complete": complete}
	if !complete {
		payload["nextCursor"] = next
	}
	return payload
}

func mailBusinessCollectionPayload(collection string, items []map[string]any) map[string]any {
	return map[string]any{"count": len(items), collection: items}
}

// mailOutputPage preserves legacy payload compatibility while keeping cursor
// controls in the unified envelope's meta.pagination contract.
func mailOutputPage(rt *shortcut.RuntimeContext, collection string, items []map[string]any, complete bool, next string) error {
	payload := mailCollectionPayload(collection, items, complete, next)
	if !output.UsesUnifiedResult(rt.Command()) {
		return rt.Output(payload)
	}
	pagination, err := output.NewPagination(complete, next)
	if err != nil {
		return mailResponseError("mail/pagination", "invalid_pagination", err.Error())
	}
	business := map[string]any{"count": len(items), collection: items}
	meta := &output.Meta{Pagination: pagination, Count: output.NewCount(len(items))}
	return output.StoreResult(rt.Command().Context(), output.Success(business, output.WithMeta(meta)))
}

func hardenPublicMailContracts() {
	collections := []struct {
		declaration *shortcut.Shortcut
		collection  string
		description string
		paginated   bool
	}{
		{&ThreadList, "threads", "严格校验的邮件会话列表", true},
		{&FolderList, "folders", "严格校验的邮箱文件夹列表", false},
		{&TagList, "tags", "严格校验的邮件标签列表", false},
		{&UserSearch, "users", "严格校验的企业邮箱用户搜索结果", true},
		{&TemplateList, "templates", "严格校验的邮件模板列表", true},
		{&ContactList, "contacts", "严格校验的邮件联系人列表", true},
	}
	for _, item := range collections {
		item.declaration.OutputRollout = output.RolloutUnifiedActive
		item.declaration.Contract.Result = mailCollectionResult(item.collection, item.description)
		if item.paginated {
			item.declaration.Contract.Pagination = mailCursorPagination()
		}
		item.declaration.Contract.Interface = &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: mailCompositeReason}
	}
	markMailCompatibilityOnly(&ThreadList)
	markMailCompatibilityOnly(&TagList)
	markMailCompatibilityOnly(&TemplateList)
	markMailCompatibilityOnly(&ContactList)
}

func markMailCompatibilityOnly(declaration *shortcut.Shortcut) {
	declaration.OutputRollout = output.RolloutLegacyOnly
	declaration.Contract.Result = nil
	declaration.Contract.Pagination = nil
	declaration.Contract.Interface = &contract.InterfaceSpec{
		Mode:         contract.InterfaceModeComposite,
		Availability: contract.InterfaceAvailable,
		Reason:       mailCompatibilityInterfaceReason,
	}
}

func markMailUnavailable(declaration *shortcut.Shortcut, reason string) {
	declaration.OutputRollout = output.RolloutLegacyOnly
	declaration.Contract.Result = nil
	declaration.Contract.Pagination = nil
	declaration.Contract.Interface = &contract.InterfaceSpec{
		Mode:         contract.InterfaceModeComposite,
		Availability: contract.InterfaceUnavailable,
		Reason:       reason,
	}
}
