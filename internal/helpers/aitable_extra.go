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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/spf13/cobra"
)

func runAitableHelperTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	return runAitableProductTool(cmd, runner, "aitable-helper", tool, params)
}

func newAitableFieldSearchOptionsCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "search-options",
		Short:             i18n.T("搜索单选/多选字段选项"),
		Example:           "  dws aitable field search-options --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --keyword 已完成",
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
			params := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"fieldId": fieldID,
			}
			if keyword := aitableStringFlag(cmd, "keyword"); keyword != "" {
				params["keyword"] = keyword
			}
			if cmd.Flags().Changed("limit") {
				limit, _ := cmd.Flags().GetInt("limit")
				if limit <= 0 || limit > 3000 {
					return apperrors.NewValidation(fmt.Sprintf("--limit must be in [1, 3000], got %d", limit))
				}
				params["limit"] = limit
			}
			return runAitableTool(cmd, runner, "search_field_options", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("field-id", "", i18n.T("目标字段 ID，必须是 singleSelect 或 multipleSelect 类型 (必填)"))
	cmd.Flags().String("keyword", "", i18n.T("选项名称模糊搜索关键词"))
	cmd.Flags().Int("limit", 0, i18n.T("返回 option 数量上限 [1,3000]"))
	return cmd
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

func newAitableViewGetCardCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewGetProjectionCommand(runner, "card", "card", nil, true)
}

func newAitableViewGetTimebarCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewGetProjectionCommand(runner, "timebar", "ganttTimebar", []string{"Gantt"}, false)
}

func newAitableViewGetAggregateCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewGetProjectionCommand(runner, "aggregate", "aggregate", []string{"Grid"}, false)
}

func newAitableViewGetFilterCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewGetProjectionCommand(runner, "filter", "filter", nil, false)
}

func newAitableViewGetSortCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewGetProjectionCommand(runner, "sort", "sort", nil, false)
}

func newAitableViewGetGroupCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewGetProjectionCommand(runner, "group", "group", nil, false)
}

func newAitableViewGetVisibleFieldsCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewGetProjectionCommand(runner, "visible-fields", "columns", nil, false)
}

func newAitableViewGetFieldWidthsCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewGetProjectionCommand(runner, "field-widths", "custom.widthMap", []string{"Grid"}, false)
}

func newAitableViewGetProjectionCommand(runner executor.Runner, use, blockKey string, allowedViewTypes []string, dynamicCard bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             i18n.T("获取视图 " + use + " 配置"),
		Example:           fmt.Sprintf("  dws aitable view get %s --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID", use),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewIds": []string{viewID},
			}
			result, err := runAitableProductToolResult(cmd, runner, "aitable", "get_views", params)
			if err != nil {
				return err
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, result)
			}
			view, viewType, err := aitableViewFromResult(result, viewID)
			if err != nil {
				return err
			}
			key := blockKey
			if dynamicCard {
				key, err = aitableDispatchCardKey(viewType)
				if err != nil {
					return err
				}
			} else if err := aitableRequireViewType(viewType, use, allowedViewTypes); err != nil {
				return err
			}
			data := aitableWalkViewPath(view, key)
			if data == nil {
				data = aitableDefaultViewProjection(use)
			}
			return writeCommandPayload(cmd, aitableProjectionEnvelope(result, data))
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	return cmd
}

func newAitableViewUpdateCardCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "card",
		Short:             i18n.T("更新视图 card 配置"),
		Example:           `  dws aitable view update card --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --cover-field-id fldXXX`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			noCover, _ := cmd.Flags().GetBool("no-cover")
			coverFieldID := aitableStringFlag(cmd, "cover-field-id")
			if noCover && coverFieldID != "" {
				return apperrors.NewValidation("--no-cover 与 --cover-field-id 互斥，请只指定一个")
			}
			baseID, tableID, viewID, blockKey, err := aitableViewUpdatePreflight(cmd, runner, "card", nil, true)
			if err != nil {
				return err
			}
			typed := map[string]any{}
			if noCover {
				typed["coverFieldId"] = "NONE"
			} else if coverFieldID != "" {
				typed["coverFieldId"] = coverFieldID
			}
			aitableCollectStringFlag(cmd, "cover-resize-mode", "coverResizeMode", typed)
			aitableCollectBoolFlag(cmd, "hidden-field-title", "hiddenFieldTitle", typed)
			aitableCollectStringFlag(cmd, "cover-mode", "coverMode", typed)
			aitableCollectBoolFlag(cmd, "display-field-name", "displayFieldName", typed)
			block, err := aitableMergeJSONObjectFlag(cmd, typed)
			if err != nil {
				return err
			}
			return runAitableViewUpdateBlock(cmd, runner, baseID, tableID, viewID, blockKey, block, nil)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("json", "", i18n.T("完整 card JSON 对象"))
	cmd.Flags().String("cover-field-id", "", i18n.T("封面字段 ID"))
	cmd.Flags().Bool("no-cover", false, i18n.T("清除封面字段"))
	cmd.Flags().String("cover-resize-mode", "", i18n.T("封面裁剪模式: cover/contain/stretch"))
	cmd.Flags().Bool("hidden-field-title", false, i18n.T("Kanban 是否隐藏字段标题"))
	cmd.Flags().String("cover-mode", "", i18n.T("Gallery 封面模式"))
	cmd.Flags().Bool("display-field-name", false, i18n.T("Gallery 是否显示字段名"))
	return cmd
}

func newAitableViewUpdateTimebarCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "timebar",
		Short:             i18n.T("更新视图 timebar 配置"),
		Example:           `  dws aitable view update timebar --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --start-field fldStart`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, blockKey, err := aitableViewUpdatePreflight(cmd, runner, "ganttTimebar", []string{"Gantt"}, false)
			if err != nil {
				return err
			}
			typed := map[string]any{}
			aitableCollectStringFlag(cmd, "start-field", "startField", typed)
			aitableCollectStringFlag(cmd, "end-field", "endField", typed)
			aitableCollectStringFlag(cmd, "display-field-id", "displayFieldId", typed)
			aitableCollectStringFlag(cmd, "timeline-scale", "timelineScale", typed)
			aitableCollectBoolFlag(cmd, "official-holiday", "officialHoliday", typed)
			if raw := aitableStringFlag(cmd, "color-configs"); raw != "" {
				arr, err := parseAitableJSONArray(raw, "color-configs")
				if err != nil {
					return err
				}
				typed["colorConfigs"] = arr
			}
			block, err := aitableMergeJSONObjectFlag(cmd, typed)
			if err != nil {
				return err
			}
			return runAitableViewUpdateBlock(cmd, runner, baseID, tableID, viewID, blockKey, block, nil)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("json", "", i18n.T("完整 ganttTimebar JSON 对象"))
	cmd.Flags().String("start-field", "", i18n.T("开始日期字段 ID"))
	cmd.Flags().String("end-field", "", i18n.T("结束日期字段 ID"))
	cmd.Flags().String("display-field-id", "", i18n.T("显示字段 ID"))
	cmd.Flags().String("timeline-scale", "", i18n.T("时间轴粒度"))
	cmd.Flags().String("color-configs", "", i18n.T("颜色配置 JSON 数组"))
	cmd.Flags().Bool("official-holiday", false, i18n.T("是否显示法定节假日"))
	return cmd
}

func newAitableViewUpdateAggregateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "aggregate",
		Short:             i18n.T("更新视图字段聚合统计"),
		Example:           `  dws aitable view update aggregate --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --field-id fldX --action SUM`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, blockKey, err := aitableViewUpdatePreflight(cmd, runner, "aggregate", []string{"Grid"}, false)
			if err != nil {
				return err
			}
			typed := map[string]any{}
			fieldID := aitableStringFlag(cmd, "field-id")
			action := aitableStringFlag(cmd, "action")
			if (fieldID == "") != (action == "") {
				return apperrors.NewValidation("--field-id and --action must be specified together")
			}
			if fieldID != "" {
				typed[fieldID] = action
			}
			if raw := aitableStringFlag(cmd, "clear-field-id"); raw != "" {
				for _, key := range parseAitableCSVValues(raw) {
					typed[key] = nil
				}
			}
			block, err := aitableMergeJSONObjectFlag(cmd, typed)
			if err != nil {
				return err
			}
			return runAitableViewUpdateBlock(cmd, runner, baseID, tableID, viewID, blockKey, block, nil)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("json", "", i18n.T("aggregate JSON 对象"))
	cmd.Flags().String("field-id", "", i18n.T("字段 ID"))
	cmd.Flags().String("action", "", i18n.T("聚合动作"))
	cmd.Flags().String("clear-field-id", "", i18n.T("要清除聚合的字段 ID，逗号分隔"))
	return cmd
}

func newAitableViewUpdateFieldWidthsCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "field-widths",
		Short:             i18n.T("更新视图字段列宽"),
		Example:           `  dws aitable view update field-widths --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --field-id fldX --width 200`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, blockKey, err := aitableViewUpdatePreflight(cmd, runner, "fieldWidths", []string{"Grid"}, false)
			if err != nil {
				return err
			}
			typed := map[string]any{}
			fieldID := aitableStringFlag(cmd, "field-id")
			widthChanged := cmd.Flags().Changed("width")
			if (fieldID == "") != !widthChanged {
				return apperrors.NewValidation("--field-id and --width must be specified together")
			}
			if fieldID != "" {
				width, _ := cmd.Flags().GetInt("width")
				typed[fieldID] = width
			}
			block, err := aitableMergeJSONObjectFlag(cmd, typed)
			if err != nil {
				return err
			}
			return runAitableViewUpdateBlock(cmd, runner, baseID, tableID, viewID, blockKey, block, nil)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("json", "", i18n.T("fieldWidths JSON 对象"))
	cmd.Flags().String("field-id", "", i18n.T("字段 ID"))
	cmd.Flags().Int("width", 0, i18n.T("列宽像素值"))
	return cmd
}

func newAitableViewUpdateVisibleFieldsCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "visible-fields",
		Short:             i18n.T("更新视图可见字段列表"),
		Example:           `  dws aitable view update visible-fields --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --field-ids fld1,fld2`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			fieldIDs := parseAitableCSVValues(aitableStringFlag(cmd, "field-ids"))
			if raw := aitableStringFlag(cmd, "json"); raw != "" {
				arr, err := parseAitableJSONArray(raw, "json")
				if err != nil {
					return err
				}
				fieldIDs = fieldIDs[:0]
				for _, item := range arr {
					s, ok := item.(string)
					if !ok {
						return apperrors.NewValidation(fmt.Sprintf("--json array elements must be strings, got %T", item))
					}
					fieldIDs = append(fieldIDs, s)
				}
			}
			if len(fieldIDs) == 0 {
				return apperrors.NewValidation("must specify --field-ids or --json")
			}
			return runAitableViewUpdateBlock(cmd, runner, baseID, tableID, viewID, "visibleFieldIds", fieldIDs, nil)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("field-ids", "", i18n.T("可见字段 ID 列表，逗号分隔"))
	cmd.Flags().String("json", "", i18n.T("可见字段 ID JSON 字符串数组"))
	return cmd
}

func newAitableViewUpdateFilterCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewUpdateArrayCommand(runner, "filter", "filter")
}

func newAitableViewUpdateSortCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewUpdateArrayCommand(runner, "sort", "sort")
}

func newAitableViewUpdateGroupCommand(runner executor.Runner) *cobra.Command {
	return newAitableViewUpdateArrayCommand(runner, "group", "group")
}

func newAitableViewUpdateArrayCommand(runner executor.Runner, use, blockKey string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             i18n.T("更新视图 " + use + " 配置"),
		Example:           fmt.Sprintf("  dws aitable view update %s --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --json '[]'", use),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			raw, err := aitableRequiredFlag(cmd, "json")
			if err != nil {
				return err
			}
			parsed, err := parseAitableJSONValue(raw, "json")
			if err != nil {
				return err
			}
			cfg := map[string]any{blockKey: parsed}
			if err := normalizeAitableViewConfigBlock(cmd, cfg); err != nil {
				return err
			}
			return runAitableViewUpdateBlock(cmd, runner, baseID, tableID, viewID, blockKey, cfg[blockKey], nil)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("json", "", i18n.T(use+" JSON 数组 (必填)"))
	return cmd
}

func newAitableViewUpdateNameCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "name",
		Short:             i18n.T("重命名视图"),
		Example:           `  dws aitable view update name --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --name 新视图名`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			name, err := aitableRequiredFlag(cmd, "name")
			if err != nil {
				return err
			}
			return runAitableViewUpdateBlock(cmd, runner, baseID, tableID, viewID, "", nil, map[string]any{"newViewName": name})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("name", "", i18n.T("新视图名称 (必填)"))
	return cmd
}

