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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return reportHandler{}
	})
}

type reportHandler struct{}

func (reportHandler) Name() string {
	return "report"
}

func (reportHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:     "report",
		Aliases: []string{"log"},
		Short:   "日志 / 模版 / 统计",
		Long: `钉钉日志：模版、创建、详情、列表、统计。

子命令:
  template  日志模版（list / detail）
  create    创建日志
  detail    获取日志详情
  list      查询收到的日志列表
  stats     获取日志统计数据
  sent      查询已发送的日志列表`,
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	template := &cobra.Command{
		Use:               "template",
		Short:             "日志模版",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	template.AddCommand(
		newReportTemplateListCommand(runner),
		newReportTemplateDetailCommand(runner),
	)

	root.AddCommand(
		template,
		newReportCreateCommand(runner),
		newReportDetailCommand(runner),
		newReportListCommand(runner),
		newReportStatsCommand(runner),
		newReportSentCommand(runner),
	)
	return root
}

// ── flexTimeLayouts: supported date formats, most specific first ──

var flexTimeLayouts = []string{
	time.RFC3339,                // 2006-01-02T15:04:05+08:00
	"2006-01-02T15:04:05Z",      // UTC Z suffix
	"2006-01-02T15:04:05-07:00", // with offset but no colon
	"2006-01-02T15:04:05",       // no timezone
	"2006-01-02 15:04:05",       // space-separated
	"2006-01-02T15:04",          // no seconds
	"2006-01-02 15:04",          // no seconds, space
	"2006-01-02",                // date only
	"2006/01/02 15:04:05",       // slash + time
	"2006/01/02",                // slash date
	"20060102",                  // compact YYYYMMDD
}

// parseFlexTimeToMillis parses a date string using multiple formats and returns Unix milliseconds.
// Supports 11 formats for maximum compatibility with user input.
func parseFlexTimeToMillis(flagName, value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, apperrors.NewValidation(fmt.Sprintf(
			"--%s is required\n  hint: example: 2026-03-10T14:00:00+08:00", flagName))
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.Local
	}
	for _, layout := range flexTimeLayouts {
		t, err := time.ParseInLocation(layout, value, loc)
		if err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, apperrors.NewValidation(fmt.Sprintf(
		"cannot parse time for --%s (input: %q)\n  hint: supported formats: 2026-03-23T14:00:00+08:00, 2026-03-23 14:00:00, 2026-03-23",
		flagName, value))
}

// validateTimeRange checks that endMs is strictly after startMs.
func validateTimeRange(startMs, endMs int64) error {
	if endMs <= startMs {
		return apperrors.NewValidation("--end must be after --start\n  hint: swap the values or adjust the time range")
	}
	return nil
}

// ── template list ──────────────────────────────────────────

func newReportTemplateListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "获取当前用户可用的日志模版列表",
		Example:           "  dws report template list",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "report", "get_available_report_templates", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "report", "get_available_report_templates", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	return cmd
}

// ── template detail ────────────────────────────────────────

func newReportTemplateDetailCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "detail",
		Short:             "获取日志模版详情",
		Example:           "  dws report template detail --name <templateName>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			if name == "" {
				return apperrors.NewValidation("--name is required")
			}
			params := map[string]any{
				"report_template_name": name,
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "report", "get_template_details_by_name", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "report", "get_template_details_by_name", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	cmd.Flags().String("name", "", "模版名称 (必填)")
	preferLegacyLeaf(cmd)
	return cmd
}

// ── create ─────────────────────────────────────────────────

func newReportCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "创建日志",
		Long: `按模版创建一条日志。--contents 为 JSON 数组，每项需含 key、sort、content、contentType、type，
与远程 create_report 一致；可先通过 report template list / template detail 取得 templateId 与控件定义。

注意：每个 contents 项的 key 必须精确等于模板的 field_name（中文/英文逐字匹配，不是控件 ID 或别名）。
key 与 field_name 不一致时钉钉 API 会返回 SYSTEM_ERROR (success=false)，CLI 层不会预先拦截。
请先用 report template detail 查到 report_template_fields[].field_name 后再填。`,
		Example: `  # Step 1：查模板，取 report_template_fields[].field_name 当作 contents[].key
  dws report template detail --name "周报"

  # Step 2：用上一步拿到的 field_name 作为 key 创建日志
  dws report create --template-id TPL_ID --contents '[{"key":"<field_name>","sort":"0","content":"完成开发","contentType":"markdown","type":"1"}]'

  # 同时通知到接收人单聊
  dws report create --template-id TPL_ID --contents '[...]' --to-chat --to-user-ids userId1,userId2`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			tplID, _ := cmd.Flags().GetString("template-id")
			if tplID == "" {
				return apperrors.NewValidation("--template-id is required")
			}
			contentsJSON, _ := cmd.Flags().GetString("contents")
			if contentsJSON == "" {
				return apperrors.NewValidation("--contents is required")
			}
			var contents []map[string]any
			if err := json.Unmarshal([]byte(contentsJSON), &contents); err != nil {
				return apperrors.NewValidation(fmt.Sprintf("--contents JSON parse failed: %v", err))
			}
			ddFrom, _ := cmd.Flags().GetString("dd-from")
			if ddFrom == "" {
				ddFrom = "dws"
			}
			toChat, _ := cmd.Flags().GetBool("to-chat")
			params := map[string]any{
				"templateId": tplID,
				"contents":   contents,
				"ddFrom":     ddFrom,
				"toChat":     toChat,
			}
			if v, _ := cmd.Flags().GetString("to-user-ids"); v != "" {
				params["toUserIds"] = parseUserIDs(v)
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "report", "create_report", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "report", "create_report", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	cmd.Flags().String("template-id", "", "日志模版 ID (必填)")
	cmd.Flags().String("contents", "", "日志内容 JSON 数组 (必填)，每项含 key/sort/content/contentType/type；key 必须精确等于模板 field_name (用 report template detail --name <模板名> 查询)")
	cmd.Flags().String("dd-from", "dws", "创建来源标识")
	cmd.Flags().Bool("to-chat", false, "是否发送到日志接收人单聊")
	cmd.Flags().String("to-user-ids", "", "接收人 userId，逗号分隔 (可选)")
	preferLegacyLeaf(cmd)
	return cmd
}

// ── detail ─────────────────────────────────────────────────

func newReportDetailCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "detail",
		Short:             "获取日志详情",
		Example:           "  dws report detail --report-id <reportId>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			reportID, _ := cmd.Flags().GetString("report-id")
			if reportID == "" {
				return apperrors.NewValidation("--report-id is required")
			}
			params := map[string]any{
				"report_id": reportID,
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "report", "get_report_entry_details", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "report", "get_report_entry_details", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	cmd.Flags().String("report-id", "", "日志 ID (必填)")
	preferLegacyLeaf(cmd)
	return cmd
}

// ── list (received reports) ────────────────────────────────
// Key fix: cursor defaults to 0, size defaults to 20, flexible date parsing

func newReportListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "查询当前人收到的日志列表",
		Example: `  dws report list --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00"
  dws report list --start "2026-03-10 00:00:00" --end "2026-03-10 23:59:59" --cursor 0 --size 20`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			startStr, _ := cmd.Flags().GetString("start")
			endStr, _ := cmd.Flags().GetString("end")

			startMs, err := parseFlexTimeToMillis("start", startStr)
			if err != nil {
				return err
			}
			endMs, err := parseFlexTimeToMillis("end", endStr)
			if err != nil {
				return err
			}
			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}

			// cursor defaults to 0, size defaults to 20
			cursor, _ := cmd.Flags().GetInt("cursor")
			size, _ := cmd.Flags().GetInt("size")
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 && !cmd.Flags().Changed("size") {
				size = v
			}

			params := map[string]any{
				"startTime": float64(startMs),
				"endTime":   float64(endMs),
				"cursor":    float64(cursor),
				"size":      float64(size),
			}

			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "report", "get_received_report_list", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "report", "get_received_report_list", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	cmd.Flags().String("start", "", "开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)")
	cmd.Flags().String("end", "", "结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)")
	cmd.Flags().Int("cursor", 0, "分页游标，首次传 0 (默认 0)")
	cmd.Flags().Int("size", 20, "每页条数，最大 20 (默认 20)")
	cmd.Flags().Int("limit", 0, "--size 的别名")
	_ = cmd.Flags().MarkHidden("limit")
	preferLegacyLeaf(cmd)
	return cmd
}

