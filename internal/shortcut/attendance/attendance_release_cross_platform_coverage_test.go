// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package attendance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func attendanceRuntimeForTest(t *testing.T, declaration shortcut.Shortcut, values map[string]string) *shortcut.RuntimeContext {
	t.Helper()
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	return shortcut.RuntimeContextForTest(cmd, declaration)
}

type attendanceCoverageCaller struct {
	text      string
	err       error
	calls     int
	tools     []string
	arguments []map[string]any
}

func (caller *attendanceCoverageCaller) CallTool(_ context.Context, _, tool string, arguments map[string]any) (*edition.ToolResult, error) {
	caller.calls++
	caller.tools = append(caller.tools, tool)
	caller.arguments = append(caller.arguments, arguments)
	if caller.err != nil {
		return nil, caller.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: caller.text}}}, nil
}

func (*attendanceCoverageCaller) Format() string { return "json" }
func (*attendanceCoverageCaller) DryRun() bool   { return false }
func (*attendanceCoverageCaller) Fields() string { return "" }
func (*attendanceCoverageCaller) JQ() string     { return "" }

func executeAttendanceResponse(t *testing.T, declaration shortcut.Shortcut, values map[string]string, response string) (*attendanceCoverageCaller, error) {
	t.Helper()
	caller := &attendanceCoverageCaller{text: response}
	helpers.InitDepsForTest(t, caller)
	err := declaration.Execute(attendanceRuntimeForTest(t, declaration, values))
	return caller, err
}

func TestCrossPlatformCoverageAttendanceRequestBindingMatrices(t *testing.T) {
	userCases := []struct {
		name  string
		items []map[string]any
		valid bool
	}{
		{"valid", []map[string]any{{"userId": "u1", "workDate": float64(1000)}}, true},
		{"missing user", []map[string]any{{"workDate": float64(1000)}}, false},
		{"wrong user type", []map[string]any{{"userId": 1, "workDate": float64(1000)}}, false},
		{"blank user", []map[string]any{{"userId": " ", "workDate": float64(1000)}}, false},
		{"wrong user", []map[string]any{{"userId": "u2", "workDate": float64(1000)}}, false},
		{"missing time", []map[string]any{{"userId": "u1"}}, false},
		{"wrong time type", []map[string]any{{"userId": "u1", "workDate": "1000"}}, false},
		{"before range", []map[string]any{{"userId": "u1", "workDate": float64(899)}}, false},
		{"after range", []map[string]any{{"userId": "u1", "workDate": float64(1101)}}, false},
	}
	for _, tc := range userCases {
		err := attendanceValidateUserAndTimeBinding(tc.items, "attendance/test", []string{"u1"}, "userId", "workDate", 900, 1100)
		if (err == nil) != tc.valid {
			t.Errorf("user/time %s: err=%v valid=%t", tc.name, err, tc.valid)
		}
	}

	approvalCases := []struct {
		name  string
		item  map[string]any
		valid bool
	}{
		{"valid", map[string]any{"userId": "u1", "bizType": float64(3), "beginTime": float64(900), "endTime": float64(1100)}, true},
		{"missing user", map[string]any{"bizType": float64(3), "beginTime": float64(900), "endTime": float64(1100)}, false},
		{"wrong user", map[string]any{"userId": "u2", "bizType": float64(3), "beginTime": float64(900), "endTime": float64(1100)}, false},
		{"missing type", map[string]any{"userId": "u1", "beginTime": float64(900), "endTime": float64(1100)}, false},
		{"wrong type", map[string]any{"userId": "u1", "bizType": float64(4), "beginTime": float64(900), "endTime": float64(1100)}, false},
		{"invalid range", map[string]any{"userId": "u1", "bizType": float64(3), "beginTime": float64(1001), "endTime": float64(999)}, false},
		{"before range", map[string]any{"userId": "u1", "bizType": float64(3), "beginTime": float64(700), "endTime": float64(899)}, false},
		{"crosses start", map[string]any{"userId": "u1", "bizType": float64(3), "beginTime": float64(800), "endTime": float64(900)}, true},
		{"crosses end", map[string]any{"userId": "u1", "bizType": float64(3), "beginTime": float64(1100), "endTime": float64(1200)}, true},
		{"after range", map[string]any{"userId": "u1", "bizType": float64(3), "beginTime": float64(1101), "endTime": float64(1200)}, false},
	}
	for _, tc := range approvalCases {
		err := attendanceValidateApprovalBinding([]map[string]any{tc.item}, "attendance/test", []string{"u1"}, map[int]struct{}{3: {}}, 900, 1100)
		if (err == nil) != tc.valid {
			t.Errorf("approval %s: err=%v valid=%t", tc.name, err, tc.valid)
		}
	}
	if err := attendanceValidateExpectedStrings([]map[string]any{{"approveType": "LEAVE"}}, "attendance/test", "approveType", "OVERTIME"); err == nil {
		t.Fatal("expected string mismatch was accepted")
	}
}

func TestCrossPlatformCoverageAttendanceCheckRecordActualTimeBinding(t *testing.T) {
	const startMillis, endMillis = int64(1000), int64(2000)
	items := []map[string]any{{"id": 1, "userId": "u1", "workDate": startMillis - 24*60*60*1000, "userCheckTime": startMillis}}
	if err := attendanceValidateUserAndTimeBinding(items, "attendance/test", []string{"u1"}, "userId", "userCheckTime", startMillis, endMillis); err != nil {
		t.Fatalf("cross-midnight work date with in-range actual check was rejected: %v", err)
	}

	invalid := []struct {
		name   string
		item   map[string]any
		reason string
	}{
		{"before range", map[string]any{"userId": "u1", "userCheckTime": startMillis - 1}, "request_range_mismatch"},
		{"after range", map[string]any{"userId": "u1", "userCheckTime": endMillis + 1}, "request_range_mismatch"},
		{"missing actual time", map[string]any{"userId": "u1", "workDate": startMillis}, "missing_request_binding"},
		{"null actual time", map[string]any{"userId": "u1", "userCheckTime": nil}, "malformed_request_binding"},
		{"wrong actual time type", map[string]any{"userId": "u1", "userCheckTime": "1000"}, "malformed_request_binding"},
		{"zero actual time", map[string]any{"userId": "u1", "userCheckTime": 0}, "malformed_request_binding"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			err := attendanceValidateUserAndTimeBinding([]map[string]any{tc.item}, "attendance/test", []string{"u1"}, "userId", "userCheckTime", startMillis, endMillis)
			typed, ok := err.(*apperrors.Error)
			reason := ""
			if ok {
				reason = typed.Reason
			}
			if err == nil || !ok || typed.Reason != tc.reason {
				t.Fatalf("err=%v reason=%q want=%q", err, reason, tc.reason)
			}
		})
	}
}

