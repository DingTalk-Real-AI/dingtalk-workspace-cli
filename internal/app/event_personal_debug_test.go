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
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/spf13/cobra"
)

func TestApplyPersonalConsumeFiltersDebugRawEvents(t *testing.T) {
	cfg := consume.Config{EventKey: personal.EventSingleChat, ReadySubscribeID: "sub-1"}
	opts := personalConsumeOptions{
		DebugRawEvents: true,
		Common: commonConsumeOptions{
			EventTypes: []string{"should-not-survive"},
			Filter:     "^should-not-survive$",
		},
	}
	applyPersonalConsumeFilters(&cfg, opts, "sub-1", "user_im_message_receive_o2o")
	if cfg.EventTypes != nil || cfg.Filter != "" || cfg.SubscribeID != "" {
		t.Fatalf("raw debug filters = eventTypes=%#v filter=%q subscribeID=%q, want catch-all", cfg.EventTypes, cfg.Filter, cfg.SubscribeID)
	}
	if cfg.EventKey != personal.EventSingleChat || cfg.ReadySubscribeID != "sub-1" {
		t.Fatalf("raw debug cleared ready identity: eventKey=%q subscribeID=%q", cfg.EventKey, cfg.ReadySubscribeID)
	}
}

func TestApplyPersonalConsumeFiltersDefault(t *testing.T) {
	cfg := consume.Config{}
	opts := personalConsumeOptions{Common: commonConsumeOptions{Filter: "^user_im_"}}
	applyPersonalConsumeFilters(&cfg, opts, "sub-1", "user_im_message_receive_o2o")
	if len(cfg.EventTypes) != 1 || cfg.EventTypes[0] != "user_im_message_receive_o2o" {
		t.Fatalf("eventTypes = %#v", cfg.EventTypes)
	}
	if cfg.Filter != "^user_im_" || cfg.SubscribeID != "sub-1" {
		t.Fatalf("filter=%q subscribeID=%q", cfg.Filter, cfg.SubscribeID)
	}
}

func TestPersonalEventProjectorSelectsExplicitModes(t *testing.T) {
	if personalEventProjector(false, false) == nil {
		t.Fatal("default personal consume safe transport projector = nil")
	}
	if personalEventProjector(false, true) == nil {
		t.Fatal("flatten personal consume projector = nil")
	}
	projector := personalEventProjector(true, false)
	if projector == nil {
		t.Fatal("debug raw personal consume projector = nil")
	}
	ev := transport.Event{
		EventID: "raw-event",
		Data:    `{"payload":{"uid":100001,"bizid":"internal-bizid"}}`,
		Headers: map[string]string{"TOPIC": "raw"},
	}
	projected, err := projector(ev)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := projected.(transport.Event); !ok || got.EventID != ev.EventID || got.Data != ev.Data || got.Headers["TOPIC"] != "raw" {
		t.Fatalf("debug raw projection = %#v", projected)
	}
}

func TestCrossPlatformCoveragePersonalVoIPDefaultOutputRedactsRoomCode(t *testing.T) {
	ev := transport.Event{
		EventID:       "transport-event-1",
		EventBornTime: 1787903566711,
		EventType:     personal.EventVoIPCallReceiveInvite,
		SubscribeID:   "sub-1",
		Data:          `{"eventId":"business-event-1","eventKey":"user_voip_call_receive_invite","occurredAtMs":1787903566579,"subId":"sub-1","payload":{"bizid":"VOIP-1","body":{"callId":"call-1","roomCode":"7286913750"}}}`,
	}
	formatter, err := consume.NewFormatter(consume.FormatNDJSON,
		consume.WithProjector(personalEventProjector(false, false)),
	)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := formatter.Render(ev)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rendered, []byte("7286913750")) || bytes.Contains(rendered, []byte("roomCode")) {
		t.Fatalf("default VoIP output leaked room code: %s", rendered)
	}
	var envelope transport.Event
	if err := json.Unmarshal(bytes.TrimSpace(rendered), &envelope); err != nil {
		t.Fatalf("default VoIP output no longer preserves transport envelope: %v\n%s", err, rendered)
	}
	if envelope.EventID != ev.EventID || envelope.EventType != ev.EventType || envelope.SubscribeID != ev.SubscribeID {
		t.Fatalf("default VoIP transport identity changed: %#v", envelope)
	}
	if !strings.Contains(envelope.Data, `"callId":"call-1"`) {
		t.Fatalf("default VoIP output dropped non-sensitive payload: %s", envelope.Data)
	}
}

