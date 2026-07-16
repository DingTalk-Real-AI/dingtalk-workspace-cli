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
	Flags: []shortcut.Flag{
		{Name: "email", Type: shortcut.FlagString, Desc: "要查询的邮箱地址（可选，默认取你绑定的第一个邮箱）", Required: false},
		{Name: "size", Type: shortcut.FlagString, Desc: "返回条数上限（可选，默认 20）", Required: false},
	},
	Tips: []string{
		`dws mail +unread-mail`,
		`dws mail +unread-mail --email user@company.com`,
		`dws mail +unread-mail --size 50`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
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
		size := rt.Str("size")
		if size == "" {
			size = "20"
		}
		data, err := rt.CallMCPData("mail", "search_emails", map[string]any{
			"email": email,
			"query": "isRead:false",
			"size":  size,
		})
		if err != nil {
			return err
		}

		// Step 3 — project each message to a compact record (shared helpers).
		messages := searchMailMessages(data)
		out := make([]map[string]any, 0, len(messages))
		for _, m := range messages {
			out = append(out, map[string]any{
				"subject":   searchMailFirstString(m, "subject", "title", "topic"),
				"from":      searchMailFrom(m),
				"date":      searchMailFirstAny(m, "date", "sentDate", "receivedDate", "sentTime", "internalDate", "createTime"),
				"messageId": searchMailFirstString(m, "messageId", "id", "mailId", "emailId", "internetMessageId"),
			})
		}
		return rt.Output(map[string]any{"messages": out, "email": email})
	},
}

func init() {
	shortcut.Register(UnreadMail)
}
