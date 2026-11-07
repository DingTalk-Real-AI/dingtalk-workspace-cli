package helpers

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func requireSheetBatchError(t *testing.T, name, want string, call func() error) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		err := call()
		if err == nil || want != "" && !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want substring %q", err, want)
		}
	})
}

func TestCrossPlatformCoverageSheetBatchStrictHelperBranches(t *testing.T) {
	requireSheetBatchError(t, "dimension required", "dimension 必填", func() error {
		_, err := batchDimension(nil)
		return err
	})
	for _, test := range []struct {
		name      string
		value     any
		dimension string
		prefix    bool
		want      string
	}{
		{"row numeric parse", 1.5, "ROWS", false, "必须是整数"},
		{"column numeric", 1, "COLUMNS", false, "列字母字符串"},
		{"empty", " ", "ROWS", false, "不能为空"},
		{"duplicate bang", "Sheet!1!2", "ROWS", true, "前缀格式非法"},
		{"column overflow", strings.Repeat("Z", 20), "COLUMNS", false, "超出支持范围"},
	} {
		test := test
		requireSheetBatchError(t, "dimension index "+test.name, test.want, func() error {
			_, _, err := batchDimensionA1Index(test.value, test.dimension, "position", test.prefix)
			return err
		})
	}

	for _, test := range []struct {
		name  string
		input map[string]any
		want  string
	}{
		{"delete unknown", map[string]any{"unknown": true}, "未知字段"},
		{"delete sheet", map[string]any{"dimension": "ROWS", "position": 1, "length": 1}, "sheet-id 必填"},
		{"delete position", map[string]any{"sheet-id": "s", "dimension": "ROWS", "length": 1}, "position 必填"},
		{"delete mode", map[string]any{"sheet-id": "s", "dimension": "ROWS"}, "position/length 必填"},
		{"delete raw count", map[string]any{"sheet-id": "s", "dimension": "ROWS", "startIndex": 0}, "count 必填"},
	} {
		test := test
		requireSheetBatchError(t, test.name, test.want, func() error {
			_, err := BuildBatchDeleteDimensionArgs(test.input)
			return err
		})
	}
	for _, test := range []struct {
		name  string
		input map[string]any
		want  string
	}{
		{"move sheet", map[string]any{"dimension": "ROWS"}, "sheet-id 必填"},
		{"move dimension", map[string]any{"sheet-id": "s"}, "dimension 必填"},
		{"move mode", map[string]any{"sheet-id": "s", "dimension": "ROWS"}, "必填"},
		{"move too many", map[string]any{"sheet-id": "s", "dimension": "ROWS", "start-index": "1", "end-index": "5001", "destination-index": "5002"}, "最多包含 5000"},
	} {
		test := test
		requireSheetBatchError(t, test.name, test.want, func() error {
			_, err := BuildBatchMoveDimensionArgs(test.input)
			return err
		})
	}

	requireSheetBatchError(t, "required string type", "必须是字符串", func() error {
		_, err := requiredBatchString(map[string]any{"value": 1}, "value")
		return err
	})
	requireSheetBatchError(t, "optional string type", "必须是字符串", func() error {
		_, _, err := optionalBatchString(map[string]any{"value": 1}, "value")
		return err
	})
	requireSheetBatchError(t, "optional bool type", "必须是布尔值", func() error {
		_, _, err := optionalBatchBool(map[string]any{"value": "true"}, "value")
		return err
	})

	integerValues := []any{int8(1), int16(2), int32(3), int64(4), uint(5), uint8(6), uint16(7), uint32(8), uint64(9), float32(10), json.Number("11")}
	for _, value := range integerValues {
		got, exists, err := batchStrictInt(map[string]any{"value": value}, "value", 0, 20, true)
		if err != nil || !exists || got < 1 || got > 11 {
			t.Errorf("batchStrictInt(%T(%v)) = %d, %v, %v", value, value, got, exists, err)
		}
	}
	if strconv.IntSize == 64 {
		requireSheetBatchError(t, "uint overflow", "超出整数范围", func() error {
			_, _, err := batchStrictInt(map[string]any{"value": ^uint(0)}, "value", 0, math.MaxInt, true)
			return err
		})
	}
	requireSheetBatchError(t, "uint64 overflow", "超出整数范围", func() error {
		_, _, err := batchStrictInt(map[string]any{"value": uint64(math.MaxInt64) + 1}, "value", 0, math.MaxInt, true)
		return err
	})
	requireSheetBatchError(t, "float32 fraction", "必须是整数", func() error {
		_, _, err := batchStrictInt(map[string]any{"value": float32(1.5)}, "value", 0, 10, true)
		return err
	})
	requireSheetBatchError(t, "json number", "必须是整数", func() error {
		_, _, err := batchStrictInt(map[string]any{"value": json.Number("1.5")}, "value", 0, 10, true)
		return err
	})

	requireSheetBatchError(t, "array required", "必填", func() error {
		_, _, err := batchJSONArray(nil, "value", true)
		return err
	})
	requireSheetBatchError(t, "array marshal", "JSON 编码失败", func() error {
		_, _, err := batchJSONArray(map[string]any{"value": func() {}}, "value", false)
		return err
	})
	requireSheetBatchError(t, "array decode", "JSON 数组", func() error {
		_, _, err := batchJSONArray(map[string]any{"value": "{"}, "value", false)
		return err
	})
	requireSheetBatchError(t, "object required", "必填", func() error {
		_, _, err := batchJSONObject(nil, "value", true)
		return err
	})
	if object, exists, err := batchJSONObject(map[string]any{"value": `{"a":1}`}, "value", true); err != nil || !exists || object["a"] != float64(1) {
		t.Fatalf("string object = %#v, %v, %v", object, exists, err)
	}
	requireSheetBatchError(t, "object marshal", "JSON 编码失败", func() error {
		_, _, err := batchJSONObject(map[string]any{"value": func() {}}, "value", false)
		return err
	})
	requireSheetBatchError(t, "object decode", "JSON object", func() error {
		_, _, err := batchJSONObject(map[string]any{"value": "["}, "value", false)
		return err
	})
	requireSheetBatchError(t, "object empty", "不能为空 object", func() error {
		_, _, err := batchJSONObject(map[string]any{"value": "null"}, "value", false)
		return err
	})
	requireSheetBatchError(t, "json text marshal", "JSON 编码失败", func() error {
		_, _, err := batchJSONText(map[string]any{"value": func() {}}, "value")
		return err
	})
}

