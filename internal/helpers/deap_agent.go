// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

const (
	deapAgentServerID = "deap-dev"

	// dingtalkTagProductID 是钉钉数字员工的用户可见产品标识；内部 MCP server 仍为 deap-dev。
	// 契约要求 CanonicalPath 严格等于 <ProductID>.<Name>。
	dingtalkTagProductID = "dingtalk-tag"

	deapAgentCreateTool    = "create_digital_employee"
	deapAgentDetailTool    = "get_digital_employee_detail"
	deapAgentListTool      = "list_digital_employees"
	deapAgentAuthCodeTool  = "get_dws_auth_code"
	deapAgentSaveDraftTool = "update_digital_employee_draft"
	deapAgentPublishTool   = "publish_digital_employee"
	deapAgentDeleteTool    = "delete_digital_employee"
	deapAgentRunStatusTool = "query_de_run_status"
	deapAgentTraceTool     = "query_de_trace"

	deapAgentResponseModeMentionOnly       = "mention_only"
	deapAgentResponseModeTargetedProactive = "targeted_proactive"
	deapAgentResponseModeCombined          = "mention_only,targeted_proactive"
	deapAgentMainProgramTypeOpenCode       = "open_code"
	deapAgentMainProgramTypeLocalAgent     = "local_agent"
)

var deapAgentResponseModeValues = []string{
	deapAgentResponseModeMentionOnly,
	deapAgentResponseModeTargetedProactive,
	deapAgentResponseModeCombined,
}

var deapAgentMainProgramTypeValues = []string{
	deapAgentMainProgramTypeOpenCode,
	deapAgentMainProgramTypeLocalAgent,
}

var deapAgentDryRun = &contract.DryRunSpec{
	PreviewKind: contract.DryRunPreviewInvocation,
	RemoteReads: false,
}

// deapAgentSourceTypes 是 chat-2 message_source 表登记的来源类型白名单，与
// MessageSourceType 枚举逐字一致；服务端对未知类型直接报参数错，不做兜底解析。
// im_message 的 sourceId 是钉钉开放态消息 ID（openMessageId），单聊/群@/群感知三条
// 链路含义一致；trigger_rule 的 sourceId 是群感知规则 ID。
var deapAgentSourceTypes = []string{"im_message", "trigger_rule"}

func init() {
	RegisterPublic(func() Handler {
		return deapHandler{}
	})
}

// deapHandler 挂载顶级命令 `dws dingtalk-tag`：
//
//	dingtalk-tag manage       数字员工管理（创建 / 详情 / 列表 / 临时授权码 / 草稿 / 发布 / 删除）
//	dingtalk-tag run          执行观测（执行状态 / 执行 trace）
//	dingtalk-tag capability   数字员工能力资源（Skill / MCP）
//
// 各子组的资源边界和安全属性不同，均平级挂在 DEAP 产品下。
type deapHandler struct{}

func (deapHandler) Name() string {
	return "dingtalk-tag"
}

func (deapHandler) Command(executor.Runner) *cobra.Command {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: dingtalkTagProductID,
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-tag"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("DingTalk Tag 数字员工指南", "dingtalk-tag", "references/dingtalk-tag-index.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "创建和管理数字员工、查询执行状态，并把已有本地数字员工接入 DSH",
			UseWhen: []string{
				"创建、修改、发布或删除 DEAP 数字员工",
				"查数字员工某次执行的状态或完整模型链路",
				"创建或查询可配置到数字员工草稿的 Skill/MCP 资源",
				"把已有且已发布的 local_agent 数字员工接入本地 DSH",
			},
			AvoidWhen: []string{
				"开放平台应用、机器人配置与版本发布用 dev；普通企业消息收发用 chat",
			},
		},
	})
	root := &cobra.Command{
		Use:               "dingtalk-tag",
		Short:             "DEAP 平台",
		Long:              "钉钉数字员工命令组：manage 负责数字员工生命周期和临时 DWS 授权码，run 负责执行状态与 trace，capability 负责 Skill/MCP 能力资源，connect 把已有且已发布的 local_agent 数字员工接入 DSH，channel 提供 DSH 受限机器协议。固定调用 MCP product/server deap-dev；identity.corpId/userId 由可信登录态注入且不对 CLI 暴露。端点跟随当前 MCP 环境自动选择规范网关；DINGTALK_DEAP_DEV_MCP_URL 仅用于本地调试覆盖。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              groupRunE,
	}
	newGroupCommand(root)
	root.AddCommand(
		newDeapManageCommand(),
		newDeapRunCommand(),
		newDeapCapabilityCommand(),
		newDeapConnectCommand(),
		newDeapChannelCommand(),
	)
	return root
}

