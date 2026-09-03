// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type wikiCoverageCall struct {
	product string
	tool    string
	args    map[string]any
}

type wikiCoverageCaller struct {
	responses map[string][]string
	errors    map[string][]error
	indexes   map[string]int
	calls     []wikiCoverageCall
	dryRun    bool
}

func (c *wikiCoverageCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	key := product + "/" + tool
	c.calls = append(c.calls, wikiCoverageCall{product: product, tool: tool, args: args})
	if c.indexes == nil {
		c.indexes = map[string]int{}
	}
	index := c.indexes[key]
	c.indexes[key]++
	if sequence := c.errors[key]; index < len(sequence) && sequence[index] != nil {
		return nil, sequence[index]
	}
	sequence := c.responses[key]
	if len(sequence) == 0 {
		return nil, fmt.Errorf("unexpected MCP call %s", key)
	}
	if index >= len(sequence) {
		index = len(sequence) - 1
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: sequence[index]}}}, nil
}

func (c *wikiCoverageCaller) Format() string { return "json" }
func (c *wikiCoverageCaller) DryRun() bool   { return c.dryRun }
func (c *wikiCoverageCaller) Fields() string { return "" }
func (c *wikiCoverageCaller) JQ() string     { return "" }

func runWikiCoverageCLI(t *testing.T, caller *wikiCoverageCaller, args ...string) (map[string]any, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().String("jq", "", "")
	root.PersistentFlags().String("fields", "", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.AddCommand(shortcut.Commands()...)
	root.SetIn(strings.NewReader(""))
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"wiki"}, args...))
	executed, err := root.ExecuteC()
	if err == nil {
		if _, _, emitErr := output.EmitStoredResult(executed); emitErr != nil {
			err = emitErr
		}
	}
	if stdout.Len() == 0 {
		return nil, err
	}
	var payload map[string]any
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), decodeErr)
	}
	if data, ok := payload["data"].(map[string]any); ok {
		return data, err
	}
	return payload, err
}

