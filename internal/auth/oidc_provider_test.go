package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dwserrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const oidcTestRandom = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const oidcTestState = oidcTestRandom + ".1792023000." + oidcTestRandom

func oidcTestCompleted() map[string]any {
	return map[string]any{"accessToken": "secret-access", "refreshToken": "secret-refresh", "expiresIn": 3600, "corpId": "corp", "corpName": "测试组织", "clientId": "dws-app"}
}

func oidcTestCreated(endpoint, redirect string) map[string]any {
	return map[string]any{"provider": "ant", "expires_in": 600, "authorization_request": map[string]any{"authorization_endpoint": endpoint, "parameters": map[string]string{"client_id": "registered-app", "response_type": "code", "scope": "openid", "redirect_uri": redirect, "state": oidcTestState, "nonce": oidcTestRandom, "code_challenge": oidcTestRandom, "code_challenge_method": "S256"}}}
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
			if !stage.CompareAndSwap(2, 3) || body["provider"] != provider || body["code"] != "enterprise-code" || body["state"] != oidcTestState || len(body) != 3 {
				t.Error("common exchange lost the selected provider or business authorization result")
			}
			completed := oidcTestCompleted()
			json.NewEncoder(w).Encode(completed)
		default:
			t.Error("common flow called an unexpected endpoint")
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
				body := oidcTestCreated(server.URL+"/authorize", "ant-sdk://oidc/callback")
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
					params["state"] = strings.Repeat("a", 1025)
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
					auth["authorization_endpoint"] = server.URL + "/authorize?state=private"
				case "credentials-query":
					auth["authorization_endpoint"] = server.URL + "/authorize?access_token=private"
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

type oidcPortalTransportFunc func(*http.Request) (*http.Response, error)

func (f oidcPortalTransportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
