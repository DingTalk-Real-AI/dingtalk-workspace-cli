// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const (
	calendarCoverageStart = "2026-08-17T09:00:00+08:00"
	calendarCoverageEnd   = "2026-08-17T10:00:00+08:00"
	calendarCoverageEvent = `{"success":true,"result":{"id":"event-1","summary":"updated","description":"description","start":{"dateTime":"2026-08-17T09:00:00+08:00"},"end":{"dateTime":"2026-08-17T10:00:00+08:00"},"timeZone":"Asia/Shanghai","location":"room","freeBusy":"busy"}}`
)

func calendarRuntimeForTest(t *testing.T, declaration shortcut.Shortcut, values map[string]string) *shortcut.RuntimeContext {
	t.Helper()
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return shortcut.RuntimeContextForTest(cmd, declaration)
}

func TestCrossPlatformCoverageCalendarCommonBranches(t *testing.T) {
	if got := calendarExample("+unknown"); !strings.Contains(got, "+unknown") {
		t.Fatalf("default example=%q", got)
	}
	if calendarReadShortcut("+x", "x", "x", "items", nil, nil, nil).Contract.Result == nil {
		t.Fatal("collection read shortcut did not build a result")
	}
	collectionResult := calendarCollectionResult("items", "items")
	var collectionSchema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(collectionResult.DataSchema, &collectionSchema); err != nil {
		t.Fatalf("decode collection result schema: %v", err)
	}
	for _, field := range []string{"hasMore", "nextCursor"} {
		if _, exists := collectionSchema.Properties[field]; exists {
			t.Fatalf("pagination field %q leaked into business Result schema", field)
		}
	}
	if _, exists := collectionSchema.Properties["complete"]; !exists {
		t.Fatal("collection Result schema lost business completeness evidence")
	}
	if calendarWriteShortcut("+x", "x", "x", "dws calendar +x", nil, nil, nil).Safety.Confirmation != "user_required" {
		t.Fatal("write shortcut did not build safety")
	}

	badResponses := []map[string]any{
		{"success": "yes"},
		{"success": false},
		{"result": map[string]any{}},
	}
	for _, response := range badResponses {
		if _, err := requireCalendarObject(response, "calendar/test"); err == nil {
			t.Fatalf("bad object accepted: %#v", response)
		}
	}
	if got := calendarContainers(nil); got != nil {
		t.Fatalf("nil containers=%#v", got)
	}
	if !calendarOnlyEnvelopeFields(map[string]any{"success": true, "result": map[string]any{}}) || calendarOnlyEnvelopeFields(map[string]any{"id": "x"}) {
		t.Fatal("envelope field classification drift")
	}

	if _, _, err := requireCalendarCollection(map[string]any{"success": false}, "calendar/test", "items"); err == nil {
		t.Fatal("failed response accepted as collection")
	}
	if _, _, err := requireCalendarCollection(map[string]any{"success": true, "result": []any{"bad"}}, "calendar/test", "items"); err == nil {
		t.Fatal("bad wrapper array accepted")
	}
	items, _, err := requireCalendarCollection(map[string]any{"success": true, "result": []any{map[string]any{"id": "x"}}}, "calendar/test", "items")
	if err != nil || len(items) != 1 {
		t.Fatalf("wrapper array rejected: %#v %v", items, err)
	}
	if _, err := projectCalendarRows([]any{map[string]any{"ignored": "x"}}, "calendar/test", map[string][]string{"id": {"id"}}, "id"); err == nil {
		t.Fatal("unidentifiable projected row accepted")
	}
	if !calendarEmptyValue(nil) || !calendarEmptyValue(" ") || calendarEmptyValue(1) {
		t.Fatal("calendarEmptyValue classification drift")
	}

	if page, err := calendarPagination(nil, "calendar/test"); err != nil || page.Known {
		t.Fatalf("nil pagination=%+v %v", page, err)
	}
	for _, container := range []map[string]any{
		{"hasMore": "yes"},
		{"hasMore": true, "nextCursor": 1},
		{"hasMore": false, "nextCursor": "cursor"},
	} {
		if _, err := calendarPagination(container, "calendar/test"); err == nil {
			t.Fatalf("bad pagination accepted: %#v", container)
		}
	}
	if _, err := calendarPagination(map[string]any{"nextToken": "cursor"}, "calendar/test"); err == nil {
		t.Fatal("cursor without hasMore accepted")
	}
	page, err := calendarPagination(map[string]any{"hasMore": true, "nextToken": "cursor"}, "calendar/test")
	if err != nil || !page.Known || !page.HasMore || page.NextCursor != "cursor" {
		t.Fatalf("cursor-only pagination=%+v %v", page, err)
	}
	out := map[string]any{}
	addCalendarPagination(out, page)
	if out["nextCursor"] != "cursor" || out["hasMore"] != true {
		t.Fatalf("pagination output=%#v", out)
	}

	if _, _, err := validateCalendarRange("start", "bad", "end", calendarCoverageEnd); err == nil {
		t.Fatal("bad start accepted")
	}
	if _, _, err := validateCalendarRange("start", calendarCoverageStart, "end", "bad"); err == nil {
		t.Fatal("bad end accepted")
	}
	if _, _, err := validateCalendarRange("start", calendarCoverageEnd, "end", calendarCoverageStart); err == nil {
		t.Fatal("reversed range accepted")
	}
	if got := calendarStringList([]string{" a ", "", "a", "b"}); len(got) != 2 {
		t.Fatalf("deduplicated list=%#v", got)
	}

	want := time.Date(2026, 8, 17, 1, 0, 0, 0, time.UTC)
	millis := want.UnixMilli()
	for _, actual := range []any{
		map[string]any{"value": calendarCoverageStart},
		calendarCoverageStart,
		strconv.FormatInt(millis, 10),
		float64(millis), int64(millis), int(millis),
	} {
		if !calendarTimeEquivalent(actual, calendarCoverageStart) {
			t.Fatalf("equivalent time rejected: %#v", actual)
		}
	}
	for _, actual := range []any{math.NaN(), math.Inf(1), 1.5, struct{}{}} {
		if calendarTimeEquivalent(actual, calendarCoverageStart) {
			t.Fatalf("bad time accepted: %#v", actual)
		}
	}
	if calendarTimeEquivalent(calendarCoverageStart, "bad") {
		t.Fatal("bad expected time accepted")
	}

	base := map[string]any{"id": "event-1", "summary": "updated", "start": map[string]any{"dateTime": calendarCoverageStart}}
	if err := verifyCalendarEvent(base, "wrong", nil); err == nil {
		t.Fatal("wrong event id accepted")
	}
	if err := verifyCalendarEvent(base, "event-1", map[string]any{"unknown": "ignored"}); err != nil {
		t.Fatalf("unknown verification field rejected: %v", err)
	}
	if err := verifyCalendarEvent(base, "event-1", map[string]any{"description": "x"}); err == nil {
		t.Fatal("missing readback field accepted")
	}
	if err := verifyCalendarEvent(base, "event-1", map[string]any{"summary": "wrong"}); err == nil {
		t.Fatal("mismatched readback field accepted")
	}
	if err := verifyCalendarEvent(base, "event-1", map[string]any{"startDateTime": calendarCoverageEnd}); err == nil {
		t.Fatal("mismatched readback time accepted")
	}
	item := shortcut.Shortcut{Description: "description", Intent: "intent"}
	finalizeCalendarShortcut(&item, calendarObjectResult("result"), calendarCursorPagination())
	if item.Contract.Interface == nil || item.Contract.Result == nil || item.Contract.Pagination == nil {
		t.Fatalf("finalized shortcut=%#v", item)
	}
	pageShortcut := EventGet
	pageShortcut.Execute = func(rt *shortcut.RuntimeContext) error {
		return outputCalendarPage(rt, map[string]any{"count": 0}, calendarPageEvidence{})
	}
	if err := runCalendarCoverage(t, pageShortcut, &calendarCoverageCaller{responses: map[string][]string{}}, "--event", "event-1"); err == nil {
		t.Fatal("unknown pagination was emitted")
	}
	pageShortcut.Execute = func(rt *shortcut.RuntimeContext) error {
		return outputCalendarPage(rt, map[string]any{"count": 0}, calendarPageEvidence{Known: true, NextCursor: "cursor"})
	}
	if err := runCalendarCoverage(t, pageShortcut, &calendarCoverageCaller{responses: map[string][]string{}}, "--event", "event-1"); err == nil {
		t.Fatal("inconsistent pagination was emitted")
	}
	legacyPage := EventGet
	legacyPage.OutputRollout = output.RolloutLegacyOnly
	legacyPage.Execute = func(rt *shortcut.RuntimeContext) error {
		return outputCalendarPage(rt, map[string]any{"count": 0}, calendarPageEvidence{Known: true})
	}
	if err := runCalendarCoverage(t, legacyPage, &calendarCoverageCaller{responses: map[string][]string{}}, "--event", "event-1"); err != nil {
		t.Fatalf("legacy pagination output: %v", err)
	}

	unifiedPage := EventGet
	unifiedPage.OutputRollout = output.RolloutUnifiedActive
	cmd := corecmd.New(shortcut.FromShortcut(unifiedPage))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	rt := shortcut.RuntimeContextForTest(cmd, unifiedPage)
	if err := outputCalendarPage(rt, map[string]any{"count": 1, "items": []any{map[string]any{"id": "item-1"}}}, calendarPageEvidence{Known: true, HasMore: true, NextCursor: "cursor-2"}); err != nil {
		t.Fatalf("store unified pagination result: %v", err)
	}
	if code, emitted, err := output.EmitStoredResult(cmd); err != nil || !emitted || code != 0 {
		t.Fatalf("emit unified pagination result: code=%d emitted=%v err=%v", code, emitted, err)
	}
	var envelope struct {
		Data map[string]any `json:"data"`
		Meta struct {
			Pagination *struct {
				EndpointExhausted bool   `json:"endpoint_exhausted"`
				NextToken         string `json:"next_token"`
			} `json:"pagination"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode unified pagination output: %v\n%s", err, stdout.String())
	}
	for _, field := range []string{"hasMore", "nextCursor"} {
		if _, exists := envelope.Data[field]; exists {
			t.Fatalf("pagination field %q leaked into business data: %s", field, stdout.String())
		}
	}
	if envelope.Data["complete"] != false {
		t.Fatalf("business completeness=%#v, want false: %s", envelope.Data["complete"], stdout.String())
	}
	if envelope.Meta.Pagination == nil || envelope.Meta.Pagination.EndpointExhausted || envelope.Meta.Pagination.NextToken != "cursor-2" {
		t.Fatalf("pagination meta=%+v: %s", envelope.Meta.Pagination, stdout.String())
	}
}

func TestCrossPlatformCoverageCalendarEventGetSearchAndSuggestion(t *testing.T) {
	get := &calendarCoverageCaller{responses: map[string][]string{"get_calendar_detail": {`{"success":true,"result":{"id":"event-1"}}`}}}
	if err := runCalendarCoverage(t, EventGet, get, "--event", "event-1", "--calendar-id", "primary"); err != nil {
		t.Fatal(err)
	}
	getFailure := &calendarCoverageCaller{responses: map[string][]string{"get_calendar_detail": {"__ERROR__"}}}
	if err := runCalendarCoverage(t, EventGet, getFailure, "--event", "event-1"); err == nil {
		t.Fatal("get transport failure accepted")
	}

	search := &calendarCoverageCaller{responses: map[string][]string{"list_calendar_events": {`{"success":true,"result":{"events":[{"id":"event-1","summary":"Needle","description":"body","location":"room"},{"id":"event-2","summary":"other"}],"hasMore":true,"nextCursor":"next"}}`}}}
	if err := runCalendarCoverage(t, EventSearch, search, "--query", "needle", "--start", calendarCoverageStart, "--end", calendarCoverageEnd, "--calendar-id", "primary", "--cursor", "cursor", "--limit", "2"); err != nil {
		t.Fatal(err)
	}
	for name, response := range map[string]string{
		"call":           "__ERROR__",
		"collection":     `{"success":true,"result":{"hasMore":false}}`,
		"item":           `{"success":true,"result":{"events":[{"summary":"x"}],"hasMore":false}}`,
		"pagination-bad": `{"success":true,"result":{"events":[],"hasMore":"bad"}}`,
		"pagination":     `{"success":true,"result":{"events":[]}}`,
	} {
		t.Run("search-"+name, func(t *testing.T) {
			caller := &calendarCoverageCaller{responses: map[string][]string{"list_calendar_events": {response}}}
			if err := runCalendarCoverage(t, EventSearch, caller, "--query", "x"); err == nil {
				t.Fatal("bad search response accepted")
			}
		})
	}

	suggestion := &calendarCoverageCaller{responses: map[string][]string{"list_suggested_event_times": {`{"success":true,"result":{"recommendEventTimes":[{"startTime":"2026-08-17T09:00:00+08:00","endTime":"2026-08-17T10:00:00+08:00"}]}}`}}}
	if err := runCalendarCoverage(t, Suggestion, suggestion, "--users", " user-1,user-1 ", "--duration", "30", "--start", calendarCoverageStart, "--end", calendarCoverageEnd, "--timezone", "Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"duration":    {"--users", "u", "--duration", "0"},
		"range-pair":  {"--users", "u", "--start", calendarCoverageStart},
		"range-order": {"--users", "u", "--start", calendarCoverageEnd, "--end", calendarCoverageStart},
	} {
		t.Run("suggestion-"+name, func(t *testing.T) {
			if err := runCalendarCoverage(t, Suggestion, &calendarCoverageCaller{responses: map[string][]string{}}, args...); err == nil {
				t.Fatal("bad suggestion arguments accepted")
			}
		})
	}
	emptyUsers := Suggestion
	for index := range emptyUsers.Flags {
		if emptyUsers.Flags[index].Name == "users" {
			emptyUsers.Flags[index].Required = false
		}
	}
	if err := runCalendarCoverage(t, emptyUsers, &calendarCoverageCaller{responses: map[string][]string{}}); err == nil {
		t.Fatal("empty suggestion users accepted")
	}
	for name, response := range map[string]string{
		"call":        "__ERROR__",
		"collection":  `{"success":true,"result":{}}`,
		"projection":  `{"success":true,"result":{"recommendEventTimes":[{"endTime":"x"}]}}`,
		"missing-end": `{"success":true,"result":{"recommendEventTimes":[{"startTime":"x"}]}}`,
	} {
		t.Run("suggestion-response-"+name, func(t *testing.T) {
			caller := &calendarCoverageCaller{responses: map[string][]string{"list_suggested_event_times": {response}}}
			if err := runCalendarCoverage(t, Suggestion, caller, "--users", "u"); err == nil {
				t.Fatal("bad suggestion response accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageCalendarCreateAndRSVPBranches(t *testing.T) {
	attendees := make([]string, 501)
	for index := range attendees {
		attendees[index] = "u" + strconv.Itoa(index)
	}
	for name, args := range map[string][]string{
		"title":     {"--title", strings.Repeat("x", 2049), "--start", calendarCoverageStart, "--end", calendarCoverageEnd},
		"desc":      {"--title", "x", "--desc", strings.Repeat("x", 5001), "--start", calendarCoverageStart, "--end", calendarCoverageEnd},
		"range":     {"--title", "x", "--start", calendarCoverageEnd, "--end", calendarCoverageStart},
		"attendees": {"--title", "x", "--attendees", strings.Join(attendees, ","), "--start", calendarCoverageStart, "--end", calendarCoverageEnd},
	} {
		t.Run("create-args-"+name, func(t *testing.T) {
			if err := runCalendarCoverage(t, EventCreate, &calendarCoverageCaller{responses: map[string][]string{}}, args...); err == nil {
				t.Fatal("bad create args accepted")
			}
		})
	}
	directCases := []map[string]string{
		{"title": strings.Repeat("x", 2049), "start": calendarCoverageStart, "end": calendarCoverageEnd},
		{"title": "x", "desc": strings.Repeat("x", 5001), "start": calendarCoverageStart, "end": calendarCoverageEnd},
		{"title": "x", "start": calendarCoverageEnd, "end": calendarCoverageStart},
		{"title": "x", "attendees": strings.Join(attendees, ","), "start": calendarCoverageStart, "end": calendarCoverageEnd},
	}
	for index, values := range directCases {
		rt := calendarRuntimeForTest(t, EventCreate, values)
		if _, _, err := calendarCreateParams(rt); err == nil {
			t.Fatalf("direct invalid create case %d accepted", index)
		}
	}
	if err := EventCreate.Execute(calendarRuntimeForTest(t, EventCreate, directCases[0])); err == nil {
		t.Fatal("create execute accepted invalid params")
	}
	if err := runCalendarCoverage(t, EventCreate, &calendarCoverageCaller{responses: map[string][]string{}}, "--title", "x", "--start", calendarCoverageStart, "--end", calendarCoverageEnd, "--dry-run", "--yes"); err != nil {
		t.Fatalf("create dry-run: %v", err)
	}
	for name, responses := range map[string]map[string][]string{
		"write-call":    {"create_calendar_event": {"__ERROR__"}},
		"write-shape":   {"create_calendar_event": {`{"result":{"id":"event-1"}}`}},
		"missing-id":    {"create_calendar_event": {`{"success":true,"result":{}}`}},
		"read-call":     {"create_calendar_event": {`{"success":true,"result":{"id":"event-1"}}`}, "get_calendar_detail": {"__ERROR__"}},
		"read-shape":    {"create_calendar_event": {`{"success":true,"result":{"id":"event-1"}}`}, "get_calendar_detail": {`{"success":true,"result":{}}`}},
		"read-mismatch": {"create_calendar_event": {`{"success":true,"result":{"id":"event-1"}}`}, "get_calendar_detail": {`{"success":true,"result":{"id":"event-1","summary":"wrong","start":{"dateTime":"2026-08-17T09:00:00+08:00"},"end":{"dateTime":"2026-08-17T10:00:00+08:00"}}}`}},
	} {
		t.Run("create-"+name, func(t *testing.T) {
			caller := &calendarCoverageCaller{responses: responses}
			if err := runCalendarCoverage(t, EventCreate, caller, "--title", "x", "--start", calendarCoverageStart, "--end", calendarCoverageEnd, "--calendar-id", "primary", "--yes"); err == nil {
				t.Fatal("bad create path accepted")
			}
		})
	}

	preflight := `{"success":true,"result":{"id":"event-1"}}`
	if err := runCalendarCoverage(t, RSVP, &calendarCoverageCaller{responses: map[string][]string{"get_calendar_detail": {preflight}}}, "--event", "event-1", "--status", "accept", "--dry-run", "--yes"); err != nil {
		t.Fatalf("rsvp dry-run: %v", err)
	}
	for name, responses := range map[string]map[string][]string{
		"preflight-call":  {"get_calendar_detail": {"__ERROR__"}},
		"preflight-shape": {"get_calendar_detail": {`{"success":true,"result":{}}`}},
		"write-call":      {"get_calendar_detail": {preflight}, "respond": {"__ERROR__"}},
		"write-shape":     {"get_calendar_detail": {preflight}, "respond": {`{"result":{}}`}},
		"read-call":       {"get_calendar_detail": {preflight, "__ERROR__"}, "respond": {`{"success":true}`}},
		"read-shape":      {"get_calendar_detail": {preflight, `{"success":true,"result":{}}`}, "respond": {`{"success":true}`}},
		"read-mismatch":   {"get_calendar_detail": {preflight, `{"success":true,"result":{"id":"event-1","responseStatus":"declined"}}`}, "respond": {`{"success":true}`}},
		"write-mismatch":  {"get_calendar_detail": {preflight, preflight}, "respond": {`{"success":true,"result":{"status":"declined"}}`}},
	} {
		t.Run("rsvp-"+name, func(t *testing.T) {
			if err := runCalendarCoverage(t, RSVP, &calendarCoverageCaller{responses: responses}, "--event", "event-1", "--status", "accept", "--calendar-id", "primary", "--yes"); err == nil {
				t.Fatal("bad RSVP path accepted")
			}
		})
	}
	for name, write := range map[string]string{"readback": `{"success":true}`, "terminal": `{"success":true,"result":{"status":"accepted"}}`} {
		t.Run("rsvp-success-"+name, func(t *testing.T) {
			read := preflight
			if name == "readback" {
				read = `{"success":true,"result":{"id":"event-1","responseStatus":"accepted"}}`
			}
			caller := &calendarCoverageCaller{responses: map[string][]string{"get_calendar_detail": {preflight, read}, "respond": {write}}}
			if err := runCalendarCoverage(t, RSVP, caller, "--event", "event-1", "--status", "accept", "--yes"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCrossPlatformCoverageCalendarUpdateStages(t *testing.T) {
	for name, args := range map[string][]string{
		"none":    {"--event", "event-1", "--yes"},
		"pair":    {"--event", "event-1", "--start", calendarCoverageStart, "--yes"},
		"range":   {"--event", "event-1", "--start", calendarCoverageEnd, "--end", calendarCoverageStart, "--yes"},
		"overlap": {"--event", "event-1", "--add-attendees", "u", "--remove-attendees", "u", "--yes"},
	} {
		t.Run("args-"+name, func(t *testing.T) {
			if err := runCalendarCoverage(t, EventUpdate, &calendarCoverageCaller{responses: map[string][]string{}}, args...); err == nil {
				t.Fatal("bad update args accepted")
			}
		})
	}

	preflight := `{"success":true,"result":{"id":"event-1"}}`
	if err := runCalendarCoverage(t, EventUpdate, &calendarCoverageCaller{responses: map[string][]string{"get_calendar_detail": {preflight}}}, "--event", "event-1", "--title", "updated", "--add-attendees", "u-add", "--remove-attendees", "u-remove", "--dry-run", "--yes"); err != nil {
		t.Fatalf("update dry-run: %v", err)
	}
	success := map[string][]string{
		"get_calendar_detail":         {preflight, calendarCoverageEvent},
		"update_calendar_event":       {`{"success":true}`},
		"remove_calendar_participant": {`{"success":true}`},
		"add_calendar_participant":    {`{"success":true}`},
		"get_calendar_participants":   {`{"success":true,"result":{"attendees":[{"displayName":"added","userId":"u-add"}]}}`},
	}
	if err := runCalendarCoverage(t, EventUpdate, &calendarCoverageCaller{responses: success}, "--event", "event-1", "--title", "updated", "--desc", "description", "--start", calendarCoverageStart, "--end", calendarCoverageEnd, "--timezone", "Asia/Shanghai", "--location", "room", "--free-busy", "busy", "--add-attendees", "u-add", "--remove-attendees", "u-remove", "--calendar-id", "primary", "--yes"); err != nil {
		t.Fatalf("full update: %v", err)
	}

	type updateFailure struct {
		name      string
		responses map[string][]string
		args      []string
	}
	baseArgs := []string{"--event", "event-1", "--title", "updated", "--yes"}
	cases := []updateFailure{
		{"preflight-call", map[string][]string{"get_calendar_detail": {"__ERROR__"}}, baseArgs},
		{"preflight-shape", map[string][]string{"get_calendar_detail": {`{"success":true,"result":{}}`}}, baseArgs},
		{"update-call", map[string][]string{"get_calendar_detail": {preflight}, "update_calendar_event": {"__ERROR__"}}, baseArgs},
		{"update-shape", map[string][]string{"get_calendar_detail": {preflight}, "update_calendar_event": {`{"result":{}}`}}, baseArgs},
		{"remove-call", map[string][]string{"get_calendar_detail": {preflight}, "remove_calendar_participant": {"__ERROR__"}}, []string{"--event", "event-1", "--remove-attendees", "u", "--yes"}},
		{"remove-shape", map[string][]string{"get_calendar_detail": {preflight}, "remove_calendar_participant": {`{"result":{}}`}}, []string{"--event", "event-1", "--remove-attendees", "u", "--yes"}},
		{"add-call", map[string][]string{"get_calendar_detail": {preflight}, "add_calendar_participant": {"__ERROR__"}}, []string{"--event", "event-1", "--add-attendees", "u", "--yes"}},
		{"add-shape", map[string][]string{"get_calendar_detail": {preflight}, "add_calendar_participant": {`{"result":{}}`}}, []string{"--event", "event-1", "--add-attendees", "u", "--yes"}},
		{"verify-call", map[string][]string{"get_calendar_detail": {preflight, "__ERROR__"}, "update_calendar_event": {`{"success":true}`}}, baseArgs},
		{"verify-shape", map[string][]string{"get_calendar_detail": {preflight, `{"success":true,"result":{}}`}, "update_calendar_event": {`{"success":true}`}}, baseArgs},
		{"verify-field", map[string][]string{"get_calendar_detail": {preflight, `{"success":true,"result":{"id":"event-1","summary":"wrong"}}`}, "update_calendar_event": {`{"success":true}`}}, baseArgs},
		{"attendee-call", map[string][]string{"get_calendar_detail": {preflight, preflight}, "add_calendar_participant": {`{"success":true}`}, "get_calendar_participants": {"__ERROR__"}}, []string{"--event", "event-1", "--add-attendees", "u", "--yes"}},
		{"attendee-shape", map[string][]string{"get_calendar_detail": {preflight, preflight}, "add_calendar_participant": {`{"success":true}`}, "get_calendar_participants": {`{"success":true,"result":{}}`}}, []string{"--event", "event-1", "--add-attendees", "u", "--yes"}},
		{"attendee-missing", map[string][]string{"get_calendar_detail": {preflight, preflight}, "add_calendar_participant": {`{"success":true}`}, "get_calendar_participants": {`{"success":true,"result":{"attendees":[]}}`}}, []string{"--event", "event-1", "--add-attendees", "u", "--yes"}},
		{"attendee-present", map[string][]string{"get_calendar_detail": {preflight, preflight}, "remove_calendar_participant": {`{"success":true}`}, "get_calendar_participants": {`{"success":true,"result":{"attendees":[{"displayName":"still","userId":"u"}]}}`}}, []string{"--event", "event-1", "--remove-attendees", "u", "--yes"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := runCalendarCoverage(t, EventUpdate, &calendarCoverageCaller{responses: tc.responses}, tc.args...); err == nil {
				t.Fatal("bad update path accepted")
			}
		})
	}
	if err := calendarStageError("stage", []string{"done"}, nil); err == nil || !strings.Contains(err.Error(), "done") {
		t.Fatalf("stage error missing ledger: %v", err)
	}
}

func TestCrossPlatformCoverageCalendarLegacyStrictBranches(t *testing.T) {
	for name, args := range map[string][]string{
		"agenda-order": {"--start", calendarCoverageEnd, "--end", calendarCoverageStart},
		"room-limit":   {"--limit", "bad"},
		"room-page":    {"--page", "-1"},
		"room-start":   {"--start", "bad"},
		"room-end":     {"--start", calendarCoverageStart, "--end", "bad"},
		"room-order":   {"--start", calendarCoverageEnd, "--end", calendarCoverageStart},
	} {
		t.Run(name, func(t *testing.T) {
			declaration := EventList
			if strings.HasPrefix(name, "room-") {
				declaration = RoomFind
			}
			if err := runCalendarCoverage(t, declaration, &calendarCoverageCaller{responses: map[string][]string{}}, args...); err == nil {
				t.Fatal("bad input accepted")
			}
		})
	}
	roomExecuteValidation := RoomFind
	roomExecuteValidation.Validate = nil
	if err := runCalendarCoverage(t, roomExecuteValidation, &calendarCoverageCaller{responses: map[string][]string{}}, "--limit", "bad"); err == nil {
		t.Fatal("room execute validation accepted bad limit")
	}
	if err := runCalendarCoverage(t, EventSearch, &calendarCoverageCaller{responses: map[string][]string{}}, "--query", "x", "--limit", "0"); err == nil {
		t.Fatal("search accepted bad limit")
	}
	if err := runCalendarCoverage(t, EventSearch, &calendarCoverageCaller{responses: map[string][]string{}}, "--query", "x", "--start", calendarCoverageEnd, "--end", calendarCoverageStart); err == nil {
		t.Fatal("search accepted reversed range")
	}
	for name, declarationAndTool := range map[string]struct {
		declaration shortcut.Shortcut
		tool        string
		args        []string
	}{
		"agenda": {EventList, "list_calendar_events", nil},
		"room":   {RoomFind, "query_available_meeting_room", []string{"--start", calendarCoverageStart, "--end", calendarCoverageEnd}},
		"busy":   {BusySearch, "query_busy_status", []string{"--users", "u", "--start", calendarCoverageStart, "--end", calendarCoverageEnd}},
	} {
		t.Run(name+"-call", func(t *testing.T) {
			caller := &calendarCoverageCaller{responses: map[string][]string{declarationAndTool.tool: {"__ERROR__"}}}
			if err := runCalendarCoverage(t, declarationAndTool.declaration, caller, declarationAndTool.args...); err == nil {
				t.Fatal("transport failure accepted")
			}
		})
	}

	room := &calendarCoverageCaller{responses: map[string][]string{"query_available_meeting_room": {`{"success":true,"result":{"rooms":[{"id":"room-1","name":"room"}],"hasMore":true,"totalCount":1,"pageIndex":0,"pageSize":20}}`}}}
	if err := runCalendarCoverage(t, RoomFind, room, "--group-id", "g", "--room-name", " room ", "--available"); err != nil {
		t.Fatal(err)
	}
	for _, response := range []string{`{"success":true,"result":{}}`, `{"success":true,"result":{"rooms":[{"name":"missing id"}]}}`} {
		caller := &calendarCoverageCaller{responses: map[string][]string{"query_available_meeting_room": {response}}}
		if err := runCalendarCoverage(t, RoomFind, caller); err == nil {
			t.Fatal("malformed room response accepted")
		}
	}
	agendaProjection := &calendarCoverageCaller{responses: map[string][]string{"list_calendar_events": {`{"success":true,"result":{"events":[{"summary":"missing id"}],"hasMore":false}}`}}}
	if err := runCalendarCoverage(t, EventList, agendaProjection); err == nil {
		t.Fatal("agenda item without id accepted")
	}

	for name, response := range map[string]string{
		"missing":  `{"success":true,"result":{"busy":[{}]}}`,
		"bad-list": `{"success":true,"result":{"busy":[{"scheduleItems":{}}]}}`,
		"bad-item": `{"success":true,"result":{"busy":[{"scheduleItems":["bad"]}]}}`,
		"bad-time": `{"success":true,"result":{"busy":[{"scheduleItems":[{"start":{"date":"2026-08-17"}}]}]}}`,
	} {
		t.Run("busy-"+name, func(t *testing.T) {
			caller := &calendarCoverageCaller{responses: map[string][]string{"query_busy_status": {response}}}
			if err := runCalendarCoverage(t, BusySearch, caller, "--users", "u", "--rooms", "r", "--start", calendarCoverageStart, "--end", calendarCoverageEnd); err == nil {
				t.Fatal("bad busy response accepted")
			}
		})
	}
	validBusy := &calendarCoverageCaller{responses: map[string][]string{"query_busy_status": {`{"success":true,"result":{"busy":[{"scheduleItems":[{"start":{"dateTime":"2026-08-17T09:00:00+08:00"},"end":{"date":"2026-08-17"}}]}]}}`}}}
	if err := runCalendarCoverage(t, BusySearch, validBusy, "--users", "u", "--rooms", "r", "--start", calendarCoverageStart, "--end", calendarCoverageEnd); err != nil {
		t.Fatal(err)
	}
	if calendarBusyDateTime("raw") != "raw" {
		t.Fatal("raw busy time was not preserved")
	}
}
