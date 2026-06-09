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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

type devdocCommandRunner struct {
	last executor.Invocation
}

func (r *devdocCommandRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	return executor.Result{Invocation: invocation}, nil
}

func TestDevdocArticleSearchAcceptsWukongKeywordAlias(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"article", "search", "--keyword", "openConversationId", "--page", "2", "--size", "5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "search_open_platform_docs_rag" {
		t.Fatalf("tool = %q, want search_open_platform_docs_rag", runner.last.Tool)
	}
	if got := runner.last.Params["keyword"]; got != "openConversationId" {
		t.Fatalf("keyword = %#v, want openConversationId", got)
	}
	if got := runner.last.Params["page"]; got != 2 {
		t.Fatalf("page = %#v, want 2", got)
	}
	if got := runner.last.Params["size"]; got != 5 {
		t.Fatalf("size = %#v, want 5", got)
	}
}

func TestDevdocArticleSearchAcceptsPositionalKeyword(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"article", "search", "MCP"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if got := runner.last.Params["keyword"]; got != "MCP" {
		t.Fatalf("keyword = %#v, want MCP", got)
	}
}

func TestDevdocErrorDiagnosePassesRequestID(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"error", "diagnose", "--request-id", "req-123", "--page", "2", "--size", "5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "search_open_error_code_rag" {
		t.Fatalf("tool = %q, want search_open_error_code_rag", runner.last.Tool)
	}
	if got := runner.last.Params["requestId"]; got != "req-123" {
		t.Fatalf("requestId = %#v, want req-123", got)
	}
	if got := runner.last.Params["page"]; got != 2 {
		t.Fatalf("page = %#v, want 2", got)
	}
	if got := runner.last.Params["size"]; got != 5 {
		t.Fatalf("size = %#v, want 5", got)
	}
}

func TestDevdocErrorDiagnoseMapsTraceIDAlias(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"error", "diagnose", "--trace-id", "trace-abc", "--api", "创建日程"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if got := runner.last.Params["requestId"]; got != "trace-abc" {
		t.Fatalf("requestId = %#v, want trace-abc", got)
	}
	if _, ok := runner.last.Params["traceId"]; ok {
		t.Fatalf("traceId should not be sent, params = %#v", runner.last.Params)
	}
	if _, ok := runner.last.Params["apiName"]; ok {
		t.Fatalf("apiName should not be sent, params = %#v", runner.last.Params)
	}
	if got := runner.last.Params["query"]; got != "创建日程" {
		t.Fatalf("query = %#v, want 创建日程", got)
	}
}

func TestDevdocErrorDiagnosePassesErrorContext(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"error", "troubleshoot",
		"--error-code", "33012",
		"--error-message", "missing scope",
		"--context", "create calendar failed",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if got := runner.last.Params["errorCode"]; got != "33012" {
		t.Fatalf("errorCode = %#v, want 33012", got)
	}
	if _, ok := runner.last.Params["errorMessage"]; ok {
		t.Fatalf("errorMessage should not be sent, params = %#v", runner.last.Params)
	}
	if _, ok := runner.last.Params["context"]; ok {
		t.Fatalf("context should not be sent, params = %#v", runner.last.Params)
	}
	if got := runner.last.Params["query"]; got != "missing scope create calendar failed" {
		t.Fatalf("query = %#v, want merged error context", got)
	}
}

func TestDevdocErrorDiagnoseMergesAllContextIntoQuery(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"error", "diagnose",
		"--query", "机器人回调失败",
		"--error-message", "missing scope",
		"--api", "创建日程",
		"--context", "应用无权限",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if got := runner.last.Tool; got != "search_open_error_code_rag" {
		t.Fatalf("tool = %q, want search_open_error_code_rag", got)
	}
	if got := runner.last.Params["query"]; got != "机器人回调失败 missing scope 创建日程 应用无权限" {
		t.Fatalf("query = %#v, want merged context", got)
	}
	for _, key := range []string{"apiName", "errorMessage", "context"} {
		if _, ok := runner.last.Params[key]; ok {
			t.Fatalf("%s should not be sent, params = %#v", key, runner.last.Params)
		}
	}
}

func TestDevdocErrorDiagnoseRequiresTroubleshootInput(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"error", "diagnose", "--api", "创建日程"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "one of --query") {
		t.Fatalf("error = %q, want required input hint", err.Error())
	}
	if runner.last.Tool != "" {
		t.Fatalf("tool = %q, want no call", runner.last.Tool)
	}
}
