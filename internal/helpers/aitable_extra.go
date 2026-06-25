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
	"fmt"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/spf13/cobra"
)

func runAitableHelperTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	return runAitableProductTool(cmd, runner, "aitable-helper", tool, params)
}

func newAitableRecordHistoryListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "history-list",
		Short:             i18n.T("查询行记录变更历史"),
		Example:           "  dws aitable record history-list --base-id BASE_ID --table-id TABLE_ID --record-id RECORD_ID --limit 50 --offset 0",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, err := requiredAitableBaseTable(cmd)
			if err != nil {
				return err
			}
			recordID, err := aitableRequiredFlag(cmd, "record-id")
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId":   baseID,
				"tableId":  tableID,
				"recordId": recordID,
			}
			if cmd.Flags().Changed("offset") {
				offset, _ := cmd.Flags().GetInt("offset")
				if offset < 0 {
					return apperrors.NewValidation(fmt.Sprintf("--offset must be >= 0, got %d", offset))
				}
				params["offset"] = offset
			}
			if cmd.Flags().Changed("limit") {
				limit, _ := cmd.Flags().GetInt("limit")
				if limit < 1 || limit > 50 {
					return apperrors.NewValidation(fmt.Sprintf("--limit must be in [1, 50], got %d", limit))
				}
				params["limit"] = limit
			}
			return runAitableHelperTool(cmd, runner, "query_record_history", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("record-id", "", i18n.T("Record ID (必填)"))
	cmd.Flags().Int("offset", 0, i18n.T("分页偏移量，>= 0"))
	cmd.Flags().Int("limit", 0, i18n.T("分页大小 [1, 50]，不传使用服务端默认值"))
	return cmd
}

func newAitableRecordShareURLCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "share-url",
		Short:             i18n.T("批量获取记录分享链接"),
		Example:           "  dws aitable record share-url --base-id BASE_ID --table-id TABLE_ID --record-ids rec1,rec2 --view-id VIEW_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, err := requiredAitableBaseTable(cmd)
			if err != nil {
				return err
			}
			recordIDsRaw, err := aitableRequiredFlag(cmd, "record-ids")
			if err != nil {
				return err
			}
			recordIDs := parseAitableCSVValues(recordIDsRaw)
			if len(recordIDs) == 0 {
				return apperrors.NewValidation("--record-ids must contain at least one record ID")
			}
			if len(recordIDs) > 20 {
				return apperrors.NewValidation(fmt.Sprintf("--record-ids exceeds limit: got %d, max 20", len(recordIDs)))
			}
			params := map[string]any{
				"baseId":    baseID,
				"tableId":   tableID,
				"recordIds": recordIDs,
			}
			if viewID := aitableStringFlag(cmd, "view-id"); viewID != "" {
				params["viewId"] = viewID
			}
			return runAitableHelperTool(cmd, runner, "get_record_share_url", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("record-ids", "", i18n.T("Record ID 列表，逗号分隔，单次最多 20 条 (必填)"))
	cmd.Flags().String("view-id", "", i18n.T("View ID，可选"))
	return cmd
}

func newAitableRecordUpsertCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "upsert",
		Short:             i18n.T("批量创建或更新记录"),
		Example:           "  dws aitable record upsert --base-id BASE_ID --table-id TABLE_ID --records '[{\"recordId\":\"rec1\",\"cells\":{\"fld\":\"x\"}},{\"cells\":{\"fld\":\"new\"}}]'",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, err := requiredAitableBaseTable(cmd)
			if err != nil {
				return err
			}
			records, err := resolveAitableRecordsInput(cmd, "records", "fields")
			if err != nil {
				return err
			}
			if len(records) == 0 {
				return apperrors.NewValidation("--records must contain at least one record")
			}
			if len(records) > 100 {
				return apperrors.NewValidation(fmt.Sprintf("--records exceeds limit: got %d, max 100", len(records)))
			}
			return runAitableHelperTool(cmd, runner, "record_upsert", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"records": records,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("records", "", i18n.T("记录 JSON 数组 (必填，可改用 --records-file)"))
	cmd.Flags().String("records-file", "", i18n.T("从文件读取 records JSON"))
	addAitableHiddenStringFlag(cmd, "fields", "--records 的兼容别名")
	return cmd
}

func newAitableRecordPrimaryDocGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "primary-doc-get",
		Short:             i18n.T("查询记录的主键文档"),
		Example:           "  dws aitable record primary-doc-get --base-id BASE_ID --table-id TABLE_ID --record-id RECORD_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, err := requiredAitableBaseTable(cmd)
			if err != nil {
				return err
			}
			recordID, err := aitableRequiredFlag(cmd, "record-id")
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "get_primary_doc", map[string]any{
				"baseId":   baseID,
				"tableId":  tableID,
				"recordId": recordID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("record-id", "", i18n.T("Record ID (必填)"))
	return cmd
}

func newAitableRecordPrimaryDocCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "primary-doc-create",
		Short:             i18n.T("为记录创建主键文档"),
		Example:           "  dws aitable record primary-doc-create --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --record-id RECORD_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, err := requiredAitableBaseTable(cmd)
			if err != nil {
				return err
			}
			fieldID, err := aitableRequiredFlag(cmd, "field-id")
			if err != nil {
				return err
			}
			recordID, err := aitableRequiredFlag(cmd, "record-id")
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "create_primary_doc", map[string]any{
				"baseId":   baseID,
				"tableId":  tableID,
				"fieldId":  fieldID,
				"recordId": recordID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("field-id", "", i18n.T("PrimaryDoc 字段 ID (必填)"))
	cmd.Flags().String("record-id", "", i18n.T("Record ID (必填)"))
	return cmd
}

func newAitableViewGetLockCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewHelperGetCommand(runner, "lock", i18n.T("获取视图锁定状态"), "get_view_lock_status")
}

func newAitableViewGetFrozenColsCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewHelperGetCommand(runner, "frozen-cols", i18n.T("获取视图冻结列数"), "get_frozen_columns_of_view")
}

func newAitableViewGetRowHeightCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewHelperGetCommand(runner, "row-height", i18n.T("获取视图行高"), "get_cell_height_of_view")
}

func newAitableViewGetFillColorRuleCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "fill-color-rule",
		Short:             i18n.T("获取视图数据高亮规则"),
		Example:           "  dws aitable view get fill-color-rule --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "get_views", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewIds": []string{viewID},
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	return cmd
}

func newAitableViewHelperGetCommand(runner executor.Runner, use, short, tool string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             short,
		Example:           fmt.Sprintf("  dws aitable view get %s --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID", use),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := requiredAitableViewParams(cmd)
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, tool, params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	return cmd
}

func newAitableViewUpdateFrozenColsCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "frozen-cols",
		Short:             i18n.T("更新视图冻结列数"),
		Example:           "  dws aitable view update frozen-cols --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --count 1",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("count") {
				return apperrors.NewValidation("--count is required")
			}
			count, _ := cmd.Flags().GetInt("count")
			if count < 0 {
				return apperrors.NewValidation(fmt.Sprintf("--count must be >= 0, got %d", count))
			}
			params, err := requiredAitableViewParams(cmd)
			if err != nil {
				return err
			}
			params["count"] = count
			return runAitableHelperTool(cmd, runner, "set_frozen_columns_of_view", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().Int("count", 0, i18n.T("冻结列数，>= 0；0 表示取消冻结 (必填)"))
	return cmd
}

func newAitableViewUpdateRowHeightCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "row-height",
		Short:             i18n.T("更新视图行高"),
		Example:           "  dws aitable view update row-height --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --cell-height 56",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("cell-height") {
				return apperrors.NewValidation("--cell-height is required")
			}
			cellHeight, _ := cmd.Flags().GetInt("cell-height")
			if cellHeight <= 0 {
				return apperrors.NewValidation(fmt.Sprintf("--cell-height must be > 0, got %d", cellHeight))
			}
			params, err := requiredAitableViewParams(cmd)
			if err != nil {
				return err
			}
			params["cellHeight"] = cellHeight
			return runAitableHelperTool(cmd, runner, "set_cell_height_of_view", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().Int("cell-height", 0, i18n.T("单元格高度，像素值 (必填)"))
	return cmd
}

func newAitableViewUpdateFillColorRuleCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "fill-color-rule",
		Short:             i18n.T("更新视图数据高亮规则"),
		Example:           "  dws aitable view update fill-color-rule --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --json '[]'",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := aitableRequiredFlag(cmd, "json")
			if err != nil {
				return err
			}
			conditionalFormats, err := parseAitableJSONArray(raw, "json")
			if err != nil {
				return err
			}
			params, err := requiredAitableViewParams(cmd)
			if err != nil {
				return err
			}
			params["conditionalFormats"] = conditionalFormats
			return runAitableTool(cmd, runner, "set_view_fill_color_rule", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("json", "", i18n.T("conditionalFormats JSON 数组 (必填)"))
	return cmd
}

func newAitableViewLockCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "lock",
		Short:             i18n.T("锁定或解锁视图"),
		Example:           "  dws aitable view lock --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --off",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := requiredAitableViewParams(cmd)
			if err != nil {
				return err
			}
			action := "lock"
			if off, _ := cmd.Flags().GetBool("off"); off {
				action = "unlock"
			}
			params["action"] = action
			return runAitableHelperTool(cmd, runner, "lock_or_unlock_view", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().Bool("off", false, i18n.T("解锁视图；不传则锁定"))
	return cmd
}

func newAitableViewDuplicateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "duplicate",
		Short:             i18n.T("复制视图"),
		Example:           "  dws aitable view duplicate --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --new-name 副本视图",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId":       baseID,
				"tableId":      tableID,
				"sourceViewId": viewID,
			}
			if newName := aitableStringFlag(cmd, "new-name"); newName != "" {
				params["newViewName"] = newName
			}
			return runAitableHelperTool(cmd, runner, "duplicate_view", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("new-name", "", i18n.T("新视图名称"))
	return cmd
}

func requiredAitableViewParams(cmd *cobra.Command) (map[string]any, error) {
	baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"baseId":  baseID,
		"tableId": tableID,
		"viewId":  viewID,
	}, nil
}

func newAitableWorkflowCommand(runner executor.Runner) *cobra.Command {
	group := newAitableExtraGroup("workflow", i18n.T("自动化工作流管理"))
	group.AddCommand(
		newAitableWorkflowEnableCommand(runner),
		newAitableWorkflowDisableCommand(runner),
		newAitableWorkflowGetCommand(runner),
		newAitableWorkflowListCommand(runner),
	)
	return group
}

func newAitableWorkflowEnableCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "enable",
		Short:             i18n.T("启用指定工作流"),
		Example:           "  dws aitable workflow enable --base-id BASE_ID --workflow-id WORKFLOW_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			workflowID, err := aitableRequiredFlag(cmd, "workflow-id")
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "enable_workflow", map[string]any{
				"baseId":     baseID,
				"workflowId": workflowID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().String("workflow-id", "", i18n.T("工作流 ID (必填)"))
	return cmd
}

func newAitableWorkflowDisableCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "disable",
		Short:             i18n.T("禁用指定工作流"),
		Example:           "  dws aitable workflow disable --base-id BASE_ID --workflow-id WORKFLOW_ID --yes",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			workflowID, err := aitableRequiredFlag(cmd, "workflow-id")
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, i18n.T("工作流"), workflowID) {
				return nil
			}
			return runAitableHelperTool(cmd, runner, "disable_workflow", map[string]any{
				"baseId":     baseID,
				"workflowId": workflowID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().String("workflow-id", "", i18n.T("工作流 ID (必填)"))
	return cmd
}

func newAitableWorkflowGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("获取单个工作流详情"),
		Example:           "  dws aitable workflow get --base-id BASE_ID --workflow-id WORKFLOW_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			workflowID, err := aitableRequiredFlag(cmd, "workflow-id")
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "get_workflow", map[string]any{
				"baseId":     baseID,
				"workflowId": workflowID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().String("workflow-id", "", i18n.T("工作流 ID (必填)"))
	return cmd
}

func newAitableWorkflowListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             i18n.T("列出 Base 下的工作流"),
		Example:           "  dws aitable workflow list --base-id BASE_ID --limit 50 --offset 100",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			params := map[string]any{"baseId": baseID}
			if cmd.Flags().Changed("limit") {
				limit, _ := cmd.Flags().GetInt("limit")
				if limit < 1 || limit > 100 {
					return apperrors.NewValidation(fmt.Sprintf("--limit must be in [1, 100], got %d", limit))
				}
				params["limit"] = limit
			}
			if cmd.Flags().Changed("offset") {
				offset, _ := cmd.Flags().GetInt("offset")
				if offset < 0 {
					return apperrors.NewValidation(fmt.Sprintf("--offset must be >= 0, got %d", offset))
				}
				params["offset"] = offset
			}
			return runAitableHelperTool(cmd, runner, "list_workflows", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().Int("limit", 0, i18n.T("分页大小 [1, 100]，不传使用服务端默认值"))
	cmd.Flags().Int("offset", 0, i18n.T("分页偏移量，>= 0"))
	return cmd
}

func newAitableAdvpermCommand(runner executor.Runner) *cobra.Command {
	group := newAitableExtraGroup("advperm", i18n.T("高级权限管理"))
	group.AddCommand(
		newAitableAdvpermEnableCommand(runner),
		newAitableAdvpermDisableCommand(runner),
		newAitableAdvpermRoleListCommand(runner),
		newAitableAdvpermRoleGetCommand(runner),
		newAitableAdvpermRoleCreateCommand(runner),
		newAitableAdvpermRoleUpdateCommand(runner),
		newAitableAdvpermRoleDeleteCommand(runner),
	)
	return group
}

func newAitableAdvpermEnableCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "enable",
		Short:             i18n.T("开启高级权限总开关"),
		Example:           "  dws aitable advperm enable --base-id BASE_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "set_advanced_permission", map[string]any{
				"baseId":  baseID,
				"enabled": true,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	return cmd
}

func newAitableAdvpermDisableCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "disable",
		Short:             i18n.T("关闭高级权限总开关"),
		Example:           "  dws aitable advperm disable --base-id BASE_ID --yes",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, i18n.T("高级权限"), baseID) {
				return nil
			}
			return runAitableHelperTool(cmd, runner, "set_advanced_permission", map[string]any{
				"baseId":  baseID,
				"enabled": false,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	return cmd
}

func newAitableAdvpermRoleListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "role-list",
		Short:             i18n.T("列出 Base 下所有角色"),
		Example:           "  dws aitable advperm role-list --base-id BASE_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "list_roles", map[string]any{"baseId": baseID})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	return cmd
}

func newAitableAdvpermRoleGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "role-get",
		Short:             i18n.T("获取单个角色完整配置"),
		Example:           "  dws aitable advperm role-get --base-id BASE_ID --role-id ROLE_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, roleID, err := requiredAitableRoleParams(cmd)
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "get_role", map[string]any{
				"baseId": baseID,
				"roleId": roleID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().String("role-id", "", i18n.T("角色 ID (必填)"))
	return cmd
}

func newAitableAdvpermRoleCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "role-create",
		Short:             i18n.T("创建自定义角色"),
		Example:           "  dws aitable advperm role-create --base-id BASE_ID --name 市场可读 --sub-roles '[{\"targetId\":\"tbl\",\"targetType\":\"sheet\",\"authLevel\":\"read\"}]'",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			name, err := aitableRequiredFlag(cmd, "name")
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId": baseID,
				"name":   name,
			}
			if err := appendAitableRoleOptionalFlags(cmd, params); err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "create_role", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	addAitableRoleMutationFlags(cmd, true)
	return cmd
}

func newAitableAdvpermRoleUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "role-update",
		Short:             i18n.T("增量更新自定义角色配置"),
		Example:           "  dws aitable advperm role-update --base-id BASE_ID --role-id ROLE_ID --name 新名字",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, roleID, err := requiredAitableRoleParams(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId": baseID,
				"roleId": roleID,
			}
			if name := aitableStringFlag(cmd, "name"); name != "" {
				params["name"] = name
			}
			if err := appendAitableRoleOptionalFlags(cmd, params); err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "patch_role", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().String("role-id", "", i18n.T("角色 ID (必填)"))
	addAitableRoleMutationFlags(cmd, false)
	return cmd
}

func newAitableAdvpermRoleDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "role-delete",
		Short:             i18n.T("删除自定义角色"),
		Example:           "  dws aitable advperm role-delete --base-id BASE_ID --role-id ROLE_ID --yes",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, roleID, err := requiredAitableRoleParams(cmd)
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, i18n.T("角色"), roleID) {
				return nil
			}
			return runAitableHelperTool(cmd, runner, "delete_role", map[string]any{
				"baseId": baseID,
				"roleId": roleID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().String("role-id", "", i18n.T("角色 ID (必填)"))
	return cmd
}

func requiredAitableRoleParams(cmd *cobra.Command) (string, string, error) {
	baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
	if err != nil {
		return "", "", err
	}
	roleID, err := aitableRequiredFlag(cmd, "role-id")
	if err != nil {
		return "", "", err
	}
	return baseID, roleID, nil
}

func addAitableRoleMutationFlags(cmd *cobra.Command, nameRequired bool) {
	label := i18n.T("角色名称")
	if nameRequired {
		label = i18n.T("角色名称 (必填)")
	}
	cmd.Flags().String("name", "", label)
	cmd.Flags().String("role-type", "", i18n.T("角色类型"))
	cmd.Flags().String("flow-type", "", i18n.T("流程类型"))
	cmd.Flags().String("sub-roles", "", i18n.T("子角色配置 JSON 数组"))
}

func appendAitableRoleOptionalFlags(cmd *cobra.Command, params map[string]any) error {
	if roleType := aitableStringFlag(cmd, "role-type"); roleType != "" {
		params["roleType"] = roleType
	}
	if flowType := aitableStringFlag(cmd, "flow-type"); flowType != "" {
		params["flowType"] = flowType
	}
	if subRolesRaw := aitableStringFlag(cmd, "sub-roles"); subRolesRaw != "" {
		subRoles, err := parseAitableJSONArray(subRolesRaw, "sub-roles")
		if err != nil {
			return err
		}
		params["subRoles"] = subRoles
	}
	return nil
}

func newAitableSectionCommand(runner executor.Runner) *cobra.Command {
	group := newAitableExtraGroup("section", i18n.T("文件夹与节点管理"))
	group.AddCommand(
		newAitableSectionCreateCommand(runner),
		newAitableSectionRenameCommand(runner),
		newAitableSectionDeleteCommand(runner),
		newAitableSectionReorderCommand(runner),
		newAitableSectionListEmptyCommand(runner),
		newAitableSectionListNodesCommand(runner),
		newAitableSectionMoveNodeCommand(runner),
	)
	return group
}

func newAitableSectionCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("创建文件夹"),
		Example:           "  dws aitable section create --base-id BASE_ID --name 我的文件夹 --parent-section-id SECTION_ID --index 0",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			name, err := aitableRequiredFlag(cmd, "name")
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId": baseID,
				"name":   name,
			}
			if cmd.Flags().Changed("parent-section-id") {
				parentSectionID, _ := cmd.Flags().GetString("parent-section-id")
				params["parentSectionId"] = parentSectionID
			}
			if index, _ := cmd.Flags().GetInt("index"); index >= 0 {
				params["index"] = index
			}
			return runAitableHelperTool(cmd, runner, "create_section", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().String("name", "", i18n.T("文件夹名称 (必填)"))
	cmd.Flags().String("parent-section-id", "", i18n.T("父文件夹 ID；空字符串表示根目录"))
	cmd.Flags().Int("index", -1, i18n.T("目标位置，0-based；不传则追加"))
	return cmd
}

func newAitableSectionRenameCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "rename",
		Short:             i18n.T("重命名文件夹"),
		Example:           "  dws aitable section rename --base-id BASE_ID --section-id SECTION_ID --new-name 新名称",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, sectionID, err := requiredAitableSectionParams(cmd)
			if err != nil {
				return err
			}
			newName, err := aitableRequiredFlag(cmd, "new-name")
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "rename_section", map[string]any{
				"baseId":    baseID,
				"sectionId": sectionID,
				"newName":   newName,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().String("section-id", "", i18n.T("文件夹 ID (必填)"))
	cmd.Flags().String("new-name", "", i18n.T("新文件夹名称 (必填)"))
	return cmd
}

func newAitableSectionDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             i18n.T("删除文件夹"),
		Example:           "  dws aitable section delete --base-id BASE_ID --section-id SECTION_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, sectionID, err := requiredAitableSectionParams(cmd)
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "delete_section", map[string]any{
				"baseId":    baseID,
				"sectionId": sectionID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().String("section-id", "", i18n.T("文件夹 ID (必填)"))
	return cmd
}

func newAitableSectionReorderCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "reorder",
		Short:             i18n.T("调整文件夹顺序"),
		Example:           "  dws aitable section reorder --base-id BASE_ID --section-id SECTION_ID --target-index 0",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, sectionID, err := requiredAitableSectionParams(cmd)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("target-index") {
				return apperrors.NewValidation("--target-index is required")
			}
			targetIndex, _ := cmd.Flags().GetInt("target-index")
			if targetIndex < 0 {
				return apperrors.NewValidation(fmt.Sprintf("--target-index must be >= 0, got %d", targetIndex))
			}
			return runAitableHelperTool(cmd, runner, "reorder_section", map[string]any{
				"baseId":      baseID,
				"sectionId":   sectionID,
				"targetIndex": targetIndex,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().String("section-id", "", i18n.T("文件夹 ID (必填)"))
	cmd.Flags().Int("target-index", -1, i18n.T("目标位置，0-based (必填)"))
	return cmd
}

func newAitableSectionListEmptyCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list-empty",
		Short:             i18n.T("列出空文件夹"),
		Example:           "  dws aitable section list-empty --base-id BASE_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "list_empty_sections", map[string]any{"baseId": baseID})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	return cmd
}

func newAitableSectionListNodesCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list-nodes",
		Short:             i18n.T("列出全部节点"),
		Example:           "  dws aitable section list-nodes --base-id BASE_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return runAitableHelperTool(cmd, runner, "list_nsheet_nodes", map[string]any{"baseId": baseID})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	return cmd
}

func newAitableSectionMoveNodeCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "move-node",
		Short:             i18n.T("移动节点"),
		Example:           "  dws aitable section move-node --base-id BASE_ID --node-id NODE_ID --new-parent-section-id SECTION_ID --target-index 0",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			nodeID, err := aitableRequiredFlag(cmd, "node-id")
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("new-parent-section-id") {
				return apperrors.NewValidation("--new-parent-section-id is required; pass empty string to move to base root")
			}
			newParentSectionID, _ := cmd.Flags().GetString("new-parent-section-id")
			params := map[string]any{
				"baseId":             baseID,
				"nodeId":             nodeID,
				"newParentSectionId": newParentSectionID,
			}
			if targetIndex, _ := cmd.Flags().GetInt("target-index"); targetIndex >= 0 {
				params["targetIndex"] = targetIndex
			}
			return runAitableHelperTool(cmd, runner, "move_nsheet_node", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseFlag(cmd)
	cmd.Flags().String("node-id", "", i18n.T("要移动的节点 ID (必填)"))
	cmd.Flags().String("new-parent-section-id", "", i18n.T("目标父文件夹 ID；空字符串表示根目录 (必填)"))
	cmd.Flags().Int("target-index", -1, i18n.T("目标位置，0-based"))
	return cmd
}

func requiredAitableSectionParams(cmd *cobra.Command) (string, string, error) {
	baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
	if err != nil {
		return "", "", err
	}
	sectionID, err := aitableRequiredFlag(cmd, "section-id")
	if err != nil {
		return "", "", err
	}
	return baseID, sectionID, nil
}

func addAitableBaseFlag(cmd *cobra.Command) {
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
}

func newAitableExtraGroup(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:               use,
		Short:             short,
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}
