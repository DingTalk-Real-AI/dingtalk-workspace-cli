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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/paging"
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
				params := map[string]any{}
				if cursor := aitableStringFlag(cmd, "cursor"); cursor != "" {
					params["cursor"] = cursor
				}
				return runAitableTool(cmd, runner, "list_bases", params)
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
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base", "id")
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
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	addAitableHiddenStringFlag(cmd, "id", "--base-id 的兼容别名")
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
			if folderID := aitableStringFlag(cmd, "folder-id"); folderID != "" {
				params["folderId"] = folderID
			}
			return runAitableTool(cmd, runner, "create_base", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("name", "", i18n.T("Base 名称 (必填)"))
	cmd.Flags().String("template-id", "", i18n.T("模板 ID"))
	cmd.Flags().String("folder-id", "", i18n.T("父文件夹 ID"))
	return cmd
}

func newAitableBaseCopyCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "copy",
		Short: i18n.T("复制 AI 表格"),
		Long: i18n.T(`复制 AI 表格到指定目录。支持完整复制或仅复制结构（不含数据）。
复制操作会创建一个新的 Base，包含原 Base 的表、字段、视图等配置。
如果选择仅复制结构（--only-struct），则不会复制实际的记录数据。

权限要求：需要对源 Base 有"阅读"权限，且对目标文件夹有"编辑"权限。

注意：--target-folder-id 参数如果传入的是文档/文件夹 URL（如 https://alidocs.dingtalk.com/i/nodes/xxx），
需要先调用文档的 dws 命令（如 dws doc info --node URL）获取 dentryUuid，再将 dentryUuid 传入本命令。
MCP 层不会会自动解析 URL，必须直接传入 dentryUuid 以避免报错。`),
		Example: `  dws aitable base copy --base-id BASE_ID --target-folder-id FOLDER_ID
  dws aitable base copy --base-id BASE_ID --target-folder-id FOLDER_ID --only-struct
  # 查询 baseId: dws aitable base list
  # 查询 folderId: dws doc list --folder PARENT_FOLDER_ID`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlag(cmd, "base-id")
			if err != nil {
				return err
			}
			targetFolderID, err := aitableRequiredFlag(cmd, "target-folder-id")
			if err != nil {
				return err
			}
			onlyCopyMeta, err := cmd.Flags().GetBool("only-struct")
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "copy_base", map[string]any{
				"baseId":         baseID,
				"targetFolderId": targetFolderID,
				"onlyCopyMeta":   onlyCopyMeta,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("源 Base ID (必填)"))
	cmd.Flags().String("target-folder-id", "", i18n.T("目标文件夹 ID (必填)"))
	cmd.Flags().Bool("only-struct", false, i18n.T("是否仅复制结构（不含数据），默认 false 表示完整复制"))
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
		Aliases:           []string{"list"},
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
			fieldsRaw := aitableStringFlag(cmd, "fields")
			if strings.TrimSpace(fieldsRaw) == "" {
				fieldsRaw = "[]"
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
	cmd.Flags().String("fields", "[]", i18n.T("字段 JSON 数组"))
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
		Aliases:           []string{"list"},
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
				name := aitableFlagOrFallback(cmd, "name", "field-name")
				fieldType := aitableFlagOrFallback(cmd, "type", "field-type")
				if name == "" || fieldType == "" {
					return apperrors.NewValidation("must specify either --fields or both --name and --type")
				}
				field := map[string]any{
					"fieldName": name,
					"type":      normalizeAitableFieldType(fieldType),
				}
				if configRaw := aitableStringFlag(cmd, "config"); configRaw != "" {
					configValue, err := parseAitableJSONObject(configRaw, "config")
					if err != nil {
						return err
					}
					field["config"] = configValue
				}
				if aiConfigRaw := aitableStringFlag(cmd, "ai-config"); aiConfigRaw != "" {
					aiConfigValue, err := parseAitableJSONObject(aiConfigRaw, "ai-config")
					if err != nil {
						return err
					}
					field["aiConfig"] = aiConfigValue
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
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("fields", "", i18n.T("字段 JSON 数组"))
	cmd.Flags().String("name", "", i18n.T("单字段名称"))
	addAitableHiddenStringFlag(cmd, "field-name", "--name 的兼容别名")
	cmd.Flags().String("type", "", i18n.T("单字段类型"))
	addAitableHiddenStringFlag(cmd, "field-type", "--type 的兼容别名")
	cmd.Flags().String("config", "", i18n.T("字段配置 JSON"))
	cmd.Flags().String("ai-config", "", i18n.T("AI 字段配置 JSON"))
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
			aiConfigRaw := aitableStringFlag(cmd, "ai-config")
			if name == "" && configRaw == "" && aiConfigRaw == "" {
				return apperrors.NewValidation("at least one of --name, --config or --ai-config is required")
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
			if aiConfigRaw != "" {
				aiConfigValue, err := parseAitableJSONObject(aiConfigRaw, "ai-config")
				if err != nil {
					return err
				}
				params["aiConfig"] = aiConfigValue
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
	cmd.Flags().String("ai-config", "", i18n.T("AI 字段配置 JSON"))
	return cmd
}

// ── record ──────────────────────────────────────────────────

func newAitableRecordQueryCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "query",
		Aliases: []string{"list"},
		Short:   i18n.T("查询记录"),
		Long: i18n.T(`查询 AI 表格记录。

默认行为：返回单页数据（受 --limit 控制，未传时由服务端默认）。
传入 --all 时自动翻页累计全部数据，受 --page-limit 安全阀控制（默认 50 页）。

翻页输出契约（仅 --all 时）：
  - records  - 累计所有页的记录
  - hasMore  - true 表示触发 page-limit 或中途出错，需用 --cursor <X> 续拉
  - cursor   - 续拉用的下一页 cursor（hasMore=true 时有效）
  - partial  - true 表示中途某页失败，records 是失败前已累计的数据
  - pages    - 实际翻了多少页`),
		Example: `  dws aitable record query --base-id B --table-id T --limit 50
  dws aitable record query --base-id B --table-id T --keyword 关键词
  dws aitable record query --base-id B --table-id T --all
  dws aitable record query --base-id B --table-id T --all --page-limit 0    # 不限制
  dws aitable record query --base-id B --table-id T --all --cursor <SAVED>  # 接续上次断点`,
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

			// 构建单页查询参数
			baseParams := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
			}
			if recordIDs := aitableStringFlag(cmd, "record-ids"); recordIDs != "" {
				baseParams["recordIds"] = parseAitableCSVValues(recordIDs)
			}
			if fieldIDs := aitableStringFlag(cmd, "field-ids"); fieldIDs != "" {
				baseParams["fieldIds"] = parseAitableCSVValues(fieldIDs)
			}
			if filtersRaw := aitableStringFlag(cmd, "filters"); filtersRaw != "" {
				filters, err := parseAitableJSONObject(filtersRaw, "filters")
				if err != nil {
					return err
				}
				if err := validateAitableFiltersStructure(filters); err != nil {
					return err
				}
				baseParams["filters"] = normalizeAitableFilters(filters)
			}
			if sortRaw := aitableStringFlag(cmd, "sort"); sortRaw != "" {
				sortValue, err := parseAitableJSONArray(sortRaw, "sort")
				if err != nil {
					return err
				}
				baseParams["sort"] = normalizeAitableSort(sortValue)
			}
			if keyword := aitableFlagOrFallback(cmd, "query", "keyword"); keyword != "" {
				baseParams["keyword"] = keyword
			}
			if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
				baseParams["limit"] = limit
			} else if pageSize, _ := cmd.Flags().GetInt("page-size"); pageSize > 0 {
				baseParams["limit"] = pageSize
			}

			fetchAll, _ := cmd.Flags().GetBool("all")
			initialCursor := aitableFlagOrFallback(cmd, "cursor", "page-token")

			// 非 --all 模式：保持原行为（单页查询）
			if !fetchAll {
				params := cloneMap(baseParams)
				if initialCursor != "" {
					params["cursor"] = initialCursor
				}
				return runAitableTool(cmd, runner, "query_records", params)
			}

			// --all 模式：用 pkg/paging 循环
			return runRecordQueryAll(cmd, runner, baseParams, initialCursor)
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
	cmd.Flags().Int("page-size", 0, i18n.T("--limit 的兼容别名"))
	_ = cmd.Flags().MarkHidden("page-size")
	cmd.Flags().String("cursor", "", i18n.T("分页游标"))
	cmd.Flags().String("page-token", "", i18n.T("--cursor 的兼容别名"))
	_ = cmd.Flags().MarkHidden("page-token")
	cmd.Flags().Bool("all", false, i18n.T("自动翻页获取全部记录（受 --page-limit 上限）"))
	cmd.Flags().Int("page-limit", 50, i18n.T("自动翻页最大页数（默认 50 ≈ 5000 条），0 表示无限制"))
	return cmd
}

