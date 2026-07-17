// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build darwin

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

type preflightRoundTripFunc func(*http.Request) (*http.Response, error)

func (f preflightRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func seedUnreadableTokenStorage(t *testing.T, configDir string, data *TokenData) {
	t.Helper()
	t.Setenv(keychain.DisableKeychainEnv, "1")
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x7f}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(replacement DEK) error = %v", err)
	}
}

func setPreflightTestCredentials(t *testing.T) {
	t.Helper()
	SetClientID("preflight-client-id")
	SetClientSecret("preflight-client-secret")
	resetClientIDFromMCP()
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
	})
}

func profileCiphertextPathForTest(corpID string) string {
	account := strings.ReplaceAll(TokenAccountForCorpID(corpID), ":", "_")
	return filepath.Join(keychain.StorageDir(keychain.Service), account+".enc")
}

func TestLoadTokenDataFallsBackToLegacyOnlyWhenCurrentSlotIsMissing(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_fallback", "corp_fallback", "Fallback Org")

	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if err := DeleteTokenDataKeychainForCorpID(data.CorpID); err != nil {
		t.Fatalf("DeleteTokenDataKeychainForCorpID() error = %v", err)
	}
	if err := preflightTokenPersistence(configDir); err != nil {
		t.Fatalf("preflightTokenPersistence() with missing profile slot error = %v", err)
	}

	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loaded.AccessToken != data.AccessToken {
		t.Fatalf("fallback access token = %q, want %q", loaded.AccessToken, data.AccessToken)
	}
}

func TestLoadTokenDataUsesIdentitySlotWhenOrganizationMirrorIsUnreadable(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_unreadable", "corp_unreadable", "Unreadable Org")

	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if err := os.WriteFile(profileCiphertextPathForTest(data.CorpID), []byte("corrupt ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile ciphertext) error = %v", err)
	}

	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loaded == nil || loaded.AccessToken != data.AccessToken || loaded.UserID != data.UserID {
		t.Fatalf("LoadTokenData() = %#v, want identity token %#v", loaded, data)
	}
}

func TestPreflightTokenPersistenceAllowsEmptyStorageWithoutCreatingDEK(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()

	if err := preflightTokenPersistence(configDir); err != nil {
		t.Fatalf("preflightTokenPersistence() error = %v", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	if _, err := os.Stat(dekPath); !os.IsNotExist(err) {
		t.Fatalf("preflight created a DEK at %q; stat error = %v", dekPath, err)
	}
}

func TestPreflightTokenPersistenceRejectsUnreadableProfileSlot(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_preflight", "corp_preflight", "Preflight Org")

	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if err := os.WriteFile(profileCiphertextPathForTest(data.CorpID), []byte("corrupt ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile ciphertext) error = %v", err)
	}

	err := preflightTokenPersistence(configDir)
	if err == nil || !strings.Contains(err.Error(), "profile token slot") {
		t.Fatalf("preflightTokenPersistence() error = %v, want unreadable profile slot", err)
	}
	if !strings.Contains(err.Error(), "dws auth logout --profile \""+data.CorpID+"\"") {
		t.Fatalf("preflightTokenPersistence() error = %v, want per-profile recovery hint", err)
	}
}

func TestExactOrgCurrentRefreshRejectsUnreadableOrgMirror(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_exact_refresh", "corp_exact", "Exact Org")
	data.UserID = "user_exact"
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if err := os.WriteFile(profileCiphertextPathForTest(data.CorpID), []byte("corrupt ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile ciphertext) error = %v", err)
	}

	SetRuntimeProfile("corp_exact:user_exact")
	defer SetRuntimeProfile("")
	if err := preflightTokenRefreshPersistence(configDir, data); err == nil ||
		!strings.Contains(err.Error(), "profile token slot") {
		t.Fatalf("preflightTokenRefreshPersistence(exact current) error = %v, want unreadable org mirror", err)
	}
}

func TestExactNonOrgCurrentRefreshIgnoresUnreadableOrgMirror(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	first := testToken("at_first", "corp_exact", "Exact Org")
	first.UserID = "user_first"
	second := testToken("at_second", "corp_exact", "Exact Org")
	second.UserID = "user_second"
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}
	if err := os.WriteFile(profileCiphertextPathForTest(first.CorpID), []byte("corrupt ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile ciphertext) error = %v", err)
	}

	SetRuntimeProfile("corp_exact:user_first")
	defer SetRuntimeProfile("")
	if err := preflightTokenRefreshPersistence(configDir, first); err != nil {
		t.Fatalf("preflightTokenRefreshPersistence(exact non-current) error = %v", err)
	}
	updated := *first
	updated.AccessToken = "at_first_refreshed"
	updated.RefreshToken = "rt_first_refreshed"
	if err := SaveTokenData(configDir, &updated); err != nil {
		t.Fatalf("SaveTokenData(exact non-current refresh) error = %v", err)
	}
	loaded, err := LoadTokenDataForProfile(configDir, "corp_exact:user_first")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(refreshed) error = %v", err)
	}
	if loaded.AccessToken != updated.AccessToken || loaded.RefreshToken != updated.RefreshToken {
		t.Fatalf("refreshed exact token = %#v, want %#v", loaded, updated)
	}
}

