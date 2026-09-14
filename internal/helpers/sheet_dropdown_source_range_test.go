package helpers

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
)

func TestCrossPlatformCoverageSetDropdownSourceRangeCommand(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
	installScriptedCaller(t, caller)
	err := executeDimensionCoverage(t, "set-dropdown",
		"--node", "node-1",
		"--sheet-id", "target-sheet",
		"--range", "B2:B100",
		"--source-sheet-id", "source-sheet",
		"--source-range", "$t$1:$t$3",
		"--multi-select",
	)
	if err != nil {
		t.Fatalf("set SourceRange dropdown: %v", err)
	}
	if caller.tool != "set_dropdown_lists" {
		t.Fatalf("tool = %q, want set_dropdown_lists", caller.tool)
	}
	if _, exists := caller.args["options"]; exists {
		t.Fatalf("SourceRange request must omit options: %#v", caller.args)
	}
	source, ok := caller.args["sourceRange"].(map[string]any)
	if !ok {
		t.Fatalf("sourceRange = %#v", caller.args["sourceRange"])
	}
	if source["sheetId"] != "source-sheet" || source["a1Notation"] != "$t$1:$t$3" {
		t.Fatalf("sourceRange = %#v", source)
	}
	if caller.args["enableMultiSelect"] != true {
		t.Fatalf("enableMultiSelect = %#v", caller.args["enableMultiSelect"])
	}
}

func TestCrossPlatformCoverageSetDropdownInlineCommandStillOmitsSourceRange(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
	installScriptedCaller(t, caller)
	err := executeDimensionCoverage(t, "set-dropdown",
		"--node", "node-1",
		"--sheet-id", "sheet-1",
		"--range", "A1:A3",
		"--options", `[{"value":"one","color":"#ff0000"}]`,
	)
	if err != nil {
		t.Fatalf("set inline dropdown: %v", err)
	}
	if _, exists := caller.args["sourceRange"]; exists {
		t.Fatalf("inline request must omit sourceRange: %#v", caller.args)
	}
	options, ok := caller.args["options"].([]map[string]any)
	if !ok || len(options) != 1 || options[0]["value"] != "one" {
		t.Fatalf("options = %#v", caller.args["options"])
	}
}

