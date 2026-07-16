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

// Package contact provides declarative shortcuts for the DingTalk contact
// (通讯录) service: user / department / role / relation queries and the HR
// roster (花名册) lookups. Each shortcut maps 1:1 onto an MCP tool declared in
// internal/helpers/contact.go.
package contact

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"

// GetSelf 获取当前登录用户信息（我是谁 / 本人）。
// ListFollowings 获取当前用户的特别关注列表。
var ListFollowings = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+list-followings",
	Product:     "contact",
	Description: "获取当前用户的特别关注列表",
	Intent:      "当你想查看本人在通讯录里「特别关注」的联系人名单（例如常打交道的同事、上级）时使用；无需输入，返回关注对象的用户列表，可用于快速定位这些人的 userId 再发消息或排日程。",
	Risk:        shortcut.RiskRead,
	Tips: []string{
		`dws contact +list-followings`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Project the raw {arguments,result:{models:[…]}} envelope down to a
		// clean {count, followings:[{openDingTalkId}]} list.
		data, err := rt.CallMCPData("contact", "list_my_followings", nil)
		if err != nil {
			return err
		}
		followings := listFollowingsProject(data)
		return rt.Output(map[string]any{"count": len(followings), "followings": followings})
	},
}

// listFollowingsProject reshapes a list_my_followings response into a clean
// list, unwrapping the result.models container the gateway uses.
func listFollowingsProject(data map[string]any) []map[string]any {
	var raw []any
	if res, ok := data["result"].(map[string]any); ok {
		raw, _ = res["models"].([]any)
	}
	if raw == nil {
		for _, k := range []string{"models", "list", "items"} {
			if arr, ok := data[k].([]any); ok {
				raw = arr
				break
			}
		}
	}
	out := make([]map[string]any, 0, len(raw))
	for _, it := range raw {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		for _, k := range []string{"openDingTalkId", "openDingtalkId", "userId", "name"} {
			if v, ok := m[k]; ok && v != nil {
				row[k] = v
			}
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// SearchUser 按关键词搜索通讯录用户。
var SearchUser = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+search-user",
	Product:     "contact",
	Description: "按关键词搜索通讯录用户",
	Intent:      "当你只知道某人的姓名、花名或部分名字，需要把它解析成 userId 及部门等信息以便后续发消息、排日程或指派任务时使用；输入搜索关键词（--query），返回匹配的用户列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词", Required: true},
	},
	Tips: []string{
		`dws contact +search-user --query "张三"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("contact", "search_contact_by_key_word", map[string]any{
			"keyword": rt.Str("query"),
		})
		if err != nil {
			return err
		}
		users := searchUserProject(data)
		return rt.Output(map[string]any{"count": len(users), "users": users})
	},
}

// searchUserProject reshapes the raw search_contact_by_key_word response into a
// clean, stable user list (name/userId/flowerName/openDingTalkId/title) — the
// the clean output projection applied to every list command.
// Field names are probed defensively across candidate keys.
func searchUserProject(data map[string]any) []map[string]any {
	raw, ok := data["result"].([]any)
	if !ok {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		for _, k := range []string{"name", "userId", "flowerName", "openDingTalkId", "title"} {
			if v, ok := m[k]; ok {
				row[k] = v
			}
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// SearchMobile 按手机号搜索通讯录用户。
var SearchMobile = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+search-mobile",
	Product:     "contact",
	Description: "按手机号搜索通讯录用户",
	Intent:      "当你手里只有某人的手机号、需要反查出对应的通讯录用户和 userId 时使用；输入手机号（--mobile），返回该手机号所属的用户信息，适合从电话或名片信息定位到具体员工。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "mobile", Type: shortcut.FlagString, Desc: "手机号", Required: true},
	},
	Tips: []string{
		`dws contact +search-mobile --mobile 13800138000`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("contact", "search_user_by_mobile", map[string]any{
			"mobile": rt.Str("mobile"),
		})
		if err != nil {
			return err
		}
		users := searchUserProject(data)
		return rt.Output(map[string]any{"count": len(users), "users": users})
	},
}

// GetUser 批量获取用户详情（组织管理信息：部门、主管、管理员权限）。
// ListRoles 获取企业所有角色（标签）列表。
var ListRoles = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+list-roles",
	Product:     "contact",
	Description: "获取企业所有角色（标签）列表",
	Intent:      "当你想总览企业里都有哪些角色/员工标签（如「管理员」「财务」「销售」）及其角色 ID 时使用；无需输入，返回全量角色列表，常用于按角色圈定人群前先摸清有哪些角色可选。",
	Risk:        shortcut.RiskRead,
	Tips: []string{
		`dws contact +list-roles`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("contact", "get_org_labels", map[string]any{})
		if err != nil {
			return err
		}
		roles := listRolesProject(data)
		return rt.Output(map[string]any{"count": len(roles), "roles": roles})
	},
}

// listRolesProject reshapes the raw get_org_labels response into a clean
// role/label list ({labelId, labelName}) — clean output projection.
// The list container and field names are probed defensively across candidate
// keys so the projection tolerates response-shape drift.
func listRolesProject(data map[string]any) []map[string]any {
	raw := listRolesResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := listRolesFirst(m, "labelId", "label_id", "id"); ok {
			row["labelId"] = v
		}
		if v, ok := listRolesFirst(m, "labelName", "label_name", "name"); ok {
			row["labelName"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// listRolesResolveList locates the list payload inside the response, tolerating
// a bare top-level array or nesting under result/data/list/items containers.
func listRolesResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "labels"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		// container may itself wrap the list one level deeper
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "labels", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// listRolesFirst returns the first present candidate key's value.
func listRolesFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// SearchRole 根据角色名称精确匹配查询角色（角色ID、名称）。
// ListRoleMembers 根据角色 ID 查询该角色下的成员列表。
var ListRoleMembers = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+list-role-members",
	Product:     "contact",
	Description: "查询角色下的成员列表",
	Intent:      "当你已知某个角色 ID、想列出该角色（标签）下的全部成员以便群发通知或统计人群时使用；输入角色 ID（--id），返回该角色下的用户列表，通常先用 +search-role 拿到角色 ID 再调用。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "角色 ID", Required: true},
	},
	Tips: []string{
		`dws contact +list-role-members --id 12345`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("contact", "get_label_members_by_labelId", map[string]any{
			"labelId": rt.Str("id"),
		})
		if err != nil {
			return err
		}
		members := memberListProject(data)
		return rt.Output(map[string]any{"count": len(members), "members": members})
	},
}

// memberListProject reshapes a user/member list response into a clean
// {userId, name} list — clean output projection. Both the list
// container and the per-item field names are probed defensively across
// candidate keys, so an empty/unknown shape yields an empty list rather than a
// crash or fabricated data. Shared by role-member and dept-member listings.
func memberListProject(data map[string]any) []map[string]any {
	if data == nil {
		return []map[string]any{}
	}
	raw := memberListFindList(data)
	if raw == nil {
		for _, container := range []string{"result", "data"} {
			if inner, ok := data[container].(map[string]any); ok {
				if r := memberListFindList(inner); r != nil {
					raw = r
					break
				}
			}
		}
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v := memberListFirst(m, "userId", "user_id", "userid", "id"); v != nil {
			row["userId"] = v
		}
		if v := memberListFirst(m, "name", "userName", "user_name", "flowerName"); v != nil {
			row["name"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// memberListFindList returns the first slice found under the common list
// container keys, or nil when none is present.
func memberListFindList(m map[string]any) []any {
	for _, k := range []string{"result", "data", "list", "items", "members", "userList"} {
		if arr, ok := m[k].([]any); ok {
			return arr
		}
	}
	return nil
}

// memberListFirst returns the value of the first present key among keys.
func memberListFirst(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// SearchDept 按关键词搜索部门。
// ListSubDepts 查看指定部门的子部门。
var ListSubDepts = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+list-sub-depts",
	Product:     "contact",
	Description: "查看指定部门的子部门",
	Intent:      "当你想逐层浏览组织架构、查看某个部门下一级的子部门时使用；输入父部门 ID（--dept，根部门为 1），返回其直属子部门列表，可用于自顶向下遍历部门树。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "dept", Type: shortcut.FlagInt, Desc: "部门 ID（钉钉根部门为 1）", Required: true},
	},
	Tips: []string{
		`dws contact +list-sub-depts --dept 1`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("contact", "get_sub_depts_by_dept_id", map[string]any{
			"deptId": rt.Int("dept"),
		})
		if err != nil {
			return err
		}
		depts := listSubDeptsProject(data)
		return rt.Output(map[string]any{"count": len(depts), "depts": depts})
	},
}

// listSubDeptsProject reshapes get_sub_depts_by_dept_id into a clean
// {deptId, deptName} list — clean output projection. Both the list
// container and the per-item field names are probed defensively across
// candidate keys, so an empty/unknown shape yields an empty list rather than a
// crash or fabricated data.
func listSubDeptsProject(data map[string]any) []map[string]any {
	if data == nil {
		return []map[string]any{}
	}
	// Locate the list container: it may sit at the top level or be nested one
	// level under a common envelope key.
	raw := listSubDeptsFindList(data)
	if raw == nil {
		for _, container := range []string{"result", "data"} {
			if inner, ok := data[container].(map[string]any); ok {
				if r := listSubDeptsFindList(inner); r != nil {
					raw = r
					break
				}
			}
		}
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v := listSubDeptsFirst(m, "deptId", "dept_id", "id"); v != nil {
			row["deptId"] = v
		}
		if v := listSubDeptsFirst(m, "deptName", "dept_name", "name"); v != nil {
			row["deptName"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// listSubDeptsFindList returns the first slice found under the common list
// container keys, or nil when none is present.
func listSubDeptsFindList(m map[string]any) []any {
	for _, k := range []string{"result", "data", "list", "items"} {
		if arr, ok := m[k].([]any); ok {
			return arr
		}
	}
	return nil
}

// listSubDeptsFirst returns the value of the first present key among keys.
func listSubDeptsFirst(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// GetDept 获取部门详情（部门 ID、名称、人数）。
// ListDeptMembers 查看部门成员（仅本部门，不含下级）。
var ListDeptMembers = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+list-dept-members",
	Product:     "contact",
	Description: "查看部门成员（仅本部门，不含下级）",
	Intent:      "当你想列出一个或多个部门本级的员工（不含下级子部门）以便群发通知、统计或指派任务时使用；输入部门 ID 列表（--depts，逗号分隔），返回这些部门下的成员，如需含下级需自行遍历子部门。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "depts", Type: shortcut.FlagStringSlice, Desc: "部门 ID 列表，逗号分隔", Required: true},
	},
	Tips: []string{
		`dws contact +list-dept-members --depts 12345,67890`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("contact", "get_dept_members_by_deptId", map[string]any{
			"deptIds": rt.StrSlice("depts"),
		})
		if err != nil {
			return err
		}
		members := memberListProject(data)
		return rt.Output(map[string]any{"count": len(members), "members": members})
	},
}

// ListRosterFields 查询花名册当前用户有权限的字段列表（hrmregister server）。
var ListRosterFields = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+list-roster-fields",
	Product:     "hrmregister",
	Description: "查询花名册有权限的字段列表",
	Intent:      "当你要查询花名册（HR 档案）信息、需要先知道当前身份有权访问哪些字段及其字段编码（fieldCode）时使用；无需输入，返回可用字段列表，通常作为调用 +get-roster 前的准备步骤以指定 --fields。",
	Risk:        shortcut.RiskRead,
	Tips: []string{
		`dws contact +list-roster-fields`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("list_authorized_roster_fields", map[string]any{})
	},
}

// GetRoster 查询员工花名册字段信息（个人档案，hrmregister server）。
var GetRoster = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+get-roster",
	Product:     "hrmregister",
	Description: "查询员工花名册字段信息（学历、家庭、银行卡、合同等）",
	Intent:      "当你需要查看某员工在 HR 花名册中的详细档案字段（如学历、家庭、银行卡、合同等）时使用；可传员工 ID（--staff-id）和要查的字段编码（--fields，来自 +list-roster-fields），不传则按默认查询，返回授权范围内的花名册信息。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "staff-id", Type: shortcut.FlagString, Desc: "查询员工 ID（可选）"},
		{Name: "fields", Type: shortcut.FlagStringSlice, Desc: "指定字段集合，逗号分隔，可通过 +list-roster-fields 获取（可选）"},
	},
	Tips: []string{
		`dws contact +get-roster --staff-id STAFF_ID`,
		`dws contact +get-roster --staff-id STAFF_ID --fields fieldCode1,fieldCode2`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Changed("staff-id") {
			params["staffId"] = rt.Str("staff-id")
		}
		if rt.Changed("fields") {
			params["fieldCodeList"] = rt.StrSlice("fields")
		}
		return rt.CallMCP("get_authorized_emp_rosterInfo", params)
	},
}

func init() {
	shortcut.Register(
		ListFollowings,
		SearchUser,
		SearchMobile,
		ListRoles,
		ListRoleMembers,
		ListSubDepts,
		ListDeptMembers,
		ListRosterFields,
		GetRoster,
	)
}
