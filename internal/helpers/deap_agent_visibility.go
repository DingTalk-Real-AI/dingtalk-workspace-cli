// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

// 所有查询使用绑定时固定的精确 Profile，不随全局 current 切换，也不缓存放行结果。
var employeeVisibilityCall = employeeVisibilityProfileCall
var employeeVisibilityToken = func(ctx context.Context, profile string) (*auth.TokenData, error) {
	return auth.NewOAuthProvider(deapConnectConfigDir(), nil).GetTokenSnapshotForProfile(ctx, profile)
}

func employeeVisibilityProfileCall(ctx context.Context, profile, server, tool string, args map[string]any) (map[string]any, error) {
	token, err := employeeVisibilityToken(ctx, profile)
	if err != nil || token == nil || auth.ProfileSelector(auth.Profile{CorpID: token.CorpID, UserID: token.UserID}) != profile {
		return nil, fmt.Errorf("visibility_profile_unavailable")
	}
	if deps == nil {
		return nil, fmt.Errorf("visibility_caller_unavailable")
	}
	caller, ok := deps.Caller.(managedIdentityTokenCaller)
	if !ok {
		return nil, fmt.Errorf("visibility_caller_unavailable")
	}
	result, err := caller.CallToolWithToken(ctx, token.AccessToken, server, tool, args)
	if err != nil || result == nil {
		return nil, fmt.Errorf("visibility_query_failed")
	}
	for _, block := range result.Content {
		if block.Type != "text" || strings.TrimSpace(block.Text) == "" {
			continue
		}
		value, err := decodeMCPJSON(block.Text)
		if err != nil {
			return nil, fmt.Errorf("visibility_response_invalid")
		}
		if success, exists := value["success"]; (exists && success != true) || value["error"] != nil {
			return nil, fmt.Errorf("visibility_query_rejected")
		}
		return value, nil
	}
	return nil, fmt.Errorf("visibility_response_empty")
}

func employeeVisibilityAccess(ctx context.Context, b digitalEmployeeBinding, sender, senderName string) (string, bool, error) {
	return employeeConversationAccess(ctx, b, sender, senderName, "direct")
}

