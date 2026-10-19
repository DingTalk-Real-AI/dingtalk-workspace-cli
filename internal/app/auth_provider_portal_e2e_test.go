// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

// TestAntOIDCPortalEndToEnd runs the built CLI against the portal backend's
// Mock SDK. Normal tests never start a server or contact an OP.
func TestAntOIDCPortalEndToEnd(t *testing.T) {
	base, binary := os.Getenv("DWS_OIDC_MOCK_URL"), os.Getenv("DWS_OIDC_E2E_BIN")
	if base == "" && binary == "" {
		t.Skip("set DWS_OIDC_MOCK_URL and DWS_OIDC_E2E_BIN for the portal E2E")
	}
	base = strings.TrimRight(base, "/")
	if err := authpkg.NewOIDCLoginProvider("ant", base, nil).Validate(); err != nil {
		t.Fatal("DWS_OIDC_MOCK_URL must be a valid HTTPS portal URL (HTTP is allowed only for loopback tests)")
	}
	if binary == "" {
		t.Fatal("DWS_OIDC_E2E_BIN must point to a built executable")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal("invalid DWS_OIDC_E2E_BIN")
	}
	if stat, err := os.Stat(binary); err != nil || stat.IsDir() || stat.Mode()&0111 == 0 {
		t.Fatal("DWS_OIDC_E2E_BIN must point to a built executable")
	}
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(base), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv("DWS_KEYCHAIN_DIR", t.TempDir())
	t.Setenv("DWS_DISABLE_KEYCHAIN", "1")
	t.Setenv("DWS_NO_UPDATE_CHECK", "1")
	t.Setenv("DO_NOT_TRACK", "1")
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binary, "auth", "provider-login", "ant", "--login-timeout", "30", "-f", "json")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatal("DWS did not complete Mock SDK login")
	}
	var login struct {
		Success bool   `json:"success"`
		CorpID  string `json:"corp_id"`
	}
	if json.Unmarshal(stdout.Bytes(), &login) != nil || !login.Success {
		t.Fatal("DWS did not return a valid SDK login success result")
	}
	token, err := authpkg.LoadTokenData(configDir)
	if err != nil || token == nil || token.AccessToken == "" || token.RefreshToken == "" || !token.IsAccessTokenValid() || !token.IsRefreshTokenValid() {
		t.Fatal("DWS did not persist both valid credentials")
	}
	if login.CorpID != token.CorpID {
		t.Fatal("login result corp_id does not match the saved session")
	}
	for _, secret := range []string{token.AccessToken, token.RefreshToken} {
		if bytes.Contains(stdout.Bytes(), []byte(secret)) || bytes.Contains(stderr.Bytes(), []byte(secret)) {
			t.Fatal("DWS leaked a credential in login output")
		}
	}
	storedURL, err := os.ReadFile(filepath.Join(configDir, "mcp_url"))
	if err != nil || strings.TrimSpace(string(storedURL)) != base {
		t.Fatal("DWS changed the configured portal endpoint")
	}
	status, err := exec.CommandContext(ctx, binary, "auth", "status", "--read-only", "-f", "json").Output()
	var state struct {
		TokenValid bool `json:"token_valid"`
	}
	if err != nil || json.Unmarshal(status, &state) != nil || !state.TokenValid {
		t.Fatal("a new DWS process could not read the saved login")
	}
}
