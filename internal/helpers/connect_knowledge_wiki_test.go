// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// fakeWikiFetcher is an in-test wikiFetcher: no DingTalk calls, fully scripted.
type fakeWikiFetcher struct {
	nodes      []wikiNode
	content    map[string]string
	listErr    error
	readErrFor map[string]error
}

func (f fakeWikiFetcher) listNodes(_ context.Context, _ string) ([]wikiNode, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.nodes, nil
}

func (f fakeWikiFetcher) readContent(_ context.Context, nodeID string) (string, error) {
	if f.readErrFor != nil {
		if err, ok := f.readErrFor[nodeID]; ok {
			return "", err
		}
	}
	return f.content[nodeID], nil
}

// ① --knowledge-source syntax parses wiki:<spaceId> / doc:<docId> correctly.
func TestParseKnowledgeSource(t *testing.T) {
	cases := []struct {
		raw      string
		wantKind knowledgeSourceKind
		wantRef  string
		wantErr  bool
	}{
		{"wiki:SPACE123", knowledgeSourceWiki, "SPACE123", false},
		{"  wiki:SPACE123  ", knowledgeSourceWiki, "SPACE123", false},
		{"WIKI:Sp", knowledgeSourceWiki, "Sp", false}, // scheme is case-insensitive
		{"doc:NODE9", knowledgeSourceDoc, "NODE9", false},
		{"/var/knowledge", knowledgeSourceDir, "/var/knowledge", false},
		{"./local", knowledgeSourceDir, "./local", false},
		{"wiki:", 0, "", true},
		{"doc:", 0, "", true},
		{"", 0, "", true},
	}
	for _, c := range cases {
		got, err := parseKnowledgeSource(c.raw)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseKnowledgeSource(%q) expected error, got %+v", c.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseKnowledgeSource(%q) unexpected error: %v", c.raw, err)
			continue
		}
		if got.kind != c.wantKind || got.ref != c.wantRef {
			t.Errorf("parseKnowledgeSource(%q) = {%d,%q}, want {%d,%q}", c.raw, got.kind, got.ref, c.wantKind, c.wantRef)
		}
	}
}

// ③ A pulled wiki space lands in the retriever and the right chunk is retrieved.
func TestLoadWikiKnowledgeBaseRetrieves(t *testing.T) {
	fetcher := fakeWikiFetcher{
		nodes: []wikiNode{
			{id: "n1", name: "安装指南"},
			{id: "n2", name: "缓存问题"},
		},
		content: map[string]string{
			"n1": "# 安装\n\ndws 通过 brew install dws 安装。",
			"n2": "# 缓存问题\n\n服务发现已下线。出现 endpoint_not_resolved 时，检查静态端点目录并升级到包含该能力的 dws 版本。",
		},
	}
	src := knowledgeSource{kind: knowledgeSourceWiki, ref: "SPACE123"}
	kb := loadWikiKnowledgeBase(context.Background(), fetcher, src, t.TempDir(), io.Discard)
	if kb == nil || len(kb.chunks) == 0 {
		t.Fatalf("expected a non-empty knowledge base, got %+v", kb)
	}
	got := kb.augment("endpoint_not_resolved 是干什么的")
	if !strings.Contains(got, "静态端点目录") {
		t.Fatalf("augment missed the cache chunk pulled from wiki:\n%s", got)
	}
}

// ③ A doc:<docId> source pulls a single node into the retriever.
func TestLoadWikiKnowledgeBaseSingleDoc(t *testing.T) {
	fetcher := fakeWikiFetcher{
		content: map[string]string{
			"NODE9": "# 机器人建联\n\n卡片停在数据加载中说明回复超过了 AI 助理应答窗口。",
		},
	}
	src := knowledgeSource{kind: knowledgeSourceDoc, ref: "NODE9"}
	kb := loadWikiKnowledgeBase(context.Background(), fetcher, src, t.TempDir(), io.Discard)
	if kb == nil || len(kb.chunks) == 0 {
		t.Fatalf("expected a non-empty knowledge base, got %+v", kb)
	}
	if got := kb.augment("数据加载中怎么办"); !strings.Contains(got, "AI 助理应答窗口") {
		t.Fatalf("augment missed the single doc chunk:\n%s", got)
	}
}

