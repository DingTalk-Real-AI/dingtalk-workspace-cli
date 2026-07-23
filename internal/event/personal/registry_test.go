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

package personal

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCatalogEnabledEvents(t *testing.T) {
	items := Catalog("", true, false)
	keys := make([]string, 0, len(items))
	for _, item := range items {
		keys = append(keys, item.EventKey)
		if item.Status != StatusEnabled {
			t.Fatalf("%s status = %q, want enabled", item.EventKey, item.Status)
		}
	}
	want := []string{
		EventMention,
		EventSingleChat,
		EventInChat,
		EventFromUser,
		EventReadO2O,
		EventReadGroup,
		EventRecallO2O,
		EventRecallGroup,
		EventReactionO2O,
		EventReactionGroup,
	}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %#v, want %#v", keys, want)
	}
}

func TestEventFromUserIsPublic(t *testing.T) {
	if _, ok := Lookup(EventFromUser); !ok {
		t.Fatalf("Lookup(%q) failed", EventFromUser)
	}
	if !IsPublic(EventFromUser) {
		t.Fatalf("IsPublic(%q) = false, want public", EventFromUser)
	}
}

func TestLegacyEventKeysAreUnknown(t *testing.T) {
	legacyKeys := []string{
		"im_message_receive_at",
		"im_message_receive_o2o",
		"im_message_receive_group",
		"im_message_receive_user",
		"im_message_read_o2o",
		"im_message_read_group",
		"im_message_recall_o2o",
		"im_message_recall_group",
		"im_message_emotion_o2o",
		"im_message_emotion_group",
		"im_message_reaction_o2o",
		"im_message_reaction_group",
		"user_im_message_emotion_o2o",
		"user_im_message_emotion_group",
	}
	for _, key := range legacyKeys {
		if _, ok := Lookup(key); ok {
			t.Fatalf("Lookup(%q) succeeded, want unknown", key)
		}
		if _, _, err := BuildRuleParam(key, RuleOptions{}); err == nil || !strings.Contains(err.Error(), "unknown personal event key") {
			t.Fatalf("BuildRuleParam(%q) error = %v, want unknown personal event key", key, err)
		}
	}
}

func TestDefinitionJSONHidesInternalSchemaIDs(t *testing.T) {
	raw, err := json.Marshal(Definitions())
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	out := string(raw)
	for _, leaked := range []string{"schema_ids", "im_msg_23", "im_msg_29"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("definitions JSON leaked %q: %s", leaked, out)
		}
	}
}

func TestSchemaDocumentsDefaultToTransportEnvelope(t *testing.T) {
	for _, eventKey := range []string{EventMention, EventSingleChat, EventInChat, EventFromUser} {
		t.Run(eventKey, func(t *testing.T) {
			def, ok := Lookup(eventKey)
			if !ok {
				t.Fatalf("Lookup(%q) failed", eventKey)
			}
			doc := BuildSchemaDocument(def)
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			out := string(raw)
			for _, want := range []string{
				"event_key",
				"display_name",
				"description",
				"category",
				"rule_type",
				"required_params",
				"jq_root_path",
				"schema",
				"type",
				"seq",
				"event_id",
				"event_born_time",
				"event_type",
				"subscribe_id",
				"source_id",
				"data",
				"headers",
				"received_at_unix_ms",
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("schema for %s missing %q: %s", eventKey, want, out)
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
				"audit",
				"tenant",
				"subject",
				"traceId",
				"msgIdMetaq",
				"at_users",
				"sender_user_id",
				"sender_open_dingtalk_id",
				"conversation_id",
				"message_id",
				"create_time",
				"event_time",
			} {
				if strings.Contains(out, leaked) {
					t.Fatalf("schema for %s leaked %q: %s", eventKey, leaked, out)
				}
			}
			if doc.JQRootPath != ".data | fromjson" {
				t.Fatalf("jq_root_path = %q, want .data | fromjson", doc.JQRootPath)
			}
			if doc.RequiredParams == nil {
				t.Fatalf("required_params = nil, want empty slice")
			}
			props, ok := doc.Schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema.properties = %#v, want object", doc.Schema["properties"])
			}
			wantProperties := []string{
				"type", "seq", "event_id", "event_born_time", "event_corp_id",
				"event_type", "event_unified_app_id", "event_scope", "subscribe_id",
				"source_id", "rule_type", "data", "headers", "received_at_unix_ms",
			}
			if len(props) != len(wantProperties) {
				t.Fatalf("schema.properties = %#v, want exactly %d transport fields", props, len(wantProperties))
			}
			for _, name := range wantProperties {
				if _, ok := props[name].(map[string]any); !ok {
					t.Fatalf("schema.properties.%s = %#v, want object", name, props[name])
				}
			}
		})
	}
}

