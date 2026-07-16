package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type reportTestCaller struct {
	dry      bool
	format   string
	response string
	err      error
	calls    []string
}

func (c *reportTestCaller) CallTool(_ context.Context, _, tool string, _ map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, tool)
	if c.err != nil {
		return nil, c.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.response}}}, nil
}

func (c *reportTestCaller) Format() string { return c.format }
func (c *reportTestCaller) DryRun() bool   { return c.dry }
func (*reportTestCaller) Fields() string   { return "" }
func (*reportTestCaller) JQ() string       { return "" }

func installReportTestDeps(t *testing.T, caller *reportTestCaller) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	previous := deps
	t.Cleanup(func() { deps = previous })
	InitDeps(caller)
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	deps.Out.w = out
	deps.Out.errW = errOut
	return out, errOut
}

func TestCrossPlatformCoverageReportReadableListEnrichment(t *testing.T) {
	caller := &reportTestCaller{format: "json", response: `{
		"result":{"reportName":"Detail","creatorName":"Alice","createTime":1700000000000,"readStatus":true,"url":"dingtalk://detail","contents":[{"key":"Done","content":"A|B"}]}}
	`}
	installReportTestDeps(t, caller)
	body := map[string]any{
		"success":    true,
		"hasMore":    false,
		"nextCursor": "next",
		"result": []any{
			map[string]any{"reportId": "older", "createTime": float64(1600000000000), "senderName": "Old"},
			map[string]any{"report_id": "newer", "createTime": "2026-01-02T03:04:05Z"},
			map[string]any{"title": "no id"},
		},
	}
	got := enrichReportListReadable(context.Background(), "sent", body).(map[string]any)
	rows := got["result"].([]map[string]string)
	if len(rows) != 2 || got["count"] != 2 || !got["agentDisplayContentIncluded"].(bool) {
		t.Fatalf("enriched sent list = %#v", got)
	}
	if !strings.Contains(got["agentDisplayMarkdown"].(string), "A｜B") || len(caller.calls) != 2 {
		t.Fatalf("markdown/calls = %q / %#v", got["agentDisplayMarkdown"], caller.calls)
	}
	if got["nextCursor"] != "next" || got["hasMore"] != false {
		t.Fatalf("pagination = %#v", got)
	}

	caller.err = errors.New("detail unavailable")
	inbox := enrichReportListReadable(context.Background(), "list", map[string]any{
		"result": map[string]any{"items": []any{map[string]any{"reportId": "id"}}, "cursor": "nested"},
	}).(map[string]any)
	if inbox["agentDisplayContentIncluded"].(bool) || inbox["cursor"] != "nested" {
		t.Fatalf("enriched inbox = %#v", inbox)
	}
	if enrichReportListReadable(context.Background(), "list", []any{"not-map"}) == nil {
		t.Fatal("non-map list enrichment changed value")
	}
	plain := map[string]any{"success": "ok"}
	if got := enrichReportListReadable(context.Background(), "list", plain).(map[string]any); got["success"] != "ok" || len(got) != 1 {
		t.Fatalf("map without list was changed: %#v", got)
	}
}

