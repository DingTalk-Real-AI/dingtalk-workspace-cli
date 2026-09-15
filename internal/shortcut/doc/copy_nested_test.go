// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package doc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageDocCopyNestedSourcesAndAnchors(t *testing.T) {
	const source = `["p",{"uuid":"source"},["span",{"bold":true},"nested text"]]`
	const clone = `["p",{"uuid":"new"},["span",{"bold":true},"nested text"]]`
	const anchor = `["blockquote",{"uuid":"ref"},["p",{"uuid":"anchor-child"},"keep"]]`
	const before = `["root",{},["ul",{"uuid":"list"},["li",{"uuid":"item"},` + source + `]],` + anchor + `]`
	for _, tc := range []struct{ name, ref, after string }{
		{"nested-source", "ref", `["root",{},["ul",{"uuid":"list"},["li",{"uuid":"item"},` + source + `]],` + anchor + `,` + clone + `]`},
		{"nested-anchor", "anchor-child", `["root",{},["ul",{"uuid":"list"},["li",{"uuid":"item"},` + source + `]],["blockquote",{"uuid":"ref"},["p",{"uuid":"anchor-child"},"keep"],` + clone + `]]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tree []any
			if err := json.Unmarshal([]byte(tc.after), &tree); err != nil {
				t.Fatal(err)
			}
			entries := []any{}
			for _, block := range tree[2:] {
				encoded, _ := json.Marshal(block)
				entries = append(entries, map[string]any{"jsonml": string(encoded)})
			}
			c := &docCoverageCaller{responses: map[string][]map[string]any{
				"get_document_content":  {{"jsonml": before}},
				"insert_document_block": {{"blockId": "new"}},
				"list_document_blocks":  {{"blocks": entries, "hasMore": false}},
			}}
			result := runDocCoverageEnvelope(t, Update, c, "--node", "n", "--command", "block_copy_insert_after", "--block-id", "source", "--after-block-id", tc.ref, "--yes")
			if result["operation"] != "doc.update" || len(c.history) != 3 {
				t.Fatal(result, c.history)
			}
			insert := c.history[1]
			if insert.tool != "insert_document_block" || insert.params["nodeId"] != "n" || insert.params["referenceBlockId"] != tc.ref || insert.params["where"] != "after" || insert.params["format"] != "jsonml" {
				t.Fatal(insert)
			}
			var actual []any
			if err := json.Unmarshal([]byte(insert.params["jsonml"].(string)), &actual); err != nil {
				t.Fatal(err)
			}
			if jsonMLBlockIdentity(actual) != "" || canonicalBlockContent(actual, "jsonml") != canonicalBlockContent(source, "jsonml") {
				t.Fatal(actual)
			}
		})
	}
	for _, tc := range []struct{ name, ids, ref, body, reason string }{
		{"multi-stays-top-level", "source,ref", "ref", before, "复制源块不存在"},
		{"missing-source", "missing", "ref", before, "复制源块不存在"},
		{"missing-anchor", "source", "missing", before, "复制目标锚点不存在"},
		{"nested-resource", "source", "ref", strings.Replace(before, `["span",{"bold":true},"nested text"]`, `["img",{"src":"https://example.com/x"}]`, 1), "含资源引用"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_content": {{"jsonml": tc.body}}}}
			err := runDocCoverage(t, Update, c, "--node", "n", "--command", "block_copy_insert_after", "--block-id", tc.ids, "--after-block-id", tc.ref, "--yes")
			if err == nil || !strings.Contains(err.Error(), tc.reason) || len(c.history) != 1 {
				t.Fatal(err, c.history)
			}
		})
	}
}

func TestCrossPlatformCoverageDocCopyRejectsFalsePositiveReadback(t *testing.T) {
	const anchor = `["blockquote",{"uuid":"ref"},["p",{"uuid":"child"},"keep"]]`
	const clone = `["p",{"uuid":"new"},"copied"]`
	expected := canonicalBlockContent(clone, "jsonml")
	for _, tc := range []struct {
		body, id string
		want     bool
	}{
		{`["root",{},` + anchor + `,` + clone + `]`, "new", true},
		{`["root",{},["blockquote",{"uuid":"ref"},` + clone + `]]`, "new", false},
		{`["root",{},` + clone + `,` + anchor + `]`, "new", false},
		{`["root",{},` + anchor + `,` + clone + `]`, "wrong", false},
		{`["root",{},` + anchor + `,` + clone + `]`, "", true},
		{`["root",{},` + anchor + `,` + clone + `]`, "ref", false},
		{`["root",{},` + anchor + `,["p",{"uuid":"new"},"wrong"]]`, "new", false},
		{`invalid-json`, "new", false},
	} {
		if got := verifyDocCopySibling(map[string]any{"blockId": tc.id}, map[string]any{"jsonml": tc.body}, "ref", expected); got != tc.want {
			t.Fatalf("%s/%s: %v", tc.body, tc.id, got)
		}
	}
	if verifyDocCopySibling(map[string]any{"blockId": "new"}, map[string]any{"blocks": []any{map[string]any{"jsonml": "broken"}, map[string]any{"jsonml": clone}}}, "ref", expected) {
		t.Fatal("invalid sibling readback accepted")
	}
}
