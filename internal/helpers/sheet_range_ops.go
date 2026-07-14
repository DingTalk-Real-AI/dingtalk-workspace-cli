package helpers

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// newRangeCmd creates the range parent command with its core children
// (read/update/clear/sort/fill/copy-to/move-to).
// set-style, batch-set-style, and batch-clear are added by newSheetCommand().
func newRangeCmd() *cobra.Command {
	rangeCmd := &cobra.Command{Use: "range", Short: "数据区域操作"}

	rangeReadCmd := &cobra.Command{
		Use:     "read",
		Aliases: []string{"get"},
		Short:   "读取工作表数据（别名: get）",
		Long: `读取钉钉电子表格中指定工作表的指定范围数据，返回 per-cell 结构化信息。

别名: dws sheet range get 与 dws sheet range read 等价。

返回内容为 cells 二维数组，每个 cell 包含：
  value           单元格值（内容由 --value-render-option 决定）
  dataValidation  数据验证配置（下拉列表/复选框），无则为 null
  hyperlink       单元格级超链接（path/sheet/range），无则省略

注意：range read/get 不返回合并单元格结构。查看合并范围请使用
dws sheet info --node NODE_ID --sheet-id SHEET_ID --format json，并读取 mergedRanges。

取值模式 (--value-render-option):
  formatted_value  格式化后的展示值（默认），如 ¥1,000.00、2025-06-01
  raw_value        原始值，如 1000、45808
  formula          公式文本，如 =SUM(A1:A10)，无公式时回退原始值

范围格式使用 A1 表示法：
  --range A1:D10            读取当前工作表 A1:D10 区域
  --range "Sheet1!A1:D10"   读取 Sheet1 工作表的 A1:D10 区域（忽略 --sheet-id）
  不传 --range 则默认读取整个工作表的全部非空数据

超时处理建议: 读取大范围数据时若出现超时或响应过慢，请主动缩小 --range 查询范围，
建议单次读取的单元格数量控制在 5000 个以内（例如 50 行 × 100 列、100 行 × 50 列）。
对于大表可采用分页读取策略：先通过 info 获取 nonEmptyRange.range
或 nonEmptyRange.lastRow / nonEmptyRange.lastColumn 确定 A1 边界；
再按行分批读取，避免不传 --range 直接读取整个大工作表。`,
		Example: `  dws sheet range read --node NODE_ID
  dws sheet range read --node NODE_ID --sheet-id SHEET_ID
  dws sheet range read --node NODE_ID --sheet-id "Sheet1" --range "A1:D10"
  dws sheet range read --node NODE_ID --range "Sheet1!A1:D10"
  dws sheet range read --node NODE_ID --value-render-option raw_value
  dws sheet range read --node NODE_ID --value-render-option formula

  # 使用 get 别名
  dws sheet range get --node NODE_ID --sheet-id SHEET_ID --range "A1:D10"`,
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
			return callMCPToolCellInfos(toolArgs)
		},
	}
	rangeReadCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	rangeReadCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (不传则默认第一个工作表)")
	rangeReadCmd.Flags().String("range", "", "读取范围，A1 表示法 (如 A1:D10，不传则读取全部数据)")
	rangeReadCmd.Flags().String("value-render-option", "", "取值模式: formatted_value(默认) | raw_value | formula")

	rangeUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新工作表指定区域内容",
		Long: `更新钉钉在线表格指定区域内单元格内容。

--values 二维 JSON 数组，行列维度需与 --range 完全一致。每个 cell 是以下之一：
  - {} 空对象：跳过该单元格，保留原值不变
  - {type:"text",...} 或 {type:"richText",...} 对象

【type=text】普通文本（可带 cellStyles）
  {"type":"text", "text":"内容"}
  {"type":"text", "text":"内容", "cellStyles":{"fontWeight":"bold","fontColor":"#FF0000"}}
  - text 以 "=" 开头识别为公式（如 "=SUM(B2:B4)"）
  - text 为空字符串 "" 表示清空该单元格
  - 写数字 / 布尔请用字符串形式（如 {"type":"text","text":"100"}），
    服务端按内容自动识别为数字 / 布尔

【{} 跳过】保留原值不变
  只更新部分单元格时，用 {} 占位不需要修改的位置，避免拆分成多次调用

【type=richText】富文本（多子项组合：超链接 / 附件 / 图片 / 带样式片段）
  {"type":"richText", "texts":[...]}
  texts[].type 枚举:
    • text       {"type":"text",       "text":"文字",   "style":{...}?}
    • link       {"type":"link",       "text":"显示文字","link":"https://...", "subType":"path"?, "style":{...}?}
    • attachment {"type":"attachment", "text":"文件名.pdf","resourceId":"...","mimeType":"application/pdf","size":12345?}
    • image      {"type":"image",      "text":"",       "resourceId":"...","resourceUrl":"/core/api/resources/img/xxx","width":?,"height":?}
  resourceId / resourceUrl 通过 dws sheet media-upload 获取。
  link.subType 可为 path/sheet/range；不传默认 path。sheet/range 必须使用真实工作表名称或 A1 范围，禁止猜 Sheet1。

【style 子结构】（仅 richText 的 text / link 子项支持）
  bold / italic / underline / strike: boolean
  color: 16 进制色值字符串如 "#FF0000"
  size:  字号正整数

【cellStyles 子结构】（可选，与 type 同级，作用于整格）
  fontWeight/fontColor/fontSize/fontStyle/backgroundColor/horizontalAlignment/verticalAlignment/wordWrap/numberFormat/textUnderline/textLineThrough
  只设样式不改值时，可传 {"cellStyles":{...}} 保留原值；大面积统一样式优先用 dws sheet range set-style。

【hyperlink 子结构】（可选，与 type 同级，作用于整格）
  1) 不传 hyperlink 字段                         → 保留原整格超链接
  2) {"hyperlink":{"type":"none"}}                → 显式清除整格超链接
  3) {"hyperlink":{"type":"path","link":"https://..."}}  → 写外部链接
     {"hyperlink":{"type":"sheet","link":"Sheet2"}}      → 写工作表链接
     {"hyperlink":{"type":"range","link":"Sheet2!A1"}}   → 写区域链接

【dataValidation 子结构】（可选，与 type 同级）三种语义：
  1) 不传 dataValidation 字段                      → 保留原 DV
  2) {"dataValidation":{"type":"none"}}             → 显式清除该单元格 DV
  3) {"dataValidation":{"type":"dropdown",...}}     → 写新 dropdown（覆盖）
     {"dataValidation":{"type":"checkbox",...}}    → 写新 checkbox（覆盖）
  dropdown: {"dataValidation":{"type":"dropdown","options":[{"value":"选项1"}],"enableMultiSelect":false}}
  checkbox: {"dataValidation":{"type":"checkbox","checked":true}}
  可与 text/richText 共存，也可单独使用（如 {dataValidation:{type:"none"}} 仅清除 DV 不写值）

注意：
  - 仅支持 text / richText 两种 type；number / boolean / null 等不再支持
  - type=text 顶层旧 style 字段不要作为新写法使用；整格样式用 cellStyles，片段样式用 richText 子项 style
  - 单元格级超链接写在 cell.hyperlink；richText link 仅表示富文本片段链接
  - 写图片到单元格建议直接用 dws sheet write-image（更简洁）
  - 只设样式或批量刷整片区域样式请用 dws sheet range set-style；写值同时设置少量 cell 样式可用 cellStyles
  - 目标范围与已有合并区域冲突时，range update 会返回 MERGED_CELLS_CONFLICT；先用 sheet info 查看 mergedRanges，取消合并后写入，必要时再重新合并
  - csv-put 的合并处理不同：目标区域含合并单元格时会打散合并并写入纯值
  - 清空整片区域请用 dws sheet range clear`,
		Example: `  # 写入文本
  dws sheet range update --node NODE_ID --sheet-id SHEET_ID --range "A1:B2" \
    --values '[[{"type":"text","text":"姓名"},{"type":"text","text":"分数"}],[{"type":"text","text":"张三"},{"type":"text","text":"90"}]]'

  # 写入公式
  dws sheet range update --node NODE_ID --sheet-id SHEET_ID --range "C2" \
    --values '[[{"type":"text","text":"=A2&B2"}]]'

  # 写入带整格样式的文本（红色加粗）
  dws sheet range update --node NODE_ID --sheet-id SHEET_ID --range "A1" \
    --values '[[{"type":"text","text":"重要","cellStyles":{"fontWeight":"bold","fontColor":"#FF0000"}}]]'

  # 写入单元格级超链接
  dws sheet range update --node NODE_ID --sheet-id SHEET_ID --range "A1" \
    --values '[[{"type":"text","text":"钉钉","hyperlink":{"type":"path","link":"https://dingtalk.com"}}]]'

  # 清理单元格级超链接，保留当前值
  dws sheet range update --node NODE_ID --sheet-id SHEET_ID --range "A1" \
    --values '[[{"hyperlink":{"type":"none"}}]]'

  # 富文本片段链接
  dws sheet range update --node NODE_ID --sheet-id SHEET_ID --range "A1" \
    --values '[[{"type":"richText","texts":[{"type":"link","text":"钉钉","link":"https://dingtalk.com"}]}]]'

  # 富文本：普通文字 + 带下划线的超链接
  dws sheet range update --node NODE_ID --sheet-id SHEET_ID --range "A1" \
    --values '[[{"type":"richText","texts":[{"type":"text","text":"请访问 "},{"type":"link","text":"官网","link":"https://dingtalk.com","style":{"underline":true,"color":"#0080FF"}}]}]]'

  # 清空单个单元格（text 为空字符串）
  dws sheet range update --node NODE_ID --sheet-id SHEET_ID --range "A1" \
    --values '[[{"type":"text","text":""}]]'

  # 清空整片区域请用 range clear
  dws sheet range clear --node NODE_ID --sheet-id SHEET_ID --range "A1:B3"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "range", "values"); err != nil {
				return err
			}
			valuesStr := mustGetFlag(cmd, "values")
			var values [][]any
			if err := json.Unmarshal([]byte(valuesStr), &values); err != nil {
				return fmt.Errorf("--values JSON 解析失败: %w", err)
			}
			for i, row := range values {
				for j, cell := range row {
					path := fmt.Sprintf("--values[%d][%d]", i, j)
					if cell == nil {
						return fmt.Errorf("%s: 不支持 null，每个单元格必须是 {type:text,...}、{type:richText,...}、{hyperlink:...} 对象，或 {} 表示保留原值；运行 'dws sheet range update --help' 查看完整形态与示例", path)
					}
					cellMap, ok := cell.(map[string]any)
					if !ok {
						return fmt.Errorf("%s: 不支持原始值 %v，每个单元格必须是 object（如 {\"type\":\"text\",\"text\":\"...\"}）；运行 'dws sheet range update --help' 查看完整形态与示例", path, cell)
					}
					if len(cellMap) == 0 {
						continue
					}
					if err := validateComplexValueCell(cellMap, path); err != nil {
						return err
					}
				}
			}
			return callMCPTool("set_cell_range", map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"rangeAddress": mustGetFlag(cmd, "range"),
				"cells":        values,
			})
		},
	}
	rangeUpdateCmd.Flags().String("node", "", "表格文档 ID (必填)")
	rangeUpdateCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	rangeUpdateCmd.Flags().String("range", "", "目标单元格区域地址，如 A1:B3 (必填)")
	rangeUpdateCmd.Flags().String("values", "", "单元格内容，二维 JSON 数组 (必填)；每个元素必须是 object：{type:text,text:...}、{type:richText,texts:[...]}、{dataValidation:...}、{cellStyles:...}、{hyperlink:...} 或 {}（详见 --help 长描述）")

	rangeClearCmd := &cobra.Command{
		Use:   "clear",
		Short: "清除工作表指定区域",
		Long: `清除钉钉电子表格中指定工作表指定范围的单元格内容、格式或全部。

