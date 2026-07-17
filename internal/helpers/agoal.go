package helpers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var agoalLoadLocation = time.LoadLocation

// ──────────────────────────────────────────────────────────
// dws agoal — Agoal 管理
// ──────────────────────────────────────────────────────────

func newAgoalCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "agoal",
		Short: "Agoal 管理",
		Long: `管理钉钉 Agoal：战略解码、经营合约、计分卡、用户目标、周月报。

命令结构:
  dws agoal strategy list                 获取战略解码列表
  dws agoal strategy detail               获取战略解码详情
  dws agoal strategy update               更新战略解码
  dws agoal contract list                 获取经营合约列表
  dws agoal contract fields               获取经营合约字段列表
  dws agoal contract detail               获取经营合约详情
  dws agoal contract update               更新经营合约
  dws agoal scorecard detail              获取计分卡详情
  dws agoal scorecard entity-detail       获取计分卡实体详情
  dws agoal scorecard update              更新计分卡
  dws agoal user rules                    获取用户规则
  dws agoal user objectives               查询用户目标列表
  dws agoal report list-statistics        获取周月报数据跟催列表
  dws agoal report submit-detail          获取周月报规则提交详情
  dws agoal obj-template list             获取目标模板列表
  dws agoal obj-template create-or-update 新增或更新目标模板`,
		RunE: groupRunE,
	}

	// ── strategy: 战略解码管理 ──────────────────────────────────

	strategyCmd := &cobra.Command{Use: "strategy", Short: "战略解码管理", RunE: groupRunE}

	strategyListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取战略解码列表",
		Long: `按部门或个人维度查询战略解码列表。
scopeType 支持:
  DEPT     — 按部门查询
  PERSONAL — 按个人查询

--scope-id 为对应维度的钉钉部门 id 或用户 id。`,
		Example: `  dws agoal strategy list --scope-type PERSONAL --scope-id USER_ID
  dws agoal strategy list --scope-type DEPT --scope-id DEPT_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "scope-type", "scope-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"scopeType": mustGetFlag(cmd, "scope-type"),
				"openId":    mustGetFlag(cmd, "scope-id"),
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			return callMCPTool("list_strategy_decodings", toolArgs)
		},
	}
	strategyListCmd.Flags().String("scope-type", "", "解码范围类型: DEPT/PERSONAL (必填)")
	strategyListCmd.Flags().String("scope-id", "", "scope-type 对应的钉钉部门 id 或用户 id (必填)")
	strategyListCmd.Flags().String("request-id", "", "requestId (可选)")

	strategyDetailCmd := &cobra.Command{
		Use:     "detail",
		Short:   "获取战略解码详情",
		Long:    `根据战略解码 id (profileId) 获取战略解码的详细信息。`,
		Example: `  dws agoal strategy detail --profile-id PROFILE_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "profile-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"profileId": mustGetFlag(cmd, "profile-id"),
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			return callMCPTool("get_strategy_decoding_detail", toolArgs)
		},
	}
	strategyDetailCmd.Flags().String("profile-id", "", "战略解码 id (必填)")
	strategyDetailCmd.Flags().String("request-id", "", "requestId (可选)")

	strategyUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新战略解码",
		Long: `谨慎操作，基于查询接口返回的老数据进行修改，本接口是覆盖逻辑，会根据战略解码id进行对应的修改。
--content 为 JSON 数组，每个实体对象包含:
  id           — 实体 id
  title        — 标题对象 {"title":"标题文本"}
  linkEntityId — 所属的实体 ID（查询接口中此字段有值一定要传进来）
  entityType   — 类型: OGSM_OBJECTIVE/目的、OGSM_GOAL/目标、OGSM_STRATEGY/策略、OGSM_MEASUREMENT/衡量标准、OGSM_TACTICS/行动方案
  status       — 状态: NORMAL/正常、PRE_PUBLISH_THEN_UPDATE/预发布更新、PRE_PUBLISH_THEN_CREATE/预发布新增、PRE_PUBLISH_THEN_DELETE/预发布删除
  supporters   — 承接人数组 [{type, dingId, staffId}]，type: USER/个人、DEPARTMENT/部门，staffId 类型为 USER 时必填
  indicators   — 关键指标 id 字符串数组，如 ["id1","id2"]
  linkSources  — 资源关联数组 [{id, linkType, linkId, source, objectId, keyResultId}]
                 linkType: project/项目、task/任务、campaign/战役空间、product/产品空间、doc/文档、standard/其它
                 source: teambition
  executors    — 人员 dingId 字符串数组，如 ["dingId1","dingId2"]
  teams        — 部门 dingId 字符串数组，如 ["dingId1","dingId2"]`,
		Example: `  dws agoal strategy update --profile-id PROFILE_ID --content '[{"id":"entity1","title":{"title":"新目标"},"entityType":"OGSM_OBJECTIVE","status":"NORMAL","executors":["dingId1"],"teams":["deptDingId1"]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "profile-id", "content"); err != nil {
				return err
			}
			var contentArr []any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "content")), &contentArr); err != nil {
				return fmt.Errorf("--content must be a valid JSON array: %w", err)
			}
			toolArgs := map[string]any{
				"profileId": mustGetFlag(cmd, "profile-id"),
				"content":   contentArr,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			return callMCPTool("update_strategy_decoding", toolArgs)
		},
	}
	strategyUpdateCmd.Flags().String("profile-id", "", "战略解码 id (必填)")
	strategyUpdateCmd.Flags().String("content", "", "实体列表 JSON 数组 (必填)")
	strategyUpdateCmd.Flags().String("request-id", "", "requestId (可选)")

	strategyCmd.AddCommand(strategyListCmd, strategyDetailCmd, strategyUpdateCmd)

	// ── contract: 经营合约管理 ──────────────────────────────────

	contractCmd := &cobra.Command{Use: "contract", Short: "经营合约管理", RunE: groupRunE}

	contractListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取经营合约列表",
		Long: `按部门或个人维度查询经营合约列表。
