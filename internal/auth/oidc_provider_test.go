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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
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

func oidcTestJSONResponse(t *testing.T, status int, body any) *http.Response {
	t.Helper()
	content, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(content)))}
}

func TestCrossPlatformCoverageOIDCLoginEntryGuards(t *testing.T) {
	for _, entry := range []string{"login", "login-and-save"} {
		for _, scenario := range []string{"invalid-provider", "pre-canceled", "preflight-failure"} {
			if entry == "login" && scenario == "preflight-failure" {
				continue
			}
			t.Run(entry+"/"+scenario, func(t *testing.T) {
				dir := externalExchangeTestConfig(t)
				preflights, requests := 0, 0
				testseam.Swap(t, &profilesLoad, func(gotDir string) (*ProfilesConfig, error) {
					preflights++
					if gotDir != dir {
						t.Error("preflight used another config directory")
					}
					return nil, errors.New("private-preflight-canary")
				})
				p := NewOIDCLoginProvider("ant", "https://portal.example", oidcAuthorizeFunc(func(context.Context, OIDCAuthorizationRequest) (OIDCAuthorizationResult, error) {
					t.Error("entry guard invoked the authorizer")
					return OIDCAuthorizationResult{}, nil
				}))
				p.client = &http.Client{Transport: oidcPortalTransportFunc(func(*http.Request) (*http.Response, error) {
					requests++
					return nil, errors.New("unexpected portal request")
				})}
				ctx, cancel := context.WithCancel(t.Context())
				t.Cleanup(cancel)
				if scenario == "invalid-provider" {
					p.provider = "invalid/provider"
				} else if scenario == "pre-canceled" {
					cancel()
				}
				var data *TokenData
				var err error
				if entry == "login" {
					data, err = p.Login(ctx)
				} else {
					data, err = p.LoginAndSave(ctx, dir)
				}
				if data != nil || err == nil || requests != 0 || strings.Contains(err.Error(), "private-preflight-canary") {
					t.Fatalf("entry guard failed: requests=%d error=%v", requests, err)
				}
				wantPreflights := 0
				if scenario == "preflight-failure" {
					wantPreflights = 1
				}
				if preflights != wantPreflights {
					t.Fatalf("preflights=%d, want %d", preflights, wantPreflights)
				}
				if scenario == "pre-canceled" {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("cancellation identity lost: %v", err)
					}
				} else {
					want := dwserrors.CategoryValidation
					if scenario == "preflight-failure" {
						want = dwserrors.CategoryAuth
					}
					var typed *dwserrors.Error
					if !errors.As(err, &typed) || typed.Category != want {
						t.Fatalf("wrong guard classification: %v", err)
					}
				}
				cfg, err := LoadProfiles(dir)
				if err != nil || len(cfg.Profiles) != 0 {
					t.Fatalf("entry guard changed profiles: %v", err)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageOIDCPreparePersistenceCredentialGuards(t *testing.T) {
	for _, scenario := range []string{"edition-hook", "canonical-read-failure", "legacy-read-failure"} {
		t.Run(scenario, func(t *testing.T) {
			dir := externalExchangeTestConfig(t)
			previousEdition := edition.Get()
			t.Cleanup(func() { edition.Override(previousEdition) })
			hooks := &edition.Hooks{}
			if scenario == "edition-hook" {
				hooks.SaveToken = func(string, []byte) error {
					t.Error("unsupported edition token hook was invoked")
					return nil
				}
			}
			edition.Override(hooks)
			reads := 0
			testseam.Swap(t, &authKeychainGet, func(service, account string) (string, error) {
				reads++
				wantAccount := secretAccountKey("app")
				if reads == 2 {
					wantAccount = legacyClientSecretAccountKey("app")
				}
				if service != keychain.Service || account != wantAccount {
					t.Error("snapshot read an unrelated credential slot")
				}
				if scenario == "canonical-read-failure" || reads == 2 {
					return "", errors.New("private-secret-read-canary")
				}
				return "previous-secret", nil
			})
			testseam.Swap(t, &profilesLoad, func(string) (*ProfilesConfig, error) {
				t.Error("credential guard reached profile preflight")
				return nil, errors.New("unexpected preflight")
			})
			p := NewOIDCLoginProvider("ant", "https://portal.example", nil)
			p.ExchangeCredentials = &OIDCExchangeCredentials{ClientID: "app", ClientSecret: "new-secret"}
			err := p.preparePersistence(dir)
			wantReads := 0
			if scenario == "canonical-read-failure" {
				wantReads = 1
			} else if scenario == "legacy-read-failure" {
				wantReads = 2
			}
			if err == nil || reads != wantReads || strings.Contains(err.Error(), "canary") {
				t.Fatalf("credential guard failed: reads=%d error=%v", reads, err)
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCSDKFailureBoundaries(t *testing.T) {
	for _, scenario := range []string{"canceled-after-authorize", "unavailable", "network-unreachable"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			requests, authorizations := 0, 0
			p := NewOIDCLoginProvider("ant", "https://portal.example", oidcAuthorizeFunc(func(sdkCtx context.Context, request OIDCAuthorizationRequest) (OIDCAuthorizationResult, error) {
				authorizations++
				if sdkCtx.Err() != nil || request.Parameters["state"] != oidcTestState {
					t.Error("authorizer received a canceled context or changed state")
				}
				if _, ok := sdkCtx.Deadline(); !ok {
					t.Error("authorizer has no transaction deadline")
				}
				switch scenario {
				case "canceled-after-authorize":
					cancel()
					return OIDCAuthorizationResult{Code: "code", State: oidcTestState}, nil
				case "unavailable":
					return OIDCAuthorizationResult{}, fmt.Errorf("private-sdk-canary: %w", errOIDCAuthorizerUnavailable)
				default:
					return OIDCAuthorizationResult{}, fmt.Errorf("private-sdk-canary: %w", dwserrors.NewAPI("private-network-canary", dwserrors.WithReason("NETWORK_UNREACHABLE"), dwserrors.WithCause(errors.New("private-cause-canary"))))
				}
			}))
			p.client = &http.Client{Transport: oidcPortalTransportFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				var body map[string]string
				if r.Method != http.MethodPost || r.URL.String() != "https://portal.example/cli/auth/oidc/transactions" || r.Header.Get("Authorization") != "" || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["provider"] != "ant" {
					t.Error("SDK failure performed an unexpected portal operation")
				}
				return oidcTestJSONResponse(t, http.StatusCreated, oidcTestCreated("https://idp.example/authorize", "ant-sdk://oidc/callback")), nil
			})}
			data, err := p.Login(ctx)
			if data != nil || err == nil || requests != 1 || authorizations != 1 {
				t.Fatalf("SDK failure reached exchange: requests=%d authorizations=%d error=%v", requests, authorizations, err)
			}
			if scenario == "canceled-after-authorize" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("SDK cancellation lost identity: %v", err)
				}
			} else {
				var typed *dwserrors.Error
				wantCategory, wantReason := dwserrors.CategoryAuth, "OIDC_SDK_UNAVAILABLE"
				if scenario == "network-unreachable" {
					wantCategory, wantReason = dwserrors.CategoryAPI, "NETWORK_UNREACHABLE"
				}
				if !errors.As(err, &typed) || typed.Category != wantCategory || typed.Reason != wantReason {
					t.Fatalf("SDK error classification changed: %v", err)
				}
			}
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				if strings.Contains(cause.Error(), "canary") {
					t.Fatal("foreign SDK diagnostics escaped through the error chain")
				}
			}
		})
	}
}

type oidcCancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b oidcCancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func TestCrossPlatformCoverageOIDCLoginAndSaveCancellationAfterExchange(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	previousProfile := RuntimeProfile()
	SetRuntimeProfile("")
	t.Cleanup(func() { SetRuntimeProfile(previousProfile) })
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	requests, saves := 0, 0
	testseam.Swap(t, &oauthSaveToken, func(string, *TokenData) error { saves++; return nil })
	p := NewOIDCLoginProvider("ant", "https://portal.example", oidcAuthorizeFunc(func(_ context.Context, request OIDCAuthorizationRequest) (OIDCAuthorizationResult, error) {
		return OIDCAuthorizationResult{Code: "code", State: request.Parameters["state"]}, nil
	}))
	p.IdentityEnricher = func(context.Context, *TokenData) error {
		t.Error("canceled login reached identity enrichment")
		return nil
	}
	p.client = &http.Client{Transport: oidcPortalTransportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		var body map[string]string
		if r.Method != http.MethodPost || r.URL.Host != "portal.example" || r.Header.Get("Authorization") != "" || json.NewDecoder(r.Body).Decode(&body) != nil || body["provider"] != "ant" {
			t.Error("invalid login request")
		}
		if requests == 1 {
			if r.URL.Path != "/cli/auth/oidc/transactions" || len(body) != 1 {
				t.Error("login did not prepare first")
			}
			return oidcTestJSONResponse(t, http.StatusCreated, oidcTestCreated("https://idp.example/authorize", "ant-sdk://oidc/callback")), nil
		}
		if requests != 2 || r.URL.Path != "/cli/auth/oidc/exchange" || len(body) != 3 || body["code"] != "code" || body["state"] != oidcTestState {
			t.Error("login exchanged unexpected credentials or retried")
		}
		response := oidcTestJSONResponse(t, http.StatusOK, oidcTestCompleted())
		// Deliver a complete token, then cancel when its response is released.
		// This exercises cancellation between successful login and persistence.
		response.Body = oidcCancelOnCloseBody{ReadCloser: response.Body, cancel: cancel}
		return response, nil
	})}
	data, err := p.LoginAndSave(ctx, dir)
	if data != nil || !errors.Is(err, context.Canceled) || requests != 2 || saves != 0 {
		t.Fatalf("canceled completed login persisted: requests=%d saves=%d error=%v", requests, saves, err)
	}
	cfg, err := LoadProfiles(dir)
	if err != nil || len(cfg.Profiles) != 0 {
		t.Fatalf("canceled completed login published a profile: %v", err)
	}
	if _, err := LoadTokenDataKeychain(); !errors.Is(err, ErrTokenDataNotFound) {
		t.Fatalf("canceled completed login saved token material: %v", err)
	}
}

func TestCrossPlatformCoverageOIDCAuthorizationRequestURLParsing(t *testing.T) {
	for _, scenario := range []string{"invalid-base", "malformed-query"} {
		t.Run(scenario, func(t *testing.T) {
			p := NewOIDCLoginProvider("ant", "https://portal.example", nil)
			created := oidcTestCreated("https://idp.example/authorize", "ant-sdk://oidc/callback")
			if scenario == "invalid-base" {
				p.baseURL = "http://portal.example"
			} else {
				created["authorization_request"].(map[string]any)["authorization_endpoint"] = "https://idp.example/authorize?tenant=%zz"
			}
			content, err := json.Marshal(created)
			if err != nil {
				t.Fatal(err)
			}
			var tx OIDCExchangePreparation
			if err := json.Unmarshal(content, &tx); err != nil {
				t.Fatal(err)
			}
			request, err := p.authorizationRequest(tx)
			var typed *dwserrors.Error
			if !errors.As(err, &typed) || typed.Category != dwserrors.CategoryAuth || request.AuthorizationEndpoint != "" || request.Parameters != nil {
				t.Fatalf("unsafe authorization URL accepted: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCPostRejectsResponseTypeMismatch(t *testing.T) {
	requests := 0
	p := NewOIDCLoginProvider("ant", "https://portal.example", nil)
	p.client = &http.Client{Transport: oidcPortalTransportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		var body map[string]string
		if r.Method != http.MethodPost || r.URL.Path != "/cli/auth/oidc/transactions" || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["provider"] != "ant" {
			t.Error("unexpected preparation request")
		}
		response := oidcTestJSONResponse(t, http.StatusCreated, map[string]any{"provider": "ant", "expires_in": "private-type-canary"})
		response.Header.Set("X-Dws-Trace-Id", "type-mismatch-trace")
		return response, nil
	})}
	tx, err := p.PrepareExchange(t.Context())
	var typed *dwserrors.Error
	if tx != nil || requests != 1 || !errors.As(err, &typed) || typed.Category != dwserrors.CategoryAuth || typed.ServerDiag.TraceID != "type-mismatch-trace" || strings.Contains(err.Error(), "private-type-canary") {
		t.Fatalf("response type mismatch lost safe classification or trace: %v", err)
	}
}

func TestCrossPlatformCoverageOIDCJSONRequestConstruction(t *testing.T) {
	for _, scenario := range []string{"pre-canceled", "unmarshalable-body", "invalid-endpoint", "bearer"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			endpoint, bearer := "https://portal.example/protected", ""
			var body any = map[string]string{"provider": "ant"}
			switch scenario {
			case "pre-canceled":
				cancel()
			case "unmarshalable-body":
				body = func() {}
			case "invalid-endpoint":
				endpoint = "https://portal.example/%zz"
			case "bearer":
				bearer = "private-bearer-canary"
			}
			requests := 0
			client := &http.Client{Transport: oidcPortalTransportFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				var received map[string]string
				if scenario != "bearer" || r.Context().Err() != nil || r.Method != http.MethodPost || r.URL.String() != endpoint || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" || r.Header.Get("Authorization") != "Bearer "+bearer || json.NewDecoder(r.Body).Decode(&received) != nil || len(received) != 1 || received["provider"] != "ant" {
					t.Error("request construction changed headers, endpoint or JSON body")
				}
				response := oidcTestJSONResponse(t, http.StatusOK, map[string]bool{"ok": true})
				response.Header.Set("X-Dws-Trace-Id", "request-trace")
				return response, nil
			})}
			status, content, trace, err := oidcJSONRequestWithTrace(ctx, client, endpoint, body, bearer)
			if scenario == "bearer" {
				if err != nil || status != http.StatusOK || string(content) != `{"ok":true}` || trace != "request-trace" || requests != 1 {
					t.Fatalf("bearer request failed: status=%d trace=%q requests=%d error=%v", status, trace, requests, err)
				}
				return
			}
			if err == nil || status != 0 || content != nil || trace != "" || requests != 0 {
				t.Fatalf("invalid request reached transport: status=%d requests=%d error=%v", status, requests, err)
			}
			if scenario == "pre-canceled" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("pre-cancellation lost identity: %v", err)
				}
			} else {
				want := dwserrors.CategoryValidation
				if scenario == "unmarshalable-body" {
					want = dwserrors.CategoryInternal
				}
				var typed *dwserrors.Error
				if !errors.As(err, &typed) || typed.Category != want {
					t.Fatalf("request construction error misclassified: %v", err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageOIDCSecurityValidatorBoundaries(t *testing.T) {
	t.Run("random-value", func(t *testing.T) {
		for _, value := range []string{"", strings.Repeat("a", 42), strings.Repeat("a", 44), strings.Repeat("a", 42) + "+", strings.Repeat("a", 42) + "/", strings.Repeat("a", 42) + "=", strings.Repeat("a", 42) + "\n"} {
			if oidcRandomValue(value) {
				t.Errorf("invalid random value accepted: %q", value)
			}
		}
		if !oidcRandomValue(strings.Repeat("a", 37) + "AZ09_-") {
			t.Fatal("valid base64url alphabet rejected")
		}
	})
	t.Run("log-endpoint", func(t *testing.T) {
		for _, endpoint := range []string{"https://portal.example/%zz", "/authorize", ""} {
			if got := oidcLogEndpoint(endpoint); got != endpoint {
				t.Errorf("unparseable or hostless endpoint changed: got %q, want %q", got, endpoint)
			}
		}
		if got := oidcLogEndpoint("https://portal.example/authorize?state=private-state#private-fragment"); got != "https://portal.example/authorize" {
			t.Fatalf("endpoint diagnostics retained authorization parameters: %q", got)
		}
	})
	for _, tc := range []struct {
		name, content string
		valid         bool
	}{
		{"nested-duplicate-object", `{"outer":{"token":1,"token":2}}`, false},
		{"nested-invalid-object-value", `{"outer":{"token":}}`, false},
		{"nested-invalid-object-close", `{"outer":{"token":1]}}`, false},
		{"duplicate-object-in-array", `[{"token":1,"token":2}]`, false},
		{"invalid-array-value", `[1,{"token":}]`, false},
		{"unterminated-array", `[1`, false},
		{"empty-array", `[]`, true},
		{"valid-nested-array", `[null,true,123456789012345678901234567890,["value"],{"token":1},{"token":2}]`, true},
	} {
		t.Run("unique-json/"+tc.name, func(t *testing.T) {
			if got := oidcUniqueJSON([]byte(tc.content)); got != tc.valid {
				t.Fatalf("unique JSON validation=%t, want %t", got, tc.valid)
			}
		})
	}
}
