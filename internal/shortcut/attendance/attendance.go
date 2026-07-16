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

// Package attendance provides declarative shortcuts for the DingTalk attendance
// (考勤) service: attendance records, check results/records, approvals, shifts,
// schedules, class/group/rule management, statistics, settings, reports and
// vacation. Each shortcut maps 1:1 onto an MCP tool declared in
// internal/helpers/attendance.go.
//
// Note: most attendance MCP tools are served by the "attendance-wukong" MCP
// server; a few (get_user_attendance_record, batch_get_employee_shifts,
// query_attendance_group_or_rules) are served by the default "attendance"
// server. The Product field of each shortcut reflects the ground-truth server.
package attendance

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const (
	serverDefault = "attendance"
	serverWukong  = "attendance-wukong"
)

// ── shared helpers ──────────────────────────────────────────

// dayMillis parses a YYYY-MM-DD date into a local millisecond timestamp.
func dayMillis(s string) (int64, error) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.Local)
	if err != nil {
		return 0, fmt.Errorf("日期格式错误，应为 YYYY-MM-DD（如 2026-04-01）: %w", err)
	}
	return t.UnixMilli(), nil
}

// dayDateTime parses a YYYY-MM-DD date and renders it as "yyyy-MM-dd HH:mm:ss".
func dayDateTime(s string) (string, error) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.Local)
	if err != nil {
		return "", fmt.Errorf("日期格式错误，应为 YYYY-MM-DD（如 2026-04-01）: %w", err)
	}
	return t.Format("2006-01-02 15:04:05"), nil
}

// flexMillis parses YYYY-MM-DD or "yyyy-MM-dd HH:mm:ss" into a local ms timestamp.
func flexMillis(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t.UnixMilli(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t.UnixMilli(), nil
	}
	return 0, fmt.Errorf("日期格式错误，应为 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss")
}

// flexDateTime parses YYYY-MM-DD or "yyyy-MM-dd HH:mm:ss" into "yyyy-MM-dd HH:mm:ss".
func flexDateTime(s string) (string, error) {
	s = strings.TrimSpace(s)
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t.Format("2006-01-02 15:04:05"), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t.Format("2006-01-02 15:04:05"), nil
	}
	return "", fmt.Errorf("日期格式错误，应为 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss")
}

// dateToMillis parses YYYY-MM-DD / datetime; when endOfDay is set a bare date
// resolves to 23:59:59 (mirrors helper parseDateToTimestamp "end" behaviour).
func dateToMillis(s string, endOfDay bool) (int64, error) {
	s = strings.TrimSpace(s)
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t.UnixMilli(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		if endOfDay {
			return t.Add(23*time.Hour + 59*time.Minute + 59*time.Second).UnixMilli(), nil
		}
		return t.UnixMilli(), nil
	}
	return 0, fmt.Errorf("日期格式错误，应为 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss")
}

// convertClassCheckTime converts "HH:mm" checkTime strings inside a classVO to
// millisecond offsets (1970-01-01 HH:mm in UTC+8), mirroring the helper.
func convertClassCheckTime(classVO map[string]any) {
	cst := time.FixedZone("CST", 8*3600)
	convertOne := func(obj map[string]any) {
		v, ok := obj["checkTime"]
		if !ok || v == nil {
			return
		}
		if ct, ok := v.(string); ok {
			ct = strings.TrimSpace(ct)
			if t, err := time.ParseInLocation("2006-01-02 15:04", "1970-01-01 "+ct, cst); err == nil {
				obj["checkTime"] = float64(t.UnixMilli())
			}
		}
	}
	if sections, ok := classVO["sections"].([]any); ok {
		for _, sec := range sections {
			if secMap, ok := sec.(map[string]any); ok {
				if times, ok := secMap["times"].([]any); ok {
					for _, t := range times {
						if tMap, ok := t.(map[string]any); ok {
							convertOne(tMap)
						}
					}
				}
			}
		}
	}
	if setting, ok := classVO["setting"].(map[string]any); ok {
		if restList, ok := setting["topRestTimeList"].([]any); ok {
			for _, item := range restList {
				if itemMap, ok := item.(map[string]any); ok {
					convertOne(itemMap)
				}
			}
		}
	}
}

// approveTypeMapping maps query keywords to bizType numbers (query_user_approve).
var approveTypeMapping = map[string]int{
	"overtime": 1, "加班": 1,
	"trip": 2, "travel": 2, "business_trip": 2, "business-trip": 2, "出差": 2, "外出": 2,
	"leave": 3, "请假": 3,
	"patch": 4, "repair_check": 4, "repair-check": 4, "补卡": 4,
}

// approveTemplateTypeMapping maps keywords to template approveType strings.
var approveTemplateTypeMapping = map[string]string{
	"repair_check": "REPAIR_CHECK", "repair-check": "REPAIR_CHECK", "patch": "REPAIR_CHECK", "补卡": "REPAIR_CHECK",
	"leave": "LEAVE", "请假": "LEAVE",
	"overtime": "OVERTIME", "加班": "OVERTIME",
	"travel": "TRAVEL", "外出": "TRAVEL",
	"out": "OUT", "trip": "OUT", "business_trip": "OUT", "business-trip": "OUT", "出差": "OUT",
}

// ── record ──────────────────────────────────────────────────

// GetRecord 查询个人考勤详情（get_user_attendance_record）。
// ── check ───────────────────────────────────────────────────

// CheckResult 查询打卡结果（query_check_result）。
var CheckResult = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+check-result",
	Product:     serverWukong,
	Description: "查询用户打卡结果（迟到/早退/缺卡等）",
	Intent:      "当你需要批量统计一批员工在某段时间的打卡结果（正常/迟到/早退/缺卡等判定结论）时使用，例如做月度考勤汇总或异常人员排查；输入多个 userId（最多 100 人）和不超过 1 个月的日期区间，返回每人每天的打卡结果状态，支持分页。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "用户 userId 列表，逗号分隔，最多 100 人", Required: true},
		{Name: "start", Type: shortcut.FlagString, Desc: "起始日期 YYYY-MM-DD", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束日期 YYYY-MM-DD，跨度不超过 1 个月", Required: true},
		{Name: "offset", Type: shortcut.FlagInt, Default: "0", Desc: "分页偏移量，默认 0"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "100", Desc: "分页大小，1-1000，默认 100"},
	},
	Tips: []string{`dws attendance +check-result --users userId1,userId2 --start 2026-04-01 --end 2026-04-30 --limit 50`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		from, err := dayDateTime(rt.Str("start"))
		if err != nil {
			return err
		}
		to, err := dayDateTime(rt.Str("end"))
		if err != nil {
			return err
		}
		return rt.CallMCP("query_check_result", map[string]any{
			"QueryCheckResultRequest": map[string]any{
				"userIds":      rt.StrSlice("users"),
				"workDateFrom": from,
				"workDateTo":   to,
				"offset":       rt.Int("offset"),
				"limit":        rt.Int("limit"),
			},
		})
	},
}

// CheckRecord 查询打卡流水（query_check_record）。
var CheckRecord = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+check-record",
	Product:     serverWukong,
	Description: "查询用户打卡流水（打卡时间/地点/定位方式）",
	Intent:      "当你要查看员工每一次实际打卡的原始记录（具体打卡时刻、打卡地点、定位/Wifi/蓝牙等方式）而不是判定结论时使用，例如核实某人是否在指定地点打卡；输入 userId 列表和不超过 1 个月的日期区间，返回逐条打卡流水。与 +check-result 的区别是这里返回明细流水而非迟到早退结论。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "用户 userId 列表，逗号分隔", Required: true},
		{Name: "start", Type: shortcut.FlagString, Desc: "起始日期 YYYY-MM-DD", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束日期 YYYY-MM-DD，跨度不超过 1 个月", Required: true},
	},
	Tips: []string{`dws attendance +check-record --users userId1 --start 2026-04-01 --end 2026-04-30`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		from, err := dayDateTime(rt.Str("start"))
		if err != nil {
			return err
		}
		to, err := dayDateTime(rt.Str("end"))
		if err != nil {
			return err
		}
		return rt.CallMCP("query_check_record", map[string]any{
			"QueryCheckRecordRequest": map[string]any{
				"userIds":       rt.StrSlice("users"),
				"checkDateFrom": from,
				"checkDateTo":   to,
			},
		})
	},
}