func TestFlattenedSchemaDocumentsUseMessageDTO(t *testing.T) {
	for _, eventKey := range []string{EventMention, EventSingleChat, EventInChat, EventFromUser} {
		t.Run(eventKey, func(t *testing.T) {
			def, ok := Lookup(eventKey)
			if !ok {
				t.Fatalf("Lookup(%q) failed", eventKey)
			}
			doc := BuildSchemaDocumentForMode(def, true)
			if doc.JQRootPath != "." {
				t.Fatalf("jq_root_path = %q, want .", doc.JQRootPath)
			}
			props, ok := doc.Schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema.properties = %#v, want object", doc.Schema["properties"])
			}
			wantProperties := []string{
				"type", "event_id", "timestamp", "subscribe_id", "message_id",
				"conversation_id", "sender", "sender_open_dingtalk_id", "content",
				"create_time", "event_time",
			}
			if len(props) != len(wantProperties) {
				t.Fatalf("schema.properties = %#v, want exactly %d DTO fields", props, len(wantProperties))
			}
			for _, name := range wantProperties {
				if _, ok := props[name].(map[string]any); !ok {
					t.Fatalf("schema.properties.%s = %#v, want object", name, props[name])
				}
			}
			for _, transportField := range []string{"data", "headers", "seq", "event_type"} {
				if _, ok := props[transportField]; ok {
					t.Fatalf("flattened schema exposed transport field %q", transportField)
				}
			}
		})
	}
}

func TestTargetUIDEventSchemasRequireUserOrOpenDingTalkID(t *testing.T) {
	wantConstraints := &ParameterConstraints{
		RequireOneOf:      [][]string{{"user", "open-dingtalk-id"}},
		MutuallyExclusive: [][]string{{"user", "open-dingtalk-id"}},
	}
	for _, eventKey := range []string{
		EventSingleChat,
		EventFromUser,
		EventReadO2O,
		EventRecallO2O,
		EventReactionO2O,
	} {
		t.Run(eventKey, func(t *testing.T) {
			def, ok := Lookup(eventKey)
			if !ok {
				t.Fatalf("Lookup(%q) failed", eventKey)
			}
			if len(def.RequiredParams) != 0 {
				t.Fatalf("required_params = %#v, want no unconditional parameters", def.RequiredParams)
			}
			if !reflect.DeepEqual(def.Constraints, wantConstraints) {
				t.Fatalf("constraints = %#v, want %#v", def.Constraints, wantConstraints)
			}
			doc := BuildSchemaDocument(def)
			if !reflect.DeepEqual(doc.Constraints, wantConstraints) {
				t.Fatalf("schema constraints = %#v, want %#v", doc.Constraints, wantConstraints)
			}
		})
	}

	mention, _ := Lookup(EventMention)
	if len(mention.RequiredParams) != 0 {
		t.Fatalf("mention required_params = %#v, want none", mention.RequiredParams)
	}
	if mention.Constraints != nil {
		t.Fatalf("mention constraints = %#v, want none", mention.Constraints)
	}
	group, _ := Lookup(EventInChat)
	if want := []string{"group"}; !reflect.DeepEqual(group.RequiredParams, want) {
		t.Fatalf("group required_params = %#v, want %#v", group.RequiredParams, want)
	}
	if group.Constraints != nil {
		t.Fatalf("group constraints = %#v, want none", group.Constraints)
	}
}

func TestDefinitionCopiesDoNotMutateRegistryConstraints(t *testing.T) {
	def, ok := Lookup(EventSingleChat)
	if !ok {
		t.Fatalf("Lookup(%q) failed", EventSingleChat)
	}
	def.Constraints.RequireOneOf[0][0] = "mutated"

	again, _ := Lookup(EventSingleChat)
	if got := again.Constraints.RequireOneOf[0][0]; got != "user" {
		t.Fatalf("registry constraint mutated through lookup copy: %q", got)
	}
}

