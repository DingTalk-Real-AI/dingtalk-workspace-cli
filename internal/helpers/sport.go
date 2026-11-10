// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// ──────────────────────────────────────────────────────────
// dws sport — 企业运动健康（钉钉运动）
//
// CLI 接入工具清单（21 个）；get_user_org_list 不接入 CLI，
// MCP 平台上的下线由服务维护方单独处理：
//   list_emp_step_ranking, list_dept_step_ranking, list_emp_ranking_in_dept,
//   get_emp_step_ext_info, get_emp_step_infos, get_dept_step_infos_by_dept_id,
//   get_org_health_step_info, get_org_total_step_count,
//   create_competition, get_competition_config, get_competition_rule_detail,
//   list_competition_instances, get_current_competition_instance,
//   get_next_competition_instance, send_competition_group_msg,
//   get_signup_count, count_daily_participants, count_total_participants,
//   count_reached_users, list_daily_participation_trend
// ──────────────────────────────────────────────────────────

const sportServerID = "sport"

// newSportCommand 构建企业运动健康命令树，由 register_sport.go 手写注册。
// misc 长尾入口不展示在根帮助中，但叶子声明、直接调用与 Schema 查询保持可用。
func newSportCommand() *cobra.Command {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "sport",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "钉钉企业运动健康：企业/部门/员工步数榜单与明细、运动比赛创建与查询、参与达标统计与开赛播报",
			UseWhen: []string{
				"用户要查企业步数概况、总步数或员工/部门步数排行与明细",
				"用户要创建绑定钉钉群的运动比赛，查询比赛配置、期次或报名",
				"用户要统计比赛的参与/达标人数、逐日趋势或发送开赛播报",
			},
			AvoidWhen: []string{
				"个人运动手环、运动 App 私人数据不属于企业运动榜，不要调用",
				"创建比赛是无幂等写操作：查询已有比赛用 sport competition config，不要试探性重复创建",
			},
		},
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-misc"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("企业运动健康命令参考", "dingtalk-misc", "references/sport.md"),
			},
		},
	})

	root := sportGroupCommand(&cobra.Command{
		Use:    "sport",
		Short:  "企业运动健康",
		Hidden: true,
		Long: `企业运动健康（钉钉运动）：步数榜单/明细、运动比赛、参与达标统计。

命令结构:
  dws sport org status                企业是否开通步数榜
  dws sport step org-info             企业单日步数概况
  dws sport step org-total            企业区间总步数
  dws sport step emp-ranking          全企业员工步数排行
  dws sport step dept-ranking         部门人均步数排行
  dws sport step dept-emp-ranking     部门内员工排行
  dws sport step emp-info             员工单日步数详情
  dws sport step emp-infos            员工多日步数明细
  dws sport step dept-infos           部门多日步数汇总
  dws sport competition create        创建运动比赛（写操作）
  dws sport competition config        比赛配置详情
  dws sport competition rule-detail   比赛规则详情（仅创建人）
  dws sport competition instances     全部期实例
  dws sport competition current-instance 当前进行中的一期
  dws sport competition next-instance 下一期未开始的实例
  dws sport competition announce      发送开赛播报（写操作，无幂等）
  dws sport stats signup              累计报名人数
  dws sport stats participants-daily  某天参与人数
  dws sport stats participants        参与人数
  dws sport stats reached             达标人数
  dws sport stats trend               逐日参与/达标趋势`,
		RunE: GroupRunE,
	})

	root.AddCommand(newSportOrgGroup(), newSportStepGroup(), newSportCompetitionGroup(), newSportStatsGroup())
	return root
}

// sportReadLeaf 声明一个只读叶子命令的通用 Safety。
func sportReadLeaf() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	}
}