func aitableViewUpdatePreflight(cmd *cobra.Command, runner executor.Runner, blockKey string, allowedViewTypes []string, dynamicCard bool) (string, string, string, string, error) {
	baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
	if err != nil {
		return "", "", "", "", err
	}
	if commandDryRun(cmd) {
		if dynamicCard {
			return baseID, tableID, viewID, "kanbanCard", nil
		}
		return baseID, tableID, viewID, blockKey, nil
	}
	if !dynamicCard && len(allowedViewTypes) == 0 {
		return baseID, tableID, viewID, blockKey, nil
	}
	result, err := runAitableProductToolResult(cmd, runner, "aitable", "get_views", map[string]any{
		"baseId":  baseID,
		"tableId": tableID,
		"viewIds": []string{viewID},
	})
	if err != nil {
		return "", "", "", "", err
	}
	_, viewType, err := aitableViewFromResult(result, viewID)
	if err != nil {
		return "", "", "", "", err
	}
	if dynamicCard {
		dispatched, err := aitableDispatchCardKey(viewType)
		return baseID, tableID, viewID, dispatched, err
	}
	if err := aitableRequireViewType(viewType, blockKey, allowedViewTypes); err != nil {
		return "", "", "", "", err
	}
	return baseID, tableID, viewID, blockKey, nil
}

func runAitableViewUpdateBlock(cmd *cobra.Command, runner executor.Runner, baseID, tableID, viewID, blockKey string, block any, extra map[string]any) error {
	params := map[string]any{
		"baseId":  baseID,
		"tableId": tableID,
		"viewId":  viewID,
	}
	for k, v := range extra {
		params[k] = v
	}
	if blockKey != "" {
		params["config"] = map[string]any{blockKey: block}
	}
	return runAitableTool(cmd, runner, "update_view", params)
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
			result, err := runAitableProductToolResult(cmd, runner, "aitable", "get_views", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewIds": []string{viewID},
			})
			if err != nil {
				return err
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, result)
			}
			view, _, err := aitableViewFromResult(result, viewID)
			if err != nil {
				return err
			}
			data := aitableWalkViewPath(view, "conditionalFormats")
			if data == nil {
				data = []any{}
			}
			return writeCommandPayload(cmd, aitableProjectionEnvelope(result, data))
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

func parseAitableJSONValue(raw, flagName string) (any, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--%s JSON parse failed: %v", flagName, err))
	}
	return value, nil
}

func aitableMergeJSONObjectFlag(cmd *cobra.Command, typed map[string]any) (map[string]any, error) {
	block := map[string]any{}
	if raw := aitableStringFlag(cmd, "json"); raw != "" {
		parsed, err := parseAitableJSONObject(raw, "json")
		if err != nil {
			return nil, err
		}
		for k, v := range parsed {
			block[k] = v
		}
	}
	for k, v := range typed {
		block[k] = v
	}
	if len(block) == 0 {
		return nil, apperrors.NewValidation("must specify --json or at least one typed flag")
	}
	return block, nil
}

func aitableCollectStringFlag(cmd *cobra.Command, flagName, key string, dst map[string]any) {
	if value := aitableStringFlag(cmd, flagName); value != "" {
		dst[key] = value
	}
}

func aitableCollectBoolFlag(cmd *cobra.Command, flagName, key string, dst map[string]any) {
	if cmd.Flags().Changed(flagName) {
		value, _ := cmd.Flags().GetBool(flagName)
		dst[key] = value
	}
}

var aitableKnownViewConfigKeys = map[string]bool{
	"visibleFieldIds": true,
	"filter":          true,
	"sort":            true,
	"group":           true,
	"fieldWidths":     true,
	"aggregate":       true,
	"kanbanCard":      true,
	"ganttTimebar":    true,
	"galleryCard":     true,
}

var aitableRoutedViewConfigKeys = map[string]string{
	"flags":              "dws aitable view lock [--off]",
	"frozenColCount":     "dws aitable view update frozen-cols --count N",
	"cellHeight":         "dws aitable view update row-height --cell-height N",
	"rowHeightLevel":     "dws aitable view update row-height --cell-height N",
	"conditionalFormats": "dws aitable view update fill-color-rule --json '[...]'",
}

