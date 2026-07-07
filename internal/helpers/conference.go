package helpers

import (
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type openCompatHandler struct {
	name    string
	buildFn func() *cobra.Command
}

func (h openCompatHandler) Name() string { return h.name }

func (h openCompatHandler) Command(_ executor.Runner) *cobra.Command {
	return h.buildFn()
}

func init() {
	RegisterPublic(func() Handler {
		return openCompatHandler{name: "conference", buildFn: newConferenceCommand}
	})
}

func newConferenceCommand() *cobra.Command {
	runUnavailable := func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("conference 视频会议能力当前不可用：该产品已从开源 CLI 下线\n  hint: 直接发起/邀请/会中控制请在钉钉客户端操作；如需预约日程，请改用 dws calendar event create")
	}

	root := &cobra.Command{
		Use:    "conference",
		Short:  "视频会议",
		Hidden: true,
		Long: `视频会议产品已从当前开源 CLI 下线。

该命令路径仅作为旧版本兼容入口保留，不再调用后端工具。
直接发起会议、邀请入会、会中控制请在钉钉客户端操作；如需预约日程，请改用 dws calendar event create。`,
		RunE: runUnavailable,
	}

	meetingCmd := &cobra.Command{Use: "meeting", Short: "会议管理（已下线）", RunE: groupRunE}

	meetingCreateCmd := &cobra.Command{
		Use:     "reserve",
		Aliases: []string{"create"},
		Short:   "预约会议（已下线）",
		Long:    `视频会议预约能力已下线；如需预约日程，请改用 dws calendar event create。`,
		Example: `  dws conference meeting reserve --title "产品评审会" \
    --start 2026-03-11T14:00:00+08:00 --end 2026-03-11T15:00:00+08:00`,
		RunE: runUnavailable,
	}

	meetingCreateCmd.Flags().String("title", "", "会议标题 (必填)")
	meetingCreateCmd.Flags().String("start", "", "开始时间 ISO-8601 格式，如 2026-03-11T14:00:00+08:00 (必填)")
	meetingCreateCmd.Flags().String("end", "", "结束时间 ISO-8601 格式，如 2026-03-11T15:00:00+08:00 (必填)")
	meetingCmd.AddCommand(meetingCreateCmd)
	root.AddCommand(meetingCmd)

	// member 子命令组 — 成员管理
	memberCmd := &cobra.Command{Use: "member", Short: "成员管理（已下线）", RunE: groupRunE}

	memberInviteCmd := &cobra.Command{
		Use:   "invite",
		Short: "邀请指定人入会（已下线）",
		Long:  `视频会议邀请入会能力已下线；请在钉钉客户端操作。`,
		Example: `  dws conference member invite --conference-id "xxx" \
    --nicks "张三,李四" --open-dingtalk-ids "id1,id2"`,
		RunE: runUnavailable,
	}
	memberInviteCmd.Flags().String("conference-id", "", "会议ID (必填)")
	memberInviteCmd.Flags().String("meeting-id", "", "会议ID别名")
	memberInviteCmd.Flags().String("nicks", "", "被邀请人昵称，逗号分隔 (必填)")
	memberInviteCmd.Flags().String("nick", "", "被邀请人昵称别名")
	memberInviteCmd.Flags().String("nicknames", "", "被邀请人昵称别名")
	memberInviteCmd.Flags().String("open-dingtalk-ids", "", "被邀请人 openDingTalkId，逗号分隔，通过 contact/aisearch 获取 (必填)")
	memberInviteCmd.Flags().String("ids", "", "openDingTalkId 别名")
	memberInviteCmd.Flags().MarkHidden("meeting-id")
	memberInviteCmd.Flags().MarkHidden("nick")
	memberInviteCmd.Flags().MarkHidden("nicknames")
	memberInviteCmd.Flags().MarkHidden("ids")
	memberCmd.AddCommand(memberInviteCmd)
	root.AddCommand(memberCmd)

	return root
}
