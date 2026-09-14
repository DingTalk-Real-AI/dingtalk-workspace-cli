package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// ============================================================================
// set-style / batch-set-style
//
// 方案 A（对齐飞书 +cells-set-style）：样式统一走 set_cell_range 的 cellStyles 路径，
// 每个单元格产出 {"cellStyles":{...}}（不写值，保留原值）；纯样式写入对合并单元格安全
// （底层 setComplexValues 仅对写值 cell 拦截合并冲突）。
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

var (
	hAlignEnum     = map[string]bool{"left": true, "center": true, "right": true, "general": true}
	vAlignEnum     = map[string]bool{"top": true, "middle": true, "bottom": true}
	fontWeightEnum = map[string]bool{"bold": true, "normal": true}
	wordWrapEnum   = map[string]bool{"overflow": true, "clip": true, "autoWrap": true}
	fontStyleEnum  = map[string]bool{"normal": true, "italic": true}
	fontLineEnum   = map[string]bool{"none": true, "underline": true, "line-through": true}
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
	// 字体样式扩展（whole-range 标量）
	FontStyle  string `json:"fontStyle,omitempty"`  // normal / italic
	FontLine   string `json:"fontLine,omitempty"`   // none / underline / line-through
	FontFamily string `json:"fontFamily,omitempty"` // 字体族名，如 Arial
	// 四边边框 JSON（whole-range，per-cell 应用=网格），形如
	// {"top":{"style":"solid","color":"#000"},"bottom":{...},"left":{...},"right":{...}}
	BorderStylesJSON string `json:"borderStylesJson,omitempty"`
}

// borderStyleEnum 为引擎 BorderStyle 枚举（粗细已含在 style 内：solid=细/medium=中/thick=粗）。
var borderStyleEnum = map[string]bool{
	"none": true, "dotted": true, "dashed": true, "solid": true, "medium": true,
	"thick": true, "double": true, "hair": true, "dashDotDot": true, "dashDot": true,
	"mediumDashDotDot": true, "slantDashDot": true, "mediumDashDot": true, "mediumDashed": true,
}

var borderEdgeEnum = map[string]bool{"top": true, "bottom": true, "left": true, "right": true}

// borderEdgeFields 是每条边的合法字段：服务端 borderStyles 的边对象只有这两个。
var borderEdgeFields = []string{"style", "color"}

// parseBorderStyles 解析 --border-styles-json，校验边名与 style 枚举，返回可注入 cells 的 borderStyles 对象。
func parseBorderStyles(jsonStr string) (map[string]any, error) {
	if jsonStr == "" {
		return nil, nil
	}
	var raw map[string]map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("--border-styles-json 解析失败: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("--border-styles-json 不能为空对象")
	}
	out := map[string]any{}
	for edge, spec := range raw {
		if !borderEdgeEnum[edge] {
			return nil, fmt.Errorf("--border-styles-json 非法边名: %s（合法: top/bottom/left/right）", edge)
		}
		// 每条边只认 style / color。写错的键（colour）或类型不对的 color（123）
		// 此前被静默忽略：命令报成功，却画出一条没有颜色的边框。这与 --sheets /
		// --styles 拒绝未知键是同一条不变量——部分丢样式比直接报错难发现得多。
		if err := rejectUnknownFields(spec, "边框每条边", borderEdgeFields); err != nil {
			return nil, fmt.Errorf("--border-styles-json.%s: %w", edge, err)
		}
		styleVal, hasStyle := spec["style"]
		if !hasStyle || styleVal == nil {
			return nil, fmt.Errorf("--border-styles-json.%s 缺少 style", edge)
		}
		style, isStr := styleVal.(string)
		if !isStr {
			return nil, fmt.Errorf("--border-styles-json.%s.style 必须是字符串，实际是 %T", edge, styleVal)
		}
		if style == "" {
			return nil, fmt.Errorf("--border-styles-json.%s 缺少 style", edge)
		}
		if !borderStyleEnum[style] {
			return nil, fmt.Errorf("--border-styles-json.%s.style 非法: %s（合法: solid/medium/thick/dashed/dotted/double/hair/none/...）", edge, style)
		}
		edgeObj := map[string]any{"style": style}
		if colorVal, ok := spec["color"]; ok && colorVal != nil {
			color, isStr := colorVal.(string)
			if !isStr {
				return nil, fmt.Errorf("--border-styles-json.%s.color 必须是字符串，实际是 %T", edge, colorVal)
			}
			if color == "" {
				return nil, fmt.Errorf("--border-styles-json.%s.color 不能为空字符串（不需要颜色就整个省掉该字段）", edge)
			}
			edgeObj["color"] = color
		}
		out[edge] = edgeObj
	}
	return out, nil
}

