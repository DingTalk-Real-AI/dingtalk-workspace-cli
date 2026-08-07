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
	return NewFormatterWithWriters(os.Stdout, os.Stderr)
}

// NewFormatterWithWriters 是注入式构造函数（B45，WS1 改动点3）：数据流 w 与
// 诊断流 errW 均由调用方注入。nil 按默认进程流处理（w→os.Stdout、
// errW→os.Stderr），因此 NewFormatter 的既有行为不变，只是收口到本构造器。
func NewFormatterWithWriters(w, errW io.Writer) *Formatter {
	if w == nil {
		w = os.Stdout
	}
	if errW == nil {
		errW = os.Stderr
	}
	return &Formatter{w: w, errW: errW}
}

// SetWriters 运行期替换两个写入目标（B46，WS1 改动点3）：deps.Out 的 writer
// 不再是构造期一次性硬编码，而是可注入接缝。nil 侧保留当前值（支持单侧替换）。
// 既有的 deps.Out.w / deps.Out.errW 直接替换习惯不受影响。
func (f *Formatter) SetWriters(w, errW io.Writer) {
	if w != nil {
		f.w = w
	}
	if errW != nil {
		f.errW = errW
	}
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

// PrintTable renders the table body (header + separator + data rows) to the
// data stream f.w, and the trailing "共 N 条" summary line to the diagnostic
// stream f.errW (B52, 契约规范 §5.1): the summary is a human-readable count,
// not part of the machine-consumable table payload, so it must not pollute
// stdout when the table output is piped.
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
	fmt.Fprintf(f.errW, "共 %d 条\n", len(rows))
}

// PrintSuccess/PrintError/PrintInfo/PrintDim write to the stderr stream: they
// are human-readable progress/status lines, not the command payload. Keeping
// stdout reserved for machine-consumable output (JSON/table/csv/...) lets
// agents pipe `-f json` results without parsing [OK]/[INFO] noise out of it.
func (f *Formatter) PrintSuccess(msg string)  { fmt.Fprintf(f.errW, "[OK] %s\n", msg) }
func (f *Formatter) PrintError(msg string)    { fmt.Fprintf(f.errW, "[ERROR] %s\n", msg) }
func (f *Formatter) PrintWarning(msg string)  { fmt.Fprintf(f.errW, "[WARN] %s\n", msg) }
func (f *Formatter) PrintInfo(msg string)     { fmt.Fprintf(f.errW, "[INFO] %s\n", msg) }
func (f *Formatter) PrintProgress(msg string) { fmt.Fprintf(f.errW, "%s\n", msg) }
func (f *Formatter) PrintDim(msg string)      { fmt.Fprintf(f.errW, "  %s\n", msg) }

// PrintKeyValue writes key/value preview & progress lines to the diagnostic
// stream (B51): like PrintInfo/PrintSuccess these are human-readable status,
// not the command payload — stdout stays reserved for machine-consumable
// output so `-f json` results pipe without parsing noise out of them.
func (f *Formatter) PrintKeyValue(key, value string) {
	fmt.Fprintf(f.errW, "%-16s%s\n", key+":", value)
}
