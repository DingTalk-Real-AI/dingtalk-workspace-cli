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

package app

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/fatih/color"
)

const (
	// PatAuthRetryTimeout is the maximum time to wait for user authorization
	// when a PAT scope error is detected.
	PatAuthRetryTimeout = 10 * time.Minute

	// PatAuthPollInterval is how often we poll to check if the user has
	// completed authorization.
	PatAuthPollInterval = 5 * time.Second

	patScopeAuthRequiredCode = "PAT_SCOPE_AUTH_REQUIRED"
)

// PatScopeError holds information about a missing PAT scope.
type PatScopeError struct {
	OriginalError string
	Identity      string
	ErrorType     string
	Message       string
	Hint          string
	MissingScope  string
}

func (e *PatScopeError) Error() string {
	return e.OriginalError
}

// patScopeRegex matches PAT-protocol scope error patterns from the API.
// Only matches explicit scope-related keywords; generic "permission denied" or
// "forbidden" are intentionally excluded to avoid false positives on business
// authorization errors (e.g. mailbox access denied, 403 Forbidden).
var patScopeRegex = regexp.MustCompile(`(?i)(missing_scope|insufficient_scope|scope.*required)`)

// scopeValueRegex extracts a scope identifier (e.g. "calendar:read",
// "mail:user_mailbox.message:send") from an error message.
// Supports multi-segment scopes with multiple colons (resource:sub:action).
var scopeValueRegex = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9_.]*(?::[a-zA-Z][a-zA-Z0-9_.]*)+)`)

// identityValueRegex extracts an identity label from an error message.
var identityValueRegex = regexp.MustCompile(`(?i)identity["\s:]+([a-zA-Z_]+)`)

// isPatScopeError checks if an error looks like a PAT scope/permission error
// that can be resolved by re-authorizing with additional scopes.
func isPatScopeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())

	// Check for missing_scope pattern in error message or hint
	if patScopeRegex.MatchString(msg) {
		return true
	}

	var typed *apperrors.Error
	if stderrors.As(err, &typed) {
		// Check message, reason, and hint for scope-related patterns
		fullText := strings.ToLower(typed.Message + " " + typed.Reason + " " + typed.Hint)
		if typed.Category == apperrors.CategoryAuth {
			if strings.Contains(fullText, "missing_scope") || strings.Contains(fullText, "insufficient_scope") ||
				(strings.Contains(fullText, "scope") && strings.Contains(fullText, "required")) {
				return true
			}
		}
		// Any category with scope/permission hints
		if strings.Contains(fullText, "missing_scope") || strings.Contains(fullText, "insufficient_scope") {
			return true
		}
	}

	return false
}

// extractPatScopeError parses an error to extract PAT scope details.
func extractPatScopeError(err error) *PatScopeError {
	if err == nil {
		return nil
	}

	msg := err.Error()
	scope := ""

	var typed *apperrors.Error
	if stderrors.As(err, &typed) {
		msg = typed.Message
		if typed.Reason != "" {
			msg += " (" + typed.Reason + ")"
		}
	}

	// Try to extract scope value (e.g. "calendar:read") from error message.
	scopeMatch := scopeValueRegex.FindStringSubmatch(msg)
	if len(scopeMatch) > 1 {
		scope = scopeMatch[1]
	}

	// Try to extract identity from error message.
	identity := "user"
	identityMatch := identityValueRegex.FindStringSubmatch(msg)
	if len(identityMatch) > 1 {
		identity = identityMatch[1]
	}

	return &PatScopeError{
		OriginalError: err.Error(),
		Identity:      identity,
		ErrorType:     "missing_scope",
		Message:       msg,
		Hint:          fmt.Sprintf("run `dws auth login --scope %q` to authorize the missing scope", scope),
		MissingScope:  scope,
	}
}

