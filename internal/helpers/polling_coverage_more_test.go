package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type scriptedToolStep struct {
	text string
	err  error
}

type scriptedToolCaller struct {
	steps  []scriptedToolStep
	index  int
	format string
	dry    bool
}

func (c *scriptedToolCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	if len(c.steps) == 0 {
		return &edition.ToolResult{}, nil
	}
	index := c.index
	if index >= len(c.steps) {
		index = len(c.steps) - 1
	}
	c.index++
	step := c.steps[index]
	if step.err != nil {
		return nil, step.err
	}
	return textToolResult(step.text), nil
}

func (c *scriptedToolCaller) Format() string { return c.format }
func (c *scriptedToolCaller) DryRun() bool   { return c.dry }
func (*scriptedToolCaller) Fields() string   { return "" }
func (*scriptedToolCaller) JQ() string       { return "" }

func installScriptedCaller(t *testing.T, caller *scriptedToolCaller) {
	t.Helper()
	previous := deps
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	t.Cleanup(func() { deps = previous })
}

func installImmediateTiming(t *testing.T) {
	t.Helper()
	previousSleep := helperSleep
	previousAfter := helperAfter
	helperSleep = func(time.Duration) {}
	helperAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	t.Cleanup(func() {
		helperSleep = previousSleep
		helperAfter = previousAfter
	})
}

func TestCrossPlatformCoverageDocPollingCoverage(t *testing.T) {
	installImmediateTiming(t)
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })
	cases := [][]scriptedToolStep{
		{{err: errors.New("query failed")}},
		{{text: `{`}},
		{{text: `{"status":"SUCCESS"}`}},
		{{text: `{"status":"SUCCESS","downloadUrl":"https://example.test/file"}`}},
		{{text: `{"status":"FAILED","message":"failed"}`}},
		{{text: `{"status":"FAILED"}`}},
		{{text: `{"status":"PROCESSING"}`}},
	}
	for _, steps := range cases {
		caller := &scriptedToolCaller{steps: steps}
		installScriptedCaller(t, caller)
		_, _ = pollDocExportJob(context.Background(), "job")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = pollDocExportJob(cancelled, "job")

	importCases := [][]scriptedToolStep{
		{{err: errors.New("query failed")}},
		{{text: `{`}},
		{{text: `{"status":"completed","nodeId":"node"}`}},
		{{text: `{"status":"failed","message":"failed"}`}},
		{{text: `{"status":"failed"}`}},
		{{text: `{"status":"processing"}`}},
		{{text: `{"status":"unknown"}`}},
	}
	importConfig := docImportFlowConfig()
	importConfig.poll.wait = func(ctx context.Context, _ time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	for _, steps := range importCases {
		caller := &scriptedToolCaller{steps: steps}
		installScriptedCaller(t, caller)
		_, _ = pollImportTask(context.Background(), "task", importConfig)
	}
	_, _ = pollImportTask(cancelled, "task", importConfig)
}

func TestCrossPlatformCoverageSheetExportPollingCoverage(t *testing.T) {
	installImmediateTiming(t)
	oldArgs := os.Args
	os.Args = []string{"dws", "sheet"}
	t.Cleanup(func() { os.Args = oldArgs })
	cases := []struct {
		format string
		steps  []scriptedToolStep
	}{
		{"table", []scriptedToolStep{{err: errors.New("temporary")}, {text: `{"status":"SUCCESS","downloadUrl":"https://example.test/file.xlsx"}`}}},
		{"json", []scriptedToolStep{{text: `{`}}},
		{"table", []scriptedToolStep{{text: `{"status":"SUCCESS"}`}}},
		{"table", []scriptedToolStep{{text: `{"status":"FAILED","message":"failed"}`}}},
		{"table", []scriptedToolStep{{text: `{"status":"FAIL"}`}}},
		{"table", []scriptedToolStep{{text: `{"status":"PROCESSING"}`}}},
		{"table", []scriptedToolStep{{text: `{"status":"MYSTERY"}`}}},
	}
	for _, tc := range cases {
		caller := &scriptedToolCaller{steps: tc.steps, format: tc.format}
		installScriptedCaller(t, caller)
		_, _ = pollSheetExportJob(context.Background(), "job")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = pollSheetExportJob(cancelled, "job")

	for _, raw := range []string{`{`, `{}`, `{"success":false}`, `{"success":false,"message":"failed"}`, `{"result":{"jobId":"job"}}`} {
		_, _ = parseExportSubmitResult(raw)
	}
	for _, raw := range []string{`{`, `{}`, `{"result":{"status":"SUCCESS","downloadUrl":"url","message":"ok"}}`} {
		_, _, _, _ = parseExportQueryResult(raw)
	}
	for _, raw := range []string{"", "https://example.test/", "https://example.test/a%2Fb.xlsx?x=1", "https://example.test/%"} {
		_ = inferSheetExportFilename(raw)
	}
}

func TestCrossPlatformCoverageChunkedWriteCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })

	cases := []struct {
		name      string
		tool      string
		args      map[string]any
		markdown  string
		chunkSize int
		steps     []scriptedToolStep
		cancel    bool
	}{
		{"create", "create_document", map[string]any{"title": "x"}, "abcdef", 3, []scriptedToolStep{{text: `{"nodeId":"node"}`}, {text: `{}`}}, false},
		{"create-error", "create_document", nil, "abcdef", 3, []scriptedToolStep{{err: errors.New("create failed")}}, false},
		{"create-missing-id", "create_document", nil, "abcdef", 3, []scriptedToolStep{{text: `{}`}}, false},
		{"update", "update_document", map[string]any{"nodeId": "node", "mode": "overwrite"}, "abcdef", 3, []scriptedToolStep{{text: `{}`}, {text: `{}`}}, false},
		{"update-first-error", "update_document", map[string]any{"nodeId": "node"}, "abcdef", 3, []scriptedToolStep{{err: errors.New("update failed")}}, false},
		{"update-later-error", "update_document", map[string]any{"nodeId": "node"}, "abcdef", 3, []scriptedToolStep{{text: `{}`}, {err: errors.New("update failed")}}, false},
		{"update-timeout-floor", "update_document", map[string]any{"nodeId": "node"}, strings.Repeat("x", 12000), 6000, []scriptedToolStep{{text: `{}`}, {err: errors.New("HSFTimeoutException")}}, false},
		{"cancelled", "update_document", map[string]any{"nodeId": "node"}, "abcdef", 3, []scriptedToolStep{{text: `{}`}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: tc.steps}
			installScriptedCaller(t, caller)
			ctx := context.Background()
			if tc.cancel {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			_, _, _, _ = chunkedWrite(ctx, tc.tool, tc.args, tc.markdown, "test", tc.chunkSize)
		})
	}
}

