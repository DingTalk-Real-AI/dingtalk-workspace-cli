package cli_compat_test

import (
	"testing"
)

// ── event list ──────────────────────────────────────────────

func TestCalEventList_should_call_list_calendar_events_with_no_args(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "event", "list"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "list_calendar_events")
	last := cap.last()
	if last == nil {
		t.Fatal("no MCP call captured")
	}
	if last.Args["startTime"] == nil {
		t.Error("expected startTime to be set (default today 00:00), but it was nil")
	}
	if last.Args["endTime"] == nil {
		t.Error("expected endTime to be set (default today 23:59), but it was nil")
	}
}

func TestCalEventList_should_default_end_when_only_start_provided(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "event", "list"}, map[string]string{
		"start": "2026-03-10",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "list_calendar_events")
	last := cap.last()
	if last == nil {
		t.Fatal("no MCP call captured")
	}
	if last.Args["startTime"] == nil {
		t.Error("expected startTime to be set, but it was nil")
	}
	if last.Args["endTime"] == nil {
		t.Error("expected endTime to be defaulted when only --start provided, but it was nil")
	}
}

func TestCalEventList_should_default_start_when_only_end_provided(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "event", "list"}, map[string]string{
		"end": "2026-03-10T18:00:00+08:00",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "list_calendar_events")
	last := cap.last()
	if last == nil {
		t.Fatal("no MCP call captured")
	}
	if last.Args["startTime"] == nil {
		t.Error("expected startTime to be defaulted when only --end provided, but it was nil")
	}
	if last.Args["endTime"] == nil {
		t.Error("expected endTime to be set, but it was nil")
	}
}

func TestCalEventList_should_parse_start_time_to_millis(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "event", "list"}, map[string]string{
		"start": "2026-03-10T14:00:00+08:00",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolArg(t, cap, "startTime", float64(1773122400000))
}

func TestCalEventList_should_parse_end_time_to_millis(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "event", "list"}, map[string]string{
		"end": "2026-03-10T18:00:00+08:00",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolArg(t, cap, "endTime", float64(1773136800000))
}

func TestCalEventList_should_return_error_for_invalid_start_format(t *testing.T) {
	_ = setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "event", "list"}, map[string]string{
		"start": "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid date format")
	}
}

func TestCalEventList_should_pass_both_times(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "list"}, map[string]string{
		"start": "2026-03-10T14:00:00+08:00",
		"end":   "2026-03-10T18:00:00+08:00",
	})
	last := cap.last()
	if last == nil {
		t.Fatal("no MCP call captured")
	}
	if last.Args["startTime"] == nil || last.Args["endTime"] == nil {
		t.Error("expected both startTime and endTime")
	}
}

// ── event get ───────────────────────────────────────────────

func TestCalEventGet_should_call_get_calendar_detail(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "event", "get"}, map[string]string{
		"id": "EVT001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "get_calendar_detail")
	assertToolArg(t, cap, "eventId", "EVT001")
}

func TestCalEventGet_should_pass_different_event_ids(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "get"}, map[string]string{"id": "EVT_XYZ"})
	assertToolArg(t, cap, "eventId", "EVT_XYZ")
}

func TestCalEventGet_should_make_single_call(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "get"}, map[string]string{"id": "E1"})
	assertCallCount(t, cap, 1)
}

func TestCalEventGet_should_handle_long_event_id(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	longID := "evt_0123456789abcdef0123456789abcdef0123456789abcdef"
	_ = execCmd(t, root, []string{"calendar", "event", "get"}, map[string]string{"id": longID})
	assertToolArg(t, cap, "eventId", longID)
}

func TestCalEventGet_should_return_no_error(t *testing.T) {
	_ = setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "event", "get"}, map[string]string{"id": "E1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── event create ────────────────────────────────────────────

func TestCalEventCreate_should_call_create_calendar_event(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "event", "create"}, map[string]string{
		"title": "Q1 复盘会",
		"start": "2026-03-10T14:00:00+08:00",
		"end":   "2026-03-10T15:00:00+08:00",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "create_calendar_event")
	assertToolArg(t, cap, "summary", "Q1 复盘会")
}

