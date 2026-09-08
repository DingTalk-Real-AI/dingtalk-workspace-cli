package app

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageCalendarEventOptionsFinalSchema(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)
	for _, action := range []string{"create", "update"} {
		tool := snapshot.Tools["calendar."+action+"_calendar_event"]
		if tool == nil {
			t.Fatalf("missing %s", action)
		}
		parameters := schemaContractMap(tool["parameters"])
		for flag, property := range map[string]string{"is-all-day": "isAllDay", "add-online-meeting": "onlineMeeting.add"} {
			p := parameters[flag]
			if p == nil {
				t.Fatalf("%s missing %s", action, flag)
			}
			if got := schemaContractString(p["property"]); got != property {
				t.Errorf("%s %s property=%s", action, flag, got)
			}
			if got := schemaContractString(p["type"]); got != "boolean" {
				t.Errorf("%s %s type=%s", action, flag, got)
			}
			if p["required"] == true {
				t.Errorf("%s %s became required", action, flag)
			}
		}
		meeting := parameters["add-online-meeting"]
		if schemaContractString(meeting["default"]) != "true" {
			t.Fatalf("%s meeting default=%v", action, meeting["default"])
		}
		if !strings.Contains(schemaContractString(meeting["description"]), "不传") {
			t.Fatalf("%s missing omission semantics", action)
		}
		for _, name := range []string{"start", "end"} {
			if !strings.Contains(schemaContractString(parameters[name]["description"]), "yyyy-MM-dd") {
				t.Fatalf("%s %s missing all-day date format", action, name)
			}
		}
	}
}