// newDeapManageCommand 管理态：全部是对数字员工配置本体的读写，含不可逆操作。
func newDeapManageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "manage",
		Short:             "数字员工生命周期管理",
		Long:              "钉钉数字员工管理：创建草稿、查询详情与列表、获取临时 DWS 授权码、全量覆写草稿、发布与删除。授权码属于高敏感凭证；save-draft / publish / delete 均为高影响写操作，先 --dry-run 确认再加 --yes。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              groupRunE,
	}
	newGroupCommand(cmd)
	cmd.AddCommand(
		newDeapAgentCreateCommand(),
		newDeapAgentDetailCommand(),
		newDeapAgentListCommand(),
		newDeapAgentAuthCodeCommand(),
		newDeapAgentSaveDraftCommand(),
		newDeapAgentPublishCommand(),
		newDeapAgentDeleteCommand(),
	)
	return cmd
}

// newDeapAgentAuthCodeCommand 获取指定数字员工的短期 DWS 授权信息。
// dwsAuthCode 等字段由服务端在 data 中返回，CLI 不解析、不缓存，也不改变响应 envelope。
func newDeapAgentAuthCodeCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "get-dws-auth-code",
		Short:     "获取数字员工的临时 DWS 授权码",
		Long:      "按 agentUuid 获取数字员工的临时 DWS 授权信息。clientId 是可选的授权应用 ID；不传时由服务端选择默认应用。服务端响应中的 success、errorCode、errorMsg 和 data 会原样输出；data 包含 dwsClientId、uid、dwsAuthCode、staffId、orgId。dwsAuthCode 是高敏感短期凭证，不得写入文档、日志、命令历史、缓存或代码库。",
		Tool:      deapAgentAuthCodeTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工 ID", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "client-id", Usage: "用于授权的应用 ID；不传时由服务端选择默认应用", Bind: "clientId", Trim: true, OmitEmpty: true},
		},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "high",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: deapAgentAuthCodeTool,
				CanonicalPath: "dingtalk-tag.get_dws_auth_code",
				CLIPath:       "dingtalk-tag manage get-dws-auth-code", PrimaryCLIPath: "dingtalk-tag manage get-dws-auth-code",
				Group: "manage",
			},
			Description: "按 agentUuid 获取数字员工的临时 DWS 授权信息；clientId 可选。服务端返回 success、errorCode、errorMsg，以及包含 dwsClientId、uid、dwsAuthCode、staffId、orgId 的 data。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentAuthCodeTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定数字员工的临时 DWS 授权码",
				UseWhen:      []string{"已知 agentUuid，需要以该数字员工身份短期调用 DWS 时"},
				AvoidWhen:    []string{"普通用户 DWS 登录使用 auth login；只管理数字员工配置时不需要获取授权码"},
				Examples:     []string{"dws dingtalk-tag manage get-dws-auth-code --agent-uuid <agentUuid> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "agent-uuid", Property: "agentUuid"},
				{Name: "client-id", Property: "clientId"},
			},
		},
	})
}

// newDeapRunCommand 执行态：全部是只读，且均只按来源定位（不接 runId）。
func newDeapRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "run",
		Short:             "数字员工执行观测",
		Long:              "DEAP 数字员工的执行观测：按来源反查执行状态（run-status）与完整模型链路（trace）。两个命令均只按 --source-id + --source-type 定位，不接 runId（调用方拿不到，runId 是出参）。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              groupRunE,
	}
	newGroupCommand(cmd)
	cmd.AddCommand(
		newDeapAgentRunStatusCommand(),
		newDeapAgentTraceCommand(),
	)
	return cmd
}

func newDeapAgentCreateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "create",
		Short:     "创建草稿态数字员工",
		Long:      "创建草稿态 DEAP 数字员工并返回生成的 agentUuid。创建不会自动发布；创建成功后应补齐头像、岗位、响应模式和人设提示词，再调用发布工具。name 和 description 必填，identity 由 MCP 可信注入；员工档案可用独立 flag 或 profile-json 提供，同时提供时独立 flag 覆盖 JSON 同名字段。mainProgramType 仅支持 open_code、local_agent；无特殊要求时默认不传，由 OpenAPI 按 open_code 处理；允许显式传 open_code，只有明确接入本地 Agent/DSH 时才传 local_agent。",
		Tool:      deapAgentCreateTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "name", Usage: "数字员工名称，同组织内唯一（最多 30 个 Unicode 码点）", Bind: "name", Required: true, Trim: true},
			{Name: "description", Usage: "数字员工职责描述（最多 300 个 Unicode 码点）", Bind: "description", Required: true, Trim: true},
			{Name: "dept-id", Usage: "归属部门 ID", Bind: "deptId", Trim: true, OmitEmpty: true},
			{Name: "dept-name", Usage: "归属部门名称", Bind: "deptName", Trim: true, OmitEmpty: true},
			{Name: "icon", Usage: "头像地址或 OSS objectPath", Bind: "icon", Trim: true, OmitEmpty: true},
			{Name: "profile-json", Usage: "digitalTagEmployeeProfile JSON 对象；responseMode 支持单值或英文逗号分隔的双值组合；独立档案 flag 会覆盖同名字段", Bind: "digitalTagEmployeeProfile", Trim: true, OmitEmpty: true, Format: "json", Transform: deapAgentProfileJSON, SchemaDescription: "数字员工档案 JSON；仅接收 employeeNo、positionName、directSupervisorUid、mainProgramType、responseMode；mainProgramType 仅支持 open_code、local_agent，无特殊要求时省略（服务端默认 open_code），也允许显式传 open_code，明确接入本地 Agent/DSH 时传 local_agent；responseMode 支持 mention_only、targeted_proactive 或二者组合；独立档案参数优先"},
			{Name: "employee-no", Usage: "数字员工工号（最多 64 个 Unicode 码点）", Bind: "digitalTagEmployeeProfile.employeeNo", Trim: true, OmitEmpty: true},
			{Name: "position-name", Usage: "岗位名称（最多 128 个 Unicode 码点，发布前必填）", Bind: "digitalTagEmployeeProfile.positionName", Trim: true, OmitEmpty: true},
			{Name: "supervisor-uid", Usage: "直属上级钉钉 uid", Bind: "digitalTagEmployeeProfile.directSupervisorUid", Trim: true, OmitEmpty: true},
			{Name: "main-program-type", Usage: "可选主程序类型：open_code 或 local_agent；无特殊要求时省略（OpenAPI 默认 open_code），也可显式传 open_code；接入本地 Agent/DSH 时传 local_agent", Bind: "digitalTagEmployeeProfile.mainProgramType", Trim: true, OmitEmpty: true, Enum: deapAgentMainProgramTypeValues},
			{Name: "response-mode", Usage: "响应模式：mention_only、targeted_proactive，或英文逗号分隔的组合 mention_only,targeted_proactive（发布前必填）", Bind: "digitalTagEmployeeProfile.responseMode", Trim: true, OmitEmpty: true, Transform: deapAgentResponseMode},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := deapAgentMaxRunes(cmd, "name", 30); err != nil {
				return err
			}
			if err := deapAgentMaxRunes(cmd, "description", 300); err != nil {
				return err
			}
			if err := deapAgentMaxRunes(cmd, "employee-no", 64); err != nil {
				return err
			}
			return deapAgentMaxRunes(cmd, "position-name", 128)
		},
		Call: deapAgentCallWithProfile,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "create_digital_employee",
				CanonicalPath: "dingtalk-tag.create_digital_employee",
				CLIPath:       "dingtalk-tag manage create", PrimaryCLIPath: "dingtalk-tag manage create",
				Group: "manage",
			},
			Description: "创建草稿态 DEAP 数字员工并返回生成的 agentUuid。创建不会自动发布；创建成功后应补齐头像、岗位、响应模式和人设提示词，再调用发布工具。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentCreateTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "创建新的草稿态 DEAP 数字员工",
				UseWhen:      []string{"需要从零创建数字员工并获得 agentUuid 时"},
				AvoidWhen:    []string{"已有 agentUuid 只需修改草稿时使用 save-draft", "创建普通开放平台应用时使用 dev app create"},
				Examples:     []string{`dws dingtalk-tag manage create --name "值班助手" --description "处理值班问题" --dept-id dept-1 --dept-name "值班组" --position-name "值班员" --response-mode mention_only --dry-run --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "profile-json", Property: "digitalTagEmployeeProfile", InterfaceType: "object"},
				{Name: "employee-no", Property: "digitalTagEmployeeProfile.employeeNo"},
				{Name: "position-name", Property: "digitalTagEmployeeProfile.positionName"},
				{Name: "supervisor-uid", Property: "digitalTagEmployeeProfile.directSupervisorUid"},
				{Name: "main-program-type", Property: "digitalTagEmployeeProfile.mainProgramType", Enum: deapAgentMainProgramTypeValues, Description: "可选主程序类型；无特殊要求时省略（OpenAPI 默认 open_code），也允许显式传 open_code；明确接入本地 Agent/DSH 时传 local_agent"},
				{Name: "response-mode", Property: "digitalTagEmployeeProfile.responseMode", Enum: deapAgentResponseModeValues, Description: "响应模式；支持 mention_only、targeted_proactive，或英文逗号分隔的双值组合 mention_only,targeted_proactive"},
			},
		},
	})
}

func newDeapAgentDetailCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "detail",
		Short:     "查询数字员工管理态详情",
		Long:      "按 agentUuid 查询 DEAP 数字员工详情。--type draft 返回未发布草稿及其 Skill/MCP 引用配置，published 返回线上已发布配置，默认 draft；返回 status 中 online 表示已发布，dev/offline 表示未发布。仅有该数字员工管理权限的调用人可读；ID 不存在或不是数字员工时返回 NOT_FOUND。保存草稿前应先查询 draft；iconUrl 可能是带签名的临时地址，长期保存应使用 icon。",
		Tool:      deapAgentDetailTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工 ID", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "type", Usage: "详情来源：draft（未发布草稿）或 published（线上已发布配置）", Bind: "type", Default: "draft", ArgDefault: "draft", Trim: true, Enum: []string{"draft", "published"}},
		},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "get_digital_employee_detail",
				CanonicalPath: "dingtalk-tag.get_digital_employee_detail",
				CLIPath:       "dingtalk-tag manage detail", PrimaryCLIPath: "dingtalk-tag manage detail",
				Group: "manage",
			},
			Description: "按 agentUuid 查询数字员工 draft 或 published 详情及其 Skill/MCP 引用配置。type 默认 draft；返回 status 中 online 表示已发布，dev/offline 表示未发布。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentDetailTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "按 agentUuid 查询数字员工草稿或已发布详情",
				UseWhen:      []string{"需要读取数字员工完整配置、发布前检查或保存草稿前回读时"},
				AvoidWhen:    []string{"需要分页查找多个数字员工时使用 list", "只查一次运行状态时使用 run-status"},
				Examples: []string{
					"dws dingtalk-tag manage detail --agent-uuid <agentUuid> --type draft --format json",
					"dws dingtalk-tag manage detail --agent-uuid <agentUuid> --type published --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "type", Property: "type", Enum: []string{"draft", "published"}},
			},
		},
	})
}

func newDeapAgentListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "list",
		Short:     "分页查询数字员工",
		Long:      "分页查询当前身份有权查看的数字员工。管理员可看本组织全部，非管理员只返回本人参与的数字员工；支持按名称、岗位或工号模糊搜索，也可按 mainProgramType 筛选。mainProgramType 仅支持 open_code、local_agent；不传表示不过滤。page 和 page-size 必须大于等于 1。",
		Tool:      deapAgentListTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "keyword", Usage: "按名称、岗位或工号模糊匹配", Bind: "keyword", Trim: true, OmitEmpty: true},
			{Name: "main-program-type", Usage: "按主程序类型筛选：open_code 或 local_agent；不传表示不过滤", Bind: "mainProgramType", Trim: true, OmitEmpty: true, Enum: deapAgentMainProgramTypeValues},
			{Name: "page", Usage: "页码", Bind: "page", Kind: LeafInt, Default: "1", ArgDefault: "1"},
			{Name: "page-size", Usage: "每页数量", Bind: "pageSize", Kind: LeafInt, Default: "20", ArgDefault: "20"},
		},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			page, _ := cmd.Flags().GetInt("page")
			if page < 1 {
				return apperrors.NewValidation("参数 --page 不能小于 1")
			}
			pageSize, _ := cmd.Flags().GetInt("page-size")
			if pageSize < 1 {
				return apperrors.NewValidation("参数 --page-size 不能小于 1")
			}
			return nil
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "list_digital_employees",
				CanonicalPath: "dingtalk-tag.list_digital_employees",
				CLIPath:       "dingtalk-tag manage list", PrimaryCLIPath: "dingtalk-tag manage list",
				Group: "manage",
			},
			Description: "分页查询当前身份有权查看的 DEAP 数字员工。管理员可查看本组织全部，非管理员只返回自己参与的数字员工；支持按名称、岗位或工号模糊搜索，也可按 open_code、local_agent 主程序类型筛选。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentListTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "分页查找当前用户可管理或参与的数字员工",
				UseWhen:      []string{"需要按名称、岗位或工号查找数字员工，或尚不知道 agentUuid 时"},
				AvoidWhen:    []string{"已知 agentUuid 需要完整配置时使用 detail", "需要查询运行记录时使用 run-status"},
				Examples:     []string{`dws dingtalk-tag manage list --keyword "值班" --main-program-type local_agent --page 1 --page-size 20 --format json`},
			},
		},
	})
}

func newDeapAgentSaveDraftCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "save-draft",
		Short:     "全量覆写数字员工草稿",
		Long:      "全量覆写指定数字员工的基础草稿但不发布。既有基础字段未传会被清空；新增 Skill/MCP 文件参数不传时保持原配置，只有显式提供空数组文件才清空。档案字段可用独立 flag 或 profile-json 提供，同时提供时独立 flag 覆盖 JSON 同名字段。mainProgramType 仅支持 open_code、local_agent；无特殊要求时默认不传，由 OpenAPI 按 open_code 处理；允许显式传 open_code，只有明确保持或切换为本地 Agent/DSH 模式时才传 local_agent。修改前必须先查询 detail、保留其它全部仍需配置的字段，再整体提交。请先 --dry-run 检查参数，再经用户确认加 --yes。",
		Tool:      deapAgentSaveDraftTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工 ID", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "name", Usage: "数字员工名称（最多 30 个 Unicode 码点）", Bind: "name", Trim: true, OmitEmpty: true},
			{Name: "description", Usage: "数字员工职责描述（最多 300 个 Unicode 码点）；不传会被清空", Bind: "description", Trim: true, OmitEmpty: true},
			{Name: "icon", Usage: "头像地址或 objectPath；不传会被清空，不要持久化详情中的临时 iconUrl", Bind: "icon", Trim: true, OmitEmpty: true},
			{Name: "dept-id", Usage: "归属部门 ID；不传会被清空", Bind: "deptId", Trim: true, OmitEmpty: true},
			{Name: "dept-name", Usage: "归属部门名称", Bind: "deptName", Trim: true, OmitEmpty: true},
			{Name: "prompt", Usage: "人设/System Prompt（最多 5000 个 Unicode 码点）", Bind: "prompt", Trim: true, OmitEmpty: true},
			{Name: "profile-json", Usage: "digitalTagEmployeeProfile JSON 对象；responseMode 支持单值或英文逗号分隔的双值组合；独立档案 flag 会覆盖同名字段", Bind: "digitalTagEmployeeProfile", Trim: true, OmitEmpty: true, Format: "json", Transform: deapAgentProfileJSON},
			{Name: "employee-no", Usage: "数字员工工号（最多 64 个 Unicode 码点）", Bind: "digitalTagEmployeeProfile.employeeNo", Trim: true, OmitEmpty: true},
			{Name: "position-name", Usage: "岗位名称（最多 128 个 Unicode 码点，发布前必填）", Bind: "digitalTagEmployeeProfile.positionName", Trim: true, OmitEmpty: true},
			{Name: "supervisor-uid", Usage: "直属上级钉钉 uid", Bind: "digitalTagEmployeeProfile.directSupervisorUid", Trim: true, OmitEmpty: true},
			{Name: "main-program-type", Usage: "可选主程序类型：open_code 或 local_agent；无特殊要求时省略（OpenAPI 默认 open_code），也可显式传 open_code；保持或切换为本地 Agent/DSH 模式时传 local_agent", Bind: "digitalTagEmployeeProfile.mainProgramType", Trim: true, OmitEmpty: true, Enum: deapAgentMainProgramTypeValues},
			{Name: "response-mode", Usage: "响应模式：mention_only、targeted_proactive，或英文逗号分隔的组合 mention_only,targeted_proactive（发布前必填）", Bind: "digitalTagEmployeeProfile.responseMode", Trim: true, OmitEmpty: true, Transform: deapAgentResponseMode},
			{Name: "skills-file", Usage: "Skill 草稿配置 JSON 数组文件，元素为 skillId/enabled/attributes；不传保持原配置，显式空数组才清空", Bind: "skillsFile", Trim: true, OmitEmpty: true},
			{Name: "mcps-file", Usage: "MCP 草稿配置 JSON 数组文件，元素为 mcpId/enabled/config；不传保持原配置，显式空数组才清空，凭据只允许使用安全引用", Bind: "mcpsFile", Trim: true, OmitEmpty: true},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "idempotent",
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := deapAgentMaxRunes(cmd, "name", 30); err != nil {
				return err
			}
			if err := deapAgentMaxRunes(cmd, "description", 300); err != nil {
				return err
			}
			if err := deapAgentMaxRunes(cmd, "prompt", 5000); err != nil {
				return err
			}
			if err := deapAgentMaxRunes(cmd, "employee-no", 64); err != nil {
				return err
			}
			return deapAgentMaxRunes(cmd, "position-name", 128)
		},
		Call: deapAgentCallWithProfileAndDraftFiles,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "update_digital_employee_draft",
				CanonicalPath: "dingtalk-tag.update_digital_employee_draft",
				CLIPath:       "dingtalk-tag manage save-draft", PrimaryCLIPath: "dingtalk-tag manage save-draft",
				Group: "manage",
			},
			Description: "全量覆写指定数字员工的草稿但不发布。未传字段会被清空；增量修改前必须先查询详情、保留全部仍需配置的字段，再整体提交。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentSaveDraftTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "全量覆写数字员工草稿配置",
				UseWhen:      []string{"已先读取完整详情、明确保留字段，并需要保存尚未发布的数字员工草稿时"},
				AvoidWhen:    []string{"只改单字段但尚未 detail 回读全量时不要执行", "准备直接上线时仍需另行执行 publish"},
				Examples:     []string{`dws dingtalk-tag manage save-draft --agent-uuid <agentUuid> --name "值班助手" --description "处理值班问题" --dry-run --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "profile-json", Property: "digitalTagEmployeeProfile", InterfaceType: "object"},
				{Name: "employee-no", Property: "digitalTagEmployeeProfile.employeeNo"},
				{Name: "position-name", Property: "digitalTagEmployeeProfile.positionName"},
				{Name: "supervisor-uid", Property: "digitalTagEmployeeProfile.directSupervisorUid"},
				{Name: "main-program-type", Property: "digitalTagEmployeeProfile.mainProgramType", Enum: deapAgentMainProgramTypeValues, Description: "可选主程序类型；无特殊要求时省略（OpenAPI 默认 open_code），也允许显式传 open_code；保持或切换为本地 Agent/DSH 模式时传 local_agent"},
				{Name: "response-mode", Property: "digitalTagEmployeeProfile.responseMode", Enum: deapAgentResponseModeValues, Description: "响应模式；支持 mention_only、targeted_proactive，或英文逗号分隔的双值组合 mention_only,targeted_proactive"},
				{Name: "skills-file", Property: "skills", InterfaceType: "array"},
				{Name: "mcps-file", Property: "mcps", InterfaceType: "array"},
			},
		},
	})
}

func newDeapAgentPublishCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "publish",
		Short:     "发布数字员工",
		Long:      "发布指定数字员工当前已保存的草稿。发布不携带配置；名称、头像、描述、组织、岗位、响应模式或人设缺失时返回 INVALID_PARAM。这是高影响操作，真实执行前必须确认。",
		Tool:      deapAgentPublishTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工 ID", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "allow-join-group", Usage: "可选，是否允许加入群聊", Bind: "allowJoinGroup", Kind: LeafBool},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "publish_digital_employee",
				CanonicalPath: "dingtalk-tag.publish_digital_employee",
				CLIPath:       "dingtalk-tag manage publish", PrimaryCLIPath: "dingtalk-tag manage publish",
				Group: "manage",
			},
			Description: "发布指定数字员工当前已保存的草稿。发布不携带配置；名称、头像、描述、组织、岗位、响应模式或人设缺失时返回 INVALID_PARAM。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentPublishTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "校验并发布数字员工，使草稿配置进入线上生效流程",
				UseWhen:      []string{"草稿配置已完成并经用户明确确认，需要发布到钉钉时"},
				AvoidWhen:    []string{"只需保存未发布草稿时使用 save-draft", "发布必填配置不完整时先 detail 检查并补齐"},
				Examples:     []string{"dws dingtalk-tag manage publish --agent-uuid <agentUuid> --dry-run --format json"},
			},
		},
	})
}

func newDeapAgentDeleteCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "delete",
		Short:     "删除数字员工",
		Long:      "删除指定 DEAP 数字员工。该操作不可逆且可能包含跨系统副作用；失败时不要盲目重试，应先查询确认数字员工是否仍存在。真实执行前必须确认。",
		Tool:      deapAgentDeleteTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工 ID", Bind: "agentUuid", Required: true, Trim: true},
		},
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "delete_digital_employee",
				CanonicalPath: "dingtalk-tag.delete_digital_employee",
				CLIPath:       "dingtalk-tag manage delete", PrimaryCLIPath: "dingtalk-tag manage delete",
				Group: "manage",
			},
			Description: "删除指定 DEAP 数字员工。该操作不可逆且可能包含跨系统副作用；失败时不要盲目重试，应先查询确认数字员工是否仍存在。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentDeleteTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "永久删除指定数字员工",
				UseWhen:      []string{"用户明确要求删除数字员工，并已确认 agentUuid 与不可逆影响时"},
				AvoidWhen:    []string{"只需停止发布或暂时修改配置时不要删除", "未确认目标与影响范围时不要执行"},
				Examples:     []string{"dws dingtalk-tag manage delete --agent-uuid <agentUuid> --dry-run --format json"},
			},
		},
	})
}

