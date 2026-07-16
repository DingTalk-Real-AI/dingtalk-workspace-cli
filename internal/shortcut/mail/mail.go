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

// Package mail provides declarative shortcuts for the DingTalk mail (邮箱) service:
// mailbox / message / draft / thread / folder / tag / user / attachment / template /
// contact / auto-reply / rule operations. Each shortcut maps 1:1 onto an MCP tool
// declared in internal/helpers/mail.go.
package mail

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"

// ── mailbox ────────────────────────────────────────────────

// MailboxList 查询当前用户可用的邮箱地址。
// ── message ────────────────────────────────────────────────

// Search 使用 KQL 查询表达式搜索邮件。
// Messages 列出指定文件夹中的邮件（底层按 folderId 查询）。
// Message 根据邮件 ID 获取邮件完整内容（含正文）。
// MessageVerify 根据 internetMessageId 查询邮件发送状态。
// Send 发送一封邮件到指定收件人。
// MessageMove 将多封邮件批量移动到目标文件夹。
// MessageDelete 批量删除指定邮件（移入已删除文件夹或永久删除）。
// MessageModify 批量修改邮件状态（标记已读/未读/添加标签/移除标签）。
// ── draft ──────────────────────────────────────────────────

// DraftCreate 创建一封邮件草稿并保存到草稿箱。
// DraftEdit 更新草稿箱中已有草稿的内容。
// DraftSend 将草稿箱中已有的草稿发送出去。
// ── thread ─────────────────────────────────────────────────

// ThreadList 列出指定邮箱文件夹下的邮件会话。
var ThreadList = shortcut.Shortcut{
	Service:     "mail",
	Command:     "+thread-list",
	Product:     "mail",
	Description: "列出指定邮箱文件夹下的邮件会话（thread）",
	Intent:      "当你想按会话（同一往来主题的邮件串）而非单封邮件来浏览某个文件夹时使用；传入邮箱和文件夹 ID，可按时间范围和升降序筛选，返回会话列表及其 conversationId，供 +thread 查看详情。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "email", Type: shortcut.FlagString, Desc: "会话所属邮箱地址", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "邮件文件夹 ID（不是文件夹名称）", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "本次列出的会话数，最大 100"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，首次请求可不传"},
		{Name: "start", Type: shortcut.FlagString, Desc: "开始 UTC 时间，如 2024-01-01T00:00:00Z"},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束 UTC 时间，如 2024-12-31T23:59:59Z"},
		{Name: "ascending", Type: shortcut.FlagBool, Desc: "是否按时间升序"},
	},
	Tips: []string{
		`dws mail +thread-list --email user@company.com --folder 104 --limit 20`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"email":    rt.Str("email"),
			"folderId": rt.Str("folder"),
			"size":     rt.Int("limit"),
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		if rt.Changed("start") {
			params["startTime"] = rt.Str("start")
		}
		if rt.Changed("end") {
			params["endTime"] = rt.Str("end")
		}
		if rt.Changed("ascending") {
			params["isAscending"] = rt.Bool("ascending")
		}
		data, err := rt.CallMCPData("mail", "list_mailbox_threads", params)
		if err != nil {
			return err
		}
		threads := threadListProject(data)
		return rt.Output(map[string]any{"count": len(threads), "threads": threads})
	},
}

