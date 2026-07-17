package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ============================================================================
// set-style / batch-set-style 辅助函数与命令（透传 update_range 样式字段）
// ============================================================================

// parseA1Range 解析 A1 表示法（支持 "Sheet1!A1:B3" / "A1:B3" / "A1"），返回 rows/cols。
func parseA1Range(addr string) (rows, cols int, err error) {
	if i := strings.Index(addr, "!"); i >= 0 {
		addr = addr[i+1:]
	}
	addr = strings.TrimSpace(strings.ToUpper(addr))
	if addr == "" {
		return 0, 0, fmt.Errorf("range 不能为空")
	}
	parts := strings.SplitN(addr, ":", 2)
	c1, r1, err := parseA1Cell(parts[0])
	if err != nil {
		return 0, 0, err
	}
	c2, r2 := c1, r1
	if len(parts) == 2 {
		c2, r2, err = parseA1Cell(parts[1])
		if err != nil {
			return 0, 0, err
		}
	}
	if c2 < c1 {
		c1, c2 = c2, c1
	}
	if r2 < r1 {
		r1, r2 = r2, r1
	}
	return r2 - r1 + 1, c2 - c1 + 1, nil
}

func parseA1Cell(s string) (col, row int, err error) {
	var i int
	for i = 0; i < len(s); i++ {
		c := s[i]
		if c < 'A' || c > 'Z' {
			break
		}
		col = col*26 + int(c-'A'+1)
	}
	if i == 0 || i == len(s) {
		return 0, 0, fmt.Errorf("无效单元格地址: %s", s)
	}
	for _, c := range s[i:] {
		if c < '0' || c > '9' {
			return 0, 0, fmt.Errorf("无效单元格地址: %s", s)
		}
		row = row*10 + int(c-'0')
	}
	if row == 0 || col == 0 {
		return 0, 0, fmt.Errorf("无效单元格地址: %s", s)
	}
	return col, row, nil
}

func fillStringMatrix(rows, cols int, v string) [][]string {
	m := make([][]string, rows)
	for i := range m {
		row := make([]string, cols)
		for j := range row {
			row[j] = v
		}
		m[i] = row
	}
	return m
}

func fillIntMatrix(rows, cols int, v int) [][]int {
	m := make([][]int, rows)
	for i := range m {
		row := make([]int, cols)
		for j := range row {
			row[j] = v
		}
		m[i] = row
	}
	return m
}

var (
	hAlignEnum     = map[string]bool{"left": true, "center": true, "right": true, "general": true}
	vAlignEnum     = map[string]bool{"top": true, "middle": true, "bottom": true}
	fontWeightEnum = map[string]bool{"bold": true, "normal": true}
	wordWrapEnum   = map[string]bool{"overflow": true, "clip": true, "autoWrap": true}
)

// styleSpec 描述一次 set-style 调用的样式参数（与 CLI flag / 批次 JSON 对齐）。
type styleSpec struct {
	BgColor         string `json:"bgColor,omitempty"`
	BgColorsJSON    string `json:"bgColorsJson,omitempty"`
	FontSize        int    `json:"fontSize,omitempty"`
	FontSizesJSON   string `json:"fontSizesJson,omitempty"`
	HAlign          string `json:"hAlign,omitempty"`
	HAlignsJSON     string `json:"hAlignsJson,omitempty"`
	VAlign          string `json:"vAlign,omitempty"`
	VAlignsJSON     string `json:"vAlignsJson,omitempty"`
	FontColor       string `json:"fontColor,omitempty"`
	FontColorsJSON  string `json:"fontColorsJson,omitempty"`
	FontWeight      string `json:"fontWeight,omitempty"`
	FontWeightsJSON string `json:"fontWeightsJson,omitempty"`
	WordWrap        string `json:"wordWrap,omitempty"`
	NumberFormat    string `json:"numberFormat,omitempty"`
}

// batchItem 是 batch-set-style 批次配置里的一项
// 嵌入 styleSpec，额外必填 sheetId + range。
type batchItem struct {
	SheetID string `json:"sheetId"`
	Range   string `json:"range"`
	styleSpec
}

