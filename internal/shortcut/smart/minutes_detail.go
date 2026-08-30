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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/minutesdata"
)

var (
	smartMinutesMarshalIndent = json.MarshalIndent
	smartMinutesPublishBytes  = localio.PublishBytes
)

// MinutesDetail: fetch several artifacts of ONE minute (听记) in a single command
// and print them as one projected bundle.
//
// dws exposes each artifact as its own atomic tool (get_minutes_basic_info /
// get_minutes_ai_summary / get_minutes_keywords / get_minutes_transcription /
// list_minutes_todos). To assemble a full picture a user otherwise has to call
// 4–5 commands and stitch the taskUuid through each. This shortcut fans them out
// for one taskUuid, tolerates partial failure (a failing artifact is recorded as
// an error string rather than aborting the whole bundle) and projects the result
// through rt.Output so it honours --format/--jq/--fields.
//
// --artifacts selects which artifacts to pull (default: all). Each tool's params
// mirror the helper call sites in internal/helpers/minutes.go: every one takes a
// single "taskUuid", and transcription additionally takes "direction".
//
//	dws minutes +detail --id <taskUuid>
//	dws minutes +detail --id <taskUuid> --artifacts summary,todos
//	dws minutes +detail --id <taskUuid> --direction 1
var MinutesDetail = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+detail",
	Product:     "minutes",
	Description: "批量聚合听记基础信息、摘要、关键词、完整逐字稿和行动项，支持安全文件输出",
	Intent: "当你已有一个或最多 50 个 taskUuid，要一次读取所选 basic/summary/keywords/transcript/todos 产物时使用；" +
		"逐字稿默认追完所有分页并可用 file/both 安全落盘，批量结果带 complete 与逐项 failure ledger。" +
		"任一所选产物失败都会保留已取得结果并返回非零，绝不会把 partial bundle 当成完整成功；这是纯只读操作。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "minutes",
			Name:           "shortcut_detail",
			CanonicalPath:  "minutes.shortcut_detail",
			CLIPath:        "minutes +detail",
			PrimaryCLIPath: "minutes +detail",
		},
		Description: "批量聚合听记基础信息、摘要、关键词、完整逐字稿和行动项，支持安全文件输出",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "一条命令聚合取一条妙记（听记）的多项产物（基础信息/摘要/关键词/逐字稿/待办）",
			UseWhen:      []string{"当你已经有某条听记的 taskUuid，想在一次操作里同时拿到它的基础信息、AI 摘要、关键词、逐字稿和待办，而不想分别敲 4~5 个子命令再自己拼时使用；内部按 --artifacts 选择要拉的产物（默认全部：basic/summary/keywords/transcript/todos），逐个调用对应的原子工具并聚合成一个结果，某一项失败不会中断整体（会以错误字符串记录在该项下）。这是纯只读操作，不会修改听记；--direction 仅影响逐字稿排序（0=正序默认，1=倒序）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws minutes +detail --id <taskUuid>",
				"dws minutes +detail --ids <uuid1,uuid2> --artifacts summary,transcript --transcript-output file",
			},
		},
		Result: minutesDetailResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "单个听记 taskUuid"},
		{Name: "ids", Type: shortcut.FlagStringSlice, Desc: "多个听记 taskUuid，最多 50 个"},
		{Name: "artifacts", Type: shortcut.FlagStringSlice, Desc: "要拉取的产物子集（默认全部）", Required: false, Enum: []string{"basic", "summary", "keywords", "transcript", "todos"}},
		{Name: "direction", Type: shortcut.FlagString, Desc: "逐字稿排序: 0=正序(默认), 1=倒序（可选）", Required: false, Enum: []string{"0", "1"}},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "逐字稿单页/续拉的起始 nextToken"},
		{Name: "single-page", Type: shortcut.FlagBool, Desc: "逐字稿只读取一页并返回 nextToken"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "100", Desc: "逐字稿自动翻页安全上限"},
		{Name: "transcript-output", Type: shortcut.FlagString, Default: "inline", Desc: "逐字稿输出方式", Enum: []string{"inline", "file", "both"}},
		{Name: "output-dir", Type: shortcut.FlagString, Default: "minutes-transcripts", Desc: "file/both 模式下的安全相对输出目录"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"id", "ids"}, Description: "--id 与 --ids 只能选择一种，去重后 taskUuid 必须为 1..50 个"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-limit"}, Description: "--page-limit 必须大于 0"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"transcript-output", "output-dir"}, Description: "file/both 输出目录必须是安全相对路径"},
	},
	Tips: []string{
		`dws minutes +detail --id <taskUuid>`,
		`dws minutes +detail --ids <uuid1,uuid2> --artifacts summary,transcript --transcript-output file`,
		`dws minutes +detail --id <taskUuid> --direction 1 --transcript-output both`,
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("page-limit") <= 0 {
			return apperrors.NewValidation("--page-limit 必须大于 0")
		}
		if strings.TrimSpace(rt.Str("id")) != "" && len(rt.StrSlice("ids")) > 0 {
			return apperrors.NewValidation("--id 与 --ids 只能选择一种")
		}
		ids := minutesDetailIDs(rt)
		if len(ids) == 0 || len(ids) > 50 {
			return apperrors.NewValidation("taskUuid 数量必须为 1..50")
		}
		if rt.Str("transcript-output") != "inline" {
			return localio.ValidateOutput(rt.Str("output-dir"))
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		ids := minutesDetailIDs(rt)
		direction := rt.Str("direction")
		if direction == "" {
			direction = "0"
		}

		// Resolve which artifacts to pull; default to all in a stable order.
		want := rt.StrSlice("artifacts")
		if len(want) == 0 {
			want = minutesArtifactOrder
		}

		results := make([]map[string]any, 0, len(ids))
		allFailures := make([]map[string]any, 0)
		succeeded := 0
		for _, taskUUID := range ids {
			bundle, failures := readMinutesDetail(rt, taskUUID, want, direction)
			results = append(results, bundle)
			if len(failures) == 0 {
				succeeded++
			}
			for _, failure := range failures {
				failure["taskUuid"] = taskUUID
				allFailures = append(allFailures, failure)
			}
		}
		if !rt.Changed("ids") && len(results) == 1 {
			if err := rt.Output(results[0]); err != nil {
				return err
			}
			if len(allFailures) > 0 {
				return minutesDetailReadError(ids, allFailures)
			}
			return nil
		}
		payload := map[string]any{
			"operation": "minutes.detail", "complete": len(allFailures) == 0,
			"requested": len(ids), "succeeded": succeeded, "failed": len(ids) - succeeded,
			"results": results, "failures": allFailures,
		}
		if err := rt.Output(payload); err != nil {
			return err
		}
		if len(allFailures) > 0 {
			return minutesDetailReadError(ids, allFailures)
		}
		return nil
	},
}

