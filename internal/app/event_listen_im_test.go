// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type listenIMFakeReader struct {
	responses map[string]map[string]any
	calls     []string
}

type listenIMErrorReader struct{ err error }

func (r listenIMErrorReader) CallMCPData(string, string, map[string]any) (map[string]any, error) {
	return nil, r.err
}

type listenIMHelperCaller struct {
	text string
	err  error
}

func (c listenIMHelperCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return c.result()
}

func (c listenIMHelperCaller) CallReadTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return c.result()
}

func (c listenIMHelperCaller) result() (*edition.ToolResult, error) {
	if c.err != nil {
		return nil, c.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.text}}}, nil
}

func (listenIMHelperCaller) Format() string { return "json" }
func (listenIMHelperCaller) DryRun() bool   { return false }
func (listenIMHelperCaller) Fields() string { return "" }
func (listenIMHelperCaller) JQ() string     { return "" }

func (f *listenIMFakeReader) CallMCPData(product, tool string, _ map[string]any) (map[string]any, error) {
	key := product + "/" + tool
	f.calls = append(f.calls, key)
	if response, ok := f.responses[key]; ok {
		return response, nil
	}
	return map[string]any{}, nil
}

func TestCrossPlatformCoverageCompileListenIMPlanResolvesGroupAndMapsMultipleEvents(t *testing.T) {
	reader := &listenIMFakeReader{responses: map[string]map[string]any{
		"im/search_groups": {
			"result": []any{map[string]any{"title": "项目群", "openConversationId": "cid-1"}},
		},
	}}
	plan, err := compileListenIMPlan(reader, listenIMOptions{
		Kind:      "group",
		Events:    []string{"message", "reaction", "recall"},
		ChatQuery: "项目群",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{personal.EventInChat, personal.EventReactionGroup, personal.EventRecallGroup}
	if !reflect.DeepEqual(plan.EventKeys, wantKeys) || plan.GroupID != "cid-1" {
		t.Fatalf("plan = %#v, want keys=%v group=cid-1", plan, wantKeys)
	}
	if !reflect.DeepEqual(reader.calls, []string{"im/search_groups"}) {
		t.Fatalf("resolver calls = %#v", reader.calls)
	}
}

func TestCrossPlatformCoverageCompileListenIMPlanReturnsStructuredAmbiguityBeforeSubscription(t *testing.T) {
	reader := &listenIMFakeReader{responses: map[string]map[string]any{
		"contact/search_contact_by_key_word": {
			"result": []any{
				map[string]any{"name": "张三", "userId": "u1"},
				map[string]any{"name": "张三", "userId": "u2"},
			},
		},
	}}
	_, err := compileListenIMPlan(reader, listenIMOptions{
		Kind:      "sender",
		Events:    []string{"message"},
		UserQuery: "张三",
	})
	if err == nil {
		t.Fatal("ambiguous sender unexpectedly compiled")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "resolution_ambiguous" {
		t.Fatalf("ambiguity error = %#v", err)
	}
}

func TestCrossPlatformCoverageEventListenIMCommandDelegatesOneCompiledConsumeLifecycle(t *testing.T) {
	reader := &listenIMFakeReader{responses: map[string]map[string]any{
		"im/search_groups": {
			"result": []any{map[string]any{"title": "项目群", "openConversationId": "cid-1"}},
		},
	}}
	oldReader := eventListenIMReader
	oldRun := eventRunPersonalConsume
	t.Cleanup(func() {
		eventListenIMReader = oldReader
		eventRunPersonalConsume = oldRun
	})
	eventListenIMReader = func() targetresolver.Reader { return reader }
	var captured personalConsumeOptions
	var calls int
	eventRunPersonalConsume = func(_ *cobra.Command, opts personalConsumeOptions) error {
		calls++
		captured = opts
		return nil
	}

	cmd := newEventListenIMCommand()
	cmd.SetArgs([]string{
		"--kind", "group",
		"--events", "message,reaction",
		"--chat-query", "项目群",
		"--max-events", "2",
		"--duration", "30s",
		"--dry-run",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("consume lifecycle calls = %d, want 1", calls)
	}
	if !reflect.DeepEqual(captured.EventKeys, []string{personal.EventInChat, personal.EventReactionGroup}) ||
		captured.GroupID != "cid-1" || !captured.Flatten || !captured.Common.DryRun ||
		captured.Common.MaxEvents != 2 || captured.Common.Duration.String() != "30s" {
		t.Fatalf("captured options = %#v", captured)
	}
}

func TestCrossPlatformCoverageCompileListenIMPlanRejectsIncompatibleKindAndTargets(t *testing.T) {
	reader := &listenIMFakeReader{}
	cases := []listenIMOptions{
		{Kind: "at-me", Events: []string{"reaction"}},
		{Kind: "all-group", Events: []string{"message"}, ChatID: "cid"},
		{Kind: "sender", Events: []string{"message"}},
		{Kind: "group", Events: []string{"message"}, ChatID: "cid", ChatQuery: "群"},
		{Kind: "group", Events: []string{"message", "reaction"}, ChatID: "cid", QueryCSV: "关键词"},
	}
	for _, opts := range cases {
		if _, err := compileListenIMPlan(reader, opts); err == nil {
			t.Errorf("options unexpectedly accepted: %#v", opts)
		}
	}
}

func TestCrossPlatformCoverageListenIMCompletionBranches(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		err  error
		ok   bool
	}{
		{name: "transport", err: errors.New("transport")},
		{name: "empty", text: "  ", ok: true},
		{name: "invalid json", text: "{invalid"},
		{name: "valid", text: `{"result":{"ok":true}}`, ok: true},
	} {
		t.Run("reader "+tc.name, func(t *testing.T) {
			helpers.InitDeps(listenIMHelperCaller{text: tc.text, err: tc.err})
			data, err := (eventTargetReader{}).CallMCPData("im", "search_groups", nil)
			if (err == nil) != tc.ok {
				t.Fatalf("data=%#v error=%v ok=%v", data, err, tc.ok)
			}
		})
	}

	if plan, err := compileListenIMPlan(&listenIMFakeReader{}, listenIMOptions{Events: []string{" MESSAGE ", "message"}}); err != nil || len(plan.EventKeys) != 1 {
		t.Fatalf("default/deduplicated plan = %#v, %v", plan, err)
	}
	if _, err := compileListenIMPlan(&listenIMFakeReader{}, listenIMOptions{Kind: "at-me"}); err == nil {
		t.Fatal("empty event set unexpectedly accepted")
	}
	if _, err := compileListenIMPlan(&listenIMFakeReader{}, listenIMOptions{Kind: "unknown", Events: []string{"message"}}); err == nil {
		t.Fatal("unknown kind unexpectedly accepted")
	}

	reader := &listenIMFakeReader{responses: map[string]map[string]any{
		"contact/search_contact_by_key_word": {
			"result": []any{map[string]any{"name": "甲", "openDingTalkId": "D-user"}},
		},
	}}
	plan, err := compileListenIMPlan(reader, listenIMOptions{Kind: "sender", Events: []string{"message"}, UserQuery: "甲"})
	if err != nil || plan.UserID != "" || plan.OpenDingTalkID != "D-user" {
		t.Fatalf("open-id sender plan = %#v, %v", plan, err)
	}
	wantErr := errors.New("resolution failed")
	if _, err := compileListenIMPlan(listenIMErrorReader{err: wantErr}, listenIMOptions{Kind: "sender", Events: []string{"message"}, UserQuery: "甲"}); !errors.Is(err, wantErr) {
		t.Fatalf("sender resolution error = %v", err)
	}
	if _, err := compileListenIMPlan(listenIMErrorReader{err: wantErr}, listenIMOptions{Kind: "group", Events: []string{"message"}, ChatQuery: "群"}); !errors.Is(err, wantErr) {
		t.Fatalf("group resolution error = %v", err)
	}

	cmd := newEventListenIMCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--kind", "sender"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "event +listen-im") {
		t.Fatalf("command compile error = %v", err)
	}
}

func TestCrossPlatformCoverageEventListenIME2ELifecycleCleansAndRollsBack(t *testing.T) {
	newReader := func() *listenIMFakeReader {
		return &listenIMFakeReader{responses: map[string]map[string]any{
			"im/search_groups": {
				"result": []any{map[string]any{"title": "项目群", "openConversationId": "cid-1"}},
			},
		}}
	}
	installFacade := func(t *testing.T, reader *listenIMFakeReader) {
		t.Helper()
		oldReader := eventListenIMReader
		oldRun := eventRunPersonalConsume
		t.Cleanup(func() {
			eventListenIMReader = oldReader
			eventRunPersonalConsume = oldRun
		})
		eventListenIMReader = func() targetresolver.Reader { return reader }
		eventRunPersonalConsume = runPersonalEventConsume
	}
	installLifecycle := func(t *testing.T) {
		t.Helper()
		restore := installPersonalManySeams(t)
		t.Cleanup(restore)
		t.Setenv("DWS_CONFIG_DIR", t.TempDir())
		personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
			return personal.Identity{
				AccessToken: "token", CorpID: "corp", UserID: "user",
				ClientID: "client", SourceID: "open",
			}, nil
		}
		personalUpsertRunState = func(string, personal.RunState) error { return nil }
		personalValidateConsumeConfig = func(consume.Config) error { return nil }
		personalValidateNoOutputConflict = func(consume.Config, string) error { return nil }
	}

	t.Run("ready then clean every created subscription", func(t *testing.T) {
		reader := newReader()
		installFacade(t, reader)
		installLifecycle(t)
		var created, deleted, removed []string
		personalEnsureSubscription = func(_ context.Context, _ *personal.Client, _ personal.Identity, opts personalConsumeOptions) (*personal.Subscription, string, string, error) {
			created = append(created, opts.EventKey)
			return &personal.Subscription{SubscribeID: "sub-" + opts.EventKey}, opts.EventKey, "group", nil
		}
		personalDeleteSubscription = func(_ *personal.Client, _ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		}
		personalRemoveRunStates = func(_ string, ids []string) error {
			removed = append(removed, ids...)
			return nil
		}
		personalConsumeRunMany = func(_ context.Context, cfg consume.Config, specs []consume.ConsumerSpec) error {
			if len(specs) != 2 || !cfg.Flatten {
				t.Fatalf("consume specs/config = %#v / %#v", specs, cfg)
			}
			fmt.Fprintf(cfg.Stderr, "[event] ready event_count=%d bus_pid=123\n", len(specs))
			return nil
		}

		cmd := newEventListenIMCommand()
		var stderr bytes.Buffer
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{
			"--kind", "group", "--events", "message,reaction",
			"--chat-query", "项目群", "--max-events", "1",
		})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		wantEvents := []string{personal.EventInChat, personal.EventReactionGroup}
		if !reflect.DeepEqual(created, wantEvents) {
			t.Fatalf("created = %#v, want %#v", created, wantEvents)
		}
		wantDeleted := []string{"sub-" + personal.EventReactionGroup, "sub-" + personal.EventInChat}
		if !reflect.DeepEqual(deleted, wantDeleted) || !reflect.DeepEqual(removed, wantDeleted) {
			t.Fatalf("deleted=%#v removed=%#v want=%#v", deleted, removed, wantDeleted)
		}
		if !strings.Contains(stderr.String(), "[event] ready event_count=2") {
			t.Fatalf("missing ready marker: %s", stderr.String())
		}
		if !reflect.DeepEqual(reader.calls, []string{"im/search_groups"}) {
			t.Fatalf("resolver calls = %#v", reader.calls)
		}
	})

	t.Run("second create failure rolls back first without starting consumer", func(t *testing.T) {
		reader := newReader()
		installFacade(t, reader)
		installLifecycle(t)
		wantErr := errors.New("second subscription failed")
		calls := 0
		personalEnsureSubscription = func(_ context.Context, _ *personal.Client, _ personal.Identity, opts personalConsumeOptions) (*personal.Subscription, string, string, error) {
			calls++
			if calls == 2 {
				return nil, "", "", wantErr
			}
			return &personal.Subscription{SubscribeID: "sub-first"}, opts.EventKey, "group", nil
		}
		var deleted, removed []string
		personalDeleteSubscription = func(_ *personal.Client, _ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		}
		personalRemoveRunStates = func(_ string, ids []string) error {
			removed = append(removed, ids...)
			return nil
		}
		personalConsumeRunMany = func(context.Context, consume.Config, []consume.ConsumerSpec) error {
			t.Fatal("consumer started after partial subscription failure")
			return nil
		}

		cmd := newEventListenIMCommand()
		var stderr bytes.Buffer
		cmd.SetErr(&stderr)
		cmd.SilenceUsage = true
		cmd.SetArgs([]string{
			"--kind", "group", "--events", "message,reaction",
			"--chat-query", "项目群",
		})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if !reflect.DeepEqual(deleted, []string{"sub-first"}) || !reflect.DeepEqual(removed, []string{"sub-first"}) {
			t.Fatalf("rollback deleted=%#v removed=%#v", deleted, removed)
		}
	})
}
