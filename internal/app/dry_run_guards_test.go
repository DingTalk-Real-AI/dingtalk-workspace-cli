// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"encoding/json"
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

func TestInvocationReadOnlyDryRunCapabilityIsNotSerialized(t *testing.T) {
	invocation := executor.NewHelperInvocation("overlay.pat.pat.batch_plan", "pat", "pat.batch_plan", map[string]any{"dryRun": true})
	invocation.DryRun = true
	invocation.AllowReadOnlyDuringDryRun = true

	encoded, err := json.Marshal(invocation)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "AllowReadOnlyDuringDryRun") || strings.Contains(string(encoded), "allow_read_only") {
		t.Fatalf("read-only dry-run capability leaked into JSON: %s", encoded)
	}
	if invocation.SkipExecutionDuringDryRun() {
		t.Fatal("allowed read-only invocation should not be skipped")
	}

	invocation.AllowReadOnlyDuringDryRun = false
	if !invocation.SkipExecutionDuringDryRun() {
		t.Fatal("ordinary dry-run invocation should be skipped")
	}
	result, err := (executor.EchoRunner{}).Run(context.Background(), invocation)
	if err != nil {
		t.Fatalf("EchoRunner.Run() error = %v", err)
	}
	if result.Response["dry_run"] != true {
		t.Fatalf("EchoRunner dry_run = %#v, want true", result.Response["dry_run"])
	}
}

func TestRuntimeRunnerRejectsForgedReadOnlyDryRunMarker(t *testing.T) {
	tests := []struct {
		name    string
		product string
		tool    string
		params  map[string]any
	}{
		{name: "other product", product: "calendar", tool: "pat.batch_plan", params: map[string]any{"dryRun": true}},
		{name: "other PAT tool", product: "pat", tool: "pat.batch_grant", params: map[string]any{"dryRun": true}},
		{name: "missing request dryRun", product: "pat", tool: "pat.batch_plan", params: map[string]any{}},
		{name: "false request dryRun", product: "pat", tool: "pat.scope_revoke", params: map[string]any{"dryRun": false}},
		{name: "string request dryRun", product: "pat", tool: "pat.scope_revoke", params: map[string]any{"dryRun": "true"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fallback := &countingErrorRunner{}
			runner := &runtimeRunner{globalFlags: &GlobalFlags{DryRun: true}, fallback: fallback}
			invocation := executor.NewHelperInvocation("forged", tt.product, tt.tool, tt.params)
			invocation.DryRun = true
			invocation.AllowReadOnlyDuringDryRun = true
			if isReadOnlyDryRunInvocation(invocation) {
				t.Fatal("forged tuple passed the final read-only predicate")
			}

			result, err := runner.Run(context.Background(), invocation)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result.Response["dry_run"] != true || !result.Invocation.DryRun {
				t.Fatalf("forged marker did not fall back to Echo: %#v", result)
			}
			if result.Invocation.AllowReadOnlyDuringDryRun {
				t.Fatal("forged marker survived the runtime boundary")
			}
			if got := fallback.calls.Load(); got != 0 {
				t.Fatalf("fallback calls = %d, want 0", got)
			}
		})
	}
}

type countingErrorRunner struct {
	calls atomic.Int64
}

func (r *countingErrorRunner) Run(context.Context, executor.Invocation) (executor.Result, error) {
	r.calls.Add(1)
	return executor.Result{}, errors.New("runner must not be called")
}
