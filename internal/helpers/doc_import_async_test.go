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
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/asynctask"
)

func decodeSingleTaskResult(t *testing.T, stdout string) asynctask.TaskResult {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	var result asynctask.TaskResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("stdout is not a TaskResult: %v; stdout=%q", err, stdout)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout contains data after TaskResult: err=%v stdout=%q", err, stdout)
	}
	return result
}

func installDocImportUploadStub(t *testing.T, wantURL, wantPath string, calls *int) {
	t.Helper()
	SetHTTPPutFile(func(_ context.Context, uploadURL string, headers map[string]string, filePath string, fileSize int64) error {
		(*calls)++
		if uploadURL != wantURL || filePath != wantPath || fileSize <= 0 {
			return errors.New("unexpected upload request")
		}
		if len(headers) != 0 {
			return errors.New("unexpected upload headers")
		}
		return nil
	})
	t.Cleanup(func() { SetHTTPPutFile(nil) })
}

func TestDocImportAsyncReturnsPendingAfterUploadWithoutQuery(t *testing.T) {
	filePath := writeImportFixture(t, "md")
	uploadCalls := 0
	installDocImportUploadStub(t, "https://upload.example.test/object", filePath, &uploadCalls)

	type contextKey string
	const key contextKey = "doc-import-async"
	ctx := context.WithValue(context.Background(), key, "context-value")
	caller := &docAsyncRecordingCaller{
		format: "json",
		responses: []docAsyncRecordedResponse{
			{text: `{"sessionId":"session-1","uploadUrl":"https://upload.example.test/object"}`},
			{text: `{"taskId":"task-1"}`},
		},
	}
	stdout, err := runDocAsyncCapturedCommand(t, ctx, caller, "import", "--file", filePath, "--async")
	if err != nil {
		t.Fatalf("doc import --async error = %v", err)
	}
	if uploadCalls != 1 {
		t.Fatalf("upload calls = %d, want 1", uploadCalls)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("MCP calls = %#v, want create+confirm only", caller.calls)
	}
	wantTools := []string{"create_import_session", "confirm_import"}
	for i, call := range caller.calls {
		if call.server != "doc" || call.tool != wantTools[i] {
			t.Fatalf("call[%d] = %s/%s, want doc/%s", i, call.server, call.tool, wantTools[i])
		}
		if call.ctx.Value(key) != "context-value" {
			t.Fatalf("call[%d] did not use cmd.Context(): %#v", i, call.ctx)
		}
	}

	want := asynctask.TaskResult{
		ID:      "task-1",
		Type:    "import",
		Status:  asynctask.StatusPending,
		Message: "任务已提交，请稍后查询",
	}
	if got := decodeSingleTaskResult(t, stdout); !reflect.DeepEqual(got, want) {
		t.Fatalf("TaskResult = %#v, want %#v", got, want)
	}
}

func TestDocImportAsyncJSONFormatIsCaseInsensitive(t *testing.T) {
	filePath := writeImportFixture(t, "md")
	uploadCalls := 0
	installDocImportUploadStub(t, "https://upload.example.test/object", filePath, &uploadCalls)
	caller := &docAsyncRecordingCaller{
		format: " JSON ",
		responses: []docAsyncRecordedResponse{
			{text: `{"sessionId":"session-1","uploadUrl":"https://upload.example.test/object"}`},
			{text: `{"taskId":"task-1"}`},
		},
	}
	stdout, err := runDocAsyncCapturedCommand(t, context.Background(), caller, "import", "--file", filePath, "--async")
	if err != nil {
		t.Fatalf("doc import --async --format JSON error = %v", err)
	}
	if result := decodeSingleTaskResult(t, stdout); result.ID != "task-1" || result.Status != asynctask.StatusPending {
		t.Fatalf("TaskResult = %#v", result)
	}
}