// newDeapAgentRunStatusCommand 只按来源定位。不提供 --run-id：调用方手上只会有来源侧原始 ID
// （钉钉开放态消息 ID、群感知规则 ID），拿不到 runId；留个填不了的 flag 只会诱导瞎传。
// runId 作为出参返回，供人工去 SLS / Langfuse 控制台比对。
func newDeapAgentRunStatusCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "run-status",
		Short:     "查询数字员工执行状态",
		Long:      "查询指定数字员工的执行状态。--agent-uuid、--source-id、--source-type 均必填，按来源反查本次执行。返回 result（1 成功 / -1 失败 / 0 运行中 / 2 中止）、runId 与 messageId。trigger_rule 的来源 ID 可能对应多次执行，只返回最新一次。",
		Tool:      deapAgentRunStatusTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工唯一标识", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "source-id", Usage: "来源侧原始 ID；im_message 传钉钉开放态 openMessageId（不是 DWS openTaskId），trigger_rule 传群感知规则 ID", Bind: "sourceId", Required: true, Trim: true},
			{Name: "source-type", Usage: "来源类型", Bind: "sourceType", Required: true, Trim: true, Enum: deapAgentSourceTypes},
		},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "query_de_run_status",
				CanonicalPath: "dingtalk-tag.query_de_run_status",
				CLIPath:       "dingtalk-tag run run-status", PrimaryCLIPath: "dingtalk-tag run run-status",
				Group: "run",
			},
			Description: "数字员工执行状态查询。agentUuid、sourceId、sourceType 均必填，按来源反查本次执行。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentRunStatusTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "按来源 ID 查数字员工单次执行状态",
				UseWhen:      []string{"持有钉钉开放态消息 ID 或群感知规则 ID，需要判断执行结果时"},
				AvoidWhen: []string{
					"需要完整模型链路时使用 trace",
					"手上只有 DWS openTaskId 时先查发送状态换成 openMessageId",
				},
				Examples: []string{
					"dws dingtalk-tag run run-status --agent-uuid <agentUuid> --source-id <openMessageId> --source-type im_message --format json",
					"dws dingtalk-tag run run-status --agent-uuid <agentUuid> --source-id <perceptionRuleId> --source-type trigger_rule --format json",
				},
			},
		},
	})
}