// applyStyleSpec 将 styleSpec 铺到 toolArgs，同时做枚举/维度/上限校验。
func applyStyleSpec(spec *styleSpec, rows, cols int, toolArgs map[string]any) error {
	if rows <= 0 || cols <= 0 {
		return fmt.Errorf("range 行列数必须大于 0")
	}
	if rows > 1000 {
		return fmt.Errorf("单次样式更新行数上限为 1000（当前 %d 行）", rows)
	}
	if rows*cols > 30000 {
		return fmt.Errorf("单次样式更新单元格总数上限为 30000（当前 %d×%d=%d）", rows, cols, rows*cols)
	}
	if err := apply2DString(spec.BgColor, spec.BgColorsJSON, rows, cols, "bg-color", "backgroundColors", nil, toolArgs); err != nil {
		return err
	}
	if err := applyFontSize(spec, rows, cols, toolArgs); err != nil {
		return err
	}
	if err := apply2DString(spec.HAlign, spec.HAlignsJSON, rows, cols, "h-align", "horizontalAlignments", hAlignEnum, toolArgs); err != nil {
		return err
	}
	if err := apply2DString(spec.VAlign, spec.VAlignsJSON, rows, cols, "v-align", "verticalAlignments", vAlignEnum, toolArgs); err != nil {
		return err
	}
	if err := apply2DString(spec.FontColor, spec.FontColorsJSON, rows, cols, "font-color", "fontColors", nil, toolArgs); err != nil {
		return err
	}
	if err := apply2DString(spec.FontWeight, spec.FontWeightsJSON, rows, cols, "font-weight", "fontWeights", fontWeightEnum, toolArgs); err != nil {
		return err
	}
	if spec.WordWrap != "" {
		if !wordWrapEnum[spec.WordWrap] {
			return fmt.Errorf("--word-wrap 枚举非法: %s（合法值: overflow / clip / autoWrap）", spec.WordWrap)
		}
		toolArgs["wordWrap"] = spec.WordWrap
	}
	if spec.NumberFormat != "" {
		toolArgs["numberFormat"] = spec.NumberFormat
	}
	for _, k := range []string{"backgroundColors", "fontSizes", "horizontalAlignments", "verticalAlignments", "fontColors", "fontWeights", "wordWrap", "numberFormat"} {
		if _, ok := toolArgs[k]; ok {
			return nil
		}
	}
	return fmt.Errorf("至少需要指定一个样式参数（--bg-color / --font-size / --h-align / --v-align / --font-color / --font-weight / --word-wrap / --number-format 或对应的 *-json 形式）")
}

func applyFontSize(spec *styleSpec, rows, cols int, toolArgs map[string]any) error {
	if spec.FontSize != 0 && spec.FontSizesJSON != "" {
		return fmt.Errorf("--font-size 与 --font-sizes-json 不能同时指定")
	}
	if spec.FontSize != 0 {
		if spec.FontSize < 0 {
			return fmt.Errorf("--font-size 必须为正整数")
		}
		toolArgs["fontSizes"] = fillIntMatrix(rows, cols, spec.FontSize)
		return nil
	}
	if spec.FontSizesJSON != "" {
		var m [][]int
		if err := json.Unmarshal([]byte(spec.FontSizesJSON), &m); err != nil {
			return fmt.Errorf("--font-sizes-json 解析失败: %w", err)
		}
		if err := checkMatrixShape(len(m), maxColLen2D(m), rows, cols, "font-sizes-json"); err != nil {
			return err
		}
		toolArgs["fontSizes"] = m
	}
	return nil
}

// apply2DString 处理一组 scalar + *-json 样式 flag，含枚举校验。传 enum=nil 跳过枚举校验。
func apply2DString(scalar, jsonStr string, rows, cols int, flagName, toolKey string, enum map[string]bool, toolArgs map[string]any) error {
	if scalar != "" && jsonStr != "" {
		return fmt.Errorf("--%s 与 --%s-json 不能同时指定", flagName, flagName)
	}
	if scalar != "" {
		if enum != nil && !enum[scalar] {
			return fmt.Errorf("--%s 枚举非法: %s", flagName, scalar)
		}
		toolArgs[toolKey] = fillStringMatrix(rows, cols, scalar)
		return nil
	}
	if jsonStr == "" {
		return nil
	}
	var m [][]string
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return fmt.Errorf("--%s-json 解析失败: %w", flagName, err)
	}
	if err := checkMatrixShape(len(m), maxColLenStr(m), rows, cols, flagName+"-json"); err != nil {
		return err
	}
	if enum != nil {
		for _, row := range m {
			for _, v := range row {
				if v == "" {
					continue
				}
				if !enum[v] {
					return fmt.Errorf("--%s-json 包含非法枚举值: %s", flagName, v)
				}
			}
		}
	}
	toolArgs[toolKey] = m
	return nil
}

