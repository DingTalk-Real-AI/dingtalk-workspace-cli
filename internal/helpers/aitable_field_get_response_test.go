// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAitableFieldGetRejectsIncompleteSuccess(t *testing.T) {
	testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
		ready := make(chan time.Time, 1)
		ready <- time.Time{}
		return ready
	})

	for _, tc := range []struct {
		name      string
		command   string
		responses []string
		wantCalls int
		wantError bool
		wantCount int
	}{
		{
			name:      "short success then valid fields",
			command:   "get",
			responses: []string{`{"success":true}`, `{"success":true,"data":{"fields":[{"fieldId":"f","type":"text"}]}}`},
			wantCalls: 2,
			wantCount: 1,
		},
		{
			name:      "missing fields after bounded retries",
			command:   "get",
			responses: []string{`{"success":true}`, `{"success":true}`, `{"success":true}`, `{"success":true}`},
			wantCalls: aitableMaxRetries + 1,
			wantError: true,
		},
		{
			name:      "empty fields is valid",
			command:   "get",
			responses: []string{`{"success":true,"data":{"fields":[]}}`},
			wantCalls: 1,
		},
		{
			name:      "field list alias validates too",
			command:   "list",
			responses: []string{`{"success":true,"data":{}}`, `{"success":true,"data":{"fields":[]}}`},
			wantCalls: 2,
		},
		{
			name:      "non-object field is invalid",
			command:   "get",
			responses: []string{`{"success":true,"data":{"fields":[1]}}`, `{"success":true,"data":{"fields":[1]}}`, `{"success":true,"data":{"fields":[1]}}`, `{"success":true,"data":{"fields":[1]}}`},
			wantCalls: aitableMaxRetries + 1,
			wantError: true,
		},
		{
			name:      "invalid JSON is not a successful read",
			command:   "get",
			responses: []string{"{", "{", "{", "{"},
			wantCalls: aitableMaxRetries + 1,
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableTestCaller{responses: tc.responses}
			out := installAitableDeps(t, caller)
			deps.Out.errW = io.Discard
			root := newAitableCommand()
			installExampleGlobalFlags(root)
			root.SilenceErrors, root.SilenceUsage = true, true
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"field", tc.command, "--base-id=b", "--table-id=t", "--format=json"})
			err := corecmd.ExecuteContextForTest(root, context.Background())
			if len(caller.calls) != tc.wantCalls {
				t.Fatalf("get_fields calls=%d, want %d", len(caller.calls), tc.wantCalls)
			}
			for _, call := range caller.calls {
				if call.server != "aitable" || call.tool != "get_fields" {
					t.Fatalf("unexpected call: %#v", call)
				}
			}
			if tc.wantError {
				var apiErr *apperrors.Error
				if !errors.As(err, &apiErr) || apiErr.Reason != "target_invalid_response" || apiErr.Retryable {
					t.Fatalf("error=%v, want terminal target_invalid_response", err)
				}
				if out.Len() != 0 {
					t.Fatalf("incomplete response leaked to stdout: %q", out.String())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Data struct {
					Fields []map[string]any `json:"fields"`
				} `json:"data"`
			}
			if err := json.Unmarshal(out.Bytes(), &payload); err != nil || payload.Data.Fields == nil || len(payload.Data.Fields) != tc.wantCount {
				t.Fatalf("output=%q, decode=%v, want %d fields", out.String(), err, tc.wantCount)
			}
			if strings.Count(out.String(), `"success"`) != 1 {
				t.Fatalf("expected exactly one rendered result: %q", out.String())
			}
		})
	}
}

func TestCrossPlatformCoverageAitableFieldGetPreservesUpstreamError(t *testing.T) {
	caller := &aitableTestCaller{errors: []error{errors.New("permission denied")}}
	out := installAitableDeps(t, caller)
	deps.Out.errW = io.Discard
	root := newAitableCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors, root.SilenceUsage = true, true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"field", "get", "--base-id=b", "--table-id=t", "--format=json"})
	err := corecmd.ExecuteContextForTest(root, context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "permission denied") || len(caller.calls) != 1 || out.Len() != 0 {
		t.Fatalf("error=%v calls=%d stdout=%q", err, len(caller.calls), out.String())
	}
}

func TestCrossPlatformCoverageAitableFieldGetDryRunKeepsPreview(t *testing.T) {
	caller := &aitableTestCaller{dryRun: true}
	out := installAitableDeps(t, caller)
	deps.Out.errW = io.Discard
	root := newAitableCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors, root.SilenceUsage = true, true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"field", "get", "--base-id=b", "--table-id=t", "--format=json"})
	if err := corecmd.ExecuteContextForTest(root, context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 0 || !strings.Contains(out.String(), `"executed": false`) {
		t.Fatalf("dry-run calls=%d output=%q", len(caller.calls), out.String())
	}
}
