// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageDevAppErrorProjectionEdges(t *testing.T) {
	unknown := devAppFailureResult(map[string]any{"outcome": "unknown"})
	if unknown == nil {
		t.Fatal("unknown outcome was not classified")
	}
	if env, err := output.EnvelopeFromResult(unknown); err != nil || env.Error == nil || env.Error.ExecutionStarted == nil || !*env.Error.ExecutionStarted {
		t.Fatalf("unknown envelope = %#v, err=%v", env, err)
	}

	detail := map[string]any{
		"errorType":   "api",
		"message":     "nested failure",
		"code":        "DETAIL_CODE",
		"details":     map[string]any{"detail": true},
		"nextCommand": "dws next",
	}
	content := map[string]any{
		"content": []any{"skip", map[string]any{
			"error_detail":     detail,
			"checkpoint":       "checkpoint-1",
			"knownSideEffects": []string{"effect"},
			"rpc_data":         map[string]any{"raw": true},
		}},
		"rpc_data": map[string]any{"raw": true},
		"code":     "ERROR",
	}
	if devAppErrorDetailMap(nil) != nil {
		t.Fatal("nil error detail unexpectedly produced a map")
	}
	unknownValue := true
	if details := devAppErrorDetails(nil, nil, apperrors.ServerDiagnostics{Unknown: &unknownValue}); details["unknown"] != true {
		t.Fatalf("unknown diagnostics = %#v", details)
	}
	if _, ok := devAppRecursiveValue(nil, 0, "checkpoint"); ok {
		t.Fatal("nil recursive value unexpectedly found")
	}
	info := devAppErrorInfo(content, "fallback")
	if info.Message != "nested failure" || info.UpstreamCode != "DETAIL_CODE" || info.RPCData == nil {
		t.Fatalf("error info = %#v", info)
	}
	if info.Details["checkpoint"] != "checkpoint-1" || info.Details["knownSideEffects"] == nil || info.Details["nextCommand"] != "dws next" {
		t.Fatalf("error details = %#v", info.Details)
	}
	reserved := devAppFailureResult(map[string]any{
		"success":     false,
		"errorDetail": map[string]any{"reason": "confirmation_required", "details": map[string]any{"source": "server"}},
	})
	reservedEnv, err := output.EnvelopeFromResult(reserved)
	if err != nil || reservedEnv.Error == nil || reservedEnv.Error.Subtype == "confirmation_required" {
		t.Fatalf("reserved reason = %#v, err=%v", reservedEnv, err)
	}
	if !containsDevAppAction([]string{"same"}, "same") || containsDevAppAction(nil, "same") {
		t.Fatal("action deduplication edges failed")
	}
	for _, code := range []any{"ERROR", "BUSINESS_ERROR", "-1", "other"} {
		_ = isDevAppWrapperCode(code)
	}
	for _, value := range []map[string]any{{}, {"key": "value"}} {
		_ = cloneDevAppMap(value)
	}
	_, _ = devAppUpstreamCode(map[string]any{"code": "ERROR"}, nil, apperrors.ServerDiagnostics{ServerErrorCode: "REAL"})
	_, _ = devAppUpstreamCode(map[string]any{"data": []any{"skip", map[string]any{"code": "DEEP"}}}, nil, apperrors.ServerDiagnostics{})
}

