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
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

type wikiCommandRunner struct {
	last executor.Invocation
}

func (r *wikiCommandRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	return executor.Result{Invocation: invocation}, nil
}

func TestWikiSpaceSearchWithKeywordUsesSearchTool(t *testing.T) {
	t.Parallel()

	runner := &wikiCommandRunner{}
	cmd := wikiHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"space", "search", "--keyword", "产品文档", "--limit", "5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "search_wikiSpaces" {
		t.Fatalf("tool = %q, want search_wikiSpaces", runner.last.Tool)
	}
	if got := runner.last.Params["keyword"]; got != "产品文档" {
		t.Fatalf("keyword = %#v, want 产品文档", got)
	}
	if got := runner.last.Params["pageSize"]; got != "5" {
		t.Fatalf("pageSize = %#v, want 5", got)
	}
}

func TestWikiSpaceSearchAcceptsWukongQueryAlias(t *testing.T) {
	t.Parallel()

	runner := &wikiCommandRunner{}
	cmd := wikiHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"space", "search", "--query", "产品文档", "--limit", "5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "search_wikiSpaces" {
		t.Fatalf("tool = %q, want search_wikiSpaces", runner.last.Tool)
	}
	if got := runner.last.Params["keyword"]; got != "产品文档" {
		t.Fatalf("keyword = %#v, want 产品文档", got)
	}
	if got := runner.last.Params["pageSize"]; got != "5" {
		t.Fatalf("pageSize = %#v, want 5", got)
	}
}

func TestWikiSpaceSearchMyWikiSpaceWithoutKeywordUsesListTool(t *testing.T) {
	t.Parallel()

	runner := &wikiCommandRunner{}
	cmd := wikiHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"space", "search", "--type", "myWikiSpace"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "list_wikiSpaces" {
		t.Fatalf("tool = %q, want list_wikiSpaces", runner.last.Tool)
	}
	if got := runner.last.Params["wikiSpaceType"]; got != "myWikiSpace" {
		t.Fatalf("wikiSpaceType = %#v, want myWikiSpace", got)
	}
}

func TestWikiSpaceSearchWithoutKeywordRejectsOtherTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{name: "no type", args: []string{"space", "search"}},
		{name: "org type", args: []string{"space", "search", "--type", "orgWikiSpace"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &wikiCommandRunner{}
			cmd := wikiHandler{}.Command(runner)
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute() error = nil, want keyword validation failure")
			}
			message := strings.ToLower(err.Error())
			if !strings.Contains(message, "keyword") || !strings.Contains(message, "query") {
				t.Fatalf("error = %q, want keyword and query hints", err.Error())
			}
			if runner.last.Tool != "" {
				t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
			}
		})
	}
}

func TestWikiSpaceGetWorkspaceAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{name: "workspace", args: []string{"space", "get", "--workspace", "WS_001"}},
		{name: "workspace-id", args: []string{"space", "get", "--workspace-id", "WS_001"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &wikiCommandRunner{}
			cmd := wikiHandler{}.Command(runner)
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs(tc.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
			}
			if runner.last.Tool != "get_wikiSpace" {
				t.Fatalf("tool = %q, want get_wikiSpace", runner.last.Tool)
			}
			if got := runner.last.Params["workspaceId"]; got != "WS_001" {
				t.Fatalf("workspaceId = %#v, want WS_001", got)
			}
		})
	}
}

func TestWikiSpaceCreateAcceptsWukongDescAlias(t *testing.T) {
	t.Parallel()

	runner := &wikiCommandRunner{}
	cmd := wikiHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"space", "create", "--name", "技术方案", "--desc", "团队技术方案归档"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "create_wikiSpace" {
		t.Fatalf("tool = %q, want create_wikiSpace", runner.last.Tool)
	}
	if got := runner.last.Params["description"]; got != "团队技术方案归档" {
		t.Fatalf("description = %#v, want 团队技术方案归档", got)
	}
}

