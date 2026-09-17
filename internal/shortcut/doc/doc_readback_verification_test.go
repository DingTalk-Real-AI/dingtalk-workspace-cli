// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

func TestCrossPlatformCoverageDocReadbackRetriesStaleContent(t *testing.T) {
	testseam.Swap(t, &docVerifyWait, func(context.Context, time.Duration) error { return nil })
	testseam.Swap(t, &docVerifyDelays, []time.Duration{time.Millisecond})
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": "old"}, {"markdown": "old\nnew"}},
	}}
	if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "append", "--content", "new", "--yes"); err != nil {
		t.Fatal(err)
	}
	reads := 0
	for _, call := range caller.history {
		if call.tool == "get_document_content" {
			reads++
		}
	}
	if reads != 2 {
		t.Fatalf("readback calls = %d, want 2; history=%#v", reads, caller.history)
	}
}

func TestCrossPlatformCoverageCompactDocVerificationKeepsBoundedContentEvidence(t *testing.T) {
	expected := "新增结论：本周发布完成"
	readback := strings.Repeat("历史正文\n", 2000) + expected
	summary := compactDocVerification(map[string]any{"markdown": readback}, expected, "append", "markdown", nil)
	if summary["verified"] != true || summary["kind"] != "content" || summary["mode"] != "append" {
		t.Fatalf("summary = %#v", summary)
	}
	if summary["readbackBytes"] != len(readback) || summary["readbackSha256"] == "" {
		t.Fatalf("summary evidence = %#v", summary)
	}
	excerpt, _ := summary["evidenceExcerpt"].(string)
	if !strings.Contains(excerpt, expected) || len([]rune(excerpt)) > docVerificationExcerptRunes+1 {
		t.Fatalf("excerpt = %q", excerpt)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 800 || bytes.Contains(encoded, []byte(strings.Repeat("历史正文", 20))) {
		t.Fatalf("verification summary is not compact: %d bytes", len(encoded))
	}
}

func TestCompactDocVerificationSummarizesBlockReadback(t *testing.T) {
	summary := compactDocVerification(map[string]any{
		"blocks": []any{
			map[string]any{"blockId": "block-1", "paragraph": map[string]any{"text": strings.Repeat("a", 2000)}},
			map[string]any{"blockId": "block-2", "paragraph": map[string]any{"text": strings.Repeat("b", 2000)}},
		},
	}, "", "", "", map[string]any{"blockId": "block-1"})
	if summary["verified"] != true || summary["kind"] != "blocks" || summary["readbackBlockCount"] != 2 || summary["targetBlockId"] != "block-1" {
		t.Fatalf("summary = %#v", summary)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 300 {
		t.Fatalf("block verification summary is not compact: %s", encoded)
	}
}

func TestCrossPlatformCoverageDocVerificationMetadataAndMissingContent(t *testing.T) {
	summary := compactDocVerification(map[string]any{
		"nodeId": "node-1", "folderId": "folder-1", "workspaceId": "space-1",
		"name": "report", "contentType": "ALIDOC", "revision": 3.0,
	}, "", "", "", nil)
	if summary["kind"] != "metadata" || summary["nodeId"] != "node-1" || summary["revision"] != 3 {
		t.Fatalf("metadata summary = %#v", summary)
	}
	if got := matchingDocumentContent(map[string]any{"markdown": "old"}, "new", "overwrite", "markdown"); got != "" {
		t.Fatalf("unexpected matching content = %q", got)
	}
}

func TestMarkdownServiceEscapedNumericLabelAndSoftBreakAreEquivalent(t *testing.T) {
	expected := "# 文学分析要点\n\n**1. 五幕结构**\n正文内容。\n\n**2. 核心冲突**\n- 条目一\n- 条目二\n"
	server := "# 文学分析要点\n\n**1\\. 五幕结构** 正文内容。\n\n**2\\. 核心冲突**\n- 条目一\n- 条目二\n"
	if verifyUpdatedDocumentContent(map[string]any{"markdown": server}, expected, "overwrite", "markdown") {
		return
	}
	expectedFingerprint, _ := markdownServiceSemanticFingerprint(expected)
	serverFingerprint, _ := markdownServiceSemanticFingerprint(server)
	t.Fatalf("escaped numeric label and soft break failed semantic verification:\nexpected: %s\nserver:   %s", expectedFingerprint, serverFingerprint)
}

func TestCrossPlatformCoverageDocReadbackStopsOnCancellation(t *testing.T) {
	//lint:ignore SA1012 This regression verifies the documented nil-context fallback.
	if err := waitForDocVerification(nil, time.Nanosecond); err != nil {
		t.Fatalf("completed verification wait = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForDocVerification(cancelled, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled verification wait = %v, want context.Canceled", err)
	}

	cmd := &cobra.Command{Use: "verify"}
	cmd.SetContext(cancelled)
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": "stale"}},
	}}
	helpers.InitDeps(caller)
	rt := shortcut.RuntimeContextForTest(cmd, Update)
	_, err := readDocVerification(rt, "get_document_content", map[string]any{"nodeId": "n"}, func(map[string]any) bool { return false })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled document verification = %v, want context.Canceled", err)
	}
	if len(caller.history) != 1 {
		t.Fatalf("cancelled document verification calls = %d, want 1", len(caller.history))
	}
}

