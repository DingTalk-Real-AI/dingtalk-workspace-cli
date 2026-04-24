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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/safety"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DWS_RUNTIME_CONTENT_SCAN",
		Category:    configmeta.CategoryRuntime,
		Description: "启用 MCP 响应内容安全扫描",
		Example:     "true",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DWS_RUNTIME_CONTENT_SCAN_ENFORCE",
		Category:    configmeta.CategoryRuntime,
		Description: "内容安全扫描发现问题时阻断响应",
		Example:     "true",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DWS_RUNTIME_CONTENT_SCAN_REPORT",
		Category:    configmeta.CategoryRuntime,
		Description: "在 JSON 输出中包含安全扫描报告",
		Example:     "true",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DINGTALK_AGENT",
		Category:    configmeta.CategoryExternal,
		Description: "业务 Agent 名称；仅用于 x-dingtalk-agent 请求头，与 claw-type/host-owned PAT 判定无关",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DINGTALK_TRACE_ID",
		Category:    configmeta.CategoryExternal,
		Description: "MCP 请求 x-dingtalk-trace-id 头",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DINGTALK_SESSION_ID",
		Category:    configmeta.CategoryExternal,
		Description: "MCP 请求 x-dingtalk-session-id 头",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DINGTALK_MESSAGE_ID",
		Category:    configmeta.CategoryExternal,
		Description: "MCP 请求 x-dingtalk-message-id 头",
	})
}

const (
	runtimeContentScanEnv             = "DWS_RUNTIME_CONTENT_SCAN"
	runtimeContentScanEnforceEnv      = "DWS_RUNTIME_CONTENT_SCAN_ENFORCE"
	runtimeContentScanReportOutputEnv = "DWS_RUNTIME_CONTENT_SCAN_REPORT"

	// Environment variables for MCP request headers (passed from caller)
	envDingtalkAgent     = "DINGTALK_AGENT"
	envDingtalkTraceID   = "DINGTALK_TRACE_ID"
	envDingtalkSessionID = "DINGTALK_SESSION_ID"
	envDingtalkMessageID = "DINGTALK_MESSAGE_ID"
)

// hostOwnedPATDecisionOnce ensures the host-owned PAT decision is logged at
// most once per CLI process. The log line is emitted at Debug level so
// `--debug` (or `--verbose`) surfaces it on stderr; the file logger at
// ~/.dws/logs/dws.log captures it unconditionally at DEBUG. It records
// ONLY the derived booleans — never the env value, token, client-id or
// flow-id — so logs remain safe to attach to issues.
// See docs/pat/host-integration.md §10.
var hostOwnedPATDecisionOnce sync.Once

// logHostOwnedPATDecisionOnce emits the single-shot debug trace. It is
// called lazily from the runtime Run path (which executes AFTER
// PersistentPreRunE has applied --debug / --verbose via configureLogLevel)
// so the line actually surfaces when the user asks for it.
func logHostOwnedPATDecisionOnce() {
	hostOwnedPATDecisionOnce.Do(func() {
		slog.Debug("runtime.host_owned_pat",
			"hostOwned", authpkg.HostOwnsPATFlow(),
			"agentCodeEnvPresent", os.Getenv(authpkg.AgentCodeEnv) != "",
		)
	})
}

