// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageIsRefreshTokenRejected(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"mcp authCode.notFound", &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"}, true},
		{"mcp other business code", &MCPTokenExchangeError{Code: "other.error", Message: "boom"}, false},
		{"wrapped mcp rejection", fmt.Errorf("refresh: %w", &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode}), true},
		{"http 400 has no reviewed business code", &HTTPStatusError{StatusCode: http.StatusBadRequest}, false},
		{"http 401 has no reviewed business code", &HTTPStatusError{StatusCode: http.StatusUnauthorized}, false},
		{"http 403 has no reviewed business code", &HTTPStatusError{StatusCode: http.StatusForbidden}, false},
		{"http 500 is transient", &HTTPStatusError{StatusCode: http.StatusInternalServerError}, false},
		{"http 429 is transient", &HTTPStatusError{StatusCode: http.StatusTooManyRequests}, false},
		{"plain error is unknown", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRefreshTokenRejected(tt.err); got != tt.want {
				t.Fatalf("isRefreshTokenRejected(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// orgSlotFallbackFixture wires the injectable seams lockedRefresh depends on
// and records refresh attempts plus organization slot lookups.
type orgSlotFallbackFixture struct {
	provider       *OAuthProvider
	stale          *TokenData
	orgMirror      *TokenData
	renewed        *TokenData
	rejected       *MCPTokenExchangeError
	refreshErr     error
	orgRefreshErr  error
	refreshCalls   []string
	refreshUserIDs []string
	orgLoads       int
}

func newOrgSlotFallbackFixture(t *testing.T) *orgSlotFallbackFixture {
	t.Helper()
	isolateOAuthPersistence(t)

	f := &orgSlotFallbackFixture{
		provider: &OAuthProvider{configDir: t.TempDir(), logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Output: io.Discard},
		stale: &TokenData{
			AccessToken:  "old-access",
			RefreshToken: "stale-refresh",
			ExpiresAt:    time.Now().Add(-time.Hour),
			RefreshExpAt: time.Now().Add(time.Hour),
			CorpID:       "corp-1",
			UserID:       "user-1",
		},
		orgMirror: &TokenData{
			AccessToken:  "org-access",
			RefreshToken: "org-refresh",
			ExpiresAt:    time.Now().Add(-time.Minute),
			RefreshExpAt: time.Now().Add(time.Hour),
			CorpID:       "corp-1",
			UserID:       "user-1",
		},
		renewed: &TokenData{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresAt:    time.Now().Add(time.Hour),
			RefreshExpAt: time.Now().Add(24 * time.Hour),
			CorpID:       "corp-1",
			UserID:       "user-1",
		},
		rejected: &MCPTokenExchangeError{Code: legacyMCPRefreshRejectedCode, Message: "authCode not found"},
	}
	f.refreshErr = f.rejected

	testseam.Swap(t, &oauthAcquireLock, func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil })
	testseam.Swap(t, &oauthLoadTokenLocked, func(configDir, _ string) (*TokenData, error) { return oauthLoadToken(configDir) })
	testseam.Swap(t, &oauthLoadToken, func(string) (*TokenData, error) { return f.stale, nil })
	testseam.Swap(t, &oauthRefreshToken, func(_ *OAuthProvider, _ context.Context, data *TokenData) (*TokenData, error) {
		f.refreshCalls = append(f.refreshCalls, data.RefreshToken)
		f.refreshUserIDs = append(f.refreshUserIDs, data.UserID)
		switch data.RefreshToken {
		case "stale-refresh":
			return nil, f.refreshErr
		case "org-refresh":
			return f.renewed, f.orgRefreshErr
		}
		return nil, fmt.Errorf("unexpected refresh token %q", data.RefreshToken)
	})
	testseam.Swap(t, &tokenLoadKeychainForCorpID, func(corpID string) (*TokenData, error) {
		f.orgLoads++
		if corpID != "corp-1" {
			return nil, ErrTokenDataNotFound
		}
		return f.orgMirror, nil
	})
	return f
}

func TestCrossPlatformCoverageLockedRefreshFallsBackToOrgSlot(t *testing.T) {
	f := newOrgSlotFallbackFixture(t)

	got, err := f.provider.lockedRefresh(context.Background())
	if err != nil || got != f.renewed {
		t.Fatalf("lockedRefresh() = %#v, %v; want renewed token, nil", got, err)
	}
	if len(f.refreshCalls) != 2 || f.refreshCalls[0] != "stale-refresh" || f.refreshCalls[1] != "org-refresh" {
		t.Fatalf("refresh attempts = %v, want [stale-refresh org-refresh]", f.refreshCalls)
	}
	if f.orgLoads != 1 {
		t.Fatalf("organization slot loads = %d, want 1", f.orgLoads)
	}
}

func TestCrossPlatformCoverageRepairMarkerForcesOrganizationSlotWrite(t *testing.T) {
	cfg := &ProfilesConfig{
		Version: profilesVersion,
		Profiles: []Profile{
			{Name: "legacy", CorpID: "corp-1", UserID: ""},
			{Name: "user-1", CorpID: "corp-1", UserID: "user-1"},
		},
	}
	selector := profileSelector("corp-1", "user-1")

	without := &TokenData{CorpID: "corp-1", UserID: "user-1"}
	if plan := planTokenPersistenceWrites(cfg, without, selector); plan.WriteOrganization {
		t.Fatalf("explicit selector preserved unresolved org slot: WriteOrganization = true, want false")
	}

	with := &TokenData{CorpID: "corp-1", UserID: "user-1", RepairOrganizationMirror: true}
	if plan := planTokenPersistenceWrites(cfg, with, selector); !plan.WriteOrganization {
		t.Fatalf("repair marker did not force the organization slot write")
	}
}

func TestCrossPlatformCoverageLockedRefreshFallbackRepairsPersistedSlots(t *testing.T) {
	isolateOAuthPersistence(t)
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")

	// Fake MCP refresh endpoint: the first call rejects the stale identity
	// refresh_token with the reviewed business code; the second call (the
	// organization mirror) succeeds and returns a rotated credential.
	var refreshCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if refreshCalls.Add(1) == 1 {
			fmt.Fprint(w, `{"errorCode":"invalidParameter.authCode.notFound","errorMsg":"authCode not found"}`)
			return
		}
		fmt.Fprint(w, `{"accessToken":"new-access","refreshToken":"new-refresh","expiresIn":7200,"corpId":"corp-1","userId":"user-1","userName":"User One"}`)
	}))
	defer srv.Close()

	configDir := setupMCPConfigDir(t, srv.URL)
	resetAppConfigCache()

	// Seed the pre-fallback state: an identity slot whose refresh_token the
	// server rejects, plus a legacy organization mirror (no userId) with a
	// still-valid refresh_token and a preserved unresolved sibling profile so
	// an explicit --profile refresh would normally skip the org slot.
	cfg := &ProfilesConfig{
		Version: profilesVersion,
		Profiles: []Profile{
			{Name: "corp-1", CorpID: "corp-1", UserID: "", ClientID: "mcp-client"},
			{Name: "user-1", CorpID: "corp-1", UserID: "user-1", UserName: "User One", ClientID: "mcp-client"},
		},
	}
	if err := SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	orgMirror := &TokenData{
		AccessToken:  "org-access",
		RefreshToken: "org-refresh",
		ExpiresAt:    time.Now().Add(-time.Minute),
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp-1",
		Source:       "mcp",
		ClientID:     "mcp-client",
	}
	if err := SaveTokenDataKeychainForCorpID("corp-1", orgMirror); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID() error = %v", err)
	}
	staleIdentity := &TokenData{
		AccessToken:  "stale-access",
		RefreshToken: "stale-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp-1",
		UserID:       "user-1",
		UserName:     "User One",
		Source:       "mcp",
		ClientID:     "mcp-client",
	}
	if err := SaveTokenDataKeychainForIdentity("corp-1", "user-1", staleIdentity); err != nil {
		t.Fatalf("SaveTokenDataKeychainForIdentity() error = %v", err)
	}

	SetRuntimeProfile("corp-1:user-1")
	t.Cleanup(func() { SetRuntimeProfile("") })

	p := &OAuthProvider{
		configDir:  configDir,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Output:     io.Discard,
		httpClient: srv.Client(),
	}
	got, err := p.lockedRefresh(context.Background())
	if err != nil {
		t.Fatalf("lockedRefresh() error = %v", err)
	}
	if got == nil || got.AccessToken != "new-access" || got.RefreshToken != "new-refresh" {
		t.Fatalf("lockedRefresh() = %#v, want rotated credential", got)
	}
	if refreshCalls.Load() != 2 {
		t.Fatalf("MCP refresh calls = %d, want primary rejection plus fallback", refreshCalls.Load())
	}

	// The fallback consumed the mirror's refresh_token: both persisted slots
	// must now carry the rotated credential instead of the consumed one.
	orgSlot, err := LoadTokenDataKeychainForCorpID("corp-1")
	if err != nil {
		t.Fatalf("LoadTokenDataKeychainForCorpID() error = %v", err)
	}
	if orgSlot.RefreshToken != "new-refresh" || orgSlot.UserID != "user-1" {
		t.Fatalf("organization slot = %#v, want new-refresh for user-1", orgSlot)
	}
	identitySlot, err := LoadTokenDataKeychainForIdentity("corp-1", "user-1")
	if err != nil {
		t.Fatalf("LoadTokenDataKeychainForIdentity() error = %v", err)
	}
	if identitySlot.RefreshToken != "new-refresh" {
		t.Fatalf("identity slot = %#v, want new-refresh", identitySlot)
	}
}

