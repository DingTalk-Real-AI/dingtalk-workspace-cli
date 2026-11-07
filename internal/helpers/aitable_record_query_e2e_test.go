// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type recordQueryE2EStep struct {
	result *edition.ToolResult
	err    error
}

type recordQueryE2ECaller struct {
	steps  []recordQueryE2EStep
	calls  []aitableTestCall
	dryRun bool
}

func (c *recordQueryE2ECaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	c.calls = append(c.calls, aitableTestCall{server: server, tool: tool, args: cloned})
	index := len(c.calls) - 1
	if index >= len(c.steps) {
		return nil, fmt.Errorf("unexpected tool call %d", index+1)
	}
	return c.steps[index].result, c.steps[index].err
}

func (*recordQueryE2ECaller) Format() string { return "json" }
func (c *recordQueryE2ECaller) DryRun() bool { return c.dryRun }
func (*recordQueryE2ECaller) Fields() string { return "" }
func (*recordQueryE2ECaller) JQ() string     { return "" }

func runRecordQueryCLI(t *testing.T, caller *recordQueryE2ECaller, extraArgs ...string) (string, error) {
	t.Helper()
	testseam.Protect(t, &deps)
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })

	InitDeps(caller)
	out := &bytes.Buffer{}
	deps.Out.w = out
	deps.Out.errW = out
	args := []string{"record", "query", "--base-id", "base-e2e", "--table-id", "table-e2e", "--all"}
	args = append(args, extraArgs...)
	os.Args = append([]string{"dws", "aitable"}, args...)

	command := newAitableCommand()
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetArgs(args)
	err := corecmd.ExecuteForTest(command)
	return out.String(), err
}

func recordQueryTextStep(text string) recordQueryE2EStep {
	return recordQueryE2EStep{result: textToolResult(text)}
}

func TestCrossPlatformCoverageRecordQueryCLICompleteE2E(t *testing.T) {
	caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{
		recordQueryTextStep(`{"data":{"records":[{"id":"r1"},{"id":"r2"}],"nextCursor":"cursor-2","totalCount":3}}`),
		recordQueryTextStep(`{"data":{"records":[{"id":"r3"}],"nextCursor":"","totalCount":3}}`),
	}}
	out, err := runRecordQueryCLI(t, caller, "--page-limit", "0")
	if err != nil {
		t.Fatalf("record query CLI returned error: %v", err)
	}
	for _, want := range []string{`"complete": true`, `"fetchedCount": 3`, `"totalCount": 3`, `"id": "r3"`} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Fatalf("record query CLI output missing %s:\n%s", want, out)
		}
	}
	if len(caller.calls) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(caller.calls))
	}
	if got := caller.calls[0].args["cursor"]; got != nil {
		t.Fatalf("first-page cursor = %#v, want absent", got)
	}
	if got := caller.calls[1].args["cursor"]; got != "cursor-2" {
		t.Fatalf("second-page cursor = %#v, want cursor-2", got)
	}
}

func TestCrossPlatformCoverageRecordQueryCLICompletesOnSuccessfulEmptyTrailingPage(t *testing.T) {
	caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{
		recordQueryTextStep(`{"data":{"records":[{"id":"r1"},{"id":"r2"}],"nextCursor":"after-full-page","totalCount":2}}`),
		recordQueryTextStep(`{"data":{"records":[],"nextCursor":"","totalCount":2}}`),
	}}
	out, err := runRecordQueryCLI(t, caller, "--page-limit", "0")
	if err != nil {
		t.Fatalf("record query CLI treated successful empty trailing page as error: %v", err)
	}
	for _, want := range []string{`"complete": true`, `"fetchedCount": 2`, `"pages": 2`, `"totalCount": 2`} {
		if !strings.Contains(out, want) {
			t.Fatalf("record query CLI output missing %s:\n%s", want, out)
		}
	}
	if len(caller.calls) != 2 || caller.calls[1].args["cursor"] != "after-full-page" {
		t.Fatalf("record query CLI calls = %#v, want one continuation request", caller.calls)
	}
}