// batchItem 是 batch-set-style 批次配置里的一项：嵌入 styleSpec，额外必填 sheetId + range。
type batchItem struct {
	SheetID string `json:"sheetId"`
	Range   string `json:"range"`
	styleSpec
}

// strGrid 返回一个 getter：给定 (i,j) 返回该格的字符串样式值与是否生效。
// scalar 与 *-json 二选一；scalar 作用整个 range；json 逐格（空串表示该格不设置）。
func strGrid(scalar, jsonStr, flagName string, enum map[string]bool, rows, cols int) (func(i, j int) (string, bool), error) {
	if scalar != "" && jsonStr != "" {
		return nil, fmt.Errorf("--%s 与 --%s-json 不能同时指定", flagName, flagName)
	}
	if scalar != "" {
		if enum != nil && !enum[scalar] {
			return nil, fmt.Errorf("--%s 枚举非法: %s", flagName, scalar)
		}
		return func(i, j int) (string, bool) { return scalar, true }, nil
	}
	if jsonStr == "" {
		return nil, nil
	}
	var m [][]string
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return nil, fmt.Errorf("--%s-json 解析失败: %w", flagName, err)
	}
	if err := checkMatrixShape(len(m), maxColLenStr(m), rows, cols, flagName+"-json"); err != nil {
		return nil, err
	}
	if enum != nil {
		for _, row := range m {
			for _, v := range row {
				if v != "" && !enum[v] {
					return nil, fmt.Errorf("--%s-json 包含非法枚举值: %s", flagName, v)
				}
			}
		}
	}
	return func(i, j int) (string, bool) {
		if i < len(m) && j < len(m[i]) && m[i][j] != "" {
			return m[i][j], true
		}
		return "", false
	}, nil
}

// intGrid 同 strGrid，用于 fontSize（0 表示未设置）。
func intGrid(scalar int, jsonStr, flagName string, rows, cols int) (func(i, j int) (int, bool), error) {
	if scalar != 0 && jsonStr != "" {
		return nil, fmt.Errorf("--%s 与 --%s-json 不能同时指定", flagName, flagName)
	}
	if scalar != 0 {
		if scalar < 0 {
			return nil, fmt.Errorf("--%s 必须为正整数", flagName)
		}
		return func(i, j int) (int, bool) { return scalar, true }, nil
	}
	if jsonStr == "" {
		return nil, nil
	}
	var m [][]int
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return nil, fmt.Errorf("--%s-json 解析失败: %w", flagName, err)
	}
	if err := checkMatrixShape(len(m), maxColLen2D(m), rows, cols, flagName+"-json"); err != nil {
		return nil, err
	}
	return func(i, j int) (int, bool) {
		if i < len(m) && j < len(m[i]) && m[i][j] != 0 {
			return m[i][j], true
		}
		return 0, false
	}, nil
}

