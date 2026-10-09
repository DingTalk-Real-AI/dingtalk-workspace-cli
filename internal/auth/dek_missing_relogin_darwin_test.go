// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

//go:build darwin

package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func seedMissingDEKLogin(t *testing.T) (string, *TokenData) {
	t.Helper()
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	old := testToken("old-access", "corp-missing-dek", "Missing DEK Org")
	old.UserID = "user-missing-dek"
	if err := SaveTokenData(configDir, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(keychain.StorageDir(keychain.Service), "dek")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTokenDataKeychain(); !keychain.IsDEKMissing(err) {
		t.Fatalf("legacy read = %v, want missing DEK", err)
	}
	return configDir, old
}

func TestCrossPlatformCoverageMissingDEKLoginPreparationPreservesCiphertext(t *testing.T) {
	configDir, _ := seedMissingDEKLogin(t)
	path := filepath.Join(keychain.StorageDir(keychain.Service), "auth-token.enc")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareLoginPersistence(configDir); err != nil {
		t.Fatalf("prepare login = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("preparation changed old ciphertext: %v", err)
	}
	if _, err := os.Stat(filepath.Join(keychain.StorageDir(keychain.Service), "dek")); !os.IsNotExist(err) {
		t.Fatalf("preparation created DEK: %v", err)
	}
}

func TestCrossPlatformCoverageMissingDEKFreshLoginReplacesOnlyTargets(t *testing.T) {
	configDir, old := seedMissingDEKLogin(t)
	unrelated := profileCiphertextPathForTest("another-corp")
	untouched := []byte("unrelated ciphertext")
	if err := os.WriteFile(unrelated, untouched, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := preflightTokenRefreshPersistence(configDir, old); !keychain.IsDEKMissing(err) {
		t.Fatalf("refresh preflight = %v, want missing DEK", err)
	}
	if err := repairLoginCiphertextMismatchTargets(configDir, old); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTokenDataKeychain(); !keychain.IsDEKMissing(err) {
		t.Fatalf("non-fresh repair changed old slot: %v", err)
	}
	fresh := *old
	fresh.AccessToken = "new-access"
	if err := SaveLoginTokenData(configDir, &fresh); err != nil {
		t.Fatalf("fresh login = %v", err)
	}
	for _, account := range []string{keychain.AccountToken, TokenAccountForCorpID(old.CorpID), TokenAccountForIdentity(old.CorpID, old.UserID)} {
		got, err := loadTokenDataKeychainAccount(account)
		if err != nil || got == nil || got.AccessToken != fresh.AccessToken {
			t.Fatalf("target %q not replaced: %v", account, err)
		}
	}
	after, err := os.ReadFile(unrelated)
	if err != nil || !bytes.Equal(after, untouched) {
		t.Fatalf("unrelated slot changed: %v", err)
	}
}

func TestCrossPlatformCoverageMissingDEKOAuthStartsAuthorization(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "force"}[force], func(t *testing.T) {
			configDir, _ := seedMissingDEKLogin(t)
			setPreflightTestCredentials(t)
			listenerErr := errors.New("authorization reached")
			calls := 0
			testseam.Swap(t, &oauthListen, func(string, string) (net.Listener, error) {
				calls++
				return nil, listenerErr
			})
			provider := NewOAuthProvider(configDir, nil)
			provider.NoBrowser = true
			_, err := provider.Login(context.Background(), force)
			if calls == 0 || !errors.Is(err, listenerErr) {
				t.Fatalf("authorization calls = %d, error = %v", calls, err)
			}
		})
	}
}

func TestCrossPlatformCoverageMissingDEKAuthCodeExchangePersistsNewToken(t *testing.T) {
	configDir, old := seedMissingDEKLogin(t)
	setPreflightTestCredentials(t)
	provider := NewOAuthProvider(configDir, nil)
	calls := 0
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(
			`{"accessToken":"new-access","refreshToken":"new-refresh","expiresIn":7200,"corpId":"corp-missing-dek"}`,
		))}, nil
	})}
	got, err := provider.ExchangeAuthCode(context.Background(), "code", old.UserID)
	if err != nil || got == nil || got.AccessToken != "new-access" || calls != 1 {
		t.Fatalf("exchange calls = %d, error = %v", calls, err)
	}
	stored, err := LoadTokenDataForProfile(configDir, "")
	if err != nil || stored == nil || stored.AccessToken != "new-access" {
		t.Fatalf("saved login unreadable: %v", err)
	}
}