// PrintPatAuthError prints a human-readable PAT authorization error.
func PrintPatAuthError(w io.Writer, scopeErr *PatScopeError) {
	bold := color.New(color.Bold).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	fmt.Fprintln(w)
	fmt.Fprintf(w, "{\n")
	fmt.Fprintf(w, "  %s: %s,\n", bold("\"ok\""), "false")
	fmt.Fprintf(w, "  %s: %q,\n", bold("\"identity\""), scopeErr.Identity)
	fmt.Fprintf(w, "  %s: {\n", bold("\"error\""))
	fmt.Fprintf(w, "    %s: %q,\n", bold("\"type\""), scopeErr.ErrorType)
	fmt.Fprintf(w, "    %s: %q,\n", bold("\"message\""), scopeErr.Message)
	fmt.Fprintf(w, "    %s: %q\n", bold("\"hint\""), scopeErr.Hint)
	fmt.Fprintf(w, "  }\n")
	fmt.Fprintf(w, "}\n")
	fmt.Fprintln(w)

	// Print authorization instructions
	fmt.Fprintf(w, "%s %s\n", green("▶"), bold("需要额外授权"))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", dim("#"), dim("运行以下命令完成授权"))

	if scopeErr.MissingScope != "" {
		fmt.Fprintf(w, "  %s %s\n", cyan("$"), cyan(fmt.Sprintf("dws auth login --scope %q", scopeErr.MissingScope)))
	} else {
		fmt.Fprintf(w, "  %s %s\n", cyan("$"), cyan("dws auth login"))
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s 在浏览器中打开授权链接，完成授权后重新执行命令\n", dim("ℹ"))
	fmt.Fprintln(w)
}

// PrintPatAuthJSON prints a machine-readable PAT authorization error.
func PrintPatAuthJSON(w io.Writer, scopeErr *PatScopeError) {
	fmt.Fprintln(w, buildPATScopeHostJSON(scopeErr))
}

// WaitForPatAuthorization polls until the user completes authorization or timeout.
// It returns true if authorization was completed, false if timed out or cancelled.
func WaitForPatAuthorization(ctx context.Context, configDir string, output io.Writer) bool {
	bold := color.New(color.Bold).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	timeout := PatAuthRetryTimeout
	deadline := time.Now().Add(timeout)
	pollTicker := time.NewTicker(PatAuthPollInterval)
	defer pollTicker.Stop()
	start := time.Now()

	fmt.Fprintln(output)
	fmt.Fprintf(output, "%s %s\n", yellow("⏳"), bold("等待用户授权..."))
	fmt.Fprintf(output, "  %s 请在另一个终端完成 dws auth login 授权\n", dim("ℹ"))
	fmt.Fprintf(output, "  %s 超时时间: %s\n", dim("⏱"), timeout)
	fmt.Fprintln(output)

	pollCount := 0
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(output, "%s 操作已取消\n", red("✗"))
			return false

		case <-time.After(time.Until(deadline)):
			fmt.Fprintf(output, "%s 等待授权超时 (%s)\n", red("✗"), timeout)
			fmt.Fprintf(output, "  %s 请重新执行命令\n", dim("ℹ"))
			return false

		case <-pollTicker.C:
			pollCount++
			elapsed := time.Since(start).Truncate(time.Second)
			remaining := time.Until(deadline).Truncate(time.Second)

			// Check if token is now valid
			tokenData, err := authpkg.LoadTokenData(configDir)
			if err == nil && tokenData != nil {
				if tokenData.IsAccessTokenValid() || tokenData.IsRefreshTokenValid() {
					fmt.Fprintf(output, "\r%s %s (%s 已用, %s 剩余)          \n",
						green("✓"), bold("授权成功!"), elapsed, remaining)
					fmt.Fprintln(output)
					return true
				}
			}

			// Show polling status
			fmt.Fprintf(output, "\r%s [%d] 等待授权中... (%s 已用, %s 剩余)          ",
				dim("⟳"), pollCount, elapsed, remaining)
		}
	}
}

