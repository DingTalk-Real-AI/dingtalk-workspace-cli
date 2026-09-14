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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
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

	oauthRefreshToken = func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) {
		return nil, &MCPTokenExchangeError{
			Code:    legacyMCPRefreshRejectedCode,
			Message: "不合法的临时授权码",
		}
	}
	_, err := provider.GetTokenSnapshot(context.Background())
	if err == nil || !strings.Contains(err.Error(), "dws auth login") ||
		!strings.Contains(err.Error(), "--profile") ||
		!strings.Contains(err.Error(), `profile: "corp:user"`) ||
		strings.Contains(err.Error(), `dws auth login --profile "corp:user"`) ||
		!strings.Contains(err.Error(), legacyMCPRefreshRejectedCode) {
		t.Fatalf("legacy MCP refresh guidance = %v", err)
	}
	var exchangeErr *MCPTokenExchangeError
	if !errors.As(err, &exchangeErr) || !exchangeErr.requiresReauthorization() {
		t.Fatalf("legacy MCP refresh cause was not preserved: %v", err)
	}
	if markCalls != 2 {
		t.Fatalf("legacy MCP rejection marked profile expired %d times, want 2", markCalls)
	}

	SetRuntimeProfile("External Worker")
	_, err = provider.GetTokenSnapshot(context.Background())
	SetRuntimeProfile("")
	if err == nil || !strings.Contains(err.Error(), "dws auth login") ||
		!strings.Contains(err.Error(), `profile: "External Worker"`) ||
		strings.Contains(err.Error(), `dws auth login --profile "External Worker"`) {
		t.Fatalf("legacy MCP refresh guidance did not isolate spaced selector as display data: %v", err)
	}
	if markCalls != 3 {
		t.Fatalf("spaced legacy MCP rejection marked profile expired %d times, want 3", markCalls)
	}
}

func TestCrossPlatformCoverageLegacyGlobalSlotRecoversRejectedIdentityRefresh(t *testing.T) {
	selected := &TokenData{
		AccessToken:  "expired-identity-access",
		RefreshToken: "rejected-identity-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp-v1039",
		UserID:       "user-v1039",
		UserName:     "V1039 User",
		Source:       "mcp",
	}
	legacy := &TokenData{
		AccessToken:  "valid-legacy-global-access",
		RefreshToken: "valid-legacy-global-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       selected.CorpID,
		Source:       "mcp",
	}

	testseam.Swap(t, &oauthAcquireLock, func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil })
	testseam.Swap(t, &oauthLoadTokenLocked, func(string, string) (*TokenData, error) { return selected, nil })
	testseam.Swap(t, &oauthRefreshToken, func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) {
		return nil, &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"}
	})
	testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
	testseam.Swap(t, &tokenLoadProfiles, func(string) (*ProfilesConfig, error) {
		return &ProfilesConfig{Version: profilesVersion, Profiles: []Profile{{
			Name:     "V1039 User",
			CorpID:   selected.CorpID,
			UserID:   selected.UserID,
			UserName: selected.UserName,
		}}}, nil
	})
	testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound })
	var saved *TokenData
	testseam.Swap(t, &oauthSaveTokenLocked, func(_ string, data *TokenData) error {
		copy := *data
		saved = &copy
		return nil
	})

	recovered, err := NewOAuthProvider(t.TempDir(), nil).lockedRefresh(context.Background())
	if err != nil {
		t.Fatalf("lockedRefresh() error = %v", err)
	}
	if recovered.AccessToken != legacy.AccessToken || recovered.RefreshToken != legacy.RefreshToken {
		t.Fatalf("recovered token = %#v, want legacy credential material %#v", recovered, legacy)
	}
	if recovered.UserID != selected.UserID || recovered.UserName != selected.UserName {
		t.Fatalf("recovered identity = %q/%q, want selected identity %q/%q", recovered.UserID, recovered.UserName, selected.UserID, selected.UserName)
	}
	if saved == nil || saved.AccessToken != recovered.AccessToken || saved.UserID != selected.UserID {
		t.Fatalf("saved recovery token = %#v, want recovered identity token %#v", saved, recovered)
	}
}