func TestActionSchemaDocumentsMatchOutputDTOs(t *testing.T) {
	tests := []struct {
		name       string
		eventKeys  []string
		properties []string
	}{
		{
			name:      "read",
			eventKeys: []string{EventReadO2O, EventReadGroup},
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "message_id",
				"conversation_id", "reader", "reader_open_dingtalk_id", "sender",
				"sender_open_dingtalk_id", "read_time", "event_time",
			},
		},
		{
			name:      "recall",
			eventKeys: []string{EventRecallO2O, EventRecallGroup},
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "message_id",
				"conversation_id", "recaller", "recaller_open_dingtalk_id", "sender",
				"sender_open_dingtalk_id", "recall_time", "event_time",
			},
		},
		{
			name:      "reaction",
			eventKeys: []string{EventReactionO2O, EventReactionGroup},
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
			t.Run(tt.name+"/"+eventKey, func(t *testing.T) {
				def, ok := Lookup(eventKey)
				if !ok {
					t.Fatalf("Lookup(%q) failed", eventKey)
				}
				doc := BuildSchemaDocumentForMode(def, true)
				if doc.JQRootPath != "." {
					t.Fatalf("jq_root_path = %q, want .", doc.JQRootPath)
				}
				props, ok := doc.Schema["properties"].(map[string]any)
				if !ok {
					t.Fatalf("schema.properties = %#v", doc.Schema["properties"])
				}
				if len(props) != len(tt.properties) {
					t.Fatalf("schema properties = %#v, want exactly %d DTO fields", props, len(tt.properties))
				}
				for _, name := range tt.properties {
					if _, ok := props[name].(map[string]any); !ok {
						t.Fatalf("schema.properties.%s = %#v, want object", name, props[name])
					}
				}
				for _, internal := range []string{"payload", "uid", "corpid", "clientId", "filterSubId", "bizid"} {
					if _, ok := props[internal]; ok {
						t.Fatalf("schema exposed internal property %q", internal)
					}
				}
			})
		}
	}
}

func TestBuildRuleParamMention(t *testing.T) {
	rule, param, err := BuildRuleParam(EventMention, RuleOptions{})
	if err != nil {
		t.Fatalf("BuildRuleParam() error = %v", err)
	}
	if rule != "at" {
		t.Fatalf("rule = %q, want at", rule)
	}
	if len(param) != 0 {
		t.Fatalf("param = %#v, want empty map", param)
	}
}

func TestBuildRuleParamSingleChatRequiresPeer(t *testing.T) {
	_, _, err := BuildRuleParam(EventSingleChat, RuleOptions{})
	if err == nil || !strings.Contains(err.Error(), "one of --user or --open-dingtalk-id is required") {
		t.Fatalf("error = %v, want target identity requirement", err)
	}
}

func TestBuildRuleParamSingleChatUserIDMapsToStaffID(t *testing.T) {
	rule, param, err := BuildRuleParam(EventSingleChat, RuleOptions{UserID: "staff-1"})
	if err != nil {
		t.Fatalf("BuildRuleParam() error = %v", err)
	}
	if rule != "singleChat" {
		t.Fatalf("rule = %q, want singleChat", rule)
	}
	if param["targetUidType"] != "staffId" || param["targetUid"] != "staff-1" {
		t.Fatalf("param = %#v", param)
	}
}

func TestBuildRuleParamSingleChatOpenDingTalkID(t *testing.T) {
	rule, param, err := BuildRuleParam(EventSingleChat, RuleOptions{OpenDingTalkID: "open-user-1"})
	if err != nil {
		t.Fatalf("BuildRuleParam() error = %v", err)
	}
	if rule != "singleChat" {
		t.Fatalf("rule = %q, want singleChat", rule)
	}
	if param["targetUidType"] != "openDingtalkId" || param["targetUid"] != "open-user-1" {
		t.Fatalf("param = %#v", param)
	}
}

