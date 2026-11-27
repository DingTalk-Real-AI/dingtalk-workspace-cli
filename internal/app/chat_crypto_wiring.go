// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto"
	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
	shortcutchat "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func init() {
	client := newAppMessageCryptoClient()
	helpers.SetChatCryptoClient(client)
	chatmsg.SetMessageDecryptClient(client)
	shortcutchat.SetMessageReadCryptoClient(client)
}

var (
	appMessageCryptoCurrentIdentity       = msgcrypto.CurrentIdentity
	appMessageCryptoOpenSession           = msgcrypto.OpenSession
	appMessageCryptoAvailable             = msgcrypto.Available
	appMessageCryptoFetchProfileWithToken = fetchMessageCryptoProfileWithScopedToken
)

type messageCryptoAuthContextKey struct{}

type messageCryptoAuthContext struct {
	ExplicitToken string
	ClientID      string
}

func withMessageCryptoAuthContext(ctx context.Context, explicitToken, clientID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, messageCryptoAuthContextKey{}, messageCryptoAuthContext{
		ExplicitToken: strings.TrimSpace(explicitToken),
		ClientID:      strings.TrimSpace(clientID),
	})
}

func messageCryptoAuthFromContext(ctx context.Context) (messageCryptoAuthContext, bool) {
	if ctx == nil {
		return messageCryptoAuthContext{}, false
	}
	authCtx, ok := ctx.Value(messageCryptoAuthContextKey{}).(messageCryptoAuthContext)
	if !ok || strings.TrimSpace(authCtx.ExplicitToken) == "" {
		return messageCryptoAuthContext{}, false
	}
	authCtx.ExplicitToken = strings.TrimSpace(authCtx.ExplicitToken)
	authCtx.ClientID = strings.TrimSpace(authCtx.ClientID)
	return authCtx, true
}

const maxStatelessTokenIdentityCacheEntries = 64

type statelessTokenIdentityCache struct {
	mu      sync.Mutex
	entries map[string]messagecrypto.Identity
}

func newStatelessTokenIdentityCache() *statelessTokenIdentityCache {
	return &statelessTokenIdentityCache{
		entries: make(map[string]messagecrypto.Identity),
	}
}

