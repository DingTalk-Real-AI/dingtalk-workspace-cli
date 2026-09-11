// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDocFolderSearchRetainsMetadataAndRejectsUnknownMembership(t *testing.T) {
	raw := map[string]any{"documents": []any{true, map[string]any{"name": "no id"}, map[string]any{"nodeId": "inside", "title": "known", "extra": "kept"}, map[string]any{"nodeId": "outside"}}, "hasMore": false}
	rows := searchDocsProjectWithMetadata(raw)
	if rows[1]["metadata"].(map[string]any)["extra"] != "kept" {
		t.Fatal("metadata lost")
	}
	for _, tc := range []struct {
		name           string
		source, folder map[string]any
		want           bool
		fail           int
	}{
		{"inside", map[string]any{"documents": []any{map[string]any{"nodeId": "inside"}, map[string]any{"nodeId": "outside"}}, "hasMore": false}, map[string]any{"nodes": []any{map[string]any{"nodeId": "inside"}}, "hasMore": false}, true, 0},
		{"unknown-source", raw, map[string]any{"nodes": []any{map[string]any{"nodeId": "inside"}}, "hasMore": false}, false, 0},
		{"unknown-folder", map[string]any{"documents": []any{}, "hasMore": false}, map[string]any{"nodes": []any{map[string]any{"name": "missing ID"}}, "hasMore": false}, false, 0},
		{"folder-limit", map[string]any{"documents": []any{}, "hasMore": false}, map[string]any{"nodes": []any{map[string]any{"nodeId": "inside"}}, "hasMore": true, "nextPageToken": "next"}, false, 0},
		{"folder-error", map[string]any{"documents": []any{}, "hasMore": false}, nil, false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &docCoverageCaller{failAt: tc.fail, responses: map[string][]map[string]any{"search_documents": {tc.source}, "list_nodes": {tc.folder}}}
			err := runDocCoverage(t, Search, c, "--query", "known", "--folder", "folder", "--page-all", "--max-pages", "1", "--with-metadata")
			if (err == nil) != tc.want {
				t.Fatal(err)
			}
		})
	}
	s := Search
	s.Execute = func(rt *shortcut.RuntimeContext) error {
		_, err := filterDocSearchFolder(rt, map[string]any{"complete": false})
		return err
	}
	if err := runDocCoverage(t, s, &docCoverageCaller{}, "--query", "q"); err == nil {
		t.Fatal("partial candidates accepted")
	}
}