func TestCrossPlatformCoverageSetDropdownModeConstraints(t *testing.T) {
	base := []string{"--node", "node", "--sheet-id", "sheet", "--range", "A1"}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "neither", want: "one of the flags"},
		{name: "both", args: []string{"--options", `[{"value":"one"}]`, "--source-sheet-id", "source", "--source-range", "A1:A3"}, want: "none of the others"},
		{name: "source sheet without range", args: []string{"--options", `[{"value":"one"}]`, "--source-sheet-id", "source"}, want: "must all be set"},
		{name: "range without source sheet", args: []string{"--source-range", "A1:A3"}, want: "must all be set"},
		{name: "sheet prefix", args: []string{"--source-sheet-id", "source", "--source-range", "Sheet2!A1:A3"}, want: "不能包含工作表前缀"},
		{name: "formula", args: []string{"--source-sheet-id", "source", "--source-range", "=A1:A3"}, want: "必须是单一连续区域"},
		{name: "multi region", args: []string{"--source-sheet-id", "source", "--source-range", "A1:A3,C1:C3"}, want: "必须是单一连续区域"},
		{name: "blank source range", args: []string{"--source-sheet-id", "source", "--source-range", " \t "}, want: "--source-range 不能为空"},
		{name: "blank source sheet", args: []string{"--source-sheet-id", " \t ", "--source-range", "A1:A3"}, want: "--source-sheet-id 不能为空"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			installScriptedCaller(t, caller)
			args := append(append([]string{}, base...), tc.args...)
			err := executeDimensionCoverage(t, "set-dropdown", args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want contains %q", err, tc.want)
			}
			if caller.calls != 0 {
				t.Fatalf("calls = %d, validation must fail before request", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageSetDropdownRunEValidation(t *testing.T) {
	tests := []struct {
		name    string
		options string
		source  string
		rangeID string
		want    string
	}{
		{name: "neither mode", want: "必须且只能指定一个"},
		{name: "source range without sheet", rangeID: "A1:A3", want: "必须同时指定 --source-sheet-id"},
		{name: "source sheet without range", options: `[{"value":"one"}]`, source: "source", want: "必须同时指定 --source-range"},
		{name: "invalid options JSON", options: "{", want: "JSON 解析失败"},
		{name: "missing option value", options: `[{}]`, want: "value 为空"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := dimensionCoverageCommand(t, "set-dropdown")
			for name, value := range map[string]string{
				"node": "node", "sheet-id": "sheet", "range": "A1",
				"options": tc.options, "source-sheet-id": tc.source, "source-range": tc.rangeID,
			} {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("set --%s: %v", name, err)
				}
			}
			err := cmd.RunE(cmd, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSetDropdownPreservesSchemaConstraints(t *testing.T) {
	cmd := dimensionCoverageCommand(t, "set-dropdown")
	if constraints := runtimeannotate.CommandConstraints(cmd); !runtimeannotate.ConstraintsEmpty(constraints) {
		t.Fatalf("constraints = %#v, want no new schema-level constraints", constraints)
	}
}

func TestCrossPlatformCoverageBatchSetDropdownSourceRange(t *testing.T) {
	got, err := translateBatchOp(map[string]any{
		"toolName": "set-dropdown",
		"input": map[string]any{
			"sheet-id":        "target-sheet",
			"range":           "B2:B100",
			"source-sheet-id": "source-sheet",
			"source-range":    "T:T",
			"multi-select":    true,
		},
	})
	if err != nil {
		t.Fatalf("translate SourceRange batch op: %v", err)
	}
	input := got["input"].(map[string]any)
	if _, exists := input["options"]; exists {
		t.Fatalf("SourceRange batch input must omit options: %#v", input)
	}
	source := input["sourceRange"].(map[string]any)
	if source["sheetId"] != "source-sheet" || source["a1Notation"] != "T:T" {
		t.Fatalf("sourceRange = %#v", source)
	}

	invalid := []map[string]any{
		{},
		{"options": []any{map[string]any{"value": "one"}}, "source-sheet-id": "source", "source-range": "A1:A3"},
		{"source-range": "A1:A3"},
		{"source-sheet-id": "source", "source-range": "A1:A3", "colors": []any{"#fff"}},
		{"source-sheet-id": "source", "source-range": "A1:A3", "source-colors": []any{"#fff"}},
		{"source-sheet-id": "source", "source-range": "Sheet2!A1:A3"},
	}
	for _, value := range invalid {
		if _, err := translateBatchOp(map[string]any{"toolName": "set-dropdown", "input": value}); err == nil {
			t.Errorf("invalid batch SourceRange input %#v returned nil", value)
		}
	}
}

func TestCrossPlatformCoverageBatchSetDropdownValidationGuidance(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{
			name:  "inline top-level colors",
			input: map[string]any{"options": []any{map[string]any{"value": "one"}}, "colors": []any{"#fff"}},
			want:  "Inline 颜色请写入 options[].color",
		},
		{
			name:  "source top-level colors",
			input: map[string]any{"source-sheet-id": "source", "source-range": "A1:A3", "source-colors": []any{"#fff"}},
			want:  "SourceRange 颜色写入暂不支持",
		},
		{
			name:  "blank source range",
			input: map[string]any{"source-sheet-id": "source", "source-range": " \t "},
			want:  "--source-range 不能为空",
		},
		{
			name:  "blank source sheet",
			input: map[string]any{"source-sheet-id": " \t ", "source-range": "A1:A3"},
			want:  "--source-sheet-id 不能为空",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := translateBatchOp(map[string]any{"toolName": "set-dropdown", "input": tc.input})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageDropdownHelpDescribesDynamicAndInvalidContracts(t *testing.T) {
	setDropdown := dimensionCoverageCommand(t, "set-dropdown")
	if !strings.Contains(setDropdown.Long, "会自动调整引用并保持 valid") ||
		!strings.Contains(setDropdown.Long, "只有 invalid 时才重新选择来源并写入") {
		t.Fatalf("set-dropdown help does not describe verified structural behavior:\n%s", setDropdown.Long)
	}
	getDropdown := dimensionCoverageCommand(t, "get-dropdown")
	if !strings.Contains(getDropdown.Long, `仅在 sourceRangeStatus="valid" 时返回`) ||
		!strings.Contains(getDropdown.Long, "invalid 结果仍保留配置组，但省略 sourceRange") {
		t.Fatalf("get-dropdown help does not describe conditional sourceRange readback:\n%s", getDropdown.Long)
	}
}

func TestCrossPlatformCoverageValidateSourceRangeDataValidation(t *testing.T) {
	valid := []any{
		map[string]any{"type": "dropdown", "sourceRange": map[string]any{"sheetId": "source", "a1Notation": "T1:T3"}},
		map[string]any{"type": "dropdown", "sourceRange": map[string]any{"sheetId": "missing-sheet", "a1Notation": "not-validated-locally"}, "enableMultiSelect": true},
	}
	for _, value := range valid {
		if err := validateDataValidation(value, "dv"); err != nil {
			t.Errorf("valid SourceRange data validation %#v: %v", value, err)
		}
	}

	invalid := []any{
		map[string]any{"type": "dropdown", "options": []any{map[string]any{"value": "one"}}, "sourceRange": map[string]any{"sheetId": "source", "a1Notation": "T1:T3"}},
		map[string]any{"type": "dropdown", "sourceRange": "T1:T3"},
		map[string]any{"type": "dropdown", "sourceRange": map[string]any{"a1Notation": "T1:T3"}},
		map[string]any{"type": "dropdown", "sourceRange": map[string]any{"sheetId": "source"}},
		map[string]any{"type": "dropdown", "sourceRange": map[string]any{"sheetId": "source", "a1Notation": "T1:T3", "colors": []any{"#fff"}}},
		map[string]any{"type": "dropdown", "sourceRange": map[string]any{"sheetId": "source", "a1Notation": "T1:T3"}, "enableMultiSelect": "yes"},
	}
	for _, value := range invalid {
		if err := validateDataValidation(value, "dv"); err == nil {
			t.Errorf("invalid SourceRange data validation %#v returned nil", value)
		}
	}
}
