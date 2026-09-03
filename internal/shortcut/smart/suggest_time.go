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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// SuggestTime: recommend meeting slots for several people, resolved BY NAME.
//
// Steps: parse the --with name CSV → resolve each name to a unique userId
// (fails listing every name that can't be resolved) → ask the calendar service
// for suggested slots free for everyone in the given range. Replaces looking up
// each userId by hand and then calling `calendar event suggest --users ...`.
//
//	dws calendar +suggest-time --with 张三,李四 \
//	  --start "2026-03-10T09:00:00+08:00" --end "2026-03-10T18:00:00+08:00" --duration 60
var SuggestTime = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+suggest-time",
	Product:     "calendar",
	Description: "按姓名解析多位参与者，推荐大家都有空的可开会时间段（自动解析 userId）",
	Intent: "当你想为几个人凑一个大家都空闲的开会时间、但只知道他们的姓名而不想逐个手动查 userId 时使用；" +
		"内部会把 --with 里的姓名逐个搜通讯录解析成唯一 userId（任何一个没匹配到或匹配到多人都会明确报出来，绝不瞎猜），" +
		"再基于所有参与者的忙闲，在给定时间范围内推荐若干可用时段。只读，不创建任何日程。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_suggest_time",
			CanonicalPath:  "calendar.shortcut_suggest_time",
			CLIPath:        "calendar +suggest-time",
			PrimaryCLIPath: "calendar +suggest-time",
		},
		Description: "按姓名解析多位参与者，推荐大家都有空的可开会时间段（自动解析 userId）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按姓名解析多位参与者，推荐大家都有空的可开会时间段（自动解析 userId）",
			UseWhen:      []string{"当你想为几个人凑一个大家都空闲的开会时间、但只知道他们的姓名而不想逐个手动查 userId 时使用；内部会把 --with 里的姓名逐个搜通讯录解析成唯一 userId（任何一个没匹配到或匹配到多人都会明确报出来，绝不瞎猜），再基于所有参与者的忙闲，在给定时间范围内推荐若干可用时段。只读，不创建任何日程。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws calendar +suggest-time --with 张三,李四 --start \"2026-03-10T09:00:00+08:00\" --end \"2026-03-10T18:00:00+08:00\"",
				"dws calendar +suggest-time --with 张三,李四,王五 --start \"2026-03-10T09:00:00+08:00\" --end \"2026-03-10T18:00:00+08:00\" --duration 30",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "with", Type: shortcut.FlagStringSlice, Desc: "参与者姓名（逗号分隔的 CSV，如 张三,李四）", Required: true},
		{Name: "start", Type: shortcut.FlagString, Desc: "时间范围开始（ISO8601，如 2026-03-10T09:00:00+08:00）", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "时间范围结束（ISO8601，如 2026-03-10T18:00:00+08:00）", Required: true},
		{Name: "duration", Type: shortcut.FlagString, Desc: "会议时长（分钟，可选）", Required: false},
	},
	Tips: []string{
		`dws calendar +suggest-time --with 张三,李四 --start "2026-03-10T09:00:00+08:00" --end "2026-03-10T18:00:00+08:00"`,
		`dws calendar +suggest-time --with 张三,李四,王五 --start "2026-03-10T09:00:00+08:00" --end "2026-03-10T18:00:00+08:00" --duration 30`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if err := calendarSmartValidateRange(rt.Str("start"), rt.Str("end")); err != nil {
			return err
		}
		// Step 1 — resolve every participant name to a unique userId.
		names := rt.StrSlice("with")
		if len(names) == 0 {
			return apperrors.NewValidation("--with 至少需要一个参与者姓名")
		}
		userIDs := make([]string, 0, len(names))
		var failures []string
		for _, raw := range names {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			user, err := resolveUser(rt, name)
			if err != nil {
				failures = append(failures, err.Error())
				continue
			}
			userIDs = append(userIDs, user.userID)
		}
		if len(failures) > 0 {
			return apperrors.NewValidation(
				"以下参与者无法解析为唯一 userId：\n- " + strings.Join(failures, "\n- "))
		}
		if len(userIDs) == 0 {
			return apperrors.NewValidation("--with 至少需要一个有效的参与者姓名")
		}

		// Step 2 — assemble the suggest-times request exactly as the calendar
		// helper does (start/end are ISO8601 strings, attendeeUserIds is a
		// string slice, durationMinutes is an optional string).
		params := map[string]any{
			"start":           rt.Str("start"),
			"end":             rt.Str("end"),
			"attendeeUserIds": userIDs,
		}
		if rt.Changed("duration") {
			params["durationMinutes"] = rt.Str("duration")
		}

		// Step 3 — recommend slots free for everyone over the range, then project
		// result.recommendEventTimes[] down to a clean {start,end,conflicts} list
		// (dropping the null padding entries the gateway leaves in
		// timeConflictAttendees).
		data, err := rt.CallMCPData("calendar", "list_suggested_event_times", params)
		if err != nil {
			return err
		}
		suggestions, err := calendarSmartSuggestedSlots(data)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(suggestions), "suggestions": suggestions, "complete": true})
	},
}

func init() {
	finalizeCalendarSmart(&SuggestTime, "严格校验的共同空闲建议时段")
	shortcut.Register(SuggestTime)
}
