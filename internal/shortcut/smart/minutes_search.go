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
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// MinutesSearch: search MY minutes (听记) by keyword and print a projected list.
//
// Steps:
//  1. list my minutes via list_by_keyword_and_time_range with
//     belongingConditionId="created", maxResults=20 and keyword=--query,
//     mirroring helpers.callListByKeywordRange;
//  2. project each entry to {title, createTime, taskUuid} — field parsing is
//     defensive (multiple candidate keys) — and print via rt.Output so it
//     honours --format/--jq/--fields;
//  3. if nothing matched, report "没搜到妙记" instead of an empty raw dump.
//
// Read-only: it only lists and reshapes, never mutates any minute.
//
//	dws minutes +minutes-search --query 周会
var MinutesSearch = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+minutes-search",
	Product:     "minutes",
	Description: "按关键词搜索我的妙记并投影列表",
	Intent: "当你想按关键词快速找回自己创建的会议听记（妙记），只需要看到匹配到的标题、创建时间和 taskUuid 列表、而不想拿到一大坨原始字段时使用；" +
		"内部按 --query 关键词列出你创建的听记（最多 20 条），再在本地投影出每条的标题、创建时间和 taskUuid。" +
		"这是纯只读操作，只做搜索与本地投影，不会修改任何听记；若没有匹配的听记则提示「没搜到妙记」。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "按关键词搜索听记（必填）", Required: true},
	},
	Tips: []string{
		`dws minutes +minutes-search --query 周会`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — search my minutes. belongingConditionId / maxResults / keyword
		// mirror helpers.callListByKeywordRange.
		data, err := rt.CallMCPData("minutes", "list_by_keyword_and_time_range", map[string]any{
			"belongingConditionId": "created",
			"maxResults":           float64(20),
			"keyword":              rt.Str("query"),
		})
		if err != nil {
			return err
		}

		// Step 2 — project matched entries. latestMinutesItems (from
		// latest_minutes.go) defensively unwraps the list container.
		items := latestMinutesItems(data)
		results := make([]map[string]any, 0, len(items))
		for _, m := range items {
			results = append(results, map[string]any{
				"title":      minutesSearchTitle(m),
				"createTime": minutesSearchCreateTime(m),
				// latestMinutesUUID probes taskUuid/taskUUID/uuid/id in order.
				"taskUuid": latestMinutesUUID(m),
			})
		}

		// Step 3 — empty result guard.
		if len(results) == 0 {
			return apperrors.NewValidation("没搜到妙记")
		}

		return rt.Output(map[string]any{"minutes": results})
	},
}

// minutesSearchTitle reads a minute's display title, tolerating the common
// title keys the gateway may use.
func minutesSearchTitle(m map[string]any) string {
	for _, key := range []string{"title", "minutesTitle", "name", "subject"} {
		if s, ok := m[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// minutesSearchCreateTime reads a minute's create time, returning the raw value
// (usually epoch millis) under whichever candidate key is present.
func minutesSearchCreateTime(m map[string]any) any {
	for _, key := range []string{"createTime", "gmtCreate", "startTime", "createTimeStart"} {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

func init() {
	shortcut.Register(MinutesSearch)
}
