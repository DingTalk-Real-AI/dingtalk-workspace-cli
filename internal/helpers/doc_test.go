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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type docCommandRunner struct {
	calls     int
	last      executor.Invocation
	all       []executor.Invocation
	responses []map[string]any
}

func (r *docCommandRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.calls++
	r.last = invocation
	r.all = append(r.all, invocation)
	result := executor.Result{Invocation: invocation}
	if idx := r.calls - 1; idx >= 0 && idx < len(r.responses) {
		result.Response = r.responses[idx]
	}
	return result, nil
}

func executeDocCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, string, error) {
	t.Helper()

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func newDocTestRoot(runner executor.Runner) *cobra.Command {
	cmd := docHandler{}.Command(runner)
	cmd.PersistentFlags().Bool("dry-run", false, "dry run")
	cmd.PersistentFlags().Bool("yes", false, "skip confirmation")
	return cmd
}

func TestDocPermissionListLimitAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want int
	}{
		{name: "limit", args: []string{"--node", "NODE_001", "--limit", "50"}, want: 50},
		{name: "max results", args: []string{"--node", "NODE_001", "--max-results", "40"}, want: 40},
		{name: "page size", args: []string{"--node", "NODE_001", "--page-size", "10"}, want: 10},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &docCommandRunner{}
			cmd := newDocPermissionListCommand(runner)
			_, errOut, err := executeDocCommand(t, cmd, tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
			}
			if runner.last.Tool != "list_permission" {
				t.Fatalf("tool = %q, want list_permission", runner.last.Tool)
			}
			if got := runner.last.Params["maxResults"]; got != tc.want {
				t.Fatalf("maxResults = %#v, want %d", got, tc.want)
			}
		})
	}
}

func TestDocPermissionListMaxresultsRejected(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocPermissionListCommand(runner)
	_, _, err := executeDocCommand(t, cmd, "--node", "NODE_001", "--maxresults", "10")
	if err == nil {
		t.Fatal("Execute() error = nil, want unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown flag: --maxresults") {
		t.Fatalf("error = %q, want unknown flag for --maxresults", err.Error())
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestDocUpdateOverwriteRequiresYes(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocTestRoot(runner)
	_, _, err := executeDocCommand(t, cmd,
		"update",
		"--node", "NODE_001",
		"--content", "# overwrite probe",
		"--mode", "overwrite",
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want --yes validation failure")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %q, want --yes hint", err.Error())
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestDocUpdateOverwriteAllowsYesAndDryRun(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		extraArgs  []string
		wantDryRun bool
	}{
		{name: "yes", extraArgs: []string{"--yes"}},
		{name: "dry run", extraArgs: []string{"--dry-run"}, wantDryRun: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &docCommandRunner{}
			cmd := newDocTestRoot(runner)
			args := []string{
				"update",
				"--node", "NODE_001",
				"--content", "# overwrite probe",
				"--mode", "overwrite",
			}
			args = append(args, tc.extraArgs...)
			_, errOut, err := executeDocCommand(t, cmd, args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
			}
			if runner.calls != 1 {
				t.Fatalf("runner calls = %d, want 1", runner.calls)
			}
			if runner.last.DryRun != tc.wantDryRun {
				t.Fatalf("DryRun = %v, want %v", runner.last.DryRun, tc.wantDryRun)
			}
		})
	}
}

func TestDocListAcceptsFolderCompatibilityAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{name: "node", args: []string{"--node", "FOLDER_001"}},
		{name: "file id", args: []string{"--file-id", "FOLDER_001"}},
		{name: "nodee typo", args: []string{"--nodee", "FOLDER_001"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &docCommandRunner{}
			cmd := newDocListCommand(runner)
			_, errOut, err := executeDocCommand(t, cmd, tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
			}
			if runner.last.Tool != "list_nodes" {
				t.Fatalf("tool = %q, want list_nodes", runner.last.Tool)
			}
			if got := runner.last.Params["folderId"]; got != "FOLDER_001" {
				t.Fatalf("folderId = %#v, want FOLDER_001", got)
			}
		})
	}
}

func TestDocListAcceptsWukongPaginationAliases(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocListCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd, "--workspace", "WS_001", "--limit", "20", "--cursor", "TOKEN_001")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "list_nodes" {
		t.Fatalf("tool = %q, want list_nodes", runner.last.Tool)
	}
	if got := runner.last.Params["workspaceId"]; got != "WS_001" {
		t.Fatalf("workspaceId = %#v, want WS_001", got)
	}
	if got := runner.last.Params["pageSize"]; got != 20 {
		t.Fatalf("pageSize = %#v, want 20", got)
	}
	if got := runner.last.Params["pageToken"]; got != "TOKEN_001" {
		t.Fatalf("pageToken = %#v, want TOKEN_001", got)
	}
}

