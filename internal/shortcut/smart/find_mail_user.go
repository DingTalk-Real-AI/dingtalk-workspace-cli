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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
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
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "mail",
			Name:           "shortcut_find_mail_user",
			CanonicalPath:  "mail.shortcut_find_mail_user",
			CLIPath:        "mail +find-mail-user",
			PrimaryCLIPath: "mail +find-mail-user",
		},
		Description: "按关键词搜索邮箱联系人并投影列表（姓名/昵称/邮箱/工号等）",
		Parameters: []contract.ParamDecl{
			// Keep the published composite Shortcut property stable. Execute owns
			// the explicit query -> keyword adapter for search_mail_users.
			{Name: "query", Property: "query"},
			{Name: "limit", Property: "limit"},
			{Name: "cursor", Property: "cursor"},
		},
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按关键词搜索邮箱联系人并投影列表（姓名/昵称/邮箱/工号等）",
			UseWhen:      []string{"当你只知道某人的姓名、花名或邮箱片段，想在企业邮箱通讯录里按关键词把匹配的邮箱用户找出来、并只看一份精简清单（姓名、昵称、邮箱地址、工号、职位、工作地）而不想拿到一大坨原始字段时使用；内部按 --query 关键词调用邮箱用户搜索，再在本地把每个匹配用户投影成整洁记录打印出来，可配合 --format/--jq/--fields。这是纯只读操作，只做搜索与本地投影，不会修改任何数据；注意仅企业邮箱可用（个人邮箱如 xxx@dingtalk.com 会因无权限报错）；若没有命中则提示「没搜到邮箱联系人」。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws mail +find-mail-user --query \"张三\"",
				"dws mail +find-mail-user --query alice --limit 10",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词（姓名/花名/邮箱片段，必填且不能为空）", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "返回条数上限（可选，1-100）", Required: false},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，取自上一页 nextCursor", Required: false},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"query"}, Description: "不能为空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "1-100"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if err := smartMailValidateRequiredText(rt, "query"); err != nil {
			return err
		}
		return smartMailValidatePageSize(rt, "limit", false)
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
		if rt.Changed("cursor") {
			args["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("mail", "search_mail_users", args)
		if err != nil {
			return err
		}

		// Step 2 — project matched users.
		users, err := smartMailCollection(data, "mail/search_mail_users", "users")
		if err != nil {
			return err
		}
		results := make([]map[string]any, 0, len(users))
		for _, u := range users {
			projected, err := findMailUserProjection(u)
			if err != nil {
				return err
			}
			results = append(results, projected)
		}

		complete, next, err := smartMailPage(data, "mail/search_mail_users", "", rt.Str("cursor"))
		if err != nil {
			return err
		}
		return smartMailOutputPage(rt, "users", results, complete, next)
	},
}

func findMailUserProjection(user map[string]any) (map[string]any, error) {
	result := map[string]any{
		"name":         findMailUserString(user, "name", "displayName", "userName", "nick"),
		"nickname":     findMailUserString(user, "nickname", "nick", "displayName"),
		"employeeNo":   findMailUserAny(user, "employeeNo", "employeeNumber", "jobNumber"),
		"jobTitle":     findMailUserString(user, "jobTitle", "title", "position"),
		"workLocation": findMailUserString(user, "workLocation", "location", "workPlace"),
	}
	email, emailPresent, err := findMailUserIdentityString(user, "email", "mail", "emailAddress", "address", "account")
	if err != nil {
		return nil, err
	}
	if emailPresent {
		result["email"] = email
	}
	id, idPresent, err := findMailUserIdentity(user, "id", "userId", "userid")
	if err != nil {
		return nil, err
	}
	if idPresent {
		result["id"] = id
	}
	if !emailPresent && !idPresent {
		return nil, smartMailError("mail/search_mail_users", "missing_item_identity", "邮箱用户缺少非空 id 或 email")
	}
	return result, nil
}

func findMailUserIdentityString(m map[string]any, keys ...string) (string, bool, error) {
	for _, key := range keys {
		value, present := m[key]
		if !present || value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return "", false, smartMailError("mail/search_mail_users", "malformed_item_identity", "邮箱用户 email 必须是字符串")
		}
		text = strings.TrimSpace(text)
		if text != "" {
			return text, true, nil
		}
	}
	return "", false, nil
}

func findMailUserIdentity(m map[string]any, keys ...string) (any, bool, error) {
	for _, key := range keys {
		value, present := m[key]
		if !present || value == nil {
			continue
		}
		if text, ok := value.(string); ok {
			text = strings.TrimSpace(text)
			if text != "" {
				return text, true, nil
			}
			continue
		}
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
			return value, true, nil
		default:
			return nil, false, smartMailError("mail/search_mail_users", "malformed_item_identity", "邮箱用户 id 必须是字符串或数字")
		}
	}
	return nil, false, nil
}

func findMailUserString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func findMailUserAny(m map[string]any, keys ...string) any {
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
	hardenSmartMail(&FindMailUser, "users", "严格校验的邮箱用户搜索结果")
	shortcut.Register(FindMailUser)
}
