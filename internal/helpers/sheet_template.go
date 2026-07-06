package helpers

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newSheetTemplateCmd builds the `dws sheet template` command group.
func newSheetTemplateCmd() *cobra.Command {
	templateCmd := &cobra.Command{
		Use:   "template",
		Short: "表格模板管理",
		RunE:  groupRunE,
	}

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

	attachUnknownSubcommandGuard(templateCmd)

	return templateCmd
}
