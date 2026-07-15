package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/authretry"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestRuntimeRunnerAuthRefreshFromHTTP401RetriesCurrentEndpoint(t *testing.T) {
	configDir := setupRuntimeAuthRefreshTest(t, "corp-http-refresh", "old-access")
	originalCause := errors.New("token rejected by HTTP 401")
	var refreshes atomic.Int32
	var invalidations atomic.Int32
	var overlayTokenCache atomic.Value
	overlayTokenCache.Store("old-access")
	edition.Override(&edition.Hooks{
		OnAuthError: func(_ string, err error) error {
			var typed *apperrors.Error
			if errors.As(err, &typed) && typed.Reason == "http_401" {
				return &authretry.AuthRefreshRequired{Cause: originalCause}
			}
			return nil
		},
		TokenProvider: func(_ context.Context, fallback func() (string, error)) (string, error) {
			if cached, _ := overlayTokenCache.Load().(string); cached != "" {
				return cached, nil
			}
			token, err := fallback()
			if err == nil {
				overlayTokenCache.Store(token)
			}
			return token, err
		},
		InvalidateAuthCaches: func() {
			overlayTokenCache.Store("")
			invalidations.Add(1)
		},
	})
	stubRejectedTokenRefresh(t, func(_ context.Context, gotConfigDir, rejected string) (string, error) {
		refreshes.Add(1)
		if gotConfigDir != configDir || rejected != "old-access" {
			t.Fatalf("refresh args = (%q, %q), want (%q, old-access)", gotConfigDir, rejected, configDir)
		}
		return rotateRuntimeTestToken(t, gotConfigDir, "new-access"), nil
	})

	var calls atomic.Int32
	server := newAuthRefreshMCPServer(t, func(token string) (int, map[string]any, bool) {
		calls.Add(1)
		if token == "old-access" {
			return http.StatusUnauthorized, nil, false
		}
		if got := invalidations.Load(); got != 1 {
			t.Errorf("cache invalidations before retry = %d, want 1", got)
		}
		return http.StatusOK, map[string]any{"success": true, "tokenGeneration": "new"}, false
	})
	defer server.Close()

	result, err := newAuthRefreshRuntimeRunner(server).executeInvocation(context.Background(), server.URL, authRefreshTestInvocation())
	if err != nil {
		t.Fatalf("executeInvocation() error = %v", err)
	}
	if result.Response["endpoint"] != server.URL {
		t.Fatalf("retry endpoint = %#v, want %q", result.Response["endpoint"], server.URL)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("MCP calls = %d, want 2", got)
	}
	if got := refreshes.Load(); got != 1 {
		t.Fatalf("refreshes = %d, want 1", got)
	}
	if got := invalidations.Load(); got != 1 {
		t.Fatalf("cache invalidations = %d, want 1", got)
	}
}

