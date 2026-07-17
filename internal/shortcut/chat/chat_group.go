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
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ChatSearch searches groups by keyword (search_groups on the im server).
var ChatSearch = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-search",
	Product:     "im",
	Description: "按关键词搜索群聊",
	Intent:      "当你只记得群名称关键词、需要拿到群 openConversationId 以便发消息或管理该群时使用；按群名模糊搜索，只读分页返回匹配的群列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "群名称关键词"},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "--query 的别名", Hidden: true},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页返回数量"},
		{Name: "size", Type: shortcut.FlagInt, Desc: "--limit 的旧版别名", Hidden: true},
		{Name: "cursor", Type: shortcut.FlagString, Default: "0", Desc: "分页游标，翻页传 nextCursor"},
		{Name: "exclude-muted", Type: shortcut.FlagBool, Desc: "排除已设置免打扰的群聊"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"query", "keyword"}},
	},
	Tips: []string{`dws chat +chat-search --query "项目冲刺"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		query := rt.Str("query")
		if query == "" {
			query = rt.Str("keyword")
		}
		params := map[string]any{
			"keyword": query,
			"limit":   rt.IntFirst("limit", "size"),
			"cursor":  rt.Str("cursor"),
		}
		if rt.Bool("exclude-muted") {
			params["excludeMuted"] = true
		}
		return rt.CallMCP("search_groups", params)
	},
}

// ChatMembersList lists members of a group (get_group_members, chat server).
// ChatMembersGet batch-queries member detail by ids (list_group_member_by_ids, im).
var ChatMembersGet = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-members-get",
	Product:     "im",
	Description: "根据成员 openDingTalkId 批量查询群成员详情",
	Intent:      "当你已有若干成员的 openDingTalkId、需要批量获取他们在该群内的详情（群昵称、角色等）时使用；只读，需传群 openConversationId 和成员 openDingTalkId 列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "成员 openDingTalkId 列表", Required: true},
	},
	Tips: []string{`dws chat +chat-members-get --id <openConversationId> --users odid1,odid2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("list_group_member_by_ids", map[string]any{
			"openConversationId":    rt.Str("id"),
			"cid":                   rt.Str("id"),
			"memberOpenDingTalkIds": rt.StrSlice("users"),
		})
	},
}

// ChatMemberAdd adds members to a group (add_group_member, chat server).
// ChatMemberRemove removes members from a group (remove_group_member, chat server).
// ChatUpdateName renames a group (update_group_name, chat server).
// ChatTransferOwner transfers group ownership (transfer_group_owner, im).
var ChatTransferOwner = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-transfer-owner",
	Product:     "im",
	Description: "转让群主",
	Intent:      "当你要把群主身份转让给他人时使用；会实际变更群主（自己不再是群主），需传群 openConversationId 和新群主的 userId 或 openDingTalkId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "new-owner", Type: shortcut.FlagString, Desc: "新群主 userId 或 openDingTalkId", Required: true},
	},
	Tips: []string{`dws chat +chat-transfer-owner --group <openConversationId> --new-owner <openDingTalkId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		newOwner := rt.Str("new-owner")
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"cid":                rt.Str("group"),
		}
		if isOpenID(newOwner) {
			params["newOwnerOpenDingTalkId"] = newOwner
		} else {
			params["newOwnerUid"] = newOwner
		}
		return rt.CallMCP("transfer_group_owner", params)
	},
}

// ChatInviteURL gets the group invite url (get_group_invite_url, im).
var ChatInviteURL = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-invite-url",
	Product:     "im",
	Description: "获取群邀请链接",
	Intent:      "当你想拿到一条群邀请链接分享给别人加群时使用；只读生成链接，需传群 openConversationId，可用 --expires-seconds 设置有效期（0 表示永久）。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "expires-seconds", Type: shortcut.FlagInt, Desc: "链接有效期（秒），0 表示永久"},
	},
	Tips: []string{`dws chat +chat-invite-url --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"cid":                rt.Str("group"),
		}
		if rt.Changed("expires-seconds") {
			params["expiresSeconds"] = rt.Int("expires-seconds")
		}
		return rt.CallMCP("get_group_invite_url", params)
	},
}

