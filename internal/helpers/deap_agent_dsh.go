// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func resolveExactOperatorOpenDingTalkID(ctx context.Context, accessToken, userID string) (string, error) {
	// get_user_info_by_user_ids 的真实响应只包含组织资料，不提供 openDingTalkId。
	// openDingTalkId 取决于调用 Token 对应的应用/身份上下文，因此必须使用
	// 数字员工 Access Token 查询主管，才能与该员工收到的事件发送者 ID 保持一致。
	accessToken = strings.TrimSpace(accessToken)
	userID = strings.TrimSpace(userID)
	if accessToken == "" || userID == "" || deps == nil || deps.Caller == nil {
		return "", fmt.Errorf("resolve supervisor operator identity is unavailable")
	}
	caller, ok := deps.Caller.(managedIdentityTokenCaller)
	if !ok {
		return "", fmt.Errorf("resolve supervisor operator identity is unavailable")
	}
	result, err := caller.CallToolWithToken(ctx, accessToken, "contact", "search_contact_by_key_word", map[string]any{"keyword": userID})
	if err != nil || result == nil {
		return "", fmt.Errorf("resolve supervisor operator identity failed")
	}
	var response map[string]any
	for _, content := range result.Content {
		if content.Type != "text" || strings.TrimSpace(content.Text) == "" {
			continue
		}
		response, err = decodeMCPJSON(content.Text)
		if err != nil {
			return "", fmt.Errorf("resolve supervisor operator identity returned invalid JSON")
		}
		if success, exists := response["success"].(bool); exists && !success {
			return "", fmt.Errorf("resolve supervisor operator identity was rejected")
		}
		break
	}
	if response == nil {
		return "", fmt.Errorf("resolve supervisor operator identity returned no JSON")
	}
	candidates := map[string]struct{}{}
	collectExactOpenDingTalkIDs(response, userID, candidates)
	if len(candidates) != 1 {
		return "", apperrors.NewValidation("主管 userId 未能精确解析出唯一 operatorOpenDingTalkId；禁止按姓名猜测或手工覆盖")
	}
	for candidate := range candidates {
		return candidate, nil
	}
	return "", apperrors.NewValidation("主管 operatorOpenDingTalkId 缺失")
}

func collectExactOpenDingTalkIDs(value any, targetUserID string, candidates map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		matched := false
		for _, key := range []string{"userId", "user_id", "userid", "uid", "staffId", "staff_id"} {
			if jsonScalar(typed[key]) == targetUserID {
				matched = true
				break
			}
		}
		if nested, ok := typed[targetUserID]; ok {
			collectOpenDingTalkScalars(nested, candidates)
		}
		if matched {
			collectOpenDingTalkScalars(typed, candidates)
		}
		for _, nested := range typed {
			collectExactOpenDingTalkIDs(nested, targetUserID, candidates)
		}
	case []any:
		for _, nested := range typed {
			collectExactOpenDingTalkIDs(nested, targetUserID, candidates)
		}
	}
}

func collectOpenDingTalkScalars(value any, candidates map[string]struct{}) {
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"openDingTalkId", "openDingtalkId", "open_dingtalk_id", "operatorOpenDingTalkId"} {
			if candidate := jsonScalar(object[key]); candidate != "" {
				candidates[candidate] = struct{}{}
			}
		}
	}
}

func runDigitalEmployeeDSHRegister(ctx context.Context, registration map[string]any) (string, error) {
	input, err := json.Marshal(registration)
	if err != nil {
		return "", fmt.Errorf("encode DSH registration: %w", err)
	}
	commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(commandCtx, "dsh-dingtalk", "digital-employee", "register", "--stdin", "--json")
	command.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("dsh-dingtalk register exited unsuccessfully")
	}
	if stdout.Len() > 64*1024 {
		return "", fmt.Errorf("dsh-dingtalk register output too large")
	}
	var response struct {
		Status string `json:"status"`
	}
	decoder := json.NewDecoder(&stdout)
	if err := decoder.Decode(&response); err != nil {
		return "", fmt.Errorf("dsh-dingtalk register returned invalid JSON")
	}
	switch response.Status {
	case "created", "updated", "unchanged":
		return response.Status, nil
	default:
		return "", fmt.Errorf("dsh-dingtalk register returned invalid status")
	}
}
