package helpers

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// stripInjections undoes every repair the plan recorded, so what remains must be
// the original content. Stripping is anchored (TrimSuffix on the chunk before the
// boundary, TrimPrefix on the chunk after it) rather than a substring search,
// which is why MarkdownDegradation reports the two halves separately.
func stripInjections(p MarkdownChunkPlan) []string {
	out := append([]string(nil), p.Chunks...)
	for _, d := range p.Degradations {
		if d.InjectedSuffix != "" && d.ChunkIndex < len(out) {
			out[d.ChunkIndex] = strings.TrimSuffix(out[d.ChunkIndex], d.InjectedSuffix)
		}
		if d.InjectedPrefix != "" && d.ChunkIndex+1 < len(out) {
			out[d.ChunkIndex+1] = strings.TrimPrefix(out[d.ChunkIndex+1], d.InjectedPrefix)
		}
	}
	return out
}

// dropWhitespace removes every space, tab and newline. Comparing content with
// whitespace removed is the strongest no-loss statement compatible with repairs:
// boundary whitespace legitimately moves, but no other character may appear or
// disappear.
func dropWhitespace(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' {
			return -1
		}
		return r
	}, s)
}

// hasUnclosedFence replays the production fence rule over a single chunk. A
// naive count of "```" cannot do this: a four-backtick fence legitimately
// contains three-backtick lines, so only the state machine knows whether the
// chunk ends inside a code block.
func hasUnclosedFence(chunk string) bool {
	scan := scanMarkdownStructure(normalizeMarkdownNewlines(chunk))
	for _, r := range scan.regions {
		if r.kind == regionFence && scan.lines[r.lastLine].kind != lineFenceClose {
			return true
		}
	}
	return false
}

func degradationKinds(p MarkdownChunkPlan) []string {
	kinds := make([]string, 0, len(p.Degradations))
	for _, d := range p.Degradations {
		kinds = append(kinds, d.Kind)
	}
	return kinds
}

func TestCrossPlatformCoverageMarkdownChunkTierSelection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		content  string
		limit    int
		want     []string
		wantKind []string
	}{{
		name:    "blank line boundary is invisible",
		content: "alpha\n\nbravo\n\ncharlie",
		limit:   12,
		want:    []string{"alpha\n\nbravo", "charlie"},
	}, {
		// The heading interrupts the paragraph, so cutting right before it is
		// safe even without a blank line. This is the boundary the previous
		// implementation corrupted into "para# Title".
		name:    "heading interrupts a paragraph",
		content: "para\n# Title",
		limit:   8,
		want:    []string{"para", "# Title"},
	}, {
		// Greedy-latest within a tier: every chunk lands in the same document,
		// so an earlier safe boundary would only cost an extra round trip.
		// Aligning to the heading here would emit 3 chunks (10/84/30).
		name:    "greedy within the safe tier",
		content: strings.Repeat("a", 10) + "\n\n# h\n\n" + strings.Repeat("b", 80) + "\n\n" + strings.Repeat("c", 30),
		limit:   100,
		want: []string{
			strings.Repeat("a", 10) + "\n\n# h\n\n" + strings.Repeat("b", 80),
			strings.Repeat("c", 30),
		},
	}, {
		name:     "paragraph soft break becomes two paragraphs",
		content:  "first line\nsecond line",
		limit:    12,
		want:     []string{"first line", "second line"},
		wantKind: []string{"paragraph_split"},
	}, {
		name:     "unordered list splits between items",
		content:  "- alpha\n- bravo\n- charlie",
		limit:    10,
		want:     []string{"- alpha", "- bravo", "- charlie"},
		wantKind: []string{"list_split", "list_split"},
	}, {
		// Ordered lists are repair tier, not soft: the server renumbers from 1,
		// which is a visible change an unordered split does not cause.
		name:     "ordered list split is heavier than unordered",
		content:  "1. alpha\n2. bravo",
		limit:    10,
		want:     []string{"1. alpha", "2. bravo"},
		wantKind: []string{"ordered_list_split"},
	}, {
		name:     "blockquote splits between lines",
		content:  "> one\n> two\n> three",
		limit:    8,
		want:     []string{"> one", "> two", "> three"},
		wantKind: []string{"blockquote_split", "blockquote_split"},
	}, {
		// Each half is still an indented code block, so no injection is needed —
		// but one block becomes two.
		name:     "indented code splits at a line boundary",
		content:  "    code a\n    code b",
		limit:    12,
		want:     []string{"    code a", "    code b"},
		wantKind: []string{"indented_code_split"},
	}, {
		name:     "single long line falls back to whitespace",
		content:  "aaa bbb ccc ddd",
		limit:    8,
		want:     []string{"aaa bbb ", "ccc ddd"},
		wantKind: []string{"hard_split_at_whitespace"},
	}, {
		name:     "single long word falls back to runes",
		content:  strings.Repeat("界", 7),
		limit:    3,
		want:     []string{"界界界", "界界界", "界"},
		wantKind: []string{"hard_rune_split", "hard_rune_split"},
	}, {
		name:    "leading blank lines never emit an empty chunk",
		content: "\n\n\nalpha\n\nbravo",
		limit:   6,
		want:    []string{"alpha", "bravo"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			plan := SplitMarkdownForAppend(tc.content, tc.limit)
			if len(plan.Chunks) != len(tc.want) {
				t.Fatalf("chunks = %#v, want %#v", plan.Chunks, tc.want)
			}
			for i, want := range tc.want {
				if plan.Chunks[i] != want {
					t.Errorf("chunk %d = %q, want %q", i, plan.Chunks[i], want)
				}
			}
			if got := degradationKinds(plan); strings.Join(got, ",") != strings.Join(tc.wantKind, ",") {
				t.Errorf("degradations = %v, want %v", got, tc.wantKind)
			}
		})
	}
}

