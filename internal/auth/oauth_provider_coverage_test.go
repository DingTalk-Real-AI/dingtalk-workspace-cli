package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type oauthLoginFixture struct {
	configDir       string
	callbackBase    string
	provider        *OAuthProvider
	server          *httptest.Server
	exchangeEntered chan struct{}
	exchangeRelease chan struct{}
	statusEntered   chan struct{}
	statusRelease   chan struct{}
	statusCalls     atomic.Int32
	exchangeError   atomic.Bool
}

func newOAuthLoginFixture(t *testing.T, status func(int32) CLIAuthStatus) *oauthLoginFixture {
	t.Helper()
	SetClientID("")
	SetClientSecret("")
	resetClientIDFromMCP()
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
	})

	f := &oauthLoginFixture{
		exchangeEntered: make(chan struct{}),
		exchangeRelease: make(chan struct{}),
		statusEntered:   make(chan struct{}),
		statusRelease:   make(chan struct{}),
	}
	var exchangeOnce sync.Once
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ClientIDPath):
			_ = json.NewEncoder(w).Encode(ClientIDResponse{Success: true, Result: "test-client"})
		case strings.HasSuffix(r.URL.Path, MCPOAuthTokenPath):
			exchangeOnce.Do(func() {
				close(f.exchangeEntered)
				<-f.exchangeRelease
			})
			if f.exchangeError.Load() {
				http.Error(w, "exchange failed", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  "access",
				"refreshToken": "refresh",
				"expiresIn":    7200,
				"corpId":       "corp-1",
				"userId":       "user-1",
			})
		case strings.HasSuffix(r.URL.Path, CLIAuthEnabledPath):
			call := f.statusCalls.Add(1)
			if call == 1 {
				close(f.statusEntered)
				<-f.statusRelease
			}
			_ = json.NewEncoder(w).Encode(status(call))
		case strings.HasSuffix(r.URL.Path, SuperAdminPath):
			_ = json.NewEncoder(w).Encode(SuperAdminResponse{
				Success: true,
				Result:  []SuperAdmin{{StaffID: "admin-1", Name: "Admin"}},
			})
		case strings.HasSuffix(r.URL.Path, SendCliAuthApplyPath):
			_ = json.NewEncoder(w).Encode(SendApplyResponse{Success: true, Result: true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	f.configDir = setupMCPConfigDir(t, f.server.URL)

	oldClient := oauthHTTPClient
	oauthHTTPClient = f.server.Client()
	t.Cleanup(func() { oauthHTTPClient = oldClient })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.callbackBase = "http://" + listener.Addr().String()
	oldListen := oauthListen
	oauthListen = func(string, string) (net.Listener, error) { return listener, nil }
	t.Cleanup(func() { oauthListen = oldListen })

	f.provider = &OAuthProvider{
		configDir:  f.configDir,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Output:     io.Discard,
		httpClient: f.server.Client(),
		NoBrowser:  true,
	}
	return f
}

func httpGetBody(t *testing.T, rawURL string) (int, string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		t.Fatalf("GET %s: %v", rawURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(data)
}

func TestCrossPlatformCoverageOAuthLoginCallbackAndAPIs(t *testing.T) {
	f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus {
		return CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}
	})

	loginDone := make(chan struct {
		token *TokenData
		err   error
	}, 1)
	go func() {
		token, err := f.provider.Login(context.Background(), true)
		loginDone <- struct {
			token *TokenData
			err   error
		}{token, err}
	}()

	for _, path := range []string{"/api/superAdmin", "/api/sendApply?adminStaffId=admin-1", "/api/cliAuthEnabled"} {
		_, body := httpGetBody(t, f.callbackBase+path)
		if !strings.Contains(body, "授权尚未完成") {
			t.Fatalf("pre-auth %s = %q", path, body)
		}
	}
	_, body := httpGetBody(t, f.callbackBase+"/api/sendApply")
	if !strings.Contains(body, "adminStaffId") {
		t.Fatalf("missing admin response = %q", body)
	}
	_, body = httpGetBody(t, f.callbackBase+"/api/status")
	if !strings.Contains(body, "test-client") {
		t.Fatalf("initial status = %q", body)
	}
	if _, body = httpGetBody(t, f.callbackBase+"/success"); !strings.Contains(body, "<html") {
		t.Fatalf("success page = %q", body)
	}

	callbackDone := make(chan string, 1)
	go func() {
		_, callbackBody := httpGetBody(t, f.callbackBase+CallbackPath+"?code=good")
		callbackDone <- callbackBody
	}()
	<-f.exchangeEntered
	_, body = httpGetBody(t, f.callbackBase+CallbackPath+"?authCode=good")
	if !strings.Contains(body, "正在处理授权") {
		t.Fatalf("concurrent callback = %q", body)
	}
	close(f.exchangeRelease)
	<-f.statusEntered

	_, body = httpGetBody(t, f.callbackBase+CallbackPath+"?code=good")
	if !strings.Contains(body, "<html") {
		t.Fatalf("cached callback = %q", body)
	}
	_, body = httpGetBody(t, f.callbackBase+CallbackPath)
	if !strings.Contains(body, "<html") {
		t.Fatalf("cached no-code callback = %q", body)
	}
	_, body = httpGetBody(t, f.callbackBase+"/api/superAdmin")
	if !strings.Contains(body, "admin-1") {
		t.Fatalf("super admins = %q", body)
	}
	_, body = httpGetBody(t, f.callbackBase+"/api/sendApply?adminStaffId=admin-1")
	if !strings.Contains(body, "true") {
		t.Fatalf("send apply = %q", body)
	}
	_, body = httpGetBody(t, f.callbackBase+"/api/status")
	if !strings.Contains(body, "admin-1") || !strings.Contains(body, "true") {
		t.Fatalf("updated status = %q", body)
	}
	_, body = httpGetBody(t, f.callbackBase+"/api/cliAuthEnabled")
	if !strings.Contains(body, "cliAuthEnabled") {
		t.Fatalf("auth enabled API = %q", body)
	}

	close(f.statusRelease)
	if callbackBody := <-callbackDone; !strings.Contains(callbackBody, "<html") {
		t.Fatalf("callback body = %q", callbackBody)
	}
	result := <-loginDone
	if result.err != nil || result.token == nil || result.token.AccessToken != "access" {
		t.Fatalf("Login = %#v, %v", result.token, result.err)
	}

	if _, err := f.provider.Status(); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if token, err := f.provider.GetAccessToken(context.Background()); err != nil || token != "access" {
		t.Fatalf("GetAccessToken = %q, %v", token, err)
	}
	if err := f.provider.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
}

