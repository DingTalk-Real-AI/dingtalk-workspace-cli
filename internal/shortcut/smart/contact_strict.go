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

func contactSmartResult(item *shortcut.Shortcut) *contract.ResultSpec {
	result := &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}}
	switch item.Command {
	case "+by-mobile", "+lookup":
		result.DataSchema = json.RawMessage(fmt.Sprintf(`{"type":"object","description":%q,"properties":{"profile":{"type":"object","description":"与请求稳定身份精确绑定的用户详情","properties":{"userId":{"type":"string","minLength":1,"description":"稳定用户 ID"},"orgUserId":{"type":"string","minLength":1,"description":"稳定组织用户 ID"}},"additionalProperties":true}},"required":["profile"],"additionalProperties":false}`, item.Description))
		result.SensitivePaths = []string{"profile.userId", "profile.orgUserId", "profile.orgUserName", "profile.orgUserMobile", "profile.orgAuthEmail", "profile.orgEmail", "profile.depts.deptName"}
	case "+dept-members", "+team":
		result.DataSchema = json.RawMessage(fmt.Sprintf(`{"type":"object","description":%q,"properties":{"count":{"type":"integer","minimum":0,"description":"严格校验的直属成员数量"},"members":{"type":"array","description":"带稳定 userId 的直属成员","items":{"type":"object","description":"带稳定用户身份的直属成员","properties":{"userId":{"type":"string","minLength":1,"description":"稳定用户 ID"},"name":{"type":"string","description":"成员姓名"}},"required":["userId"],"additionalProperties":false}}},"required":["count","members"],"additionalProperties":false}`, item.Description))
		result.SensitivePaths = []string{"members.userId", "members.name"}
	case "+org":
		result.DataSchema = json.RawMessage(fmt.Sprintf(`{"type":"object","description":%q,"properties":{"department":{"type":"object","description":"与用户主部门 ID 精确绑定的部门详情","properties":{"deptId":{"type":"integer","minimum":1,"description":"稳定部门 ID"},"deptName":{"type":"string","description":"部门名称"}},"required":["deptId"],"additionalProperties":true}},"required":["department"],"additionalProperties":false}`, item.Description))
		result.SensitivePaths = []string{"department.deptName"}
	case "+resolve-dept":
		result.DataSchema = json.RawMessage(fmt.Sprintf(`{"type":"object","description":%q,"properties":{"resolved":{"type":"boolean","description":"是否唯一解析为一个部门"},"deptId":{"type":"string","minLength":1,"description":"唯一命中时的稳定部门 ID"},"name":{"type":"string","description":"唯一命中时的部门名称"},"count":{"type":"integer","minimum":0,"description":"多命中时的候选数量"},"candidates":{"type":"array","description":"多命中时供消歧的部门候选","items":{"type":"object","description":"带稳定部门身份的候选","properties":{"deptId":{"type":"string","minLength":1,"description":"稳定部门 ID"},"name":{"type":"string","description":"部门名称"}},"required":["deptId","name"],"additionalProperties":false}}},"required":["resolved"],"additionalProperties":false}`, item.Description))
		result.SensitivePaths = []string{"name", "candidates.name"}
	case "+me":
		result.DataSchema = json.RawMessage(fmt.Sprintf(`{"type":"object","description":%q,"properties":{"userId":{"type":"string","minLength":1,"description":"当前用户稳定 ID"},"name":{"type":"string","description":"当前用户姓名"},"mobile":{"type":"string","description":"当前用户手机号"},"email":{"type":"string","description":"当前用户邮箱"},"org":{"type":"string","description":"当前用户组织名称"},"dept":{"type":"string","description":"当前用户主部门名称"}},"required":["userId"],"additionalProperties":false}`, item.Description))
		result.SensitivePaths = []string{"userId", "name", "mobile", "email", "org", "dept"}
	default:
		result.DataSchema = json.RawMessage(fmt.Sprintf(`{"type":"object","description":%q,"additionalProperties":true}`, item.Description))
	}
	return result
}

