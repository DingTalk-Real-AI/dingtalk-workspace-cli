// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/spf13/cobra"
)

func TestEventConsumeAcceptsOrderedVariadicEventKeys(t *testing.T) {
	oldRun := eventRunPersonalConsume
	defer func() { eventRunPersonalConsume = oldRun }()
	var got personalConsumeOptions
	eventRunPersonalConsume = func(_ *cobra.Command, opts personalConsumeOptions) error {
		got = opts
		return nil
	}

	cmd := newEventConsumeCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		personal.EventMention,
		personal.EventSingleChat,
		personal.EventMention,
		"--user", "test-user-001",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{personal.EventMention, personal.EventSingleChat}
	if !reflect.DeepEqual(got.EventKeys, want) || got.EventKey != personal.EventMention {
		t.Fatalf("event keys = %#v, first = %q", got.EventKeys, got.EventKey)
	}
}

func TestPreparePersonalMultiOptionsCombinationMatrix(t *testing.T) {
	tests := []struct {
		name    string
		opts    personalConsumeOptions
		wantErr string
	}{
		{
			name: "no target events",
			opts: personalConsumeOptions{EventKeys: []string{personal.EventMention, personal.EventAllSingleChat}},
		},
		{
			name: "user and no target",
			opts: personalConsumeOptions{
				EventKeys: []string{personal.EventSingleChat, personal.EventReadO2O, personal.EventMention},
				UserID:    "test-user-001",
			},
		},
		{
			name: "group and no target",
			opts: personalConsumeOptions{
				EventKeys: []string{personal.EventInChat, personal.EventGroupUpdated, personal.EventMention},
				GroupID:   "cid-test",
			},
		},
		{
			name: "open dingtalk id",
			opts: personalConsumeOptions{
				EventKeys:      []string{personal.EventSingleChat, personal.EventRecallO2O},
				OpenDingTalkID: "open-test-user",
			},
		},
		{
			name: "user and group mixed",
			opts: personalConsumeOptions{
				EventKeys: []string{personal.EventSingleChat, personal.EventInChat},
				UserID:    "test-user-001",
			},
			wantErr: "cannot be consumed in one command",
		},
		{
			name:    "missing user target",
			opts:    personalConsumeOptions{EventKeys: []string{personal.EventSingleChat, personal.EventReadO2O}},
			wantErr: "one of --user or --open-dingtalk-id",
		},
		{
			name: "missing group target",
			opts: personalConsumeOptions{
				EventKeys: []string{personal.EventInChat, personal.EventGroupUpdated},
			},
			wantErr: "--group is required",
		},
		{
			name: "target on no target events",
			opts: personalConsumeOptions{
				EventKeys: []string{personal.EventMention, personal.EventAllSingleChat},
				UserID:    "test-user-001",
			},
			wantErr: "do not use --user",
		},
		{
			name: "filter message events",
			opts: personalConsumeOptions{
				EventKeys: []string{personal.EventMention, personal.EventAllGroupChat},
				QueryCSV:  "alarm",
			},
		},
		{
			name: "filter mixed with action",
			opts: personalConsumeOptions{
				EventKeys: []string{personal.EventSingleChat, personal.EventReadO2O},
				UserID:    "test-user-001",
				QueryCSV:  "alarm",
			},
			wantErr: "require all selected events to be message receive events",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plans, err := preparePersonalMultiOptions(test.opts)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("preparePersonalMultiOptions() error = %v", err)
			}
			if len(plans) != len(test.opts.EventKeys) {
				t.Fatalf("plans = %d, want %d", len(plans), len(test.opts.EventKeys))
			}
			for _, plan := range plans {
				def, _ := personal.Lookup(plan.EventKey)
				if def.RuleType == "at" || def.RuleType == "all" {
					if plan.UserID != "" || plan.OpenDingTalkID != "" || plan.GroupID != "" {
						t.Fatalf("no-target plan retained target: %#v", plan)
					}
				}
			}
		})
	}
}