func TestCrossPlatformCoverageRecordQueryCLIFailsClosedE2E(t *testing.T) {
	tests := []struct {
		name  string
		steps []recordQueryE2EStep
		args  []string
	}{
		{name: "nil result", steps: []recordQueryE2EStep{{result: nil}}},
		{name: "empty content", steps: []recordQueryE2EStep{{result: &edition.ToolResult{}}}},
		{name: "empty text", steps: []recordQueryE2EStep{recordQueryTextStep("")}},
		{name: "invalid json", steps: []recordQueryE2EStep{recordQueryTextStep("{")}},
		{name: "null payload", steps: []recordQueryE2EStep{recordQueryTextStep("null")}},
		{name: "missing records", steps: []recordQueryE2EStep{recordQueryTextStep(`{"data":{"nextCursor":"c"}}`)}},
		{name: "missing records with has more", steps: []recordQueryE2EStep{recordQueryTextStep(`{"data":{"hasMore":true}}`)}},
		{name: "records wrong type", steps: []recordQueryE2EStep{recordQueryTextStep(`{"records":{}}`)}},
		{name: "records null", steps: []recordQueryE2EStep{recordQueryTextStep(`{"records":null}`)}},
		{name: "record item wrong type", steps: []recordQueryE2EStep{recordQueryTextStep(`{"records":["bad"]}`)}},
		{name: "trailing json", steps: []recordQueryE2EStep{recordQueryTextStep(`{"records":[]} {}`)}},
		{name: "has more without cursor", steps: []recordQueryE2EStep{recordQueryTextStep(`{"records":[],"hasMore":true}`)}},
		{name: "invalid total", steps: []recordQueryE2EStep{recordQueryTextStep(`{"records":[],"totalCount":"many"}`)}},
		{name: "first transport error", steps: []recordQueryE2EStep{{err: errors.New("offline")}}},
		{
			name: "second page error",
			steps: []recordQueryE2EStep{
				recordQueryTextStep(`{"records":[{"id":"kept"}],"nextCursor":"retry-me"}`),
				{err: errors.New("upstream reset")},
			},
			args: []string{"--page-limit", "0"},
		},
		{
			name:  "page limit",
			steps: []recordQueryE2EStep{recordQueryTextStep(`{"records":[{"id":"kept"}],"nextCursor":"resume-me"}`)},
			args:  []string{"--page-limit", "1"},
		},
		{
			name: "cursor cycle",
			steps: []recordQueryE2EStep{
				recordQueryTextStep(`{"records":[{"id":"r1"}],"nextCursor":"same"}`),
				recordQueryTextStep(`{"records":[{"id":"r2"}],"nextCursor":"same"}`),
			},
			args: []string{"--page-limit", "0"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &recordQueryE2ECaller{steps: test.steps}
			out, err := runRecordQueryCLI(t, caller, test.args...)
			if err == nil {
				t.Fatalf("CLI treated %s as success; output=%s", test.name, out)
			}
			if out != "" {
				t.Fatalf("CLI emitted success output for %s: %q", test.name, out)
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error type = %T, want *errors.Error", err)
			}
			if typed.Reason == "" || typed.Details["incomplete_result"] == nil {
				t.Fatalf("error lacks recovery metadata: %#v", typed)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordQueryCLIInitialCursorE2E(t *testing.T) {
	caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{
		recordQueryTextStep(`{"records":[]}`),
	}}
	if out, err := runRecordQueryCLI(t, caller, "--cursor", "resume-from-here", "--page-limit", "0"); err != nil || out == "" {
		t.Fatalf("resume query = %q, %v", out, err)
	}
	want := map[string]any{"baseId": "base-e2e", "tableId": "table-e2e", "cursor": "resume-from-here"}
	if !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("resume args = %#v, want %#v", caller.calls[0].args, want)
	}
}

func TestCrossPlatformCoverageRecordQueryCLIPageLimitHelpE2E(t *testing.T) {
	command := newAitableCommand()
	command.SilenceErrors = true
	command.SilenceUsage = true
	out := &bytes.Buffer{}
	command.SetOut(out)
	command.SetErr(out)
	command.SetArgs([]string{"record", "query", "--help"})
	if err := corecmd.ExecuteForTest(command); err != nil {
		t.Fatalf("record query help failed: %v", err)
	}
	for _, want := range []string{"返回非零结构化错误", "不完整结果", "错误详情保留已取记录和续传 cursor"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("record query help missing %q:\n%s", want, out.String())
		}
	}
}

// 失效游标不能继续暴露为恢复点，也不能把旧页作为新一轮查询的数据。
func TestCrossPlatformCoverageRecordQueryCLIDiscardsInvalidPagination(t *testing.T) {
	for _, code := range []string{"INVALID_CURSOR", "CURSOR_SNAPSHOT_CHANGED", "CURSOR_SNAPSHOT_UNAVAILABLE"} {
		for _, transportError := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/transport=%t", code, transportError), func(t *testing.T) {
				failed := recordQueryTextStep(fmt.Sprintf(`{"status":"error","error":{"code":%q,"message":"query failed","retryable":false}}`, code))
				if transportError {
					failed = recordQueryE2EStep{err: fmt.Errorf("wrapped: %w", apperrors.NewAPI("query failed", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: code, TraceID: "trace-query"})))}
				}
				caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{
					recordQueryTextStep(`{"records":[{"id":"old-row"}],"nextCursor":"stale-cursor"}`), failed,
				}}
				out, err := runRecordQueryCLI(t, caller, "--page-limit", "0")
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Retryable || out != "" || len(caller.calls) != 2 {
					t.Fatalf("unsafe recovery: out=%q err=%#v calls=%d", out, err, len(caller.calls))
				}
				if typed.Details["discard_previous_results"] != true || typed.ServerDiag.ServerErrorCode != code {
					t.Fatalf("missing discard policy or code: %#v", typed)
				}
				incomplete, ok := typed.Details["incomplete_result"].(map[string]any)
				if !ok || incomplete["discardedCount"] != 1 || incomplete["cursor"] != nil || incomplete["records"] != nil {
					t.Fatalf("stale result remains resumable: %#v", incomplete)
				}
				if transportError && typed.ServerDiag.TraceID != "trace-query" {
					t.Fatalf("trace was lost: %#v", typed.ServerDiag)
				}
			})
		}
	}
}

