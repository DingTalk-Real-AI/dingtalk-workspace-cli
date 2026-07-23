// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func runContactUpdateCommand(t *testing.T, input string, args ...string) (*contactEnterpriseCaller, error) {
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

	root := newContactCommand()
	root.PersistentFlags().Bool("yes", false, "skip confirmation")
	RegisterCamelCaseAliases(root)
	root.SetIn(strings.NewReader(input))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	return caller, root.Execute()
}

func TestCrossPlatformCoverageContactUpdateCommandsExposeExpectedFlags(t *testing.T) {
	root := newContactCommand()
	cases := []struct {
		path  []string
		flags []string
	}{
		{[]string{"dept", "create"}, []string{"name", "parent", "create-dept-group"}},
		{[]string{"dept", "update"}, []string{"dept", "name", "parent"}},
		{[]string{"user", "update"}, []string{"user-id", "org-user-name", "depts", "master-user-id"}},
		{[]string{"user", "update-self"}, []string{"nick", "avatar-file-id"}},
		{[]string{"account", "update"}, []string{"user-id", "org-user-name", "depts", "master-user-id", "nick", "avatar-file-id"}},
	}
	for _, tc := range cases {
		cmd := requireWukongSyncCommand(t, root, tc.path...)
		requireWukongSyncFlags(t, cmd, tc.flags...)
	}
}

func TestCrossPlatformCoverageContactUpdateCommandsMapMCPArguments(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		toolName string
		wantArgs map[string]any
	}{
		{
			name:     "create department at root",
			args:     []string{"dept", "create", "--name", " 产品部 ", "--create-dept-group=true", "--yes"},
			toolName: "department_create",
			wantArgs: map[string]any{"deptName": "产品部", "createDeptGroup": true},
		},
		{
			name:     "create department with camel aliases",
			args:     []string{"dept", "create", "--deptName", "研发组", "--superDeptId", "42", "--createDeptGroup=false", "--yes"},
			toolName: "department_create",
			wantArgs: map[string]any{"deptName": "研发组", "createDeptGroup": false, "superDeptId": int64(42)},
		},
		{
			name:     "update department",
			args:     []string{"dept", "modify", "--dept-id", "7", "--name", "研发中心", "--parent", "1", "--yes"},
			toolName: "department_update",
			wantArgs: map[string]any{"deptId": int64(7), "deptName": "研发中心", "superDeptId": int64(1)},
		},
		{
			name:     "update employee",
			args:     []string{"user", "update", "--userId", "user-1", "--orgUserName", "张三", "--depts", `[{"deptId":1}]`, "--masterUserId", "manager-1", "--yes"},
			toolName: "employee_update",
			wantArgs: map[string]any{
				"userId":       "user-1",
				"orgUserName":  "张三",
				"depts":        []map[string]any{{"deptId": float64(1)}},
				"masterUserId": "manager-1",
			},
		},
		{
			name:     "update current user profile",
			args:     []string{"user", "update-me", "--nick", "新昵称", "--avatarFileId", "file-1", "--yes"},
			toolName: "self_user_profile_update",
			wantArgs: map[string]any{"nick": "新昵称", "avatarFileId": "file-1"},
		},
		{
			name:     "update enterprise account",
			args:     []string{"account", "edit", "--user-id", "user-2", "--org-user-name", "李四", "--depts", `[{"deptId":2}]`, "--master-user-id", "manager-2", "--nick", "小李", "--avatar-file-id", "file-2", "--yes"},
			toolName: "exclusive_account_user_update",
			wantArgs: map[string]any{
				"userId":       "user-2",
				"orgUserName":  "李四",
				"depts":        []map[string]any{{"deptId": float64(2)}},
				"masterUserId": "manager-2",
				"nick":         "小李",
				"avatarFileId": "file-2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller, err := runContactUpdateCommand(t, "", tt.args...)
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

func TestCrossPlatformCoverageContactUpdateCommandsRequireConfirmation(t *testing.T) {
	tests := [][]string{
		{"dept", "create", "--name", "产品部", "--create-dept-group=true"},
		{"dept", "update", "--dept", "7", "--name", "研发中心"},
		{"user", "update", "--user-id", "user-1", "--org-user-name", "张三"},
		{"user", "update-self", "--nick", "新昵称"},
		{"account", "update", "--user-id", "user-2", "--nick", "小李"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args[:2], "-"), func(t *testing.T) {
			caller, err := runContactUpdateCommand(t, "no\n", args...)
			if err != nil {
				t.Fatalf("declined confirmation returned error: %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("declined confirmation made %d remote call(s)", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageContactUpdateCommandsValidateInput(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"create missing name", []string{"dept", "create", "--create-dept-group=true", "--yes"}, "required"},
		{"create blank name", []string{"dept", "create", "--name", " ", "--create-dept-group=true", "--yes"}, "不能为空"},
		{"create missing group choice", []string{"dept", "create", "--name", "产品部", "--yes"}, "--create-dept-group"},
		{"create detached false is rejected", []string{"dept", "create", "--name", "产品部", "--create-dept-group", "false", "--yes"}, "unknown command"},
		{"create invalid parent", []string{"dept", "create", "--name", "产品部", "--create-dept-group=true", "--parent", "bad", "--yes"}, "must be an integer"},
		{"update invalid department", []string{"dept", "update", "--dept", "root", "--name", "产品部", "--yes"}, "根部门 deptId=1"},
		{"update invalid parent", []string{"dept", "update", "--dept", "7", "--name", "产品部", "--parent", "bad", "--yes"}, "must be an integer"},
		{"update missing name", []string{"dept", "update", "--dept", "7", "--yes"}, "required"},
		{"update blank name", []string{"dept", "update", "--dept", "7", "--name", " ", "--yes"}, "不能为空"},
		{"employee missing id", []string{"user", "update", "--org-user-name", "张三", "--yes"}, "required"},
		{"employee blank id", []string{"user", "update", "--user-id", " ", "--org-user-name", "张三", "--yes"}, "不能为空"},
		{"employee no changes", []string{"user", "update", "--user-id", "user-1", "--org-user-name", " ", "--depts", " ", "--master-user-id", " ", "--yes"}, "至少需要一个修改项"},
		{"employee invalid departments", []string{"user", "update", "--user-id", "user-1", "--depts", "bad", "--yes"}, "--depts JSON 解析失败"},
		{"self no changes", []string{"user", "update-self", "--nick", " ", "--avatar-file-id", " ", "--yes"}, "至少需要一个修改项"},
		{"account missing id", []string{"account", "update", "--nick", "小李", "--yes"}, "required"},
		{"account blank id", []string{"account", "update", "--user-id", " ", "--nick", "小李", "--yes"}, "不能为空"},
		{"account no changes", []string{"account", "update", "--user-id", "user-2", "--nick", " ", "--yes"}, "至少需要一个修改项"},
		{"account invalid departments", []string{"account", "update", "--user-id", "user-2", "--depts", "bad", "--yes"}, "--depts JSON 解析失败"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller, err := runContactUpdateCommand(t, "", tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid input made %d remote call(s)", len(caller.calls))
			}
		})
	}
}
