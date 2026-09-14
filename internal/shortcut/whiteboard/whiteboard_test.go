// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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
	_, err := runWhiteboardCoverageOutput(t, declaration, caller, stdin, args...)
	return err
}

func runWhiteboardCoverageOutput(t *testing.T, declaration shortcut.Shortcut, caller *whiteboardCoverageCaller, stdin string, args ...string) ([]byte, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.SetIn(strings.NewReader(stdin))
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	service := &cobra.Command{Use: "whiteboard"}
	leaf := corecmd.New(shortcut.FromShortcut(declaration))
	service.AddCommand(leaf)
	root.AddCommand(service)
	root.SetArgs(append([]string{"whiteboard", declaration.Command}, args...))
	if err := root.Execute(); err != nil {
		return nil, err
	}
	_, _, err := output.EmitStoredResult(leaf)
	return stdout.Bytes(), err
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

func validStandaloneWhiteboardQueryResponse(nodeID, view string, revision int, nodes string) string {
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
		"success": true, "nodeId": nodeID, "revision": revision, "view": view,
		"resultJson": string(result),
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

func validStandaloneWhiteboardUpdateResponse(nodeID, mode string, previous, committed int, requestIDs, realIDs []string, deleted int) string {
	idMap := make(map[string]any, len(requestIDs))
	created := make([]any, len(realIDs))
	for index := range requestIDs {
		idMap[requestIDs[index]] = realIDs[index]
		created[index] = realIDs[index]
	}
	envelope, err := json.Marshal(map[string]any{
		"success": true, "nodeId": nodeID, "mode": mode, "pageId": "page-1",
		"previousRevision": previous, "committedRevision": committed,
		"createdNodeIds": created, "idMap": idMap, "deletedNodeCount": deleted,
		"idempotentReplay": false, "message": "completed",
	})
	if err != nil {
		panic(err)
	}
	return string(envelope)
}

func TestCrossPlatformCoverageWhiteboardStandaloneReceiptRequestIDCompatibility(t *testing.T) {
	expected, err := parseWhiteboardSource(`{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	request := map[string]any{
		"nodeId": "wb", "mode": "append", "expectedRevision": 12, "requestId": "req-1",
	}
	baseReceipt := func() map[string]any {
		return map[string]any{
			"success": true, "nodeId": "wb", "mode": "append", "pageId": "page-1",
			"previousRevision": 12, "committedRevision": 13,
			"createdNodeIds": []any{"real-1"}, "idMap": map[string]any{"n1": "real-1"},
			"deletedNodeCount": 0, "idempotentReplay": false, "message": "completed",
		}
	}

	withoutEcho, err := requireStandaloneWhiteboardUpdateReceipt(baseReceipt(), request, expected)
	if err != nil {
		t.Fatalf("response without requestId echo was rejected: %v", err)
	}
	if withoutEcho.RequestID != "req-1" {
		t.Fatalf("requestId fallback=%q, want req-1", withoutEcho.RequestID)
	}

	matching := baseReceipt()
	matching["requestId"] = "req-1"
	withEcho, err := requireStandaloneWhiteboardUpdateReceipt(matching, request, expected)
	if err != nil || withEcho.RequestID != "req-1" {
		t.Fatalf("matching requestId echo receipt=%#v err=%v", withEcho, err)
	}

	mismatched := baseReceipt()
	mismatched["requestId"] = "req-other"
	if _, err := requireStandaloneWhiteboardUpdateReceipt(mismatched, request, expected); !hasWhiteboardErrorReason(err, "receipt_request_mismatch") {
		t.Fatalf("mismatched requestId error=%v", err)
	}

	for name, value := range map[string]any{"null": nil, "blank": " ", "wrong type": 1} {
		t.Run(name, func(t *testing.T) {
			malformed := baseReceipt()
			malformed["requestId"] = value
			if _, err := requireStandaloneWhiteboardUpdateReceipt(malformed, request, expected); !hasWhiteboardErrorReason(err, "malformed_receipt_request_id") {
				t.Fatalf("malformed requestId error=%v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageWhiteboardStandaloneReceiptIDMapCompatibility(t *testing.T) {
	expected, err := parseWhiteboardSource(`{"overwrite":true,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"},{"id":"n2","type":"shape"}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	request := map[string]any{
		"nodeId": "wb", "mode": "overwrite", "pageId": "page",
		"expectedRevision": 2, "requestId": "req-1",
	}
	baseReceipt := func() map[string]any {
		return map[string]any{
			"success": true, "nodeId": "wb", "mode": "overwrite", "pageId": "page",
			"previousRevision": 2, "committedRevision": 3,
			"createdNodeIds": []any{"real-1", "real-2"}, "deletedNodeCount": 57,
			"idempotentReplay": false, "message": "done",
		}
	}

	for _, value := range []any{"missing", nil} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			receipt := baseReceipt()
			if value == nil {
				receipt["idMap"] = nil
			}
			got, err := requireStandaloneWhiteboardUpdateReceipt(receipt, request, expected)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"n1": "real-1", "n2": "real-2"}
			if !reflect.DeepEqual(got.IDMap, want) {
				t.Fatalf("derived idMap = %#v, want %#v", got.IDMap, want)
			}
		})
	}

	malformed := baseReceipt()
	malformed["idMap"] = []any{}
	if _, err := requireStandaloneWhiteboardUpdateReceipt(malformed, request, expected); !hasWhiteboardErrorReason(err, "malformed_id_map") {
		t.Fatalf("malformed idMap error=%v", err)
	}
}