// ChatQuit quits a group (quit_group, im).
var ChatQuit = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-quit",
	Product:     "im",
	Description: "退出群聊",
	Intent:      "当你想让当前用户主动退出某个群时使用；会实际退群，需传群 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +chat-quit --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("quit_group", map[string]any{"openConversationId": rt.Str("group")})
	},
}

// ChatUpdateIcon updates the group icon (update_group_icon, im).
var ChatUpdateIcon = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-update-icon",
	Product:     "im",
	Description: "更新群头像",
	Intent:      "当你想更换群头像时使用；会实际更新群头像，需传群 openConversationId 和已上传头像的 mediaId（以 @ 开头）。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "icon-media-id", Type: shortcut.FlagString, Desc: "群头像 mediaId（以 @ 开头）", Required: true},
	},
	Tips: []string{`dws chat +chat-update-icon --group <openConversationId> --icon-media-id <mediaId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_group_icon", map[string]any{
			"openConversationId": rt.Str("group"),
			"iconMediaId":        rt.Str("icon-media-id"),
		})
	},
}

// ChatUpdateSettings updates a group setting (update_group_settings, im).
var ChatUpdateSettings = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-update-settings",
	Product:     "im",
	Description: "更新群设置（settingKey + status）",
	Intent:      "当你想调整群的某项开关设置（如是否可被搜索 searchable、是否仅管理员可@所有人 onlyAdminCanAtAll）时使用；会实际修改群设置，需传 settingKey 和 status（0关/1开）。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "setting-key", Type: shortcut.FlagString, Desc: "群设置项 key，如 searchable / onlyAdminCanAtAll", Required: true},
		{Name: "status", Type: shortcut.FlagInt, Desc: "设置值：0=关闭，1=开启", Required: true},
	},
	Tips: []string{`dws chat +chat-update-settings --group <openConversationId> --setting-key searchable --status 1`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_group_settings", map[string]any{
			"openConversationId": rt.Str("group"),
			"settingKey":         rt.Str("setting-key"),
			"status":             rt.Int("status"),
		})
	},
}

// ChatDismiss dismisses (destroys) a group (dismiss_group, im).
var ChatDismiss = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-dismiss",
	Product:     "im",
	Description: "解散群聊（不可逆，需群主权限）",
	Intent:      "当你要彻底解散一个群时使用；会实际销毁群聊，不可逆且需群主权限，仅需传群 openConversationId，操作前务必确认。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +chat-dismiss --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("dismiss_group", map[string]any{"openConversationId": rt.Str("group")})
	},
}

// ChatSetHistory sets new-member history visibility (update_show_history_msg_option, im).
var ChatSetHistory = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-set-history",
	Product:     "im",
	Description: "设置新成员入群可查看历史消息范围",
	Intent:      "当你想控制新成员入群后能看到多少历史消息时使用；会实际修改群配置，需传群 openConversationId 和范围（FORBIDDEN 不可见 / RECENT_100 最近100条 / ALL 全部）。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "option", Type: shortcut.FlagString, Desc: "可见范围", Required: true, Enum: []string{"FORBIDDEN", "RECENT_100", "ALL"}},
	},
	Tips: []string{`dws chat +chat-set-history --group <openConversationId> --option RECENT_100`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_show_history_msg_option", map[string]any{
			"openConversationId": rt.Str("group"),
			"option":             rt.Str("option"),
		})
	},
}

// ChatUpdateNick sets the caller's in-group nickname (update_group_nick, im).
var ChatUpdateNick = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-update-nick",
	Product:     "im",
	Description: "设置当前用户在群内的群昵称",
	Intent:      "当你想设置当前用户在某个群里显示的群昵称时使用；会实际更新本人在该群的昵称，需传群 openConversationId 和昵称。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "nick", Type: shortcut.FlagString, Desc: "个人群昵称", Required: true},
	},
	Tips: []string{`dws chat +chat-update-nick --group <openConversationId> --nick "我的群昵称"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_group_nick", map[string]any{
			"openConversationId": rt.Str("group"),
			"nick":               rt.Str("nick"),
		})
	},
}

