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
	"fmt"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return attendanceHandler{}
	})
}

type attendanceHandler struct{}

func (attendanceHandler) Name() string {
	return "attendance"
}

func (attendanceHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:   "attendance",
		Short: "考勤打卡 / 排班 / 统计",
		Long: `管理钉钉考勤：查询个人考勤详情、批量查询排班、获取考勤统计摘要、查询考勤组与规则。

子命令:
  record   考勤记录（个人考勤详情）
  shift    排班管理（批量查询班次 / 排班信息）
  summary  获取考勤统计摘要
  rules    查询考勤组与考勤规则`,
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	record := &cobra.Command{
		Use:               "record",
		Short:             "考勤记录",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	record.AddCommand(
		newAttendanceRecordGetCommand(runner),
		newAttendanceRecordListCommand(runner),
	)

	shift := &cobra.Command{
		Use:               "shift",
		Short:             "排班管理",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	shift.AddCommand(newAttendanceShiftListCommand(runner))

	root.AddCommand(
		record,
		shift,
		newAttendanceSummaryCommand(runner),
		newAttendanceRulesCommand(runner),
	)
	return root
}

// ── record get ─────────────────────────────────────────────

func newAttendanceRecordGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查询个人考勤详情",
		Example:           "  dws attendance record get --user USER_ID --date 2026-03-08",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			userID, _ := cmd.Flags().GetString("user")
			dateStr, _ := cmd.Flags().GetString("date")
			if userID == "" {
				return apperrors.NewValidation("--user is required")
			}
			if dateStr == "" {
				return apperrors.NewValidation("--date is required (format: YYYY-MM-DD)")
			}
			t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
			if err != nil {
				return apperrors.NewValidation("--date format error, use YYYY-MM-DD, e.g. 2026-03-08")
			}
			params := map[string]any{
				"userId":   userID,
				"workDate": t.UnixMilli(),
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "attendance", "get_user_attendance_record", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "attendance", "get_user_attendance_record", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	cmd.Flags().String("user", "", "钉钉用户 ID (必填)")
	cmd.Flags().String("date", "", "查询日期，格式 YYYY-MM-DD (必填)")
	preferLegacyLeaf(cmd)
	return cmd
}

// ── record list ────────────────────────────────────────────

func newAttendanceRecordListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "批量查询考勤打卡记录",
		Long:              "批量查询多个员工在指定日期范围的考勤打卡记录。",
		Hidden:            true,
		Example:           "  dws attendance record list --users userId1,userId2 --start 2026-03-03 --end 2026-03-07",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			usersStr, _ := cmd.Flags().GetString("users")
			startStr, _ := cmd.Flags().GetString("start")
			endStr, _ := cmd.Flags().GetString("end")
			if usersStr == "" {
				return apperrors.NewValidation("--users is required")
			}
			if startStr == "" {
				return apperrors.NewValidation("--start is required (format: YYYY-MM-DD)")
			}
			if endStr == "" {
				return apperrors.NewValidation("--end is required (format: YYYY-MM-DD)")
			}
			startT, err := time.ParseInLocation("2006-01-02", startStr, time.Local)
			if err != nil {
				return apperrors.NewValidation("--start date format error, use YYYY-MM-DD")
			}
			endT, err := time.ParseInLocation("2006-01-02", endStr, time.Local)
			if err != nil {
				return apperrors.NewValidation("--end date format error, use YYYY-MM-DD")
			}
			var userIds []any
			for _, u := range strings.Split(usersStr, ",") {
				if s := strings.TrimSpace(u); s != "" {
					userIds = append(userIds, s)
				}
			}
			params := map[string]any{
				"userIds":      userIds,
				"fromDateTime": startT.UnixMilli(),
				"toDateTime":   endT.UnixMilli(),
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "attendance", "get_user_attendance_record", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "attendance", "get_user_attendance_record", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	cmd.Flags().String("users", "", "Comma-separated user ID list (required)")
	cmd.Flags().String("start", "", "Start date, format YYYY-MM-DD (required)")
	cmd.Flags().String("end", "", "End date, format YYYY-MM-DD (required)")
	preferLegacyLeaf(cmd)
	return cmd
}

// ── shift list ─────────────────────────────────────────────

func newAttendanceShiftListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "批量查询员工班次信息",
		Long:              "批量查询多个员工在指定日期的班次信息。最多 7 天，最多 50 人。",
		Example:           "  dws attendance shift list --users userId1,userId2 --start 2026-03-03 --end 2026-03-07",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			usersStr, _ := cmd.Flags().GetString("users")
			startStr, _ := cmd.Flags().GetString("start")
			endStr, _ := cmd.Flags().GetString("end")
			if usersStr == "" {
				return apperrors.NewValidation("--users is required")
			}
			if startStr == "" {
				return apperrors.NewValidation("--start is required (format: YYYY-MM-DD)")
			}
			if endStr == "" {
				return apperrors.NewValidation("--end is required (format: YYYY-MM-DD)")
			}
			startT, err := time.ParseInLocation("2006-01-02", startStr, time.Local)
			if err != nil {
				return apperrors.NewValidation("--start date format error, use YYYY-MM-DD")
			}
			endT, err := time.ParseInLocation("2006-01-02", endStr, time.Local)
			if err != nil {
				return apperrors.NewValidation("--end date format error, use YYYY-MM-DD")
			}
			var userIds []any
			for _, u := range strings.Split(usersStr, ",") {
				if s := strings.TrimSpace(u); s != "" {
					userIds = append(userIds, s)
				}
			}
			params := map[string]any{
				"userIds":      userIds,
				"fromDateTime": startT.UnixMilli(),
				"toDateTime":   endT.UnixMilli(),
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "attendance", "batch_get_employee_shifts", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "attendance", "batch_get_employee_shifts", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	cmd.Flags().String("users", "", "Comma-separated user ID list, max 50 (required)")
	cmd.Flags().String("start", "", "Start date, format YYYY-MM-DD (required)")
	cmd.Flags().String("end", "", "End date, format YYYY-MM-DD (required)")
	preferLegacyLeaf(cmd)
	return cmd
}

// ── summary ────────────────────────────────────────────────

func newAttendanceSummaryCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "summary",
		Short:             "查询某个人的考勤统计摘要",
		Long:              "查询某个人的考勤统计摘要。--user 与 --date 均必填。",
		Example:           `  dws attendance summary --user USER_ID --date "2026-03-12 15:00:00"`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			userID, _ := cmd.Flags().GetString("user")
			workDateStr, _ := cmd.Flags().GetString("date")
			if userID == "" {
				return apperrors.NewValidation("--user is required, provide DingTalk user ID")
			}
			if workDateStr == "" {
				return apperrors.NewValidation("--date is required, format yyyy-MM-dd HH:mm:ss")
			}
			_, err := time.ParseInLocation("2006-01-02 15:04:05", workDateStr, time.Local)
			if err != nil {
				return apperrors.NewValidation("--date format error, use yyyy-MM-dd HH:mm:ss")
			}
			// Build nested structure QueryUserAttendVO
			vo := map[string]any{
				"userId":    userID,
				"queryDate": workDateStr,
			}
			params := map[string]any{
				"QueryUserAttendVO": vo,
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "attendance", "get_attendance_summary", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "attendance", "get_attendance_summary", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	cmd.Flags().String("user", "", "钉钉用户 ID（必填）")
	cmd.Flags().String("date", "", "工作日期，格式 yyyy-MM-dd HH:mm:ss，如 2026-03-12 15:00:00（必填）")
	preferLegacyLeaf(cmd)
	return cmd
}

// ── rules ──────────────────────────────────────────────────

func newAttendanceRulesCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "rules",
		Short:             "查询考勤组与考勤规则",
		Long:              "调用 MCP 工具 query_attendance_group_or_rules 查询考勤组/考勤规则。\n例如：我属于哪个考勤组、打卡范围是什么、弹性工时怎么算。",
		Example:           "  dws attendance rules --date 2026-03-14\n  dws attendance rules --date \"2026-03-14 09:00:00\"",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dateStr, _ := cmd.Flags().GetString("date")
			if dateStr == "" {
				return apperrors.NewValidation("--date is required")
			}
			// Support YYYY-MM-DD or yyyy-MM-dd HH:mm:ss, normalize to yyyy-MM-dd HH:mm:ss
			var dateFormatted string
			if t, err := time.ParseInLocation("2006-01-02 15:04:05", dateStr, time.Local); err == nil {
				dateFormatted = t.Format("2006-01-02 15:04:05")
			} else if t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local); err == nil {
				dateFormatted = t.Format("2006-01-02 15:04:05")
			} else {
				return apperrors.NewValidation(fmt.Sprintf("--date format error, use YYYY-MM-DD or yyyy-MM-dd HH:mm:ss: %s", dateStr))
			}
			params := map[string]any{
				"date": dateFormatted,
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "attendance", "query_attendance_group_or_rules", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "attendance", "query_attendance_group_or_rules", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	cmd.Flags().String("date", "", "考勤日期，格式 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss (必填)")
	preferLegacyLeaf(cmd)
	return cmd
}
