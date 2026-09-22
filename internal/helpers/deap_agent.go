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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	deapAgentServerID = "deap-dev"

	// dingtalkTagProductID 是钉钉数字员工的用户可见产品标识；内部 MCP server 仍为 deap-dev。
	// 契约要求 CanonicalPath 严格等于 <ProductID>.<Name>。
	dingtalkTagProductID = "dingtalk-tag"

	deapAgentCreateTool        = "create_digital_employee"
	deapAgentDetailTool        = "get_digital_employee_detail"
	deapAgentListTool          = "list_digital_employees"
	deapAgentAuthCodeTool      = "get_dws_auth_code"
	deapAgentSaveDraftTool     = "update_digital_employee_draft"
	deapAgentPublishTool       = "publish_digital_employee"
	deapAgentDeleteTool        = "delete_digital_employee"
	deapAgentSetVisibilityTool = "set_visibility"
	deapAgentRunStatusTool     = "query_de_run_status"
	deapAgentTraceTool         = "query_de_trace"

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
	PreviewKind: contract.DryRunPreviewRequest,
	RemoteReads: false,
}

var deapAgentPlanDryRun = &contract.DryRunSpec{
	PreviewKind: contract.DryRunPreviewPlan,
	RemoteReads: false,
}

// deapAgentSourceTypes 是 chat-2 message_source 表登记的来源类型白名单，与
// MessageSourceType 枚举逐字一致；服务端对未知类型直接报参数错，不做兜底解析。
// im_message 的 sourceId 是钉钉开放态消息 ID（openMessageId），单聊/群@/群感知三条
// 链路含义一致；trigger_rule 的 sourceId 是群感知规则 ID。
var deapAgentSourceTypes = []string{"im_message", "trigger_rule"}

func init() {
	RegisterPublicNamed("dingtalk-tag", func() Handler {
		return deapHandler{}
	})
}

// deapHandler 挂载顶级命令 `dws dingtalk-tag`：
//
//	dingtalk-tag manage       数字员工管理（创建 / 详情 / 列表 / DWS 登录 / 草稿 / 发布 / 删除）
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
			AgentSummary: "创建和管理数字员工、登录数字员工 DWS 并保存独立 Profile、查询执行状态，并把已有本地数字员工接入 DSH 或其他本地 Agent",
			UseWhen: []string{
				"创建、修改、发布或删除 DEAP 数字员工",
				"A2A 或其他场景需要登录指定数字员工的 DWS",
				"查数字员工某次执行的状态或完整模型链路",
				"创建或查询可配置到数字员工草稿的 Skill/MCP 资源",
				"把已有且已发布的 local_agent 数字员工接入 DSH 或其他支持的本地 Agent",
			},
			AvoidWhen: []string{
				"开放平台应用、机器人配置与版本发布用 dev；普通企业消息收发用 chat",
			},
		},
	})
	root := &cobra.Command{
		Use:               "dingtalk-tag",
		Short:             "DEAP 平台",
		Long:              "钉钉数字员工命令组：manage 负责数字员工生命周期，并通过 login 为 A2A、仅保存 Profile 或其他场景完成数字员工 DWS 登录；run 负责执行状态与 trace，capability 负责 Skill/MCP 能力资源。connect 将已有且已发布的 local_agent 数字员工接入 DSH 或其他支持的本地 Agent，status/list/stop/restart/unbind 管理本机绑定；channel 提供受限机器协议。固定调用 MCP product/server deap-dev；identity.corpId/userId 由可信登录态注入且不对 CLI 暴露。端点跟随当前 MCP 环境自动选择规范网关；DINGTALK_DEAP_DEV_MCP_URL 仅用于本地调试覆盖。",
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
		Long:              "钉钉数字员工管理：创建草稿、查询详情与列表、通过 login 完成数字员工 DWS 登录并保存独立 Profile、更新草稿、发布与删除。login 用于 A2A 或其他需要登录数字员工 DWS 的场景；企业本地 Agent 接入使用顶层 connect。对用户统一使用 userId 表示人员。login 内部消费高敏感授权码且不输出凭证；save-draft / publish / delete 均为高影响写操作，先 --dry-run 确认再加 --yes。",
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
		newDeapAgentLoginCommand(),
		newDeapAgentSaveDraftCommand(),
		newDeapAgentSetVisibilityCommand(),
		newDeapAgentPublishCommand(),
		newDeapAgentDeleteCommand(),
	)
	return cmd
}

