// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// TestSportDryRunEnvelopeUsesRealRoot 覆盖实际 app root、adapter 与输出信封，
// 防止只有 fake caller 返回预览而真实 CLI 输出 JSON-RPC 回显的假绿。
func TestSportDryRunEnvelopeUsesRealRoot(t *testing.T) {
	setTestHome(t, t.TempDir())
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	cases := []struct {
		name      string
		args      []string
		tool      string
		arguments map[string]any
	}{
		{"开通状态", []string{"sport", "org", "status"}, "get_org_step_participate_status", map[string]any{}},
		{
			"创建比赛",
			[]string{"sport", "competition", "create", "--user-id", "fixture-user", "--name", "比赛预览",
				"--sport-type", "WALK", "--competition-type", "FIXED", "--start-date", "20260430", "--end-date", "20260501",
				"--goal-value", "8000", "--target-day-value", "2", "--group-id", "fixture-group",
				"--need-signup", "false", "--allow-free-join", "false", "--cover-image-url", "https://example.test/cover.png",
				"--daily-limit", "9000", "--open-conversation-id", "fixture-conversation"},
			"create_competition",
			map[string]any{
				"userId": "fixture-user",
				"CompetitionCreateMcpRequest": map[string]any{
					"competitionName": "比赛预览", "sportType": "WALK", "competitionType": "FIXED",
					"firstPeriodStartDate": "20260430", "firstPeriodEndDate": "20260501", "goalValue": "8000", "targetDayValue": "2",
					"groupId": "fixture-group", "needSignup": "false", "allowFreeJoin": "false",
					"coverImageUrl": "https://example.test/cover.png", "dailyLimit": "9000", "openConversationId": "fixture-conversation",
				},
			},
		},
		{
			"开赛播报", []string{"sport", "competition", "announce", "--user-id", "fixture-user", "--rule-code", "fixture-rule"},
			"send_competition_group_msg", map[string]any{"userId": "fixture-user", "ruleCode": "fixture-rule"},
		},
	}
	for _, tc := range cases {
		for _, prefix := range []bool{true, false} {
			position := "后置flag"
			if prefix {
				position = "前置flag"
			}
			t.Run(tc.name+"/"+position, func(t *testing.T) {
				helpers.InitDepsForTest(t, helpers.GetCaller())
				runner := &countingErrorRunner{}
				testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(*GlobalFlags) executor.Runner { return runner })
				root := NewRootCommand()
				var stdout, stderr bytes.Buffer
				root.SetOut(&stdout)
				root.SetErr(&stderr)
				args := append([]string{}, tc.args...)
				if prefix {
					args = append([]string{"--dry-run"}, args...)
				} else {
					args = append(args, "--dry-run")
				}
				root.SetArgs(append(args, "--format", "json"))
				if err := root.Execute(); err != nil {
					t.Fatalf("真实 root 预览失败：%v\n%s", err, stderr.String())
				}
				if runner.calls.Load() != 0 {
					t.Fatal("预览不得调用业务 Runner")
				}
				var envelope output.Envelope
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatalf("解析真实输出失败：%v\n%s", err, stdout.String())
				}
				if !envelope.OK || !envelope.DryRun || envelope.Outcome != output.OutcomeSuccess {
					t.Fatalf("预览信封缺少顶层 dry_run 或成功状态：%s", stdout.String())
				}
				want := map[string]any{"tool": tc.tool, "arguments": tc.arguments, "executed": false}
				if !reflect.DeepEqual(envelope.Data, want) {
					t.Fatalf("预览必须只含待调用参数，不应暴露 JSON-RPC 包装：got=%v want=%v", envelope.Data, want)
				}
			})
		}
	}
}

func TestSportDryRunSchemaCapabilityDelivery(t *testing.T) {
	helpers.InitDepsForTest(t, helpers.GetCaller())
	paths := []string{
		"sport org status", "sport step org-info", "sport step org-total",
		"sport step emp-ranking", "sport step dept-ranking", "sport step dept-emp-ranking",
		"sport step emp-info", "sport step emp-infos", "sport step dept-infos",
		"sport competition create", "sport competition config", "sport competition rule-detail",
		"sport competition instances", "sport competition current-instance", "sport competition next-instance", "sport competition announce",
		"sport stats signup", "sport stats participants-daily", "sport stats participants", "sport stats reached", "sport stats trend",
	}
	// DryRunSpec 的 remote_reads=false 使用 omitempty，不发布多余的零值字段。
	want := map[string]any{"preview_kind": "request"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			full := executeShortcutSchemaQuery(t, "--cli-path", path)
			compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
			for _, payload := range []map[string]any{full, compact} {
				if !reflect.DeepEqual(payload["dry_run"], want) {
					t.Fatalf("%s 的最终 Schema 预览契约不符：got=%v want=%v", path, payload["dry_run"], want)
				}
			}
		})
	}
}

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
