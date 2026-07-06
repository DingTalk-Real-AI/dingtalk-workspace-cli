package helpers

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newConferenceCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "conference",
		Short: "视频会议：发起/预约/邀请入会/会中控制",
		Long:  `钉钉视频会议：发起即时会议、预约会议、邀请成员入会、会中控制（静音/摄像头/共享/录制/字幕/视图）。`,
		RunE:  groupRunE,
	}

	meetingCmd := &cobra.Command{Use: "meeting", Short: "会议管理", RunE: groupRunE}

	meetingCreateCmd := &cobra.Command{
		Use:     "reserve",
		Aliases: []string{"create"},
		Short:   "预约会议",
		Long:    `预约钉钉会议，指定标题与时间段。注意：不会自动关联日历日程。`,
		Example: `  dws conference meeting reserve --title "产品评审会" \
    --start 2026-03-11T14:00:00+08:00 --end 2026-03-11T15:00:00+08:00`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "title"); err != nil {
				return err
			}
			startTime, err := parseISOTimeToMillis("start", mustGetFlag(cmd, "start"))
			if err != nil {
				return err
			}
			endTime, err := parseISOTimeToMillis("end", mustGetFlag(cmd, "end"))
			if err != nil {
				return err
			}
			if err := validateTimeRange(startTime, endTime); err != nil {
				return err
			}
			return callMCPTool("create_meeting_reservation", map[string]any{
				"title":     mustGetFlag(cmd, "title"),
				"startTime": startTime,
				"endTime":   endTime,
			})
		},
	}

	meetingCreateCmd.Flags().String("title", "", "会议标题 (必填)")
	meetingCreateCmd.Flags().String("start", "", "开始时间 ISO-8601 格式，如 2026-03-11T14:00:00+08:00 (必填)")
	meetingCreateCmd.Flags().String("end", "", "结束时间 ISO-8601 格式，如 2026-03-11T15:00:00+08:00 (必填)")
	meetingCmd.AddCommand(meetingCreateCmd)
	root.AddCommand(meetingCmd)

	// member 子命令组 — 成员管理
	memberCmd := &cobra.Command{Use: "member", Short: "成员管理", RunE: groupRunE}

	memberInviteCmd := &cobra.Command{
		Use:   "invite",
		Short: "邀请指定人入会",
		Long:  `邀请指定联系人加入目标会议。需提供 conferenceId 和被邀请人的 openDingTalkId（通过通讯录工具获取）。`,
		Example: `  dws conference member invite --conference-id "xxx" \
    --nicks "张三,李四" --open-dingtalk-ids "id1,id2"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			confID, err := mustFlagOrFallback(cmd, "conference-id", "meeting-id", "conferenceId")
			if err != nil {
				return err
			}
			nicksStr, err := mustFlagOrFallback(cmd, "nicks", "nick", "nicknames")
			if err != nil {
				return err
			}
			idsStr, err := mustFlagOrFallback(cmd, "open-dingtalk-ids", "openDingTalkIds", "ids")
			if err != nil {
				return err
			}
			nicks := parseCSVValues(nicksStr)
			openDingTalkIds := parseCSVValues(idsStr)
			if len(nicks) != len(openDingTalkIds) {
				return fmt.Errorf("--nicks 和 --open-dingtalk-ids 数量不匹配: %d vs %d", len(nicks), len(openDingTalkIds))
			}
			inviteeList := make([]map[string]any, len(nicks))
			for i := range nicks {
				inviteeList[i] = map[string]any{
					"nick":           nicks[i],
					"openDingTalkId": openDingTalkIds[i],
				}
			}
			return callMCPTool("invite_meeting_participants", map[string]any{
				"inviteeList":  inviteeList,
				"conferenceId": confID,
			})
		},
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