func TestDocSearchAcceptsWukongPaginationAliases(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocSearchCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd, "--query", "方案", "--limit", "20", "--cursor", "TOKEN_001")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "search_documents" {
		t.Fatalf("tool = %q, want search_documents", runner.last.Tool)
	}
	if got := runner.last.Params["pageSize"]; got != 20 {
		t.Fatalf("pageSize = %#v, want 20", got)
	}
	if got := runner.last.Params["pageToken"]; got != "TOKEN_001" {
		t.Fatalf("pageToken = %#v, want TOKEN_001", got)
	}
}

func TestDocExportDryRunPassesWukongExportFormat(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocTestRoot(runner)
	out, errOut, err := executeDocCommand(t, cmd,
		"--dry-run",
		"export",
		"--node", "NODE_001",
		"--output", "out.docx",
		"--export-format", "docx",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0 for dry-run", runner.calls)
	}
	var inv executor.Invocation
	if err := json.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", out, err)
	}
	if inv.Tool != "submit_export_job" {
		t.Fatalf("tool = %q, want submit_export_job", inv.Tool)
	}
	if got := inv.Params["exportFormat"]; got != "docx" {
		t.Fatalf("exportFormat = %#v, want docx", got)
	}
}

func TestDocUploadDryRunUsesWukongFileUploadWorkflow(t *testing.T) {
	t.Parallel()

	contentPath := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(contentPath, []byte("pdf"), 0600); err != nil {
		t.Fatal(err)
	}

	runner := &docCommandRunner{}
	cmd := newDocTestRoot(runner)
	out, errOut, err := executeDocCommand(t, cmd,
		"--dry-run",
		"upload",
		"--file", contentPath,
		"--name", "Q1汇报",
		"--folder", "FOLDER_001",
		"--convert",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0 for dry-run", runner.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", out, err)
	}
	step1, ok := payload["step_1_get_file_upload_info"].(map[string]any)
	if !ok {
		t.Fatalf("missing step_1_get_file_upload_info in %#v", payload)
	}
	if got := step1["tool"]; got != "get_file_upload_info" {
		t.Fatalf("step1 tool = %#v, want get_file_upload_info", got)
	}
	step3, ok := payload["step_3_commit_uploaded_file"].(map[string]any)
	if !ok {
		t.Fatalf("missing step_3_commit_uploaded_file in %#v", payload)
	}
	params, ok := step3["params"].(map[string]any)
	if !ok {
		t.Fatalf("step3 params type = %T", step3["params"])
	}
	if got := params["name"]; got != "Q1汇报.pdf" {
		t.Fatalf("name = %#v, want Q1汇报.pdf", got)
	}
	if got := params["folderId"]; got != "FOLDER_001" {
		t.Fatalf("folderId = %#v, want FOLDER_001", got)
	}
	if got := params["convertToOnlineDoc"]; got != true {
		t.Fatalf("convertToOnlineDoc = %#v, want true", got)
	}
}

func TestDocCommentListAcceptsWukongPaginationAliases(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocCommentListCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd, "--node", "NODE_001", "--limit", "20", "--cursor", "TOKEN_001")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "list_comments" {
		t.Fatalf("tool = %q, want list_comments", runner.last.Tool)
	}
	if got := runner.last.Params["pageSize"]; got != 20 {
		t.Fatalf("pageSize = %#v, want 20", got)
	}
	if got := runner.last.Params["nextToken"]; got != "TOKEN_001" {
		t.Fatalf("nextToken = %#v, want TOKEN_001", got)
	}
}

func TestDocCommentReplyEmojiIsBoolFlag(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocCommentReplyCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd,
		"--node", "NODE_001",
		"--comment-key", "COMMENT_KEY",
		"--content", "比心",
		"--emoji",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "reply_comment" {
		t.Fatalf("tool = %q, want reply_comment", runner.last.Tool)
	}
	if got := runner.last.Params["emoji"]; got != true {
		t.Fatalf("emoji = %#v, want true", got)
	}
}

func TestDocCreateAcceptsParentFolderAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{
			name: "parent folder id",
			args: []string{"--name", "doc", "--parent-folder-id", "FOLDER_001", "--content", "hello"},
		},
		{
			name: "parent folder",
			args: []string{"--name", "doc", "--parent-folder", "FOLDER_001", "--content", "hello"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &docCommandRunner{}
			cmd := newDocCreateCommand(runner)
			_, errOut, err := executeDocCommand(t, cmd, tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
			}
			if runner.last.Tool != "create_document" {
				t.Fatalf("tool = %q, want create_document", runner.last.Tool)
			}
			if got := runner.last.Params["folderId"]; got != "FOLDER_001" {
				t.Fatalf("folderId = %#v, want FOLDER_001", got)
			}
		})
	}
}

