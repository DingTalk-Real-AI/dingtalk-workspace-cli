// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// 本批仅注册未接入的只读工具；已有配置、期次和统计命令保持原路径及契约。
func sportReadCommand(use, tool, summary, detail, avoid, example, resultSchema string, flags ...corecmd.FlagSpec) *cobra.Command {
	params := make([]contract.ParamDecl, 0, len(flags))
	for _, flag := range flags {
		kind := "string"
		if flag.Kind == corecmd.KindBool {
			kind = "boolean"
		} else if flag.Kind == corecmd.KindInt || flag.Name == "cursor" {
			kind = "number"
		}
		params = append(params, sportTypedParam(flag.Name, flag.Bind, flag.Usage, kind, flag.Required))
	}
	decl := sportServiceContract(tool, "sport competition "+use, "competition", summary, avoid, example, resultSchema, params...)
	for _, flag := range flags {
		if flag.Name == "cursor" {
			decl.Pagination = &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "cursor"}
		}
	}
	return NewLeafCommand(LeafSpec{
		Use: use, Short: summary, Long: detail + " 只读，无需确认；企业及当前用户身份由平台注入，不接受操作人 user-id/corp-id。",
		Example: example, Server: sportServerID, Tool: tool, OutputRollout: output.RolloutUnifiedActive,
		Safety: sportReadLeaf(), Contract: decl, Flags: flags, Validate: sportValidateReadLimits, ResultCall: sportReadServiceCallResult,
	})
}

func sportReadPageFlags(sizeFlag, sizeProperty, cursorDefault string) []corecmd.FlagSpec {
	return []corecmd.FlagSpec{
		{Name: "cursor", Bind: "cursor", Default: cursorDefault, Trim: true, Transform: sportParseReadCursor, Usage: "Long 游标，翻页原样回传 meta.pagination.next_token；个人列表首页不传或传 null，企业列表首页 0"},
		{Name: sizeFlag, Bind: sizeProperty, Kind: corecmd.KindInt, Default: "20", Usage: "每页条数，CLI 默认20；企业比赛列表1~100，个人列表及排行榜1~50"},
	}
}

func newSportCompetitionListCmd() *cobra.Command {
	return sportReadCommand("list", "list_competition_configs", "分页查询企业启用中的比赛",
		"企业比赛索引入口，按创建时间倒序列出启用中的配置；不含审批中、已拒绝或已删除比赛。行内 code 即 ruleCode，可继续查询 config/instances。空列表不是失败。首页 cursor=0，每页1~100。每次只取一页：data.result.list 为列表，源 hasMore/nextCursor 转为 meta.pagination.endpoint_exhausted/next_token；空页不代表结束，以 endpoint_exhausted 为准，无 total。",
		"我的创建/参与视角用 my-list；已知 ruleCode 的配置详情用 config，不要用创建命令查找比赛",
		"dws sport competition list --cursor 0 --size 20 --format json", sportReadPageSchema(sportConfigRowSchema), sportReadPageFlags("size", "size", "0")...)
}

func newSportCompetitionRankTopCmd() *cobra.Command {
	return sportReadCommand("rank-top", "get_competition_rank_top_n", "查询比赛 TopN 排行榜",
		"查询一期比赛排行榜，名次升序；rank-type 为 DAY/WEEK/MONTH/TOTAL（本期总榜），不同于 stats reached，后者不支持 TOTAL。sort-by-reached-times=true 按达标次数排序，缺省 false 按运动量主指标排序。top-n 为1~50、默认10。competition-code 缺省取当前进行中的一期。虽然描述称指定期次可不传规则，本批 MCP Schema 将 ruleCode 无条件必填，CLI 保留必填。userId 是 staffId，查询失败可降级 null；空数组不是失败。",
		"全员游标分页榜用 ranking；人数统计用 stats，不得把 TopN 当全员名单",
		"dws sport competition rank-top --rule-code RULE123 --rank-type TOTAL --top-n 10 --format json", `{"type":"array","description":"TopN 榜单，空数组是合法结果","items":`+sportRankTopRowSchema+`}`,
		corecmd.FlagSpec{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "真实比赛规则 code（MCP Schema 必填）"},
		corecmd.FlagSpec{Name: "rank-type", Bind: "rankType", Required: true, Trim: true, Enum: []string{"DAY", "WEEK", "MONTH", "TOTAL"}, Usage: "DAY/WEEK/MONTH/TOTAL，本期总榜支持 TOTAL"},
		corecmd.FlagSpec{Name: "sort-by-reached-times", Bind: "sortByReachedTimes", Kind: corecmd.KindBool, Usage: "true 按达标次数排序；缺省 false 按运动量排序"},
		corecmd.FlagSpec{Name: "top-n", Bind: "topN", Kind: corecmd.KindInt, Default: "10", Usage: "返回条数1~50，默认10"},
		corecmd.FlagSpec{Name: "competition-code", Bind: "competitionCode", Trim: true, OmitEmpty: true, Usage: "指定期次 code；缺省为当前进行中的一期"})
}

