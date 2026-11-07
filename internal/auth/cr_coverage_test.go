// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

func TestCrossPlatformCoverageAppCredentialExplicitAndDerivedFailureBranches(t *testing.T) {
	t.Run("malformed app config", func(t *testing.T) {
		isolateAppCredentialKeychain(t)
		dir := t.TempDir()
		if err := os.WriteFile(GetAppConfigPath(dir), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveAppConfigCredentialPair(dir); err == nil || !strings.Contains(err.Error(), "load app config") {
			t.Fatalf("malformed app config error = %v", err)
		}
	})
	t.Run("strict resolver trims explicit secret", func(t *testing.T) {
		isolateAppCredentialKeychain(t)
		dir := t.TempDir()
		writeCredentialConfig(t, dir, "id", PlainSecret("  padded-secret  "))
		_, secret, _, _, err := ResolveAppCredentialsStrict(dir)
		if err != nil || secret != "padded-secret" {
			t.Fatalf("strict explicit secret = %q, %v", secret, err)
		}
	})
	t.Run("empty explicit secret", func(t *testing.T) {
		isolateAppCredentialKeychain(t)
		dir := t.TempDir()
		writeCredentialConfig(t, dir, "id", PlainSecret("   "))
		if _, err := ResolveAppConfigCredentialPair(dir); !errors.Is(err, ErrClientSecretEmpty) {
			t.Fatalf("empty explicit secret = %v", err)
		}
	})
	t.Run("plain conflict", func(t *testing.T) {
		entries := isolateAppCredentialKeychain(t)
		dir := t.TempDir()
		entries[secretAccountKey("id")] = "canonical"
		entries[legacyClientSecretAccountKey("id")] = "legacy"
		writeCredentialConfig(t, dir, "id", PlainSecret("plain"))
		if _, err := ResolveAppConfigCredentialPair(dir); !errors.Is(err, ErrClientSecretConflict) {
			t.Fatalf("plain conflict = %v", err)
		}
	})
	for _, tc := range []struct {
		name      string
		legacyRef bool
		readError bool
		conflict  bool
		samePeer  bool
		want      error
	}{
		{"canonical peer read error", false, true, false, false, ErrSecretResolve},
		{"canonical peer conflict", false, false, true, false, ErrClientSecretConflict},
		{"canonical matching peer migrates", false, false, false, true, nil},
		{"legacy peer read error", true, true, false, false, ErrSecretResolve},
		{"legacy peer conflict", true, false, true, false, ErrClientSecretConflict},
		{"legacy matching peer migrates", true, false, false, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := isolateAppCredentialKeychain(t)
			dir := t.TempDir()
			id, secret := "id", "secret"
			ref := secretAccountKey(id)
			entries[ref] = secret
			if tc.legacyRef {
				ref = legacyClientSecretAccountKey(id)
				entries[ref] = secret
			}
			writeCredentialConfig(t, dir, id, SecretInput{Ref: &SecretRef{Source: "keychain", ID: ref}})
			if tc.legacyRef {
				if tc.readError {
					secretKeychainGet = func(_ string, account string) (string, error) {
						if account == legacyClientSecretAccountKey(id) {
							return secret, nil
						}
						return "", errors.New("canonical unavailable")
					}
				} else if tc.conflict {
					entries[secretAccountKey(id)] = "different"
				} else if tc.samePeer {
					entries[secretAccountKey(id)] = secret
				}
			} else {
				if tc.readError {
					authKeychainGet = func(string, string) (string, error) { return "", errors.New("legacy unavailable") }
				} else if tc.conflict {
					entries[legacyClientSecretAccountKey(id)] = "different"
				} else if tc.samePeer {
					entries[legacyClientSecretAccountKey(id)] = secret
				}
			}
			pair, err := ResolveAppConfigCredentialPair(dir)
			if tc.want != nil {
				if !errors.Is(err, tc.want) {
					t.Fatalf("error = %v, want %v", err, tc.want)
				}
				return
			}
			if err != nil || pair.ClientSecret != secret {
				t.Fatalf("pair = %#v, %v", pair, err)
			}
		})
	}
	for _, tc := range []struct {
		name      string
		canonical error
		legacy    error
	}{
		{"canonical read error", errors.New("canonical"), nil},
		{"legacy read error", nil, errors.New("legacy")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateAppCredentialKeychain(t)
			dir := t.TempDir()
			writeCredentialConfig(t, dir, "id", SecretInput{})
			if tc.canonical != nil {
				secretKeychainGet = func(string, string) (string, error) { return "", tc.canonical }
			}
			if tc.legacy != nil {
				authKeychainGet = func(string, string) (string, error) { return "", tc.legacy }
			}
			if _, err := ResolveAppConfigCredentialPair(dir); !errors.Is(err, ErrSecretResolve) {
				t.Fatalf("derived read error = %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageAppCredentialMigrationFailureBranches(t *testing.T) {
	isolateAppCredentialKeychain(t)
	migrateAppConfigSecret("", nil, "")

	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrateAppConfigSecret(filepath.Join(blocked, "child"), &AppConfig{ClientID: "id"}, "secret")

	dir := t.TempDir()
	writeCredentialConfig(t, dir, "new-id", PlainSecret("new-secret"))
	migrateAppConfigSecret(dir, &AppConfig{ClientID: "old-id"}, "old-secret")
	migrateAppConfigSecret(dir, &AppConfig{ClientID: "new-id"}, "different-secret")
	if err := os.WriteFile(GetAppConfigPath(dir), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrateAppConfigSecret(dir, &AppConfig{ClientID: "new-id"}, "new-secret")
	writeCredentialConfig(t, dir, "new-id", SecretInput{Ref: &SecretRef{Source: "file", ID: filepath.Join(dir, "missing")}})
	migrateAppConfigSecret(dir, &AppConfig{ClientID: "new-id"}, "new-secret")
	writeCredentialConfig(t, dir, "new-id", PlainSecret("new-secret"))

	oldSet := secretKeychainSet
	secretKeychainSet = func(string, string, string) error { return errors.New("set failed") }
	migrateAppConfigSecret(dir, &AppConfig{ClientID: "new-id"}, "new-secret")
	secretKeychainSet = oldSet

	oldWrite := appConfigAtomicWrite
	appConfigAtomicWrite = func(string, []byte) error { return errors.New("write failed") }
	migrateAppConfigSecret(dir, &AppConfig{ClientID: "new-id"}, "new-secret")
	appConfigAtomicWrite = oldWrite

	if _, err := resolveConfigSecretWithoutMigration(nil); !errors.Is(err, ErrClientIDEmpty) {
		t.Fatalf("nil config = %v", err)
	}
	badRef := &AppConfig{ClientID: "id", ClientSecret: SecretInput{Ref: &SecretRef{Source: "keychain", ID: secretAccountKey("other")}}}
	if _, err := resolveConfigSecretWithoutMigration(badRef); !errors.Is(err, ErrClientSecretRefMismatch) {
		t.Fatalf("mismatched ref = %v", err)
	}
	missingRef := &AppConfig{ClientID: "id", ClientSecret: SecretInput{Ref: &SecretRef{Source: "file", ID: filepath.Join(t.TempDir(), "missing")}}}
	if _, err := resolveConfigSecretWithoutMigration(missingRef); !errors.Is(err, ErrSecretResolve) {
		t.Fatalf("unreadable explicit ref = %v", err)
	}
	zero := &AppConfig{ClientID: "id"}
	secretKeychainGet = func(string, string) (string, error) { return "", errors.New("read failed") }
	if _, err := resolveConfigSecretWithoutMigration(zero); !errors.Is(err, ErrSecretResolve) {
		t.Fatalf("derived read failure = %v", err)
	}
	entries := isolateAppCredentialKeychain(t)
	entries[secretAccountKey("id")] = "canonical"
	entries[legacyClientSecretAccountKey("id")] = "legacy"
	if _, err := resolveConfigSecretWithoutMigration(zero); !errors.Is(err, ErrClientSecretConflict) {
		t.Fatalf("derived conflict = %v", err)
	}
}

func TestCrossPlatformCoverageClientSecretStoreFailureBranches(t *testing.T) {
	isolateAppCredentialKeychain(t)
	if err := SaveClientSecret("", ""); err != nil {
		t.Fatal(err)
	}
	fail := errors.New("keychain failed")
	authKeychainSet = func(string, string, string) error { return fail }
	if err := SaveClientSecret("id", "secret"); !errors.Is(err, fail) {
		t.Fatalf("save failure = %v", err)
	}

	isolateAppCredentialKeychain(t)
	authKeychainGet = func(string, string) (string, error) { return "", fail }
	if _, err := LoadClientSecretStrict("id"); !errors.Is(err, fail) {
		t.Fatalf("canonical load failure = %v", err)
	}
	isolateAppCredentialKeychain(t)
	calls := 0
	authKeychainGet = func(string, string) (string, error) {
		calls++
		if calls == 2 {
			return "", fail
		}
		return "", nil
	}
	if _, err := LoadClientSecretStrict("id"); !errors.Is(err, fail) {
		t.Fatalf("legacy load failure = %v", err)
	}

	entries := isolateAppCredentialKeychain(t)
	entries[legacyClientSecretAccountKey("id")] = "legacy"
	authKeychainSet = func(string, string, string) error { return fail }
	if got, err := LoadClientSecretStrict("id"); err != nil || got != "legacy" {
		t.Fatalf("best-effort canonical copy = %q, %v", got, err)
	}
	entries = isolateAppCredentialKeychain(t)
	entries[legacyClientSecretAccountKey("id")] = "legacy"
	authKeychainRemove = func(string, string) error { return fail }
	if got, err := LoadClientSecretStrict("id"); err != nil || got != "legacy" {
		t.Fatalf("best-effort legacy cleanup = %q, %v", got, err)
	}
	isolateAppCredentialKeychain(t)
	authKeychainRemove = func(string, string) error { return fail }
	if err := SaveClientSecret("id", "secret"); err != nil {
		t.Fatalf("best-effort save cleanup = %v", err)
	}
}

func TestCrossPlatformCoverageAppConfigResetFailureBranches(t *testing.T) {
	oldAcquire := appConfigAcquireDualLock
	appConfigAcquireDualLock = func(context.Context, string) (*DualLock, error) { return nil, errors.New("lock failed") }
	if err := DeleteAppConfig(t.TempDir()); err == nil {
		t.Fatal("DeleteAppConfig lock failure succeeded")
	}
	appConfigAcquireDualLock = oldAcquire

	oldCleanup, oldRemove := appConfigRemoveCredentialEntries, appConfigRemove
	t.Cleanup(func() {
		appConfigAcquireDualLock = oldAcquire
		appConfigRemoveCredentialEntries, appConfigRemove = oldCleanup, oldRemove
	})
	appConfigRemoveCredentialEntries = func(string, ...string) error { return errors.New("cleanup failed") }
	if err := DeleteAppConfig(t.TempDir()); err == nil || !strings.Contains(err.Error(), "credential entries") {
		t.Fatalf("cleanup failure = %v", err)
	}
	appConfigRemoveCredentialEntries = func(string, ...string) error { return nil }
	appConfigRemove = func(string) error { return errors.New("remove failed") }
	if err := DeleteAppConfig(t.TempDir()); err == nil || !strings.Contains(err.Error(), "removing app config") {
		t.Fatalf("remove failure = %v", err)
	}
}

func TestCrossPlatformCoverageAppTokenResponseAndRedirectFailureBranches(t *testing.T) {
	oldClient, oldNewRequest := appTokenHTTPClient, appTokenNewRequest
	t.Cleanup(func() { appTokenHTTPClient, appTokenNewRequest = oldClient, oldNewRequest })
	appTokenHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("x"), config.MaxResponseBodySize+1)))}, nil
	})}
	if _, _, err := FetchAppToken(context.Background(), "id", "secret"); err == nil || !strings.Contains(err.Error(), "安全上限") {
		t.Fatalf("oversized token response = %v", err)
	}
	appTokenNewRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) {
		return nil, errors.New("request failed")
	}
	if _, _, err := FetchAppToken(context.Background(), "id", "secret"); err == nil || !strings.Contains(err.Error(), "creating request") {
		t.Fatalf("request creation failure = %v", err)
	}

	origin, _ := http.NewRequest(http.MethodGet, "https://api.dingtalk.com/x", nil)
	next, _ := http.NewRequest(http.MethodGet, "https://api.dingtalk.com/y", nil)
	if err := appTokenRedirectPolicy(next, nil); err != nil {
		t.Fatal(err)
	}
	via := make([]*http.Request, 10)
	for i := range via {
		via[i] = origin
	}
	if appTokenRedirectPolicy(next, via) == nil {
		t.Fatal("redirect limit succeeded")
	}
	cross, _ := http.NewRequest(http.MethodGet, "https://oapi.dingtalk.com/y", nil)
	if appTokenRedirectPolicy(cross, []*http.Request{origin}) == nil {
		t.Fatal("cross-origin token redirect succeeded")
	}
	explicit, _ := url.Parse("https://api.dingtalk.com:443/x")
	if appTokenPort(nil) != "443" || appTokenPort(explicit) != "443" || !sameAppTokenOrigin(origin.URL, explicit) {
		t.Fatal("app token origin/port normalization changed")
	}
}

