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
)

func TestApplyPersonalConsumeFiltersDebugRawEvents(t *testing.T) {
	cfg := consume.Config{}
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
	for _, name := range []string{"user", "group", "query"} {
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
		{"--as", "app", "--user", "507971"},
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
