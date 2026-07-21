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
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeAitableParityCommand(t *testing.T, caller *driveParityCaller, args ...string) (string, error) {
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
	root.SetOut(&output)
	root.SetErr(&output)
	root.AddCommand(newAitableCommand())
	root.SetArgs(append([]string{"aitable"}, args...))
	return output.String(), root.Execute()
}

func TestAitableViewCreateVisibleFieldIDsAssemblesConfig(t *testing.T) {
	caller := &driveParityCaller{}
	_, err := executeAitableParityCommand(t, caller,
		"view", "create", "--base-id", "base-1", "--table-id", "table-1", "--view-type", "Grid",
		"--config", `{"sort":{"fieldId":"fldStatus","direction":"asc"}}`,
		"--visible-field-ids", "fldPrimary, fldStatus")
	if err != nil {
		t.Fatalf("view create returned error: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "create_view" {
		t.Fatalf("calls = %#v, want one create_view call", caller.calls)
	}
	config, ok := caller.calls[0].args["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %#v, want object", caller.calls[0].args["config"])
	}
	if got := config["visibleFieldIds"]; !reflect.DeepEqual(got, []string{"fldPrimary", "fldStatus"}) {
		t.Fatalf("visibleFieldIds = %#v", got)
	}
	if got, ok := config["sort"].([]any); !ok || len(got) != 1 {
		t.Fatalf("sort = %#v, want normalized one-item array", config["sort"])
	}
}

func TestAitableViewCreateRejectsInvalidConfigBeforeCall(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		wantError string
	}{
		{name: "malformed", config: "not-json", wantError: "--config JSON parse failed"},
		{name: "non-object", config: `[{"visibleFieldIds":["fldPrimary"]}]`, wantError: "must be a JSON object"},
		{name: "unsupported", config: `{"description":{"content":[]}}`, wantError: "--desc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &driveParityCaller{}
			_, err := executeAitableParityCommand(t, caller,
				"view", "create", "--base-id", "base-1", "--table-id", "table-1", "--view-type", "Grid",
				"--config", test.config)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want %q", err, test.wantError)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("calls = %#v, want none", caller.calls)
			}
		})
	}
}

func TestAitableViewCreateRejectsDuplicateVisibleFieldInputs(t *testing.T) {
	caller := &driveParityCaller{}
	_, err := executeAitableParityCommand(t, caller,
		"view", "create", "--base-id", "base-1", "--table-id", "table-1", "--view-type", "Grid",
		"--config", `{"visibleFieldIds":["fldPrimary"]}`,
		"--visible-field-ids", "fldPrimary,fldStatus")
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("error = %v, want duplicate input error", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want none", caller.calls)
	}
}

func TestAitableFieldCreateKeepsBatchPrecedenceForMixedInputs(t *testing.T) {
	caller := &driveParityCaller{}
	_, err := executeAitableParityCommand(t, caller,
		"field", "create", "--base-id", "base-1", "--table-id", "table-1",
		"--fields", `[{"fieldName":"状态","type":"text"}]`,
		"--config", `{"options":[{"name":"完成"}]}`)
	if err != nil {
		t.Fatalf("field create returned error: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "create_fields" {
		t.Fatalf("calls = %#v, want one create_fields call", caller.calls)
	}
	wantFields := []any{map[string]any{"fieldName": "状态", "type": "text"}}
	if got := caller.calls[0].args["fields"]; !reflect.DeepEqual(got, wantFields) {
		t.Fatalf("fields = %#v, want %#v", got, wantFields)
	}
}

func TestAitableFieldCreateRejectsIncompleteSingleMode(t *testing.T) {
	caller := &driveParityCaller{}
	_, err := executeAitableParityCommand(t, caller,
		"field", "create", "--base-id", "base-1", "--table-id", "table-1",
		"--options", `[{"name":"完成"}]`)
	if err == nil || !strings.Contains(err.Error(), "requires both --name and --type") {
		t.Fatalf("error = %v, want incomplete single-mode error", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want none", caller.calls)
	}
}

func TestAitableRecordUpdateDocumentsFieldNameCompatibility(t *testing.T) {
	root := newAitableCommand()
	command, remaining, err := root.Find([]string{"record", "update"})
	if err != nil || len(remaining) != 0 || command == nil {
		t.Fatalf("record update command not found: command=%v remaining=%v err=%v", command, remaining, err)
	}
	for _, expected := range []string{"fieldId", "字段名", "精确匹配", "fieldId 的值优先"} {
		if !strings.Contains(command.Long, expected) {
			t.Errorf("record update help missing %q", expected)
		}
	}
}