// cloneMap 浅拷贝 map[string]any，避免在循环中共享底层 map 导致 cursor 覆写问题。
func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func newAitableRecordGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("按 Record ID 获取记录"),
		Example:           "  dws aitable record get --base-id BASE_ID --table-id TABLE_ID --record-ids rec1,rec2",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlag(cmd, "base-id")
			if err != nil {
				return err
			}
			tableID, err := aitableRequiredFlag(cmd, "table-id")
			if err != nil {
				return err
			}
			recordIDsRaw, err := aitableRequiredFlag(cmd, "record-ids")
			if err != nil {
				return err
			}
			recordIDs := parseAitableCSVValues(recordIDsRaw)
			if len(recordIDs) == 0 {
				return apperrors.NewValidation("--record-ids is required")
			}

			params := map[string]any{
				"baseId":    baseID,
				"tableId":   tableID,
				"recordIds": recordIDs,
			}
			if fieldIDs := aitableStringFlag(cmd, "field-ids"); fieldIDs != "" {
				params["fieldIds"] = parseAitableCSVValues(fieldIDs)
			}
			return runAitableTool(cmd, runner, "query_records", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("record-ids", "", i18n.T("Record ID 列表，逗号分隔 (必填)"))
	cmd.Flags().String("field-ids", "", i18n.T("Field ID 列表，逗号分隔"))
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
			records, err := resolveAitableRecordCreateRecords(cmd)
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
	cmd.Flags().String("records-file", "", i18n.T("从文件读取 records JSON"))
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	addAitableHiddenStringFlag(cmd, "fields", "--records 的兼容别名")
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
			records, err := resolveAitableRecordUpdateRecords(cmd)
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
	cmd.Flags().String("records-file", "", i18n.T("从文件读取 records JSON"))
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	addAitableHiddenStringFlag(cmd, "fields", "--records 的兼容别名")
	addAitableHiddenStringFlag(cmd, "record-id", "--records 的兼容输入：单条记录 ID")
	addAitableHiddenStringFlag(cmd, "cells", "--records 的兼容输入：单条记录 cells JSON 对象")
	addAitableHiddenStringFlag(cmd, "data", "--records 的兼容输入：records 数组、单条 record 对象或 cells 对象")
	return cmd
}

func newAitableRecordBatchUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "batch-update",
		Hidden: true,
		Short:  i18n.T("批量更新记录（同一 cells 应用到多条 recordId）"),
		Long: i18n.T(`将同一份 cells JSON 批量应用到多条记录上。

与 record update 的区别：
  - record update         每条记录可有不同 cells，参数是 records JSON 数组
  - record batch-update   所有记录共享同一份 cells，CLI 展开后调用 update_records

单次最多 100 条；超出请拆分多次调用。`),
		Example:           `  dws aitable record batch-update --base-id BASE_ID --table-id TABLE_ID --record-ids rec1,rec2 --cells '{"fldStatusId":"已完成"}'`,
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
			recordIDsRaw, err := aitableRequiredFlag(cmd, "record-ids")
			if err != nil {
				return err
			}
			recordIDs := parseAitableCSVValues(recordIDsRaw)
			if len(recordIDs) == 0 {
				return apperrors.NewValidation("--record-ids must not be empty")
			}
			if len(recordIDs) > 100 {
				return apperrors.NewValidation(fmt.Sprintf("--record-ids exceeds limit: got %d, max 100", len(recordIDs)))
			}
			cellsRaw, err := aitableRequiredFlag(cmd, "cells")
			if err != nil {
				return err
			}
			cells, err := parseAitableJSONObject(cellsRaw, "cells")
			if err != nil {
				return err
			}
			if len(cells) == 0 {
				return apperrors.NewValidation("--cells must not be empty")
			}
			records := make([]any, 0, len(recordIDs))
			for _, recordID := range recordIDs {
				records = append(records, map[string]any{
					"recordId": recordID,
					"cells":    cells,
				})
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
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("record-ids", "", i18n.T("Record ID 列表，逗号分隔 (必填)"))
	cmd.Flags().String("cells", "", i18n.T("统一应用的 cells JSON 对象 (必填)"))
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
		Use:   "upload",
		Short: i18n.T("准备附件上传 (仅返回 uploadUrl + fileToken, 不上传文件)"),
		Long: i18n.T(`准备附件上传 — 3 步流程的第 1 步.

本命令只调用 prepare_attachment_upload, 返回 OSS 上传地址 uploadUrl 和 fileToken,
不实际上传文件二进制. 拿到响应后你还需要:
  2. HTTP PUT 文件二进制 → uploadUrl
  3. 把 fileToken 填到 record 的 attachment 字段, 格式 [{"fileToken":"ft_xxx"}]`),
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
			if !cmd.Flags().Changed("size") {
				return apperrors.NewValidation("--size is required")
			}
			size, _ := cmd.Flags().GetInt64("size")
			if size <= 0 {
				return apperrors.NewValidation("--size must be greater than 0")
			}
			params := map[string]any{
				"baseId":   baseID,
				"fileName": fileName,
				"size":     size,
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

// ── dashboard / chart ───────────────────────────────────────

func newAitableDashboardGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("获取仪表盘详情"),
		Example:           "  dws aitable dashboard get --base-id BASE_ID --dashboard-id DASHBOARD_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			dashboardID, err := aitableRequiredFlag(cmd, "dashboard-id")
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "get_dashboard", map[string]any{
				"baseId":      baseID,
				"dashboardId": dashboardID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableDashboardIDFlags(cmd)
	return cmd
}

func newAitableDashboardCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: i18n.T("创建仪表盘"),
		Example: `  dws aitable dashboard create --base-id BASE_ID --config '{"dashboardName":"销售看板"}'
  dws aitable dashboard create --base-id BASE_ID --name 销售看板`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			config := map[string]any{}
			if configRaw := aitableStringFlag(cmd, "config"); configRaw != "" {
				parsed, err := parseAitableJSONObject(configRaw, "config")
				if err != nil {
					return err
				}
				config = parsed
			}
			if name := aitableStringFlag(cmd, "name"); name != "" {
				config["dashboardName"] = name
			}
			if len(config) == 0 {
				return apperrors.NewValidation("must specify either --config or --name")
			}
			return runAitableTool(cmd, runner, "create_dashboard", map[string]any{
				"baseId": baseID,
				"config": config,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	cmd.Flags().String("config", "", i18n.T("仪表盘配置 JSON，结构先看 dashboard config-example；可用 --name 简化创建"))
	cmd.Flags().String("name", "", i18n.T("仪表盘名称（替代 --config 的简化创建）"))
	return cmd
}

func newAitableDashboardUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: i18n.T("更新仪表盘"),
		Example: `  dws aitable dashboard update --base-id BASE_ID --dashboard-id DASHBOARD_ID --config '{"dashboardName":"新名称"}'
  dws aitable dashboard update --base-id BASE_ID --dashboard-id DASHBOARD_ID --name 新名称`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, dashboardID, err := requiredAitableDashboard(cmd)
			if err != nil {
				return err
			}
			config := map[string]any{}
			if configRaw := aitableStringFlag(cmd, "config"); configRaw != "" {
				parsed, err := parseAitableJSONObject(configRaw, "config")
				if err != nil {
					return err
				}
				config = parsed
			}
			if name := aitableStringFlag(cmd, "name"); name != "" {
				config["dashboardName"] = name
			}
			if len(config) == 0 {
				return apperrors.NewValidation("must specify either --config or --name")
			}
			return runAitableTool(cmd, runner, "update_dashboard", map[string]any{
				"baseId":      baseID,
				"dashboardId": dashboardID,
				"config":      config,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableDashboardIDFlags(cmd)
	cmd.Flags().String("config", "", i18n.T("仪表盘配置 JSON，结构先看 dashboard config-example；可用 --name 简化改名"))
	cmd.Flags().String("name", "", i18n.T("新仪表盘名称（替代 --config 的简化改名）"))
	return cmd
}

func newAitableDashboardDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             i18n.T("删除仪表盘"),
		Long:              i18n.T("删除指定仪表盘（高风险、不可逆）。使用 --yes 跳过确认。"),
		Example:           "  dws aitable dashboard delete --base-id BASE_ID --dashboard-id DASHBOARD_ID --yes",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, dashboardID, err := requiredAitableDashboard(cmd)
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, i18n.T("仪表盘"), dashboardID) {
				return nil
			}
			params := map[string]any{
				"baseId":      baseID,
				"dashboardId": dashboardID,
			}
			if reason := aitableStringFlag(cmd, "reason"); reason != "" {
				params["reason"] = reason
			}
			return runAitableTool(cmd, runner, "delete_dashboard", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableDashboardIDFlags(cmd)
	cmd.Flags().String("reason", "", i18n.T("删除原因（可选）"))
	return cmd
}

func newAitableDashboardConfigExampleCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "config-example",
		Short:             i18n.T("获取仪表盘配置模板"),
		Example:           "  dws aitable dashboard config-example",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAitableTool(cmd, runner, "get_dashboard_config_example", map[string]any{})
		},
	}
	preferLegacyLeaf(cmd)
	return cmd
}

func newAitableDashboardShareGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("获取仪表盘分享配置"),
		Example:           "  dws aitable dashboard share get --base-id BASE_ID --dashboard-id DASHBOARD_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, dashboardID, err := requiredAitableDashboard(cmd)
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "get_dashboard_share", map[string]any{
				"baseId":      baseID,
				"dashboardId": dashboardID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableDashboardIDFlags(cmd)
	return cmd
}

func newAitableDashboardShareUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新仪表盘分享配置"),
		Example:           "  dws aitable dashboard share update --base-id BASE_ID --dashboard-id DASHBOARD_ID --enabled true --share-type PUBLIC",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, dashboardID, err := requiredAitableDashboard(cmd)
			if err != nil {
				return err
			}
			params, err := aitableShareUpdateParams(cmd)
			if err != nil {
				return err
			}
			params["baseId"] = baseID
			params["dashboardId"] = dashboardID
			return runAitableTool(cmd, runner, "update_dashboard_share", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableDashboardIDFlags(cmd)
	addAitableShareUpdateFlags(cmd)
	return cmd
}

func newAitableChartGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("获取图表详情"),
		Example:           "  dws aitable chart get --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, dashboardID, chartID, err := requiredAitableChart(cmd)
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "get_chart", map[string]any{
				"baseId":      baseID,
				"dashboardId": dashboardID,
				"chartId":     chartID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableChartIDFlags(cmd)
	return cmd
}

func newAitableChartCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("创建图表"),
		Example:           "  dws aitable chart create --base-id BASE_ID --dashboard-id DASHBOARD_ID --config '{\"chartName\":\"销售柱图\"}' --layout '{\"x\":0,\"y\":0,\"w\":6,\"h\":4}'",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			dashboardID, err := aitableRequiredFlag(cmd, "dashboard-id")
			if err != nil {
				return err
			}
			configRaw, err := aitableRequiredFlag(cmd, "config")
			if err != nil {
				return err
			}
			layoutRaw, err := aitableRequiredFlag(cmd, "layout")
			if err != nil {
				return err
			}
			configValue, err := parseAitableJSONObject(configRaw, "config")
			if err != nil {
				return err
			}
			layoutValue, err := parseAitableJSONObject(layoutRaw, "layout")
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "create_chart", map[string]any{
				"baseId":      baseID,
				"dashboardId": dashboardID,
				"config":      configValue,
				"layout":      layoutValue,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	cmd.Flags().String("dashboard-id", "", i18n.T("Dashboard ID (必填)"))
	cmd.Flags().String("config", "", i18n.T("图表配置 JSON (必填)"))
	cmd.Flags().String("layout", "", i18n.T("图表布局 JSON (必填)"))
	return cmd
}

func newAitableChartUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新图表"),
		Example:           `  dws aitable chart update --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID --config '{"chartName":"新图表"}'`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, dashboardID, chartID, err := requiredAitableChart(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId":      baseID,
				"dashboardId": dashboardID,
				"chartId":     chartID,
			}
			if configRaw := aitableStringFlag(cmd, "config"); configRaw != "" {
				config, err := parseAitableJSONObject(configRaw, "config")
				if err != nil {
					return err
				}
				params["config"] = config
			}
			if layoutRaw := aitableStringFlag(cmd, "layout"); layoutRaw != "" {
				layout, err := parseAitableJSONObject(layoutRaw, "layout")
				if err != nil {
					return err
				}
				params["layout"] = layout
			}
			if _, hasConfig := params["config"]; !hasConfig {
				if _, hasLayout := params["layout"]; !hasLayout {
					return apperrors.NewValidation("at least one of --config or --layout is required")
				}
			}
			return runAitableTool(cmd, runner, "update_chart", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableChartIDFlags(cmd)
	cmd.Flags().String("config", "", i18n.T("图表配置 JSON"))
	cmd.Flags().String("layout", "", i18n.T("图表布局 JSON"))
	return cmd
}

func newAitableChartDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             i18n.T("删除图表"),
		Long:              i18n.T("删除指定图表（高风险、不可逆）。使用 --yes 跳过确认。"),
		Example:           "  dws aitable chart delete --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID --yes",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, dashboardID, chartID, err := requiredAitableChart(cmd)
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, i18n.T("图表"), chartID) {
				return nil
			}
			params := map[string]any{
				"baseId":      baseID,
				"dashboardId": dashboardID,
				"chartId":     chartID,
			}
			if reason := aitableStringFlag(cmd, "reason"); reason != "" {
				params["reason"] = reason
			}
			return runAitableTool(cmd, runner, "delete_chart", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableChartIDFlags(cmd)
	cmd.Flags().String("reason", "", i18n.T("删除原因（可选）"))
	return cmd
}

func newAitableChartWidgetsExampleCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "widgets-example",
		Short:             i18n.T("获取图表配置模板"),
		Example:           "  dws aitable chart widgets-example",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAitableTool(cmd, runner, "get_dashboard_widgets_example", map[string]any{})
		},
	}
	preferLegacyLeaf(cmd)
	return cmd
}

func newAitableChartShareGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("获取图表分享配置"),
		Example:           "  dws aitable chart share get --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, dashboardID, chartID, err := requiredAitableChart(cmd)
			if err != nil {
				return err
			}
			return runAitableTool(cmd, runner, "get_chart_share", map[string]any{
				"baseId":      baseID,
				"dashboardId": dashboardID,
				"chartId":     chartID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableChartIDFlags(cmd)
	return cmd
}

func newAitableChartShareUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新图表分享配置"),
		Example:           "  dws aitable chart share update --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID --enabled true --share-type PUBLIC",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, dashboardID, chartID, err := requiredAitableChart(cmd)
			if err != nil {
				return err
			}
			params, err := aitableShareUpdateParams(cmd)
			if err != nil {
				return err
			}
			params["baseId"] = baseID
			params["dashboardId"] = dashboardID
			params["chartId"] = chartID
			return runAitableTool(cmd, runner, "update_chart_share", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableChartIDFlags(cmd)
	addAitableShareUpdateFlags(cmd)
	return cmd
}

// ── view ───────────────────────────────────────────────────

func newAitableViewGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("获取视图配置"),
		Example:           "  dws aitable view get --base-id BASE_ID --table-id TABLE_ID\n  dws aitable view get --base-id BASE_ID --table-id TABLE_ID --view-ids view1,view2",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, err := requiredAitableBaseTable(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
			}
			if viewIDs := aitableStringFlag(cmd, "view-ids"); viewIDs != "" {
				params["viewIds"] = parseAitableCSVValues(viewIDs)
			}
			return runAitableTool(cmd, runner, "get_views", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("view-ids", "", i18n.T("View ID 列表，逗号分隔；不传返回全部视图"))
	return cmd
}

func newAitableViewListCommand(runner executor.Runner) *cobra.Command {
	cmd := newAitableViewGetCommand(runner)
	cmd.Use = "list"
	cmd.Aliases = nil
	cmd.Short = i18n.T("列出视图配置（view get 的别名）")
	cmd.Example = "  dws aitable view list --base-id BASE_ID --table-id TABLE_ID"
	return cmd
}

func newAitableViewCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("创建视图"),
		Example:           "  dws aitable view create --base-id BASE_ID --table-id TABLE_ID --view-type Grid --name 视图名",
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
			viewType, err := aitableRequiredFlag(cmd, "view-type")
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId":   baseID,
				"tableId":  tableID,
				"viewType": viewType,
			}
			if name := aitableStringFlag(cmd, "name"); name != "" {
				params["viewName"] = name
			}
			if desc := aitableStringFlag(cmd, "desc"); desc != "" {
				descValue, err := parseAitableJSONObject(desc, "desc")
				if err != nil {
					return err
				}
				params["viewDescription"] = descValue
			}
			if viewSubType := aitableStringFlag(cmd, "view-sub-type"); viewSubType != "" {
				params["viewSubType"] = viewSubType
			}
			if configRaw := aitableStringFlag(cmd, "config"); configRaw != "" {
				configValue, err := parseAitableJSONObject(configRaw, "config")
				if err != nil {
					return err
				}
				params["config"] = configValue
			}
			return runAitableTool(cmd, runner, "create_view", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
	cmd.Flags().String("view-type", "", i18n.T("视图类型 (必填)"))
	cmd.Flags().String("name", "", i18n.T("视图名称"))
	cmd.Flags().String("view-sub-type", "", i18n.T("视图子类型"))
	cmd.Flags().String("desc", "", i18n.T("视图描述 JSON"))
	cmd.Flags().String("config", "", i18n.T("视图配置 JSON"))
	return cmd
}

func newAitableViewUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新视图"),
		Example:           `  dws aitable view update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --config '{"visibleFieldIds":["fld1","fld2"]}'`,
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
				"viewId":  viewID,
			}
			if name := aitableStringFlag(cmd, "name"); name != "" {
				params["newViewName"] = name
			}
			if descRaw := aitableStringFlag(cmd, "desc"); descRaw != "" {
				desc, err := parseAitableJSONObject(descRaw, "desc")
				if err != nil {
					return err
				}
				params["viewDescription"] = desc
			}
			if configRaw := aitableStringFlag(cmd, "config"); configRaw != "" {
				config, err := parseAitableJSONObject(configRaw, "config")
				if err != nil {
					return err
				}
				params["config"] = config
			}
			if _, hasName := params["newViewName"]; !hasName {
				if _, hasDesc := params["viewDescription"]; !hasDesc {
					if _, hasConfig := params["config"]; !hasConfig {
						return apperrors.NewValidation("at least one of --name, --desc or --config is required")
					}
				}
			}
			return runAitableTool(cmd, runner, "update_view", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("name", "", i18n.T("新视图名称"))
	cmd.Flags().String("desc", "", i18n.T("视图描述 JSON"))
	cmd.Flags().String("config", "", i18n.T("视图配置 JSON"))
	return cmd
}

func newAitableViewDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             i18n.T("删除视图"),
		Long:              i18n.T("删除指定视图（高风险、不可逆）。使用 --yes 跳过确认。"),
		Example:           "  dws aitable view delete --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --yes",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, i18n.T("视图"), viewID) {
				return nil
			}
			return runAitableTool(cmd, runner, "delete_view", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewId":  viewID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	return cmd
}

// ── helpers ────────────────────────────────────────────────

func runAitableTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	return runAitableProductTool(cmd, runner, "aitable", tool, params)
}

func runAitableFormTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	return runAitableProductTool(cmd, runner, "aitable-form", tool, params)
}

