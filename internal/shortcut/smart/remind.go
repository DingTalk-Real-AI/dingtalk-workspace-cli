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

// Remind: create a personal todo for YOURSELF with an optional due/reminder time,
// in one command. It resolves the current user and explicitly passes executorIds;
// the real todo backend does not reliably default a missing executor to "me".
//
// Steps: take --task as the subject, and (optionally) parse --at into epoch
// milliseconds (mirroring the todo helper's parseISOTimeToMillis, which stores
// dueTime as int64 millis) → create_personal_todo. Replaces having to look up
// your own userId before `todo +create`.
//
//	dws todo +remind --task "交周报" --at 2026-03-10T18:00:00+08:00
var Remind = shortcut.Shortcut{
	Service:     "todo",
	Command:     "+remind",
	Product:     "todo",
	Description: "给自己创建一条带截止/提醒时间的待办",
	Intent: "当你想给自己记一件事、并（可选）设一个截止/提醒时间，又不想先查自己的 userId 时使用；" +
		"内部先解析当前登录用户的 userId，再显式设置 executorIds，--at 会按 ISO8601 解析为截止时间。会真实创建待办。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "task", Type: shortcut.FlagString, Desc: "待办标题/内容", Required: true},
		{Name: "at", Type: shortcut.FlagString, Desc: "截止/提醒时间（ISO8601，可选，如 2026-03-10T18:00:00+08:00）"},
	},
	Tips: []string{`dws todo +remind --task "交周报" --at 2026-03-10T18:00:00+08:00`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		profile, err := rt.CallMCPData("contact", "get_current_user_profile", nil)
		if err != nil {
			return err
		}
		userID := myAttendanceCurrentUserID(profile)
		if userID == "" {
			return apperrors.NewValidation("无法解析当前登录用户的 userId，无法给自己创建待办")
		}

		vo := map[string]any{
			"subject":     rt.Str("task"),
			"executorIds": []string{userID},
		}

		// Optional due/reminder time. The todo helper feeds --due through
		// parseISOTimeToMillis and stores dueTime as epoch milliseconds (int64),
		// so we do the same here rather than passing a raw string.
		if rt.Changed("at") {
			ms, err := shortcutRemindParseMillis("at", rt.Str("at"))
			if err != nil {
				return err
			}
			vo["dueTime"] = ms
		}

		return rt.CallMCP("create_personal_todo", map[string]any{
			"PersonalTodoCreateVO": vo,
		})
	},
}

// shortcutRemindParseMillis parses an ISO8601 timestamp into epoch milliseconds,
// returning a clear validation error naming the offending flag. Mirrors the todo
// helper's parseISOTimeToMillis (which the CLI uses to build dueTime).
func shortcutRemindParseMillis(flag, value string) (int64, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0, apperrors.NewValidation(fmt.Sprintf(
			"--%s 时间格式无效：%q，请使用 ISO8601（如 2026-03-10T18:00:00+08:00）", flag, value))
	}
	return t.UnixMilli(), nil
}

func init() {
	shortcut.Register(Remind)
}
