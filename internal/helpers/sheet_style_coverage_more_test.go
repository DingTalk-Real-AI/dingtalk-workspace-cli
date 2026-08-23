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
	for _, tc := range []struct {
		scalar  int
		jsonStr string
	}{
		{1, `[[1]]`}, {-1, ""}, {12, ""}, {0, `{`}, {0, `[[1,2]]`}, {0, `[[1]]`}, {0, ""},
	} {
		if get, err := intGrid(tc.scalar, tc.jsonStr, "font-size", 1, 1); err == nil && get != nil {
			_, _ = get(0, 0)
			_, _ = get(9, 9)
		}
	}
	for _, tc := range []struct {
		scalar, raw string
		enum        map[string]bool
	}{
		{"x", `[["x"]]`, nil}, {"bad", "", hAlignEnum}, {"left", "", hAlignEnum},
		{"", "", nil}, {"", `{`, nil}, {"", `[["x","y"]]`, nil},
		{"", `[[""]]`, hAlignEnum}, {"", `[["bad"]]`, hAlignEnum}, {"", `[["left"]]`, hAlignEnum},
	} {
		if get, err := strGrid(tc.scalar, tc.raw, "align", tc.enum, 1, 1); err == nil && get != nil {
			_, _ = get(0, 0)
			_, _ = get(9, 9)
		}
	}
	for _, raw := range []string{"", `{`, `{}`, `{"nope":{"style":"solid"}}`, `{"top":{}}`,
		`{"top":{"style":"bogus"}}`, `{"top":{"style":"solid"}}`, `{"top":{"style":"solid","color":"#000"}}`} {
		_, _ = parseBorderStyles(raw)
	}
	for _, raw := range []string{"Sheet1!A1:B2", "A1:B2", "Sheet1!", "!A1"} {
		_, _, _ = splitSheetPrefixedRange(raw, 0)
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
		{styleSpec{FontStyle: "bogus"}, 1, 1}, {styleSpec{FontStyle: "italic"}, 1, 1},
		{styleSpec{FontLine: "bogus"}, 1, 1}, {styleSpec{FontLine: "underline"}, 1, 1},
		{styleSpec{FontLine: "line-through"}, 1, 1}, {styleSpec{FontLine: "none"}, 1, 1},
		{styleSpec{FontFamily: "Arial"}, 1, 1},
		{styleSpec{BorderStylesJSON: `{"top":{"style":"solid"}}`}, 1, 1},
		{styleSpec{BorderStylesJSON: `{`}, 1, 1},
		{styleSpec{BgColor: "#FFF", FontSize: 12, HAlign: "center", VAlign: "top",
			FontColor: "#000", FontWeight: "bold"}, 2, 2},
	} {
		_, _ = buildStyleCells(&tc.spec, tc.rows, tc.cols)
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
