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

// RespondEvent: accept / decline / tentatively respond to a calendar event
// invitation in one command, as the current user (the event attendee).
//
// This wraps the `respond` MCP tool from helpers/calendar.go verbatim
// (mirroring `dws calendar event respond`): it takes eventId + responseStatus,
// where responseStatus is one of needsAction/accepted/declined/tentative. The
// shortcut exposes the ergonomic verbs accept/decline/tentative on --response
// and maps them onto the tool's accepted/declined/tentative values so the
// caller never has to remember the past-tense wire spelling.
//
//	dws calendar +respond-event --event EVENT_ID --response accept
//	dws calendar +respond-event --event EVENT_ID --response decline
//	dws calendar +respond-event --event EVENT_ID --response tentative
var RespondEvent = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+respond-event",
	Product:     "calendar",
	Description: "接受 / 拒绝 / 暂定回复一个日程邀请（作为参会人设置自己的响应状态）",
	Intent: "当你收到一个日程邀请、想直接确认接受、婉拒或标记为暂定，而不想记忆 accepted/declined 这类过去式接口取值时使用；" +
		"你只需提供日程的 eventId（用 `dws calendar event list` 查询）和一个直白的动作 accept/decline/tentative，" +
		"内部会把它映射为服务端的 accepted/declined/tentative 并调用日历的 respond 能力设置你的参会响应状态。" +
		"注意：这会真实修改你在该日程上的响应状态；订阅日历下的日程没有参会人，因此无法响应。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "event", Type: shortcut.FlagString, Desc: "日程 eventId（用 `dws calendar event list` 查询）", Required: true},
		{Name: "response", Type: shortcut.FlagString, Desc: "响应动作：accept(接受) / decline(拒绝) / tentative(暂定)", Required: true, Enum: []string{"accept", "decline", "tentative"}},
	},
	Tips: []string{
		`dws calendar +respond-event --event EVENT_ID --response accept`,
		`dws calendar +respond-event --event EVENT_ID --response decline`,
		`dws calendar +respond-event --event EVENT_ID --response tentative`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		eventID := strings.TrimSpace(rt.Str("event"))
		if eventID == "" {
			return apperrors.NewValidation("请用 --event 提供日程的 eventId（可用 `dws calendar event list` 查询）")
		}

		// Map the ergonomic verb onto the tool's responseStatus wire value.
		// The `respond` tool accepts accepted/declined/tentative (see
		// helpers/calendar.go event respond); the Enum on --response already
		// guarantees one of accept/decline/tentative here.
		var status string
		switch strings.TrimSpace(strings.ToLower(rt.Str("response"))) {
		case "accept":
			status = "accepted"
		case "decline":
			status = "declined"
		case "tentative":
			status = "tentative"
		default:
			return apperrors.NewValidation("--response 只允许 accept / decline / tentative")
		}

		// Call `respond` verbatim: eventId + responseStatus, mirroring
		// `dws calendar event respond`.
		return rt.CallMCP("respond", map[string]any{
			"eventId":        eventID,
			"responseStatus": status,
		})
	},
}

func init() {
	shortcut.Register(RespondEvent)
}