func TestRuntimeRunnerAuthRefreshFromMCPBusinessMarker(t *testing.T) {
	setupRuntimeAuthRefreshTest(t, "corp-mcp-refresh", "old-access")
	originalCause := errors.New("TOKEN_VERIFIED_FAILED")
	var refreshes atomic.Int32
	edition.Override(&edition.Hooks{
		ClassifyToolResult: func(content map[string]any) error {
			if content["code"] == "TOKEN_VERIFIED_FAILED" {
				return &authretry.AuthRefreshRequired{Cause: originalCause}
			}
			return nil
		},
	})
	stubRejectedTokenRefresh(t, func(_ context.Context, configDir, rejected string) (string, error) {
		refreshes.Add(1)
		if rejected != "old-access" {
			t.Fatalf("rejected token = %q, want old-access", rejected)
		}
		return rotateRuntimeTestToken(t, configDir, "new-access"), nil
	})

	var calls atomic.Int32
	server := newAuthRefreshMCPServer(t, func(token string) (int, map[string]any, bool) {
		calls.Add(1)
		if token == "old-access" {
			return http.StatusOK, map[string]any{"success": false, "code": "TOKEN_VERIFIED_FAILED"}, true
		}
		return http.StatusOK, map[string]any{"success": true}, false
	})
	defer server.Close()

	if _, err := newAuthRefreshRuntimeRunner(server).executeInvocation(context.Background(), server.URL, authRefreshTestInvocation()); err != nil {
		t.Fatalf("executeInvocation() error = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("MCP calls = %d, want 2", got)
	}
	if got := refreshes.Load(); got != 1 {
		t.Fatalf("refreshes = %d, want 1", got)
	}
}

func TestRuntimeRunnerAuthRefreshFailurePurgesCredentialAndPreservesCause(t *testing.T) {
	configDir := setupRuntimeAuthRefreshTest(t, "corp-refresh-failure", "old-access")
	originalCause := errors.New("original auth failure")
	edition.Override(&edition.Hooks{OnAuthError: func(_ string, _ error) error {
		return &authretry.AuthRefreshRequired{Cause: originalCause}
	}})
	stubRejectedTokenRefresh(t, func(context.Context, string, string) (string, error) {
		return "", errors.New("refresh exchange failed")
	})

	server := newAuthRefreshMCPServer(t, func(string) (int, map[string]any, bool) {
		return http.StatusUnauthorized, nil, false
	})
	defer server.Close()

	_, err := newAuthRefreshRuntimeRunner(server).executeInvocation(context.Background(), server.URL, authRefreshTestInvocation())
	if !errors.Is(err, originalCause) {
		t.Fatalf("executeInvocation() error = %v, want original cause", err)
	}
	if _, err := authpkg.LoadTokenData(configDir); err == nil {
		t.Fatal("credential still exists after refresh failure")
	}
}

func TestRuntimeRunnerAuthRefreshFailurePreservesConcurrentlyRotatedCredential(t *testing.T) {
	configDir := setupRuntimeAuthRefreshTest(t, "corp-refresh-race", "old-access")
	originalCause := errors.New("original auth failure")
	edition.Override(&edition.Hooks{OnAuthError: func(_ string, _ error) error {
		return &authretry.AuthRefreshRequired{Cause: originalCause}
	}})
	stubRejectedTokenRefresh(t, func(_ context.Context, gotConfigDir, rejected string) (string, error) {
		if gotConfigDir != configDir || rejected != "old-access" {
			t.Fatalf("refresh args = (%q, %q), want (%q, old-access)", gotConfigDir, rejected, configDir)
		}
		rotateRuntimeTestToken(t, gotConfigDir, "rotated-during-failure")
		return "", errors.New("refresh response was ambiguous after persistence")
	})

	server := newAuthRefreshMCPServer(t, func(string) (int, map[string]any, bool) {
		return http.StatusUnauthorized, nil, false
	})
	defer server.Close()

	_, err := newAuthRefreshRuntimeRunner(server).executeInvocation(context.Background(), server.URL, authRefreshTestInvocation())
	if !errors.Is(err, originalCause) {
		t.Fatalf("executeInvocation() error = %v, want original cause", err)
	}
	stored, err := authpkg.LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() after ambiguous refresh error = %v", err)
	}
	if stored.AccessToken != "rotated-during-failure" {
		t.Fatalf("persisted access token = %q, want rotated-during-failure", stored.AccessToken)
	}
}

