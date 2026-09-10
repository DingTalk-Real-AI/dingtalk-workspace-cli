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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDocParityValidationStopsBeforeNetwork(t *testing.T) {
	cases := []struct {
		s    shortcut.Shortcut
		args []string
	}{
		{Fetch, []string{"--node", "n", "--regex"}},
		{Fetch, []string{"--node", "n", "--scope", "keyword", "--keyword", "x", "--regex", "--context-unit", "characters"}},
		{Fetch, []string{"--node", "n", "--scope", "chapter", "--start-block-id", "x", "--max-depth", "1"}},
		{Fetch, []string{"--node", "n", "--scope", "keyword", "--keyword", "x", "--context-before", "-1", "--context-unit", "blocks"}},
		{Search, []string{"--query", "q", "--folder", ""}},
		{Search, []string{"--query", "q", "--folder", "f"}},
		{Search, []string{"--query", "q", "--folder", "f", "--page-all", "--cursor", "c"}},
		{Search, []string{"--query", "q", "--created-after", "2026-01-01", "--created-from", "1"}},
		{Search, []string{"--query", "q", "--created-after", "2026-02-01", "--created-before", "2026-01-01"}},
		{Script, []string{"--command", "parse"}},
		{Script, []string{"--command", "parse", "--content", "x", "--node", "n"}},
		{Script, []string{"--command", "parse", "--content", "x", "--min-words", "-1"}},
		{Script, []string{"--command", "parse", "--content", "x", "--min-words", "2", "--max-words", "1"}},
		{Script, []string{"--command", "init-draft", "--node", "n"}},
		{MediaUpload, []string{"--node", "n", "--file", "../bad", "--yes"}},
		{MediaPreview, []string{"--node", "n", "--resource-id", "12345678-1234-1234-1234-123456789012", "--overwrite"}},
		{Update, []string{"--node", "n", "--command", "block_delete", "--start-block-id", "a", "--yes"}},
		{Update, []string{"--node", "n", "--command", "block_delete", "--start-block-id", "a", "--end-block-id", "b", "--block-id", "c", "--yes"}},
		{Update, []string{"--node", "n", "--command", "append", "--content", "x", "--start-block-id", "a", "--end-block-id", "b", "--yes"}},
		{MediaInsert, []string{"--node", "n", "--from-clipboard", "--file", "x", "--yes"}},
		{ResourceUpdate, []string{"--node", "n", "--from-clipboard", "--image", "https://example.com/x.png", "--yes"}},
	}
	for _, tc := range cases {
		t.Run(tc.s.Command+strings.Join(tc.args, "_"), func(t *testing.T) {
			c := &docCoverageCaller{}
			if err := runDocCoverage(t, withDocParityAliases(tc.s), c, tc.args...); err == nil || c.calls != 0 {
				t.Fatalf("error=%v calls=%d", err, c.calls)
			}
		})
	}
	for _, raw := range []string{"0", "1767225600000", "2026-01-01", "2026-01-01T08:00:00+08:00"} {
		if _, err := parseDocSearchTime(raw); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCrossPlatformCoverageDocRangesRejectUnprovenSources(t *testing.T) {
	data := map[string]any{"jsonml": `["root",{},["h1",{"uuid":"a"},"Title"],["p",{"uuid":"b"},"Body"],["h1",{"uuid":"c"},"End"]]`}
	for _, pair := range [][2]string{{"a", "b"}, {"0", "-1"}, {"b", "b"}} {
		ids, err := docRangeIDs(data, pair[0], pair[1])
		if err != nil || len(ids) == 0 {
			t.Fatal(ids, err)
		}
	}
	for _, pair := range [][2]string{{"b", "a"}, {"missing", "b"}, {"a", "missing"}} {
		if _, err := docRangeIDs(data, pair[0], pair[1]); err == nil {
			t.Fatal("bad range accepted")
		}
	}
	for _, raw := range []string{`[]`, `["fragment",{}]`, `["root",{},true]`, `["root",{},["p",{}]]`, `["root",{},["p",{"uuid":"a"}],["p",{"uuid":"a"}]]`} {
		if _, err := documentTopBlocks(map[string]any{"jsonml": raw}); err == nil {
			t.Fatal("unproven source accepted", raw)
		}
	}
	for _, raw := range []string{"a,a", "a,", strings.Repeat("a,", 21)} {
		if _, err := docCopyIDs(raw); err == nil {
			t.Fatal("invalid copy IDs")
		}
	}
	if ids, err := docCopyIDs(" a , b "); err != nil || strings.Join(ids, ",") != "a,b" {
		t.Fatal(ids, err)
	}
	if _, err := projectKeywordBlocks(data, "[", true, 0, 0); err == nil {
		t.Fatal("invalid regex")
	}
	if got, err := selectDocChapter(data, "a", 4, 4); err != nil || got["selectedBlockCount"] != 3 {
		t.Fatal(got, err)
	}
}

func TestCrossPlatformCoverageDocCommentAggregationRequiresStableExplicitResults(t *testing.T) {
	for _, value := range []map[string]any{{}, {"comments": []any{true}}, {"comments": []any{map[string]any{"text": "missing key"}}}} {
		c := &docCoverageCaller{responses: map[string][]map[string]any{"list_comments": {value}}}
		if err := runDocCoverage(t, Fetch, c, "--node", "node-1", "--include-comments"); err == nil {
			t.Fatal("malformed comments treated as empty")
		}
	}
	c := &docCoverageCaller{responses: map[string][]map[string]any{"list_comments": {
		{"comments": []any{map[string]any{"commentKey": "one"}}, "hasMore": true, "nextToken": "next"},
		{"comments": []any{map[string]any{"commentKey": "two"}}, "hasMore": false},
	}}}
	result := runDocCoverageEnvelope(t, Fetch, c, "--node", "node-1", "--include-comments")
	encoded, _ := json.Marshal(result)
	text := string(encoded)
	if !strings.Contains(text, "one") || !strings.Contains(text, "two") {
		t.Fatal("comments lost")
	}
	found := false
	for _, call := range c.history {
		if call.tool == "list_comments" && call.params["nextToken"] == "next" {
			found = true
		}
	}
	if !found {
		t.Fatal("cursor lost")
	}
}

func TestCrossPlatformCoverageDocClipboardFailureDryRunAndCleanup(t *testing.T) {
	reads := 0
	testseam.Swap(t, &docClipboardImage, func(context.Context) ([]byte, error) { reads++; return nil, errors.New("no image") })
	c := &docCoverageCaller{dryRun: true}
	if err := runDocCoverage(t, MediaInsert, c, "--node", "n", "--from-clipboard", "--dry-run"); err != nil || reads != 0 || c.calls != 0 {
		t.Fatalf("dry run err=%v reads=%d calls=%d", err, reads, c.calls)
	}
	c = &docCoverageCaller{}
	if err := runDocCoverage(t, MediaInsert, c, "--node", "n", "--from-clipboard", "--yes"); err == nil || c.calls != 0 {
		t.Fatal("clipboard failure reached RPC")
	}
	t.Chdir(t.TempDir())
	testseam.Swap(t, &docClipboardImage, func(context.Context) ([]byte, error) { return []byte("fixture"), nil })
	c = &docCoverageCaller{failAt: 1}
	if err := runDocCoverage(t, MediaInsert, c, "--node", "n", "--from-clipboard", "--yes"); err == nil {
		t.Fatal("expected failure")
	}
	paths, _ := filepath.Glob(".dws-clipboard-*")
	if len(paths) != 0 {
		t.Fatal("temporary clipboard file leaked")
	}
	var buffer limitedClipboardBuffer
	if _, err := buffer.Write(bytes.Repeat([]byte("x"), docClipboardLimit*2+1025)); err == nil {
		t.Fatal("unbounded clipboard")
	}
}

func TestCrossPlatformCoverageDocCreateMediaPreflightAndLocalDraft(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("ok.txt", []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("empty.txt", nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"missing", "empty.txt", "ok.txt,ok.txt", "../escape"} {
		c := &docCoverageCaller{}
		if err := runDocCoverage(t, Create, c, "--content", "body", "--media-files", file); err == nil || c.calls != 0 {
			t.Fatalf("preflight %s err=%v calls=%d", file, err, c.calls)
		}
	}
	c := &docCoverageCaller{dryRun: true}
	if err := runDocCoverage(t, Create, c, "--content", "body", "--media-files", "ok.txt", "--dry-run"); err != nil || c.calls != 0 {
		t.Fatal(err, c.calls)
	}
	for _, format := range []string{"markdown", "jsonml"} {
		ctx, _ := output.WithResultStore(context.Background())
		if err := runDocCoverage(t, Script, &docCoverageCaller{ctx: ctx}, "--command", "init-draft", "--doc-format", format); err != nil {
			t.Fatal(err)
		}
	}
	paths, _ := filepath.Glob("doc-draft-*/*")
	if len(paths) != 2 {
		t.Fatal("draft files absent")
	}
}
