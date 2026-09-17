// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

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
					"category": "network", "reason": "timeout", "message": "bad",
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
		map[string]any{"selector": "bad", "ok": false, "error": map[string]any{"category": "forbidden", "actions": []string{"fix"}, "code": "E"}},
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