// buildStyleCells 依据 styleSpec 生成 set_cell_range 的 cells 矩阵（每格 {cellStyles:{...}} 或 {} 跳过）。
func buildStyleCells(spec *styleSpec, rows, cols int) ([][]any, error) {
	if rows <= 0 || cols <= 0 {
		return nil, fmt.Errorf("range 行列数必须大于 0")
	}
	if rows > 1000 {
		return nil, fmt.Errorf("单次样式更新行数上限为 1000（当前 %d 行）", rows)
	}
	if rows*cols > 30000 {
		return nil, fmt.Errorf("单次样式更新单元格总数上限为 30000（当前 %d×%d=%d）", rows, cols, rows*cols)
	}

	bgGet, err := strGrid(spec.BgColor, spec.BgColorsJSON, "bg-color", nil, rows, cols)
	if err != nil {
		return nil, err
	}
	sizeGet, err := intGrid(spec.FontSize, spec.FontSizesJSON, "font-size", rows, cols)
	if err != nil {
		return nil, err
	}
	hGet, err := strGrid(spec.HAlign, spec.HAlignsJSON, "h-align", hAlignEnum, rows, cols)
	if err != nil {
		return nil, err
	}
	vGet, err := strGrid(spec.VAlign, spec.VAlignsJSON, "v-align", vAlignEnum, rows, cols)
	if err != nil {
		return nil, err
	}
	fcGet, err := strGrid(spec.FontColor, spec.FontColorsJSON, "font-color", nil, rows, cols)
	if err != nil {
		return nil, err
	}
	fwGet, err := strGrid(spec.FontWeight, spec.FontWeightsJSON, "font-weight", fontWeightEnum, rows, cols)
	if err != nil {
		return nil, err
	}

	// whole-range 标量样式
	if spec.WordWrap != "" && !wordWrapEnum[spec.WordWrap] {
		return nil, fmt.Errorf("--word-wrap 枚举非法: %s（合法值: overflow / clip / autoWrap）", spec.WordWrap)
	}
	if spec.FontStyle != "" && !fontStyleEnum[spec.FontStyle] {
		return nil, fmt.Errorf("--font-style 枚举非法: %s（合法值: normal / italic）", spec.FontStyle)
	}
	if spec.FontLine != "" && !fontLineEnum[spec.FontLine] {
		return nil, fmt.Errorf("--font-line 枚举非法: %s（合法值: none / underline / line-through）", spec.FontLine)
	}
	borderObj, err := parseBorderStyles(spec.BorderStylesJSON)
	if err != nil {
		return nil, err
	}

	anySet := bgGet != nil || sizeGet != nil || hGet != nil || vGet != nil || fcGet != nil || fwGet != nil ||
		spec.WordWrap != "" || spec.NumberFormat != "" || spec.FontStyle != "" || spec.FontLine != "" || spec.FontFamily != "" ||
		borderObj != nil
	if !anySet {
		return nil, fmt.Errorf("至少需要指定一个样式参数（--bg-color / --font-size / --h-align / --v-align / --font-color / --font-weight / --word-wrap / --number-format / --font-style / --font-line / --font-family / --border-styles-json 或对应的 *-json 形式）")
	}

	cells := make([][]any, rows)
	for i := 0; i < rows; i++ {
		row := make([]any, cols)
		for j := 0; j < cols; j++ {
			cs := map[string]any{}
			if bgGet != nil {
				if v, ok := bgGet(i, j); ok {
					cs["backgroundColor"] = v
				}
			}
			if sizeGet != nil {
				if v, ok := sizeGet(i, j); ok {
					cs["fontSize"] = v
				}
			}
			if hGet != nil {
				if v, ok := hGet(i, j); ok {
					cs["horizontalAlignment"] = v
				}
			}
			if vGet != nil {
				if v, ok := vGet(i, j); ok {
					cs["verticalAlignment"] = v
				}
			}
			if fcGet != nil {
				if v, ok := fcGet(i, j); ok {
					cs["fontColor"] = v
				}
			}
			if fwGet != nil {
				if v, ok := fwGet(i, j); ok {
					cs["fontWeight"] = v
				}
			}
			if spec.WordWrap != "" {
				cs["wordWrap"] = spec.WordWrap
			}
			if spec.NumberFormat != "" {
				cs["numberFormat"] = spec.NumberFormat
			}
			if spec.FontStyle != "" {
				cs["fontStyle"] = spec.FontStyle
			}
			// font-line 作为单选：设定 textUnderline / textLineThrough 两个布尔
			switch spec.FontLine {
			case "underline":
				cs["textUnderline"] = true
				cs["textLineThrough"] = false
			case "line-through":
				cs["textUnderline"] = false
				cs["textLineThrough"] = true
			case "none":
				cs["textUnderline"] = false
				cs["textLineThrough"] = false
			}
			if spec.FontFamily != "" {
				cs["fontFamily"] = spec.FontFamily
			}

			cell := map[string]any{}
			if len(cs) > 0 {
				cell["cellStyles"] = cs
			}
			if borderObj != nil {
				cell["borderStyles"] = borderObj // 整区共用，per-cell 应用（网格）
			}
			row[j] = cell // 空 map = {} 跳过（保留原值）
		}
		cells[i] = row
	}
	return cells, nil
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
	spec.FontStyle, _ = cmd.Flags().GetString("font-style")
	spec.FontLine, _ = cmd.Flags().GetString("font-line")
	spec.FontFamily, _ = cmd.Flags().GetString("font-family")
	spec.BorderStylesJSON, _ = cmd.Flags().GetString("border-styles-json")
	return spec
}

