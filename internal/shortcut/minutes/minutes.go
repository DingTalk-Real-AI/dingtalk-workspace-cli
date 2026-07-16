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

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"

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
	minutes := callListProject(data)
	return rt.Output(map[string]any{"count": len(minutes), "minutes": minutes})
}

// callListProject reshapes the raw list_by_keyword_and_time_range response into a
// clean listening-note list (taskUuid/title/creator/startTime/endTime/url/status)
// — the clean output projection applied to every list command.
// The list container and field names are probed defensively across candidate
// keys so the projection tolerates response-shape drift; unknown keys are never
// invented.
func callListProject(data map[string]any) []map[string]any {
	raw := callListResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := callListFirst(m, "taskUuid", "task_uuid", "uuid", "id"); ok {
			row["taskUuid"] = v
		}
		if v, ok := callListFirst(m, "title", "name"); ok {
			row["title"] = v
		}
		if v, ok := callListFirst(m, "creator", "creatorName", "createUserName", "creatorNick"); ok {
			row["creator"] = v
		}
		if v, ok := callListFirst(m, "startTime", "gmtStart", "beginTime"); ok {
			row["startTime"] = v
		}
		if v, ok := callListFirst(m, "endTime", "gmtEnd", "deadline"); ok {
			row["endTime"] = v
		}
		if v, ok := callListFirst(m, "url", "shareUrl", "link"); ok {
			row["url"] = v
		}
		if v, ok := callListFirst(m, "status", "taskStatus", "state"); ok {
			row["status"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// callListResolveList locates the list payload inside the response, tolerating a
// bare top-level array or nesting under result/data/list/items/records containers.
func callListResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "records", "dataList"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		// container may itself wrap the list one level deeper
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "records", "dataList", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// callListFirst returns the first present candidate key's value.
func callListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// ── get ─────────────────────────────────────────────────────────────────────

// ── update ──────────────────────────────────────────────────────────────────

// ── record ──────────────────────────────────────────────────────────────────

var RecordStart = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+record-start",
	Product:     "minutes",
	Description: "发起听记（开始录音）",
	Intent:      "当你要开始一场实时会议/通话的 AI 听记、立刻启动录音并生成一条新听记任务时使用；可选传入 AI 助理会话 ID，会真实发起录音，返回新建听记的 taskUuid 供后续暂停/恢复/结束。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "session-id", Type: shortcut.FlagString, Desc: "AI 助理会话 ID (可选)"},
	},
	Tips: []string{`dws minutes +record-start --session-id <sessionId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"cmd": "create"}
		if rt.Changed("session-id") {
			params["sessionId"] = rt.Str("session-id")
		}
		return rt.CallMCP(listeningNoteCmdTool, params)
	},
}

var RecordPause = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+record-pause",
	Product:     "minutes",
	Description: "暂停听记录音",
	Intent:      "录音进行中想临时中断（如中场休息、切换话题）又不想结束整条听记时使用；传入正在录音的听记 taskUuid，会真实暂停该次录音，之后可用 +record-resume 继续。",
	Risk:        shortcut.RiskWrite,
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
		return rt.CallMCP(listeningNoteCmdTool, params)
	},
}

var RecordResume = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+record-resume",
	Product:     "minutes",
	Description: "恢复听记录音",
	Intent:      "之前用 +record-pause 暂停过的听记，现在想接着录时使用；传入该听记 taskUuid，会真实恢复录音，继续追加到同一条听记中。",
	Risk:        shortcut.RiskWrite,
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
		return rt.CallMCP(listeningNoteCmdTool, params)
	},
}

var RecordStop = shortcut.Shortcut{
	Service:     "minutes",
	Command:     "+record-stop",
	Product:     "minutes",
	Description: "结束听记录音",
	Intent:      "会议开完、想彻底停止录音并让系统开始生成转写与 AI 纪要时使用；传入正在录音的听记 taskUuid，会真实结束该次录音，结束后无法再恢复到这条听记继续录。",
	Risk:        shortcut.RiskWrite,
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
		return rt.CallMCP(listeningNoteCmdTool, params)
	},
}

// ── mind-graph ──────────────────────────────────────────────────────────────

// ── speaker ─────────────────────────────────────────────────────────────────

// ── hot-word ────────────────────────────────────────────────────────────────

// ── replace-text ────────────────────────────────────────────────────────────

// ── upload ──────────────────────────────────────────────────────────────────

// ── permission ──────────────────────────────────────────────────────────────

// ── tag ─────────────────────────────────────────────────────────────────────

func init() {
	shortcut.Register(
		ListMine,
		ListShared,
		ListAll,
		RecordStart,
		RecordPause,
		RecordResume,
		RecordStop,
	)
}
