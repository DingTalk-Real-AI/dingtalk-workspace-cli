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

package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"io"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/config"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
)

const (
	trustedDomainsEnv     = "DWS_TRUSTED_DOMAINS"
	defaultTrustedDomains = "*.dingtalk.com"

	// defaultHTTPTimeout is the default timeout for HTTP transport requests.
	defaultHTTPTimeout = 30 * time.Second

	// Default retry parameters for JSON-RPC calls.
	defaultMaxRetries    = 2
	defaultRetryDelay    = 10 * time.Millisecond
	defaultRetryMaxDelay = 80 * time.Millisecond

	// Security headers
	HeaderSource      = "X-Cli-Source"
	HeaderVersion     = "X-Cli-Version"
	HeaderExecutionId = "X-Cli-Execution-Id"
	SourceValue       = "dws-cli"
)

// Supported MCP protocol versions, ordered from newest to oldest.
var supportedProtocolVersions = []string{
	"2025-03-26",
	"2024-11-05",
	"2024-06-18",
}

type Client struct {
	HTTPClient       *http.Client
	MaxRetries       int
	RetryDelay       time.Duration
	RetryMaxDelay    time.Duration
	AuthToken        string
	ExtraHeaders     map[string]string
	SnapshotRecorder SnapshotRecorder
	TrustedDomains   []string
	ExecutionId      string // Request tracing ID for debugging
	sleep            func(context.Context, time.Duration) error
	wildcardOnce     sync.Once
	// Stderr is the writer for warning messages. Defaults to os.Stderr.
	Stderr io.Writer
}

type SnapshotRecorder interface {
	RecordJSONRPC(method, endpoint string, requestBody, responseBody []byte) string
}

type requestEnvelope struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type responseEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type InitializeResult struct {
	RequestedProtocolVersion string         `json:"requested_protocol_version"`
	ProtocolVersion          string         `json:"protocol_version"`
	Capabilities             map[string]any `json:"capabilities"`
	ServerInfo               map[string]any `json:"server_info"`
}

