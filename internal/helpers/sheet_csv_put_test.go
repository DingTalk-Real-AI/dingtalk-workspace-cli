package helpers

import (
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
	tool string
	args map[string]any
}

func (c *csvPutRecordingCaller) CallTool(_ context.Context, _ string, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.tool = tool
	c.args = args
	return &edition.ToolResult{}, nil
}

func (*csvPutRecordingCaller) Format() string { return "json" }
func (*csvPutRecordingCaller) DryRun() bool   { return false }
func (*csvPutRecordingCaller) Fields() string { return "" }
func (*csvPutRecordingCaller) JQ() string     { return "" }

func TestCSVPutFormulaContractAndPassThrough(t *testing.T) {
	const formulaCSV = "=1+1,'=1+1"

	root := &cobra.Command{Use: "sheet"}
	root.AddCommand(newDataCmds()...)
	csvPut := findCoverageSubcommand(t, root, "csv-put")
	for _, want := range []string{"支持公式", "以 = 开头时默认按公式解析", "以 = 开头的字面文本", "'=1+1"} {
		if !strings.Contains(csvPut.Short+"\n"+csvPut.Long+"\n"+csvPut.Example, want) {
			t.Fatalf("csv-put help does not contain %q", want)
		}
	}
	if strings.Contains(csvPut.Long, "不支持公式") || strings.Contains(csvPut.Long, "=开头当文本") {
		t.Fatalf("csv-put help still advertises the old formula contract: %s", csvPut.Long)
	}
	if strings.Contains(csvPut.Long, "写入公式文本") {
		t.Fatalf("csv-put help incorrectly describes apostrophe escaping as formula text: %s", csvPut.Long)
	}

	caller := &csvPutRecordingCaller{}
	InitDepsForTest(t, caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	testseam.Swap(t, &os.Args, []string{"dws", "sheet", "csv-put"})

	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"csv-put",
		"--node", "node",
		"--sheet-id", "Sheet1",
		"--start-cell", "A1",
		"--csv", formulaCSV,
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute csv-put: %v", err)
	}
	if caller.tool != "set_range_from_csv" {
		t.Fatalf("tool = %q, want set_range_from_csv", caller.tool)
	}
	if got := caller.args["csv"]; got != formulaCSV {
		t.Fatalf("csv argument = %#v, want exact pass-through %q", got, formulaCSV)
	}
	if _, ok := caller.args["interpretFormulas"]; ok {
		t.Fatalf("csv-put unexpectedly added an interpretFormulas argument: %#v", caller.args)
	}

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

	batchHelp := newBatchUpdateCmd().Long
	for _, want := range []string{"csv-put", "以 = 开头时按公式解析", "以 = 开头的字面文本", "'=1+1"} {
		if !strings.Contains(batchHelp, want) {
			t.Fatalf("batch-update help does not contain %q", want)
		}
	}
	if strings.Contains(batchHelp, "写入公式文本") {
		t.Fatalf("batch-update help incorrectly describes apostrophe escaping as formula text: %s", batchHelp)
	}
}
