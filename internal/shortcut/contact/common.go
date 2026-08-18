// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package contact

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/responsecheck"
)

const (
	contactCompositeReason   = "Reviewed Contact Shortcut adapter: the executable CLI owns strict success, collection, item, stable-identity, and unified-output validation."
	contactUnavailableReason = "Exact live-leaf probes across multiple authorized profiles could not safely produce a guaranteed zero-result fixture, so empty-result truth cannot be proved without guessing."
)

var contactReadSafety = contract.SafetySpec{
	Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
}

func contactCollectionResult(collection, description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","description":"当前响应中通过严格校验的项目数量"},%q:{"type":"array","description":%q,"items":{"type":"object","description":"带稳定身份的已校验项目","additionalProperties":true}}},"required":["count",%q],"additionalProperties":false}`,
			description, collection, description, collection,
		)),
	}
}

func contactObjectResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(`{"type":"object","description":%q,"additionalProperties":true}`, description)),
	}
}

func finalizeContactShortcut(item *shortcut.Shortcut, result *contract.ResultSpec, available bool) {
	item.OutputRollout = output.RolloutUnifiedActive
	item.Safety = contactReadSafety
	if item.Contract.Identity.Name == "" {
		name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(item.Command, "+"), "-", "_")
		examples := append([]string(nil), item.Tips...)
		parameters := make([]contract.ParamDecl, 0, len(item.Flags))
		for _, flag := range item.Flags {
			parameters = append(parameters, contract.ParamDecl{Name: flag.Name})
		}
		item.Contract = corecmd.ContractDecl{
			Description: item.Description,
			Identity: contract.ToolIdentitySpec{
				ProductID: "contact", Name: name, CanonicalPath: "contact." + name,
				CLIPath: "contact " + item.Command, PrimaryCLIPath: "contact " + item.Command,
			},
			Selection: contract.SelectionSpec{
				AgentSummary: item.Description, UseWhen: []string{item.Intent},
				AvoidWhen: []string{"该能力未公开；使用已审核的 Contact Shortcut 或对应原子命令"},
				Examples:  examples,
			},
			Parameters: parameters,
		}
	}
	item.Contract.Result = result
	availability := contract.InterfaceAvailable
	reason := contactCompositeReason
	if !available {
		availability = contract.InterfaceUnavailable
		reason = contactUnavailableReason
	}
	item.Contract.Interface = &contract.InterfaceSpec{
		Mode: contract.InterfaceModeComposite, Availability: availability, Reason: reason,
	}
}

func unavailableContact(operation string) error {
	return responsecheck.Error(operation, "capability_unavailable", contactUnavailableReason)
}

func contactEnvelope(data map[string]any, operation string) (map[string]any, error) {
	envelope, err := responsecheck.RequireSuccess(data, operation)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"errorCode", "errorMsg", "errorMessage", "error"} {
		if contactFailureValue(envelope[key]) {
			return nil, responsecheck.Error(operation, "conflicting_failure_evidence", "success=true 响应同时携带失败字段 "+key)
		}
	}
	return envelope, nil
}

func contactFailureValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed != "" && trimmed != "0" && !strings.EqualFold(trimmed, "success")
	case float64:
		return typed != 0
	case int:
		return typed != 0
	case bool:
		return typed
	case map[string]any:
		return len(typed) != 0
	case []any:
		return len(typed) != 0
	default:
		return true
	}
}

func strictUserSearch(data map[string]any, operation string, allowMissingResult bool) ([]map[string]any, error) {
	envelope, err := contactEnvelope(data, operation)
	if err != nil {
		return nil, err
	}
	if _, present := envelope["result"]; !present && allowMissingResult {
		return nil, responsecheck.Error(operation, "missing_result", "手机号搜索未返回显式 result；不能把缺失详情当作成功的空结果")
	}
	items, err := responsecheck.RequireObjectCollection(envelope, operation, "result")
	if err != nil {
		return nil, err
	}
	return projectUsers(items, operation)
}

func strictMobileSearch(data map[string]any, operation string) ([]map[string]any, error) {
	envelope, err := contactEnvelope(data, operation)
	if err != nil {
		return nil, err
	}
	raw, present := envelope["result"]
	if !present || raw == nil {
		return nil, responsecheck.Error(operation, "missing_result", "手机号搜索未返回显式 result；不能把缺失详情当作成功的空结果")
	}
	item, ok := raw.(map[string]any)
	if !ok || len(item) == 0 {
		return nil, responsecheck.Error(operation, "malformed_result", fmt.Sprintf("响应 result 应为非空用户对象，实际为 %T", raw))
	}
	users, err := projectUsers([]map[string]any{item}, operation)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func projectUsers(items []map[string]any, operation string) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(items))
	for index, item := range items {
		if contactString(item, "userId") == "" && contactString(item, "openDingTalkId") == "" {
			return nil, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("用户结果第 %d 项缺少 userId/openDingTalkId", index))
		}
		row := map[string]any{}
		for _, key := range []string{"name", "userId", "flowerName", "openDingTalkId", "title"} {
			if value, ok := item[key]; ok && value != nil {
				row[key] = value
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func strictMembers(data map[string]any, operation, path string) ([]map[string]any, error) {
	if _, err := contactEnvelope(data, operation); err != nil {
		return nil, err
	}
	items, err := responsecheck.RequireObjectCollection(data, operation, path)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for index, item := range items {
		user, ok := item["userInfo"].(map[string]any)
		if !ok || len(user) == 0 {
			return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("%s[%d].userInfo 应为非空对象", path, index))
		}
		id := contactString(user, "userId")
		if id == "" {
			return nil, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("%s[%d] 缺少 userId", path, index))
		}
		row := map[string]any{"userId": id}
		if name := contactString(user, "name"); name != "" {
			row["name"] = name
		}
		out = append(out, row)
	}
	return out, nil
}

func strictSubDepts(data map[string]any, operation string) ([]map[string]any, error) {
	if _, err := contactEnvelope(data, operation); err != nil {
		return nil, err
	}
	items, err := responsecheck.RequireObjectCollection(data, operation, "result")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for index, item := range items {
		id, ok := contactInt64(item["deptId"])
		if !ok || id <= 0 {
			return nil, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("部门结果第 %d 项缺少有效 deptId", index))
		}
		row := map[string]any{"deptId": id}
		if name := contactString(item, "deptName"); name != "" {
			row["deptName"] = name
		}
		out = append(out, row)
	}
	return out, nil
}

func contactString(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return strings.TrimSpace(value)
}

func contactInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		if typed != float64(int64(typed)) {
			return 0, false
		}
		return int64(typed), true
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