func maxColLenStr(m [][]string) int {
	max := 0
	for _, row := range m {
		if len(row) > max {
			max = len(row)
		}
	}
	return max
}

func maxColLen2D(m [][]int) int {
	max := 0
	for _, row := range m {
		if len(row) > max {
			max = len(row)
		}
	}
	return max
}

func checkMatrixShape(gotRows, gotCols, wantRows, wantCols int, flagName string) error {
	if gotRows != wantRows || gotCols != wantCols {
		return fmt.Errorf("--%s 维度与 range 不一致：期望 %d×%d，实际 %d×%d", flagName, wantRows, wantCols, gotRows, gotCols)
	}
	return nil
}

// readStyleSpecFromFlags 从 cobra flag 读取样式参数。
func readStyleSpecFromFlags(cmd *cobra.Command) *styleSpec {
	spec := &styleSpec{}
	spec.BgColor, _ = cmd.Flags().GetString("bg-color")
	spec.BgColorsJSON, _ = cmd.Flags().GetString("bg-colors-json")
	spec.FontSize, _ = cmd.Flags().GetInt("font-size")
	spec.FontSizesJSON, _ = cmd.Flags().GetString("font-sizes-json")
	spec.HAlign, _ = cmd.Flags().GetString("h-align")
	spec.HAlignsJSON, _ = cmd.Flags().GetString("h-aligns-json")
	spec.VAlign, _ = cmd.Flags().GetString("v-align")
	spec.VAlignsJSON, _ = cmd.Flags().GetString("v-aligns-json")
	spec.FontColor, _ = cmd.Flags().GetString("font-color")
	spec.FontColorsJSON, _ = cmd.Flags().GetString("font-colors-json")
	spec.FontWeight, _ = cmd.Flags().GetString("font-weight")
	spec.FontWeightsJSON, _ = cmd.Flags().GetString("font-weights-json")
	spec.WordWrap, _ = cmd.Flags().GetString("word-wrap")
	spec.NumberFormat, _ = cmd.Flags().GetString("number-format")
	return spec
}

// bindStyleFlags 绑定共用的样式 flag。
func bindStyleFlags(cmd *cobra.Command) {
	cmd.Flags().String("bg-color", "", "背景色（#RRGGBB），一键刷整个 range；与 --bg-colors-json 二选一")
	cmd.Flags().String("bg-colors-json", "", "背景色二维 JSON 数组，维度需与 --range 一致")
	cmd.Flags().Int("font-size", 0, "字号，一键刷整个 range；与 --font-sizes-json 二选一")
	cmd.Flags().String("font-sizes-json", "", "字号二维 JSON 数组，维度需与 --range 一致")
	cmd.Flags().String("h-align", "", "水平对齐（left/center/right/general），一键刷整个 range")
	cmd.Flags().String("h-aligns-json", "", "水平对齐二维 JSON 数组")
	cmd.Flags().String("v-align", "", "垂直对齐（top/middle/bottom），一键刷整个 range")
	cmd.Flags().String("v-aligns-json", "", "垂直对齐二维 JSON 数组")
	cmd.Flags().String("font-color", "", "字体颜色（#RRGGBB），一键刷整个 range")
	cmd.Flags().String("font-colors-json", "", "字体颜色二维 JSON 数组")
	cmd.Flags().String("font-weight", "", "字体粗细（bold/normal），一键刷整个 range")
	cmd.Flags().String("font-weights-json", "", "字体粗细二维 JSON 数组")
	cmd.Flags().String("word-wrap", "", "换行方式（overflow/clip/autoWrap），整个 range 共用")
	cmd.Flags().String("number-format", "", "数字格式 code（常用 General/@/#,##0/#,##0.00/0%/0.00%/yyyy/m/d/h:mm:ss；@ 为文本，适合长数字 ID）")
}

