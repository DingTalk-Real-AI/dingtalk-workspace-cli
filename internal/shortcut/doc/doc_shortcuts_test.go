// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type docCoverageCaller struct {
	failAt    int
	calls     int
	responses map[string][]map[string]any
	ctx       context.Context
	history   []docCoverageCall
}

type docCoverageCall struct {
	tool   string
	params map[string]any
}

type docCoverageErrorReader struct{}

func (docCoverageErrorReader) Read([]byte) (int, error) { return 0, errors.New("stdin failed") }

func (f *docCoverageCaller) CallTool(_ context.Context, _, tool string, params map[string]any) (*edition.ToolResult, error) {
	f.calls++
	f.history = append(f.history, docCoverageCall{tool: tool, params: params})
	if f.failAt == f.calls {
		return nil, errors.New("injected doc coverage failure")
	}
	value := docCoveragePayload(tool)
	if queue := f.responses[tool]; len(queue) > 0 {
		value = queue[0]
		f.responses[tool] = queue[1:]
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(encoded)}}}, nil
}

func (f *docCoverageCaller) Format() string { return "json" }
func (f *docCoverageCaller) DryRun() bool   { return false }
func (f *docCoverageCaller) Fields() string { return "" }
func (f *docCoverageCaller) JQ() string     { return "" }

func docCoveragePayload(tool string) map[string]any {
	switch tool {
	case "create_document":
		return map[string]any{"data": map[string]any{"nodeId": "node-1"}}
	case "get_document_content":
		return map[string]any{"data": map[string]any{"revision": 1, "jsonml": `["root",{},["p",{"uuid":"block-1"},"alpha beta"]]`}}
	case "list_document_blocks":
		return map[string]any{"blocks": []any{map[string]any{"element": map[string]any{"id": "block-1", "paragraph": map[string]any{"text": "alpha beta"}}}}}
	case "submit_export_job":
		return map[string]any{"jobId": "job-1"}
	case "query_export_job":
		return map[string]any{"status": "SUCCESS", "downloadUrl": "https://download.dingtalk.com/export.docx"}
	case "list_doc_versions":
		return map[string]any{"versions": []any{map[string]any{"version": 3.0}, map[string]any{"versionNumber": "4"}}}
	case "search_doc_templates":
		return map[string]any{"templates": []any{map[string]any{"templateId": "template-1"}}}
	case "get_document_style":
		return map[string]any{"data": map[string]any{"cover": map[string]any{"resourceId": "resource-1", "imageUrl": "https://download.dingtalk.com/cover.png"}}}
	case "download_doc_attachment":
		return map[string]any{"downloadUrl": "https://download.dingtalk.com/file.bin", "fileName": "file.bin", "headers": map[string]any{"x-test": "ok", "ignored": 1}}
	case "list_comments":
		return map[string]any{"commentList": []any{map[string]any{"commentKey": "comment-1", "content": "review", "quote": "alpha"}}}
	default:
		return map[string]any{"ok": true, "result": map[string]any{"id": "id-1"}}
	}
}

func runDocCoverage(t *testing.T, declaration shortcut.Shortcut, caller *docCoverageCaller, args ...string) error {
	return runDocCoverageInput(t, declaration, caller, strings.NewReader(""), args...)
}

func runDocCoverageInput(t *testing.T, declaration shortcut.Shortcut, caller *docCoverageCaller, input io.Reader, args ...string) error {
	return runDocCoveragePath(t, declaration, caller, input, declaration.Command, args...)
}