// ── approve ─────────────────────────────────────────────────

// ListApprove 查询用户考勤审批单（query_user_approve）。
var ListApprove = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+list-approve",
	Product:     serverWukong,
	Description: "查询用户考勤审批单（补卡/加班/请假/出差外出）",
	Intent:      "当你想查看某些员工已提交的考勤类审批单（加班、请假、出差外出、补卡）时使用，例如核对某人这段时间请了几次假或有没有补卡审批；输入 userId 列表、审批类型（overtime/加班、leave/请假、trip/出差外出、patch/补卡）和日期区间，返回匹配的审批单记录。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "用户 userId 列表，逗号分隔", Required: true},
		{Name: "types", Type: shortcut.FlagStringSlice, Desc: "审批类型，逗号分隔：overtime/加班、trip/travel/出差/外出、leave/请假、patch/补卡", Required: true},
		{Name: "start", Type: shortcut.FlagString, Desc: "起始日期 YYYY-MM-DD", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束日期 YYYY-MM-DD", Required: true},
	},
	Tips: []string{`dws attendance +list-approve --users userId1 --types overtime,leave --start 2026-04-01 --end 2026-04-30`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		from, err := dayDateTime(rt.Str("start"))
		if err != nil {
			return err
		}
		to, err := dayDateTime(rt.Str("end"))
		if err != nil {
			return err
		}
		var bizTypes []int
		for _, t := range rt.StrSlice("types") {
			s := strings.TrimSpace(t)
			if s == "" {
				continue
			}
			bizType, ok := approveTypeMapping[strings.ToLower(s)]
			if !ok {
				return fmt.Errorf("无效的审批类型: %s，支持: overtime/加班、trip/travel/出差/外出、leave/请假、patch/补卡", s)
			}
			bizTypes = append(bizTypes, bizType)
		}
		return rt.CallMCP("query_user_approve", map[string]any{
			"QueryUserApproveRequest": map[string]any{
				"userIds":  rt.StrSlice("users"),
				"bizTypes": bizTypes,
				"fromDate": from,
				"toDate":   to,
			},
		})
	},
}

// GetApproveTemplate 查询考勤审批提交链接（query_at_approve_template）。
var GetApproveTemplate = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-approve-template",
	Product:     serverWukong,
	Description: "查询补卡/请假/加班/外出/出差审批提交链接",
	Intent:      "当用户想自己发起一条考勤审批（补卡、请假、加班、外出、出差）、需要拿到对应审批表单的提交入口链接时使用；输入审批类型（如 leave/请假），返回可直接打开填写并提交的审批链接。本命令只返回链接、不代替用户提交审批。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "type", Type: shortcut.FlagString, Desc: "审批类型：repair-check/补卡、leave/请假、overtime/加班、travel/外出、out/出差（或 REPAIR_CHECK/LEAVE/OVERTIME/TRAVEL/OUT）", Required: true},
	},
	Tips: []string{`dws attendance +get-approve-template --type leave`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		input := strings.TrimSpace(rt.Str("type"))
		approveType, ok := approveTemplateTypeMapping[strings.ToLower(input)]
		if !ok {
			approveType = strings.ToUpper(input)
		}
		switch approveType {
		case "REPAIR_CHECK", "LEAVE", "OVERTIME", "TRAVEL", "OUT":
		default:
			return fmt.Errorf("无效的审批类型: %s，支持: repair-check/补卡、leave/请假、overtime/加班、travel/外出、out/出差，或 REPAIR_CHECK/LEAVE/OVERTIME/TRAVEL/OUT", input)
		}
		return rt.CallMCP("query_at_approve_template", map[string]any{
			"approveType": approveType,
		})
	},
}

// ── shift ───────────────────────────────────────────────────

// ListShift 批量查询员工班次信息（batch_get_employee_shifts）。
// ── schedule ────────────────────────────────────────────────

// GetSchedule 获取指定用户的排班记录（getScheduleByRange）。
var GetSchedule = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-schedule",
	Product:     serverWukong,
	Description: "获取指定用户一段时间内的排班记录",
	Intent:      "当你要查看排班制考勤组下员工在某段时间的排班记录（含排班 id、班次、是否休息）时使用，尤其是需要拿到排班 id 用于后续 BOSS 改签打卡（+boss-check）的场景；输入 userId 列表和起止时间，返回逐日排班明细。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "用户 userId 列表，逗号分隔", Required: true},
		{Name: "start", Type: shortcut.FlagString, Desc: "开始日期 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束日期 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss", Required: true},
	},
	Tips: []string{`dws attendance +get-schedule --users user001,user002 --start 2026-04-01 --end 2026-04-30`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		begin, err := dateToMillis(rt.Str("start"), false)
		if err != nil {
			return err
		}
		end, err := dateToMillis(rt.Str("end"), true)
		if err != nil {
			return err
		}
		return rt.CallMCP("getScheduleByRange", map[string]any{
			"GetScheduleByRangeRequest": map[string]any{
				"userIdList":    rt.StrSlice("users"),
				"workDateBegin": begin,
				"workDateEnd":   end,
			},
		})
	},
}

