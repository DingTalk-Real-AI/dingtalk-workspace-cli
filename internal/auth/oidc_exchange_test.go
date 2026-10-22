package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dwserrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageOIDCExchangePrepareAndExchange(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var body map[string]string
		if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || r.Header.Get("Authorization") != "" {
			t.Error("invalid portal request")
		}
		switch r.URL.Path {
		case "/cli/auth/oidc/transactions":
			if len(body) != 1 || body["provider"] != "ant" {
				t.Error("unexpected preparation parameters")
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(oidcTestCreated("https://idp.example/authorize", "ant-sdk://oidc/callback"))
		case "/cli/auth/oidc/exchange":
			if len(body) != 5 || body["provider"] != "ant" || body["code"] != " original+code%20 " || body["state"] != oidcTestState || body["client_id"] != "dws-app" || body["client_secret"] != "dws-secret" {
				t.Error("exchange changed opaque parameters or lost explicit provider/credentials")
			}
			json.NewEncoder(w).Encode(oidcTestCompleted())
		default:
			t.Error("split flow called SDK or another endpoint")
		}
	}))
	defer server.Close()
	p := NewOIDCLoginProvider("ant", server.URL, nil)
	tx, err := p.PrepareExchange(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if tx.Provider != "ant" || tx.ExpiresIn != 600 || tx.AuthorizationRequest.Parameters["state"] != oidcTestState || len(tx.AuthorizationRequest.Parameters) != 8 {
		t.Fatal("preparation did not preserve the complete authorization request")
	}
	p.ExchangeCredentials = &OIDCExchangeCredentials{ClientID: "dws-app", ClientSecret: "dws-secret"}
	data, err := p.Exchange(t.Context(), " original+code%20 ", oidcTestState)
	if err != nil {
		t.Fatal(err)
	}
	if data.AccessToken != "secret-access" || data.RefreshToken != "secret-refresh" || data.CorpID != "corp" || data.ClientID != "dws-app" || !data.FreshAuthorization || data.Source != "mcp" {
		t.Fatal("exchange changed the existing token projection")
	}
	if len(paths) != 2 {
		t.Fatal("unexpected request count")
	}
}