// 群事件来自员工自身订阅；local_agent 已入群即有群内访问权，私聊仍查个人可见范围。
func employeeConversationAccess(ctx context.Context, b digitalEmployeeBinding, sender, senderName, conversationType string) (string, bool, error) {
	if conversationType != "direct" && conversationType != "group" {
		return "deap_visibility", false, fmt.Errorf("visibility_conversation_type_invalid")
	}
	if b.SupervisorProfile == "" && b.RuntimeBindingID == "" {
		return "local_allowlist", false, nil
	}
	const policy = "deap_visibility"
	if !validMachineString(sender) || b.SupervisorProfile == "" {
		return policy, false, fmt.Errorf("visibility_supervisor_missing: use connect restart with the original supervisor profile")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	published, err := employeeVisibilityCall(ctx, b.SupervisorProfile, deapAgentServerID, deapAgentDetailTool, map[string]any{"agentUuid": b.AgentUUID, "snapshot": "published"})
	if err != nil {
		return policy, false, err
	}
	data := businessDataMap(published)
	identity, ok := publishedDigitalEmployeeIdentity(published)
	if !ok || jsonScalar(data["agentUuid"]) != b.AgentUUID || auth.ProfileSelector(auth.Profile{CorpID: identity.CorpID, UserID: identity.UserID}) != b.DWSProfile || jsonScalar(data["snapshot"]) != "published" {
		return policy, false, fmt.Errorf("visibility_identity_mismatch")
	}
	switch jsonScalar(data["type"]) {
	case "local_agent":
	case "open_code":
		return "local_allowlist", false, nil
	default:
		return policy, false, fmt.Errorf("visibility_type_unknown")
	}
	if jsonScalar(data["status"]) != "online" || sender == identity.OpenDingTalkID {
		return policy, false, nil
	}
	if conversationType == "group" {
		return policy, true, nil
	}
	scope := jsonScalar(data["visibility"])
	if scope != "ALL" && scope != "PARTIAL" {
		return policy, false, fmt.Errorf("visibility_scope_unknown")
	}
	users, err := employeeVisibilityIDs(data["staffIds"])
	if err != nil {
		return policy, false, err
	}
	depts, err := employeeVisibilityIDs(data["deptIds"])
	if err != nil {
		return policy, false, err
	}
	// 已明确配置的 userId 在员工身份下正向解析，不能直接比较两个不同命名空间的 ID。
	if scope == "ALL" {
		users = nil
	}
	for _, id := range users {
		response, err := employeeVisibilityCall(ctx, b.DWSProfile, "contact", "search_contact_by_key_word", map[string]any{"keyword": id})
		if err != nil {
			return policy, false, err
		}
		candidates := map[string]struct{}{}
		collectExactOpenDingTalkIDs(response, id, candidates)
		if len(candidates) != 1 {
			return policy, false, fmt.Errorf("visibility_member_unresolved")
		}
		if _, found := candidates[sender]; found {
			return policy, true, nil
		}
	}
	if scope == "PARTIAL" && len(depts) == 0 {
		return policy, false, nil
	}
	// 名称只是检索提示；必须精确匹配事件的开放 ID，随后确认当前企业成员身份。
	// 不能依赖 get_user_id_by_open_dingtalk_id：员工身份下该接口可能不支持关系型开放 ID。
	if strings.TrimSpace(senderName) == "" {
		return policy, false, fmt.Errorf("visibility_sender_unresolved")
	}
	response, err := employeeVisibilityCall(ctx, b.DWSProfile, "contact", "search_contact_by_key_word", map[string]any{"keyword": senderName})
	if err != nil {
		return policy, false, err
	}
	ids := map[string]struct{}{}
	rows, ok := response["result"].([]any)
	if !ok {
		return policy, false, fmt.Errorf("visibility_sender_invalid")
	}
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if ok && jsonScalar(m["openDingTalkId"]) == sender && jsonScalar(m["userId"]) != "" {
			ids[jsonScalar(m["userId"])] = struct{}{}
		}
	}
	if len(ids) != 1 {
		return policy, false, fmt.Errorf("visibility_sender_unresolved")
	}
	var userID string
	for id := range ids {
		userID = id
	}
	response, err = employeeVisibilityCall(ctx, b.DWSProfile, "contact", "get_user_info_by_user_ids", map[string]any{"user_id_list": []string{userID}})
	if err != nil {
		return policy, false, err
	}
	rows, ok = response["result"].([]any)
	if !ok {
		return policy, false, fmt.Errorf("visibility_membership_invalid")
	}
	memberDepts := map[string]bool{}
	members := 0
	for _, row := range rows {
		m, _ := row.(map[string]any)
		org, _ := m["orgEmployeeModel"].(map[string]any)
		if jsonScalar(org["orgUserId"]) != userID {
			continue
		}
		members++
		ds, _ := org["depts"].([]any)
		for _, d := range ds {
			v, _ := d.(map[string]any)
			if id := jsonScalar(v["deptId"]); id != "" {
				memberDepts[id] = true
			}
		}
	}
	if members != 1 {
		return policy, false, fmt.Errorf("visibility_membership_unresolved")
	}
	if scope == "ALL" {
		return policy, true, nil
	}
	// 部门范围包含子部门；遍历有界，接口失败或超限绝不按本地白名单放行。
	queue := append([]string(nil), depts...)
	seen := map[string]bool{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		if memberDepts[id] {
			return policy, true, nil
		}
		if len(seen) > 256 {
			return policy, false, fmt.Errorf("visibility_department_limit")
		}
		deptID, parseErr := strconv.ParseInt(id, 10, 64)
		if parseErr != nil || deptID <= 0 {
			return policy, false, fmt.Errorf("visibility_department_invalid")
		}
		response, err = employeeVisibilityCall(ctx, b.DWSProfile, "contact", "get_sub_depts_by_dept_id", map[string]any{"deptId": deptID})
		if err != nil {
			return policy, false, err
		}
		children, ok := response["result"].([]any)
		if !ok {
			return policy, false, fmt.Errorf("visibility_departments_invalid")
		}
		for _, child := range children {
			m, _ := child.(map[string]any)
			next := jsonScalar(m["deptId"])
			if next == "" {
				return policy, false, fmt.Errorf("visibility_departments_invalid")
			}
			queue = append(queue, next)
		}
	}
	return policy, false, nil
}

func employeeVisibilityIDs(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("visibility_ids_invalid")
	}
	ids := make([]string, 0, len(values))
	for _, v := range values {
		id := jsonScalar(v)
		if !validMachineString(id) {
			return nil, fmt.Errorf("visibility_ids_invalid")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// 旧绑定迁移只接受原 operator 的管理账号，并核对员工发布身份。
func migrateEmployeeVisibilitySupervisor(ctx context.Context, b *digitalEmployeeBinding) error {
	if b.SupervisorProfile != "" || b.RuntimeBindingID == "" {
		return nil
	}
	selector, supervisor, err := currentSupervisorProfile(ctx, deapConnectConfigDir())
	if err != nil {
		return err
	}
	token, err := employeeVisibilityToken(ctx, b.DWSProfile)
	if err != nil {
		return fmt.Errorf("visibility_employee_profile_unavailable")
	}
	operator, err := resolveExactOperatorOpenDingTalkID(ctx, token.AccessToken, supervisor.UserID)
	if err != nil || operator != b.OperatorOpenDingTalkID {
		return fmt.Errorf("visibility_supervisor_mismatch")
	}
	published, err := employeeVisibilityCall(ctx, selector, deapAgentServerID, deapAgentDetailTool, map[string]any{"agentUuid": b.AgentUUID, "snapshot": "published"})
	if err != nil {
		return err
	}
	identity, ok := publishedDigitalEmployeeIdentity(published)
	if !ok || auth.ProfileSelector(auth.Profile{CorpID: identity.CorpID, UserID: identity.UserID}) != b.DWSProfile || jsonScalar(businessDataMap(published)["agentUuid"]) != b.AgentUUID {
		return fmt.Errorf("visibility_identity_mismatch")
	}
	b.SupervisorProfile = selector
	return nil
}
