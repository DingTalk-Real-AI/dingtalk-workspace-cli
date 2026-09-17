// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestCrossPlatformCoverageChatMessageRangeAllResultMatchesProjectedFields(t *testing.T) {
	normalized, err := contract.NormalizeResultSpec(chatMessageRangeAllResult(), "chat.search_messages_by_time_range")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(normalized.DataSchema, &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	for _, name := range []string{
		"result", "count", "conversationCount", "pagesFetched", "paginationKnown",
		"complete", "hasMore", "stopReason", "truncated", "failedCount", "failures",
		"partial", "queryRange", "nextActions", "paging",
	} {
		if _, ok := properties[name]; !ok {
			t.Errorf("declared Result missing projected field %q", name)
		}
	}
	result := properties["result"].(map[string]any)
	groups := result["properties"].(map[string]any)["conversationMessagesList"].(map[string]any)
	group := groups["items"].(map[string]any)
	messages := group["properties"].(map[string]any)["messages"].(map[string]any)
	message := messages["items"].(map[string]any)
	textSchema := message["properties"].(map[string]any)["text"].(map[string]any)
	if got := textSchema["type"]; !reflect.DeepEqual(got, []any{"string", "null"}) {
		t.Fatalf("message text type = %#v, want nullable string", got)
	}
}
