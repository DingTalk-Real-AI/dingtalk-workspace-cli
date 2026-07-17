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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type docAsyncRecordedCall struct {
	ctx    context.Context
	server string
	tool   string
	args   map[string]any
}

type docAsyncRecordedResponse struct {
	text string
	err  error
}

type docAsyncRecordingCaller struct {
	format    string
	responses []docAsyncRecordedResponse
	calls     []docAsyncRecordedCall
}

func (c *docAsyncRecordingCaller) CallTool(ctx context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	clonedArgs := make(map[string]any, len(args))
	for key, value := range args {
		clonedArgs[key] = value
	}
	c.calls = append(c.calls, docAsyncRecordedCall{ctx: ctx, server: server, tool: tool, args: clonedArgs})

	index := len(c.calls) - 1
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

func (c *docAsyncRecordingCaller) Format() string { return c.format }
func (*docAsyncRecordingCaller) DryRun() bool     { return false }
func (*docAsyncRecordingCaller) Fields() string   { return "" }
func (*docAsyncRecordingCaller) JQ() string       { return "" }

func runDocAsyncCoverageCommand(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	previousDeps, previousArgs := deps, os.Args
	defer func() {
		deps = previousDeps
		os.Args = previousArgs
	}()
	os.Args = []string{"dws", "doc"}
	return runDocCoverageCommand(t, caller, args...)
}

func runDocAsyncCapturedCommand(t *testing.T, ctx context.Context, caller edition.ToolCaller, args ...string) (string, error) {
	t.Helper()
	previousDeps, previousArgs := deps, os.Args
	defer func() {
		deps = previousDeps
		os.Args = previousArgs
	}()
	os.Args = []string{"dws", "doc"}

	InitDeps(caller)
	var stdout, stderr bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = &stderr
	root := newDocCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	return stdout.String(), err
}

func TestDocExportAsyncSubmitReturnsPendingWithoutPollingOrDownload(t *testing.T) {
	previousHTTPGet := httpGetFile
	downloadCalls := 0
	httpGetFile = func(context.Context, string, map[string]string, string) error {
		downloadCalls++
		return nil
	}
	t.Cleanup(func() { httpGetFile = previousHTTPGet })

	caller := &docAsyncRecordingCaller{
		responses: []docAsyncRecordedResponse{{text: `{"jobId":"job-1"}`}},
	}
	if err := runDocAsyncCoverageCommand(t, caller, "export", "--node", "node-1", "--async"); err != nil {
		t.Fatalf("doc export --async error = %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("MCP calls = %d, want one submit call: %#v", len(caller.calls), caller.calls)
	}
	call := caller.calls[0]
	if call.server != "doc" || call.tool != "submit_export_job" {
		t.Fatalf("submit call = %s/%s, want doc/submit_export_job", call.server, call.tool)
	}
	if got := call.args["nodeId"]; got != "node-1" {
		t.Fatalf("submit nodeId = %#v, want node-1", got)
	}
	if got := call.args["exportFormat"]; got != "docx" {
		t.Fatalf("submit exportFormat = %#v, want docx", got)
	}
	if downloadCalls != 0 {
		t.Fatalf("download calls = %d, want zero", downloadCalls)
	}
}

func TestDocExportSyncStillRequiresOutput(t *testing.T) {
	caller := &docAsyncRecordingCaller{}
	err := runDocAsyncCoverageCommand(t, caller, "export", "--node", "node-1")
	if err == nil || !strings.Contains(err.Error(), "flag --output is required") {
		t.Fatalf("doc export error = %v, want missing --output", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("MCP calls = %d, want validation before MCP", len(caller.calls))
	}
}

func TestDocExportGetTaskIDUsesSharedQueryAdapter(t *testing.T) {
	type contextKey string
	const key contextKey = "doc-export-get-test"
	ctx := context.WithValue(context.Background(), key, "context-value")
	caller := &docAsyncRecordingCaller{
		format:    "json",
		responses: []docAsyncRecordedResponse{{text: `{"status":"SUCCESS","downloadUrl":"https://example.test/export.docx"}`}},
	}
	stdout, err := runDocAsyncCapturedCommand(t, ctx, caller, "export", "get", "--task-id", "job-1")
	if err != nil {
		t.Fatalf("doc export get --task-id error = %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("MCP calls = %d, want one query call", len(caller.calls))
	}
	call := caller.calls[0]
	if call.server != "doc" || call.tool != "query_export_job" {
		t.Fatalf("query call = %s/%s, want doc/query_export_job", call.server, call.tool)
	}
	if got := call.args["jobId"]; got != "job-1" {
		t.Fatalf("query jobId = %#v, want job-1", got)
	}
	if call.ctx.Value(key) != "context-value" {
		t.Fatalf("query context was not cmd.Context(): %#v", call.ctx)
	}
	decoder := json.NewDecoder(strings.NewReader(stdout))
	var result asynctask.TaskResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("query stdout is not a TaskResult: %v; stdout=%q", err, stdout)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("query stdout contains data after TaskResult: err=%v stdout=%q", err, stdout)
	}
	if result.ID != "job-1" || result.Type != "export" || result.Status != asynctask.StatusSuccess || result.ResultURL != "https://example.test/export.docx" {
		t.Fatalf("query TaskResult = %#v", result)
	}
}

func TestDocExportGetKeepsPublicJobIDCompatibility(t *testing.T) {
	root := newDocCommand()
	getCmd, remaining, err := root.Find([]string{"export", "get"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("find doc export get = (%v, %v)", remaining, err)
	}
	jobIDFlag := getCmd.Flags().Lookup("job-id")
	if jobIDFlag == nil || jobIDFlag.Hidden || jobIDFlag.Deprecated != "" {
		t.Fatalf("--job-id flag = %#v, want visible non-deprecated compatibility flag", jobIDFlag)
	}

	caller := &docAsyncRecordingCaller{
		responses: []docAsyncRecordedResponse{{text: `{"status":"PROCESSING"}`}},
	}
	if err := runDocAsyncCoverageCommand(t, caller, "export", "get", "--job-id", "job-1"); err != nil {
		t.Fatalf("doc export get --job-id error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].args["jobId"] != "job-1" {
		t.Fatalf("compatibility query calls = %#v, want jobId=job-1", caller.calls)
	}
}

func TestDocTaskGetHelpDocumentsUnifiedTaskResultStatuses(t *testing.T) {
	root := newDocCommand()
	for _, path := range [][]string{{"export", "get"}, {"import", "get"}} {
		cmd, remaining, err := root.Find(path)
		if err != nil || len(remaining) != 0 {
			t.Fatalf("find doc %s = (%v, %v)", strings.Join(path, " "), remaining, err)
		}
		for _, status := range []string{"PENDING", "PROCESSING", "SUCCESS", "FAILED", "TIMEOUT"} {
			if !strings.Contains(cmd.Long, status) {
				t.Errorf("doc %s help does not document %s: %q", strings.Join(path, " "), status, cmd.Long)
			}
		}
		if !strings.Contains(cmd.Long, "resultUrl") {
			t.Errorf("doc %s help does not document TaskResult.resultUrl: %q", strings.Join(path, " "), cmd.Long)
		}
	}
}

func TestDocExportGetRejectsBothIDFlagsBeforeMCP(t *testing.T) {
	caller := &docAsyncRecordingCaller{}
	err := runDocAsyncCoverageCommand(t, caller, "export", "get", "--task-id", "job-1", "--job-id", "job-2")
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("both ID flags error = %v, want mutual exclusion error", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("MCP calls = %d, want validation before MCP", len(caller.calls))
	}
}

func TestDocExportAsyncJSONIsExactlyOneTaskResultAndUsesCommandContext(t *testing.T) {
	type contextKey string
	const key contextKey = "doc-export-test"
	ctx := context.WithValue(context.Background(), key, "context-value")
	caller := &docAsyncRecordingCaller{
		format:    "json",
		responses: []docAsyncRecordedResponse{{text: `{"result":{"jobId":"job-1"}}`}},
	}

	stdout, err := runDocAsyncCapturedCommand(t, ctx, caller, "export", "--node", "node-1", "--async")
	if err != nil {
		t.Fatalf("doc export --async --format json error = %v", err)
	}
	decoder := json.NewDecoder(strings.NewReader(stdout))
	var result asynctask.TaskResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("stdout is not one TaskResult JSON object: %v\nstdout=%q", err, stdout)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout contains data after TaskResult: err=%v stdout=%q", err, stdout)
	}
	want := asynctask.TaskResult{
		ID:      "job-1",
		Type:    "export",
		Status:  asynctask.StatusPending,
		Message: "任务已提交，请稍后查询",
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("TaskResult = %#v, want %#v", result, want)
	}
	if len(caller.calls) != 1 || caller.calls[0].ctx.Value(key) != "context-value" {
		t.Fatalf("submit context was not cmd.Context(): %#v", caller.calls)
	}
}

func TestDocExportGetFailedAndTimeoutPrintStructuredResultThenError(t *testing.T) {
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
					text: `{"status":"` + string(test.status) + `","message":"task message"}`,
				}},
			}
			stdout, err := runDocAsyncCapturedCommand(t, context.Background(), caller, "export", "get", "--task-id", "job-1")
			if err == nil {
				t.Fatal("doc export get error = nil, want nonzero error")
			}
			var result asynctask.TaskResult
			if decodeErr := json.Unmarshal([]byte(stdout), &result); decodeErr != nil {
				t.Fatalf("structured stdout decode error = %v; stdout=%q", decodeErr, stdout)
			}
			if result.ID != "job-1" || result.Type != "export" || result.Status != test.status || result.Message != "task message" {
				t.Fatalf("structured result = %#v", result)
			}
		})
	}
}

