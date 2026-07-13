package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// applyGlobalFilter applies the global --jq / --fields output filters to data
// when either is set. Returns handled=true when it wrote the (filtered) output,
// so callers skip their default JSON encoding. The filter helpers live in
// internal/output (the same ones used by `dws api`); the helper Formatter used
// by product commands previously ignored these flags, making them no-ops.
func (f *Formatter) applyGlobalFilter(data any) (handled bool, err error) {
	if deps == nil || deps.Caller == nil {
		return false, nil
	}
	jq := strings.TrimSpace(deps.Caller.JQ())
	fields := strings.TrimSpace(deps.Caller.Fields())
	if jq == "" && fields == "" {
		return false, nil
	}
	format := output.Format(strings.TrimSpace(deps.Caller.Format()))
	if format == "" {
		format = output.FormatJSON
	}
	return true, output.WriteFiltered(f.w, format, data, fields, jq)
}

// Formatter provides output formatting compatible with the old Wukong CLI.
type Formatter struct {
	w    io.Writer
	errW io.Writer
}

func NewFormatter() *Formatter {
	return &Formatter{w: os.Stdout, errW: os.Stderr}
}

// PrintJSON serializes data as pretty-printed JSON and writes it to the output stream.
// Go 的 json.Encoder 默认开启 HTML 转义（SetEscapeHTML(true)），会将 &、<、> 分别
// 转义为 \u0026、\u003c、\u003e。对于大多数 CLI 输出场景这是安全的默认行为。
// 如果返回值中包含 URL 等不应被转义的内容，请使用 PrintJSONUnescaped。
func (f *Formatter) PrintJSON(data any) error {
	if handled, err := f.applyGlobalFilter(data); handled {
		return err
	}
	enc := json.NewEncoder(f.w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// PrintJSONUnescaped 与 PrintJSON 功能相同，但禁用了 HTML 转义。
// 适用于返回值中包含带查询参数的 URL（如预签名上传 URL）的场景，
// 避免 & 被转义为 \u0026 导致 URL 无法直接使用。
//
// 使用场景示例：
//   - minutes upload create 返回的 presignedUrl 包含多个 & 分隔的查询参数
//   - 其他返回值中包含需要原样输出的 URL 的接口
//
// 影响范围：仅在调用方显式选择时生效，不影响全局 PrintJSON 的行为。
func (f *Formatter) PrintJSONUnescaped(data any) error {
	if handled, err := f.applyGlobalFilter(data); handled {
		return err
	}
	enc := json.NewEncoder(f.w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(data)
}

func (f *Formatter) PrintRaw(text string) {
	fmt.Fprint(f.w, text)
	if !strings.HasSuffix(text, "\n") {
		fmt.Fprintln(f.w)
	}
}

func runeWidth(s string) int {
	w := 0
	for _, r := range s {
		if r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a ||
			(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
			(r >= 0xac00 && r <= 0xd7a3) ||
			(r >= 0xf900 && r <= 0xfaff) ||
			(r >= 0xfe10 && r <= 0xfe19) ||
			(r >= 0xfe30 && r <= 0xfe6f) ||
			(r >= 0xff00 && r <= 0xff60) ||
			(r >= 0xffe0 && r <= 0xffe6) ||
			(r >= 0x20000 && r <= 0x2fffd) ||
			(r >= 0x30000 && r <= 0x3fffd)) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func padRight(s string, width int) string {
	rw := runeWidth(s)
	if rw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-rw)
}

func (f *Formatter) PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = runeWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && runeWidth(cell) > widths[i] {
				widths[i] = runeWidth(cell)
			}
		}
	}
	for i, h := range headers {
		fmt.Fprintf(f.w, "%s  ", padRight(h, widths[i]))
		_ = i
	}
	fmt.Fprintln(f.w)
	for _, w := range widths {
		fmt.Fprintf(f.w, "%s  ", strings.Repeat("-", w))
	}
	fmt.Fprintln(f.w)
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				fmt.Fprintf(f.w, "%s  ", padRight(cell, widths[i]))
			}
		}
		fmt.Fprintln(f.w)
	}
	fmt.Fprintf(f.w, "共 %d 条\n", len(rows))
}

func (f *Formatter) PrintSuccess(msg string) { fmt.Fprintf(f.w, "[OK] %s\n", msg) }
func (f *Formatter) PrintError(msg string)   { fmt.Fprintf(f.w, "[ERROR] %s\n", msg) }
func (f *Formatter) PrintWarning(msg string) { fmt.Fprintf(f.errW, "[WARN] %s\n", msg) }
func (f *Formatter) PrintInfo(msg string)    { fmt.Fprintf(f.w, "[INFO] %s\n", msg) }
func (f *Formatter) PrintDim(msg string)     { fmt.Fprintf(f.w, "  %s\n", msg) }

func (f *Formatter) PrintKeyValue(key, value string) {
	fmt.Fprintf(f.w, "%-16s%s\n", key+":", value)
}
