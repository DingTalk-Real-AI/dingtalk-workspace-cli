package main

import "reflect"

const calendarTimeFormatAlternatives = `[{"format":"date"},{"format":"date-time"}]`
const calendarAllDayRequiredWhen = "is-all-day is explicitly provided (true or false)"

// compatibleReviewedCalendarTimeFormats is a base-owned, one-way migration.
// Only the four calendar time parameters may replace date-time with the exact
// date/date-time union. The new optional is-all-day selector preserves every
// historical invocation; update requires both times only when that new selector
// is explicitly supplied. Everything else in each parameter must be identical.
// Like reviewedParameterTypeChanges, this authorization must land in main before
// a feature may consume it. It is not a general format/required_when relaxation.
func compatibleReviewedCalendarTimeFormats(toolPath, name string, oldTool, newTool toolSchema) bool {
	var cliPath, requiredWhen string
	switch toolPath {
	case "calendar/calendar.create_calendar_event":
		cliPath = "calendar event create"
	case "calendar/calendar.update_calendar_event":
		cliPath = "calendar event update"
		requiredWhen = calendarAllDayRequiredWhen
	default:
		return false
	}
	property := map[string]string{"start": "startDateTime", "end": "endDateTime"}[name]
	if property == "" || oldTool.PrimaryCLIPath != cliPath || newTool.PrimaryCLIPath != cliPath {
		return false
	}
	if _, existed := oldTool.Parameters["is-all-day"]; existed {
		return false
	}
	selector, exists := newTool.Parameters["is-all-day"]
	if !exists || selector.Type != `"boolean"` || selector.Property != "isAllDay" ||
		selector.Required || selector.CLIRequired || selector.RequiredWhen != "" ||
		selector.Default != "" || selector.InterfaceDefault != "" || selector.Format != "" ||
		selector.AnyOf != "" || len(selector.Enum) != 0 {
		return false
	}
	old, oldExists := oldTool.Parameters[name]
	next, newExists := newTool.Parameters[name]
	if !oldExists || !newExists || old.Type != `"string"` || old.Property != property ||
		old.Format != "date-time" || old.AnyOf != "" || old.RequiredWhen != "" ||
		next.Format != "" || next.AnyOf != calendarTimeFormatAlternatives || next.RequiredWhen != requiredWhen {
		return false
	}
	old.Format, old.AnyOf, old.RequiredWhen = next.Format, next.AnyOf, next.RequiredWhen
	return reflect.DeepEqual(old, next)
}