func runDocCoveragePath(t *testing.T, declaration shortcut.Shortcut, caller *docCoverageCaller, input io.Reader, commandPath string, args ...string) error {
	t.Helper()
	helpers.InitDeps(caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	service := &cobra.Command{Use: "doc"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetIn(input)
	if caller.ctx != nil {
		root.SetContext(caller.ctx)
	}
	root.SetArgs(append([]string{"doc", commandPath}, args...))
	return root.Execute()
}

func TestCrossPlatformCoverageRevisionSelectionAndKeywordUseLiveShapes(t *testing.T) {
	revisionPayload := map[string]any{"data": map[string]any{"revision": json.Number("9")}}
	if got, ok := nestedRevision(revisionPayload); !ok || got != 9 {
		t.Fatalf("nestedRevision = %d/%v", got, ok)
	}

	blocks := map[string]any{"blocks": []any{
		map[string]any{"element": map[string]any{
			"id": "block-1", "paragraph": map[string]any{"text": "前缀😀真实追加：beta。后缀"},
		}},
	}}
	matches := findSelectionMatches(blocks, "真实追加：beta。")
	if len(matches) != 1 {
		t.Fatalf("matches = %#v", matches)
	}
	if got := matches[0]; got.blockID != "block-1" || got.start != 4 || got.end != 14 {
		t.Fatalf("selection match = %#v", got)
	}

	jsonml := `["root",{},["p",{"uuid":"block-jsonml"},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"旧入口兼容追加：gamma。"]]]]`
	projected := projectKeywordMatches(map[string]any{"jsonml": jsonml}, "gamma", 80, 120)
	if projected["count"] != 1 {
		t.Fatalf("keyword projection = %#v", projected)
	}
	rows := projected["matches"].([]map[string]any)
	if rows[0]["blockId"] != "block-jsonml" || rows[0]["content"] != "旧入口兼容追加：gamma。" {
		t.Fatalf("keyword row = %#v", rows[0])
	}

	unicodeProjection := projectKeywordMatches(map[string]any{"id": "unicode-block", "text": "KABtargetCD"}, "TARGET", 1, 1)
	unicodeRows := unicodeProjection["matches"].([]map[string]any)
	if len(unicodeRows) != 1 || unicodeRows[0]["content"] != "BtargetC" || !utf8.ValidString(unicodeRows[0]["content"].(string)) {
		t.Fatalf("unicode keyword projection = %#v", unicodeProjection)
	}
}

func TestCrossPlatformCoverageDocCompositePartialWriteContracts(t *testing.T) {
	assertPartial := func(t *testing.T, err error, reason, stage, nodeID string, wantSteps int) *apperrors.Error {
		t.Helper()
		if err == nil {
			t.Fatal("partial write unexpectedly succeeded")
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) {
			t.Fatalf("partial write error = %#v", err)
		}
		if typed.Reason != reason || typed.FailureStage != stage || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable {
			t.Fatalf("partial write metadata = %#v", typed)
		}
		if typed.Details["status"] != "partial_success" {
			t.Fatalf("partial write details = %#v", typed.Details)
		}
		data, _ := typed.Details["data"].(map[string]any)
		if data["nodeId"] != nodeID {
			t.Fatalf("partial write data = %#v", data)
		}
		steps, _ := typed.Details["steps"].([]map[string]any)
		if len(steps) != wantSteps || steps[0]["status"] != "success" || steps[len(steps)-1]["status"] == "success" {
			t.Fatalf("partial write steps = %#v", steps)
		}
		return typed
	}

	create := &docCoverageCaller{failAt: 2, responses: map[string][]map[string]any{}}
	err := runDocCoverage(t, Create, create, "--name", "n", "--content", `[]`, "--doc-format", "jsonml")
	typed := assertPartial(t, err, "doc_create_initial_content_failed", "write_jsonml", "node-1", 2)
	compensation, _ := typed.Details["compensation"].(map[string]any)
	if compensation["available"] != true || compensation["nodeId"] != "node-1" || len(create.history) != 2 {
		t.Fatalf("create compensation=%#v history=%#v", compensation, create.history)
	}

	checkpointUpdate := &docCoverageCaller{failAt: 2, responses: map[string][]map[string]any{
		"save_doc_version": {{"version": 7.0}},
	}}
	err = runDocCoverage(t, CheckpointUpdate, checkpointUpdate, "--node", "n", "--content", "body", "--yes")
	typed = assertPartial(t, err, "doc_checkpoint_update_failed", "update", "n", 3)
	data, _ := typed.Details["data"].(map[string]any)
	compensation, _ = typed.Details["compensation"].(map[string]any)
	if data["checkpointVersion"] != 7 || compensation["version"] != 7 {
		t.Fatalf("checkpoint recovery metadata data=%#v compensation=%#v", data, compensation)
	}

	checkpointVerify := &docCoverageCaller{failAt: 3, responses: map[string][]map[string]any{}}
	err = runDocCoverage(t, CheckpointUpdate, checkpointVerify, "--node", "n", "--content", "body", "--yes")
	assertPartial(t, err, "doc_checkpoint_verification_failed", "verify", "n", 3)

	historyVerify := &docCoverageCaller{failAt: 3, responses: map[string][]map[string]any{}}
	err = runDocCoverage(t, VersionRevert, historyVerify, "--node", "n", "--version", "3", "--yes")
	assertPartial(t, err, "doc_history_revert_verification_failed", "verify", "n", 3)
}

func TestCrossPlatformCoverageDocUpdateAliasReachesNestedBranches(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantTools []string
	}{
		{
			name:      "plain text replace",
			args:      []string{"--doc", "alias-node", "--command", "str_replace", "--old", "alpha", "--new", "gamma", "--yes"},
			wantTools: []string{"list_document_blocks", "update_document_block"},
		},
		{
			name:      "block copy",
			args:      []string{"--doc", "alias-node", "--command", "block_copy_insert_after", "--block-id", "block-1", "--after-block-id", "after", "--yes"},
			wantTools: []string{"list_document_blocks", "insert_document_block"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{}}
			if err := runDocCoverage(t, Update, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.history) != len(tc.wantTools) {
				t.Fatalf("calls = %#v", caller.history)
			}
			for index, call := range caller.history {
				if call.tool != tc.wantTools[index] || call.params["nodeId"] != "alias-node" {
					t.Fatalf("call %d = %#v, want tool=%s nodeId=alias-node", index, call, tc.wantTools[index])
				}
			}
		})
	}
}

