// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler { return calendarHandler{} })
}

// calendarHandler contributes the `calendar attendee list|add|delete` group.
// wukong renamed the calendar participant commands to "attendee" (former name:
// participant); the envelope still exposes them under "participant". These
// leaves call the same MCP tools (get/add/remove_calendar_participant) so the
// wukong command surface is aligned without dropping the legacy participant
// path. MergeCommandTree folds the attendee group into the calendar tree.
type calendarHandler struct{}

func (calendarHandler) Name() string { return "calendar" }

func (calendarHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "calendar",
		Short:             i18n.T("日历"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	event := &cobra.Command{
		Use:               "event",
		Short:             i18n.T("日程管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	event.AddCommand(newCalendarEventListCommand(runner))

	attendee := &cobra.Command{
		Use:               "attendee",
		Short:             i18n.T("参会人管理（与 participant 等价，对齐 wukong 命名）"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	attendee.AddCommand(
		newCalendarAttendeeListCommand(runner),
		newCalendarAttendeeAddCommand(runner),
		newCalendarAttendeeDeleteCommand(runner),
	)
	root.AddCommand(event, attendee)
	return root
}

func newCalendarEventListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   i18n.T("列出日程"),
		Example: "  dws calendar event list\n  dws calendar event list --start 2026-06-29 --end 2026-06-30",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			now := time.Now()
			todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())

			startStr := strings.TrimSpace(firstNonEmptyFlag(cmd, "start", "startTime"))
			endStr := strings.TrimSpace(firstNonEmptyFlag(cmd, "end", "endTime"))

			var startMs, endMs int64
			if startStr != "" {
				ms, err := cmdutil.ParseISOTimeToMillis("start", startStr)
				if err != nil {
					return err
				}
				startMs = ms
			}
			if endStr != "" {
				ms, err := cmdutil.ParseISOTimeToMillis("end", endStr)
				if err != nil {
					return err
				}
				endMs = ms
			}

			// Default missing side to today; if only one side is given, use its
			// date so the range is always valid (e.g. --end 2026-03-10 → start=2026-03-10 00:00).
			if startStr == "" && endStr == "" {
				startMs = todayStart.UnixMilli()
				endMs = todayEnd.UnixMilli()
			} else if startStr == "" {
				endTime := time.UnixMilli(endMs)
				startMs = time.Date(endTime.Year(), endTime.Month(), endTime.Day(), 0, 0, 0, 0, endTime.Location()).UnixMilli()
			} else if endStr == "" {
				startTime := time.UnixMilli(startMs)
				endMs = time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 23, 59, 59, 0, startTime.Location()).UnixMilli()
			}

			if err := cmdutil.ValidateTimeRange(startMs, endMs); err != nil {
				return err
			}

			params := map[string]any{
				"startTime": startMs,
				"endTime":   endMs,
			}
			if v := strings.TrimSpace(firstNonEmptyFlag(cmd, "calendar-id", "calendarId")); v != "" {
				params["calendarId"] = v
			}
			return runCalendarTool(cmd, runner, "list_calendar_events", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("start", "", i18n.T("开始时间 (可选, 默认今天 00:00)"))
	cmd.Flags().String("end", "", i18n.T("结束时间 (可选, 默认今天 23:59)"))
	cmd.Flags().String("startTime", "", i18n.T("--start 的别名"))
	cmd.Flags().String("endTime", "", i18n.T("--end 的别名"))
	cmd.Flags().String("calendar-id", "", i18n.T("日历 ID (可选, 默认主日历)"))
	_ = cmd.Flags().MarkHidden("startTime")
	_ = cmd.Flags().MarkHidden("endTime")
	return cmd
}

func newCalendarAttendeeListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list", Short: i18n.T("查询日程参会人"),
		Example: "  dws calendar attendee list --event <eventId>", Args: cobra.NoArgs, DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID := strings.TrimSpace(firstNonEmptyFlag(cmd, "event", "event-id"))
			if eventID == "" {
				return apperrors.NewValidation("missing required flag(s): --event")
			}
			params := map[string]any{"eventId": eventID}
			if v := strings.TrimSpace(firstNonEmptyFlag(cmd, "calendar-id", "calendarId")); v != "" {
				params["calendarId"] = v
			}
			return runCalendarTool(cmd, runner, "get_calendar_participants", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("event", "", i18n.T("日程 eventId (必填)"))
	cmd.Flags().String("calendar-id", "", i18n.T("日历 ID (可选, 默认主日历)"))
	return cmd
}

func newCalendarAttendeeAddCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use: "add", Short: i18n.T("添加日程参会人"),
		Example: "  dws calendar attendee add --event <eventId> --users userId1,userId2", Args: cobra.NoArgs, DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID := strings.TrimSpace(firstNonEmptyFlag(cmd, "event", "event-id"))
			users := strings.TrimSpace(firstNonEmptyFlag(cmd, "users", "attendees", "user-ids"))
			if eventID == "" {
				return apperrors.NewValidation("missing required flag(s): --event")
			}
			if users == "" {
				return apperrors.NewValidation("missing required flag(s): --users")
			}
			params := map[string]any{"eventId": eventID, "attendeesToAdd": csvToList(users)}
			if v := strings.TrimSpace(firstNonEmptyFlag(cmd, "optional")); v != "" {
				params["optional"] = v
			}
			if v := strings.TrimSpace(firstNonEmptyFlag(cmd, "calendar-id", "calendarId")); v != "" {
				params["calendarId"] = v
			}
			return runCalendarTool(cmd, runner, "add_calendar_participant", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("event", "", i18n.T("日程 eventId (必填)"))
	cmd.Flags().String("users", "", i18n.T("参会人 userId 列表，逗号分隔 (必填)"))
	cmd.Flags().String("optional", "", i18n.T("是否可选参会人 (可选)"))
	cmd.Flags().String("calendar-id", "", i18n.T("日历 ID (可选)"))
	return cmd
}

func newCalendarAttendeeDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use: "delete", Short: i18n.T("移除日程参会人"),
		Example: "  dws calendar attendee delete --event <eventId> --users userId1", Args: cobra.NoArgs, DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID := strings.TrimSpace(firstNonEmptyFlag(cmd, "event", "event-id"))
			users := strings.TrimSpace(firstNonEmptyFlag(cmd, "users", "attendees", "user-ids"))
			if eventID == "" {
				return apperrors.NewValidation("missing required flag(s): --event")
			}
			if users == "" {
				return apperrors.NewValidation("missing required flag(s): --users")
			}
			params := map[string]any{"eventId": eventID, "attendeesToRemove": csvToList(users)}
			if v := strings.TrimSpace(firstNonEmptyFlag(cmd, "calendar-id", "calendarId")); v != "" {
				params["calendarId"] = v
			}
			return runCalendarTool(cmd, runner, "remove_calendar_participant", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("event", "", i18n.T("日程 eventId (必填)"))
	cmd.Flags().String("users", "", i18n.T("参会人 userId 列表，逗号分隔 (必填)"))
	cmd.Flags().String("calendar-id", "", i18n.T("日历 ID (可选)"))
	return cmd
}

func runCalendarTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	inv := executor.NewHelperInvocation(cobracmd.LegacyCommandPath(cmd), "calendar", tool, params)
	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, inv)
	}
	result, err := runner.Run(cmd.Context(), inv)
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

func csvToList(s string) []any {
	parts := strings.Split(s, ",")
	out := make([]any, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