func TestCrossPlatformCoverageMarkdownChunkRepairsTablesAndFences(t *testing.T) {
	t.Run("table repeats header and delimiter", func(t *testing.T) {
		content := "| 姓名 | 部门 |\n|---|---|\n" + strings.Repeat("| 张三 | 技术部 |\n", 5)
		plan := SplitMarkdownForAppend(content, 60)
		if len(plan.Chunks) < 2 {
			t.Fatalf("expected a split, got %#v", plan.Chunks)
		}
		header := "| 姓名 | 部门 |\n|---|---|\n"
		for i, chunk := range plan.Chunks {
			if !strings.HasPrefix(chunk, header) {
				t.Errorf("chunk %d lost the header: %q", i, chunk)
			}
		}
		for _, d := range plan.Degradations {
			if d.Kind != "table_split" || d.InjectedPrefix != header {
				t.Errorf("degradation = %#v", d)
			}
		}
		// The repeated header is charged to the chunk that carries it.
		for i, chunk := range plan.Chunks {
			if n := utf8.RuneCountInString(chunk); n > 60 {
				t.Errorf("chunk %d is %d runes, over the limit", i, n)
			}
		}
	})

	t.Run("fence closes and reopens with the original marker", func(t *testing.T) {
		for _, open := range []string{"```go", "````", "~~~~", "~~~ yaml"} {
			marker := open[:strings.LastIndexAny(open, "`~")+1]
			content := open + "\n" + strings.Repeat("body\n", 6) + marker
			plan := SplitMarkdownForAppend(content, 30)
			if len(plan.Chunks) < 2 {
				t.Fatalf("%q: expected a split, got %#v", open, plan.Chunks)
			}
			for i, chunk := range plan.Chunks {
				if !strings.HasPrefix(chunk, open) {
					t.Errorf("%q chunk %d lost the opening fence: %q", open, i, chunk)
				}
				if !strings.HasSuffix(chunk, marker) {
					t.Errorf("%q chunk %d is not closed: %q", open, i, chunk)
				}
				if hasUnclosedFence(chunk) {
					t.Errorf("%q chunk %d leaves a fence open: %q", open, i, chunk)
				}
			}
		}
	})

	t.Run("fence interior keeps trailing blank lines", func(t *testing.T) {
		content := "```\nalpha\n\n\nbravo\ncharlie\ndelta\n```"
		plan := SplitMarkdownForAppend(content, 22)
		joined := strings.Join(stripInjections(plan), "")
		if dropWhitespace(joined) != dropWhitespace(content) {
			t.Fatalf("content changed: %q", joined)
		}
		if !strings.Contains(strings.Join(plan.Chunks, "|"), "alpha\n\n\n") {
			t.Errorf("blank lines inside the fence were trimmed: %#v", plan.Chunks)
		}
	})

	t.Run("repair wider than the limit is dropped and reported", func(t *testing.T) {
		// The reopening fence alone is longer than the limit, so preserving the
		// structure is impossible; the splitter must say so rather than emit
		// chunks over the limit or loop forever.
		content := "```" + strings.Repeat("x", 40) + "\n" + strings.Repeat("body\n", 4) + "```"
		plan := SplitMarkdownForAppend(content, 20)
		for i, chunk := range plan.Chunks {
			if n := utf8.RuneCountInString(chunk); n > 20 {
				t.Errorf("chunk %d is %d runes, over the limit", i, n)
			}
		}
		if !strings.Contains(strings.Join(degradationKinds(plan), ","), "repair_skipped") {
			t.Errorf("expected repair_skipped, got %v", degradationKinds(plan))
		}
	})
}