func TestCrossPlatformCoverageOAuthLoginMissingCallbackCode(t *testing.T) {
	f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus {
		return CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}
	})
	close(f.exchangeRelease)
	close(f.statusRelease)
	done := make(chan error, 1)
	go func() {
		_, err := f.provider.Login(context.Background(), true)
		done <- err
	}()
	status, body := httpGetBody(t, f.callbackBase+CallbackPath)
	if status != http.StatusBadRequest || strings.TrimSpace(body) == "" {
		t.Fatalf("missing callback = %d %q", status, body)
	}
	if err := <-done; err == nil {
		t.Fatal("missing callback code did not fail login")
	}
}

func TestCrossPlatformCoverageOAuthLoginEarlyAndListenerEdges(t *testing.T) {
	var buf bytes.Buffer
	p := &OAuthProvider{configDir: t.TempDir(), Output: &buf}
	if p.output() != &buf || (*OAuthProvider)(nil).output() != io.Discard {
		t.Fatal("provider output selection failed")
	}

	valid := &TokenData{
		AccessToken:  "valid",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
	}
	if err := SaveTokenData(p.configDir, valid); err != nil {
		t.Fatal(err)
	}
	got, err := p.Login(context.Background(), false)
	if err != nil || got.AccessToken != "valid" {
		t.Fatalf("silent Login = %#v, %v", got, err)
	}

	oldListen := oauthListen
	t.Cleanup(func() { oauthListen = oldListen })
	oauthListen = func(string, string) (net.Listener, error) { return nil, errors.New("listen") }
	SetClientID("client")
	SetClientSecret("secret")
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
	})
	if _, err := p.Login(context.Background(), true); err == nil || !strings.Contains(err.Error(), "listener") {
		t.Fatalf("listener failure = %v", err)
	}
}

func TestCrossPlatformCoverageOAuthLoginTimeoutAndServerError(t *testing.T) {
	oldTimeout := oauthLoginTimeout
	oldListen := oauthListen
	t.Cleanup(func() {
		oauthLoginTimeout = oldTimeout
		oauthListen = oldListen
	})
	oauthLoginTimeout = time.Millisecond
	f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus {
		return CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}
	})
	close(f.exchangeRelease)
	close(f.statusRelease)
	if _, err := f.provider.Login(context.Background(), true); err == nil || !strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Fatalf("login timeout = %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	oauthLoginTimeout = time.Second
	oauthListen = func(string, string) (net.Listener, error) { return listener, nil }
	if _, err := f.provider.Login(context.Background(), true); err == nil || !strings.Contains(err.Error(), "callback server") {
		t.Fatalf("callback server failure = %v", err)
	}
}