func TestCrossPlatformCoverageReportReadableValueFormatting(t *testing.T) {
	columns := reportListDisplayColumns("sent")
	if len(columns) != 6 || len(reportListDisplayColumns("list")) != 5 {
		t.Fatal("display columns changed")
	}
	if !strings.Contains(reportListAgentDisplayInstruction(true, "header"), "日志内容") ||
		!strings.Contains(reportListAgentDisplayInstruction(false, "header"), "禁止返回日志正文") {
		t.Fatal("display instructions changed")
	}
	if reportReadableListTitle("sent") == reportReadableListTitle("list") {
		t.Fatal("sent and inbox titles should differ")
	}
	if reportListSuccess(map[string]any{}) != true || reportListSuccess(map[string]any{"success": false}) != false {
		t.Fatal("reportListSuccess changed")
	}

	values := []any{
		" text ", float64(1.25), true, false,
		[]any{"a", float64(2), nil}, map[string]any{"text": "nested"}, nil,
	}
	for _, value := range values {
		_ = reportDisplayStringFromValue(value)
		_ = reportReadableContentFromValue(value)
	}
	content := reportReadableContentFromValue([]any{
		map[string]any{"key": "Done", "content": "work"},
		map[string]any{"value": "bare"},
		map[string]any{"key": "empty"},
	})
	if content != "Done：work；bare" {
		t.Fatalf("readable content = %q", content)
	}
	if reportFirstDisplayValue(map[string]any{"first": nil, "second": "ok"}, "first", "second") != "ok" {
		t.Fatal("reportFirstDisplayValue did not fall through")
	}
	if reportCompactDisplayText(" a | b \n c ", 4) != "a ｜ ..." || reportCompactDisplayText("short", 0) != "short" {
		t.Fatal("compact display formatting changed")
	}

	row := reportReadableRow(map[string]any{"creatorName": "Alice", "read": false})
	if row["标题"] != "Alice的日志" || row["状态"] != "未读" {
		t.Fatalf("readable row = %#v", row)
	}
	if reportReadableRow(map[string]any{})["标题"] != "日志" {
		t.Fatal("empty row title fallback changed")
	}
	for _, status := range []any{true, false, "read", "unread", "custom", float64(1), float64(0), float64(2), nil} {
		_ = reportStatusValueToString(status)
	}

	timestamps := []any{float64(1700000000), int64(1700000000000), int(1700000000), json.Number("1700000000"), json.Number("bad"), "1700000000", "2026-01-02T03:04:05Z", "2026-01-02 03:04", "bad", "", nil}
	for _, timestamp := range timestamps {
		_ = reportTimeValueToMillis(timestamp)
		_ = reportTimeValueToString(timestamp)
	}
	if normalizeReportTimestamp(-1) != 0 || normalizeReportTimestamp(1700000000) != 1700000000000 || reportMillisToLocalString(0) != "" {
		t.Fatal("timestamp normalization changed")
	}

	entries := makeReportReadableEntries([]map[string]any{
		{"reportId": "zero", "title": "Z"},
		{"reportId": "old", "createTime": float64(1600000000000)},
		{"reportId": "new", "createTime": float64(1700000000000)},
		{"reportId": "same-a", "createTime": "date-b"},
		{"reportId": "same-b", "createTime": "date-a"},
	})
	sortReportReadableEntries(entries)
	if entries[0].reportID != "new" || entries[len(entries)-1].reportID != "zero" {
		t.Fatalf("sorted entries = %#v", entries)
	}

	table := reportListMarkdownTable([]string{"A", "B"}, []map[string]string{{"A": "x|y\nnext", "B": "z"}})
	if !strings.Contains(table, "x｜y next") || reportListMarkdownHeader([]string{"A"}) != "| A |" {
		t.Fatalf("markdown table = %q", table)
	}
}

func TestCrossPlatformCoverageReportResponseTraversalAndLinks(t *testing.T) {
	detail := map[string]any{"result": map[string]any{
		"report_name": "Title", "creatorName": "Alice", "createTime": "2026-01-02T03:04:05Z",
		"readStatus": "read", "contents": []any{map[string]any{"key": "Done", "content": "work"}},
		"url": "dingtalk://report",
	}}
	if reportReadableDetailTitle(detail) != "Title" || reportReadableDetailSender(detail) != "Alice" ||
		reportReadableDetailDate(detail) == "" || reportReadableDetailStatus(detail) != "已读" ||
		reportReadableDetailContent(detail) != "Done：work" || !strings.Contains(reportMarkdownLinkFromResponse(detail), "dingtalk://report") {
		t.Fatalf("detail traversal failed: %#v", detail)
	}
	if reportStringFromResponse(map[string]any{"dingtalkOpenMarkdownLink": " [link](url) "}, "dingtalkOpenMarkdownLink") != "[link](url)" {
		t.Fatal("reportStringFromResponse trimming changed")
	}
	for _, value := range []any{
		map[string]any{"result": map[string]any{"dingtalkOpenUrl": " nested "}},
		map[string]any{"result": map[string]any{"dingtalkOpenLink": map[string]any{"url": " nested-link "}}},
		map[string]any{"url": " top "},
		map[string]any{"dingtalkOpenLink": map[string]any{"url": " top-link "}},
		[]any{},
	} {
		_ = reportURLFromResponse(value)
	}
	body := map[string]any{"result": map[string]any{}}
	attachReportURL(body, "dingtalk://url")
	if body["url"] != "dingtalk://url" || body["result"].(map[string]any)["url"] != "dingtalk://url" {
		t.Fatalf("attachReportURL = %#v", body)
	}
	if reportDingtalkOpenLink("url")["url"] != "url" || !strings.Contains(reportDingtalkMarkdownLink("url"), "url") {
		t.Fatal("report link helpers changed")
	}

	ids := []map[string]any{{"reportId": " id "}, {"reportID": "id"}, {"report_id": "id"}, {"report_Id": "id"}, {"report-id": "id"}, {"reportId": 1}}
	for index, item := range ids {
		got := reportIDFromMap(item)
		if index < 5 && got != "id" {
			t.Errorf("reportIDFromMap(%#v) = %q", item, got)
		}
	}
	if reportIDFromCreateResponse(map[string]any{"result": " id "}) != "id" ||
		reportIDFromCreateResponse(map[string]any{"result": map[string]any{"report_id": "nested"}}) != "nested" ||
		reportIDFromCreateResponse(map[string]any{"reportId": "top"}) != "top" ||
		reportIDFromCreateResponse(map[string]any{}) != "" {
		t.Fatal("create response ID extraction changed")
	}
}