func TestCrossPlatformCoverageWikiSpaceWorkflows(t *testing.T) {
	t.Run("list default string limit", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/list_wikiSpaces": {`{"success":true,"result":{"wikiSpaces":[{"workspaceId":"w1","name":"Docs","wikiSpaceType":"orgWikiSpace"}],"hasMore":false}}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-list")
		spaces, _ := out["spaces"].([]any)
		space, _ := spaces[0].(map[string]any)
		if err != nil || out["count"] != float64(1) || out["requestedType"] != "orgWikiSpace" || space["spaceType"] != "orgWikiSpace" || caller.calls[0].args["pageSize"] != 20 {
			t.Fatalf("list output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("list explicit legacy string limit and cursor", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/list_wikiSpaces": {`{"wikiSpaces":[],"hasMore":false}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-list", "--type", "myWikiSpace", "--limit", "7", "--page-token", "next")
		if err != nil || out["count"] != float64(0) || out["requestedType"] != "myWikiSpace" || caller.calls[0].args["pageSize"] != 7 || caller.calls[0].args["pageToken"] != "next" {
			t.Fatalf("list output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	for _, value := range []string{"bad", "0", "51"} {
		t.Run("list rejects "+value, func(t *testing.T) {
			caller := &wikiCoverageCaller{}
			if _, err := runWikiCoverageCLI(t, caller, "+space-list", "--limit", value); err == nil || len(caller.calls) != 0 {
				t.Fatalf("limit %q err=%v calls=%#v", value, err, caller.calls)
			}
		})
	}

	t.Run("search parses string limit", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/search_wikiSpaces": {`{"success":true,"wikiSpaces":[{"spaceId":"w2","spaceName":"Plan"}]}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-search", "--query", "Plan", "--limit", "12")
		if err != nil || out["count"] != float64(1) || len(caller.calls) != 1 {
			t.Fatalf("search output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
		if got := caller.calls[0]; got.product != "wiki" || got.tool != "search_wikiSpaces" || len(got.args) != 2 || got.args["keyword"] != "Plan" || got.args["pageSize"] != 12 {
			t.Fatalf("search call = %#v, want exact keyword/pageSize request", got)
		}
		if _, exists := caller.calls[0].args["query"]; exists {
			t.Fatalf("search request leaked compatibility property query: %#v", caller.calls[0].args)
		}
		if _, exists := caller.calls[0].args["limit"]; exists {
			t.Fatalf("search request leaked compatibility property limit: %#v", caller.calls[0].args)
		}
	})

	t.Run("get requires business id", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{"wiki/get_wikiSpace": {`{"success":true,"result":{"name":"missing id"}}`}}}
		if _, err := runWikiCoverageCLI(t, caller, "+space-get", "--workspace", "w"); err == nil {
			t.Fatal("space without id succeeded")
		}
	})

	t.Run("create dry-run", func(t *testing.T) {
		caller := &wikiCoverageCaller{}
		out, err := runWikiCoverageCLI(t, caller, "+space-create", "--name", "Docs", "--desc", "D", "--icon", "I", "--dry-run")
		if err != nil || out["executed"] != false || len(caller.calls) != 0 {
			t.Fatalf("dry-run output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("create and exact readback", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/create_wikiSpace": {`{"success":true,"result":{"workspaceId":"w3"}}`},
			"wiki/get_wikiSpace":    {`{"success":true,"data":{"workspaceId":"w3","name":"Docs","wikiSpaceType":"orgWikiSpace"}}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-create", "--name", "Docs")
		if err != nil || out["workspaceId"] != "w3" || out["spaceType"] != "orgWikiSpace" || out["spaceTypeVerified"] != true || out["spaceTypeEvidence"] != "get_wikiSpace.wikiSpaceType" || len(caller.calls) != 2 {
			t.Fatalf("create output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("create verifies type through scoped paginated lists", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/create_wikiSpace": {`{"success":true,"result":{"workspaceId":"w3"}}`},
			"wiki/get_wikiSpace":    {`{"success":true,"data":{"workspaceId":"w3","name":"Docs"}}`},
			"wiki/list_wikiSpaces": {
				`{"wikiSpaces":[],"hasMore":true,"nextCursor":"org-2"}`,
				`{"wikiSpaces":[],"hasMore":false}`,
				`{"wikiSpaces":[{"workspaceId":"w3","name":"Docs"}],"hasMore":false}`,
			},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-create", "--name", "Docs")
		if err != nil || out["spaceType"] != "myWikiSpace" || out["spaceTypeVerified"] != true || out["spaceTypeEvidence"] != "list_wikiSpaces.myWikiSpace.workspaceId_match" {
			t.Fatalf("create scoped type output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
		if len(caller.calls) != 5 || caller.calls[2].args["wikiSpaceType"] != "orgWikiSpace" || caller.calls[3].args["pageToken"] != "org-2" || caller.calls[4].args["wikiSpaceType"] != "myWikiSpace" {
			t.Fatalf("create scoped type calls=%#v", caller.calls)
		}
	})

	t.Run("create preserves id when type cannot be verified", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/create_wikiSpace": {`{"success":true,"result":{"workspaceId":"w3"}}`},
			"wiki/get_wikiSpace":    {`{"success":true,"data":{"workspaceId":"w3","name":"Docs"}}`},
			"wiki/list_wikiSpaces": {
				`{"wikiSpaces":[],"hasMore":false}`,
				`{"wikiSpaces":[],"hasMore":false}`,
			},
		}}
		_, err := runWikiCoverageCLI(t, caller, "+space-create", "--name", "Docs")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "space_type_unverified" || typed.Details["workspaceId"] != "w3" || typed.Details["spaceTypeVerified"] != false || typed.Details["status"] != "partial_success" {
			t.Fatalf("unverified type err=%#v calls=%#v", err, caller.calls)
		}
	})

	t.Run("create rejects conflicting scoped type evidence", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/create_wikiSpace": {`{"success":true,"result":{"workspaceId":"w3"}}`},
			"wiki/get_wikiSpace":    {`{"success":true,"data":{"workspaceId":"w3","name":"Docs"}}`},
			"wiki/list_wikiSpaces": {
				`{"wikiSpaces":[{"workspaceId":"w3"}],"hasMore":false}`,
				`{"wikiSpaces":[{"workspaceId":"w3"}],"hasMore":false}`,
			},
		}}
		_, err := runWikiCoverageCLI(t, caller, "+space-create", "--name", "Docs")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "space_type_unverified" || typed.Details["workspaceId"] != "w3" || !strings.Contains(fmt.Sprint(typed.Details["verificationFailure"]), "同时出现在") {
			t.Fatalf("conflicting type err=%#v calls=%#v", err, caller.calls)
		}
	})

	t.Run("delete preflight dry-run and terminal", func(t *testing.T) {
		dry := &wikiCoverageCaller{responses: map[string][]string{"wiki/get_wikiSpace": {`{"workspaceId":"w4","name":"Docs"}`}}}
		out, err := runWikiCoverageCLI(t, dry, "+delete-space", "--workspace", "w4", "--dry-run", "--yes")
		if err != nil || out["executed"] != false || len(dry.calls) != 1 {
			t.Fatalf("delete dry-run output=%#v err=%v calls=%#v", out, err, dry.calls)
		}
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/get_wikiSpace":    {`{"workspaceId":"w4","name":"Docs"}`},
			"wiki/delete_wikiSpace": {`{"success":true}`},
		}}
		out, err = runWikiCoverageCLI(t, caller, "+space-delete", "--workspace", "w4", "--yes")
		if err != nil || out["deleted"] != true || len(caller.calls) != 2 {
			t.Fatalf("delete output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	for _, args := range [][]string{{"--name", strings.Repeat("界", 33)}, {"--name", "x", "--desc", strings.Repeat("界", 501)}} {
		caller := &wikiCoverageCaller{}
		if _, err := runWikiCoverageCLI(t, caller, append([]string{"+space-create"}, args...)...); err == nil || len(caller.calls) != 0 {
			t.Fatalf("invalid create args succeeded: %v", args)
		}
	}
}

func TestCrossPlatformCoverageWikiSpaceTypeVerificationFailureModes(t *testing.T) {
	runCreate := func(t *testing.T, caller *wikiCoverageCaller, want string) {
		t.Helper()
		_, err := runWikiCoverageCLI(t, caller, "+space-create", "--name", "Docs")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "space_type_unverified" || !strings.Contains(fmt.Sprint(typed.Details["verificationFailure"]), want) {
			t.Fatalf("verification error=%#v details=%#v, want %q", err, typed, want)
		}
	}

	t.Run("conflicting direct type fields", func(t *testing.T) {
		runCreate(t, &wikiCoverageCaller{responses: map[string][]string{
			"wiki/create_wikiSpace": {`{"success":true,"workspaceId":"w"}`},
			"wiki/get_wikiSpace":    {`{"workspaceId":"w","spaceType":"orgWikiSpace","wikiSpaceType":"myWikiSpace"}`},
		}}, "互相冲突")
	})
	t.Run("scoped list transport error", func(t *testing.T) {
		runCreate(t, &wikiCoverageCaller{
			responses: map[string][]string{
				"wiki/create_wikiSpace": {`{"success":true,"workspaceId":"w"}`},
				"wiki/get_wikiSpace":    {`{"workspaceId":"w","name":"Docs"}`},
			},
			errors: map[string][]error{"wiki/list_wikiSpaces": {errors.New("scoped list failed")}},
		}, "scoped list failed")
	})

	for _, tc := range []struct {
		name      string
		responses []string
		want      string
	}{
		{name: "malformed scoped collection", responses: []string{`{"success":true}`}, want: "缺少声明的业务数组"},
		{name: "missing has more", responses: []string{`{"wikiSpaces":[]}`}, want: "提供 hasMore"},
		{name: "malformed has more", responses: []string{`{"wikiSpaces":[],"hasMore":"yes"}`}, want: "不是布尔值"},
		{name: "missing next cursor", responses: []string{`{"wikiSpaces":[],"hasMore":true}`}, want: "缺少游标"},
		{name: "stalled cursor", responses: []string{
			`{"wikiSpaces":[],"hasMore":true,"nextCursor":"same"}`,
			`{"wikiSpaces":[],"hasMore":true,"nextCursor":"same"}`,
		}, want: "未变化"},
		{name: "cyclic cursor", responses: []string{
			`{"wikiSpaces":[],"hasMore":true,"nextCursor":"first"}`,
			`{"wikiSpaces":[],"hasMore":true,"nextCursor":"second"}`,
			`{"wikiSpaces":[],"hasMore":true,"nextCursor":"first"}`,
		}, want: "形成循环"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runCreate(t, &wikiCoverageCaller{responses: map[string][]string{
				"wiki/create_wikiSpace": {`{"success":true,"workspaceId":"w"}`},
				"wiki/get_wikiSpace":    {`{"workspaceId":"w","name":"Docs"}`},
				"wiki/list_wikiSpaces":  tc.responses,
			}}, tc.want)
		})
	}

	t.Run("scoped list page limit", func(t *testing.T) {
		pages := make([]string, 20)
		for index := range pages {
			pages[index] = fmt.Sprintf(`{"wikiSpaces":[],"hasMore":true,"nextCursor":"page-%02d"}`, index+1)
		}
		runCreate(t, &wikiCoverageCaller{responses: map[string][]string{
			"wiki/create_wikiSpace": {`{"success":true,"workspaceId":"w"}`},
			"wiki/get_wikiSpace":    {`{"workspaceId":"w","name":"Docs"}`},
			"wiki/list_wikiSpaces":  pages,
		}}, "超过 20 页")
	})
}

func TestCrossPlatformCoverageWikiNodeSearchAutoPagination(t *testing.T) {
	caller := &wikiCoverageCaller{responses: map[string][]string{
		"doc/search_documents": {
			`{"documents":[{"nodeId":"n1","name":"Plan A"}],"hasMore":true,"nextCursor":"cursor-2"}`,
			`{"documents":[{"nodeId":"n2","name":"Plan B"}],"hasMore":false}`,
		},
	}}
	out, err := runWikiCoverageCLI(t, caller, "+node-search", "--workspace", "w1", "--query", "Plan", "--extensions", "adoc,txt", "--limit", "1", "--page-all", "--page-limit", "2")
	if err != nil || out["count"] != float64(2) || out["autoPageComplete"] != true {
		t.Fatalf("node-search output=%#v err=%v calls=%#v", out, err, caller.calls)
	}
	if len(caller.calls) != 2 || caller.calls[1].args["pageToken"] != "cursor-2" {
		t.Fatalf("node-search pagination calls=%#v", caller.calls)
	}
	for index, call := range caller.calls {
		if call.product != "doc" || call.tool != "search_documents" || call.args["pageSize"] != 1 {
			t.Fatalf("node-search call[%d]=%#v", index, call)
		}
		extensions, ok := call.args["extensions"].([]string)
		if !ok || len(extensions) != 2 || extensions[0] != "adoc" || extensions[1] != "txt" {
			t.Fatalf("node-search extensions[%d]=%#v", index, call.args["extensions"])
		}
	}

	for _, value := range []string{"0", "31"} {
		caller := &wikiCoverageCaller{}
		if _, err := runWikiCoverageCLI(t, caller, "+node-search", "--workspace", "w1", "--query", "Plan", "--limit", value); err == nil || len(caller.calls) != 0 {
			t.Fatalf("node-search limit %s accepted: err=%v calls=%#v", value, err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageWikiAutoPaginationRejectsCursorCycles(t *testing.T) {
	caller := &wikiCoverageCaller{responses: map[string][]string{
		"doc/search_documents": {
			`{"documents":[],"hasMore":true,"nextCursor":"cursor-a"}`,
			`{"documents":[],"hasMore":true,"nextCursor":"cursor-b"}`,
			`{"documents":[],"hasMore":true,"nextCursor":"cursor-a"}`,
		},
	}}
	_, err := runWikiCoverageCLI(t, caller, "+node-search", "--workspace", "w1", "--query", "Plan", "--page-all", "--page-limit", "4")
	if err == nil || !strings.Contains(err.Error(), "游标形成循环") || len(caller.calls) != 3 {
		t.Fatalf("cursor cycle err=%v calls=%#v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageWikiMemberWritesUseTerminalEvidenceOnly(t *testing.T) {
	for _, tc := range []struct {
		command string
		tool    string
		extra   []string
	}{
		{command: "+member-add", tool: "add_member", extra: []string{"--role", "READER"}},
		{command: "+member-update", tool: "update_member", extra: []string{"--role", "EDITOR"}},
		{command: "+member-remove", tool: "remove_member"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			caller := &wikiCoverageCaller{responses: map[string][]string{"wiki/" + tc.tool: {`{"success":true}`}}}
			args := append([]string{tc.command, "--workspace", "w", "--users", "u1,u2"}, tc.extra...)
			out, err := runWikiCoverageCLI(t, caller, args...)
			verification, _ := out["verification"].(map[string]any)
			if err != nil || out["verifiedBy"] != "write_terminal_success" || verification["readbackAvailable"] != false || len(caller.calls) != 1 {
				t.Fatalf("member output=%#v err=%v calls=%#v", out, err, caller.calls)
			}
			if caller.calls[0].tool == "list_member" {
				t.Fatal("member write used capped list as readback")
			}
			if tc.tool != "remove_member" && caller.calls[0].args["roleId"] != strings.ToUpper(tc.extra[1]) {
				t.Fatalf("role was not normalized: %#v", caller.calls[0].args)
			}
		})
	}

	t.Run("dry-run has no write", func(t *testing.T) {
		caller := &wikiCoverageCaller{}
		out, err := runWikiCoverageCLI(t, caller, "+member-add", "--workspace", "w", "--users", "u", "--role", "READER", "--dry-run")
		if err != nil || out["executed"] != false || len(caller.calls) != 0 {
			t.Fatalf("dry-run output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("rejects missing terminal evidence", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{"wiki/add_member": {`{"result":{"accepted":true}}`}}}
		if _, err := runWikiCoverageCLI(t, caller, "+member-add", "--workspace", "w", "--users", "u", "--role", "READER"); err == nil {
			t.Fatal("member write without success=true succeeded")
		}
	})

	many := make([]string, 31)
	for index := range many {
		many[index] = fmt.Sprintf("u%d", index)
	}
	caller := &wikiCoverageCaller{}
	if _, err := runWikiCoverageCLI(t, caller, "+member-remove", "--workspace", "w", "--users", strings.Join(many, ",")); err == nil || len(caller.calls) != 0 {
		t.Fatal("more than 30 members reached MCP")
	}
}

func TestCrossPlatformCoverageWikiMemberListContracts(t *testing.T) {
	caller := &wikiCoverageCaller{responses: map[string][]string{
		"wiki/list_member": {`{"success":true,"members":[{"userId":"u1","nick":"A","roleId":"READER","type":"USER","outer":false}],"truncated":false}`},
	}}
	out, err := runWikiCoverageCLI(t, caller, "+member-list", "--workspace", "w", "--limit", "50", "--filter-role", "READER,EDITOR")
	if err != nil || out["count"] != float64(1) || caller.calls[0].args["maxResults"] != 50 {
		t.Fatalf("member list output=%#v err=%v calls=%#v", out, err, caller.calls)
	}
	for _, value := range []string{"0", "51"} {
		caller = &wikiCoverageCaller{}
		if _, err := runWikiCoverageCLI(t, caller, "+member-list", "--workspace", "w", "--limit", value); err == nil || len(caller.calls) != 0 {
			t.Fatalf("invalid member limit %s reached MCP", value)
		}
	}
}

func TestCrossPlatformCoverageWikiReadWorkflows(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		responses map[string][]string
		wantTool  string
		wantCount float64
	}{
		{name: "node list", args: []string{"+node-list", "--workspace", "w", "--folder", "f", "--limit", "2", "--cursor", "c"}, responses: map[string][]string{"doc/list_nodes": {`{"success":true,"nodes":[{"id":"n","title":"Doc","parentId":"f","spaceId":"w"}],"hasMore":false}`}}, wantTool: "list_nodes", wantCount: 1},
		{name: "node search", args: []string{"+node-search", "--workspace", "w", "--query", "Doc", "--extensions", "adoc", "--limit", "2", "--cursor", "c"}, responses: map[string][]string{"doc/search_documents": {`{"success":true,"documents":[],"hasMore":false}`}}, wantTool: "search_documents", wantCount: 0},
		{name: "feed list", args: []string{"+feed-list", "--workspace", "w", "--limit", "2", "--cursor", "c", "--exclude-file"}, responses: map[string][]string{"wiki/list_workspace_feeds": {`{"success":true,"feeds":[{"feedId":"x","feedType":"update","fileId":"n"}],"hasMore":false}`}}, wantTool: "list_workspace_feeds", wantCount: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &wikiCoverageCaller{responses: tc.responses}
			out, err := runWikiCoverageCLI(t, caller, tc.args...)
			if err != nil || out["count"] != tc.wantCount || caller.calls[0].tool != tc.wantTool {
				t.Fatalf("output=%#v err=%v calls=%#v", out, err, caller.calls)
			}
		})
	}

	t.Run("node get", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"success":true,"result":{"nodeId":"n","title":"Doc","nodeType":"file","extension":"adoc","parentId":"f","hasChildren":false,"futureField":"keep"}}`}}}
		out, err := runWikiCoverageCLI(t, caller, "+node-get", "--node", "n")
		if err != nil || out["nodeId"] != "n" || out["name"] != "Doc" || out["type"] != "file" || out["extension"] != "adoc" || out["parentFolderId"] != "f" || out["hasChildren"] != false || out["futureField"] != "keep" {
			t.Fatalf("node get output=%#v err=%v", out, err)
		}
	})

	t.Run("node list preserves type and hierarchy evidence", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"doc/list_nodes": {`{"nodes":[{"nodeId":"folder","name":"资料区","nodeType":"folder","hasChildren":true,"workspaceId":"w"},{"nodeId":"doc","name":"方案","nodeType":"file","extension":"adoc","parentId":"folder","hasChildren":false,"workspaceId":"w"}],"hasMore":false}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+node-list", "--workspace", "w")
		if err != nil || out["count"] != float64(2) {
			t.Fatalf("node list output=%#v err=%v", out, err)
		}
		nodes := out["nodes"].([]any)
		folder := nodes[0].(map[string]any)
		doc := nodes[1].(map[string]any)
		if folder["type"] != "folder" || folder["hasChildren"] != true || doc["extension"] != "adoc" || doc["parentFolderId"] != "folder" || doc["hasChildren"] != false {
			t.Fatalf("node hierarchy evidence folder=%#v doc=%#v", folder, doc)
		}
	})
}

func TestCrossPlatformCoverageWikiFeedListPreservesBusinessPayload(t *testing.T) {
	caller := &wikiCoverageCaller{responses: map[string][]string{
		"wiki/list_workspace_feeds": {`{"success":true,"feeds":[{"type":1,"time":1750067400000,"content":{"doc":{"name":"Important","docKey":"dk1"}},"users":[{"nick":"Alice","userId":"u1"}],"futureField":{"value":"keep"}}],"hasMore":false}`},
	}}
	out, err := runWikiCoverageCLI(t, caller, "+feed-list", "--workspace", "w")
	if err != nil {
		t.Fatal(err)
	}
	feeds := out["feeds"].([]any)
	feed := feeds[0].(map[string]any)
	content, contentOK := feed["content"].(map[string]any)
	users, usersOK := feed["users"].([]any)
	if !contentOK || content["doc"].(map[string]any)["name"] != "Important" {
		t.Fatalf("content was lost or changed: %#v", feed)
	}
	if !usersOK || users[0].(map[string]any)["nick"] != "Alice" {
		t.Fatalf("users were lost or changed: %#v", feed)
	}
	if _, ok := feed["futureField"]; !ok {
		t.Fatalf("unknown feed business fields must be preserved: %#v", feed)
	}
}

func TestCrossPlatformCoverageWikiWriteWorkflows(t *testing.T) {
	t.Run("confirmation gates precede every remote call", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			args []string
		}{
			{name: "delete space", args: []string{"+delete-space", "--workspace", "w"}},
			{name: "copy node", args: []string{"+node-copy", "--workspace", "w", "--node", "source"}},
			{name: "move node", args: []string{"+move", "--workspace", "target", "--node", "n"}},
			{name: "move node to drive", args: []string{"+move-to-drive", "--node", "n"}},
			{name: "delete node", args: []string{"+node-delete", "--workspace", "w", "--node", "n"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				caller := &wikiCoverageCaller{}
				_, err := runWikiCoverageCLI(t, caller, tc.args...)
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Reason != "confirmation_required" {
					t.Fatalf("unconfirmed error = %#v, want confirmation_required", err)
				}
				if len(caller.calls) != 0 {
					t.Fatalf("unconfirmed shortcut reached MCP: %#v", caller.calls)
				}
			})
		}
	})

	t.Run("node create dry and verified", func(t *testing.T) {
		dry := &wikiCoverageCaller{}
		out, err := runWikiCoverageCLI(t, dry, "+node-create", "--workspace", "w", "--folder", "f", "--name", "Doc", "--type", "adoc", "--dry-run")
		if err != nil || out["executed"] != false || len(dry.calls) != 0 {
			t.Fatalf("create dry output=%#v err=%v calls=%#v", out, err, dry.calls)
		}
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"doc/create_file":       {`{"success":true,"data":{"fileId":"n"}}`},
			"doc/get_document_info": {`{"success":true,"nodeId":"n","workspaceId":"w","folderId":"f","name":"Doc","nodeType":"file","extension":"adoc"}`},
		}}
		out, err = runWikiCoverageCLI(t, caller, "+node-create", "--workspace", "w", "--folder", "f", "--name", "Doc")
		if err != nil || out["nodeId"] != "n" || out["workspaceId"] != "w" || out["requestedType"] != "adoc" || len(caller.calls) != 2 {
			t.Fatalf("create output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("node copy dry and verified", func(t *testing.T) {
		dry := &wikiCoverageCaller{}
		out, err := runWikiCoverageCLI(t, dry, "+node-copy", "--workspace", "w", "--folder", "f", "--node", "source", "--dry-run", "--yes")
		if err != nil || out["executed"] != false || len(dry.calls) != 0 {
			t.Fatalf("copy dry output=%#v err=%v calls=%#v", out, err, dry.calls)
		}
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"doc/copy_document": {`{"success":true,"nodeId":"copy"}`},
			"doc/get_document_info": {
				`{"success":true,"nodeId":"source","workspaceId":"source-w","name":"Doc","extension":"adoc"}`,
				`{"success":true,"nodeId":"copy","workspaceId":"w","name":"Doc 副本","extension":"adoc"}`,
			},
		}}
		out, err = runWikiCoverageCLI(t, caller, "+node-copy", "--workspace", "w", "--node", "source", "--yes")
		if err != nil || out["nodeId"] != "copy" || out["sourceNodeId"] != "source" || out["targetWorkspaceId"] != "w" || len(caller.calls) != 3 {
			t.Fatalf("copy output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("move within wiki and to drive", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old","folderId":"old-f"}`, `{"nodeId":"n","workspaceId":"target","parentFolderId":"f"}`},
			"doc/move_document":     {`{"success":true}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+move", "--workspace", "target", "--folder", "f", "--node", "n", "--yes")
		if err != nil || out["nodeId"] != "n" || out["targetFolderId"] != "f" || len(caller.calls) != 3 {
			t.Fatalf("move output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
		caller = &wikiCoverageCaller{responses: map[string][]string{
			"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"nodeId":"n","workspaceId":"drive"}`},
			"doc/move_document":     {`{"success":true}`},
			"wiki/list_wikiSpaces":  {`{"wikiSpaces":[{"workspaceId":"drive","name":"我的文档"}],"hasMore":false}`},
		}}
		out, err = runWikiCoverageCLI(t, caller, "+move-to-drive", "--workspace", "old", "--node", "n", "--yes")
		if err != nil || out["nodeId"] != "n" || out["targetDomain"] != "my_documents" || out["sourceWorkspaceId"] != "old" || out["targetWorkspaceId"] != "drive" || len(caller.calls) != 4 {
			t.Fatalf("move-to-drive output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("move to drive follows my documents space pagination", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"nodeId":"n","workspaceId":"drive","folderId":"root"}`},
			"doc/move_document":     {`{"success":true}`},
			"wiki/list_wikiSpaces": {
				`{"wikiSpaces":[{"workspaceId":"other"}],"hasMore":true,"nextCursor":"next"}`,
				`{"wikiSpaces":[{"workspaceId":"drive","name":"我的文档"}],"hasMore":false}`,
			},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+move-to-drive", "--node", "n", "--yes")
		if err != nil || out["targetWorkspaceId"] != "drive" || out["targetFolderId"] != "root" || len(caller.calls) != 5 || caller.calls[4].args["pageToken"] != "next" {
			t.Fatalf("paginated move-to-drive output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("move dry-run only preflights", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`}}}
		out, err := runWikiCoverageCLI(t, caller, "+move", "--workspace", "target", "--node", "n", "--dry-run", "--yes")
		if err != nil || out["executed"] != false || len(caller.calls) != 1 {
			t.Fatalf("move dry output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("node delete dry and terminal", func(t *testing.T) {
		dry := &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"w"}`}}}
		out, err := runWikiCoverageCLI(t, dry, "+node-delete", "--workspace", "w", "--node", "n", "--dry-run", "--yes")
		if err != nil || out["executed"] != false || len(dry.calls) != 1 {
			t.Fatalf("delete dry output=%#v err=%v calls=%#v", out, err, dry.calls)
		}
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"doc/get_document_info": {`{"nodeId":"n","workspaceId":"w"}`},
			"doc/delete_document":   {`{"success":true}`},
		}}
		out, err = runWikiCoverageCLI(t, caller, "+node-delete", "--workspace", "w", "--node", "n", "--yes")
		if err != nil || out["deleted"] != true || len(caller.calls) != 2 {
			t.Fatalf("delete output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})
}

func TestCrossPlatformCoverageWikiNodeVerificationEdges(t *testing.T) {
	if got := wikiNodeType(map[string]any{"type": "FOLDER"}); got != "folder" {
		t.Fatalf("fallback node type = %q", got)
	}
	if err := requireWikiNodeIdentity(map[string]any{"name": "Doc"}, "doc/get_document_info", "n"); err == nil || !strings.Contains(err.Error(), "缺少 nodeId") {
		t.Fatalf("missing node identity error = %v", err)
	}

	assertErr := func(name string, caller *wikiCoverageCaller, args ...string) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			if _, err := runWikiCoverageCLI(t, caller, args...); err == nil {
				t.Fatalf("%s unexpectedly succeeded; calls=%#v", name, caller.calls)
			}
		})
	}

	copyArgs := []string{"+node-copy", "--workspace", "w", "--node", "source", "--yes"}
	assertErr("copy source identity mismatch", &wikiCoverageCaller{responses: map[string][]string{
		"doc/get_document_info": {`{"nodeId":"other","name":"Doc"}`},
	}}, copyArgs...)
	assertErr("copy source workspace missing", &wikiCoverageCaller{responses: map[string][]string{
		"doc/get_document_info": {`{"nodeId":"source","name":"Doc"}`},
	}}, copyArgs...)

	t.Run("copy reports verified target folder", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"doc/get_document_info": {
				`{"nodeId":"source","workspaceId":"source-w","name":"Doc","extension":"adoc"}`,
				`{"nodeId":"copy","workspaceId":"w","folderId":"f","name":"Doc copy","extension":"adoc"}`,
			},
			"doc/copy_document": {`{"success":true,"nodeId":"copy"}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+node-copy", "--workspace", "w", "--folder", "f", "--node", "source", "--yes")
		if err != nil || out["targetFolderId"] != "f" {
			t.Fatalf("copy output=%#v err=%v", out, err)
		}
	})

	assertErr("move source workspace missing", &wikiCoverageCaller{responses: map[string][]string{
		"doc/get_document_info": {`{"nodeId":"n","name":"Doc"}`},
	}}, "+move", "--workspace", "target", "--node", "n", "--yes")

	t.Run("move to drive dry run records domain and folder", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"doc/get_document_info": {`{"nodeId":"n","workspaceId":"source","name":"Doc"}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+move-to-drive", "--node", "n", "--folder", "f", "--dry-run", "--yes")
		if err != nil || out["executed"] != false {
			t.Fatalf("move-to-drive dry output=%#v err=%v", out, err)
		}
	})

	moveCaller := func(spaces []string, listErrors []error) *wikiCoverageCaller {
		return &wikiCoverageCaller{
			responses: map[string][]string{
				"doc/get_document_info": {`{"nodeId":"n","workspaceId":"source"}`, `{"nodeId":"n","workspaceId":"target"}`},
				"doc/move_document":     {`{"success":true}`},
				"wiki/list_wikiSpaces":  spaces,
			},
			errors: map[string][]error{"wiki/list_wikiSpaces": listErrors},
		}
	}
	assertErr("my documents list transport", moveCaller(nil, []error{errors.New("my documents list failed")}), "+move-to-drive", "--node", "n", "--yes")
	assertErr("my documents malformed collection", moveCaller([]string{`{"success":true}`}, nil), "+move-to-drive", "--node", "n", "--yes")
	assertErr("my documents missing cursor", moveCaller([]string{`{"wikiSpaces":[],"hasMore":true}`}, nil), "+move-to-drive", "--node", "n", "--yes")
	t.Run("my documents list page limit", func(t *testing.T) {
		pages := make([]string, 20)
		for index := range pages {
			pages[index] = fmt.Sprintf(`{"wikiSpaces":[],"hasMore":true,"nextCursor":"page-%02d"}`, index+1)
		}
		assertErr("limit", moveCaller(pages, nil), "+move-to-drive", "--node", "n", "--yes")
	})
}

func TestCrossPlatformCoverageWikiResponseValidationBranches(t *testing.T) {
	if _, err := requireWikiResponse(nil, "op"); err == nil {
		t.Fatal("empty response accepted")
	}
	for _, data := range []map[string]any{{"success": "yes"}, {"success": false}, {"success": false, "message": "denied"}} {
		if _, err := requireWikiResponse(data, "op"); err == nil {
			t.Fatalf("invalid response accepted: %#v", data)
		}
	}
	if _, err := requireWikiWrite(map[string]any{"result": map[string]any{"id": "x"}}, "op"); err == nil {
		t.Fatal("write without terminal success accepted")
	}
	if _, err := requireWikiWrite(map[string]any{"success": false}, "op"); err == nil {
		t.Fatal("failed write response accepted")
	}
	for _, data := range []map[string]any{
		{"success": false}, {"result": "bad"}, {"result": map[string]any{}}, {"only": true},
	} {
		if _, err := requireWikiObject(data, "op"); err == nil {
			t.Fatalf("invalid object accepted: %#v", data)
		}
	}
	if object, err := requireWikiObject(map[string]any{"success": true, "id": "x"}, "op"); err != nil || object["id"] != "x" {
		t.Fatalf("direct object=%#v err=%v", object, err)
	}
	for _, data := range []map[string]any{
		{"success": false}, {"result": "bad"}, {"result": map[string]any{"items": "bad"}}, {"items": []any{1}}, {"success": true},
	} {
		if _, _, err := requireWikiCollection(data, "op", "items"); err == nil {
			t.Fatalf("invalid collection accepted: %#v", data)
		}
	}
	if value := nestedWikiString(map[string]any{"data": map[string]any{"id": " nested "}}, "id"); value != "nested" {
		t.Fatalf("nested string=%q", value)
	}
	if value := nestedWikiString(map[string]any{"id": " direct "}, "id"); value != "direct" {
		t.Fatalf("direct string=%q", value)
	}
	if value := nestedWikiString(map[string]any{"id": 1}, "id"); value != "" {
		t.Fatalf("non-string=%q", value)
	}

	page := map[string]any{"nextToken": "n", "hasMore": true, "truncated": false, "totalCount": 1, "autoPageComplete": true, "autoPageStopReason": "done", "pagesFetched": 2}
	out := map[string]any{}
	addWikiPagination(out, page)
	for _, key := range []string{"nextCursor", "hasMore", "truncated", "totalCount", "autoPageComplete", "autoPageStopReason", "pagesFetched"} {
		if _, ok := out[key]; !ok {
			t.Fatalf("pagination output missing %s: %#v", key, out)
		}
	}
	rows := projectWikiRows([]any{map[string]any{"legacy": "x", "empty": nil}}, map[string][]string{"value": {"empty", "legacy"}})
	if len(rows) != 1 || rows[0]["value"] != "x" {
		t.Fatalf("projected rows=%#v", rows)
	}
}

func TestCrossPlatformCoverageWikiAliasAndCancellationBranches(t *testing.T) {
	caller := &wikiCoverageCaller{responses: map[string][]string{"wiki/remove_member": {`{"success":true}`}}}
	out, err := runWikiCoverageCLI(t, caller, "+member-remove", "--workspace", "w", "--user", "u1")
	if err != nil || out["userCount"] != float64(1) {
		t.Fatalf("visible member alias output=%#v err=%v calls=%#v", out, err, caller.calls)
	}

	cmd := &cobra.Command{Use: "probe"}
	cmd.Flags().StringSlice("users", nil, "")
	cmd.Flags().StringSlice("user", nil, "")
	rt := shortcut.RuntimeContextForTest(cmd, MemberRemove)
	if got := wikiStringSliceFirst(rt, "users", "user"); len(got) != 0 {
		t.Fatalf("unset string slice=%v", got)
	}

	pageCmd := &cobra.Command{Use: "page"}
	pageCmd.Flags().Bool("page-all", true, "")
	pageCmd.Flags().Int("page-limit", 2, "")
	pageCmd.Flags().Int("max-items", 0, "")
	pageCmd.Flags().Int("page-delay", 1, "")
	pageCmd.Flags().String("cursor", "", "")
	pageCmd.Flags().String("page-token", "", "")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	pageCmd.SetContext(cancelled)
	pageRT := shortcut.RuntimeContextForTest(pageCmd, SpaceList)
	_, _, err = collectWikiPages(pageRT, "probe", 1, []string{"items"}, func(string, int) (map[string]any, error) {
		return map[string]any{"items": []any{}, "hasMore": true, "nextCursor": "next"}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled page delay err=%v", err)
	}
}

func TestCrossPlatformCoverageWikiPaginationFailureModes(t *testing.T) {
	t.Run("two pages", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/list_wikiSpaces": {
				`{"wikiSpaces":[{"workspaceId":"w1"}],"hasMore":true,"nextCursor":"next"}`,
				`{"wikiSpaces":[{"workspaceId":"w2"}],"hasMore":false}`,
			},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-list", "--limit", "1", "--page-all", "--page-limit", "2")
		if err != nil || out["count"] != float64(2) || out["autoPageComplete"] != true {
			t.Fatalf("pagination output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("max items trims first page", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/list_wikiSpaces": {`{"wikiSpaces":[{"workspaceId":"w1"},{"workspaceId":"w2"}],"hasMore":true,"nextCursor":"next"}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-list", "--limit", "2", "--page-all", "--max-items", "1")
		spaces, ok := out["spaces"].([]any)
		if err != nil || !ok || out["count"] != float64(1) || len(spaces) != 1 || out["autoPageStopReason"] != "max_items" {
			t.Fatalf("max-items output=%#v err=%v", out, err)
		}
	})

	t.Run("max items trims remaining page", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/list_wikiSpaces": {
				`{"wikiSpaces":[{"workspaceId":"w1"},{"workspaceId":"w2"}],"hasMore":true,"nextCursor":"next"}`,
				`{"wikiSpaces":[{"workspaceId":"w3"},{"workspaceId":"w4"}],"hasMore":false}`,
			},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-list", "--limit", "2", "--page-all", "--page-limit", "2", "--max-items", "3")
		spaces, ok := out["spaces"].([]any)
		if err != nil || !ok || out["count"] != float64(3) || len(spaces) != 3 || out["autoPageStopReason"] != "max_items" {
			t.Fatalf("max-items output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
		if len(caller.calls) != 2 || caller.calls[1].args["pageSize"] != 1 {
			t.Fatalf("remaining page request=%#v, want pageSize=1", caller.calls)
		}
		third, ok := spaces[2].(map[string]any)
		if !ok || third["workspaceId"] != "w3" {
			t.Fatalf("trimmed spaces=%#v, want w3 as final item", spaces)
		}
	})

	cases := []struct {
		name     string
		response string
		args     []string
	}{
		{name: "missing has more", response: `{"wikiSpaces":[]}`, args: []string{"--page-all"}},
		{name: "malformed has more", response: `{"wikiSpaces":[],"hasMore":"yes"}`, args: []string{"--page-all"}},
		{name: "missing cursor", response: `{"wikiSpaces":[],"hasMore":true}`, args: []string{"--page-all"}},
		{name: "stalled cursor", response: `{"wikiSpaces":[],"hasMore":true,"nextCursor":"same"}`, args: []string{"--cursor", "same", "--page-all"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &wikiCoverageCaller{responses: map[string][]string{"wiki/list_wikiSpaces": {tc.response}}}
			if _, err := runWikiCoverageCLI(t, caller, append([]string{"+space-list"}, tc.args...)...); err == nil {
				t.Fatalf("pagination failure %s succeeded", tc.name)
			}
		})
	}

	t.Run("page limit preserves cursor and accumulated items", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/list_wikiSpaces": {`{"wikiSpaces":[{"workspaceId":"w1"}],"hasMore":true,"nextCursor":"next"}`},
		}}
		_, err := runWikiCoverageCLI(t, caller, "+space-list", "--limit", "1", "--page-all", "--page-limit", "1")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "page_limit_reached" || typed.Details["nextCursor"] != "next" || typed.Details["pagesFetched"] != 1 || typed.Details["itemsFetched"] != 1 || typed.Details["status"] != "partial_success" {
			t.Fatalf("page-limit error=%#v details=%#v", err, typed)
		}
		items, ok := typed.Details["items"].([]any)
		if !ok || len(items) != 1 {
			t.Fatalf("page-limit items=%#v", typed.Details["items"])
		}
	})

	for _, args := range [][]string{{"--page-limit", "0", "--page-all"}, {"--max-items", "1"}, {"--page-delay", "1"}} {
		caller := &wikiCoverageCaller{}
		if _, err := runWikiCoverageCLI(t, caller, append([]string{"+space-list"}, args...)...); err == nil || len(caller.calls) != 0 {
			t.Fatalf("invalid auto-page controls succeeded: %v", args)
		}
	}
}