func TestCrossPlatformCoverageDocDeleteReadbackConsumesEveryPage(t *testing.T) {
	testseam.Swap(t, &docVerifyWait, func(context.Context, time.Duration) error { return nil })
	testseam.Swap(t, &docVerifyDelays, []time.Duration{time.Millisecond})
	firstPage := make([]any, 50)
	for index := range firstPage {
		firstPage[index] = map[string]any{"id": fmt.Sprintf("block-%d", index), "text": "body"}
	}
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"list_document_blocks": {
			{"blocks": firstPage, "hasMore": true, "totalCount": 51},
			{"blocks": []any{map[string]any{"id": "target", "text": "stale"}}, "hasMore": false, "totalCount": 51},
			{"blocks": firstPage, "hasMore": false, "totalCount": 50},
		},
	}}
	if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "block_delete", "--block-id", "target", "--yes"); err != nil {
		t.Fatal(err)
	}
	starts := []int{}
	for _, call := range caller.history {
		if call.tool == "list_document_blocks" {
			starts = append(starts, call.params["startIndex"].(int))
		}
	}
	if fmt.Sprint(starts) != "[0 50 0]" {
		t.Fatalf("pagination starts = %v, want [0 50 0]", starts)
	}
}

func TestCrossPlatformCoverageDocReplacePreflightIsGloballyUnique(t *testing.T) {
	firstPage := make([]any, 50)
	for index := range firstPage {
		text := "body"
		if index == 0 {
			text = "unique needle"
		}
		firstPage[index] = map[string]any{"id": fmt.Sprintf("block-%d", index), "text": text}
	}
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"list_document_blocks": {
			{"blocks": firstPage, "hasMore": true, "totalCount": 51},
			{"blocks": []any{map[string]any{"id": "block-50", "text": "another needle"}}, "hasMore": false, "totalCount": 51},
		},
	}}
	err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "str_replace", "--old", "needle", "--new", "changed", "--yes")
	if err == nil {
		t.Fatal("str_replace accepted a second match on a later page")
	}
	for _, call := range caller.history {
		if call.tool == "update_document_block" {
			t.Fatalf("ambiguous replace executed a write: %#v", caller.history)
		}
	}
}

func TestCrossPlatformCoverageDocVerificationPreservesMeaning(t *testing.T) {
	expected := "[\"p\",{},\"text\"]"
	serverExpanded := "[\"p\",{\"uuid\":\"generated\",\"style\":{}},[\"span\",{\"data-type\":\"text\"},[\"span\",{\"data-type\":\"leaf\"},\"text\"]]]"
	if normalizeJSONMLForVerification(expected) != normalizeJSONMLForVerification(serverExpanded) {
		t.Fatal("generated JSONML text wrappers should not change document meaning")
	}
	defaultFree := `[["hr",{}],["code",{}],["table",{},["tr",{},["tc",{},["p",{},"cell"]]]]]`
	serverDefaulted := `[["hr",{"sz":1}],["code",{"code":"","syntax":"plaintext","theme":"default","wrap":true,"showLineNumber":true,"fold":false}],["table",{},["tr",{},["tc",{"colSpan":1,"rowSpan":1,"vAlign":"middle"},["p",{},"cell"]]]]]`
	if !verifyUpdatedDocumentContent(map[string]any{"jsonml": serverDefaulted}, defaultFree, "overwrite", "jsonml") {
		t.Fatal("server-generated JSONML schema defaults should not fail readback verification")
	}
	linkA := "[\"a\",{\"href\":\"https://example.com/a\"},\"text\"]"
	linkB := "[\"a\",{\"href\":\"https://example.com/b\"},\"text\"]"
	if normalizeJSONMLForVerification(linkA) == normalizeJSONMLForVerification(linkB) {
		t.Fatal("semantic JSONML attributes were ignored")
	}
	tableA := `[["table",{"jc":"center"},["tr",{},["tc",{},["p",{},"cell"]]]]]`
	tableB := `[["table",{"jc":"right"},["tr",{},["tc",{},["p",{},"cell"]]]]]`
	if normalizeJSONMLForVerification(tableA) == normalizeJSONMLForVerification(tableB) {
		t.Fatal("semantic JSONML table alignment was ignored")
	}
	codeA := "~~~go\n  return nil\n~~~"
	codeB := "~~~go\nreturn nil\n~~~"
	if normalizeMarkdownForVerification(codeA) == normalizeMarkdownForVerification(codeB) {
		t.Fatal("fenced code indentation was ignored")
	}
	if !verifyUpdatedDocumentContent(map[string]any{"markdown": "# Server title\n\nbody"}, "body", "overwrite", "markdown") {
		t.Fatal("server-generated document title prevented body verification")
	}
}

func TestCrossPlatformCoverageMarkdownSemanticRoundTrip(t *testing.T) {
	input := strings.Join([]string{
		"sales_data.xlsx",
		"### 1.",
		"+10.22%",
		"| name | value |",
		"| -------- | -------- |",
		"| sales_data.xlsx | +10.22% |",
	}, "\n")
	server := strings.Join([]string{
		`sales\_data.xlsx`,
		`### 1\.`,
		`\+10.22%`,
		"|name|value|",
		"|---|---|",
		`|sales\_data.xlsx|\+10.22%|`,
	}, "\n")
	if !markdownSemanticallyEquivalent(input, server) {
		t.Fatal("server Markdown escaping and table delimiter normalization changed the semantic fingerprint")
	}
	if !verifyUpdatedDocumentContent(map[string]any{"markdown": server}, input, "overwrite", "markdown") {
		t.Fatal("equivalent server Markdown failed overwrite verification")
	}
	if !verifyUpdatedDocumentContent(map[string]any{"markdown": "existing\n\n" + server}, input, "append", "markdown") {
		t.Fatal("equivalent server Markdown failed append verification")
	}
}