func TestCrossPlatformCoverageReportCreateEnrichmentBranches(t *testing.T) {
	caller := &reportTestCaller{format: "json", response: `{"result":{"url":"dingtalk://detail"}}`}
	out, _ := installReportTestDeps(t, caller)
	if enrichReportCreateWithDetailURL(context.Background(), []any{"unchanged"}) == nil {
		t.Fatal("non-map create response changed")
	}
	direct := enrichReportCreateWithDetailURL(context.Background(), map[string]any{"url": "dingtalk://direct"}).(map[string]any)
	if direct["dingtalkOpenUrl"] != "dingtalk://direct" {
		t.Fatalf("direct URL enrichment = %#v", direct)
	}
	missing := enrichReportCreateWithDetailURL(context.Background(), map[string]any{}).(map[string]any)
	if missing["urlLookupError"] == nil {
		t.Fatal("missing report ID had no lookup error")
	}
	resolved := enrichReportCreateWithDetailURL(context.Background(), map[string]any{"reportId": "id"}).(map[string]any)
	if resolved["url"] != "dingtalk://detail" {
		t.Fatalf("detail URL enrichment = %#v", resolved)
	}

	caller.response = "not-json"
	invalid := enrichReportCreateWithDetailURL(context.Background(), map[string]any{"reportId": "id"}).(map[string]any)
	if !strings.Contains(invalid["urlLookupError"].(string), "parse") {
		t.Fatalf("invalid detail response = %#v", invalid)
	}
	caller.response = `{}`
	noURL := enrichReportCreateWithDetailURL(context.Background(), map[string]any{"reportId": "id"}).(map[string]any)
	if noURL["urlLookupError"] == nil {
		t.Fatalf("detail without URL = %#v", noURL)
	}
	caller.err = errors.New("lookup failed")
	failed := enrichReportCreateWithDetailURL(context.Background(), map[string]any{"reportId": "id"}).(map[string]any)
	if !strings.Contains(failed["urlLookupError"].(string), "lookup failed") {
		t.Fatalf("detail lookup error = %#v", failed)
	}

	caller.err = nil
	caller.response = `{}`
	out.Reset()
	if err := printReportCreateWithDetailURL(context.Background(), "not-json"); err != nil || out.String() != "not-json\n" {
		t.Fatalf("raw create output = %q, %v", out.String(), err)
	}
	out.Reset()
	if err := printReportCreateWithDetailURL(context.Background(), `{}`); err != nil || !strings.Contains(out.String(), "urlLookupError") {
		t.Fatalf("JSON create output = %q, %v", out.String(), err)
	}
}

func TestCrossPlatformCoverageCallReportCreateWithDetailURLCoverage(t *testing.T) {
	caller := &reportTestCaller{dry: true, format: "json", response: `{"ok":true}`}
	installReportTestDeps(t, caller)
	_ = callReportCreateWithDetailURL(map[string]any{"template_id": "template"})

	caller.dry = false
	caller.response = `{"reportId":"id","url":"dingtalk://direct"}`
	if err := callReportCreateWithDetailURL(map[string]any{"template_id": "template"}); err != nil {
		t.Fatal(err)
	}
	caller.err = errors.New("create failed")
	if err := callReportCreateWithDetailURL(map[string]any{}); err == nil {
		t.Fatal("report create error was ignored")
	}
}

