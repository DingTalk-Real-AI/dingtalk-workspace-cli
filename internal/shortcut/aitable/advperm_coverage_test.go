// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageAdvpermShortcutsPublishUnifiedResultAndSafety(t *testing.T) {
	for _, declaration := range []shortcut.Shortcut{RoleMembersAdd, RoleMembersUpdate, RoleCopy, RoleQueryUser} {
		if declaration.Service != "aitable" || declaration.Product != "aitable" {
			t.Errorf("%s route = %q/%q, want aitable/aitable", declaration.Command, declaration.Service, declaration.Product)
		}
		if declaration.OutputRollout != output.RolloutUnifiedActive || declaration.Contract.Result == nil || len(declaration.Contract.Result.Outcomes) == 0 || len(declaration.Contract.Result.DataSchema) == 0 {
			t.Fatalf("%s missing unified result declaration", declaration.Command)
		}
	}
	if RoleCopy.Safety.Confirmation != "user_required" || RoleCopy.Safety.Idempotency != "non_idempotent" {
		t.Fatalf("role-copy safety = %#v", RoleCopy.Safety)
	}
	if RoleMembersUpdate.Flags[4].RequiredWhen != "default-role-mode=1" || RoleMembersUpdate.Validate == nil || RoleMembersAdd.Validate == nil {
		t.Fatal("member validation contract was not published")
	}
	for _, declaration := range []shortcut.Shortcut{RoleMembersAdd, RoleMembersUpdate} {
		for surface, text := range map[string]string{
			"flags": declaration.Flags[2].Desc, "tips": strings.Join(declaration.Tips, " "),
			"constraints": declaration.Constraints[0].Description,
		} {
			for _, phrase := range []string{`"1"=群`, `"2"=用户`, `"3"=组织`, `"4"=部门`, `"5"=虚拟组`, `"6"=角色组`, `"8"=动态组`, "非空 String", "不是 userId", "JSON number", "Java Long", "docOrgId"} {
				if !strings.Contains(text, phrase) {
					t.Errorf("%s %s missing %q", declaration.Command, surface, phrase)
				}
			}
		}
		if declaration.Intent != declaration.Contract.Description || declaration.Contract.Selection.UseWhen[0] != declaration.Intent {
			t.Errorf("%s intent and contract drifted", declaration.Command)
		}
	}
}

