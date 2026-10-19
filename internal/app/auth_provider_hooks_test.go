// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func withOIDCProviders(t *testing.T, providers func() []edition.OIDCProvider) {
	t.Helper()
	previous := edition.Get()
	hooks := *previous
	hooks.OIDCProviders = providers
	edition.Override(&hooks)
	t.Cleanup(func() { edition.Override(previous) })
}

func TestCrossPlatformCoverageProviderLoginRegisteredHooks(t *testing.T) {
	for _, provider := range []string{"ant", "enterprise_acme"} {
		for _, scenario := range []string{"success", "sdk_error", "sdk_typed_error", "cancel", "timeout", "state_mismatch", "empty_code", "backend_reject", "unregistered", "no_hook"} {
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				dir := t.TempDir()
				t.Setenv("DWS_CONFIG_DIR", dir)
				t.Setenv(keychain.StorageDirEnv, filepath.Join(dir, "keys"))
				t.Setenv(keychain.DisableKeychainEnv, "1")
				t.Setenv(keychain.TestNamespaceEnv, dir)
				defer keychain.RemoveAuthTokenEntries(keychain.Service)
				const state = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				const sdkSecret = "private-sdk-error-material"
				var transactionCalls, exchangeCalls, sdkCalls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost {
						t.Error("OIDC requests must use POST")
					}
					var body map[string]string
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					switch r.URL.Path {
					case "/cli/auth/oidc/transactions":
						transactionCalls.Add(1)
						if len(body) != 1 || body["provider"] != provider || sdkCalls.Load() != 0 {
							t.Error("provider transaction must precede SDK authorization")
						}
						if scenario == "backend_reject" {
							w.WriteHeader(http.StatusBadRequest)
							io.WriteString(w, `{"error":{"code":"unsupported_provider","action":"stop"}}`)
							return
						}
						w.WriteHeader(http.StatusCreated)
						json.NewEncoder(w).Encode(map[string]any{
							"provider": provider, "expires_in": 600,
							"authorization_request": map[string]any{
								"authorization_endpoint": "https://sdk.example.invalid/authorize",
								"parameters":             map[string]string{"client_id": "oidc-app", "response_type": "code", "scope": "openid", "redirect_uri": "ant-sdk://oidc/callback", "state": state, "nonce": state, "code_challenge": state, "code_challenge_method": "S256"},
							},
						})
					case "/cli/auth/oidc/exchange":
						exchangeCalls.Add(1)
						if sdkCalls.Load() != 1 || len(body) != 5 || body["provider"] != provider || body["code"] != "sdk-code" || body["state"] != state || body["client_id"] != "dws-app" || body["client_secret"] != "private-dws-secret" {
							t.Error("exchange lost the selected provider, registered SDK result or DWS credentials")
						}
						json.NewEncoder(w).Encode(map[string]any{"accessToken": "private-access", "refreshToken": "private-refresh", "expiresIn": 3600, "corpId": "hook-corp", "userId": "hook-user", "corpName": "Hook Corp", "userName": "Hook User"})
					default:
						t.Error("registered provider entered an unexpected endpoint or Mock SDK: " + r.URL.Path)
						w.WriteHeader(http.StatusNotFound)
					}
				}))
				defer server.Close()
				withOIDCProviders(t, func() []edition.OIDCProvider {
					if scenario == "unregistered" {
						return nil
					}
					return []edition.OIDCProvider{{Name: provider, Authorize: func(ctx context.Context, request edition.OIDCAuthorizationRequest) (edition.OIDCAuthorizationResult, error) {
						sdkCalls.Add(1)
						if transactionCalls.Load() != 1 || request.AuthorizationEndpoint != "https://sdk.example.invalid/authorize" || len(request.Parameters) != 8 || request.Parameters["state"] != state || request.Parameters["client_id"] != "oidc-app" {
							t.Error("hook did not receive the validated server-owned request")
						}
						if _, ok := ctx.Deadline(); !ok {
							t.Error("hook lost the transaction deadline")
						}
						result := edition.OIDCAuthorizationResult{Code: "sdk-code", State: state}
						switch scenario {
						case "sdk_error":
							return result, errors.New(sdkSecret)
						case "sdk_typed_error":
							return result, fmt.Errorf("%s: %w", sdkSecret, apperrors.NewAuth("SDK failure"))
						case "cancel":
							return result, context.Canceled
						case "timeout":
							return result, context.DeadlineExceeded
						case "state_mismatch":
							request.Parameters["state"] = "tampered-state"
							result.State = "tampered-state"
						case "empty_code":
							result.Code = ""
						}
						return result, nil
					}}}
				})
				if scenario == "no_hook" {
					withOIDCProviders(t, nil)
				}
				var logs bytes.Buffer
				previousLogger := slog.Default()
				slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
				t.Cleanup(func() { slog.SetDefault(previousLogger) })
				if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte(server.URL), 0600); err != nil {
					t.Fatal(err)
				}
				root, out, stderr := providerLoginRoot()
				if scenario == "success" {
					root = NewRootCommand(t.Context())
					root.SetOut(out)
					root.SetErr(stderr)
				}
				root.SetArgs([]string{"auth", "provider-login", provider, "--client-id", "dws-app", "--client-secret", "private-dws-secret", "--format", "json"})
				var err error
				if scenario == "success" {
					err = root.Execute()
				} else {
					err = corecmd.ExecuteForTest(root)
				}
				if transactionCalls.Load() != 1 {
					t.Fatalf("transaction calls = %d", transactionCalls.Load())
				}
				if strings.Contains(logs.String()+out.String()+stderr.String(), sdkSecret) {
					t.Fatal("hook error leaked SDK material into logs/output")
				}
				if scenario == "success" {
					if err != nil || sdkCalls.Load() != 1 || exchangeCalls.Load() != 1 {
						t.Fatalf("registered provider failed: sdk=%d exchange=%d error=%v", sdkCalls.Load(), exchangeCalls.Load(), err)
					}
					saved, err := authpkg.LoadTokenDataForProfile(dir, "hook-corp:hook-user")
					if err != nil || saved.AccessToken != "private-access" || saved.RefreshToken != "private-refresh" {
						t.Fatalf("registered provider did not persist credentials: %v", err)
					}
					return
				}
				if err == nil || exchangeCalls.Load() != 0 || strings.Contains(err.Error(), sdkSecret) {
					t.Fatalf("failed authorization exchanged a code or exposed SDK error: %v", err)
				}
				var wireError bytes.Buffer
				if jsonErr := apperrors.PrintJSON(&wireError, err); jsonErr != nil {
					t.Fatal(jsonErr)
				}
				if strings.Contains(wireError.String(), sdkSecret) {
					t.Fatal("hook error leaked SDK material into JSON cause")
				}
				wantSDKCalls := int32(1)
				if scenario == "backend_reject" || scenario == "unregistered" || scenario == "no_hook" {
					wantSDKCalls = 0
				}
				if sdkCalls.Load() != wantSDKCalls {
					t.Fatalf("SDK calls = %d, want %d", sdkCalls.Load(), wantSDKCalls)
				}
				if scenario == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatal("hook cancellation lost its identity")
				}
				if scenario == "timeout" && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("hook timeout lost its identity")
				}
				if scenario == "unregistered" || scenario == "no_hook" {
					var typed *apperrors.Error
					if !errors.As(err, &typed) || typed.Reason != "OIDC_SDK_UNAVAILABLE" {
						t.Fatalf("missing hook did not report unavailable: %v", err)
					}
				}
				if saved, _ := authpkg.LoadTokenDataForProfile(dir, "hook-corp:hook-user"); saved != nil {
					t.Fatal("failed authorization persisted credentials")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageProviderLoginInvalidRegistration(t *testing.T) {
	authorize := func(context.Context, edition.OIDCAuthorizationRequest) (edition.OIDCAuthorizationResult, error) {
		t.Fatal("invalid registration invoked an SDK")
		return edition.OIDCAuthorizationResult{}, nil
	}
	for _, providers := range [][]edition.OIDCProvider{
		{{Name: "", Authorize: authorize}},
		{{Name: "Bad/name", Authorize: authorize}},
		{{Name: strings.Repeat("a", 65), Authorize: authorize}},
		{{Name: "ant"}},
		{{Name: "ant", Authorize: authorize}, {Name: "ant", Authorize: authorize}},
	} {
		t.Run("invalid", func(t *testing.T) {
			withOIDCProviders(t, func() []edition.OIDCProvider { return providers })
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("invalid registration reached the network")
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			_, err := loginWithProviderLogin(t.Context(), t.TempDir(), "ant", server.URL, nil, nil)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Category != apperrors.CategoryInternal {
				t.Fatalf("invalid registration must be a configuration error: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageProviderLoginHooksStayLazy(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	withOIDCProviders(t, func() []edition.OIDCProvider {
		t.Fatal("help/dry-run enumerated providers or initialized an SDK")
		return nil
	})
	for _, args := range [][]string{{"--help"}, {"ant", "--dry-run"}} {
		root, _, _ := providerLoginRoot()
		root.SetArgs(append([]string{"auth", "provider-login"}, args...))
		if err := corecmd.ExecuteForTest(root); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCrossPlatformCoverageProviderLoginHookSelectionAndIsolation(t *testing.T) {
	withOIDCProviders(t, func() []edition.OIDCProvider {
		return []edition.OIDCProvider{
			{Name: "other", Authorize: func(context.Context, edition.OIDCAuthorizationRequest) (edition.OIDCAuthorizationResult, error) {
				t.Fatal("selected another provider's authorizer")
				return edition.OIDCAuthorizationResult{}, nil
			}},
			{Name: "ant", Authorize: func(_ context.Context, request edition.OIDCAuthorizationRequest) (edition.OIDCAuthorizationResult, error) {
				state := request.Parameters["state"]
				request.Parameters["state"] = "SDK-local-change"
				return edition.OIDCAuthorizationResult{Code: "sdk-code", State: state}, nil
			}},
		}
	})
	authorizer, err := resolveOIDCAuthorizer("ant")
	if err != nil || authorizer == nil {
		t.Fatalf("cannot resolve registered ant provider: %v", err)
	}
	request := authpkg.OIDCAuthorizationRequest{Parameters: map[string]string{"state": "server-state"}}
	result, err := authorizer.Authorize(t.Context(), request)
	if err != nil || result.Code != "sdk-code" || result.State != "server-state" || request.Parameters["state"] != "server-state" {
		t.Fatalf("SDK modified the core request or lost its result: %v", err)
	}
	// Hook removal must affect the next invocation; registrations are not cached.
	withOIDCProviders(t, nil)
	if next, err := resolveOIDCAuthorizer("ant"); err != nil || next != nil {
		t.Fatalf("a previous invocation's authorizer remained registered: %v", err)
	}
}
