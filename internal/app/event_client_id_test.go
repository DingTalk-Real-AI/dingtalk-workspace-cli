// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoveragePersonalEventMissingClientID(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "stored", true: "explicit"}[explicit], func(t *testing.T) {
			dir := setupPersonalIdentityToken(t, &authpkg.TokenData{AccessToken: "test-bearer", ExpiresAt: time.Now().Add(time.Hour)})
			t.Setenv("DWS_CLIENT_ID", "")
			t.Setenv("DWS_CLIENT_SECRET", "")
			testseam.Swap(t, &personalClientIDMetadata, func(string) string { return "" })
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/cli/clientId" || r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("x-user-access-token") != "" {
					t.Error("metadata request used unexpected path, method, or credentials")
				}
				_, _ = w.Write([]byte(`{"success":true,"result":"managed-client"}`))
			}))
			defer srv.Close()
			if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte(srv.URL), 0o600); err != nil {
				t.Fatal(err)
			}
			bearer := ""
			if explicit {
				bearer = "explicit-bearer"
			}
			identity, err := resolvePersonalEventIdentityForToken(context.Background(), dir, "open", bearer)
			if err != nil {
				t.Fatalf("missing AppKey should be fetched: %v", err)
			}
			if identity.ClientID != "managed-client" || calls.Load() != 1 {
				t.Fatalf("id=%q calls=%d", identity.ClientID, calls.Load())
			}
			if edition.Get().AuthClientID != "" {
				t.Fatal("metadata mutated edition")
			}
		})
	}
}

func TestCrossPlatformCoveragePersonalEventClientIDFailureMatrix(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		body, reason string
		retry        bool
	}{
		{"rate-limit", 429, "secret", "event_client_id_fetch_failed", true},
		{"unavailable", 503, "secret", "event_client_id_fetch_failed", true},
		{"denied", 403, "secret", "event_client_id_fetch_failed", false},
		{"redirect", 302, "secret", "event_client_id_fetch_failed", false},
		{"rejected", 200, `{"success":false,"errorMsg":"secret"}`, "event_client_id_rejected", false},
		{"empty", 200, `{"success":true,"result":" "}`, "event_client_id_invalid_response", false},
		{"malformed", 200, `secret`, "event_client_id_invalid_response", false},
		{"wrong-type", 200, `{"success":true,"result":123}`, "event_client_id_invalid_response", false},
		{"placeholder", 200, `{"success":true,"result":"<client-id>"}`, "event_client_id_invalid_response", false},
		{"newline", 200, `{"success":true,"result":"id\nsecret"}`, "event_client_id_invalid_response", false},
		{"too-large", 200, strings.Repeat("s", 65537), "event_client_id_invalid_response", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/redirect-secret")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			_, err := fetchPersonalEventClientID(context.Background(), srv.URL)
			var structured *apperrors.Error
			if !errors.As(err, &structured) || structured.Reason != tc.reason || structured.Retryable != tc.retry || !structured.RetryableSet {
				t.Fatalf("unexpected error: %#v", err)
			}
			if strings.Contains(err.Error(), "secret") || structured.Cause != nil {
				t.Fatal("response or transport detail escaped")
			}
			if calls.Load() != 1 {
				t.Fatalf("requests=%d", calls.Load())
			}
		})
	}
}

func TestCrossPlatformCoveragePersonalEventClientIDContext(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cancel    bool
		wantRetry bool
	}{{"deadline", false, true}, {"cancelled", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(eventRuntimeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 30*time.Second {
					t.Error("missing 30 second bound")
				}
				<-r.Context().Done()
				return nil, r.Context().Err()
			})))
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			if tc.cancel {
				cancel()
			}
			_, err := fetchPersonalEventClientID(ctx, "https://mcp.example.test")
			var structured *apperrors.Error
			if !errors.As(err, &structured) || structured.Retryable != tc.wantRetry || calls > 1 {
				t.Fatalf("error=%#v calls=%d", err, calls)
			}
		})
	}
	for _, endpoint := range []string{"%", "https://user:secret@example.test", "file:///tmp/secret"} {
		_, err := fetchPersonalEventClientID(context.Background(), endpoint)
		var structured *apperrors.Error
		if !errors.As(err, &structured) || structured.Reason != "event_client_id_configuration" {
			t.Fatalf("config error: %#v", err)
		}
	}
}

