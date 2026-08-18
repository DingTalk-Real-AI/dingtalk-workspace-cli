// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageContactSmartStrictDecoders(t *testing.T) {
	user, err := strictMobileContactUser(map[string]any{
		"success": true, "result": map[string]any{"userId": "stable-user", "orgUserName": "Fixture"},
	})
	if err != nil || user.userID != "stable-user" {
		t.Fatalf("valid mobile user rejected: user=%v err=%v", user, err)
	}
	for _, data := range []map[string]any{
		{},
		{"success": true},
		{"success": true, "result": []any{}},
		{"success": true, "result": map[string]any{"orgUserName": "no-id"}},
		{"success": true, "errorMessage": "conflict", "result": map[string]any{"userId": "stable-user"}},
	} {
		if got, decodeErr := strictMobileContactUser(data); decodeErr == nil {
			t.Errorf("broken mobile response returned success: %v", got)
		}
	}

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
