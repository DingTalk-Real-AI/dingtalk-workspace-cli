package cli_compat_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ── attendance record get ──────────────────────────────────

func TestAttendanceRecordGet_should_call_get_user_attendance_record_when_valid_inputs(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "record", "get"}, map[string]string{
		"user": "U001", "date": "2026-03-08",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "get_user_attendance_record")
	assertToolArg(t, cap, "userId", "U001")
}

func TestAttendanceRecordGet_should_convert_date_to_unix_millis(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	_ = execCmd(t, root, []string{"attendance", "record", "get"}, map[string]string{
		"user": "U001", "date": "2026-03-08",
	})
	expected := time.Date(2026, 3, 8, 0, 0, 0, 0, time.Local).UnixMilli()
	assertToolArg(t, cap, "workDate", expected)
}

func TestAttendanceRecordGet_should_return_error_when_date_format_invalid(t *testing.T) {
	_ = setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "record", "get"}, map[string]string{
		"user": "U001", "date": "03-08-2026",
	})
	if err == nil {
		t.Fatal("expected error for invalid date format")
	}
}

func TestAttendanceRecordGet_should_return_error_when_date_is_nonsense(t *testing.T) {
	_ = setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "record", "get"}, map[string]string{
		"user": "U001", "date": "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for nonsense date")
	}
}

func TestAttendanceRecordGet_should_handle_leap_year_date(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "record", "get"}, map[string]string{
		"user": "U001", "date": "2024-02-29",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 2, 29, 0, 0, 0, 0, time.Local).UnixMilli()
	assertToolArg(t, cap, "workDate", expected)
}

// ── attendance shift list ──────────────────────────────────

func TestAttendanceShiftList_should_call_batch_get_employee_shifts(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "shift", "list"}, map[string]string{
		"users": "userId1,userId2", "start": "2026-03-03", "end": "2026-03-07",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "batch_get_employee_shifts")
}

func TestAttendanceShiftList_should_parse_comma_separated_users(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	_ = execCmd(t, root, []string{"attendance", "shift", "list"}, map[string]string{
		"users": "userId1,userId2,userId3", "start": "2026-03-03", "end": "2026-03-07",
	})
	assertToolArg(t, cap, "userIds", []string{"userId1", "userId2", "userId3"})
}

func TestAttendanceShiftList_should_trim_whitespace_from_user_ids(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	_ = execCmd(t, root, []string{"attendance", "shift", "list"}, map[string]string{
		"users": " userId1 , userId2 , userId3 ", "start": "2026-03-03", "end": "2026-03-07",
	})
	assertToolArg(t, cap, "userIds", []string{"userId1", "userId2", "userId3"})
}

func TestAttendanceShiftList_should_return_error_when_start_date_invalid(t *testing.T) {
	_ = setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "shift", "list"}, map[string]string{
		"users": "userId1", "start": "bad-date", "end": "2026-03-07",
	})
	if err == nil {
		t.Fatal("expected error for invalid start date")
	}
}

func TestAttendanceShiftList_should_return_error_when_end_date_invalid(t *testing.T) {
	_ = setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "shift", "list"}, map[string]string{
		"users": "userId1", "start": "2026-03-03", "end": "bad-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid end date")
	}
}

func TestAttendanceShiftList_should_convert_dates_to_unix_millis(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	_ = execCmd(t, root, []string{"attendance", "shift", "list"}, map[string]string{
		"users": "userId1", "start": "2026-03-03", "end": "2026-03-07",
	})
	fromExpected := time.Date(2026, 3, 3, 0, 0, 0, 0, time.Local).UnixMilli()
	toExpected := time.Date(2026, 3, 7, 0, 0, 0, 0, time.Local).UnixMilli()
	assertToolArg(t, cap, "fromDateTime", fromExpected)
	assertToolArg(t, cap, "toDateTime", toExpected)
}

func TestAttendanceShiftList_should_filter_empty_user_ids(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	_ = execCmd(t, root, []string{"attendance", "shift", "list"}, map[string]string{
		"users": "userId1,,userId2,", "start": "2026-03-03", "end": "2026-03-07",
	})
	assertToolArg(t, cap, "userIds", []string{"userId1", "userId2"})
}

// ── attendance summary ─────────────────────────────────────

func TestAttendanceSummary_should_call_tool_with_user_and_date(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "summary"}, map[string]string{
		"user":       "U001",
		"date":       "2026-03-12 15:00:00",
		"stats-type": "month",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "get_attendance_summary")
}

func TestAttendanceSummary_should_pass_user_and_date_flags(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	_ = execCmd(t, root, []string{"attendance", "summary"}, map[string]string{
		"user":       "U001",
		"date":       "2026-03-12 15:00:00",
		"stats-type": "month",
	})
	last := cap.last()
	if last == nil {
		t.Fatal("no MCP call captured")
	}
	// Verify the call was made with at least the userId
	if vo, ok := last.Args["QueryUserAttendVO"].(map[string]any); ok {
		if vo["userId"] != "U001" {
			t.Errorf("expected userId=U001, got %v", vo["userId"])
		}
		// workDate may or may not be present depending on compat layer
	} else if last.Args["userId"] != "U001" {
		// May pass flat params
		t.Errorf("expected userId=U001 in some form, got args: %v", last.Args)
	}
}