清除类型 (--type):
  content  仅清除单元格的值，保留格式（默认）
  format   仅清除格式，保留值
  all      清除值和格式

范围格式使用 A1 表示法：
  --range A1:B3   清除 A1 到 B3 的矩形区域
  --range A1      清除单个单元格
  --range A:C     清除整列范围

该操作会删除内容或格式；必须先获得用户确认，再追加 --yes 执行。`,
		Example: `  dws sheet range clear --node NODE_ID --sheet-id SHEET_ID --range "A1:B3" --yes
  dws sheet range clear --node NODE_ID --sheet-id SHEET_ID --range "A1:B3" --type format --yes
  dws sheet range clear --node NODE_ID --sheet-id SHEET_ID --range "A1:B3" --type all --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "range"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"range":   mustGetFlag(cmd, "range"),
			}
			if v, _ := cmd.Flags().GetString("type"); v != "" {
				toolArgs["type"] = v
			}
			return callMCPTool("clear_range", toolArgs)
		},
	}
	rangeClearCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	rangeClearCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	rangeClearCmd.Flags().String("range", "", "清除范围，A1 表示法 (必填，如 A1:B3)")
	rangeClearCmd.Flags().String("type", "", "清除类型: content(仅值,默认) / format(仅格式) / all(全部)")

	rangeSortCmd := &cobra.Command{
		Use:   "sort",
		Short: "对工作表指定区域排序",
		Long: `对钉钉电子表格中指定工作表指定范围的数据进行排序。

支持多级排序（按 --sort-keys 数组顺序优先级递减）。
column 使用字母列名（如 "A"、"B"、"AA"），表示排序的目标列。

注意：排序前必须先 range read 排序范围的前 3-5 行，对比首行与后续行模式判断表头：
  - 首行全文本 + 后续行含数字/日期 → 加 --has-header
  - 首行与后续行模式一致 → 不加
  - 首行语义像列标题且与后续行明显不同 → 加 --has-header
  表头误排入数据不可撤销，禁止不读就排。`,
		Example: `  dws sheet range sort --node NODE_ID --sheet-id SHEET_ID --range "A1:D10" \
    --sort-keys '[{"column":"A","ascending":true}]'
  dws sheet range sort --node NODE_ID --sheet-id SHEET_ID --range "A1:D10" \
    --sort-keys '[{"column":"A","ascending":true},{"column":"C","ascending":false}]' --has-header`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "range", "sort-keys"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"range":   mustGetFlag(cmd, "range"),
			}
			sortKeysStr, _ := cmd.Flags().GetString("sort-keys")
			var sortKeys []any
			if err := json.Unmarshal([]byte(sortKeysStr), &sortKeys); err != nil {
				return fmt.Errorf("--sort-keys JSON 解析失败: %w", err)
			}
			toolArgs["sortKeys"] = sortKeys
			if v, _ := cmd.Flags().GetBool("has-header"); v {
				toolArgs["hasHeader"] = true
			}
			return callMCPTool("sort_range", toolArgs)
		},
	}
	rangeSortCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	rangeSortCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	rangeSortCmd.Flags().String("range", "", "排序范围，A1 表示法 (必填，如 A1:D10)")
	rangeSortCmd.Flags().String("sort-keys", "", `排序规则 JSON 数组 (必填，如 [{"column":"A","ascending":true}])`)
	rangeSortCmd.Flags().Bool("has-header", false, "首行是否为表头（不参与排序）")

	rangeFillCmd := &cobra.Command{
		Use:   "fill",
		Short: "自动填充工作表指定区域",
		Long: `基于源数据范围自动填充到目标范围。

目标范围须与源范围在行或列维度对齐（不支持对角填充）。

填充类型 (--fill-type):
  不传          自动检测（根据源数据智能判断：数值序列递增、日期递增、文本复制等）
  copy          复制内容和格式
  onlystyle     仅格式
  withoutstyle  仅值无格式`,
		Example: `  dws sheet range fill --node NODE_ID --sheet-id SHEET_ID \
    --source-range "A1:A5" --target-range "A6:A20"
  dws sheet range fill --node NODE_ID --sheet-id SHEET_ID \
    --source-range "A1:A5" --target-range "A6:A20" --fill-type copy`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "source-range", "target-range"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":           mustGetFlag(cmd, "node"),
				"sheetId":          mustGetFlag(cmd, "sheet-id"),
				"sourceRange":      mustGetFlag(cmd, "source-range"),
				"destinationRange": mustGetFlag(cmd, "target-range"),
			}
			if v, _ := cmd.Flags().GetString("fill-type"); v != "" {
				toolArgs["fillType"] = v
			}
			return callMCPTool("fill_range", toolArgs)
		},
	}
	rangeFillCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	rangeFillCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	rangeFillCmd.Flags().String("source-range", "", "源数据范围，A1 表示法 (必填，如 A1:A5)")
	rangeFillCmd.Flags().String("target-range", "", "目标填充范围，A1 表示法 (必填，如 A6:A20)")
	rangeFillCmd.Flags().String("fill-type", "", "填充类型: 不传则自动检测 / copy(复制) / onlystyle(仅格式) / withoutstyle(仅值)")

	rangeCopyCmd := &cobra.Command{
		Use:   "copy-to",
		Short: "复制工作表指定区域到目标位置",
		Long: `将源范围的数据复制到目标位置。支持跨工作表复制。

跨工作表复制的两种方式：
  1. --target-sheet-id "Sheet2"          显式指定目标工作表
  2. --target-range "Sheet2!A1"          在目标范围中携带工作表前缀

限制：
  - 同一工作表内源和目标范围不能重叠`,
		Example: `  dws sheet range copy-to --node NODE_ID --sheet-id SHEET_ID \
    --source-range "A1:C5" --target-range "D1"
  dws sheet range copy-to --node NODE_ID --sheet-id SHEET_ID \
    --source-range "A1:C5" --target-range "A1" --target-sheet-id Sheet2
  dws sheet range copy-to --node NODE_ID --sheet-id SHEET_ID \
    --source-range "A1:C5" --target-range "Sheet2!A1"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "source-range", "target-range"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":           mustGetFlag(cmd, "node"),
				"sheetId":          mustGetFlag(cmd, "sheet-id"),
				"sourceRange":      mustGetFlag(cmd, "source-range"),
				"destinationRange": mustGetFlag(cmd, "target-range"),
			}
			if v, _ := cmd.Flags().GetString("target-sheet-id"); v != "" {
				toolArgs["targetSheetId"] = v
			}
			if v, _ := cmd.Flags().GetString("paste-type"); v != "" {
				toolArgs["pasteType"] = v
			}
			return callMCPTool("copy_range", toolArgs)
		},
	}
	rangeCopyCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	rangeCopyCmd.Flags().String("sheet-id", "", "源工作表 ID 或名称 (必填)")
	rangeCopyCmd.Flags().String("source-range", "", "源范围，A1 表示法 (必填，如 A1:C5)")
	rangeCopyCmd.Flags().String("target-range", "", "目标位置，A1 表示法 (必填，如 D1；支持 Sheet2!A1 表示法指定目标工作表)")
	rangeCopyCmd.Flags().String("target-sheet-id", "", "目标工作表 ID 或名称（可选，不传则复制到同一工作表）")
	rangeCopyCmd.Flags().String("paste-type", "", "粘贴类型: values(仅值) / formulas(仅公式) / formats(仅格式) / all(全部,默认)")

	rangeMoveCmd := &cobra.Command{
		Use:   "move-to",
		Short: "移动工作表指定区域到目标位置",
		Long: `将源范围的数据移动到目标位置，源区域将被清空。支持跨工作表移动。

跨工作表移动的两种方式：
  1. --target-sheet-id "Sheet2"          显式指定目标工作表
  2. --target-range "Sheet2!A1"          在目标范围中携带工作表前缀

限制：
  - 同一工作表内源和目标范围不能重叠`,
		Example: `  dws sheet range move-to --node NODE_ID --sheet-id SHEET_ID \
    --source-range "A1:C5" --target-range "D1" --yes
  dws sheet range move-to --node NODE_ID --sheet-id SHEET_ID \
    --source-range "A1:C5" --target-range "A1" --target-sheet-id Sheet2 --yes
  dws sheet range move-to --node NODE_ID --sheet-id SHEET_ID \
    --source-range "A1:C5" --target-range "Sheet2!A1" --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "source-range", "target-range"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":           mustGetFlag(cmd, "node"),
				"sheetId":          mustGetFlag(cmd, "sheet-id"),
				"sourceRange":      mustGetFlag(cmd, "source-range"),
				"destinationRange": mustGetFlag(cmd, "target-range"),
			}
			if v, _ := cmd.Flags().GetString("target-sheet-id"); v != "" {
				toolArgs["targetSheetId"] = v
			}
			return callMCPTool("move_range", toolArgs)
		},
	}
	rangeMoveCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	rangeMoveCmd.Flags().String("sheet-id", "", "源工作表 ID 或名称 (必填)")
	rangeMoveCmd.Flags().String("source-range", "", "源范围，A1 表示法 (必填，如 A1:C5)")
	rangeMoveCmd.Flags().String("target-range", "", "目标位置，A1 表示法 (必填，如 D1；支持 Sheet2!A1 表示法指定目标工作表)")
	rangeMoveCmd.Flags().String("target-sheet-id", "", "目标工作表 ID 或名称（可选，不传则移动到同一工作表）")

	rangeCmd.AddCommand(rangeReadCmd, rangeUpdateCmd, rangeClearCmd, rangeSortCmd, rangeFillCmd, rangeCopyCmd, rangeMoveCmd)
	return rangeCmd
}
