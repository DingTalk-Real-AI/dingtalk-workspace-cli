// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func runAitableStatsCLI(t *testing.T, args ...string) (*aitableTestCaller, error) {
	t.Helper()
	caller := &aitableTestCaller{}
	installAitableDeps(t, caller)
	command := newAitableCommand()
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetArgs(args)
	return caller, command.Execute()
}

func TestCrossPlatformCoverageAitableRecordsStatsForwardsValidatedContract(t *testing.T) {
	caller, err := runAitableStatsCLI(t,
		"record", "stats",
		"--base-id", "base-stats",
		"--table-id", "table-stats",
		"--stats", `[{"fieldId":"fldCount","statsType":"COUNT"},{"fieldId":"fldAmount","statsType":"SUM"}]`,
		"--filters", `{"operator":"and","operands":[{"operator":"gt","operands":["fldAmount",0]}]}`,
		"--sort", `[{"fieldId":"fldAmount","direction":"DESC"}]`,
		"--limit", "500",
		"--keyword", "华东",
		"--search-field-ids", "fldName,fldRegion",
		"--data-version", "42",
	)
	if err != nil {
		t.Fatalf("record stats returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.server != "aitable" || call.tool != "query_records_stats" {
		t.Fatalf("tool = %s/%s, want aitable/query_records_stats", call.server, call.tool)
	}
	wantStats := []map[string]any{
		{"fieldId": "fldCount", "statsType": "COUNT"},
		{"fieldId": "fldAmount", "statsType": "SUM"},
	}
	if !reflect.DeepEqual(call.args["stats"], wantStats) {
		t.Fatalf("stats = %#v, want %#v", call.args["stats"], wantStats)
	}
	filters := call.args["filters"].(map[string]any)
	condition := filters["operands"].([]any)[0].(map[string]any)
	if got := condition["operands"].([]any)[1]; got != json.Number("0") {
		t.Fatalf("numeric filter value = %#v (%T), want json.Number(0)", got, got)
	}
	if call.args["sort"] != `[{"fieldId":"fldAmount","direction":"DESC"}]` ||
		call.args["limit"] != 500 || call.args["keyword"] != "华东" || call.args["dataVersion"] != "42" {
		t.Fatalf("forwarded args = %#v", call.args)
	}
	if !reflect.DeepEqual(call.args["searchFieldIds"], []string{"fldName", "fldRegion"}) {
		t.Fatalf("searchFieldIds = %#v", call.args["searchFieldIds"])
	}
}

func TestCrossPlatformCoverageAitableGroupStatsForwardsStringDSL(t *testing.T) {
	group := `[{"fieldId":"fldRegion","direction":"ASC","fieldConfig":null,"arraySplitMode":true}]`
	sortDSL := `[{"fieldId":"fldAmount","direction":"DESC"}]`
	caller, err := runAitableStatsCLI(t,
		"record", "group-stats",
		"--base-id", "base-stats",
		"--table-id", "table-stats",
		"--stats", `[{"fieldId":"fldStore","statsType":"distinct"},{"fieldId":"fldAmount","statsType":"avg"}]`,
		"--filters", `{"operator":"OR","operands":[{"operator":"not_before","operands":["fldDate","2026-01-01"]}]}`,
		"--group", group,
		"--sort", sortDSL,
		"--limit", "1000",
		"--data-version", "43",
	)
	if err != nil {
		t.Fatalf("record group-stats returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.server != "aitable" || call.tool != "query_stats" {
		t.Fatalf("tool = %s/%s, want aitable/query_stats", call.server, call.tool)
	}
	if call.args["group"] != group || call.args["sortDsl"] != sortDSL || call.args["limit"] != 1000 || call.args["dataVersion"] != "43" {
		t.Fatalf("forwarded args = %#v", call.args)
	}
	stats := call.args["stats"].([]map[string]any)
	if len(stats) != 2 || stats[0]["statsType"] != "distinct" || stats[1]["statsType"] != "avg" {
		t.Fatalf("stats = %#v", stats)
	}
}

func TestCrossPlatformCoverageAitableStatsRejectsInvalidInputsBeforeCallingMCP(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "records stats require uppercase",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"count"}]`},
			wantErr: "大写",
		},
		{
			name:    "records stats require at least one item",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[]`},
			wantErr: "至少需要一个统计项",
		},
		{
			name: "records stats enforce item cap",
			args: []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[
				{"fieldId":"f01","statsType":"COUNT"},{"fieldId":"f02","statsType":"COUNT"},
				{"fieldId":"f03","statsType":"COUNT"},{"fieldId":"f04","statsType":"COUNT"},
				{"fieldId":"f05","statsType":"COUNT"},{"fieldId":"f06","statsType":"COUNT"},
				{"fieldId":"f07","statsType":"COUNT"},{"fieldId":"f08","statsType":"COUNT"},
				{"fieldId":"f09","statsType":"COUNT"},{"fieldId":"f10","statsType":"COUNT"},
				{"fieldId":"f11","statsType":"COUNT"},{"fieldId":"f12","statsType":"COUNT"},
				{"fieldId":"f13","statsType":"COUNT"},{"fieldId":"f14","statsType":"COUNT"},
				{"fieldId":"f15","statsType":"COUNT"},{"fieldId":"f16","statsType":"COUNT"},
				{"fieldId":"f17","statsType":"COUNT"},{"fieldId":"f18","statsType":"COUNT"},
				{"fieldId":"f19","statsType":"COUNT"},{"fieldId":"f20","statsType":"COUNT"},
				{"fieldId":"f21","statsType":"COUNT"}
			]`},
			wantErr: "单次最多支持 20 个统计项",
		},
		{
			name:    "records stats require item fields",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":" ","statsType":"COUNT"}]`},
			wantErr: "均不能为空",
		},
		{
			name:    "records stats reject unknown item fields",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT","unknownKey":123}]`},
			wantErr: "不支持的字段",
		},
		{
			name:    "records stats reject duplicate field",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"},{"fieldId":"f","statsType":"SUM"}]`},
			wantErr: "重复",
		},
		{
			name:    "group stats require lowercase",
			args:    []string{"record", "group-stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"}]`},
			wantErr: "小写",
		},
		{
			name:    "group must be array DSL",
			args:    []string{"record", "group-stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"count"}]`, "--group", `{}`},
			wantErr: "JSON 数组",
		},
		{
			name:    "group limit capped",
			args:    []string{"record", "group-stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"count"}]`, "--limit", "1001"},
			wantErr: "[1, 1000]",
		},
		{
			name:    "filters require logical root",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"}]`, "--filters", `{"operator":"eq","operands":[]}`},
			wantErr: `root "operator" must be "and" or "or"`,
		},
		{
			name:    "records filters reject malformed JSON",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"}]`, "--filters", `{`},
			wantErr: "必须是 JSON 对象",
		},
		{
			name:    "records filters reject null",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"}]`, "--filters", `null`},
			wantErr: "不能是 null",
		},
		{
			name:    "records filters reject a second JSON value",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"}]`, "--filters", `{} {}`},
			wantErr: "只能包含一个 JSON 对象",
		},
		{
			name:    "records filters reject invalid trailing content",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"}]`, "--filters", `{} trailing`},
			wantErr: "无效的尾随内容",
		},
		{
			name:    "records filters require operand array",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"}]`, "--filters", `{"operator":"and","operands":{}}`},
			wantErr: `"operands" must be an array`,
		},
		{
			name:    "records filters reject silently unsupported operators",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"}]`, "--filters", `{"operator":"and","operands":[{"operator":"date_between","operands":["fldDate",[1,2]]}]}`},
			wantErr: `unsupported filter operator "date_between"`,
		},
		{
			name:    "records sort requires an item",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"}]`, "--sort", `[]`},
			wantErr: "至少需要一个条目",
		},
		{
			name:    "records sort items must be objects",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"}]`, "--sort", `[1]`},
			wantErr: "必须是 JSON 对象",
		},
		{
			name:    "records limit must be positive",
			args:    []string{"record", "stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"COUNT"}]`, "--limit", "0"},
			wantErr: "必须大于 0",
		},
		{
			name:    "group filters reject malformed JSON",
			args:    []string{"record", "group-stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"count"}]`, "--filters", `{`},
			wantErr: "必须是 JSON 对象",
		},
		{
			name:    "group filters require logical root",
			args:    []string{"record", "group-stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"count"}]`, "--filters", `{"operator":"eq","operands":[]}`},
			wantErr: `root "operator" must be "and" or "or"`,
		},
		{
			name:    "group filters reject silently unsupported operators",
			args:    []string{"record", "group-stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"count"}]`, "--filters", `{"operator":"and","operands":[{"operator":"date_between","operands":["fldDate",[1,2]]}]}`},
			wantErr: `unsupported filter operator "date_between"`,
		},
		{
			name:    "group sort rejects invalid DSL",
			args:    []string{"record", "group-stats", "--base-id", "b", "--table-id", "t", "--stats", `[{"fieldId":"f","statsType":"count"}]`, "--sort", `{}`},
			wantErr: "JSON 数组",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller, err := runAitableStatsCLI(t, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
			var structured *apperrors.Error
			if !errors.As(err, &structured) || structured.Category != apperrors.CategoryValidation {
				t.Fatalf("error = %#v, want structured validation error", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls = %d, want 0", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageAitableStatsPublishInterfaceAndResultContracts(t *testing.T) {
	root := newAitableCommand()
	tests := []struct {
		path string
		rpc  string
	}{
		{path: "aitable record stats", rpc: "query_records_stats"},
		{path: "aitable record group-stats", rpc: "query_stats"},
	}
	for _, test := range tests {
		leaf := findCLIPath(root, test.path)
		if leaf == nil {
			t.Fatalf("missing leaf %q", test.path)
		}
		final, ok := contractfinal.RuntimeContractFinal(leaf)
		if !ok || final.Interface == nil || final.Interface.Ref == nil || final.Interface.Ref.RPCName != test.rpc {
			t.Fatalf("%s interface = %#v", test.path, final.Interface)
		}
		if final.Result == nil || len(final.Result.Outcomes) == 0 || len(final.Result.DataSchema) == 0 {
			t.Fatalf("%s result = %#v", test.path, final.Result)
		}
	}
}