func hasWhiteboardErrorReason(err error, reason string) bool {
	var structured *apperrors.Error
	return errors.As(err, &structured) && structured.Reason == reason
}

func TestCrossPlatformCoverageWhiteboardStandaloneStrictRoutingAndReadback(t *testing.T) {
	queryCaller := &whiteboardCoverageCaller{responses: map[string][]string{
		toolQueryStandalone: {validStandaloneWhiteboardQueryResponse("wb", "all", 12, `[{"id":"real-1","type":"text"}]`)},
	}}
	if err := runWhiteboardCoverage(t, Query, queryCaller, "", "--node", "wb", "--view", "all"); err != nil {
		t.Fatal(err)
	}
	if len(queryCaller.calls) != 1 || queryCaller.calls[0].tool != toolQueryStandalone || queryCaller.calls[0].args["partId"] != nil {
		t.Fatalf("query calls = %#v", queryCaller.calls)
	}

	validSource := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`
	updateCaller := &whiteboardCoverageCaller{responses: map[string][]string{
		toolUpdateStandalone: {validStandaloneWhiteboardUpdateResponse("wb", "append", 12, 13, []string{"n1"}, []string{"real-1"}, 0)},
		toolQueryStandalone:  {validStandaloneWhiteboardQueryResponse("wb", "page", 13, `[{"id":"real-1","type":"text"}]`)},
	}}
	if err := runWhiteboardCoverage(t, Update, updateCaller, "",
		"--node", "wb", "--expected-revision", "12", "--request-id", "req-1", "--source", validSource, "--yes"); err != nil {
		t.Fatal(err)
	}
	if len(updateCaller.calls) != 2 || updateCaller.calls[0].tool != toolUpdateStandalone || updateCaller.calls[1].tool != toolQueryStandalone {
		t.Fatalf("update calls = %#v", updateCaller.calls)
	}
	wantWrite := map[string]any{
		"nodeId": "wb", "mode": "append", "nodes": `[{"id":"n1","type":"text"}]`,
		"expectedRevision": 12, "requestId": "req-1",
	}
	if !reflect.DeepEqual(updateCaller.calls[0].args, wantWrite) {
		t.Fatalf("write args = %#v, want %#v", updateCaller.calls[0].args, wantWrite)
	}
	if got := updateCaller.calls[1].args; got["nodeId"] != "wb" || got["view"] != "page" || got["pageId"] != "page-1" {
		t.Fatalf("readback args = %#v", got)
	}
}

func TestCrossPlatformCoverageWhiteboardRoutingNeverFallsBackAcrossKinds(t *testing.T) {
	caller := &whiteboardCoverageCaller{responses: map[string][]string{}}
	if err := runWhiteboardCoverage(t, Query, caller, "", "--node", "wb"); err == nil {
		t.Fatal("missing standalone response unexpectedly succeeded")
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != toolQueryStandalone {
		t.Fatalf("query fell back across kinds: %#v", caller.calls)
	}

	caller = &whiteboardCoverageCaller{responses: map[string][]string{}}
	if err := runWhiteboardCoverage(t, Query, caller, "", "--node", "doc", "--part-id", ""); err == nil {
		t.Fatal("explicit empty part unexpectedly succeeded")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("explicit empty part reached remote call: %#v", caller.calls)
	}
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

func TestCrossPlatformCoverageWhiteboardConnectorValidationStopsBeforeRPC(t *testing.T) {
	valid := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"left","type":"shape"},{"id":"right","type":"shape"},{"id":"line","type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"left"},"anchor":{"mode":"fixed","side":"right"}},"end":{"type":"node","nodeRef":{"scope":"request","id":"right"},"anchor":{"mode":"fixed","side":"left"},"marker":{"catalogId":"arrow.filled"}},"routing":"straight"}]}}`
	if _, err := parseWhiteboardSource(valid); err != nil {
		t.Fatalf("valid connector source rejected: %v", err)
	}

	invalid := map[string]string{
		"document scoped reference":  `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"left","type":"shape"},{"id":"right","type":"shape"},{"id":"line","type":"connector","start":{"type":"node","nodeRef":{"scope":"document","id":"left"}},"end":{"type":"node","nodeRef":{"scope":"request","id":"right"}},"routing":"straight"}]}}`,
		"reference outside request":  `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"left","type":"shape"},{"id":"line","type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"left"}},"end":{"type":"node","nodeRef":{"scope":"request","id":"missing"}},"routing":"straight"}]}}`,
		"reference connector":        `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"left","type":"shape"},{"id":"line","type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"left"}},"end":{"type":"node","nodeRef":{"scope":"request","id":"line"}},"routing":"straight"}]}}`,
		"reference hidden node":      `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"left","type":"shape"},{"id":"right","type":"shape","hidden":true},{"id":"line","type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"left"}},"end":{"type":"node","nodeRef":{"scope":"request","id":"right"}},"routing":"straight"}]}}`,
		"self loop":                  `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"left","type":"shape"},{"id":"line","type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"left"}},"end":{"type":"node","nodeRef":{"scope":"request","id":"left"}},"routing":"straight"}]}}`,
		"connector geometry":         `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"line","type":"connector","x":1,"start":{"type":"point","point":{"x":0,"y":0}},"end":{"type":"point","point":{"x":1,"y":1}},"routing":"straight"}]}}`,
		"query-only resolved point":  `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"left","type":"shape"},{"id":"right","type":"shape"},{"id":"line","type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"left"},"resolvedPoint":{"x":0,"y":0}},"end":{"type":"node","nodeRef":{"scope":"request","id":"right"}},"routing":"straight"}]}}`,
		"invalid anchor":             `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"left","type":"shape"},{"id":"right","type":"shape"},{"id":"line","type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"left"},"anchor":{"mode":"fixed","side":"center"}},"end":{"type":"node","nodeRef":{"scope":"request","id":"right"}},"routing":"straight"}]}}`,
		"query-only anchor position": `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"left","type":"shape"},{"id":"right","type":"shape"},{"id":"line","type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"left"},"anchor":{"mode":"auto","position":0.5}},"end":{"type":"node","nodeRef":{"scope":"request","id":"right"}},"routing":"straight"}]}}`,
		"invalid marker":             `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"line","type":"connector","start":{"type":"point","point":{"x":0,"y":0}},"end":{"type":"point","point":{"x":1,"y":1},"marker":{"catalogId":"triangle"}},"routing":"straight"}]}}`,
		"straight waypoints":         `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"line","type":"connector","start":{"type":"point","point":{"x":0,"y":0}},"end":{"type":"point","point":{"x":1,"y":1}},"routing":"straight","waypoints":[]}]}}`,
		"polyline without waypoint":  `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"line","type":"connector","start":{"type":"point","point":{"x":0,"y":0}},"end":{"type":"point","point":{"x":1,"y":1}},"routing":"polyline"}]}}`,
		"non-finite point":           `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"line","type":"connector","start":{"type":"point","point":{"x":"NaN","y":0}},"end":{"type":"point","point":{"x":1,"y":1}},"routing":"curve"}]}}`,
	}
	for name, source := range invalid {
		t.Run(name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{responses: map[string][]string{}}
			if err := runWhiteboardCoverage(t, Update, caller, "", "--node", "doc", "--part-id", "part", "--source", source, "--yes"); err == nil {
				t.Fatal("invalid connector unexpectedly succeeded")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid connector crossed RPC boundary: %#v", caller.calls)
			}
		})
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
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(Update.Contract.Result.DataSchema, &schema); err != nil {
		t.Fatal(err)
	}
	for _, field := range schema.Required {
		if field == "source" {
			t.Fatal("successful update contract still requires the full source snapshot")
		}
	}
}

