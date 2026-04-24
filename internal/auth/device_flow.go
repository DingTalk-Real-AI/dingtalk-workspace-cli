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
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/fatih/color"
)

const (
	// defaultPollInterval is the default seconds between device token polls.
	// The server-side Redis TTL is 10 minutes; a 2-second interval keeps the
	// user-perceived latency low while staying well within rate limits.
	defaultPollInterval = 2
	// maxPollInterval caps the polling interval to prevent DoS via slow_down.
	maxPollInterval = 30
	// maxPollTotalWait caps the total wait time for device authorization.
	// Aligned with the server-side Redis TTL (10 minutes).
	maxPollTotalWait = 10 * time.Minute
)

type DeviceFlowProvider struct {
	configDir       string
	clientID        string
	scope           string
	baseURL         string
	terminalBaseURL string
	logger          *slog.Logger
	Output          io.Writer
	httpClient      *http.Client
}

func NewDeviceFlowProvider(configDir string, logger *slog.Logger) *DeviceFlowProvider {
	return &DeviceFlowProvider{
		configDir:       configDir,
		clientID:        ClientID(),
		scope:           DefaultScopes,
		baseURL:         DefaultDeviceBaseURL,
		terminalBaseURL: GetMCPBaseURL(),
		logger:          logger,
		Output:          os.Stderr,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *DeviceFlowProvider) SetBaseURL(baseURL string) {
	p.baseURL = strings.TrimRight(baseURL, "/")
}

// SetTerminalBaseURL sets the terminal API base URL for device flow polling.
func (p *DeviceFlowProvider) SetTerminalBaseURL(baseURL string) {
	p.terminalBaseURL = strings.TrimRight(baseURL, "/")
}

// SetScope overrides the OAuth scope for the device flow.
func (p *DeviceFlowProvider) SetScope(scope string) {
	if p != nil {
		p.scope = scope
	}
}

func (p *DeviceFlowProvider) output() io.Writer {
	if p != nil && p.Output != nil {
		return p.Output
	}
	return io.Discard
}

type DeviceAuthResponse struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
	FlowID                  string `json:"flowId"`
}

type DeviceTokenResponse struct {
	AuthCode    string `json:"authCode"`
	RedirectURL string `json:"redirectUrl"`
	Error       string `json:"error"`
}

// DevicePollResponse represents the response from the terminal API poll endpoint.
type DevicePollResponse struct {
	Success bool           `json:"success"`
	Code    string         `json:"code,omitempty"`
	Message string         `json:"message,omitempty"`
	Data    DevicePollData `json:"data"`
}

type DevicePollData struct {
	Status   string `json:"status"`
	AuthCode string `json:"authCode,omitempty"`
	FlowID   string `json:"flowId,omitempty"`
}

type serviceResult struct {
	Success   bool            `json:"success"`
	Result    json.RawMessage `json:"result"`
	ErrorCode string          `json:"errorCode"`
	ErrorMsg  string          `json:"errorMsg"`
}

// resetCredentialState clears any stale credential state inherited from
// previous login methods (OAuth, PAT, etc.) so that device flow always
// starts fresh by fetching clientID from MCP.
//
// This is a defensive measure: no matter what a prior login wrote to
// app.json or runtime globals, device flow will re-fetch from MCP and
// set the correct clientIDFromMCP flag, ensuring exchangeCode() uses
// the MCP proxy path (which doesn't require clientSecret).
func (p *DeviceFlowProvider) resetCredentialState() {
	p.clientID = ""
	clientMu.Lock()
	clientIDFromMCP = false
	clientMu.Unlock()
}