func TestCrossPlatformCoverageAttendanceCheckRecordExecuteBindsActualCheckTime(t *testing.T) {
	values := map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31"}
	startMillis, _ := dayMillis(values["start"])
	execute := func(t *testing.T, records []map[string]any) (map[string]any, error) {
		t.Helper()
		caller := &attendanceCoverageCaller{text: attendanceResponseJSON(t, map[string]any{"success": true, "result": records})}
		helpers.InitDepsForTest(t, caller)
		cmd := corecmd.New(shortcut.FromShortcut(CheckRecord))
		for name, value := range values {
			if err := cmd.Flags().Set(name, value); err != nil {
				t.Fatalf("set --%s: %v", name, err)
			}
		}
		ctx, _ := output.WithResultStore(context.Background())
		cmd.SetContext(ctx)
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(io.Discard)
		err := CheckRecord.Execute(shortcut.RuntimeContextForTest(cmd, CheckRecord))
		if err != nil {
			return nil, err
		}
		if caller.calls != 1 {
			t.Fatalf("calls=%d", caller.calls)
		}
		if code, emitted, err := output.EmitStoredResult(cmd); err != nil || !emitted || code != 0 {
			t.Fatalf("emit code=%d emitted=%t err=%v", code, emitted, err)
		}
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		return envelope, nil
	}

	t.Run("keeps cross-midnight work date when actual check is in range", func(t *testing.T) {
		envelope, err := execute(t, []map[string]any{
			{"id": 1, "userId": "u1", "workDate": startMillis - 24*60*60*1000, "userCheckTime": startMillis + 1000},
			{"id": 2, "userId": "u1", "workDate": startMillis, "userCheckTime": startMillis},
		})
		if err != nil {
			t.Fatal(err)
		}
		data, ok := envelope["data"].(map[string]any)
		records, recordsOK := data["records"].([]any)
		if !ok || !recordsOK || data["count"] != float64(2) || len(records) != 2 || records[0].(map[string]any)["id"] != float64(1) || records[1].(map[string]any)["id"] != float64(2) {
			t.Fatalf("data=%#v", envelope["data"])
		}
	})

	t.Run("actual-time overfetch fails closed", func(t *testing.T) {
		if _, err := execute(t, []map[string]any{{"id": 1, "userId": "u1", "workDate": startMillis, "userCheckTime": startMillis - 1}}); err == nil {
			t.Fatal("out-of-range actual check was silently filtered")
		}
	})

	invalid := []struct {
		name    string
		records []map[string]any
	}{
		{"missing identity outside range", []map[string]any{{"userId": "u1", "userCheckTime": startMillis - 1}}},
		{"zero identity outside range", []map[string]any{{"id": 0, "userId": "u1", "userCheckTime": startMillis - 1}}},
		{"wrong user outside range", []map[string]any{{"id": 1, "userId": "other", "userCheckTime": startMillis - 1}}},
		{"missing actual time", []map[string]any{{"id": 1, "userId": "u1", "workDate": startMillis - 1}}},
		{"malformed actual time outside range", []map[string]any{{"id": 1, "userId": "u1", "userCheckTime": "bad"}}},
		{"duplicate identities outside range", []map[string]any{{"id": 1, "userId": "u1", "userCheckTime": startMillis - 2}, {"id": 1, "userId": "u1", "userCheckTime": startMillis - 1}}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := execute(t, tc.records); err == nil {
				t.Fatal("invalid raw overfetch was hidden")
			}
		})
	}

	t.Run("downstream failure", func(t *testing.T) {
		caller := &attendanceCoverageCaller{err: fmt.Errorf("backend unavailable")}
		helpers.InitDepsForTest(t, caller)
		if err := CheckRecord.Execute(attendanceRuntimeForTest(t, CheckRecord, values)); err == nil || caller.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, caller.calls)
		}
	})

	t.Run("missing collection", func(t *testing.T) {
		caller, err := executeAttendanceResponse(t, CheckRecord, values, `{"success":true,"result":null}`)
		if err == nil || caller.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, caller.calls)
		}
	})
}

