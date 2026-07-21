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
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type sheetParityCaller struct {
	calls     []driveParityCall
	responses map[string][]string
	dryRun    bool
}

func (c *sheetParityCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, driveParityCall{server: server, tool: tool, args: args})
	text := `{}`
	if responses := c.responses[tool]; len(responses) > 0 {
		text = responses[0]
		c.responses[tool] = responses[1:]
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*sheetParityCaller) Format() string { return "json" }
func (c *sheetParityCaller) DryRun() bool { return c.dryRun }
func (*sheetParityCaller) Fields() string { return "" }
func (*sheetParityCaller) JQ() string     { return "" }

func executeSheetParityCommand(t *testing.T, caller *sheetParityCaller, stdin string, args ...string) (string, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	var output bytes.Buffer
	deps.Out.w = &output
	deps.Out.errW = &output

	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolP("yes", "y", false, "skip confirmation")
	root.PersistentFlags().Bool("dry-run", false, "preview")
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&output)
	root.SetErr(&output)
	root.AddCommand(newSheetCommand())
	root.SetArgs(append([]string{"sheet"}, args...))
	err := root.Execute()
	return output.String(), err
}

func TestSheetFormulaAndVersionCommandsRegistered(t *testing.T) {
	root := newSheetCommand()
	for _, path := range []string{"formula-verify", "version save", "version list", "version revert"} {
		command, remaining, err := root.Find(strings.Fields(path))
		if err != nil || len(remaining) != 0 || command == nil {
			t.Errorf("sheet %s not registered: command=%v remaining=%v err=%v", path, command, remaining, err)
		}
	}
}

func TestSheetFormulaVerifyMapsTargetsAndLimits(t *testing.T) {
	caller := &sheetParityCaller{responses: map[string][]string{"verify_formula": {`{"status":"success","totalErrors":0}`}}}
	_, err := executeSheetParityCommand(t, caller, "",
		"formula-verify", "--node", "sheet-doc-1", "--sheet-id", "Sheet1", "--range", "A1:D10",
		"--max-locations-per-error", "5", "--max-cells", "1000")
	if err != nil {
		t.Fatalf("formula-verify returned error: %v", err)
	}
	want := driveParityCall{server: "sheet", tool: "verify_formula", args: map[string]any{
		"nodeId":               "sheet-doc-1",
		"targets":              []map[string]any{{"sheetId": "Sheet1", "range": "A1:D10"}},
		"maxLocationsPerError": 5,
		"maxCells":             1000,
	}}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestSheetFormulaVerifyRejectsAmbiguousTargets(t *testing.T) {
	caller := &sheetParityCaller{}
	_, err := executeSheetParityCommand(t, caller, "",
		"formula-verify", "--node", "sheet-doc-1", "--sheet-id", "Sheet1", "--targets", `[{"sheetId":"Sheet2"}]`)
	if err == nil || !strings.Contains(err.Error(), "--targets") {
		t.Fatalf("error = %v, want target conflict", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote calls = %#v, want none", caller.calls)
	}
}

func TestSheetFormulaVerifyExitOnError(t *testing.T) {
	caller := &sheetParityCaller{responses: map[string][]string{"verify_formula": {`{"status":"errors_found","totalErrors":2}`}}}
	output, err := executeSheetParityCommand(t, caller, "", "formula-verify", "--node", "sheet-doc-1", "--exit-on-error")
	if err == nil || !strings.Contains(err.Error(), "formula errors found") {
		t.Fatalf("error = %v, want formula errors found", err)
	}
	if !strings.Contains(output, `"totalErrors": 2`) {
		t.Fatalf("output = %q", output)
	}
}

func TestSheetVersionSaveAndListReuseDocTools(t *testing.T) {
	caller := &sheetParityCaller{}
	if _, err := executeSheetParityCommand(t, caller, "", "version", "save", "--node", "sheet-doc-1"); err != nil {
		t.Fatalf("version save: %v", err)
	}
	if _, err := executeSheetParityCommand(t, caller, "", "version", "list", "--node", "sheet-doc-1", "--limit", "10", "--cursor", "next-1"); err != nil {
		t.Fatalf("version list: %v", err)
	}
	want := []driveParityCall{
		{server: "doc", tool: "save_doc_version", args: map[string]any{"nodeId": "sheet-doc-1"}},
		{server: "doc", tool: "list_doc_versions", args: map[string]any{"nodeId": "sheet-doc-1", "maxResults": 10, "nextCursor": "next-1"}},
	}
	if !reflect.DeepEqual(caller.calls, want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestSheetVersionRevertPreflightsVersionThenReverts(t *testing.T) {
	caller := &sheetParityCaller{responses: map[string][]string{
		"list_doc_versions":  {`{"result":{"versions":[{"version":3}],"hasMore":false}}`},
		"revert_doc_version": {`{"success":true}`},
	}}
	_, err := executeSheetParityCommand(t, caller, "", "version", "revert", "--node", "sheet-doc-1", "--version", "3", "--yes")
	if err != nil {
		t.Fatalf("version revert: %v", err)
	}
	wantTools := []string{"list_doc_versions", "revert_doc_version"}
	if len(caller.calls) != len(wantTools) {
		t.Fatalf("calls = %#v", caller.calls)
	}
	for index, tool := range wantTools {
		if caller.calls[index].server != "doc" || caller.calls[index].tool != tool {
			t.Fatalf("call[%d] = %#v, want doc/%s", index, caller.calls[index], tool)
		}
	}
}
