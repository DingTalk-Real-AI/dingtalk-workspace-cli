package helpers

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageCalendarEventOptions(t *testing.T) {
	for _, action := range []string{"create", "update"} {
		for _, tc := range []struct {
			name  string
			flags []string
			want  map[string]any
		}{
			{"omitted", nil, nil},
			{"all_day", []string{"--is-all-day"}, map[string]any{"isAllDay": true}},
			{"not_all_day", []string{"--is-all-day=false"}, map[string]any{"isAllDay": false}},
			{"add_meeting", []string{"--add-online-meeting"}, map[string]any{"onlineMeeting": map[string]any{"add": true}}},
			{"explicit_add_meeting", []string{"--add-online-meeting=true"}, map[string]any{"onlineMeeting": map[string]any{"add": true}}},
			{"no_meeting", []string{"--add-online-meeting=false"}, map[string]any{"onlineMeeting": map[string]any{"add": false}}},
			{"combined", []string{"--is-all-day", "--add-online-meeting=false"}, map[string]any{"isAllDay": true, "onlineMeeting": map[string]any{"add": false}}},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				caller := &scriptedToolCaller{}
				installScriptedCaller(t, caller)
				args := []string{"event", action, "--title", "test"}
				want := map[string]any{"summary": "test"}
				if action == "update" {
					args = append(args, "--id", "event-1")
					want["eventId"] = "event-1"
				}
				if _, explicitAllDay := tc.want["isAllDay"]; action == "create" || explicitAllDay {
					start, end := "2030-01-01T09:00:00+08:00", "2030-01-01T10:00:00+08:00"
					if tc.want["isAllDay"] == true {
						start, end = "2030-01-01", "2030-01-02"
					}
					args = append(args, "--start", start, "--end", end)
					want["startDateTime"], want["endDateTime"] = start, end
				}
				for k, v := range tc.want {
					want[k] = v
				}
				args = append(args, tc.flags...)
				if err := executeCalendarOptionsTest(t, newCalendarCommand(), args...); err != nil {
					t.Fatal(err)
				}
				if caller.calls != 1 || caller.server != "calendar" || caller.tool != action+"_calendar_event" || !reflect.DeepEqual(caller.args, want) {
					t.Fatalf("calls=%d tool=%s/%s args=%#v, want %#v", caller.calls, caller.server, caller.tool, caller.args, want)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageCalendarAllDayDates(t *testing.T) {
	for _, action := range []string{"create", "update"} {
		for _, tc := range []struct {
			name, start, end string
			valid            bool
		}{
			{"leap", "2032-02-29", "2032-03-01", true},
			{"invalid_day", "2030-02-29", "2030-03-01", false},
			{"invalid_month", "2030-13-01", "2031-01-01", false},
			{"short_month", "2030-1-01", "2030-01-02", false},
			{"short_day", "2030-01-1", "2030-01-02", false},
			{"timestamp", "2030-01-01T09:00:00+08:00", "2030-01-02", false},
			{"same_day", "2030-01-01", "2030-01-01", false},
			{"reversed", "2030-01-02", "2030-01-01", false},
			{"bad_end", "2030-01-01", "2030-02-30", false},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				caller := &scriptedToolCaller{}
				installScriptedCaller(t, caller)
				args := []string{"event", action, "--is-all-day", "--start-date", tc.start, "--end-time", tc.end}
				if action == "create" {
					args = append(args, "--title", "test")
				} else {
					args = append(args, "--id", "event-1")
				}
				err := executeCalendarOptionsTest(t, newCalendarCommand(), args...)
				if tc.valid {
					if err != nil || caller.calls != 1 || caller.args["startDateTime"] != tc.start || caller.args["endDateTime"] != tc.end {
						t.Fatalf("err=%v calls=%d args=%#v", err, caller.calls, caller.args)
					}
				} else if err == nil || (!strings.Contains(err.Error(), "yyyy-MM-dd") && !strings.Contains(err.Error(), "exclusive end date")) || caller.calls != 0 {
					t.Fatalf("err=%v calls=%d", err, caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageCalendarEventOptionsPartialUpdate(t *testing.T) {
	for _, allDay := range []string{"true", "false"} {
		for _, timeFlags := range [][]string{nil, {"--start-date", "2030-01-01"}, {"--end-time", "2030-01-02"}} {
			t.Run(allDay+strings.Join(timeFlags, " "), func(t *testing.T) {
				caller := &scriptedToolCaller{}
				installScriptedCaller(t, caller)
				if allDay == "false" && len(timeFlags) > 0 {
					timeFlags = []string{timeFlags[0], "2030-01-01T09:00:00+08:00"}
				}
				args := append([]string{"event", "update", "--id", "event-1", "--is-all-day=" + allDay}, timeFlags...)
				err := executeCalendarOptionsTest(t, newCalendarCommand(), args...)
				if err == nil || !strings.Contains(err.Error(), "is required") || caller.calls != 0 {
					t.Fatalf("err=%v calls=%d", err, caller.calls)
				}
			})
		}
	}
	for _, flag := range []string{"--start", "--end"} {
		t.Run("time_only_"+flag, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			installScriptedCaller(t, caller)
			if err := executeCalendarOptionsTest(t, newCalendarCommand(), "event", "update", "--id", "event-1", flag, "2030-01-01T09:00:00+08:00"); err != nil {
				t.Fatal(err)
			}
			if caller.calls != 1 || len(caller.args) != 2 {
				t.Fatalf("calls=%d args=%v", caller.calls, caller.args)
			}
		})
	}
	for _, flag := range []string{"--is-all-day=invalid", "--add-online-meeting=invalid"} {
		t.Run(flag, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			installScriptedCaller(t, caller)
			if err := executeCalendarOptionsTest(t, newCalendarCommand(), "event", "update", "--id", "event-1", flag); err == nil || caller.calls != 0 {
				t.Fatalf("err=%v calls=%d", err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageCalendarEventOptionsTimedDates(t *testing.T) {
	for _, tc := range []struct {
		start, end string
		valid      bool
	}{
		{"2030-01-01T09:00:00+08:00", "2030-01-01T10:00:00+08:00", true},
		{"2030-01-01T01:00:00Z", "2030-01-01T10:00:00+08:00", true},
		{"2030-01-01", "2030-01-01T10:00:00+08:00", false},
		{"2030-01-01T09:00:00+08:00", "2030-01-02", false},
		{"2030-01-01T09:00:00", "2030-01-01T10:00:00+08:00", false},
		{"2030-01-01T09:00:00Z", "2030-01-01T10:00:00+08:00", false},
	} {
		t.Run(tc.start+"/"+tc.end, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			installScriptedCaller(t, caller)
			err := executeCalendarOptionsTest(t, newCalendarCommand(), "event", "update", "--id", "event-1", "--is-all-day=false", "--start-date", tc.start, "--end-time", tc.end)
			if tc.valid {
				if err != nil || caller.calls != 1 || caller.args["startDateTime"] != tc.start || caller.args["endDateTime"] != tc.end {
					t.Fatalf("err=%v calls=%d args=%v", err, caller.calls, caller.args)
				}
			} else if err == nil || caller.calls != 0 || (!strings.Contains(err.Error(), "--start") && !strings.Contains(err.Error(), "--end")) {
				t.Fatalf("err=%v calls=%d", err, caller.calls)
			}
		})
	}
}

func executeCalendarOptionsTest(t *testing.T, root *cobra.Command, args ...string) error {
	t.Helper()
	testseam.Swap(t, &os.Args, append([]string{"dws", "calendar"}, args...))
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}