// ImportSchedule 导入排班记录到排班制考勤组（generateTurnSchedule）。
var ImportSchedule = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+import-schedule",
	Product:     serverWukong,
	Description: "导入排班记录到排班制考勤组",
	Intent:      "当管理员需要为排班制考勤组批量设置员工某几天上哪个班或休息时使用；输入考勤组 ID 和一组排班记录 JSON（含 userId/workDate/classId/isRest），会实际写入并生成员工的排班，直接影响这些人后续的打卡要求，务必确认 classId 和日期正确。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group-id", Type: shortcut.FlagInt, Desc: "考勤组 ID", Required: true},
		{Name: "schedules", Type: shortcut.FlagString, Desc: `排班记录 JSON 数组，每条含 userId/workDate(yyyy-MM-dd HH:mm:ss)/classId/isRest(Y|N)`, Required: true},
	},
	Tips: []string{`dws attendance +import-schedule --group-id 123456 --schedules '[{"userId":"user001","workDate":"2026-04-22 09:00:00","classId":123,"isRest":"N"}]'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		var scheduleVOS []map[string]any
		if err := json.Unmarshal([]byte(rt.Str("schedules")), &scheduleVOS); err != nil {
			return fmt.Errorf("invalid --schedules JSON: %w", err)
		}
		if len(scheduleVOS) == 0 {
			return fmt.Errorf("--schedules 至少需要一条排班记录")
		}
		required := []string{"userId", "workDate", "classId", "isRest"}
		for i, sched := range scheduleVOS {
			for _, field := range required {
				if _, ok := sched[field]; !ok {
					return fmt.Errorf("schedule[%d] 缺少必填字段: %s（必填: userId, workDate, classId, isRest）", i, field)
				}
			}
			if wd, ok := sched["workDate"].(string); ok {
				norm, err := flexDateTime(wd)
				if err != nil {
					return fmt.Errorf("schedule[%d] workDate 格式错误: %w", i, err)
				}
				sched["workDate"] = norm
			}
		}
		return rt.CallMCP("generateTurnSchedule", map[string]any{
			"GenerateTurnScheduleRequest": map[string]any{
				"groupId":     int64(rt.Int("group-id")),
				"scheduleVOS": scheduleVOS,
				"param": map[string]any{
					"useHistoryGroupAndShift": false,
				},
			},
		})
	},
}

// ── class ───────────────────────────────────────────────────

// SearchClass 查询当前用户可管理的班次列表（get_class_list）。
var SearchClass = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+search-class",
	Product:     serverWukong,
	Description: "查询当前用户可管理的班次详情列表",
	Intent:      "当你要浏览或按名称查找当前用户能管理的班次、以便拿到班次 ID 用于建组、排班或改班次时使用；可选按班次名关键字模糊搜索、按 ALL/我负责的过滤并分页，返回班次列表及详情。要看某个具体班次的完整配置用 +get-class。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "班次名称关键字，模糊搜索"},
		{Name: "filter-type", Type: shortcut.FlagString, Enum: []string{"ALL", "MINE_OWN"}, Desc: "班次类型：ALL 全部 / MINE_OWN 我负责的"},
		{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码，从 1 开始"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页条数，最大 200"},
	},
	Tips: []string{`dws attendance +search-class --query "早班" --filter-type MINE_OWN`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		pageQuery := map[string]any{}
		if v := rt.Int("page"); v > 0 {
			pageQuery["pageIndex"] = v
		}
		if v := rt.Int("limit"); v > 0 {
			pageQuery["pageSize"] = v
		}
		shiftParam := map[string]any{}
		if v := rt.Str("query"); v != "" {
			shiftParam["searchName"] = v
		}
		if v := rt.Str("filter-type"); v != "" {
			shiftParam["filterType"] = v
		}
		params := map[string]any{}
		if len(pageQuery) > 0 {
			params["PageQuery"] = pageQuery
		}
		if len(shiftParam) > 0 {
			params["ShiftParamVO"] = shiftParam
		}
		data, err := rt.CallMCPData(serverWukong, "get_class_list", params)
		if err != nil {
			return err
		}
		classes := searchClassProject(data)
		return rt.Output(map[string]any{"count": len(classes), "classes": classes})
	},
}

// searchClassProject reshapes the raw get_class_list response into a clean class
// list ({classId, name, ownerName}) — clean output projection. Both
// the list container and per-item field names are probed defensively across
// candidate keys, so an unknown/empty shape yields an empty list rather than a
// crash or fabricated data.
func searchClassProject(data map[string]any) []map[string]any {
	raw := attendanceResolveList(data, "classList", "shiftList", "list", "items")
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := attendanceFirst(m, "id", "classId", "class_id"); ok {
			row["classId"] = v
		}
		if v, ok := attendanceFirst(m, "name", "className", "class_name"); ok {
			row["name"] = v
		}
		if v, ok := attendanceFirst(m, "ownerName", "owner_name", "owner"); ok {
			row["ownerName"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// attendanceResolveList locates the list payload inside a response, tolerating a
// bare top-level array under a common container key or nesting one level deeper
// under result/data. extraKeys are probed ahead of the generic containers.
func attendanceResolveList(data map[string]any, extraKeys ...string) []any {
	keys := append(append([]string{}, extraKeys...), "result", "data", "list", "items")
	for _, key := range keys {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range append(append([]string{}, extraKeys...), "list", "items", "result", "data") {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// attendanceFirst returns the first present candidate key's value.
func attendanceFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// GetClass 根据班次 ID 查询班次详情（get_class_detail）。
var GetClass = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-class",
	Product:     serverWukong,
	Description: "根据班次 ID 查询班次详情",
	Intent:      "当你已知某个班次的 ID、想查看它的完整配置（上下班时间段、休息时段、负责人等）时使用，例如更新班次前先读取现状；输入班次 ID，返回该班次的详细设置。若不知道班次 ID，先用 +search-class 搜索。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "class-id", Type: shortcut.FlagInt, Desc: "班次 ID", Required: true},
	},
	Tips: []string{`dws attendance +get-class --class-id 1170996821`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_class_detail", map[string]any{
			"classId": int64(rt.Int("class-id")),
		})
	},
}

// CreateClass 创建班次（create_class_setting）。
var CreateClass = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+create-class",
	Product:     serverWukong,
	Description: "创建班次（checkTime 用 HH:mm，自动转时间戳）",
	Intent:      "当你要新建一个考勤班次（定义几点上班、几点下班、休息时段等）供后续排班或考勤组使用时使用；输入班次名称和包含 sections 上下班时间段的 class-vo JSON（checkTime 用 HH:mm），会实际在企业里创建一个新班次。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "班次名称", Required: true},
		{Name: "owner", Type: shortcut.FlagString, Desc: "班次负责人 userId"},
		{Name: "class-vo", Type: shortcut.FlagString, Desc: `完整 TopAtClassVO JSON，必须含 sections（如 {"sections":[{"times":[{"checkType":"OnDuty","checkTime":"09:00","across":0},{"checkType":"OffDuty","checkTime":"18:00","across":0}]}]}）`, Required: true},
	},
	Tips: []string{`dws attendance +create-class --name "早班" --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"08:00","across":0},{"checkType":"OffDuty","checkTime":"17:00","across":0}]}]}'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		classVO := map[string]any{}
		if raw := rt.Str("class-vo"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &classVO); err != nil {
				return fmt.Errorf("invalid --class-vo JSON: %w", err)
			}
		}
		if rt.Changed("name") {
			classVO["name"] = rt.Str("name")
		}
		if rt.Changed("owner") {
			classVO["owner"] = rt.Str("owner")
		}
		if classVO["name"] == nil || classVO["name"] == "" {
			return fmt.Errorf("--name 是必填项，请指定班次名称")
		}
		if classVO["sections"] == nil {
			return fmt.Errorf("--class-vo 中必须包含 sections 字段（班次上下班时间段）")
		}
		convertClassCheckTime(classVO)
		return rt.CallMCP("create_class_setting", map[string]any{
			"TopAtClassVO": classVO,
		})
	},
}