// ChatUpdateAlias sets the caller's private alias for a group (update_user_group_alias, im).
var ChatUpdateAlias = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-update-alias",
	Product:     "im",
	Description: "设置群备注（仅自己可见）",
	Intent:      "当你想给某个群设置仅自己可见的备注名以便区分同名群时使用；会实际保存本人对该群的备注，需传群 openConversationId 和备注标题。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "alias-title", Type: shortcut.FlagString, Desc: "群备注标题", Required: true},
	},
	Tips: []string{`dws chat +chat-update-alias --group <openConversationId> --alias-title "项目A群"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_user_group_alias", map[string]any{
			"openConversationId": rt.Str("group"),
			"aliasTitle":         rt.Str("alias-title"),
		})
	},
}

// ChatListMine lists groups the caller owns/administers (list_owned_or_admin_groups, im).
var ChatListMine = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-list-mine",
	Product:     "im",
	Description: "拉取我创建/管理的群",
	Intent:      "当你想查看自己作为群主或管理员在管理哪些群时使用；只读分页返回，可用 --role OWNER/ADMIN 按角色过滤。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "role", Type: shortcut.FlagString, Desc: "角色过滤", Enum: []string{"OWNER", "ADMIN"}},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "最多返回群数量，不传返回全部"},
		{Name: "exclude-muted", Type: shortcut.FlagBool, Desc: "排除已设置免打扰的群聊"},
	},
	Tips: []string{`dws chat +chat-list-mine --role OWNER`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Changed("role") {
			params["roleFilter"] = rt.Str("role")
		}
		if rt.Int("limit") > 0 {
			params["limit"] = rt.Int("limit")
		}
		if rt.Bool("exclude-muted") {
			params["excludeMuted"] = true
		}
		data, err := rt.CallMCPData("im", "list_owned_or_admin_groups", params)
		if err != nil {
			return err
		}
		groups := chatListMineProject(data)
		return rt.Output(map[string]any{"count": len(groups), "groups": groups})
	},
}

// chatListMineProject reshapes list_owned_or_admin_groups into a clean group
// list ({openConversationId, name, role, ownerUserId}) — output-projection
// clean output projection. List container and per-item field names are probed
// defensively across candidate keys so shape drift yields an empty list rather
// than a crash or fabricated data.
func chatListMineProject(data map[string]any) []map[string]any {
	raw := chatGroupResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := chatGroupFirst(m, "openConversationId", "openconversation_id", "conversationId", "id"); ok {
			row["openConversationId"] = v
		}
		if v, ok := chatGroupFirst(m, "name", "groupName", "title"); ok {
			row["name"] = v
		}
		if v, ok := chatGroupFirst(m, "role", "roleType", "memberRole"); ok {
			row["role"] = v
		}
		if v, ok := chatGroupFirst(m, "ownerUserId", "ownerId", "owner"); ok {
			row["ownerUserId"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// ChatListAll paginates all groups the caller joined (list_my_groups_pagination, im).
var ChatListAll = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-list-all",
	Product:     "im",
	Description: "分页拉取我加入的所有群列表",
	Intent:      "当你想遍历当前用户加入的所有群做统计或批量操作时使用；只读分页返回全部已加入的群列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagInt, Default: "100", Desc: "每页返回数量（最大 200）"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，翻页传 nextCursor"},
	},
	Tips: []string{`dws chat +chat-list-all --limit 50`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Int("limit") > 0 {
			params["limit"] = rt.Int("limit")
		}
		if c := rt.Str("cursor"); c != "" && c != "0" {
			params["cursor"] = c
		}
		data, err := rt.CallMCPData("im", "list_my_groups_pagination", params)
		if err != nil {
			return err
		}
		groups := chatListAllProject(data)
		payload := map[string]any{"count": len(groups), "groups": groups}
		if v, ok := chatGroupFirst(data, "nextCursor", "next_cursor", "cursor"); ok {
			payload["nextCursor"] = v
		}
		if v, ok := chatGroupFirst(data, "hasMore", "has_more"); ok {
			payload["hasMore"] = v
		}
		return rt.Output(payload)
	},
}

// chatListAllProject reshapes list_my_groups_pagination into a clean group list
// ({openConversationId, name}) — clean output projection. List
// container and per-item field names are probed defensively across candidate
// keys so shape drift yields an empty list rather than a crash or fabricated
// data.
func chatListAllProject(data map[string]any) []map[string]any {
	raw := chatGroupResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := chatGroupFirst(m, "openConversationId", "openconversation_id", "conversationId", "id"); ok {
			row["openConversationId"] = v
		}
		if v, ok := chatGroupFirst(m, "name", "groupName", "title"); ok {
			row["name"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// chatGroupResolveList locates the list payload inside a group-list response,
// tolerating a bare top-level array or nesting under common envelope keys.
func chatGroupResolveList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, key := range []string{"result", "data", "list", "items", "groups", "conversations"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "groups", "conversations", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// chatGroupFirst returns the first present candidate key's value.
func chatGroupFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// ChatListJoinRequests paginates join-validation records (list_apply_join_group_records, im).
var ChatListJoinRequests = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-list-join-requests",
	Product:     "im",
	Description: "分页拉取入群验证记录",
	Intent:      "当你作为群主/管理员想查看待处理的入群申请时使用；只读分页返回入群验证记录（含 recordId、申请人与邀请人 ID），供后续用 chat-audit-join 审批。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "单页数量（最大 50）"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，翻页传 nextCursor"},
	},
	Tips: []string{`dws chat +chat-list-join-requests --limit 30`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Int("limit") > 0 {
			params["limit"] = rt.Int("limit")
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		return rt.CallMCP("list_apply_join_group_records", params)
	},
}

// ChatAuditJoin audits a join-validation record (audit_join_group, im).
var ChatAuditJoin = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-audit-join",
	Product:     "im",
	Description: "审批入群验证（通过/拒绝/删除/忽略/拉黑）",
	Intent:      "当你要处理某条入群申请时使用；会实际执行通过/拒绝/删除/忽略/拉黑动作，需传群 openConversationId、recordId、申请人与邀请人 userId 及 status。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "record-id", Type: shortcut.FlagInt, Desc: "申请记录 ID", Required: true},
		{Name: "applicant", Type: shortcut.FlagString, Desc: "申请人 userId", Required: true},
		{Name: "inviter", Type: shortcut.FlagString, Desc: "邀请人 userId", Required: true},
		{Name: "status", Type: shortcut.FlagString, Desc: "审批动作", Required: true, Enum: []string{"AuditApprove", "AuditDelete", "AuditIgnore", "AuditRefuse", "AuditBlock"}},
		{Name: "description", Type: shortcut.FlagString, Desc: "审批说明"},
	},
	Tips: []string{`dws chat +chat-audit-join --group <openConversationId> --record-id 123 --applicant <userId> --inviter <userId> --status AuditApprove`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"applyRecordId":      rt.Int("record-id"),
			"applicantUid":       rt.Str("applicant"),
			"inviterUid":         rt.Str("inviter"),
			"status":             rt.Str("status"),
		}
		if rt.Changed("description") {
			params["auditDescription"] = rt.Str("description")
		}
		return rt.CallMCP("audit_join_group", params)
	},
}

// ChatGetByID looks up a group by numeric group id (get_conv_info_by_group_id, im).
var ChatGetByID = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-get-by-id",
	Product:     "im",
	Description: "根据群号获取群聊信息",
	Intent:      "当你只知道群号（数字）、需要换取群 openConversationId 及群信息时使用；只读，需传 --group-id。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group-id", Type: shortcut.FlagInt, Desc: "群号（数字类型）", Required: true},
	},
	Tips: []string{`dws chat +chat-get-by-id --group-id 12345678`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_conv_info_by_group_id", map[string]any{"groupId": rt.Int("group-id")})
	},
}

// ChatAddBot adds a custom robot to a group (add_robot_to_group, bot).
var ChatAddBot = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-add-bot",
	Product:     "bot",
	Description: "将机器人添加到群中",
	Intent:      "当你想把某个机器人添加进群（比如让日报机器人进群播报）时使用；会实际把机器人加入群聊，需传机器人 robotCode 和群 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "robot-code", Type: shortcut.FlagString, Desc: "机器人 Code", Required: true},
		{Name: "id", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +chat-add-bot --robot-code <robotCode> --id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("add_robot_to_group", map[string]any{
			"robotCode":          rt.Str("robot-code"),
			"openConversationId": rt.Str("id"),
		})
	},
}

// ChatBots lists robots in a group (list_group_bots, bot).
var ChatBots = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-bots",
	Product:     "bot",
	Description: "查看群内所有机器人",
	Intent:      "当你想查看某个群里已添加了哪些机器人时使用；需传群 openConversationId，只读返回群内机器人列表（含 openBotId，供后续移除）。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +chat-bots --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("bot", "list_group_bots", map[string]any{"openConversationId": rt.Str("group")})
		if err != nil {
			return err
		}
		bots := chatBotsProject(data)
		return rt.Output(map[string]any{"count": len(bots), "bots": bots})
	},
}

// chatBotsProject reshapes list_group_bots into a clean bot list
// ({openBotId, name}) — clean output projection. List container and
// per-item field names are probed defensively across candidate keys so shape
// drift yields an empty list rather than a crash or fabricated data.
func chatBotsProject(data map[string]any) []map[string]any {
	raw := chatGroupResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := chatGroupFirst(m, "openBotId", "open_bot_id", "botId", "robotCode", "id"); ok {
			row["openBotId"] = v
		}
		if v, ok := chatGroupFirst(m, "name", "botName", "nick", "title"); ok {
			row["name"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// ChatRemoveBot removes a robot from a group (remove_robot_in_group, bot).
var ChatRemoveBot = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-remove-bot",
	Product:     "bot",
	Description: "从群内移除机器人",
	Intent:      "当你想把某个机器人从群里移除时使用；会实际移除机器人，不可逆，需传群 openConversationId 和机器人 openBotId。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "bot-id", Type: shortcut.FlagString, Desc: "机器人 openBotId", Required: true},
	},
	Tips: []string{`dws chat +chat-remove-bot --id <openConversationId> --bot-id <openBotId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("remove_robot_in_group", map[string]any{
			"openConversationId": rt.Str("id"),
			"openBotId":          rt.Str("bot-id"),
		})
	},
}