func TestRuntimeRunnerAuthRefreshSecondMarkerDoesNotLoopAndPurges(t *testing.T) {
	configDir := setupRuntimeAuthRefreshTest(t, "corp-retry-exhausted", "old-access")
	originalCause := errors.New("token still rejected after refresh")
	var refreshes atomic.Int32
	edition.Override(&edition.Hooks{ClassifyToolResult: func(content map[string]any) error {
		if content["code"] == "USER_TOKEN_ILLEGAL" {
			return &authretry.AuthRefreshRequired{Cause: originalCause}
		}
		return nil
	}})
	stubRejectedTokenRefresh(t, func(_ context.Context, configDir, _ string) (string, error) {
		refreshes.Add(1)
		return rotateRuntimeTestToken(t, configDir, "new-access"), nil
	})

	var calls atomic.Int32
	server := newAuthRefreshMCPServer(t, func(string) (int, map[string]any, bool) {
		calls.Add(1)
		return http.StatusOK, map[string]any{"success": false, "code": "USER_TOKEN_ILLEGAL"}, true
	})
	defer server.Close()

	_, err := newAuthRefreshRuntimeRunner(server).executeInvocation(context.Background(), server.URL, authRefreshTestInvocation())
	if !errors.Is(err, originalCause) {
		t.Fatalf("executeInvocation() error = %v, want original cause", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("MCP calls = %d, want exactly 2", got)
	}
	if got := refreshes.Load(); got != 1 {
		t.Fatalf("refreshes = %d, want exactly 1", got)
	}
	if _, err := authpkg.LoadTokenData(configDir); err == nil {
		t.Fatal("credential still exists after retry exhaustion")
	}
}

func TestRuntimeRunnerAuthRefreshSkipsHTTP403AndOrdinaryBusinessErrors(t *testing.T) {
	t.Run("http 403 marker is rejected by core gate", func(t *testing.T) {
		configDir := setupRuntimeAuthRefreshTest(t, "corp-http-forbidden", "old-access")
		originalCause := errors.New("ordinary permission denied")
		var refreshes atomic.Int32
		edition.Override(&edition.Hooks{OnAuthError: func(_ string, _ error) error {
			return &authretry.AuthRefreshRequired{Cause: originalCause}
		}})
		stubRejectedTokenRefresh(t, func(context.Context, string, string) (string, error) {
			refreshes.Add(1)
			return "", nil
		})
		server := newAuthRefreshMCPServer(t, func(string) (int, map[string]any, bool) {
			return http.StatusForbidden, nil, false
		})
		defer server.Close()

		_, err := newAuthRefreshRuntimeRunner(server).executeInvocation(context.Background(), server.URL, authRefreshTestInvocation())
		if !errors.Is(err, originalCause) {
			t.Fatalf("executeInvocation() error = %v, want permission cause", err)
		}
		if got := refreshes.Load(); got != 0 {
			t.Fatalf("refreshes = %d, want 0", got)
		}
		if _, err := authpkg.LoadTokenData(configDir); err != nil {
			t.Fatalf("credential was purged for HTTP 403: %v", err)
		}
	})

	t.Run("ordinary MCP business error is not a marker", func(t *testing.T) {
		configDir := setupRuntimeAuthRefreshTest(t, "corp-business-error", "old-access")
		var refreshes atomic.Int32
		edition.Override(&edition.Hooks{ClassifyToolResult: func(map[string]any) error {
			return errors.New("ordinary business failure")
		}})
		stubRejectedTokenRefresh(t, func(context.Context, string, string) (string, error) {
			refreshes.Add(1)
			return "", nil
		})
		server := newAuthRefreshMCPServer(t, func(string) (int, map[string]any, bool) {
			return http.StatusOK, map[string]any{"success": false, "code": "PARAM_ERROR"}, true
		})
		defer server.Close()

		if _, err := newAuthRefreshRuntimeRunner(server).executeInvocation(context.Background(), server.URL, authRefreshTestInvocation()); err == nil || err.Error() != "ordinary business failure" {
			t.Fatalf("executeInvocation() error = %v, want ordinary business failure", err)
		}
		if got := refreshes.Load(); got != 0 {
			t.Fatalf("refreshes = %d, want 0", got)
		}
		if _, err := authpkg.LoadTokenData(configDir); err != nil {
			t.Fatalf("credential was purged for business error: %v", err)
		}
	})
}

func TestAuthRefreshHTTP401SourceWithPermissionCauseDoesNotRefresh(t *testing.T) {
	configDir := setupRuntimeAuthRefreshTest(t, "corp-permission-cause", "old-access")
	var refreshes atomic.Int32
	stubRejectedTokenRefresh(t, func(context.Context, string, string) (string, error) {
		refreshes.Add(1)
		return "", errors.New("refresh must not be attempted")
	})
	sourceErr := apperrors.NewAuth("token rejected", apperrors.WithReason("http_401"))
	permissionCause := apperrors.NewAuth("permission denied", apperrors.WithReason("http_403"))

	_, err := (&runtimeRunner{}).maybeAuthRefreshRetry(
		context.Background(), "https://unused.invalid", authRefreshTestInvocation(), "old-access", sourceErr,
		&authretry.AuthRefreshRequired{Cause: permissionCause},
	)
	if !errors.Is(err, permissionCause) {
		t.Fatalf("maybeAuthRefreshRetry() error = %v, want permission cause", err)
	}
	if got := refreshes.Load(); got != 0 {
		t.Fatalf("refreshes = %d, want 0", got)
	}
	if _, err := authpkg.LoadTokenData(configDir); err != nil {
		t.Fatalf("credential was purged for permission cause: %v", err)
	}
}

func TestAuthRefreshRetryExhaustionPreservesDifferentPersistedToken(t *testing.T) {
	configDir := setupRuntimeAuthRefreshTest(t, "corp-retry-token-race", "persisted-newer")
	var refreshes atomic.Int32
	stubRejectedTokenRefresh(t, func(context.Context, string, string) (string, error) {
		refreshes.Add(1)
		return "", errors.New("second refresh must not be attempted")
	})
	originalCause := apperrors.NewAuth("retry token rejected", apperrors.WithReason("http_401"))

	_, err := (&runtimeRunner{}).maybeAuthRefreshRetry(
		withAuthRetrying(context.Background()), "https://unused.invalid", authRefreshTestInvocation(), "retry-token", originalCause,
		&authretry.AuthRefreshRequired{Cause: originalCause},
	)
	if !errors.Is(err, originalCause) {
		t.Fatalf("maybeAuthRefreshRetry() error = %v, want original cause", err)
	}
	if got := refreshes.Load(); got != 0 {
		t.Fatalf("refreshes = %d, want 0", got)
	}
	stored, err := authpkg.LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() after retry exhaustion = %v", err)
	}
	if stored.AccessToken != "persisted-newer" {
		t.Fatalf("persisted access token = %q, want persisted-newer", stored.AccessToken)
	}
}