func TestCrossPlatformCoverageMarkdownServiceLineBreakNormalization(t *testing.T) {
	input := strings.Join([]string{
		"### 一、五幕结构",
		"",
		"**第一幕：宿命相遇**  ",
		"维罗纳街头爆发冲突。",
		"",
		"1. **家族世仇 vs. 个体爱情**  ",
		"   旧秩序压制青年自由恋爱。",
		"",
		"2. **命运偶然 vs. 人为选择**  ",
		"   偶然事件与冲动选择共同造成悲剧。",
	}, "\n")
	server := strings.Join([]string{
		"### 一、五幕结构",
		"",
		"**第一幕：宿命相遇**   维罗纳街头爆发冲突。",
		"",
		"1. **家族世仇 vs. 个体爱情**   旧秩序压制青年自由恋爱。",
		"2. **命运偶然 vs. 人为选择**   偶然事件与冲动选择共同造成悲剧。",
	}, "\n")
	if !verifyUpdatedDocumentContent(map[string]any{"markdown": server}, input, "overwrite", "markdown") {
		inputFingerprint, _ := markdownServiceSemanticFingerprint(input)
		serverFingerprint, _ := markdownServiceSemanticFingerprint(server)
		t.Fatalf("service line-break and list-tightness normalization failed verification:\ninput:  %s\nserver: %s", inputFingerprint, serverFingerprint)
	}
	missingListItem := strings.Replace(server, "2. **命运偶然 vs. 人为选择**   偶然事件与冲动选择共同造成悲剧。", "", 1)
	if verifyUpdatedDocumentContent(map[string]any{"markdown": missingListItem}, input, "overwrite", "markdown") {
		t.Fatal("missing list item passed semantic verification")
	}
	changedText := strings.Replace(server, "共同造成悲剧", "不会造成悲剧", 1)
	if verifyUpdatedDocumentContent(map[string]any{"markdown": changedText}, input, "overwrite", "markdown") {
		t.Fatal("changed document text passed semantic verification")
	}
}

func TestCrossPlatformCoverageDocContentInputNormalizesLineEndings(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("body.md", []byte("first\r\nsecond\rthird"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := "first\nsecond\nthird"
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": want}},
	}}
	if err := runDocCoverage(t, Create, caller, "--name", "normalized", "--content", "@body.md"); err != nil {
		t.Fatal(err)
	}
	if got := caller.history[0].params["markdown"]; got != want {
		t.Fatalf("create markdown = %#v, want normalized line endings %#v", got, want)
	}
}

func TestCrossPlatformCoverageMarkdownSemanticDifferencesRemainStrict(t *testing.T) {
	for _, test := range []struct {
		name  string
		left  string
		right string
	}{
		{name: "emphasis", left: `*important*`, right: `\*important\*`},
		{name: "inline code", left: "`sales_data`", right: "`sales\\_data`"},
		{name: "fenced code", left: "```\nsales_data\n```", right: "```\nsales\\_data\n```"},
		{name: "link destination", left: "[source](https://example.com/a)", right: "[source](https://example.com/b)"},
		{name: "table alignment", left: "|a|\n|---|\n|x|", right: "|a|\n|:---|\n|x|"},
		{name: "table columns", left: "|a|b|\n|---|---|\n|x|y|", right: "|a|\n|---|\n|x|"},
		{name: "table content", left: "|a|\n|---|\n|x|", right: "|a|\n|---|\n|y|"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if markdownSemanticallyEquivalent(test.left, test.right) {
				t.Fatal("meaningful Markdown difference was ignored")
			}
		})
	}
	oversized := strings.Repeat("x", docMarkdownVerifyMax+1)
	if _, ok := markdownSemanticFingerprint(oversized); ok {
		t.Fatal("oversized Markdown entered semantic verification")
	}
	if _, ok := markdownServiceSemanticFingerprint(oversized); ok {
		t.Fatal("oversized Markdown entered service semantic verification")
	}
	testseam.Swap(t, &docMarkdownConvert, func([]byte, io.Writer) error { return errors.New("render") })
	if _, ok := markdownSemanticFingerprint("body"); ok {
		t.Fatal("failed Markdown render produced a semantic fingerprint")
	}
}

func TestCrossPlatformCoverageMarkdownServiceFingerprintNodeKinds(t *testing.T) {
	for _, source := range []string{
		"    indented code\n",
		"<script>\nalert('x')\n</script>\n",
		"before <span>inline</span> after\n",
		"<https://example.com/path>\n",
		"[ref]: https://example.com/path \"title\"\n\n[link][ref]\n",
		"![alt](https://example.com/image.png \"title\")\n",
	} {
		if fingerprint, ok := markdownServiceSemanticFingerprint(source); !ok || fingerprint == "" {
			t.Fatalf("fingerprint failed for %q: %q/%v", source, fingerprint, ok)
		}
	}
	testseam.Swap(t, &docMarkdown, goldmark.New(goldmark.WithExtensions(extension.Typographer)))
	if fingerprint, ok := markdownServiceSemanticFingerprint("before -- after"); !ok || fingerprint == "" {
		t.Fatalf("typographer string fingerprint = %q/%v", fingerprint, ok)
	}
}

func TestCrossPlatformCoverageDocElementReadbackUsesNestedElement(t *testing.T) {
	wrapper := map[string]any{
		"blockType": "paragraph",
		"element":   map[string]any{"id": "inserted", "blockType": "paragraph", "paragraph": map[string]any{"text": "body"}},
	}
	if got := canonicalBlockContent(wrapper, "markdown"); got != "body" {
		t.Fatalf("nested element content = %q, want body", got)
	}
	blocks := orderedDocumentBlocks(map[string]any{"blocks": []any{wrapper}})
	if len(blocks) != 1 || blockIdentity(blocks[0], "") != "inserted" {
		t.Fatalf("nested element blocks = %#v", blocks)
	}
}

