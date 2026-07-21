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
	if personalEventProjector(false, false) != nil {
		t.Fatal("default personal consume should preserve transport envelope")
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

func TestValidatePersonalEventOutputModeAllowsFlattenStructuredFormats(t *testing.T) {
	for _, format := range []consume.Format{consume.FormatNDJSON, consume.FormatJSON, consume.FormatPretty, consume.FormatCompact} {
		if err := validatePersonalEventOutputMode(true, false, format); err != nil {
			t.Fatalf("validatePersonalEventOutputMode(true, false, %q) error = %v", format, err)
		}
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