func TestCrossPlatformCoverageMarkdownChunkHazards(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		limit   int
		// forbidden is a boundary that must never be chosen, expressed as the
		// exact text a chunk would end with if it were.
		forbidden string
		wantKinds string
	}{{
		// `para\n---` is a setext H2, not a paragraph followed by a rule.
		// Splitting there turns one heading into a paragraph plus a horizontal
		// rule, so the dash line must not be treated as a block start.
		name:      "setext underline with dashes",
		content:   "para\n---\n\ntail text here",
		limit:     10,
		forbidden: "para",
	}, {
		name:      "setext underline with equals",
		content:   "para\n===\n\ntail text here",
		limit:     10,
		forbidden: "para",
	}, {
		// A dash rule *is* safe when no paragraph is open above it.
		name:    "dash rule after a blank line is safe",
		content: "alpha\n\n---\n\nbravo",
		limit:   9,
	}, {
		// `***` cannot be read as a setext underline, so it always interrupts.
		name:    "asterisk rule interrupts a paragraph",
		content: "para\n***\nmore text",
		limit:   9,
	}, {
		// The blank line belongs to the code block, so it is not a safe boundary.
		// Cutting is still allowed at the code block's own line boundaries, but
		// it must be reported rather than treated as invisible.
		name:      "indented code with an interior blank line",
		content:   "    alpha\n\n    bravo",
		limit:     11,
		wantKinds: "indented_code_split",
	}, {
		// A loose list's interior blank line is inside the list.
		name:      "loose list interior blank line",
		content:   "- alpha\n\n  more\n- bravo",
		limit:     16,
		forbidden: "- alpha",
	}, {
		// Whether a table interrupts a paragraph is parser-dependent, so this
		// boundary is allowed but must be reported, never treated as safe.
		name:      "table header after a paragraph line is not safe",
		content:   "para\n| a | b |\n|---|---|\n| 1 | 2 |",
		limit:     24,
		wantKinds: "block_boundary_split",
	}, {
		name:      "html block split is reported",
		content:   "<div>\nalpha\nbravo\n</div>",
		limit:     12,
		wantKinds: "html_block_split",
	}, {
		name:      "link reference definitions are reported",
		content:   "[ref]: https://example.com/one\n\nalpha text\n\nbravo text",
		limit:     16,
		wantKinds: "link_reference_split",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			plan := SplitMarkdownForAppend(tc.content, tc.limit)
			for i, chunk := range plan.Chunks {
				if n := utf8.RuneCountInString(chunk); n > tc.limit {
					t.Errorf("chunk %d is %d runes, over the limit: %q", i, n, chunk)
				}
			}
			if tc.forbidden != "" {
				for i, chunk := range plan.Chunks {
					if chunk == tc.forbidden {
						t.Errorf("chunk %d cut at a forbidden boundary: %#v", i, plan.Chunks)
					}
				}
			}
			if tc.wantKinds != "" {
				got := strings.Join(degradationKinds(plan), ",")
				if !strings.Contains(got, tc.wantKinds) {
					t.Errorf("degradations = %q, want to contain %q", got, tc.wantKinds)
				}
			}
			joined := strings.Join(stripInjections(plan), "")
			if dropWhitespace(joined) != dropWhitespace(normalizeMarkdownNewlines(tc.content)) {
				t.Errorf("content changed:\n got %q\nwant %q", joined, tc.content)
			}
		})
	}
}

