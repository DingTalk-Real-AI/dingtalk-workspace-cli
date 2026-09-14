// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package message

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	ModeOff        = "off"
	ModeThirdParty = "third_party"
	ModeRequired   = "required"
	ModeDegraded   = "degraded"
)

// Runtime is the narrow MCP surface required by the DWS message crypto layer.
type Runtime interface {
	CallMCPReadData(product, tool string, params map[string]any) (map[string]any, error)
	CallMCPWriteDataStrict(product, tool string, params map[string]any) (map[string]any, error)
	DryRun() bool
}

type Identity struct {
	CorpID  string
	StaffID string
}

type Cipher interface {
	EncryptMessage(ctx context.Context, corpID, staffID string, plaintext []byte) ([]byte, error)
	DecryptMessage(ctx context.Context, corpID, staffID string, ciphertext []byte) ([]byte, error)
}

type Session struct {
	Cipher  Cipher
	CorpID  string
	StaffID string
	Close   func() error
}

type SessionOptions struct {
	ConfigDir           string
	CLIVersion          string
	KeyServer           string
	AllowedRedirectHost string
	KeystoreDir         string
}

type Options struct {
	Identity            string
	MsgType             string
	OpenConversationID  string
	ReceiverOpenTalkID  string
	RequirePolicy       bool
	Layer               string
	Ciphertext          string
	ConfigDir           string
	CLIVersion          string
	KeyServer           string
	AllowedRedirectHost string
	KeystoreDir         string
	Now                 func() time.Time
}

type Policy struct {
	Mode                string
	Provider            string
	KeyServer           string
	AllowedRedirectHost string
	StaffIDTransform    string
	TTL                 time.Duration
	Reason              string
}

type DecryptResult struct {
	Layer          string
	Plaintext      string
	DingCiphertext string
	KeyVersion     int
}

type BatchDecryptItem struct {
	MessageID      string
	ConversationID string
	Ciphertext     string
}

type BatchDecryptItemResult struct {
	MessageID        string
	ConversationID   string
	Status           string
	PlaintextContent string
	KeyVersion       int
	Reason           string
}

type BatchDecryptResult struct {
	Items    []BatchDecryptItemResult
	Failures []BatchDecryptItemResult
}

type PolicyDecision struct {
	Policy  Policy
	Enabled bool
}

type Client struct {
	Identity     func(context.Context, string) (Identity, error)
	OpenSession  func(context.Context, SessionOptions) (*Session, error)
	BackendReady func() bool
	PolicyCache  *PolicyCache
}

func DefaultClient() *Client {
	return &Client{
		BackendReady: func() bool { return false },
		PolicyCache:  NewPolicyCache(time.Now),
	}
}

func (c *Client) PolicyDecision(ctx context.Context, rt Runtime, opts Options) (PolicyDecision, error) {
	identity, err := c.currentIdentity(ctx, opts.ConfigDir)
	if err != nil {
		return PolicyDecision{}, err
	}
	policy, err := c.policy(ctx, rt, identity.CorpID, opts)
	if err != nil {
		return PolicyDecision{}, err
	}
	return PolicyDecision{Policy: policy, Enabled: policyRequiresEncryption(policy.Mode)}, nil
}

func (c *Client) DecryptInbound(ctx context.Context, rt Runtime, opts Options) (DecryptResult, error) {
	layer := strings.TrimSpace(opts.Layer)
	if layer == "" {
		layer = "full"
	}
	if layer != "full" && layer != "safechat" && layer != "ding" {
		return DecryptResult{}, fmt.Errorf("--layer 仅支持 full/safechat/ding")
	}
	dingCiphertext := strings.TrimSpace(opts.Ciphertext)
	if layer == "full" || layer == "safechat" {
		session, err := c.openSession(ctx, opts, Policy{KeyServer: opts.KeyServer, AllowedRedirectHost: opts.AllowedRedirectHost})
		if err != nil {
			return DecryptResult{}, err
		}
		defer closeSession(session)
		plain, err := session.Cipher.DecryptMessage(ctx, session.CorpID, session.StaffID, []byte(opts.Ciphertext))
		if err != nil {
			return DecryptResult{}, err
		}
		dingCiphertext = string(plain)
		if layer == "safechat" {
			return DecryptResult{Layer: layer, Plaintext: dingCiphertext, DingCiphertext: dingCiphertext}, nil
		}
	}
	data, err := rt.CallMCPWriteDataStrict("im", "batch_ding_decrypt_messages", map[string]any{
		"items": []map[string]any{{
			"messageId":      "manual",
			"dingCiphertext": dingCiphertext,
		}},
	})
	if err != nil {
		return DecryptResult{}, err
	}
	items := parseBatchDecryptResults(data)
	if len(items) == 0 {
		return DecryptResult{}, fmt.Errorf("im/batch_ding_decrypt_messages 未返回 plaintextContent")
	}
	item := items[0]
	if item.Status != "" && item.Status != "success" {
		return DecryptResult{}, fmt.Errorf("im/batch_ding_decrypt_messages 解密失败: %s", item.Reason)
	}
	plaintext := strings.TrimSpace(item.PlaintextContent)
	if plaintext == "" {
		return DecryptResult{}, fmt.Errorf("im/batch_ding_decrypt_messages 未返回 plaintextContent")
	}
	return DecryptResult{Layer: layer, Plaintext: plaintext, DingCiphertext: dingCiphertext, KeyVersion: item.KeyVersion}, nil
}

