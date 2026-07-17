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
	"reflect"
	"strings"
	"testing"
)

func TestDriveTaskGetCommandRegistered(t *testing.T) {
	root := newDriveCommand()
	command, remaining, err := root.Find([]string{"task", "get"})
	if err != nil || len(remaining) != 0 || command == nil || command.Name() != "get" {
		t.Fatalf("drive task get not registered: command=%v remaining=%v err=%v", command, remaining, err)
	}
}

func TestDriveTaskGetExportRoutesToDocQueryExportJob(t *testing.T) {
	caller := &driveParityCaller{}
	_, err := executeDriveParityCommand(t, caller, "", "task", "get", "--type", "export", "--id", "job-1")
	if err != nil {
		t.Fatalf("drive task get export returned error: %v", err)
	}

	want := []driveParityCall{{
		server: "doc",
		tool:   "query_export_job",
		args:   map[string]any{"jobId": "job-1"},
	}}
	if !reflect.DeepEqual(caller.calls, want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestDriveTaskGetImportRoutesToDocQueryImportTask(t *testing.T) {
	caller := &driveParityCaller{}
	_, err := executeDriveParityCommand(t, caller, "", "task", "get", "--type", "import", "--id", "task-1")
	if err != nil {
		t.Fatalf("drive task get import returned error: %v", err)
	}

	want := []driveParityCall{{
		server: "doc",
		tool:   "query_import_task",
		args:   map[string]any{"taskId": "task-1"},
	}}
	if !reflect.DeepEqual(caller.calls, want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestDriveTaskGetRejectsInvalidFlagsBeforeCallingMCP(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing type",
			args:    []string{"task", "get", "--id", "job-1"},
			wantErr: "--type",
		},
		{
			name:    "missing id",
			args:    []string{"task", "get", "--type", "export"},
			wantErr: "--id",
		},
		{
			name:    "unsupported type",
			args:    []string{"task", "get", "--type", "unknown", "--id", "task-1"},
			wantErr: `unsupported --type "unknown"; expected export or import`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &driveParityCaller{}
			_, err := executeDriveParityCommand(t, caller, "", tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("calls = %#v, want none", caller.calls)
			}
		})
	}
}

func TestDriveTaskGetDryRunDoesNotCallMCPAndPrintsID(t *testing.T) {
	caller := &driveParityCaller{dryRun: true}
	output, err := executeDriveParityCommand(t, caller, "", "task", "get", "--type", "export", "--id", "job-1", "--dry-run")
	if err != nil {
		t.Fatalf("drive task get dry-run returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want none", caller.calls)
	}
	if !strings.Contains(output, "job-1") {
		t.Fatalf("dry-run output = %q, want task ID", output)
	}
}

func TestDriveTaskGetPrintsUnifiedJSON(t *testing.T) {
	caller := &driveParityCaller{}
	output, err := executeDriveParityCommand(t, caller, "", "task", "get", "--type", "export", "--id", "job-1")
	if err != nil {
		t.Fatalf("drive task get returned error: %v", err)
	}

	want := "{\n  \"id\": \"job-1\",\n  \"type\": \"export\",\n  \"status\": \"PROCESSING\"\n}\n"
	if output != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
}
