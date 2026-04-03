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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/spf13/cobra"
)

// ── base ────────────────────────────────────────────────────

func newAitableBaseListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             i18n.T("获取 AI 表格列表"),
		Example:           "  dws aitable base list\n  dws aitable base list --limit 5 --cursor NEXT_CURSOR",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
				params["limit"] = limit
			}
			if cursor := aitableStringFlag(cmd, "cursor"); cursor != "" {
				params["cursor"] = cursor
			}
			return runAitableTool(cmd, runner, "list_bases", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().Int("limit", 0, i18n.T("每页数量"))
	cmd.Flags().String("cursor", "", i18n.T("分页游标"))
	return cmd
}

func newAitableBaseSearchCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "search",
		Short:             i18n.T("搜索 AI 表格"),
		Example:           "  dws aitable base search --query 项目管理",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			query := aitableFlagOrFallback(cmd, "query", "keyword")
			if query == "" {
				return apperrors.NewValidation("--query is required")
			}
			params := map[string]any{"query": query}
			if cursor := aitableStringFlag(cmd, "cursor"); cursor != "" {
				params["cursor"] = cursor
			}
			return runAitableTool(cmd, runner, "search_bases", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("query", "", i18n.T("Base 名称关键词 (必填)"))
	cmd.Flags().String("keyword", "", i18n.T("--query 的别名"))
	_ = cmd.Flags().MarkHidden("keyword")
	cmd.Flags().String("cursor", "", i18n.T("分页游标"))
	return cmd
}

func newAitableBaseGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("获取 AI 表格信息"),
		Example:           "  dws aitable base get --base-id BASE_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "get_base", map[string]any{
				"baseId": baseID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	return cmd
}

func newAitableBaseCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("创建 AI 表格"),
		Example:           "  dws aitable base create --name 项目跟踪",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := aitableRequiredFlag(cmd, "name")
			if err != nil {
				return err
			}
			params := map[string]any{"baseName": name}
			if templateID := aitableStringFlag(cmd, "template-id"); templateID != "" {
				params["templateId"] = templateID
			}
			return runAitableTool(cmd, runner, "create_base", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("name", "", i18n.T("Base 名称 (必填)"))
	cmd.Flags().String("template-id", "", i18n.T("模板 ID"))
	return cmd
}

func newAitableBaseUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新 AI 表格"),
		Example:           "  dws aitable base update --base-id BASE_ID --name 新名称",
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
				"baseId":      baseID,
				"newBaseName": name,
			}
			if desc := aitableStringFlag(cmd, "desc"); desc != "" {
				params["description"] = desc
			}
			return runAitableTool(cmd, runner, "update_base", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("name", "", i18n.T("新名称 (必填)"))
	cmd.Flags().String("desc", "", i18n.T("备注文本"))
	return cmd
}

// ── table ───────────────────────────────────────────────────

func newAitableTableGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("获取数据表"),
		Example:           "  dws aitable table get --base-id BASE_ID\n  dws aitable table get --base-id BASE_ID --table-ids tbl1,tbl2",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			params := map[string]any{"baseId": baseID}
			if tableIDs := aitableStringFlag(cmd, "table-ids"); tableIDs != "" {
				params["tableIds"] = parseAitableCSVValues(tableIDs)
			}
			return runAitableTool(cmd, runner, "get_tables", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-ids", "", i18n.T("Table ID 列表，逗号分隔"))
	return cmd
}

func newAitableTableCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("创建数据表"),
		Example:           "  dws aitable table create --base-id BASE_ID --name 任务表 --fields '[{\"fieldName\":\"名称\",\"type\":\"text\"}]'",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			tableName := aitableFlagOrFallback(cmd, "name", "table-name")
			if tableName == "" {
				return apperrors.NewValidation("--name is required")
			}
			fieldsRaw, err := aitableRequiredFlag(cmd, "fields")
			if err != nil {
				return err
			}
			fields, err := parseAitableFieldsJSON(fieldsRaw)
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "create_table", map[string]any{
				"baseId":    baseID,
				"tableName": tableName,
				"fields":    fields,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("name", "", i18n.T("表格名称 (必填)"))
	cmd.Flags().String("table-name", "", i18n.T("--name 的别名"))
	_ = cmd.Flags().MarkHidden("table-name")
	cmd.Flags().String("fields", "", i18n.T("字段 JSON 数组 (必填)"))
	return cmd
}

func newAitableTableUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新数据表"),
		Example:           "  dws aitable table update --base-id BASE_ID --table-id TABLE_ID --name 新表名",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			tableID, err := aitableRequiredFlag(cmd, "table-id")
			if err != nil {
				return err
			}
			name, err := aitableRequiredFlag(cmd, "name")
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "update_table", map[string]any{
				"baseId":       baseID,
				"tableId":      tableID,
				"newTableName": name,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("name", "", i18n.T("新表名 (必填)"))
	return cmd
}

// ── field ───────────────────────────────────────────────────

func newAitableFieldGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("获取字段详情"),
		Example:           "  dws aitable field get --base-id BASE_ID --table-id TABLE_ID\n  dws aitable field get --base-id BASE_ID --table-id TABLE_ID --field-ids fld1,fld2",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			tableID, err := aitableRequiredFlag(cmd, "table-id")
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
			}
			if fieldIDs := aitableStringFlag(cmd, "field-ids"); fieldIDs != "" {
				params["fieldIds"] = parseAitableCSVValues(fieldIDs)
			}
			return runAitableTool(cmd, runner, "get_fields", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("field-ids", "", i18n.T("Field ID 列表，逗号分隔"))
	return cmd
}

func newAitableFieldCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("创建字段"),
		Example:           "  dws aitable field create --base-id BASE_ID --table-id TABLE_ID --fields '[{\"fieldName\":\"状态\",\"type\":\"singleSelect\"}]'",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			tableID, err := aitableRequiredFlag(cmd, "table-id")
			if err != nil {
				return err
			}

			var fields []any
			fieldsRaw := aitableStringFlag(cmd, "fields")
			if fieldsRaw != "" {
				fields, err = parseAitableFieldsJSON(fieldsRaw)
				if err != nil {
					return err
				}
			} else {
				name, nameErr := aitableRequiredFlag(cmd, "name")
				if nameErr != nil {
					return apperrors.NewValidation("must specify either --fields or both --name and --type")
				}
				fieldType, typeErr := aitableRequiredFlag(cmd, "type")
				if typeErr != nil {
					return apperrors.NewValidation("must specify either --fields or both --name and --type")
				}
				field := map[string]any{
					"fieldName": name,
					"type":      fieldType,
				}
				if configRaw := aitableStringFlag(cmd, "config"); configRaw != "" {
					configValue, err := parseAitableJSONObject(configRaw, "config")
					if err != nil {
						return err
					}
					field["config"] = configValue
				}
				fields = []any{field}
			}

			return runAitableTool(cmd, runner, "create_fields", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"fields":  fields,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("fields", "", i18n.T("字段 JSON 数组"))
	cmd.Flags().String("name", "", i18n.T("单字段名称"))
	cmd.Flags().String("type", "", i18n.T("单字段类型"))
	cmd.Flags().String("config", "", i18n.T("字段配置 JSON"))
	return cmd
}

func newAitableFieldUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新字段"),
		Example:           "  dws aitable field update --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --name 新字段名",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			tableID, err := aitableRequiredFlag(cmd, "table-id")
			if err != nil {
				return err
			}
			fieldID, err := aitableRequiredFlag(cmd, "field-id")
			if err != nil {
				return err
			}
			name := aitableStringFlag(cmd, "name")
			configRaw := aitableStringFlag(cmd, "config")
			if name == "" && configRaw == "" {
				return apperrors.NewValidation("at least one of --name or --config is required")
			}

			params := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"fieldId": fieldID,
			}
			if name != "" {
				params["newFieldName"] = name
			}
			if configRaw != "" {
				configValue, err := parseAitableJSONObject(configRaw, "config")
				if err != nil {
					return err
				}
				params["config"] = configValue
			}
			return runAitableTool(cmd, runner, "update_field", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("field-id", "", i18n.T("Field ID (必填)"))
	cmd.Flags().String("name", "", i18n.T("新字段名"))
	cmd.Flags().String("config", "", i18n.T("字段配置 JSON"))
	return cmd
}