func TestCrossPlatformCoverageMarkdownChunkEdgeContracts(t *testing.T) {
	t.Run("limit disables splitting", func(t *testing.T) {
		// Callers rely on this as an explicit "send it in one call whatever the
		// size" escape hatch.
		for _, limit := range []int{0, -1} {
			plan := SplitMarkdownForAppend("alpha\nbravo\ncharlie", limit)
			if len(plan.Chunks) != 1 || plan.Chunks[0] != "alpha\nbravo\ncharlie" {
				t.Fatalf("limit=%d chunks = %#v", limit, plan.Chunks)
			}
			if plan.Degraded() {
				t.Errorf("limit=%d must not degrade: %#v", limit, plan.Degradations)
			}
		}
	})

	t.Run("empty content yields one empty chunk", func(t *testing.T) {
		// chunkedWrite and the doc shortcuts both index Chunks[0] unguarded.
		plan := SplitMarkdownForAppend("", DefaultMarkdownChunkRunes)
		if len(plan.Chunks) != 1 || plan.Chunks[0] != "" {
			t.Fatalf("chunks = %#v", plan.Chunks)
		}
	})

	t.Run("newlines are normalized on the single-chunk path too", func(t *testing.T) {
		// Normalizing rather than deleting matters: deleting a lone CR silently
		// joins two lines. Doing it unconditionally matters so short and long
		// content never disagree.
		for _, limit := range []int{5, DefaultMarkdownChunkRunes} {
			plan := SplitMarkdownForAppend("a\r\nb\rc", limit)
			if got := strings.Join(plan.Chunks, ""); !strings.Contains(got, "a\nb\nc") {
				t.Errorf("limit=%d joined = %q, want CRLF and lone CR normalized", limit, got)
			}
		}
	})

	t.Run("whitespace-only content", func(t *testing.T) {
		plan := SplitMarkdownForAppend("\n\n\n\n\n\n\n\n", 3)
		if len(plan.Chunks) == 0 {
			t.Fatal("plan must always hold at least one chunk")
		}
	})

	t.Run("projections", func(t *testing.T) {
		plan := SplitMarkdownForAppend("first line\nsecond line", 12)
		if !plan.Degraded() {
			t.Fatalf("expected a degraded plan: %#v", plan)
		}
		if got := plan.ExpectedDocument(); got != "first line\n\nsecond line" {
			t.Errorf("ExpectedDocument = %q", got)
		}
		warnings := plan.Warnings()
		if len(warnings) != 1 || !strings.Contains(warnings[0], "段落") {
			t.Errorf("Warnings = %#v", warnings)
		}
		summary := plan.Summary()
		if summary["chunks"] != 2 || summary["degraded"] != true || summary["limit"] != 12 {
			t.Errorf("Summary = %#v", summary)
		}

		single := SplitMarkdownForAppend("short", DefaultMarkdownChunkRunes)
		if single.Degraded() || single.Warnings() != nil {
			t.Errorf("single chunk must not degrade: %#v", single)
		}
		if got := single.ExpectedDocument(); got != "short" {
			t.Errorf("single ExpectedDocument = %q", got)
		}
	})

	t.Run("warnings aggregate repeated kinds", func(t *testing.T) {
		plan := SplitMarkdownForAppend("- a\n- b\n- c\n- d", 4)
		warnings := plan.Warnings()
		if len(warnings) != 1 || !strings.Contains(warnings[0], "处") {
			t.Errorf("Warnings = %#v", warnings)
		}
	})
}