func newCommandRunnerWithFlags(loader cli.CatalogLoader, flags *GlobalFlags) executor.Runner {
	// Ensure DWS_CLIENT_ID env is populated from persisted config before
	// resolveIdentityHeaders reads it.  This covers fresh-process cold starts
	// where no env var has been inherited from a parent process.
	if os.Getenv("DWS_CLIENT_ID") == "" {
		if cid := authpkg.ClientID(); cid != "" {
			_ = os.Setenv("DWS_CLIENT_ID", cid)
		}
	}

	var httpClient *http.Client
	if flags != nil && flags.Timeout > 0 {
		httpClient = &http.Client{Timeout: time.Duration(flags.Timeout) * time.Second}
	}
	transportClient := transport.NewClient(httpClient)
	transportClient.ExtraHeaders = resolveIdentityHeaders()
	transportClient.FileLogger = FileLoggerInstance()
	return &runtimeRunner{
		loader:             loader,
		transport:          transportClient,
		globalFlags:        flags,
		fallback:           executor.EchoRunner{},
		scanner:            newRuntimeContentScanner(),
		enforceContentScan: runtimeFlagEnabled(os.Getenv(runtimeContentScanEnforceEnv), false),
		includeScanReport:  runtimeFlagEnabled(os.Getenv(runtimeContentScanReportOutputEnv), false),
	}
}

type runtimeRunner struct {
	loader             cli.CatalogLoader
	transport          *transport.Client
	globalFlags        *GlobalFlags
	fallback           executor.Runner
	scanner            safety.Scanner
	enforceContentScan bool
	includeScanReport  bool
}

func (r *runtimeRunner) Run(ctx context.Context, invocation executor.Invocation) (executor.Result, error) {
	// Emit the one-shot host-owned PAT decision log. Placed here (not in
	// the constructor) so it fires AFTER PersistentPreRunE has configured
	// slog level per --debug / --verbose. The Once guard makes repeat
	// invocations within the same process free.
	logHostOwnedPATDecisionOnce()

	if r.loader == nil || r.transport == nil {
		return r.fallback.Run(ctx, invocation)
	}

	// Mock mode: skip catalog validation, use a placeholder endpoint.
	if r.globalFlags != nil && r.globalFlags.Mock {
		endpoint := fmt.Sprintf("https://mock-mcp-%s.dingtalk.com", invocation.CanonicalProduct)
		if override, ok := productEndpointOverride(invocation.CanonicalProduct); ok {
			endpoint = override
		}
		return r.executeInvocation(ctx, endpoint, invocation)
	}

	// Prefetch the Keychain token in the background. Keychain access costs
	// ~70ms on macOS; starting it here lets the load overlap with endpoint
	// resolution and catalog loading below.
	go getCachedRuntimeToken(ctx)

	if shouldUseDirectRuntime(invocation) {
		if endpoint, ok := directRuntimeEndpoint(invocation.CanonicalProduct, invocation.Tool); ok {
			return r.executeInvocation(ctx, endpoint, invocation)
		}
	}

	catalogStart := time.Now()
	catalog, err := r.loader.Load(ctx)
	RecordTiming(ctx, "catalog_load", time.Since(catalogStart))
	if err != nil {
		var degraded *cli.CatalogDegraded
		if !errors.As(err, &degraded) {
			return executor.Result{}, err
		}
	}

	product, ok := catalog.FindProduct(invocation.CanonicalProduct)
	if !ok || strings.TrimSpace(product.Endpoint) == "" {
		return r.handleCatalogMiss(ctx, invocation, "product missing from discovery catalog and no supplement/env override")
	}
	if _, ok := product.FindTool(invocation.Tool); !ok {
		return r.handleCatalogMiss(ctx, invocation, fmt.Sprintf("tool %q not declared by product %q in discovery catalog", invocation.Tool, invocation.CanonicalProduct))
	}
	if r.globalFlags != nil && r.globalFlags.DryRun {
		invocation.DryRun = true
	}

	endpoint := product.Endpoint
	if override, ok := productEndpointOverride(invocation.CanonicalProduct); ok {
		endpoint = override
	}
	return r.executeInvocation(ctx, endpoint, invocation)
}