func TestCrossPlatformCoveragePersonalEventClientIDNoFallbackBoundaries(t *testing.T) {
	testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(eventRuntimeRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("unexpected metadata request")
		return nil, errors.New("MCP unavailable")
	})))
	dir := setupPersonalIdentityToken(t, &authpkg.TokenData{AccessToken: "local-bearer", ClientID: "profile-client", ExpiresAt: time.Now().Add(time.Hour)})
	testseam.Swap(t, &personalClientIDMetadata, func(string) string { return "app-client" })
	identity, err := resolvePersonalEventIdentityForToken(context.Background(), dir, "open", "", personalIdentityOptions{ClientID: "ignored-existing-flag"})
	if err != nil || identity.ClientID != "profile-client" {
		t.Fatalf("stored profile precedence: %#v %v", identity, err)
	}
	identity, err = resolvePersonalEventIdentity(context.Background(), dir, "open", personalIdentityOptions{ClientID: "parent-client"})
	if err != nil || identity.ClientID != "parent-client" {
		t.Fatalf("bus parent precedence: %#v %v", identity, err)
	}
	testseam.Swap(t, &personalLoadProfiles, func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{CurrentProfile: "selected", Profiles: []authpkg.Profile{{Name: "selected", ClientID: "selected-client"}}}, nil
	})
	testseam.Swap(t, &personalRuntimeEventClientID, func() string { return "runtime-client" })
	for _, tc := range []struct{ override, want string }{{"flag-client", "flag-client"}, {"", "runtime-client"}} {
		identity, err = resolvePersonalEventIdentityForToken(context.Background(), dir, "open", "explicit", personalIdentityOptions{ClientID: tc.override})
		if err != nil || identity.ClientID != tc.want {
			t.Fatalf("explicit precedence: %#v %v", identity, err)
		}
	}
	testseam.Swap(t, &personalRuntimeEventClientID, func() string { return "" })
	identity, err = resolvePersonalEventIdentityForToken(context.Background(), dir, "open", "explicit")
	if err != nil || identity.ClientID != "selected-client" {
		t.Fatalf("explicit profile precedence: %#v %v", identity, err)
	}
	testseam.Swap(t, &personalClientIDMetadata, func(string) string { return "" })
	testseam.Swap(t, &personalClientID, func() string { return "" })
	testseam.Swap(t, &personalResolveAppCredentialsStrict, func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "custom-client", "custom-secret", "", "", nil
	})
	id, err := resolvePersonalClientID(context.Background(), dir, "", "custom")
	if err != nil || id != "custom-client" {
		t.Fatalf("custom credentials: %q %v", id, err)
	}
	testseam.Swap(t, &personalResolveAppCredentialsStrict, func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "", "", "", "", errors.New("missing pair")
	})
	if _, err = resolvePersonalClientID(context.Background(), dir, "", "custom"); err == nil {
		t.Fatal("missing custom credentials accepted")
	}
	testseam.Swap(t, &personalClientID, func() string { return "existing-custom-client" })
	if id, err := resolvePersonalClientID(context.Background(), dir, "", "custom"); err != nil || id != "existing-custom-client" {
		t.Fatalf("existing custom identity: %q %v", id, err)
	}
	edition.Override(&edition.Hooks{Name: "enterprise"})
	if _, err = resolvePersonalClientID(context.Background(), dir, "", "normal"); err == nil {
		t.Fatal("customized edition fetched open AppKey")
	}
}

func TestCrossPlatformCoveragePersonalEventClientIDTransportFailure(t *testing.T) {
	for _, tc := range []struct {
		name         string
		transportErr error
		wantRetry    bool
	}{
		{"eof", io.EOF, true},
		{"unclassified", errors.New("secret transport detail"), false},
		{"dns-temporary", &net.DNSError{Err: "temporary failure", IsTemporary: true}, true},
		{"dns-permanent", &net.DNSError{Err: "no such host", IsNotFound: true}, false},
		{"read-body", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(eventRuntimeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 30*time.Second || time.Until(deadline) < 29*time.Second {
					t.Error("default metadata timeout must be 30 seconds")
				}
				if tc.transportErr != nil {
					return nil, tc.transportErr
				}
				return &http.Response{StatusCode: 200, Body: eventRuntimeTokenReadErrorBody{}, Header: http.Header{}}, nil
			})))
			_, err := fetchPersonalEventClientID(context.Background(), "https://mcp.example.test")
			var structured *apperrors.Error
			if !errors.As(err, &structured) || structured.Retryable != tc.wantRetry || calls != 1 {
				t.Fatalf("error=%#v calls=%d", err, calls)
			}
			if strings.Contains(err.Error(), "secret") || structured.Cause != nil {
				t.Fatal("transport detail leaked")
			}
		})
	}
}

func checkPersonalClientIDSocketError(t *testing.T, socketErr error) {
	t.Helper()
	calls := 0
	testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(eventRuntimeRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("socket", socketErr)}
	})))
	_, err := fetchPersonalEventClientID(context.Background(), "https://mcp.example.test")
	var structured *apperrors.Error
	if !errors.As(err, &structured) || !structured.Retryable || calls != 1 {
		t.Fatalf("socket interruption: error=%#v calls=%d", err, calls)
	}
}

func TestCrossPlatformCoveragePersonalEventClientIDFailsBeforeSubscription(t *testing.T) {
	dir := setupPersonalIdentityToken(t, &authpkg.TokenData{AccessToken: "bearer", ExpiresAt: time.Now().Add(time.Hour)})
	t.Setenv("DWS_CONFIG_DIR", dir)
	t.Setenv("DWS_CLIENT_ID", "")
	testseam.Swap(t, &personalClientIDMetadata, func(string) string { return "" })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cli/clientId" {
			t.Errorf("subscription side effect: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"success":true,"result":""}`)
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte(srv.URL), 0600); err != nil {
		t.Fatal(err)
	}
	for _, dry := range []bool{false, true} {
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention, Common: commonConsumeOptions{DryRun: dry}})
		var structured *apperrors.Error
		if !errors.As(err, &structured) || structured.Reason != "event_client_id_invalid_response" {
			t.Fatalf("pre-subscription error=%#v", err)
		}
	}
}