func TestCrossPlatformCoverageVersionRevertRequiresTargetEvidence(t *testing.T) {
	if revertResultMatchesVersion(map[string]any{"ok": true}, 3) || currentDocumentMatchesRestoredVersion(map[string]any{"version": 99}, 3) {
		t.Fatal("readability or an unrelated current version must not prove a revert")
	}
	if revertResultMatchesVersion(map[string]any{"version": 3}, 3) {
		t.Fatal("the request version parameter must not prove its own revert")
	}
	if revertResultMatchesVersion(map[string]any{"data": map[string]any{"request": map[string]any{"targetVersion": 3}}}, 3) {
		t.Fatal("request echo containers must not provide version evidence")
	}
	for _, failed := range []map[string]any{
		{"data": map[string]any{"success": "false", "revertedToVersion": 3}},
		{"data": map[string]any{"ok": false, "revertedToVersion": 3}},
		{"data": map[string]any{"status": "FAILED", "revertedToVersion": 3}},
		{"data": map[string]any{"state": "failure", "revertedToVersion": 3}},
		{"data": []any{map[string]any{"error_code": "REVERT_FAILED", "revertedToVersion": 3}}},
		{"data": map[string]any{"errorCode": 500.0, "revertedToVersion": 3}},
		{"data": map[string]any{"code": json.Number("500"), "revertedToVersion": 3}},
	} {
		if revertResultMatchesVersion(failed, 3) {
			t.Fatalf("explicit failure %#v must override target-version evidence", failed)
		}
	}
	for _, succeeded := range []map[string]any{
		{"revertedToVersion": 3},
		{"status": "SUCCESS", "revertedToVersion": 3},
		{"state": "succeeded", "revertedToVersion": 3},
		{"errorCode": "0", "revertedToVersion": 3},
		{"code": 200, "revertedToVersion": 3},
		{"code": "204", "revertedToVersion": 3},
	} {
		if !revertResultMatchesVersion(succeeded, 3) {
			t.Fatalf("explicit success %#v suppressed target-version evidence", succeeded)
		}
	}
	for _, absentOrSuccess := range []any{nil, "", "OK", "SUCCESS", json.Number("0"), json.Number("200"), 0.0, 201.0, 0, 202} {
		if revertErrorCodeIsFailure(absentOrSuccess) {
			t.Fatalf("success code %#v was treated as an explicit failure", absentOrSuccess)
		}
	}
	for _, failedCode := range []any{json.Number("bad"), json.Number("500"), 1.0, 500.0, 1, 500} {
		if !revertErrorCodeIsFailure(failedCode) {
			t.Fatalf("failure code %#v was not treated as an explicit failure", failedCode)
		}
	}
	if revertStatusIsFailure(500) || revertStatusIsFailure("PROCESSING") {
		t.Fatal("non-failure status was treated as an explicit failure")
	}
}

func TestCrossPlatformCoverageDocReadbackDefensiveEdges(t *testing.T) {
	testseam.Swap(t, &docVerifyWait, func(context.Context, time.Duration) error { return nil })
	for _, tc := range []struct {
		name      string
		responses []map[string]any
		failAt    int
	}{
		{"call failure", nil, 1},
		{"missing blocks", []map[string]any{{"ok": true}}, 0},
		{"stalled page", []map[string]any{{"blocks": []any{map[string]any{"id": "a"}}, "hasMore": true}, {"blocks": []any{map[string]any{"id": "a"}}, "hasMore": true}}, 0},
		{"empty continued page", []map[string]any{{"blocks": []any{}, "hasMore": true}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &docVerifyDelays, []time.Duration{})
			caller := &docCoverageCaller{failAt: tc.failAt, responses: map[string][]map[string]any{"list_document_blocks": tc.responses}}
			err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "block_delete", "--block-id", "target", "--yes")
			if err == nil {
				t.Fatal("defensive readback unexpectedly succeeded")
			}
		})
	}

	t.Run("identical adjacent pages advance by requested indexes", func(t *testing.T) {
		testseam.Swap(t, &docVerifyDelays, []time.Duration{})
		blocks := make([]any, docBlockReadPageSize)
		for index := range blocks {
			blocks[index] = map[string]any{"blockType": "paragraph"}
		}
		caller := &docCoverageCaller{responses: map[string][]map[string]any{"list_document_blocks": {
			{"blocks": blocks, "hasMore": true, "totalCount": 2 * docBlockReadPageSize},
			{"blocks": blocks, "hasMore": false, "totalCount": 2 * docBlockReadPageSize},
		}}}
		if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "block_delete", "--block-id", "target", "--yes"); err != nil {
			t.Fatal(err)
		}
		starts := []int{}
		for _, call := range caller.history {
			if call.tool == "list_document_blocks" {
				starts = append(starts, call.params["startIndex"].(int))
			}
		}
		if fmt.Sprint(starts) != "[0 50]" {
			t.Fatalf("pagination starts = %v, want [0 50]", starts)
		}
	})

	t.Run("total count terminates pagination", func(t *testing.T) {
		testseam.Swap(t, &docVerifyDelays, []time.Duration{})
		caller := &docCoverageCaller{responses: map[string][]map[string]any{"list_document_blocks": {{"blocks": []any{map[string]any{"id": "other"}}, "totalCount": 1}}}}
		if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "block_delete", "--block-id", "target", "--yes"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("explicit has more overrides inconsistent total count", func(t *testing.T) {
		testseam.Swap(t, &docVerifyDelays, []time.Duration{})
		caller := &docCoverageCaller{responses: map[string][]map[string]any{"list_document_blocks": {
			{"blocks": []any{map[string]any{"id": "first"}}, "hasMore": true, "totalCount": 1},
			{"blocks": []any{map[string]any{"id": "second"}}, "hasMore": false, "totalCount": 2},
		}}}
		if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "block_delete", "--block-id", "target", "--yes"); err != nil {
			t.Fatal(err)
		}
		reads := 0
		for _, call := range caller.history {
			if call.tool == "list_document_blocks" {
				reads++
			}
		}
		if reads != 2 {
			t.Fatalf("read calls = %d, want both explicitly advertised pages", reads)
		}
	})

	t.Run("block read safety limit", func(t *testing.T) {
		testseam.Swap(t, &docVerifyDelays, []time.Duration{})
		pages := make([]map[string]any, docBlockReadMaxItems/docBlockReadPageSize)
		for index := range pages {
			pages[index] = map[string]any{"blocks": []any{map[string]any{"id": fmt.Sprintf("block-%d", index)}}, "hasMore": true}
		}
		caller := &docCoverageCaller{responses: map[string][]map[string]any{"list_document_blocks": pages}}
		if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "block_delete", "--block-id", "target", "--yes"); err == nil {
			t.Fatal("oversized block read returned nil")
		}
	})

	if blocks, ok := documentBlockEntries(map[string]any{"jsonml": `["root",{},["p",{"uuid":"a"},"x"]]`}); !ok || len(blocks) == 0 {
		t.Fatalf("jsonml blocks=%#v ok=%v", blocks, ok)
	}
	if _, ok := documentBlockEntries(map[string]any{"jsonml": `{`}); ok {
		t.Fatal("invalid jsonml produced blocks")
	}
	if blocks, ok := documentBlockEntries(map[string]any{"data": map[string]any{"items": []any{"x"}}}); !ok || len(blocks) != 1 {
		t.Fatalf("nested items=%#v ok=%v", blocks, ok)
	}
	if _, ok := documentBlockEntries(nil); ok {
		t.Fatal("nil produced blocks")
	}

	for _, tc := range []struct {
		value any
		want  bool
	}{
		{map[string]any{"totalCount": float64(2)}, true},
		{map[string]any{"totalCount": float64(-1)}, false},
		{map[string]any{"totalCount": 2.5}, false},
		{map[string]any{"data": map[string]any{"total_count": 2}}, true},
		{map[string]any{"totalCount": -1}, false},
		{nil, false},
	} {
		_, ok := nestedNonNegativeInt(tc.value, "totalCount", "total_count")
		if ok != tc.want {
			t.Fatalf("nestedNonNegativeInt(%#v) ok=%v want=%v", tc.value, ok, tc.want)
		}
	}

	for _, raw := range []string{
		`[]`, `[1,["p",{},"x"]]`, `["span",{},"a","b"]`,
		`["p",{"block_id":"x","custom":true},"x"]`, `true`,
	} {
		if normalizeJSONMLForVerification(raw) == "" {
			t.Fatalf("empty normalized JSONML for %s", raw)
		}
	}
	if !isGeneratedTextSpan(nil) || isGeneratedTextSpan(map[string]any{"a": 1, "b": 2}) || isGeneratedTextSpan(map[string]any{"data-type": 3}) || isGeneratedTextSpan(map[string]any{"data-type": "other"}) {
		t.Fatal("generated span classification failed")
	}
}

