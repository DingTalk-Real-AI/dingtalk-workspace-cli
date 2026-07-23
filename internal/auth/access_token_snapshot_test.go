package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCrossPlatformCoverageOAuthProviderTokenSnapshotPreservesLoadFailure(t *testing.T) {
	oldLoad := oauthLoadToken
	want := errors.New("keychain permission denied")
	oauthLoadToken = func(string) (*TokenData, error) { return nil, want }
	t.Cleanup(func() { oauthLoadToken = oldLoad })

	_, err := NewOAuthProvider(t.TempDir(), nil).GetTokenSnapshot(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want cause %v", err, want)
	}
	if errors.Is(err, ErrTokenDataNotFound) {
		t.Fatalf("load failure was misclassified as missing credentials: %v", err)
	}
}

func TestCrossPlatformCoverageOAuthProviderLoginPreservesLoadFailure(t *testing.T) {
	oldLoad := oauthLoadToken
	want := errors.New("keychain permission denied")
	oauthLoadToken = func(string) (*TokenData, error) { return nil, want }
	t.Cleanup(func() { oauthLoadToken = oldLoad })

	_, err := NewOAuthProvider(t.TempDir(), nil).Login(context.Background(), false)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want cause %v", err, want)
	}
}

func TestCrossPlatformCoverageOAuthProviderTokenSnapshotReturnsExpiryMetadata(t *testing.T) {
	oldLoad := oauthLoadToken
	expiresAt := time.Now().Add(time.Hour)
	oauthLoadToken = func(string) (*TokenData, error) {
		return &TokenData{AccessToken: "token", ExpiresAt: expiresAt}, nil
	}
	t.Cleanup(func() { oauthLoadToken = oldLoad })

	snapshot, err := NewOAuthProvider(t.TempDir(), nil).GetTokenSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AccessToken != "token" || !snapshot.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestCrossPlatformCoverageTokenMarkerRevisionChangesOnEveryPublication(t *testing.T) {
	configDir := t.TempDir()
	if err := WriteTokenMarker(configDir); err != nil {
		t.Fatal(err)
	}
	first, present, err := ReadTokenMarkerRevision(configDir)
	if err != nil || !present || first == "" {
		t.Fatalf("first marker = %q, %v, %v", first, present, err)
	}
	if err := WriteTokenMarker(configDir); err != nil {
		t.Fatal(err)
	}
	second, present, err := ReadTokenMarkerRevision(configDir)
	if err != nil || !present || second == "" || second == first {
		t.Fatalf("second marker = %q, %v, %v; first=%q", second, present, err, first)
	}
}
