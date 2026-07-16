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
	"fmt"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// FreeBusy: check whether a person is busy or free in a time window, by NAME.
//
// Steps: resolve the person's name → unique userId → query their busy/free
// status over the given range. Replaces `contact +search-user` (copy userId) →
// `calendar busy search --users <id> --start ... --end ...`.
//
//	dws calendar +free --who 张三 --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00"
var FreeBusy = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+free",
	Product:     "calendar",
	Description: "按姓名查询某人在指定时间段内的忙闲状态（自动解析 userId）",
	Intent: "当你只知道对方姓名、想知道 TA 在某段时间内是空闲还是被日程占用（比如约会前先看看有没有空）而不想先手动查 userId 时使用；" +
		"内部先按姓名搜通讯录解析出唯一 userId，姓名匹配到多人时会列出候选让你区分，再按时间范围查询忙闲。只读，不产生任何日程变更。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "who", Type: shortcut.FlagString, Desc: "要查忙闲的人的姓名/花名", Required: true},
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间（ISO8601，如 2026-03-10T14:00:00+08:00）", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间（ISO8601，如 2026-03-10T18:00:00+08:00）", Required: true},
	},
	Tips: []string{`dws calendar +free --who 张三 --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — resolve the person's name to a unique userId.
		user, err := resolveUser(rt, rt.Str("who"))
		if err != nil {
			return err
		}

		// Step 2 — parse the ISO8601 range to epoch millis, exactly as the
		// calendar busy tool expects (startTime/endTime are int64 millis).
		startMillis, err := freebusyParseMillis("start", rt.Str("start"))
		if err != nil {
			return err
		}
		endMillis, err := freebusyParseMillis("end", rt.Str("end"))
		if err != nil {
			return err
		}
		if endMillis <= startMillis {
			return apperrors.NewValidation("--end 必须晚于 --start")
		}

		// Step 3 — query busy status for that single user over the range and
		// project the verbose result[].scheduleItems[].{start,end}.dateTime
		// structure down to a flat list of {start,end} busy slots.
		data, err := rt.CallMCPData("calendar", "query_busy_status", map[string]any{
			"startTime": startMillis,
			"endTime":   endMillis,
			"userIds":   []string{user.userID},
		})
		if err != nil {
			return err
		}
		busy := freebusySlots(data)
		return rt.Output(map[string]any{
			"who":    user.name,
			"userId": user.userID,
			"busy":   busy,
			"free":   len(busy) == 0,
		})
	},
}

// freebusySlots flattens a query_busy_status response (result[] → scheduleItems[]
// → {start,end}.dateTime) into a flat list of {start,end} busy slots.
func freebusySlots(data map[string]any) []map[string]any {
	out := []map[string]any{}
	entries, _ := data["result"].([]any)
	for _, e := range entries {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		items, _ := em["scheduleItems"].([]any)
		for _, it := range items {
			im, ok := it.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, map[string]any{
				"start": freebusyDateTime(im["start"]),
				"end":   freebusyDateTime(im["end"]),
			})
		}
	}
	return out
}

// freebusyDateTime pulls the readable timestamp out of a {date,dateTime,timeZone}
// object, falling back to the raw value.
func freebusyDateTime(v any) any {
	if m, ok := v.(map[string]any); ok {
		if dt, ok := m["dateTime"]; ok && dt != nil {
			return dt
		}
		if d, ok := m["date"]; ok && d != nil {
			return d
		}
	}
	return v
}

// freebusyParseMillis parses an ISO8601 timestamp into epoch milliseconds,
// returning a clear validation error naming the offending flag.
func freebusyParseMillis(flag, value string) (int64, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0, apperrors.NewValidation(fmt.Sprintf(
			"--%s 时间格式无效：%q，请使用 ISO8601（如 2026-03-10T14:00:00+08:00）", flag, value))
	}
	return t.UnixMilli(), nil
}

func init() {
	shortcut.Register(FreeBusy)
}