func parityFullJSONML(ids, texts []string) map[string]any {
	root := []any{"root", map[string]any{}}
	for i, id := range ids {
		root = append(root, []any{"p", map[string]any{"uuid": id}, texts[i]})
	}
	encoded, _ := json.Marshal(root)
	return map[string]any{"jsonml": string(encoded)}
}
func parityBlockRead(ids, texts []string) map[string]any {
	blocks := []any{}
	for i, id := range ids {
		encoded, _ := json.Marshal([]any{"p", map[string]any{"uuid": id}, texts[i]})
		blocks = append(blocks, map[string]any{"blockId": id, "index": i, "jsonml": string(encoded), "element": map[string]any{"id": id, "blockType": "paragraph", "paragraph": map[string]any{"text": texts[i]}}})
	}
	return map[string]any{"blocks": blocks, "hasMore": false}
}
func TestCrossPlatformCoverageDocRangeReplaceDeleteAndPartialRecovery(t *testing.T) {
	for _, format := range []string{"markdown", "jsonml"} {
		for _, count := range []int{1, 2} {
			content := "new"
			if format == "jsonml" {
				content = `["p",{},"new"]`
			}
			end := "a"
			if count == 2 {
				end = "b"
			}
			c := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_content": {parityFullJSONML([]string{"a", "b", "keep"}, []string{"old", "old2", "outside"})}, "list_document_blocks": {parityBlockRead([]string{"a", "b", "keep"}, []string{"new", "old2", "outside"}), parityBlockRead([]string{"a", "keep"}, []string{"new", "outside"})}}}
			if err := runDocCoverage(t, Update, c, "--node", "n", "--command", "block_replace", "--start-block-id", "a", "--end-block-id", end, "--doc-format", format, "--content", content, "--yes"); err != nil {
				t.Fatal(format, count, err)
			}
		}
	}
	for _, failure := range []int{1, 2, 4} {
		c := &docCoverageCaller{failAt: failure, responses: map[string][]map[string]any{"get_document_content": {parityFullJSONML([]string{"a", "b"}, []string{"old", "old2"})}, "list_document_blocks": {parityBlockRead([]string{"a", "b"}, []string{"new", "old2"})}}}
		if err := runDocCoverage(t, Update, c, "--node", "n", "--command", "block_replace", "--start-block-id", "a", "--end-block-id", "b", "--content", "new", "--yes"); err == nil {
			t.Fatal("partial write reported success")
		}
	}
	for _, command := range []string{"block_replace", "block_delete"} {
		args := []string{"--node", "n", "--command", command, "--start-block-id", "a", "--end-block-id", "b", "--content", "new"}
		if err := runDocCoverage(t, Update, &docCoverageCaller{dryRun: true}, append(args, "--dry-run")...); err != nil {
			t.Fatal(err)
		}
		c := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_content": {parityFullJSONML([]string{"a"}, []string{"old"})}}}
		if err := runDocCoverage(t, Update, c, append(args, "--yes")...); err == nil || c.calls != 1 {
			t.Fatal("missing range must stop before write", err, c.calls)
		}
	}
	c := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_content": {parityFullJSONML([]string{"a", "b", "keep"}, []string{"old", "old2", "outside"})}, "list_document_blocks": {parityBlockRead([]string{"a", "keep"}, []string{"old", "outside"}), parityBlockRead([]string{"keep"}, []string{"outside"})}}}
	if err := runDocCoverage(t, Update, c, "--node", "n", "--command", "block_delete", "--start-block-id", "a", "--end-block-id", "b", "--yes"); err != nil {
		t.Fatal(err)
	}
	c = &docCoverageCaller{failAt: 1}
	if err := runDocCoverage(t, Update, c, "--node", "n", "--command", "block_delete", "--start-block-id", "a", "--end-block-id", "b", "--yes"); err == nil {
		t.Fatal("read error accepted")
	}
	ids, texts := []string{}, []string{}
	for i := 0; i < 51; i++ {
		ids = append(ids, strings.Repeat("a", i+1))
		texts = append(texts, "body")
	}
	if _, err := docRangeIDs(parityFullJSONML(ids, texts), "0", "-1"); err == nil {
		t.Fatal("unbounded range")
	}
}

func TestCrossPlatformCoverageDocMultiCopyPreflightOrderAndFailureDetails(t *testing.T) {
	args := []string{"--node", "n", "--command", "block_copy_insert_after", "--block-id", "a,b", "--after-block-id", "ref", "--yes"}
	c := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content":  {parityFullJSONML([]string{"a", "b", "ref"}, []string{"A", "B", "R"})},
		"insert_document_block": {{"blockId": "new1"}, {"elementId": "new2"}},
		"list_document_blocks":  {parityBlockRead([]string{"a", "b", "ref", "new1"}, []string{"A", "B", "R", "A"}), parityBlockRead([]string{"a", "b", "ref", "new1", "new2"}, []string{"A", "B", "R", "A", "B"})},
	}}
	result := runDocCoverageEnvelope(t, Update, c, args...)
	encoded, _ := json.Marshal(result)
	if !bytes.Contains(encoded, []byte("new2")) {
		t.Fatal("copy result lacks receipts")
	}
	for _, tc := range []struct {
		data map[string]any
		fail int
	}{
		{nil, 1}, {map[string]any{}, 0}, {parityFullJSONML([]string{"a", "b"}, []string{"A", "B"}), 0},
		{parityFullJSONML([]string{"a", "ref"}, []string{"A", "R"}), 0},
		{map[string]any{"jsonml": `["root",{},["p",{"uuid":"a"},["img",{"src":"https://example.com/x"}]],["p",{"uuid":"b"}],["p",{"uuid":"ref"}]]`}, 0},
		{parityFullJSONML([]string{"a", "b", "ref"}, []string{"A", "B", "R"}), 2},
	} {
		c := &docCoverageCaller{failAt: tc.fail, responses: map[string][]map[string]any{"get_document_content": {tc.data}}}
		if err := runDocCoverage(t, Update, c, args...); err == nil {
			t.Fatal("invalid source/failed write succeeded")
		}
	}
	s := Update
	s.Execute = func(rt *shortcut.RuntimeContext) error { return executeDocMultiCopy(rt, "n") }
	if err := runDocCoverage(t, s, &docCoverageCaller{}, "--node", "n", "--command", "block_copy_insert_after", "--block-id", "a,a", "--after-block-id", "ref", "--yes"); err == nil {
		t.Fatal("duplicates")
	}
}