func TestCrossPlatformCoverageAdvpermShortcutsDispatchExactParameters(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantTool string
		wantArgs map[string]any
	}{
		{
			name:     "add members",
			args:     []string{"+role-members-add", "--base-id", "B", "--role-id", "R", "--members", `[{"id":" U ","memberType":" 2 "}]`, "--yes"},
			wantTool: "add_role_members",
			wantArgs: map[string]any{"baseId": "B", "roleId": "R", "members": []any{map[string]any{"id": "U", "memberType": "2"}}},
		},
		{
			name:     "replace cross org members preserves String and number",
			args:     []string{"+role-members-update", "--base-id", "B", "--role-id", "10685308981", "--members", `[{"id":" 12345 ","memberType":" 2 ","memberIdBelongOrgId":123456},{"id":"67890","memberType":"4"}]`, "--default-role-mode", "0", "--yes"},
			wantTool: "update_role_members",
			wantArgs: map[string]any{"baseId": "B", "roleId": "10685308981", "members": []any{map[string]any{"id": "12345", "memberType": "2", "memberIdBelongOrgId": float64(123456)}, map[string]any{"id": "67890", "memberType": "4"}}, "defaultRoleMode": 0},
		},
		{
			name:     "update custom default trims id",
			args:     []string{"+role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "1", "--default-role-id", " R-default ", "--yes"},
			wantTool: "update_role_members",
			wantArgs: map[string]any{"baseId": "B", "roleId": "R", "members": []any{}, "defaultRoleMode": 1, "defaultRoleId": "R-default"},
		},
		{
			name:     "update inherited default",
			args:     []string{"+role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "2", "--yes"},
			wantTool: "update_role_members",
			wantArgs: map[string]any{"baseId": "B", "roleId": "R", "members": []any{}, "defaultRoleMode": 2},
		},
		{
			name:     "copy with name",
			args:     []string{"+role-copy", "--base-id", "B", "--source-role-id", "R", "--name", "copy", "--yes"},
			wantTool: "copy_role",
			wantArgs: map[string]any{"baseId": "B", "sourceRoleId": "R", "name": "copy"},
		},
		{
			name:     "copy without name",
			args:     []string{"+role-copy", "--base-id", "B", "--source-role-id", "R", "--yes"},
			wantTool: "copy_role",
			wantArgs: map[string]any{"baseId": "B", "sourceRoleId": "R"},
		},
		{
			name:     "copy empty name uses server default",
			args:     []string{"+role-copy", "--base-id", "B", "--source-role-id", "10685308981", "--name", "", "--yes"},
			wantTool: "copy_role",
			wantArgs: map[string]any{"baseId": "B", "sourceRoleId": "10685308981", "name": ""},
		},
		{
			name:     "query empty target uses caller",
			args:     []string{"+role-query-user", "--base-id", "B", "--target-uid", ""},
			wantTool: "query_user_roles",
			wantArgs: map[string]any{"baseId": "B", "targetUid": ""},
		},
		{
			name:     "query current user",
			args:     []string{"+role-query-user", "--base-id", "B"},
			wantTool: "query_user_roles",
			wantArgs: map[string]any{"baseId": "B"},
		},
		{
			name:     "query target user",
			args:     []string{"+role-query-user", "--base-id", "B", "--target-uid", "U"},
			wantTool: "query_user_roles",
			wantArgs: map[string]any{"baseId": "B", "targetUid": "U"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &datasourceCoverageCaller{}
			if err := runDatasourceShortcutCLI(t, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.toolLog) != 1 || caller.toolLog[0] != tc.wantTool {
				t.Fatalf("tools = %v, want %q", caller.toolLog, tc.wantTool)
			}
			if !reflect.DeepEqual(caller.serverLog, []string{"aitable"}) {
				t.Fatalf("servers = %v, want [aitable]", caller.serverLog)
			}
			if !reflect.DeepEqual(caller.argLog[0], tc.wantArgs) {
				t.Fatalf("args = %#v, want %#v", caller.argLog[0], tc.wantArgs)
			}
		})
	}
}

type advpermDirectRouteCaller struct {
	*datasourceCoverageCaller
	resolveCalls int
}

func (c *advpermDirectRouteCaller) ResolveToolProduct(context.Context, []string, string) (string, error) {
	c.resolveCalls++
	return "", errors.New("unexpected compatibility discovery")
}

func TestCrossPlatformCoverageAdvpermShortcutsDirectRouteSafety(t *testing.T) {
	for _, tc := range []struct {
		tool string
		args []string
	}{
		{"add_role_members", []string{"+role-members-add", "--base-id", "B", "--role-id", "R", "--members", `[{"id":"U","memberType":"2"}]`}},
		{"update_role_members", []string{"+role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "0"}},
		{"copy_role", []string{"+role-copy", "--base-id", "B", "--source-role-id", "R"}},
		{"query_user_roles", []string{"+role-query-user", "--base-id", "B"}},
	} {
		for _, mode := range []string{"confirmed", "unconfirmed", "dry-run", "MCP failure"} {
			t.Run(tc.tool+"/"+mode, func(t *testing.T) {
				caller := &advpermDirectRouteCaller{datasourceCoverageCaller: &datasourceCoverageCaller{dryRun: mode == "dry-run"}}
				helpers.InitDepsForTest(t, caller)
				root := newPlatformCoverageRoot()
				args := append([]string{"aitable"}, tc.args...)
				wantCalls, wantErr := 1, ""
				switch mode {
				case "confirmed":
					args = append(args, "--yes")
				case "unconfirmed":
					if tc.tool != "query_user_roles" {
						wantCalls, wantErr = 0, "confirmation_required"
					}
				case "dry-run":
					args = append(args, "--dry-run")
					wantCalls = 0
				case "MCP failure":
					args = append(args, "--yes")
					caller.err = errors.New("MCP unavailable")
					wantErr = "MCP unavailable"
				}
				root.SetArgs(args)
				err := corecmd.ExecuteContextForTest(root, context.Background())
				if wantErr == "confirmation_required" {
					if !apperrors.IsConfirmationRequired(err) {
						t.Fatalf("error = %v, want confirmation_required", err)
					}
				} else if (wantErr == "" && err != nil) || (wantErr != "" && (err == nil || !strings.Contains(err.Error(), wantErr))) {
					t.Fatalf("error = %v, want %q", err, wantErr)
				}
				if caller.resolveCalls != 0 || len(caller.toolLog) != wantCalls {
					t.Fatalf("discovery=%d tools=%v, want zero discovery and %d calls", caller.resolveCalls, caller.toolLog, wantCalls)
				}
				if wantCalls == 1 && (caller.serverLog[0] != "aitable" || caller.toolLog[0] != tc.tool) {
					t.Fatalf("servers=%v tools=%v, want aitable/%s", caller.serverLog, caller.toolLog, tc.tool)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAdvpermShortcutsProjectBusinessDataOnly(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"add", []string{"+role-members-add", "--base-id", "B", "--role-id", "R", "--members", `[{"id":"U","memberType":"2"}]`, "--yes"}},
		{"update", []string{"+role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "0", "--yes"}},
		{"copy", []string{"+role-copy", "--base-id", "B", "--source-role-id", "R", "--yes"}},
		{"query", []string{"+role-query-user", "--base-id", "B"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &datasourceCoverageCaller{resp: `{"success":true,"data":{"marker":"business"}}`}
			helpers.InitDepsForTest(t, caller)
			root := newPlatformCoverageRoot()
			var stdout bytes.Buffer
			root.SetOut(&stdout)
			root.SetArgs(append([]string{"aitable"}, tc.args...))
			if err := corecmd.ExecuteContextForTest(root, context.Background()); err != nil {
				t.Fatal(err)
			}
			executed, _, err := root.Find([]string{"aitable", tc.args[0]})
			if err != nil {
				t.Fatal(err)
			}
			if _, emitted, err := output.EmitStoredResult(executed); err != nil || !emitted {
				t.Fatalf("emit = (%t, %v)", emitted, err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			data, ok := envelope["data"].(map[string]any)
			if !ok || data["marker"] != "business" || data["success"] != nil || data["data"] != nil {
				t.Fatalf("projected data = %#v", envelope["data"])
			}
		})
	}
}

func TestCrossPlatformCoverageAdvpermShortcutUnifiedDataProjectionFailures(t *testing.T) {
	dryRunCmd := &cobra.Command{Use: "+role-query-user"}
	dryRunCmd.Flags().Bool("dry-run", true, "")
	dryRunContext, _ := output.WithResultStore(context.Background())
	dryRunCmd.SetContext(dryRunContext)
	output.SetCommandRollout(dryRunCmd, output.RolloutUnifiedActive)
	if err := executeAdvancedPermissionMCP(shortcut.RuntimeContextForTest(dryRunCmd, RoleQueryUser), "query_user_roles", map[string]any{}, false); err != nil {
		t.Fatal(err)
	}

	t.Run("MCP failure", func(t *testing.T) {
		helpers.InitDepsForTest(t, &datasourceCoverageCaller{err: errors.New("MCP unavailable")})
		err := executeAdvancedPermissionMCP(shortcut.RuntimeContextForTest(&cobra.Command{}, RoleQueryUser), "query_user_roles", map[string]any{}, false)
		if err == nil || !strings.Contains(err.Error(), "MCP unavailable") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("missing data", func(t *testing.T) {
		helpers.InitDepsForTest(t, &datasourceCoverageCaller{resp: `{"success":true}`})
		err := executeAdvancedPermissionMCP(shortcut.RuntimeContextForTest(&cobra.Command{}, RoleQueryUser), "query_user_roles", map[string]any{}, false)
		if err == nil || !strings.Contains(err.Error(), "缺少非空 data") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageAdvpermShortcutExecuteValidationBranches(t *testing.T) {
	addCmd := &cobra.Command{Use: "+role-members-add"}
	addCmd.Flags().String("members", "", "")
	if err := addCmd.Flags().Set("members", "{}"); err != nil {
		t.Fatal(err)
	}
	addRuntime := shortcut.RuntimeContextForTest(addCmd, RoleMembersAdd)
	if err := RoleMembersAdd.Validate(addRuntime); err == nil {
		t.Fatal("member validation accepted a JSON object")
	}
	if err := RoleMembersAdd.Execute(addRuntime); err == nil {
		t.Fatal("member execution accepted a JSON object")
	}

	updateCmd := &cobra.Command{Use: "+role-members-update"}
	updateCmd.Flags().String("base-id", "", "")
	updateCmd.Flags().String("role-id", "", "")
	updateCmd.Flags().String("members", "", "")
	updateCmd.Flags().Int("default-role-mode", 0, "")
	updateCmd.Flags().String("default-role-id", "", "")
	for name, value := range map[string]string{"base-id": "B", "role-id": "R", "members": "[]", "default-role-mode": "3"} {
		if err := updateCmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := RoleMembersUpdate.Execute(shortcut.RuntimeContextForTest(updateCmd, RoleMembersUpdate)); err == nil {
		t.Fatal("member update accepted an invalid default-role-mode")
	}

	if err := updateCmd.Flags().Set("members", "{}"); err != nil {
		t.Fatal(err)
	}
	updateRuntime := shortcut.RuntimeContextForTest(updateCmd, RoleMembersUpdate)
	if err := RoleMembersUpdate.Validate(updateRuntime); err == nil {
		t.Fatal("member update validation accepted a JSON object")
	}
	if err := RoleMembersUpdate.Execute(updateRuntime); err == nil {
		t.Fatal("member update execution accepted a JSON object")
	}

	for name, value := range map[string]string{"members": "[]", "default-role-mode": "1"} {
		if err := updateCmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := RoleMembersUpdate.Execute(shortcut.RuntimeContextForTest(updateCmd, RoleMembersUpdate)); err == nil {
		t.Fatal("member update accepted a missing custom default role")
	}

	if err := updateCmd.Flags().Set("default-role-mode", "0"); err != nil {
		t.Fatal(err)
	}
	caller := &datasourceCoverageCaller{}
	helpers.InitDepsForTest(t, caller)
	if err := RoleMembersUpdate.Execute(shortcut.RuntimeContextForTest(updateCmd, RoleMembersUpdate)); err != nil {
		t.Fatal(err)
	}
	if len(caller.toolLog) != 1 || caller.toolLog[0] != "update_role_members" {
		t.Fatalf("tools = %v", caller.toolLog)
	}
}

func TestCrossPlatformCoverageAdvpermShortcutMemberLimit(t *testing.T) {
	raw := "[" + strings.Repeat(`{"id":"U"},`, 200) + `{"id":"U"}]`
	if _, err := parseRoleMembers(raw); err == nil || !strings.Contains(err.Error(), "最多 200") {
		t.Fatalf("error = %v", err)
	}
}

func TestCrossPlatformCoverageAdvpermShortcutValidationStopsBeforeMCP(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"add malformed members", []string{"+role-members-add", "--base-id", "B", "--role-id", "R", "--members", "{}"}, "JSON"},
		{"add invalid JSON", []string{"+role-members-add", "--base-id", "B", "--role-id", "R", "--members", "["}, "JSON"},
		{"add null member", []string{"+role-members-add", "--base-id", "B", "--role-id", "R", "--members", "[null]"}, "id 和 memberType"},
		{"add empty member", []string{"+role-members-add", "--base-id", "B", "--role-id", "R", "--members", "[{}]"}, "id 和 memberType"},
		{"add numeric member type", []string{"+role-members-add", "--base-id", "B", "--role-id", "R", "--members", `[{"id":"12345","memberType":2}]`}, "id 和 memberType"},
		{"add numeric member id", []string{"+role-members-add", "--base-id", "B", "--role-id", "R", "--members", `[{"id":12345,"memberType":"2"}]`}, "id 和 memberType"},
		{"update missing members", []string{"+role-members-update", "--base-id", "B", "--role-id", "R", "--default-role-mode", "0"}, "members"},
		{"update null members", []string{"+role-members-update", "--base-id", "B", "--role-id", "R", "--members", "null", "--default-role-mode", "0"}, "JSON"},
		{"update missing default mode", []string{"+role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]"}, "default-role-mode"},
		{"update invalid mode", []string{"+role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "3"}, "仅支持"},
		{"update missing custom default", []string{"+role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "1"}, "default-role-id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &datasourceCoverageCaller{}
			err := runDatasourceShortcutCLI(t, caller, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if len(caller.toolLog) != 0 {
				t.Fatalf("invalid invocation called MCP: %v", caller.toolLog)
			}
		})
	}
}
