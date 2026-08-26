// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package contact

import (
	"bytes"
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

type contactCaller struct {
	payload  string
	payloads map[string]string
	err      error
	errors   map[string]error
	calls    int
	product  string
	tool     string
	args     map[string]any
	history  []contactCall
}

type contactCall struct {
	tool string
	args map[string]any
}

func (caller *contactCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	caller.calls++
	caller.product, caller.tool, caller.args = product, tool, args
	caller.history = append(caller.history, contactCall{tool: tool, args: args})
	if caller.err != nil {
		return nil, caller.err
	}
	if err := caller.errors[tool]; err != nil {
		return nil, err
	}
	payload := caller.payload
	if toolPayload, ok := caller.payloads[tool]; ok {
		payload = toolPayload
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: payload}}}, nil
}
func (*contactCaller) Format() string { return "json" }
func (*contactCaller) DryRun() bool   { return false }
func (*contactCaller) Fields() string { return "" }
func (*contactCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageContactStrictSearchAndCollections(t *testing.T) {
	users, err := strictUserSearch(map[string]any{
		"success": true, "result": []any{map[string]any{"userId": "stable-user", "name": "Fixture"}},
	}, "contact/test", false)
	if err != nil || len(users) != 1 {
		t.Fatalf("valid users rejected: users=%v err=%v", users, err)
	}
	users, err = strictUserSearch(map[string]any{"success": true, "result": []any{}}, "contact/test", false)
	if err != nil || len(users) != 0 {
		t.Fatalf("explicit zero rejected: users=%v err=%v", users, err)
	}

	broken := []map[string]any{
		{},
		{"result": []any{}},
		{"success": "true", "result": []any{}},
		{"success": false, "result": []any{}},
		{"success": true},
		{"success": true, "result": map[string]any{}},
		{"success": true, "result": []any{"bad"}},
		{"success": true, "result": []any{map[string]any{}}},
		{"success": true, "result": []any{map[string]any{"name": "no-id"}}},
		{"success": true, "result": []any{map[string]any{"userId": "same"}, map[string]any{"userId": "same"}}},
		{"success": true, "result": []any{map[string]any{"userId": "good", "name": float64(1)}}},
		{"success": true, "errorCode": "FAILED", "result": []any{}},
	}
	for index, data := range broken {
		if got, projectErr := strictUserSearch(data, "contact/test", false); projectErr == nil {
			t.Errorf("broken response %d returned success: %v", index, got)
		}
	}

	if got, projectErr := strictMembers(map[string]any{
		"success": true, "deptUserList": []any{map[string]any{"userInfo": map[string]any{"userId": "stable-user"}}},
	}, "contact/members", "deptUserList"); projectErr != nil || len(got) != 1 {
		t.Fatalf("valid members rejected: got=%v err=%v", got, projectErr)
	}
	for _, data := range []map[string]any{
		{"success": false},
		{"success": true},
		{"success": true, "deptUserList": map[string]any{}},
		{"success": true, "deptUserList": []any{map[string]any{"unexpected": true}}},
		{"success": true, "deptUserList": []any{map[string]any{"userInfo": map[string]any{"name": "no-id"}}}},
		{"success": true, "deptUserList": []any{map[string]any{"userInfo": map[string]any{"userId": "same"}}, map[string]any{"userInfo": map[string]any{"userId": "same"}}}},
		{"success": true, "deptUserList": []any{map[string]any{"userInfo": map[string]any{"userId": "good", "name": true}}}},
	} {
		if got, projectErr := strictMembers(data, "contact/members", "deptUserList"); projectErr == nil {
			t.Errorf("broken members returned success: %v", got)
		}
	}
}

func TestCrossPlatformCoverageContactSubDepartmentsRejectBadIdentityAndShape(t *testing.T) {
	depts, err := strictSubDepts(map[string]any{
		"success": true,
		"result":  []any{map[string]any{"deptId": float64(1), "deptName": "Fixture"}},
	}, "contact/sub-depts")
	if err != nil || len(depts) != 1 || depts[0]["deptId"] != int64(1) {
		t.Fatalf("valid sub departments rejected: depts=%v err=%v", depts, err)
	}
	for _, broken := range []map[string]any{
		{"success": true},
		{"success": true, "result": []any{map[string]any{"deptId": float64(0)}}},
		{"success": true, "result": []any{map[string]any{"deptId": float64(1)}, map[string]any{"deptId": float64(1)}}},
		{"success": true, "result": []any{map[string]any{"deptId": float64(1), "deptName": map[string]any{}}}},
	} {
		if got, projectErr := strictSubDepts(broken, "contact/sub-depts"); projectErr == nil {
			t.Errorf("broken sub departments returned success: %v", got)
		}
	}
}

func TestCrossPlatformCoverageContactSearchMobileUsesReviewedObjectShape(t *testing.T) {
	caller := &contactCaller{payloads: map[string]string{
		"search_user_by_mobile": `{"success":true,"result":{"userId":"stable-user","orgUserName":"Fixture"}}`,
	}, errors: map[string]error{"get_user_info_by_user_ids": errors.New("detail permission unavailable")}}
	helpers.InitDepsForTest(t, caller)
	cmd := &cobra.Command{Use: "+search-mobile"}
	cmd.Flags().String("mobile", "", "")
	if err := cmd.Flags().Set("mobile", "+86 138-0013-8000"); err != nil {
		t.Fatal(err)
	}
	declaration := SearchMobile
	declaration.OutputRollout = output.RolloutLegacyOnly
	if err := declaration.Validate(shortcut.RuntimeContextForTest(cmd, declaration)); err != nil {
		t.Fatalf("formatted mobile validation: %v", err)
	}
	if err := declaration.Execute(shortcut.RuntimeContextForTest(cmd, declaration)); err != nil {
		t.Fatalf("search-mobile: %v", err)
	}
	if caller.calls != 1 || caller.product != "contact" || caller.history[0].tool != "search_user_by_mobile" || caller.history[0].args["mobile"] != "8613800138000" {
		t.Fatalf("mapping = calls:%d product:%q history:%#v", caller.calls, caller.product, caller.history)
	}
	caller.errors = nil

	caller.calls, caller.history = 0, nil
	caller.errors = map[string]error{"search_user_by_mobile": errors.New("lookup unavailable")}
	if err := declaration.Execute(shortcut.RuntimeContextForTest(cmd, declaration)); err == nil {
		t.Fatal("search-mobile swallowed the exact lookup transport error")
	}
	if caller.calls != 1 || caller.history[0].args["mobile"] != "8613800138000" {
		t.Fatalf("transport failure mapping = calls:%d history:%#v", caller.calls, caller.history)
	}
	caller.errors = nil

	caller.calls, caller.history = 0, nil
	caller.payloads["search_user_by_mobile"] = `{"success":true}`
	if err := declaration.Execute(shortcut.RuntimeContextForTest(cmd, declaration)); err != nil {
		t.Fatalf("reviewed exact-mobile zero rejected: %v", err)
	}
	if caller.calls != 1 || caller.history[0].tool != "search_user_by_mobile" {
		t.Fatalf("zero match must stop after exact lookup: %#v", caller.history)
	}
	caller.payloads["search_user_by_mobile"] = `{"success":true,"result":null}`
	if err := declaration.Execute(shortcut.RuntimeContextForTest(cmd, declaration)); err == nil {
		t.Fatal("null result must not become a successful empty search")
	}

	caller.payloads["search_user_by_mobile"] = `{"success":true,"result":{"openDingTalkId":"open-only"}}`
	caller.calls, caller.history = 0, nil
	if err := declaration.Execute(shortcut.RuntimeContextForTest(cmd, declaration)); err == nil {
		t.Fatal("mobile candidate without userId returned success")
	}
	if caller.calls != 1 {
		t.Fatalf("missing userId calls=%d history=%#v", caller.calls, caller.history)
	}
	invalidCommand := &cobra.Command{Use: "+search-mobile"}
	invalidCommand.Flags().String("mobile", "", "")
	if err := invalidCommand.Flags().Set("mobile", "not-a-phone"); err != nil {
		t.Fatal(err)
	}
	caller.calls, caller.history = 0, nil
	if err := declaration.Execute(shortcut.RuntimeContextForTest(invalidCommand, declaration)); err == nil {
		t.Fatal("invalid mobile reached execution")
	}
	if caller.calls != 0 {
		t.Fatalf("invalid mobile made calls: %#v", caller.history)
	}
}

func TestCrossPlatformCoverageContactMobileProofHelperMatrix(t *testing.T) {
	user, found, err := strictMobileLookup(map[string]any{
		"success": true, "result": map[string]any{"userId": "stable-user", "orgUserName": "Fixture"},
	}, "contact/mobile")
	if err != nil || !found || user["userId"] != "stable-user" || user["name"] != "Fixture" {
		t.Fatalf("exact mobile result rejected: user=%#v found=%v err=%v", user, found, err)
	}
	if user, found, err := strictMobileLookup(map[string]any{"success": true}, "contact/mobile"); err != nil || found || user != nil {
		t.Fatalf("reviewed no-match rejected: user=%#v found=%v err=%v", user, found, err)
	}
	for _, broken := range []map[string]any{
		{"success": false},
		{"success": true, "result": nil},
		{"success": true, "result": []any{}},
		{"success": true, "result": map[string]any{}},
		{"success": true, "result": map[string]any{"orgUserName": "no-id"}},
		{"success": true, "result": map[string]any{"userId": "stable-user", "orgUserName": 7}},
	} {
		if user, found, err := strictMobileLookup(broken, "contact/mobile"); err == nil {
			t.Errorf("broken mobile lookup returned success: user=%#v found=%v data=%#v", user, found, broken)
		}
	}
	for input, want := range map[string]string{
		"+86 138-0013-8000": "8613800138000",
		"008613800138000":   "8613800138000",
		"(138) 0013-8000":   "13800138000",
	} {
		if got, err := normalizeContactMobile(input); err != nil || got != want {
			t.Errorf("normalize %q = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "123", "1+3800138000", "13800x138000"} {
		if got, err := normalizeContactMobile(input); err == nil {
			t.Errorf("invalid mobile %q normalized to %q", input, got)
		}
	}
}

func TestCrossPlatformCoverageContactRequiredSearchInputsFailBeforeRemoteCall(t *testing.T) {
	caller := &contactCaller{payload: `{"success":true,"result":[]}`}
	helpers.InitDepsForTest(t, caller)
	mobileCommand := &cobra.Command{Use: "+search-mobile"}
	mobileCommand.Flags().String("mobile", "", "")
	if err := SearchMobile.Execute(shortcut.RuntimeContextForTest(mobileCommand, SearchMobile)); err == nil {
		t.Fatal("missing mobile unexpectedly reached execution")
	}
	if caller.calls != 0 {
		t.Fatalf("missing mobile made %d remote calls", caller.calls)
	}

	userCommand := &cobra.Command{Use: "+search-user"}
	userCommand.Flags().String("query", "fixture", "")
	declaration := SearchUser
	declaration.OutputRollout = output.RolloutLegacyOnly
	if err := declaration.Execute(shortcut.RuntimeContextForTest(userCommand, declaration)); err != nil {
		t.Fatalf("search-user regressed: %v", err)
	}
	if caller.calls != 1 || caller.args["keyword"] != "fixture" {
		t.Fatalf("search-user mapping = calls:%d args:%#v", caller.calls, caller.args)
	}
}

func TestCrossPlatformCoverageContactInputConstraintsFailBeforeRemoteCall(t *testing.T) {
	caller := &contactCaller{payload: `{"success":true,"result":[]}`}
	helpers.InitDepsForTest(t, caller)
	tests := []struct {
		declaration shortcut.Shortcut
		flag        string
		value       string
	}{
		{SearchUser, "query", "   "},
		{SearchMobile, "mobile", "   "},
		{SearchMobile, "mobile", "not-a-phone"},
		{ListRoleMembers, "id", "0"},
		{ListRoleMembers, "id", "not-a-number"},
		{ListSubDepts, "dept", "0"},
		{ListDeptMembers, "depts", "1,1"},
		{ListDeptMembers, "depts", "1,invalid"},
		{ListFollowings, "open-id", "   "},
	}
	for _, test := range tests {
		command := &cobra.Command{Use: test.declaration.Command}
		switch test.declaration.Command {
		case "+list-sub-depts":
			command.Flags().Int(test.flag, 0, "")
		case "+list-dept-members":
			command.Flags().StringSlice(test.flag, nil, "")
		default:
			command.Flags().String(test.flag, "", "")
		}
		if err := command.Flags().Set(test.flag, test.value); err != nil {
			t.Fatalf("set %s %s: %v", test.declaration.Command, test.flag, err)
		}
		if err := test.declaration.Validate(shortcut.RuntimeContextForTest(command, test.declaration)); err == nil {
			t.Errorf("%s accepted invalid %s=%q", test.declaration.Command, test.flag, test.value)
		}
	}
	if caller.calls != 0 {
		t.Fatalf("invalid inputs made %d remote calls", caller.calls)
	}
}

func TestCrossPlatformCoverageListRolesCompatibilityUsesStrictLegacyMCP(t *testing.T) {
	caller := &contactCaller{payload: `{"success":true,"result":[{"groupName":"Fixture group","labels":[{"labelId":1,"name":"Fixture role"},{"labelId":null,"name":null}]}]}`}
	helpers.InitDepsForTest(t, caller)
	declaration := ListRoles
	declaration.OutputRollout = output.RolloutLegacyOnly
	command := &cobra.Command{Use: declaration.Command}
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	if err := declaration.Execute(shortcut.RuntimeContextForTest(command, declaration)); err != nil {
		t.Fatalf("list roles compatibility call: %v", err)
	}
	if caller.calls != 1 || caller.product != "contact" || caller.tool != "get_org_labels" || len(caller.args) != 0 {
		t.Fatalf("list roles mapping = calls:%d product:%q tool:%q args:%#v", caller.calls, caller.product, caller.tool, caller.args)
	}
	var payload struct {
		Count int              `json:"count"`
		Roles []map[string]any `json:"roles"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode list roles output: %v", err)
	}
	if payload.Count != 2 || len(payload.Roles) != 2 || payload.Roles[0]["labelName"] != "Fixture role" {
		t.Fatalf("legacy role output drift: %#v", payload)
	}
	if value, present := payload.Roles[1]["labelId"]; !present || value != nil || payload.Roles[1]["labelName"] != nil {
		t.Fatalf("reviewed placeholder was not preserved: %#v", payload.Roles[1])
	}

	caller.calls = 0
	caller.payload = `{"success":true,"result":[{"groupName":"Fixture group","labels":[{"labelId":1,"name":"Fixture role"},{"name":"Missing identity"}]}]}`
	if err := declaration.Execute(shortcut.RuntimeContextForTest(&cobra.Command{Use: declaration.Command}, declaration)); err == nil {
		t.Fatal("list roles accepted a malformed role element")
	}
	if caller.calls != 1 {
		t.Fatalf("malformed response calls = %d, want 1", caller.calls)
	}

	caller.calls = 0
	caller.err = errors.New("permission denied")
	if err := declaration.Execute(shortcut.RuntimeContextForTest(&cobra.Command{Use: declaration.Command}, declaration)); err == nil {
		t.Fatal("list roles swallowed downstream error")
	}
	if caller.calls != 1 {
		t.Fatalf("downstream error calls = %d, want 1", caller.calls)
	}
}

func TestCrossPlatformCoverageRosterCompatibilityDelegatesToLegacyMCP(t *testing.T) {
	caller := &contactCaller{payload: `{"success":true,"result":[]}`}
	helpers.InitDepsForTest(t, caller)

	listDeclaration := ListRosterFields
	listDeclaration.OutputRollout = output.RolloutLegacyOnly
	if err := listDeclaration.Execute(shortcut.RuntimeContextForTest(&cobra.Command{Use: listDeclaration.Command}, listDeclaration)); err != nil {
		t.Fatalf("list roster fields compatibility call: %v", err)
	}
	if caller.calls != 1 || caller.product != "hrmregister" || caller.tool != "list_authorized_roster_fields" || len(caller.args) != 0 {
		t.Fatalf("list roster fields mapping = calls:%d product:%q tool:%q args:%#v", caller.calls, caller.product, caller.tool, caller.args)
	}

	caller.calls, caller.history = 0, nil
	getCommand := &cobra.Command{Use: GetRoster.Command}
	getCommand.Flags().String("staff-id", "", "")
	getCommand.Flags().StringSlice("fields", nil, "")
	if err := getCommand.Flags().Set("staff-id", "fixture-staff"); err != nil {
		t.Fatal(err)
	}
	if err := getCommand.Flags().Set("fields", "field-a,field-b"); err != nil {
		t.Fatal(err)
	}
	getDeclaration := GetRoster
	getDeclaration.OutputRollout = output.RolloutLegacyOnly
	if err := getDeclaration.Execute(shortcut.RuntimeContextForTest(getCommand, getDeclaration)); err != nil {
		t.Fatalf("get roster compatibility call: %v", err)
	}
	fields, ok := caller.args["fieldCodeList"].([]string)
	if caller.calls != 1 || caller.product != "hrmregister" || caller.tool != "get_authorized_emp_rosterInfo" || caller.args["staffId"] != "fixture-staff" || !ok || len(fields) != 2 || fields[0] != "field-a" || fields[1] != "field-b" {
		t.Fatalf("get roster mapping = calls:%d product:%q tool:%q args:%#v", caller.calls, caller.product, caller.tool, caller.args)
	}

	caller.err = errors.New("permission denied")
	caller.calls = 0
	if err := listDeclaration.Execute(shortcut.RuntimeContextForTest(&cobra.Command{Use: listDeclaration.Command}, listDeclaration)); err == nil {
		t.Fatal("list roster fields swallowed downstream error")
	}
	if caller.calls != 1 {
		t.Fatalf("downstream error calls = %d, want 1", caller.calls)
	}
}

func TestCrossPlatformCoverageContactFollowingsAndRolesRejectBadElements(t *testing.T) {
	followings, err := strictFollowings(map[string]any{
		"success": true,
		"result": map[string]any{"models": []any{
			map[string]any{"openDingTalkId": "open-1"},
			map[string]any{"openDingTalkId": "open-2"},
		}},
	}, "contact/followings", "open-2")
	if err != nil || len(followings) != 1 || followings[0]["openDingTalkId"] != "open-2" {
		t.Fatalf("strict followings = %#v, err=%v", followings, err)
	}
	followings, err = strictFollowings(map[string]any{
		"success": true,
		"result":  map[string]any{"models": []any{map[string]any{"openDingTalkId": "open-1"}}},
	}, "contact/followings", "guaranteed-missing")
	if err != nil || len(followings) != 0 {
		t.Fatalf("filtered explicit zero = %#v, err=%v", followings, err)
	}
	caller := &contactCaller{payload: `{"success":true,"result":{"models":[{"openDingTalkId":"open-1"}]}}`}
	helpers.InitDepsForTest(t, caller)
	command := &cobra.Command{Use: "+list-followings"}
	command.Flags().String("open-id", "guaranteed-missing", "")
	declaration := ListFollowings
	declaration.OutputRollout = output.RolloutLegacyOnly
	if executeErr := declaration.Execute(shortcut.RuntimeContextForTest(command, declaration)); executeErr != nil {
		t.Fatalf("exact followings mapping: %v", executeErr)
	}
	if caller.calls != 1 || caller.product != "contact" || caller.tool != "list_my_followings" {
		t.Fatalf("followings mapping = calls:%d product:%q tool:%q", caller.calls, caller.product, caller.tool)
	}
	for _, broken := range []map[string]any{
		{"success": false},
		{"success": true},
		{"success": true, "result": map[string]any{}},
		{"success": true, "result": map[string]any{"models": map[string]any{}}},
		{"success": true, "result": map[string]any{"models": []any{"bad"}}},
		{"success": true, "result": map[string]any{"models": []any{map[string]any{}}}},
		{"success": true, "result": map[string]any{"models": []any{map[string]any{"openDingTalkId": "same"}, map[string]any{"openDingTalkId": "same"}}}},
	} {
		if got, parseErr := strictFollowings(broken, "contact/followings", ""); parseErr == nil {
			t.Errorf("broken followings returned success: %#v", got)
		}
	}

	roles, err := strictRoles(map[string]any{
		"success": true,
		"result": []any{map[string]any{
			"groupName": "Fixture group",
			"labels": []any{
				map[string]any{"labelId": float64(1), "name": "Fixture role"},
				map[string]any{"labelId": nil, "name": nil},
			},
		}},
	}, "contact/roles")
	if err != nil || len(roles) != 2 || roles[0]["labelId"] != int64(1) || roles[0]["labelName"] != "Fixture role" || roles[1]["labelId"] != nil || roles[1]["labelName"] != nil {
		t.Fatalf("strict roles = %#v, err=%v", roles, err)
	}
	for _, broken := range []map[string]any{
		{"success": false},
		{"success": true},
		{"success": true, "result": map[string]any{}},
		{"success": true, "result": []any{"bad"}},
		{"success": true, "result": []any{map[string]any{"labels": []any{}}}},
		{"success": true, "result": []any{map[string]any{"groupName": "Fixture"}}},
		{"success": true, "result": []any{map[string]any{"groupName": "Fixture", "labels": map[string]any{}}}},
		{"success": true, "result": []any{map[string]any{"groupName": "Fixture", "labels": []any{"bad"}}}},
		{"success": true, "result": []any{map[string]any{"groupName": "Fixture", "labels": []any{map[string]any{"labelId": nil}}}}},
		{"success": true, "result": []any{map[string]any{"groupName": "Fixture", "labels": []any{map[string]any{"labelId": nil, "name": ""}}}}},
		{"success": true, "result": []any{map[string]any{"groupName": "Fixture", "labels": []any{map[string]any{"labelId": float64(1), "name": "One"}, map[string]any{"labelId": float64(1), "name": "Duplicate"}}}}},
	} {
		if got, parseErr := strictRoles(broken, "contact/roles"); parseErr == nil {
			t.Errorf("broken roles returned success: %#v", got)
		}
	}
	if users, err := projectUsers([]map[string]any{{"openDingTalkId": "open-only"}}, "contact/users"); err != nil || users[0]["openDingTalkId"] != "open-only" {
		t.Fatalf("open-ID-only user rejected: %#v err=%v", users, err)
	}
	if members, err := strictMembers(map[string]any{"success": true, "deptUserList": []any{map[string]any{"userInfo": map[string]any{"userId": "u1", "name": "Fixture"}}}}, "contact/members", "deptUserList"); err != nil || members[0]["name"] != "Fixture" {
		t.Fatalf("named member rejected: %#v err=%v", members, err)
	}
}

func TestCrossPlatformCoverageContactDirectContracts(t *testing.T) {
	available := []*shortcut.Shortcut{
		&ListFollowings, &SearchUser, &SearchMobile, &ListRoleMembers, &ListSubDepts, &ListDeptMembers,
	}
	for _, item := range available {
		if item.OutputRollout != output.RolloutUnifiedActive || item.Contract.Result == nil || len(item.Contract.Result.SensitivePaths) == 0 || strings.TrimSpace(item.Safety.Effect) == "" {
			t.Errorf("%s lacks Contract/Result/Safety/unified output", item.Command)
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(item.Contract.Result.DataSchema, &schema); err != nil || schema["type"] != "object" {
			t.Errorf("%s invalid Result schema: schema=%v err=%v", item.Command, schema, err)
		}
	}
	for item, want := range map[*shortcut.Shortcut]map[string]string{
		&SearchMobile:    {"mobile": "mobile"},
		&ListRoleMembers: {"id": "id"},
		&ListSubDepts:    {"dept": "dept"},
		&ListDeptMembers: {"depts": "depts"},
	} {
		for _, parameter := range item.Contract.Parameters {
			if property, ok := want[parameter.Name]; ok {
				if parameter.Property != property {
					t.Errorf("%s parameter %s property=%q want=%q", item.Command, parameter.Name, parameter.Property, property)
				}
				delete(want, parameter.Name)
			}
		}
		if len(want) != 0 {
			t.Errorf("%s missing compatibility parameters: %#v", item.Command, want)
		}
	}
	if ListRoles.OutputRollout != output.RolloutLegacyOnly || ListRoles.Contract.Result != nil || ListRoles.Contract.Interface == nil || ListRoles.Contract.Interface.Availability != "available" || ListRoles.Contract.Interface.Reason != contactCompatibilityInterfaceReason {
		t.Errorf("+list-roles compatibility delivery drift: rollout=%q result=%v interface=%#v", ListRoles.OutputRollout, ListRoles.Contract.Result, ListRoles.Contract.Interface)
	}
	for _, item := range []*shortcut.Shortcut{&ListRosterFields, &GetRoster} {
		if item.OutputRollout != output.RolloutLegacyOnly || item.Contract.Result != nil || item.Contract.Interface == nil || item.Contract.Interface.Availability != "unavailable" {
			t.Errorf("%s unavailable delivery drift: rollout=%q result=%v interface=%#v", item.Command, item.OutputRollout, item.Contract.Result, item.Contract.Interface)
		}
	}
}

func TestCrossPlatformCoverageContactPrimitiveMatrices(t *testing.T) {
	for index, test := range []struct {
		value any
		want  bool
	}{
		{nil, false}, {"", false}, {"0", false}, {"SUCCESS", false}, {"bad", true},
		{float64(0), false}, {float64(1), true}, {0, false}, {1, true}, {false, false}, {true, true},
		{map[string]any{}, false}, {map[string]any{"x": 1}, true}, {[]any{}, false}, {[]any{1}, true}, {struct{}{}, true},
	} {
		if got := contactFailureValue(test.value); got != test.want {
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
		got, ok := contactInt64(test.value)
		if got != test.want || ok != test.ok {
			t.Errorf("integer matrix %d = (%d,%v), want (%d,%v)", index, got, ok, test.want, test.ok)
		}
	}
	if got := normalizedContactIDList([]string{" 1 ", "2"}); len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("normalized IDs = %#v", got)
	}
	if _, err := strictUserSearch(map[string]any{"success": true}, "contact/test", true); err == nil {
		t.Fatal("missing optional-result evidence returned success")
	}
}

func TestCrossPlatformCoverageContactValidatorsAcceptCanonicalInputs(t *testing.T) {
	command := &cobra.Command{Use: "contact"}
	command.Flags().String("required", " value ", "")
	command.Flags().String("optional", " value ", "")
	command.Flags().String("id", " 7 ", "")
	command.Flags().Int("dept", 7, "")
	command.Flags().StringSlice("depts", []string{" 7 ", "8"}, "")
	rt := shortcut.RuntimeContextForTest(command, SearchUser)
	for _, err := range []error{
		validateContactNonBlank(rt, "contact/test", "required"),
		validateContactOptionalNonBlank(rt, "contact/test", "optional"),
		validateContactPositiveStringID(rt, "contact/test", "id"),
		validateContactPositiveInt(rt, "contact/test", "dept"),
		validateContactPositiveIDList(rt, "contact/test", "depts"),
	} {
		if err != nil {
			t.Fatalf("canonical validation failed: %v", err)
		}
	}
	command.Flags().StringSlice("empty-list", nil, "")
	if err := validateContactPositiveIDList(rt, "contact/test", "empty-list"); err == nil {
		t.Fatal("empty ID list returned success")
	}
}

func TestCrossPlatformCoverageContactDirectExecutors(t *testing.T) {
	tests := []struct {
		declaration shortcut.Shortcut
		flag        string
		value       string
		payload     string
		tool        string
	}{
		{ListFollowings, "open-id", "open-1", `{"success":true,"result":{"models":[{"openDingTalkId":"open-1"}]}}`, "list_my_followings"},
		{SearchUser, "query", "fixture", `{"success":true,"result":[{"userId":"user-1"}]}`, "search_contact_by_key_word"},
		{ListRoleMembers, "id", "7", `{"success":true,"labelUserList":[{"userInfo":{"userId":"user-1"}}]}`, "get_label_members_by_labelId"},
		{ListSubDepts, "dept", "7", `{"success":true,"result":[{"deptId":8}]}`, "get_sub_depts_by_dept_id"},
		{ListDeptMembers, "depts", " 7 ,8", `{"success":true,"deptUserList":[{"userInfo":{"userId":"user-1"}}]}`, "get_dept_members_by_deptId"},
	}
	for _, test := range tests {
		t.Run(test.declaration.Command, func(t *testing.T) {
			caller := &contactCaller{payload: test.payload}
			helpers.InitDepsForTest(t, caller)
			declaration := test.declaration
			declaration.OutputRollout = output.RolloutLegacyOnly
			command := &cobra.Command{Use: declaration.Command}
			switch declaration.Command {
			case "+list-sub-depts":
				command.Flags().Int(test.flag, 0, "")
			case "+list-dept-members":
				command.Flags().StringSlice(test.flag, nil, "")
			default:
				command.Flags().String(test.flag, "", "")
			}
			if err := command.Flags().Set(test.flag, test.value); err != nil {
				t.Fatal(err)
			}
			rt := shortcut.RuntimeContextForTest(command, declaration)
			if err := declaration.Execute(rt); err != nil {
				t.Fatalf("success execution: %v", err)
			}
			if caller.tool != test.tool {
				t.Fatalf("tool=%q want=%q", caller.tool, test.tool)
			}
			caller.err = errors.New("transport")
			if err := declaration.Execute(rt); err == nil {
				t.Fatal("transport error returned success")
			}
			caller.err = nil
			caller.payload = `{"success":true}`
			if err := declaration.Execute(rt); err == nil {
				t.Fatal("projection error returned success")
			}
		})
	}
}
