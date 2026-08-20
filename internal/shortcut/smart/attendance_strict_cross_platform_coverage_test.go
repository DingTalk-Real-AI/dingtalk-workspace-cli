// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/responsecheck"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageSmartAttendanceEmptyOnlyLeavesAreUnavailable(t *testing.T) {
	for _, declaration := range []struct {
		name         string
		rollout      output.RolloutState
		hasResult    bool
		availability string
	}{
		{MyAttendance.Command, MyAttendance.OutputRollout, MyAttendance.Contract.Result != nil, MyAttendance.Contract.Interface.Availability},
		{ThisMonthAttendance.Command, ThisMonthAttendance.OutputRollout, ThisMonthAttendance.Contract.Result != nil, ThisMonthAttendance.Contract.Interface.Availability},
	} {
		if declaration.rollout != output.RolloutLegacyOnly {
			t.Errorf("%s rollout = %q, want legacy_only while unavailable", declaration.name, declaration.rollout)
		}
		if declaration.hasResult || declaration.availability != contract.InterfaceAvailable {
			t.Errorf("%s must omit Result and preserve its Schema-compatible non-public Interface", declaration.name)
		}
	}
}

func TestCrossPlatformCoverageSmartAttendanceProfileFailsClosed(t *testing.T) {
	valid := map[string]any{"success": true, "result": []any{map[string]any{"orgEmployeeModel": map[string]any{"userId": "u1"}}}}
	if got := strictAttendanceCurrentUserID(valid); got != "u1" {
		t.Fatalf("valid profile identity = %q", got)
	}
	invalid := []map[string]any{
		nil,
		{"result": map[string]any{"userId": "stale"}},
		{"success": false, "result": map[string]any{"userId": "stale"}},
		{"success": true, "result": nil},
		{"success": true, "result": []any{"malformed", map[string]any{"userId": "u1"}}},
		{"success": true, "result": []any{map[string]any{"userId": "u1"}, map[string]any{"userId": "u2"}}},
		{"success": true, "result": map[string]any{"userId": "u1", "orgEmployeeModel": map[string]any{"userId": "u2"}}},
	}
	for index, profile := range invalid {
		if got := strictAttendanceCurrentUserID(profile); got != "" {
			t.Errorf("invalid profile %d resolved %q", index, got)
		}
	}
}

type smartAttendanceProfileCaller struct {
	profile       string
	attendance    string
	profileErr    error
	attendanceErr error
	calls         []string
	arguments     []map[string]any
}

