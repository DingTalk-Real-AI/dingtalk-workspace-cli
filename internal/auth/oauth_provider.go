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
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
)

// oauthHTTPClient is a dedicated HTTP client for OAuth operations with
// explicit timeout and TLS configuration, replacing http.DefaultClient.
var oauthHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}

var (
	oauthListen          = net.Listen
	oauthOpenBrowser     = openBrowser
	oauthLoginTimeout    = 5 * time.Minute
	oauthApprovalTimeout = 10 * time.Minute
	oauthPollInterval    = 5 * time.Second
	oauthSuccessPause    = 2 * time.Second
	oauthLoadToken       = LoadTokenData
	oauthLoadTokenLocked = loadTokenDataForProfileLocked
	oauthAcquireLock     = AcquireDualLock
	oauthMarkProfile     = MarkProfileStatus
	oauthFetchClientID   = FetchClientIDFromMCP
	oauthExchange        = func(p *OAuthProvider, ctx context.Context, code string) (*TokenData, error) {
		return p.exchangeCode(ctx, code)
	}
	oauthCheckStatus = func(p *OAuthProvider, ctx context.Context, token string) (*CLIAuthStatus, error) {
		return p.CheckCLIAuthEnabled(ctx, token)
	}
	oauthGetAdmins    = GetSuperAdmins
	oauthSendApply    = SendCliAuthApply
	oauthSaveToken    = SaveTokenData
	oauthRefreshToken = func(p *OAuthProvider, ctx context.Context, data *TokenData) (*TokenData, error) {
		return p.refreshWithRefreshToken(ctx, data)
	}
	oauthSleep = time.Sleep
)

func oauthFetchClientIDForLoginRegion(ctx context.Context, region LoginRegion) (string, error) {
	if region.IsInternational() {
		return FetchClientIDFromMCPForLoginRegion(ctx, region)
	}
	return oauthFetchClientID(ctx)
}

func oauthGetAdminsForLoginRegion(ctx context.Context, accessToken string, region LoginRegion) (*SuperAdminResponse, error) {
	if region.IsInternational() {
		return GetSuperAdminsForLoginRegion(ctx, accessToken, region)
	}
	return oauthGetAdmins(ctx, accessToken)
}

func oauthSendApplyForLoginRegion(ctx context.Context, accessToken, adminStaffID string, region LoginRegion) (*SendApplyResponse, error) {
	if region.IsInternational() {
		return SendCliAuthApplyForLoginRegion(ctx, accessToken, adminStaffID, region)
	}
	return oauthSendApply(ctx, accessToken, adminStaffID)
}

// OAuthProvider handles the DingTalk OAuth 2.0 authorization code flow.
type OAuthProvider struct {
	configDir     string
	clientID      string
	credentials   *AppCredentialPair
	credentialErr error
	logger        *slog.Logger
	Output        io.Writer
	httpClient    *http.Client
	NoBrowser     bool
	TargetCorpID  string
	// IdentityEnricher resolves userId/userName/corpName while the freshly
	// exchanged access token is still only in memory.
	IdentityEnricher func(context.Context, *TokenData) error
	LoginRegion      LoginRegion
}

// NewOAuthProvider creates a new OAuth provider.
func NewOAuthProvider(configDir string, logger *slog.Logger) *OAuthProvider {
	pair, err := resolveOAuthCredentialPair(configDir)
	p := &OAuthProvider{
		configDir:     configDir,
		credentialErr: err,
		logger:        logger,
		Output:        os.Stderr,
		httpClient:    oauthHTTPClient,
	}
	if pair != nil {
		copy := *pair
		p.credentials = &copy
		p.clientID = copy.ClientID
	} else {
		// Keep the legacy observable constructor value for callers that only
		// inspect the provider. Login always resets this before managed MCP use.
		p.clientID = ClientID()
	}
	return p
}

