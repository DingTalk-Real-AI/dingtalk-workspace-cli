package helpers

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func executeSheetReadCompletionCommand(t *testing.T, caller *scriptedToolCaller, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	installScriptedCaller(t, caller)
	installSheetProductArgs(t)

	stdout := &bytes.Buffer{}
	deps.Out.w = stdout

	cmd := newSheetCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return stdout, cmd.Execute()
}

func decodeSheetReadOutput(t *testing.T, stdout *bytes.Buffer) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode sheet read output %q: %v", stdout.String(), err)
	}
	return got
}

func assertSheetReadCompletionMetadata(t *testing.T, got map[string]any) {
	t.Helper()
	if hasMore, ok := got["hasMore"].(bool); !ok || !hasMore {
		t.Fatalf("hasMore = %#v, want true", got["hasMore"])
	}
	if got["resolvedRange"] != "A1:A30001" {
		t.Fatalf("resolvedRange = %#v", got["resolvedRange"])
	}
	if got["returnedRange"] != "A1:A30000" {
		t.Fatalf("returnedRange = %#v", got["returnedRange"])
	}
	reasons, ok := got["truncationReasons"].([]any)
	if !ok || len(reasons) == 0 || reasons[0] != "max_cells" {
		t.Fatalf("truncationReasons = %#v", got["truncationReasons"])
	}
}

// csv-get intentionally projects the MCP response rather than inventing a
// client-side page model. Keep all completion metadata visible and make one
// request only, even when the server reports a partial success.
func TestSheetCSVGetPreservesCompletionMetadataWithoutAutoPagination(t *testing.T) {
	const response = `{"csv":"[row=1]1\n","colIndices":["A"],"rowIndices":[1],"hasMore":true,"truncationReasons":["max_cells","max_chars"],"resolvedRange":"A1:A30001","returnedRange":"A1:A30000","message":"partial"}`
	caller := &scriptedToolCaller{
		format: "raw",
		steps:  []scriptedToolStep{{text: response}},
	}

	stdout, err := executeSheetReadCompletionCommand(t, caller,
		"csv-get", "--node", "NODE")
	if err != nil {
		t.Fatalf("csv-get: %v", err)
	}
	if caller.calls != 1 || caller.tool != "get_range_as_csv" {
		t.Fatalf("calls/tool = %d/%q, want one get_range_as_csv call", caller.calls, caller.tool)
	}
	if _, exists := caller.args["range"]; exists {
		t.Fatalf("omitted range was unexpectedly synthesized by the CLI: %#v", caller.args["range"])
	}
	if got := strings.TrimSpace(stdout.String()); got != response {
		t.Fatalf("raw csv-get response changed:\n got: %s\nwant: %s", got, response)
	}

	got := decodeSheetReadOutput(t, stdout)
	assertSheetReadCompletionMetadata(t, got)
	reasons := got["truncationReasons"].([]any)
	if len(reasons) != 2 || reasons[1] != "max_chars" {
		t.Fatalf("truncationReasons = %#v, want max_cells and max_chars", reasons)
	}
}

// range read parses the get_cell_infos envelope to remove empty per-cell
// schema shells. That cleanup must not discard new top-level completion fields.
func TestSheetRangeReadPreservesCompletionMetadataWithoutAutoPagination(t *testing.T) {
	const response = `{"cells":[[{"value":"1","dataValidation":{"type":null},"hyperlink":{"url":null}}]],"colIndices":["A"],"rowIndices":[1],"hasMore":true,"truncationReasons":["max_cells"],"resolvedRange":"A1:A30001","returnedRange":"A1:A30000","message":"partial"}`
	caller := &scriptedToolCaller{
		format: "json",
		steps:  []scriptedToolStep{{text: response}},
	}

	stdout, err := executeSheetReadCompletionCommand(t, caller,
		"range", "read", "--node", "NODE")
	if err != nil {
		t.Fatalf("range read: %v", err)
	}
	if caller.calls != 1 || caller.tool != "get_cell_infos" {
		t.Fatalf("calls/tool = %d/%q, want one get_cell_infos call", caller.calls, caller.tool)
	}
	if _, exists := caller.args["range"]; exists {
		t.Fatalf("omitted range was unexpectedly synthesized by the CLI: %#v", caller.args["range"])
	}

	got := decodeSheetReadOutput(t, stdout)
	assertSheetReadCompletionMetadata(t, got)
	cells := got["cells"].([]any)
	row := cells[0].([]any)
	cell := row[0].(map[string]any)
	if _, exists := cell["dataValidation"]; exists {
		t.Fatalf("empty dataValidation shell was not removed: %#v", cell)
	}
	if _, exists := cell["hyperlink"]; exists {
		t.Fatalf("empty hyperlink shell was not removed: %#v", cell)
	}
}

