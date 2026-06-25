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
	"bytes"
	"context"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

type sheetCommandRunner struct {
	last  executor.Invocation
	calls []executor.Invocation
}

func (r *sheetCommandRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	r.calls = append(r.calls, invocation)
	return executor.Result{Invocation: invocation}, nil
}

func executeSheetCommand(t *testing.T, runner *sheetCommandRunner, args ...string) {
	t.Helper()
	cmd := sheetHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
}

func TestSheetCreateCallsCreateWorkspaceSheet(t *testing.T) {
	t.Parallel()

	runner := &sheetCommandRunner{}
	executeSheetCommand(t, runner, "create", "--name", "销售数据", "--folder", "FOLDER_001", "--workspace", "WS_001")

	if runner.last.Tool != "create_workspace_sheet" {
		t.Fatalf("tool = %q, want create_workspace_sheet", runner.last.Tool)
	}
	if got := runner.last.Params["name"]; got != "销售数据" {
		t.Fatalf("name = %#v", got)
	}
	if got := runner.last.Params["folderId"]; got != "FOLDER_001" {
		t.Fatalf("folderId = %#v", got)
	}
	if got := runner.last.Params["workspaceId"]; got != "WS_001" {
		t.Fatalf("workspaceId = %#v", got)
	}
}

func TestSheetRangeUpdateCallsSetCellRange(t *testing.T) {
	t.Parallel()

	runner := &sheetCommandRunner{}
	executeSheetCommand(t, runner,
		"range", "update",
		"--node", "NODE_001",
		"--sheet-id", "SHEET_001",
		"--range", "A1:B1",
		"--values", `[[{"type":"text","text":"姓名"},{"type":"text","text":"分数"}]]`,
	)

	if runner.last.Tool != "set_cell_range" {
		t.Fatalf("tool = %q, want set_cell_range", runner.last.Tool)
	}
	if got := runner.last.Params["rangeAddress"]; got != "A1:B1" {
		t.Fatalf("rangeAddress = %#v", got)
	}
	cells, ok := runner.last.Params["cells"].([][]any)
	if !ok || len(cells) != 1 || len(cells[0]) != 2 {
		t.Fatalf("cells = %#v", runner.last.Params["cells"])
	}
}

func TestSheetRangeSetStyleExpandsScalarStyle(t *testing.T) {
	t.Parallel()

	runner := &sheetCommandRunner{}
	executeSheetCommand(t, runner,
		"range", "set-style",
		"--node", "NODE_001",
		"--sheet-id", "SHEET_001",
		"--range", "A1:B2",
		"--bg-color", "#FFF2CC",
		"--font-weight", "bold",
	)

	if runner.last.Tool != "update_range" {
		t.Fatalf("tool = %q, want update_range", runner.last.Tool)
	}
	bg, ok := runner.last.Params["backgroundColors"].([][]string)
	if !ok || len(bg) != 2 || len(bg[0]) != 2 || bg[1][1] != "#FFF2CC" {
		t.Fatalf("backgroundColors = %#v", runner.last.Params["backgroundColors"])
	}
	weights, ok := runner.last.Params["fontWeights"].([][]string)
	if !ok || weights[0][0] != "bold" {
		t.Fatalf("fontWeights = %#v", runner.last.Params["fontWeights"])
	}
}

func TestSheetFilterViewUpdateCriteriaCallsSetCriteria(t *testing.T) {
	t.Parallel()

	runner := &sheetCommandRunner{}
	executeSheetCommand(t, runner,
		"filter-view", "update-criteria",
		"--node", "NODE_001",
		"--sheet-id", "SHEET_001",
		"--filter-view-id", "FV_001",
		"--column", "2",
		"--filter-criteria", `{"filterType":"values","visibleValues":["销售部"]}`,
	)

	if runner.last.Tool != "set_filter_view_criteria" {
		t.Fatalf("tool = %q, want set_filter_view_criteria", runner.last.Tool)
	}
	if got := runner.last.Params["filterViewId"]; got != "FV_001" {
		t.Fatalf("filterViewId = %#v", got)
	}
	if got := runner.last.Params["column"]; got != 2 {
		t.Fatalf("column = %#v", got)
	}
}

func TestSheetCondFormatCreateExpandsCondition(t *testing.T) {
	t.Parallel()

	runner := &sheetCommandRunner{}
	executeSheetCommand(t, runner,
		"cond-format", "create",
		"--node", "NODE_001",
		"--sheet-id", "SHEET_001",
		"--ranges", `["A1:A10"]`,
		"--condition", `{"numberCondition":{"operator":"greater","value1":"80"}}`,
		"--cell-style", `{"backgroundColor":"#FFCDD2"}`,
	)

	if runner.last.Tool != "create_cond_format" {
		t.Fatalf("tool = %q, want create_cond_format", runner.last.Tool)
	}
	if _, ok := runner.last.Params["numberCondition"]; !ok {
		t.Fatalf("numberCondition missing from %#v", runner.last.Params)
	}
	if _, ok := runner.last.Params["cellStyle"]; !ok {
		t.Fatalf("cellStyle missing from %#v", runner.last.Params)
	}
}

func TestSheetCSVCommandsAcceptFileIDAlias(t *testing.T) {
	t.Parallel()

	runner := &sheetCommandRunner{}
	executeSheetCommand(t, runner,
		"csv-get",
		"--file-id", "NODE_001",
		"--sheet-id", "SHEET_001",
		"--range", "A1:B2",
	)

	if runner.last.Tool != "get_range_as_csv" {
		t.Fatalf("tool = %q, want get_range_as_csv", runner.last.Tool)
	}
	if got := runner.last.Params["nodeId"]; got != "NODE_001" {
		t.Fatalf("csv-get nodeId = %#v", got)
	}

	executeSheetCommand(t, runner,
		"csv-put",
		"--file-id", "NODE_002",
		"--sheet-id", "SHEET_002",
		"--csv", "a,b\n1,2",
		"--start-cell", "A1",
	)

	if runner.last.Tool != "set_range_from_csv" {
		t.Fatalf("tool = %q, want set_range_from_csv", runner.last.Tool)
	}
	if got := runner.last.Params["nodeId"]; got != "NODE_002" {
		t.Fatalf("csv-put nodeId = %#v", got)
	}
}

func TestSheetReplaceAllowsEmptyReplacement(t *testing.T) {
	t.Parallel()

	runner := &sheetCommandRunner{}
	executeSheetCommand(t, runner,
		"replace",
		"--node", "NODE_001",
		"--sheet-id", "SHEET_001",
		"--find", "临时",
		"--replacement", "",
	)

	if runner.last.Tool != "replace_all" {
		t.Fatalf("tool = %q, want replace_all", runner.last.Tool)
	}
	if got := runner.last.Params["replaceText"]; got != "" {
		t.Fatalf("replaceText = %#v, want empty string", got)
	}
}