// ── record ──────────────────────────────────────────────────

func newAitableRecordQueryCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "query",
		Short:             i18n.T("查询记录"),
		Example:           "  dws aitable record query --base-id BASE_ID --table-id TABLE_ID --keyword 关键词 --limit 50",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			tableID, err := aitableRequiredFlag(cmd, "table-id")
			if err != nil {
				return err
			}

			params := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
			}
			if recordIDs := aitableStringFlag(cmd, "record-ids"); recordIDs != "" {
				params["recordIds"] = parseAitableCSVValues(recordIDs)
			}
			if fieldIDs := aitableStringFlag(cmd, "field-ids"); fieldIDs != "" {
				params["fieldIds"] = parseAitableCSVValues(fieldIDs)
			}
			if filtersRaw := aitableStringFlag(cmd, "filters"); filtersRaw != "" {
				filters, err := parseAitableJSONObject(filtersRaw, "filters")
				if err != nil {
					return err
				}
				params["filters"] = filters
			}
			if sortRaw := aitableStringFlag(cmd, "sort"); sortRaw != "" {
				sortValue, err := parseAitableJSONArray(sortRaw, "sort")
				if err != nil {
					return err
				}
				params["sort"] = sortValue
			}
			if keyword := aitableFlagOrFallback(cmd, "query", "keyword"); keyword != "" {
				params["keyword"] = keyword
			}
			if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
				params["limit"] = limit
			}
			if cursor := aitableStringFlag(cmd, "cursor"); cursor != "" {
				params["cursor"] = cursor
			}
			return runAitableTool(cmd, runner, "query_records", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("record-ids", "", i18n.T("Record ID 列表，逗号分隔"))
	cmd.Flags().String("field-ids", "", i18n.T("Field ID 列表，逗号分隔"))
	cmd.Flags().String("filters", "", i18n.T("过滤条件 JSON"))
	cmd.Flags().String("sort", "", i18n.T("排序 JSON 数组"))
	cmd.Flags().String("query", "", i18n.T("全文关键词"))
	cmd.Flags().String("keyword", "", i18n.T("--query 的别名"))
	_ = cmd.Flags().MarkHidden("keyword")
	cmd.Flags().Int("limit", 0, i18n.T("单次最大记录数"))
	cmd.Flags().String("cursor", "", i18n.T("分页游标"))
	return cmd
}

func newAitableRecordCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("新增记录"),
		Example:           "  dws aitable record create --base-id BASE_ID --table-id TABLE_ID --records '[{\"cells\":{\"fld1\":\"hello\"}}]'",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			tableID, err := aitableRequiredFlag(cmd, "table-id")
			if err != nil {
				return err
			}
			recordsRaw, err := aitableRequiredFlag(cmd, "records")
			if err != nil {
				return err
			}
			records, err := parseAitableJSONArray(recordsRaw, "records")
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "create_records", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"records": records,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("records", "", i18n.T("记录 JSON 数组 (必填)"))
	return cmd
}

func newAitableRecordUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新记录"),
		Example:           "  dws aitable record update --base-id BASE_ID --table-id TABLE_ID --records '[{\"recordId\":\"rec1\",\"cells\":{\"fld1\":\"updated\"}}]'",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			tableID, err := aitableRequiredFlag(cmd, "table-id")
			if err != nil {
				return err
			}
			recordsRaw, err := aitableRequiredFlag(cmd, "records")
			if err != nil {
				return err
			}
			records, err := parseAitableJSONArray(recordsRaw, "records")
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "update_records", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"records": records,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("records", "", i18n.T("记录 JSON 数组 (必填)"))
	return cmd
}

// ── template ────────────────────────────────────────────────

func newAitableTemplateSearchCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "search",
		Short:             i18n.T("搜索模板"),
		Example:           "  dws aitable template search --query 项目管理",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			query := aitableFlagOrFallback(cmd, "query", "keyword")
			if query == "" {
				return apperrors.NewValidation("--query is required")
			}
			params := map[string]any{"query": query}
			if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
				params["limit"] = limit
			}
			if cursor := aitableStringFlag(cmd, "cursor"); cursor != "" {
				params["cursor"] = cursor
			}
			return runAitableTool(cmd, runner, "search_templates", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("query", "", i18n.T("模板关键词 (必填)"))
	cmd.Flags().String("keyword", "", i18n.T("--query 的别名"))
	_ = cmd.Flags().MarkHidden("keyword")
	cmd.Flags().Int("limit", 0, i18n.T("每页数量"))
	cmd.Flags().String("cursor", "", i18n.T("分页游标"))
	return cmd
}

// ── attachment ──────────────────────────────────────────────

func newAITableAttachmentUploadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "upload",
		Short:             i18n.T("准备附件上传"),
		Example:           "  dws aitable attachment upload --base-id BASE_ID --file-name report.pdf --size 1024",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlag(cmd, "base-id")
			if err != nil {
				return err
			}
			fileName, err := aitableRequiredFlag(cmd, "file-name")
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId":   baseID,
				"fileName": fileName,
			}
			if size, _ := cmd.Flags().GetInt64("size"); size > 0 {
				params["size"] = size
			}
			if mimeType := aitableStringFlag(cmd, "mime-type"); mimeType != "" {
				params["mimeType"] = mimeType
			}
			return runAitableTool(cmd, runner, "prepare_attachment_upload", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("file-name", "", i18n.T("文件名 (必填)"))
	cmd.Flags().Int64("size", 0, i18n.T("文件大小（字节）"))
	cmd.Flags().String("mime-type", "", i18n.T("文件 MIME Type"))
	return cmd
}

// ── helpers ────────────────────────────────────────────────

func runAitableTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	invocation := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd),
		"aitable",
		tool,
		params,
	)
	invocation.DryRun = commandDryRun(cmd)
	result, err := runner.Run(cmd.Context(), invocation)
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

func aitableStringFlag(cmd *cobra.Command, name string) string {
	if cmd == nil {
		return ""
	}
	if value, err := cmd.Flags().GetString(name); err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, err := cmd.InheritedFlags().GetString(name); err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}

func aitableFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) string {
	if value := aitableStringFlag(cmd, primary); value != "" {
		return value
	}
	for _, alias := range aliases {
		if value := aitableStringFlag(cmd, alias); value != "" {
			return value
		}
	}
	return ""
}

func aitableRequiredFlag(cmd *cobra.Command, name string) (string, error) {
	if value := aitableStringFlag(cmd, name); value != "" {
		return value, nil
	}
	return "", apperrors.NewValidation(fmt.Sprintf("--%s is required", name))
}

func aitableRequiredFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) (string, error) {
	if value := aitableFlagOrFallback(cmd, primary, aliases...); value != "" {
		return value, nil
	}
	return "", apperrors.NewValidation(fmt.Sprintf("--%s is required", primary))
}

func parseAitableCSVValues(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func parseAitableFieldsJSON(raw string) ([]any, error) {
	var fields []any
	if err := json.Unmarshal([]byte(raw), &fields); err == nil {
		return fields, nil
	}
	var wrapper map[string]any
	if err := json.Unmarshal([]byte(raw), &wrapper); err == nil {
		if wrappedFields, ok := wrapper["fields"].([]any); ok {
			return wrappedFields, nil
		}
	}
	return nil, apperrors.NewValidation("--fields JSON parse failed: expect a JSON array")
}

func parseAitableJSONArray(raw, flagName string) ([]any, error) {
	var value []any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--%s JSON parse failed: %v", flagName, err))
	}
	return value, nil
}

func parseAitableJSONObject(raw, flagName string) (map[string]any, error) {
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--%s JSON parse failed: %v", flagName, err))
	}
	return value, nil
}
