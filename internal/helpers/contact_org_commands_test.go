// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type contactOrgTestCall struct {
	tool string
	args map[string]any
}

type contactOrgTestCaller struct {
	calls []contactOrgTestCall
}

func (c *contactOrgTestCaller) CallTool(_ context.Context, _ string, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, contactOrgTestCall{tool: tool, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*contactOrgTestCaller) Format() string { return "json" }
func (*contactOrgTestCaller) DryRun() bool   { return false }
func (*contactOrgTestCaller) Fields() string { return "" }
func (*contactOrgTestCaller) JQ() string     { return "" }

func executeContactOrgTestCommand(t *testing.T, caller *contactOrgTestCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard

	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newContactCommand())
	root.SetArgs(append([]string{"contact"}, args...))
	return root.Execute()
}

func TestContactOrganizationCommandsMapWukongContracts(t *testing.T) {
	caller := &contactOrgTestCaller{}
	if err := executeContactOrgTestCommand(t, caller, "org", "create", "--org-name", "Acme", "--creator-username", "Alice"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := executeContactOrgTestCommand(t, caller, "user", "invite", "--org-user-name", "Bob", "--org-user-mobile", "13800138000", "--depts", `[{"deptId":1}]`); err != nil {
		t.Fatalf("user invite: %v", err)
	}
	if err := executeContactOrgTestCommand(t, caller, "account", "create", "--org-user-name", "Carol", "--login-id", "carol001", "--dept-ids", "1,2", "--send-pwd-via-sms"); err != nil {
		t.Fatalf("account create: %v", err)
	}

	want := []contactOrgTestCall{
		{tool: "org_create", args: map[string]any{"orgName": "Acme", "creatorUsername": "Alice"}},
		{tool: "add_employee", args: map[string]any{"orgUserName": "Bob", "orgUserMobile": "13800138000", "depts": []map[string]any{{"deptId": float64(1)}}}},
		{tool: "exclusive_account_create", args: map[string]any{"orgUserName": "Carol", "loginId": "carol001", "sendPwdViaSms": true, "deptIds": []int64{1, 2}}},
	}
	if !reflect.DeepEqual(caller.calls, want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestContactOrganizationCommandsRejectMissingRequiredFlags(t *testing.T) {
	caller := &contactOrgTestCaller{}
	if err := executeContactOrgTestCommand(t, caller, "org", "create", "--org-name", "Acme"); err == nil {
		t.Fatal("org create without --creator-username unexpectedly succeeded")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote calls = %#v, want none", caller.calls)
	}
}
