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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// connect_knowledge_wiki extends the local-file knowledge retriever
// (connect_knowledge.go) to a DingTalk knowledge base (wiki) source. It does
// NOT reimplement any DingTalk API call: it reuses the existing `doc` tools that
// back `dws doc list` / `dws doc read` (list_nodes + get_document_content) via
// the shared executor.Runner, dumps each document as markdown text, and feeds
// that into the very same chunker/retriever the local-directory source uses.
//
// On startup the connector pulls the wiki space, caches it under
// ~/.dws/connect/<clientId>/knowledge/wiki-<spaceId>/, and builds the knowledge
// base from that directory. A pull failure (network / permission) degrades to a
// stale cache if one exists, otherwise to an empty knowledge base — it never
// blocks the connection.

// knowledgeSourceKind classifies a --knowledge-source value.
type knowledgeSourceKind int

const (
	// knowledgeSourceDir is a plain local directory (the legacy behavior, kept
	// for any value without a recognized scheme prefix).
	knowledgeSourceDir knowledgeSourceKind = iota
	// knowledgeSourceWiki is a wiki:<spaceId> source — a whole knowledge space.
	knowledgeSourceWiki
	// knowledgeSourceDoc is a doc:<docId> source — a single document node.
	knowledgeSourceDoc
)

var (
	wikiLoadKnowledgeBase = loadKnowledgeBase
	wikiRemoveAll         = os.RemoveAll
	wikiMkdirAll          = os.MkdirAll
	wikiWriteFile         = os.WriteFile
)

type knowledgeSource struct {
	kind knowledgeSourceKind
	// ref is the directory path (dir), the spaceId (wiki) or the nodeId (doc).
	ref string
}

// parseKnowledgeSource splits a --knowledge-source value into its kind and ref.
// "wiki:<spaceId>" / "doc:<docId>" select the DingTalk knowledge base; anything
// else (including bare paths) is treated as a local directory, so the legacy
// --knowledge-dir semantics keep working when routed through here.
func parseKnowledgeSource(raw string) (knowledgeSource, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return knowledgeSource{}, fmt.Errorf("empty knowledge source")
	}
	if rest, ok := cutKnowledgeScheme(value, "wiki"); ok {
		if rest == "" {
			return knowledgeSource{}, fmt.Errorf("knowledge source %q is missing a spaceId (expected wiki:<spaceId>)", raw)
		}
		return knowledgeSource{kind: knowledgeSourceWiki, ref: rest}, nil
	}
	if rest, ok := cutKnowledgeScheme(value, "doc"); ok {
		if rest == "" {
			return knowledgeSource{}, fmt.Errorf("knowledge source %q is missing a docId (expected doc:<docId>)", raw)
		}
		return knowledgeSource{kind: knowledgeSourceDoc, ref: rest}, nil
	}
	return knowledgeSource{kind: knowledgeSourceDir, ref: value}, nil
}

// cutKnowledgeScheme returns the part after "scheme:" and true when value starts
// with that scheme. The scheme match is case-insensitive; the ref is trimmed.
func cutKnowledgeScheme(value, scheme string) (string, bool) {
	prefix := scheme + ":"
	if len(value) < len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(value[len(prefix):]), true
}

// wikiNode is one document node listed under a wiki space.
type wikiNode struct {
	id   string
	name string
}

// wikiFetcher abstracts the two reused doc capabilities so the pull logic can be
// unit-tested with a fake (no real DingTalk calls). The real implementation
// (runnerWikiFetcher) drives executor.Runner.
type wikiFetcher interface {
	// listNodes returns the document nodes under a wiki space.
	listNodes(ctx context.Context, spaceID string) ([]wikiNode, error)
	// readContent returns a node's content as plain markdown text.
	readContent(ctx context.Context, nodeID string) (string, error)
}

// runnerWikiFetcher reuses the existing `doc` tools through the shared runner.
// It calls the exact RPCs that back `dws doc list` (list_nodes) and
// `dws doc read --content-format markdown` (get_document_content); no new API
// surface is added here.
type runnerWikiFetcher struct {
	runner executor.Runner
}

