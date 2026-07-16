package helpers

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageSplitMarkdownSafePreservesContentAndLimitsChunks(t *testing.T) {
	short := "short 文本"
	if got := splitMarkdownSafe(short, 100); len(got) != 1 || got[0] != short {
		t.Fatalf("short split = %#v", got)
	}

	content := "# Heading\r\nparagraph one\r\n\r\n## Two\r\n| a | b |\r\n|---|---|\r\n| 1 | 2 |\r\nnormal\r\n### Three\r\n```go\r\nfmt.Println(\"hello\")\r\n```\r\ntail"
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	chunks := splitMarkdownSafe(content, 35)
	if strings.Join(chunks, "") != normalized {
		t.Fatalf("split content was not preserved:\nwant %q\n got %q", normalized, strings.Join(chunks, ""))
	}
	for _, chunk := range chunks {
		if utf8.RuneCountInString(chunk) > 35 {
			t.Errorf("chunk exceeds limit: %d %q", utf8.RuneCountInString(chunk), chunk)
		}
	}

	blocks := parseMarkdownBlocks("before\n```\nunclosed")
	if len(blocks) < 2 || blocks[len(blocks)-1].blockType != blockCodeBlock {
		t.Fatalf("unclosed code blocks = %#v", blocks)
	}
}

func TestCrossPlatformCoverageMergeBlocksUsesHeadingsAndHardSplits(t *testing.T) {
	blocks := []markdownBlock{
		{text: "aaaa", blockType: blockNormal},
		{text: "# h\n", blockType: blockH1},
		{text: "bbbb", blockType: blockNormal},
		{text: "cccc", blockType: blockNormal},
	}
	chunks := mergeBlocksIntoChunks(blocks, 9)
	if strings.Join(chunks, "") != "aaaa# h\nbbbbcccc" || len(chunks) < 2 {
		t.Fatalf("heading merge = %#v", chunks)
	}

	chunks = mergeBlocksIntoChunks([]markdownBlock{{text: "aaaa"}, {text: "bbbb"}, {text: "cccc"}}, 8)
	if strings.Join(chunks, "") != "aaaabbbbcccc" {
		t.Fatalf("greedy merge = %#v", chunks)
	}

	oversized := "one\n\ntwo\n\n" + strings.Repeat("界", 11)
	chunks = mergeBlocksIntoChunks([]markdownBlock{{text: oversized}}, 5)
	if strings.Join(chunks, "") != oversized {
		t.Fatalf("oversized merge = %#v", chunks)
	}
	for _, chunk := range chunks {
		if utf8.RuneCountInString(chunk) > 5 {
			t.Errorf("hard-split chunk exceeds limit: %q", chunk)
		}
	}

	if got := mergeBlocksIntoChunks(nil, 5); len(got) != 0 {
		t.Fatalf("empty merge = %#v", got)
	}
}

func TestCrossPlatformCoverageHardSplitBlockCoversParagraphAndRuneBoundaries(t *testing.T) {
	for _, text := range []string{
		"aa\n\nbb\n\ncc",
		"aa\n\n" + strings.Repeat("x", 12),
		strings.Repeat("界", 13),
		"",
	} {
		chunks := hardSplitBlock(text, 5)
		if strings.Join(chunks, "") != text {
			t.Errorf("hardSplitBlock(%q) = %#v", text, chunks)
		}
		for _, chunk := range chunks {
			if utf8.RuneCountInString(chunk) > 5 {
				t.Errorf("chunk exceeds limit: %q", chunk)
			}
		}
	}
}

func TestCrossPlatformCoverageRuntimeDefaultsRegistryValidationAndSnapshot(t *testing.T) {
	runtimeDefaultsMu.Lock()
	previous := runtimeDefaults
	runtimeDefaults = make(map[string]edition.RuntimeDefaultFn)
	runtimeDefaultsMu.Unlock()
	t.Cleanup(func() {
		runtimeDefaultsMu.Lock()
		runtimeDefaults = previous
		runtimeDefaultsMu.Unlock()
	})

	resolver := func(context.Context) (string, bool) { return "value", true }
	RegisterRuntimeDefault("$value", resolver)
	snapshot := RuntimeDefaultsSnapshot()
	if len(snapshot) != 1 || snapshot["$value"] == nil {
		t.Fatalf("RuntimeDefaultsSnapshot() = %#v", snapshot)
	}
	delete(snapshot, "$value")
	if len(RuntimeDefaultsSnapshot()) != 1 {
		t.Fatal("snapshot mutated the runtime registry")
	}

	for _, tc := range []struct {
		name string
		fn   edition.RuntimeDefaultFn
	}{
		{"", resolver}, {"$nil", nil}, {"$value", resolver},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("RegisterRuntimeDefault(%q) did not panic", tc.name)
				}
			}()
			RegisterRuntimeDefault(tc.name, tc.fn)
		}()
	}
}
