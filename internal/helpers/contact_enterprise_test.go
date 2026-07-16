// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type contactEnterpriseCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type contactEnterpriseCaller struct {
	calls []contactEnterpriseCall
}

func (c *contactEnterpriseCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, contactEnterpriseCall{
		productID: productID,
		toolName:  toolName,
		args:      args,
	})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*contactEnterpriseCaller) Format() string { return "json" }
func (*contactEnterpriseCaller) DryRun() bool   { return false }
func (*contactEnterpriseCaller) Fields() string { return "" }
func (*contactEnterpriseCaller) JQ() string     { return "" }

func runContactEnterpriseCommand(t *testing.T, args ...string) (*contactEnterpriseCaller, error) {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	caller := &contactEnterpriseCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard
	os.Args = append([]string{"dws", "contact"}, args...)

	cmd := newContactCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return caller, cmd.Execute()
}

func TestCrossPlatformCoverageContactEnterpriseCommandsExposeExpectedFlags(t *testing.T) {
	root := newContactCommand()
	cases := []struct {
		path  []string
		flags []string
	}{
		{[]string{"org", "create"}, []string{"org-name", "creator-username"}},
		{[]string{"user", "invite"}, []string{"org-user-name", "org-user-mobile", "depts"}},
		{[]string{"account", "create"}, []string{"org-user-name", "login-id", "org-user-mobile", "email", "dept-ids", "send-pwd-via-sms"}},
	}
	for _, tc := range cases {
		cmd := requireWukongSyncCommand(t, root, tc.path...)
		requireWukongSyncFlags(t, cmd, tc.flags...)
	}
}

func TestCrossPlatformCoverageContactEnterpriseCommandsMapMCPArguments(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		toolName string
		wantArgs map[string]any
	}{
		{
			name:     "create organization",
			args:     []string{"org", "create", "--org-name", "测试企业", "--creator-username", "张三"},
			toolName: "org_create",
			wantArgs: map[string]any{
				"orgName":         "测试企业",
				"creatorUsername": "张三",
			},
		},
		{
			name:     "invite employee",
			args:     []string{"user", "invite", "--org-user-name", " 李四 ", "--org-user-mobile", " 13800138000 ", "--depts", `[{"deptId":1}]`},
			toolName: "add_employee",
			wantArgs: map[string]any{
				"orgUserName":   "李四",
				"orgUserMobile": "13800138000",
				"depts":         []map[string]any{{"deptId": float64(1)}},
			},
		},
		{
			name: "create enterprise account",
			args: []string{
				"account", "create",
				"--org-user-name", " 王五 ",
				"--login-id", " wangwu001 ",
				"--org-user-mobile", " 13900139000 ",
				"--email", " wangwu@example.com ",
				"--dept-ids", "1, 2,3",
				"--send-pwd-via-sms",
			},
			toolName: "exclusive_account_create",
			wantArgs: map[string]any{
				"orgUserName":   "王五",
				"loginId":       "wangwu001",
				"orgUserMobile": "13900139000",
				"email":         "wangwu@example.com",
				"deptIds":       []int64{1, 2, 3},
				"sendPwdViaSms": true,
			},
		},
		{
			name:     "create enterprise account without optional send flag",
			args:     []string{"account", "create", "--org-user-name", "王五", "--login-id", "wangwu001"},
			toolName: "exclusive_account_create",
			wantArgs: map[string]any{
				"orgUserName": "王五",
				"loginId":     "wangwu001",
			},
		},
		{
			name:     "create enterprise account with explicit false send flag",
			args:     []string{"account", "create", "--org-user-name", "王五", "--login-id", "wangwu001", "--send-pwd-via-sms=false"},
			toolName: "exclusive_account_create",
			wantArgs: map[string]any{
				"orgUserName":   "王五",
				"loginId":       "wangwu001",
				"sendPwdViaSms": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller, err := runContactEnterpriseCommand(t, tt.args...)
			if err != nil {
				t.Fatalf("command returned error: %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("tool call count = %d, want 1", len(caller.calls))
			}
			call := caller.calls[0]
			if call.productID != "contact" || call.toolName != tt.toolName {
				t.Fatalf("tool call = %s/%s, want contact/%s", call.productID, call.toolName, tt.toolName)
			}
			if !reflect.DeepEqual(call.args, tt.wantArgs) {
				t.Fatalf("tool args = %#v, want %#v", call.args, tt.wantArgs)
			}
		})
	}
}

func TestCrossPlatformCoverageContactUserInviteRejectsInvalidDepartmentsJSON(t *testing.T) {
	caller, err := runContactEnterpriseCommand(t,
		"user", "invite",
		"--org-user-name", "张三",
		"--org-user-mobile", "13800138000",
		"--depts", "not-json",
	)
	if err == nil || !strings.Contains(err.Error(), "--depts JSON 解析失败") {
		t.Fatalf("error = %v, want departments JSON validation", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("invalid input made %d remote call(s)", len(caller.calls))
	}
}

func TestCrossPlatformCoverageContactAccountCreateRejectsInvalidDepartmentIDs(t *testing.T) {
	for _, deptIDs := range []string{"1,not-an-id,3", "1,,3", "   "} {
		t.Run(deptIDs, func(t *testing.T) {
			caller, err := runContactEnterpriseCommand(t,
				"account", "create",
				"--org-user-name", "张三",
				"--login-id", "zhangsan001",
				"--dept-ids", deptIDs,
			)
			if err == nil || !strings.Contains(err.Error(), "--dept-ids 解析失败") {
				t.Fatalf("error = %v, want department ID validation", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid input made %d remote call(s)", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageContactOrgAndAccountCreateRejectWhitespaceIdentityFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"organization name", []string{"org", "create", "--org-name", "   ", "--creator-username", "张三"}},
		{"organization creator", []string{"org", "create", "--org-name", "测试企业", "--creator-username", "   "}},
		{"account user name", []string{"account", "create", "--org-user-name", "   ", "--login-id", "zhangsan001"}},
		{"account login ID", []string{"account", "create", "--org-user-name", "张三", "--login-id", "   "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller, err := runContactEnterpriseCommand(t, tt.args...)
			if err == nil || !strings.Contains(err.Error(), "不能为空") {
				t.Fatalf("error = %v, want non-blank identity validation", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("blank input made %d remote call(s)", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageContactEnterpriseCommandsRequireCoreIdentityFlags(t *testing.T) {
	tests := [][]string{
		{"org", "create", "--org-name", "测试企业"},
		{"user", "invite", "--org-user-name", "张三"},
		{"account", "create", "--org-user-name", "张三"},
	}
	for _, args := range tests {
		caller, err := runContactEnterpriseCommand(t, args...)
		if err == nil {
			t.Fatalf("%v returned nil error, want required-flag validation", args)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("%v made %d remote call(s)", args, len(caller.calls))
		}
	}
}
