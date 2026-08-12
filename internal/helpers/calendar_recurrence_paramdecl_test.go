package helpers

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestCalendarRecurrenceParamDeclsCoverHintPins(t *testing.T) {
	decls := calendarRecurrenceParamDecls()
	byName := map[string]contract.ParamDecl{}
	for _, d := range decls {
		byName[d.Name] = d
	}
	wantWhen := map[string]string{
		"recurrence-day-of-month": "recurrence-type is absoluteMonthly or absoluteYearly",
		"recurrence-days-of-week": "recurrence-type is weekly or relativeMonthly",
		"recurrence-index":        "recurrence-type is relativeMonthly",
		"recurrence-interval":     "any recurrence-* flag is provided",
		"recurrence-type":         "any recurrence-* flag is provided",
		"recurrence-end-date":     "recurrence-range-type is endDate",
		"recurrence-count":        "recurrence-range-type is numbered",
		"recurrence-range-type":   "any recurrence-* flag is provided",
	}
	wantProperty := map[string]string{
		"recurrence-day-of-month":      "recurrence.pattern.dayOfMonth",
		"recurrence-days-of-week":      "recurrence.pattern.daysOfWeek",
		"recurrence-index":             "recurrence.pattern.index",
		"recurrence-interval":          "recurrence.pattern.interval",
		"recurrence-type":              "recurrence.pattern.type",
		"recurrence-end-date":          "recurrence.range.endDate",
		"recurrence-count":             "recurrence.range.numberOfOccurrences",
		"recurrence-range-type":        "recurrence.range.type",
		"recurrence-first-day-of-week": "recurrence.pattern.firstDayOfWeek",
	}
	if len(decls) != len(wantProperty) {
		t.Fatalf("calendarRecurrenceParamDecls len = %d, want %d: %#v", len(decls), len(wantProperty), decls)
	}
	for name, when := range wantWhen {
		d, ok := byName[name]
		if !ok {
			t.Fatalf("missing ParamDecl %q", name)
		}
		if d.Required == nil || *d.Required {
			t.Fatalf("%s Required = %#v, want explicit false", name, d.Required)
		}
		if d.RequiredWhen != when {
			t.Fatalf("%s RequiredWhen = %q, want %q", name, d.RequiredWhen, when)
		}
	}
	for name, prop := range wantProperty {
		d, ok := byName[name]
		if !ok {
			t.Fatalf("missing ParamDecl %q", name)
		}
		if d.Property != prop {
			t.Fatalf("%s Property = %q, want %q", name, d.Property, prop)
		}
	}
}