func TestCrossPlatformCoverageOAuthProviderOtherMethods(t *testing.T) {
	dir := t.TempDir()
	_ = DeleteTokenDataKeychain()
	t.Cleanup(func() { _ = DeleteTokenDataKeychain() })
	p := &OAuthProvider{configDir: dir, Output: io.Discard}
	if _, err := p.GetAccessToken(context.Background()); err == nil {
		t.Fatal("missing access token unexpectedly succeeded")
	}
	expired := &TokenData{
		AccessToken:  "expired",
		RefreshToken: "",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(-time.Hour),
		CorpID:       "corp-expired",
	}
	if err := SaveTokenData(dir, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetAccessToken(context.Background()); err == nil {
		t.Fatal("expired credentials unexpectedly succeeded")
	}
	if _, err := p.lockedRefresh(context.Background()); err == nil {
		t.Fatal("expired refresh token unexpectedly succeeded")
	}

	p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				"{\"accessToken\":\"new\",\"refreshToken\":\"refresh\",\"expiresIn\":7200,\"corpId\":\"corp-new\"}",
			)),
			Header: make(http.Header),
		}, nil
	})}
	SetClientID("direct-client")
	SetClientSecret("direct-secret")
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
	})
	token, err := p.ExchangeAuthCode(context.Background(), "code", "uid")
	if err != nil || token.UserID != "uid" {
		t.Fatalf("ExchangeAuthCode = %#v, %v", token, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestCrossPlatformCoverageOAuthPersistConfigEdges(t *testing.T) {
	p := &OAuthProvider{configDir: t.TempDir(), logger: slog.Default()}
	SetClientID("")
	SetClientSecret("")
	p.persistAppConfigIfNeeded()
	SetClientID(DefaultClientID)
	SetClientSecret("secret")
	p.persistAppConfigIfNeeded()
	SetClientID("custom-client")
	SetClientSecret("custom-secret")
	p.persistAppConfigIfNeeded()
	cfg, err := LoadAppConfig(p.configDir)
	if err != nil || cfg.ClientID != "custom-client" {
		t.Fatalf("persisted config = %#v, %v", cfg, err)
	}

	badDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(badDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	(&OAuthProvider{configDir: badDir, logger: slog.Default()}).persistAppConfigIfNeeded()
	SetClientID("")
	SetClientSecret("")
}

func TestCrossPlatformCoverageBuildOAuthAuthURLTarget(t *testing.T) {
	raw := buildAuthURL("client", "http://localhost/callback", "corp")
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("corpId") != "corp" {
		t.Fatalf("auth URL = %q", raw)
	}
}

func TestCrossPlatformCoverageOAuthLoginDenialAndPolling(t *testing.T) {
	oldPoll := oauthPollInterval
	oldApproval := oauthApprovalTimeout
	oldPause := oauthSuccessPause
	oldBrowser := oauthOpenBrowser
	t.Cleanup(func() {
		oauthPollInterval = oldPoll
		oauthApprovalTimeout = oldApproval
		oauthSuccessPause = oldPause
		oauthOpenBrowser = oldBrowser
	})
	oauthPollInterval = time.Millisecond
	oauthSuccessPause = 0
	oauthOpenBrowser = func(string) error { return errors.New("browser") }

	terminal := []struct {
		name   string
		status CLIAuthStatus
	}{
		{"user forbidden", CLIAuthStatus{Success: true, Result: &CLIAuthResult{UserScope: "forbidden"}}},
		{"user not allowed", CLIAuthStatus{Success: true, Result: &CLIAuthResult{UserScope: "specified"}}},
		{"channel required", CLIAuthStatus{Success: true, Result: &CLIAuthResult{ChannelScope: "specified"}}},
		{"enterprise message", CLIAuthStatus{Success: false, ErrorCode: "ENTERPRISE_NOT_AUTHORIZED", ErrorMsg: "enterprise denied"}},
	}
	for _, tt := range terminal {
		t.Run(tt.name, func(t *testing.T) {
			f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus { return tt.status })
			close(f.exchangeRelease)
			close(f.statusRelease)
			f.provider.NoBrowser = false
			done := make(chan error, 1)
			go func() {
				_, err := f.provider.Login(context.Background(), true)
				done <- err
			}()
			_, _ = httpGetBody(t, f.callbackBase+CallbackPath+"?code=denied")
			if err := <-done; err == nil {
				t.Fatal("denied login succeeded")
			}
		})
	}

	t.Run("channel not allowed", func(t *testing.T) {
		t.Setenv("DWS_CHANNEL", "blocked")
		f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus {
			return CLIAuthStatus{Success: true, Result: &CLIAuthResult{ChannelScope: "specified", AllowedChannels: []string{"allowed"}}}
		})
		close(f.exchangeRelease)
		close(f.statusRelease)
		done := make(chan error, 1)
		go func() {
			_, err := f.provider.Login(context.Background(), true)
			done <- err
		}()
		_, _ = httpGetBody(t, f.callbackBase+CallbackPath+"?code=denied")
		if err := <-done; err == nil {
			t.Fatal("channel-denied login succeeded")
		}
	})

	t.Run("approval timeout", func(t *testing.T) {
		oauthApprovalTimeout = 2 * time.Millisecond
		f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus {
			return CLIAuthStatus{Success: true, Result: &CLIAuthResult{}}
		})
		close(f.exchangeRelease)
		close(f.statusRelease)
		done := make(chan error, 1)
		go func() {
			_, err := f.provider.Login(context.Background(), true)
			done <- err
		}()
		_, _ = httpGetBody(t, f.callbackBase+CallbackPath+"?code=pending")
		if err := <-done; err == nil {
			t.Fatal("approval timeout login succeeded")
		}
		oauthApprovalTimeout = oldApproval
	})

	t.Run("poll becomes enabled", func(t *testing.T) {
		f := newOAuthLoginFixture(t, func(call int32) CLIAuthStatus {
			return CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: call > 1}}
		})
		close(f.exchangeRelease)
		close(f.statusRelease)
		done := make(chan error, 1)
		go func() {
			_, err := f.provider.Login(context.Background(), true)
			done <- err
		}()
		_, _ = httpGetBody(t, f.callbackBase+CallbackPath+"?code=pending")
		if err := <-done; err != nil {
			t.Fatalf("poll-enabled login failed: %v", err)
		}
	})

	t.Run("context canceled while pending", func(t *testing.T) {
		f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus {
			return CLIAuthStatus{Success: true, Result: &CLIAuthResult{}}
		})
		close(f.exchangeRelease)
		close(f.statusRelease)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := f.provider.Login(ctx, true)
			done <- err
		}()
		_, _ = httpGetBody(t, f.callbackBase+CallbackPath+"?code=pending")
		cancel()
		if err := <-done; err == nil {
			t.Fatal("canceled pending login succeeded")
		}
	})
}

