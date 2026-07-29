package helpers

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSheetVersionCmd() *cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "表格历史版本管理",
		Long:  `管理钉钉在线电子表格的历史版本：手动保存、查看版本列表、回滚到指定版本。`,
		RunE:  groupRunE,
	}

	versionSaveCmd := &cobra.Command{
		Use:     "save",
		Short:   "手动保存表格版本快照",
		Example: `  dws sheet version save --node SHEET_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("doc", "save_doc_version", map[string]any{
				"nodeId": nodeID,
			})
		},
	}
	versionSaveCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")

	versionListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "查看表格历史版本列表",
		Example: `  dws sheet version list --node SHEET_ID
  dws sheet version list --node SHEET_ID --limit 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["maxResults"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token", "next-token"); v != "" {
				toolArgs["nextCursor"] = v
			}
			return callMCPToolOnServer("doc", "list_doc_versions", toolArgs)
		},
	}
	versionListCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	versionListCmd.Flags().Int("limit", 0, "返回版本数量上限")
	versionListCmd.Flags().String("cursor", "", "分页游标")

	versionRevertCmd := &cobra.Command{
		Use:     "revert",
		Short:   "[危险] 回滚表格到指定版本",
		Example: `  dws sheet version revert --node SHEET_ID --version 3 --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("version") {
				return fmt.Errorf("flag --version is required")
			}
			version, _ := cmd.Flags().GetInt("version")
			return callMCPToolOnServer("doc", "revert_doc_version", map[string]any{
				"nodeId":  nodeID,
				"version": version,
			})
		},
	}
	versionRevertCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	versionRevertCmd.Flags().Int("version", 0, "目标版本号 (必填，从 list 获取)")

	for _, c := range []*cobra.Command{versionSaveCmd, versionListCmd, versionRevertCmd} {
		c.Flags().String("url", "", "")
		c.Flags().String("id", "", "")
		c.Flags().String("node-id", "", "")
		c.Flags().String("doc-id", "", "")
		c.Flags().String("file-id", "", "")
		_ = c.Flags().MarkHidden("url")
		_ = c.Flags().MarkHidden("id")
		_ = c.Flags().MarkHidden("node-id")
		_ = c.Flags().MarkHidden("doc-id")
		_ = c.Flags().MarkHidden("file-id")
	}

	versionCmd.AddCommand(versionSaveCmd, versionListCmd, versionRevertCmd)
	return versionCmd
}
