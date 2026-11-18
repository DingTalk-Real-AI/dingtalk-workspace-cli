// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf16"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// 新工具沿用已有 sport 路由；操作人及企业身份由平台注入，不声明为业务 flag。
func sportMutationSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent"}
}

func sportServiceContract(tool, path, group, summary, avoid, example, resultSchema string, params ...contract.ParamDecl) corecmd.ContractDecl {
	return corecmd.ContractDecl{
		Identity: sportIdentity(tool, path, group), Description: summary,
		Interface: sportInterface(tool), Parameters: params,
		DryRun:    &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest},
		Selection: contract.SelectionSpec{AgentSummary: summary, UseWhen: []string{summary}, AvoidWhen: []string{avoid}, Examples: []string{example}},
		Result: &contract.ResultSpec{
			Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
			DataSchema: json.RawMessage(fmt.Sprintf(`{"type":"object","description":"服务端 ServiceResult，受理不等于业务完成","properties":{"success":{"type":"boolean","description":"服务调用是否成功"},"result":%s,"errorCode":{"type":["string","null"],"description":"服务端错误码"},"errorMsg":{"type":["string","null"],"description":"服务端错误信息"}}}`, resultSchema)),
		},
	}
}

func sportTypedParam(name, property, description, kind string, required bool) contract.ParamDecl {
	return contract.ParamDecl{Name: name, Property: property, Description: description, InterfaceType: kind, Required: sportBoolPtr(required)}
}

const sportBooleanResultSchema = `{"type":"boolean","description":"操作回执；邀请不保证送达，编辑已结束比赛可能不生效，步数报表仅表示受理"}`

func newSportCompetitionInviteCmd() *cobra.Command {
	const example = "dws sport competition invite --rule-code RULE123 --receiver-user-id 00123 --format json"
	return NewLeafCommand(LeafSpec{
		Use: "invite", Short: "发送比赛邀请（无幂等）",
		Long:    "向员工私聊或目标群发送比赛邀请，receiver-user-id 与 target-group-id 至少一个，也可同时传。员工工号与数字群 cid 均保持字符串（含前导零）。真实 IM 推送，无幂等，执行前须用户确认；success=true/result=true 只表示受理，不保证送达。待审批比赛会改发群主审批提醒。不受灰度网关限制；创建后的开赛播报用 announce。操作人和企业由平台注入。",
		Example: example, OutputRollout: output.RolloutUnifiedActive, Server: sportServerID, Tool: "send_competition_invite_msg",
		ResultCall: sportServiceCallResult, Safety: sportMutationSafety(),
		Flags: []corecmd.FlagSpec{
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "真实比赛规则 code（必填）"},
			{Name: "receiver-user-id", Bind: "receiverUserId", Trim: true, OmitEmpty: true, Usage: "被邀请人的企业内员工工号，保留前导零"},
			{Name: "target-group-id", Bind: "targetGroupId", Trim: true, OmitEmpty: true, Usage: "目标群数字 cid 字符串"},
		},
		Constraints: []corecmd.Constraint{{Kind: corecmd.AtLeastOne, Flags: []string{"receiver-user-id", "target-group-id"}}},
		Contract: sportServiceContract("send_competition_invite_msg", "sport competition invite", "competition", "向已确认的员工或目标群发送比赛邀请", "不要重复发送；创建场景播报用 sport competition announce", example, sportBooleanResultSchema,
			sportTypedParam("rule-code", "ruleCode", "比赛规则 code", "string", true),
			sportTypedParam("receiver-user-id", "receiverUserId", "被邀请人工号，与目标群至少一个", "string", false),
			sportTypedParam("target-group-id", "targetGroupId", "目标群 cid，与被邀请人至少一个", "string", false)),
	})
}

func newSportCompetitionDeleteCmd() *cobra.Command {
	const example = "dws sport competition delete --rule-code RULE123 --format json"
	return NewLeafCommand(LeafSpec{
		Use: "delete", Short: "删除比赛（不可恢复，仅创建人）",
		Long:    "仅比赛创建人可删除，执行前必须确认目标及不可恢复影响。未开始时物理删除规则及全部实例并异步清参与关系；进行中截断结束日并软删各期；已结束仅软删。删除后比赛读路径不可见，用户历史打卡明细仍可查。重复删除被拒绝（活动已删除），不要自动重试；不受灰度网关限制。操作人和企业由平台注入。",
		Example: example, OutputRollout: output.RolloutUnifiedActive, Server: sportServerID, Tool: "delete_competition", ResultCall: sportServiceCallResult,
		Safety: contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent"},
		Flags:  []corecmd.FlagSpec{{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "待删除的真实比赛规则 code（必填）"}},
		Contract: sportServiceContract("delete_competition", "sport competition delete", "competition", "创建人删除已确认的比赛", "删除不可恢复；编辑用 update，不得用删除再创建代替编辑", example, sportBooleanResultSchema,
			sportTypedParam("rule-code", "ruleCode", "待删除比赛规则 code", "string", true)),
	})
}

