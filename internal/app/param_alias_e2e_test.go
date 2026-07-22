// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type paramAliasToolCall struct {
	server string
	tool   string
	args   map[string]any
}

type paramAliasCaptureCaller struct {
	calls []paramAliasToolCall
}

func (c *paramAliasCaptureCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	copyArgs := make(map[string]any, len(args))
	for key, value := range args {
		copyArgs[key] = value
	}
	c.calls = append(c.calls, paramAliasToolCall{server: server, tool: tool, args: copyArgs})
	text := paramAliasResponseForTool(tool)
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

// paramAliasResponseForTool supplies deterministic, business-shape-valid
// responses for the complete-command equivalence matrix. Most commands only
// print the transport result and need an empty object; smart shortcuts that
// inspect a read response receive the smallest shape that lets their full RunE
// complete without falling back to a validation error.
func paramAliasResponseForTool(tool string) string {
	switch tool {
	case "list_calendar_events":
		return `{"result":{"events":[]}}`
	case "search_mail_users":
		return `{"users":[{"name":"Fixture User","email":"fixture@example.com","id":"fixture-user"}]}`
	case "search_dept_by_keyword":
		return `{"deptList":[{"deptId":1,"name":"Fixture Dept"}]}`
	case "search_groups":
		return `{"result":{"items":[{"openConversationId":"fixture-conversation","title":"Fixture Group"}]}}`
	default:
		return `{}`
	}
}

func (*paramAliasCaptureCaller) Format() string { return "json" }
func (*paramAliasCaptureCaller) DryRun() bool   { return false }
func (*paramAliasCaptureCaller) Fields() string { return "" }
func (*paramAliasCaptureCaller) JQ() string     { return "" }

// paramAliasCaptureRunner covers helpers (currently dev app) that dispatch
// through executor.Runner instead of edition.ToolCaller. Keeping both capture
// boundaries in one call list lets the matrix compare the final request shape
// without knowing which transport adapter a command uses.
type paramAliasCaptureRunner struct {
	caller *paramAliasCaptureCaller
}

func (r *paramAliasCaptureRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	copyArgs := make(map[string]any, len(invocation.Params))
	for key, value := range invocation.Params {
		copyArgs[key] = value
	}
	r.caller.calls = append(r.caller.calls, paramAliasToolCall{
		server: invocation.CanonicalProduct,
		tool:   invocation.Tool,
		args:   copyArgs,
	})
	invocation.Implemented = true
	return executor.Result{Invocation: invocation, Response: map[string]any{}}, nil
}

func executeParamAliasE2E(t *testing.T, caller *paramAliasCaptureCaller, args ...string) (*pipeline.Context, error) {
	t.Helper()
	originalArgs := os.Args
	os.Args = append([]string{"dws"}, args...)
	defer func() { os.Args = originalArgs }()

	originalRunnerFactory := rootNewCommandRunnerWithFlags
	rootNewCommandRunnerWithFlags = func(cli.CatalogLoader, *GlobalFlags) executor.Runner {
		return &paramAliasCaptureRunner{caller: caller}
	}
	root := NewRootCommand()
	rootNewCommandRunnerWithFlags = originalRunnerFactory
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	originalCaller := helpers.GetCaller()
	helpers.InitDeps(caller)
	defer helpers.InitDeps(originalCaller)

	ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
	if err != nil {
		return ctx, err
	}
	return ctx, root.Execute()
}

func TestParamAliasReadCommandFinalPayload(t *testing.T) {
	caller := &paramAliasCaptureCaller{}
	start := "2026-03-10T14:00:00+08:00"
	end := "2026-03-10T18:00:00+08:00"
	ctx, err := executeParamAliasE2E(t, caller,
		"calendar", "event", "list",
		"--date", start,
		"--end-time", end,
		"--calendar", "primary",
		"--max-results", "7",
		"--next-cursor", "cursor-1",
	)
	if err != nil {
		t.Fatalf("calendar alias E2E error = %v", err)
	}
	if len(ctx.Corrections) != 5 {
		t.Fatalf("calendar corrections = %#v, want 5", ctx.Corrections)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "list_calendar_events" {
		t.Fatalf("calendar calls = %#v", caller.calls)
	}
	startMS, _ := cmdutil.ParseISOTimeToMillis("start", start)
	endMS, _ := cmdutil.ParseISOTimeToMillis("end", end)
	want := map[string]any{
		"startTime":  startMS,
		"endTime":    endMS,
		"calendarId": "primary",
		"limit":      7,
		"cursor":     "cursor-1",
	}
	if !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("calendar payload = %#v, want %#v", caller.calls[0].args, want)
	}
}

func TestParamAliasWriteCommandFinalPayload(t *testing.T) {
	caller := &paramAliasCaptureCaller{}
	ctx, err := executeParamAliasE2E(t, caller,
		"chat", "message", "send",
		"--to-user", "D-recipient",
		"--text", "hello alias",
		"--uuid", "alias-e2e",
	)
	if err != nil {
		t.Fatalf("chat write alias E2E error = %v", err)
	}
	if len(ctx.Corrections) != 1 || ctx.Corrections[0].Original != "--to-user" || ctx.Corrections[0].Corrected != "--user" {
		t.Fatalf("chat corrections = %#v", ctx.Corrections)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "send_personal_message" {
		t.Fatalf("chat calls = %#v", caller.calls)
	}
	payload := caller.calls[0].args
	if payload["receiverOpenDingTalkId"] != "D-recipient" || payload["uuid"] != "alias-e2e" || payload["msgType"] != "markdown" {
		t.Fatalf("chat payload identity fields = %#v", payload)
	}
	content, _ := payload["content"].(string)
	if !strings.Contains(content, "hello alias") {
		t.Fatalf("chat payload content = %q", content)
	}
	for _, forbidden := range []string{"user", "to-user", "userId"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("chat payload leaked pre-normalization field %q: %#v", forbidden, payload)
		}
	}
}

func TestParamAliasCanonicalConflictFailsBeforeRunE(t *testing.T) {
	caller := &paramAliasCaptureCaller{}
	for _, args := range [][]string{
		{"calendar", "event", "list", "--date", "2026-03-10", "--start", "2026-03-11"},
		{"calendar", "event", "list", "--start", "2026-03-11", "--date", "2026-03-10"},
	} {
		root := NewRootCommand()
		root.SetArgs(args)
		originalCaller := helpers.GetCaller()
		helpers.InitDeps(caller)
		ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
		helpers.InitDeps(originalCaller)
		var conflict *pipeline.FlagConflictError
		if !stderrors.As(err, &conflict) {
			t.Fatalf("RunPreParseArgs(%v) error = %v, want FlagConflictError (ctx=%#v)", args, err, ctx)
		}
		if conflict.Canonical != "start" || !reflect.DeepEqual(conflict.Spellings, []string{"date", "start"}) {
			t.Fatalf("conflict = %#v", conflict)
		}
	}
	if len(caller.calls) != 0 {
		t.Fatalf("conflicting argv reached RunE/tool dispatch: %#v", caller.calls)
	}
}

func TestAllReviewedParamAliasGuardsReachFinalErrorsWithoutDispatch(t *testing.T) {
	concepts, err := cli.LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}

	guardCounts := map[pipeline.FlagProtection]int{}
	for _, fixture := range concepts.Fixture {
		var wantProtection pipeline.FlagProtection
		var wantReason string
		switch fixture.Expect {
		case "did-you-mean:blocked":
			wantProtection = pipeline.FlagProtectionBlocked
			wantReason = "blocked_flag"
		case "did-you-mean:ambiguous":
			wantProtection = pipeline.FlagProtectionAmbiguous
			wantReason = "ambiguous_flag"
		default:
			continue
		}
		guardCounts[wantProtection]++

		t.Run(fixture.Command+"/"+fixture.Emitted, func(t *testing.T) {
			value := "FIXTURE_VALUE"
			args := append(strings.Fields(fixture.Command), "--"+fixture.Emitted, value)
			caller := &paramAliasCaptureCaller{}
			ctx, executeErr := executeParamAliasE2E(t, caller, args...)

			morphed := cmdutil.Morph(fixture.Emitted)
			if ctx == nil || ctx.ProtectedFlags[morphed] != wantProtection {
				t.Fatalf("guard protection = %#v, want %s for %q", ctx, wantProtection, morphed)
			}
			assertLeftUnchanged(t, ctx, fixture.Emitted, value)

			var appErr *apperrors.Error
			if !stderrors.As(executeErr, &appErr) {
				t.Fatalf("final error = %T %v, want *errors.Error", executeErr, executeErr)
			}
			if appErr.Category != apperrors.CategoryValidation || appErr.Reason != wantReason || apperrors.ExitCode(executeErr) != 3 {
				t.Fatalf("final error contract = category %q reason %q exit %d, want validation/%s/3", appErr.Category, appErr.Reason, apperrors.ExitCode(executeErr), wantReason)
			}
			if !strings.Contains(appErr.Message, "unknown flag: --"+fixture.Emitted) || !strings.Contains(appErr.Message, "See 'dws "+fixture.Command+" --help' for usage.") {
				t.Fatalf("final error message = %q", appErr.Message)
			}
			if !strings.Contains(appErr.Hint, "--"+fixture.Emitted) || !strings.Contains(appErr.Hint, "--help") {
				t.Fatalf("final error hint = %q", appErr.Hint)
			}
			wantAction := "Run 'dws " + fixture.Command + " --help' for valid flags"
			if !reflect.DeepEqual(appErr.Actions, []string{wantAction}) || len(appErr.AvailableFlags) == 0 || appErr.Cause == nil {
				t.Fatalf("final recovery fields = actions %v flags %v cause %v", appErr.Actions, appErr.AvailableFlags, appErr.Cause)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("guarded flag reached RunE/tool dispatch: %#v", caller.calls)
			}
		})
	}

	if guardCounts[pipeline.FlagProtectionBlocked] != 9 || guardCounts[pipeline.FlagProtectionAmbiguous] != 3 {
		t.Fatalf("reviewed guard coverage = blocked %d ambiguous %d, want 9/3", guardCounts[pipeline.FlagProtectionBlocked], guardCounts[pipeline.FlagProtectionAmbiguous])
	}
}

func TestFlagConflictErrorFormattingIsDeterministic(t *testing.T) {
	err := (&pipeline.FlagConflictError{Command: "dws demo", Canonical: "start", Spellings: []string{"start", "date"}}).Error()
	want := `conflicting parameter spellings for --start on "dws demo": --date, --start; pass exactly one spelling`
	if err != want {
		t.Fatalf("FlagConflictError = %q, want %q", err, want)
	}
}