// The document service rewrites [@name](alidocs-mcp://doc/mention?openDingTalkId=X)
// into a profile link while committing markdown, so the authored destination can
// never appear in a readback. Before mention canonicalization every verified
// write path reported doc_write_verification_failed even though the content had
// landed, which pushed agents into retrying and duplicating content.
//
// Only the link shape matters here, so every identifier below is synthetic. Real
// user names and real openDingTalkId / corpId / staffId values must never be
// committed as fixtures.
const (
	docMentionAuthored = "请 [@测试甲](alidocs-mcp://doc/mention?openDingTalkId=DEXAMPLEMENTIONIDAAAA) 跟进。"
	docMentionServer   = "请 [@测试甲](dingtalk://dingtalkclient/page/profile?corp_id=dingexamplecorpid&staff_id=100001) 跟进。"
)

func TestCrossPlatformCoverageDocMentionRewritePassesVerifiedWrites(t *testing.T) {
	for _, tc := range []struct {
		name     string
		readback string
		args     []string
	}{
		{
			name:     "append",
			readback: "锚点段落\n\n" + docMentionServer,
			args:     []string{"--node", "n", "--command", "append", "--content", docMentionAuthored, "--yes"},
		},
		{
			name:     "overwrite",
			readback: docMentionServer,
			args:     []string{"--node", "n", "--command", "overwrite", "--content", docMentionAuthored, "--yes"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{
				"get_document_content": {{"markdown": tc.readback}},
			}}
			if err := runDocCoverage(t, Update, caller, tc.args...); err != nil {
				t.Fatalf("verified %s write with a rewritten mention must succeed: %v", tc.name, err)
			}
		})
	}
}

func TestCrossPlatformCoverageDocMentionRewritePassesVerifiedCreate(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"create_document":      {{"nodeId": "created-node"}},
		"get_document_content": {{"markdown": docMentionServer}},
	}}
	if err := runDocCoverage(t, Create, caller,
		"--name", "周报", "--content", docMentionAuthored, "--yes"); err != nil {
		t.Fatalf("verified create with a rewritten mention must succeed: %v", err)
	}
}

func TestCrossPlatformCoverageDocMentionCanonicalizationStillDetectsDrift(t *testing.T) {
	// Canonicalization drops only the rewritten destination. Label text, node
	// order and the presence of the link stay in the fingerprint, so a write
	// that did not actually land must still fail.
	for _, tc := range []struct {
		name     string
		readback string
	}{
		{"mention dropped entirely", "请 跟进。"},
		{"label changed", "请 [@测试乙](dingtalk://dingtalkclient/page/profile?corp_id=dingexamplecorpid&staff_id=100002) 跟进。"},
		{"link degraded to plain text", "请 @测试甲 跟进。"},
		{"surrounding prose lost", docMentionServer + "\n多写了一段"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{
				"get_document_content": {{"markdown": tc.readback}},
			}}
			err := runDocCoverage(t, Update, caller,
				"--node", "n", "--command", "overwrite", "--content", docMentionAuthored, "--yes")
			var typed *apperrors.Error
			if err == nil || !errors.As(err, &typed) || typed.Reason != "doc_write_verification_failed" {
				t.Fatalf("readback %q must still fail verification, got %v", tc.readback, err)
			}
		})
	}
}