func resolveOAuthCredentialPair(configDir string) (*AppCredentialPair, error) {
	if pair, selected, err := credentialPairFromValues(runtimeCredentialValues()); selected || err != nil {
		if err != nil {
			return nil, ErrFlagCredentialPairIncomplete
		}
		return &pair, nil
	}
	if pair, selected, err := credentialPairFromValues(os.Getenv(EnvClientID), os.Getenv(EnvClientSecret), string(CredentialSourceEnv), ErrEnvCredentialPairIncomplete); selected || err != nil {
		if err != nil {
			return nil, err
		}
		clearMCPRuntimeCredentials()
		return &pair, nil
	}
	pair, err := ResolveAppConfigCredentialPair(configDir)
	if err == nil {
		clearMCPRuntimeCredentials()
		return &pair, nil
	}
	if errors.Is(err, ErrAppConfigMissing) || errors.Is(err, ErrClientIDEmpty) || errors.Is(err, ErrClientSecretEmpty) {
		return nil, nil
	}
	return nil, err
}

func runtimeCredentialValues() (string, string, string, error) {
	clientMu.RLock()
	id, secret, fromMCP := runtimeClientID, runtimeClientSecret, clientIDFromMCP
	clientMu.RUnlock()
	if fromMCP {
		return "", "", string(CredentialSourceFlag), ErrFlagCredentialPairIncomplete
	}
	return id, secret, string(CredentialSourceFlag), ErrFlagCredentialPairIncomplete
}

func (p *OAuthProvider) snapshotCredentialPair() {
	pair, err := resolveOAuthCredentialPair(p.configDir)
	p.credentials = nil
	p.credentialErr = err
	if pair != nil {
		copy := *pair
		p.credentials = &copy
		p.clientID = copy.ClientID
	}
}

