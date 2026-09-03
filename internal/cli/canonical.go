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

package cli

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// schemaCommandCatalogError / payloads use deliverySchemaCatalog
// (RegisterSchemaSourceRoot → ResolveSchemaBuild). There is no committed
// Schema Catalog embed fallback.
var schemaCommandCatalogError = deliverySchemaCatalogError

// NewMCPCommand registers the mcp product declaration and returns its root
// command. The app layer attaches reviewed static MCP helpers as
// subcommands; live discovery surfaces are retired.
func NewMCPCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/mcp.json
	// products.mcp). Schema assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "mcp",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-shared"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("Schema 与 MCP 使用指南", "dingtalk-shared", "references/schema-usage.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "解析当前身份可用的 MCP 服务连接信息，并静态查看或调用已发布工具",
			UseWhen: []string{
				"需要把钉钉 MCP 市场中的服务连接到支持 Streamable HTTP 的 Agent 或客户端",
				"已知 mcpId，需要查看或调用当前身份可用的已发布 MCP 工具",
			},
			AvoidWhen: []string{
				"查询普通钉钉业务数据时使用对应产品命令，不要使用 mcp",
			},
		},
	})
	// The legacy dynamic MCP surface has been removed; the app layer adds
	// reviewed static MCP helpers as subcommands of this command.
	cmd := &cobra.Command{
		Use:               "mcp",
		Short:             "管理和调用已发布 MCP 服务",
		Long:              "管理经过审核并纳入 Schema 的 MCP 服务连接辅助能力，并通过静态 published 子命令查看或调用已发布工具。",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	return cmd
}

// NewSchemaCommand serves the typed Schema contract. Production assembles from
// declarations via ResolveSchemaBuild (factory registered by internal/app).
// A malformed assembly fails closed; the command takes no discovery loader
// because queries never run service discovery.
func NewSchemaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema [path]",
		Short: "渐进查看命令 Schema (产品 / 分组 / 工具参数)",
		Long: `查看当前可运行命令的 Schema 元数据。

不带参数时列出产品和工具数量；传产品或分组路径逐层展开；传具体工具路径输出扁平参数 Schema（对齐 GWS：parameters 内联 required，键为 CLI flag）。普通 Agent 查询应使用 --compact：它按稳定字段白名单输出选参、约束、安全语义和已评审的返回契约。省略 --compact 的 full leaf 保留参数映射、接口绑定和 provenance，仅用于定向审计；--all 输出全部工具的完整 leaf Schema，用于审计/CI。helper、MCP 与本地 Cobra 命令均须通过 ContractFinal.Identity 声明进入收集的身份集，并从同一声明装配的 ToolSpec 投影；查询不执行服务发现或临时合成第二份 Schema。`,
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			compact, _ := cmd.Flags().GetBool("compact")
			cliPath, _ := cmd.Flags().GetString("cli-path")
			cliPath = strings.TrimSpace(cliPath)
			if cliPath != "" && len(args) > 0 {
				return apperrors.NewValidation("--cli-path and positional argument are mutually exclusive")
			}
			if len(args) == 1 && strings.EqualFold(strings.TrimSpace(args[0]), "list") {
				args = nil
			}
			if all && (cliPath != "" || len(args) > 0) {
				return apperrors.NewValidation("--all cannot be combined with a schema path")
			}
			if cliPath != "" {
				args = []string{cliPath}
			}
			if err := schemaCommandCatalogError(); err != nil {
				return fmt.Errorf("load typed Schema registry: %w", err)
			}
			var payload map[string]any
			var err error
			if all {
				payload, err = deliverySchemaAllPayload()
			} else if len(args) == 0 {
				payload, err = deliverySchemaOverviewPayload()
			} else {
				payload, err = queryDeliverySchemaPayload(args)
			}
			if err != nil {
				return err
			}
			if compact {
				payload = stripSchemaPayloadCompact(payload)
			}
			return output.WriteFiltered(cmd.OutOrStdout(), output.ResolveFormat(cmd, output.FormatJSON), payload, output.ResolveFields(cmd), output.ResolveJQ(cmd))
		},
	}
	cmd.Flags().Bool("all", false, "输出全部工具的完整 leaf Schema（包括参数和约束，用于审计/CI）")
	cmd.Flags().Bool("compact", false, "按稳定字段白名单输出 Agent 选参、约束、安全语义和返回契约")
	cmd.Flags().String("cli-path", "", "按 CLI 命令路径查询")
	return cmd
}

// splitSchemaPathTokens splits a CLI path on dots, slashes, and
// whitespace, returning only non-empty tokens.
func splitSchemaPathTokens(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '.' || r == '/' || r == ' ' || r == '\t'
	})
	out := fields[:0]
	for _, f := range fields {
		if s := strings.TrimSpace(f); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// normalizeSchemaQueryCLIPath accepts the historical query spellings while
// keeping authored Registry CLI paths strict and space-separated. Canonical
// identity lookup still runs before this compatibility normalization.
func normalizeSchemaQueryCLIPath(path string) string {
	parts := splitSchemaPathTokens(strings.TrimSpace(path))
	if len(parts) > 0 && parts[0] == "dws" {
		parts = parts[1:]
	}
	return strings.Join(parts, " ")
}