func TestDocExportCreateSharesHandlerAndPublicFlags(t *testing.T) {
	root := newDocCommand()
	exportCmd, remaining, err := root.Find([]string{"export"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("find doc export = (%v, %v)", remaining, err)
	}
	createCmd, remaining, err := root.Find([]string{"export", "create"})
	if err != nil || len(remaining) != 0 || createCmd.Name() != "create" {
		t.Fatalf("find doc export create = command %q, remaining=%v, err=%v", createCmd.Name(), remaining, err)
	}
	if createCmd.Hidden {
		t.Fatal("doc export create is hidden")
	}
	if !createCmd.Runnable() {
		t.Fatal("doc export create is not runnable")
	}
	if createCmd.HasSubCommands() {
		t.Fatalf("doc export create has children: %v", createCmd.Commands())
	}
	if exportCmd.RunE == nil || createCmd.RunE == nil || reflect.ValueOf(exportCmd.RunE).Pointer() != reflect.ValueOf(createCmd.RunE).Pointer() {
		t.Fatal("doc export and doc export create do not share the same RunE handler")
	}

	publicFlags := func(commandName string, flags *pflag.FlagSet) []string {
		t.Helper()
		var names []string
		flags.VisitAll(func(flag *pflag.Flag) {
			if !flag.Hidden {
				names = append(names, flag.Name)
			}
		})
		sort.Strings(names)
		return names
	}
	parentFlags := publicFlags("doc export", exportCmd.LocalNonPersistentFlags())
	childFlags := publicFlags("doc export create", createCmd.LocalNonPersistentFlags())
	wantFlags := []string{"async", "export-format", "node", "output"}
	if !reflect.DeepEqual(parentFlags, wantFlags) {
		t.Fatalf("doc export public flags = %v, want %v", parentFlags, wantFlags)
	}
	if !reflect.DeepEqual(childFlags, parentFlags) {
		t.Fatalf("doc export create public flags = %v, want %v", childFlags, parentFlags)
	}
	for _, command := range []*cobra.Command{exportCmd, createCmd} {
		asyncFlag := command.Flags().Lookup("async")
		if asyncFlag == nil || asyncFlag.DefValue != "false" {
			t.Fatalf("%s --async = %#v, want default false", command.CommandPath(), asyncFlag)
		}
		for _, name := range []string{"format", "url", "id", "node-id", "doc-id", "file-id"} {
			flag := command.Flags().Lookup(name)
			if flag == nil || !flag.Hidden {
				t.Fatalf("%s --%s = %#v, want hidden compatibility flag", command.CommandPath(), name, flag)
			}
		}
	}
}

func TestDocExportCreateAsyncDispatchesSharedHandler(t *testing.T) {
	caller := &docAsyncRecordingCaller{
		responses: []docAsyncRecordedResponse{{text: `{"jobId":"job-1"}`}},
	}
	if err := runDocAsyncCoverageCommand(t, caller, "export", "create", "--node", "node-1", "--async"); err != nil {
		t.Fatalf("doc export create --async error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "submit_export_job" {
		t.Fatalf("doc export create calls = %#v, want one submit and no poll", caller.calls)
	}
}

func TestDocExportSyncJSONKeepsHistoricalProgressOutput(t *testing.T) {
	previousHTTPGet, previousAfter := httpGetFile, helperAfter
	httpGetFile = func(context.Context, string, map[string]string, string) error { return nil }
	helperAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	t.Cleanup(func() {
		httpGetFile = previousHTTPGet
		helperAfter = previousAfter
	})
	caller := &docAsyncRecordingCaller{
		format: "json",
		responses: []docAsyncRecordedResponse{
			{text: `{"jobId":"job-1"}`},
			{text: `{"status":"SUCCESS","downloadUrl":"https://example.test/export.docx"}`},
		},
	}
	stdout, err := runDocAsyncCapturedCommand(t, context.Background(), caller, "export", "--node", "node-1", "--output", "out.docx")
	if err != nil {
		t.Fatalf("sync doc export error = %v", err)
	}
	for _, progress := range []string{"[1/3] 提交导出任务", "任务已提交，jobId: job-1", "[2/3] 等待导出完成", "[3/3] 下载文件到 out.docx", "导出完成: out.docx"} {
		if !strings.Contains(stdout, progress) {
			t.Errorf("sync stdout missing %q: %q", progress, stdout)
		}
	}
}
