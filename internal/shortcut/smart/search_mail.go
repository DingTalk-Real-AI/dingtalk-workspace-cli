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
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
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
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "mail",
			Name:           "shortcut_search_mail",
			CanonicalPath:  "mail.shortcut_search_mail",
			CLIPath:        "mail +search-mail",
			PrimaryCLIPath: "mail +search-mail",
		},
		Description: "按 KQL 关键词搜索邮件并投影列表（主题/发件人/时间/messageId）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按 KQL 关键词搜索邮件并投影列表（主题/发件人/时间/messageId）",
			UseWhen:      []string{"当你想按关键词（KQL 表达式，如 subject:周报、from:alice、hasAttachments:true、folderId:2 等）快速搜自己的邮件、并只看一份精简清单（主题、发件人、时间、邮件 messageId）而不想翻完整正文时使用；内部先确定要搜的邮箱地址——你可以用 --email 指定，不指定时自动取你绑定的第一个邮箱——再执行邮件搜索，最后在本地把每封邮件投影成 {subject, from, date, messageId} 打印出来，可配合 --format/--jq/--fields。这是纯只读操作，只做搜索与本地投影，不会修改、发送或删除任何邮件；若没有命中则返回空列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws mail +search-mail --query \"subject:周报\"",
				"dws mail +search-mail --query \"from:alice AND date>2025-06-01T00:00:00Z\"",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "KQL 搜索表达式（如 subject:周报、from:alice、folderId:2），不能为空", Required: true},
		{Name: "email", Type: shortcut.FlagString, Desc: "要搜索的邮箱地址（可选，默认取你绑定的第一个邮箱）", Required: false},
		{Name: "size", Type: shortcut.FlagString, Desc: "返回条数上限（可选，默认 20；显式提供时必须是 1-100 之间的整数）", Required: false},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，取自上一页 nextCursor", Required: false},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"query"}, Description: "不能为空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"size"}, Description: "1-100"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if err := smartMailValidateRequiredText(rt, "query"); err != nil {
			return err
		}
		return smartMailValidateStringPageSize(rt, "size")
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

		// Step 2 — search that mailbox. email/query/size mirror the search_emails
		// call in helpers.messageSearch; size is passed as a string.
		args := map[string]any{
			"email": email,
			"query": rt.Str("query"),
			"size":  size,
		}
		if rt.Changed("cursor") {
			args["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("mail", "search_emails", args)
		if err != nil {
			return err
		}

		// Step 3 — project each message to a compact record.
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

// searchMailFirstMailbox lists the current user's bound mailboxes via
// list_user_mailboxes and returns the first mailbox address. The gateway wraps
// the list under "mailboxes" (per helper docs); we probe a few container keys
// and address field names defensively.
func searchMailFirstMailbox(rt *shortcut.RuntimeContext) (string, error) {
	data, err := rt.CallMCPData("mail", "list_user_mailboxes", nil)
	if err != nil {
		return "", err
	}
	mailboxes, err := searchMailMailboxAddresses(data)
	if err != nil {
		return "", err
	}
	if len(mailboxes) > 0 {
		return mailboxes[0], nil
	}
	return "", apperrors.NewValidation("未找到可用邮箱，请用 --email 指定要搜索的邮箱地址")
}

// searchMailMailboxAddresses accepts the two reviewed mailbox wire shapes:
// non-empty address strings and non-empty objects carrying an email field.
// The collection itself and every item remain fail-closed, so malformed rows
// cannot be skipped and misreported as an empty mailbox list.
func searchMailMailboxAddresses(data map[string]any) ([]string, error) {
	const operation = "mail/list_user_mailboxes"
	if err := smartMailSuccess(data, operation); err != nil {
		return nil, err
	}
	paths := []string{"emailAccounts", "result.emailAccounts", "data.emailAccounts"}
	var value any
	selectedPath := ""
	for _, path := range paths {
		candidate, present := smartMailLookup(data, path)
		if !present {
			continue
		}
		if selectedPath != "" {
			return nil, smartMailError(operation, "conflicting_collection", fmt.Sprintf("响应同时包含 %s 与 %s，无法选择唯一邮箱集合", selectedPath, path))
		}
		value = candidate
		selectedPath = path
	}
	if selectedPath == "" {
		return nil, smartMailError(operation, "missing_collection", "成功响应缺少 emailAccounts 数组；不能把未知响应结构当作空结果")
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, smartMailError(operation, "malformed_collection", fmt.Sprintf("响应 %s 应为数组，实际为 %T", selectedPath, value))
	}
	addresses := make([]string, 0, len(raw))
	for index, item := range raw {
		var address string
		switch typed := item.(type) {
		case string:
			address = strings.TrimSpace(typed)
		case map[string]any:
			address = searchMailFirstString(typed, "email")
		default:
			return nil, smartMailError(operation, "malformed_item", fmt.Sprintf("响应 %s[%d] 必须是邮箱字符串或含 email 的对象", selectedPath, index))
		}
		if address == "" {
			return nil, smartMailError(operation, "missing_item_identity", fmt.Sprintf("响应 %s[%d] 缺少非空邮箱地址", selectedPath, index))
		}
		addresses = append(addresses, address)
	}
	return addresses, nil
}

// searchMailFrom reads a message's sender, tolerating both a plain string and a
// nested object ({name, email/address}).
func searchMailFrom(m map[string]any) (any, error) {
	raw, present := m["from"]
	switch v := raw.(type) {
	case string:
		if normalized := strings.TrimSpace(v); normalized != "" {
			return normalized, nil
		}
	case map[string]any:
		name := searchMailFirstString(v, "name", "displayName")
		addr := searchMailFirstString(v, "email", "emailAddress", "address")
		switch {
		case name != "" && addr != "":
			return name + " <" + addr + ">", nil
		case addr != "":
			return addr, nil
		case name != "":
			return name, nil
		}
		return nil, smartMailError("mail/search_emails", "malformed_sender", "from 对象缺少姓名或邮箱")
	case nil:
		if present {
			return nil, smartMailError("mail/search_emails", "malformed_sender", "from 不能为 null")
		}
	default:
		return nil, smartMailError("mail/search_emails", "malformed_sender", fmt.Sprintf("from 字段类型错误：%T", raw))
	}
	if s := searchMailFirstString(m, "sender", "fromAddress", "fromName", "fromEmail"); s != "" {
		return s, nil
	}
	if present {
		return nil, smartMailError("mail/search_emails", "malformed_sender", "from 不能为空")
	}
	return nil, smartMailError("mail/search_emails", "missing_sender", "消息缺少可验证的发件人字段")
}

func searchMailFirstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func searchMailFirstAny(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			if s, isStr := v.(string); isStr {
				if strings.TrimSpace(s) == "" {
					continue
				}
				return strings.TrimSpace(s)
			}
			return v
		}
	}
	return nil
}

func init() {
	hardenSmartMail(&SearchMail, "messages", "严格校验的邮件搜索摘要")
	shortcut.Register(SearchMail)
}
