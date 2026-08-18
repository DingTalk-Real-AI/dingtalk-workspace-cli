// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package contact

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type contactCaller struct {
	payload string
	calls   int
	product string
	tool    string
	args    map[string]any
}

func (caller *contactCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	caller.calls++
	caller.product, caller.tool, caller.args = product, tool, args
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: caller.payload}}}, nil
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
		{"success": true},
		{"success": true, "deptUserList": map[string]any{}},
		{"success": true, "deptUserList": []any{map[string]any{}}},
		{"success": true, "deptUserList": []any{map[string]any{"userInfo": map[string]any{"name": "no-id"}}}},
	} {
		if got, projectErr := strictMembers(data, "contact/members", "deptUserList"); projectErr == nil {
			t.Errorf("broken members returned success: %v", got)
		}
	}
}

func TestCrossPlatformCoverageContactSearchMobileUsesReviewedObjectShape(t *testing.T) {
	caller := &contactCaller{payload: `{"success":true,"result":[{"userId":"stable-user","name":"Fixture"}]}`}
	helpers.InitDepsForTest(t, caller)
	cmd := &cobra.Command{Use: "+search-mobile"}
	cmd.Flags().String("mobile", "", "")
	if err := cmd.Flags().Set("mobile", "fixture-mobile"); err != nil {
		t.Fatal(err)
	}
	declaration := SearchMobile
	declaration.OutputRollout = output.RolloutLegacyOnly
	if err := declaration.Execute(shortcut.RuntimeContextForTest(cmd, declaration)); err != nil {
		t.Fatalf("search-mobile: %v", err)
	}
	if caller.calls != 1 || caller.product != "contact" || caller.tool != "search_contact_by_key_word" || caller.args["keyword"] != "fixture-mobile" {
		t.Fatalf("mapping = calls:%d product:%q tool:%q args:%v", caller.calls, caller.product, caller.tool, caller.args)
	}

	caller.payload = `{"success":true,"result":[]}`
	if err := declaration.Execute(shortcut.RuntimeContextForTest(cmd, declaration)); err != nil {
		t.Fatalf("explicit mobile zero rejected: %v", err)
	}
	caller.payload = `{"success":true}`
	if err := declaration.Execute(shortcut.RuntimeContextForTest(cmd, declaration)); err == nil {
		t.Fatal("missing result must not become a successful empty search")
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

func TestCrossPlatformCoverageUnavailableContactMakesNoRemoteCall(t *testing.T) {
	caller := &contactCaller{payload: `{"success":true,"result":[]}`}
	helpers.InitDepsForTest(t, caller)
	for _, declaration := range []shortcut.Shortcut{ListRoles, ListRosterFields, GetRoster} {
		if err := declaration.Execute(shortcut.RuntimeContextForTest(&cobra.Command{Use: declaration.Command}, declaration)); err == nil {
			t.Errorf("%s unavailable error = %v", declaration.Command, err)
		}
	}
	if caller.calls != 0 {
		t.Fatalf("unavailable Contact shortcuts made %d calls", caller.calls)
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
		{"success": true},
		{"success": true, "result": map[string]any{}},
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
			"labels":    []any{map[string]any{"labelId": float64(1), "name": "Fixture role"}},
		}},
	}, "contact/roles")
	if err != nil || len(roles) != 1 || roles[0]["labelId"] != int64(1) {
		t.Fatalf("strict roles = %#v, err=%v", roles, err)
	}
	for _, broken := range []map[string]any{
		{"success": true},
		{"success": true, "result": map[string]any{}},
		{"success": true, "result": []any{map[string]any{"groupName": "Fixture", "labels": []any{map[string]any{"labelId": nil, "name": ""}}}}},
		{"success": true, "result": []any{map[string]any{"groupName": "Fixture", "labels": []any{map[string]any{"labelId": float64(1), "name": "One"}, map[string]any{"labelId": float64(1), "name": "Duplicate"}}}}},
	} {
		if got, parseErr := strictRoles(broken, "contact/roles"); parseErr == nil {
			t.Errorf("broken roles returned success: %#v", got)
		}
	}
}

func TestCrossPlatformCoverageContactDirectContracts(t *testing.T) {
	items := []*shortcut.Shortcut{
		&ListFollowings, &SearchUser, &SearchMobile, &ListRoles, &ListRoleMembers,
		&ListSubDepts, &ListDeptMembers, &ListRosterFields, &GetRoster,
	}
	for _, item := range items {
		if item.OutputRollout != output.RolloutUnifiedActive || item.Contract.Result == nil || strings.TrimSpace(item.Safety.Effect) == "" {
			t.Errorf("%s lacks Contract/Result/Safety/unified output", item.Command)
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(item.Contract.Result.DataSchema, &schema); err != nil || schema["type"] != "object" {
			t.Errorf("%s invalid Result schema: schema=%v err=%v", item.Command, schema, err)
		}
	}
}