// ChatSetAdmin sets/unsets group admins (update_conv_member_roles, im).
var ChatSetAdmin = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-set-admin",
	Product:     "im",
	Description: "设置 / 取消群管理员",
	Intent:      "当你想把某些成员设为或取消群管理员时使用；会实际变更成员角色，需传群 openConversationId 和成员 userId/openDingTalkId 列表，加 --off 取消管理员。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "成员 userId 或 openDingTalkId 列表", Required: true},
		{Name: "off", Type: shortcut.FlagBool, Desc: "取消管理员（不传则设为管理员）"},
	},
	Tips: []string{`dws chat +chat-set-admin --group <openConversationId> --users userId1,userId2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		userIDs, openIDs := splitIDs(rt.StrSlice("users"))
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"admin":              !rt.Bool("off"),
		}
		if len(userIDs) > 0 {
			params["uids"] = userIDs
		}
		if len(openIDs) > 0 {
			params["openDingTalkIds"] = openIDs
		}
		return rt.CallMCP("update_conv_member_roles", params)
	},
}

// ChatMute mutes/unmutes the whole group (set_group_mute, im).
var ChatMute = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-mute",
	Product:     "im",
	Description: "全员禁言 / 取消全员禁言",
	Intent:      "当你想对整个群开启或取消全员禁言时使用；会实际切换群的全员禁言状态，需传群 openConversationId，加 --off 取消禁言。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "off", Type: shortcut.FlagBool, Desc: "取消全员禁言（不传则开启禁言）"},
	},
	Tips: []string{`dws chat +chat-mute --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("set_group_mute", map[string]any{
			"openConversationId": rt.Str("group"),
			"mute":               !rt.Bool("off"),
		})
	},
}

