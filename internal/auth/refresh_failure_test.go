// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageClassifyRefreshFailureUsesStructuredSignals(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want RefreshFailureClass
	}{
		{name: "deadline", err: context.DeadlineExceeded, want: RefreshFailureTransient},
		{name: "network", err: &url.Error{Op: "Post", URL: "https://oauth.test", Err: context.DeadlineExceeded}, want: RefreshFailureTransient},
		{name: "request timeout", err: &HTTPStatusError{StatusCode: http.StatusRequestTimeout}, want: RefreshFailureTransient},
		{name: "rate limited", err: &HTTPStatusError{StatusCode: http.StatusTooManyRequests}, want: RefreshFailureTransient},
		{name: "server unavailable", err: &HTTPStatusError{StatusCode: http.StatusServiceUnavailable}, want: RefreshFailureTransient},
		{name: "refresh rejected", err: &HTTPStatusError{StatusCode: http.StatusUnauthorized}, want: RefreshFailureTerminal},
		{name: "invalid grant", err: &HTTPStatusError{StatusCode: http.StatusBadRequest}, want: RefreshFailureTerminal},
		{name: "forbidden", err: &HTTPStatusError{StatusCode: http.StatusForbidden}, want: RefreshFailureTerminal},
		{name: "local persistence", err: errors.New("save refreshed token failed"), want: RefreshFailureUnknown},
		{name: "nil error", err: nil, want: RefreshFailureUnknown},
		{name: "dns failure", err: &net.DNSError{Err: "no such host", Name: "oauth.test"}, want: RefreshFailureTransient},
		{name: "redirect status", err: &HTTPStatusError{StatusCode: http.StatusFound}, want: RefreshFailureUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyRefreshFailure(tt.err); got != tt.want {
				t.Fatalf("ClassifyRefreshFailure() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageHTTPStatusErrorRetainsStatusThroughWrapping(t *testing.T) {
	want := &HTTPStatusError{StatusCode: http.StatusTooManyRequests}
	err := errors.Join(errors.New("refresh failed"), want)
	if got := ClassifyRefreshFailure(err); got != RefreshFailureTransient {
		t.Fatalf("ClassifyRefreshFailure() = %q, want transient", got)
	}
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("HTTP status error not retained: %v", err)
	}
	if got, want := statusErr.Error(), "HTTP 429"; got != want {
		t.Fatalf("HTTP status error = %q, want %q", got, want)
	}
	var nilStatus *HTTPStatusError
	if got, want := nilStatus.Error(), "OAuth endpoint request failed"; got != want {
		t.Fatalf("nil HTTP status error = %q, want %q", got, want)
	}
}

func TestCrossPlatformCoverageOAuthExchangeDisplayErrorFallsBackToPlainError(t *testing.T) {
	if got, want := oauthExchangeDisplayError(&HTTPStatusError{StatusCode: http.StatusBadGateway}), "HTTP 502: token exchange failed"; got != want {
		t.Fatalf("status display error = %q, want %q", got, want)
	}
	if got, want := oauthExchangeDisplayError(errors.New("exchange failed")), "exchange failed"; got != want {
		t.Fatalf("plain display error = %q, want %q", got, want)
	}
}

func TestCrossPlatformCoveragePostJSONClassifiesStatusWithoutLoggingResponseBody(t *testing.T) {
	const secretBody = `{"refreshToken":"must-not-reach-logs"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(secretBody))
	}))
	defer server.Close()

	provider := &OAuthProvider{httpClient: server.Client()}
	_, err := provider.postJSON(context.Background(), server.URL, map[string]string{"grantType": "refresh_token"})
	if got := ClassifyRefreshFailure(err); got != RefreshFailureTransient {
		t.Fatalf("ClassifyRefreshFailure() = %q, want transient: %v", got, err)
	}
	if strings.Contains(err.Error(), "must-not-reach-logs") {
		t.Fatalf("postJSON error leaked response body: %v", err)
	}
	if got := httpStatusResponseBody(err); !strings.Contains(got, "must-not-reach-logs") {
		t.Fatalf("postJSON did not retain bounded response details for internal classification: %q", got)
	}
}

func TestCrossPlatformCoverageGetTokenSnapshotOnlyExpiresProfileForNonTransientRefreshFailures(t *testing.T) {
	oldLoad := oauthLoadToken
	oldLoadLocked := oauthLoadTokenLocked
	oldAcquire := oauthAcquireLock
	oldRefresh := oauthRefreshToken
	oldMark := oauthMarkProfile
	oldEdition := edition.Get()
	t.Cleanup(func() {
		oauthLoadToken = oldLoad
		oauthLoadTokenLocked = oldLoadLocked
		oauthAcquireLock = oldAcquire
		oauthRefreshToken = oldRefresh
		oauthMarkProfile = oldMark
		edition.Override(oldEdition)
	})
	edition.Override(&edition.Hooks{})

	expired := &TokenData{
		AccessToken:  "expired-access",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshToken: "refresh",
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp",
		UserID:       "user",
	}
	oauthLoadToken = func(string) (*TokenData, error) { return expired, nil }
	oauthLoadTokenLocked = func(string, string) (*TokenData, error) { return expired, nil }
	oauthAcquireLock = func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil }

	markCalls := 0
	oauthMarkProfile = func(_, _, status string) error {
		if status != ProfileStatusExpired {
			t.Fatalf("profile status = %q, want %q", status, ProfileStatusExpired)
		}
		markCalls++
		return nil
	}
	provider := NewOAuthProvider(t.TempDir(), nil)

	oauthRefreshToken = func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) {
		return nil, &HTTPStatusError{StatusCode: http.StatusServiceUnavailable}
	}
	if _, err := provider.GetTokenSnapshot(context.Background()); ClassifyRefreshFailure(err) != RefreshFailureTransient {
		t.Fatalf("transient refresh error = %v", err)
	}
	if markCalls != 0 {
		t.Fatalf("transient refresh marked profile expired %d times", markCalls)
	}

	oauthRefreshToken = func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) {
		return nil, &HTTPStatusError{StatusCode: http.StatusUnauthorized}
	}
	if _, err := provider.GetTokenSnapshot(context.Background()); ClassifyRefreshFailure(err) != RefreshFailureTerminal {
		t.Fatalf("terminal refresh error = %v", err)
	}
	if markCalls != 1 {
		t.Fatalf("terminal refresh marked profile expired %d times, want 1", markCalls)
	}
}
