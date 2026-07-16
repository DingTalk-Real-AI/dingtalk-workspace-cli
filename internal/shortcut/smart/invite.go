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
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Invite: add people BY NAME as participants to an EXISTING calendar event.
//
// Steps: resolve every name in --with to a unique userId, then batch-add them
// all to the --event day's event as attendees. Replaces the manual flow of
// `contact +search-user` (copy each userId) → `calendar attendee add`.
//
//	dws calendar +invite --event EVENT_ID --with 张三,李四
var Invite = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+invite",
	Product:     "calendar",
	Description: "按姓名把参会人加入已有日程（自动解析 userId 后批量添加）",
	Intent: "当你已经有一个日程（知道 eventId），想按姓名把几位同事拉进来当参会人时使用；" +
		"内部先把 --with 里每个姓名解析成唯一 userId，再一次性把他们全部加到 --event 指定的日程里。" +
		"会真实修改日程并发出参会邀请。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "event", Type: shortcut.FlagString, Desc: "已有日程的 eventId", Required: true},
		{Name: "with", Type: shortcut.FlagString, Desc: "参会人姓名，逗号分隔", Required: true},
	},
	Tips: []string{
		`dws calendar +invite --event EVENT_ID --with 张三`,
		`dws calendar +invite --event EVENT_ID --with 张三,李四`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		eventID := strings.TrimSpace(rt.Str("event"))
		if eventID == "" {
			return apperrors.NewValidation("--event 需要一个有效的日程 eventId")
		}

		// Resolve every participant name to a unique userId first, so an
		// unknown/ambiguous name fails before we touch the event.
		var userIDs []string
		for _, name := range strings.Split(rt.Str("with"), ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			user, err := resolveUser(rt, name)
			if err != nil {
				return err
			}
			userIDs = append(userIDs, user.userID)
		}
		if len(userIDs) == 0 {
			return apperrors.NewValidation("--with 需要至少一个有效的参会人姓名")
		}

		// Batch-add all participants. eventId + attendeesToAdd copied verbatim
		// from the helper's `attendee add` call site (add_calendar_participant).
		return rt.CallMCP("add_calendar_participant", map[string]any{
			"eventId":        eventID,
			"attendeesToAdd": userIDs,
		})
	},
}

func init() {
	shortcut.Register(Invite)
}
