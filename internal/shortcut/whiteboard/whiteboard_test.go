// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type whiteboardCoverageCall struct {
	server string
	tool   string
	args   map[string]any
}

type whiteboardCoverageCaller struct {
	responses map[string][]string
	calls     []whiteboardCoverageCall
	dry       bool
}

func (caller *whiteboardCoverageCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	caller.calls = append(caller.calls, whiteboardCoverageCall{server: server, tool: tool, args: args})
	queue := caller.responses[tool]
	if len(queue) == 0 {
		return nil, errors.New("missing fake response for " + tool)
	}
	caller.responses[tool] = queue[1:]
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: queue[0]}}}, nil
}

func (*whiteboardCoverageCaller) Format() string      { return "json" }
func (caller *whiteboardCoverageCaller) DryRun() bool { return caller.dry }
func (*whiteboardCoverageCaller) Fields() string      { return "" }
func (*whiteboardCoverageCaller) JQ() string          { return "" }

func runWhiteboardCoverage(t *testing.T, declaration shortcut.Shortcut, caller *whiteboardCoverageCaller, stdin string, args ...string) error {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	service := &cobra.Command{Use: "whiteboard"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	root.SetArgs(append([]string{"whiteboard", declaration.Command}, args...))
	return root.Execute()
}

func directWhiteboardRuntime(t *testing.T, declaration shortcut.Shortcut, caller *whiteboardCoverageCaller, args ...string) *shortcut.RuntimeContext {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Flags().Parse(args); err != nil {
		t.Fatal(err)
	}
	return shortcut.RuntimeContextForTest(cmd, declaration)
}

func validWhiteboardQueryResponse(nodes string) string {
	var decoded []any
	if err := json.Unmarshal([]byte(nodes), &decoded); err != nil {
		panic(err)
	}
	result, err := json.Marshal(map[string]any{
		"schemaVersion": "1.0", "catalogVersion": "dml-v1",
		"pages": []any{map[string]any{"id": "page-1", "nodes": decoded}},
	})
	if err != nil {
		panic(err)
	}
	envelope, err := json.Marshal(map[string]any{
		"success": true, "resultJson": string(result),
		"resultSummary": map[string]any{
			"nodeCount": len(decoded), "pageCount": 1, "readOnlyNodeCount": 0,
			"unknownNodeCount": 0, "resultBytes": len(result), "resultSha256": "0123456789abcdef",
		},
	})
	if err != nil {
		panic(err)
	}
	return string(envelope)
}

func validWhiteboardUpdateResponse(mode string, requestIDs, realIDs []string, deleted int) string {
	idMap := make(map[string]any, len(requestIDs))
	created := make([]any, len(realIDs))
	for index := range requestIDs {
		idMap[requestIDs[index]] = realIDs[index]
		created[index] = realIDs[index]
	}
	result, err := json.Marshal(map[string]any{
		"mode": mode, "createdNodeIds": created, "idMap": idMap,
		"deletedNodeCount": deleted, "message": "completed",
	})
	if err != nil {
		panic(err)
	}
	envelope, err := json.Marshal(map[string]any{
		"success": true, "nodeId": "doc", "partId": "part", "resultJson": string(result),
	})
	if err != nil {
		panic(err)
	}
	return string(envelope)
}

func TestCrossPlatformCoverageWhiteboardQueryRejectsFalseSuccessAndMalformedNodes(t *testing.T) {
	broken := map[string]map[string]any{
		"empty":                 {},
		"missing success":       {"resultJson": map[string]any{}},
		"success false":         {"success": false, "resultJson": map[string]any{}},
		"success wrong type":    {"success": "true", "resultJson": map[string]any{}},
		"missing result":        {"success": true},
		"result wrong type":     {"success": true, "resultJson": []any{}},
		"result invalid string": {"success": true, "resultJson": "{"},
		"missing pages": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1",
		}},
		"empty pages": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{},
		}},
		"missing page id": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{map[string]any{"nodes": []any{}}},
		}},
		"duplicate page id": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{
				map[string]any{"id": "page", "nodes": []any{}}, map[string]any{"id": "page", "nodes": []any{}},
			},
		}},
		"missing nodes": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{map[string]any{"id": "page"}},
		}},
		"nodes wrong type": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{map[string]any{"id": "page", "nodes": map[string]any{}}},
		}},
		"bad node": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{map[string]any{"id": "page", "nodes": []any{"bad"}}},
		}},
		"missing node id": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{map[string]any{"id": "page", "nodes": []any{map[string]any{"type": "text"}}}},
		}},
		"cross-page duplicate node id": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{
				map[string]any{"id": "page-1", "nodes": []any{map[string]any{"id": "same", "type": "text"}}},
				map[string]any{"id": "page-2", "nodes": []any{map[string]any{"id": "same", "type": "shape"}}},
			},
		}},
		"missing summary": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{map[string]any{"id": "page", "nodes": []any{}}},
		}},
		"summary count conflict": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{map[string]any{"id": "page", "nodes": []any{}}},
		}, "resultSummary": map[string]any{
			"nodeCount": float64(1), "pageCount": float64(1), "readOnlyNodeCount": float64(0),
			"unknownNodeCount": float64(0), "resultBytes": float64(2), "resultSha256": "hash",
		}},
	}
	for name, payload := range broken {
		t.Run(name, func(t *testing.T) {
			if _, err := projectWhiteboardQuery(payload, "doc", "part"); err == nil {
				t.Fatalf("payload unexpectedly accepted: %#v", payload)
			}
		})
	}

	explicitEmpty := map[string]any{
		"success": true,
		"resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{map[string]any{"id": "page", "nodes": []any{}}},
		},
		"resultSummary": map[string]any{
			"nodeCount": float64(0), "pageCount": float64(1), "readOnlyNodeCount": float64(0),
			"unknownNodeCount": float64(0), "resultBytes": float64(2), "resultSha256": "hash",
		},
	}
	got, err := projectWhiteboardQuery(explicitEmpty, "doc", "part")
	if err != nil || got["nodeId"] != "doc" || got["partId"] != "part" {
		t.Fatalf("explicit empty result = %#v, err=%v", got, err)
	}
	source := got["source"].(map[string]any)
	pages := source["pages"].([]any)
	if len(pages) != 1 || len(pages[0].(map[string]any)["nodes"].([]any)) != 0 {
		t.Fatalf("explicit nested empty was not preserved: %#v", source)
	}
}

