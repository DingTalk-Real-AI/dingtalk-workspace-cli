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
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/asynctask"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type sheetExportRecordedCall struct {
	ctx    context.Context
	server string
	tool   string
	args   map[string]any
}

type sheetExportRecordedResponse struct {
	text string
	err  error
}

type sheetExportRecordingCaller struct {
	format    string
	dryRun    bool
	responses []sheetExportRecordedResponse
	calls     []sheetExportRecordedCall
	afterCall func(int)
}

func (c *sheetExportRecordingCaller) CallTool(ctx context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	clonedArgs := make(map[string]any, len(args))
	for key, value := range args {
		clonedArgs[key] = value
	}
	c.calls = append(c.calls, sheetExportRecordedCall{ctx: ctx, server: server, tool: tool, args: clonedArgs})

	index := len(c.calls) - 1
	if c.afterCall != nil {
		c.afterCall(index)
	}
	if len(c.responses) == 0 {
		return textToolResult(`{}`), nil
	}
	if index >= len(c.responses) {
		index = len(c.responses) - 1
	}
	response := c.responses[index]
	if response.err != nil {
		return nil, response.err
	}
	return textToolResult(response.text), nil
}

func (c *sheetExportRecordingCaller) Format() string {
	if c.format == "" {
		return "table"
	}
	return c.format
}

func (c *sheetExportRecordingCaller) DryRun() bool { return c.dryRun }
func (*sheetExportRecordingCaller) Fields() string { return "" }
func (*sheetExportRecordingCaller) JQ() string     { return "" }

func executeSheetExportAsyncCommand(t *testing.T, ctx context.Context, caller edition.ToolCaller, args ...string) (string, error) {
	t.Helper()
	previousDeps, previousArgs := deps, os.Args
	defer func() {
		deps = previousDeps
		os.Args = previousArgs
	}()

	InitDeps(caller)
	var stdout, stderr bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = &stderr
	os.Args = append([]string{"dws", "sheet", "export"}, args...)
	root := newExportCmd()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	return stdout.String(), err
}

func decodeExactlyOneSheetTaskResult(t *testing.T, output string) asynctask.TaskResult {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(output))
	var result asynctask.TaskResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("stdout is not one TaskResult JSON object: %v; stdout=%q", err, output)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout contains data after TaskResult: err=%v stdout=%q", err, output)
	}
	return result
}

func TestSheetExportAsyncReturnsOnePendingTaskWithoutPollingOrDownload(t *testing.T) {
	type contextKey string
	const key contextKey = "sheet-export-submit"
	ctx := context.WithValue(context.Background(), key, "command-context")

	previousHTTPGet := httpGetFile
	downloadCalls := 0
	httpGetFile = func(context.Context, string, map[string]string, string) error {
		downloadCalls++
		return nil
	}
	t.Cleanup(func() { httpGetFile = previousHTTPGet })

	caller := &sheetExportRecordingCaller{
		format:    " JSON ",
		responses: []sheetExportRecordedResponse{{text: `{"result":{"jobId":"job-1"}}`}},
	}
	output, err := executeSheetExportAsyncCommand(t, ctx, caller, "--node", "node-1", "--async")
	if err != nil {
		t.Fatalf("sheet export --async error = %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("MCP calls = %#v, want one submit and no query", caller.calls)
	}
	call := caller.calls[0]
	if call.server != "sheet" || call.tool != "submit_export_job" {
		t.Fatalf("submit call = %s/%s, want sheet/submit_export_job", call.server, call.tool)
	}
	if !reflect.DeepEqual(call.args, map[string]any{"nodeId": "node-1", "exportFormat": "xlsx"}) {
		t.Fatalf("submit args = %#v", call.args)
	}
	if call.ctx.Value(key) != "command-context" {
		t.Fatalf("submit context was not cmd.Context(): %#v", call.ctx)
	}
	if downloadCalls != 0 {
		t.Fatalf("download calls = %d, want zero", downloadCalls)
	}

	result := decodeExactlyOneSheetTaskResult(t, output)
	want := asynctask.TaskResult{
		ID:      "job-1",
		Type:    "export",
		Status:  asynctask.StatusPending,
		Message: "任务已提交，请稍后查询",
	}
	if result != want {
		t.Fatalf("TaskResult = %#v, want %#v", result, want)
	}
}

func TestParseExportSubmitResultMissingJobIDDoesNotLeakResponse(t *testing.T) {
	const signedURL = "https://upload.example.test/object?token=temporary&signature=private"
	_, err := parseExportSubmitResult(`{"success":true,"downloadUrl":"` + signedURL + `"}`)
	if err == nil || !strings.Contains(err.Error(), "未返回 jobId") {
		t.Fatalf("missing jobId error = %v", err)
	}
	for _, forbidden := range []string{signedURL, "upload.example.test", "token=temporary", "signature=private"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("missing jobId error leaked %q: %v", forbidden, err)
		}
	}
}

