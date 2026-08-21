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
	contactCompositeReason              = "Reviewed Contact Shortcut adapter: the executable CLI owns strict success, collection, item, stable-identity, and unified-output validation."
	contactCompatibilityInterfaceReason = "Historical CLI and full-Schema compatibility: this executable shortcut remains callable with its published interface shape, but semantic catalog policy keeps it unavailable to Agents until strict live evidence is complete."
	contactUnavailableReason            = "Exact live-leaf probes across multiple authorized profiles could not safely produce a guaranteed zero-result fixture, so empty-result truth cannot be proved without guessing."
	contactRosterGapReason              = "Exact raw roster probes are blocked by the current authorization or platform capability before a typed business response is available."
)

var contactReadSafety = contract.SafetySpec{
	Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
}

func markContactCompatibilityOnly(declaration *shortcut.Shortcut) {
	declaration.OutputRollout = output.RolloutLegacyOnly
	declaration.Contract.Result = nil
	declaration.Contract.Pagination = nil
	declaration.Contract.Interface = &contract.InterfaceSpec{
		Mode:         contract.InterfaceModeComposite,
		Availability: contract.InterfaceAvailable,
		Reason:       contactCompatibilityInterfaceReason,
	}
}

func contactCollectionResult(collection, description string) *contract.ResultSpec {
	itemSchema := `{"type":"object","description":"带稳定身份的已校验项目","additionalProperties":true}`
	sensitive := []string{}
	switch collection {
	case "followings":
		itemSchema = `{"type":"object","description":"带稳定开放用户身份的特别关注联系人","properties":{"openDingTalkId":{"type":"string","minLength":1,"description":"稳定开放用户 ID"}},"required":["openDingTalkId"],"additionalProperties":false}`
		sensitive = []string{"followings.openDingTalkId"}
	case "users":
		itemSchema = `{"type":"object","description":"至少带 userId 或 openDingTalkId 之一的通讯录用户","properties":{"name":{"type":"string","description":"用户姓名"},"userId":{"type":"string","minLength":1,"description":"稳定用户 ID"},"flowerName":{"type":"string","description":"用户花名"},"openDingTalkId":{"type":"string","minLength":1,"description":"稳定开放用户 ID"},"title":{"type":"string","description":"职位名称"}},"additionalProperties":false}`
		sensitive = []string{"users.name", "users.userId", "users.flowerName", "users.openDingTalkId", "users.title"}
	case "members":
		itemSchema = `{"type":"object","description":"带稳定用户身份的成员","properties":{"userId":{"type":"string","minLength":1,"description":"稳定用户 ID"},"name":{"type":"string","description":"成员姓名"}},"required":["userId"],"additionalProperties":false}`
		sensitive = []string{"members.userId", "members.name"}
	case "depts":
		itemSchema = `{"type":"object","description":"带稳定部门身份的直属子部门","properties":{"deptId":{"type":"integer","minimum":1,"description":"稳定部门 ID"},"deptName":{"type":"string","description":"部门名称"}},"required":["deptId"],"additionalProperties":false}`
		sensitive = []string{"depts.deptName"}
	}
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","minimum":0,"description":"当前响应中通过严格校验的项目数量"},%q:{"type":"array","description":%q,"items":%s}},"required":["count",%q],"additionalProperties":false}`,
			description, collection, description, itemSchema, collection,
		)),
		SensitivePaths: sensitive,
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
		item.OutputRollout = output.RolloutLegacyOnly
		item.Contract.Result = nil
	}
	item.Contract.Interface = &contract.InterfaceSpec{
		Mode: contract.InterfaceModeComposite, Availability: availability, Reason: reason,
	}
}

func validateContactNonBlank(rt *shortcut.RuntimeContext, operation string, flags ...string) error {
	for _, flag := range flags {
		if strings.TrimSpace(rt.Str(flag)) == "" {
			return responsecheck.Error(operation, "empty_parameter", "--"+flag+" 不能为空白")
		}
	}
	return nil
}

func validateContactMobile(rt *shortcut.RuntimeContext, operation, flag string) error {
	if _, err := normalizeContactMobile(rt.Str(flag)); err != nil {
		return responsecheck.Error(operation, "invalid_mobile", "--"+flag+" 必须是至少 6 位数字的手机号，可包含国家码、空格、连字符或括号")
	}
	return nil
}

func validateContactOptionalNonBlank(rt *shortcut.RuntimeContext, operation, flag string) error {
	if rt.Changed(flag) && strings.TrimSpace(rt.Str(flag)) == "" {
		return responsecheck.Error(operation, "empty_parameter", "--"+flag+" 显式传入时不能为空白")
	}
	return nil
}

func validateContactPositiveStringID(rt *shortcut.RuntimeContext, operation, flag string) error {
	value := strings.TrimSpace(rt.Str(flag))
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return responsecheck.Error(operation, "invalid_parameter", "--"+flag+" 必须为正整数")
	}
	return nil
}