// styleFlagNames 是 bindStyleFlags 绑定的全部样式 flag。
// set-style 与 batch-set-style 共用这套 flag，但 batch 的 --batch 模式下样式来自
// 配置文件，需要据此判断用户是否传了不会生效的样式 flag。
// 新增样式 flag 必须同步登记（TestStyleFlagNamesMatchBoundFlags 会盯住漂移）。
var styleFlagNames = []string{
	"bg-color", "bg-colors-json",
	"border-styles-json",
	"font-color", "font-colors-json",
	"font-family", "font-line",
	"font-size", "font-sizes-json",
	"font-style",
	"font-weight", "font-weights-json",
	"h-align", "h-aligns-json",
	"number-format",
	"v-align", "v-aligns-json",
	"word-wrap",
}

// changedStyleFlags 返回用户显式设置过的样式 flag（带 -- 前缀，顺序稳定）。
func changedStyleFlags(cmd *cobra.Command) []string {
	var out []string
	for _, name := range styleFlagNames {
		if cmd.Flags().Changed(name) {
			out = append(out, "--"+name)
		}
	}
	return out
}

// bindStyleFlags 绑定共用的样式 flag。
// 说明文案不能写死 --range：set-style 用 --range，batch-set-style 用 --ranges。
func bindStyleFlags(cmd *cobra.Command) {
	cmd.Flags().String("bg-color", "", "背景色（#RRGGBB），一键刷满目标区域；与 --bg-colors-json 二选一")
	cmd.Flags().String("bg-colors-json", "", "背景色二维 JSON 数组，维度需与目标区域一致")
	cmd.Flags().Int("font-size", 0, "字号，一键刷满目标区域；与 --font-sizes-json 二选一")
	cmd.Flags().String("font-sizes-json", "", "字号二维 JSON 数组，维度需与目标区域一致")
	cmd.Flags().String("h-align", "", "水平对齐（left/center/right/general），一键刷满目标区域")
	cmd.Flags().String("h-aligns-json", "", "水平对齐二维 JSON 数组，维度需与目标区域一致")
	cmd.Flags().String("v-align", "", "垂直对齐（top/middle/bottom），一键刷满目标区域")
	cmd.Flags().String("v-aligns-json", "", "垂直对齐二维 JSON 数组，维度需与目标区域一致")
	cmd.Flags().String("font-color", "", "字体颜色（#RRGGBB），一键刷满目标区域")
	cmd.Flags().String("font-colors-json", "", "字体颜色二维 JSON 数组，维度需与目标区域一致")
	cmd.Flags().String("font-weight", "", "字体粗细（bold/normal），一键刷满目标区域")
	cmd.Flags().String("font-weights-json", "", "字体粗细二维 JSON 数组，维度需与目标区域一致")
	cmd.Flags().String("word-wrap", "", "换行方式（overflow/clip/autoWrap），整个目标区域共用")
	cmd.Flags().String("number-format", "", "数字格式 code（常用 General/@/#,##0/#,##0.00/0%/0.00%/yyyy/m/d/h:mm:ss；@ 为文本，适合长数字 ID）")
	cmd.Flags().String("font-style", "", "字体样式（normal/italic），整个目标区域共用")
	cmd.Flags().String("font-line", "", "字体线条（none/underline/line-through），整个目标区域共用")
	cmd.Flags().String("font-family", "", "字体族（如 Arial / 微软雅黑），整个目标区域共用")
	cmd.Flags().String("border-styles-json", "", `四边边框 JSON（整区共用，逐格应用=网格），如 '{"top":{"style":"solid","color":"#000"},"bottom":{"style":"solid"}}'；style 取 solid/medium/thick/dashed/dotted/double/none 等（粗细含在 style 内）；每条边只接受 style / color 两个键，写错的键或非字符串 color 直接报错`)
}