func TestCrossPlatformCoverageWhiteboardUpdateSuccessOmitsFullSnapshot(t *testing.T) {
	parsed := &parsedUpdate{Nodes: []map[string]any{{"id": "request-node", "type": "shape"}}}
	projected := map[string]any{
		"source":  map[string]any{"pages": []any{map[string]any{"id": "page", "nodes": []any{map[string]any{"id": "real-node"}}}}},
		"summary": map[string]any{"nodeCount": 1, "pageCount": 1, "resultSha256": "hash"},
	}
	receipt := &verifiedUpdateReceipt{
		Message: "completed", CreatedNodeIDs: []string{"real-node"},
		IDMap: map[string]string{"request-node": "real-node"}, DeletedNodeCount: 0,
	}
	result := projectWhiteboardUpdateSuccess(
		map[string]any{"nodeId": "doc", "partId": "part"}, "append", parsed, projected, receipt,
	)
	if _, exists := result["source"]; exists {
		t.Fatalf("successful update leaked full source snapshot: %#v", result["source"])
	}
	if _, exists := result["pages"]; exists {
		t.Fatalf("successful update leaked pages at top level: %#v", result["pages"])
	}
	if result["verified"] != true || result["verifiedNodeCount"] != 1 || result["summary"] == nil {
		t.Fatalf("successful update lost verification evidence: %#v", result)
	}
	compactReceipt, ok := result["receipt"].(map[string]any)
	if !ok || compactReceipt["message"] != "completed" || compactReceipt["deletedNodeCount"] != 0 {
		t.Fatalf("successful update receipt is incomplete: %#v", result["receipt"])
	}
	if _, exists := compactReceipt["resultJson"]; exists {
		t.Fatalf("successful update leaked raw resultJson: %#v", compactReceipt)
	}
}