// handleCatalogMiss decides what to do when discovery catalog does not cover the
// requested product / tool and no `directRuntimeEndpoint` match fired earlier.
//
// Previously every catalog miss silently fell through to EchoRunner, which
// returns an empty `executor.Result{Response: nil}`. The helper-invocation
// adapter then converted that into `&edition.ToolResult{}`, whose `Content`
// marshals to `null`, surfacing as `{"Content": null}` at the CLI. Users had no
// signal that endpoint resolution failed — see the fix-wukong-discovery-missing-servers plan (Phase 3) for the full trace.
//
// New contract:
//   - Dry-run (invocation.DryRun or globalFlags.DryRun): keep EchoRunner so
//     `--dry-run` still prints the planned payload without real execution.
//   - Otherwise: return an explicit apperrors.NewAPI("endpoint_not_resolved")
//     with the offending product/tool attached. This fails fast to stderr and
//     makes missing envelopes / supplement gaps immediately visible.
func (r *runtimeRunner) handleCatalogMiss(ctx context.Context, invocation executor.Invocation, detail string) (executor.Result, error) {
	dryRun := invocation.DryRun || (r.globalFlags != nil && r.globalFlags.DryRun)
	if dryRun {
		invocation.DryRun = true
		return r.fallback.Run(ctx, invocation)
	}
	return executor.Result{}, apperrors.NewAPI(
		fmt.Sprintf("endpoint not resolved for product %q (tool %q): %s", invocation.CanonicalProduct, invocation.Tool, detail),
		apperrors.WithOperation("discovery.resolve"),
		apperrors.WithReason("endpoint_not_resolved"),
		apperrors.WithServerKey(invocation.CanonicalProduct),
		apperrors.WithHint("产品 envelope 可能未下发到 discovery，或已经被 serverDeps fail-fast 丢弃；可执行 'dws cache refresh' 强制重新 discovery，仍失败请向 Portal 确认 envelope 状态。"),
		apperrors.WithActions("dws cache refresh"),
	)
}

