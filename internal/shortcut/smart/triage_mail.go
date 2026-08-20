// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// TriageMail aligns the Lark task-level triage entry while preserving DWS's
// easier mailbox and inbox resolution. It returns summaries only; callers use
// +message or +messages for full bodies.
var TriageMail = shortcut.Shortcut{
	Service: "mail", Command: "+triage", Product: "mail",
	Description: "列出或筛选邮件摘要，自动解析邮箱与收件箱", Intent: "快速浏览收件箱摘要，或用 KQL 条件筛选邮件；不提供条件时自动定位当前邮箱的收件箱，返回可继续交给 +message/+messages 的稳定 ID。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Description: "列出或筛选邮件摘要，自动解析邮箱与收件箱",
		Identity: contract.ToolIdentitySpec{
			ProductID: "mail", Name: "shortcut_triage", CanonicalPath: "mail.shortcut_triage",
			CLIPath: "mail +triage", PrimaryCLIPath: "mail +triage",
		},
		Interface: &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: "Resolves the current mailbox and inbox locally, then strictly validates search_emails summaries and pagination."},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出或筛选邮件摘要，自动解析邮箱与收件箱",
			UseWhen:      []string{"快速浏览收件箱摘要，或用 KQL 条件筛选邮件；不提供条件时自动定位当前邮箱的收件箱，返回可继续交给 +message/+messages 的稳定 ID。"},
			AvoidWhen:    []string{"已知单个 messageId 并要读正文时用 mail +message；需要多封完整正文时用 mail +messages"},
			Examples:     []string{`dws mail +triage --limit 20 --format json`, `dws mail +triage --query "isRead:false" --format json`},
		},
		Parameters: []contract.ParamDecl{{Name: "limit", Property: "size"}, {Name: "cursor", Property: "cursor"}, {Name: "query", Property: "query"}},
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "可选 KQL 条件；不传时列出收件箱"},
		{Name: "email", Type: shortcut.FlagString, Desc: "邮箱地址；不传时自动取当前身份首个邮箱"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页摘要数量，必须在 1-100 之间"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "1-100"}},
	Validate: func(rt *shortcut.RuntimeContext) error {
		return smartMailValidatePageSize(rt, "limit", true)
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		email := rt.Str("email")
		if email == "" {
			resolved, err := searchMailFirstMailbox(rt)
			if err != nil {
				return err
			}
			email = resolved
		}
		query := rt.Str("query")
		if query == "" {
			folderID, err := recentMailInboxFolder(rt, email)
			if err != nil {
				return err
			}
			query = "folderId:" + folderID
		}
		args := map[string]any{"email": email, "query": query, "size": strconv.Itoa(rt.Int("limit"))}
		if rt.Changed("cursor") {
			args["cursor"] = rt.Str("cursor")
		}
		data, err := rt.CallMCPData("mail", "search_emails", args)
		if err != nil {
			return err
		}
		messages, err := smartMailSearchRows(data, "mail/search_emails")
		if err != nil {
			return err
		}
		rows := make([]map[string]any, 0, len(messages))
		for _, message := range messages {
			from, err := searchMailFrom(message)
			if err != nil {
				return err
			}
			rows = append(rows, map[string]any{
				"messageId": searchMailFirstString(message, "id"),
				"subject":   searchMailFirstString(message, "subject"),
				"from":      from,
				"date":      searchMailFirstAny(message, "receivedDateTime", "date", "sentDateTime"),
			})
		}
		complete, next, err := smartMailPage(data, "mail/search_emails", "", rt.Str("cursor"))
		if err != nil {
			return err
		}
		return smartMailOutputPage(rt, "messages", rows, complete, next)
	},
}

func init() {
	hardenSmartMail(&TriageMail, "messages", "严格校验的邮件摘要与分页证据")
	shortcut.Register(TriageMail)
}