func TestCalEventCreate_should_pass_start_and_end_as_strings(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "create"}, map[string]string{
		"title": "会议",
		"start": "2026-03-10T14:00:00+08:00",
		"end":   "2026-03-10T15:00:00+08:00",
	})
	assertToolArg(t, cap, "startDateTime", "2026-03-10T14:00:00+08:00")
	assertToolArg(t, cap, "endDateTime", "2026-03-10T15:00:00+08:00")
}

func TestCalEventCreate_should_include_description_when_provided(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "create"}, map[string]string{
		"title": "会议",
		"start": "2026-03-10T14:00:00+08:00",
		"end":   "2026-03-10T15:00:00+08:00",
		"desc":  "讨论Q1业务",
	})
	assertToolArg(t, cap, "description", "讨论Q1业务")
}

func TestCalEventCreate_should_not_include_description_when_empty(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "create"}, map[string]string{
		"title": "会议",
		"start": "2026-03-10T14:00:00+08:00",
		"end":   "2026-03-10T15:00:00+08:00",
		"desc":  "",
	})
	assertArgNotPresent(t, cap, "description")
}

func TestCalEventCreate_should_make_single_call(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "create"}, map[string]string{
		"title": "T", "start": "2026-03-10T14:00:00+08:00", "end": "2026-03-10T15:00:00+08:00",
	})
	assertCallCount(t, cap, 1)
}

// ── event update ────────────────────────────────────────────

func TestCalEventUpdate_should_call_update_calendar_event(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "event", "update"}, map[string]string{
		"id":    "EVT001",
		"title": "新标题",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "update_calendar_event")
	assertToolArg(t, cap, "eventId", "EVT001")
	assertToolArg(t, cap, "summary", "新标题")
}

func TestCalEventUpdate_should_only_pass_changed_fields(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "update"}, map[string]string{
		"id": "EVT001", "title": "", "start": "", "end": "",
	})
	assertArgNotPresent(t, cap, "summary")
	assertArgNotPresent(t, cap, "startDateTime")
	assertArgNotPresent(t, cap, "endDateTime")
}

func TestCalEventUpdate_should_pass_start_and_end_when_provided(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "update"}, map[string]string{
		"id":    "E1",
		"start": "2026-04-01T09:00:00+08:00",
		"end":   "2026-04-01T10:00:00+08:00",
	})
	assertToolArg(t, cap, "startDateTime", "2026-04-01T09:00:00+08:00")
	assertToolArg(t, cap, "endDateTime", "2026-04-01T10:00:00+08:00")
}

func TestCalEventUpdate_should_always_pass_event_id(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "update"}, map[string]string{"id": "EID"})
	assertToolArg(t, cap, "eventId", "EID")
}

func TestCalEventUpdate_should_make_single_call(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "update"}, map[string]string{"id": "E1"})
	assertCallCount(t, cap, 1)
}

// ── event delete ────────────────────────────────────────────

func TestCalEventDelete_should_not_call_mcp_when_not_confirmed(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "delete"}, map[string]string{"id": "E1"})
	assertCallCount(t, cap, 0)
}

func TestCalEventDelete_should_call_delete_calendar_event_when_confirmed(t *testing.T) {
	cap := setupTestDepsAutoConfirm(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "delete"}, map[string]string{"id": "E1"})
	assertToolName(t, cap, "delete_calendar_event")
	assertToolArg(t, cap, "eventId", "E1")
}

func TestCalEventDelete_should_return_nil_when_not_confirmed(t *testing.T) {
	_ = setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "event", "delete"}, map[string]string{"id": "E1"})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCalEventDelete_should_pass_event_id_when_confirmed(t *testing.T) {
	cap := setupTestDepsAutoConfirm(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "delete"}, map[string]string{"id": "EVT_ABC"})
	assertToolArg(t, cap, "eventId", "EVT_ABC")
}

func TestCalEventDelete_should_make_single_call_when_confirmed(t *testing.T) {
	cap := setupTestDepsAutoConfirm(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "event", "delete"}, map[string]string{"id": "E1"})
	assertCallCount(t, cap, 1)
}

