// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	clierrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageCalendarCommandWiring(t *testing.T) {
	for _, tc := range []struct {
		changes string
		want    []string
	}{
		{"eventCreated", []string{"eventCreated"}},
		{"eventUpdated", []string{"eventUpdated"}},
		{"eventCancelled", []string{"eventCancelled"}},
		{"eventUpdated,eventCreated,eventCancelled,eventUpdated", []string{"eventCancelled", "eventCreated", "eventUpdated"}},
		{"*", []string{"*"}},
		{"", []string{}},
	} {
		t.Run(tc.changes, func(t *testing.T) {
			var captured personalConsumeOptions
			testseam.Swap(t, &eventRunPersonalConsume, func(_ *cobra.Command, opts personalConsumeOptions) error {
				captured = opts
				return nil
			})
			cmd := newEventConsumeCommand()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{personal.EventCalendarEventChanged, "--calendar-ids", "primary,other,primary", "--change-types=" + tc.changes, "--flatten"})
			if err := corecmd.ExecuteForTest(cmd); err != nil {
				t.Fatal(err)
			}
			plan, err := preparePersonalSubscription(personal.Identity{}, captured)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"calendarIds": []string{"other", "primary"}, "changeTypes": tc.want}
			if !captured.Flatten || !reflect.DeepEqual(plan.Request.RuleParam, want) {
				t.Fatalf("calendar plan = %#v, captured = %#v", plan, captured)
			}
		})
	}
}

func TestCrossPlatformCoverageCalendarCommandEmptyFilters(t *testing.T) {
	testseam.Swap(t, &eventRunPersonalConsume, func(_ *cobra.Command, opts personalConsumeOptions) error {
		plan, err := preparePersonalSubscription(personal.Identity{}, opts)
		if err != nil {
			return err
		}
		want := map[string]any{"calendarIds": []string{}, "changeTypes": []string{}}
		if !reflect.DeepEqual(plan.Request.RuleParam, want) {
			t.Fatalf("explicit empty flags lost their empty-array semantics: %#v", plan.Request.RuleParam)
		}
		return nil
	})
	cmd := newEventConsumeCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{personal.EventCalendarEventChanged, "--calendar-ids=", "--change-types="})
	if err := corecmd.ExecuteForTest(cmd); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageCalendarCommandDiscovery(t *testing.T) {
	for _, args := range [][]string{
		{"event", "list", "--category", "calendar", "--format", "json"},
		{"event", "schema", personal.EventCalendarEventChanged, "--flatten", "--format", "json"},
		{"schema", "--cli-path", "event consume", "--compact", "--format", "json"},
	} {
		root := NewRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(io.Discard)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !json.Valid(out.Bytes()) {
			t.Fatalf("%v produced invalid JSON: %s", args, out.String())
		}
		if args[0] == "schema" {
			var doc map[string]any
			_ = json.Unmarshal(out.Bytes(), &doc)
			params := doc["parameters"].(map[string]any)
			for _, name := range []string{"calendar-ids", "change-types"} {
				param, ok := params[name].(map[string]any)
				if !ok || param["type"] != "array" || param["required"] != false {
					t.Fatalf("parameter %s = %#v", name, params[name])
				}
			}
			want := []any{"eventCreated", "eventUpdated", "eventCancelled", "*"}
			if got := params["change-types"].(map[string]any)["enum"]; !reflect.DeepEqual(got, want) {
				t.Fatalf("change-types enum = %#v, want %#v", got, want)
			}
		} else if !strings.Contains(out.String(), personal.EventCalendarEventChanged) {
			t.Fatalf("%v cannot discover calendar event: %s", args, out.String())
		}
		for _, changeType := range []string{"eventCreated", "eventUpdated", "eventCancelled"} {
			if !strings.Contains(out.String(), changeType) {
				t.Fatalf("%v does not describe %s: %s", args, changeType, out.String())
			}
		}
	}
}

func TestCrossPlatformCoverageCalendarOptionsRejectedBeforeSubscription(t *testing.T) {
	identity := personal.Identity{}
	for _, opts := range []personalConsumeOptions{
		{EventKey: personal.EventCalendarEventChanged, QueryCSV: "subject"},
		{EventKey: personal.EventCalendarEventChanged, FilterJSON: `{}`},
		{EventKey: personal.EventCalendarEventChanged, UserID: "someone"},
		{EventKey: personal.EventCalendarEventChanged, CalendarIDs: []string{"primary", "*"}},
		{EventKey: personal.EventCalendarEventChanged, ChangeTypes: []string{"eventDeleted"}},
		{EventKey: personal.EventCalendarEventChanged, ChangeTypes: []string{"eventUpdated", "*"}},
		{EventKey: personal.EventMention, CalendarIDs: []string{}},
		{EventKey: personal.EventTodoTaskCreated, ChangeTypes: []string{}},
	} {
		if _, err := preparePersonalSubscription(identity, opts); err == nil {
			t.Fatalf("accepted invalid options: %#v", opts)
		}
		if err := validatePersonalSubscriptionOptions(opts); err == nil {
			t.Fatalf("dry-run accepted invalid options: %#v", opts)
		}
	}
	// Reuse must reject overrides without even querying a subscription.
	for _, opts := range []personalConsumeOptions{
		{SubscribeID: "existing", CalendarIDs: []string{}},
		{SubscribeID: "existing", ChangeTypes: []string{"eventCreated"}},
	} {
		_, _, _, err := ensurePersonalSubscription(context.Background(), nil, identity, opts)
		if err == nil || !strings.Contains(err.Error(), "reusing --subscribe-id") {
			t.Fatalf("reuse overrides = %v", err)
		}
	}
	for _, args := range [][]string{
		{personal.EventCalendarEventChanged, "--query", "subject"},
		{personal.EventCalendarEventChanged, "--change-types", "eventDeleted"},
		{personal.EventCalendarEventChanged, "--change-types", "eventCancelled,*"},
		{personal.EventCalendarEventChanged, "--subscribe-id", "existing", "--calendar-ids=primary"},
		{personal.EventTodoTaskCreated, "--calendar-ids="},
	} {
		cmd := newEventConsumeCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(args)
		err := corecmd.ExecuteForTest(cmd)
		if err == nil || clierrors.ExitCode(err) != clierrors.ExitCodeValidation {
			t.Fatalf("%v: error = %v, exit = %d", args, err, clierrors.ExitCode(err))
		}
	}
}

func TestCrossPlatformCoverageCalendarMultiOptions(t *testing.T) {
	opts := personalConsumeOptions{EventKeys: []string{personal.EventCalendarEventChanged, personal.EventCardAction}}
	plans, err := preparePersonalMultiOptions(opts)
	if err != nil || len(plans) != 2 {
		t.Fatalf("unfiltered mixed events = %#v, %v", plans, err)
	}
	opts.CalendarIDs = []string{"primary"}
	if _, err := preparePersonalMultiOptions(opts); err == nil {
		t.Fatal("calendar filters silently applied to a non-calendar event")
	}
	var preview bytes.Buffer
	printPersonalMultiDryRun(&preview, consume.Config{}, []personalConsumeOptions{
		{EventKey: personal.EventCalendarEventChanged, CalendarIDs: []string{"primary"}, ChangeTypes: []string{"eventCreated"}},
	})
	if !strings.Contains(preview.String(), `"calendarIds":["primary"]`) || !strings.Contains(preview.String(), `"changeTypes":["eventCreated"]`) {
		t.Fatalf("calendar filter missing from preview: %s", preview.String())
	}
}