// ── stats ──────────────────────────────────────────────────

func newReportStatsCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "stats",
		Short:             "获取日志统计数据",
		Example:           "  dws report stats --report-id <reportId>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			reportID, _ := cmd.Flags().GetString("report-id")
			if reportID == "" {
				return apperrors.NewValidation("--report-id is required")
			}
			params := map[string]any{
				"report_id": reportID,
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "report", "get_report_statistics_by_id", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "report", "get_report_statistics_by_id", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	cmd.Flags().String("report-id", "", "日志 ID (必填)")
	preferLegacyLeaf(cmd)
	return cmd
}

// ── sent (my created reports) ──────────────────────────────
// Key fix: cursor defaults to 0, size defaults to 20, start/end default to last 30 days

func newReportSentCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sent",
		Short: "查询当前人创建的日志列表",
		Example: `  dws report sent
  dws report sent --cursor 0 --size 20
  dws report sent --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00"
  dws report sent --template-name "日报"`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// cursor defaults to 0, size defaults to 20
			cursor, _ := cmd.Flags().GetInt("cursor")
			size, _ := cmd.Flags().GetInt("size")
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 && !cmd.Flags().Changed("size") {
				size = v
			}

			params := map[string]any{
				"cursor": float64(cursor),
				"size":   float64(size),
			}

			// Default time range: last 30 days
			now := time.Now()
			startDefault := now.AddDate(0, 0, -30).Truncate(24 * time.Hour).Format(time.RFC3339)
			endDefault := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location()).Format(time.RFC3339)

			startStr, _ := cmd.Flags().GetString("start")
			if startStr == "" {
				startStr = startDefault
			}
			endStr, _ := cmd.Flags().GetString("end")
			if endStr == "" {
				endStr = endDefault
			}

			startMs, err := parseFlexTimeToMillis("start", startStr)
			if err != nil {
				return err
			}
			params["startTime"] = float64(startMs)

			endMs, err := parseFlexTimeToMillis("end", endStr)
			if err != nil {
				return err
			}
			params["endTime"] = float64(endMs)

			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}

			// Optional modified time filters
			if v, _ := cmd.Flags().GetString("modified-start"); v != "" {
				ms, err := parseFlexTimeToMillis("modified-start", v)
				if err != nil {
					return err
				}
				params["modifiedStartTime"] = float64(ms)
			}
			if v, _ := cmd.Flags().GetString("modified-end"); v != "" {
				ms, err := parseFlexTimeToMillis("modified-end", v)
				if err != nil {
					return err
				}
				params["modifiedEndTime"] = float64(ms)
			}

			// Optional template name filter
			if v, _ := cmd.Flags().GetString("template-name"); v != "" {
				params["report_template_name"] = v
			}

			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "report", "get_send_report_list", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "report", "get_send_report_list", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	cmd.Flags().Int("cursor", 0, "分页游标，首次传 0 (默认 0)")
	cmd.Flags().Int("size", 20, "每页条数，最大 20 (默认 20)")
	cmd.Flags().Int("limit", 0, "--size 的别名")
	_ = cmd.Flags().MarkHidden("limit")
	cmd.Flags().String("start", "", "创建开始时间 ISO-8601 (默认最近 30 天)")
	cmd.Flags().String("end", "", "创建结束时间 ISO-8601 (默认最近 30 天)")
	cmd.Flags().String("modified-start", "", "修改开始时间 ISO-8601 (可选)")
	cmd.Flags().String("modified-end", "", "修改结束时间 ISO-8601 (可选)")
	cmd.Flags().String("template-name", "", "日志模板名称 (可选，不传查全部)")
	preferLegacyLeaf(cmd)
	return cmd
}

// ── helpers ────────────────────────────────────────────────

func parseUserIDs(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
