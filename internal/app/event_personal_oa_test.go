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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/spf13/cobra"
)

func TestPersonalOAEventListAndSchemaCommands(t *testing.T) {
	list := newEventListCommand()
	list.SilenceUsage = true
	list.SilenceErrors = true
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	list.SetArgs([]string{"--category", "oa"})
	if err := list.Execute(); err != nil {
		t.Fatalf("event list --category oa error = %v", err)
	}
	tests := []struct {
		eventKey   string
		properties []string
	}{
		{
			eventKey: personal.EventOAApprovalTaskCreated,
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "process_instance_id",
				"process_code", "task_id", "title", "status", "create_time", "event_time",
			},
		},
		{
			eventKey: personal.EventOAApprovalTaskFinished,
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "process_instance_id",
				"process_code", "task_id", "title", "status", "result", "create_time",
				"finish_time", "event_time",
			},
		},
		{
			eventKey: personal.EventOAApprovalTaskRedirected,
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "process_instance_id",
				"process_code", "task_id", "title", "status", "result", "create_time",
				"finish_time", "event_time",
			},
		},
		{
			eventKey: personal.EventOAApprovalInstanceStarted,
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "process_instance_id",
				"process_code", "title", "status", "create_time", "event_time",
			},
		},
		{
			eventKey: personal.EventOAApprovalInstanceCC,
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "process_instance_id",
				"process_code", "title", "status", "create_time", "event_time",
			},
		},
		{
			eventKey: personal.EventOAApprovalInstanceTerminated,
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "process_instance_id",
				"process_code", "title", "status", "create_time", "finish_time", "event_time",
			},
		},
		{
			eventKey: personal.EventOAApprovalInstanceFinished,
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "process_instance_id",
				"process_code", "title", "status", "result", "create_time", "finish_time",
				"event_time",
			},
		},
	}
	for _, tt := range tests {
		eventKey := tt.eventKey
		if !strings.Contains(listOut.String(), eventKey) {
			t.Fatalf("OA event list missing %s:\n%s", eventKey, listOut.String())
		}

		schema := newEventSchemaCommand()
		schema.SilenceUsage = true
		schema.SilenceErrors = true
		var schemaOut bytes.Buffer
		schema.SetOut(&schemaOut)
		schema.SetArgs([]string{eventKey, "--flatten"})
		if err := schema.Execute(); err != nil {
			t.Fatalf("event schema %s --flatten error = %v", eventKey, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(schemaOut.Bytes(), &doc); err != nil {
			t.Fatalf("decode schema for %s: %v\n%s", eventKey, err, schemaOut.String())
		}
		if doc["event_key"] != eventKey || doc["rule_type"] != "all" || doc["jq_root_path"] != "." {
			t.Fatalf("schema document for %s = %#v", eventKey, doc)
		}
		schemaBody, ok := doc["schema"].(map[string]any)
		if !ok {
			t.Fatalf("schema body for %s = %#v", eventKey, doc["schema"])
		}
		properties, ok := schemaBody["properties"].(map[string]any)
		if !ok || len(properties) != len(tt.properties) {
			t.Fatalf("schema properties for %s = %#v, want %d fields", eventKey, schemaBody["properties"], len(tt.properties))
		}
		for _, name := range tt.properties {
			if _, ok := properties[name].(map[string]any); !ok {
				t.Fatalf("schema property %s for %s = %#v", name, eventKey, properties[name])
			}
		}
		if _, ok := properties["payload"]; ok {
			t.Fatalf("schema for %s exposed generic payload: %#v", eventKey, properties)
		}
	}
	if strings.Contains(listOut.String(), personal.EventMention) {
		t.Fatalf("OA category list leaked IM event:\n%s", listOut.String())
	}
}

func TestPersonalOAEventConsumeDryRunAndValidation(t *testing.T) {
	oldIdentity := personalResolveEventIdentity
	oldGet := personalGetSubscription
	t.Cleanup(func() {
		personalResolveEventIdentity = oldIdentity
		personalGetSubscription = oldGet
	})
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return personal.Identity{
			AccessToken:  "token",
			LocalSubject: "subject",
			ClientID:     "client",
			SourceID:     "open",
		}, nil
	}
	personalGetSubscription = func(_ *personal.Client, _ context.Context, subscribeID string) (*personal.Subscription, error) {
		switch subscribeID {
		case "oa-sub-task":
			return &personal.Subscription{
				SubscribeID: subscribeID,
				EventKey:    personal.EventOAApprovalTaskCreated,
				RuleType:    "all",
			}, nil
		case "im-sub-at":
			return &personal.Subscription{
				SubscribeID: subscribeID,
				EventKey:    personal.EventMention,
				RuleType:    "at",
			}, nil
		default:
			t.Fatalf("unexpected subscription lookup %q", subscribeID)
			return nil, nil
		}
	}

	oaEvents := []string{
		personal.EventOAApprovalTaskCreated,
		personal.EventOAApprovalTaskFinished,
		personal.EventOAApprovalTaskRedirected,
		personal.EventOAApprovalInstanceStarted,
		personal.EventOAApprovalInstanceCC,
		personal.EventOAApprovalInstanceTerminated,
		personal.EventOAApprovalInstanceFinished,
	}
	for _, eventKey := range oaEvents {
		t.Run(eventKey+"/dry-run", func(t *testing.T) {
			cmd := newEventConsumeCommand()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			var stderr bytes.Buffer
			cmd.SetOut(io.Discard)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{eventKey, "--dry-run"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("OA dry-run error = %v", err)
			}
			if !strings.Contains(stderr.String(), "event_types      : "+eventKey) {
				t.Fatalf("OA dry-run does not select %s:\n%s", eventKey, stderr.String())
			}
		})

		for _, args := range [][]string{
			{"--user", "user-1"},
			{"--open-dingtalk-id", "open-user-1"},
			{"--group", "cid-1"},
			{"--query", "urgent"},
			{"--filter-json", `{"field":"content","op":"eq","value":"urgent"}`},
			{"--role-types", "executor"},
		} {
			name := strings.TrimPrefix(args[0], "--")
			t.Run(eventKey+"/reject-"+name, func(t *testing.T) {
				cmd := newEventConsumeCommand()
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)
				cmd.SetArgs(append([]string{eventKey}, append(args, "--dry-run")...))
				err := cmd.Execute()
				if err == nil || !strings.Contains(err.Error(), "not supported") {
					t.Fatalf("OA consume %s error = %v, want unsupported option", args[0], err)
				}
			})
		}
	}

	t.Run("multi-dry-run", func(t *testing.T) {
		cmd := newEventConsumeCommand()
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		var stderr bytes.Buffer
		cmd.SetOut(io.Discard)
		cmd.SetErr(&stderr)
		cmd.SetArgs(append(append([]string(nil), oaEvents...), "--dry-run"))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("multi OA dry-run error = %v", err)
		}
		for _, eventKey := range oaEvents {
			want := "event_key=" + eventKey + " rule_type=all rule_param={}"
			if !strings.Contains(stderr.String(), want) {
				t.Fatalf("multi OA dry-run missing %q:\n%s", want, stderr.String())
			}
		}
	})

	reuseOverrides := [][]string{
		{"--user", "user-1"},
		{"--open-dingtalk-id", "open-user-1"},
		{"--group", "cid-1"},
		{"--query", "urgent"},
		{"--filter-json", `{"field":"content","op":"eq","value":"urgent"}`},
	}
	t.Run("reuse-dry-run/implicit-event-key/resolves-oa-event", func(t *testing.T) {
		cmd := newEventConsumeCommand()
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		var stderr bytes.Buffer
		cmd.SetOut(io.Discard)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"--subscribe-id", "oa-sub-task", "--dry-run"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("implicit reused OA dry-run error = %v", err)
		}
		if !strings.Contains(stderr.String(), "event_types      : "+personal.EventOAApprovalTaskCreated) {
			t.Fatalf("implicit reused OA dry-run did not resolve event key:\n%s", stderr.String())
		}
	})
	for _, explicitEventKey := range []bool{true, false} {
		mode := "implicit-event-key"
		if explicitEventKey {
			mode = "explicit-event-key"
		}
		for _, override := range reuseOverrides {
			flag := override[0]
			t.Run("reuse-dry-run/"+mode+"/"+strings.TrimPrefix(flag, "--"), func(t *testing.T) {
				args := make([]string, 0, 6)
				if explicitEventKey {
					args = append(args, personal.EventOAApprovalTaskCreated)
				}
				args = append(args, "--subscribe-id", "oa-sub-task", flag, override[1], "--dry-run")
				cmd := newEventConsumeCommand()
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)
				cmd.SetArgs(args)
				err := cmd.Execute()
				if err == nil || !strings.Contains(err.Error(), flag+" not supported for OA event") {
					t.Fatalf("%s reused OA dry-run %s error = %v", mode, flag, err)
				}
			})
		}
	}

	t.Run("reuse-dry-run/implicit-im-remains-supported", func(t *testing.T) {
		cmd := newEventConsumeCommand()
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--subscribe-id", "im-sub-at", "--query", "urgent", "--dry-run"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("implicit reused IM dry-run error = %v", err)
		}
	})

	for _, override := range reuseOverrides {
		flag, value := override[0], override[1]
		t.Run("multi-reject-"+strings.TrimPrefix(flag, "--"), func(t *testing.T) {
			cmd := newEventConsumeCommand()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			args := append([]string(nil), oaEvents...)
			args = append(args, flag, value, "--dry-run")
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "not supported for OA event") {
				t.Fatalf("multi OA consume %s error = %v", flag, err)
			}
		})
	}

	for _, test := range []struct {
		name string
		args []string
	}{
		{
			name: "message query remains supported",
			args: []string{personal.EventMention, "--query", "urgent", "--dry-run"},
		},
		{
			name: "single group lifecycle filter remains supported",
			args: []string{
				personal.EventGroupUpdated,
				"--group", "cid-1",
				"--filter-json", `{"field":"future","op":"eq","value":"value"}`,
				"--dry-run",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := newEventConsumeCommand()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(test.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("existing IM consume behavior changed: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoveragePersonalOAValidationBranches(t *testing.T) {
	invalid := personalConsumeOptions{
		EventKey: personal.EventOAApprovalTaskCreated,
		UserID:   "user-1",
	}
	if err := validatePersonalSubscriptionOptions(invalid); err == nil ||
		!strings.Contains(err.Error(), "--user not supported for OA event") {
		t.Fatalf("validatePersonalSubscriptionOptions() error = %v", err)
	}
	if _, err := preparePersonalSubscription(personal.Identity{}, invalid); err == nil ||
		!strings.Contains(err.Error(), "--user not supported for OA event") {
		t.Fatalf("preparePersonalSubscription() error = %v", err)
	}

	oldGet := personalGetSubscription
	t.Cleanup(func() { personalGetSubscription = oldGet })
	personalGetSubscription = func(*personal.Client, context.Context, string) (*personal.Subscription, error) {
		return &personal.Subscription{
			SubscribeID: "oa-sub-without-event-key",
			RuleType:    "all",
		}, nil
	}
	_, _, _, err := ensurePersonalSubscription(
		context.Background(),
		nil,
		personal.Identity{},
		personalConsumeOptions{
			SubscribeID: "oa-sub-without-event-key",
			EventKey:    personal.EventOAApprovalTaskCreated,
			UserID:      "user-1",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "--user not supported for OA event") {
		t.Fatalf("ensurePersonalSubscription() error = %v", err)
	}
}

func TestPersonalOAMultiConsumeCreatesIndependentAllSubscriptionsOnSharedBus(t *testing.T) {
	restoreMany := installPersonalManySeams(t)
	defer restoreMany()
	oldCreate := personalCreateSubscription
	defer func() { personalCreateSubscription = oldCreate }()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	identity := personal.Identity{
		AccessToken:  "token",
		LocalSubject: "subject",
		ClientID:     "client",
		SourceID:     "open",
	}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return identity, nil
	}
	var requests []personal.CreateSubscriptionRequest
	personalCreateSubscription = func(_ *personal.Client, _ context.Context, req personal.CreateSubscriptionRequest) (*personal.Subscription, error) {
		requests = append(requests, req)
		return &personal.Subscription{SubscribeID: "sub-" + req.EventKey}, nil
	}
	personalEnsureSubscription = ensurePersonalSubscription
	var states []personal.RunState
	personalUpsertRunState = func(_ string, state personal.RunState) error {
		states = append(states, state)
		return nil
	}
	personalDeleteSubscription = func(*personal.Client, context.Context, string) error { return nil }
	personalRemoveRunStates = func(string, []string) error { return nil }
	personalValidateConsumeConfig = func(consume.Config) error { return nil }
	runManyCalls := 0
	var gotSpecs []consume.ConsumerSpec
	personalConsumeRunMany = func(_ context.Context, _ consume.Config, specs []consume.ConsumerSpec) error {
		runManyCalls++
		gotSpecs = append([]consume.ConsumerSpec(nil), specs...)
		return nil
	}

	eventKeys := []string{
		personal.EventOAApprovalTaskCreated,
		personal.EventOAApprovalTaskFinished,
		personal.EventOAApprovalTaskRedirected,
		personal.EventOAApprovalInstanceStarted,
		personal.EventOAApprovalInstanceCC,
		personal.EventOAApprovalInstanceTerminated,
		personal.EventOAApprovalInstanceFinished,
	}
	if err := runPersonalEventConsume(newPersonalCoverageCommand(), personalConsumeOptions{
		EventKeys: eventKeys,
		Flatten:   true,
	}); err != nil {
		t.Fatalf("multi OA consume error = %v", err)
	}
	if runManyCalls != 1 {
		t.Fatalf("RunMany calls = %d, want one shared-bus consume call", runManyCalls)
	}
	if len(requests) != len(eventKeys) || len(states) != len(eventKeys) || len(gotSpecs) != len(eventKeys) {
		t.Fatalf("requests=%d states=%d specs=%d, want %d each", len(requests), len(states), len(gotSpecs), len(eventKeys))
	}
	for i, eventKey := range eventKeys {
		req := requests[i]
		if req.EventKey != eventKey || req.RuleType != "all" || req.RuleParam == nil || len(req.RuleParam) != 0 || req.Filter != nil {
			t.Fatalf("subscription request[%d] = %#v, want %s all/{}", i, req, eventKey)
		}
		if states[i].EventKey != eventKey || states[i].RuleType != "all" {
			t.Fatalf("run state[%d] = %#v", i, states[i])
		}
		wantSpec := consume.ConsumerSpec{
			EventKey:         eventKey,
			EventTypes:       []string{eventKey},
			SubscribeID:      "sub-" + eventKey,
			ReadySubscribeID: "sub-" + eventKey,
		}
		if !reflect.DeepEqual(gotSpecs[i], wantSpec) {
			t.Fatalf("consumer spec[%d] = %#v, want %#v", i, gotSpecs[i], wantSpec)
		}
	}
}

func TestPersonalOAReusedSubscriptionRejectsDefinitionOverridesAtRuntime(t *testing.T) {
	oldGet := personalGetSubscription
	t.Cleanup(func() { personalGetSubscription = oldGet })
	getCalls := 0
	personalGetSubscription = func(*personal.Client, context.Context, string) (*personal.Subscription, error) {
		getCalls++
		return &personal.Subscription{
			SubscribeID: "oa-sub-task",
			EventKey:    personal.EventOAApprovalTaskCreated,
			RuleType:    "all",
		}, nil
	}

	tests := []struct {
		name string
		set  func(*personalConsumeOptions)
	}{
		{name: "user", set: func(opts *personalConsumeOptions) { opts.UserID = "user-1" }},
		{name: "open-dingtalk-id", set: func(opts *personalConsumeOptions) { opts.OpenDingTalkID = "open-user-1" }},
		{name: "group", set: func(opts *personalConsumeOptions) { opts.GroupID = "cid-1" }},
		{name: "query", set: func(opts *personalConsumeOptions) { opts.QueryCSV = "urgent" }},
		{name: "filter-json", set: func(opts *personalConsumeOptions) { opts.FilterJSON = `{"field":"content","op":"eq","value":"urgent"}` }},
	}
	for _, explicitEventKey := range []bool{true, false} {
		mode := "implicit-event-key"
		if explicitEventKey {
			mode = "explicit-event-key"
		}
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				opts := personalConsumeOptions{SubscribeID: "oa-sub-task"}
				if explicitEventKey {
					opts.EventKey = personal.EventOAApprovalTaskCreated
				}
				test.set(&opts)
				before := getCalls
				_, _, _, err := ensurePersonalSubscription(
					context.Background(),
					nil,
					personal.Identity{},
					opts,
				)
				if err == nil || !strings.Contains(err.Error(), "--"+test.name+" not supported for OA event") {
					t.Fatalf("reused OA subscription %s error = %v", test.name, err)
				}
				if getCalls != before+1 {
					t.Fatalf("subscription lookup calls = %d, want %d", getCalls, before+1)
				}
			})
		}
	}
}

func TestPersonalOAImplicitReuseRuntimeLooksUpEventBeforeValidation(t *testing.T) {
	oldIdentity := personalResolveEventIdentity
	oldGet := personalGetSubscription
	t.Cleanup(func() {
		personalResolveEventIdentity = oldIdentity
		personalGetSubscription = oldGet
	})
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return personal.Identity{
			AccessToken:  "token",
			LocalSubject: "subject",
			ClientID:     "client",
			SourceID:     "open",
		}, nil
	}
	getCalls := 0
	personalGetSubscription = func(*personal.Client, context.Context, string) (*personal.Subscription, error) {
		getCalls++
		return &personal.Subscription{
			SubscribeID: "oa-sub-task",
			EventKey:    personal.EventOAApprovalTaskCreated,
			RuleType:    "all",
		}, nil
	}

	cmd := newEventConsumeCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--subscribe-id", "oa-sub-task", "--group", "cid-1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--group not supported for OA event "+personal.EventOAApprovalTaskCreated) {
		t.Fatalf("implicit reused OA runtime error = %v", err)
	}
	if getCalls != 1 {
		t.Fatalf("subscription lookup calls = %d, want 1", getCalls)
	}
}