// 普通网络错误仍可保留断点，不能把所有分页错误都误判成版本切换。
func TestCrossPlatformCoverageRecordQueryCLINetworkFailureKeepsRecoveryPoint(t *testing.T) {
	caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{
		recordQueryTextStep(`{"records":[{"id":"kept"}],"nextCursor":"resume"}`),
		{err: errors.New("connection reset")},
	}}
	_, err := runRecordQueryCLI(t, caller, "--page-limit", "0")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || !typed.Retryable {
		t.Fatalf("unexpected network error: %#v", err)
	}
	incomplete := typed.Details["incomplete_result"].(map[string]any)
	if incomplete["cursor"] != "resume" || len(incomplete["records"].([]any)) != 1 {
		t.Fatalf("network recovery point lost: %#v", incomplete)
	}
}

// 普通单页入口也必须停在第一次游标失败，不受只读工具通用重试策略影响。
func TestCrossPlatformCoverageRecordQueryCLISinglePageRejectsCursorRetry(t *testing.T) {
	for _, transportError := range []bool{false, true} {
		failed := recordQueryTextStep(`{"status":"error","error":{"code":"INVALID_CURSOR","message":"cursor failed","retryable":true}}`)
		if transportError {
			retryable := true
			failed = recordQueryE2EStep{err: apperrors.NewAPI("query failed", apperrors.WithServerDiag(apperrors.ServerDiagnostics{
				ServerErrorCode: "INVALID_CURSOR", ServerRetryable: &retryable,
			}))}
		}
		caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{failed}}
		out, err := runRecordQueryCLI(t, caller, "--all=false", "--cursor", "legacy")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Retryable || len(caller.calls) != 1 || out != "" {
			t.Fatalf("single-page query retried invalid cursor: out=%q err=%#v calls=%d", out, err, len(caller.calls))
		}
	}
}

// 只有结构化业务码可以触发丢弃策略；用户文本包含同名字符串不构成证据。
func TestCrossPlatformCoverageRecordQueryRecoveryOnlyClassifiesKnownCodes(t *testing.T) {
	for _, err := range []error{nil, errors.New("INVALID_CURSOR"), &CLIError{Message: "CURSOR_SNAPSHOT_CHANGED"},
		apperrors.NewAPI("failed", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "PERMISSION_DENIED"}))} {
		if got := RecordQueryRecoveryError(err); got != err {
			t.Fatalf("unrelated error was replaced: %#v -> %#v", err, got)
		}
	}
}

// offset 超限有独立恢复语义：丢弃累计结果但不能直接从头重查，必须先收窄条件。
func TestCrossPlatformCoverageRecordQueryOffsetLimitRequiresNarrowing(t *testing.T) {
	src := apperrors.NewAPI("query failed", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "CURSOR_OFFSET_LIMIT"}))
	var typed *apperrors.Error
	if !errors.As(RecordQueryRecoveryError(src), &typed) {
		t.Fatalf("offset limit error was not classified: %#v", src)
	}
	if typed.Retryable || typed.Reason != "pagination_offset_limit_exceeded" {
		t.Fatalf("offset limit reason/retryable = %#v", typed)
	}
	if typed.Details["discard_previous_results"] != true ||
		typed.Details["restart_from_first_page"] != false ||
		typed.Details["narrow_filters_required"] != true {
		t.Fatalf("offset limit details = %#v", typed.Details)
	}
	if typed.ServerDiag.ServerErrorCode != "CURSOR_OFFSET_LIMIT" {
		t.Fatalf("offset limit code lost: %#v", typed.ServerDiag)
	}
}