// ── participant list ────────────────────────────────────────

func TestCalParticipantList_should_call_get_calendar_participants(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "participant", "list"}, map[string]string{
		"event": "EVT001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "get_calendar_participants")
	assertToolArg(t, cap, "eventId", "EVT001")
}

func TestCalParticipantList_should_handle_different_event_ids(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "participant", "list"}, map[string]string{
		"event": "EVT_XYZ",
	})
	assertToolArg(t, cap, "eventId", "EVT_XYZ")
}

func TestCalParticipantList_should_make_single_call(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "participant", "list"}, map[string]string{
		"event": "E1",
	})
	assertCallCount(t, cap, 1)
}

func TestCalParticipantList_should_not_pass_attendees(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "participant", "list"}, map[string]string{
		"event": "E1",
	})
	assertArgNotPresent(t, cap, "attendeesToAdd")
}

func TestCalParticipantList_should_return_no_error(t *testing.T) {
	_ = setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "participant", "list"}, map[string]string{
		"event": "E1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── participant add ─────────────────────────────────────────

func TestCalParticipantAdd_should_call_add_calendar_participant(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "participant", "add"}, map[string]string{
		"event": "EVT001", "users": "userId1,userId2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "add_calendar_participant")
	assertToolArg(t, cap, "attendeesToAdd", []string{"userId1", "userId2"})
}

func TestCalParticipantAdd_should_parse_single_user(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "participant", "add"}, map[string]string{
		"event": "E1", "users": "uid_only",
	})
	assertToolArg(t, cap, "attendeesToAdd", []string{"uid_only"})
}

func TestCalParticipantAdd_should_pass_event_id(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "participant", "add"}, map[string]string{
		"event": "EVT_99", "users": "u1",
	})
	assertToolArg(t, cap, "eventId", "EVT_99")
}

func TestCalParticipantAdd_should_trim_spaces_in_users(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "participant", "add"}, map[string]string{
		"event": "E1", "users": " u1 , u2 ",
	})
	assertToolArg(t, cap, "attendeesToAdd", []string{"u1", "u2"})
}

func TestCalParticipantAdd_should_make_single_call(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "participant", "add"}, map[string]string{
		"event": "E1", "users": "u1",
	})
	assertCallCount(t, cap, 1)
}

// ── room search ─────────────────────────────────────────────

func TestCalRoomSearch_should_call_query_available_meeting_room(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "room", "search"}, map[string]string{
		"start": "2026-03-10T14:00:00+08:00",
		"end":   "2026-03-10T15:00:00+08:00",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "query_available_meeting_room")
}

func TestCalRoomSearch_should_parse_times_to_millis(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "room", "search"}, map[string]string{
		"start": "2026-03-10T14:00:00+08:00",
		"end":   "2026-03-10T15:00:00+08:00",
	})
	assertToolArg(t, cap, "startTime", float64(1773122400000))
	assertToolArg(t, cap, "endTime", float64(1773126000000))
}

func TestCalRoomSearch_should_pass_groupId_when_provided(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "room", "search"}, map[string]string{
		"start":    "2026-03-10T14:00:00+08:00",
		"end":      "2026-03-10T15:00:00+08:00",
		"group-id": "group_123",
	})
	assertToolArg(t, cap, "groupId", "group_123")
}

func TestCalRoomSearch_should_return_error_for_bad_start_time(t *testing.T) {
	_ = setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "room", "search"}, map[string]string{
		"start": "bad", "end": "2026-03-10T15:00:00+08:00",
	})
	if err == nil {
		t.Fatal("expected error for bad start time")
	}
}

func TestCalRoomSearch_should_return_error_for_bad_end_time(t *testing.T) {
	_ = setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "room", "search"}, map[string]string{
		"start": "2026-03-10T14:00:00+08:00", "end": "bad",
	})
	if err == nil {
		t.Fatal("expected error for bad end time")
	}
}

