package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newDataCmds() []*cobra.Command {
	findCmd := &cobra.Command{
		Use:   "find",
		Short: "在工作表中搜索单元格内容",
		Long: `在钉钉电子表格的指定工作表中查找匹配指定文本的所有单元格，返回匹配的单元格地址和值。

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表，通过 --query 指定查找内容（--find 为其别名）。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。

支持多种查找模式：
  默认模式         在单元格值中进行子字符串匹配（区分大小写）
  忽略大小写       通过 --match-case=false 实现
  完整单元格匹配   通过 --match-entire-cell 实现
  正则表达式       通过 --use-regexp 实现，--query 作为正则表达式匹配
  搜索公式文本     通过 --match-formula 实现，在公式文本中查找而非计算结果
  包含隐藏单元格   通过 --include-hidden 实现

可通过 --range 限定搜索范围（A1 表示法），不传时搜索整个工作表。`,
		Example: `  # 基本搜索
  dws sheet find --node NODE_ID --sheet-id SHEET_ID --query "销售额"

  # 在指定范围内搜索
  dws sheet find --node NODE_ID --sheet-id SHEET_ID --query "合计" --range "A1:D100"

  # 正则表达式搜索（不区分大小写）
  dws sheet find --node NODE_ID --sheet-id SHEET_ID --query "^total" --use-regexp --match-case=false

  # 精确匹配整个单元格内容
  dws sheet find --node NODE_ID --sheet-id SHEET_ID --query "完成" --match-entire-cell

  # 搜索公式文本（使用 --find 别名）
  dws sheet find --node NODE_ID --sheet-id SHEET_ID --find "SUM" --match-formula`,
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := mustFlagOrFallback(cmd, "query", "find")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"text":    text,
			}
			if v, _ := cmd.Flags().GetString("range"); v != "" {
				toolArgs["range"] = v
			}
			matchCase, _ := cmd.Flags().GetBool("match-case")
			toolArgs["matchCase"] = matchCase
			matchEntireCell, _ := cmd.Flags().GetBool("match-entire-cell")
			toolArgs["matchEntireCell"] = matchEntireCell
			useRegExp, _ := cmd.Flags().GetBool("use-regexp")
			toolArgs["useRegExp"] = useRegExp
			matchFormula, _ := cmd.Flags().GetBool("match-formula")
			toolArgs["matchFormulaText"] = matchFormula
			includeHidden, _ := cmd.Flags().GetBool("include-hidden")
			toolArgs["includeHidden"] = includeHidden
			return callMCPTool("find_cells", toolArgs)
		},
	}
	findCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	findCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	findCmd.Flags().String("query", "", "搜索文本 (必填，别名: --find)")
	findCmd.Flags().String("find", "", "--query 的别名")
	_ = findCmd.Flags().MarkHidden("find")
	findCmd.Flags().String("range", "", "搜索范围，A1 表示法 (如 A1:D10)")
	findCmd.Flags().Bool("match-case", true, "区分大小写 (默认 true)")
	findCmd.Flags().Bool("match-entire-cell", false, "精确匹配整个单元格内容")
	findCmd.Flags().Bool("use-regexp", false, "启用正则表达式搜索")
	findCmd.Flags().Bool("match-formula", false, "搜索公式文本而非显示值")
	findCmd.Flags().Bool("include-hidden", false, "包含隐藏单元格")

	replaceCmd := &cobra.Command{
		Use:   "replace",
		Short: "查找替换/批量替换/精确匹配替换/正则替换文本",
		Long: `在钉钉表格的指定工作表中，全局查找并替换文本内容。

支持以下替换选项：
  --match-case           区分大小写（默认 false）
  --match-entire-cell    完整单元格匹配（默认 false）
  --use-regexp           使用正则表达式匹配（默认 false）
  --include-hidden       包含隐藏行/列（默认 false）
  --range                限定替换范围，A1 表示法（不传时在整个工作表中替换）

返回被替换的单元格数量。

nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。`,
		Example: `  # 基本替换
  dws sheet replace --node NODE_ID --sheet-id SHEET_ID --find "旧文本" --replacement "新文本"

  # 精确匹配整个单元格后替换
  dws sheet replace --node NODE_ID --sheet-id SHEET_ID --find "待处理" --replacement "已完成" --match-entire-cell

  # 正则表达式替换
  dws sheet replace --node NODE_ID --sheet-id SHEET_ID --find "\\d{4}" --replacement "****" --use-regexp

  # 在指定范围内替换
  dws sheet replace --node NODE_ID --sheet-id SHEET_ID --find "旧" --replacement "新" --range "A1:D100"

  # 删除匹配内容（替换为空）
  dws sheet replace --node NODE_ID --sheet-id SHEET_ID --find "临时" --replacement ""`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"nodeId":      mustGetFlag(cmd, "node"),
				"sheetId":     mustGetFlag(cmd, "sheet-id"),
				"text":        mustGetFlag(cmd, "find"),
				"replaceText": mustGetFlag(cmd, "replacement"),
			}
			if v, _ := cmd.Flags().GetString("range"); v != "" {
				toolArgs["range"] = v
			}
			matchCase, _ := cmd.Flags().GetBool("match-case")
			toolArgs["matchCase"] = matchCase
			matchEntireCell, _ := cmd.Flags().GetBool("match-entire-cell")
			toolArgs["matchEntireCell"] = matchEntireCell
			useRegExp, _ := cmd.Flags().GetBool("use-regexp")
			toolArgs["useRegExp"] = useRegExp
			includeHidden, _ := cmd.Flags().GetBool("include-hidden")
			toolArgs["includeHidden"] = includeHidden
			return callMCPTool("replace_all", toolArgs)
		},
	}
	replaceCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	replaceCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	replaceCmd.Flags().String("find", "", "查找文本 (必填)")
	replaceCmd.Flags().String("replacement", "", "替换文本 (必填，可为空字符串表示删除)")
	replaceCmd.Flags().String("range", "", "替换范围，A1 表示法 (如 A1:D100)")
	replaceCmd.Flags().Bool("match-case", false, "区分大小写 (默认 false)")
	replaceCmd.Flags().Bool("match-entire-cell", false, "完整单元格匹配")
	replaceCmd.Flags().Bool("use-regexp", false, "启用正则表达式匹配")
	replaceCmd.Flags().Bool("include-hidden", false, "包含隐藏行/列")

	appendCmd := &cobra.Command{
		Use:   "append",
		Short: "在工作表末尾追加数据",
		Long: `在钉钉电子表格的指定工作表末尾追加若干行数据。

系统会自动定位到工作表中最后一行有数据的位置，在其下方插入新行数据。
如果工作表为空，数据将从第一行开始写入。

--values 为二维 JSON 数组，外层数组的每个元素代表一行，内层数组的每个元素代表该行中的一个单元格值。
追加的数据列数应与工作表已有数据的列数保持一致，以确保数据对齐。

nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。`,
		Example: `  dws sheet append --node NODE_ID --sheet-id SHEET_ID --values '[["张三","销售部",50000]]'
  dws sheet append --node NODE_ID --sheet-id SHEET_ID \
    --values '[["李四","市场部",38000],["王五","销售部",62000]]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "values"); err != nil {
				return err
			}
			var values [][]any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "values")), &values); err != nil {
				return fmt.Errorf("--values JSON 解析失败: %w", err)
			}
			return callMCPTool("append_rows", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"values":  values,
			})
		},
	}
	appendCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	appendCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	appendCmd.Flags().String("values", "", "追加数据，二维 JSON 数组 (必填)")

	csvPutCmd := &cobra.Command{
		Use:   "csv-put",
		Short: "将 CSV 数据写入表格指定位置（纯值，自动扩容）",
		Long: `将 RFC 4180 格式的 CSV 文本写入指定工作表的指定单元格位置。
只写纯值，不支持公式/样式/批注。=开头当文本处理。自动扩容行列。
目标区域如含合并单元格，csv-put 会打散合并并覆盖所有单元格；这不同于
range update 与合并区域冲突时返回 MERGED_CELLS_CONFLICT 的行为。

--csv 支持三种输入：直接传文本、@filepath 从文件读取、- 从 stdin 读取。
--allow-overwrite 默认 false，目标区域有数据时需显式传 --allow-overwrite 才能覆盖。`,
		Example: `  dws sheet csv-put --node NODE_ID --sheet-id SHEET_ID --start-cell A1 \
    --csv 'name,score\nalice,95\nbob,87'

  dws sheet csv-put --node NODE_ID --sheet-id SHEET_ID --start-cell B2 \
    --csv @data.csv --allow-overwrite

  cat data.csv | dws sheet csv-put --node NODE_ID --sheet-id SHEET_ID \
    --start-cell A1 --csv -

  dws sheet csv-put --node NODE_ID --sheet-id SHEET_ID --start-cell A1 \
    --csv @data.csv --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "csv", "start-cell"); err != nil {
				return err
			}
			csvContent := mustGetFlag(cmd, "csv")
			switch {
			case csvContent == "-":
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("读取 stdin 失败: %w", err)
				}
				csvContent = string(data)
			case strings.HasPrefix(csvContent, "@"):
				data, err := os.ReadFile(strings.TrimPrefix(csvContent, "@"))
				if err != nil {
					return fmt.Errorf("读取 CSV 文件失败: %w", err)
				}
				csvContent = string(data)
			}
			csvContent = strings.ReplaceAll(csvContent, "\r", "")
			csvContent = strings.TrimPrefix(csvContent, "\xef\xbb\xbf")
			toolArgs := map[string]any{
				"nodeId":    mustGetFlag(cmd, "node"),
				"sheetId":   mustGetFlag(cmd, "sheet-id"),
				"csv":       csvContent,
				"startCell": mustGetFlag(cmd, "start-cell"),
			}
			if cmd.Flags().Changed("allow-overwrite") {
				v, _ := cmd.Flags().GetBool("allow-overwrite")
				toolArgs["allowOverwrite"] = v
			}
			return callMCPTool("set_range_from_csv", toolArgs)
		},
	}
	csvPutCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	csvPutCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	csvPutCmd.Flags().String("csv", "", "CSV 文本、@文件路径 或 - 表示 stdin (必填)")
	csvPutCmd.Flags().String("start-cell", "", "起始单元格，A1 表示法 (必填)")
	csvPutCmd.Flags().Bool("allow-overwrite", false, "允许覆盖已有数据 (默认 false)")

	csvGetCmd := &cobra.Command{
		Use:   "csv-get",
		Short: "以 CSV 格式读取工作表数据",
		Long: `以 RFC 4180 CSV 格式读取钉钉电子表格数据。

返回内容：
  csv            带 [row=N] 行号前缀的 CSV 文本
  colIndices     列字母映射数组（定位列用 colIndices[j]，禁止手数逗号）
  rowIndices     行号映射数组
  hasMore        是否因 maxChars 截断

取值模式（--value-render-option）：
  formatted_value  格式化后的展示值（默认），如 ¥1,000.00、2025-06-01
  raw_value        原始值，如 1000、45808
  formula          公式文本，如 =SUM(A1:A10)，无公式时回退原始值

与 range read 的区别：
  - CSV 格式 token 消耗约为 JSON 的 1/3
  - 支持选择取值模式
  - 自动防爆（max-chars 截断 + has_more 标志）
  - [row=N] 前缀防止行号计算错误

注意：csv-get 不返回合并单元格结构。查看合并范围请使用
dws sheet info --node NODE_ID --sheet-id SHEET_ID --format json，并读取 mergedRanges。`,
		Example: `  dws sheet csv-get --node NODE_ID
  dws sheet csv-get --node NODE_ID --sheet-id SHEET_ID --range "A1:D10"
  dws sheet csv-get --node NODE_ID --range "A1:Z500" --value-render-option raw_value
  dws sheet csv-get --node NODE_ID --range "A1:D10" --max-chars 50000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"nodeId": mustGetFlag(cmd, "node"),
			}
			if v, _ := cmd.Flags().GetString("sheet-id"); v != "" {
				toolArgs["sheetId"] = v
			}
			if v, _ := cmd.Flags().GetString("range"); v != "" {
				toolArgs["range"] = v
			}
			if v, _ := cmd.Flags().GetString("value-render-option"); v != "" {
				toolArgs["valueRenderOption"] = v
			}
			if cmd.Flags().Changed("max-chars") {
				v, _ := cmd.Flags().GetInt("max-chars")
				toolArgs["maxChars"] = v
			}
			return callMCPTool("get_range_as_csv", toolArgs)
		},
	}
	csvGetCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	csvGetCmd.Flags().String("sheet-id", "", "工作表 ID 或名称")
	csvGetCmd.Flags().String("range", "", "读取范围，A1 表示法 (不传则读取全部非空数据)")
	csvGetCmd.Flags().String("value-render-option", "", "取值模式: formatted_value | raw_value | formula")
	csvGetCmd.Flags().Int("max-chars", 0, "CSV 最大字符数 (默认 200000)")

	return []*cobra.Command{findCmd, replaceCmd, appendCmd, csvPutCmd, csvGetCmd}
}
