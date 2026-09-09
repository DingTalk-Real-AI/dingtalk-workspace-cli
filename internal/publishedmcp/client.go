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

// Package publishedmcp owns runtime protocol access to published MCP servers.
// Command construction and Schema assembly depend only on its static Client
// methods and never perform discovery during startup.
package publishedmcp

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/mcpschema"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

const (
	maxToolListPages        = 100
	maxToolListPayloadBytes = 20 << 20
	maxToolListCursorBytes  = 64 << 10
	maxToolListTools        = 10_000
)

var digestInputSchema = mcpschema.Digest

type Client struct {
	transport       *transport.Client
	invokeTransport *transport.Client
}

// ValidatedInvocationResult is evidence from the schema snapshot used by one
// discover-validate-call operation plus the arbitrary remote call result.
type ValidatedInvocationResult struct {
	InputSchemaValidation string
	InputSchemaDigest     string
	Result                transport.ToolCallResult
}

func New(base *transport.Client, token string, headers map[string]string) *Client {
	if base == nil {
		base = transport.NewClient(nil)
	}
	authenticated := base.WithAuth(token, headers)
	discovery := authenticated.WithRedirectsDisabled()
	return &Client{
		transport:       discovery,
		invokeTransport: discovery.WithMaxRetries(0),
	}
}

func (c *Client) Tools(ctx context.Context, endpoint string) (transport.ToolsListResult, error) {
	return c.tools(ctx, endpoint, maxToolListPages, maxToolListPayloadBytes)
}

func (c *Client) tools(ctx context.Context, endpoint string, maxPages, maxPayloadBytes int) (transport.ToolsListResult, error) {
	return c.toolsWithLimits(ctx, endpoint, maxPages, maxPayloadBytes, maxToolListCursorBytes, maxToolListTools)
}

func (c *Client) toolsWithLimits(ctx context.Context, endpoint string, maxPages, maxPayloadBytes, maxCursorBytes, maxTools int) (transport.ToolsListResult, error) {
	if err := c.transport.ValidateTrustedEndpoint(endpoint); err != nil {
		return transport.ToolsListResult{}, err
	}
	var aggregate transport.ToolsListResult
	cursor := ""
	seenCursors := map[[sha256.Size]byte]struct{}{}
	aggregateBytes := 0
	for page := 1; page <= maxPages; page++ {
		result, err := c.transport.ListToolsPage(ctx, endpoint, cursor)
		if err != nil {
			return transport.ToolsListResult{}, err
		}
		aggregateBytes, err = appendToolPage(&aggregate, result, aggregateBytes, maxPayloadBytes, maxCursorBytes, maxTools)
		if err != nil {
			return transport.ToolsListResult{}, err
		}
		nextCursor := result.NextCursor
		if nextCursor == "" {
			return aggregate, nil
		}
		cursorDigest := sha256.Sum256([]byte(nextCursor))
		if _, exists := seenCursors[cursorDigest]; exists {
			return transport.ToolsListResult{}, apperrors.NewDiscovery(
				fmt.Sprintf("tools/list returned repeated cursor after page %d", page),
			)
		}
		seenCursors[cursorDigest] = struct{}{}
		cursor = nextCursor
	}
	return transport.ToolsListResult{}, apperrors.NewDiscovery(
		fmt.Sprintf("tools/list exceeded the %d-page safety limit", maxPages),
	)
}

func appendToolPage(aggregate *transport.ToolsListResult, page transport.ToolsListResult, currentBytes, maxBytes, maxCursorBytes, maxTools int) (int, error) {
	pageBytes := page.RawResponseBytes
	if pageBytes <= 0 {
		pageBytes = page.RawResultBytes
	}
	if pageBytes <= 0 {
		return currentBytes, apperrors.NewDiscovery("tools/list result is missing its raw byte measurement")
	}
	if currentBytes > maxBytes || pageBytes > maxBytes-currentBytes {
		return currentBytes, apperrors.NewDiscovery(
			fmt.Sprintf("tools/list exceeded the %d-byte aggregate safety limit", maxBytes),
		)
	}
	if len(page.NextCursor) > maxCursorBytes {
		return currentBytes, apperrors.NewDiscovery(
			fmt.Sprintf("tools/list cursor exceeded the %d-byte safety limit", maxCursorBytes),
		)
	}
	if len(aggregate.Tools) > maxTools || len(page.Tools) > maxTools-len(aggregate.Tools) {
		return currentBytes, apperrors.NewDiscovery(
			fmt.Sprintf("tools/list exceeded the %d-tool aggregate safety limit", maxTools),
		)
	}

	aggregate.Tools = append(aggregate.Tools, page.Tools...)
	aggregate.RawResponseBytes = currentBytes + pageBytes
	aggregate.RawResultBytes += page.RawResultBytes
	return aggregate.RawResponseBytes, nil
}

