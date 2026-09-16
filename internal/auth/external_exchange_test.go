// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func externalExchangeTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")
	t.Setenv(keychain.StorageDirEnv, filepath.Join(dir, "keys"))
	t.Setenv(keychain.DisableKeychainEnv, "1")
	// Windows uses DPAPI/Registry rather than StorageDirEnv. Each case must
	// isolate that backend too, including canonical and legacy secret slots.
	t.Setenv(keychain.TestNamespaceEnv, dir)
	t.Cleanup(func() {
		if err := keychain.RemoveAuthTokenEntries(keychain.Service); err != nil {
			t.Errorf("clean exchange test tokens: %v", err)
		}
		if err := keychain.RemoveAccountEntriesWithPrefixes(keychain.Service, secretKeyPrefix, clientSecretPrefix, appTokenPrefix); err != nil {
			t.Errorf("clean exchange test application credentials: %v", err)
		}
	})
	testseam.Swap(t, &runtimeClientID, "")
	testseam.Swap(t, &runtimeClientSecret, "")
	testseam.Swap(t, &clientIDFromMCP, false)
	testseam.Swap(t, &defaultAuthClientID, "<unset>")
	testseam.Swap(t, &defaultAuthSecret, "<unset>")
	return dir
}

func externalExchangeTestServer(t *testing.T, dir string, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte(server.URL), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestExternalExchangeFixturesIsolatePlatformCredentials(t *testing.T) {
	externalExchangeTestConfig(t)
	account := secretAccountKey("fixture-app")
	if err := keychain.Set(keychain.Service, account, "parent-test-secret"); err != nil {
		t.Fatal(err)
	}
	parentNamespace := os.Getenv(keychain.TestNamespaceEnv)
	t.Run("isolated", func(t *testing.T) {
		externalExchangeTestConfig(t)
		if os.Getenv(keychain.TestNamespaceEnv) == parentNamespace || keychain.Exists(keychain.Service, account) {
			t.Fatal("exchange fixture inherited another test's credentials")
		}
		if err := keychain.Set(keychain.Service, account, "child-test-secret"); err != nil {
			t.Fatal(err)
		}
	})
	if value, err := keychain.Get(keychain.Service, account); err != nil || value != "parent-test-secret" {
		t.Fatalf("child cleanup changed parent credentials: %v", err)
	}
}

func TestExternalExchangeEmptySandboxPersistsIdentityAndRefreshesWithOriginalClient(t *testing.T) {
	for _, selection := range []string{"", "default", "custom-app"} {
		t.Run("client="+selection, func(t *testing.T) {
			dir := externalExchangeTestConfig(t)
			wantClient := "official-app"
			if selection == "custom-app" {
				wantClient = selection
			}
			discoveries, exchanges, refreshes := 0, 0, 0
			externalExchangeTestServer(t, dir, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == ClientIDPath {
					discoveries++
					io.WriteString(w, `{"success":true,"result":"official-app"}`)
					return
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["clientId"] != wantClient || body["clientSecret"] != "" {
					t.Error("wrong application or leaked unrelated secret")
				}
				switch r.URL.Path {
				case MCPOAuthTokenPath:
					exchanges++
					if body["authCode"] != "test-code" {
						t.Error("missing auth code")
					}
				case MCPRefreshTokenPath:
					refreshes++
					if body["refreshToken"] != "test-refresh" || body["grantType"] != "refresh_token" {
						t.Error("bad refresh request")
					}
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				io.WriteString(w, `{"accessToken":"test-access","refreshToken":"test-refresh","expiresIn":7200,"corpId":"external-corp"}`)
			})
			data, err := ExchangeExternalAuthCode(context.Background(), dir, ExternalExchangeRequest{
				ClientID: selection, AuthCode: "test-code",
				ResolveIdentity: func(_ context.Context, token, corp string) (ManagedIdentity, error) {
					if token != "test-access" || corp != "external-corp" {
						t.Fatal("identity lookup did not use new token")
					}
					return ManagedIdentity{CorpID: corp, UserID: "external-user", UserName: "Employee"}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadProfiles(dir)
			if err != nil || cfg.CurrentProfile != "external-corp:external-user" {
				t.Fatalf("initial default not saved: %v", err)
			}
			loaded, err := LoadTokenDataForProfile(dir, cfg.CurrentProfile)
			if err != nil || loaded.UserID != "external-user" || loaded.ClientID != wantClient || loaded.Source != "mcp" {
				t.Fatalf("incomplete saved identity/application: %v", err)
			}
			SetClientID("unrelated-app")
			SetClientSecret("unrelated-secret")
			provider := &OAuthProvider{configDir: dir, Output: io.Discard}
			if _, err := provider.refreshWithRefreshToken(context.Background(), data); err != nil {
				t.Fatal(err)
			}
			wantDiscovery := 1
			if selection == "custom-app" {
				wantDiscovery = 0
			}
			if discoveries != wantDiscovery || exchanges != 1 || refreshes != 1 {
				t.Fatalf("unexpected calls discovery=%d exchange=%d refresh=%d", discoveries, exchanges, refreshes)
			}
		})
	}
}

func TestExternalExchangePreservesExistingDefaultAndUpdatesExactSlot(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	oldProfile := RuntimeProfile()
	SetRuntimeProfile("")
	t.Cleanup(func() { SetRuntimeProfile(oldProfile) })
	supervisor := &TokenData{AccessToken: "supervisor-token", CorpID: "supervisor-corp", UserID: "supervisor-user", ExpiresAt: time.Now().Add(time.Hour)}
	if err := SaveTokenData(dir, supervisor); err != nil {
		t.Fatal(err)
	}
	SetRuntimeProfile("unrelated:selector")
	externalExchangeTestServer(t, dir, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"accessToken":"employee-token","refreshToken":"employee-refresh","expiresIn":7200,"corpId":"employee-corp"}`)
	})
	request := ExternalExchangeRequest{ClientID: "employee-app", AuthCode: "test-code", ResolveIdentity: func(_ context.Context, _, corp string) (ManagedIdentity, error) {
		return ManagedIdentity{CorpID: corp, UserID: "employee-user"}, nil
	}}
	for i := 0; i < 2; i++ {
		if _, err := ExchangeExternalAuthCode(context.Background(), dir, request); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := LoadProfiles(dir)
	if err != nil || cfg.CurrentProfile != "supervisor-corp:supervisor-user" || len(cfg.Profiles) != 2 {
		t.Fatalf("default or slots changed: %v", err)
	}
	loaded, err := LoadTokenDataForProfile(dir, cfg.CurrentProfile)
	if err != nil || loaded.AccessToken != "supervisor-token" {
		t.Fatal("supervisor overwritten")
	}
	if RuntimeProfile() != "unrelated:selector" {
		t.Fatal("runtime selector changed")
	}
}

func TestExternalExchangeRejectsInvalidIdentityAndNeverTriesAnotherApplication(t *testing.T) {
	for _, kind := range []string{"lookup-error", "missing-user", "wrong-corp", "expected-user", "expected-corp", "server-error", "missing-token-corp"} {
		t.Run(kind, func(t *testing.T) {
			dir := externalExchangeTestConfig(t)
			requests := 0
			externalExchangeTestServer(t, dir, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if kind == "server-error" {
					w.WriteHeader(403)
					io.WriteString(w, "test-code test-access test-refresh")
					return
				}
				corp := "corp"
				if kind == "missing-token-corp" {
					corp = ""
				}
				fmt.Fprintf(w, `{"accessToken":"test-access","refreshToken":"test-refresh","expiresIn":7200,"corpId":%q}`, corp)
			})
			request := ExternalExchangeRequest{ClientID: "custom-app", AuthCode: "test-code", ResolveIdentity: func(_ context.Context, _, corp string) (ManagedIdentity, error) {
				if kind == "lookup-error" {
					return ManagedIdentity{}, fmt.Errorf("test-access")
				}
				id := ManagedIdentity{CorpID: corp, UserID: "user"}
				if kind == "missing-user" {
					id.UserID = ""
				}
				if kind == "wrong-corp" {
					id.CorpID = "other"
				}
				return id, nil
			}}
			if kind == "expected-user" {
				request.ExpectedUserID = "other"
			}
			if kind == "expected-corp" {
				request.ExpectedCorpID = "other"
			}
			_, err := ExchangeExternalAuthCode(context.Background(), dir, request)
			if err == nil {
				t.Fatal("invalid identity accepted")
			}
			for _, secret := range []string{"test-code", "test-access", "test-refresh"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatal("secret leaked")
				}
			}
			cfg, loadErr := LoadProfiles(dir)
			if loadErr != nil || len(cfg.Profiles) != 0 {
				t.Fatal("invalid identity persisted")
			}
			if requests != 1 {
				t.Fatalf("retried one-time code: %d requests", requests)
			}
		})
	}
}

func TestExternalExchangeApplicationSelectionKeepsMatchedDirectCredentials(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	SetClientID("configured-app")
	SetClientSecret("configured-secret")
	discoveries := 0
	externalExchangeTestServer(t, dir, func(w http.ResponseWriter, r *http.Request) {
		discoveries++
		io.WriteString(w, `{"success":true,"result":"official-app"}`)
	})
	for _, tc := range []struct{ requested, explicit, id, secret string }{
		{"", "", "configured-app", "configured-secret"},
		{"configured-app", "", "configured-app", "configured-secret"},
		{"other-app", "", "other-app", ""},
		{"default", "", "official-app", ""},
		{"other-app", "explicit-secret", "other-app", "explicit-secret"},
	} {
		id, secret, _, err := resolveExternalExchangeClient(context.Background(), dir, tc.requested, tc.explicit)
		if err != nil || id != tc.id || secret != tc.secret {
			t.Fatalf("incorrect application/credential pairing for %q: %v", tc.requested, err)
		}
	}
	if _, _, _, err := resolveExternalExchangeClient(context.Background(), dir, "default", "explicit-secret"); err == nil {
		t.Fatal("default accepted a local secret")
	}
	if discoveries != 1 {
		t.Fatal("unexpected discovery or network call")
	}
}

func TestExternalExchangeDirectCredentialsAreSavedOnlyAfterIdentityVerification(t *testing.T) {
	for _, valid := range []bool{false, true} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			dir := externalExchangeTestConfig(t)
			SetClientID("direct-app")
			SetClientSecret("direct-secret")
			requests, secretWrites := 0, 0
			testseam.Swap(t, &oauthHTTPClient, &http.Client{Transport: postJSONRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				if r.URL.String() != UserAccessTokenURL || body["clientId"] != "direct-app" || body["clientSecret"] != "direct-secret" || body["code"] != "test-code" {
					t.Fatal("direct credentials/endpoint mismatch")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"accessToken":"direct-access","refreshToken":"direct-refresh","expiresIn":7200,"corpId":"direct-corp"}`))}, nil
			})})
			testseam.Swap(t, &oauthSaveClientSecret, func(id, secret string) error {
				secretWrites++
				if id != "direct-app" || secret != "direct-secret" {
					t.Fatal("wrong secret persisted")
				}
				return nil
			})
			data, err := ExchangeExternalAuthCode(context.Background(), dir, ExternalExchangeRequest{
				AuthCode: "test-code", ResolveIdentity: func(_ context.Context, _, corp string) (ManagedIdentity, error) {
					if !valid {
						return ManagedIdentity{}, fmt.Errorf("lookup failed")
					}
					return ManagedIdentity{CorpID: corp, UserID: "direct-user"}, nil
				},
			})
			if (err == nil) != valid || requests != 1 {
				t.Fatalf("unexpected direct result: %v", err)
			}
			if valid {
				if secretWrites != 1 || data.ClientID != "direct-app" || data.Source == "mcp" {
					t.Fatal("direct credential metadata missing")
				}
			} else if secretWrites != 0 {
				t.Fatal("saved credentials before identity verification")
			}
		})
	}
}
