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
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/asynctask"
)

func TestQueryAsyncTaskExportRoutesToDocAndParsesResultEnvelope(t *testing.T) {
	call := func(_ context.Context, server, tool string, args map[string]any) (string, error) {
		if server != "doc" {
			t.Fatalf("server = %q, want doc", server)
		}
		if tool != "query_export_job" {
			t.Fatalf("tool = %q, want query_export_job", tool)
		}
		if !reflect.DeepEqual(args, map[string]any{"jobId": "job-1"}) {
			t.Fatalf("args = %#v, want jobId", args)
		}
		return `{"result":{"status":"completed","downloadUrl":"https://download.invalid","message":"ready","createTime":"now"}}`, nil
	}

	got, err := queryAsyncTaskWith(context.Background(), call, "export", "job-1")
	if err != nil {
		t.Fatalf("queryAsyncTaskWith() error = %v", err)
	}
	want := asynctask.TaskResult{
		ID:         "job-1",
		Type:       "export",
		Status:     asynctask.StatusSuccess,
		ResultURL:  "https://download.invalid",
		Message:    "ready",
		CreateTime: "now",
	}
	if got != want {
		t.Fatalf("queryAsyncTaskWith() = %#v, want %#v", got, want)
	}
}

func TestQueryAsyncTaskImportRoutesToDocAndParsesTopLevelResponse(t *testing.T) {
	call := func(_ context.Context, server, tool string, args map[string]any) (string, error) {
		if server != "doc" {
			t.Fatalf("server = %q, want doc", server)
		}
		if tool != "query_import_task" {
			t.Fatalf("tool = %q, want query_import_task", tool)
		}
		if !reflect.DeepEqual(args, map[string]any{"taskId": "task-1"}) {
			t.Fatalf("args = %#v, want taskId", args)
		}
		return `{"status":"running","documentUrl":"https://docs.invalid/1","message":"working","createTime":"earlier"}`, nil
	}

	got, err := queryAsyncTaskWith(context.Background(), call, "import", "task-1")
	if err != nil {
		t.Fatalf("queryAsyncTaskWith() error = %v", err)
	}
	want := asynctask.TaskResult{
		ID:         "task-1",
		Type:       "import",
		Status:     asynctask.StatusProcessing,
		ResultURL:  "https://docs.invalid/1",
		Message:    "working",
		CreateTime: "earlier",
	}
	if got != want {
		t.Fatalf("queryAsyncTaskWith() = %#v, want %#v", got, want)
	}
}

func TestQueryAsyncTaskReturnsBusinessErrors(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  string
	}{
		{
			name:     "top level",
			response: `{"success":false,"message":"top-level failed"}`,
			wantErr:  "top-level failed",
		},
		{
			name:     "result envelope",
			response: `{"success":true,"result":{"success":false,"message":"nested failed"}}`,
			wantErr:  "nested failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := func(context.Context, string, string, map[string]any) (string, error) {
				return tt.response, nil
			}
			_, err := queryAsyncTaskWith(context.Background(), call, "export", "job-1")
			if err == nil {
				t.Fatal("queryAsyncTaskWith() error = nil, want business error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("queryAsyncTaskWith() error = %q, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestQueryAsyncTaskRejectsInvalidJSON(t *testing.T) {
	call := func(context.Context, string, string, map[string]any) (string, error) {
		return `{`, nil
	}

	_, err := queryAsyncTaskWith(context.Background(), call, "export", "job-1")
	if err == nil {
		t.Fatal("queryAsyncTaskWith() error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), "parse async task response") {
		t.Fatalf("queryAsyncTaskWith() error = %q, want parse context", err)
	}
}

func TestQueryAsyncTaskRejectsUnsupportedTypeBeforeCallingMCP(t *testing.T) {
	called := false
	call := func(context.Context, string, string, map[string]any) (string, error) {
		called = true
		return `{}`, nil
	}

	_, err := queryAsyncTaskWith(context.Background(), call, "archive", "task-1")
	if err == nil {
		t.Fatal("queryAsyncTaskWith() error = nil, want unsupported type error")
	}
	if !strings.Contains(err.Error(), `unsupported task type "archive"; expected export or import`) {
		t.Fatalf("queryAsyncTaskWith() error = %q", err)
	}
	if called {
		t.Fatal("MCP caller ran for unsupported task type")
	}
}

func TestTaskIDFromFlags(t *testing.T) {
	tests := []struct {
		name       string
		taskID     string
		jobID      string
		want       string
		wantErrAll []string
	}{
		{name: "task id", taskID: "task-1", want: "task-1"},
		{name: "job id", jobID: "job-1", want: "job-1"},
		{name: "both", taskID: "task-1", jobID: "job-1", wantErrAll: []string{"--task-id", "--job-id"}},
		{name: "neither", wantErrAll: []string{"--task-id"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "get", Example: "dws doc export get --task-id <ID>"}
			cmd.Flags().String("task-id", "", "task ID")
			cmd.Flags().String("job-id", "", "job ID")
			if tt.taskID != "" {
				if err := cmd.Flags().Set("task-id", tt.taskID); err != nil {
					t.Fatal(err)
				}
			}
			if tt.jobID != "" {
				if err := cmd.Flags().Set("job-id", tt.jobID); err != nil {
					t.Fatal(err)
				}
			}

			got, err := taskIDFromFlags(cmd)
			if len(tt.wantErrAll) == 0 {
				if err != nil {
					t.Fatalf("taskIDFromFlags() error = %v", err)
				}
				if got != tt.want {
					t.Fatalf("taskIDFromFlags() = %q, want %q", got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatal("taskIDFromFlags() error = nil, want error")
			}
			for _, fragment := range tt.wantErrAll {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("taskIDFromFlags() error = %q, want %q", err, fragment)
				}
			}
		})
	}
}