type ToolDescriptor struct {
	Name         string         `json:"name"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Sensitive    bool           `json:"sensitive,omitempty"`
}

type ToolsListResult struct {
	Tools []ToolDescriptor `json:"tools"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ToolCallResult struct {
	Content           map[string]any `json:"-"`
	StructuredContent map[string]any `json:"structuredContent,omitempty"`
	Blocks            []ContentBlock `json:"content,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
}

func (r *ToolCallResult) UnmarshalJSON(data []byte) error {
	type rawResult struct {
		Content           json.RawMessage `json:"content"`
		StructuredContent map[string]any  `json:"structuredContent"`
		IsError           bool            `json:"isError,omitempty"`
	}

	var raw rawResult
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.StructuredContent = cloneAnyMap(raw.StructuredContent)
	r.IsError = raw.IsError

	if len(bytes.TrimSpace(raw.Content)) == 0 {
		if len(r.StructuredContent) > 0 {
			r.Content = cloneAnyMap(r.StructuredContent)
		}
		return nil
	}

	var object map[string]any
	if err := json.Unmarshal(raw.Content, &object); err == nil {
		r.Content = object
		return nil
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(raw.Content, &blocks); err == nil {
		r.Blocks = blocks
		if len(r.StructuredContent) > 0 {
			r.Content = cloneAnyMap(r.StructuredContent)
			return nil
		}
		for _, block := range blocks {
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			var parsed map[string]any
			if err := json.Unmarshal([]byte(text), &parsed); err == nil {
				r.Content = parsed
				return nil
			}
		}
		return nil
	}

	return fmt.Errorf("unsupported tools/call content shape")
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout:       defaultHTTPTimeout,
			CheckRedirect: safeRedirectPolicy,
		}
	} else if httpClient.CheckRedirect == nil {
		// Wrap existing client with safe redirect policy
		httpClient.CheckRedirect = safeRedirectPolicy
	}
	return &Client{
		HTTPClient:    httpClient,
		MaxRetries:    defaultMaxRetries,
		RetryDelay:    defaultRetryDelay,
		RetryMaxDelay: defaultRetryMaxDelay,
	}
}

// safeRedirectPolicy prevents credential headers from being forwarded
// when a response redirects to a different host (e.g. API 302 → CDN).
// Strips Authorization, x-user-access-token on cross-host redirects;
// other headers like X-Cli-* pass through.
func safeRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("too many redirects")
	}
	if len(via) > 0 && req.URL.Host != via[0].URL.Host {
		// Cross-host redirect: strip sensitive headers to prevent credential leakage
		req.Header.Del("Authorization")
		req.Header.Del("x-user-access-token")
	}
	return nil
}

// WithAuth returns a shallow copy of c with the given auth token and extra
// headers. The returned client shares the underlying HTTP client but is safe
// to use concurrently with the original.
func (c *Client) WithAuth(token string, headers map[string]string) *Client {
	return &Client{
		HTTPClient:       c.HTTPClient,
		MaxRetries:       c.MaxRetries,
		RetryDelay:       c.RetryDelay,
		RetryMaxDelay:    c.RetryMaxDelay,
		AuthToken:        token,
		ExtraHeaders:     headers,
		SnapshotRecorder: c.SnapshotRecorder,
		TrustedDomains:   c.TrustedDomains,
		ExecutionId:      c.ExecutionId,
		sleep:            c.sleep,
		Stderr:           c.Stderr,
	}
}

// WithExecutionId returns a shallow copy of c with the given execution ID.
// The execution ID is included in requests for tracing and debugging.
func (c *Client) WithExecutionId(executionId string) *Client {
	copy := c.WithAuth(c.AuthToken, c.ExtraHeaders)
	copy.ExecutionId = executionId
	return copy
}

func SupportedProtocolVersions() []string {
	return append([]string(nil), supportedProtocolVersions...)
}

func (c *Client) Initialize(ctx context.Context, endpoint string) (InitializeResult, error) {
	for _, version := range SupportedProtocolVersions() {
		params := map[string]any{
			"capabilities": map[string]any{},
			"clientInfo": map[string]any{
				"name":    "dws",
				"version": "0.0.0-dev",
			},
			"protocolVersion": version,
		}

		var payload InitializeResult
		if err := c.callJSONRPC(ctx, endpoint, requestEnvelope{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "initialize",
			Params:  params,
		}, true, &payload); err == nil {
			if payload.ProtocolVersion == "" {
				payload.ProtocolVersion = version
			}
			payload.RequestedProtocolVersion = version
			return payload, nil
		}
	}
	return InitializeResult{}, apperrors.NewDiscovery(fmt.Sprintf("initialize failed for all supported protocol versions at %s", RedactURL(endpoint)))
}

func (c *Client) NotifyInitialized(ctx context.Context, endpoint string) error {
	return c.callJSONRPC(ctx, endpoint, requestEnvelope{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		Params:  map[string]any{},
	}, false, nil)
}

func (c *Client) ListTools(ctx context.Context, endpoint string) (ToolsListResult, error) {
	var payload ToolsListResult
	if err := c.callJSONRPC(ctx, endpoint, requestEnvelope{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}, true, &payload); err != nil {
		return ToolsListResult{}, err
	}
	return payload, nil
}

func (c *Client) CallTool(ctx context.Context, endpoint, tool string, arguments map[string]any) (ToolCallResult, error) {
	var payload ToolCallResult
	if err := c.callJSONRPC(ctx, endpoint, requestEnvelope{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      tool,
			"arguments": arguments,
		},
	}, true, &payload); err != nil {
		return ToolCallResult{}, err
	}
	return payload, nil
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func (c *Client) callJSONRPC(ctx context.Context, endpoint string, request requestEnvelope, expectResponse bool, out any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return apperrors.NewInternal("failed to encode JSON-RPC request")
	}

	resp, err := c.doWithRetry(ctx, endpoint, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return apperrors.NewDiscovery(
			"failed to read JSON-RPC response",
			apperrors.WithOperation(request.Method),
			apperrors.WithReason(reasonForMethod(request.Method, "response_read_failed")),
			apperrors.WithHint(i18n.T("检查服务连通性后重试；如持续失败，请确认 MCP 服务响应正常。")),
			apperrors.WithActions(discoveryActions("")...),
		)
	}
	snapshotPath := ""
	if c.SnapshotRecorder != nil && (expectResponse || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || len(bytes.TrimSpace(data)) > 0) {
		snapshotPath = c.SnapshotRecorder.RecordJSONRPC(request.Method, endpoint, body, data)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return httpStatusError(request.Method, endpoint, resp.StatusCode, snapshotPath)
	}

	if !expectResponse {
		return nil
	}

	var envelope responseEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return apperrors.NewDiscovery(
			fmt.Sprintf("unexpected protocol response from %s", RedactURL(endpoint)),
			apperrors.WithOperation(request.Method),
			apperrors.WithReason(reasonForMethod(request.Method, "invalid_response")),
			apperrors.WithHint(i18n.T("MCP 服务返回了无法解析的协议响应；检查服务版本或上游代理。")),
			apperrors.WithActions(discoveryActions(snapshotPath)...),
			apperrors.WithSnapshot(snapshotPath),
		)
	}
	if envelope.Error != nil {
		return jsonrpcEnvelopeError(request.Method, envelope.Error, snapshotPath)
	}
	if len(envelope.Result) == 0 {
		return apperrors.NewDiscovery(
			fmt.Sprintf("JSON-RPC %s returned an empty result payload", request.Method),
			apperrors.WithOperation(request.Method),
			apperrors.WithReason(reasonForMethod(request.Method, "empty_result")),
			apperrors.WithHint(i18n.T("服务返回了空结果；请稍后重试，必要时查看 recovery snapshot。")),
			apperrors.WithActions(discoveryActions(snapshotPath)...),
			apperrors.WithSnapshot(snapshotPath),
		)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return apperrors.NewDiscovery(
			fmt.Sprintf("failed to decode JSON-RPC %s result", request.Method),
			apperrors.WithOperation(request.Method),
			apperrors.WithReason(reasonForMethod(request.Method, "result_decode_failed")),
			apperrors.WithHint(i18n.T("结果格式与客户端预期不一致；请检查服务协议变更或回退到最近可用版本。")),
			apperrors.WithActions(discoveryActions(snapshotPath)...),
			apperrors.WithSnapshot(snapshotPath),
		)
	}
	return nil
}

func (c *Client) doWithRetry(ctx context.Context, endpoint string, body []byte) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, apperrors.NewDiscovery(
				"failed to create JSON-RPC request",
				apperrors.WithOperation("jsonrpc"),
				apperrors.WithReason("request_build_failed"),
				apperrors.WithHint(i18n.T("请检查服务 endpoint 是否为空或格式不合法。")),
				apperrors.WithCause(&CallError{
					Stage: CallStageRequest,
					Cause: err,
				}),
			)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		// Set security headers for request tracing
		req.Header.Set(HeaderSource, SourceValue)
		if c.ExecutionId != "" {
			req.Header.Set(HeaderExecutionId, c.ExecutionId)
		}
		if token := sanitizeBearerToken(c.AuthToken); token != "" {
			if c.isEndpointTrusted(endpoint) {
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("x-user-access-token", token)
			}
		}
		for key, value := range c.ExtraHeaders {
			if key != "" && value != "" {
				req.Header.Set(key, value)
			}
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
		} else if !retryable(resp.StatusCode) || attempt == c.MaxRetries {
			return resp, nil
		} else {
			lastErr = fmt.Errorf("retryable HTTP %d", resp.StatusCode)
			resp.Body.Close()
		}

		if attempt < c.MaxRetries {
			delay := c.retryDelayForAttempt(attempt, respRetryAfter(resp))
			if err := c.sleepForRetry(ctx, delay); err != nil {
				return nil, apperrors.NewDiscovery(
					"request cancelled during retry",
					apperrors.WithOperation("jsonrpc"),
					apperrors.WithReason("request_cancelled"),
					apperrors.WithHint(i18n.T("请求在重试过程中被取消；请检查调用侧超时设置。")),
					apperrors.WithCause(&CallError{
						Stage: CallStageRequest,
						Cause: err,
					}),
				)
			}
		}
	}
	return nil, apperrors.NewDiscovery(
		fmt.Sprintf("request to %s failed: %v", RedactURL(endpoint), lastErr),
		apperrors.WithOperation("jsonrpc"),
		apperrors.WithReason("request_failed"),
		apperrors.WithRetryable(true),
		apperrors.WithHint(i18n.T("请检查网络连通性和 MCP 服务状态后重试。")),
		apperrors.WithActions(discoveryActions("")...),
		apperrors.WithCause(&CallError{
			Stage: CallStageRequest,
			Cause: lastErr,
		}),
	)
}

func retryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

func respRetryAfter(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	return strings.TrimSpace(resp.Header.Get("Retry-After"))
}

func (c *Client) retryDelayForAttempt(attempt int, retryAfter string) time.Duration {
	base := c.RetryDelay
	if base <= 0 {
		base = 10 * time.Millisecond
	}
	maxDelay := c.RetryMaxDelay
	if maxDelay <= 0 {
		maxDelay = base * 8
	}
	if delay, ok := parseRetryAfter(retryAfter); ok {
		if delay > maxDelay {
			return maxDelay
		}
		if delay > 0 {
			return delay
		}
	}
	delay := base << attempt
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

func (c *Client) sleepForRetry(ctx context.Context, delay time.Duration) error {
	if c != nil && c.sleep != nil {
		return c.sleep(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func parseRetryAfter(raw string) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds > 0 {
		return seconds, true
	}
	deadline, err := http.ParseTime(raw)
	if err != nil {
		return 0, false
	}
	delay := time.Until(deadline)
	if delay < 0 {
		return 0, true
	}
	return delay, true
}

// isEndpointTrusted checks whether the endpoint is HTTPS and belongs to a
// trusted domain. When no auth token is set this is a no-op.
// Set DWS_ALLOW_HTTP_ENDPOINTS=1 for development/testing to allow HTTP, but
// only for loopback addresses (127.0.0.1, ::1, localhost).
// Set DWS_TRUSTED_DOMAINS=* to trust all domains.
func (c *Client) isEndpointTrusted(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		if os.Getenv("DWS_ALLOW_HTTP_ENDPOINTS") != "1" {
			return false
		}
		// Even with the bypass, only allow loopback addresses.
		host := parsed.Hostname()
		if host != "127.0.0.1" && host != "::1" && host != "localhost" {
			return false
		}
	}
	host := parsed.Hostname()
	domains := c.trustedDomainsList()
	if len(domains) == 0 {
		return true
	}
	for _, pattern := range domains {
		if pattern == "*" {
			c.warnWildcardDomains()
			return true
		}
		if matchDomain(host, pattern) {
			return true
		}
	}
	return false
}

func (c *Client) trustedDomainsList() []string {
	if len(c.TrustedDomains) > 0 {
		return c.TrustedDomains
	}
	if envVal := strings.TrimSpace(os.Getenv(trustedDomainsEnv)); envVal != "" {
		return strings.Split(envVal, ",")
	}
	return strings.Split(defaultTrustedDomains, ",")
}

func (c *Client) warnWildcardDomains() {
	c.wildcardOnce.Do(func() {
		w := c.Stderr
		if w == nil {
			w = os.Stderr
		}
		_, _ = fmt.Fprintln(w, "[WARN] DWS_TRUSTED_DOMAINS=* sends bearer token to ANY HTTPS endpoint. Use only for development.")
	})
}

func matchDomain(host, pattern string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // e.g. ".dingtalk.com"
		return strings.HasSuffix(host, suffix) || host == pattern[2:]
	}
	return host == pattern
}

func RedactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	for key := range query {
		query.Set(key, "REDACTED")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func sanitizeBearerToken(raw string) string {
	token := strings.TrimSpace(raw)
	if token == "" {
		return ""
	}
	for _, r := range token {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return token
}

func httpStatusError(method, endpoint string, statusCode int, snapshotPath string) error {
	message := fmt.Sprintf("request to %s returned HTTP %d", RedactURL(endpoint), statusCode)
	opts := []apperrors.Option{
		apperrors.WithOperation(method),
		apperrors.WithReason(fmt.Sprintf("http_%d", statusCode)),
		apperrors.WithRetryable(retryable(statusCode)),
		apperrors.WithSnapshot(snapshotPath),
		apperrors.WithCause(&CallError{
			Stage:      CallStageHTTP,
			HTTPStatus: statusCode,
			Cause:      errors.New(message),
		}),
	}

	switch {
	case statusCode == http.StatusUnauthorized:
		opts = append(opts,
			apperrors.WithHint(i18n.T("认证失败；请检查登录状态或产品 URL 覆盖。")),
			apperrors.WithActions(authActions(snapshotPath)...),
		)
		return apperrors.NewAuth(message, opts...)
	case statusCode == http.StatusForbidden:
		opts = append(opts,
			apperrors.WithHint(i18n.T("权限不足；请检查当前身份是否有权限访问该服务或工具。")),
			apperrors.WithActions(authActions(snapshotPath)...),
		)
		return apperrors.NewAuth(message, opts...)
	case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
		opts = append(opts,
			apperrors.WithHint(i18n.T("请求被上游服务拒绝；请检查参数、认证和权限配置。")),
			apperrors.WithActions(runtimeActions(snapshotPath)...),
		)
		if method != "tools/call" {
			opts = append(opts, apperrors.WithActions(discoveryActions(snapshotPath)...))
			return apperrors.NewDiscovery(message, opts...)
		}
		return apperrors.NewAPI(message, opts...)
	default:
		opts = append(opts,
			apperrors.WithHint(i18n.T("上游服务异常；可稍后重试，若持续失败请查看 recovery snapshot。")),
			apperrors.WithActions(actionsForMethod(method, snapshotPath)...),
		)
		if method == "tools/call" {
			return apperrors.NewAPI(message, opts...)
		}
		return apperrors.NewDiscovery(message, opts...)
	}
}

func jsonrpcEnvelopeError(method string, rpcErr *RPCError, snapshotPath string) error {
	message := fmt.Sprintf("JSON-RPC %s failed with code %d: %s", method, rpcErr.Code, rpcErr.Message)
	reason := reasonForMethod(method, "jsonrpc_"+jsonrpcCodeLabel(rpcErr.Code))
	opts := []apperrors.Option{
		apperrors.WithOperation(method),
		apperrors.WithReason(reason),
		apperrors.WithRPCCode(rpcErr.Code),
		apperrors.WithRPCData(rpcErr.Data),
		apperrors.WithSnapshot(snapshotPath),
		apperrors.WithCause(&CallError{
			Stage:   CallStageJSONRPC,
			RPCCode: rpcErr.Code,
			Cause:   errors.New(message),
		}),
	}

	if rpcErr.Code == -32602 {
		opts = append(opts,
			apperrors.WithHint(i18n.T("参数不符合工具输入 schema；请检查 --json/--params/flags。")),
			apperrors.WithActions(runtimeActions(snapshotPath)...),
		)
		return apperrors.NewValidation(message, opts...)
	}

	if looksAuthRPCError(rpcErr) {
		opts = append(opts,
			apperrors.WithHint(i18n.T("调用被拒绝；请检查认证状态、租户身份或访问权限。")),
			apperrors.WithActions(authActions(snapshotPath)...),
		)
		return apperrors.NewAuth(message, opts...)
	}

	if method == "tools/call" {
		if rpcErr.Code == -32600 || rpcErr.Code == -32601 {
			opts = append(opts,
				apperrors.WithHint(i18n.T("工具协议不兼容；请检查服务版本、工具名或刷新发现缓存。")),
				apperrors.WithActions(discoveryActions(snapshotPath)...),
			)
			return apperrors.NewDiscovery(message, opts...)
		}
		opts = append(opts,
			apperrors.WithHint(i18n.T("工具调用失败；请检查参数和上游服务状态。")),
			apperrors.WithActions(runtimeActions(snapshotPath)...),
		)
		return apperrors.NewAPI(message, opts...)
	}

	opts = append(opts,
		apperrors.WithHint(i18n.T("服务发现/协商失败；请检查网络、服务版本或执行缓存刷新。")),
		apperrors.WithActions(discoveryActions(snapshotPath)...),
	)
	return apperrors.NewDiscovery(message, opts...)
}

func looksAuthRPCError(rpcErr *RPCError) bool {
	if rpcErr == nil {
		return false
	}
	if rpcErr.Code == http.StatusUnauthorized || rpcErr.Code == http.StatusForbidden {
		return true
	}
	return looksAuthRelated(rpcErr.Message)
}

func jsonrpcCodeLabel(code int) string {
	switch code {
	case -32700:
		return "parse_error"
	case -32600:
		return "invalid_request"
	case -32601:
		return "method_not_found"
	case -32602:
		return "invalid_params"
	case -32603:
		return "internal_error"
	default:
		if code < 0 {
			return fmt.Sprintf("server_error_%d", -code)
		}
		return fmt.Sprintf("error_%d", code)
	}
}

func reasonForMethod(method, suffix string) string {
	replacer := strings.NewReplacer("/", "_", "-", "_", " ", "_")
	method = replacer.Replace(strings.TrimSpace(method))
	if method == "" {
		method = "jsonrpc"
	}
	return method + "_" + suffix
}

func looksAuthRelated(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	for _, token := range []string{
		"unauthorized",
		"forbidden",
		"permission",
		"access denied",
		"auth",
		"token",
		"credential",
		"login",
	} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

func actionsForMethod(method, snapshotPath string) []string {
	if method == "tools/call" {
		return runtimeActions(snapshotPath)
	}
	return discoveryActions(snapshotPath)
}

func authActions(snapshotPath string) []string {
	actions := []string{
		i18n.T("检查登录状态后重试"),
	}
	if snapshotPath != "" {
		actions = append(actions, fmt.Sprintf("dws recovery plan --snapshot %s", snapshotPath))
	}
	return actions
}

func runtimeActions(snapshotPath string) []string {
	actions := []string{
		i18n.T("检查认证、权限和参数后重试原命令"),
	}
	if snapshotPath != "" {
		actions = append(actions, fmt.Sprintf("dws recovery plan --snapshot %s", snapshotPath))
	}
	return actions
}

func discoveryActions(snapshotPath string) []string {
	actions := []string{
		"dws cache refresh",
		i18n.T("检查服务连通性和协议版本后重试"),
	}
	if snapshotPath != "" {
		actions = append(actions, fmt.Sprintf("dws recovery plan --snapshot %s", snapshotPath))
	}
	return actions
}
