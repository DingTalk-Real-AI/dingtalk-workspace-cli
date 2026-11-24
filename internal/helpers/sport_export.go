// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

var sportExportNow = time.Now

func newSportCompetitionExportCmd() *cobra.Command {
	const example = "dws sport competition export --competition-code COMP456 --export-scope STAFF --staff-ids 00123,00456 --format json"
	decl := sportServiceContract("submit_competition_export", "sport competition export", "competition", "创建人提交指定期比赛的数据导出任务", "未开始比赛不可导出；勿重复提交，已有任务用 export-task 查询；步数报表用 step export", example,
		`{"type":"integer","description":"已受理的导出任务 taskId，用 export-task 查询；不是文件已生成"}`,
		sportTypedParam("competition-code", "competitionCode", "比赛实例 code", "string", true),
		sportTypedParam("export-scope", "exportScope", "ALL 全员或 STAFF 指定员工，缺省 ALL", "string", false),
		sportTypedParam("staff-ids", "staffIds", "STAFF 时必填，逗号分隔员工工号字符串", "string", false))
	decl.Result.Outcomes = append(decl.Result.Outcomes, contract.ResultOutcomePending)
	return NewLeafCommand(LeafSpec{
		Use: "export", Short: "提交比赛数据导出任务",
		Long:    "异步导出指定期比赛的数据，返回 taskId 后须用 competition export-task 查询。PROCESSING 不是失败，勿重复提交。仅比赛创建人可提交（即使失败文案写群主/管理员，实际权限仍是创建人），缺省每人每天最多5次，比赛未开始不可导出。ALL 导全员；STAFF 必须提供非空 staff-ids，工号保持字符串和前导零，服务端 trim/去重。文件后台上传钉盘并推送操作人钉钉，受理不等于完成。操作人和企业由平台注入，执行前须确认导出范围。",
		Example: example, OutputRollout: output.RolloutUnifiedActive, Server: sportServerID, Tool: "submit_competition_export", ResultCall: sportServiceCallResult,
		Safety: sportMutationSafety(), Contract: decl, Validate: sportValidateCompetitionExport,
		Flags: []corecmd.FlagSpec{
			{Name: "competition-code", Bind: "competitionCode", Required: true, Trim: true, Usage: "待导出比赛实例 code（必填）"},
			{Name: "export-scope", Bind: "exportScope", Default: "ALL", Enum: []string{"ALL", "STAFF"}, Trim: true, Usage: "ALL 全员（缺省）或 STAFF 指定员工"},
			{Name: "staff-ids", Bind: "staffIds", OmitEmpty: true, RequiredWhen: "export-scope=STAFF", Usage: "逗号分隔员工工号，STAFF 时必填，保留前导零"},
		},
	})
}

func newSportCompetitionExportTaskCmd() *cobra.Command {
	const example = "dws sport competition export-task --task-id 12345 --format json"
	decl := sportServiceContract("get_competition_export_task", "sport competition export-task", "competition", "查询已受理比赛导出任务的状态和下载链接", "仅查当前企业的已知 taskId；步数报表没有任务查询接口；不得自行抓取钉盘链接", example,
		`{"type":"object","description":"导出任务详情","properties":{"taskId":{"type":"integer","description":"任务 ID"},"status":{"type":"string","enum":["PROCESSING","SUCCESS","FAILED"],"description":"处理中、成功或失败"},"recordCount":{"type":["number","null"],"description":"成功时导出记录数"},"ossUrl":{"type":["string","null"],"description":"汇总钉盘下载链接，需登录态，原样交付"},"fileName":{"type":["string","null"],"description":"导出文件名"},"errorMsg":{"type":["string","null"],"description":"任务失败原因"},"dailyOssUrls":{"type":["array","null"],"description":"每日明细链接列表，超过30天可按自然月拆分，无文件时为null","items":{"type":"string"}}}}`,
		sportTypedParam("task-id", "taskId", "提交比赛导出时返回的正整数 Long taskId", "number", true))
	decl.Result.Outcomes = append(decl.Result.Outcomes, contract.ResultOutcomePending)
	return NewLeafCommand(LeafSpec{
		Use: "export-task", Short: "查询比赛导出任务与钉盘链接",
		Long:    "按提交导出返回的 taskId 点查当前企业任务，无操作人参数。PROCESSING 返回 pending，稍后查询同一 taskId；SUCCESS 返回成功，ossUrl 与 dailyOssUrls（可能为 null）为需要钉钉登录态的钉盘链接，须原样交付用户点击，不自行下载解析；FAILED 返回失败及原因。任务不存在报错，不自动重新提交导出。仅用于比赛导出，步数报表没有 taskId。查询时保持与提交相同的企业/profile。",
		Example: example, OutputRollout: output.RolloutUnifiedActive, Server: sportServerID, Tool: "get_competition_export_task", ResultCall: sportServiceCallResult,
		Safety: sportReadLeaf(), Contract: decl,
		Flags: []corecmd.FlagSpec{{Name: "task-id", Bind: "taskId", Required: true, Trim: true, Transform: sportParseTaskID, Usage: "真实导出任务 ID，正整数 Long，保持精度（必填）"}},
	})
}