// newRangeSetStyleCmd 构造 dws sheet range set-style 命令。
func newRangeSetStyleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-style",
		Short: "设置指定单元格区域的样式",
		Long: `通过 update_range 透传样式字段，为指定 range 设置背景色 / 字号 / 对齐 / 字体颜色 / 字体粗细 / 换行方式 / 数字格式。

每个样式维度提供两种用法，二选一：
  --xxx             单值，一键刷整个 range（由 CLI 本地展开为二维数组）
  --xxx-json        二维 JSON 数组，逐单元格指定，维度需与 --range 一致

单次调用建议：行数 ≤ 1000 且行×列 ≤ 5000；服务端硬限 rows ≤ 1000 且 rows×cols ≤ 30000。

数字格式（--number-format）传格式 code：
  General 常规；@ 文本；0/0.00 普通数字；#,##0/#,##0.00 千分位；0%/0.00% 百分比；yyyy/m/d 日期；h:mm/h:mm:ss 时间；0.00E+00/##0.0E+0 科学计数；货币格式如 "¥"#,##0.00 或 $#,##0.00。
  商品ID、规格ID、订单号、手机号等数字形态标识符请设置 @，否则 General/默认展示可能转为科学计数法。
传入格式 code 字符串。`,
		Example: `  # 给 A1:B3 打上黄底粗体居中
  dws sheet range set-style --node NODE_ID --sheet-id SHEET_ID --range "A1:B3" \
    --bg-color "#FFF2CC" --font-weight bold --h-align center

  # 给 C1:C5 逐单元格设置不同背景色
  dws sheet range set-style --node NODE_ID --sheet-id SHEET_ID --range "C1:C5" \
    --bg-colors-json '[["#FF0000"],["#00FF00"],["#0000FF"],["#FFFF00"],["#FF00FF"]]'

  # 整片 range 启用自动换行
  dws sheet range set-style --node NODE_ID --sheet-id SHEET_ID --range "A1:E10" --word-wrap autoWrap

  # 长数字标识符按文本展示，避免科学计数法
  dws sheet range set-style --node NODE_ID --sheet-id SHEET_ID --range "A2:A100" --number-format "@"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "range"); err != nil {
				return err
			}
			node := mustGetFlag(cmd, "node")
			sheetID := mustGetFlag(cmd, "sheet-id")
			rangeAddr := mustGetFlag(cmd, "range")

			rows, cols, err := parseA1Range(rangeAddr)
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":       node,
				"sheetId":      sheetID,
				"rangeAddress": rangeAddr,
			}
			spec := readStyleSpecFromFlags(cmd)
			if err := applyStyleSpec(spec, rows, cols, toolArgs); err != nil {
				return err
			}
			return callMCPTool("update_range", toolArgs)
		},
	}
	cmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	cmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	cmd.Flags().String("range", "", "目标单元格区域地址，如 A1:B3 (必填)")
	bindStyleFlags(cmd)
	return cmd
}

// newRangeBatchSetStyleCmd 构造 dws sheet range batch-set-style 命令（方案 A：CLI 侧顺序循环）。
func newRangeBatchSetStyleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch-set-style",
		Short: "按配置文件批量设置样式",
		Long: `读取一份 JSON 配置，逐条调用 update_range 给不同 range 设置样式，CLI 内部顺序执行。

配置格式（JSON 数组，每个元素为一个批次项）：
[
  {
    "sheetId": "Sheet1",
    "range":   "A1:B3",
    "bgColor":    "#FFF2CC",
    "fontSize":   12,
    "hAlign":     "center",
    "vAlign":     "middle",
    "fontColor":  "#333333",
    "fontWeight": "bold",
    "wordWrap":   "autoWrap",
    "numberFormat": "General"
  },
  {
    "sheetId": "Sheet1",
    "range":   "C1:C5",
    "bgColorsJson": "[[\"#FF0000\"],[\"#00FF00\"],[\"#0000FF\"],[\"#FFFF00\"],[\"#FF00FF\"]]"
  }
]