func TestCrossPlatformCoveragePersonalVoIPDebugRawOutputRequiresExplicitOptIn(t *testing.T) {
	ev := transport.Event{
		EventID:   "transport-event-1",
		EventType: personal.EventVoIPCallReceiveInvite,
		Data:      `{"payload":{"body":{"roomCode":"7286913750"}}}`,
	}
	formatter, err := consume.NewFormatter(consume.FormatNDJSON,
		consume.WithProjector(personalEventProjector(true, false)),
	)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := formatter.Render(ev)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rendered, []byte("7286913750")) || !bytes.Contains(rendered, []byte("roomCode")) {
		t.Fatalf("explicit debug raw output did not preserve original payload: %s", rendered)
	}
}

func TestEventConsumeFlattenRejectsRawModesBeforeIdentityResolution(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "raw format",
			args: []string{personal.EventMention, "--flatten", "--format", "raw"},
			want: "--flatten and --format raw are mutually exclusive",
		},
		{
			name: "raw debug",
			args: []string{personal.EventMention, "--flatten", "--debug-raw-events"},
			want: "--flatten and --debug-raw-events are mutually exclusive",
		},
		{
			name: "VoIP raw format without explicit debug opt-in",
			args: []string{personal.EventVoIPCallReceiveInvite, "--format", "raw"},
			want: "--format raw for VoIP events requires explicit --debug-raw-events",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			cmd := newEventConsumeCommand()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Execute() error = %v, want %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "login") || strings.Contains(err.Error(), "token") {
				t.Fatalf("output-mode validation ran after identity resolution: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoveragePersonalVoIPReusedSubscriptionRawRequiresDebugOptIn(t *testing.T) {
	oldIdentity := personalResolveEventIdentity
	oldGet := personalGetSubscription
	oldUpsert := personalUpsertRunState
	oldConsume := personalConsumeRun
	t.Cleanup(func() {
		personalResolveEventIdentity = oldIdentity
		personalGetSubscription = oldGet
		personalUpsertRunState = oldUpsert
		personalConsumeRun = oldConsume
	})
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) {
		return personal.Identity{
			AccessToken:  "token",
			LocalSubject: "subject",
			ClientID:     "client",
			SourceID:     "open",
		}, nil
	}
	personalGetSubscription = func(_ *personal.Client, _ context.Context, subscribeID string) (*personal.Subscription, error) {
		return &personal.Subscription{
			SubscribeID: subscribeID,
			EventKey:    personal.EventVoIPCallReceiveInvite,
			RuleType:    "all",
		}, nil
	}
	personalUpsertRunState = func(string, personal.RunState) error { return nil }
	var consumeDryRuns []bool
	personalConsumeRun = func(_ context.Context, cfg consume.Config) error {
		consumeDryRuns = append(consumeDryRuns, cfg.DryRun)
		if cfg.EventKey != personal.EventVoIPCallReceiveInvite || cfg.Format != consume.FormatRaw {
			t.Fatalf("resolved reused VoIP consume config = %#v", cfg)
		}
		return nil
	}

	for _, dryRun := range []bool{true, false} {
		mode := "live"
		if dryRun {
			mode = "dry-run"
		}
		t.Run(mode, func(t *testing.T) {
			baseArgs := []string{"--subscribe-id", "voip-sub", "--format", "raw"}
			if dryRun {
				baseArgs = append(baseArgs, "--dry-run")
			}

			cmd := newEventConsumeCommand()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			cmd.SetArgs(baseArgs)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "--format raw for VoIP events requires explicit --debug-raw-events") {
				t.Fatalf("reused VoIP raw without debug error = %v", err)
			}

			cmd = newEventConsumeCommand()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			cmd.SetArgs(append(append([]string(nil), baseArgs...), "--debug-raw-events"))
			if err := cmd.Execute(); err != nil {
				t.Fatalf("reused VoIP raw with explicit debug error = %v", err)
			}
		})
	}
	if !reflect.DeepEqual(consumeDryRuns, []bool{true, false}) {
		t.Fatalf("reused VoIP consume dry-run sequence = %#v, want [true false]", consumeDryRuns)
	}
}