func TestCalRoomSearch_should_make_single_call(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "room", "search"}, map[string]string{
		"start": "2026-03-10T14:00:00+08:00", "end": "2026-03-10T15:00:00+08:00",
	})
	assertCallCount(t, cap, 1)
}

// ── room add ────────────────────────────────────────────────

func TestCalRoomAdd_should_call_add_meeting_room(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "room", "add"}, map[string]string{
		"event": "EVT001", "rooms": "room1,room2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "add_meeting_room")
	assertToolArg(t, cap, "eventId", "EVT001")
	assertToolArg(t, cap, "roomIds", []string{"room1", "room2"})
}

func TestCalRoomAdd_should_handle_single_room(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "room", "add"}, map[string]string{
		"event": "E1", "rooms": "room_single",
	})
	assertToolArg(t, cap, "roomIds", []string{"room_single"})
}

func TestCalRoomAdd_should_trim_room_ids(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "room", "add"}, map[string]string{
		"event": "E1", "rooms": " r1 , r2 ",
	})
	assertToolArg(t, cap, "roomIds", []string{"r1", "r2"})
}

func TestCalRoomAdd_should_pass_event_id(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "room", "add"}, map[string]string{
		"event": "EVT_X", "rooms": "r1",
	})
	assertToolArg(t, cap, "eventId", "EVT_X")
}

func TestCalRoomAdd_should_make_single_call(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "room", "add"}, map[string]string{
		"event": "E1", "rooms": "r1",
	})
	assertCallCount(t, cap, 1)
}

// ── room list-groups ────────────────────────────────────────

func TestCalRoomListGroups_should_call_list_meeting_room_groups(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "room", "list-groups"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "list_meeting_room_groups")
}

func TestCalRoomListGroups_should_pass_nil_args(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "room", "list-groups"}, nil)
	assertNilArgs(t, cap)
}

func TestCalRoomListGroups_should_make_single_call(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "room", "list-groups"}, nil)
	assertCallCount(t, cap, 1)
}

func TestCalRoomListGroups_should_return_no_error(t *testing.T) {
	_ = setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "room", "list-groups"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCalRoomListGroups_should_not_call_mcp_in_dry_run(t *testing.T) {
	cap := setupTestDepsWithDryRun(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "room", "list-groups"}, nil)
	assertCallCount(t, cap, 0)
}

// ── busy search ─────────────────────────────────────────────

func TestCalBusySearch_should_call_query_busy_status(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "busy", "search"}, map[string]string{
		"users": "userId1,userId2",
		"start": "2026-03-10T14:00:00+08:00",
		"end":   "2026-03-10T18:00:00+08:00",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "query_busy_status")
	assertToolArg(t, cap, "userIds", []string{"userId1", "userId2"})
}

func TestCalBusySearch_should_parse_times_to_millis(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "busy", "search"}, map[string]string{
		"users": "u1", "start": "2026-03-10T14:00:00+08:00", "end": "2026-03-10T18:00:00+08:00",
	})
	assertToolArg(t, cap, "startTime", float64(1773122400000))
	assertToolArg(t, cap, "endTime", float64(1773136800000))
}

func TestCalBusySearch_should_return_error_for_bad_start(t *testing.T) {
	_ = setupTestDeps(t, "calendar")
	root := buildRoot()
	err := execCmd(t, root, []string{"calendar", "busy", "search"}, map[string]string{
		"users": "u1", "start": "bad", "end": "2026-03-10T18:00:00+08:00",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCalBusySearch_should_handle_single_user(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "busy", "search"}, map[string]string{
		"users": "single_uid", "start": "2026-03-10T14:00:00+08:00", "end": "2026-03-10T18:00:00+08:00",
	})
	assertToolArg(t, cap, "userIds", []string{"single_uid"})
}

func TestCalBusySearch_should_make_single_call(t *testing.T) {
	cap := setupTestDeps(t, "calendar")
	root := buildRoot()
	_ = execCmd(t, root, []string{"calendar", "busy", "search"}, map[string]string{
		"users": "u1", "start": "2026-03-10T14:00:00+08:00", "end": "2026-03-10T18:00:00+08:00",
	})
	assertCallCount(t, cap, 1)
}
