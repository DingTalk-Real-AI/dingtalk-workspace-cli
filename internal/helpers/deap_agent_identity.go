// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type managedIdentityTokenCaller interface {
	CallToolWithToken(ctx context.Context, token, productID, toolName string, args map[string]any) (*edition.ToolResult, error)
}

// resolveDigitalEmployeeManagedIdentity 使用刚换取、尚未落盘的 Access Token
// 查询当前用户。它只接受 expectedCorpID 下唯一且带精确 userId 的在线身份，
// 不使用主管 Profile 或本地历史记录兜底。
func resolveDigitalEmployeeManagedIdentity(ctx context.Context, accessToken, expectedCorpID string) (auth.ManagedIdentity, error) {
	accessToken = strings.TrimSpace(accessToken)
	expectedCorpID = strings.TrimSpace(expectedCorpID)
	if accessToken == "" || expectedCorpID == "" || deps == nil || deps.Caller == nil {
		return auth.ManagedIdentity{}, fmt.Errorf("managed identity lookup is unavailable")
	}
	caller, ok := deps.Caller.(managedIdentityTokenCaller)
	if !ok {
		return auth.ManagedIdentity{}, fmt.Errorf("managed identity lookup is unavailable")
	}
	result, err := caller.CallToolWithToken(ctx, accessToken, "contact", "get_current_user_profile", nil)
	if err != nil || result == nil {
		return auth.ManagedIdentity{}, fmt.Errorf("managed identity lookup failed")
	}

	identity, err := auth.ManagedIdentityFromToolResult(result, expectedCorpID)
	if err != nil {
		return auth.ManagedIdentity{}, fmt.Errorf("managed identity lookup returned no unique identity")
	}
	return identity, nil
}