func (r *runtimeRunner) executeInvocation(ctx context.Context, endpoint string, invocation executor.Invocation) (result executor.Result, retErr error) {
	// Route stdio:// endpoints to the local StdioClient — no HTTP, no auth.
	if IsStdioEndpoint(endpoint) {
		return r.executeStdioInvocation(ctx, invocation)
	}

	invokeStart := time.Now()
	execID := generateExecutionID()
	r.transport.ExecutionId = execID

	// Lazy bind FileLogger: it may be nil at construction time because
	// configureLogLevel runs later in PersistentPreRunE.
	if r.transport.FileLogger == nil {
		r.transport.FileLogger = FileLoggerInstance()
	}

	fl := r.transport.FileLogger

	defer func() {
		var errCat, errReason string
		if retErr != nil {
			var typed *apperrors.Error
			if errors.As(retErr, &typed) {
				errCat = string(typed.Category)
				errReason = typed.Reason
			} else {
				errCat = "unknown"
				errReason = retErr.Error()
			}
		}
		logging.LogCommandEnd(fl, execID,
			invocation.CanonicalProduct, invocation.Tool,
			retErr == nil, time.Since(invokeStart), errCat, errReason)
	}()

	// Check if this product has plugin-level auth credentials registered.
	// If so, use the plugin's token instead of the default DingTalk OAuth token.
	// This allows third-party MCP servers (e.g. Bailian) to use their own API keys.
	pluginAuth, hasPluginAuth := LookupPluginAuth(invocation.CanonicalProduct)

	authToken := ""
	if hasPluginAuth {
		authToken = pluginAuth.Token
	} else {
		authToken = r.resolveAuthToken(ctx)
	}

	var timeoutSec int
	if r.globalFlags != nil {
		timeoutSec = r.globalFlags.Timeout
	}
	logging.LogCommandStart(fl, execID,
		invocation.CanonicalProduct, invocation.Tool, endpoint, version, authToken != "", timeoutSec)

	if invocation.DryRun {
		return executor.Result{
			Invocation: invocation,
			Response: map[string]any{
				"dry_run":  true,
				"endpoint": transport.RedactURL(endpoint),
				"request":  executor.ToolCallRequest(invocation.Tool, invocation.Params),
				"note":     "execution skipped by --dry-run",
			},
		}, nil
	}

	// Mock mode: return predefined mock response without network call.
	if r.globalFlags != nil && r.globalFlags.Mock {
		invocation.Implemented = true
		return executor.Result{
			Invocation: invocation,
			Response: map[string]any{
				"endpoint": transport.RedactURL(endpoint),
				"content": map[string]any{
					"success": true,
					"result":  []any{},
					"_mock":   true,
					"_tool":   invocation.Tool,
				},
			},
		}, nil
	}

	// Fail-fast: reject unauthenticated requests before making network calls.
	// This provides a clear error message instead of cryptic HTTP 400 from MCP.
	if strings.TrimSpace(authToken) == "" {
		return executor.Result{}, apperrors.NewAuth(
			"未登录，请先执行 dws auth login",
			apperrors.WithReason("not_authenticated"),
			apperrors.WithHint("运行 'dws auth login' 完成登录后重试"),
			apperrors.WithActions("dws auth login"),
		)
	}

	var tc *transport.Client
	if hasPluginAuth {
		// Use plugin-level auth: inject the plugin's token and trust its domains.
		tc = r.transport.WithAuth(authToken, pluginAuth.ExtraHeaders)
		tc.TrustedDomains = pluginAuth.TrustedDomains
	} else {
		// Default path: use DingTalk OAuth token with identity headers.
		tc = r.transport.WithAuth(authToken, resolveIdentityHeaders())
	}

	callCtx := ctx
	if r.globalFlags != nil && r.globalFlags.Timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(r.globalFlags.Timeout)*time.Second)
		defer cancel()
	}

	callStart := time.Now()
	callResult, err := tc.CallTool(callCtx, endpoint, invocation.Tool, invocation.Params)
	RecordTiming(ctx, "mcp_call", time.Since(callStart))
	if err != nil {
		if isAuthError(err) {
			if fn := edition.Get().OnAuthError; fn != nil {
				if overrideErr := fn(defaultConfigDir(), err); overrideErr != nil {
					captureRuntimeFailure(invocation, err, overrideErr)
					return executor.Result{}, overrideErr
				}
			}
		}
		// PAT scope error: offer human-readable output and retry after authorization
		if isPatScopeError(err) {
			scopeErr := extractPatScopeError(err)
			captureRuntimeFailure(invocation, err, err)
			return retryWithPatAuthRetry(ctx, r, invocation, scopeErr, defaultConfigDir(), os.Stderr)
		}
		captureRuntimeFailure(invocation, err, err)
		return executor.Result{}, err
	}

	// ---- Edition hook gets first dibs (preserves overlay PATError passthrough) ----
	if fn := edition.Get().ClassifyToolResult; fn != nil {
		if editionErr := fn(callResult.Content); editionErr != nil {
			if patCheck := apperrors.AsPatAuthCheckError(editionErr); patCheck != nil {
				if IsPatRetrying(ctx) {
					return executor.Result{}, patCheck // already retried once, don't loop
				}
				return handlePatAuthCheck(ctx, r, invocation, patCheck, defaultConfigDir(), os.Stderr)
			}
			return executor.Result{}, editionErr
		}
	}

	// ---- Structured PAT auth check (open-source fallback) ----
	if patCheck := apperrors.ClassifyPatAuthCheck(callResult.Content); patCheck != nil {
		if IsPatRetrying(ctx) {
			return executor.Result{}, patCheck // already retried once, don't loop
		}
		return handlePatAuthCheck(ctx, r, invocation, patCheck, defaultConfigDir(), os.Stderr)
	}

	if callResult.IsError {
		diag := transport.ExtractServerDiagnosticsFromMap(callResult.Content)
		logBusinessError(r.transport.FileLogger, "mcp_tool_error", invocation, callResult.Content, diag)

		// ClassifyToolResult hook: let the overlay intercept known error
		// patterns (PAT permission, gateway-auth) before generic handling.
		if classify := edition.Get().ClassifyToolResult; classify != nil {
			if hookErr := classify(callResult.Content); hookErr != nil {
				captureRuntimeFailure(invocation, hookErr, hookErr)
				return executor.Result{}, hookErr
			}
		}

		mcpErr := apperrors.NewAPI(
			extractMCPErrorMessage(callResult),
			apperrors.WithOperation("tools/call"),
			apperrors.WithReason("mcp_tool_error"),
			apperrors.WithServerKey(invocation.CanonicalProduct),
			apperrors.WithHint("MCP tool returned a business error; check tool parameters and refer to skill documentation."),
			apperrors.WithServerDiag(diag),
		)
		// PAT scope error in business response: offer human-readable output and retry
		if isPatScopeError(mcpErr) {
			scopeErr := extractPatScopeError(mcpErr)
			captureRuntimeFailure(invocation, mcpErr, mcpErr)
			return retryWithPatAuthRetry(ctx, r, invocation, scopeErr, defaultConfigDir(), os.Stderr)
		}
		captureRuntimeFailure(invocation, mcpErr, mcpErr)
		return executor.Result{}, mcpErr
	}

	scanReport, err := r.scanContent(callResult.Content)
	if err != nil {
		return executor.Result{}, err
	}

	if bizErr := detectBusinessError(callResult.Content); bizErr != "" {
		diag := transport.ExtractServerDiagnosticsFromMap(callResult.Content)
		logBusinessError(r.transport.FileLogger, "business_error", invocation, callResult.Content, diag)
		return executor.Result{}, apperrors.NewAPI(bizErr,
			apperrors.WithOperation("tools/call"),
			apperrors.WithReason("business_error"),
			apperrors.WithServerKey(invocation.CanonicalProduct),
			apperrors.WithHint("The API returned a business-level error. Check required parameters and values."),
			apperrors.WithServerDiag(diag),
		)
	}

	invocation.Implemented = true
	response := map[string]any{
		"endpoint": transport.RedactURL(endpoint),
		"content":  callResult.Content,
	}
	if r.includeScanReport && scanReport.Scanned {
		response["safety"] = scanReport
	}
	return executor.Result{Invocation: invocation, Response: response}, nil
}

