// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	smartCoverageStart = "2026-08-17T09:00:00+08:00"
	smartCoverageEnd   = "2026-08-17T10:00:00+08:00"
	smartCoverageEvent = `{"success":true,"result":{"id":"event-placeholder","summary":"fixture title","start":{"dateTime":"2026-08-17T09:00:00+08:00"},"end":{"dateTime":"2026-08-17T10:00:00+08:00"}}}`
	smartTodoPageEmpty = `{"success":true,"result":{"todoCards":[],"hasMore":false}}`
)

func smartContact() calendarSmartTestStep {
	return calendarSmartTestStep{text: `{"result":[{"userId":"user-placeholder","name":"fixture person"}]}`}
}

func TestCrossPlatformCoverageCalendarSmartBookBranches(t *testing.T) {
	if _, _, err := runCalendarSmartCLI(t, &calendarSmartTestCaller{}, "calendar", "+book", "--title", "x", "--start", smartCoverageEnd, "--end", smartCoverageStart, "--yes"); err == nil {
		t.Fatal("book accepted reversed range")
	}
	if _, _, err := runCalendarSmartCLI(t, &calendarSmartTestCaller{}, "calendar", "+book", "--title", "x", "--start", smartCoverageStart, "--end", smartCoverageEnd, "--dry-run", "--yes"); err != nil {
		t.Fatal(err)
	}
	blankWith := &calendarSmartTestCaller{}
	if _, _, err := runCalendarSmartCLI(t, blankWith, "calendar", "+book", "--title", "x", "--start", smartCoverageStart, "--end", smartCoverageEnd, "--with", " , ", "--yes"); err == nil {
		t.Fatal("blank invitee list accepted")
	}

	for name, caller := range map[string]*calendarSmartTestCaller{
		"create-call": {steps: map[string][]calendarSmartTestStep{"calendar/create_calendar_event": {{err: errors.New("create")}}}},
		"missing-id":  {steps: map[string][]calendarSmartTestStep{"calendar/create_calendar_event": {{text: `{"success":true,"result":{}}`}}}},
		"read-call":   {steps: map[string][]calendarSmartTestStep{"calendar/create_calendar_event": {{text: `{"success":true,"result":{"id":"event-placeholder"}}`}}, "calendar/get_calendar_detail": {{err: errors.New("read")}}}},
		"read-shape":  {steps: map[string][]calendarSmartTestStep{"calendar/create_calendar_event": {{text: `{"success":true,"result":{"id":"event-placeholder"}}`}}, "calendar/get_calendar_detail": {{text: `{"success":true,"result":{}}`}}}},
		"read-verify": {steps: map[string][]calendarSmartTestStep{"calendar/create_calendar_event": {{text: `{"success":true,"result":{"id":"event-placeholder"}}`}}, "calendar/get_calendar_detail": {{text: strings.Replace(smartCoverageEvent, "fixture title", "wrong", 1)}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := runCalendarSmartCLI(t, caller, "calendar", "+book", "--title", "fixture title", "--start", smartCoverageStart, "--end", smartCoverageEnd, "--yes"); err == nil {
				t.Fatal("bad book path accepted")
			}
		})
	}

	rollbackFailure := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{
		"contact/search_contact_by_key_word": {{text: smartContact().text}},
		"calendar/create_calendar_event":     {{text: `{"success":true,"result":{"id":"event-placeholder"}}`}},
		"calendar/add_calendar_participant":  {{text: `{"success":false}`}},
		"calendar/delete_calendar_event":     {{err: errors.New("rollback")}},
	}}
	if _, _, err := runCalendarSmartCLI(t, rollbackFailure, "calendar", "+book", "--title", "fixture title", "--start", smartCoverageStart, "--end", smartCoverageEnd, "--with", "fixture person", "--yes"); err == nil {
		t.Fatal("failed rollback accepted")
	}

	participantCases := map[string]*calendarSmartTestCaller{
		"call": {steps: map[string][]calendarSmartTestStep{
			"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/create_calendar_event": {{text: `{"success":true,"result":{"id":"event-placeholder"}}`}}, "calendar/add_calendar_participant": {{text: `{"success":true}`}}, "calendar/get_calendar_detail": {{text: smartCoverageEvent}}, "calendar/get_calendar_participants": {{err: errors.New("participants")}},
		}},
		"shape": {steps: map[string][]calendarSmartTestStep{
			"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/create_calendar_event": {{text: `{"success":true,"result":{"id":"event-placeholder"}}`}}, "calendar/add_calendar_participant": {{text: `{"success":true}`}}, "calendar/get_calendar_detail": {{text: smartCoverageEvent}}, "calendar/get_calendar_participants": {{text: `{"success":true}`}},
		}},
		"profile": {steps: map[string][]calendarSmartTestStep{
			"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/create_calendar_event": {{text: `{"success":true,"result":{"id":"event-placeholder"}}`}}, "calendar/add_calendar_participant": {{text: `{"success":true}`}}, "calendar/get_calendar_detail": {{text: smartCoverageEvent}}, "calendar/get_calendar_participants": {{text: `{"success":true,"result":[{"displayName":"fixture person","self":true}]}`}}, "contact/get_current_user_profile": {{err: errors.New("profile")}},
		}},
		"missing": {steps: map[string][]calendarSmartTestStep{
			"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/create_calendar_event": {{text: `{"success":true,"result":{"id":"event-placeholder"}}`}}, "calendar/add_calendar_participant": {{text: `{"success":true}`}}, "calendar/get_calendar_detail": {{text: smartCoverageEvent}}, "calendar/get_calendar_participants": {{text: `{"success":true,"result":[{"userId":"other"}]}`}},
		}},
	}
	for name, caller := range participantCases {
		t.Run("participants-"+name, func(t *testing.T) {
			if _, _, err := runCalendarSmartCLI(t, caller, "calendar", "+book", "--title", "fixture title", "--start", smartCoverageStart, "--end", smartCoverageEnd, "--with", "fixture person", "--yes"); err == nil {
				t.Fatal("bad participant verification accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageCalendarSmartInviteAndRescheduleBranches(t *testing.T) {
	preflight := `{"success":true,"result":{"id":"event-placeholder"}}`
	inviteDry := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/get_calendar_detail": {{text: preflight}}}}
	if _, _, err := runCalendarSmartCLI(t, inviteDry, "calendar", "+invite", "--event", "event-placeholder", "--with", "fixture person", "--dry-run", "--yes"); err != nil {
		t.Fatal(err)
	}
	for name, caller := range map[string]*calendarSmartTestCaller{
		"preflight-call":     {steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/get_calendar_detail": {{err: errors.New("preflight")}}}},
		"write-call":         {steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/get_calendar_detail": {{text: preflight}}, "calendar/add_calendar_participant": {{err: errors.New("write")}}}},
		"receipt":            {steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/get_calendar_detail": {{text: preflight}}, "calendar/add_calendar_participant": {{text: `{"result":{}}`}}}},
		"participants-call":  {steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/get_calendar_detail": {{text: preflight}}, "calendar/add_calendar_participant": {{text: `{"success":true}`}}, "calendar/get_calendar_participants": {{err: errors.New("participants")}}}},
		"participants-shape": {steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/get_calendar_detail": {{text: preflight}}, "calendar/add_calendar_participant": {{text: `{"success":true}`}}, "calendar/get_calendar_participants": {{text: `{"success":true}`}}}},
		"profile":            {steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/get_calendar_detail": {{text: preflight}}, "calendar/add_calendar_participant": {{text: `{"success":true}`}}, "calendar/get_calendar_participants": {{text: `{"success":true,"result":[{"displayName":"fixture person","self":true}]}`}}, "contact/get_current_user_profile": {{err: errors.New("profile")}}}},
		"missing":            {steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}, "calendar/get_calendar_detail": {{text: preflight}}, "calendar/add_calendar_participant": {{text: `{"success":true}`}}, "calendar/get_calendar_participants": {{text: `{"success":true,"result":[{"userId":"other"}]}`}}}},
	} {
		t.Run("invite-"+name, func(t *testing.T) {
			if _, _, err := runCalendarSmartCLI(t, caller, "calendar", "+invite", "--event", "event-placeholder", "--with", "fixture person", "--yes"); err == nil {
				t.Fatal("bad invite path accepted")
			}
		})
	}

	rescheduleDry := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"calendar/get_calendar_detail": {{text: preflight}}}}
	if _, _, err := runCalendarSmartCLI(t, rescheduleDry, "calendar", "+reschedule", "--event", "event-placeholder", "--start", smartCoverageStart, "--end", smartCoverageEnd, "--dry-run", "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCalendarSmartCLI(t, &calendarSmartTestCaller{}, "calendar", "+reschedule", "--event", "event-placeholder", "--start", smartCoverageEnd, "--end", smartCoverageStart, "--yes"); err == nil {
		t.Fatal("reschedule accepted reversed range")
	}
	for name, caller := range map[string]*calendarSmartTestCaller{
		"preflight": {steps: map[string][]calendarSmartTestStep{"calendar/get_calendar_detail": {{err: errors.New("preflight")}}}},
		"write":     {steps: map[string][]calendarSmartTestStep{"calendar/get_calendar_detail": {{text: preflight}}, "calendar/update_calendar_event": {{err: errors.New("write")}}}},
		"receipt":   {steps: map[string][]calendarSmartTestStep{"calendar/get_calendar_detail": {{text: preflight}}, "calendar/update_calendar_event": {{text: `{"result":{}}`}}}},
		"read":      {steps: map[string][]calendarSmartTestStep{"calendar/get_calendar_detail": {{text: preflight}, {err: errors.New("read")}}, "calendar/update_calendar_event": {{text: `{"success":true}`}}}},
		"shape":     {steps: map[string][]calendarSmartTestStep{"calendar/get_calendar_detail": {{text: preflight}, {text: `{"success":true,"result":{}}`}}, "calendar/update_calendar_event": {{text: `{"success":true}`}}}},
		"times":     {steps: map[string][]calendarSmartTestStep{"calendar/get_calendar_detail": {{text: preflight}, {text: `{"success":true,"result":{"id":"event-placeholder","start":{"dateTime":"2026-08-17T08:00:00+08:00"},"end":{"dateTime":"2026-08-17T10:00:00+08:00"}}}`}}, "calendar/update_calendar_event": {{text: `{"success":true}`}}}},
	} {
		t.Run("reschedule-"+name, func(t *testing.T) {
			if _, _, err := runCalendarSmartCLI(t, caller, "calendar", "+reschedule", "--event", "event-placeholder", "--start", smartCoverageStart, "--end", smartCoverageEnd, "--yes"); err == nil {
				t.Fatal("bad reschedule path accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageCalendarSmartReadCommands(t *testing.T) {
	if _, ok := conflictsEndTime(map[string]any{"endTime": smartCoverageEnd}); !ok {
		t.Fatal("top-level endTime was not parsed")
	}
	conflictEvents := `{"success":true,"result":{"events":[{"id":"a","summary":"","start":{"dateTime":"2026-08-17T09:00:00+08:00"},"end":{"dateTime":"2026-08-17T10:00:00+08:00"}},{"id":"b","summary":"B","start":{"dateTime":"2026-08-17T09:30:00+08:00"},"end":{"dateTime":"2026-08-17T10:30:00+08:00"}}],"hasMore":false}}`
	for _, command := range []string{"+conflicts", "+free-slots", "+tomorrow", "+week"} {
		caller := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"calendar/list_calendar_events": {{text: conflictEvents}}}}
		args := []string{"calendar", command}
		if command == "+free-slots" {
			args = append(args, "--from", "0", "--to", "24")
		}
		if _, _, err := runCalendarSmartCLI(t, caller, args...); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
	for name, response := range map[string]string{
		"empty":  smartTodoPageEmpty,
		"future": `{"success":true,"result":{"events":[{"id":"future","summary":"future","start":{"dateTime":"2099-08-17T09:00:00+08:00"},"end":{"dateTime":"2099-08-17T10:00:00+08:00"}}],"hasMore":false}}`,
	} {
		t.Run("next-"+name, func(t *testing.T) {
			if name == "empty" {
				response = `{"success":true,"result":{"events":[],"hasMore":false}}`
			}
			caller := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"calendar/list_calendar_events": {{text: response}}}}
			if _, _, err := runCalendarSmartCLI(t, caller, "calendar", "+next-event"); err != nil {
				t.Fatal(err)
			}
		})
	}
	if got := shortcutNextEventPick([]map[string]any{{"id": ""}, {"id": "x", "start": "bad"}}, time.Now()); got != nil {
		t.Fatalf("invalid next event picked: %#v", got)
	}
	if _, _, err := runCalendarSmartCLI(t, &calendarSmartTestCaller{}, "calendar", "+free-slots", "--from", "24", "--to", "1"); err == nil {
		t.Fatal("bad free-slot window accepted")
	}
	if _, _, err := runCalendarSmartCLI(t, &calendarSmartTestCaller{}, "calendar", "+suggest-time", "--with", "fixture person", "--start", smartCoverageEnd, "--end", smartCoverageStart); err == nil {
		t.Fatal("suggest-time accepted reversed range")
	}
	suggest := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{
		"contact/search_contact_by_key_word":  {{text: smartContact().text}},
		"calendar/list_suggested_event_times": {{text: `{"success":true,"result":{"recommendEventTimes":[]}}`}},
	}}
	if _, _, err := runCalendarSmartCLI(t, suggest, "calendar", "+suggest-time", "--with", "fixture person", "--start", smartCoverageStart, "--end", smartCoverageEnd, "--duration", "30"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCalendarSmartCLI(t, &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"calendar/get_calendar_detail": {{text: `{"success":true,"result":{"id":"event-placeholder"}}`}}}}, "calendar", "+cancel-event", "--event", "event-placeholder", "--dry-run", "--yes"); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageTodoSmartWriteBranches(t *testing.T) {
	for name, args := range map[string][]string{
		"assign-empty-task": {"todo", "+assign", "--task", "   ", "--to", "fixture person", "--yes"},
		"assign-bad-due":    {"todo", "+assign", "--task", "task", "--to", "fixture person", "--due", "tomorrow", "--yes"},
		"remind-empty-task": {"todo", "+remind", "--task", "   ", "--yes"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := runCalendarSmartCLI(t, &calendarSmartTestCaller{}, args...); err == nil {
				t.Fatal("invalid todo shortcut arguments accepted")
			}
		})
	}

	assignDry := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}}}
	if _, _, err := runCalendarSmartCLI(t, assignDry, "todo", "+assign", "--task", "task", "--to", "fixture person", "--due", smartCoverageEnd, "--dry-run", "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCalendarSmartCLI(t, &calendarSmartTestCaller{}, "todo", "+assign", "--task", "task", "--to", "fixture person", "--yes"); err == nil {
		t.Fatal("assign contact resolution failure accepted")
	}
	for name, responses := range map[string]map[string][]calendarSmartTestStep{
		"call":    {"contact/search_contact_by_key_word": {{text: smartContact().text}}, "todo/create_personal_todo": {{err: errors.New("write")}}},
		"verify":  {"contact/search_contact_by_key_word": {{text: smartContact().text}}, "todo/create_personal_todo": {{text: `{"result":{}}`}}},
		"success": {"contact/search_contact_by_key_word": {{text: smartContact().text}}, "todo/create_personal_todo": {{text: `{"success":true,"result":{"taskId":"task-placeholder"}}`}}, "todo/get_todo_detail": {{text: `{"success":true,"result":{"todoDetailModel":{"taskId":"task-placeholder","subject":"task"}}}`}}},
	} {
		t.Run("assign-"+name, func(t *testing.T) {
			_, _, err := runCalendarSmartCLI(t, &calendarSmartTestCaller{steps: responses}, "todo", "+assign", "--task", "task", "--to", "fixture person", "--due", smartCoverageEnd, "--yes")
			if name == "success" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("bad assign path accepted")
			}
		})
	}

	multiDry := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}}}
	if _, _, err := runCalendarSmartCLI(t, multiDry, "todo", "+assign-multi", "--task", "task", "--to", "fixture person", "--dry-run", "--yes"); err != nil {
		t.Fatal(err)
	}
	multiSuccess := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}, "todo/create_personal_todo": {{text: `{"success":true,"result":{"taskId":"task-placeholder"}}`}}, "todo/get_todo_detail": {{text: `{"success":true,"result":{"todoDetailModel":{"taskId":"task-placeholder","subject":"task"}}}`}}}}
	if _, _, err := runCalendarSmartCLI(t, multiSuccess, "todo", "+assign-multi", "--task", "task", "--to", "fixture person", "--yes"); err != nil {
		t.Fatal(err)
	}
	multiFailure := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"contact/search_contact_by_key_word": {{text: smartContact().text}}, "todo/create_personal_todo": {{err: errors.New("write")}}}}
	if _, _, err := runCalendarSmartCLI(t, multiFailure, "todo", "+assign-multi", "--task", "task", "--to", "fixture person", "--yes"); err == nil {
		t.Fatal("assign-multi write failure accepted")
	}

	doneDry := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"todo/get_user_todos_in_current_org": {{text: `{"success":true,"result":{"todoCards":[{"taskId":"task-placeholder","subject":"needle"}],"hasMore":false}}`}}}}
	if _, _, err := runCalendarSmartCLI(t, doneDry, "todo", "+todo-done", "--task", "needle", "--dry-run", "--yes"); err != nil {
		t.Fatal(err)
	}
	doneSuccess := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"todo/get_user_todos_in_current_org": {{text: `{"success":true,"result":{"todoCards":[{"taskId":"task-placeholder","subject":"needle"}],"hasMore":false}}`}}, "todo/update_todo_done_status": {{text: `{"success":true}`}}, "todo/get_todo_detail": {{text: `{"success":true,"result":{"todoDetailModel":{"taskId":"task-placeholder","isDone":true}}}`}}}}
	if _, _, err := runCalendarSmartCLI(t, doneSuccess, "todo", "+todo-done", "--task", "needle", "--yes"); err != nil {
		t.Fatal(err)
	}
	doneCall := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"todo/get_user_todos_in_current_org": {{text: `{"success":true,"result":{"todoCards":[{"taskId":"task-placeholder","subject":"needle"}],"hasMore":false}}`}}, "todo/update_todo_done_status": {{err: errors.New("write")}}}}
	if _, _, err := runCalendarSmartCLI(t, doneCall, "todo", "+todo-done", "--task", "needle", "--yes"); err == nil {
		t.Fatal("todo-done write failure accepted")
	}
	doneVerify := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"todo/get_user_todos_in_current_org": {{text: `{"success":true,"result":{"todoCards":[{"taskId":"task-placeholder","subject":"needle"}],"hasMore":false}}`}}, "todo/update_todo_done_status": {{text: `{"result":{}}`}}}}
	if _, _, err := runCalendarSmartCLI(t, doneVerify, "todo", "+todo-done", "--task", "needle", "--yes"); err == nil {
		t.Fatal("todo-done unverifiable write accepted")
	}

	remindDry := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"contact/get_current_user_profile": {{text: `{"result":{"userId":"user-placeholder"}}`}}}}
	if _, _, err := runCalendarSmartCLI(t, remindDry, "todo", "+remind", "--task", "task", "--at", smartCoverageEnd, "--dry-run", "--yes"); err != nil {
		t.Fatal(err)
	}
	remindProfileFailure := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"contact/get_current_user_profile": {{err: errors.New("profile")}}}}
	if _, _, err := runCalendarSmartCLI(t, remindProfileFailure, "todo", "+remind", "--task", "task", "--yes"); err == nil {
		t.Fatal("remind profile failure accepted")
	}
	remindSuccess := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"contact/get_current_user_profile": {{text: `{"result":{"userId":"user-placeholder"}}`}}, "todo/create_personal_todo": {{text: `{"success":true,"result":{"taskId":"task-placeholder"}}`}}, "todo/get_todo_detail": {{text: `{"success":true,"result":{"todoDetailModel":{"taskId":"task-placeholder","subject":"task"}}}`}}}}
	if _, _, err := runCalendarSmartCLI(t, remindSuccess, "todo", "+remind", "--task", "task", "--yes"); err != nil {
		t.Fatal(err)
	}
	for name, step := range map[string]calendarSmartTestStep{
		"call":    {err: errors.New("write")},
		"receipt": {text: `{"result":{}}`},
	} {
		t.Run("remind-"+name, func(t *testing.T) {
			caller := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{
				"contact/get_current_user_profile": {{text: `{"result":{"userId":"user-placeholder"}}`}},
				"todo/create_personal_todo":        {step},
			}}
			if _, _, err := runCalendarSmartCLI(t, caller, "todo", "+remind", "--task", "task", "--yes"); err == nil {
				t.Fatal("bad remind write accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageTodoSmartReadAndSharedBranches(t *testing.T) {
	for _, response := range []map[string]any{{}, {"success": false}, {"result": "bad"}, {"result": map[string]any{"todoCards": map[string]any{}}}} {
		if _, _, err := shortcutTodoCardsStrict(response); err == nil {
			t.Fatalf("bad todo cards accepted: %#v", response)
		}
	}
	deep := map[string]any{"result": map[string]any{"result": map[string]any{"result": map[string]any{"result": map[string]any{"result": map[string]any{"result": map[string]any{}}}}}}}
	if _, _, err := shortcutTodoCardsStrict(deep); err == nil {
		t.Fatal("over-deep todo wrapper accepted")
	}
	steps := make([]calendarSmartTestStep, todoMaxPages)
	for index := range steps {
		steps[index] = calendarSmartTestStep{text: `{"success":true,"result":{"todoCards":[],"hasMore":true}}`}
	}
	capCaller := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"todo/get_user_todos_in_current_org": steps}}
	if _, _, err := runCalendarSmartCLI(t, capCaller, "todo", "+created-todos"); err == nil {
		t.Fatal("todo page cap accepted incomplete data")
	}

	dayStart, dayEnd := calendarDayRange(0)
	dueMillis := dayStart.Add(time.Hour).UnixMilli()
	due := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"todo/get_user_todos_in_current_org": {{text: `{"success":true,"result":{"todoCards":[{"taskId":"task-placeholder","subject":"inside","dueTime":` + fmtInt64(dueMillis) + `},{"taskId":"task-out","subject":"outside","dueTime":` + fmtInt64(dayEnd.UnixMilli()) + `}],"hasMore":false}}`}}}}
	if _, _, err := runCalendarSmartCLI(t, due, "todo", "+due-today"); err != nil {
		t.Fatal(err)
	}
	badDue := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{"todo/get_user_todos_in_current_org": {{text: `{"success":true,"result":{"todoCards":[{"taskId":"task-placeholder","subject":"bad"}],"hasMore":false}}`}}}}
	if _, _, err := runCalendarSmartCLI(t, badDue, "todo", "+due-today"); err == nil {
		t.Fatal("due-today accepted missing due time")
	}
}

func fmtInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}
