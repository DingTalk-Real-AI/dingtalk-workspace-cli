package helpers

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestCrossPlatformCoverageWikiPullRemainingCoverage(t *testing.T) {
	ctx := context.Background()
	if _, err := pullWikiDocs(ctx, fakeWikiFetcher{readErrFor: map[string]error{"doc": errors.New("read")}}, knowledgeSource{kind: knowledgeSourceDoc, ref: "doc"}); err == nil {
		t.Fatal("single doc read error returned nil")
	}
	if _, err := pullWikiDocs(ctx, fakeWikiFetcher{content: map[string]string{"doc": " "}}, knowledgeSource{kind: knowledgeSourceDoc, ref: "doc"}); err == nil {
		t.Fatal("empty single doc returned nil")
	}
	if _, err := pullWikiDocs(ctx, fakeWikiFetcher{listErr: errors.New("list")}, knowledgeSource{kind: knowledgeSourceWiki, ref: "space"}); err == nil {
		t.Fatal("wiki list error returned nil")
	}
	if _, err := pullWikiDocs(ctx, fakeWikiFetcher{}, knowledgeSource{kind: knowledgeSourceWiki, ref: "space"}); err == nil {
		t.Fatal("empty wiki node list returned nil")
	}
	if _, err := pullWikiDocs(ctx, fakeWikiFetcher{nodes: []wikiNode{{id: "node"}}, content: map[string]string{"node": " "}}, knowledgeSource{kind: knowledgeSourceWiki, ref: "space"}); err == nil {
		t.Fatal("wiki without readable docs returned nil")
	}
	docs, err := pullWikiDocs(ctx, fakeWikiFetcher{nodes: []wikiNode{{id: "node"}}, content: map[string]string{"node": "body"}}, knowledgeSource{kind: knowledgeSourceWiki, ref: "space"})
	if err != nil || len(docs) != 1 || docs[0].stem != "node" {
		t.Fatalf("unnamed node docs = %#v, %v", docs, err)
	}
	if _, err := pullWikiDocs(ctx, fakeWikiFetcher{}, knowledgeSource{kind: knowledgeSourceDir, ref: "dir"}); err == nil {
		t.Fatal("local source passed to wiki pull returned nil")
	}
}

func TestCrossPlatformCoverageWikiCacheFailureRemainingCoverage(t *testing.T) {
	originalLoad := wikiLoadKnowledgeBase
	originalRemove := wikiRemoveAll
	originalMkdir := wikiMkdirAll
	originalWrite := wikiWriteFile
	t.Cleanup(func() {
		wikiLoadKnowledgeBase = originalLoad
		wikiRemoveAll = originalRemove
		wikiMkdirAll = originalMkdir
		wikiWriteFile = originalWrite
	})
	boom := errors.New("cache failure")
	docs := []wikiDoc{{stem: "doc", text: "body"}}

	wikiRemoveAll = func(string) error { return boom }
	if err := writeWikiCache(t.TempDir(), docs); err == nil {
		t.Fatal("remove cache error returned nil")
	}
	wikiRemoveAll = func(string) error { return nil }
	wikiMkdirAll = func(string, os.FileMode) error { return boom }
	if err := writeWikiCache(t.TempDir(), docs); err == nil {
		t.Fatal("mkdir cache error returned nil")
	}
	wikiMkdirAll = func(string, os.FileMode) error { return nil }
	wikiWriteFile = func(string, []byte, os.FileMode) error { return boom }
	if err := writeWikiCache(t.TempDir(), docs); err == nil {
		t.Fatal("write cache error returned nil")
	}

	fetcher := fakeWikiFetcher{content: map[string]string{"doc": "body"}}
	wikiRemoveAll = func(string) error { return boom }
	wikiMkdirAll = originalMkdir
	wikiWriteFile = originalWrite
	if kb := loadWikiKnowledgeBase(context.Background(), fetcher, knowledgeSource{kind: knowledgeSourceDoc, ref: "doc"}, t.TempDir(), nil); len(kb.chunks) == 0 {
		t.Fatal("write failure did not build in memory")
	}
	wikiRemoveAll = originalRemove
	wikiLoadKnowledgeBase = func(string) (*knowledgeBase, error) { return nil, boom }
	if kb := loadWikiKnowledgeBase(context.Background(), fetcher, knowledgeSource{kind: knowledgeSourceDoc, ref: "doc"}, t.TempDir(), nil); kb == nil || len(kb.chunks) != 0 {
		t.Fatalf("load failure base = %#v", kb)
	}
}