func normalizeAitableViewConfigBlock(cmd *cobra.Command, cfg map[string]any) error {
	var routed []string
	var unknown []string
	for key := range cfg {
		if aitableKnownViewConfigKeys[key] {
			continue
		}
		if _, ok := aitableRoutedViewConfigKeys[key]; ok {
			routed = append(routed, key)
			continue
		}
		unknown = append(unknown, key)
	}
	if len(routed) > 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "以下 key 不能通过 view update --config 修改，请改用对应子命令：")
		for _, key := range routed {
			fmt.Fprintf(cmd.ErrOrStderr(), "- %s -> %s\n", key, aitableRoutedViewConfigKeys[key])
		}
	}
	if len(unknown) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: config 中包含可能不被支持的 key: %v\n", unknown)
	}
	if value, ok := cfg["filter"]; ok && value != nil {
		switch typed := value.(type) {
		case []any:
			for i, item := range typed {
				typed[i] = normalizeAitableFilters(item)
			}
			cfg["filter"] = typed
		case map[string]any:
			cfg["filter"] = []any{normalizeAitableFilters(typed)}
		default:
			return apperrors.NewValidation(fmt.Sprintf("invalid config.filter: must be a JSON array or object, got %T", value))
		}
	}
	for _, key := range []string{"sort", "group"} {
		value, ok := cfg[key]
		if !ok || value == nil {
			continue
		}
		switch value.(type) {
		case []any:
		case map[string]any:
			cfg[key] = []any{value}
		default:
			return apperrors.NewValidation(fmt.Sprintf("invalid config.%s: must be a JSON array, got %T", key, value))
		}
	}
	return nil
}

func aitableViewFromResult(result executor.Result, viewID string) (map[string]any, string, error) {
	content := aitableResultContent(result.Response)
	data := content
	if nested, ok := content["data"].(map[string]any); ok {
		data = nested
	}
	views := aitableAnySlice(data["views"])
	if len(views) == 0 {
		views = aitableAnySlice(content["views"])
	}
	for _, item := range views {
		view, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if viewID == "" || aitableMapString(view, "viewId") == viewID {
			return view, aitableMapString(view, "viewType"), nil
		}
	}
	return nil, "", apperrors.NewValidation(fmt.Sprintf("view %q not found in get_views response", viewID))
}

func aitableResultContent(resp map[string]any) map[string]any {
	if resp == nil {
		return map[string]any{}
	}
	if content, ok := resp["content"].(map[string]any); ok {
		return content
	}
	return resp
}

func aitableProjectionEnvelope(result executor.Result, data any) map[string]any {
	content := aitableResultContent(result.Response)
	out := make(map[string]any, len(content)+2)
	for k, v := range content {
		out[k] = v
	}
	if _, ok := out["status"]; !ok {
		out["status"] = "success"
	}
	if _, ok := out["success"]; !ok {
		out["success"] = true
	}
	out["data"] = data
	return out
}

func aitableDispatchCardKey(viewType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(viewType)) {
	case "kanban":
		return "kanbanCard", nil
	case "gallery":
		return "galleryCard", nil
	default:
		return "", apperrors.NewValidation(fmt.Sprintf("viewType %q does not support card; expected Kanban or Gallery", viewType))
	}
}

func aitableRequireViewType(viewType, attr string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	for _, want := range allowed {
		if strings.EqualFold(viewType, want) {
			return nil
		}
	}
	return apperrors.NewValidation(fmt.Sprintf("viewType %q does not support %s; expected %s", viewType, attr, strings.Join(allowed, "/")))
}

func aitableWalkViewPath(view map[string]any, path string) any {
	if path == "" {
		return nil
	}
	var cur any = view
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
		if cur == nil {
			return nil
		}
	}
	return cur
}

func aitableDefaultViewProjection(use string) any {
	switch use {
	case "filter":
		return map[string]any{"operator": "and", "operands": []any{}}
	case "sort", "group", "visible-fields", "fill-color-rule":
		return []any{}
	default:
		return map[string]any{}
	}
}

func aitableAnySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func aitableMapString(m map[string]any, key string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return ""
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
