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

// ActionItems: fetch the extracted to-do items (待办事项) from MY most recent
// minutes (听记) in one step.
//
// Steps:
//  1. list my minutes via list_by_keyword_and_time_range
//     (belongingConditionId = "created", maxResults = 20);
//  2. pick the newest entry (largest create time, falling back to the first
//     item since lists come back newest-first) and read its taskUuid;
//  3. print that minute's extracted to-do items via list_minutes_todos.
//
// If the list is empty it reports "暂无妙记" instead of failing obscurely.
//
//	dws minutes +action-items
var ActionItems = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+action-items",
	Product:     "minutes",
	Description: "取我最新一条妙记里的待办事项",
	Intent: "当你刚开完会、只想立刻知道自己最近这条听记里系统帮你提取出了哪些待办事项，" +
		"却不想先翻听记列表、复制 taskUuid、再手动去查待办时使用；" +
		"内部会先列出你创建的听记，自动挑出最新的一条，再拉取这条听记里提取的待办事项清单（含待办内容、参与人、待办时间等）。" +
		"这是只读操作，不会新增或修改任何待办；若你名下没有任何听记则提示「暂无妙记」。",
	Risk: shortcut.RiskRead,
	Tips: []string{
		`dws minutes +action-items`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — list my minutes (newest first). belongingConditionId /
		// maxResults mirror helpers.callListByKeywordRange.
		data, err := rt.CallMCPData("minutes", "list_by_keyword_and_time_range", map[string]any{
			"belongingConditionId": "created",
			"maxResults":           float64(20),
		})
		if err != nil {
			return err
		}

		// Step 2 — locate the newest minute's taskUuid. Reuse the defensive
		// list/UUID/createTime parsing from latest_minutes.go.
		taskUUID := latestMinutesTaskUUID(data)
		if taskUUID == "" {
			return apperrors.NewValidation("暂无妙记")
		}

		// Step 3 — print its extracted to-do items (taskUuid param mirrors
		// helpers' list_minutes_todos call).
		return rt.CallMCP("list_minutes_todos", map[string]any{
			"taskUuid": taskUUID,
		})
	},
}

func init() {
	shortcut.Register(ActionItems)
}
