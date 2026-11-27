// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

const highlightedMarkdown = `before <span style="background-color: #FFC000;">important</span> after`

func highlightJSONML(before, marked, after, color string) string {
	encoded, _ := json.Marshal([]any{"root", map[string]any{}, []any{"p", map[string]any{},
		[]any{"span", map[string]any{}, before},
		[]any{"span", map[string]any{"highlight": color}, marked},
		[]any{"span", map[string]any{}, after}}})
	return string(encoded)
}

func TestCrossPlatformCoverageHighlightRequiresExactJSONML(t *testing.T) {
	for _, tc := range []struct {
		name, expected, actual, jsonml string
		want                           bool
	}{
		{"native highlight", "before ==important== after", highlightedMarkdown, highlightJSONML("before ", "important", " after", "#FFC000"), true},
		{"punctuation adjacent delimiters", "opaque==note==。", `opaque<span style="background-color: #FFC000;">note</span>。`, highlightJSONML("opaque", "note", "。", "#FFC000"), true},
		{"wrong range", "before ==important== after", highlightedMarkdown, highlightJSONML("before i", "mportant", " after", "#FFC000"), false},
		{"wrong color", "before ==important== after", highlightedMarkdown, highlightJSONML("before ", "important", " after", "#FF0000"), false},
		{"missing highlight", "before ==important== after", highlightedMarkdown, highlightJSONML("before ", "important", " after", ""), false},
		{"missing text", "before ==important== after", highlightedMarkdown, highlightJSONML("before ", "importan", " after", "#FFC000"), false},
		{"wrong exported text", "before ==important== after", strings.Replace(highlightedMarkdown, "important", "changed", 1), highlightJSONML("before ", "important", " after", "#FFC000"), false},
		{"missing JSONML", "before ==important== after", highlightedMarkdown, "", false},
		{"fragment JSONML", "before ==important== after", highlightedMarkdown, `["p",{},["span",{"highlight":"#FFC000"},"important"]]`, false},
		{"code literal", "`opaque==key==` and ==important==", "`opaque==key==` and <span style=\"background-color: #FFC000;\">important</span>", highlightJSONML("opaque==key== and ", "important", "", "#FFC000"), true},
		{"escaped literal", `\=\=opaque\=\= and ==important==`, `==opaque== and <span style="background-color: #FFC000;">important</span>`, highlightJSONML("==opaque== and ", "important", "", "#FFC000"), true},
		{"autolink after highlight", "before ==important== <https://example.com>", `before <span style="background-color: #FFC000;">important</span> <https://example.com>`, highlightJSONML("before ", "important", " https://example.com", "#FFC000"), true},
		{"autolink inside highlight", "before ==<https://example.com>== after", `before <span style="background-color: #FFC000;"><https://example.com></span> after`, highlightJSONML("before ", "https://example.com", " after", "#FFC000"), true},
		{"email autolink", "before ==important== <team@example.com>", `before <span style="background-color: #FFC000;">important</span> <team@example.com>`, highlightJSONML("before ", "important", " team@example.com", "#FFC000"), true},
		{"damaged code literal", "`opaque==key==` and ==important==", "`opaque==key==` and <span style=\"background-color: #FFC000;\">important</span>", highlightJSONML("opaquekey and ", "important", "", "#FFC000"), false},
		{"changed number", "before ==12== after", `before <span style="background-color: #FFC000;">13</span> after`, highlightJSONML("before ", "13", " after", "#FFC000"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := map[string]any{"markdown": tc.actual, tableAlignmentReadback: map[string]any{"jsonml": tc.jsonml}}
			if got := verifyUpdatedDocumentContent(value, tc.expected, "overwrite", "markdown"); got != tc.want {
				t.Fatalf("verified=%v, want %v", got, tc.want)
			}
		})
	}
	value := map[string]any{"markdown": highlightedMarkdown, tableAlignmentReadback: map[string]any{"jsonml": highlightJSONML("before ", "important", " after", "#FFC000"), "complete": false}}
	if verifyUpdatedDocumentContent(value, "before ==important== after", "overwrite", "markdown") {
		t.Fatal("incomplete JSONML accepted")
	}
	delete(value, tableAlignmentReadback)
	if verifyUpdatedDocumentContent(value, "before ==important== after", "overwrite", "markdown") {
		t.Fatal("Markdown alone accepted")
	}
	if !verifyUpdatedDocumentContent(map[string]any{"markdown": "opaque==；note==。"}, "opaque==；note==。", "overwrite", "markdown") {
		t.Fatal("non-opening punctuation delimiters must stay literal")
	}
}

func TestCrossPlatformCoverageHighlightAppendAndAlignedTables(t *testing.T) {
	value := map[string]any{"markdown": "old\n\n" + highlightedMarkdown}
	var root, paragraph []any
	_ = json.Unmarshal([]byte(highlightJSONML("before ", "important", " after", "#FFC000")), &paragraph)
	root = []any{"root", map[string]any{}, []any{"p", map[string]any{}, "old"}, paragraph[2]}
	encoded, _ := json.Marshal(root)
	value[tableAlignmentReadback] = map[string]any{"jsonml": string(encoded)}
	if !verifyUpdatedDocumentContent(value, "before ==important== after", "append", "markdown") {
		t.Fatal("appended highlight rejected")
	}
	if verifyUpdatedDocumentContent(value, "before ==important== after", "overwrite", "markdown") {
		t.Fatal("extra body accepted for overwrite")
	}
	for _, alignment := range []string{"right", "left"} {
		_ = json.Unmarshal([]byte(tableJSONML(alignment)), &root)
		root = append(root, paragraph[2])
		encoded, _ = json.Marshal(root)
		value = map[string]any{"markdown": exportedTable + "\n\n" + highlightedMarkdown, tableAlignmentReadback: map[string]any{"jsonml": string(encoded)}}
		if got := verifyUpdatedDocumentContent(value, alignedTable+"\n\nbefore ==important== after", "overwrite", "markdown"); got != (alignment == "right") {
			t.Fatalf("combined alignment=%s verified=%v", alignment, got)
		}
	}
}

