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
	"time"

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
			name: "duplicate keys collapse to one",
			opts: personalConsumeOptions{
				EventKeys: []string{personal.EventMention, personal.EventMention},
			},
			wantErr: "multiple event keys are required",
		},
		{
			name: "unknown event",
			opts: personalConsumeOptions{
				EventKeys: []string{personal.EventMention, "user_im_unknown"},
			},
			wantErr: "unknown personal event key",
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
			name: "user identity flags conflict",
			opts: personalConsumeOptions{
				EventKeys:      []string{personal.EventSingleChat, personal.EventReadO2O},
				UserID:         "test-user-001",
				OpenDingTalkID: "open-test-user",
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "group target on user events",
			opts: personalConsumeOptions{
				EventKeys: []string{personal.EventSingleChat, personal.EventReadO2O},
				UserID:    "test-user-001",
				GroupID:   "cid-test",
			},
			wantErr: "--group cannot be used",
		},
		{
			name: "user target on group events",
			opts: personalConsumeOptions{
				EventKeys: []string{personal.EventInChat, personal.EventGroupUpdated},
				UserID:    "test-user-001",
				GroupID:   "cid-test",
			},
			wantErr: "cannot be used with group-scoped events",
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
		{
			name: "invalid message filter",
			opts: personalConsumeOptions{
				EventKeys:  []string{personal.EventMention, personal.EventAllSingleChat},
				FilterJSON: "{",
			},
			wantErr: "filter",
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

func TestPreparePersonalMultiOptionsRejectsNonPublicEvent(t *testing.T) {
	oldLookup := personalLookupDefinition
	t.Cleanup(func() { personalLookupDefinition = oldLookup })
	personalLookupDefinition = func(eventKey string) (personal.Definition, bool) {
		def, ok := personal.Lookup(eventKey)
		if eventKey == personal.EventAllSingleChat {
			def.Public = false
		}
		return def, ok
	}

	_, err := preparePersonalMultiOptions(personalConsumeOptions{
		EventKeys: []string{personal.EventMention, personal.EventAllSingleChat},
	})
	if err == nil || !strings.Contains(err.Error(), "not publicly available") {
		t.Fatalf("error = %v", err)
	}
}

func TestDedupePersonalEventKeysSkipsEmptyValues(t *testing.T) {
	got := dedupePersonalEventKeys([]string{"", " event-a ", "event-a", "event-b"})
	if !reflect.DeepEqual(got, []string{"event-a", "event-b"}) {
		t.Fatalf("deduped keys = %#v", got)
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

func TestCrossPlatformCoverageEventConsumeMultiRejectsExplicitSingleOnlyFlagsEvenWhenEmpty(t *testing.T) {
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

func TestCrossPlatformCoverageRunPersonalEventConsumeManyPersistsFailureBeforeRollback(t *testing.T) {
	restore := installPersonalManySeams(t)
	defer restore()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	var order []string
	personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
		return &personalOrderingAttemptStore{order: &order}
	}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return personal.Identity{
			AccessToken:  "token",
			ClientID:     "client",
			SourceID:     "open",
			LocalSubject: "subject",
		}, nil
	}
	calls := 0
	personalEnsureSubscription = func(
		_ context.Context,
		_ *personal.Client,
		_ personal.Identity,
		opts personalConsumeOptions,
	) (*personal.Subscription, string, string, error) {
		calls++
		if calls == 2 {
			return nil, "", "", errors.New("second subscription failed")
		}
		return &personal.Subscription{SubscribeID: "sub-first"}, opts.EventKey, "all", nil
	}
	personalUpsertRunState = func(string, personal.RunState) error { return nil }
	personalDeleteSubscription = func(_ *personal.Client, _ context.Context, _ string) error {
		order = append(order, "delete")
		return nil
	}
	personalRemoveRunStates = func(string, []string) error { return nil }
	personalValidateConsumeConfig = func(consume.Config) error { return nil }

	err := runPersonalEventConsume(newPersonalCoverageCommand(), personalConsumeOptions{
		EventKeys: []string{personal.EventMention, personal.EventAllSingleChat},
	})
	if err == nil {
		t.Fatal("partial creation unexpectedly succeeded")
	}
	if !reflect.DeepEqual(order, []string{"complete_failure", "delete"}) {
		t.Fatalf("failure/rollback order = %#v", order)
	}
}

func TestCrossPlatformCoverageRunPersonalEventConsumeSinglePersistsLocalFailureBeforeRollback(t *testing.T) {
	restore := installPersonalManySeams(t)
	defer restore()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	var order []string
	personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
		return &personalOrderingAttemptStore{order: &order}
	}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return personal.Identity{
			AccessToken:  "token",
			ClientID:     "client",
			SourceID:     "open",
			LocalSubject: "subject",
		}, nil
	}
	personalEnsureSubscription = func(
		_ context.Context,
		_ *personal.Client,
		_ personal.Identity,
		opts personalConsumeOptions,
	) (*personal.Subscription, string, string, error) {
		return &personal.Subscription{SubscribeID: "sub-one"}, opts.EventKey, "all", nil
	}
	personalUpsertRunState = func(string, personal.RunState) error {
		return errors.New("state disk failed")
	}
	personalDeleteSubscription = func(_ *personal.Client, _ context.Context, _ string) error {
		order = append(order, "delete")
		return nil
	}
	personalRemoveRunStates = func(string, []string) error { return nil }
	personalValidateConsumeConfig = func(consume.Config) error { return nil }

	err := runPersonalEventConsume(newPersonalCoverageCommand(), personalConsumeOptions{
		EventKey: personal.EventMention,
	})
	if err == nil {
		t.Fatal("run-state failure unexpectedly succeeded")
	}
	if !reflect.DeepEqual(order, []string{"complete_failure", "delete"}) {
		t.Fatalf("failure/rollback order = %#v", order)
	}
}

func TestCrossPlatformCoverageRunPersonalEventConsumeManyCancellationReleasesBeforeCanceledCleanup(t *testing.T) {
	restore := installPersonalManySeams(t)
	defer restore()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	var order []string
	personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
		return &personalOrderingAttemptStore{order: &order}
	}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return personal.Identity{
			AccessToken:  "token",
			ClientID:     "client",
			SourceID:     "open",
			LocalSubject: "subject",
		}, nil
	}
	calls := 0
	personalEnsureSubscription = func(
		_ context.Context,
		_ *personal.Client,
		_ personal.Identity,
		opts personalConsumeOptions,
	) (*personal.Subscription, string, string, error) {
		calls++
		if calls == 2 {
			return nil, "", "", context.Canceled
		}
		return &personal.Subscription{SubscribeID: "sub-first"}, opts.EventKey, "all", nil
	}
	personalUpsertRunState = func(string, personal.RunState) error { return nil }
	personalDeleteSubscription = func(_ *personal.Client, cleanupCtx context.Context, _ string) error {
		if cleanupCtx.Err() == nil {
			t.Fatal("cancellation cleanup received a live context")
		}
		order = append(order, "delete")
		return cleanupCtx.Err()
	}
	personalRemoveRunStates = func(string, []string) error { return nil }
	personalValidateConsumeConfig = func(consume.Config) error { return nil }

	cmd := newPersonalCoverageCommand()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	err := runPersonalEventConsume(cmd, personalConsumeOptions{
		EventKeys: []string{personal.EventMention, personal.EventAllSingleChat},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if !reflect.DeepEqual(order, []string{"release", "delete"}) {
		t.Fatalf("release/canceled-cleanup order = %#v", order)
	}
}

func TestCrossPlatformCoverageRunPersonalEventConsumeManyRejectsInvalidSubscriptionResults(t *testing.T) {
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
		QueryCSV:  "alarm",
		Common:    commonConsumeOptions{DryRun: true},
	})
	if err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	if !strings.Contains(stderr.String(), "subscription[0]") ||
		!strings.Contains(stderr.String(), "subscription[1]") ||
		!strings.Contains(stderr.String(), "filter=") {
		t.Fatalf("dry-run subscriptions missing:\n%s", stderr.String())
	}
}