// newRangeSetStyleCmd 构造 dws sheet range set-style 命令（走 set_cell_range 的 cellStyles 路径）。
func newRangeSetStyleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-style",
		Short: "设置指定单元格区域的样式",
		Long: `为指定 range 设置背景色 / 字号 / 对齐 / 字体颜色 / 字体粗细 / 换行 / 数字格式 / 斜体 / 下划线删除线 / 字体族。
底层走 set_cell_range 的 cellStyles（仅设样式、保留原值），纯样式写入可作用于含合并单元格的区域。

每个样式维度（bg-color/font-size/h-align/v-align/font-color/font-weight）提供两种用法，二选一：
  --xxx             单值，一键刷整个 range
  --xxx-json        二维 JSON 数组，逐单元格指定，维度需与 --range 一致

整区共用的标量样式（无 *-json 形式）：
  --word-wrap    overflow/clip/autoWrap
  --number-format 数字格式 code
  --font-style   normal/italic（斜体）
  --font-line    none/underline/line-through（下划线/删除线，单选）
  --font-family  字体族名

单次调用建议：行数 ≤ 1000 且行×列 ≤ 30000（服务端硬限）。

数字格式（--number-format）传格式 code：
  General 常规；@ 文本；0/0.00 普通数字；#,##0/#,##0.00 千分位；0%/0.00% 百分比；yyyy/m/d 日期；h:mm/h:mm:ss 时间；货币格式如 "¥"#,##0.00。
  商品ID/订单号/手机号等数字形态标识符请设置 @，否则可能转为科学计数法。`,
		Example: `  # 给 A1:B3 打上黄底粗体居中
  dws sheet range set-style --node NODE_ID --sheet-id SHEET_ID --range "A1:B3" \
    --bg-color "#FFF2CC" --font-weight bold --h-align center

  # 斜体 + 下划线 + 指定字体
  dws sheet range set-style --node NODE_ID --sheet-id SHEET_ID --range "A1:A10" \
    --font-style italic --font-line underline --font-family "微软雅黑"

  # 给 C1:C5 逐单元格设置不同背景色
  dws sheet range set-style --node NODE_ID --sheet-id SHEET_ID --range "C1:C5" \
    --bg-colors-json '[["#FF0000"],["#00FF00"],["#0000FF"],["#FFFF00"],["#FF00FF"]]'

  # 长数字标识符按文本展示
  dws sheet range set-style --node NODE_ID --sheet-id SHEET_ID --range "A2:A100" --number-format "@"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "range"); err != nil {
				return err
			}
			node := mustGetFlag(cmd, "node")
			sheetID := mustGetFlag(cmd, "sheet-id")
			input := map[string]any{
				"sheet-id": sheetID,
				"range":    mustGetFlag(cmd, "range"),
			}
			for _, name := range styleFlagNames {
				if !cmd.Flags().Changed(name) {
					continue
				}
				if name == "font-size" {
					input[name], _ = cmd.Flags().GetInt(name)
				} else {
					input[name], _ = cmd.Flags().GetString(name)
				}
			}
			toolArgs, err := BuildBatchSetStyleArgs(input)
			if err != nil {
				return err
			}
			toolArgs["nodeId"] = node
			return callMCPTool("set_cell_range", toolArgs)
		},
	}
	cmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	cmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	cmd.Flags().String("range", "", "目标单元格区域地址，如 A1:B3 (必填)")
	bindStyleFlags(cmd)
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "range_set_style",
				CanonicalPath:  "sheet.range_set_style",
				CLIPath:        "sheet range set-style",
				PrimaryCLIPath: "sheet range set-style",
			},
			Description: "为指定范围设置背景、字体、对齐、换行、数字格式、斜体/下划线或边框。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "sheet", RPCName: "set_cell_range"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "为指定范围设置背景、字体、对齐、换行、数字格式、斜体/下划线或边框。",
				UseWhen:      []string{"需要批量刷样式或数字格式（百分比/货币/日期）时"},
				AvoidWhen:    []string{"写单元格值/公式用 range update；多区域样式用 range batch-set-style"},
				Examples:     []string{"dws sheet range set-style --node <NODE_ID> --sheet-id <SHEET_ID> --range \"B2:B10\" --number-format \"¥#,##0.00\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
				{Name: "range", Property: "rangeAddress"},
				{Name: "sheet-id", Property: "sheetId"},
			},
		},
	})
	return cmd
}

// newRangeBatchSetStyleCmd 构造 dws sheet range batch-set-style 命令。
// 组装为**一次** batch_update 调用（服务端原子事务），对齐飞书 +cells-batch-set-style。
func newRangeBatchSetStyleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch-set-style",
		Short: "批量设置样式（服务端原子事务）",
		Long: `批量为多个区域设置样式，CLI 组装为**一次** batch_update 调用，由服务端原子执行
