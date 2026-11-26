// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package msgcrypto

import (
	"context"
	"errors"
	"fmt"
	"os"
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

	ephemeralKeystoreDir string
}

// Close releases the underlying cipher and removes any session-scoped
// ephemeral keystore directory created for stateless token auth.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	var closeErr error
	if s.Cipher != nil {
		closeErr = s.Cipher.Close()
	}
	var removeErr error
	if s.ephemeralKeystoreDir != "" {
		if err := sessionRemoveAll(s.ephemeralKeystoreDir); err != nil {
			removeErr = fmt.Errorf("清理临时密钥目录失败: %w", err)
		} else {
			s.ephemeralKeystoreDir = ""
		}
	}
	return errors.Join(closeErr, removeErr)
}

// SessionOptions configures OpenSession.
type SessionOptions struct {
	ConfigDir           string
	CLIVersion          string
	KeyServer           string
	AllowedRedirectHost string
	KeystoreDir         string
	AccessToken         string
	ClientID            string
	CorpID              string
	StaffID             string
	Debug               bool
	Logf                func(format string, args ...any)
}

var (
	sessionAvailable        = Available
	sessionOpenBackend      = Open
	sessionCurrentIdentity  = CurrentIdentity
	sessionMkdirTemp        = os.MkdirTemp
	sessionRemoveAll        = os.RemoveAll
	sessionNewOAuthProvider = func(configDir string) tokenSnapshotProvider {
		return auth.NewOAuthProvider(configDir, nil)
	}
)

type tokenSnapshotProvider interface {
	GetTokenSnapshot(context.Context) (*auth.TokenData, error)
}

// CurrentIdentity reads the current login snapshot without opening SafeChat.
func CurrentIdentity(ctx context.Context, configDir string) (Identity, error) {
	if strings.TrimSpace(configDir) == "" {
		configDir = config.DefaultConfigDir()
	}
	snap, err := sessionNewOAuthProvider(configDir).GetTokenSnapshot(ctx)
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

// OpenSession opens a SafeChat cipher for the current login organization, or
// for an explicit stateless access token when opts.AccessToken is set.
func OpenSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	if !sessionAvailable() {
		return nil, ErrUnavailable
	}
	explicitToken := strings.TrimSpace(opts.AccessToken)
	stateless := explicitToken != ""

	var (
		identity Identity
		err      error
	)
	if stateless {
		corpID := strings.TrimSpace(opts.CorpID)
		if corpID == "" {
			return nil, ErrNoCorpID
		}
		staffID := strings.TrimSpace(opts.StaffID)
		if staffID == "" {
			staffID = "dws-safechat"
		}
		identity = Identity{CorpID: corpID, StaffID: staffID}
	} else {
		identity, err = sessionCurrentIdentity(ctx, opts.ConfigDir)
		if err != nil {
			return nil, err
		}
	}

	keyServer := strings.TrimSpace(opts.KeyServer)
	if keyServer == "" {
		keyServer = DefaultSafeChatKeyServer
	}
	allowedHost := strings.TrimSpace(opts.AllowedRedirectHost)
	if allowedHost == "" {
		allowedHost = DefaultSafeChatRedirectHost
	}

	keystoreDir := strings.TrimSpace(opts.KeystoreDir)
	ephemeralDir := ""
	if stateless && keystoreDir == "" {
		ephemeralDir, err = sessionMkdirTemp("", "dws-safechat-*")
		if err != nil {
			return nil, fmt.Errorf("创建临时密钥目录失败: %w", err)
		}
		keystoreDir = ephemeralDir
	}

	var authCode AuthCodeProvider
	if stateless {
		authCode = NewStaticTokenPortalAuthCode(explicitToken, opts.ClientID, identity.CorpID, opts.CLIVersion)
	} else {
		authCode = NewPortalAuthCode(configDirOrDefault(opts.ConfigDir), opts.CLIVersion)
	}

	cfg := Config{
		AuthCode:            authCode,
		KeyServer:           keyServer,
		AllowedRedirectHost: allowedHost,
		KeystoreDir:         keystoreDir,
		Debug:               opts.Debug,
		Logf:                opts.Logf,
	}
	cipher, err := sessionOpenBackend(ctx, cfg)
	if err != nil {
		var cleanupErr error
		if ephemeralDir != "" {
			if rerr := sessionRemoveAll(ephemeralDir); rerr != nil {
				cleanupErr = fmt.Errorf("清理临时密钥目录失败: %w", rerr)
			}
		}
		if errors.Is(err, ErrUnavailable) {
			return nil, errors.Join(err, cleanupErr)
		}
		return nil, errors.Join(fmt.Errorf("初始化加解密后端失败: %w", err), cleanupErr)
	}
	return &Session{
		Cipher:               cipher,
		CorpID:               identity.CorpID,
		StaffID:              identity.StaffID,
		KeystoreDir:          cfg.withDefaults().KeystoreDir,
		ephemeralKeystoreDir: ephemeralDir,
	}, nil
}

func configDirOrDefault(configDir string) string {
	if strings.TrimSpace(configDir) == "" {
		return config.DefaultConfigDir()
	}
	return strings.TrimSpace(configDir)
}
