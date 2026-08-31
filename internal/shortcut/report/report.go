// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package report registers strict declarative shortcuts for DingTalk reports.
package report

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const (
	reportMaximumInboxDays  = 180
	reportMaximumOutboxDays = 20
)

func reportParseISOMillis(name, value string) (int64, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return 0, apperrors.NewValidation(
			fmt.Sprintf("--%s 时间格式无效，需 ISO-8601（如 2026-03-10T00:00:00+08:00）", name),
			apperrors.WithReason("invalid_time_format"),
		)
	}
	return parsed.UnixMilli(), nil
}

func reportValidatePage(cursor, size int) error {
	if cursor < 0 {
		return apperrors.NewValidation("--cursor 不能小于 0", apperrors.WithReason("invalid_cursor"))
	}
	if size < 1 || size > 20 {
		return apperrors.NewValidation("--size 必须在 1 到 20 之间", apperrors.WithReason("invalid_page_size"))
	}
	return nil
}

func reportValidateRange(startName, startValue, endName, endValue string, maximumDays int) (int64, int64, error) {
	start, err := reportParseISOMillis(startName, startValue)
	if err != nil {
		return 0, 0, err
	}
	end, err := reportParseISOMillis(endName, endValue)
	if err != nil {
		return 0, 0, err
	}
	if end <= start {
		return 0, 0, apperrors.NewValidation("结束时间必须晚于开始时间", apperrors.WithReason("invalid_time_range"))
	}
	if maximumDays > 0 && end-start > int64(maximumDays)*int64(24*time.Hour/time.Millisecond) {
		return 0, 0, apperrors.NewValidation(
			fmt.Sprintf("时间范围不能超过 %d 天", maximumDays),
			apperrors.WithReason("time_range_too_large"),
		)
	}
	return start, end, nil
}

func reportValidateContinuation(page reportPageEvidence, current int, operation string) error {
	if !page.HasMore {
		return nil
	}
	next, err := strconv.ParseInt(page.Next, 10, 64)
	if err != nil || next <= int64(current) {
		return reportResponseError(operation, "stalled_cursor", "Report continuation cursor 没有严格前进")
	}
	return nil
}

var InboxList = shortcut.Shortcut{
	Service: "report", Command: "+inbox-list", Product: "report",
	Description:   "列出我收到的日志",
	Intent:        "需要按明确时间范围读取别人发给我的日志摘要并取得稳定 reportId 时使用；后端必须提供可验证的终止或严格前进 cursor。",
	Risk:          shortcut.RiskRead,
	Safety:        reportReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: reportContract(
		"+inbox-list", "列出我收到的日志",
		"需要按明确时间范围读取别人发给我的日志摘要并取得稳定 reportId 时使用；后端必须提供可验证的终止或严格前进 cursor。",
		reportCollectionResult("reports", "严格验证的收件箱日志页"), reportPagination(),
		[]contract.ParamDecl{
			{Name: "start", Property: "start"}, {Name: "end", Property: "end"},
			{Name: "cursor", Property: "cursor"}, {Name: "size", Property: "size"},
			{Name: "sender-user-ids", Property: "senderUserIds", InterfaceType: "array"},
		},
		"dws report +inbox-list --start \"2026-03-10T00:00:00+08:00\" --end \"2026-03-10T23:59:59+08:00\" --cursor 0 --size 20",
	),
	Flags: []shortcut.Flag{
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间 ISO-8601；结束时间必须晚于开始时间，跨度不得超过 180 天", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间 ISO-8601；结束时间必须晚于开始时间，跨度不得超过 180 天", Required: true},
		{Name: "cursor", Type: shortcut.FlagInt, Default: "0", Desc: "分页游标；--cursor 不能小于 0，续页 cursor 必须严格前进"},
		{Name: "size", Type: shortcut.FlagInt, Default: "20", Desc: "每页条数；--size 必须在 1 到 20 之间"},
		{Name: "sender-user-ids", Type: shortcut.FlagStringSlice, Desc: "发送人 staffId 列表"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"start", "end"}, Description: "结束时间必须晚于开始时间，跨度不得超过 180 天"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"cursor"}, Description: "--cursor 不能小于 0，续页 cursor 必须严格前进"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"size"}, Description: "--size 必须在 1 到 20 之间"},
	},
	Tips: []string{`dws report +inbox-list --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --cursor 0 --size 20`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if err := reportValidatePage(rt.Int("cursor"), rt.Int("size")); err != nil {
			return err
		}
		_, _, err := reportValidateRange("start", rt.Str("start"), "end", rt.Str("end"), reportMaximumInboxDays)
		return err
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		const operation = "report/get_received_report_list"
		start, end, err := reportValidateRange("start", rt.Str("start"), "end", rt.Str("end"), reportMaximumInboxDays)
		if err != nil {
			return err
		}
		params := map[string]any{
			"startTime": start, "endTime": end,
			"cursor": rt.Int("cursor"), "size": rt.Int("size"),
		}
		if rt.Changed("sender-user-ids") {
			params["senderUserIds"] = rt.StrSlice("sender-user-ids")
		}
		data, err := rt.CallMCPData("report", "get_received_report_list", params)
		if err != nil {
			return err
		}
		reports, page, err := reportProjectEntries(data, operation)
		if err != nil {
			return err
		}
		if err := reportValidateContinuation(page, rt.Int("cursor"), operation); err != nil {
			return err
		}
		return outputReportPage(rt, "reports", reports, page)
	},
}

