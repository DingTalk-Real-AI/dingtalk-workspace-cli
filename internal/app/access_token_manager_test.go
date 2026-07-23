package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type tokenManagerSnapshotProvider struct {
	load func() (*authpkg.TokenData, error)
}

func (p tokenManagerSnapshotProvider) GetAccessToken(context.Context) (string, error) {
	data, err := p.load()
	if err != nil || data == nil {
		return "", err
	}
	return data.AccessToken, nil
}

func (p tokenManagerSnapshotProvider) GetTokenSnapshot(context.Context) (*authpkg.TokenData, error) {
	return p.load()
}

type tokenManagerLegacyGetter struct {
	token string
	err   error
}

func (g tokenManagerLegacyGetter) GetToken() (string, string, error) {
	return g.token, "file", g.err
}

func installTokenManagerFakes(t *testing.T, load func() (*authpkg.TokenData, error)) {
	t.Helper()
	oldProvider, oldLegacy := newAccessTokenProvider, newLegacyTokenManager
	oldEdition := edition.Get()
	edition.Override(&edition.Hooks{})
	newAccessTokenProvider = func(string) accessTokenGetter {
		return tokenManagerSnapshotProvider{load: load}
	}
	newLegacyTokenManager = func(string) legacyTokenGetter {
		return tokenManagerLegacyGetter{err: authpkg.ErrTokenDataNotFound}
	}
	t.Cleanup(func() {
		newAccessTokenProvider, newLegacyTokenManager = oldProvider, oldLegacy
		edition.Override(oldEdition)
	})
}

func TestCrossPlatformCoverageTokenManagerCachesUntilMarkerRevisionChanges(t *testing.T) {
	configDir := t.TempDir()
	if err := authpkg.WriteTokenMarker(configDir); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	token := "token-a"
	installTokenManagerFakes(t, func() (*authpkg.TokenData, error) {
		calls.Add(1)
		return &authpkg.TokenData{AccessToken: token, ExpiresAt: time.Now().Add(time.Hour)}, nil
	})

	manager := NewTokenManager()
	first, err := manager.Get(context.Background(), configDir, "")
	if err != nil || first.AccessToken != "token-a" {
		t.Fatalf("first token = %#v, %v", first, err)
	}
	second, err := manager.Get(context.Background(), configDir, "")
	if err != nil || second.AccessToken != "token-a" || calls.Load() != 1 {
		t.Fatalf("cached token = %#v, %v, calls=%d", second, err, calls.Load())
	}

	token = "token-b"
	if err := authpkg.WriteTokenMarker(configDir); err != nil {
		t.Fatal(err)
	}
	rotated, err := manager.Get(context.Background(), configDir, "")
	if err != nil || rotated.AccessToken != "token-b" || calls.Load() != 2 {
		t.Fatalf("rotated token = %#v, %v, calls=%d", rotated, err, calls.Load())
	}
}

func TestCrossPlatformCoverageTokenManagerDoesNotCacheWithoutExpiryOrRevision(t *testing.T) {
	configDir := t.TempDir()
	var calls atomic.Int32
	installTokenManagerFakes(t, func() (*authpkg.TokenData, error) {
		calls.Add(1)
		return &authpkg.TokenData{AccessToken: "token"}, nil
	})
	manager := NewTokenManager()
	for range 2 {
		if _, err := manager.Get(context.Background(), configDir, ""); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("provider calls = %d, want 2", calls.Load())
	}
}

func TestCrossPlatformCoverageTokenManagerTreatsMalformedMarkerAsUncacheable(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "token.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	installTokenManagerFakes(t, func() (*authpkg.TokenData, error) {
		calls.Add(1)
		return &authpkg.TokenData{AccessToken: "token", ExpiresAt: time.Now().Add(time.Hour)}, nil
	})
	manager := NewTokenManager()
	for range 2 {
		if snapshot, err := manager.Get(context.Background(), configDir, ""); err != nil || snapshot.AccessToken != "token" {
			t.Fatalf("snapshot = %#v, error = %v", snapshot, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("provider calls = %d, want 2", calls.Load())
	}
}

func TestCrossPlatformCoverageTokenManagerDoesNotCacheOpaqueEditionStorageWithProviderFallback(t *testing.T) {
	configDir := t.TempDir()
	if err := authpkg.WriteTokenMarker(configDir); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	installTokenManagerFakes(t, func() (*authpkg.TokenData, error) {
		calls.Add(1)
		return &authpkg.TokenData{AccessToken: "token", ExpiresAt: time.Now().Add(time.Hour)}, nil
	})
	edition.Override(&edition.Hooks{
		LoadToken: func(string) ([]byte, error) { return nil, nil },
		TokenProvider: func(_ context.Context, fallback func() (string, error)) (string, error) {
			return fallback()
		},
	})
	manager := NewTokenManager()
	for range 2 {
		if _, err := manager.Get(context.Background(), configDir, ""); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("provider calls = %d, want 2", calls.Load())
	}
}

func TestCrossPlatformCoverageTokenManagerCoalescesConcurrentLoads(t *testing.T) {
	configDir := t.TempDir()
	if err := authpkg.WriteTokenMarker(configDir); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	release := make(chan struct{})
	installTokenManagerFakes(t, func() (*authpkg.TokenData, error) {
		calls.Add(1)
		<-release
		return &authpkg.TokenData{AccessToken: "token", ExpiresAt: time.Now().Add(time.Hour)}, nil
	})
	manager := NewTokenManager()
	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	errs := make(chan error, workers)
	for range workers {
		go func() {
			defer wg.Done()
			_, err := manager.Get(context.Background(), configDir, "")
			errs <- err
		}()
	}
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", calls.Load())
	}
}

func TestCrossPlatformCoverageTokenManagerPreservesProviderFailure(t *testing.T) {
	configDir := t.TempDir()
	want := errors.New("keychain permission denied")
	installTokenManagerFakes(t, func() (*authpkg.TokenData, error) { return nil, want })
	_, err := NewTokenManager().Get(context.Background(), configDir, "")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want cause %v", err, want)
	}
	if errors.Is(err, authpkg.ErrTokenDataNotFound) {
		t.Fatalf("provider failure was misclassified as missing credentials: %v", err)
	}
}

func TestCrossPlatformCoverageTokenResolutionErrorOnlyClassifiesTrueMissingCredential(t *testing.T) {
	missing := tokenResolutionError(authpkg.ErrTokenDataNotFound)
	var typed interface{ Unwrap() error }
	if !errors.As(missing, &typed) || !errors.Is(missing, authpkg.ErrTokenDataNotFound) {
		t.Fatalf("missing error = %v", missing)
	}
	want := errors.New("decrypt failed")
	if got := tokenResolutionError(want); !errors.Is(got, want) || errors.Is(got, authpkg.ErrTokenDataNotFound) {
		t.Fatalf("storage error = %v", got)
	}
	if got := tokenResolutionError(context.Canceled); !errors.Is(got, context.Canceled) {
		t.Fatalf("cancellation = %v", got)
	}
}