func TestPreparePersonalMultiOptionsRejectsSingleOnlyFlags(t *testing.T) {
	base := personalConsumeOptions{EventKeys: []string{personal.EventMention, personal.EventAllSingleChat}}
	tests := []struct {
		name string
		set  func(*personalConsumeOptions)
	}{
		{name: "subscribe-id", set: func(o *personalConsumeOptions) { o.SubscribeID = "sub" }},
		{name: "rule", set: func(o *personalConsumeOptions) { o.Rule = "all" }},
		{name: "event-types", set: func(o *personalConsumeOptions) { o.Common.EventTypes = []string{"x"} }},
		{name: "filter", set: func(o *personalConsumeOptions) { o.Common.Filter = "x" }},
		{name: "foreground", set: func(o *personalConsumeOptions) { o.Common.Foreground = true }},
		{name: "force", set: func(o *personalConsumeOptions) { o.Common.Force = true }},
		{name: "debug-raw-events", set: func(o *personalConsumeOptions) { o.DebugRawEvents = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := base
			test.set(&opts)
			if _, err := preparePersonalMultiOptions(opts); err == nil {
				t.Fatal("option succeeded")
			}
		})
	}
}

func TestEventConsumeMultiRejectsExplicitSingleOnlyFlagsEvenWhenEmpty(t *testing.T) {
	oldRun := eventRunPersonalConsume
	defer func() { eventRunPersonalConsume = oldRun }()
	eventRunPersonalConsume = func(*cobra.Command, personalConsumeOptions) error {
		t.Fatal("personal consume ran after explicit multi-event flag")
		return nil
	}

	flags := []string{
		"--subscribe-id=",
		"--rule=",
		"--event-types=",
		"--filter=",
		"--foreground=false",
		"--force=false",
		"--debug-raw-events=false",
	}
	for _, flag := range flags {
		t.Run(flag, func(t *testing.T) {
			cmd := newEventConsumeCommand()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{personal.EventMention, personal.EventAllSingleChat, flag})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "not supported when consuming multiple events") {
				t.Fatalf("Execute() error = %v", err)
			}
		})
	}
}

func TestRunPersonalEventConsumeManyCreatesAndCleansAllSubscriptions(t *testing.T) {
	restore := installPersonalManySeams(t)
	defer restore()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	identity := personal.Identity{AccessToken: "token", CorpID: "corp", UserID: "user", ClientID: "client", SourceID: "open"}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) { return identity, nil }
	createdKeys := make([]string, 0, 2)
	personalEnsureSubscription = func(_ context.Context, _ *personal.Client, _ personal.Identity, opts personalConsumeOptions) (*personal.Subscription, string, string, error) {
		createdKeys = append(createdKeys, opts.EventKey)
		return &personal.Subscription{SubscribeID: "sub-" + opts.EventKey}, opts.EventKey, "all", nil
	}
	var states []personal.RunState
	personalUpsertRunState = func(_ string, state personal.RunState) error {
		states = append(states, state)
		return nil
	}
	var deleted []string
	personalDeleteSubscription = func(_ *personal.Client, _ context.Context, id string) error {
		deleted = append(deleted, id)
		return nil
	}
	var removed []string
	personalRemoveRunStates = func(_ string, ids []string) error {
		removed = append(removed, ids...)
		return nil
	}
	personalValidateConsumeConfig = func(consume.Config) error { return nil }
	personalConsumeRunMany = func(_ context.Context, cfg consume.Config, specs []consume.ConsumerSpec) error {
		if !cfg.Flatten || cfg.Projector == nil || len(specs) != 2 {
			t.Fatalf("consume config/specs = %#v / %#v", cfg, specs)
		}
		for i, spec := range specs {
			if spec.EventKey != createdKeys[i] || spec.SubscribeID != "sub-"+createdKeys[i] || !reflect.DeepEqual(spec.EventTypes, []string{createdKeys[i]}) {
				t.Fatalf("spec[%d] = %#v", i, spec)
			}
		}
		return nil
	}

	cmd := newPersonalCoverageCommand()
	err := runPersonalEventConsume(cmd, personalConsumeOptions{
		EventKeys: []string{personal.EventMention, personal.EventAllSingleChat},
		Flatten:   true,
	})
	if err != nil {
		t.Fatalf("runPersonalEventConsume() error = %v", err)
	}
	if len(states) != 2 || len(deleted) != 2 || len(removed) != 2 {
		t.Fatalf("states=%#v deleted=%#v removed=%#v", states, deleted, removed)
	}
}

