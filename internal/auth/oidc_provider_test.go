package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dwserrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const oidcTestState = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestCrossPlatformCoverageOIDCSDKTransactionBeforeLogin(t *testing.T) {
	var calls []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/cli/auth/oidc/transactions":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if len(body) != 1 || body["provider"] != "ant" {
				w.WriteHeader(400)
				io.WriteString(w, `{"error":{"code":"invalid_request","action":"stop"}}`)
				return
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(oidcTestCreated(server.URL+"/cli/auth/oidc/mock/sdk/authorize", "ant-sdk://oidc/callback"))
		case "/cli/auth/oidc/mock/sdk/login":
			json.NewEncoder(w).Encode(map[string]any{"sdk_session_token": "sdk-secret", "expires_in": 60})
		case "/cli/auth/oidc/mock/sdk/authorize":
			if r.Header.Get("Authorization") != "Bearer sdk-secret" {
				t.Error("SDK authorization lost its session")
			}
			json.NewEncoder(w).Encode(map[string]string{"code": " original-code ", "state": oidcTestState})
		case "/cli/auth/oidc/exchange":
			json.NewEncoder(w).Encode(oidcTestCompleted())
		default:
			t.Error("unexpected request")
		}
	}))
	defer server.Close()
	p := NewAntSDKOIDCProvider(server.URL).OIDCLoginProvider
	if _, err := p.Login(t.Context()); err != nil {
		t.Fatalf("SDK login did not complete: %v", err)
	}
	want := "/cli/auth/oidc/transactions,/cli/auth/oidc/mock/sdk/login,/cli/auth/oidc/mock/sdk/authorize,/cli/auth/oidc/exchange"
	if strings.Join(calls, ",") != want {
		t.Fatalf("incorrect SDK request order: %v", calls)
	}
}

func oidcTestCompleted() map[string]any {
	return map[string]any{"accessToken": "secret-access", "refreshToken": "secret-refresh", "expiresIn": 3600, "corpId": "corp", "corpName": "测试组织"}
}

func oidcTestCreated(endpoint, redirect string) map[string]any {
	return map[string]any{"provider": "ant", "expires_in": 600, "authorization_request": map[string]any{"authorization_endpoint": endpoint, "parameters": map[string]string{"client_id": "registered-app", "response_type": "code", "scope": "openid", "redirect_uri": redirect, "state": oidcTestState, "nonce": oidcTestState, "code_challenge": oidcTestState, "code_challenge_method": "S256"}}}
}

func oidcTestProvider(t *testing.T, exchange http.HandlerFunc) *OIDCLoginProvider {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("invalid SDK flow request")
		}
		switch r.URL.Path {
		case "/cli/auth/oidc/transactions":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["provider"] != "ant" || len(body) != 1 {
				t.Error("transaction must contain only provider")
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(oidcTestCreated(server.URL+"/cli/auth/oidc/mock/sdk/authorize", "ant-sdk://oidc/callback"))
		case "/cli/auth/oidc/mock/sdk/login":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 0 || r.Header.Get("Authorization") != "" {
				t.Error("SDK login must not receive credentials")
			}
			json.NewEncoder(w).Encode(map[string]any{"sdk_session_token": "sdk-secret", "expires_in": 60})
		case "/cli/auth/oidc/mock/sdk/authorize":
			var request OIDCAuthorizationRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if r.Header.Get("Authorization") != "Bearer sdk-secret" || request.AuthorizationEndpoint != server.URL+"/cli/auth/oidc/mock/sdk/authorize" || len(request.Parameters) != 8 || request.Parameters["redirect_uri"] != "ant-sdk://oidc/callback" || request.Parameters["state"] != oidcTestState || request.Parameters["client_secret"] != "" || request.Parameters["client_id"] != "registered-app" {
				t.Error("SDK authorization request was not preserved")
			}
			json.NewEncoder(w).Encode(map[string]string{"code": " original-code ", "state": oidcTestState})
		case "/cli/auth/oidc/exchange":
			if r.Header.Get("Authorization") != "" {
				t.Error("SDK session leaked to token exchange")
			}
			exchange(w, r)
		default:
			t.Error("unexpected OIDC endpoint")
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	return NewAntSDKOIDCProvider(server.URL).OIDCLoginProvider
}

func TestCrossPlatformCoverageOIDCProviderHTTPFlow(t *testing.T) {
	var count atomic.Int32
	p := oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if len(body) != 2 || body["code"] != " original-code " || body["state"] != oidcTestState {
			t.Error("exchange changed original code/state")
		}
		count.Add(1)
		json.NewEncoder(w).Encode(oidcTestCompleted())
	})
	data, err := p.Login(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if data.AccessToken != "secret-access" || data.RefreshToken != "secret-refresh" || data.Source != "mcp" || data.UserID != "" || data.ClientID != "" || data.CorpID != "corp" || data.CorpName != "测试组织" || data.RefreshExpAt.Sub(data.ExpiresAt) < 22*time.Hour {
		t.Fatalf("incorrect session projection: %#v", data)
	}
	if count.Load() != 1 {
		t.Fatal("exchange must be sent once")
	}
}