// newDeapAgentTraceCommand 入参与 run-status 一致（只按来源定位）：服务端先用 sourceId 反查 traceId，
// 再过两级权限，最后取 Langfuse 原文。不接 runId：调用方拿不到。
func newDeapAgentTraceCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "trace",
		Short:     "查询数字员工执行 Trace",
		Long:      "查询指定数字员工的执行 Trace。--agent-uuid、--source-id、--source-type 均必填（与 run-status 一致）。返回内容可能包含完整对话和模型输入输出；服务端会先执行管理者/触发人两级授权，无权时返回 NO_PERMISSION。",
		Tool:      deapAgentTraceTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工唯一标识", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "source-id", Usage: "来源侧原始 ID；im_message 传钉钉开放态 openMessageId（不是 DWS openTaskId），trigger_rule 传群感知规则 ID", Bind: "sourceId", Required: true, Trim: true},
			{Name: "source-type", Usage: "来源类型", Bind: "sourceType", Required: true, Trim: true, Enum: deapAgentSourceTypes},
		},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "high",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "query_de_trace",
				CanonicalPath: "dingtalk-tag.query_de_trace",
				CLIPath:       "dingtalk-tag run trace", PrimaryCLIPath: "dingtalk-tag run trace",
				Group: "run",
			},
			Description: "数字员工执行 Trace 查询。agentUuid、sourceId、sourceType 均必填，服务端先执行管理者/触发人两级授权。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentTraceTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "在服务端授权后查询数字员工完整执行 Trace",
				UseWhen:      []string{"需要排查某次执行的完整模型链路（提示词、工具调用、模型输入输出）时"},
				AvoidWhen: []string{
					"只需执行成败结果时使用 run-status（本命令返回完整对话内容，敏感度更高）",
				},
				Examples: []string{
					"dws dingtalk-tag run trace --agent-uuid <agentUuid> --source-id <openMessageId> --source-type im_message --format json",
				},
			},
		},
	})
}

func deapAgentCallWithProfile(_ *cobra.Command, tool string, args map[string]any) error {
	profileValue, profileProvided := args["digitalTagEmployeeProfile"]
	profile := map[string]any{}
	if existing, ok := profileValue.(map[string]any); ok {
		for key, value := range existing {
			profile[key] = value
		}
	}
	for _, field := range []struct {
		argument string
		property string
	}{
		{"digitalTagEmployeeProfile.employeeNo", "employeeNo"},
		{"digitalTagEmployeeProfile.positionName", "positionName"},
		{"digitalTagEmployeeProfile.directSupervisorUid", "directSupervisorUid"},
		{"digitalTagEmployeeProfile.mainProgramType", "mainProgramType"},
		{"digitalTagEmployeeProfile.responseMode", "responseMode"},
	} {
		if value, ok := args[field.argument]; ok {
			profile[field.property] = value
			delete(args, field.argument)
		}
	}
	if profileProvided || len(profile) > 0 {
		args["digitalTagEmployeeProfile"] = profile
	}
	return callMCPToolOnServer(deapAgentServerID, tool, args)
}

