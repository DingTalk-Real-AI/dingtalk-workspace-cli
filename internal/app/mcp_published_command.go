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
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/publishedmcp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type mcpPublishedTransport interface {
	Tools(context.Context, string) (transport.ToolsListResult, error)
	Invoke(context.Context, string, string, map[string]any) (transport.ToolCallResult, error)
}

type mcpPublishedTransportFactory func(context.Context) (mcpPublishedTransport, error)

func newMCPPublishedGroup(caller edition.ToolCaller, factory mcpPublishedTransportFactory) *cobra.Command {
	group := &cobra.Command{
		Use:               "published",
		Short:             "查看和调用已发布的 MCP 工具",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}
	corecmd.ApplyGroupPolicy(group, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	if factory == nil {
		factory = newAuthenticatedMCPPublishedTransportFactory(nil, nil)
	}
	group.AddCommand(
		newMCPPublishedToolsCommand(caller, factory),
		newMCPPublishedInvokeCommand(caller, factory),
	)
	return group
}

func newMCPPublishedToolsCommand(caller edition.ToolCaller, factory mcpPublishedTransportFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "tools <mcpId>",
		Short:             "列出当前身份可用的已发布 MCP 工具",
		Example:           "  dws mcp published tools 2480 --format json",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			mcpID := strings.TrimSpace(args[0])
			endpoint, err := resolvePublishedMCPEndpoint(cmd.Context(), caller, mcpID)
			if err != nil {
				return err
			}
			client, err := factory(cmd.Context())
			if err != nil {
				return err
			}
			tools, err := client.Tools(cmd.Context(), endpoint)
			if err != nil {
				return fmt.Errorf("列出已发布 MCP 工具: %w", err)
			}
			return output.WriteCommandPayload(cmd, map[string]any{
				"mcpId":     mcpID,
				"endpoint":  transport.RedactURL(endpoint),
				"toolCount": len(tools.Tools),
				"tools":     tools.Tools,
			}, output.FormatJSON)
		},
	}
	cli.AnnotateRuntimePositionals(cmd, contract.RuntimeSchemaPositional{
		Name: "mcp_id", Type: "string", Description: "钉钉 MCP 市场中的 mcpId", Required: true, Index: 0,
	})
	helpers.DeclareLeafMetadata(cmd, helpers.LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "medium",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: helpers.LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "mcp", Name: "published_tools", CanonicalPath: "mcp.published_tools",
				CLIPath: "mcp published tools", PrimaryCLIPath: "mcp published tools",
			},
			Description: "按 mcpId 列出当前用户和组织身份可用的已发布 MCP 工具",
			Interface: &contract.InterfaceSpec{
				Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable,
				Reason: "Resolves a per-identity endpoint through mcp-meta, then calls the selected published server's tools/list method without creating dynamic Cobra commands.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按 mcpId 查看当前身份可调用的已发布 MCP 工具及输入 Schema",
				UseWhen:      []string{"已知 mcpId，需要查看该服务实际发布的工具和参数"},
				AvoidWhen:    []string{"创建或修改 MCP 服务与工具时使用 dev mcp"},
				Examples:     []string{"dws mcp published tools 2480 --format json"},
			},
		},
	})
	return cmd
}

func newMCPPublishedInvokeCommand(caller edition.ToolCaller, factory mcpPublishedTransportFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "invoke <mcpId> <tool>",
		Short:             "调用当前身份可用的已发布 MCP 工具",
		Long:              "调用指定 mcpId 下的已发布工具。由于远端工具的副作用无法由静态 CLI Contract 判断，真实调用一律需要显式确认。",
		Example:           `  dws mcp published invoke 2480 search --params '{"query":"example"}' --dry-run --format json`,
		Args:              cobra.ExactArgs(2),
		DisableAutoGenTag: true,
		RunE:              runMCPPublishedInvoke(caller, factory),
	}
	cmd.Flags().String("params", "{}", "工具参数 JSON 对象")
	cli.AnnotateRuntimePositionals(cmd,
		contract.RuntimeSchemaPositional{
			Name: "mcp_id", Type: "string", Description: "钉钉 MCP 市场中的 mcpId", Required: true, Index: 0,
		},
		contract.RuntimeSchemaPositional{
			Name: "tool", Type: "string", Description: "tools 命令返回的已发布工具名", Required: true, Index: 1,
		},
	)
	helpers.DeclareLeafMetadata(cmd, helpers.LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, _, _, err := parseMCPPublishedInvoke(cmd, args)
			return err
		},
		Contract: helpers.LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "mcp", Name: "published_invoke", CanonicalPath: "mcp.published_invoke",
				CLIPath: "mcp published invoke", PrimaryCLIPath: "mcp published invoke",
			},
			Description: "经确认后按 mcpId 和工具名调用当前身份可用的已发布 MCP 工具",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewInvocation, RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable,
				Reason: "Resolves a per-identity endpoint through mcp-meta and invokes an arbitrary published server tool; the static wrapper therefore cannot bind one pinned interface_ref or infer the remote tool's effect.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "经用户确认后调用指定 mcpId 下的已发布 MCP 工具",
				UseWhen:      []string{"已通过 mcp published tools 确认工具名和参数，并需要执行该工具"},
				AvoidWhen:    []string{"尚未核对工具 Schema 或无法确认远端副作用时不要真实执行"},
				Examples:     []string{`dws mcp published invoke 2480 search --params '{"query":"example"}' --dry-run --format json`},
			},
		},
	})
	return cmd
}