func runAitableProductTool(cmd *cobra.Command, runner executor.Runner, product, tool string, params map[string]any) error {
	invocation := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd),
		product,
		tool,
		params,
	)
	invocation.DryRun = commandDryRun(cmd)
	if invocation.DryRun {
		result, err := runner.Run(cmd.Context(), invocation)
		if err != nil {
			return err
		}
		return writeCommandPayload(cmd, result)
	}

	var lastErr error
	for attempt := 0; attempt <= aitableHelperMaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			fmt.Fprintf(cmd.ErrOrStderr(), "[%s retry %d/%d] %s after %v...\n", product, attempt, aitableHelperMaxRetries, tool, backoff)
			timer := time.NewTimer(backoff)
			select {
			case <-cmd.Context().Done():
				timer.Stop()
				return cmd.Context().Err()
			case <-timer.C:
			}
		}

		result, err := runner.Run(cmd.Context(), invocation)
		lastErr = err
		if err == nil {
			return writeCommandPayload(cmd, result)
		}
		if !aitableErrorRetryable(err) {
			return err
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return nil
}

const aitableHelperMaxRetries = 3

func aitableErrorRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())

	retryablePatterns := []string{
		"timeout", "deadline exceeded", "connection reset",
		"connection refused", "broken pipe", "eof",
		"network is unreachable", "i/o timeout",
		"tls handshake", "server misbehaving", "temporary failure",
		"no such host",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	if strings.Contains(msg, "system_error") || strings.Contains(msg, "internal_error") ||
		strings.Contains(msg, "service_unavailable") || strings.Contains(msg, "gateway_timeout") {
		return true
	}

	if strings.Contains(msg, "retryable") && strings.Contains(msg, "true") {
		return true
	}

	return false
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

func requiredAitableDashboard(cmd *cobra.Command) (string, string, error) {
	baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
	if err != nil {
		return "", "", err
	}
	dashboardID, err := aitableRequiredFlag(cmd, "dashboard-id")
	if err != nil {
		return "", "", err
	}
	return baseID, dashboardID, nil
}

func requiredAitableChart(cmd *cobra.Command) (string, string, string, error) {
	baseID, dashboardID, err := requiredAitableDashboard(cmd)
	if err != nil {
		return "", "", "", err
	}
	chartID, err := aitableRequiredFlag(cmd, "chart-id")
	if err != nil {
		return "", "", "", err
	}
	return baseID, dashboardID, chartID, nil
}

func addAitableHiddenStringFlag(cmd *cobra.Command, name, usage string) {
	cmd.Flags().String(name, "", i18n.T(usage))
	_ = cmd.Flags().MarkHidden(name)
}

func addAitableDashboardIDFlags(cmd *cobra.Command) {
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	cmd.Flags().String("dashboard-id", "", i18n.T("Dashboard ID (必填)"))
}

func addAitableChartIDFlags(cmd *cobra.Command) {
	addAitableDashboardIDFlags(cmd)
	cmd.Flags().String("chart-id", "", i18n.T("Chart ID (必填)"))
}

func addAitableShareUpdateFlags(cmd *cobra.Command) {
	cmd.Flags().String("enabled", "", i18n.T("是否开启分享 (必填): true/false"))
	cmd.Flags().String("share-type", "", i18n.T("分享类型: PUBLIC / ORG"))
	cmd.Flags().String("allow-back-to-doc", "", i18n.T("是否允许从分享页返回源文档: true/false"))
}

func aitableShareUpdateParams(cmd *cobra.Command) (map[string]any, error) {
	enabledRaw := aitableStringFlag(cmd, "enabled")
	if enabledRaw == "" {
		return nil, apperrors.NewValidation("--enabled is required")
	}
	enabled, err := parseAitableBoolString(enabledRaw, "enabled")
	if err != nil {
		return nil, err
	}
	params := map[string]any{"enabled": enabled}
	if shareType := aitableStringFlag(cmd, "share-type"); shareType != "" {
		params["shareType"] = shareType
	}
	if allowRaw := aitableStringFlag(cmd, "allow-back-to-doc"); allowRaw != "" {
		allow, err := parseAitableBoolString(allowRaw, "allow-back-to-doc")
		if err != nil {
			return nil, err
		}
		params["allowBackToDoc"] = allow
	}
	return params, nil
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
		return normalizeAitableFields(fields), nil
	}
	var wrapper map[string]any
	if err := json.Unmarshal([]byte(raw), &wrapper); err == nil {
		if wrappedFields, ok := wrapper["fields"].([]any); ok {
			return normalizeAitableFields(wrappedFields), nil
		}
	}
	return nil, apperrors.NewValidation("--fields JSON parse failed: expect a JSON array")
}

func normalizeAitableFields(fields []any) []any {
	for _, field := range fields {
		fieldMap, ok := field.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := fieldMap["fieldName"]; !ok {
			if name, ok := fieldMap["name"].(string); ok && strings.TrimSpace(name) != "" {
				fieldMap["fieldName"] = strings.TrimSpace(name)
				delete(fieldMap, "name")
			}
		}
		if fieldType, ok := fieldMap["type"].(string); ok {
			fieldMap["type"] = normalizeAitableFieldType(fieldType)
		}
	}
	return fields
}

func normalizeAitableFieldType(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return trimmed
	}
	key := strings.ToLower(strings.NewReplacer("-", "", "_", "", " ", "").Replace(trimmed))
	aliases := map[string]string{
		"text":               "text",
		"number":             "number",
		"singleselect":       "singleSelect",
		"select":             "singleSelect",
		"multipleselect":     "multipleSelect",
		"multiselect":        "multipleSelect",
		"date":               "date",
		"currency":           "currency",
		"progress":           "progress",
		"rating":             "rating",
		"checkbox":           "checkbox",
		"user":               "user",
		"member":             "user",
		"person":             "user",
		"department":         "department",
		"group":              "group",
		"url":                "url",
		"richtext":           "richText",
		"telephone":          "telephone",
		"phone":              "telephone",
		"email":              "email",
		"attachment":         "attachment",
		"file":               "attachment",
		"geolocation":        "geolocation",
		"formula":            "formula",
		"unidirectionallink": "unidirectionalLink",
		"bidirectionallink":  "bidirectionalLink",
		"creator":            "creator",
		"lastmodifier":       "lastModifier",
		"createdtime":        "createdTime",
		"lastmodifiedtime":   "lastModifiedTime",
	}
	if normalized, ok := aliases[key]; ok {
		return normalized
	}
	return trimmed
}