func TestCrossPlatformCoverageLegacyGlobalSlotRejectsBlankUserIDForMultiAccountCorp(t *testing.T) {
	selected := &TokenData{
		AccessToken:  "expired-identity-access",
		RefreshToken: "rejected-identity-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp-v1039-multi",
		UserID:       "user-v1039-a",
		Source:       "mcp",
	}
	legacy := &TokenData{
		AccessToken:  "valid-legacy-global-access",
		RefreshToken: "valid-legacy-global-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       selected.CorpID,
		Source:       "mcp",
	}
	rejection := &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"}

	testseam.Swap(t, &oauthAcquireLock, func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil })
	testseam.Swap(t, &oauthLoadTokenLocked, func(string, string) (*TokenData, error) { return selected, nil })
	testseam.Swap(t, &oauthRefreshToken, func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) { return nil, rejection })
	testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
	testseam.Swap(t, &tokenLoadProfiles, func(string) (*ProfilesConfig, error) {
		return &ProfilesConfig{Version: profilesVersion, Profiles: []Profile{
			{Name: "User A", CorpID: selected.CorpID, UserID: selected.UserID},
			{Name: "User B", CorpID: selected.CorpID, UserID: "user-v1039-b"},
		}}, nil
	})
	testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound })
	saved := false
	testseam.Swap(t, &oauthSaveTokenLocked, func(string, *TokenData) error {
		saved = true
		return nil
	})

	_, err := NewOAuthProvider(t.TempDir(), nil).lockedRefresh(context.Background())
	if !errors.Is(err, rejection) {
		t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
	}
	if saved {
		t.Fatal("legacy global recovery saved a blank-user token for a multi-account organization")
	}
}

func TestCrossPlatformCoverageLegacyGlobalSlotRejectsBlankSelectedUserIDForMultiAccountCorp(t *testing.T) {
	selected := &TokenData{
		AccessToken:  "expired-identity-access",
		RefreshToken: "rejected-identity-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp-v1039-blank-selected",
		Source:       "mcp",
	}
	legacy := &TokenData{
		AccessToken:  "valid-legacy-global-access",
		RefreshToken: "valid-legacy-global-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       selected.CorpID,
		Source:       "mcp",
	}
	rejection := &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"}

	testseam.Swap(t, &oauthAcquireLock, func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil })
	testseam.Swap(t, &oauthLoadTokenLocked, func(string, string) (*TokenData, error) { return selected, nil })
	testseam.Swap(t, &oauthRefreshToken, func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) { return nil, rejection })
	testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
	testseam.Swap(t, &tokenLoadProfiles, func(string) (*ProfilesConfig, error) {
		return &ProfilesConfig{Version: profilesVersion, Profiles: []Profile{
			{Name: "Blank A", CorpID: selected.CorpID, UserID: ""},
			{Name: "Blank B", CorpID: selected.CorpID, UserID: ""},
		}}, nil
	})
	testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound })
	saved := false
	testseam.Swap(t, &oauthSaveTokenLocked, func(string, *TokenData) error {
		saved = true
		return nil
	})

	_, err := NewOAuthProvider(t.TempDir(), nil).lockedRefresh(context.Background())
	if !errors.Is(err, rejection) {
		t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
	}
	if saved {
		t.Fatal("legacy global recovery saved a blank-selected token for a multi-account organization")
	}
}

func TestCrossPlatformCoverageLegacyGlobalSlotRejectsDifferentUserID(t *testing.T) {
	selected := &TokenData{
		AccessToken:  "expired-identity-access",
		RefreshToken: "rejected-identity-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp-v1039-user-mismatch",
		UserID:       "user-v1039-selected",
		Source:       "mcp",
	}
	legacy := &TokenData{
		AccessToken:  "valid-legacy-global-access",
		RefreshToken: "valid-legacy-global-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       selected.CorpID,
		UserID:       "user-v1039-other",
		Source:       "mcp",
	}
	rejection := &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"}

	testseam.Swap(t, &oauthAcquireLock, func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil })
	testseam.Swap(t, &oauthLoadTokenLocked, func(string, string) (*TokenData, error) { return selected, nil })
	testseam.Swap(t, &oauthRefreshToken, func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) { return nil, rejection })
	testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
	testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound })
	saved := false
	testseam.Swap(t, &oauthSaveTokenLocked, func(string, *TokenData) error {
		saved = true
		return nil
	})

	_, err := NewOAuthProvider(t.TempDir(), nil).lockedRefresh(context.Background())
	if !errors.Is(err, rejection) {
		t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
	}
	if saved {
		t.Fatal("legacy global recovery saved a token owned by a different user")
	}
}

