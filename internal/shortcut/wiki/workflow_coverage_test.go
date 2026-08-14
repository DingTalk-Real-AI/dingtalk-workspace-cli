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
			"wiki/list_wikiSpaces": {`{"success":true,"result":{"wikiSpaces":[{"workspaceId":"w1","name":"Docs"}],"hasMore":false}}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-list")
		if err != nil || out["count"] != float64(1) || caller.calls[0].args["pageSize"] != 20 {
			t.Fatalf("list output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("list explicit legacy string limit and cursor", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/list_wikiSpaces": {`{"wikiSpaces":[],"hasMore":false}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-list", "--type", "myWikiSpace", "--limit", "7", "--page-token", "next")
		if err != nil || out["count"] != float64(0) || caller.calls[0].args["pageSize"] != 7 || caller.calls[0].args["pageToken"] != "next" {
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
		if err != nil || out["count"] != float64(1) || caller.calls[0].args["pageSize"] != 12 {
			t.Fatalf("search output=%#v err=%v calls=%#v", out, err, caller.calls)
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
			"wiki/get_wikiSpace":    {`{"success":true,"data":{"workspaceId":"w3","name":"Docs"}}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-create", "--name", "Docs")
		if err != nil || out["workspaceId"] != "w3" || len(caller.calls) != 2 {
			t.Fatalf("create output=%#v err=%v calls=%#v", out, err, caller.calls)
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
		caller := &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"success":true,"result":{"nodeId":"n","title":"Doc"}}`}}}
		out, err := runWikiCoverageCLI(t, caller, "+node-get", "--node", "n")
		if err != nil || out["nodeId"] != "n" {
			t.Fatalf("node get output=%#v err=%v", out, err)
		}
	})
}