// sportResult 为返回业务 JSON 对象的 sport 工具声明统一返回契约。
func sportResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"additionalProperties":true}`, description)),
	}
}

func sportIdentity(tool, cliPath, group string) contract.ToolIdentitySpec {
	return contract.ToolIdentitySpec{
		ProductID:      sportServerID,
		Name:           tool,
		CanonicalPath:  sportServerID + "." + tool,
		CLIPath:        cliPath,
		PrimaryCLIPath: cliPath,
		Group:          group,
	}
}

func sportInterface(tool string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{
		Mode:         contract.InterfaceModeMCP,
		Availability: contract.InterfaceAvailable,
		Ref:          &contract.InterfaceRefSpec{ProductID: sportServerID, RPCName: tool},
	}
}

func sportParam(name, property, description string) contract.ParamDecl {
	return contract.ParamDecl{Name: name, Property: property, Description: description}
}

func sportRequiredParam(name, property, description string) contract.ParamDecl {
	p := sportParam(name, property, description)
	p.Required = sportBoolPtr(true)
	return p
}

// ── org：前置确认 ─────────────────────────────────────────

func newSportOrgGroup() *cobra.Command {
	orgCmd := sportGroupCommand(&cobra.Command{Use: "org", Short: "企业开通状态", RunE: GroupRunE})
	statusCmd := NewLeafCommand(LeafSpec{
		Use:           "status",
		Short:         "查询企业是否开通步数榜",
		Long:          "查询当前企业是否已开通钉钉运动步数榜。返回 false 表示未开通，是正常业务状态而非链路故障：此时排行接口返回空列表，应提示用户在钉钉客户端开通，不要反复重试排行查询。",
		Example:       "  dws sport org status --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "get_org_step_participate_status",
		Safety:        sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("get_org_step_participate_status", "sport org status", "org"),
			Description: "查询当前企业是否已开通步数榜",
			Result:      sportResult("企业步数榜开通状态，true=已开通，false=未开通"),
			Interface:   sportInterface("get_org_step_participate_status"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询企业是否开通步数榜",
				UseWhen:      []string{"查排行前确认企业已开通步数榜，避免把未开榜空列表误判为故障"},
				AvoidWhen:    []string{"查询步数数据本身不要使用本命令"},
				Examples:     []string{"dws sport org status --format json"},
			},
		},
	})

	orgCmd.AddCommand(statusCmd)
	return orgCmd
}

// ── step：步数榜单与明细 ──────────────────────────────────

func newSportStepGroup() *cobra.Command {
	stepCmd := sportGroupCommand(&cobra.Command{
		Use:   "step",
		Short: "步数榜单与明细",
		Long:  "企业步数概况、员工/部门排行与多日明细。日期一律 yyyyMMdd 8 位纯数字；周榜/月榜的 statDate 传周期内任意一天，服务端自动换算；排行翻页首页 cursor=0，翻页传上一页返回的 nextCursor。",
		RunE:  GroupRunE,
	})

	orgInfoCmd := NewLeafCommand(LeafSpec{
		Use:           "org-info",
		Short:         "查询企业单日步数概况",
		Long:          "查询企业某天的总步数、累计总步数与参与人数。单日企业级查询首选本命令。",
		Example:       "  dws sport step org-info --stat-date 20261109 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "get_org_health_step_info",
		Flags: []corecmd.FlagSpec{
			{Name: "stat-date", Bind: "statDate", Required: true, Trim: true, Usage: "统计日期，yyyyMMdd（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("get_org_health_step_info", "sport step org-info", "step"),
			Description: "查询企业单日步数概况（当日总步数、累计总步数、参与人数）",
			Result:      sportResult("企业单日步数概况"),
			Interface:   sportInterface("get_org_health_step_info"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询企业某天的步数概况与参与人数",
				UseWhen:      []string{"用户问企业今天/某天走了多少步、多少人参与时"},
				AvoidWhen:    []string{"查询某个员工或部门的数据不要使用本命令"},
				Examples:     []string{"dws sport step org-info --stat-date 20261109 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("stat-date", "statDate", "统计日期，yyyyMMdd"),
			},
		},
	})

	orgTotalCmd := NewLeafCommand(LeafSpec{
		Use:           "org-total",
		Short:         "查询企业区间总步数",
		Long:          "查询企业一段时间区间内的总步数，含首尾两天。仅返回一个总步数，不含其他指标。",
		Example:       "  dws sport step org-total --begin-date 20261101 --end-date 20261107 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "get_org_total_step_count",
		Flags: []corecmd.FlagSpec{
			{Name: "begin-date", Bind: "beginStatDate", Required: true, Trim: true, Usage: "开始日期，yyyyMMdd，含当天（必填）"},
			{Name: "end-date", Bind: "endStatDate", Required: true, Trim: true, Usage: "结束日期，yyyyMMdd，含当天（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("get_org_total_step_count", "sport step org-total", "step"),
			Description: "查询企业区间总步数（含首尾两天）",
			Result:      sportResult("企业区间总步数"),
			Interface:   sportInterface("get_org_total_step_count"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询企业某段时间区间的总步数",
				UseWhen:      []string{"用户问企业某段时间总共走了多少步时"},
				AvoidWhen:    []string{"需要人数、人均等明细指标时不要使用本命令"},
				Examples:     []string{"dws sport step org-total --begin-date 20261101 --end-date 20261107 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("begin-date", "beginStatDate", "开始日期，yyyyMMdd"),
				sportRequiredParam("end-date", "endStatDate", "结束日期，yyyyMMdd"),
			},
		},
	})

	empRankingCmd := NewLeafCommand(LeafSpec{
		Use:           "emp-ranking",
		Short:         "查询全企业员工步数排行",
		Long:          "查询企业员工步数排行（日/周/月榜）。statType: D=日榜、W=周榜、M=月榜；周月榜的 statDate 传周期内任意一天。返回 userId、昵称、部门、当日与周期步数、点赞数。",
		Example:       "  dws sport step emp-ranking --stat-date 20261109 --stat-type D --cursor 0 --size 20 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "list_emp_step_ranking",
		Flags: []corecmd.FlagSpec{
			{Name: "stat-date", Bind: "statDate", Required: true, Trim: true, Usage: "统计日期，yyyyMMdd；周月榜传周期内任一天（必填）"},
			{Name: "stat-type", Bind: "statType", Required: true, Trim: true, Enum: []string{"D", "W", "M"}, Usage: "榜单类型: D=日榜/W=周榜/M=月榜（必填）"},
			{Name: "cursor", Bind: "cursor", Kind: corecmd.KindInt, ArgDefault: "0", Usage: "分页游标，首页传 0"},
			{Name: "size", Bind: "size", Kind: corecmd.KindInt, ArgDefault: "20", Usage: "每页条数 1~100"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("list_emp_step_ranking", "sport step emp-ranking", "step"),
			Description: "查询企业员工步数排行（日/周/月榜）",
			Result:      sportResult("企业员工步数排行列表与分页信息"),
			Interface:   sportInterface("list_emp_step_ranking"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询企业员工步数排行榜",
				UseWhen:      []string{"用户要看企业员工步数榜（日/周/月）时"},
				AvoidWhen:    []string{"部门人均排行用 sport step dept-ranking；查询指定部门内部排行用 dept-emp-ranking"},
				Examples:     []string{"dws sport step emp-ranking --stat-date 20261109 --stat-type D --cursor 0 --size 20 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("stat-date", "statDate", "统计日期，yyyyMMdd"),
				sportRequiredParam("stat-type", "statType", "榜单类型: D/W/M"),
				sportParam("cursor", "cursor", "分页游标，首页 0"),
				sportParam("size", "size", "每页条数 1~100"),
			},
		},
	})

	deptRankingCmd := NewLeafCommand(LeafSpec{
		Use:           "dept-ranking",
		Short:         "查询部门人均步数排行",
		Long:          "查询企业部门人均步数排行。仅支持周榜/月榜（部门榜没有日榜）。返回 deptId、部门名称与人均总步数，按人均降序；deptId 可用于 dept-emp-ranking 与 dept-infos。",
		Example:       "  dws sport step dept-ranking --stat-date 20261109 --stat-type W --cursor 0 --size 20 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "list_dept_step_ranking",
		Flags: []corecmd.FlagSpec{
			{Name: "stat-date", Bind: "statDate", Required: true, Trim: true, Usage: "统计日期，yyyyMMdd；周月榜传周期内任一天（必填）"},
			{Name: "stat-type", Bind: "statType", Required: true, Trim: true, Enum: []string{"W", "M"}, Usage: "榜单类型: W=周榜/M=月榜（部门榜无日榜）（必填）"},
			{Name: "cursor", Bind: "cursor", Kind: corecmd.KindInt, ArgDefault: "0", Usage: "分页游标，首页传 0"},
			{Name: "size", Bind: "size", Kind: corecmd.KindInt, ArgDefault: "20", Usage: "每页条数 1~100"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("list_dept_step_ranking", "sport step dept-ranking", "step"),
			Description: "查询企业部门人均步数排行（仅周榜/月榜）",
			Result:      sportResult("部门人均步数排行列表与分页信息"),
			Interface:   sportInterface("list_dept_step_ranking"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询企业部门人均步数排行榜",
				UseWhen:      []string{"用户要看部门步数排行时；返回的 deptId 供部门内查询使用"},
				AvoidWhen:    []string{"部门日榜不存在：日榜需求改用 emp-ranking"},
				Examples:     []string{"dws sport step dept-ranking --stat-date 20261109 --stat-type W --cursor 0 --size 20 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("stat-date", "statDate", "统计日期，yyyyMMdd"),
				sportRequiredParam("stat-type", "statType", "榜单类型: W/M（无日榜）"),
				sportParam("cursor", "cursor", "分页游标，首页 0"),
				sportParam("size", "size", "每页条数 1~100"),
			},
		},
	})

	deptEmpRankingCmd := NewLeafCommand(LeafSpec{
		Use:           "dept-emp-ranking",
		Short:         "查询部门内员工步数排行",
		Long:          "查询某个部门内员工的步数排行（日/周/月榜）。deptId 取自 dept-ranking 的返回结果。结果不按是否达标过滤。",
		Example:       "  dws sport step dept-emp-ranking --dept-id 12345 --stat-date 20261109 --stat-type D --cursor 0 --size 20 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "list_emp_ranking_in_dept",
		Flags: []corecmd.FlagSpec{
			{Name: "dept-id", Bind: "deptId", Required: true, Trim: true, Usage: "部门 id，取自 dept-ranking 返回结果（必填）"},
			{Name: "stat-date", Bind: "statDate", Required: true, Trim: true, Usage: "统计日期，yyyyMMdd（必填）"},
			{Name: "stat-type", Bind: "statType", Required: true, Trim: true, Enum: []string{"D", "W", "M"}, Usage: "榜单类型: D/W/M（必填）"},
			{Name: "cursor", Bind: "cursor", Kind: corecmd.KindInt, ArgDefault: "0", Usage: "分页游标，首页传 0"},
			{Name: "size", Bind: "size", Kind: corecmd.KindInt, ArgDefault: "20", Usage: "每页条数 1~100"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("list_emp_ranking_in_dept", "sport step dept-emp-ranking", "step"),
			Description: "查询指定部门内员工步数排行",
			Result:      sportResult("部门内员工步数排行列表与分页信息"),
			Interface:   sportInterface("list_emp_ranking_in_dept"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询某部门内部员工步数排行",
				UseWhen:      []string{"先由 dept-ranking 拿到 deptId，再查该部门内部排行时"},
				AvoidWhen:    []string{"全企业排行用 emp-ranking"},
				Examples:     []string{"dws sport step dept-emp-ranking --dept-id 12345 --stat-date 20261109 --stat-type D --cursor 0 --size 20 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("dept-id", "deptId", "部门 id"),
				sportRequiredParam("stat-date", "statDate", "统计日期，yyyyMMdd"),
				sportRequiredParam("stat-type", "statType", "榜单类型: D/W/M"),
				sportParam("cursor", "cursor", "分页游标，首页 0"),
				sportParam("size", "size", "每页条数 1~100"),
			},
		},
	})

	empInfoCmd := NewLeafCommand(LeafSpec{
		Use:           "emp-info",
		Short:         "查询员工单日步数详情",
		Long:          "查询某员工单天的步数、名次与钉钉运动开关状态。仅支持单日：查多天明细必须改用 emp-infos。userId 为钉钉 staffId 纯数字，取自 emp-ranking 返回结果。",
		Example:       "  dws sport step emp-info --user-id 0123456789 --stat-date 20261109 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "get_emp_step_ext_info",
		Flags: []corecmd.FlagSpec{
			{Name: "user-id", Bind: "userId", Required: true, Trim: true, Usage: "员工钉钉 staffId 纯数字（必填）"},
			{Name: "stat-date", Bind: "statDate", Required: true, Trim: true, Usage: "统计日期，yyyyMMdd，仅单日（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("get_emp_step_ext_info", "sport step emp-info", "step"),
			Description: "查询员工单日步数、名次与运动开关状态",
			Result:      sportResult("员工单日步数详情，含名次与钉钉运动开关状态"),
			Interface:   sportInterface("get_emp_step_ext_info"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询某员工某天的步数和名次",
				UseWhen:      []string{"用户问某人今天的步数和名次时"},
				AvoidWhen:    []string{"查多天明细用 emp-infos；本命令仅支持单日"},
				Examples:     []string{"dws sport step emp-info --user-id 0123456789 --stat-date 20261109 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("user-id", "userId", "员工钉钉 staffId"),
				sportRequiredParam("stat-date", "statDate", "统计日期，yyyyMMdd"),
			},
		},
	})

	empInfosCmd := NewLeafCommand(LeafSpec{
		Use:           "emp-infos",
		Short:         "查询员工多日步数明细",
		Long:          "查询某员工最近多天的步数明细（1~30 天）。返回每行的日期、当日步数、部门与昵称，不含名次。stat-dates 为逗号分隔的 yyyyMMdd 日期串，如 20261030,20261031,20261101。",
		Example:       "  dws sport step emp-infos --user-id 0123456789 --stat-dates 20261107,20261108,20261109 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "get_emp_step_infos",
		Flags: []corecmd.FlagSpec{
			{Name: "user-id", Bind: "userId", Required: true, Trim: true, Usage: "员工钉钉 staffId 纯数字（必填）"},
			{Name: "stat-dates", Bind: "statDates", Required: true, Trim: true, Usage: "逗号分隔日期串，每项 yyyyMMdd，1~30 个（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("get_emp_step_infos", "sport step emp-infos", "step"),
			Description: "查询员工多日（1~30 天）步数明细，不含名次",
			Result:      sportResult("员工多日步数明细列表"),
			Interface:   sportInterface("get_emp_step_infos"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询某员工最近 N 天的步数明细",
				UseWhen:      []string{"用户要看某人最近多天每天的步数时"},
				AvoidWhen:    []string{"单日名次查询用 emp-info"},
				Examples:     []string{"dws sport step emp-infos --user-id 0123456789 --stat-dates 20261107,20261108,20261109 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("user-id", "userId", "员工钉钉 staffId"),
				sportRequiredParam("stat-dates", "statDates", "逗号分隔日期串，1~30 个"),
			},
		},
	})

	deptInfosCmd := NewLeafCommand(LeafSpec{
		Use:           "dept-infos",
		Short:         "查询部门多日步数汇总明细",
		Long:          "查询某部门多天的步数汇总（1~30 天）。返回每行的日期、部门总步数与总人数。deptId 取自 dept-ranking 的返回结果。",
		Example:       "  dws sport step dept-infos --dept-id 12345 --stat-dates 20261107,20261108,20261109 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "get_dept_step_infos_by_dept_id",
		Flags: []corecmd.FlagSpec{
			{Name: "dept-id", Bind: "deptId", Required: true, Trim: true, Usage: "部门 id，取自 dept-ranking 返回结果（必填）"},
			{Name: "stat-dates", Bind: "statDates", Required: true, Trim: true, Usage: "逗号分隔日期串，每项 yyyyMMdd，1~30 个（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("get_dept_step_infos_by_dept_id", "sport step dept-infos", "step"),
			Description: "查询部门多日（1~30 天）步数汇总明细",
			Result:      sportResult("部门多日步数汇总明细列表"),
			Interface:   sportInterface("get_dept_step_infos_by_dept_id"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询某部门最近 N 天的步数与人数表现",
				UseWhen:      []string{"用户要看某部门最近多天的整体表现时"},
				AvoidWhen:    []string{"部门内部员工排行用 dept-emp-ranking"},
				Examples:     []string{"dws sport step dept-infos --dept-id 12345 --stat-dates 20261107,20261108,20261109 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("dept-id", "deptId", "部门 id"),
				sportRequiredParam("stat-dates", "statDates", "逗号分隔日期串，1~30 个"),
			},
		},
	})

	stepCmd.AddCommand(orgInfoCmd, orgTotalCmd, empRankingCmd, deptRankingCmd, deptEmpRankingCmd, empInfoCmd, empInfosCmd, deptInfosCmd)
	return stepCmd
}

// ── competition：运动比赛 ─────────────────────────────────

func newSportCompetitionGroup() *cobra.Command {
	competitionCmd := sportGroupCommand(&cobra.Command{
		Use:   "competition",
		Short: "运动比赛管理",
		Long:  "创建与查询绑定钉钉群的企业运动比赛。create 与 announce 是真实写操作：create 无删除手段，参数必须先与用户确认；announce 无幂等，一次创建只调用一次。",
		RunE:  GroupRunE,
	})

	competitionCmd.AddCommand(
		newSportCompetitionCreateCmd(),
		newSportCompetitionConfigCmd(),
		newSportCompetitionRuleDetailCmd(),
		newSportCompetitionInstancesCmd(),
		newSportCompetitionCurrentInstanceCmd(),
		newSportCompetitionNextInstanceCmd(),
		newSportCompetitionAnnounceCmd(),
	)
	return competitionCmd
}

// sportCreateFlatKeys 是 create_competition 顶层展开的竞赛字段；Call 钩子会把
// 它们重新包装进服务端要求的嵌套 CompetitionCreateMcpRequest 对象（各字段按
// 字符串传值）。
var sportCreateFlatKeys = []string{
	"competitionName", "sportType", "competitionType",
	"firstPeriodStartDate", "firstPeriodEndDate",
	"goalValue", "targetDayValue", "dailyLimit",
	"groupId", "openConversationId",
	"needSignup", "allowFreeJoin", "coverImageUrl",
}

// sportCallResult 是 sport 叶子命令的统一结果执行体：经 helpers 的统一
// MCP 调用通道（网关 token、未登录、PAT、业务错误体分类、多 JSON 值拒绝
// 均在同一层处理），返回值保留服务端原始形状（含 null）。RolloutUnified
// Active 命令必须经本路径把结果存入统一结果，不能直接打印。
// dry-run 在本地返回请求预览，并由 output.WithDryRun 标记统一信封；
// 不调用 ToolCaller，避免将 adapter 的 JSON-RPC 回显当作业务结果。
func sportCallResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	caller := GetCaller()
	if caller == nil {
		return nil, apperrors.NewInternal("MCP caller is not initialized")
	}
	if corecmd.BoolFlag(cmd, "dry-run") || caller.DryRun() {
		return output.Success(map[string]any{
			"tool": tool, "arguments": args, "executed": false,
		}, output.WithDryRun()), nil
	}
	data, err := CallMCPToolDataOnServer(cmd.Context(), sportServerID, tool, args)
	if err != nil {
		return nil, err
	}
	return output.Success(data), nil
}

// sportCreateResult 把平铺 toolArgs 重组为 create_competition 的嵌套入参后委托。
func sportCreateResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	return sportCallResult(cmd, tool, sportBuildCreatePayload(args))
}

// sportBuildCreatePayload 把 Call 收到的平铺 toolArgs 重组为 create_competition
// 的嵌套入参 {userId, CompetitionCreateMcpRequest:{...}}。
func sportBuildCreatePayload(args map[string]any) map[string]any {
	request := make(map[string]any, len(args))
	for _, key := range sportCreateFlatKeys {
		if value, ok := args[key]; ok {
			request[key] = value
		}
	}
	payload := map[string]any{"CompetitionCreateMcpRequest": request}
	if userID, ok := args["userId"]; ok {
		payload["userId"] = userID
	}
	return payload
}

// sportValidateDate 检查 yyyyMMdd 8 位纯数字且是真实日期。
// 调用方已保证 8 位纯数字，因此分段 strconv.Atoi 不会失败，无需错误分支。
func sportValidateDate(field, value string) error {
	if len(value) != 8 {
		return apperrors.NewValidation(fmt.Sprintf("--%s 必须是 yyyyMMdd 8 位纯数字，收到 %q", field, value))
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return apperrors.NewValidation(fmt.Sprintf("--%s 必须是 yyyyMMdd 8 位纯数字，收到 %q", field, value))
		}
	}
	y, _ := strconv.Atoi(value[:4])
	m, _ := strconv.Atoi(value[4:6])
	d, _ := strconv.Atoi(value[6:])
	if m < 1 || m > 12 {
		return apperrors.NewValidation(fmt.Sprintf("--%s 月份非法：%q", field, value))
	}
	daysInMonth := map[int]int{1: 31, 3: 31, 4: 30, 5: 31, 6: 30, 7: 31, 8: 31, 9: 30, 10: 31, 11: 30, 12: 31}
	maxDay := daysInMonth[m]
	if m == 2 {
		maxDay = 28
		if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			maxDay = 29
		}
	}
	if d < 1 || d > maxDay {
		return apperrors.NewValidation(fmt.Sprintf("--%s 不是真实日期：%q", field, value))
	}
	return nil
}

// sportParseDate 把已通过 sportValidateDate 的 yyyyMMdd 解析为 UTC 零点时间。
func sportParseDate(value string) time.Time {
	t, _ := time.Parse("20060102", value)
	return t
}

func newSportCompetitionCreateCmd() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   "create",
		Short: "创建企业运动比赛（写操作，不可删除）",
		Long: `创建绑定钉钉群的运动比赛。真实写操作：创建后无法删除，参数错误需重建，务必先与用户逐项确认；禁止用试探性参数反复创建，查询已有比赛用 sport competition config。