func newSportStepExportCmd() *cobra.Command {
	const example = "dws sport step export --stat-date 20261001 --end-date 20261010 --format json"
	return NewLeafCommand(LeafSpec{
		Use: "export", Short: "提交企业步数报表，异步投递钉钉",
		Long:    "提交企业步数报表异步导出，true 仅表示受理成功，不是文件已生成。无 taskId、无进度查询接口，文件由平台异步投递操作人钉钉；应告知用户留意消息，不在对话中等待文件或重复提交。一人一天一行（日期/部门/姓名/工号/步数），不含排名。日期 yyyyMMdd，含首尾最多31天，结束日期必须早于今天（CLI 按本机日期预检，最终以服务端日期为准）。staff-ids 不传或空串表示全员；非空传逗号分隔工号，保留前导零。生产仅企业主/子管理员可导出，预发放行不代表生产有权限。缺省同企业10分钟冷却，开关/权限/限流由服务端判断。操作人及企业由平台注入，执行前须确认范围。",
		Example: example, OutputRollout: output.RolloutUnifiedActive, Server: sportServerID, Tool: "submit_step_report_export", ResultCall: sportServiceCallResult,
		Safety: sportMutationSafety(), Validate: sportValidateStepExport,
		Flags: []corecmd.FlagSpec{
			{Name: "stat-date", Bind: "statDate", Required: true, Trim: true, Usage: "开始日期 yyyyMMdd（含当天，必填）"},
			{Name: "end-date", Bind: "endDate", Required: true, Trim: true, Usage: "结束日期 yyyyMMdd（含当天，须早于今天，必填）"},
			{Name: "staff-ids", Bind: "staffIds", OmitEmpty: true, Usage: "逗号分隔员工工号；不传或空串导出全员"},
		},
		Contract: sportServiceContract("submit_step_report_export", "sport step export", "step", "企业管理员提交历史步数报表并接收钉钉文件", "不含排名；当天数据不可导出；没有任务查询，不等待文件；比赛数据用 competition export", example, sportBooleanResultSchema,
			sportTypedParam("stat-date", "statDate", "开始日期 yyyyMMdd", "string", true), sportTypedParam("end-date", "endDate", "结束日期 yyyyMMdd，早于今天，最多31天", "string", true),
			sportTypedParam("staff-ids", "staffIds", "逗号分隔工号，空串表示全员", "string", false)),
	})
}

func sportValidateStaffIDs(value string) error {
	for _, id := range strings.Split(value, ",") {
		if strings.TrimSpace(id) != "" {
			return nil
		}
	}
	return apperrors.NewValidation("--staff-ids 必须包含至少一个非空工号，不能仅含空白或逗号")
}

func sportValidateCompetitionExport(cmd *cobra.Command, _ []string) error {
	ids := MustGetStringFlag(cmd, "staff-ids")
	if MustGetStringFlag(cmd, "export-scope") == "STAFF" || ids != "" {
		return sportValidateStaffIDs(ids)
	}
	return nil
}

