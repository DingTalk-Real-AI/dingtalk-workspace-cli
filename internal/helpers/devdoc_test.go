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
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

type devdocCommandRunner struct {
	last      executor.Invocation
	calls     []executor.Invocation
	errors    map[string]error
	responses map[string]map[string]any
}

func (r *devdocCommandRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	r.calls = append(r.calls, invocation)
	if r.errors != nil {
		if err := r.errors[invocation.Tool]; err != nil {
			return executor.Result{}, err
		}
	}
	if r.responses != nil {
		if content, ok := r.responses[invocation.Tool]; ok {
			invocation.Implemented = true
			return executor.Result{
				Invocation: invocation,
				Response:   map[string]any{"content": content},
			}, nil
		}
	}
	return executor.Result{Invocation: invocation}, nil
}

func TestDevdocArticleSearchAcceptsWukongKeywordAlias(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"article", "search", "--keyword", "openConversationId", "--cursor", "tok-d", "--page-size", "5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != devdocArticleSearchTool {
		t.Fatalf("tool = %q, want %s", runner.last.Tool, devdocArticleSearchTool)
	}
	if got := runner.last.Params["keyword"]; got != "openConversationId" {
		t.Fatalf("keyword = %#v, want openConversationId", got)
	}
	if _, ok := runner.last.Params["query"]; ok {
		t.Fatalf("query must not be sent to article RAG search: %#v", runner.last.Params)
	}
	if got := runner.last.Params["cursor"]; got != "tok-d" {
		t.Fatalf("cursor = %#v, want tok-d", got)
	}
	if got := runner.last.Params["size"]; got != 5 {
		t.Fatalf("size = %#v, want 5", got)
	}
	if _, ok := runner.last.Params["pageSize"]; ok {
		t.Fatalf("pageSize must not be sent to RAG search: %#v", runner.last.Params)
	}
	if _, ok := runner.last.Params["CliRagSearchReqVO"]; ok {
		t.Fatalf("CliRagSearchReqVO wrapper must not be sent to RAG search: %#v", runner.last.Params)
	}
}

func TestDevdocArticleSearchPassesCursor(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"article", "search", "--query", "Webhook", "--cursor", "3", "--size", "5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != devdocArticleSearchTool {
		t.Fatalf("tool = %q, want %s", runner.last.Tool, devdocArticleSearchTool)
	}
	if got := runner.last.Params["cursor"]; got != "3" {
		t.Fatalf("cursor = %#v, want 3", got)
	}
	if _, ok := runner.last.Params["page"]; ok {
		t.Fatalf("page must be omitted when cursor is set: %#v", runner.last.Params)
	}
}

func TestDevdocErrorDiagnoseBuildsErrorSearchQuery(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"error", "diagnose",
		"--error-code", "40014",
		"--error-message", "不合法的access_token",
		"--api", "获取用户信息",
		"--cursor", "2",
		"--page", "2",
		"--size", "3",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != devdocErrorDiagnoseTool {
		t.Fatalf("tool = %q, want %s", runner.last.Tool, devdocErrorDiagnoseTool)
	}
	if got := runner.last.Params["errorCode"]; got != "40014" {
		t.Fatalf("errorCode = %#v, want 40014", got)
	}
	if got := runner.last.Params["query"]; got != "不合法的access_token 获取用户信息" {
		t.Fatalf("query = %#v, want merged message and api", got)
	}
	if _, ok := runner.last.Params["page"]; ok {
		t.Fatalf("page must be omitted when cursor is set: %#v", runner.last.Params)
	}
	if got := runner.last.Params["cursor"]; got != "2" {
		t.Fatalf("cursor = %#v, want 2", got)
	}
	if got := runner.last.Params["size"]; got != 3 {
		t.Fatalf("size = %#v, want 3", got)
	}
}

func TestDevdocErrorDiagnoseFallsBackToArticleSearchTools(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{
		errors: map[string]error{
			devdocErrorDiagnoseTool: errors.New("PARAM_ERROR - 未找到指定工具"),
			devdocArticleSearchTool: errors.New("unknown tool"),
		},
	}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"error", "diagnose",
		"--error-code", "40014",
		"--query", "获取用户信息",
		"--request-id", "req-1",
		"--cursor", "2",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if len(runner.calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(runner.calls))
	}
	if got := runner.calls[0].Tool; got != devdocErrorDiagnoseTool {
		t.Fatalf("first tool = %q, want %s", got, devdocErrorDiagnoseTool)
	}
	if got := runner.calls[1].Tool; got != devdocArticleSearchTool {
		t.Fatalf("second tool = %q, want %s", got, devdocArticleSearchTool)
	}
	if got := runner.calls[1].Params["cursor"]; got != "2" {
		t.Fatalf("RAG fallback cursor = %#v, want 2", got)
	}
	if got := runner.calls[1].Params["keyword"]; got != "40014 获取用户信息 req-1" {
		t.Fatalf("RAG fallback keyword = %#v, want merged diagnostic query", got)
	}
	if _, ok := runner.calls[1].Params["query"]; ok {
		t.Fatalf("RAG fallback query must not be sent: %#v", runner.calls[1].Params)
	}
	if _, ok := runner.calls[1].Params["page"]; ok {
		t.Fatalf("RAG fallback page must be omitted when cursor is set: %#v", runner.calls[1].Params)
	}
	if got := runner.calls[2].Tool; got != devdocArticleSearchLegacyTool {
		t.Fatalf("third tool = %q, want %s", got, devdocArticleSearchLegacyTool)
	}
	if got := runner.last.Params["keyword"]; got != "40014 获取用户信息 req-1" {
		t.Fatalf("fallback keyword = %#v, want merged diagnostic keyword", got)
	}
	if _, ok := runner.last.Params["cursor"]; ok {
		t.Fatalf("legacy fallback params must not include cursor: %#v", runner.last.Params)
	}
	if got := runner.last.Params["page"]; got != 2 {
		t.Fatalf("legacy fallback page = %#v, want cursor-derived page 2", got)
	}
}