func TestCrossPlatformCoverageOIDCSavedTokenRefreshKeepsExchangeApplication(t *testing.T) {
	for _, entry := range []string{"provider-login", "token-exchange", "provider-login-default", "token-exchange-default"} {
		t.Run(entry, func(t *testing.T) {
			custom := !strings.HasSuffix(entry, "-default")
			wantClient := "managed-app"
			if custom {
				wantClient = "exchanged-app-b"
			}
			dir := externalExchangeTestConfig(t)
			testseam.Swap(t, &runtimeClientID, "configured-app-a")
			previousProfile := RuntimeProfile()
			SetRuntimeProfile("")
			t.Cleanup(func() { SetRuntimeProfile(previousProfile) })
			refreshCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				switch r.URL.Path {
				case "/cli/auth/oidc/transactions":
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(oidcTestCreated("https://idp.example/authorize", "ant-sdk://oidc/callback"))
					return
				case "/cli/auth/oidc/exchange":
					if custom && (body["client_id"] != wantClient || body["client_secret"] != " exchange-secret ") {
						t.Error("exchange lost explicit application credentials")
					}
					if !custom && (body["client_id"] != "" || body["client_secret"] != "") {
						t.Error("managed exchange received local application credentials")
					}
				case MCPRefreshTokenPath:
					refreshCalls++
					wantSecret := ""
					if custom {
						wantSecret = " exchange-secret "
					}
					if body["clientId"] != wantClient || body["clientSecret"] != wantSecret {
						t.Errorf("refresh used another application: %q", body["clientId"])
						w.WriteHeader(http.StatusBadRequest)
						return
					}
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				completed := oidcTestCompleted()
				completed["userId"] = "new-user"
				completed["clientId"] = wantClient
				_ = json.NewEncoder(w).Encode(completed)
			}))
			defer server.Close()
			t.Cleanup(PushMCPBaseURLOverride(server.URL))
			p := NewOIDCLoginProvider("ant", server.URL, oidcAuthorizeFunc(func(_ context.Context, request OIDCAuthorizationRequest) (OIDCAuthorizationResult, error) {
				return OIDCAuthorizationResult{Code: "code", State: request.Parameters["state"]}, nil
			}))
			if custom {
				p.ExchangeCredentials = &OIDCExchangeCredentials{ClientID: wantClient, ClientSecret: " exchange-secret "}
			}
			var err error
			if strings.HasPrefix(entry, "provider-login") {
				_, err = p.LoginAndSave(t.Context(), dir)
			} else {
				_, err = p.ExchangeAndSave(t.Context(), dir, "code", oidcTestState, func(context.Context, string, string) (ManagedIdentity, error) {
					return ManagedIdentity{CorpID: "corp", UserID: "new-user"}, nil
				})
			}
			if err != nil {
				t.Fatal(err)
			}
			// Read the persisted token, as a later CLI process would, rather than
			// refreshing the in-memory result of the exchange.
			data, err := LoadTokenDataForProfile(dir, "corp:new-user")
			if err != nil {
				t.Fatal(err)
			}
			if data.ClientID != wantClient || data.MCPClientSecretRequired != custom {
				t.Errorf("saved token lost application binding: %q", data.ClientID)
			}
			wantSavedSecret := ""
			if custom {
				wantSavedSecret = " exchange-secret "
			}
			if secret, err := LoadClientSecretStrict(wantClient); err != nil || secret != wantSavedSecret {
				t.Errorf("saved token lost refresh credentials: %v", err)
			}
			encoded, err := json.Marshal(data)
			var stored map[string]any
			if json.Unmarshal(encoded, &stored) != nil || stored["oidc_provider"] != "ant" {
				t.Fatal("saved token lost explicit OIDC provider")
			}
			if err != nil || strings.Contains(string(encoded), "exchange-secret") {
				t.Fatal("token projection exposed the application secret")
			}
			err = withProfilesLock(dir, func() error {
				_, refreshErr := (&OAuthProvider{configDir: dir, httpClient: server.Client()}).refreshWithRefreshToken(t.Context(), data)
				return refreshErr
			})
			if err != nil {
				t.Fatal(err)
			}
			refreshed, err := LoadTokenDataForProfile(dir, "corp:new-user")
			if err != nil || refreshed.OIDCProvider != "ant" || refreshed.ClientID != wantClient || refreshed.MCPClientSecretRequired != custom || refreshed.MCPBaseURL != server.URL || refreshCalls != 1 {
				t.Fatalf("refresh did not preserve application binding: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCExchangeValidationAndFailure(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(400)
		io.WriteString(w, `{"error":{"code":"provider_mismatch","action":"restart_login","message":"private-code"}}`)
	}))
	defer server.Close()
	p := NewOIDCLoginProvider("ant", server.URL, nil)
	for _, input := range [][2]string{{"", oidcTestState}, {" ", oidcTestState}, {"code\n", oidcTestState}, {strings.Repeat("c", 8193), oidcTestState}, {"code", ""}, {"code", "bad state"}, {"code", strings.Repeat("s", 1025)}} {
		_, err := p.Exchange(t.Context(), input[0], input[1])
		var typed *dwserrors.Error
		if !errors.As(err, &typed) || typed.Category != dwserrors.CategoryValidation {
			t.Fatalf("expected validation, got %v", err)
		}
	}
	if requests != 0 {
		t.Fatal("invalid inputs reached portal")
	}
	_, err := p.Exchange(t.Context(), "private-code", oidcTestState)
	if err == nil || strings.Contains(err.Error(), "private-code") || requests != 1 {
		t.Fatal("unsafe error or automatic retry")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.Exchange(ctx, "code", oidcTestState); !errors.Is(err, context.Canceled) || requests != 1 {
		t.Fatal("canceled exchange reached portal")
	}
}

func TestCrossPlatformCoverageOIDCApplicationBindingValidation(t *testing.T) {
	for _, scenario := range []string{"missing-managed-id", "wrong-custom-id", "legacy-custom-response"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := oidcTestCompleted()
				delete(body, "clientId")
				if scenario == "wrong-custom-id" {
					body["clientId"] = "another-app"
				}
				_ = json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			p := NewOIDCLoginProvider("ant", server.URL, nil)
			if scenario != "missing-managed-id" {
				p.ExchangeCredentials = &OIDCExchangeCredentials{ClientID: "custom-app", ClientSecret: "secret"}
			}
			data, err := p.Exchange(t.Context(), "code", oidcTestState)
			if scenario == "legacy-custom-response" {
				if err != nil || data.ClientID != "custom-app" {
					t.Fatalf("explicit application binding was lost: %v", err)
				}
			} else if err == nil || data != nil {
				t.Fatal("accepted a missing or mismatched application binding")
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCRefreshSecretAndEndpointBoundaries(t *testing.T) {
	for _, scenario := range []string{"missing-secret", "missing-endpoint", "redirect", "managed-secret-present", "invalid-response", "503-html", "429-empty", "503-oversized"} {
		t.Run(scenario, func(t *testing.T) {
			dir := externalExchangeTestConfig(t)
			requests := 0
			trap := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("refresh credentials reached a different Portal or redirect target")
			}))
			defer trap.Close()
			t.Cleanup(PushMCPBaseURLOverride(trap.URL))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				if scenario == "managed-secret-present" && body["clientSecret"] != "" {
					t.Error("managed refresh leaked an unrelated local secret")
				}
				switch scenario {
				case "invalid-response":
					io.WriteString(w, `{"errorCode":"invalid","errorMsg":"secret-canary"}`)
					return
				case "503-html":
					w.WriteHeader(http.StatusServiceUnavailable)
					io.WriteString(w, "<html>secret-canary</html>")
					return
				case "429-empty":
					w.WriteHeader(http.StatusTooManyRequests)
					return
				case "503-oversized":
					w.WriteHeader(http.StatusServiceUnavailable)
					io.WriteString(w, strings.Repeat("x", 65<<10))
					return
				}
				w.Header().Set("Location", trap.URL)
				w.WriteHeader(http.StatusTemporaryRedirect)
				io.WriteString(w, `{}`)
			}))
			defer server.Close()
			data := &TokenData{ClientID: "app", Source: "mcp", OIDCProvider: "ant", RefreshToken: "refresh", MCPBaseURL: server.URL, MCPClientSecretRequired: scenario != "managed-secret-present"}
			if scenario == "missing-endpoint" {
				data.MCPBaseURL = ""
			}
			if scenario != "missing-secret" {
				if err := SaveClientSecret("app", "secret-canary"); err != nil {
					t.Fatal(err)
				}
			}
			_, err := (&OAuthProvider{configDir: dir}).refreshWithRefreshToken(t.Context(), data)
			if err == nil || strings.Contains(err.Error(), "secret-canary") {
				t.Fatal("refresh failure was accepted or leaked a secret")
			}
			wantRequests := 0
			if scenario != "missing-secret" && scenario != "missing-endpoint" {
				wantRequests = 1
			}
			if requests != wantRequests {
				t.Fatalf("unexpected refresh request count: %d", requests)
			}
			if (strings.HasPrefix(scenario, "503") || scenario == "429-empty") && ClassifyRefreshFailure(err) != RefreshFailureTransient {
				t.Fatalf("temporary Portal failure would expire the profile: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCRefreshRejectionRetainsSafeCode(t *testing.T) {
	for _, tc := range []struct {
		name         string
		code         string
		wantRejected bool
	}{
		{"rejected", legacyMCPRefreshRejectedCode, true},
		{"padded-rejection", " " + legacyMCPRefreshRejectedCode + " ", true},
		{"unknown-code", "secret-canary-code", false},
		{"message-only", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != MCPRefreshTokenPath {
					t.Errorf("unexpected refresh endpoint: %s", r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"errorCode": tc.code, "errorMsg": "secret-canary-message"})
			}))
			defer server.Close()
			p := &OAuthProvider{httpClient: server.Client()}
			_, err := p.refreshWithRefreshToken(t.Context(), &TokenData{ClientID: "app", Source: "mcp", OIDCProvider: "ant", RefreshToken: "refresh", MCPBaseURL: server.URL})
			var classified *dwserrors.Error
			if !errors.As(err, &classified) || classified.Category != dwserrors.CategoryAuth {
				t.Fatalf("want classified auth error, got %v", err)
			}
			var business *MCPTokenExchangeError
			if got := errors.As(err, &business); got != tc.wantRejected || isRefreshTokenRejected(err) != tc.wantRejected {
				t.Fatalf("rejection classification = %t, want %t: %v", got, tc.wantRejected, err)
			}
			if business != nil && business.Code != legacyMCPRefreshRejectedCode {
				t.Fatalf("unexpected preserved code: %q", business.Code)
			}
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				if strings.Contains(cause.Error(), "secret-canary") {
					t.Fatal("refresh error chain retained untrusted response data")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCCredentialPersistenceRollsBack(t *testing.T) {
	for _, entry := range []string{"provider-login", "token-exchange"} {
		t.Run(entry, func(t *testing.T) {
			dir := externalExchangeTestConfig(t)
			previousProfile := RuntimeProfile()
			SetRuntimeProfile("")
			t.Cleanup(func() { SetRuntimeProfile(previousProfile) })
			old := &TokenData{AccessToken: "previous-token", CorpID: "corp", UserID: "user", ClientID: "app", ExpiresAt: time.Now().Add(time.Hour)}
			if err := SaveTokenData(dir, old); err != nil {
				t.Fatal(err)
			}
			if err := SaveClientSecret("app", "previous-secret"); err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &tokenWriteMarker, func(string) error { return errors.New("injected marker failure") })
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/cli/auth/oidc/transactions" {
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(oidcTestCreated("https://idp.example/authorize", "ant-sdk://oidc/callback"))
					return
				}
				body := oidcTestCompleted()
				body["clientId"], body["userId"] = "app", "user"
				_ = json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			p := NewOIDCLoginProvider("ant", server.URL, oidcAuthorizeFunc(func(_ context.Context, request OIDCAuthorizationRequest) (OIDCAuthorizationResult, error) {
				return OIDCAuthorizationResult{Code: "code", State: request.Parameters["state"]}, nil
			}))
			p.ExchangeCredentials = &OIDCExchangeCredentials{ClientID: "app", ClientSecret: "new-secret"}
			var err error
			if entry == "provider-login" {
				_, err = p.LoginAndSave(t.Context(), dir)
			} else {
				_, err = p.ExchangeAndSave(t.Context(), dir, "code", oidcTestState, func(context.Context, string, string) (ManagedIdentity, error) {
					return ManagedIdentity{CorpID: "corp", UserID: "user"}, nil
				})
			}
			if err == nil {
				t.Fatal("injected persistence failure was ignored")
			}
			loaded, err := LoadTokenDataForProfile(dir, "corp:user")
			if err != nil || loaded.AccessToken != "previous-token" {
				t.Fatal("previous token was not restored")
			}
			if secret, err := LoadClientSecretStrict("app"); err != nil || secret != "previous-secret" {
				t.Fatal("previous application secret was not restored")
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCExchangePrepareRejectsInvalidAuthorization(t *testing.T) {
	for _, scenario := range []string{"provider", "verifier", "nonce", "challenge", "expires", "status"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tx := oidcTestCreated("https://idp.example/authorize", "ant-sdk://oidc/callback")
				params := tx["authorization_request"].(map[string]any)["parameters"].(map[string]string)
				status := 201
				switch scenario {
				case "provider":
					tx["provider"] = "another"
				case "verifier":
					params["code_verifier"] = "private-verifier"
				case "nonce":
					delete(params, "nonce")
				case "challenge":
					params["code_challenge_method"] = "plain"
				case "expires":
					tx["expires_in"] = 0
				case "status":
					status = 200
				}
				w.WriteHeader(status)
				json.NewEncoder(w).Encode(tx)
			}))
			defer server.Close()
			if tx, err := NewOIDCLoginProvider("ant", server.URL, nil).PrepareExchange(t.Context()); err == nil || tx != nil || strings.Contains(err.Error(), "private-verifier") {
				t.Fatal("unsafe preparation accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCExchangeSaveIdentityAndDefault(t *testing.T) {
	for _, scenario := range []string{"first", "existing", "lookup-fails", "typed-lookup-fails", "missing-user", "wrong-corp", "wrong-user", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(keychain.StorageDirEnv, filepath.Join(dir, "keys"))
			t.Setenv(keychain.DisableKeychainEnv, "1")
			t.Setenv(keychain.TestNamespaceEnv, dir)
			SetRuntimeProfile("")
			t.Cleanup(func() { SetRuntimeProfile("") })
			if scenario != "first" {
				if err := SaveTokenData(dir, &TokenData{AccessToken: "old-access", CorpID: "old-corp", UserID: "old-user", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				completed := oidcTestCompleted()
				if scenario == "wrong-user" {
					completed["userId"] = "another-user"
				}
				json.NewEncoder(w).Encode(completed)
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			resolve := func(_ context.Context, token, corp string) (ManagedIdentity, error) {
				if token != "secret-access" || corp != "corp" {
					t.Error("lookup did not use new token")
				}
				id := ManagedIdentity{CorpID: corp, UserID: "new-user", UserName: "New User"}
				switch scenario {
				case "lookup-fails":
					return id, errors.New("secret-access")
				case "typed-lookup-fails":
					return id, dwserrors.NewAPI("secret-access", dwserrors.WithCause(errors.New("secret-access")))
				case "missing-user":
					id.UserID = ""
				case "wrong-corp":
					id.CorpID = "other"
				case "canceled":
					cancel()
				}
				return id, nil
			}
			p := NewOIDCLoginProvider("ant", server.URL, nil)
			data, err := p.ExchangeAndSave(ctx, dir, "code", oidcTestState, resolve)
			cfg, loadErr := LoadProfiles(dir)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if scenario == "first" || scenario == "existing" {
				if err != nil || data.UserID != "new-user" {
					t.Fatalf("save failed: %v", err)
				}
				wantCurrent := "old-corp:old-user"
				if scenario == "first" {
					wantCurrent = "corp:new-user"
				}
				if cfg.CurrentProfile != wantCurrent {
					t.Fatal("save switched existing default")
				}
				loaded, err := LoadTokenDataForProfile(dir, "corp:new-user")
				if err != nil || loaded.AccessToken != "secret-access" {
					t.Fatal("missing saved token")
				}
			} else {
				if err == nil || data != nil || strings.Contains(err.Error(), "secret-access") || len(cfg.Profiles) != 1 || cfg.CurrentProfile != "old-corp:old-user" {
					t.Fatal("identity failure changed existing credentials or leaked token")
				}
			}
		})
	}
}