func (p *DeviceFlowProvider) Login(ctx context.Context) (*TokenData, error) {
	// Defensive reset: clear any stale credential state from previous login
	// methods (OAuth scan, PAT, etc.) so we always re-fetch from MCP.
	// This ensures --device login works regardless of what app.json contains.
	p.resetCredentialState()

	if p.logger != nil {
		p.logger.Debug("fetching client ID from MCP server (device flow always re-fetches)")
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

	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		tokenData, err := p.loginOnce(ctx, attempt)
		if err == nil {
			return tokenData, nil
		}
		if isInvalidGrantError(err) && attempt < maxAttempts {
			dfPrintWarning(p.output(), i18n.T("授权码已过期，正在重新发起设备授权流程..."))
			_, _ = fmt.Fprintln(p.output(), "")
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("%s", i18n.Tf("设备授权流程失败（已重试 %d 次）", maxAttempts))
}

func (p *DeviceFlowProvider) loginOnce(ctx context.Context, attempt int) (*TokenData, error) {
	dfPrintStep(p.output(), 1, i18n.T("请求设备授权码..."), attempt)
	_, _ = fmt.Fprintln(p.output(), "")

	authResp, err := p.requestDeviceCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("请求设备授权码失败"), err)
	}
	dfPrintDeviceCodeBox(p.output(), authResp)

	if authResp.VerificationURIComplete != "" {
		if bErr := openBrowser(authResp.VerificationURIComplete); bErr != nil && p.logger != nil {
			p.logger.Debug("could not open browser", "error", bErr)
		}
	}

	dfPrintStep(p.output(), 2, i18n.T("等待用户授权..."), 0)
	dfPrintDim(p.output(), fmt.Sprintf(i18n.T("  (每 %d 秒轮询一次)"), authResp.Interval))
	_, _ = fmt.Fprintln(p.output(), "")

	tokenResult, err := p.waitForAuthorization(ctx, authResp)
	if err != nil {
		return nil, err
	}

	_, _ = fmt.Fprintln(p.output(), "")
	dfPrintStep(p.output(), 3, i18n.T("使用授权码换取 Access Token..."), 0)

	oauthProvider := &OAuthProvider{
		configDir: p.configDir,
		clientID:  p.clientID,
		logger:    p.logger,
	}
	tokenData, err := oauthProvider.exchangeCode(ctx, tokenResult.AuthCode)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("换取 token 失败"), err)
	}

	// Check if CLI auth is enabled for this organization (fail-closed: block on error)
	dfPrintStep(p.output(), 4, i18n.T("检查组织 CLI 授权状态..."), 0)
	authStatus, authErr := oauthProvider.CheckCLIAuthEnabled(ctx, tokenData.AccessToken)
	if authErr != nil {
		_, _ = fmt.Fprintln(p.output(), "")
		_, _ = fmt.Fprintln(p.output(), dfRed(i18n.T("⚠️  无法检查 CLI 数据访问权限状态")))
		_, _ = fmt.Fprintln(p.output(), i18n.T("   请检查网络连接后重试。"))
		_, _ = fmt.Fprintln(p.output(), "")
		return nil, fmt.Errorf("%s: %w", i18n.T("检查 CLI 授权状态失败"), authErr)
	}
	denialReason := classifyDenialReason(authStatus, os.Getenv("DWS_CHANNEL"))
	if denialReason != "" {
		_, _ = fmt.Fprintln(p.output(), "")
		switch denialReason {
		case "user_forbidden":
			_, _ = fmt.Fprintln(p.output(), dfRed(i18n.T("⚠️  该组织已禁止所有成员使用 CLI")))
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, errors.New(i18n.T("该组织已禁止所有成员使用 CLI"))
		case "user_not_allowed":
			_, _ = fmt.Fprintln(p.output(), dfRed(i18n.T("⚠️  您不在该组织的 CLI 授权人员范围内")))
			_, _ = fmt.Fprintln(p.output(), i18n.T("   请联系组织管理员将您加入 CLI 授权人员名单。"))
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, errors.New(i18n.T("您不在该组织的 CLI 授权人员范围内，请联系组织管理员"))
		case "channel_not_allowed":
			ch := os.Getenv("DWS_CHANNEL")
			_, _ = fmt.Fprintf(p.output(), dfRed(i18n.T("⚠️  当前渠道 %s 未获得该组织授权"))+"\n", ch)
			_, _ = fmt.Fprintln(p.output(), i18n.T("   请联系组织管理员开通该渠道的访问权限。"))
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, fmt.Errorf(i18n.T("当前渠道 %s 未获得该组织授权，请联系组织管理员"), ch)
		case "channel_required":
			_, _ = fmt.Fprintln(p.output(), dfRed(i18n.T("⚠️  当前组织已开启渠道管控")))
			_, _ = fmt.Fprintln(p.output(), i18n.T("   请升级到最新版本的 CLI 后重试。"))
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, errors.New(i18n.T("当前组织已开启渠道管控，请升级到最新版本的 CLI 后重试"))
		case "no_auth":
			_, _ = fmt.Fprintln(p.output(), dfRed(i18n.T("⚠️  认证已失效")))
			_, _ = fmt.Fprintln(p.output(), i18n.T("   请执行 dws auth 重新登录。"))
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, errors.New(i18n.T("认证已失效，请执行 dws auth 重新登录"))
		default:
			// cli_not_enabled or unknown — show existing admin-apply flow
			_, _ = fmt.Fprintln(p.output(), dfRed(i18n.T("⚠️  该组织尚未开启 CLI 数据访问权限")))
			_, _ = fmt.Fprintln(p.output(), i18n.T("   你所选择的组织管理员尚未开启「允许成员通过 CLI 访问其个人数据」的权限。"))
			_, _ = fmt.Fprintln(p.output(), "")

			admins, adminErr := GetSuperAdmins(ctx, tokenData.AccessToken)
			if adminErr == nil && admins.Success && len(admins.Result) > 0 {
				maxAdmins := 3
				if len(admins.Result) < maxAdmins {
					maxAdmins = len(admins.Result)
				}
				var adminNames []string
				for i := 0; i < maxAdmins; i++ {
					adminNames = append(adminNames, admins.Result[i].Name)
				}
				_, _ = fmt.Fprintf(p.output(), "   %s%s\n", i18n.T("组织主管理员："), strings.Join(adminNames, "、"))
			}

			_, _ = fmt.Fprintln(p.output(), i18n.T("   请联系组织主管理员开启后重新登录。"))
			_, _ = fmt.Fprintln(p.output(), "")
			_, _ = fmt.Fprintf(p.output(), "   %s%s\n", i18n.T("管理员操作入口："), config.GetDeveloperSettingsURL())
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, errors.New(i18n.T("该组织尚未开启 CLI 数据访问权限，请联系管理员开启"))
		}
	}

	// Save token data with associated client ID for refresh
	tokenData.ClientID = p.clientID
	if err := SaveTokenData(p.configDir, tokenData); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("保存 token 失败"), err)
	}

	// Always persist clientId to app.json so future process startups
	// can load it via ResolveAppCredentials and populate DWS_CLIENT_ID env.
	if p.clientID != "" {
		_ = os.Setenv("DWS_CLIENT_ID", p.clientID)
		_ = SaveAppConfig(p.configDir, &AppConfig{ClientID: p.clientID})
	}

	// Persist app credentials if using custom client credentials
	oauthProvider.persistAppConfigIfNeeded()

	return tokenData, nil
}

