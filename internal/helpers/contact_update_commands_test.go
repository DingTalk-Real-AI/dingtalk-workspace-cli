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
		{[]string{"user", "update-ownness"}, []string{"user-id", "ownness-text"}},
		{[]string{"account", "update"}, []string{"user-id", "org-user-name", "depts", "master-user-id", "nick", "avatar-file-id"}},
		{[]string{"label", "create"}, []string{"name", "type", "parent-id"}},
		{[]string{"label", "update"}, []string{"id", "name"}},
		{[]string{"label", "delete"}, []string{"id"}},
		{[]string{"label", "add-members"}, []string{"id", "users"}},
		{[]string{"label", "remove-members"}, []string{"id", "users"}},
		{[]string{"label", "update-member-scope"}, []string{"user", "id", "depts"}},
		{[]string{"ext-field", "create"}, []string{"name"}},
		{[]string{"ext-field", "update"}, []string{"code", "org-self-tag", "client-display", "is-search"}},
		{[]string{"ext-field", "delete"}, []string{"code", "org-self-tag"}},
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
			name:     "update user ownness",
			args:     []string{"user", "update-ownness", "--user-id", "user-1", "--ownness-text", "居家办公中", "--yes"},
			toolName: "user_ownness_update",
			wantArgs: map[string]any{"userId": "user-1", "ownnessText": "居家办公中"},
		},
		{
			name:     "update user ownness with aliases",
			args:     []string{"user", "set-ownness", "--userId", "user-1", "--ownnessText", "专注开发中", "--yes"},
			toolName: "user_ownness_update",
			wantArgs: map[string]any{"userId": "user-1", "ownnessText": "专注开发中"},
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
		{
			name:     "create label role under group",
			args:     []string{"label", "create", "--name", "管理员", "--type", "role", "--parent-id", "12345", "--yes"},
			toolName: "add_label",
			wantArgs: map[string]any{"parentId": int64(12345), "labelModel": map[string]any{"name": "管理员"}},
		},
		{
			name:     "create label group at root",
			args:     []string{"label", "create", "--name", "管理层", "--type", "group", "--yes"},
			toolName: "add_label",
			wantArgs: map[string]any{"parentId": int64(-1), "labelModel": map[string]any{"name": "管理层"}},
		},
		{
			name:     "create label with camel aliases",
			args:     []string{"label", "create", "--labelName", "财务", "--labelType", "role", "--parentId", "42", "--yes"},
			toolName: "add_label",
			wantArgs: map[string]any{"parentId": int64(42), "labelModel": map[string]any{"name": "财务"}},
		},
		{
			name:     "update label",
			args:     []string{"label", "update", "--id", "12345", "--name", "新名称", "--yes"},
			toolName: "update_label",
			wantArgs: map[string]any{"labelId": int64(12345), "label": map[string]any{"name": "新名称"}},
		},
		{
			name:     "update label with camel aliases",
			args:     []string{"label", "update", "--labelId", "12345", "--labelName", "新名称", "--yes"},
			toolName: "update_label",
			wantArgs: map[string]any{"labelId": int64(12345), "label": map[string]any{"name": "新名称"}},
		},
		{
			name:     "delete label",
			args:     []string{"label", "delete", "--id", "12345", "--yes"},
			toolName: "delete_label",
			wantArgs: map[string]any{"id": int64(12345)},
		},
		{
			name:     "add label members",
			args:     []string{"label", "add-members", "--id", "1,2", "--users", "u1,u2", "--yes"},
			toolName: "add_label_members",
			wantArgs: map[string]any{"labelIds": []int64{1, 2}, "staffIds": []string{"u1", "u2"}},
		},
		{
			name:     "remove label members",
			args:     []string{"label", "remove-members", "--id", "1,2", "--users", "u1,u2", "--yes"},
			toolName: "remove_label_members",
			wantArgs: map[string]any{"labelIds": []int64{1, 2}, "staffIds": []string{"u1", "u2"}},
		},
		{
			name:     "update label member scope",
			args:     []string{"label", "update-member-scope", "--user", "u1", "--id", "12345", "--depts", "1,2,3", "--yes"},
			toolName: "update_label_member_scope",
			wantArgs: map[string]any{"staffId": "u1", "labelId": int64(12345), "deptIds": []int64{1, 2, 3}},
		},
		{
			name:     "create ext field",
			args:     []string{"ext-field", "create", "--name", "职级", "--yes"},
			toolName: "add_org_ext_attrs",
			wantArgs: map[string]any{"orgEmpAttrModels": []map[string]any{{"name": "职级", "orgSelfTag": int64(1), "newAdd": true}}},
		},
		{
			name:     "update ext field",
			args:     []string{"ext-field", "update", "--code", "rank", "--client-display", "true", "--is-search", "false", "--yes"},
			toolName: "update_org_ext_attrs",
			wantArgs: map[string]any{"orgEmpAttrModels": []map[string]any{{"code": "rank", "orgSelfTag": int64(1), "clientDisplay": true, "isSearch": false}}},
		},
		{
			name:     "update ext field with org self tag",
			args:     []string{"ext-field", "update", "--code", "rank", "--org-self-tag", "0", "--client-display", "true", "--is-search", "true", "--yes"},
			toolName: "update_org_ext_attrs",
			wantArgs: map[string]any{"orgEmpAttrModels": []map[string]any{{"code": "rank", "orgSelfTag": int64(0), "clientDisplay": true, "isSearch": true}}},
		},
		{
			name:     "update ext field with blank org self tag",
			args:     []string{"ext-field", "update", "--code", "rank", "--org-self-tag", " ", "--client-display", "true", "--is-search", "true", "--yes"},
			toolName: "update_org_ext_attrs",
			wantArgs: map[string]any{"orgEmpAttrModels": []map[string]any{{"code": "rank", "orgSelfTag": int64(1), "clientDisplay": true, "isSearch": true}}},
		},
		{
			name:     "delete ext field",
			args:     []string{"ext-field", "delete", "--code", "rank", "--yes"},
			toolName: "remove_org_ext_attrs",
			wantArgs: map[string]any{"orgEmpAttrModels": []map[string]any{{"code": "rank", "orgSelfTag": int64(1), "toDelete": true}}},
		},
		{
			name:     "delete ext field with blank org self tag",
			args:     []string{"ext-field", "delete", "--code", "rank", "--org-self-tag", " ", "--yes"},
			toolName: "remove_org_ext_attrs",
			wantArgs: map[string]any{"orgEmpAttrModels": []map[string]any{{"code": "rank", "orgSelfTag": int64(1), "toDelete": true}}},
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
		{"user", "update-ownness", "--user-id", "user-1", "--ownness-text", "居家办公中"},
		{"account", "update", "--user-id", "user-2", "--nick", "小李"},
		{"label", "create", "--name", "管理员", "--type", "group"},
		{"label", "update", "--id", "12345", "--name", "新名称"},
		{"label", "delete", "--id", "12345"},
		{"label", "add-members", "--id", "12345", "--users", "u1"},
		{"label", "remove-members", "--id", "12345", "--users", "u1"},
		{"label", "update-member-scope", "--user", "u1", "--id", "12345", "--depts", "1"},
		{"ext-field", "create", "--name", "职级"},
		{"ext-field", "update", "--code", "rank", "--client-display", "true", "--is-search", "false"},
		{"ext-field", "delete", "--code", "rank"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args[:2], "-"), func(t *testing.T) {
			caller, err := runContactUpdateCommand(t, "no\n", args...)
			if err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
				t.Fatalf("declined confirmation error = %v, want 用户取消了操作", err)
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
		{"ownness missing id", []string{"user", "update-ownness", "--ownness-text", "居家办公中", "--yes"}, "required"},
		{"ownness blank id", []string{"user", "update-ownness", "--user-id", " ", "--ownness-text", "居家办公中", "--yes"}, "不能为空"},
		{"ownness missing text", []string{"user", "update-ownness", "--user-id", "user-1", "--yes"}, "required"},
		{"ownness blank text", []string{"user", "update-ownness", "--user-id", "user-1", "--ownness-text", " ", "--yes"}, "不能为空"},
		{"account missing id", []string{"account", "update", "--nick", "小李", "--yes"}, "required"},
		{"account blank id", []string{"account", "update", "--user-id", " ", "--nick", "小李", "--yes"}, "不能为空"},
		{"account no changes", []string{"account", "update", "--user-id", "user-2", "--nick", " ", "--yes"}, "至少需要一个修改项"},
		{"account invalid departments", []string{"account", "update", "--user-id", "user-2", "--depts", "bad", "--yes"}, "--depts JSON 解析失败"},
		{"label create missing name", []string{"label", "create", "--type", "role", "--parent-id", "123", "--yes"}, "required"},
		{"label create blank name", []string{"label", "create", "--name", " ", "--type", "role", "--parent-id", "123", "--yes"}, "不能为空"},
		{"label create missing type", []string{"label", "create", "--name", "管理员", "--yes"}, "required"},
		{"label create invalid type", []string{"label", "create", "--name", "管理员", "--type", "team", "--yes"}, "仅支持 role"},
		{"label create role missing parent", []string{"label", "create", "--name", "管理员", "--type", "role", "--yes"}, "必须通过 --parent-id"},
		{"label create role zero parent", []string{"label", "create", "--name", "管理员", "--type", "role", "--parent-id", "0", "--yes"}, "有效的角色组 ID"},
		{"label create invalid parent", []string{"label", "create", "--name", "管理员", "--type", "role", "--parent-id", "bad", "--yes"}, "must be an integer"},
		{"label create group with parent", []string{"label", "create", "--name", "管理员", "--type", "group", "--parent-id", "42", "--yes"}, "无需 --parent-id"},
		{"label update missing id", []string{"label", "update", "--name", "新名称", "--yes"}, "required"},
		{"label update blank id", []string{"label", "update", "--id", " ", "--name", "新名称", "--yes"}, "must be an integer"},
		{"label update missing name", []string{"label", "update", "--id", "123", "--yes"}, "required"},
		{"label update blank name", []string{"label", "update", "--id", "123", "--name", " ", "--yes"}, "不能为空"},
		{"label delete missing id", []string{"label", "delete", "--yes"}, "required"},
		{"label delete blank id", []string{"label", "delete", "--id", " ", "--yes"}, "must be an integer"},
		{"label add-members missing id", []string{"label", "add-members", "--users", "u1", "--yes"}, "required"},
		{"label add-members missing users", []string{"label", "add-members", "--id", "123", "--yes"}, "required"},
		{"label add-members invalid id", []string{"label", "add-members", "--id", "bad", "--users", "u1", "--yes"}, "不是有效整数"},
		{"label add-members blank id", []string{"label", "add-members", "--id", " ", "--users", "u1", "--yes"}, "至少需要一个有效整数值"},
		{"label add-members blank users", []string{"label", "add-members", "--id", "123", "--users", " ", "--yes"}, "至少需要一个成员 ID"},
		{"label remove-members missing id", []string{"label", "remove-members", "--users", "u1", "--yes"}, "required"},
		{"label remove-members blank users", []string{"label", "remove-members", "--id", "123", "--users", " ", "--yes"}, "至少需要一个成员 ID"},
		{"label remove-members blank id", []string{"label", "remove-members", "--id", " ", "--users", "u1", "--yes"}, "至少需要一个有效整数值"},
		{"label update-member-scope missing user", []string{"label", "update-member-scope", "--id", "123", "--depts", "1", "--yes"}, "required"},
		{"label update-member-scope missing id", []string{"label", "update-member-scope", "--user", "u1", "--depts", "1", "--yes"}, "required"},
		{"label update-member-scope missing depts", []string{"label", "update-member-scope", "--user", "u1", "--id", "123", "--yes"}, "required"},
		{"label update-member-scope blank user", []string{"label", "update-member-scope", "--user", " ", "--id", "123", "--depts", "1", "--yes"}, "不能为空"},
		{"label update-member-scope blank depts", []string{"label", "update-member-scope", "--user", "u1", "--id", "123", "--depts", " ", "--yes"}, "至少需要一个有效整数值"},
		{"ext-field create missing name", []string{"ext-field", "create", "--yes"}, "required"},
		{"ext-field create blank name", []string{"ext-field", "create", "--name", " ", "--yes"}, "不能为空"},
		{"ext-field update missing code", []string{"ext-field", "update", "--client-display", "true", "--is-search", "false", "--yes"}, "required"},
		{"ext-field update blank code", []string{"ext-field", "update", "--code", " ", "--client-display", "true", "--is-search", "false", "--yes"}, "不能为空"},
		{"ext-field update missing client-display", []string{"ext-field", "update", "--code", "rank", "--is-search", "false", "--yes"}, "required"},
		{"ext-field update invalid bool", []string{"ext-field", "update", "--code", "rank", "--client-display", "yes", "--is-search", "false", "--yes"}, "boolean"},
		{"ext-field update missing is-search", []string{"ext-field", "update", "--code", "rank", "--client-display", "true", "--yes"}, "required"},
		{"ext-field update invalid org self tag", []string{"ext-field", "update", "--code", "rank", "--org-self-tag", "bad", "--client-display", "true", "--is-search", "false", "--yes"}, "必须是整数"},
		{"ext-field delete missing code", []string{"ext-field", "delete", "--yes"}, "required"},
		{"ext-field delete blank code", []string{"ext-field", "delete", "--code", " ", "--yes"}, "不能为空"},
		{"ext-field delete invalid org self tag", []string{"ext-field", "delete", "--code", "rank", "--org-self-tag", "bad", "--yes"}, "必须是整数"},
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
