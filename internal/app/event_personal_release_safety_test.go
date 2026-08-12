// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/source"
)

func TestPersonalConsumeCleanupOwnershipRuntimeMatrix(t *testing.T) {
	runErr := errors.New("runtime failed")
	for _, foreground := range []bool{false, true} {
		for _, selfCreated := range []bool{false, true} {
			for _, ephemeral := range []bool{false, true} {
				for _, failRuntime := range []bool{false, true} {
					name := strings.Join([]string{
						map[bool]string{false: "background", true: "foreground"}[foreground],
						map[bool]string{false: "reused", true: "self-created"}[selfCreated],
						map[bool]string{false: "persistent", true: "ephemeral"}[ephemeral],
						map[bool]string{false: "success", true: "error"}[failRuntime],
					}, "/")
					t.Run(name, func(t *testing.T) {
						restore := installPersonalManySeams(t)
						t.Cleanup(restore)
						t.Setenv("DWS_CONFIG_DIR", t.TempDir())

						oldNewSource := personalNewStreamSource
						oldBusRun := personalBusRun
						oldConsumeRun := personalConsumeRun
						t.Cleanup(func() {
							personalNewStreamSource = oldNewSource
							personalBusRun = oldBusRun
							personalConsumeRun = oldConsumeRun
						})

						personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
							return personal.Identity{
								AccessToken:  "token",
								ClientID:     "client",
								SourceID:     "open",
								LocalSubject: "subject",
							}, nil
						}
						personalEnsureSubscription = func(
							context.Context,
							*personal.Client,
							personal.Identity,
							personalConsumeOptions,
						) (*personal.Subscription, string, string, error) {
							return &personal.Subscription{SubscribeID: "sub-one"}, personal.EventMention, "at", nil
						}
						personalValidateConsumeConfig = func(consume.Config) error { return nil }
						personalValidateNoOutputConflict = func(consume.Config, string) error { return nil }
						personalUpsertRunState = func(string, personal.RunState) error { return nil }

						deleteCalls := 0
						removeCalls := 0
						personalDeleteSubscription = func(*personal.Client, context.Context, string) error {
							deleteCalls++
							return nil
						}
						personalRemoveRunStates = func(string, []string) error {
							removeCalls++
							return nil
						}

						personalNewStreamSource = func(context.Context, personalStreamSourceOptions) (*source.PersonalSource, error) {
							return nil, nil
						}
						personalBusRun = func(context.Context, bus.Config) error {
							if failRuntime {
								return runErr
							}
							return nil
						}
						personalConsumeRun = func(context.Context, consume.Config) error {
							if failRuntime {
								return runErr
							}
							return nil
						}

						opts := personalConsumeOptions{
							EventKey:       personal.EventMention,
							Ephemeral:      ephemeral,
							ControlBaseURL: "https://mcp.example.test/dws",
							Common: commonConsumeOptions{
								Foreground: foreground,
							},
						}
						if !selfCreated {
							opts.SubscribeID = "sub-one"
						}

						err := runPersonalEventConsumeSingle(newPersonalCoverageCommand(), opts)
						if failRuntime {
							if !errors.Is(err, runErr) {
								t.Fatalf("runtime error = %v, want %v", err, runErr)
							}
						} else if err != nil {
							t.Fatalf("consume error = %v", err)
						}

						wantCleanup := 0
						if selfCreated || ephemeral {
							wantCleanup = 1
						}
						if deleteCalls != wantCleanup || removeCalls != wantCleanup {
							t.Fatalf(
								"cleanup delete/remove = %d/%d, want %d/%d",
								deleteCalls,
								removeCalls,
								wantCleanup,
								wantCleanup,
							)
						}
					})
				}
			}
		}
	}
}

