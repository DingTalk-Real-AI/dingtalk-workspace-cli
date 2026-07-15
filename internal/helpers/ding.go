package helpers

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws ding — DING 消息
// ──────────────────────────────────────────────────────────

// remindType: 服务端 API 1=应用内 2=短信 3=电话
var dingRemindTypeMap = map[string]int{"app": 1, "sms": 2, "call": 3}

func newDingCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "ding",
		Short: "DING 消息 / 发送 / 撤回",
		Long:  `发送和撤回 DING 消息（应用内/短信/电话）。预发环境可用。`,
		RunE:  groupRunE,
	}

	dingMessageCmd := &cobra.Command{Use: "message", Short: "DING 消息管理", RunE: groupRunE}

	dingMessageSendCmd := &cobra.Command{
		Use:   "send",
		Short: "发送 DING 消息",
		Long: `发送 DING 消息。类型:
  app  = 应用内 DING (默认)
  sms  = 短信 DING (有成本)
  call = 电话 DING (有成本)`,
		Example: `  # 查询 userId: dws contact user search --keyword "姓名"
  dws ding message send --robot-code <robot-code> --users userId1,userId2 --content "请查看"
  dws ding message send --robot-code <robot-code> --type call --users userId1 --content "紧急告警"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "users", "content"); err != nil {
				return err
			}
			robotCode := mustGetFlag(cmd, "robot-code")
			if robotCode == "" {
				robotCode = os.Getenv("DINGTALK_DING_ROBOT_CODE")
			}
			if robotCode == "" {
				return fmt.Errorf("flag --robot-code is required (or set DINGTALK_DING_ROBOT_CODE env var)")
			}
			typeStr := mustGetFlag(cmd, "type")
			remindType, ok := dingRemindTypeMap[typeStr]
			if !ok {
				remindType = 1 // 默认应用内
			}
			toStr := mustGetFlag(cmd, "users")
			var receiverUserIdList []string
			for _, uid := range strings.Split(toStr, ",") {
				if u := strings.TrimSpace(uid); u != "" {
					receiverUserIdList = append(receiverUserIdList, u)
				}
			}
			return callMCPTool("send_ding_message", map[string]any{
				"robotCode":          robotCode,
				"remindType":         remindType,
				"receiverUserIdList": receiverUserIdList,
				"content":            mustGetFlag(cmd, "content"),
			})
		},
	}

	dingMessageRecallCmd := &cobra.Command{
		Use:     "recall",
		Short:   "撤回 DING 消息",
		Example: `  dws ding message recall --robot-code <robot-code> --id <open-ding-id>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			robotCode := mustGetFlag(cmd, "robot-code")
			if robotCode == "" {
				robotCode = os.Getenv("DINGTALK_DING_ROBOT_CODE")
			}
			if robotCode == "" {
				return fmt.Errorf("flag --robot-code is required (or set DINGTALK_DING_ROBOT_CODE env var)")
			}
			return callMCPTool("recall_ding_message", map[string]any{
				"robotCode":  robotCode,
				"openDingId": mustGetFlag(cmd, "id"),
			})
		},
	}

	dingMessageListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询 DING 消息历史",
		Long: `查询当前用户的 DING 消息列表，支持按类型过滤。
--type 支持: ALL(全部)、UNREAD(未读)、SEND(已发)、NEW_COMMENT(新评论)、DELETED(已删除)。
--type 为服务端必填字段，空值会报「type不能为空」；不传时 CLI 默认按 ALL 查询。
列表项会返回 DING 内容，调用方可在同一结果中读取 openDingId、状态与 content，无需再发起详情查询。`,
		Example: `  dws ding message list                 # 默认 --type ALL
  dws ding message list --type UNREAD
  dws ding message list --type SEND --cursor 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetInt64("cursor"); v > 0 {
				toolArgs["cursor"] = v
			}
			// type 是服务端必填，空值会报错；不传或传空时兜底为 ALL。
			t, _ := cmd.Flags().GetString("type")
			if t == "" {
				t = "ALL"
			}
			toolArgs["type"] = t
			return callMCPToolOnServer("im", "list_ding_messages", toolArgs)
		},
	}

	dingMessageReceiverStatusCmd := &cobra.Command{
		Use:   "receiver-status",
		Short: "查看 DING 接收状态",
		Long:  `查看指定 DING 消息的接收者状态（已读/未读等）。`,
		Example: `  dws ding message receiver-status --ding-id <openDingId>
  # 查询 dingId: dws ding message list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "ding-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "list_ding_receiver_status", map[string]any{
				"openDingId": mustGetFlag(cmd, "ding-id"),
			})
		},
	}

	// ── send-personal: 以用户身份发送 DING ──────────────────────

	dingMessageSendPersonalCmd := &cobra.Command{
		Use:   "send-personal",
		Short: "以用户身份发送 DING",
		Long: `以当前用户身份（非机器人）发送 DING 消息。提醒类型:
  app  = 应用内 DING (默认)
  sms  = 短信 DING (有成本)
  call = 电话 DING (有成本)`,
		Example: `  # 查询 openDingTalkId: dws contact user search --query "姓名"
  dws ding message send-personal --users openDingTalkId1,openDingTalkId2 --content "请查看"
  dws ding message send-personal --type call --users openDingTalkId1 --content "紧急告警"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "users", "content"); err != nil {
				return err
			}
			users := parseCSVValues(mustGetFlag(cmd, "users"))
			toolArgs := map[string]any{
				"receiverOpenDingTalkIds": users,
				"content":                 mustGetFlag(cmd, "content"),
				"remindType":              mustGetFlag(cmd, "type"),
			}
			if v, _ := cmd.Flags().GetString("uuid"); v != "" {
				toolArgs["uuid"] = v
			}
			return callMCPToolOnServer("im", "send_personal_ding", toolArgs)
		},
	}

	// ── send-by-message: 消息转 DING ─────────────────────────────

	dingMessageSendByMessageCmd := &cobra.Command{
		Use:   "send-by-message",
		Short: "消息转 DING（将聊天消息转为 DING 通知）",
		Long: `将指定聊天消息转为 DING 发送给指定接收者。提醒类型:
  app  = 应用内 DING (默认)
  sms  = 短信 DING (有成本)
  call = 电话 DING (有成本)`,
		Example: `  # 查询 openDingTalkId: dws contact user search --query "姓名"
  # 查询 openConversationId: dws chat search --keyword "群名"
  dws ding message send-by-message --group <openConversationId> --message-id <openMessageId> --users id1,id2
  dws ding message send-by-message --group <openConversationId> --message-id <openMessageId> --users id1 --type sms`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "message-id", "users"); err != nil {
				return err
			}
			users := parseCSVValues(mustGetFlag(cmd, "users"))
			toolArgs := map[string]any{
				"openConversationId":      mustGetFlag(cmd, "group"),
				"openMessageId":           mustGetFlag(cmd, "message-id"),
				"receiverOpenDingTalkIds": users,
				"remindType":              mustGetFlag(cmd, "type"),
			}
			if v, _ := cmd.Flags().GetString("uuid"); v != "" {
				toolArgs["uuid"] = v
			}
			return callMCPToolOnServer("im", "send_ding_by_message", toolArgs)
		},
	}

	// ── recall-personal: 以用户身份撤回 DING ────────────────────

	dingMessageRecallPersonalCmd := &cobra.Command{
		Use:   "recall-personal",
		Short: "以用户身份撤回 DING",
		Long:  `以当前用户身份撤回已发送的 DING 消息。需要提供发送时返回的 openDingId。`,
		Example: `  dws ding message recall-personal --id <openDingId>
  # 查询 openDingId: dws ding message list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "recall_personal_ding", map[string]any{
				"openDingId": mustGetFlag(cmd, "id"),
			})
		},
	}

	dingMessageSendCmd.Flags().String("robot-code", "", "机器人 ID，发 DING 的机器人编码 (必填，可从 应用管理→机器人 获取，或设 DINGTALK_DING_ROBOT_CODE)")
	dingMessageSendCmd.Flags().String("type", "app", "提醒类型: app/sms/call (默认 app)")
	dingMessageSendCmd.Flags().String("users", "", "接收人 userId 列表 (必填)")
	dingMessageSendCmd.Flags().String("content", "", "消息内容 (必填)")
	dingMessageRecallCmd.Flags().String("robot-code", "", "机器人 ID (必填，或设 DINGTALK_DING_ROBOT_CODE)")
	dingMessageRecallCmd.Flags().String("id", "", "DING 消息 ID (必填)")
	dingMessageListCmd.Flags().Int64("cursor", 0, "分页游标（首次传 0，翻页传返回的 nextCursor）")
	dingMessageListCmd.Flags().String("type", "ALL", "消息类型: ALL / UNREAD / SEND / NEW_COMMENT / DELETED（必填，服务端不接受空值；默认 ALL 全部）")
	dingMessageReceiverStatusCmd.Flags().String("ding-id", "", "DING 消息 openDingId (必填)")
	_ = dingMessageReceiverStatusCmd.MarkFlagRequired("ding-id")
	dingMessageSendPersonalCmd.Flags().String("users", "", "接收者 openDingTalkId 列表，逗号分隔 (必填)")
	_ = dingMessageSendPersonalCmd.MarkFlagRequired("users")
	dingMessageSendPersonalCmd.Flags().String("content", "", "DING 内容 (必填)")
	_ = dingMessageSendPersonalCmd.MarkFlagRequired("content")
	dingMessageSendPersonalCmd.Flags().String("type", "app", "提醒类型: app/sms/call (默认 app)")
	dingMessageSendPersonalCmd.Flags().String("uuid", "", "幂等唯一标识（可选，不传由服务端生成）")
	dingMessageRecallPersonalCmd.Flags().String("id", "", "DING 消息 openDingId (必填)")
	_ = dingMessageRecallPersonalCmd.MarkFlagRequired("id")
	dingMessageSendByMessageCmd.Flags().String("group", "", "原消息所在会话 openConversationId (必填)")
	_ = dingMessageSendByMessageCmd.MarkFlagRequired("group")
	dingMessageSendByMessageCmd.Flags().String("message-id", "", "原消息 openMessageId (必填)")
	_ = dingMessageSendByMessageCmd.MarkFlagRequired("message-id")
	dingMessageSendByMessageCmd.Flags().String("users", "", "接收者 openDingTalkId 列表，逗号分隔 (必填)")
	_ = dingMessageSendByMessageCmd.MarkFlagRequired("users")
	dingMessageSendByMessageCmd.Flags().String("type", "app", "提醒类型: app/sms/call (默认 app)")
	dingMessageSendByMessageCmd.Flags().String("uuid", "", "幂等唯一标识（可选，不传由服务端生成）")
	dingMessageCmd.AddCommand(dingMessageSendCmd, dingMessageRecallCmd, dingMessageListCmd, dingMessageReceiverStatusCmd, dingMessageSendPersonalCmd, dingMessageRecallPersonalCmd, dingMessageSendByMessageCmd)
	root.AddCommand(dingMessageCmd)
	return root
}