func newSportCompetitionAuditCmd() *cobra.Command {
	const example = "dws sport competition audit --rule-code RULE123 --approved=false --format json"
	return NewLeafCommand(LeafSpec{
		Use: "audit", Short: "审批成员比赛（仅绑定群群主）",
		Long:    "仅绑定群群主可审批待审批比赛；approved 必须显式传 true 或 false，false 不能省略。true 同意后比赛生效并以创建人身份补发开赛广播（真实推送、无幂等）；false 拒绝且不进行参与同步。先核对比赛详情及审批结论并取得用户确认。非群主、非待审批或规则不存在会被拒绝。受灰度网关限制，非灰度企业可能报系统繁忙；勿重复审批。操作人和企业由平台注入。",
		Example: example, OutputRollout: output.RolloutUnifiedActive, Server: sportServerID, Tool: "audit_competition", ResultCall: sportServiceCallResult,
		Safety: sportMutationSafety(),
		Flags: []corecmd.FlagSpec{
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "待审批比赛规则 code（必填）"},
			{Name: "approved", Bind: "approved", Kind: corecmd.KindBool, Required: true, Usage: "必须显式指定：true 同意并广播，false 拒绝"},
		},
		Contract: sportServiceContract("audit_competition", "sport competition audit", "competition", "群主按已确认结论审批成员发起的比赛", "不是待审批比赛或不是群主时不要调用；不得缺省审批结论", example, sportBooleanResultSchema,
			sportTypedParam("rule-code", "ruleCode", "待审批比赛规则 code", "string", true), sportTypedParam("approved", "approved", "显式 true 同意或 false 拒绝", "boolean", true)),
	})
}

func newSportCompetitionUpdateCmd() *cobra.Command {
	const example = `dws sport competition update --request '{"ruleCode":"RULE123","competitionName":"步行挑战","coverImageUrl":"https://example.com/cover.png","competitionType":"FIXED","firstPeriodStartDate":"20261201","firstPeriodEndDate":"20261231","periodType":null,"cycleCount":null,"goalValue":6000,"targetDayValue":20,"dailyLimit":null,"extra":null}' --format json`
	return NewLeafCommand(LeafSpec{
		Use: "update", Short: "全量编辑比赛（仅创建人）",
		Long: `必须先用 competition rule-detail 查询当前值，改完字段后经用户确认，再完整回传 --request JSON 对象（也支持 @文件）。这是全量赋值，不是部分更新：CLI 要求显式包含以下 12 个键，可空字段用 null 表明清空，禁止猜测原值或仅传变更字段。
ruleCode、competitionName（≤128）、coverImageUrl（≤512）、competitionType（FIXED/RECURRING）、firstPeriodStartDate、firstPeriodEndDate、periodType（WEEK/MONTH/CUSTOM）、cycleCount（1~12）、goalValue、targetDayValue、dailyLimit、extra。数字必须为 JSON number，不能是字符串；extra 为字符串或 null。
未开始：应用全部可修改字段并删除重建实例；进行中：仅名称/头图/期数生效，其余结构字段忽略；已结束：空处理静默成功。成功后须回查详情，不能把 success=true 当成字段已生效。sportType 与报名开关创建后不可编辑，不得放入 request。FIXED 的循环参数为 null，结束日期必填；RECURRING 须有周期和期数，CUSTOM 须结束日期，MONTH 开始日须≤28。进行中期数不得小于当前期号，实际应用与状态相关约束由服务端校验。
受灰度网关限制，非灰度企业可能报系统繁忙。操作人和企业由平台注入，不放入 request。`,
		Example: example, OutputRollout: output.RolloutUnifiedActive, Server: sportServerID, Tool: "update_competition", ResultCall: sportServiceCallResult,
		Safety: sportMutationSafety(),
		Flags:  []corecmd.FlagSpec{{Name: "request", Bind: "request", Required: true, Trim: true, Input: []string{corecmd.InputFile}, Transform: sportParseUpdateRequest, Usage: "查询详情后完整回传的 12 字段 JSON 对象；可空字段须显式 null；支持 @文件"}},
		Contract: sportServiceContract("update_competition", "sport competition update", "competition", "比赛创建人以完整原值及确认后的修改编辑比赛", "禁止部分更新、猜测原值或自动重复调用；成功后回查实际生效字段", example, sportBooleanResultSchema,
			sportTypedParam("request", "request", "完整 CompetitionUpdateMcpRequest 对象，字段要求见帮助；不是 JSON 字符串透传", "object", true)),
	})
}

