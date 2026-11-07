package helpers

import (
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func newSheetVersionCmd() *cobra.Command {
	versionCmd := newGroupCommand(&cobra.Command{
		Use:   "version",
		Short: "表格历史版本管理",
		Long:  `管理钉钉在线电子表格的历史版本：手动保存、查看版本列表、回滚到指定版本。`,
		RunE:  groupRunE,
	})

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
	DeclareLeafMetadata(versionSaveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "version_save",
				CanonicalPath:  "sheet.version_save",
				CLIPath:        "sheet version save",
				PrimaryCLIPath: "sheet version save",
			},
			Description: "手动保存表格版本快照",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "手动保存表格版本快照",
				UseWhen:      []string{"用户说 保存版本/存个快照，目标是在线表格"},
				AvoidWhen:    []string{"回滚用 version revert；查看历史用 version list"},
				Examples:     []string{"dws sheet version save --node <SHEET_ID> --format json"},
			},
		},
	})
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
	DeclareLeafMetadata(versionListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "version_list",
				CanonicalPath:  "sheet.version_list",
				CLIPath:        "sheet version list",
				PrimaryCLIPath: "sheet version list",
			},
			Description: "查看表格历史版本列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查看表格历史版本列表",
				UseWhen:      []string{"用户说 看历史版本/版本列表，目标是在线表格"},
				AvoidWhen:    []string{"回滚用 version revert"},
				Examples:     []string{"dws sheet version list --node <SHEET_ID> --limit 10 --format json"},
			},
		},
	})
	versionListCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	versionListCmd.Flags().Int("limit", 0, "返回版本数量上限")
	versionListCmd.Flags().String("cursor", "", "分页游标")

	versionRevertCmd := &cobra.Command{
		Use:   "revert",
		Short: "[危险] 回滚表格到指定历史版本或 revision",
		Long: `将在线表格恢复到指定历史版本或精确 revision。

通常应从 version list 选择已保存的历史版本。用户明确要求恢复到某个精确 revision 时，
也可传入已从同一工作簿真实查询结果确认的 revision，即使它不在版本列表中。未列入
版本列表的 revision 只有在服务端仍可恢复时才能成功；禁止猜测 revision。`,
		Example: `  dws sheet version revert --node SHEET_ID --version 3`,
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
	DeclareLeafMetadata(versionRevertCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "version_revert",
				CanonicalPath:  "sheet.version_revert",
				CLIPath:        "sheet version revert",
				PrimaryCLIPath: "sheet version revert",
			},
			Description: "回滚表格到指定历史版本或已确认的精确 revision",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "回滚表格到指定历史版本或已确认的精确 revision",
				UseWhen:      []string{"用户说 回滚到某个版本/恢复到之前的表格，或明确要求恢复到同一工作簿中已确认的 revision"},
				AvoidWhen:    []string{"普通文件回滚用 drive revert；在线文档用 doc version revert；目标 revision 未经同一工作簿的真实查询结果确认时不要猜测"},
				Examples:     []string{"dws sheet version revert --node <SHEET_ID> --version 3 --format json"},
			},
		},
	})
	versionRevertCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	versionRevertCmd.Flags().Int("version", 0, "目标历史版本或已确认 revision (必填，通常从 version list 获取)")

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
