package helpers

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newNotifyCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "notify",
		Short: "工作通知 / 消息推送",
		Long:  `管理钉钉工作通知：获取 AgentId、发送、查看进度、撤回。`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("notify is not yet available in this environment")
		},
	}

	agentCmd := &cobra.Command{Use: "agent", Short: "应用 Agent 管理", RunE: groupRunE}

	agentGetIDCmd := &cobra.Command{
		Use:     "get-id",
		Short:   "获取应用的 AgentId",
		Example: `  dws notify agent get-id`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_agent_id", nil)
		},
	}

	messageCmd := &cobra.Command{Use: "message", Short: "通知消息管理", RunE: groupRunE}

	messageSendCmd := &cobra.Command{
		Use:   "send",
		Short: "发送工作通知",
		Example: `  dws notify message send --agent 123 --title "上线通知" --content "v2.0 已发布" --users userId1,userId2
  dws notify message send --agent 123 --title "公告" --content "..." --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "agent", "title", "content"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"agent_id": mustGetFlag(cmd, "agent"),
				"title":    mustGetFlag(cmd, "title"),
				"content":  mustGetFlag(cmd, "content"),
			}
			if v, _ := cmd.Flags().GetString("users"); v != "" {
				toolArgs["userid_list"] = v
			}
			if v, _ := cmd.Flags().GetString("depts"); v != "" {
				toolArgs["dept_id_list"] = v
			}
			if v, _ := cmd.Flags().GetBool("all"); v {
				toolArgs["to_all_user"] = true
			}
			return callMCPTool("send_work_notification", toolArgs)
		},
	}

	messageGetProgressCmd := &cobra.Command{
		Use:     "get-progress",
		Short:   "查看通知发送进度",
		Example: `  dws notify message get-progress --agent 123 --task TASK_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "agent", "task"); err != nil {
				return err
			}
			return callMCPTool("get_notification_send_progress", map[string]any{
				"agent_id": mustGetFlag(cmd, "agent"),
				"task_id":  mustGetFlag(cmd, "task"),
			})
		},
	}

	messageGetStatusCmd := &cobra.Command{
		Use:     "get-status",
		Short:   "查看通知发送结果",
		Example: `  dws notify message get-status --agent 123 --task TASK_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "agent", "task"); err != nil {
				return err
			}
			return callMCPTool("query_work_notification_status", map[string]any{
				"agent_id": mustGetFlag(cmd, "agent"),
				"task_id":  mustGetFlag(cmd, "task"),
			})
		},
	}

	messageRevokeCmd := &cobra.Command{
		Use:     "revoke",
		Short:   "撤回工作通知",
		Example: `  dws notify message revoke --agent 123 --task TASK_ID --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "agent", "task"); err != nil {
				return err
			}
			taskId := mustGetFlag(cmd, "task")
			if !confirmDelete("工作通知", taskId) {
				return nil
			}
			return callMCPTool("revoke_work_notification", map[string]any{
				"agent_id":    mustGetFlag(cmd, "agent"),
				"msg_task_id": taskId,
			})
		},
	}

	agentCmd.AddCommand(agentGetIDCmd)

	messageCmd.PersistentFlags().String("agent", "", "AgentId (必填, 先 get-id 获取)")
	_ = messageCmd.MarkPersistentFlagRequired("agent")
	messageSendCmd.Flags().String("title", "", "通知标题 (必填)")
	messageSendCmd.Flags().String("content", "", "通知内容 (必填)")
	messageSendCmd.Flags().String("users", "", "用户 ID 列表")
	messageSendCmd.Flags().String("depts", "", "部门 ID 列表")
	messageSendCmd.Flags().Bool("all", false, "发送给全员")
	messageGetProgressCmd.Flags().String("task", "", "任务 ID (必填)")
	messageGetStatusCmd.Flags().String("task", "", "任务 ID (必填)")
	messageRevokeCmd.Flags().String("task", "", "任务 ID (必填)")
	messageCmd.AddCommand(messageSendCmd, messageGetProgressCmd, messageGetStatusCmd, messageRevokeCmd)

	root.AddCommand(agentCmd, messageCmd)
	return root
}
