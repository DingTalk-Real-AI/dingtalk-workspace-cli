package helpers

import (
	"io"
	"os"
	"testing"
)

func TestCrossPlatformCoverageSheetInfoIncludePassthrough(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "sheet"}
	t.Cleanup(func() { os.Args = oldArgs })

	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"sheetId":"s1"}`},
	}}
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard

	root := newSheetCommand()
	if root.PersistentFlags().Lookup("dry-run") == nil {
		root.PersistentFlags().Bool("dry-run", false, "")
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs([]string{"info", "--node", "n1", "--sheet-id", "Sheet1", "--include", "row_heights,col_widths"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	include, _ := caller.calls[0]["include"].([]string)
	if len(include) != 2 || include[0] != "row_heights" || include[1] != "col_widths" {
		t.Fatalf("include = %#v", caller.calls[0]["include"])
	}
}
