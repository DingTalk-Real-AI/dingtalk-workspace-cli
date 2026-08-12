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
	"fmt"
	"strings"
)

// SearchItems locates and flattens the message list returned by the two
// DingTalk message-search interfaces. Grouped search responses carry the
// conversation identity on the group rather than each message, so the
// flattener copies that identity onto every returned message before callers
// perform scope checks.
func SearchItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	for _, root := range []map[string]any{data, childMap(data, "result")} {
		if root == nil {
			continue
		}
		if groups := searchMapSlice(root["conversationMessagesList"]); groups != nil {
			return flattenSearchGroups(groups)
		}
	}
	keys := []string{"list", "messages", "messageList", "items", "data", "records", "result"}
	for _, key := range keys {
		if items := searchMapSlice(data[key]); items != nil {
			return items
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, innerKey := range []string{"list", "messages", "messageList", "items", "data", "records"} {
				if items := searchMapSlice(inner[innerKey]); items != nil {
					return items
				}
			}
		}
	}
	return nil
}

func childMap(data map[string]any, key string) map[string]any {
	if value, ok := data[key].(map[string]any); ok {
		return value
	}
	return nil
}

func flattenSearchGroups(groups []map[string]any) []map[string]any {
	out := make([]map[string]any, 0)
	for _, group := range groups {
		messages := searchMapSlice(group["messages"])
		if messages == nil {
			continue
		}
		conversationID := cleanSearchScalar(group["openConversationId"])
		conversationTitle := cleanSearchScalar(group["title"])
		singleChat, hasSingleChat := group["singleChat"]
		for _, message := range messages {
			item := make(map[string]any, len(message)+3)
			for key, value := range message {
				item[key] = value
			}
			if _, exists := item["openConversationId"]; !exists && conversationID != "" {
				item["openConversationId"] = conversationID
			}
			if _, exists := item["conversationTitle"]; !exists && conversationTitle != "" {
				item["conversationTitle"] = conversationTitle
			}
			if _, exists := item["singleChat"]; !exists && hasSingleChat {
				item["singleChat"] = singleChat
			}
			out = append(out, item)
		}
	}
	return out
}

func searchMapSlice(value any) []map[string]any {
	switch items := value.(type) {
	case []map[string]any:
		return items
	case []any:
		return searchMaps(items)
	default:
		return nil
	}
}

func searchMaps(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if message, ok := item.(map[string]any); ok {
			out = append(out, message)
		}
	}
	return out
}

// FilterConversationScope keeps only messages belonging to the explicitly
// requested conversations. A message without a conversation identity is
// reported as unverifiable rather than treated as in-scope.
func FilterConversationScope(messages []map[string]any, conversationIDs []string) (matched []map[string]any, unverifiableMessageIDs []string) {
	requested := make(map[string]struct{}, len(conversationIDs))
	for _, conversationID := range conversationIDs {
		if value := strings.TrimSpace(conversationID); value != "" {
			requested[value] = struct{}{}
		}
	}
	matched = make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		conversationID := cleanSearchScalar(ConversationID(message))
		if conversationID == "" {
			messageID := cleanSearchScalar(MessageID(message))
			if messageID == "" {
				messageID = "<unknown>"
			}
			unverifiableMessageIDs = append(unverifiableMessageIDs, messageID)
			continue
		}
		if _, ok := requested[conversationID]; ok {
			matched = append(matched, message)
		}
	}
	return matched, uniqueStrings(unverifiableMessageIDs)
}

// GroupSearchMessages restores the established typed search envelope after a
// client-side scoped scan. Group order follows first occurrence in the search
// result, and each message retains its original fields.
func GroupSearchMessages(messages []map[string]any) []map[string]any {
	groups := make([]map[string]any, 0)
	index := make(map[string]int)
	for _, message := range messages {
		conversationID := cleanSearchScalar(ConversationID(message))
		if conversationID == "" {
			continue
		}
		groupIndex, ok := index[conversationID]
		if !ok {
			group := map[string]any{
				"openConversationId": conversationID,
				"messages":           []map[string]any{},
			}
			if title := cleanSearchScalar(message["conversationTitle"]); title != "" {
				group["title"] = title
			}
			if singleChat, exists := message["singleChat"]; exists {
				group["singleChat"] = singleChat
			}
			groups = append(groups, group)
			groupIndex = len(groups) - 1
			index[conversationID] = groupIndex
		}
		groupMessages := groups[groupIndex]["messages"].([]map[string]any)
		groups[groupIndex]["messages"] = append(groupMessages, message)
	}
	return groups
}

func cleanSearchScalar(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" || strings.EqualFold(text, "null") {
		return ""
	}
	return text
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
