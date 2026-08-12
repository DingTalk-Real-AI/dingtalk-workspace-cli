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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/minutesdata"
)

// ActionItems: fetch the extracted to-do items (待办事项) from MY most recent
// minutes (听记) in one step.
//
// Steps:
//  1. list my minutes via list_by_keyword_and_time_range
//     (belongingConditionId = "created", maxResults = 20);
//  2. pick the newest entry by an explicit comparable timestamp and read its taskUuid;
//  3. print that minute's extracted to-do items via list_minutes_todos.
//
// If the list is empty it reports "暂无妙记" instead of failing obscurely.
//
//	dws minutes +action-items
var ActionItems = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+action-items",
	Product:     "minutes",
	Description: "读取指定或我最新一条听记中已抽取的行动项",
	Intent: "当你要读取已知 taskUuid（--id）的听记行动项，或不传 --id 自动选择自己最新听记时使用；" +
		"只接受服务端显式 actions/dingtalkTodoList 数组，合法空数组表示没有抽取到行动项，缺字段或错误响应不会被当成空成功。" +
		"这是只读的 Minutes 产物读取，不会创建或修改钉钉 Todo。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: minutesSmartContract(
		"+action-items",
		"读取最新听记中已抽取的行动项",
		"要读取指定听记（--id）或默认最新听记中由听记服务抽取的 actions/dingtalkTodoList 时使用；该命令只读，不会创建钉钉待办。",
		[]string{"要创建或修改真正的钉钉待办时使用 Todo 产品命令"},
		[]string{"dws minutes +action-items --id <taskUuid>", "dws minutes +action-items"},
		nil,
	),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid；不传时选择我最新的一条"},
	},
	Tips: []string{
		`dws minutes +action-items --id <taskUuid>`,
		`dws minutes +action-items`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		taskUUID := rt.Str("id")
		if taskUUID == "" {
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
			taskUUID, err = latestMinutesTaskUUID(data)
			if err != nil {
				return err
			}
			if taskUUID == "" {
				return apperrors.NewValidation("暂无妙记")
			}
		}

		// Step 3 — validate and print its extracted to-do items. Empty arrays are
		// valid business data; a missing result/actions collection is not.
		todosData, err := rt.CallMCPData("minutes", "list_minutes_todos", map[string]any{
			"taskUuid": taskUUID,
		})
		if err != nil {
			return err
		}
		if err := minutesdata.ValidateArtifact("todos", taskUUID, todosData); err != nil {
			return err
		}
		return rt.Output(todosData["result"])
	},
}

func init() {
	shortcut.Register(finalizeMinutesSmartShortcut(ActionItems))
}
