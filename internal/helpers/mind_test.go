// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

const mindTestSource = `{"schemaVersion":"1.0","catalogVersion":"mind-v1","pages":[{"id":"p","title":"计划","nodes":[{"id":"r","type":"topic","text":"Root"}]}]}`

type mindTestCaller struct {
	allowedServer string
	dry           bool
	calls         int
	tool          string
	args          map[string]any
	response      string
}

func (c *mindTestCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	expectedServer := c.allowedServer
	if expectedServer == "" {
		expectedServer = "mind"
	}
	if server != expectedServer {
		return nil, fmt.Errorf("wrong server %q", server)
	}
	c.calls++
	c.tool = tool
	c.args = args
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.response}}}, nil
}
func (c *mindTestCaller) Format() string { return "json" }
func (c *mindTestCaller) DryRun() bool   { return c.dry }
func (c *mindTestCaller) Fields() string { return "" }
func (c *mindTestCaller) JQ() string     { return "" }
func installMindCaller(t *testing.T, c *mindTestCaller) *bytes.Buffer {
	t.Helper()
	testseam.Protect(t, &deps)
	InitDeps(c)
	buf := &bytes.Buffer{}
	deps.Out.w = buf
	deps.Out.errW = &bytes.Buffer{}
	return buf
}
func mindExecute(t *testing.T, c *mindTestCaller, args ...string) error {
	t.Helper()
	installMindCaller(t, c)
	cmd := newMindCommand()
	cmd.PersistentFlags().Bool("yes", false, "confirm")
	cmd.PersistentFlags().Bool("dry-run", c.dry, "preview")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetArgs(args)
	return corecmd.ExecuteForTest(cmd)
}
func mindArgs(t *testing.T, raw string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCrossPlatformCoverageMindFiveCommandsProjectBusinessArguments(t *testing.T) {
	tests := []struct {
		name, tool string
		args       []string
		check      func(*testing.T, map[string]any)
	}{
		{"create", "create_mind", []string{"create", "--name", "项目计划", "--source", mindTestSource, "--request-id", "req", "--folder", "folder", "--workspace", "workspace", "--yes"}, func(t *testing.T, v map[string]any) {
			if v["source"] != mindTestSource || v["folderId"] != "folder" || v["workspaceId"] != "workspace" {
				t.Fatalf("args %#v", v)
			}
		}},
		{"query", "list_mind_nodes", []string{"node", "list", "--node-id", "file", "--mode", "SUBTREE", "--page-id", "page", "--topic-id", "root", "--next-token", "mindc1_" + strings.Repeat("a", 43)}, func(t *testing.T, v map[string]any) {
			if v["depth"] != 8 || v["maxResults"] != 200 || v["nextToken"] != "mindc1_"+strings.Repeat("a", 43) {
				t.Fatalf("args %#v", v)
			}
		}},
		{"create topic", "create_mind_node", []string{"node", "create", "--node-id", "file", "--page-id", "page", "--request-id", "req", "--parent-topic-id", "root", "--topic", `{"id":"temp","type":"topic","text":"New"}`, "--before-topic-id", "before", "--expected-version", "0", "--yes"}, func(t *testing.T, v map[string]any) {
			if mindArgs(t, v["node"].(string))["id"] != "temp" || v["expectedVersion"] != "0" || v["parentTopicId"] != "root" {
				t.Fatalf("args %#v", v)
			}
		}},
		{"update empty", "update_mind_node", []string{"node", "update", "--node-id", "file", "--page-id", "page", "--request-id", "req", "--topic-id", "topic", "--text", "", "--yes"}, func(t *testing.T, v map[string]any) {
			if text, ok := mindArgs(t, v["patch"].(string))["text"]; !ok || text != "" {
				t.Fatalf("args %#v", v)
			}
		}},
		{"delete", "delete_mind_node", []string{"node", "delete", "--node-id", "file", "--page-id", "page", "--request-id", "req", "--topic-id", "topic", "--yes"}, func(t *testing.T, v map[string]any) {
			if v["deleteMode"] != "subtree" {
				t.Fatalf("args %#v", v)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &mindTestCaller{response: `{}`}
			err := mindExecute(t, c, tt.args...)
			if c.calls != 1 || c.tool != tt.tool {
				t.Fatalf("calls=%d tool=%q err=%v", c.calls, c.tool, err)
			}
			tt.check(t, c.args)
			for _, k := range []string{"uid", "orgId", "mcpBelongOrgId", "McpAccessUserInfo", "MindCreateRequest"} {
				if _, ok := c.args[k]; ok {
					t.Fatalf("unexpected HSF/identity wrapper %s", k)
				}
			}
		})
	}
}
func TestCrossPlatformCoverageMindValidationAndConfirmationPreventRemoteCalls(t *testing.T) {
	update := []string{"node", "update", "--node-id", "file", "--page-id", "page", "--request-id", "req", "--topic-id", "topic"}
	tests := [][]string{
		{"node", "list", "--node-id", "file", "--depth", "0"},
		{"node", "list", "--node-id", "file", "--max-results", "201"},
		{"node", "list", "--node-id", "file", "--topic-id", "topic"},
		{"node", "list", "--node-id", "file", "--next-token", "token"},
		{"node", "list", "--node-id", "file", "--mode", "NODE"},
		{"node", "list", "--node-id", "file", "--page-id", " "},
		append(append([]string{}, update...), "--yes"),
		append(append([]string{}, update...), "--text", "change"),
		append(append([]string{}, update...), "--text", "change", "--expected-version", "01", "--yes"),
		{"create", "--name", "x", "--request-id", "req", "--source", `{}`, "--yes"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			c := &mindTestCaller{}
			if err := mindExecute(t, c, args...); err == nil {
				t.Fatal("invalid/unconfirmed command accepted")
			}
			if c.calls != 0 {
				t.Fatal("called remote before validation/confirmation")
			}
		})
	}
}
func TestCrossPlatformCoverageMindDryRunAndFileSourceAreLocal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mind.json")
	if err := os.WriteFile(path, []byte(mindTestSource), 0600); err != nil {
		t.Fatal(err)
	}
	c := &mindTestCaller{dry: true}
	if err := mindExecute(t, c, "create", "--name", "x", "--source", path, "--request-id", "req"); err != nil {
		t.Fatal(err)
	}
	if c.calls != 0 {
		t.Fatal("dry-run called remote")
	}
	source, err := loadMindSource(path)
	if err != nil || source != mindTestSource {
		t.Fatalf("source=%#v err=%v", source, err)
	}
	if _, err := loadMindSource(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing file accepted")
	}
}
func TestCrossPlatformCoverageMindWriteReceiptsRequireConfirmedEffects(t *testing.T) {
	args := mindArgs(t, `{"requestId":"req","nodeId":"file","pageId":"page","topicId":"topic","patch":{"text":""}}`)
	valid := `{"requestId":"req","nodeId":"file","pageId":"page","topicId":"topic","status":"APPLIED","applied":true,"readVersion":"0","committedVersion":"1","node":{"id":"topic","type":"topic","text":""}}`
	tests := []struct {
		name, raw string
		outcome   output.Outcome
	}{
		{"applied", valid, output.OutcomeSuccess},
		{"pending", strings.Replace(strings.Replace(valid, `"APPLIED"`, `"COMMITTED_UNVERIFIED"`, 1), `"applied":true`, `"applied":false`, 1), output.OutcomePending},
		{"unknown", strings.Replace(valid, `"APPLIED"`, `"UNKNOWN"`, 1), output.OutcomeFailure},
		{"conflict", strings.Replace(valid, `"APPLIED"`, `"CONFLICT"`, 1), output.OutcomeFailure},
		{"no change", strings.Replace(strings.Replace(valid, `"APPLIED"`, `"NO_CHANGE"`, 1), `"applied":true`, `"applied":false`, 1), output.OutcomeSuccess},
		{"not confirmed", strings.Replace(valid, `"applied":true`, `"applied":false`, 1), output.OutcomeFailure},
		{"wrong request", strings.Replace(valid, `"req"`, `"wrong"`, 1), output.OutcomeFailure},
		{"wrong file", strings.Replace(valid, `"file"`, `"wrong"`, 1), output.OutcomeFailure},
		{"wrong text", strings.Replace(valid, `"text":""`, `"text":"wrong"`, 1), output.OutcomeFailure},
		{"numeric version", strings.Replace(valid, `"committedVersion":"1"`, `"committedVersion":1`, 1), output.OutcomeFailure},
		{"regressed version", strings.Replace(valid, `"committedVersion":"1"`, `"committedVersion":"0"`, 1), output.OutcomeFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receipt, err := decodeMindReply(tt.raw)
			if err != nil {
				t.Fatal(err)
			}
			result := mindWriteResult("update_mind_node", args, receipt)
			if result.Outcome() != tt.outcome {
				t.Fatalf("outcome=%v want %v", result.Outcome(), tt.outcome)
			}
			if _, err := output.EnvelopeFromResult(result); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCrossPlatformCoverageMindCreateAndDeleteReceipts(t *testing.T) {
	args := map[string]any{"requestId": "req", "name": "x", "source": mindTestSource}
	raw := `{"requestId":"req","nodeId":"file","status":"READY","requestedContentApplied":true,"revision":"0","pageIdMap":{"p":"page"},"nodeIdMap":{"r":"root"}}`
	receipt, _ := decodeMindReply(raw)
	if got := mindWriteResult("create_mind", args, receipt); got.Outcome() != output.OutcomeSuccess {
		t.Fatalf("create %#v", got)
	}
	receipt["nodeIdMap"] = map[string]any{"wrong": "root"}
	if mindWriteResult("create_mind", args, receipt).Outcome() != output.OutcomeFailure {
		t.Fatal("missing source ID accepted")
	}
	deleteArgs := mindArgs(t, `{"requestId":"req","nodeId":"file","pageId":"page","topicId":"topic"}`)
	deletion, _ := decodeMindReply(`{"requestId":"req","nodeId":"file","pageId":"page","topicId":"topic","status":"APPLIED","applied":true,"readVersion":"1","committedVersion":"2","deletedCountKnown":true,"deletedCount":2}`)
	if mindWriteResult("delete_mind_node", deleteArgs, deletion).Outcome() != output.OutcomeSuccess {
		t.Fatal("valid deletion failed")
	}
	deletion["deletedCount"] = json.Number("-1")
	if mindWriteResult("delete_mind_node", deleteArgs, deletion).Outcome() != output.OutcomeFailure {
		t.Fatal("invalid known count accepted")
	}
}
func TestCrossPlatformCoverageMindQueryPreservesPagesAndTruncation(t *testing.T) {
	raw := `{"nodeId":"file","schemaVersion":"1.0","catalogVersion":"mind-v1","readVersion":"3","items":[{"id":"page","title":"Page","rootId":"root","nodes":[{"id":"topic","type":"topic","text":"x"}]}],"hasMore":true,"truncated":true,"nextToken":"opaque"}`
	receipt, _ := decodeMindReply(raw)
	args := map[string]any{"nodeId": "file", "pageId": "page"}
	result := mindQueryResult("list_mind_nodes", args, receipt)
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Meta.Pagination.NextToken != "opaque" || env.Meta.Pagination.EndpointExhausted {
		t.Fatal("pagination lost")
	}
	data := result.Data().(map[string]any)
	if data["truncated"] != true || data["items"] == nil {
		t.Fatal("page/truncation lost")
	}
	if _, ok := data["hasMore"]; ok {
		t.Fatal("pagination duplicated into business data")
	}
	delete(receipt, "nextToken")
	if mindQueryResult("list_mind_nodes", args, receipt).Outcome() != output.OutcomeFailure {
		t.Fatal("missing token accepted")
	}
}
func TestCrossPlatformCoverageMindResponseProjectionAndDiagnostics(t *testing.T) {
	for _, raw := range []string{``, `null`, `{}`, `{"success":true,"result":{}}`, `{"success":true,"result":{"status":"READY"},"status":"UNKNOWN"}`, `{"a":1,"a":2}`, `{"status":"READY"}{}`} {
		if _, err := decodeMindReply(raw); err == nil {
			t.Fatalf("malformed accepted: %s", raw)
		}
	}
	value, err := decodeMindReply(`{"success":true,"traceId":"trace","result":{"status":"READY"}}`)
	if err != nil || value["status"] != "READY" || value["traceId"] != "trace" {
		t.Fatalf("wrapped response: %#v %v", value, err)
	}
	c := &mindTestCaller{response: `{"success":false,"errorCode":"mind.permission.denied","errorMessage":"permission denied","traceId":"trace","errorDetail":{"stage":"Solution.permission","code":"mind.permission.denied","message":"no access"}}`}
	installMindCaller(t, c)
	result, err := callMindResult(&cobra.Command{}, "create_mind", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.TraceID != "trace" || env.Error.Stage != "Solution.permission" || env.Error.Details["receipt"] == nil {
		t.Fatalf("diagnostics lost %#v", env)
	}
}

func TestCrossPlatformCoverageMindCreateTopicAndPendingRecoveryReceipts(t *testing.T) {
	args := mindArgs(t, `{"requestId":"req","nodeId":"file","pageId":"page","parentTopicId":"root","node":{"id":"temp","type":"topic","text":"O'Brien"}}`)
	raw := `{"requestId":"req","nodeId":"file","pageId":"page","topicId":"topic","status":"APPLIED","applied":true,"readVersion":"0","committedVersion":"1","nodeIdMap":{"temp":"topic"},"node":{"id":"topic","type":"topic","text":"O'Brien","parentId":"root"}}`
	receipt, _ := decodeMindReply(raw)
	if mindWriteResult("create_mind_node", args, receipt).Outcome() != output.OutcomeSuccess {
		t.Fatal("valid topic creation failed")
	}
	receipt["status"] = "COMMITTED_UNVERIFIED"
	receipt["applied"] = false
	env, err := output.EnvelopeFromResult(mindWriteResult("create_mind_node", args, receipt))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env.Meta.Operation.NextCommand, "--topic") || !strings.Contains(env.Meta.Operation.NextCommand, `'\''`) || strings.Contains(env.Meta.Operation.NextCommand, "--yes") {
		t.Fatalf("unsafe or incomplete recovery command %q", env.Meta.Operation.NextCommand)
	}
	create := map[string]any{"requestId": "req", "source": mindTestSource, "name": "x"}
	pending := map[string]any{"requestId": "req", "nodeId": "file", "status": "INITIALIZING", "requestedContentApplied": false}
	if _, err := output.EnvelopeFromResult(mindWriteResult("create_mind", create, pending)); err != nil {
		t.Fatal(err)
	}
	pending["status"] = "UNKNOWN"
	delete(pending, "nodeId")
	result := mindWriteResult("create_mind", create, pending)
	if result.Outcome() != output.OutcomeFailure {
		t.Fatal("unknown creation accepted")
	}
}
func TestCrossPlatformCoverageMindWrappedSuccessAndLocalReadValidation(t *testing.T) {
	args := map[string]any{"requestId": "req", "name": "x", "source": mindTestSource}
	c := &mindTestCaller{response: `{"success":true,"result":{"requestId":"req","nodeId":"file","status":"READY","requestedContentApplied":true,"revision":"0","pageIdMap":{"p":"page"},"nodeIdMap":{"r":"root"}}}`}
	installMindCaller(t, c)
	result, err := callMindResult(&cobra.Command{}, "create_mind", args)
	if err != nil || result.Outcome() != output.OutcomeSuccess {
		t.Fatalf("wrapped creation: %v %v", result, err)
	}
	if _, exists := result.Data().(map[string]any)["success"]; exists {
		t.Fatal("transport success duplicated in data")
	}
	for _, request := range []string{"", strings.Repeat("x", 129), strings.Repeat("😀", 65)} {
		if _, err := validateMindRequestID(request); err == nil {
			t.Fatal("invalid request ID accepted")
		}
	}
	if _, err := mindTextPatch(strings.Repeat("x", 10001)); err == nil {
		t.Fatal("oversized patch accepted")
	}
	if _, err := loadMindTopic(`{"id":"temp","type":"topic","text":null}`); err == nil {
		t.Fatal("invalid topic accepted")
	}
	c.response = `{"nodeId":"file","schemaVersion":"1.0","catalogVersion":"mind-v1","readVersion":"0","items":[],"hasMore":false,"truncated":false}`
	result, err = callMindResult(&cobra.Command{}, "list_mind_nodes", map[string]any{"nodeId": "file"})
	if err != nil || result.Outcome() != output.OutcomeSuccess {
		t.Fatalf("empty query: %v %v", result, err)
	}
}

// Exercise the real flag projection pipeline: even a confirmed command with
// valid request identity must reject malformed OpenNodes before any RPC.
func TestCrossPlatformCoverageMindOpenNodesValidationRejectsBeforeRemoteCall(t *testing.T) {
	sources := []struct{ name, raw string }{
		{"syntax", `{"schemaVersion":`},
		{"duplicate field", strings.Replace(mindTestSource, `"schemaVersion":"1.0"`, `"schemaVersion":"1.0","schemaVersion":"1.0"`, 1)},
		{"wrong version", strings.Replace(mindTestSource, "mind-v1", "dml-v1", 1)},
		{"wrong text type", strings.Replace(mindTestSource, `"text":"Root"`, `"text":42`, 1)},
		{"null text", strings.Replace(mindTestSource, `"text":"Root"`, `"text":null`, 1)},
		{"unknown field", strings.Replace(mindTestSource, `"text":"Root"`, `"text":"Root","checkpoint":{}`, 1)},
		{"missing parent", strings.Replace(mindTestSource, `"text":"Root"`, `"text":"Root"},{"id":"a","type":"topic","text":"Child","parentId":"missing"`, 1)},
		{"multiple roots", strings.Replace(mindTestSource, `"text":"Root"`, `"text":"Root"},{"id":"a","type":"topic","text":"Second root"`, 1)},
	}
	for _, test := range sources {
		for _, fromFile := range []bool{false, true} {
			for _, dryRun := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/file=%v/dry-run=%v", test.name, fromFile, dryRun), func(t *testing.T) {
					source := test.raw
					if fromFile {
						source = filepath.Join(t.TempDir(), "invalid-mind.json")
						if err := os.WriteFile(source, []byte(test.raw), 0600); err != nil {
							t.Fatal(err)
						}
					}
					caller := &mindTestCaller{dry: dryRun}
					err := mindExecute(t, caller, "create", "--name", "Invalid mind", "--request-id", "request", "--source", source, "--yes")
					if err == nil || apperrors.ExitCode(err) != apperrors.ExitCodeValidation {
						t.Fatalf("want local validation/exit 3, got %v", err)
					}
					if caller.calls != 0 {
						t.Fatalf("invalid OpenNodes reached remote: %d calls", caller.calls)
					}
				})
			}
		}
	}
	for _, topic := range []string{`{"id":"a","type":"topic","text":null}`, `{"id":"a","type":"shape","text":"x"}`, `{"id":"a","type":"topic","text":"x","parentId":"r"}`} {
		caller := &mindTestCaller{}
		err := mindExecute(t, caller, "node", "create", "--node-id", "file", "--page-id", "page", "--parent-topic-id", "root", "--request-id", "request", "--topic", topic, "--yes")
		if err == nil || apperrors.ExitCode(err) != apperrors.ExitCodeValidation || caller.calls != 0 {
			t.Fatalf("topic validation failed: err=%v calls=%d", err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageMindWikiBlankCreationDoesNotAcceptInitialContent(t *testing.T) {
	for _, withSource := range []bool{false, true} {
		t.Run(fmt.Sprintf("source=%v", withSource), func(t *testing.T) {
			caller := &mindTestCaller{allowedServer: "doc", response: `{"nodeId":"blank-mind"}`}
			installMindCaller(t, caller)
			cmd := newWikiCommand()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			args := []string{"node", "create", "--workspace", "workspace", "--name", "Blank mind", "--type", "amind"}
			if withSource {
				args = append(args, "--source", mindTestSource)
			}
			cmd.SetArgs(args)
			err := corecmd.ExecuteForTest(cmd)
			if withSource {
				if err == nil || apperrors.ExitCode(err) != apperrors.ExitCodeValidation || caller.calls != 0 {
					t.Fatalf("wiki accepted initialization content: err=%v calls=%d", err, caller.calls)
				}
				return
			}
			if err != nil || caller.calls != 1 || caller.tool != "create_file" || caller.args["type"] != "amind" {
				t.Fatalf("blank creation: err=%v calls=%d tool=%s args=%v", err, caller.calls, caller.tool, caller.args)
			}
			if _, exists := caller.args["source"]; exists {
				t.Fatal("wiki blank creation included initial content")
			}
		})
	}
}