// executeStdioInvocation dispatches a tool call through a local StdioClient
// subprocess instead of the HTTP transport. This is used for plugin stdio
// servers whose endpoints use the stdio:// scheme.
func (r *runtimeRunner) executeStdioInvocation(ctx context.Context, invocation executor.Invocation) (executor.Result, error) {
	if invocation.DryRun {
		return executor.Result{
			Invocation: invocation,
			Response: map[string]any{
				"dry_run":   true,
				"transport": "stdio",
				"request":   executor.ToolCallRequest(invocation.Tool, invocation.Params),
				"note":      "execution skipped by --dry-run",
			},
		}, nil
	}

	client, ok := LookupStdioClient(invocation.CanonicalProduct)
	if !ok {
		return executor.Result{}, apperrors.NewInternal(
			fmt.Sprintf("stdio client not found for %q", invocation.CanonicalProduct))
	}

	callCtx := ctx
	if r.globalFlags != nil && r.globalFlags.Timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(r.globalFlags.Timeout)*time.Second)
		defer cancel()
	}

	callResult, err := client.CallTool(callCtx, invocation.Tool, invocation.Params)
	if err != nil {
		return executor.Result{}, apperrors.NewAPI(
			fmt.Sprintf("stdio call failed: %v", err),
			apperrors.WithOperation("tools/call"),
			apperrors.WithReason("stdio_error"),
		)
	}

	if callResult.IsError {
		return executor.Result{}, apperrors.NewAPI(
			extractMCPErrorMessage(callResult),
			apperrors.WithOperation("tools/call"),
			apperrors.WithReason("mcp_tool_error"),
			apperrors.WithServerKey(invocation.CanonicalProduct),
		)
	}

	invocation.Implemented = true
	return executor.Result{
		Invocation: invocation,
		Response: map[string]any{
			"transport": "stdio",
			"content":   callResult.Content,
		},
	}, nil
}