func readMinutesDetail(rt *shortcut.RuntimeContext, taskUUID string, want []string, direction string) (map[string]any, []map[string]any) {
	bundle := map[string]any{"taskUuid": taskUUID}
	failures := make([]map[string]any, 0)
	for _, raw := range want {
		name := strings.ToLower(strings.TrimSpace(raw))
		tool, ok := minutesArtifactTools[name]
		if !ok {
			continue
		}
		if name == "transcript" {
			result, err := collectMinutesTranscript(rt, taskUUID, direction, rt.Str("cursor"), rt.Bool("single-page"), rt.Int("page-limit"))
			transcript := minutesdata.TranscriptPayload(taskUUID, direction, result)
			if err == nil && rt.Str("transcript-output") != "inline" {
				raw, marshalErr := smartMinutesMarshalIndent(transcript, "", "  ")
				if marshalErr == nil {
					raw = append(raw, '\n')
					published, publishErr := smartMinutesPublishBytes(raw, localio.PublishBytesOptions{Output: strings.TrimSuffix(rt.Str("output-dir"), "/") + "/", PreferredName: taskUUID + ".json"})
					if publishErr == nil {
						transcript["path"] = published.RelativePath
						transcript["sizeBytes"] = published.SizeBytes
						transcript["fileComplete"] = true
						if rt.Str("transcript-output") == "file" {
							delete(transcript, "paragraphList")
							transcript["inline"] = false
						}
					} else {
						err = publishErr
					}
				} else {
					err = marshalErr
				}
			}
			bundle[name] = transcript
			if err != nil {
				failures = append(failures, map[string]any{"artifact": name, "error": err.Error()})
			}
			continue
		}
		data, err := rt.CallMCPData("minutes", tool, map[string]any{"taskUuid": taskUUID})
		if err != nil {
			bundle[name] = map[string]any{"error": err.Error()}
			failures = append(failures, map[string]any{"artifact": name, "error": err.Error()})
			continue
		}
		if err := minutesdata.ValidateArtifact(name, taskUUID, data); err != nil {
			bundle[name] = map[string]any{"error": err.Error()}
			failures = append(failures, map[string]any{"artifact": name, "error": err.Error()})
			continue
		}
		bundle[name] = data
	}
	bundle["complete"] = len(failures) == 0
	bundle["failureCount"] = len(failures)
	if len(failures) > 0 {
		bundle["failures"] = failures
	}
	return bundle, failures
}

func minutesDetailIDs(rt *shortcut.RuntimeContext) []string {
	values := []string{rt.Str("id")}
	values = append(values, rt.StrSlice("ids")...)
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			if item != "" && !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	return out
}

func minutesDetailReadError(ids []string, failures []map[string]any) error {
	return apperrors.NewAPI(
		fmt.Sprintf("听记详情读取不完整：%d 个产物失败", len(failures)),
		apperrors.WithOperation("minutes/+detail"),
		apperrors.WithOrigin("shortcut"),
		apperrors.WithFailureStage("artifact_read"),
		apperrors.WithReason("minutes_detail_incomplete"),
		apperrors.WithRetryable(false),
		apperrors.WithDetails(map[string]any{"taskUuids": ids, "failures": failures}),
	)
}

// minutesArtifactTools maps the user-facing artifact name to the real MCP tool
// (ground truth: internal/helpers/minutes.go). Each tool takes a single
// "taskUuid"; transcript additionally takes "direction".
var minutesArtifactTools = map[string]string{
	"basic":      "get_minutes_basic_info",
	"summary":    "get_minutes_ai_summary",
	"keywords":   "get_minutes_keywords",
	"transcript": "get_minutes_transcription",
	"todos":      "list_minutes_todos",
}

// minutesArtifactOrder is the stable default fan-out order (map iteration is
// unordered, so the "all" case uses this slice).
var minutesArtifactOrder = []string{"basic", "summary", "keywords", "transcript", "todos"}

func init() {
	shortcut.Register(finalizeMinutesSmartShortcut(MinutesDetail))
}