func (p *DeviceFlowProvider) requestDeviceCode(ctx context.Context) (*DeviceAuthResponse, error) {
	params := url.Values{"client_id": {p.clientID}}
	if p.scope != "" {
		params.Set("scope", p.scope)
	}
	endpoint := p.baseURL + DeviceCodePath
	body, err := p.postForm(ctx, endpoint, params)
	if err != nil {
		return nil, err
	}

	var sr serviceResult
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("解析响应失败"), err)
	}
	if !sr.Success {
		return nil, fmt.Errorf("%s: [%s] %s", i18n.T("服务端返回错误"), sr.ErrorCode, sr.ErrorMsg)
	}

	var resp DeviceAuthResponse
	if err := json.Unmarshal(sr.Result, &resp); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("解析设备授权数据失败"), err)
	}
	if resp.DeviceCode == "" || resp.UserCode == "" {
		return nil, errors.New(i18n.T("服务端返回了空的 device_code 或 user_code"))
	}
	if resp.Interval <= 0 || resp.Interval > maxPollInterval {
		resp.Interval = defaultPollInterval
	}
	if resp.ExpiresIn <= 0 {
		resp.ExpiresIn = 900
	}
	return &resp, nil
}

func (p *DeviceFlowProvider) pollDeviceToken(ctx context.Context, deviceCode string) (*DeviceTokenResponse, error) {
	params := url.Values{
		"grant_type":  {DeviceGrantType},
		"device_code": {deviceCode},
		"client_id":   {p.clientID},
	}
	endpoint := p.baseURL + DeviceTokenPath
	body, err := p.postForm(ctx, endpoint, params)
	if err != nil {
		return nil, err
	}

	var sr serviceResult
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("解析响应失败"), err)
	}
	if !sr.Success {
		return nil, fmt.Errorf("%s: %s %s", i18n.T("服务端返回错误"), sr.ErrorCode, sr.ErrorMsg)
	}

	var resp DeviceTokenResponse
	if err := json.Unmarshal(sr.Result, &resp); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("解析令牌数据失败"), err)
	}
	return &resp, nil
}

// pollDeviceStatus polls the terminal API for device authorization status.
//
// Note: The server returns success=false for REJECTED and EXPIRED terminal
// states (with a valid data.Status value).  These are normal business outcomes,
// not transport errors, so we return the response to the caller and let the
// status-switch handle them.
func (p *DeviceFlowProvider) pollDeviceStatus(ctx context.Context, flowID string) (*DevicePollResponse, error) {
	endpoint := fmt.Sprintf("%s%s?flowId=%s", p.terminalBaseURL, DevicePollPath, url.QueryEscape(flowID))
	body, err := p.doGet(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var resp DevicePollResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("解析响应失败"), err)
	}
	// REJECTED/EXPIRED carry success=false but have a valid data.Status;
	// only treat as a real server error when data.Status is empty.
	if !resp.Success && resp.Data.Status == "" {
		return nil, fmt.Errorf("%s: [%s] %s", i18n.T("服务端返回错误"), resp.Code, resp.Message)
	}
	return &resp, nil
}