func TestRunPersonalEventConsumeManyRollsBackPartialCreation(t *testing.T) {
	restore := installPersonalManySeams(t)
	defer restore()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return personal.Identity{AccessToken: "token", ClientID: "client", SourceID: "open", LocalSubject: "subject"}, nil
	}
	wantErr := errors.New("second subscription failed")
	calls := 0
	personalEnsureSubscription = func(_ context.Context, _ *personal.Client, _ personal.Identity, opts personalConsumeOptions) (*personal.Subscription, string, string, error) {
		calls++
		if calls == 2 {
			return nil, "", "", wantErr
		}
		return &personal.Subscription{SubscribeID: "sub-first"}, opts.EventKey, "all", nil
	}
	personalUpsertRunState = func(string, personal.RunState) error { return nil }
	var deleted []string
	personalDeleteSubscription = func(_ *personal.Client, _ context.Context, id string) error {
		deleted = append(deleted, id)
		return nil
	}
	var removed []string
	personalRemoveRunStates = func(_ string, ids []string) error {
		removed = append(removed, ids...)
		return nil
	}
	personalValidateConsumeConfig = func(consume.Config) error { return nil }
	personalConsumeRunMany = func(context.Context, consume.Config, []consume.ConsumerSpec) error {
		t.Fatal("RunMany called after partial creation failure")
		return nil
	}

	err := runPersonalEventConsume(newPersonalCoverageCommand(), personalConsumeOptions{
		EventKeys: []string{personal.EventMention, personal.EventAllSingleChat},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v", err)
	}
	if !reflect.DeepEqual(deleted, []string{"sub-first"}) || !reflect.DeepEqual(removed, []string{"sub-first"}) {
		t.Fatalf("rollback deleted=%#v removed=%#v", deleted, removed)
	}
}

