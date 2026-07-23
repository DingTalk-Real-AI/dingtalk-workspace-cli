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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const rejectedTokenRefreshFailureCooldown = 3 * time.Second

type rejectedTokenRefreshKey struct {
	configDir   string
	profile     string
	tokenDigest [sha256.Size]byte
}

type rejectedTokenRefreshCall struct {
	done         chan struct{}
	participants int
	token        string
	err          error
}

type rejectedTokenRefreshFailure struct {
	at  time.Time
	err error
}

var rejectedTokenRefreshCoordinator = struct {
	sync.Mutex
	inFlight map[rejectedTokenRefreshKey]*rejectedTokenRefreshCall
	failures map[rejectedTokenRefreshKey]rejectedTokenRefreshFailure
	now      func() time.Time
}{
	inFlight: make(map[rejectedTokenRefreshKey]*rejectedTokenRefreshCall),
	failures: make(map[rejectedTokenRefreshKey]rejectedTokenRefreshFailure),
	now:      time.Now,
}

// MarkAccessTokenStale loads the persisted TokenData, sets ExpiresAt to a past
// instant (preserving access_token and refresh_token), and writes it back. The
// next OAuthProvider.GetAccessToken call will see IsAccessTokenValid() == false
// and proceed to lockedRefresh, exchanging the refresh_token for a fresh
// access_token.
//
// Use this only when the server has rejected the current access_token but the
// local expiry has not yet elapsed (zombie token scenario). It does not delete
// any token material and is safe to call concurrently — actual refresh is
// serialized by lockedRefresh's dual-layer locking.
//
// Returns the original load error when there is no usable token on disk; a
// nil error when there is no access_token to invalidate (no-op).
func MarkAccessTokenStale(configDir string) error {
	data, err := LoadTokenData(configDir)
	if err != nil {
		return err
	}
	if data == nil || data.AccessToken == "" {
		return nil
	}
	data.ExpiresAt = time.Now().Add(-1 * time.Minute)
	return SaveTokenData(configDir, data)
}