var OutboxList = shortcut.Shortcut{
	Service: "report", Command: "+outbox-list", Product: "report",
	Description:   "列出我发出的日志",
	Intent:        "需要按创建或修改时间回顾自己提交的日志并取得稳定 reportId 时使用；每次创建或修改时间窗不得超过 20 天。",
	Risk:          shortcut.RiskRead,
	Safety:        reportReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: reportContract(
		"+outbox-list", "列出我发出的日志",
		"需要按创建或修改时间回顾自己提交的日志并取得稳定 reportId 时使用；每次创建或修改时间窗不得超过 20 天。",
		reportCollectionResult("reports", "严格验证的发件箱日志页"), reportPagination(),
		[]contract.ParamDecl{
			{Name: "cursor", Property: "cursor"}, {Name: "size", Property: "size"},
			{Name: "start", Property: "start"}, {Name: "end", Property: "end"},
			{Name: "modified-start", Property: "modifiedStart"}, {Name: "modified-end", Property: "modifiedEnd"},
			{Name: "template-name", Property: "templateName"},
		},
		"dws report +outbox-list --cursor 0 --size 20",
	),
	Flags: []shortcut.Flag{
		{Name: "cursor", Type: shortcut.FlagInt, Default: "0", Desc: "分页游标；--cursor 不能小于 0，续页 cursor 必须严格前进"},
		{Name: "size", Type: shortcut.FlagInt, Default: "20", Desc: "每页条数；--size 必须在 1 到 20 之间"},
		{Name: "start", Type: shortcut.FlagString, Desc: "创建开始时间 ISO-8601；创建时间范围必须有效且不得超过 20 天"},
		{Name: "end", Type: shortcut.FlagString, Desc: "创建结束时间 ISO-8601；创建时间范围必须有效且不得超过 20 天"},
		{Name: "modified-start", Type: shortcut.FlagString, Desc: "修改开始时间 ISO-8601；修改时间必须成对提供、范围有效且不得超过 20 天"},
		{Name: "modified-end", Type: shortcut.FlagString, Desc: "修改结束时间 ISO-8601；修改时间必须成对提供、范围有效且不得超过 20 天"},
		{Name: "template-name", Type: shortcut.FlagString, Desc: "日志模板名称"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"cursor"}, Description: "--cursor 不能小于 0，续页 cursor 必须严格前进"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"size"}, Description: "--size 必须在 1 到 20 之间"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"start", "end"}, Description: "创建时间范围必须有效且不得超过 20 天"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"modified-start", "modified-end"}, Description: "修改时间必须成对提供、范围有效且不得超过 20 天"},
	},
	Tips: []string{"dws report +outbox-list --cursor 0 --size 20"},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if err := reportValidatePage(rt.Int("cursor"), rt.Int("size")); err != nil {
			return err
		}
		if rt.Changed("start") && rt.Changed("end") {
			if _, _, err := reportValidateRange("start", rt.Str("start"), "end", rt.Str("end"), reportMaximumOutboxDays); err != nil {
				return err
			}
		}
		if rt.Changed("modified-start") != rt.Changed("modified-end") {
			return apperrors.NewValidation("--modified-start 与 --modified-end 必须同时提供", apperrors.WithReason("incomplete_modified_range"))
		}
		if rt.Changed("modified-start") {
			_, _, err := reportValidateRange("modified-start", rt.Str("modified-start"), "modified-end", rt.Str("modified-end"), reportMaximumOutboxDays)
			return err
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		const operation = "report/get_send_report_list"
		now := time.Now()
		defaultEnd := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
		startValue := defaultEnd.Add(-reportMaximumOutboxDays * 24 * time.Hour).Format(time.RFC3339)
		endValue := defaultEnd.Format(time.RFC3339)
		if rt.Changed("start") {
			startValue = rt.Str("start")
		}
		if rt.Changed("end") {
			endValue = rt.Str("end")
		}
		start, end, err := reportValidateRange("start", startValue, "end", endValue, reportMaximumOutboxDays)
		if err != nil {
			return err
		}
		params := map[string]any{
			"cursor": rt.Int("cursor"), "size": rt.Int("size"),
			"startTime": start, "endTime": end,
		}
		if rt.Changed("modified-start") {
			modifiedStart, modifiedEnd, rangeErr := reportValidateRange("modified-start", rt.Str("modified-start"), "modified-end", rt.Str("modified-end"), reportMaximumOutboxDays)
			if rangeErr != nil {
				return rangeErr
			}
			params["modifiedStartTime"], params["modifiedEndTime"] = modifiedStart, modifiedEnd
		}
		if name := strings.TrimSpace(rt.Str("template-name")); name != "" {
			params["report_template_name"] = name
		}
		data, err := rt.CallMCPData("report", "get_send_report_list", params)
		if err != nil {
			return err
		}
		reports, page, err := reportProjectEntries(data, operation)
		if err != nil {
			return err
		}
		if err := reportValidateContinuation(page, rt.Int("cursor"), operation); err != nil {
			return err
		}
		return outputReportPage(rt, "reports", reports, page)
	},
}