func newSportCompetitionMyListCmd() *cobra.Command {
	flags := append([]corecmd.FlagSpec{{Name: "list-scope", Bind: "listScope", Default: "JOINED", Trim: true, Enum: []string{"JOINED", "CREATED", "CAN_JOIN"}, Usage: "JOINED 我参与的（缺省）/CREATED 我创建的/CAN_JOIN 可参与的；切换须重置 cursor"}}, sportReadPageFlags("size", "size", "")...)
	return sportReadCommand("my-list", "list_my_competition_rules", "分页查询我的比赛规则列表",
		"按 JOINED/CREATED/CAN_JOIN 查询我的组织比赛，一条规则无论多少期只出一行。首页不传 cursor 或显式传 null；切换 list-scope 必须重置游标，三视角游标不可混用。每页1~50，CLI 默认20。返回 data.result.list，分页读取 meta.pagination；无 total，可参与数量用 my-can-join-count。ruleCode 可串接详情；导出必须用 competitionCode（可能为 null），不能拿规则 code 代替。canEdit/canInvite/canExport 仅是权限提示，不能代替写操作确认或实际服务端校验。空列表不是失败。",
		"企业启用配置列表用 list；逐期参与记录用 my-participated；此命令不会报名或参与比赛",
		"dws sport competition my-list --list-scope CREATED --size 20 --format json", sportReadPageSchema(sportMyRuleRowSchema), flags...)
}

func newSportCompetitionMyCanJoinCountCmd() *cobra.Command {
	return sportReadCommand("my-can-join-count", "count_my_can_join_competitions", "查询我可参与的比赛数量",
		"返回当前用户可参与比赛数量，是 my-list --list-scope CAN_JOIN 的配套计数。0 是正常业务结果，不代表失败；本命令不执行报名。",
		"可参与比赛明细用 my-list --list-scope CAN_JOIN；已参与总数用 my-participated-count",
		"dws sport competition my-can-join-count --format json", sportReadCountSchema)
}

func newSportCompetitionActivityDetailCmd() *cobra.Command {
	return sportReadCommand("activity-detail", "get_competition_activity_detail", "查询比赛活动详情与我的报名状态",
		"当前用户视角的活动详情，含 signedUp 报名状态、isCreator/canApprove 和循环赛 periodList。competition-code 可选，缺省取当期；非循环赛 periodList 可为 null。canApprove 不是审批结果，也不能代替审批确认；本命令不报名、不审批。活动不存在按服务端 QW_COMPETITION_QUERY_FAILED 报错。",
		"完整编辑原值用 rule-detail；企业配置详情用 config；个人达标进度用 my-achievement",
		"dws sport competition activity-detail --rule-code RULE123 --format json", sportActivityDetailSchema,
		corecmd.FlagSpec{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "真实比赛规则 code（必填）"},
		corecmd.FlagSpec{Name: "competition-code", Bind: "competitionCode", Trim: true, OmitEmpty: true, Usage: "指定期次 code，可选，缺省取当期"})
}

