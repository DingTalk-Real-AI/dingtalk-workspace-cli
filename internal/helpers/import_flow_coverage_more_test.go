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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func importCoverageCommand(t *testing.T, filePath string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "import"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("folder", "", "")
	cmd.Flags().String("workspace", "", "")
	if err := cmd.Flags().Set("file", filePath); err != nil {
		t.Fatalf("set import file: %v", err)
	}
	if err := cmd.Flags().Set("workspace", "workspace-coverage"); err != nil {
		t.Fatalf("set import workspace: %v", err)
	}
	return cmd
}

func TestCrossPlatformCoverageImportFlowRemainingBranches(t *testing.T) {
	t.Run("targetless doc import omits target fields from the session", func(t *testing.T) {
		previousDeps := deps
		previousArgs := os.Args
		t.Cleanup(func() {
			deps = previousDeps
			os.Args = previousArgs
			SetHTTPPutFile(nil)
		})

		caller := &sheetImportCaller{responses: map[string][]string{
			"create_import_session": {`{"sessionId":"session-default-root","uploadUrl":"https://upload.example.test/object"}`},
			"confirm_import":        {`{"taskId":"task-default-root"}`},
			"query_import_task":     {`{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-default-root"}`},
		}}
		InitDeps(caller)
		var output bytes.Buffer
		deps.Out.w = &output
		os.Args = []string{"dws", "doc"}
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })

		cmd := importCoverageCommand(t, writeImportFixture(t, "md"))
		if err := cmd.Flags().Set("workspace", ""); err != nil {
			t.Fatal(err)
		}
		cfg := docImportFlowConfig()
		cfg.poll.maxPolls = 1
		cfg.poll.interval = func(int) time.Duration { return 0 }
		cfg.poll.wait = func(context.Context, time.Duration) error { return nil }
		if err := runImportCommand(cmd, nil, cfg); err != nil {
			t.Fatalf("targetless doc import failed: %v", err)
		}
		if len(caller.calls) != 3 {
			t.Fatalf("targetless doc import calls = %#v, want create/confirm/query", caller.calls)
		}
		if args := caller.calls[0].args; args["targetFolderId"] != nil || args["workspaceId"] != nil {
			t.Fatalf("default-root session unexpectedly carries target fields: %#v", args)
		}
	})

	t.Run("missing flag aliases are skipped", func(t *testing.T) {
		cmd := &cobra.Command{Use: "import"}
		cmd.Flags().String("folder", "", "")
		if err := cmd.Flags().Set("folder", "folder-1"); err != nil {
			t.Fatal(err)
		}
		if got := importFlagValue(cmd, "folder-token", "folder"); got != "folder-1" {
			t.Fatalf("importFlagValue() = %q, want folder-1", got)
		}
	})

	t.Run("zero poll policy falls back and honors cancellation", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := pollImportTask(ctx, "task-cancelled", importFlowConfig{serverID: "doc"})
		if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
			t.Fatalf("pollImportTask() error = %v, want cancellation", err)
		}
	})

	t.Run("nil command context and legacy timeout error", func(t *testing.T) {
		oldArgs := os.Args
		os.Args = []string{"dws", "doc"}
		t.Cleanup(func() { os.Args = oldArgs })

		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"sessionId":"session-1","uploadUrl":"https://upload.example.test/object"}`},
			{text: `{"taskId":"task-1"}`},
			{text: `{"status":"processing"}`},
		}})
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })

		cfg := docImportFlowConfig()
		cfg.poll.maxPolls = 1
		cfg.poll.interval = func(int) time.Duration { return 0 }
		cfg.poll.wait = func(context.Context, time.Duration) error { return nil }
		err := runImportCommand(importCoverageCommand(t, writeImportFixture(t, "md")), nil, cfg)
		if err == nil || !strings.Contains(err.Error(), "手动查询") {
			t.Fatalf("runImportCommand() error = %v, want manual-query timeout", err)
		}
	})

	t.Run("get command accepts a nil command context", func(t *testing.T) {
		oldArgs := os.Args
		os.Args = []string{"dws", "doc"}
		t.Cleanup(func() { os.Args = oldArgs })

		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"status":"processing"}`}}})
		cmd := &cobra.Command{Use: "get"}
		cmd.Flags().String("task-id", "task-1", "")
		if err := runImportGetCommand(cmd, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportGetCommand() error = %v", err)
		}
	})

	t.Run("root document URL has no node ID", func(t *testing.T) {
		if got := extractNodeIDFromDocURL("/"); got != "" {
			t.Fatalf("extractNodeIDFromDocURL(/) = %q, want empty", got)
		}
	})
}