// retryWithPatAuthRetry wraps an invocation that failed with a PAT scope error.
// It waits for the user to complete authorization and then retries the invocation.
func retryWithPatAuthRetry(ctx context.Context, runner executor.Runner, invocation executor.Invocation, scopeErr *PatScopeError, configDir string, output io.Writer) (executor.Result, error) {
	hostOwnedPAT := authpkg.HostOwnsPATFlow()
	slog.Debug("pat.host_owned_decision",
		"site", "retryWithPatAuthRetry",
		"hostOwned", hostOwnedPAT,
		"agentCodeEnvSet", os.Getenv(authpkg.AgentCodeEnv) != "",
	)
	if hostOwnedPAT {
		return executor.Result{}, &apperrors.PATError{RawJSON: buildPATScopeHostJSON(scopeErr)}
	}

	// Print the PAT error in human-readable format
	PrintPatAuthError(output, scopeErr)

	// Wait for user to complete authorization
	authorized := WaitForPatAuthorization(ctx, configDir, output)
	if !authorized {
		return executor.Result{}, apperrors.NewAuth(
			"等待用户授权超时",
			apperrors.WithReason("pat_auth_timeout"),
			apperrors.WithHint(fmt.Sprintf("授权超时 (%s)，请重新执行命令", PatAuthRetryTimeout)),
			apperrors.WithActions("dws auth login"),
		)
	}

	// Clear the token cache so the new token is loaded
	ResetRuntimeTokenCache()

	// Retry the invocation
	fmt.Fprintln(output)
	fmt.Fprintf(output, "%s %s\n", color.New(color.FgGreen).SprintFunc()("▶"),
		color.New(color.Bold).SprintFunc()("授权完成，正在重试..."))
	fmt.Fprintln(output)

	return runner.Run(ctx, invocation)
}

// ---- handlePatAuthCheck (runner.go entry point) -----------------------------

const (
	// patPollInterval is how often we poll the device flow status endpoint.
	patPollInterval = 2 * time.Second
	// patPollTimeout is the maximum time to wait for user authorization via device flow.
	patPollTimeout = 10 * time.Minute
)

// patRetryingKey is a context key to prevent recursive PAT auth checks.
// After APPROVED, the retry should not trigger another PAT flow.
type patRetryingKeyType struct{}

var patRetryingKey = patRetryingKeyType{}

// IsPatRetrying returns true if the current context is already in a PAT retry.
func IsPatRetrying(ctx context.Context) bool {
	v, _ := ctx.Value(patRetryingKey).(bool)
	return v
}