func (c *smartAttendanceProfileCaller) CallTool(_ context.Context, server, tool string, arguments map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, server+"/"+tool)
	c.arguments = append(c.arguments, arguments)
	text := c.profile
	callErr := c.profileErr
	if len(c.calls) > 1 {
		text = c.attendance
		callErr = c.attendanceErr
	}
	if callErr != nil {
		return nil, callErr
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}
func (*smartAttendanceProfileCaller) Format() string { return "json" }
func (*smartAttendanceProfileCaller) DryRun() bool   { return false }
func (*smartAttendanceProfileCaller) Fields() string { return "" }
func (*smartAttendanceProfileCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageSmartAttendanceInvalidProfileMakesZeroAttendanceCalls(t *testing.T) {
	caller := &smartAttendanceProfileCaller{}
	helpers.InitDepsForTest(t, caller)
	for _, declaration := range []*shortcut.Shortcut{&MyAttendance, &ThisMonthAttendance} {
		caller.calls = nil
		caller.arguments = nil
		caller.profile = `{"success":false,"result":{"userId":"stale"}}`
		rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: declaration.Command}, *declaration)
		if err := declaration.Execute(rt); err == nil {
			t.Errorf("%s accepted success=false profile", declaration.Command)
		}
		if len(caller.calls) != 1 || !strings.HasSuffix(caller.calls[0], "/get_current_user_profile") {
			t.Errorf("%s calls = %#v, want profile call only", declaration.Command, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageSmartAttendanceStrictResolverShapes(t *testing.T) {
	valid := []map[string]any{
		{"success": true, "result": map[string]any{"userId": " u1 "}},
		{"success": true, "result": map[string]any{"orgEmployeeModel": map[string]any{"userId": "u1"}}},
		{"success": true, "result": map[string]any{"userId": "u1", "orgEmployeeModel": map[string]any{"userId": "u1"}}},
		{"success": true, "result": []any{map[string]any{"userId": "u1"}, map[string]any{"orgEmployeeModel": map[string]any{"userId": "u1"}}}},
	}
	for index, profile := range valid {
		if got := strictAttendanceCurrentUserID(profile); got != "u1" {
			t.Errorf("valid profile %d resolved %q", index, got)
		}
	}
	invalid := []map[string]any{
		{"success": true, "result": []any{}},
		{"success": true, "result": map[string]any{}},
		{"success": true, "result": map[string]any{"userId": 1}},
		{"success": true, "result": map[string]any{"userId": " "}},
		{"success": true, "result": map[string]any{"orgEmployeeModel": "bad"}},
		{"success": true, "result": map[string]any{"orgEmployeeModel": map[string]any{"userId": 1}}},
		{"success": true, "result": map[string]any{"orgEmployeeModel": map[string]any{"userId": " "}}},
		{"success": true, "result": true},
	}
	for index, profile := range invalid {
		if got := strictAttendanceCurrentUserID(profile); got != "" {
			t.Errorf("invalid profile %d resolved %q", index, got)
		}
	}
}

func TestCrossPlatformCoverageSmartAttendanceRecordIdentityFailsClosed(t *testing.T) {
	for _, data := range []map[string]any{
		{"success": true, "result": []any{map[string]any{"id": 0}}},
		{"success": true, "result": []any{map[string]any{"id": ""}}},
		{"success": true, "result": []any{map[string]any{"id": 1}, map[string]any{"id": 1}}},
	} {
		if records, err := responsecheck.RequireObjectCollection(data, "attendance/test", "result"); err == nil {
			seen := map[int64]struct{}{}
			valid := true
			for _, record := range records {
				identity, ok := smartAttendancePositiveInteger(record["id"])
				if !ok {
					valid = false
					break
				}
				if _, duplicate := seen[identity]; duplicate {
					valid = false
					break
				}
				seen[identity] = struct{}{}
			}
			if valid {
				t.Fatalf("invalid records accepted: %#v", data)
			}
		}
	}
}

func TestCrossPlatformCoverageSmartAttendanceRecordOutputContract(t *testing.T) {
	result := attendanceRecordsResult()
	if result == nil || len(result.Outcomes) != 2 || !json.Valid(result.DataSchema) {
		t.Fatalf("invalid strict attendance result declaration: %#v", result)
	}

	for _, value := range []any{int(1), int32(2), int64(3), float64(4), json.Number("5")} {
		if _, ok := smartAttendancePositiveInteger(value); !ok {
			t.Errorf("valid record identity rejected: %#v", value)
		}
	}
	for _, value := range []any{int(0), int32(-1), int64(0), float64(1.5), json.Number("bad"), "1", nil} {
		if _, ok := smartAttendancePositiveInteger(value); ok {
			t.Errorf("invalid record identity accepted: %#v", value)
		}
	}

	rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: "attendance-test"}, shortcut.Shortcut{})
	invalid := []map[string]any{
		nil,
		{"success": false, "result": []any{}},
		{"success": true, "result": nil},
		{"success": true, "result": []any{"bad"}},
		{"success": true, "result": []any{map[string]any{"id": 0}}},
		{"success": true, "result": []any{map[string]any{"id": 1}, map[string]any{"id": 1}}},
	}
	for index, data := range invalid {
		if err := outputStrictAttendanceRecords(rt, data); err == nil {
			t.Errorf("invalid strict output %d accepted", index)
		}
	}
	if err := outputStrictAttendanceRecords(rt, map[string]any{"success": true, "result": []any{map[string]any{"id": int64(1)}}}); err != nil {
		t.Fatalf("valid strict output rejected: %v", err)
	}
}

func TestCrossPlatformCoverageSmartAttendanceExecuteSuccessAndFailures(t *testing.T) {
	validProfile := `{"success":true,"result":{"userId":"u1"}}`
	validAttendance := `{"success":true,"result":[{"id":1}]}`
	for _, declaration := range []*shortcut.Shortcut{&MyAttendance, &ThisMonthAttendance} {
		t.Run(declaration.Command+" success", func(t *testing.T) {
			caller := &smartAttendanceProfileCaller{profile: validProfile, attendance: validAttendance}
			helpers.InitDepsForTest(t, caller)
			rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: declaration.Command}, *declaration)
			if err := declaration.Execute(rt); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 2 || !strings.HasSuffix(caller.calls[1], "/query_check_record") {
				t.Fatalf("calls=%#v", caller.calls)
			}
			request, ok := caller.arguments[1]["QueryCheckRecordRequest"].(map[string]any)
			users, usersOK := request["userIds"].([]string)
			if !ok || !usersOK || len(users) != 1 || users[0] != "u1" || strings.TrimSpace(fmt.Sprint(request["checkDateFrom"])) == "" || strings.TrimSpace(fmt.Sprint(request["checkDateTo"])) == "" {
				t.Fatalf("request=%#v", caller.arguments[1])
			}
		})
		t.Run(declaration.Command+" profile call error", func(t *testing.T) {
			caller := &smartAttendanceProfileCaller{profileErr: fmt.Errorf("profile unavailable")}
			helpers.InitDepsForTest(t, caller)
			rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: declaration.Command}, *declaration)
			if err := declaration.Execute(rt); err == nil || len(caller.calls) != 1 {
				t.Fatalf("err=%v calls=%#v", err, caller.calls)
			}
		})
		t.Run(declaration.Command+" attendance call error", func(t *testing.T) {
			caller := &smartAttendanceProfileCaller{profile: validProfile, attendanceErr: fmt.Errorf("attendance unavailable")}
			helpers.InitDepsForTest(t, caller)
			rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: declaration.Command}, *declaration)
			if err := declaration.Execute(rt); err == nil || len(caller.calls) != 2 {
				t.Fatalf("err=%v calls=%#v", err, caller.calls)
			}
		})
		t.Run(declaration.Command+" malformed attendance", func(t *testing.T) {
			caller := &smartAttendanceProfileCaller{profile: validProfile, attendance: `{"success":true,"result":null}`}
			helpers.InitDepsForTest(t, caller)
			rt := shortcut.RuntimeContextForTest(&cobra.Command{Use: declaration.Command}, *declaration)
			if err := declaration.Execute(rt); err == nil || len(caller.calls) != 2 {
				t.Fatalf("err=%v calls=%#v", err, caller.calls)
			}
		})
	}
}
