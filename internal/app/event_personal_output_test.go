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
	"encoding/json"
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
		{name: "default", args: []string{personal.EventSingleChat, "--as", "user"}},
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

func TestPersonalEventSchemaUsesSingleJSONSchema(t *testing.T) {
	for _, eventKey := range []string{
		personal.EventMention,
		personal.EventSingleChat,
		personal.EventInChat,
		personal.EventFromUser,
	} {
		t.Run(eventKey, func(t *testing.T) {
			cmd := newEventSchemaCommand()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{eventKey})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			got := out.String()
			var doc map[string]any
			if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
				t.Fatalf("schema output for %s is not JSON: %v\n%s", eventKey, err, got)
			}
			for _, want := range []string{
				"event_key",
				"display_name",
				"description",
				"category",
				"rule_type",
				"required_params",
				"jq_root_path",
				"schema",
				"event_id",
				"timestamp",
				"subscribe_id",
				"content",
				"sender",
				"sender_open_dingtalk_id",
				"conversation_id",
				"message_id",
				"create_time",
				"event_time",
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("schema output for %s missing %q: %s", eventKey, want, got)
				}
			}
			for _, leaked := range []string{
				"message.text",
				"chat.openConversationId",
				"sender.userId",
				"sender.unionId",
				"auth",
				"resolved_output_schema",
				"decoded_data_schema",
				"filter_schema",
				"payload_schema",
				"output_schema",
				"data_json_path",
				"headers",
				"audit",
				"tenant",
				"subject",
				"traceId",
				"msgIdMetaq",
				"at_users",
				"sender_user_id",
			} {
				if strings.Contains(got, leaked) {
					t.Fatalf("schema output for %s leaked %q: %s", eventKey, leaked, got)
				}
			}
			if doc["jq_root_path"] != ".data | fromjson" {
				t.Fatalf("jq_root_path = %#v, want .data | fromjson", doc["jq_root_path"])
			}
			schema, ok := doc["schema"].(map[string]any)
			if !ok {
				t.Fatalf("schema = %#v, want object", doc["schema"])
			}
			props, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema.properties = %#v, want object", schema["properties"])
			}
			if _, ok := props["content"].(map[string]any); !ok {
				t.Fatalf("schema.properties.content = %#v, want object", props["content"])
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
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out.String())
	}
	if doc["event_key"] != personal.EventSingleChat {
		t.Fatalf("event_key = %#v, want %s", doc["event_key"], personal.EventSingleChat)
	}
}

func TestPersonalEventSchemaRejectsTableFormat(t *testing.T) {
	cmd := newEventSchemaCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{personal.EventSingleChat, "--format", "table"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "event schema only supports json output") {
		t.Fatalf("Execute() error = %v, want json-only format validation", err)
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