// UpdateClass 更新班次（update_class_setting）。
var UpdateClass = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+update-class",
	Product:     serverWukong,
	Description: "更新已有班次（仅传要修改的字段）",
	Intent:      "当你要修改一个已存在班次的配置（改名、换负责人或调整上下班时间段）时使用；输入班次 ID 和需要变更的字段（其余保持原值），会实际更新该班次设置，进而影响所有使用该班次的员工打卡要求。建议先用 +get-class 确认现状。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "class-id", Type: shortcut.FlagInt, Desc: "班次 ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "班次名称（不传则保持原值）"},
		{Name: "owner", Type: shortcut.FlagString, Desc: "班次负责人 userId（不传则保持原值）"},
		{Name: "class-vo", Type: shortcut.FlagString, Desc: "完整 TopAtClassVO JSON（checkTime 用 HH:mm，自动转时间戳）"},
	},
	Tips: []string{`dws attendance +update-class --class-id 1170996821 --name "新早班"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		classVO := map[string]any{}
		if raw := rt.Str("class-vo"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &classVO); err != nil {
				return fmt.Errorf("invalid --class-vo JSON: %w", err)
			}
		}
		classVO["id"] = int64(rt.Int("class-id"))
		if rt.Changed("name") {
			classVO["name"] = rt.Str("name")
		}
		if rt.Changed("owner") {
			classVO["owner"] = rt.Str("owner")
		}
		convertClassCheckTime(classVO)
		return rt.CallMCP("update_class_setting", map[string]any{
			"TopAtClassVO": classVO,
		})
	},
}

// ── adjustment rule ─────────────────────────────────────────

// GetAdjustmentRule 查询补卡规则详情（get_adjustment_rule_detail）。
var GetAdjustmentRule = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-adjustment-rule",
	Product:     serverWukong,
	Description: "根据补卡规则主键 ID 查询补卡规则详情",
	Intent:      "当你已知某条补卡规则的主键 ID、想查看它的具体规则内容（每月可补卡次数、时限、适用范围等）时使用；输入 adjustmentId，返回该补卡规则的完整详情。不知道 ID 时先用 +search-adjustment-rule 列出。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "adjustment-id", Type: shortcut.FlagInt, Desc: "补卡规则主键 ID", Required: true},
	},
	Tips: []string{`dws attendance +get-adjustment-rule --adjustment-id 12345`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_adjustment_rule_detail", map[string]any{
			"adjustmentId": int64(rt.Int("adjustment-id")),
		})
	},
}

// SearchAdjustmentRule 查询补卡规则列表（get_adjustment_rule）。
var SearchAdjustmentRule = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+search-adjustment-rule",
	Product:     serverWukong,
	Description: "查询当前用户可管理的补卡规则列表",
	Intent:      "当你要浏览或按名称查找当前用户可管理的补卡规则、以便拿到规则 ID 做进一步查看时使用；可选按规则名关键字模糊搜索并分页，返回补卡规则列表。要看某条规则的完整内容用 +get-adjustment-rule。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "补卡规则名称关键字，模糊搜索"},
		{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码，从 1 开始"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页条数，200 以内"},
	},
	Tips: []string{`dws attendance +search-adjustment-rule --query "标准" --page 1 --limit 50`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		param := map[string]any{}
		if v := rt.Str("query"); v != "" {
			param["name"] = v
		}
		page := rt.Int("page")
		if page <= 0 {
			page = 1
		}
		limit := rt.Int("limit")
		if limit <= 0 {
			limit = 20
		}
		param["currentPage"] = page
		param["pageSize"] = limit
		data, err := rt.CallMCPData(serverWukong, "get_adjustment_rule", map[string]any{
			"ATRuleQueryParam": param,
		})
		if err != nil {
			return err
		}
		rules := searchRuleProject(data)
		return rt.Output(map[string]any{"count": len(rules), "rules": rules})
	},
}

// searchRuleProject reshapes the raw get_adjustment_rule / get_overtime_rule
// response into a clean rule list ({ruleId, name}) — output-projection fidelity
// for clean output. The list container and per-item field names are probed defensively
// across candidate keys, so an unknown/empty shape yields an empty list rather
// than a crash or fabricated data.
func searchRuleProject(data map[string]any) []map[string]any {
	raw := attendanceResolveList(data, "ruleList", "rules", "list", "items")
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := attendanceFirst(m, "id", "ruleId", "rule_id", "adjustmentId", "overtimeId"); ok {
			row["ruleId"] = v
		}
		if v, ok := attendanceFirst(m, "name", "ruleName", "rule_name"); ok {
			row["name"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// ── overtime rule ───────────────────────────────────────────

// GetOvertimeRule 查询加班规则详情（get_overtime_rule_detail）。
var GetOvertimeRule = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-overtime-rule",
	Product:     serverWukong,
	Description: "根据加班规则主键 ID 查询加班规则详情",
	Intent:      "当你已知某条加班规则的主键 ID、想查看它的具体内容（工作日/休息日/节假日加班计算方式、适用范围等）时使用；输入 overtimeId，返回该加班规则的完整详情。不知道 ID 时先用 +search-overtime-rule 列出。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "overtime-id", Type: shortcut.FlagInt, Desc: "加班规则主键 ID", Required: true},
	},
	Tips: []string{`dws attendance +get-overtime-rule --overtime-id 12345`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_overtime_rule_detail", map[string]any{
			"overtimeId": int64(rt.Int("overtime-id")),
		})
	},
}

// SearchOvertimeRule 查询加班规则列表（get_overtime_rule）。
var SearchOvertimeRule = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+search-overtime-rule",
	Product:     serverWukong,
	Description: "查询当前用户可管理的加班规则列表",
	Intent:      "当你要浏览或按名称查找当前用户可管理的加班规则、以便拿到规则 ID 做进一步查看时使用；可选按规则名关键字模糊搜索并分页，返回加班规则列表。要看某条规则的完整内容用 +get-overtime-rule。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "加班规则名称关键字，模糊搜索"},
		{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码，从 1 开始"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页条数，200 以内"},
	},
	Tips: []string{`dws attendance +search-overtime-rule --query "节假日" --page 1 --limit 50`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		param := map[string]any{}
		if v := rt.Str("query"); v != "" {
			param["name"] = v
		}
		page := rt.Int("page")
		if page <= 0 {
			page = 1
		}
		limit := rt.Int("limit")
		if limit <= 0 {
			limit = 20
		}
		param["currentPage"] = page
		param["pageSize"] = limit
		data, err := rt.CallMCPData(serverWukong, "get_overtime_rule", map[string]any{
			"ATRuleQueryParam": param,
		})
		if err != nil {
			return err
		}
		rules := searchRuleProject(data)
		return rt.Output(map[string]any{"count": len(rules), "rules": rules})
	},
}

// ── group ───────────────────────────────────────────────────

// SearchGroup 查询当前用户可管理的考勤组列表（get_simple_groups）。
var SearchGroup = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+search-group",
	Product:     serverWukong,
	Description: "查询当前用户可管理的考勤组列表",
	Intent:      "当你要浏览或按名称查找当前用户可管理的考勤组、以便拿到考勤组 ID 用于查看详情、改成员或改配置时使用；可选按名称关键字、考勤组类型（固定班制/排班制/自由工时）过滤，并可选带出定位/Wifi/蓝牙信息，分页返回考勤组简要列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "考勤组名称关键字，模糊搜索"},
		{Name: "type", Type: shortcut.FlagString, Enum: []string{"FIXED", "TURN", "NONE"}, Desc: "考勤组类型：FIXED 固定班制 / TURN 排班制 / NONE 自由工时"},
		{Name: "query-position", Type: shortcut.FlagBool, Desc: "是否查询地理定位和 Wifi 名称"},
		{Name: "query-ble", Type: shortcut.FlagBool, Desc: "是否查询蓝牙设备列表"},
		{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码，从 1 开始"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页条数，200 以内"},
	},
	Tips: []string{`dws attendance +search-group --query "研发" --type FIXED --limit 50`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		searchParam := map[string]any{}
		if v := rt.Str("query"); v != "" {
			searchParam["queryStr"] = v
		}
		if v := rt.Str("type"); v != "" {
			searchParam["type"] = v
		}
		if rt.Changed("query-position") {
			searchParam["queryPositionAndWifiNames"] = rt.Bool("query-position")
		}
		if rt.Changed("query-ble") {
			searchParam["queryBleDeviceList"] = rt.Bool("query-ble")
		}
		if len(searchParam) == 0 {
			searchParam["queryPositionAndWifiNames"] = false
			searchParam["queryBleDeviceList"] = false
		}
		page := rt.Int("page")
		if page <= 0 {
			page = 1
		}
		limit := rt.Int("limit")
		if limit <= 0 {
			limit = 20
		}
		data, err := rt.CallMCPData(serverWukong, "get_simple_groups", map[string]any{
			"param": searchParam,
			"pageQuery": map[string]any{
				"pageIndex": page,
				"pageSize":  limit,
			},
		})
		if err != nil {
			return err
		}
		groups := searchGroupProject(data)
		return rt.Output(map[string]any{"count": len(groups), "groups": groups})
	},
}

// searchGroupProject reshapes the raw get_simple_groups response into a clean
// attendance-group list ({groupId, name, type}) — clean output projection.
// The list container and per-item field names are probed defensively
// across candidate keys, so an unknown/empty shape yields an empty list rather
// than a crash or fabricated data.
func searchGroupProject(data map[string]any) []map[string]any {
	raw := attendanceResolveList(data, "groupList", "groups", "list", "items")
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := attendanceFirst(m, "id", "groupId", "group_id"); ok {
			row["groupId"] = v
		}
		if v, ok := attendanceFirst(m, "name", "groupName", "group_name"); ok {
			row["name"] = v
		}
		if v, ok := attendanceFirst(m, "type", "groupType", "group_type"); ok {
			row["type"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// GetGroup 根据考勤组 ID 查询考勤组全量信息（get_group_detail）。
var GetGroup = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-group",
	Product:     serverWukong,
	Description: "根据考勤组 ID 查询考勤组全量信息",
	Intent:      "当你已知考勤组 ID、想一次性看它的全部配置（成员、班次、打卡地点、Wifi、蓝牙、规则等）时使用，例如修改考勤组前先了解现状；输入考勤组 ID，返回该组的全量信息。只想取部分子集时用 +get-group-filtered。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group-id", Type: shortcut.FlagInt, Desc: "考勤组 ID", Required: true},
	},
	Tips: []string{`dws attendance +get-group --group-id 123456`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_group_detail", map[string]any{
			"groupId": int64(rt.Int("group-id")),
		})
	},
}

// GetGroupFiltered 按需查询考勤组成员/地址/蓝牙/Wifi（get_group_filtered_detail）。
var GetGroupFiltered = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-group-filtered",
	Product:     serverWukong,
	Description: "按需查询考勤组成员/打卡地址/蓝牙/Wifi 子集",
	Intent:      "当你只想要考勤组的某几类信息（成员、打卡地址、Wifi、蓝牙），不需要整组全量数据以减少返回体积时使用；输入考勤组 ID 并用开关指定要哪些子集，只返回勾选的部分。需要全部信息时用 +get-group。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group-id", Type: shortcut.FlagInt, Desc: "考勤组 ID", Required: true},
		{Name: "member", Type: shortcut.FlagBool, Desc: "是否查询考勤组成员信息"},
		{Name: "position", Type: shortcut.FlagBool, Desc: "是否查询打卡地址"},
		{Name: "wifi", Type: shortcut.FlagBool, Desc: "是否查询打卡 Wifi"},
		{Name: "bles", Type: shortcut.FlagBool, Desc: "是否查询打卡蓝牙"},
	},
	Tips: []string{`dws attendance +get-group-filtered --group-id 123456 --member --position`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_group_filtered_detail", map[string]any{
			"groupId": int64(rt.Int("group-id")),
			"GroupResultFilter": map[string]any{
				"includeMember":   rt.Bool("member"),
				"includePosition": rt.Bool("position"),
				"includeWifi":     rt.Bool("wifi"),
				"includeBles":     rt.Bool("bles"),
			},
		})
	},
}

// UpdateGroupMembers 更新考勤组成员（update_group_member）。
var UpdateGroupMembers = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+update-group-members",
	Product:     serverWukong,
	Description: "更新考勤组成员（增删考勤人员/部门/无需考勤人员）",
	Intent:      "当你要把员工或部门加入/移出某个考勤组，或调整无需考勤人员名单时使用，例如新人入职后加入考勤组、离职后移出；输入考勤组 ID 和要增删的 userId/部门列表，会实际变更该考勤组成员，直接影响这些人是否需要打卡。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group-id", Type: shortcut.FlagInt, Desc: "考勤组 ID", Required: true},
		{Name: "add-users", Type: shortcut.FlagStringSlice, Desc: "添加考勤人员 userId 列表，逗号分隔，最多 20 个"},
		{Name: "remove-users", Type: shortcut.FlagStringSlice, Desc: "删除考勤人员 userId 列表，逗号分隔，最多 20 个"},
		{Name: "add-extra-users", Type: shortcut.FlagStringSlice, Desc: "添加无需考勤人员 userId 列表，逗号分隔，最多 20 个"},
		{Name: "remove-extra-users", Type: shortcut.FlagStringSlice, Desc: "删除无需考勤成员 userId 列表，逗号分隔，最多 20 个"},
		{Name: "add-depts", Type: shortcut.FlagStringSlice, Desc: "添加考勤部门 ID 列表，逗号分隔（全公司根部门 id 为 -1）"},
		{Name: "remove-depts", Type: shortcut.FlagStringSlice, Desc: "删除考勤部门 ID 列表，逗号分隔（全公司根部门 id 为 -1）"},
	},
	Tips: []string{`dws attendance +update-group-members --group-id 123456 --add-users userId1,userId2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		updateParam := map[string]any{}
		if v := rt.StrSlice("add-users"); len(v) > 0 {
			updateParam["addUsers"] = v
		}
		if v := rt.StrSlice("remove-users"); len(v) > 0 {
			updateParam["removeUsers"] = v
		}
		if v := rt.StrSlice("add-extra-users"); len(v) > 0 {
			updateParam["addExtraUsers"] = v
		}
		if v := rt.StrSlice("remove-extra-users"); len(v) > 0 {
			updateParam["removeExtraUsers"] = v
		}
		if v := rt.StrSlice("add-depts"); len(v) > 0 {
			updateParam["addDepts"] = v
		}
		if v := rt.StrSlice("remove-depts"); len(v) > 0 {
			updateParam["removeDepts"] = v
		}
		if len(updateParam) == 0 {
			return fmt.Errorf("至少需要指定一个变更项：--add-users / --remove-users / --add-extra-users / --remove-extra-users / --add-depts / --remove-depts")
		}
		return rt.CallMCP("update_group_member", map[string]any{
			"groupId":     int64(rt.Int("group-id")),
			"updateParam": updateParam,
		})
	},
}

