// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDocReadbackRetriesStaleContent(t *testing.T) {
	testseam.Swap(t, &docVerifySleep, func(time.Duration) {})
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

func TestCrossPlatformCoverageDocDeleteReadbackConsumesEveryPage(t *testing.T) {
	testseam.Swap(t, &docVerifySleep, func(time.Duration) {})
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
	serverExpanded := "[\"p\",{\"uuid\":\"generated\"},[\"span\",{\"data-type\":\"text\"},[\"span\",{\"data-type\":\"leaf\"},\"text\"]]]"
	if normalizeJSONMLForVerification(expected) != normalizeJSONMLForVerification(serverExpanded) {
		t.Fatal("generated JSONML text wrappers should not change document meaning")
	}
	linkA := "[\"a\",{\"href\":\"https://example.com/a\"},\"text\"]"
	linkB := "[\"a\",{\"href\":\"https://example.com/b\"},\"text\"]"
	if normalizeJSONMLForVerification(linkA) == normalizeJSONMLForVerification(linkB) {
		t.Fatal("semantic JSONML attributes were ignored")
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

func TestCrossPlatformCoverageMarkdownSemanticDifferencesRemainStrict(t *testing.T) {
	for _, test := range []struct {
		name  string
		left  string
		right string
	}{
		{name: "emphasis", left: `*important*`, right: `\*important\*`},
		{name: "inline code", left: "`sales_data`", right: "`sales\\_data`"},
		{name: "fenced code", left: "```\nsales_data\n```", right: "```\nsales\\_data\n```"},
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
	if !revertResultMatchesVersion(map[string]any{"revertedToVersion": 3}, 3) {
		t.Fatal("explicit target-version acknowledgement was not accepted")
	}
}

func TestCrossPlatformCoverageDocReadbackDefensiveEdges(t *testing.T) {
	testseam.Swap(t, &docVerifySleep, func(time.Duration) {})
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

	t.Run("total count terminates pagination", func(t *testing.T) {
		testseam.Swap(t, &docVerifyDelays, []time.Duration{})
		caller := &docCoverageCaller{responses: map[string][]map[string]any{"list_document_blocks": {{"blocks": []any{map[string]any{"id": "other"}}, "totalCount": 1}}}}
		if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "block_delete", "--block-id", "target", "--yes"); err != nil {
			t.Fatal(err)
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