const (
	// wikiListPageLimit bounds pagination so a huge / misconfigured space cannot
	// spin the pull loop forever at startup.
	wikiListPageLimit = 50
	// wikiListPageSize is the per-page request size for list_nodes.
	wikiListPageSize = 50
)

func (f runnerWikiFetcher) listNodes(ctx context.Context, spaceID string) ([]wikiNode, error) {
	var nodes []wikiNode
	seen := map[string]struct{}{}
	pageToken := ""
	for page := 0; page < wikiListPageLimit; page++ {
		params := map[string]any{
			"workspaceId": spaceID,
			"pageSize":    wikiListPageSize,
		}
		if pageToken != "" {
			params["pageToken"] = pageToken
		}
		inv := executor.NewHelperInvocation("doc list", "doc", "list_nodes", params)
		res, err := f.runner.Run(ctx, inv)
		if err != nil {
			return nil, err
		}
		for _, n := range collectWikiNodes(res.Response) {
			if _, dup := seen[n.id]; dup {
				continue
			}
			seen[n.id] = struct{}{}
			nodes = append(nodes, n)
		}
		pageToken = firstStringForKeys(res.Response, "nextPageToken", "pageToken", "nextToken")
		if pageToken == "" {
			break
		}
	}
	return nodes, nil
}

func (f runnerWikiFetcher) readContent(ctx context.Context, nodeID string) (string, error) {
	inv := executor.NewHelperInvocation("doc read", "doc", "get_document_content", map[string]any{
		"nodeId": nodeID,
		"format": "markdown",
	})
	res, err := f.runner.Run(ctx, inv)
	if err != nil {
		return "", err
	}
	return firstStringForKeys(res.Response, "markdown", "content", "text", "data"), nil
}

// loadWikiKnowledgeBase pulls a wiki space (or a single doc node) into the local
// cache and builds the knowledge base from it. cacheRoot is typically
// ~/.dws/connect/<clientId>/knowledge. On a pull failure it falls back to an
// existing (stale) cache, and finally to an empty base — it never returns an
// error, so the connector keeps starting. Progress / warnings go to logw.
func loadWikiKnowledgeBase(ctx context.Context, fetcher wikiFetcher, src knowledgeSource, cacheRoot string, logw io.Writer) *knowledgeBase {
	if logw == nil {
		logw = io.Discard
	}
	cacheDir := filepath.Join(cacheRoot, wikiCacheLeaf(src))

	docs, err := pullWikiDocs(ctx, fetcher, src)
	if err != nil {
		// Pull failed: try the previous cache before giving up.
		if kb, lerr := wikiLoadKnowledgeBase(cacheDir); lerr == nil {
			fmt.Fprintf(logw, "[connect][knowledge] 拉取知识库失败（%v），使用旧缓存兜底：%s\n", err, cacheDir)
			return kb
		}
		fmt.Fprintf(logw, "[connect][knowledge] 拉取知识库失败（%v）且无可用缓存，以空知识起，不阻断连接\n", err)
		return &knowledgeBase{}
	}

	if writeErr := writeWikiCache(cacheDir, docs); writeErr != nil {
		// Cache write is best-effort: build straight from what we pulled so a
		// read-only / full disk does not break the feature.
		fmt.Fprintf(logw, "[connect][knowledge] 缓存写入失败（%v），改用内存构建\n", writeErr)
		return buildKnowledgeBaseFromDocs(docs)
	}

	kb, lerr := wikiLoadKnowledgeBase(cacheDir)
	if lerr != nil {
		// Cache wrote but produced no usable chunks (e.g. all docs empty):
		// degrade to an empty base rather than failing the connector.
		fmt.Fprintf(logw, "[connect][knowledge] 知识库无可用内容（%v），以空知识起\n", lerr)
		return &knowledgeBase{}
	}
	fmt.Fprintf(logw, "[connect][knowledge] 知识库已拉取并缓存：%d 个文档 / %d 个片段（%s）\n", len(docs), len(kb.chunks), cacheDir)
	return kb
}