func TestRunPersonalEventConsumeManyRejectsInvalidSubscriptionResults(t *testing.T) {
	for _, test := range []struct {
		name      string
		ensure    func(int, personalConsumeOptions) *personal.Subscription
		upsertErr error
		wantErr   string
	}{
		{
			name:    "nil subscription",
			ensure:  func(int, personalConsumeOptions) *personal.Subscription { return nil },
			wantErr: "empty subscription",
		},
		{
			name:    "empty subscribe id",
			ensure:  func(int, personalConsumeOptions) *personal.Subscription { return &personal.Subscription{} },
			wantErr: "empty subscribe_id",
		},
		{
			name: "duplicate subscribe id",
			ensure: func(int, personalConsumeOptions) *personal.Subscription {
				return &personal.Subscription{SubscribeID: "sub-duplicate"}
			},
			wantErr: "duplicate subscribe_id",
		},
		{
			name: "run state write failure",
			ensure: func(_ int, opts personalConsumeOptions) *personal.Subscription {
				return &personal.Subscription{SubscribeID: "sub-" + opts.EventKey}
			},
			upsertErr: errors.New("state write failed"),
			wantErr:   "save run state",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			restore := installPersonalManySeams(t)
			defer restore()
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
				return personal.Identity{AccessToken: "token", ClientID: "client", SourceID: "open", LocalSubject: "subject"}, nil
			}
			calls := 0
			personalEnsureSubscription = func(_ context.Context, _ *personal.Client, _ personal.Identity, opts personalConsumeOptions) (*personal.Subscription, string, string, error) {
				calls++
				return test.ensure(calls, opts), opts.EventKey, "all", nil
			}
			personalUpsertRunState = func(string, personal.RunState) error { return test.upsertErr }
			personalDeleteSubscription = func(*personal.Client, context.Context, string) error { return nil }
			personalRemoveRunStates = func(string, []string) error { return nil }
			personalValidateConsumeConfig = func(consume.Config) error { return nil }
			personalConsumeRunMany = func(context.Context, consume.Config, []consume.ConsumerSpec) error {
				t.Fatal("RunMany called with invalid subscription result")
				return nil
			}

			err := runPersonalEventConsume(newPersonalCoverageCommand(), personalConsumeOptions{
				EventKeys: []string{personal.EventMention, personal.EventAllSingleChat},
			})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestRunPersonalEventConsumeManyDryRunDoesNotCreateSubscriptions(t *testing.T) {
	restore := installPersonalManySeams(t)
	defer restore()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return personal.Identity{AccessToken: "token", ClientID: "client", SourceID: "open", LocalSubject: "subject"}, nil
	}
	personalEnsureSubscription = func(context.Context, *personal.Client, personal.Identity, personalConsumeOptions) (*personal.Subscription, string, string, error) {
		t.Fatal("dry-run created a subscription")
		return nil, "", "", nil
	}
	personalValidateConsumeConfig = func(consume.Config) error { return nil }

	cmd := newPersonalCoverageCommand()
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	err := runPersonalEventConsume(cmd, personalConsumeOptions{
		EventKeys: []string{personal.EventMention, personal.EventAllSingleChat},
		Common:    commonConsumeOptions{DryRun: true},
	})
	if err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	if !strings.Contains(stderr.String(), "subscription[0]") || !strings.Contains(stderr.String(), "subscription[1]") {
		t.Fatalf("dry-run subscriptions missing:\n%s", stderr.String())
	}
}

func TestStopPersonalConsumersUsesTargetedRPCAndLegacyFallback(t *testing.T) {
	oldStop := personalStopConsumers
	oldQuery := personalQueryStatus
	oldFind := personalFindProcess
	oldSignal := personalSignalProcess
	defer func() {
		personalStopConsumers = oldStop
		personalQueryStatus = oldQuery
		personalFindProcess = oldFind
		personalSignalProcess = oldSignal
	}()

	personalStopConsumers = func(string, []string) (transport.ConsumerStopResp, error) {
		return transport.ConsumerStopResp{Stopped: []string{"sub-a"}}, nil
	}
	personalQueryStatus = func(string) (*transport.StatusResp, error) {
		t.Fatal("legacy status queried after targeted stop succeeded")
		return nil, nil
	}
	if err := stopPersonalConsumers(io.Discard, "endpoint", []string{"sub-a"}); err != nil {
		t.Fatal(err)
	}

	personalStopConsumers = func(string, []string) (transport.ConsumerStopResp, error) {
		return transport.ConsumerStopResp{}, busctl.ErrConsumerStopUnsupported
	}
	personalQueryStatus = func(string) (*transport.StatusResp, error) {
		return &transport.StatusResp{Consumers: []transport.StatusConsumer{{PID: 321, SubscribeID: "sub-a"}}}, nil
	}
	proc := &os.Process{}
	personalFindProcess = func(int) (*os.Process, error) { return proc, nil }
	signals := 0
	personalSignalProcess = func(*os.Process, os.Signal) error { signals++; return nil }
	var warning bytes.Buffer
	if err := stopPersonalConsumers(&warning, "endpoint", []string{"sub-a"}); err != nil {
		t.Fatal(err)
	}
	if signals != 1 || !strings.Contains(warning.String(), "falling back to process signal") {
		t.Fatalf("signals=%d warning=%q", signals, warning.String())
	}

	wantErr := errors.New("targeted stop transport failed")
	personalStopConsumers = func(string, []string) (transport.ConsumerStopResp, error) {
		return transport.ConsumerStopResp{}, wantErr
	}
	personalQueryStatus = func(string) (*transport.StatusResp, error) {
		t.Fatal("legacy fallback ran for a non-compatibility error")
		return nil, nil
	}
	if err := stopPersonalConsumers(io.Discard, "endpoint", []string{"sub-a"}); !errors.Is(err, wantErr) {
		t.Fatalf("transport error = %v", err)
	}
}

func installPersonalManySeams(t *testing.T) func() {
	t.Helper()
	oldIdentity := personalResolveEventIdentity
	oldEnsure := personalEnsureSubscription
	oldUpsert := personalUpsertRunState
	oldDelete := personalDeleteSubscription
	oldRemove := personalRemoveRunStates
	oldRunMany := personalConsumeRunMany
	oldValidate := personalValidateConsumeConfig
	oldConflict := personalValidateNoOutputConflict
	return func() {
		personalResolveEventIdentity = oldIdentity
		personalEnsureSubscription = oldEnsure
		personalUpsertRunState = oldUpsert
		personalDeleteSubscription = oldDelete
		personalRemoveRunStates = oldRemove
		personalConsumeRunMany = oldRunMany
		personalValidateConsumeConfig = oldValidate
		personalValidateNoOutputConflict = oldConflict
	}
}
