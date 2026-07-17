package helpers

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageCalendarOptionalFlagsRemainingCoverage(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	start := "2030-01-01T09:00:00+08:00"
	end := "2030-01-01T10:00:00+08:00"
	if err := executeFilterCoverage(t, newCalendarCommand(),
		"event", "create", "--title", "meeting", "--start", start, "--end", end,
		"--rich-text-desc", "rich", "--location", "room", "--free-busy", "busy",
		"--calendar-id", "calendar", "--remind-minutes", "5,10",
	); err != nil {
		t.Fatalf("event create options: %v", err)
	}
	if err := executeFilterCoverage(t, newCalendarCommand(), "event", "respond", "--id", "event", "--status", "accepted", "--calendar-id", "calendar"); err != nil {
		t.Fatalf("event response calendar: %v", err)
	}
	if err := executeFilterCoverage(t, newCalendarCommand(), "attendee", "list", "--event", "event", "--calendar-id", "calendar"); err != nil {
		t.Fatalf("attendee list calendar: %v", err)
	}
	if err := executeFilterCoverage(t, newCalendarCommand(), "attendee", "add", "--event", "event"); err == nil {
		t.Fatal("attendee add without users returned nil")
	}
	if err := executeFilterCoverage(t, newCalendarCommand(), "attendee", "add", "--event", "event", "--attendees", "u1", "--calendar-id", "calendar"); err != nil {
		t.Fatalf("attendee add calendar: %v", err)
	}
	if err := executeFilterCoverage(t, newCalendarCommand(), "attendee", "delete", "--event", "event"); err == nil {
		t.Fatal("attendee delete without users returned nil")
	}
	if err := executeFilterCoverage(t, newCalendarCommand(), "attendee", "delete", "--event", "event", "--attendees", "u1", "--calendar-id", "calendar"); err != nil {
		t.Fatalf("attendee delete calendar: %v", err)
	}
	if err := executeFilterCoverage(t, newCalendarCommand(), "busy", "search", "--users", "u1", "--start", end, "--end", start); err == nil {
		t.Fatal("reversed busy range returned nil")
	}
	if err := executeFilterCoverage(t, newCalendarCommand(), "attachment", "add", "--event", "event", "--files", "file:report.pdf", "--calendar-id", "calendar"); err != nil {
		t.Fatalf("attachment calendar: %v", err)
	}
}

func TestCrossPlatformCoverageCalendarUnknownFlagAndSuggestionRemainingCoverage(t *testing.T) {
	root := &cobra.Command{Use: "calendar"}
	group := &cobra.Command{Use: "room"}
	group.SuggestionsMinimumDistance = 3
	group.AddCommand(&cobra.Command{Use: "search", SuggestFor: []string{"serach"}, Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group)
	installUnknownVerbFallback(group)
	oldArgs := os.Args
	os.Args = []string{"dws", "calendar", "room", "--unknown"}
	t.Cleanup(func() { os.Args = oldArgs })
	if err := group.RunE(group, nil); err != nil {
		t.Fatalf("unknown flag fallback: %v", err)
	}
	printUnknownSubcmdError(group, "serach")
}