（底层每项走 set_cell_range 的 cellStyles/borderStyles）。任一项失败默认整批回滚。

两种入口，二选一：

1) --ranges（对齐飞书 +cells-batch-set-style）：同一组样式应用到多个区域，支持跨工作表
   每项必须带工作表前缀（格式 "SheetName!A1:B3"），最多 100 项；样式用与 set-style 相同的 flag。

2) --batch <file>（每项可用不同样式）：JSON 数组，每个元素为一个批次项
[
  {
    "sheetId": "Sheet1",
    "range":   "A1:B3",
    "bgColor":    "#FFF2CC",
    "fontSize":   12,
    "hAlign":     "center",
    "fontWeight": "bold",
    "fontStyle":  "italic",
    "fontLine":   "underline",
    "fontFamily": "Arial",
    "borderStylesJson": "{\"top\":{\"style\":\"solid\"}}",
    "numberFormat": "General"
  },
  {
    "sheetId": "Sheet1",
    "range":   "C1:C5",
    "bgColorsJson": "[[\"#FF0000\"],[\"#00FF00\"],[\"#0000FF\"],[\"#FFFF00\"],[\"#FF00FF\"]]"
  }
]

每项执行与 set-style 一致的本地校验（至少一项样式字段 + rows≤1000 + rows×cols≤30000 + 枚举），
全部校验通过后才下发，避免部分生效。
批量上限：最多 100 个区域，且**所有区域累计**不超过 200000 个单元格（一次请求要把全部
单元格矩阵建好，累计量才是真正的峰值约束）。超出请拆成多次调用。
--continue-on-error 透传给服务端 batch_update：默认严格事务（失败整批回滚），传入后遇失败继续执行其余项。`,
		Example: `  # 同一组样式刷多个区域（可跨工作表），一次原子提交
  dws sheet range batch-set-style --node NODE_ID \
    --ranges '["Sheet1!A1:B2","Sheet2!D1:D10"]' \
    --bg-color "#FFF2CC" --font-weight bold --font-family "微软雅黑"

  # 每项不同样式：用配置文件
  dws sheet range batch-set-style --node NODE_ID --batch ./styles.json
  dws sheet range batch-set-style --node NODE_ID --batch ./styles.json --continue-on-error`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node"); err != nil {
				return err
			}
			node := mustGetFlag(cmd, "node")
			rangesStr, _ := cmd.Flags().GetString("ranges")
			batchPath, _ := cmd.Flags().GetString("batch")
			if rangesStr == "" && batchPath == "" {
				return fmt.Errorf("--ranges 与 --batch 至少提供一个")
			}
			if rangesStr != "" && batchPath != "" {
				return fmt.Errorf("--ranges 与 --batch 二选一，不能同时指定")
			}
			// --batch 模式每项样式来自配置文件；命令行样式 flag 不会生效，
			// 静默忽略会让用户以为刷上了，所以直接拒。
			if batchPath != "" {
				if used := changedStyleFlags(cmd); len(used) > 0 {
					return fmt.Errorf("--batch 模式下样式来自配置文件，%s 不会生效；请把样式写进配置文件，或改用 --ranges",
						strings.Join(used, " / "))
				}
			}

			var operations []any
			var err error
			if rangesStr != "" {
				operations, err = buildBatchStyleOpsFromRanges(cmd, rangesStr)
			} else {
				operations, err = buildBatchStyleOpsFromFile(batchPath)
			}
			if err != nil {
				return err
			}

			continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
			toolArgs, err := buildBatchUpdateToolArgsForCommand(node, operations, continueOnError)
			if err != nil {
				return err
			}
			return callMCPTool("batch_update", toolArgs)
		},
	}
	cmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	cmd.Flags().String("ranges", "", `目标区域 JSON 数组，每项须带工作表前缀且最多 100 项，如 '["Sheet1!A1:B2","Sheet2!D1:D10"]'（与 --batch 二选一，同一组样式应用到所有区域）`)
	cmd.Flags().String("batch", "", "批次配置 JSON 文件路径（与 --ranges 二选一，每项可用不同样式）")
	cmd.Flags().Bool("continue-on-error", false, "遇到失败时继续执行其余项（默认严格事务，整批回滚）")
	bindStyleFlags(cmd)
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "range_batch_set_style",
				CanonicalPath:  "sheet.range_batch_set_style",
				CLIPath:        "sheet range batch-set-style",
				PrimaryCLIPath: "sheet range batch-set-style",
			},
			Description: "批量为多个区域设置样式，组装为一次 batch_update 原子提交。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "The CLI assembles style cell matrices locally from --ranges or a local batch file and submits them as one JSON-encoded sheet/batch_update operation array; no single direct MCP interface represents the wrapper input shape.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "批量为多个区域设置样式，组装为一次 batch_update 原子提交。",
				UseWhen:      []string{"同一组样式要刷多个区域（可跨工作表），或多个区域样式不同需一次原子提交时"},
				AvoidWhen:    []string{"单一区域统一样式用 range set-style"},
				Examples:     []string{"dws sheet range batch-set-style --node <NODE_ID> --ranges '[\"Sheet1!A1:B2\"]' --bg-color \"#FFF2CC\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})
	return cmd
}

// maxBatchStyleRanges 与飞书 +cells-batch-set-style 的 --ranges 上限一致。
const maxBatchStyleRanges = 100

// maxBatchStyleCells 是一次 fan-out batch 累计可物化的单元格上限（对齐飞书
// maxStampMatrixCells / checkBatchStampBudget）。batch 会把每个 range 的 cells
// 矩阵全部建好再序列化成一个请求，所以真正的峰值约束是**跨 range 的总和**，
// 而不是 buildStyleCells 里的单区域上限：100 个区域各 30000 格 = 300 万个
// cell map。单区域上限继续由 buildStyleCells 按服务端硬限（30000）把关。
const maxBatchStyleCells = 200000

// addBatchStyleCells 累加待物化的单元格数，超限即报错。
// 必须在 buildStyleCells 分配矩阵**之前**调用，否则超限的那份已经建出来了。
func addBatchStyleCells(total *int64, rows, cols int) error {
	*total += int64(rows) * int64(cols)
	if *total > maxBatchStyleCells {
		return fmt.Errorf("所有区域累计展开 %d 个单元格，超过 %d 的安全上限；请减少区域数量或缩小区域范围", *total, maxBatchStyleCells)
	}
	return nil
}

// splitSheetPrefixedRange 拆分 "SheetName!A1:B3" → (sheetName, rangeAddr)。
// 两侧都必须在**修剪之后**仍非空：只按原始串里 ! 的位置判断会放过 " !A1:B2"，
// 修剪后工作表名成了空串，操作却照样带着 sheetId:"" 提交 —— 服务端要么让整批
// batch_update 失败，要么更危险地落到默认工作表而不是用户指定的那张表。
func splitSheetPrefixedRange(rng string, idx int) (string, string, error) {
	i := strings.Index(rng, "!")
	if i < 0 {
		return "", "", fmt.Errorf("--ranges[%d] (%q) 必须包含工作表前缀，格式为 \"SheetName!A1:B3\"", idx, rng)
	}
	sheetName := strings.TrimSpace(rng[:i])
	rangeAddr := strings.TrimSpace(rng[i+1:])
	if sheetName == "" || rangeAddr == "" {
		return "", "", fmt.Errorf("--ranges[%d] (%q) 必须包含工作表前缀，格式为 \"SheetName!A1:B3\"", idx, rng)
	}
	return sheetName, rangeAddr, nil
}

// buildBatchStyleOpsFromRanges 把「一组样式 + 多个带前缀 range」翻成 batch_update 的 operations。
func buildBatchStyleOpsFromRanges(cmd *cobra.Command, rangesStr string) ([]any, error) {
	var ranges []string
	if err := json.Unmarshal([]byte(rangesStr), &ranges); err != nil {
		return nil, fmt.Errorf("--ranges JSON 解析失败: %w\n  hint: --ranges 必须是 JSON 字符串数组，如 '[\"Sheet1!A1:B3\"]'", err)
	}
	if len(ranges) == 0 {
		return nil, fmt.Errorf("--ranges 不能为空数组")
	}
	if len(ranges) > maxBatchStyleRanges {
		return nil, fmt.Errorf("--ranges 最多 %d 项，当前 %d 项", maxBatchStyleRanges, len(ranges))
	}
	spec := readStyleSpecFromFlags(cmd)
	operations := make([]any, 0, len(ranges))
	var totalCells int64
	for i, rng := range ranges {
		sheetName, rangeAddr, err := splitSheetPrefixedRange(rng, i)
		if err != nil {
			return nil, err
		}
		rows, cols, err := parseA1Range(rangeAddr)
		if err != nil {
			return nil, fmt.Errorf("--ranges[%d] (%q) 解析失败: %w", i, rng, err)
		}
		if err := addBatchStyleCells(&totalCells, rows, cols); err != nil {
			return nil, fmt.Errorf("--ranges[%d] (%q): %w", i, rng, err)
		}
		cells, err := buildStyleCells(spec, rows, cols)
		if err != nil {
			return nil, fmt.Errorf("--ranges[%d] (%q): %w", i, rng, err)
		}
		operations = append(operations, map[string]any{
			"toolName": "set_cell_range",
			"input": map[string]any{
				"sheetId":      sheetName,
				"rangeAddress": rangeAddr,
				"cells":        cells,
			},
		})
	}
	return operations, nil
}

// buildBatchStyleOpsFromFile 把配置文件里的多条「range + 各自样式」翻成 batch_update 的 operations。
func buildBatchStyleOpsFromFile(batchPath string) ([]any, error) {
	data, err := os.ReadFile(batchPath)
	if err != nil {
		return nil, fmt.Errorf("读取 --batch 文件失败: %w", err)
	}
	var items []batchItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("--batch JSON 解析失败: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("--batch 配置为空")
	}
	total := len(items)
	if total > maxBatchStyleRanges {
		return nil, fmt.Errorf("--batch 最多 %d 条，当前 %d 条", maxBatchStyleRanges, total)
	}
	operations := make([]any, 0, total)
	var totalCells int64
	for i, item := range items {
		// 判空看修剪后的值："   " 这类纯空白与漏填等价，放过去就是一次 sheetId 为
		// 空白的 set_cell_range。但下发仍用原值：sheetId 可以是工作表**名**，名字
		// 允许带首尾空格，这里替用户修剪就会指向另一张表（或找不到表）。--ranges
		// 那条路径受 "Name!A1" 的书写形式所限只能修剪，需要精确名字时请走 --batch。
		if strings.TrimSpace(item.SheetID) == "" || strings.TrimSpace(item.Range) == "" {
			return nil, fmt.Errorf("--batch 第 %d/%d 条缺少 sheetId 或 range", i+1, total)
		}
		rows, cols, err := parseA1Range(item.Range)
		if err != nil {
			return nil, fmt.Errorf("--batch 第 %d/%d 条 range 解析失败: %w", i+1, total, err)
		}
		if err := addBatchStyleCells(&totalCells, rows, cols); err != nil {
			return nil, fmt.Errorf("--batch 第 %d/%d 条: %w", i+1, total, err)
		}
		spec := item.styleSpec
		cells, err := buildStyleCells(&spec, rows, cols)
		if err != nil {
			return nil, fmt.Errorf("--batch 第 %d/%d 条样式校验失败: %w", i+1, total, err)
		}
		operations = append(operations, map[string]any{
			"toolName": "set_cell_range",
			"input": map[string]any{
				"sheetId":      item.SheetID,
				"rangeAddress": item.Range,
				"cells":        cells,
			},
		})
	}
	return operations, nil
}
