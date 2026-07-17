package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoveragePureScalarAndCommandHelpersCoverage(t *testing.T) {
	previousDeps := deps
	InitDeps(&productExampleCaller{})
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	t.Cleanup(func() { deps = previousDeps })

	for _, value := range []any{" text ", float64(12), int64(13), 14, json.Number("15"), true, nil} {
		_ = toFlatString(value)
	}
	for _, value := range []any{"2026-01-02", "2026-01-02 03:04:05", "bad", float64(1), int64(2), 3, true} {
		_, _ = normalizeWorkDate(value)
	}
	for _, value := range []string{"", "12", " 34 ", "1a"} {
		_ = isNumericUserID(value)
	}

	root := &cobra.Command{Use: "calendar"}
	known := &cobra.Command{Use: "event", Aliases: []string{"e"}}
	hidden := &cobra.Command{Use: "hidden", Hidden: true}
	root.AddCommand(known, hidden)
	for _, args := range [][]string{{"--x"}, {"event"}, {"e"}, {"missing"}} {
		_ = findUnknownVerb(root, args)
	}
	printUnknownSubcmdError(root, "evnt")
	for _, depth := range []int{0, 1, 3} {
		_ = stripCommandPrefix([]string{"calendar", "event", "--x"}, depth)
	}
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	root.Flags().StringP("known", "k", "", "")
	for _, args := range [][]string{
		{"dws", "calendar", "--known=x"},
		{"dws", "calendar", "--unknown"},
		{"dws", "calendar", "-z"},
		{"dws", "calendar", "--", "--ignored"},
	} {
		os.Args = args
		_ = findUnknownFlag(root)
	}

	for _, event := range []any{
		nil,
		map[string]any{"start": map[string]any{"dateTime": "2026-01-02T03:04:05Z"}},
		map[string]any{"start": map[string]any{"dateTime": "bad"}, "created": float64(2)},
		map[string]any{"updated": float64(3)},
		map[string]any{},
	} {
		_ = calendarEventSortKey(event)
	}
	_ = buildReminders("5,bad,0,10")

	params := map[string]string{"existing": "old"}
	putChatChmodParam(params, "blank", "")
	putChatChmodParam(params, "existing", "new")
	putChatChmodParam(params, "new", " value ")
	meta := conversationLocalFileMeta{FileName: "x.txt", FileType: "text/plain", ContentPath: "/tmp/x", FileSize: 3}
	if _, err := buildConversationFileContent(1, 2, meta); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{json.Number("1"), json.Number("bad"), float64(2), float64(-1), int64(3), 4, "5", "bad", true} {
		_, _ = int64FromJSONScalar(value)
	}
}

