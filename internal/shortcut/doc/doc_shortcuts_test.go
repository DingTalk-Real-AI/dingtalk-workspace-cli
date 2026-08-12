// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type docCoverageCaller struct {
	mu        sync.Mutex
	failAt    int
	calls     int
	dryRun    bool
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
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.history = append(f.history, docCoverageCall{tool: tool, params: params})
	if f.failAt == f.calls {
		return nil, errors.New("injected doc coverage failure")
	}
	value := docCoveragePayload(tool)
	if tool == "revert_doc_version" {
		value = map[string]any{"version": params["version"]}
	}
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
func (f *docCoverageCaller) DryRun() bool   { return f.dryRun }
func (f *docCoverageCaller) Fields() string { return "" }
func (f *docCoverageCaller) JQ() string     { return "" }

func docCoveragePayload(tool string) map[string]any {
	switch tool {
	case "create_document":
		return map[string]any{"data": map[string]any{"nodeId": "node-1"}}
	case "get_document_content":
		return map[string]any{"data": map[string]any{"revision": 1, "markdown": "alpha beta x body gamma legacy []", "jsonml": `["root",{},["p",{"uuid":"block-1"},"alpha beta x body gamma legacy []"]]`}}
	case "list_document_blocks":
		return map[string]any{"blocks": []any{map[string]any{"element": map[string]any{"id": "block-1", "paragraph": map[string]any{"text": "alpha beta x body gamma []"}}}}}
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

func TestCrossPlatformCoverageFetchResolvesUniqueTitleBeforeRead(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"search_documents": {{
			"documents": []any{map[string]any{"nodeId": "resolved-node", "name": "项目周报", "docType": "adoc"}},
			"hasMore":   false,
		}},
	}}
	if err := runDocCoverage(t, Fetch, caller, "--query", "项目周报"); err != nil {
		t.Fatal(err)
	}
	if len(caller.history) != 2 || caller.history[0].tool != "search_documents" || caller.history[1].tool != "get_document_content" {
		t.Fatalf("fetch resolution calls = %#v", caller.history)
	}
	if caller.history[1].params["nodeId"] != "resolved-node" {
		t.Fatalf("resolved fetch params = %#v", caller.history[1].params)
	}

	ambiguous := &docCoverageCaller{responses: map[string][]map[string]any{
		"search_documents": {{
			"documents": []any{
				map[string]any{"nodeId": "a", "name": "项目周报", "docType": "adoc"},
				map[string]any{"nodeId": "b", "name": "项目周报", "docType": "adoc"},
			},
			"hasMore": false,
		}},
	}}
	if err := runDocCoverage(t, Fetch, ambiguous, "--query", "项目周报"); err == nil {
		t.Fatal("ambiguous title unexpectedly succeeded")
	}
	if len(ambiguous.history) != 1 || ambiguous.history[0].tool != "search_documents" {
		t.Fatalf("ambiguous fetch reached content read: %#v", ambiguous.history)
	}
}

func TestCrossPlatformCoverageTemplateGoldenRouteStopsBeforeAmbiguousCreate(t *testing.T) {
	payload := map[string]any{
		"templates": []any{
			map[string]any{"templateId": "template-b", "templateName": "团队周报", "templateSource": "PUBLIC"},
			map[string]any{"templateId": "template-a", "templateName": "运营周报", "templateSource": "PUBLIC"},
		},
	}
	candidates := collectTemplateCandidates(payload)
	if len(candidates) != 2 || candidates[0]["templateId"] != "template-b" || candidates[1]["templateId"] != "template-a" {
		t.Fatalf("template candidates = %#v", candidates)
	}

	search := &docCoverageCaller{responses: map[string][]map[string]any{"search_doc_templates": {payload}}}
	if err := runDocCoverage(t, TemplateSearch, search, "--query", "周报", "--source", "PUBLIC"); err != nil {
		t.Fatal(err)
	}
	if len(search.history) != 1 || search.history[0].tool != "search_doc_templates" {
		t.Fatalf("template search calls = %#v", search.history)
	}

	ambiguous := &docCoverageCaller{responses: map[string][]map[string]any{"search_doc_templates": {payload}}}
	err := runDocCoverage(t, CreateFromTemplate, ambiguous, "--query", "周报", "--source", "PUBLIC")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "template_selection_required" || typed.ExecutionStarted == nil || *typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable {
		t.Fatalf("ambiguous template error = %#v", err)
	}
	if len(ambiguous.history) != 1 || ambiguous.history[0].tool != "search_doc_templates" {
		t.Fatalf("ambiguous template reached write: %#v", ambiguous.history)
	}
	if got, ok := typed.Details["candidates"].([]map[string]any); !ok || len(got) != 2 {
		t.Fatalf("ambiguous template details = %#v", typed.Details)
	}

	notFound := &docCoverageCaller{responses: map[string][]map[string]any{"search_doc_templates": {{"templates": []any{}}}}}
	err = runDocCoverage(t, CreateFromTemplate, notFound, "--query", "不存在")
	if !errors.As(err, &typed) || typed.Reason != "template_not_found" {
		t.Fatalf("not-found template error = %#v", err)
	}

	if selection := CreateFromTemplate.Contract.Selection; len(selection.AvoidWhen) < 2 || !strings.Contains(strings.Join(selection.AvoidWhen, " "), "+template-search") {
		t.Fatalf("create-from-template selection = %#v", selection)
	}
	mutualRoutes := []struct {
		name      string
		selection contract.SelectionSpec
		siblings  []string
	}{
		{name: "+template-list", selection: TemplateList.Contract.Selection, siblings: []string{"+template-search", "+create-from-template"}},
		{name: "+template-search", selection: TemplateSearch.Contract.Selection, siblings: []string{"+template-list", "+create-from-template"}},
		{name: "+create-from-template", selection: CreateFromTemplate.Contract.Selection, siblings: []string{"+template-list", "+template-search"}},
	}
	for _, route := range mutualRoutes {
		avoid := strings.Join(route.selection.AvoidWhen, " ")
		for _, sibling := range route.siblings {
			if !strings.Contains(avoid, sibling) {
				t.Errorf("%s Selection does not exclude sibling %s: %#v", route.name, sibling, route.selection)
			}
		}
	}
}

