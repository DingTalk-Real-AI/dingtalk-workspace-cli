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
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type rejectedTokenHookStore struct {
	mu      sync.Mutex
	data    TokenData
	deletes int
}

func (s *rejectedTokenHookStore) load(string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Marshal(s.data)
}

func (s *rejectedTokenHookStore) save(_ string, blob []byte) error {
	var data TokenData
	if err := json.Unmarshal(blob, &data); err != nil {
		return err
	}
	s.mu.Lock()
	s.data = data
	s.mu.Unlock()
	return nil
}

func (s *rejectedTokenHookStore) delete(string) error {
	s.mu.Lock()
	s.data = TokenData{}
	s.deletes++
	s.mu.Unlock()
	return nil
}

func (s *rejectedTokenHookStore) snapshot() (TokenData, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data, s.deletes
}

func installRejectedTokenHookStore(t *testing.T, data TokenData) *rejectedTokenHookStore {
	t.Helper()
	store := &rejectedTokenHookStore{data: data}
	previousHooks := edition.Get()
	edition.Override(&edition.Hooks{
		LoadToken:   store.load,
		SaveToken:   store.save,
		DeleteToken: store.delete,
	})
	t.Cleanup(func() { edition.Override(previousHooks) })
	return store
}

func installOAuthRefreshStub(t *testing.T, fn func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error)) {
	t.Helper()
	resetRejectedTokenRefreshCoordinator(t)
	previous := oauthRefreshToken
	oauthRefreshToken = fn
	t.Cleanup(func() { oauthRefreshToken = previous })
}

func resetRejectedTokenRefreshCoordinator(t *testing.T) {
	t.Helper()
	reset := func() {
		rejectedTokenRefreshCoordinator.Lock()
		rejectedTokenRefreshCoordinator.inFlight = make(map[rejectedTokenRefreshKey]*rejectedTokenRefreshCall)
		rejectedTokenRefreshCoordinator.failures = make(map[rejectedTokenRefreshKey]rejectedTokenRefreshFailure)
		rejectedTokenRefreshCoordinator.now = time.Now
		rejectedTokenRefreshCoordinator.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func waitForRejectedTokenRefreshParticipants(t *testing.T, key rejectedTokenRefreshKey, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		rejectedTokenRefreshCoordinator.Lock()
		call := rejectedTokenRefreshCoordinator.inFlight[key]
		got := 0
		if call != nil {
			got = call.participants
		}
		rejectedTokenRefreshCoordinator.Unlock()
		if got >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("refresh participants = %d, want %d", got, want)
		}
		time.Sleep(time.Millisecond)
	}
}

func installProfilesAcquireProbe(t *testing.T) <-chan struct{} {
	t.Helper()
	previous := profilesAcquireDualLock
	attempted := make(chan struct{}, 1)
	profilesAcquireDualLock = func(ctx context.Context, configDir string) (*DualLock, error) {
		attempted <- struct{}{}
		return previous(ctx, configDir)
	}
	t.Cleanup(func() { profilesAcquireDualLock = previous })
	return attempted
}

func waitForProfilesAcquire(t *testing.T, attempted <-chan struct{}) {
	t.Helper()
	select {
	case <-attempted:
	case <-time.After(2 * time.Second):
		t.Fatal("public opaque token mutation did not enter the Core dual lock")
	}
}

func validRejectedTokenData(accessToken string) TokenData {
	return TokenData{
		AccessToken:  accessToken,
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		Source:       "mcp",
		ClientID:     "client-id",
	}
}