func (c *statelessTokenIdentityCache) key(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (c *statelessTokenIdentityCache) Get(token string) (messagecrypto.Identity, bool) {
	if c == nil || token == "" {
		return messagecrypto.Identity{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.entries[c.key(token)]
	return id, ok
}

func (c *statelessTokenIdentityCache) Set(token string, id messagecrypto.Identity) {
	if c == nil || token == "" || strings.TrimSpace(id.CorpID) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= maxStatelessTokenIdentityCacheEntries {
		clear(c.entries)
	}
	c.entries[c.key(token)] = id
}

func newAppMessageCryptoClient() *messagecrypto.Client {
	identityCache := newStatelessTokenIdentityCache()
	resolveIdentity := func(ctx context.Context, rt messagecrypto.Runtime, configDir string) (messagecrypto.Identity, error) {
		if authCtx, ok := messageCryptoAuthFromContext(ctx); ok {
			return resolveStatelessTokenMessageCryptoIdentity(ctx, rt, authCtx.ExplicitToken, identityCache)
		}
		identity, err := appMessageCryptoCurrentIdentity(ctx, configDir)
		return messagecrypto.Identity{CorpID: identity.CorpID, StaffID: identity.StaffID}, err
	}

	return &messagecrypto.Client{
		Identity: func(ctx context.Context, configDir string) (messagecrypto.Identity, error) {
			return resolveIdentity(ctx, nil, configDir)
		},
		IdentityWithRuntime: resolveIdentity,
		OpenSession: func(ctx context.Context, opts messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
			sessionOpts := msgcrypto.SessionOptions{
				ConfigDir:           opts.ConfigDir,
				CLIVersion:          firstNonEmptyAppCrypto(opts.CLIVersion, RawVersion()),
				KeyServer:           firstNonEmptyAppCrypto(opts.KeyServer, msgcrypto.DefaultSafeChatKeyServer),
				AllowedRedirectHost: firstNonEmptyAppCrypto(opts.AllowedRedirectHost, msgcrypto.DefaultSafeChatRedirectHost),
				KeystoreDir:         opts.KeystoreDir,
			}
			if authCtx, ok := messageCryptoAuthFromContext(ctx); ok {
				identity, err := resolveIdentity(ctx, opts.Runtime, opts.ConfigDir)
				if err != nil {
					return nil, err
				}
				sessionOpts.AccessToken = authCtx.ExplicitToken
				sessionOpts.ClientID = authCtx.ClientID
				sessionOpts.CorpID = identity.CorpID
				sessionOpts.StaffID = identity.StaffID
			}
			session, err := appMessageCryptoOpenSession(ctx, sessionOpts)
			if err != nil {
				return nil, err
			}
			return &messagecrypto.Session{
				Cipher:  session.Cipher,
				CorpID:  session.CorpID,
				StaffID: session.StaffID,
				Close:   session.Close,
			}, nil
		},
		BackendReady: appMessageCryptoAvailable,
		PolicyCache:  messagecrypto.NewPolicyCache(time.Now),
	}
}

func resolveStatelessTokenMessageCryptoIdentity(
	ctx context.Context,
	_ messagecrypto.Runtime,
	explicitToken string,
	cache *statelessTokenIdentityCache,
) (messagecrypto.Identity, error) {
	corpID := strings.TrimSpace(resolveRuntimeDefault(ctx, "$corpId"))
	staffID := strings.TrimSpace(resolveRuntimeDefault(ctx, "$currentUserId"))
	if corpID != "" {
		return messagecrypto.Identity{
			CorpID:  corpID,
			StaffID: firstNonEmptyAppCrypto(staffID, "dws-safechat"),
		}, nil
	}
	if cached, ok := cache.Get(explicitToken); ok && strings.TrimSpace(cached.CorpID) != "" {
		return cached, nil
	}
	result, used, err := appMessageCryptoFetchProfileWithToken(ctx, explicitToken)
	if !used {
		return messagecrypto.Identity{}, fmt.Errorf("使用 --token 解密消息失败：当前运行时不支持请求级令牌身份查询，请提供运行时 corpId 或使用支持请求级令牌的调用器")
	}
	if err != nil {
		return messagecrypto.Identity{}, fmt.Errorf("使用 --token 查询当前用户身份失败: %w", err)
	}
	resolvedCorpID, resolvedUserID := extractContactProfileIdentityFromToolResult(result)
	if resolvedCorpID == "" {
		return messagecrypto.Identity{}, fmt.Errorf("使用 --token 解密消息失败：contact/get_current_user_profile 未返回组织 corpId")
	}
	resolved := messagecrypto.Identity{
		CorpID:  resolvedCorpID,
		StaffID: firstNonEmptyAppCrypto(staffID, resolvedUserID, "dws-safechat"),
	}
	cache.Set(explicitToken, resolved)
	return resolved, nil
}

func fetchMessageCryptoProfileWithScopedToken(ctx context.Context, explicitToken string) (*edition.ToolResult, bool, error) {
	tokenCaller, ok := helpers.GetCaller().(tokenOverrideToolCaller)
	if !ok {
		return nil, false, nil
	}
	result, err := tokenCaller.CallToolWithToken(ctx, explicitToken, "contact", "get_current_user_profile", nil)
	return result, true, err
}

func extractContactProfileIdentityFromToolResult(result *edition.ToolResult) (corpID, userID string) {
	if result == nil {
		return "", ""
	}
	if profile, ok := authpkg.ContactProfileIdentityFromToolResult(result); ok && strings.TrimSpace(profile.CorpID) != "" {
		return strings.TrimSpace(profile.CorpID), strings.TrimSpace(profile.UserID)
	}
	for _, block := range result.Content {
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(text), &data); err != nil {
			continue
		}
		if c, u := extractContactProfileIdentityFromMap(data); c != "" {
			return c, u
		}
	}
	return "", ""
}

func extractContactProfileIdentityFromMap(data map[string]any) (corpID, userID string) {
	src := data
	if nested, ok := data["result"].(map[string]any); ok {
		src = nested
	}
	corpID = appCryptoMapString(src, "corpId", "corp_id")
	userID = appCryptoMapString(src, "userId", "userid", "orgUserId", "staffId")
	return corpID, userID
}

func appCryptoMapString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok && value != nil {
			if s, ok := value.(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func firstNonEmptyAppCrypto(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
