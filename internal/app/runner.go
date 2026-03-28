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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/safety"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

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

func newCommandRunnerWithFlags(loader cli.CatalogLoader, flags *GlobalFlags) executor.Runner {
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

	if shouldUseDirectRuntime(invocation) {
		if endpoint, ok := directRuntimeEndpoint(invocation.CanonicalProduct); ok {
			return r.executeInvocation(ctx, endpoint, invocation)
		}
	}

	catalog, err := r.loader.Load(ctx)
	if err != nil {
		return executor.Result{}, err
	}

	product, ok := catalog.FindProduct(invocation.CanonicalProduct)
	if !ok || strings.TrimSpace(product.Endpoint) == "" {
		return r.fallback.Run(ctx, invocation)
	}
	if _, ok := product.FindTool(invocation.Tool); !ok {
		return r.fallback.Run(ctx, invocation)
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

func (r *runtimeRunner) executeInvocation(ctx context.Context, endpoint string, invocation executor.Invocation) (executor.Result, error) {
	tc := r.transport.WithAuth(r.resolveAuthToken(ctx), resolveIdentityHeaders())

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

	callResult, err := tc.CallTool(ctx, endpoint, invocation.Tool, invocation.Params)
	if err != nil {
		captureRuntimeFailure(invocation, err, err)
		return executor.Result{}, err
	}

	if callResult.IsError {
		mcpErr := apperrors.NewAPI(
			extractMCPErrorMessage(callResult),
			apperrors.WithOperation("tools/call"),
			apperrors.WithReason("mcp_tool_error"),
			apperrors.WithServerKey(invocation.CanonicalProduct),
			apperrors.WithHint("MCP tool returned a business error; check tool parameters and refer to skill documentation."),
		)
		captureRuntimeFailure(invocation, mcpErr, mcpErr)
		return executor.Result{}, mcpErr
	}

	scanReport, err := r.scanContent(callResult.Content)
	if err != nil {
		return executor.Result{}, err
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

func (r *runtimeRunner) resolveAuthToken(ctx context.Context) string {
	explicitToken := ""
	if r != nil && r.globalFlags != nil {
		explicitToken = r.globalFlags.Token
	}
	return resolveRuntimeAuthToken(ctx, explicitToken)
}

func resolveRuntimeAuthToken(ctx context.Context, explicitToken string) string {
	if token := strings.TrimSpace(explicitToken); token != "" {
		return token
	}
	configDir := defaultConfigDir()
	provider := authpkg.NewOAuthProvider(configDir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	configureOAuthProviderCompatibility(provider, configDir)
	token, tokenErr := provider.GetAccessToken(ctx)
	if tokenErr == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token)
	}
	// If the error is a decryption failure (corrupted data), surface
	// it immediately instead of falling back to empty token.
	if tokenErr != nil && errors.Is(tokenErr, authpkg.ErrTokenDecryption) {
		slog.Error(tokenErr.Error())
		return ""
	}
	manager := authpkg.NewManager(configDir, nil)
	configureLegacyAuthManagerCompatibility(manager)
	if token, _, err := manager.GetToken(); err == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token)
	}
	return ""
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

	// Inject environment variable based headers for MCP gateway tracking
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
	return headers
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