// resetCredentialState clears any stale credential state inherited from
// previous login methods before the OAuth flow falls back to MCP-managed
// credentials. Complete runtime AppKey/AppSecret overrides skip this reset.
func (p *OAuthProvider) resetCredentialState() {
	p.clientID = ""
	p.credentials = nil
	clearRuntimeCredentials()
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
	p.snapshotCredentialPair()
	if p.credentialErr != nil {
		return nil, fmt.Errorf("应用凭证配置无效: %w", p.credentialErr)
	}
	// Smart degradation: try silent refresh before opening browser.
	if !force {
		data, err := oauthLoadToken(p.configDir)
		if err != nil && !errors.Is(err, ErrTokenDataNotFound) && !os.IsNotExist(err) {
			// A damaged selected slot must not make browser reauthorization
			// impossible. The target identity is unknown until token exchange, so
			// continue into the full flow and let the target-only preflight reject
			// an unsafe overwrite after identity enrichment.
			if p.logger != nil {
				p.logger.Warn(i18n.T("读取现有登录态失败，将尝试扫码登录"), "error", err)
			}
		}
		if err == nil {
			// Case 1: access_token still valid — no action needed.
			if data.IsAccessTokenValid() {
				if p.logger != nil {
					p.logger.Debug("access_token still valid, skipping login")
				}
				return data, nil
			}
			// Case 2: refresh using refresh_token (with lock to prevent concurrent refresh).
			if data.IsRefreshTokenValid() {
				if p.logger != nil {
					p.logger.Debug("access_token expired, trying refresh_token")
				}
				refreshed, rErr := p.lockedRefresh(ctx)
				if rErr == nil {
					return refreshed, nil
				}
				if p.logger != nil {
					p.logger.Warn(i18n.T("refresh_token 刷新失败，将尝试扫码登录"), "error", rErr)
				}
			}
		}
	}
	if err := prepareLoginPersistence(p.configDir); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("本地登录态无法安全更新"), err)
	}

	// Fall through: full browser OAuth flow.
	if p.credentials != nil {
		p.clientID = p.credentials.ClientID
		clearMCPRuntimeCredentials()
	} else {
		// Defensive reset: clear any stale credential state from previous login
		// methods so we can re-fetch clientID from MCP. This ensures --force
		// login works regardless of what app.json contains.
		p.resetCredentialState()

		if p.logger != nil {
			p.logger.Debug("fetching client ID from MCP server (OAuth flow always re-fetches)")
		}
		mcpClientID, mcpErr := oauthFetchClientIDForLoginRegion(ctx, p.LoginRegion)
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
	listener, err := oauthListen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting callback listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, CallbackPath)
	logging.AuthDebug(
		"auth.login.oauth.flow.start",
		"client_id", strings.TrimSpace(p.clientID),
		"target_corp_id", strings.TrimSpace(p.TargetCorpID),
		"callback_port", port,
		"force", force,
		"no_browser", p.NoBrowser,
	)

	// Channel to pass callback result (token data or error with CLI auth status)
	type callbackResult struct {
		token           *TokenData
		err             error
		cliAuthDisabled bool
		denialReason    string
		errorMsg        string // server-provided errorMsg from /cli/cliAuthEnabled
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
		logging.AuthDebug(
			"auth.login.oauth.callback.received",
			"callback_port", port,
			"has_authorization_code", code != "",
		)

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
		tokenData, exchangeErr := oauthExchange(p, ctx, code)
		if exchangeErr != nil {
			logging.AuthDebug(
				"auth.login.oauth.token_exchange.error",
				"callback_port", port,
				"error", exchangeErr,
			)
			// Clear in-progress state on error
			callbackTokenMu.Lock()
			if callbackCodeInProgress == code {
				callbackCodeInProgress = ""
			}
			callbackTokenMu.Unlock()

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintf(w, "<html><body><h1>授权失败</h1><p>%s</p></body></html>", html.EscapeString(oauthExchangeDisplayError(exchangeErr)))
			select {
			case resultCh <- callbackResult{err: exchangeErr}:
			default:
			}
			return
		}
		logging.AuthDebug(
			"auth.login.oauth.token_exchange.done",
			"callback_port", port,
			"corp_id", strings.TrimSpace(tokenData.CorpID),
			"user_id", strings.TrimSpace(tokenData.UserID),
			"user_name", strings.TrimSpace(tokenData.UserName),
			"source", strings.TrimSpace(tokenData.Source),
			"access_expires_at", tokenData.ExpiresAt,
			"refresh_expires_at", tokenData.RefreshExpAt,
		)

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
		authStatus, statusErr := oauthCheckStatus(p, ctx, tokenData.AccessToken)
		var denialReason string
		if statusErr != nil {
			denialReason = "unknown"
		} else {
			denialReason = classifyDenialReason(authStatus, os.Getenv("DWS_CHANNEL"))
		}
		cliAuthEnabled := denialReason == ""
		logging.AuthDebug(
			"auth.login.oauth.organization_access.checked",
			"callback_port", port,
			"corp_id", strings.TrimSpace(tokenData.CorpID),
			"enabled", cliAuthEnabled,
			"denial_reason", denialReason,
		)

		// Server-provided errorMsg (nil-safe), surfaced both on the page and to
		// the terminal so portal can update copy without releasing the CLI.
		serverMsg := ""
		if authStatus != nil {
			serverMsg = authStatus.ErrorMsg
		}

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
		case denialReason == "enterprise_not_authorized":
			_, _ = fmt.Fprint(w, renderEnterpriseDeniedHTML(serverMsg))
		default:
			_, _ = fmt.Fprint(w, notEnabledHTML)
		}
		// Ensure response is flushed to client
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Notify main goroutine with full result
		select {
		case resultCh <- callbackResult{token: tokenData, cliAuthDisabled: !cliAuthEnabled, denialReason: denialReason, errorMsg: serverMsg}:
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
		result, err := oauthGetAdminsForLoginRegion(ctx, token.AccessToken, p.LoginRegion)
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
		result, err := oauthSendApplyForLoginRegion(ctx, token.AccessToken, adminStaffID, p.LoginRegion)
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
		data, _ := json.Marshal(map[string]any{
			"clientId":        p.clientID,
			"authorizeUrl":    AuthorizeURLForLoginRegion(p.LoginRegion),
			"applySent":       applySent,
			"selectedAdminId": selectedAdminId,
		})
		_, _ = w.Write(data)
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
		result, err := oauthCheckStatus(p, ctx, token.AccessToken)
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

	authURL := buildAuthURLForRegion(p.clientID, redirectURI, p.TargetCorpID, p.LoginRegion)
	if p.logger != nil {
		p.logger.Debug("authorization URL", "url", authURL)
	}
	if !p.NoBrowser {
		if err := oauthOpenBrowser(authURL); err != nil && p.logger != nil {
			p.logger.Warn(i18n.T("无法自动打开浏览器"), "error", err)
		}
	}

	_, _ = fmt.Fprintln(p.output(), "")
	_, _ = fmt.Fprintln(p.output(), i18n.T("🔐 登录钉钉"))
	_, _ = fmt.Fprintln(p.output(), "")
	_, _ = fmt.Fprintln(p.output(), i18n.T("请在浏览器中完成扫码授权。"))
	_, _ = fmt.Fprintf(p.output(), i18n.T("如果浏览器未自动打开，请手动访问:\n  %s\n\n"), authURL)
	_, _ = fmt.Fprintln(p.output(), i18n.T("⏳ 等待授权中..."))

	timeout := time.NewTimer(oauthLoginTimeout)
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
		case "enterprise_not_authorized":
			if msg := strings.TrimSpace(result.errorMsg); msg != "" {
				return nil, errors.New(msg)
			}
			return nil, errors.New(i18n.T("本次请求未通过企业安全认证"))
		}

		_, _ = fmt.Fprintln(p.output(), "")
		_, _ = fmt.Fprintln(p.output(), i18n.T("⏳ 该组织尚未开启 CLI 数据访问权限，请在浏览器中提交授权申请..."))

		// Poll for CLI auth status while waiting
		applyTimeout := time.NewTimer(oauthApprovalTimeout)
		defer applyTimeout.Stop()
		pollTicker := time.NewTicker(oauthPollInterval)
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
					oauthSleep(oauthSuccessPause)
					result.token = currentToken
					result.cliAuthDisabled = false
					goto continueLogin
				}

				// Check if CLI auth is now enabled (admin approved)
				if currentToken != nil {
					authStatus, err := oauthCheckStatus(p, ctx, currentToken.AccessToken)
					if err == nil && classifyDenialReason(authStatus, os.Getenv("DWS_CHANNEL")) == "" {
						_, _ = fmt.Fprintf(p.output(), "\r%s\n", i18n.T("✅ 权限已开启，继续登录..."))
						oauthSleep(oauthSuccessPause)
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
	logging.AuthDebug(
		"auth.login.oauth.persistence.start",
		"callback_port", port,
		"corp_id", strings.TrimSpace(tokenData.CorpID),
		"user_id", strings.TrimSpace(tokenData.UserID),
		"user_name", strings.TrimSpace(tokenData.UserName),
	)
	if err := p.persistLoginToken(ctx, tokenData); err != nil {
		logging.AuthDebug(
			"auth.login.oauth.persistence.error",
			"callback_port", port,
			"corp_id", strings.TrimSpace(tokenData.CorpID),
			"user_id", strings.TrimSpace(tokenData.UserID),
			"error", err,
		)
		return nil, fmt.Errorf("%s: %w", i18n.T("保存 token 失败"), err)
	}
	logging.AuthDebug(
		"auth.login.oauth.persistence.done",
		"callback_port", port,
		"corp_id", strings.TrimSpace(tokenData.CorpID),
		"user_id", strings.TrimSpace(tokenData.UserID),
		"user_name", strings.TrimSpace(tokenData.UserName),
	)

	// Persist the exact pair snapshotted before authorization began.
	p.persistAppConfigIfNeeded()

	return tokenData, nil
}

func oauthExchangeDisplayError(err error) string {
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) && statusErr != nil {
		return fmt.Sprintf("HTTP %d: token exchange failed", statusErr.StatusCode)
	}
	return err.Error()
}