func TestCrossPlatformCoverageWikiMCPFailuresPropagate(t *testing.T) {
	caller := &wikiCoverageCaller{
		responses: map[string][]string{"wiki/list_wikiSpaces": {`{"wikiSpaces":[]}`}},
		errors:    map[string][]error{"wiki/list_wikiSpaces": {errors.New("read failed")}},
	}
	if _, err := runWikiCoverageCLI(t, caller, "+space-list"); err == nil {
		t.Fatal("MCP failure was swallowed")
	}
}

func TestCrossPlatformCoverageWikiSpaceFailureBranches(t *testing.T) {
	assertErr := func(name string, caller *wikiCoverageCaller, args ...string) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			if _, err := runWikiCoverageCLI(t, caller, args...); err == nil {
				t.Fatalf("%s unexpectedly succeeded; calls=%#v", name, caller.calls)
			}
		})
	}
	backend := errors.New("backend failed")
	assertErr("search invalid limit", &wikiCoverageCaller{}, "+space-search", "--query", "x", "--limit", "bad")
	assertErr("search call", &wikiCoverageCaller{errors: map[string][]error{"wiki/search_wikiSpaces": {backend}}}, "+space-search", "--query", "x")
	assertErr("search collection", &wikiCoverageCaller{responses: map[string][]string{"wiki/search_wikiSpaces": {`{"success":true}`}}}, "+space-search", "--query", "x")
	assertErr("get call", &wikiCoverageCaller{errors: map[string][]error{"wiki/get_wikiSpace": {backend}}}, "+space-get", "--workspace", "w")
	assertErr("get object", &wikiCoverageCaller{responses: map[string][]string{"wiki/get_wikiSpace": {`{"success":true}`}}}, "+space-get", "--workspace", "w")
	caller := &wikiCoverageCaller{responses: map[string][]string{"wiki/get_wikiSpace": {`{"workspaceId":"w","name":"Docs"}`}}}
	if out, err := runWikiCoverageCLI(t, caller, "+space-get", "--workspace", "w"); err != nil || out["workspaceId"] != "w" {
		t.Fatalf("space get output=%#v err=%v", out, err)
	}

	assertErr("create write call", &wikiCoverageCaller{errors: map[string][]error{"wiki/create_wikiSpace": {backend}}}, "+space-create", "--name", "Docs")
	assertErr("create terminal", &wikiCoverageCaller{responses: map[string][]string{"wiki/create_wikiSpace": {`{"result":{"workspaceId":"w"}}`}}}, "+space-create", "--name", "Docs")
	assertErr("create id", &wikiCoverageCaller{responses: map[string][]string{"wiki/create_wikiSpace": {`{"success":true}`}}}, "+space-create", "--name", "Docs")
	assertErr("create readback call", &wikiCoverageCaller{responses: map[string][]string{"wiki/create_wikiSpace": {`{"success":true,"workspaceId":"w"}`}}, errors: map[string][]error{"wiki/get_wikiSpace": {backend}}}, "+space-create", "--name", "Docs")
	assertErr("create readback object", &wikiCoverageCaller{responses: map[string][]string{"wiki/create_wikiSpace": {`{"success":true,"workspaceId":"w"}`}, "wiki/get_wikiSpace": {`{"success":true}`}}}, "+space-create", "--name", "Docs")
	assertErr("create readback mismatch", &wikiCoverageCaller{responses: map[string][]string{"wiki/create_wikiSpace": {`{"success":true,"workspaceId":"w"}`}, "wiki/get_wikiSpace": {`{"workspaceId":"other"}`}}}, "+space-create", "--name", "Docs")
	spaceMismatch := &wikiCoverageCaller{responses: map[string][]string{"wiki/create_wikiSpace": {`{"success":true,"result":{"workspaceId":"w"}}`}, "wiki/get_wikiSpace": {`{"success":true,"data":{"workspaceId":"other"}}`}}}
	if _, err := runWikiCoverageCLI(t, spaceMismatch, "+space-create", "--name", "Docs"); err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("space create mismatch error=%v calls=%#v", err, spaceMismatch.calls)
	}

	assertErr("delete preflight call", &wikiCoverageCaller{errors: map[string][]error{"wiki/get_wikiSpace": {backend}}}, "+delete-space", "--workspace", "w", "--yes")
	assertErr("delete preflight object", &wikiCoverageCaller{responses: map[string][]string{"wiki/get_wikiSpace": {`{"success":true}`}}}, "+delete-space", "--workspace", "w", "--yes")
	assertErr("delete write call", &wikiCoverageCaller{responses: map[string][]string{"wiki/get_wikiSpace": {`{"workspaceId":"w"}`}}, errors: map[string][]error{"wiki/delete_wikiSpace": {backend}}}, "+delete-space", "--workspace", "w", "--yes")
	assertErr("delete terminal", &wikiCoverageCaller{responses: map[string][]string{"wiki/get_wikiSpace": {`{"workspaceId":"w"}`}, "wiki/delete_wikiSpace": {`{"result":{}}`}}}, "+delete-space", "--workspace", "w", "--yes")
	delWrite := &wikiCoverageCaller{responses: map[string][]string{"wiki/get_wikiSpace": {`{"success":true,"result":{"workspaceId":"w"}}`}}, errors: map[string][]error{"wiki/delete_wikiSpace": {backend}}}
	if _, err := runWikiCoverageCLI(t, delWrite, "+delete-space", "--workspace", "w", "--yes"); err == nil || !strings.Contains(err.Error(), "backend failed") {
		t.Fatalf("delete write error=%v calls=%#v", err, delWrite.calls)
	}
	delTerminal := &wikiCoverageCaller{responses: map[string][]string{"wiki/get_wikiSpace": {`{"success":true,"result":{"workspaceId":"w"}}`}, "wiki/delete_wikiSpace": {`{"result":{}}`}}}
	if _, err := runWikiCoverageCLI(t, delTerminal, "+delete-space", "--workspace", "w", "--yes"); err == nil || !strings.Contains(err.Error(), "success=true") {
		t.Fatalf("delete terminal error=%v calls=%#v", err, delTerminal.calls)
	}

	assertErr("member list call", &wikiCoverageCaller{errors: map[string][]error{"wiki/list_member": {backend}}}, "+member-list", "--workspace", "w")
	assertErr("member list collection", &wikiCoverageCaller{responses: map[string][]string{"wiki/list_member": {`{"success":true}`}}}, "+member-list", "--workspace", "w")
	assertErr("member write call", &wikiCoverageCaller{errors: map[string][]error{"wiki/add_member": {backend}}}, "+member-add", "--workspace", "w", "--users", "u", "--role", "READER")
}

