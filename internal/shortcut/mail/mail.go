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

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

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
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "mail",
			Name:           "shortcut_thread_list",
			CanonicalPath:  "mail.shortcut_thread_list",
			CLIPath:        "mail +thread-list",
			PrimaryCLIPath: "mail +thread-list",
		},
		Description: "列出指定邮箱文件夹下的邮件会话（thread）",
		Parameters: []contract.ParamDecl{
			{Name: "folder", Property: "folder"},
			{Name: "limit", Property: "limit"},
			{Name: "start", Property: "start"},
			{Name: "end", Property: "end"},
			{Name: "ascending", Property: "ascending"},
		},
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出指定邮箱文件夹下的邮件会话（thread）",
			UseWhen:      []string{"当你想按会话（同一往来主题的邮件串）而非单封邮件来浏览某个文件夹时使用；传入邮箱和文件夹 ID，可按时间范围和升降序筛选，返回会话列表及其 conversationId，供 +thread 查看详情。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws mail +thread-list --email user@company.com --folder 104 --limit 20"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "email", Type: shortcut.FlagString, Desc: "会话所属邮箱地址", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "邮件文件夹 ID（不是文件夹名称）", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "本次列出的会话数，最大 100"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，首次请求可不传"},
		{Name: "start", Type: shortcut.FlagString, Desc: "开始 UTC 时间，如 2024-01-01T00:00:00Z"},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束 UTC 时间，如 2024-12-31T23:59:59Z"},
		{Name: "ascending", Type: shortcut.FlagBool, Desc: "是否按时间升序"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "--limit 必须在 1-100 之间"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"start", "end"}, Description: "--start/--end 必须是 UTC RFC3339 时间，且 end 不能早于 start"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return mailValidateThreadList(rt)
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
		threads, err := mailProjectCollection(data, "mail/list_mailbox_threads", "result.conversations", []string{"id"}, map[string][]string{
			"conversationId": {"id"}, "subject": {"subject"}, "lastUpdated": {"lastModifiedDateTime"}, "isRead": {"isRead"},
		})
		if err != nil {
			return err
		}
		complete, next, err := mailPage(data, "mail/list_mailbox_threads", "result", rt.Str("cursor"))
		if err != nil {
			return err
		}
		return mailOutputPage(rt, "threads", threads, complete, next)
	},
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
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "mail",
			Name:           "shortcut_folder_list",
			CanonicalPath:  "mail.shortcut_folder_list",
			CLIPath:        "mail +folder-list",
			PrimaryCLIPath: "mail +folder-list",
		},
		Description: "列出顶层文件夹或指定父文件夹下的子文件夹",
		Parameters:  []contract.ParamDecl{{Name: "folder", Property: "folder"}},
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出顶层文件夹或指定父文件夹下的子文件夹",
			UseWhen:      []string{"当你需要了解某个邮箱有哪些文件夹、或要取得文件夹 ID 以便移动邮件、按文件夹列信/建规则时使用；传入邮箱（可选父文件夹 ID 查子级），返回文件夹列表及其 ID。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws mail +folder-list --email user@company.com"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "email", Type: shortcut.FlagString, Desc: "邮件所属邮箱地址，不能为空", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "父文件夹 ID，不传则返回顶层文件夹"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"email"}, Description: "不能为空"}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return mailValidateRequiredText(rt, "email")
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
		folders, err := mailProjectCollection(data, "mail/list_folders", "folders", []string{"id"}, map[string][]string{
			"id": {"id"}, "name": {"displayName"}, "parentId": {"parentFolderId"},
		})
		if err != nil {
			return err
		}
		return rt.Output(mailBusinessCollectionPayload("folders", folders))
	},
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
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "mail",
			Name:           "shortcut_tag_list",
			CanonicalPath:  "mail.shortcut_tag_list",
			CLIPath:        "mail +tag-list",
			PrimaryCLIPath: "mail +tag-list",
		},
		Description: "列出指定邮箱下的所有邮件标签",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出指定邮箱下的所有邮件标签",
			UseWhen:      []string{"当你要查看邮箱里有哪些标签、或需要取得标签 ID 以便给邮件/会话加标签时使用；传入邮箱地址，返回全部邮件标签及其 ID。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws mail +tag-list --email user@company.com"},
		},
	},
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
		tags, err := mailProjectCollection(data, "mail/list_tags", "tags", []string{"id"}, map[string][]string{
			"id": {"id"}, "name": {"name"}, "parentId": {"parentId"},
		})
		if err != nil {
			return err
		}
		return rt.Output(mailBusinessCollectionPayload("tags", tags))
	},
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
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "mail",
			Name:           "shortcut_user_search",
			CanonicalPath:  "mail.shortcut_user_search",
			CLIPath:        "mail +user-search",
			PrimaryCLIPath: "mail +user-search",
		},
		Description: "按关键词或工号搜索邮箱用户（仅企业邮箱）",
		Parameters: []contract.ParamDecl{
			{Name: "employee-no", Property: "employeeNo"},
			{Name: "limit", Property: "limit"},
		},
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按关键词或工号搜索邮箱用户（仅企业邮箱）",
			UseWhen:      []string{"当你只知道同事的姓名或工号、需要查出其企业邮箱地址以便发信或添加联系人时使用；提供关键词或工号（至少其一），返回匹配的企业邮箱用户列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws mail +user-search --keyword \"张三\"",
				"dws mail +user-search --email user@company.com --employee-no \"E123456\"",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "keyword", Type: shortcut.FlagString, Desc: "搜索关键词；显式提供时不能为空（未提供 --employee-no 时为必填）"},
		{Name: "employee-no", Type: shortcut.FlagString, Desc: "按工号精确搜索；显式提供时不能为空"},
		{Name: "email", Type: shortcut.FlagString, Desc: "搜索目标邮箱地址"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，取自响应中的 nextCursor"},
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页返回数量，必须是 1-100 之间的整数"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"keyword", "employee-no"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"keyword", "employee-no"}, Description: "不能为空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "1-100"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if err := mailValidateStringPageSize(rt, "limit", false); err != nil {
			return err
		}
		for _, name := range []string{"keyword", "employee-no"} {
			if rt.Changed(name) && strings.TrimSpace(rt.Str(name)) == "" {
				return apperrors.NewValidation("--" + name + " 显式提供时不能为空")
			}
		}
		if strings.TrimSpace(rt.Str("keyword")) == "" && strings.TrimSpace(rt.Str("employee-no")) == "" {
			return apperrors.NewValidation("--keyword 和 --employee-no 至少需要一个非空值")
		}
		return nil
	},
	Tips: []string{
		`dws mail +user-search --keyword "张三"`,
		`dws mail +user-search --email user@company.com --employee-no "E123456"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		size, err := mailStringPageSize(rt, "limit", false)
		if err != nil {
			return err
		}
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
			params["size"] = size
		}
		data, err := rt.CallMCPData("mail", "search_mail_users", params)
		if err != nil {
			return err
		}
		users, err := mailProjectCollection(data, "mail/search_mail_users", "users", []string{"id", "email"}, map[string][]string{
			"name": {"name"}, "email": {"email"}, "employeeNo": {"employeeNo"}, "userId": {"id"},
		})
		if err != nil {
			return err
		}
		complete, next, err := mailPage(data, "mail/search_mail_users", "", rt.Str("cursor"))
		if err != nil {
			return err
		}
		return mailOutputPage(rt, "users", users, complete, next)
	},
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
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "mail",
			Name:           "shortcut_template_list",
			CanonicalPath:  "mail.shortcut_template_list",
			CLIPath:        "mail +template-list",
			PrimaryCLIPath: "mail +template-list",
		},
		Description: "列出指定邮箱的所有邮件模板",
		Parameters:  []contract.ParamDecl{{Name: "limit", Property: "limit"}},
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出指定邮箱的所有邮件模板",
			UseWhen:      []string{"当你想查看某个邮箱下已有哪些邮件模板、或需要取得模板 ID 以便查看详情或更新时使用；传入邮箱和每页数量，返回模板列表，支持分页。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws mail +template-list --email user@company.com --limit 20"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "email", Type: shortcut.FlagString, Desc: "用户邮箱地址", Required: true},
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页返回数量，必须是 1-100 之间的整数", Required: true},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，取自响应中的 nextCursor"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "--limit 必须在 1-100 之间"}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return mailValidateStringPageSize(rt, "limit", true)
	},
	Tips: []string{
		`dws mail +template-list --email user@company.com --limit 20`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		size, err := mailStringPageSize(rt, "limit", true)
		if err != nil {
			return err
		}
		params := map[string]any{
			"email": rt.Str("email"),
			"size":  size,
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("mail", "list_user_message_templates", params)
		if err != nil {
			return err
		}
		templates, err := mailProjectCollection(data, "mail/list_user_message_templates", "templates", []string{"id"}, map[string][]string{
			"id": {"id"}, "name": {"name"}, "subject": {"subject"},
		})
		if err != nil {
			return err
		}
		complete, next, err := mailPage(data, "mail/list_user_message_templates", "", rt.Str("cursor"))
		if err != nil {
			return err
		}
		return mailOutputPage(rt, "templates", templates, complete, next)
	},
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
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "mail",
			Name:           "shortcut_contact_list",
			CanonicalPath:  "mail.shortcut_contact_list",
			CLIPath:        "mail +contact-list",
			PrimaryCLIPath: "mail +contact-list",
		},
		Description: "列出指定邮箱的所有邮件联系人",
		Parameters:  []contract.ParamDecl{{Name: "limit", Property: "limit"}},
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出指定邮箱的所有邮件联系人",
			UseWhen:      []string{"当你想查看某邮箱通讯录里有哪些联系人、或需要取得联系人 ID 以便更新或删除时使用；传入邮箱和每页数量，返回联系人列表，支持分页。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws mail +contact-list --email user@company.com --limit 20"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "email", Type: shortcut.FlagString, Desc: "用户邮箱地址", Required: true},
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页返回数量，必须是 1-100 之间的整数", Required: true},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，取自响应中的 nextCursor"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "--limit 必须在 1-100 之间"}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return mailValidateStringPageSize(rt, "limit", true)
	},
	Tips: []string{
		`dws mail +contact-list --email user@company.com --limit 20`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		size, err := mailStringPageSize(rt, "limit", true)
		if err != nil {
			return err
		}
		params := map[string]any{
			"email": rt.Str("email"),
			"size":  size,
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("mail", "list_user_mail_contacts", params)
		if err != nil {
			return err
		}
		contacts, err := mailProjectCollection(data, "mail/list_user_mail_contacts", "contacts", []string{"id"}, map[string][]string{
			"id": {"id"}, "contactEmail": {"contactEmail", "email"}, "displayName": {"displayName", "name"},
		})
		if err != nil {
			return err
		}
		complete, next, err := mailPage(data, "mail/list_user_mail_contacts", "", rt.Str("cursor"))
		if err != nil {
			return err
		}
		return mailOutputPage(rt, "contacts", contacts, complete, next)
	},
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
	hardenPublicMailContracts()
	shortcut.Register(
		ThreadList,
		FolderList,
		TagList,
		UserSearch,
		TemplateList,
		ContactList,
	)
}