func TestExchangeAuthCodePreflightsOrphanProfileCiphertextBeforeHTTP(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	setPreflightTestCredentials(t)
	configDir := t.TempDir()
	data := testToken("at_orphan", "corp_orphan", "Orphan Org")

	// Simulate interruption after the profile ciphertext rename but before
	// profiles.json is updated by saveTokenDataLocked.
	if err := SaveTokenDataKeychainForCorpID(data.CorpID, data); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, profilesJSONFile)); !os.IsNotExist(err) {
		t.Fatalf("profiles.json stat error = %v, want missing metadata", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x6f}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(replacement DEK) error = %v", err)
	}

	var calls atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected HTTP request")
	})}
	_, err := provider.ExchangeAuthCode(context.Background(), "auth-code", "")
	if err == nil || !strings.Contains(err.Error(), "auth token ciphertext inventory") {
		t.Fatalf("ExchangeAuthCode() error = %v, want orphan ciphertext preflight error", err)
	}
	if !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("ExchangeAuthCode() error = %v, want ciphertext key mismatch in error chain", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("HTTP calls = %d, want 0", got)
	}
}

func TestPortableAuthExportRejectsCiphertextFromAnotherDEK(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_portable", "", "")
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if !PortableAuthSourceReady() {
		t.Fatal("PortableAuthSourceReady() = false before replacing DEK")
	}

	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x7f}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(replacement DEK) error = %v", err)
	}
	if PortableAuthSourceReady() {
		t.Fatal("PortableAuthSourceReady() = true for ciphertext from another DEK")
	}
	var bundle bytes.Buffer
	if err := ExportPortableAuthBundle(configDir, &bundle); err == nil {
		t.Fatal("ExportPortableAuthBundle() error = nil for ciphertext from another DEK")
	}
	if bundle.Len() != 0 {
		t.Fatalf("ExportPortableAuthBundle() wrote %d bytes, want 0", bundle.Len())
	}
}

func TestRefreshPreflightIgnoresUnreadableUnrelatedProfile(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	dataA := testToken("at_a", "corp_a", "A Org")
	dataB := testToken("at_b", "corp_b", "B Org")
	if err := SaveTokenData(configDir, dataA); err != nil {
		t.Fatalf("SaveTokenData(A) error = %v", err)
	}
	if err := SaveTokenData(configDir, dataB); err != nil {
		t.Fatalf("SaveTokenData(B) error = %v", err)
	}
	if err := os.WriteFile(profileCiphertextPathForTest(dataA.CorpID), []byte("corrupt ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(A profile ciphertext) error = %v", err)
	}

	if err := preflightTokenRefreshPersistence(configDir, dataB); err != nil {
		t.Fatalf("preflightTokenRefreshPersistence(B) error = %v", err)
	}
	loaded, err := NewOAuthProvider(configDir, nil).Login(context.Background(), false)
	if err != nil {
		t.Fatalf("Login() with valid B and unreadable A error = %v", err)
	}
	if loaded.AccessToken != dataB.AccessToken {
		t.Fatalf("Login() access token = %q, want %q", loaded.AccessToken, dataB.AccessToken)
	}
}

func TestOAuthLoginPreflightsTokenPersistence(t *testing.T) {
	setPreflightTestCredentials(t)
	for _, force := range []bool{false, true} {
		t.Run("force="+map[bool]string{false: "false", true: "true"}[force], func(t *testing.T) {
			cleanupKeychain(t)
			configDir := t.TempDir()
			seedUnreadableTokenStorage(t, configDir, testToken("at_login", "corp_login", "Login Org"))

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			provider := NewOAuthProvider(configDir, nil)
			provider.NoBrowser = true
			_, err := provider.Login(ctx, force)
			if err == nil || !strings.Contains(err.Error(), "legacy token slot") {
				t.Fatalf("Login(force=%v) error = %v, want token persistence preflight error", force, err)
			}
		})
	}
}