func TestCrossPlatformCoverageTemplateCreateProvesUniquenessAcrossPages(t *testing.T) {
	firstPage := map[string]any{
		"templates":  []any{map[string]any{"templateId": "template-a", "templateName": "周报"}},
		"hasMore":    true,
		"nextCursor": "page-2",
	}

	ambiguous := &docCoverageCaller{responses: map[string][]map[string]any{
		"search_doc_templates": {
			firstPage,
			{"templates": []any{map[string]any{"templateId": "template-b", "templateName": "周报二"}}, "hasMore": false},
		},
	}}
	err := runDocCoverage(t, CreateFromTemplate, ambiguous, "--query", "周报")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "template_selection_required" {
		t.Fatalf("cross-page ambiguity = %#v", err)
	}
	if len(ambiguous.history) != 2 || ambiguous.history[1].params["nextCursor"] != "page-2" {
		t.Fatalf("cross-page search calls = %#v", ambiguous.history)
	}

	unique := &docCoverageCaller{responses: map[string][]map[string]any{
		"search_doc_templates": {
			firstPage,
			{"templates": []any{}, "hasMore": false},
		},
	}}
	if err := runDocCoverage(t, CreateFromTemplate, unique, "--query", "周报"); err != nil {
		t.Fatal(err)
	}
	if len(unique.history) != 3 || unique.history[2].tool != "apply_doc_template" || unique.history[2].params["templateId"] != "template-a" {
		t.Fatalf("unique cross-page create calls = %#v", unique.history)
	}

	missingCursor := &docCoverageCaller{responses: map[string][]map[string]any{"search_doc_templates": {firstPage}}}
	missingCursor.responses["search_doc_templates"][0] = map[string]any{
		"templates": []any{map[string]any{"templateId": "template-a"}},
		"hasMore":   true,
	}
	err = runDocCoverage(t, CreateFromTemplate, missingCursor, "--query", "周报")
	if !errors.As(err, &typed) || typed.Reason != "template_pagination_missing_next_cursor" || len(missingCursor.history) != 1 {
		t.Fatalf("missing template cursor = %#v history=%#v", err, missingCursor.history)
	}
}

func TestCrossPlatformCoverageExportRecoveryReusesJobAndSafeDownloader(t *testing.T) {
	for _, status := range []string{"INIT", "init", " PROCESSING "} {
		if !docExportStatusPollable(status) {
			t.Errorf("export status %q should remain pollable", status)
		}
	}
	for _, status := range []string{"", "FAILED", "CANCELLED", "SUCCESS"} {
		if docExportStatusPollable(status) {
			t.Errorf("export status %q should not remain pollable", status)
		}
	}
	init := &docCoverageCaller{responses: map[string][]map[string]any{"query_export_job": {{"status": "INIT"}}}}
	err := runDocCoverage(t, Export, init, "--node", "n", "--export-format", "docx", "--output", "export.docx", "--max-polls", "1")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "doc_export_poll_timeout" {
		t.Fatalf("INIT export status = %#v, want poll timeout", err)
	}

	pollFailure := &docCoverageCaller{failAt: 2, responses: map[string][]map[string]any{}}
	err = runDocCoverage(t, Export, pollFailure, "--node", "n", "--export-format", "docx", "--output", "export.docx")
	if !errors.As(err, &typed) || typed.Reason != "doc_export_poll_failed" || typed.Details["jobId"] != "job-1" || typed.ExecutionStarted == nil || !*typed.ExecutionStarted {
		t.Fatalf("export poll error = %#v", err)
	}
	if !strings.Contains(strings.Join(typed.Actions, " "), "+export-get --job-id job-1") {
		t.Fatalf("export recovery actions = %#v", typed.Actions)
	}

	t.Chdir(t.TempDir())
	testseam.Swap(t, &docDownload, func(_ context.Context, _ string, opts localio.DownloadOptions) (localio.DownloadResult, error) {
		if opts.Output != "recovered.docx" {
			t.Fatalf("recovery output = %#v", opts)
		}
		return localio.DownloadResult{RelativePath: "recovered.docx", SizeBytes: 42}, nil
	})
	recovery := &docCoverageCaller{responses: map[string][]map[string]any{
		"query_export_job": {{"status": "SUCCESS", "downloadUrl": "https://download.dingtalk.com/recovered.docx"}},
	}}
	if err := runDocCoverage(t, ExportGet, recovery, "--job-id", "job-1", "--output", "recovered.docx"); err != nil {
		t.Fatal(err)
	}
	if len(recovery.history) != 1 || recovery.history[0].tool != "query_export_job" {
		t.Fatalf("export recovery calls = %#v", recovery.history)
	}
	if err := runDocCoverage(t, ExportGet, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--job-id", "job-1", "--output", "/tmp/unsafe.docx"); err == nil {
		t.Fatal("absolute recovery output unexpectedly accepted")
	}
}