func runMCPPublishedInvoke(caller edition.ToolCaller, factory mcpPublishedTransportFactory) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		mcpID, tool, params, err := parseMCPPublishedInvoke(cmd, args)
		if err != nil {
			return err
		}
		if corecmd.BoolFlag(cmd, "dry-run") {
			return output.WriteCommandPayload(cmd, map[string]any{
				"kind":      "helper_invocation",
				"dry_run":   true,
				"executed":  false,
				"product":   "mcp",
				"mcpId":     mcpID,
				"tool":      tool,
				"arguments": params,
			}, output.FormatJSON)
		}
		endpoint, err := resolvePublishedMCPEndpoint(cmd.Context(), caller, mcpID)
		if err != nil {
			return err
		}
		client, err := factory(cmd.Context())
		if err != nil {
			return err
		}
		result, err := client.Invoke(cmd.Context(), endpoint, tool, params)
		if err != nil {
			return fmt.Errorf("调用已发布 MCP 工具: %w", err)
		}
		if result.IsError {
			return apperrors.NewAPI(
				"已发布 MCP 工具返回错误: "+extractMCPErrorMessage(result),
				apperrors.WithReason("published_mcp_tool_error"),
			)
		}
		return output.WriteCommandPayload(cmd, map[string]any{
			"mcpId":    mcpID,
			"tool":     tool,
			"endpoint": transport.RedactURL(endpoint),
			"result":   result,
		}, output.FormatJSON)
	}
}

func parseMCPPublishedInvoke(cmd *cobra.Command, args []string) (string, string, map[string]any, error) {
	if len(args) != 2 {
		return "", "", nil, apperrors.NewValidation("需要提供 mcpId 和工具名")
	}
	mcpID := strings.TrimSpace(args[0])
	tool := strings.TrimSpace(args[1])
	if mcpID == "" {
		return "", "", nil, apperrors.NewValidation("mcpId 不能为空")
	}
	if tool == "" {
		return "", "", nil, apperrors.NewValidation("工具名不能为空")
	}
	raw, err := cmd.Flags().GetString("params")
	if err != nil {
		return "", "", nil, err
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &params); err != nil || params == nil {
		return "", "", nil, apperrors.NewValidation("--params 必须是 JSON 对象")
	}
	return mcpID, tool, params, nil
}

func resolvePublishedMCPEndpoint(ctx context.Context, caller edition.ToolCaller, mcpID string) (string, error) {
	if caller == nil {
		return "", fmt.Errorf("MCP tool caller is not configured")
	}
	mcpID = strings.TrimSpace(mcpID)
	if mcpID == "" {
		return "", apperrors.NewValidation("mcpId 不能为空")
	}
	result, err := caller.CallTool(ctx, mcpMetaServerID, mcpMetaURLTool, map[string]any{"mcpId": mcpID})
	if err != nil {
		return "", fmt.Errorf("获取 MCP 服务地址: %w", err)
	}
	if result == nil {
		return "", fmt.Errorf("MCP 元服务返回空结果")
	}
	for _, block := range result.Content {
		if block.Type != "text" || strings.TrimSpace(block.Text) == "" {
			continue
		}
		if err := apperrors.ClassifyMCPResponseText(block.Text); err != nil {
			return "", err
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(block.Text), &payload); err != nil {
			return "", fmt.Errorf("MCP 元服务返回了无效 JSON: %w", err)
		}
		endpoint := stringMapValue(payload, "mcpURL")
		if nested, ok := payload["result"].(map[string]any); ok {
			endpoint = firstNonEmpty(stringMapValue(nested, "mcpURL"), endpoint)
		}
		if endpoint == "" {
			return "", fmt.Errorf("MCP 元服务响应缺少 mcpURL")
		}
		return endpoint, nil
	}
	return "", fmt.Errorf("MCP 元服务返回空结果")
}

func newAuthenticatedMCPPublishedTransportFactory(runner executor.Runner, flags *GlobalFlags) mcpPublishedTransportFactory {
	return func(ctx context.Context) (mcpPublishedTransport, error) {
		base, token, headers, err := resolveMCPPublishedClientConfig(ctx, runner, flags)
		if err != nil {
			return nil, err
		}
		return publishedmcp.New(base, token, headers), nil
	}
}

func resolveMCPPublishedClientConfig(
	ctx context.Context,
	runner executor.Runner,
	flags *GlobalFlags,
) (*transport.Client, string, map[string]string, error) {
	explicitToken := ""
	if flags != nil {
		explicitToken = flags.Token
	}
	token, err := resolveRuntimeAuthToken(ctx, explicitToken)
	if err != nil {
		return nil, "", nil, err
	}
	var base *transport.Client
	if runtime, ok := runner.(*runtimeRunner); ok {
		base = runtime.transport
	}
	return base, token, resolveIdentityHeaders(), nil
}

func stringMapValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
