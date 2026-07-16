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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// FindMailUser: search mailbox users by keyword (name / nickname / email
// fragment) and project a compact contact list in one step.
//
// Steps:
//
//  1. call search_mail_users with keyword=--query (and size=--limit when
//     given). The tool name, server ("mail") and the "keyword"/"size" argument
//     keys are taken verbatim from helpers.mail.go's `mail user search` command
//     (callMCPTool("search_mail_users", …) with toolArgs["keyword"]/["size"]);
//
//  2. in Go, defensively unwrap the user list and project each entry to
//     {name, nickname, email, employeeNo, jobTitle, workLocation, id} — field
//     parsing probes several candidate keys — and print via rt.Output so it
//     honours --format/--jq/--fields;
//
//  3. if nothing matched, report "没搜到邮箱联系人" instead of an empty raw dump.
//
// Read-only: it only searches and reshapes, never mutating anything.
//
// Note: per the helper docs this only works for enterprise mailboxes (not
// @dingtalk.com personal mailboxes).
//
//	dws mail +find-mail-user --query "张三"
//	dws mail +find-mail-user --query alice --limit 10
var FindMailUser = shortcut.Shortcut{
	Service:     "mail",
	Command:     "+find-mail-user",
	Product:     "mail",
	Description: "按关键词搜索邮箱联系人并投影列表（姓名/昵称/邮箱/工号等）",
	Intent: "当你只知道某人的姓名、花名或邮箱片段，想在企业邮箱通讯录里按关键词把匹配的邮箱用户找出来、" +
		"并只看一份精简清单（姓名、昵称、邮箱地址、工号、职位、工作地）而不想拿到一大坨原始字段时使用；" +
		"内部按 --query 关键词调用邮箱用户搜索，再在本地把每个匹配用户投影成整洁记录打印出来，可配合 --format/--jq/--fields。" +
		"这是纯只读操作，只做搜索与本地投影，不会修改任何数据；" +
		"注意仅企业邮箱可用（个人邮箱如 xxx@dingtalk.com 会因无权限报错）；若没有命中则提示「没搜到邮箱联系人」。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词（姓名/花名/邮箱片段，必填）", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "返回条数上限（可选）", Required: false},
	},
	Tips: []string{
		`dws mail +find-mail-user --query "张三"`,
		`dws mail +find-mail-user --query alice --limit 10`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — search mailbox users. keyword/size mirror the toolArgs built in
		// helpers.mail.go's `mail user search` command; size is passed as a string.
		args := map[string]any{"keyword": rt.Str("query")}
		if rt.Changed("limit") {
			args["size"] = strconv.Itoa(rt.Int("limit"))
		}
		data, err := rt.CallMCPData("mail", "search_mail_users", args)
		if err != nil {
			return err
		}

		// Step 2 — project matched users.
		users := findMailUserUnwrap(data)
		results := make([]map[string]any, 0, len(users))
		for _, u := range users {
			results = append(results, map[string]any{
				"name":         findMailUserString(u, "name", "displayName", "userName", "nick"),
				"nickname":     findMailUserString(u, "nickname", "nick", "displayName"),
				"email":        findMailUserString(u, "email", "mail", "emailAddress", "address", "account"),
				"employeeNo":   findMailUserAny(u, "employeeNo", "employeeNumber", "jobNumber"),
				"jobTitle":     findMailUserString(u, "jobTitle", "title", "position"),
				"workLocation": findMailUserString(u, "workLocation", "location", "workPlace"),
				"id":           findMailUserAny(u, "id", "userId", "userid"),
			})
		}

		// Step 3 — empty result guard.
		if len(results) == 0 {
			return apperrors.NewValidation("没搜到邮箱联系人")
		}

		return rt.Output(map[string]any{"users": results, "count": len(results)})
	},
}

// findMailUserUnwrap extracts the user list from a search_mail_users response.
// The helper documents the list under "users"; we probe several container keys
// at the top level and one level deep, defensively.
func findMailUserUnwrap(data map[string]any) []map[string]any {
	keys := []string{"users", "contacts", "result", "data", "list", "items", "records"}
	for _, key := range keys {
		if arr, ok := data[key].([]any); ok {
			return findMailUserToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range keys {
				if arr, ok := inner[k2].([]any); ok {
					return findMailUserToMaps(arr)
				}
			}
		}
	}
	return nil
}

func findMailUserToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func findMailUserString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := m[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func findMailUserAny(m map[string]any, keys ...string) any {
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
	shortcut.Register(FindMailUser)
}