func TestCrossPlatformCoverageCommentListPaginationFlagsAndAlias(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{}}
	if err := runDocCoverage(t, CommentList, caller, "--node", "n", "--page-size", "20", "--cursor", "next-1"); err != nil {
		t.Fatal(err)
	}
	if len(caller.history) != 1 || caller.history[0].tool != "list_comments" || caller.history[0].params["pageSize"] != 20 || caller.history[0].params["nextToken"] != "next-1" {
		t.Fatalf("comment pagination call = %#v", caller.history)
	}
}

func TestCrossPlatformCoverageMediaFailuresKeepStableIDsAndForbidPathEscape(t *testing.T) {
	if err := runDocCoverage(t, MediaInsert, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--file", "/tmp/media.bin", "--dry-run", "--yes"); err == nil {
		t.Fatal("absolute media input unexpectedly accepted")
	}
	if err := runDocCoverage(t, Import, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--file", "/tmp/report.docx", "--workspace", "workspace-1"); err == nil {
		t.Fatal("absolute import input unexpectedly accepted")
	}

	resolveFailure := &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}
	err := runDocCoverage(t, MediaDownload, resolveFailure, "--node", "node-1", "--resource-id", "resource-1", "--output", "media.bin")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "doc_media_resolve_failed" || typed.Details["nodeId"] != "node-1" || typed.Details["resourceId"] != "resource-1" {
		t.Fatalf("media recovery error = %#v", err)
	}
	if !strings.Contains(strings.Join(typed.Actions, " "), "不要 curl/wget") {
		t.Fatalf("media recovery actions = %#v", typed.Actions)
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
	err := runDocCoverage(t, Create, create, "--name", "n", "--content", `["root",{}]`, "--doc-format", "jsonml")
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

	checkpointMismatch := &docCoverageCaller{responses: map[string][]map[string]any{
		"save_doc_version":     {{"version": 8.0}},
		"get_document_content": {{"markdown": "different content"}},
	}}
	err = runDocCoverage(t, CheckpointUpdate, checkpointMismatch, "--node", "n", "--content", "body", "--yes")
	assertPartial(t, err, "doc_checkpoint_verification_failed", "verify", "n", 3)

	historyVerify := &docCoverageCaller{failAt: 3, responses: map[string][]map[string]any{}}
	err = runDocCoverage(t, VersionRevert, historyVerify, "--node", "n", "--version", "3", "--yes")
	assertPartial(t, err, "doc_history_revert_verification_failed", "verify", "n", 3)
}

func TestCrossPlatformCoverageDocUpdateAliasReachesNestedBranches(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		responses map[string][]map[string]any
		wantTools []string
	}{
		{
			name: "plain text replace",
			args: []string{"--doc", "alias-node", "--command", "str_replace", "--old", "alpha", "--new", "gamma", "--yes"},
			responses: map[string][]map[string]any{"list_document_blocks": {
				{"blocks": []any{map[string]any{"element": map[string]any{"id": "block-1", "paragraph": map[string]any{"text": "alpha beta"}}}}},
				{"blocks": []any{map[string]any{"element": map[string]any{"id": "block-1", "paragraph": map[string]any{"text": "gamma beta"}}}}},
			}},
			wantTools: []string{"list_document_blocks", "update_document_block", "list_document_blocks"},
		},
		{
			name: "block copy",
			args: []string{"--doc", "alias-node", "--command", "block_copy_insert_after", "--block-id", "block-1", "--after-block-id", "after", "--yes"},
			responses: map[string][]map[string]any{"list_document_blocks": {
				{"blocks": []any{map[string]any{"element": map[string]any{"id": "block-1", "paragraph": map[string]any{"text": "alpha"}}}}},
				{"blocks": []any{map[string]any{"element": map[string]any{"id": "id-1", "paragraph": map[string]any{"text": "alpha"}}}}},
			}},
			wantTools: []string{"list_document_blocks", "insert_document_block", "list_document_blocks"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: tc.responses}
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

func TestCrossPlatformCoverageDocWritesStopOnUnknownCommitAndRequireVerification(t *testing.T) {
	unknown := &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}
	err := runDocCoverage(t, Update, unknown, "--node", "n", "--command", "append", "--content", "x", "--yes")
	var typed *apperrors.Error
	if err == nil || !errors.As(err, &typed) || typed.Reason != "doc_write_commit_unknown" || !typed.RetryableSet || typed.Retryable {
		t.Fatalf("unknown write error = %#v", err)
	}
	if len(unknown.history) != 1 || unknown.history[0].tool != "update_document" {
		t.Fatalf("unknown write was replayed or verified: %#v", unknown.history)
	}

	verificationFailure := &docCoverageCaller{failAt: 2, responses: map[string][]map[string]any{}}
	err = runDocCoverage(t, Update, verificationFailure, "--node", "n", "--command", "append", "--content", "x", "--yes")
	if err == nil || !errors.As(err, &typed) || typed.Reason != "doc_write_verification_failed" || typed.Details["status"] != "partial_success" {
		t.Fatalf("verification failure = %#v", err)
	}
	if len(verificationFailure.history) != 2 || verificationFailure.history[1].tool != "get_document_content" {
		t.Fatalf("verification calls = %#v", verificationFailure.history)
	}
}

func TestCrossPlatformCoverageDocCreateRejectsSuccessfulMismatchedReadback(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": "truncated"}},
	}}
	err := runDocCoverage(t, Create, caller, "--name", "n", "--content", "complete body")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "doc_write_verification_failed" || typed.FailureStage != "verify" {
		t.Fatalf("create mismatch error = %#v, want verification failure", err)
	}
	if len(caller.history) != 2 || caller.history[0].tool != "create_document" || caller.history[1].tool != "get_document_content" {
		t.Fatalf("create mismatch calls = %#v, want one write followed by one read", caller.history)
	}
}