func TestCrossPlatformCoverageOAuthCredentialEarlyFailureBranches(t *testing.T) {
	isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "env-id")
	t.Setenv(EnvClientSecret, "")
	p := NewOAuthProvider(t.TempDir(), nil)
	if _, err := p.Login(context.Background(), true); err == nil || !strings.Contains(err.Error(), "应用凭证配置无效") {
		t.Fatalf("OAuth half-env login = %v", err)
	}
	d := NewDeviceFlowProvider(t.TempDir(), nil)
	if _, err := d.Login(context.Background()); err == nil || !strings.Contains(err.Error(), "应用凭证配置无效") {
		t.Fatalf("device half-env login = %v", err)
	}
	if _, err := (&OAuthProvider{credentialErr: errors.New("bad pair")}).directCredentialPair(); err == nil {
		t.Fatal("direct credential error succeeded")
	}
	if _, err := (&OAuthProvider{credentialErr: errors.New("bad pair")}).exchangeCode(context.Background(), "code"); err == nil {
		t.Fatal("exchange with invalid credential pair succeeded")
	}
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	if _, err := (&OAuthProvider{configDir: t.TempDir()}).directCredentialPair(); err == nil {
		t.Fatal("missing direct credential pair succeeded")
	}
	dir := t.TempDir()
	writeCredentialConfig(t, dir, "id", SecretInput{Ref: &SecretRef{Source: "keychain", ID: secretAccountKey("other")}})
	if _, err := resolveOAuthCredentialPair(dir); !errors.Is(err, ErrClientSecretRefMismatch) {
		t.Fatalf("OAuth app config failure = %v", err)
	}
	writeCredentialConfig(t, dir, "config-id", PlainSecret("config-secret"))
	if pair, err := ResolveAppCredentialPair(dir, "", ""); err != nil || pair.ClientID != "config-id" {
		t.Fatalf("raw app-config fallback pair = %#v, %v", pair, err)
	}
	if pair, err := resolveAppCredentialPairWithoutMigration(dir, "flag-id", "flag-secret"); err != nil || pair.Source != "flag" {
		t.Fatalf("non-migrating flag pair = %#v, %v", pair, err)
	}
	t.Setenv(EnvClientID, "env-id")
	t.Setenv(EnvClientSecret, "env-secret")
	if pair, err := resolveAppCredentialPairWithoutMigration(dir, "", ""); err != nil || pair.Source != "env" {
		t.Fatalf("non-migrating env pair = %#v, %v", pair, err)
	}
	(*OAuthProvider)(nil).persistAppConfigIfNeeded()
}