func TestCrossPlatformCoverageAttendanceDetailExecuteFailsClosed(t *testing.T) {
	detailCases := []struct {
		name        string
		declaration shortcut.Shortcut
		flags       map[string]string
		valid       string
		invalid     []string
	}{
		{
			name:        "class",
			declaration: GetClass,
			flags:       map[string]string{"class-id": "7"},
			valid:       `{"success":true,"result":{"shiftVO":{"id":7}}}`,
			invalid: []string{
				`{"result":{"shiftVO":{"id":7}}}`,
				`{"success":false,"result":{"shiftVO":{"id":7}}}`,
				`{"success":true,"result":null}`,
				`{"success":true,"result":"bad"}`,
				`{"success":true,"result":{"shiftVO":{"id":8}}}`,
			},
		},
		{
			name:        "overtime",
			declaration: GetOvertimeRule,
			flags:       map[string]string{"overtime-id": "9"},
			valid:       `{"success":true,"result":[{"id":9}]}`,
			invalid: []string{
				`{"result":[{"id":9}]}`,
				`{"success":false,"result":[{"id":9}]}`,
				`{"success":true,"result":null}`,
				`{"success":true,"result":["bad"]}`,
				`{"success":true,"result":[{"id":10}]}`,
			},
		},
	}
	for _, detail := range detailCases {
		t.Run(detail.name+" valid", func(t *testing.T) {
			caller, err := executeAttendanceResponse(t, detail.declaration, detail.flags, detail.valid)
			if err != nil || caller.calls != 1 {
				t.Fatalf("valid detail err=%v calls=%d", err, caller.calls)
			}
		})
		for index, response := range detail.invalid {
			t.Run(fmt.Sprintf("%s invalid %d", detail.name, index), func(t *testing.T) {
				caller, err := executeAttendanceResponse(t, detail.declaration, detail.flags, response)
				if err == nil || caller.calls != 1 {
					t.Fatalf("invalid detail err=%v calls=%d", err, caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAttendanceSelfSettingExecuteBinding(t *testing.T) {
	invalid := []string{
		`{"result":{"userId":"user","checkRemindSetting":{}}}`,
		`{"success":false,"result":{"userId":"user","checkRemindSetting":{}}}`,
		`{"success":true,"result":null}`,
		`{"success":true,"result":"bad"}`,
		`{"success":true,"result":{"userId":"other","checkRemindSetting":{}}}`,
		`{"success":true,"result":{"userId":"user"}}`,
		`{"success":true,"result":{"userId":"user","checkRemindSetting":null}}`,
		`{"success":true,"result":{"userId":"user","checkRemindSetting":true}}`,
	}
	for index, response := range invalid {
		t.Run(fmt.Sprintf("invalid %d", index), func(t *testing.T) {
			caller, err := executeAttendanceResponse(t, GetSelfSetting, map[string]string{"user": "user", "setting-scene": "checkRemind"}, response)
			if err == nil || caller.calls != 1 {
				t.Fatalf("invalid self setting err=%v calls=%d", err, caller.calls)
			}
		})
	}
	t.Run("normalizes user once", func(t *testing.T) {
		caller, err := executeAttendanceResponse(t, GetSelfSetting, map[string]string{"user": " user ", "setting-scene": "checkRemind"}, `{"success":true,"result":{"userId":"user","checkRemindSetting":{}}}`)
		if err != nil || caller.calls != 1 {
			t.Fatalf("normalized self setting err=%v calls=%d", err, caller.calls)
		}
		request, ok := caller.arguments[0]["RuleMcpQuerySelfSettingRequest"].(map[string]any)
		if !ok || request["userId"] != "user" {
			t.Fatalf("normalized request = %#v", caller.arguments[0])
		}
	})

	validTypes := []struct {
		scene string
		field string
		value any
	}{
		{"checkRemind", "checkRemindSetting", map[string]any{}},
		{"fastCheck", "fastCheckLateNeedConfirm", false},
		{"checkResultNotify", "checkResultMsg", float64(0)},
		{"lackRemind", "lackRemindUser", float64(1)},
		{"personalAttendStatNotify", "personDailyReportSwitch", float64(1)},
		{"bossAttendStatNotify", "bossMonthReportType", float64(3)},
	}
	for _, item := range validTypes {
		value := map[string]any{"userId": "user", item.field: item.value}
		if err := attendanceValidateSelfSetting(value, "user", item.scene); err != nil {
			t.Errorf("scene %s rejected observed type: %v", item.scene, err)
		}
	}

}

func TestCrossPlatformCoverageAttendanceParamMappingsAreExplicit(t *testing.T) {
	expected := map[*shortcut.Shortcut]map[string]string{
		&CheckResult:          {"users": "users", "start": "start", "end": "end", "offset": "offset", "limit": "limit"},
		&CheckRecord:          {"users": "users", "start": "start", "end": "end"},
		&ListApprove:          {"users": "users", "types": "types", "start": "start", "end": "end"},
		&GetApproveTemplate:   {"type": "type"},
		&GetSchedule:          {"users": "users", "start": "start", "end": "end"},
		&SearchClass:          {"query": "query", "filter-type": "filterType", "page": "page", "limit": "limit"},
		&GetClass:             {"class-id": "classId"},
		&SearchAdjustmentRule: {"query": "query", "page": "page", "limit": "limit"},
		&GetOvertimeRule:      {"overtime-id": "overtimeId"},
		&SearchOvertimeRule:   {"query": "query", "page": "page", "limit": "limit"},
		&GetSelfSetting:       {"setting-scene": "settingScene", "user": "user"},
	}
	for declaration, want := range expected {
		got := make(map[string]string, len(declaration.Contract.Parameters))
		for _, parameter := range declaration.Contract.Parameters {
			got[parameter.Name] = parameter.Property
		}
		if len(got) != len(want) {
			t.Errorf("%s parameter count=%d want=%d: %#v", declaration.Command, len(got), len(want), got)
		}
		for name, property := range want {
			if got[name] != property {
				t.Errorf("%s --%s property=%q want=%q", declaration.Command, name, got[name], property)
			}
		}
	}
}

func TestCrossPlatformCoverageAttendanceRuntimeConstraintsFailBeforeCall(t *testing.T) {
	manyUsers := make([]string, 101)
	for index := range manyUsers {
		manyUsers[index] = fmt.Sprintf("u%d", index)
	}
	cases := []struct {
		name        string
		declaration shortcut.Shortcut
		values      map[string]string
	}{
		{"check result too many users", CheckResult, map[string]string{"users": strings.Join(manyUsers, ","), "start": "2026-01-01", "end": "2026-01-31"}},
		{"check result reversed dates", CheckResult, map[string]string{"users": "u1", "start": "2026-02-01", "end": "2026-01-31"}},
		{"check result over month", CheckResult, map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-02-02"}},
		{"check result bad limit", CheckResult, map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31", "limit": "1001"}},
		{"check result bad offset", CheckResult, map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31", "offset": "-1"}},
		{"check record duplicate users", CheckRecord, map[string]string{"users": "u1,u1", "start": "2026-01-01", "end": "2026-01-31"}},
		{"approve empty types", ListApprove, map[string]string{"users": "u1", "types": ",", "start": "2026-01-01", "end": "2026-01-31"}},
		{"approve reversed dates", ListApprove, map[string]string{"users": "u1", "types": "leave", "start": "2026-02-01", "end": "2026-01-31"}},
		{"schedule empty users", GetSchedule, map[string]string{"users": ",", "start": "2026-01-01", "end": "2026-01-31"}},
		{"schedule reversed dates", GetSchedule, map[string]string{"users": "u1", "start": "2026-02-01", "end": "2026-01-31"}},
		{"search class page zero", SearchClass, map[string]string{"page": "0"}},
		{"search adjustment limit too large", SearchAdjustmentRule, map[string]string{"limit": "201"}},
		{"search overtime limit zero", SearchOvertimeRule, map[string]string{"limit": "0"}},
		{"class id zero", GetClass, map[string]string{"class-id": "0"}},
		{"overtime id zero", GetOvertimeRule, map[string]string{"overtime-id": "0"}},
		{"leave records reversed dates", GetLeaveRecords, map[string]string{"user": "u1", "start": "2026-02-01", "end": "2026-01-31"}},
		{"checkin duplicate users", GetCheckinRecord, map[string]string{"operator-corp-id": "corp", "operator-staff-id": "operator", "staff-ids": "u1,u1", "start": "2026-01-01 00:00:00", "end": "2026-01-02 00:00:00"}},
		{"checkin over seven days", GetCheckinRecord, map[string]string{"operator-corp-id": "corp", "operator-staff-id": "operator", "staff-ids": "u1", "start": "2026-01-01 00:00:00", "end": "2026-01-08 00:00:01"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &attendanceCoverageCaller{text: `{"success":true,"result":[]}`}
			helpers.InitDepsForTest(t, caller)
			rt := attendanceRuntimeForTest(t, tc.declaration, tc.values)
			if tc.declaration.Validate == nil {
				t.Fatalf("%s has no Validate", tc.declaration.Command)
			}
			if err := tc.declaration.Validate(rt); err == nil {
				t.Fatalf("invalid values accepted for %s", tc.declaration.Command)
			}
			if caller.calls != 0 {
				t.Fatalf("%s made %d downstream calls before validation", tc.declaration.Command, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAttendancePaginationLivesInEnvelopeMeta(t *testing.T) {
	emit := func(complete bool, nextToken string) map[string]any {
		t.Helper()
		cmd := corecmd.New(shortcut.FromShortcut(CheckResult))
		ctx, _ := output.WithResultStore(context.Background())
		cmd.SetContext(ctx)
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(io.Discard)
		rt := shortcut.RuntimeContextForTest(cmd, CheckResult)
		items := []map[string]any{{"id": int64(1), "userId": "redacted", "workDate": int64(1)}}
		if err := attendanceOutputCollection(rt, "records", items, complete, map[string]any{"limit": 1, "nextOffset": 1}, true, nextToken); err != nil {
			t.Fatal(err)
		}
		if _, emitted, err := output.EmitStoredResult(cmd); err != nil || !emitted {
			t.Fatalf("emit err=%v emitted=%t", err, emitted)
		}
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		return envelope
	}

	envelope := emit(false, "1")
	data, ok := envelope["data"].(map[string]any)
	if !ok || data["count"] != float64(1) {
		t.Fatalf("business data=%#v", envelope["data"])
	}
	for _, forbidden := range []string{"complete", "limit", "nextOffset", "nextPage", "totalCount", "totalPage"} {
		if _, present := data[forbidden]; present {
			t.Errorf("pagination field %q leaked into business data", forbidden)
		}
	}
	meta, ok := envelope["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta=%#v", envelope["meta"])
	}
	pagination, ok := meta["pagination"].(map[string]any)
	if !ok || pagination["endpoint_exhausted"] != false || pagination["next_token"] != "1" {
		t.Fatalf("pagination=%#v", meta["pagination"])
	}
	exhaustedEnvelope := emit(true, "")
	exhaustedMeta := exhaustedEnvelope["meta"].(map[string]any)
	exhaustedPagination := exhaustedMeta["pagination"].(map[string]any)
	if exhaustedPagination["endpoint_exhausted"] != true {
		t.Fatalf("exhausted pagination=%#v", exhaustedPagination)
	}
	if _, present := exhaustedPagination["next_token"]; present {
		t.Fatalf("exhausted pagination published next_token: %#v", exhaustedPagination)
	}
}

func TestCrossPlatformCoverageAttendanceCommonCallersFailClosed(t *testing.T) {
	tests := []struct {
		name string
		run  func(*shortcut.RuntimeContext) error
		text string
		err  error
		want bool
	}{
		{
			name: "collection downstream error",
			run: func(rt *shortcut.RuntimeContext) error {
				return attendanceCallCollection(rt, "attendance-test", "collection", nil, "items", true, nil, nil, "result")
			},
			err:  fmt.Errorf("downstream unavailable"),
			want: false,
		},
		{
			name: "collection malformed",
			run: func(rt *shortcut.RuntimeContext) error {
				return attendanceCallCollection(rt, "attendance-test", "collection", nil, "items", true, nil, nil, "result")
			},
			text: `{"success":true,"result":null}`,
			want: false,
		},
		{
			name: "collection validator",
			run: func(rt *shortcut.RuntimeContext) error {
				return attendanceCallCollection(rt, "attendance-test", "collection", nil, "items", true, nil, func([]map[string]any) error {
					return fmt.Errorf("binding mismatch")
				}, "result")
			},
			text: `{"success":true,"result":[]}`,
			want: false,
		},
		{
			name: "collection success",
			run: func(rt *shortcut.RuntimeContext) error {
				return attendanceCallCollection(rt, "attendance-test", "collection", nil, "items", true, map[string]any{"source": "test"}, nil, "result")
			},
			text: `{"success":true,"result":[]}`,
			want: true,
		},
		{
			name: "value downstream error",
			run: func(rt *shortcut.RuntimeContext) error {
				return attendanceCallValue(rt, "attendance-test", "value", nil)
			},
			err:  fmt.Errorf("downstream unavailable"),
			want: false,
		},
		{
			name: "value malformed",
			run: func(rt *shortcut.RuntimeContext) error {
				return attendanceCallValue(rt, "attendance-test", "value", nil)
			},
			text: `{"success":true}`,
			want: false,
		},
		{
			name: "value success",
			run: func(rt *shortcut.RuntimeContext) error {
				return attendanceCallValue(rt, "attendance-test", "value", nil)
			},
			text: `{"success":true,"result":false}`,
			want: true,
		},
		{
			name: "object downstream error",
			run: func(rt *shortcut.RuntimeContext) error {
				return attendanceCallObject(rt, "attendance-test", "object", nil)
			},
			err:  fmt.Errorf("downstream unavailable"),
			want: false,
		},
		{
			name: "object malformed",
			run: func(rt *shortcut.RuntimeContext) error {
				return attendanceCallObject(rt, "attendance-test", "object", nil)
			},
			text: `{"success":true,"result":[]}`,
			want: false,
		},
		{
			name: "object success",
			run: func(rt *shortcut.RuntimeContext) error {
				return attendanceCallObject(rt, "attendance-test", "object", nil)
			},
			text: `{"success":true,"result":{"id":1}}`,
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &attendanceCoverageCaller{text: tc.text, err: tc.err}
			helpers.InitDepsForTest(t, caller)
			rt := attendanceRuntimeForTest(t, CheckResult, nil)
			err := tc.run(rt)
			if (err == nil) != tc.want || caller.calls != 1 {
				t.Fatalf("err=%v calls=%d wantSuccess=%t", err, caller.calls, tc.want)
			}
		})
	}

	t.Run("legacy output", func(t *testing.T) {
		rt := attendanceRuntimeForTest(t, ListLeaveTypes, nil)
		if err := attendanceOutputCollection(rt, "items", []map[string]any{{"leaveCode": "code"}}, true, map[string]any{"source": "legacy"}, false, ""); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("invalid continuation", func(t *testing.T) {
		rt := attendanceRuntimeForTest(t, CheckResult, nil)
		if err := attendanceOutputCollection(rt, "records", nil, false, nil, true, ""); err == nil {
			t.Fatal("non-terminal pagination without next token was accepted")
		}
	})
}

func TestCrossPlatformCoverageAttendanceIdentityAndInputHelpers(t *testing.T) {
	if value, ok := attendanceNestedValue(map[string]any{"outer": map[string]any{"id": int32(7)}}, "outer", "id"); !ok || value != int32(7) {
		t.Fatalf("nested identity=%#v ok=%t", value, ok)
	}
	for _, path := range [][]string{{"missing"}, {"outer", "id"}} {
		item := map[string]any{"outer": "not-an-object"}
		if _, ok := attendanceNestedValue(item, path...); ok {
			t.Fatalf("invalid nested path accepted: %#v", path)
		}
	}

	positive := []any{int(1), int32(2), int64(3), float64(4), json.Number("5")}
	for _, value := range positive {
		if _, ok := attendancePositiveInteger(value); !ok {
			t.Errorf("positive identity rejected: %#v", value)
		}
	}
	invalid := []any{int(0), int32(-1), int64(0), 1.5, json.Number("bad"), "1", nil}
	for _, value := range invalid {
		if _, ok := attendancePositiveInteger(value); ok {
			t.Errorf("invalid identity accepted: %#v", value)
		}
	}
	if err := attendanceValidatePositiveIntegerIDs([]map[string]any{{"nested": map[string]any{"id": int32(1)}}}, "attendance/test", "nested", "id"); err != nil {
		t.Fatal(err)
	}
	for _, items := range [][]map[string]any{
		{{}},
		{{"id": 0}},
		{{"id": 1}, {"id": 1}},
	} {
		if err := attendanceValidatePositiveIntegerIDs(items, "attendance/test", "id"); err == nil {
			t.Fatalf("invalid identity collection accepted: %#v", items)
		}
	}
	if err := attendanceValidateExpectedStrings([]map[string]any{{"code": " A "}, {"code": "B"}}, "attendance/test", "code", ""); err != nil {
		t.Fatal(err)
	}
	for _, items := range [][]map[string]any{
		{{}},
		{{"code": 1}},
		{{"code": " "}},
		{{"code": "A"}, {"code": "A"}},
	} {
		if err := attendanceValidateExpectedStrings(items, "attendance/test", "code", ""); err == nil {
			t.Fatalf("invalid string identity collection accepted: %#v", items)
		}
	}

	userCases := []struct {
		values  []string
		maximum int
		valid   bool
	}{
		{[]string{"u1"}, 1, true},
		{nil, 1, false},
		{[]string{"u1", "u2"}, 1, false},
		{[]string{""}, 1, false},
		{[]string{" u1"}, 1, false},
		{[]string{"u1", "u1"}, 2, false},
	}
	for _, tc := range userCases {
		err := attendanceValidateUserIDs(tc.values, tc.maximum)
		if (err == nil) != tc.valid {
			t.Errorf("users=%#v maximum=%d err=%v", tc.values, tc.maximum, err)
		}
	}

	for _, tc := range []struct {
		start string
		end   string
		valid bool
	}{
		{"2026-01-01", "2026-02-01", true},
		{"bad", "2026-01-01", false},
		{"2026-01-01", "bad", false},
		{"2026-02-01", "2026-01-01", false},
		{"2026-01-01", "2026-02-02", false},
	} {
		err := attendanceValidateMonthRange(tc.start, tc.end)
		if (err == nil) != tc.valid {
			t.Errorf("month range %q..%q err=%v", tc.start, tc.end, err)
		}
	}
}

func TestCrossPlatformCoverageAttendancePaginationEvidenceBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string]any
		page      int
		limit     int
		itemCount int
		complete  bool
		valid     bool
	}{
		{"invalid request", map[string]any{"success": true, "result": map[string]any{}}, 0, 10, 0, false, false},
		{"malformed result", map[string]any{"success": true, "result": []any{}}, 1, 10, 0, false, false},
		{"current page type", map[string]any{"success": true, "result": map[string]any{"currentPage": "1", "totalPage": 1}}, 1, 10, 0, false, false},
		{"current page mismatch", map[string]any{"success": true, "result": map[string]any{"currentPage": 2, "totalPage": 1}}, 1, 10, 0, false, false},
		{"total page type", map[string]any{"success": true, "result": map[string]any{"totalPage": "1"}}, 1, 10, 0, false, false},
		{"negative total page", map[string]any{"success": true, "result": map[string]any{"totalPage": -1}}, 1, 10, 0, false, false},
		{"zero pages invalid current page", map[string]any{"success": true, "result": map[string]any{"totalPage": 0}}, 2, 10, 0, false, false},
		{"page out of range", map[string]any{"success": true, "result": map[string]any{"totalPage": 1}}, 2, 10, 0, false, false},
		{"total count type", map[string]any{"success": true, "result": map[string]any{"totalCount": "1"}}, 1, 10, 0, false, false},
		{"negative total count", map[string]any{"success": true, "result": map[string]any{"totalCount": -1}}, 1, 10, 0, false, false},
		{"empty with nonzero total", map[string]any{"success": true, "result": map[string]any{"totalCount": 1}}, 1, 10, 0, false, false},
		{"count before page", map[string]any{"success": true, "result": map[string]any{"totalCount": 5}}, 2, 10, 0, false, false},
		{"items exceed total", map[string]any{"success": true, "result": map[string]any{"totalCount": 5}}, 1, 10, 6, false, false},
		{"missing evidence", map[string]any{"success": true, "result": map[string]any{}}, 1, 10, 0, false, false},
		{"conflicting evidence", map[string]any{"success": true, "result": map[string]any{"totalPage": 2, "totalCount": 5}}, 1, 10, 5, false, false},
		{"total mismatch", map[string]any{"success": true, "result": map[string]any{"totalPage": 3, "totalCount": 15}}, 1, 10, 10, false, false},
		{"no progress", map[string]any{"success": true, "result": map[string]any{"totalPage": 2}}, 1, 10, 0, false, false},
		{"short page", map[string]any{"success": true, "result": map[string]any{"totalPage": 2}}, 1, 10, 5, false, false},
		{"terminal count mismatch", map[string]any{"success": true, "result": map[string]any{"totalCount": 15}}, 2, 10, 4, false, false},
		{"empty terminal", map[string]any{"success": true, "result": map[string]any{"totalPage": 0, "totalCount": 0}}, 1, 10, 0, true, true},
		{"full nonterminal", map[string]any{"success": true, "result": map[string]any{"currentPage": 1, "totalPage": 2, "totalCount": 20}}, 1, 10, 10, false, true},
		{"short terminal", map[string]any{"success": true, "result": map[string]any{"currentPage": 2, "totalPage": 2, "totalCount": 15}}, 2, 10, 5, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			complete, extra, err := attendancePageEvidence(tc.data, "attendance/test", tc.page, tc.limit, tc.itemCount)
			if (err == nil) != tc.valid {
				t.Fatalf("err=%v extra=%#v", err, extra)
			}
			if tc.valid && complete != tc.complete {
				t.Fatalf("complete=%t want=%t extra=%#v", complete, tc.complete, extra)
			}
			if tc.valid && !tc.complete && extra["nextPage"] != tc.page+1 {
				t.Fatalf("nonterminal extra=%#v", extra)
			}
		})
	}

	for _, tc := range []struct {
		page  string
		limit string
		valid bool
	}{
		{"1", "200", true},
		{"0", "20", false},
		{"1", "0", false},
		{"1", "201", false},
	} {
		rt := attendanceRuntimeForTest(t, SearchClass, map[string]string{"page": tc.page, "limit": tc.limit})
		page, limit, err := attendancePageInput(rt)
		if (err == nil) != tc.valid {
			t.Errorf("page=%s limit=%s got=(%d,%d) err=%v", tc.page, tc.limit, page, limit, err)
		}
		if err2 := attendanceValidatePageRequest(rt); (err2 == nil) != tc.valid {
			t.Errorf("validator page=%s limit=%s err=%v", tc.page, tc.limit, err2)
		}
	}

	for _, tc := range []struct {
		value any
		want  int
		valid bool
	}{
		{int(1), 1, true}, {int32(2), 2, true}, {int64(3), 3, true}, {float64(4), 4, true},
		{float64(1.5), 0, false}, {"1", 0, false},
	} {
		got, ok := attendanceInt(tc.value)
		if got != tc.want || ok != tc.valid {
			t.Errorf("attendanceInt(%#v)=(%d,%t), want (%d,%t)", tc.value, got, ok, tc.want, tc.valid)
		}
	}
}

func attendanceResponseJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCrossPlatformCoverageAttendanceChangedValidators(t *testing.T) {
	tests := []struct {
		name        string
		declaration shortcut.Shortcut
		values      map[string]string
		valid       bool
	}{
		{"check result valid", CheckResult, map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31", "limit": "1", "offset": "0"}, true},
		{"check record valid", CheckRecord, map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31"}, true},
		{"approve empty users", ListApprove, map[string]string{"types": "leave", "start": "2026-01-01", "end": "2026-01-31"}, false},
		{"approve empty types", ListApprove, map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31"}, false},
		{"approve invalid start", ListApprove, map[string]string{"users": "u1", "types": "leave", "start": "bad", "end": "2026-01-31"}, false},
		{"approve invalid end", ListApprove, map[string]string{"users": "u1", "types": "leave", "start": "2026-01-01", "end": "bad"}, false},
		{"approve duplicate mapped type", ListApprove, map[string]string{"users": "u1", "types": "leave,请假", "start": "2026-01-01", "end": "2026-01-31"}, false},
		{"approve invalid type", ListApprove, map[string]string{"users": "u1", "types": "invalid", "start": "2026-01-01", "end": "2026-01-31"}, false},
		{"approve valid", ListApprove, map[string]string{"users": "u1", "types": "leave", "start": "2026-01-01", "end": "2026-01-31"}, true},
		{"schedule invalid start", GetSchedule, map[string]string{"users": "u1", "start": "bad", "end": "2026-01-31"}, false},
		{"schedule invalid end", GetSchedule, map[string]string{"users": "u1", "start": "2026-01-01", "end": "bad"}, false},
		{"schedule reverse", GetSchedule, map[string]string{"users": "u1", "start": "2026-02-01", "end": "2026-01-31"}, false},
		{"schedule valid", GetSchedule, map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31"}, true},
		{"class valid", GetClass, map[string]string{"class-id": "1"}, true},
		{"overtime valid", GetOvertimeRule, map[string]string{"overtime-id": "1"}, true},
		{"self setting empty user", GetSelfSetting, map[string]string{"user": "", "setting-scene": "checkRemind"}, false},
		{"self setting valid", GetSelfSetting, map[string]string{"user": "u1", "setting-scene": "checkRemind"}, true},
		{"leave records empty user", GetLeaveRecords, map[string]string{"user": "", "start": "2026-01-01", "end": "2026-01-31"}, false},
		{"leave records invalid start", GetLeaveRecords, map[string]string{"user": "u1", "start": "bad", "end": "2026-01-31"}, false},
		{"leave records invalid end", GetLeaveRecords, map[string]string{"user": "u1", "start": "2026-01-01", "end": "bad"}, false},
		{"leave records valid", GetLeaveRecords, map[string]string{"user": "u1", "start": "2026-01-01", "end": "2026-01-31"}, true},
		{"checkin missing operator", GetCheckinRecord, map[string]string{"operator-corp-id": "", "operator-staff-id": "op", "staff-ids": "u1", "start": "2026-01-01 00:00:00", "end": "2026-01-02 00:00:00"}, false},
		{"checkin invalid start", GetCheckinRecord, map[string]string{"operator-corp-id": "corp", "operator-staff-id": "op", "staff-ids": "u1", "start": "bad", "end": "2026-01-02 00:00:00"}, false},
		{"checkin invalid end", GetCheckinRecord, map[string]string{"operator-corp-id": "corp", "operator-staff-id": "op", "staff-ids": "u1", "start": "2026-01-01 00:00:00", "end": "bad"}, false},
		{"checkin reverse", GetCheckinRecord, map[string]string{"operator-corp-id": "corp", "operator-staff-id": "op", "staff-ids": "u1", "start": "2026-01-02 00:00:00", "end": "2026-01-01 00:00:00"}, false},
		{"checkin valid", GetCheckinRecord, map[string]string{"operator-corp-id": "corp", "operator-staff-id": "op", "staff-ids": "u1", "start": "2026-01-01 00:00:00", "end": "2026-01-02 00:00:00"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := attendanceRuntimeForTest(t, tc.declaration, tc.values)
			if tc.declaration.Validate == nil {
				t.Fatalf("%s has no validator", tc.declaration.Command)
			}
			err := tc.declaration.Validate(rt)
			if (err == nil) != tc.valid {
				t.Fatalf("err=%v wantValid=%t", err, tc.valid)
			}
		})
	}
}

func TestCrossPlatformCoverageAttendanceExactRecordExecutors(t *testing.T) {
	startMillis, _ := dayMillis("2026-01-01")
	endStartMillis, _ := dayMillis("2026-01-31")
	endMillis, _ := dateToMillis("2026-01-31", true)
	nextDayStartMillis, _ := dayMillis("2026-02-01")
	finalDayNoon := endStartMillis + 12*60*60*1000
	validRecord := map[string]any{"id": 1, "userId": "u1", "workDate": startMillis}
	if endMillis != nextDayStartMillis-1 {
		t.Fatalf("end-of-day=%d want=%d", endMillis, nextDayStartMillis-1)
	}
	explicitEnd, err := dateToMillis("2026-01-31 12:34:56", true)
	explicitWant, _ := flexMillis("2026-01-31 12:34:56")
	if err != nil || explicitEnd != explicitWant {
		t.Fatalf("explicit datetime end=%d want=%d err=%v", explicitEnd, explicitWant, err)
	}

	t.Run("check result bad execute bounds", func(t *testing.T) {
		for _, values := range []map[string]string{
			{"users": "u1", "start": "2026-01-01", "end": "2026-01-31", "limit": "0"},
			{"users": "u1", "start": "2026-01-01", "end": "2026-01-31", "offset": "-1"},
		} {
			caller, err := executeAttendanceResponse(t, CheckResult, values, `{"success":true,"result":[]}`)
			if err == nil || caller.calls != 0 {
				t.Fatalf("values=%#v err=%v calls=%d", values, err, caller.calls)
			}
		}
	})

	t.Run("check result downstream and response failures", func(t *testing.T) {
		values := map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31", "limit": "2"}
		responses := []string{
			`{"success":true,"result":null}`,
			attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{map[string]any{"id": 0, "userId": "u1", "workDate": startMillis}}}),
			attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{map[string]any{"id": 1, "userId": "other", "workDate": startMillis}}}),
		}
		for _, response := range responses {
			caller, err := executeAttendanceResponse(t, CheckResult, values, response)
			if err == nil || caller.calls != 1 {
				t.Fatalf("response=%s err=%v calls=%d", response, err, caller.calls)
			}
		}
		caller := &attendanceCoverageCaller{err: fmt.Errorf("backend unavailable")}
		helpers.InitDepsForTest(t, caller)
		if err := CheckResult.Execute(attendanceRuntimeForTest(t, CheckResult, values)); err == nil || caller.calls != 1 {
			t.Fatalf("downstream err=%v calls=%d", err, caller.calls)
		}
	})

	t.Run("check result short and full pages", func(t *testing.T) {
		shortValues := map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31", "limit": "2", "offset": "0"}
		caller, err := executeAttendanceResponse(t, CheckResult, shortValues, attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{validRecord}}))
		if err != nil || caller.calls != 1 {
			t.Fatalf("short err=%v calls=%d", err, caller.calls)
		}
		fullValues := map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31", "limit": "1", "offset": "4"}
		caller, err = executeAttendanceResponse(t, CheckResult, fullValues, attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{validRecord}}))
		if err != nil || caller.calls != 1 {
			t.Fatalf("full err=%v calls=%d", err, caller.calls)
		}
	})

	t.Run("end date includes the full business day", func(t *testing.T) {
		checkValues := map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31", "limit": "2"}
		checkRow := map[string]any{"id": 1, "userId": "u1", "workDate": finalDayNoon}
		caller, err := executeAttendanceResponse(t, CheckResult, checkValues, attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{checkRow}}))
		if err != nil || caller.calls != 1 {
			t.Fatalf("check result rejected final-day midday row: err=%v calls=%d", err, caller.calls)
		}

		approveValues := map[string]string{"users": "u1", "types": "leave", "start": "2026-01-01", "end": "2026-01-31"}
		approval := map[string]any{"id": 1, "userId": "u1", "bizType": 3, "beginTime": finalDayNoon, "endTime": finalDayNoon + 60*60*1000}
		caller, err = executeAttendanceResponse(t, ListApprove, approveValues, attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{approval}}))
		if err != nil || caller.calls != 1 {
			t.Fatalf("approval rejected final-day midday interval: err=%v calls=%d", err, caller.calls)
		}

		checkRow["workDate"] = endMillis
		caller, err = executeAttendanceResponse(t, CheckResult, checkValues, attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{checkRow}}))
		if err != nil || caller.calls != 1 {
			t.Fatalf("check result rejected final millisecond: err=%v calls=%d", err, caller.calls)
		}
		checkRow["workDate"] = nextDayStartMillis
		if caller, err = executeAttendanceResponse(t, CheckResult, checkValues, attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{checkRow}})); err == nil || caller.calls != 1 {
			t.Fatalf("check result accepted next-day row: err=%v calls=%d", err, caller.calls)
		}

		approval["beginTime"] = nextDayStartMillis
		approval["endTime"] = nextDayStartMillis + 1000
		caller, err = executeAttendanceResponse(t, ListApprove, approveValues, attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{approval}}))
		typed, ok := err.(*apperrors.Error)
		reason := ""
		if ok {
			reason = typed.Reason
		}
		if err == nil || caller.calls != 1 || !ok || typed.Reason != "request_range_mismatch" {
			t.Fatalf("approval next-day result err=%v calls=%d reason=%q", err, caller.calls, reason)
		}
	})

	collectionCases := []struct {
		name        string
		declaration shortcut.Shortcut
		values      map[string]string
		valid       map[string]any
		invalid     map[string]any
	}{
		{"check record", CheckRecord, map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31"}, map[string]any{"id": 1, "userId": "u1", "workDate": startMillis - 24*60*60*1000, "userCheckTime": startMillis}, map[string]any{"id": 1, "userId": "other", "userCheckTime": startMillis}},
		{"approve", ListApprove, map[string]string{"users": "u1", "types": "leave", "start": "2026-01-01", "end": "2026-01-31"}, map[string]any{"id": 1, "userId": "u1", "bizType": 3, "beginTime": startMillis, "endTime": endMillis}, map[string]any{"id": 1, "userId": "u1", "bizType": 4, "beginTime": startMillis, "endTime": endMillis}},
		{"schedule", GetSchedule, map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31"}, validRecord, map[string]any{"id": 1, "userId": "other", "workDate": startMillis}},
	}
	t.Run("schedule null responses fail closed", func(t *testing.T) {
		values := map[string]string{"users": "u1", "start": "2026-01-01", "end": "2026-01-31"}
		for _, tc := range []struct {
			name     string
			response string
			reason   string
		}{
			{name: "literal null", response: `null`, reason: "empty_tool_response"},
			{name: "null collection", response: `{"success":true,"result":null}`, reason: "malformed_collection"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				caller, err := executeAttendanceResponse(t, GetSchedule, values, tc.response)
				typed, ok := err.(*apperrors.Error)
				actualReason := ""
				if ok {
					actualReason = typed.Reason
				}
				if err == nil || caller.calls != 1 || !ok || actualReason != tc.reason {
					t.Fatalf("err=%v calls=%d reason=%q want=%q", err, caller.calls, actualReason, tc.reason)
				}
			})
		}
	})
	for _, tc := range collectionCases {
		t.Run(tc.name+" valid", func(t *testing.T) {
			caller, err := executeAttendanceResponse(t, tc.declaration, tc.values, attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{tc.valid}}))
			if err != nil || caller.calls != 1 {
				t.Fatalf("err=%v calls=%d", err, caller.calls)
			}
		})
		t.Run(tc.name+" invalid binding", func(t *testing.T) {
			caller, err := executeAttendanceResponse(t, tc.declaration, tc.values, attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{tc.invalid}}))
			if err == nil || caller.calls != 1 {
				t.Fatalf("err=%v calls=%d", err, caller.calls)
			}
		})
		t.Run(tc.name+" invalid identity", func(t *testing.T) {
			invalidIdentity := make(map[string]any, len(tc.valid))
			for key, value := range tc.valid {
				invalidIdentity[key] = value
			}
			invalidIdentity["id"] = 0
			caller, err := executeAttendanceResponse(t, tc.declaration, tc.values, attendanceResponseJSON(t, map[string]any{"success": true, "result": []any{invalidIdentity}}))
			if err == nil || caller.calls != 1 {
				t.Fatalf("err=%v calls=%d", err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAttendanceSearchProjectorsFailClosed(t *testing.T) {
	classValid := map[string]any{"success": true, "result": map[string]any{"items": []any{map[string]any{"shiftVO": map[string]any{"id": 1, "name": "n", "ownerName": "o"}}}}}
	classes, err := searchClassProject(classValid)
	if err != nil || len(classes) != 1 || classes[0]["classId"] != int64(1) || classes[0]["name"] != "n" || classes[0]["ownerName"] != "o" {
		t.Fatalf("class projection=%#v err=%v", classes, err)
	}
	for _, data := range []map[string]any{
		{"success": true, "result": map[string]any{"items": []any{map[string]any{"shiftVO": "bad"}}}},
		{"success": true, "result": map[string]any{"items": []any{map[string]any{"name": "missing id"}}}},
		{"success": true, "result": map[string]any{"items": []any{map[string]any{"id": 1}, map[string]any{"classId": 1}}}},
	} {
		if _, err := searchClassProject(data); err == nil {
			t.Fatalf("invalid class projection accepted: %#v", data)
		}
	}

	ruleValid := map[string]any{"success": true, "result": map[string]any{"items": []any{map[string]any{"entityVO": map[string]any{"id": 2, "name": "rule"}}}}}
	rules, err := searchRuleProject(ruleValid, "attendance/test", "result.items")
	if err != nil || len(rules) != 1 || rules[0]["ruleId"] != int64(2) || rules[0]["name"] != "rule" {
		t.Fatalf("rule projection=%#v err=%v", rules, err)
	}
	for _, data := range []map[string]any{
		{"success": true, "result": map[string]any{"items": []any{map[string]any{"entityVO": "bad"}}}},
		{"success": true, "result": map[string]any{"items": []any{map[string]any{"name": "missing id"}}}},
		{"success": true, "result": map[string]any{"items": []any{map[string]any{"id": 1}, map[string]any{"ruleId": 1}}}},
	} {
		if _, err := searchRuleProject(data, "attendance/test", "result.items"); err == nil {
			t.Fatalf("invalid rule projection accepted: %#v", data)
		}
	}

	groupValid := map[string]any{"success": true, "result": map[string]any{"items": []any{map[string]any{"group_id": "g1", "group_name": "group", "group_type": "FIXED"}}}}
	groups, err := searchGroupProject(groupValid)
	if err != nil || len(groups) != 1 || groups[0]["groupId"] != "g1" || groups[0]["name"] != "group" || groups[0]["type"] != "FIXED" {
		t.Fatalf("group projection=%#v err=%v", groups, err)
	}
	if _, err := searchGroupProject(map[string]any{"success": true, "result": map[string]any{"items": []any{map[string]any{"name": "missing id"}}}}); err == nil {
		t.Fatal("group without identity was accepted")
	}
}

func TestCrossPlatformCoverageAttendanceSearchExecutorsPagination(t *testing.T) {
	type searchCase struct {
		name        string
		declaration shortcut.Shortcut
		tool        string
		resultKey   string
		item        map[string]any
	}
	tests := []searchCase{
		{"class", SearchClass, "get_class_list", "items", map[string]any{"shiftVO": map[string]any{"id": 1}}},
		{"adjustment", SearchAdjustmentRule, "get_adjustment_rule", "adjustmentList", map[string]any{"entityVO": map[string]any{"id": 1}}},
		{"overtime", SearchOvertimeRule, "get_overtime_rule", "atRuleList", map[string]any{"entityVO": map[string]any{"id": 1}}},
	}
	for _, tc := range tests {
		t.Run(tc.name+" invalid page", func(t *testing.T) {
			caller, err := executeAttendanceResponse(t, tc.declaration, map[string]string{"page": "0", "limit": "1"}, `{"success":true}`)
			if err == nil || caller.calls != 0 {
				t.Fatalf("err=%v calls=%d", err, caller.calls)
			}
		})
		t.Run(tc.name+" malformed collection", func(t *testing.T) {
			caller, err := executeAttendanceResponse(t, tc.declaration, map[string]string{"query": "q", "page": "1", "limit": "1"}, `{"success":true,"result":{}}`)
			if err == nil || caller.calls != 1 {
				t.Fatalf("err=%v calls=%d", err, caller.calls)
			}
		})
		t.Run(tc.name+" missing pagination evidence", func(t *testing.T) {
			result := map[string]any{tc.resultKey: []any{tc.item}}
			caller, err := executeAttendanceResponse(t, tc.declaration, map[string]string{"page": "1", "limit": "1"}, attendanceResponseJSON(t, map[string]any{"success": true, "result": result}))
			if err == nil || caller.calls != 1 {
				t.Fatalf("err=%v calls=%d", err, caller.calls)
			}
		})
		for _, page := range []struct {
			name      string
			totalPage int
			total     int
		}{
			{"terminal", 1, 1},
			{"nonterminal", 2, 2},
		} {
			t.Run(tc.name+" "+page.name, func(t *testing.T) {
				result := map[string]any{tc.resultKey: []any{tc.item}, "currentPage": 1, "totalPage": page.totalPage, "totalCount": page.total}
				caller, err := executeAttendanceResponse(t, tc.declaration, map[string]string{"query": "q", "page": "1", "limit": "1"}, attendanceResponseJSON(t, map[string]any{"success": true, "result": result}))
				if err != nil || caller.calls != 1 {
					t.Fatalf("err=%v calls=%d", err, caller.calls)
				}
				if caller.tools[0] != tc.tool {
					t.Fatalf("tool=%q want=%q", caller.tools[0], tc.tool)
				}
			})
		}
	}

	t.Run("group invalid page", func(t *testing.T) {
		caller, err := executeAttendanceResponse(t, SearchGroup, map[string]string{"page": "0", "limit": "1"}, `{"success":true}`)
		if err == nil || caller.calls != 0 {
			t.Fatalf("err=%v calls=%d", err, caller.calls)
		}
	})
	t.Run("group malformed and pagination failure", func(t *testing.T) {
		for _, response := range []string{
			`{"success":true,"result":{}}`,
			`{"success":true,"result":{"items":[{"id":"g1"}]}}`,
		} {
			caller, err := executeAttendanceResponse(t, SearchGroup, map[string]string{"query": "q", "type": "FIXED", "query-position": "true", "query-ble": "false", "page": "1", "limit": "1"}, response)
			if err == nil || caller.calls != 1 {
				t.Fatalf("response=%s err=%v calls=%d", response, err, caller.calls)
			}
		}
	})
	t.Run("group terminal", func(t *testing.T) {
		response := `{"success":true,"result":{"items":[{"id":"g1","name":"group","type":"FIXED"}],"currentPage":1,"totalPage":1,"totalCount":1}}`
		caller, err := executeAttendanceResponse(t, SearchGroup, map[string]string{"page": "1", "limit": "1"}, response)
		if err != nil || caller.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, caller.calls)
		}
	})
}

func TestCrossPlatformCoverageAttendanceRemainingChangedExecutors(t *testing.T) {
	t.Run("approve template exact type", func(t *testing.T) {
		for _, tc := range []struct {
			value    string
			response string
			valid    bool
			calls    int
		}{
			{"invalid", `{"success":true,"result":[]}`, false, 0},
			{"leave", `{"success":true,"result":[{"processCode":"p1","approveType":"OVERTIME","submitUrl":"https://example.invalid/1"}]}`, false, 1},
			{"leave", `{"success":true,"result":[{"processCode":"p1","approveType":"LEAVE","submitUrl":"https://example.invalid/1"}]}`, true, 1},
			{"TRAVEL", `{"success":true,"result":[{"processCode":"p1","approveType":"TRAVEL","submitUrl":"https://example.invalid/1"},{"processCode":"p2","approveType":"TRAVEL","submitUrl":"https://example.invalid/2"}]}`, true, 1},
			{"OUT", `{"success":true,"result":[{"processCode":"p1","approveType":"OUT","submitUrl":"https://example.invalid/1"},{"processCode":"p2","approveType":"OUT","submitUrl":"https://example.invalid/2"}]}`, true, 1},
		} {
			caller, err := executeAttendanceResponse(t, GetApproveTemplate, map[string]string{"type": tc.value}, tc.response)
			if (err == nil) != tc.valid || caller.calls != tc.calls {
				t.Errorf("type=%q err=%v calls=%d wantValid=%t wantCalls=%d", tc.value, err, caller.calls, tc.valid, tc.calls)
			}
		}
	})

	t.Run("hidden adjustment detail still fails closed", func(t *testing.T) {
		caller, err := executeAttendanceResponse(t, GetAdjustmentRule, map[string]string{"adjustment-id": "7"}, `{"success":true,"result":{"id":7}}`)
		if err != nil || caller.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, caller.calls)
		}
	})

	t.Run("summary object", func(t *testing.T) {
		caller, err := executeAttendanceResponse(t, GetSummary, map[string]string{"user": "u1", "date": "2026-01-01", "stats-type": "week"}, `{"success":true,"result":{"days":1}}`)
		if err != nil || caller.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, caller.calls)
		}
	})

	t.Run("self setting helper rejects invalid request and scene", func(t *testing.T) {
		if err := attendanceValidateSelfSetting(map[string]any{"userId": "u1", "checkRemindSetting": map[string]any{}}, " ", "checkRemind"); err == nil {
			t.Fatal("empty requested user accepted")
		}
		if err := attendanceValidateSelfSetting(map[string]any{"userId": "u1"}, "u1", "not-a-scene"); err == nil {
			t.Fatal("invalid setting scene accepted")
		}
	})

	t.Run("report value", func(t *testing.T) {
		caller, err := executeAttendanceResponse(t, QueryReportData, map[string]string{"users": "u1", "columns": "1, 2", "start": "2026-01-01 00:00:00", "end": "2026-01-02 00:00:00"}, `{"success":true,"result":false}`)
		if err != nil || caller.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, caller.calls)
		}
	})

	t.Run("leave type exact collection", func(t *testing.T) {
		caller, err := executeAttendanceResponse(t, ListLeaveTypes, nil, `{"success":true,"result":[{"leaveCode":"code"}]}`)
		if err != nil || caller.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, caller.calls)
		}
	})

	t.Run("leave records optional code", func(t *testing.T) {
		values := map[string]string{"user": "u1", "leave-code": "code", "start": "2026-01-01", "end": "2026-01-31"}
		caller, err := executeAttendanceResponse(t, GetLeaveRecords, values, `{"success":true,"result":[]}`)
		if err != nil || caller.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, caller.calls)
		}
	})

	t.Run("checkin record nested collection", func(t *testing.T) {
		values := map[string]string{"operator-corp-id": "corp", "operator-staff-id": "op", "staff-ids": "u1", "start": "2026-01-01 00:00:00", "end": "2026-01-02 00:00:00"}
		caller, err := executeAttendanceResponse(t, GetCheckinRecord, values, `{"success":true,"result":{"list":[]}}`)
		if err != nil || caller.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, caller.calls)
		}
	})
}

var _ edition.ToolCaller = (*attendanceCoverageCaller)(nil)