func TestValidatePersonalEventOutputModeAllowsFlattenStructuredFormats(t *testing.T) {
	for _, format := range []consume.Format{consume.FormatNDJSON, consume.FormatJSON, consume.FormatPretty, consume.FormatCompact} {
		if err := validatePersonalEventOutputMode([]string{personal.EventVoIPCallReceiveInvite}, true, false, format); err != nil {
			t.Fatalf("validatePersonalEventOutputMode(VoIP, true, false, %q) error = %v", format, err)
		}
	}
	if err := validatePersonalEventOutputMode([]string{personal.EventVoIPCallReceiveInvite}, false, true, consume.FormatRaw); err != nil {
		t.Fatalf("explicit VoIP raw debug mode error = %v", err)
	}
}

func TestEventConsumeFlattenFlagIsForwarded(t *testing.T) {
	oldRun := eventRunPersonalConsume
	t.Cleanup(func() { eventRunPersonalConsume = oldRun })

	var got personalConsumeOptions
	eventRunPersonalConsume = func(_ *cobra.Command, opts personalConsumeOptions) error {
		got = opts
		return nil
	}
	cmd := newEventConsumeCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{personal.EventMention, "--flatten", "--format", "compact"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !got.Flatten || got.Common.FormatRaw != "compact" {
		t.Fatalf("forwarded options = %#v", got)
	}
}

func TestEventConsumeDebugRawEventsRequiresUserMode(t *testing.T) {
	cmd := newEventConsumeCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--as", "app", "--debug-raw-events"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "app event is not publicly available yet") {
		t.Fatalf("Execute() error = %v, want public availability guard", err)
	}
}

func TestEventConsumeAsAppRejectedBeforeEventKeyValidation(t *testing.T) {
	cmd := newEventConsumeCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--as", "app", personal.EventSingleChat})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "app event is not publicly available yet") {
		t.Fatalf("Execute() error = %v, want public availability guard", err)
	}
}

func TestEventConsumePersonalParamSpecFlags(t *testing.T) {
	cmd := newEventConsumeCommand()
	for _, name := range []string{"user", "open-dingtalk-id", "group", "query"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("flag --%s is not registered", name)
		}
	}
	for _, name := range []string{
		"peer-user-id",
		"peer-union-id",
		"sender-user-id",
		"sender-union-id",
		"open-conversation-id",
		"keyword",
		"odid",
	} {
		if cmd.Flags().Lookup(name) != nil {
			t.Fatalf("retired flag --%s is still registered", name)
		}
	}
}

func TestEventConsumeRetiredPersonalFlagsAreUnknown(t *testing.T) {
	for _, name := range []string{
		"peer-user-id",
		"peer-union-id",
		"sender-user-id",
		"sender-union-id",
		"open-conversation-id",
		"keyword",
		"odid",
	} {
		t.Run(name, func(t *testing.T) {
			cmd := newEventConsumeCommand()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			cmd.SetArgs([]string{personal.EventSingleChat, "--" + name, "x"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "unknown flag: --"+name) {
				t.Fatalf("Execute() error = %v, want unknown flag", err)
			}
		})
	}
}

func TestEventConsumeAsAppRejectedBeforePersonalParamSpecFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--as", "app", "--user", "test-user-001"},
		{"--as", "app", "--open-dingtalk-id", "open-user-1"},
		{"--as", "app", "--group", "cid"},
		{"--as", "app", "--query", "报警"},
	} {
		cmd := newEventConsumeCommand()
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "app event is not publicly available yet") {
			t.Fatalf("Execute(%v) error = %v, want public availability guard", args, err)
		}
	}
}