// GetTokenSnapshot returns a valid token together with its expiry metadata.
// Storage and refresh failures retain their original cause; only a confirmed
// missing credential is reported as ErrTokenDataNotFound.
func (p *OAuthProvider) GetTokenSnapshot(ctx context.Context) (*TokenData, error) {
	data, err := oauthLoadToken(p.configDir)
	if err != nil {
		if errors.Is(err, ErrTokenDataNotFound) || os.IsNotExist(err) {
			return nil, fmt.Errorf("%s: %w", i18n.T("未登录，请运行 dws auth login"), ErrTokenDataNotFound)
		}
		return nil, fmt.Errorf("load access token: %w", err)
	}
	profileSelector := StableTokenProfileSelector(p.configDir, data)

	// Fast path: access_token still valid — no lock needed.
	if data.IsAccessTokenValid() {
		return data, nil
	}

	// Slow path: token expired — try locked refresh.
	if data.IsRefreshTokenValid() {
		refreshed, rErr := p.lockedRefresh(ctx)
		if rErr == nil {
			return refreshed, nil
		}
		// A network, timeout, rate-limit or 5xx failure does not invalidate the
		// refresh credential. Keep the profile active so a long-running source
		// can retry after backoff. Terminal and unknown failures remain fatal.
		if ClassifyRefreshFailure(rErr) != RefreshFailureTransient {
			_ = oauthMarkProfile(p.configDir, profileSelector, ProfileStatusExpired)
		}
		if p.logger != nil {
			p.logger.Warn(i18n.T("refresh_token 刷新失败"), "error", rErr)
		}
		var exchangeErr *MCPTokenExchangeError
		if errors.As(rErr, &exchangeErr) && exchangeErr.requiresReauthorization() {
			return nil, fmt.Errorf(
				"%s: %w",
				legacyRefreshReauthorizationGuidance(profileSelector),
				rErr,
			)
		}
		return nil, fmt.Errorf("%s: %w", i18n.T("refresh_token 刷新失败"), rErr)
	} else {
		_ = oauthMarkProfile(p.configDir, profileSelector, ProfileStatusExpired)
	}

	return nil, fmt.Errorf("%s: %w", i18n.T("所有凭证已失效，请运行 dws auth login 重新登录"), ErrTokenDataNotFound)
}