// TestCrossPlatformCoverageMarkdownChunkInvariantsOnSeededCorpus is the
// regression net for the whole algorithm. The seed is fixed so a failure is
// reproducible, and the corpus crosses every block type against limits small
// enough to force each tier.
//
// The old block-rebuilding splitter failed the content invariant on roughly one
// input in five, because it reassembled text instead of slicing it.
func TestCrossPlatformCoverageMarkdownChunkInvariantsOnSeededCorpus(t *testing.T) {
	generators := []func(n int) string{
		func(n int) string { return fmt.Sprintf("# Heading %d", n) },
		func(n int) string { return fmt.Sprintf("### Heading %d", n) },
		func(n int) string { return strings.Repeat("文字", 1+n%9) },
		func(n int) string { return "first line\nsecond line\nthird line" },
		func(n int) string { return "| a | b |\n|---|---|\n" + strings.Repeat("| 1 | 2 |\n", 1+n%5) },
		func(n int) string { return "```go\n" + strings.Repeat("code\n", 1+n%4) + "```" },
		func(n int) string { return "~~~~\n" + strings.Repeat("t\n", 1+n%3) + "~~~~" },
		func(n int) string { return "````\n``` nested\n````" },
		func(n int) string { return "- alpha\n- bravo\n- charlie" },
		func(n int) string { return "1. alpha\n2. bravo\n3. charlie" },
		func(n int) string { return "- alpha\n\n  continued\n- bravo" },
		func(n int) string { return "> quoted one\n> quoted two" },
		func(n int) string { return "    indented\n\n    code" },
		func(n int) string { return "setext heading\n---" },
		func(n int) string { return "setext heading\n===" },
		func(n int) string { return "***" },
		func(n int) string { return strings.Repeat("x", 30+n%40) },
		func(n int) string { return "word " + strings.Repeat("wordy ", 6+n%5) },
		func(n int) string { return "a\r\nb\rc" },
		func(n int) string { return "<div>\nhtml body\n</div>" },
		func(n int) string { return "<script>\n\nstill script\n</script>" },
		func(n int) string { return "<pre>\n" + strings.Repeat("pre line\n", 1+n%4) + "</pre>" },
		func(n int) string { return "<pre>\nfirst\n\nafter blank\n</pre>" },
		func(n int) string { return "<pre\n  class=\"c\">\n" + strings.Repeat("x\n", 1+n%3) + "</pre>" },
		func(n int) string { return "<style>\n" + strings.Repeat(".a{b:1}\n", 1+n%3) + "</style>" },
		func(n int) string { return "<textarea>\n" + strings.Repeat("row\n", 1+n%3) + "</textarea>" },
		func(n int) string { return "[ref]: https://example.com/x" },
	}
	limits := []int{1, 2, 3, 7, 17, 64, 200}
	rng := rand.New(rand.NewSource(20260817))

	for iter := 0; iter < 4000; iter++ {
		var parts []string
		for k := 0; k < 1+rng.Intn(6); k++ {
			parts = append(parts, generators[rng.Intn(len(generators))](rng.Intn(10)))
		}
		content := strings.Join(parts, "\n\n")
		limit := limits[rng.Intn(len(limits))]
		plan := SplitMarkdownForAppend(content, limit)
		normalized := normalizeMarkdownNewlines(content)

		fail := func(format string, args ...any) {
			t.Fatalf("seed=20260817 iter=%d limit=%d content=%q\n"+format,
				append([]any{iter, limit, normalized}, args...)...)
		}

		// I1: a plan always has an indexable first chunk.
		if len(plan.Chunks) == 0 {
			fail("empty plan")
		}
		// I2: no chunk exceeds the limit, injections included.
		for i, chunk := range plan.Chunks {
			if n := utf8.RuneCountInString(chunk); n > limit {
				fail("chunk %d is %d runes: %q", i, n, chunk)
			}
		}
		// I3: stripping the recorded repairs recovers the content exactly.
		if got := dropWhitespace(strings.Join(stripInjections(plan), "")); got != dropWhitespace(normalized) {
			fail("content changed:\n got %q\nwant %q", got, dropWhitespace(normalized))
		}
		// I4 and I5 assume the structure could be preserved. When the limit is
		// smaller than a fence line or a table header, the plan says so via
		// repair_skipped and no self-containedness claim is being made.
		if strings.Contains(strings.Join(degradationKinds(plan), ","), "repair_skipped") {
			continue
		}
		// I4: no chunk ends inside a code block.
		for i, chunk := range plan.Chunks {
			if hasUnclosedFence(chunk) {
				fail("chunk %d leaves a fence open: %q", i, chunk)
			}
		}
		// I5: a table body row is never orphaned from its header, so no chunk
		// may open with a delimiter row.
		for i, chunk := range plan.Chunks {
			if first := strings.SplitN(chunk, "\n", 2)[0]; isTableDelimiterRow(strings.TrimSpace(first)) {
				fail("chunk %d starts with a delimiter row: %q", i, chunk)
			}
		}
	}
}

