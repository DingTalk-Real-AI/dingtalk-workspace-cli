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
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/spf13/cobra"
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
			got := out.String()
			assertPersonalOutputHidesSchemaIDs(t, got)
			for _, eventKey := range []string{
				personal.EventFromUser,
				personal.EventReadO2O,
				personal.EventReadGroup,
				personal.EventRecallO2O,
				personal.EventRecallGroup,
				personal.EventReactionO2O,
				personal.EventReactionGroup,
			} {
				if !strings.Contains(got, eventKey) {
					t.Fatalf("list output missing %s: %s", eventKey, got)
				}
			}
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
	if !strings.Contains(got, personal.EventFromUser) {
		t.Fatalf("list output missing public event %s: %s", personal.EventFromUser, got)
	}
	if strings.Contains(got, "CLIENT_ID") || strings.Contains(got, "ClientSecret") {
		t.Fatalf("list default appears to use legacy application output: %s", got)
	}
}

func TestEventPublicHelpHidesAppMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "consume", cmd: newEventConsumeCommand()},
		{name: "list", cmd: newEventListCommand()},
		{name: "schema", cmd: newEventSchemaCommand()},
		{name: "status", cmd: newEventStatusCommand()},
		{name: "stop", cmd: newEventStopCommand()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			tc.cmd.SetOut(&out)
			tc.cmd.SetArgs([]string{"--help"})
			if tc.name == "schema" {
				tc.cmd.SetArgs([]string{personal.EventSingleChat, "--help"})
			}
			if err := tc.cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			got := out.String()
			for _, hidden := range []string{"--as", "user|app", "应用事件" + " Stream"} {
				if strings.Contains(got, hidden) {
					t.Fatalf("%s help leaked %q:\n%s", tc.name, hidden, got)
				}
			}
		})
	}
}

func TestEventListAppOnlyFlagsRejectedForPersonalEvents(t *testing.T) {
	cmd := newEventListCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--all"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--all are not supported for personal events") {
		t.Fatalf("Execute() error = %v, want unsupported flag validation", err)
	}
}

func TestEventAsAppRejected(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	for _, cmd := range []*cobra.Command{
		newEventListCommand(),
		newEventStatusCommand(),
		newEventConsumeCommand(),
		newEventStopCommand(),
		newEventSchemaCommand(),
	} {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetArgs([]string{"--as", "app"})
		if cmd.Use == "schema <event_key>" {
			cmd.SetArgs([]string{personal.EventSingleChat, "--as", "app"})
		}
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "app event is not publicly available yet") {
			t.Fatalf("%s Execute() error = %v, want public availability guard", cmd.Use, err)
		}
	}
}

