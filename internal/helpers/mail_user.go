// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func newMailUserGetCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   "get",
		Short: "根据完整企业邮箱精确查询员工",
		Long: `根据完整企业邮箱地址，精确查询当前组织内对应的钉钉员工。

--org-email 是目标员工的邮箱。操作人 uid 和组织 orgId 由平台根据当前登录身份注入，无需传入。
必须使用已知的完整邮箱；不得猜测或补全域名，不进行邮箱别名到主邮箱的转换。
保留邮箱大小写和 + 后缀，服务端会去除首尾空格。

返回 success、result、errorCode、errorMsg。success=true 且 result.uid 为空或不存在表示未找到有效员工；
result 包含 uid（钉钉全局用户 ID）、orgId、staffId（组织内 userId，不是工号）、name、orgEmail。
仅知道姓名或工号时使用 mail user search；查询邮箱容量和别名使用 mail mailbox profile。`,
		Example:       "  dws mail user get --org-email zhangsan@example.com",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        "mail",
		Tool:          "get_user_by_org_email",
		Flags: []LeafFlag{
			{Name: "org-email", Usage: "待查询员工的完整企业邮箱 (必填)，不自动补全域名或转换别名", Required: true, MarkRequired: true, Bind: "orgEmail", Trim: true},
		},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "mail", Name: "get_user_by_org_email", CanonicalPath: "mail.get_user_by_org_email",
				CLIPath: "mail user get", PrimaryCLIPath: "mail user get",
			},
			Description: "根据完整企业邮箱精确查询当前组织内的钉钉员工",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode: "mcp", Availability: "available",
				Ref: &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "get_user_by_org_email"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "org-email", Property: "orgEmail", InterfaceType: "string", Required: boolPtr(true)},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "用已知完整企业邮箱查询当前组织内员工的 uid、staffId 和姓名",
				UseWhen:      []string{"已知完整企业邮箱，需要精确查询该员工在当前组织的身份时"},
				AvoidWhen:    []string{"只有姓名或工号时使用 mail user search；查询邮箱容量和别名使用 mail mailbox profile；不得猜测邮箱或跨组织查询"},
				Examples:     []string{"dws mail user get --org-email zhangsan@example.com"},
			},
			Result: &contract.ResultSpec{
				Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{
					"type":"object",
					"description":"按企业邮箱查询员工的业务结果；未找到员工也是成功查询",
					"properties":{
						"success":{"type":"boolean","description":"查询是否成功，不代表一定找到员工"},
						"result":{"type":["object","null"],"description":"员工信息；未找到时可为 null、缺失或 uid 为空",
							"properties":{
								"uid":{"type":["integer","null"],"description":"员工的钉钉全局 UID，不同于组织内 userId/staffId"},
								"orgId":{"type":["integer","null"],"description":"当前查询范围的钉钉组织 ID"},
								"staffId":{"type":["string","null"],"description":"员工在组织内的 userId（staffId），不是工号"},
								"name":{"type":["string","null"],"description":"员工在企业通讯录中的姓名"},
								"orgEmail":{"type":["string","null"],"description":"目标员工登记的完整企业邮箱地址"}
							}},
						"errorCode":{"type":["string","null"],"description":"查询失败时的错误码"},
						"errorMsg":{"type":["string","null"],"description":"查询失败时的原因"}
					},"required":["success"]
				}`),
				SensitivePaths: []string{"result.uid", "result.staffId", "result.name", "result.orgEmail"},
			},
		},
		ResultCall: callMailUserLookupResult,
	})
}

func newMailUserBatchGetCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   "batch-get",
		Short: "根据多个完整企业邮箱批量查询员工",
		Long: `根据 1 至 100 个完整企业邮箱，批量精确查询当前组织内的钉钉员工。

--org-emails 接受逗号分隔的邮箱，也可重复传入。上限按去重前数量计算。
操作人 uid 和组织 orgId 由平台根据当前登录身份注入，无需传入。
服务端去除首尾空格后忽略大小写去重，保留首次出现的地址及顺序；不补全域名、不转换邮箱别名。
空元素单独记为失败；后端因非法邮箱拒绝整批时，CLI 自动逐项查询，保留其他邮箱的结果。
超过 100 个邮箱时请分批调用；鉴权、权限或连接故障不会触发逐项兜底。
逐项兜底期间发生全局故障时保留原错误分类和退出码；结构化错误通过 error.details.partialResult 保留已确认结果。
取消操作会立即交回框架处理；取消和原始 PAT 授权协议保持不变。

返回 success、result、errorCode、errorMsg。result.users 是匹配到的员工数组，字段与 mail user get 一致；
result.notFoundOrgEmails 是未找到有效员工的邮箱数组。两个数组均按去重后的请求顺序返回。
全部未找到时 users=[]，全部找到时 notFoundOrgEmails=[]；未匹配邮箱不表示调用失败。
部分失败时 outcome=partial_failure（退出码 7），data.succeeded 保留已确认查询（found 和 user），
data.failed 列出逐项错误，data.unknown 列出返回缺失或异常而无法确认的邮箱；不得当作未找到。
staffId 即组织内 userId，不是工号；uid 是钉钉全局用户 ID。
单个邮箱可用 mail user get；只有姓名或工号时使用 mail user search。`,
		Example:       "  dws mail user batch-get --org-emails alice@example.com,bob@example.com",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        "mail",
		Tool:          "batch_get_users_by_org_emails",
		Flags: []LeafFlag{
			{Name: "org-emails", Usage: "1 至 100 个完整企业邮箱，逗号分隔或重复传入 (必填)；上限按去重前数量计算，单项错误不丢弃其他结果", Kind: LeafStringSlice, Required: true, MarkRequired: true, Bind: "orgEmails"},
		},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "mail", Name: "batch_get_users_by_org_emails", CanonicalPath: "mail.batch_get_users_by_org_emails",
				CLIPath: "mail user batch-get", PrimaryCLIPath: "mail user batch-get",
			},
			Description: "根据 1 至 100 个完整企业邮箱批量精确查询当前组织内员工，返回匹配员工与未匹配邮箱",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode: "mcp", Availability: "available",
				Ref: &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "batch_get_users_by_org_emails"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "org-emails", Property: "orgEmails", InterfaceType: "array", Required: boolPtr(true)},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "用一批完整企业邮箱查询当前组织内员工，并列出未匹配的邮箱",
				UseWhen:      []string{"已知多个完整企业邮箱，需要一次查询对应员工的 uid、staffId 和姓名并区分未匹配项时"},
				AvoidWhen:    []string{"只有姓名或工号时使用 mail user search；单个邮箱可用 mail user get；不得猜测邮箱或跨组织查询"},
				Examples:     []string{"dws mail user batch-get --org-emails alice@example.com,bob@example.com"},
			},
			Result: &contract.ResultSpec{
				Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePartialFailure, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{
					"type":"object",
					"description":"成功时返回 result；部分失败时返回 total/succeeded/failed/unknown，保留已确认的员工和未匹配结果",
					"properties":{
						"success":{"type":"boolean","description":"整批查询是否成功，不代表所有邮箱均匹配员工"},
						"result":{"type":"object","description":"按去重后的请求顺序返回匹配员工和未匹配邮箱",
							"properties":{
								"users":{"type":"array","description":"匹配到的有效员工；全部未找到时为空数组",
									"items":{"type":"object","properties":{
										"uid":{"type":["integer","null"],"description":"员工的钉钉全局 UID，不同于组织内 userId/staffId"},
										"orgId":{"type":["integer","null"],"description":"当前查询范围的钉钉组织 ID"},
										"staffId":{"type":["string","null"],"description":"员工在组织内的 userId（staffId），不是工号"},
										"name":{"type":["string","null"],"description":"员工在企业通讯录中的姓名"},
										"orgEmail":{"type":["string","null"],"description":"员工登记的完整企业邮箱，可忽略大小写与请求邮箱关联"}
									}}},
								"notFoundOrgEmails":{"type":"array","description":"未匹配有效员工的邮箱，保留去空格后首次出现的地址；全部找到时为空数组","items":{"type":"string"}}
							},"required":["users","notFoundOrgEmails"]},
						"errorCode":{"type":["string","null"],"description":"整批查询失败时的错误码"},
						"errorMsg":{"type":["string","null"],"description":"整批查询失败的原因"},
						"total":{"type":"integer","description":"部分失败时的查询项数；邮箱忽略大小写去重，空元素按原下标单独计数"},
						"succeeded":{"type":"array","description":"已确认的查询，包括找到和确认未找到的邮箱","items":{"type":"object","properties":{
							"id":{"type":"string","description":"首次出现的去空格后请求邮箱"},
							"orgEmail":{"type":"string","description":"本项请求邮箱"},
							"found":{"type":"boolean","description":"是否找到员工；false 表示已确认未匹配"},
							"user":{"description":"找到时的员工信息","$ref":"#/properties/result/properties/users/items"}
						},"required":["id","orgEmail","found"]}},
						"failed":{"type":"array","description":"明确失败的查询项","items":{"type":"object","properties":{
							"id":{"type":"string","description":"请求邮箱；空元素使用 org-emails[原始零起始下标]"},
							"error":{"type":"object","description":"该项错误","properties":{
								"type":{"type":"string","description":"错误类别"},
								"message":{"type":"string","description":"失败原因"}
							}}
						},"required":["id","error"]}},
						"unknown":{"type":"array","description":"返回缺失或异常、无法确认查询结果的邮箱；不能当成未找到","items":{"type":"object","properties":{
							"id":{"type":"string","description":"请求邮箱"},
							"reason":{"type":"string","description":"无法确认的原因"}
						},"required":["id","reason"]}}
					},"oneOf":[{"required":["success","result"]},{"required":["total","succeeded","failed","unknown"]}]
				}`),
				SensitivePaths: []string{"result.users.uid", "result.users.staffId", "result.users.name", "result.users.orgEmail", "result.notFoundOrgEmails", "succeeded.id", "succeeded.orgEmail", "succeeded.user", "failed.id", "unknown.id"},
			},
		},
		Validate:   validateMailUserBatchGet,
		ResultCall: callMailUserBatchLookupResult,
	})
}