func newSportCompetitionMyAchievementCmd() *cobra.Command {
	return sportReadCommand("my-achievement", "get_personal_achievement", "查询我的比赛达标进度",
		"查询当前用户在指定期次的今日进度、累计运动值、达标天数、名次及 weekReachList（周一至周日7条）。competition-code 必填，可从 my-list/current-instance 的真实返回取得。rankNo=-1 表示无排名，不是错误；todayProgress/cumulativeValue 为字符串，保留服务端口径。不能指定其他员工查询。",
		"企业参与达标统计用 stats；其他参赛者排名用 ranking；报名状态用 activity-detail",
		"dws sport competition my-achievement --competition-code COMP456 --format json", sportAchievementSchema,
		corecmd.FlagSpec{Name: "competition-code", Bind: "competitionCode", Required: true, Trim: true, Usage: "真实期次比赛 code（必填）"})
}

func newSportCompetitionMyParticipatedCmd() *cobra.Command {
	return sportReadCommand("my-participated", "list_my_participated_comp", "分页查询我参与过的比赛期次",
		"按比赛实例分页，含 relationStatus（0待加入/1已加入）和 achievedDays；后者仅已加入且进行中的期次填充，不能据未填充值推断历史成绩。首页 cursor 不传或传 null；page-size 为1~50，CLI 默认20。data.result.list 为本页记录，源 hasMore/nextCursor 转为 meta.pagination；无 total，总数用 my-participated-count。空列表不是失败，以 endpoint_exhausted 判断结束。",
		"按规则聚合的我的比赛用 my-list；本命令不执行参赛或报名",
		"dws sport competition my-participated --page-size 20 --format json", sportReadPageSchema(sportParticipatedRowSchema), sportReadPageFlags("page-size", "pageSize", "")...)
}

func newSportCompetitionMyParticipatedCountCmd() *cobra.Command {
	return sportReadCommand("my-participated-count", "count_my_participated_comp", "查询我参与过的比赛总数",
		"返回当前用户参与过的比赛总数，是 my-participated 的配套计数；列表分页不含 total，不能用单页行数代替总数。0 是正常业务结果。",
		"参与记录明细用 my-participated；可参与数量用 my-can-join-count",
		"dws sport competition my-participated-count --format json", sportReadCountSchema)
}

func newSportCompetitionRankingCmd() *cobra.Command {
	flags := append([]corecmd.FlagSpec{{Name: "competition-code", Bind: "competitionCode", Required: true, Trim: true, Usage: "真实期次比赛 code（必填）"}}, sportReadPageFlags("size", "size", "")...)
	return sportReadCommand("ranking", "list_competition_rank", "分页查询比赛全员排行榜",
		"指定一期比赛的全员排行榜，名次升序。competition-code 必填，首页 cursor 不传或传 null，每页1~50，CLI 默认20。data.result.list 为本页榜单，分页读取 meta.pagination，不自动翻页、没有 total。行内 userId 为 staffId，身份换取失败降级 null；累计运动值 cumulativeValue 是字符串。平台操作人只用于底层自身标记，当前出参不声明 isSelf，不能编造该字段。空列表不是失败；用户要求全量时按 next_token 续查直到 endpoint_exhausted。",
		"只查前几名或按 DAY/WEEK/MONTH/TOTAL 排序用 rank-top；个人进度用 my-achievement",
		"dws sport competition ranking --competition-code COMP456 --size 20 --format json", sportReadPageSchema(sportRankingRowSchema), flags...)
}

func sportParseReadCursor(raw string) (any, error) {
	if raw == "" || raw == "null" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, apperrors.NewValidation("--cursor 必须是原样回传的 Long 整数或 null，不能是小数或超出 int64 范围")
	}
	return value, nil
}

func sportValidateReadLimits(cmd *cobra.Command, _ []string) error {
	for _, name := range []string{"size", "page-size", "top-n"} {
		if cmd.Flags().Lookup(name) == nil {
			continue
		}
		value, _ := cmd.Flags().GetInt(name)
		max := 50
		if cmd.Name() == "list" {
			max = 100
		}
		if value < 1 || value > max {
			return apperrors.NewValidation(fmt.Sprintf("--%s 必须为1~%d", name, max))
		}
	}
	return nil
}

func sportReadServiceCallResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	paged := tool == "list_competition_configs" || tool == "list_my_competition_rules" || tool == "list_my_participated_comp" || tool == "list_competition_rank"
	if paged {
		if args == nil {
			args = map[string]any{}
		}
		if _, exists := args["cursor"]; !exists {
			// Transform 的 nil 会省略键；本批个人列表首页须显式发送 JSON null。
			args["cursor"] = nil
		}
	}
	caller := GetCaller()
	if caller == nil || corecmd.BoolFlag(cmd, "dry-run") || caller.DryRun() {
		return sportCallResult(cmd, tool, args)
	}
	text, err := CallMCPToolTextOnServerContext(cmd.Context(), sportServerID, tool, args)
	if err != nil {
		if tool == "is_org_admin" {
			return nil, classifySportOrgAdminChannelError(err)
		}
		return nil, err
	}
	body, err := sportDecodeObject(text)
	if err != nil {
		return nil, apperrors.NewAPI(tool + " 返回无效 ServiceResult（解码失败），不能把异常当成空结果")
	}
	success, present := body["success"]
	if !present {
		// 生产 app runner 会给缺 success 的对象回执补 success=true（全产品统一约定），
		// 本分支只拦截不经 runner 的直接 ToolCaller；服务端 ServiceResult 始终携带 success。
		return nil, apperrors.NewAPI(tool + " 返回无效 ServiceResult（缺少 success 字段），不能把异常当成空结果")
	}
	if success != true {
		return nil, apperrors.NewAPI(tool + " 返回无效 ServiceResult（success 非 true），不能把异常当成空结果")
	}
	if paged {
		page, ok := body["result"].(map[string]any)
		items, listOK := page["list"].([]any)
		more, moreOK := page["hasMore"].(bool)
		if !ok || !listOK || !moreOK {
			return nil, apperrors.NewAPI(tool + " 返回无效分页，必须包含 list 数组与 hasMore 布尔值")
		}
		next := ""
		if more {
			number, valid := page["nextCursor"].(json.Number)
			cursor, parseErr := number.Int64()
			if !valid || parseErr != nil || (args["cursor"] != nil && fmt.Sprint(args["cursor"]) == strconv.FormatInt(cursor, 10)) {
				return nil, apperrors.NewAPI(tool + " 声明有下一页，但 nextCursor 缺失、非法或未前进")
			}
			next = strconv.FormatInt(cursor, 10)
		}
		// 分页控制信息属于统一 meta，不在业务 Result 中保留第二套续页事实。
		body["result"] = map[string]any{"list": items}
		return output.Success(body, output.WithMeta(&output.Meta{Count: output.NewCount(len(items)), Pagination: &output.Pagination{EndpointExhausted: !more, NextToken: next, Pages: 1, Items: len(items)}})), nil
	}
	switch tool {
	case "is_org_admin":
		if _, ok := body["result"].(bool); !ok {
			return nil, apperrors.NewAPI(tool + " 缺少布尔 result 判定，不能当作非管理员")
		}
	case "get_competition_rank_top_n":
		if _, ok := body["result"].([]any); !ok {
			return nil, apperrors.NewAPI(tool + " 缺少 result 数组")
		}
	case "count_my_can_join_competitions", "count_my_participated_comp":
		number, ok := body["result"].(json.Number)
		value, err := number.Int64()
		if !ok || err != nil || value < 0 {
			return nil, apperrors.NewAPI(tool + " 缺少非负整数计数")
		}
	default:
		if _, ok := body["result"].(map[string]any); !ok {
			return nil, apperrors.NewAPI(tool + " 缺少 result 详情对象")
		}
	}
	return output.Success(body), nil
}