func TestBuildRuleParamSender(t *testing.T) {
	_, _, err := BuildRuleParam(EventFromUser, RuleOptions{})
	if err == nil || !strings.Contains(err.Error(), "one of --user or --open-dingtalk-id is required") {
		t.Fatalf("error = %v, want sender requirement", err)
	}

	rule, param, err := BuildRuleParam(EventFromUser, RuleOptions{UserID: "staff-1"})
	if err != nil {
		t.Fatalf("BuildRuleParam() error = %v", err)
	}
	if rule != "sender" {
		t.Fatalf("rule = %q, want sender", rule)
	}
	if param["targetUidType"] != "staffId" || param["targetUid"] != "staff-1" {
		t.Fatalf("param = %#v", param)
	}

	rule, param, err = BuildRuleParam(EventFromUser, RuleOptions{OpenDingTalkID: "open-user-1"})
	if err != nil {
		t.Fatalf("BuildRuleParam(openDingtalkId) error = %v", err)
	}
	if rule != "sender" || param["targetUidType"] != "openDingtalkId" || param["targetUid"] != "open-user-1" {
		t.Fatalf("openDingtalkId rule = %q, param = %#v", rule, param)
	}
}

func TestBuildRuleParamGroup(t *testing.T) {
	_, _, err := BuildRuleParam(EventInChat, RuleOptions{})
	if err == nil || !strings.Contains(err.Error(), "--group is required") {
		t.Fatalf("error = %v, want group requirement", err)
	}

	rule, param, err := BuildRuleParam(EventInChat, RuleOptions{GroupID: "cid-1"})
	if err != nil {
		t.Fatalf("BuildRuleParam() error = %v", err)
	}
	if rule != "group" {
		t.Fatalf("rule = %q, want group", rule)
	}
	if param["openConversationId"] != "cid-1" {
		t.Fatalf("param = %#v", param)
	}
}

func TestBuildRuleParamActionEvents(t *testing.T) {
	for _, eventKey := range []string{EventReadO2O, EventRecallO2O, EventReactionO2O} {
		t.Run(eventKey, func(t *testing.T) {
			if _, _, err := BuildRuleParam(eventKey, RuleOptions{}); err == nil || !strings.Contains(err.Error(), "one of --user or --open-dingtalk-id is required for "+eventKey) {
				t.Fatalf("missing target identity error = %v", err)
			}
			if _, _, err := BuildRuleParam(eventKey, RuleOptions{GroupID: "cid-1"}); err == nil || !strings.Contains(err.Error(), "--group is not supported for "+eventKey) {
				t.Fatalf("wrong group error = %v", err)
			}
			rule, param, err := BuildRuleParam(eventKey, RuleOptions{UserID: "staff-1"})
			if err != nil {
				t.Fatal(err)
			}
			if rule != "singleChat" || param["targetUid"] != "staff-1" || param["targetUidType"] != "staffId" {
				t.Fatalf("rule = %q, param = %#v", rule, param)
			}
			rule, param, err = BuildRuleParam(eventKey, RuleOptions{OpenDingTalkID: "open-user-1"})
			if err != nil {
				t.Fatal(err)
			}
			if rule != "singleChat" || param["targetUid"] != "open-user-1" || param["targetUidType"] != "openDingtalkId" {
				t.Fatalf("openDingtalkId rule = %q, param = %#v", rule, param)
			}
		})
	}

	for _, eventKey := range []string{EventReadGroup, EventRecallGroup, EventReactionGroup} {
		t.Run(eventKey, func(t *testing.T) {
			if _, _, err := BuildRuleParam(eventKey, RuleOptions{}); err == nil || !strings.Contains(err.Error(), "--group is required for "+eventKey) {
				t.Fatalf("missing group error = %v", err)
			}
			if _, _, err := BuildRuleParam(eventKey, RuleOptions{UserID: "staff-1"}); err == nil || !strings.Contains(err.Error(), "--user is not supported for "+eventKey) {
				t.Fatalf("wrong user error = %v", err)
			}
			if _, _, err := BuildRuleParam(eventKey, RuleOptions{OpenDingTalkID: "open-user-1"}); err == nil || !strings.Contains(err.Error(), "--open-dingtalk-id is not supported for "+eventKey) {
				t.Fatalf("wrong openDingtalkId error = %v", err)
			}
			rule, param, err := BuildRuleParam(eventKey, RuleOptions{GroupID: "cid-1"})
			if err != nil {
				t.Fatal(err)
			}
			if rule != "group" || param["openConversationId"] != "cid-1" {
				t.Fatalf("rule = %q, param = %#v", rule, param)
			}
		})
	}
}

