// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import "testing"

func TestShortcutTodoCardsUnwrapsNestedResult(t *testing.T) {
	data := map[string]any{
		"result": map[string]any{
			"result": map[string]any{
				"todoCards": []any{
					map[string]any{
						"subject": "DWS shortcut 真实测试待办",
						"taskId":  "55155814691",
					},
				},
			},
		},
	}

	cards := shortcutTodoCards(data)
	if len(cards) != 1 {
		t.Fatalf("len(cards)=%d, want 1", len(cards))
	}
	if got := cards[0]["subject"]; got != "DWS shortcut 真实测试待办" {
		t.Fatalf("subject=%v", got)
	}
	if got := cards[0]["taskId"]; got != "55155814691" {
		t.Fatalf("taskId=%v", got)
	}
}