func TestCrossPlatformCoverageRunPersonalEventConsumeManySetupAndCleanupEdges(t *testing.T) {
	valid := personalConsumeOptions{EventKeys: []string{personal.EventMention, personal.EventAllSingleChat}}

	t.Run("prepare error", func(t *testing.T) {
		err := runPersonalEventConsumeMany(newPersonalCoverageCommand(), personalConsumeOptions{
			EventKeys: []string{personal.EventMention},
		})
		if err == nil || !strings.Contains(err.Error(), "multiple event keys") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("format warning and identity error", func(t *testing.T) {
		restore := installPersonalManySeams(t)
		defer restore()
		cmd := newPersonalCoverageCommand()
		var stderr bytes.Buffer
		cmd.SetErr(&stderr)
		if err := cmd.Flags().Set("format", "bogus"); err != nil {
			t.Fatal(err)
		}
		wantErr := errors.New("identity")
		personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
			return personal.Identity{}, wantErr
		}
		opts := valid
		opts.Common.FormatRaw = "bogus"
		if err := runPersonalEventConsumeMany(cmd, opts); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v", err)
		}
		if !strings.Contains(stderr.String(), "using ndjson") {
			t.Fatalf("warning = %q", stderr.String())
		}
	})

	t.Run("flatten raw conflict", func(t *testing.T) {
		cmd := newPersonalCoverageCommand()
		if err := cmd.Flags().Set("format", "raw"); err != nil {
			t.Fatal(err)
		}
		opts := valid
		opts.Flatten = true
		opts.Common.FormatRaw = "raw"
		if err := runPersonalEventConsumeMany(cmd, opts); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("route validation and output conflict", func(t *testing.T) {
		restore := installPersonalManySeams(t)
		defer restore()
		personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
			return personal.Identity{AccessToken: "token", ClientID: "client", SourceID: "open", LocalSubject: "subject"}, nil
		}

		opts := valid
		opts.Common.RoutesRaw = []string{"bad-route"}
		if err := runPersonalEventConsumeMany(newPersonalCoverageCommand(), opts); err == nil {
			t.Fatal("invalid route succeeded")
		}

		wantErr := errors.New("validate")
		personalValidateConsumeConfig = func(consume.Config) error { return wantErr }
		if err := runPersonalEventConsumeMany(newPersonalCoverageCommand(), valid); !errors.Is(err, wantErr) {
			t.Fatalf("config validation error = %v", err)
		}

		personalValidateConsumeConfig = func(consume.Config) error { return nil }
		personalValidateNoOutputConflict = func(consume.Config, string) error { return wantErr }
		cmd := newPersonalCoverageCommand()
		if err := cmd.Flags().Set("output", "events.json"); err != nil {
			t.Fatal(err)
		}
		if err := runPersonalEventConsumeMany(cmd, valid); !errors.Is(err, wantErr) {
			t.Fatalf("output conflict error = %v", err)
		}
	})

	t.Run("runtime error reports cleanup failures", func(t *testing.T) {
		restore := installPersonalManySeams(t)
		defer restore()
		personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
			return personal.Identity{AccessToken: "token", ClientID: "client", SourceID: "open", LocalSubject: "subject"}, nil
		}
		personalEnsureSubscription = func(_ context.Context, _ *personal.Client, _ personal.Identity, opts personalConsumeOptions) (*personal.Subscription, string, string, error) {
			return &personal.Subscription{SubscribeID: "sub-" + opts.EventKey}, opts.EventKey, "all", nil
		}
		personalUpsertRunState = func(string, personal.RunState) error { return nil }
		personalValidateConsumeConfig = func(consume.Config) error { return nil }
		cleanupErr := errors.New("cleanup")
		personalDeleteSubscription = func(*personal.Client, context.Context, string) error { return cleanupErr }
		personalRemoveRunStates = func(string, []string) error { return cleanupErr }
		runErr := errors.New("run many")
		personalConsumeRunMany = func(context.Context, consume.Config, []consume.ConsumerSpec) error { return runErr }

		cmd := newPersonalCoverageCommand()
		var stderr bytes.Buffer
		cmd.SetErr(&stderr)
		if err := runPersonalEventConsumeMany(cmd, valid); !errors.Is(err, runErr) {
			t.Fatalf("runtime error = %v", err)
		}
		if !strings.Contains(stderr.String(), "failed to clean personal subscription") ||
			!strings.Contains(stderr.String(), "failed to clean personal event run state") {
			t.Fatalf("cleanup warnings = %q", stderr.String())
		}
	})
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
	oldLookup := personalLookupDefinition
	oldEnsure := personalEnsureSubscription
	oldAttemptStore := personalNewSubscriptionAttemptStore
	oldUpsert := personalUpsertRunState
	oldDelete := personalDeleteSubscription
	oldRemove := personalRemoveRunStates
	oldRunMany := personalConsumeRunMany
	oldValidate := personalValidateConsumeConfig
	oldConflict := personalValidateNoOutputConflict
	personalNewSubscriptionAttemptStore = func(string) personalSubscriptionAttemptStore {
		return personalNoopAttemptStore{}
	}
	return func() {
		personalResolveEventIdentity = oldIdentity
		personalLookupDefinition = oldLookup
		personalEnsureSubscription = oldEnsure
		personalNewSubscriptionAttemptStore = oldAttemptStore
		personalUpsertRunState = oldUpsert
		personalDeleteSubscription = oldDelete
		personalRemoveRunStates = oldRemove
		personalConsumeRunMany = oldRunMany
		personalValidateConsumeConfig = oldValidate
		personalValidateNoOutputConflict = oldConflict
	}
}

