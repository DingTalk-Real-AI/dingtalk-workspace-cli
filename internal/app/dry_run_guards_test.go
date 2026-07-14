// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

func TestToolCallerAdapterDryRunNeverInvokesRunner(t *testing.T) {
	runner := &countingErrorRunner{}
	caller := newToolCallerAdapter(runner, &GlobalFlags{DryRun: true, Format: "json"})
	result, err := caller.CallTool(context.Background(), "aitable-helper", "set_advanced_permission", map[string]any{"enabled": false})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if got := runner.calls.Load(); got != 0 {
		t.Fatalf("runner calls = %d, want 0", got)
	}
	if result == nil || len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, `"dry_run":true`) {
		t.Fatalf("dry-run result = %#v", result)
	}

	var nilAdapter *toolCallerAdapter
	if nilAdapter.DryRun() || nilAdapter.Format() != "json" {
		t.Fatal("nil adapter accessors are not safe")
	}
	if _, err := nilAdapter.CallTool(context.Background(), "x", "y", nil); err == nil {
		t.Fatal("nil adapter accepted a tool call")
	}
}

func TestRuntimeRunnerGlobalDryRunStopsBeforeInjectedFallback(t *testing.T) {
	fallback := &countingErrorRunner{}
	runner := &runtimeRunner{globalFlags: &GlobalFlags{DryRun: true}, fallback: fallback}
	result, err := runner.Run(context.Background(), executor.NewHelperInvocation(
		"test",
		"aitable",
		"tool",
		map[string]any{"id": "x"},
	))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Invocation.DryRun || result.Response["dry_run"] != true {
		t.Fatalf("dry-run result = %#v", result)
	}
	if got := fallback.calls.Load(); got != 0 {
		t.Fatalf("fallback calls = %d, want 0", got)
	}
}

type countingErrorRunner struct {
	calls atomic.Int64
}

func (r *countingErrorRunner) Run(context.Context, executor.Invocation) (executor.Result, error) {
	r.calls.Add(1)
	return executor.Result{}, errors.New("runner must not be called")
}