func TestCrossPlatformCoverageLockedRefreshOrgSlotFallbackGuardrails(t *testing.T) {
	t.Run("transient failure does not fall back", func(t *testing.T) {
		f := newOrgSlotFallbackFixture(t)
		f.refreshErr = &HTTPStatusError{StatusCode: http.StatusInternalServerError}

		_, err := f.provider.lockedRefresh(context.Background())
		if err == nil || err.Error() != f.refreshErr.Error() {
			t.Fatalf("lockedRefresh() error = %v, want transient failure", err)
		}
		if f.orgLoads != 0 {
			t.Fatalf("organization slot loads = %d, want 0 for transient failure", f.orgLoads)
		}
		if len(f.refreshCalls) != 1 {
			t.Fatalf("refresh attempts = %v, want only the primary attempt", f.refreshCalls)
		}
	})

	t.Run("missing org slot preserves rejection", func(t *testing.T) {
		f := newOrgSlotFallbackFixture(t)
		testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) {
			f.orgLoads++
			return nil, ErrTokenDataNotFound
		})

		_, err := f.provider.lockedRefresh(context.Background())
		if !errors.Is(err, f.rejected) {
			t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
		}
		if len(f.refreshCalls) != 1 {
			t.Fatalf("refresh attempts = %v, want only the primary attempt", f.refreshCalls)
		}
	})

	t.Run("nil org data from keychain preserves rejection", func(t *testing.T) {
		f := newOrgSlotFallbackFixture(t)
		testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) {
			f.orgLoads++
			return nil, nil
		})

		_, err := f.provider.lockedRefresh(context.Background())
		if !errors.Is(err, f.rejected) {
			t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
		}
		if len(f.refreshCalls) != 1 {
			t.Fatalf("refresh attempts = %v, want only the primary attempt", f.refreshCalls)
		}
	})

	t.Run("org slot refresh failure preserves rejection", func(t *testing.T) {
		f := newOrgSlotFallbackFixture(t)
		f.orgRefreshErr = fmt.Errorf("org mirror refresh failed")

		_, err := f.provider.lockedRefresh(context.Background())
		if !errors.Is(err, f.rejected) {
			t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
		}
		if len(f.refreshCalls) != 2 {
			t.Fatalf("refresh attempts = %v, want primary and fallback attempts", f.refreshCalls)
		}
	})

	t.Run("different user in org slot is rejected", func(t *testing.T) {
		f := newOrgSlotFallbackFixture(t)
		f.orgMirror.UserID = "user-2"

		_, err := f.provider.lockedRefresh(context.Background())
		if !errors.Is(err, f.rejected) {
			t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
		}
		if len(f.refreshCalls) != 1 {
			t.Fatalf("refresh attempts = %v, mismatched user must not refresh", f.refreshCalls)
		}
	})

	t.Run("empty org slot user identity is backfilled", func(t *testing.T) {
		f := newOrgSlotFallbackFixture(t)
		f.orgMirror.UserID = ""
		f.orgMirror.UserName = ""

		got, err := f.provider.lockedRefresh(context.Background())
		if err != nil || got != f.renewed {
			t.Fatalf("lockedRefresh() = %#v, %v; want renewed token, nil", got, err)
		}
		if len(f.refreshCalls) != 2 {
			t.Fatalf("refresh attempts = %v, want fallback attempt", f.refreshCalls)
		}
		if f.refreshUserIDs[1] != "user-1" {
			t.Fatalf("fallback refresh UserID = %q, want backfilled current identity user-1", f.refreshUserIDs[1])
		}
	})

	t.Run("same rejected refresh token is skipped", func(t *testing.T) {
		f := newOrgSlotFallbackFixture(t)
		f.orgMirror.RefreshToken = "stale-refresh"

		_, err := f.provider.lockedRefresh(context.Background())
		if !errors.Is(err, f.rejected) {
			t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
		}
		if len(f.refreshCalls) != 1 {
			t.Fatalf("refresh attempts = %v, retrying the rejected token must not run", f.refreshCalls)
		}
	})

	t.Run("expired org refresh token is skipped", func(t *testing.T) {
		f := newOrgSlotFallbackFixture(t)
		f.orgMirror.RefreshExpAt = time.Now().Add(-time.Hour)

		_, err := f.provider.lockedRefresh(context.Background())
		if !errors.Is(err, f.rejected) {
			t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
		}
		if len(f.refreshCalls) != 1 {
			t.Fatalf("refresh attempts = %v, want only the primary attempt", f.refreshCalls)
		}
	})

	t.Run("missing corp id skips fallback", func(t *testing.T) {
		f := newOrgSlotFallbackFixture(t)
		f.stale.CorpID = ""

		_, err := f.provider.lockedRefresh(context.Background())
		if !errors.Is(err, f.rejected) {
			t.Fatalf("lockedRefresh() error = %v, want original rejection", err)
		}
		if f.orgLoads != 0 {
			t.Fatalf("organization slot loads = %d, want 0 without corpId", f.orgLoads)
		}
	})

	t.Run("direct mode terminal status does not fall back without business code", func(t *testing.T) {
		f := newOrgSlotFallbackFixture(t)
		f.refreshErr = &HTTPStatusError{StatusCode: http.StatusBadRequest}

		_, err := f.provider.lockedRefresh(context.Background())
		if err == nil || err.Error() != f.refreshErr.Error() {
			t.Fatalf("lockedRefresh() error = %v, want direct terminal status", err)
		}
		if f.orgLoads != 0 {
			t.Fatalf("fallback slot loads = %d, want 0 without reviewed business code", f.orgLoads)
		}
		if len(f.refreshCalls) != 1 {
			t.Fatalf("refresh attempts = %v, want only the primary attempt", f.refreshCalls)
		}
	})
}

func TestCrossPlatformCoverageRefreshFromOrgSlotBoundaries(t *testing.T) {
	f := newOrgSlotFallbackFixture(t)

	if _, err := f.provider.refreshFromOrgSlot(context.Background(), nil); err == nil {
		t.Fatal("refreshFromOrgSlot(nil) succeeded")
	}
	f.orgMirror.CorpID = "corp-2"
	if _, err := f.provider.refreshFromOrgSlot(context.Background(), f.stale); err == nil {
		t.Fatal("refreshFromOrgSlot with mismatched corpId succeeded")
	}
	if len(f.refreshCalls) != 0 {
		t.Fatalf("refresh attempts = %v, corpId mismatch must not refresh", f.refreshCalls)
	}
}