func TestCrossPlatformCoverageDocMentionCanonicalizationScopedToMentionWrites(t *testing.T) {
	// The relaxation is gated on the authored side carrying the mention
	// protocol. A write with no mention must keep the strict comparison, so a
	// server that silently rewrote an ordinary link still fails.
	authored := "见 [钉钉](https://www.dingtalk.com) 说明。"
	server := "见 [钉钉](https://example.com/rewritten) 说明。"
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content": {{"markdown": server}},
	}}
	err := runDocCoverage(t, Update, caller,
		"--node", "n", "--command", "overwrite", "--content", authored, "--yes")
	var typed *apperrors.Error
	if err == nil || !errors.As(err, &typed) || typed.Reason != "doc_write_verification_failed" {
		t.Fatalf("non-mention writes must keep strict destination comparison, got %v", err)
	}

	if _, ok := markdownServiceSemanticTokens(strings.Repeat("x", docMarkdownVerifyMax+1)); ok {
		t.Fatal("oversized input must not produce fingerprint tokens")
	}
	if docContentHasMentionLink(authored) || !docContentHasMentionLink(docMentionAuthored) {
		t.Fatal("mention protocol detection failed")
	}
	ordinary := docFingerprintLinkTokenPrefix + "https://www.dingtalk.com"
	if isMentionProtocolLinkToken(ordinary) || isProfileLinkToken(ordinary) {
		t.Fatal("ordinary destinations must not be treated as mention or profile links")
	}
}

// Mention pairing is positional: only a position where the author wrote the
// mention protocol may hold a profile link on readback. An earlier revision
// collapsed every profile-shaped link instead, which silently stopped verifying
// an ordinary profile link the author wrote themselves.
func TestCrossPlatformCoverageDocMentionVerificationPairing(t *testing.T) {
	const (
		mentionA = "alidocs-mcp://doc/mention?openDingTalkId=DEXAMPLEAAAA"
		mentionB = "alidocs-mcp://doc/mention?openDingTalkId=DEXAMPLEBBBB"
		profile1 = "dingtalk://dingtalkclient/page/profile?corp_id=dingexamplecorpid&staff_id=100001"
		profile2 = "dingtalk://dingtalkclient/page/profile?corp_id=dingexamplecorpid&staff_id=100002"
	)

	for _, tc := range []struct {
		name     string
		expected string
		actual   string
		mode     string
		want     bool
	}{
		// Service-side normalization that must stay tolerated, with and without
		// a mention in the body.
		{"overwrite tolerates an added document title", "正文段落。", "# 文档标题\n\n正文段落。", "overwrite", true},
		{
			"overwrite tolerates an added title alongside a mention",
			"请 [@测试甲](" + mentionA + ") 跟进。",
			"# 文档标题\n\n请 [@测试甲](" + profile1 + ") 跟进。",
			"overwrite", true,
		},
		{"append tolerates preceding content", "新增段落。", "旧段落。\n\n新增段落。", "append", true},
		{
			"append tolerates preceding content alongside a mention",
			"追加 [@测试甲](" + mentionA + ") 完。",
			"旧段落。\n\n追加 [@测试甲](" + profile1 + ") 完。",
			"append", true,
		},

		// A mention and an ordinary profile link coexisting: the mention may be
		// rewritten, the ordinary link may not drift.
		{
			"mention rewrite beside an intact ordinary profile link",
			"请 [@测试甲](" + mentionA + ") 跟进，负责人 [某人](" + profile1 + ")。",
			"请 [@测试甲](" + profile2 + ") 跟进，负责人 [某人](" + profile1 + ")。",
			"overwrite", true,
		},
		{
			"ordinary profile link drifting to another target is rejected",
			"请 [@测试甲](" + mentionA + ") 跟进，负责人 [某人](" + profile1 + ")。",
			"请 [@测试甲](" + profile2 + ") 跟进，负责人 [某人](" + profile2 + ")。",
			"overwrite", false,
		},

		// Drift at the mention position itself.
		{"dropped mention is rejected", "请 [@测试甲](" + mentionA + ") 跟进。", "请 跟进。", "overwrite", false},
		{
			"changed mention label is rejected",
			"请 [@测试甲](" + mentionA + ") 跟进。",
			"请 [@测试乙](" + profile1 + ") 跟进。",
			"overwrite", false,
		},
		{
			"mention degraded to plain text is rejected",
			"请 [@测试甲](" + mentionA + ") 跟进。", "请 @测试甲 跟进。", "overwrite", false,
		},

		// Without an authored mention nothing is relaxed.
		{
			"profile link drift without any mention is rejected",
			"负责人 [某人](" + profile1 + ")。", "负责人 [某人](" + profile2 + ")。", "overwrite", false,
		},
		{
			"ordinary link drift without any mention is rejected",
			"见 [钉钉](https://www.dingtalk.com)。", "见 [钉钉](https://example.com)。", "overwrite", false,
		},

		// The comparison layer accepts any profile destination at a mention
		// position — targets are simply not compared, whether there is one mention
		// or several, same label or not. That is why the envelope reports
		// mentionTargetsVerified=false instead of claiming target verification;
		// see TestCrossPlatformCoverageDocMentionTargetsReportedUnverified.
		{
			"swapped mention targets pass the comparison (targets are not compared)",
			"[@同名](" + mentionA + ") 与 [@同名](" + mentionB + ")",
			"[@同名](" + profile2 + ") 与 [@同名](" + profile1 + ")",
			"overwrite", true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := verifyUpdatedDocumentContent(
				map[string]any{"markdown": tc.actual}, tc.expected, tc.mode, "markdown")
			if got != tc.want {
				t.Fatalf("verify = %v, want %v\nexpected=%q\nactual=%q", got, tc.want, tc.expected, tc.actual)
			}
		})
	}
}