func validateAitableFiltersStructure(parsed map[string]any) error {
	if parsed == nil {
		return nil
	}
	op, hasOp := parsed["operator"]
	if !hasOp {
		return apperrors.NewValidation(`invalid --filters structure: missing "operator" field at root level`)
	}
	opStr, ok := op.(string)
	if !ok {
		return apperrors.NewValidation(fmt.Sprintf(`invalid --filters structure: "operator" must be a string, got %T`, op))
	}
	opLower := strings.ToLower(opStr)
	if opLower != "and" && opLower != "or" {
		return apperrors.NewValidation(fmt.Sprintf(`invalid --filters structure: root "operator" must be "and" or "or", got %q`, opStr))
	}
	operands, hasOperands := parsed["operands"]
	if !hasOperands {
		return apperrors.NewValidation(`invalid --filters structure: missing "operands" array at root level`)
	}
	operandsArr, ok := operands.([]any)
	if !ok {
		return apperrors.NewValidation(fmt.Sprintf(`invalid --filters structure: "operands" must be an array, got %T`, operands))
	}
	for i, item := range operandsArr {
		cond, ok := item.(map[string]any)
		if !ok {
			continue
		}
		childOp, has := cond["operator"]
		if !has {
			continue
		}
		childOpStr, ok := childOp.(string)
		if !ok {
			continue
		}
		if suggestion := suggestAitableFilterOperator(childOpStr); suggestion != "" {
			return apperrors.NewValidation(fmt.Sprintf(`invalid filter operator %q in operands[%d]. Did you mean %q?`, childOpStr, i, suggestion))
		}
	}
	return nil
}

var aitableValidFilterOperators = map[string]bool{
	"eq": true, "ne": true, "gt": true, "lt": true, "gte": true, "lte": true,
	"contain": true, "exclusive": true, "exist": true, "un_exist": true,
	"any_of": true, "all_of": true, "none_of": true,
	"date_eq": true, "before": true, "after": true, "not_before": true, "not_after": true,
	"from_now": true, "date_between": true,
	"and": true, "or": true,
}

var aitableFilterOperatorAliases = map[string]string{
	"equal": "eq", "equals": "eq", "is": "eq", "==": "eq",
	"not_equal": "ne", "not_equals": "ne", "is_not": "ne", "!=": "ne",
	"like": "contain", "contains": "contain", "include": "contain",
	"greater_than": "gt", "less_than": "lt",
	"greater_than_or_equal": "gte", "less_than_or_equal": "lte",
}

func suggestAitableFilterOperator(op string) string {
	if aitableValidFilterOperators[op] {
		return ""
	}
	if suggestion, ok := aitableFilterOperatorAliases[op]; ok {
		return suggestion
	}
	return "eq"
}

func normalizeAitableFilters(parsed any) any {
	filterMap, ok := parsed.(map[string]any)
	if !ok {
		return parsed
	}
	operands, has := filterMap["operands"]
	if !has {
		return parsed
	}
	operandsArr, ok := operands.([]any)
	if !ok {
		return parsed
	}
	normalized := make([]any, 0, len(operandsArr))
	for _, item := range operandsArr {
		cond, ok := item.(map[string]any)
		if !ok {
			normalized = append(normalized, item)
			continue
		}
		if fieldID, hasFieldID := cond["fieldId"]; hasFieldID {
			newCond := map[string]any{"operator": cond["operator"]}
			if value, hasValue := cond["value"]; hasValue {
				newCond["operands"] = []any{fieldID, value}
			} else {
				newCond["operands"] = []any{fieldID}
			}
			normalized = append(normalized, newCond)
			continue
		}
		if _, hasChildOperands := cond["operands"]; hasChildOperands {
			normalized = append(normalized, normalizeAitableFilters(cond))
			continue
		}
		normalized = append(normalized, cond)
	}
	return map[string]any{
		"operator": filterMap["operator"],
		"operands": normalized,
	}
}

func normalizeAitableSort(items []any) []any {
	normalized := make([]any, 0, len(items))
	for _, item := range items {
		sortItem, ok := item.(map[string]any)
		if !ok {
			normalized = append(normalized, item)
			continue
		}
		if val, hasOrder := sortItem["order"]; hasOrder {
			if _, hasDirection := sortItem["direction"]; !hasDirection {
				sortItem["direction"] = val
				delete(sortItem, "order")
			}
		}
		normalized = append(normalized, sortItem)
	}
	return normalized
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

// runRecordQueryAll 实现 dws aitable record query --all：用 pkg/paging 反复调
// query_records 直到拉完 / 触发 page-limit / 中途失败优雅降级。
//
// 输出契约（写到 stdout 的 JSON）：
//
//	{
//	  "records": [...累计所有页...],
//	  "hasMore": true|false,
//	  "cursor":  "下一页 cursor，hasMore=true 时有效",
//	  "partial": true|false,
//	  "pages":   实际翻了多少页
//	}
func runRecordQueryAll(cmd *cobra.Command, runner executor.Runner, baseParams map[string]any, initialCursor string) error {
	pageLimit, _ := cmd.Flags().GetInt("page-limit")

	// dry-run 模式：只打印第一次请求的预览，不实际循环
	if commandDryRun(cmd) {
		params := cloneMap(baseParams)
		if initialCursor != "" {
			params["cursor"] = initialCursor
		}
		params["__paging__"] = map[string]any{"all": true, "pageLimit": pageLimit}
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "aitable", "query_records", params,
		))
	}

	fetcher := func(ctx context.Context, cursor string) (paging.Page, error) {
		params := cloneMap(baseParams)
		if cursor != "" {
			params["cursor"] = cursor
		}
		invocation := executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "aitable", "query_records", params,
		)
		result, err := runner.Run(ctx, invocation)
		if err != nil {
			return paging.Page{}, err
		}
		records, nextCursor := extractRecordsAndCursor(result.Response)
		return paging.Page{Records: records, NextCursor: nextCursor}, nil
	}

	res := paging.FetchAll(cmd.Context(), fetcher, paging.Options{
		PageLimit:     pageLimit,
		InitialCursor: initialCursor,
	})

	// 组装输出
	output := map[string]any{
		"records": res.Records,
		"hasMore": res.HasMore,
		"pages":   res.Pages,
	}
	if res.LastCursor != "" {
		output["cursor"] = res.LastCursor
	}
	if res.Partial {
		output["partial"] = true
	}
	// 因 page-limit 截断不算错误，其它错误透传到 stderr 提示但不退出 1（保留已拉数据）
	if res.Err != nil && !errors.Is(res.Err, paging.ErrPageLimitReached) {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v (returned %d records before failure)\n", res.Err, len(res.Records))
	}

	return writeCommandPayload(cmd, output)
}

