// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package msgcrypto

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const (
	// DefaultSafeChatKeyServer is the verified SafeChat key server URL.
	DefaultSafeChatKeyServer = "https://server.safeding.com/DDSecureInter/getCorpSecureKey"
	// DefaultSafeChatRedirectHost is the host the vendor SDK reports to goProxy.
	DefaultSafeChatRedirectHost = "server.safeding.com"
)

// Identity is the current DingTalk login organization and staff id used by
// message crypto. It contains no token or key material.
type Identity struct {
	CorpID  string
	StaffID string
}

// Session owns one opened SafeChat cipher for the current organization.
type Session struct {
	Cipher      Cipher
	CorpID      string
	StaffID     string
	KeystoreDir string
}

// Close releases the underlying cipher.
func (s *Session) Close() error {
	if s == nil || s.Cipher == nil {
		return nil
	}
	return s.Cipher.Close()
}

// SessionOptions configures OpenSession.
type SessionOptions struct {
	ConfigDir           string
	CLIVersion          string
	KeyServer           string
	AllowedRedirectHost string
	KeystoreDir         string
	Debug               bool
	Logf                func(format string, args ...any)
}

// CurrentIdentity reads the current login snapshot without opening SafeChat.
func CurrentIdentity(ctx context.Context, configDir string) (Identity, error) {
	if strings.TrimSpace(configDir) == "" {
		configDir = config.DefaultConfigDir()
	}
	snap, err := auth.NewOAuthProvider(configDir, nil).GetTokenSnapshot(ctx)
	if err != nil {
		return Identity{}, fmt.Errorf("读取登录态失败（先 dws auth login）: %w", err)
	}
	corpID := strings.TrimSpace(snap.CorpID)
	if corpID == "" {
		return Identity{}, ErrNoCorpID
	}
	staffID := strings.TrimSpace(snap.UserID)
	if staffID == "" {
		staffID = "dws-safechat"
	}
	return Identity{CorpID: corpID, StaffID: staffID}, nil
}

// OpenSession opens a SafeChat cipher for the current login organization.
func OpenSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	if !Available() {
		return nil, ErrUnavailable
	}
	identity, err := CurrentIdentity(ctx, opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	keyServer := strings.TrimSpace(opts.KeyServer)
	if keyServer == "" {
		keyServer = DefaultSafeChatKeyServer
	}
	allowedHost := strings.TrimSpace(opts.AllowedRedirectHost)
	if allowedHost == "" {
		allowedHost = DefaultSafeChatRedirectHost
	}
	cfg := Config{
		AuthCode:            NewPortalAuthCode(configDirOrDefault(opts.ConfigDir), opts.CLIVersion),
		KeyServer:           keyServer,
		AllowedRedirectHost: allowedHost,
		KeystoreDir:         strings.TrimSpace(opts.KeystoreDir),
		Debug:               opts.Debug,
		Logf:                opts.Logf,
	}
	cipher, err := Open(ctx, cfg)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			return nil, err
		}
		return nil, fmt.Errorf("初始化加解密后端失败: %w", err)
	}
	return &Session{
		Cipher:      cipher,
		CorpID:      identity.CorpID,
		StaffID:     identity.StaffID,
		KeystoreDir: cfg.withDefaults().KeystoreDir,
	}, nil
}

func configDirOrDefault(configDir string) string {
	if strings.TrimSpace(configDir) == "" {
		return config.DefaultConfigDir()
	}
	return strings.TrimSpace(configDir)
}
