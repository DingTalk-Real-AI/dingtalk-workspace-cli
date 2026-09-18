// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"errors"
	"reflect"
	"testing"
)

func TestCrossPlatformCoverageCalendarSmartInvitationsUseWriteReceipt(t *testing.T) {
	for _, command := range []string{"+invite", "+book"} {
		t.Run(command, func(t *testing.T) {
			caller := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{
				"contact/search_contact_by_key_word": {
					{text: `{"result":[{"userId":"user-1","name":"First Legal Name"}]}`},
					{text: `{"result":[{"userId":"user-2","name":"Second Legal Name"}]}`},
				},
				"calendar/create_calendar_event":    {{text: `{"success":true,"result":{"id":"event-placeholder"}}`}},
				"calendar/get_calendar_detail":      {{text: smartCoverageEvent}},
				"calendar/add_calendar_participant": {{text: `{"success":true}`}},
				// Participants expose display names, not the user IDs used to add them.
				"calendar/get_calendar_participants": {{text: `{"success":true,"result":[{"displayName":"First Nickname","responseStatus":"needsAction"},{"displayName":"Second Nickname","responseStatus":"accepted"}]}`}},
			}}
			args := []string{"calendar", command, "--with", "First Nickname,Second Nickname", "--yes"}
			if command == "+invite" {
				args = append(args, "--event", "event-placeholder")
			} else {
				args = append(args, "--title", "fixture title", "--start", smartCoverageStart, "--end", smartCoverageEnd)
			}
			payload, _, err := runCalendarSmartCLI(t, caller, args...)
			if err != nil || payload["success"] != true || payload["eventId"] != "event-placeholder" || payload["verified"] != false {
				t.Fatalf("successful invite receipt rejected or readback claimed: payload=%#v err=%v", payload, err)
			}
			if command == "+invite" {
				if payload["acknowledged"] != true || payload["invitedCount"] != float64(2) {
					t.Fatalf("invite acknowledgement=%#v", payload)
				}
			} else if payload["eventVerified"] != true || payload["attendeesAcknowledged"] != true {
				t.Fatalf("book verification=%#v", payload)
			}
			if caller.counts["calendar/add_calendar_participant"] != 1 || caller.counts["calendar/get_calendar_participants"] != 0 || caller.counts["contact/get_current_user_profile"] != 0 || caller.counts["calendar/delete_calendar_event"] != 0 {
				t.Fatalf("unexpected invitation calls: %#v", caller.counts)
			}
			for _, call := range caller.calls {
				if call.key == "calendar/add_calendar_participant" && (call.args["eventId"] != "event-placeholder" || !reflect.DeepEqual(call.args["attendeesToAdd"], []string{"user-1", "user-2"})) {
					t.Fatalf("invitation targets changed: %#v", call.args)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageCalendarSmartInvitationsRejectUnsuccessfulReceipts(t *testing.T) {
	for _, command := range []string{"+invite", "+book"} {
		for name, receipt := range map[string]calendarSmartTestStep{
			"rejected":  {text: `{"success":false,"message":"fixture rejection"}`},
			"missing":   {text: `{"result":{}}`},
			"malformed": {text: `{"success":"true"}`},
			"empty":     {text: `{}`},
			"transport": {err: errors.New("fixture transport error")},
		} {
			t.Run(command+"/"+name, func(t *testing.T) {
				caller := &calendarSmartTestCaller{steps: map[string][]calendarSmartTestStep{
					"contact/search_contact_by_key_word": {{text: smartContact().text}},
					"calendar/create_calendar_event":     {{text: `{"success":true,"result":{"id":"event-placeholder"}}`}},
					"calendar/add_calendar_participant":  {receipt},
				}}
				args := []string{"calendar", command, "--with", "fixture person", "--yes"}
				if command == "+invite" {
					caller.steps["calendar/get_calendar_detail"] = []calendarSmartTestStep{{text: smartCoverageEvent}}
					args = append(args, "--event", "event-placeholder")
				} else {
					caller.steps["calendar/delete_calendar_event"] = []calendarSmartTestStep{{text: `{"success":true}`}}
					caller.steps["calendar/get_calendar_detail"] = []calendarSmartTestStep{{err: errors.New("event not found")}}
					args = append(args, "--title", "fixture title", "--start", smartCoverageStart, "--end", smartCoverageEnd)
				}
				payload, outputText, err := runCalendarSmartCLI(t, caller, args...)
				if err == nil || payload != nil || outputText != "" || caller.counts["calendar/add_calendar_participant"] != 1 {
					t.Fatalf("unsuccessful receipt accepted: payload=%#v err=%v calls=%#v", payload, err, caller.counts)
				}
				wantDeletes := 0
				if command == "+book" {
					wantDeletes = 1
				}
				if caller.counts["calendar/delete_calendar_event"] != wantDeletes || caller.counts["calendar/get_calendar_participants"] != 0 {
					t.Fatalf("failure handling changed: %#v", caller.counts)
				}
			})
		}
	}
}