func TestCrossPlatformCoverageChartMailAndSheetPureHelpersCoverage(t *testing.T) {
	valid := map[string]any{
		"position":   map[string]any{"row": float64(1), "col": float64(1)},
		"dimensions": map[string]any{"width": float64(10), "height": float64(10)},
		"chart": map[string]any{
			"type":   "line",
			"series": []any{map[string]any{"value": []any{"A1:A2"}}},
		},
	}
	chartCases := []map[string]any{
		{},
		{"position": "bad"},
		{"position": map[string]any{"col": 1}},
		{"position": map[string]any{"row": 1}},
		{"position": map[string]any{"row": 1, "col": 1}},
	}
	for _, props := range chartCases {
		_ = validateChartProperties(props)
	}
	mutations := []func(map[string]any){
		func(m map[string]any) { m["dimensions"] = "bad" },
		func(m map[string]any) { m["dimensions"] = map[string]any{"height": 1} },
		func(m map[string]any) { m["dimensions"] = map[string]any{"width": float64(-1), "height": 1} },
		func(m map[string]any) { m["dimensions"] = map[string]any{"width": 1} },
		func(m map[string]any) { m["dimensions"] = map[string]any{"width": 1, "height": float64(-1)} },
		func(m map[string]any) { delete(m, "chart") },
		func(m map[string]any) { m["chart"] = "bad" },
		func(m map[string]any) { m["chart"] = map[string]any{} },
		func(m map[string]any) { m["chart"] = map[string]any{"type": 1} },
		func(m map[string]any) { m["chart"] = map[string]any{"type": "bad"} },
		func(m map[string]any) { m["chart"] = map[string]any{"type": "line"} },
		func(m map[string]any) { m["chart"] = map[string]any{"type": "line", "series": "bad"} },
		func(m map[string]any) { m["chart"] = map[string]any{"type": "line", "series": []any{}} },
		func(m map[string]any) { m["chart"] = map[string]any{"type": "line", "series": []any{"bad"}} },
		func(m map[string]any) { m["chart"] = map[string]any{"type": "line", "series": []any{map[string]any{}}} },
		func(m map[string]any) {
			m["chart"] = map[string]any{"type": "line", "series": []any{map[string]any{"value": "bad"}}}
		},
		func(m map[string]any) {
			m["chart"] = map[string]any{"type": "line", "series": []any{map[string]any{"value": []any{}}}}
		},
	}
	for _, mutate := range mutations {
		var clone map[string]any
		raw, _ := json.Marshal(valid)
		_ = json.Unmarshal(raw, &clone)
		mutate(clone)
		_ = validateChartProperties(clone)
	}
	if err := validateChartProperties(valid); err != nil {
		t.Fatal(err)
	}

	mailCases := []string{
		`{`,
		`[1]`,
		`[{"object":"bad","or":[]}]`,
		`[{"object":"from","or":"bad"}]`,
		`[{"object":"from","or":[1]}]`,
		`[{"object":"from","or":[{"and":"bad"}]}]`,
		`[{"object":"from","or":[{"and":[1]}]}]`,
		`[{"object":"from","or":[{"and":[{"operation":"bad"}]}]}]`,
		`[{"object":"from","or":[{"and":[{"operation":"include"}]}]}]`,
	}
	for _, raw := range mailCases {
		_, _ = parseMailRuleConditions(raw)
	}
	_ = generateContentId("My Image.PNG", 2)
	_ = validateInlineAttachmentType("x.png")
	_ = validateInlineAttachmentType("x.pdf")
	_ = inlineHtmlTag("cid", "x.png")
	_ = injectInlineCids("hello\n[inline:x.png]", []inlineAttachInfo{{name: "x.png", contentId: "cid"}, {name: "y.png", contentId: "cid2"}})
	for _, raw := range []string{`{`, `{}`, `{"result":{"message":{"id":"m"}}}`, `{"result":{"messageId":"m2"}}`} {
		_, _ = parseMailDraftId(raw)
	}
	for _, raw := range []string{`{`, `{}`, `{"result":{"uploadUrl":"/u"}}`} {
		_, _ = parseMailUploadSession(raw)
	}
	for _, kind := range []string{"personal", "enterprise", ""} {
		_ = mailUploadBaseURL(kind)
	}

	for _, text := range []string{`{`, `{}`, `{"fileName":"../x\\y.txt"}`, `{"result":{"fileName":"x.txt"}}`} {
		_ = extractFileNameFromResponse(text)
	}
	for _, index := range []int{-1, 0, 25, 26, 701} {
		_ = sheetColumnLetterFromZeroBased(index)
	}
	for _, value := range []any{nil, "bad", map[string]any{}, map[string]any{"x": nil}, map[string]any{"x": true}} {
		_ = isEmptyDataValidation(value)
		_ = isEmptyHyperlink(value)
	}
	_ = fillIntMatrix(2, 3, 4)
	_ = maxColLenStr([][]string{{"a"}, {"b", "c"}})
	_ = maxColLen2D([][]int{{1}, {2, 3}})
	_ = checkMatrixShape(1, 2, 1, 2, "x")
	_ = checkMatrixShape(1, 1, 1, 2, "x")
	for _, value := range []any{1, -1, int64(2), int64(-2), float64(3), float64(3.5), float64(-3), json.Number("4"), json.Number("bad"), "bad"} {
		_, _ = nonNegativeJSONInt(value)
	}
	_ = buildNonEmptyRangeFromLegacy(map[string]any{})
	_ = buildNonEmptyRangeFromLegacy(map[string]any{"lastNonEmptyRow": float64(1), "lastNonEmptyColumn": float64(2)})
	_ = normalizeNonEmptyRangeObject(nil)
	_ = normalizeNonEmptyRangeObject(map[string]any{"range": "A1:B2"})
	_ = normalizeNonEmptyRangeObject(map[string]any{"range": "A1:B2", "lastCell": "B2", "lastRow": float64(2), "lastColumn": "B"})
	for _, sheet := range []map[string]any{
		{"nonEmptyRange": map[string]any{"range": "A1:B2", "lastCell": "B2", "lastRow": float64(2), "lastColumn": "B"}},
		{"lastNonEmptyRow": float64(1), "lastNonEmptyColumn": float64(2)},
		{},
	} {
		normalizeSheetInfoCoordinatesForAgent(sheet)
	}

	styles := []struct {
		spec       styleSpec
		rows, cols int
	}{
		{styleSpec{}, 0, 1},
		{styleSpec{}, 1001, 1},
		{styleSpec{}, 1000, 31},
		{styleSpec{BgColor: "red", BgColorsJSON: `[["red"]]`}, 1, 1},
		{styleSpec{FontSize: 1, FontSizesJSON: `[[1]]`}, 1, 1},
		{styleSpec{FontSize: -1}, 1, 1},
		{styleSpec{FontSizesJSON: `{`}, 1, 1},
		{styleSpec{FontSizesJSON: `[[1,2]]`}, 1, 1},
		{styleSpec{FontSizesJSON: `[[1]]`}, 1, 1},
		{styleSpec{HAlign: "bad"}, 1, 1},
		{styleSpec{VAlign: "bad"}, 1, 1},
		{styleSpec{FontWeight: "bad"}, 1, 1},
		{styleSpec{WordWrap: "bad"}, 1, 1},
		{styleSpec{WordWrap: "clip"}, 1, 1},
		{styleSpec{NumberFormat: "0.00"}, 1, 1},
		{styleSpec{}, 1, 1},
	}
	for _, style := range styles {
		_ = applyStyleSpec(&style.spec, style.rows, style.cols, map[string]any{})
	}
	for _, tc := range []struct {
		scalar, raw string
		enum        map[string]bool
	}{
		{"x", `[["x"]]`, nil},
		{"bad", "", hAlignEnum},
		{"", "{", nil},
		{"", `[["x","y"]]`, nil},
		{"", `[["bad"]]`, hAlignEnum},
		{"", `[[""]]`, hAlignEnum},
		{"", `[["left"]]`, hAlignEnum},
	} {
		_ = apply2DString(tc.scalar, tc.raw, 1, 1, "align", "alignments", tc.enum, map[string]any{})
	}
	views := []map[string]any{{"id": "one"}, {"filterViewId": "two"}}
	_, _ = findFilterViewByID(views, "one")
	_, _ = findFilterViewByID(views, "two")
	_, _ = findFilterViewByID(views, "missing")
}