func TestCrossPlatformCoverageOIDCProviderRejectsIncompleteSessions(t *testing.T) {
	for _, field := range []string{"accessToken"} {
		t.Run(field, func(t *testing.T) {
			p := oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				body := oidcTestCompleted()
				delete(body, field)
				json.NewEncoder(w).Encode(body)
			})
			data, err := p.Login(t.Context())
			if err == nil || data != nil || strings.Contains(err.Error(), "secret-access") {
				t.Fatalf("invalid/unsafe session accepted: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCProviderErrorsAndNoRedirect(t *testing.T) {
	for _, status := range []int{400, 401, 409, 410, 500, 503, 307} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			p := oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://example.invalid/leak")
				w.WriteHeader(status)
				io.WriteString(w, `{"error":{"message":"secret-access original-code"}}`)
			})
			_, err := p.Login(t.Context())
			if err == nil || strings.Contains(err.Error(), "secret-access") || strings.Contains(err.Error(), "original-code") {
				t.Fatalf("unsafe protocol error: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCProviderRejectsUnsafeEndpoints(t *testing.T) {
	for _, endpoint := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com?token=secret", "ftp://example.com"} {
		p := NewOIDCLoginProvider("ant", endpoint, nil)
		if err := p.Validate(); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe endpoint accepted/leaked: %v", err)
		}
	}
}

type oidcAuthorizeFunc func(context.Context, OIDCAuthorizationRequest) (OIDCAuthorizationResult, error)

func (f oidcAuthorizeFunc) Authorize(ctx context.Context, request OIDCAuthorizationRequest) (OIDCAuthorizationResult, error) {
	return f(ctx, request)
}

func TestCrossPlatformCoverageOIDCBusinessAuthorizerReusesCommonFlow(t *testing.T) {
	const provider = "enterprise_acme"
	const endpoint = "https://enterprise.example/authorize"
	var stage atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/cli/auth/oidc/transactions":
			if !stage.CompareAndSwap(0, 1) || body["provider"] != provider || len(body) != 1 {
				t.Error("common flow did not start with the selected provider transaction")
			}
			created := oidcTestCreated(endpoint, "enterprise-sdk://oidc/callback")
			created["provider"] = provider
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(created)
		case "/cli/auth/oidc/exchange":
			if !stage.CompareAndSwap(2, 3) || body["code"] != "enterprise-code" || body["state"] != oidcTestState || len(body) != 2 {
				t.Error("common exchange lost the business authorization result")
			}
			completed := oidcTestCompleted()
			json.NewEncoder(w).Encode(completed)
		default:
			t.Error("common flow attempted an Ant SDK request")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	authorizer := oidcAuthorizeFunc(func(ctx context.Context, request OIDCAuthorizationRequest) (OIDCAuthorizationResult, error) {
		if !stage.CompareAndSwap(1, 2) || request.AuthorizationEndpoint != endpoint || request.Parameters["state"] != oidcTestState {
			t.Error("business authorization ran before the transaction or lost its parameters")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("business authorization has no deadline")
		}
		return OIDCAuthorizationResult{Code: "enterprise-code", State: oidcTestState}, nil
	})
	p := NewOIDCLoginProvider(provider, server.URL, authorizer)
	dir := externalExchangeTestConfig(t)
	if _, err := p.LoginAndSave(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadTokenDataForProfile(dir, "corp")
	if err != nil || saved.RefreshToken != "secret-refresh" || stage.Load() != 3 {
		t.Fatalf("common flow did not save the business session: stage=%d error=%v", stage.Load(), err)
	}
}

func TestCrossPlatformCoverageOIDCSDKConfigurationFailurePreventsSDK(t *testing.T) {
	for _, code := range []string{"unsupported_provider", "temporarily_unavailable", "provider_unavailable"} {
		t.Run(code, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/cli/auth/oidc/transactions" {
					t.Error("configuration failure started SDK")
				}
				status := http.StatusServiceUnavailable
				if code == "unsupported_provider" {
					status = http.StatusBadRequest
				}
				w.WriteHeader(status)
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "action": "stop", "message": "private-config"}})
			}))
			defer server.Close()
			p := NewAntSDKOIDCProvider(server.URL).OIDCLoginProvider
			p.authorizer = oidcAuthorizeFunc(func(context.Context, OIDCAuthorizationRequest) (OIDCAuthorizationResult, error) {
				t.Error("unconfigured provider called SDK")
				return OIDCAuthorizationResult{}, nil
			})
			_, err := p.Login(t.Context())
			if err == nil || !strings.Contains(err.Error(), "not configured") || strings.Contains(err.Error(), "private") || calls.Load() != 1 {
				t.Fatalf("configuration failure not clear/immediate/safe: calls=%d error=%v", calls.Load(), err)
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCSDKUnavailableAfterTransaction(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/cli/auth/oidc/transactions" {
			t.Error("production SDK attempted an invented HTTP protocol")
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(oidcTestCreated("https://buc.example/authorize", "ant-sdk://oidc/callback"))
	}))
	defer server.Close()
	p := NewAntSDKOIDCProvider(server.URL).OIDCLoginProvider
	_, err := p.Login(t.Context())
	if err == nil || !strings.Contains(err.Error(), "SDK is not integrated") || calls.Load() != 1 {
		t.Fatalf("missing production SDK not reported after transaction: %v", err)
	}
}

