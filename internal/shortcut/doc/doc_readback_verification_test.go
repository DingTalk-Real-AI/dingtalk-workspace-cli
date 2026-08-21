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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
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

func TestCompactDocVerificationKeepsBoundedContentEvidence(t *testing.T) {
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
	testseam.Swap(t, &docMarkdownConvert, func([]byte, io.Writer) error { return errors.New("render") })
	if _, ok := markdownSemanticFingerprint("body"); ok {
		t.Fatal("failed Markdown render produced a semantic fingerprint")
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