func TestDocImportAsyncSyncModeStillPollsAndKeepsJSONPure(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})
	os.Args = []string{"dws", "doc"}

	filePath := writeImportFixture(t, "md")
	uploadCalls := 0
	installDocImportUploadStub(t, "https://upload.example.test/object", filePath, &uploadCalls)
	caller := &docAsyncRecordingCaller{
		format: "json",
		responses: []docAsyncRecordedResponse{
			{text: `{"sessionId":"session-1","uploadUrl":"https://upload.example.test/object"}`},
			{text: `{"taskId":"task-1"}`},
			{text: `{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-1","documentName":"sales","documentType":"doc"}`},
		},
	}
	InitDeps(caller)
	var stdout bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = io.Discard

	cfg := docImportFlowConfig()
	cfg.poll.maxPolls = 1
	cfg.poll.interval = func(int) time.Duration { return 0 }
	cfg.poll.wait = func(context.Context, time.Duration) error { return nil }
	if err := runImportCommand(importCoverageCommand(t, filePath), nil, cfg); err != nil {
		t.Fatalf("sync doc import error = %v", err)
	}
	if uploadCalls != 1 {
		t.Fatalf("upload calls = %d, want 1", uploadCalls)
	}
	if len(caller.calls) != 3 || caller.calls[2].tool != "query_import_task" {
		t.Fatalf("sync calls = %#v, want create+confirm+query", caller.calls)
	}
	var result map[string]any
	decoder := json.NewDecoder(strings.NewReader(stdout.String()))
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("sync stdout is not pure JSON: %v; stdout=%q", err, stdout.String())
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("sync stdout contains progress after JSON: err=%v stdout=%q", err, stdout.String())
	}
	if result["taskId"] != "task-1" || result["documentUrl"] != "https://alidocs.dingtalk.com/i/nodes/node-1" {
		t.Fatalf("sync result = %#v", result)
	}
}

func TestDocImportGetUnifiedMapsCompletedResult(t *testing.T) {
	caller := &docAsyncRecordingCaller{
		format:    "json",
		responses: []docAsyncRecordedResponse{{text: `{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-1"}`}},
	}
	stdout, err := runDocAsyncCapturedCommand(t, context.Background(), caller, "import", "get", "--task-id", "task-1")
	if err != nil {
		t.Fatalf("doc import get error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].server != "doc" || caller.calls[0].tool != "query_import_task" {
		t.Fatalf("query calls = %#v", caller.calls)
	}
	want := asynctask.TaskResult{
		ID:        "task-1",
		Type:      "import",
		Status:    asynctask.StatusSuccess,
		ResultURL: "https://alidocs.dingtalk.com/i/nodes/node-1",
	}
	if got := decodeSingleTaskResult(t, stdout); !reflect.DeepEqual(got, want) {
		t.Fatalf("TaskResult = %#v, want %#v", got, want)
	}
}

func TestDocImportGetUnifiedFailedAndTimeoutPrintResultThenError(t *testing.T) {
	for _, test := range []struct {
		name   string
		status asynctask.Status
	}{
		{name: "failed", status: asynctask.StatusFailed},
		{name: "timeout", status: asynctask.StatusTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &docAsyncRecordingCaller{
				format: "json",
				responses: []docAsyncRecordedResponse{{
					text: `{"status":"` + strings.ToLower(string(test.status)) + `","message":"task message"}`,
				}},
			}
			stdout, err := runDocAsyncCapturedCommand(t, context.Background(), caller, "import", "get", "--task-id", "task-1")
			if err == nil {
				t.Fatal("doc import get error = nil, want nonzero error")
			}
			result := decodeSingleTaskResult(t, stdout)
			if result.ID != "task-1" || result.Type != "import" || result.Status != test.status || result.Message != "task message" {
				t.Fatalf("structured result = %#v", result)
			}
		})
	}
}

