package helpers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func mustBatchOperationsJSON(t *testing.T, operations []any) string {
	t.Helper()
	encoded, err := json.Marshal(operations)
	if err != nil {
		t.Fatalf("marshal expected batch operations: %v", err)
	}
	return string(encoded)
}

func decodeBatchOperationsJSON(t *testing.T, args map[string]any) []any {
	t.Helper()
	if _, exists := args["operations"]; exists {
		t.Fatalf("legacy operations field must be omitted: %#v", args)
	}
	raw, ok := args["operationsJson"].(string)
	if !ok || raw == "" {
		t.Fatalf("operationsJson = %#v, want non-empty string", args["operationsJson"])
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	var operations []any
	if err := decoder.Decode(&operations); err != nil {
		t.Fatalf("decode operationsJson: %v\n%s", err, raw)
	}
	return operations
}

func TestCrossPlatformCoverageBuildBatchUpdateToolArgsPreservesJSONTypesAndText(t *testing.T) {
	csv := "name,note,formula\n中文,\"a,b\",=1+1\nquote,\"say \"\"hi\"\"\",'=1+1"
	operations := []any{
		map[string]any{
			"toolName": "insert_dimension",
			"input":    map[string]any{"sheetId": "Sheet1", "length": 2, "hidden": false},
		},
		map[string]any{
			"toolName": "set_cell_range",
			"input": map[string]any{
				"sheetId": "Sheet1",
				"cells":   []any{[]any{map[string]any{"cellStyle": map[string]any{"bold": true}}}},
			},
		},
		map[string]any{
			"toolName": "set_range_from_csv",
			"input":    map[string]any{"sheetId": "Sheet1", "csv": csv, "autoConvert": false},
		},
	}

	args, err := buildBatchUpdateToolArgs("node-1", operations, true)
	if err != nil {
		t.Fatal(err)
	}
	if args["nodeId"] != "node-1" || args["continueOnError"] != true {
		t.Fatalf("batch args = %#v", args)
	}
	decoded := decodeBatchOperationsJSON(t, args)
	insertInput := decoded[0].(map[string]any)["input"].(map[string]any)
	if length, ok := insertInput["length"].(json.Number); !ok || length.String() != "2" {
		t.Fatalf("length = %#v, want JSON number 2", insertInput["length"])
	}
	if hidden, ok := insertInput["hidden"].(bool); !ok || hidden {
		t.Fatalf("hidden = %#v, want JSON boolean false", insertInput["hidden"])
	}
	style := decoded[1].(map[string]any)["input"].(map[string]any)["cells"].([]any)[0].([]any)[0].(map[string]any)["cellStyle"].(map[string]any)
	if bold, ok := style["bold"].(bool); !ok || !bold {
		t.Fatalf("bold = %#v, want JSON boolean true", style["bold"])
	}
	csvInput := decoded[2].(map[string]any)["input"].(map[string]any)
	if csvInput["csv"] != csv || csvInput["autoConvert"] != false {
		t.Fatalf("CSV operation changed after round trip: %#v", csvInput)
	}

	withoutContinue, err := buildBatchUpdateToolArgs("node-1", operations, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := withoutContinue["continueOnError"]; exists {
		t.Fatalf("false continueOnError must be omitted: %#v", withoutContinue)
	}
	if _, err := buildBatchUpdateToolArgs("node-1", []any{func() {}}, false); err == nil {
		t.Fatal("unsupported JSON value should fail before a remote call")
	}
}

func TestCrossPlatformCoverageSheetBatchOperationTranslationCoversEveryMapping(t *testing.T) {
	samples := map[string]map[string]any{
		"range clear":        {"sheet-id": "Sheet1", "range": "A1:B2", "type": "all"},
		"range update":       {"sheet-id": "Sheet1", "range": "A1", "values": []any{"value"}},
		"merge-cells":        {"sheet-id": "Sheet1", "range": "A1:B2", "merge-type": "mergeRows"},
		"unmerge-cells":      {"sheet-id": "Sheet1", "range": "A1:B2"},
		"range fill":         {"sheet-id": "Sheet1", "source-range": "A1", "target-range": "B2"},
		"range copy-to":      {"sheet-id": "Sheet1", "source-range": "A1", "target-range": "B2", "target-sheet-id": "Sheet2", "paste-type": "values"},
		"add-dimension":      {"sheet-id": "Sheet1", "dimension": "ROWS", "length": float64(2)},
		"delete-dimension":   {"sheet-id": "Sheet1", "dimension": "ROWS", "position": 1, "length": float64(2)},
		"move-dimension":     {"sheet-id": "Sheet1", "dimension": "ROWS", "start-index": 1, "end-index": "2", "destination-index": float64(4)},
		"set-dropdown":       {"sheet-id": "Sheet1", "range": "A1:B2", "options": []string{"a", "b"}, "multi-select": true},
		"delete-dropdown":    {"sheet-id": "Sheet1", "range": "A1:B2"},
		"csv-put":            {"sheet-id": "Sheet1", "csv": "a,b\r\n1,2", "start-cell": "A1", "allow-overwrite": true},
		"delete-float-image": {"sheet-id": "Sheet1", "float-image-id": "image-id"},
		"update-dimension":   {"sheet-id": "Sheet1", "dimension": "ROWS", "start-index": "1", "length": 2, "pixel-size": "24", "hidden": true},
		"group-dimension":    {"sheet-id": "Sheet1", "range": "1:2", "group-state": "expand"},
		"ungroup-dimension":  {"sheet-id": "Sheet1", "range": "1:2"},
		"range set-style":    {"sheet-id": "Sheet1", "range": "A1:B2", "font-weight": "bold"},
		"replace":            {"sheet-id": "Sheet1", "find": "old", "replacement": ""},
		"insert-dimension":   {"sheet-id": "Sheet1", "dimension": "ROWS", "position": "3", "length": float64(2)},
		"range move-to":      {"sheet-id": "Sheet1", "source-range": "A1", "target-range": "B2"},
		"range sort":         {"sheet-id": "Sheet1", "range": "A1:B2", "sort-keys": []any{map[string]any{"column": "A", "ascending": true}}},
		"new":                {"name": "New sheet"},
		"delete-sheet":       {"sheet-id": "Sheet1"},
		"update":             {"sheet-id": "Sheet1", "hidden": false, "frozen-row-count": float64(0)},
		"show-gridline":      {"sheet-id": "Sheet1"},
		"hide-gridline":      {"sheet-id": "Sheet1"},
		"chart delete":       {"sheet-id": "Sheet1", "chart-id": "chart-id"},
		"pivot-table delete": {"sheet-id": "Sheet1", "pivot-table-id": "pivot-id"},
		"cond-format create": {"sheet-id": "Sheet1", "ranges": []any{"A1:A3"}, "condition": map[string]any{"numberCondition": map[string]any{"operator": "greater", "value1": "1"}}},
		"cond-format update": {"sheet-id": "Sheet1", "rule-id": "rule-id", "cell-style": map[string]any{"bold": true}},
		"cond-format delete": {"sheet-id": "Sheet1", "rule-id": "rule-id"},
		"filter create":      {"sheet-id": "Sheet1", "range": "A1:B3"},
		"filter update":      {"sheet-id": "Sheet1", "criteria": []any{map[string]any{"column": float64(0), "filterType": "values", "visibleValues": []any{"x"}}}},
		"filter delete":      {"sheet-id": "Sheet1"},
		"filter-view create": {"sheet-id": "Sheet1", "name": "view", "range": "A1:B3"},
		"filter-view update": {"sheet-id": "Sheet1", "filter-view-id": "view-id", "name": "renamed"},
		"filter-view delete": {"sheet-id": "Sheet1", "filter-view-id": "view-id"},
		"create-float-image": {"sheet-id": "Sheet1", "src": "/resource", "range": "A1", "width": float64(100), "height": float64(80)},
		"update-float-image": {"sheet-id": "Sheet1", "float-image-id": "image-id", "offset-x": float64(0)},
	}
	if got := len(batchOpDispatch); got != 39 {
		t.Fatalf("batch dispatch size = %d, want 39", got)
	}
	if got := len(samples); got != len(batchOpDispatch) {
		t.Fatalf("sample count = %d, dispatch count = %d", got, len(batchOpDispatch))
	}
	mcpTools := make(map[string]struct{})
	for name, mapping := range batchOpDispatch {
		mcpTools[mapping.mcpTool] = struct{}{}
		opInput, exists := samples[name]
		if !exists {
			t.Errorf("missing valid sample for %q", name)
			continue
		}
		got, err := translateBatchOp(map[string]any{"toolName": name, "input": opInput})
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
	if got := len(mcpTools); got != 37 {
		t.Fatalf("unique MCP tool count = %d, want 37", got)
	}
	if batchOpDispatch["range update"].mcpTool != batchOpDispatch["range set-style"].mcpTool {
		t.Fatal("range update and range set-style must share set_cell_range")
	}
	if batchOpDispatch["show-gridline"].mcpTool != batchOpDispatch["hide-gridline"].mcpTool {
		t.Fatal("show-gridline and hide-gridline must share set_gridline_visibility")
	}
	if _, err := translateBatchOp(map[string]any{"toolName": "unknown"}); err == nil {
		t.Fatal("unknown batch operation should fail")
	}
	if _, err := translateBatchOp(map[string]any{"toolName": "range clear"}); err == nil {
		t.Fatal("missing input should fail")
	}
}

func TestCrossPlatformCoverageSheetBatchDimensionA1Translation(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    map[string]any
		want     map[string]any
	}{
		{
			name:     "delete rows string",
			toolName: "delete-dimension",
			input:    map[string]any{"sheet-id": "sheet", "dimension": "ROWS", "position": "3", "length": 2},
			want:     map[string]any{"sheetId": "sheet", "dimension": "ROWS", "startIndex": 2, "count": 2},
		},
		{
			name:     "delete rows JSON number",
			toolName: "delete-dimension",
			input:    map[string]any{"sheet-id": "sheet", "dimension": "rows", "position": float64(3), "length": float64(2)},
			want:     map[string]any{"sheetId": "sheet", "dimension": "ROWS", "startIndex": 2, "count": 2},
		},
		{
			name:     "delete columns",
			toolName: "delete-dimension",
			input:    map[string]any{"sheet-id": "sheet", "dimension": "COLUMNS", "position": "ab", "length": 1},
			want:     map[string]any{"sheetId": "sheet", "dimension": "COLUMNS", "startIndex": 27, "count": 1},
		},
		{
			name:     "delete sheet prefix",
			toolName: "delete-dimension",
			input:    map[string]any{"sheet-id": "ignored", "dimension": "COLUMNS", "position": " Source ! c ", "length": 1},
			want:     map[string]any{"sheetId": "Source", "dimension": "COLUMNS", "startIndex": 2, "count": 1},
		},
		{
			name:     "delete raw compatibility",
			toolName: "delete-dimension",
			input:    map[string]any{"sheet-id": "sheet", "dimension": "ROWS", "startIndex": float64(7), "count": float64(2)},
			want:     map[string]any{"sheetId": "sheet", "dimension": "ROWS", "startIndex": 7, "count": 2},
		},
		{
			name:     "move rows",
			toolName: "move-dimension",
			input:    map[string]any{"sheet-id": "sheet", "dimension": "ROWS", "start-index": "2", "end-index": "4", "destination-index": "1"},
			want:     map[string]any{"sheetId": "sheet", "dimension": "ROWS", "startIndex": 1, "endIndex": 3, "destinationIndex": 0},
		},
		{
			name:     "move columns",
			toolName: "move-dimension",
			input:    map[string]any{"sheet-id": "sheet", "dimension": "COLUMNS", "start-index": "B", "end-index": "c", "destination-index": "D"},
			want:     map[string]any{"sheetId": "sheet", "dimension": "COLUMNS", "startIndex": 1, "endIndex": 2, "destinationIndex": 3},
		},
		{
			name:     "move raw compatibility",
			toolName: "move-dimension",
			input:    map[string]any{"sheet-id": "sheet", "dimension": "ROWS", "startIndex": float64(7), "endIndex": float64(8), "destinationIndex": float64(9)},
			want:     map[string]any{"sheetId": "sheet", "dimension": "ROWS", "startIndex": 7, "endIndex": 8, "destinationIndex": 9},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			translated, err := translateBatchOp(map[string]any{"toolName": tc.toolName, "input": tc.input})
			if err != nil {
				t.Fatal(err)
			}
			got := translated["input"].(map[string]any)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("input = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSheetBatchDimensionA1Validation(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    map[string]any
		want     string
	}{
		{"dimension", "delete-dimension", map[string]any{"sheet-id": "s", "dimension": "PAGES", "position": "1", "length": 1}, "ROWS 或 COLUMNS"},
		{"row zero", "delete-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "position": "0", "length": 1}, "1-based"},
		{"row suffix", "delete-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "position": "3x", "length": 1}, "1-based"},
		{"column cell", "delete-dimension", map[string]any{"sheet-id": "s", "dimension": "COLUMNS", "position": "A1", "length": 1}, "列字母"},
		{"empty prefix", "delete-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "position": "!3", "length": 1}, "前缀格式非法"},
		{"empty position after prefix", "delete-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "position": "Sheet!", "length": 1}, "前缀格式非法"},
		{"delete public raw mix", "delete-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "position": "2", "length": 1, "startIndex": 1}, "不能混用"},
		{"delete missing length", "delete-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "position": "2"}, "length 必填"},
		{"move missing destination", "move-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "start-index": "1", "end-index": "2"}, "destination-index 必填"},
		{"move reversed", "move-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "start-index": "3", "end-index": "2", "destination-index": "1"}, "大于等于"},
		{"move destination inside", "move-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "start-index": "2", "end-index": "4", "destination-index": "3"}, "不能落在源范围"},
		{"move numeric column", "move-dimension", map[string]any{"sheet-id": "s", "dimension": "COLUMNS", "start-index": 1, "end-index": "B", "destination-index": "C"}, "列字母字符串"},
		{"move sheet prefix", "move-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "start-index": "Sheet!1", "end-index": "2", "destination-index": "3"}, "不支持工作表前缀"},
		{"move public raw mix", "move-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "start-index": "1", "end-index": "2", "destinationIndex": 3}, "不能混用"},
		{"unknown field", "move-dimension", map[string]any{"sheet-id": "s", "dimension": "ROWS", "start-index": "1", "end-index": "2", "destination-index": "3", "yes": true}, "未知字段"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := translateBatchOp(map[string]any{"toolName": tc.toolName, "input": tc.input})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestSheetBatchP0P1StrictValidation(t *testing.T) {
	tests := []struct {
		name string
		op   map[string]any
		want string
	}{
		{"tool-name-type", map[string]any{"toolName": 1, "input": map[string]any{}}, "非空字符串"},
		{"input-type", map[string]any{"toolName": "new", "input": "{}"}, "input 必须是 object"},
		{"operation-unknown", map[string]any{"toolName": "new", "input": map[string]any{"name": "x"}, "yes": true}, "未知字段"},
		{"input-unknown", map[string]any{"toolName": "new", "input": map[string]any{"name": "x", "node": "n"}}, "未知字段"},
		{"replacement-required", map[string]any{"toolName": "replace", "input": map[string]any{"sheet-id": "s", "find": "x"}}, "replacement 必填"},
		{"integer-string", map[string]any{"toolName": "insert-dimension", "input": map[string]any{"sheet-id": "s", "dimension": "ROWS", "position": "1", "length": "3"}}, "必须是整数"},
		{"integer-fraction", map[string]any{"toolName": "insert-dimension", "input": map[string]any{"sheet-id": "s", "dimension": "ROWS", "position": "1", "length": 1.5}}, "必须是整数"},
		{"integer-limit", map[string]any{"toolName": "insert-dimension", "input": map[string]any{"sheet-id": "s", "dimension": "ROWS", "position": "1", "length": 5001}}, "1~5000"},
		{"bool-string", map[string]any{"toolName": "update", "input": map[string]any{"sheet-id": "s", "hidden": "false"}}, "必须是布尔值"},
		{"update-mixed", map[string]any{"toolName": "update", "input": map[string]any{"sheet-id": "s", "index": 0, "hidden": false}}, "index 只能单独更新"},
		{"float-image-file", map[string]any{"toolName": "create-float-image", "input": map[string]any{"file": "x"}}, "media-upload"},
		{"sort-empty", map[string]any{"toolName": "range sort", "input": map[string]any{"sheet-id": "s", "range": "A1:B2", "sort-keys": []any{}}}, "不能为空数组"},
		{"filter-update-empty", map[string]any{"toolName": "filter update", "input": map[string]any{"sheet-id": "s", "criteria": []any{}}}, "不能为空数组"},
		{"style-shape", map[string]any{"toolName": "range set-style", "input": map[string]any{"sheet-id": "s", "range": "A1:B2", "bg-colors-json": []any{[]any{"#fff"}}}}, "维度与 range 不一致"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := translateBatchOp(tc.op)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestSheetBatchP0P1PreservesExplicitValuesAndNormalizesJSON(t *testing.T) {
	updated, err := translateBatchOp(map[string]any{
		"toolName": "update",
		"input": map[string]any{
			"sheet-id": "s", "hidden": false, "frozen-row-count": float64(0), "tab-color": "",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	updateInput := updated["input"].(map[string]any)
	if updateInput["hidden"] != false || updateInput["frozenRowCount"] != 0 {
		t.Fatalf("explicit false/zero lost: %#v", updateInput)
	}
	if value, ok := updateInput["tabColor"]; !ok || value != "" {
		t.Fatalf("explicit empty tab color lost: %#v", updateInput)
	}

	replaced, err := translateBatchOp(map[string]any{
		"toolName": "replace",
		"input":    map[string]any{"sheet-id": "s", "find": "x", "replacement": ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := replaced["input"].(map[string]any)["replaceText"]; got != "" {
		t.Fatalf("replaceText = %#v", got)
	}

	sorted, err := translateBatchOp(map[string]any{
		"toolName": "range sort",
		"input": map[string]any{
			"sheet-id": "s", "range": "A1:B2",
			"sort-keys": `[{"column":"A","ascending":true}]`, "has-header": false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sortInput := sorted["input"].(map[string]any)
	if _, ok := sortInput["sortKeys"].([]any); !ok {
		t.Fatalf("sortKeys not normalized array: %#v", sortInput["sortKeys"])
	}
	if sortInput["hasHeader"] != false {
		t.Fatalf("explicit false hasHeader lost: %#v", sortInput)
	}
}

func TestSheetBatchP0P1GeneratedRequestContracts(t *testing.T) {
	tests := []struct {
		cliTool string
		input   map[string]any
		mcpTool string
		want    map[string]any
	}{
		{"new", map[string]any{"name": "A"}, "create_sheet", map[string]any{"name": "A"}},
		{"delete-sheet", map[string]any{"sheet-id": "s"}, "delete_sheet", map[string]any{"sheetId": "s"}},
		{"show-gridline", map[string]any{"sheet-id": "s"}, "set_gridline_visibility", map[string]any{"sheetId": "s", "visibility": "visible"}},
		{"chart delete", map[string]any{"sheet-id": "s", "chart-id": "c"}, "delete_float_chart", map[string]any{"sheetId": "s", "floatChartId": "c"}},
		{"pivot-table delete", map[string]any{"sheet-id": "s", "pivot-table-id": "p"}, "delete_pivot_table", map[string]any{"sheetId": "s", "pivotTableId": "p"}},
		{"filter-view delete", map[string]any{"sheet-id": "s", "filter-view-id": "v"}, "delete_filter_view", map[string]any{"sheetId": "s", "filterViewId": "v"}},
	}
	for _, tc := range tests {
		got, err := translateBatchOp(map[string]any{"toolName": tc.cliTool, "input": tc.input})
		if err != nil {
			t.Fatalf("%s: %v", tc.cliTool, err)
		}
		if got["toolName"] != tc.mcpTool {
			t.Fatalf("%s MCP tool = %v", tc.cliTool, got["toolName"])
		}
		actual := got["input"].(map[string]any)
		if len(actual) != len(tc.want) {
			t.Fatalf("%s input = %#v, want %#v", tc.cliTool, actual, tc.want)
		}
		for key, want := range tc.want {
			if actual[key] != want {
				t.Fatalf("%s input[%s] = %#v, want %#v", tc.cliTool, key, actual[key], want)
			}
		}
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

func TestCrossPlatformCoverageSheetBatchDestructiveExamplesDoNotBypassConfirmation(t *testing.T) {
	for _, test := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "batch update", cmd: newBatchUpdateCmd()},
		{name: "batch clear", cmd: newRangeBatchClearCmd()},
	} {
		t.Run(test.name, func(t *testing.T) {
			if strings.Contains(test.cmd.Example, "--yes") {
				t.Fatalf("stored destructive example bypasses confirmation:\n%s", test.cmd.Example)
			}
			if !strings.Contains(test.cmd.Long, "获得用户确认后加 --yes") {
				t.Fatalf("help must explain how to proceed after explicit confirmation:\n%s", test.cmd.Long)
			}
		})
	}
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
	caller := &scriptedToolCaller{}
	if err := executeSheetBatchCommand(t, caller, batchUpdateCoverageCommand(), "--node", "node", "--operations", valid); err != nil {
		t.Fatalf("batch update success: %v", err)
	}
	if caller.calls != 1 || caller.tool != "batch_update" {
		t.Fatalf("batch calls = %d, tool = %q", caller.calls, caller.tool)
	}
	decoded := decodeBatchOperationsJSON(t, caller.args)
	if len(decoded) != 1 || decoded[0].(map[string]any)["toolName"] != "fill_range" {
		t.Fatalf("translated operationsJson = %#v", decoded)
	}
	boom := errors.New("batch failed")
	strictCaller := &scriptedToolCaller{steps: []scriptedToolStep{{err: boom}}}
	if err := executeSheetBatchCommand(t, strictCaller, batchUpdateCoverageCommand(), "--node", "node", "--operations", valid); err == nil {
		t.Fatal("strict batch error returned nil")
	}
	if strictCaller.calls != 1 {
		t.Fatalf("strict failed write retried %d times, want exactly one call", strictCaller.calls)
	}
	lenientCaller := &scriptedToolCaller{steps: []scriptedToolStep{{err: boom}}}
	if err := executeSheetBatchCommand(t, lenientCaller, batchUpdateCoverageCommand(), "--node", "node", "--operations", valid, "--continue-on-error"); err == nil {
		t.Fatal("lenient batch error returned nil")
	}
	if lenientCaller.calls != 1 {
		t.Fatalf("lenient failed write retried %d times, want exactly one call", lenientCaller.calls)
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
