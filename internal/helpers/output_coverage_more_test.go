package helpers

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type outputFilterCaller struct{}

func (outputFilterCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return &edition.ToolResult{}, nil
}
func (outputFilterCaller) Format() string { return "" }
func (outputFilterCaller) DryRun() bool   { return false }
func (outputFilterCaller) Fields() string { return "name" }
func (outputFilterCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageFormatterRemainingFilterBranches(t *testing.T) {
	origDeps := deps
	t.Cleanup(func() { deps = origDeps })
	var out bytes.Buffer
	f := &Formatter{w: &out, errW: &out}

	deps = nil
	if handled, err := f.applyGlobalFilter(map[string]any{"name": "alice"}); handled || err != nil {
		t.Fatalf("nil deps handled=%v err=%v", handled, err)
	}
	deps = &Deps{Out: f}
	if handled, err := f.applyGlobalFilter(map[string]any{"name": "alice"}); handled || err != nil {
		t.Fatalf("nil caller handled=%v err=%v", handled, err)
	}

	deps = &Deps{Caller: outputFilterCaller{}, Out: f}
	if err := f.PrintJSON(map[string]any{"name": "alice", "hidden": true}); err != nil {
		t.Fatalf("filtered JSON: %v", err)
	}
	if err := f.PrintJSONUnescaped(map[string]any{"name": "bob", "hidden": true}); err != nil {
		t.Fatalf("filtered unescaped JSON: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("filtered output is empty")
	}
	if got := runeWidth("中a"); got != 3 {
		t.Fatalf("rune width=%d", got)
	}
	f.PrintTable([]string{"name"}, nil)
}

// TestFormatterStatusLinesNeverReachDataStream is the B56 total-flow assertion
// (AC-07/AC-11): with the data and diagnostic streams separated, every
// human-readable status line ([OK]/[INFO]/[ERROR]/[WARN]/key-value/table
// summary/progress) must land on the diagnostic stream, leaving the data
// stream free of status noise. The data payload (JSON/table rows/raw) is the
// only thing allowed on the data stream.
func TestFormatterStatusLinesNeverReachDataStream(t *testing.T) {
	var out, errOut bytes.Buffer
	f := NewFormatterWithWriters(&out, &errOut)

	f.PrintSuccess("done")
	f.PrintInfo("checking")
	f.PrintError("broken")
	f.PrintWarning("careful")
	f.PrintProgress("step 1/2")
	f.PrintDim("hint")
	f.PrintKeyValue("Tool", "create_todo")
	f.PrintTable([]string{"name"}, [][]string{{"alice"}})

	// Table body (header/rows) is payload and belongs on the data stream;
	// every status token must be absent from it.
	dataStream := out.String()
	for _, banned := range []string{"[OK]", "[INFO]", "[ERROR]", "[WARN]", "Tool:", "共", "step 1/2"} {
		if strings.Contains(dataStream, banned) {
			t.Fatalf("status token %q leaked to data stream: %q", banned, dataStream)
		}
	}
	if !strings.Contains(dataStream, "alice") {
		t.Fatalf("table data row missing from data stream: %q", dataStream)
	}

	diagnostic := errOut.String()
	for _, token := range []string{"[OK]", "[INFO]", "[ERROR]", "[WARN]", "Tool:", "共 1 条", "step 1/2"} {
		if !strings.Contains(diagnostic, token) {
			t.Fatalf("diagnostic stream missing %q: %q", token, diagnostic)
		}
	}
}
