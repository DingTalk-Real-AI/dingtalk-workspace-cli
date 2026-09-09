// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type chartAuditReasonCaller struct {
	aitableTestCaller
	reasons []string
}

func (c *chartAuditReasonCaller) CallTool(ctx context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.reasons = append(c.reasons, audit.LocalReason(ctx))
	return c.aitableTestCaller.CallTool(ctx, server, tool, args)
}

func TestCrossPlatformCoverageAitableChartDeleteKeepsReasonOutOfMCP(t *testing.T) {
	testseam.Swap(t, &deps, deps)
	caller := &chartAuditReasonCaller{}
	for _, reason := range []string{"清理重复图表", ""} {
		args := []string{"chart", "delete", "--base-id", "b", "--dashboard-id", "d", "--chart-id", "c", "--yes"}
		if reason != "" {
			args = append(args, "--reason", reason)
		}
		if err := runAitableCoverageCommand(t, caller, args...); err != nil {
			t.Fatal(err)
		}
	}
	if len(caller.calls) != 2 || len(caller.reasons) != 2 || caller.reasons[0] != "清理重复图表" || caller.reasons[1] != "" {
		t.Fatalf("calls=%v local reasons=%v", caller.calls, caller.reasons)
	}
	for _, call := range caller.calls {
		if call.tool != "delete_chart" || len(call.args) != 4 || call.args["confirm"] != true {
			t.Fatalf("unexpected MCP request: %+v", call)
		}
		if _, exists := call.args["reason"]; exists {
			t.Fatalf("local reason leaked to MCP: %v", call.args)
		}
		if _, exists := call.args["_local"]; exists {
			t.Fatalf("local audit metadata leaked to MCP: %v", call.args)
		}
	}
}

func TestCrossPlatformCoverageAitableSnapshotDatasourceContract(t *testing.T) {
	for _, extra := range [][]string{{"--auto"}, {"--auto=false"}, {"--auto-sync-setting", `{}`}} {
		t.Run(strings.Join(extra, " "), func(t *testing.T) {
			args := append([]string{"update", "--base-id", "b", "--table-id", "t"}, extra...)
			caller, err := runAitableDatasourceCommand(t, args...)
			if err == nil || !strings.Contains(err.Error(), "source-config") || len(caller.calls) != 0 {
				t.Fatalf("missing sourceConfig: err=%v calls=%v", err, caller.calls)
			}
		})
	}
	for _, command := range []string{"create", "update"} {
		t.Run(command+" rejects selective sync", func(t *testing.T) {
			args := []string{command, "--base-id", "b", "--source-config", `{}`, "--field-ids", "f1"}
			if command == "create" {
				args = append(args, "--datasource-type", "OA")
			} else {
				args = append(args, "--table-id", "t")
			}
			caller, err := runAitableDatasourceCommand(t, args...)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || !strings.Contains(err.Error(), "全量同步") || len(caller.calls) != 0 {
				t.Fatalf("selective sync: err=%v calls=%v", err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableExplicitRetryableFalseStopsRead(t *testing.T) {
	for _, err := range []error{
		errors.New(`MCP tool failed: {"success":true,"error":{"retryable":false}}`),
		errors.New(`{"error":{"type":"SYSTEM_ERROR","message":"timeout","retryable":false}}`),
		errors.New(`{"content":[{"type":"text","text":"{\"error\":{\"retryable\":false}}"}],"success":true}`),
		fmt.Errorf("wrapped: %w", apperrors.NewAPI("SYSTEM_ERROR", apperrors.WithRetryable(false))),
	} {
		calls := 0
		_, got := callAitableReadWithRetry(context.Background(), "get_base", func(context.Context) (int, error) { calls++; return 0, err })
		if got == nil || calls != 1 {
			t.Fatalf("error=%v calls=%d, want one call", got, calls)
		}
	}
	if !isAitableRetryableError(errors.New(`{"error":{"retryable":true}}`)) {
		t.Fatal("explicit true should allow read retry")
	}
	if isAitableRetryableError(errors.New(`{"error":{"message":"retryable is false"},"success":true}`)) {
		t.Fatal("unrelated true is not a retry instruction")
	}
}
