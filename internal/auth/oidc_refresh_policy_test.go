// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dwserrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageMCPRefreshDoesNotInferOIDC(t *testing.T) {
	for _, binding := range []string{"portal", "secret-mode", "both"} {
		t.Run(binding, func(t *testing.T) {
			dir := externalExchangeTestConfig(t)
			testseam.Swap(t, &runtimeClientID, "ordinary-mcp-app")
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var body map[string]string
				if r.URL.Path != MCPRefreshTokenPath || json.NewDecoder(r.Body).Decode(&body) != nil || body["clientId"] != "ordinary-mcp-app" || body["clientSecret"] != "" {
					t.Error("ordinary MCP refresh used OIDC application bindings")
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"errorCode": legacyMCPRefreshRejectedCode, "errorMsg": "ordinary MCP rejection"})
			}))
			defer server.Close()
			t.Cleanup(PushMCPBaseURLOverride(server.URL))
			trap := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("a Portal field alone selected the OIDC refresh path")
			}))
			defer trap.Close()
			data := &TokenData{Source: "mcp", RefreshToken: "refresh"}
			if binding != "secret-mode" {
				data.MCPBaseURL = trap.URL
			}
			data.MCPClientSecretRequired = binding != "portal"
			_, err := (&OAuthProvider{configDir: dir, httpClient: server.Client()}).refreshWithRefreshToken(t.Context(), data)
			var rejected *MCPTokenExchangeError
			if requests != 1 || !errors.As(err, &rejected) || rejected.Message != "ordinary MCP rejection" {
				t.Fatalf("non-OIDC session entered OIDC refresh handling: requests=%d error=%v", requests, err)
			}
		})
	}
}

func TestCrossPlatformCoverageSecretModeDoesNotGrantOIDCPersistence(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	data := &TokenData{Source: "mcp", ClientID: "app", CorpID: "corp", AccessToken: "access", MCPClientSecretRequired: true, FreshAuthorization: true}
	err := withProfilesLock(dir, func() error {
		return saveTokenDataLockedForSelectorAndSecret(dir, data, "", "secret")
	})
	if err == nil {
		t.Fatal("secret mode alone bypassed the required user identity")
	}
	if secret, err := LoadClientSecretStrict("app"); err != nil || secret != "" {
		t.Fatal("invalid login persisted an application secret")
	}
	data.OIDCProvider = "ant"
	if err := withProfilesLock(dir, func() error {
		return saveTokenDataLockedForSelectorAndSecret(dir, data, "", "secret")
	}); err != nil {
		t.Fatalf("explicit OIDC login lost organization-scoped persistence: %v", err)
	}
	stored, err := LoadTokenDataForProfile(dir, "corp")
	if err != nil || stored.OIDCProvider != "ant" {
		t.Fatalf("saved organization credential lost its OIDC source: %v", err)
	}
}