当前仅支持 FIXED（固定不循环）比赛：服务端 Schema 将 groupId 与 firstPeriodEndDate 列为无条件必填，而工具描述仍声称循环模式可省结束日期，契约冲突期间 RECURRING 创建在 CLI 侧阻断；不要用空值或猜测日期绕过。

字段说明：
  --name              比赛名称（必填）
  --sport-type        WALK 走路（目标单位=步）/ RUN 跑步（=米）/ COMPREHENSIVE 综合运动（=千卡）（必填）
  --competition-type  仅 FIXED（必填）
  --start-date        首期开始日期 yyyyMMdd（必填）
  --end-date          首期结束日期 yyyyMMdd（必填）
  --goal-value        每日目标值 >0，单位随运动类型（必填）
  --target-day-value  周期内需达标天数 >0，不得超过单期实际天数（必填）
  --group-id          绑定群 id，群必须属于当前企业（必填；open-conversation-id 仅是可选补充，不能替代）
  --need-signup       true=须手动报名；false=自动视为已报名（必填）
  --allow-free-join   是否允许非群成员凭分享链接报名（必填）
  --cover-image-url   封面图 URL，优先使用官方默认头图（必填）
  --daily-limit       可选，每日运动量上限，必须 ≥ goal-value
  --open-conversation-id 可选，群会话 id 补充字段

