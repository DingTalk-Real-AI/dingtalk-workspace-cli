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

// Package minutes contains declarative shortcuts for DingTalk AI 听记 (minutes).
// Tool names and params mirror internal/helpers/minutes.go verbatim.
package minutes

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/minutesdata"
)

// listeningNoteCmdTool is the gateway-registered name of the 听记指令 tool.
// It is registered under a Chinese title; the legacy English name returns
// "PARAM_ERROR - 未找到指定工具". Copied verbatim from the helper.
const listeningNoteCmdTool = "执行听记指令-发起AI听记录音"

// ── list ────────────────────────────────────────────────────────────────────

var ListMine = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+list-mine",
	Product:     "minutes",
	Description: "查询我创建的听记列表",
	Intent:      "当你想找回自己发起或录制的某次听记（会议纪要），却只记得大概的标题关键字时使用；可按关键字筛选并分页，返回自己创建的听记列表及其 taskUuid，便于后续查看摘要、转写或待办。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "minutes",
			Name:           "shortcut_list_mine",
			CanonicalPath:  "minutes.shortcut_list_mine",
			CLIPath:        "minutes +list-mine",
			PrimaryCLIPath: "minutes +list-mine",
		},
		Description: "查询我创建的听记列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询我创建的听记列表",
			UseWhen:      []string{"当你想找回自己发起或录制的某次听记（会议纪要），却只记得大概的标题关键字时使用；可按关键字筛选并分页，返回自己创建的听记列表及其 taskUuid，便于后续查看摘要、转写或待办。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws minutes +list-mine --query \"周会\" --limit 10"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字筛选"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "10", Desc: "每页数据条数"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页 token (首页留空)"},
	},
	Tips: []string{`dws minutes +list-mine --query "周会" --limit 10`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return callList(rt, "created")
	},
}

var ListShared = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+list-shared",
	Product:     "minutes",
	Description: "查询他人共享给我的听记列表",
	Intent:      "当你要找同事分享给你的会议听记、想快速定位别人共享过来的纪要时使用；可按关键字筛选并分页，返回他人共享给你的听记列表及 taskUuid。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "minutes",
			Name:           "shortcut_list_shared",
			CanonicalPath:  "minutes.shortcut_list_shared",
			CLIPath:        "minutes +list-shared",
			PrimaryCLIPath: "minutes +list-shared",
		},
		Description: "查询他人共享给我的听记列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询他人共享给我的听记列表",
			UseWhen:      []string{"当你要找同事分享给你的会议听记、想快速定位别人共享过来的纪要时使用；可按关键字筛选并分页，返回他人共享给你的听记列表及 taskUuid。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws minutes +list-shared --limit 20"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字筛选"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "10", Desc: "每页数据条数"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页 token (首页留空)"},
	},
	Tips: []string{`dws minutes +list-shared --limit 20`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return callList(rt, "shared")
	},
}

var ListAll = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+list-all",
	Product:     "minutes",
	Description: "查询我有权限访问的所有听记列表",
	Intent:      "当你不确定某条听记是自己创建还是别人共享、想在所有可访问的听记中一次性检索时使用；合并「我创建的」和「共享给我的」，按关键字筛选并分页返回全部有权限的听记及 taskUuid。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "minutes",
			Name:           "shortcut_list_all",
			CanonicalPath:  "minutes.shortcut_list_all",
			CLIPath:        "minutes +list-all",
			PrimaryCLIPath: "minutes +list-all",
		},
		Description: "查询我有权限访问的所有听记列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询我有权限访问的所有听记列表",
			UseWhen:      []string{"当你不确定某条听记是自己创建还是别人共享、想在所有可访问的听记中一次性检索时使用；合并「我创建的」和「共享给我的」，按关键字筛选并分页返回全部有权限的听记及 taskUuid。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws minutes +list-all --query \"周会\" --limit 20"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字筛选"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "10", Desc: "每页数据条数"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页 token (首页留空)"},
	},
	Tips: []string{`dws minutes +list-all --query "周会" --limit 20`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return callList(rt, "noLimit")
	},
}

// callList wraps list_by_keyword_and_time_range for mine/shared/all.
func callList(rt *shortcut.RuntimeContext, belonging string) error {
	params := map[string]any{"belongingConditionId": belonging}
	if rt.Changed("limit") {
		params["maxResults"] = rt.Int("limit")
	}
	if rt.Changed("query") {
		params["keyword"] = rt.Str("query")
	}
	if rt.Changed("cursor") {
		params["nextToken"] = rt.Str("cursor")
	}
	data, err := rt.CallMCPData("minutes", "list_by_keyword_and_time_range", params)
	if err != nil {
		return err
	}
	minutes, err := callListProject(data)
	if err != nil {
		return err
	}
	return rt.Output(map[string]any{"count": len(minutes), "minutes": minutes})
}

// callListProject reshapes the raw list_by_keyword_and_time_range response into a
// clean listening-note list (taskUuid/title/creator/startTime/endTime/url/status)
// — the clean output projection applied to every list command.
// The list container and field names are probed defensively across candidate
// keys so the projection tolerates response-shape drift; unknown keys are never
// invented.
func callListProject(data map[string]any) ([]map[string]any, error) {
	page, err := minutesdata.ParseListPage(data)
	if err != nil {
		return nil, err
	}
	return minutesdata.ProjectList(page)
}

// ── get ─────────────────────────────────────────────────────────────────────

// ── update ──────────────────────────────────────────────────────────────────

// ── record ──────────────────────────────────────────────────────────────────