func TestCrossPlatformCoverageMissingDEKDeviceStartsAuthorization(t *testing.T) {
	configDir, _ := seedMissingDEKLogin(t)
	setPreflightTestCredentials(t)
	provider := NewDeviceFlowProvider(configDir, nil)
	provider.Output = io.Discard
	requestErr := errors.New("device authorization reached")
	calls := 0
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, requestErr
	})}
	_, err := provider.Login(context.Background())
	if calls == 0 || !errors.Is(err, requestErr) {
		t.Fatalf("device authorization calls = %d, error = %v", calls, err)
	}
}

// TestCrossPlatformCoverageMissingDEKV1MultiProfileReloginPersists reproduces
// the v1 dual-profile registry blocker: after the shared DEK is lost, the v1
// migration runs during persistence and reads the non-target organization's
// slot. That read must not strand the fresh login, and the non-target
// ciphertext must stay intact.
func TestCrossPlatformCoverageMissingDEKV1MultiProfileReloginPersists(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()

	target := testToken("old-target", "corp-target", "Target Org")
	other := testToken("old-other", "corp-other", "Other Org")
	if err := SaveTokenData(configDir, target); err != nil {
		t.Fatal(err)
	}
	if err := SaveTokenData(configDir, other); err != nil {
		t.Fatal(err)
	}
	// Rewrite the registry as a valid v1 dual-profile install that still has
	// global/organization/identity ciphertext for both organizations.
	raw := `{
  "version": 1,
  "currentProfile": "corp-target",
  "profiles": [
    {"name":"Target Org","corpId":"corp-target","corpName":"Target Org","userId":"user_corp-target"},
    {"name":"Other Org","corpId":"corp-other","corpName":"Other Org","userId":"user_corp-other"}
  ]
}`
	if err := os.WriteFile(ProfilesPath(configDir), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	// Delete the shared DEK.
	if err := os.Remove(filepath.Join(keychain.StorageDir(keychain.Service), "dek")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTokenDataKeychain(); !keychain.IsDEKMissing(err) {
		t.Fatalf("legacy read = %v, want missing DEK", err)
	}

	// Snapshot the non-target organization's ciphertext before the fresh login.
	otherOrgPath := profileCiphertextPathForTest(other.CorpID)
	otherIdentityPath := filepath.Join(keychain.StorageDir(keychain.Service),
		strings.ReplaceAll(TokenAccountForIdentity(other.CorpID, other.UserID), ":", "_")+".enc")
	orgBefore, err := os.ReadFile(otherOrgPath)
	if err != nil {
		t.Fatal(err)
	}
	identityBefore, err := os.ReadFile(otherIdentityPath)
	if err != nil {
		t.Fatal(err)
	}

	fresh := *target
	fresh.AccessToken = "new-access"
	if err := SaveLoginTokenData(configDir, &fresh); err != nil {
		t.Fatalf("fresh login = %v", err)
	}

	// The v1 migration must not touch non-target ciphertext.
	orgAfter, err := os.ReadFile(otherOrgPath)
	if err != nil || !bytes.Equal(orgAfter, orgBefore) {
		t.Fatalf("non-target org slot changed: %v", err)
	}
	identityAfter, err := os.ReadFile(otherIdentityPath)
	if err != nil || !bytes.Equal(identityAfter, identityBefore) {
		t.Fatalf("non-target identity slot changed: %v", err)
	}

	// The fresh credential is persisted and readable.
	stored, err := LoadTokenDataForProfile(configDir, "")
	if err != nil || stored == nil || stored.AccessToken != "new-access" {
		t.Fatalf("saved login unreadable: %v", err)
	}
}