func (r *runtimeRunner) resolveAuthToken(ctx context.Context) string {
	explicitToken := ""
	if r != nil && r.globalFlags != nil {
		explicitToken = r.globalFlags.Token
	}
	if token := strings.TrimSpace(explicitToken); token != "" {
		return token
	}
	if tp := edition.Get().TokenProvider; tp != nil {
		token, _ := tp(ctx, func() (string, error) {
			return resolveAccessTokenFromDir(ctx, defaultConfigDir())
		})
		return token
	}
	return getCachedRuntimeToken(ctx)
}

func resolveRuntimeAuthToken(ctx context.Context, explicitToken string) string {
	if token := strings.TrimSpace(explicitToken); token != "" {
		return token
	}
	// Use cached token to avoid repeated Keychain access (~70ms per call)
	return getCachedRuntimeToken(ctx)
}

// Cached token state for process lifetime
var (
	cachedRuntimeToken     string
	cachedRuntimeTokenOnce sync.Once
)

// getCachedRuntimeToken returns a cached access token, loading it only once per process.
// This avoids repeated Keychain access which takes ~70ms each time.
func getCachedRuntimeToken(ctx context.Context) string {
	cachedRuntimeTokenOnce.Do(func() {
		loadStart := time.Now()
		defer func() { RecordTiming(ctx, "auth_keychain", time.Since(loadStart)) }()

		configDir := defaultConfigDir()
		token, tokenErr := resolveAccessTokenFromDir(ctx, configDir)
		if tokenErr != nil && errors.Is(tokenErr, authpkg.ErrTokenDecryption) {
			slog.Error(tokenErr.Error())
			return
		}
		if token != "" {
			cachedRuntimeToken = token
		}
	})
	return cachedRuntimeToken
}

// generateExecutionID returns a random 16-char hex string used to correlate
// all log entries (command_start, jsonrpc_request, command_end, etc.) belonging
// to a single command invocation.
func generateExecutionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ResetRuntimeTokenCache clears the cached token, forcing a reload on next access.
// This should be called after login/logout operations.
func ResetRuntimeTokenCache() {
	cachedRuntimeTokenOnce = sync.Once{}
	cachedRuntimeToken = ""
}

func newRuntimeContentScanner() safety.Scanner {
	if !runtimeFlagEnabled(os.Getenv(runtimeContentScanEnv), true) {
		return nil
	}
	return safety.NewContentScanner()
}

func (r *runtimeRunner) scanContent(content map[string]any) (safety.Report, error) {
	if r == nil || r.scanner == nil {
		return safety.Report{Scanned: false}, nil
	}
	report := r.scanner.ScanPayload(content)
	if r.enforceContentScan && len(report.Findings) > 0 {
		return report, apperrors.NewValidation("runtime response blocked by content safety scan")
	}
	return report, nil
}

func runtimeFlagEnabled(raw string, defaultValue bool) bool {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return defaultValue
	}
	switch trimmed {
	case "0", "false", "no", "n", "off":
		return false
	default:
		return true
	}
}

func isAuthError(err error) bool {
	var appErr *apperrors.Error
	if errors.As(err, &appErr) {
		return appErr.Category == apperrors.CategoryAuth
	}
	return false
}

func productEndpointOverride(productID string) (string, bool) {
	key := "DINGTALK_" + strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(productID), "-", "_")) + "_MCP_URL"
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", false
	}
	return value, true
}

