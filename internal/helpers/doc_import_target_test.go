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
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func fastDocImportConfig() importFlowConfig {
	cfg := docImportFlowConfig()
	cfg.poll.maxPolls = 2
	cfg.poll.interval = func(int) time.Duration { return 0 }
	cfg.poll.wait = func(context.Context, time.Duration) error { return nil }
	return cfg
}

func executeDocImportCommand(t *testing.T, caller *sheetImportCaller, cfg importFlowConfig, args ...string) (string, error) {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
		SetHTTPPutFile(nil)
	})

	InitDeps(caller)
	var output bytes.Buffer
	deps.Out.w = &output
	deps.Out.errW = &output
	os.Args = []string{"dws", "doc"}
	SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })

	command := &cobra.Command{
		Use:          "import",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, positional []string) error {
			return runImportCommand(cmd, positional, cfg)
		},
	}
	command.Flags().String("file", "", "")
	command.Flags().String("folder", "", "")
	command.Flags().String("folder-id", "", "")
	command.Flags().String("workspace", "", "")
	command.Flags().String("workspace-id", "", "")
	command.Flags().String("name", "", "")
	command.SetArgs(args)
	err := command.Execute()
	return output.String(), err
}

func TestDocImportDefaultTargetIsResolvedAndVerified(t *testing.T) {
	filePath := writeImportFixture(t, "md")
	caller := &sheetImportCaller{responses: map[string][]string{
		"list_wikiSpaces":       {`{"success":true,"result":{"wikiSpaces":[{"workspaceId":"my-space","name":"我的文档"}]}}`},
		"create_import_session": {`{"sessionId":"session-1","uploadUrl":"https://upload.test/file"}`},
		"confirm_import":        {`{"taskId":"task-1"}`},
		"query_import_task":     {`{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-1","documentName":"sales","documentType":"ALIDOC"}`},
		"get_document_info":     {`{"result":{"nodeId":"node-1","workspaceId":"my-space","folderId":"root-folder","name":"sales","contentType":"ALIDOC"}}`},
	}}

	output, err := executeDocImportCommand(t, caller, fastDocImportConfig(), "--file", filePath)
	if err != nil {
		t.Fatalf("doc import: %v\n%s", err, output)
	}
	if len(caller.calls) != 5 {
		t.Fatalf("calls = %#v, want 5", caller.calls)
	}
	wantTools := []string{"list_wikiSpaces", "create_import_session", "confirm_import", "query_import_task", "get_document_info"}
	for index, want := range wantTools {
		if got := caller.calls[index].tool; got != want {
			t.Fatalf("call[%d] tool = %q, want %q", index, got, want)
		}
	}
	if got := caller.calls[1].args["workspaceId"]; got != "my-space" {
		t.Fatalf("create_import_session workspaceId = %#v", got)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output)
	}
	if result["nodeId"] != "node-1" || result["verified"] != true {
		t.Fatalf("result = %#v", result)
	}
	target, _ := result["target"].(map[string]any)
	if target["source"] != "default_personal_workspace" || target["workspaceId"] != "my-space" {
		t.Fatalf("target = %#v", target)
	}
}

func TestDocImportExplicitFolderSkipsDefaultResolution(t *testing.T) {
	filePath := writeImportFixture(t, "md")
	caller := &sheetImportCaller{responses: map[string][]string{
		"create_import_session": {`{"sessionId":"session-1","uploadUrl":"https://upload.test/file"}`},
		"confirm_import":        {`{"taskId":"task-1"}`},
		"query_import_task":     {`{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-1"}`},
		"get_document_info":     {`{"nodeId":"node-1","folderId":"folder-1","workspaceId":"space-1"}`},
	}}

	output, err := executeDocImportCommand(t, caller, fastDocImportConfig(), "--file", filePath, "--folder", "folder-1")
	if err != nil {
		t.Fatalf("doc import: %v\n%s", err, output)
	}
	for _, call := range caller.calls {
		if call.tool == "list_wikiSpaces" {
			t.Fatalf("explicit target unexpectedly resolved default: %#v", caller.calls)
		}
	}
	if got := caller.calls[0].args["targetFolderId"]; got != "folder-1" {
		t.Fatalf("targetFolderId = %#v", got)
	}
}