func TestWikiSpaceListAcceptsWukongCursorAlias(t *testing.T) {
	t.Parallel()

	runner := &wikiCommandRunner{}
	cmd := wikiHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"space", "list", "--type", "orgWikiSpace", "--limit", "50", "--cursor", "TOKEN_001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "list_wikiSpaces" {
		t.Fatalf("tool = %q, want list_wikiSpaces", runner.last.Tool)
	}
	if got := runner.last.Params["wikiSpaceType"]; got != "orgWikiSpace" {
		t.Fatalf("wikiSpaceType = %#v, want orgWikiSpace", got)
	}
	if got := runner.last.Params["pageSize"]; got != "50" {
		t.Fatalf("pageSize = %#v, want 50", got)
	}
	if got := runner.last.Params["pageToken"]; got != "TOKEN_001" {
		t.Fatalf("pageToken = %#v, want TOKEN_001", got)
	}
}

func TestWikiMemberAddUsesWorkspaceIDAlias(t *testing.T) {
	t.Parallel()

	runner := &wikiCommandRunner{}
	cmd := wikiHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"member", "add",
		"--workspace-id", "WS_001",
		"--user", "uid1,uid2",
		"--role", "reader",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "add_member" {
		t.Fatalf("tool = %q, want add_member", runner.last.Tool)
	}
	if got := runner.last.Params["workspaceId"]; got != "WS_001" {
		t.Fatalf("workspaceId = %#v, want WS_001", got)
	}
	if got := runner.last.Params["roleId"]; got != "READER" {
		t.Fatalf("roleId = %#v, want READER", got)
	}
	users, ok := runner.last.Params["userIds"].([]string)
	if !ok {
		t.Fatalf("userIds type = %T, want []string", runner.last.Params["userIds"])
	}
	if strings.Join(users, ",") != "uid1,uid2" {
		t.Fatalf("userIds = %#v, want uid1,uid2", users)
	}
}

func TestWikiMemberUpdateAcceptsWukongUsersAlias(t *testing.T) {
	t.Parallel()

	runner := &wikiCommandRunner{}
	cmd := wikiHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"member", "update",
		"--workspace", "WS_001",
		"--users", "uid1,uid2",
		"--role", "editor",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "update_member" {
		t.Fatalf("tool = %q, want update_member", runner.last.Tool)
	}
	if got := runner.last.Params["roleId"]; got != "EDITOR" {
		t.Fatalf("roleId = %#v, want EDITOR", got)
	}
	users, ok := runner.last.Params["userIds"].([]string)
	if !ok {
		t.Fatalf("userIds type = %T, want []string", runner.last.Params["userIds"])
	}
	if strings.Join(users, ",") != "uid1,uid2" {
		t.Fatalf("userIds = %#v, want uid1,uid2", users)
	}
}

func TestWikiMemberListLimitAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		flag string
		want int
	}{
		{name: "limit primary", flag: "--limit", want: 50},
		{name: "max-results alias", flag: "--max-results", want: 40},
		{name: "page-size alias", flag: "--page-size", want: 30},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &wikiCommandRunner{}
			cmd := wikiHandler{}.Command(runner)
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs([]string{"member", "list", "--workspace", "WS_001", tc.flag, strconv.Itoa(tc.want)})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
			}
			if runner.last.Tool != "list_member" {
				t.Fatalf("tool = %q, want list_member", runner.last.Tool)
			}
			if got := runner.last.Params["workspaceId"]; got != "WS_001" {
				t.Fatalf("workspaceId = %#v, want WS_001", got)
			}
			if got := runner.last.Params["maxResults"]; got != tc.want {
				t.Fatalf("maxResults = %#v, want %d", got, tc.want)
			}
		})
	}
}

func TestWikiMemberListRejectsRemovedAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		flag string
	}{
		{
			name: "maxresults",
			args: []string{"member", "list", "--workspace", "WS_001", "--maxresults", "10"},
			flag: "maxresults",
		},
		{
			name: "id",
			args: []string{"member", "list", "--id", "WS_001"},
			flag: "id",
		},
		{
			name: "space",
			args: []string{"member", "list", "--space", "WS_001"},
			flag: "space",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &wikiCommandRunner{}
			cmd := wikiHandler{}.Command(runner)
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute() error = nil, want unknown flag")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "unknown flag: --"+tc.flag) {
				t.Fatalf("error = %q, want unknown flag for --%s", err.Error(), tc.flag)
			}
			if runner.last.Tool != "" {
				t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
			}
		})
	}
}