// threadListProject reshapes the raw list_mailbox_threads response into a clean
// {conversationId, subject, lastUpdated, isRead} thread list —
// clean output projection. Both the list container and per-item
// field names are probed defensively across candidate keys, so an empty/unknown
// shape yields an empty list rather than a crash or fabricated data.
func threadListProject(data map[string]any) []map[string]any {
	raw := threadListResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := threadListFirst(m, "conversationId", "conversation_id", "id", "threadId"); ok {
			row["conversationId"] = v
		}
		if v, ok := threadListFirst(m, "subject", "title", "topic"); ok {
			row["subject"] = v
		}
		if v, ok := threadListFirst(m, "lastUpdated", "last_updated", "updateTime", "modifiedTime", "sentDateTime"); ok {
			row["lastUpdated"] = v
		}
		if v, ok := threadListFirst(m, "isRead", "is_read", "read", "unread"); ok {
			row["isRead"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// threadListResolveList locates the list payload inside the response, tolerating
// a bare top-level array container or nesting one level deeper.
func threadListResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "threads", "conversations"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "threads", "conversations", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// threadListFirst returns the first present candidate key's value.
func threadListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// Thread 根据会话 ID 获取会话详情。
// ThreadUpdate 修改单个邮件会话的状态或标签。
// ThreadBatchUpdate 批量修改邮件会话的状态或标签。
// ThreadTrash 删除指定邮件会话（移入已删除文件夹，不可撤销）。
// ThreadBatchTrash 批量删除指定邮件会话（不可撤销）。
// ── folder ─────────────────────────────────────────────────

// FolderList 列举邮件文件夹。
var FolderList = shortcut.Shortcut{
	Service:     "mail",
	Command:     "+folder-list",
	Product:     "mail",
	Description: "列出顶层文件夹或指定父文件夹下的子文件夹",
	Intent:      "当你需要了解某个邮箱有哪些文件夹、或要取得文件夹 ID 以便移动邮件、按文件夹列信/建规则时使用；传入邮箱（可选父文件夹 ID 查子级），返回文件夹列表及其 ID。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "email", Type: shortcut.FlagString, Desc: "邮件所属邮箱地址", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "父文件夹 ID，不传则返回顶层文件夹"},
	},
	Tips: []string{
		`dws mail +folder-list --email user@company.com`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"email": rt.Str("email"),
		}
		if rt.Changed("folder") {
			params["folderId"] = rt.Str("folder")
		}
		data, err := rt.CallMCPData("mail", "list_folders", params)
		if err != nil {
			return err
		}
		folders := folderListProject(data)
		return rt.Output(map[string]any{"count": len(folders), "folders": folders})
	},
}

// folderListProject reshapes the raw list_folders response into a clean
// {id, name, parentId} folder list — clean output projection. Both
// the list container and per-item field names are probed defensively across
// candidate keys, so an empty/unknown shape yields an empty list rather than a
// crash or fabricated data.
func folderListProject(data map[string]any) []map[string]any {
	raw := folderListResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := folderListFirst(m, "id", "folderId", "folder_id"); ok {
			row["id"] = v
		}
		if v, ok := folderListFirst(m, "name", "folderName", "folder_name", "displayName"); ok {
			row["name"] = v
		}
		if v, ok := folderListFirst(m, "parentId", "parent_id", "parentFolderId"); ok {
			row["parentId"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// folderListResolveList locates the list payload inside the response, tolerating
// a bare top-level array container or nesting one level deeper.
func folderListResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "folders"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "folders", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// folderListFirst returns the first present candidate key's value.
func folderListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// FolderCreate 创建邮件文件夹。
// FolderUpdate 更新邮件文件夹名称。
// FolderDelete 删除邮件文件夹。
// ── tag ────────────────────────────────────────────────────

// TagList 列举邮件标签。
var TagList = shortcut.Shortcut{
	Service:     "mail",
	Command:     "+tag-list",
	Product:     "mail",
	Description: "列出指定邮箱下的所有邮件标签",
	Intent:      "当你要查看邮箱里有哪些标签、或需要取得标签 ID 以便给邮件/会话加标签时使用；传入邮箱地址，返回全部邮件标签及其 ID。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "email", Type: shortcut.FlagString, Desc: "用户的邮箱地址", Required: true},
	},
	Tips: []string{
		`dws mail +tag-list --email user@company.com`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("mail", "list_tags", map[string]any{
			"email": rt.Str("email"),
		})
		if err != nil {
			return err
		}
		tags := tagListProject(data)
		return rt.Output(map[string]any{"count": len(tags), "tags": tags})
	},
}

// tagListProject reshapes the raw list_tags response into a clean
// {id, name, parentId} tag list — clean output projection. Both the
// list container and per-item field names are probed defensively across
// candidate keys, so an empty/unknown shape yields an empty list rather than a
// crash or fabricated data.
func tagListProject(data map[string]any) []map[string]any {
	raw := tagListResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := tagListFirst(m, "id", "tagId", "tag_id"); ok {
			row["id"] = v
		}
		if v, ok := tagListFirst(m, "name", "tagName", "tag_name", "displayName"); ok {
			row["name"] = v
		}
		if v, ok := tagListFirst(m, "parentId", "parent_id"); ok {
			row["parentId"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// tagListResolveList locates the list payload inside the response, tolerating a
// bare top-level array container or nesting one level deeper.
func tagListResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "tags"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "tags", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// tagListFirst returns the first present candidate key's value.
func tagListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// TagCreate 创建邮件标签。
// TagUpdate 更新邮件标签名称。
// TagDelete 删除邮件标签。
// ── user ───────────────────────────────────────────────────

// UserSearch 按关键词或工号搜索邮箱用户。
var UserSearch = shortcut.Shortcut{
	Service:     "mail",
	Command:     "+user-search",
	Product:     "mail",
	Description: "按关键词或工号搜索邮箱用户（仅企业邮箱）",
	Intent:      "当你只知道同事的姓名或工号、需要查出其企业邮箱地址以便发信或添加联系人时使用；提供关键词或工号（至少其一），返回匹配的企业邮箱用户列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "keyword", Type: shortcut.FlagString, Desc: "搜索关键词（未提供 --employee-no 时为必填）"},
		{Name: "employee-no", Type: shortcut.FlagString, Desc: "按工号精确搜索"},
		{Name: "email", Type: shortcut.FlagString, Desc: "搜索目标邮箱地址"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，取自响应中的 nextCursor"},
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页返回数量"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"keyword", "employee-no"}},
	},
	Tips: []string{
		`dws mail +user-search --keyword "张三"`,
		`dws mail +user-search --email user@company.com --employee-no "E123456"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Str("keyword") != "" {
			params["keyword"] = rt.Str("keyword")
		}
		if rt.Str("employee-no") != "" {
			params["employeeNo"] = rt.Str("employee-no")
		}
		if rt.Changed("email") {
			params["email"] = rt.Str("email")
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		if rt.Changed("limit") {
			params["size"] = rt.Str("limit")
		}
		data, err := rt.CallMCPData("mail", "search_mail_users", params)
		if err != nil {
			return err
		}
		users := userSearchProject(data)
		return rt.Output(map[string]any{"count": len(users), "users": users})
	},
}

// userSearchProject reshapes the raw search_mail_users response into a clean
// {name, email, employeeNo, userId} user list — clean output projection.
// Both the list container and per-item field names are probed defensively
// across candidate keys, so an empty/unknown shape yields an empty list rather
// than a crash or fabricated data.
func userSearchProject(data map[string]any) []map[string]any {
	raw := userSearchResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := userSearchFirst(m, "name", "userName", "displayName", "nickName"); ok {
			row["name"] = v
		}
		if v, ok := userSearchFirst(m, "email", "mail", "emailAddress"); ok {
			row["email"] = v
		}
		if v, ok := userSearchFirst(m, "employeeNo", "employee_no", "employeeNumber", "jobNumber"); ok {
			row["employeeNo"] = v
		}
		if v, ok := userSearchFirst(m, "userId", "user_id", "id"); ok {
			row["userId"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// userSearchResolveList locates the list payload inside the response, tolerating
// a bare top-level array container or nesting one level deeper.
func userSearchResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "users"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "users", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// userSearchFirst returns the first present candidate key's value.
func userSearchFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// ── attachment ─────────────────────────────────────────────

// AttachmentList 列举邮件附件。
// ── template ───────────────────────────────────────────────

// TemplateCreate 创建邮件模板。
// TemplateList 列举邮件模板。
var TemplateList = shortcut.Shortcut{
	Service:     "mail",
	Command:     "+template-list",
	Product:     "mail",
	Description: "列出指定邮箱的所有邮件模板",
	Intent:      "当你想查看某个邮箱下已有哪些邮件模板、或需要取得模板 ID 以便查看详情或更新时使用；传入邮箱和每页数量，返回模板列表，支持分页。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "email", Type: shortcut.FlagString, Desc: "用户邮箱地址", Required: true},
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页返回数量", Required: true},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，取自响应中的 nextCursor"},
	},
	Tips: []string{
		`dws mail +template-list --email user@company.com --limit 20`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"email": rt.Str("email"),
			"size":  rt.Str("limit"),
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("mail", "list_user_message_templates", params)
		if err != nil {
			return err
		}
		templates := templateListProject(data)
		return rt.Output(map[string]any{"count": len(templates), "templates": templates})
	},
}

// templateListProject reshapes the raw list_user_message_templates response into
// a clean {id, name, subject} template list — clean output projection.
// Both the list container and per-item field names are probed defensively
// across candidate keys, so an empty/unknown shape yields an empty list rather
// than a crash or fabricated data.
func templateListProject(data map[string]any) []map[string]any {
	raw := templateListResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := templateListFirst(m, "id", "templateId", "template_id"); ok {
			row["id"] = v
		}
		if v, ok := templateListFirst(m, "name", "templateName", "template_name", "displayName"); ok {
			row["name"] = v
		}
		if v, ok := templateListFirst(m, "subject", "title"); ok {
			row["subject"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// templateListResolveList locates the list payload inside the response,
// tolerating a bare top-level array container or nesting one level deeper.
func templateListResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "templates"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "templates", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// templateListFirst returns the first present candidate key's value.
func templateListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// TemplateGet 获取邮件模板详情。
// TemplateUpdate 更新邮件模板。
// TemplateDelete 删除邮件模板。
// ── contact ────────────────────────────────────────────────

// ContactCreate 创建邮件联系人。
// ContactList 列举邮件联系人。
var ContactList = shortcut.Shortcut{
	Service:     "mail",
	Command:     "+contact-list",
	Product:     "mail",
	Description: "列出指定邮箱的所有邮件联系人",
	Intent:      "当你想查看某邮箱通讯录里有哪些联系人、或需要取得联系人 ID 以便更新或删除时使用；传入邮箱和每页数量，返回联系人列表，支持分页。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "email", Type: shortcut.FlagString, Desc: "用户邮箱地址", Required: true},
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页返回数量", Required: true},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，取自响应中的 nextCursor"},
	},
	Tips: []string{
		`dws mail +contact-list --email user@company.com --limit 20`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"email": rt.Str("email"),
			"size":  rt.Str("limit"),
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("mail", "list_user_mail_contacts", params)
		if err != nil {
			return err
		}
		contacts := contactListProject(data)
		return rt.Output(map[string]any{"count": len(contacts), "contacts": contacts})
	},
}

// contactListProject reshapes the raw list_user_mail_contacts response into a
// clean {id, contactEmail, displayName} contact list — output-projection
// clean output projection. Both the list container and per-item field names are probed
// defensively across candidate keys, so an empty/unknown shape yields an empty
// list rather than a crash or fabricated data.
func contactListProject(data map[string]any) []map[string]any {
	raw := contactListResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := contactListFirst(m, "id", "contactId", "contact_id"); ok {
			row["id"] = v
		}
		if v, ok := contactListFirst(m, "contactEmail", "contact_email", "email"); ok {
			row["contactEmail"] = v
		}
		if v, ok := contactListFirst(m, "displayName", "display_name", "name"); ok {
			row["displayName"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// contactListResolveList locates the list payload inside the response,
// tolerating a bare top-level array container or nesting one level deeper.
func contactListResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "contacts"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "contacts", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// contactListFirst returns the first present candidate key's value.
func contactListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// ContactUpdate 更新邮件联系人。
// ContactBatchDelete 批量删除邮件联系人。
// ── auto-reply ─────────────────────────────────────────────

// AutoReplyGet 获取用户的自动回复配置。
// ── rule ───────────────────────────────────────────────────

// RuleList 列出个人收信规则。
// RuleCreate 创建个人收信规则。
// RuleUpdate 更新个人收信规则。
// RuleDelete 删除个人收信规则。
// RuleAdjust 调整收信规则排序。
func init() {
	shortcut.Register(
		ThreadList,
		FolderList,
		TagList,
		UserSearch,
		TemplateList,
		ContactList,
	)
}