// resolveIdentityHeaders loads or creates agent identity and returns HTTP
// headers to inject into MCP requests. Best-effort: returns nil on failure.
func resolveIdentityHeaders() map[string]string {
	id := authpkg.EnsureExists(defaultConfigDir())
	headers := id.Headers()
	if headers == nil {
		headers = make(map[string]string)
	}

	// Inject environment variable based headers for MCP gateway tracking.
	// DINGTALK_AGENT, if set by the caller, is forwarded verbatim as the
	// x-dingtalk-agent header. It does NOT influence claw-type (which the
	// open-source edition pins to edition.DefaultOSSClawType via the
	// MergeHeaders hook below) and it does NOT influence the host-owned
	// PAT decision (driven solely by DINGTALK_DWS_AGENTCODE).
	envHeaders := map[string]string{
		"x-dingtalk-agent":      os.Getenv(envDingtalkAgent),
		"x-dingtalk-trace-id":   os.Getenv(envDingtalkTraceID),
		"x-dingtalk-session-id": os.Getenv(envDingtalkSessionID),
		"x-dingtalk-message-id": os.Getenv(envDingtalkMessageID),
	}
	for k, v := range envHeaders {
		if v != "" {
			headers[k] = v
		}
	}

	// Inject third-party channel headers. DWS_CHANNEL is forwarded as the
	// upstream channelCode.
	if v := authpkg.CurrentChannelCode(); v != "" {
		headers["x-dws-channel"] = v
	}

	if fn := edition.Get().MergeHeaders; fn != nil {
		headers = fn(headers)
	}
	return headers
}

// detectBusinessError checks the MCP response content for DingTalk business
// errors (success=false + errorCode/errorMsg) that are not flagged at the MCP
// protocol level. Returns the error message, or "" if the response is OK.
func detectBusinessError(content map[string]any) string {
	success, ok := content["success"]
	if !ok {
		return ""
	}
	b, ok := success.(bool)
	if !ok || b {
		return ""
	}
	if msg, ok := content["errorMsg"].(string); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	if code, ok := content["errorCode"].(string); ok && strings.TrimSpace(code) != "" {
		return "business error: code " + strings.TrimSpace(code)
	}
	return "business error: success=false"
}

// extractMCPErrorMessage builds an error message from a ToolCallResult with
// isError=true. It extracts text from content blocks when available.
func extractMCPErrorMessage(result transport.ToolCallResult) string {
	// Try text from content blocks first.
	for _, block := range result.Blocks {
		text := strings.TrimSpace(block.Text)
		if text != "" {
			return text
		}
	}
	// Try stringified content map.
	if msg, ok := result.Content["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	if msg, ok := result.Content["error"].(string); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	return "MCP tool returned an error response"
}

// logBusinessError logs MCP tool errors and business errors to the file logger
// so they can be diagnosed offline. These errors arrive as HTTP 200 responses
// and would otherwise not be captured by transport-level logging.
func logBusinessError(logger *slog.Logger, reason string, inv executor.Invocation, content map[string]any, diag apperrors.ServerDiagnostics) {
	if logger == nil {
		return
	}
	attrs := []any{
		"product", inv.CanonicalProduct,
		"tool", inv.Tool,
		"reason", reason,
	}
	if diag.TraceID != "" {
		attrs = append(attrs, "trace_id", diag.TraceID)
	}
	if diag.ServerErrorCode != "" {
		attrs = append(attrs, "server_error_code", diag.ServerErrorCode)
	}
	if diag.TechnicalDetail != "" {
		attrs = append(attrs, "technical_detail", diag.TechnicalDetail)
	}
	if msg, ok := content["error"].(string); ok {
		attrs = append(attrs, "error", msg)
	}
	if msg, ok := content["errorMsg"].(string); ok {
		attrs = append(attrs, "errorMsg", msg)
	}
	if msg, ok := content["message"].(string); ok {
		attrs = append(attrs, "message", msg)
	}
	logger.Warn("business_error", attrs...)
}
