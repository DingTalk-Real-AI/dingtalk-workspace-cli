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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/i18n"
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

	// Fall through: full browser OAuth flow.
	// Find a free port for the callback server.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting callback listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, CallbackPath)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("authCode")
		if code == "" {
			code = r.URL.Query().Get("code")
		}
		if code == "" {
			select {
			case errCh <- errors.New(i18n.T("回调中未收到授权码")):
			default:
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, i18n.T("授权失败：未收到授权码"))
			return
		}
		select {
		case codeCh <- code:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, successHTML)
		default:
			// Select already exited (timeout/cancel); discard late callback.
			w.WriteHeader(http.StatusGone)
		}
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

	var authCode string
	select {
	case authCode = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-timeout.C:
		return nil, errors.New(i18n.T("授权超时（5分钟），请重试"))
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	tokenData, err := p.exchangeCode(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("换取 token 失败"), err)
	}
	if err := SaveTokenData(p.configDir, tokenData); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("保存 token 失败"), err)
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