// 同时用于入参和新工具回执，保留 Long/小数原文，禁止多个 JSON 值及非对象根。
func sportDecodeObject(raw string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("必须是非 null 的 JSON 对象")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("必须恰好包含一个 JSON 对象")
	}
	return value, nil
}

func sportParseUpdateRequest(raw string) (any, error) {
	request, err := sportDecodeObject(raw)
	if err != nil {
		return nil, apperrors.NewValidation("--request 解析失败：" + err.Error())
	}
	fields := []string{"ruleCode", "competitionName", "coverImageUrl", "competitionType", "firstPeriodStartDate", "firstPeriodEndDate", "periodType", "cycleCount", "goalValue", "targetDayValue", "dailyLimit", "extra"}
	for _, key := range fields {
		if _, exists := request[key]; !exists {
			return nil, apperrors.NewValidation("--request 必须完整回传，缺少 " + key + "；可空字段使用 null")
		}
	}
	if len(request) != len(fields) {
		return nil, apperrors.NewValidation("--request 含未知或不可编辑字段；不要传操作人、sportType 或报名开关")
	}
	for _, key := range []string{"ruleCode", "competitionName", "coverImageUrl", "competitionType", "firstPeriodStartDate"} {
		value, ok := request[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return nil, apperrors.NewValidation("--request." + key + " 必须是非空字符串")
		}
	}
	for _, field := range []struct {
		key string
		max int
	}{{"competitionName", 128}, {"coverImageUrl", 512}} {
		if len(utf16.Encode([]rune(request[field.key].(string)))) > field.max {
			return nil, apperrors.NewValidation(fmt.Sprintf("--request.%s 长度不得超过 %d", field.key, field.max))
		}
	}
	for _, key := range []string{"firstPeriodEndDate", "periodType", "extra"} {
		if _, ok := request[key].(string); request[key] != nil && !ok {
			return nil, apperrors.NewValidation("--request." + key + " 必须是字符串或 null")
		}
	}
	for _, key := range []string{"goalValue", "targetDayValue", "dailyLimit", "cycleCount"} {
		if request[key] == nil && (key == "dailyLimit" || key == "cycleCount") {
			continue
		}
		number, ok := request[key].(json.Number)
		if !ok {
			return nil, apperrors.NewValidation("--request." + key + " 必须是 JSON number")
		}
		value, err := number.Float64()
		if err != nil || math.IsInf(value, 0) || value <= 0 || (key == "targetDayValue" && math.Trunc(value) != value) || (key == "cycleCount" && (math.Trunc(value) != value || value > 12)) {
			return nil, apperrors.NewValidation("--request." + key + " 须大于0；达标天数须整数，期数须1~12整数")
		}
	}
	start := request["firstPeriodStartDate"].(string)
	if err := sportValidateDate("request.firstPeriodStartDate", start); err != nil {
		return nil, err
	}
	end, _ := request["firstPeriodEndDate"].(string)
	if end != "" {
		if err := sportValidateDate("request.firstPeriodEndDate", end); err != nil {
			return nil, err
		}
		if end < start {
			return nil, apperrors.NewValidation("--request 首期结束日期不得早于开始日期")
		}
	}
	switch request["competitionType"] {
	case "FIXED":
		if request["periodType"] != nil || request["cycleCount"] != nil || end == "" {
			return nil, apperrors.NewValidation("FIXED 须有结束日期，periodType/cycleCount 必须为 null")
		}
	case "RECURRING":
		period, _ := request["periodType"].(string)
		if request["cycleCount"] == nil || (period != "WEEK" && period != "MONTH" && period != "CUSTOM") || (period == "CUSTOM" && end == "") || (period == "MONTH" && start[6:] > "28") {
			return nil, apperrors.NewValidation("RECURRING 须有 WEEK/MONTH/CUSTOM 与期数；CUSTOM 须结束日期，MONTH 开始日≤28")
		}
	default:
		return nil, apperrors.NewValidation("competitionType 仅支持 FIXED/RECURRING")
	}
	if request["competitionType"] == "FIXED" || request["periodType"] == "CUSTOM" {
		days := (sportParseDate(end).Unix()-sportParseDate(start).Unix())/86400 + 1
		target, _ := request["targetDayValue"].(json.Number).Float64()
		if target > float64(days) {
			return nil, apperrors.NewValidation("--request.targetDayValue 不得超过单期实际天数")
		}
	}
	return request, nil
}