func TestCrossPlatformCoverageDocMediaUploadIntegrityFailuresAndNoMutation(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("asset.txt", []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"missing", "."} {
		if _, err := hashDocFile(path); err == nil {
			t.Fatal("hash must fail", path)
		}
	}
	helpers.SwapHTTPPutFileForTest(t, func(context.Context, string, map[string]string, string, int64) error { return nil })
	for _, tc := range []struct {
		name               string
		fail               int
		resource, download string
		dirFail            bool
		want               bool
	}{
		{"match", 0, "12345678-1234-1234-1234-123456789012", "original", false, true},
		{"invalid-id", 0, "bad", "original", false, false},
		{"read-failed", 2, "12345678-1234-1234-1234-123456789012", "original", false, false},
		{"directory-failed", 0, "12345678-1234-1234-1234-123456789012", "original", true, false},
		{"download-failed", 0, "12345678-1234-1234-1234-123456789012", "error", false, false},
		{"hash-failed", 0, "12345678-1234-1234-1234-123456789012", "missing", false, false},
		{"wrong-bytes", 0, "12345678-1234-1234-1234-123456789012", "different", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := output.WithResultStore(context.Background())
			c := &docCoverageCaller{ctx: ctx, failAt: tc.fail, responses: map[string][]map[string]any{"get_doc_attachment_upload_info": {{"uploadUrl": "https://example.com/upload", "resourceId": tc.resource, "resourceUrl": "/resources/known"}}, "download_doc_attachment": {{"downloadUrl": "https://example.com/file.txt"}}}}
			if tc.dirFail {
				testseam.Swap(t, &docMkdirTemp, func(string, string) (string, error) { return "", errors.New("disk full") })
			}
			testseam.Swap(t, &docDownload, func(_ context.Context, _ string, opts localio.DownloadOptions) (localio.DownloadResult, error) {
				if tc.download == "error" {
					return localio.DownloadResult{}, errors.New("interrupted")
				}
				path := filepath.Join(opts.BaseDir, "asset.txt")
				if tc.download != "missing" {
					if err := os.WriteFile(path, []byte(tc.download), 0600); err != nil {
						return localio.DownloadResult{}, err
					}
				}
				return localio.DownloadResult{AbsolutePath: path, RelativePath: "asset.txt", SizeBytes: 8}, nil
			})
			err := runDocCoverage(t, MediaUpload, c, "--node", "n", "--file", "asset.txt", "--yes")
			if (err == nil) != tc.want {
				t.Fatal(err)
			}
			for _, call := range c.history {
				if call.tool == "insert_document_block" {
					t.Fatal("standalone upload inserted content")
				}
			}
		})
	}
	ctx, _ := output.WithResultStore(context.Background())
	if err := runDocCoverage(t, MediaUpload, &docCoverageCaller{ctx: ctx, dryRun: true}, "--node", "n", "--file", "asset.txt", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	s := MediaUpload
	s.Validate = nil
	s.Constraints = nil
	if err := runDocCoverage(t, s, &docCoverageCaller{}, "--node", "n", "--file", "missing", "--yes"); err == nil {
		t.Fatal("missing source")
	}
}

func TestCrossPlatformCoverageDocCreateWithMediaAndPartialProgress(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("asset.txt", []byte("body"), 0600); err != nil {
		t.Fatal(err)
	}
	helpers.SwapHTTPPutFileForTest(t, func(context.Context, string, map[string]string, string, int64) error { return nil })
	for _, fail := range []int{0, 3} {
		c := &docCoverageCaller{failAt: fail, responses: map[string][]map[string]any{
			"get_document_content":           {{"markdown": "body"}},
			"get_doc_attachment_upload_info": {{"uploadUrl": "https://example.com/upload", "resourceId": "12345678-1234-1234-1234-123456789012", "resourceUrl": "/resources/known"}},
			"insert_document_block":          {{"blockId": "media"}},
			"list_document_blocks":           {{"blocks": []any{map[string]any{"blockId": "media", "jsonml": `["card",{"uuid":"media","resourceId":"12345678-1234-1234-1234-123456789012"}]`}}, "hasMore": false}},
		}}
		err := runDocCoverage(t, Create, c, "--content", "body", "--media-files", "asset.txt", "--yes")
		if (err == nil) != (fail == 0) {
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				t.Logf("cause: %v", cause)
			}
			t.Fatalf("history: %#v", c.history)
		}
	}
	if err := runDocCoverage(t, Create, &docCoverageCaller{}, "--content", "body", "--media-files", strings.Repeat("x,", 21)); err == nil {
		t.Fatal("media bound")
	}
	testseam.Swap(t, &docRel, func(base, path string) (string, error) {
		rel, err := filepath.Rel(base, path)
		_ = os.Remove(path)
		return rel, err
	})
	c := &docCoverageCaller{}
	if err := runDocCoverage(t, Create, c, "--content", "body", "--media-files", "asset.txt"); err == nil || c.calls != 0 {
		t.Fatal("source vanished before creation", err)
	}
}

