// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

func TestResolveAuxiliaryAccessToken_explicitToken(t *testing.T) {
	tok, err := ResolveAuxiliaryAccessToken(context.Background(), "/any/dir", "  bearer-xyz  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "bearer-xyz" {
		t.Fatalf("got %q, want bearer-xyz", tok)
	}
}

func TestResolveAuxiliaryAccessToken_emptyConfigDir(t *testing.T) {
	_, err := ResolveAuxiliaryAccessToken(context.Background(), "  ", "")
	if err == nil {
		t.Fatal("expected error for empty config directory")
	}
}

func TestResolveAccessTokenFromDirPreservesRefreshFailure(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, filepath.Join(root, "keychain"))

	if err := authpkg.SaveTokenData(configDir, &authpkg.TokenData{
		AccessToken:  "expired-access",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       "corp_refresh",
		UserID:       "user_refresh",
		ClientID:     "client_refresh",
		Source:       "mcp",
	}); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})
	http.DefaultTransport = refreshFailureRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("refresh endpoint rejected token")
	})

	token, err := resolveAccessTokenFromDir(context.Background(), configDir)
	if token != "" {
		t.Fatalf("token = %q, want empty", token)
	}
	if err == nil {
		t.Fatal("resolveAccessTokenFromDir() error = nil")
	}
	if !strings.Contains(err.Error(), "refresh endpoint rejected token") {
		t.Fatalf("error = %q, want original refresh failure", err)
	}
}

type refreshFailureRoundTripFunc func(*http.Request) (*http.Response, error)

func (f refreshFailureRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
