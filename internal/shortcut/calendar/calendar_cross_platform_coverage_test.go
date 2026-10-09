// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type calendarCoverageCaller struct {
	responses map[string][]string
	history   []string
	arguments []map[string]any
}

func (caller *calendarCoverageCaller) CallTool(_ context.Context, _, tool string, arguments map[string]any) (*edition.ToolResult, error) {
	caller.history = append(caller.history, tool)
	caller.arguments = append(caller.arguments, arguments)
	queue := caller.responses[tool]
	if len(queue) == 0 {
		return nil, errors.New("missing fake response for " + tool)
	}
	caller.responses[tool] = queue[1:]
	if queue[0] == "__ERROR__" {
		return nil, errors.New("injected calendar failure")
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: queue[0]}}}, nil
}

func (*calendarCoverageCaller) Format() string { return "json" }
func (*calendarCoverageCaller) DryRun() bool   { return false }
func (*calendarCoverageCaller) Fields() string { return "" }
func (*calendarCoverageCaller) JQ() string     { return "" }

func runCalendarCoverage(t *testing.T, declaration shortcut.Shortcut, caller *calendarCoverageCaller, args ...string) error {
	t.Helper()
	_, err := runCalendarCoverageCommand(t, declaration, caller, args...)
	return err
}

func runCalendarCoverageCommand(t *testing.T, declaration shortcut.Shortcut, caller *calendarCoverageCaller, args ...string) (*cobra.Command, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	service := &cobra.Command{Use: "calendar"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"calendar", declaration.Command}, args...))
	return corecmd.ExecuteCForTest(root)
}