func TestCrossPlatformCoverageSheetBatchBuilderErrorBranches(t *testing.T) {
	type builder = batchArgsBuilder
	tests := []struct {
		name  string
		build builder
		input map[string]any
		want  string
	}{
		{"style unknown", BuildBatchSetStyleArgs, map[string]any{"unknown": true}, "未知字段"},
		{"style sheet", BuildBatchSetStyleArgs, map[string]any{"range": "A1"}, "sheet-id 必填"},
		{"style range", BuildBatchSetStyleArgs, map[string]any{"sheet-id": "s"}, "range 必填"},
		{"style string", BuildBatchSetStyleArgs, map[string]any{"sheet-id": "s", "range": "A1", "bg-color": 1}, "必须是字符串"},
		{"style font size", BuildBatchSetStyleArgs, map[string]any{"sheet-id": "s", "range": "A1", "font-size": "12"}, "必须是整数"},
		{"style JSON", BuildBatchSetStyleArgs, map[string]any{"sheet-id": "s", "range": "A1", "bg-colors-json": func() {}}, "JSON 编码失败"},
		{"replace unknown", BuildBatchReplaceArgs, map[string]any{"unknown": true}, "未知字段"},
		{"replace replacement", BuildBatchReplaceArgs, map[string]any{"sheet-id": "s", "find": "x", "replacement": 1}, "必须是字符串"},
		{"replace range", BuildBatchReplaceArgs, map[string]any{"sheet-id": "s", "find": "x", "replacement": "", "range": 1}, "必须是字符串"},
		{"replace bool", BuildBatchReplaceArgs, map[string]any{"sheet-id": "s", "find": "x", "replacement": "", "match-case": "false"}, "必须是布尔值"},
		{"insert unknown", BuildBatchInsertDimensionArgs, map[string]any{"unknown": true}, "未知字段"},
		{"insert sheet", BuildBatchInsertDimensionArgs, map[string]any{}, "sheet-id 必填"},
		{"insert dimension", BuildBatchInsertDimensionArgs, map[string]any{"sheet-id": "s"}, "dimension 必填"},
		{"insert dimension value", BuildBatchInsertDimensionArgs, map[string]any{"sheet-id": "s", "dimension": "PAGES"}, "ROWS 或 COLUMNS"},
		{"insert position", BuildBatchInsertDimensionArgs, map[string]any{"sheet-id": "s", "dimension": "ROWS"}, "position 必填"},
		{"move range unknown", BuildBatchMoveRangeArgs, map[string]any{"unknown": true}, "未知字段"},
		{"move range sheet", BuildBatchMoveRangeArgs, map[string]any{}, "sheet-id 必填"},
		{"move range source", BuildBatchMoveRangeArgs, map[string]any{"sheet-id": "s"}, "source-range 必填"},
		{"move range target", BuildBatchMoveRangeArgs, map[string]any{"sheet-id": "s", "source-range": "A1"}, "target-range 必填"},
		{"move range target sheet", BuildBatchMoveRangeArgs, map[string]any{"sheet-id": "s", "source-range": "A1", "target-range": "A2", "target-sheet-id": 1}, "必须是字符串"},
		{"sort unknown", BuildBatchSortRangeArgs, map[string]any{"unknown": true}, "未知字段"},
		{"sort sheet", BuildBatchSortRangeArgs, map[string]any{}, "sheet-id 必填"},
		{"sort range", BuildBatchSortRangeArgs, map[string]any{"sheet-id": "s"}, "range 必填"},
		{"sort item", BuildBatchSortRangeArgs, map[string]any{"sheet-id": "s", "range": "A1", "sort-keys": []any{1}}, "必须是 object"},
		{"sort bool", BuildBatchSortRangeArgs, map[string]any{"sheet-id": "s", "range": "A1", "sort-keys": []any{map[string]any{}}, "has-header": "false"}, "必须是布尔值"},
		{"sheet ID unknown", buildBatchSheetIDOnly, map[string]any{"unknown": true}, "未知字段"},
		{"update unknown", BuildBatchUpdateSheetArgs, map[string]any{"unknown": true}, "未知字段"},
		{"update name", BuildBatchUpdateSheetArgs, map[string]any{"sheet-id": "s", "name": 1}, "必须是字符串"},
		{"update title", BuildBatchUpdateSheetArgs, map[string]any{"sheet-id": "s", "title": 1}, "必须是字符串"},
		{"update aliases", BuildBatchUpdateSheetArgs, map[string]any{"sheet-id": "s", "name": "a", "title": "b"}, "不能同时指定"},
		{"update index", BuildBatchUpdateSheetArgs, map[string]any{"sheet-id": "s", "index": "1"}, "必须是整数"},
		{"update frozen", BuildBatchUpdateSheetArgs, map[string]any{"sheet-id": "s", "frozen-column-count": "1"}, "必须是整数"},
		{"update tab color", BuildBatchUpdateSheetArgs, map[string]any{"sheet-id": "s", "tab-color": 1}, "必须是字符串"},
		{"update empty", BuildBatchUpdateSheetArgs, map[string]any{"sheet-id": "s"}, "至少指定一个"},
		{"gridline", buildBatchGridlineArgs("visible"), map[string]any{"unknown": true}, "未知字段"},
		{"object delete", func(input map[string]any) (map[string]any, error) {
			return buildBatchObjectDeleteArgs(input, "object-id", "objectId")
		}, map[string]any{"unknown": true}, "未知字段"},
		{"condition unknown", BuildBatchCreateCondFormatArgs, map[string]any{"unknown": true}, "未知字段"},
		{"condition sheet", BuildBatchCreateCondFormatArgs, map[string]any{}, "sheet-id 必填"},
		{"condition rule", BuildBatchUpdateCondFormatArgs, map[string]any{"sheet-id": "s", "ranges": []any{"A1"}}, "rule-id 必填"},
		{"condition ranges required", BuildBatchCreateCondFormatArgs, map[string]any{"sheet-id": "s"}, "ranges 必填"},
		{"condition range", BuildBatchCreateCondFormatArgs, map[string]any{"sheet-id": "s", "ranges": []any{""}, "condition": map[string]any{"numberCondition": map[string]any{}}}, "非空字符串"},
		{"condition required", BuildBatchCreateCondFormatArgs, map[string]any{"sheet-id": "s", "ranges": []any{"A1"}}, "condition 必填"},
		{"condition count", BuildBatchCreateCondFormatArgs, map[string]any{"sheet-id": "s", "ranges": []any{"A1"}, "condition": map[string]any{"numberCondition": map[string]any{}, "textCondition": map[string]any{}}}, "只能包含一种"},
		{"condition type", BuildBatchCreateCondFormatArgs, map[string]any{"sheet-id": "s", "ranges": []any{"A1"}, "condition": map[string]any{"unknownCondition": map[string]any{}}}, "不支持"},
		{"condition value", BuildBatchCreateCondFormatArgs, map[string]any{"sheet-id": "s", "ranges": []any{"A1"}, "condition": map[string]any{"numberCondition": "bad"}}, "必须是 object"},
		{"condition style", BuildBatchUpdateCondFormatArgs, map[string]any{"sheet-id": "s", "rule-id": "r", "cell-style": func() {}}, "JSON 编码失败"},
		{"condition update empty", BuildBatchUpdateCondFormatArgs, map[string]any{"sheet-id": "s", "rule-id": "r"}, "至少指定一个"},
		{"filter create unknown", BuildBatchCreateFilterArgs, map[string]any{"unknown": true}, "未知字段"},
		{"filter create criteria", BuildBatchCreateFilterArgs, map[string]any{"sheet-id": "s", "range": "A1", "criteria": func() {}}, "JSON 编码失败"},
		{"filter update unknown", BuildBatchUpdateFilterArgs, map[string]any{"unknown": true}, "未知字段"},
		{"filter update sheet", BuildBatchUpdateFilterArgs, map[string]any{}, "sheet-id 必填"},
		{"filter update criteria", BuildBatchUpdateFilterArgs, map[string]any{"sheet-id": "s", "criteria": "["}, "JSON 数组"},
		{"filter update missing", BuildBatchUpdateFilterArgs, map[string]any{"sheet-id": "s"}, "criteria 必填"},
		{"filter view create unknown", BuildBatchCreateFilterViewArgs, map[string]any{"unknown": true}, "未知字段"},
		{"filter view create criteria", BuildBatchCreateFilterViewArgs, map[string]any{"sheet-id": "s", "name": "n", "range": "A1", "criteria": func() {}}, "JSON 编码失败"},
		{"filter view update unknown", BuildBatchUpdateFilterViewArgs, map[string]any{"unknown": true}, "未知字段"},
		{"filter view update name", BuildBatchUpdateFilterViewArgs, map[string]any{"sheet-id": "s", "filter-view-id": "v", "name": 1}, "必须是字符串"},
		{"filter view update criteria", BuildBatchUpdateFilterViewArgs, map[string]any{"sheet-id": "s", "filter-view-id": "v", "criteria": "["}, "JSON 数组"},
		{"filter view update empty", BuildBatchUpdateFilterViewArgs, map[string]any{"sheet-id": "s", "filter-view-id": "v"}, "至少指定一个"},
		{"float create unknown", BuildBatchCreateFloatImageArgs, map[string]any{"unknown": true}, "未知字段"},
		{"float create sheet", BuildBatchCreateFloatImageArgs, map[string]any{}, "sheet-id 必填"},
		{"float create src", BuildBatchCreateFloatImageArgs, map[string]any{"sheet-id": "s"}, "src 必填"},
		{"float create range", BuildBatchCreateFloatImageArgs, map[string]any{"sheet-id": "s", "src": "src"}, "range 必填"},
		{"float create width", BuildBatchCreateFloatImageArgs, map[string]any{"sheet-id": "s", "src": "src", "range": "A1"}, "width 必填"},
		{"float create height", BuildBatchCreateFloatImageArgs, map[string]any{"sheet-id": "s", "src": "src", "range": "A1", "width": 1}, "height 必填"},
		{"float create offset", BuildBatchCreateFloatImageArgs, map[string]any{"sheet-id": "s", "src": "src", "range": "A1", "width": 1, "height": 1, "offset-x": "0"}, "必须是整数"},
		{"float update file", BuildBatchUpdateFloatImageArgs, map[string]any{"file": "x"}, "media-upload"},
		{"float update unknown", BuildBatchUpdateFloatImageArgs, map[string]any{"unknown": true}, "未知字段"},
		{"float update src type", BuildBatchUpdateFloatImageArgs, map[string]any{"sheet-id": "s", "float-image-id": "i", "src": 1}, "必须是字符串"},
		{"float update src empty", BuildBatchUpdateFloatImageArgs, map[string]any{"sheet-id": "s", "float-image-id": "i", "src": " "}, "不能为空"},
		{"float update width", BuildBatchUpdateFloatImageArgs, map[string]any{"sheet-id": "s", "float-image-id": "i", "width": "1"}, "必须是整数"},
		{"float update empty", BuildBatchUpdateFloatImageArgs, map[string]any{"sheet-id": "s", "float-image-id": "i"}, "至少指定一个"},
	}

	for _, test := range tests {
		test := test
		requireSheetBatchError(t, test.name, test.want, func() error {
			_, err := test.build(test.input)
			return err
		})
	}

	if args, err := BuildBatchUpdateSheetArgs(map[string]any{"sheet-id": "s", "name": "renamed"}); err != nil || args["title"] != "renamed" {
		t.Fatalf("name alias = %#v, %v", args, err)
	}
}