func TestCrossPlatformCoverageSelectionMatchesEnumerateEveryCandidate(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		selection string
		want      int
	}{
		{name: "repeated omitted range", text: "left A right; left B right", selection: "left...right", want: 3},
		{name: "empty prefix", text: "one right; two right", selection: "...right", want: 2},
		{name: "empty suffix", text: "left one; left two", selection: "left...", want: 2},
		{name: "both empty anchors", text: "whole block", selection: "...", want: 1},
		{name: "empty block", text: "", selection: "...", want: 0},
		{name: "overlapping literal", text: "aaa", selection: "aa", want: 2},
		{name: "empty selection", text: "text", selection: "", want: 0},
		{name: "missing prefix", text: "text", selection: "left...right", want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matches := findSelectionMatches(map[string]any{"id": "block", "text": tc.text}, tc.selection)
			if len(matches) != tc.want {
				t.Fatalf("matches = %#v, want %d", matches, tc.want)
			}
		})
	}

	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"list_document_blocks": {{"items": []any{map[string]any{"id": "block", "text": "left A right; left B right"}}}},
	}}
	err := runDocCoverage(t, CommentCreate, caller, "--node", "n", "--content", "review", "--selection", "left...right", "--yes")
	if err == nil || !strings.Contains(err.Error(), "AMBIGUOUS_SELECTION") {
		t.Fatalf("same-block ambiguity error = %v", err)
	}
	if len(caller.history) != 1 || caller.history[0].tool != "list_document_blocks" {
		t.Fatalf("ambiguous selection reached a write: %#v", caller.history)
	}
}