func finalizeContactSmart(item *shortcut.Shortcut) {
	item.OutputRollout = output.RolloutUnifiedActive
	item.Contract.Result = contactSmartResult(item)
	operation := "contact/" + strings.TrimPrefix(item.Command, "+")
	requiredStrings := make([]string, 0, len(item.Flags))
	for index := range item.Flags {
		flag := &item.Flags[index]
		if flag.Required && flag.Type == shortcut.FlagString {
			requiredStrings = append(requiredStrings, flag.Name)
			fact := "--" + flag.Name + " 不能为空白"
			if !strings.Contains(flag.Desc, fact) {
				flag.Desc += "；" + fact
			}
			item.Constraints = append(item.Constraints, shortcut.Constraint{Kind: shortcut.ConstraintCustom, Flags: []string{flag.Name}, Description: fact})
		}
	}
	previousValidate := item.Validate
	if previousValidate != nil || len(requiredStrings) > 0 {
		item.Validate = func(rt *shortcut.RuntimeContext) error {
			if previousValidate != nil {
				if err := previousValidate(rt); err != nil {
					return err
				}
			}
			return validateContactSmartNonBlank(rt, operation, requiredStrings...)
		}
	}
	switch item.Command {
	case "+by-mobile":
		item.Contract.Parameters = []contract.ParamDecl{{Name: "mobile", Property: "keyword"}}
	case "+dept-members":
		item.Contract.Parameters = []contract.ParamDecl{{Name: "dept", Property: "query"}}
	case "+lookup", "+org", "+team":
		item.Contract.Parameters = []contract.ParamDecl{{Name: "name", Property: "keyword"}}
	case "+resolve-dept":
		item.Contract.Parameters = []contract.ParamDecl{{Name: "name", Property: "query"}}
	}
}

func validateContactSmartNonBlank(rt *shortcut.RuntimeContext, operation string, flags ...string) error {
	for _, flag := range flags {
		if strings.TrimSpace(rt.Str(flag)) == "" {
			return responsecheck.Error(operation, "empty_parameter", "--"+flag+" 不能为空白")
		}
	}
	return nil
}