func TestSheetExportSubmitBusinessErrorDoesNotLeakSignedURL(t *testing.T) {
	const signedURL = "https://download.example.test/object?token=sheet-secret&signature=private"
	caller := &sheetExportRecordingCaller{
		format:    "json",
		responses: []sheetExportRecordedResponse{{text: `{"success":false,"message":"sheet submit failed","downloadUrl":"` + signedURL + `"}`}},
	}
	stdout, err := executeSheetExportAsyncCommand(t, context.Background(), caller,
		"create", "--node", "node-1", "--async", "--format", "json")
	if err == nil || !strings.Contains(err.Error(), "sheet submit failed") {
		t.Fatalf("submit error = %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("submit error stdout = %q, want empty", stdout)
	}
	for _, forbidden := range []string{signedURL, "download.example.test", "sheet-secret", "signature=private"} {
		if strings.Contains(stdout, forbidden) || strings.Contains(err.Error(), forbidden) {
			t.Fatalf("submit error leaked %q: stdout=%q error=%v", forbidden, stdout, err)
		}
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "submit_export_job" {
		t.Fatalf("submit calls = %#v, want one submit call", caller.calls)
	}
}

func TestSheetExportSyncPollErrorIncludesSubmittedTaskID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	caller := &sheetExportRecordingCaller{
		format:    " JSON ",
		responses: []sheetExportRecordedResponse{{text: `{"jobId":"job-resumable"}`}},
		afterCall: func(index int) {
			if index == 0 {
				cancel()
			}
		},
	}

	output, err := executeSheetExportAsyncCommand(t, ctx, caller, "--node", "node-1")
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("sync poll error = %v, want wrapped context cancellation", err)
	}
	if !strings.Contains(err.Error(), "taskId=job-resumable") {
		t.Fatalf("sync poll error lost resumable task ID: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "submit_export_job" {
		t.Fatalf("calls = %#v, want completed submit before cancellation", caller.calls)
	}
	if strings.TrimSpace(output) != "" {
		t.Fatalf("JSON stdout on poll error = %q, want empty", output)
	}
}

func TestSheetExportTrimmedJSONSuppressesRetryProgressOnPollErrors(t *testing.T) {
	previousAfter := helperAfter
	helperAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	t.Cleanup(func() { helperAfter = previousAfter })

	queryErr := errors.New("temporary query error")
	caller := &sheetExportRecordingCaller{
		format: " JSON ",
		responses: []sheetExportRecordedResponse{
			{text: `{"jobId":"job-retry"}`},
			{err: queryErr},
		},
	}
	output, err := executeSheetExportAsyncCommand(t, context.Background(), caller, "--node", "node-1")
	if err == nil || !strings.Contains(err.Error(), "taskId=job-retry") {
		t.Fatalf("poll error = %v, want resumable task ID", err)
	}
	if strings.TrimSpace(output) != "" {
		t.Fatalf("trimmed JSON poll-error stdout = %q, want empty", output)
	}
	if len(caller.calls) != 31 || caller.calls[0].tool != "submit_export_job" {
		t.Fatalf("poll-error calls = %d/%#v, want submit plus 30 queries", len(caller.calls), caller.calls)
	}
}

func TestSheetExportGetUsesDocQueryAdapterAndCommandContext(t *testing.T) {
	type contextKey string
	const key contextKey = "sheet-export-get"
	ctx := context.WithValue(context.Background(), key, "command-context")
	caller := &sheetExportRecordingCaller{
		format:    "json",
		responses: []sheetExportRecordedResponse{{text: `{"status":"completed","downloadUrl":"https://example.test/export.xlsx"}`}},
	}

	output, err := executeSheetExportAsyncCommand(t, ctx, caller, "get", "--task-id", "job-1")
	if err != nil {
		t.Fatalf("sheet export get error = %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("MCP calls = %#v, want one query", caller.calls)
	}
	call := caller.calls[0]
	if call.server != "doc" || call.tool != "query_export_job" {
		t.Fatalf("query call = %s/%s, want doc/query_export_job", call.server, call.tool)
	}
	if !reflect.DeepEqual(call.args, map[string]any{"jobId": "job-1"}) {
		t.Fatalf("query args = %#v", call.args)
	}
	if call.ctx.Value(key) != "command-context" {
		t.Fatalf("query context was not cmd.Context(): %#v", call.ctx)
	}

	result := decodeExactlyOneSheetTaskResult(t, output)
	if result.ID != "job-1" || result.Type != "export" || result.Status != asynctask.StatusSuccess || result.ResultURL != "https://example.test/export.xlsx" {
		t.Fatalf("TaskResult = %#v", result)
	}
}

func TestSheetExportGetErrorAndTimeoutPrintResultThenReturnError(t *testing.T) {
	for _, tt := range []struct {
		name       string
		serverJSON string
		wantStatus asynctask.Status
	}{
		{name: "error normalizes to failed", serverJSON: `{"status":"ERROR","message":"conversion failed"}`, wantStatus: asynctask.StatusFailed},
		{name: "lowercase timeout", serverJSON: `{"status":"timeout","message":"query window elapsed"}`, wantStatus: asynctask.StatusTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller := &sheetExportRecordingCaller{
				format:    "json",
				responses: []sheetExportRecordedResponse{{text: tt.serverJSON}},
			}
			output, err := executeSheetExportAsyncCommand(t, context.Background(), caller, "get", "--task-id", "job-1")
			if err == nil {
				t.Fatal("sheet export get error = nil, want nonzero product-command result")
			}
			result := decodeExactlyOneSheetTaskResult(t, output)
			if result.ID != "job-1" || result.Type != "export" || result.Status != tt.wantStatus || result.Message == "" {
				t.Fatalf("TaskResult = %#v", result)
			}
		})
	}
}

func TestSheetExportGetHiddenJobIDAndMutualExclusion(t *testing.T) {
	root := newExportCmd()
	getCmd, remaining, err := root.Find([]string{"get"})
	if err != nil || len(remaining) != 0 || getCmd.Name() != "get" {
		t.Fatalf("find sheet export get = command %q, remaining=%v, err=%v", getCmd.Name(), remaining, err)
	}
	jobIDFlag := getCmd.Flags().Lookup("job-id")
	if jobIDFlag == nil || !jobIDFlag.Hidden {
		t.Fatalf("--job-id = %#v, want hidden compatibility flag", jobIDFlag)
	}

	caller := &sheetExportRecordingCaller{responses: []sheetExportRecordedResponse{{text: `{"status":"PROCESSING"}`}}}
	if _, err := executeSheetExportAsyncCommand(t, context.Background(), caller, "get", "--job-id", "job-1"); err != nil {
		t.Fatalf("sheet export get --job-id error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].args["jobId"] != "job-1" {
		t.Fatalf("compatibility query calls = %#v", caller.calls)
	}

	caller = &sheetExportRecordingCaller{}
	_, err = executeSheetExportAsyncCommand(t, context.Background(), caller, "get", "--task-id", "job-1", "--job-id", "job-2")
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("both ID flags error = %v, want mutual exclusion error", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("MCP calls = %#v, want conflict before MCP", caller.calls)
	}
}

func TestSheetExportGetDirectHandlerSuppliesNonNilContext(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	defer func() {
		deps = previousDeps
		os.Args = previousArgs
	}()
	caller := &sheetExportRecordingCaller{responses: []sheetExportRecordedResponse{{text: `{"status":"PROCESSING"}`}}}
	InitDeps(caller)
	os.Args = []string{"dws", "sheet", "export", "get"}
	root := newExportCmd()
	getCmd, remaining, err := root.Find([]string{"get"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("find get = remaining=%v err=%v", remaining, err)
	}
	if err := getCmd.Flags().Set("task-id", "job-1"); err != nil {
		t.Fatal(err)
	}
	if err := getCmd.RunE(getCmd, nil); err != nil {
		t.Fatalf("direct runSheetExportGet error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].ctx == nil {
		t.Fatalf("direct handler context/calls = %#v, want one call with non-nil context", caller.calls)
	}
}

func TestSheetExportCreateSharesHandlerAndExactPublicFlags(t *testing.T) {
	root := newExportCmd()
	createCmd, remaining, err := root.Find([]string{"create"})
	if err != nil || len(remaining) != 0 || createCmd.Name() != "create" {
		t.Fatalf("find sheet export create = command %q, remaining=%v, err=%v", createCmd.Name(), remaining, err)
	}
	if root.Hidden || createCmd.Hidden || !root.Runnable() || !createCmd.Runnable() || createCmd.HasSubCommands() {
		t.Fatalf("runnable visibility contract: parent(hidden=%v runnable=%v), create(hidden=%v runnable=%v children=%v)", root.Hidden, root.Runnable(), createCmd.Hidden, createCmd.Runnable(), createCmd.HasSubCommands())
	}
	if root.RunE == nil || createCmd.RunE == nil || reflect.ValueOf(root.RunE).Pointer() != reflect.ValueOf(createCmd.RunE).Pointer() {
		t.Fatal("sheet export and sheet export create do not share RunE")
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
	want := []string{"async", "node", "output"}
	if got := publicFlags(root.LocalNonPersistentFlags()); !reflect.DeepEqual(got, want) {
		t.Fatalf("sheet export public flags = %v, want %v", got, want)
	}
	if got := publicFlags(createCmd.LocalNonPersistentFlags()); !reflect.DeepEqual(got, want) {
		t.Fatalf("sheet export create public flags = %v, want %v", got, want)
	}

	caller := &sheetExportRecordingCaller{responses: []sheetExportRecordedResponse{{text: `{"jobId":"job-1"}`}}}
	if _, err := executeSheetExportAsyncCommand(t, context.Background(), caller, "create", "--node", "node-1", "--async"); err != nil {
		t.Fatalf("sheet export create --async error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "submit_export_job" {
		t.Fatalf("create calls = %#v, want submit only", caller.calls)
	}
}

func TestSheetExportSheetHelpAdvertisesCreateAndGetLeaves(t *testing.T) {
	help := newSheetCommand().Long
	for _, fragment := range []string{
		"dws sheet export create",
		"dws sheet export get",
		"--async",
	} {
		if !strings.Contains(help, fragment) {
			t.Errorf("sheet help does not advertise %q", fragment)
		}
	}
}

func TestSheetExportGetDryRunPrintsTaskIDWithoutMCP(t *testing.T) {
	caller := &sheetExportRecordingCaller{dryRun: true}
	output, err := executeSheetExportAsyncCommand(t, context.Background(), caller, "get", "--task-id", "job-1")
	if err != nil {
		t.Fatalf("sheet export get dry-run error = %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run MCP calls = %#v, want zero", caller.calls)
	}
	for _, fragment := range []string{"查询表格导出任务结果", "job-1"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("dry-run output missing %q: %q", fragment, output)
		}
	}
}

func TestSheetExportSyncJSONKeepsHistoricalContractAndDownload(t *testing.T) {
	previousHTTPGet, previousAfter := httpGetFile, helperAfter
	helperAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	t.Cleanup(func() {
		httpGetFile = previousHTTPGet
		helperAfter = previousAfter
	})

	for _, tt := range []struct {
		name       string
		args       []string
		wantOutput string
	}{
		{name: "url only", args: []string{"--node", "node-1"}},
		{name: "download", args: []string{"--node", "node-1", "--output", "out.xlsx"}, wantOutput: "out.xlsx"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			downloadCalls := 0
			httpGetFile = func(_ context.Context, _ string, _ map[string]string, output string) error {
				downloadCalls++
				if output != tt.wantOutput {
					t.Fatalf("download output = %q, want %q", output, tt.wantOutput)
				}
				return nil
			}
			caller := &sheetExportRecordingCaller{
				format: " JSON ",
				responses: []sheetExportRecordedResponse{
					{text: `{"jobId":"job-1"}`},
					{text: `{"status":"SUCCESS","downloadUrl":"https://example.test/export.xlsx"}`},
				},
			}
			output, err := executeSheetExportAsyncCommand(t, context.Background(), caller, tt.args...)
			if err != nil {
				t.Fatalf("sync sheet export error = %v", err)
			}
			var result map[string]any
			decoder := json.NewDecoder(strings.NewReader(output))
			if err := decoder.Decode(&result); err != nil {
				t.Fatalf("sync output decode error = %v; output=%q", err, output)
			}
			if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
				t.Fatalf("sync output contains extra JSON/text: err=%v output=%q", err, output)
			}
			if result["success"] != true || result["jobId"] != "job-1" || result["downloadUrl"] != "https://example.test/export.xlsx" {
				t.Fatalf("sync result = %#v", result)
			}
			if tt.wantOutput == "" {
				if _, exists := result["outputPath"]; exists || downloadCalls != 0 {
					t.Fatalf("url-only result/download = %#v / %d", result, downloadCalls)
				}
			} else if result["outputPath"] != tt.wantOutput || downloadCalls != 1 {
				t.Fatalf("download result/calls = %#v / %d", result, downloadCalls)
			}
		})
	}
}

func TestSheetExportSyncDownloadErrorIncludesSubmittedTaskID(t *testing.T) {
	previousHTTPGet, previousAfter := httpGetFile, helperAfter
	helperAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	downloadErr := errors.New("download interrupted")
	const downloadURL = "https://example.test/export.xlsx?token=temporary"
	httpGetFile = func(_ context.Context, gotURL string, _ map[string]string, output string) error {
		if gotURL != downloadURL || output != "out.xlsx" {
			t.Fatalf("download call = (%q, %q)", gotURL, output)
		}
		return &url.Error{Op: "GET", URL: downloadURL, Err: downloadErr}
	}
	t.Cleanup(func() {
		httpGetFile = previousHTTPGet
		helperAfter = previousAfter
	})

	caller := &sheetExportRecordingCaller{
		format: "json",
		responses: []sheetExportRecordedResponse{
			{text: `{"jobId":"job-download"}`},
			{text: `{"status":"SUCCESS","downloadUrl":"` + downloadURL + `"}`},
		},
	}
	output, err := executeSheetExportAsyncCommand(t, context.Background(), caller, "--node", "node-1", "--output", "out.xlsx")
	if err == nil || !errors.Is(err, downloadErr) {
		t.Fatalf("download error = %v, want wrapped download failure", err)
	}
	if !strings.Contains(err.Error(), "taskId=job-download") {
		t.Fatalf("download error lost submitted task ID: %v", err)
	}
	if strings.Contains(err.Error(), downloadURL) {
		t.Fatalf("download error leaked temporary download URL: %v", err)
	}
	if len(caller.calls) != 2 || caller.calls[0].tool != "submit_export_job" || caller.calls[1].tool != "query_export_job" {
		t.Fatalf("calls = %#v, want submit then query before download failure", caller.calls)
	}
	if strings.TrimSpace(output) != "" {
		t.Fatalf("JSON stdout on download error = %q, want empty", output)
	}
}

func TestSheetExportChangesPreserveSheetImportRawGetAndProgressContracts(t *testing.T) {
	getCaller := &sheetImportCaller{responses: map[string][]string{
		"query_import_task": {`{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-2/","documentType":"1","custom":"kept"}`},
	}}
	output, err := executeSheetImportCommand(t, getCaller, fastSheetImportConfig(), "get", "--task-id", "task-2")
	if err != nil {
		t.Fatalf("sheet import get error = %v", err)
	}
	if len(getCaller.calls) != 1 || getCaller.calls[0].server != "doc" || getCaller.calls[0].tool != "query_import_task" {
		t.Fatalf("sheet import get calls = %#v", getCaller.calls)
	}
	if !strings.Contains(output, `"nodeId": "node-2"`) || !strings.Contains(output, `"custom": "kept"`) || strings.Contains(output, `"resultUrl"`) {
		t.Fatalf("sheet import get raw contract changed: %q", output)
	}

	progressCaller := &sheetImportCaller{responses: map[string][]string{
		"create_import_session": {`{"sessionId":"session-1","uploadUrl":"https://upload.example.test/object"}`},
		"confirm_import":        {`{"taskId":"task-1"}`},
		"query_import_task":     {`{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-1"}`},
	}}
	SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
	output, err = executeSheetImportCommand(t, progressCaller, fastSheetImportConfig(),
		"--file", writeImportFixture(t, "xlsx"), "--workspace", "workspace-1")
	if err != nil {
		t.Fatalf("sheet import error = %v", err)
	}
	for _, fragment := range []string{"[1/4] 创建导入会话", "[4/4] 等待格式转换完成", "导入完成", `"nodeId": "node-1"`} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("sheet import output missing %q: %q", fragment, output)
		}
	}
}
