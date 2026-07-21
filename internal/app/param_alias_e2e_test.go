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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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
	text := `{}`
	if tool == "list_calendar_events" {
		text = `{"result":{"events":[]}}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*paramAliasCaptureCaller) Format() string { return "json" }
func (*paramAliasCaptureCaller) DryRun() bool   { return false }
func (*paramAliasCaptureCaller) Fields() string { return "" }
func (*paramAliasCaptureCaller) JQ() string     { return "" }

func executeParamAliasE2E(t *testing.T, caller *paramAliasCaptureCaller, args ...string) (*pipeline.Context, error) {
	t.Helper()
	originalArgs := os.Args
	os.Args = append([]string{"dws"}, args...)
	defer func() { os.Args = originalArgs }()

	root := NewRootCommand()
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

func TestParamAliasBlockedFlagReachesReviewedFinalError(t *testing.T) {
	caller := &paramAliasCaptureCaller{}
	ctx, err := executeParamAliasE2E(t, caller,
		"chat", "message", "list-by-sender", "--time", "2026-03-10T00:00:00+08:00",
	)
	if ctx == nil || ctx.ProtectedFlags["time"] != pipeline.FlagProtectionBlocked {
		t.Fatalf("blocked flag did not survive PreParse: %#v", ctx)
	}
	var appErr *apperrors.Error
	if !stderrors.As(err, &appErr) || appErr.Reason != "blocked_flag" || !strings.Contains(appErr.Hint, "--help") {
		t.Fatalf("blocked final error = %T %v", err, err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("blocked flag reached RunE/tool dispatch: %#v", caller.calls)
	}
}

func TestFlagConflictErrorFormattingIsDeterministic(t *testing.T) {
	err := (&pipeline.FlagConflictError{Command: "dws demo", Canonical: "start", Spellings: []string{"start", "date"}}).Error()
	want := `conflicting parameter spellings for --start on "dws demo": --date, --start; pass exactly one spelling`
	if err != want {
		t.Fatalf("FlagConflictError = %q, want %q", err, want)
	}
}
