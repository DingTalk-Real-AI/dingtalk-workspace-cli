package helpers

import (
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws blackboard — 公告
// MCP 工具名: list_user_blackboards, create_blackboard
// ──────────────────────────────────────────────────────────

func newBlackboardCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "blackboard",
		Short: "企业公告管理",
		Long:  `管理钉钉企业公告：查询公告列表、创建并发送公告。`,
		RunE:  groupRunE,
	}

	// ── dws blackboard list ──────────────────────────────
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "查询用户公告列表",
		Example: `  dws blackboard list
  dws blackboard list --unread
  dws blackboard list --start "2026-05-10T00:00:00+08:00" --end "2026-05-18T23:59:59+08:00"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}

			if v, _ := cmd.Flags().GetString("start"); v != "" {
				ms, err := parseISOTimeToMillis("start", v)
				if err != nil {
					return err
				}
				toolArgs["startTime"] = ms
			}
			if v, _ := cmd.Flags().GetString("end"); v != "" {
				ms, err := parseISOTimeToMillis("end", v)
				if err != nil {
					return err
				}
				toolArgs["endTime"] = ms
			}
			if v, _ := cmd.Flags().GetBool("unread"); v {
				toolArgs["readStatus"] = "0"
			}
			return callMCPTool("list_user_blackboards", toolArgs)
		},
	}

	listCmd.Flags().String("start", "", "起始时间 ISO-8601 (如 2026-05-10T00:00:00+08:00)")
	listCmd.Flags().String("end", "", "结束时间 ISO-8601 (如 2026-05-18T23:59:59+08:00)")
	listCmd.Flags().Bool("unread", false, "仅显示未读公告")

	// ── dws blackboard create ────────────────────────────
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "[危险] 创建并发送公告(全员，不可撤回)",
		Example: `  dws blackboard create --title "系统升级通知" --content "<p>今晚22点系统维护</p>"
  dws blackboard create --title "重要公告" --content "<p>内容</p>" --push-top --send-ding`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "title", "content"); err != nil {
				return err
			}

			toolArgs := map[string]any{
				"title":   mustGetFlag(cmd, "title"),
				"content": mustGetFlag(cmd, "content"),
				"receivers": map[string]any{
					"deptIds": []string{"-1"},
				},
			}

			if v, _ := cmd.Flags().GetBool("push-top"); v {
				toolArgs["isPushTop"] = true
			}
			if v, _ := cmd.Flags().GetBool("send-ding"); v {
				toolArgs["isSendDing"] = true
			}
			if v, _ := cmd.Flags().GetBool("send-todo"); v {
				toolArgs["sendTodoTask"] = true
			}

			return callMCPTool("create_blackboard", toolArgs)
		},
	}

	createCmd.Flags().String("title", "", "公告标题 (必填)")
	createCmd.Flags().String("content", "", "公告正文, 支持HTML富文本 (必填)")
	createCmd.Flags().Bool("push-top", false, "是否置顶")
	createCmd.Flags().Bool("send-ding", false, "是否发送DING通知")
	createCmd.Flags().Bool("send-todo", false, "是否给接收人发起待办")

	root.AddCommand(listCmd, createCmd)

	root.AddCommand(
		hintSubCmd("get", "use: dws blackboard list"),
		hintSubCmd("send", "use: dws blackboard create"),
	)

	return root
}
