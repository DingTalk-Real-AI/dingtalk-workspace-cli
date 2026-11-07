// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func runAITableAdvpermCommand(t *testing.T, caller *aitableTestCaller, args ...string) error {
	t.Helper()
	installAitableDeps(t, caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	cmd := newAitableCommand()
	installExampleGlobalFlags(cmd)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"--yes", "advperm"}, args...))
	return corecmd.ExecuteContextForTest(cmd, context.Background())
}

func TestCrossPlatformCoverageAITableAdvpermRoleMemberCommandsDispatchExactParameters(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantTool string
		wantArgs map[string]any
	}{
		{
			name:     "add members",
			args:     []string{"role-members-add", "--base-id", "B", "--role-id", "R", "--members", `[{"id":" U ","memberType":" 2 "}]`},
			wantTool: "add_role_members",
			wantArgs: map[string]any{"baseId": "B", "roleId": "R", "members": []any{map[string]any{"id": "U", "memberType": "2"}}},
		},
		{
			name:     "replace cross org members preserves String and number",
			args:     []string{"role-members-update", "--base-id", "B", "--role-id", "10685308981", "--members", `[{"id":" 12345 ","memberType":" 2 ","memberIdBelongOrgId":123456},{"id":"67890","memberType":"4"}]`, "--default-role-mode", "0"},
			wantTool: "update_role_members",
			wantArgs: map[string]any{"baseId": "B", "roleId": "10685308981", "members": []any{map[string]any{"id": "12345", "memberType": "2", "memberIdBelongOrgId": float64(123456)}, map[string]any{"id": "67890", "memberType": "4"}}, "defaultRoleMode": 0},
		},
		{
			name:     "replace members no default",
			args:     []string{"role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "0"},
			wantTool: "update_role_members",
			wantArgs: map[string]any{"baseId": "B", "roleId": "R", "members": []any{}, "defaultRoleMode": 0},
		},
		{
			name:     "replace members custom default trims id",
			args:     []string{"role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "1", "--default-role-id", " R-default "},
			wantTool: "update_role_members",
			wantArgs: map[string]any{"baseId": "B", "roleId": "R", "members": []any{}, "defaultRoleMode": 1, "defaultRoleId": "R-default"},
		},
		{
			name:     "replace members inherited default",
			args:     []string{"role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "2"},
			wantTool: "update_role_members",
			wantArgs: map[string]any{"baseId": "B", "roleId": "R", "members": []any{}, "defaultRoleMode": 2},
		},
		{
			name:     "copy with name",
			args:     []string{"role-copy", "--base-id", "B", "--source-role-id", "R", "--name", "copy"},
			wantTool: "copy_role",
			wantArgs: map[string]any{"baseId": "B", "sourceRoleId": "R", "name": "copy"},
		},
		{
			name:     "copy server name",
			args:     []string{"role-copy", "--base-id", "B", "--source-role-id", "R"},
			wantTool: "copy_role",
			wantArgs: map[string]any{"baseId": "B", "sourceRoleId": "R"},
		},
		{
			name:     "copy empty name uses server default",
			args:     []string{"role-copy", "--base-id", "B", "--source-role-id", "10685308981", "--name", ""},
			wantTool: "copy_role",
			wantArgs: map[string]any{"baseId": "B", "sourceRoleId": "10685308981", "name": ""},
		},
		{
			name:     "query empty target uses caller",
			args:     []string{"role-query-user", "--base-id", "B", "--target-uid", ""},
			wantTool: "query_user_roles",
			wantArgs: map[string]any{"baseId": "B", "targetUid": ""},
		},
		{
			name:     "query current user",
			args:     []string{"role-query-user", "--base-id", "B"},
			wantTool: "query_user_roles",
			wantArgs: map[string]any{"baseId": "B"},
		},
		{
			name:     "query target user",
			args:     []string{"role-query-user", "--base-id", "B", "--target-uid", "U"},
			wantTool: "query_user_roles",
			wantArgs: map[string]any{"baseId": "B", "targetUid": "U"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableTestCaller{responses: []string{`{"success":true,"data":{}}`}}
			if err := runAITableAdvpermCommand(t, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v", caller.calls)
			}
			if call := caller.calls[0]; call.server != "aitable" || call.tool != tc.wantTool || !reflect.DeepEqual(call.args, tc.wantArgs) {
				t.Fatalf("call = %#v, want aitable/%s %#v", call, tc.wantTool, tc.wantArgs)
			}
		})
	}
}

func TestCrossPlatformCoverageAITableAdvpermRoleMemberCommandsPropagateMCPFailures(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"add members", []string{"role-members-add", "--base-id", "B", "--role-id", "R", "--members", `[{"id":"U","memberType":"2"}]`}},
		{"replace members", []string{"role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "0"}},
		{"copy role", []string{"role-copy", "--base-id", "B", "--source-role-id", "R"}},
		{"query user roles", []string{"role-query-user", "--base-id", "B"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableTestCaller{errors: []error{errors.New("MCP unavailable")}}
			err := runAITableAdvpermCommand(t, caller, tc.args...)
			if err == nil || !strings.Contains(err.Error(), "MCP unavailable") {
				t.Fatalf("error = %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v", caller.calls)
			}
		})
	}
}

type aitableAdvpermDirectCaller struct {
	*aitableTestCaller
	resolveCalls int
}

func (c *aitableAdvpermDirectCaller) ResolveToolProduct(context.Context, []string, string) (string, error) {
	c.resolveCalls++
	return "", errors.New("unexpected compatibility discovery")
}

func TestCrossPlatformCoverageAITableAdvpermDirectRouteSafety(t *testing.T) {
	for _, tc := range []struct {
		tool string
		args []string
	}{
		{"add_role_members", []string{"role-members-add", "--base-id", "B", "--role-id", "R", "--members", `[{"id":"U","memberType":"2"}]`}},
		{"update_role_members", []string{"role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "0"}},
		{"copy_role", []string{"role-copy", "--base-id", "B", "--source-role-id", "R"}},
		{"query_user_roles", []string{"role-query-user", "--base-id", "B"}},
	} {
		for _, mode := range []string{"confirmed", "unconfirmed", "dry-run", "MCP failure"} {
			t.Run(tc.tool+"/"+mode, func(t *testing.T) {
				caller := &aitableAdvpermDirectCaller{aitableTestCaller: &aitableTestCaller{
					responses: []string{`{"success":true,"data":{"marker":"business"}}`}, dryRun: mode == "dry-run",
				}}
				InitDepsForTest(t, caller)
				deps.Out.w, deps.Out.errW = io.Discard, io.Discard
				cmd := newAitableCommand()
				installExampleGlobalFlags(cmd)
				cmd.SilenceErrors, cmd.SilenceUsage = true, true
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)
				args := append([]string{"advperm"}, tc.args...)
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
					caller.errors = []error{errors.New("MCP unavailable")}
					wantErr = "MCP unavailable"
				}
				cmd.SetArgs(args)
				err := corecmd.ExecuteContextForTest(cmd, context.Background())
				if wantErr == "confirmation_required" {
					if !apperrors.IsConfirmationRequired(err) {
						t.Fatalf("error = %v, want confirmation_required", err)
					}
				} else if (wantErr == "" && err != nil) || (wantErr != "" && (err == nil || !strings.Contains(err.Error(), wantErr))) {
					t.Fatalf("error = %v, want %q", err, wantErr)
				}
				if caller.resolveCalls != 0 || len(caller.calls) != wantCalls {
					t.Fatalf("discovery=%d calls=%#v, want zero discovery and %d calls", caller.resolveCalls, caller.calls, wantCalls)
				}
				if wantCalls == 1 && (caller.calls[0].server != "aitable" || caller.calls[0].tool != tc.tool) {
					t.Fatalf("call = %#v, want aitable/%s", caller.calls[0], tc.tool)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAITableAdvpermRoleMemberValidation(t *testing.T) {
	tooMany := make([]map[string]string, 201)
	for index := range tooMany {
		tooMany[index] = map[string]string{"id": string(rune('a' + index%26)), "memberType": "2"}
	}
	tooManyJSON, err := json.Marshal(tooMany)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"add missing members", []string{"role-members-add", "--base-id", "B", "--role-id", "R"}, "members"},
		{"add malformed members", []string{"role-members-add", "--base-id", "B", "--role-id", "R", "--members", "{}"}, "JSON"},
		{"add null members", []string{"role-members-add", "--base-id", "B", "--role-id", "R", "--members", "null"}, "不能是 null"},
		{"add null member", []string{"role-members-add", "--base-id", "B", "--role-id", "R", "--members", "[null]"}, "id 和 memberType"},
		{"add empty member", []string{"role-members-add", "--base-id", "B", "--role-id", "R", "--members", "[{}]"}, "id 和 memberType"},
		{"add too many members", []string{"role-members-add", "--base-id", "B", "--role-id", "R", "--members", string(tooManyJSON)}, "最多 200"},
		{"add numeric member type", []string{"role-members-add", "--base-id", "B", "--role-id", "R", "--members", `[{"id":"12345","memberType":2}]`}, "id 和 memberType"},
		{"add numeric member id", []string{"role-members-add", "--base-id", "B", "--role-id", "R", "--members", `[{"id":12345,"memberType":"2"}]`}, "id 和 memberType"},
		{"add missing base", []string{"role-members-add", "--role-id", "R", "--members", "[]"}, "base-id"},
		{"update missing members", []string{"role-members-update", "--base-id", "B", "--role-id", "R", "--default-role-mode", "0"}, "members"},
		{"update null members", []string{"role-members-update", "--base-id", "B", "--role-id", "R", "--members", "null", "--default-role-mode", "0"}, "不能是 null"},
		{"update missing default mode", []string{"role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]"}, "default-role-mode"},
		{"update missing base", []string{"role-members-update", "--role-id", "R", "--members", "[]", "--default-role-mode", "0"}, "base-id"},
		{"update invalid default mode", []string{"role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "3"}, "仅支持"},
		{"update missing custom default id", []string{"role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "1"}, "default-role-id"},
		{"update blank custom default id", []string{"role-members-update", "--base-id", "B", "--role-id", "R", "--members", "[]", "--default-role-mode", "1", "--default-role-id", "  "}, "default-role-id"},
		{"copy missing source role", []string{"role-copy", "--base-id", "B"}, "source-role-id"},
		{"copy missing base", []string{"role-copy", "--source-role-id", "R"}, "base-id"},
		{"query missing base", []string{"role-query-user"}, "base-id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableTestCaller{}
			err := runAITableAdvpermCommand(t, caller, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid invocation dispatched %#v", caller.calls)
			}
		})
	}
}