func TestCrossPlatformCoverageExchangeAuthCodeWithoutExplicitUIDPersistsPair(t *testing.T) {
	isolateAppCredentialKeychain(t)
	oldExchange, oldSave := oauthExchange, oauthSaveToken
	t.Cleanup(func() { oauthExchange, oauthSaveToken = oldExchange, oldSave })
	oauthExchange = func(*OAuthProvider, context.Context, string) (*TokenData, error) {
		return &TokenData{AccessToken: "access", CorpID: "corp", UserID: "user"}, nil
	}
	oauthSaveToken = func(string, *TokenData) error { return nil }
	p := &OAuthProvider{
		configDir:   t.TempDir(),
		credentials: &AppCredentialPair{ClientID: "id", ClientSecret: "secret", Source: "flag"},
	}
	if token, err := p.ExchangeAuthCode(context.Background(), "code", ""); err != nil || token.AccessToken != "access" {
		t.Fatalf("ExchangeAuthCode without uid = %#v, %v", token, err)
	}
}

func TestCrossPlatformCoverageOAuthRefreshCredentialFailuresAndFallback(t *testing.T) {
	entries := isolateAppCredentialKeychain(t)
	p := &OAuthProvider{configDir: t.TempDir(), httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"accessToken":"new","refreshToken":"next","expiresIn":7200}`)), Request: req}, nil
	})}}
	entries[secretAccountKey("id")] = "canonical"
	entries[legacyClientSecretAccountKey("id")] = "legacy"
	if _, err := p.refreshWithRefreshToken(context.Background(), &TokenData{ClientID: "id", RefreshToken: "refresh"}); !errors.Is(err, ErrClientSecretConflict) {
		t.Fatalf("refresh conflict = %v", err)
	}
	delete(entries, secretAccountKey("id"))
	delete(entries, legacyClientSecretAccountKey("id"))
	writeCredentialConfig(t, p.configDir, "id", PlainSecret("fallback-secret"))
	if token, err := p.refreshWithRefreshToken(context.Background(), &TokenData{ClientID: "id", RefreshToken: "refresh"}); err != nil || token.AccessToken != "new" {
		t.Fatalf("refresh app-config fallback = %#v, %v", token, err)
	}
}