// TestCrossPlatformCoverageMarkdownChunkDegradationBookkeeping pins the contracts
// that decide the ExpectedDocument / degradations story every caller depends on.
// The seeded corpus exercises these paths but asserts only aggregate invariants;
// these cases assert the exact field values, so an off-by-one in ChunkIndex or a
// wrong ExpectedDocument join can never pass silently.
func TestCrossPlatformCoverageMarkdownChunkDegradationBookkeeping(t *testing.T) {
	t.Run("ChunkIndex points at the chunk before each boundary", func(t *testing.T) {
		// Four list items, each within the limit, split into four chunks with a
		// degradation at each of the three boundaries, indexed 0,1,2.
		plan := SplitMarkdownForAppend("- aa\n- bb\n- cc\n- dd", 5)
		if len(plan.Chunks) != 4 || len(plan.Degradations) != 3 {
			t.Fatalf("chunks=%#v degradations=%#v", plan.Chunks, plan.Degradations)
		}
		for i, d := range plan.Degradations {
			if d.ChunkIndex != i {
				t.Errorf("degradation %d has ChunkIndex %d, want %d", i, d.ChunkIndex, i)
			}
		}
	})

	t.Run("ExpectedDocument carries the repeated table header", func(t *testing.T) {
		// This is the case ExpectedDocument exists for: after a table repair the
		// server holds N tables, so verification must expect the repeated header,
		// not the single-table input.
		header := "| a | b |\n|---|---|\n"
		content := header + strings.Repeat("| 1 | 2 |\n", 6)
		plan := SplitMarkdownForAppend(content, 40)
		if len(plan.Chunks) < 2 {
			t.Fatalf("fixture must split, got %d chunk(s)", len(plan.Chunks))
		}
		expected := plan.ExpectedDocument()
		// The header appears once per chunk, i.e. more often than in the input.
		if got, want := strings.Count(expected, "| a | b |"), len(plan.Chunks); got != want {
			t.Errorf("header appears %d times in ExpectedDocument, want %d", got, want)
		}
		// ExpectedDocument is exactly the chunks joined with a blank line, and it
		// re-parses without any orphaned delimiter row.
		if expected != strings.Join(plan.Chunks, "\n\n") {
			t.Errorf("ExpectedDocument is not the chunks joined by a blank line")
		}
		rescan := SplitMarkdownForAppend(expected, len(expected)*4)
		if len(rescan.Chunks) != 1 || rescan.Degraded() {
			t.Errorf("ExpectedDocument does not round-trip as one clean document: %#v", rescan)
		}
	})

	t.Run("one document reports every distinct degradation kind", func(t *testing.T) {
		// A realistic mixed document: an oversized table, an oversized fenced
		// code block, and a long paragraph, each forced to split by the limit.
		content := strings.Join([]string{
			"| a | b |\n|---|---|\n" + strings.Repeat("| 1 | 2 |\n", 6),
			"```go\n" + strings.Repeat("fmt.Println(1)\n", 6) + "```",
			strings.Repeat("word ", 40),
		}, "\n\n")
		plan := SplitMarkdownForAppend(content, 45)

		kinds := map[string]bool{}
		lastLineByKind := map[string]int{}
		for _, d := range plan.Degradations {
			kinds[d.Kind] = true
			// Lines within a kind are reported in ascending order.
			if prev, seen := lastLineByKind[d.Kind]; seen && d.Line < prev {
				t.Errorf("%s degradation lines out of order: %d after %d", d.Kind, d.Line, prev)
			}
			lastLineByKind[d.Kind] = d.Line
		}
		for _, want := range []string{"table_split", "code_block_split"} {
			if !kinds[want] {
				t.Errorf("missing %s in %v", want, degradationKinds(plan))
			}
		}
		// Stripping every recorded repair still recovers the whole document.
		if got := dropWhitespace(strings.Join(stripInjections(plan), "")); got != dropWhitespace(content) {
			t.Errorf("mixed document not recoverable after stripping injections")
		}
	})

	t.Run("regression: large document ending in a heading keeps the newline", func(t *testing.T) {
		// The bug that started this rewrite: the old splitter rebuilt block text
		// and dropped the newline before a trailing heading/table/fence, so a
		// heading at the very end stopped being a heading. Reproduce it at
		// production scale.
		body := strings.Repeat("这是一段正文内容。\n\n", 6000) // ~60k runes, no trailing newline
		content := body + "## 附录"
		plan := SplitMarkdownForAppend(content, DefaultMarkdownChunkRunes)
		if len(plan.Chunks) < 2 {
			t.Fatalf("fixture must exceed the limit, got %d chunk(s)", len(plan.Chunks))
		}
		// The heading must survive as its own line, never glued to preceding text.
		last := plan.Chunks[len(plan.Chunks)-1]
		if !strings.Contains(last, "\n## 附录") {
			r := []rune(last)
			t.Fatalf("trailing heading is not on its own line: %q", string(r[max(0, len(r)-30):]))
		}
		// The original bug produced a single line like "正文内容。## 附录". Assert no
		// chunk has body text and the heading marker on the same line.
		for i, chunk := range plan.Chunks {
			for _, line := range strings.Split(chunk, "\n") {
				if strings.Contains(line, "。##") {
					t.Errorf("chunk %d glued the heading onto a text line: %q", i, line)
				}
			}
		}
		// And nothing was lost.
		if got := dropWhitespace(strings.Join(plan.Chunks, "")); got != dropWhitespace(normalizeMarkdownNewlines(content)) {
			t.Error("content changed at production scale")
		}
	})

	t.Run("repaired chunks stay within the limit at the production limit", func(t *testing.T) {
		// The concern: a continuation chunk carries a re-emitted table header, so
		// could header + rows push it over the limit? No — the header is charged
		// against the window budget before rows are packed. The seeded corpus
		// proves this at tiny limits where the header dwarfs the limit; this pins
		// it at the real DefaultMarkdownChunkRunes with a wide header, which is the
		// value that actually ships.
		limit := DefaultMarkdownChunkRunes
		const cols = 40
		header := "|" + strings.Repeat(" 列名称占位 |", cols) + "\n"
		delim := "|" + strings.Repeat("---|", cols) + "\n"
		row := "|" + strings.Repeat(" 单元格数据 |", cols) + "\n"
		content := header + delim + strings.Repeat(row, limit/utf8.RuneCountInString(row)+50)

		plan := SplitMarkdownForAppend(content, limit)
		if len(plan.Chunks) < 2 {
			t.Fatalf("fixture must split, got %d chunk(s)", len(plan.Chunks))
		}
		sawInjectedContinuation := false
		for i, chunk := range plan.Chunks {
			if n := utf8.RuneCountInString(chunk); n > limit {
				t.Errorf("chunk %d is %d runes, over the limit %d", i, n, limit)
			}
			if i > 0 && strings.HasPrefix(chunk, header) {
				sawInjectedContinuation = true
			}
		}
		if !sawInjectedContinuation {
			t.Error("expected at least one continuation chunk carrying the re-emitted header")
		}
	})

	t.Run("closed pre block does not swallow a following oversized table", func(t *testing.T) {
		// Regression for the type-1 HTML closer bug: <pre>/<style>/<textarea> were
		// all classified as type 1 but the terminator search only looked for
		// </script>, so a closed <pre> block swallowed the rest of the document.
		// A following oversized table would then be cut as HTML lines with no
		// header re-emitted, leaving invalid tables downstream.
		header := "| a | b |\n|---|---|\n"
		table := header + strings.Repeat("| 1 | 2 |\n", 12)
		for _, open := range []string{"<pre>", "<style>", "<textarea>"} {
			closer := strings.Replace(open, "<", "</", 1)
			content := open + "\ninner\n" + closer + "\n\n" + table
			plan := SplitMarkdownForAppend(content, 40)

			kinds := strings.Join(degradationKinds(plan), ",")
			if !strings.Contains(kinds, "table_split") {
				t.Errorf("%s: table was not split as a table (degradations=%q)", open, kinds)
			}
			if strings.Contains(kinds, "html_block_split") {
				t.Errorf("%s: table was cut as HTML lines (degradations=%q)", open, kinds)
			}
			// Every continuation chunk of the table carries the re-emitted header.
			for i, chunk := range plan.Chunks {
				if i > 0 && strings.Contains(chunk, "| 1 | 2 |") && !strings.HasPrefix(chunk, header) {
					t.Errorf("%s: chunk %d has table rows without the header: %q", open, i, chunk)
				}
			}
		}
	})
}