func TestCrossPlatformCoverageSheetResponseCleanupCoverage(t *testing.T) {
	previous := deps
	caller := &helpersCoreCaller{format: "json"}
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	t.Cleanup(func() { deps = previous })

	for _, text := range []string{
		"",
		"not-json",
		`{"cells":"bad"}`,
		`{"cells":["bad",["bad",{"value":1},{"dataValidation":null,"hyperlink":null},{"dataValidation":{"x":null},"hyperlink":{"x":null}},{"dataValidation":{"x":true},"hyperlink":{"x":true}}]]}`,
	} {
		caller.result = textToolResult(text)
		_ = callMCPToolCellInfos(nil)
	}
	for _, text := range []string{"", "not-json", `{}`, `{"lastNonEmptyRow":1,"lastNonEmptyColumn":2}`} {
		caller.result = textToolResult(text)
		_ = callMCPToolSheetInfo(nil)
	}
	caller.err = fmt.Errorf("failed")
	_ = callMCPToolCellInfos(nil)
	_ = callMCPToolSheetInfo(nil)
}

func TestCrossPlatformCoverageSmallHandlerAndFormatterCoverage(t *testing.T) {
	if (Manifest{Vendor: " vendor ", Name: " name "}).FullName() != "vendor/name" {
		t.Fatal("manifest full name")
	}
	_ = (openCompatHandler{name: "conference", buildFn: newConferenceCommand}).Name()
	_ = (wukongHandler{name: "doc", buildFn: newDocCommand}).Name()
	_ = (devHandler{}).Name()
	_ = newDocCommentCommand()
	_ = newHrmregisterCommand()
	_ = newPatCommand()

	var out bytes.Buffer
	f := &Formatter{w: &out, errW: io.Discard}
	f.PrintSuccess("ok")
	f.PrintError("bad")
	if out.Len() == 0 {
		t.Fatal("formatter output is empty")
	}

	parent := &cobra.Command{Use: "sheet"}
	group := &cobra.Command{Use: "range"}
	group.AddCommand(&cobra.Command{Use: "read"})
	parent.AddCommand(group)
	_ = deepSuggestSubcommand(parent, "read")
	_ = deepSuggestSubcommand(parent, "missing")
	_ = time.Now()
}