每条记录执行与 set-style 一致的校验（至少一项样式字段 + rows≤1000 + rows×cols≤30000 + 枚举）。
某条失败时默认停止后续批次；要继续执行请加 --continue-on-error。`,
		Example: `  dws sheet range batch-set-style --node NODE_ID --batch ./styles.json
  dws sheet range batch-set-style --node NODE_ID --batch ./styles.json --continue-on-error`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "batch"); err != nil {
				return err
			}
			node := mustGetFlag(cmd, "node")
			batchPath := mustGetFlag(cmd, "batch")
			continueOnErr, _ := cmd.Flags().GetBool("continue-on-error")
			jsonMode := deps.Caller.Format() == "json"
			ctx := context.Background()
			var jsonResults []any

			data, err := os.ReadFile(batchPath)
			if err != nil {
				return fmt.Errorf("读取 --batch 文件失败: %w", err)
			}
			var items []batchItem
			if err := json.Unmarshal(data, &items); err != nil {
				return fmt.Errorf("--batch JSON 解析失败: %w", err)
			}
			if len(items) == 0 {
				return fmt.Errorf("--batch 配置为空")
			}

			total := len(items)
			var firstErr error
			var failed int
			for i, item := range items {
				if item.SheetID == "" || item.Range == "" {
					err := fmt.Errorf("第 %d/%d 条缺少 sheetId 或 range", i+1, total)
					fmt.Fprintln(os.Stderr, err)
					failed++
					if firstErr == nil {
						firstErr = err
					}
					if !continueOnErr {
						break
					}
					continue
				}
				rows, cols, err := parseA1Range(item.Range)
				if err != nil {
					err = fmt.Errorf("第 %d/%d 条 range 解析失败: %w", i+1, total, err)
					fmt.Fprintln(os.Stderr, err)
					failed++
					if firstErr == nil {
						firstErr = err
					}
					if !continueOnErr {
						break
					}
					continue
				}
				toolArgs := map[string]any{
					"nodeId":       node,
					"sheetId":      item.SheetID,
					"rangeAddress": item.Range,
				}
				if err := applyStyleSpec(&item.styleSpec, rows, cols, toolArgs); err != nil {
					err = fmt.Errorf("第 %d/%d 条样式校验失败: %w", i+1, total, err)
					fmt.Fprintln(os.Stderr, err)
					failed++
					if firstErr == nil {
						firstErr = err
					}
					if !continueOnErr {
						break
					}
					continue
				}
				fmt.Fprintf(os.Stderr, "[%d/%d] update_range sheet=%s range=%s\n", i+1, total, item.SheetID, item.Range)
				if deps.Caller.DryRun() {
					// JSON mode emits one aggregate preview below. Human formats
					// reuse the shared per-call preview printer.
					if !jsonMode {
						_ = callMCPTool("update_range", toolArgs)
					}
					if jsonMode {
						jsonResults = append(jsonResults, map[string]any{
							"index":     i + 1,
							"sheetId":   item.SheetID,
							"range":     item.Range,
							"ok":        true,
							"dryRun":    true,
							"tool":      "update_range",
							"arguments": toolArgs,
						})
					}
					continue
				}
				if jsonMode {
					text, cerr := callMCPToolReturnText(ctx, "update_range", toolArgs)
					entry := map[string]any{"index": i + 1, "sheetId": item.SheetID, "range": item.Range}
					if cerr != nil {
						entry["ok"] = false
						entry["error"] = cerr.Error()
						jsonResults = append(jsonResults, entry)
						cerr = fmt.Errorf("第 %d/%d 条 update_range 失败: %w", i+1, total, cerr)
						fmt.Fprintln(os.Stderr, cerr)
						failed++
						if firstErr == nil {
							firstErr = cerr
						}
						if !continueOnErr {
							break
						}
						continue
					}
					var parsed any
					if json.Unmarshal([]byte(text), &parsed) == nil {
						entry["result"] = parsed
					} else {
						entry["result"] = text
					}
					entry["ok"] = true
					jsonResults = append(jsonResults, entry)
					continue
				}
				if err := callMCPTool("update_range", toolArgs); err != nil {
					err = fmt.Errorf("第 %d/%d 条 update_range 失败: %w", i+1, total, err)
					fmt.Fprintln(os.Stderr, err)
					failed++
					if firstErr == nil {
						firstErr = err
					}
					if !continueOnErr {
						break
					}
				}
			}
			fmt.Fprintf(os.Stderr, "batch-set-style 完成：共 %d 条，失败 %d 条\n", total, failed)
			if jsonMode {
				if err := deps.Out.PrintJSON(map[string]any{
					"total":   total,
					"failed":  failed,
					"results": jsonResults,
					"success": failed == 0,
				}); err != nil {
					return err
				}
			}
			if failed > 0 && !continueOnErr {
				return firstErr
			}
			if failed > 0 {
				return fmt.Errorf("batch-set-style 共 %d 条失败（首个错误：%v）", failed, firstErr)
			}
			return nil
		},
	}
	cmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	cmd.Flags().String("batch", "", "批次配置 JSON 文件路径 (必填)")
	cmd.Flags().Bool("continue-on-error", false, "遇到失败时继续执行后续条目（默认遇错即停）")
	return cmd
}