func TestEventStatusAppOnlyFlagsRejectedForPersonalEvents(t *testing.T) {
	cmd := newEventStatusCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--all", "--fail-on-orphan"})
	err := cmd.Execute()
	if err == nil ||
		!strings.Contains(err.Error(), "--all") ||
		!strings.Contains(err.Error(), "--fail-on-orphan") ||
		!strings.Contains(err.Error(), "not supported for personal events") {
		t.Fatalf("Execute() error = %v, want unsupported flag validation", err)
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

func TestPersonalEventSchemaDefaultsToTransportEnvelope(t *testing.T) {
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
	for _, field := range []string{"type", "seq", "event_type", "data", "headers", "subscribe_id"} {
		if _, ok := props[field]; !ok {
			t.Fatalf("default envelope schema missing %q: %#v", field, props)
		}
	}
	for _, field := range []string{"content", "sender", "conversation_id", "timestamp"} {
		if _, ok := props[field]; ok {
			t.Fatalf("default envelope schema unexpectedly contains flat field %q", field)
		}
	}
}

func TestPersonalEventFlattenedSchemaUsesSingleJSONSchema(t *testing.T) {
	for _, eventKey := range []string{
		personal.EventMention,
		personal.EventSingleChat,
		personal.EventInChat,
	} {
		t.Run(eventKey, func(t *testing.T) {
			cmd := newEventSchemaCommand()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{eventKey, "--flatten"})
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
			if doc["jq_root_path"] != "." {
				t.Fatalf("jq_root_path = %#v, want .", doc["jq_root_path"])
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

func TestPersonalActionEventSchemaMatchesFlatOutput(t *testing.T) {
	tests := []struct {
		eventKeys  []string
		properties []string
	}{
		{
			eventKeys: []string{personal.EventReadO2O, personal.EventReadGroup},
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "message_id",
				"conversation_id", "reader", "reader_open_dingtalk_id", "sender",
				"sender_open_dingtalk_id", "read_time", "event_time",
			},
		},
		{
			eventKeys: []string{personal.EventRecallO2O, personal.EventRecallGroup},
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "message_id",
				"conversation_id", "recaller", "recaller_open_dingtalk_id", "sender",
				"sender_open_dingtalk_id", "recall_time", "event_time",
			},
		},
		{
			eventKeys: []string{personal.EventReactionO2O, personal.EventReactionGroup},
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "message_id",
				"conversation_id", "operator", "operator_open_dingtalk_id", "reaction_name",
				"reaction_text", "operation_type", "operation_time", "sender",
				"sender_open_dingtalk_id", "event_time",
			},
		},
	}

	for _, tt := range tests {
		for _, eventKey := range tt.eventKeys {
			t.Run(eventKey, func(t *testing.T) {
				cmd := newEventSchemaCommand()
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				var out bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetArgs([]string{eventKey, "--flatten"})
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				var doc map[string]any
				if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
					t.Fatalf("schema output is not JSON: %v\n%s", err, out.String())
				}
				if doc["event_key"] != eventKey || doc["jq_root_path"] != "." {
					t.Fatalf("schema metadata = %#v", doc)
				}
				schema, ok := doc["schema"].(map[string]any)
				if !ok {
					t.Fatalf("schema = %#v", doc["schema"])
				}
				properties, ok := schema["properties"].(map[string]any)
				if !ok || len(properties) != len(tt.properties) {
					t.Fatalf("schema.properties = %#v, want exactly %d flat fields", schema["properties"], len(tt.properties))
				}
				for _, field := range tt.properties {
					if _, ok := properties[field]; !ok {
						t.Fatalf("schema missing %q: %#v", field, properties)
					}
				}
				for _, internal := range []string{"payload", "uid", "corpid", "clientId", "filterSubId", "bizid"} {
					if _, ok := properties[internal]; ok {
						t.Fatalf("schema exposed internal property %q", internal)
					}
				}
			})
		}
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
	if doc["jq_root_path"] != ".data | fromjson" {
		t.Fatalf("jq_root_path = %#v, want default envelope path", doc["jq_root_path"])
	}
}

func TestPersonalEventFromUserIsPubliclyAvailable(t *testing.T) {
	if err := ensurePublicPersonalEvent(personal.EventFromUser); err != nil {
		t.Fatalf("ensurePublicPersonalEvent() error = %v", err)
	}

	schemaCmd := newEventSchemaCommand()
	schemaCmd.SilenceUsage = true
	schemaCmd.SilenceErrors = true
	var schemaOut bytes.Buffer
	schemaCmd.SetOut(&schemaOut)
	schemaCmd.SetArgs([]string{personal.EventFromUser})
	if err := schemaCmd.Execute(); err != nil {
		t.Fatalf("schema Execute() error = %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(schemaOut.Bytes(), &doc); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, schemaOut.String())
	}
	if doc["event_key"] != personal.EventFromUser || doc["rule_type"] != "sender" {
		t.Fatalf("schema document = %#v", doc)
	}

	configDir := setupPersonalIdentityToken(t, &authpkg.TokenData{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       "corp-1",
		UserID:       "user-1",
		ClientID:     "client-1",
	})
	t.Setenv("DWS_CONFIG_DIR", configDir)

	consumeCmd := newEventConsumeCommand()
	consumeCmd.SilenceUsage = true
	consumeCmd.SilenceErrors = true
	consumeCmd.SetArgs([]string{personal.EventFromUser, "--user", "test-user-001", "--dry-run"})
	if err := consumeCmd.Execute(); err != nil {
		t.Fatalf("consume dry-run Execute() error = %v", err)
	}
	openIDConsumeCmd := newEventConsumeCommand()
	openIDConsumeCmd.SilenceUsage = true
	openIDConsumeCmd.SilenceErrors = true
	openIDConsumeCmd.SetArgs([]string{personal.EventFromUser, "--open-dingtalk-id", "open-user-1", "--dry-run"})
	if err := openIDConsumeCmd.Execute(); err != nil {
		t.Fatalf("consume openDingtalkId dry-run Execute() error = %v", err)
	}

	conflictingTargetCmd := newEventConsumeCommand()
	conflictingTargetCmd.SilenceUsage = true
	conflictingTargetCmd.SilenceErrors = true
	conflictingTargetCmd.SetArgs([]string{personal.EventFromUser, "--user", "test-user-001", "--open-dingtalk-id", "open-user-1", "--dry-run"})
	err := conflictingTargetCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--user and --open-dingtalk-id are mutually exclusive for "+personal.EventFromUser) {
		t.Fatalf("conflicting target identity error = %v", err)
	}

	groupOpenIDCmd := newEventConsumeCommand()
	groupOpenIDCmd.SilenceUsage = true
	groupOpenIDCmd.SilenceErrors = true
	groupOpenIDCmd.SetArgs([]string{personal.EventInChat, "--group", "cid-1", "--open-dingtalk-id", "open-user-1", "--dry-run"})
	err = groupOpenIDCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--open-dingtalk-id is not supported for "+personal.EventInChat+"; use --group") {
		t.Fatalf("group openDingtalkId error = %v", err)
	}

	missingUserCmd := newEventConsumeCommand()
	missingUserCmd.SilenceUsage = true
	missingUserCmd.SilenceErrors = true
	missingUserCmd.SetArgs([]string{personal.EventFromUser})
	err = missingUserCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "one of --user or --open-dingtalk-id is required for "+personal.EventFromUser) {
		t.Fatalf("missing target identity error = %v", err)
	}
}

func TestEventConsumeCobraSchemaIncludesOpenDingTalkID(t *testing.T) {
	root := NewRootCommand()
	root.SilenceUsage = true
	root.SilenceErrors = true
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"schema", "event consume"})
	if err := root.Execute(); err != nil {
		t.Fatalf("schema event consume Execute() error = %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out.String())
	}
	params, ok := doc["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("schema parameters = %#v", doc["parameters"])
	}
	if _, ok := params["open-dingtalk-id"]; !ok {
		t.Fatalf("schema parameters missing open-dingtalk-id: %#v", params)
	}
	if _, ok := params["odid"]; ok {
		t.Fatalf("schema parameters unexpectedly include odid alias: %#v", params)
	}
	if _, ok := params["flatten"]; !ok {
		t.Fatalf("schema parameters missing flatten: %#v", params)
	}
	for _, name := range []string{"user", "open-dingtalk-id", "group"} {
		param, ok := params[name].(map[string]any)
		if !ok {
			t.Fatalf("schema parameter %s = %#v", name, params[name])
		}
		if got, exists := param["required_when"]; exists {
			t.Fatalf("schema parameter %s unexpectedly declares required_when = %#v", name, got)
		}
	}
	constraints, ok := doc["constraints"].(map[string]any)
	if !ok {
		t.Fatalf("schema constraints = %#v", doc["constraints"])
	}
	assertJSONConstraintGroup := func(field string, want []string) {
		t.Helper()
		groups, ok := constraints[field].([]any)
		if !ok {
			t.Fatalf("schema constraint %s = %#v", field, constraints[field])
		}
		for _, rawGroup := range groups {
			group, ok := rawGroup.([]any)
			if !ok || len(group) != len(want) {
				continue
			}
			matched := true
			for i := range want {
				if group[i] != want[i] {
					matched = false
					break
				}
			}
			if matched {
				return
			}
		}
		t.Fatalf("schema constraint %s = %#v, missing %#v", field, groups, want)
	}
	assertJSONConstraintGroup("require_one_of", []string{"event_key", "subscribe-id"})
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
	if err == nil || !strings.Contains(err.Error(), "app event is not publicly available yet") {
		t.Fatalf("Execute() error = %v, want public availability guard", err)
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