func TestPersonalIMReusedSubscriptionWithExistingOverridesRemainsSupported(t *testing.T) {
	oldGet := personalGetSubscription
	t.Cleanup(func() { personalGetSubscription = oldGet })
	personalGetSubscription = func(*personal.Client, context.Context, string) (*personal.Subscription, error) {
		return &personal.Subscription{
			SubscribeID: "im-sub",
			EventKey:    personal.EventSingleChat,
			RuleType:    "singleChat",
		}, nil
	}

	sub, eventKey, ruleType, err := ensurePersonalSubscription(
		context.Background(),
		nil,
		personal.Identity{},
		personalConsumeOptions{
			SubscribeID: "im-sub",
			EventKey:    personal.EventSingleChat,
			UserID:      "user-1",
			QueryCSV:    "urgent",
			FilterJSON:  `{"field":"content","op":"eq","value":"urgent"}`,
		},
	)
	if err != nil {
		t.Fatalf("reused IM subscription error = %v", err)
	}
	if sub.SubscribeID != "im-sub" || eventKey != personal.EventSingleChat || ruleType != "singleChat" {
		t.Fatalf("reused IM subscription = %#v, event=%q rule=%q", sub, eventKey, ruleType)
	}
}

func TestPersonalOAStatusAndStopCommandWiring(t *testing.T) {
	oldStatus := eventRunPersonalStatus
	oldStop := eventRunPersonalStop
	t.Cleanup(func() {
		eventRunPersonalStatus = oldStatus
		eventRunPersonalStop = oldStop
	})

	var statusOpts personalStatusOptions
	eventRunPersonalStatus = func(_ *cobra.Command, opts personalStatusOptions) error {
		statusOpts = opts
		return nil
	}
	status := newEventStatusCommand()
	status.SilenceUsage = true
	status.SilenceErrors = true
	status.SetOut(io.Discard)
	status.SetErr(io.Discard)
	status.SetArgs([]string{
		"--event", personal.EventOAApprovalTaskCreated,
		"--subscribe-id", "oa-sub-task",
		"--status", "all",
	})
	if err := status.Execute(); err != nil {
		t.Fatalf("OA event status error = %v", err)
	}
	if statusOpts.EventKey != personal.EventOAApprovalTaskCreated ||
		statusOpts.SubscribeID != "oa-sub-task" ||
		statusOpts.Status != "all" {
		t.Fatalf("OA status options = %#v", statusOpts)
	}

	var stopOpts personalStopOptions
	eventRunPersonalStop = func(_ *cobra.Command, opts personalStopOptions) error {
		stopOpts = opts
		return nil
	}
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	event := &cobra.Command{Use: "event"}
	event.AddCommand(newEventStopCommand())
	root.AddCommand(event)
	root.SetArgs([]string{"event", "stop", "oa-sub-task", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatalf("OA event stop error = %v", err)
	}
	if stopOpts.SubscribeID != "oa-sub-task" || stopOpts.All {
		t.Fatalf("OA stop options = %#v", stopOpts)
	}
}