func TestDocImportDefaultResolutionFailureIsNotStarted(t *testing.T) {
	filePath := writeImportFixture(t, "md")
	caller := &sheetImportCaller{responses: map[string][]string{
		"list_wikiSpaces": {`{"result":{"wikiSpaces":[]}}`},
	}}

	_, err := executeDocImportCommand(t, caller, fastDocImportConfig(), "--file", filePath)
	if err == nil {
		t.Fatal("expected resolution failure")
	}
	var structured *apperrors.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if structured.Reason != "doc_import_default_target_unavailable" || structured.FailureStage != "resolve_default_target" {
		t.Fatalf("structured error = %#v", structured)
	}
	if structured.ExecutionStarted == nil || *structured.ExecutionStarted {
		t.Fatalf("ExecutionStarted = %#v, want false", structured.ExecutionStarted)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "list_wikiSpaces" {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

func TestDocImportPlacementMismatchIsPartialSuccess(t *testing.T) {
	filePath := writeImportFixture(t, "md")
	caller := &sheetImportCaller{responses: map[string][]string{
		"create_import_session": {`{"sessionId":"session-1","uploadUrl":"https://upload.test/file"}`},
		"confirm_import":        {`{"taskId":"task-1"}`},
		"query_import_task":     {`{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-1"}`},
		"get_document_info":     {`{"nodeId":"node-1","folderId":"wrong-folder"}`},
	}}

	_, err := executeDocImportCommand(t, caller, fastDocImportConfig(), "--file", filePath, "--folder", "folder-1")
	if err == nil {
		t.Fatal("expected placement verification failure")
	}
	var structured *apperrors.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if structured.Reason != "doc_import_placement_unverified" || structured.Details["status"] != "partial_success" {
		t.Fatalf("structured error = %#v", structured)
	}
	if structured.ExecutionStarted == nil || !*structured.ExecutionStarted {
		t.Fatalf("ExecutionStarted = %#v, want true", structured.ExecutionStarted)
	}
}

func TestParsePersonalDocWorkspaceIDRejectsAmbiguousResponse(t *testing.T) {
	for _, text := range []string{
		`{`,
		`{"wikiSpaces":[]}`,
		`{"wikiSpaces":"not-a-list"}`,
		`{"wikiSpaces":[1]}`,
		`{"wikiSpaces":[{"workspaceId":"a"},{"workspaceId":"b"}]}`,
		`{"wikiSpaces":[{"name":"我的文档"}]}`,
	} {
		if _, err := parsePersonalDocWorkspaceID(text); err == nil {
			t.Fatalf("parsePersonalDocWorkspaceID(%s) unexpectedly succeeded", text)
		}
	}
}

func TestCrossPlatformCoverageDocImportTargetDefensiveBranches(t *testing.T) {
	if err := resolveDefaultDocImportTarget(context.Background(), nil); err != nil {
		t.Fatalf("nil import target: %v", err)
	}
	for _, file := range []*preparedImportFile{{folder: "folder-1"}, {workspace: "space-1"}} {
		if err := resolveDefaultDocImportTarget(context.Background(), file); err != nil {
			t.Fatalf("explicit import target: %v", err)
		}
	}

	for _, text := range []string{`{`, `{}`, `[]`} {
		if _, err := parseImportedDocumentInfo(text); err == nil {
			t.Fatalf("parseImportedDocumentInfo(%q) unexpectedly succeeded", text)
		}
	}
	if info, err := parseImportedDocumentInfo(`{"data":{"document":{"nodeId":"node-1"}}}`); err != nil || info["nodeId"] != "node-1" {
		t.Fatalf("nested document info = %#v, %v", info, err)
	}

	for _, test := range []struct {
		raw  string
		want string
	}{
		{"https://alidocs.dingtalk.com/i/nodes/n?workspaceId=space-query", "space-query"},
		{"https://alidocs.dingtalk.com/i/nodes/node-path", "node-path"},
		{"https://alidocs.dingtalk.com/i/spaces/space-path", "space-path"},
		{"https://alidocs.dingtalk.com/i/folders/folder-path", "folder-path"},
		{"https://alidocs.dingtalk.com/unknown/path", "https://alidocs.dingtalk.com/unknown/path"},
		{"plain-id", "plain-id"},
	} {
		if got := canonicalImportTargetID(test.raw); got != test.want {
			t.Fatalf("canonicalImportTargetID(%q) = %q, want %q", test.raw, got, test.want)
		}
	}

	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	verifyFailure := func(t *testing.T, file preparedImportFile, response, documentURL string) {
		t.Helper()
		responses := map[string][]string{}
		if response != "" {
			responses["get_document_info"] = []string{response}
		}
		InitDeps(&sheetImportCaller{responses: responses})
		if _, _, err := verifyImportedDocumentPlacement(context.Background(), file, "task-1", documentURL); err == nil {
			t.Fatal("placement verification unexpectedly succeeded")
		}
	}
	verifyFailure(t, preparedImportFile{}, "", "")
	verifyFailure(t, preparedImportFile{}, "", "https://alidocs.dingtalk.com/i/nodes/node-1")
	verifyFailure(t, preparedImportFile{}, `{`, "https://alidocs.dingtalk.com/i/nodes/node-1")
	verifyFailure(t, preparedImportFile{}, `{}`, "https://alidocs.dingtalk.com/i/nodes/node-1")
	verifyFailure(t, preparedImportFile{}, `{"nodeId":"other"}`, "https://alidocs.dingtalk.com/i/nodes/node-1")
	verifyFailure(t, preparedImportFile{workspace: "space-1"}, `{"nodeId":"node-1","workspaceId":"wrong"}`, "https://alidocs.dingtalk.com/i/nodes/node-1")
}