// CreateGroup 创建考勤组（create_group_setting）。
var CreateGroup = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+create-group",
	Product:     serverWukong,
	Description: "创建考勤组（复杂子对象用 --group-vo JSON 传入）",
	Intent:      "当你要为团队新建一个考勤组（选定固定班制/排班制/自由工时、班次、打卡范围等）时使用；输入考勤组名称、类型和 group-vo JSON（固定班制须含 workDayClassList 和 defaultClassId），会实际创建一个新的考勤组。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "考勤组名称", Required: true},
		{Name: "type", Type: shortcut.FlagString, Enum: []string{"FIXED", "TURN", "NONE"}, Desc: "考勤组类型：FIXED 固定班制 / TURN 排班制 / NONE 自由工时", Required: true},
		{Name: "owner", Type: shortcut.FlagString, Desc: "考勤组主负责人 userId"},
		{Name: "group-vo", Type: shortcut.FlagString, Desc: `完整 groupVO JSON（FIXED 时须含 workDayClassList 和 defaultClassId）`},
	},
	Tips: []string{`dws attendance +create-group --name "研发考勤组" --type FIXED --group-vo '{"defaultClassId":1170996821,"workDayClassList":[0,1170996821,0,0,0,0,0]}'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		groupType := rt.Str("type")
		groupVO := map[string]any{}
		if raw := rt.Str("group-vo"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &groupVO); err != nil {
				return fmt.Errorf("invalid --group-vo JSON: %w", err)
			}
		}
		groupVO["name"] = rt.Str("name")
		groupVO["type"] = groupType
		if rt.Changed("owner") {
			groupVO["owner"] = rt.Str("owner")
		}
		if groupType == "FIXED" {
			workDayClassList, ok := groupVO["workDayClassList"]
			if !ok || workDayClassList == nil {
				return fmt.Errorf("固定班制(FIXED)时 --group-vo 必须包含 workDayClassList（工作日班次列表，不能为空）")
			}
			if list, ok := workDayClassList.([]any); ok && len(list) == 0 {
				return fmt.Errorf("workDayClassList 不能为空数组，至少需包含一个班次 ID")
			}
			if defaultClassID, ok := groupVO["defaultClassId"]; !ok || defaultClassID == nil {
				return fmt.Errorf("固定班制(FIXED)时 --group-vo 必须包含 defaultClassId（默认班次 ID）")
			}
		}
		return rt.CallMCP("create_group_setting", map[string]any{
			"groupVO": groupVO,
		})
	},
}

// UpdateGroup 更新考勤组配置（update_group_setting）。
var UpdateGroup = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+update-group",
	Product:     serverWukong,
	Description: "更新考勤组配置（仅修改需要变更的字段）",
	Intent:      "当你要修改已存在考勤组的配置（改名、改类型/负责人、是否允许外勤打卡、更换所选班次等）时使用；输入考勤组 ID 和需要变更的字段，会实际更新该组配置，影响组内成员的打卡规则。改成员用 +update-group-members，建议先用 +get-group 确认现状。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group-id", Type: shortcut.FlagInt, Desc: "考勤组 ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "考勤组名称"},
		{Name: "type", Type: shortcut.FlagString, Enum: []string{"FIXED", "TURN", "NONE"}, Desc: "考勤组类型：FIXED / TURN / NONE"},
		{Name: "owner", Type: shortcut.FlagString, Desc: "考勤组主负责人 userId"},
		{Name: "enable-outside-check", Type: shortcut.FlagString, Enum: []string{"true", "false"}, Desc: "是否允许外勤打卡，传 true 或 false"},
		{Name: "class-ids", Type: shortcut.FlagString, Desc: `所选班次 id 列表 JSON 数组，如 '[123,456]'`},
		{Name: "group-vo", Type: shortcut.FlagString, Desc: "完整 groupVO JSON，用于修改复杂子对象"},
	},
	Tips: []string{`dws attendance +update-group --group-id 123456 --name "研发考勤组"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		groupVO := map[string]any{}
		if raw := rt.Str("group-vo"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &groupVO); err != nil {
				return fmt.Errorf("invalid --group-vo JSON: %w", err)
			}
		}
		if rt.Changed("name") {
			groupVO["name"] = rt.Str("name")
		}
		if rt.Changed("type") {
			groupVO["type"] = rt.Str("type")
		}
		if rt.Changed("owner") {
			groupVO["owner"] = rt.Str("owner")
		}
		if rt.Changed("enable-outside-check") {
			v, err := strconv.ParseBool(rt.Str("enable-outside-check"))
			if err != nil {
				return fmt.Errorf("--enable-outside-check 值必须为 true 或 false")
			}
			groupVO["enableOutsideCheck"] = v
		}
		if rt.Changed("class-ids") {
			var ids []any
			if err := json.Unmarshal([]byte(rt.Str("class-ids")), &ids); err != nil {
				return fmt.Errorf("invalid --class-ids JSON array: %w", err)
			}
			groupVO["classIds"] = ids
		}
		if len(groupVO) == 0 {
			return fmt.Errorf("至少需要指定一个修改项：--name / --type / --owner / --enable-outside-check / --class-ids / --group-vo")
		}
		groupVO["id"] = int64(rt.Int("group-id"))
		return rt.CallMCP("update_group_setting", map[string]any{
			"groupId": int64(rt.Int("group-id")),
			"groupVO": groupVO,
		})
	},
}

