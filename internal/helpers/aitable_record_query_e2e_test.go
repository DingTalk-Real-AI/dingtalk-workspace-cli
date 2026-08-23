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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type recordQueryE2EStep struct {
	result *edition.ToolResult
	err    error
}

type recordQueryE2ECaller struct {
	steps []recordQueryE2EStep
	calls []aitableTestCall
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
func (*recordQueryE2ECaller) DryRun() bool   { return false }
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
	err := command.Execute()
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
	if err := command.Execute(); err != nil {
		t.Fatalf("record query help failed: %v", err)
	}
	for _, want := range []string{"返回非零结构化错误", "不完整结果", "错误详情保留已取记录和续传 cursor"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("record query help missing %q:\n%s", want, out.String())
		}
	}
}