func TestCrossPlatformCoverageCalendarListRequiresExplicitCollectionAndPagination(t *testing.T) {
	var explicitEmpty map[string]any
	if err := json.Unmarshal([]byte(`{"success":true,"result":{"events":[],"hasMore":false}}`), &explicitEmpty); err != nil {
		t.Fatal(err)
	}
	events, page, err := eventListProject(explicitEmpty)
	if err != nil || len(events) != 0 || !page.Known || page.HasMore {
		t.Fatalf("explicit empty: events=%v page=%+v err=%v", events, page, err)
	}
	var serviceEmptySentinel map[string]any
	if err := json.Unmarshal([]byte(`{"success":true,"result":{"events":[{"attendees":null,"categories":null,"meetingRooms":null,"reminders":null}],"hasMore":false}}`), &serviceEmptySentinel); err != nil {
		t.Fatal(err)
	}
	events, page, err = eventListProject(serviceEmptySentinel)
	if err != nil || events == nil || len(events) != 0 || !page.Known || page.HasMore {
		t.Fatalf("service empty sentinel: events=%#v page=%+v err=%v", events, page, err)
	}

	for name, payload := range map[string]string{
		"missing collection":   `{"success":true,"result":{"hasMore":false}}`,
		"bad collection":       `{"success":true,"result":{"events":{}}}`,
		"bad item":             `{"success":true,"result":{"events":["bad"],"hasMore":false}}`,
		"empty item":           `{"success":true,"result":{"events":[{}],"hasMore":false}}`,
		"unknown null item":    `{"success":true,"result":{"events":[{"summary":null}],"hasMore":false}}`,
		"sentinel with cursor": `{"success":true,"result":{"events":[{"attendees":null}],"hasMore":false,"nextCursor":"unexpected"}}`,
		"missing pagination":   `{"success":true,"result":{"events":[]}}`,
		"missing next cursor":  `{"success":true,"result":{"events":[],"hasMore":true}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal([]byte(payload), &data); err != nil {
				t.Fatal(err)
			}
			if _, _, err := eventListProject(data); err == nil {
				t.Fatalf("payload unexpectedly accepted: %s", payload)
			}
		})
	}

	var nextPage map[string]any
	if err := json.Unmarshal([]byte(`{"success":true,"result":{"events":[{"id":"event-1","summary":"review"}],"hasMore":true,"nextCursor":"cursor-2"}}`), &nextPage); err != nil {
		t.Fatal(err)
	}
	events, page, err = eventListProject(nextPage)
	if err != nil || len(events) != 1 || events[0]["eventId"] != "event-1" || !page.HasMore || page.NextCursor != "cursor-2" {
		t.Fatalf("next page projection: events=%#v page=%+v err=%v", events, page, err)
	}
}

func TestCrossPlatformCoverageCalendarObjectsAndWritesFailClosed(t *testing.T) {
	for name, payload := range map[string]map[string]any{
		"empty":               {},
		"success false":       {"success": false, "errorMsg": "denied"},
		"nonterminal receipt": {"result": map[string]any{"id": "event-1"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := requireCalendarWriteResponse(payload, "calendar/write"); err == nil {
				t.Fatalf("payload unexpectedly accepted: %#v", payload)
			}
		})
	}
	if _, err := requireCalendarWriteResponse(map[string]any{"success": true, "result": map[string]any{"id": "event-1"}}, "calendar/write"); err != nil {
		t.Fatalf("terminal write rejected: %v", err)
	}
	event, err := requireCalendarEvent(map[string]any{"success": true, "result": map[string]any{"id": "event-1", "summary": "review"}}, "calendar/get_calendar_detail", "event-1")
	if err != nil || normalizeCalendarEvent(event)["eventId"] != "event-1" {
		t.Fatalf("event normalization: event=%#v err=%v", event, err)
	}
	if _, err := requireCalendarEvent(map[string]any{"success": true, "result": map[string]any{"summary": "missing id"}}, "calendar/get_calendar_detail", "event-1"); err == nil {
		t.Fatal("event without id unexpectedly accepted")
	}
}

func TestCrossPlatformCoverageCalendarCreateReadsBackTerminalState(t *testing.T) {
	caller := &calendarCoverageCaller{responses: map[string][]string{
		"create_calendar_event": {`{"success":true,"result":{"id":"event-1"}}`},
		"get_calendar_detail":   {`{"success":true,"result":{"id":"event-1","summary":"review","start":{"dateTime":"2026-03-10T14:00:00+08:00"},"end":{"dateTime":"2026-03-10T15:00:00+08:00"}}}`},
	}}
	err := runCalendarCoverage(t, EventCreate, caller,
		"--title", "review", "--start", "2026-03-10T14:00:00+08:00", "--end", "2026-03-10T15:00:00+08:00", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(caller.history, ","); got != "create_calendar_event,get_calendar_detail" {
		t.Fatalf("history=%s", got)
	}

	noReceipt := &calendarCoverageCaller{responses: map[string][]string{"create_calendar_event": {`{"result":{"id":"event-2"}}`}}}
	err = runCalendarCoverage(t, EventCreate, noReceipt,
		"--title", "review", "--start", "2026-03-10T14:00:00+08:00", "--end", "2026-03-10T15:00:00+08:00", "--yes")
	if err == nil || !strings.Contains(err.Error(), "success=true") {
		t.Fatalf("non-terminal create error=%v", err)
	}
}

func TestCrossPlatformCoverageCalendarNestedReadbackReportsVerified(t *testing.T) {
	const event = `{"success":true,"result":{"id":"event-1","summary":"review","start":{"dateTime":"2026-08-17T09:00:00+08:00","timeZone":"Asia/Shanghai"},"end":{"dateTime":"2026-08-17T10:00:00+08:00","timeZone":"Asia/Shanghai"},"freeBusy":"BUSY"}}`
	for _, declaration := range []shortcut.Shortcut{EventCreate, EventUpdate} {
		t.Run(declaration.Command, func(t *testing.T) {
			args := []string{"--title", "review", "--start", calendarCoverageStart, "--end", calendarCoverageEnd, "--timezone", "Asia/Shanghai", "--free-busy", "busy", "--calendar-id", "team-calendar", "--yes"}
			responses := map[string][]string{"create_calendar_event": {`{"success":true,"result":{"id":"event-1"}}`}, "get_calendar_detail": {event}}
			wantHistory := "create_calendar_event,get_calendar_detail"
			writeIndex := 0
			if declaration.Command == EventUpdate.Command {
				args = append(args, "--event", "event-1")
				responses = map[string][]string{"get_calendar_detail": {event, event}, "update_calendar_event": {`{"success":true}`}}
				wantHistory = "get_calendar_detail,update_calendar_event,get_calendar_detail"
				writeIndex = 1
			}
			caller := &calendarCoverageCaller{responses: responses}
			cmd, err := runCalendarCoverageCommand(t, declaration, caller, args...)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			cmd.SetOut(&out)
			if code, emitted, err := output.EmitStoredResult(cmd); err != nil || !emitted || code != 0 {
				t.Fatalf("result code=%d emitted=%v err=%v", code, emitted, err)
			}
			var result struct {
				OK      bool   `json:"ok"`
				Outcome string `json:"outcome"`
				Data    struct {
					Success  bool           `json:"success"`
					EventID  string         `json:"eventId"`
					Verified bool           `json:"verified"`
					Event    map[string]any `json:"event"`
				} `json:"data"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatalf("decode output: %v; output=%s", err, out.String())
			}
			if !result.OK || result.Outcome != "success" || !result.Data.Success || !result.Data.Verified || result.Data.EventID != "event-1" || result.Data.Event["freeBusy"] != "BUSY" {
				t.Fatalf("incorrect verified result: %s", out.String())
			}
			if got := strings.Join(caller.history, ","); got != wantHistory {
				t.Fatalf("history=%s, want %s", got, wantHistory)
			}
			if args := caller.arguments[writeIndex]; args["timeZone"] != "Asia/Shanghai" || args["freeBusy"] != "busy" || args["calendarId"] != "team-calendar" {
				t.Fatalf("write arguments changed: %#v", args)
			}
		})
	}
}