func TestDocCreateAcceptsContentPathAlias(t *testing.T) {
	t.Parallel()

	contentPath := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(contentPath, []byte("# from file"), 0600); err != nil {
		t.Fatal(err)
	}

	runner := &docCommandRunner{}
	cmd := newDocCreateCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd, "--name", "doc", "--content-path", contentPath)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "create_document" {
		t.Fatalf("tool = %q, want create_document", runner.last.Tool)
	}
	if got := runner.last.Params["markdown"]; got != "# from file" {
		t.Fatalf("markdown = %#v, want # from file", got)
	}
}

func TestDocReadPassesJsonMLFormatAndOutput(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocReadCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd,
		"--node", "NODE_001",
		"--content-format", "jsonml",
		"--output", "body.json",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "get_document_content" {
		t.Fatalf("tool = %q, want get_document_content", runner.last.Tool)
	}
	if got := runner.last.Params["format"]; got != "jsonml" {
		t.Fatalf("format = %#v, want jsonml", got)
	}
	if got := runner.last.Params["__output__"]; got != "body.json" {
		t.Fatalf("__output__ = %#v, want body.json", got)
	}
}

func TestDocCreatePassesJsonMLContentAndFixFlag(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{responses: []map[string]any{{"nodeId": "NODE_NEW"}}}
	cmd := newDocCreateCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd,
		"--name", "doc",
		"--content", `{"jsonml":[["p",{},"hello"]]}`,
		"--content-format", "jsonml",
		"--fix-jsonml",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.calls != 2 {
		t.Fatalf("calls = %d, want 2", runner.calls)
	}
	if runner.all[0].Tool != "create_document" {
		t.Fatalf("first tool = %q, want create_document", runner.all[0].Tool)
	}
	if runner.last.Tool != "update_document" {
		t.Fatalf("last tool = %q, want update_document", runner.last.Tool)
	}
	if got := runner.last.Params["format"]; got != "jsonml" {
		t.Fatalf("format = %#v, want jsonml", got)
	}
	jsonml, ok := runner.last.Params["jsonml"].(string)
	if !ok || !strings.Contains(jsonml, `"root"`) || !strings.Contains(jsonml, `"hello"`) {
		t.Fatalf("jsonml = %#v, want normalized root JSONML", runner.last.Params["jsonml"])
	}
	if _, ok := runner.last.Params["markdown"]; ok {
		t.Fatalf("markdown = %#v, want omitted", runner.last.Params["markdown"])
	}
	if _, ok := runner.last.Params["fixJsonml"]; ok {
		t.Fatalf("fixJsonml = %#v, want omitted", runner.last.Params["fixJsonml"])
	}
}

