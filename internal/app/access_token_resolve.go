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

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const accessTokenRefreshWindow = 5 * time.Minute

type legacyTokenGetter interface {
	GetToken() (string, string, error)
}

type accessTokenSnapshotGetter interface {
	GetTokenSnapshot(context.Context) (*authpkg.TokenData, error)
}

// AccessTokenSnapshot is the minimal bearer view needed by the process cache.
// Refresh-token material never leaves the auth package.
type AccessTokenSnapshot struct {
	AccessToken string
	ExpiresAt   time.Time
	Source      string
}

type tokenManagerKey struct {
	configDir string
	profile   string
}

type tokenManagerEntry struct {
	mu       sync.Mutex
	snapshot AccessTokenSnapshot
	revision string
}

// TokenManager is the only process cache for user access tokens. Cache entries
// are isolated by config directory and profile, expiry-aware, and invalidated
// by the credential publication marker written by auth storage.
type TokenManager struct {
	mu      sync.Mutex
	entries map[tokenManagerKey]*tokenManagerEntry
	now     func() time.Time
}

func NewTokenManager() *TokenManager {
	return &TokenManager{entries: make(map[tokenManagerKey]*tokenManagerEntry), now: time.Now}
}

var runtimeTokenManager = NewTokenManager()

var (
	newAccessTokenProvider = func(configDir string) accessTokenGetter {
		discard := slog.New(slog.NewTextHandler(io.Discard, nil))
		provider := authpkg.NewOAuthProvider(configDir, discard)
		configureOAuthProviderCompatibility(provider, configDir)
		return provider
	}
	newLegacyTokenManager = func(configDir string) legacyTokenGetter {
		manager := authpkg.NewManager(configDir, nil)
		configureLegacyAuthManagerCompatibility(manager)
		return manager
	}
)

// Get resolves an access token for the active runtime profile.
func (m *TokenManager) Get(ctx context.Context, configDir, explicitToken string) (AccessTokenSnapshot, error) {
	if token := strings.TrimSpace(explicitToken); token != "" {
		return AccessTokenSnapshot{AccessToken: token, Source: "explicit"}, nil
	}
	if strings.TrimSpace(configDir) == "" {
		return AccessTokenSnapshot{}, fmt.Errorf("config directory is empty")
	}
	key := tokenManagerKey{
		configDir: canonicalTokenConfigDir(configDir),
		profile:   strings.TrimSpace(authpkg.RuntimeProfile()),
	}
	entry := m.entry(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := time.Now()
	if m != nil && m.now != nil {
		now = m.now()
	}
	revision, present, err := authpkg.ReadTokenMarkerRevision(configDir)
	if err != nil {
		return AccessTokenSnapshot{}, err
	}
	if tokenSnapshotUsable(entry.snapshot, now) && present && revision != "" && revision == entry.revision {
		return entry.snapshot, nil
	}

	// Treat the marker and credential as one optimistic snapshot. A concurrent
	// login/refresh between the reads causes a retry instead of caching stale A
	// under the publication marker for B.
	for attempt := 0; attempt < 4; attempt++ {
		beforeRevision, beforePresent, err := authpkg.ReadTokenMarkerRevision(configDir)
		if err != nil {
			return AccessTokenSnapshot{}, err
		}
		snapshot, err := resolveTokenSnapshotWithEdition(ctx, configDir, key.profile)
		if err != nil {
			return AccessTokenSnapshot{}, err
		}
		afterRevision, afterPresent, err := authpkg.ReadTokenMarkerRevision(configDir)
		if err != nil {
			return AccessTokenSnapshot{}, err
		}
		if beforePresent != afterPresent || beforeRevision != afterRevision {
			continue
		}
		if strings.TrimSpace(snapshot.AccessToken) == "" {
			return AccessTokenSnapshot{}, noCredentialsError()
		}
		if tokenSnapshotUsable(snapshot, now) && afterPresent && afterRevision != "" {
			entry.snapshot = snapshot
			entry.revision = afterRevision
		} else {
			entry.snapshot = AccessTokenSnapshot{}
			entry.revision = ""
		}
		return snapshot, nil
	}
	return AccessTokenSnapshot{}, fmt.Errorf("token publication changed repeatedly while resolving credentials")
}

func (m *TokenManager) entry(key tokenManagerKey) *tokenManagerEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = make(map[tokenManagerKey]*tokenManagerEntry)
	}
	entry := m.entries[key]
	if entry == nil {
		entry = &tokenManagerEntry{}
		m.entries[key] = entry
	}
	return entry
}

func (m *TokenManager) Invalidate() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.entries = make(map[tokenManagerKey]*tokenManagerEntry)
	m.mu.Unlock()
}