func TestCrossPlatformCoverageCalendarReadbackFailuresKeepWriteReceipt(t *testing.T) {
	const preflight = `{"success":true,"result":{"id":"event-1"}}`
	const wrongTitle = `{"success":true,"result":{"id":"event-1","summary":"wrong","start":{"dateTime":"2026-08-17T09:00:00+08:00"},"end":{"dateTime":"2026-08-17T10:00:00+08:00"}}}`
	for _, declaration := range []shortcut.Shortcut{EventCreate, EventUpdate} {
		for name, payload := range map[string]string{
			"transport": "__ERROR__",
			"object":    `{"success":true,"result":{}}`,
			"id":        `{"success":true,"result":{"id":"other"}}`,
			"field":     wrongTitle,
		} {
			t.Run(declaration.Command+"/"+name, func(t *testing.T) {
				args := []string{"--title", "review", "--start", calendarCoverageStart, "--end", calendarCoverageEnd, "--calendar-id", "team-calendar", "--yes"}
				responses := map[string][]string{"create_calendar_event": {`{"success":true,"result":{"id":"event-1"}}`}, "get_calendar_detail": {payload}}
				wantSteps := []string{"create_event"}
				wantHistory := "create_calendar_event,get_calendar_detail"
				if declaration.Command == EventUpdate.Command {
					args = append(args, "--event", "event-1")
					responses = map[string][]string{"get_calendar_detail": {preflight, payload}, "update_calendar_event": {`{"success":true}`}}
					wantSteps = []string{"update_event"}
					wantHistory = "get_calendar_detail,update_calendar_event,get_calendar_detail"
				}
				caller := &calendarCoverageCaller{responses: responses}
				err := runCalendarCoverage(t, declaration, caller, args...)
				details := calendarVerificationDetailsForTest(t, err)
				receipt, _ := details["writeReceipt"].(map[string]any)
				if receipt["eventId"] != "event-1" || receipt["calendarId"] != "team-calendar" || receipt["failedStage"] != "verify_event" || receipt["verified"] != false || !reflect.DeepEqual(receipt["completedSteps"], wantSteps) {
					t.Fatalf("incorrect write receipt: %#v", details)
				}
				if name != "transport" && details["verificationReason"] == nil {
					t.Fatalf("lost verification reason: %#v", details)
				}
				if !strings.Contains(err.Error(), "eventId=event-1") || !strings.Contains(err.Error(), "成功写回执") || strings.Join(caller.history, ",") != wantHistory {
					t.Fatalf("lost write context or repeated write: err=%v history=%v", err, caller.history)
				}
				if declaration.Command == EventUpdate.Command {
					var typed *apperrors.Error
					if !errors.As(err, &typed) || typed.Reason != "partial_or_unknown_effect" || typed.Operation != "calendar/update" || typed.ExitCode() != 1 || typed.Cause == nil {
						t.Fatalf("update error contract changed: %#v", err)
					}
				}
			})
		}
	}
	for name, payload := range map[string]string{
		"transport":  "__ERROR__",
		"projection": `{"success":true,"result":{}}`,
		"set":        `{"success":true,"result":{"attendees":[]}}`,
	} {
		t.Run("attendees/"+name, func(t *testing.T) {
			caller := &calendarCoverageCaller{responses: map[string][]string{
				"get_calendar_detail":         {preflight, `{"success":true,"result":{"id":"event-1","summary":"review"}}`},
				"update_calendar_event":       {`{"success":true}`},
				"remove_calendar_participant": {`{"success":true}`},
				"add_calendar_participant":    {`{"success":true}`},
				"get_calendar_participants":   {payload},
			}}
			err := runCalendarCoverage(t, EventUpdate, caller, "--event", "event-1", "--title", "review", "--remove-attendees", "old", "--add-attendees", "new", "--yes")
			details := calendarVerificationDetailsForTest(t, err)
			receipt, _ := details["writeReceipt"].(map[string]any)
			if receipt["eventId"] != "event-1" || receipt["failedStage"] != "verify_attendees" || receipt["verified"] != false || !reflect.DeepEqual(receipt["completedSteps"], []string{"update_event", "remove_attendees", "add_attendees"}) {
				t.Fatalf("incorrect attendee receipt: %#v", details)
			}
			if _, exists := receipt["calendarId"]; exists {
				t.Fatalf("invented explicit calendar: %#v", receipt)
			}
			if got := strings.Join(caller.history, ","); got != "get_calendar_detail,update_calendar_event,remove_calendar_participant,add_calendar_participant,get_calendar_detail,get_calendar_participants" {
				t.Fatalf("unexpected replay or rollback: %s", got)
			}
		})
	}
}