func legacyRefreshReauthorizationGuidance(profileSelector string) string {
	guidance := "旧版登录态已无法由当前认证服务刷新；本地 profile 已保留，请重新运行 dws auth login 完成一次重新授权"
	profileSelector = strings.TrimSpace(profileSelector)
	if profileSelector == "" {
		return guidance
	}
	return fmt.Sprintf(
		"%s；为保留原身份，请把 --profile 参数设置为下方 profile 标识（标识仅作数据展示，不是可执行命令）:\nprofile: %s",
		guidance,
		strconv.Quote(profileSelector),
	)
}

// GetAccessToken returns a valid access token, auto-refreshing if needed.
// Uses a file lock with double-check pattern to prevent concurrent refresh
// from multiple CLI processes.
func (p *OAuthProvider) GetAccessToken(ctx context.Context) (string, error) {
	data, err := p.GetTokenSnapshot(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(data.AccessToken), nil
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
	lock, err := oauthAcquireLock(ctx, p.configDir)
	if err != nil {
		return nil, fmt.Errorf("acquiring dual lock: %w", err)
	}
	defer lock.Release()

	// Double-check: re-load from disk — another goroutine/process may have refreshed
	// while we were waiting for the lock.
	data, err := loadOAuthTokenUnderHeldLock(p.configDir, RuntimeProfile())
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
	if err := preflightTokenRefreshPersistence(p.configDir, data); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("本地登录态无法安全更新"), err)
	}

	if p.logger != nil {
		p.logger.Debug("refreshing token (dual-locked)")
	}
	refreshed, rErr := oauthRefreshToken(p, ctx, data)
	if rErr == nil || !isRefreshTokenRejected(rErr) {
		return refreshed, rErr
	}
	// A stale identity slot can survive an older organization-only refresh.
	// Retry once with the same-corp organization mirror while holding the
	// existing dual lock; the fallback marks the publication so the rotated
	// credential is written back into the mirror slot it consumed.
	logging.AuthDebug(
		"auth.refresh.fallback.triggered",
		"corp_id", strings.TrimSpace(data.CorpID),
		"user_id", strings.TrimSpace(data.UserID),
		"error", rErr,
	)
	fallback, fErr := p.refreshFromOrgSlot(ctx, data)
	if fErr != nil {
		logging.AuthDebug("auth.refresh.fallback.unavailable", "error", fErr)
		// The organization mirror may be absent for long-lived local logins
		// that predate mirror publication. Recover from the legacy global
		// slot before giving up.
		if recovered, recoverErr := p.recoverRefreshFromLegacyGlobalSlot(ctx, data, rErr); recoverErr == nil {
			return recovered, nil
		}
		return nil, rErr
	}
	if p.logger != nil {
		p.logger.Warn(i18n.T("当前身份的 refresh_token 已失效，已从组织镜像 token 恢复登录态"))
	}
	return fallback, nil
}