func validateContactPositiveInt(rt *shortcut.RuntimeContext, operation, flag string) error {
	if rt.Int(flag) <= 0 {
		return responsecheck.Error(operation, "invalid_parameter", "--"+flag+" 必须大于 0")
	}
	return nil
}

func validateContactPositiveIDList(rt *shortcut.RuntimeContext, operation, flag string) error {
	values := rt.StrSlice(flag)
	if len(values) == 0 {
		return responsecheck.Error(operation, "empty_parameter", "--"+flag+" 至少需要一个正整数 ID")
	}
	seen := make(map[int64]bool, len(values))
	for _, value := range values {
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || parsed <= 0 {
			return responsecheck.Error(operation, "invalid_parameter", "--"+flag+" 每项都必须为正整数")
		}
		if seen[parsed] {
			return responsecheck.Error(operation, "duplicate_parameter", "--"+flag+" 不能包含重复 ID")
		}
		seen[parsed] = true
	}
	return nil
}

func normalizedContactIDList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strings.TrimSpace(value))
	}
	return out
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

// strictMobileLookup decodes the dedicated exact-mobile interface. The live
// service returns one result object for a match and omits result for a
// successful no-match. That omission is reviewed only for this exact lookup;
// null, empty objects, arrays, and malformed identities remain failures.
func strictMobileLookup(data map[string]any, operation string) (map[string]any, bool, error) {
	envelope, err := contactEnvelope(data, operation)
	if err != nil {
		return nil, false, err
	}
	raw, present := envelope["result"]
	if !present {
		return nil, false, nil
	}
	result, ok := raw.(map[string]any)
	if !ok || len(result) == 0 {
		return nil, false, responsecheck.Error(operation, "malformed_result", "手机号精确查询 result 必须是非空对象")
	}
	userID := contactString(result, "userId")
	if userID == "" {
		return nil, false, responsecheck.Error(operation, "missing_stable_identity", "手机号精确查询结果缺少非空 userId")
	}
	row := map[string]any{"userId": userID}
	if name, present, valid := contactOptionalString(result, "orgUserName"); !valid {
		return nil, false, responsecheck.Error(operation, "malformed_optional_field", "手机号精确查询 orgUserName 必须是字符串")
	} else if present && name != "" {
		row["name"] = name
	}
	return row, true, nil
}

func normalizeContactMobile(value string) (string, error) {
	return normalizeContactMobilePart(value, 6)
}

func normalizeContactMobilePart(value string, minimumDigits int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("empty mobile")
	}
	var digits strings.Builder
	for index, r := range trimmed {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '+' && index == 0:
		case r == ' ' || r == '-' || r == '(' || r == ')':
		default:
			return "", fmt.Errorf("invalid mobile character")
		}
	}
	normalized := digits.String()
	if strings.HasPrefix(normalized, "00") {
		normalized = strings.TrimPrefix(normalized, "00")
	}
	if len(normalized) < minimumDigits {
		return "", fmt.Errorf("mobile too short")
	}
	return normalized, nil
}

func strictFollowings(data map[string]any, operation, expectedOpenID string) ([]map[string]any, error) {
	envelope, err := contactEnvelope(data, operation)
	if err != nil {
		return nil, err
	}
	result, ok := envelope["result"].(map[string]any)
	if !ok || result == nil {
		return nil, responsecheck.Error(operation, "malformed_result", fmt.Sprintf("响应 result 应为对象，实际为 %T", envelope["result"]))
	}
	raw, present := result["models"]
	if !present {
		return nil, responsecheck.Error(operation, "missing_collection", "响应缺少显式 result.models 集合")
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, responsecheck.Error(operation, "malformed_collection", fmt.Sprintf("响应 result.models 应为数组，实际为 %T", raw))
	}
	out := make([]map[string]any, 0, len(items))
	seen := make(map[string]bool, len(items))
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("响应 result.models[%d] 应为对象，实际为 %T", index, rawItem))
		}
		openID := contactString(item, "openDingTalkId")
		if openID == "" {
			openID = contactString(item, "openDingtalkId")
		}
		if openID == "" {
			return nil, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("响应 result.models[%d] 缺少 openDingTalkId", index))
		}
		if seen[openID] {
			return nil, responsecheck.Error(operation, "duplicate_stable_identity", fmt.Sprintf("响应包含重复 openDingTalkId（索引 %d）", index))
		}
		seen[openID] = true
		if expectedOpenID == "" || openID == expectedOpenID {
			out = append(out, map[string]any{"openDingTalkId": openID})
		}
	}
	return out, nil
}

