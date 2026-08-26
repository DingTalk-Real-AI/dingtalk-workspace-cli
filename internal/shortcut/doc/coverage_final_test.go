// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageDocFinalCommonAndCanonicalBranches(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("staged.md", []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateWorkspaceInputPath("file", "staged.md"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		stub func(*testing.T)
	}{
		{"getwd", func(t *testing.T) {
			testseam.Swap(t, &docGetwd, func() (string, error) { return "", errors.New("getwd") })
		}},
		{"base symlink", func(t *testing.T) {
			testseam.Swap(t, &docEvalSymlinks, func(string) (string, error) { return "", errors.New("base") })
		}},
		{"file symlink", func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &docEvalSymlinks, func(path string) (string, error) {
				calls++
				if calls == 2 {
					return "", errors.New("file")
				}
				return path, nil
			})
		}},
		{"relative escape", func(t *testing.T) {
			testseam.Swap(t, &docRel, func(string, string) (string, error) { return "../escape", nil })
		}},
		{"relative error", func(t *testing.T) {
			testseam.Swap(t, &docRel, func(string, string) (string, error) { return "", errors.New("rel") })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.stub(t)
			if err := validateWorkspaceInputPath("file", "staged.md"); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
	if _, err := validateJSONMLNode(&cobra.Command{}, `["",{}]`); err == nil {
		t.Fatal("empty JSONML tag succeeded")
	}
	if _, err := validateJSONMLNode(&cobra.Command{}, ""); err == nil {
		t.Fatal("empty JSONML node reached shortcut validation")
	}

	validation := classifyDocWriteFailure(apperrors.NewValidation("bad"))
	if validation.reason != "invalid_input" || validation.executionStarted {
		t.Fatalf("validation transition = %#v", validation)
	}
	permission := classifyDocWriteFailure(apperrors.NewAPI("forbidden"))
	if permission.reason != "permission_denied" {
		t.Fatalf("permission transition = %#v", permission)
	}
	auth := classifyDocWriteFailure(apperrors.NewAuth("login"))
	if auth.reason != "authentication_required" {
		t.Fatalf("auth transition = %#v", auth)
	}
	notStarted := classifyDocWriteFailure(apperrors.NewAPI("later", apperrors.WithExecutionStarted(false), apperrors.WithRetryable(true)))
	if notStarted.state != docFailureRetryable || notStarted.reason != "doc_write_not_started" {
		t.Fatalf("retryable transition = %#v", notStarted)
	}
	for _, cause := range []error{apperrors.NewAPI("permission denied"), apperrors.NewAuth("login")} {
		var typed *apperrors.Error
		if err := docUnknownWriteError("doc.test", "write", "n", cause); !errors.As(err, &typed) || typed.Category != apperrors.CategoryAuth {
			t.Fatalf("auth-classified error = %#v", err)
		}
	}
	for _, existing := range []error{
		apperrors.NewValidation("existing"),
		apperrors.NewAPI("existing", apperrors.WithDetails(map[string]any{"status": "partial_success"})),
	} {
		if got := docUnknownWriteError("doc.test", "write", "n", existing); got != existing {
			t.Fatalf("existing classified error was wrapped: %p != %p", got, existing)
		}
	}

	if !containsText(nil, "") || !containsText("alpha", "ph") || containsText("alpha", "z") ||
		!containsText(map[string]any{"nested": "alpha"}, "alpha") || containsText(map[string]any{"nested": "beta"}, "alpha") ||
		!containsText([]any{"alpha"}, "alpha") || containsText([]any{"beta"}, "alpha") || containsText(3, "alpha") {
		t.Fatal("containsText branch contract failed")
	}

	jsonElement := []any{"p", map[string]any{"uuid": "b"}, "text"}
	jsonPayload := map[string]any{"jsonml": `["p",{"uuid":"b"},"text"]`}
	for _, value := range []any{jsonPayload, `["p",{"uuid":"b"},"text"]`, jsonElement} {
		if canonicalBlockContent(value, "jsonml") == "" {
			t.Fatalf("empty canonical JSONML for %#v", value)
		}
	}
	for _, value := range []any{
		map[string]any{"text": "alpha"},
		map[string]any{"id": "ignored", "nested": map[string]any{"text": "beta"}},
		[]any{"p", map[string]any{"text": "gamma"}},
		[]any{map[string]any{"text": "delta"}},
		"epsilon",
	} {
		if canonicalBlockContent(value, "markdown") == "" {
			t.Fatalf("empty canonical markdown for %#v", value)
		}
	}

	jsonTree := map[string]any{
		"jsonml":  `["root",{},["p",{"uuid":"ref"},"before"],["p",{"uuid":"new"},"after"]]`,
		"content": "not-json",
		"nested":  []any{"p", map[string]any{"uuid": "direct"}, "direct"},
	}
	if len(orderedCanonicalBlocks(jsonTree, "jsonml")) < 3 || findJSONMLBlock(jsonTree, "new") == nil {
		t.Fatalf("JSONML traversal failed: %#v", orderedJSONMLBlocks(jsonTree))
	}
	if findJSONMLBlock(jsonTree, "missing") != nil || jsonMLBlockIdentity([]any{"p"}) != "" || jsonMLBlockIdentity([]any{"p", "attrs"}) != "" {
		t.Fatal("JSONML identity negative contract failed")
	}
	if canonicalBlockIdentity(jsonElement, "jsonml") != "b" || canonicalBlockIdentity("bad", "jsonml") != "" {
		t.Fatal("JSONML canonical identity failed")
	}

	elementTree := map[string]any{"wrapper": []any{map[string]any{"id": "ref", "text": "before"}, map[string]any{"id": "new", "text": "after"}}}
	if len(orderedCanonicalBlocks(elementTree, "element")) != 2 || canonicalBlockIdentity(elementTree, "element") != "" || canonicalBlockIdentity("bad", "element") != "" {
		t.Fatal("element canonical traversal failed")
	}
	if !verifyInsertedCanonicalBlock(map[string]any{"blockId": "new"}, elementTree, "ref", "after", "after", "element", 0) ||
		!verifyInsertedCanonicalBlock(map[string]any{}, elementTree, "ref", "after", "after", "element", 0) ||
		verifyInsertedCanonicalBlock(map[string]any{}, elementTree, "new", "after", "after", "element", 0) {
		t.Fatal("inserted block verification branch contract failed")
	}
	if verifyInsertedCanonicalBlock(map[string]any{"blockId": "other"}, elementTree, "ref", "after", "after", "element", 0) {
		t.Fatal("inserted block with mismatched result ID verified")
	}
	if verifyInsertedCanonicalBlock(map[string]any{"blockId": "new"}, elementTree, "ref", "after", "wrong", "element", 0) {
		t.Fatal("inserted block with mismatched content verified")
	}
	if !verifyInsertedCanonicalBlockContent(map[string]any{}, elementTree, "ref", "after", "element") {
		t.Fatal("copy insertion positional fallback failed")
	}
	for _, tc := range []struct {
		name  string
		value any
		want  int
	}{
		{name: "non map", value: "heading", want: 0},
		{name: "element wrapper", value: map[string]any{"element": map[string]any{"blockType": "heading", "heading": map[string]any{"level": "1"}}}, want: 1},
		{name: "non heading", value: map[string]any{"blockType": "paragraph", "heading": map[string]any{"level": "1"}}, want: 0},
		{name: "missing heading", value: map[string]any{"blockType": "heading"}, want: 0},
		{name: "integer", value: map[string]any{"heading": map[string]any{"level": 1}}, want: 1},
		{name: "integral float", value: map[string]any{"heading": map[string]any{"level": float64(2)}}, want: 2},
		{name: "fractional float", value: map[string]any{"heading": map[string]any{"level": 2.5}}, want: 0},
		{name: "valid json number", value: map[string]any{"heading": map[string]any{"level": json.Number("3")}}, want: 3},
		{name: "invalid json number", value: map[string]any{"heading": map[string]any{"level": json.Number("bad")}}, want: 0},
		{name: "invalid string", value: map[string]any{"heading": map[string]any{"level": "bad"}}, want: 0},
		{name: "unsupported level", value: map[string]any{"heading": map[string]any{"level": true}}, want: 0},
	} {
		t.Run("heading level "+tc.name, func(t *testing.T) {
			if got := canonicalHeadingLevel(tc.value); got != tc.want {
				t.Fatalf("canonicalHeadingLevel(%#v) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
	if blockContentEquals(map[string]any{}, "missing", "after", "element") {
		t.Fatal("missing block matched")
	}
	if blockContentEquals(nil, "missing", "after", "element") {
		t.Fatal("nil block tree matched")
	}
	if blockContentEquals(map[string]any{}, "missing", "after", "jsonml") {
		t.Fatal("missing JSONML block matched")
	}
	if got := documentContentCandidates("root", "markdown"); len(got) != 1 || got[0] != "root" {
		t.Fatalf("root content candidates = %#v", got)
	}
	if got := documentContentCandidates([]any{map[string]any{"content": "nested"}}, "markdown"); len(got) != 1 || got[0] != "nested" {
		t.Fatalf("nested content candidates = %#v", got)
	}
	// splitDocMarkdown was replaced by the shared splitter. Its two load-bearing
	// contracts are kept: a non-positive limit disables splitting entirely...
	if got := helpers.SplitMarkdownForAppend("a\nb", 0); len(got.Chunks) != 1 {
		t.Fatalf("disabled split = %#v", got.Chunks)
	}
	// ...and a long single paragraph still splits at the line boundary. What
	// changed deliberately: the boundary newline no longer trails the preceding
	// chunk (it used to be "ab\n"), because a chunk is now a self-contained block
	// sequence rather than a raw byte range, and the paragraph break is reported.
	plan := helpers.SplitMarkdownForAppend("ab\ncd", 4)
	if len(plan.Chunks) != 2 || plan.Chunks[0] != "ab" || plan.Chunks[1] != "cd" {
		t.Fatalf("newline split = %#v", plan.Chunks)
	}
	if len(plan.Degradations) != 1 || plan.Degradations[0].Kind != "paragraph_split" {
		t.Fatalf("newline split degradations = %#v", plan.Degradations)
	}
}

func TestCrossPlatformCoverageDocFinalExecutionFailureBranches(t *testing.T) {
	// Must exceed the production chunk limit to reach the chunk-append branch;
	// tie it to the constant so a limit bump cannot silently drop that coverage.
	longContent := strings.Repeat("x", helpers.DefaultMarkdownChunkRunes+1)
	if err := runDocCoverage(t, Create, &docCoverageCaller{failAt: 2, responses: map[string][]map[string]any{}}, "--name", "n", "--content", longContent); err == nil {
		t.Fatal("partial chunk create succeeded")
	}
	for _, failAt := range []int{1, 2} {
		err := runDocCoverage(t, Update, &docCoverageCaller{failAt: failAt, responses: map[string][]map[string]any{}},
			"--node", "n", "--command", "block_insert_after", "--after-block-id", "block-1", "--content", "x", "--yes")
		if err == nil {
			t.Fatalf("block mutation failAt=%d succeeded", failAt)
		}
	}
	mismatch := &docCoverageCaller{responses: map[string][]map[string]any{
		"insert_document_block": {{"blockId": "new"}},
		"list_document_blocks":  {{"blocks": []any{map[string]any{"id": "new", "text": "wrong"}}}},
	}}
	if err := runDocCoverage(t, Update, mismatch, "--node", "n", "--command", "block_insert_after", "--after-block-id", "block-1", "--content", "x", "--yes"); err == nil {
		t.Fatal("mismatched block verification succeeded")
	}

	for _, tc := range []struct {
		name      string
		responses map[string][]map[string]any
		failAt    int
		args      []string
	}{
		{"query error", map[string][]map[string]any{}, 1, []string{"--job-id", "j"}},
		{"unknown", map[string][]map[string]any{"query_export_job": {{}}}, 0, []string{"--job-id", "j"}},
		{"failed", map[string][]map[string]any{"query_export_job": {{"status": "CANCELLED"}}}, 0, []string{"--job-id", "j"}},
		{"missing url", map[string][]map[string]any{"query_export_job": {{"status": "SUCCESS"}}}, 0, []string{"--job-id", "j", "--output", "out"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = runDocCoverage(t, ExportGet, &docCoverageCaller{failAt: tc.failAt, responses: tc.responses}, tc.args...)
		})
	}
	t.Run("export getwd", func(t *testing.T) {
		testseam.Swap(t, &docGetwd, func() (string, error) { return "", errors.New("getwd") })
		_ = runDocCoverage(t, ExportGet, &docCoverageCaller{responses: map[string][]map[string]any{"query_export_job": {{"status": "SUCCESS", "downloadUrl": "https://example.com/x"}}}}, "--job-id", "j", "--output", "out")
	})
	t.Run("export download", func(t *testing.T) {
		testseam.Swap(t, &docDownload, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
			return localio.DownloadResult{}, errors.New("download")
		})
		_ = runDocCoverage(t, ExportGet, &docCoverageCaller{responses: map[string][]map[string]any{"query_export_job": {{"status": "SUCCESS", "downloadUrl": "https://example.com/x"}}}}, "--job-id", "j", "--output", "out")
	})

	searchCaller := &docCoverageCaller{responses: map[string][]map[string]any{"search_documents": {{"documents": []any{}, "hasMore": false}}}}
	if err := runDocCoverage(t, Search, searchCaller, "--query", "q", "--creator-uids", "u", "--editor-uids", "e", "--mentioned-uids", "m", "--workspace-ids", "w"); err != nil {
		t.Fatal(err)
	}
	if got := searchCaller.history[0].params["pageSize"]; got != 10 {
		t.Fatalf("default search page size = %#v", got)
	}
	listCaller := &docCoverageCaller{responses: map[string][]map[string]any{"list_nodes": {{"nodes": []any{}, "hasMore": false}}}}
	if err := runDocCoverage(t, List, listCaller, "--folder", "f", "--workspace", "w"); err != nil {
		t.Fatal(err)
	}
	if got := listCaller.history[0].params["pageSize"]; got != 50 {
		t.Fatalf("default list page size = %#v", got)
	}

	_ = runDocCoverage(t, CommentReply, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--comment-key", "c", "--content", "x", "--emoji", "--mention", "[bad", "--yes")
	if err := runDocCoverage(t, CommentReply, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--comment-key", "c", "--content", "x", "--mention", "u", "--yes"); err != nil {
		t.Fatal(err)
	}
	_ = runDocCoverage(t, CommentUpdate, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--comment-key", "c", "--content", "x", "--mention", "[bad")
	_ = runDocCoverage(t, ExportSubmit, &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}, "--node", "n", "--export-format", "docx")
	if err := runDocCoverage(t, ExportSubmit, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--export-format", "docx"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		responses map[string][]map[string]any
		failAt    int
		args      []string
	}{
		{"template search error", map[string][]map[string]any{}, 1, []string{"--query", "q"}},
		{"template not found", map[string][]map[string]any{"search_doc_templates": {{"templates": []any{}, "hasMore": false}}}, 0, []string{"--query", "q"}},
		{"template resolved", map[string][]map[string]any{"search_doc_templates": {{"templates": []any{map[string]any{"templateId": "t"}}, "hasMore": false}}}, 0, []string{"--query", "q", "--source", "PUBLIC"}},
		{"template continue", map[string][]map[string]any{"search_doc_templates": {{"templates": []any{map[string]any{"templateId": "t"}}, "nextCursor": "p2"}}}, 0, []string{"--query", "q", "--cursor", "p1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = runDocCoverage(t, TemplateSearch, &docCoverageCaller{responses: tc.responses, failAt: tc.failAt}, tc.args...)
		})
	}

	for _, raw := range [][]string{{"[bad"}, {"", " "}} {
		if _, err := normalizeMentionUserIDs(raw); err == nil {
			t.Fatalf("invalid mentions %#v succeeded", raw)
		}
	}
	if _, ok := sliceUTF16Range("A😀B", 1, 2); ok {
		t.Fatal("selection ending inside surrogate pair succeeded")
	}
	if _, ok := sliceUTF16Range("abc", -1, 1); ok {
		t.Fatal("negative selection range succeeded")
	}

	commentCases := []struct {
		name      string
		caller    *docCoverageCaller
		args      []string
		wantError bool
	}{
		{"block read", &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}, []string{"--node", "n", "--content", "c", "--block-id", "b", "--start", "0", "--end", "1", "--yes"}, true},
		{"range", &docCoverageCaller{responses: map[string][]map[string]any{"list_document_blocks": {{"id": "b", "text": ""}}}}, []string{"--node", "n", "--content", "c", "--block-id", "b", "--start", "0", "--end", "1", "--yes"}, true},
		{"selected mismatch", &docCoverageCaller{responses: map[string][]map[string]any{"list_document_blocks": {{"id": "b", "text": "alpha"}}}}, []string{"--node", "n", "--content", "c", "--block-id", "b", "--start", "0", "--end", "1", "--selected-text", "z", "--yes"}, true},
		{"write error", &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}, []string{"--node", "n", "--content", "c", "--yes"}, true},
		{"mention", &docCoverageCaller{responses: map[string][]map[string]any{}}, []string{"--node", "n", "--content", "c", "--mention", "[bad", "--yes"}, true},
	}
	for _, tc := range commentCases {
		t.Run(tc.name, func(t *testing.T) {
			err := runDocCoverage(t, CommentCreate, tc.caller, tc.args...)
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v wantError=%v", err, tc.wantError)
			}
		})
	}

	if !versionEvidenceMatches([]any{map[string]any{"target_version": 3}}, 3, map[string]bool{"targetversion": true}) {
		t.Fatal("array version evidence did not match")
	}
	if !versionEvidenceMatches(map[string]any{"wrapper": map[string]any{"target_version": 3}}, 3, map[string]bool{"targetversion": true}) {
		t.Fatal("nested version evidence did not match")
	}
	withDescription := collectTemplateCandidates(map[string]any{"templateId": "t", "description": "d"})
	if len(withDescription) != 1 || withDescription[0]["description"] != "d" {
		t.Fatalf("template description = %#v", withDescription)
	}

	duplicateTemplates := &docCoverageCaller{responses: map[string][]map[string]any{"search_doc_templates": {
		{"templates": []any{map[string]any{"templateId": "t"}}, "hasMore": true, "nextCursor": "p2"},
		{"templates": []any{map[string]any{"templateId": "t"}}, "hasMore": false},
	}}}
	_ = runDocCoverage(t, CreateFromTemplate, duplicateTemplates, "--query", "q")
	stalledTemplates := &docCoverageCaller{responses: map[string][]map[string]any{"search_doc_templates": {
		{"templates": []any{map[string]any{"templateId": "t"}}, "hasMore": true, "nextCursor": "p2"},
		{"templates": []any{map[string]any{"templateId": "t"}}, "hasMore": true, "nextCursor": "p2"},
	}}}
	if err := runDocCoverage(t, CreateFromTemplate, stalledTemplates, "--query", "q"); err == nil {
		t.Fatal("stalled template cursor succeeded")
	}
	maxPages := make([]map[string]any, 20)
	for index := range maxPages {
		maxPages[index] = map[string]any{"templates": []any{map[string]any{"templateId": "t"}}, "hasMore": true, "nextCursor": "p" + strings.Repeat("x", index+1)}
	}
	if err := runDocCoverage(t, CreateFromTemplate, &docCoverageCaller{responses: map[string][]map[string]any{"search_doc_templates": maxPages}}, "--query", "q"); err == nil {
		t.Fatal("template max-pages succeeded")
	}

	t.Chdir(t.TempDir())
	if err := os.WriteFile("media.bin", []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = runDocCoverage(t, ResourceUpdate, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--file", "media.bin", "--dry-run", "--yes")
	if err := runDocCoverage(t, Import, &docCoverageCaller{dryRun: true, responses: map[string][]map[string]any{}}, "--file", "media.bin", "--folder", "f", "--dry-run"); err != nil {
		t.Fatal(err)
	}

	t.Run("empty preview", func(t *testing.T) {
		testseam.Swap(t, &docDownload, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
			return localio.DownloadResult{}, nil
		})
		_ = runDocCoverage(t, MediaPreview, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--resource-id", "r")
	})
	t.Run("empty download", func(t *testing.T) {
		testseam.Swap(t, &docDownload, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
			return localio.DownloadResult{}, nil
		})
		_ = runDocCoverage(t, MediaDownload, &docCoverageCaller{responses: map[string][]map[string]any{}}, "--node", "n", "--resource-id", "r", "--output", "out")
	})
}