func TestCrossPlatformCoverageCalendarVerificationErrorPreservesCause(t *testing.T) {
	createRT := calendarRuntimeForTest(t, EventCreate, map[string]string{"calendar-id": "team-calendar"})
	updateRT := calendarRuntimeForTest(t, EventUpdate, map[string]string{"event": "event-1"})
	for _, update := range []bool{false, true} {
		name := "create"
		if update {
			name = "update"
		}
		t.Run(name+"/typed", func(t *testing.T) {
			retryable := true
			nextRetry := time.Date(2026, 10, 7, 1, 0, 0, 0, time.UTC)
			original := apperrors.NewAuth("read authorization expired",
				apperrors.WithReason("read_token_expired"), apperrors.WithOperation("calendar/get_calendar_detail"),
				apperrors.WithExecutionStarted(false), apperrors.WithRetryable(true),
				apperrors.WithRetryAfterSeconds(5), apperrors.WithNextRetryAt(nextRetry),
				apperrors.WithHint("retry the original command"), apperrors.WithActions("dws calendar +create"),
				apperrors.WithRPCCode(-32001), apperrors.WithDetails(map[string]any{"diagnostic": "kept"}),
				apperrors.WithServerDiag(apperrors.ServerDiagnostics{
					TraceID: "trace-read", ServerErrorCode: "READ_EXPIRED", TechnicalDetail: "read diagnostic",
					FriendlyHint: "retry the whole operation", ActionURL: "https://example.invalid/retry", ServerRetryable: &retryable,
				}),
			).(*apperrors.Error)
			completed := []string{"update_event"}
			got := calendarCreateVerificationError(createRT, "event-1", original)
			if update {
				got = calendarUpdateVerificationError(updateRT, "verify_event", completed, original)
			}
			var enriched *apperrors.Error
			if !errors.As(got, &enriched) || enriched == original || !errors.Is(got, original) || enriched.Details["diagnostic"] != "kept" || enriched.Details["verificationReason"] != original.Reason {
				t.Fatalf("lost typed identity or details: %#v", got)
			}
			if update {
				if enriched.Category != apperrors.CategoryAPI || enriched.Reason != "partial_or_unknown_effect" || enriched.ExitCode() != 1 {
					t.Fatalf("update outer contract changed: %#v", enriched)
				}
				completed[0] = "mutated"
				receipt := enriched.Details["writeReceipt"].(map[string]any)
				if !reflect.DeepEqual(receipt["completedSteps"], []string{"update_event"}) {
					t.Fatalf("receipt aliases mutable stage list: %#v", receipt)
				}
			} else if enriched.Category != original.Category || enriched.Reason != original.Reason || enriched.Operation != original.Operation || enriched.ExitCode() != original.ExitCode() || enriched.RPCCode != original.RPCCode || enriched.ServerDiag.TraceID != "trace-read" || enriched.ServerDiag.ServerErrorCode != "READ_EXPIRED" || enriched.ServerDiag.TechnicalDetail != "read diagnostic" {
				t.Fatalf("create classification or diagnostics changed: %#v", enriched)
			}
			if enriched.ExecutionStarted == nil || !*enriched.ExecutionStarted || !enriched.RetryableSet || enriched.Retryable || enriched.RetryAfterSeconds != nil || enriched.NextRetryAt != nil || len(enriched.Actions) != 0 || enriched.ServerDiag.FriendlyHint != "" || enriched.ServerDiag.ActionURL != "" || enriched.ServerDiag.ServerRetryable != nil || !strings.Contains(enriched.Hint, "不要重复创建") {
				t.Fatalf("read-side retry guidance leaked: %#v", enriched)
			}
			if *original.ExecutionStarted || !original.Retryable || original.RetryAfterSeconds == nil || original.NextRetryAt == nil || original.Hint != "retry the original command" || len(original.Actions) != 1 || original.ServerDiag.FriendlyHint != "retry the whole operation" || original.ServerDiag.ActionURL == "" || original.ServerDiag.ServerRetryable == nil || len(original.Details) != 1 {
				t.Fatalf("original error was mutated: %#v", original)
			}
			enriched.Details["diagnostic"] = "changed"
			if original.Details["diagnostic"] != "kept" {
				t.Fatal("decorated details alias original details")
			}
		})
		t.Run(name+"/legacy", func(t *testing.T) {
			original := &helpers.CLIError{Code: helpers.CodeAuthTokenExpired, Message: "read token expired", Suggestion: "retry the original command", Operation: "calendar/get_calendar_detail", Details: map[string]any{"diagnostic": "kept"}}
			got := calendarCreateVerificationError(createRT, "event-1", original)
			if update {
				got = calendarUpdateVerificationError(updateRT, "verify_event", []string{"update_event"}, original)
			}
			if !errors.Is(got, original) || strings.Contains(got.Error(), original.Suggestion) || !strings.Contains(got.Error(), "eventId=event-1") {
				t.Fatalf("legacy identity lost or unsafe guidance rendered: %v", got)
			}
			if !update {
				var enriched *helpers.CLIError
				if !errors.As(got, &enriched) || enriched == original || enriched.Code != original.Code || enriched.Operation != original.Operation || enriched.ExitCode() != original.ExitCode() || !strings.Contains(enriched.Suggestion, "只读核对") {
					t.Fatalf("legacy classification changed: %#v", got)
				}
			}
			details := calendarVerificationDetailsForTest(t, got)
			if details["diagnostic"] != "kept" || details["writeReceipt"] == nil {
				t.Fatalf("legacy details lost: %#v", details)
			}
			details["diagnostic"] = "changed"
			if original.Details["diagnostic"] != "kept" || len(original.Details) != 1 || original.Suggestion != "retry the original command" || original.Cause != nil {
				t.Fatalf("original legacy error mutated: %#v", original)
			}
		})
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("read unavailable")} {
		created := calendarCreateVerificationError(createRT, "event-1", cause)
		updated := calendarUpdateVerificationError(updateRT, "verify_event", []string{"update_event"}, cause)
		if !errors.Is(created, cause) || !errors.Is(updated, cause) || apperrors.ExitCode(created) != apperrors.ExitCode(cause) || apperrors.ExitCode(updated) != 1 {
			t.Fatalf("error identity or exit changed: create=%v update=%v", created, updated)
		}
		if !strings.Contains(created.Error(), `"eventId":"event-1"`) || !strings.Contains(created.Error(), "成功写回执") || !strings.Contains(updated.Error(), "eventId=event-1") {
			t.Fatalf("untyped error lost textual receipt: create=%v update=%v", created, updated)
		}
	}
	for _, raw := range []error{&apperrors.PATError{RawJSON: `{"code":"PAT_NO_PERMISSION"}`}, &helpers.PATError{RawJSON: `{"code":"PAT_NO_PERMISSION"}`}} {
		if calendarCreateVerificationError(nil, "event-1", raw) != raw || calendarUpdateVerificationError(updateRT, "verify_event", []string{"update_event"}, raw) != raw {
			t.Fatal("host-owned raw error must remain untouched")
		}
	}
}