func (c *Client) BatchDecryptInbound(ctx context.Context, rt Runtime, opts Options, items []BatchDecryptItem) (BatchDecryptResult, error) {
	items = normalizeBatchDecryptItems(items)
	if len(items) == 0 {
		return BatchDecryptResult{}, nil
	}
	session, err := c.openSession(ctx, opts, Policy{KeyServer: opts.KeyServer, AllowedRedirectHost: opts.AllowedRedirectHost})
	if err != nil {
		return BatchDecryptResult{}, err
	}
	defer closeSession(session)
	dingItems := make([]map[string]any, 0, len(items))
	result := BatchDecryptResult{}
	for _, item := range items {
		plain, err := session.Cipher.DecryptMessage(ctx, session.CorpID, session.StaffID, []byte(item.Ciphertext))
		if err != nil {
			result.Failures = append(result.Failures, BatchDecryptItemResult{
				MessageID:      item.MessageID,
				ConversationID: item.ConversationID,
				Status:         "failed",
				Reason:         "safechat_decrypt_failed",
			})
			continue
		}
		dingItems = append(dingItems, map[string]any{
			"messageId":      item.MessageID,
			"conversationId": item.ConversationID,
			"dingCiphertext": string(plain),
		})
	}
	if len(dingItems) == 0 {
		return result, nil
	}
	data, err := rt.CallMCPWriteDataStrict("im", "batch_ding_decrypt_messages", map[string]any{
		"items": dingItems,
	})
	if err != nil {
		return BatchDecryptResult{}, err
	}
	result.Items = append(result.Items, parseBatchDecryptResults(data)...)
	return result, nil
}

func (c *Client) currentIdentity(ctx context.Context, configDir string) (Identity, error) {
	if c != nil && c.Identity != nil {
		return c.Identity(ctx, configDir)
	}
	return Identity{}, fmt.Errorf("msgcrypto/message: identity provider is not configured")
}

func (c *Client) openSession(ctx context.Context, opts Options, policy Policy) (*Session, error) {
	if c == nil || c.OpenSession == nil {
		return nil, fmt.Errorf("msgcrypto/message: SafeChat session provider is not configured")
	}
	keyServer := firstNonEmpty(policy.KeyServer, opts.KeyServer)
	redirectHost := firstNonEmpty(policy.AllowedRedirectHost, opts.AllowedRedirectHost)
	return c.OpenSession(ctx, SessionOptions{
		ConfigDir:           opts.ConfigDir,
		CLIVersion:          opts.CLIVersion,
		KeyServer:           keyServer,
		AllowedRedirectHost: redirectHost,
		KeystoreDir:         opts.KeystoreDir,
	})
}

func closeSession(session *Session) {
	if session != nil && session.Close != nil {
		_ = session.Close()
	}
}

func (c *Client) policy(ctx context.Context, rt Runtime, corpID string, opts Options) (Policy, error) {
	key := policyCacheKey(corpID, opts)
	if c != nil && c.PolicyCache != nil {
		if policy, ok := c.PolicyCache.Get(key); ok {
			return policy, nil
		}
	}
	data, err := rt.CallMCPReadData("im", "get_message_crypto_policy", map[string]any{
		"openConversationId":     strings.TrimSpace(opts.OpenConversationID),
		"receiverOpenDingTalkId": strings.TrimSpace(opts.ReceiverOpenTalkID),
		"identity":               strings.TrimSpace(opts.Identity),
		"msgType":                strings.TrimSpace(opts.MsgType),
	})
	if err != nil {
		return Policy{}, err
	}
	policy, err := parsePolicy(data)
	if err != nil {
		return Policy{}, err
	}
	if c != nil && c.PolicyCache != nil {
		c.PolicyCache.Set(key, policy)
	}
	return policy, nil
}