// ② Pull failure with no prior cache degrades to an empty (non-nil) base and
// does not panic.
func TestLoadWikiKnowledgeBaseFailNoCacheEmpty(t *testing.T) {
	fetcher := fakeWikiFetcher{listErr: errors.New("network down")}
	src := knowledgeSource{kind: knowledgeSourceWiki, ref: "SPACE123"}
	kb := loadWikiKnowledgeBase(context.Background(), fetcher, src, t.TempDir(), io.Discard)
	if kb == nil {
		t.Fatal("expected a non-nil empty base, got nil")
	}
	if len(kb.chunks) != 0 {
		t.Fatalf("expected empty base on failure, got %d chunks", len(kb.chunks))
	}
	// augment on an empty base must pass the question through unchanged.
	if got := kb.augment("任意问题"); got != "任意问题" {
		t.Fatalf("empty base should pass through, got %q", got)
	}
}

// ② Pull failure falls back to a previously written (stale) cache.
func TestLoadWikiKnowledgeBaseFailUsesStaleCache(t *testing.T) {
	cacheRoot := t.TempDir()
	src := knowledgeSource{kind: knowledgeSourceWiki, ref: "SPACE123"}

	// First pull succeeds and populates the cache.
	good := fakeWikiFetcher{
		nodes:   []wikiNode{{id: "n1", name: "缓存问题"}},
		content: map[string]string{"n1": "# 缓存问题\n\n服务发现已下线。出现 endpoint_not_resolved 时，检查静态端点目录并升级到包含该能力的 dws 版本。"},
	}
	if kb := loadWikiKnowledgeBase(context.Background(), good, src, cacheRoot, io.Discard); len(kb.chunks) == 0 {
		t.Fatal("warm-up pull should have populated the cache")
	}

	// Second pull fails; must fall back to the stale cache, not go empty.
	bad := fakeWikiFetcher{listErr: errors.New("permission denied")}
	kb := loadWikiKnowledgeBase(context.Background(), bad, src, cacheRoot, io.Discard)
	if kb == nil || len(kb.chunks) == 0 {
		t.Fatalf("expected stale-cache fallback to keep chunks, got %+v", kb)
	}
	if got := kb.augment("endpoint_not_resolved"); !strings.Contains(got, "静态端点目录") {
		t.Fatalf("stale cache content not retrievable:\n%s", got)
	}
}

