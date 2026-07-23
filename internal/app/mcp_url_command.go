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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

const (
	mcpMetaServerID = "mcp-meta"
	mcpMetaURLTool  = "get_mcp_server_url"
)

func newMCPURLGroup(caller edition.ToolCaller) *cobra.Command {
	group := &cobra.Command{
		Use:               "url",
		Short:             "管理 MCP 服务连接地址",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	group.AddCommand(newMCPURLGetCommand(caller))
	return group
}

func newMCPURLGetCommand(caller edition.ToolCaller) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <mcpId>",
		Short: "按 mcpId 获取 MCP 的 Streamable HTTP 服务地址",
		Long: "输入 MCP 市场 mcpId，返回以当前用户和组织身份访问该 MCP 的 " +
			"Streamable HTTP 服务地址。\n\n" +
			"安全提示：返回的 mcpURL 和 mcpJSON 可能包含身份凭据，仅限个人使用，" +
			"请勿分享到群聊、文档、邮件、代码仓库或日志。",
		Example: "  dws mcp url get 2480\n" +
			"  dws mcp url get 2480 --format json",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if caller == nil {
				return fmt.Errorf("MCP tool caller is not configured")
			}
			mcpID := strings.TrimSpace(args[0])
			if mcpID == "" {
				return fmt.Errorf("mcpId 不能为空")
			}

			result, err := caller.CallTool(cmd.Context(), mcpMetaServerID, mcpMetaURLTool, map[string]any{
				"mcpId": mcpID,
			})
			if err != nil {
				return fmt.Errorf("获取 MCP 服务地址: %w", err)
			}
			return writeMCPURLResult(cmd, result)
		},
	}
	cli.AnnotateRuntimePositionals(cmd, cli.RuntimeSchemaPositional{
		Name:        "mcp_id",
		Type:        "string",
		Description: "钉钉 MCP 市场中的 mcpId",
		Required:    true,
		Index:       0,
	})
	return cmd
}

func writeMCPURLResult(cmd *cobra.Command, result *edition.ToolResult) error {
	if result == nil {
		return fmt.Errorf("MCP 元服务返回空结果")
	}
	// get_mcp_server_url returns one JSON document in its first non-empty text
	// block. Other block types and trailing blocks are intentionally ignored.
	for _, block := range result.Content {
		if block.Type != "text" || strings.TrimSpace(block.Text) == "" {
			continue
		}
		if err := apperrors.ClassifyMCPResponseText(block.Text); err != nil {
			return err
		}

		var payload any
		if err := json.Unmarshal([]byte(block.Text), &payload); err != nil {
			return fmt.Errorf("MCP 元服务返回了无效 JSON: %w", err)
		}
		return output.WriteCommandPayload(cmd, payload, output.FormatJSON)
	}
	return fmt.Errorf("MCP 元服务返回空结果")
}