func strictRoles(data map[string]any, operation string) ([]map[string]any, error) {
	envelope, err := contactEnvelope(data, operation)
	if err != nil {
		return nil, err
	}
	rawGroups, present := envelope["result"]
	if !present {
		return nil, responsecheck.Error(operation, "missing_collection", "响应缺少显式 result 角色分组集合")
	}
	groups, ok := rawGroups.([]any)
	if !ok {
		return nil, responsecheck.Error(operation, "malformed_collection", fmt.Sprintf("响应 result 应为数组，实际为 %T", rawGroups))
	}
	out := make([]map[string]any, 0)
	seen := map[int64]bool{}
	for groupIndex, rawGroup := range groups {
		group, ok := rawGroup.(map[string]any)
		if !ok || group == nil {
			return nil, responsecheck.Error(operation, "malformed_group", fmt.Sprintf("响应 result[%d] 应为对象，实际为 %T", groupIndex, rawGroup))
		}
		groupName := contactString(group, "groupName")
		if groupName == "" {
			return nil, responsecheck.Error(operation, "malformed_group", fmt.Sprintf("响应 result[%d] 缺少 groupName", groupIndex))
		}
		rawLabels, present := group["labels"]
		if !present {
			return nil, responsecheck.Error(operation, "missing_collection", fmt.Sprintf("响应 result[%d] 缺少 labels 集合", groupIndex))
		}
		labels, ok := rawLabels.([]any)
		if !ok {
			return nil, responsecheck.Error(operation, "malformed_collection", fmt.Sprintf("响应 result[%d].labels 应为数组，实际为 %T", groupIndex, rawLabels))
		}
		for labelIndex, rawLabel := range labels {
			label, ok := rawLabel.(map[string]any)
			if !ok || label == nil {
				return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("响应 result[%d].labels[%d] 应为对象，实际为 %T", groupIndex, labelIndex, rawLabel))
			}
			id, ok := contactInt64(label["labelId"])
			name := contactString(label, "name")
			if !ok || id <= 0 || name == "" {
				return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("响应 result[%d].labels[%d] 缺少有效 labelId/name", groupIndex, labelIndex))
			}
			if seen[id] {
				return nil, responsecheck.Error(operation, "duplicate_stable_identity", fmt.Sprintf("响应包含重复 labelId（分组 %d，索引 %d）", groupIndex, labelIndex))
			}
			seen[id] = true
			out = append(out, map[string]any{"labelId": id, "name": name, "groupName": groupName})
		}
	}
	return out, nil
}

func projectUsers(items []map[string]any, operation string) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(items))
	seen := make(map[string]bool, len(items))
	for index, item := range items {
		userID := contactString(item, "userId")
		openID := contactString(item, "openDingTalkId")
		if userID == "" && openID == "" {
			return nil, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("用户结果第 %d 项缺少 userId/openDingTalkId", index))
		}
		identity := userID
		if identity == "" {
			identity = openID
		}
		if seen[identity] {
			return nil, responsecheck.Error(operation, "duplicate_stable_identity", fmt.Sprintf("用户结果第 %d 项重复稳定身份", index))
		}
		seen[identity] = true
		row := map[string]any{}
		for _, key := range []string{"name", "userId", "flowerName", "openDingTalkId", "title"} {
			value, present, valid := contactOptionalString(item, key)
			if !valid {
				return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("用户结果第 %d 项的 %s 必须是字符串", index, key))
			}
			if present && value != "" {
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
	seen := make(map[string]bool, len(items))
	for index, item := range items {
		user, ok := item["userInfo"].(map[string]any)
		if !ok || len(user) == 0 {
			return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("%s[%d].userInfo 应为非空对象", path, index))
		}
		id := contactString(user, "userId")
		if id == "" {
			return nil, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("%s[%d] 缺少 userId", path, index))
		}
		if seen[id] {
			return nil, responsecheck.Error(operation, "duplicate_stable_identity", fmt.Sprintf("%s[%d] 重复 userId", path, index))
		}
		seen[id] = true
		row := map[string]any{"userId": id}
		name, _, valid := contactOptionalString(user, "name")
		if !valid {
			return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("%s[%d].userInfo.name 必须是字符串", path, index))
		}
		if name != "" {
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
	seen := make(map[int64]bool, len(items))
	for index, item := range items {
		id, ok := contactInt64(item["deptId"])
		if !ok || id <= 0 {
			return nil, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("部门结果第 %d 项缺少有效 deptId", index))
		}
		if seen[id] {
			return nil, responsecheck.Error(operation, "duplicate_stable_identity", fmt.Sprintf("部门结果第 %d 项重复 deptId", index))
		}
		seen[id] = true
		row := map[string]any{"deptId": id}
		name, _, valid := contactOptionalString(item, "deptName")
		if !valid {
			return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("部门结果第 %d 项的 deptName 必须是字符串", index))
		}
		if name != "" {
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

func contactOptionalString(object map[string]any, key string) (string, bool, bool) {
	raw, present := object[key]
	if !present || raw == nil {
		return "", present, true
	}
	value, ok := raw.(string)
	if !ok {
		return "", true, false
	}
	return strings.TrimSpace(value), true, true
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
