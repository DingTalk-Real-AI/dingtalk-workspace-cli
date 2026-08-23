// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type contractDefectCaller struct {
	dryRun    bool
	calls     []guardedMutationCall
	readCalls []guardedMutationCall
	responses map[string]string
	errors    map[string]error
}

func (c *contractDefectCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, guardedMutationCall{productID: productID, toolName: toolName, args: args})
	key := productID + "/" + toolName
	if err := c.errors[key]; err != nil {
		return nil, err
	}
	text := c.responses[key]
	if text == "" {
		text = `{}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (c *contractDefectCaller) CallReadTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.readCalls = append(c.readCalls, guardedMutationCall{productID: productID, toolName: toolName, args: args})
	key := productID + "/" + toolName
	if err := c.errors[key]; err != nil {
		return nil, err
	}
	text := c.responses[key]
	if text == "" {
		text = `{}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*contractDefectCaller) Format() string { return "json" }
func (c *contractDefectCaller) DryRun() bool { return c.dryRun }
func (*contractDefectCaller) Fields() string { return "" }
func (*contractDefectCaller) JQ() string     { return "" }

func executeContractDefectCommand(t *testing.T, caller *contractDefectCaller, build func() *cobra.Command, args ...string) (string, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	InitDeps(caller)
	var stdout bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = io.Discard

	root := build()
	if root.PersistentFlags().Lookup("yes") == nil {
		root.PersistentFlags().Bool("yes", false, "confirm high-risk operation")
	}
	if root.PersistentFlags().Lookup("dry-run") == nil {
		root.PersistentFlags().Bool("dry-run", false, "preview without executing")
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), err
}

func TestApprovalRevokeDryRunSkipsConfirmationAndEmitsPreview(t *testing.T) {
	caller := &contractDefectCaller{dryRun: true}
	output, err := executeContractDefectCommand(t, caller, newOaCommand,
		"approval", "revoke", "--instance-id", "instance-dry-run", "--dry-run")
	if err != nil {
		t.Fatalf("approval revoke dry-run returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run tool calls = %#v, want none", caller.calls)
	}
	if !strings.Contains(output, `"tool": "revoke_processInstance"`) ||
		!strings.Contains(output, `"processInstanceId": "instance-dry-run"`) {
		t.Fatalf("dry-run output = %q, want revoke preview", output)
	}
}

func TestDocVersionRevertDryRunSkipsRemotePreflightAndEmitsPreview(t *testing.T) {
	caller := &contractDefectCaller{dryRun: true}
	output, err := executeContractDefectCommand(t, caller, newDocCommand,
		"version", "revert", "--node", "node-dry-run", "--version", "7", "--dry-run")
	if err != nil {
		t.Fatalf("doc version revert dry-run returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run mutation calls = %#v, want none", caller.calls)
	}
	if len(caller.readCalls) != 0 {
		t.Fatalf("dry-run read calls = %#v, want none (no remote version preflight)", caller.readCalls)
	}
	if !strings.Contains(output, `"tool": "revert_doc_version"`) ||
		!strings.Contains(output, `"version": 7`) {
		t.Fatalf("dry-run output = %q, want version revert preview", output)
	}
}

func TestDocVersionRevertDryRunDoesNotRejectMissingVersionRemotely(t *testing.T) {
	caller := &contractDefectCaller{dryRun: true}
	output, err := executeContractDefectCommand(t, caller, newDocCommand,
		"version", "revert", "--node", "node-dry-run", "--version", "999", "--dry-run")
	if err != nil {
		t.Fatalf("doc version revert dry-run returned error: %v", err)
	}
	if len(caller.readCalls) != 0 {
		t.Fatalf("dry-run read calls = %#v, want none (no remote version preflight)", caller.readCalls)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run mutation calls = %#v, want none", caller.calls)
	}
	if !strings.Contains(output, `"tool": "revert_doc_version"`) ||
		!strings.Contains(output, `"version": 999`) {
		t.Fatalf("dry-run output = %q, want preview without remote missing-version rejection", output)
	}
}