操作人 --user-id 是该 MCP 服务所绑定企业内的服务侧 userId，取当前会话已验证可用的值。返回 ruleCode 与 status：1=已生效（群主/管理员创建），0=待群主审批（普通成员创建）。`,
		Example:       `  dws sport competition create --user-id 0123456789 --name 走路挑战赛 --sport-type WALK --competition-type FIXED --start-date 20261111 --end-date 20261130 --goal-value 8000 --target-day-value 18 --group-id 123456 --need-signup false --allow-free-join false --cover-image-url "https://gw.alicdn.com/imgextra/i2/O1CN01Pa9N1djducB1WPcg_!!6000000007915-2-tps-750-580.png" --format json`,
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		Tool:          "create_competition",
		Flags: []corecmd.FlagSpec{
			{Name: "user-id", Bind: "userId", Required: true, Trim: true, Usage: "操作人 userId（服务侧企业内的 userId，必填）"},
			{Name: "name", Bind: "competitionName", Required: true, Trim: true, Usage: "比赛名称（必填）"},
			{Name: "sport-type", Bind: "sportType", Required: true, Trim: true, Enum: []string{"WALK", "RUN", "COMPREHENSIVE"}, Usage: "运动类型: WALK/RUN/COMPREHENSIVE（必填）"},
			{Name: "competition-type", Bind: "competitionType", Required: true, Trim: true, Enum: []string{"FIXED"}, Usage: "比赛类型: 当前仅 FIXED（必填）"},
			{Name: "start-date", Bind: "firstPeriodStartDate", Required: true, Trim: true, Usage: "首期开始日期 yyyyMMdd（必填）"},
			{Name: "end-date", Bind: "firstPeriodEndDate", Required: true, Trim: true, Usage: "首期结束日期 yyyyMMdd（必填）"},
			{Name: "goal-value", Bind: "goalValue", Required: true, Trim: true, Usage: "每日目标值 >0，单位步/米/千卡（必填）"},
			{Name: "target-day-value", Bind: "targetDayValue", Required: true, Trim: true, Usage: "周期内需达标天数 >0（必填）"},
			{Name: "group-id", Bind: "groupId", Required: true, Trim: true, Usage: "绑定群 id（必填）"},
			{Name: "need-signup", Bind: "needSignup", Required: true, Trim: true, Enum: []string{"true", "false"}, Usage: "是否需报名确认（必填）"},
			{Name: "allow-free-join", Bind: "allowFreeJoin", Required: true, Trim: true, Enum: []string{"true", "false"}, Usage: "是否允许非群成员报名（必填）"},
			{Name: "cover-image-url", Bind: "coverImageUrl", Required: true, Trim: true, Usage: "封面图 URL，优先官方默认头图（必填）"},
			{Name: "daily-limit", Bind: "dailyLimit", Trim: true, OmitEmpty: true, Usage: "每日运动量上限，≥ goal-value（可选）"},
			{Name: "open-conversation-id", Bind: "openConversationId", Trim: true, OmitEmpty: true, Usage: "群会话 id，可选补充（可选）"},
			{Name: "period-type", Bind: "periodType", Hidden: true, OmitEmpty: true, Usage: "循环周期类型；RECURRING 创建当前被阻断，不要传"},
			{Name: "cycle-count", Bind: "cycleCount", Hidden: true, OmitEmpty: true, Usage: "循环场次；RECURRING 创建当前被阻断，不要传"},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Validate:   sportValidateCreateArgs,
		ResultCall: sportCreateResult,
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("create_competition", "sport competition create", "competition"),
			Description: "创建绑定钉钉群的固定（FIXED）运动比赛，返回 ruleCode 与审批状态",
			Result:      sportResult("新比赛返回值：ruleCode 与 status（1=已生效，0=待群主审批）"),
			Interface:   sportInterface("create_competition"),
			Selection: contract.SelectionSpec{
				AgentSummary: "创建绑定钉钉群的运动比赛（写操作，需用户确认全部参数）",
				UseWhen:      []string{"用户明确要求创建运动比赛且全部要素（名称/类型/目标/群/日期/封面）已确认时"},
				AvoidWhen:    []string{"查询已有比赛用 sport competition config；RECURRING 循环比赛当前被阻断；参数未与用户确认前不要调用"},
				Examples:     []string{"dws sport competition create --user-id 0123456789 --name 走路挑战赛 --sport-type WALK --competition-type FIXED --start-date 20261111 --end-date 20261130 --goal-value 8000 --target-day-value 18 --group-id 123456 --need-signup false --allow-free-join false --cover-image-url <官方默认头图> --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("user-id", "userId", "操作人 userId"),
				sportRequiredParam("name", "competitionName", "比赛名称"),
				sportRequiredParam("sport-type", "sportType", "运动类型: WALK/RUN/COMPREHENSIVE"),
				sportRequiredParam("competition-type", "competitionType", "比赛类型: 当前仅 FIXED"),
				sportRequiredParam("start-date", "firstPeriodStartDate", "首期开始日期 yyyyMMdd"),
				sportRequiredParam("end-date", "firstPeriodEndDate", "首期结束日期 yyyyMMdd"),
				sportRequiredParam("goal-value", "goalValue", "每日目标值 >0"),
				sportRequiredParam("target-day-value", "targetDayValue", "周期内需达标天数 >0"),
				sportRequiredParam("group-id", "groupId", "绑定群 id"),
				sportRequiredParam("need-signup", "needSignup", "是否需报名确认"),
				sportRequiredParam("allow-free-join", "allowFreeJoin", "是否允许非群成员报名"),
				sportRequiredParam("cover-image-url", "coverImageUrl", "封面图 URL"),
				sportParam("daily-limit", "dailyLimit", "每日运动量上限，≥ goalValue"),
				sportParam("open-conversation-id", "openConversationId", "群会话 id，可选补充"),
			},
		},
	})
}

// sportValidateCreateArgs 在确认之前完成本地可判定的参数校验：FIXED-only
// 阻断、日期合法性与业务数值约束。真实业务约束仍以服务端权威校验为准。
func sportValidateCreateArgs(cmd *cobra.Command, _ []string) error {
	mustFlag := func(name string) string { return strings.TrimSpace(MustGetStringFlag(cmd, name)) }

	competitionType := mustFlag("competition-type")
	if competitionType != "FIXED" {
		return apperrors.NewValidation("当前仅支持 --competition-type FIXED：服务端对 RECURRING 的必填约束与描述存在冲突，循环比赛创建在 CLI 侧阻断，待服务端统一契约后放开")
	}
	if mustFlag("period-type") != "" || mustFlag("cycle-count") != "" {
		return apperrors.NewValidation("RECURRING 创建当前被阻断：不要传 --period-type / --cycle-count")
	}
	for _, field := range []struct{ flag, value string }{
		{"start-date", mustFlag("start-date")},
		{"end-date", mustFlag("end-date")},
	} {
		if err := sportValidateDate(field.flag, field.value); err != nil {
			return err
		}
	}
	start := sportParseDate(mustFlag("start-date"))
	end := sportParseDate(mustFlag("end-date"))
	if end.Before(start) {
		return apperrors.NewValidation("--end-date 不得早于 --start-date")
	}
	periodDays := int(end.Sub(start).Hours()/24) + 1

	goalValue := mustFlag("goal-value")
	if !sportIsPositiveNumber(goalValue) {
		return apperrors.NewValidation("--goal-value 必须是大于 0 的数字")
	}
	targetDayValue := mustFlag("target-day-value")
	targetDays, ok := sportParsePositiveInt(targetDayValue)
	if !ok {
		return apperrors.NewValidation("--target-day-value 必须是大于 0 的整数")
	}
	if targetDays > periodDays {
		return apperrors.NewValidation(fmt.Sprintf("--target-day-value（%d）不得超过单期实际天数（%d，按 --start-date 至 --end-date 推算）", targetDays, periodDays))
	}
	if dailyLimit := mustFlag("daily-limit"); dailyLimit != "" {
		if !sportIsPositiveNumber(dailyLimit) {
			return apperrors.NewValidation("--daily-limit 必须是大于 0 的数字")
		}
		if sportCompareNumbers(dailyLimit, goalValue) < 0 {
			return apperrors.NewValidation("--daily-limit 必须大于等于 --goal-value")
		}
	}
	return nil
}

func sportIsPositiveNumber(value string) bool {
	if value == "" {
		return false
	}
	dotSeen := false
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r == '.' && !dotSeen:
			dotSeen = true
		default:
			return false
		}
	}
	return strings.Trim(value, "0.") != ""
}

func sportParsePositiveInt(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	n := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
		if n > 1_000_000 {
			return 0, false
		}
	}
	if n <= 0 {
		return 0, false
	}
	return n, true
}

// sportCompareNumbers 比较两个非负十进制数字字符串（任意小数位数），
// 返回 -1/0/1；任一非数字返回 0 并由调用方先行校验兜底。实现为十进制
// 串规范化后的逐段比较，避免定点缩放的高位截断。
func sportCompareNumbers(a, b string) int {
	intA, fracA := splitDecimalParts(a)
	intB, fracB := splitDecimalParts(b)
	if len(intA) != len(intB) {
		if len(intA) < len(intB) {
			return -1
		}
		return 1
	}
	if cmp := strings.Compare(intA, intB); cmp != 0 {
		return cmp
	}
	if len(fracA) < len(fracB) {
		fracA += strings.Repeat("0", len(fracB)-len(fracA))
	} else if len(fracB) < len(fracA) {
		fracB += strings.Repeat("0", len(fracA)-len(fracB))
	}
	return strings.Compare(fracA, fracB)
}

// splitDecimalParts 返回去前导零的整数部分与去尾随零的小数部分。
func splitDecimalParts(value string) (string, string) {
	intPart, fracPart := value, ""
	if idx := strings.IndexByte(value, '.'); idx >= 0 {
		intPart, fracPart = value[:idx], value[idx+1:]
	}
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}
	return intPart, strings.TrimRight(fracPart, "0")
}

func newSportCompetitionConfigCmd() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "config",
		Short:         "查询比赛配置详情",
		Long:          "按 ruleCode 查询比赛整体配置：名称、运动项目、每日目标值、达标天数目标、周期配置、整体起止、是否需报名、累计报名人数与启用状态。返回 null 表示 corpId 无法解码或配置不存在。",
		Example:       "  dws sport competition config --rule-code RULE123 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "get_competition_config",
		Flags: []corecmd.FlagSpec{
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "活动 ruleCode（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("get_competition_config", "sport competition config", "competition"),
			Description: "查询比赛整体配置详情",
			Result:      sportResult("比赛配置详情；null 表示配置不存在"),
			Interface:   sportInterface("get_competition_config"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询运动比赛的配置详情",
				UseWhen:      []string{"已知 ruleCode，查询比赛名称/目标/周期等配置时；也用于创建前确认已有比赛"},
				AvoidWhen:    []string{"规则明细（含绑定群信息）仅创建人可查，用 rule-detail"},
				Examples:     []string{"dws sport competition config --rule-code RULE123 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("rule-code", "ruleCode", "活动 ruleCode"),
			},
		},
	})
}

func newSportCompetitionRuleDetailCmd() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "rule-detail",
		Short:         "查询比赛规则详情（仅创建人）",
		Long:          "按 ruleCode 查询比赛规则明细（含头图、周期、每日上限、绑定群 cid 与群名、报名人数、规则状态与比赛状态）。仅比赛创建人可查，非创建人会被拒，此时改用 sport competition config。",
		Example:       "  dws sport competition rule-detail --user-id 0123456789 --rule-code RULE123 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "get_competition_rule_detail",
		Flags: []corecmd.FlagSpec{
			{Name: "user-id", Bind: "userId", Required: true, Trim: true, Usage: "操作人 userId，必须是比赛创建人（必填）"},
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "活动 ruleCode（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("get_competition_rule_detail", "sport competition rule-detail", "competition"),
			Description: "比赛创建人查询规则明细（含绑定群与状态）",
			Result:      sportResult("比赛规则明细；非创建人会被拒绝"),
			Interface:   sportInterface("get_competition_rule_detail"),
			Selection: contract.SelectionSpec{
				AgentSummary: "比赛创建人查询规则明细",
				UseWhen:      []string{"操作人就是创建人，需要绑定群或规则状态明细时"},
				AvoidWhen:    []string{"操作人不是创建人时必被拒，改用 sport competition config"},
				Examples:     []string{"dws sport competition rule-detail --user-id 0123456789 --rule-code RULE123 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("user-id", "userId", "操作人 userId（创建人）"),
				sportRequiredParam("rule-code", "ruleCode", "活动 ruleCode"),
			},
		},
	})
}

func newSportCompetitionInstancesCmd() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "instances",
		Short:         "查询比赛全部期实例",
		Long:          "按 ruleCode 查询比赛的全部期实例，返回期号、标题、起止日期与状态，按 cycleNo 升序。",
		Example:       "  dws sport competition instances --rule-code RULE123 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "list_competition_instances",
		Flags: []corecmd.FlagSpec{
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "活动 ruleCode（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("list_competition_instances", "sport competition instances", "competition"),
			Description: "查询比赛全部期实例（按期号升序）",
			Result:      sportResult("比赛全部期实例列表"),
			Interface:   sportInterface("list_competition_instances"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询比赛的全部期实例",
				UseWhen:      []string{"需要完整期次列表时"},
				AvoidWhen:    []string{"只关心当前期用 current-instance，下一期用 next-instance"},
				Examples:     []string{"dws sport competition instances --rule-code RULE123 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("rule-code", "ruleCode", "活动 ruleCode"),
			},
		},
	})
}

func newSportCompetitionCurrentInstanceCmd() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "current-instance",
		Short:         "查询当前进行中的一期",
		Long:          "按 ruleCode 查询当前进行中的比赛期实例，返回 competitionCode 与本期结束日期（回答「本期什么时候结束」）。返回 null 表示当前没有进行中的实例。",
		Example:       "  dws sport competition current-instance --rule-code RULE123 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "get_current_competition_instance",
		Flags: []corecmd.FlagSpec{
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "活动 ruleCode（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("get_current_competition_instance", "sport competition current-instance", "competition"),
			Description: "查询当前进行中的一期实例（含 competitionCode）",
			Result:      sportResult("当前期实例；null 表示没有进行中的实例"),
			Interface:   sportInterface("get_current_competition_instance"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前进行中的比赛期",
				UseWhen:      []string{"回答「本期什么时候结束」或获取 competitionCode 供统计命令使用时"},
				AvoidWhen:    []string{"查询下一期用 next-instance"},
				Examples:     []string{"dws sport competition current-instance --rule-code RULE123 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("rule-code", "ruleCode", "活动 ruleCode"),
			},
		},
	})
}

func newSportCompetitionNextInstanceCmd() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "next-instance",
		Short:         "查询下一期未开始的实例",
		Long:          "按 ruleCode 查询下一期未开始的比赛实例，返回开始日期（回答「下一期什么时候开始」）。",
		Example:       "  dws sport competition next-instance --rule-code RULE123 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "get_next_competition_instance",
		Flags: []corecmd.FlagSpec{
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "活动 ruleCode（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("get_next_competition_instance", "sport competition next-instance", "competition"),
			Description: "查询下一期未开始的实例（含开始日期）",
			Result:      sportResult("下一期实例；null 表示没有未开始的下一期"),
			Interface:   sportInterface("get_next_competition_instance"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询比赛的下一期开始时间",
				UseWhen:      []string{"回答「下一期什么时候开始」时"},
				AvoidWhen:    []string{"查询当前进行中的一期用 current-instance"},
				Examples:     []string{"dws sport competition next-instance --rule-code RULE123 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("rule-code", "ruleCode", "活动 ruleCode"),
			},
		},
	})
}

func newSportCompetitionAnnounceCmd() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   "announce",
		Short: "发送比赛开赛播报（写操作，无幂等）",
		Long: `按 ruleCode 向比赛绑定群发送播报：规则已生效（status=1）发送开赛广播（文字+活动卡片）；待群主审批（status=0）发送 at 群主的审批提醒（仅群主与发起人可见）。服务端按 ruleCode 自动定位绑定群，无需传群参数。