func TestCrossPlatformCoverageSheetBatchCommandsPropagateEncodingErrors(t *testing.T) {
	boom := errors.New("encode failed")
	testseam.Swap(t, &buildBatchUpdateToolArgsForCommand, func(string, []any, bool) (map[string]any, error) {
		return nil, boom
	})

	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
	}{
		{"batch update", batchUpdateCoverageCommand, []string{"--node", "node", "--operations", `[{"toolName":"new","input":{"name":"sheet"}}]`}},
		{"batch clear", rangeBatchClearCoverageCommand, []string{"--node", "node", "--ranges", `["Sheet!A1"]`}},
		{"batch style", newRangeBatchSetStyleCmd, []string{"--node", "node", "--ranges", `["Sheet!A1"]`, "--font-weight", "bold"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := executeSheetBatchCommand(t, &scriptedToolCaller{}, test.cmd(), test.args...)
			if !errors.Is(err, boom) {
				t.Fatalf("error = %v, want %v", err, boom)
			}
		})
	}
}

func TestCrossPlatformCoverageStandaloneSheetBuildersPropagateValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"condition create", []string{"cond-format", "create", "--node", "node", "--sheet-id", " ", "--ranges", `["A1"]`, "--condition", `{"numberCondition":{}}`}},
		{"condition update", []string{"cond-format", "update", "--node", "node", "--sheet-id", " ", "--rule-id", "rule", "--ranges", `["A1"]`}},
		{"insert dimension", []string{"insert-dimension", "--node", "node", "--sheet-id", " ", "--dimension", "ROWS", "--position", "1", "--length", "1"}},
		{"filter update", []string{"filter", "update", "--node", "node", "--sheet-id", " ", "--criteria", `[]`}},
		{"float image create", []string{"create-float-image", "--node", "node", "--sheet-id", " ", "--src", "/resource", "--range", "A1", "--width", "1", "--height", "1"}},
		{"pivot delete", []string{"pivot-table", "delete", "--node", "node", "--sheet-id", " ", "--pivot-table-id", "pivot", "--yes"}},
		{"range sort", []string{"range", "sort", "--node", "node", "--sheet-id", " ", "--range", "A1", "--sort-keys", `[{}]`}},
		{"range move", []string{"range", "move-to", "--node", "node", "--sheet-id", " ", "--source-range", "A1", "--target-range", "A2", "--yes"}},
		{"show gridline", []string{"show-gridline", "--node", "node", "--sheet-id", " "}},
		{"hide gridline", []string{"hide-gridline", "--node", "node", "--sheet-id", " "}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newSheetCommand, test.args...)
			if err == nil || !strings.Contains(err.Error(), "不能为空") {
				t.Fatalf("error = %v, want shared builder validation", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("remote calls = %#v, want none", caller.calls)
			}
		})
	}
}
