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

// RecentMail: list recent inbox mail threads (conversations) and project a
// compact list in one step.
//
// Steps:
//
//  1. resolve the mailbox address — use --email when given, otherwise pick the
//     current user's first bound mailbox via list_user_mailboxes (reusing
//     searchMailFirstMailbox from search_mail.go);
//
//  2. resolve the folder ID — list_mailbox_threads requires a folderId (a folder
//     ID, not a name). Use --folder when given, otherwise resolve the inbox by
//     listing the mailbox's top-level folders via list_folders and matching the
//     inbox displayName (收件箱 / Inbox);
//
//  3. list the folder's threads via list_mailbox_threads (email / folderId / size
//     mirror the helpers.mail thread-list call; size defaults to 20, capped at
//     100, matching the helper's 1..100 limit);
//
//  4. in Go, project each conversation to {subject, from, date, threadId} and
//     print the list via rt.Output so it honours --format/--jq/--fields.
//
// Read-only: it only lists and projects, never mutating any mail.
//
//	dws mail +recent-mail
//	dws mail +recent-mail --limit 30
//	dws mail +recent-mail --email user@company.com --folder 2
var RecentMail = shortcut.Shortcut{
	Service:     "mail",
	Command:     "+recent-mail",
	Product:     "mail",
	Description: "列出收件箱近期邮件会话并投影列表（主题/发件人/时间/threadId）",
	Intent: "当你想快速看一眼自己邮箱里近期的邮件会话（收件箱线程/conversation），只需要一份精简清单（主题、发件人、最后修改时间、会话 threadId）、" +
		"而不想翻完整正文或原始字段时使用；" +
		"内部先确定要看的邮箱地址——你可以用 --email 指定，不指定时自动取你绑定的第一个邮箱——再解析要看的文件夹——" +
		"你可以用 --folder 指定文件夹 ID，不指定时自动定位收件箱——然后列出该文件夹下的近期会话，" +
		"最后在本地把每条会话投影成 {subject, from, date, threadId} 打印出来，可配合 --format/--jq/--fields。" +
		"这是纯只读操作，只做列举与本地投影，不会修改、发送或删除任何邮件；若最近没有邮件则提示「最近没有邮件」。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagInt, Desc: "返回会话条数上限（可选，默认 20，最大 100）", Required: false},
		{Name: "email", Type: shortcut.FlagString, Desc: "要查看的邮箱地址（可选，默认取你绑定的第一个邮箱）", Required: false},
		{Name: "folder", Type: shortcut.FlagString, Desc: "文件夹 ID（可选，默认定位收件箱）", Required: false},
	},
	Tips: []string{
		`dws mail +recent-mail`,
		`dws mail +recent-mail --limit 30`,
		`dws mail +recent-mail --email user@company.com --folder 2`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — resolve the mailbox address (reuses search_mail.go).
		email := rt.Str("email")
		if email == "" {
			resolved, err := searchMailFirstMailbox(rt)
			if err != nil {
				return err
			}
			email = resolved
		}

		// Step 2 — resolve the folder ID. list_mailbox_threads requires folderId.
		folderID := rt.Str("folder")
		if folderID == "" {
			resolved, err := recentMailInboxFolder(rt, email)
			if err != nil {
				return err
			}
			folderID = resolved
		}

		// Step 3 — list threads. email/folderId/size mirror the helpers.mail
		// thread-list call; size is an int in 1..100.
		size := rt.Int("limit")
		if size <= 0 {
			size = 20
		}
		if size > 100 {
			size = 100
		}
		data, err := rt.CallMCPData("mail", "list_mailbox_threads", map[string]any{
			"email":    email,
			"folderId": folderID,
			"size":     size,
		})
		if err != nil {
			return err
		}

		// Step 4 — project each conversation to a compact record.
		threads := searchMailUnwrapList(data, "conversations", "threads", "result", "data", "list", "items", "records")
		results := make([]map[string]any, 0, len(threads))
		for _, t := range threads {
			results = append(results, map[string]any{
				"subject":  searchMailFirstString(t, "subject", "title", "topic"),
				"from":     recentMailSenders(t),
				"date":     searchMailFirstAny(t, "lastModifiedDateTime", "date", "sentTime", "sentDate", "receivedDate", "createTime"),
				"threadId": searchMailFirstString(t, "threadId", "id", "conversationId"),
			})
		}

		// Empty result guard.
		if len(results) == 0 {
			return apperrors.NewValidation("最近没有邮件")
		}

		return rt.Output(map[string]any{"count": len(results), "mails": results})
	},
}

// recentMailInboxFolder lists the mailbox's top-level folders via list_folders
// and returns the inbox folder ID, matching the inbox displayName defensively.
// The helper documents the list under "folders" with id / displayName fields.
func recentMailInboxFolder(rt *shortcut.RuntimeContext, email string) (string, error) {
	data, err := rt.CallMCPData("mail", "list_folders", map[string]any{
		"email": email,
	})
	if err != nil {
		return "", err
	}
	folders := searchMailUnwrapList(data, "folders", "list", "items", "data", "result", "records")
	inboxNames := map[string]bool{
		"收件箱": true, "inbox": true,
	}
	for _, f := range folders {
		name := searchMailFirstString(f, "displayName", "name", "folderName")
		if inboxNames[name] {
			if id := searchMailFirstString(f, "id", "folderId"); id != "" {
				return id, nil
			}
		}
	}
	return "", apperrors.NewValidation("未找到收件箱文件夹，请用 --folder 指定文件夹 ID（可通过 dws mail folder list 获取）")
}

// recentMailSenders reads a conversation's sender list. The helper documents a
// "senders" array of {email, name}; we tolerate a plain string, a single object,
// and fall back to the shared searchMailFrom logic.
func recentMailSenders(t map[string]any) any {
	if arr, ok := t["senders"].([]any); ok && len(arr) > 0 {
		names := make([]string, 0, len(arr))
		for _, it := range arr {
			m, ok := it.(map[string]any)
			if !ok {
				if s, ok := it.(string); ok && s != "" {
					names = append(names, s)
				}
				continue
			}
			name := searchMailFirstString(m, "name", "displayName")
			addr := searchMailFirstString(m, "email", "emailAddress", "address")
			switch {
			case name != "" && addr != "":
				names = append(names, name+" <"+addr+">")
			case addr != "":
				names = append(names, addr)
			case name != "":
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			return names
		}
	}
	return searchMailFrom(t)
}

func init() {
	shortcut.Register(RecentMail)
}
