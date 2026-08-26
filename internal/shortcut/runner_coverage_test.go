// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package shortcut

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type runtimeContextKey struct{}

type runtimeContextCaller struct {
	value any
}

func (c *runtimeContextCaller) CallTool(ctx context.Context, _ string, _ string, _ map[string]any) (*edition.ToolResult, error) {
	c.value = ctx.Value(runtimeContextKey{})
	return nil, errors.New("stop after context capture")
}

func (*runtimeContextCaller) Format() string { return "json" }
func (*runtimeContextCaller) DryRun() bool   { return false }
func (*runtimeContextCaller) Fields() string { return "" }
func (*runtimeContextCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageRuntimeContextForTest(t *testing.T) {
	cmd := &cobra.Command{Use: "run"}
	rt := RuntimeContextForTest(cmd, Shortcut{Service: "sample", Command: "run"})
	if rt == nil || rt.cmd != cmd || rt.shortcut.Service != "sample" {
		t.Fatalf("RuntimeContextForTest = %#v", rt)
	}
}

func TestCrossPlatformCoverageRuntimeMCPCallsPreserveCommandContext(t *testing.T) {
	caller := &runtimeContextCaller{}
	old := helpers.GetCaller()
	t.Cleanup(func() { helpers.InitDeps(old) })
	helpers.InitDeps(caller)

	ctx := context.WithValue(context.Background(), runtimeContextKey{}, "command-context")
	cmd := &cobra.Command{Use: "+write"}
	cmd.SetContext(ctx)
	output.SetCommandRollout(cmd, output.RolloutDualValidate)
	rt := RuntimeContextForTest(cmd, Shortcut{Service: "aitable", Command: "+write"})
	if err := rt.CallMCP("update_records", map[string]any{"id": "r1"}); err == nil {
		t.Fatal("dual-validate context capture unexpectedly succeeded")
	}
	if caller.value != "command-context" {
		t.Fatalf("dual-validate caller context value = %#v", caller.value)
	}
	if _, err := rt.CallMCPWriteDataStrict("aitable", "update_records", map[string]any{"id": "r1"}); err == nil {
		t.Fatal("capture caller unexpectedly succeeded")
	}
	if caller.value != "command-context" {
		t.Fatalf("caller context value = %#v", caller.value)
	}

	dryCaller := &dualValidateCaller{format: "json", dryRun: true}
	helpers.InitDeps(dryCaller)
	dryCmd := &cobra.Command{Use: "+read"}
	dryCmd.Flags().Bool("dry-run", true, "")
	output.SetCommandRollout(dryCmd, output.RolloutDualValidate)
	var dryOut bytes.Buffer
	helpers.GetFormatter().SetWriters(&dryOut, &dryOut)
	dryRT := RuntimeContextForTest(dryCmd, Shortcut{Service: "aitable", Command: "+read"})
	if err := dryRT.CallMCP("get_fields", map[string]any{"baseId": "b"}); err != nil {
		t.Fatalf("dual-validate dry-run call = %v", err)
	}
	if !strings.Contains(dryOut.String(), `"dry_run": true`) {
		t.Fatalf("dual-validate dry-run output = %q", dryOut.String())
	}
}

func TestShortcutCommandResultRejectsStringSuccess(t *testing.T) {
	result := shortcutCommandResult(map[string]any{"success": "false"})
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Outcome != output.OutcomeFailure || env.Error == nil ||
		env.Error.Subtype != "invalid_success_type" || env.Error.Hint == "" {
		t.Fatalf("string success envelope = %+v", env)
	}
}

func TestGenericWriteProjectionRequiresExplicitSuccessEvidence(t *testing.T) {
	rt := RuntimeContextForTest(&cobra.Command{Use: "+write"}, Shortcut{
		Service: "sample",
		Command: "+write",
		Risk:    RiskWrite,
	})
	result := rt.resultForPayload("update_item", map[string]any{"id": "item-1"})
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Outcome != output.OutcomeFailure || env.Error == nil ||
		env.Error.Subtype != "projection_unknown" || env.Error.ExecutionStarted == nil ||
		!*env.Error.ExecutionStarted || env.Error.Retryable {
		t.Fatalf("opaque write envelope = %+v", env)
	}

	result = rt.resultForPayload("update_item", map[string]any{"success": true, "id": "item-1"})
	if result.Outcome() != output.OutcomeSuccess {
		t.Fatalf("explicit write outcome = %q", result.Outcome())
	}
}

func TestCrossPlatformCoverageRuntimeWriteDataRemainingBranches(t *testing.T) {
	caller := &runtimeReadCoverageCaller{text: `not-json`}
	old := helpers.GetCaller()
	t.Cleanup(func() { helpers.InitDeps(old) })
	helpers.InitDeps(caller)
	rt := &RuntimeContext{}
	if _, err := rt.callMCPWriteData("aitable", "update_records", nil); err == nil || !strings.Contains(err.Error(), "解析") {
		t.Fatalf("invalid write JSON = %v", err)
	}
	if caller.args == nil {
		t.Fatal("nil write parameters were not normalized")
	}

	caller.text = ""
	legacy, err := rt.CallMCPWriteData("chat", "send_personal_message", nil)
	if err != nil || legacy == nil || len(legacy) != 0 {
		t.Fatalf("legacy empty acknowledgement = %#v, %v", legacy, err)
	}
	if _, err = rt.CallMCPWriteDataStrict("aitable", "update_records", nil); err == nil {
		t.Fatal("strict empty acknowledgement was accepted")
	} else {
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "empty_tool_response" || !typed.RetryableSet || typed.Retryable {
			t.Fatalf("strict empty acknowledgement error = %#v", err)
		}
	}
}

func TestFrameworkShortcutUnifiedOutputAndProjectionEdges(t *testing.T) {
	oldCaller := helpers.GetCaller()
	t.Cleanup(func() { helpers.InitDeps(oldCaller) })
	ctx, _ := output.WithResultStore(context.Background())
	cmd := &cobra.Command{Use: "+read"}
	cmd.SetContext(ctx)
	output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
	rt := RuntimeContextForTest(cmd, Shortcut{Service: "sample", Command: "+read", Safety: contract.SafetySpec{Effect: "read"}})
	if err := rt.Output(map[string]any{"id": "x"}); err != nil {
		t.Fatal(err)
	}
	caller := &runtimeReadCoverageCaller{text: `{"success":true,"id":"server"}`}
	helpers.InitDeps(caller)
	callCtx, _ := output.WithResultStore(context.Background())
	callCmd := &cobra.Command{Use: "+call"}
	callCmd.SetContext(callCtx)
	output.SetCommandRollout(callCmd, output.RolloutUnifiedActive)
	callRT := RuntimeContextForTest(callCmd, Shortcut{Service: "sample", Command: "+call", Safety: contract.SafetySpec{Effect: "read"}})
	if err := callRT.CallMCP("get", nil); err != nil {
		t.Fatal(err)
	}
	caller.err = errors.New("backend")
	errorCtx, _ := output.WithResultStore(context.Background())
	errorCmd := &cobra.Command{Use: "+error"}
	errorCmd.SetContext(errorCtx)
	output.SetCommandRollout(errorCmd, output.RolloutUnifiedActive)
	errorRT := RuntimeContextForTest(errorCmd, Shortcut{Service: "sample", Command: "+error", Safety: contract.SafetySpec{Effect: "read"}})
	if err := errorRT.CallMCP("get", nil); err == nil {
		t.Fatal("backend error swallowed")
	}
	caller.err = nil

	dryCtx, _ := output.WithResultStore(context.Background())
	dryCmd := &cobra.Command{Use: "+dry"}
	dryCmd.SetContext(dryCtx)
	dryCmd.Flags().Bool("dry-run", true, "")
	_ = dryCmd.Flags().Set("dry-run", "true")
	output.SetCommandRollout(dryCmd, output.RolloutUnifiedActive)
	dryRT := RuntimeContextForTest(dryCmd, Shortcut{Service: "sample", Command: "+dry", Safety: contract.SafetySpec{Effect: "read"}})
	if err := dryRT.CallMCP("get", nil); err != nil {
		t.Fatal(err)
	}

	dualCmd := &cobra.Command{Use: "+dual"}
	dualCmd.SetContext(context.Background())
	dualCmd.SetOut(&bytes.Buffer{})
	output.SetCommandRollout(dualCmd, output.RolloutDualValidate)
	dualRT := RuntimeContextForTest(dualCmd, Shortcut{Service: "sample", Command: "+dual", Safety: contract.SafetySpec{Effect: "read"}})
	if err := dualRT.Output(map[string]any{"success": "false"}); err != nil {
		t.Fatalf("dual validation should accept a typed failure result: %v", err)
	}

	for _, tc := range []struct {
		name    string
		payload any
		outcome output.Outcome
	}{
		{"scalar", "value", output.OutcomeSuccess},
		{"nested success", map[string]any{"content": map[string]any{"success": true}}, output.OutcomeSuccess},
		{"failure fallback", map[string]any{"success": false}, output.OutcomeFailure},
		{"failure message", map[string]any{"content": map[string]any{"success": false, "errorMessage": "bad"}}, output.OutcomeFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := shortcutCommandResult(tc.payload, output.WithDryRun())
			if result.Outcome() != tc.outcome {
				t.Fatalf("outcome=%s", result.Outcome())
			}
		})
	}
	if hasExplicitShortcutSuccess("scalar") || hasExplicitShortcutSuccess(map[string]any{"success": false}) || !hasExplicitShortcutSuccess(map[string]any{"content": map[string]any{"success": true}}) {
		t.Fatal("explicit success classification mismatch")
	}
	for _, shortcut := range []Shortcut{
		{Safety: contract.SafetySpec{Effect: "write"}},
		{Safety: contract.SafetySpec{Effect: "destructive"}},
		{Risk: RiskHighWrite},
		{Risk: RiskRead},
	} {
		probe := RuntimeContextForTest(&cobra.Command{Use: "probe"}, shortcut)
		_ = probe.isWriteShortcut()
	}
	devRT := RuntimeContextForTest(&cobra.Command{Use: "dev"}, Shortcut{Service: "devapp", Command: "+get"})
	if got := devRT.resultForPayload("get_dev_app", map[string]any{"success": true}); got.Outcome() != output.OutcomeSuccess {
		t.Fatalf("devapp result=%s", got.Outcome())
	}
}