func (p *DeviceFlowProvider) waitForAuthorization(ctx context.Context, auth *DeviceAuthResponse) (*DeviceTokenResponse, error) {
	if auth.FlowID == "" {
		// Keep the pre-flowId device-code polling path for regular device flow
		// login responses that do not include terminal polling metadata.
		return p.waitForAuthorizationByDeviceCode(ctx, auth)
	}
	return p.waitForAuthorizationByFlowID(ctx, auth)
}

func (p *DeviceFlowProvider) waitForAuthorizationByFlowID(ctx context.Context, auth *DeviceAuthResponse) (*DeviceTokenResponse, error) {
	startTime := time.Now()
	interval := time.Duration(auth.Interval) * time.Second
	deadline := time.Duration(auth.ExpiresIn) * time.Second
	pollCount := 0

	for {
		elapsed := time.Since(startTime)
		if elapsed >= maxPollTotalWait || elapsed >= deadline {
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, fmt.Errorf("%s", i18n.Tf("设备授权码已过期（%d 秒），请重试", auth.ExpiresIn))
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		pollCount++
		elapsedSec := int(time.Since(startTime).Seconds())
		dfPrintPollStatus(p.output(), pollCount, elapsedSec)

		pollResp, err := p.pollDeviceStatus(ctx, auth.FlowID)
		if err != nil {
			dfPrintPollResult(p.output(), "network_error", i18n.T("网络错误，继续重试..."))
			if p.logger != nil {
				p.logger.Debug("poll error", "error", err)
			}
			continue
		}

		switch pollResp.Data.Status {
		case StatusApproved:
			dfPrintPollResult(p.output(), "authorized", i18n.T("授权成功!"))
			return &DeviceTokenResponse{AuthCode: pollResp.Data.AuthCode}, nil
		case StatusPending:
			dfPrintPollResult(p.output(), "pending", i18n.T("等待用户授权..."))
		case StatusRejected:
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, errors.New(i18n.T("用户拒绝了授权请求"))
		case StatusExpired:
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, errors.New(i18n.T("设备授权码已过期"))
		default:
			dfPrintPollResult(p.output(), "unknown", fmt.Sprintf(i18n.T("未知状态: %s"), pollResp.Data.Status))
		}
	}
}

func (p *DeviceFlowProvider) waitForAuthorizationByDeviceCode(ctx context.Context, auth *DeviceAuthResponse) (*DeviceTokenResponse, error) {
	startTime := time.Now()
	interval := time.Duration(auth.Interval) * time.Second
	deadline := time.Duration(auth.ExpiresIn) * time.Second
	pollCount := 0

	for {
		elapsed := time.Since(startTime)
		if elapsed >= maxPollTotalWait || elapsed >= deadline {
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, fmt.Errorf("%s", i18n.Tf("设备授权码已过期（%d 秒），请重试", auth.ExpiresIn))
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		pollCount++
		elapsedSec := int(time.Since(startTime).Seconds())
		dfPrintPollStatus(p.output(), pollCount, elapsedSec)

		resp, err := p.pollDeviceToken(ctx, auth.DeviceCode)
		if err != nil {
			dfPrintPollResult(p.output(), "network_error", i18n.T("网络错误，继续重试..."))
			if p.logger != nil {
				p.logger.Debug("poll error", "error", err)
			}
			continue
		}

		if resp.Error == "" {
			dfPrintPollResult(p.output(), "authorized", i18n.T("授权成功!"))
			return resp, nil
		}
		switch resp.Error {
		case "authorization_pending":
			dfPrintPollResult(p.output(), "pending", i18n.T("等待用户授权..."))
		case "slow_down":
			interval += 5 * time.Second
			if interval > maxPollInterval*time.Second {
				interval = maxPollInterval * time.Second
			}
			dfPrintPollResult(p.output(), "slow_down", fmt.Sprintf(i18n.T("轮询过快，间隔增加至 %ds"), int(interval.Seconds())))
		case "access_denied":
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, errors.New(i18n.T("用户拒绝了授权请求"))
		case "expired_token":
			_, _ = fmt.Fprintln(p.output(), "")
			return nil, errors.New(i18n.T("设备授权码已过期"))
		default:
			dfPrintPollResult(p.output(), "unknown", fmt.Sprintf(i18n.T("未知错误: %s"), resp.Error))
		}
	}
}

func (p *DeviceFlowProvider) postForm(ctx context.Context, endpoint string, params url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("创建请求失败"), err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("发送请求失败"), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("读取响应失败"), err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateBody(body, 200))
	}
	return body, nil
}

