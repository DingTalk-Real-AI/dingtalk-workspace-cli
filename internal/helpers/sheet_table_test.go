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

type sheetTableCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type sheetTableCaller struct {
	calls  []sheetTableCall
	dryRun bool
}

func (c *sheetTableCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, sheetTableCall{productID: productID, toolName: toolName, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*sheetTableCaller) Format() string { return "json" }
func (c *sheetTableCaller) DryRun() bool { return c.dryRun }
func (*sheetTableCaller) Fields() string { return "" }
func (*sheetTableCaller) JQ() string     { return "" }

func executeSheetTableCommand(t *testing.T, caller *sheetTableCaller, stdin io.Reader, args ...string) error {
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
	root.SilenceErrors = true
	root.SilenceUsage = true
	if stdin != nil {
		root.SetIn(stdin)
	}
	root.SetArgs(args)
	return root.Execute()
}

func TestSheetTableCommandsRegistered(t *testing.T) {
	root := newSheetCommand()
	for _, path := range []string{"table-get", "table-put", "table-read", "table-write"} {
		cmd, remaining, err := root.Find([]string{path})
		if err != nil || len(remaining) != 0 {
			t.Fatalf("dws sheet %s not registered: cmd=%v remaining=%v err=%v", path, cmd, remaining, err)
		}
	}
	if cmd, remaining, _ := root.Find([]string{"table"}); cmd != nil && len(remaining) == 0 {
		t.Fatal("dws sheet table parent should not be registered")
	}

	getCmd, _, _ := root.Find([]string{"table-get"})
	for _, flag := range []string{"node", "sheet-id", "range", "no-header"} {
		if getCmd.Flags().Lookup(flag) == nil {
			t.Errorf("table-get: missing flag --%s", flag)
		}
	}
	putCmd, _, _ := root.Find([]string{"table-put"})
	for _, flag := range []string{"node", "sheets"} {
		if putCmd.Flags().Lookup(flag) == nil {
			t.Errorf("table-put: missing flag --%s", flag)
		}
	}
}

func TestSheetTableGetBuildsToolArgs(t *testing.T) {
	caller := &sheetTableCaller{}
	err := executeSheetTableCommand(t, caller, nil,
		"table-get", "--node", "node1", "--sheet-id", "sheet1", "--range", "A1:B2", "--no-header")
	if err != nil {
		t.Fatalf("table-get returned error: %v", err)
	}
	want := sheetTableCall{
		productID: "sheet",
		toolName:  "table_get",
		args: map[string]any{
			"nodeId":   "node1",
			"sheetId":  "sheet1",
			"range":    "A1:B2",
			"noHeader": true,
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestSheetTableGetOmitsUnsetOptionalArgs(t *testing.T) {
	caller := &sheetTableCaller{}
	if err := executeSheetTableCommand(t, caller, nil, "table-get", "--node", "node1"); err != nil {
		t.Fatalf("table-get returned error: %v", err)
	}
	want := map[string]any{"nodeId": "node1"}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("calls = %#v, want args %#v", caller.calls, want)
	}
}

func TestSheetTableGetPreservesExplicitFalseNoHeader(t *testing.T) {
	caller := &sheetTableCaller{}
	if err := executeSheetTableCommand(t, caller, nil, "table-get", "--node", "node1", "--no-header=false"); err != nil {
		t.Fatalf("table-get returned error: %v", err)
	}
	if got, ok := caller.calls[0].args["noHeader"]; !ok || got != false {
		t.Fatalf("noHeader = %#v, present=%v; want explicit false", got, ok)
	}
}

func TestSheetTableGetRequiresNodeBeforeCall(t *testing.T) {
	caller := &sheetTableCaller{}
	if err := executeSheetTableCommand(t, caller, nil, "table-get"); err == nil {
		t.Fatal("table-get without --node returned nil error")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote calls = %d, want 0", len(caller.calls))
	}
}

func TestSheetTablePutNormalizesAcceptedInputs(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "array", input: `[ {"name":"Sheet1","columns":["name"],"data":[["Alice"]]} ]`},
		{name: "wrapped", input: `{"sheets":[{"name":"Sheet1","columns":["name"],"data":[["Alice"]]}]}`},
		{name: "single", input: `{"name":"Sheet1","columns":["name"],"data":[["Alice"]]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &sheetTableCaller{}
			if err := executeSheetTableCommand(t, caller, nil, "table-put", "--node", "node1", "--sheets", tt.input); err != nil {
				t.Fatalf("table-put returned error: %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("remote calls = %d, want 1", len(caller.calls))
			}
			call := caller.calls[0]
			if call.productID != "sheet" || call.toolName != "table_put" || call.args["nodeId"] != "node1" {
				t.Fatalf("call = %#v", call)
			}
			sheets, ok := call.args["sheets"].([]any)
			if !ok || len(sheets) != 1 {
				t.Fatalf("sheets = %#v, want one-element array", call.args["sheets"])
			}
		})
	}
}

func TestSheetTablePutReadsStdinAndFile(t *testing.T) {
	payload := `[{"name":"Sheet1","columns":["name"],"data":[["Alice"]]}]`

	t.Run("stdin", func(t *testing.T) {
		caller := &sheetTableCaller{}
		if err := executeSheetTableCommand(t, caller, strings.NewReader(payload), "table-put", "--node", "node1", "--sheets", "-"); err != nil {
			t.Fatalf("table-put stdin returned error: %v", err)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("remote calls = %d, want 1", len(caller.calls))
		}
	})

	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "table.json")
		if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
			t.Fatalf("write table JSON: %v", err)
		}
		caller := &sheetTableCaller{}
		if err := executeSheetTableCommand(t, caller, nil, "table-put", "--node", "node1", "--sheets", "@"+path); err != nil {
			t.Fatalf("table-put file returned error: %v", err)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("remote calls = %d, want 1", len(caller.calls))
		}
	})
}

func TestSheetTablePutRejectsInvalidInputBeforeCall(t *testing.T) {
	for _, input := range []string{`not-json`, `123`, `[]`, `{"sheets":{}}`, `{"sheets":[]}`} {
		caller := &sheetTableCaller{}
		if err := executeSheetTableCommand(t, caller, nil, "table-put", "--node", "node1", "--sheets", input); err == nil {
			t.Errorf("input %q returned nil error", input)
		}
		if len(caller.calls) != 0 {
			t.Errorf("input %q made %d remote calls, want 0", input, len(caller.calls))
		}
	}
}

func TestSheetTablePutDryRunSkipsRemoteCall(t *testing.T) {
	caller := &sheetTableCaller{dryRun: true}
	input := `[{"name":"Sheet1","columns":["name"],"data":[["Alice"]]}]`
	if err := executeSheetTableCommand(t, caller, nil, "table-put", "--node", "node1", "--sheets", input); err != nil {
		t.Fatalf("table-put dry-run returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote calls = %d, want 0", len(caller.calls))
	}
}

func TestSheetBatchUpdateRejectsTableCommands(t *testing.T) {
	for _, toolName := range []string{"table-get", "table-put"} {
		_, err := translateBatchOp(map[string]any{
			"toolName": toolName,
			"input":    map[string]any{"sheet-id": "sheet1"},
		})
		if err == nil || !strings.Contains(err.Error(), toolName) {
			t.Errorf("translateBatchOp(%q) error = %v, want rejection naming command", toolName, err)
		}
	}
}
