// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/whiteboard"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type whiteboardReconciliationCaller struct{ calls []string }

func (c *whiteboardReconciliationCaller) CallTool(_ context.Context, server, tool string, _ map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, server+"/"+tool)
	var response string
	switch tool {
	case "update_whiteboard":
		response = `{"success":true,"nodeId":"doc","partId":"part","resultJson":{"mode":"append","message":"completed","createdNodeIds":["real-1"],"idMap":{"n1":"real-1"},"deletedNodeCount":0}}`
	case "read_whiteboard_content":
		response = `{"success":true,"resultJson":{"schemaVersion":"1.0","catalogVersion":"dml-v1","pages":[{"id":"page-1","nodes":[{"id":"real-1","type":"group","x":41}]}]},"resultSummary":{"nodeCount":1,"pageCount":1,"readOnlyNodeCount":0,"unknownNodeCount":0,"resultBytes":1,"resultSha256":"digest"}}`
	default:
		return nil, fmt.Errorf("unexpected tool %s", tool)
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: response}}}, nil
}

func (*whiteboardReconciliationCaller) Format() string { return "json" }
func (*whiteboardReconciliationCaller) DryRun() bool   { return false }
func (*whiteboardReconciliationCaller) Fields() string { return "" }
func (*whiteboardReconciliationCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageWhiteboardCommittedFailureSurvivesUnifiedOutput(t *testing.T) {
	caller := &whiteboardReconciliationCaller{}
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	group := &cobra.Command{Use: "whiteboard"}
	leaf := corecmd.New(shortcut.FromShortcut(whiteboard.Update))
	group.AddCommand(leaf)
	root.AddCommand(group)
	root.SetArgs([]string{"whiteboard", "+update", "--node", "doc", "--part-id", "part", "--yes", "--source",
		`{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"group","x":40}]}}`})
	err := root.Execute()
	if err == nil {
		t.Fatal("coordinate drift must not report complete success")
	}
	code, emitErr := output.EmitResult(leaf, output.Failure(errorInfoFromExecutionError(err)))
	if emitErr != nil || code != 1 {
		t.Fatalf("emit: code=%d err=%v", code, emitErr)
	}
	var envelope output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Outcome != output.OutcomeFailure || envelope.Data != nil || envelope.Error == nil {
		t.Fatalf("incorrect failure envelope: %s", stdout.String())
	}
	failure := envelope.Error
	if failure.Subtype != "readback_field_mismatch" || failure.Retryable || failure.ExecutionStarted == nil || !*failure.ExecutionStarted || !strings.Contains(failure.Hint, "停止重提并只读对账") {
		t.Fatalf("lost no-replay contract: %s", stdout.String())
	}
	wantDetails := map[string]any{
		"nodeId": "doc", "partId": "part", "mode": "append", "commitState": "committed", "verified": false,
		"receipt": map[string]any{"message": "completed", "createdNodeIds": []any{"real-1"},
			"idMap": map[string]any{"n1": "real-1"}, "deletedNodeCount": float64(0)},
	}
	if !reflect.DeepEqual(failure.Details, wantDetails) {
		t.Fatalf("wire lost receipt or emitted a snapshot: %s", stdout.String())
	}
	if !reflect.DeepEqual(caller.calls, []string{"whiteboard/update_whiteboard", "whiteboard/read_whiteboard_content"}) {
		t.Fatalf("unexpected replay or discovery: %v", caller.calls)
	}
}