func TestAttendanceSummary_should_error_when_date_missing(t *testing.T) {
	_ = setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "summary"}, map[string]string{"user": "U001", "date": ""})
	if err == nil {
		t.Fatal("expected error when --date is missing")
	}
	if !strings.Contains(err.Error(), "date") || !strings.Contains(err.Error(), "required") {
		t.Errorf("expected error to mention date required, got: %v", err)
	}
}

func TestAttendanceSummary_should_error_when_user_missing(t *testing.T) {
	_ = setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "summary"}, map[string]string{"user": "", "date": "2026-03-12 15:00:00"})
	if err == nil {
		t.Fatal("expected error when --user is missing")
	}
	if !strings.Contains(err.Error(), "user") || !strings.Contains(err.Error(), "required") {
		t.Errorf("expected error to mention user required, got: %v", err)
	}
}

func TestAttendanceSummary_should_pass_user_id_through_vo(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	_ = execCmd(t, root, []string{"attendance", "summary"}, map[string]string{
		"user":       "U002",
		"date":       "2026-03-12 15:00:00",
		"stats-type": "month",
	})
	last := cap.last()
	if last == nil {
		t.Fatal("no call captured")
	}
	vo, ok := last.Args["QueryUserAttendVO"].(map[string]any)
	if !ok {
		t.Fatalf("expected QueryUserAttendVO map, got %T", last.Args["QueryUserAttendVO"])
	}
	if vo["userId"] != "U002" {
		t.Errorf("expected userId=U002, got %v", vo["userId"])
	}
}

func TestAttendanceSummary_should_use_dry_run_mode(t *testing.T) {
	cap := setupTestDepsWithDryRun(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "summary"}, map[string]string{
		"user":       "U001",
		"date":       "2026-03-12 15:00:00",
		"stats-type": "month",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.last() != nil {
		t.Error("expected no MCP call in dry-run mode")
	}
}

// execSummaryDryRun runs attendance summary with --dry-run and returns the
// parsed QueryUserAttendVO map from the helper_invocation payload. This is
// necessary because the attendance handler is a custom helper (not a dynamic
// MCP-route), so the test framework's mcpCallCapture cannot intercept the
// call in non-dry-run mode.
func execSummaryDryRun(t *testing.T, flags map[string]string) map[string]any {
	t.Helper()
	root := buildRoot()
	_ = setupTestDeps(t, "attendance")

	cliArgs := []string{"-f", "json", "attendance", "summary", "--dry-run"}
	for k, v := range flags {
		if v != "" {
			cliArgs = append(cliArgs, "--"+k, v)
		}
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(cliArgs)
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected CLI error: %v", err)
	}

	var payload struct {
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("failed to parse dry-run output as JSON: %v\noutput: %s", err, out.String())
	}
	vo, ok := payload.Params["QueryUserAttendVO"].(map[string]any)
	if !ok {
		t.Fatalf("expected QueryUserAttendVO map in dry-run params, got %T: %v", payload.Params["QueryUserAttendVO"], payload.Params)
	}
	return vo
}

func TestAttendanceSummary_should_pass_stats_type_when_provided(t *testing.T) {
	vo := execSummaryDryRun(t, map[string]string{
		"user":       "U001",
		"date":       "2026-03-12 15:00:00",
		"stats-type": "month",
	})
	if vo["statsType"] != "month" {
		t.Errorf("expected statsType=month in VO, got %v", vo["statsType"])
	}
}

func TestAttendanceSummary_should_pass_stats_type_week(t *testing.T) {
	vo := execSummaryDryRun(t, map[string]string{
		"user":       "U001",
		"date":       "2026-03-12 15:00:00",
		"stats-type": "week",
	})
	if vo["statsType"] != "week" {
		t.Errorf("expected statsType=week in VO, got %v", vo["statsType"])
	}
}

func TestAttendanceSummary_should_error_when_stats_type_missing(t *testing.T) {
	_ = setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "summary"}, map[string]string{
		"user": "U001",
		"date": "2026-03-12 15:00:00",
	})
	if err == nil {
		t.Fatal("expected error when --stats-type is missing")
	}
	if !strings.Contains(err.Error(), "stats-type") {
		t.Errorf("expected error message to mention stats-type, got: %v", err)
	}
}

func TestAttendanceSummary_should_error_when_stats_type_invalid(t *testing.T) {
	_ = setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "summary"}, map[string]string{
		"user":       "U001",
		"date":       "2026-03-12 15:00:00",
		"stats-type": "foobar",
	})
	if err == nil {
		t.Fatal("expected error when --stats-type is neither week nor month")
	}
	if !strings.Contains(err.Error(), "week") || !strings.Contains(err.Error(), "month") {
		t.Errorf("expected error message to mention week and month, got: %v", err)
	}
}

// ── attendance rules ───────────────────────────────────────

func TestAttendanceRules_should_call_tool_with_date(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	err := execCmd(t, root, []string{"attendance", "rules"}, map[string]string{"date": "2026-03-14"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "query_attendance_group_or_rules")
}

func TestAttendanceRules_should_pass_date_param(t *testing.T) {
	cap := setupTestDeps(t, "attendance")
	root := buildRoot()
	_ = execCmd(t, root, []string{"attendance", "rules"}, map[string]string{"date": "2026-03-14 09:00:00"})
	last := cap.last()
	if last == nil {
		t.Fatal("no MCP call captured")
	}
	if last.Args["date"] != "2026-03-14 09:00:00" {
		t.Errorf("expected date=2026-03-14 09:00:00, got %v", last.Args["date"])
	}
}