// ForceRefreshRejectedToken refreshes rejectedAccessToken only while it is
// still the credential stored for the active profile. The compare and refresh
// run under the same process + file lock used by ordinary expiry refresh, so a
// late rejection cannot invalidate or refresh over a token another caller has
// already rotated.
//
// When the stored token no longer matches, the newer token is returned without
// calling the refresh endpoint. Refresh failures leave the stored credential in
// place; login/logout remain the only owners of credential deletion.
func (p *OAuthProvider) ForceRefreshRejectedToken(ctx context.Context, rejectedAccessToken string) (string, error) {
	if p == nil || strings.TrimSpace(p.configDir) == "" {
		return "", fmt.Errorf("config directory is empty")
	}
	rejectedAccessToken = strings.TrimSpace(rejectedAccessToken)
	if rejectedAccessToken == "" {
		return "", fmt.Errorf("rejected access token is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	profile := strings.TrimSpace(RuntimeProfile())
	key := newRejectedTokenRefreshKey(p.configDir, profile, rejectedAccessToken)
	call, leader := beginRejectedTokenRefresh(key)
	if !leader {
		select {
		case <-call.done:
			return call.token, call.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	token, err, recordFailure := p.forceRefreshRejectedTokenOnce(ctx, profile, rejectedAccessToken, key)
	finishRejectedTokenRefresh(key, call, token, err, recordFailure)
	return token, err
}

func (p *OAuthProvider) forceRefreshRejectedTokenOnce(
	ctx context.Context,
	profile string,
	rejectedAccessToken string,
	key rejectedTokenRefreshKey,
) (string, error, bool) {
	lock, err := oauthAcquireLock(ctx, p.configDir)
	if err != nil {
		return "", fmt.Errorf("acquiring dual lock: %w", err), false
	}
	defer lock.Release()

	data, err := loadOAuthTokenUnderHeldLock(p.configDir, profile)
	if err != nil {
		return "", fmt.Errorf("reload rejected token: %w", err), false
	}
	current := strings.TrimSpace(data.AccessToken)
	if current == "" {
		clearRejectedTokenRefreshFailure(key)
		return "", fmt.Errorf("stored access token is empty"), false
	}
	if current != rejectedAccessToken {
		clearRejectedTokenRefreshFailure(key)
		return current, nil, false
	}
	if cachedErr := recentRejectedTokenRefreshFailure(key); cachedErr != nil {
		return "", cachedErr, false
	}
	if !data.IsRefreshTokenValid() {
		return "", fmt.Errorf("refresh_token 已过期"), true
	}
	if err := preflightTokenRefreshPersistence(p.configDir, data); err != nil {
		return "", fmt.Errorf("本地登录态无法安全更新: %w", err), true
	}

	refreshed, err := oauthRefreshToken(p, ctx, data)
	if err != nil {
		return "", err, true
	}
	if refreshed == nil || strings.TrimSpace(refreshed.AccessToken) == "" {
		return "", fmt.Errorf("force refresh returned empty access token"), true
	}
	return strings.TrimSpace(refreshed.AccessToken), nil, false
}

func newRejectedTokenRefreshKey(configDir, profile, rejectedAccessToken string) rejectedTokenRefreshKey {
	canonicalDir := filepath.Clean(configDir)
	if absolute, err := filepath.Abs(configDir); err == nil {
		canonicalDir = filepath.Clean(absolute)
	}
	return rejectedTokenRefreshKey{
		configDir:   canonicalDir,
		profile:     strings.TrimSpace(profile),
		tokenDigest: sha256.Sum256([]byte(strings.TrimSpace(rejectedAccessToken))),
	}
}

func beginRejectedTokenRefresh(key rejectedTokenRefreshKey) (*rejectedTokenRefreshCall, bool) {
	rejectedTokenRefreshCoordinator.Lock()
	defer rejectedTokenRefreshCoordinator.Unlock()

	if call := rejectedTokenRefreshCoordinator.inFlight[key]; call != nil {
		call.participants++
		return call, false
	}
	call := &rejectedTokenRefreshCall{done: make(chan struct{}), participants: 1}
	rejectedTokenRefreshCoordinator.inFlight[key] = call
	return call, true
}

func finishRejectedTokenRefresh(
	key rejectedTokenRefreshKey,
	call *rejectedTokenRefreshCall,
	token string,
	err error,
	recordFailure bool,
) {
	rejectedTokenRefreshCoordinator.Lock()
	defer rejectedTokenRefreshCoordinator.Unlock()

	call.token = token
	call.err = err
	delete(rejectedTokenRefreshCoordinator.inFlight, key)
	if err == nil {
		delete(rejectedTokenRefreshCoordinator.failures, key)
	} else if recordFailure && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		now := time.Now()
		if rejectedTokenRefreshCoordinator.now != nil {
			now = rejectedTokenRefreshCoordinator.now()
		}
		rejectedTokenRefreshCoordinator.failures[key] = rejectedTokenRefreshFailure{at: now, err: err}
	}
	close(call.done)
}

func recentRejectedTokenRefreshFailure(key rejectedTokenRefreshKey) error {
	rejectedTokenRefreshCoordinator.Lock()
	defer rejectedTokenRefreshCoordinator.Unlock()

	now := time.Now()
	if rejectedTokenRefreshCoordinator.now != nil {
		now = rejectedTokenRefreshCoordinator.now()
	}
	for failureKey, failure := range rejectedTokenRefreshCoordinator.failures {
		age := now.Sub(failure.at)
		if age < 0 || age >= rejectedTokenRefreshFailureCooldown {
			delete(rejectedTokenRefreshCoordinator.failures, failureKey)
		}
	}
	if failure, ok := rejectedTokenRefreshCoordinator.failures[key]; ok {
		return failure.err
	}
	return nil
}

func clearRejectedTokenRefreshFailure(key rejectedTokenRefreshKey) {
	rejectedTokenRefreshCoordinator.Lock()
	delete(rejectedTokenRefreshCoordinator.failures, key)
	rejectedTokenRefreshCoordinator.Unlock()
}

// loadOAuthTokenUnderHeldLock mirrors LoadTokenDataForProfile without taking a
// second, non-reentrant auth lock. Opaque edition storage hooks (for example
// Wukong's encrypted .data file) are read inside the caller's dual lock so the
// compare-and-refresh decision covers both Core and embedded storage.
func loadOAuthTokenUnderHeldLock(configDir, profile string) (*TokenData, error) {
	hooks := edition.Get()
	if hooks.LoadToken == nil {
		data, err := oauthLoadTokenLocked(configDir, profile)
		if err != nil {
			return nil, err
		}
		if data == nil {
			return nil, fmt.Errorf("stored token data is empty")
		}
		return data, nil
	}
	if strings.TrimSpace(profile) != "" {
		return nil, fmt.Errorf("profile selection is not supported by the current auth backend")
	}
	blob, err := hooks.LoadToken(configDir)
	if err != nil {
		return nil, err
	}
	var data TokenData
	if err := json.Unmarshal(blob, &data); err != nil {
		return nil, fmt.Errorf("parsing token data from hook: %w", err)
	}
	return &data, nil
}
