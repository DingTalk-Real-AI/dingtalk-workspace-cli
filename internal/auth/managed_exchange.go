// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// ManagedExchangeRequest 描述一次受管身份的短期授权码换票。ClientID 必须来自
// 同一份授权响应，PreserveProfile 是发起操作的主管精确 Profile。
type ManagedExchangeRequest struct {
	ClientID string
	AuthCode string
	// ExpectedUserID 是 DEAP 授权响应 staffId 对应的钉钉 Profile userId。
	ExpectedUserID string
	// ExpectedCorpID 来自已发布数字员工的 profile.corpId。
	ExpectedCorpID  string
	PreserveProfile string
	ResolveIdentity ManagedIdentityResolver
}

// ManagedIdentity 是使用刚换取的 Access Token 查询到的当前用户身份。
// Managed Exchange 只信任该在线结果，不使用本地历史 Profile 补全身份。
type ManagedIdentity struct {
	CorpID   string
	CorpName string
	UserID   string
	UserName string
}

// ManagedIdentityResolver 使用尚未落盘的 Access Token 查询当前用户身份。
// expectedCorpID 用于调用方在多组织响应中做精确选择。
type ManagedIdentityResolver func(ctx context.Context, accessToken, expectedCorpID string) (ManagedIdentity, error)

var (
	managedExchangePreparePersistence = prepareLoginPersistence
	managedExchangePersistToken       = persistManagedExchangeToken
)

// ExchangeManagedAuthCode 使用显式 ClientID 直接走 MCP Exchange，并把结果写入
// 数字员工的精确身份槽。它不读取或修改全局 ClientID/ClientSecret、环境变量、
// app.json 或 RuntimeProfile；PreserveProfile 只参与本次持久化写计划。
func ExchangeManagedAuthCode(ctx context.Context, configDir string, request ManagedExchangeRequest) (*TokenData, error) {
	clientID := strings.TrimSpace(request.ClientID)
	authCode := strings.TrimSpace(request.AuthCode)
	expectedUserID := strings.TrimSpace(request.ExpectedUserID)
	expectedCorpID := strings.TrimSpace(request.ExpectedCorpID)
	preserveProfile := strings.TrimSpace(request.PreserveProfile)
	if clientID == "" || authCode == "" || expectedUserID == "" || expectedCorpID == "" || preserveProfile == "" || request.ResolveIdentity == nil {
		return nil, fmt.Errorf("managed exchange requires clientId, authCode, expected userId, expected corpId, supervisor profile and identity resolver")
	}
	if err := managedExchangePreparePersistence(configDir); err != nil {
		return nil, fmt.Errorf("local login state cannot be safely updated: %w", err)
	}

	provider := &OAuthProvider{
		configDir:  configDir,
		clientID:   clientID,
		Output:     io.Discard,
		httpClient: oauthHTTPClient,
	}
	data, err := provider.exchangeCodeViaMCPClientID(ctx, authCode, clientID)
	if err != nil {
		// 授权码、Token 和服务端原始正文都不得进入错误链或调试输出。
		return nil, fmt.Errorf("managed token exchange failed")
	}
	if data == nil {
		return nil, fmt.Errorf("managed token exchange returned no token data")
	}
	returnedCorpID := strings.TrimSpace(data.CorpID)
	if returnedCorpID == "" || returnedCorpID != expectedCorpID {
		return nil, fmt.Errorf("managed token organization does not match the published digital employee")
	}
	identity, err := request.ResolveIdentity(ctx, data.AccessToken, expectedCorpID)
	if err != nil {
		return nil, fmt.Errorf("managed token identity lookup failed")
	}
	resolvedUID := strings.TrimSpace(identity.UserID)
	if resolvedUID == "" || resolvedUID != expectedUserID {
		return nil, fmt.Errorf("managed token identity does not match the authorized digital employee")
	}
	resolvedCorpID := strings.TrimSpace(identity.CorpID)
	if resolvedCorpID == "" || resolvedCorpID != expectedCorpID || resolvedCorpID != returnedCorpID {
		return nil, fmt.Errorf("managed identity organization does not match the published digital employee")
	}
	if tokenUID := strings.TrimSpace(data.UserID); tokenUID != "" && tokenUID != resolvedUID {
		return nil, fmt.Errorf("managed token identity does not match the resolved current user")
	}
	data.CorpID = returnedCorpID
	data.UserID = resolvedUID
	data.UserName = strings.TrimSpace(identity.UserName)
	if corpName := strings.TrimSpace(identity.CorpName); corpName != "" {
		data.CorpName = corpName
	}
	data.ClientID = clientID
	data.Source = "mcp"
	data.FreshAuthorization = true
	if err := managedExchangePersistToken(configDir, preserveProfile, data); err != nil {
		return nil, fmt.Errorf("save managed digital employee profile: %w", err)
	}
	return data, nil
}

func persistManagedExchangeToken(configDir, preserveProfile string, data *TokenData) error {
	if data == nil {
		return fmt.Errorf("token data is empty")
	}
	if err := preflightTokenWritePersistenceForSelector(configDir, data, preserveProfile); err != nil {
		return fmt.Errorf("local login state cannot be safely updated: %w", err)
	}
	return withProfilesLock(configDir, func() error {
		return saveTokenDataLockedForSelector(configDir, data, preserveProfile)
	})
}
