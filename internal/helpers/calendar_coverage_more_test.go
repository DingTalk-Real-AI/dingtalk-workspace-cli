package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func newRecurrenceTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "event"}
	cmd.Flags().String("recurrence-type", "", "")
	cmd.Flags().Int("recurrence-interval", 0, "")
	cmd.Flags().String("recurrence-days-of-week", "", "")
	cmd.Flags().Int("recurrence-day-of-month", 0, "")
	cmd.Flags().String("recurrence-index", "", "")
	cmd.Flags().String("recurrence-first-day-of-week", "", "")
	cmd.Flags().String("recurrence-range-type", "", "")
	cmd.Flags().String("recurrence-end-date", "", "")
	cmd.Flags().Int("recurrence-count", 0, "")
	return cmd
}

func TestCrossPlatformCoverageCalendarMeetingRoomHintCoverage(t *testing.T) {
	for _, message := range []string{"plain", "所选组织不支持预定钉钉会议室", `{"errorCode": "400056"}`, `{"errorCode":"400056"}`} {
		_ = isMeetingRoomDisabledError(message)
	}
	ordinary := errors.New("ordinary")
	if withMeetingRoomDisabledHint(nil) != nil || withMeetingRoomDisabledHint(ordinary) != ordinary {
		t.Fatal("ordinary error wrapping changed")
	}
	cli := &CLIError{Message: "plain", Suggestion: "old"}
	_ = withMeetingRoomDisabledHint(cli)
	cli = &CLIError{Message: `{"errorCode":"400056"}`, Suggestion: "old"}
	_ = withMeetingRoomDisabledHint(cli)
}

