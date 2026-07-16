package helpers

import (
	"bytes"
	"context"
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
