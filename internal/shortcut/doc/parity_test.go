package doc

import (
	"testing"
)

func TestCrossPlatformCoverageDocTagsUseDeclaredCSV(t *testing.T) {
	caller := &docCoverageCaller{}
	if err := runDocCoverage(t, Fetch, caller, "--node", "node-1", "--scope", "tags", "--tags", "h1,h2"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, call := range caller.history {
		if call.tool == "get_document_content" {
			found = true
			if got, ok := call.params["tags"].(string); !ok || got != "h1,h2" {
				t.Fatalf("tags must be CSV: %#v", call.params["tags"])
			}
		}
	}
	if !found {
		t.Fatal("missing content read")
	}
	for _, args := range [][]string{{"--node", "node-1", "--scope", "tags"}, {"--node", "node-1", "--scope", "tags", "--tags", " , "}} {
		c := &docCoverageCaller{}
		if err := runDocCoverage(t, Fetch, c, args...); err == nil || c.calls != 0 {
			t.Fatalf("empty tags: err=%v calls=%d", err, c.calls)
		}
	}
}

func TestCrossPlatformCoverageDocParityAliasesAndSearchDates(t *testing.T) {
	c := &docCoverageCaller{}
	if err := runDocCoverage(t, withDocParityAliases(Fetch), c, "--doc", "node-1"); err != nil {
		t.Fatal(err)
	}
	for _, call := range c.history {
		if call.tool == "get_document_content" && call.params["nodeId"] != "node-1" {
			t.Fatal("alias lost identity")
		}
	}
	c = &docCoverageCaller{}
	if err := runDocCoverage(t, withDocParityAliases(Fetch), c, "--doc", "other", "--node", "node-1"); err == nil || c.calls != 0 {
		t.Fatalf("alias conflict not rejected: %v", err)
	}
	c = &docCoverageCaller{}
	if err := runDocCoverage(t, withDocParityAliases(Search), c, "--query", "q", "--page-size", "2", "--created-after", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if c.history[0].params["pageSize"] != 2 || c.history[0].params["createdTimeFrom"] != 1767225600000 {
		t.Fatalf("request=%#v", c.history[0].params)
	}
	for _, s := range []string{"nonsense", "2026-99-01"} {
		if _, err := parseDocSearchTime(s); err == nil {
			t.Fatal("invalid date accepted")
		}
	}
}

func TestCrossPlatformCoverageDocChapterAndRegexExactBlocks(t *testing.T) {
	data := map[string]any{"jsonml": `["root",{},["h1",{"uuid":"a"},"Alpha"],["p",{"uuid":"b"},"known body"],["h2",{"uuid":"c"},"Nested"],["h1",{"uuid":"d"},"End"]]`}
	got, err := selectDocChapter(data, "a", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got["selectedBlockCount"] != 3 {
		t.Fatalf("chapter=%#v", got)
	}
	match, err := projectKeywordBlocks(data, "known.*body", true, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if match["count"] != 1 {
		t.Fatal("regex missing")
	}
	matches := match["matches"].([]map[string]any)
	if matches[0]["blockId"] != "b" || len(matches[0]["contextBlocks"].([]any)) != 3 {
		t.Fatal("wrong context")
	}
	zero, err := projectKeywordBlocks(data, "absent", false, 0, 0)
	if err != nil || zero["count"] != 0 {
		t.Fatal("not explicit empty")
	}
	for _, id := range []string{"missing", "b"} {
		if _, err := selectDocChapter(data, id, 0, 0); err == nil {
			t.Fatal("invalid chapter accepted")
		}
	}
	if _, err := documentTopBlocks(map[string]any{"jsonml": `["fragment",{}]`}); err == nil {
		t.Fatal("partial source accepted")
	}
}

func TestCrossPlatformCoverageDocScriptProfileAndConstraints(t *testing.T) {
	for _, tc := range []struct {
		content, format string
		blocks          int
	}{{"# Heading\n\nA body.", "markdown", 2}, {`["root",{},["h1",{},"标题"],["p",{},"正文"]]`, "jsonml", 2}} {
		p, err := docScriptProfile(tc.content, tc.format)
		if err != nil || p["block_count"] != tc.blocks {
			t.Fatalf("profile=%#v err=%v", p, err)
		}
	}
	if _, err := docScriptProfile("not json", "jsonml"); err == nil {
		t.Fatal("bad JSONML accepted")
	}
	caller := &docCoverageCaller{}
	if err := runDocCoverage(t, Script, caller, "--command", "parse", "--content", "body", "--required-blocks", "heading"); err == nil || caller.calls != 0 {
		t.Fatalf("failed structure passed: %v", err)
	}
	if docDefaultTitle("# A title\nbody", "markdown") != "A title" || docDefaultTitle("body", "markdown") != "未命名文档" {
		t.Fatal("title fallback")
	}
}