func sportValidateStepExport(cmd *cobra.Command, _ []string) error {
	start, end := MustGetStringFlag(cmd, "stat-date"), MustGetStringFlag(cmd, "end-date")
	for field, value := range map[string]string{"stat-date": start, "end-date": end} {
		if err := sportValidateDate(field, value); err != nil {
			return err
		}
	}
	if end < start || (sportParseDate(end).Unix()-sportParseDate(start).Unix())/86400+1 > 31 {
		return apperrors.NewValidation("步数导出日期不得倒挂，含首尾最多31天")
	}
	if end >= sportExportNow().Format("20060102") {
		return apperrors.NewValidation("--end-date 必须早于今天，当天数据不可导出")
	}
	if ids := MustGetStringFlag(cmd, "staff-ids"); ids != "" {
		return sportValidateStaffIDs(ids)
	}
	return nil
}

func sportParseTaskID(raw string) (any, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return nil, apperrors.NewValidation("--task-id 必须是正整数 Long，不能是小数或超出 int64 范围")
	}
	return id, nil
}

// 新工具回执有明确的 ServiceResult 契约；调用仍走统一错误分类，单独解码以免 Long 被 float64 截断。
func sportServiceCallResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	caller := GetCaller()
	if caller == nil || corecmd.BoolFlag(cmd, "dry-run") || caller.DryRun() {
		return sportCallResult(cmd, tool, args)
	}
	text, err := CallMCPToolTextOnServerContext(cmd.Context(), sportServerID, tool, args)
	if err != nil {
		return nil, err
	}
	body, err := sportDecodeObject(text)
	if err != nil {
		return nil, apperrors.NewAPI(tool + " 返回了无效 ServiceResult：" + err.Error() + "；不要自动重放写操作")
	}
	if body["success"] != true {
		return nil, apperrors.NewAPI(tool + " 未返回 success=true，无法确认操作成功；不要自动重放")
	}
	switch tool {
	case "submit_competition_export":
		id, ok := sportResponseTaskID(body["result"])
		if !ok {
			return nil, apperrors.NewAPI("导出受理回执缺少有效 taskId；勿重复提交")
		}
		return output.Pending(body, sportExportOperation(cmd, id)), nil
	case "get_competition_export_task":
		task, ok := body["result"].(map[string]any)
		id, valid := sportResponseTaskID(task["taskId"])
		if !ok || !valid || id != fmt.Sprint(args["taskId"]) {
			return nil, apperrors.NewAPI("导出查询回执缺少或不匹配 taskId")
		}
		switch task["status"] {
		case "PROCESSING":
			return output.Pending(body, sportExportOperation(cmd, id)), nil
		case "SUCCESS":
			if link, _ := task["ossUrl"].(string); strings.TrimSpace(link) == "" {
				return nil, apperrors.NewAPI("导出任务标记成功但没有汇总下载链接，请查询同一 taskId 核实")
			}
			return output.Success(body), nil
		case "FAILED":
			message, _ := task["errorMsg"].(string)
			return nil, apperrors.NewAPI("比赛导出任务失败："+message, apperrors.WithOperation("sport/export-task/"+id))
		default:
			return nil, apperrors.NewAPI("导出任务返回未知或缺失状态，请查询同一 taskId 核实")
		}
	default:
		if body["result"] != true {
			return nil, apperrors.NewAPI(tool + " 未返回 result=true，无法确认操作已受理；不要自动重放")
		}
		// 步数报表没有任务ID和查询接口；success 仅指提交已受理，不编造可轮询 pending。
		return output.Success(body), nil
	}
}

func sportResponseTaskID(value any) (string, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return "", false
	}
	id, err := number.Int64()
	return strconv.FormatInt(id, 10), err == nil && id > 0
}

func sportExportOperation(cmd *cobra.Command, id string) *output.OperationInfo {
	next := "dws sport competition export-task --task-id " + id + " --format json"
	if flag := cmd.Flag("profile"); flag != nil && flag.Value.String() != "" {
		next += " --profile '" + strings.ReplaceAll(flag.Value.String(), "'", "'\\''") + "'"
	}
	return &output.OperationInfo{ID: id, State: "PROCESSING", NextCommand: next}
}