func TestDocImportGetUnifiedDoesNotChangeSheetImportGet(t *testing.T) {
	caller := &sheetImportCaller{responses: map[string][]string{
		"query_import_task": {`{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-sheet/","documentType":"1"}`},
	}}
	stdout, err := executeSheetImportCommand(t, caller, fastSheetImportConfig(), "get", "--task-id", "task-sheet")
	if err != nil {
		t.Fatalf("sheet import get error = %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("sheet import get output is not its historical JSON object: %v; stdout=%q", err, stdout)
	}
	if result["status"] != "completed" || result["documentUrl"] != "https://alidocs.dingtalk.com/i/nodes/node-sheet/" || result["nodeId"] != "node-sheet" {
		t.Fatalf("sheet import get result = %#v", result)
	}
	if _, ok := result["id"]; ok {
		t.Fatalf("sheet import get unexpectedly adopted TaskResult: %#v", result)
	}
	if _, ok := result["resultUrl"]; ok {
		t.Fatalf("sheet import get unexpectedly renamed documentUrl: %#v", result)
	}
}

func TestDocImportAsyncCreateSharesHandlerAndPublicFlags(t *testing.T) {
	root := newDocCommand()
	for _, fragment := range []string{
		"dws doc import create",
		"dws doc import                        导入本地文件为在线文档 (兼容入口",
		"dws doc import get                    按任务 ID 查询异步或未完成的导入任务",
	} {
		if !strings.Contains(root.Long, fragment) {
			t.Errorf("doc root overview missing %q:\n%s", fragment, root.Long)
		}
	}
	importCmd, remaining, err := root.Find([]string{"import"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("find doc import = (%v, %v)", remaining, err)
	}
	createCmd, remaining, err := root.Find([]string{"import", "create"})
	if err != nil || len(remaining) != 0 || createCmd == nil || createCmd.Name() != "create" {
		t.Fatalf("find doc import create = command=%v remaining=%v err=%v", createCmd, remaining, err)
	}
	if createCmd.Hidden || !createCmd.Runnable() || createCmd.HasSubCommands() {
		t.Fatalf("doc import create must be a visible runnable leaf: hidden=%v runnable=%v children=%v", createCmd.Hidden, createCmd.Runnable(), createCmd.Commands())
	}
	if importCmd.RunE == nil || createCmd.RunE == nil || reflect.ValueOf(importCmd.RunE).Pointer() != reflect.ValueOf(createCmd.RunE).Pointer() {
		t.Fatal("doc import and doc import create do not share the same RunE handler")
	}

	publicFlags := func(flags *pflag.FlagSet) []string {
		var names []string
		flags.VisitAll(func(flag *pflag.Flag) {
			if !flag.Hidden {
				names = append(names, flag.Name)
			}
		})
		sort.Strings(names)
		return names
	}
	wantFlags := []string{"async", "file", "folder", "name", "workspace"}
	if got := publicFlags(importCmd.LocalNonPersistentFlags()); !reflect.DeepEqual(got, wantFlags) {
		t.Fatalf("doc import public flags = %v, want %v", got, wantFlags)
	}
	if got := publicFlags(createCmd.LocalNonPersistentFlags()); !reflect.DeepEqual(got, wantFlags) {
		t.Fatalf("doc import create public flags = %v, want %v", got, wantFlags)
	}
	for _, command := range []*cobra.Command{importCmd, createCmd} {
		if flag := command.Flags().Lookup("async"); flag == nil || flag.DefValue != "false" {
			t.Fatalf("%s --async = %#v, want default false", command.CommandPath(), flag)
		}
		for _, name := range []string{"folder", "workspace"} {
			flag := command.Flags().Lookup(name)
			if flag == nil || !strings.Contains(flag.Usage, "可选") || strings.Contains(flag.Usage, "至少") {
				t.Fatalf("%s --%s help = %q, want optional destination matching runtime", command.CommandPath(), name, flag.Usage)
			}
		}
		for _, name := range []string{"folder-id", "workspace-id"} {
			flag := command.Flags().Lookup(name)
			if flag == nil || !flag.Hidden {
				t.Fatalf("%s --%s = %#v, want hidden compatibility flag", command.CommandPath(), name, flag)
			}
		}
	}
}

func TestDocImportAsyncCreateDispatchesSharedFlow(t *testing.T) {
	filePath := writeImportFixture(t, "md")
	uploadCalls := 0
	installDocImportUploadStub(t, "https://upload.example.test/object", filePath, &uploadCalls)
	caller := &docAsyncRecordingCaller{
		format: "json",
		responses: []docAsyncRecordedResponse{
			{text: `{"sessionId":"session-1","uploadUrl":"https://upload.example.test/object"}`},
			{text: `{"taskId":"task-1"}`},
		},
	}
	stdout, err := runDocAsyncCapturedCommand(t, context.Background(), caller, "import", "create", "--file", filePath, "--async")
	if err != nil {
		t.Fatalf("doc import create --async error = %v", err)
	}
	if uploadCalls != 1 || len(caller.calls) != 2 || caller.calls[0].tool != "create_import_session" || caller.calls[1].tool != "confirm_import" {
		t.Fatalf("create flow upload=%d calls=%#v", uploadCalls, caller.calls)
	}
	if result := decodeSingleTaskResult(t, stdout); result.ID != "task-1" || result.Status != asynctask.StatusPending {
		t.Fatalf("create TaskResult = %#v", result)
	}
}
