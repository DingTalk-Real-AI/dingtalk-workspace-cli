package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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

func TestCrossPlatformCoverageOAuthProviderLoginReauthorizesAfterLoadFailureAndRejectsUnreadableTarget(t *testing.T) {
	cleanupKeychain(t)
	setLoginPreflightCredentials(t)

	oldLoad := oauthLoadToken
	oldOpenBrowser := oauthOpenBrowser
	oldExchange := oauthExchange
	oldCheckStatus := oauthCheckStatus
	oldSave := oauthSaveToken
	oldKeychainGet := authKeychainGet
	oldLoginTimeout := oauthLoginTimeout
	t.Cleanup(func() {
		oauthLoadToken = oldLoad
		oauthOpenBrowser = oldOpenBrowser
		oauthExchange = oldExchange
		oauthCheckStatus = oldCheckStatus
		oauthSaveToken = oldSave
		authKeychainGet = oldKeychainGet
		oauthLoginTimeout = oldLoginTimeout
	})
	oauthLoginTimeout = 2 * time.Second

	loadErr := errors.New("keychain permission denied")
	targetErr := errors.New("target token ciphertext is unreadable")
	oauthLoadToken = func(string) (*TokenData, error) { return nil, loadErr }

	browserCalls := 0
	oauthOpenBrowser = func(authURL string) error {
		browserCalls++
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		callbackURL := parsed.Query().Get("redirect_uri") + "?code=reauthorize"
		response, err := (&http.Client{Timeout: 5 * time.Second}).Get(callbackURL)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, response.Body)
		return response.Body.Close()
	}

	exchangeCalls := 0
	oauthExchange = func(*OAuthProvider, context.Context, string) (*TokenData, error) {
		exchangeCalls++
		return &TokenData{
			AccessToken: "new-access",
			CorpID:      "corp-target",
			UserID:      "user-target",
		}, nil
	}
	oauthCheckStatus = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) {
		return &CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}, nil
	}

	targetReads := 0
	authKeychainGet = func(_ string, account string) (string, error) {
		if account == TokenAccountForIdentity("corp-target", "user-target") {
			targetReads++
			return "", targetErr
		}
		return "", nil
	}
	saveCalls := 0
	oauthSaveToken = func(string, *TokenData) error {
		saveCalls++
		return nil
	}

	provider := NewOAuthProvider(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	provider.Output = io.Discard
	_, err := provider.Login(context.Background(), false)
	if !errors.Is(err, targetErr) {
		t.Fatalf("Login() error = %v, want target cause %v", err, targetErr)
	}
	if errors.Is(err, loadErr) {
		t.Fatalf("Login() returned stale load failure instead of reauthorizing: %v", err)
	}
	if browserCalls != 1 || exchangeCalls != 1 {
		t.Fatalf("authorization calls = browser:%d exchange:%d, want 1 each", browserCalls, exchangeCalls)
	}
	if targetReads != 1 {
		t.Fatalf("target slot reads = %d, want 1", targetReads)
	}
	if saveCalls != 0 {
		t.Fatalf("SaveTokenData calls = %d, want 0", saveCalls)
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