type PolicyCache struct {
	now func() time.Time
	mu  sync.Mutex
	m   map[string]policyEntry
}

type policyEntry struct {
	policy    Policy
	expiresAt time.Time
}

func NewPolicyCache(now func() time.Time) *PolicyCache {
	if now == nil {
		now = time.Now
	}
	return &PolicyCache{now: now, m: map[string]policyEntry{}}
}

func (c *PolicyCache) Get(key string) (Policy, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.m[key]
	if !ok || !c.now().Before(entry.expiresAt) {
		delete(c.m, key)
		return Policy{}, false
	}
	return entry.policy, true
}

func (c *PolicyCache) Set(key string, policy Policy) {
	if policy.TTL <= 0 {
		return
	}
	c.mu.Lock()
	c.m[key] = policyEntry{policy: policy, expiresAt: c.now().Add(policy.TTL)}
	c.mu.Unlock()
}

func parsePolicy(data map[string]any) (Policy, error) {
	src := data
	if result, ok := data["result"].(map[string]any); ok {
		src = result
	}
	mode := strings.TrimSpace(stringField(src, "mode"))
	if mode == "" && len(src) == 1 && src["success"] == true {
		mode = ModeOff
	}
	if mode != ModeOff && mode != ModeThirdParty && mode != ModeRequired && mode != ModeDegraded {
		return Policy{}, fmt.Errorf("im/get_message_crypto_policy 返回未知 mode %q", mode)
	}
	return Policy{
		Mode:                mode,
		Provider:            firstNonEmpty(stringField(src, "provider"), "safechat"),
		KeyServer:           stringField(src, "keyServer"),
		AllowedRedirectHost: stringField(src, "allowedRedirectHost"),
		StaffIDTransform:    firstNonEmpty(stringField(src, "staffIdTransform"), "raw"),
		TTL:                 time.Duration(intField(src, "ttlSeconds")) * time.Second,
		Reason:              stringField(src, "reason"),
	}, nil
}

func policyRequiresEncryption(mode string) bool {
	return mode == ModeRequired || mode == ModeThirdParty
}

func normalizeBatchDecryptItems(items []BatchDecryptItem) []BatchDecryptItem {
	out := make([]BatchDecryptItem, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		item.MessageID = strings.TrimSpace(item.MessageID)
		item.ConversationID = strings.TrimSpace(item.ConversationID)
		item.Ciphertext = strings.TrimSpace(item.Ciphertext)
		if item.MessageID == "" || item.Ciphertext == "" {
			continue
		}
		key := item.MessageID + "\x00" + item.ConversationID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func parseBatchDecryptResults(data map[string]any) []BatchDecryptItemResult {
	src := data
	if result, ok := data["result"].(map[string]any); ok {
		src = result
	}
	var raw []any
	for _, key := range []string{"items", "results", "messages"} {
		if arr, ok := src[key].([]any); ok {
			raw = arr
			break
		}
	}
	out := make([]BatchDecryptItemResult, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, BatchDecryptItemResult{
			MessageID:        stringField(m, "messageId", "openMessageId", "msgId"),
			ConversationID:   stringField(m, "conversationId", "openConversationId"),
			Status:           firstNonEmpty(stringField(m, "status"), "success"),
			PlaintextContent: stringField(m, "plaintextContent", "plaintext"),
			KeyVersion:       intField(m, "keyVersion"),
			Reason:           stringField(m, "reason", "error"),
		})
	}
	return out
}

func policyCacheKey(corpID string, opts Options) string {
	return strings.Join([]string{corpID, opts.OpenConversationID, opts.ReceiverOpenTalkID, opts.Identity, opts.MsgType}, "\x00")
}

func stringField(data map[string]any, names ...string) string {
	for _, name := range names {
		if value, ok := data[name]; ok && value != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func intField(data map[string]any, names ...string) int {
	for _, name := range names {
		switch value := data[name].(type) {
		case int:
			return value
		case int64:
			return int(value)
		case float64:
			return int(value)
		case string:
			var out int
			if _, err := fmt.Sscanf(value, "%d", &out); err == nil {
				return out
			}
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