// 快照缺版本信息走独立文案分支，但仍要求从第一页重查。
func TestCrossPlatformCoverageRecordQuerySnapshotUnavailableReason(t *testing.T) {
	src := apperrors.NewAPI("query failed", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "CURSOR_SNAPSHOT_UNAVAILABLE"}))
	var typed *apperrors.Error
	if !errors.As(RecordQueryRecoveryError(src), &typed) {
		t.Fatalf("snapshot-unavailable error was not classified: %#v", src)
	}
	if typed.Reason != "pagination_snapshot_unavailable" || typed.Details["restart_from_first_page"] != true {
		t.Fatalf("snapshot-unavailable semantics = %#v", typed)
	}
}

// aitableServerDiag 需覆盖统一诊断、旧 CLIError.ServerCode 与保留的业务 JSON 三种来源，以及 nil 输入。
func TestCrossPlatformCoverageAitableServerDiagSources(t *testing.T) {
	if code := aitableServerDiag(nil).ServerErrorCode; code != "" {
		t.Fatalf("nil error must yield empty diagnostics, got %q", code)
	}
	typed := apperrors.NewAPI("boom", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "INVALID_CURSOR"}))
	if got := aitableServerDiag(typed).ServerErrorCode; got != "INVALID_CURSOR" {
		t.Fatalf("typed diag = %q", got)
	}
	legacy := &CLIError{Message: "human message", ServerCode: "CURSOR_SNAPSHOT_CHANGED"}
	if got := aitableServerDiag(legacy).ServerErrorCode; got != "CURSOR_SNAPSHOT_CHANGED" {
		t.Fatalf("legacy diag = %q", got)
	}
	businessJSON := errors.New(`{"error":{"code":"CURSOR_OFFSET_LIMIT"}}`)
	if got := aitableServerDiag(businessJSON).ServerErrorCode; got != "CURSOR_OFFSET_LIMIT" {
		t.Fatalf("business JSON diag = %q", got)
	}
}

func TestRecordQueryRecoveryNonResumableErrorCursor(t *testing.T) {
	src := apperrors.NewAPI("query failed", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "NON_RESUMABLE_ERROR_CURSOR"}))
	var typed *apperrors.Error
	if !errors.As(RecordQueryRecoveryError(src), &typed) {
		t.Fatalf("NON_RESUMABLE_ERROR_CURSOR was not classified: %#v", src)
	}
	if typed.Retryable {
		t.Fatal("NON_RESUMABLE_ERROR_CURSOR must not be retryable")
	}
	if typed.Reason != "pagination_non_resumable_error" {
		t.Fatalf("reason = %q", typed.Reason)
	}
	if typed.Details["discard_previous_results"] != true ||
		typed.Details["restart_from_first_page"] != false ||
		typed.Details["stop_pagination"] != true {
		t.Fatalf("details = %#v", typed.Details)
	}
	if typed.ServerDiag.ServerErrorCode != "NON_RESUMABLE_ERROR_CURSOR" {
		t.Fatalf("server code lost: %#v", typed.ServerDiag)
	}
}

func TestNonResumableCursorResponseError(t *testing.T) {
	for _, executionStarted := range []bool{false, true} {
		var typed *apperrors.Error
		if !errors.As(NonResumableCursorResponseError("error-v1:GUARD", executionStarted), &typed) {
			t.Fatalf("guard-cursor response error was not structured (executionStarted=%v)", executionStarted)
		}
		if typed.Retryable {
			t.Fatal("guard-cursor response error must not be retryable")
		}
		if typed.Reason != "pagination_non_resumable_error" {
			t.Fatalf("reason = %q", typed.Reason)
		}
		if typed.Details["discard_previous_results"] != true ||
			typed.Details["stop_pagination"] != true ||
			typed.Details["restart_from_first_page"] != false ||
			typed.Details["guard_cursor"] != "error-v1:GUARD" {
			t.Fatalf("details = %#v", typed.Details)
		}
		if typed.ServerDiag.ServerErrorCode != "NON_RESUMABLE_ERROR_CURSOR" {
			t.Fatalf("server code = %#v", typed.ServerDiag)
		}
		if typed.ExecutionStarted == nil || *typed.ExecutionStarted != executionStarted {
			t.Fatalf("executionStarted = %#v, want %v", typed.ExecutionStarted, executionStarted)
		}
	}
}