func TestCrossPlatformCoverageDocDestructiveConfirmationBoundaries(t *testing.T) {
	tests := []struct {
		name string
		decl shortcut.Shortcut
		args []string
		want []docCoverageCall
	}{
		{
			name: "comment delete",
			decl: CommentDelete,
			args: []string{"--node", "n", "--comment-key", "c"},
			want: []docCoverageCall{{tool: "delete_comment", params: map[string]any{"nodeId": "n", "commentKey": "c"}}},
		},
		{
			name: "resource delete",
			decl: ResourceDelete,
			args: []string{"--node", "n"},
			want: []docCoverageCall{{tool: "update_document_style", params: map[string]any{"nodeId": "n", "cover": map[string]any{"action": "clear"}}}},
		},
		{
			name: "history revert",
			decl: VersionRevert,
			args: []string{"--node", "n", "--version", "3"},
			want: []docCoverageCall{
				{tool: "list_doc_versions", params: map[string]any{"nodeId": "n"}},
				{tool: "revert_doc_version", params: map[string]any{"nodeId": "n", "version": 3}},
				{tool: "get_document_info", params: map[string]any{"nodeId": "n"}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			unconfirmed := &docCoverageCaller{responses: map[string][]map[string]any{}}
			if err := runDocCoverage(t, tc.decl, unconfirmed, tc.args...); err == nil {
				t.Fatal("destructive shortcut without --yes must reject")
			}
			if unconfirmed.calls != 0 || len(unconfirmed.history) != 0 {
				t.Fatalf("unconfirmed shortcut called MCP: %#v", unconfirmed.history)
			}

			confirmed := &docCoverageCaller{responses: map[string][]map[string]any{}}
			if err := runDocCoverage(t, tc.decl, confirmed, append(tc.args, "--yes")...); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(confirmed.history, tc.want) {
				t.Fatalf("confirmed calls = %#v, want %#v", confirmed.history, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageReviewInfersInlineBlockFromUniqueQuote(t *testing.T) {
	comments := map[string]any{"commentList": []any{
		map[string]any{"commentKey": "inline", "content": "review", "isGlobal": false, "quote": "真实追加：beta。"},
		map[string]any{"commentKey": "global", "content": "global", "isGlobal": true},
	}}
	blocks := map[string]any{"blocks": []any{
		map[string]any{"element": map[string]any{"id": "block-1", "paragraph": map[string]any{"text": "真实追加：beta。"}}},
	}}
	items := projectReviewComments(comments, blocks)
	if len(items) != 2 {
		t.Fatalf("review items = %#v", items)
	}
	if items[0]["blockId"] != "block-1" || items[0]["context"] != "真实追加：beta。" {
		t.Fatalf("inline review = %#v", items[0])
	}
	if items[1]["blockId"] != "" {
		t.Fatalf("global review = %#v", items[1])
	}
}

func TestCrossPlatformCoverageDocContentCommandsAndFailureBoundaries(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("body.json", []byte(`["root",{},"body"]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("body.md", []byte("from file"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		cmd  shortcut.Shortcut
		args []string
	}{
		{"create markdown", Create, []string{"--name", "n", "--content", "body", "--folder", "f", "--workspace", "w"}},
		{"create dry", Create, []string{"--name", "n", "--content", "body", "--dry-run"}},
		{"create jsonml file", Create, []string{"--name", "n", "--content", "@body.json", "--doc-format", "jsonml"}},
		{"create stdin", Create, []string{"--name", "n", "--content", "-"}},
		{"fetch simple", Fetch, []string{"--node", "n"}},
		{"fetch keyword", Fetch, []string{"--node", "n", "--detail", "full", "--scope", "keyword", "--keyword", "alpha|none", "--context-before", "1", "--context-after", "1"}},
		{"fetch scoped", Fetch, []string{"--node", "n", "--scope", "range", "--start-block-id", "a", "--end-block-id", "b", "--tags", "p,h1", "--max-depth", "2"}},
		{"inspect base", Inspect, []string{"--node", "n"}},
		{"inspect all", Inspect, []string{"--node", "n", "--include-style", "--include-permissions", "--include-history", "--include-media", "--include-comments"}},
		{"update append", Update, []string{"--node", "n", "--command", "append", "--content", "x", "--yes"}},
		{"update overwrite jsonml", Update, []string{"--node", "n", "--command", "overwrite", "--content", `[]`, "--doc-format", "jsonml", "--yes"}},
		{"update insert text", Update, []string{"--node", "n", "--command", "block_insert_after", "--after-block-id", "b", "--content", "x", "--yes"}},
		{"update insert jsonml", Update, []string{"--node", "n", "--command", "block_insert_after", "--after-block-id", "b", "--content", `[]`, "--doc-format", "jsonml", "--yes"}},
		{"update replace text", Update, []string{"--node", "n", "--command", "block_replace", "--block-id", "b", "--content", "x", "--yes"}},
		{"update replace jsonml", Update, []string{"--node", "n", "--command", "block_replace", "--block-id", "b", "--content", `[]`, "--doc-format", "jsonml", "--yes"}},
		{"update delete", Update, []string{"--node", "n", "--command", "block_delete", "--block-id", "b", "--yes"}},
		{"update replace", Update, []string{"--node", "n", "--command", "str_replace", "--old", "alpha", "--new", "gamma", "--yes"}},
		{"update copy", Update, []string{"--node", "n", "--command", "block_copy_insert_after", "--block-id", "block-1", "--after-block-id", "b", "--yes"}},
		{"update revision", Update, []string{"--node", "n", "--command", "append", "--content", "x", "--expected-revision", "1", "--yes"}},
		{"update dry", Update, []string{"--node", "n", "--command", "append", "--content", "x", "--dry-run", "--yes"}},
		{"checkpoint dry", CheckpointUpdate, []string{"--node", "n", "--content", "x", "--dry-run", "--yes"}},
		{"checkpoint success", CheckpointUpdate, []string{"--node", "n", "--content", "x", "--yes"}},
		{"export dry", Export, []string{"--node", "n", "--output", "out.docx", "--dry-run"}},
		{"export success", Export, []string{"--node", "n", "--output", "out.docx"}},
	}

	testseam.Swap(t, &docDownload, func(_ context.Context, _ string, _ localio.DownloadOptions) (localio.DownloadResult, error) {
		return localio.DownloadResult{RelativePath: "out.docx", SizeBytes: 7}, nil
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{}}
			var err error
			if tc.name == "create stdin" {
				err = runDocCoverageInput(t, tc.cmd, caller, strings.NewReader("stdin body"), tc.args...)
			} else {
				err = runDocCoverage(t, tc.cmd, caller, tc.args...)
			}
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
		})
	}

	for _, command := range []shortcut.Shortcut{Create, Fetch, Inspect, Update, CheckpointUpdate, Export} {
		for failAt := 1; failAt <= 7; failAt++ {
			args := map[string][]string{
				"+create":            {"--name", "n", "--content", `[]`, "--doc-format", "jsonml"},
				"+fetch":             {"--node", "n", "--scope", "keyword", "--keyword", "x"},
				"+inspect":           {"--node", "n", "--include-style", "--include-permissions", "--include-history", "--include-media", "--include-comments"},
				"+update":            {"--node", "n", "--command", "append", "--content", "x", "--expected-revision", "1", "--yes"},
				"+checkpoint-update": {"--node", "n", "--content", "x", "--yes"},
				"+export":            {"--node", "n", "--output", "out.docx"},
			}[command.Command]
			_ = runDocCoverage(t, command, &docCoverageCaller{failAt: failAt, responses: map[string][]map[string]any{}}, args...)
		}
	}
}

func TestCrossPlatformCoverageDocContentValidationAndPureHelpers(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("body.md", []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(t.TempDir()), "outside.md")
	_ = outside
	badCases := []struct {
		cmd  shortcut.Shortcut
		args []string
	}{
		{Create, []string{"--name", "n", "--content", "@"}},
		{Create, []string{"--name", "n", "--content", "@/absolute"}},
		{Create, []string{"--name", "n", "--content", "not-json", "--doc-format", "jsonml"}},
		{Create, []string{"--name", "n", "--content", `{}`, "--doc-format", "jsonml"}},
		{Fetch, []string{"--node", "n", "--revision", "1"}},
		{Fetch, []string{"--node", "n", "--scope", "keyword"}},
		{Update, []string{"--node", "n"}},
		{Update, []string{"--command", "append", "--content", "x", "--yes"}},
		{Update, []string{"--node", "n", "--command", "append", "--yes"}},
		{Update, []string{"--node", "n", "--command", "block_delete", "--yes"}},
		{Update, []string{"--node", "n", "--command", "str_replace", "--old", "x", "--yes"}},
		{Update, []string{"--node", "n", "--command", "append", "--content", `[]`, "--doc-format", "jsonml", "--yes"}},
	}
	for _, tc := range badCases {
		if err := runDocCoverage(t, tc.cmd, &docCoverageCaller{responses: map[string][]map[string]any{}}, tc.args...); err == nil {
			t.Errorf("%s %#v unexpectedly succeeded", tc.cmd.Command, tc.args)
		}
	}

	createNoNode := &docCoverageCaller{responses: map[string][]map[string]any{"create_document": {{"ok": true}}}}
	if err := runDocCoverage(t, Create, createNoNode, "--name", "n", "--content", `[]`, "--doc-format", "jsonml"); err == nil {
		t.Fatal("jsonml create without node id succeeded")
	}
	conflict := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_content": {{"revision": 2}}}}
	if err := runDocCoverage(t, Update, conflict, "--node", "n", "--command", "append", "--content", "x", "--expected-revision", "1", "--yes"); err == nil {
		t.Fatal("revision conflict succeeded")
	}
	missingRevision := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_content": {{"ok": true}}}}
	if err := runDocCoverage(t, Update, missingRevision, "--node", "n", "--command", "append", "--content", "x", "--expected-revision", "1", "--yes"); err == nil {
		t.Fatal("missing revision succeeded")
	}

	for _, value := range []any{
		map[string]any{"revision": 2.5}, map[string]any{"revision": json.Number("bad")}, map[string]any{"revision": "bad"},
		map[string]any{"data": []any{map[string]any{"versionNumber": "3"}}}, []any{map[string]any{"version": 4.0}}, "none",
	} {
		_, _ = nestedRevision(value)
	}
	if _, err := validateJSONML(`[`); err == nil {
		t.Fatal("invalid jsonml succeeded")
	}
	if _, err := validateJSONML(`{}`); err == nil {
		t.Fatal("object jsonml succeeded")
	}
	if nestedMap(map[string]any{"result": map[string]any{"data": map[string]any{"x": 1}}})["x"] != 1 {
		t.Fatal("nestedMap did not unwrap")
	}
	_ = stringSliceNonEmpty([]string{"", " a "})

	blocks := map[string]any{"items": []any{map[string]any{"id": "b", "text": "alpha alpha"}, map[string]any{"id": "resource", "src": "x"}}}
	_ = projectKeywordMatches(blocks, "alpha|beta", -1, -1)
	_ = projectKeywordMatches(map[string]any{"jsonml": "bad"}, "none", 1, 1)
	_ = findBlock(blocks, "b")
	_ = findBlock([]any{blocks}, "missing")
	_ = containsResourceReference(blocks)
	_ = containsResourceReference([]any{map[string]any{"x": "y"}})
	stripBlockIDs(blocks)

	if err := runDocCoveragePath(t, Update, &docCoverageCaller{responses: map[string][]map[string]any{}}, strings.NewReader(""), "+update", "--doc", "n", "--command", "append", "--text", "legacy", "--yes"); err != nil {
		t.Fatalf("visible update flag aliases: %v", err)
	}
	missingNode := Update
	missingNode.Flags = append([]shortcut.Flag(nil), Update.Flags...)
	missingNode.Flags[0].Required = false
	if err := runDocCoverage(t, missingNode, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--command", "append", "--content", "x", "--yes"); err == nil {
		t.Fatal("custom missing-node validation was not reached")
	}
	unknown := Update
	unknown.Flags = append([]shortcut.Flag(nil), Update.Flags...)
	unknown.Flags[1].Enum = append(append([]string(nil), unknown.Flags[1].Enum...), "bogus")
	if err := runDocCoverage(t, unknown, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--command", "bogus", "--yes"); err == nil {
		t.Fatal("unknown update command succeeded")
	}

	_ = runDocCoverageInput(t, Create, &docCoverageCaller{responses: map[string][]map[string]any{}}, docCoverageErrorReader{}, "--name", "n", "--content", "-")
	for _, seamCase := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"getwd", func(t *testing.T) {
			testseam.Swap(t, &docGetwd, func() (string, error) { return "", errors.New("getwd") })
			_ = runDocCoverage(t, Create, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--name", "n", "--content", "@body.md")
		}},
		{"eval-base", func(t *testing.T) {
			testseam.Swap(t, &docEvalSymlinks, func(string) (string, error) { return "", errors.New("eval") })
			_ = runDocCoverage(t, Create, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--name", "n", "--content", "@body.md")
		}},
		{"eval-file", func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &docEvalSymlinks, func(value string) (string, error) {
				calls++
				if calls == 2 {
					return "", errors.New("eval file")
				}
				return value, nil
			})
			_ = runDocCoverage(t, Create, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--name", "n", "--content", "@body.md")
		}},
		{"rel", func(t *testing.T) {
			testseam.Swap(t, &docRel, func(string, string) (string, error) { return "", errors.New("rel") })
			_ = runDocCoverage(t, Create, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--name", "n", "--content", "@body.md")
		}},
		{"read", func(t *testing.T) {
			testseam.Swap(t, &docReadFile, func(string) ([]byte, error) { return nil, errors.New("read") })
			_ = runDocCoverage(t, Create, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--name", "n", "--content", "@body.md")
		}},
	} {
		t.Run(seamCase.name, seamCase.run)
	}

	_ = runDocCoverage(t, CheckpointUpdate, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--content", "@missing", "--yes")
	_ = runDocCoverage(t, Update, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--command", "append", "--content", "@missing", "--yes")
	_ = runDocCoverage(t, Update, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--command", "overwrite", "--content", "bad", "--doc-format", "jsonml", "--yes")
	_ = runDocCoverage(t, Update, &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}, "--node", "n", "--command", "str_replace", "--old", "alpha", "--new", "x", "--yes")
	_ = runDocCoverage(t, Update, &docCoverageCaller{responses: map[string][]map[string]any{"list_document_blocks": {{"items": []any{map[string]any{"id": "a", "text": "alpha"}, map[string]any{"id": "b", "text": "alpha"}}}}}}, "--node", "n", "--command", "str_replace", "--old", "alpha", "--new", "x", "--yes")
	_ = runDocCoverage(t, Update, &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}, "--node", "n", "--command", "block_copy_insert_after", "--block-id", "block-1", "--yes")
	_ = runDocCoverage(t, Update, &docCoverageCaller{responses: map[string][]map[string]any{"list_document_blocks": {{"ok": true}}}}, "--node", "n", "--command", "block_copy_insert_after", "--block-id", "missing", "--yes")
	_ = runDocCoverage(t, Update, &docCoverageCaller{responses: map[string][]map[string]any{"list_document_blocks": {{"id": "block-1", "resourceId": "r"}}}}, "--node", "n", "--command", "block_copy_insert_after", "--block-id", "block-1", "--yes")

	for _, response := range []map[string]any{
		{"ok": true},
		{"jobId": "j"},
	} {
		caller := &docCoverageCaller{responses: map[string][]map[string]any{"submit_export_job": {response}, "query_export_job": {{"status": "FAILED", "message": "bad"}}}}
		_ = runDocCoverage(t, Export, caller, "--node", "n", "--output", "x", "--max-polls", "1")
	}
	timeout := &docCoverageCaller{responses: map[string][]map[string]any{"query_export_job": {{"status": "PROCESSING"}}}}
	_ = runDocCoverage(t, Export, timeout, "--node", "n", "--output", "x", "--max-polls", "1")
	processingThenSuccess := &docCoverageCaller{responses: map[string][]map[string]any{"query_export_job": {{"status": "PROCESSING"}, {"status": "SUCCESS", "downloadUrl": "https://download.dingtalk.com/x"}}}}
	_ = runDocCoverage(t, Export, processingThenSuccess, "--node", "n", "--output", "x", "--max-polls", "2")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cancelledCaller := &docCoverageCaller{ctx: cancelled, responses: map[string][]map[string]any{"query_export_job": {{"status": "PROCESSING"}}}}
	_ = runDocCoverage(t, Export, cancelledCaller, "--node", "n", "--output", "x", "--max-polls", "2")
	_ = runDocCoverage(t, Export, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--output", "x", "--max-polls", "0")
	missingURL := &docCoverageCaller{responses: map[string][]map[string]any{"query_export_job": {{"status": "SUCCESS"}}}}
	_ = runDocCoverage(t, Export, missingURL, "--node", "n", "--output", "x")
}

func TestCrossPlatformCoverageDocHistoryTemplateReviewAndMedia(t *testing.T) {
	t.Chdir(t.TempDir())
	testseam.Swap(t, &docDownload, func(_ context.Context, _ string, _ localio.DownloadOptions) (localio.DownloadResult, error) {
		return localio.DownloadResult{RelativePath: "artifact.bin", SizeBytes: 9}, nil
	})
	commands := []struct {
		decl shortcut.Shortcut
		args []string
	}{
		{VersionList, []string{"--node", "n", "--page-size", "2", "--page-token", "p"}},
		{VersionList, []string{"--node", "n", "--limit", "2", "--cursor", "p"}},
		{VersionRevert, []string{"--node", "n", "--version", "3", "--yes"}},
		{VersionRevert, []string{"--node", "n", "--version", "3", "--dry-run", "--yes"}},
		{CreateFromTemplate, []string{"--template-id", "t", "--name", "n", "--folder", "f", "--workspace", "w"}},
		{CreateFromTemplate, []string{"--query", "q", "--source", "PUBLIC", "--dry-run"}},
		{Review, []string{"--node", "n"}},
		{CommentUpdate, []string{"--node", "n", "--comment-key", "c", "--content", "x", "--mention", "u"}},
		{CommentDelete, []string{"--node", "n", "--comment-key", "c", "--dry-run", "--yes"}},
		{CommentCreate, []string{"--node", "n", "--content", "x", "--yes"}},
		{CommentCreate, []string{"--node", "n", "--content", "x", "--block-id", "b", "--start", "0", "--end", "1", "--selected-text", "a", "--mention", "u", "--yes"}},
		{CommentCreate, []string{"--node", "n", "--content", "x", "--selection", "alpha", "--yes"}},
		{MediaList, []string{"--node", "n"}},
		{MediaPreview, []string{"--node", "n", "--resource-id", "r"}},
		{MediaPreview, []string{"--node", "n", "--resource-id", "r", "--dry-run"}},
		{MediaDownload, []string{"--node", "n", "--resource-id", "r", "--output", "m.bin"}},
		{MediaDownload, []string{"--node", "n", "--resource-id", "r", "--output", "m.bin", "--dry-run"}},
		{ResourceDownload, []string{"--node", "n", "--output", "cover.png"}},
		{ResourceDownload, []string{"--node", "n", "--output", "cover.png", "--dry-run"}},
		{ResourceDelete, []string{"--node", "n", "--dry-run", "--yes"}},
		{BackgroundUpdate, []string{"--node", "n", "--color", "#ABCDEF"}},
		{BackgroundDelete, []string{"--node", "n", "--dry-run", "--yes"}},
		{BackgroundDelete, []string{"--node", "n", "--yes"}},
		{ResourceDelete, []string{"--node", "n", "--yes"}},
		{CommentDelete, []string{"--node", "n", "--comment-key", "c", "--yes"}},
	}
	for _, item := range commands {
		if err := runDocCoverage(t, item.decl, &docCoverageCaller{responses: map[string][]map[string]any{}}, item.args...); err != nil {
			t.Errorf("%s: %v", item.decl.Command, err)
		}
	}

	for _, declaration := range []shortcut.Shortcut{VersionRevert, CreateFromTemplate, Review, MediaList, MediaDownload, ResourceDownload} {
		args := map[string][]string{
			"+history-revert":       {"--node", "n", "--version", "3", "--yes"},
			"+create-from-template": {"--query", "q"},
			"+review":               {"--node", "n"},
			"+media-list":           {"--node", "n"},
			"+media-download":       {"--node", "n", "--resource-id", "r", "--output", "m.bin"},
			"+resource-download":    {"--node", "n", "--output", "cover.png"},
		}[declaration.Command]
		for failAt := 1; failAt <= 4; failAt++ {
			_ = runDocCoverage(t, declaration, &docCoverageCaller{failAt: failAt, responses: map[string][]map[string]any{}}, args...)
		}
	}

	for _, value := range []any{
		map[string]any{"version": 3.0}, map[string]any{"version": 3.5}, map[string]any{"version": "3"}, map[string]any{"version": "bad"},
		[]any{map[string]any{"revision": 3.0}}, "none",
	} {
		_ = containsVersion(value, 3)
	}
	_ = collectTemplateIDs(map[string]any{"template_id": "t1", "nested": []any{map[string]any{"templateId": "t1"}, map[string]any{"templateId": "t2"}, "x"}})
	_ = collectTemplateIDs("none")
	_ = collectMediaItems(map[string]any{"id": "b", "resourceId": "r", "src": "u", "name": "n", "type": "file", "mimeType": "x", "viewType": "v", "children": []any{map[string]any{"resourceUrl": "u2"}}})
	_ = nestedStringDeep([]any{map[string]any{"x": map[string]any{"url": " u "}}}, "url")
	_ = nestedStringDeep("none", "url")

	badComments := [][]string{
		{"--node", "n", "--content", "x", "--block-id", "b", "--yes"},
		{"--node", "n", "--content", "x", "--block-id", "b", "--start", "2", "--end", "1", "--yes"},
		{"--node", "n", "--content", "x", "--block-id", "b", "--start", "0", "--end", "1", "--selection", "x", "--yes"},
	}
	for _, args := range badComments {
		if err := runDocCoverage(t, CommentCreate, &docCoverageCaller{responses: map[string][]map[string]any{}}, args...); err == nil {
			t.Errorf("invalid comment args succeeded: %#v", args)
		}
	}
	ambiguous := &docCoverageCaller{responses: map[string][]map[string]any{"list_document_blocks": {{"items": []any{map[string]any{"id": "a", "text": "x"}, map[string]any{"id": "b", "text": "x"}}}}}}
	if err := runDocCoverage(t, CommentCreate, ambiguous, "--node", "n", "--content", "c", "--selection", "x", "--yes"); err == nil {
		t.Fatal("ambiguous comment selection succeeded")
	}
	_ = findSelectionMatches(map[string]any{"id": "b", "text": "left middle right"}, "left...right")
	_ = findSelectionMatches([]any{map[string]any{"id": "b", "text": "none"}}, "x")
	_ = runDocCoverage(t, CommentCreate, &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}, "--node", "n", "--content", "c", "--selection", "x", "--yes")

	globalReview := &docCoverageCaller{responses: map[string][]map[string]any{"list_comments": {{"comments": []any{map[string]any{"commentKey": "global", "content": "g"}}}}}}
	_ = runDocCoverage(t, Review, globalReview, "--node", "n")
	_ = runDocCoverage(t, VersionRevert, &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}, "--node", "n", "--version", "3", "--yes")
	_ = runDocCoverage(t, VersionRevert, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--version", "99", "--yes")
	_ = runDocCoverage(t, CreateFromTemplate, &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}, "--template-id", "t")
	multipleTemplates := &docCoverageCaller{responses: map[string][]map[string]any{"search_doc_templates": {{"templates": []any{map[string]any{"templateId": "a"}, map[string]any{"templateId": "b"}}}}}}
	_ = runDocCoverage(t, CreateFromTemplate, multipleTemplates, "--query", "q")

	if err := os.WriteFile("media.bin", []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = runDocCoverage(t, MediaInsert, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--file", "media.bin", "--dry-run", "--yes")
	_ = runDocCoverage(t, ResourceUpdate, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--image", "https://example.com/cover.png", "--dry-run", "--yes")
	_ = runDocCoverage(t, Import, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--file", "media.bin", "--folder", "f", "--dry-run")

	resourceOnly := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_style": {{"resourceId": "r"}}}}
	_ = runDocCoverage(t, ResourceDownload, resourceOnly, "--node", "n", "--output", "cover.png")
	emptyStyle := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_style": {{"ok": true}}}}
	_ = runDocCoverage(t, ResourceDownload, emptyStyle, "--node", "n", "--output", "cover.png")
	_ = runDocCoverage(t, ResourceDownload, &docCoverageCaller{failAt: 2, responses: map[string][]map[string]any{"get_document_style": {{"resourceId": "r"}}}}, "--node", "n", "--output", "cover.png")
	_, _ = downloadResolvedResource(nil, map[string]any{}, ".", "x")
	_ = runDocCoverage(t, MediaPreview, &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}, "--node", "n", "--resource-id", "r")
	_ = runDocCoverage(t, BackgroundUpdate, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--color", "bad")
	_ = runDocCoverage(t, BackgroundUpdate, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--color", "#ABCDEG")

	t.Run("preview mkdir failure", func(t *testing.T) {
		testseam.Swap(t, &docMkdirTemp, func(string, string) (string, error) { return "", errors.New("mkdir") })
		_ = runDocCoverage(t, MediaPreview, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--resource-id", "r")
	})
	t.Run("preview download cleanup", func(t *testing.T) {
		removed := false
		testseam.Swap(t, &docDownload, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
			return localio.DownloadResult{}, errors.New("download")
		})
		testseam.Swap(t, &docRemoveAll, func(string) error { removed = true; return nil })
		_ = runDocCoverage(t, MediaPreview, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--resource-id", "r")
		if !removed {
			t.Fatal("preview failure did not clean temporary directory")
		}
	})
}

func TestCrossPlatformCoverageDocDownloadAndWorkingDirectoryErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	testseam.Swap(t, &docDownload, func(_ context.Context, _ string, _ localio.DownloadOptions) (localio.DownloadResult, error) {
		return localio.DownloadResult{}, errors.New("download failed")
	})
	for _, item := range []struct {
		decl shortcut.Shortcut
		args []string
	}{
		{Export, []string{"--node", "n", "--output", "out.docx"}},
		{MediaDownload, []string{"--node", "n", "--resource-id", "r", "--output", "out.bin"}},
		{ResourceDownload, []string{"--node", "n", "--output", "out.png"}},
	} {
		if err := runDocCoverage(t, item.decl, &docCoverageCaller{responses: map[string][]map[string]any{}}, item.args...); err == nil {
			t.Errorf("%s download error was ignored", item.decl.Command)
		}
	}

	testseam.Swap(t, &docGetwd, func() (string, error) { return "", errors.New("getwd failed") })
	for _, item := range []struct {
		decl shortcut.Shortcut
		args []string
	}{
		{Export, []string{"--node", "n", "--output", "out.docx"}},
		{MediaDownload, []string{"--node", "n", "--resource-id", "r", "--output", "out.bin"}},
		{ResourceDownload, []string{"--node", "n", "--output", "out.png"}},
	} {
		_ = runDocCoverage(t, item.decl, &docCoverageCaller{responses: map[string][]map[string]any{}}, item.args...)
	}
}

func TestCrossPlatformCoverageDocDownloadsHaveNoOverwriteEscape(t *testing.T) {
	for _, item := range []struct {
		decl shortcut.Shortcut
		args []string
	}{
		{Export, []string{"--node", "n", "--output", "out.docx"}},
		{MediaDownload, []string{"--node", "n", "--resource-id", "r", "--output", "out.bin"}},
		{ResourceDownload, []string{"--node", "n", "--output", "out.png"}},
	} {
		t.Run(item.decl.Command, func(t *testing.T) {
			for _, flag := range item.decl.Flags {
				if flag.Name == "overwrite" {
					t.Fatal("download shortcut still declares --overwrite")
				}
			}
			caller := &docCoverageCaller{responses: map[string][]map[string]any{}}
			err := runDocCoverage(t, item.decl, caller, append(item.args, "--overwrite")...)
			if err == nil {
				t.Fatal("--overwrite unexpectedly accepted")
			}
			if caller.calls != 0 {
				t.Fatalf("rejected --overwrite performed %d MCP calls", caller.calls)
			}
		})
	}
}