// Readback cannot tell which user a mention resolved to: the service rewrites
// openDingTalkId into a profile link and the two identifiers have no local
// mapping. Rather than failing an otherwise correct write, or letting "verified"
// imply more than it checked, the result says so explicitly.
func TestCrossPlatformCoverageDocMentionTargetsReportedUnverified(t *testing.T) {
	const mention = "alidocs-mcp://doc/mention?openDingTalkId=DEXAMPLEAAAA"
	const profile = "dingtalk://dingtalkclient/page/profile?corp_id=dingexamplecorpid&staff_id=100001"

	withMention := "请 [@测试甲](" + mention + ") 跟进。"
	summary := compactDocVerification(
		map[string]any{"markdown": "请 [@测试甲](" + profile + ") 跟进。"},
		withMention, "overwrite", "markdown", nil)
	// A mention write cannot be fully verified, so the summary must not claim it.
	if summary["verified"] != false {
		t.Fatalf("a mention write must not report itself as verified: %#v", summary)
	}
	if summary["mentionTargetsVerified"] != false {
		t.Fatalf("mention targets must be reported as unverified: %#v", summary)
	}

	plain := "普通段落，无 @人。"
	plainSummary := compactDocVerification(
		map[string]any{"markdown": plain}, plain, "overwrite", "markdown", nil)
	if _, present := plainSummary["mentionTargetsVerified"]; present {
		t.Fatalf("a write without mentions must not carry the flag: %#v", plainSummary)
	}

	warnings := withMentionTargetWarning([]string{"既有告警"}, withMention)
	if len(warnings) != 2 || warnings[0] != "既有告警" ||
		!strings.Contains(warnings[1], "需用户自行核对") {
		t.Fatalf("mention warning must be appended after existing ones: %#v", warnings)
	}
	if got := withMentionTargetWarning(nil, plain); got != nil {
		t.Fatalf("no mention means no warning: %#v", got)
	}

	// The scope marker sits beside "verified" so the top level qualifies itself,
	// and the verify step carries the same scope.
	data := map[string]any{"verified": true}
	steps := []map[string]any{{"name": "update_document", "status": "success"},
		{"name": "verify", "status": "success"}}
	annotateMentionVerificationScope(data, steps, withMention)
	if data["verified"] != false {
		t.Fatalf("top level must not claim verified for a mention write: %#v", data)
	}
	if data["verificationScope"] != "partial" {
		t.Fatalf("top level must declare a partial scope: %#v", data)
	}
	local, _ := data["unverifiableLocally"].([]string)
	if len(local) != 1 || local[0] != "mention_targets" {
		t.Fatalf("the gap must be marked as not checkable locally: %#v", data["unverifiableLocally"])
	}
	gaps, _ := data["unverified"].([]string)
	if len(gaps) != 1 || gaps[0] != "mention_targets" {
		t.Fatalf("the gap must be named explicitly: %#v", data["unverified"])
	}
	if steps[1]["scope"] != "partial" {
		t.Fatalf("verify step must carry the scope: %#v", steps)
	}
	// A consumer that only switches on steps[].status must not read this as a
	// fully verified readback, so the status itself stops saying "success".
	if steps[1]["status"] != "partial" {
		t.Fatalf("verify step must not stay success for a mention write: %#v", steps)
	}
	if _, present := steps[0]["scope"]; present {
		t.Fatalf("only the verify step is scoped: %#v", steps[0])
	}
	if steps[0]["status"] != "success" {
		t.Fatalf("the write step keeps its own status: %#v", steps[0])
	}

	plainData := map[string]any{"verified": true}
	plainSteps := []map[string]any{{"name": "verify", "status": "success"}}
	annotateMentionVerificationScope(plainData, plainSteps, plain)
	if plainData["verified"] != true {
		t.Fatalf("a write without mentions stays fully verified: %#v", plainData)
	}
	if _, present := plainData["verificationScope"]; present {
		t.Fatalf("a write without mentions stays unqualified: %#v", plainData)
	}
	if _, present := plainSteps[0]["scope"]; present {
		t.Fatalf("a write without mentions leaves steps untouched: %#v", plainSteps[0])
	}
	if plainSteps[0]["status"] != "success" {
		t.Fatalf("a write without mentions keeps a successful verify: %#v", plainSteps[0])
	}

	// The warning carries exactly two facts: what the caller must check itself,
	// and that the rest was verified.
	for _, fact := range []string{"需用户自行核对", "均已通过回读校验"} {
		if !strings.Contains(docMentionTargetUnverifiedWarning, fact) {
			t.Fatalf("warning must state %q: %q", fact, docMentionTargetUnverifiedWarning)
		}
	}
}