func TestCrossPlatformCoverageWikiWriteWorkflows(t *testing.T) {
	t.Run("node create dry and verified", func(t *testing.T) {
		dry := &wikiCoverageCaller{}
		out, err := runWikiCoverageCLI(t, dry, "+node-create", "--workspace", "w", "--folder", "f", "--name", "Doc", "--type", "adoc", "--dry-run")
		if err != nil || out["executed"] != false || len(dry.calls) != 0 {
			t.Fatalf("create dry output=%#v err=%v calls=%#v", out, err, dry.calls)
		}
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"doc/create_file":       {`{"success":true,"data":{"fileId":"n"}}`},
			"doc/get_document_info": {`{"success":true,"nodeId":"n","workspaceId":"w","folderId":"f"}`},
		}}
		out, err = runWikiCoverageCLI(t, caller, "+node-create", "--workspace", "w", "--folder", "f", "--name", "Doc")
		if err != nil || out["nodeId"] != "n" || len(caller.calls) != 2 {
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
			"doc/copy_document":     {`{"success":true,"nodeId":"copy"}`},
			"doc/get_document_info": {`{"success":true,"nodeId":"copy","workspaceId":"w"}`},
		}}
		out, err = runWikiCoverageCLI(t, caller, "+node-copy", "--workspace", "w", "--node", "source", "--yes")
		if err != nil || out["nodeId"] != "copy" || len(caller.calls) != 2 {
			t.Fatalf("copy output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
	})

	t.Run("move within wiki and to drive", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old","folderId":"old-f"}`, `{"nodeId":"n","workspaceId":"target","folderId":"f"}`},
			"doc/move_document":     {`{"success":true}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+move", "--workspace", "target", "--folder", "f", "--node", "n", "--yes")
		if err != nil || out["nodeId"] != "n" || len(caller.calls) != 3 {
			t.Fatalf("move output=%#v err=%v calls=%#v", out, err, caller.calls)
		}
		caller = &wikiCoverageCaller{responses: map[string][]string{
			"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"nodeId":"n","workspaceId":"drive"}`},
			"doc/move_document":     {`{"success":true}`},
		}}
		out, err = runWikiCoverageCLI(t, caller, "+move-to-drive", "--node", "n", "--yes")
		if err != nil || out["nodeId"] != "n" {
			t.Fatalf("move-to-drive output=%#v err=%v calls=%#v", out, err, caller.calls)
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

	t.Run("max items", func(t *testing.T) {
		caller := &wikiCoverageCaller{responses: map[string][]string{
			"wiki/list_wikiSpaces": {`{"wikiSpaces":[{"workspaceId":"w1"}],"hasMore":true,"nextCursor":"next"}`},
		}}
		out, err := runWikiCoverageCLI(t, caller, "+space-list", "--limit", "2", "--page-all", "--max-items", "1")
		if err != nil || out["autoPageStopReason"] != "max_items" {
			t.Fatalf("max-items output=%#v err=%v", out, err)
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
		{name: "page limit", response: `{"wikiSpaces":[],"hasMore":true,"nextCursor":"next"}`, args: []string{"--page-all", "--page-limit", "1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &wikiCoverageCaller{responses: map[string][]string{"wiki/list_wikiSpaces": {tc.response}}}
			if _, err := runWikiCoverageCLI(t, caller, append([]string{"+space-list"}, tc.args...)...); err == nil {
				t.Fatalf("pagination failure %s succeeded", tc.name)
			}
		})
	}

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

	copyArgs := []string{"+node-copy", "--workspace", "w", "--node", "source", "--yes"}
	assertErr("copy write", &wikiCoverageCaller{errors: map[string][]error{"doc/copy_document": {backend}}}, copyArgs...)
	assertErr("copy terminal", &wikiCoverageCaller{responses: map[string][]string{"doc/copy_document": {`{"result":{"fileId":"n"}}`}}}, copyArgs...)
	assertErr("copy id", &wikiCoverageCaller{responses: map[string][]string{"doc/copy_document": {`{"success":true}`}}}, copyArgs...)
	assertErr("copy readback call", &wikiCoverageCaller{responses: map[string][]string{"doc/copy_document": {`{"success":true,"fileId":"n"}`}}, errors: map[string][]error{"doc/get_document_info": {backend}}}, copyArgs...)
	assertErr("copy readback object", &wikiCoverageCaller{responses: map[string][]string{"doc/copy_document": {`{"success":true,"fileId":"n"}`}, "doc/get_document_info": {`{"success":true}`}}}, copyArgs...)

	moveArgs := []string{"+move", "--workspace", "target", "--node", "n", "--yes"}
	assertErr("move preflight call", &wikiCoverageCaller{errors: map[string][]error{"doc/get_document_info": {backend}}}, moveArgs...)
	assertErr("move preflight object", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"success":true}`}}}, moveArgs...)
	assertErr("move write", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`}}, errors: map[string][]error{"doc/move_document": {backend}}}, moveArgs...)
	assertErr("move terminal", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`}, "doc/move_document": {`{"result":{}}`}}}, moveArgs...)
	assertErr("move readback call", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`}, "doc/move_document": {`{"success":true}`}}, errors: map[string][]error{"doc/get_document_info": {nil, backend}}}, moveArgs...)
	assertErr("move readback object", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"success":true}`}, "doc/move_document": {`{"success":true}`}}}, moveArgs...)
	assertErr("move id mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"nodeId":"other","workspaceId":"target"}`}, "doc/move_document": {`{"success":true}`}}}, moveArgs...)
	assertErr("move workspace mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"nodeId":"n","workspaceId":"other"}`}, "doc/move_document": {`{"success":true}`}}}, moveArgs...)
	assertErr("move folder mismatch", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"old"}`, `{"nodeId":"n","workspaceId":"target","folderId":"other"}`}, "doc/move_document": {`{"success":true}`}}}, append(moveArgs, "--folder", "f")...)
	assertErr("drive move unchanged", &wikiCoverageCaller{responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"n","workspaceId":"same"}`, `{"nodeId":"n","workspaceId":"same"}`}, "doc/move_document": {`{"success":true}`}}}, "+move-to-drive", "--node", "n", "--yes")

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
