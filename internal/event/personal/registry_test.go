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
	"strings"
	"testing"
)

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
	peer := param["peer"].(map[string]any)
	if peer["id_type"] != "unionId" || peer["id"] != "union-1" {
		t.Fatalf("peer = %#v", peer)
	}
}

func TestBuildRuleParamPending(t *testing.T) {
	_, _, err := BuildRuleParam(EventFromUser, RuleOptions{SenderUserID: "u1"})
	if !IsSchemaPending(err) {
		t.Fatalf("error = %v, want schema pending", err)
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
