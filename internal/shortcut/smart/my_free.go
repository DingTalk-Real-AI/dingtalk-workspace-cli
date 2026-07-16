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
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// MyFree: show MY own busy slots over a range — the self version of +free that
// needs no --who (you rarely want to type your own name). Defaults to today.
//
// Steps:
//
//  1. resolve my own userId via the zero-arg get_current_user_profile (reusing
//     myAttendanceCurrentUserID);
//
//  2. query_busy_status for [start,end] (defaults to today 00:00→tomorrow 00:00
//     in local time), then project result[].scheduleItems[] to a flat
//     {start,end} busy list (reusing freebusySlots).
//
//     dws calendar +my-free
//     dws calendar +my-free --start 2026-07-10T09:00:00+08:00 --end 2026-07-10T18:00:00+08:00
var MyFree = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+my-free",
	Product:     "calendar",
	Description: "查我自己在某时间段的忙闲（默认今天，无需输入姓名）",
	Intent: "当你（或 AI agent）想知道『我自己什么时候有空/忙』、用于安排会议或回复邀约时使用；" +
		"不用像 +free 那样传别人的姓名——内部自动解析当前用户的 userId，再查其忙闲时段。" +
		"默认查今天（本地时区 00:00 到次日 00:00），也可用 --start/--end 指定 ISO8601 时间范围。" +
		"只读操作，只查忙闲、不创建或修改任何日程；返回按时间排列的忙碌时段，空则表示这段时间全空。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间（ISO8601，可选，默认今天 00:00）", Required: false},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间（ISO8601，可选，默认次日 00:00）", Required: false},
	},
	Tips: []string{
		`dws calendar +my-free`,
		`dws calendar +my-free --start 2026-07-10T09:00:00+08:00 --end 2026-07-10T18:00:00+08:00`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — resolve my own userId (reuse the +my-attendance parser).
		profile, err := rt.CallMCPData("contact", "get_current_user_profile", nil)
		if err != nil {
			return err
		}
		userID := myAttendanceCurrentUserID(profile)
		if userID == "" {
			return apperrors.NewValidation("无法解析当前用户的 userId")
		}

		// Step 2 — resolve the [start,end] window. Both flags are optional and
		// default to today's [00:00, next-day 00:00) in local time.
		now := time.Now()
		startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		startMillis := startOfToday.UnixMilli()
		endMillis := startOfToday.AddDate(0, 0, 1).UnixMilli()
		if rt.Str("start") != "" {
			ms, perr := freebusyParseMillis("start", rt.Str("start"))
			if perr != nil {
				return perr
			}
			startMillis = ms
		}
		if rt.Str("end") != "" {
			ms, perr := freebusyParseMillis("end", rt.Str("end"))
			if perr != nil {
				return perr
			}
			endMillis = ms
		}
		if endMillis <= startMillis {
			return apperrors.NewValidation("--end 必须晚于 --start")
		}

		// Step 3 — query and project busy slots (reuse freebusySlots).
		data, err := rt.CallMCPData("calendar", "query_busy_status", map[string]any{
			"startTime": startMillis,
			"endTime":   endMillis,
			"userIds":   []string{userID},
		})
		if err != nil {
			return err
		}
		busy := freebusySlots(data)
		return rt.Output(map[string]any{
			"userId": userID,
			"busy":   busy,
			"free":   len(busy) == 0,
		})
	},
}

func init() {
	shortcut.Register(MyFree)
}