// ── summary ─────────────────────────────────────────────────

// GetSummary 查询个人考勤统计摘要（get_user_attendance_summary）。
var GetSummary = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-summary",
	Product:     serverWukong,
	Description: "查询某个人的考勤统计摘要（周/月）",
	Intent:      "当你想快速了解某个人一周或一月的考勤汇总（出勤天数、迟到早退次数、加班、请假时长等统计口径）而不是逐天明细时使用；输入 userId、所在周期内的任意日期和统计类型（week/month），返回该周期的考勤统计摘要。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "user", Type: shortcut.FlagString, Desc: "钉钉用户 userId", Required: true},
		{Name: "date", Type: shortcut.FlagString, Desc: "查询日期 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss", Required: true},
		{Name: "stats-type", Type: shortcut.FlagString, Enum: []string{"week", "month"}, Desc: "统计类型：week 周统计 / month 月统计", Required: true},
	},
	Tips: []string{`dws attendance +get-summary --user USER_ID --date 2026-03-12 --stats-type week`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		queryDate, err := flexMillis(rt.Str("date"))
		if err != nil {
			return err
		}
		return rt.CallMCP("get_user_attendance_summary", map[string]any{
			"userId":    rt.Str("user"),
			"queryDate": queryDate,
			"statsType": rt.Str("stats-type"),
		})
	},
}

// ── rules ───────────────────────────────────────────────────

// GetRules 查询考勤组与考勤规则（query_attendance_group_or_rules）。
// ── self setting ────────────────────────────────────────────

// GetSelfSetting 查询个人规则设置（query_self_setting）。
var GetSelfSetting = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-self-setting",
	Product:     serverWukong,
	Description: "查询个人规则设置（打卡提醒/极速打卡/缺卡提醒等）",
	Intent:      "当你想查看某个用户在指定场景下的个人考勤规则开关配置（如打卡提醒、极速打卡、缺卡提醒、考勤结果通知等是否开启）时使用；输入设置场景 settingScene 和用户 userId，返回该用户对应场景的个人设置。查企业全局设置用 +get-global-setting。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "setting-scene", Type: shortcut.FlagString, Enum: []string{"checkRemind", "fastCheck", "checkResultNotify", "lackRemind", "personalAttendStatNotify", "bossAttendStatNotify"}, Desc: "查询设置项场景", Required: true},
		{Name: "user", Type: shortcut.FlagString, Desc: "查询用户 userId", Required: true},
	},
	Tips: []string{`dws attendance +get-self-setting --setting-scene checkRemind --user USER_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("query_self_setting", map[string]any{
			"RuleMcpQuerySelfSettingRequest": map[string]any{
				"settingScene": rt.Str("setting-scene"),
				"userId":       rt.Str("user"),
			},
		})
	},
}

// ── global setting ──────────────────────────────────────────

// GetGlobalSetting 查询全局规则设置（query_global_setting，仅管理员）。
var GetGlobalSetting = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-global-setting",
	Product:     serverWukong,
	Description: "查询全局规则设置（仅管理员）",
	Intent:      "当管理员想查看企业层面某场景的全局考勤规则设置（如全公司的打卡提醒、极速打卡、缺卡提醒等默认配置）时使用；输入设置场景 settingScene 和范围确认，返回企业全局设置。仅管理员可用；查某个人的设置用 +get-self-setting。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "setting-scene", Type: shortcut.FlagString, Enum: []string{"checkRemind", "fastCheck", "checkResultNotify", "lackRemind", "personalAttendStatNotify", "bossAttendStatNotify"}, Desc: "查询设置项场景", Required: true},
		{Name: "scope", Type: shortcut.FlagString, Enum: []string{"企业", "全公司", "所有人"}, Desc: "全局范围确认：企业/全公司/所有人", Required: true},
	},
	Tips: []string{`dws attendance +get-global-setting --scope 企业 --setting-scene checkRemind`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("query_global_setting", map[string]any{
			"RuleMcpQueryGlobalSettingRequest": map[string]any{
				"settingScene": rt.Str("setting-scene"),
			},
		})
	},
}

// ── report ──────────────────────────────────────────────────

// ListReportColumns 获取企业考勤字段列表（get_report_columns）。
var ListReportColumns = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+list-report-columns",
	Product:     serverWukong,
	Description: "获取企业考勤报表字段列表（仅管理员）",
	Intent:      "当你准备查考勤报表数据、需要先知道企业有哪些可查字段及其字段 ID（如出勤天数、迟到次数、加班时长等列）时使用；无需参数，返回全部报表字段列表，拿到 columnId 后再配合 +query-report-data 查具体数值。仅管理员可用。",
	Risk:        shortcut.RiskRead,
	Tips:        []string{`dws attendance +list-report-columns`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_report_columns", map[string]any{})
	},
}

// QueryReportData 根据字段查询考勤数据（get_report_columns_value）。
var QueryReportData = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+query-report-data",
	Product:     serverWukong,
	Description: "根据字段查询考勤报表数据（仅管理员）",
	Intent:      "当你要按指定考勤字段导出/查询一批员工在某时间段的报表数值（如各人的出勤天数、迟到次数、加班时长）时使用，常用于做考勤汇总或核算；输入 userId 列表（最多 20 人）、字段 ID 列表（来自 +list-report-columns）和不超过 32 天的时间区间，返回对应字段的数据。仅管理员可用。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "目标用户 userId 列表，逗号分隔，最多 20 人", Required: true},
		{Name: "columns", Type: shortcut.FlagStringSlice, Desc: "字段 ID 列表，逗号分隔（可用 +list-report-columns 获取）", Required: true},
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间 yyyy-MM-dd HH:mm:ss", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间 yyyy-MM-dd HH:mm:ss，跨度不超过 32 天", Required: true},
	},
	Tips: []string{`dws attendance +query-report-data --users userId1,userId2 --columns 1001,1002 --start "2026-03-01 00:00:00" --end "2026-03-31 23:59:59"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		startStr := rt.Str("start")
		endStr := rt.Str("end")
		if _, err := time.ParseInLocation("2006-01-02 15:04:05", startStr, time.Local); err != nil {
			return fmt.Errorf("--start 格式错误，应为 yyyy-MM-dd HH:mm:ss: %w", err)
		}
		if _, err := time.ParseInLocation("2006-01-02 15:04:05", endStr, time.Local); err != nil {
			return fmt.Errorf("--end 格式错误，应为 yyyy-MM-dd HH:mm:ss: %w", err)
		}
		var columnIds []int64
		for _, c := range rt.StrSlice("columns") {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			id, err := strconv.ParseInt(c, 10, 64)
			if err != nil {
				return fmt.Errorf("无效的字段 ID %q，必须为数字: %w", c, err)
			}
			columnIds = append(columnIds, id)
		}
		return rt.CallMCP("get_report_columns_value", map[string]any{
			"McpQueryParam": map[string]any{
				"targetUserIds": rt.StrSlice("users"),
				"columnIds":     columnIds,
				"fromDate":      startStr,
				"toDate":        endStr,
			},
		})
	},
}

