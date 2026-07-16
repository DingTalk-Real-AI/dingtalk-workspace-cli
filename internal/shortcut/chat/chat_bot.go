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

package chat

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// BotSearch searches robots created by the current user (search_my_robots, bot).
var BotSearch = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+bot-search",
	Product:     "bot",
	Description: "搜索当前用户自己创建的机器人",
	Intent:      "当你要管理或复用自己创建的机器人（比如查到其 robotCode 以便让它进群或发消息）时使用；按机器人名称模糊搜索，只返回当前用户名下创建的机器人列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "page", Type: shortcut.FlagInt, Default: "1", Desc: "页码"},
		{Name: "size", Type: shortcut.FlagInt, Desc: "每页数量"},
		{Name: "name", Type: shortcut.FlagString, Desc: "robotName 模糊匹配"},
	},
	Tips: []string{`dws chat +bot-search --page 1 --name "日报"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"currentPage": rt.Int("page")}
		if rt.Int("size") > 0 {
			params["pageSize"] = rt.Int("size")
		}
		if rt.Str("name") != "" {
			params["robotName"] = rt.Str("name")
		}
		data, err := rt.CallMCPData("bot", "search_my_robots", params)
		if err != nil {
			return err
		}
		robots := botSearchProject(data)
		return rt.Output(map[string]any{"count": len(robots), "robots": robots})
	},
}

// botSearchProject reshapes the raw search_my_robots response into a clean robot
// list (robotCode/robotName/status) — clean output projection. The
// list container and field names are probed defensively across candidate keys.
func botSearchProject(data map[string]any) []map[string]any {
	raw := botResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := botFirst(m, "robotCode", "robot_code", "code", "id"); ok {
			row["robotCode"] = v
		}
		if v, ok := botFirst(m, "robotName", "robot_name", "name"); ok {
			row["robotName"] = v
		}
		if v, ok := botFirst(m, "status", "robotStatus", "state"); ok {
			row["status"] = v
		}
		if v, ok := botFirst(m, "gmtCreate", "createTime", "create_time"); ok {
			row["gmtCreate"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// botResolveList locates the list payload inside the response, tolerating a bare
// top-level array or nesting under result/data/list/items containers.
func botResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "robots", "records"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "robots", "records", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// botFirst returns the first present candidate key's value.
func botFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// BotFind searches all available robots (search_bots, bot).
var BotFind = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+bot-find",
	Product:     "bot",
	Description: "搜索全部可用机器人（含他人/官方，返回 openDingTalkId 可发单聊）",
	Intent:      "当你想找到平台上任意可用机器人（含他人创建或官方助手，例如某个日报/审批机器人）以便与其发起单聊时使用；输入关键词，返回含 openDingTalkId 的机器人列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页返回数量"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，翻页传 nextCursor"},
	},
	Tips: []string{`dws chat +bot-find --query "日报"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"keyword": rt.Str("query")}
		if rt.Int("limit") > 0 {
			params["limit"] = rt.Int("limit")
		}
		if rt.Str("cursor") != "" {
			params["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("bot", "search_bots", params)
		if err != nil {
			return err
		}
		bots := botFindProject(data)
		return rt.Output(map[string]any{"count": len(bots), "bots": bots})
	},
}

// botFindProject reshapes the raw search_bots response into a clean bot list
// (openDingTalkId/name/robotCode) — clean output projection. The
// list container and field names are probed defensively across candidate keys.
func botFindProject(data map[string]any) []map[string]any {
	raw := botResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := botFirst(m, "openDingTalkId", "open_ding_talk_id", "openId"); ok {
			row["openDingTalkId"] = v
		}
		if v, ok := botFirst(m, "name", "robotName", "botName"); ok {
			row["name"] = v
		}
		if v, ok := botFirst(m, "robotCode", "robot_code", "code", "id"); ok {
			row["robotCode"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// SearchCommonGroups searches groups shared with given people (search_common_groups, chat server).
func init() {
	shortcut.Register(
		BotSearch,
		BotFind,
	)
}
