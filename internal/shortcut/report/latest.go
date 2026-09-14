// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package report

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const (
	reportLatestPageSize = 20
	reportLatestMaxPages = 10
	reportLatestMaxItems = reportLatestPageSize * reportLatestMaxPages
	reportLatestTimeout  = 2 * time.Minute
)

var ReportLatest = shortcut.Shortcut{
	Service: "report", Command: "+report-latest", Product: "report",
	Description:   "读取我最近提交的一篇日志详情",
	Intent:        "只想查看明确 20 天内自己提交的最新日志详情时使用；默认最近 20 天，也可成对指定创建时间窗，有界遍历全部候选页后按精确 reportId 读回详情。",
	Risk:          shortcut.RiskRead,
	Safety:        reportReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: reportContract(
		"+report-latest", "读取我最近提交的一篇日志详情",
		"只想查看明确 20 天内自己提交的最新日志详情时使用；默认最近 20 天，也可成对指定创建时间窗，有界遍历全部候选页后按精确 reportId 读回详情。",
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
		originalContext := rt.Command().Context()
		if originalContext == nil {
			originalContext = context.Background()
		}
		boundedContext, cancel := context.WithTimeout(originalContext, reportLatestTimeout)
		rt.Command().SetContext(boundedContext)
		defer func() {
			rt.Command().SetContext(originalContext)
			cancel()
		}()

		now := time.Now()
		// 固定在调用开始时刻，避免把未来的“当天结束”放进查询窗。后者会让
		// 分页期间新提交的日志插入候选集合，造成重复、漏项或游标漂移。
		end := now
		start := end.Add(-reportMaximumOutboxDays * 24 * time.Hour)
		if rt.Changed("start") {
			startMillis, endMillis, err := reportValidateRange("start", rt.Str("start"), "end", rt.Str("end"), reportMaximumOutboxDays)
			if err != nil {
				return err
			}
			start, end = time.UnixMilli(startMillis), time.UnixMilli(endMillis)
		}
		params := map[string]any{
			"size":      reportLatestPageSize,
			"startTime": start.UnixMilli(), "endTime": end.UnixMilli(),
		}
		if keyword := strings.TrimSpace(rt.Str("keyword")); keyword != "" {
			params["report_template_name"] = keyword
		}
		entries, err := reportCollectLatestCandidates(rt, params, listOperation)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return apperrors.NewValidation("所选时间窗内没有可验证的已发送日志", apperrors.WithReason("no_sent_report_fixture"))
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

func reportCollectLatestCandidates(rt *shortcut.RuntimeContext, baseParams map[string]any, operation string) ([]map[string]any, error) {
	entries := make([]map[string]any, 0, reportLatestPageSize)
	seen := make(map[string]int64, reportLatestPageSize)
	cursor := 0
	for pageIndex := 0; pageIndex < reportLatestMaxPages; pageIndex++ {
		params := make(map[string]any, len(baseParams)+1)
		for key, value := range baseParams {
			params[key] = value
		}
		params["cursor"] = cursor
		data, err := rt.CallMCPData("report", "get_send_report_list", params)
		if err != nil {
			return nil, err
		}
		pageEntries, page, err := reportProjectEntries(data, operation)
		if err != nil {
			return nil, err
		}
		if err := reportValidateContinuation(page, cursor, operation); err != nil {
			return nil, err
		}
		for _, entry := range pageEntries {
			id, _ := entry["reportId"].(string)
			created, _ := entry["createTime"].(int64)
			if previous, duplicate := seen[id]; duplicate {
				if previous != created {
					return nil, reportResponseError(operation, "conflicting_duplicate_item", "Report 候选分页中的重复 reportId 具有冲突 createTime")
				}
				// 服务端分页边界偶尔会重叠。同一 ID、同一排序键属于可安全
				// 去重的重复证据，不应让完整读取失败。
				continue
			}
			if len(entries) >= reportLatestMaxItems {
				return nil, reportResponseError(operation, "latest_item_limit_reached", "最新日志候选超过有界条数上限")
			}
			seen[id] = created
			entries = append(entries, entry)
		}
		if !page.HasMore {
			return entries, nil
		}
		// reportProjectEntries 已将 continuation 校验为整数并规范化为十进制字符串。
		next, _ := strconv.Atoi(page.Next)
		cursor = next
	}
	return nil, reportResponseError(
		operation,
		"latest_page_limit_reached",
		"达到最新日志候选分页上限时服务端仍有后续页；拒绝从不完整集合宣称最新日志",
	)
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
