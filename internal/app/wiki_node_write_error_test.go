// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/wiki"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageWikiNodeWriteFailureEnvelopeKeepsReceipt(t *testing.T) {
	caller := &paramAliasCaptureCaller{}
	_, err := executeParamAliasE2E(t, caller, "wiki", "+node-copy", "--workspace", "wrong-target", "--node", "node-1", "--yes")
	if err == nil || len(caller.calls) != 3 {
		t.Fatalf("wrong-target copy err=%v calls=%#v", err, caller.calls)
	}
	cmd := &cobra.Command{Use: "failure"}
	cmd.Flags().String("format", "json", "")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	code, emitErr := output.EmitResult(cmd, output.FailureWithExitCode(errorInfoFromExecutionError(err), apperrors.ExitCode(err)))
	if emitErr != nil || code == 0 || stderr.Len() != 0 {
		t.Fatalf("emit code=%d err=%v stderr=%q", code, emitErr, stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	info, _ := envelope["error"].(map[string]any)
	details, _ := info["details"].(map[string]any)
	receipt, _ := details["writeReceipt"].(map[string]any)
	if envelope["ok"] != false || envelope["outcome"] != "failure" || info["execution_started"] != true || receipt["returnedNodeId"] != "copy-1" || receipt["sourceNodeId"] != "node-1" || receipt["expectedWorkspaceId"] != "wrong-target" || receipt["newResourceConfirmed"] != false {
		t.Fatalf("unified failure lost write receipt: %s", stdout.String())
	}
}

type wikiLegacyReadbackCaller struct {
	mcpURLTestCaller
	writes int
}

func (c *wikiLegacyReadbackCaller) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	if product == "doc" && tool == "get_document_info" && c.writes == 0 {
		return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"nodeId":"source","workspaceId":"source-w","name":"Source","extension":"adoc"}`}}}, nil
	}
	if product == "doc" && tool == "copy_document" {
		c.writes++
		return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":true,"nodeId":"copy-legacy"}`}}}, nil
	}
	return c.mcpURLTestCaller.CallTool(ctx, product, tool, args)
}

func TestCrossPlatformCoverageWikiLegacyReadbackHintDoesNotReplayWrite(t *testing.T) {
	const unsafeHint = "刷新后请重试整个复制命令"
	cause := &helpers.CLIError{Code: helpers.CodeAuthTokenExpired, Message: "readback expired", Operation: "doc/get_document_info", Suggestion: unsafeHint}
	caller := &wikiLegacyReadbackCaller{mcpURLTestCaller: mcpURLTestCaller{err: cause}}
	helpers.InitDepsForTest(t, caller)
	cmd := &cobra.Command{Use: "copy"}
	cmd.Flags().String("workspace", "w", "")
	cmd.Flags().String("node", "source", "")
	cmd.Flags().String("folder", "", "")
	cmd.Flags().String("format", "json", "")
	err := wiki.NodeCopy.Execute(shortcut.RuntimeContextForTest(cmd, wiki.NodeCopy))
	if err == nil || caller.writes != 1 || caller.toolName != "get_document_info" {
		t.Fatalf("copy/readback calls changed: err=%v caller=%#v", err, caller)
	}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	code, emitErr := output.EmitResult(cmd, output.FailureWithExitCode(errorInfoFromExecutionError(err), apperrors.ExitCode(err)))
	if emitErr != nil || code != helpers.ExitAuth || stderr.Len() != 0 {
		t.Fatalf("failure emit code=%d err=%v stderr=%q", code, emitErr, stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	info, _ := envelope["error"].(map[string]any)
	if strings.Contains(stdout.String(), unsafeHint) || !strings.Contains(schemaContractString(info["hint"]), "禁止直接重试") || !strings.Contains(schemaContractString(info["message"]), `"returnedNodeId":"copy-legacy"`) {
		t.Errorf("legacy hint encourages replay or loses receipt: %s", stdout.String())
	}
	var legacy *helpers.CLIError
	if !errors.As(err, &legacy) || legacy == cause || legacy.Code != cause.Code || legacy.Operation != cause.Operation || !errors.Is(err, cause) || cause.Suggestion != unsafeHint {
		t.Errorf("legacy identity or original cause changed: err=%#v cause=%#v", err, cause)
	}
}