func TestCrossPlatformCoverageWhiteboardUpdateReceiptSchemaAndRuntimeBranches(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(Update.Contract.Result.DataSchema, &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	receiptSchema := properties["receipt"].(map[string]any)
	branches, ok := receiptSchema["oneOf"].([]any)
	if !ok || len(branches) != 2 {
		t.Fatal("receipt must declare mutually exclusive execution and preview branches")
	}
	branchRequired := [][]any{
		{"message", "createdNodeIds", "idMap", "deletedNodeCount"},
		{"dryRun", "executed"},
	}
	for index, raw := range branches {
		branch := raw.(map[string]any)
		if !reflect.DeepEqual(branch["required"], branchRequired[index]) || branch["additionalProperties"] != false {
			t.Fatalf("receipt branch %d permits empty, partial or mixed receipts: %#v", index, branch)
		}
	}
	previewProps := branches[1].(map[string]any)["properties"].(map[string]any)
	for field, want := range map[string]bool{"dryRun": true, "executed": false} {
		if got, exists := previewProps[field].(map[string]any)["const"]; !exists || got != want {
			t.Fatalf("preview %s const=%#v, want %v", field, got, want)
		}
	}
	states, ok := schema["oneOf"].([]any)
	if !ok || len(states) != 2 {
		t.Fatal("result must bind verified/source to the receipt branch")
	}
	for index, raw := range states {
		state := raw.(map[string]any)
		props := state["properties"].(map[string]any)
		if props["verified"].(map[string]any)["const"] != (index == 0) {
			t.Fatalf("verified discriminator missing from state %d", index)
		}
		wantReceipt := []any{"message"}
		if index == 1 {
			wantReceipt = []any{"dryRun", "executed"}
			if !reflect.DeepEqual(state["required"], []any{"source"}) || props["verifiedNodeCount"].(map[string]any)["const"] != float64(0) {
				t.Fatal("preview must carry source and zero verified nodes")
			}
		} else if !reflect.DeepEqual(state["not"], map[string]any{"required": []any{"source"}}) {
			t.Fatal("executed result must not contain a full source snapshot")
		}
		if !reflect.DeepEqual(props["receipt"].(map[string]any)["required"], wantReceipt) {
			t.Fatalf("state %d is not bound to its receipt kind", index)
		}
	}

	for _, tc := range []struct {
		name, source, mode, nodes string
		requestIDs, realIDs       []string
		dry                       bool
	}{
		{"append", `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`, "append", `[{"id":"real-1","type":"text"}]`, []string{"n1"}, []string{"real-1"}, false},
		{"clear", `{"overwrite":true,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[]}}`, "overwrite", `[]`, nil, nil, false},
		{"preview", `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`, "append", "", nil, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{dry: tc.dry, responses: map[string][]string{}}
			if !tc.dry {
				caller.responses[toolUpdate] = []string{validWhiteboardUpdateResponse(tc.mode, tc.requestIDs, tc.realIDs, 0)}
				caller.responses[toolQuery] = []string{validWhiteboardQueryResponse(tc.nodes)}
			}
			args := []string{"--node", "doc", "--part-id", "part", "--source", tc.source, "--yes"}
			if tc.dry {
				args = append(args, "--dry-run")
			}
			encoded, err := runWhiteboardCoverageOutput(t, Update, caller, "", args...)
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(encoded, &envelope); err != nil {
				t.Fatalf("decode emitted result: %v: %s", err, encoded)
			}
			data := envelope.Data
			receipt, ok := data["receipt"].(map[string]any)
			index := 0
			if tc.dry {
				index = 1
			}
			if !ok || len(receipt) != len(branchRequired[index]) {
				t.Fatalf("runtime receipt does not match branch %d: %#v", index, receipt)
			}
			for _, key := range branchRequired[index] {
				if value, exists := receipt[key.(string)]; !exists || value == nil {
					t.Errorf("receipt field %s missing or null: %#v", key, receipt)
				}
			}
			_, hasSource := data["source"]
			if data["verified"] != !tc.dry || hasSource != tc.dry {
				t.Fatalf("runtime verified/source conflicts with receipt: %#v", data)
			}
			if tc.dry {
				if receipt["dryRun"] != true || receipt["executed"] != false || len(caller.calls) != 0 {
					t.Fatalf("dry-run executed or lost its markers: %#v calls=%#v", receipt, caller.calls)
				}
			} else if created, ok := receipt["createdNodeIds"].([]any); !ok || len(created) != len(tc.realIDs) || len(caller.calls) != 2 {
				t.Fatalf("success must emit a real array (including clear) after write/readback: %#v", receipt)
			}
		})
	}
}

