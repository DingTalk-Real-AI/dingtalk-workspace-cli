package helpers

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func newSheetCommentCmd() *cobra.Command {
	commentCmd := &cobra.Command{
		Use:   "comment",
		Short: "表格评论 / 单元格评论管理",
		Long:  `管理钉钉表格的单元格评论：查询评论列表、创建评论、回复评论、更新评论、删除评论。`,
		RunE:  groupRunE,
	}

	commentListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询表格评论列表",
		Example: `  dws sheet comment list --node <SHEET_ID>
  dws sheet comment list --node <SHEET_ID> --sheet-id Sheet1 --range A2
  dws sheet comment list --node <SHEET_ID> --resolve-status unresolved`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			if v, _ := cmd.Flags().GetInt("limit"); cmd.Flags().Changed("limit") {
				toolArgs["pageSize"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "next-token"); v != "" {
				toolArgs["nextToken"] = v
			}
			if v, _ := cmd.Flags().GetString("resolve-status"); v != "" {
				toolArgs["resolveStatus"] = v
			}
			if v, _ := cmd.Flags().GetString("sheet-id"); v != "" {
				toolArgs["sheetId"] = v
			}
			if v, _ := cmd.Flags().GetString("range"); v != "" {
				toolArgs["range"] = v
			}
			return callMCPToolOnServer("doc-comment", "list_sheet_comments", toolArgs)
		},
	}
	DeclareLeafMetadata(commentListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "list_sheet_comments",
				CanonicalPath:  "sheet.list_sheet_comments",
				CLIPath:        "sheet comment list",
				PrimaryCLIPath: "sheet comment list",
			},
			Description: "查询表格单元格评论列表，支持分页与按解决状态过滤",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询表格单元格评论列表，支持分页与按解决状态过滤",
				UseWhen:      []string{"用户说 看某格的评论/表格里有哪些批注"},
				AvoidWhen:    []string{"新建评论用 comment create"},
				Examples:     []string{"dws sheet comment list --node <SHEET_ID> --format json"},
			},
		},
	})
	commentListCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	commentListCmd.Flags().Int("limit", 50, "每页返回的评论数量，默认 50，最大 50")
	commentListCmd.Flags().String("cursor", "", "分页游标")
	commentListCmd.Flags().String("resolve-status", "", "按解决状态过滤: resolved / unresolved")
	commentListCmd.Flags().String("sheet-id", "", "工作表 ID 或名称（与 --range 一起按单元格过滤）")
	commentListCmd.Flags().String("range", "", "单元格位置 A1 表示法（与 --sheet-id 一起按单元格过滤）")

	commentCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建单元格评论",
		Example: `  dws sheet comment create --node <SHEET_ID> --sheet-id Sheet1 --range A2 --content "这个数字有问题"
  dws sheet comment create --node <SHEET_ID> --sheet-id Sheet1 --range A2 --content "请确认" --mention uid1,uid2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "content", "sheet-id", "range"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":  nodeID,
				"content": mustGetFlag(cmd, "content"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"range":   mustGetFlag(cmd, "range"),
			}
			if v, _ := cmd.Flags().GetString("mention"); v != "" {
				toolArgs["mentionedUserIds"] = parseCommentMentionIds(v)
			}
			return callMCPToolOnServer("doc-comment", "create_sheet_comment", toolArgs)
		},
	}
	DeclareLeafMetadata(commentCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "create_sheet_comment",
				CanonicalPath:  "sheet.create_sheet_comment",
				CLIPath:        "sheet comment create",
				PrimaryCLIPath: "sheet comment create",
			},
			Description: "在指定单元格上创建评论，可 @ 用户",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "在指定单元格上创建评论，可 @ 用户",
				UseWhen:      []string{"用户说 给单元格加评论/批注/@某人讨论这个数据"},
				AvoidWhen:    []string{"回复已有评论用 comment reply；不要把评论写进单元格值"},
				Examples:     []string{"dws sheet comment create --node <SHEET_ID> --sheet-id Sheet1 --range A2 --content \"这个数字有问题\" --format json"},
			},
		},
	})
	commentCreateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	commentCreateCmd.Flags().String("content", "", "评论内容 (必填)")
	commentCreateCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	commentCreateCmd.Flags().String("range", "", "单元格位置 A1 表示法 (必填)")
	commentCreateCmd.Flags().String("mention", "", "被 @ 的用户 uid 列表，逗号分隔")

	commentReplyCmd := &cobra.Command{
		Use:   "reply",
		Short: "回复单元格评论",
		Example: `  dws sheet comment reply --node <SHEET_ID> --comment-key <COMMENT_KEY> --content "已核实"
  dws sheet comment reply --node <SHEET_ID> --comment-key <COMMENT_KEY> --content "比心" --emoji`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "content", "comment-key"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":          nodeID,
				"content":         mustGetFlag(cmd, "content"),
				"replyCommentKey": mustGetFlag(cmd, "comment-key"),
			}
			if v, _ := cmd.Flags().GetBool("emoji"); v {
				toolArgs["emoji"] = true
			}
			if v, _ := cmd.Flags().GetString("mention"); v != "" {
				toolArgs["mentionedUserIds"] = parseCommentMentionIds(v)
			}
			return callMCPToolOnServer("doc-comment", "reply_comment", toolArgs)
		},
	}
	DeclareLeafMetadata(commentReplyCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "reply_sheet_comment",
				CanonicalPath:  "sheet.reply_sheet_comment",
				CLIPath:        "sheet comment reply",
				PrimaryCLIPath: "sheet comment reply",
			},
			Description: "回复指定单元格评论，支持表情贴图回复",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "回复指定单元格评论，支持表情贴图回复",
				UseWhen:      []string{"用户说 回复这条评论"},
				AvoidWhen:    []string{"修改评论内容用 comment update"},
				Examples:     []string{"dws sheet comment reply --node <SHEET_ID> --comment-key <COMMENT_KEY> --content \"已核实\" --format json"},
			},
		},
	})
	commentReplyCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	commentReplyCmd.Flags().String("content", "", "回复内容 (必填)")
	commentReplyCmd.Flags().String("comment-key", "", "被回复评论的 commentKey (必填)")
	commentReplyCmd.Flags().Bool("emoji", false, "作为表情贴图回复")
	commentReplyCmd.Flags().String("mention", "", "被 @ 的用户 uid 列表，逗号分隔")

	commentUpdateCmd := &cobra.Command{
		Use:     "update",
		Short:   "更新单元格评论",
		Example: `  dws sheet comment update --node <SHEET_ID> --comment-key <COMMENT_KEY> --content "已按最新数据修正"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "content", "comment-key"); err != nil {
				return err
			}
			return callMCPToolOnServer("doc-comment", "update_comment", map[string]any{
				"nodeId":     nodeID,
				"commentKey": mustGetFlag(cmd, "comment-key"),
				"content":    mustGetFlag(cmd, "content"),
			})
		},
	}
	DeclareLeafMetadata(commentUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "update_sheet_comment",
				CanonicalPath:  "sheet.update_sheet_comment",
				CLIPath:        "sheet comment update",
				PrimaryCLIPath: "sheet comment update",
			},
			Description: "更新单元格评论内容",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新单元格评论内容",
				UseWhen:      []string{"用户说 改一下这条评论"},
				AvoidWhen:    []string{"回复评论用 comment reply"},
				Examples:     []string{"dws sheet comment update --node <SHEET_ID> --comment-key <COMMENT_KEY> --content \"已修正\" --format json"},
			},
		},
	})
	commentUpdateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	commentUpdateCmd.Flags().String("comment-key", "", "待更新评论的 commentKey (必填)")
	commentUpdateCmd.Flags().String("content", "", "更新后的评论内容 (必填)")

	commentDeleteCmd := &cobra.Command{
		Use:     "delete",
		Short:   "删除单元格评论",
		Example: `  dws sheet comment delete --node <SHEET_ID> --comment-key <COMMENT_KEY> --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "comment-key"); err != nil {
				return err
			}
			commentKey := mustGetFlag(cmd, "comment-key")
			return callMCPToolOnServer("doc-comment", "delete_comment", map[string]any{
				"nodeId":     nodeID,
				"commentKey": commentKey,
			})
		},
	}
	DeclareLeafMetadata(commentDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "delete_sheet_comment",
				CanonicalPath:  "sheet.delete_sheet_comment",
				CLIPath:        "sheet comment delete",
				PrimaryCLIPath: "sheet comment delete",
			},
			Description: "删除单元格评论（不可恢复）",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除单元格评论（不可恢复）",
				UseWhen:      []string{"用户明确要求删除某条评论"},
				AvoidWhen:    []string{"标记解决不等于删除"},
				Examples:     []string{"dws sheet comment delete --node <SHEET_ID> --comment-key <COMMENT_KEY> --format json"},
			},
		},
	})
	commentDeleteCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	commentDeleteCmd.Flags().String("comment-key", "", "待删除评论的 commentKey (必填)")

	for _, c := range []*cobra.Command{commentListCmd, commentCreateCmd, commentReplyCmd, commentUpdateCmd, commentDeleteCmd} {
		c.Flags().String("url", "", "")
		c.Flags().String("id", "", "")
		c.Flags().String("node-id", "", "")
		_ = c.Flags().MarkHidden("url")
		_ = c.Flags().MarkHidden("id")
		_ = c.Flags().MarkHidden("node-id")
	}

	commentCmd.AddCommand(commentListCmd, commentCreateCmd, commentReplyCmd, commentUpdateCmd, commentDeleteCmd)
	commentCmd.AddCommand(newCommentBaseCommands("sheet")...)
	return commentCmd
}
