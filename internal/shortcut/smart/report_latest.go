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

package smart

import (
	"strconv"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ReportLatest: fetch the detail of MY most recently submitted report (日志/汇报)
// in one step (DingTalk-native).
//
// Steps:
//
//  1. list the reports I sent via get_send_report_list. cursor/size/startTime/
//     endTime mirror helpers.runReportSent — the server caps a single query span
//     at 20 days, so we default to the last-20-days window just like the helper.
//     An optional --keyword is passed through as report_template_name (the same
//     filter the helper exposes as --template-name).
//
//  2. locate the newest entry: the item with the largest create time
//     (createTime/gmtCreate/sendTime), falling back to the first item that
//     carries a reportId (lists come back newest-first).
//
//  3. print that report's body via get_report_entry_details (report_id param
//     mirrors helpers.runReportDetail). If nothing carries a reportId we fall
//     back to printing the picked list row via rt.Output.
//
// If I have not sent any report in the window it reports "暂无日志" instead of
// failing obscurely.
//
//	dws report +report-latest
//	dws report +report-latest --keyword 日报
var ReportLatest = shortcut.Shortcut{
	Service:     "report",
	Command:     "+report-latest",
	Product:     "report",
	Description: "取我最新提交的一篇日志/汇报详情（钉钉原生）",
	Intent: "当你只想快速看回自己最近发出的一篇日志/汇报，却不想先翻发件箱列表、复制 reportId 再查详情时使用；" +
		"内部先列出你近期发出的日志（默认最近 20 天，服务端单次查询跨度上限 20 天；可用 --keyword 按日志模板名过滤），" +
		"自动挑出创建时间最新的一条，再拉取它的正文详情（字段明细、发送人、时间、钉钉跳转链接等）。" +
		"这是只读操作，不会创建或修改任何日志；若这段时间内你没有发出过日志则提示「暂无日志」。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "keyword", Type: shortcut.FlagString, Desc: "按日志模板名过滤（可选，对应 report outbox list 的 --template-name）", Required: false},
	},
	Tips: []string{
		`dws report +report-latest`,
		`dws report +report-latest --keyword 日报`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — list the reports I sent. Mirror helpers.runReportSent: the
		// server caps a single get_send_report_list query at a 20-day span, so
		// default startTime/endTime to the last-20-days window. cursor/size are
		// floats and report_template_name is the optional keyword filter.
		now := time.Now()
		startMs := now.AddDate(0, 0, -20).Truncate(24 * time.Hour).UnixMilli()
		endMs := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location()).UnixMilli()

		listArgs := map[string]any{
			"cursor":    float64(0),
			"size":      float64(20),
			"startTime": float64(startMs),
			"endTime":   float64(endMs),
		}
		if kw := strings.TrimSpace(rt.Str("keyword")); kw != "" {
			listArgs["report_template_name"] = kw
		}

		data, err := rt.CallMCPData("report", "get_send_report_list", listArgs)
		if err != nil {
			return err
		}

		// Step 2 — pick the newest sent report.
		row, reportID := shortcutReportLatestPick(data)
		if row == nil {
			return apperrors.NewValidation("暂无日志")
		}

		// Step 3 — print its detail (report_id mirrors helpers.runReportDetail).
		// If we somehow could not read a reportId, fall back to the list row.
		if reportID == "" {
			return rt.Output(row)
		}
		return rt.CallMCP("get_report_entry_details", map[string]any{
			"report_id": reportID,
		})
	},
}

// shortcutReportLatestPick walks a get_send_report_list response, finds the
// report entries, and returns the newest one (largest create time, falling back
// to the first entry that carries a reportId) together with its reportId.
// Returns (nil, "") when there are no entries.
func shortcutReportLatestPick(data map[string]any) (map[string]any, string) {
	items := shortcutReportLatestItems(data)
	if len(items) == 0 {
		return nil, ""
	}

	var firstItem map[string]any
	firstID := ""
	var bestItem map[string]any
	bestID := ""
	var bestTime int64
	haveTime := false

	for _, m := range items {
		id := shortcutReportLatestID(m)
		if id == "" {
			continue
		}
		if firstItem == nil {
			firstItem = m
			firstID = id
		}
		if t, ok := shortcutReportLatestCreateMillis(m); ok {
			if !haveTime || t > bestTime {
				haveTime = true
				bestTime = t
				bestItem = m
				bestID = id
			}
		}
	}

	if haveTime && bestItem != nil {
		return bestItem, bestID
	}
	if firstItem != nil {
		return firstItem, firstID
	}
	// No entry carried a reportId; still surface the first raw item so the
	// caller can print something rather than mis-report "暂无日志".
	return items[0], ""
}

// shortcutReportLatestItems pulls the report entries out of a
// get_send_report_list response, probing the same container keys the helper's
// findReportListItems uses (list/items/data/reports/records/report_list/
// reportList nested under the top level or under result).
func shortcutReportLatestItems(data map[string]any) []map[string]any {
	keys := []string{"list", "items", "data", "reports", "records", "report_list", "reportList", "result"}
	for _, key := range keys {
		if arr, ok := data[key].([]any); ok {
			return shortcutReportLatestToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range []string{"list", "items", "data", "reports", "records", "report_list", "reportList"} {
				if arr, ok := inner[k2].([]any); ok {
					return shortcutReportLatestToMaps(arr)
				}
			}
		}
	}
	return nil
}

func shortcutReportLatestToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// shortcutReportLatestID reads an entry's reportId, mirroring the key aliases in
// helpers.reportIDFromMap.
func shortcutReportLatestID(m map[string]any) string {
	for _, key := range []string{"reportId", "reportID", "report_id", "report_Id", "report-id"} {
		if id, ok := m[key].(string); ok {
			if id = strings.TrimSpace(id); id != "" {
				return id
			}
		}
	}
	return ""
}

// shortcutReportLatestCreateMillis reads an entry's create time in millis,
// mirroring the create-time keys helpers.reportReadableCreateTimeMillis probes.
func shortcutReportLatestCreateMillis(m map[string]any) (int64, bool) {
	for _, key := range []string{"createTime", "gmtCreate", "sendTime", "time", "modifiedTime"} {
		switch v := m[key].(type) {
		case float64:
			if v > 0 {
				return int64(v), true
			}
		case string:
			if s := strings.TrimSpace(v); s != "" {
				if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
					return n, true
				}
			}
		}
	}
	return 0, false
}

func init() {
	shortcut.Register(ReportLatest)
}