func TestCrossPlatformCoverageOAuthLoginExchangeFailure(t *testing.T) {
	f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus {
		return CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}
	})
	f.exchangeError.Store(true)
	close(f.exchangeRelease)
	close(f.statusRelease)
	done := make(chan error, 1)
	go func() {
		_, err := f.provider.Login(context.Background(), true)
		done <- err
	}()
	_, body := httpGetBody(t, f.callbackBase+CallbackPath+"?code=bad")
	if !strings.Contains(body, "failed") {
		t.Fatalf("exchange failure page = %q", body)
	}
	if err := <-done; err == nil {
		t.Fatal("exchange failure login succeeded")
	}
}

func TestCrossPlatformCoverageOAuthRefreshAndParsingEdges(t *testing.T) {
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")
	dir := t.TempDir()
	p := &OAuthProvider{configDir: dir, Output: io.Discard}
	oldOAuthClient := oauthHTTPClient
	responseBody := "{\"accessToken\":\"new-access\",\"refreshToken\":\"new-refresh\",\"expiresIn\":7200,\"corpId\":\"new-corp\"}"
	p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(responseBody)), Header: make(http.Header)}, nil
	})}
	SetClientID("direct")
	SetClientSecret("secret")
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
		oauthHTTPClient = oldOAuthClient
	})

	for name, body := range map[string][]byte{
		"malformed": []byte("{"),
		"missing":   []byte("{}"),
	} {
		t.Run("direct "+name, func(t *testing.T) {
			if _, err := p.parseTokenResponse(body); err == nil {
				t.Fatal("invalid direct token parsed")
			}
		})
		t.Run("mcp "+name, func(t *testing.T) {
			if _, err := p.parseMCPTokenResponse(body); err == nil {
				t.Fatal("invalid MCP token parsed")
			}
		})
	}
	if parsed, err := p.parseTokenResponse([]byte("{\"accessToken\":\"a\",\"expiresIn\":0}")); err != nil || parsed.ExpiresAt.IsZero() {
		t.Fatalf("default direct expiry = %#v, %v", parsed, err)
	}
	if parsed, err := p.parseMCPTokenResponse([]byte("{\"accessToken\":\"a\",\"expiresIn\":0,\"corpName\":\"Corp\"}")); err != nil || parsed.CorpName != "Corp" {
		t.Fatalf("default MCP expiry = %#v, %v", parsed, err)
	}

	if _, err := p.postJSON(context.Background(), "https://example.test", map[string]any{"bad": func() {}}); err == nil {
		t.Fatal("unmarshalable request succeeded")
	}
	if _, err := p.postJSON(context.Background(), "://bad", map[string]string{}); err == nil {
		t.Fatal("invalid request URL succeeded")
	}
	p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network")
	})}
	if _, err := p.postJSON(context.Background(), "https://example.test", map[string]string{}); err == nil {
		t.Fatal("network request succeeded")
	}
	p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errorReader{}), Header: make(http.Header)}, nil
	})}
	if _, err := p.postJSON(context.Background(), "https://example.test", map[string]string{}); err == nil {
		t.Fatal("response read failure succeeded")
	}
	p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 250))), Header: make(http.Header)}, nil
	})}
	if _, err := p.postJSON(context.Background(), "https://example.test", map[string]string{}); err == nil {
		t.Fatal("HTTP failure succeeded")
	}

	p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(responseBody)), Header: make(http.Header)}, nil
	})}
	original := &TokenData{
		ClientID: "direct", Source: "direct", RefreshToken: "refresh", PersistentCode: "pc",
		CorpID: "corp", CorpName: "Original", UserID: "user", UserName: "Name",
	}
	if err := SaveClientSecret("direct", "secret"); err != nil {
		t.Fatal(err)
	}
	updated, err := p.refreshWithRefreshToken(context.Background(), original)
	if err != nil || updated.CorpID != "corp" || updated.PersistentCode != "pc" || updated.CorpName != "Original" {
		t.Fatalf("direct refresh = %#v, %v", updated, err)
	}
	missing := &TokenData{ClientID: "missing-client", RefreshToken: "refresh"}
	SetClientID("")
	SetClientSecret("")
	if _, err := p.refreshWithRefreshToken(context.Background(), missing); err == nil {
		t.Fatal("refresh without credentials succeeded")
	}

	mcpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, responseBody)
	}))
	defer mcpSrv.Close()
	configDir := setupMCPConfigDir(t, mcpSrv.URL)
	resetAppConfigCache()
	oauthHTTPClient = mcpSrv.Client()
	mcpProvider := &OAuthProvider{configDir: configDir, httpClient: mcpSrv.Client()}
	mcpOriginal := &TokenData{ClientID: "mcp-client", Source: "mcp", RefreshToken: "refresh", CorpID: "corp"}
	if updated, err := mcpProvider.refreshViaMCP(context.Background(), mcpOriginal); err != nil || updated.Source != "mcp" {
		t.Fatalf("MCP refresh = %#v, %v", updated, err)
	}
	if _, err := mcpProvider.refreshViaMCP(context.Background(), &TokenData{Source: "mcp"}); err == nil {
		t.Fatalf("MCP refresh without client ID succeeded (fallback client ID %q)", ClientID())
	}
}