func TestDevAppSharedResultMapperClassifiesServiceOutcomes(t *testing.T) {
	t.Run("normalized success", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{
			"content": map[string]any{"success": true, "result": map[string]any{"id": "a"}},
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := env.Data.(map[string]any)
		if env.Outcome != output.OutcomeSuccess || data["id"] != "a" {
			t.Fatalf("success envelope=%+v data=%#v", env, env.Data)
		}
	})

	t.Run("explicit pending outcome wins over success flag", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{
			"success":      true,
			"outcome":      "pending",
			"unifiedAppId": "a1",
			"versionId":    "v1",
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomePending || !env.OK || result.ExitCode() != 0 {
			t.Fatalf("pending envelope=%+v rc=%d", env, result.ExitCode())
		}
		if env.Meta == nil || env.Meta.Operation == nil || env.Meta.Operation.ID != "v1" ||
			!strings.Contains(env.Meta.Operation.NextCommand, "dev app version status") {
			t.Fatalf("pending operation=%+v", env.Meta)
		}
	})

	t.Run("pending recovery command quotes identifiers", func(t *testing.T) {
		cases := []struct {
			name    string
			payload map[string]any
			parts   []string
		}{
			{
				name:    "task",
				payload: map[string]any{"outcome": "pending", "taskId": `task id; echo "bad"`},
				parts:   []string{"--task-id " + ShellQuoteArg(`task id; echo "bad"`)},
			},
			{
				name: "version",
				payload: map[string]any{
					"outcome":      "pending",
					"unifiedAppId": `app id; $(echo bad)`,
					"versionId":    `version'one`,
				},
				parts: []string{
					"--unified-app-id " + ShellQuoteArg(`app id; $(echo bad)`),
					"--version-id " + ShellQuoteArg(`version'one`),
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				result := DevAppCommandResultFromPayload("", tc.payload, false)
				env, err := output.EnvelopeFromResult(result)
				if err != nil {
					t.Fatal(err)
				}
				if env.Meta == nil || env.Meta.Operation == nil || env.Outcome != output.OutcomePending {
					t.Fatalf("pending envelope=%+v", env)
				}
				for _, part := range tc.parts {
					if !strings.Contains(env.Meta.Operation.NextCommand, part) {
						t.Fatalf("next command %q does not contain quoted part %q", env.Meta.Operation.NextCommand, part)
					}
				}
			})
		}
	})

	t.Run("explicit failure outcome is not success", func(t *testing.T) {
		result := DevAppCommandResultFromPayload(devAppScopeSetTool, map[string]any{
			"outcome": "failure",
			"errorDetail": map[string]any{
				"errorType": "permission",
				"message":   "scope update rejected",
			},
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.OK || env.Outcome != output.OutcomeFailure || env.Error == nil ||
			env.Error.Type != "auth" || env.Error.Message != "scope update rejected" || result.ExitCode() != 2 {
			t.Fatalf("failure envelope=%+v rc=%d", env, result.ExitCode())
		}
	})

	t.Run("dev error categories preserve exit codes", func(t *testing.T) {
		want := map[string]int{"api": 1, "auth": 2, "validation": 3, "internal": 5}
		for category, exitCode := range want {
			result := DevAppCommandResultFromPayload("", map[string]any{
				"success": false,
				"errorDetail": map[string]any{
					"errorType": category,
					"message":   "failure",
				},
			}, false)
			env, err := output.EnvelopeFromResult(result)
			if err != nil {
				t.Fatal(err)
			}
			if env.Outcome != output.OutcomeFailure || env.Error == nil || env.Error.Type != category || result.ExitCode() != exitCode {
				t.Fatalf("category=%s envelope=%+v rc=%d", category, env, result.ExitCode())
			}
		}
	})

	t.Run("non boolean success fails closed", func(t *testing.T) {
		for _, invalid := range []any{"false", 0, map[string]any{"value": false}} {
			result := DevAppCommandResultFromPayload("", map[string]any{"success": invalid}, false)
			env, err := output.EnvelopeFromResult(result)
			if err != nil {
				t.Fatal(err)
			}
			if env.Outcome != output.OutcomeFailure || env.Error == nil ||
				env.Error.Subtype != "invalid_success_type" || env.Error.Hint == "" {
				t.Fatalf("invalid success=%#v envelope=%+v", invalid, env)
			}
		}
	})

	t.Run("service result unknown write is a failure", func(t *testing.T) {
		result := DevAppCommandResultFromPayload(devAppScopeSetTool, map[string]any{
			"success": true,
			"result": map[string]any{
				"outcome":          "unknown",
				"executionStarted": true,
				"retryable":        false,
				"versionId":        "v1",
				"unifiedAppId":     "u1",
				"nextCommand":      "dws dev app version status --unified-app-id u1 --version-id v1 --format json",
				"message":          "Scope 设置已开始，但远端结果无法确认",
			},
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomeFailure || env.Error == nil || env.Error.Subtype != "remote_outcome_unknown" {
			t.Fatalf("unknown envelope=%+v", env)
		}
		if env.Error.ExecutionStarted == nil || !*env.Error.ExecutionStarted || env.Error.Retryable {
			t.Fatalf("unknown error state=%+v", env.Error)
		}
		if env.Error.Details["unifiedAppId"] != "u1" || env.Error.Details["versionId"] != "v1" ||
			env.Error.Details["nextCommand"] != "dws dev app version status --unified-app-id u1 --version-id v1 --format json" {
			t.Fatalf("unknown recovery details=%#v", env.Error.Details)
		}
	})

	t.Run("pending approval", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{
			"versionStatus": "AUDIT", "versionId": "v1", "unifiedAppId": "u1",
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomePending || env.Meta == nil || env.Meta.Operation == nil || env.Meta.Operation.NextCommand == "" {
			t.Fatalf("pending envelope=%+v", env)
		}
	})

	t.Run("approval selection uses tool normalization", func(t *testing.T) {
		result := DevAppCommandResultFromPayload(devAppVersionPublishTool, map[string]any{
			"approvalMode": "SELECT_APPROVER",
			"unifiedAppId": "u1",
			"versionId":    "v1",
			"approvalCandidates": []any{
				map[string]any{"userId": "user-1", "name": "Alice"},
			},
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomePending || env.Meta == nil || env.Meta.Operation == nil {
			t.Fatalf("approval envelope=%+v", env)
		}
		if env.Meta.Operation.State != "waiting_for_approver_selection" || env.Meta.Operation.NextCommand == "" {
			t.Fatalf("approval operation=%+v", env.Meta.Operation)
		}
		if strings.Contains(env.Meta.Operation.NextCommand, "--yes") {
			t.Fatalf("approval next_command bypasses confirmation: %q", env.Meta.Operation.NextCommand)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{
			"items": []any{}, "hasMore": true, "nextCursor": "next",
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Meta == nil || env.Meta.Pagination == nil || env.Meta.Pagination.EndpointExhausted || env.Meta.Pagination.NextToken != "next" {
			t.Fatalf("pagination envelope=%+v", env)
		}
		data, ok := env.Data.(map[string]any)
		if !ok {
			t.Fatalf("pagination data type=%T, want map[string]any", env.Data)
		}
		if _, exists := data["hasMore"]; exists {
			t.Fatalf("business data must not retain hasMore: %#v", data)
		}
		if _, exists := data["nextCursor"]; exists {
			t.Fatalf("business data must not retain nextCursor: %#v", data)
		}
		if _, exists := data["items"]; !exists {
			t.Fatalf("business data lost items while extracting pagination: %#v", data)
		}
	})

	for name, payload := range map[string]map[string]any{
		"missing cursor":  {"items": []any{}, "hasMore": true},
		"wrong flag type": {"items": []any{}, "hasMore": "false"},
	} {
		t.Run("pagination inconsistent "+name, func(t *testing.T) {
			result := DevAppCommandResultFromPayload("", payload, false)
			env, err := output.EnvelopeFromResult(result)
			if err != nil {
				t.Fatal(err)
			}
			if env.Outcome != output.OutcomeFailure || env.Error == nil || env.Error.Subtype != "pagination_inconsistent" {
				t.Fatalf("pagination envelope=%+v", env)
			}
		})
	}

	t.Run("terminal cursor is not resumable", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{
			"items": []any{}, "hasMore": false, "nextCursor": "terminal-cursor",
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomeSuccess || env.Meta == nil || env.Meta.Pagination == nil ||
			!env.Meta.Pagination.EndpointExhausted || env.Meta.Pagination.NextToken != "" {
			t.Fatalf("terminal pagination envelope=%+v", env)
		}
		data, _ := env.Data.(map[string]any)
		if _, exists := data["nextCursor"]; exists {
			t.Fatalf("terminal data leaked cursor: %#v", data)
		}
	})

	for _, tool := range []string{devAppListTool, devAppPermissionListTool, devAppEventListTool, devAppVersionListTool} {
		t.Run("declared pagination missing "+tool, func(t *testing.T) {
			result := DevAppCommandResultFromPayload(tool, map[string]any{"items": []any{}}, false)
			env, err := output.EnvelopeFromResult(result)
			if err != nil {
				t.Fatal(err)
			}
			if env.Outcome != output.OutcomeFailure || env.Error == nil ||
				env.Error.Subtype != "pagination_inconsistent" {
				t.Fatalf("missing pagination envelope=%+v", env)
			}
		})

		t.Run("dry run preview needs no server pagination "+tool, func(t *testing.T) {
			result := DevAppCommandResultFromPayload(tool, map[string]any{
				"invocation": map[string]any{"tool": tool},
			}, true)
			env, err := output.EnvelopeFromResult(result)
			if err != nil {
				t.Fatal(err)
			}
			if env.Outcome != output.OutcomeSuccess || !env.DryRun || env.Meta != nil {
				t.Fatalf("dry-run preview envelope=%+v", env)
			}
		})
	}

	t.Run("partial", func(t *testing.T) {
		started := true
		result := DevAppCommandResultFromPayload("", map[string]any{
			"multiProfile": true,
			"profiles": []any{
				map[string]any{"selector": "a", "ok": true, "result": map[string]any{"id": "r1"}},
				map[string]any{"selector": "b", "ok": false, "error": map[string]any{
					"errorType": "network", "reason": "timeout", "message": "bad",
					"stage": "response", "execution_started": started,
				}},
			},
		}, false)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomePartialFailure || result.ExitCode() != 7 {
			t.Fatalf("partial outcome=%s rc=%d", env.Outcome, result.ExitCode())
		}
		partial, ok := env.Data.(*output.PartialData)
		if !ok || len(partial.Failed) != 1 || partial.Failed[0].Error == nil ||
			partial.Failed[0].Error.Type != "api" || partial.Failed[0].Error.Subtype != "timeout" ||
			partial.Failed[0].Error.ExecutionStarted == nil || !*partial.Failed[0].Error.ExecutionStarted {
			t.Fatalf("partial error projection=%#v", env.Data)
		}
	})

	t.Run("dry run is completed preview", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{"tool": "x"}, true)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomeSuccess || !env.DryRun {
			t.Fatalf("dry-run envelope=%+v", env)
		}
	})

	t.Run("dry run still enforces pagination contract", func(t *testing.T) {
		result := DevAppCommandResultFromPayload(devAppListTool, map[string]any{
			"items": []any{}, "hasMore": false,
		}, true)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomeSuccess || !env.DryRun || env.Meta == nil ||
			env.Meta.Pagination == nil || !env.Meta.Pagination.EndpointExhausted {
			t.Fatalf("dry-run pagination envelope=%+v", env)
		}
		data, ok := env.Data.(map[string]any)
		if !ok {
			t.Fatalf("dry-run pagination data type=%T", env.Data)
		}
		if _, exists := data["hasMore"]; exists {
			t.Fatalf("dry-run data leaked hasMore: %#v", data)
		}
		if _, exists := data["nextCursor"]; exists {
			t.Fatalf("dry-run data leaked nextCursor: %#v", data)
		}
	})

	t.Run("dry run still fails closed on invalid success type", func(t *testing.T) {
		result := DevAppCommandResultFromPayload("", map[string]any{"success": "false"}, true)
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomeFailure || env.Error == nil || env.Error.Subtype != "invalid_success_type" {
			t.Fatalf("dry-run invalid success envelope=%+v", env)
		}
	})
}

func TestCrossPlatformCoverageDevAppErrorDetailDepthAndTypeFallback(t *testing.T) {
	deep := map[string]any{"message": "too deep"}
	for range 10 {
		deep = map[string]any{"data": deep}
	}
	if devAppErrorDetailMap(deep) != nil {
		t.Fatal("deep error detail unexpectedly traversed")
	}

	content := map[string]any{
		"error": map[string]any{"errorType": "auth"},
		"data":  map[string]any{"errorDetail": map[string]any{"message": "denied"}},
	}
	info := devAppErrorInfo(content, "fallback")
	if info.Type != "auth" {
		t.Fatalf("errorType fallback = %q, want auth", info.Type)
	}
	businessInfo := devAppErrorInfo(map[string]any{
		"errorDetail": map[string]any{"category": "application_scope"},
	}, "fallback")
	if businessInfo.Type != "api" {
		t.Fatalf("business category = %q, want api", businessInfo.Type)
	}
	diagInfo := devAppErrorInfo(map[string]any{
		"error": map[string]any{"details": map[string]any{"fromDiag": true}},
	}, "fallback")
	if diagInfo.Details["fromDiag"] != true {
		t.Fatalf("diagnostic details = %#v", diagInfo.Details)
	}
}

func TestCrossPlatformCoverageDevAppApprovalIdentifiersFailClosedOnWindows(t *testing.T) {
	testseam.Swap(t, &devAppQuoteScopeValue, driveLatestWindowsScopeValue)
	content := map[string]any{
		"approvalMode": "SELECT_APPROVER",
		"unifiedAppId": "app&bad",
		"versionId":    "version%bad",
		"approvalCandidates": []any{
			map[string]any{"userId": "user-1", "name": "Alice"},
		},
	}
	normalizeDevAppVersionApproval(content)
	steps, _ := content["nextSteps"].([]map[string]any)
	if len(steps) != 2 {
		t.Fatalf("approval steps = %#v", content["nextSteps"])
	}
	command, _ := steps[1]["command"].(string)
	if command != "" || !strings.Contains(strings.Join(anyStrings(steps[1]["missingParams"]), ","), "unifiedAppId") {
		t.Fatalf("unsafe approval step = %#v", steps[1])
	}
}

func anyStrings(value any) []string {
	values, _ := value.([]string)
	return values
}

func TestDevAppRecoveryCommandsDoNotBypassConfirmation(t *testing.T) {
	steps := append(devAppRobotPublishSteps("u1"), devAppRobotRetryStep("task1", true))
	approval := map[string]any{
		"approvalMode": "SELECT_APPROVER",
		"unifiedAppId": "u1",
		"versionId":    "v1",
		"approvalCandidates": []any{
			map[string]any{"userId": "user-1", "name": "Alice"},
		},
	}
	normalizeDevAppVersionApproval(approval)
	steps = append(steps, approval["nextSteps"].([]map[string]any)...)
	for _, step := range steps {
		for _, key := range []string{"command", "dryRunCommand"} {
			command, _ := step[key].(string)
			for _, field := range strings.Fields(command) {
				if field == "--yes" || strings.HasPrefix(field, "--yes=") {
					t.Fatalf("step %v publishes confirmation bypass in %s: %q", step["id"], key, command)
				}
			}
		}
	}
}

func TestFrameworkDevAppMapperBoundaryMatrix(t *testing.T) {
	if got := devAppDataWithoutPagination("scalar"); got != "scalar" {
		t.Fatalf("non-object data = %#v, want scalar preserved", got)
	}
	for name, payload := range map[string]any{
		"scalar":       "value",
		"bad cursor":   map[string]any{"nextCursor": 1},
		"cursor only":  map[string]any{"nextCursor": "next"},
		"empty cursor": map[string]any{"nextCursor": ""},
		"exhausted":    map[string]any{"hasMore": false},
	} {
		t.Run(name, func(t *testing.T) { _, _ = devAppPaginationMeta(payload) })
	}

	if devAppMultiProfileResult(map[string]any{}) != nil || devAppMultiProfileResult(map[string]any{"multiProfile": true}) != nil {
		t.Fatal("non-profile payload classified")
	}
	allSuccess := devAppMultiProfileResult(map[string]any{"multiProfile": true, "profiles": []any{map[string]any{"ok": true}}})
	if allSuccess != nil {
		t.Fatalf("all-success multi profile=%v", allSuccess)
	}
	allFailed := devAppMultiProfileResult(map[string]any{"multiProfile": true, "profiles": []any{
		map[string]any{"ok": false, "error": map[string]any{
			"type": "authorization", "message": "denied", "retryable": true, "execution_started": false,
			"actions": []any{"retry", 1}, "details": map[string]any{"scope": "x"}, "requestId": "req", "traceId": "trace",
		}},
		"malformed",
	}})
	allFailedEnv, err := output.EnvelopeFromResult(allFailed)
	if err != nil || allFailedEnv.Outcome != output.OutcomeFailure || allFailedEnv.Error == nil || allFailedEnv.Error.Details == nil {
		t.Fatalf("all-failed=%+v err=%v", allFailedEnv, err)
	}
	mixed := devAppMultiProfileResult(map[string]any{"multiProfile": true, "profiles": []any{
		map[string]any{"selector": "ok", "ok": true},
		map[string]any{"selector": "bad", "ok": false, "error": map[string]any{"errorType": "forbidden", "actions": []string{"fix"}, "code": "E"}},
		123,
	}})
	if env, err := output.EnvelopeFromResult(mixed); err != nil || env.Outcome != output.OutcomePartialFailure {
		t.Fatalf("mixed=%+v err=%v", env, err)
	}
	duplicate := devAppMultiProfileResult(map[string]any{"multiProfile": true, "profiles": []any{
		map[string]any{"selector": "same", "ok": true},
		map[string]any{"selector": "same", "ok": false},
	}})
	if env, err := output.EnvelopeFromResult(duplicate); err != nil || env.Outcome != output.OutcomeFailure || env.Error.Type != "internal" {
		t.Fatalf("duplicate=%+v err=%v", env, err)
	}

	for _, payload := range []map[string]any{
		{"success": false, "message": "no", "errorCode": "E"},
		{"status": "FAILED", "code": 1},
		{"status": "OK"},
	} {
		_ = devAppFailureResult(payload)
	}
	for _, raw := range []string{"validation", "forbidden", "timeout"} {
		_ = devAppWireErrorType(raw)
	}

	for _, content := range []map[string]any{
		{},
		{"mustAskUser": true},
		{"status": "WAITING", "taskId": "task"},
		{"status": "WAITING", "requestId": "req"},
		{"status": "WAITING", "taskId": "task", "nextSteps": []map[string]any{{"command": "dws next"}}},
		{"status": "WAITING", "requestId": "req", "nextSteps": []any{map[string]any{"dryRunCommand": "dws preview"}, "bad"}},
		{"approvalSubmitted": true, "unifiedAppId": "app", "versionId": "v"},
	} {
		result := devAppPendingResult(content)
		if result != nil {
			_, _ = output.EnvelopeFromResult(result)
		}
	}
	if got := devAppRecoveryCommand(map[string]any{"taskId": "t"}); !bytes.Contains([]byte(got), []byte("robot result")) {
		t.Fatalf("task recovery=%q", got)
	}
	if got := devAppRecoveryCommand(map[string]any{}); got != "" {
		t.Fatalf("empty recovery=%q", got)
	}

	if got := devAppEnvelopeData(executor.Result{}); got == nil {
		t.Fatal("unimplemented result should be preserved")
	}
	for _, result := range []executor.Result{
		{Invocation: executor.Invocation{Implemented: true, Kind: "helper_invocation", DryRun: true}, Response: map[string]any{"content": map[string]any{"preview": true}}},
		{Invocation: executor.Invocation{Implemented: true, Kind: "helper_invocation"}, Response: map[string]any{"content": map[string]any{"items": []any{}, "hasMore": false}}},
	} {
		cmd := &cobra.Command{Use: "legacy-envelope"}
		cmd.SetContext(context.Background())
		cmd.SetOut(&bytes.Buffer{})
		if err := writeDevAppEnvelope(cmd, result); err != nil {
			t.Fatal(err)
		}
	}

	legacy := &cobra.Command{Use: "legacy"}
	legacy.SetContext(context.Background())
	legacy.SetOut(&bytes.Buffer{})
	if err := writeDevRolloutResult(legacy, output.Success(nil), output.NewSuccessEnvelope(nil), output.FormatJSON); err != nil {
		t.Fatal(err)
	}
	dual := &cobra.Command{Use: "dual"}
	dual.SetContext(context.Background())
	dual.SetOut(&bytes.Buffer{})
	output.SetCommandRollout(dual, output.RolloutDualValidate)
	if err := writeDevRolloutResult(dual, output.Pending(nil, nil), output.NewSuccessEnvelope(nil), output.FormatJSON); err == nil {
		t.Fatal("dual validate accepted malformed shadow result")
	}
	ctx, _ := output.WithResultStore(context.Background())
	active := &cobra.Command{Use: "active"}
	active.SetContext(ctx)
	output.SetCommandRollout(active, output.RolloutUnifiedActive)
	if err := writeDevRolloutResult(active, output.Success(nil), nil, output.FormatJSON); err != nil {
		t.Fatal(err)
	}
}

func TestDevAppErrorInfoProjectsStructuredErrorDetail(t *testing.T) {
	result := DevAppCommandResultFromPayload("set_dev_app_scope", map[string]any{
		"success":   false,
		"errorCode": "BUSINESS_ERROR",
		"errorMsg":  "legacy message",
		"errorDetail": map[string]any{
			"errorType":        "validation",
			"code":             "ONLY_INNERAPP_SUPPORT",
			"message":          "仅支持企业内部应用",
			"reason":           "inner_app_only",
			"hint":             "请改用企业自建应用",
			"retryable":        false,
			"executionStarted": false,
			"nextCommand":      "查询应用类型",
			"actions":          []any{"停止当前操作"},
			"contractVersion":  "opendev.appdev.v1",
			"details":          map[string]any{"scope": "application"},
		},
	}, false)
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Outcome != output.OutcomeFailure || env.Error == nil {
		t.Fatalf("structured error envelope=%+v", env)
	}
	info := env.Error
	if info.Type != "validation" || info.Subtype != "inner_app_only" ||
		info.UpstreamCode != "ONLY_INNERAPP_SUPPORT" || info.Message != "仅支持企业内部应用" ||
		info.Hint != "请改用企业自建应用" || info.ExecutionStarted == nil || *info.ExecutionStarted {
		t.Fatalf("structured projection=%+v", info)
	}
	if len(info.Actions) != 2 || info.Actions[0] != "停止当前操作" || info.Actions[1] != "查询应用类型" {
		t.Fatalf("actions=%v", info.Actions)
	}
	if info.Details["contractVersion"] != "opendev.appdev.v1" ||
		info.Details["scope"] != "application" {
		t.Fatalf("details=%#v", info.Details)
	}
}

func TestDevAppErrorInfoPreservesUnknownWriteState(t *testing.T) {
	result := DevAppCommandResultFromPayload("set_dev_app_scope", map[string]any{
		"success": false,
		"result": map[string]any{
			"errorDetail": map[string]any{
				"code":             "PUBLISH_UNKNOWN",
				"message":          "Scope 设置已开始，但远端结果无法确认",
				"reason":           "remote_outcome_unknown",
				"outcome":          "unknown",
				"executionStarted": true,
				"retryable":        false,
				"nextStep":         "先查询 versionId 的版本状态，不要直接重试设置",
				"checkpoint":       map[string]any{"versionId": "v1"},
			},
		},
	}, false)
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Retryable || env.Error.ExecutionStarted == nil || !*env.Error.ExecutionStarted {
		t.Fatalf("unknown write error=%+v", env.Error)
	}
	checkpoint, ok := env.Error.Details["checkpoint"].(map[string]any)
	if !ok || checkpoint["versionId"] != "v1" || env.Error.Subtype != "remote_outcome_unknown" || env.Error.Details["outcome"] != "unknown" ||
		env.Error.Details["nextCommand"] != "先查询 versionId 的版本状态，不要直接重试设置" {
		t.Fatalf("unknown details=%#v", env.Error.Details)
	}
	if len(env.Error.Actions) != 1 || !strings.Contains(env.Error.Actions[0], "不要直接重试") {
		t.Fatalf("unknown actions=%v", env.Error.Actions)
	}
}

func TestCrossPlatformCoverageDevAppUnknownRecoveryUsesNestedIdentifiers(t *testing.T) {
	t.Parallel()
	result := DevAppCommandResultFromPayload("set_dev_app_scope", map[string]any{
		"result": map[string]any{
			"outcome": "unknown",
			"errorDetail": map[string]any{
				"unifiedAppId": "app id; bad",
				"versionId":    "version'one",
			},
		},
	}, false)
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Details["unifiedAppId"] != "app id; bad" || env.Error.Details["versionId"] != "version'one" {
		t.Fatalf("nested recovery details=%#v", env.Error)
	}
	if _, quotable := devAppQuoteScopeValue("app id; bad"); !quotable {
		// This platform's quoter fail-closes on shell-hostile IDs: no command
		// is fabricated, the recovery stays on the generic unknown guidance.
		if _, fabricated := env.Error.Details["nextCommand"]; fabricated {
			t.Fatalf("fail-closed platform must not fabricate a recovery command: %#v", env.Error.Details["nextCommand"])
		}
		return
	}
	wantCommand := "dws dev app version status --unified-app-id " + ShellQuoteArg("app id; bad") + " --version-id " + ShellQuoteArg("version'one") + " --format json"
	if env.Error.Details["nextCommand"] != wantCommand || !containsDevAppAction(env.Error.Actions, wantCommand) {
		t.Fatalf("nested recovery command details=%#v actions=%v", env.Error.Details, env.Error.Actions)
	}
}

func TestCrossPlatformCoverageDevAppUnknownRecoveryFailsClosedOnWindowsQuoting(t *testing.T) {
	// Not parallel: the seam swap is process-global and would leak into the
	// parallel POSIX expectation test above.
	testseam.Swap(t, &devAppQuoteScopeValue, driveLatestWindowsScopeValue)
	result := DevAppCommandResultFromPayload("set_dev_app_scope", map[string]any{
		"result": map[string]any{
			"outcome": "unknown",
			"errorDetail": map[string]any{
				"unifiedAppId": "app id; bad",
				"versionId":    "version'one",
			},
		},
	}, false)
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Error == nil {
		t.Fatal("windows-seam recovery produced no error")
	}
	if _, fabricated := env.Error.Details["nextCommand"]; fabricated {
		t.Fatalf("windows-seam must not fabricate a recovery command: %#v", env.Error.Details["nextCommand"])
	}
	if env.Error.Details["unifiedAppId"] != "app id; bad" {
		t.Fatalf("windows-seam nested recovery=%#v", env.Error)
	}
}

func TestCrossPlatformCoverageDevAppMultiProfilePreservesLocalCategory(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		category string
		wantType string
	}{
		{category: "auth", wantType: "auth"},
		{category: "validation", wantType: "validation"},
		{category: "internal", wantType: "internal"},
		{category: "application_scope", wantType: "api"},
	} {
		result := devAppMultiProfileResult(map[string]any{
			"multiProfile": true,
			"profiles": []any{
				map[string]any{"selector": "only", "ok": false, "error": map[string]any{
					"category": tc.category,
					"message":  "profile denied",
				}},
			},
		})
		env, err := output.EnvelopeFromResult(result)
		if err != nil || env.Error == nil {
			t.Fatalf("category=%s envelope=%+v err=%v", tc.category, env, err)
		}
		if env.Error.Type != tc.wantType {
			t.Fatalf("category=%s projected type=%q, want %q", tc.category, env.Error.Type, tc.wantType)
		}
	}

	profile := func(category string) map[string]any {
		return map[string]any{"multiProfile": true, "profiles": []any{
			map[string]any{"selector": "a", "ok": false, "error": map[string]any{"category": "auth"}},
			map[string]any{"selector": "b", "ok": false, "error": map[string]any{"category": category}},
		}}
	}
	same, err := output.EnvelopeFromResult(devAppMultiProfileResult(profile("auth")))
	if err != nil || same.Error == nil || same.Error.Type != "auth" {
		t.Fatalf("same-category aggregate=%+v err=%v", same.Error, err)
	}
	mixed, err := output.EnvelopeFromResult(devAppMultiProfileResult(profile("validation")))
	if err != nil || mixed.Error == nil || mixed.Error.Type != "api" {
		t.Fatalf("mixed-category aggregate=%+v err=%v", mixed.Error, err)
	}
	withUnknown, err := output.EnvelopeFromResult(devAppMultiProfileResult(map[string]any{
		"multiProfile": true,
		"profiles": []any{
			map[string]any{"selector": "a", "ok": false, "error": map[string]any{"category": "auth"}},
			"malformed",
		},
	}))
	if err != nil || withUnknown.Error == nil || withUnknown.Error.Type != "api" {
		t.Fatalf("unknown-present aggregate=%+v err=%v, want api (no replay invitation)", withUnknown.Error, err)
	}
}

func TestCrossPlatformCoverageDevAppResourceStateStaysCallSuccess(t *testing.T) {
	t.Parallel()
	// A successful query whose business data describes a failed/expired
	// resource must not be reclassified as a failed call.
	payload := map[string]any{
		"success": true,
		"result": map[string]any{
			"unifiedAppId": "u-1",
			"data": map[string]any{
				"status":        "FAILED",
				"versionStatus": "EXPIRED",
				"outcome":       "failure",
			},
		},
	}
	result := DevAppCommandResultFromPayload("", payload, false)
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Outcome != output.OutcomeSuccess || env.Error != nil || result.ExitCode() != 0 {
		t.Fatalf("resource-state envelope=%+v rc=%d", env, result.ExitCode())
	}
	if got := devAppCallStateString(payload, "status"); got != "" {
		t.Fatalf("data-nested status leaked as call state = %q", got)
	}
	if got := devAppCallStateString(nil, "status"); got != "" {
		t.Fatalf("nil call state = %q, want empty", got)
	}
}

func TestDevAppErrorInfoKeepsLegacyErrorFields(t *testing.T) {
	result := DevAppCommandResultFromPayload("get_dev_app", map[string]any{
		"success":   false,
		"errorCode": 40001,
		"errorMsg":  "legacy failure",
	}, false)
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Type != "api" || env.Error.Message != "legacy failure" || env.Error.UpstreamCode != 40001 {
		t.Fatalf("legacy projection=%+v", env.Error)
	}
}
