// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

//go:build darwin || linux

package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

func TestCrossPlatformCoverageMissingDEKSequentialProfileRelogin(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()

	first := testToken("old-first", "corp-first", "First Org")
	second := testToken("old-second", "corp-second", "Second Org")
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}
	if err := os.Remove(filepath.Join(keychain.StorageDir(keychain.Service), "dek")); err != nil {
		t.Fatalf("remove old DEK: %v", err)
	}

	freshFirst := *first
	freshFirst.AccessToken = "new-first"
	if err := SaveLoginTokenData(configDir, &freshFirst); err != nil {
		t.Fatalf("SaveLoginTokenData(first) error = %v", err)
	}
	if _, err := LoadTokenDataForProfile(configDir, TokenProfileSelector(second)); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataForProfile(second) error = %v, want ciphertext key mismatch", err)
	}
	freshSecond := *second
	freshSecond.AccessToken = "new-second"
	if err := SaveLoginTokenData(configDir, &freshSecond); err != nil {
		t.Fatalf("SaveLoginTokenData(second) error = %v", err)
	}

	for _, want := range []*TokenData{&freshFirst, &freshSecond} {
		got, err := LoadTokenDataForProfile(configDir, TokenProfileSelector(want))
		if err != nil || got == nil || got.AccessToken != want.AccessToken {
			t.Fatalf("LoadTokenDataForProfile(%q) = %#v, %v; want access token %q",
				TokenProfileSelector(want), got, err, want.AccessToken)
		}
	}
}

func TestCrossPlatformCoverageV1ExplicitProfileMissingDEKRequestsRetry(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv("DWS_DEBUG_AUTH", "1")
	configDir := t.TempDir()

	first := testToken("old-first", "corp-first", "First Org")
	second := testToken("old-second", "corp-second", "Second Org")
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}
	v1 := &ProfilesConfig{
		Version:        1,
		CurrentProfile: first.CorpID,
		Profiles: []Profile{
			{Name: first.CorpName, CorpID: first.CorpID, CorpName: first.CorpName, UserID: first.UserID},
			{Name: second.CorpName, CorpID: second.CorpID, CorpName: second.CorpName, UserID: second.UserID},
		},
	}
	raw, err := json.Marshal(v1)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProfilesPath(configDir), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(keychain.StorageDir(keychain.Service), "dek")); err != nil {
		t.Fatalf("remove old DEK: %v", err)
	}
	SetRuntimeProfile(TokenProfileSelector(first))

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	fresh := *first
	fresh.AccessToken = "new-first"
	err = SaveLoginTokenData(configDir, &fresh)
	if err == nil || !keychain.IsDEKMissing(err) {
		t.Fatalf("first SaveLoginTokenData() error = %v, want missing DEK", err)
	}
	// The guidance copy is locale-aware, so compare against the active catalog
	// entry instead of hard-coding the Chinese text.
	wantGuidance := i18n.T(profileLoginRetryGuidance)
	if err.Error() != wantGuidance {
		t.Fatalf("first SaveLoginTokenData() error = %q, want %q", err, wantGuidance)
	}
	for _, implementationDetail := range []string{"v1", "v2", "DEK", "token", "配置"} {
		if strings.Contains(err.Error(), implementationDetail) {
			t.Fatalf("first SaveLoginTokenData() error exposed %q: %v", implementationDetail, err)
		}
	}
	if got, ok := LoginRetryGuidance(err); !ok || got != wantGuidance {
		t.Fatalf("LoginRetryGuidance() = %q, %v; want %q, true", got, ok, wantGuidance)
	}
	wrapped := fmt.Errorf("保存 token 失败: %w", err)
	if got, ok := LoginRetryGuidance(wrapped); !ok || got != wantGuidance {
		t.Fatalf("LoginRetryGuidance(wrapped) = %q, %v; want %q, true", got, ok, wantGuidance)
	}
	gotLogs := logs.String()
	for _, want := range []string{
		`"msg":"auth.token.persist.retry_after_profile_migration"`,
		`"from_version":1`,
		`"to_version":2`,
		`"reason":"organization_slot_dek_missing"`,
	} {
		if !strings.Contains(gotLogs, want) {
			t.Fatalf("diagnostic logs missing %q:\n%s", want, gotLogs)
		}
	}
	if strings.Contains(gotLogs, fresh.AccessToken) {
		t.Fatalf("diagnostic logs exposed access token:\n%s", gotLogs)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil || cfg.Version != profilesVersion {
		t.Fatalf("profiles after first login = %#v, %v; want v%d", cfg, err, profilesVersion)
	}
	if err := SaveLoginTokenData(configDir, &fresh); err != nil {
		t.Fatalf("second SaveLoginTokenData() error = %v", err)
	}
	stored, err := LoadTokenDataForProfile(configDir, TokenProfileSelector(&fresh))
	if err != nil || stored == nil || stored.AccessToken != fresh.AccessToken {
		t.Fatalf("stored token after retry = %#v, %v; want access token %q", stored, err, fresh.AccessToken)
	}
}