// classifySportOrgAdminChannelError 把统一调用通道提前分类的业务错误映射回
// is_org_admin 的三态语义：显式 success=false（如 QW_ADMIN_CHECK_FAILED）表示
// 组织管理员查询服务不可用，必须提示稍后重试，绝不能当作非管理员；其余错误（未
// 登录、网关、非业务错误等）原样返回。
//
// 生产链路中 success=false 在 app runner 已被 detectBusinessError 分类为
// reason=business_error 的 *apperrors.Error；直接实现 ToolCaller 的调用方则由
// parseMCPToolTextResult 分类为 *CLIError。两种形态都要识别。
func classifySportOrgAdminChannelError(err error) error {
	var appErr *apperrors.Error
	if errors.As(err, &appErr) && appErr.Reason == "business_error" {
		// 保留服务端诊断（Server Code、Trace），只替换面向用户的语义与恢复建议。
		classified := *appErr
		classified.Message = fmt.Sprintf("is_org_admin 调用失败（组织管理员查询不可用，errorMsg=%s）：不能当作非管理员，请稍后重试", appErr.Message)
		classified.Hint = sportOrgAdminRetryHint
		classified.Actions = append([]string{sportOrgAdminRetryHint}, appErr.Actions...)
		classified.Cause = err
		return &classified
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != CodeMCPToolError {
		return err
	}
	var body map[string]any
	if json.Unmarshal([]byte(cliErr.Message), &body) != nil {
		return err
	}
	if success, ok := body["success"].(bool); !ok || success {
		return err
	}
	return apperrors.NewAPI(fmt.Sprintf("is_org_admin 返回 success=false（组织管理员查询不可用，errorCode=%v，errorMsg=%v）：不能当作非管理员，请稍后重试", body["errorCode"], body["errorMsg"]), apperrors.WithHint(sportOrgAdminRetryHint))
}

const sportOrgAdminRetryHint = "组织管理员判定服务暂不可用：不能当作非管理员处理，请稍后重试"

func sportReadPageSchema(row string) string {
	return `{"type":"object","description":"当前页业务数据；分页控制信息见 meta.pagination，不含 total","properties":{"list":{"type":"array","description":"当前页记录，空列表是合法业务结果","items":` + row + `}}}`
}

const sportReadCountSchema = `{"type":"integer","description":"当前用户的比赛数量，0是合法结果","minimum":0}`

const sportConfigRowSchema = `{"type":"object","description":"企业启用中的比赛配置","properties":{
"name":{"type":"string","description":"比赛名称"},
"code":{"type":"string","description":"比赛配置 code，即后续查询的 ruleCode"},
"coverImageUrl":{"type":"string","description":"封面图地址"},
"groupId":{"type":"string","description":"绑定群标识"},
"groupName":{"type":"string","description":"创建时冗余的群名，不追踪改名"},
"competitionType":{"type":"string","description":"比赛类型"},
"sportType":{"type":"string","description":"运动项目"},
"periodType":{"type":["string","null"],"description":"周期类型"},
"cycleCount":{"type":["number","null"],"description":"循环比赛期数"},
"startDate":{"type":"string","description":"整体开始日期"},
"endDate":{"type":"string","description":"整体结束日期"},
"goalValue":{"type":"number","description":"每日目标值"},
"targetDayValue":{"type":"number","description":"达标天数目标"},
"dailyLimit":{"type":["number","null"],"description":"每日计数上限，null表示无上限"},
"needSignup":{"type":"boolean","description":"是否需要报名"},
"allowFreeJoin":{"type":"boolean","description":"是否允许自由加入"},
"signupCount":{"type":"number","description":"累计报名人数"},
"status":{"type":"number","description":"启用状态"}}}`

const sportRankTopRowSchema = `{"type":"object","description":"TopN 榜单行","properties":{
"userId":{"type":["string","null"],"description":"员工 staffId，保留前导零，换取失败为 null"},
"nickName":{"type":["string","null"],"description":"昵称，缺失可为空"},
"rankNo":{"type":"number","description":"名次，从1起"},
"itemData":{"type":"number","description":"主指标运动量：步数/米/千卡"},
"reachedTimes":{"type":"number","description":"达标次数"},
"calorie":{"type":"number","description":"卡路里"},
"durationMinutes":{"type":"number","description":"运动时长（分钟）"},
"competitionCode":{"type":"string","description":"实际定位到的期次 code"}}}`

const sportMyRuleRowSchema = `{"type":"object","description":"我的比赛规则，一条规则一行","properties":{
"ruleCode":{"type":"string","description":"规则 code"},
"competitionCode":{"type":["string","null"],"description":"当期 code，已结束取最大期号，未命中为 null"},
"competitionName":{"type":"string","description":"当期名称，无当期回落规则名称"},
"coverImageUrl":{"type":"string","description":"当期或规则头图"},
"sportType":{"type":"string","description":"运动项目"},
"startTime":{"type":"string","description":"规则开始日 yyyyMMdd"},
"endTime":{"type":"string","description":"规则结束日 yyyyMMdd"},
"periodStartDate":{"type":"string","description":"展示期开始日 yyyyMMdd"},
"periodEndDate":{"type":"string","description":"展示期结束日 yyyyMMdd"},
"competitionStatus":{"type":"string","description":"实时比赛状态"},
"groupId":{"type":"string","description":"绑定群 cid"},
"groupName":{"type":"string","description":"创建时冗余群名"},
"allowFreeJoin":{"type":"boolean","description":"是否允许自由加入"},
"participantCount":{"type":"number","description":"规则累计报名人数，只增不减"},
"canEdit":{"type":"boolean","description":"创建人编辑提示，已结束编辑可能空处理"},
"canInvite":{"type":"boolean","description":"创建人且允许自由加入且未结束"},
"canExport":{"type":"boolean","description":"创建人且活动已开始"},
"sessionId":{"type":"string","description":"会话 ID"}}}`

const sportActivityDetailSchema = `{"type":"object","description":"当前用户视角的比赛活动详情","properties":{
"ruleCode":{"type":"string","description":"规则 code"},
"competitionCode":{"type":"string","description":"当期活动 code"},
"title":{"type":"string","description":"活动标题"},
"coverImageUrl":{"type":"string","description":"头图 URL"},
"sportType":{"type":"string","description":"运动项目"},
"goalValue":{"type":"number","description":"每日目标：步数/米/卡路里"},
"targetDayValue":{"type":"number","description":"达标天数目标"},
"dailyLimit":{"type":["number","null"],"description":"每日上限，null表示无上限"},
"periodStartDate":{"type":"string","description":"本期开始日 yyyyMMdd"},
"periodEndDate":{"type":"string","description":"本期结束日 yyyyMMdd"},
"competitionStatus":{"type":"string","description":"当期状态：0未开始/1进行中/2已结束"},
"status":{"type":"number","description":"规则状态：0审批中/1启用/2已拒绝"},
"signedUp":{"type":"string","description":"SIGNED_UP已报名/CAN_SIGNUP可报名/CANNOT_SIGNUP不可报名"},
"sessionId":{"type":"string","description":"会话 ID"},
"groupName":{"type":"string","description":"创建时冗余群名"},
"isCreator":{"type":"boolean","description":"当前用户是否为创建人"},
"canApprove":{"type":"boolean","description":"当前用户是否为绑定群群主，不代表已审批"},
"allowFreeJoin":{"type":"boolean","description":"是否允许自由加入"},
"periodList":{"type":["array","null"],"description":"循环赛各期，期号升序，非循环赛为 null","items":{"type":"object","properties":{
"competitionCode":{"type":"string","description":"期比赛 code"},"title":{"type":"string","description":"本期标题"},
"periodStartDate":{"type":"string","description":"本期开始日 yyyyMMdd"},"periodEndDate":{"type":"string","description":"本期结束日 yyyyMMdd"},
"competitionStatus":{"type":"string","description":"本期状态：0未开始/1进行中/2已结束"}}}}}}`

const sportAchievementSchema = `{"type":"object","description":"当前用户个人达标数据","properties":{
"userId":{"type":"string","description":"当前用户 staffId，保留前导零"},
"nick":{"type":["string","null"],"description":"昵称，缺失可为空"},
"avatar":{"type":["string","null"],"description":"头像媒体ID，缺失可为空"},
"todayProgress":{"type":"string","description":"今日进度，步数/米/卡路里字符串"},
"cumulativeValue":{"type":"string","description":"累计运动值字符串"},
"achievedDays":{"type":"number","description":"达标天数"},
"rankNo":{"type":"number","description":"排行榜名次，无数据为 -1"},
"weekReachList":{"type":"array","description":"周一到周日固定7条达标记录","items":{"type":"object","properties":{
"date":{"type":"string","description":"日期 yyyyMMdd"},"achieved":{"type":"boolean","description":"当天是否达标"},
"value":{"type":"string","description":"当天运动数据值字符串"},"today":{"type":"boolean","description":"是否为当天"},
"inCompetitionPeriod":{"type":"boolean","description":"是否在比赛日期范围内"},"participant":{"type":"boolean","description":"当天是否有运动数据参与"}}}}}}`

const sportParticipatedRowSchema = `{"type":"object","description":"当前用户参与过的比赛实例","properties":{
"competitionCode":{"type":"string","description":"期比赛 code"},"ruleCode":{"type":"string","description":"所属规则 code"},
"cycleNo":{"type":"number","description":"期号从1起，FIXED为1"},"title":{"type":"string","description":"本期标题"},
"coverImageUrl":{"type":"string","description":"本期头图"},"sportType":{"type":"string","description":"运动项目"},
"goalValue":{"type":"number","description":"目标值：步数/米/卡路里"},"targetDayValue":{"type":"number","description":"达标天数目标"},
"dailyLimit":{"type":["number","null"],"description":"每日上限，null表示无上限"},
"periodStartDate":{"type":"string","description":"本期开始日 yyyyMMdd"},"periodEndDate":{"type":"string","description":"本期结束日 yyyyMMdd"},
"competitionStatus":{"type":"string","description":"本期状态：0未开始/1进行中/2已结束"},
"relationStatus":{"type":"number","description":"参与状态：0待加入/1已加入"},
"achievedDays":{"type":["number","null"],"description":"累计达标天数，仅已加入且进行中填充，不在榜为0"}}}`

const sportRankingRowSchema = `{"type":"object","description":"全员排行榜行","properties":{
"userId":{"type":["string","null"],"description":"员工 staffId，换取失败为 null，保留前导零"},
"nickName":{"type":"string","description":"用户昵称"},"avatar":{"type":"string","description":"头像 URL"},
"cumulativeValue":{"type":"string","description":"累计运动值字符串，步数/米/卡路里"},
"achievedDays":{"type":"number","description":"达标天数"},"rank":{"type":"number","description":"排名"}}}`

const sportOrgAdminResultSchema = `{"type":"boolean","description":"是否企业管理员：true=管理员（企业主/子管理员）；false=确认非管理员；success=false 时本命令返回错误而非 false"}`

// newSportOrgAdminCmd 判断当前用户是否企业管理员。无业务入参，操作人与企业由平台注入，不接受 user-id/corp-id；供受限操作前权限预判与话术分流。
func newSportOrgAdminCmd() *cobra.Command {
	const example = "dws sport org admin --format json"
	return NewLeafCommand(LeafSpec{
		Use: "admin", Short: "判断当前用户是否企业管理员",
		Long:    "判断当前用户是否企业管理员（企业主与子管理员均算管理员），只读无副作用，不受灰度。三态语义：result=true=管理员；result=false=确认非管理员；调用失败（success=false，如 QW_ADMIN_CHECK_FAILED 组织服务查询不可用）时本命令返回错误而不是 false，绝不能当作非管理员处理，应提示用户稍后重试。典型用途是在导出、删除、审批等受限操作前预判权限并提前话术分流：非管理员时告知用户需联系企业管理员，避免直接调用受限工具拿失败码；预判与执行之间存在权限变化窗口，受限工具仍会服务端复查，两者结论以受限工具为准。",
		Example: example, OutputRollout: output.RolloutUnifiedActive, Server: sportServerID, Tool: "is_org_admin",
		ResultCall: sportReadServiceCallResult, Safety: sportReadLeaf(),
		Contract: sportServiceContract("is_org_admin", "sport org admin", "org", "判断当前用户是否企业管理员（预发与生产口径一致）", "把调用失败当非管理员；判定结果不能代替受限工具的服务端权限复查", example, sportOrgAdminResultSchema),
	})
}