// extractRecordsAndCursor 从 query_records MCP 响应里抽出 records 数组与 nextCursor。
// 兼容两种包装：result.Response 直接含 records / 经 content/data 二次包装。
func extractRecordsAndCursor(resp map[string]any) ([]any, string) {
	if resp == nil {
		return nil, ""
	}
	src := resp
	if content, ok := src["content"].(map[string]any); ok && len(content) > 0 {
		src = content
	}
	data, _ := src["data"].(map[string]any)
	if data == nil {
		data = src
	}
	records, _ := data["records"].([]any)
	cursor, _ := data["nextCursor"].(string)
	if cursor == "" {
		// 容错：部分上游可能用 cursor / pageToken 字段名
		if c, ok := data["cursor"].(string); ok {
			cursor = c
		} else if c, ok := data["pageToken"].(string); ok {
			cursor = c
		}
	}
	return records, cursor
}

func resolveAitableRecordCreateRecords(cmd *cobra.Command) ([]any, error) {
	return resolveAitableRecordsInput(cmd, "records", "fields")
}

func resolveAitableRecordUpdateRecords(cmd *cobra.Command) ([]any, error) {
	if records, err := resolveAitableRecordsInput(cmd, "records", "fields"); err == nil {
		return records, nil
	} else if aitableStringFlag(cmd, "records-file") != "" ||
		aitableStringFlag(cmd, "records") != "" ||
		aitableStringFlag(cmd, "fields") != "" {
		return nil, err
	}

	recordID := aitableStringFlag(cmd, "record-id")
	if dataRaw := aitableStringFlag(cmd, "data"); dataRaw != "" {
		return parseAitableRecordUpdateData(dataRaw, recordID)
	}

	cellsRaw := aitableStringFlag(cmd, "cells")
	if recordID != "" || cellsRaw != "" {
		if recordID == "" {
			return nil, apperrors.NewValidation("--record-id is required when using --cells")
		}
		if cellsRaw == "" {
			return nil, apperrors.NewValidation("--cells is required when using --record-id")
		}
		cells, err := parseAitableJSONObject(cellsRaw, "cells")
		if err != nil {
			return nil, err
		}
		return []any{map[string]any{
			"recordId": recordID,
			"cells":    cells,
		}}, nil
	}

	return nil, apperrors.NewValidation("--records is required")
}

func resolveAitableRecordsInput(cmd *cobra.Command, primary string, aliases ...string) ([]any, error) {
	if filePath := aitableStringFlag(cmd, "records-file"); filePath != "" {
		return parseAitableRecordsFile(filePath)
	}
	if recordsRaw := aitableStringFlag(cmd, primary); recordsRaw != "" {
		return parseAitableJSONArray(recordsRaw, primary)
	}
	for _, alias := range aliases {
		if recordsRaw := aitableStringFlag(cmd, alias); recordsRaw != "" {
			return parseAitableJSONArray(recordsRaw, alias)
		}
	}
	return nil, apperrors.NewValidation("--records is required")
}

func parseAitableRecordsFile(filePath string) ([]any, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--records-file read failed: %v", err))
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return nil, apperrors.NewValidation("--records-file is empty")
	}
	return parseAitableJSONArray(raw, "records-file")
}

func parseAitableRecordUpdateData(raw, recordID string) ([]any, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--data JSON parse failed: %v", err))
	}

	switch typed := value.(type) {
	case []any:
		return typed, nil
	case map[string]any:
		if recordsValue, ok := typed["records"]; ok {
			records, ok := recordsValue.([]any)
			if !ok {
				return nil, apperrors.NewValidation("--data.records must be a JSON array")
			}
			return records, nil
		}

		if _, ok := typed["cells"]; ok {
			if strings.TrimSpace(recordID) != "" && strings.TrimSpace(recordIDFromRecordObject(typed)) == "" {
				typed["recordId"] = strings.TrimSpace(recordID)
			}
			if strings.TrimSpace(recordIDFromRecordObject(typed)) == "" {
				return nil, apperrors.NewValidation("--data record object requires recordId")
			}
			return []any{typed}, nil
		}

		if strings.TrimSpace(recordID) != "" {
			return []any{map[string]any{
				"recordId": strings.TrimSpace(recordID),
				"cells":    typed,
			}}, nil
		}

		return nil, apperrors.NewValidation("--record-id is required when --data is a cells object")
	default:
		return nil, apperrors.NewValidation("--data must be a JSON array or object")
	}
}

func recordIDFromRecordObject(record map[string]any) string {
	if record == nil {
		return ""
	}
	if value, ok := record["recordId"].(string); ok {
		return value
	}
	return ""
}