func TestCrossPlatformCoverageLegacyRefreshFailureKeepsBlankCurrentSelectorIsolated(t *testing.T) {
	fixture := seedBlankProfileSelectorFixture(t, "Fixture Organization", "Fixture Organization", true)
	expired := *fixture.blankToken
	expired.ExpiresAt = time.Now().Add(-time.Hour)
	expired.RefreshExpAt = time.Now().Add(time.Hour)

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
	oauthLoadToken = func(string) (*TokenData, error) { return &expired, nil }
	oauthLoadTokenLocked = func(string, string) (*TokenData, error) { return &expired, nil }
	oauthAcquireLock = func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil }
	oauthRefreshToken = func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) {
		return nil, &MCPTokenExchangeError{
			Code:    legacyMCPRefreshRejectedCode,
			Message: "legacy refresh rejected",
		}
	}
	var markedSelector string
	oauthMarkProfile = func(configDir, selector, status string) error {
		markedSelector = selector
		return MarkProfileStatus(configDir, selector, status)
	}

	provider := NewOAuthProvider(fixture.configDir, nil)
	_, err := provider.GetTokenSnapshot(context.Background())
	if err == nil || !strings.Contains(err.Error(), "dws auth login") ||
		!strings.Contains(err.Error(), "--profile") ||
		!strings.Contains(err.Error(), "profile: "+strconv.Quote(fixture.blankSelector)) ||
		strings.Contains(err.Error(), "dws auth login --profile") {
		t.Fatalf("legacy blank refresh guidance = %v, want selector %q", err, fixture.blankSelector)
	}
	if markedSelector != fixture.blankSelector {
		t.Fatalf("marked selector = %q, want blank %q", markedSelector, fixture.blankSelector)
	}
	cfg, loadErr := LoadProfiles(fixture.configDir)
	if loadErr != nil {
		t.Fatalf("LoadProfiles() error = %v", loadErr)
	}
	for _, profile := range cfg.Profiles {
		switch profile.UserID {
		case "":
			if profile.Status != ProfileStatusExpired {
				t.Fatalf("blank profile status = %q, want expired", profile.Status)
			}
		case fixture.exactUserID:
			if profile.Status != ProfileStatusActive {
				t.Fatalf("exact profile status = %q, want active", profile.Status)
			}
		}
	}
}

func TestCrossPlatformCoverageLegacyGlobalSlotRejectsSingleProfileIdentityMismatch(t *testing.T) {
	selected := &TokenData{
		AccessToken:  "expired-identity-access",
		RefreshToken: "rejected-identity-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp-v1039-single-mismatch",
		UserID:       "user-v1039-selected",
		Source:       "mcp",
	}
	legacy := &TokenData{
		AccessToken:  "valid-legacy-global-access",
		RefreshToken: "valid-legacy-global-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       selected.CorpID,
		Source:       "mcp",
	}
	rejection := &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"}

	testseam.Swap(t, &oauthAcquireLock, func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil })
	testseam.Swap(t, &oauthLoadTokenLocked, func(string, string) (*TokenData, error) { return selected, nil })
	testseam.Swap(t, &oauthRefreshToken, func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) { return nil, rejection })
	testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
	testseam.Swap(t, &tokenLoadProfiles, func(string) (*ProfilesConfig, error) {
		return &ProfilesConfig{Version: profilesVersion, Profiles: []Profile{{
			Name:   "Other User",
			CorpID: selected.CorpID,
			UserID: "user-v1039-other",
		}}}, nil
	})
	testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound })
	saved := false
	testseam.Swap(t, &oauthSaveTokenLocked, func(string, *TokenData) error {
		saved = true
		return nil
	})

	_, err := NewOAuthProvider(t.TempDir(), nil).lockedRefresh(context.Background())
	if !errors.Is(err, rejection) {
		t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
	}
	if saved {
		t.Fatal("legacy global recovery saved a blank-user token whose single profile identity does not match")
	}
}

