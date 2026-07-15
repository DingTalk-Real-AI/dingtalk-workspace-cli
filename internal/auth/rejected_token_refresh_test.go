package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type rejectedTokenRoundTripper func(*http.Request) (*http.Response, error)

func (fn rejectedTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestForceRefreshRejectedTokenConcurrentCallersExchangeOnce(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	setPreflightTestCredentials(t)
	configDir := t.TempDir()
	data := testToken("old-access", "corp-concurrent-refresh", "Concurrent Refresh Org")
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	var exchanges atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.httpClient = &http.Client{Transport: rejectedTokenRoundTripper(func(*http.Request) (*http.Response, error) {
		exchanges.Add(1)
		time.Sleep(10 * time.Millisecond)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"accessToken":"new-access","refreshToken":"new-refresh","expiresIn":7200,"corpId":"corp-concurrent-refresh"}`,
			)),
		}, nil
	})}

	const callers = 20
	start := make(chan struct{})
	results := make(chan string, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			token, err := provider.ForceRefreshRejectedToken(context.Background(), "old-access")
			results <- token
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("ForceRefreshRejectedToken() error = %v", err)
		}
	}
	for token := range results {
		if token != "new-access" {
			t.Fatalf("ForceRefreshRejectedToken() token = %q, want new-access", token)
		}
	}
	if got := exchanges.Load(); got != 1 {
		t.Fatalf("refresh exchanges = %d, want 1", got)
	}
}

func TestForceRefreshRejectedTokenReusesAlreadyRotatedToken(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	if err := SaveTokenData(configDir, testToken("already-new", "corp-already-rotated", "Already Rotated Org")); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	var exchanges atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.httpClient = &http.Client{Transport: rejectedTokenRoundTripper(func(*http.Request) (*http.Response, error) {
		exchanges.Add(1)
		return nil, errors.New("unexpected refresh exchange")
	})}

	token, err := provider.ForceRefreshRejectedToken(context.Background(), "old-rejected")
	if err != nil {
		t.Fatalf("ForceRefreshRejectedToken() error = %v", err)
	}
	if token != "already-new" {
		t.Fatalf("ForceRefreshRejectedToken() token = %q, want already-new", token)
	}
	if got := exchanges.Load(); got != 0 {
		t.Fatalf("refresh exchanges = %d, want 0", got)
	}
}

func TestDeleteTokenDataIfAccessTokenMatchesDeletesMatchingCredential(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	if err := SaveTokenData(configDir, testToken("rejected-access", "corp-delete-match", "Delete Match Org")); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	deleted, err := DeleteTokenDataIfAccessTokenMatches(context.Background(), configDir, "rejected-access")
	if err != nil {
		t.Fatalf("DeleteTokenDataIfAccessTokenMatches() error = %v", err)
	}
	if !deleted {
		t.Fatal("DeleteTokenDataIfAccessTokenMatches() deleted = false, want true")
	}
	if _, err := LoadTokenData(configDir); err == nil {
		t.Fatal("matching credential still exists after conditional delete")
	}
}

func TestDeleteTokenDataIfAccessTokenMatchesPreservesDifferentCredential(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	if err := SaveTokenData(configDir, testToken("rotated-access", "corp-delete-mismatch", "Delete Mismatch Org")); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	deleted, err := DeleteTokenDataIfAccessTokenMatches(context.Background(), configDir, "rejected-access")
	if err != nil {
		t.Fatalf("DeleteTokenDataIfAccessTokenMatches() error = %v", err)
	}
	if deleted {
		t.Fatal("DeleteTokenDataIfAccessTokenMatches() deleted = true, want false")
	}
	stored, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if stored.AccessToken != "rotated-access" {
		t.Fatalf("persisted access token = %q, want rotated-access", stored.AccessToken)
	}
}

func TestDeleteTokenDataIfAccessTokenMatchesConcurrentRotationPreservesNewToken(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	const iterations = 50
	for i := range iterations {
		corpID := "corp-delete-race"
		if err := SaveTokenData(configDir, testToken("rejected-access", corpID, "Delete Race Org")); err != nil {
			t.Fatalf("iteration %d SaveTokenData(old) error = %v", i, err)
		}

		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, err := DeleteTokenDataIfAccessTokenMatches(context.Background(), configDir, "rejected-access")
			errs <- err
		}()
		go func() {
			defer wg.Done()
			<-start
			errs <- SaveTokenData(configDir, testToken("rotated-access", corpID, "Delete Race Org"))
		}()
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("iteration %d concurrent operation error = %v", i, err)
			}
		}

		stored, err := LoadTokenData(configDir)
		if err != nil {
			t.Fatalf("iteration %d LoadTokenData() error = %v", i, err)
		}
		if stored.AccessToken != "rotated-access" {
			t.Fatalf("iteration %d persisted access token = %q, want rotated-access", i, stored.AccessToken)
		}
	}
}

func TestDeleteTokenDataIfAccessTokenMatchesHookBackend(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()

	var storeMu sync.Mutex
	var stored []byte
	deleteCalls := 0
	previousHooks := edition.Get()
	edition.Override(&edition.Hooks{
		SaveToken: func(_ string, data []byte) error {
			storeMu.Lock()
			defer storeMu.Unlock()
			stored = append([]byte(nil), data...)
			return nil
		},
		LoadToken: func(string) ([]byte, error) {
			storeMu.Lock()
			defer storeMu.Unlock()
			if len(stored) == 0 {
				return nil, ErrTokenDataNotFound
			}
			return append([]byte(nil), stored...), nil
		},
		DeleteToken: func(string) error {
			storeMu.Lock()
			defer storeMu.Unlock()
			deleteCalls++
			stored = nil
			return nil
		},
	})
	t.Cleanup(func() { edition.Override(previousHooks) })

	callWithDeadlockGuard := func(expectedAccessToken string) (bool, error) {
		t.Helper()
		type result struct {
			deleted bool
			err     error
		}
		done := make(chan result, 1)
		go func() {
			deleted, err := DeleteTokenDataIfAccessTokenMatches(context.Background(), configDir, expectedAccessToken)
			done <- result{deleted: deleted, err: err}
		}()
		select {
		case got := <-done:
			return got.deleted, got.err
		case <-time.After(2 * time.Second):
			t.Fatal("DeleteTokenDataIfAccessTokenMatches() deadlocked with hook-backed storage")
			return false, nil
		}
	}

	if err := SaveTokenData(configDir, testToken("rejected-hook-access", "corp-hook-delete", "Hook Delete Org")); err != nil {
		t.Fatalf("SaveTokenData(matching) error = %v", err)
	}
	deleted, err := callWithDeadlockGuard("rejected-hook-access")
	if err != nil {
		t.Fatalf("DeleteTokenDataIfAccessTokenMatches(matching) error = %v", err)
	}
	if !deleted {
		t.Fatal("DeleteTokenDataIfAccessTokenMatches(matching) deleted = false, want true")
	}
	storeMu.Lock()
	matchingDeleteCalls := deleteCalls
	matchingStoreEmpty := len(stored) == 0
	storeMu.Unlock()
	if matchingDeleteCalls != 1 {
		t.Fatalf("matching DeleteToken hook calls = %d, want 1", matchingDeleteCalls)
	}
	if !matchingStoreEmpty {
		t.Fatal("matching credential still exists in hook-backed storage")
	}

	if err := SaveTokenData(configDir, testToken("rotated-hook-access", "corp-hook-delete", "Hook Delete Org")); err != nil {
		t.Fatalf("SaveTokenData(rotated) error = %v", err)
	}
	deleted, err = callWithDeadlockGuard("rejected-hook-access")
	if err != nil {
		t.Fatalf("DeleteTokenDataIfAccessTokenMatches(rotated) error = %v", err)
	}
	if deleted {
		t.Fatal("DeleteTokenDataIfAccessTokenMatches(rotated) deleted = true, want false")
	}
	storeMu.Lock()
	rotatedDeleteCalls := deleteCalls
	storeMu.Unlock()
	if rotatedDeleteCalls != 1 {
		t.Fatalf("DeleteToken hook calls after mismatch = %d, want 1", rotatedDeleteCalls)
	}
	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData(rotated) error = %v", err)
	}
	if loaded.AccessToken != "rotated-hook-access" {
		t.Fatalf("hook-backed access token = %q, want rotated-hook-access", loaded.AccessToken)
	}
}
