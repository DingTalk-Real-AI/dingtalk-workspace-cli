package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func calendarTimeMigrationFixture(action, name string) (toolSchema, toolSchema) {
	path := "calendar event " + action
	old := parameterSchema{Type: `"string"`, Property: name + "DateTime", Format: "date-time", Required: action == "create"}
	next := old
	next.Format, next.AnyOf = "", calendarTimeFormatAlternatives
	if action == "update" {
		next.RequiredWhen = calendarAllDayRequiredWhen
	}
	return toolSchema{PrimaryCLIPath: path, Parameters: map[string]parameterSchema{name: old}},
		toolSchema{PrimaryCLIPath: path, Parameters: map[string]parameterSchema{name: next, "is-all-day": {Type: `"boolean"`, Property: "isAllDay"}}}
}

func TestCrossPlatformCoverageCalendarTimeFormatMigration(t *testing.T) {
	for _, action := range []string{"create", "update"} {
		for _, name := range []string{"start", "end"} {
			t.Run(action+"/"+name, func(t *testing.T) {
				old, next := calendarTimeMigrationFixture(action, name)
				path := "calendar/calendar." + action + "_calendar_event"
				if !compatibleReviewedCalendarTimeFormats(path, name, old, next) {
					t.Fatal("exact migration rejected")
				}
				if got := checkToolCompatibility(path, old, next); len(got) != 0 {
					t.Fatalf("migration: %v", got)
				}
				if compatibleReviewedCalendarTimeFormats(path, name, next, old) {
					t.Fatal("reverse migration accepted")
				}
			})
		}
	}
	mutations := map[string]func(*parameterSchema){
		"type":           func(p *parameterSchema) { p.Type = `"integer"` },
		"property":       func(p *parameterSchema) { p.Property = "other" },
		"default":        func(p *parameterSchema) { p.Default = `"now"` },
		"required":       func(p *parameterSchema) { p.Required = true },
		"cli_required":   func(p *parameterSchema) { p.CLIRequired = true },
		"interface_type": func(p *parameterSchema) { p.InterfaceType = "other" },
		"enum":           func(p *parameterSchema) { p.Enum = []string{"today"} },
		"required_when":  func(p *parameterSchema) { p.RequiredWhen = "always" },
		"missing_union":  func(p *parameterSchema) { p.AnyOf = "" },
		"narrow_union":   func(p *parameterSchema) { p.AnyOf = `[{"format":"date"}]` },
		"extra_branch":   func(p *parameterSchema) { p.AnyOf = `[{"format":"date"},{"format":"date-time"},{}]` },
		"top_format":     func(p *parameterSchema) { p.Format = "date-time" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			old, next := calendarTimeMigrationFixture("update", "start")
			p := next.Parameters["start"]
			mutate(&p)
			next.Parameters["start"] = p
			if compatibleReviewedCalendarTimeFormats("calendar/calendar.update_calendar_event", "start", old, next) {
				t.Fatal("unreviewed change accepted")
			}
			if len(checkToolCompatibility("calendar/calendar.update_calendar_event", old, next)) == 0 {
				t.Fatal("unreviewed change passed full checker")
			}
		})
	}
	for _, mutation := range []string{"missing selector", "required selector", "default selector", "existing selector", "other tool", "other parameter"} {
		t.Run(mutation, func(t *testing.T) {
			old, next := calendarTimeMigrationFixture("update", "start")
			path, name := "calendar/calendar.update_calendar_event", "start"
			selector := next.Parameters["is-all-day"]
			switch mutation {
			case "missing selector":
				delete(next.Parameters, "is-all-day")
			case "required selector":
				selector.Required = true
				next.Parameters["is-all-day"] = selector
			case "default selector":
				selector.Default = "true"
				next.Parameters["is-all-day"] = selector
			case "existing selector":
				old.Parameters["is-all-day"] = selector
			case "other tool":
				path = "calendar/calendar.list_calendar_events"
			case "other parameter":
				name = "recurrence-end-date"
			}
			if compatibleReviewedCalendarTimeFormats(path, name, old, next) {
				t.Fatal("unreviewed scope accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageNormalizeParameterPreservesAnyOf(t *testing.T) {
	raw := json.RawMessage(`{"type":"string","required":false,"field_provenance":{},"anyOf":[{"format":"date"},{"format":"date-time"}]}`)
	p, err := normalizeParameter(raw)
	if err != nil || p.AnyOf != calendarTimeFormatAlternatives {
		t.Fatalf("normalized: %+v, %v", p, err)
	}
	next := p
	next.AnyOf = `[{"format":"date"}]`
	got := checkParameterCompatibility("sample/sample.run", "time", p, next)
	if !strings.Contains(strings.Join(got, "\n"), "changed anyOf") {
		t.Fatalf("anyOf change ignored: %v", got)
	}
}