// QueryReportLeave 查询用户假期数据（get_leave_time_by_leave_names）。
var QueryReportLeave = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+query-report-leave",
	Product:     serverWukong,
	Description: "查询用户假期数据（仅管理员）",
	Intent:      "当你要统计一批员工在某时间段内按假期类型的休假时长（如各人休了多少年假、病假）时使用，常用于假期核算或报表；输入 userId 列表（最多 20 人）、可选的假期名称列表（不填查全部）和不超过 32 天的时间区间，返回各假期类型的时长数据。仅管理员可用。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "目标用户 userId 列表，逗号分隔，最多 20 人", Required: true},
		{Name: "leave-names", Type: shortcut.FlagStringSlice, Desc: "假期类型名称列表，逗号分隔，不填则查询所有假期类型"},
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间 yyyy-MM-dd HH:mm:ss", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间 yyyy-MM-dd HH:mm:ss，跨度不超过 32 天", Required: true},
	},
	Tips: []string{`dws attendance +query-report-leave --users userId1 --leave-names 年假,病假 --start "2026-03-01 00:00:00" --end "2026-03-31 23:59:59"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		startStr := rt.Str("start")
		endStr := rt.Str("end")
		if _, err := time.ParseInLocation("2006-01-02 15:04:05", startStr, time.Local); err != nil {
			return fmt.Errorf("--start 格式错误，应为 yyyy-MM-dd HH:mm:ss: %w", err)
		}
		if _, err := time.ParseInLocation("2006-01-02 15:04:05", endStr, time.Local); err != nil {
			return fmt.Errorf("--end 格式错误，应为 yyyy-MM-dd HH:mm:ss: %w", err)
		}
		return rt.CallMCP("get_leave_time_by_leave_names", map[string]any{
			"McpLeaveQueryParam": map[string]any{
				"targetUserIds": rt.StrSlice("users"),
				"leaveNames":    rt.StrSlice("leave-names"),
				"fromDate":      startStr,
				"toDate":        endStr,
			},
		})
	},
}

// ── vacation ────────────────────────────────────────────────

// ListLeaveTypes 查询当前用户假期规则列表（get_leave_types）。
var ListLeaveTypes = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+list-leave-types",
	Product:     serverWukong,
	Description: "查询当前用户可用的假期规则列表",
	Intent:      "当你想知道企业有哪些假期类型（年假、事假、病假等）及其对应的假期编码 code、单位、是否带薪时使用，通常是查余额或改假期规则前先拿到 leaveCode；无需参数，返回当前用户可用的假期规则列表。",
	Risk:        shortcut.RiskRead,
	Tips:        []string{`dws attendance +list-leave-types`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_leave_types", map[string]any{
			"McpLeaveTypeRequest": map[string]any{},
		})
	},
}

// GetLeaveBalance 查询指定员工假期余额（get_leave_balance_quota）。
var GetLeaveBalance = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-leave-balance",
	Product:     serverWukong,
	Description: "查询指定员工的假期余额",
	Intent:      "当你想知道某些员工当前还剩多少假（如还有几天年假可用）时使用；输入 userId 列表，可选传假期规则 code 只查某类假（不传则返回所有假期类型的余额），返回各员工的假期剩余额度。假期 code 可用 +list-leave-types 获取。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "目标员工 userId 列表，逗号分隔", Required: true},
		{Name: "leave-code", Type: shortcut.FlagString, Desc: "假期规则 code（不传则查询所有假期）"},
	},
	Tips: []string{`dws attendance +get-leave-balance --users userId1,userId2 --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		req := map[string]any{
			"targetUserIds": rt.StrSlice("users"),
		}
		if v := rt.Str("leave-code"); v != "" {
			req["leaveCode"] = v
		}
		return rt.CallMCP("get_leave_balance_quota", map[string]any{
			"McpLeaveBalanceRequest": req,
		})
	},
}

// GetLeaveRecords 查询指定员工假期余额变更记录（get_leave_balance_records_v2）。
var GetLeaveRecords = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-leave-records",
	Product:     serverWukong,
	Description: "查询指定员工的假期余额变更记录",
	Intent:      "当你想追溯某个员工假期额度的变动流水（何时发放、扣减、因请假消耗多少）以核对余额来龙去脉时使用；输入单个 userId、日期区间，可选假期 code（不传查所有假期），返回该员工在该时间段的余额变更记录。只想看当前余额用 +get-leave-balance。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "user", Type: shortcut.FlagString, Desc: "目标员工 userId", Required: true},
		{Name: "leave-code", Type: shortcut.FlagString, Desc: "假期规则 code（不传则查询所有假期）"},
		{Name: "start", Type: shortcut.FlagString, Desc: "查询开始日期 YYYY-MM-DD", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "查询结束日期 YYYY-MM-DD", Required: true},
	},
	Tips: []string{`dws attendance +get-leave-records --user USER_ID --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --start 2026-04-01 --end 2026-04-22`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		start, err := dayMillis(rt.Str("start"))
		if err != nil {
			return err
		}
		end, err := dayMillis(rt.Str("end"))
		if err != nil {
			return err
		}
		req := map[string]any{
			"targetUserId":   rt.Str("user"),
			"startTimeStamp": start,
			"endTimeStamp":   end,
		}
		if v := rt.Str("leave-code"); v != "" {
			req["leaveCode"] = v
		}
		return rt.CallMCP("get_leave_balance_records_v2", map[string]any{
			"McpLeaveRecordRequest": req,
		})
	},
}

// UpdateLeaveType 更新假期规则（save_leave_type）。
var UpdateLeaveType = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+update-leave-type",
	Product:     serverWukong,
	Description: "更新已有假期规则（仅传要修改的字段）",
	Intent:      "当管理员要修改某类假期的规则设置（改名称、假期单位、是否带薪、一天折算小时数、新员工可请假时机、适用范围等）时使用；输入假期编码 leaveCode 和需要变更的字段，会实际更新该假期规则，影响全体适用员工的请假规则。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "leave-code", Type: shortcut.FlagString, Desc: "假期编码", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "假期名称"},
		{Name: "unit", Type: shortcut.FlagString, Enum: []string{"day", "halfDay", "hour"}, Desc: "假期单位：day/halfDay/hour"},
		{Name: "paid", Type: shortcut.FlagBool, Desc: "是否带薪假期"},
		{Name: "per-hours", Type: shortcut.FlagInt, Desc: "一天折算小时数"},
		{Name: "when-can-leave", Type: shortcut.FlagString, Enum: []string{"entry", "formal"}, Desc: "新员工请假规则：entry/formal"},
		{Name: "visibility-rules", Type: shortcut.FlagString, Desc: `适用范围规则 JSON 数组，如 [{"type":"dept","visible":["1","2"]}]；全公司可见传 [{"type":"dept","visible":["-1"]}]`},
	},
	Tips: []string{`dws attendance +update-leave-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --name "事假（修改版）"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		req := map[string]any{
			"leaveCode": rt.Str("leave-code"),
		}
		changed := 0
		if rt.Changed("name") {
			req["leaveName"] = rt.Str("name")
			changed++
		}
		if rt.Changed("unit") {
			req["leaveUnit"] = rt.Str("unit")
			changed++
		}
		if rt.Changed("paid") {
			req["paidLeave"] = rt.Bool("paid")
			changed++
		}
		if rt.Changed("per-hours") {
			if v := rt.Int("per-hours"); v > 0 {
				req["perHoursInDay"] = v
			}
			changed++
		}
		if rt.Changed("when-can-leave") {
			if v := rt.Str("when-can-leave"); v != "" {
				req["whenCanLeave"] = v
			}
			changed++
		}
		if rt.Changed("visibility-rules") {
			raw := strings.TrimSpace(rt.Str("visibility-rules"))
			if raw != "" {
				var rules []map[string]any
				if err := json.Unmarshal([]byte(raw), &rules); err != nil {
					return fmt.Errorf("invalid --visibility-rules JSON: %w", err)
				}
				if len(rules) == 0 {
					return fmt.Errorf("--visibility-rules 不能是空数组，如需改为全公司可见请显式传 [{\"type\":\"dept\",\"visible\":[\"-1\"]}]")
				}
				req["visibilityRules"] = rules
			}
			changed++
		}
		if changed == 0 {
			return fmt.Errorf("至少需要指定一个更新字段：--name / --unit / --paid / --per-hours / --when-can-leave / --visibility-rules")
		}
		return rt.CallMCP("save_leave_type", map[string]any{
			"McpSaveLeaveTypeRequest": req,
		})
	},
}

// SaveLeaveBalance 设置员工假期余额（update_leave_balance）。
var SaveLeaveBalance = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+save-leave-balance",
	Product:     serverWukong,
	Description: "设置员工假期余额（SET 覆盖，非累加）",
	Intent:      "当管理员要给某员工发放或调整某类假期的余额（如年度发放年假、手工修正额度）时使用；输入员工 userId、假期编码、目标数量和变更原因，可选有效期。注意这是 SET 覆盖写入（把余额直接设为该数量而非在原基础上累加），会实际改变员工可用假期，操作前务必确认数量。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "target", Type: shortcut.FlagString, Desc: "目标员工工号 userId", Required: true},
		{Name: "leave-code", Type: shortcut.FlagString, Desc: "假期编码", Required: true},
		{Name: "num", Type: shortcut.FlagString, Desc: "余额数量（如 8 天传 8，7.5 天传 7.5）", Required: true},
		{Name: "reason", Type: shortcut.FlagString, Desc: "变更原因，最长 100 字符", Required: true},
		{Name: "start", Type: shortcut.FlagString, Desc: "有效期开始日期 YYYY-MM-DD"},
		{Name: "end", Type: shortcut.FlagString, Desc: "有效期结束日期 YYYY-MM-DD"},
	},
	Tips: []string{`dws attendance +save-leave-balance --target user001 --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --num 8 --reason "年度发放"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		numFloat, err := strconv.ParseFloat(rt.Str("num"), 64)
		if err != nil {
			return fmt.Errorf("--num 格式错误: %w（应为数字，如 8 或 7.5）", err)
		}
		req := map[string]any{
			"targetUserId": rt.Str("target"),
			"leaveCode":    rt.Str("leave-code"),
			"quotaNum":     strconv.Itoa(int(numFloat * 100)),
			"reason":       rt.Str("reason"),
		}
		if v := rt.Str("start"); v != "" {
			ts, err := dateToMillis(v, false)
			if err != nil {
				return err
			}
			req["startTime"] = ts
		}
		if v := rt.Str("end"); v != "" {
			ts, err := dateToMillis(v, true)
			if err != nil {
				return err
			}
			req["endTime"] = ts
		}
		return rt.CallMCP("update_leave_balance", map[string]any{
			"McpUpdateLeaveBalanceRequest": req,
		})
	},
}