// handlePatAuthCheck is called by runner.executeInvocation when a PAT
// authorization error is detected.  It injects the server-assigned clientId
// as x-robot-uid header, prints authorization details, opens the browser,
// polls the device flow endpoint until the user authorizes, and retries the
// original invocation on success.
func handlePatAuthCheck(
	ctx context.Context,
	r *runtimeRunner,
	invocation executor.Invocation,
	patErr *apperrors.PATError,
	configDir string,
	output io.Writer,
) (executor.Result, error) {
	// Parse authorization details from PATError.RawJSON.
	var patData struct {
		Code string `json:"code"`
		Data struct {
			Desc         string `json:"desc"`
			FlowID       string `json:"flowId"`
			URI          string `json:"uri"`
			ClientID     string `json:"clientId"`
			ClientSecret string `json:"clientSecret"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(patErr.RawJSON), &patData); err != nil {
		return executor.Result{}, patErr
	}

	slog.Debug("PAT auth check",
		"clientId", patData.Data.ClientID,
		"flowId", patData.Data.FlowID,
		"hasSecret", patData.Data.ClientSecret != "",
	)
	hostOwnedPAT := authpkg.HostOwnsPATFlow()
	slog.Debug("pat.host_owned_decision",
		"site", "handlePatAuthCheck",
		"hostOwned", hostOwnedPAT,
		"agentCodeEnvSet", os.Getenv(authpkg.AgentCodeEnv) != "",
	)

	// Inject clientId/clientSecret from PAT response as runtime credentials
	// so that subsequent device flow auth uses the server-assigned app identity.
	var appCfg *authpkg.AppConfig
	if patData.Data.ClientID != "" {
		if patData.Data.ClientSecret != "" {
			// When both clientId and clientSecret are provided, use direct mode
			// (DingTalk API) rather than MCP proxy — the MCP proxy does not hold
			// the secret for this particular app.
			authpkg.SetClientID(patData.Data.ClientID)
			authpkg.SetClientSecret(patData.Data.ClientSecret)
		} else {
			// No clientSecret — rely on MCP proxy to manage the secret server-side.
			authpkg.SetClientIDFromMCP(patData.Data.ClientID)
		}

		// Persist clientId (and optionally secret) to ~/.dws/app.json so that
		// future process invocations can load it at startup and populate
		// DWS_CLIENT_ID env before the first MCP request.
		appCfg = &authpkg.AppConfig{
			ClientID: patData.Data.ClientID,
		}
		if patData.Data.ClientSecret != "" {
			appCfg.ClientSecret = authpkg.PlainSecret(patData.Data.ClientSecret)
		}
	}

	if appCfg != nil {
		if err := authpkg.SaveAppConfig(configDir, appCfg); err != nil {
			slog.Warn("failed to persist app config from PAT", "error", err)
			if !hostOwnedPAT && patData.Data.FlowID != "" {
				fmt.Fprintf(output, "  \u26a0 保存应用配置失败: %v (下次启动可能需要重新授权)\n", err)
			}
		}
	}

	// In host-controlled PAT mode (driven solely by DINGTALK_DWS_AGENTCODE),
	// or when flowId is absent, the CLI returns machine-readable JSON to
	// stderr and leaves UI/polling/retry to the host. `claw-type` is NOT
	// used for this decision — it is only forwarded on the wire via
	// edition.MergeHeaders and surfaced in hostControl for traceability.
	if hostOwnedPAT || patData.Data.FlowID == "" {
		if hostOwnedPAT {
			return executor.Result{}, &apperrors.PATError{RawJSON: enrichPATErrorForHostControl(patErr.RawJSON, patData.Data.FlowID)}
		}
		return executor.Result{}, patErr
	}

	bold := color.New(color.Bold).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	greenFn := color.New(color.FgGreen).SprintFunc()
	yellowFn := color.New(color.FgYellow).SprintFunc()
	redFn := color.New(color.FgRed).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	fmt.Fprintln(output)
	fmt.Fprintf(output, "%s %s\n", greenFn("▶"), bold("需要 PAT 授权"))
	if patData.Data.Desc != "" {
		fmt.Fprintf(output, "  %s %s\n", dim("ℹ"), patData.Data.Desc)
	}
	if patData.Data.URI != "" {
		fmt.Fprintf(output, "  %s %s\n\n", dim("🔗"), cyan(patData.Data.URI))
		// Best-effort browser open.
		_ = tryOpenBrowser(patData.Data.URI)
	}

	// Poll the device flow status until user authorizes, rejects, or timeout.
	fmt.Fprintf(output, "%s %s\n", yellowFn("⏳"), bold("等待用户授权..."))
	fmt.Fprintf(output, "  %s 请在浏览器中完成授权，超时时间: %s\n", dim("ℹ"), patPollTimeout)
	fmt.Fprintln(output)

	pollCtx, cancel := context.WithTimeout(ctx, patPollTimeout)
	defer cancel()

	status, authCode, err := pollPatDeviceFlow(pollCtx, patData.Data.FlowID, configDir, output)
	if err != nil {
		fmt.Fprintf(output, "%s 轮询授权状态失败: %v\n", redFn("✗"), err)
		return executor.Result{}, patErr
	}

	switch status {
	case authpkg.StatusApproved:
		fmt.Fprintf(output, "%s %s\n", greenFn("✓"), bold("授权成功!"))
		fmt.Fprintln(output)

		// Exchange authCode for a fresh access token (mirrors device_flow loginOnce).
		if authCode != "" {
			slog.Debug("PAT retry: exchanging authCode for token", "hasCode", true)
			tokenData, exchErr := authpkg.ExchangeCodeForToken(ctx, configDir, authCode)
			if exchErr != nil {
				slog.Warn("PAT retry: exchangeCode failed, retrying with existing token", "error", exchErr)
				fmt.Fprintf(output, "  %s 换取新 token 失败: %v (将使用现有凭证重试)\n", yellowFn("⚠"), exchErr)
			} else {
				if err := authpkg.SaveTokenData(configDir, tokenData); err != nil {
					slog.Warn("PAT retry: failed to save new token", "error", err)
					fmt.Fprintf(output, "  %s 保存新 token 失败: %v\n", yellowFn("⚠"), err)
				} else {
					slog.Debug("PAT retry: token refreshed and saved")
				}
			}
		}

		// Clear token cache so the new credentials take effect.
		ResetRuntimeTokenCache()

		// Workaround: brief delay to let server-side authorization state propagate
		// before retrying.  Without this the retry may use stale credentials.
		slog.Debug("PAT retry: waiting for server-side state propagation", "delay", "1s")
		time.Sleep(1 * time.Second)

		// Retry the original invocation with pat-retrying flag to prevent recursion.
		fmt.Fprintf(output, "%s %s\n", greenFn("▶"), bold("授权完成，正在重试..."))
		fmt.Fprintln(output)
		slog.Debug("PAT retry: identity env check",
			"DWS_CLIENT_ID", os.Getenv("DWS_CLIENT_ID"),
		)
		retryCtx := context.WithValue(ctx, patRetryingKey, true)
		return r.Run(retryCtx, invocation)

	case authpkg.StatusRejected:
		fmt.Fprintf(output, "%s %s\n", redFn("✗"), bold("用户已拒绝授权"))
		return executor.Result{}, apperrors.NewAuth(
			"用户已拒绝授权",
			apperrors.WithReason("pat_auth_rejected"),
			apperrors.WithHint("用户在浏览器中拒绝了授权请求，请重新执行命令。"),
		)

	case authpkg.StatusExpired:
		fmt.Fprintf(output, "%s %s\n", redFn("✗"), bold("授权超时"))
		return executor.Result{}, apperrors.NewAuth(
			"授权超时",
			apperrors.WithReason("pat_auth_expired"),
			apperrors.WithHint("授权链接已过期，请重新执行命令。"),
		)

	case authpkg.StatusCancelled:
		fmt.Fprintf(output, "%s %s\n", redFn("✗"), bold("操作已取消"))
		return executor.Result{}, apperrors.NewAuth(
			"操作已取消",
			apperrors.WithReason("pat_auth_cancelled"),
			apperrors.WithHint("用户取消了授权操作。"),
		)

	default:
		fmt.Fprintf(output, "%s 未知授权状态: %s\n", redFn("✗"), status)
		return executor.Result{}, patErr
	}
}

func enrichPATErrorForHostControl(raw string, flowID string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}

	data, _ := payload["data"].(map[string]any)
	if data == nil {
		data = make(map[string]any)
		payload["data"] = data
	}
	data["hostControl"] = buildHostControlState()
	delete(data, "callbacks")

	// docs/pat/contract.md §2: stderr JSON is single-line.
	encoded, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return string(encoded)
}

func buildPATScopeHostJSON(scopeErr *PatScopeError) string {
	payload := map[string]any{
		"success": false,
		"code":    patScopeAuthRequiredCode,
		"data": map[string]any{
			"identity":     scopeErr.Identity,
			"errorType":    scopeErr.ErrorType,
			"message":      scopeErr.Message,
			"hint":         scopeErr.Hint,
			"missingScope": scopeErr.MissingScope,
			"hostControl":  buildHostControlState(),
		},
	}
	// docs/pat/contract.md §2: stderr JSON is single-line.
	b, err := json.Marshal(payload)
	if err != nil {
		return `{"success":false,"code":"PAT_SCOPE_AUTH_REQUIRED"}`
	}
	return string(b)
}

// buildHostControlState returns the host-control block used by the active
// retry / scope-auth paths. Core fields come from apperrors.HostControlBlock
// so the classifier-side injection and this call-site agree byte-for-byte;
// callbackOwner is a legacy key preserved for backwards compatibility with
// existing hosts and is layered on top here.
//
// NOTE: this function is only called from host-owned paths (caller has
// already confirmed HostOwnsPATFlow()), so HostControlBlock() is expected
// to be non-nil. The defensive fallback below reuses the same provider
// the classifier wires in init() to avoid drift.
func buildHostControlState() map[string]any {
	block := apperrors.HostControlBlock()
	if block == nil {
		claw := hostControlProviderFromEnv()
		block = map[string]any{
			"clawType":     claw,
			"mode":         "host",
			"pollingOwner": "host",
			"retryOwner":   "host",
		}
	}
	block["callbackOwner"] = "host"
	return block
}

// pollPatDeviceFlow polls the PAT device flow status endpoint until a terminal
// state (APPROVED/REJECTED/EXPIRED) is reached or the context is cancelled.
// Returns the final status string and the authCode (non-empty only on APPROVED).
func pollPatDeviceFlow(ctx context.Context, flowID string, configDir string, output io.Writer) (string, string, error) {
	pollURL := fmt.Sprintf("%s%s?flowId=%s",
		authpkg.GetMCPBaseURL(), authpkg.DevicePollPath, url.QueryEscape(flowID))

	// Load user access token for the poll request header.
	var accessToken string
	if tokenData, err := authpkg.LoadTokenData(configDir); err == nil && tokenData != nil {
		accessToken = tokenData.AccessToken
	}

	// Use a client that does NOT follow redirects, so we can detect SSO 302.
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	ticker := time.NewTicker(patPollInterval)
	defer ticker.Stop()

	dim := color.New(color.Faint).SprintFunc()
	pollCount := 0

	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				return authpkg.StatusCancelled, "", nil
			}
			return authpkg.StatusExpired, "", nil
		case <-ticker.C:
			pollCount++
			fmt.Fprintf(output, "\r%s [%d] 等待授权中...          ", dim("⟳"), pollCount)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
			if err != nil {
				slog.Debug("PAT poll: failed to create request", "error", err)
				continue
			}
			if accessToken != "" {
				req.Header.Set("x-user-access-token", accessToken)
			}
			resp, err := noRedirectClient.Do(req)
			if err != nil {
				slog.Debug("PAT poll: request failed", "error", err)
				continue // transient network error, keep polling
			}

			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			// If we got a redirect (302/301), SSO gateway intercepted — skip JSON parse.
			if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
				continue
			}

			var pollResp authpkg.DevicePollResponse
			if err := json.Unmarshal(bodyBytes, &pollResp); err != nil {
				slog.Debug("PAT poll: failed to parse response", "error", err, "body", string(bodyBytes))
				continue
			}

			status := authpkg.ParseDeviceFlowStatus(pollResp.Data.Status, pollResp.Success)
			switch status {
			case authpkg.StatusApproved:
				fmt.Fprintln(output) // clear the polling line
				return status, pollResp.Data.AuthCode, nil
			case authpkg.StatusRejected, authpkg.StatusExpired:
				fmt.Fprintln(output) // clear the polling line
				return status, "", nil
			case authpkg.StatusPending:
				// keep polling
			default:
				// ParseDeviceFlowStatus normalizes empty+!success to EXPIRED,
				// so this branch handles truly unknown statuses.
				fmt.Fprintln(output)
				return status, "", nil
			}
		}
	}
}

// tryOpenBrowser opens url in the default browser; errors are silently ignored.
func tryOpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return nil
	}
	return cmd.Start()
}
