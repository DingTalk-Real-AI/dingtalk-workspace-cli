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
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// MyGroups: list the groups I've joined and project just the key fields
// (会话id / 名称 / 群主 / 人数 / 类型) into a clean, composed payload — instead of
// paging through `chat group list-all` and squinting at the raw MCP response.
//
// Steps:
//  1. list my groups via list_my_groups_pagination (im server); param `limit`
//     is copied verbatim from chat.go's `chat group list-all` call site.
//  2. defensively project each group's key fields (field names probed across
//     several candidate keys, since the gateway shape isn't guaranteed);
//  3. optionally keep only groups whose type matches --type (Go-side filter —
//     the underlying tool has no server-side type parameter).
//
// Read-only: it never modifies any group or membership.
//
//	dws chat +my-groups
//	dws chat +my-groups --type group
var MyGroups = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+my-groups",
	Product:     "chat",
	Description: "列出我加入的群，可按类型过滤并投影关键字段",
	Intent: "当你想快速看一眼自己都加入了哪些群、以及每个群的会话ID、名称、群主和人数，而不想翻分页或盯着原始返回时使用；" +
		"内部分页拉取你加入的群列表，把每个群防御式地投影成 会话id / 名称 / 群主 / 人数 / 类型 等关键字段，输出成干净的结果。" +
		"可选 --type 在本地按群类型过滤（底层接口本身不带类型参数，故为客户端过滤）。这是只读操作，不会改动任何群或成员关系。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "type", Type: shortcut.FlagString, Desc: "按群类型过滤（可选，如返回中的 groupType/conversationType，大小写不敏感）", Required: false},
	},
	Tips: []string{
		`dws chat +my-groups`,
		`dws chat +my-groups --type group`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — list my groups. `limit` mirrors chat.go's list_my_groups_pagination
		// call site (chat group list-all); pass a generous page size.
		data, err := rt.CallMCPData("im", "list_my_groups_pagination", map[string]any{
			"limit": 200,
		})
		if err != nil {
			return err
		}

		// Step 2 — project key fields defensively.
		groups := myGroupsExtract(data)
		typeFilter := strings.TrimSpace(rt.Str("type"))

		projected := make([]map[string]any, 0, len(groups))
		for _, g := range groups {
			row := myGroupsProject(g)
			if typeFilter != "" {
				gt, _ := row["type"].(string)
				if !strings.EqualFold(strings.TrimSpace(gt), typeFilter) {
					continue
				}
			}
			projected = append(projected, row)
		}

		return rt.Output(map[string]any{
			"count":  len(projected),
			"groups": projected,
		})
	},
}

// myGroupsExtract walks a list_my_groups_pagination response and returns its
// group entries. The gateway wraps the list under one of several common
// container keys, so we probe them (and one nested level) before giving up.
func myGroupsExtract(data map[string]any) []map[string]any {
	for _, key := range []string{"result", "list", "groups", "groupList", "items", "data", "records", "conversations"} {
		if arr, ok := data[key].([]any); ok {
			return myGroupsToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range []string{"list", "groups", "groupList", "items", "records", "result", "conversations"} {
				if arr, ok := inner[k2].([]any); ok {
					return myGroupsToMaps(arr)
				}
			}
		}
	}
	return nil
}

func myGroupsToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// myGroupsProject reshapes a single group into the projected key fields, probing
// multiple candidate keys for each because the response shape isn't guaranteed.
func myGroupsProject(m map[string]any) map[string]any {
	row := map[string]any{}
	if v := myGroupsStr(m, "openConversationId", "openConversationID", "conversationId", "openCid", "cid", "id"); v != "" {
		row["conversationId"] = v
	}
	if v := myGroupsStr(m, "name", "groupName", "title", "conversationName", "chatName"); v != "" {
		row["name"] = v
	}
	if v := myGroupsStr(m, "ownerUserId", "owner", "ownerId", "ownerOpenDingTalkId", "ownerOpenId", "groupOwnerId"); v != "" {
		row["owner"] = v
	}
	if v, ok := myGroupsInt(m, "memberCount", "memberNum", "memberSize", "userCount", "totalMember", "count"); ok {
		row["memberCount"] = v
	}
	if v := myGroupsStr(m, "groupType", "conversationType", "type", "chatType"); v != "" {
		row["type"] = v
	}
	return row
}

func myGroupsStr(m map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := m[key].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(v)
		}
	}
	return ""
}

func myGroupsInt(m map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		switch v := m[key].(type) {
		case float64:
			return int64(v), true
		case string:
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

func init() {
	shortcut.Register(MyGroups)
}
