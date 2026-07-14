// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type sheetPivotGridlineCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type sheetPivotGridlineCaller struct {
	calls []sheetPivotGridlineCall
}

func (c *sheetPivotGridlineCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, sheetPivotGridlineCall{productID: productID, toolName: toolName, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*sheetPivotGridlineCaller) Format() string { return "json" }
func (*sheetPivotGridlineCaller) DryRun() bool   { return false }
func (*sheetPivotGridlineCaller) Fields() string { return "" }
func (*sheetPivotGridlineCaller) JQ() string     { return "" }

func executeSheetPivotGridlineCommand(t *testing.T, caller *sheetPivotGridlineCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	InitDeps(caller)
	deps.Out.w = io.Discard
	os.Args = append([]string{"dws", "sheet"}, args...)
	root := newSheetCommand()
	root.PersistentFlags().Bool("yes", false, "skip confirmation")
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	return root.Execute()
}

func TestSheetPivotAndGridlineCommandsRegistered(t *testing.T) {
	root := newSheetCommand()
	cases := []struct {
		path  []string
		flags []string
	}{
		{[]string{"pivot-table", "list"}, []string{"node", "sheet-id", "pivot-table-id"}},
		{[]string{"pivot-table", "create"}, []string{"node", "source", "properties", "target-sheet-id", "target-position"}},
		{[]string{"pivot-table", "update"}, []string{"node", "sheet-id", "pivot-table-id", "properties"}},
		{[]string{"pivot-table", "delete"}, []string{"node", "sheet-id", "pivot-table-id"}},
		{[]string{"show-gridline"}, []string{"node", "sheet-id"}},
		{[]string{"hide-gridline"}, []string{"node", "sheet-id"}},
	}

	for _, tc := range cases {
		cmd, remaining, err := root.Find(tc.path)
		if err != nil || len(remaining) != 0 {
			t.Fatalf("dws sheet %s not registered: cmd=%v remaining=%v err=%v", strings.Join(tc.path, " "), cmd, remaining, err)
		}
		for _, flag := range tc.flags {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("dws sheet %s: missing flag --%s", strings.Join(tc.path, " "), flag)
			}
		}
	}
}