// refreshFromOrgSlot retries a rejected refresh with the token mirrored in
// the organization slot. The mirror must match the current corp, be valid,
// and differ from the rejected token. When both slots carry user identities,
// they must agree; legacy mirrors with an empty UserID are backfilled from the
// current identity before refresh.
func (p *OAuthProvider) refreshFromOrgSlot(ctx context.Context, current *TokenData) (*TokenData, error) {
	if current == nil {
		return nil, fmt.Errorf("no current token data")
	}
	corpID := strings.TrimSpace(current.CorpID)
	if corpID == "" {
		return nil, fmt.Errorf("current token has no corpId")
	}
	orgData, err := tokenLoadKeychainForCorpID(corpID)
	if err != nil {
		return nil, err
	}
	if orgData == nil {
		return nil, ErrTokenDataNotFound
	}
	if strings.TrimSpace(orgData.CorpID) != corpID {
		return nil, fmt.Errorf("organization token mirror for corpId %q contains token for corpId %q; refusing refresh fallback", corpID, orgData.CorpID)
	}
	if !orgData.IsRefreshTokenValid() {
		return nil, fmt.Errorf("organization mirror refresh_token 已过期")
	}
	if orgData.RefreshToken == current.RefreshToken {
		return nil, fmt.Errorf("organization mirror holds the same rejected refresh_token")
	}
	currentUserID := strings.TrimSpace(current.UserID)
	orgUserID := strings.TrimSpace(orgData.UserID)
	if currentUserID != "" && orgUserID != "" && orgUserID != currentUserID {
		return nil, fmt.Errorf("organization token mirror for corpId %q belongs to userId %q; refusing refresh fallback for userId %q", corpID, orgData.UserID, current.UserID)
	}
	if orgUserID == "" {
		orgData.UserID = current.UserID
		orgData.UserName = current.UserName
	}
	// The refresh below consumes the mirror's refresh_token. Mark the
	// publication so persistence writes the rotated credential back into the
	// organization slot even under an explicit runtime selector whose plan
	// would otherwise skip it (for example a preserved unresolved sibling).
	orgData.RepairOrganizationMirror = true
	refreshed, err := oauthRefreshToken(p, ctx, orgData)
	if err != nil {
		return nil, err
	}
	logging.AuthDebug(
		"auth.refresh.fallback.success",
		"corp_id", corpID,
		"new_at_expires_at", refreshed.ExpiresAt.Format(time.RFC3339),
	)
	return refreshed, nil
}

