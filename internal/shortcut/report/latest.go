// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package report

import (
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var ReportLatest = shortcut.Shortcut{
	Service: "report", Command: "+report-latest", Product: "report",
	Description:   "读取我最近提交的一篇日志详情",
	Intent:        "只想查看明确 20 天内自己提交的最新日志详情时使用；默认最近 20 天，也可成对指定创建时间窗，完整验证候选后按精确 reportId 读回详情。",
	Risk:          shortcut.RiskRead,
	Safety:        reportReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: reportContract(
		"+report-latest", "读取我最近提交的一篇日志详情",
		"只想查看明确 20 天内自己提交的最新日志详情时使用；默认最近 20 天，也可成对指定创建时间窗，完整验证候选后按精确 reportId 读回详情。",
		reportLatestResult(), nil,
		[]contract.ParamDecl{
			{Name: "keyword", Property: "report_template_name"},
			{Name: "start", Property: "startTime"}, {Name: "end", Property: "endTime"},
		},
		"dws report +report-latest", `dws report +report-latest --start "2026-03-01T00:00:00+08:00" --end "2026-03-20T00:00:00+08:00"`,
	),
	Flags: []shortcut.Flag{
		{Name: "keyword", Type: shortcut.FlagString, Desc: "按日志模板名称精确过滤"},
		{Name: "start", Type: shortcut.FlagString, Desc: "创建开始时间 ISO-8601；--start 与 --end 必须同时提供，且创建时间范围必须有效并不得超过 20 天"},
		{Name: "end", Type: shortcut.FlagString, Desc: "创建结束时间 ISO-8601；--start 与 --end 必须同时提供，且创建时间范围必须有效并不得超过 20 天"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"start", "end"}, Description: "--start 与 --end 必须同时提供，且创建时间范围必须有效并不得超过 20 天"},
	},
	Tips: []string{"dws report +report-latest", `dws report +report-latest --start "2026-03-01T00:00:00+08:00" --end "2026-03-20T00:00:00+08:00"`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Changed("start") != rt.Changed("end") {
			return apperrors.NewValidation("--start 与 --end 必须同时提供", apperrors.WithReason("incomplete_creation_range"))
		}
		if rt.Changed("start") {
			_, _, err := reportValidateRange("start", rt.Str("start"), "end", rt.Str("end"), reportMaximumOutboxDays)
			return err
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		const listOperation = "report/get_send_report_list"
		now := time.Now()
		end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
		start := end.Add(-reportMaximumOutboxDays * 24 * time.Hour)
		if rt.Changed("start") {
			startMillis, endMillis, err := reportValidateRange("start", rt.Str("start"), "end", rt.Str("end"), reportMaximumOutboxDays)
			if err != nil {
				return err
			}
			start, end = time.UnixMilli(startMillis), time.UnixMilli(endMillis)
		}
		params := map[string]any{
			"cursor": 0, "size": 20,
			"startTime": start.UnixMilli(), "endTime": end.UnixMilli(),
		}
		if keyword := strings.TrimSpace(rt.Str("keyword")); keyword != "" {
			params["report_template_name"] = keyword
		}
		data, err := rt.CallMCPData("report", "get_send_report_list", params)
		if err != nil {
			return err
		}
		entries, page, err := reportProjectEntries(data, listOperation)
		if err != nil {
			return err
		}
		if page.HasMore {
			return reportResponseError(listOperation, "incomplete_latest_candidates", "发件箱仍有后续页，不能从不完整集合宣称最新日志")
		}
		if len(entries) == 0 {
			return apperrors.NewValidation("最近 20 天没有可验证的已发送日志", apperrors.WithReason("no_sent_report_fixture"))
		}
		latestID, err := reportLatestEntryID(entries, listOperation)
		if err != nil {
			return err
		}
		const detailOperation = "report/get_report_entry_details"
		detailData, err := rt.CallMCPData("report", "get_report_entry_details", map[string]any{"report_id": latestID})
		if err != nil {
			return err
		}
		detail, err := reportProjectDetail(detailData, detailOperation, latestID)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"report": detail})
	},
}

func reportLatestEntryID(entries []map[string]any, operation string) (string, error) {
	var latestID string
	var latestTime int64
	latestCount := 0
	for index, entry := range entries {
		created, ok := entry["createTime"].(int64)
		if !ok || created <= 0 {
			return "", reportResponseError(operation, "missing_latest_order", "发件箱项目缺少正整数 createTime，不能证明哪一篇最新")
		}
		id, ok := entry["reportId"].(string)
		if !ok || strings.TrimSpace(id) == "" {
			return "", reportResponseError(operation, "missing_item_identity", "发件箱项目缺少稳定 reportId")
		}
		if index == 0 || created > latestTime {
			latestID, latestTime = id, created
			latestCount = 1
		} else if created == latestTime {
			latestCount++
		}
	}
	if latestCount != 1 {
		return "", reportResponseError(operation, "ambiguous_latest_order", "多篇日志具有相同的最高 createTime，不能确定唯一最新日志")
	}
	return latestID, nil
}

func init() {
	shortcut.Register(ReportLatest)
}
