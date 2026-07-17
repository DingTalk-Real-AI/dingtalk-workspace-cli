package helpers

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageSheetStyleRangeAndFontCoverage(t *testing.T) {
	for _, raw := range []string{"", "Sheet1!A1:B2", "B2:A1", "A1", "A", "1", "A0", "A-1", "A1:bad"} {
		_, _, _ = parseA1Range(raw)
	}
	_ = fillStringMatrix(2, 2, "x")
	_ = fillIntMatrix(2, 2, 1)
	for _, spec := range []*styleSpec{
		{FontSize: 1, FontSizesJSON: `[[1]]`},
		{FontSize: -1},
		{FontSize: 12},
		{FontSizesJSON: `{`},
		{FontSizesJSON: `[[1,2]]`},
		{FontSizesJSON: `[[1]]`},
	} {
		_ = applyFontSize(spec, 1, 1, map[string]any{})
	}
	for _, tc := range []struct {
		scalar, raw string
		enum        map[string]bool
	}{
		{"x", `[["x"]]`, nil}, {"bad", "", hAlignEnum}, {"left", "", hAlignEnum},
		{"", "", nil}, {"", `{`, nil}, {"", `[["x","y"]]`, nil},
		{"", `[[""]]`, hAlignEnum}, {"", `[["bad"]]`, hAlignEnum}, {"", `[["left"]]`, hAlignEnum},
	} {
		_ = apply2DString(tc.scalar, tc.raw, 1, 1, "align", "alignments", tc.enum, map[string]any{})
	}
	_ = maxColLenStr([][]string{{}, {"a", "b"}})
	_ = maxColLen2D([][]int{{}, {1, 2}})
	_ = checkMatrixShape(1, 1, 1, 1, "matrix")
	_ = checkMatrixShape(1, 2, 1, 1, "matrix")

	for _, tc := range []struct {
		spec       styleSpec
		rows, cols int
	}{
		{styleSpec{}, 0, 1}, {styleSpec{}, 1001, 1}, {styleSpec{}, 1000, 31},
		{styleSpec{WordWrap: "invalid"}, 1, 1}, {styleSpec{WordWrap: "clip"}, 1, 1},
		{styleSpec{NumberFormat: "General"}, 1, 1}, {styleSpec{}, 1, 1},
	} {
		_ = applyStyleSpec(&tc.spec, tc.rows, tc.cols, map[string]any{})
	}
}

func TestCrossPlatformCoverageSheetBatchSetStyleCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "sheet"}
	t.Cleanup(func() { os.Args = oldArgs })
	write := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "batch.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cases := []struct {
		name     string
		body     string
		path     string
		format   string
		response string
		callErr  error
		cont     bool
	}{
		{name: "missing-file", path: filepath.Join(t.TempDir(), "missing")},
		{name: "invalid-json", body: `{`},
		{name: "empty", body: `[]`},
		{name: "missing-fields-stop", body: `[{"sheetId":""}]`},
		{name: "missing-fields-continue", body: `[{"sheetId":""},{"sheetId":"Sheet1","range":"A1","bgColor":"red"}]`, cont: true},
		{name: "invalid-range", body: `[{"sheetId":"Sheet1","range":"bad","bgColor":"red"}]`},
		{name: "invalid-range-continue", body: `[{"sheetId":"Sheet1","range":"bad","bgColor":"red"},{"sheetId":"Sheet1","range":"A1","bgColor":"red"}]`, cont: true},
		{name: "invalid-style", body: `[{"sheetId":"Sheet1","range":"A1"}]`},
		{name: "invalid-style-continue", body: `[{"sheetId":"Sheet1","range":"A1"},{"sheetId":"Sheet1","range":"A2","bgColor":"red"}]`, cont: true},
		{name: "json-success", body: `[{"sheetId":"Sheet1","range":"A1","bgColor":"red"}]`, format: "json", response: `{"ok":true}`},
		{name: "json-raw-result", body: `[{"sheetId":"Sheet1","range":"A1","bgColor":"red"}]`, format: "json", response: `{`},
		{name: "json-call-error-stop", body: `[{"sheetId":"Sheet1","range":"A1","bgColor":"red"}]`, format: "json", callErr: errors.New("failed")},
		{name: "json-call-error-continue", body: `[{"sheetId":"Sheet1","range":"A1","bgColor":"red"},{"sheetId":"Sheet1","range":"A2","fontSize":12}]`, format: "json", callErr: errors.New("failed"), cont: true},
		{name: "raw-success", body: `[{"sheetId":"Sheet1","range":"A1","bgColor":"red"}]`, format: "raw", response: `{"ok":true}`},
		{name: "raw-call-error", body: `[{"sheetId":"Sheet1","range":"A1","bgColor":"red"}]`, format: "raw", callErr: errors.New("failed"), cont: true},
		{name: "raw-call-error-stop", body: `[{"sheetId":"Sheet1","range":"A1","bgColor":"red"}]`, format: "raw", callErr: errors.New("failed")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &helpersCoreCaller{format: tc.format, err: tc.callErr, result: textToolResult(tc.response)}
			installHelpersCoreDeps(t, caller)
			deps.Out.w = io.Discard
			deps.Out.errW = io.Discard
			cmd := newRangeBatchSetStyleCmd()
			_ = cmd.Flags().Set("node", "node")
			path := tc.path
			if path == "" {
				path = write(t, tc.body)
			}
			_ = cmd.Flags().Set("batch", path)
			_ = cmd.Flags().Set("continue-on-error", map[bool]string{true: "true", false: "false"}[tc.cont])
			_ = cmd.RunE(cmd, nil)
		})
	}
}
