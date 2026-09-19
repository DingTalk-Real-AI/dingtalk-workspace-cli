// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageExternalExchangeReviewConfigDirectoryAndSource(t *testing.T) {
	defaultDir := externalExchangeTestConfig(t)
	writeApp := func(dir, id, secret string) {
		t.Helper()
		if err := os.WriteFile(GetAppConfigPath(dir), []byte(fmt.Sprintf(`{"clientId":%q,"clientSecret":%q}`, id, secret)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeApp(defaultDir, "wrong-app", "wrong-secret")
	// Poison the existing global cache, then prove external resolution ignores it.
	testseam.Swap(t, &cachedResolvedID, "wrong-app")
	testseam.Swap(t, &cachedResolvedSecret, "wrong-secret")
	testseam.Swap(t, &cachedResolvedValid, true)
	target := t.TempDir()
	writeApp(target, "target-app", "target-secret")
	id, secret, source, err := resolveExternalExchangeClient(context.Background(), target, "", "")
	if err != nil || id != "target-app" || secret != "target-secret" || source != "app" {
		t.Fatalf("target app resolution failed: %v", err)
	}
	second := t.TempDir()
	writeApp(second, "second-app", "second-secret")
	id, secret, source, err = resolveExternalExchangeClient(context.Background(), second, "", "")
	if err != nil || id != "second-app" || secret != "second-secret" || source != "app" {
		t.Fatalf("cross-directory cache contamination: %v", err)
	}
	id, secret, source, err = resolveExternalExchangeClient(context.Background(), target, "other-app", "")
	if err != nil || id != "other-app" || secret != "" || source != "mcp" {
		t.Fatal("reused another application's secret")
	}
	// Malformed target config must not fall back and consume a code with global credentials.
	os.WriteFile(GetAppConfigPath(second), []byte("invalid"), 0600)
	if _, _, _, err = resolveExternalExchangeClient(context.Background(), second, "", ""); err == nil {
		t.Fatal("invalid config accepted")
	}
}

func TestCrossPlatformCoverageExternalExchangeReviewDirectProvenanceSurvivesPersistence(t *testing.T) {
	for _, source := range []string{"app", "env", "default", "flag"} {
		t.Run(source, func(t *testing.T) {
			externalExchangeTestConfig(t)
			dir := t.TempDir() // deliberately differs from DWS_CONFIG_DIR
			request := ExternalExchangeRequest{AuthCode: "fixture-code", ResolveIdentity: func(_ context.Context, _, corp string) (ManagedIdentity, error) {
				return ManagedIdentity{CorpID: corp, UserID: "user"}, nil
			}}
			switch source {
			case "app":
				os.WriteFile(GetAppConfigPath(dir), []byte(`{"clientId":"app-id","clientSecret":"app-secret"}`), 0600)
			case "env":
				t.Setenv("DWS_CLIENT_ID", "app-id")
				t.Setenv("DWS_CLIENT_SECRET", "app-secret")
			case "default":
				testseam.Swap(t, &defaultAuthClientID, "app-id")
				testseam.Swap(t, &defaultAuthSecret, "app-secret")
			case "flag":
				request.ClientID = "app-id"
				request.ClientSecret = "app-secret"
			}
			testseam.Swap(t, &oauthHTTPClient, &http.Client{Transport: postJSONRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != UserAccessTokenURL {
					t.Fatal("incorrect direct endpoint")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"accessToken":"test-token","refreshToken":"test-refresh","expiresIn":7200,"corpId":"corp"}`))}, nil
			})})
			_, err := ExchangeExternalAuthCode(context.Background(), dir, request)
			if err != nil {
				t.Fatal(err)
			}
			data, err := LoadTokenDataForProfile(dir, "corp:user")
			if err != nil || data.Source != source || data.ClientID != "app-id" {
				t.Fatalf("persisted provenance wrong: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageExternalExchangeReviewPersistenceRollbackIncludesSecret(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, failure := range []string{"secret-write", "identity-write", "profile-write", "marker-write", "none"} {
			t.Run(fmt.Sprintf("existing=%v/%s", existing, failure), func(t *testing.T) {
				dir := externalExchangeTestConfig(t)
				old := &TokenData{CorpID: "corp", UserID: "user", ClientID: "app", Source: "flag", AccessToken: "old-token", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(time.Hour)}
				if err := SaveTokenData(dir, old); err != nil {
					t.Fatal(err)
				}
				if existing {
					if err := SaveClientSecret("app", "old-secret"); err != nil {
						t.Fatal(err)
					}
				}
				before, err := os.ReadFile(filepath.Join(dir, "profiles.json"))
				if err != nil {
					t.Fatal(err)
				}
				boom := fmt.Errorf("injected persistence failure")
				switch failure {
				case "secret-write":
					testseam.Swap(t, &oauthSaveClientSecret, func(id, secret string) error {
						if err := SaveClientSecret(id, secret); err != nil {
							return err
						}
						return boom
					})
				case "identity-write":
					original := tokenSaveKeychainForIdentity
					testseam.Swap(t, &tokenSaveKeychainForIdentity, func(c, u string, d *TokenData) error {
						if d.AccessToken == "new-token" {
							return boom
						}
						return original(c, u, d)
					})
				case "profile-write":
					testseam.Swap(t, &tokenUpsertProfile, func(string, *TokenData, bool) error { return boom })
				case "marker-write":
					original := tokenWriteMarker
					calls := 0
					testseam.Swap(t, &tokenWriteMarker, func(dir string) error {
						calls++
						if calls == 1 {
							return boom
						}
						return original(dir)
					})
				}
				fresh := *old
				fresh.AccessToken = "new-token"
				fresh.FreshAuthorization = true
				err = persistExternalExchangeTokenWithSecret(dir, &fresh, "new-secret")
				if failure == "none" {
					if err != nil || LoadClientSecret("app") != "new-secret" {
						t.Fatalf("commit failed: %v", err)
					}
					loaded, e := LoadTokenDataKeychainForIdentity("corp", "user")
					if e != nil || loaded.AccessToken != "new-token" {
						t.Fatal("new identity missing")
					}
					return
				}
				if err == nil {
					t.Fatal("expected persistence failure")
				}
				want := ""
				if existing {
					want = "old-secret"
				}
				if LoadClientSecret("app") != want {
					t.Fatal("client secret not rolled back")
				}
				loaded, e := LoadTokenDataKeychainForIdentity("corp", "user")
				if e != nil || loaded.AccessToken != "old-token" {
					t.Fatal("old identity not restored")
				}
				after, e := os.ReadFile(filepath.Join(dir, "profiles.json"))
				if e != nil || string(before) != string(after) {
					t.Fatal("profile not restored")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageExternalExchangeReviewSecretSnapshotFailureDoesNotOverwrite(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	original := authKeychainGet
	testseam.Swap(t, &authKeychainGet, func(service, account string) (string, error) {
		if account == clientSecretPrefix+"app" {
			return "", fmt.Errorf("unreadable")
		}
		return original(service, account)
	})
	writes := 0
	testseam.Swap(t, &oauthSaveClientSecret, func(string, string) error { writes++; return nil })
	err := persistExternalExchangeTokenWithSecret(dir, &TokenData{CorpID: "corp", UserID: "user", ClientID: "app", AccessToken: "new-token", FreshAuthorization: true}, "new-secret")
	if err == nil || writes != 0 {
		t.Fatal("snapshot failure allowed credential overwrite")
	}
	if authKeychainExists(keychain.Service, TokenAccountForIdentity("corp", "user")) {
		t.Fatal("identity written despite snapshot failure")
	}
}

func TestCrossPlatformCoverageExternalExchangeReviewPreflightDoesNotConsumeCode(t *testing.T) {
	for _, kind := range []string{"invalid-config", "unreadable-secret", "unsupported-hook"} {
		t.Run(kind, func(t *testing.T) {
			externalExchangeTestConfig(t)
			dir := t.TempDir()
			req := ExternalExchangeRequest{AuthCode: "fixture-code", ResolveIdentity: func(context.Context, string, string) (ManagedIdentity, error) {
				t.Fatal("unexpected identity lookup")
				return ManagedIdentity{}, nil
			}}
			if kind == "invalid-config" {
				os.WriteFile(GetAppConfigPath(dir), []byte("{"), 0600)
			} else {
				req.ClientID = "app"
				req.ClientSecret = "secret"
				if kind == "unreadable-secret" {
					original := authKeychainGet
					testseam.Swap(t, &authKeychainGet, func(service, account string) (string, error) {
						if account == clientSecretPrefix+"app" {
							return "", fmt.Errorf("failed")
						}
						return original(service, account)
					})
				} else {
					h := *edition.Get()
					h.SaveToken = func(string, []byte) error { return nil }
					original := edition.Get()
					edition.Override(&h)
					t.Cleanup(func() { edition.Override(original) })
				}
			}
			testseam.Swap(t, &oauthHTTPClient, &http.Client{Transport: postJSONRoundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("consumed code before preflight")
				return nil, fmt.Errorf("unexpected")
			})})
			if _, err := ExchangeExternalAuthCode(context.Background(), dir, req); err == nil {
				t.Fatal("preflight accepted invalid persistence/configuration")
			}
		})
	}
}