func TestCrossPlatformCoverageOIDCRefreshDoesNotRecover(t *testing.T) {
	for _, custom := range []bool{false, true} {
		for _, scenario := range []string{"rejected-org", "rejected-legacy", "unknown-code", "unauthorized", "rate-limited", "unavailable"} {
			name := scenario
			if custom {
				name += "/custom"
			}
			t.Run(name, func(t *testing.T) {
				f := newOrgSlotFallbackFixture(t)
				f.provider.configDir = externalExchangeTestConfig(t)
				f.stale.Source, f.stale.ClientID = "mcp", "original-app"
				f.stale.MCPClientSecretRequired = custom
				f.stale.OIDCProvider = "ant"
				if custom {
					if err := SaveClientSecret(f.stale.ClientID, "original-secret"); err != nil {
						t.Fatal(err)
					}
				}
				// Both candidates deliberately belong to another application.
				f.orgMirror.ClientID = "another-app"
				if scenario == "rejected-legacy" {
					f.orgMirror = nil
				}
				legacyLoads, saves, marks, requests := 0, 0, 0, 0
				testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) {
					legacyLoads++
					candidate := *f.stale
					candidate.ClientID, candidate.AccessToken = "another-app", "another-access"
					candidate.ExpiresAt = time.Now().Add(time.Hour)
					return &candidate, nil
				})
				testseam.Swap(t, &oauthSaveTokenLocked, func(string, *TokenData) error { saves++; return nil })
				testseam.Swap(t, &oauthMarkProfile, func(_, _, status string) error {
					if status != ProfileStatusExpired {
						t.Errorf("unexpected profile status: %s", status)
					}
					marks++
					return nil
				})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					var body map[string]string
					if r.URL.Path != MCPRefreshTokenPath || json.NewDecoder(r.Body).Decode(&body) != nil || body["clientId"] != "original-app" || body["refreshToken"] != f.stale.RefreshToken {
						t.Error("refresh changed the selected application's credentials")
					}
					wantSecret := ""
					if custom {
						wantSecret = "original-secret"
					}
					if body["clientSecret"] != wantSecret {
						t.Error("refresh used another application's secret")
					}
					code := legacyMCPRefreshRejectedCode
					switch scenario {
					case "unknown-code":
						code = "private-code"
					case "unauthorized":
						w.WriteHeader(http.StatusUnauthorized)
					case "rate-limited":
						w.WriteHeader(http.StatusTooManyRequests)
					case "unavailable":
						w.WriteHeader(http.StatusServiceUnavailable)
					}
					_ = json.NewEncoder(w).Encode(map[string]string{"errorCode": code, "errorMsg": "private-response"})
				}))
				defer server.Close()
				f.stale.MCPBaseURL = server.URL
				f.provider.httpClient = server.Client()
				testseam.Swap(t, &oauthRefreshToken, func(p *OAuthProvider, ctx context.Context, data *TokenData) (*TokenData, error) {
					return p.refreshWithRefreshToken(ctx, data)
				})
				got, err := f.provider.GetTokenSnapshot(t.Context())
				if err == nil || got != nil || requests != 1 || f.orgLoads != 0 || legacyLoads != 0 || saves != 0 {
					t.Fatalf("OIDC attempted credential recovery: requests=%d org=%d legacy=%d saves=%d error=%v", requests, f.orgLoads, legacyLoads, saves, err)
				}
				var displayed strings.Builder
				if printErr := dwserrors.PrintHuman(&displayed, err); printErr != nil {
					t.Fatal(printErr)
				}
				transient := scenario == "rate-limited" || scenario == "unavailable"
				if transient {
					if marks != 0 || ClassifyRefreshFailure(err) != RefreshFailureTransient || strings.Contains(displayed.String(), "provider-login") {
						t.Fatalf("temporary failure expired credentials or forced login: %v", err)
					}
				} else if marks != 1 || !strings.Contains(displayed.String(), "provider-login") || strings.Contains(displayed.String(), "dws auth login") {
					t.Fatalf("wrong reauthentication guidance: marks=%d error=%v", marks, err)
				}
				if !transient && custom && !strings.Contains(displayed.String(), "original --client-id/--client-secret") {
					t.Fatal("reauthentication guidance lost the custom application requirement")
				}
				if strings.Contains(displayed.String(), "private-") || strings.Contains(displayed.String(), "original-secret") {
					t.Fatal("refresh error exposed response or credentials")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageOIDCRefreshRejectsIncompleteBinding(t *testing.T) {
	for _, missing := range []string{"client", "portal", "source", "invalid-portal", "invalid-provider"} {
		t.Run(missing, func(t *testing.T) {
			dir := externalExchangeTestConfig(t)
			testseam.Swap(t, &runtimeClientID, "unrelated-global-app")
			data := &TokenData{ClientID: "original-app", Source: "mcp", OIDCProvider: "ant", MCPBaseURL: "https://portal.example", RefreshToken: "refresh"}
			switch missing {
			case "client":
				data.ClientID = ""
			case "portal":
				data.MCPBaseURL = ""
			case "source":
				data.Source = ""
			case "invalid-portal":
				data.MCPBaseURL = "http://portal.example"
			case "invalid-provider":
				data.OIDCProvider = "private-invalid/provider"
			}
			requests := 0
			client := &http.Client{Transport: oidcPortalTransportFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})}
			_, err := (&OAuthProvider{configDir: dir, httpClient: client}).refreshWithRefreshToken(t.Context(), data)
			if err == nil || requests != 0 || strings.Contains(err.Error(), "private-") {
				t.Fatalf("invalid OIDC binding used an unrelated application or endpoint: requests=%d error=%v", requests, err)
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCRefreshCancellationAndExpiry(t *testing.T) {
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded, io.ErrUnexpectedEOF, nil} {
		f := newOrgSlotFallbackFixture(t)
		f.stale.Source, f.stale.MCPBaseURL = "mcp", "https://portal.example"
		f.stale.OIDCProvider = "ant"
		f.refreshErr = failure
		marks := 0
		testseam.Swap(t, &oauthMarkProfile, func(_, _, _ string) error { marks++; return nil })
		if failure == nil {
			f.stale.RefreshExpAt = time.Now().Add(-time.Hour)
		}
		_, err := f.provider.GetTokenSnapshot(t.Context())
		var displayed strings.Builder
		if printErr := dwserrors.PrintHuman(&displayed, err); printErr != nil {
			t.Fatal(printErr)
		}
		if f.orgLoads != 0 {
			t.Fatal("cancellation or expiry triggered another credential")
		}
		if failure != nil {
			if !errors.Is(err, failure) || marks != 0 || strings.Contains(err.Error(), "provider-login") {
				t.Fatalf("temporary failure was replaced by login failure: %v", err)
			}
		} else if err == nil || marks != 1 || !strings.Contains(displayed.String(), "provider-login ant") || len(f.refreshCalls) != 0 {
			t.Fatalf("expired OIDC credential did not request provider login: %v", err)
		}
	}
}

func TestCrossPlatformCoverageOIDCRefreshPreCanceledBeforeBindingAndIO(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	requests, reads, saves := 0, 0, 0
	testseam.Swap(t, &authKeychainGet, func(string, string) (string, error) {
		reads++
		return "", errors.New("unexpected credential read")
	})
	testseam.Swap(t, &oauthSaveTokenLocked, func(string, *TokenData) error { saves++; return nil })
	p := &OAuthProvider{configDir: dir, httpClient: &http.Client{Transport: oidcPortalTransportFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected refresh request")
	})}}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	cancel()
	// Exercise the real refresh implementation, not the oauthRefreshToken seam.
	// Cancellation must win even when the persisted OIDC binding is incomplete.
	for _, complete := range []bool{false, true} {
		data := &TokenData{OIDCProvider: "ant", RefreshToken: "refresh", MCPClientSecretRequired: true}
		if complete {
			data.Source, data.ClientID, data.MCPBaseURL = "mcp", "original-app", "https://portal.example"
		}
		refreshed, err := p.refreshWithRefreshToken(ctx, data)
		if refreshed != nil || !errors.Is(err, context.Canceled) || requests != 0 || reads != 0 || saves != 0 {
			t.Fatalf("canceled OIDC refresh performed IO or lost cancellation: complete=%t requests=%d reads=%d saves=%d error=%v", complete, requests, reads, saves, err)
		}
	}
}

func TestCrossPlatformCoverageOIDCSecretPersistencePreCanceled(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	previousProfile := RuntimeProfile()
	SetRuntimeProfile("")
	t.Cleanup(func() { SetRuntimeProfile(previousProfile) })
	previous := &TokenData{AccessToken: "previous-access", CorpID: "corp", UserID: "user", ClientID: "app", Source: "mcp", OIDCProvider: "ant", ExpiresAt: time.Now().Add(time.Hour)}
	if err := SaveTokenData(dir, previous); err != nil {
		t.Fatal(err)
	}
	if err := SaveClientSecret("app", "previous-secret"); err != nil {
		t.Fatal(err)
	}
	writes := 0
	testseam.Swap(t, &authKeychainSet, func(string, string, string) error {
		writes++
		return errors.New("unexpected canceled credential write")
	})
	testseam.Swap(t, &oauthSaveToken, func(string, *TokenData) error { writes++; return nil })
	testseam.Swap(t, &oauthSaveClientSecret, func(string, string) error { writes++; return nil })
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	cancel()
	fresh := &TokenData{AccessToken: "new-access", CorpID: "corp", UserID: "user", ClientID: "app", Source: "mcp", OIDCProvider: "ant", MCPBaseURL: "https://portal.example", MCPClientSecretRequired: true, ExpiresAt: time.Now().Add(time.Hour)}
	err := (&OAuthProvider{configDir: dir}).persistLoginTokenWithSecret(ctx, fresh, "new-private-secret")
	if !errors.Is(err, context.Canceled) || writes != 0 || !fresh.FreshAuthorization {
		t.Fatalf("canceled secret persistence wrote credentials or stopped before preparation: writes=%d error=%v", writes, err)
	}
	saved, err := LoadTokenDataForProfile(dir, "corp:user")
	if err != nil || saved.AccessToken != "previous-access" {
		t.Fatalf("canceled secret persistence replaced the previous token: %v", err)
	}
	secret, err := LoadClientSecretStrict("app")
	if err != nil || secret != "previous-secret" {
		t.Fatalf("canceled secret persistence replaced the previous application secret: %v", err)
	}
	cfg, err := LoadProfiles(dir)
	if err != nil || len(cfg.Profiles) != 1 || cfg.CurrentProfile != "corp:user" {
		t.Fatalf("canceled secret persistence changed the profile registry: %v", err)
	}
}