type personalNoopAttemptStore struct{}

func (personalNoopAttemptStore) Claim(specs []personal.AttemptSpec, _ time.Duration) (*personal.AttemptClaim, error) {
	fingerprints := make([]string, 0, len(specs))
	for _, spec := range specs {
		fingerprints = append(fingerprints, spec.Fingerprint)
	}
	return &personal.AttemptClaim{
		AttemptID:    "test-attempt",
		Fingerprints: fingerprints,
	}, nil
}

func (personalNoopAttemptStore) CompleteSuccess(*personal.AttemptClaim) error {
	return nil
}

func (personalNoopAttemptStore) CompleteFailure(
	_ *personal.AttemptClaim,
	_ []string,
	failure personal.AttemptFailure,
) (personal.AttemptHold, error) {
	return personal.AttemptHold{
		Fingerprint:  failure.Fingerprint,
		Retryability: failure.Retryability,
	}, nil
}

func (personalNoopAttemptStore) Release(*personal.AttemptClaim) error {
	return nil
}

type personalOrderingAttemptStore struct {
	order *[]string
}

func (s *personalOrderingAttemptStore) Claim(
	specs []personal.AttemptSpec,
	lease time.Duration,
) (*personal.AttemptClaim, error) {
	return personalNoopAttemptStore{}.Claim(specs, lease)
}

func (s *personalOrderingAttemptStore) CompleteSuccess(*personal.AttemptClaim) error {
	*s.order = append(*s.order, "complete_success")
	return nil
}

func (s *personalOrderingAttemptStore) CompleteFailure(
	_ *personal.AttemptClaim,
	_ []string,
	failure personal.AttemptFailure,
) (personal.AttemptHold, error) {
	*s.order = append(*s.order, "complete_failure")
	return personal.AttemptHold{
		Fingerprint:  failure.Fingerprint,
		Retryability: failure.Retryability,
	}, nil
}

func (s *personalOrderingAttemptStore) Release(*personal.AttemptClaim) error {
	*s.order = append(*s.order, "release")
	return nil
}