// Per-node read failures are skipped without aborting the whole pull.
func TestPullWikiDocsSkipsUnreadableNodes(t *testing.T) {
	fetcher := fakeWikiFetcher{
		nodes: []wikiNode{{id: "ok", name: "好文档"}, {id: "bad", name: "坏文档"}},
		content: map[string]string{
			"ok":  "# 好文档\n\n这是可读内容。",
			"bad": "",
		},
		readErrFor: map[string]error{"bad": errors.New("403")},
	}
	docs, err := pullWikiDocs(context.Background(), fetcher, knowledgeSource{kind: knowledgeSourceWiki, ref: "S"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 1 || !strings.Contains(docs[0].text, "可读内容") {
		t.Fatalf("expected only the readable doc, got %+v", docs)
	}
}

// collectWikiNodes is tolerant of wrapper layers and id/name field spellings.
func TestCollectWikiNodesTolerant(t *testing.T) {
	resp := map[string]any{
		"content": map[string]any{
			"nodes": []any{
				map[string]any{"nodeId": "a", "name": "Alpha"},
				map[string]any{"id": "b", "title": "Beta"},
			},
		},
	}
	nodes := collectWikiNodes(resp)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d (%+v)", len(nodes), nodes)
	}
	if nodes[0].id != "a" || nodes[0].name != "Alpha" || nodes[1].id != "b" || nodes[1].name != "Beta" {
		t.Fatalf("node extraction wrong: %+v", nodes)
	}
}

func TestSanitizeWikiStem(t *testing.T) {
	cases := map[string]string{
		"SPACE123":               "SPACE123",
		"a/b/../c":               "a_b_.._c",
		"https://x/nodes/N9?q=1": "https___x_nodes_N9_q_1",
		"":                       "doc",
		"...":                    "doc",
	}
	for in, want := range cases {
		if got := sanitizeWikiStem(in); got != want {
			t.Errorf("sanitizeWikiStem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRunnerWikiFetcherCoverage(t *testing.T) {
	runner := &onboardSeqRunner{responses: []map[string]any{
		{
			"result": map[string]any{"nodes": []any{
				map[string]any{"nodeId": "n1", "name": "One"},
				map[string]any{"nodeId": "", "name": "empty"},
			}},
			"nextPageToken": "next",
		},
		{"data": []any{
			map[string]any{"nodeId": "n1", "name": "duplicate"},
			map[string]any{"id": "n2", "title": "Two"},
		}},
	}}
	fetcher := runnerWikiFetcher{runner: runner}
	nodes, err := fetcher.listNodes(context.Background(), "space")
	if err != nil || len(nodes) != 2 || nodes[1].id != "n2" {
		t.Fatalf("listNodes() = %#v, %v", nodes, err)
	}
	reader := runnerWikiFetcher{runner: connectResponseRunner{response: map[string]any{"result": map[string]any{"markdown": "# body"}}}}
	if text, err := reader.readContent(context.Background(), "n1"); err != nil || text != "# body" {
		t.Fatalf("readContent() = %q, %v", text, err)
	}

	failed := runnerWikiFetcher{runner: connectResponseRunner{err: errors.New("runner failed")}}
	if _, err := failed.listNodes(context.Background(), "space"); err == nil {
		t.Fatal("listNodes runner error was ignored")
	}
	if _, err := failed.readContent(context.Background(), "node"); err == nil {
		t.Fatal("readContent runner error was ignored")
	}
}

func TestWikiKnowledgeUtilityCoverage(t *testing.T) {
	kb := buildKnowledgeBaseFromDocs([]wikiDoc{{stem: "one", text: "# title\nbody"}, {stem: "empty", text: " "}})
	if len(kb.chunks) == 0 {
		t.Fatal("in-memory knowledge base is empty")
	}

	values := []any{
		map[string]any{"ignored": map[string]any{"content": "nested"}},
		[]any{map[string]any{"text": "array"}},
		map[string]any{"content": " direct "},
		map[string]any{"content": "", "other": "ignored"},
	}
	wants := []string{"nested", "array", " direct ", ""}
	for i, value := range values {
		if got := firstStringForKeys(value, "content", "text"); got != wants[i] {
			t.Fatalf("firstStringForKeys case %d = %q, want %q", i, got, wants[i])
		}
	}

	cache := filepath.Join(t.TempDir(), "cache")
	if err := writeWikiCache(cache, []wikiDoc{
		{stem: "same", text: "one"},
		{stem: "same", text: "two"},
		{stem: "", text: "three"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"same.md", "same-2.md", "doc.md"} {
		if _, err := os.Stat(filepath.Join(cache, name)); err != nil {
			t.Fatalf("missing cached file %s: %v", name, err)
		}
	}
	if leaf := wikiCacheLeaf(knowledgeSource{kind: knowledgeSourceDoc, ref: "a/b"}); leaf != "doc-a_b" {
		t.Fatalf("wikiCacheLeaf() = %q", leaf)
	}
	if got := sanitizeWikiStem(strings.Repeat("a", 100)); len(got) != 80 {
		t.Fatalf("long stem length = %d", len(got))
	}
}

func TestLoadConnectKnowledgeSourceCoverage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "knowledge.md"), []byte("# Local\nlocal body"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{Use: "connect"}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	if kb, err := loadConnectKnowledgeSource(cmd, connectResponseRunner{}, "client", dir); err != nil || len(kb.chunks) == 0 {
		t.Fatalf("local source = %#v, %v", kb, err)
	}
	if _, err := loadConnectKnowledgeSource(cmd, connectResponseRunner{}, "client", t.TempDir()); err == nil {
		t.Fatal("empty local source succeeded")
	}
	if _, err := loadConnectKnowledgeSource(cmd, connectResponseRunner{}, "client", ""); err == nil {
		t.Fatal("empty source succeeded")
	}

	t.Setenv("HOME", t.TempDir())
	runner := connectResponseRunner{response: map[string]any{"markdown": "# Remote\nremote body"}}
	if kb, err := loadConnectKnowledgeSource(cmd, runner, "../client", "doc:node"); err != nil || len(kb.chunks) == 0 {
		t.Fatalf("remote source = %#v, %v", kb, err)
	}
	root := connectKnowledgeCacheRoot("../client")
	if !strings.Contains(root, filepath.Join("connect", "client", "knowledge")) {
		t.Fatalf("cache root = %q", root)
	}
}