func TestCrossPlatformCoverageLegacyGlobalSlotRefreshesExpiredLegacyCredential(t *testing.T) {
	selected := &TokenData{
		AccessToken:  "expired-identity-access",
		RefreshToken: "rejected-identity-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp-v1039-legacy-refresh",
		UserID:       "user-v1039",
		UserName:     "V1039 User",
		Source:       "mcp",
	}
	legacy := &TokenData{
		AccessToken:  "expired-legacy-global-access",
		RefreshToken: "valid-legacy-global-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       selected.CorpID,
		UserID:       selected.UserID,
		Source:       "mcp",
	}
	refreshed := &TokenData{
		AccessToken:  "refreshed-legacy-access",
		RefreshToken: "rotated-legacy-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       selected.CorpID,
		UserID:       selected.UserID,
		Source:       "mcp",
	}
	rejection := &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"}

	testseam.Swap(t, &oauthAcquireLock, func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil })
	testseam.Swap(t, &oauthLoadTokenLocked, func(string, string) (*TokenData, error) { return selected, nil })
	refreshCalls := 0
	testseam.Swap(t, &oauthRefreshToken, func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) {
		refreshCalls++
		if refreshCalls == 1 {
			return nil, rejection
		}
		return refreshed, nil
	})
	testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
	testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound })

	recovered, err := NewOAuthProvider(t.TempDir(), nil).lockedRefresh(context.Background())
	if err != nil {
		t.Fatalf("lockedRefresh() error = %v", err)
	}
	if refreshCalls != 2 {
		t.Fatalf("oauthRefreshToken called %d times, want 2 (identity rejection + legacy refresh)", refreshCalls)
	}
	if recovered.AccessToken != refreshed.AccessToken {
		t.Fatalf("recovered access token = %q, want refreshed legacy credential %q", recovered.AccessToken, refreshed.AccessToken)
	}
	if recovered.RefreshToken != refreshed.RefreshToken {
		t.Fatalf("recovered refresh token = %q, want rotated credential %q", recovered.RefreshToken, refreshed.RefreshToken)
	}
}

func TestCrossPlatformCoverageLegacyGlobalSlotRejectsProfilesLoadError(t *testing.T) {
	selected := &TokenData{
		AccessToken:  "expired-identity-access",
		RefreshToken: "rejected-identity-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp-v1039-profiles-error",
		UserID:       "user-v1039",
		Source:       "mcp",
	}
	legacy := &TokenData{
		AccessToken:  "valid-legacy-global-access",
		RefreshToken: "valid-legacy-global-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       selected.CorpID,
		Source:       "mcp",
	}
	rejection := &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"}

	testseam.Swap(t, &oauthAcquireLock, func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil })
	testseam.Swap(t, &oauthLoadTokenLocked, func(string, string) (*TokenData, error) { return selected, nil })
	testseam.Swap(t, &oauthRefreshToken, func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) { return nil, rejection })
	testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
	testseam.Swap(t, &tokenLoadProfiles, func(string) (*ProfilesConfig, error) { return nil, errors.New("profiles read failed") })
	testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound })
	saved := false
	testseam.Swap(t, &oauthSaveTokenLocked, func(string, *TokenData) error {
		saved = true
		return nil
	})

	_, err := NewOAuthProvider(t.TempDir(), nil).lockedRefresh(context.Background())
	if !errors.Is(err, rejection) {
		t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
	}
	if saved {
		t.Fatal("legacy global recovery saved a blank-user token when profiles could not be loaded")
	}
}