func TestCrossPlatformCoverageTodoListAutoPageCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "todo"}
	t.Cleanup(func() { os.Args = oldArgs })
	root := newTodoCommand()
	cmd, _, err := root.Find([]string{"task", "list"})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		caller *scriptedToolCaller
		page   string
		size   int
		role   string
	}{
		{name: "dry", caller: &scriptedToolCaller{dry: true}, page: "1", size: 45},
		{name: "json", caller: &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"result":{"todoCards":[{"id":1}]}}`}}}, page: "bad", size: 45},
		{name: "raw", caller: &scriptedToolCaller{format: "table", steps: []scriptedToolStep{{text: `{"result":{"todoCards":[{"id":1}]}}`}}}, page: "0", size: 45},
		{name: "empty", caller: &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"todoCards":[]}}`}}}, page: "1", size: 45},
		{name: "invalid-json", caller: &scriptedToolCaller{steps: []scriptedToolStep{{text: `{`}}}, page: "1", size: 45},
		{name: "call-error", caller: &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("failed")}}}, page: "1", size: 45},
		{name: "invalid-role", caller: &scriptedToolCaller{}, page: "1", size: 45, role: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installScriptedCaller(t, tc.caller)
			_ = cmd.Flags().Set("role-types", tc.role)
			_ = todoListAutoPage(cmd, tc.page, tc.size)
			_ = cmd.Flags().Set("role-types", "")
		})
	}
}

func TestCrossPlatformCoverageAitableRetryAndPaginationCoverage(t *testing.T) {
	installImmediateTiming(t)
	oldArgs := os.Args
	os.Args = []string{"dws", "aitable"}
	t.Cleanup(func() { os.Args = oldArgs })
	for _, steps := range [][]scriptedToolStep{
		{{err: errors.New("validation failed")}},
		{{err: errors.New("HTTP 500 temporary")}, {text: `{"success":true}`}},
		{{err: errors.New("HTTP 503 retryable")}},
	} {
		caller := &scriptedToolCaller{steps: steps, format: "json"}
		installScriptedCaller(t, caller)
		_ = callAitableTool("tool", nil)
		caller.index = 0
		_ = callAitableHelperTool("tool", nil)
	}

	for _, steps := range [][]scriptedToolStep{
		{{err: errors.New("first page failed")}},
		{{text: ""}},
		{{text: `{`}},
		{{text: `{"data":{"records":[{"id":"one"}],"nextCursor":"next"}}`}, {err: errors.New("second page failed")}},
		{{text: `{"records":[{"id":"one"}],"nextCursor":"next"}`}},
		{{text: `{"data":{"records":[{"id":"one"}],"cursor":"next"}}`}},
	} {
		caller := &scriptedToolCaller{steps: steps, format: "json"}
		installScriptedCaller(t, caller)
		_ = recordQueryFetchAll(map[string]any{}, 1)
	}
	for _, steps := range [][]scriptedToolStep{
		{{text: `{"data":{"records":[{"id":"one"}],"nextCursor":"next"}}`}, {err: errors.New("second page failed")}},
		{{text: `{"data":{"records":[{"id":"one"}],"cursor":"next"}}`}, {text: ""}},
		{{text: `{"records":[{"id":"one"}],"nextCursor":"next"}`}, {text: `{`}},
		{{text: `{"records":[{"id":"one"}],"cursor":"next"}`}, {text: `{"records":[]}`}},
	} {
		caller := &scriptedToolCaller{steps: steps, format: "json"}
		installScriptedCaller(t, caller)
		_ = recordQueryFetchAll(map[string]any{}, 0)
	}
}
