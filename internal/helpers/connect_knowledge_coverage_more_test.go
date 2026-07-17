package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageKnowledgeLoadAndAugmentRemainingCoverage(t *testing.T) {
	if _, err := loadKnowledgeBase(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing knowledge directory returned nil")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".hidden"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden", "secret.md"), []byte("hidden"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "visible.md"), []byte("# First\nbody\n# Second\n"+strings.Repeat("x", knowledgeMaxChunkRunes)), 0o600); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(dir, "large.txt")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(knowledgeMaxFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	kb, err := loadKnowledgeBase(dir)
	if err != nil || len(kb.chunks) < 2 {
		t.Fatalf("load chunks=%d err=%v", len(kb.chunks), err)
	}

	origRead := knowledgeReadFile
	knowledgeReadFile = func(string) ([]byte, error) { return nil, errors.New("read failed") }
	if _, err := loadKnowledgeBase(dir); err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("read failure err=%v", err)
	}
	knowledgeReadFile = origRead
	t.Cleanup(func() { knowledgeReadFile = origRead })

	if got := kb.augment("!"); got != "!" {
		t.Fatalf("term-free question=%q", got)
	}

	long := strings.Repeat("alpha beta ", 150)
	ranked := &knowledgeBase{chunks: []knowledgeChunk{
		{source: "one", text: long, terms: map[string]int{"alpha": 1, "beta": 1}},
		{source: "two", text: long, terms: map[string]int{"alpha": 3}},
		{source: "three", text: long, terms: map[string]int{"alpha": 2}},
		{source: "four", text: long, terms: map[string]int{"alpha": 1}},
	}}
	got := ranked.augment("alpha beta")
	if !strings.Contains(got, "用户问题：alpha beta") {
		t.Fatalf("augmented prompt missing question: %q", got)
	}
}
