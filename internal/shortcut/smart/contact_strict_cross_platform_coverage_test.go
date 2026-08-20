// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type contactSmartStrictCall struct {
	tool string
	args map[string]any
}

type contactSmartStrictCaller struct {
	calls      []contactSmartStrictCall
	searchZero bool
	payloads   map[string]string
	errors     map[string]error
}

func (caller *contactSmartStrictCaller) CallTool(_ context.Context, _, tool string, args map[string]any) (*edition.ToolResult, error) {
	caller.calls = append(caller.calls, contactSmartStrictCall{tool: tool, args: args})
	if err := caller.errors[tool]; err != nil {
		return nil, err
	}
	if payload, ok := caller.payloads[tool]; ok {
		return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: payload}}}, nil
	}
	payload := `{"success":true,"result":[]}`
	switch tool {
	case "search_contact_by_key_word":
		if !caller.searchZero {
			payload = `{"success":true,"result":[{"userId":"stable-user","openDingTalkId":"stable-open"}]}`
		}
	case "get_user_info_by_user_ids":
		payload = `{"success":true,"result":[{"orgEmployeeModel":{"orgUserId":"stable-user"}}]}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: payload}}}, nil
}

func (*contactSmartStrictCaller) Format() string { return "json" }
func (*contactSmartStrictCaller) DryRun() bool   { return false }
func (*contactSmartStrictCaller) Fields() string { return "" }
func (*contactSmartStrictCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageContactSmartStrictDecoders(t *testing.T) {
	profile, err := strictUserDetail(map[string]any{
		"success": true,
		"result":  []any{map[string]any{"orgEmployeeModel": map[string]any{"orgUserId": "stable-user"}}},
	}, "stable-user", "contact/detail")
	if err != nil || profile["orgUserId"] != "stable-user" {
		t.Fatalf("valid detail rejected: profile=%v err=%v", profile, err)
	}
	for _, data := range []map[string]any{
		{"success": true, "result": []any{}},
		{"success": true, "result": []any{map[string]any{}}},
		{"success": true, "result": []any{map[string]any{"orgEmployeeModel": map[string]any{"orgUserId": "other"}}}},
		{"success": true, "result": []any{map[string]any{"orgEmployeeModel": map[string]any{"orgUserId": "stable-user"}}, map[string]any{"orgEmployeeModel": map[string]any{"orgUserId": "stable-user"}}}},
	} {
		if got, decodeErr := strictUserDetail(data, "stable-user", "contact/detail"); decodeErr == nil {
			t.Errorf("broken detail returned success: %v", got)
		}
	}
	if _, err := strictContactMembers(map[string]any{
		"success": true,
		"deptUserList": []any{
			map[string]any{"userInfo": map[string]any{"userId": "same"}},
			map[string]any{"userInfo": map[string]any{"userId": "same"}},
		},
	}, "contact/members"); err == nil {
		t.Fatal("duplicate smart member identity returned success")
	}
	if _, err := strictContactMembers(map[string]any{
		"success":      true,
		"deptUserList": []any{map[string]any{"userInfo": map[string]any{"userId": "stable", "name": true}}},
	}, "contact/members"); err == nil {
		t.Fatal("malformed optional smart member name returned success")
	}
	department, err := strictDeptDetail(map[string]any{
		"success": true, "result": map[string]any{"deptId": "7", "deptName": "Fixture"},
	}, 7, "contact/dept")
	if err != nil || department["deptId"] != int64(7) {
		t.Fatalf("department ID was not normalized: department=%v err=%v", department, err)
	}
	if _, err := strictPrimaryDeptID(map[string]any{"depts": []any{
		map[string]any{"deptId": float64(1)}, map[string]any{"deptId": float64(1)},
	}}, "contact/detail"); err == nil {
		t.Fatal("duplicate profile department identity returned success")
	}
	if _, err := strictPrimaryDeptID(map[string]any{"depts": []any{
		map[string]any{"deptId": float64(1)}, map[string]any{"deptName": "missing-id"},
	}}, "contact/detail"); err == nil {
		t.Fatal("malformed later profile department was silently ignored")
	}
}

func TestCrossPlatformCoverageContactSmartContracts(t *testing.T) {
	items := []*shortcut.Shortcut{&ByMobile, &DeptMembers, &Lookup, &Org, &ResolveDept, &Team, &Whoami}
	for _, item := range items {
		if item.OutputRollout != output.RolloutUnifiedActive || item.Contract.Result == nil || len(item.Contract.Result.SensitivePaths) == 0 || strings.TrimSpace(item.Safety.Effect) == "" {
			t.Errorf("%s lacks Contract/Result/Safety/unified output", item.Command)
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(item.Contract.Result.DataSchema, &schema); err != nil || schema["type"] != "object" {
			t.Errorf("%s invalid Result schema: schema=%v err=%v", item.Command, schema, err)
		}
	}
	if Whoami.Validate != nil {
		t.Fatal("parameterless +me must not publish an empty runtime validator")
	}
}

func TestCrossPlatformCoverageContactSmartBlankInputsFailBeforeRemoteCall(t *testing.T) {
	caller := &contactSmartStrictCaller{}
	helpers.InitDepsForTest(t, caller)
	tests := []struct {
		declaration shortcut.Shortcut
		flag        string
	}{
		{ByMobile, "mobile"},
		{DeptMembers, "dept"},
		{Lookup, "name"},
		{Org, "name"},
		{ResolveDept, "name"},
		{Team, "name"},
	}
	for _, test := range tests {
		command := &cobra.Command{Use: test.declaration.Command}
		command.Flags().String(test.flag, "", "")
		if err := command.Flags().Set(test.flag, "   "); err != nil {
			t.Fatal(err)
		}
		if err := test.declaration.Validate(shortcut.RuntimeContextForTest(command, test.declaration)); err == nil {
			t.Errorf("%s accepted blank --%s", test.declaration.Command, test.flag)
		}
	}
	if len(caller.calls) != 0 {
		t.Fatalf("blank smart inputs made remote calls: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageByMobileUsesExplicitArraySearchBeforeDetail(t *testing.T) {
	caller := &contactSmartStrictCaller{}
	helpers.InitDepsForTest(t, caller)
	command := &cobra.Command{Use: "+by-mobile"}
	command.Flags().String("mobile", "", "")
	if err := command.Flags().Set("mobile", "fixture-mobile"); err != nil {
		t.Fatal(err)
	}
	declaration := ByMobile
	declaration.OutputRollout = output.RolloutLegacyOnly
	if err := declaration.Execute(shortcut.RuntimeContextForTest(command, declaration)); err != nil {
		t.Fatalf("by-mobile known execution: %v", err)
	}
	if len(caller.calls) != 2 || caller.calls[0].tool != "search_contact_by_key_word" || caller.calls[0].args["keyword"] != "fixture-mobile" || caller.calls[1].tool != "get_user_info_by_user_ids" {
		t.Fatalf("by-mobile calls = %#v", caller.calls)
	}

	caller.calls = nil
	caller.searchZero = true
	if err := declaration.Execute(shortcut.RuntimeContextForTest(command, declaration)); err == nil {
		t.Fatal("explicit zero unexpectedly became a successful detail")
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "search_contact_by_key_word" {
		t.Fatalf("zero-match calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageContactSmartPrimitiveMatrices(t *testing.T) {
	for index, test := range []struct {
		value any
		want  bool
	}{
		{nil, false}, {"", false}, {"0", false}, {"SUCCESS", false}, {"bad", true},
		{float64(0), false}, {float64(1), true}, {0, false}, {1, true}, {false, false}, {true, true},
		{map[string]any{}, false}, {map[string]any{"x": 1}, true}, {[]any{}, false}, {[]any{1}, true}, {struct{}{}, true},
	} {
		if got := strictContactFailure(test.value); got != test.want {
			t.Errorf("failure matrix %d = %v, want %v", index, got, test.want)
		}
	}
	for index, test := range []struct {
		value any
		want  int64
		ok    bool
	}{
		{float64(7), 7, true}, {float64(7.5), 0, false}, {int64(8), 8, true}, {9, 9, true},
		{json.Number("10"), 10, true}, {json.Number("bad"), 0, false}, {" 11 ", 11, true}, {"bad", 0, false}, {true, 0, false},
	} {
		got, ok := strictContactInt64(test.value)
		if got != test.want || ok != test.ok {
			t.Errorf("integer matrix %d = (%d,%v), want (%d,%v)", index, got, ok, test.want, test.ok)
		}
	}
	if value, present, valid := strictContactOptionalString(map[string]any{"x": nil}, "x"); value != "" || !present || !valid {
		t.Fatalf("nil optional string = (%q,%v,%v)", value, present, valid)
	}
	if value, present, valid := strictContactOptionalString(map[string]any{"x": true}, "x"); value != "" || !present || valid {
		t.Fatalf("bad optional string = (%q,%v,%v)", value, present, valid)
	}
	if got := strictContactFirst(map[string]any{"a": " ", "b": " value "}, "a", "b"); got != "value" {
		t.Fatalf("first string = %q", got)
	}
}

func TestCrossPlatformCoverageContactSmartStrictHelperBranches(t *testing.T) {
	dummy := shortcut.Shortcut{Command: "+dummy", Description: "Dummy", Flags: []shortcut.Flag{{Name: "name", Type: shortcut.FlagString, Required: true}}, Validate: func(*shortcut.RuntimeContext) error { return errors.New("prior") }}
	if result := contactSmartResult(&dummy); result == nil || len(result.DataSchema) == 0 {
		t.Fatal("default smart result missing")
	}
	finalizeContactSmart(&dummy)
	dummyCommand := &cobra.Command{Use: "+dummy"}
	dummyCommand.Flags().String("name", "value", "")
	if err := dummy.Validate(shortcut.RuntimeContextForTest(dummyCommand, dummy)); err == nil {
		t.Fatal("previous validator failure was discarded")
	}
	for _, data := range []map[string]any{
		{}, {"success": false}, {"success": true, "errorCode": "FAILED"},
	} {
		if _, err := strictContactEnvelope(data, "contact/test"); err == nil {
			t.Errorf("bad envelope returned success: %#v", data)
		}
	}
	caller := &contactSmartStrictCaller{payloads: map[string]string{}, errors: map[string]error{}}
	helpers.InitDepsForTest(t, caller)
	resolverCommand := &cobra.Command{Use: "+lookup"}
	resolverCommand.Flags().String("name", "fixture", "")
	resolverRuntime := shortcut.RuntimeContextForTest(resolverCommand, Lookup)
	for _, payload := range []string{
		`{"success":true}`,
		`{"success":true,"result":[{"name":"missing-id"}]}`,
		`{"success":true,"result":[{"userId":"same"},{"userId":"same"}]}`,
		`{"success":true,"result":[{"userId":"one"},{"userId":"two"}]}`,
	} {
		caller.payloads["search_contact_by_key_word"] = payload
		if _, err := strictResolveContactUser(resolverRuntime, "fixture"); err == nil {
			t.Errorf("bad resolver payload returned success: %s", payload)
		}
	}
	for _, data := range []map[string]any{
		{"success": true},
		{"success": true, "deptList": []any{}},
		{"success": true, "deptList": []any{map[string]any{"deptName": "missing-id"}}},
		{"success": true, "deptList": []any{map[string]any{"deptId": 1, "deptName": ""}}},
		{"success": true, "deptList": []any{map[string]any{"deptId": 1, "deptName": "One"}, map[string]any{"deptId": 1, "deptName": "Two"}}},
	} {
		got, err := strictDeptCandidates(data, "contact/dept")
		if list, ok := data["deptList"].([]any); ok && len(list) == 0 {
			if err != nil || len(got) != 0 {
				t.Errorf("explicit empty departments rejected: got=%#v err=%v", got, err)
			}
		} else if err == nil {
			t.Errorf("bad department candidates returned success: %#v", data)
		}
	}
	candidates, err := strictDeptCandidates(map[string]any{"success": true, "deptList": []any{map[string]any{"deptId": "7", "deptName": "<red>Fixture</red>"}}}, "contact/dept")
	if err != nil || len(candidates) != 1 || candidates[0].id != 7 || candidates[0].name != "Fixture" {
		t.Fatalf("valid candidates = %#v err=%v", candidates, err)
	}
	for _, data := range []map[string]any{
		{"success": false},
		{"success": true},
		{"success": true, "deptUserList": []any{map[string]any{"x": 1}}},
		{"success": true, "deptUserList": []any{map[string]any{"userInfo": map[string]any{"name": "no-id"}}}},
	} {
		if _, err := strictContactMembers(data, "contact/members"); err == nil {
			t.Errorf("bad members returned success: %#v", data)
		}
	}
	members, err := strictContactMembers(map[string]any{"success": true, "deptUserList": []any{map[string]any{"userInfo": map[string]any{"userId": "u1", "name": "Fixture"}}}}, "contact/members")
	if err != nil || len(members) != 1 {
		t.Fatalf("valid members=%#v err=%v", members, err)
	}
	for _, data := range []map[string]any{
		{"success": false},
		{"success": true, "result": []any{}},
		{"success": true, "result": map[string]any{"deptId": 0}},
		{"success": true, "result": map[string]any{"deptId": 8}},
		{"success": true, "result": map[string]any{"deptId": 7, "deptName": true}},
	} {
		if _, err := strictDeptDetail(data, 7, "contact/dept"); err == nil {
			t.Errorf("bad detail returned success: %#v", data)
		}
	}
	for _, data := range []map[string]any{
		{"success": true, "result": []any{map[string]any{"x": 1}}},
		{"success": true, "result": []any{map[string]any{"orgEmployeeModel": map[string]any{"name": "no-id"}}}},
	} {
		if _, err := strictUserDetail(data, "u1", "contact/detail"); err == nil {
			t.Errorf("bad user detail returned success: %#v", data)
		}
	}
	for _, model := range []map[string]any{
		{}, {"depts": []any{}}, {"depts": []any{"bad"}}, {"depts": []any{map[string]any{}}},
	} {
		if _, err := strictPrimaryDeptID(model, "contact/detail"); err == nil {
			t.Errorf("bad primary department returned success: %#v", model)
		}
	}
	if id, err := strictPrimaryDeptID(map[string]any{"depts": []any{map[string]any{"deptId": 7}, map[string]any{"deptId": 8}}}, "contact/detail"); err != nil || id != 7 {
		t.Fatalf("primary dept=(%d,%v)", id, err)
	}
	who, err := strictWhoami(map[string]any{"success": true, "result": []any{map[string]any{"orgEmployeeModel": map[string]any{
		"userId": "u1", "orgUserName": "Fixture", "orgUserMobile": "m", "orgAuthEmail": "e", "orgName": "o", "depts": []any{map[string]any{"deptName": "d"}},
	}}}})
	if err != nil || who["userId"] != "u1" || who["dept"] != "d" {
		t.Fatalf("whoami=%#v err=%v", who, err)
	}
}

func TestCrossPlatformCoverageContactSmartExecutors(t *testing.T) {
	basePayloads := map[string]string{
		"search_contact_by_key_word": `{"success":true,"result":[{"userId":"u1","openDingTalkId":"o1"}]}`,
		"get_user_info_by_user_ids":  `{"success":true,"result":[{"orgEmployeeModel":{"orgUserId":"u1","depts":[{"deptId":7}]}}]}`,
		"search_dept_by_keyword":     `{"success":true,"deptList":[{"deptId":7,"deptName":"Fixture"}]}`,
		"get_dept_members_by_deptId": `{"success":true,"deptUserList":[{"userInfo":{"userId":"u1","name":"Fixture"}}]}`,
		"get_dept_info_by_dept_id":   `{"success":true,"result":{"deptId":7,"deptName":"Fixture"}}`,
		"get_current_user_profile":   `{"success":true,"result":[{"orgEmployeeModel":{"userId":"u1","orgUserName":"Fixture"}}]}`,
	}
	tests := []struct {
		declaration shortcut.Shortcut
		flag        string
		value       string
	}{
		{ByMobile, "mobile", "fixture"}, {DeptMembers, "dept", "fixture"}, {Lookup, "name", "fixture"},
		{Org, "name", "fixture"}, {ResolveDept, "name", "fixture"}, {Team, "name", "fixture"}, {Whoami, "", ""},
	}
	for _, test := range tests {
		t.Run(test.declaration.Command, func(t *testing.T) {
			caller := &contactSmartStrictCaller{payloads: basePayloads, errors: map[string]error{}}
			helpers.InitDepsForTest(t, caller)
			declaration := test.declaration
			declaration.OutputRollout = output.RolloutLegacyOnly
			command := &cobra.Command{Use: declaration.Command}
			if test.flag != "" {
				command.Flags().String(test.flag, "", "")
				if err := command.Flags().Set(test.flag, test.value); err != nil {
					t.Fatal(err)
				}
			}
			rt := shortcut.RuntimeContextForTest(command, declaration)
			if err := declaration.Execute(rt); err != nil {
				t.Fatalf("success execution: %v; calls=%#v", err, caller.calls)
			}
			caller.calls = nil
			firstTool := map[string]string{
				"+dept-members": "search_dept_by_keyword", "+resolve-dept": "search_dept_by_keyword", "+me": "get_current_user_profile",
			}[declaration.Command]
			if firstTool == "" {
				firstTool = "search_contact_by_key_word"
			}
			caller.errors[firstTool] = errors.New("transport")
			if err := declaration.Execute(rt); err == nil {
				t.Fatal("first transport error returned success")
			}
		})
	}

	for _, declaration := range []shortcut.Shortcut{ByMobile, Lookup, Org, Team} {
		caller := &contactSmartStrictCaller{payloads: basePayloads, errors: map[string]error{"get_user_info_by_user_ids": errors.New("transport")}}
		helpers.InitDepsForTest(t, caller)
		copy := declaration
		copy.OutputRollout = output.RolloutLegacyOnly
		command := &cobra.Command{Use: copy.Command}
		flag := "name"
		if copy.Command == "+by-mobile" {
			flag = "mobile"
		}
		command.Flags().String(flag, "", "")
		if err := command.Flags().Set(flag, "fixture"); err != nil {
			t.Fatal(err)
		}
		if err := copy.Execute(shortcut.RuntimeContextForTest(command, copy)); err == nil {
			t.Errorf("%s second transport error returned success", copy.Command)
		}
	}
	for _, declaration := range []shortcut.Shortcut{DeptMembers, Team} {
		caller := &contactSmartStrictCaller{payloads: basePayloads, errors: map[string]error{"get_dept_members_by_deptId": errors.New("transport")}}
		helpers.InitDepsForTest(t, caller)
		copy := declaration
		copy.OutputRollout = output.RolloutLegacyOnly
		command := &cobra.Command{Use: copy.Command}
		flag := "name"
		if copy.Command == "+dept-members" {
			flag = "dept"
		}
		command.Flags().String(flag, "", "")
		if err := command.Flags().Set(flag, "fixture"); err != nil {
			t.Fatal(err)
		}
		if err := copy.Execute(shortcut.RuntimeContextForTest(command, copy)); err == nil {
			t.Errorf("%s final transport error returned success", copy.Command)
		}
	}

	runFailure := func(t *testing.T, declaration shortcut.Shortcut, flag string, payloads map[string]string, errorsByTool map[string]error) {
		t.Helper()
		caller := &contactSmartStrictCaller{payloads: payloads, errors: errorsByTool}
		helpers.InitDepsForTest(t, caller)
		declaration.OutputRollout = output.RolloutLegacyOnly
		command := &cobra.Command{Use: declaration.Command}
		if flag != "" {
			command.Flags().String(flag, "", "")
			if err := command.Flags().Set(flag, "fixture"); err != nil {
				t.Fatal(err)
			}
		}
		if err := declaration.Execute(shortcut.RuntimeContextForTest(command, declaration)); err == nil {
			t.Fatalf("%s malformed downstream response returned success", declaration.Command)
		}
	}
	badProfile := map[string]string{
		"search_contact_by_key_word": `{"success":true,"result":[{"userId":"u1"}]}`,
		"get_user_info_by_user_ids":  `{"success":true,"result":[{"x":1}]}`,
	}
	runFailure(t, ByMobile, "mobile", badProfile, map[string]error{})
	runFailure(t, Lookup, "name", badProfile, map[string]error{})
	badMembers := map[string]string{
		"search_dept_by_keyword":     `{"success":true,"deptList":[{"deptId":7,"deptName":"Fixture"}]}`,
		"get_dept_members_by_deptId": `{"success":true}`,
	}
	runFailure(t, DeptMembers, "dept", badMembers, map[string]error{})
	badOrgProfile := map[string]string{
		"search_contact_by_key_word": `{"success":true,"result":[{"userId":"u1"}]}`,
		"get_user_info_by_user_ids":  `{"success":true,"result":[{"x":1}]}`,
	}
	runFailure(t, Org, "name", badOrgProfile, map[string]error{})
	runFailure(t, Team, "name", badOrgProfile, map[string]error{})
	missingDept := map[string]string{
		"search_contact_by_key_word": `{"success":true,"result":[{"userId":"u1"}]}`,
		"get_user_info_by_user_ids":  `{"success":true,"result":[{"orgEmployeeModel":{"orgUserId":"u1"}}]}`,
	}
	runFailure(t, Org, "name", missingDept, map[string]error{})
	runFailure(t, Team, "name", missingDept, map[string]error{})
	validOrg := map[string]string{
		"search_contact_by_key_word": `{"success":true,"result":[{"userId":"u1"}]}`,
		"get_user_info_by_user_ids":  `{"success":true,"result":[{"orgEmployeeModel":{"orgUserId":"u1","depts":[{"deptId":7}]}}]}`,
		"get_dept_info_by_dept_id":   `{"success":true}`,
	}
	runFailure(t, Org, "name", validOrg, map[string]error{})
	runFailure(t, Org, "name", validOrg, map[string]error{"get_dept_info_by_dept_id": errors.New("transport")})
	validTeam := map[string]string{
		"search_contact_by_key_word": `{"success":true,"result":[{"userId":"u1"}]}`,
		"get_user_info_by_user_ids":  `{"success":true,"result":[{"orgEmployeeModel":{"orgUserId":"u1","depts":[{"deptId":7}]}}]}`,
		"get_dept_members_by_deptId": `{"success":true}`,
	}
	runFailure(t, Team, "name", validTeam, map[string]error{})
}