func TestPersonalConsumeCleanupOwnershipOnRunStateFailure(t *testing.T) {
	stateErr := errors.New("save state failed")
	for _, test := range []struct {
		name        string
		selfCreated bool
		ephemeral   bool
		wantCleanup int
	}{
		{name: "self-created", selfCreated: true, wantCleanup: 1},
		{name: "reused persistent", wantCleanup: 0},
		{name: "reused ephemeral", ephemeral: true, wantCleanup: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			restore := installPersonalManySeams(t)
			t.Cleanup(restore)
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())

			personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
				return personal.Identity{AccessToken: "token", ClientID: "client", SourceID: "open"}, nil
			}
			personalEnsureSubscription = func(
				context.Context,
				*personal.Client,
				personal.Identity,
				personalConsumeOptions,
			) (*personal.Subscription, string, string, error) {
				return &personal.Subscription{SubscribeID: "sub-one"}, personal.EventMention, "at", nil
			}
			personalValidateConsumeConfig = func(consume.Config) error { return nil }
			personalUpsertRunState = func(string, personal.RunState) error { return stateErr }
			deleteCalls := 0
			removeCalls := 0
			personalDeleteSubscription = func(*personal.Client, context.Context, string) error {
				deleteCalls++
				return nil
			}
			personalRemoveRunStates = func(string, []string) error {
				removeCalls++
				return nil
			}

			opts := personalConsumeOptions{
				EventKey:       personal.EventMention,
				Ephemeral:      test.ephemeral,
				ControlBaseURL: "https://mcp.example.test/dws",
			}
			if !test.selfCreated {
				opts.SubscribeID = "sub-one"
			}
			if err := runPersonalEventConsumeSingle(newPersonalCoverageCommand(), opts); !errors.Is(err, stateErr) {
				t.Fatalf("state error = %v, want %v", err, stateErr)
			}
			if deleteCalls != test.wantCleanup || removeCalls != test.wantCleanup {
				t.Fatalf(
					"cleanup delete/remove = %d/%d, want %d/%d",
					deleteCalls,
					removeCalls,
					test.wantCleanup,
					test.wantCleanup,
				)
			}
		})
	}
}

func TestPersonalReusedSubscriptionEventKeyResolution(t *testing.T) {
	oldGet := personalGetSubscription
	t.Cleanup(func() { personalGetSubscription = oldGet })

	for _, test := range []struct {
		name      string
		requested string
		actual    string
		wantKey   string
		wantErr   bool
	}{
		{
			name:      "matching key uses actual",
			requested: personal.EventMention,
			actual:    personal.EventMention,
			wantKey:   personal.EventMention,
		},
		{
			name:    "implicit key uses actual",
			actual:  personal.EventOAApprovalTaskCreated,
			wantKey: personal.EventOAApprovalTaskCreated,
		},
		{
			name:      "missing actual falls back to requested",
			requested: personal.EventOAApprovalTaskCreated,
			wantKey:   personal.EventOAApprovalTaskCreated,
		},
		{
			name:      "requested IM mismatches actual OA",
			requested: personal.EventMention,
			actual:    personal.EventOAApprovalTaskCreated,
			wantErr:   true,
		},
		{
			name:      "requested OA mismatches actual IM",
			requested: personal.EventOAApprovalTaskCreated,
			actual:    personal.EventMention,
			wantErr:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			personalGetSubscription = func(*personal.Client, context.Context, string) (*personal.Subscription, error) {
				return &personal.Subscription{
					SubscribeID: "sub-one",
					EventKey:    test.actual,
				}, nil
			}
			_, eventKey, _, err := ensurePersonalSubscription(
				context.Background(),
				nil,
				personal.Identity{},
				personalConsumeOptions{SubscribeID: "sub-one", EventKey: test.requested},
			)
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "does not match reused subscription") {
					t.Fatalf("mismatch error = %v", err)
				}
				if !strings.Contains(err.Error(), test.requested) || !strings.Contains(err.Error(), test.actual) {
					t.Fatalf("mismatch error does not identify both keys: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve reused subscription: %v", err)
			}
			if eventKey != test.wantKey {
				t.Fatalf("resolved event key = %q, want %q", eventKey, test.wantKey)
			}
		})
	}
}