// sheet +read calls get_cell_infos through this direct passthrough instead of
// callMCPToolCellInfos. Lock that third read surface to the same raw envelope
// contract and to one request only.
func TestSheetCellInfosDirectPassthroughPreservesCompletionMetadata(t *testing.T) {
	const response = `{"cells":[[{"value":"1"}]],"colIndices":["A"],"rowIndices":[1],"hasMore":true,"truncationReasons":["max_cells"],"resolvedRange":"A1:A30001","returnedRange":"A1:A30000","message":"partial"}`
	caller := &scriptedToolCaller{
		format: "raw",
		steps:  []scriptedToolStep{{text: response}},
	}
	installScriptedCaller(t, caller)
	stdout := &bytes.Buffer{}
	deps.Out.w = stdout

	if err := CallMCPToolOnServer("sheet", "get_cell_infos", map[string]any{"nodeId": "NODE"}); err != nil {
		t.Fatalf("direct get_cell_infos: %v", err)
	}
	if caller.calls != 1 || caller.server != "sheet" || caller.tool != "get_cell_infos" {
		t.Fatalf("calls/server/tool = %d/%q/%q", caller.calls, caller.server, caller.tool)
	}
	if got := strings.TrimSpace(stdout.String()); got != response {
		t.Fatalf("raw get_cell_infos response changed:\n got: %s\nwant: %s", got, response)
	}
	assertSheetReadCompletionMetadata(t, decodeSheetReadOutput(t, stdout))
}

// Large-CP failure is not a pageable success. Both the raw csv-get path and
// both get_cell_infos paths must retain the backend code and safe user message
// exactly, matching the generic get_all_sheets error-classification path.
//
// 两条路径的 Message 形态不同：csv-get / cell-infos-direct 走终端展示路径
// （callMCPToolInternalOptsContext），业务错误展示为提取的 errorMessage 加
// "(code: ...)" 后缀——后端错误码必须保持可见；range read 走数据编排路径
// （parseMCPToolTextResult），下游（如 drive list --depth 的限流重试）需要
// 从 Message 反解析 errorCode，因此必须保留原始 JSON payload。
func TestSheetReadPreservesWorkbookSizeOverLimitError(t *testing.T) {
	const response = `{"success":false,"errorCode":"forbidden.document.sizeOverLimit","errorMessage":"The workbook data is too large to process. Use a smaller copy or split the workbook, then try again."}`
	const wantDisplay = "The workbook data is too large to process. Use a smaller copy or split the workbook, then try again. (code: forbidden.document.sizeOverLimit)"
	tests := []struct {
		name   string
		args   []string
		tool   string
		direct bool
		want   string
	}{
		{name: "csv-get", args: []string{"csv-get", "--node", "NODE"}, tool: "get_range_as_csv", want: wantDisplay},
		{name: "range-read", args: []string{"range", "read", "--node", "NODE"}, tool: "get_cell_infos", want: response},
		{name: "cell-infos-direct", tool: "get_cell_infos", direct: true, want: wantDisplay},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedToolCaller{
				format: "json",
				steps:  []scriptedToolStep{{text: response}},
			}
			var stdout *bytes.Buffer
			var err error
			if test.direct {
				installScriptedCaller(t, caller)
				stdout = &bytes.Buffer{}
				deps.Out.w = stdout
				err = CallMCPToolOnServer("sheet", test.tool, map[string]any{"nodeId": "NODE"})
			} else {
				stdout, err = executeSheetReadCompletionCommand(t, caller, test.args...)
			}
			if err == nil {
				t.Fatal("large-CP response was accepted as success")
			}
			var cliErr *CLIError
			if !errors.As(err, &cliErr) || cliErr.Code != CodeMCPToolError {
				t.Fatalf("error = %T %v, want MCP tool error", err, err)
			}
			if cliErr.Message != test.want {
				t.Fatalf("backend error payload changed:\n got: %s\nwant: %s", cliErr.Message, test.want)
			}
			for _, want := range []string{
				"forbidden.document.sizeOverLimit",
				"The workbook data is too large to process.",
			} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not preserve %q", err, want)
				}
			}
			if stdout.Len() != 0 {
				t.Fatalf("failure wrote a success body: %q", stdout.String())
			}
			if caller.calls != 1 || caller.tool != test.tool {
				t.Fatalf("calls/tool = %d/%q, want one %s call", caller.calls, caller.tool, test.tool)
			}
		})
	}
}