// InvokeValidated performs complete fresh discovery, exact unique tool
// selection, bounded fail-closed input validation, and one zero-retry call on
// the same resolved endpoint.
func (c *Client) InvokeValidated(ctx context.Context, endpoint, tool string, arguments map[string]any) (ValidatedInvocationResult, error) {
	tools, err := c.Tools(ctx, endpoint)
	if err != nil {
		return ValidatedInvocationResult{}, fmt.Errorf("发现已发布 MCP 工具: %w", err)
	}
	inputSchema, matches := exactToolInputSchema(tools, tool)
	if matches == 0 {
		return ValidatedInvocationResult{}, apperrors.NewValidation(
			fmt.Sprintf("已发布 MCP 中不存在工具 %q", tool),
			apperrors.WithReason("published_mcp_tool_not_found"),
			apperrors.WithHint("先执行 dws mcp published tools <mcpId> --format json 获取当前身份可用的实时工具列表"),
		)
	}
	if matches > 1 {
		return ValidatedInvocationResult{}, apperrors.NewValidation(
			fmt.Sprintf("已发布 MCP 返回了 %d 个同名工具 %q，无法唯一确定实时 inputSchema", matches, tool),
			apperrors.WithReason("published_mcp_tool_ambiguous"),
			apperrors.WithHint("停止调用并检查 MCP 服务的已发布工具定义，确保工具名唯一"),
		)
	}
	if len(inputSchema) == 0 {
		return ValidatedInvocationResult{}, apperrors.NewValidation(
			fmt.Sprintf("已发布 MCP 工具 %q 未提供 inputSchema，无法安全校验参数", tool),
			apperrors.WithReason("published_mcp_input_schema_unavailable"),
			apperrors.WithHint("先执行 dws mcp published tools <mcpId> --format json 核对服务返回的工具 Schema"),
		)
	}
	if err := mcpschema.ValidateInputSchema(arguments, inputSchema); err != nil {
		return ValidatedInvocationResult{}, err
	}
	digest, err := digestInputSchema(inputSchema)
	if err != nil {
		return ValidatedInvocationResult{}, err
	}
	result, err := c.invokeTransport.CallTool(ctx, endpoint, tool, arguments)
	if err != nil {
		return ValidatedInvocationResult{}, publishedMCPInvocationError(err)
	}
	return ValidatedInvocationResult{
		InputSchemaValidation: mcpschema.ValidationEvidence,
		InputSchemaDigest:     digest,
		Result:                result,
	}, nil
}

func publishedMCPInvocationError(err error) error {
	var typed *apperrors.Error
	if errors.As(err, &typed) {
		switch typed.RPCCode {
		case -32700, -32600, -32601, -32602:
			started := false
			typed.ExecutionStarted = &started
			typed.Retryable = false
			typed.RetryableSet = true
			typed.Actions = nil
			return err
		}
		if typed.Category == apperrors.CategoryValidation || (typed.ExecutionStarted != nil && !*typed.ExecutionStarted) {
			return err
		}
	}
	return apperrors.NewAPI(
		"published MCP tool execution result is unknown: "+err.Error(),
		apperrors.WithOperation("tools/call"),
		apperrors.WithFailureStage("execution"),
		apperrors.WithReason("published_mcp_execution_unknown"),
		apperrors.WithRetryable(false),
		apperrors.WithHint("请求可能已到达服务端；先从业务侧核实结果，不能盲目重放。"),
		apperrors.WithCause(err),
	)
}

func exactToolInputSchema(tools transport.ToolsListResult, toolName string) (map[string]any, int) {
	var inputSchema map[string]any
	matches := 0
	for _, tool := range tools.Tools {
		if tool.Name == toolName {
			inputSchema = tool.InputSchema
			matches++
		}
	}
	return inputSchema, matches
}
