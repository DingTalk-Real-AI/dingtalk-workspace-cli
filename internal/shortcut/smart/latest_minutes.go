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

// LatestMinutes: fetch the details of MY most recent minutes (听记) in one step.
//
// Steps:
//  1. list my minutes via list_by_keyword_and_time_range (belongingConditionId
//     = "created"), optionally filtered by --keyword;
//  2. pick the newest entry by an explicit comparable timestamp and read its taskUuid;
//  3. print that minute's basic info via get_minutes_basic_info.
//
// If the list is empty it reports "暂无妙记" instead of failing obscurely.
//
//	dws minutes +latest
//	dws minutes +latest --keyword 周会
var LatestMinutes = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+latest",
	Aliases:     []string{"+latest-minutes"},
	Product:     "minutes",
	Description: "取我最新的一条妙记（听记）详情",
	Intent: "当你只想快速看回自己最近的一条会议听记，却不想先翻列表、复制 taskUuid 再查详情时使用；" +
		"内部先列出你创建的听记（可用 --keyword 缩小范围），自动挑出最新的一条，再拉取它的基础信息（标题、创建人、时间、访问链接等）。" +
		"这是只读操作，不会修改任何听记；若你名下没有任何听记则提示「暂无妙记」。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: minutesSmartContract(
		"+latest",
		"取我最新的一条妙记（听记）详情",
		"需要按真实创建/开始时间选出当前用户最新一条听记并直接读取基础详情时使用；可用 --keyword 缩小候选集。",
		[]string{"已知 taskUuid 时直接使用基础信息原子命令；需要搜索多条结果时使用 +search"},
		[]string{"dws minutes +latest", "dws minutes +latest --keyword 周会"},
		[]string{"+latest-minutes"},
	),
	Flags: []shortcut.Flag{
		{Name: "keyword", Type: shortcut.FlagString, Desc: "按关键字过滤听记（可选）", Required: false},
	},
	Tips: []string{
		`dws minutes +latest`,
		`dws minutes +latest --keyword 周会`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — list my minutes (newest first). belongingConditionId /
		// maxResults / keyword mirror helpers.callListByKeywordRange.
		listArgs := map[string]any{
			"belongingConditionId": "created",
			"maxResults":           float64(20),
		}
		if kw := rt.Str("keyword"); kw != "" {
			listArgs["keyword"] = kw
		}
		data, err := rt.CallMCPData("minutes", "list_by_keyword_and_time_range", listArgs)
		if err != nil {
			return err
		}

		// Step 2 — locate the newest minute's taskUuid.
		taskUUID, err := latestMinutesTaskUUID(data)
		if err != nil {
			return err
		}
		if taskUUID == "" {
			return apperrors.NewValidation("暂无妙记")
		}

		// Step 3 — validate and print its basic info. A successful-looking empty
		// result must not become a Shortcut success.
		basicData, err := rt.CallMCPData("minutes", "get_minutes_basic_info", map[string]any{
			"taskUuid": taskUUID,
		})
		if err != nil {
			return err
		}
		basic, err := minutesdata.Basic(taskUUID, basicData)
		if err != nil {
			return err
		}
		return rt.Output(basic)
	},
}

// latestMinutesTaskUUID picks the newest minute's taskUuid: the item with the
// largest explicit create/start time. Unknown list shapes and rows without a
// comparable timestamp fail closed instead of becoming an empty or arbitrary
// "latest" success.
func latestMinutesTaskUUID(data map[string]any) (string, error) {
	page, err := minutesdata.ParseListPage(data)
	if err != nil {
		return "", err
	}
	return minutesdata.LatestTaskUUID(page)
}

func init() {
	shortcut.Register(finalizeMinutesSmartShortcut(LatestMinutes))
}
