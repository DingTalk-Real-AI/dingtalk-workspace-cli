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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
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

func TestToolCallerAdapterDryRunAllowsOnlyExplicitReadCapability(t *testing.T) {
	runner := &readOnlyDryRunRunner{}
	caller := newToolCallerAdapter(runner, &GlobalFlags{DryRun: true, Format: "json"})

	result, err := caller.(edition.ReadToolCaller).CallReadTool(
		context.Background(),
		"im",
		"search_groups",
		map[string]any{"keyword": "project"},
	)
	if err != nil {
		t.Fatalf("CallReadTool() error = %v", err)
	}
	if got := runner.readCalls.Load(); got != 1 {
		t.Fatalf("read calls = %d, want 1", got)
	}
	if got := runner.regularCalls.Load(); got != 0 {
		t.Fatalf("regular calls = %d, want 0", got)
	}
	if runner.invocation.DryRun {
		t.Fatal("read-only invocation was left in dry-run mode")
	}
	if result == nil || len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, `"read":true`) {
		t.Fatalf("read result = %#v", result)
	}

	failClosed := newToolCallerAdapter(&countingErrorRunner{}, &GlobalFlags{DryRun: true})
	if _, err := failClosed.(edition.ReadToolCaller).CallReadTool(
		context.Background(), "im", "search_groups", nil,
	); err == nil {
		t.Fatal("runner without read-only capability was accepted")
	}
}

func TestCrossPlatformCoverageReadOnlyGuardErrorPaths(t *testing.T) {
	var nilAdapter *toolCallerAdapter
	if _, err := nilAdapter.CallReadTool(context.Background(), "im", "search_groups", nil); err == nil {
		t.Fatal("nil adapter accepted a read-only call")
	}

	regularRunner := &capturingSuccessRunner{}
	regular := newToolCallerAdapter(regularRunner, &GlobalFlags{DryRun: false, Format: "json"})
	if _, err := regular.(edition.ReadToolCaller).CallReadTool(context.Background(), "im", "search_groups", nil); err != nil {
		t.Fatalf("non-dry read should use the regular runner: %v", err)
	}
	if got := regularRunner.calls.Load(); got != 1 {
		t.Fatalf("regular runner calls = %d, want 1", got)
	}

	readFailure := newToolCallerAdapter(&failingReadOnlyRunner{}, &GlobalFlags{DryRun: true, Format: "json"})
	if _, err := readFailure.(edition.ReadToolCaller).CallReadTool(context.Background(), "im", "search_groups", nil); err == nil {
		t.Fatal("read-only runner error was swallowed")
	}

	var nilRuntime *runtimeRunner
	if _, err := nilRuntime.RunReadOnly(context.Background(), executor.Invocation{}); err == nil {
		t.Fatal("nil runtime runner accepted a read-only call")
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

func TestRuntimeRunnerReadOnlyClonePreservesGlobalDryRunBarrier(t *testing.T) {
	fallback := &capturingSuccessRunner{}
	flags := &GlobalFlags{DryRun: true}
	runner := &runtimeRunner{globalFlags: flags, fallback: fallback}
	invocation := executor.NewHelperInvocation(
		"test",
		"im",
		"search_groups",
		map[string]any{"keyword": "project"},
	)

	if _, err := runner.RunReadOnly(context.Background(), invocation); err != nil {
		t.Fatalf("RunReadOnly() error = %v", err)
	}
	if got := fallback.calls.Load(); got != 1 {
		t.Fatalf("fallback calls = %d, want 1", got)
	}
	if fallback.invocation.DryRun {
		t.Fatal("read-only fallback invocation was left in dry-run mode")
	}
	if !flags.DryRun {
		t.Fatal("RunReadOnly mutated the process-wide dry-run flag")
	}

	if _, err := runner.Run(context.Background(), invocation); err != nil {
		t.Fatalf("ordinary Run() error = %v", err)
	}
	if got := fallback.calls.Load(); got != 1 {
		t.Fatalf("ordinary dry-run reached fallback; calls = %d", got)
	}
}

type countingErrorRunner struct {
	calls atomic.Int64
}

func (r *countingErrorRunner) Run(context.Context, executor.Invocation) (executor.Result, error) {
	r.calls.Add(1)
	return executor.Result{}, errors.New("runner must not be called")
}

type readOnlyDryRunRunner struct {
	regularCalls atomic.Int64
	readCalls    atomic.Int64
	invocation   executor.Invocation
}

func (r *readOnlyDryRunRunner) Run(context.Context, executor.Invocation) (executor.Result, error) {
	r.regularCalls.Add(1)
	return executor.Result{}, errors.New("regular runner must not be called")
}

func (r *readOnlyDryRunRunner) RunReadOnly(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.readCalls.Add(1)
	r.invocation = invocation
	return executor.Result{
		Invocation: invocation,
		Response:   map[string]any{"read": true},
	}, nil
}

type capturingSuccessRunner struct {
	calls      atomic.Int64
	invocation executor.Invocation
}

func (r *capturingSuccessRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.calls.Add(1)
	r.invocation = invocation
	return executor.Result{
		Invocation: invocation,
		Response:   map[string]any{"read": true},
	}, nil
}

type failingReadOnlyRunner struct{}

func (*failingReadOnlyRunner) Run(context.Context, executor.Invocation) (executor.Result, error) {
	return executor.Result{}, errors.New("regular runner must not be called")
}

func (*failingReadOnlyRunner) RunReadOnly(context.Context, executor.Invocation) (executor.Result, error) {
	return executor.Result{}, errors.New("read failed")
}