func TestCrossPlatformCoverageRuntimeOutputForToolRollouts(t *testing.T) {
	t.Run("unified", func(t *testing.T) {
		ctx, _ := output.WithResultStore(context.Background())
		cmd := &cobra.Command{Use: "+get"}
		cmd.SetContext(ctx)
		output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
		rt := RuntimeContextForTest(cmd, Shortcut{Service: "devapp", Command: "+get", Safety: contract.SafetySpec{Effect: "read"}})
		if err := rt.OutputForTool("get_dev_app", map[string]any{"success": true, "result": map[string]any{"unifiedAppId": "app"}}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("dual validate", func(t *testing.T) {
		cmd := &cobra.Command{Use: "+get"}
		cmd.SetContext(context.Background())
		cmd.SetOut(&bytes.Buffer{})
		output.SetCommandRollout(cmd, output.RolloutDualValidate)
		rt := RuntimeContextForTest(cmd, Shortcut{Service: "devapp", Command: "+get", Safety: contract.SafetySpec{Effect: "read"}})
		if err := rt.OutputForTool("get_dev_app", map[string]any{"success": true, "result": map[string]any{"unifiedAppId": "app"}}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("dual validate failure", func(t *testing.T) {
		testseam.Swap(t, &validateShadowResult, func(output.CommandResult) error { return context.Canceled })
		cmd := &cobra.Command{Use: "+get"}
		cmd.SetContext(context.Background())
		output.SetCommandRollout(cmd, output.RolloutDualValidate)
		rt := RuntimeContextForTest(cmd, Shortcut{Service: "devapp", Command: "+get", Safety: contract.SafetySpec{Effect: "read"}})
		if err := rt.OutputForTool("get_dev_app", map[string]any{"success": true}); !errors.Is(err, context.Canceled) {
			t.Fatalf("dual validation error=%v", err)
		}
	})

	t.Run("legacy", func(t *testing.T) {
		cmd := &cobra.Command{Use: "+get"}
		cmd.SetContext(context.Background())
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		output.SetCommandRollout(cmd, output.RolloutLegacyOnly)
		rt := RuntimeContextForTest(cmd, Shortcut{Service: "sample", Command: "+get", Safety: contract.SafetySpec{Effect: "read"}})
		if err := rt.OutputForTool("get", map[string]any{"id": "item"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout.String(), `"id"`) {
			t.Fatalf("legacy output=%q", stdout.String())
		}
	})
}