func TestCrossPlatformCoverageHighlightCreateAndUpdateWriteOnce(t *testing.T) {
	testseam.Swap(t, &docVerifyWait, func(context.Context, time.Duration) error { return nil })
	testseam.Swap(t, &docVerifyDelays, []time.Duration{})
	for _, leaf := range []string{"create", "update"} {
		t.Run(leaf, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_content": {{"markdown": highlightedMarkdown}, {"jsonml": highlightJSONML("before ", "important", " after", "#FFC000")}}}}
			if leaf == "create" {
				runDocCoverageEnvelope(t, Create, caller, "--name", "Synthetic", "--content", "before ==important== after", "--yes")
			} else if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "overwrite", "--content", "before ==important== after", "--yes"); err != nil {
				t.Fatal(err)
			}
			writes, fullReads := 0, 0
			for _, call := range caller.history {
				if call.tool == "create_document" || call.tool == "update_document" {
					writes++
				}
				if call.tool == "get_document_content" && call.params["format"] == "jsonml" {
					fullReads++
				}
			}
			if writes != 1 || fullReads != 1 {
				t.Fatalf("writes=%d fullReads=%d", writes, fullReads)
			}
		})
	}
}

func TestCrossPlatformCoverageHighlightJSONMLFailureDoesNotRewrite(t *testing.T) {
	testseam.Swap(t, &docVerifyWait, func(context.Context, time.Duration) error { return nil })
	testseam.Swap(t, &docVerifyDelays, []time.Duration{})
	caller := &docCoverageCaller{failAt: 3, responses: map[string][]map[string]any{"get_document_content": {{"markdown": highlightedMarkdown}}}}
	if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "overwrite", "--content", "before ==important== after", "--yes"); err == nil {
		t.Fatal("failed full read accepted")
	}
	writes := 0
	for _, call := range caller.history {
		if call.tool == "update_document" {
			writes++
		}
	}
	if writes != 1 {
		t.Fatalf("writes=%d", writes)
	}
}

func TestCrossPlatformCoverageHighlightWordBoundaries(t *testing.T) {
	for _, tc := range []struct{ expected, actual string }{{"a b", "ab"}, {"ab", "a b"}} {
		value := map[string]any{"markdown": `<span style="background-color: #FFC000;">` + tc.expected + `</span>`, tableAlignmentReadback: map[string]any{"jsonml": highlightJSONML("", tc.actual, "", "#FFC000")}}
		if verifyUpdatedDocumentContent(value, "=="+tc.expected+"==", "overwrite", "markdown") {
			t.Fatal("JSONML word boundary mismatch accepted")
		}
	}
}

func TestCrossPlatformCoverageHighlightEntities(t *testing.T) {
	for _, tc := range []struct{ source, text string }{{"&copy", "&copy"}, {`\&amp;`, "&amp;"}, {"&amp;", "&"}, {"&#x41;", "A"}} {
		value := map[string]any{"markdown": highlightedMarkdown + " " + tc.source, tableAlignmentReadback: map[string]any{"jsonml": highlightJSONML("before ", "important", " after "+tc.text, "#FFC000")}}
		if !verifyUpdatedDocumentContent(value, "before ==important== after "+tc.source, "overwrite", "markdown") {
			t.Fatalf("valid entity/literal %q rejected", tc.source)
		}
	}
}

func TestCrossPlatformCoverageHighlightAppendAfterHighlightedParagraph(t *testing.T) {
	var old, fresh []any
	_ = json.Unmarshal([]byte(highlightJSONML("", "old", "", "#FFC000")), &old)
	_ = json.Unmarshal([]byte(highlightJSONML("", "new", "", "#FFC000")), &fresh)
	encoded, _ := json.Marshal(append(old, fresh[2]))
	value := map[string]any{"markdown": `<span style="background-color: #FFC000;">old</span>` + "\n\n" + `<span style="background-color: #FFC000;">new</span>`, tableAlignmentReadback: map[string]any{"jsonml": string(encoded)}}
	if !verifyUpdatedDocumentContent(value, "==new==", "append", "markdown") {
		t.Fatal("valid highlighted suffix rejected")
	}
}

func TestCrossPlatformCoverageHighlightKeepsOtherLargeExactPaths(t *testing.T) {
	large := strings.Repeat("x", docMarkdownVerifyMax+1)
	if !verifyUpdatedDocumentContent(map[string]any{"markdown": large}, large, "overwrite", "markdown") {
		t.Fatal("plain Markdown exact match was changed")
	}
	encoded, _ := json.Marshal([]any{"root", map[string]any{}, []any{"p", map[string]any{}, large}})
	if !verifyUpdatedDocumentContent(map[string]any{"jsonml": string(encoded)}, string(encoded), "overwrite", "jsonml") {
		t.Fatal("JSONML exact match was changed")
	}
}