func TestDocVersionRevertNonDryRunPreflightsVersion(t *testing.T) {
	caller := &contractDefectCaller{
		responses: map[string]string{
			"doc/list_doc_versions":  `{"versions":[{"version":7}]}`,
			"doc/revert_doc_version": `{}`,
		},
	}
	output, err := executeContractDefectCommand(t, caller, newDocCommand,
		"version", "revert", "--node", "node-live", "--version", "7", "--yes")
	if err != nil {
		t.Fatalf("doc version revert returned error: %v", err)
	}
	// Outside --dry-run, list_doc_versions rides the normal CallTool channel
	// (CallReadTool is dry-run-only). Expect preflight then mutation.
	if len(caller.calls) != 2 ||
		caller.calls[0].productID != "doc" || caller.calls[0].toolName != "list_doc_versions" ||
		caller.calls[1].productID != "doc" || caller.calls[1].toolName != "revert_doc_version" {
		t.Fatalf("non-dry-run calls = %#v, want list_doc_versions then revert_doc_version", caller.calls)
	}
	if strings.Contains(output, `"dry_run": true`) {
		t.Fatalf("non-dry-run output unexpectedly previewed: %q", output)
	}
}

func TestDocVersionRevertPublishesRuntimeSafety(t *testing.T) {
	cmd, remaining, err := newDocCommand().Find([]string{"version", "revert"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("find doc version revert: command=%v remaining=%v err=%v", cmd, remaining, err)
	}
	final, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok || final.Safety == nil {
		t.Fatal("doc version revert must publish ContractFinal Safety")
	}
	if safety := *final.Safety; safety.Effect != "write" || safety.Risk != "medium" ||
		safety.Confirmation != "user_required" || safety.Idempotency != "unknown" {
		t.Fatalf("doc version revert Safety = %#v, want write/medium/user_required/unknown", safety)
	}
}

func TestDriveDeleteDryRunSkipsConfirmationAndEOFIsObservable(t *testing.T) {
	caller := &contractDefectCaller{dryRun: true}
	output, err := executeContractDefectCommand(t, caller, newDriveCommand,
		"delete", "--node", "node-dry-run", "--dry-run")
	if err != nil {
		t.Fatalf("drive delete dry-run returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run mutation calls = %#v, want none", caller.calls)
	}
	if !strings.Contains(output, `"tool": "delete_document"`) ||
		!strings.Contains(output, `"nodeId": "node-dry-run"`) {
		t.Fatalf("dry-run output = %q, want delete preview", output)
	}

	caller = &contractDefectCaller{}
	_, err = executeContractDefectCommand(t, caller, newDriveCommand,
		"delete", "--node", "node-eof")
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "confirmation_required" {
		t.Fatalf("closed-stdin error = %#v, want typed confirmation_required", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("closed-stdin mutation calls = %#v, want none", caller.calls)
	}
}

func TestDocRenamePreservesCallerProvidedDisplayName(t *testing.T) {
	renameCmd, remaining, err := newDocCommand().Find([]string{"rename"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("find doc rename: command=%v remaining=%v err=%v", renameCmd, remaining, err)
	}
	nameFlag := renameCmd.Flags().Lookup("name")
	if nameFlag == nil {
		t.Fatal("doc rename --name flag is missing")
	}
	if !strings.Contains(nameFlag.Usage, "原样传给服务端") ||
		!strings.Contains(nameFlag.Usage, "drive rename") ||
		strings.Contains(nameFlag.Usage, "自动去掉") {
		t.Fatalf("doc rename --name usage = %q, want verbatim forwarding and drive rename routing", nameFlag.Usage)
	}

	caller := &contractDefectCaller{}
	if _, err = executeContractDefectCommand(t, caller, newDocCommand,
		"rename", "--node", "node-1", "--name", "release.v2"); err != nil {
		t.Fatalf("doc rename returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool calls = %#v, want one rename call", caller.calls)
	}
	if got := caller.calls[0].args["newName"]; got != "release.v2" {
		t.Fatalf("newName = %#v, want release.v2", got)
	}
}

func TestDriveRenameUsesNodeTypeAndCurrentExtension(t *testing.T) {
	tests := []struct {
		name        string
		metadata    string
		nodeID      string
		wantFileID  string
		requested   string
		wantNewName string
	}{
		{
			name:        "file strips matching extension outside old whitelist",
			metadata:    `{"result":{"type":"file","extension":"heic"}}`,
			nodeID:      "https://alidocs.dingtalk.com/i/nodes/node-1",
			wantFileID:  "node-1",
			requested:   "photo.HEIC",
			wantNewName: "photo",
		},
		{
			name:        "folder preserves dotted display name",
			metadata:    `{"result":{"type":"folder","extension":"v2"}}`,
			requested:   "release.v2",
			wantNewName: "release.v2",
		},
		{
			name:        "different current extension is preserved",
			metadata:    `{"result":{"nodeType":"file","fileExtension":"txt"}}`,
			requested:   "report.final",
			wantNewName: "report.final",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &contractDefectCaller{responses: map[string]string{
				"drive/get_file_info": test.metadata,
			}}
			nodeID := test.nodeID
			if nodeID == "" {
				nodeID = "node-1"
			}
			if _, err := executeContractDefectCommand(t, caller, newDriveCommand,
				"rename", "--node", nodeID, "--name", test.requested); err != nil {
				t.Fatalf("drive rename returned error: %v", err)
			}
			if len(caller.calls) != 2 {
				t.Fatalf("tool calls = %#v, want metadata read plus rename", caller.calls)
			}
			if caller.calls[0].productID != "drive" || caller.calls[0].toolName != "get_file_info" {
				t.Fatalf("first call = %#v, want drive/get_file_info", caller.calls[0])
			}
			wantFileID := test.wantFileID
			if wantFileID == "" {
				wantFileID = nodeID
			}
			if got := caller.calls[0].args["fileId"]; got != wantFileID {
				t.Fatalf("metadata fileId = %#v, want %q", got, wantFileID)
			}
			if caller.calls[1].productID != "doc" || caller.calls[1].toolName != "rename_document" {
				t.Fatalf("second call = %#v, want doc/rename_document", caller.calls[1])
			}
			if got := caller.calls[1].args["newName"]; got != test.wantNewName {
				t.Fatalf("newName = %#v, want %q", got, test.wantNewName)
			}
		})
	}
}

func TestDriveRenameMetadataFailuresDoNotMutate(t *testing.T) {
	tests := []struct {
		name      string
		responses map[string]string
		errors    map[string]error
	}{
		{
			name:   "metadata call error",
			errors: map[string]error{"drive/get_file_info": errors.New("metadata unavailable")},
		},
		{
			name:      "invalid metadata JSON",
			responses: map[string]string{"drive/get_file_info": `{`},
		},
		{
			name:      "metadata result missing",
			responses: map[string]string{"drive/get_file_info": `{}`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &contractDefectCaller{responses: test.responses, errors: test.errors}
			_, err := executeContractDefectCommand(t, caller, newDriveCommand,
				"rename", "--node", "node-1", "--name", "report.txt")
			if err == nil {
				t.Fatal("drive rename returned nil error")
			}
			if len(caller.calls) != 1 || caller.calls[0].toolName != "get_file_info" {
				t.Fatalf("tool calls = %#v, want metadata read only", caller.calls)
			}
		})
	}
}

func TestDriveRenameDryRunDefersMetadataNormalization(t *testing.T) {
	caller := &contractDefectCaller{dryRun: true}
	output, err := executeContractDefectCommand(t, caller, newDriveCommand,
		"rename", "--node", "node-1", "--name", "photo.heic", "--dry-run")
	if err != nil {
		t.Fatalf("drive rename dry-run returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run tool calls = %#v, want none", caller.calls)
	}
	if !strings.Contains(output, `"newName": "photo.heic"`) {
		t.Fatalf("dry-run output = %q, want unmodified input name", output)
	}
}

func TestDriveRenameBaseNameConservativeEdges(t *testing.T) {
	tests := []struct {
		name      string
		nodeType  string
		extension string
		want      string
	}{
		{name: ".txt", nodeType: "file", extension: "txt", want: ".txt"},
		{name: "report.txt", nodeType: "directory", extension: "txt", want: "report.txt"},
		{name: "report.txt", nodeType: "dir", extension: "txt", want: "report.txt"},
		{name: " report.txt ", nodeType: "file", extension: "", want: "report.txt"},
		{name: "report.txt", nodeType: "file", extension: ".txt", want: "report"},
	}
	for _, test := range tests {
		if got := driveRenameBaseName(test.name, test.nodeType, test.extension); got != test.want {
			t.Fatalf("driveRenameBaseName(%q, %q, %q) = %q, want %q",
				test.name, test.nodeType, test.extension, got, test.want)
		}
	}
}

func TestDriveInfoRestoresFileSizeWhenDocMetadataIsNull(t *testing.T) {
	caller := &contractDefectCaller{responses: map[string]string{
		"drive/get_file_info":   `{"result":{"fileId":"node-1","extension":"adoc","fileSize":93682}}`,
		"doc/get_document_info": `{"result":{"title":"Doc","fileSize":null}}`,
	}}
	output, err := executeContractDefectCommand(t, caller, newDriveCommand,
		"info", "--node", "node-1")
	if err != nil {
		t.Fatalf("drive info returned error: %v", err)
	}
	var payload struct {
		Result struct {
			FileSize int64 `json:"fileSize"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode drive info output %q: %v", output, err)
	}
	if payload.Result.FileSize != 93682 {
		t.Fatalf("fileSize = %d, want 93682", payload.Result.FileSize)
	}
}
