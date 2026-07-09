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

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return agoalHandler{}
	})
}

type agoalHandler struct{}

func (agoalHandler) Name() string {
	return "agoal"
}

func (agoalHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "agoal",
		Short:             i18n.T("Agoal 管理"),
		Long:              i18n.T("管理钉钉 Agoal：战略解码、经营合约、计分卡、用户目标、目标模板、周月报。"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(
		newAgoalStrategyCommand(runner),
		newAgoalContractCommand(runner),
		newAgoalScorecardCommand(runner),
		newAgoalUserCommand(runner),
		newAgoalObjTemplateCommand(runner),
		newAgoalReportCommand(runner),
	)
	return root
}

// ── strategy: 战略解码管理 ──────────────────────────────────

func newAgoalStrategyCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "strategy",
		Short: i18n.T("战略解码管理"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newAgoalStrategyListCommand(runner),
		newAgoalStrategyDetailCommand(runner),
		newAgoalStrategyUpdateCommand(runner),
	)
	return cmd
}

func newAgoalStrategyListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: i18n.T("获取战略解码列表"),
		Long: i18n.T(`按部门或个人维度查询战略解码列表。
scopeType 支持: DEPT(按部门)、PERSONAL(按个人)。
--scope-id 为对应维度的钉钉部门 id 或用户 id。`),
		Example: `  dws agoal strategy list --scope-type PERSONAL --scope-id USER_ID --format json
  dws agoal strategy list --scope-type DEPT --scope-id DEPT_ID --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			scopeType, _ := cmd.Flags().GetString("scope-type")
			scopeID, _ := cmd.Flags().GetString("scope-id")
			if strings.TrimSpace(scopeType) == "" || strings.TrimSpace(scopeID) == "" {
				return apperrors.NewValidation("--scope-type and --scope-id are required")
			}
			params := map[string]any{
				"scopeType": scopeType,
				"openId":    scopeID,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "list_strategy_decodings", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("scope-type", "", i18n.T("解码范围类型: DEPT/PERSONAL (必填)"))
	cmd.Flags().String("scope-id", "", i18n.T("scope-type 对应的钉钉部门 id 或用户 id (必填)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

func newAgoalStrategyDetailCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "detail",
		Short:   i18n.T("获取战略解码详情"),
		Long:    i18n.T("根据战略解码 id (profileId) 获取战略解码的详细信息。"),
		Example: `  dws agoal strategy detail --profile-id PROFILE_ID --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			profileID, _ := cmd.Flags().GetString("profile-id")
			if strings.TrimSpace(profileID) == "" {
				return apperrors.NewValidation("--profile-id is required")
			}
			params := map[string]any{
				"profileId": profileID,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "get_strategy_decoding_detail", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("profile-id", "", i18n.T("战略解码 id (必填)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

func newAgoalStrategyUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: i18n.T("更新战略解码"),
		Long: i18n.T(`谨慎操作，基于查询接口返回的老数据进行修改，本接口是覆盖逻辑。
--content 为 JSON 数组，每个实体包含 id/title/linkEntityId/entityType/status/supporters/indicators/linkSources/executors/teams 等字段。`),
		Example: `  dws agoal strategy update --profile-id PROFILE_ID --content '[{"id":"entity1","title":{"title":"新目标"},"entityType":"OGSM_OBJECTIVE","status":"NORMAL","executors":["dingId1"]}]' --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			profileID, _ := cmd.Flags().GetString("profile-id")
			contentStr, _ := cmd.Flags().GetString("content")
			if strings.TrimSpace(profileID) == "" {
				return apperrors.NewValidation("--profile-id is required")
			}
			if strings.TrimSpace(contentStr) == "" {
				return apperrors.NewValidation("--content is required")
			}
			var contentArr []any
			if err := json.Unmarshal([]byte(contentStr), &contentArr); err != nil {
				return fmt.Errorf("--content must be a valid JSON array: %w", err)
			}
			params := map[string]any{
				"profileId": profileID,
				"content":   contentArr,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "update_strategy_decoding", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("profile-id", "", i18n.T("战略解码 id (必填)"))
	cmd.Flags().String("content", "", i18n.T("实体列表 JSON 数组 (必填)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

// ── contract: 经营合约管理 ──────────────────────────────────

func newAgoalContractCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contract",
		Short: i18n.T("经营合约管理"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newAgoalContractListCommand(runner),
		newAgoalContractFieldsCommand(runner),
		newAgoalContractDetailCommand(runner),
		newAgoalContractUpdateCommand(runner),
	)
	return cmd
}

func newAgoalContractListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: i18n.T("获取经营合约列表"),
		Long: i18n.T(`按部门或个人维度查询经营合约列表。
scopeType 支持: DEPT(按部门)、PERSONAL(按个人)。
--scope-id 为通讯录里的部门 id 或用户 id。`),
		Example: `  dws agoal contract list --scope-type PERSONAL --scope-id USER_ID --format json
  dws agoal contract list --scope-type DEPT --scope-id DEPT_ID --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			scopeType, _ := cmd.Flags().GetString("scope-type")
			scopeID, _ := cmd.Flags().GetString("scope-id")
			if strings.TrimSpace(scopeType) == "" || strings.TrimSpace(scopeID) == "" {
				return apperrors.NewValidation("--scope-type and --scope-id are required")
			}
			params := map[string]any{
				"scopeType": scopeType,
				"openId":    scopeID,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "list_op_contracts", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("scope-type", "", i18n.T("合约范围类型: DEPT/PERSONAL (必填)"))
	cmd.Flags().String("scope-id", "", i18n.T("scope-type 对应的钉钉部门 id 或用户 id (必填)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

func newAgoalContractFieldsCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "fields",
		Short:   i18n.T("获取经营合约字段列表"),
		Long:    i18n.T("获取指定组织下经营合约的字段配置信息。"),
		Example: `  dws agoal contract fields --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "list_op_contract_fields", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

func newAgoalContractDetailCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "detail",
		Short:   i18n.T("获取经营合约详情"),
		Long:    i18n.T("根据经营合约 id 获取经营合约的详细信息。"),
		Example: `  dws agoal contract detail --contract-id CONTRACT_ID --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			contractID, _ := cmd.Flags().GetString("contract-id")
			if strings.TrimSpace(contractID) == "" {
				return apperrors.NewValidation("--contract-id is required")
			}
			params := map[string]any{
				"contractId": contractID,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "get_op_contract_detail", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("contract-id", "", i18n.T("经营合约 id (必填)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

func newAgoalContractUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: i18n.T("更新经营合约"),
		Long: i18n.T(`谨慎操作，必须基于查询接口返回的老数据进行修改，本接口是覆盖逻辑。
--dimensions 为 JSON 数组，每个维度包含 id/title/description/weight/objectives/dimensionConfig/children。`),
		Example: `  dws agoal contract update --contract-id CONTRACT_ID --dimensions '[{"id":"dim1","title":"业绩","weight":60}]' --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			contractID, _ := cmd.Flags().GetString("contract-id")
			dimensionsStr, _ := cmd.Flags().GetString("dimensions")
			if strings.TrimSpace(contractID) == "" {
				return apperrors.NewValidation("--contract-id is required")
			}
			if strings.TrimSpace(dimensionsStr) == "" {
				return apperrors.NewValidation("--dimensions is required")
			}
			var dimensionsArr []any
			if err := json.Unmarshal([]byte(dimensionsStr), &dimensionsArr); err != nil {
				return fmt.Errorf("--dimensions must be a valid JSON array: %w", err)
			}
			params := map[string]any{
				"contractId": contractID,
				"dimensions": dimensionsArr,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			if v, _ := cmd.Flags().GetString("audit-config"); v != "" {
				params["auditConfig"] = v
			}
			if v, _ := cmd.Flags().GetString("objective-template"); v != "" {
				params["objectiveTemplate"] = v
			}
			return runAgoalTool(cmd, runner, "update_op_contract", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("contract-id", "", i18n.T("经营合约 id (必填)"))
	cmd.Flags().String("dimensions", "", i18n.T("维度内容列表 JSON 数组 (必填)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	cmd.Flags().String("audit-config", "", i18n.T("审批配置 JSON (可选)"))
	cmd.Flags().String("objective-template", "", i18n.T("合约模板 JSON (可选)"))
	return cmd
}

// ── scorecard: 计分卡管理 ───────────────────────────────────

func newAgoalScorecardCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scorecard",
		Short: i18n.T("计分卡管理"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newAgoalScorecardDetailCommand(runner),
		newAgoalScorecardEntityDetailCommand(runner),
		newAgoalScorecardUpdateCommand(runner),
	)
	return cmd
}

func newAgoalScorecardDetailCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detail",
		Short: i18n.T("获取计分卡详情"),
		Long: i18n.T(`根据部门 id 和时间获取计分卡详情。
--selected-time 接受 ISO-8601 字符串，传入对应周期起始时刻。`),
		Example: `  dws agoal scorecard detail --selected-time "2026-01-01T00:00:00+08:00" --dept-id DEPT_ID --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			selectedTime, _ := cmd.Flags().GetString("selected-time")
			deptID, _ := cmd.Flags().GetString("dept-id")
			if strings.TrimSpace(selectedTime) == "" || strings.TrimSpace(deptID) == "" {
				return apperrors.NewValidation("--selected-time and --dept-id are required")
			}
			selectedTimeMs, err := parseAgoalTimeToMillis(selectedTime)
			if err != nil {
				return fmt.Errorf("--selected-time: %w", err)
			}
			params := map[string]any{
				"selectedTime": selectedTimeMs,
				"deptId":       deptID,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "get_score_card_detail", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("selected-time", "", i18n.T("周期起始时刻 ISO-8601 (必填)，如 \"2026-01-01T00:00:00+08:00\""))
	cmd.Flags().String("dept-id", "", i18n.T("部门 id (必填)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

func newAgoalScorecardEntityDetailCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "entity-detail",
		Short:   i18n.T("获取计分卡实体详情"),
		Long:    i18n.T("根据计分卡 id 和实体 id 获取计分卡实体详情。"),
		Example: `  dws agoal scorecard entity-detail --sc-id SC_ID --entity-id ENTITY_ID --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			scID, _ := cmd.Flags().GetString("sc-id")
			entityID, _ := cmd.Flags().GetString("entity-id")
			if strings.TrimSpace(scID) == "" || strings.TrimSpace(entityID) == "" {
				return apperrors.NewValidation("--sc-id and --entity-id are required")
			}
			params := map[string]any{
				"scId":     scID,
				"entityId": entityID,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "get_score_card_entity_detail", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("sc-id", "", i18n.T("计分卡 id (必填)"))
	cmd.Flags().String("entity-id", "", i18n.T("实体 id (必填)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

func newAgoalScorecardUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: i18n.T("更新计分卡"),
		Long: i18n.T(`谨慎操作，本接口是覆盖逻辑。
--tracking-period-type 支持: MONTHLY(月度)、QUARTERLY(季度)。`),
		Example: `  dws agoal scorecard update --dept-id DEPT_ID --selected-time "2026-01-01T00:00:00+08:00" --id SC_ID --tracking-period-type MONTHLY --content '[{"id":"dim1","title":"业绩","items":[{"id":"item1","title":"收入","target":"100"}]}]' --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			deptID, _ := cmd.Flags().GetString("dept-id")
			selectedTime, _ := cmd.Flags().GetString("selected-time")
			scID, _ := cmd.Flags().GetString("id")
			trackingPeriodType, _ := cmd.Flags().GetString("tracking-period-type")
			contentStr, _ := cmd.Flags().GetString("content")
			if strings.TrimSpace(deptID) == "" || strings.TrimSpace(selectedTime) == "" || strings.TrimSpace(scID) == "" || strings.TrimSpace(trackingPeriodType) == "" {
				return apperrors.NewValidation("--dept-id, --selected-time, --id and --tracking-period-type are required")
			}
			if strings.TrimSpace(contentStr) == "" {
				return apperrors.NewValidation("--content is required")
			}
			selectedTimeMs, err := parseAgoalTimeToMillis(selectedTime)
			if err != nil {
				return fmt.Errorf("--selected-time: %w", err)
			}
			var contentArr []any
			if err := json.Unmarshal([]byte(contentStr), &contentArr); err != nil {
				return fmt.Errorf("--content must be a valid JSON array: %w", err)
			}
			params := map[string]any{
				"deptId":             deptID,
				"selectedTime":       selectedTimeMs,
				"id":                 scID,
				"trackingPeriodType": trackingPeriodType,
				"content":            contentArr,
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "update_score_card", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("dept-id", "", i18n.T("部门 id (必填)"))
	cmd.Flags().String("selected-time", "", i18n.T("周期起始时刻 ISO-8601 (必填)"))
	cmd.Flags().String("id", "", i18n.T("计分卡 id (必填)"))
	cmd.Flags().String("tracking-period-type", "", i18n.T("追踪周期类型: MONTHLY/QUARTERLY (必填)"))
	cmd.Flags().String("content", "", i18n.T("维度列表 JSON 数组 (必填)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

// ── user: 用户目标管理 ──────────────────────────────────────

func newAgoalUserCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: i18n.T("用户目标管理"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newAgoalUserRulesCommand(runner),
		newAgoalUserObjectivesCommand(runner),
	)
	return cmd
}

func newAgoalUserRulesCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rules",
		Short:   i18n.T("获取用户规则周期列表"),
		Long:    i18n.T("查询用户规则周期列表。可选 --user-id，不传则默认取操作人自己。"),
		Example: `  dws agoal user rules --format json
  dws agoal user rules --user-id USER_ID --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if v, _ := cmd.Flags().GetString("user-id"); v != "" {
				params["dingUserId"] = v
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "get_user_rules", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("user-id", "", i18n.T("用户 id (可选，不传则默认操作人自己)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

func newAgoalUserObjectivesCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "objectives",
		Short:   i18n.T("查询用户目标列表"),
		Long:    i18n.T("查询用户目标列表。--period-ids 为逗号分隔的周期 id 列表。"),
		Example: `  dws agoal user objectives --user-id USER_ID --rule-id RULE_ID --period-ids "period1,period2" --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			userID, _ := cmd.Flags().GetString("user-id")
			ruleID, _ := cmd.Flags().GetString("rule-id")
			periodIDs, _ := cmd.Flags().GetString("period-ids")
			if strings.TrimSpace(userID) == "" || strings.TrimSpace(ruleID) == "" || strings.TrimSpace(periodIDs) == "" {
				return apperrors.NewValidation("--user-id, --rule-id and --period-ids are required")
			}
			params := map[string]any{
				"dingUserId":      userID,
				"objectiveRuleId": ruleID,
				"periodIds":       strings.Split(periodIDs, ","),
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "list_user_objectives", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("user-id", "", i18n.T("用户 id (必填)"))
	cmd.Flags().String("rule-id", "", i18n.T("规则 id (必填)"))
	cmd.Flags().String("period-ids", "", i18n.T("周期 id 列表，逗号分隔 (必填)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

// ── obj-template: 目标模板管理 ───────────────────────────────

func newAgoalObjTemplateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "obj-template",
		Short: i18n.T("目标模板管理"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newAgoalObjTemplateListCommand(runner),
		newAgoalObjTemplateCreateOrUpdateCommand(runner),
	)
	return cmd
}

func newAgoalObjTemplateListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   i18n.T("获取目标模板列表"),
		Long:    i18n.T("获取目标模板列表，可选关键词搜索、分页。"),
		Example: `  dws agoal obj-template list --format json
  dws agoal obj-template list --keyword "业绩" --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				params["keyword"] = v
			}
			if v, _ := cmd.Flags().GetInt("page"); v != 0 {
				params["page"] = v
			}
			if v, _ := cmd.Flags().GetInt("page-size"); v != 0 {
				params["pageSize"] = v
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "list_obj_template", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("keyword", "", i18n.T("搜索关键词 (可选)"))
	cmd.Flags().Int("page", 0, i18n.T("页码 (可选)"))
	cmd.Flags().Int("page-size", 0, i18n.T("每页数量 (可选)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

func newAgoalObjTemplateCreateOrUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-or-update",
		Short: i18n.T("新增或更新目标模板"),
		Long: i18n.T(`新增或更新目标模板。新增时 --title 必填；更新时 --template-id 必填。
--dimensions 为 JSON 字符串，更新时必须基于老数据修改。`),
		Example: `  dws agoal obj-template create-or-update --title "业绩模板" --objective-weight --dimension-weight --dimensions '[...]' --format json
  dws agoal obj-template create-or-update --template-id TPL_ID --dimensions '[...]' --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			templateID, _ := cmd.Flags().GetString("template-id")
			title, _ := cmd.Flags().GetString("title")
			dimensionsStr, _ := cmd.Flags().GetString("dimensions")

			// --dimensions is always required.
			if strings.TrimSpace(dimensionsStr) == "" {
				return apperrors.NewValidation("--dimensions is required")
			}
			if strings.TrimSpace(templateID) != "" {
				params["templateId"] = templateID
			}
			if strings.TrimSpace(title) != "" {
				params["title"] = title
			}
			if cmd.Flags().Changed("objective-weight") {
				v, _ := cmd.Flags().GetBool("objective-weight")
				params["objectiveWeight"] = v
			}
			if cmd.Flags().Changed("dimension-weight") {
				v, _ := cmd.Flags().GetBool("dimension-weight")
				params["dimensionWeight"] = v
			}
			if cmd.Flags().Changed("compute-by-weight") {
				v, _ := cmd.Flags().GetBool("compute-by-weight")
				params["computeByWeight"] = v
			}
			params["dimensions"] = dimensionsStr
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "create_or_update_obj_template", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("template-id", "", i18n.T("模板 id（更新时必填）"))
	cmd.Flags().String("title", "", i18n.T("模板标题（新增时必填）"))
	cmd.Flags().Bool("objective-weight", false, i18n.T("是否启用目标权重"))
	cmd.Flags().Bool("dimension-weight", false, i18n.T("是否启用维度权重"))
	cmd.Flags().Bool("compute-by-weight", false, i18n.T("维度是否参与计算"))
	cmd.Flags().String("dimensions", "", i18n.T("模板维度 JSON (必填)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

// ── report: 周月报管理 ──────────────────────────────────────

func newAgoalReportCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: i18n.T("周月报管理"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newAgoalReportListStatisticsCommand(runner),
		newAgoalReportSubmitDetailCommand(runner),
	)
	return cmd
}

func newAgoalReportListStatisticsCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list-statistics",
		Short:   i18n.T("获取周月报数据跟催列表"),
		Long:    i18n.T("返回各规则的人员提交情况统计（按时/迟交/未提交人数）。可选关键词搜索。"),
		Example: `  dws agoal report list-statistics --format json
  dws agoal report list-statistics --keyword "周报规则" --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				params["keyword"] = v
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "list_report_statistics", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("keyword", "", i18n.T("搜索关键词 (可选)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

func newAgoalReportSubmitDetailCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit-detail",
		Short: i18n.T("获取周月报规则提交详情"),
		Long: i18n.T(`获取某规则的人员提交详情。
--submit-state 支持: ON_TIME(按时)、LATE(迟交)、NOT_SUBMITTED(未提交)。`),
		Example: `  dws agoal report submit-detail --template-id TPL_ID --submit-state ON_TIME --format json
  dws agoal report submit-detail --template-id TPL_ID --submit-state LATE --query-date "2026-06-18T00:00:00+08:00" --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			templateID, _ := cmd.Flags().GetString("template-id")
			submitState, _ := cmd.Flags().GetString("submit-state")
			if strings.TrimSpace(templateID) == "" || strings.TrimSpace(submitState) == "" {
				return apperrors.NewValidation("--template-id and --submit-state are required")
			}
			params := map[string]any{
				"templateId":  templateID,
				"submitState": submitState,
			}
			if v, _ := cmd.Flags().GetString("query-date"); v != "" {
				queryDateMs, err := parseAgoalTimeToMillis(v)
				if err != nil {
					return fmt.Errorf("--query-date: %w", err)
				}
				t := time.UnixMilli(queryDateMs)
				params["queryDate"] = t.Format("2006-01-02")
			}
			if v, _ := cmd.Flags().GetInt("page"); v != 0 {
				params["page"] = v
			}
			if v, _ := cmd.Flags().GetInt("page-size"); v != 0 {
				params["pageSize"] = v
			}
			if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				params["keyword"] = v
			}
			if v, _ := cmd.Flags().GetString("request-id"); v != "" {
				params["requestId"] = v
			}
			return runAgoalTool(cmd, runner, "get_submit_detail", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("template-id", "", i18n.T("周月报模板 id (必填)"))
	cmd.Flags().String("submit-state", "", i18n.T("提交状态: ON_TIME/LATE/NOT_SUBMITTED (必填)"))
	cmd.Flags().String("query-date", "", i18n.T("查询日期 ISO-8601 (可选)"))
	cmd.Flags().Int("page", 0, i18n.T("页码 (可选)"))
	cmd.Flags().Int("page-size", 0, i18n.T("每页数量 (可选)"))
	cmd.Flags().String("keyword", "", i18n.T("搜索员工名称 (可选)"))
	cmd.Flags().String("request-id", "", i18n.T("requestId (可选)"))
	return cmd
}

// ── helper ─────────────────────────────────────────────────

// parseAgoalTimeToMillis 将 ISO-8601 时间字符串解析为毫秒时间戳。
// 支持格式：RFC3339（含时区）、无时区（默认 Asia/Shanghai）、仅日期。
func parseAgoalTimeToMillis(value string) (int64, error) {
	// 尝试带时区的 RFC3339 格式
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UnixMilli(), nil
	}

	shanghaiLoc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		shanghaiLoc = time.FixedZone("CST", 8*3600)
	}

	// 尝试不带时区的日期时间格式
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", value, shanghaiLoc); err == nil {
		return t.UnixMilli(), nil
	}

	// 尝试空格分隔的日期时间格式
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", value, shanghaiLoc); err == nil {
		return t.UnixMilli(), nil
	}

	// 尝试仅日期格式
	if t, err := time.ParseInLocation("2006-01-02", value, shanghaiLoc); err == nil {
		return t.UnixMilli(), nil
	}

	return 0, fmt.Errorf("invalid ISO-8601 time format %q, expected e.g. \"2026-01-01T00:00:00+08:00\"", value)
}

func runAgoalTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	invocation := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd),
		"agoal",
		tool,
		params,
	)
	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, invocation)
	}
	result, err := runner.Run(cmd.Context(), invocation)
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}
