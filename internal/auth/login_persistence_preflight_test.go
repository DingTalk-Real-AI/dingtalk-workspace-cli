// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type loginPreflightRoundTripFunc func(*http.Request) (*http.Response, error)

func (f loginPreflightRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// loginPreflightMemoryListener keeps OAuth callback tests hermetic. The
// production flow still sees a TCP-shaped address, while HTTP travels through
// net.Pipe and therefore does not require permission to bind a loopback port.
type loginPreflightMemoryListener struct {
	connections chan net.Conn
	closed      chan struct{}
	closeOnce   sync.Once
}

func newLoginPreflightMemoryListener() *loginPreflightMemoryListener {
	return &loginPreflightMemoryListener{
		connections: make(chan net.Conn, 1),
		closed:      make(chan struct{}),
	}
}

func (l *loginPreflightMemoryListener) Accept() (net.Conn, error) {
	select {
	case connection := <-l.connections:
		return connection, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *loginPreflightMemoryListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (l *loginPreflightMemoryListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 43819}
}

func (l *loginPreflightMemoryListener) Dial() (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case l.connections <- server:
		return client, nil
	case <-l.closed:
		_ = client.Close()
		_ = server.Close()
		return nil, net.ErrClosed
	}
}

type loginPersistenceMemoryKeychain struct {
	mu          sync.Mutex
	values      map[string]string
	readErrors  map[string]error
	writeErrors map[string]error
	reads       map[string]int
}

func installLoginPersistenceMemoryKeychain(t *testing.T) *loginPersistenceMemoryKeychain {
	t.Helper()
	store := &loginPersistenceMemoryKeychain{
		values:      make(map[string]string),
		readErrors:  make(map[string]error),
		writeErrors: make(map[string]error),
		reads:       make(map[string]int),
	}
	oldGet := authKeychainGet
	oldSet := authKeychainSet
	oldRemove := authKeychainRemove
	oldExists := authKeychainExists
	t.Cleanup(func() {
		authKeychainGet = oldGet
		authKeychainSet = oldSet
		authKeychainRemove = oldRemove
		authKeychainExists = oldExists
	})
	authKeychainGet = func(_, account string) (string, error) {
		store.mu.Lock()
		defer store.mu.Unlock()
		store.reads[account]++
		if err := store.readErrors[account]; err != nil {
			return "", err
		}
		return store.values[account], nil
	}
	authKeychainSet = func(_, account, value string) error {
		store.mu.Lock()
		defer store.mu.Unlock()
		if err := store.writeErrors[account]; err != nil {
			return err
		}
		store.values[account] = value
		delete(store.readErrors, account)
		return nil
	}
	authKeychainRemove = func(_, account string) error {
		store.mu.Lock()
		defer store.mu.Unlock()
		delete(store.values, account)
		delete(store.readErrors, account)
		return nil
	}
	authKeychainExists = func(_, account string) bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		_, exists := store.values[account]
		return exists && store.readErrors[account] == nil
	}
	return store
}

func (s *loginPersistenceMemoryKeychain) putToken(t *testing.T, account string, token *TokenData) {
	t.Helper()
	raw, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(token) error = %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[account] = string(raw)
	delete(s.readErrors, account)
}

func (s *loginPersistenceMemoryKeychain) token(account string) (*TokenData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.readErrors[account]; err != nil {
		return nil, err
	}
	raw := s.values[account]
	if raw == "" {
		return nil, ErrTokenDataNotFound
	}
	var token TokenData
	if err := json.Unmarshal([]byte(raw), &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func (s *loginPersistenceMemoryKeychain) setReadError(account string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readErrors[account] = err
}

func (s *loginPersistenceMemoryKeychain) setWriteError(account string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		delete(s.writeErrors, account)
		return
	}
	s.writeErrors[account] = err
}

func (s *loginPersistenceMemoryKeychain) readCount(account string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads[account]
}

func setLoginPreflightCredentials(t *testing.T) {
	t.Helper()
	SetClientID("login-preflight-client")
	SetClientSecret("login-preflight-secret")
	resetClientIDFromMCP()
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
	})
}

func writeFutureProfilesForLoginPreflight(t *testing.T, configDir string) {
	t.Helper()
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(configDir) error = %v", err)
	}
	raw, err := json.Marshal(&ProfilesConfig{Version: profilesMaxVersion + 1})
	if err != nil {
		t.Fatalf("json.Marshal(future profiles) error = %v", err)
	}
	if err := os.WriteFile(ProfilesPath(configDir), raw, 0o600); err != nil {
		t.Fatalf("WriteFile(future profiles) error = %v", err)
	}
}

type halfMigratedLoginFixture struct {
	configDir string
	store     *loginPersistenceMemoryKeychain
	corpA     string
	userA     string
	tokenA    *TokenData
	corpB     string
	userB     string
	tokenB    *TokenData
}