func TestCrossPlatformCoverageLegacyGlobalSlotRejectsSameRefreshToken(t *testing.T) {
	selected := &TokenData{
		AccessToken:  "expired-identity-access",
		RefreshToken: "shared-rejected-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp-v1039-same-refresh",
		UserID:       "user-v1039",
		Source:       "mcp",
	}
	legacy := &TokenData{
		AccessToken:  "expired-legacy-global-access",
		RefreshToken: selected.RefreshToken,
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       selected.CorpID,
		UserID:       selected.UserID,
		Source:       "mcp",
	}
	rejection := &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"}

	testseam.Swap(t, &oauthAcquireLock, func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil })
	testseam.Swap(t, &oauthLoadTokenLocked, func(string, string) (*TokenData, error) { return selected, nil })
	testseam.Swap(t, &oauthRefreshToken, func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) { return nil, rejection })
	testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
	testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound })
	saved := false
	testseam.Swap(t, &oauthSaveTokenLocked, func(string, *TokenData) error {
		saved = true
		return nil
	})

	_, err := NewOAuthProvider(t.TempDir(), nil).lockedRefresh(context.Background())
	if !errors.Is(err, rejection) {
		t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
	}
	if saved {
		t.Fatal("legacy global recovery saved a token holding the same rejected refresh_token")
	}
}

func TestCrossPlatformCoverageLegacyGlobalSlotRejectsNilSelectedAndEmptyLegacy(t *testing.T) {
	rejection := &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"}
	provider := NewOAuthProvider(t.TempDir(), nil)

	// nil selected must be rejected before any dereference.
	if _, err := provider.recoverRefreshFromLegacyGlobalSlot(context.Background(), nil, rejection); !errors.Is(err, rejection) {
		t.Fatalf("nil selected error = %v, want original rejection", err)
	}

	// A keychain load that returns (nil, nil) must be rejected before any dereference.
	testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return nil, nil })
	selected := &TokenData{
		CorpID: "corp-v1039-nil-legacy",
		UserID: "user-v1039",
		Source: "mcp",
	}
	if _, err := provider.recoverRefreshFromLegacyGlobalSlot(context.Background(), selected, rejection); !errors.Is(err, rejection) {
		t.Fatalf("nil legacy error = %v, want original rejection", err)
	}
}

func TestCrossPlatformCoverageLegacyGlobalRecoveryRejectsNonReauthorizationErrors(t *testing.T) {
	provider := NewOAuthProvider(t.TempDir(), nil)
	selected := &TokenData{CorpID: "corp-v1039-plain", UserID: "user-v1039", Source: "mcp"}

	plainErr := errors.New("plain refresh failure")
	if _, err := provider.recoverRefreshFromLegacyGlobalSlot(context.Background(), selected, plainErr); !errors.Is(err, plainErr) {
		t.Fatalf("plain error = %v, want original plain failure", err)
	}
}

