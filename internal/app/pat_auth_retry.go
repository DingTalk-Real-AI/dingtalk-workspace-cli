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
	"bytes"
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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/tui"
)

const (
	// PatAuthRetryTimeout is the maximum time to wait for user authorization
	// when a PAT scope error is detected.
	PatAuthRetryTimeout = 10 * time.Minute

	// PatAuthPollInterval is how often we poll to check if the user has
	// completed authorization.
	PatAuthPollInterval = 5 * time.Second

	patScopeAuthRequiredCode = "PAT_SCOPE_AUTH_REQUIRED"
	patOrgPolicyDeniedCode   = "PAT_ORG_POLICY_DENIED"
)

var openBrowserFunc = tryOpenBrowser

var (
	patAuthorizationTimeout       = PatAuthRetryTimeout
	patAuthorizationPollInterval  = PatAuthPollInterval
	patLoadTokenData              = authpkg.LoadTokenData
	patWaitForAuthorization       = WaitForPatAuthorization
	patPollDeviceFlowWithInterval = pollPatDeviceFlowWithInterval
	patSaveAppConfig              = authpkg.SaveAppConfig
	patExchangeCodeForToken       = authpkg.ExchangeCodeForToken
	patSaveTokenData              = authpkg.SaveTokenData
	patSleep                      = time.Sleep
	patPollHTTPDo                 = (*http.Client).Do
	patPollNewRequest             = http.NewRequestWithContext
	patBrowserOpenCommand         = browserOpenCommand
)

type patSuppressBrowserOpenKeyType struct{}

var patSuppressBrowserOpenKey = patSuppressBrowserOpenKeyType{}

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
	fmt.Fprintln(w)
	fmt.Fprintf(w, "{\n")
	fmt.Fprintf(w, "  %s: %s,\n", tui.Bold("\"ok\""), "false")
	fmt.Fprintf(w, "  %s: %q,\n", tui.Bold("\"identity\""), scopeErr.Identity)
	fmt.Fprintf(w, "  %s: {\n", tui.Bold("\"error\""))
	fmt.Fprintf(w, "    %s: %q,\n", tui.Bold("\"type\""), scopeErr.ErrorType)
	fmt.Fprintf(w, "    %s: %q,\n", tui.Bold("\"message\""), scopeErr.Message)
	fmt.Fprintf(w, "    %s: %q\n", tui.Bold("\"hint\""), scopeErr.Hint)
	fmt.Fprintf(w, "  }\n")
	fmt.Fprintf(w, "}\n")
	fmt.Fprintln(w)

	// Print authorization instructions
	fmt.Fprintf(w, "%s %s\n", tui.StateMark("warning"), tui.Bold("需要额外授权"))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", tui.Dim("#"), tui.Dim("运行以下命令完成授权"))

	if scopeErr.MissingScope != "" {
		fmt.Fprintf(w, "  %s %s\n", tui.Cyan("$"), tui.Cyan(fmt.Sprintf("dws auth login --scope %q", scopeErr.MissingScope)))
	} else {
		fmt.Fprintf(w, "  %s %s\n", tui.Cyan("$"), tui.Cyan("dws auth login"))
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s 在浏览器中打开授权链接，完成授权后重新执行命令\n", tui.Dim("ℹ"))
	fmt.Fprintln(w)
}

// PrintPatAuthJSON prints a machine-readable PAT authorization error.
func PrintPatAuthJSON(w io.Writer, scopeErr *PatScopeError) {
	fmt.Fprintln(w, buildPATScopeJSON(scopeErr, authpkg.HostOwnsPATFlow()))
}

func wantsStructuredPATOutput(r *runtimeRunner) bool {
	if r == nil || r.globalFlags == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.globalFlags.Format), "json")
}

func wantsStructuredPATOutputFromRunner(runner executor.Runner) bool {
	rr, ok := runner.(*runtimeRunner)
	if !ok {
		return false
	}
	return wantsStructuredPATOutput(rr)
}

func currentPATOpenBrowser(ctx context.Context, configDir string) bool {
	if suppressed, _ := ctx.Value(patSuppressBrowserOpenKey).(bool); suppressed {
		return false
	}
	return pat.EffectiveOpenBrowser(configDir)
}

