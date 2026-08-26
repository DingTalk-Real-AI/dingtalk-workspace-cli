package helpers

import (
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

// newSheetTemplateCmd builds the `dws sheet template` command group.
func newSheetTemplateCmd() *cobra.Command {
	templateCmd := newDeepGroupCommand(&cobra.Command{
		Use:   "template",
		Short: "表格模板管理",
		RunE:  groupRunE,
	})

	templateListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取表格模板列表",
		Long:  `获取当前用户可用的表格模板列表，支持按来源筛选。`,
		Example: `  dws sheet template list
  dws sheet template list --source MY
  dws sheet template list --source PUBLIC
  dws sheet template list --limit 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetString("source"); v != "" {
				toolArgs["templateSource"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["maxResults"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token", "next-token"); v != "" {
				toolArgs["nextCursor"] = v
			}
			return callMCPToolOnServer("sheet", "list_sheet_templates", toolArgs)
		},
	}
	DeclareLeafMetadata(templateListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "template_list",
				CanonicalPath:  "sheet.template_list",
				CLIPath:        "sheet template list",
				PrimaryCLIPath: "sheet template list",
			},
			Description: "分页列出可用表格模板。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "分页列出可用表格模板。",
				UseWhen:      []string{"浏览可用表格模板以便 apply 时"},
				AvoidWhen:    []string{"按关键词搜索用 template search；应用模板建新文档用 template apply"},
				Examples:     []string{"dws sheet template list --limit 20"},
			},
		},
	})
	templateListCmd.Flags().String("source", "", "模板来源: MY(我的模版)/PUBLIC(公开模版)，不传默认 MY")
	templateListCmd.Flags().Int("limit", 0, "返回数量上限")
	templateListCmd.Flags().String("cursor", "", "分页游标")

	templateSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索表格模板",
		Long:  `根据关键词搜索表格模板。`,
		Example: `  dws sheet template search --query "预算"
  dws sheet template search --query "排班表" --limit 10
  dws sheet template search --query "财务" --source PUBLIC`,
		RunE: func(cmd *cobra.Command, args []string) error {
			query, _ := cmd.Flags().GetString("query")
			if query == "" {
				query = flagOrFallback(cmd, "keyword", "name")
			}
			if query == "" {
				return fmt.Errorf("flag --query is required")
			}
			toolArgs := map[string]any{
				"searchName": query,
			}
			if v, _ := cmd.Flags().GetString("source"); v != "" {
				toolArgs["templateSource"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["maxResults"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token", "next-token"); v != "" {
				toolArgs["nextCursor"] = v
			}
			return callMCPToolOnServer("sheet", "search_sheet_templates", toolArgs)
		},
	}
	DeclareLeafMetadata(templateSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "template_search",
				CanonicalPath:  "sheet.template_search",
				CLIPath:        "sheet template search",
				PrimaryCLIPath: "sheet template search",
			},
			Description: "按关键词搜索表格模板。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按关键词搜索表格模板。",
				UseWhen:      []string{"用户说找某个主题的表格模板时"},
				AvoidWhen:    []string{"无关键词浏览用 template list；应用用 template apply"},
				Examples:     []string{"dws sheet template search --query \"销售\""},
			},
		},
	})
	templateSearchCmd.Flags().String("query", "", "搜索关键词 (必填)")
	templateSearchCmd.Flags().String("keyword", "", "--query 的别名")
	_ = templateSearchCmd.Flags().MarkHidden("keyword")
	templateSearchCmd.Flags().String("name", "", "--query 的别名")
	_ = templateSearchCmd.Flags().MarkHidden("name")
	templateSearchCmd.Flags().String("source", "", "模板来源: MY(我的模版)/PUBLIC(公开模版)，不传默认 MY")
	templateSearchCmd.Flags().Int("limit", 0, "返回数量上限")
	templateSearchCmd.Flags().String("cursor", "", "分页游标")

	templateApplyCmd := &cobra.Command{
		Use:   "apply",
		Short: "应用表格模板",
		Long:  `使用指定模板创建新表格文档。`,
		Example: `  dws sheet template apply --template-id TPL_ID --name "月度预算表"
  dws sheet template apply --template-id TPL_ID --name "排班表" --folder FOLDER_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			tplID := flagOrFallback(cmd, "template-id", "template", "tpl-id")
			if tplID == "" {
				return fmt.Errorf("flag --template-id is required")
			}
			toolArgs := map[string]any{
				"templateId": tplID,
			}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				toolArgs["name"] = v
			}
			if v, _ := cmd.Flags().GetString("folder"); v != "" {
				toolArgs["folderId"] = v
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("sheet", "apply_sheet_template", toolArgs)
		},
	}
	DeclareLeafMetadata(templateApplyCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "template_apply",
				CanonicalPath:  "sheet.template_apply",
				CLIPath:        "sheet template apply",
				PrimaryCLIPath: "sheet template apply",
			},
			Description: "应用模板创建新的电子表格文档。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "应用模板创建新的电子表格文档。",
				UseWhen:      []string{"已有 templateId，需要据此生成新表格文档时"},
				AvoidWhen:    []string{"空白新建用 sheet create；先搜模板用 search/list"},
				Examples:     []string{"dws sheet template apply --template-id <TEMPLATE_ID> --name \"新表格\""},
			},
		},
	})
	templateApplyCmd.Flags().String("template-id", "", "模板 ID (必填)")
	templateApplyCmd.Flags().String("template", "", "--template-id 的别名")
	_ = templateApplyCmd.Flags().MarkHidden("template")
	templateApplyCmd.Flags().String("tpl-id", "", "--template-id 的别名")
	_ = templateApplyCmd.Flags().MarkHidden("tpl-id")
	templateApplyCmd.Flags().String("name", "", "新表格文档名称 (可选)")
	templateApplyCmd.Flags().String("folder", "", "目标文件夹 ID (可选)")
	templateApplyCmd.Flags().String("parent-id", "", "--folder 的别名")
	_ = templateApplyCmd.Flags().MarkHidden("parent-id")
	templateApplyCmd.Flags().String("workspace", "", "知识库 ID (可选)")
	templateApplyCmd.Flags().String("workspace-id", "", "--workspace 的别名")
	_ = templateApplyCmd.Flags().MarkHidden("workspace-id")

	templateCmd.AddCommand(templateListCmd, templateSearchCmd, templateApplyCmd)
	for _, child := range templateCmd.Commands() {
		RegisterCrossProductAliases(child)
	}

	return templateCmd
}