func TestCrossPlatformCoverageForceRefreshRejectedTokenConcurrentCallersExchangeOnce(t *testing.T) {
	store := installRejectedTokenHookStore(t, validRejectedTokenData("old-access"))
	var refreshCalls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	installOAuthRefreshStub(t, func(p *OAuthProvider, _ context.Context, data *TokenData) (*TokenData, error) {
		if refreshCalls.Add(1) == 1 {
			close(started)
		}
		<-release
		updated := *data
		updated.AccessToken = "new-access"
		updated.ExpiresAt = time.Now().Add(time.Hour)
		if err := saveTokenDataLocked(p.configDir, &updated); err != nil {
			return nil, err
		}
		return &updated, nil
	})

	provider := NewOAuthProvider(t.TempDir(), nil)
	const workers = 8
	results := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			token, err := provider.ForceRefreshRejectedToken(context.Background(), "old-access")
			results <- token
			errs <- err
		}()
	}
	<-started
	close(release)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for token := range results {
		if token != "new-access" {
			t.Fatalf("token = %q, want new-access", token)
		}
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	stored, deletes := store.snapshot()
	if stored.AccessToken != "new-access" || deletes != 0 {
		t.Fatalf("stored token = %q, deletes = %d", stored.AccessToken, deletes)
	}
}

func TestCrossPlatformCoverageForceRefreshRejectedTokenFailurePreservesCredential(t *testing.T) {
	store := installRejectedTokenHookStore(t, validRejectedTokenData("old-access"))
	refreshErr := errors.New("temporary refresh failure")
	installOAuthRefreshStub(t, func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) {
		return nil, refreshErr
	})

	_, err := NewOAuthProvider(t.TempDir(), nil).ForceRefreshRejectedToken(context.Background(), "old-access")
	if !errors.Is(err, refreshErr) {
		t.Fatalf("error = %v, want refresh cause", err)
	}
	stored, deletes := store.snapshot()
	if stored.AccessToken != "old-access" || stored.RefreshToken != "refresh-token" || deletes != 0 {
		t.Fatalf("credential changed after transient failure: %#v, deletes=%d", stored, deletes)
	}
}

func TestCrossPlatformCoverageForceRefreshRejectedTokenFailureIsSingleflightAndCooledDown(t *testing.T) {
	store := installRejectedTokenHookStore(t, validRejectedTokenData("old-access"))
	previousProfile := RuntimeProfile()
	SetRuntimeProfile("")
	t.Cleanup(func() { SetRuntimeProfile(previousProfile) })

	refreshErr := errors.New("temporary refresh failure")
	var refreshCalls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	baseNow := time.Now()
	var nowNanos atomic.Int64
	nowNanos.Store(baseNow.UnixNano())
	installOAuthRefreshStub(t, func(p *OAuthProvider, _ context.Context, data *TokenData) (*TokenData, error) {
		call := refreshCalls.Add(1)
		if call == 1 {
			close(started)
			<-release
			return nil, refreshErr
		}
		updated := *data
		updated.AccessToken = "recovered-access"
		updated.ExpiresAt = time.Now().Add(time.Hour)
		if err := saveTokenDataLocked(p.configDir, &updated); err != nil {
			return nil, err
		}
		return &updated, nil
	})
	rejectedTokenRefreshCoordinator.Lock()
	rejectedTokenRefreshCoordinator.now = func() time.Time {
		return time.Unix(0, nowNanos.Load())
	}
	rejectedTokenRefreshCoordinator.Unlock()

	configDir := t.TempDir()
	provider := NewOAuthProvider(configDir, nil)
	const workers = 8
	start := make(chan struct{})
	ready := make(chan struct{}, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			_, err := provider.ForceRefreshRejectedToken(context.Background(), "old-access")
			errs <- err
		}()
	}
	for range workers {
		<-ready
	}
	close(start)
	<-started
	key := newRejectedTokenRefreshKey(configDir, "", "old-access")
	waitForRejectedTokenRefreshParticipants(t, key, workers)
	close(release)
	wg.Wait()
	close(errs)

	for err := range errs {
		if !errors.Is(err, refreshErr) {
			t.Fatalf("shared refresh error = %v, want %v", err, refreshErr)
		}
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls after concurrent failure = %d, want 1", got)
	}
	stored, deletes := store.snapshot()
	if stored.AccessToken != "old-access" || stored.RefreshToken != "refresh-token" || deletes != 0 {
		t.Fatalf("credential changed after shared failure: %#v, deletes=%d", stored, deletes)
	}

	if _, err := provider.ForceRefreshRejectedToken(context.Background(), "old-access"); !errors.Is(err, refreshErr) {
		t.Fatalf("cooldown error = %v, want %v", err, refreshErr)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls inside cooldown = %d, want 1", got)
	}

	nowNanos.Store(baseNow.Add(rejectedTokenRefreshFailureCooldown + time.Nanosecond).UnixNano())
	token, err := provider.ForceRefreshRejectedToken(context.Background(), "old-access")
	if err != nil || token != "recovered-access" {
		t.Fatalf("refresh after cooldown = %q, %v", token, err)
	}
	if got := refreshCalls.Load(); got != 2 {
		t.Fatalf("refresh calls after cooldown = %d, want 2", got)
	}
	stored, deletes = store.snapshot()
	if stored.AccessToken != "recovered-access" || deletes != 0 {
		t.Fatalf("stored token after cooldown recovery = %q, deletes=%d", stored.AccessToken, deletes)
	}
}