无幂等：重复调用会重复推送，一次创建只调用一次。返回 true 仅代表链路执行完成，不代表消息一定可见。`,
		Example:       "  dws sport competition announce --user-id 0123456789 --rule-code RULE123 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "send_competition_group_msg",
		Flags: []corecmd.FlagSpec{
			{Name: "user-id", Bind: "userId", Required: true, Trim: true, Usage: "操作人 userId（必填）"},
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "活动 ruleCode（必填）"},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("send_competition_group_msg", "sport competition announce", "competition"),
			Description: "向比赛绑定群发送开赛播报或审批提醒（无幂等）",
			Result:      sportResult("发送结果；true 仅代表链路执行完成，不保证消息可见"),
			Interface:   sportInterface("send_competition_group_msg"),
			Selection: contract.SelectionSpec{
				AgentSummary: "向比赛绑定群发送一次开赛播报（写操作，无幂等）",
				UseWhen:      []string{"用户明确要求向绑定群播报比赛，且本次调用只执行一次时"},
				AvoidWhen:    []string{"重复调用会重复推送；一次创建只允许调用一次"},
				Examples:     []string{"dws sport competition announce --user-id 0123456789 --rule-code RULE123 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("user-id", "userId", "操作人 userId"),
				sportRequiredParam("rule-code", "ruleCode", "活动 ruleCode"),
			},
		},
	})
}

// ── stats：活动统计 ───────────────────────────────────────

func newSportStatsGroup() *cobra.Command {
	statsCmd := sportGroupCommand(&cobra.Command{
		Use:   "stats",
		Short: "比赛参与与达标统计",
		Long:  "比赛报名、参与、达标与逐日趋势统计。competitionCode 可选时缺省为当前进行中的一期，可由 sport competition current-instance 获取。",
		RunE:  GroupRunE,
	})

	signupCmd := NewLeafCommand(LeafSpec{
		Use:           "signup",
		Short:         "查询累计报名人数",
		Long:          "按 ruleCode 查询比赛累计报名人数。任何报名路径首次成功时自增，只增不减。返回 null 表示配置不存在。",
		Example:       "  dws sport stats signup --rule-code RULE123 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "get_signup_count",
		Flags: []corecmd.FlagSpec{
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "活动 ruleCode（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("get_signup_count", "sport stats signup", "stats"),
			Description: "查询比赛累计报名人数",
			Result:      sportResult("累计报名人数；null 表示配置不存在"),
			Interface:   sportInterface("get_signup_count"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询比赛的累计报名人数",
				UseWhen:      []string{"统计比赛报名规模时"},
				AvoidWhen:    []string{"报名人数不等于参与人数，参与统计用 participants"},
				Examples:     []string{"dws sport stats signup --rule-code RULE123 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("rule-code", "ruleCode", "活动 ruleCode"),
			},
		},
	})

	participantsDailyCmd := NewLeafCommand(LeafSpec{
		Use:           "participants-daily",
		Short:         "查询某天参与人数",
		Long:          "查询当前进行中的一期比赛某天的参与人数（当天有运动数据的人数）。stat-date 缺省为今天。返回 null 表示参数非法或没有进行中的实例。",
		Example:       "  dws sport stats participants-daily --rule-code RULE123 --stat-date 20261109 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "count_daily_participants",
		Flags: []corecmd.FlagSpec{
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "活动 ruleCode（必填）"},
			{Name: "stat-date", Bind: "statDate", Trim: true, OmitEmpty: true, Usage: "统计日期 yyyyMMdd，缺省今天（可选）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("count_daily_participants", "sport stats participants-daily", "stats"),
			Description: "查询比赛某天的参与人数",
			Result:      sportResult("某天参与人数；null 表示参数非法或没有进行中的实例"),
			Interface:   sportInterface("count_daily_participants"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询比赛某天的参与人数",
				UseWhen:      []string{"统计比赛某天的当日参与情况时"},
				AvoidWhen:    []string{"累计参与人数用 participants"},
				Examples:     []string{"dws sport stats participants-daily --rule-code RULE123 --stat-date 20261109 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("rule-code", "ruleCode", "活动 ruleCode"),
				sportParam("stat-date", "statDate", "统计日期 yyyyMMdd，缺省今天"),
			},
		},
	})

	participantsCmd := NewLeafCommand(LeafSpec{
		Use:           "participants",
		Short:         "查询参与人数",
		Long:          "查询比赛的参与人数。competition-code 缺省为当前活动期。",
		Example:       "  dws sport stats participants --rule-code RULE123 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "count_total_participants",
		Flags: []corecmd.FlagSpec{
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "活动 ruleCode（必填）"},
			{Name: "competition-code", Bind: "competitionCode", Trim: true, OmitEmpty: true, Usage: "活动期 code，缺省当前期（可选）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("count_total_participants", "sport stats participants", "stats"),
			Description: "查询比赛参与人数",
			Result:      sportResult("参与人数"),
			Interface:   sportInterface("count_total_participants"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询比赛的参与人数",
				UseWhen:      []string{"统计比赛累计或某期参与人数时"},
				AvoidWhen:    []string{"达标人数用 reached"},
				Examples:     []string{"dws sport stats participants --rule-code RULE123 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("rule-code", "ruleCode", "活动 ruleCode"),
				sportParam("competition-code", "competitionCode", "活动期 code，缺省当前期"),
			},
		},
	})

	reachedCmd := NewLeafCommand(LeafSpec{
		Use:           "reached",
		Short:         "查询达标人数",
		Long:          "查询达到每日目标值（goalValue）的人数。rank-type 仅支持 DAY/WEEK/MONTH，TOTAL 口径不支持。competition-code 为活动当前期，可由 sport competition current-instance 获取。返回 null 表示参数非法或当前没有进行中的比赛。",
		Example:       "  dws sport stats reached --rule-code RULE123 --rank-type DAY --query-date 20261109 --competition-code COMP456 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "count_reached_users",
		Flags: []corecmd.FlagSpec{
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "活动 ruleCode（必填）"},
			{Name: "rank-type", Bind: "rankType", Required: true, Trim: true, Enum: []string{"DAY", "WEEK", "MONTH"}, Usage: "达标口径: DAY/WEEK/MONTH，不支持 TOTAL（必填）"},
			{Name: "query-date", Bind: "queryDate", Required: true, Trim: true, Usage: "统计时间 yyyyMMdd（必填）"},
			{Name: "competition-code", Bind: "competitionCode", Required: true, Trim: true, Usage: "活动当前期 code（必填）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("count_reached_users", "sport stats reached", "stats"),
			Description: "查询达到每日目标值的人数（按日/周/月口径）",
			Result:      sportResult("达标人数；null 表示参数非法或没有进行中的比赛"),
			Interface:   sportInterface("count_reached_users"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询比赛达标人数",
				UseWhen:      []string{"统计某口径下达标人数时"},
				AvoidWhen:    []string{"TOTAL 口径不支持，只允许 DAY/WEEK/MONTH"},
				Examples:     []string{"dws sport stats reached --rule-code RULE123 --rank-type DAY --query-date 20261109 --competition-code COMP456 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("rule-code", "ruleCode", "活动 ruleCode"),
				sportRequiredParam("rank-type", "rankType", "达标口径: DAY/WEEK/MONTH"),
				sportRequiredParam("query-date", "queryDate", "统计时间 yyyyMMdd"),
				sportRequiredParam("competition-code", "competitionCode", "活动当前期 code"),
			},
		},
	})

	trendCmd := NewLeafCommand(LeafSpec{
		Use:           "trend",
		Short:         "查询逐日参与/达标趋势",
		Long:          "查询比赛开期至今每天的参与人数与达标人数，日期升序，最多 90 天，可用于生成活动进展曲线。competition-code 缺省为当前进行中的一期。返回 null/空数组表示参数非法或实例尚未开始。",
		Example:       "  dws sport stats trend --rule-code RULE123 --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        sportServerID,
		ResultCall:    sportCallResult,
		Tool:          "list_daily_participation_trend",
		Flags: []corecmd.FlagSpec{
			{Name: "rule-code", Bind: "ruleCode", Required: true, Trim: true, Usage: "活动 ruleCode（必填）"},
			{Name: "competition-code", Bind: "competitionCode", Trim: true, OmitEmpty: true, Usage: "活动期 code，缺省当前期（可选）"},
		},
		Safety: sportReadLeaf(),
		Contract: corecmd.ContractDecl{
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Identity:    sportIdentity("list_daily_participation_trend", "sport stats trend", "stats"),
			Description: "查询比赛逐日参与/达标趋势（最多 90 天）",
			Result:      sportResult("逐日参与与达标人数趋势列表"),
			Interface:   sportInterface("list_daily_participation_trend"),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询比赛逐日参与/达标趋势",
				UseWhen:      []string{"复盘比赛、生成活动进展曲线时"},
				AvoidWhen:    []string{"单日参与数用 participants-daily"},
				Examples:     []string{"dws sport stats trend --rule-code RULE123 --format json"},
			},
			Parameters: []contract.ParamDecl{
				sportRequiredParam("rule-code", "ruleCode", "活动 ruleCode"),
				sportParam("competition-code", "competitionCode", "活动期 code，缺省当前期"),
			},
		},
	})

	statsCmd.AddCommand(signupCmd, participantsDailyCmd, participantsCmd, reachedCmd, trendCmd)
	return statsCmd
}

// sportGroupCommand 声明导航容器分组节点，与 helpers.newGroupCommand 语义一致。
func sportGroupCommand(command *cobra.Command) *cobra.Command {
	corecmd.ApplyGroupPolicy(command, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	return command
}

func sportBoolPtr(v bool) *bool { return &v }