// wikiDoc is one pulled document: a stable file stem plus its markdown text.
type wikiDoc struct {
	stem string
	text string
}

// pullWikiDocs fetches the documents for a source. For a wiki space it lists the
// nodes then reads each; for a single doc it reads that one node. Per-node read
// failures are skipped (logged by the caller via the returned set being smaller)
// rather than aborting the whole pull. A hard failure (list error, or zero
// readable docs) is returned so the caller can fall back to cache.
func pullWikiDocs(ctx context.Context, fetcher wikiFetcher, src knowledgeSource) ([]wikiDoc, error) {
	switch src.kind {
	case knowledgeSourceDoc:
		text, err := fetcher.readContent(ctx, src.ref)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("doc %s 内容为空", src.ref)
		}
		return []wikiDoc{{stem: sanitizeWikiStem(src.ref), text: text}}, nil
	case knowledgeSourceWiki:
		nodes, err := fetcher.listNodes(ctx, src.ref)
		if err != nil {
			return nil, err
		}
		if len(nodes) == 0 {
			return nil, fmt.Errorf("wiki space %s 下没有文档节点", src.ref)
		}
		var docs []wikiDoc
		for _, n := range nodes {
			text, rerr := fetcher.readContent(ctx, n.id)
			if rerr != nil || strings.TrimSpace(text) == "" {
				continue
			}
			stem := n.name
			if strings.TrimSpace(stem) == "" {
				stem = n.id
			}
			docs = append(docs, wikiDoc{stem: sanitizeWikiStem(stem), text: text})
		}
		if len(docs) == 0 {
			return nil, fmt.Errorf("wiki space %s 下没有可读文档", src.ref)
		}
		return docs, nil
	default:
		return nil, fmt.Errorf("source kind %d is not a wiki/doc source", src.kind)
	}
}

// buildKnowledgeBaseFromDocs chunks pulled docs directly into a knowledge base,
// reusing the same chunker/term-index as the local-file path. Used when the
// cache write fails but the pull succeeded.
func buildKnowledgeBaseFromDocs(docs []wikiDoc) *knowledgeBase {
	kb := &knowledgeBase{}
	for _, d := range docs {
		source := d.stem + ".md"
		for _, chunk := range splitKnowledgeChunks(d.text) {
			kb.chunks = append(kb.chunks, knowledgeChunk{source: source, text: chunk, terms: knowledgeTerms(chunk)})
		}
	}
	return kb
}

