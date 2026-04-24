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
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
)

// oauthHTTPClient is a dedicated HTTP client for OAuth operations with
// explicit timeout and TLS configuration, replacing http.DefaultClient.
var oauthHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}

// OAuthProvider handles the DingTalk OAuth 2.0 authorization code flow.
type OAuthProvider struct {
	configDir  string
	clientID   string
	logger     *slog.Logger
	Output     io.Writer
	httpClient *http.Client
}

// NewOAuthProvider creates a new OAuth provider.
func NewOAuthProvider(configDir string, logger *slog.Logger) *OAuthProvider {
	return &OAuthProvider{
		configDir:  configDir,
		clientID:   ClientID(),
		logger:     logger,
		Output:     os.Stderr,
		httpClient: oauthHTTPClient,
	}
}

// resetCredentialState clears any stale credential state inherited from
// previous login methods so that OAuth flow always starts fresh by
// fetching clientID from MCP.
func (p *OAuthProvider) resetCredentialState() {
	p.clientID = ""
	clientMu.Lock()
	clientIDFromMCP = false
	clientMu.Unlock()
}

func (p *OAuthProvider) output() io.Writer {
	if p != nil && p.Output != nil {
		return p.Output
	}
	return io.Discard
}

// Login performs authentication with smart degradation:
// 1. If force=false, try silent token refresh first (refresh_token)
// 2. If all silent methods fail (or force=true), fall back to browser OAuth flow
func (p *OAuthProvider) Login(ctx context.Context, force bool) (*TokenData, error) {
	// Smart degradation: try silent refresh before opening browser.
	if !force {
		data, err := LoadTokenData(p.configDir)
		if err == nil {
			// Case 1: access_token still valid — no action needed.
			if data.IsAccessTokenValid() {
				if p.logger != nil {
					p.logger.Debug("access_token still valid, skipping login")
				}
				// Even on early return, persist custom app credentials if provided
				// via --client-id/--client-secret flags. Without this, the flags
				// are only in runtime globals and lost when the process exits.
				p.persistAppConfigIfNeeded()
				return data, nil
			}
			// Case 2: refresh using refresh_token (with lock to prevent concurrent refresh).
			if data.IsRefreshTokenValid() {
				if p.logger != nil {
					p.logger.Debug("access_token expired, trying refresh_token")
				}
				refreshed, rErr := p.lockedRefresh(ctx)
				if rErr == nil {
					p.persistAppConfigIfNeeded()
					return refreshed, nil
				}
				if p.logger != nil {
					p.logger.Warn(i18n.T("refresh_token 刷新失败，将尝试扫码登录"), "error", rErr)
				}
			}
		}
	}

	// Fall through: full browser OAuth flow.
	// Defensive reset: clear stale credential state from previous login methods,
	// but preserve user-provided --client-id if present.
	userClientID := p.clientID
	p.resetCredentialState()

	if userClientID != "" && userClientID != DefaultClientID {
		// User provided --client-id flag: use it directly, skip MCP fetch.
		p.clientID = userClientID
		if p.logger != nil {
			p.logger.Debug("using user-provided client ID, skipping MCP fetch", "clientID", userClientID)
		}
	} else {
		// No user-provided client ID: fetch from MCP server.
		if p.logger != nil {
			p.logger.Debug("fetching client ID from MCP server")
		}
		mcpClientID, mcpErr := FetchClientIDFromMCP(ctx)
		if mcpErr != nil {
			return nil, fmt.Errorf("%s: %w", i18n.T("获取 Client ID 失败"), mcpErr)
		}
		p.clientID = mcpClientID
		SetClientIDFromMCP(mcpClientID)
		if p.logger != nil {
			p.logger.Debug("fetched client ID from MCP server", "clientID", mcpClientID)
		}
	}

	// Find a free port for the callback server.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting callback listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, CallbackPath)

	// Channel to pass callback result (token data or error with CLI auth status)
	type callbackResult struct {
		token           *TokenData
		err             error
		cliAuthDisabled bool
		denialReason    string
	}
	resultCh := make(chan callbackResult, 1)
	errCh := make(chan error, 1)

	// Shared state for API handlers (protected by mutex)
	var (
		callbackToken           *TokenData
		callbackProcessedCode   string // The auth code that has been successfully processed
		callbackAuthDisabled    bool
		callbackApplySent       bool   // Whether apply request was sent
		callbackSelectedAdminId string // Selected admin ID for apply
		callbackCodeInProgress  string // Code currently being processed (to prevent concurrent exchange)
		callbackTokenMu         sync.Mutex
	)

	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		// Get code first to check if this is a new authorization or page refresh
		code := r.URL.Query().Get("authCode")
		if code == "" {
			code = r.URL.Query().Get("code")
		}

		// Check state and handle page refresh or concurrent requests
		callbackTokenMu.Lock()
		processedCode := callbackProcessedCode
		processedAuthDisabled := callbackAuthDisabled
		codeInProgress := callbackCodeInProgress
		hasToken := callbackToken != nil

		// Case 1: This code was already successfully processed - show cached page
		if code != "" && code == processedCode {
			callbackTokenMu.Unlock()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if processedAuthDisabled {
				_, _ = fmt.Fprint(w, notEnabledHTML)
			} else {
				_, _ = fmt.Fprint(w, successHTML)
			}
			return
		}

		// Case 2: This code is being processed by another request - show wait page
		if code != "" && code == codeInProgress {
			callbackTokenMu.Unlock()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<html><head><meta http-equiv="refresh" content="1"></head><body><p>正在处理授权，请稍候...</p></body></html>`)
			return
		}

		// Case 3: No code but we have a processed token - show cached page
		if code == "" && hasToken {
			callbackTokenMu.Unlock()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if processedAuthDisabled {
				_, _ = fmt.Fprint(w, notEnabledHTML)
			} else {
				_, _ = fmt.Fprint(w, successHTML)
			}
			return
		}

		// Case 4: New code - mark as in-progress and process
		if code != "" {
			callbackCodeInProgress = code
		}
		callbackTokenMu.Unlock()

		if code == "" {
			select {
			case errCh <- errors.New(i18n.T("回调中未收到授权码")):
			default:
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, i18n.T("授权失败：未收到授权码"))
			return
		}

		// Exchange code for token
		tokenData, exchangeErr := p.exchangeCode(ctx, code)
		if exchangeErr != nil {
			// Clear in-progress state on error
			callbackTokenMu.Lock()
			if callbackCodeInProgress == code {
				callbackCodeInProgress = ""
			}
			callbackTokenMu.Unlock()

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintf(w, "<html><body><h1>授权失败</h1><p>%s</p></body></html>", exchangeErr.Error())
			select {
			case resultCh <- callbackResult{err: exchangeErr}:
			default:
			}
			return
		}

		// Mark as processed immediately after successful exchange
		callbackTokenMu.Lock()
		previouslyProcessed := callbackProcessedCode != ""
		callbackToken = tokenData
		callbackProcessedCode = code // Remember this code was successfully processed
		callbackCodeInProgress = ""  // Clear in-progress state
		// Reset apply state for new authorization (user switched org)
		if previouslyProcessed {
			callbackApplySent = false
			callbackSelectedAdminId = ""
		}
		callbackTokenMu.Unlock()

		// Check CLI auth enabled status (fail-closed: treat errors as disabled)
		authStatus, statusErr := p.CheckCLIAuthEnabled(ctx, tokenData.AccessToken)
		var denialReason string
		if statusErr != nil {
			denialReason = "unknown"
		} else {
			denialReason = classifyDenialReason(authStatus, os.Getenv("DWS_CHANNEL"))
		}
		cliAuthEnabled := denialReason == ""

		// Update CLI auth disabled state
		callbackTokenMu.Lock()
		callbackAuthDisabled = !cliAuthEnabled
		callbackTokenMu.Unlock()

		// Display appropriate HTML based on auth status and denial reason
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch {
		case cliAuthEnabled:
			_, _ = fmt.Fprint(w, successHTML)
		case denialReason == "user_forbidden" || denialReason == "user_not_allowed":
			_, _ = fmt.Fprint(w, accessDeniedHTML)
		case denialReason == "channel_not_allowed" || denialReason == "channel_required":
			_, _ = fmt.Fprint(w, channelDeniedHTML)
		default:
			_, _ = fmt.Fprint(w, notEnabledHTML)
		}
		// Ensure response is flushed to client
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Notify main goroutine with full result
		select {
		case resultCh <- callbackResult{token: tokenData, cliAuthDisabled: !cliAuthEnabled, denialReason: denialReason}:
		default:
		}
	})

	// API endpoint: get super admins
	mux.HandleFunc("/api/superAdmin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callbackTokenMu.Lock()
		token := callbackToken
		callbackTokenMu.Unlock()
		if token == nil {
			_, _ = w.Write([]byte(`{"success":false,"errorMsg":"授权尚未完成"}`))
			return
		}
		result, err := GetSuperAdmins(ctx, token.AccessToken)
		if err != nil {
			_, _ = fmt.Fprintf(w, `{"success":false,"errorMsg":"%s"}`, err.Error())
			return
		}
		data, _ := json.Marshal(result)
		_, _ = w.Write(data)
	})

	// API endpoint: send CLI auth apply
	mux.HandleFunc("/api/sendApply", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		adminStaffID := r.URL.Query().Get("adminStaffId")
		if adminStaffID == "" {
			_, _ = w.Write([]byte(`{"success":false,"errorMsg":"缺少 adminStaffId 参数"}`))
			return
		}
		callbackTokenMu.Lock()
		token := callbackToken
		callbackTokenMu.Unlock()
		if token == nil {
			_, _ = w.Write([]byte(`{"success":false,"errorMsg":"授权尚未完成"}`))
			return
		}
		result, err := SendCliAuthApply(ctx, token.AccessToken, adminStaffID)
		if err != nil {
			_, _ = fmt.Fprintf(w, `{"success":false,"errorMsg":"%s"}`, err.Error())
			return
		}
		// Mark apply as sent and save selected admin on success
		if result.Success && result.Result {
			callbackTokenMu.Lock()
			callbackApplySent = true
			callbackSelectedAdminId = adminStaffID
			callbackTokenMu.Unlock()
		}
		data, _ := json.Marshal(result)
		_, _ = w.Write(data)
	})

	// API endpoint: get current status (clientId, applySent, selectedAdminId)
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callbackTokenMu.Lock()
		applySent := callbackApplySent
		selectedAdminId := callbackSelectedAdminId
		callbackTokenMu.Unlock()
		_, _ = fmt.Fprintf(w, `{"clientId":"%s","applySent":%t,"selectedAdminId":"%s"}`, p.clientID, applySent, selectedAdminId)
	})

	// API endpoint: check CLI auth enabled status
	mux.HandleFunc("/api/cliAuthEnabled", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callbackTokenMu.Lock()
		token := callbackToken
		callbackTokenMu.Unlock()
		if token == nil {
			_, _ = w.Write([]byte(`{"success":false,"errorMsg":"授权尚未完成"}`))
			return
		}
		result, err := p.CheckCLIAuthEnabled(ctx, token.AccessToken)
		if err != nil {
			_, _ = fmt.Fprintf(w, `{"success":false,"errorMsg":"%s"}`, err.Error())
			return
		}
		data, _ := json.Marshal(result)
		_, _ = w.Write(data)
	})

	// Success page endpoint
	mux.HandleFunc("/success", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, successHTML)
	})

	server := &http.Server{Handler: mux}
	go func() {
		if serveErr := server.Serve(listener); !errors.Is(serveErr, http.ErrServerClosed) {
			select {
			case errCh <- fmt.Errorf("callback server error: %w", serveErr):
			default:
			}
		}
	}()
	defer func() {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		_ = server.Shutdown(shutCtx)
	}()

	authURL := buildAuthURL(p.clientID, redirectURI)
	if p.logger != nil {
		p.logger.Debug("authorization URL", "url", authURL)
	}
	if err := openBrowser(authURL); err != nil && p.logger != nil {
		p.logger.Warn(i18n.T("无法自动打开浏览器"), "error", err)
	}

	_, _ = fmt.Fprintln(p.output(), "")
	_, _ = fmt.Fprintln(p.output(), i18n.T("🔐 登录钉钉"))
	_, _ = fmt.Fprintln(p.output(), "")
	_, _ = fmt.Fprintln(p.output(), i18n.T("请在浏览器中完成扫码授权。"))
	_, _ = fmt.Fprintf(p.output(), i18n.T("如果浏览器未自动打开，请手动访问:\n  %s\n\n"), authURL)
	_, _ = fmt.Fprintln(p.output(), i18n.T("⏳ 等待授权中..."))

	timeout := time.NewTimer(5 * time.Minute)
	defer timeout.Stop()

	var result callbackResult
	select {
	case result = <-resultCh:
	case err := <-errCh:
		return nil, err
	case <-timeout.C:
		return nil, errors.New(i18n.T("授权超时（5分钟），请重试"))
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Handle callback errors
	if result.err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("换取 token 失败"), result.err)
	}

	// Handle CLI auth disabled - for terminal denial reasons, exit immediately
	// (page shows accessDeniedHTML/channelDeniedHTML with no apply button,
	// so polling for apply submission would hang forever).
	// Error messages are kept consistent with the text shown on the HTML pages.
	if result.cliAuthDisabled {
		switch result.denialReason {
		case "user_forbidden", "user_not_allowed":
			return nil, errors.New(i18n.T("您不在该组织的 CLI 授权人员范围内，请联系组织管理员将您加入授权名单"))
		case "channel_not_allowed", "channel_required":
			return nil, errors.New(i18n.T("当前渠道未获得该组织授权，或组织已开启渠道管控，请联系组织管理员开通渠道访问权限，或升级到最新版本的 CLI"))
		}

		_, _ = fmt.Fprintln(p.output(), "")
		_, _ = fmt.Fprintln(p.output(), i18n.T("⏳ 该组织尚未开启 CLI 数据访问权限，请在浏览器中提交授权申请..."))

		// Poll for CLI auth status while waiting
		applyTimeout := time.NewTimer(10 * time.Minute)
		defer applyTimeout.Stop()
		pollTicker := time.NewTicker(5 * time.Second)
		defer pollTicker.Stop()

		elapsedSeconds := 0
		for {
			select {
			case <-applyTimeout.C:
				return nil, errors.New(i18n.T("操作超时，请重新登录"))
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-pollTicker.C:
				elapsedSeconds += 5

				// Get latest token and state (user may have switched org)
				callbackTokenMu.Lock()
				currentToken := callbackToken
				currentAuthDisabled := callbackAuthDisabled
				applySent := callbackApplySent
				callbackTokenMu.Unlock()

				// Check if user switched to an org with CLI auth enabled
				if currentToken != nil && !currentAuthDisabled {
					_, _ = fmt.Fprintf(p.output(), "\r%s\n", i18n.T("✅ 权限已开启，继续登录..."))
					time.Sleep(2 * time.Second)
					result.token = currentToken
					result.cliAuthDisabled = false
					goto continueLogin
				}

				// Check if CLI auth is now enabled (admin approved)
				if currentToken != nil {
					authStatus, err := p.CheckCLIAuthEnabled(ctx, currentToken.AccessToken)
					if err == nil && classifyDenialReason(authStatus, os.Getenv("DWS_CHANNEL")) == "" {
						_, _ = fmt.Fprintf(p.output(), "\r%s\n", i18n.T("✅ 权限已开启，继续登录..."))
						time.Sleep(2 * time.Second)
						result.token = currentToken
						result.cliAuthDisabled = false
						goto continueLogin
					}
				}

				// Show polling status based on apply state
				if applySent {
					_, _ = fmt.Fprintf(p.output(), "\r⏳ %s (%ds/600s)   ", i18n.T("等待管理员审批中"), elapsedSeconds)
				} else {
					_, _ = fmt.Fprintf(p.output(), "\r⏳ %s (%ds/600s)   ", i18n.T("等待提交申请中"), elapsedSeconds)
				}
			}
		}
	}

continueLogin:
	tokenData := result.token

	// Save token data with associated client ID for refresh
	tokenData.ClientID = p.clientID
	if err := SaveTokenData(p.configDir, tokenData); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("保存 token 失败"), err)
	}

	// Persist app credentials (with secret) if using custom client credentials.
	// MUST run BEFORE os.Setenv below to avoid env-matching short circuit.
	p.persistAppConfigIfNeeded()

	// Always persist clientId to app.json so future process startups
	// can load it via ResolveAppCredentials and populate DWS_CLIENT_ID env.
	if p.clientID != "" {
		_ = os.Setenv("DWS_CLIENT_ID", p.clientID)
		if !HasAppConfig(p.configDir) {
			_ = SaveAppConfig(p.configDir, &AppConfig{ClientID: p.clientID})
		}
	}

	return tokenData, nil
}

// GetAccessToken returns a valid access token, auto-refreshing if needed.
// Uses a file lock with double-check pattern to prevent concurrent refresh
// from multiple CLI processes.
func (p *OAuthProvider) GetAccessToken(ctx context.Context) (string, error) {
	data, err := LoadTokenData(p.configDir)
	if err != nil {
		return "", errors.New(i18n.T("未登录，请运行 dws auth login"))
	}

	// Fast path: access_token still valid — no lock needed.
	if data.IsAccessTokenValid() {
		return data.AccessToken, nil
	}

	// Slow path: token expired — try locked refresh.
	if data.IsRefreshTokenValid() {
		refreshed, rErr := p.lockedRefresh(ctx)
		if rErr == nil {
			return refreshed.AccessToken, nil
		}
		if p.logger != nil {
			p.logger.Warn(i18n.T("refresh_token 刷新失败"), "error", rErr)
		}
	}

	return "", errors.New(i18n.T("所有凭证已失效，请运行 dws auth login 重新登录"))
}

// lockedRefresh attempts to refresh the token while holding dual-layer locks.
// It uses a double-check pattern with both process-level and file-level locking:
//
// Layer 1 (Process Lock - sync.Map):
//
//	Prevents multiple goroutines within the same process from refreshing simultaneously.
//	If another goroutine is already refreshing, we wait for it and then re-check.
//
// Layer 2 (File Lock - flock/LockFileEx):
//
//	Prevents multiple CLI processes from refreshing simultaneously.
//	If another process is refreshing, we wait for the file lock and then re-check.
//
// Double-Check Pattern:
//
//	After acquiring the lock, we re-load from disk because another goroutine/process
//	may have already completed the refresh while we were waiting. This prevents the
//	classic race where two callers both see an expired token and both call the
//	refresh API, invalidating each other's refresh_token.
func (p *OAuthProvider) lockedRefresh(ctx context.Context) (*TokenData, error) {
	// Acquire dual-layer lock (process-level + file-level)
	lock, err := AcquireDualLock(ctx, p.configDir)
	if err != nil {
		return nil, fmt.Errorf("acquiring dual lock: %w", err)
	}
	defer lock.Release()

	// Double-check: re-load from disk — another goroutine/process may have refreshed
	// while we were waiting for the lock.
	data, err := LoadTokenData(p.configDir)
	if err != nil {
		return nil, err
	}
	if data.IsAccessTokenValid() {
		if p.logger != nil {
			if lock.Waited {
				p.logger.Debug("token already refreshed by another goroutine/process")
			} else {
				p.logger.Debug("token still valid after acquiring lock")
			}
		}
		return data, nil
	}

	// Still expired — we need to actually refresh.
	if !data.IsRefreshTokenValid() {
		return nil, fmt.Errorf("refresh_token 已过期")
	}

	if p.logger != nil {
		p.logger.Debug("refreshing token (dual-locked)")
	}
	return p.refreshWithRefreshToken(ctx, data)
}

// ExchangeAuthCode takes an AuthCode and an optional UserID provided by an
// external host, exchanges it for tokens, and persists them.
func (p *OAuthProvider) ExchangeAuthCode(ctx context.Context, authCode, uid string) (*TokenData, error) {
	tokenData, err := p.exchangeCode(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("换取 token 失败"), err)
	}
	if uid != "" {
		tokenData.UserID = uid
	}
	if err := SaveTokenData(p.configDir, tokenData); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("保存 token 失败"), err)
	}
	return tokenData, nil
}

// Logout clears all stored credentials.
func (p *OAuthProvider) Logout() error {
	return DeleteTokenData(p.configDir)
}

// Status returns the current auth status.
func (p *OAuthProvider) Status() (*TokenData, error) {
	return LoadTokenData(p.configDir)
}

// persistAppConfigIfNeeded saves app credentials if custom ones were used.
// This ensures the client secret is available for future token refreshes.
func (p *OAuthProvider) persistAppConfigIfNeeded() {
	// Check if custom credentials were provided via runtime flags
	clientID, clientSecret := getRuntimeCredentials()
	if clientID == "" || clientSecret == "" {
		return
	}

	// Skip if using default placeholder credentials
	if clientID == DefaultClientID {
		return
	}

	// Save app config with secret stored in keychain
	config := &AppConfig{
		ClientID:     clientID,
		ClientSecret: PlainSecret(clientSecret),
	}
	if err := SaveAppConfig(p.configDir, config); err != nil {
		if p.logger != nil {
			p.logger.Warn("failed to persist app credentials", "error", err)
		}
	}
}