func TestBuildRuleParamRejectsWrongScopedFlags(t *testing.T) {
	if _, _, err := BuildRuleParam(EventMention, RuleOptions{UserID: "staff-1"}); err == nil || !strings.Contains(err.Error(), "--user is not supported for "+EventMention) {
		t.Fatalf("mention with user error = %v, want unsupported user", err)
	}
	if _, _, err := BuildRuleParam(EventSingleChat, RuleOptions{UserID: "staff-1", GroupID: "cid-1"}); err == nil || !strings.Contains(err.Error(), "--group is not supported for "+EventSingleChat) {
		t.Fatalf("singleChat with group error = %v, want unsupported group", err)
	}
	if _, _, err := BuildRuleParam(EventSingleChat, RuleOptions{UserID: "staff-1", OpenDingTalkID: "open-user-1"}); err == nil || !strings.Contains(err.Error(), "--user and --open-dingtalk-id are mutually exclusive for "+EventSingleChat) {
		t.Fatalf("singleChat with both identities error = %v, want mutually exclusive", err)
	}
	if _, _, err := BuildRuleParam(EventMention, RuleOptions{OpenDingTalkID: "open-user-1"}); err == nil || !strings.Contains(err.Error(), "--open-dingtalk-id is not supported for "+EventMention) {
		t.Fatalf("mention with openDingtalkId error = %v, want unsupported openDingtalkId", err)
	}
	if _, _, err := BuildRuleParam(EventInChat, RuleOptions{UserID: "staff-1", GroupID: "cid-1"}); err == nil || !strings.Contains(err.Error(), "--user is not supported for "+EventInChat) {
		t.Fatalf("group with user error = %v, want unsupported user", err)
	}
	if _, _, err := BuildRuleParam(EventInChat, RuleOptions{OpenDingTalkID: "open-user-1", GroupID: "cid-1"}); err == nil || !strings.Contains(err.Error(), "--open-dingtalk-id is not supported for "+EventInChat) {
		t.Fatalf("group with openDingtalkId error = %v, want unsupported openDingtalkId", err)
	}
}

func TestBuildFilterQueryAndJSON(t *testing.T) {
	filter, canonical, err := BuildFilter(`{"field":"conversation_id","op":"eq","value":"cid1"}`, "P0, 故障")
	if err != nil {
		t.Fatalf("BuildFilter() error = %v", err)
	}
	m := filter.(map[string]any)
	parts := m["and"].([]any)
	if len(parts) != 2 {
		t.Fatalf("parts len = %d, want 2", len(parts))
	}
	if !strings.Contains(canonical, "contains_any") {
		t.Fatalf("canonical = %s, want contains_any", canonical)
	}
	if !strings.Contains(canonical, "payload.body.content") || strings.Contains(canonical, "message.text") {
		t.Fatalf("canonical = %s, want query filter on payload.body.content only", canonical)
	}
	if !strings.Contains(canonical, "payload.body.openConversationId") || strings.Contains(canonical, "conversation_id") {
		t.Fatalf("canonical = %s, want conversation_id alias mapped to payload.body.openConversationId", canonical)
	}
}

func TestIdempotencyKeyUsesLocalIdentityKey(t *testing.T) {
	left := Identity{LocalSubject: "refresh:left", ClientID: "client-1", SourceID: "open"}
	right := Identity{LocalSubject: "refresh:right", ClientID: "client-1", SourceID: "open"}
	ruleParam := map[string]any{"targetUid": "test-user-001", "targetUidType": "staffId"}
	leftKey := IdempotencyKey(left, EventSingleChat, "singleChat", ruleParam, "")
	rightKey := IdempotencyKey(right, EventSingleChat, "singleChat", ruleParam, "")
	if leftKey == rightKey {
		t.Fatalf("idempotency key collapsed for different local subjects: %s", leftKey)
	}
}

func TestIdempotencyKeySeparatesTargetUIDTypes(t *testing.T) {
	identity := Identity{LocalSubject: "refresh:subject", ClientID: "client-1", SourceID: "open"}
	staffIDKey := IdempotencyKey(identity, EventSingleChat, "singleChat", map[string]any{
		"targetUid":     "same-value",
		"targetUidType": "staffId",
	}, "")
	openIDKey := IdempotencyKey(identity, EventSingleChat, "singleChat", map[string]any{
		"targetUid":     "same-value",
		"targetUidType": "openDingtalkId",
	}, "")
	if staffIDKey == openIDKey {
		t.Fatalf("idempotency key collapsed for different targetUidType: %s", staffIDKey)
	}
}