func TestCrossPlatformCoverageForceRefreshRejectedTokenChangedDuringCooldownUsesNewToken(t *testing.T) {
	store := installRejectedTokenHookStore(t, validRejectedTokenData("old-access"))
	previousProfile := RuntimeProfile()
	SetRuntimeProfile("")
	t.Cleanup(func() { SetRuntimeProfile(previousProfile) })

	refreshErr := errors.New("temporary refresh failure")
	var refreshCalls atomic.Int32
	baseNow := time.Now()
	installOAuthRefreshStub(t, func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) {
		refreshCalls.Add(1)
		return nil, refreshErr
	})
	rejectedTokenRefreshCoordinator.Lock()
	rejectedTokenRefreshCoordinator.now = func() time.Time { return baseNow }
	rejectedTokenRefreshCoordinator.Unlock()

	configDir := t.TempDir()
	provider := NewOAuthProvider(configDir, nil)
	if _, err := provider.ForceRefreshRejectedToken(context.Background(), "old-access"); !errors.Is(err, refreshErr) {
		t.Fatalf("initial refresh error = %v, want %v", err, refreshErr)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("initial refresh calls = %d, want 1", got)
	}

	store.mu.Lock()
	store.data = validRejectedTokenData("externally-refreshed")
	store.mu.Unlock()
	token, err := provider.ForceRefreshRejectedToken(context.Background(), "old-access")
	if err != nil || token != "externally-refreshed" {
		t.Fatalf("refresh after external publication = %q, %v", token, err)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("external publication triggered another exchange: calls=%d", got)
	}
	key := newRejectedTokenRefreshKey(configDir, "", "old-access")
	rejectedTokenRefreshCoordinator.Lock()
	_, failurePresent := rejectedTokenRefreshCoordinator.failures[key]
	rejectedTokenRefreshCoordinator.Unlock()
	if failurePresent {
		t.Fatal("old-token failure cache was not cleared after external publication")
	}
}

func TestCrossPlatformCoverageOpaquePublisherWaitsForRejectedTokenRefresh(t *testing.T) {
	store := installRejectedTokenHookStore(t, validRejectedTokenData("old-access"))
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	installOAuthRefreshStub(t, func(p *OAuthProvider, _ context.Context, data *TokenData) (*TokenData, error) {
		close(started)
		<-release
		updated := *data
		updated.AccessToken = "refreshed-from-old"
		updated.ExpiresAt = time.Now().Add(time.Hour)
		if err := saveTokenDataLocked(p.configDir, &updated); err != nil {
			return nil, err
		}
		return &updated, nil
	})

	configDir := t.TempDir()
	provider := NewOAuthProvider(configDir, nil)
	refreshResult := make(chan struct {
		token string
		err   error
	}, 1)
	go func() {
		token, err := provider.ForceRefreshRejectedToken(context.Background(), "old-access")
		refreshResult <- struct {
			token string
			err   error
		}{token: token, err: err}
	}()
	<-started

	acquireAttempted := installProfilesAcquireProbe(t)
	publishResult := make(chan error, 1)
	go func() {
		publishResult <- SaveTokenData(configDir, ptrTokenData(validRejectedTokenData("login-published")))
	}()
	waitForProfilesAcquire(t, acquireAttempted)
	releaseOnce.Do(func() { close(release) })

	refresh := <-refreshResult
	if refresh.err != nil || refresh.token != "refreshed-from-old" {
		t.Fatalf("refresh result = %q, %v", refresh.token, refresh.err)
	}
	if err := <-publishResult; err != nil {
		t.Fatalf("publish token: %v", err)
	}
	stored, deletes := store.snapshot()
	if stored.AccessToken != "login-published" || deletes != 0 {
		t.Fatalf("older refresh overwrote login publication: token=%q deletes=%d", stored.AccessToken, deletes)
	}
}

