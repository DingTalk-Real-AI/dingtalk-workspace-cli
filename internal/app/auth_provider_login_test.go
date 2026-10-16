// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License")

package app

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

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func providerLoginRoot() (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	root := &cobra.Command{Use: "dws", TraverseChildren: true, SilenceErrors: true, SilenceUsage: true}
	root.SetContext(context.Background())
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().String("profile", "", "")
	root.PersistentFlags().String("client-id", "", "")
	root.PersistentFlags().String("client-secret", "", "")
	auth := &cobra.Command{Use: "auth", TraverseChildren: true}
	auth.AddCommand(newAuthProviderLoginCommand(nil))
	root.AddCommand(auth)
	out, stderr := new(bytes.Buffer), new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(stderr)
	return root, out, stderr
}

func TestCrossPlatformCoverageProviderLoginCLIValidationAndDryRun(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &authProviderLogin, func(context.Context, string, string, string, edition.ToolCaller, *authpkg.OIDCExchangeCredentials) (*authpkg.TokenData, error) {
		t.Error("validation/dry-run reached authentication")
		return nil, nil
	})
	for _, args := range [][]string{
		{"provider-login"},
		{"provider-login", ""},
		{"provider-login", "Bad/name"},
		{"provider-login", strings.Repeat("a", 65)},
		{"provider-login", "ant", "--no-browser"},
		{"provider-login", "ant", "--device"},
		{"provider-login", "ant", "--token=private"},
		{"provider-login", "ant", "--intl"},
		{"provider-login", "ant", "--client-id=app"},
		{"provider-login", "ant", "--client-secret=private"},
		{"provider-login", "ant", "--client-id=app", "--client-secret="},
		{"provider-login", "ant", "--client-id= ", "--client-secret=private"},
		{"provider-login", "ant", "--client-id=", "--client-secret="},
		{"provider-login", "ant", "--login-timeout=-1"},
	} {
		root, out, stderr := providerLoginRoot()
		root.SetArgs(append([]string{"auth"}, args...))
		err := corecmd.ExecuteForTest(root)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation {
			t.Fatalf("args %v: want validation, got %v", args, err)
		}
		if strings.Contains(out.String()+stderr.String(), "private") {
			t.Fatal("credential leaked")
		}
	}
	for _, provider := range []string{"ant", "enterprise_acme"} {
		root, out, _ := providerLoginRoot()
		root.SetArgs([]string{"auth", "provider-login", provider, "--dry-run"})
		if err := corecmd.ExecuteForTest(root); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "planned") || !strings.Contains(out.String(), provider) {
			t.Fatal("missing provider dry-run result")
		}
	}
}

func TestCrossPlatformCoverageProviderLoginPublicRootRejectsIncompleteCredentials(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &authProviderLogin, func(context.Context, string, string, string, edition.ToolCaller, *authpkg.OIDCExchangeCredentials) (*authpkg.TokenData, error) {
		t.Fatal("conflicting credentials started provider login")
		return nil, nil
	})
	for _, args := range [][]string{
		{"--client-id", "app", "auth", "provider-login", "ant"},
		{"auth", "provider-login", "ant", "--client-id", "app"},
		{"--client-secret", "private", "auth", "provider-login", "ant"},
		{"auth", "provider-login", "ant", "--client-secret", "private"},
	} {
		root := NewRootCommand(t.Context())
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs(args)
		err := root.Execute()
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || strings.Contains(err.Error(), "private") {
			t.Fatalf("credential conflict did not preserve validation: %v", err)
		}
	}
}

