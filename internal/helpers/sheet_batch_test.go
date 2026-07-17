package helpers

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageSheetBatchOperationTranslationCoversEveryMapping(t *testing.T) {
	input := map[string]any{
		"sheet-id": "Sheet1", "range": "A1:B2", "type": "all", "values": []any{"value"},
		"merge-type": "mergeRows", "source-range": "A1", "target-range": "B2",
		"target-sheet-id": "Sheet2", "paste-type": "values", "dimension": "row",
		"length": float64(2), "position": "3", "start-index": 1, "end-index": "2",
		"destination-index": float64(4), "options": []string{"a", "b"}, "multi-select": true,
		"csv": "a,b\r\n1,2", "start-cell": "A1", "allow-overwrite": true,
		"float-image-id": "image-id", "pixel-size": "24", "hidden": true,
		"group-state": "fold",
	}
	for name, mapping := range batchOpDispatch {
		got, err := translateBatchOp(map[string]any{"toolName": name, "input": input})
		if err != nil {
			t.Errorf("translateBatchOp(%q): %v", name, err)
			continue
		}
		if got["toolName"] != mapping.mcpTool {
			t.Errorf("translateBatchOp(%q) tool = %v, want %q", name, got["toolName"], mapping.mcpTool)
		}
		if _, ok := got["input"].(map[string]any); !ok {
			t.Errorf("translateBatchOp(%q) input = %#v", name, got["input"])
		}
	}
	if _, err := translateBatchOp(map[string]any{"toolName": "unknown"}); err == nil {
		t.Fatal("unknown batch operation should fail")
	}
	if _, err := translateBatchOp(map[string]any{"toolName": "range clear"}); err != nil {
		t.Fatalf("nil input should be accepted: %v", err)
	}
}

func TestCrossPlatformCoverageSheetBatchValueConversionsAndDefaults(t *testing.T) {
	if got := batchStr(map[string]any{"second": 42}, "first", "second"); got != "42" {
		t.Fatalf("batchStr() = %q", got)
	}
	if got := batchStr(nil, "missing"); got != "" {
		t.Fatalf("missing batchStr() = %q", got)
	}
	for _, tc := range []struct {
		input map[string]any
		want  int
	}{
		{map[string]any{"n": float64(3)}, 3},
		{map[string]any{"n": 4}, 4},
		{map[string]any{"n": "5"}, 5},
		{nil, 0},
	} {
		if got := batchInt(tc.input, "missing", "n"); got != tc.want {
			t.Errorf("batchInt(%v) = %d, want %d", tc.input, got, tc.want)
		}
	}
	for _, input := range []map[string]any{nil, {"type": nil}, {"type": ""}} {
		if got := batchStrOr(input, "type", "content"); got != "content" {
			t.Errorf("batchStrOr(%v) = %q", input, got)
		}
	}
	if got := batchStrOr(map[string]any{"type": "all"}, "type", "content"); got != "all" {
		t.Fatalf("explicit batchStrOr() = %q", got)
	}

	if got := BuildMergeCellsArgs(nil)["mergeType"]; got != "mergeAll" {
		t.Fatalf("default merge type = %v", got)
	}
	if _, ok := BuildFillRangeArgs(nil)["fillType"]; ok {
		t.Fatal("empty fill type should be omitted")
	}
	if got := BuildFillRangeArgs(map[string]any{"fill-type": "down"})["fillType"]; got != "down" {
		t.Fatalf("fill type = %v", got)
	}
	if got := BuildGroupDimensionArgs(nil)["groupState"]; got != "expand" {
		t.Fatalf("default group state = %v", got)
	}
	if _, ok := BuildUpdateDimensionArgs(nil)["pixelSize"]; ok {
		t.Fatal("zero pixel size should be omitted")
	}
	if _, ok := BuildSetDropdownArgs(nil)["enableMultiSelect"]; ok {
		t.Fatal("unset multi-select should be omitted")
	}
}

func TestCrossPlatformCoverageResolveCSVContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.csv")
	if err := os.WriteFile(path, []byte("\xef\xbb\xbfhead\r\nvalue"), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	if got := resolveCsvContent("@" + path); got != "head\nvalue" {
		t.Fatalf("file csv = %q", got)
	}
	missing := "@" + filepath.Join(dir, "missing.csv")
	if got := resolveCsvContent(missing); got != missing {
		t.Fatalf("missing file csv = %q", got)
	}

	oldStdin := os.Stdin
	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := pipeWrite.WriteString("a,b\r\n1,2"); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	_ = pipeWrite.Close()
	os.Stdin = pipeRead
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = pipeRead.Close()
	})
	if got := resolveCsvContent("-"); got != "a,b\n1,2" {
		t.Fatalf("stdin csv = %q", got)
	}
	if got := resolveCsvContent("plain\r\n"); got != strings.ReplaceAll("plain\r\n", "\r", "") {
		t.Fatalf("plain csv = %q", got)
	}

	closedRead, closedWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("closed pipe: %v", err)
	}
	_ = closedWrite.Close()
	_ = closedRead.Close()
	os.Stdin = closedRead
	if got := resolveCsvContent("-"); got != "-" {
		t.Fatalf("failed stdin read = %q", got)
	}
}

func executeSheetBatchCommand(t *testing.T, caller *scriptedToolCaller, cmd *cobra.Command, args ...string) error {
	t.Helper()
	oldDeps := deps
	oldArgs := os.Args
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	os.Args = []string{"dws", "sheet"}
	t.Cleanup(func() {
		deps = oldDeps
		os.Args = oldArgs
	})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func batchUpdateCoverageCommand() *cobra.Command {
	cmd := newBatchUpdateCmd()
	cmd.Flags().String("node", "", "")
	cmd.Flags().String("operations", "", "")
	cmd.Flags().Bool("continue-on-error", false, "")
	return cmd
}

func rangeBatchClearCoverageCommand() *cobra.Command {
	cmd := newRangeBatchClearCmd()
	cmd.Flags().String("node", "", "")
	cmd.Flags().String("ranges", "", "")
	cmd.Flags().String("type", "", "")
	return cmd
}

func TestCrossPlatformCoverageSheetBatchUpdateCommandRemainingCoverage(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--node", "node", "--operations", "["},
	} {
		if err := executeSheetBatchCommand(t, &scriptedToolCaller{}, batchUpdateCoverageCommand(), args...); err == nil {
			t.Fatalf("batch arguments %v returned nil", args)
		}
	}
	invalid := []string{
		"[]",
		"[1]",
		`[{"toolName":"unknown","input":{}}]`,
	}
	for _, operations := range invalid {
		if err := executeSheetBatchCommand(t, &scriptedToolCaller{}, batchUpdateCoverageCommand(), "--node", "node", "--operations", operations); err == nil {
			t.Fatalf("operations %s returned nil", operations)
		}
	}

	valid := `[{"toolName":"range fill","input":{"sheet-id":"sheet","source-range":"A1","target-range":"A2","fill-type":"down"}}]`
	if err := executeSheetBatchCommand(t, &scriptedToolCaller{}, batchUpdateCoverageCommand(), "--node", "node", "--operations", valid); err != nil {
		t.Fatalf("batch update success: %v", err)
	}
	boom := errors.New("batch failed")
	if err := executeSheetBatchCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: boom}}}, batchUpdateCoverageCommand(), "--node", "node", "--operations", valid); err == nil {
		t.Fatal("strict batch error returned nil")
	}
	if err := executeSheetBatchCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: boom}}}, batchUpdateCoverageCommand(), "--node", "node", "--operations", valid, "--continue-on-error"); err == nil {
		t.Fatal("lenient batch error returned nil")
	}
}

func TestCrossPlatformCoverageSheetRangeBatchClearCommandRemainingCoverage(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--node", "node", "--ranges", "["},
	} {
		if err := executeSheetBatchCommand(t, &scriptedToolCaller{}, rangeBatchClearCoverageCommand(), args...); err == nil {
			t.Fatalf("clear arguments %v returned nil", args)
		}
	}
	for _, ranges := range []string{"[]", `["A1:B2"]`, `["!A1:B2"]`, `["Sheet!"]`} {
		if err := executeSheetBatchCommand(t, &scriptedToolCaller{}, rangeBatchClearCoverageCommand(), "--node", "node", "--ranges", ranges); err == nil {
			t.Fatalf("ranges %s returned nil", ranges)
		}
	}
	if err := executeSheetBatchCommand(t, &scriptedToolCaller{}, rangeBatchClearCoverageCommand(), "--node", "node", "--ranges", `[" Sheet ! A1:B2 "]`); err != nil {
		t.Fatalf("default clear type: %v", err)
	}
	if err := executeSheetBatchCommand(t, &scriptedToolCaller{}, rangeBatchClearCoverageCommand(), "--node", "node", "--ranges", `["Sheet!A1:B2"]`, "--type", "all"); err != nil {
		t.Fatalf("explicit clear type: %v", err)
	}
}