func TestCrossPlatformCoverageDocRangeReplaceWaitsForWholeSelectedEffect(t *testing.T) {
	c := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {parityFullJSONML([]string{"a", "b"}, []string{"old", "old2"})},
		"list_document_blocks": {parityBlockRead([]string{"a", "b"}, []string{"new", "old2"}), parityBlockRead([]string{"a", "b"}, []string{"wrong", "old2"}), parityBlockRead([]string{"a", "b"}, []string{"new", "old2"}), parityBlockRead([]string{"a"}, []string{"new"})},
	}}
	if err := runDocCoverage(t, Update, c, "--node", "n", "--command", "block_replace", "--start-block-id", "a", "--end-block-id", "b", "--content", "new", "--yes"); err != nil {
		t.Fatal(err)
	}
	c = &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content":  {parityFullJSONML([]string{"a", "b", "ref"}, []string{"A", "B", "R"})},
		"insert_document_block": {{}},
		"list_document_blocks":  {parityBlockRead([]string{"a", "b", "ref", "new1"}, []string{"A", "B", "R", "A"})},
	}}
	if err := runDocCoverage(t, Update, c, "--node", "n", "--command", "block_copy_insert_after", "--block-id", "a,b", "--after-block-id", "ref", "--yes"); err == nil || !strings.Contains(err.Error(), "缺少新块ID") {
		t.Fatal("missing receipt cannot continue copy", err)
	}
	if _, err := docRangeIDs(map[string]any{}, "a", "b"); err == nil {
		t.Fatal("unproven range")
	}
}

func TestCrossPlatformCoverageDocMediaPreviewPersistentOutputAndValidation(t *testing.T) {
	t.Chdir(t.TempDir())
	c := &docCoverageCaller{}
	if err := runDocCoverage(t, MediaPreview, c, "--node", "n", "--resource-id", "bad"); err == nil || c.calls != 0 {
		t.Fatal("invalid resource ID")
	}
	if err := runDocCoverage(t, MediaPreview, c, "--node", "n", "--resource-id", "12345678-1234-1234-1234-123456789012", "--output", "../bad"); err == nil || c.calls != 0 {
		t.Fatal("escaping output")
	}
	c = &docCoverageCaller{responses: map[string][]map[string]any{"download_doc_attachment": {{"downloadUrl": "https://example.com/file.txt"}}}}
	testseam.Swap(t, &docDownload, func(_ context.Context, _ string, opts localio.DownloadOptions) (localio.DownloadResult, error) {
		if opts.Overwrite {
			t.Fatal("legacy preview must not overwrite")
		}
		return localio.DownloadResult{AbsolutePath: filepath.Join(opts.BaseDir, "output.txt"), RelativePath: "output.txt", SizeBytes: 4}, nil
	})
	if err := runDocCoverage(t, MediaPreview, c, "--node", "n", "--resource-id", "12345678-1234-1234-1234-123456789012", "--output", "output.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageDocAttachmentViewOptionsAndReadback(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("asset.txt", []byte("body"), 0600); err != nil {
		t.Fatal(err)
	}
	helpers.SwapHTTPPutFileForTest(t, func(context.Context, string, map[string]string, string, int64) error { return nil })
	for _, tc := range []struct{ input, persisted string }{{"summary", "wideCard"}, {"preview", "preview"}} {
		encoded, _ := json.Marshal([]any{"card", map[string]any{"uuid": "media", "viewType": tc.persisted, "resourceId": "12345678-1234-1234-1234-123456789012"}})
		c := &docCoverageCaller{responses: map[string][]map[string]any{
			"get_doc_attachment_upload_info": {{"uploadUrl": "https://example.com/upload", "resourceId": "12345678-1234-1234-1234-123456789012", "resourceUrl": "/resources/known"}},
			"insert_document_block":          {{"blockId": "media"}}, "list_document_blocks": {{"blocks": []any{map[string]any{"blockId": "media", "jsonml": string(encoded)}}, "hasMore": false}},
		}}
		if err := runDocCoverage(t, MediaInsert, c, "--node", "n", "--file", "asset.txt", "--file-view", tc.input, "--yes"); err != nil {
			t.Fatal(err)
		}
		for _, call := range c.history {
			if call.tool == "insert_document_block" {
				element := call.params["element"].(map[string]any)
				if element["attachment"].(map[string]any)["viewType"] != tc.input {
					t.Fatal("wrong view sent")
				}
			}
		}
	}
	s := MediaInsert
	s.Flags = append([]shortcut.Flag(nil), s.Flags...)
	s.Flags[0].Enum = nil
	for _, args := range [][]string{{"--node", "n", "--file", "asset.txt", "--file-view", "invalid", "--yes"}, {"--node", "n", "--file", "asset.txt", "--mime-type", "image/png", "--file-view", "preview", "--yes"}} {
		c := &docCoverageCaller{}
		if err := runDocCoverage(t, s, c, args...); err == nil || c.calls != 0 {
			t.Fatal("invalid attachment view reached RPC", err)
		}
	}
}

