// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/responsecheck"
)

func contactSmartResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(`{"type":"object","description":%q,"additionalProperties":true}`, description)),
	}
}

func finalizeContactSmart(item *shortcut.Shortcut) {
	item.OutputRollout = output.RolloutUnifiedActive
	item.Contract.Result = contactSmartResult(item.Description)
}

func strictContactEnvelope(data map[string]any, operation string) (map[string]any, error) {
	envelope, err := responsecheck.RequireSuccess(data, operation)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"errorCode", "errorMsg", "errorMessage", "error"} {
		if strictContactFailure(envelope[key]) {
			return nil, responsecheck.Error(operation, "conflicting_failure_evidence", "success=true 响应同时携带失败字段 "+key)
		}
	}
	return envelope, nil
}

func strictContactFailure(value any) bool {
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

func strictResolveContactUser(rt *shortcut.RuntimeContext, name string) (contactUser, error) {
	data, err := rt.CallMCPData("contact", "search_contact_by_key_word", map[string]any{"keyword": name})
	if err != nil {
		return contactUser{}, err
	}
	if _, err := strictContactEnvelope(data, "contact/search_contact_by_key_word"); err != nil {
		return contactUser{}, err
	}
	items, err := responsecheck.RequireObjectCollection(data, "contact/search_contact_by_key_word", "result")
	if err != nil {
		return contactUser{}, err
	}
	users := make([]contactUser, 0, len(items))
	for index, item := range items {
		user := contactUser{
			userID:         strictContactString(item, "userId"),
			openDingTalkID: strictContactString(item, "openDingTalkId"),
			name:           strictContactString(item, "name"),
		}
		if user.userID == "" {
			return contactUser{}, responsecheck.Error("contact/search_contact_by_key_word", "missing_stable_identity", fmt.Sprintf("用户结果第 %d 项缺少 userId", index))
		}
		users = append(users, user)
	}
	switch len(users) {
	case 0:
		return contactUser{}, responsecheck.Error("contact/search_contact_by_key_word", "not_found", "未找到匹配的通讯录用户")
	case 1:
		return users[0], nil
	default:
		return contactUser{}, responsecheck.Error("contact/search_contact_by_key_word", "ambiguous_match", fmt.Sprintf("匹配到 %d 个用户；请提供更精确的姓名", len(users)))
	}
}

func strictUserDetail(data map[string]any, expectedUserID, operation string) (map[string]any, error) {
	if _, err := strictContactEnvelope(data, operation); err != nil {
		return nil, err
	}
	items, err := responsecheck.RequireObjectCollection(data, operation, "result")
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		return nil, responsecheck.Error(operation, "unexpected_detail_count", fmt.Sprintf("用户详情应唯一，实际返回 %d 项", len(items)))
	}
	model, ok := items[0]["orgEmployeeModel"].(map[string]any)
	if !ok || len(model) == 0 {
		return nil, responsecheck.Error(operation, "malformed_result", "用户详情缺少非空 orgEmployeeModel")
	}
	actual := strictContactString(model, "orgUserId")
	if actual == "" {
		actual = strictContactString(model, "userId")
	}
	if actual == "" {
		return nil, responsecheck.Error(operation, "missing_stable_identity", "用户详情缺少 userId/orgUserId")
	}
	if expectedUserID != "" && actual != expectedUserID {
		return nil, responsecheck.Error(operation, "identity_mismatch", "用户详情稳定身份与请求不一致")
	}
	return model, nil
}