// writeWikiCache replaces the cache directory with the freshly pulled docs (one
// .md per document) so loadKnowledgeBase can index it like any local directory.
// It clears stale files first so deleted wiki docs do not linger in answers.
func writeWikiCache(cacheDir string, docs []wikiDoc) error {
	if err := wikiRemoveAll(cacheDir); err != nil {
		return err
	}
	if err := wikiMkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	used := map[string]int{}
	for _, d := range docs {
		stem := d.stem
		if stem == "" {
			stem = "doc"
		}
		// De-duplicate stems so two same-named docs do not overwrite each other.
		if n := used[stem]; n > 0 {
			used[stem] = n + 1
			stem = fmt.Sprintf("%s-%d", stem, n+1)
		} else {
			used[stem] = 1
		}
		path := filepath.Join(cacheDir, stem+".md")
		if err := wikiWriteFile(path, []byte(d.text), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// wikiCacheLeaf is the per-source cache subdirectory name, e.g. "wiki-<spaceId>"
// or "doc-<docId>", with the id sanitized for filesystem safety.
func wikiCacheLeaf(src knowledgeSource) string {
	prefix := "wiki"
	if src.kind == knowledgeSourceDoc {
		prefix = "doc"
	}
	return prefix + "-" + sanitizeWikiStem(src.ref)
}

// sanitizeWikiStem maps an id/name to a safe file stem: path separators and
// other awkward characters become '_', so a doc title or URL-ish id cannot
// escape the cache directory.
func sanitizeWikiStem(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "doc"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	stem := strings.Trim(b.String(), "._")
	if stem == "" {
		return "doc"
	}
	if len(stem) > 80 {
		stem = stem[:80]
	}
	return stem
}

// collectWikiNodes walks a list_nodes response and gathers every {id, name}
// node it can find. It is tolerant of wrapper layers (content/result/data) and
// of the exact id/name field spelling. A map is treated as a node only when it
// carries an id *directly* (shallow lookup, not the recursive extractor) —
// otherwise a wrapper object would be mistaken for a single node and sibling
// nodes in a list would be skipped.
func collectWikiNodes(resp map[string]any) []wikiNode {
	var nodes []wikiNode
	var walk func(any)
	walk = func(cur any) {
		switch x := cur.(type) {
		case map[string]any:
			if id := shallowStringForKeys(x, "nodeId", "nodeID", "node_id", "id"); id != "" {
				name := shallowStringForKeys(x, "name", "title", "nodeName")
				nodes = append(nodes, wikiNode{id: id, name: name})
				return
			}
			for _, v := range x {
				walk(v)
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(resp)
	return nodes
}

// shallowStringForKeys returns the first non-empty string value for any of keys
// at this map level only (no recursion), preserving key priority order.
func shallowStringForKeys(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// firstStringForKeys returns the first non-empty string found for any of keys by
// walking the value recursively (depth-first). It is the shared, source-shape
// tolerant extractor used for both node ids and document content.
func firstStringForKeys(v any, keys ...string) string {
	keySet := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		keySet[k] = struct{}{}
	}
	var walk func(any) string
	walk = func(cur any) string {
		switch x := cur.(type) {
		case map[string]any:
			for _, k := range keys {
				if s, ok := x[k].(string); ok && strings.TrimSpace(s) != "" {
					return s
				}
			}
			for key, val := range x {
				if _, direct := keySet[key]; direct {
					continue
				}
				if found := walk(val); found != "" {
					return found
				}
			}
		case []any:
			for _, item := range x {
				if found := walk(item); found != "" {
					return found
				}
			}
		}
		return ""
	}
	return walk(v)
}

// loadConnectKnowledgeSource resolves a --knowledge-source value and builds the
// knowledge base. A bare path (or dir: semantics) reuses the strict local-file
// loader (a typo'd directory stays loud, matching --knowledge-dir). A
// wiki:/doc: source pulls from the DingTalk knowledge base, caches it under
// ~/.dws/connect/<clientId>/knowledge, and never blocks the connection on a pull
// failure. A nil knowledge base means "leave any existing base untouched".
func loadConnectKnowledgeSource(cmd *cobra.Command, runner executor.Runner, clientID, raw string) (*knowledgeBase, error) {
	src, err := parseKnowledgeSource(raw)
	if err != nil {
		return nil, err
	}
	logw := cmd.ErrOrStderr()
	if src.kind == knowledgeSourceDir {
		kb, lerr := loadKnowledgeBase(src.ref)
		if lerr != nil {
			return nil, lerr
		}
		fmt.Fprintf(logw, "[connect] 知识库已加载：%d 个片段（%s）\n", len(kb.chunks), src.ref)
		return kb, nil
	}
	fetcher := runnerWikiFetcher{runner: runner}
	cacheRoot := connectKnowledgeCacheRoot(clientID)
	return loadWikiKnowledgeBase(cmd.Context(), fetcher, src, cacheRoot, logw), nil
}

// connectKnowledgeCacheRoot is the per-robot knowledge cache root:
// ~/.dws/connect/<clientId>/knowledge. clientId is sanitized so it cannot escape
// the directory; an empty clientId degrades to a shared "default" bucket.
func connectKnowledgeCacheRoot(clientID string) string {
	id := sanitizeWikiStem(clientID)
	return filepath.Join(homeDir(), ".dws", "connect", id, "knowledge")
}