func TestSheetPivotListBuildsToolArgs(t *testing.T) {
	caller := &sheetPivotGridlineCaller{}
	err := executeSheetPivotGridlineCommand(t, caller,
		"pivot-table", "list", "--node", "node1", "--sheet-id", "sheet1", "--pivot-table-id", "pivot1")
	if err != nil {
		t.Fatalf("pivot-table list returned error: %v", err)
	}
	want := sheetPivotGridlineCall{
		productID: "sheet",
		toolName:  "list_pivot_tables",
		args: map[string]any{
			"nodeId":       "node1",
			"sheetId":      "sheet1",
			"pivotTableId": "pivot1",
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestSheetPivotCreateBuildsToolArgs(t *testing.T) {
	caller := &sheetPivotGridlineCaller{}
	err := executeSheetPivotGridlineCommand(t, caller,
		"pivot-table", "create",
		"--node", "node1",
		"--source", "'Data'!A1:D20",
		"--properties", `{"rows":[{"field":"team"}],"values":[{"field":"amount","summarize_by":" SUM "}]}`,
		"--target-sheet-id", "summary",
		"--target-position", "B2")
	if err != nil {
		t.Fatalf("pivot-table create returned error: %v", err)
	}
	want := sheetPivotGridlineCall{
		productID: "sheet",
		toolName:  "create_pivot_table",
		args: map[string]any{
			"nodeId":         "node1",
			"source":         "'Data'!A1:D20",
			"targetSheetId":  "summary",
			"targetPosition": "B2",
			"properties": map[string]any{
				"rows":   []any{map[string]any{"field": "team"}},
				"values": []any{map[string]any{"field": "amount", "summarize_by": "sum"}},
			},
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestSheetPivotCreateReadsPropertiesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pivot.json")
	if err := os.WriteFile(path, []byte(`{"values":[{"field":"amount","summarize_by":"average"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := &sheetPivotGridlineCaller{}
	if err := executeSheetPivotGridlineCommand(t, caller,
		"pivot-table", "create", "--node", "node1", "--source", "'Data'!A1:B10", "--properties", "@"+path); err != nil {
		t.Fatalf("pivot-table create with @file returned error: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != "create_pivot_table" {
		t.Fatalf("calls = %#v, want create_pivot_table", caller.calls)
	}
}

func TestSheetPivotUpdateAllowsPartialProperties(t *testing.T) {
	caller := &sheetPivotGridlineCaller{}
	err := executeSheetPivotGridlineCommand(t, caller,
		"pivot-table", "update", "--node", "node1", "--sheet-id", "sheet1", "--pivot-table-id", "pivot1",
		"--properties", `{"show_subtotals":false}`)
	if err != nil {
		t.Fatalf("pivot-table update returned error: %v", err)
	}
	wantArgs := map[string]any{
		"nodeId":       "node1",
		"sheetId":      "sheet1",
		"pivotTableId": "pivot1",
		"properties":   map[string]any{"show_subtotals": false},
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != "update_pivot_table" || !reflect.DeepEqual(caller.calls[0].args, wantArgs) {
		t.Fatalf("calls = %#v, want update_pivot_table %#v", caller.calls, wantArgs)
	}
}

func TestSheetPivotDeleteBuildsToolArgsAfterConfirmation(t *testing.T) {
	caller := &sheetPivotGridlineCaller{}
	if err := executeSheetPivotGridlineCommand(t, caller,
		"pivot-table", "delete", "--node", "node1", "--sheet-id", "sheet1", "--pivot-table-id", "pivot1", "--yes"); err != nil {
		t.Fatalf("pivot-table delete returned error: %v", err)
	}
	wantArgs := map[string]any{"nodeId": "node1", "sheetId": "sheet1", "pivotTableId": "pivot1"}
	if len(caller.calls) != 1 || caller.calls[0].toolName != "delete_pivot_table" || !reflect.DeepEqual(caller.calls[0].args, wantArgs) {
		t.Fatalf("calls = %#v, want delete_pivot_table %#v", caller.calls, wantArgs)
	}
}

func TestSheetPivotRejectsInvalidPropertiesBeforeCall(t *testing.T) {
	tests := []struct {
		name    string
		command string
		props   string
		wantErr string
	}{
		{"create invalid JSON", "create", `{`, "JSON 解析失败"},
		{"create missing values", "create", `{"rows":[{"field":"team"}]}`, "缺少必填字段 values"},
		{"create empty values", "create", `{"values":[]}`, "至少包含一项"},
		{"create invalid summarize", "create", `{"values":[{"field":"amount","summarize_by":"total"}]}`, "summarize_by 不支持"},
		{"create invalid row", "create", `{"rows":[{}],"values":[{"field":"amount"}]}`, "rows[0].field"},
		{"create invalid collapse", "create", `{"values":[{"field":"amount"}],"collapse":"team"}`, "collapse 必须"},
		{"update empty properties", "update", `{}`, "不能为空对象"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &sheetPivotGridlineCaller{}
			args := []string{"pivot-table", tt.command, "--node", "node1", "--properties", tt.props}
			if tt.command == "create" {
				args = append(args, "--source", "'Data'!A1:B10")
			} else {
				args = append(args, "--sheet-id", "sheet1", "--pivot-table-id", "pivot1")
			}
			err := executeSheetPivotGridlineCommand(t, caller, args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("remote calls = %d, want 0", len(caller.calls))
			}
		})
	}
}

func TestSheetPivotAndGridlineRequireMandatoryFlags(t *testing.T) {
	for _, args := range [][]string{
		{"pivot-table", "list", "--node", "node1"},
		{"pivot-table", "create", "--node", "node1", "--source", "'Data'!A1:B10"},
		{"pivot-table", "update", "--node", "node1", "--sheet-id", "sheet1", "--pivot-table-id", "pivot1"},
		{"pivot-table", "delete", "--node", "node1", "--sheet-id", "sheet1"},
		{"show-gridline", "--node", "node1"},
		{"hide-gridline", "--sheet-id", "sheet1"},
	} {
		caller := &sheetPivotGridlineCaller{}
		if err := executeSheetPivotGridlineCommand(t, caller, args...); err == nil {
			t.Fatalf("dws sheet %s returned nil error", strings.Join(args, " "))
		}
		if len(caller.calls) != 0 {
			t.Fatalf("dws sheet %s made %d remote calls, want 0", strings.Join(args, " "), len(caller.calls))
		}
	}
}

func TestSheetGridlineCommandsBuildToolArgs(t *testing.T) {
	for _, tt := range []struct {
		command    string
		nodeFlag   string
		visibility string
	}{
		{"show-gridline", "node-id", "visible"},
		{"hide-gridline", "file-id", "hidden"},
	} {
		t.Run(tt.command, func(t *testing.T) {
			caller := &sheetPivotGridlineCaller{}
			if err := executeSheetPivotGridlineCommand(t, caller, tt.command, "--"+tt.nodeFlag, "node1", "--sheet-id", "sheet1"); err != nil {
				t.Fatalf("%s returned error: %v", tt.command, err)
			}
			want := sheetPivotGridlineCall{
				productID: "sheet",
				toolName:  "set_gridline_visibility",
				args: map[string]any{
					"nodeId":     "node1",
					"sheetId":    "sheet1",
					"visibility": tt.visibility,
				},
			}
			if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
				t.Fatalf("calls = %#v, want %#v", caller.calls, want)
			}
		})
	}
}