func TestCrossPlatformCoverageCalendarUnconfirmedWritesDoNotInventReceipt(t *testing.T) {
	for name, response := range map[string]string{
		"transport":    "__ERROR__",
		"no-success":   `{"result":{"id":"event-1"}}`,
		"missing-id":   `{"success":true,"result":{}}`,
		"unsuccessful": `{"success":false,"errorMsg":"denied"}`,
	} {
		t.Run("create/"+name, func(t *testing.T) {
			caller := &calendarCoverageCaller{responses: map[string][]string{"create_calendar_event": {response}}}
			err := runCalendarCoverage(t, EventCreate, caller, "--title", "review", "--start", calendarCoverageStart, "--end", calendarCoverageEnd, "--yes")
			if err == nil || strings.Contains(err.Error(), "成功写回执") || strings.Join(caller.history, ",") != "create_calendar_event" {
				t.Fatalf("unconfirmed write claimed receipt or continued: err=%v history=%v", err, caller.history)
			}
			if details := calendarVerificationDetailsForTest(t, err); details["writeReceipt"] != nil {
				t.Fatalf("invented write receipt: %#v", details)
			}
		})
	}
	for _, response := range []string{"__ERROR__", `{"result":{}}`} {
		caller := &calendarCoverageCaller{responses: map[string][]string{
			"get_calendar_detail":   {`{"success":true,"result":{"id":"event-1"}}`},
			"update_calendar_event": {response},
		}}
		err := runCalendarCoverage(t, EventUpdate, caller, "--event", "event-1", "--title", "review", "--yes")
		if err == nil || strings.Contains(err.Error(), "成功写回执") || strings.Join(caller.history, ",") != "get_calendar_detail,update_calendar_event" {
			t.Fatalf("unconfirmed update claimed receipt or continued: err=%v history=%v", err, caller.history)
		}
		if details := calendarVerificationDetailsForTest(t, err); details["writeReceipt"] != nil {
			t.Fatalf("invented update receipt: %#v", details)
		}
	}
}