func TestCrossPlatformCoverageProviderLoginFailedInvocationClearsCredentials(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	for _, args := range [][]string{
		{"--client-id", "rejected-app", "--client-secret", "rejected-secret", "auth", "provider-login", "ant", "--device"},
		{"--client-secret", "rejected-secret", "auth", "provider-login", "other"},
		{"--client-secret", "rejected-secret", "auth", "provider-login"},
	} {
		root := NewRootCommand(t.Context())
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatal("invalid provider-login invocation succeeded")
		}
		for _, name := range []string{"client-id", "client-secret"} {
			flag := root.PersistentFlags().Lookup(name)
			if flag.Changed || flag.Value.String() != "" {
				t.Errorf("failed invocation retained --%s", name)
			}
		}
		root.SetArgs([]string{"version"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if authpkg.ClientID() == "rejected-app" || authpkg.ClientSecret() == "rejected-secret" {
			t.Fatal("next invocation consumed rejected credentials")
		}
	}
}

func TestCrossPlatformCoverageProviderLoginCLIRealHTTPPersistence(t *testing.T) {
	for _, scenario := range []string{"ant", "ant_custom_client", "enterprise_acme"} {
		t.Run(scenario, func(t *testing.T) {
			provider := scenario
			custom := scenario == "ant_custom_client"
			if custom {
				provider = "ant"
			}
			dir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", dir)
			t.Setenv(keychain.StorageDirEnv, filepath.Join(dir, "keys"))
			t.Setenv(keychain.DisableKeychainEnv, "1")
			t.Setenv(keychain.TestNamespaceEnv, dir)
			defer keychain.RemoveAuthTokenEntries(keychain.Service)
			if err := authpkg.SaveTokenData(dir, &authpkg.TokenData{AccessToken: "existing", CorpID: "existing-corp", UserID: "existing-user", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			const state = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			var requests atomic.Int32
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if provider != "ant" && r.URL.Path != "/cli/auth/oidc/transactions" {
					t.Error("another provider entered the Ant SDK flow")
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				switch r.URL.Path {
				case "/cli/auth/oidc/transactions":
					if len(body) != 1 || body["provider"] != provider {
						t.Error("transaction must contain only provider")
					}
					w.WriteHeader(201)
					json.NewEncoder(w).Encode(map[string]any{"provider": provider, "expires_in": 600, "authorization_request": map[string]any{"authorization_endpoint": server.URL + "/cli/auth/oidc/mock/sdk/authorize", "parameters": map[string]string{"client_id": "oidc-app", "response_type": "code", "scope": "openid", "redirect_uri": "ant-sdk://oidc/callback", "state": state, "nonce": state, "code_challenge": state, "code_challenge_method": "S256"}}})
				case "/cli/auth/oidc/mock/sdk/login":
					if len(body) != 0 {
						t.Error("SDK login body must be empty")
					}
					json.NewEncoder(w).Encode(map[string]any{"sdk_session_token": "secret-sdk", "expires_in": 60})
				case "/cli/auth/oidc/mock/sdk/authorize":
					if len(body) != 2 || r.Header.Get("Authorization") != "Bearer secret-sdk" || body["authorization_endpoint"] != server.URL+"/cli/auth/oidc/mock/sdk/authorize" {
						t.Error("SDK authorization request lost its metadata or session")
					}
					json.NewEncoder(w).Encode(map[string]string{"code": "one-code", "state": state})
				case "/cli/auth/oidc/exchange":
					if (custom && (len(body) != 4 || body["client_id"] != "dws-app" || body["client_secret"] != "secret-custom")) || (!custom && len(body) != 2) {
						t.Error("exchange did not preserve optional credential pair")
					}
					if body["code"] != "one-code" || body["state"] != state {
						t.Error("bad exchange")
					}
					json.NewEncoder(w).Encode(map[string]any{"accessToken": "secret-access", "refreshToken": "secret-refresh", "expiresIn": 3600, "corpId": "ant-corp", "corpName": "Ant Organization"})
				default:
					t.Error("unexpected endpoint")
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			root, out, stderr := providerLoginRoot()
			args := []string{"auth", "provider-login", provider, "--mcp-url", server.URL}
			if custom {
				args = append(args, "--client-id", "dws-app", "--client-secret", "secret-custom")
			}
			root.SetArgs(args)
			err := corecmd.ExecuteForTest(root)
			if provider != "ant" {
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Reason != "OIDC_SDK_UNAVAILABLE" || requests.Load() != 1 {
					t.Fatalf("unimplemented provider must stop after backend check: requests=%d error=%v", requests.Load(), err)
				}
				profiles, loadErr := authpkg.LoadProfiles(dir)
				if loadErr != nil || len(profiles.Profiles) != 1 || profiles.CurrentProfile != "existing-corp:existing-user" {
					t.Fatal("unimplemented provider changed saved profiles")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if stderr.Len() != 0 {
				t.Fatal("Provider login unexpectedly printed browser output")
			}
			if strings.Contains(out.String(), "secret-") || !strings.Contains(out.String(), `"success": true`) || !strings.Contains(out.String(), `"corp_id": "ant-corp"`) {
				t.Fatalf("unsafe/missing save output: %s", out.String())
			}
			saved, err := authpkg.LoadTokenDataForProfile(dir, "ant-corp")
			if err != nil || saved.RefreshToken != "secret-refresh" || saved.Source != "mcp" || saved.CorpName != "Ant Organization" || saved.UserID != "" {
				t.Fatalf("session not saved: %v", err)
			}
			if custom {
				if _, err := os.Stat(authpkg.GetAppConfigPath(dir)); !os.IsNotExist(err) {
					t.Fatal("provider exchange credentials were persisted as the ordinary application")
				}
			}
			profiles, err := authpkg.LoadProfiles(dir)
			if err != nil || profiles.CurrentProfile != "ant-corp" || len(profiles.Profiles) != 2 {
				t.Fatalf("normal login did not select the saved profile: %v", err)
			}
			base, err := os.ReadFile(filepath.Join(dir, "mcp_url"))
			if err != nil || string(base) != server.URL {
				t.Fatalf("mock MCP URL not persisted: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageProviderLoginProviderAvailabilityComesFromBackend(t *testing.T) {
	for _, provider := range []string{"ant", "enterprise_acme"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", dir)
			t.Setenv(keychain.StorageDirEnv, filepath.Join(dir, "keys"))
			t.Setenv(keychain.DisableKeychainEnv, "1")
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var body map[string]string
				if r.URL.Path != "/cli/auth/oidc/transactions" || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["provider"] != provider {
					t.Error("provider must be sent unchanged to transactions, without starting SDK")
				}
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"error":{"code":"unsupported_provider","action":"stop"}}`)
			}))
			defer server.Close()
			root, _, _ := providerLoginRoot()
			root.SetArgs([]string{"auth", "provider-login", provider, "--mcp-url", server.URL})
			err := corecmd.ExecuteForTest(root)
			var typed *apperrors.Error
			if requests.Load() != 1 || !errors.As(err, &typed) || typed.Category != apperrors.CategoryAuth || !strings.Contains(err.Error(), provider) {
				t.Fatalf("provider availability was not decided by backend: requests=%d error=%v", requests.Load(), err)
			}
		})
	}
}

func TestCrossPlatformCoverageProviderLoginPublicRootCredentialPair(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	stopped := errors.New("test provider stopped")
	calls := 0
	testseam.Swap(t, &authProviderLogin, func(_ context.Context, _, provider, _ string, _ edition.ToolCaller, credentials *authpkg.OIDCExchangeCredentials) (*authpkg.TokenData, error) {
		calls++
		if provider != "ant" || credentials == nil || credentials.ClientID != "test-dws-app" || credentials.ClientSecret != "test-dws-secret" {
			t.Fatal("credential pair did not reach provider")
		}
		if authpkg.ClientID() == "test-dws-app" || authpkg.ClientSecret() == "test-dws-secret" {
			t.Fatal("provider credentials changed global application")
		}
		return nil, stopped
	})
	for _, args := range [][]string{
		{"auth", "provider-login", "ant", "--client-id", "test-dws-app", "--client-secret", "test-dws-secret"},
		{"--client-id", "test-dws-app", "--client-secret", "test-dws-secret", "auth", "provider-login", "ant"},
		{"--client-id", "test-dws-app", "auth", "provider-login", "ant", "--client-secret", "test-dws-secret"},
	} {
		for _, dryRun := range []bool{false, true} {
			root := NewRootCommand(t.Context())
			output := new(bytes.Buffer)
			root.SetOut(output)
			root.SetErr(output)
			argv := append([]string{}, args...)
			if dryRun {
				argv = append(argv, "--dry-run")
			}
			root.SetArgs(argv)
			before := calls
			err := root.Execute()
			if dryRun {
				if err != nil || calls != before {
					t.Fatalf("dry-run reached provider or failed: %v", err)
				}
			} else if !errors.Is(err, stopped) || calls != before+1 {
				t.Fatalf("complete pair rejected: %v", err)
			}
			if strings.Contains(output.String(), "test-dws-secret") {
				t.Fatal("secret leaked to output")
			}
			for _, name := range []string{"client-id", "client-secret"} {
				flag := root.PersistentFlags().Lookup(name)
				if flag.Changed || flag.Value.String() != "" {
					t.Fatalf("provider invocation retained %s", name)
				}
			}
		}
	}
}

func TestCrossPlatformCoverageProviderLoginPreRunFailureClearsCredentials(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	// PersistentPreRunE 校验失败发生在凭证消费之后、RunE 之前；RunE 内的 defer
	// 与输出边界清理都不会执行，只能依赖 preRun 失败分支的清理。
	t.Setenv(envDWSAgentHost, "bad\r\nhost")
	root := NewRootCommand(t.Context())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"auth", "provider-login", "ant", "--client-id", "rejected-app", "--client-secret", "rejected-secret"})
	if err := root.Execute(); err == nil {
		t.Fatal("preRun validation failure did not stop execution")
	}
	for _, name := range []string{"client-id", "client-secret"} {
		flag := root.PersistentFlags().Lookup(name)
		if flag.Changed || flag.Value.String() != "" {
			t.Errorf("preRun failure retained --%s", name)
		}
	}
	if authpkg.ClientID() == "rejected-app" || authpkg.ClientSecret() == "rejected-secret" {
		t.Fatal("preRun failure leaked provider credentials into the global application")
	}
	// 恢复合法环境后再验证后续执行，否则 version 的 preRun 会再次失败。
	t.Setenv(envDWSAgentHost, "")
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if authpkg.ClientID() == "rejected-app" || authpkg.ClientSecret() == "rejected-secret" {
		t.Fatal("next invocation consumed rejected credentials")
	}
}
