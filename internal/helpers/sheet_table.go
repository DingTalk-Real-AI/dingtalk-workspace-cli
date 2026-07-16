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
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newTableCmds() []*cobra.Command {
	tableGetCmd := &cobra.Command{
		Use:     "table-get",
		Aliases: []string{"table-read"},
		Short:   "读取结构化 table 数据",
		Long: `读取结构化 table 数据。

返回列名、行数据、pandas-style dtypes 和表格 number formats。`,
		Example: `  dws sheet table-get --node NODE_ID
  dws sheet table-get --node NODE_ID --sheet-id SHEET_ID --range A1:D20
  dws sheet table-get --node NODE_ID --sheet-id Sheet1 --no-header`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "file-id", "node-id", "doc-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId": nodeID,
			}
			if v, _ := cmd.Flags().GetString("sheet-id"); v != "" {
				toolArgs["sheetId"] = v
			}
			if v, _ := cmd.Flags().GetString("range"); v != "" {
				toolArgs["range"] = v
			}
			if cmd.Flags().Changed("no-header") {
				v, _ := cmd.Flags().GetBool("no-header")
				toolArgs["noHeader"] = v
			}
			return callMCPTool("table_get", toolArgs)
		},
	}
	tableGetCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	tableGetCmd.Flags().String("sheet-id", "", "工作表 ID 或名称")
	tableGetCmd.Flags().String("range", "", "读取范围，A1 表示法；可带 sheet 前缀，如 Sheet1!A1:D10")
	tableGetCmd.Flags().Bool("no-header", false, "首行不作为表头，自动生成 col1/col2/...")

	tablePutCmd := &cobra.Command{
		Use:     "table-put",
		Aliases: []string{"table-write"},
		Short:   "写入结构化 table 数据",
		Long: `写入结构化 table 数据。

支持一次写入一个或多个 sheet；目标 sheet 存在时写入，传 name 且不存在时会创建 sheet。

--sheets 支持三种输入：
  JSON 数组                  [{"name":"Sheet1","columns":["name"],"data":[["Alice"]]}]
  JSON 对象                  {"sheets":[{"name":"Sheet1","columns":["name"],"data":[["Alice"]]}]}
  单个 sheet spec JSON 对象   {"name":"Sheet1","columns":["name"],"data":[["Alice"]]}

--sheets 可传 @filepath 从文件读取，或传 - 从 stdin 读取。`,
		Example: `  dws sheet table-put --node NODE_ID \
    --sheets '[{"name":"Sheet1","columns":["name","score"],"data":[["Alice",95]],"dtypes":{"score":"float64"}}]'

  dws sheet table-put --node NODE_ID --sheets @table.json
  cat table.json | dws sheet table-put --node NODE_ID --sheets -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "file-id", "node-id", "doc-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "sheets"); err != nil {
				return err
			}
			raw, err := readTableJSONFlag(cmd, "sheets")
			if err != nil {
				return err
			}
			sheets, err := parseTablePutSheets(raw)
			if err != nil {
				return err
			}
			return callMCPTool("table_put", map[string]any{
				"nodeId": nodeID,
				"sheets": sheets,
			})
		},
	}
	tablePutCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	tablePutCmd.Flags().String("sheets", "", "sheet table JSON、@文件路径 或 - 表示 stdin (必填)")

	return []*cobra.Command{tableGetCmd, tablePutCmd}
}

func readTableJSONFlag(cmd *cobra.Command, flagName string) (string, error) {
	value := mustGetFlag(cmd, flagName)
	switch {
	case value == "-":
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("读取 stdin 失败: %w", err)
		}
		return string(data), nil
	case strings.HasPrefix(value, "@"):
		data, err := os.ReadFile(strings.TrimPrefix(value, "@"))
		if err != nil {
			return "", fmt.Errorf("读取 JSON 文件失败: %w", err)
		}
		return string(data), nil
	default:
		return value, nil
	}
}

func parseTablePutSheets(raw string) ([]any, error) {
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("--sheets JSON 解析失败: %w", err)
	}

	switch value := payload.(type) {
	case []any:
		if len(value) == 0 {
			return nil, fmt.Errorf("--sheets must contain at least one sheet spec")
		}
		return value, nil
	case map[string]any:
		if sheets, ok := value["sheets"]; ok {
			items, ok := sheets.([]any)
			if !ok {
				return nil, fmt.Errorf("--sheets.sheets must be a JSON array")
			}
			if len(items) == 0 {
				return nil, fmt.Errorf("--sheets.sheets must contain at least one sheet spec")
			}
			return items, nil
		}
		return []any{value}, nil
	default:
		return nil, fmt.Errorf("--sheets must be a JSON array, an object with sheets, or a single sheet spec object")
	}
}