func validateContactSmartMobile(rt *shortcut.RuntimeContext, operation, flag string) error {
	if _, err := normalizeContactSmartMobile(rt.Str(flag)); err != nil {
		return responsecheck.Error(operation, "invalid_mobile", "--"+flag+" 必须是至少 6 位数字的手机号，可包含国家码、空格、连字符或括号")
	}
	return nil
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
	seen := make(map[string]bool, len(items))
	for index, item := range items {
		user := contactUser{
			userID:         strictContactString(item, "userId"),
			openDingTalkID: strictContactString(item, "openDingTalkId"),
			name:           strictContactString(item, "name"),
		}
		if user.userID == "" {
			return contactUser{}, responsecheck.Error("contact/search_contact_by_key_word", "missing_stable_identity", fmt.Sprintf("用户结果第 %d 项缺少 userId", index))
		}
		if seen[user.userID] {
			return contactUser{}, responsecheck.Error("contact/search_contact_by_key_word", "duplicate_stable_identity", fmt.Sprintf("用户结果第 %d 项重复 userId", index))
		}
		seen[user.userID] = true
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

func strictResolveContactUserByMobile(rt *shortcut.RuntimeContext, mobile string) (contactUser, map[string]any, error) {
	if _, err := normalizeContactSmartMobile(mobile); err != nil {
		return contactUser{}, nil, responsecheck.Error("contact/by-mobile", "invalid_mobile", "--mobile 必须是至少 6 位数字的手机号，可包含国家码、空格、连字符或括号")
	}
	user, err := strictResolveContactUser(rt, mobile)
	if err != nil {
		return contactUser{}, nil, err
	}
	data, err := rt.CallMCPData("contact", "get_user_info_by_user_ids", map[string]any{
		"user_id_list": []string{user.userID},
	})
	if err != nil {
		return contactUser{}, nil, err
	}
	profile, err := strictUserDetail(data, user.userID, "contact/get_user_info_by_user_ids")
	if err != nil {
		return contactUser{}, nil, err
	}
	actualMobile, present, valid := strictContactOptionalString(profile, "orgUserMobile")
	if !present || !valid || actualMobile == "" {
		return contactUser{}, nil, responsecheck.Error("contact/get_user_info_by_user_ids", "missing_mobile_evidence", "手机号候选详情缺少可核验的 orgUserMobile")
	}
	stateCode, _, valid := strictContactOptionalString(profile, "stateCode")
	if !valid {
		return contactUser{}, nil, responsecheck.Error("contact/get_user_info_by_user_ids", "malformed_mobile_evidence", "手机号候选详情 stateCode 必须是字符串")
	}
	if !contactSmartMobileMatches(mobile, actualMobile, stateCode) {
		return contactUser{}, nil, responsecheck.Error("contact/get_user_info_by_user_ids", "mobile_mismatch", "手机号候选详情与请求手机号不一致")
	}
	return user, profile, nil
}

func normalizeContactSmartMobile(value string) (string, error) {
	return normalizeContactSmartMobilePart(value, 6)
}

func normalizeContactSmartMobilePart(value string, minimumDigits int) (string, error) {
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

func contactSmartMobileMatches(expected, actual, stateCode string) bool {
	expectedDigits, err := normalizeContactSmartMobile(expected)
	if err != nil {
		return false
	}
	actualDigits, err := normalizeContactSmartMobile(actual)
	if err != nil {
		return false
	}
	if expectedDigits == actualDigits {
		return true
	}
	stateDigits, err := normalizeContactSmartMobilePart(stateCode, 1)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(actualDigits, stateDigits) && expectedDigits == stateDigits+actualDigits
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
	seen := make(map[string]bool, len(items))
	for index, item := range items {
		user, ok := item["userInfo"].(map[string]any)
		if !ok || len(user) == 0 {
			return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("deptUserList[%d].userInfo 应为非空对象", index))
		}
		id := strictContactString(user, "userId")
		if id == "" {
			return nil, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("部门成员第 %d 项缺少 userId", index))
		}
		if seen[id] {
			return nil, responsecheck.Error(operation, "duplicate_stable_identity", fmt.Sprintf("部门成员第 %d 项重复 userId", index))
		}
		seen[id] = true
		row := map[string]any{"userId": id}
		name, _, valid := strictContactOptionalString(user, "name")
		if !valid {
			return nil, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("部门成员第 %d 项的 name 必须是字符串", index))
		}
		if name != "" {
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
	if _, _, valid := strictContactOptionalString(detail, "deptName"); !valid {
		return nil, responsecheck.Error(operation, "malformed_result", "部门详情 deptName 必须是字符串")
	}
	detail["deptId"] = id
	return detail, nil
}

func strictPrimaryDeptID(model map[string]any, operation string) (int64, error) {
	raw, ok := model["depts"].([]any)
	if !ok || len(raw) == 0 {
		return 0, responsecheck.Error(operation, "missing_collection", "用户详情缺少非空 depts 数组")
	}
	seen := make(map[int64]bool, len(raw))
	var primary int64
	for index, item := range raw {
		dept, ok := item.(map[string]any)
		if !ok || len(dept) == 0 {
			return 0, responsecheck.Error(operation, "malformed_item", fmt.Sprintf("depts[%d] 不是非空对象", index))
		}
		id, ok := strictContactInt64(dept["deptId"])
		if !ok || id <= 0 {
			return 0, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("depts[%d] 缺少有效 deptId", index))
		}
		if seen[id] {
			return 0, responsecheck.Error(operation, "duplicate_stable_identity", fmt.Sprintf("depts[%d] 重复 deptId", index))
		}
		seen[id] = true
		if index == 0 {
			primary = id
		}
	}
	return primary, nil
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

func strictContactOptionalString(object map[string]any, key string) (string, bool, bool) {
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