func resolveTokenSnapshotWithEdition(ctx context.Context, configDir, profile string) (AccessTokenSnapshot, error) {
	hooks := edition.Get()
	opaqueStorage := hooks.LoadToken != nil || hooks.SaveToken != nil || hooks.DeleteToken != nil
	provider := hooks.TokenProvider
	if provider == nil {
		snapshot, err := resolveAccessTokenSnapshotFromDir(ctx, configDir, profile)
		if err != nil {
			return AccessTokenSnapshot{}, err
		}
		// Opaque edition storage hooks have no publication-revision contract.
		// Resolve them on every logical request instead of caching a token that
		// may be replaced outside the default auth store.
		if opaqueStorage {
			snapshot.ExpiresAt = time.Time{}
		}
		return snapshot, nil
	}
	var fallbackSnapshot AccessTokenSnapshot
	var fallbackCalled bool
	token, err := provider(ctx, func() (string, error) {
		fallbackCalled = true
		var fallbackErr error
		fallbackSnapshot, fallbackErr = resolveAccessTokenSnapshotFromDir(ctx, configDir, profile)
		if fallbackErr != nil {
			return "", fallbackErr
		}
		return fallbackSnapshot.AccessToken, nil
	})
	if err != nil {
		return AccessTokenSnapshot{}, fmt.Errorf("edition token provider: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return AccessTokenSnapshot{}, noCredentialsError()
	}
	if fallbackCalled && token == fallbackSnapshot.AccessToken {
		if opaqueStorage {
			fallbackSnapshot.ExpiresAt = time.Time{}
		}
		return fallbackSnapshot, nil
	}
	// Edition providers expose no lifetime metadata, so resolve them on every
	// logical request instead of recreating a process-lifetime string cache.
	return AccessTokenSnapshot{AccessToken: token, Source: "edition"}, nil
}

func resolveAccessTokenSnapshotFromDir(ctx context.Context, configDir, profile string) (AccessTokenSnapshot, error) {
	provider := newAccessTokenProvider(configDir)
	if snapshotProvider, ok := provider.(accessTokenSnapshotGetter); ok {
		data, err := snapshotProvider.GetTokenSnapshot(ctx)
		if err == nil && data != nil && strings.TrimSpace(data.AccessToken) != "" {
			return AccessTokenSnapshot{
				AccessToken: strings.TrimSpace(data.AccessToken),
				ExpiresAt:   data.ExpiresAt,
				Source:      "oauth",
			}, nil
		}
		if err != nil && !errors.Is(err, authpkg.ErrTokenDataNotFound) {
			return AccessTokenSnapshot{}, err
		}
		if strings.TrimSpace(profile) != "" {
			return AccessTokenSnapshot{}, authpkg.ErrTokenDataNotFound
		}
		return resolveLegacyToken(configDir, err)
	}

	token, err := provider.GetAccessToken(ctx)
	if err == nil && strings.TrimSpace(token) != "" {
		return AccessTokenSnapshot{AccessToken: strings.TrimSpace(token), Source: "oauth_compat"}, nil
	}
	if err != nil && !errors.Is(err, authpkg.ErrTokenDataNotFound) {
		return AccessTokenSnapshot{}, err
	}
	if strings.TrimSpace(profile) != "" {
		return AccessTokenSnapshot{}, authpkg.ErrTokenDataNotFound
	}
	return resolveLegacyToken(configDir, err)
}

func resolveLegacyToken(configDir string, oauthErr error) (AccessTokenSnapshot, error) {
	token, source, err := newLegacyTokenManager(configDir).GetToken()
	if err == nil && strings.TrimSpace(token) != "" {
		return AccessTokenSnapshot{AccessToken: strings.TrimSpace(token), Source: source}, nil
	}
	if err != nil && !errors.Is(err, authpkg.ErrTokenDataNotFound) {
		return AccessTokenSnapshot{}, err
	}
	if oauthErr != nil {
		return AccessTokenSnapshot{}, oauthErr
	}
	return AccessTokenSnapshot{}, authpkg.ErrTokenDataNotFound
}

func resolveAccessTokenFromDir(ctx context.Context, configDir string) (string, error) {
	snapshot, err := resolveAccessTokenSnapshotFromDir(ctx, configDir, authpkg.RuntimeProfile())
	if err != nil {
		return "", err
	}
	return snapshot.AccessToken, nil
}

// ResolveAuxiliaryAccessToken resolves every non-runner bearer token through
// the same TokenManager used by MCP tool calls.
func ResolveAuxiliaryAccessToken(ctx context.Context, configDir, explicitToken string) (string, error) {
	snapshot, err := runtimeTokenManager.Get(ctx, configDir, explicitToken)
	if err != nil {
		return "", err
	}
	return snapshot.AccessToken, nil
}

func tokenSnapshotUsable(snapshot AccessTokenSnapshot, now time.Time) bool {
	return strings.TrimSpace(snapshot.AccessToken) != "" &&
		!snapshot.ExpiresAt.IsZero() &&
		now.Before(snapshot.ExpiresAt.Add(-accessTokenRefreshWindow))
}

func canonicalTokenConfigDir(configDir string) string {
	if absolute, err := filepath.Abs(configDir); err == nil {
		return filepath.Clean(absolute)
	}
	return filepath.Clean(configDir)
}

func noCredentialsError() error {
	if edition.Get().IsEmbedded {
		return fmt.Errorf("认证信息已失效，请重新认证: %w", authpkg.ErrTokenDataNotFound)
	}
	return fmt.Errorf("no credentials found, run: dws auth login: %w", authpkg.ErrTokenDataNotFound)
}