func TestCrossPlatformCoverageOIDCSDKPreservesRequestAndValidatesResult(t *testing.T) {
	for _, mode := range []string{"valid", "empty-code", "empty-state", "wrong-state", "mutated-state", "sdk-error", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			var exchanged atomic.Int32
			p := oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				exchanged.Add(1)
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				if body["code"] != " original-code " || body["state"] != oidcTestState {
					t.Error("exchange changed SDK result")
				}
				json.NewEncoder(w).Encode(oidcTestCompleted())
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			p.authorizer = oidcAuthorizeFunc(func(ctx context.Context, request OIDCAuthorizationRequest) (OIDCAuthorizationResult, error) {
				want := oidcTestCreated(p.baseURL+"/cli/auth/oidc/mock/sdk/authorize", "ant-sdk://oidc/callback")["authorization_request"]
				actual, _ := json.Marshal(request)
				expected, _ := json.Marshal(want)
				if string(actual) != string(expected) {
					t.Error("SDK did not receive complete authorization_request")
				}
				result := OIDCAuthorizationResult{Code: " original-code ", State: oidcTestState}
				switch mode {
				case "empty-code":
					result.Code = " "
				case "empty-state":
					result.State = ""
				case "wrong-state":
					result.State = "wrong"
				case "mutated-state":
					request.Parameters["state"] = "wrong"
					result.State = "wrong"
				case "sdk-error":
					return result, fmt.Errorf("private-sdk-session")
				case "canceled":
					cancel()
				}
				return result, nil
			})
			_, err := p.Login(ctx)
			if mode == "valid" {
				if err != nil || exchanged.Load() != 1 {
					t.Fatalf("SDK success failed: %v", err)
				}
			} else if err == nil || exchanged.Load() != 0 || strings.Contains(err.Error(), "private") {
				t.Fatalf("SDK invalid result accepted/leaked: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCSDKHTTPBoundaries(t *testing.T) {
	for _, mode := range []string{"login-denied", "login-redirect", "empty-session", "session-expiry", "duplicate-session", "oversized-session", "authorize-denied", "authorize-redirect", "duplicate-result", "empty-state", "wrong-state"} {
		t.Run(mode, func(t *testing.T) {
			var leaked atomic.Int32
			trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
			defer trap.Close()
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/cli/auth/oidc/transactions":
					w.WriteHeader(201)
					json.NewEncoder(w).Encode(oidcTestCreated(server.URL+"/cli/auth/oidc/mock/sdk/authorize", "ant-sdk://oidc/callback"))
				case "/cli/auth/oidc/mock/sdk/login":
					switch mode {
					case "login-denied":
						w.WriteHeader(403)
						io.WriteString(w, `{"error":"private-sdk"}`)
					case "login-redirect":
						w.Header().Set("Location", trap.URL)
						w.WriteHeader(307)
						io.WriteString(w, `{}`)
					case "empty-session":
						io.WriteString(w, `{}`)
					case "session-expiry":
						io.WriteString(w, `{"sdk_session_token":"private-sdk","expires_in":61}`)
					case "duplicate-session":
						io.WriteString(w, `{"sdk_session_token":"private-sdk","sdk_session_token":"private-sdk","expires_in":60}`)
					case "oversized-session":
						io.WriteString(w, strings.Repeat("private-sdk", 7000))
					default:
						io.WriteString(w, `{"sdk_session_token":"private-sdk","expires_in":60}`)
					}
				case "/cli/auth/oidc/mock/sdk/authorize":
					switch mode {
					case "authorize-denied":
						w.WriteHeader(403)
						io.WriteString(w, `{"error":"private-sdk"}`)
					case "authorize-redirect":
						w.Header().Set("Location", trap.URL)
						w.WriteHeader(307)
						io.WriteString(w, `{}`)
					case "duplicate-result":
						io.WriteString(w, `{"code":"private-code","state":"wrong","state":"wrong"}`)
					case "empty-state":
						io.WriteString(w, `{"code":"private-code"}`)
					case "wrong-state":
						io.WriteString(w, `{"code":"private-code","state":"wrong"}`)
					default:
						t.Error("invalid SDK session reached authorization")
						w.WriteHeader(500)
					}
				default:
					t.Error("invalid SDK result reached exchange")
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			p := NewAntSDKOIDCProvider(server.URL).OIDCLoginProvider
			_, err := p.Login(t.Context())
			if err == nil || strings.Contains(err.Error(), "private") || leaked.Load() != 0 {
				t.Fatalf("SDK transport accepted/leaked unsafe data: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCProtocolNetworkClassification(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); <-r.Context().Done() }))
		p := NewOIDCLoginProvider("ant", server.URL, nil)
		if timeout {
			p.client = &http.Client{Timeout: 20 * time.Millisecond}
		} else {
			server.Close()
		}
		_, err := p.Login(t.Context())
		server.Close()
		var typed *dwserrors.Error
		if !errors.As(err, &typed) || typed.Category != dwserrors.CategoryAPI || dwserrors.ExitCode(err) != 1 {
			t.Fatalf("network error lost API classification: %v", err)
		}
		want := "NETWORK_UNREACHABLE"
		if timeout {
			want = "NETWORK_TIMEOUT"
		}
		if typed.Reason != want || strings.Contains(err.Error(), server.URL) {
			t.Fatalf("unsafe/wrong network classification: %#v", typed)
		}
	}
}

func TestCrossPlatformCoverageOIDCRequestsDoNotRetry(t *testing.T) {
	for _, operation := range []string{"transactions", "exchange"} {
		for _, status := range []int{202, 429, 503, 500} {
			t.Run(fmt.Sprintf("%s/%d", operation, status), func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.URL.Path != "/cli/auth/oidc/"+operation {
						t.Errorf("unexpected request: %s", r.URL.Path)
					}
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(status)
					if status == 202 {
						json.NewEncoder(w).Encode(map[string]any{})
						return
					}
					code := "temporarily_unavailable"
					if status == 429 {
						code = "rate_limited"
					}
					json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "action": "retry_same_request", "message": "private"}})
				}))
				defer server.Close()
				p := NewOIDCLoginProvider("ant", server.URL, nil)
				ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
				defer cancel()
				var data *TokenData
				var err error
				if operation == "transactions" {
					data, err = p.Login(ctx)
				} else {
					data, err = p.exchange(ctx, "code", oidcTestState)
				}
				if err == nil || data != nil || calls.Load() != 1 || strings.Contains(err.Error(), "private") || errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("request must fail immediately without retry or leaked details: calls=%d err=%v", calls.Load(), err)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageOIDCAuthorizationRequestBoundaries(t *testing.T) {
	for _, mutation := range []string{"wrong-provider", "bad-lifetime", "redirect", "state", "nonce", "pkce", "scope", "unknown-param", "http-external", "userinfo", "fragment", "parameter-collision", "credentials-query"} {
		t.Run(mutation, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]string
				json.NewDecoder(r.Body).Decode(&request)
				body := oidcTestCreated(server.URL+"/cli/auth/oidc/mock/sdk/authorize", "ant-sdk://oidc/callback")
				auth := body["authorization_request"].(map[string]any)
				params := auth["parameters"].(map[string]string)
				switch mutation {
				case "wrong-provider":
					body["provider"] = "other"
				case "bad-lifetime":
					body["expires_in"] = 0
				case "redirect":
					params["redirect_uri"] = "not-an-absolute-uri"
				case "state":
					params["state"] = "short"
				case "nonce":
					delete(params, "nonce")
				case "pkce":
					params["code_challenge_method"] = "plain"
				case "scope":
					params["scope"] = "profile"
				case "unknown-param":
					params["client_secret"] = "private"
				case "http-external":
					auth["authorization_endpoint"] = "http://example.com/authorize"
				case "userinfo":
					auth["authorization_endpoint"] = "https://user:private@example.com"
				case "fragment":
					auth["authorization_endpoint"] = "https://example.com/#private"
				case "parameter-collision":
					auth["authorization_endpoint"] = server.URL + "/cli/auth/oidc/mock/sdk/authorize?state=private"
				case "credentials-query":
					auth["authorization_endpoint"] = server.URL + "/cli/auth/oidc/mock/sdk/authorize?access_token=private"
				}
				w.WriteHeader(201)
				json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			p := NewOIDCLoginProvider("ant", server.URL, nil)
			p.authorizer = oidcAuthorizeFunc(func(context.Context, OIDCAuthorizationRequest) (OIDCAuthorizationResult, error) {
				t.Error("started SDK on malformed authorization response")
				return OIDCAuthorizationResult{}, nil
			})
			_, err := p.Login(t.Context())
			if err == nil || strings.Contains(err.Error(), "private") {
				t.Fatalf("unsafe authorization response: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCProtocolMalformedResponses(t *testing.T) {
	for _, body := range []string{`{"accessToken":"a","accessToken":"b"}`, `[]`, `{"accessToken":`, strings.Repeat("private", 12000)} {
		p := oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })
		if _, err := p.Login(t.Context()); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("malformed response accepted/leaked: %v", err)
		}
	}
	p := oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		body := oidcTestCompleted()
		body["expiresIn"] = "invalid"
		json.NewEncoder(w).Encode(body)
	})
	if _, err := p.Login(t.Context()); err == nil {
		t.Fatal("accepted invalid token expiry")
	}
	p = oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]string{})
	})
	if _, err := p.Login(t.Context()); err == nil {
		t.Fatal("retried non-retryable response")
	}
}