func TestCrossPlatformCoverageDocCopyAnchorSecondRowAndWrongPosition(t *testing.T) {
	for _, good := range []bool{true, false} {
		ids, texts := []string{"source", "ref", "new1", "tail"}, []string{"A", "R", "A", "T"}
		if !good {
			ids, texts = []string{"source", "ref", "tail", "new1"}, []string{"A", "R", "T", "A"}
		}
		read := parityBlockRead(ids, texts)
		actual := orderedJSONMLBlocks(read)
		if len(actual) != 4 {
			t.Fatalf("blocks list was mistaken for a JSONML node: count=%d", len(actual))
		}
		expected := canonicalBlockContent([]any{"p", map[string]any{}, "A"}, "jsonml")
		if got := verifyInsertedCanonicalBlock(map[string]any{"blockId": "new1"}, read, "ref", "after", expected, "jsonml", 0); got != good {
			t.Fatal("wrong anchored position verdict", got)
		}
	}
	if jsonMLBlockIdentity([]any{map[string]any{"blockId": "source"}, map[string]any{"blockId": "ref"}}) != "" {
		t.Fatal("wrapper list has false identity")
	}
	c := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content":  {parityFullJSONML([]string{"a", "b", "ref"}, []string{"A", "B", "R"})},
		"insert_document_block": {{"blockId": "new1"}},
		"list_document_blocks":  {parityBlockRead([]string{"a", "new1", "b", "ref"}, []string{"A", "A", "B", "R"})},
	}}
	err := runDocCoverage(t, Update, c, "--node", "n", "--command", "block_copy_insert_after", "--src-block-ids", "a,b", "--after-block-id", "ref", "--yes")
	if err == nil {
		t.Fatal("mispositioned copy was accepted")
	}
	encoded, _ := json.Marshal(err)
	if !bytes.Contains(encoded, []byte("new1")) {
		t.Fatalf("partial copy lost its known receipt: %s", encoded)
	}
}

func TestCrossPlatformCoverageDocSingleCopyPreservesLegacySuccessEnvelope(t *testing.T) {
	c := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content":  {parityFullJSONML([]string{"source", "ref"}, []string{"A", "R"})},
		"insert_document_block": {{"blockId": "new"}},
		"list_document_blocks":  {parityBlockRead([]string{"source", "ref", "new"}, []string{"A", "R", "A"})},
	}}
	result := runDocCoverageEnvelope(t, Update, c, "--node", "n", "--command", "block_copy_insert_after", "--block-id", "source", "--after-block-id", "ref", "--yes")
	if result["operation"] != "doc.update" {
		t.Fatal("legacy operation changed", result)
	}
	data := result["data"].(map[string]any)
	if data["verified"] != true || data["result"] == nil || data["verification"] == nil || data["copies"] != nil {
		t.Fatal("legacy success shape changed", data)
	}
}
