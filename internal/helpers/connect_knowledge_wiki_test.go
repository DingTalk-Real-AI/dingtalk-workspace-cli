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
	"context"
	"errors"
	"io"
	"strings"
	"testing"
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