func validateMailUserBatchGet(cmd *cobra.Command, _ []string) error {
	emails, err := cmd.Flags().GetStringSlice("org-emails")
	if err != nil {
		return err
	}
	if len(emails) < 1 || len(emails) > 100 {
		return apperrors.NewValidation("--org-emails must contain 1 to 100 addresses before deduplication")
	}
	return nil
}

func callMailUserLookupResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	if deps.Caller.DryRun() {
		return output.Success(map[string]any{"tool": tool, "arguments": args, "executed": false}, output.WithDryRun()), nil
	}
	data, err := fetchMailUserLookupData(cmd, tool, args)
	if err != nil {
		return nil, err
	}
	return output.Success(data), nil
}

func fetchMailUserLookupData(cmd *cobra.Command, tool string, args map[string]any) (map[string]json.RawMessage, error) {
	text, err := callMCPToolReturnTextOnServer(cmd.Context(), "mail", tool, args)
	if err != nil {
		return nil, err
	}
	// Preserve integer IDs and mapped null/empty employee results exactly.
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &data); err != nil || data == nil {
		return nil, apperrors.NewAPI(tool + " returned an invalid result object")
	}
	var success bool
	if err := json.Unmarshal(data["success"], &success); err != nil || !success {
		return nil, apperrors.NewAPI(tool + " did not return success=true")
	}
	return data, nil
}