func TestCrossPlatformCoverageOpaqueLogoutWaitsForRejectedTokenRefresh(t *testing.T) {
	for _, tc := range []struct {
		name   string
		logout func(string) error
	}{
		{name: "current profile", logout: func(configDir string) error {
			return DeleteTokenDataForProfile(configDir, "")
		}},
		{name: "all profiles", logout: DeleteAllTokenData},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := installRejectedTokenHookStore(t, validRejectedTokenData("old-access"))
			started := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			installOAuthRefreshStub(t, func(p *OAuthProvider, _ context.Context, data *TokenData) (*TokenData, error) {
				close(started)
				<-release
				updated := *data
				updated.AccessToken = "refreshed-before-logout"
				updated.ExpiresAt = time.Now().Add(time.Hour)
				if err := saveTokenDataLocked(p.configDir, &updated); err != nil {
					return nil, err
				}
				return &updated, nil
			})

			configDir := t.TempDir()
			provider := NewOAuthProvider(configDir, nil)
			refreshResult := make(chan error, 1)
			go func() {
				_, err := provider.ForceRefreshRejectedToken(context.Background(), "old-access")
				refreshResult <- err
			}()
			<-started

			acquireAttempted := installProfilesAcquireProbe(t)
			logoutResult := make(chan error, 1)
			go func() { logoutResult <- tc.logout(configDir) }()
			waitForProfilesAcquire(t, acquireAttempted)
			releaseOnce.Do(func() { close(release) })

			if err := <-refreshResult; err != nil {
				t.Fatalf("refresh: %v", err)
			}
			if err := <-logoutResult; err != nil {
				t.Fatalf("logout: %v", err)
			}
			stored, deletes := store.snapshot()
			if stored.AccessToken != "" || stored.RefreshToken != "" || deletes != 1 {
				t.Fatalf("refresh resurrected logged-out credential: %#v deletes=%d", stored, deletes)
			}
		})
	}
}

func ptrTokenData(data TokenData) *TokenData {
	return &data
}

func TestCrossPlatformCoverageOAuthLockedRefreshReadsOpaqueEditionStore(t *testing.T) {
	data := validRejectedTokenData("expired-access")
	data.ExpiresAt = time.Now().Add(-time.Hour)
	store := installRejectedTokenHookStore(t, data)
	var refreshCalls atomic.Int32
	installOAuthRefreshStub(t, func(p *OAuthProvider, _ context.Context, current *TokenData) (*TokenData, error) {
		refreshCalls.Add(1)
		updated := *current
		updated.AccessToken = "proactively-refreshed"
		updated.ExpiresAt = time.Now().Add(time.Hour)
		if err := saveTokenDataLocked(p.configDir, &updated); err != nil {
			return nil, err
		}
		return &updated, nil
	})

	token, err := NewOAuthProvider(t.TempDir(), nil).GetAccessToken(context.Background())
	if err != nil || token != "proactively-refreshed" {
		t.Fatalf("GetAccessToken() = %q, %v", token, err)
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls.Load())
	}
	stored, deletes := store.snapshot()
	if stored.AccessToken != "proactively-refreshed" || deletes != 0 {
		t.Fatalf("stored token = %q, deletes = %d", stored.AccessToken, deletes)
	}
}