func deapAgentMCPInterface(tool string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{
		Mode: contract.InterfaceModeMCP, Availability: contract.InterfaceAvailable,
		Ref: &contract.InterfaceRefSpec{ProductID: deapAgentServerID, RPCName: tool},
	}
}

func deapAgentNoArgs(cmd *cobra.Command) {
	cmd.Args = cobra.NoArgs
}

func deapAgentMaxRunes(cmd *cobra.Command, flagName string, max int) error {
	value, _ := cmd.Flags().GetString(flagName)
	value = strings.TrimSpace(value)
	if value != "" && utf8.RuneCountInString(value) > max {
		return apperrors.NewValidation(fmt.Sprintf("参数 --%s 最多允许 %d 个 Unicode 码点", flagName, max))
	}
	return nil
}

func deapAgentJSONObject(raw string) (any, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, apperrors.NewValidation("参数必须是合法 JSON 对象：" + err.Error())
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, apperrors.NewValidation("参数必须解码为 JSON 对象")
	}
	return object, nil
}

func deapAgentJSONArray(raw string) (any, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, apperrors.NewValidation("参数必须是合法 JSON 数组：" + err.Error())
	}
	array, ok := value.([]any)
	if !ok {
		return nil, apperrors.NewValidation("参数必须解码为 JSON 数组")
	}
	return array, nil
}

func deapAgentResponseMode(raw string) (any, error) {
	normalized, err := deapAgentNormalizeResponseMode(raw)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func deapAgentNormalizeResponseMode(raw string) (string, error) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 || len(parts) > 2 {
		return "", deapAgentInvalidResponseMode()
	}

	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		mode := strings.TrimSpace(part)
		if mode == "" || seen[mode] {
			return "", deapAgentInvalidResponseMode()
		}
		switch mode {
		case deapAgentResponseModeMentionOnly, deapAgentResponseModeTargetedProactive:
			seen[mode] = true
		default:
			return "", deapAgentInvalidResponseMode()
		}
	}

	if seen[deapAgentResponseModeMentionOnly] && seen[deapAgentResponseModeTargetedProactive] {
		return deapAgentResponseModeCombined, nil
	}
	if seen[deapAgentResponseModeMentionOnly] {
		return deapAgentResponseModeMentionOnly, nil
	}
	return deapAgentResponseModeTargetedProactive, nil
}

func deapAgentInvalidResponseMode() error {
	return apperrors.NewValidation("响应模式只允许 mention_only、targeted_proactive，或二者用英文逗号分隔的组合；不能包含空项、重复项或其它值")
}

func deapAgentProfileJSON(raw string) (any, error) {
	value, err := deapAgentJSONObject(raw)
	if err != nil {
		return nil, err
	}
	profile := value.(map[string]any)
	allowed := map[string]bool{
		"employeeNo": true, "positionName": true,
		"directSupervisorUid": true, "mainProgramType": true, "responseMode": true,
	}
	for key := range profile {
		if !allowed[key] {
			return nil, apperrors.NewValidation(fmt.Sprintf("profile-json 不接受字段 %q", key))
		}
	}
	if employeeNo, ok := profile["employeeNo"].(string); ok && utf8.RuneCountInString(employeeNo) > 64 {
		return nil, apperrors.NewValidation("profile.employeeNo 最多允许 64 个 Unicode 码点")
	}
	if position, ok := profile["positionName"].(string); ok && utf8.RuneCountInString(position) > 128 {
		return nil, apperrors.NewValidation("profile.positionName 最多允许 128 个 Unicode 码点")
	}
	if rawProgramType, exists := profile["mainProgramType"]; exists {
		programType, ok := rawProgramType.(string)
		if !ok {
			return nil, apperrors.NewValidation("profile.mainProgramType 必须是字符串")
		}
		programType = strings.TrimSpace(programType)
		if programType == "" {
			return nil, apperrors.NewValidation("profile.mainProgramType 不能为空")
		}
		if programType != deapAgentMainProgramTypeOpenCode &&
			programType != deapAgentMainProgramTypeLocalAgent {
			return nil, deapAgentInvalidMainProgramType()
		}
		profile["mainProgramType"] = programType
	}
	if rawMode, exists := profile["responseMode"]; exists {
		mode, ok := rawMode.(string)
		if !ok {
			return nil, apperrors.NewValidation("profile.responseMode 必须是字符串")
		}
		normalized, normalizeErr := deapAgentNormalizeResponseMode(mode)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		profile["responseMode"] = normalized
	}
	return profile, nil
}

func deapAgentInvalidMainProgramType() error {
	return apperrors.NewValidation("主程序类型只允许 open_code 或 local_agent；a2a 暂不支持")
}
