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

package smart

import (
	"strings"
	"time"
)

// calendarDayRange returns the [00:00, next-day 00:00) window for "today plus
// offsetDays" in the local timezone. Shared by the day-scoped calendar
// shortcuts (+today offset 0, +tomorrow offset 1, +conflicts --in-days).
func calendarDayRange(offsetDays int) (time.Time, time.Time) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, offsetDays)
	return start, start.AddDate(0, 0, 1)
}

// calendarProjectEvents lists and projects the events in a list_calendar_events
// response into clean {title,start,end,location,eventId} items, skipping events
// that carry no id. Shared by +today / +tomorrow / +week so the projection lives
// in exactly one place (reuses shortcutNextEventProject).
func calendarProjectEvents(data map[string]any) []map[string]any {
	events := make([]map[string]any, 0)
	for _, e := range shortcutNextEventList(data) {
		if id, _ := e["id"].(string); strings.TrimSpace(id) == "" {
			continue
		}
		events = append(events, shortcutNextEventProject(e))
	}
	return events
}