func TestCrossPlatformCoverageDocJSONMLBlockVerificationUsesJSONMLReadback(t *testing.T) {
	for _, test := range []struct {
		name     string
		command  string
		blockArg string
		response string
	}{
		{name: "insert", command: "block_insert_after", blockArg: "--after-block-id", response: `["root",{},["p",{"uuid":"ref"},"before"],["p",{"uuid":"id-1"},"after"]]`},
		{name: "replace", command: "block_replace", blockArg: "--block-id", response: `["root",{},["p",{"uuid":"target"},"after"]]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			blockID := "ref"
			if test.command == "block_replace" {
				blockID = "target"
			}
			caller := &docCoverageCaller{responses: map[string][]map[string]any{
				"list_document_blocks": {{"jsonml": test.response}},
			}}
			err := runDocCoverage(t, Update, caller,
				"--node", "n", "--command", test.command, test.blockArg, blockID,
				"--content", `["p",{},"after"]`, "--doc-format", "jsonml", "--yes")
			if err != nil {
				t.Fatal(err)
			}
			if got := caller.history[len(caller.history)-1].params["format"]; got != "jsonml" {
				t.Fatalf("verification format = %#v, want jsonml", got)
			}
		})
	}
}

func TestCrossPlatformCoverageDocVersionRevertPaginationAndVerification(t *testing.T) {
	t.Run("target on second page", func(t *testing.T) {
		caller := &docCoverageCaller{responses: map[string][]map[string]any{
			"list_doc_versions": {
				{"versions": []any{map[string]any{"version": 1.0}}, "hasMore": true, "nextCursor": "page-2"},
				{"versions": []any{map[string]any{"version": 3.0}}, "hasMore": false},
			},
		}}
		if err := runDocCoverage(t, VersionRevert, caller, "--node", "n", "--version", "3", "--yes"); err != nil {
			t.Fatal(err)
		}
		if len(caller.history) != 4 || caller.history[1].params["nextCursor"] != "page-2" {
			t.Fatalf("version pagination calls = %#v", caller.history)
		}
	})

	t.Run("empty write response and ordinary document info", func(t *testing.T) {
		caller := &docCoverageCaller{responses: map[string][]map[string]any{
			"revert_doc_version": {{}},
			"get_document_info":  {{"nodeId": "n", "revision": 99.0}},
		}}
		if err := runDocCoverage(t, VersionRevert, caller, "--node", "n", "--version", "3", "--yes"); err != nil {
			t.Fatal(err)
		}
	})

	for _, test := range []struct {
		name      string
		responses []map[string]any
		want      string
	}{
		{name: "not found", responses: []map[string]any{{"versions": []any{}, "hasMore": false}}, want: "目标版本 3 不存在"},
		{name: "missing cursor", responses: []map[string]any{{"versions": []any{}, "hasMore": true}}, want: "无法证明分页已经完整"},
		{name: "stalled cursor", responses: []map[string]any{
			{"versions": []any{}, "hasMore": true, "nextCursor": "same"},
			{"versions": []any{}, "hasMore": true, "nextCursor": "same"},
		}, want: "无法证明分页已经完整"},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{"list_doc_versions": test.responses}}
			err := runDocCoverage(t, VersionRevert, caller, "--node", "n", "--version", "3", "--yes")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("version preflight error = %v, want %q", err, test.want)
			}
			for _, call := range caller.history {
				if call.tool == "revert_doc_version" {
					t.Fatalf("preflight failure executed revert: %#v", caller.history)
				}
			}
		})
	}

	pages := make([]map[string]any, 20)
	for index := range pages {
		pages[index] = map[string]any{"versions": []any{}, "hasMore": true, "nextCursor": fmt.Sprintf("page-%d", index+1)}
	}
	caller := &docCoverageCaller{responses: map[string][]map[string]any{"list_doc_versions": pages}}
	if err := runDocCoverage(t, VersionRevert, caller, "--node", "n", "--version", "3", "--yes"); err == nil || !strings.Contains(err.Error(), "无法证明分页已经完整") {
		t.Fatalf("max-pages preflight error = %v", err)
	}

	for _, value := range []any{json.Number("3"), 3, "3"} {
		if !versionNumberMatches(value, 3) {
			t.Errorf("version value %#v did not match", value)
		}
	}
	for _, value := range []any{json.Number("bad"), 4, "bad", true} {
		if versionNumberMatches(value, 3) {
			t.Errorf("version value %#v unexpectedly matched", value)
		}
	}
	if !versionEvidenceMatches([]any{map[string]any{"target-version": "3"}}, 3, map[string]bool{"targetversion": true}) {
		t.Fatal("nested version evidence did not match")
	}
	if versionEvidenceMatches(map[string]any{"version": 4}, 3, map[string]bool{"version": true}) {
		t.Fatal("mismatched version evidence unexpectedly matched")
	}
}

func TestCrossPlatformCoverageDocWriteErrorStateMachine(t *testing.T) {
	tests := []struct {
		name       string
		cause      error
		wantState  string
		wantReason string
		wantRetry  bool
	}{
		{
			name:       "permission is terminal",
			cause:      apperrors.NewAuth("forbidden", apperrors.WithReason("http_403"), apperrors.WithRetryable(false)),
			wantState:  "failed",
			wantReason: "permission_denied",
		},
		{
			name: "confirmed not started may retry",
			cause: apperrors.NewAPI("rate limited", apperrors.WithReason("http_429"),
				apperrors.WithExecutionStarted(false), apperrors.WithRetryable(true)),
			wantState:  "retryable",
			wantReason: "http_429",
			wantRetry:  true,
		},
		{
			name:       "unknown write never replays",
			cause:      errors.New("connection reset"),
			wantState:  "unknown",
			wantReason: "doc_write_commit_unknown",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := docUnknownWriteError("doc.update", "update_document", "n", tc.cause)
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error = %#v", err)
			}
			if typed.Details["state"] != tc.wantState || typed.Reason != tc.wantReason || typed.Retryable != tc.wantRetry {
				t.Fatalf("transition = reason=%q retry=%v details=%#v", typed.Reason, typed.Retryable, typed.Details)
			}
		})
	}
}

func TestCrossPlatformCoverageDocLongWritesChunkOnceAndVerify(t *testing.T) {
	long := strings.Repeat("段落😀", 4000)
	chunks := splitDocMarkdown(long, 10000)
	if len(chunks) < 2 || strings.Join(chunks, "") != long {
		t.Fatalf("split chunks=%d roundtrip=%v", len(chunks), strings.Join(chunks, "") == long)
	}

	update := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": long}},
	}}
	if err := runDocCoverage(t, Update, update, "--node", "n", "--command", "overwrite", "--content", long, "--yes"); err != nil {
		t.Fatal(err)
	}
	if len(update.history) != len(chunks)+1 || update.history[0].params["mode"] != "overwrite" || update.history[1].params["mode"] != "append" || update.history[len(update.history)-1].tool != "get_document_content" {
		t.Fatalf("long update calls = %#v", update.history)
	}

	partial := &docCoverageCaller{failAt: 2, responses: map[string][]map[string]any{}}
	err := runDocCoverage(t, Update, partial, "--node", "n", "--command", "append", "--content", long, "--yes")
	var typed *apperrors.Error
	if err == nil || !errors.As(err, &typed) || typed.Reason != "doc_update_chunk_commit_unknown" || len(partial.history) != 2 {
		t.Fatalf("partial long update err=%#v calls=%#v", err, partial.history)
	}

	create := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": long}},
	}}
	if err := runDocCoverage(t, Create, create, "--name", "long", "--content", long); err != nil {
		t.Fatal(err)
	}
	if len(create.history) != len(chunks)+1 || create.history[0].tool != "create_document" || create.history[1].tool != "update_document" || create.history[len(create.history)-1].tool != "get_document_content" {
		t.Fatalf("long create calls = %#v", create.history)
	}
}

func TestCrossPlatformCoverageUpdateVerificationNormalizesMarkdownRoundTrip(t *testing.T) {
	expected := "# 值班表\r\n\r\n| 姓名 | 班次 |\r\n| --- | --- |\r\n| 小王 | 早班 |\r\n"
	serverMarkdown := "#  值班表\n\n|姓名|班次|\n|---|---|\n|小王|早班|\n"
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"data": map[string]any{"markdown": serverMarkdown}}},
	}}
	if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "overwrite", "--content", expected, "--yes"); err != nil {
		t.Fatalf("normalized Markdown verification failed: %v", err)
	}
	if !verifyUpdatedDocumentContent(map[string]any{"markdown": "已有内容\n" + serverMarkdown}, expected, "append", "markdown") {
		t.Fatal("normalized append verification did not find the appended content")
	}
	if verifyUpdatedDocumentContent(map[string]any{"markdown": "# 值班表\n|姓名|班次|"}, expected, "overwrite", "markdown") {
		t.Fatal("incomplete overwrite content passed normalized verification")
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
		{"update overwrite jsonml", Update, []string{"--node", "n", "--command", "overwrite", "--content", `["root",{}]`, "--doc-format", "jsonml", "--yes"}},
		{"update insert text", Update, []string{"--node", "n", "--command", "block_insert_after", "--after-block-id", "b", "--content", "x", "--yes"}},
		{"update insert jsonml", Update, []string{"--node", "n", "--command", "block_insert_after", "--after-block-id", "b", "--content", `["p",{},"x"]`, "--doc-format", "jsonml", "--yes"}},
		{"update replace text", Update, []string{"--node", "n", "--command", "block_replace", "--block-id", "b", "--content", "x", "--yes"}},
		{"update replace jsonml", Update, []string{"--node", "n", "--command", "block_replace", "--block-id", "b", "--content", `["p",{},"x"]`, "--doc-format", "jsonml", "--yes"}},
		{"update delete", Update, []string{"--node", "n", "--command", "block_delete", "--block-id", "b", "--yes"}},
		{"update replace", Update, []string{"--node", "n", "--command", "str_replace", "--old", "alpha", "--new", "gamma", "--yes"}},
		{"update copy", Update, []string{"--node", "n", "--command", "block_copy_insert_after", "--block-id", "block-1", "--after-block-id", "b", "--yes"}},
		{"update revision", Update, []string{"--node", "n", "--command", "overwrite", "--content", `["root",{}]`, "--doc-format", "jsonml", "--expected-revision", "1", "--yes"}},
		{"update dry", Update, []string{"--node", "n", "--command", "append", "--content", "x", "--dry-run", "--yes"}},
		{"checkpoint dry", CheckpointUpdate, []string{"--node", "n", "--content", "x", "--dry-run", "--yes"}},
		{"checkpoint success", CheckpointUpdate, []string{"--node", "n", "--content", "x", "--yes"}},
		{"export dry", Export, []string{"--node", "n", "--export-format", "docx", "--output", "out.docx", "--dry-run"}},
		{"export success", Export, []string{"--node", "n", "--export-format", "docx", "--output", "out.docx"}},
	}

	testseam.Swap(t, &docDownload, func(_ context.Context, _ string, _ localio.DownloadOptions) (localio.DownloadResult, error) {
		return localio.DownloadResult{RelativePath: "out.docx", SizeBytes: 7}, nil
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{}}
			switch tc.name {
			case "create markdown":
				caller.responses["get_document_content"] = []map[string]any{{"markdown": "body"}}
			case "create jsonml file":
				caller.responses["get_document_content"] = []map[string]any{{"jsonml": `["root",{},"body"]`}}
			case "create stdin":
				caller.responses["get_document_content"] = []map[string]any{{"markdown": "stdin body"}}
			}
			if tc.name == "update overwrite jsonml" || tc.name == "update revision" {
				caller.responses["get_document_content"] = []map[string]any{{"jsonml": `["root",{}]`}}
			}
			switch tc.name {
			case "update append":
				caller.responses["get_document_content"] = []map[string]any{{"markdown": "existing\nx"}}
			case "update insert text":
				caller.responses["list_document_blocks"] = []map[string]any{{"items": []any{map[string]any{"id": "b", "text": "reference"}, map[string]any{"id": "id-1", "text": "x"}}}}
			case "update insert jsonml":
				caller.responses["list_document_blocks"] = []map[string]any{{"jsonml": `["root",{},["p",{"uuid":"b"},"reference"],["p",{"uuid":"id-1"},"x"]]`}}
			case "update replace text":
				caller.responses["list_document_blocks"] = []map[string]any{{"items": []any{map[string]any{"id": "b", "text": "x"}}}}
			case "update replace jsonml":
				caller.responses["list_document_blocks"] = []map[string]any{{"jsonml": `["root",{},["p",{"uuid":"b"},"x"]]`}}
			case "update delete":
				caller.responses["list_document_blocks"] = []map[string]any{{"items": []any{map[string]any{"id": "other", "text": "x"}}}}
			case "update replace":
				caller.responses["list_document_blocks"] = []map[string]any{
					{"items": []any{map[string]any{"id": "block-1", "text": "alpha beta"}}},
					{"items": []any{map[string]any{"id": "block-1", "text": "gamma beta"}}},
				}
			case "update copy":
				caller.responses["list_document_blocks"] = []map[string]any{
					{"items": []any{map[string]any{"id": "block-1", "text": "alpha beta"}}},
					{"items": []any{map[string]any{"id": "b", "text": "reference"}, map[string]any{"id": "id-1", "text": "alpha beta"}}},
				}
			case "checkpoint success":
				caller.responses["get_document_content"] = []map[string]any{{"markdown": "existing\nx"}}
			}
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
				"+create":            {"--name", "n", "--content", `["root",{}]`, "--doc-format", "jsonml"},
				"+fetch":             {"--node", "n", "--scope", "keyword", "--keyword", "x"},
				"+inspect":           {"--node", "n", "--include-style", "--include-permissions", "--include-history", "--include-media", "--include-comments"},
				"+update":            {"--node", "n", "--command", "overwrite", "--content", `["root",{}]`, "--doc-format", "jsonml", "--expected-revision", "1", "--yes"},
				"+checkpoint-update": {"--node", "n", "--content", "x", "--yes"},
				"+export":            {"--node", "n", "--export-format", "docx", "--output", "out.docx"},
			}[command.Command]
			_ = runDocCoverage(t, command, &docCoverageCaller{failAt: failAt, responses: map[string][]map[string]any{}}, args...)
		}
	}
}

func TestCrossPlatformCoverageUpdateContractAndPreflight(t *testing.T) {
	flags := make(map[string]shortcut.Flag, len(Update.Flags))
	for _, flag := range Update.Flags {
		flags[flag.Name] = flag
	}
	if !flags["node"].Required || flags["command"].Required {
		t.Fatalf("unconditional required flags: node=%v command=%v", flags["node"].Required, flags["command"].Required)
	}
	for _, name := range []string{"content", "block-id", "after-block-id", "old", "new"} {
		if got := flags[name].RequiredWhen; got != "" {
			t.Errorf("--%s RequiredWhen = %q, want compatibility-safe custom constraint", name, got)
		}
	}
	if len(Update.Constraints) != 1 || Update.Constraints[0].Kind != shortcut.ConstraintCustom ||
		!strings.Contains(Update.Constraints[0].Description, "依 command 校验") {
		t.Fatalf("update custom constraint = %#v", Update.Constraints)
	}
	cmd := corecmd.New(shortcut.FromShortcut(Update))
	for _, alias := range []string{"doc", "text"} {
		flag := cmd.Flags().Lookup(alias)
		if flag == nil {
			t.Errorf("compatibility alias --%s is not mounted", alias)
		}
	}

	invalid := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing command", args: []string{"--node", "n"}, want: "--command"},
		{name: "insert missing reference", args: []string{"--node", "n", "--command", "block_insert_after", "--content", "x"}, want: "--after-block-id"},
		{name: "copy missing reference", args: []string{"--node", "n", "--command", "block_copy_insert_after", "--block-id", "b"}, want: "--after-block-id"},
		{name: "jsonml append", args: []string{"--node", "n", "--command", "append", "--content", `["root",{}]`, "--doc-format", "jsonml"}, want: "JSONML 当前不支持 append"},
		{name: "revision without server CAS path", args: []string{"--node", "n", "--command", "append", "--content", "x", "--expected-revision", "1"}, want: "仅支持 --command overwrite --doc-format jsonml"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{}}
			err := runDocCoverage(t, Update, caller, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if strings.Contains(err.Error(), "confirmation_required") || caller.calls != 0 {
				t.Fatalf("invalid input reached confirmation or execution: err=%v calls=%d", err, caller.calls)
			}
		})
	}

	if err := runDocCoverage(t, Update, &docCoverageCaller{responses: map[string][]map[string]any{}},
		"--node", "n", "--command", "str_replace", "--old", "x", "--new", "", "--dry-run"); err != nil {
		t.Fatalf("explicit empty --new should satisfy str_replace contract: %v", err)
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
		{Create, []string{"--name", "n", "--content", `[]`, "--doc-format", "jsonml"}},
		{Create, []string{"--name", "n", "--content", `[["p",{},"x"]]`, "--doc-format", "jsonml"}},
		{Fetch, []string{"--node", "n", "--revision", "1"}},
		{Fetch, []string{"--node", "n", "--scope", "keyword"}},
		{Update, []string{"--node", "n"}},
		{Update, []string{"--command", "append", "--content", "x", "--yes"}},
		{Update, []string{"--node", "n", "--command", "append", "--yes"}},
		{Update, []string{"--node", "n", "--command", "block_delete", "--yes"}},
		{Update, []string{"--node", "n", "--command", "str_replace", "--old", "x", "--yes"}},
		{Update, []string{"--node", "n", "--command", "append", "--content", `["root",{}]`, "--doc-format", "jsonml", "--yes"}},
	}
	for _, tc := range badCases {
		if err := runDocCoverage(t, tc.cmd, &docCoverageCaller{responses: map[string][]map[string]any{}}, tc.args...); err == nil {
			t.Errorf("%s %#v unexpectedly succeeded", tc.cmd.Command, tc.args)
		}
	}
	absoluteInputErr := runDocCoverage(t, Create, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--name", "n", "--content", "@/absolute")
	if absoluteInputErr == nil || !strings.Contains(absoluteInputErr.Error(), "暂存到工作目录") || !strings.Contains(absoluteInputErr.Error(), "stdin") {
		t.Fatalf("absolute @file guidance = %v", absoluteInputErr)
	}

	createNoNode := &docCoverageCaller{responses: map[string][]map[string]any{"create_document": {{"ok": true}}}}
	if err := runDocCoverage(t, Create, createNoNode, "--name", "n", "--content", `["root",{}]`, "--doc-format", "jsonml"); err == nil {
		t.Fatal("jsonml create without node id succeeded")
	}
	revision := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_content": {{"jsonml": `["root",{}]`}}}}
	if err := runDocCoverage(t, Update, revision, "--node", "n", "--command", "overwrite", "--content", `["root",{}]`, "--doc-format", "jsonml", "--expected-revision", "7", "--yes"); err != nil {
		t.Fatalf("server revision update failed: %v", err)
	}
	if len(revision.history) < 1 || revision.history[0].tool != "update_document" || revision.history[0].params["revision"] != 7 {
		t.Fatalf("expected revision was not forwarded atomically: %#v", revision.history)
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
	if _, err := validateJSONML(`[["p",{},"x"]]`); err == nil {
		t.Fatal("nested element-array jsonml succeeded")
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

	aliasCaller := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": "legacy"}},
	}}
	if err := runDocCoveragePath(t, Update, aliasCaller, strings.NewReader(""), "+update", "--doc", "n", "--command", "append", "--text", "legacy", "--yes"); err != nil {
		t.Fatalf("compatibility aliases --doc/--text must remain executable: %v", err)
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
		_ = runDocCoverage(t, Export, caller, "--node", "n", "--export-format", "docx", "--output", "x", "--max-polls", "1")
	}
	timeout := &docCoverageCaller{responses: map[string][]map[string]any{"query_export_job": {{"status": "PROCESSING"}}}}
	_ = runDocCoverage(t, Export, timeout, "--node", "n", "--export-format", "docx", "--output", "x", "--max-polls", "1")
	processingThenSuccess := &docCoverageCaller{responses: map[string][]map[string]any{"query_export_job": {{"status": "PROCESSING"}, {"status": "SUCCESS", "downloadUrl": "https://download.dingtalk.com/x"}}}}
	_ = runDocCoverage(t, Export, processingThenSuccess, "--node", "n", "--export-format", "docx", "--output", "x", "--max-polls", "2")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cancelledCaller := &docCoverageCaller{ctx: cancelled, responses: map[string][]map[string]any{"query_export_job": {{"status": "PROCESSING"}}}}
	_ = runDocCoverage(t, Export, cancelledCaller, "--node", "n", "--export-format", "docx", "--output", "x", "--max-polls", "2")
	_ = runDocCoverage(t, Export, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--export-format", "docx", "--output", "x", "--max-polls", "0")
	missingURL := &docCoverageCaller{responses: map[string][]map[string]any{"query_export_job": {{"status": "SUCCESS"}}}}
	_ = runDocCoverage(t, Export, missingURL, "--node", "n", "--export-format", "docx", "--output", "x")
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
		{CommentCreate, []string{"--node", "n", "--content", "x", "--block-id", "block-1", "--start", "0", "--end", "1", "--selected-text", "a", "--mention", "u", "--yes"}},
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
			"+version-revert":       {"--node", "n", "--version", "3", "--yes"},
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
	if err := runDocCoverage(t, Import, &docCoverageCaller{dryRun: true, responses: map[string][]map[string]any{}}, "--file", "media.bin", "--dry-run"); err != nil {
		t.Fatalf("import to default root failed: %v", err)
	}
	if err := runDocCoverage(t, Export, &docCoverageCaller{dryRun: true, responses: map[string][]map[string]any{}}, "--node", "n", "--output", "out.docx", "--dry-run"); err != nil {
		t.Fatalf("export default format failed: %v", err)
	}
	if selected, ok := sliceUTF16Range("A😀B", 1, 3); !ok || selected != "😀" {
		t.Fatalf("UTF-16 selection = %q/%v", selected, ok)
	}
	if _, ok := sliceUTF16Range("A😀B", 2, 3); ok {
		t.Fatal("selection splitting a surrogate pair succeeded")
	}
	for _, raw := range []string{`["550582"]`, `[550582]`} {
		mentions, err := normalizeMentionUserIDs([]string{raw})
		if err != nil || len(mentions) != 1 || mentions[0] != "550582" {
			t.Fatalf("mention normalization %q = %#v/%v", raw, mentions, err)
		}
	}
	if _, err := normalizeMentionUserIDs([]string{`[{"uid":"550582"}]`}); err == nil {
		t.Fatal("object mention unexpectedly succeeded")
	}

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

func TestCrossPlatformCoverageVersionRoutesAreCanonicalAndHistoryRoutesAreCompatible(t *testing.T) {
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "doc" {
			registered[item.Command] = item
		}
	}

	pairs := map[string]string{
		"+history-save":   "+version-save",
		"+history-list":   "+version-list",
		"+history-revert": "+version-revert",
	}
	for compatibility, canonical := range pairs {
		compatItem, ok := registered[compatibility]
		if !ok {
			t.Fatalf("missing compatibility route %s", compatibility)
		}
		canonicalItem, ok := registered[canonical]
		if !ok {
			t.Fatalf("missing canonical route %s", canonical)
		}
		if compatItem.Disposition != shortcut.DispositionAliasInternal || compatItem.PrimaryCommand != canonical {
			t.Errorf("%s routing = %s/%s, want alias_internal/%s", compatibility, compatItem.Disposition, compatItem.PrimaryCommand, canonical)
		}
		if !strings.Contains(strings.Join(compatItem.Contract.Selection.AvoidWhen, " "), canonical) {
			t.Errorf("%s compatibility Selection does not route Agent to %s", compatibility, canonical)
		}
		if canonicalItem.Contract.Selection.UseWhen[0] != canonicalItem.Intent {
			t.Errorf("%s UseWhen and Intent drifted", canonical)
		}
		if strings.Contains(strings.Join(canonicalItem.Contract.Selection.AvoidWhen, " "), "+history-") {
			t.Errorf("%s canonical Selection routes back to history compatibility commands", canonical)
		}
	}

	if VersionSave.Command != "+version-save" || VersionSave.Safety.Confirmation != "user_required" {
		t.Errorf("version-save command/confirmation = %s/%s", VersionSave.Command, VersionSave.Safety.Confirmation)
	}
	if compatHistorySave.Safety.Confirmation != "not_required" {
		t.Errorf("history-save compatibility confirmation = %s", compatHistorySave.Safety.Confirmation)
	}
	if VersionRevert.Command != "+version-revert" || VersionRevert.Execute == nil {
		t.Errorf("version-revert canonical smart route is incomplete")
	}
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
		{Export, []string{"--node", "n", "--export-format", "docx", "--output", "out.docx"}},
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
		{Export, []string{"--node", "n", "--export-format", "docx", "--output", "out.docx"}},
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