func calendarVerificationDetailsForTest(t *testing.T, err error) map[string]any {
	t.Helper()
	var typed *apperrors.Error
	if errors.As(err, &typed) {
		return typed.Details
	}
	var legacy *helpers.CLIError
	if errors.As(err, &legacy) {
		return legacy.Details
	}
	t.Fatalf("expected classified verification failure, got %v", err)
	return nil
}

func TestCrossPlatformCoverageCalendarAlignedContracts(t *testing.T) {
	for _, declaration := range []shortcut.Shortcut{EventGet, EventCreate, EventUpdate, RSVP, EventSearch, Suggestion, RoomFind} {
		if declaration.Contract.Empty() || declaration.Contract.Result == nil {
			t.Errorf("%s missing contract/result", declaration.Command)
		}
		if strings.TrimSpace(declaration.Safety.Effect) == "" || strings.TrimSpace(declaration.Safety.Confirmation) == "" {
			t.Errorf("%s missing safety", declaration.Command)
		}
		if declaration.OutputRollout != output.RolloutUnifiedActive {
			t.Errorf("%s output rollout=%q", declaration.Command, declaration.OutputRollout)
		}
	}
	if EventSearch.Contract.Pagination == nil || EventList.Contract.Pagination == nil {
		t.Fatal("cursor commands must publish pagination contracts")
	}
	if EventCreate.Safety.Confirmation != "user_required" || EventUpdate.Safety.Confirmation != "user_required" || RSVP.Safety.Confirmation != "user_required" {
		t.Fatal("calendar writes must require confirmation")
	}
}

