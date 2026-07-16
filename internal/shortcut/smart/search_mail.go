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
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// SearchMail: search a mailbox by keyword and project a compact list in one step.
//
// Steps:
//
//  1. resolve the mailbox address — use --email when given, otherwise pick the
//     current user's first bound mailbox via list_user_mailboxes;
//
//  2. search that mailbox via search_emails (email / query / size mirror
//     helpers.messageSearch; size defaults to "20" as a string, matching the
//     helper's sizeVal handling);
//
//  3. in Go, project each returned message to {subject, from, date, messageId}
//     and print the list via rt.Output so it honours --format/--jq/--fields.
//
// Read-only: it only lists and projects, never mutating any mail.
//
//	dws mail +search-mail --query "subject:周报"
//	dws mail +search-mail --query "from:alice AND date>2025-06-01T00:00:00Z"
//	dws mail +search-mail --email user@company.com --query "hasAttachments:true"
var SearchMail = shortcut.Shortcut{
	Service:     "mail",
	Command:     "+search-mail",
	Product:     "mail",
	Description: "按 KQL 关键词搜索邮件并投影列表（主题/发件人/时间/messageId）",
	Intent: "当你想按关键词（KQL 表达式，如 subject:周报、from:alice、hasAttachments:true、folderId:2 等）快速搜自己的邮件、" +
		"并只看一份精简清单（主题、发件人、时间、邮件 messageId）而不想翻完整正文时使用；" +
		"内部先确定要搜的邮箱地址——你可以用 --email 指定，不指定时自动取你绑定的第一个邮箱——再执行邮件搜索，" +
		"最后在本地把每封邮件投影成 {subject, from, date, messageId} 打印出来，可配合 --format/--jq/--fields。" +
		"这是纯只读操作，只做搜索与本地投影，不会修改、发送或删除任何邮件；若没有命中则返回空列表。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "KQL 搜索表达式（如 subject:周报、from:alice、folderId:2）", Required: true},
		{Name: "email", Type: shortcut.FlagString, Desc: "要搜索的邮箱地址（可选，默认取你绑定的第一个邮箱）", Required: false},
		{Name: "size", Type: shortcut.FlagString, Desc: "返回条数上限（可选，默认 20）", Required: false},
	},
	Tips: []string{
		`dws mail +search-mail --query "subject:周报"`,
		`dws mail +search-mail --query "from:alice AND date>2025-06-01T00:00:00Z"`,
		`dws mail +search-mail --email user@company.com --query "hasAttachments:true"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if err := rt.RequireAll("query"); err != nil {
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

		// Step 2 — search that mailbox. email/query/size mirror the search_emails
		// call in helpers.messageSearch; size is passed as a string.
		size := rt.Str("size")
		if size == "" {
			size = "20"
		}
		data, err := rt.CallMCPData("mail", "search_emails", map[string]any{
			"email": email,
			"query": rt.Str("query"),
			"size":  size,
		})
		if err != nil {
			return err
		}

		// Step 3 — project each message to a compact record.
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

// searchMailFirstMailbox lists the current user's bound mailboxes via
// list_user_mailboxes and returns the first mailbox address. The gateway wraps
// the list under "mailboxes" (per helper docs); we probe a few container keys
// and address field names defensively.
func searchMailFirstMailbox(rt *shortcut.RuntimeContext) (string, error) {
	data, err := rt.CallMCPData("mail", "list_user_mailboxes", nil)
	if err != nil {
		return "", err
	}
	boxes := searchMailUnwrapList(data, "mailboxes", "list", "items", "data", "result", "records")
	for _, m := range boxes {
		if addr := searchMailFirstString(m, "email", "emailAddress", "mailbox", "address", "account"); addr != "" {
			return addr, nil
		}
	}
	// Some gateways return the addresses as a bare string list.
	for _, key := range []string{"mailboxes", "list", "items", "data", "result"} {
		if arr, ok := data[key].([]any); ok {
			for _, it := range arr {
				if s, ok := it.(string); ok && s != "" {
					return s, nil
				}
			}
		}
	}
	return "", apperrors.NewValidation("未找到可用邮箱，请用 --email 指定要搜索的邮箱地址")
}

// searchMailMessages extracts the message list from a search_emails response.
// The helper documents the list under "messages".
func searchMailMessages(data map[string]any) []map[string]any {
	return searchMailUnwrapList(data, "messages", "emails", "list", "items", "data", "result", "records")
}

// searchMailUnwrapList probes the given container keys at the top level, and one
// level deep, returning the first list found as []map[string]any.
func searchMailUnwrapList(data map[string]any, keys ...string) []map[string]any {
	for _, key := range keys {
		if arr, ok := data[key].([]any); ok {
			return searchMailToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range keys {
				if arr, ok := inner[k2].([]any); ok {
					return searchMailToMaps(arr)
				}
			}
		}
	}
	return nil
}

func searchMailToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// searchMailFrom reads a message's sender, tolerating both a plain string and a
// nested object ({name, email/address}).
func searchMailFrom(m map[string]any) any {
	switch v := m["from"].(type) {
	case string:
		if v != "" {
			return v
		}
	case map[string]any:
		name := searchMailFirstString(v, "name", "displayName")
		addr := searchMailFirstString(v, "email", "emailAddress", "address")
		switch {
		case name != "" && addr != "":
			return name + " <" + addr + ">"
		case addr != "":
			return addr
		case name != "":
			return name
		}
	}
	if s := searchMailFirstString(m, "sender", "fromAddress", "fromName", "fromEmail"); s != "" {
		return s
	}
	return ""
}

func searchMailFirstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := m[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func searchMailFirstAny(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			if s, isStr := v.(string); isStr && s == "" {
				continue
			}
			return v
		}
	}
	return nil
}

func init() {
	shortcut.Register(SearchMail)
}