// The scope disclosure only matters if it survives to what the commands
// actually publish, so assert the delivered envelope of every content-write
// path rather than just that execution succeeded.
func TestCrossPlatformCoverageDocMentionUnverifiedReachesEveryWriteEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		responses   map[string][]map[string]any
		args        []string
	}{
		{
			name:        "create",
			declaration: Create,
			responses: map[string][]map[string]any{
				"create_document":      {{"nodeId": "created-node"}},
				"get_document_content": {{"markdown": docMentionServer}},
			},
			args: []string{"--name", "周报", "--content", docMentionAuthored, "--yes"},
		},
		{
			name:        "update append",
			declaration: Update,
			responses: map[string][]map[string]any{
				"get_document_content": {{"markdown": "锚点段落\n\n" + docMentionServer}},
			},
			args: []string{"--node", "n", "--command", "append", "--content", docMentionAuthored, "--yes"},
		},
		{
			name:        "update overwrite",
			declaration: Update,
			responses: map[string][]map[string]any{
				"get_document_content": {{"markdown": docMentionServer}},
			},
			args: []string{"--node", "n", "--command", "overwrite", "--content", docMentionAuthored, "--yes"},
		},
		{
			name:        "checkpoint update",
			declaration: CheckpointUpdate,
			responses: map[string][]map[string]any{
				"save_doc_version":     {{"version": 9.0}},
				"get_document_content": {{"markdown": docMentionServer}},
			},
			args: []string{"--node", "n", "--content", docMentionAuthored, "--yes"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			envelope := runDocCoverageEnvelope(t, tc.declaration,
				&docCoverageCaller{responses: tc.responses}, tc.args...)

			// The write itself succeeded; only its verification scope is narrower.
			if envelope["ok"] != true || envelope["status"] != "success" {
				t.Fatalf("a mention write is still a successful operation: %#v", envelope)
			}
			data, _ := envelope["data"].(map[string]any)
			if data["verified"] != false {
				t.Fatalf("delivered data must not claim verified: %#v", data)
			}
			if data["verificationScope"] != "partial" {
				t.Fatalf("delivered data must declare a partial scope: %#v", data)
			}

			steps, _ := envelope["steps"].([]any)
			var verify map[string]any
			for _, entry := range steps {
				step, _ := entry.(map[string]any)
				if step["name"] == "verify" {
					verify = step
				}
			}
			if verify == nil {
				t.Fatalf("every content write publishes a verify step: %#v", steps)
			}
			if verify["status"] == "success" {
				t.Fatalf("delivered verify step must not report success: %#v", verify)
			}
			if verify["status"] != "partial" || verify["scope"] != "partial" {
				t.Fatalf("delivered verify step must be marked partial: %#v", verify)
			}

			warnings, _ := envelope["warnings"].([]any)
			if len(warnings) == 0 {
				t.Fatalf("the caller must be told what to check itself: %#v", envelope)
			}
		})
	}
}

// The append comparison has three layers: rendered HTML, the layout-tolerant
// service fingerprint, and mention-aware token pairing. Each needs its own
// input shape, otherwise a layer silently stops being exercised.
func TestCrossPlatformCoverageDocMentionSuffixComparisonLayers(t *testing.T) {
	// Whitespace collapse is layout-only: the rendered-HTML suffix differs, the
	// service fingerprint does not, so the append still verifies.
	if !markdownSemanticallyEndsWith("intro\n\ndone   now", "done now") {
		t.Fatal("layout-only whitespace difference must still match as a suffix")
	}

	mention := "[@测试甲](alidocs-mcp://doc/mention?openDingTalkId=DEXAMPLEAAAA)"
	if markdownSemanticallyEndsWith("", mention) {
		t.Fatal("an empty readback cannot contain an appended mention")
	}
	if markdownSemanticallyEndsWith(strings.Repeat("x", docMarkdownVerifyMax+1), mention) {
		t.Fatal("an oversized readback must not be accepted")
	}
}

// Mention pairing must form a consistent bijection. Neither check needs an
// identity lookup, so both are enforced locally; a permutation of two distinct
// targets stays undetectable because the authored side carries no staffId.
func TestCrossPlatformCoverageDocMentionPairingRequiresBijection(t *testing.T) {
	const (
		mentionA = "alidocs-mcp://doc/mention?openDingTalkId=DEXAMPLEAAAA"
		mentionB = "alidocs-mcp://doc/mention?openDingTalkId=DEXAMPLEBBBB"
		profile1 = "dingtalk://dingtalkclient/page/profile?corp_id=dingexamplecorpid&staff_id=100001"
		profile2 = "dingtalk://dingtalkclient/page/profile?corp_id=dingexamplecorpid&staff_id=100002"
	)

	for _, tc := range []struct {
		name     string
		expected string
		actual   string
		want     bool
	}{
		{
			"one id resolving to one target twice is accepted",
			"[@甲](" + mentionA + ") 与 [@甲](" + mentionA + ")",
			"[@甲](" + profile1 + ") 与 [@甲](" + profile1 + ")",
			true,
		},
		{
			"the same id resolving to two different targets is rejected",
			"[@甲](" + mentionA + ") 与 [@甲](" + mentionA + ")",
			"[@甲](" + profile1 + ") 与 [@甲](" + profile2 + ")",
			false,
		},
		{
			"two different ids collapsing onto one target is rejected",
			"[@甲](" + mentionA + ") 与 [@乙](" + mentionB + ")",
			"[@甲](" + profile1 + ") 与 [@乙](" + profile1 + ")",
			false,
		},
		{
			"two different ids resolving to two different targets is accepted",
			"[@甲](" + mentionA + ") 与 [@乙](" + mentionB + ")",
			"[@甲](" + profile1 + ") 与 [@乙](" + profile2 + ")",
			true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := verifyUpdatedDocumentContent(
				map[string]any{"markdown": tc.actual}, tc.expected, "overwrite", "markdown")
			if got != tc.want {
				t.Fatalf("verify = %v, want %v\nexpected=%q\nactual=%q", got, tc.want, tc.expected, tc.actual)
			}
		})
	}

	if got := docFingerprintLinkDestination(docFingerprintLinkTokenPrefix + profile1 + "\x00"); got != profile1 {
		t.Fatalf("destination extraction = %q", got)
	}
	if got := docFingerprintLinkDestination("open\x00link:bare"); got != "bare" {
		t.Fatalf("a token without a title separator must still yield its destination: %q", got)
	}
}