func TestCrossPlatformCoverageReportContentsValidationMatrix(t *testing.T) {
	valid := map[string]any{"key": " Done ", "sort": 1, "content": "", "type": "text", "contentType": "text"}
	if err := validateAndNormalizeReportContents([]map[string]any{valid}); err != nil {
		t.Fatalf("valid contents error = %v", err)
	}
	if valid["key"] != "Done" || valid["sort"] != "1" || valid["type"] != "1" || valid["contentType"] != "markdown" {
		t.Fatalf("normalized contents = %#v", valid)
	}
	validNonText := map[string]any{"key": "Number", "sort": json.Number("2"), "content": "3", "type": "number", "contentType": "raw"}
	if err := validateAndNormalizeReportContents([]map[string]any{validNonText}); err != nil || validNonText["contentType"] != "origin" {
		t.Fatalf("valid non-text = %#v, %v", validNonText, err)
	}

	invalid := [][]map[string]any{
		nil, {nil},
		{{"sort": 1, "content": "x", "type": "1", "contentType": "markdown"}},
		{{"key": 1, "sort": 1, "content": "x", "type": "1", "contentType": "markdown"}},
		{{"key": " ", "sort": 1, "content": "x", "type": "1", "contentType": "markdown"}},
		{{"key": "k", "content": "x", "type": "1", "contentType": "markdown"}},
		{{"key": "k", "sort": map[string]any{}, "content": "x", "type": "1", "contentType": "markdown"}},
		{{"key": "k", "sort": " ", "content": "x", "type": "1", "contentType": "markdown"}},
		{{"key": "k", "sort": "bad", "content": "x", "type": "1", "contentType": "markdown"}},
		{{"key": "k", "sort": 1, "type": "1", "contentType": "markdown"}},
		{{"key": "k", "sort": 1, "content": 1, "type": "1", "contentType": "markdown"}},
		{{"key": "k", "sort": 1, "content": "x", "contentType": "markdown"}},
		{{"key": "k", "sort": 1, "content": "x", "type": map[string]any{}, "contentType": "markdown"}},
		{{"key": "k", "sort": 1, "content": "x", "type": "bad", "contentType": "markdown"}},
		{{"key": "k", "sort": 1, "content": "x", "type": "1"}},
		{{"key": "k", "sort": 1, "content": "x", "type": "1", "contentType": 1}},
		{{"key": "k", "sort": 1, "content": "x", "type": "1", "contentType": "origin"}},
		{{"key": "k", "sort": 1, "content": "x", "type": "2", "contentType": "markdown"}},
	}
	for index, contents := range invalid {
		if err := validateAndNormalizeReportContents(contents); err == nil {
			t.Errorf("invalid contents case %d returned nil", index)
		}
	}

	for _, value := range []any{"1", float64(1), float64(1.5), float32(1), float32(1.5), int(1), int64(1), int32(1), json.Number("1"), json.Number("1.5"), true} {
		_, _ = reportScalarToString(value)
	}
	for _, value := range []any{"1", "text", "number", "single", "date", "multi", "image", "attachment", "bad", true} {
		_, _ = normalizeReportFieldType(value, 0)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestCrossPlatformCoverageReportContentsSourcesAndSafePaths(t *testing.T) {
	dir := t.TempDir()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := os.WriteFile("report.json", []byte(`[{}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	safe, err := resolveSafeReportContentsFilePath("report.json")
	if err != nil || !filepath.IsAbs(safe) {
		t.Fatalf("safe path = %q, %v", safe, err)
	}
	if got, err := readReportContentsFile(safe); err != nil || got != `[{}]` {
		t.Fatalf("read report = %q, %v", got, err)
	}
	for _, path := range []string{".", "-", "../outside.json", "missing.json"} {
		if _, err := resolveSafeReportContentsFilePath(path); err == nil {
			t.Errorf("unsafe path %q returned nil error", path)
		}
	}
	if _, err := resolveSafeReportContentsFilePath(dir); err == nil {
		t.Fatal("directory path returned nil error")
	}
	if _, err := readReportContentsFile("missing.json"); err == nil {
		t.Fatal("missing report file returned nil error")
	}
	if !pathEscapesUpward(filepath.Clean("..")) || !pathEscapesUpward(filepath.Clean("../x")) || pathEscapesUpward("inside") {
		t.Fatal("pathEscapesUpward changed")
	}
	if !pathWithinRoot(dir, filepath.Join(dir, "file")) || pathWithinRoot(dir, filepath.Join(filepath.Dir(dir), "outside")) {
		t.Fatal("pathWithinRoot changed")
	}

	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`[]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, "link.json"); err == nil {
		if _, err := resolveSafeReportContentsFilePath("link.json"); err == nil {
			t.Fatal("symlink escaping cwd returned nil error")
		}
	}

	if got, err := readReportContentsStdin(strings.NewReader("stdin")); err != nil || got != "stdin" {
		t.Fatalf("stdin contents = %q, %v", got, err)
	}
	if _, err := readReportContentsLimited(failingReader{}, "source"); err == nil {
		t.Fatal("reader error was ignored")
	}
	if _, err := readReportContentsLimited(io.LimitReader(strings.NewReader(strings.Repeat("x", reportContentsMaxBytes+1)), int64(reportContentsMaxBytes+1)), "large"); err == nil {
		t.Fatal("oversized contents returned nil error")
	}
	if cli, ok := reportContentsTooLargeError("large").(*CLIError); !ok || cli.Code != CodeInputTooLarge {
		t.Fatal("oversized error classification changed")
	}

	cmd := &cobra.Command{Use: "test"}
	addReportCreateFlags(cmd)
	cmd.SetIn(strings.NewReader("from stdin"))
	if err := cmd.Flags().Set("contents", "-"); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveReportContentsFromFlags(cmd); err != nil || got != "from stdin" {
		t.Fatalf("stdin flag contents = %q, %v", got, err)
	}
	_ = cmd.Flags().Set("contents", "literal")
	if got, err := resolveReportContentsFromFlags(cmd); err != nil || got != "literal" {
		t.Fatalf("literal contents = %q, %v", got, err)
	}
	_ = cmd.Flags().Set("contents-file", "report.json")
	if got, err := resolveReportContentsFromFlags(cmd); err != nil || got != `[{}]` {
		t.Fatalf("file contents = %q, %v", got, err)
	}
}

func TestCrossPlatformCoverageReportDispatchHintsAndDeprecation(t *testing.T) {
	caller := &reportTestCaller{format: "table"}
	out, _ := installReportTestDeps(t, caller)
	for _, operation := range []string{"template-list", "template-detail", "detail", "stats", "list", "sent", "create", "unknown"} {
		_ = reportDispatchHint(operation, "")
		_ = reportDispatchHint(operation, "parameter error")
	}
	if reportDispatchHint("create", "PARAM_ERROR") == "" || reportDispatchHint("unknown", "bad") != "" {
		t.Fatal("dispatch hint routing changed")
	}
	if err := withReportDispatchHint("list", nil); err != nil || !strings.Contains(out.String(), "可复用") {
		t.Fatalf("successful dispatch hint = %q, %v", out.String(), err)
	}
	caller.format = "json"
	if err := withReportDispatchHint("list", nil); err != nil {
		t.Fatal(err)
	}
	plain := errors.New("plain")
	if withReportDispatchHint("list", plain) != plain {
		t.Fatal("plain error was wrapped")
	}
	cli := &CLIError{Code: CodeMCPToolError, Message: "bad"}
	if got := withReportDispatchHint("list", cli); got != cli || cli.Suggestion == "" {
		t.Fatalf("CLI dispatch hint = %#v", got)
	}
	cli.Suggestion = "existing"
	_ = withReportDispatchHint("list", cli)
	if !strings.Contains(cli.Suggestion, "existing\n") {
		t.Fatalf("existing suggestion = %q", cli.Suggestion)
	}

	cmd := &cobra.Command{Use: "old"}
	errOut := &bytes.Buffer{}
	cmd.SetErr(errOut)
	called := false
	run := withReportDeprecationWarning("old", "new", func(*cobra.Command, []string) error { called = true; return nil })
	if err := run(cmd, nil); err != nil || !called || !strings.Contains(errOut.String(), "deprecated") {
		t.Fatalf("deprecation wrapper = %q, %v, %v", errOut.String(), called, err)
	}
	if got := parseReportUserIDs(" a, ,b "); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("parseReportUserIDs = %#v", got)
	}

	_ = time.Now() // keep time-zone dependent helpers exercised in this test file.
}