func enrichPATErrorWithOpenBrowser(raw string, openBrowser bool) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}

	data, ok := payload["data"].(map[string]any)
	if !ok || data == nil {
		data = map[string]any{}
		payload["data"] = data
	}
	if rawURI := patAuthorizationURIFromData(data); rawURI != "" {
		authURL := apperrors.PATAuthorizationURL(rawURI)
		data["uri"] = authURL
		delete(data, "authUrl")
		delete(data, "authorizationUrl")
	}
	if code, _ := payload["code"].(string); code == "PAT_ORG_POLICY_DENIED" {
		data["openBrowser"] = false
	} else {
		data["openBrowser"] = openBrowser
	}

	encoded, _ := marshalSingleLineJSONNoHTMLEscape(payload)
	return string(encoded)
}

func patAuthorizationURIFromData(data map[string]any) string {
	for _, key := range []string{"uri", "authUrl", "authorizationUrl"} {
		value, _ := data[key].(string)
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// WaitForPatAuthorization polls until the user completes authorization or timeout.
// It returns true if authorization was completed, false if timed out or cancelled.
func WaitForPatAuthorization(ctx context.Context, configDir string, output io.Writer) bool {
	timeout := patAuthorizationTimeout
	deadline := time.Now().Add(timeout)
	pollTicker := time.NewTicker(patAuthorizationPollInterval)
	defer pollTicker.Stop()
	start := time.Now()

	fmt.Fprintln(output)
	fmt.Fprintf(output, "%s %s\n", tui.StateMark("pending"), tui.Bold("等待用户授权..."))
	fmt.Fprintf(output, "  %s 请在另一个终端完成 dws auth login 授权\n", tui.Dim("ℹ"))
	fmt.Fprintf(output, "  %s 超时时间: %s\n", tui.Dim("⏱"), timeout)
	fmt.Fprintln(output)

	pollCount := 0
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(output, "%s 操作已取消\n", tui.StateMark("error"))
			return false

		case <-time.After(time.Until(deadline)):
			fmt.Fprintf(output, "%s 等待授权超时 (%s)\n", tui.StateMark("error"), timeout)
			fmt.Fprintf(output, "  %s 请重新执行命令\n", tui.Dim("ℹ"))
			return false

		case <-pollTicker.C:
			pollCount++
			elapsed := time.Since(start).Truncate(time.Second)
			remaining := time.Until(deadline).Truncate(time.Second)

			// Check if token is now valid
			tokenData, err := patLoadTokenData(configDir)
			if err == nil && tokenData != nil {
				if tokenData.IsAccessTokenValid() || tokenData.IsRefreshTokenValid() {
					fmt.Fprintf(output, "\r%s %s (%s 已用, %s 剩余)          \n",
						tui.StateMark("ok"), tui.Bold("授权成功!"), elapsed, remaining)
					fmt.Fprintln(output)
					return true
				}
			}

			// Show polling status
			fmt.Fprintf(output, "\r%s [%d] 等待授权中... (%s 已用, %s 剩余)          ",
				tui.Dim("⟳"), pollCount, elapsed, remaining)
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
		return executor.Result{}, &apperrors.PATError{RawJSON: buildPATScopeJSON(scopeErr, true)}
	}
	if wantsStructuredPATOutputFromRunner(runner) {
		return executor.Result{}, &apperrors.PATError{RawJSON: buildPATScopeJSON(scopeErr, false)}
	}

	// Print the PAT error in human-readable format
	PrintPatAuthError(output, scopeErr)

	// Wait for user to complete authorization
	authorized := patWaitForAuthorization(ctx, configDir, output)
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
	fmt.Fprintf(output, "%s %s\n", tui.StateMark("ok"), tui.Bold("授权完成，正在重试..."))
	fmt.Fprintln(output)

	return runner.Run(ctx, invocation)
}

// ---- handlePatAuthCheck (runner.go entry point) -----------------------------

const (
	// patPollInterval is how often we poll the device flow status endpoint.
	patPollInterval = 2 * time.Second
	// patMaxPollInterval caps a server-provided poll interval so a malformed
	// response cannot make the CLI look permanently stuck.
	patMaxPollInterval = 30 * time.Second
	// patPollTimeout is the maximum time to wait for user authorization via device flow.
	patPollTimeout = 10 * time.Minute
)

// patRetryingKey is a context key to prevent recursive PAT auth checks.
// After APPROVED, the retry should not trigger another PAT flow.
type patRetryingKeyType struct{}

var patRetryingKey = patRetryingKeyType{}

type patRetryRunnerFunc func(context.Context, executor.Invocation) (executor.Result, error)

func (f patRetryRunnerFunc) Run(ctx context.Context, invocation executor.Invocation) (executor.Result, error) {
	return f(ctx, invocation)
}

// IsPatRetrying returns true if the current context is already in a PAT retry.
func IsPatRetrying(ctx context.Context) bool {
	v, _ := ctx.Value(patRetryingKey).(bool)
	return v
}

func openPATAuthorizationURI(rawURI string) error {
	if rawURI == "" {
		// Defensive guard for future callers. The current call site already
		// checks for a non-empty PAT URI before invoking this helper.
		return nil
	}
	// The PAT service returns the complete authorization URL. Treat it as an
	// opaque string unless it is the known legacy DingTalk hash-route variant.
	// That variant is normalized by the PAT error contract helper before being
	// printed, opened, or returned in structured output.
	return openBrowserFunc(apperrors.PATAuthorizationURL(rawURI))
}

func printPATPollDebugResponse(output io.Writer, statusCode int, body []byte) {
	if os.Getenv("DWS_DEBUG_PAT_POLL") == "" {
		return
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		trimmed = "<empty body>"
	}
	fmt.Fprintln(output)
	fmt.Fprintf(output, "  ℹ PAT 轮询接口返回原文 (HTTP %d):\n", statusCode)
	fmt.Fprintf(output, "    %s\n", trimmed)
}

func runDirectPATAuthCheck(
	ctx context.Context,
	globalFlags *GlobalFlags,
	patErr *apperrors.PATError,
	retry func(context.Context) error,
	output io.Writer,
) error {
	if retry == nil {
		return patErr
	}
	return runDirectPATAuthCheckWithMode(ctx, globalFlags, patErr, retry, output, true)
}

func runDirectPATAuthCheckWaitOnly(
	ctx context.Context,
	globalFlags *GlobalFlags,
	patErr *apperrors.PATError,
	output io.Writer,
) error {
	ctx = context.WithValue(ctx, patSuppressBrowserOpenKey, true)
	return runDirectPATAuthCheckWithMode(ctx, globalFlags, patErr, nil, output, false)
}

func runDirectPATAuthCheckWithMode(
	ctx context.Context,
	globalFlags *GlobalFlags,
	patErr *apperrors.PATError,
	retry func(context.Context) error,
	output io.Writer,
	retryAfterApproval bool,
) error {
	if retryAfterApproval && retry == nil {
		return patErr
	}
	runner := &runtimeRunner{
		globalFlags: globalFlags,
		fallback: patRetryRunnerFunc(func(retryCtx context.Context, invocation executor.Invocation) (executor.Result, error) {
			if retry != nil {
				if err := retry(retryCtx); err != nil {
					return executor.Result{}, err
				}
			}
			invocation.Implemented = true
			return executor.Result{
				Invocation: invocation,
				Response: map[string]any{
					"ok": true,
				},
			}, nil
		}),
	}
	_, err := handlePatAuthCheck(ctx, runner, executor.Invocation{
		Kind:             "direct_pat_authorization",
		Stage:            "auth_login_recommend",
		CanonicalProduct: defaultPATProductID,
		Tool:             "pat.batch_grant",
		CanonicalPath:    "pat.batch_grant",
		Params: map[string]any{
			"retryAfterApproval": retryAfterApproval,
		},
	}, patErr, defaultConfigDir(), output)
	return err
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
			Desc             string `json:"desc"`
			FlowID           string `json:"flowId"`
			URI              string `json:"uri"`
			AuthURL          string `json:"authUrl"`
			AuthorizationURL string `json:"authorizationUrl"`
			ClientID         string `json:"clientId"`
			ClientSecret     string `json:"clientSecret"`
			PollIntervalSecs int    `json:"pollIntervalSeconds"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(patErr.RawJSON), &patData); err != nil {
		return executor.Result{}, patErr
	}
	if patData.Data.URI == "" {
		patData.Data.URI = patData.Data.AuthURL
	}
	if patData.Data.URI == "" {
		patData.Data.URI = patData.Data.AuthorizationURL
	}
	// Organization-policy denial is terminal until an administrator changes
	// the policy. Return it before reading browser policy, mutating process-wide
	// credentials, opening a browser, polling, or retrying the invocation even
	// when a lenient backend also supplies active-flow fields.
	if patData.Code == patOrgPolicyDeniedCode {
		return executor.Result{}, &apperrors.PATError{
			RawJSON: enrichPATErrorWithOpenBrowser(patErr.RawJSON, false),
		}
	}

	slog.Debug("PAT auth check",
		"clientId", patData.Data.ClientID,
		"flowId", patData.Data.FlowID,
		"hasSecret", patData.Data.ClientSecret != "",
	)
	hostOwnedPAT := authpkg.HostOwnsPATFlow()
	openBrowser := currentPATOpenBrowser(ctx, configDir)
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

		// Persist only after an explicit APPROVED result below. Raw PAT
		// interceptions (host-owned / json / empty-flow pass-through) must not
		// rewrite the shared ~/.dws/app.json state for unrelated shells or agents.
		appCfg = &authpkg.AppConfig{ClientID: patData.Data.ClientID}
		if patData.Data.ClientSecret != "" {
			appCfg.ClientSecret = authpkg.PlainSecret(patData.Data.ClientSecret)
		}
	}

	// In host-controlled PAT mode (driven solely by DINGTALK_DWS_AGENTCODE),
	// or when flowId is absent, the CLI returns machine-readable JSON to
	// stderr and leaves UI/polling/retry to the host. `claw-type` is NOT
	// used for this decision — it is only forwarded on the wire via
	// edition.MergeHeaders and surfaced in hostControl for traceability.
	if hostOwnedPAT || patData.Data.FlowID == "" {
		if hostOwnedPAT {
			return executor.Result{}, &apperrors.PATError{RawJSON: enrichPATErrorForHostControl(patErr.RawJSON)}
		}
		return executor.Result{}, &apperrors.PATError{RawJSON: enrichPATErrorWithOpenBrowser(patErr.RawJSON, openBrowser)}
	}

	if wantsStructuredPATOutput(r) {
		if openBrowser && patData.Data.URI != "" {
			_ = openPATAuthorizationURI(patData.Data.URI)
		}
		return executor.Result{}, &apperrors.PATError{RawJSON: enrichPATErrorWithOpenBrowser(patErr.RawJSON, openBrowser)}
	}

	fmt.Fprintln(output)
	fmt.Fprintf(output, "%s %s\n", tui.StateMark("warning"), tui.Bold("需要 PAT 授权"))
	if patData.Data.Desc != "" {
		fmt.Fprintf(output, "  %s %s\n", tui.Dim("ℹ"), patData.Data.Desc)
	}
	if patData.Data.URI != "" {
		authURL := apperrors.PATAuthorizationURL(patData.Data.URI)
		fmt.Fprintf(output, "  %s 授权链接: %s\n", tui.Dim("🔗"), authURL)
		fmt.Fprintln(output)
		if openBrowser {
			_ = openPATAuthorizationURI(authURL)
		}
	}

	// Poll the device flow status until user authorizes, rejects, or timeout.
	fmt.Fprintf(output, "%s %s\n", tui.StateMark("pending"), tui.Bold("等待用户授权..."))
	fmt.Fprintf(output, "  %s 请在浏览器中完成授权，超时时间: %s\n", tui.Dim("ℹ"), patPollTimeout)
	fmt.Fprintln(output)

	pollCtx, cancel := context.WithTimeout(ctx, patPollTimeout)
	defer cancel()

	status, authCode, err := patPollDeviceFlowWithInterval(
		pollCtx, patData.Data.FlowID, configDir, output,
		resolvePATPollInterval(patData.Data.PollIntervalSecs),
	)
	if err != nil {
		fmt.Fprintf(output, "%s 轮询授权状态失败: %v\n", tui.StateMark("error"), err)
		return executor.Result{}, patErr
	}

	switch status {
	case authpkg.StatusApproved:
		fmt.Fprintf(output, "%s %s\n", tui.StateMark("ok"), tui.Bold("授权成功!"))
		fmt.Fprintln(output)

		if appCfg != nil {
			if err := patSaveAppConfig(configDir, appCfg); err != nil {
				slog.Warn("failed to persist approved app config from PAT", "error", err)
				fmt.Fprintf(output, "  \u26a0 保存应用配置失败: %v (下次启动可能需要重新授权)\n", err)
			}
		}

		// Exchange authCode for a fresh access token (mirrors device_flow loginOnce).
		if authCode != "" {
			slog.Debug("PAT retry: exchanging authCode for token", "hasCode", true)
			tokenData, exchErr := patExchangeCodeForToken(ctx, configDir, authCode)
			if exchErr != nil {
				slog.Warn("PAT retry: exchangeCode failed, retrying with existing token", "error", exchErr)
				fmt.Fprintf(output, "  %s 换取新 token 失败: %v (将使用现有凭证重试)\n", tui.StateMark("warning"), exchErr)
			} else {
				if err := patSaveTokenData(configDir, tokenData); err != nil {
					slog.Warn("PAT retry: failed to save new token", "error", err)
					fmt.Fprintf(output, "  %s 保存新 token 失败: %v\n", tui.StateMark("warning"), err)
				} else {
					slog.Debug("PAT retry: token refreshed and saved")
				}
			}
		}

		// Clear token cache so the new credentials take effect.
		ResetRuntimeTokenCache()

		if shouldSkipPATRetryAfterApproval(invocation) {
			invocation.Implemented = true
			return executor.Result{
				Invocation: invocation,
				Response: map[string]any{
					"ok": true,
				},
			}, nil
		}

		// Workaround: brief delay to let server-side authorization state propagate
		// before retrying.  Without this the retry may use stale credentials.
		slog.Debug("PAT retry: waiting for server-side state propagation", "delay", "1s")
		patSleep(1 * time.Second)

		// Retry the original invocation with pat-retrying flag to prevent recursion.
		fmt.Fprintf(output, "%s %s\n", tui.StateMark("ok"), tui.Bold("授权完成，正在重试..."))
		fmt.Fprintln(output)
		slog.Debug("PAT retry: identity env check",
			"DWS_CLIENT_ID", os.Getenv("DWS_CLIENT_ID"),
		)
		retryCtx := context.WithValue(ctx, patRetryingKey, true)
		return r.Run(retryCtx, invocation)

	case authpkg.StatusRejected:
		fmt.Fprintf(output, "%s %s\n", tui.StateMark("error"), tui.Bold("用户已拒绝授权"))
		return executor.Result{}, apperrors.NewAuth(
			"用户已拒绝授权",
			apperrors.WithReason("pat_auth_rejected"),
			apperrors.WithHint("用户在浏览器中拒绝了授权请求，请重新执行命令。"),
		)

	case authpkg.StatusExpired:
		fmt.Fprintf(output, "%s %s\n", tui.StateMark("error"), tui.Bold("授权超时"))
		return executor.Result{}, apperrors.NewAuth(
			"授权超时",
			apperrors.WithReason("pat_auth_expired"),
			apperrors.WithHint("授权链接已过期，请重新执行命令。"),
		)

	case authpkg.StatusCancelled:
		fmt.Fprintf(output, "%s %s\n", tui.StateMark("error"), tui.Bold("操作已取消"))
		return executor.Result{}, apperrors.NewAuth(
			"操作已取消",
			apperrors.WithReason("pat_auth_cancelled"),
			apperrors.WithHint("用户取消了授权操作。"),
		)

	default:
		fmt.Fprintf(output, "%s 未知授权状态: %s\n", tui.StateMark("error"), status)
		return executor.Result{}, patErr
	}
}

func shouldSkipPATRetryAfterApproval(invocation executor.Invocation) bool {
	if invocation.Params == nil {
		return false
	}
	value, ok := invocation.Params["retryAfterApproval"]
	if !ok {
		return false
	}
	retry, ok := value.(bool)
	return ok && !retry
}

func enrichPATErrorForHostControl(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}

	// Route back through the classifier so host-owned active retry emits the
	// exact same PAT JSON shape as passive classification.
	if patErr := apperrors.ClassifyPatAuthCheck(payload); patErr != nil {
		return patErr.RawJSON
	}

	apperrors.ApplyHostMutations(payload)

	// stderr JSON MUST be single-line.
	encoded, _ := marshalSingleLineJSONNoHTMLEscape(payload)
	return string(encoded)
}

// buildPATScopeJSON renders the PAT_SCOPE_AUTH_REQUIRED stderr payload.
// includeHostControl=true follows the standard host-owned/CLI-owned split
// (data.hostControl is injected only if HostControlBlock is non-nil).
// includeHostControl=false is an explicit override used by the CLI-owned
// branch so that any env-mode misconfiguration cannot leak a host-owned
// contract into stderr.
func buildPATScopeJSON(scopeErr *PatScopeError, includeHostControl bool) string {
	data := map[string]any{
		"identity":     scopeErr.Identity,
		"errorType":    scopeErr.ErrorType,
		"message":      scopeErr.Message,
		"hint":         scopeErr.Hint,
		"missingScope": scopeErr.MissingScope,
		"openBrowser":  apperrors.PATOpenBrowserValue(),
	}
	if includeHostControl {
		if hostControl := apperrors.HostControlBlock(); hostControl != nil {
			data["hostControl"] = hostControl
		}
	}

	payload := map[string]any{
		"success": false,
		"code":    patScopeAuthRequiredCode,
		"data":    data,
	}
	// stderr JSON MUST be single-line.
	b, _ := jsonutil.Marshal(payload)
	return string(b)
}

func marshalSingleLineJSONNoHTMLEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out, nil
}

// pollPatDeviceFlow polls the PAT device flow status endpoint until a terminal
// state (APPROVED/REJECTED/EXPIRED) is reached or the context is cancelled.
// Returns the final status string and the authCode (non-empty only on APPROVED).
func pollPatDeviceFlow(ctx context.Context, flowID string, configDir string, output io.Writer) (string, string, error) {
	return pollPatDeviceFlowWithInterval(ctx, flowID, configDir, output, patPollInterval)
}

func pollPatDeviceFlowWithInterval(ctx context.Context, flowID string, configDir string, output io.Writer, interval time.Duration) (string, string, error) {
	if interval <= 0 {
		interval = patPollInterval
	}
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

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

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
			fmt.Fprintf(output, "\r%s [%d] 等待授权中...          ", tui.Dim("⟳"), pollCount)

			req, err := patPollNewRequest(ctx, http.MethodGet, pollURL, nil)
			if err != nil {
				slog.Debug("PAT poll: failed to create request", "error", err)
				continue
			}
			if accessToken != "" {
				req.Header.Set("x-user-access-token", accessToken)
			}
			resp, err := patPollHTTPDo(noRedirectClient, req)
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
				printPATPollDebugResponse(output, resp.StatusCode, bodyBytes)
				continue
			}

			pollData := pollResp.EffectiveData()
			status := authpkg.ParseDeviceFlowStatus(pollData.Status, pollResp.Success)
			switch status {
			case authpkg.StatusApproved:
				fmt.Fprintln(output) // clear the polling line
				return status, pollData.AuthCode, nil
			case authpkg.StatusRejected, authpkg.StatusExpired:
				fmt.Fprintln(output) // clear the polling line
				return status, "", nil
			case authpkg.StatusPending:
			default:
				// ParseDeviceFlowStatus normalizes empty+!success to EXPIRED,
				// so this branch handles truly unknown statuses.
				fmt.Fprintln(output)
				printPATPollDebugResponse(output, resp.StatusCode, bodyBytes)
				return status, "", nil
			}
		}
	}
}

func resolvePATPollInterval(seconds int) time.Duration {
	if seconds <= 0 {
		return patPollInterval
	}
	interval := time.Duration(seconds) * time.Second
	if interval > patMaxPollInterval {
		return patMaxPollInterval
	}
	return interval
}

func browserOpenCommand(goos, rawURL string) *exec.Cmd {
	switch goos {
	case "darwin":
		return exec.Command("open", rawURL)
	case "linux":
		return exec.Command("xdg-open", rawURL)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return nil
	}
}

// tryOpenBrowser opens rawURL in the default browser; errors are silently ignored.
func tryOpenBrowser(rawURL string) error {
	cmd := patBrowserOpenCommand(runtime.GOOS, rawURL)
	if cmd == nil {
		return nil
	}
	return cmd.Start()
}