// ChatMuteMember mutes/unmutes specific members (set_group_member_mute_list, im).
var ChatMuteMember = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-mute-member",
	Product:     "im",
	Description: "指定群成员禁言 / 取消禁言",
	Intent:      "当你想只禁言或解禁群里的指定成员时使用；会实际把成员加入或移出禁言名单，需传群 openConversationId 和成员列表，禁言时还需 --mute-time（毫秒）。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "成员 userId 或 openDingTalkId 列表", Required: true},
		{Name: "mute-time", Type: shortcut.FlagInt, Desc: "禁言时长（毫秒），如 300000/3600000/86400000/604800000/2592000000"},
		{Name: "off", Type: shortcut.FlagBool, Desc: "移出禁言名单（不传则加入禁言名单）"},
	},
	Tips: []string{`dws chat +chat-mute-member --group <openConversationId> --users userId1 --mute-time 3600000`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		userIDs, openIDs := splitIDs(rt.StrSlice("users"))
		off := rt.Bool("off")
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"cid":                rt.Str("group"),
			"mute":               !off,
		}
		if len(userIDs) > 0 {
			params["uids"] = userIDs
		}
		if len(openIDs) > 0 {
			params["openDingTalkIds"] = openIDs
		}
		if !off {
			if rt.Int("mute-time") <= 0 {
				return fmt.Errorf("--mute-time 为禁言时必填（毫秒）")
			}
			params["muteTime"] = rt.Int("mute-time")
		}
		return rt.CallMCP("set_group_member_mute_list", params)
	},
}

