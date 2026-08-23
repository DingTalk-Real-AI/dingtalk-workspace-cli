// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package calendarcompat contains narrow compatibility adapters for observed
// Calendar wire shapes shared by the calendar and smart Shortcut packages.
package calendarcompat

// NormalizeTerminalEmptyEvents recognizes the Calendar service's legacy empty
// page sentinel. Some deployments encode an exhausted empty page as one object
// whose collection fields are all null instead of events:[]. Only that exact
// terminal shape is normalized; arbitrary id-less or mixed event rows remain
// available to the strict caller validation and must fail closed.
func NormalizeTerminalEmptyEvents(items []any, container map[string]any) ([]any, bool) {
	if len(items) != 1 || container == nil {
		return items, false
	}
	hasMore, ok := container["hasMore"].(bool)
	if !ok || hasMore || hasNonEmptyCursor(container) {
		return items, false
	}
	placeholder, ok := items[0].(map[string]any)
	knownNullFields := map[string]struct{}{
		"attendees":    {},
		"categories":   {},
		"meetingRooms": {},
		"reminders":    {},
	}
	if !ok || len(placeholder) != len(knownNullFields) {
		return items, false
	}
	for key, value := range placeholder {
		if _, known := knownNullFields[key]; !known || value != nil {
			return items, false
		}
	}
	return []any{}, true
}

func hasNonEmptyCursor(container map[string]any) bool {
	for _, key := range []string{"nextCursor", "nextToken", "pageToken"} {
		value, present := container[key]
		if !present || value == nil || value == "" {
			continue
		}
		return true
	}
	return false
}
