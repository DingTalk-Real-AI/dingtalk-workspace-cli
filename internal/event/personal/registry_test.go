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
	}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %#v, want %#v", keys, want)
	}
}

func TestLegacyEventKeysAreUnknown(t *testing.T) {
	legacyKeys := []string{
		"im_message_receive_at",
		"im_message_receive_o2o",
		"im_message_receive_group",
		"im_message_receive_user",
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
	if err == nil || !strings.Contains(err.Error(), "--peer-user-id") {
		t.Fatalf("error = %v, want peer requirement", err)
	}

	rule, param, err := BuildRuleParam(EventSingleChat, RuleOptions{PeerUnionID: "union-1"})
	if err != nil {
		t.Fatalf("BuildRuleParam() error = %v", err)
	}
	if rule != "singleChat" {
		t.Fatalf("rule = %q, want singleChat", rule)
	}
	if param["targetUidType"] != "unionId" || param["targetUid"] != "union-1" {
		t.Fatalf("param = %#v", param)
	}
}

func TestBuildRuleParamSingleChatUserIDMapsToStaffID(t *testing.T) {
	rule, param, err := BuildRuleParam(EventSingleChat, RuleOptions{PeerUserID: "staff-1"})
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

func TestBuildRuleParamSender(t *testing.T) {
	_, _, err := BuildRuleParam(EventFromUser, RuleOptions{})
	if err == nil || !strings.Contains(err.Error(), "--sender-user-id") {
		t.Fatalf("error = %v, want sender requirement", err)
	}

	rule, param, err := BuildRuleParam(EventFromUser, RuleOptions{SenderUserID: "staff-1"})
	if err != nil {
		t.Fatalf("BuildRuleParam() error = %v", err)
	}
	if rule != "sender" {
		t.Fatalf("rule = %q, want sender", rule)
	}
	if param["targetUidType"] != "staffId" || param["targetUid"] != "staff-1" {
		t.Fatalf("param = %#v", param)
	}
}

func TestBuildRuleParamGroup(t *testing.T) {
	_, _, err := BuildRuleParam(EventInChat, RuleOptions{})
	if err == nil || !strings.Contains(err.Error(), "--open-conversation-id") {
		t.Fatalf("error = %v, want open conversation requirement", err)
	}

	rule, param, err := BuildRuleParam(EventInChat, RuleOptions{OpenConversationID: "cid-1"})
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

func TestBuildFilterKeywordAndJSON(t *testing.T) {
	filter, canonical, err := BuildFilter(`{"field":"chat.openConversationId","op":"eq","value":"cid1"}`, "P0, 故障")
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
}

func TestIdempotencyKeyUsesLocalIdentityKey(t *testing.T) {
	left := Identity{LocalSubject: "refresh:left", ClientID: "client-1", SourceID: "open"}
	right := Identity{LocalSubject: "refresh:right", ClientID: "client-1", SourceID: "open"}
	ruleParam := map[string]any{"targetUid": "507971", "targetUidType": "staffId"}
	leftKey := IdempotencyKey(left, EventSingleChat, "singleChat", ruleParam, "")
	rightKey := IdempotencyKey(right, EventSingleChat, "singleChat", ruleParam, "")
	if leftKey == rightKey {
		t.Fatalf("idempotency key collapsed for different local subjects: %s", leftKey)
	}
}