func newHalfMigratedLoginFixture(t *testing.T) *halfMigratedLoginFixture {
	t.Helper()
	store := installLoginPersistenceMemoryKeychain(t)
	configDir := t.TempDir()
	previousRuntimeProfile := RuntimeProfile()
	SetRuntimeProfile("")
	t.Cleanup(func() { SetRuntimeProfile(previousRuntimeProfile) })
	suffix := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	corpA := "corp-half-migrated-" + suffix
	userA := "user-half-migrated-" + suffix
	corpB := "corp-new-login-" + suffix
	userB := "user-new-login-" + suffix
	tokenA := testToken("access-half-migrated-"+suffix, corpA, "Half Migrated Organization")
	tokenA.UserID = ""
	tokenA.UserName = ""
	tokenB := testToken("access-new-login-"+suffix, corpB, "New Login Organization")
	tokenB.UserID = userB
	tokenB.UserName = "New Login Account"

	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        profilesVersion,
		PrimaryProfile: profileSelector(corpA, userA),
		CurrentProfile: profileSelector(corpA, userA),
		OrgCurrentProfiles: map[string]string{
			corpA: profileSelector(corpA, userA),
		},
		Profiles: []Profile{{
			Name:     "Half Migrated Account",
			CorpID:   corpA,
			CorpName: tokenA.CorpName,
			UserID:   userA,
			UserName: "Half Migrated Account",
			Status:   ProfileStatusActive,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	store.putToken(t, keychain.AccountToken, tokenA)
	return &halfMigratedLoginFixture{
		configDir: configDir,
		store:     store,
		corpA:     corpA,
		userA:     userA,
		tokenA:    tokenA,
		corpB:     corpB,
		userB:     userB,
		tokenB:    tokenB,
	}
}

func (f *halfMigratedLoginFixture) repairedError() error {
	org, err := f.store.token(TokenAccountForCorpID(f.corpA))
	if err != nil {
		return fmt.Errorf("organization repair is unavailable: %w", err)
	}
	if org.AccessToken != f.tokenA.AccessToken || org.CorpID != f.corpA || org.UserID != "" {
		return fmt.Errorf("organization repair = %#v", org)
	}
	identity, err := f.store.token(TokenAccountForIdentity(f.corpA, f.userA))
	if err != nil {
		return fmt.Errorf("identity repair is unavailable: %w", err)
	}
	if identity.AccessToken != f.tokenA.AccessToken ||
		identity.CorpID != f.corpA ||
		identity.UserID != f.userA {
		return fmt.Errorf("identity repair = %#v", identity)
	}
	return nil
}

func (f *halfMigratedLoginFixture) assertNewLoginPersisted(t *testing.T) {
	t.Helper()
	if err := f.repairedError(); err != nil {
		t.Fatal(err)
	}
	global, err := f.store.token(keychain.AccountToken)
	if err != nil {
		t.Fatalf("global token after new login: %v", err)
	}
	if global.AccessToken != f.tokenB.AccessToken ||
		global.CorpID != f.corpB ||
		global.UserID != f.userB {
		t.Fatalf("global token after new login = %#v, want B", global)
	}
}

func installHalfMigratedBrowserLogin(
	t *testing.T,
	beforeRemote func() error,
	token *TokenData,
) {
	t.Helper()
	oldListen := oauthListen
	oldOpenBrowser := oauthOpenBrowser
	oldExchange := oauthExchange
	oldCheckStatus := oauthCheckStatus
	oldSave := oauthSaveToken
	oldLoginTimeout := oauthLoginTimeout
	t.Cleanup(func() {
		oauthListen = oldListen
		oauthOpenBrowser = oldOpenBrowser
		oauthExchange = oldExchange
		oauthCheckStatus = oldCheckStatus
		oauthSaveToken = oldSave
		oauthLoginTimeout = oldLoginTimeout
	})
	oauthLoginTimeout = 2 * time.Second
	listener := newLoginPreflightMemoryListener()
	oauthListen = func(string, string) (net.Listener, error) { return listener, nil }
	oauthOpenBrowser = func(authURL string) error {
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		callbackURL, err := url.Parse(parsed.Query().Get("redirect_uri"))
		if err != nil {
			return err
		}
		query := callbackURL.Query()
		query.Set("code", "half-migrated-repair")
		callbackURL.RawQuery = query.Encode()
		request, err := http.NewRequest(http.MethodGet, callbackURL.String(), nil)
		if err != nil {
			return err
		}
		connection, err := listener.Dial()
		if err != nil {
			return err
		}
		defer connection.Close()
		if err := request.Write(connection); err != nil {
			return err
		}
		response, err := http.ReadResponse(bufio.NewReader(connection), request)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, response.Body)
		return response.Body.Close()
	}
	oauthExchange = func(*OAuthProvider, context.Context, string) (*TokenData, error) {
		if err := beforeRemote(); err != nil {
			return nil, err
		}
		return token, nil
	}
	oauthCheckStatus = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) {
		return &CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}, nil
	}
	oauthSaveToken = SaveTokenData
}