// ── group-role: 群身份管理 (im) ──────────────────────────────

// ChatRoleList lists custom group roles (list_custom_group_roles, im).
var ChatRoleList = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-list",
	Product:     "im",
	Description: "拉取会话的群身份列表",
	Intent:      "当你想查看某群自定义的群身份（如'班长''值日'）都有哪些时使用；需传群 openConversationId，只读返回群身份列表及 openRoleId。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +chat-role-list --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("im", "list_custom_group_roles", map[string]any{"openConversationId": rt.Str("group")})
		if err != nil {
			return err
		}
		roles := chatRoleListProject(data)
		return rt.Output(map[string]any{"count": len(roles), "roles": roles})
	},
}

// chatRoleListProject reshapes list_custom_group_roles into a clean group-role
// list ({openRoleId, name}) — clean output projection. List
// container and per-item field names are probed defensively across candidate
// keys so shape drift yields an empty list rather than a crash or fabricated
// data.
func chatRoleListProject(data map[string]any) []map[string]any {
	raw := chatGroupResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := chatGroupFirst(m, "openRoleId", "open_role_id", "roleId", "id"); ok {
			row["openRoleId"] = v
		}
		if v, ok := chatGroupFirst(m, "name", "roleName", "title"); ok {
			row["name"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// ChatRoleAdd adds a custom group role (add_custom_group_role, im).
var ChatRoleAdd = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-add",
	Product:     "im",
	Description: "添加群身份",
	Intent:      "当你想在群里新增一个自定义群身份/头衔时使用；会实际创建群身份，需传群 openConversationId 和身份名称。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "群身份名称", Required: true},
	},
	Tips: []string{`dws chat +chat-role-add --group <openConversationId> --name "管理员"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("add_custom_group_role", map[string]any{
			"openConversationId": rt.Str("group"),
			"name":               rt.Str("name"),
		})
	},
}

// ChatRoleUpdate renames a custom group role (update_custom_group_role, im).
var ChatRoleUpdate = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-update",
	Product:     "im",
	Description: "更新群身份名称",
	Intent:      "当你想重命名已有的群身份时使用；会实际更新身份名称，需传群 openConversationId、身份 openRoleId 和新名称。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "role-id", Type: shortcut.FlagString, Desc: "群身份 openRoleId", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "群身份新名称", Required: true},
	},
	Tips: []string{`dws chat +chat-role-update --group <openConversationId> --role-id <openRoleId> --name "新名称"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_custom_group_role", map[string]any{
			"openConversationId": rt.Str("group"),
			"openRoleId":         rt.Str("role-id"),
			"name":               rt.Str("name"),
		})
	},
}