// doGet performs an HTTP GET request and returns the response body.
func (p *DeviceFlowProvider) doGet(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("创建请求失败"), err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("发送请求失败"), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("读取响应失败"), err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateBody(body, 200))
	}
	return body, nil
}

// truncateBody returns a string of at most maxLen bytes from body, appending
// "...(truncated)" when the content exceeds the limit. This prevents leaking
// potentially sensitive response payloads in error messages.
func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "...(truncated)"
}

var (
	dfBold   = color.New(color.Bold).SprintFunc()
	dfGreen  = color.New(color.FgGreen).SprintFunc()
	dfYellow = color.New(color.FgYellow).SprintFunc()
	dfRed    = color.New(color.FgRed).SprintFunc()
	dfCyan   = color.New(color.FgCyan).SprintFunc()
	dfDim    = color.New(color.Faint).SprintFunc()
)

func dfPrintStep(w io.Writer, step int, message string, attempt int) {
	if attempt > 1 {
		_, _ = fmt.Fprintf(w, i18n.T("%s (第 %d 次尝试)\\n"), dfBold(fmt.Sprintf("▶ Step %d: %s", step, message)), attempt)
		return
	}
	_, _ = fmt.Fprintf(w, "%s\n", dfBold(fmt.Sprintf("▶ Step %d: %s", step, message)))
}

func dfPrintDeviceCodeBox(w io.Writer, auth *DeviceAuthResponse) {
	lines := []string{
		i18n.T("请在浏览器中打开以下链接，并输入授权码："),
		"",
		fmt.Sprintf(i18n.T("  链接: %s"), dfBold(auth.VerificationURI)),
		fmt.Sprintf(i18n.T("  授权码: %s"), dfBold(dfYellow(auth.UserCode))),
		"",
	}
	if auth.VerificationURIComplete != "" {
		lines = append(lines,
			i18n.T("或者直接打开以下链接："),
			fmt.Sprintf("  %s", dfCyan(auth.VerificationURIComplete)),
			"",
		)
	}
	lines = append(lines, dfDim(fmt.Sprintf(i18n.T("授权码将在 %d 秒后过期。"), auth.ExpiresIn)))
	dfPrintBox(w, lines)
	_, _ = fmt.Fprintln(w, "")
}

func dfPrintBox(w io.Writer, lines []string) {
	maxLen := 0
	for _, line := range lines {
		if l := dfPlainLength(line); l > maxLen {
			maxLen = l
		}
	}
	if maxLen < 50 {
		maxLen = 50
	}

	border := strings.Repeat("─", maxLen+4)
	_, _ = fmt.Fprintf(w, "  ┌%s┐\n", border)
	for _, line := range lines {
		pad := maxLen - dfPlainLength(line)
		if pad < 0 {
			pad = 0
		}
		_, _ = fmt.Fprintf(w, "  │  %s%s  │\n", line, strings.Repeat(" ", pad))
	}
	_, _ = fmt.Fprintf(w, "  └%s┘\n", border)
}

func dfPlainLength(s string) int {
	inEscape := false
	length := 0
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		length++
	}
	return length
}

func dfPrintPollStatus(w io.Writer, count, elapsedSec int) {
	_, _ = fmt.Fprintf(w, "  %s ", dfDim(fmt.Sprintf(i18n.T("[%d] 轮询中... (%ds)"), count, elapsedSec)))
}

func dfPrintPollResult(w io.Writer, status, message string) {
	switch status {
	case "authorized":
		_, _ = fmt.Fprintln(w, dfGreen(message))
	case "pending", "slow_down":
		_, _ = fmt.Fprintln(w, dfYellow(message))
	default:
		_, _ = fmt.Fprintln(w, dfRed(message))
	}
}

func dfPrintWarning(w io.Writer, message string) {
	_, _ = fmt.Fprintln(w, dfYellow(message))
}

func dfPrintDim(w io.Writer, message string) {
	_, _ = fmt.Fprintln(w, dfDim(message))
}

func isInvalidGrantError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid_grant") || (strings.Contains(msg, "code") && strings.Contains(msg, "expired"))
}
