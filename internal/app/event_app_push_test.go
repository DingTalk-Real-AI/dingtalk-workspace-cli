package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageAppEventPushEnvironment(t *testing.T) {
	for _, tc := range []struct {
		baseURL string
		want    string
	}{
		{"https://mcp.dingtalk.com", "https://api.dingtalk.com/v1.0/event/user/push"},
		{"https://pre-mcp.dingtalk.com/", "https://pre-api.dingtalk.com/v1.0/event/user/push"},
		{"https://untrusted.example", ""},
	} {
		t.Run(tc.baseURL, func(t *testing.T) {
			configDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(tc.baseURL), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := appEventPushURL(configDir)
			if tc.want == "" {
				if err == nil || apperrors.ExitCode(err) != 3 {
					t.Fatalf("unknown environment must fail validation: %q, %v", got, err)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("endpoint = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageAppEventPushRequestAndReceipts(t *testing.T) {
	for _, test := range []struct {
		name, response string
		duplicate      bool
	}{
		{"accepted", `{"eventId":"event-1","status":"ACCEPTED","duplicate":false}`, false},
		{"duplicate", `{"eventId":"event-1","status":"DUPLICATE","duplicate":true}`, true},
		{"wrapped", `{"success":true,"result":{"eventId":"event-1","status":"ACCEPTED"}}`, false},
		{"legacy duplicate", `{"eventId":"event-1","status":"DUPLICATE"}`, true},
		{"success code", `{"code":200,"eventId":"event-1","status":"ACCEPTED"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.Header.Get("x-acs-dingtalk-access-token") != "app-token" {
					t.Error("expected POST with application token")
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				var request appEventPushRequest
				if err := json.Unmarshal(body, &request); err != nil {
					t.Fatal(err)
				}
				if request.EventID != "event-1" || request.PayloadSchemaVersion != "3" || len(request.UserIDs) != 2 || request.OccurredAt != "2026-11-12T10:00:00+08:00" {
					t.Errorf("request = %+v", request)
				}
				if !strings.Contains(string(request.Payload), "9007199254740993") {
					t.Errorf("payload precision lost: %s", request.Payload)
				}
				_, _ = io.WriteString(w, test.response)
			}))
			defer server.Close()
			result, err := pushAppEvent(t.Context(), server.URL, "app-token", appEventPushRequest{
				EventKey: "app_user_order_created", EventID: "event-1", PayloadSchemaVersion: "3",
				UserIDs: []string{"user-1", "user-2"}, Payload: json.RawMessage(`{"id":9007199254740993}`),
				OccurredAt: "2026-11-12T10:00:00+08:00",
			})
			if err != nil || result.EventID != "event-1" || result.Duplicate != test.duplicate || calls != 1 {
				t.Fatalf("result=%+v error=%v calls=%d", result, err, calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAppEventPushFailures(t *testing.T) {
	for _, test := range []struct {
		name, body, reason, code string
		status                   int
	}{
		{"schema rejection", `{"code":"460002","message":"bad payload app-token","requestId":"trace-1","retryable":false}`, "upstream_error", "460002", 400},
		{"numeric code", `{"code":460006,"message":"event ID conflict"}`, "upstream_error", "460006", 200},
		{"HSF error", `{"success":false,"errorCode":"EVENT_NOT_AVAILABLE","errorMsg":"offline"}`, "upstream_error", "EVENT_NOT_AVAILABLE", 200},
		{"legacy error", `{"errcode":460004,"errmsg":"wrong app"}`, "upstream_error", "460004", 403},
		{"failed envelope", `{"success":false}`, "upstream_error", "", 200},
		{"invalid JSON", `not JSON`, "admission_unconfirmed", "", 200},
		{"trailing JSON", `{"eventId":"event-1","status":"ACCEPTED"}{}`, "admission_unconfirmed", "", 200},
		{"trailing garbage", `{"eventId":"event-1","status":"ACCEPTED"}x`, "admission_unconfirmed", "", 200},
		{"missing receipt", `{}`, "admission_unconfirmed", "", 200},
		{"different event", `{"eventId":"other","status":"ACCEPTED"}`, "admission_unconfirmed", "", 200},
		{"unknown status", `{"eventId":"event-1","status":"QUEUED"}`, "admission_unconfirmed", "", 200},
		{"inconsistent duplicate", `{"eventId":"event-1","status":"ACCEPTED","duplicate":true}`, "admission_unconfirmed", "", 200},
		{"retryable outage", `{"code":"460091","retryable":true}`, "upstream_error", "460091", 503},
		{"non JSON outage", `unavailable`, "upstream_error", "", 503},
		{"oversized receipt", strings.Repeat("x", (1<<20)+1), "admission_unconfirmed", "", 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			_, err := pushAppEvent(t.Context(), server.URL, "app-token", appEventPushRequest{EventID: "event-1", Payload: json.RawMessage(`{}`)})
			var classified *apperrors.Error
			if !errors.As(err, &classified) || classified.Reason != test.reason || classified.ServerDiag.ServerErrorCode != test.code {
				t.Fatalf("error=%+v, want reason=%s code=%s", err, test.reason, test.code)
			}
			if classified.Details["eventId"] != "event-1" || calls != 1 || strings.Contains(err.Error(), "app-token") {
				t.Fatalf("incorrect identity, retry count or credential exposure: error=%+v calls=%d", classified, calls)
			}
			if test.name == "schema rejection" && (classified.ServerDiag.TraceID != "trace-1" || !classified.RetryableSet || classified.Retryable) {
				t.Fatalf("server diagnostics lost: %+v", classified)
			}
		})
	}
}

func TestCrossPlatformCoverageAppEventPushTransportBoundaries(t *testing.T) {
	request := appEventPushRequest{EventID: "event-1", Payload: json.RawMessage(`{}`)}
	invalid := request
	invalid.Payload = json.RawMessage(`{`)
	if _, err := pushAppEvent(t.Context(), "https://example.invalid", "app-token", invalid); err == nil {
		t.Fatal("invalid raw payload accepted")
	}
	if _, err := pushAppEvent(t.Context(), "://bad", "app-token", request); err == nil {
		t.Fatal("invalid endpoint accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := pushAppEvent(ctx, "https://example.invalid", "app-token", request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	redirectHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectHits++
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "{")
	}))
	defer server.Close()
	if _, err := pushAppEvent(t.Context(), server.URL+"/redirect", "app-token", request); err == nil || redirectHits != 1 {
		t.Fatalf("redirect followed or accepted: calls=%d error=%v", redirectHits, err)
	}
	if _, err := pushAppEvent(t.Context(), server.URL+"/truncated", "app-token", request); err == nil {
		t.Fatal("truncated response accepted")
	}
	server.Close()
	if _, err := pushAppEvent(t.Context(), server.URL, "app-token", request); err == nil {
		t.Fatal("connection failure accepted")
	}
}

func TestCrossPlatformCoverageAppEventPushErrorCodes(t *testing.T) {
	for _, value := range []any{nil, false, "", "0", "200", "OK", "success", json.Number("0")} {
		if got := appEventPushErrorCode(value); got != "" {
			t.Fatalf("%v interpreted as error %q", value, got)
		}
	}
	if got := appEventPushErrorCode(" FAIL "); got != "FAIL" {
		t.Fatalf("code=%q", got)
	}
}