func TestExchangeAuthCodePreflightsBeforeHTTP(t *testing.T) {
	cleanupKeychain(t)
	setPreflightTestCredentials(t)
	configDir := t.TempDir()
	seedUnreadableTokenStorage(t, configDir, testToken("at_exchange", "corp_exchange", "Exchange Org"))

	var calls atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected HTTP request")
	})}
	_, err := provider.ExchangeAuthCode(context.Background(), "auth-code", "")
	if err == nil || !strings.Contains(err.Error(), "legacy token slot") {
		t.Fatalf("ExchangeAuthCode() error = %v, want token persistence preflight error", err)
	}
	if !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("ExchangeAuthCode() error = %v, want ciphertext key mismatch in error chain", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("HTTP calls = %d, want 0", got)
	}
}

func TestDeviceFlowLoginPreflightsBeforeDeviceCodeRequest(t *testing.T) {
	cleanupKeychain(t)
	setPreflightTestCredentials(t)
	configDir := t.TempDir()
	seedUnreadableTokenStorage(t, configDir, testToken("at_device", "corp_device", "Device Org"))

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected device code request", http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := NewDeviceFlowProvider(configDir, nil)
	provider.Output = io.Discard
	provider.SetBaseURL(server.URL)
	_, err := provider.Login(context.Background())
	if err == nil || !strings.Contains(err.Error(), "legacy token slot") {
		t.Fatalf("DeviceFlowProvider.Login() error = %v, want token persistence preflight error", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("device code requests = %d, want 0", got)
	}
}

func TestLockedRefreshPreflightsLegacyMirrorBeforeHTTP(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	setPreflightTestCredentials(t)
	configDir := t.TempDir()
	data := testToken("at_refresh", "corp_refresh", "Refresh Org")
	data.ExpiresAt = time.Now().Add(-time.Hour)
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	legacyPath := filepath.Join(keychain.StorageDir(keychain.Service), keychain.AccountToken+".enc")
	if err := os.WriteFile(legacyPath, []byte("corrupt legacy ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(legacy ciphertext) error = %v", err)
	}

	var calls atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected refresh request")
	})}
	_, err := provider.lockedRefresh(context.Background())
	if err == nil || !strings.Contains(err.Error(), "legacy token slot") {
		t.Fatalf("lockedRefresh() error = %v, want token persistence preflight error", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("refresh HTTP calls = %d, want 0", got)
	}
}

func TestLockedRefreshRejectsFutureProfilesVersionBeforeHTTP(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	setPreflightTestCredentials(t)
	configDir := t.TempDir()
	data := testToken("at_future_refresh", "corp_future", "Future Org")
	data.ExpiresAt = time.Now().Add(-time.Hour)
	data.RefreshExpAt = time.Now().Add(time.Hour)
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	cfg.Version = profilesVersion + 1
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(ProfilesPath(configDir), raw, 0o600); err != nil {
		t.Fatalf("write future profiles: %v", err)
	}

	var calls atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected refresh request")
	})}
	_, err = provider.Login(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("Login() error = %v, want future profiles rejection", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("refresh HTTP calls = %d, want 0", got)
	}
}

func TestExchangeAuthCodeAllowsFirstLogin(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	setPreflightTestCredentials(t)
	configDir := t.TempDir()

	var calls atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.Output = io.Discard
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"accessToken":"new-access","refreshToken":"new-refresh","expiresIn":7200,"corpId":"corp_new"}`,
			)),
		}, nil
	})}

	data, err := provider.ExchangeAuthCode(context.Background(), "new-code", "user-new")
	if err != nil {
		t.Fatalf("ExchangeAuthCode() error = %v", err)
	}
	if data.AccessToken != "new-access" || data.UserID != "user-new" {
		t.Fatalf("ExchangeAuthCode() data = %#v", data)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
}

func TestExchangeAuthCodeExplicitUIDSkipsIdentityOverride(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	setPreflightTestCredentials(t)
	configDir := t.TempDir()

	var identityCalls atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.IdentityEnricher = func(context.Context, *TokenData) error {
		identityCalls.Add(1)
		return errors.New("identity lookup should not run for explicit uid")
	}
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"accessToken":"new-access","refreshToken":"new-refresh","expiresIn":7200,"corpId":"corp_new"}`,
			)),
		}, nil
	})}

	data, err := provider.ExchangeAuthCode(context.Background(), "new-code", "explicit-user")
	if err != nil {
		t.Fatalf("ExchangeAuthCode() error = %v", err)
	}
	if data.UserID != "explicit-user" {
		t.Fatalf("ExchangeAuthCode() userId = %q, want explicit-user", data.UserID)
	}
	if got := identityCalls.Load(); got != 0 {
		t.Fatalf("IdentityEnricher calls = %d, want 0", got)
	}
}
