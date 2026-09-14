// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package calendarcompat

import "testing"

func TestCrossPlatformCoverageNormalizeTerminalEmptyEvents(t *testing.T) {
	sentinel := []any{map[string]any{
		"attendees": nil, "categories": nil, "meetingRooms": nil, "reminders": nil,
	}}
	if got, ok := NormalizeTerminalEmptyEvents(sentinel, map[string]any{"hasMore": false}); !ok || got == nil || len(got) != 0 {
		t.Fatalf("terminal sentinel = %#v, %v; want explicit empty slice", got, ok)
	}

	for name, items := range map[string][]any{
		"empty object":   {map[string]any{}},
		"missing fields": {map[string]any{"attendees": nil}},
		"unknown field": {map[string]any{
			"attendees": nil, "categories": nil, "meetingRooms": nil, "summary": nil,
		}},
		"non-null field": {map[string]any{
			"attendees": []any{}, "categories": nil, "meetingRooms": nil, "reminders": nil,
		}},
		"mixed rows": {sentinel[0], map[string]any{"id": "event-1"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := NormalizeTerminalEmptyEvents(items, map[string]any{"hasMore": false}); ok {
				t.Fatalf("invalid sentinel accepted: %#v", items)
			}
		})
	}
	for name, container := range map[string]map[string]any{
		"missing terminal evidence": {},
		"more pages":                {"hasMore": true},
		"cursor":                    {"hasMore": false, "nextCursor": "next"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := NormalizeTerminalEmptyEvents(sentinel, container); ok {
				t.Fatalf("non-terminal sentinel accepted with %#v", container)
			}
		})
	}
}