// ── checkin ─────────────────────────────────────────────────

// GetCheckinRecord 查询指定员工的签到记录（get_checkin_record）。
var GetCheckinRecord = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+get-checkin-record",
	Product:     serverWukong,
	Description: "查询指定员工一段时间内的签到记录",
	Intent:      "当你要查看员工的外勤/移动办公签到记录（signin/checkin，区别于考勤打卡）时使用，例如核实业务员的拜访签到轨迹；需提供操作者企业 ID 与员工 ID、目标员工 ID 列表（最多 100 人）以及不超过 7 天的时间区间，返回这些人的签到明细。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "operator-corp-id", Type: shortcut.FlagString, Desc: "操作者企业 ID", Required: true},
		{Name: "operator-staff-id", Type: shortcut.FlagString, Desc: "操作者员工 ID", Required: true},
		{Name: "staff-ids", Type: shortcut.FlagStringSlice, Desc: "目标员工 ID 列表，逗号分隔，最多 100 人", Required: true},
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间 yyyy-MM-dd HH:mm:ss", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间 yyyy-MM-dd HH:mm:ss，跨度最多 7 天", Required: true},
	},
	Tips: []string{`dws attendance +get-checkin-record --operator-corp-id dingXXX --operator-staff-id op001 --staff-ids user001,user002 --start "2026-04-01 00:00:00" --end "2026-04-07 00:00:00"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		startStr := rt.Str("start")
		endStr := rt.Str("end")
		startT, err := time.ParseInLocation("2006-01-02 15:04:05", startStr, time.Local)
		if err != nil {
			return fmt.Errorf("--start 格式错误，应为 yyyy-MM-dd HH:mm:ss: %w", err)
		}
		endT, err := time.ParseInLocation("2006-01-02 15:04:05", endStr, time.Local)
		if err != nil {
			return fmt.Errorf("--end 格式错误，应为 yyyy-MM-dd HH:mm:ss: %w", err)
		}
		if endT.Before(startT) {
			return fmt.Errorf("--end 不能早于 --start")
		}
		return rt.CallMCP("get_checkin_record", map[string]any{
			"QueryMcpUserRecordRequest": map[string]any{
				"operatorCorpId":  rt.Str("operator-corp-id"),
				"operatorStaffId": rt.Str("operator-staff-id"),
				"staffIds":        rt.StrSlice("staff-ids"),
				"startTime":       startStr,
				"endTime":         endStr,
			},
		})
	},
}

// ── boss check ──────────────────────────────────────────────

// BossCheck BOSS 改签打卡记录（boss_check）。
var BossCheck = shortcut.Shortcut{
	Service:     "attendance",
	Command:     "+boss-check",
	Product:     serverWukong,
	Description: "BOSS 改签打卡记录（管理员修改打卡时间/结果）",
	Intent:      "当管理员/主管要手工修正某次打卡（改打卡时间、把结果改成正常或指定迟到/早退/缺卡等）时使用，例如员工漏打卡或异常需纠正；需提供排班 planId 或打卡结果 resultId（二选一，可由 +get-schedule 获取），并指定新时间/结果/缺勤分钟/备注。会实际改写该员工的考勤结果，属敏感操作。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "plan-id", Type: shortcut.FlagString, Desc: "排班 ID（与 --result-id 二选一，可由 +get-schedule 获取 id）"},
		{Name: "result-id", Type: shortcut.FlagString, Desc: "打卡结果 ID（与 --plan-id 二选一，优先使用）"},
		{Name: "time", Type: shortcut.FlagString, Desc: "新打卡时间 yyyy-MM-dd HH:mm"},
		{Name: "result", Type: shortcut.FlagString, Enum: []string{"Normal", "TimesResultA", "TimesResultB", "TimesResultC", "TimesResultD", "TimesResultE", "TimesResultF"}, Desc: "打卡结果：Normal/迟到 A/早退 B/缺卡 C/迟到+早退 D/缺卡+早退 E/迟到+缺卡 F"},
		{Name: "absent-min", Type: shortcut.FlagInt, Desc: "缺勤时长（分钟），异常结果时传值"},
		{Name: "remark", Type: shortcut.FlagString, Desc: "备注，最长 500 字符"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"plan-id", "result-id"}},
	},
	Tips: []string{`dws attendance +boss-check --plan-id 123456 --time "2026-04-21 08:30"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		req := map[string]any{}
		if resultID := rt.Str("result-id"); resultID != "" {
			req["resultId"] = resultID
		} else if planID := rt.Str("plan-id"); planID != "" {
			req["planId"] = planID
		}
		if rt.Changed("time") {
			req["bossCheckTime"] = rt.Str("time")
		}
		if rt.Changed("result") {
			req["result"] = rt.Str("result")
		}
		if rt.Changed("absent-min") {
			req["absentMin"] = rt.Int("absent-min")
		}
		if rt.Changed("remark") {
			req["remark"] = rt.Str("remark")
		}
		return rt.CallMCP("boss_check", map[string]any{
			"BossCheckMcpRequest": req,
		})
	},
}

func init() {
	shortcut.Register(
		CheckResult,
		CheckRecord,
		ListApprove,
		GetApproveTemplate,
		GetSchedule,
		ImportSchedule,
		SearchClass,
		GetClass,
		CreateClass,
		UpdateClass,
		GetAdjustmentRule,
		SearchAdjustmentRule,
		GetOvertimeRule,
		SearchOvertimeRule,
		SearchGroup,
		GetGroup,
		GetGroupFiltered,
		UpdateGroupMembers,
		CreateGroup,
		UpdateGroup,
		GetSummary,
		GetSelfSetting,
		GetGlobalSetting,
		ListReportColumns,
		QueryReportData,
		QueryReportLeave,
		ListLeaveTypes,
		GetLeaveBalance,
		GetLeaveRecords,
		UpdateLeaveType,
		SaveLeaveBalance,
		GetCheckinRecord,
		BossCheck,
	)
}
