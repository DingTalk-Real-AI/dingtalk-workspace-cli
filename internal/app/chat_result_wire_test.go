// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"encoding/json"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func emitChatResultWireForTest(t *testing.T, result output.CommandResult) (int, map[string]any) {
	t.Helper()
	cmd := &cobra.Command{Use: "chat-result"}
	cmd.Flags().String("format", "json", "")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	code, err := output.EmitResult(cmd, result)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode result wire: %v\n%s", err, stdout.String())
	}
	return code, envelope
}

func TestCrossPlatformCoverageChatResultWireSuccessDryRunAndPartialFailure(t *testing.T) {
	pagination, err := output.NewPagination(true, "")
	if err != nil {
		t.Fatal(err)
	}
	pagination.Pages = 2
	pagination.Items = 1
	code, success := emitChatResultWireForTest(t, output.Success(
		map[string]any{
			"count":        1,
			"complete":     false,
			"warningCount": 1,
			"warnings": []map[string]any{{
				"kind": "sender_identity_unverified",
			}},
		},
		output.WithMeta(&output.Meta{Count: output.NewCount(1), Pagination: pagination}),
	))
	meta, _ := success["meta"].(map[string]any)
	paging, _ := meta["pagination"].(map[string]any)
	if code != 0 || success["ok"] != true || success["outcome"] != "success" ||
		paging["endpoint_exhausted"] != true || paging["pages"] != float64(2) || paging["items"] != float64(1) {
		t.Fatalf("success wire = %#v, code=%d", success, code)
	}
	if data, _ := success["data"].(map[string]any); data["complete"] != false || data["warningCount"] != float64(1) {
		t.Fatalf("identity warning data = %#v", success["data"])
	}

	code, dryRun := emitChatResultWireForTest(t, output.Success(
		map[string]any{"dry_run": true, "request": map[string]any{"name": "search_messages"}},
		output.WithDryRun(),
	))
	if code != 0 || dryRun["ok"] != true || dryRun["dry_run"] != true || dryRun["data"] == nil {
		t.Fatalf("dry-run wire = %#v, code=%d", dryRun, code)
	}

	cause := apperrors.NewAPI(
		"upstream busy",
		apperrors.WithRetryable(true),
		apperrors.WithRetryAfterSeconds(9),
		apperrors.WithRPCCode(-32029),
		apperrors.WithTraceID("trace-chat-result"),
	)
	partialResult := map[string]any{
		"count": 1, "complete": false, "failedCount": 1,
		"failures": []map[string]any{{"stage": "search-page"}},
	}
	incomplete := helpers.NewIncompleteResultError(
		"search incomplete",
		cause,
		false,
		apperrors.WithOperation("im/search_messages"),
		apperrors.WithReason("search_messages_incomplete"),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithDetails(map[string]any{
			"count": 1, "failedCount": 1, "stopReason": "read_failure",
			"partialResult": partialResult,
		}),
	)
	info := errorInfoFromExecutionError(incomplete)
	code, failure := emitChatResultWireForTest(t, output.FailureWithExitCode(info, apperrors.ExitCode(incomplete)))
	errorBody, _ := failure["error"].(map[string]any)
	details, _ := errorBody["details"].(map[string]any)
	partial, _ := details["partialResult"].(map[string]any)
	if code != apperrors.ExitCodeAPI || failure["ok"] != false || failure["outcome"] != "failure" || failure["data"] != nil {
		t.Fatalf("failure wire = %#v, code=%d", failure, code)
	}
	if errorBody["type"] != "api" || errorBody["subtype"] != "search_messages_incomplete" ||
		errorBody["retryable"] != true || errorBody["retry_after_seconds"] != float64(9) ||
		errorBody["rpc_code"] != float64(-32029) || errorBody["trace_id"] != "trace-chat-result" ||
		errorBody["cause"] != "upstream busy" || partial["complete"] != false || partial["count"] != float64(1) {
		t.Fatalf("typed partial failure wire = %#v", failure)
	}
	if _, duplicated := details["failures"]; duplicated {
		t.Fatalf("error details duplicated the canonical partial ledger: %#v", details)
	}
}