func TestCrossPlatformCoverageWhiteboardExecutorErrorsAndExactCallOrder(t *testing.T) {
	invalidQuery := directWhiteboardRuntime(t, Query, &whiteboardCoverageCaller{responses: map[string][]string{}}, "--node", "")
	if err := Query.Execute(invalidQuery); err == nil {
		t.Fatal("direct query accepted an invalid route")
	}
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
	invalidUpdate := directWhiteboardRuntime(t, Update, &whiteboardCoverageCaller{responses: map[string][]string{}},
		"--node", "wb", "--source", appendSource)
	if err := Update.Execute(invalidUpdate); err == nil {
		t.Fatal("direct update accepted an invalid standalone route")
	}
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

	standaloneArgs := []string{"--node", "wb", "--expected-revision", "12", "--request-id", "req-1", "--source", appendSource, "--yes"}
	standaloneReceipt := validStandaloneWhiteboardUpdateResponse("wb", "append", 12, 13, []string{"n1"}, []string{"real-1"}, 0)
	for _, tc := range []struct {
		name      string
		responses map[string][]string
	}{
		{name: "read call", responses: map[string][]string{toolUpdateStandalone: {standaloneReceipt}}},
		{name: "read projection", responses: map[string][]string{
			toolUpdateStandalone: {standaloneReceipt}, toolQueryStandalone: {`{"success":true}`},
		}},
		{name: "revision mismatch", responses: map[string][]string{
			toolUpdateStandalone: {standaloneReceipt}, toolQueryStandalone: {validStandaloneWhiteboardQueryResponse("wb", "page", 14, `[{"id":"real-1","type":"text"}]`)},
		}},
		{name: "content mismatch", responses: map[string][]string{
			toolUpdateStandalone: {standaloneReceipt}, toolQueryStandalone: {validStandaloneWhiteboardQueryResponse("wb", "page", 13, `[{"id":"real-1","type":"shape"}]`)},
		}},
	} {
		t.Run("standalone "+tc.name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{responses: tc.responses}
			if err := runWhiteboardCoverage(t, Update, caller, "", standaloneArgs...); err == nil || len(caller.calls) != 2 {
				t.Fatalf("error=%v calls=%#v", err, caller.calls)
			}
		})
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

func TestCrossPlatformCoverageWhiteboardStandaloneQueryEnvelopeMatrix(t *testing.T) {
	request := map[string]any{"nodeId": "wb", "view": "all"}
	validSource := map[string]any{
		"schemaVersion": "1.0", "catalogVersion": "dml-v1",
		"pages": []any{map[string]any{"id": "page", "nodes": []any{map[string]any{"id": "node", "type": "text"}}}},
	}
	validSummary := map[string]any{
		"nodeCount": 1, "pageCount": 1, "readOnlyNodeCount": 0,
		"unknownNodeCount": 0, "resultBytes": 1, "resultSha256": "hash",
	}
	base := func() map[string]any {
		return map[string]any{
			"success": true, "nodeId": "wb", "revision": 2, "view": "all",
			"resultJson": validSource, "resultSummary": validSummary,
		}
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"business failure", func(v map[string]any) { v["success"] = false }},
		{"target mismatch", func(v map[string]any) { v["nodeId"] = "other" }},
		{"invalid revision", func(v map[string]any) { v["revision"] = -1 }},
		{"view mismatch", func(v map[string]any) { v["view"] = "page" }},
		{"malformed summary", func(v map[string]any) { v["resultSummary"] = map[string]any{} }},
		{"invalid result JSON", func(v map[string]any) { v["resultJson"] = "{" }},
		{"wrong schema", func(v map[string]any) {
			v["resultJson"] = map[string]any{"schemaVersion": "2.0", "catalogVersion": "dml-v1", "pages": []any{}}
		}},
		{"wrong catalog", func(v map[string]any) {
			v["resultJson"] = map[string]any{"schemaVersion": "1.0", "catalogVersion": "v2", "pages": []any{}}
		}},
		{"missing pages", func(v map[string]any) {
			v["resultJson"] = map[string]any{"schemaVersion": "1.0", "catalogVersion": "dml-v1"}
		}},
		{"invalid pages", func(v map[string]any) {
			v["resultJson"] = map[string]any{"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": "bad"}
		}},
		{"summary mismatch", func(v map[string]any) { v["resultSummary"] = map[string]any{"nodeCount": 2} }},
		{"malformed download URL", func(v map[string]any) { delete(v, "resultJson"); v["resultDownloadUrl"] = " " }},
		{"conflicting payload", func(v map[string]any) { v["resultDownloadUrl"] = "https://example.test/result" }},
		{"missing payload", func(v map[string]any) { delete(v, "resultJson") }},
		{"malformed message", func(v map[string]any) { v["message"] = 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base()
			test.mutate(value)
			if result, err := projectStandaloneWhiteboardQuery(value, request); err == nil {
				t.Fatalf("unexpected success: %#v", result)
			}
		})
	}

	download := base()
	delete(download, "resultJson")
	download["resultDownloadUrl"] = "https://example.test/result"
	download["message"] = "ready"
	result, err := projectStandaloneWhiteboardQuery(download, request)
	if err != nil || result["resultDownloadUrl"] == nil || result["message"] != "ready" {
		t.Fatalf("download projection=%#v err=%v", result, err)
	}
	summary := base()
	summary["view"] = "summary"
	delete(summary, "resultJson")
	result, err = projectStandaloneWhiteboardQuery(summary, map[string]any{"nodeId": "wb", "view": "summary"})
	if err != nil || result["source"] != nil {
		t.Fatalf("summary projection=%#v err=%v", result, err)
	}
}

