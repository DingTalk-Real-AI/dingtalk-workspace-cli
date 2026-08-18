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
	caller := &contactCaller{payload: `{"success":true,"result":{"userId":"stable-user","orgUserName":"Fixture"}}`}
	helpers.InitDepsForTest(t, caller)
	cmd := &cobra.Command{Use: "+search-mobile"}
	cmd.Flags().String("mobile", "fixture-mobile", "")
	declaration := SearchMobile
	declaration.OutputRollout = output.RolloutLegacyOnly
	if err := declaration.Execute(shortcut.RuntimeContextForTest(cmd, declaration)); err != nil {
		t.Fatalf("search-mobile: %v", err)
	}
	if caller.calls != 1 || caller.product != "contact" || caller.tool != "search_user_by_mobile" || caller.args["mobile"] != "fixture-mobile" {
		t.Fatalf("mapping = calls:%d product:%q tool:%q args:%v", caller.calls, caller.product, caller.tool, caller.args)
	}

	caller.payload = `{"success":true}`
	if err := declaration.Execute(shortcut.RuntimeContextForTest(cmd, declaration)); err == nil {
		t.Fatal("missing result must not become a successful empty search")
	}
}

func TestCrossPlatformCoverageUnavailableContactMakesNoRemoteCall(t *testing.T) {
	caller := &contactCaller{payload: `{"success":true,"result":[]}`}
	helpers.InitDepsForTest(t, caller)
	for _, declaration := range []shortcut.Shortcut{ListFollowings, ListRoles, ListRosterFields, GetRoster} {
		if err := declaration.Execute(shortcut.RuntimeContextForTest(&cobra.Command{Use: declaration.Command}, declaration)); err == nil || !strings.Contains(err.Error(), "cannot be proved") {
			t.Errorf("%s unavailable error = %v", declaration.Command, err)
		}
	}
	if caller.calls != 0 {
		t.Fatalf("unavailable Contact shortcuts made %d calls", caller.calls)
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