// newDeapAgentLoginCommand 为指定数字员工完成一次受管 DWS 登录。
func newDeapAgentLoginCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Use:           "login",
		Short:         "登录数字员工的 DWS 并保存独立 Profile",
		Long:          "按 agentUuid 为已发布数字员工完成一步登录：内部申请临时 AuthCode，使用同次响应中的 dwsClientId 换票，在线核验员工身份，并保存精确 corpId:userId Profile。命令不会输出 AuthCode 或 Token，也不会切换当前主管 Profile。单次使用通过 dws --profile <corpId:userId> <command>；需要切换默认账号时执行 dws profile use <corpId:userId>。适用于 A2A、仅保存数字员工 Profile 或其他 DWS 登录场景，不绑定 local_agent、DSH 或 Bridge；企业接入本地 Agent/DSH 应使用 dws dingtalk-tag connect。",
		PostMount:     deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工 ID", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "client-id", Usage: "用于授权的应用 ID；不传时由服务端选择默认应用", Bind: "clientId", Trim: true, OmitEmpty: true},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Validate: func(cmd *cobra.Command, _ []string) error {
			if commandDryRun(cmd) {
				return nil
			}
			if deps == nil || deps.Caller == nil {
				return apperrors.NewInternal("MCP caller is not initialized")
			}
			return nil
		},
		RunE: runDeapAgentLogin,
		Contract: LeafContract{
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{"success"},
				DataSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","description":"登录或预检状态"},"agentUuid":{"type":"string","description":"数字员工 ID"},"dwsProfile":{"type":"string","description":"精确员工 Profile"},"currentProfilePreserved":{"type":"boolean","description":"是否保留当前主管身份"},"useOnce":{"type":"string","description":"单次使用员工身份的命令"},"selectProfile":{"type":"string","description":"切换员工身份的提示命令"},"steps":{"type":"array","description":"预检计划步骤","items":{"type":"string"}}}}`),
			},
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "login",
				CanonicalPath: "dingtalk-tag.login",
				CLIPath:       "dingtalk-tag manage login", PrimaryCLIPath: "dingtalk-tag manage login",
				Group: "manage",
			},
			Description: "按 agentUuid 完成数字员工受管登录、在线身份核验并保存精确 DWS Profile；AuthCode 与 Token 不进入普通输出。",
			DryRun:      deapAgentPlanDryRun,
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "发布详情、DEAP 授权、DWS managed exchange、在线身份核验与本地 Profile 持久化的受控编排"},
			Selection: contract.SelectionSpec{
				AgentSummary: "登录指定数字员工的 DWS 并保存独立 Profile",
				UseWhen:      []string{"已知 agentUuid，A2A、仅保存 Profile 或其他场景需要登录该数字员工的 DWS"},
				AvoidWhen:    []string{"企业接入本地 Agent/DSH 应使用 dingtalk-tag connect；普通用户 DWS 登录使用 auth login；只管理数字员工配置时不需要登录"},
				Examples:     []string{"dws dingtalk-tag manage login --agent-uuid <agentUuid> --format json"},
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
		Long:      "创建数字员工草稿并返回 agentUuid，不自动发布。name 和 description 必填；dept-id 可选，不传时由 OpenAPI 补齐操作人主任职部门。avatar-url 可传公网 HTTP(S) 地址或本地图片路径；本地图片复用 Skill 文件上传封装，在 CLI 内部上传为 OSS 地址后写入草稿。用户传入的主管标识始终是 userId。必须显式传入 --type open_code 或 local_agent，对应 MCP 字段 digitalTagEmployeeProfile.type；缺失或空值由 CLI 拦截，不自动选择类型。未提供 response-mode 或值为空时，CLI 默认发送 mention_only。",
		Tool:      deapAgentCreateTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "name", Usage: "数字员工名称，同组织内唯一（最多 30 个 Unicode 码点）", Bind: "name", Required: true, Trim: true},
			{Name: "description", Usage: "数字员工职责描述（最多 300 个 Unicode 码点）", Bind: "description", Required: true, Trim: true},
			{Name: "dept-id", Usage: "归属部门 ID；不传时服务端使用操作人主任职部门", Bind: "deptId", Trim: true, OmitEmpty: true},
			{Name: "avatar-url", Usage: "公网 HTTP(S) 头像地址，或本地 jpg/jpeg/png/gif/webp 图片路径（最大 10 MiB）；本地文件由 CLI 自动上传", Bind: "avatarUrl", Trim: true, OmitEmpty: true},
			{Name: "supervisor-user-id", Usage: "直属上级 userId", Bind: "digitalTagEmployeeProfile.supervisorUserId", Trim: true, OmitEmpty: true},
			{Name: "type", Usage: "必填主程序类型：open_code 或 local_agent；必须显式填写且不能为空；接入本地 Agent/DSH 时传 local_agent", Bind: "digitalTagEmployeeProfile.type", Required: true, Trim: true, Enum: deapAgentMainProgramTypeValues},
			{Name: "response-mode", Usage: "响应模式：mention_only、targeted_proactive，或英文逗号分隔的组合 mention_only,targeted_proactive；未提供或空值时默认 mention_only", Bind: "digitalTagEmployeeProfile.responseMode", Default: deapAgentResponseModeMentionOnly, ArgDefault: deapAgentResponseModeMentionOnly, Trim: true, Transform: deapAgentResponseMode},
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
			if err := deapAgentValidateAvatarURLFlag(cmd); err != nil {
				return err
			}
			return nil
		},
		Call: deapAgentCallCreateWithAvatar,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "create_digital_employee",
				CanonicalPath: "dingtalk-tag.create_digital_employee",
				CLIPath:       "dingtalk-tag manage create", PrimaryCLIPath: "dingtalk-tag manage create",
				Group: "manage",
			},
			Description: "创建数字员工草稿并返回 agentUuid。必须显式填写 type（open_code 或 local_agent），不可为空；部门可由服务端按操作人主任职部门补齐；本地头像由 CLI 上传后回写；不自动发布。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentCreateTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "创建新的草稿态 DEAP 数字员工",
				UseWhen:      []string{"需要从零创建数字员工并获得 agentUuid 时"},
				AvoidWhen:    []string{"已有 agentUuid 只需修改草稿时使用 save-draft", "创建普通开放平台应用时使用 dev app create"},
				Examples:     []string{`dws dingtalk-tag manage create --name "值班助手" --description "处理值班问题" --type open_code --avatar-url https://example.com/avatar.png --response-mode mention_only --dry-run --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "supervisor-user-id", Property: "digitalTagEmployeeProfile.supervisorUserId", Description: "直属上级 userId；输入与详情、列表输出统一使用 supervisorUserId"},
				{Name: "type", Property: "digitalTagEmployeeProfile.type", Enum: deapAgentMainProgramTypeValues, Description: "必填且不能为空，对应 MCP 字段 digitalTagEmployeeProfile.type；显式选择 open_code 或 local_agent，接入本地 Agent/DSH 时传 local_agent"},
				{Name: "response-mode", Property: "digitalTagEmployeeProfile.responseMode", Enum: deapAgentResponseModeValues, Description: "响应模式；未提供或空值时默认 mention_only；支持 mention_only、targeted_proactive 或双值组合"},
			},
		},
	})
}

func newDeapAgentDetailCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "detail",
		Short:     "查询数字员工管理态详情",
		Long:      "按 agentUuid 查询数字员工详情。--snapshot draft 读取当前草稿，published 读取已发布配置，默认 draft；MCP 字段为 snapshot，兼容旧参数 --type。返回 status 是发布/生命周期状态，不代表本地 Agent 正在运行。人员标识统一为 userId。Skill/MCP 能力资源也通过 snapshot 选择草稿或已发布配置。详情的 avatarUrl 仅按当前接口结果使用；修改头像请重新传公网 URL 或本地文件路径。",
		Tool:      deapAgentDetailTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工 ID", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "snapshot", Aliases: []string{"type"}, Usage: "详情快照：draft（未发布草稿）或 published（已发布配置）；--type 为兼容别名", Bind: "snapshot", Default: "draft", ArgDefault: "draft", Trim: true, Enum: []string{"draft", "published"}},
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
			Description: "按 agentUuid 查询数字员工 draft 或 published 详情及其 Skill/MCP 引用配置。snapshot 默认 draft；返回 status 中 online 表示已发布，dev/offline 表示未发布。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentDetailTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "按 agentUuid 查询数字员工草稿或已发布详情",
				UseWhen:      []string{"需要读取数字员工完整配置、发布前检查或保存草稿前回读时"},
				AvoidWhen:    []string{"需要分页查找多个数字员工时使用 list", "只查一次运行状态时使用 run-status"},
				Examples: []string{
					"dws dingtalk-tag manage detail --agent-uuid <agentUuid> --snapshot draft --format json",
					"dws dingtalk-tag manage detail --agent-uuid <agentUuid> --snapshot published --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "snapshot", Property: "snapshot", Enum: []string{"draft", "published"}},
			},
		},
	})
}

func newDeapAgentListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "list",
		Short:     "分页查询数字员工",
		Long:      "分页查询当前身份有权查看的数字员工。管理员可看本组织全部，非管理员只返回本人参与的数字员工；支持按名称或职责等可见信息模糊搜索，不对外提供工号语义。可按 type 筛选，取值 open_code、local_agent；不传表示不过滤。page 和 page-size 必须大于等于 1。",
		Tool:      deapAgentListTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "keyword", Usage: "按名称或职责等可见信息模糊匹配", Bind: "keyword", Trim: true, OmitEmpty: true},
			{Name: "type", Usage: "按主程序类型筛选：open_code 或 local_agent；对应 MCP 字段 type，不传表示不过滤", Bind: "type", Trim: true, OmitEmpty: true, Enum: deapAgentMainProgramTypeValues},
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
			Description: "分页查询当前身份有权查看的数字员工；支持可见基础信息关键字和 open_code/local_agent 类型筛选，不对外提供工号字段。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentListTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "分页查找当前用户可管理或参与的数字员工",
				UseWhen:      []string{"需要按名称或职责等可见信息查找数字员工，或尚不知道 agentUuid 时"},
				AvoidWhen:    []string{"已知 agentUuid 需要完整配置时使用 detail", "需要查询运行记录时使用 run-status"},
				Examples:     []string{`dws dingtalk-tag manage list --keyword "值班" --type local_agent --page 1 --page-size 20 --format json`},
			},
		},
	})
}

func newDeapAgentSaveDraftCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "save-draft",
		Short:     "更新数字员工草稿",
		Long:      "更新指定数字员工的基础信息草稿但不发布，只更新显式传入的字段，未传字段保持不变。Skill/MCP 的创建、内容更新、启停和删除统一使用 capability skill|mcp create|update|delete，CLI 会维护草稿挂载；普通用户无需在 save-draft 中手工拼完整 skills/mcps 数组。dept-id 不传时保持原部门；avatar-url 可传公网 HTTP(S) 地址或本地图片，本地图片由 CLI 自动上传。主管只接受 userId。成功返回与 detail 一致的完整草稿结构。请先 --dry-run 检查参数，再加 --yes。",
		Tool:      deapAgentSaveDraftTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工 ID", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "name", Usage: "数字员工名称（最多 30 个 Unicode 码点）", Bind: "name", Trim: true, OmitEmpty: true},
			{Name: "description", Usage: "数字员工职责描述（最多 300 个 Unicode 码点）；不传保持原值", Bind: "description", Trim: true, OmitEmpty: true},
			{Name: "avatar-url", Usage: "公网 HTTP(S) 头像地址，或本地 jpg/jpeg/png/gif/webp 图片路径（最大 10 MiB）；本地文件由 CLI 自动上传，不传保持原值", Bind: "avatarUrl", Trim: true, OmitEmpty: true},
			{Name: "dept-id", Usage: "归属部门 ID；不传保持原部门，不支持清空", Bind: "deptId", Trim: true, OmitEmpty: true},
			{Name: "prompt", Usage: "人设/System Prompt（最多 5000 个 Unicode 码点）", Bind: "prompt", Trim: true, OmitEmpty: true},
			{Name: "supervisor-user-id", Usage: "直属上级 userId", Bind: "digitalTagEmployeeProfile.supervisorUserId", Trim: true, OmitEmpty: true},
			{Name: "type", Usage: "可选主程序类型：open_code 或 local_agent；未修改时省略（保持草稿原值），也可显式传 open_code；保持或切换为本地 Agent/DSH 模式时传 local_agent", Bind: "digitalTagEmployeeProfile.type", Trim: true, OmitEmpty: true, Enum: deapAgentMainProgramTypeValues},
			{Name: "response-mode", Usage: "响应模式：mention_only、targeted_proactive，或英文逗号分隔的组合 mention_only,targeted_proactive；未传时保持草稿原值", Bind: "digitalTagEmployeeProfile.responseMode", Trim: true, OmitEmpty: true, Transform: deapAgentResponseMode},
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
			if err := deapAgentValidateAvatarURLFlag(cmd); err != nil {
				return err
			}
			return nil
		},
		Call: deapAgentCallSaveWithAvatar,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "update_digital_employee_draft",
				CanonicalPath: "dingtalk-tag.update_digital_employee_draft",
				CLIPath:       "dingtalk-tag manage save-draft", PrimaryCLIPath: "dingtalk-tag manage save-draft",
				Group: "manage",
			},
			Description: "按显式参数更新数字员工基础信息草稿，未传字段保持不变；Skill/MCP 资源及草稿挂载由 capability skill|mcp 生命周期命令管理；成功返回 detail 同构的完整草稿。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentSaveDraftTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "更新数字员工基础信息草稿并回读完整详情",
				UseWhen:      []string{"需要更新名称、职责、头像、部门、人设或运行配置时"},
				AvoidWhen:    []string{"需要管理 Skill/MCP 资源或挂载时使用 capability skill|mcp", "准备直接上线时仍需另行执行 publish"},
				Examples:     []string{`dws dingtalk-tag manage save-draft --agent-uuid <agentUuid> --name "值班助手" --description "处理值班问题" --dry-run --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "supervisor-user-id", Property: "digitalTagEmployeeProfile.supervisorUserId", Description: "直属上级 userId；输入与详情、列表输出统一使用 supervisorUserId"},
				{Name: "type", Property: "digitalTagEmployeeProfile.type", Enum: deapAgentMainProgramTypeValues, Description: "MCP 字段为 digitalTagEmployeeProfile.type；未修改时省略（保持草稿原值），也允许显式传 open_code；保持或切换为本地 Agent/DSH 模式时传 local_agent"},
				{Name: "response-mode", Property: "digitalTagEmployeeProfile.responseMode", Enum: deapAgentResponseModeValues, Description: "响应模式；未传时保持草稿原值；传值时支持 mention_only、targeted_proactive 或双值组合"},
			},
		},
	})
}

func newDeapAgentSetVisibilityCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "set-visibility",
		Short:     "设置数字员工可见范围",
		Long:      "设置指定数字员工草稿的可见范围，全量替换草稿中现有范围，不修改其他草稿字段。visibility=ALL 表示本企业全员可见，此时无需传成员或部门；visibility=PARTIAL 表示仅指定成员或部门可见，用 --user-ids 传成员 userId、--dept-ids 传部门 ID（两类列表至少一项非空，可混选）。user-ids 与 dept-ids 均为全量替换：本次未提供则清空对应维度。人员标识统一使用 userId。这是高影响写操作，先 --dry-run 检查参数，再加 --yes。",
		Tool:      deapAgentSetVisibilityTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工 ID", Bind: "agentUuid", Required: true, Trim: true},
			{Name: "visibility", Usage: "可见范围：ALL 表示本企业全员可见；PARTIAL 表示仅指定成员/部门可见（配合 --user-ids/--dept-ids）", Bind: "visibility", Required: true, Trim: true},
			{Name: "user-ids", Usage: "指定可见成员 userId，可重复或用英文逗号分隔；全量替换，未提供则清空成员维度", Bind: "staffIds", Kind: LeafStringSlice},
			{Name: "dept-ids", Usage: "指定可见部门 ID，可重复或用英文逗号分隔；全量替换，未提供则清空部门维度", Bind: "deptIds", Kind: LeafStringSlice},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: dingtalkTagProductID, Name: "set_visibility",
				CanonicalPath: "dingtalk-tag.set_visibility",
				CLIPath:       "dingtalk-tag manage set-visibility", PrimaryCLIPath: "dingtalk-tag manage set-visibility",
				Group: "manage",
			},
			Description: "设置数字员工草稿的可见范围，全量替换现有范围。visibility=ALL 表示本企业全员可见；visibility=PARTIAL 表示仅指定成员/部门可见，通过 staffIds、deptIds 提供，均为全量替换。",
			DryRun:      deapAgentDryRun,
			Interface:   deapAgentMCPInterface(deapAgentSetVisibilityTool),
			Selection: contract.SelectionSpec{
				AgentSummary: "设置数字员工草稿的可见范围，全量替换现有范围",
				UseWhen:      []string{"需要设置或调整数字员工草稿的可见范围，如切换为全员可见或指定成员、部门可见时"},
				AvoidWhen:    []string{"只更新名称、职责或 Skill/MCP 等草稿字段时使用 save-draft", "未确认目标 agentUuid 与可见范围影响时不要执行"},
				Examples: []string{
					"dws dingtalk-tag manage set-visibility --agent-uuid <agentUuid> --visibility ALL --dry-run --format json",
					`dws dingtalk-tag manage set-visibility --agent-uuid <agentUuid> --visibility PARTIAL --user-ids user-1,user-2 --dept-ids 100,200 --dry-run --format json`,
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "visibility", Property: "visibility", Description: "可见范围：ALL 表示本企业全员可见；PARTIAL 表示仅指定成员/部门可见"},
				{Name: "user-ids", Property: "staffIds", InterfaceType: "array"},
				{Name: "dept-ids", Property: "deptIds", InterfaceType: "array"},
			},
		},
	})
}

func newDeapAgentPublishCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:       "publish",
		Short:     "发布数字员工",
		Long:      "发布指定数字员工当前已保存的完整草稿，本命令不携带或修改草稿内容。发布前应已配置 responseMode；create 默认 mention_only，历史草稿缺失时先通过 save-draft 补齐，其他必填配置由服务端校验。这是高影响操作，真实执行前必须确认。",
		Tool:      deapAgentPublishTool,
		Server:    deapAgentServerID,
		PostMount: deapAgentNoArgs,
		Flags: []LeafFlag{
			{Name: "agent-uuid", Usage: "数字员工 ID", Bind: "agentUuid", Required: true, Trim: true},
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
			Description: "发布当前完整草稿，由服务端校验发布所需配置；本命令不补写草稿。",
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
		Long:      "查询指定数字员工的执行 Trace。--agent-uuid、--source-id、--source-type 均必填（与 run-status 一致）。返回内容可能包含完整对话和模型输入输出；服务端会先执行管理者/触发人两级授权，无权时返回 NO_PERMISSION。输出为 null（无 data）不是报错，而是该来源暂无可用 trace：可能 trace 尚未就绪（异步写入有延迟）、来源 ID/类型不匹配，或该来源无对应执行；可稍后重试，或先用 run-status 确认该来源是否存在执行记录。",
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
	deapAgentPrepareProfile(args)
	return callMCPToolOnServer(deapAgentServerID, tool, args)
}

func deapAgentPrepareProfile(args map[string]any) {
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
		{"digitalTagEmployeeProfile.type", "type"},
		{"digitalTagEmployeeProfile.supervisorUserId", "supervisorUserId"},
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

func deapAgentInvalidMainProgramType() error {
	return apperrors.NewValidation("主程序类型只允许 open_code 或 local_agent；a2a 暂不支持")
}