func (p *OAuthProvider) recoverRefreshFromLegacyGlobalSlot(ctx context.Context, selected *TokenData, refreshErr error) (*TokenData, error) {
	var exchangeErr *MCPTokenExchangeError
	if !errors.As(refreshErr, &exchangeErr) || !exchangeErr.requiresReauthorization() {
		return nil, refreshErr
	}
	if selected == nil {
		return nil, refreshErr
	}
	logging.AuthDebug("auth.refresh.legacy_recovery.triggered",
		"corp_id", strings.TrimSpace(selected.CorpID),
		"user_id", strings.TrimSpace(selected.UserID),
		"refresh_error_code", exchangeErr.Code,
	)
	legacy, loadErr := tokenLoadKeychain()
	if loadErr != nil {
		logging.AuthDebug("auth.refresh.legacy_recovery.failed", "step", "load_legacy", "error", loadErr)
		return nil, refreshErr
	}
	if legacy == nil {
		logging.AuthDebug("auth.refresh.legacy_recovery.failed", "step", "load_legacy", "reason", "empty_legacy")
		return nil, refreshErr
	}
	if !legacyGlobalRefreshCandidateMatches(p.configDir, selected, legacy) {
		logging.AuthDebug("auth.refresh.legacy_recovery.failed",
			"step", "candidate_mismatch",
			"legacy_corp_id", strings.TrimSpace(legacy.CorpID),
			"legacy_user_id", strings.TrimSpace(legacy.UserID),
		)
		return nil, refreshErr
	}
	recovered := *legacy
	if strings.TrimSpace(recovered.UserID) == "" {
		recovered.UserID = strings.TrimSpace(selected.UserID)
	}
	if strings.TrimSpace(recovered.UserName) == "" {
		recovered.UserName = strings.TrimSpace(selected.UserName)
	}
	if recovered.IsAccessTokenValid() {
		if err := oauthSaveTokenLocked(p.configDir, &recovered); err != nil {
			logging.AuthDebug("auth.refresh.legacy_recovery.failed", "step", "save", "error", err)
			return nil, refreshErr
		}
		logging.AuthDebug("auth.refresh.legacy_recovery.success", "via", "valid_access_token")
		return &recovered, nil
	}
	if !recovered.IsRefreshTokenValid() {
		logging.AuthDebug("auth.refresh.legacy_recovery.failed", "step", "refresh_expired")
		return nil, refreshErr
	}
	if strings.TrimSpace(recovered.RefreshToken) == strings.TrimSpace(selected.RefreshToken) {
		logging.AuthDebug("auth.refresh.legacy_recovery.failed", "step", "same_refresh_token")
		return nil, refreshErr
	}
	if err := preflightTokenRefreshPersistence(p.configDir, &recovered); err != nil {
		logging.AuthDebug("auth.refresh.legacy_recovery.failed", "step", "preflight", "error", err)
		return nil, refreshErr
	}
	refreshed, recoverErr := oauthRefreshToken(p, ctx, &recovered)
	if recoverErr != nil {
		logging.AuthDebug("auth.refresh.legacy_recovery.failed", "step", "refresh", "error", recoverErr)
		return nil, refreshErr
	}
	logging.AuthDebug("auth.refresh.legacy_recovery.success", "via", "refresh")
	return refreshed, nil
}

func legacyGlobalRefreshCandidateMatches(configDir string, selected, legacy *TokenData) bool {
	if selected == nil || legacy == nil {
		return false
	}
	selectedCorpID := strings.TrimSpace(selected.CorpID)
	legacyCorpID := strings.TrimSpace(legacy.CorpID)
	if selectedCorpID == "" || legacyCorpID != selectedCorpID {
		return false
	}
	selectedUserID := strings.TrimSpace(selected.UserID)
	legacyUserID := strings.TrimSpace(legacy.UserID)
	if legacyUserID != "" {
		return legacyUserID == selectedUserID
	}
	return legacyGlobalBlankUserIDMatchesSingleProfile(configDir, selectedCorpID, selectedUserID)
}

func legacyGlobalBlankUserIDMatchesSingleProfile(configDir, corpID, userID string) bool {
	if strings.TrimSpace(corpID) == "" {
		return false
	}
	cfg, err := tokenLoadProfiles(configDir)
	if err != nil || cfg == nil {
		logging.AuthDebug("auth.refresh.legacy_recovery.blank_user_rejected", "reason", "profiles_error", "error", err)
		return false
	}
	profiles := profilesForCorpID(cfg, corpID)
	if len(profiles) != 1 {
		logging.AuthDebug("auth.refresh.legacy_recovery.blank_user_rejected",
			"reason", "multi_profile",
			"corp_id", strings.TrimSpace(corpID),
			"profile_count", len(profiles),
		)
		return false
	}
	profile := profiles[0]
	if profile != nil && sameProfileIdentity(profile.CorpID, profile.UserID, corpID, userID) {
		return true
	}
	profileUserID := ""
	if profile != nil {
		profileUserID = strings.TrimSpace(profile.UserID)
	}
	logging.AuthDebug("auth.refresh.legacy_recovery.blank_user_rejected",
		"reason", "identity_mismatch",
		"selected_user_id", strings.TrimSpace(userID),
		"profile_user_id", profileUserID,
	)
	return false
}