var TemplateSearch = shortcut.Shortcut{
	Service: "report", Command: "+template-search", Product: "report",
	Description:   "按名称搜索可用日志模板",
	Intent:        "需要从当前用户全部可用日志模板中按名称查找稳定 templateId，或在提交日志前确认模板是否存在时使用。",
	Risk:          shortcut.RiskRead,
	Safety:        reportReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: reportContract(
		"+template-search", "按名称搜索可用日志模板",
		"需要从当前用户全部可用日志模板中按名称查找稳定 templateId，或在提交日志前确认模板是否存在时使用。",
		reportTemplateSearchResult(), nil,
		[]contract.ParamDecl{{Name: "query", Property: "report_template_name"}},
		"dws report +template-search --query 周报",
	),
	Flags:       []shortcut.Flag{{Name: "query", Type: shortcut.FlagString, Desc: "模板名称关键词，不区分大小写；--query 不能为空", Required: true}},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"query"}, Description: "--query 不能为空"}},
	Tips:        []string{"dws report +template-search --query 周报"},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if strings.TrimSpace(rt.Str("query")) == "" {
			return apperrors.NewValidation("--query 不能为空", apperrors.WithReason("empty_query"))
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		const operation = "report/get_available_report_templates"
		data, err := rt.CallMCPData("report", "get_available_report_templates", map[string]any{})
		if err != nil {
			return err
		}
		templates, err := reportProjectTemplates(data, operation)
		if err != nil {
			return err
		}
		query := strings.ToLower(strings.TrimSpace(rt.Str("query")))
		matches := make([]map[string]any, 0)
		for _, template := range templates {
			if strings.Contains(strings.ToLower(template["name"].(string)), query) {
				matches = append(matches, template)
			}
		}
		payload := map[string]any{"count": len(matches), "templates": matches}
		if !output.UsesUnifiedResult(rt.Command()) {
			return rt.Output(payload)
		}
		meta := &output.Meta{Count: output.NewCount(len(matches))}
		return output.StoreResult(rt.Command().Context(), output.Success(payload, output.WithMeta(meta)))
	},
}

func init() {
	shortcut.Register(InboxList, OutboxList, TemplateSearch)
}