func TestCrossPlatformCoverageOIDCPublicLoginPersistence(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	p := oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(oidcTestCompleted()) })
	if _, err := p.LoginAndSave(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
	data, err := LoadTokenDataForProfile(dir, "corp")
	if err != nil || data.RefreshToken != "secret-refresh" {
		t.Fatalf("OIDC token not persisted: %v", err)
	}
	for _, provider := range []string{"", "Bad/name", strings.Repeat("a", 65)} {
		if _, err := NewOIDCLoginProvider(provider, p.baseURL, nil).LoginAndSave(t.Context(), dir); err == nil {
			t.Fatal("malformed provider accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.LoginAndSave(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation identity lost")
	}
	if _, err := NewAntSDKOIDCProvider("http://example.com").LoginAndSave(t.Context(), dir); err == nil {
		t.Fatal("insecure portal accepted")
	}
	if err := os.WriteFile(ProfilesPath(dir), []byte(`{"version":999,"profiles":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.LoginAndSave(t.Context(), dir); err == nil {
		t.Fatal("unsupported storage accepted")
	}
}

func TestCrossPlatformCoverageOIDCUsesOAuthIdentityAndPersistence(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	p := oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(oidcTestCompleted()) })
	p.IdentityEnricher = func(_ context.Context, data *TokenData) error {
		if data.AccessToken != "secret-access" || data.CorpName != "测试组织" || data.UserID != "" {
			t.Fatal("identity enrichment did not receive the getToken response")
		}
		data.UserID, data.UserName = "resolved-user", "测试用户"
		return nil
	}
	if _, err := p.LoginAndSave(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
	data, err := LoadTokenDataForProfile(dir, "corp:resolved-user")
	if err != nil || data.UserName != "测试用户" || data.CorpName != "测试组织" || data.RefreshToken != "secret-refresh" {
		t.Fatalf("normal login writer did not save enriched token/profile: %v", err)
	}
	p.IdentityEnricher = func(context.Context, *TokenData) error { return errors.New("private-identity-error") }
	if _, err := p.LoginAndSave(t.Context(), dir); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatalf("identity failure was ignored or leaked: %v", err)
	}
	profiles, err := LoadProfiles(dir)
	if err != nil || len(profiles.Profiles) != 1 || profiles.CurrentProfile != "corp:resolved-user" {
		t.Fatal("failed identity enrichment changed the saved profile")
	}
}

func TestCrossPlatformCoverageOIDCTokenResponseMatchesMCPParser(t *testing.T) {
	for _, body := range []string{
		`{"accessToken":"encrypted-access"}`,
		`{"accessToken":"encrypted-access","refreshToken":"encrypted-refresh","expiresIn":7200,"corpId":"corp","corpName":"Org"}`,
	} {
		p := oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })
		data, err := p.Login(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		expected, err := (&OAuthProvider{}).parseMCPTokenResponse([]byte(body))
		if err != nil || data.AccessToken != expected.AccessToken || data.RefreshToken != expected.RefreshToken || data.CorpID != expected.CorpID || data.CorpName != expected.CorpName {
			t.Fatal("OIDC response differs from normal MCP token parsing")
		}
		if delta := expected.ExpiresAt.Sub(data.ExpiresAt); delta < 0 || delta > time.Second {
			t.Fatal("OIDC token expiry differs from normal MCP token parsing")
		}
		if delta := expected.RefreshExpAt.Sub(data.RefreshExpAt); delta < 0 || delta > time.Second {
			t.Fatal("OIDC refresh expiry differs from normal MCP token parsing")
		}
	}
}

func TestCrossPlatformCoverageOIDCDoesNotExtendMCPTokenResponseFormat(t *testing.T) {
	const body = `{"access_token":"secret-access","refresh_token":"secret-refresh","expires_in":7200,"corp_id":"corp"}`
	p := oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })
	data, err := p.Login(t.Context())
	if err == nil || data != nil {
		t.Fatal("OIDC login accepted a token response outside the existing MCP contract")
	}
	if strings.Contains(err.Error(), "secret-") {
		t.Fatal("invalid token response leaked credentials")
	}
}

type oidcPortalTransportFunc func(*http.Request) (*http.Response, error)

func (f oidcPortalTransportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCrossPlatformCoverageOIDCRemotePortalSDKFlow(t *testing.T) {
	const base = "https://portal.example"
	for _, endpoint := range []string{base + "/cli/auth/oidc/mock/sdk/authorize", "https://other.example/cli/auth/oidc/mock/sdk/authorize"} {
		t.Run(endpoint, func(t *testing.T) {
			var calls []string
			provider := NewAntSDKOIDCProvider(base)
			provider.client = &http.Client{Transport: oidcPortalTransportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Scheme != "https" || r.URL.Host != "portal.example" {
					t.Fatal("SDK credential sent outside selected HTTPS portal")
				}
				calls = append(calls, r.URL.Path)
				status := http.StatusOK
				var body any
				switch r.URL.Path {
				case "/cli/auth/oidc/transactions":
					status = http.StatusCreated
					body = oidcTestCreated(endpoint, "ant-sdk://oidc/callback")
				case "/cli/auth/oidc/mock/sdk/login":
					body = map[string]any{"sdk_session_token": "sdk-secret", "expires_in": 60}
				case "/cli/auth/oidc/mock/sdk/authorize":
					if r.Header.Get("Authorization") != "Bearer sdk-secret" {
						t.Fatal("missing SDK session")
					}
					body = map[string]string{"code": "code", "state": oidcTestState}
				case "/cli/auth/oidc/exchange":
					body = oidcTestCompleted()
				default:
					t.Fatal("unexpected portal endpoint")
				}
				encoded, err := json.Marshal(body)
				if err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(encoded)))}, nil
			})}
			data, err := provider.Login(t.Context())
			if strings.HasPrefix(endpoint, base+"/") {
				if err != nil || data == nil {
					t.Fatalf("remote portal login failed: %v", err)
				}
				if len(calls) != 4 {
					t.Fatalf("incomplete portal login: %v", calls)
				}
			} else if err == nil || len(calls) != 1 {
				t.Fatal("foreign SDK endpoint must stop before sending SDK credentials")
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCProviderExchangeCredentials(t *testing.T) {
	t.Setenv(EnvClientID, "environment-app")
	t.Setenv(EnvClientSecret, "environment-secret")
	for _, credentials := range []*OIDCExchangeCredentials{nil, {ClientID: "dws-app", ClientSecret: " secret-exact "}} {
		p := oidcTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if body["code"] != " original-code " || body["state"] != oidcTestState {
				t.Error("code/state changed")
			}
			if credentials == nil {
				if len(body) != 2 {
					t.Error("absent credentials were included")
				}
			} else if len(body) != 4 || body["client_id"] != credentials.ClientID || body["client_secret"] != credentials.ClientSecret {
				t.Error("credentials were not sent exactly")
			}
			json.NewEncoder(w).Encode(oidcTestCompleted())
		})
		p.ExchangeCredentials = credentials
		if _, err := p.Login(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	for _, credentials := range []*OIDCExchangeCredentials{{}, {ClientID: "app"}, {ClientSecret: "private"}, {ClientID: " ", ClientSecret: "private"}, {ClientID: "app", ClientSecret: " "}} {
		p := NewOIDCLoginProvider("ant", "https://portal.invalid", nil)
		p.ExchangeCredentials = credentials
		_, err := p.Login(t.Context())
		if err == nil || dwserrors.ExitCode(err) != 3 || strings.Contains(err.Error(), "private") {
			t.Fatalf("invalid pair must fail before network: %v", err)
		}
	}
}
