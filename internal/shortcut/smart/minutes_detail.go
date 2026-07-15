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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
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
	Description: "一条命令聚合取一条妙记（听记）的多项产物（基础信息/摘要/关键词/逐字稿/待办）",
	Intent: "当你已经有某条听记的 taskUuid，想在一次操作里同时拿到它的基础信息、AI 摘要、关键词、逐字稿和待办，而不想分别敲 4~5 个子命令再自己拼时使用；" +
		"内部按 --artifacts 选择要拉的产物（默认全部：basic/summary/keywords/transcript/todos），逐个调用对应的原子工具并聚合成一个结果，" +
		"某一项失败不会中断整体（会以错误字符串记录在该项下）。这是纯只读操作，不会修改听记；--direction 仅影响逐字稿排序（0=正序默认，1=倒序）。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid（必填）", Required: true},
		{Name: "artifacts", Type: shortcut.FlagStringSlice, Desc: "要拉取的产物子集（默认全部）", Required: false, Enum: []string{"basic", "summary", "keywords", "transcript", "todos"}},
		{Name: "direction", Type: shortcut.FlagString, Desc: "逐字稿排序: 0=正序(默认), 1=倒序（可选）", Required: false, Enum: []string{"0", "1"}},
	},
	Tips: []string{
		`dws minutes +detail --id <taskUuid>`,
		`dws minutes +detail --id <taskUuid> --artifacts summary,todos`,
		`dws minutes +detail --id <taskUuid> --direction 1`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		taskUUID := rt.Str("id")
		direction := rt.Str("direction")
		if direction == "" {
			direction = "0"
		}

		// Resolve which artifacts to pull; default to all in a stable order.
		want := rt.StrSlice("artifacts")
		if len(want) == 0 {
			want = minutesArtifactOrder
		}

		bundle := map[string]any{"taskUuid": taskUUID}
		for _, raw := range want {
			name := strings.ToLower(strings.TrimSpace(raw))
			tool, ok := minutesArtifactTools[name]
			if !ok {
				continue // guarded by Validate, but stay defensive
			}
			params := map[string]any{"taskUuid": taskUUID}
			if name == "transcript" {
				params["direction"] = direction
			}
			data, err := rt.CallMCPData("minutes", tool, params)
			if err != nil {
				// Partial-failure tolerance: record and continue.
				bundle[name] = map[string]any{"error": err.Error()}
				continue
			}
			bundle[name] = data
		}

		return rt.Output(bundle)
	},
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
	shortcut.Register(MinutesDetail)
}
