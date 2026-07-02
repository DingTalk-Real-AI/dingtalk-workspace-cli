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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
)

func TestPersonalEventListHidesSchemaIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "table", args: []string{"--as", "user"}},
		{name: "json", args: []string{"--as", "user", "--format", "json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newEventListCommand()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			assertPersonalOutputHidesSchemaIDs(t, out.String())
		})
	}
}

func TestEventListDefaultsToUser(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	cmd := newEventListCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, personal.EventSingleChat) || !strings.Contains(got, "EVENT_KEY") {
		t.Fatalf("list output = %s, want personal event catalog", got)
	}
	if strings.Contains(got, "CLIENT_ID") || strings.Contains(got, "ClientSecret") {
		t.Fatalf("list default appears to use app stream output: %s", got)
	}
}

func TestEventListAppOnlyFlagsRequireApp(t *testing.T) {
	cmd := newEventListCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--all"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--all are only supported with --as app") {
		t.Fatalf("Execute() error = %v, want app-only flag validation", err)
	}
}

func TestEventListAsAppAllStillUsesAppStream(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	cmd := newEventListCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--as", "app", "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "SOURCE") || !strings.Contains(out.String(), "CLIENT_ID") {
		t.Fatalf("app list output = %s, want app stream table", out.String())
	}
}

func TestEventStatusAppOnlyFlagsRequireApp(t *testing.T) {
	cmd := newEventStatusCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--all", "--fail-on-orphan"})
	err := cmd.Execute()
	if err == nil ||
		!strings.Contains(err.Error(), "--all") ||
		!strings.Contains(err.Error(), "--fail-on-orphan") ||
		!strings.Contains(err.Error(), "only supported with --as app") {
		t.Fatalf("Execute() error = %v, want app-only flag validation", err)
	}
}

func TestPersonalEventSchemaHidesSchemaIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "table", args: []string{personal.EventSingleChat, "--as", "user"}},
		{name: "json", args: []string{personal.EventSingleChat, "--as", "user", "--format", "json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newEventSchemaCommand()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			assertPersonalOutputHidesSchemaIDs(t, out.String())
			if strings.Contains(out.String(), "Schemas") {
				t.Fatalf("schema output contains Schemas line: %s", out.String())
			}
		})
	}
}

func TestEventSchemaDefaultsToUser(t *testing.T) {
	cmd := newEventSchemaCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{personal.EventSingleChat})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Rule     : singleChat") {
		t.Fatalf("schema output = %s", out.String())
	}
}

func TestEventAsBotRejected(t *testing.T) {
	cmd := newEventListCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--as", "bot"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--as bot is no longer supported; use --as app") {
		t.Fatalf("Execute() error = %v, want bot deprecation error", err)
	}
}

func assertPersonalOutputHidesSchemaIDs(t *testing.T, out string) {
	t.Helper()
	for _, leaked := range []string{"SCHEMA_IDS", "schema_ids", "im_msg_23", "im_msg_29"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("output leaked %q: %s", leaked, out)
		}
	}
}
