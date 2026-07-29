package helpers

import (
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
	return commentCmd
}