// ExchangeAuthCode takes an AuthCode and an optional UserID provided by an
// external host, exchanges it for tokens, and persists them.
func (p *OAuthProvider) ExchangeAuthCode(ctx context.Context, authCode, uid string) (*TokenData, error) {
	if err := prepareLoginPersistence(p.configDir); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("本地登录态无法安全更新"), err)
	}
	tokenData, err := oauthExchange(p, ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("换取 token 失败"), err)
	}
	if uid != "" {
		tokenData.UserID = strings.TrimSpace(uid)
		if err := p.persistKnownLoginToken(tokenData); err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.T("保存 token 失败"), err)
		}
		p.persistAppConfigIfNeeded()
		return tokenData, nil
	}
	if err := p.persistLoginToken(ctx, tokenData); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("保存 token 失败"), err)
	}
	p.persistAppConfigIfNeeded()
	return tokenData, nil
}

func (p *OAuthProvider) persistLoginToken(ctx context.Context, tokenData *TokenData) error {
	corpID, userID, userName := "", "", ""
	if tokenData != nil {
		corpID = strings.TrimSpace(tokenData.CorpID)
		userID = strings.TrimSpace(tokenData.UserID)
		userName = strings.TrimSpace(tokenData.UserName)
	}
	logging.AuthDebug(
		"auth.login.oauth.identity.before_enrich",
		"corp_id", corpID,
		"user_id", userID,
		"user_name", userName,
	)
	if err := p.prepareLoginToken(ctx, tokenData); err != nil {
		return err
	}
	logging.AuthDebug(
		"auth.login.oauth.identity.after_enrich",
		"corp_id", strings.TrimSpace(tokenData.CorpID),
		"user_id", strings.TrimSpace(tokenData.UserID),
		"user_name", strings.TrimSpace(tokenData.UserName),
	)
	if err := repairLoginCiphertextMismatchTargets(p.configDir, tokenData); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("本地登录态无法安全更新"), err)
	}
	if err := preflightTokenWritePersistence(p.configDir, tokenData); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("本地登录态无法安全更新"), err)
	}
	if err := oauthSaveToken(p.configDir, tokenData); err != nil {
		return err
	}
	return nil
}

func (p *OAuthProvider) prepareLoginToken(ctx context.Context, tokenData *TokenData) error {
	if tokenData == nil {
		return fmt.Errorf("token data is empty")
	}
	tokenData.FreshAuthorization = true
	if p != nil && p.IdentityEnricher != nil {
		if err := p.IdentityEnricher(ctx, tokenData); err != nil {
			return fmt.Errorf("resolve login identity: %w", err)
		}
	}
	// v1.0.52 and earlier deliberately persisted the freshly exchanged token
	// before best-effort contact enrichment. External-worker accounts can have a
	// valid organization token while contact cannot return a userId, so rejecting
	// that shape here makes an otherwise successful reauthorization impossible.
	// SaveTokenData remains the safety boundary: an unresolved organization token
	// cannot overwrite an organization that already has exact account identities.
	return nil
}

func (p *OAuthProvider) persistKnownLoginToken(tokenData *TokenData) error {
	if tokenData == nil {
		return fmt.Errorf("token data is empty")
	}
	if strings.TrimSpace(tokenData.CorpID) != "" && strings.TrimSpace(tokenData.UserID) == "" {
		return fmt.Errorf("resolve login identity: userId is required for corpId %q", tokenData.CorpID)
	}
	tokenData.FreshAuthorization = true
	if err := repairLoginCiphertextMismatchTargets(p.configDir, tokenData); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("本地登录态无法安全更新"), err)
	}
	if err := preflightTokenWritePersistence(p.configDir, tokenData); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("本地登录态无法安全更新"), err)
	}
	return oauthSaveToken(p.configDir, tokenData)
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
	if p == nil {
		return
	}
	if p.credentials == nil {
		p.snapshotCredentialPair()
	}
	if p.credentialErr != nil || p.credentials == nil {
		return
	}
	clientID := p.credentials.ClientID
	clientSecret := p.credentials.ClientSecret

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