func TestCrossPlatformCoverageWikiNodeFailureBranches(t *testing.T) {
	assertErr := func(name string, caller *wikiCoverageCaller, args ...string) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			if _, err := runWikiCoverageCLI(t, caller, args...); err == nil {
				t.Fatalf("%s unexpectedly succeeded; calls=%#v", name, caller.calls)
			}
		})
	}
	backend := errors.New("backend failed")
	assertErr("list call", &wikiCoverageCaller{errors: map[string][]error{"doc/list_nodes": {backend}}}, "+node-list", "--workspace", "w")
	assertErr("list collection", &wikiCoverageCaller{responses: map[string][]string{"doc/list_nodes": {`{"success":true}`}}}, "+node-list", "--workspace", "w")
	assertErr("get call", &wikiCoverageCaller{errors: map[string][]error{"doc/get_document_info": {backend}}}, "+node-get", "--node", "n")
	assertErr("get object", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"success":true}`}}}, "+node-get", "--node", "n")
	assertErr("get id", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"name":"Doc"}`}}}, "+node-get", "--node", "n")
	missingID := &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"success":true,"result":{"name":"Doc"}}`}}}
	if _, err := runWikiCoverageCLI(t, missingID, "+node-get", "--node", "n"); err == nil || !strings.Contains(err.Error(), "nodeId") {
		t.Fatalf("missing node id error=%v calls=%#v", err, missingID.calls)
	}
	assertErr("search call", &wikiCoverageCaller{errors: map[string][]error{"doc/search_documents": {backend}}}, "+node-search", "--workspace", "w", "--query", "x")
	assertErr("search collection", &wikiCoverageCaller{responses: map[string][]string{"doc/search_documents": {`{"success":true}`}}}, "+node-search", "--workspace", "w", "--query", "x")

	createArgs := []string{"+node-create", "--workspace", "w", "--name", "Doc"}
	assertErr("create write", &wikiCoverageCaller{errors: map[string][]error{"doc/create_file": {backend}}}, createArgs...)
	assertErr("create terminal", &wikiCoverageCaller{responses: map[string][]string{"doc/create_file": {`{"result":{"fileId":"n"}}`}}}, createArgs...)
	assertErr("create id", &wikiCoverageCaller{responses: map[string][]string{"doc/create_file": {`{"success":true}`}}}, createArgs...)
	assertErr("create readback call", &wikiCoverageCaller{responses: map[string][]string{"doc/create_file": {`{"success":true,"fileId":"n"}`}}, errors: map[string][]error{"doc/get_document_info": {backend}}}, createArgs...)
	assertErr("create readback object", &wikiCoverageCaller{responses: map[string][]string{"doc/create_file": {`{"success":true,"fileId":"n"}`}, "doc/get_document_info": {`{"success":true}`}}}, createArgs...)
	assertErr("create mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/create_file": {`{"success":true,"fileId":"n"}`}, "doc/get_document_info": {`{"nodeId":"other"}`}}}, createArgs...)
	createMismatch := &wikiCoverageCaller{responses: map[string][]string{"doc/create_file": {`{"success":true,"result":{"fileId":"n"}}`}, "doc/get_document_info": {`{"success":true,"result":{"nodeId":"other"}}`}}}
	if _, err := runWikiCoverageCLI(t, createMismatch, createArgs...); err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("create mismatch error=%v calls=%#v", err, createMismatch.calls)
	}
	validCreated := func(overrides string) string {
		return `{"nodeId":"n","workspaceId":"w","name":"Doc","extension":"adoc"` + overrides + `}`
	}
	assertErr("create missing workspace", &wikiCoverageCaller{responses: map[string][]string{"doc/create_file": {`{"success":true,"fileId":"n"}`}, "doc/get_document_info": {`{"nodeId":"n","name":"Doc","extension":"adoc"}`}}}, createArgs...)
	assertErr("create name mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/create_file": {`{"success":true,"fileId":"n"}`}, "doc/get_document_info": {validCreated(`,"name":"Other"`)}}}, createArgs...)
	assertErr("create type mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/create_file": {`{"success":true,"fileId":"n"}`}, "doc/get_document_info": {validCreated(`,"extension":"axls"`)}}}, createArgs...)
	assertErr("create folder mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/create_file": {`{"success":true,"fileId":"n"}`}, "doc/get_document_info": {validCreated(`,"folderId":"other"`)}}}, append(createArgs, "--folder", "f")...)

	copyArgs := []string{"+node-copy", "--workspace", "w", "--node", "source", "--yes"}
	source := `{"nodeId":"source","workspaceId":"source-w","name":"Doc","extension":"adoc"}`
	assertErr("copy source preflight call", &wikiCoverageCaller{errors: map[string][]error{"doc/get_document_info": {backend}}}, copyArgs...)
	assertErr("copy source preflight object", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"success":true}`}}}, copyArgs...)
	assertErr("copy source id", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"other"}`}}}, copyArgs...)
	assertErr("copy source workspace", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"source"}`}}}, copyArgs...)
	assertErr("copy write", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {source}}, errors: map[string][]error{"doc/copy_document": {backend}}}, copyArgs...)
	assertErr("copy terminal", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {source}, "doc/copy_document": {`{"result":{"fileId":"n"}}`}}}, copyArgs...)
	assertErr("copy id", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {source}, "doc/copy_document": {`{"success":true}`}}}, copyArgs...)
	assertErr("copy reused source id", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {source}, "doc/copy_document": {`{"success":true,"fileId":"source"}`}}}, copyArgs...)
	assertErr("copy readback call", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {source}, "doc/copy_document": {`{"success":true,"fileId":"n"}`}}, errors: map[string][]error{"doc/get_document_info": {nil, backend}}}, copyArgs...)
	assertErr("copy readback object", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {source, `{"success":true}`}, "doc/copy_document": {`{"success":true,"fileId":"n"}`}}}, copyArgs...)
	copyMismatch := &wikiCoverageCaller{responses: map[string][]string{"doc/copy_document": {`{"success":true,"result":{"fileId":"n"}}`}, "doc/get_document_info": {source, `{"success":true,"result":{"nodeId":"other"}}`}}}
	if _, err := runWikiCoverageCLI(t, copyMismatch, copyArgs...); err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("copy mismatch error=%v calls=%#v", err, copyMismatch.calls)
	}
	assertErr("copy workspace mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {source, `{"nodeId":"n","workspaceId":"other","extension":"adoc"}`}, "doc/copy_document": {`{"success":true,"fileId":"n"}`}}}, copyArgs...)
	assertErr("copy folder mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {source, `{"nodeId":"n","workspaceId":"w","folderId":"other","extension":"adoc"}`}, "doc/copy_document": {`{"success":true,"fileId":"n"}`}}}, append(copyArgs, "--folder", "f")...)
	assertErr("copy type mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {source, `{"nodeId":"n","workspaceId":"w","extension":"axls"}`}, "doc/copy_document": {`{"success":true,"fileId":"n"}`}}}, copyArgs...)

	moveArgs := []string{"+move", "--workspace", "target", "--node", "n", "--yes"}
	assertErr("move preflight call", &wikiCoverageCaller{errors: map[string][]error{"doc/get_document_info": {backend}}}, moveArgs...)
	assertErr("move preflight object", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"success":true}`}}}, moveArgs...)
	assertErr("move preflight id", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"other","workspaceId":"old"}`}}}, moveArgs...)
	assertErr("move preflight workspace", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n"}`}}}, moveArgs...)
	assertErr("move write", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`}}, errors: map[string][]error{"doc/move_document": {backend}}}, moveArgs...)
	assertErr("move terminal", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`}, "doc/move_document": {`{"result":{}}`}}}, moveArgs...)
	assertErr("move readback call", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`}, "doc/move_document": {`{"success":true}`}}, errors: map[string][]error{"doc/get_document_info": {nil, backend}}}, moveArgs...)
	assertErr("move readback object", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"success":true}`}, "doc/move_document": {`{"success":true}`}}}, moveArgs...)
	assertErr("move id mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"nodeId":"other","workspaceId":"target"}`}, "doc/move_document": {`{"success":true}`}}}, moveArgs...)
	assertErr("move workspace mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"nodeId":"n","workspaceId":"other"}`}, "doc/move_document": {`{"success":true}`}}}, moveArgs...)
	assertErr("move folder mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"nodeId":"n","workspaceId":"target","folderId":"other"}`}, "doc/move_document": {`{"success":true}`}}}, append(moveArgs, "--folder", "f")...)
	assertErr("drive move unchanged", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"same"}`, `{"nodeId":"n","workspaceId":"same"}`}, "doc/move_document": {`{"success":true}`}}}, "+move-to-drive", "--node", "n", "--yes")
	assertErr("drive move source workspace mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"actual"}`}}}, "+move-to-drive", "--workspace", "expected", "--node", "n", "--yes")
	assertErr("drive target is not my documents", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"nodeId":"n","workspaceId":"drive"}`}, "doc/move_document": {`{"success":true}`}, "wiki/list_wikiSpaces": {`{"wikiSpaces":[{"workspaceId":"other"}],"hasMore":false}`}}}, "+move-to-drive", "--node", "n", "--yes")
	assertErr("drive target space cursor cycle", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"nodeId":"n","workspaceId":"drive"}`}, "doc/move_document": {`{"success":true}`}, "wiki/list_wikiSpaces": {`{"wikiSpaces":[],"hasMore":true,"nextCursor":"next"}`, `{"wikiSpaces":[],"hasMore":true,"nextCursor":"next"}`}}}, "+move-to-drive", "--node", "n", "--yes")

	deleteArgs := []string{"+node-delete", "--workspace", "w", "--node", "n", "--yes"}
	assertErr("delete preflight call", &wikiCoverageCaller{errors: map[string][]error{"doc/get_document_info": {backend}}}, deleteArgs...)
	assertErr("delete preflight object", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"success":true}`}}}, deleteArgs...)
	assertErr("delete workspace", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"other"}`}}}, deleteArgs...)
	assertErr("delete write", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"w"}`}}, errors: map[string][]error{"doc/delete_document": {backend}}}, deleteArgs...)
	assertErr("delete terminal", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"w"}`}, "doc/delete_document": {`{"result":{}}`}}}, deleteArgs...)
	assertErr("feed collection", &wikiCoverageCaller{responses: map[string][]string{"wiki/list_workspace_feeds": {`{"success":true}`}}}, "+feed-list", "--workspace", "w")
}

func TestCrossPlatformCoverageWikiSecondPageFailures(t *testing.T) {
	caller := &wikiCoverageCaller{
		responses: map[string][]string{"wiki/list_wikiSpaces": {`{"wikiSpaces":[],"hasMore":true,"nextCursor":"next"}`}},
		errors:    map[string][]error{"wiki/list_wikiSpaces": {nil, errors.New("second page failed")}},
	}
	if _, err := runWikiCoverageCLI(t, caller, "+space-list", "--page-all"); err == nil {
		t.Fatal("second-page transport error was swallowed")
	}
	caller = &wikiCoverageCaller{responses: map[string][]string{"wiki/list_wikiSpaces": {`{"wikiSpaces":[],"hasMore":true,"nextCursor":"next"}`, `{"success":true}`}}}
	if _, err := runWikiCoverageCLI(t, caller, "+space-list", "--page-all"); err == nil {
		t.Fatal("second-page malformed collection was swallowed")
	}
}