func installCalendarCaller(t *testing.T, caller *helpersCoreCaller) {
	t.Helper()
	installHelpersCoreDeps(t, caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	oldArgs := os.Args
	os.Args = []string{"dws", "calendar"}
	t.Cleanup(func() { os.Args = oldArgs })
}

func TestCrossPlatformCoverageCalendarSearchResponseCoverage(t *testing.T) {
	caller := &helpersCoreCaller{format: "json", result: textToolResult(`{"result":{"rooms":[]}}`)}
	installCalendarCaller(t, caller)
	cmd := &cobra.Command{Use: "search"}
	cmd.SetContext(context.Background())
	args := map[string]any{"startTime": time.Now().UnixMilli(), "endTime": time.Now().Add(time.Hour).UnixMilli()}

	for _, raw := range []string{"", `{`, `{"result":{"rooms":[]}}`, `{"rooms":[]}`} {
		caller.err = nil
		caller.result = textToolResult(raw)
		for _, format := range []string{"json", "raw", "table", "unknown"} {
			caller.format = format
			_ = callSearchRoomsByName(cmd, args)
			_ = callMeetingRoomSearchResult(cmd, args, false, false, false)
		}
	}
	caller.err = errors.New(`{"errorCode":"400056"}`)
	_ = callSearchRoomsByName(cmd, args)
	_ = callMeetingRoomSearchResult(cmd, args, true, true, true)
}

func TestCrossPlatformCoverageCalendarRangeAttachmentCoverage(t *testing.T) {
	now := time.Now().UnixMilli()
	for _, tc := range []struct{ start, end, corrected bool }{
		{false, false, false}, {false, true, false}, {true, false, false}, {true, true, false}, {true, true, true},
	} {
		hint := map[string]any{}
		attachRoomSearchRange(hint, map[string]any{"startTime": now, "endTime": now + 1000}, tc.start, tc.end, tc.corrected)
	}
	attachRoomSearchRange(map[string]any{}, map[string]any{}, false, false, false)
	attachRoomSearchRange(map[string]any{}, map[string]any{"startTime": now}, false, false, false)

	newRangeCommand := func() *cobra.Command {
		cmd := &cobra.Command{Use: "events"}
		for _, name := range []string{"start", "time-min", "min-time", "start-time", "startTime", "start_time", "start-date", "startDate", "end", "time-max", "max-time", "end-time", "endTime", "end_time", "end-date", "endDate"} {
			cmd.Flags().String(name, "", "")
		}
		return cmd
	}
	for _, tc := range []struct{ start, end bool }{{}, {start: true}, {end: true}, {start: true, end: true}} {
		cmd := newRangeCommand()
		if tc.start {
			_ = cmd.Flags().Set("start", "value")
		}
		if tc.end {
			_ = cmd.Flags().Set("endDate", "value")
		}
		for _, parsed := range []map[string]any{{}, {"result": map[string]any{}}} {
			attachCalendarSearchRange(parsed, cmd, map[string]any{"startTime": now, "endTime": now + 1000})
		}
	}
	attachCalendarSearchRange(map[string]any{}, nil, map[string]any{})
	attachCalendarSearchRange(map[string]any{}, nil, map[string]any{"startTime": now})
}

func TestCrossPlatformCoverageCalendarBusyAndEventResponseCoverage(t *testing.T) {
	caller := &helpersCoreCaller{format: "json"}
	installCalendarCaller(t, caller)
	cmd := &cobra.Command{Use: "events"}
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	cmd.SetContext(context.Background())
	args := map[string]any{"startTime": time.Now().UnixMilli(), "endTime": time.Now().Add(time.Hour).UnixMilli()}
	responses := []string{
		"", `{`, `{}`,
		`{"result":["bad",{"scheduleItems":"bad"},{"scheduleItems":["bad",{}, {"status":null},{"status":""},{"status":"busy"}]}]}`,
		`{"result":{"events":["bad",{}, {"id":null},{"id":""},{"id":"later","start":{"dateTime":"2026-01-02T04:00:00Z"}},{"id":"earlier","start":{"dateTime":"2026-01-02T03:00:00Z"}}],"nextCursor":"next"}}`,
	}
	for _, raw := range responses {
		caller.result = textToolResult(raw)
		caller.err = nil
		for _, format := range []string{"json", "raw", "table", "unknown"} {
			caller.format = format
			_ = callFilteredBusyStatus(cmd, args)
			_ = callSortedCalendarEvents(cmd, "list_events", args)
		}
	}
	caller.err = errors.New("failed")
	_ = callFilteredBusyStatus(cmd, args)
	_ = callSortedCalendarEvents(cmd, "list_events", args)
	caller.err = nil
	caller.dry = true
	_ = callSortedCalendarEvents(cmd, "list_events", args)

	for _, event := range []any{
		"bad", map[string]any{}, map[string]any{"start": "bad"},
		map[string]any{"start": map[string]any{"dateTime": "bad"}},
		map[string]any{"created": float64(2)}, map[string]any{"updated": float64(3)},
	} {
		_ = calendarEventSortKey(event)
	}
}

func TestCrossPlatformCoverageBuildRecurrenceCoverage(t *testing.T) {
	_ = buildReminders("5,bad,0,10")
	if recurrence, err := buildRecurrence(newRecurrenceTestCommand()); err != nil || recurrence != nil {
		t.Fatalf("empty recurrence = %#v, %v", recurrence, err)
	}
	cases := []map[string]string{
		{"recurrence-type": "daily"},
		{"recurrence-type": "invalid", "recurrence-interval": "1", "recurrence-range-type": "noEnd"},
		{"recurrence-type": "daily", "recurrence-interval": "1", "recurrence-range-type": "invalid"},
		{"recurrence-type": "daily", "recurrence-interval": "1", "recurrence-range-type": "noEnd", "recurrence-first-day-of-week": "monday"},
		{"recurrence-type": "weekly", "recurrence-interval": "1", "recurrence-range-type": "noEnd"},
		{"recurrence-type": "weekly", "recurrence-interval": "1", "recurrence-range-type": "noEnd", "recurrence-days-of-week": "monday"},
		{"recurrence-type": "absoluteMonthly", "recurrence-interval": "1", "recurrence-range-type": "noEnd"},
		{"recurrence-type": "absoluteYearly", "recurrence-interval": "1", "recurrence-range-type": "noEnd", "recurrence-day-of-month": "15"},
		{"recurrence-type": "relativeMonthly", "recurrence-interval": "1", "recurrence-range-type": "noEnd", "recurrence-days-of-week": "monday"},
		{"recurrence-type": "relativeMonthly", "recurrence-interval": "1", "recurrence-range-type": "noEnd", "recurrence-days-of-week": "monday", "recurrence-index": "first"},
		{"recurrence-type": "daily", "recurrence-interval": "1", "recurrence-range-type": "endDate"},
		{"recurrence-type": "daily", "recurrence-interval": "1", "recurrence-range-type": "endDate", "recurrence-end-date": "bad"},
		{"recurrence-type": "daily", "recurrence-interval": "1", "recurrence-range-type": "endDate", "recurrence-end-date": "2026-12-31T23:59:59+08:00"},
		{"recurrence-type": "daily", "recurrence-interval": "1", "recurrence-range-type": "numbered"},
		{"recurrence-type": "daily", "recurrence-interval": "1", "recurrence-range-type": "numbered", "recurrence-count": "10"},
	}
	for _, values := range cases {
		cmd := newRecurrenceTestCommand()
		for name, value := range values {
			_ = cmd.Flags().Set(name, value)
		}
		_, _ = buildRecurrence(cmd)
	}
}

func TestCrossPlatformCoverageCalendarUnknownFallbackCoverage(t *testing.T) {
	root := &cobra.Command{Use: "calendar"}
	group := &cobra.Command{Use: "room", RunE: func(*cobra.Command, []string) error { return errors.New("previous") }}
	known := &cobra.Command{Use: "search", Aliases: []string{"find"}}
	hidden := &cobra.Command{Use: "secret", Hidden: true}
	group.AddCommand(known, hidden)
	root.AddCommand(group)
	installUnknownVerbFallback(group)
	_ = group.RunE(group, []string{"unknown"})
	_ = group.RunE(group, []string{"--ignored", "search"})
	_ = group.RunE(group, nil)
	group.HelpFunc()(group, []string{"calendar", "room", "unknown"})
	group.HelpFunc()(known, nil)
	printUnknownSubcmdError(group, "searhc")
	printUnknownSubcmdError(group, "unrelated")

	hint := calendarInfoHintSubCmd("query", "use search")
	group.AddCommand(hint)
	_ = hint.RunE(hint, nil)

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	group.Flags().StringP("known", "k", "", "")
	for _, args := range [][]string{
		{"dws", "calendar", "room", "--"},
		{"dws", "calendar", "room", "--help"},
		{"dws", "calendar", "room", "--known=value"},
		{"dws", "calendar", "room", "--unknown=value"},
		{"dws", "calendar", "room", "-h"},
		{"dws", "calendar", "room", "-k", "value"},
		{"dws", "calendar", "room", "-x"},
	} {
		os.Args = args
		_ = findUnknownFlag(group)
	}
	nilPrev := &cobra.Command{Use: "empty"}
	root.AddCommand(nilPrev)
	installUnknownVerbFallback(nilPrev)
	os.Args = []string{"dws", "calendar", "empty"}
	_ = nilPrev.RunE(nilPrev, nil)
}
