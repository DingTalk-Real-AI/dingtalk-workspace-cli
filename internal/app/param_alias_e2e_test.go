// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"os"
	"reflect"
	"sort"
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

type paramAliasDryRunRejectRunner struct {
	attempts []executor.Invocation
}

func (r *paramAliasDryRunRejectRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.attempts = append(r.attempts, invocation)
	return executor.Result{}, stderrors.New("dry-run reached the injected command runner")
}

type paramAliasDryRunPreview struct {
	DryRun    bool           `json:"dry_run"`
	Executed  bool           `json:"executed"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

// executeParamAliasDryRunE2E uses the existing root --dry-run barrier as a
// parameter-normalization probe. These commands do not publish command-owned
// dry-run capabilities in Schema; the test deliberately makes no such claim.
// A reject runner proves the preview stops before endpoint resolution,
// authentication, or transport execution.
func executeParamAliasDryRunE2E(t *testing.T, args ...string) (*pipeline.Context, paramAliasDryRunPreview, []executor.Invocation, error) {
	t.Helper()

	originalArgs := os.Args
	os.Args = append([]string{"dws"}, args...)
	defer func() { os.Args = originalArgs }()

	captureFile, err := os.CreateTemp(t.TempDir(), "param-alias-dry-run-*.json")
	if err != nil {
		t.Fatalf("create dry-run output capture: %v", err)
	}
	defer captureFile.Close()
	originalStdout := os.Stdout
	originalCaller := helpers.GetCaller()
	os.Stdout = captureFile
	defer func() {
		os.Stdout = originalStdout
		helpers.InitDeps(originalCaller)
	}()
	rejectRunner := &paramAliasDryRunRejectRunner{}
	originalRunnerFactory := rootNewCommandRunnerWithFlags
	rootNewCommandRunnerWithFlags = func(cli.CatalogLoader, *GlobalFlags) executor.Runner {
		return rejectRunner
	}
	root := NewRootCommand()
	rootNewCommandRunnerWithFlags = originalRunnerFactory
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)

	ctx, executeErr := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
	if executeErr == nil {
		executeErr = root.Execute()
	}

	if err := captureFile.Sync(); err != nil {
		t.Fatalf("sync dry-run output capture: %v", err)
	}
	if _, err := captureFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind dry-run output capture: %v", err)
	}
	output, err := io.ReadAll(captureFile)
	if err != nil {
		t.Fatalf("read dry-run output capture: %v", err)
	}
	var preview paramAliasDryRunPreview
	if executeErr == nil {
		if err := json.Unmarshal(output, &preview); err != nil {
			t.Fatalf("decode dry-run preview: %v\noutput=%s", err, output)
		}
	}
	return ctx, preview, append([]executor.Invocation(nil), rejectRunner.attempts...), executeErr
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
	if len(ctx.Corrections) != 1 || ctx.Corrections[0].Original != "--date" || ctx.Corrections[0].Corrected != "--start" {
		t.Fatalf("calendar corrections = %#v, want only --date to be normalized centrally", ctx.Corrections)
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

func TestChatReactionConversationAliasesReachCanonicalPayload(t *testing.T) {
	tests := []struct {
		name     string
		command  []string
		tool     string
		required []string
	}{
		{
			name:     "add emoji",
			command:  []string{"chat", "message", "add-emoji"},
			tool:     "add_emoji_reaction",
			required: []string{"--msg-id", "message-1", "--emoji", "like"},
		},
		{
			name:     "remove emoji",
			command:  []string{"chat", "message", "remove-emoji"},
			tool:     "remove_emoji_reaction",
			required: []string{"--msg-id", "message-1", "--emoji", "like"},
		},
		{
			name:    "add text emotion",
			command: []string{"chat", "message", "add-text-emotion"},
			tool:    "add_text_emotion",
			required: []string{
				"--msg-id", "message-1", "--emotion-id", "emotion-1",
				"--emotion-name", "like", "--text", "nice", "--background-id", "background-1",
			},
		},
		{
			name:    "remove text emotion",
			command: []string{"chat", "message", "remove-text-emotion"},
			tool:    "remove_text_emotion",
			required: []string{
				"--msg-id", "message-1", "--emotion-id", "emotion-1",
				"--emotion-name", "like", "--text", "nice", "--background-id", "background-1",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			canonicalArgs := append([]string(nil), test.command...)
			canonicalArgs = append(canonicalArgs, "--conversation-id", "conversation-1")
			canonicalArgs = append(canonicalArgs, test.required...)
			canonicalCaller := &paramAliasCaptureCaller{}
			if _, err := executeParamAliasE2E(t, canonicalCaller, canonicalArgs...); err != nil {
				t.Fatalf("canonical execution failed: %v", err)
			}
			if len(canonicalCaller.calls) != 1 || canonicalCaller.calls[0].tool != test.tool {
				t.Fatalf("canonical calls = %#v, want one %s call", canonicalCaller.calls, test.tool)
			}
			if canonicalCaller.calls[0].args["openConversationId"] != "conversation-1" {
				t.Fatalf("canonical payload = %#v", canonicalCaller.calls[0].args)
			}

			for _, alias := range []string{"group-id", "chat-id", "open-conversation-id"} {
				t.Run(alias, func(t *testing.T) {
					aliasArgs := append([]string(nil), test.command...)
					aliasArgs = append(aliasArgs, "--"+alias, "conversation-1")
					aliasArgs = append(aliasArgs, test.required...)
					aliasCaller := &paramAliasCaptureCaller{}
					ctx, err := executeParamAliasE2E(t, aliasCaller, aliasArgs...)
					if err != nil {
						t.Fatalf("alias execution failed: %v", err)
					}
					if ctx == nil || len(ctx.Corrections) != 1 || ctx.Corrections[0].Original != "--"+alias || ctx.Corrections[0].Corrected != "--conversation-id" {
						t.Fatalf("alias corrections = %#v", ctx)
					}
					if !reflect.DeepEqual(aliasCaller.calls, canonicalCaller.calls) {
						t.Fatalf("final calls differ\ncanonical=%#v\nalias=%#v", canonicalCaller.calls, aliasCaller.calls)
					}
				})
			}
		})
	}
}

func TestSelectedParamAliasesProduceCanonicalEquivalentDryRunPreviews(t *testing.T) {
	tests := []struct {
		name            string
		tool            string
		canonicalArgs   []string
		aliasArgs       []string
		wantCorrections int
		wantArgKeys     []string
	}{
		{
			name: "calendar read with multiple aliases",
			tool: "list_calendar_events",
			canonicalArgs: []string{
				"--dry-run", "calendar", "event", "list",
				"--start", "2026-03-10T14:00:00+08:00",
				"--end", "2026-03-10T18:00:00+08:00",
				"--calendar-id", "primary", "--limit", "7", "--cursor", "cursor-1",
			},
			aliasArgs: []string{
				"--dry-run", "calendar", "event", "list",
				"--date", "2026-03-10T14:00:00+08:00",
				"--end-time", "2026-03-10T18:00:00+08:00",
				"--calendar", "primary", "--max-results", "7", "--next-cursor", "cursor-1",
			},
			wantCorrections: 1,
			wantArgKeys:     []string{"calendarId", "cursor", "endTime", "limit", "startTime"},
		},
		{
			name: "chat write scoped recipient alias",
			tool: "send_personal_message",
			canonicalArgs: []string{
				"--dry-run", "chat", "message", "send",
				"--user", "D-recipient", "--text", "hello dry-run", "--uuid", "alias-dry-run",
			},
			aliasArgs: []string{
				"--dry-run", "chat", "message", "send",
				"--to-user", "D-recipient", "--text", "hello dry-run", "--uuid", "alias-dry-run",
			},
			wantCorrections: 1,
			wantArgKeys:     []string{"clawType", "content", "msgType", "receiverOpenDingTalkId", "uuid"},
		},
		{
			name: "mail write folder id concept alias",
			tool: "update_mail_folder",
			canonicalArgs: []string{
				"--dry-run", "mail", "folder", "update",
				"--email", "fixture@example.com", "--id", "folder-1", "--name", "Fixture Folder",
			},
			aliasArgs: []string{
				"--dry-run", "mail", "folder", "update",
				"--email", "fixture@example.com", "--folder-id", "folder-1", "--name", "Fixture Folder",
			},
			wantCorrections: 1,
			wantArgKeys:     []string{"email", "id", "name"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, canonical, canonicalAttempts, canonicalErr := executeParamAliasDryRunE2E(t, test.canonicalArgs...)
			if canonicalErr != nil {
				t.Fatalf("canonical dry-run failed: %v", canonicalErr)
			}
			ctx, alias, aliasAttempts, aliasErr := executeParamAliasDryRunE2E(t, test.aliasArgs...)
			if aliasErr != nil {
				t.Fatalf("alias dry-run failed: %v\ncontext=%#v", aliasErr, ctx)
			}

			if ctx == nil || len(ctx.Corrections) != test.wantCorrections {
				t.Fatalf("alias dry-run corrections = %#v, want %d", ctx, test.wantCorrections)
			}
			if len(canonicalAttempts) != 0 || len(aliasAttempts) != 0 {
				t.Fatalf("dry-run reached command runner\ncanonical=%#v\nalias=%#v", canonicalAttempts, aliasAttempts)
			}
			for label, preview := range map[string]paramAliasDryRunPreview{"canonical": canonical, "alias": alias} {
				if !preview.DryRun || preview.Executed {
					t.Fatalf("%s preview execution state = %#v", label, preview)
				}
				if preview.Tool != test.tool {
					t.Fatalf("%s preview tool = %q, want %q", label, preview.Tool, test.tool)
				}
				keys := make([]string, 0, len(preview.Arguments))
				for key := range preview.Arguments {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				if !reflect.DeepEqual(keys, test.wantArgKeys) {
					t.Fatalf("%s preview argument keys = %v, want %v", label, keys, test.wantArgKeys)
				}
			}
			if !reflect.DeepEqual(alias, canonical) {
				t.Fatalf("dry-run previews differ\ncanonical=%#v\nalias=%#v", canonical, alias)
			}
		})
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

	paths := make(map[string]bool)
	for _, concept := range concepts.Concepts {
		for _, path := range concept.Commands {
			paths[path] = true
		}
	}
	sourceGuards := make(map[string]pipeline.FlagProtection)
	for _, override := range concepts.Overrides {
		paths[override.CommandPath] = true
		for _, emitted := range override.Block {
			sourceGuards[override.CommandPath+"\x00"+cmdutil.Morph(emitted)] = pipeline.FlagProtectionBlocked
		}
		for _, emitted := range override.Ambiguous {
			sourceGuards[override.CommandPath+"\x00"+cmdutil.Morph(emitted)] = pipeline.FlagProtectionAmbiguous
		}
	}
	orderedPaths := make([]string, 0, len(paths))
	for path := range paths {
		orderedPaths = append(orderedPaths, path)
	}
	sort.Strings(orderedPaths)

	guardCounts := map[pipeline.FlagProtection]int{}
	testedGuards := make(map[string]pipeline.FlagProtection)
	for _, path := range orderedPaths {
		entry, ok := cli.LookupParamAlias(path)
		if !ok {
			continue
		}

		for _, protectionCase := range []struct {
			protection pipeline.FlagProtection
			reason     string
			emitted    []string
		}{
			{protection: pipeline.FlagProtectionBlocked, reason: "blocked_flag", emitted: entry.Blocked},
			{protection: pipeline.FlagProtectionAmbiguous, reason: "ambiguous_flag", emitted: entry.Ambiguous},
		} {
			for _, emitted := range protectionCase.emitted {
				protectionCase := protectionCase
				emitted := emitted
				key := path + "\x00" + cmdutil.Morph(emitted)
				if previous, duplicate := testedGuards[key]; duplicate {
					t.Fatalf("generated guard %q/%q is classified twice: %s and %s", path, emitted, previous, protectionCase.protection)
				}
				testedGuards[key] = protectionCase.protection
				guardCounts[protectionCase.protection]++

				t.Run(path+"/"+emitted, func(t *testing.T) {
					value := "FIXTURE_VALUE"
					args := append(strings.Fields(path), "--"+emitted, value)
					caller := &paramAliasCaptureCaller{}
					ctx, executeErr := executeParamAliasE2E(t, caller, args...)

					morphed := cmdutil.Morph(emitted)
					if ctx == nil || ctx.ProtectedFlags[morphed] != protectionCase.protection {
						t.Fatalf("guard protection = %#v, want %s for %q", ctx, protectionCase.protection, morphed)
					}
					assertLeftUnchanged(t, ctx, emitted, value)

					var appErr *apperrors.Error
					if !stderrors.As(executeErr, &appErr) {
						t.Fatalf("final error = %T %v, want *errors.Error", executeErr, executeErr)
					}
					if appErr.Category != apperrors.CategoryValidation || appErr.Reason != protectionCase.reason || apperrors.ExitCode(executeErr) != 3 {
						t.Fatalf("final error contract = category %q reason %q exit %d, want validation/%s/3", appErr.Category, appErr.Reason, apperrors.ExitCode(executeErr), protectionCase.reason)
					}
					if !strings.Contains(appErr.Message, "unknown flag: --"+emitted) || !strings.Contains(appErr.Message, "See 'dws "+path+" --help' for usage.") {
						t.Fatalf("final error message = %q", appErr.Message)
					}
					if !strings.Contains(appErr.Hint, "--"+emitted) || !strings.Contains(appErr.Hint, "--help") {
						t.Fatalf("final error hint = %q", appErr.Hint)
					}
					wantAction := "Run 'dws " + path + " --help' for valid flags"
					if !reflect.DeepEqual(appErr.Actions, []string{wantAction}) || len(appErr.AvailableFlags) == 0 || appErr.Cause == nil {
						t.Fatalf("final recovery fields = actions %v flags %v cause %v", appErr.Actions, appErr.AvailableFlags, appErr.Cause)
					}
					if len(caller.calls) != 0 {
						t.Fatalf("guarded flag reached RunE/tool dispatch: %#v", caller.calls)
					}
				})
			}
		}
	}

	for key, want := range sourceGuards {
		if got, ok := testedGuards[key]; !ok || got != want {
			t.Fatalf("reviewed source guard %q delivered as %s (present=%t), want %s", key, got, ok, want)
		}
	}
	if guardCounts[pipeline.FlagProtectionBlocked] == 0 || guardCounts[pipeline.FlagProtectionAmbiguous] == 0 {
		t.Fatalf("reviewed guard coverage is vacuous: blocked %d ambiguous %d", guardCounts[pipeline.FlagProtectionBlocked], guardCounts[pipeline.FlagProtectionAmbiguous])
	}
}

func TestFlagConflictErrorFormattingIsDeterministic(t *testing.T) {
	err := (&pipeline.FlagConflictError{Command: "dws demo", Canonical: "start", Spellings: []string{"start", "date"}}).Error()
	want := `conflicting parameter spellings for --start on "dws demo": --date, --start; pass exactly one spelling`
	if err != want {
		t.Fatalf("FlagConflictError = %q, want %q", err, want)
	}
}