func TestAuthRefreshPATMarkerDoesNotRefreshOrPurge(t *testing.T) {
	configDir := setupRuntimeAuthRefreshTest(t, "corp-pat-error", "old-access")
	var refreshes atomic.Int32
	stubRejectedTokenRefresh(t, func(context.Context, string, string) (string, error) {
		refreshes.Add(1)
		return "", nil
	})
	patErr := &PatScopeError{ErrorType: "missing_scope", Message: "missing required scope(s): mail:send", MissingScope: "mail:send"}
	runner := &runtimeRunner{}
	_, err := runner.maybeAuthRefreshRetry(
		context.Background(), "https://unused.invalid", authRefreshTestInvocation(), "old-access", patErr,
		&authretry.AuthRefreshRequired{Cause: patErr},
	)
	if err != patErr {
		t.Fatalf("maybeAuthRefreshRetry() error = %v, want original PAT error", err)
	}
	if got := refreshes.Load(); got != 0 {
		t.Fatalf("refreshes = %d, want 0", got)
	}
	if _, err := authpkg.LoadTokenData(configDir); err != nil {
		t.Fatalf("credential was purged for PAT error: %v", err)
	}
}

func TestAuthRefreshFailurePurgesOnlyCurrentRuntimeProfile(t *testing.T) {
	configDir := setupRuntimeAuthRefreshTest(t, "corp-profile-a", "token-a")
	authpkg.SetRuntimeProfile("corp-profile-b")
	if err := authpkg.SaveTokenData(configDir, runtimeAuthRefreshToken("corp-profile-b", "token-b")); err != nil {
		t.Fatalf("SaveTokenData(profile B) error = %v", err)
	}
	authpkg.SetRuntimeProfile("corp-profile-a")
	stubRejectedTokenRefresh(t, func(context.Context, string, string) (string, error) {
		return "", errors.New("refresh failed")
	})
	originalCause := apperrors.NewAuth("profile A rejected", apperrors.WithReason("http_401"))

	_, err := (&runtimeRunner{}).maybeAuthRefreshRetry(
		context.Background(), "https://unused.invalid", authRefreshTestInvocation(), "token-a", originalCause,
		&authretry.AuthRefreshRequired{Cause: originalCause},
	)
	if !errors.Is(err, originalCause) {
		t.Fatalf("maybeAuthRefreshRetry() error = %v, want original cause", err)
	}
	if _, err := authpkg.LoadTokenDataForProfile(configDir, "corp-profile-a"); err == nil {
		t.Fatal("failed profile A credential still exists")
	}
	profileB, err := authpkg.LoadTokenDataForProfile(configDir, "corp-profile-b")
	if err != nil {
		t.Fatalf("profile B credential was removed: %v", err)
	}
	if profileB.AccessToken != "token-b" {
		t.Fatalf("profile B access token = %q, want token-b", profileB.AccessToken)
	}
}