func TestCrossPlatformCoverageLegacyGlobalRecoveryRejectsSaveAndRefreshFailures(t *testing.T) {
	rejection := &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"}
	selected := &TokenData{
		CorpID: "corp-v1039-recovery-steps",
		UserID: "user-v1039",
		Source: "mcp",
	}

	t.Run("save_failure", func(t *testing.T) {
		legacy := &TokenData{
			AccessToken:  "valid-legacy-access",
			RefreshToken: "valid-legacy-refresh",
			ExpiresAt:    time.Now().Add(time.Hour),
			RefreshExpAt: time.Now().Add(24 * time.Hour),
			CorpID:       selected.CorpID,
			UserID:       selected.UserID,
			Source:       "mcp",
		}
		testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
		testseam.Swap(t, &oauthSaveTokenLocked, func(string, *TokenData) error { return errors.New("save failed") })
		if _, err := NewOAuthProvider(t.TempDir(), nil).recoverRefreshFromLegacyGlobalSlot(context.Background(), selected, rejection); !errors.Is(err, rejection) {
			t.Fatalf("save failure error = %v, want original rejection", err)
		}
	})

	t.Run("refresh_expired", func(t *testing.T) {
		legacy := &TokenData{
			AccessToken:  "expired-legacy-access",
			RefreshToken: "expired-legacy-refresh",
			ExpiresAt:    time.Now().Add(-time.Hour),
			RefreshExpAt: time.Now().Add(-time.Hour),
			CorpID:       selected.CorpID,
			UserID:       selected.UserID,
			Source:       "mcp",
		}
		testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
		if _, err := NewOAuthProvider(t.TempDir(), nil).recoverRefreshFromLegacyGlobalSlot(context.Background(), selected, rejection); !errors.Is(err, rejection) {
			t.Fatalf("expired refresh error = %v, want original rejection", err)
		}
	})

	t.Run("refresh_error", func(t *testing.T) {
		legacy := &TokenData{
			AccessToken:  "expired-legacy-access",
			RefreshToken: "valid-legacy-refresh",
			ExpiresAt:    time.Now().Add(-time.Hour),
			RefreshExpAt: time.Now().Add(24 * time.Hour),
			CorpID:       selected.CorpID,
			UserID:       selected.UserID,
			Source:       "mcp",
		}
		testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
		testseam.Swap(t, &oauthRefreshToken, func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) {
			return nil, errors.New("legacy refresh failed")
		})
		if _, err := NewOAuthProvider(t.TempDir(), nil).recoverRefreshFromLegacyGlobalSlot(context.Background(), selected, rejection); !errors.Is(err, rejection) {
			t.Fatalf("legacy refresh error = %v, want original rejection", err)
		}
	})

	t.Run("preflight_error", func(t *testing.T) {
		legacy := &TokenData{
			AccessToken:  "expired-legacy-access",
			RefreshToken: "valid-legacy-refresh",
			ExpiresAt:    time.Now().Add(-time.Hour),
			RefreshExpAt: time.Now().Add(24 * time.Hour),
			CorpID:       selected.CorpID,
			UserID:       selected.UserID,
			Source:       "mcp",
		}
		testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
		testseam.Swap(t, &profilesReadFile, func(string) ([]byte, error) { return nil, errors.New("read failed") })
		if _, err := NewOAuthProvider(t.TempDir(), nil).recoverRefreshFromLegacyGlobalSlot(context.Background(), selected, rejection); !errors.Is(err, rejection) {
			t.Fatalf("preflight error = %v, want original rejection", err)
		}
	})
}

func TestCrossPlatformCoverageLegacyGlobalCandidateMatchingBoundaries(t *testing.T) {
	configDir := t.TempDir()
	selected := &TokenData{CorpID: "corp-v1039-candidate", UserID: "user-v1039"}

	if legacyGlobalRefreshCandidateMatches(configDir, selected, nil) {
		t.Fatal("nil legacy accepted")
	}
	if legacyGlobalRefreshCandidateMatches(configDir, selected, &TokenData{CorpID: "corp-other", UserID: selected.UserID}) {
		t.Fatal("different corp accepted")
	}
	if legacyGlobalRefreshCandidateMatches(configDir, &TokenData{UserID: "user-v1039"}, &TokenData{UserID: "user-v1039"}) {
		t.Fatal("blank selected corp accepted")
	}

	blankSelected := &TokenData{CorpID: selected.CorpID}
	testseam.Swap(t, &tokenLoadProfiles, func(string) (*ProfilesConfig, error) {
		return &ProfilesConfig{Version: profilesVersion, Profiles: []Profile{{Name: "Blank User", CorpID: selected.CorpID}}}, nil
	})
	if !legacyGlobalRefreshCandidateMatches(configDir, blankSelected, &TokenData{CorpID: selected.CorpID}) {
		t.Fatal("both blank user IDs should match only through the single-profile guard")
	}
	if legacyGlobalRefreshCandidateMatches(configDir, blankSelected, &TokenData{CorpID: selected.CorpID, UserID: "user-other"}) {
		t.Fatal("blank selected with non-blank legacy accepted")
	}

	if legacyGlobalBlankUserIDMatchesSingleProfile(configDir, "", selected.UserID) {
		t.Fatal("blank corp accepted by single-profile check")
	}
}