func TestCrossPlatformCoverageMarkdownChunkHTMLRepair(t *testing.T) {
	t.Run("pre style textarea reopen and close on every chunk", func(t *testing.T) {
		for _, tc := range []struct{ open, close string }{
			{"<pre>", "</pre>"}, {"<style>", "</style>"}, {"<textarea>", "</textarea>"},
		} {
			content := tc.open + "\n" + strings.Repeat("row\n", 8) + tc.close
			// Limit large enough for the injected tags plus one content line.
			plan := SplitMarkdownForAppend(content, len(tc.open)+len(tc.close)+8)
			if len(plan.Chunks) < 2 {
				t.Fatalf("%s: expected a split, got %d", tc.open, len(plan.Chunks))
			}
			for i, chunk := range plan.Chunks {
				if !strings.HasPrefix(chunk, tc.open) {
					t.Errorf("%s: chunk %d not reopened: %q", tc.open, i, chunk)
				}
				if !strings.HasSuffix(chunk, tc.close) {
					t.Errorf("%s: chunk %d not closed: %q", tc.open, i, chunk)
				}
			}
			for _, d := range plan.Degradations {
				if d.Kind != "html_block_split" || d.Tier != "repair" || d.InjectedPrefix == "" {
					t.Errorf("%s: degradation = %#v", tc.open, d)
				}
			}
			// Content recoverable after stripping the injected tags.
			if got := dropWhitespace(strings.Join(stripInjections(plan), "")); got != dropWhitespace(content) {
				t.Errorf("%s: content changed after strip: %q", tc.open, got)
			}
		}
	})

	t.Run("multi-line opening tag is reopened in full", func(t *testing.T) {
		open := "<pre\n  class=\"code\"\n  id=\"x\">"
		content := open + "\n" + strings.Repeat("content line\n", 6) + "</pre>"
		plan := SplitMarkdownForAppend(content, 60)
		if len(plan.Chunks) < 2 {
			t.Fatalf("expected a split, got %d", len(plan.Chunks))
		}
		for i, chunk := range plan.Chunks {
			if i > 0 && !strings.HasPrefix(chunk, open) {
				t.Errorf("chunk %d did not reopen the full multi-line tag: %q", i, chunk)
			}
			if n := utf8.RuneCountInString(chunk); n > 60 {
				t.Errorf("chunk %d is %d runes, over the limit", i, n)
			}
		}
	})

	t.Run("interior blank line is preserved, never a silent safe split", func(t *testing.T) {
		content := "<pre>\nbefore\n\nafter\n</pre>"
		plan := SplitMarkdownForAppend(content, 16)
		if len(plan.Chunks) < 2 {
			t.Fatalf("expected a split, got %d", len(plan.Chunks))
		}
		// The blank inside the pre must not be treated as a boundary-free safe
		// cut: splitting inside a <pre> must be reported, and content survives.
		if len(plan.Degradations) == 0 {
			t.Error("splitting inside a <pre> must be reported, not silent")
		}
		if got := dropWhitespace(strings.Join(stripInjections(plan), "")); got != dropWhitespace(content) {
			t.Errorf("blank line lost: %q", got)
		}
	})

	t.Run("script is never repaired", func(t *testing.T) {
		content := "<script>\n" + strings.Repeat("var x=1\n", 6) + "</script>"
		plan := SplitMarkdownForAppend(content, 20)
		if len(plan.Chunks) < 2 {
			t.Fatalf("expected a split, got %d", len(plan.Chunks))
		}
		for i, chunk := range plan.Chunks {
			if i > 0 && strings.HasPrefix(chunk, "<script>") {
				t.Errorf("chunk %d reopened <script> — script must not be repaired: %q", i, chunk)
			}
		}
		for _, d := range plan.Degradations {
			if d.Tier == "repair" && d.Kind == "html_block_split" {
				t.Errorf("script was repaired: %#v", d)
			}
		}
	})

	t.Run("oversized single HTML line reports html_tag_hard_split with upload guidance", func(t *testing.T) {
		content := "<div>" + strings.Repeat("x", 60) + "</div>"
		plan := SplitMarkdownForAppend(content, 20)
		if !strings.Contains(strings.Join(degradationKinds(plan), ","), "html_tag_hard_split") {
			t.Fatalf("expected html_tag_hard_split, got %v", degradationKinds(plan))
		}
		joined := strings.Join(plan.Warnings(), " ")
		if !strings.Contains(joined, "doc import") {
			t.Errorf("warning must point at dws doc import: %q", joined)
		}
	})
}