var RecordStart = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+record-start",
	Product:     "minutes",
	Description: "发起听记（开始录音）",
	Intent:      "当你要开始一场实时会议/通话的 AI 听记、立刻启动录音时使用；可选传入 AI 助理会话 ID。当前网关 create 回执不返回 taskUuid，只能证明录音指令已被接受；随后需用 +latest/+search 定位新听记。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "minutes",
			Name:           "shortcut_record_start",
			CanonicalPath:  "minutes.shortcut_record_start",
			CLIPath:        "minutes +record-start",
			PrimaryCLIPath: "minutes +record-start",
		},
		Description: "发起听记（开始录音）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "发起听记（开始录音）",
			UseWhen:      []string{"当你要开始一场实时会议/通话的 AI 听记、立刻启动录音时使用；网关 create 只返回已接受回执，不返回 taskUuid，随后需用 +latest/+search 定位新听记。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws minutes +record-start --session-id <sessionId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "session-id", Type: shortcut.FlagString, Desc: "AI 助理会话 ID (可选)"},
	},
	Tips: []string{`dws minutes +record-start --session-id <sessionId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"cmd": "create"}
		if rt.Changed("session-id") {
			params["sessionId"] = rt.Str("session-id")
		}
		return executeRecordCommand(rt, "create", "", params)
	},
}

var RecordPause = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+record-pause",
	Product:     "minutes",
	Description: "暂停听记录音",
	Intent:      "录音进行中想临时中断（如中场休息、切换话题）又不想结束整条听记时使用；传入正在录音的听记 taskUuid，会真实暂停该次录音，之后可用 +record-resume 继续。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: minutesContract(
		"+record-pause",
		"暂停听记录音",
		"实时听记仍在录制，但要临时中断且保留后续恢复能力时使用；必须传当前录制任务的 taskUuid。",
		[]string{"会议已经结束时使用 +record-stop；只是查看听记状态或内容时使用只读命令"},
		[]string{"dws minutes +record-pause --id <taskUuid>"},
	),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid", Required: true},
		{Name: "session-id", Type: shortcut.FlagString, Desc: "AI 助理会话 ID (可选)"},
	},
	Tips: []string{`dws minutes +record-pause --id <taskUuid>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"cmd": "pause", "uuid": rt.Str("id")}
		if rt.Changed("session-id") {
			params["sessionId"] = rt.Str("session-id")
		}
		return executeRecordCommand(rt, "pause", rt.Str("id"), params)
	},
}

var RecordResume = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+record-resume",
	Product:     "minutes",
	Description: "恢复听记录音",
	Intent:      "之前用 +record-pause 暂停过的听记，现在想接着录时使用；传入该听记 taskUuid，会真实恢复录音，继续追加到同一条听记中。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: minutesContract(
		"+record-resume",
		"恢复听记录音",
		"实时听记此前已暂停、现在要继续向同一 taskUuid 追加录音时使用。",
		[]string{"已经结束的听记不能恢复；要新建录制时使用 +record-start"},
		[]string{"dws minutes +record-resume --id <taskUuid>"},
	),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid", Required: true},
		{Name: "session-id", Type: shortcut.FlagString, Desc: "AI 助理会话 ID (可选)"},
	},
	Tips: []string{`dws minutes +record-resume --id <taskUuid>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"cmd": "resume", "uuid": rt.Str("id")}
		if rt.Changed("session-id") {
			params["sessionId"] = rt.Str("session-id")
		}
		return executeRecordCommand(rt, "resume", rt.Str("id"), params)
	},
}

var RecordStop = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+record-stop",
	Product:     "minutes",
	Description: "结束听记录音",
	Intent:      "会议开完、想彻底停止录音并让系统开始生成转写与 AI 纪要时使用；传入正在录音的听记 taskUuid，会真实结束该次录音，结束后无法再恢复到这条听记继续录。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: minutesContract(
		"+record-stop",
		"结束听记录音",
		"会议结束且要永久停止指定 taskUuid 的实时录制，让服务进入转写和纪要处理阶段时使用。",
		[]string{"仍计划继续录音时使用 +record-pause；结束后不能通过 +record-resume 继续同一条录制"},
		[]string{"dws minutes +record-stop --id <taskUuid>"},
	),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid", Required: true},
		{Name: "session-id", Type: shortcut.FlagString, Desc: "AI 助理会话 ID (可选)"},
	},
	Tips: []string{`dws minutes +record-stop --id <taskUuid>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"cmd": "end", "uuid": rt.Str("id")}
		if rt.Changed("session-id") {
			params["sessionId"] = rt.Str("session-id")
		}
		return executeRecordCommand(rt, "end", rt.Str("id"), params)
	},
}

func executeRecordCommand(rt *shortcut.RuntimeContext, expectedCmd, taskUUID string, params map[string]any) error {
	data, err := rt.CallMCPWriteDataStrict("minutes", listeningNoteCmdTool, params)
	if err != nil {
		return err
	}
	result, err := minutesdata.RecordResult(expectedCmd, taskUUID, data)
	if err != nil {
		return err
	}
	return rt.Output(map[string]any{
		"accepted": true,
		"command":  expectedCmd,
		"taskUuid": taskUUID,
		"result":   result,
	})
}

// ── mind-graph ──────────────────────────────────────────────────────────────

// ── speaker ─────────────────────────────────────────────────────────────────

// ── hot-word ────────────────────────────────────────────────────────────────

// ── replace-text ────────────────────────────────────────────────────────────

// ── upload ──────────────────────────────────────────────────────────────────

// ── permission ──────────────────────────────────────────────────────────────

// ── tag ─────────────────────────────────────────────────────────────────────

func init() {
	shortcut.Register(finalizeMinutesShortcuts(
		ListMine,
		ListShared,
		ListAll,
		RecordStart,
		RecordPause,
		RecordResume,
		RecordStop,
	)...)
}