func TestExchangeCodeForTokenPreparesBeforeRemoteAndMarksFresh(t *testing.T) {
	cleanupKeychain(t)
	setLoginPreflightCredentials(t)
	configDir := t.TempDir()
	writeFutureProfilesForLoginPreflight(t, configDir)

	oldClient := oauthHTTPClient
	oldSaveSecret := oauthSaveClientSecret
	oldGet := authKeychainGet
	oldValidate := authValidateEntries
	t.Cleanup(func() {
		oauthHTTPClient = oldClient
		oauthSaveClientSecret = oldSaveSecret
		authKeychainGet = oldGet
		authValidateEntries = oldValidate
	})

	var httpCalls atomic.Int32
	var localReads atomic.Int32
	var inventoryCalls atomic.Int32
	oauthHTTPClient = &http.Client{Transport: loginPreflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		httpCalls.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"accessToken":"standalone-access","refreshToken":"standalone-refresh","expiresIn":7200,"corpId":"corp_standalone"}`,
			)),
		}, nil
	})}
	oauthSaveClientSecret = func(string, string) error { return nil }
	authKeychainGet = func(string, string) (string, error) {
		localReads.Add(1)
		return "", errors.New("unrelated local token is unreadable")
	}
	authValidateEntries = func(string) error {
		inventoryCalls.Add(1)
		return errors.New("unrelated orphan token is unreadable")
	}

	data, err := ExchangeCodeForToken(context.Background(), configDir, "blocked-code")
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("ExchangeCodeForToken(future schema) = %#v, %v; want pre-exchange rejection", data, err)
	}
	if got := httpCalls.Load(); got != 0 {
		t.Fatalf("HTTP calls before rejected prepare = %d, want 0", got)
	}
	if got := localReads.Load(); got != 0 {
		t.Fatalf("keychain reads before future-schema rejection = %d, want 0", got)
	}
	if got := inventoryCalls.Load(); got != 0 {
		t.Fatalf("inventory validation calls = %d, want 0", got)
	}

	// A safe registry proceeds to the remote exchange and marks the result so
	// PAT persistence cannot mistake it for a refresh of an existing account.
	safeConfigDir := t.TempDir()
	authKeychainGet = func(string, string) (string, error) {
		localReads.Add(1)
		return "", nil
	}
	data, err = ExchangeCodeForToken(context.Background(), safeConfigDir, "standalone-code")
	if err != nil {
		t.Fatalf("ExchangeCodeForToken(safe) error = %v", err)
	}
	if data == nil || data.AccessToken != "standalone-access" || !data.FreshAuthorization {
		t.Fatalf("ExchangeCodeForToken(safe) data = %#v", data)
	}
	if got := httpCalls.Load(); got != 1 {
		t.Fatalf("HTTP calls after safe prepare = %d, want 1", got)
	}
}

func TestPersistingLoginFlowsRejectFutureProfilesBeforeRemoteWork(t *testing.T) {
	t.Run("external auth-code exchange", func(t *testing.T) {
		cleanupKeychain(t)
		configDir := t.TempDir()
		writeFutureProfilesForLoginPreflight(t, configDir)

		oldExchange := oauthExchange
		t.Cleanup(func() { oauthExchange = oldExchange })
		var calls atomic.Int32
		oauthExchange = func(*OAuthProvider, context.Context, string) (*TokenData, error) {
			calls.Add(1)
			return &TokenData{AccessToken: "must-not-exchange"}, nil
		}

		_, err := NewOAuthProvider(configDir, nil).ExchangeAuthCode(context.Background(), "code", "user")
		if err == nil || !strings.Contains(err.Error(), "newer than supported") {
			t.Fatalf("ExchangeAuthCode() error = %v, want future profiles rejection", err)
		}
		if got := calls.Load(); got != 0 {
			t.Fatalf("remote exchange calls = %d, want 0", got)
		}
	})

	t.Run("browser OAuth", func(t *testing.T) {
		for _, force := range []bool{false, true} {
			t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
				cleanupKeychain(t)
				setLoginPreflightCredentials(t)
				configDir := t.TempDir()
				writeFutureProfilesForLoginPreflight(t, configDir)

				oldListen := oauthListen
				t.Cleanup(func() { oauthListen = oldListen })
				var calls atomic.Int32
				oauthListen = func(string, string) (net.Listener, error) {
					calls.Add(1)
					return nil, errors.New("must not start callback listener")
				}

				_, err := NewOAuthProvider(configDir, nil).Login(context.Background(), force)
				if err == nil || !strings.Contains(err.Error(), "newer than supported") {
					t.Fatalf("OAuthProvider.Login(force=%t) error = %v, want future profiles rejection", force, err)
				}
				if got := calls.Load(); got != 0 {
					t.Fatalf("callback listener calls = %d, want 0", got)
				}
			})
		}
	})

	t.Run("device flow", func(t *testing.T) {
		cleanupKeychain(t)
		setLoginPreflightCredentials(t)
		configDir := t.TempDir()
		writeFutureProfilesForLoginPreflight(t, configDir)

		oldLoginOnce := deviceLoginOnce
		t.Cleanup(func() { deviceLoginOnce = oldLoginOnce })
		var calls atomic.Int32
		deviceLoginOnce = func(*DeviceFlowProvider, context.Context, int) (*TokenData, error) {
			calls.Add(1)
			return &TokenData{AccessToken: "must-not-start"}, nil
		}

		_, err := NewDeviceFlowProvider(configDir, nil).Login(context.Background())
		if err == nil || !strings.Contains(err.Error(), "newer than supported") {
			t.Fatalf("DeviceFlowProvider.Login() error = %v, want future profiles rejection", err)
		}
		if got := calls.Load(); got != 0 {
			t.Fatalf("device login calls = %d, want 0", got)
		}
	})
}

func TestPersistingLoginFlowsRepairHalfMigratedGlobalBeforeRemoteAndSave(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("browser OAuth force=%t", force), func(t *testing.T) {
			setLoginPreflightCredentials(t)
			fixture := newHalfMigratedLoginFixture(t)
			installHalfMigratedBrowserLogin(t, fixture.repairedError, fixture.tokenB)

			oldLoad := oauthLoadToken
			t.Cleanup(func() { oauthLoadToken = oldLoad })
			loadCalls := 0
			if !force {
				oauthLoadToken = func(string) (*TokenData, error) {
					loadCalls++
					return nil, ErrTokenDataNotFound
				}
			}

			provider := NewOAuthProvider(fixture.configDir, nil)
			provider.Output = io.Discard
			data, err := provider.Login(context.Background(), force)
			if err != nil {
				t.Fatalf("OAuthProvider.Login(force=%t) error = %v", force, err)
			}
			if data != fixture.tokenB {
				t.Fatalf("OAuthProvider.Login(force=%t) data = %#v, want token B", force, data)
			}
			if want := map[bool]int{false: 1, true: 0}[force]; loadCalls != want {
				t.Fatalf("OAuthProvider.Login(force=%t) silent load calls = %d, want %d", force, loadCalls, want)
			}
			fixture.assertNewLoginPersisted(t)
		})
	}

	t.Run("external auth-code exchange", func(t *testing.T) {
		fixture := newHalfMigratedLoginFixture(t)
		oldExchange := oauthExchange
		oldSave := oauthSaveToken
		t.Cleanup(func() {
			oauthExchange = oldExchange
			oauthSaveToken = oldSave
		})
		remoteObservedRepair := false
		oauthExchange = func(*OAuthProvider, context.Context, string) (*TokenData, error) {
			if err := fixture.repairedError(); err != nil {
				return nil, err
			}
			remoteObservedRepair = true
			return fixture.tokenB, nil
		}
		oauthSaveToken = SaveTokenData

		data, err := NewOAuthProvider(fixture.configDir, nil).ExchangeAuthCode(
			context.Background(),
			"new-login-code",
			fixture.userB,
		)
		if err != nil {
			t.Fatalf("ExchangeAuthCode() error = %v", err)
		}
		if data != fixture.tokenB || !remoteObservedRepair {
			t.Fatalf("ExchangeAuthCode() data = %#v, repair observed = %t", data, remoteObservedRepair)
		}
		fixture.assertNewLoginPersisted(t)
	})

	t.Run("device flow", func(t *testing.T) {
		setLoginPreflightCredentials(t)
		fixture := newHalfMigratedLoginFixture(t)
		oldLoginOnce := deviceLoginOnce
		oldRequest := deviceRequestCode
		oldWait := deviceWaitAuth
		oldExchange := deviceExchangeCode
		oldCheck := deviceCheckCLIAuth
		oldSave := deviceSaveToken
		t.Cleanup(func() {
			deviceLoginOnce = oldLoginOnce
			deviceRequestCode = oldRequest
			deviceWaitAuth = oldWait
			deviceExchangeCode = oldExchange
			deviceCheckCLIAuth = oldCheck
			deviceSaveToken = oldSave
		})
		deviceLoginOnce = func(p *DeviceFlowProvider, ctx context.Context, attempt int) (*TokenData, error) {
			return p.loginOnce(ctx, attempt)
		}
		deviceRequestCode = func(*DeviceFlowProvider, context.Context) (*DeviceAuthResponse, error) {
			return &DeviceAuthResponse{DeviceCode: "device", UserCode: "user", Interval: 1}, nil
		}
		deviceWaitAuth = func(*DeviceFlowProvider, context.Context, *DeviceAuthResponse) (*DeviceTokenResponse, error) {
			return &DeviceTokenResponse{AuthCode: "new-login-code"}, nil
		}
		remoteObservedRepair := false
		deviceExchangeCode = func(*OAuthProvider, context.Context, string) (*TokenData, error) {
			if err := fixture.repairedError(); err != nil {
				return nil, err
			}
			remoteObservedRepair = true
			return fixture.tokenB, nil
		}
		deviceCheckCLIAuth = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) {
			return &CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}, nil
		}
		deviceSaveToken = SaveTokenData

		provider := NewDeviceFlowProvider(fixture.configDir, nil)
		provider.Output = io.Discard
		provider.NoBrowser = true
		data, err := provider.Login(context.Background())
		if err != nil {
			t.Fatalf("DeviceFlowProvider.Login() error = %v", err)
		}
		if data != fixture.tokenB || !remoteObservedRepair {
			t.Fatalf("DeviceFlowProvider.Login() data = %#v, repair observed = %t", data, remoteObservedRepair)
		}
		fixture.assertNewLoginPersisted(t)
	})
}

func TestPrepareLoginPersistenceRepairsOnlySafeGlobalOwner(t *testing.T) {
	t.Run("matching global overwrites damaged canonical slots", func(t *testing.T) {
		fixture := newHalfMigratedLoginFixture(t)
		corruptErr := errors.New("ciphertext key mismatch")
		fixture.store.setReadError(TokenAccountForCorpID(fixture.corpA), corruptErr)
		fixture.store.setReadError(TokenAccountForIdentity(fixture.corpA, fixture.userA), corruptErr)

		if err := prepareLoginPersistence(fixture.configDir); err != nil {
			t.Fatalf("prepareLoginPersistence() error = %v", err)
		}
		if err := fixture.repairedError(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("partial global repair retries identity from repaired organization", func(t *testing.T) {
		fixture := newHalfMigratedLoginFixture(t)
		identityAccount := TokenAccountForIdentity(fixture.corpA, fixture.userA)
		writeErr := errors.New("identity storage is temporarily unavailable")
		fixture.store.setWriteError(identityAccount, writeErr)

		err := prepareLoginPersistence(fixture.configDir)
		if !errors.Is(err, writeErr) {
			t.Fatalf("first prepareLoginPersistence() error = %v, want identity write failure", err)
		}
		org, orgErr := fixture.store.token(TokenAccountForCorpID(fixture.corpA))
		if orgErr != nil || org.AccessToken != fixture.tokenA.AccessToken || org.UserID != "" {
			t.Fatalf("organization repair after partial failure = %#v, %v", org, orgErr)
		}

		fixture.store.setWriteError(identityAccount, nil)
		if err := prepareLoginPersistence(fixture.configDir); err != nil {
			t.Fatalf("second prepareLoginPersistence() error = %v", err)
		}
		if err := fixture.repairedError(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("different global user cannot be attached to sole profile", func(t *testing.T) {
		fixture := newHalfMigratedLoginFixture(t)
		mismatched := *fixture.tokenA
		mismatched.UserID = "different-synthetic-user"
		fixture.store.putToken(t, keychain.AccountToken, &mismatched)

		err := prepareLoginPersistence(fixture.configDir)
		if err == nil || !strings.Contains(err.Error(), "does not safely match") {
			t.Fatalf("prepareLoginPersistence() error = %v, want owner mismatch", err)
		}
		if _, loadErr := fixture.store.token(TokenAccountForCorpID(fixture.corpA)); !errors.Is(loadErr, ErrTokenDataNotFound) {
			t.Fatalf("organization slot after rejected repair error = %v, want missing", loadErr)
		}
		if _, loadErr := fixture.store.token(TokenAccountForIdentity(fixture.corpA, fixture.userA)); !errors.Is(loadErr, ErrTokenDataNotFound) {
			t.Fatalf("identity slot after rejected repair error = %v, want missing", loadErr)
		}
	})

	t.Run("ambiguous same-corp profiles fail before global overwrite", func(t *testing.T) {
		fixture := newHalfMigratedLoginFixture(t)
		cfg, err := LoadProfiles(fixture.configDir)
		if err != nil {
			t.Fatalf("LoadProfiles() error = %v", err)
		}
		cfg.Profiles = append(cfg.Profiles, Profile{
			Name:     "Second Synthetic Account",
			CorpID:   fixture.corpA,
			CorpName: fixture.tokenA.CorpName,
			UserID:   fixture.userA + "-second",
			UserName: "Second Synthetic Account",
			Status:   ProfileStatusActive,
		})
		if err := SaveProfiles(fixture.configDir, cfg); err != nil {
			t.Fatalf("SaveProfiles(ambiguous) error = %v", err)
		}

		err = prepareLoginPersistence(fixture.configDir)
		if err == nil || !strings.Contains(err.Error(), "one of 2 accounts") {
			t.Fatalf("prepareLoginPersistence() error = %v, want ambiguous-owner protection", err)
		}
	})

	t.Run("damaged unrelated organization is not inspected", func(t *testing.T) {
		fixture := newHalfMigratedLoginFixture(t)
		existing := *fixture.tokenA
		existing.UserID = fixture.userA
		existing.UserName = "Half Migrated Account"
		fixture.store.putToken(t, TokenAccountForIdentity(fixture.corpA, fixture.userA), &existing)

		cfg, err := LoadProfiles(fixture.configDir)
		if err != nil {
			t.Fatalf("LoadProfiles() error = %v", err)
		}
		unrelatedCorp := fixture.corpA + "-unrelated"
		unrelatedUser := fixture.userA + "-unrelated"
		cfg.Profiles = append(cfg.Profiles, Profile{
			Name:   "Unreadable Unrelated Account",
			CorpID: unrelatedCorp,
			UserID: unrelatedUser,
			Status: ProfileStatusActive,
		})
		if err := SaveProfiles(fixture.configDir, cfg); err != nil {
			t.Fatalf("SaveProfiles(unrelated) error = %v", err)
		}
		unrelatedErr := errors.New("unrelated ciphertext is unreadable")
		unrelatedOrg := TokenAccountForCorpID(unrelatedCorp)
		unrelatedIdentity := TokenAccountForIdentity(unrelatedCorp, unrelatedUser)
		fixture.store.setReadError(unrelatedOrg, unrelatedErr)
		fixture.store.setReadError(unrelatedIdentity, unrelatedErr)

		if err := prepareLoginPersistence(fixture.configDir); err != nil {
			t.Fatalf("prepareLoginPersistence() error = %v", err)
		}
		if got := fixture.store.readCount(unrelatedOrg); got != 0 {
			t.Fatalf("unrelated organization reads = %d, want 0", got)
		}
		if got := fixture.store.readCount(unrelatedIdentity); got != 0 {
			t.Fatalf("unrelated identity reads = %d, want 0", got)
		}
	})

	t.Run("version one remains on existing migration path", func(t *testing.T) {
		store := installLoginPersistenceMemoryKeychain(t)
		configDir := t.TempDir()
		if err := SaveProfiles(configDir, &ProfilesConfig{
			Version: 1,
			Profiles: []Profile{{
				Name:   "Version One Fixture",
				CorpID: "corp-version-one-fixture",
				UserID: "user-version-one-fixture",
			}},
		}); err != nil {
			t.Fatalf("SaveProfiles(v1) error = %v", err)
		}
		store.setReadError(keychain.AccountToken, errors.New("v1 global is intentionally untouched"))

		if err := prepareLoginPersistence(configDir); err != nil {
			t.Fatalf("prepareLoginPersistence(v1) error = %v", err)
		}
		if got := store.readCount(keychain.AccountToken); got != 0 {
			t.Fatalf("v1 global reads = %d, want 0", got)
		}
	})
}

func TestPrepareLoginPersistenceUnreadableGlobalFailsClosedBeforeRemote(t *testing.T) {
	fixture := newHalfMigratedLoginFixture(t)
	globalErr := errors.New("global ciphertext is unreadable")
	fixture.store.setReadError(keychain.AccountToken, globalErr)

	err := prepareLoginPersistence(fixture.configDir)
	if !errors.Is(err, globalErr) || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("prepareLoginPersistence() error = %v, want unreadable-global protection", err)
	}
}

func TestPrepareLoginPersistenceV3UnresolvedProfileRepair(t *testing.T) {
	for _, tc := range []struct {
		name        string
		globalUID   string
		wantSuccess bool
	}{
		{name: "blank global identity repairs organization", wantSuccess: true},
		{name: "nonblank global identity is rejected", globalUID: "different-synthetic-user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := installLoginPersistenceMemoryKeychain(t)
			configDir := t.TempDir()
			corpID := "corp-v3-unresolved-" + strings.ReplaceAll(t.Name(), "/", "-")
			reserved := unresolvedProfileSelector(corpID)
			if err := SaveProfiles(configDir, &ProfilesConfig{
				Version:        profilesUnresolvedSelectorVersion,
				CurrentProfile: reserved,
				Profiles: []Profile{{
					Name:     "Synthetic External Account",
					CorpID:   corpID,
					CorpName: "Synthetic V3 Organization",
					Status:   ProfileStatusActive,
				}},
			}); err != nil {
				t.Fatalf("SaveProfiles(v3 unresolved) error = %v", err)
			}
			global := testToken("access-v3-unresolved", corpID, "Synthetic V3 Organization")
			global.UserID = tc.globalUID
			global.UserName = ""
			store.putToken(t, keychain.AccountToken, global)

			err := prepareLoginPersistence(configDir)
			if tc.wantSuccess {
				if err != nil {
					t.Fatalf("prepareLoginPersistence() error = %v", err)
				}
				org, loadErr := store.token(TokenAccountForCorpID(corpID))
				if loadErr != nil || org.AccessToken != global.AccessToken || org.UserID != "" {
					t.Fatalf("repaired organization = %#v, %v", org, loadErr)
				}
				cfg, loadErr := LoadProfiles(configDir)
				if loadErr != nil || cfg.Version != profilesUnresolvedSelectorVersion || cfg.CurrentProfile != reserved {
					t.Fatalf("profiles after repair = %#v, %v", cfg, loadErr)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "does not safely match") {
				t.Fatalf("prepareLoginPersistence() error = %v, want unresolved owner rejection", err)
			}
			if _, loadErr := store.token(TokenAccountForCorpID(corpID)); !errors.Is(loadErr, ErrTokenDataNotFound) {
				t.Fatalf("organization slot after rejected repair error = %v, want missing", loadErr)
			}
		})
	}
}

func TestPrepareLoginPersistenceMultiAccountProtectionIsTargeted(t *testing.T) {
	for _, tc := range []struct {
		name          string
		secondPresent bool
		wantSuccess   bool
	}{
		{name: "one canonical missing fails closed"},
		{name: "all canonicals present allows overwrite", secondPresent: true, wantSuccess: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := installLoginPersistenceMemoryKeychain(t)
			configDir := t.TempDir()
			corpID := "corp-multi-" + strings.ReplaceAll(t.Name(), "/", "-")
			firstUser := "first-" + strings.ReplaceAll(t.Name(), "/", "-")
			secondUser := "second-" + strings.ReplaceAll(t.Name(), "/", "-")
			if err := SaveProfiles(configDir, &ProfilesConfig{
				Version:        profilesVersion,
				CurrentProfile: profileSelector(corpID, firstUser),
				OrgCurrentProfiles: map[string]string{
					corpID: profileSelector(corpID, firstUser),
				},
				Profiles: []Profile{
					{Name: "First Synthetic Account", CorpID: corpID, UserID: firstUser},
					{Name: "Second Synthetic Account", CorpID: corpID, UserID: secondUser},
				},
			}); err != nil {
				t.Fatalf("SaveProfiles(multi) error = %v", err)
			}
			global := testToken("access-multi-global", corpID, "Synthetic Multi Organization")
			global.UserID = firstUser
			store.putToken(t, keychain.AccountToken, global)
			first := testToken("access-multi-first", corpID, "Synthetic Multi Organization")
			first.UserID = firstUser
			store.putToken(t, TokenAccountForIdentity(corpID, firstUser), first)
			if tc.secondPresent {
				second := testToken("access-multi-second", corpID, "Synthetic Multi Organization")
				second.UserID = secondUser
				store.putToken(t, TokenAccountForIdentity(corpID, secondUser), second)
			}

			err := prepareLoginPersistence(configDir)
			if tc.wantSuccess {
				if err != nil {
					t.Fatalf("prepareLoginPersistence() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "one of 2 accounts") {
				t.Fatalf("prepareLoginPersistence() error = %v, want multi-account protection", err)
			}
		})
	}
}

func TestPrepareLoginPersistenceRequiresCredentialMaterialButNotValidity(t *testing.T) {
	t.Run("empty token cannot become canonical", func(t *testing.T) {
		fixture := newHalfMigratedLoginFixture(t)
		empty := *fixture.tokenA
		empty.AccessToken = ""
		empty.RefreshToken = ""
		empty.PersistentCode = ""
		fixture.store.putToken(t, keychain.AccountToken, &empty)

		err := prepareLoginPersistence(fixture.configDir)
		if err == nil || !strings.Contains(err.Error(), "no recoverable credential material") {
			t.Fatalf("prepareLoginPersistence() error = %v, want empty-token rejection", err)
		}
	})

	t.Run("expired credential material is still recoverable", func(t *testing.T) {
		fixture := newHalfMigratedLoginFixture(t)
		expired := *fixture.tokenA
		expired.ExpiresAt = time.Now().Add(-48 * time.Hour)
		expired.RefreshExpAt = time.Now().Add(-24 * time.Hour)
		fixture.store.putToken(t, keychain.AccountToken, &expired)

		if err := prepareLoginPersistence(fixture.configDir); err != nil {
			t.Fatalf("prepareLoginPersistence() error = %v", err)
		}
		if err := fixture.repairedError(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPrepareLoginPersistenceCustomSaveHookDoesNotTouchBuiltInState(t *testing.T) {
	oldHooks := edition.Get()
	oldAcquire := profilesAcquireDualLock
	oldLoad := profilesLoad
	oldGet := authKeychainGet
	t.Cleanup(func() {
		edition.Override(oldHooks)
		profilesAcquireDualLock = oldAcquire
		profilesLoad = oldLoad
		authKeychainGet = oldGet
	})
	edition.Override(&edition.Hooks{SaveToken: func(string, []byte) error { return nil }})

	var lockCalls atomic.Int32
	var profileCalls atomic.Int32
	var keychainCalls atomic.Int32
	profilesAcquireDualLock = func(context.Context, string) (*DualLock, error) {
		lockCalls.Add(1)
		return nil, errors.New("built-in lock must not be acquired")
	}
	profilesLoad = func(string) (*ProfilesConfig, error) {
		profileCalls.Add(1)
		return nil, errors.New("built-in profiles must not be loaded")
	}
	authKeychainGet = func(string, string) (string, error) {
		keychainCalls.Add(1)
		return "", errors.New("built-in keychain must not be read")
	}

	if err := prepareLoginPersistence(t.TempDir()); err != nil {
		t.Fatalf("prepareLoginPersistence(custom hook) error = %v", err)
	}
	if lockCalls.Load() != 0 || profileCalls.Load() != 0 || keychainCalls.Load() != 0 {
		t.Fatalf(
			"built-in calls with custom hook = lock:%d profiles:%d keychain:%d, want all zero",
			lockCalls.Load(),
			profileCalls.Load(),
			keychainCalls.Load(),
		)
	}
}

func TestSaveLoginTokenDataRepairsHalfMigratedStateBeforeManualGlobalOverwrite(t *testing.T) {
	fixture := newHalfMigratedLoginFixture(t)
	manual := &TokenData{
		AccessToken: "manual-login-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}

	if err := SaveLoginTokenData(fixture.configDir, manual); err != nil {
		t.Fatalf("SaveLoginTokenData(manual) error = %v", err)
	}
	if err := fixture.repairedError(); err != nil {
		t.Fatalf("legacy profile was not repaired before manual overwrite: %v", err)
	}
	global, err := fixture.store.token(keychain.AccountToken)
	if err != nil {
		t.Fatalf("load manual global token: %v", err)
	}
	if global.AccessToken != manual.AccessToken || global.CorpID != "" {
		t.Fatalf("global token after manual login = %#v", global)
	}
}

func TestOAuthPersistLoginTokenMarksFreshBeforeUIDLessIsolationCheck(t *testing.T) {
	oldSave := oauthSaveToken
	oauthSaveToken = SaveTokenData
	t.Cleanup(func() { oauthSaveToken = oldSave })

	for _, selectorKind := range []string{"implicit", "explicit-exact"} {
		t.Run(selectorKind, func(t *testing.T) {
			fixture := seedLegacyBlankAndExactIdentitySlots(t)
			previousRuntimeProfile := RuntimeProfile()
			if selectorKind == "explicit-exact" {
				SetRuntimeProfile(profileSelector(fixture.corpID, fixture.alpha.UserID))
			} else {
				SetRuntimeProfile("")
			}
			t.Cleanup(func() { SetRuntimeProfile(previousRuntimeProfile) })

			fresh := testToken("oauth-fresh-unknown-"+selectorKind, fixture.corpID, fixture.blank.CorpName)
			fresh.UserID = ""
			fresh.UserName = ""
			provider := NewOAuthProvider(fixture.configDir, nil)
			err := provider.persistLoginToken(context.Background(), fresh)
			if err == nil || !strings.Contains(err.Error(), "UID-less token") {
				t.Fatalf("persistLoginToken() error = %v, want unresolved-sibling protection", err)
			}
			if !fresh.FreshAuthorization {
				t.Fatal("persistLoginToken did not mark exchanged token as a fresh authorization")
			}
			assertOrganizationTokenAccessForTest(t, fixture.corpID, fixture.blank.AccessToken, "")
			assertIdentityTokenAccessForTest(t, fixture.corpID, fixture.alpha.UserID, fixture.alpha.AccessToken)
		})
	}
}

func TestOAuthLoginValidExactTokenDoesNotInspectUnreadableGlobal(t *testing.T) {
	fixture := newHalfMigratedLoginFixture(t)
	fixture.store.setReadError(keychain.AccountToken, errors.New("unreadable compatibility mirror"))
	validExact := *fixture.tokenA
	validExact.UserID = fixture.userA
	validExact.UserName = "Half Migrated Account"
	fixture.store.putToken(t, TokenAccountForIdentity(fixture.corpA, fixture.userA), &validExact)

	oldLoad := oauthLoadToken
	oldListen := oauthListen
	t.Cleanup(func() {
		oauthLoadToken = oldLoad
		oauthListen = oldListen
	})
	oauthLoadToken = LoadTokenData
	listenCalls := 0
	oauthListen = func(string, string) (net.Listener, error) {
		listenCalls++
		return nil, errors.New("must not start browser authorization")
	}

	data, err := NewOAuthProvider(fixture.configDir, nil).Login(context.Background(), false)
	if err != nil {
		t.Fatalf("OAuthProvider.Login() error = %v", err)
	}
	if data.AccessToken != validExact.AccessToken ||
		data.CorpID != fixture.corpA ||
		data.UserID != fixture.userA ||
		listenCalls != 0 {
		t.Fatalf("OAuthProvider.Login() data=%#v listen=%d", data, listenCalls)
	}
	if got := fixture.store.readCount(keychain.AccountToken); got != 0 {
		t.Fatalf("global mirror reads = %d, want 0 on valid-token fast path", got)
	}
}

func TestExchangeAuthCodeChecksResolvedTargetBeforeSave(t *testing.T) {
	for _, tc := range []struct {
		name       string
		explicitID string
		wantEnrich int32
	}{
		{name: "identity enricher supplies user", wantEnrich: 1},
		{name: "external host supplies user", explicitID: "user_target", wantEnrich: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanupKeychain(t)
			configDir := t.TempDir()
			if err := SaveProfiles(configDir, &ProfilesConfig{Version: profilesVersion}); err != nil {
				t.Fatalf("SaveProfiles() error = %v", err)
			}

			targetErr := errors.New("target identity ciphertext is unreadable")
			oldGet := authKeychainGet
			oldExchange := oauthExchange
			oldSave := oauthSaveToken
			t.Cleanup(func() {
				authKeychainGet = oldGet
				oauthExchange = oldExchange
				oauthSaveToken = oldSave
			})

			authKeychainGet = func(_ string, account string) (string, error) {
				if account == TokenAccountForIdentity("corp_target", "user_target") {
					return "", targetErr
				}
				return "", nil
			}
			var exchangeCalls atomic.Int32
			oauthExchange = func(*OAuthProvider, context.Context, string) (*TokenData, error) {
				exchangeCalls.Add(1)
				return &TokenData{
					AccessToken:  "target-access",
					RefreshToken: "target-refresh",
					CorpID:       "corp_target",
				}, nil
			}
			var saveCalls atomic.Int32
			oauthSaveToken = func(string, *TokenData) error {
				saveCalls.Add(1)
				return nil
			}
			var enrichCalls atomic.Int32
			provider := NewOAuthProvider(configDir, nil)
			provider.IdentityEnricher = func(_ context.Context, data *TokenData) error {
				enrichCalls.Add(1)
				data.UserID = "user_target"
				return nil
			}

			_, err := provider.ExchangeAuthCode(context.Background(), "target-code", tc.explicitID)
			if !errors.Is(err, targetErr) {
				t.Fatalf("ExchangeAuthCode() error = %v, want target ciphertext error", err)
			}
			if got := exchangeCalls.Load(); got != 1 {
				t.Fatalf("remote exchange calls = %d, want 1", got)
			}
			if got := enrichCalls.Load(); got != tc.wantEnrich {
				t.Fatalf("identity enrichment calls = %d, want %d", got, tc.wantEnrich)
			}
			if got := saveCalls.Load(); got != 0 {
				t.Fatalf("SaveTokenData calls = %d, want 0", got)
			}
		})
	}
}

func TestDeviceFlowChecksResolvedTargetBeforeSave(t *testing.T) {
	cleanupKeychain(t)
	setLoginPreflightCredentials(t)
	configDir := t.TempDir()
	if err := SaveProfiles(configDir, &ProfilesConfig{Version: profilesVersion}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	targetErr := errors.New("target identity ciphertext is unreadable")
	oldGet := authKeychainGet
	oldLoginOnce := deviceLoginOnce
	oldRequest := deviceRequestCode
	oldWait := deviceWaitAuth
	oldExchange := deviceExchangeCode
	oldCheck := deviceCheckCLIAuth
	oldSave := deviceSaveToken
	t.Cleanup(func() {
		authKeychainGet = oldGet
		deviceLoginOnce = oldLoginOnce
		deviceRequestCode = oldRequest
		deviceWaitAuth = oldWait
		deviceExchangeCode = oldExchange
		deviceCheckCLIAuth = oldCheck
		deviceSaveToken = oldSave
	})

	authKeychainGet = func(_ string, account string) (string, error) {
		if account == TokenAccountForIdentity("corp_target", "user_target") {
			return "", targetErr
		}
		return "", nil
	}
	deviceLoginOnce = func(p *DeviceFlowProvider, ctx context.Context, attempt int) (*TokenData, error) {
		return p.loginOnce(ctx, attempt)
	}
	deviceRequestCode = func(*DeviceFlowProvider, context.Context) (*DeviceAuthResponse, error) {
		return &DeviceAuthResponse{DeviceCode: "device", UserCode: "user", Interval: 1}, nil
	}
	deviceWaitAuth = func(*DeviceFlowProvider, context.Context, *DeviceAuthResponse) (*DeviceTokenResponse, error) {
		return &DeviceTokenResponse{AuthCode: "target-code"}, nil
	}
	var exchangeCalls atomic.Int32
	deviceExchangeCode = func(*OAuthProvider, context.Context, string) (*TokenData, error) {
		exchangeCalls.Add(1)
		return &TokenData{
			AccessToken:  "target-access",
			RefreshToken: "target-refresh",
			CorpID:       "corp_target",
		}, nil
	}
	deviceCheckCLIAuth = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) {
		return &CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}, nil
	}
	var saveCalls atomic.Int32
	deviceSaveToken = func(string, *TokenData) error {
		saveCalls.Add(1)
		return nil
	}
	var enrichCalls atomic.Int32
	provider := NewDeviceFlowProvider(configDir, nil)
	provider.Output = io.Discard
	provider.NoBrowser = true
	provider.IdentityEnricher = func(_ context.Context, data *TokenData) error {
		enrichCalls.Add(1)
		data.UserID = "user_target"
		return nil
	}

	_, err := provider.Login(context.Background())
	if !errors.Is(err, targetErr) {
		t.Fatalf("DeviceFlowProvider.Login() error = %v, want target ciphertext error", err)
	}
	if got := exchangeCalls.Load(); got != 1 {
		t.Fatalf("remote exchange calls = %d, want 1", got)
	}
	if got := enrichCalls.Load(); got != 1 {
		t.Fatalf("identity enrichment calls = %d, want 1", got)
	}
	if got := saveCalls.Load(); got != 0 {
		t.Fatalf("SaveTokenData calls = %d, want 0", got)
	}
}

func TestLoginTargetPreflightIgnoresUnrelatedProfileSlots(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	cfg := &ProfilesConfig{
		Version: profilesVersion,
		Profiles: []Profile{{
			Name:   "unrelated",
			CorpID: "corp_unrelated",
			UserID: "user_unrelated",
		}},
	}
	if err := SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	unrelatedErr := errors.New("unrelated profile ciphertext is unreadable")
	oldGet := authKeychainGet
	oldValidate := authValidateEntries
	oldExchange := oauthExchange
	oldSave := oauthSaveToken
	t.Cleanup(func() {
		authKeychainGet = oldGet
		authValidateEntries = oldValidate
		oauthExchange = oldExchange
		oauthSaveToken = oldSave
	})
	authKeychainGet = func(_ string, account string) (string, error) {
		if account == TokenAccountForCorpID("corp_unrelated") ||
			account == TokenAccountForIdentity("corp_unrelated", "user_unrelated") {
			return "", unrelatedErr
		}
		return "", nil
	}
	authValidateEntries = func(string) error {
		return errors.New("unrelated orphan ciphertext is unreadable")
	}
	var exchangeCalls atomic.Int32
	oauthExchange = func(*OAuthProvider, context.Context, string) (*TokenData, error) {
		exchangeCalls.Add(1)
		return &TokenData{AccessToken: "target-access", CorpID: "corp_target"}, nil
	}
	var saveCalls atomic.Int32
	oauthSaveToken = func(string, *TokenData) error {
		saveCalls.Add(1)
		return nil
	}

	data, err := NewOAuthProvider(configDir, nil).ExchangeAuthCode(context.Background(), "target-code", "user_target")
	if err != nil {
		t.Fatalf("ExchangeAuthCode() error = %v", err)
	}
	if data == nil || data.UserID != "user_target" {
		t.Fatalf("ExchangeAuthCode() data = %#v", data)
	}
	if got := exchangeCalls.Load(); got != 1 {
		t.Fatalf("remote exchange calls = %d, want 1", got)
	}
	if got := saveCalls.Load(); got != 1 {
		t.Fatalf("SaveTokenData calls = %d, want 1", got)
	}
}

func TestDeviceLoginIgnoresUnreadableUnrelatedProfile(t *testing.T) {
	cleanupKeychain(t)
	setLoginPreflightCredentials(t)
	configDir := t.TempDir()
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version: profilesVersion,
		Profiles: []Profile{{
			Name:   "unrelated",
			CorpID: "corp_unrelated",
			UserID: "user_unrelated",
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	unrelatedErr := errors.New("unrelated profile ciphertext is unreadable")
	oldGet := authKeychainGet
	oldValidate := authValidateEntries
	oldLoginOnce := deviceLoginOnce
	oldRequest := deviceRequestCode
	oldWait := deviceWaitAuth
	oldExchange := deviceExchangeCode
	oldCheck := deviceCheckCLIAuth
	oldSave := deviceSaveToken
	t.Cleanup(func() {
		authKeychainGet = oldGet
		authValidateEntries = oldValidate
		deviceLoginOnce = oldLoginOnce
		deviceRequestCode = oldRequest
		deviceWaitAuth = oldWait
		deviceExchangeCode = oldExchange
		deviceCheckCLIAuth = oldCheck
		deviceSaveToken = oldSave
	})

	var unrelatedReads atomic.Int32
	var inventoryCalls atomic.Int32
	authKeychainGet = func(_ string, account string) (string, error) {
		if account == TokenAccountForCorpID("corp_unrelated") ||
			account == TokenAccountForIdentity("corp_unrelated", "user_unrelated") {
			unrelatedReads.Add(1)
			return "", unrelatedErr
		}
		return "", nil
	}
	authValidateEntries = func(string) error {
		inventoryCalls.Add(1)
		return errors.New("unrelated orphan ciphertext is unreadable")
	}
	deviceLoginOnce = func(p *DeviceFlowProvider, ctx context.Context, attempt int) (*TokenData, error) {
		return p.loginOnce(ctx, attempt)
	}
	deviceRequestCode = func(*DeviceFlowProvider, context.Context) (*DeviceAuthResponse, error) {
		return &DeviceAuthResponse{DeviceCode: "device", UserCode: "user", Interval: 1}, nil
	}
	deviceWaitAuth = func(*DeviceFlowProvider, context.Context, *DeviceAuthResponse) (*DeviceTokenResponse, error) {
		return &DeviceTokenResponse{AuthCode: "target-code"}, nil
	}
	var exchangeCalls atomic.Int32
	deviceExchangeCode = func(*OAuthProvider, context.Context, string) (*TokenData, error) {
		exchangeCalls.Add(1)
		return &TokenData{
			AccessToken:  "target-access",
			RefreshToken: "target-refresh",
			CorpID:       "corp_target",
			UserID:       "user_target",
		}, nil
	}
	deviceCheckCLIAuth = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) {
		return &CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}, nil
	}
	var saveCalls atomic.Int32
	deviceSaveToken = func(_ string, data *TokenData) error {
		if !data.FreshAuthorization {
			t.Error("device flow did not mark exchanged token as a fresh authorization")
		}
		saveCalls.Add(1)
		return nil
	}
	provider := NewDeviceFlowProvider(configDir, nil)
	provider.Output = io.Discard
	provider.NoBrowser = true

	data, err := provider.Login(context.Background())
	if err != nil {
		t.Fatalf("DeviceFlowProvider.Login() error = %v", err)
	}
	if data == nil || data.UserID != "user_target" {
		t.Fatalf("DeviceFlowProvider.Login() data = %#v", data)
	}
	if got := exchangeCalls.Load(); got != 1 {
		t.Fatalf("remote exchange calls = %d, want 1", got)
	}
	if got := saveCalls.Load(); got != 1 {
		t.Fatalf("SaveTokenData calls = %d, want 1", got)
	}
	if got := unrelatedReads.Load(); got != 0 {
		t.Fatalf("unrelated profile token reads = %d, want 0", got)
	}
	if got := inventoryCalls.Load(); got != 0 {
		t.Fatalf("full inventory validation calls = %d, want 0", got)
	}
}
