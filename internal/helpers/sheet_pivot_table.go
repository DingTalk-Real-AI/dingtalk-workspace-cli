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
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var validPivotSummarizeBy = map[string]bool{
	"sum": true, "count": true, "average": true,
	"max": true, "min": true, "product": true,
	"count_numbers": true, "std_dev": true, "std_dev_p": true,
	"var": true, "var_p": true, "distinct": true, "median": true,
}

func validatePivotTableProperties(props map[string]any, requireValues bool) error {
	valuesRaw, hasValues := props["values"]
	if requireValues && (!hasValues || valuesRaw == nil) {
		return fmt.Errorf("--properties 缺少必填字段 values")
	}
	if hasValues && valuesRaw != nil {
		values, ok := valuesRaw.([]any)
		if !ok {
			return fmt.Errorf("values 必须为数组")
		}
		if requireValues && len(values) == 0 {
			return fmt.Errorf("values 必须至少包含一项")
		}
		for i, item := range values {
			field, err := validatePivotField(item, fmt.Sprintf("values[%d]", i))
			if err != nil {
				return err
			}
			_ = field
			obj := item.(map[string]any)
			if raw, ok := obj["summarize_by"]; ok && raw != nil {
				value, ok := raw.(string)
				if !ok {
					return fmt.Errorf("values[%d].summarize_by 必须为字符串", i)
				}
				normalized := strings.ToLower(strings.TrimSpace(value))
				if !validPivotSummarizeBy[normalized] {
					return fmt.Errorf("values[%d].summarize_by 不支持: %q", i, value)
				}
				obj["summarize_by"] = normalized
			}
		}
	}
	for _, name := range []string{"rows", "columns", "filters"} {
		if err := validatePivotFieldArray(props, name); err != nil {
			return err
		}
	}
	if raw, ok := props["collapse"]; ok && raw != nil {
		switch raw.(type) {
		case map[string]any, []any:
		default:
			return fmt.Errorf("collapse 必须为对象或数组")
		}
	}
	return nil
}

func validatePivotField(item any, path string) (string, error) {
	obj, ok := item.(map[string]any)
	if !ok {
		return "", fmt.Errorf("%s 必须为对象", path)
	}
	raw, ok := obj["field"]
	if !ok || raw == nil {
		return "", fmt.Errorf("%s.field 为必填字段", path)
	}
	field, ok := raw.(string)
	if !ok || strings.TrimSpace(field) == "" {
		return "", fmt.Errorf("%s.field 必须为非空字符串", path)
	}
	return field, nil
}

func validatePivotFieldArray(props map[string]any, name string) error {
	raw, ok := props[name]
	if !ok || raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%s 必须为数组", name)
	}
	for i, item := range items {
		if _, err := validatePivotField(item, fmt.Sprintf("%s[%d]", name, i)); err != nil {
			return err
		}
	}
	return nil
}

func readPivotProperties(raw string, requireValues bool) (map[string]any, error) {
	if strings.HasPrefix(raw, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(raw, "@"))
		if err != nil {
			return nil, fmt.Errorf("读取 properties 文件失败: %w", err)
		}
		raw = string(data)
	}
	var properties map[string]any
	if err := json.Unmarshal([]byte(raw), &properties); err != nil {
		return nil, fmt.Errorf("--properties JSON 解析失败: %w", err)
	}
	if len(properties) == 0 {
		return nil, fmt.Errorf("--properties 不能为空对象")
	}
	if err := validatePivotTableProperties(properties, requireValues); err != nil {
		return nil, err
	}
	return properties, nil
}

func newPivotTableCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "pivot-table",
		Short: "透视表管理",
		RunE:  groupRunE,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "获取透视表列表或详情",
		Example: `  dws sheet pivot-table list --node NODE_ID --sheet-id SHEET_ID
  dws sheet pivot-table list --node NODE_ID --sheet-id SHEET_ID --pivot-table-id PT_ID`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id"); err != nil {
				return err
			}
			args := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
			}
			if value, _ := cmd.Flags().GetString("pivot-table-id"); value != "" {
				args["pivotTableId"] = value
			}
			return callMCPTool("list_pivot_tables", args)
		},
	}
	listCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	listCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	listCmd.Flags().String("pivot-table-id", "", "透视表 ID (可选，不传则返回全部)")

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "创建透视表",
		Long: `在指定数据源区域上创建原生透视表。

--source 必须使用带工作表前缀的 A1 范围，例如 "'Sheet1'!A1:D100"。
--properties 为 JSON 对象或 @file，values 至少一项；可包含 rows、columns、filters、collapse 和总计显示选项。`,
		Example: `  dws sheet pivot-table create --node NODE_ID \
    --source "'Sheet1'!A1:D100" \
    --properties '{"rows":[{"field":"部门"}],"values":[{"field":"销售额","summarize_by":"sum"}]}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlags(cmd, "node", "source", "properties"); err != nil {
				return err
			}
			properties, err := readPivotProperties(mustGetFlag(cmd, "properties"), true)
			if err != nil {
				return err
			}
			args := map[string]any{
				"nodeId":     mustGetFlag(cmd, "node"),
				"source":     mustGetFlag(cmd, "source"),
				"properties": properties,
			}
			if value, _ := cmd.Flags().GetString("target-sheet-id"); value != "" {
				args["targetSheetId"] = value
			}
			if value, _ := cmd.Flags().GetString("target-position"); value != "" {
				args["targetPosition"] = value
			}
			return callMCPTool("create_pivot_table", args)
		},
	}
	createCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	createCmd.Flags().String("source", "", "数据源区域，A1 表示法且包含工作表前缀 (必填)")
	createCmd.Flags().String("properties", "", "透视表配置 JSON 或 @文件路径 (必填)")
	createCmd.Flags().String("target-sheet-id", "", "目标工作表 ID 或名称 (可选，不传则自动新建)")
	createCmd.Flags().String("target-position", "", "透视表放置位置，A1 单元格地址 (可选)")

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新透视表配置",
		Example: `  dws sheet pivot-table update --node NODE_ID --sheet-id SHEET_ID \
    --pivot-table-id PT_ID --properties '{"show_subtotals":false}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "pivot-table-id", "properties"); err != nil {
				return err
			}
			properties, err := readPivotProperties(mustGetFlag(cmd, "properties"), false)
			if err != nil {
				return err
			}
			return callMCPTool("update_pivot_table", map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"pivotTableId": mustGetFlag(cmd, "pivot-table-id"),
				"properties":   properties,
			})
		},
	}
	updateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	updateCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	updateCmd.Flags().String("pivot-table-id", "", "透视表 ID (必填)")
	updateCmd.Flags().String("properties", "", "需要更新的透视表配置 JSON 或 @文件路径 (必填)")

	deleteCmd := &cobra.Command{
		Use:     "delete",
		Short:   "[危险] 删除透视表",
		Example: `  dws sheet pivot-table delete --node NODE_ID --sheet-id SHEET_ID --pivot-table-id PT_ID --yes`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "pivot-table-id"); err != nil {
				return err
			}
			pivotTableID := mustGetFlag(cmd, "pivot-table-id")
			return callMCPTool("delete_pivot_table", map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"pivotTableId": pivotTableID,
			})
		},
	}
	deleteCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	deleteCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	deleteCmd.Flags().String("pivot-table-id", "", "透视表 ID (必填)")

	root.AddCommand(listCmd, createCmd, updateCmd, deleteCmd)
	attachUnknownSubcommandGuard(root)
	return root
}
