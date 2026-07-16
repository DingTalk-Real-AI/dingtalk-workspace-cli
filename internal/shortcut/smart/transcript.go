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

// Transcript: fetch the verbatim transcript (逐字稿 / 语音转写原文) of MY most
// recent minutes (听记) in one step.
//
// Steps:
//  1. list my minutes via list_by_keyword_and_time_range (belongingConditionId
//     = "created"), optionally filtered by --keyword;
//  2. pick the newest entry (largest create time, falling back to the first
//     item) and read its taskUuid — reusing latestMinutesTaskUUID;
//  3. print that minute's verbatim transcript via get_minutes_transcription
//     (taskUuid + direction, mirroring helpers.minutesGetTranscriptionCmd).
//
// If the list is empty it reports "暂无妙记" instead of failing obscurely.
//
//	dws minutes +transcript
//	dws minutes +transcript --keyword 周会
//	dws minutes +transcript --direction 1
var Transcript = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+transcript",
	Product:     "minutes",
	Description: "取我最新一条妙记（听记）的逐字稿（语音转写原文）",
	Intent: "当你想直接读回自己最近一次会议听记的逐字稿（完整语音转写原文），又不想先翻列表、复制 taskUuid 再单独查转写时使用；" +
		"内部先列出你创建的听记（可用 --keyword 缩小范围），自动挑出最新的一条，再拉取它的逐字记录（每条含发言人、文本、时间戳）。" +
		"可用 --direction 控制排序：0=正序（时间递增，默认），1=倒序。这是只读操作，不会修改任何听记；" +
		"若你名下没有任何听记则提示「暂无妙记」。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "keyword", Type: shortcut.FlagString, Desc: "按关键字过滤听记（可选）", Required: false},
		{Name: "direction", Type: shortcut.FlagString, Desc: "排序方向: 0=正序(默认), 1=倒序（可选）", Required: false},
	},
	Tips: []string{
		`dws minutes +transcript`,
		`dws minutes +transcript --keyword 周会`,
		`dws minutes +transcript --direction 1`,
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

		// Step 2 — locate the newest minute's taskUuid (reuses helper from
		// latest_minutes.go).
		taskUUID := latestMinutesTaskUUID(data)
		if taskUUID == "" {
			return apperrors.NewValidation("暂无妙记")
		}

		// Step 3 — print its verbatim transcript. taskUuid + direction mirror
		// helpers.minutesGetTranscriptionCmd (get_minutes_transcription).
		// direction defaults to "0" (正序) when not provided.
		direction := rt.Str("direction")
		if direction == "" {
			direction = "0"
		}
		return rt.CallMCP("get_minutes_transcription", map[string]any{
			"taskUuid":  taskUUID,
			"direction": direction,
		})
	},
}

func init() {
	shortcut.Register(Transcript)
}
