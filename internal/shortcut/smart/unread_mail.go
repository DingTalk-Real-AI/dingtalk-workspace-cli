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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// UnreadMail: list the current user's unread emails as a compact projection.
//
// Steps:
//
//  1. resolve the mailbox address — use --email when given, otherwise pick the
//     current user's first bound mailbox via list_user_mailboxes (reusing
//     searchMailFirstMailbox from search_mail.go);
//
//  2. search that mailbox via search_emails with the KQL filter isRead:false,
//     matching the isRead field documented in helpers.messageSearch; size is
//     passed as a string, defaulting to "20" like the helper's sizeVal;
//
//  3. in Go, project each returned message to {subject, from, date, messageId}
//     via the shared searchMail* helpers and print the list with rt.Output so it
//     honours --format/--jq/--fields.
//
// Read-only: it only lists and projects, never mutating any mail.
//
//	dws mail +unread-mail
//	dws mail +unread-mail --email user@company.com
var UnreadMail = shortcut.Shortcut{
	Service:     "mail",
	Command:     "+unread-mail",
	Product:     "mail",
	Description: "列出未读邮件并投影列表（主题/发件人/时间/messageId）",
	Intent: "当你想快速看自己邮箱里有哪些未读邮件、并只看一份精简清单（主题、发件人、时间、邮件 messageId）而不想翻完整正文时使用；" +
		"内部先确定要查的邮箱地址——你可以用 --email 指定，不指定时自动取你绑定的第一个邮箱——" +
		"再用 KQL 过滤条件 isRead:false 搜索未读邮件，" +
		"最后在本地把每封邮件投影成 {subject, from, date, messageId} 打印出来，可配合 --format/--jq/--fields。" +
		"这是纯只读操作，只做搜索与本地投影，不会把邮件标记为已读，也不会修改、发送或删除任何邮件；若没有未读邮件则返回空列表。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "mail",
			Name:           "shortcut_unread_mail",
			CanonicalPath:  "mail.shortcut_unread_mail",
			CLIPath:        "mail +unread-mail",
			PrimaryCLIPath: "mail +unread-mail",
		},
		Description: "列出未读邮件并投影列表（主题/发件人/时间/messageId）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出未读邮件并投影列表（主题/发件人/时间/messageId）",
			UseWhen:      []string{"当你想快速看自己邮箱里有哪些未读邮件、并只看一份精简清单（主题、发件人、时间、邮件 messageId）而不想翻完整正文时使用；内部先确定要查的邮箱地址——你可以用 --email 指定，不指定时自动取你绑定的第一个邮箱——再用 KQL 过滤条件 isRead:false 搜索未读邮件，最后在本地把每封邮件投影成 {subject, from, date, messageId} 打印出来，可配合 --format/--jq/--fields。这是纯只读操作，只做搜索与本地投影，不会把邮件标记为已读，也不会修改、发送或删除任何邮件；若没有未读邮件则返回空列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws mail +unread-mail",
				"dws mail +unread-mail --email user@company.com",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "email", Type: shortcut.FlagString, Desc: "要查询的邮箱地址（可选，默认取你绑定的第一个邮箱）", Required: false},
		{Name: "size", Type: shortcut.FlagString, Desc: "返回条数上限（可选，默认 20；显式提供时必须是 1-100 之间的整数）", Required: false},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，取自上一页 nextCursor", Required: false},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"size"}, Description: "显式 --size 必须在 1-100 之间"}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return smartMailValidateStringPageSize(rt, "size")
	},
	Tips: []string{
		`dws mail +unread-mail`,
		`dws mail +unread-mail --email user@company.com`,
		`dws mail +unread-mail --size 50`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		size, err := smartMailStringPageSize(rt, "size", "20")
		if err != nil {
			return err
		}
		// Step 1 — resolve the mailbox address.
		email := rt.Str("email")
		if email == "" {
			resolved, err := searchMailFirstMailbox(rt)
			if err != nil {
				return err
			}
			email = resolved
		}

		// Step 2 — search unread mail. email/query/size mirror the search_emails
		// call in helpers.messageSearch; the isRead:false KQL filter matches the
		// isRead field documented there; size is passed as a string.
		args := map[string]any{
			"email": email,
			"query": "isRead:false",
			"size":  size,
		}
		if rt.Changed("cursor") {
			args["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("mail", "search_emails", args)
		if err != nil {
			return err
		}

		// Step 3 — project each message to a compact record (shared helpers).
		messages, err := smartMailSearchRows(data, "mail/search_emails")
		if err != nil {
			return err
		}
		out := make([]map[string]any, 0, len(messages))
		for _, m := range messages {
			from, err := searchMailFrom(m)
			if err != nil {
				return err
			}
			out = append(out, map[string]any{
				"subject":   searchMailFirstString(m, "subject", "title", "topic"),
				"from":      from,
				"date":      searchMailFirstAny(m, "date", "sentDate", "receivedDate", "sentTime", "internalDate", "createTime"),
				"messageId": searchMailFirstString(m, "messageId", "id", "mailId", "emailId", "internetMessageId"),
			})
		}
		complete, next, err := smartMailPage(data, "mail/search_emails", "", rt.Str("cursor"))
		if err != nil {
			return err
		}
		return smartMailOutputPage(rt, "messages", out, complete, next)
	},
}

func init() {
	hardenSmartMail(&UnreadMail, "messages", "严格校验的未读邮件摘要")
	markSmartMailCompatibilityOnly(&UnreadMail)
	shortcut.Register(UnreadMail)
}
