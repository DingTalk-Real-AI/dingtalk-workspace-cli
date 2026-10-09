// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// ExternalExchangeRequest logs in with a code obtained on another device. No
// supervisor login is required. Expected identity fields are assertions only.
type ExternalExchangeRequest struct {
	ClientID        string
	AuthCode        string
	ExpectedUserID  string
	ExpectedCorpID  string
	ResolveIdentity ManagedIdentityResolver
	// ClientSecret is an explicitly supplied direct OAuth credential. An empty
	// value may reuse a configured secret only for that same configured client.
	ClientSecret string
}

// ExchangeExternalAuthCode resolves the application before consuming the code,
// verifies the new token online, then saves its exact profile atomically.
func ExchangeExternalAuthCode(ctx context.Context, configDir string, request ExternalExchangeRequest) (*TokenData, error) {
	if strings.TrimSpace(request.AuthCode) == "" || request.ResolveIdentity == nil {
		return nil, fmt.Errorf("external exchange requires an authorization code and identity resolver")
	}
	clientID, secret, source, err := resolveExternalExchangeClient(ctx, configDir, request.ClientID, request.ClientSecret)
	if err != nil {
		return nil, err
	}
	if secret != "" {
		if edition.Get().SaveToken != nil {
			return nil, fmt.Errorf("edition token hook does not support transactional client credentials")
		}
		// Fail before consuming a one-time code if its refresh-credential slot cannot
		// be read. The authoritative snapshot is taken again under the save lock.
		if _, _, err := snapshotExchangeClientSecret(clientID); err != nil {
			return nil, fmt.Errorf("cannot read existing application credentials")
		}
	}
	return exchangeVerifiedAuthCode(ctx, configDir, ManagedExchangeRequest{
		ClientID: clientID, AuthCode: request.AuthCode,
		ExpectedUserID: request.ExpectedUserID, ExpectedCorpID: request.ExpectedCorpID,
		ResolveIdentity: request.ResolveIdentity,
	}, secret, func(data *TokenData) error {
		if secret != "" {
			data.Source = source
		}
		return persistExternalExchangeTokenWithSecret(configDir, data, secret)
	})
}

// Resolve against the supplied directory without the process-wide app cache.
// Client IDs and secrets are selected as pairs; another app's secret is never reused.
func resolveExternalExchangeClient(ctx context.Context, configDir, requestedID, explicitSecret string) (string, string, string, error) {
	clientID := strings.TrimSpace(requestedID)
	if clientID == "default" {
		if explicitSecret != "" {
			return "", "", "", fmt.Errorf("--client-id default cannot be combined with --client-secret")
		}
		id, secret, err := discoverExternalExchangeClient(ctx)
		return id, secret, "mcp", err
	}
	if clientID != "" && explicitSecret != "" {
		return clientID, explicitSecret, "flag", nil
	}
	type credentials struct {
		id, secret, source string
		managed            bool
	}
	clientMu.RLock()
	runtime := credentials{runtimeClientID, runtimeClientSecret, "flag", clientIDFromMCP}
	clientMu.RUnlock()
	h := edition.Get()
	candidates := []credentials{runtime, {h.AuthClientID, "", "default", h.AuthClientFromMCP}}
	cfg, err := LoadAppConfig(configDir)
	if err != nil {
		return "", "", "", fmt.Errorf("cannot read authorization application configuration")
	}
	if cfg != nil && strings.TrimSpace(cfg.ClientID) != "" {
		// 复用应用凭据的绑定与冲突校验，但换票前不得迁移或修改凭据。
		_, secret, _, _, err := resolveAppConfigCredentialsSnapshot(configDir, cfg, false)
		if err != nil && !(errors.Is(err, ErrClientSecretEmpty) && cfg.ClientSecret.IsZero()) {
			return "", "", "", fmt.Errorf("cannot resolve authorization application credentials")
		}
		candidates = append(candidates, credentials{cfg.ClientID, secret, "app", false})
	}
	candidates = append(candidates, credentials{os.Getenv("DWS_CLIENT_ID"), os.Getenv("DWS_CLIENT_SECRET"), "env", false}, credentials{defaultAuthClientID, defaultAuthSecret, "default", false})
	var configured credentials
	for _, c := range candidates {
		c.id = strings.TrimSpace(c.id)
		if c.id != "" && !strings.HasPrefix(c.id, "<") {
			configured = c
			break
		}
	}
	if clientID == "" {
		clientID = configured.id
	}
	if clientID == "" || clientID == "default" {
		if explicitSecret != "" {
			return "", "", "", fmt.Errorf("--client-secret requires a concrete client ID")
		}
		id, secret, err := discoverExternalExchangeClient(ctx)
		return id, secret, "mcp", err
	}
	if explicitSecret != "" {
		return clientID, explicitSecret, "flag", nil
	}
	// 按最终选定的 clientID 遍历全部候选复用密钥：configured 仅是候选列表里第一个
	// 有效应用，用户用 --client-id 显式选中后续 app/env 候选时也必须复用其同 ID 的
	// 非托管 secret；否则会以空 secret 落入托管 MCP 换票，破坏该应用正常的 OAuth
	// 授权码登录。是否托管由每个候选自身的 managed 标记判定，与首个候选无关。
	for _, c := range candidates {
		if strings.TrimSpace(c.id) == clientID && c.secret != "" && !strings.HasPrefix(c.secret, "<") && !c.managed {
			return clientID, c.secret, c.source, nil
		}
	}
	return clientID, "", "mcp", nil
}

func discoverExternalExchangeClient(ctx context.Context) (string, string, error) {
	clientID, err := FetchClientIDFromMCP(ctx)
	clientID = strings.TrimSpace(clientID)
	if err != nil || clientID == "" || clientID == "default" {
		return "", "", fmt.Errorf("official authorization application discovery failed; retry or specify the code's --client-id")
	}
	return clientID, "", nil
}

func persistExternalExchangeToken(configDir string, data *TokenData) error {
	return persistExternalExchangeTokenWithSecret(configDir, data, "")
}

// 同时读取新旧凭据槽，不进行迁移；换票失败时必须能原样恢复两者。
func snapshotExchangeClientSecret(clientID string) (string, string, error) {
	canonical, err := authKeychainGet(keychain.Service, secretAccountKey(clientID))
	if err != nil {
		return "", "", err
	}
	legacy, err := authKeychainGet(keychain.Service, legacyClientSecretAccountKey(clientID))
	return canonical, legacy, err
}

func persistExternalExchangeTokenWithSecret(configDir string, data *TokenData, secret string) error {
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		return err
	}
	if err := preflightTokenWritePersistenceForSelector(configDir, data, cfg.CurrentProfile); err != nil {
		return err
	}
	return withProfilesLock(configDir, func() error {
		cfg, err := profilesLoad(configDir)
		if err != nil {
			return err
		}
		// Choose under the write lock: preserve the latest default, or make the
		// first identity current in a genuinely empty sandbox. RuntimeProfile is
		// a command selector and must not publish a different default here.
		return saveTokenDataLockedForSelectorAndSecret(configDir, data, cfg.CurrentProfile, secret)
	})
}
