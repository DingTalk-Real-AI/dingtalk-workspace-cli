// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package shortcut

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageRuntimeContextForTest(t *testing.T) {
	cmd := &cobra.Command{Use: "run"}
	rt := RuntimeContextForTest(cmd, Shortcut{Service: "sample", Command: "run"})
	if rt == nil || rt.cmd != cmd || rt.shortcut.Service != "sample" {
		t.Fatalf("RuntimeContextForTest = %#v", rt)
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