scopeType 支持:
  DEPT     — 按部门查询
  PERSONAL — 按个人查询

--scope-id 为通讯录里的部门 id 或用户 id。`,
		Example: `  dws agoal contract list --scope-type PERSONAL --scope-id USER_ID
  dws agoal contract list --scope-type DEPT --scope-id DEPT_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "scope-type", "scope-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"scopeType": mustGetFlag(cmd, "scope-type"),
				"openId":    mustGetFlag(cmd, "scope-id"),
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			return callMCPTool("list_op_contracts", toolArgs)
		},
	}
	contractListCmd.Flags().String("scope-type", "", "合约范围类型: DEPT/PERSONAL (必填)")
	contractListCmd.Flags().String("scope-id", "", "scope-type 对应的钉钉部门 id 或用户 id (必填)")
	contractListCmd.Flags().String("request-id", "", "requestId (可选)")

	contractFieldsCmd := &cobra.Command{
		Use:     "fields",
		Short:   "获取经营合约字段列表",
		Long:    `获取指定组织下经营合约的字段配置信息。`,
		Example: `  dws agoal contract fields`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			return callMCPTool("list_op_contract_fields", toolArgs)
		},
	}
	contractFieldsCmd.Flags().String("request-id", "", "requestId (可选)")

	contractDetailCmd := &cobra.Command{
		Use:     "detail",
		Short:   "获取经营合约详情",
		Long:    `根据经营合约 id 获取经营合约的详细信息。`,
		Example: `  dws agoal contract detail --contract-id CONTRACT_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "contract-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"contractId": mustGetFlag(cmd, "contract-id"),
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			return callMCPTool("get_op_contract_detail", toolArgs)
		},
	}
	contractDetailCmd.Flags().String("contract-id", "", "经营合约 id (必填)")
	contractDetailCmd.Flags().String("request-id", "", "requestId (可选)")

	contractUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新经营合约",
		Long: `谨慎操作，一定要基于查询接口返回的老数据进行修改，然后给本接口使用，因为本接口是覆盖逻辑，会使用传入的数据去覆盖已存在的数据。
--dimensions 为 JSON 数组，每个维度对象包含:
  id              — 维度 id
  title           — 维度名称
  description     — 维度描述
  weight          — 维度权重
  objectives      — 目标列表
  dimensionConfig — 维度配置
  children        — 子维度列表

可选参数:
  --audit-config       — 审批配置 JSON，如 {"needAudit":true,"processTemplateId":"TPL_ID"}
  --objective-template — 合约模板 JSON，如 {"id":"TPL_ID","title":"模板名称"}`,
		Example: `  dws agoal contract update --contract-id CONTRACT_ID \
    --dimensions '[{"id":"dim1","title":"业绩","weight":60,"objectives":[{"id":"obj1"}]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "contract-id", "dimensions"); err != nil {
				return err
			}
			var dimensionsArr []any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "dimensions")), &dimensionsArr); err != nil {
				return fmt.Errorf("--dimensions must be a valid JSON array: %w", err)
			}
			toolArgs := map[string]any{
				"contractId": mustGetFlag(cmd, "contract-id"),
				"dimensions": dimensionsArr,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			if v, _ := cmd.Flags().GetString("audit-config"); v != "" {
				toolArgs["auditConfig"] = v
			}
			if v, _ := cmd.Flags().GetString("objective-template"); v != "" {
				toolArgs["objectiveTemplate"] = v
			}
			return callMCPTool("update_op_contract", toolArgs)
		},
	}
	contractUpdateCmd.Flags().String("contract-id", "", "经营合约 id (必填)")
	contractUpdateCmd.Flags().String("request-id", "", "requestId (可选)")
	contractUpdateCmd.Flags().String("audit-config", "", "审批配置 JSON (可选)")
	contractUpdateCmd.Flags().String("objective-template", "", "合约模板 JSON (可选)")
	contractUpdateCmd.Flags().String("dimensions", "", "维度内容列表 JSON 数组 (必填)")

	contractCmd.AddCommand(contractListCmd, contractFieldsCmd, contractDetailCmd, contractUpdateCmd)

	// ── scorecard: 计分卡管理 ───────────────────────────────────

	scorecardCmd := &cobra.Command{Use: "scorecard", Short: "计分卡管理", RunE: groupRunE}

	scorecardDetailCmd := &cobra.Command{
		Use:   "detail",
		Short: "获取计分卡详情",
		Long: `根据部门 id 和时间获取计分卡详情。
--selected-time 接受 ISO-8601 字符串，传入对应周期起始时刻:
  2026年    → "2026-01-01T00:00:00+08:00"
  2026年5月 → "2026-05-01T00:00:00+08:00"`,
		Example: `  dws agoal scorecard detail --selected-time "2026-01-01T00:00:00+08:00" --dept-id DEPT_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "selected-time", "dept-id"); err != nil {
				return err
			}
			selectedTimeMs, err := parseISO8601ToMillis(mustGetFlag(cmd, "selected-time"))
			if err != nil {
				return fmt.Errorf("--selected-time: %w", err)
			}
			toolArgs := map[string]any{
				"selectedTime": selectedTimeMs,
				"deptId":       mustGetFlag(cmd, "dept-id"),
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			return callMCPTool("get_score_card_detail", toolArgs)
		},
	}
	scorecardDetailCmd.Flags().String("selected-time", "", "ISO-8601 时间字符串，如 \"2026-01-01T00:00:00+08:00\" (必填)")
	scorecardDetailCmd.Flags().String("dept-id", "", "部门 id (必填)")
	scorecardDetailCmd.Flags().String("request-id", "", "requestId (可选)")

	scorecardEntityDetailCmd := &cobra.Command{
		Use:     "entity-detail",
		Short:   "获取计分卡实体详情",
		Long:    `根据计分卡 id 和实体 id 获取计分卡实体的详细信息。`,
		Example: `  dws agoal scorecard entity-detail --sc-id SC_ID --entity-id ENTITY_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "sc-id", "entity-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"scId":     mustGetFlag(cmd, "sc-id"),
				"entityId": mustGetFlag(cmd, "entity-id"),
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			return callMCPTool("get_score_card_entity_detail", toolArgs)
		},
	}
	scorecardEntityDetailCmd.Flags().String("sc-id", "", "计分卡 id (必填)")
	scorecardEntityDetailCmd.Flags().String("entity-id", "", "计分卡实体 id (必填)")
	scorecardEntityDetailCmd.Flags().String("request-id", "", "requestId (可选)")

	scorecardUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新计分卡",
		Long: `更新指定计分卡的内容。
--content 为 JSON 数组，每个维度对象包含:
  id    — 维度 id
  title — 维度名称
  items — 指标或关键事项列表，每项包含:
    id        — 实体 id
    title     — 名称
    reference — 参考信息
    start     — 起始值
    target    — 目标值
    executors — 负责人列表，每项包含 openId`,
		Example: `  dws agoal scorecard update --dept-id DEPT_ID --selected-time "2025-01-01T00:00:00+08:00" --id SC_ID --tracking-period-type MONTHLY --content '[{"id":"dim1","title":"业绩","items":[{"id":"item1","title":"收入","target":"100"}]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dept-id", "selected-time", "id", "tracking-period-type", "content"); err != nil {
				return err
			}
			selectedTimeMs, err := parseISO8601ToMillis(mustGetFlag(cmd, "selected-time"))
			if err != nil {
				return fmt.Errorf("--selected-time: %w", err)
			}
			var contentArr []any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "content")), &contentArr); err != nil {
				return fmt.Errorf("--content must be a valid JSON array: %w", err)
			}
			toolArgs := map[string]any{
				"deptId":             mustGetFlag(cmd, "dept-id"),
				"selectedTime":       selectedTimeMs,
				"id":                 mustGetFlag(cmd, "id"),
				"trackingPeriodType": mustGetFlag(cmd, "tracking-period-type"),
				"content":            contentArr,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			return callMCPTool("update_score_card", toolArgs)
		},
	}
	scorecardUpdateCmd.Flags().String("dept-id", "", "部门 id (必填)")
	scorecardUpdateCmd.Flags().String("selected-time", "", "ISO-8601 时间字符串，如 \"2026-01-01T00:00:00+08:00\" (必填)")
	scorecardUpdateCmd.Flags().String("id", "", "计分卡 id (必填)")
	scorecardUpdateCmd.Flags().String("tracking-period-type", "", "跟踪周期类型: MONTHLY/月度追踪、QUARTERLY/季度追踪 (必填)")
	scorecardUpdateCmd.Flags().String("content", "", "内容 JSON 数组 (必填)")
	scorecardUpdateCmd.Flags().String("request-id", "", "requestId (可选)")

	scorecardCmd.AddCommand(scorecardDetailCmd, scorecardEntityDetailCmd, scorecardUpdateCmd)

	// ── user: 用户目标管理 ──────────────────────────────────────

	userCmd := &cobra.Command{Use: "user", Short: "用户目标管理", RunE: groupRunE}

	userRulesCmd := &cobra.Command{
		Use:   "rules",
		Short: "获取用户的规则周期列表",
		Long:  `获取用户的规则周期列表。不传 dingUserId 时默认取操作人自己的规则。`,
		Example: `  dws agoal user rules
  dws agoal user rules --user-id USER_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetString("user-id"); v != "" {
				toolArgs["dingUserId"] = v
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			return callMCPTool("get_user_rules", toolArgs)
		},
	}
	userRulesCmd.Flags().String("user-id", "", "要查询的人员钉钉 id (可选，默认取操作人)")
	userRulesCmd.Flags().String("request-id", "", "requestId (可选)")

	userObjectivesCmd := &cobra.Command{
		Use:     "objectives",
		Short:   "查询用户目标列表",
		Long:    `根据用户、规则 id 和周期列表查询用户的目标。`,
		Example: `  dws agoal user objectives --user-id USER_ID --rule-id RULE_ID --period-ids "period1,period2"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "user-id", "rule-id", "period-ids"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"dingUserId":      mustGetFlag(cmd, "user-id"),
				"objectiveRuleId": mustGetFlag(cmd, "rule-id"),
				"periodIds":       parseCSVValues(mustGetFlag(cmd, "period-ids")),
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			return callMCPTool("list_user_objectives", toolArgs)
		},
	}
	userObjectivesCmd.Flags().String("user-id", "", "要查询的人员钉钉 id (必填)")
	userObjectivesCmd.Flags().String("rule-id", "", "规则 id (必填)")
	userObjectivesCmd.Flags().String("period-ids", "", "周期 id 列表，逗号分隔 (必填)")
	userObjectivesCmd.Flags().String("request-id", "", "requestId (可选)")

	userCmd.AddCommand(userRulesCmd, userObjectivesCmd)

	// ── report: 周月报管理 ──────────────────────────────────────

	reportCmd := &cobra.Command{Use: "report", Short: "周月报管理", RunE: groupRunE}

	reportListStatisticsCmd := &cobra.Command{
		Use:   "list-statistics",
		Short: "获取周月报数据跟催列表",
		Long:  `获取周月报规则的人员提交情况列表，包含按时提交、迟交、未提交的人员数量统计。`,
		Example: `  dws agoal report list-statistics
  dws agoal report list-statistics --keyword "周报规则"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			return callMCPTool("list_report_statistics", toolArgs)
		},
	}
	reportListStatisticsCmd.Flags().String("request-id", "", "requestId (可选)")
	reportListStatisticsCmd.Flags().String("keyword", "", "搜索规则名称 (可选)")

	reportSubmitDetailCmd := &cobra.Command{
		Use:   "submit-detail",
		Short: "获取周月报规则提交详情",
		Long: `获取周月报规则提交详情，包含按时提交、迟交、未提交中的具体人员以及提交的时间、周报id等。
--submit-state 支持:
  ON_TIME       — 按时提交
  LATE          — 迟交
  NOT_SUBMITTED — 未提交
--query-date 接受 ISO-8601 字符串，如 "2026-06-18T00:00:00+08:00"`,
		Example: `  dws agoal report submit-detail --template-id TPL_ID --submit-state ON_TIME
  dws agoal report submit-detail --template-id TPL_ID --submit-state LATE --query-date "2026-06-18T00:00:00+08:00" --page 1 --page-size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "template-id", "submit-state"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"templateId":  mustGetFlag(cmd, "template-id"),
				"submitState": mustGetFlag(cmd, "submit-state"),
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			if v, _ := cmd.Flags().GetString("query-date"); v != "" {
				queryDateMs, err := parseISO8601ToMillis(v)
				if err != nil {
					return fmt.Errorf("--query-date: %w", err)
				}
				toolArgs["queryDate"] = time.UnixMilli(queryDateMs).In(shanghaiLocation()).Format("2006-01-02")
			}
			if v, _ := cmd.Flags().GetInt("page"); v != 0 {
				toolArgs["page"] = v
			}
			if v, _ := cmd.Flags().GetInt("page-size"); v != 0 {
				toolArgs["pageSize"] = v
			}
			if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			return callMCPTool("get_submit_detail", toolArgs)
		},
	}
	reportSubmitDetailCmd.Flags().String("template-id", "", "规则模板 id (必填)")
	reportSubmitDetailCmd.Flags().String("submit-state", "", "提交状态: ON_TIME(按时提交)/LATE(迟交)/NOT_SUBMITTED(未提交) (必填)")
	reportSubmitDetailCmd.Flags().String("request-id", "", "requestId (可选)")
	reportSubmitDetailCmd.Flags().String("query-date", "", "查询日期，ISO-8601 格式（如 \"2026-06-18T00:00:00+08:00\"），默认为当天 (可选)")
	reportSubmitDetailCmd.Flags().Int("page", 0, "分页参数，默认为 1 (可选)")
	reportSubmitDetailCmd.Flags().Int("page-size", 0, "分页参数，默认为 10 (可选)")
	reportSubmitDetailCmd.Flags().String("keyword", "", "搜索员工名称 (可选)")

	reportCmd.AddCommand(reportListStatisticsCmd, reportSubmitDetailCmd)

	// ── template: 目标模板管理 ──────────────────────────────────

	objTemplateCmd := &cobra.Command{Use: "obj-template", Short: "目标模板管理", RunE: groupRunE}

	objTemplateListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取目标模板列表",
		Long:  `获取目标模板列表，支持关键词搜索和分页。`,
		Example: `  dws agoal obj-template list
  dws agoal obj-template list --keyword "业绩" --page 1 --page-size 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			if v, _ := cmd.Flags().GetInt("page"); v != 0 {
				toolArgs["page"] = v
			}
			if v, _ := cmd.Flags().GetInt("page-size"); v != 0 {
				toolArgs["pageSize"] = v
			}
			if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			return callMCPTool("list_obj_template", toolArgs)
		},
	}
	objTemplateListCmd.Flags().String("request-id", "", "requestId (可选)")
	objTemplateListCmd.Flags().Int("page", 0, "页码，默认 1 (可选)")
	objTemplateListCmd.Flags().Int("page-size", 0, "每页数量，默认 10 (可选)")
	objTemplateListCmd.Flags().String("keyword", "", "搜索关键词 (可选)")

	objTemplateCreateOrUpdateCmd := &cobra.Command{
		Use:   "create-or-update",
		Short: "新增或更新目标模板",
		Long: `新增或更新目标模板。覆盖逻辑，更新时一定要基于查询接口返回的老数据进行修改，新增时建议先参考已存在的模板数据。
--dimensions 为必填参数，JSON 字符串，包含目标维度、维度配置、目标内容。
--objective-weight 启用目标权重
--dimension-weight 启用维度权重
--compute-by-weight 维度是否参与计算`,
		Example: `  dws agoal obj-template create-or-update --title "业绩模板" --dimensions '[{"title":"维度1","weight":100}]'
  dws agoal obj-template create-or-update --template-id TPL_ID --dimensions '[...基于老数据修改...]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dimensions"); err != nil {
				return err
			}
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				toolArgs["requestId"] = v
			}
			if v, _ := cmd.Flags().GetString("template-id"); v != "" {
				toolArgs["templateId"] = v
			}
			if v, _ := cmd.Flags().GetString("title"); v != "" {
				toolArgs["title"] = v
			}
			if cmd.Flags().Changed("objective-weight") {
				v, _ := cmd.Flags().GetBool("objective-weight")
				toolArgs["objectiveWeight"] = v
			}
			if cmd.Flags().Changed("dimension-weight") {
				v, _ := cmd.Flags().GetBool("dimension-weight")
				toolArgs["dimensionWeight"] = v
			}
			if cmd.Flags().Changed("compute-by-weight") {
				v, _ := cmd.Flags().GetBool("compute-by-weight")
				toolArgs["computeByWeight"] = v
			}
			toolArgs["dimensions"] = mustGetFlag(cmd, "dimensions")
			return callMCPTool("create_or_update_obj_template", toolArgs)
		},
	}
	objTemplateCreateOrUpdateCmd.Flags().String("request-id", "", "requestId (可选)")
	objTemplateCreateOrUpdateCmd.Flags().String("template-id", "", "模板 id (更新时必填)")
	objTemplateCreateOrUpdateCmd.Flags().String("title", "", "模板标题 (新增时必填)")
	objTemplateCreateOrUpdateCmd.Flags().Bool("objective-weight", false, "是否启用目标权重")
	objTemplateCreateOrUpdateCmd.Flags().Bool("dimension-weight", false, "是否启用维度权重")
	objTemplateCreateOrUpdateCmd.Flags().Bool("compute-by-weight", false, "维度是否参与计算")
	objTemplateCreateOrUpdateCmd.Flags().String("dimensions", "", "模板关联的维度 JSON 字符串 (必填，更新时基于老数据修改，新增时建议参考已有模板)")

	objTemplateCmd.AddCommand(objTemplateListCmd, objTemplateCreateOrUpdateCmd)

	root.AddCommand(strategyCmd, contractCmd, scorecardCmd, userCmd, reportCmd, objTemplateCmd)

	return root
}

// parseISO8601ToMillis 将 ISO-8601 时间字符串解析为毫秒时间戳。
// 支持格式：RFC3339（含时区）、无时区（默认 Asia/Shanghai）、仅日期。
func parseISO8601ToMillis(value string) (int64, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UnixMilli(), nil
	}

	shanghaiLoc := shanghaiLocation()

	if t, err := time.ParseInLocation("2006-01-02T15:04:05", value, shanghaiLoc); err == nil {
		return t.UnixMilli(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", value, shanghaiLoc); err == nil {
		return t.UnixMilli(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", value, shanghaiLoc); err == nil {
		return t.UnixMilli(), nil
	}

	return 0, fmt.Errorf("invalid ISO-8601 time format %q, expected e.g. \"2026-01-01T00:00:00+08:00\"", value)
}

func shanghaiLocation() *time.Location {
	loc, err := agoalLoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}
