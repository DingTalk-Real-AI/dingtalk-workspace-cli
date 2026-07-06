package helpers

import (
	"github.com/spf13/cobra"
)

func newWorkbenchCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "workbench",
		Short: "工作台应用查询",
		Long:  `查询钉钉工作台：列出所有工作台应用、批量获取应用详情。`,
		RunE:  groupRunE,
	}

	appCmd := &cobra.Command{Use: "app", Short: "应用管理", RunE: groupRunE}

	appListCmd := &cobra.Command{
		Use:     "list",
		Short:   "查看所有工作台应用",
		Example: `  dws workbench app list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_user_workspace_apps", map[string]any{
				"input": "fromCLI",
			})
		},
	}

	appGetCmd := &cobra.Command{
		Use:     "get",
		Short:   "批量获取应用详情",
		Example: `  dws workbench app get --ids app1,app2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "ids"); err != nil {
				return err
			}
			return callMCPTool("batch_get_app_details", map[string]any{
				"appIds": parseCSVValues(mustGetFlag(cmd, "ids")),
			})
		},
	}

	appGetCmd.Flags().String("ids", "", "应用 ID 列表 (必填)")
	appCmd.AddCommand(appListCmd, appGetCmd)
	root.AddCommand(appCmd)
	return root
}