func TestCrossPlatformCoverageWhiteboardSourceValidationFailsClosed(t *testing.T) {
	for name, source := range map[string]string{
		"empty":             "",
		"invalid json":      "{",
		"trailing json":     `{} {}`,
		"missing source":    `{}`,
		"unknown top field": `{"extra":true}`,
		"wrong schema":      `{"source":{"schemaVersion":"2.0","catalogVersion":"dml-v1","nodes":[]}}`,
		"wrong catalog":     `{"source":{"schemaVersion":"1.0","catalogVersion":"v2","nodes":[]}}`,
		"missing nodes":     `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1"}}`,
		"nodes wrong type":  `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":{}}}`,
		"append empty":      `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[]}}`,
		"bad node":          `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[1]}}`,
		"missing node id":   `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"type":"text"}]}}`,
		"duplicate node id": `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"same","type":"text"},{"id":"same","type":"shape"}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseWhiteboardSource(source); err == nil {
				t.Fatalf("source unexpectedly accepted: %q", source)
			}
		})
	}

	for _, source := range []string{
		`{"overwrite":true,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[]}}`,
		`{"overwrite":false,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`,
	} {
		if _, err := parseWhiteboardSource(source); err != nil {
			t.Fatalf("valid source rejected: %v", err)
		}
	}
}

func TestCrossPlatformCoverageWhiteboardExactToolsConfirmationAndReadback(t *testing.T) {
	queryCaller := &whiteboardCoverageCaller{responses: map[string][]string{
		"read_whiteboard_content": {validWhiteboardQueryResponse(`[{"id":"n1","type":"text"}]`)},
	}}
	if err := runWhiteboardCoverage(t, Query, queryCaller, "", "--node", "doc", "--part-id", "part"); err != nil {
		t.Fatal(err)
	}
	if len(queryCaller.calls) != 1 || queryCaller.calls[0].server != "whiteboard" || queryCaller.calls[0].tool != "read_whiteboard_content" {
		t.Fatalf("query calls = %#v", queryCaller.calls)
	}

	source := `{"overwrite":false,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`
	declined := &whiteboardCoverageCaller{responses: map[string][]string{}}
	if err := runWhiteboardCoverage(t, Update, declined, "", "--node", "doc", "--part-id", "part", "--source", source); err == nil {
		t.Fatal("update without confirmation unexpectedly succeeded")
	}
	if len(declined.calls) != 0 {
		t.Fatalf("calls before confirmation = %#v", declined.calls)
	}

	invalid := &whiteboardCoverageCaller{responses: map[string][]string{}}
	if err := runWhiteboardCoverage(t, Update, invalid, "", "--node", "doc", "--part-id", "part", "--source", `{}`, "--yes"); err == nil {
		t.Fatal("invalid update source unexpectedly succeeded")
	}
	if len(invalid.calls) != 0 {
		t.Fatalf("invalid source reached remote calls = %#v", invalid.calls)
	}

	dryRun := &whiteboardCoverageCaller{dry: true, responses: map[string][]string{}}
	if err := runWhiteboardCoverage(t, Update, dryRun, "", "--node", "doc", "--part-id", "part", "--source", source, "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if len(dryRun.calls) != 0 {
		t.Fatalf("dry-run reached remote calls = %#v", dryRun.calls)
	}

	missingReceipt := &whiteboardCoverageCaller{responses: map[string][]string{
		"update_whiteboard": {`{"success":true}`},
	}}
	if err := runWhiteboardCoverage(t, Update, missingReceipt, "", "--node", "doc", "--part-id", "part", "--source", source, "--yes"); err == nil {
		t.Fatal("empty terminal receipt unexpectedly succeeded")
	}
	if len(missingReceipt.calls) != 1 {
		t.Fatalf("missing receipt calls = %#v", missingReceipt.calls)
	}

	verified := &whiteboardCoverageCaller{responses: map[string][]string{
		"update_whiteboard":       {validWhiteboardUpdateResponse("append", []string{"n1"}, []string{"real-1"}, 0)},
		"read_whiteboard_content": {validWhiteboardQueryResponse(`[{"id":"real-1","type":"text","source":"page"}]`)},
	}}
	if err := runWhiteboardCoverage(t, Update, verified, "", "--node", "doc", "--part-id", "part", "--source", source, "--yes"); err != nil {
		t.Fatal(err)
	}
	if len(verified.calls) != 2 || verified.calls[0].tool != "update_whiteboard" || verified.calls[1].tool != "read_whiteboard_content" {
		t.Fatalf("verified calls = %#v", verified.calls)
	}
	if verified.calls[0].args["nodeId"] != "doc" || verified.calls[0].args["partId"] != "part" || verified.calls[1].args["nodeId"] != "doc" || verified.calls[1].args["partId"] != "part" {
		t.Fatalf("stable target identity was not preserved: %#v", verified.calls)
	}

	mismatch := &whiteboardCoverageCaller{responses: map[string][]string{
		"update_whiteboard":       {validWhiteboardUpdateResponse("append", []string{"n1"}, []string{"real-1"}, 0)},
		"read_whiteboard_content": {validWhiteboardQueryResponse(`[{"id":"other","type":"text"}]`)},
	}}
	if err := runWhiteboardCoverage(t, Update, mismatch, "", "--node", "doc", "--part-id", "part", "--source", source, "--yes"); err == nil {
		t.Fatal("readback mismatch unexpectedly succeeded")
	}
	if len(mismatch.calls) != 2 {
		t.Fatalf("readback mismatch calls = %#v", mismatch.calls)
	}

	richSource := `{"overwrite":false,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text","text":"wanted","style":{"fontSize":16},"points":[1,2]}]}}`
	fieldMismatch := &whiteboardCoverageCaller{responses: map[string][]string{
		"update_whiteboard":       {validWhiteboardUpdateResponse("append", []string{"n1"}, []string{"real-1"}, 0)},
		"read_whiteboard_content": {validWhiteboardQueryResponse(`[{"id":"real-1","type":"text","text":"different","style":{"fontSize":16},"points":[1,2]}]`)},
	}}
	if err := runWhiteboardCoverage(t, Update, fieldMismatch, "", "--node", "doc", "--part-id", "part", "--source", richSource, "--yes"); err == nil {
		t.Fatal("request-critical field mismatch unexpectedly succeeded")
	}
}

func TestCrossPlatformCoverageWhiteboardReceiptRejectsFalseSuccessAndBadIdentityEvidence(t *testing.T) {
	source := `{"overwrite":false,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`
	for name, response := range map[string]string{
		"missing target":        `{"success":true,"resultJson":{"mode":"append","createdNodeIds":["real-1"],"idMap":{"n1":"real-1"},"deletedNodeCount":0,"message":"done"}}`,
		"target mismatch":       `{"success":true,"nodeId":"other","partId":"part","resultJson":{"mode":"append","createdNodeIds":["real-1"],"idMap":{"n1":"real-1"},"deletedNodeCount":0,"message":"done"}}`,
		"missing collections":   `{"success":true,"nodeId":"doc","partId":"part","resultJson":{"mode":"append","deletedNodeCount":0,"message":"done"}}`,
		"created wrong type":    `{"success":true,"nodeId":"doc","partId":"part","resultJson":{"mode":"append","createdNodeIds":{},"idMap":{"n1":"real-1"},"deletedNodeCount":0,"message":"done"}}`,
		"id map wrong type":     `{"success":true,"nodeId":"doc","partId":"part","resultJson":{"mode":"append","createdNodeIds":["real-1"],"idMap":[],"deletedNodeCount":0,"message":"done"}}`,
		"identity disagreement": `{"success":true,"nodeId":"doc","partId":"part","resultJson":{"mode":"append","createdNodeIds":["real-1"],"idMap":{"n1":"real-2"},"deletedNodeCount":0,"message":"done"}}`,
		"missing message":       `{"success":true,"nodeId":"doc","partId":"part","resultJson":{"mode":"append","createdNodeIds":["real-1"],"idMap":{"n1":"real-1"},"deletedNodeCount":0}}`,
		"append deleted nodes":  `{"success":true,"nodeId":"doc","partId":"part","resultJson":{"mode":"append","createdNodeIds":["real-1"],"idMap":{"n1":"real-1"},"deletedNodeCount":1,"message":"done"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{responses: map[string][]string{"update_whiteboard": {response}}}
			if err := runWhiteboardCoverage(t, Update, caller, "", "--node", "doc", "--part-id", "part", "--source", source, "--yes"); err == nil {
				t.Fatal("malformed terminal receipt unexpectedly succeeded")
			}
			if len(caller.calls) != 1 {
				t.Fatalf("malformed receipt calls = %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageWhiteboardReadbackNormalizesRequestScopedReferences(t *testing.T) {
	idMap := map[string]string{"frame": "real-frame", "left": "real-left"}
	requested := map[string]any{
		"parentId": "frame",
		"start": map[string]any{
			"type":    "node",
			"nodeRef": map[string]any{"scope": "request", "id": "left"},
		},
	}
	actual := map[string]any{
		"parentId": "real-frame",
		"start": map[string]any{
			"type":          "node",
			"nodeRef":       map[string]any{"scope": "document", "id": "real-left"},
			"resolvedPoint": map[string]any{"x": float64(1), "y": float64(2)},
		},
	}
	normalized := normalizeRequestedReadback(requested, idMap, "node")
	if err := requireRequestedValue(normalized, actual, "node connector"); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageWhiteboardPublicContractsStayStrictAndUnified(t *testing.T) {
	for _, declaration := range []shortcut.Shortcut{Query, Update} {
		if declaration.Contract.Empty() || declaration.Contract.Result == nil {
			t.Errorf("%s missing Contract.Result", declaration.Command)
		}
		if declaration.Contract.Pagination != nil {
			t.Errorf("%s publishes unsupported pagination", declaration.Command)
		}
		if declaration.OutputRollout != output.RolloutUnifiedActive {
			t.Errorf("%s rollout=%q", declaration.Command, declaration.OutputRollout)
		}
		if declaration.Contract.Interface == nil || declaration.Contract.Interface.Availability != "available" || strings.TrimSpace(declaration.Contract.Interface.Reason) == "" {
			t.Errorf("%s interface=%+v", declaration.Command, declaration.Contract.Interface)
		}
	}
	if Query.Safety.Confirmation != "not_required" || Query.Safety.Effect != "read" {
		t.Errorf("query safety=%+v", Query.Safety)
	}
	if Update.Safety.Confirmation != "user_required" || Update.Safety.Effect != "write" {
		t.Errorf("update safety=%+v", Update.Safety)
	}
}

func TestCrossPlatformCoverageWhiteboardExecutorErrorsAndExactCallOrder(t *testing.T) {
	queryCallError := &whiteboardCoverageCaller{responses: map[string][]string{}}
	if err := runWhiteboardCoverage(t, Query, queryCallError, "", "--node", "doc", "--part-id", "part"); err == nil || len(queryCallError.calls) != 1 {
		t.Fatalf("query call error=%v calls=%#v", err, queryCallError.calls)
	}
	queryProjectionError := &whiteboardCoverageCaller{responses: map[string][]string{
		toolQuery: {`{"success":true}`},
	}}
	if err := runWhiteboardCoverage(t, Query, queryProjectionError, "", "--node", "doc", "--part-id", "part"); err == nil || len(queryProjectionError.calls) != 1 {
		t.Fatalf("query projection error=%v calls=%#v", err, queryProjectionError.calls)
	}

	invalidDirect := directWhiteboardRuntime(t, Update, &whiteboardCoverageCaller{responses: map[string][]string{}},
		"--node", "doc", "--part-id", "part", "--source", `{}`)
	if err := Update.Execute(invalidDirect); err == nil {
		t.Fatal("direct update accepted invalid source")
	}

	overwriteSource := `{"overwrite":true,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[]}}`
	overwriteDryRun := &whiteboardCoverageCaller{dry: true, responses: map[string][]string{}}
	if err := runWhiteboardCoverage(t, Update, overwriteDryRun, "", "--node", "doc", "--part-id", "part", "--source", overwriteSource, "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if len(overwriteDryRun.calls) != 0 {
		t.Fatalf("overwrite dry-run calls=%#v", overwriteDryRun.calls)
	}

	appendSource := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`
	writeCallError := &whiteboardCoverageCaller{responses: map[string][]string{}}
	if err := runWhiteboardCoverage(t, Update, writeCallError, "", "--node", "doc", "--part-id", "part", "--source", appendSource, "--yes"); err == nil || len(writeCallError.calls) != 1 || writeCallError.calls[0].tool != toolUpdate {
		t.Fatalf("write call error=%v calls=%#v", err, writeCallError.calls)
	}

	readCallError := &whiteboardCoverageCaller{responses: map[string][]string{
		toolUpdate: {validWhiteboardUpdateResponse("append", []string{"n1"}, []string{"real-1"}, 0)},
	}}
	if err := runWhiteboardCoverage(t, Update, readCallError, "", "--node", "doc", "--part-id", "part", "--source", appendSource, "--yes"); err == nil || len(readCallError.calls) != 2 || readCallError.calls[1].tool != toolQuery {
		t.Fatalf("read call error=%v calls=%#v", err, readCallError.calls)
	}

	readProjectionError := &whiteboardCoverageCaller{responses: map[string][]string{
		toolUpdate: {validWhiteboardUpdateResponse("append", []string{"n1"}, []string{"real-1"}, 0)},
		toolQuery:  {`{"success":true}`},
	}}
	if err := runWhiteboardCoverage(t, Update, readProjectionError, "", "--node", "doc", "--part-id", "part", "--source", appendSource, "--yes"); err == nil || len(readProjectionError.calls) != 2 {
		t.Fatalf("read projection error=%v calls=%#v", err, readProjectionError.calls)
	}

	typeMismatch := &whiteboardCoverageCaller{responses: map[string][]string{
		toolUpdate: {validWhiteboardUpdateResponse("append", []string{"n1"}, []string{"real-1"}, 0)},
		toolQuery:  {validWhiteboardQueryResponse(`[{"id":"real-1","type":"shape"}]`)},
	}}
	if err := runWhiteboardCoverage(t, Update, typeMismatch, "", "--node", "doc", "--part-id", "part", "--source", appendSource, "--yes"); err == nil || len(typeMismatch.calls) != 2 {
		t.Fatalf("type mismatch error=%v calls=%#v", err, typeMismatch.calls)
	}

	overwriteVerified := &whiteboardCoverageCaller{responses: map[string][]string{
		toolUpdate: {validWhiteboardUpdateResponse("overwrite", nil, nil, 1)},
		toolQuery:  {validWhiteboardQueryResponse(`[{"id":"master-1","type":"shape","source":"master"}]`)},
	}}
	if err := runWhiteboardCoverage(t, Update, overwriteVerified, "", "--node", "doc", "--part-id", "part", "--source", overwriteSource, "--yes"); err != nil {
		t.Fatal(err)
	}
	if len(overwriteVerified.calls) != 2 || overwriteVerified.calls[0].args["mode"] != "overwrite" || overwriteVerified.calls[1].tool != toolQuery {
		t.Fatalf("overwrite call order=%#v", overwriteVerified.calls)
	}
}

func TestCrossPlatformCoverageWhiteboardQueryEnvelopeSummaryAndMessageMatrix(t *testing.T) {
	baseResult := func() map[string]any {
		return map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1",
			"pages": []any{
				map[string]any{
					"id": "page",
					"nodes": []any{
						map[string]any{"id": "node", "type": "text"},
					},
				},
			},
		}
	}
	baseSummary := func() map[string]any {
		return map[string]any{
			"nodeCount": 1, "pageCount": 1, "readOnlyNodeCount": 0,
			"unknownNodeCount": 0, "resultBytes": 1, "resultSha256": "hash",
		}
	}
	fixtures := map[string]map[string]any{
		"wrong schema":  {"success": true, "resultJson": map[string]any{"schemaVersion": "2.0", "catalogVersion": "dml-v1", "pages": []any{}}, "resultSummary": baseSummary()},
		"wrong catalog": {"success": true, "resultJson": map[string]any{"schemaVersion": "1.0", "catalogVersion": "v2", "pages": []any{}}, "resultSummary": baseSummary()},
		"bad page item": {"success": true, "resultJson": map[string]any{"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{"bad"}}, "resultSummary": baseSummary()},
		"missing node type": {"success": true, "resultJson": map[string]any{
			"schemaVersion": "1.0", "catalogVersion": "dml-v1",
			"pages": []any{
				map[string]any{
					"id": "page",
					"nodes": []any{
						map[string]any{"id": "node"},
					},
				},
			},
		}, "resultSummary": baseSummary()},
		"empty summary":      {"success": true, "resultJson": baseResult(), "resultSummary": map[string]any{}},
		"wrong message":      {"success": true, "resultJson": baseResult(), "resultSummary": baseSummary(), "message": 1},
		"conflicting error":  {"success": true, "resultJson": baseResult(), "resultSummary": baseSummary(), "errorMsg": "failed"},
		"read only too high": {"success": true, "resultJson": baseResult(), "resultSummary": map[string]any{"nodeCount": 1, "pageCount": 1, "readOnlyNodeCount": 2, "unknownNodeCount": 0, "resultBytes": 1, "resultSha256": "hash"}},
		"unknown too high":   {"success": true, "resultJson": baseResult(), "resultSummary": map[string]any{"nodeCount": 1, "pageCount": 1, "readOnlyNodeCount": 0, "unknownNodeCount": 2, "resultBytes": 1, "resultSha256": "hash"}},
		"bad result bytes":   {"success": true, "resultJson": baseResult(), "resultSummary": map[string]any{"nodeCount": 1, "pageCount": 1, "readOnlyNodeCount": 0, "unknownNodeCount": 0, "resultBytes": -1, "resultSha256": "hash"}},
		"missing digest":     {"success": true, "resultJson": baseResult(), "resultSummary": map[string]any{"nodeCount": 1, "pageCount": 1, "readOnlyNodeCount": 0, "unknownNodeCount": 0, "resultBytes": 1}},
	}
	for name, fixture := range fixtures {
		if projected, err := projectWhiteboardQuery(fixture, "doc", "part"); err == nil {
			t.Errorf("%s returned success: %#v", name, projected)
		}
	}
	valid := map[string]any{"success": true, "resultJson": baseResult(), "resultSummary": baseSummary(), "message": "ok"}
	projected, err := projectWhiteboardQuery(valid, " doc ", " part ")
	if err != nil || projected["nodeId"] != "doc" || projected["partId"] != "part" || projected["message"] != "ok" {
		t.Fatalf("valid message projection=%#v err=%v", projected, err)
	}
}

func TestCrossPlatformCoverageWhiteboardReceiptAndResultJSONRemainingMatrix(t *testing.T) {
	expected, err := parseWhiteboardSource(`{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	target := map[string]any{"nodeId": "doc", "partId": "part"}
	fixtures := map[string]map[string]any{
		"business failure":        {"success": false},
		"missing terminal result": {"success": true, "nodeId": "doc", "partId": "part"},
		"empty result map":        {"success": true, "nodeId": "doc", "partId": "part", "resultJson": map[string]any{}},
		"empty result string":     {"success": true, "nodeId": "doc", "partId": "part", "resultJson": "   "},
		"invalid result string":   {"success": true, "nodeId": "doc", "partId": "part", "resultJson": "{"},
		"trailing result string":  {"success": true, "nodeId": "doc", "partId": "part", "resultJson": `{"mode":"append"} {}`},
		"wrong result type":       {"success": true, "nodeId": "doc", "partId": "part", "resultJson": []any{}},
		"wrong mode": {"success": true, "nodeId": "doc", "partId": "part", "resultJson": map[string]any{
			"mode": "overwrite", "message": "done", "createdNodeIds": []any{"real"}, "idMap": map[string]any{"n1": "real"}, "deletedNodeCount": 0,
		}},
		"created count mismatch": {"success": true, "nodeId": "doc", "partId": "part", "resultJson": map[string]any{
			"mode": "append", "message": "done", "createdNodeIds": []any{}, "idMap": map[string]any{"n1": "real"}, "deletedNodeCount": 0,
		}},
		"empty created identity": {"success": true, "nodeId": "doc", "partId": "part", "resultJson": map[string]any{
			"mode": "append", "message": "done", "createdNodeIds": []any{" "}, "idMap": map[string]any{"n1": "real"}, "deletedNodeCount": 0,
		}},
		"duplicate created identity": {"success": true, "nodeId": "doc", "partId": "part", "resultJson": map[string]any{
			"mode": "append", "message": "done", "createdNodeIds": []any{"real", "real"}, "idMap": map[string]any{"n1": "real"}, "deletedNodeCount": 0,
		}},
		"empty id map key": {"success": true, "nodeId": "doc", "partId": "part", "resultJson": map[string]any{
			"mode": "append", "message": "done", "createdNodeIds": []any{"real"}, "idMap": map[string]any{" ": "real"}, "deletedNodeCount": 0,
		}},
		"id map count mismatch": {"success": true, "nodeId": "doc", "partId": "part", "resultJson": map[string]any{
			"mode": "append", "message": "done", "createdNodeIds": []any{"real"}, "idMap": map[string]any{}, "deletedNodeCount": 0,
		}},
	}
	for name, fixture := range fixtures {
		if receipt, err := requireWhiteboardUpdateReceipt(fixture, target, "append", expected); err == nil {
			t.Errorf("%s returned success: %#v", name, receipt)
		}
	}
}

func TestCrossPlatformCoverageWhiteboardReadbackComparatorAndScalarMatrices(t *testing.T) {
	if count := whiteboardPageOwnedNodeCount([]map[string]any{
		{"id": "master", "source": "master"}, {"id": "page", "source": "page"}, {"id": "plain"},
	}); count != 2 {
		t.Fatalf("page-owned count=%d", count)
	}

	comparisons := []struct {
		name     string
		expected any
		actual   any
		wantErr  bool
	}{
		{"map wrong type", map[string]any{"a": 1}, "bad", true},
		{"map missing key", map[string]any{"a": 1}, map[string]any{}, true},
		{"array wrong length", []any{1}, []any{}, true},
		{"array child mismatch", []any{1}, []any{2}, true},
		{"array equal", []any{1}, []any{float64(1)}, false},
		{"number wrong type", json.Number("1"), "1", true},
		{"number mismatch", json.Number("1"), float64(2), true},
		{"number equal cross type", json.Number("1"), int(1), false},
		{"large adjacent JSON integers mismatch", json.Number("9007199254740992"), json.Number("9007199254740993"), true},
		{"large JSON integer equal", json.Number("9007199254740993"), json.Number("9007199254740993"), false},
		{"scalar mismatch", "one", "two", true},
		{"scalar equal", "same", "same", false},
	}
	for _, test := range comparisons {
		err := requireRequestedValue(test.expected, test.actual, test.name)
		if (err != nil) != test.wantErr {
			t.Errorf("%s err=%v wantErr=%t", test.name, err, test.wantErr)
		}
	}

	for _, test := range []struct {
		value any
		ok    bool
	}{
		{json.Number("1.5"), true}, {json.Number("1e3"), true}, {json.Number("bad"), false},
		{float64(1), true}, {math.NaN(), false}, {math.Inf(1), false},
		{int(1), true}, {"1", false},
	} {
		_, ok := numericValue(test.value)
		if ok != test.ok {
			t.Errorf("numericValue(%#v) ok=%t want=%t", test.value, ok, test.ok)
		}
	}

	for _, test := range []struct {
		value any
		want  int
		ok    bool
	}{
		{int(1), 1, true}, {int(-1), -1, false},
		{float64(2), 2, true}, {float64(-1), 0, false}, {1.5, 0, false}, {math.NaN(), 0, false}, {math.Inf(1), 0, false},
		{json.Number("3"), 3, true}, {json.Number("bad"), 0, false}, {json.Number("-1"), 0, false},
		{"4", 0, false},
	} {
		got, ok := nonNegativeInt(test.value)
		if got != test.want || ok != test.ok {
			t.Errorf("nonNegativeInt(%#v)=(%d,%t), want (%d,%t)", test.value, got, ok, test.want, test.ok)
		}
	}

	decoder := json.NewDecoder(strings.NewReader(`{} ]`))
	var first map[string]any
	if err := decoder.Decode(&first); err != nil {
		t.Fatal(err)
	}
	if err := requireJSONEOF(decoder); err == nil {
		t.Fatal("malformed trailing JSON returned EOF success")
	}
}

func TestCrossPlatformCoverageWhiteboardBlankTargetsStopBeforeRPC(t *testing.T) {
	validSource := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`
	tests := []struct {
		name        string
		declaration shortcut.Shortcut
		args        []string
	}{
		{"query blank node", Query, []string{"--node", "   ", "--part-id", "part"}},
		{"query blank part", Query, []string{"--node", "doc", "--part-id", "   "}},
		{"update blank node", Update, []string{"--node", "   ", "--part-id", "part", "--source", validSource, "--yes"}},
		{"update blank part", Update, []string{"--node", "doc", "--part-id", "   ", "--source", validSource, "--yes"}},
	}
	for _, test := range tests {
		caller := &whiteboardCoverageCaller{responses: map[string][]string{}}
		if err := runWhiteboardCoverage(t, test.declaration, caller, "", test.args...); err == nil {
			t.Errorf("%s returned success", test.name)
		}
		if len(caller.calls) != 0 {
			t.Errorf("%s crossed RPC boundary: %#v", test.name, caller.calls)
		}
	}
	directBlankNode := directWhiteboardRuntime(t, Query, &whiteboardCoverageCaller{responses: map[string][]string{}}, "--node", "   ", "--part-id", "part")
	if err := Query.Validate(directBlankNode); err == nil {
		t.Fatal("direct query validation accepted blank node")
	}
	directBlankPart := directWhiteboardRuntime(t, Query, &whiteboardCoverageCaller{responses: map[string][]string{}}, "--node", "doc", "--part-id", "   ")
	if err := Query.Validate(directBlankPart); err == nil {
		t.Fatal("direct query validation accepted blank part")
	}
	directUpdate := directWhiteboardRuntime(t, Update, &whiteboardCoverageCaller{responses: map[string][]string{}},
		"--node", "   ", "--part-id", "part", "--source", validSource)
	if err := Update.Validate(directUpdate); err == nil {
		t.Fatal("direct update validation accepted blank node")
	}
}

func TestCrossPlatformCoverageWhiteboardSourceAndDirectVerificationRemainingMatrix(t *testing.T) {
	for name, source := range map[string]string{
		"null nodes":   `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":null}}`,
		"missing type": `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"node"}]}}`,
	} {
		if parsed, err := parseWhiteboardSource(source); err == nil {
			t.Errorf("%s returned success: %#v", name, parsed)
		}
	}
	t.Run("marshal failure", func(t *testing.T) {
		testseam.Swap(t, &whiteboardMarshalNodes, func(any) ([]byte, error) {
			return nil, errors.New("injected marshal failure")
		})
		if parsed, err := parseWhiteboardSource(`{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"node","type":"text"}]}}`); err == nil {
			t.Fatalf("marshal failure returned success: %#v", parsed)
		}
	})

	expected := &parsedUpdate{Overwrite: true, Nodes: []map[string]any{{"id": "request", "type": "text"}}}
	if err := verifyWhiteboardUpdate(expected, map[string]any{}, map[string]string{"request": "real"}); err == nil {
		t.Fatal("missing source returned success")
	}
	if err := verifyWhiteboardUpdate(expected, map[string]any{"source": map[string]any{"pages": "bad"}}, map[string]string{"request": "real"}); err == nil {
		t.Fatal("bad pages returned success")
	}
	overwriteCountMismatch := map[string]any{
		"source": map[string]any{
			"pages": []any{
				map[string]any{
					"id": "page",
					"nodes": []any{
						map[string]any{"id": "real", "type": "text"},
						map[string]any{"id": "extra", "type": "text"},
					},
				},
			},
		},
	}
	if err := verifyWhiteboardUpdate(expected, overwriteCountMismatch, map[string]string{"request": "real"}); err == nil {
		t.Fatal("overwrite count mismatch returned success")
	}
}
