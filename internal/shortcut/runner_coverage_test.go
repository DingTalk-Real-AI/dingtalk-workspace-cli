// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package shortcut

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageRuntimeContextForTest(t *testing.T) {
	cmd := &cobra.Command{Use: "run"}
	rt := RuntimeContextForTest(cmd, Shortcut{Service: "sample", Command: "run"})
	if rt == nil || rt.cmd != cmd || rt.shortcut.Service != "sample" {
		t.Fatalf("RuntimeContextForTest = %#v", rt)
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