// ChatRoleRemove deletes a custom group role (remove_custom_group_role, im).
var ChatRoleRemove = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-remove",
	Product:     "im",
	Description: "删除群身份",
	Intent:      "当你想删除某个自定义群身份时使用；会实际删除群身份，不可逆，需传群 openConversationId 和身份 openRoleId。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "role-id", Type: shortcut.FlagString, Desc: "群身份 openRoleId", Required: true},
	},
	Tips: []string{`dws chat +chat-role-remove --group <openConversationId> --role-id <openRoleId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("remove_custom_group_role", map[string]any{
			"openConversationId": rt.Str("group"),
			"openRoleId":         rt.Str("role-id"),
		})
	},
}

// ChatRoleSetUser overwrites a user's group roles (set_custom_user_roles, im).
var ChatRoleSetUser = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-set-user",
	Product:     "im",
	Description: "设置用户的群身份（覆盖该用户的全部群身份）",
	Intent:      "当你想为某成员整体设定其在群内的身份时使用；会实际改写该用户的群身份集合（覆盖其原有全部身份），需传群、用户和 openRoleId 列表（传空则清除全部）。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "user", Type: shortcut.FlagString, Desc: "用户 userId 或 openDingTalkId", Required: true},
		{Name: "role-ids", Type: shortcut.FlagStringSlice, Desc: "群身份 openRoleId 列表（空则清除全部）", Required: true},
	},
	Tips: []string{`dws chat +chat-role-set-user --group <openConversationId> --user <userId> --role-ids roleId1,roleId2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		user := rt.Str("user")
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"openRoleIds":        rt.StrSlice("role-ids"),
		}
		if isOpenID(user) {
			params["openDingTalkId"] = user
		} else {
			params["userId"] = user
		}
		return rt.CallMCP("set_custom_user_roles", params)
	},
}

// ChatRoleRemoveUser removes specific roles from a user (remove_custom_user_roles, im).
var ChatRoleRemoveUser = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-remove-user",
	Product:     "im",
	Description: "移除用户的指定群身份",
	Intent:      "当你只想撤销某成员的部分群身份、保留其余时使用；会实际移除指定的群身份，需传群、用户和要移除的 openRoleId 列表。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "user", Type: shortcut.FlagString, Desc: "用户 userId 或 openDingTalkId", Required: true},
		{Name: "role-ids", Type: shortcut.FlagStringSlice, Desc: "要移除的群身份 openRoleId 列表", Required: true},
	},
	Tips: []string{`dws chat +chat-role-remove-user --group <openConversationId> --user <userId> --role-ids roleId1`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		user := rt.Str("user")
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"openRoleIds":        rt.StrSlice("role-ids"),
		}
		if isOpenID(user) {
			params["openDingTalkId"] = user
		} else {
			params["userId"] = user
		}
		return rt.CallMCP("remove_custom_user_roles", params)
	},
}

// ChatRoleQueryUser queries a member's group roles (query_custom_user_roles, im).
var ChatRoleQueryUser = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-query-user",
	Product:     "im",
	Description: "查询群成员的群身份",
	Intent:      "当你想查看某个群成员当前拥有哪些群身份时使用；只读，需传群 openConversationId 和用户 userId 或 openDingTalkId。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "user", Type: shortcut.FlagString, Desc: "用户 userId 或 openDingTalkId", Required: true},
	},
	Tips: []string{`dws chat +chat-role-query-user --group <openConversationId> --user <userId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		user := rt.Str("user")
		params := map[string]any{"openConversationId": rt.Str("group")}
		if isOpenID(user) {
			params["openDingTalkId"] = user
		} else {
			params["userId"] = user
		}
		return rt.CallMCP("query_custom_user_roles", params)
	},
}

func init() {
	shortcut.Register(
		ChatSearch,
		ChatMembersGet,
		ChatTransferOwner,
		ChatInviteURL,
		ChatQuit,
		ChatUpdateIcon,
		ChatUpdateSettings,
		ChatDismiss,
		ChatSetHistory,
		ChatUpdateNick,
		ChatUpdateAlias,
		ChatListMine,
		ChatListAll,
		ChatListJoinRequests,
		ChatAuditJoin,
		ChatGetByID,
		ChatAddBot,
		ChatBots,
		ChatRemoveBot,
		ChatSetAdmin,
		ChatMute,
		ChatMuteMember,
		ChatRoleList,
		ChatRoleAdd,
		ChatRoleUpdate,
		ChatRoleRemove,
		ChatRoleSetUser,
		ChatRoleRemoveUser,
		ChatRoleQueryUser,
	)
}