func TestDevdocArticleSearchFallsBackOnStructuredToolMetadataError(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{
		errors: map[string]error{
			devdocArticleSearchTool: apperrors.NewAPI("business error: success=false",
				apperrors.WithReason("business_error"),
				apperrors.WithServerDiag(apperrors.ServerDiagnostics{
					ServerErrorCode: "PARAM_ERROR",
					TechnicalDetail: "Tool metadata API error: PARAM_ERROR - 未找到指定工具",
				}),
			),
		},
	}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"article", "search", "--query", "Webhook", "--cursor", "3"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(runner.calls))
	}
	if got := runner.calls[0].Tool; got != devdocArticleSearchTool {
		t.Fatalf("first tool = %q, want %s", got, devdocArticleSearchTool)
	}
	if got := runner.calls[1].Tool; got != devdocArticleSearchLegacyTool {
		t.Fatalf("second tool = %q, want %s", got, devdocArticleSearchLegacyTool)
	}
	if got := runner.calls[1].Params["page"]; got != 3 {
		t.Fatalf("legacy fallback page = %#v, want cursor-derived page 3", got)
	}
}

func TestDevdocArticleSearchDoesNotFallbackOnGenericToolMetadataError(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{
		errors: map[string]error{
			devdocArticleSearchTool: apperrors.NewAPI("business error: success=false",
				apperrors.WithReason("business_error"),
				apperrors.WithServerDiag(apperrors.ServerDiagnostics{
					ServerErrorCode: "PARAM_ERROR",
					TechnicalDetail: "Tool metadata API error: PARAM_ERROR - 参数不能为空",
				}),
			),
		},
	}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"article", "search", "--query", "Webhook"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("Execute() error = nil, want metadata PARAM_ERROR")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	if got := runner.calls[0].Tool; got != devdocArticleSearchTool {
		t.Fatalf("first tool = %q, want %s", got, devdocArticleSearchTool)
	}
}

func TestDevdocArticleSearchFallsBackOnEmptyRAGContent(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{
		responses: map[string]map[string]any{
			devdocArticleSearchTool: {
				"materials":  []any{},
				"references": []any{},
				"ragContext": nil,
			},
			devdocArticleSearchLegacyTool: {
				"success": true,
				"result": map[string]any{
					"items": []any{map[string]any{"title": "OAuth2.0鉴权 - 开放平台"}},
				},
			},
		},
	}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"article", "search", "--keyword", "OAuth2", "--size", "2"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(runner.calls))
	}
	if got := runner.calls[0].Tool; got != devdocArticleSearchTool {
		t.Fatalf("first tool = %q, want %s", got, devdocArticleSearchTool)
	}
	if got := runner.calls[1].Tool; got != devdocArticleSearchLegacyTool {
		t.Fatalf("second tool = %q, want %s", got, devdocArticleSearchLegacyTool)
	}
}

func TestDevdocArticleSearchFallsBackOnPaginationOnlyRAGContent(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{
		responses: map[string]map[string]any{
			devdocArticleSearchTool: {
				"success": true,
				"result":  map[string]any{"nextCursor": "2"},
			},
			devdocArticleSearchLegacyTool: {
				"success": true,
				"result": map[string]any{
					"items": []any{map[string]any{"title": "获取用户token - 开放平台"}},
				},
			},
		},
	}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"article", "search", "--query", "OAuth2", "--cursor", "3"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(runner.calls))
	}
	if got := runner.calls[1].Tool; got != devdocArticleSearchLegacyTool {
		t.Fatalf("second tool = %q, want %s", got, devdocArticleSearchLegacyTool)
	}
	if got := runner.calls[1].Params["page"]; got != 3 {
		t.Fatalf("legacy fallback page = %#v, want cursor-derived page 3", got)
	}
}

func TestDevdocArticleSearchKeepsNonEmptyRAGContent(t *testing.T) {
	t.Parallel()

	runner := &devdocCommandRunner{
		responses: map[string]map[string]any{
			devdocArticleSearchTool: {
				"materials": []any{map[string]any{"title": "OAuth2.0鉴权 - 开放平台"}},
			},
		},
	}
	cmd := devdocHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"article", "search", "--keyword", "OAuth2"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	if got := runner.calls[0].Tool; got != devdocArticleSearchTool {
		t.Fatalf("first tool = %q, want %s", got, devdocArticleSearchTool)
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
	if _, ok := runner.last.Params["query"]; ok {
		t.Fatalf("query must not be sent to article RAG search: %#v", runner.last.Params)
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