func TestDocUpdatePassesJsonMLRevisionAndNoFixFlag(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocTestRoot(runner)
	_, errOut, err := executeDocCommand(t, cmd,
		"update",
		"--node", "NODE_001",
		"--content", `["root",{},["p",{},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"updated"]]]]`,
		"--content-format", "jsonml",
		"--revision", "42",
		"--no-fix-jsonml",
		"--mode", "overwrite",
		"--yes",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "update_document" {
		t.Fatalf("tool = %q, want update_document", runner.last.Tool)
	}
	if got := runner.last.Params["format"]; got != "jsonml" {
		t.Fatalf("format = %#v, want jsonml", got)
	}
	if got := runner.last.Params["revision"]; got != 42 {
		t.Fatalf("revision = %#v, want 42", got)
	}
	if _, ok := runner.last.Params["noFixJsonml"]; ok {
		t.Fatalf("noFixJsonml = %#v, want omitted", runner.last.Params["noFixJsonml"])
	}
	if _, ok := runner.last.Params["index"]; ok {
		t.Fatalf("index = %#v, want omitted for JSONML update", runner.last.Params["index"])
	}
}

func TestDocBlockListPassesJsonMLFormatAndBlockID(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocBlockListCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd,
		"--node", "DOC_001",
		"--content-format", "jsonml",
		"--block-id", "BLOCK_001",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "list_document_blocks" {
		t.Fatalf("tool = %q, want list_document_blocks", runner.last.Tool)
	}
	if got := runner.last.Params["format"]; got != "jsonml" {
		t.Fatalf("format = %#v, want jsonml", got)
	}
	if got := runner.last.Params["blockId"]; got != "BLOCK_001" {
		t.Fatalf("blockId = %#v, want BLOCK_001", got)
	}
}

func TestDocBlockInsertPassesJsonMLAndParentBlock(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocBlockInsertCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd,
		"--node", "DOC_001",
		"--content-format", "jsonml",
		"--element", `["p",{},"hello"]`,
		"--parent-block", "PARENT_001",
		"--index", "1",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "insert_document_block" {
		t.Fatalf("tool = %q, want insert_document_block", runner.last.Tool)
	}
	if got := runner.last.Params["format"]; got != "jsonml" {
		t.Fatalf("format = %#v, want jsonml", got)
	}
	jsonml, ok := runner.last.Params["jsonml"].(string)
	if !ok || !strings.Contains(jsonml, `"span"`) || !strings.Contains(jsonml, `"hello"`) {
		t.Fatalf("jsonml = %#v, want normalized JSONML node", runner.last.Params["jsonml"])
	}
	if got := runner.last.Params["referenceBlockId"]; got != "PARENT_001" {
		t.Fatalf("referenceBlockId = %#v, want PARENT_001", got)
	}
	if got := runner.last.Params["index"]; got != 1 {
		t.Fatalf("index = %#v, want 1", got)
	}
	if _, ok := runner.last.Params["element"]; ok {
		t.Fatalf("element = %#v, want omitted", runner.last.Params["element"])
	}
}

func TestDocBlockUpdatePassesJsonMLAndFixFlags(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocBlockUpdateCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd,
		"--node", "DOC_001",
		"--block-id", "BLOCK_001",
		"--content-format", "jsonml",
		"--element", `["p",{},"new"]`,
		"--fix-jsonml",
		"--no-fix-jsonml",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "update_document_block" {
		t.Fatalf("tool = %q, want update_document_block", runner.last.Tool)
	}
	if got := runner.last.Params["format"]; got != "jsonml" {
		t.Fatalf("format = %#v, want jsonml", got)
	}
	if got := runner.last.Params["jsonml"]; got != `["p",{},"new"]` {
		t.Fatalf("jsonml = %#v, want original node when --no-fix-jsonml wins", got)
	}
	if _, ok := runner.last.Params["fixJsonml"]; ok {
		t.Fatalf("fixJsonml = %#v, want omitted", runner.last.Params["fixJsonml"])
	}
	if _, ok := runner.last.Params["noFixJsonml"]; ok {
		t.Fatalf("noFixJsonml = %#v, want omitted", runner.last.Params["noFixJsonml"])
	}
}

func TestDocBlockInsertTypeCallout(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocBlockInsertCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd,
		"--node", "DOC_001",
		"--type", "callout",
		"--text", "接口变更通知；DBA 审核；告警规则；安全评审",
		"--where", "end",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "insert_document_block" {
		t.Fatalf("tool = %q, want insert_document_block", runner.last.Tool)
	}
	if _, ok := runner.last.Params["where"]; ok {
		t.Fatalf("where = %#v, want omitted for --where end", runner.last.Params["where"])
	}
	element, ok := runner.last.Params["element"].(map[string]any)
	if !ok {
		t.Fatalf("element = %#v, want map", runner.last.Params["element"])
	}
	if got := element["blockType"]; got != "callout" {
		t.Fatalf("blockType = %#v, want callout", got)
	}
	callout, ok := element["callout"].(map[string]any)
	if !ok {
		t.Fatalf("callout = %#v, want map", element["callout"])
	}
	if got := callout["text"]; got != "接口变更通知；DBA 审核；告警规则；安全评审" {
		t.Fatalf("callout.text = %#v", got)
	}
}

func TestDocBlockInsertTypeListAndColumns(t *testing.T) {
	t.Parallel()

	t.Run("ordered list", func(t *testing.T) {
		t.Parallel()

		runner := &docCommandRunner{}
		cmd := newDocBlockInsertCommand(runner)
		_, errOut, err := executeDocCommand(t, cmd,
			"--node", "DOC_001",
			"--type", "ordered-list",
			"--list-id", "schedule",
			"--text", "需求评审",
		)
		if err != nil {
			t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
		}
		element := runner.last.Params["element"].(map[string]any)
		if got := element["blockType"]; got != "orderedList" {
			t.Fatalf("blockType = %#v, want orderedList", got)
		}
		list := element["orderedList"].(map[string]any)["list"].(map[string]any)
		if got := list["listId"]; got != "schedule" {
			t.Fatalf("listId = %#v, want schedule", got)
		}
	})

	t.Run("columns", func(t *testing.T) {
		t.Parallel()

		runner := &docCommandRunner{}
		cmd := newDocBlockInsertCommand(runner)
		_, errOut, err := executeDocCommand(t, cmd,
			"--node", "DOC_001",
			"--type", "columns",
			"--columns", "2",
			"--text", "方案A||方案B",
		)
		if err != nil {
			t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
		}
		element := runner.last.Params["element"].(map[string]any)
		if got := element["blockType"]; got != "columns" {
			t.Fatalf("blockType = %#v, want columns", got)
		}
		columns := element["columns"].(map[string]any)
		if got := columns["size"]; got != 2 {
			t.Fatalf("columns.size = %#v, want 2", got)
		}
		children := element["children"].([]any)
		if len(children) != 2 {
			t.Fatalf("children len = %d, want 2", len(children))
		}
	})
}
