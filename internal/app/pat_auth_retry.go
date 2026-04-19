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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/fatih/color"
)

const (
	// PatAuthRetryTimeout is the maximum time to wait for user authorization
	// when a PAT scope error is detected.
	PatAuthRetryTimeout = 10 * time.Minute

	// PatAuthPollInterval is how often we poll to check if the user has
	// completed authorization.
	PatAuthPollInterval = 5 * time.Second
)

// PatScopeError holds information about a missing PAT scope.
type PatScopeError struct {
	OriginalError string
	Identity      string
	ErrorType     string
	Message       string
	Hint          string
	MissingScope  string
	VerifyURL     string
	UserCode      string
}

func (e *PatScopeError) Error() string {
	return e.OriginalError
}

// patScopeRegex matches common PAT scope error patterns from the API.
var patScopeRegex = regexp.MustCompile(`(?i)(missing_scope|insufficient_scope|scope.*required|permission.*denied|forbidden)`)

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
			if strings.Contains(fullText, "scope") || strings.Contains(fullText, "permission") ||
				strings.Contains(fullText, "forbidden") || strings.Contains(fullText, "missing") {
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

	// Try to extract scope from error message
	scopeMatch := regexp.MustCompile(`(?i)scope[=: "]*([a-zA-Z0-9_:.]+)`).FindStringSubmatch(msg)
	if len(scopeMatch) > 1 {
		scope = scopeMatch[1]
	}

	// Try to extract identity from error message
	identity := "user"
	identityMatch := regexp.MustCompile(`(?i)identity["\s:]+([a-zA-Z_]+)`).FindStringSubmatch(msg)
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
	payload := map[string]any{
		"ok":       false,
		"identity": scopeErr.Identity,
		"error": map[string]any{
			"type":    scopeErr.ErrorType,
			"message": scopeErr.Message,
			"hint":    scopeErr.Hint,
		},
	}
	if scopeErr.MissingScope != "" {
		payload["missing_scope"] = scopeErr.MissingScope
	}
	if scopeErr.VerifyURL != "" {
		payload["verification_url"] = scopeErr.VerifyURL
	}

	data, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Fprintln(w, string(data))
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

// loadMCPClientIDIfNeeded ensures we have a client ID for device flow.
// Priority: in-memory runtime value → DWS_CLIENT_ID env → MCP remote fetch.
func loadMCPClientIDIfNeeded(ctx context.Context, configDir string) string {
	clientID := authpkg.ClientID()
	if clientID != "" {
		return clientID
	}
	// Fallback: read from environment variable (set by previous PAT auth or caller).
	if envID := os.Getenv("DWS_CLIENT_ID"); envID != "" {
		authpkg.SetClientIDFromMCP(envID)
		return envID
	}
	// Last resort: fetch from MCP server.
	mcpClientID, err := authpkg.FetchClientIDFromMCP(ctx)
	if err == nil && mcpClientID != "" {
		authpkg.SetClientIDFromMCP(mcpClientID)
		return mcpClientID
	}
	return ""
}

// TriggerPatDeviceFlow initiates a device authorization flow for the missing scope.
func TriggerPatDeviceFlow(ctx context.Context, configDir, missingScope string, output io.Writer) error {
	bold := color.New(color.Bold).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	// Ensure we have a client ID
	clientID := loadMCPClientIDIfNeeded(ctx, configDir)
	if clientID == "" {
		return fmt.Errorf("无法获取 Client ID，请先运行 dws auth login")
	}

	fmt.Fprintln(output)
	fmt.Fprintf(output, "%s %s\n", green("▶"), bold("正在发起设备授权..."))
	fmt.Fprintln(output)

	// Create device flow provider with the missing scope
	provider := authpkg.NewDeviceFlowProvider(configDir, nil)
	provider.Output = output

	// Set the scope to the missing one
	if missingScope != "" {
		provider.SetScope(missingScope)
	}

	// Run device flow with timeout
	flowCtx, cancel := context.WithTimeout(ctx, config.DeviceFlowTimeout)
	defer cancel()

	tokenData, err := provider.Login(flowCtx)
	if err != nil {
		return err
	}

	fmt.Fprintln(output)
	fmt.Fprintf(output, "%s %s\n", green("✓"), bold("授权成功!"))
	if tokenData != nil && tokenData.CorpName != "" {
		fmt.Fprintf(output, "%-16s%s\n", "企业:", tokenData.CorpName)
	}
	if tokenData != nil && tokenData.UserName != "" {
		fmt.Fprintf(output, "%-16s%s\n", "用户:", tokenData.UserName)
	}
	fmt.Fprintln(output)

	// Clear token cache so new token is used
	ResetRuntimeTokenCache()

	return nil
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

	// Inject clientId/clientSecret from PAT response as runtime credentials
	// so that subsequent device flow auth uses the server-assigned app identity.
	if patData.Data.ClientID != "" {
		authpkg.SetClientIDFromMCP(patData.Data.ClientID)
		_ = os.Setenv("DWS_CLIENT_ID", patData.Data.ClientID)
		if patData.Data.ClientSecret != "" {
			authpkg.SetClientSecret(patData.Data.ClientSecret)
			_ = os.Setenv("DWS_CLIENT_SECRET", patData.Data.ClientSecret)
		}

		// Persist clientId (and optionally secret) to ~/.dws/app.json so that
		// future process invocations can load it at startup and populate
		// DWS_CLIENT_ID env before the first MCP request.
		appCfg := &authpkg.AppConfig{
			ClientID: patData.Data.ClientID,
		}
		if patData.Data.ClientSecret != "" {
			appCfg.ClientSecret = authpkg.PlainSecret(patData.Data.ClientSecret)
		}
		if err := authpkg.SaveAppConfig(configDir, appCfg); err != nil {
			slog.Debug("failed to persist app config from PAT", "error", err)
		}
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

	// If no flowId, we can't poll — fall back to returning PATError for host-app.
	if patData.Data.FlowID == "" {
		fmt.Fprintln(output)
		return executor.Result{}, patErr
	}

	// Poll the device flow status until user authorizes, rejects, or timeout.
	fmt.Fprintf(output, "%s %s\n", yellowFn("⏳"), bold("等待用户授权..."))
	fmt.Fprintf(output, "  %s 请在浏览器中完成授权，超时时间: %s\n", dim("ℹ"), patPollTimeout)
	fmt.Fprintln(output)

	pollCtx, cancel := context.WithTimeout(ctx, patPollTimeout)
	defer cancel()

	status, err := pollPatDeviceFlow(pollCtx, patData.Data.FlowID, configDir, output)
	if err != nil {
		fmt.Fprintf(output, "%s 轮询授权状态失败: %v\n", redFn("✗"), err)
		return executor.Result{}, patErr
	}

	switch status {
	case "APPROVED":
		fmt.Fprintf(output, "%s %s\n", greenFn("✓"), bold("授权成功!"))
		fmt.Fprintln(output)

		// Clear token cache so the new credentials take effect.
		ResetRuntimeTokenCache()

		// Brief delay to let server-side authorization state propagate.
		time.Sleep(1 * time.Second)

		// Retry the original invocation with pat-retrying flag to prevent recursion.
		fmt.Fprintf(output, "%s %s\n", greenFn("▶"), bold("授权完成，正在重试..."))
		fmt.Fprintln(output)
		slog.Debug("PAT retry: identity env check",
			"DWS_CLIENT_ID", os.Getenv("DWS_CLIENT_ID"),
		)
		retryCtx := context.WithValue(ctx, patRetryingKey, true)
		return r.Run(retryCtx, invocation)

	case "REJECTED":
		fmt.Fprintf(output, "%s %s\n", redFn("✗"), bold("用户已拒绝授权"))
		return executor.Result{}, apperrors.NewAuth(
			"用户已拒绝授权",
			apperrors.WithReason("pat_auth_rejected"),
			apperrors.WithHint("用户在浏览器中拒绝了授权请求，请重新执行命令。"),
		)

	case "EXPIRED":
		fmt.Fprintf(output, "%s %s\n", redFn("✗"), bold("授权超时"))
		return executor.Result{}, apperrors.NewAuth(
			"授权超时",
			apperrors.WithReason("pat_auth_expired"),
			apperrors.WithHint("授权链接已过期，请重新执行命令。"),
		)

	default:
		fmt.Fprintf(output, "%s 未知授权状态: %s\n", redFn("✗"), status)
		return executor.Result{}, patErr
	}
}

// pollPatDeviceFlow polls the PAT device flow status endpoint until a terminal
// state (APPROVED/REJECTED/EXPIRED) is reached or the context is cancelled.
// Returns the final status string.
func pollPatDeviceFlow(ctx context.Context, flowID string, configDir string, output io.Writer) (string, error) {
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
			return "EXPIRED", nil
		case <-ticker.C:
			pollCount++
			fmt.Fprintf(output, "\r%s [%d] 等待授权中...          ", dim("⟳"), pollCount)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
			if err != nil {
				continue
			}
			if accessToken != "" {
				req.Header.Set("x-user-access-token", accessToken)
			}
			resp, err := noRedirectClient.Do(req)
			if err != nil {
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
				continue
			}

			status := pollResp.Data.Status
			switch status {
			case "APPROVED", "REJECTED", "EXPIRED":
				fmt.Fprintln(output) // clear the polling line
				return status, nil
			case "PENDING":
				// keep polling
			default:
				if status == "" && !pollResp.Success {
					// Server error or flow not found — treat as expired
					fmt.Fprintln(output)
					return "EXPIRED", nil
				}
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