func strictDeptCandidates(data map[string]any, operation string) ([]deptMembersDept, error) {
	if _, err := strictContactEnvelope(data, operation); err != nil {
		return nil, err
	}
	items, err := responsecheck.RequireObjectCollection(data, operation, "deptList")
	if err != nil {
		return nil, err
	}
	out := make([]deptMembersDept, 0, len(items))
	seen := map[int64]bool{}
	for index, item := range items {
		id, ok := strictContactInt64(item["deptId"])
		if !ok || id <= 0 {
			return nil, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("部门结果第 %d 项缺少有效 deptId", index))
		}
		if seen[id] {
			return nil, responsecheck.Error(operation, "duplicate_stable_identity", fmt.Sprintf("部门结果包含重复 deptId %d", id))
		}
		seen[id] = true
		name := stripHighlightTags(strictContactString(item, "deptName"))
		if name == "" {
			return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("部门结果第 %d 项缺少 deptName", index))
		}
		out = append(out, deptMembersDept{id: id, name: name})
	}
	return out, nil
}

func strictContactMembers(data map[string]any, operation string) ([]map[string]any, error) {
	if _, err := strictContactEnvelope(data, operation); err != nil {
		return nil, err
	}
	items, err := responsecheck.RequireObjectCollection(data, operation, "deptUserList")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for index, item := range items {
		user, ok := item["userInfo"].(map[string]any)
		if !ok || len(user) == 0 {
			return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("deptUserList[%d].userInfo 应为非空对象", index))
		}
		id := strictContactString(user, "userId")
		if id == "" {
			return nil, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("部门成员第 %d 项缺少 userId", index))
		}
		row := map[string]any{"userId": id}
		if name := strictContactString(user, "name"); name != "" {
			row["name"] = name
		}
		out = append(out, row)
	}
	return out, nil
}

func strictDeptDetail(data map[string]any, expectedID int64, operation string) (map[string]any, error) {
	if _, err := strictContactEnvelope(data, operation); err != nil {
		return nil, err
	}
	detail, err := responsecheck.RequireObjectResult(data, operation)
	if err != nil {
		return nil, err
	}
	id, ok := strictContactInt64(detail["deptId"])
	if !ok || id <= 0 {
		return nil, responsecheck.Error(operation, "missing_stable_identity", "部门详情缺少有效 deptId")
	}
	if expectedID > 0 && id != expectedID {
		return nil, responsecheck.Error(operation, "identity_mismatch", "部门详情稳定身份与请求不一致")
	}
	return detail, nil
}

func strictPrimaryDeptID(model map[string]any, operation string) (int64, error) {
	raw, ok := model["depts"].([]any)
	if !ok || len(raw) == 0 {
		return 0, responsecheck.Error(operation, "missing_collection", "用户详情缺少非空 depts 数组")
	}
	for index, item := range raw {
		dept, ok := item.(map[string]any)
		if !ok || len(dept) == 0 {
			return 0, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("depts[%d] 不是非空对象", index))
		}
		id, ok := strictContactInt64(dept["deptId"])
		if !ok || id <= 0 {
			return 0, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("depts[%d] 缺少有效 deptId", index))
		}
		return id, nil
	}
	return 0, responsecheck.Error(operation, "missing_stable_identity", "用户详情没有可用 deptId")
}

func strictWhoami(data map[string]any) (map[string]any, error) {
	model, err := strictUserDetail(data, "", "contact/get_current_user_profile")
	if err != nil {
		return nil, err
	}
	out := map[string]any{"userId": strictContactFirst(model, "userId", "orgUserId")}
	for outputKey, candidates := range map[string][]string{
		"name": {"orgUserName"}, "mobile": {"orgUserMobile"}, "email": {"orgAuthEmail", "orgEmail"}, "org": {"orgName"},
	} {
		if value := strictContactFirst(model, candidates...); value != "" {
			out[outputKey] = value
		}
	}
	if depts, ok := model["depts"].([]any); ok && len(depts) > 0 {
		if dept, ok := depts[0].(map[string]any); ok {
			if name := strictContactString(dept, "deptName"); name != "" {
				out["dept"] = name
			}
		}
	}
	return out, nil
}

func strictContactFirst(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strictContactString(object, key); value != "" {
			return value
		}
	}
	return ""
}

func strictContactString(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return strings.TrimSpace(value)
}

func strictContactInt64(value any) (int64, bool) {
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