func TestCrossPlatformCoverageCalendarRoomFindPreservesPublishedFlags(t *testing.T) {
	flags := make(map[string]shortcut.Flag, len(RoomFind.Flags))
	for _, flag := range RoomFind.Flags {
		flags[flag.Name] = flag
	}
	if flag := flags["available"]; flag.Type != shortcut.FlagBool || flag.Hidden {
		t.Fatalf("available flag=%+v, want visible bool", flag)
	}
	for _, name := range []string{"limit", "page"} {
		if flag := flags[name]; flag.Type != shortcut.FlagString || flag.Hidden {
			t.Fatalf("%s flag=%+v, want visible string", name, flag)
		}
	}

	caller := &calendarCoverageCaller{responses: map[string][]string{
		"query_available_meeting_room": {`{"success":true,"result":{"rooms":[],"hasMore":false}}`},
	}}
	err := runCalendarCoverage(t, RoomFind, caller,
		"--start", "2026-03-10T14:00:00+08:00",
		"--end", "2026-03-10T15:00:00+08:00",
		"--available", "--limit", "25", "--page", "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.arguments) != 1 {
		t.Fatalf("calls=%d, want 1", len(caller.arguments))
	}
	arguments := caller.arguments[0]
	if arguments["pageSize"] != "25" || arguments["pageIndex"] != "2" || arguments["needAvailable"] != true {
		t.Fatalf("arguments=%#v", arguments)
	}

	invalid := &calendarCoverageCaller{responses: map[string][]string{}}
	if err := runCalendarCoverage(t, RoomFind, invalid, "--limit", "101"); err == nil || !strings.Contains(err.Error(), "1-100") {
		t.Fatalf("invalid limit error=%v", err)
	}
	if len(invalid.history) != 0 {
		t.Fatalf("invalid input made calls: %v", invalid.history)
	}

	fixedNow := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	defaultParams, err := calendarAgendaParams(calendarRuntimeForTest(t, EventList, nil), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if defaultParams["startTime"] != time.Date(2026, time.August, 17, 0, 0, 0, 0, fixedNow.Location()).UnixMilli() ||
		defaultParams["endTime"] != time.Date(2026, time.August, 17, 23, 59, 59, 0, fixedNow.Location()).UnixMilli() {
		t.Fatalf("agenda default adapter arguments=%#v", defaultParams)
	}

	allParams, err := calendarAgendaParams(calendarRuntimeForTest(t, EventList, map[string]string{
		"start":       calendarCoverageStart,
		"end":         calendarCoverageEnd,
		"calendar-id": "primary",
		"cursor":      "cursor-1",
		"limit":       "7",
	}), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if allParams["calendarId"] != "primary" || allParams["cursor"] != "cursor-1" || allParams["limit"] != 7 {
		t.Fatalf("agenda optional adapter arguments=%#v", allParams)
	}

	for name, values := range map[string]map[string]string{
		"invalid-start": {"start": "bad"},
		"invalid-end":   {"end": "bad"},
		"reversed":      {"start": calendarCoverageEnd, "end": calendarCoverageStart},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := calendarAgendaParams(calendarRuntimeForTest(t, EventList, values), fixedNow); err == nil {
				t.Fatal("invalid agenda adapter input was accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageCalendarAgendaSchemaUsesExplicitRuntimeAdapter(t *testing.T) {
	properties := make(map[string]string, len(EventList.Contract.Parameters))
	for _, parameter := range EventList.Contract.Parameters {
		properties[parameter.Name] = parameter.Property
	}
	if properties["start"] != "start" || properties["end"] != "end" {
		t.Fatalf("agenda time properties=%#v", properties)
	}
	for _, flag := range EventList.Flags {
		if flag.Name == "limit" && flag.Default != "" {
			t.Fatalf("agenda limit default=%q, want published empty default", flag.Default)
		}
	}

	caller := &calendarCoverageCaller{responses: map[string][]string{
		"list_calendar_events": {`{"success":true,"result":{"events":[],"hasMore":false}}`},
	}}
	if err := runCalendarCoverage(t, EventList, caller,
		"--start", calendarCoverageStart,
		"--end", calendarCoverageEnd,
	); err != nil {
		t.Fatal(err)
	}
	if len(caller.arguments) != 1 {
		t.Fatalf("calls=%d, want 1", len(caller.arguments))
	}
	if _, ok := caller.arguments[0]["limit"]; ok {
		t.Fatalf("unset limit unexpectedly sent: %#v", caller.arguments[0])
	}
	start, err := parseMillis("start", calendarCoverageStart)
	if err != nil {
		t.Fatal(err)
	}
	end, err := parseMillis("end", calendarCoverageEnd)
	if err != nil {
		t.Fatal(err)
	}
	if caller.arguments[0]["startTime"] != start || caller.arguments[0]["endTime"] != end {
		t.Fatalf("agenda adapter arguments=%#v, published properties=%#v", caller.arguments[0], properties)
	}
	if _, leaked := caller.arguments[0][properties["start"]]; leaked {
		t.Fatalf("published composite property leaked into RPC arguments: %#v", caller.arguments[0])
	}
	if _, leaked := caller.arguments[0][properties["end"]]; leaked {
		t.Fatalf("published composite property leaked into RPC arguments: %#v", caller.arguments[0])
	}

	invalid := &calendarCoverageCaller{responses: map[string][]string{}}
	if err := runCalendarCoverage(t, EventList, invalid, "--limit", "101"); err == nil || !strings.Contains(err.Error(), "1") {
		t.Fatalf("invalid limit error=%v", err)
	}
	if len(invalid.history) != 0 {
		t.Fatalf("invalid input made calls: %v", invalid.history)
	}
}

func TestCrossPlatformCoverageCalendarAttendeeProjectionAcceptsStableUserID(t *testing.T) {
	attendees, err := attendeeListProject(map[string]any{
		"success": true,
		"result": map[string]any{
			"attendees": []any{map[string]any{
				"userId":         "user-1",
				"responseStatus": "accepted",
			}},
		},
	})
	if err != nil {
		t.Fatalf("userId-only attendee rejected: %v", err)
	}
	if len(attendees) != 1 || attendees[0]["userId"] != "user-1" {
		t.Fatalf("attendees=%#v", attendees)
	}

	if _, err := attendeeListProject(map[string]any{
		"success": true,
		"result": map[string]any{
			"attendees": []any{map[string]any{"responseStatus": "accepted"}},
		},
	}); err == nil {
		t.Fatal("attendee without displayName or userId was accepted")
	}
}
