package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dwserrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageOIDCPortalDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		code, action, message string
		status                int
	}{
		{"invalid_request", "stop", "invalid", 400},
		{"invalid_state", "restart_login", "state", 400},
		{"unsupported_provider", "stop", "provider", 400},
		{"provider_mismatch", "restart_login", "provider", 400},
		{"oidc_code_invalid", "restart_login", "authorization code", 400},
		{"oidc_identity_invalid", "restart_login", "identity", 403},
		{"rate_limited", "restart_login", "rate", 429},
		{"upstream_exchange_uncertain", "restart_login", "uncertain", 502},
		{"session_issue_failed", "restart_login", "session", 502},
		{"session_result_invalid", "restart_login", "session", 502},
		{"temporarily_unavailable", "stop", "unavailable", 503},
		{"provider_unavailable", "stop", "provider", 503},
		{"oidc_configuration_invalid", "stop", "configuration", 500},
		{"internal_error", "stop", "internal", 500},
		{"enterprise_access_denied", "stop", "rejected", 403},
	} {
		for _, operation := range []string{"prepare", "exchange"} {
			t.Run(tc.code+"/"+operation, func(t *testing.T) {
				calls := 0
				serverMessage := "OIDC login request was not completed"
				if tc.status >= 500 {
					serverMessage = "OIDC login service is unavailable"
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.Header().Set("X-Dws-Trace-Id", "header-trace")
					w.WriteHeader(tc.status)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"error":    map[string]string{"code": tc.code, "message": serverMessage, "action": tc.action},
						"trace_id": "body-trace-123", "reason": "private-reason-canary",
					})
				}))
				defer server.Close()
				p := NewOIDCLoginProvider("ant", server.URL, nil)
				var err error
				if operation == "prepare" {
					_, err = p.PrepareExchange(t.Context())
				} else {
					_, err = p.Exchange(t.Context(), "code-canary", oidcTestState)
				}
				var typed *dwserrors.Error
				if !errors.As(err, &typed) || dwserrors.ExitCode(err) != 2 || calls != 1 {
					t.Fatalf("classification or request count: calls=%d err=%v", calls, err)
				}
				if typed.ServerDiag.TraceID != "body-trace-123" || typed.ServerDiag.ServerErrorCode != tc.code || typed.ServerDiag.TechnicalDetail != serverMessage {
					t.Fatalf("lost backend diagnostics: %+v", typed.ServerDiag)
				}
				if !strings.Contains(strings.ToLower(err.Error()), tc.message) || typed.Details["action"] != tc.action || typed.Details["http_status"] != tc.status || typed.Retryable || !typed.RetryableSet {
					t.Fatalf("missing message/action/status or unsafe retry: %+v", typed)
				}
				if tc.action == "stop" && (!strings.Contains(typed.Hint, "administrator") || strings.Contains(typed.Hint, "exchange-prepare")) {
					t.Fatalf("stop instruction overwritten: %s", typed.Hint)
				}
				if operation == "exchange" && tc.action == "restart_login" && !strings.Contains(typed.Hint, "exchange-prepare") {
					t.Fatalf("split exchange lost recovery: %s", typed.Hint)
				}
				var human, machine bytes.Buffer
				if err := dwserrors.PrintHuman(&human, err); err != nil {
					t.Fatal(err)
				}
				if err := dwserrors.PrintJSON(&machine, err); err != nil {
					t.Fatal(err)
				}
				for _, rendered := range []string{human.String(), machine.String()} {
					if !strings.Contains(rendered, tc.code) || !strings.Contains(rendered, "body-trace-123") || strings.Contains(rendered, "canary") {
						t.Fatalf("missing diagnostics or unsafe output: %s", rendered)
					}
				}
			})
		}
	}
}

func TestCrossPlatformCoverageOIDCTraceFallbackAndSafeDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, body, header, trace, code string
	}{
		{"header", `{"error":{"code":"oidc_identity_invalid","action":"restart_login","message":"private-message-canary"}}`, "header-trace", "header-trace", "oidc_identity_invalid"},
		{"bad-trace", `{"trace_id":"bad\u001btrace","error":{"code":"invalid_state"}}`, "header-trace", "header-trace", "invalid_state"},
		{"bad-code-action", `{"trace_id":"body-trace","error":{"code":"bad\ncode","action":"retry_same_request"}}`, "", "body-trace", ""},
		{"long-trace", `{"trace_id":"` + strings.Repeat("t", 257) + `","error":{"code":"invalid_state"}}`, "header-trace", "header-trace", "invalid_state"},
		{"non-json", `<html>private-response-canary</html>`, "header-trace", "header-trace", ""},
		{"duplicate", `{"trace_id":"first","trace_id":"second","error":{"code":"invalid_state"}}`, "header-trace", "header-trace", ""},
		{"wrong-types", `{"trace_id":123,"error":"private-error-canary"}`, "header-trace", "header-trace", ""},
		{"oversize", strings.Repeat("x", 65537), "header-trace", "header-trace", ""},
		{"no-trace", `{"error":{"code":"internal_error"}}`, "", "", "internal_error"},
		{"unsafe-header", `{}`, "bad trace", "", ""},
		{"reflected-code", `{"trace_id":"prefix-code-canary","error":{"code":"code-canary"}}`, "header-trace", "header-trace", ""},
		{"reflected-state", `{"trace_id":"` + oidcTestState + `","error":{"code":"invalid_state"}}`, "", "", "invalid_state"},
		{"reflected-secret", `{"error":{"code":"secret-canary"}}`, "secret-canary", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Dws-Trace-Id", tc.header)
				w.WriteHeader(502)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			provider := NewOIDCLoginProvider("ant", server.URL, nil)
			provider.ExchangeCredentials = &OIDCExchangeCredentials{ClientID: "dws-app", ClientSecret: "secret-canary"}
			_, err := provider.Exchange(t.Context(), "code-canary", oidcTestState)
			var typed *dwserrors.Error
			if !errors.As(err, &typed) || typed.ServerDiag.TraceID != tc.trace || typed.ServerDiag.ServerErrorCode != tc.code {
				t.Fatalf("unexpected diagnostics: %+v", typed)
			}
			var rendered bytes.Buffer
			if err := dwserrors.PrintJSON(&rendered, err); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(rendered.String(), "canary") || strings.Contains(rendered.String(), "retry_same_request") || strings.Contains(fmt.Sprint(typed.Details), "bad\n") {
				t.Fatal("unreviewed response data escaped")
			}
		})
	}
}

type oidcDiagnosticReadFailure struct{ err error }

func (r oidcDiagnosticReadFailure) Read([]byte) (int, error) { return 0, r.err }

func TestCrossPlatformCoverageOIDCTraceOnReadFailure(t *testing.T) {
	for _, failure := range []error{io.ErrUnexpectedEOF, context.Canceled, context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			p := NewOIDCLoginProvider("ant", "https://portal.example", nil)
			p.client = &http.Client{Transport: oidcPortalTransportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 502, Header: http.Header{"X-Dws-Trace-Id": {"read-trace"}}, Body: io.NopCloser(oidcDiagnosticReadFailure{failure})}, nil
			})}
			_, err := p.PrepareExchange(t.Context())
			if failure == context.Canceled {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation changed: %v", err)
				}
				return
			}
			var typed *dwserrors.Error
			if !errors.As(err, &typed) || typed.Category != dwserrors.CategoryAPI || typed.ServerDiag.TraceID != "read-trace" {
				t.Fatalf("network classification/trace lost: %+v", typed)
			}
			if failure == context.DeadlineExceeded && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("deadline identity lost: %v", err)
			}
		})
	}
}