func setupRuntimeAuthRefreshTest(t *testing.T, corpID, accessToken string) string {
	t.Helper()
	previousHooks := edition.Get()
	previousProfile := authpkg.RuntimeProfile()
	originalRefresh := forceRefreshRejectedTokenFunc
	t.Cleanup(func() {
		forceRefreshRejectedTokenFunc = originalRefresh
		edition.Override(previousHooks)
		authpkg.SetRuntimeProfile(previousProfile)
		ResetRuntimeTokenCache()
	})
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	authpkg.SetRuntimeProfile("")
	edition.Override(&edition.Hooks{})
	ResetRuntimeTokenCache()
	if err := authpkg.SaveTokenData(configDir, runtimeAuthRefreshToken(corpID, accessToken)); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	return configDir
}

func runtimeAuthRefreshToken(corpID, accessToken string) *authpkg.TokenData {
	return &authpkg.TokenData{
		AccessToken:  accessToken,
		RefreshToken: "refresh-" + accessToken,
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       corpID,
		CorpName:     corpID + " org",
		ClientID:     "client-" + corpID,
	}
}

func rotateRuntimeTestToken(t *testing.T, configDir, accessToken string) string {
	t.Helper()
	data, err := authpkg.LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() before rotation error = %v", err)
	}
	data.AccessToken = accessToken
	data.RefreshToken = "refresh-" + accessToken
	data.ExpiresAt = time.Now().Add(2 * time.Hour)
	data.RefreshExpAt = time.Now().Add(24 * time.Hour)
	if err := authpkg.SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() during rotation error = %v", err)
	}
	return accessToken
}

func stubRejectedTokenRefresh(t *testing.T, fn func(context.Context, string, string) (string, error)) {
	t.Helper()
	forceRefreshRejectedTokenFunc = fn
}

func authRefreshTestInvocation() executor.Invocation {
	return executor.Invocation{
		Kind:             "dynamic_tool",
		CanonicalProduct: "auth-refresh-test",
		Tool:             "auth.refresh_test",
		Params:           map[string]any{"value": "same-invocation"},
	}
}

func newAuthRefreshRuntimeRunner(server *httptest.Server) *runtimeRunner {
	client := transport.NewClient(server.Client())
	client.MaxRetries = 0
	return &runtimeRunner{
		transport:   client,
		globalFlags: &GlobalFlags{Timeout: 5},
		fallback:    executor.EchoRunner{},
	}
}

func newAuthRefreshMCPServer(t *testing.T, response func(token string) (int, map[string]any, bool)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "))
		status, content, isError := response(token)
		if status != http.StatusOK {
			http.Error(w, http.StatusText(status), status)
			return
		}
		var request struct {
			ID any `json:"id"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode MCP request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"content": content,
				"isError": isError,
			},
		}); err != nil {
			t.Errorf("encode MCP response: %v", err)
		}
	}))
}