func TestCrossPlatformCoverageWhiteboardStandaloneReceiptValidationMatrix(t *testing.T) {
	expected, err := parseWhiteboardSource(`{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"text"}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"nodeId": "wb", "mode": "append", "expectedRevision": 2, "requestId": "req"}
	base := func() map[string]any {
		return map[string]any{
			"success": true, "nodeId": "wb", "mode": "append", "pageId": "page",
			"previousRevision": 2, "committedRevision": 3,
			"createdNodeIds": []any{"real"}, "idMap": map[string]any{"n1": "real"},
			"deletedNodeCount": 0, "idempotentReplay": false, "message": "done",
		}
	}
	tests := []struct {
		name   string
		mutate func(map[string]any, map[string]any)
	}{
		{"business failure", func(v, _ map[string]any) { v["success"] = false }},
		{"target mismatch", func(v, _ map[string]any) { v["nodeId"] = "other" }},
		{"mode mismatch", func(v, _ map[string]any) { v["mode"] = "overwrite" }},
		{"missing page", func(v, _ map[string]any) { delete(v, "pageId") }},
		{"page mismatch", func(v, r map[string]any) { r["pageId"] = "other" }},
		{"invalid request state", func(_ map[string]any, r map[string]any) { r["requestId"] = " " }},
		{"revision mismatch", func(v, _ map[string]any) { v["previousRevision"] = 1 }},
		{"committed revision", func(v, _ map[string]any) { v["committedRevision"] = 1 }},
		{"malformed created IDs", func(v, _ map[string]any) { v["createdNodeIds"] = []any{" "} }},
		{"created count", func(v, _ map[string]any) { v["createdNodeIds"] = []any{} }},
		{"malformed id map", func(v, _ map[string]any) { v["idMap"] = []any{} }},
		{"empty id map entry", func(v, _ map[string]any) { v["idMap"] = map[string]any{" ": "real"} }},
		{"id map count", func(v, _ map[string]any) { v["idMap"] = map[string]any{} }},
		{"id map order", func(v, _ map[string]any) { v["idMap"] = map[string]any{"n1": "other"} }},
		{"deleted count", func(v, _ map[string]any) { v["deletedNodeCount"] = 1 }},
		{"replay type", func(v, _ map[string]any) { v["idempotentReplay"] = "false" }},
		{"missing message", func(v, _ map[string]any) { v["message"] = " " }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base()
			requestCopy := maps.Clone(request)
			test.mutate(value, requestCopy)
			if receipt, err := requireStandaloneWhiteboardUpdateReceipt(value, requestCopy, expected); err == nil {
				t.Fatalf("unexpected receipt: %#v", receipt)
			}
		})
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
		{"coordinate normalization within tolerance", json.Number("40"), json.Number("40.5"), false},
		{"coordinate normalization over tolerance", json.Number("40"), json.Number("40.5001"), true},
		{"non coordinate stays exact", json.Number("40"), json.Number("40.5"), true},
		{"large adjacent JSON integers mismatch", json.Number("9007199254740992"), json.Number("9007199254740993"), true},
		{"large JSON integer equal", json.Number("9007199254740993"), json.Number("9007199254740993"), false},
		{"scalar mismatch", "one", "two", true},
		{"scalar equal", "same", "same", false},
	}
	for _, test := range comparisons {
		path := test.name
		if test.name == "coordinate normalization within tolerance" || test.name == "coordinate normalization over tolerance" {
			path = "node fixture.x"
		}
		err := requireRequestedValue(test.expected, test.actual, path)
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
