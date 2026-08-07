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

package chatmsg

import (
	"reflect"
	"testing"
)

func TestSearchItemsCarriesGroupedConversationIdentity(t *testing.T) {
	data := map[string]any{
		"result": map[string]any{
			"conversationMessagesList": []any{
				map[string]any{
					"openConversationId": "cid-group",
					"title":              "项目群",
					"singleChat":         false,
					"messages": []any{
						map[string]any{"openMessageId": "m1", "content": "hello"},
					},
				},
			},
		},
	}

	items := SearchItems(data)
	if len(items) != 1 || items[0]["openConversationId"] != "cid-group" ||
		items[0]["conversationTitle"] != "项目群" || items[0]["singleChat"] != false {
		t.Fatalf("items = %#v", items)
	}
}

func TestFilterConversationScopeDropsOtherConversationsAndRejectsMissingIdentity(t *testing.T) {
	messages := []map[string]any{
		{"openMessageId": "m1", "openConversationId": "cid-target"},
		{"openMessageId": "m2", "openConversationId": "cid-other"},
		{"openMessageId": "m3"},
	}

	matched, missing := FilterConversationScope(messages, []string{"cid-target"})
	if len(matched) != 1 || matched[0]["openMessageId"] != "m1" {
		t.Fatalf("matched = %#v", matched)
	}
	if !reflect.DeepEqual(missing, []string{"m3"}) {
		t.Fatalf("missing = %#v", missing)
	}
}

func TestGroupSearchMessagesPreservesFirstSeenOrder(t *testing.T) {
	messages := []map[string]any{
		{"openMessageId": "m1", "openConversationId": "cid-b", "conversationTitle": "B"},
		{"openMessageId": "m2", "openConversationId": "cid-a", "conversationTitle": "A"},
		{"openMessageId": "m3", "openConversationId": "cid-b", "conversationTitle": "B"},
	}

	groups := GroupSearchMessages(messages)
	if len(groups) != 2 || groups[0]["openConversationId"] != "cid-b" || groups[1]["openConversationId"] != "cid-a" {
		t.Fatalf("groups = %#v", groups)
	}
	groupMessages, _ := groups[0]["messages"].([]map[string]any)
	if len(groupMessages) != 2 || groupMessages[1]["openMessageId"] != "m3" {
		t.Fatalf("group messages = %#v", groupMessages)
	}
}
