package helpers

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type csvPutRecordingCaller struct {
	tool  string
	args  map[string]any
	calls int
	dry   bool
}

func (c *csvPutRecordingCaller) CallTool(_ context.Context, _ string, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.tool = tool
	c.args = args
	c.calls++
	return &edition.ToolResult{}, nil
}

func (*csvPutRecordingCaller) Format() string { return "json" }
func (c *csvPutRecordingCaller) DryRun() bool { return c.dry }
func (*csvPutRecordingCaller) Fields() string { return "" }
func (*csvPutRecordingCaller) JQ() string     { return "" }

func executeCSVPutContractCommand(t *testing.T, args ...string) {
	t.Helper()
	root := &cobra.Command{Use: "sheet"}
	root.AddCommand(newDataCmds()...)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"csv-put"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("execute csv-put: %v", err)
	}
}

func TestCSVPutFormulaContractAndPassThrough(t *testing.T) {
	const formulaCSV = "=1+1,'=1+1"

	root := &cobra.Command{Use: "sheet"}
	root.AddCommand(newDataCmds()...)
	csvPut := findCoverageSubcommand(t, root, "csv-put")
	for _, want := range []string{"支持公式", "以 = 开头时默认按公式解析", "以 = 开头的字面文本", "--auto-convert", "其他字段按普通文本原样写入", "'=1+1", "前置单引号会保留"} {
		if !strings.Contains(csvPut.Short+"\n"+csvPut.Long+"\n"+csvPut.Example, want) {
			t.Fatalf("csv-put help does not contain %q", want)
		}
	}
	if strings.Contains(csvPut.Long, "不支持公式") || strings.Contains(csvPut.Long, "=开头当文本") || strings.Contains(csvPut.Long, "allText") {
		t.Fatalf("csv-put help still advertises the old formula contract: %s", csvPut.Long)
	}
	if strings.Contains(csvPut.Long, "写入公式文本") {
		t.Fatalf("csv-put help incorrectly describes apostrophe escaping as formula text: %s", csvPut.Long)
	}
	autoConvert := csvPut.Flags().Lookup("auto-convert")
	if autoConvert == nil || autoConvert.DefValue != "true" {
		t.Fatalf("--auto-convert flag = %#v, want optional boolean default true", autoConvert)
	}
	for _, want := range []string{"非公式", "文本原样写入", "= 开头仍作为公式"} {
		if !strings.Contains(autoConvert.Usage, want) {
			t.Fatalf("--auto-convert help does not contain %q: %s", want, autoConvert.Usage)
		}
	}

	caller := &csvPutRecordingCaller{}
	InitDepsForTest(t, caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	testseam.Swap(t, &os.Args, []string{"dws", "sheet", "csv-put"})

	executeCSVPutContractCommand(t,
		"--node", "node",
		"--sheet-id", "Sheet1",
		"--start-cell", "A1",
		"--csv", formulaCSV,
	)
	if caller.tool != "set_range_from_csv" {
		t.Fatalf("tool = %q, want set_range_from_csv", caller.tool)
	}
	if got := caller.args["csv"]; got != formulaCSV {
		t.Fatalf("csv argument = %#v, want exact pass-through %q", got, formulaCSV)
	}
	if _, ok := caller.args["interpretFormulas"]; ok {
		t.Fatalf("csv-put unexpectedly added an interpretFormulas argument: %#v", caller.args)
	}
	if _, ok := caller.args["autoConvert"]; ok {
		t.Fatalf("default csv-put must omit autoConvert for compatibility: %#v", caller.args)
	}

	for _, tc := range []struct {
		name string
		arg  string
		want bool
	}{
		{name: "explicit false", arg: "--auto-convert=false", want: false},
		{name: "explicit true", arg: "--auto-convert=true", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller.tool = ""
			caller.args = nil
			executeCSVPutContractCommand(t,
				"--node", "node",
				"--sheet-id", "Sheet1",
				"--start-cell", "A1",
				"--csv", formulaCSV,
				tc.arg,
			)
			if got, ok := caller.args["autoConvert"].(bool); !ok || got != tc.want {
				t.Fatalf("autoConvert = %#v, want %v", caller.args["autoConvert"], tc.want)
			}
			if got := caller.args["csv"]; got != formulaCSV {
				t.Fatalf("explicit autoConvert changed CSV = %#v, want %q", got, formulaCSV)
			}
		})
	}

	caller.tool = ""
	caller.args = nil
	caller.calls = 0
	caller.dry = true
	var dryRunOutput bytes.Buffer
	deps.Out.w = &dryRunOutput
	executeCSVPutContractCommand(t,
		"--node", "node",
		"--sheet-id", "Sheet1",
		"--start-cell", "A1",
		"--csv", formulaCSV,
		"--auto-convert=false",
	)
	if caller.calls != 0 {
		t.Fatalf("dry-run made %d MCP calls", caller.calls)
	}
	for _, want := range []string{`"tool": "set_range_from_csv"`, `"autoConvert": false`, `"csv": "=1+1,'=1+1"`} {
		if !strings.Contains(dryRunOutput.String(), want) {
			t.Fatalf("dry-run output does not contain %q: %s", want, dryRunOutput.String())
		}
	}
	caller.dry = false
	deps.Out.w = io.Discard

	batchArgs := BuildCsvPutArgs(map[string]any{
		"sheet-id":   "Sheet1",
		"start-cell": "A1",
		"csv":        formulaCSV,
	})
	if got := batchArgs["csv"]; got != formulaCSV {
		t.Fatalf("batch csv argument = %#v, want exact pass-through %q", got, formulaCSV)
	}
	if _, ok := batchArgs["interpretFormulas"]; ok {
		t.Fatalf("batch csv-put unexpectedly added an interpretFormulas argument: %#v", batchArgs)
	}
	if _, ok := batchArgs["autoConvert"]; ok {
		t.Fatalf("default batch csv-put must omit autoConvert for compatibility: %#v", batchArgs)
	}
	for _, tc := range []struct {
		name string
		want bool
	}{
		{name: "false", want: false},
		{name: "true", want: true},
	} {
		t.Run("batch "+tc.name, func(t *testing.T) {
			got := BuildCsvPutArgs(map[string]any{
				"sheet-id":     "Sheet1",
				"start-cell":   "A1",
				"csv":          formulaCSV,
				"auto-convert": tc.want,
			})
			if value, ok := got["autoConvert"].(bool); !ok || value != tc.want {
				t.Fatalf("batch autoConvert = %#v, want %v", got["autoConvert"], tc.want)
			}
			if got["csv"] != formulaCSV {
				t.Fatalf("batch autoConvert changed CSV = %#v, want %q", got["csv"], formulaCSV)
			}
		})
	}
	if _, err := translateBatchOp(map[string]any{
		"toolName": "csv-put",
		"input": map[string]any{
			"sheet-id":     "Sheet1",
			"start-cell":   "A1",
			"csv":          formulaCSV,
			"auto-convert": "false",
		},
	}); err == nil || !strings.Contains(err.Error(), "必须是布尔值") {
		t.Fatalf("batch csv-put non-boolean auto-convert error = %v", err)
	}

	batchHelp := newBatchUpdateCmd().Long
	for _, want := range []string{"csv-put", `"auto-convert":false`, "关闭非公式", "以 = 开头时仍按公式解析", "普通文本", "'=1+1", "保留前置单引号", "缺省或 true"} {
		if !strings.Contains(batchHelp, want) {
			t.Fatalf("batch-update help does not contain %q", want)
		}
	}
	if strings.Contains(batchHelp, "写入公式文本") || strings.Contains(batchHelp, "allText") {
		t.Fatalf("batch-update help incorrectly describes apostrophe escaping as formula text: %s", batchHelp)
	}
}