func TestCrossPlatformCoverageOAuthProviderHighLevelEdges(t *testing.T) {
	oldLoad := oauthLoadToken
	oldAcquire := oauthAcquireLock
	oldMark := oauthMarkProfile
	oldFetch := oauthFetchClientID
	oldExchange := oauthExchange
	oldSave := oauthSaveToken
	oldRefresh := oauthRefreshToken
	oldSaveSecret := oauthSaveClientSecret
	oldSaveLocked := oauthSaveTokenLocked
	t.Cleanup(func() {
		oauthLoadToken = oldLoad
		oauthAcquireLock = oldAcquire
		oauthMarkProfile = oldMark
		oauthFetchClientID = oldFetch
		oauthExchange = oldExchange
		oauthSaveToken = oldSave
		oauthRefreshToken = oldRefresh
		oauthSaveClientSecret = oldSaveSecret
		oauthSaveTokenLocked = oldSaveLocked
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
	})
	fail := errors.New("fail")
	p := &OAuthProvider{configDir: t.TempDir(), logger: slog.Default(), Output: io.Discard}

	valid := &TokenData{AccessToken: "valid", ExpiresAt: time.Now().Add(time.Hour)}
	oauthLoadToken = func(string) (*TokenData, error) { return valid, nil }
	if got, err := p.Login(context.Background(), false); err != nil || got != valid {
		t.Fatalf("silent valid login = %#v %v", got, err)
	}
	expired := &TokenData{AccessToken: "old", ExpiresAt: time.Now().Add(-time.Hour), RefreshToken: "refresh", RefreshExpAt: time.Now().Add(time.Hour)}
	oauthLoadToken = func(string) (*TokenData, error) { return expired, nil }
	oauthAcquireLock = func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil }
	refreshed := &TokenData{AccessToken: "new", ExpiresAt: time.Now().Add(time.Hour)}
	oauthRefreshToken = func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) { return refreshed, nil }
	if got, err := p.Login(context.Background(), false); err != nil || got != refreshed {
		t.Fatalf("silent refresh login = %#v %v", got, err)
	}
	oauthAcquireLock = func(context.Context, string) (*DualLock, error) { return nil, fail }
	oauthFetchClientID = func(context.Context) (string, error) { return "", fail }
	if _, err := p.Login(context.Background(), false); !errors.Is(err, fail) {
		t.Fatalf("refresh fallback fetch error = %v", err)
	}

	oauthLoadToken = func(string) (*TokenData, error) { return nil, fail }
	if _, err := p.GetAccessToken(context.Background()); err == nil {
		t.Fatal("missing access token succeeded")
	}
	oauthLoadToken = func(string) (*TokenData, error) { return valid, nil }
	if got, err := p.GetAccessToken(context.Background()); err != nil || got != "valid" {
		t.Fatalf("valid access token = %q %v", got, err)
	}
	oauthLoadToken = func(string) (*TokenData, error) { return expired, nil }
	oauthAcquireLock = func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil }
	oauthRefreshToken = func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) { return refreshed, nil }
	if got, err := p.GetAccessToken(context.Background()); err != nil || got != "new" {
		t.Fatalf("refreshed access token = %q %v", got, err)
	}
	marked := false
	oauthRefreshToken = func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) { return nil, fail }
	oauthMarkProfile = func(string, string, string) error { marked = true; return fail }
	if _, err := p.GetAccessToken(context.Background()); err == nil || !marked {
		t.Fatalf("failed refresh = %v marked=%v", err, marked)
	}
	marked = false
	noRefresh := &TokenData{AccessToken: "old", ExpiresAt: time.Now().Add(-time.Hour), CorpID: "corp"}
	oauthLoadToken = func(string) (*TokenData, error) { return noRefresh, nil }
	if _, err := p.GetAccessToken(context.Background()); err == nil || !marked {
		t.Fatalf("expired access token = %v marked=%v", err, marked)
	}

	oauthAcquireLock = func(context.Context, string) (*DualLock, error) { return nil, fail }
	if _, err := p.lockedRefresh(context.Background()); !errors.Is(err, fail) {
		t.Fatalf("locked refresh acquire error = %v", err)
	}
	oauthAcquireLock = func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil }
	oauthLoadToken = func(string) (*TokenData, error) { return nil, fail }
	if _, err := p.lockedRefresh(context.Background()); !errors.Is(err, fail) {
		t.Fatalf("locked refresh load error = %v", err)
	}
	for _, waited := range []bool{false, true} {
		oauthAcquireLock = func(context.Context, string) (*DualLock, error) { return &DualLock{Waited: waited}, nil }
		oauthLoadToken = func(string) (*TokenData, error) { return valid, nil }
		if got, err := p.lockedRefresh(context.Background()); err != nil || got != valid {
			t.Fatalf("double-check valid token waited=%v: %#v %v", waited, got, err)
		}
	}
	oauthAcquireLock = func(context.Context, string) (*DualLock, error) { return &DualLock{}, nil }
	oauthLoadToken = func(string) (*TokenData, error) { return noRefresh, nil }
	if _, err := p.lockedRefresh(context.Background()); err == nil {
		t.Fatal("expired refresh token succeeded")
	}
	oauthLoadToken = func(string) (*TokenData, error) { return expired, nil }
	oauthRefreshToken = func(*OAuthProvider, context.Context, *TokenData) (*TokenData, error) { return refreshed, nil }
	if got, err := p.lockedRefresh(context.Background()); err != nil || got != refreshed {
		t.Fatalf("locked token refresh = %#v %v", got, err)
	}

	oauthExchange = func(*OAuthProvider, context.Context, string) (*TokenData, error) { return nil, fail }
	if _, err := p.ExchangeAuthCode(context.Background(), "code", "user"); !errors.Is(err, fail) {
		t.Fatalf("external exchange error = %v", err)
	}
	oauthExchange = func(*OAuthProvider, context.Context, string) (*TokenData, error) { return &TokenData{}, nil }
	oauthSaveToken = func(string, *TokenData) error { return fail }
	if _, err := p.ExchangeAuthCode(context.Background(), "code", "user"); !errors.Is(err, fail) {
		t.Fatalf("external exchange save error = %v", err)
	}

	oauthSaveClientSecret = func(string, string) error { return fail }
	SetClientID("direct")
	SetClientSecret("secret")
	p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"accessToken":"access","refreshToken":"refresh"}`)), Header: make(http.Header)}, nil
	})}
	var warnings bytes.Buffer
	p.Output = &warnings
	if _, err := p.exchangeCode(context.Background(), "code"); err != nil || !strings.Contains(warnings.String(), "Warning") {
		t.Fatalf("direct exchange secret warning = %v %q", err, warnings.String())
	}
	oauthSaveTokenLocked = func(string, *TokenData) error { return fail }
	if _, err := p.refreshWithRefreshToken(context.Background(), &TokenData{ClientID: "direct", RefreshToken: "refresh"}); !errors.Is(err, fail) {
		t.Fatalf("direct refresh save error = %v", err)
	}
	if _, err := oldRefresh(p, context.Background(), &TokenData{ClientID: "direct", RefreshToken: "refresh"}); !errors.Is(err, fail) {
		t.Fatalf("default refresh hook = %v", err)
	}
}

func TestCrossPlatformCoverageOAuthCallbackRemainingEdges(t *testing.T) {
	oldCheck := oauthCheckStatus
	oldAdmins := oauthGetAdmins
	oldApply := oauthSendApply
	oldSave := oauthSaveToken
	oldPoll := oauthPollInterval
	oldApproval := oauthApprovalTimeout
	oldLogin := oauthLoginTimeout
	oldPause := oauthSuccessPause
	oldSleep := oauthSleep
	t.Cleanup(func() {
		oauthCheckStatus = oldCheck
		oauthGetAdmins = oldAdmins
		oauthSendApply = oldApply
		oauthSaveToken = oldSave
		oauthPollInterval = oldPoll
		oauthApprovalTimeout = oldApproval
		oauthLoginTimeout = oldLogin
		oauthSuccessPause = oldPause
		oauthSleep = oldSleep
	})
	oauthApprovalTimeout = time.Second
	oauthLoginTimeout = time.Second
	oauthSuccessPause = 0
	oauthSleep = func(time.Duration) {}

	type loginResult struct {
		token *TokenData
		err   error
	}
	startLogin := func(ctx context.Context, f *oauthLoginFixture) <-chan loginResult {
		t.Helper()
		done := make(chan loginResult, 1)
		go func() {
			token, err := f.provider.Login(ctx, true)
			done <- loginResult{token: token, err: err}
		}()
		return done
	}
	finishExchange := func(t *testing.T, f *oauthLoginFixture, code string) string {
		t.Helper()
		bodyCh := make(chan string, 1)
		go func() {
			_, body := httpGetBody(t, f.callbackBase+CallbackPath+"?code="+url.QueryEscape(code))
			bodyCh <- body
		}()
		<-f.exchangeEntered
		close(f.exchangeRelease)
		return <-bodyCh
	}

	t.Run("switch organization and cached disabled pages", func(t *testing.T) {
		oauthPollInterval = 200 * time.Millisecond
		var checks atomic.Int32
		oauthCheckStatus = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) {
			if checks.Add(1) == 1 {
				return &CLIAuthStatus{Success: true, Result: &CLIAuthResult{}}, nil
			}
			return &CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}, nil
		}
		f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus { return CLIAuthStatus{} })
		done := startLogin(context.Background(), f)
		if body := finishExchange(t, f, "first"); !strings.Contains(body, "<html") {
			t.Fatalf("disabled callback body = %q", body)
		}
		if _, body := httpGetBody(t, f.callbackBase+CallbackPath+"?code=first"); !strings.Contains(body, "<html") {
			t.Fatalf("cached disabled callback = %q", body)
		}
		if _, body := httpGetBody(t, f.callbackBase+CallbackPath); !strings.Contains(body, "<html") {
			t.Fatalf("cached disabled no-code callback = %q", body)
		}
		if _, body := httpGetBody(t, f.callbackBase+CallbackPath+"?code=second"); !strings.Contains(body, "<html") {
			t.Fatalf("switched callback = %q", body)
		}
		result := <-done
		if result.err != nil || result.token == nil || result.token.AccessToken != "access" {
			t.Fatalf("switched login = %#v %v", result.token, result.err)
		}
	})

	t.Run("callback and API errors", func(t *testing.T) {
		oauthPollInterval = time.Hour
		fail := errors.New("hook failure")
		oauthCheckStatus = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) { return nil, fail }
		oauthGetAdmins = func(context.Context, string) (*SuperAdminResponse, error) { return nil, fail }
		oauthSendApply = func(context.Context, string, string) (*SendApplyResponse, error) { return nil, fail }
		ctx, cancel := context.WithCancel(context.Background())
		f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus { return CLIAuthStatus{} })
		done := startLogin(ctx, f)
		finishExchange(t, f, "errors")
		for _, path := range []string{"/api/superAdmin", "/api/sendApply?adminStaffId=admin", "/api/cliAuthEnabled"} {
			if _, body := httpGetBody(t, f.callbackBase+path); !strings.Contains(body, "hook failure") {
				t.Fatalf("API error %s = %q", path, body)
			}
		}
		cancel()
		if result := <-done; !errors.Is(result.err, context.Canceled) {
			t.Fatalf("canceled error login = %v", result.err)
		}
	})

	t.Run("apply sent polling", func(t *testing.T) {
		oauthPollInterval = 5 * time.Millisecond
		oauthCheckStatus = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) {
			return &CLIAuthStatus{Success: true, Result: &CLIAuthResult{}}, nil
		}
		oauthSendApply = func(context.Context, string, string) (*SendApplyResponse, error) {
			return &SendApplyResponse{Success: true, Result: true}, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus { return CLIAuthStatus{} })
		var output bytes.Buffer
		f.provider.Output = &output
		done := startLogin(ctx, f)
		finishExchange(t, f, "apply")
		if _, body := httpGetBody(t, f.callbackBase+"/api/sendApply?adminStaffId=admin"); !strings.Contains(body, "true") {
			t.Fatalf("apply response = %q", body)
		}
		time.Sleep(20 * time.Millisecond)
		cancel()
		if result := <-done; !errors.Is(result.err, context.Canceled) {
			t.Fatalf("canceled apply login = %v", result.err)
		}
		if !strings.Contains(output.String(), "Waiting for admin approval") && !strings.Contains(output.String(), "等待管理员审批中") {
			t.Fatalf("apply polling output = %q", output.String())
		}
	})

	t.Run("enterprise denial default", func(t *testing.T) {
		oauthPollInterval = time.Hour
		oauthCheckStatus = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) {
			return &CLIAuthStatus{Success: false, ErrorCode: "ENTERPRISE_NOT_AUTHORIZED"}, nil
		}
		f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus { return CLIAuthStatus{} })
		done := startLogin(context.Background(), f)
		finishExchange(t, f, "enterprise")
		if result := <-done; result.err == nil || !strings.Contains(result.err.Error(), "企业安全认证") {
			t.Fatalf("enterprise denial = %v", result.err)
		}
	})

	t.Run("initial context cancellation", func(t *testing.T) {
		f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus { return CLIAuthStatus{} })
		SetClientID("direct")
		SetClientSecret("secret")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := f.provider.Login(ctx, true); !errors.Is(err, context.Canceled) {
			t.Fatalf("initial cancellation = %v", err)
		}
	})

	t.Run("save failure", func(t *testing.T) {
		oauthCheckStatus = func(*OAuthProvider, context.Context, string) (*CLIAuthStatus, error) {
			return &CLIAuthStatus{Success: true, Result: &CLIAuthResult{CLIAuthEnabled: true}}, nil
		}
		fail := errors.New("save failure")
		oauthSaveToken = func(string, *TokenData) error { return fail }
		f := newOAuthLoginFixture(t, func(int32) CLIAuthStatus { return CLIAuthStatus{} })
		done := startLogin(context.Background(), f)
		finishExchange(t, f, "save")
		if result := <-done; !errors.Is(result.err, fail) {
			t.Fatalf("save failure login = %v", result.err)
		}
	})
}

func TestCrossPlatformCoverageOAuthHelperRemainingEdges(t *testing.T) {
	oldClient := oauthHTTPClient
	oldRequest := oauthNewRequest
	oldRetry := oauthRetryAfter
	oldSaveLocked := oauthSaveTokenLocked
	t.Cleanup(func() {
		oauthHTTPClient = oldClient
		oauthNewRequest = oldRequest
		oauthRetryAfter = oldRetry
		oauthSaveTokenLocked = oldSaveLocked
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
	})
	fail := errors.New("fail")
	networkClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, fail })}
	responseClient := func(body string) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}
	}
	p := &OAuthProvider{configDir: t.TempDir(), Output: io.Discard, httpClient: networkClient}
	SetClientID("direct")
	SetClientSecret("secret")
	if _, err := p.exchangeCode(context.Background(), "code"); !errors.Is(err, fail) {
		t.Fatalf("direct exchange request error = %v", err)
	}
	p.httpClient = responseClient("{")
	if _, err := p.exchangeCode(context.Background(), "code"); err == nil {
		t.Fatal("malformed direct exchange succeeded")
	}
	oauthHTTPClient = responseClient(`{"accessToken":"wrapper","refreshToken":"refresh"}`)
	if got, err := ExchangeCodeForToken(context.Background(), p.configDir, "code"); err != nil || got.AccessToken != "wrapper" {
		t.Fatalf("exchange wrapper = %#v %v", got, err)
	}

	SetClientIDFromMCP("mcp-client")
	p.httpClient = networkClient
	if _, err := p.exchangeCodeViaMCP(context.Background(), "code"); !errors.Is(err, fail) {
		t.Fatalf("MCP exchange request error = %v", err)
	}
	p.httpClient = responseClient("{")
	if _, err := p.exchangeCodeViaMCP(context.Background(), "code"); err == nil {
		t.Fatal("malformed MCP exchange succeeded")
	}
	resetClientIDFromMCP()
	SetClientID("direct")
	SetClientSecret("secret")
	legacy := &TokenData{RefreshToken: "refresh"}
	p.httpClient = networkClient
	if _, err := p.refreshWithRefreshToken(context.Background(), legacy); !errors.Is(err, fail) {
		t.Fatalf("direct refresh request error = %v", err)
	}
	p.httpClient = responseClient("{")
	if _, err := p.refreshWithRefreshToken(context.Background(), legacy); err == nil {
		t.Fatal("malformed direct refresh succeeded")
	}

	SetClientIDFromMCP("mcp-client")
	mcpData := &TokenData{ClientID: "mcp-client", Source: "mcp", RefreshToken: "refresh"}
	p.httpClient = networkClient
	if _, err := p.refreshWithRefreshToken(context.Background(), mcpData); !errors.Is(err, fail) {
		t.Fatalf("MCP refresh dispatch error = %v", err)
	}
	p.httpClient = responseClient("{")
	if _, err := p.refreshViaMCP(context.Background(), mcpData); err == nil {
		t.Fatal("malformed MCP refresh succeeded")
	}
	p.httpClient = responseClient(`{"accessToken":"new","refreshToken":"refresh","persistentCode":"pc"}`)
	oauthSaveTokenLocked = func(string, *TokenData) error { return fail }
	if _, err := p.refreshViaMCP(context.Background(), mcpData); !errors.Is(err, fail) {
		t.Fatalf("MCP refresh save error = %v", err)
	}
	oauthSaveTokenLocked = oldSaveLocked

	if parsed, err := p.parseTokenResponse([]byte(`{"accessToken":"a","persistentCode":"pc"}`)); err != nil || parsed.PersistentCode != "pc" {
		t.Fatalf("direct persistent code = %#v %v", parsed, err)
	}
	if _, err := p.parseMCPTokenResponse([]byte(`{"errorCode":"bad","errorMsg":"failed"}`)); err == nil {
		t.Fatal("MCP business error parsed")
	}
	if parsed, err := p.parseMCPTokenResponse([]byte(`{"accessToken":"a","persistentCode":"pc"}`)); err != nil || parsed.PersistentCode != "pc" {
		t.Fatalf("MCP persistent code = %#v %v", parsed, err)
	}
	if !strings.Contains(renderEnterpriseDeniedHTML(""), defaultEnterpriseDeniedMsg) {
		t.Fatal("default enterprise denial message missing")
	}

	oauthNewRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) { return nil, fail }
	p.httpClient = responseClient("{}")
	if _, err := p.doCheckCLIAuthEnabled(context.Background(), "token"); !errors.Is(err, fail) {
		t.Fatalf("CLI status request error = %v", err)
	}
	if _, err := doGetSuperAdmins(context.Background(), "token"); !errors.Is(err, fail) {
		t.Fatalf("admins request error = %v", err)
	}
	if _, err := doSendCliAuthApply(context.Background(), "token", "admin"); !errors.Is(err, fail) {
		t.Fatalf("apply request error = %v", err)
	}
	if _, err := doFetchClientIDFromMCP(context.Background()); !errors.Is(err, fail) {
		t.Fatalf("client-ID request error = %v", err)
	}
	oauthNewRequest = oldRequest

	for name, client := range map[string]*http.Client{
		"network": networkClient,
		"read": {Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errorReader{}), Header: make(http.Header)}, nil
		})},
	} {
		t.Run(name, func(t *testing.T) {
			p.httpClient = client
			oauthHTTPClient = client
			if _, err := p.doCheckCLIAuthEnabled(context.Background(), "token"); err == nil {
				t.Fatal("CLI status transport failure succeeded")
			}
			if _, err := doGetSuperAdmins(context.Background(), "token"); err == nil {
				t.Fatal("admins transport failure succeeded")
			}
			if _, err := doSendCliAuthApply(context.Background(), "token", "admin"); err == nil {
				t.Fatal("apply transport failure succeeded")
			}
			if _, err := doFetchClientIDFromMCP(context.Background()); err == nil {
				t.Fatal("client-ID transport failure succeeded")
			}
		})
	}

	for name, call := range map[string]func() error{
		"status": func() error { _, err := p.doCheckCLIAuthEnabled(context.Background(), "token"); return err },
		"admins": func() error { _, err := doGetSuperAdmins(context.Background(), "token"); return err },
		"apply":  func() error { _, err := doSendCliAuthApply(context.Background(), "token", "admin"); return err },
		"client": func() error { _, err := doFetchClientIDFromMCP(context.Background()); return err },
	} {
		t.Run("malformed "+name, func(t *testing.T) {
			client := responseClient("{")
			p.httpClient = client
			oauthHTTPClient = client
			if err := call(); err == nil {
				t.Fatal("malformed MCP response succeeded")
			}
		})
	}
	oauthHTTPClient = responseClient(`{"success":false,"errorCode":"bad","errorMsg":"failed"}`)
	if _, err := doFetchClientIDFromMCP(context.Background()); err == nil {
		t.Fatal("failed client-ID response succeeded")
	}

	oauthRetryAfter = func(time.Duration) <-chan time.Time { return make(chan time.Time) }
	p.httpClient = networkClient
	oauthHTTPClient = networkClient
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.CheckCLIAuthEnabled(canceled, "token"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled CLI status retry = %v", err)
	}
	if _, err := GetSuperAdmins(canceled, "token"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admins retry = %v", err)
	}
	if _, err := SendCliAuthApply(canceled, "token", "admin"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled apply retry = %v", err)
	}
	if _, err := FetchClientIDFromMCP(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled client-ID retry = %v", err)
	}
}
