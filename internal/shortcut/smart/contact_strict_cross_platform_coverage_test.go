// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

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

type contactSmartStrictCall struct {
	tool string
	args map[string]any
}

type contactSmartStrictCaller struct {
	calls      []contactSmartStrictCall
	searchZero bool
}

func (caller *contactSmartStrictCaller) CallTool(_ context.Context, _, tool string, args map[string]any) (*edition.ToolResult, error) {
	caller.calls = append(caller.calls, contactSmartStrictCall{tool: tool, args: args})
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
}

func TestCrossPlatformCoverageContactSmartContracts(t *testing.T) {
	items := []*shortcut.Shortcut{&ByMobile, &DeptMembers, &Lookup, &Org, &ResolveDept, &Team, &Whoami}
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
