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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/minutesdata"
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
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "minutes",
	Command:       "+transcript",
	Product:       "minutes",
	Description:   "读取指定或我最新一条听记的完整逐字稿，并交付分页完整性证据",
	Intent: "当你要读取已知 taskUuid（--id）的完整逐字稿，或不传 --id 自动选择自己最新听记时使用；" +
		"默认追完 nextToken、跨页去重，并在 data 输出 complete/pages、在 meta.pagination 输出端点耗尽状态和续页 token；只有显式 --single-page 才停在一页。" +
		"可用 --direction 控制排序，--keyword 仅在自动选择最新听记时缩小候选；任何分页漂移或中途失败都返回非零而不是把部分原文当成全集。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: withMinutesTranscriptResult(minutesSmartContract(
		"+transcript",
		"读取指定或最新听记的完整逐字稿",
		"需要读取逐字稿并自动追完 nextToken、去重段落，同时看到 data.complete/data.pages 与 meta.pagination 完整性证据时使用；不传 --id 时严格选择最新听记。",
		[]string{"只需单个原始分页响应时使用底层转写命令；需要汇总多个制品时使用 +detail"},
		[]string{"dws minutes +transcript --id <taskUuid>", "dws minutes +transcript --keyword 周会"},
		nil,
	)),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid；不传时选择我最新的一条"},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "按关键字过滤听记（可选）", Required: false},
		{Name: "direction", Type: shortcut.FlagString, Desc: "排序方向: 0=正序(默认), 1=倒序（可选）", Required: false, Enum: []string{"0", "1"}},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "单页/续拉的起始 nextToken"},
		{Name: "single-page", Type: shortcut.FlagBool, Desc: "只读取一页；输出 data.complete 与 meta.pagination.next_token"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "100", Desc: "自动翻页安全上限"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"page-limit"}, Description: "--page-limit 必须大于 0"}},
	Tips: []string{
		`dws minutes +transcript --id <taskUuid>`,
		`dws minutes +transcript --keyword 周会`,
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("page-limit") <= 0 {
			return apperrors.NewValidation("--page-limit 必须大于 0")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		taskUUID := rt.Str("id")
		if taskUUID == "" {
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
			taskUUID, err = latestMinutesTaskUUID(data)
			if err != nil {
				return err
			}
			if taskUUID == "" {
				return apperrors.NewValidation("暂无妙记")
			}
		}

		// Step 3 — print its verbatim transcript. taskUuid + direction mirror
		// helpers.minutesGetTranscriptionCmd (get_minutes_transcription).
		// direction defaults to "0" (正序) when not provided.
		direction := rt.Str("direction")
		if direction == "" {
			direction = "0"
		}
		result, readErr := collectMinutesTranscript(rt, taskUUID, direction, rt.Str("cursor"), rt.Bool("single-page"), rt.Int("page-limit"))
		payload := minutesdata.TranscriptPayload(taskUUID, direction, result)
		return outputMinutesTranscriptResult(rt, payload, result, readErr)
	},
}

func collectMinutesTranscript(rt *shortcut.RuntimeContext, taskUUID, direction, cursor string, singlePage bool, pageLimit int) (minutesdata.TranscriptResult, error) {
	return minutesdata.CollectTranscript(func(nextToken string) (map[string]any, error) {
		params := map[string]any{"taskUuid": taskUUID, "direction": direction}
		if nextToken != "" {
			params["nextToken"] = nextToken
		}
		return rt.CallMCPData("minutes", "get_minutes_transcription", params)
	}, cursor, singlePage, pageLimit)
}

func minutesTranscriptReadError(taskUUID string, result minutesdata.TranscriptResult, cause error) error {
	return apperrors.NewAPI(
		fmt.Sprintf("逐字稿读取不完整：已读取 %d 页、%d 个段落", result.Pages, len(result.Paragraphs)),
		apperrors.WithOperation("minutes/get_minutes_transcription"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithReason("minutes_transcript_incomplete"),
		apperrors.WithRetryable(false),
		apperrors.WithDetails(map[string]any{
			"taskUuid":       taskUUID,
			"pages":          result.Pages,
			"paragraphCount": len(result.Paragraphs),
			"nextToken":      result.NextToken,
			"cause":          cause.Error(),
		}),
	)
}

func init() {
	shortcut.Register(finalizeMinutesSmartShortcut(Transcript))
}