func TestPersonalReusedSubscriptionMismatchStopsDryRunAndRuntime(t *testing.T) {
	for _, mode := range []struct {
		name       string
		dryRun     bool
		foreground bool
	}{
		{name: "dry-run", dryRun: true},
		{name: "background"},
		{name: "foreground", foreground: true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			restore := installPersonalManySeams(t)
			t.Cleanup(restore)
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())

			oldGet := personalGetSubscription
			oldNewSource := personalNewStreamSource
			oldBusRun := personalBusRun
			oldConsumeRun := personalConsumeRun
			t.Cleanup(func() {
				personalGetSubscription = oldGet
				personalNewStreamSource = oldNewSource
				personalBusRun = oldBusRun
				personalConsumeRun = oldConsumeRun
			})

			personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
				return personal.Identity{AccessToken: "token", ClientID: "client", SourceID: "open"}, nil
			}
			personalEnsureSubscription = ensurePersonalSubscription
			personalGetSubscription = func(*personal.Client, context.Context, string) (*personal.Subscription, error) {
				return &personal.Subscription{
					SubscribeID: "sub-one",
					EventKey:    personal.EventOAApprovalTaskCreated,
					RuleType:    "all",
				}, nil
			}
			personalValidateConsumeConfig = func(consume.Config) error { return nil }
			personalValidateNoOutputConflict = func(consume.Config, string) error { return nil }

			upsertCalls := 0
			consumeCalls := 0
			busCalls := 0
			personalUpsertRunState = func(string, personal.RunState) error {
				upsertCalls++
				return nil
			}
			personalNewStreamSource = func(context.Context, personalStreamSourceOptions) (*source.PersonalSource, error) {
				return nil, nil
			}
			personalBusRun = func(context.Context, bus.Config) error {
				busCalls++
				return nil
			}
			personalConsumeRun = func(context.Context, consume.Config) error {
				consumeCalls++
				return nil
			}

			err := runPersonalEventConsumeSingle(newPersonalCoverageCommand(), personalConsumeOptions{
				EventKey:       personal.EventMention,
				SubscribeID:    "sub-one",
				ControlBaseURL: "https://mcp.example.test/dws",
				Common: commonConsumeOptions{
					DryRun:     mode.dryRun,
					Foreground: mode.foreground,
				},
			})
			if err == nil || !strings.Contains(err.Error(), "does not match reused subscription") {
				t.Fatalf("mismatch error = %v", err)
			}
			if upsertCalls != 0 || consumeCalls != 0 || busCalls != 0 {
				t.Fatalf(
					"mismatch reached upsert/consumer/bus = %d/%d/%d",
					upsertCalls,
					consumeCalls,
					busCalls,
				)
			}
		})
	}
}

func TestPersonalReusedSubscriptionUsesActualKeyInDryRunAndRuntime(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		name := map[bool]string{false: "runtime", true: "dry-run"}[dryRun]
		t.Run(name, func(t *testing.T) {
			restore := installPersonalManySeams(t)
			t.Cleanup(restore)
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())

			oldGet := personalGetSubscription
			oldConsumeRun := personalConsumeRun
			t.Cleanup(func() {
				personalGetSubscription = oldGet
				personalConsumeRun = oldConsumeRun
			})

			personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
				return personal.Identity{AccessToken: "token", ClientID: "client", SourceID: "open"}, nil
			}
			personalEnsureSubscription = ensurePersonalSubscription
			personalGetSubscription = func(*personal.Client, context.Context, string) (*personal.Subscription, error) {
				return &personal.Subscription{
					SubscribeID: "sub-oa",
					EventKey:    personal.EventOAApprovalInstanceFinished,
					RuleType:    "all",
				}, nil
			}
			personalValidateConsumeConfig = func(consume.Config) error { return nil }
			personalValidateNoOutputConflict = func(consume.Config, string) error { return nil }
			personalUpsertRunState = func(_ string, state personal.RunState) error {
				if state.EventKey != personal.EventOAApprovalInstanceFinished {
					t.Fatalf("run state event key = %q", state.EventKey)
				}
				return nil
			}

			var got consume.Config
			personalConsumeRun = func(_ context.Context, cfg consume.Config) error {
				got = cfg
				return nil
			}
			if err := runPersonalEventConsumeSingle(newPersonalCoverageCommand(), personalConsumeOptions{
				SubscribeID:    "sub-oa",
				ControlBaseURL: "https://mcp.example.test/dws",
				Common: commonConsumeOptions{
					DryRun: dryRun,
				},
			}); err != nil {
				t.Fatalf("reuse subscription: %v", err)
			}
			if got.EventKey != personal.EventOAApprovalInstanceFinished ||
				len(got.EventTypes) != 1 || got.EventTypes[0] != personal.EventOAApprovalInstanceFinished ||
				got.SubscribeID != "sub-oa" {
				t.Fatalf("consume config = %#v", got)
			}
		})
	}
}

func TestEventConsumeDryRunHelpDescribesReuseLookup(t *testing.T) {
	usage := newEventConsumeCommand().Flags().Lookup("dry-run").Usage
	for _, want := range []string{"不创建订阅", "不连接 bus", "复用 --subscribe-id", "只读查询控制面"} {
		if !strings.Contains(usage, want) {
			t.Fatalf("dry-run help %q missing %q", usage, want)
		}
	}
}
