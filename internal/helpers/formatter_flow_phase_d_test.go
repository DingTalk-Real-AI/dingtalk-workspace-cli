package helpers

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// --- B45/B46：注入式构造与可替换 writer 接缝（WS1 改动点3）---

func TestNewFormatterWithWritersInjectsBothStreams(t *testing.T) {
	var out, errOut bytes.Buffer
	f := NewFormatterWithWriters(&out, &errOut)
	f.PrintRaw("data-line")
	f.PrintInfo("diag-line")
	if !strings.Contains(out.String(), "data-line") || strings.Contains(out.String(), "diag-line") {
		t.Fatalf("data stream polluted: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "[INFO] diag-line") || strings.Contains(errOut.String(), "data-line") {
		t.Fatalf("diagnostic stream wrong: %q", errOut.String())
	}
}

// B55：默认流是进程 stdout/stderr（NewFormatter 与 nil 注入的兜底行为一致）。
func TestFormatterDefaultsToProcessStreams(t *testing.T) {
	f := NewFormatter()
	if f.w != os.Stdout {
		t.Fatalf("NewFormatter default w = %v, want os.Stdout", f.w)
	}
	if f.errW != os.Stderr {
		t.Fatalf("NewFormatter default errW = %v, want os.Stderr", f.errW)
	}
	g := NewFormatterWithWriters(nil, nil)
	if g.w != os.Stdout || g.errW != os.Stderr {
		t.Fatalf("nil-injection defaults = %v/%v, want os.Stdout/os.Stderr", g.w, g.errW)
	}
}

// B46：SetWriters 运行期接缝——nil 侧保留当前值（单侧替换），非 nil 侧替换。
func TestFormatterSetWritersReplacesStreamsAtRuntime(t *testing.T) {
	var out, errOut bytes.Buffer
	f := NewFormatterWithWriters(os.Stdout, os.Stderr)
	f.SetWriters(&out, nil)
	if f.w != &out || f.errW != os.Stderr {
		t.Fatalf("single-sided SetWriters(w, nil) = %v/%v", f.w, f.errW)
	}
	f.SetWriters(nil, &errOut)
	if f.w != &out || f.errW != &errOut {
		t.Fatalf("single-sided SetWriters(nil, errW) = %v/%v", f.w, f.errW)
	}
	f.PrintRaw("payload")
	f.PrintWarning("heads-up")
	if out.String() != "payload\n" {
		t.Fatalf("data stream = %q", out.String())
	}
	if errOut.String() != "[WARN] heads-up\n" {
		t.Fatalf("diagnostic stream = %q", errOut.String())
	}
}

// --- B51~B54：Formatter 各方法流向收口（契约规范 §5.1）---

// B51：PrintKeyValue 是人读预览/进度行，改走 errW；stdout 零字节。
func TestPrintKeyValueGoesToDiagnosticStream(t *testing.T) {
	var out, errOut bytes.Buffer
	f := NewFormatterWithWriters(&out, &errOut)
	f.PrintKeyValue("Tool", "create_todo")
	if out.Len() != 0 {
		t.Fatalf("PrintKeyValue leaked to data stream: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "Tool:") || !strings.Contains(errOut.String(), "create_todo") {
		t.Fatalf("diagnostic stream missing key/value: %q", errOut.String())
	}
}

// B52：PrintTable 数据行（表头+分隔+数据）走 f.w，"共 N 条"摘要行走 f.errW。
func TestPrintTableSplitsDataAndSummaryStreams(t *testing.T) {
	var out, errOut bytes.Buffer
	f := NewFormatterWithWriters(&out, &errOut)
	f.PrintTable([]string{"name", "role"}, [][]string{{"alice", "admin"}, {"bob", "dev"}})

	for _, want := range []string{"name", "role", "alice", "bob", "admin", "dev"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("data stream missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "共") {
		t.Fatalf("summary line leaked to data stream:\n%s", out.String())
	}
	if errOut.String() != "共 2 条\n" {
		t.Fatalf("summary stream = %q, want 共 2 条 on stderr", errOut.String())
	}
}

// B52 边界：空 rows 早退，两个流都零字节（不产出无数据的摘要）。
func TestPrintTableEmptyRowsWritesNothing(t *testing.T) {
	var out, errOut bytes.Buffer
	f := NewFormatterWithWriters(&out, &errOut)
	f.PrintTable([]string{"name"}, nil)
	f.PrintTable([]string{"name"}, [][]string{})
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Fatalf("empty table wrote bytes: out=%q err=%q", out.String(), errOut.String())
	}
}

// B53：PrintRaw 保持数据流 f.w 不变（防流向修复误伤数据输出）。
func TestPrintRawStaysOnDataStream(t *testing.T) {
	var out, errOut bytes.Buffer
	f := NewFormatterWithWriters(&out, &errOut)
	f.PrintRaw("raw payload")
	if out.String() != "raw payload\n" {
		t.Fatalf("PrintRaw data stream = %q", out.String())
	}
	f.PrintRaw("already terminated\n")
	if out.String() != "raw payload\nalready terminated\n" {
		t.Fatalf("PrintRaw double-newline: %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("PrintRaw leaked to diagnostic stream: %q", errOut.String())
	}
}

// B54：PrintWarning/PrintProgress 已走 errW 的现状回归断言（防退化）。
func TestPrintWarningAndProgressStayOnDiagnosticStream(t *testing.T) {
	var out, errOut bytes.Buffer
	f := NewFormatterWithWriters(&out, &errOut)
	f.PrintWarning("careful")
	f.PrintProgress("step 1/2")
	if out.Len() != 0 {
		t.Fatalf("warning/progress leaked to data stream: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "[WARN] careful") {
		t.Fatalf("warning missing: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "step 1/2") {
		t.Fatalf("progress missing: %q", errOut.String())
	}
}

// B54 配套：PrintSuccess/PrintError/PrintInfo/PrintDim（B47~B50 已完成项）
// 同走 errW 的防退化总断言。
func TestStatusPrintMethodsAllStayOnDiagnosticStream(t *testing.T) {
	var out, errOut bytes.Buffer
	f := NewFormatterWithWriters(&out, &errOut)
	f.PrintSuccess("ok")
	f.PrintError("bad")
	f.PrintInfo("note")
	f.PrintDim("faint")
	if out.Len() != 0 {
		t.Fatalf("status lines leaked to data stream: %q", out.String())
	}
	for _, want := range []string{"[OK] ok", "[ERROR] bad", "[INFO] note", "  faint"} {
		if !strings.Contains(errOut.String(), want) {
			t.Fatalf("diagnostic stream missing %q: %q", want, errOut.String())
		}
	}
}
