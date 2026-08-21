package helpers

import (
	"strings"
	"testing"
)

// These are the CommonMark/GFM line predicates the whole tier model rests on.
// They are tested directly because a misclassification here silently downgrades
// or upgrades a split tier, and an upgraded tier corrupts documents.

func TestCrossPlatformCoverageMarkdownScanLinePredicates(t *testing.T) {
	t.Run("leadingIndent counts tabs as four columns", func(t *testing.T) {
		for _, tc := range []struct {
			line string
			want int
		}{
			{"none", 0}, {"  two", 2}, {"\ttab", 4}, {" \ttab", 4}, {"\t\t", 8}, {"    ", 4},
		} {
			if got := leadingIndent(tc.line); got != tc.want {
				t.Errorf("leadingIndent(%q) = %d, want %d", tc.line, got, tc.want)
			}
		}
	})

	t.Run("fenceMarkerOf requires three or more markers", func(t *testing.T) {
		for _, tc := range []struct {
			line  string
			ch    byte
			n     int
			ok    bool
			isOpe bool
		}{
			{"```", '`', 3, true, true},
			{"```go", '`', 3, true, true},
			{"````", '`', 4, true, true},
			{"~~~", '~', 3, true, true},
			{"~~~ yaml", '~', 3, true, true},
			{"``", 0, 0, false, false},      // too short to fence
			{"`code`", 0, 0, false, false},  // inline code
			{"text", 0, 0, false, false},    // not a marker at all
			{"", 0, 0, false, false},        // blank line inside a fence
			{"```a`b", '`', 3, true, false}, // backtick in the info string
		} {
			ch, n, ok := fenceMarkerOf(tc.line)
			if ch != tc.ch || n != tc.n || ok != tc.ok {
				t.Errorf("fenceMarkerOf(%q) = %q,%d,%v want %q,%d,%v", tc.line, ch, n, ok, tc.ch, tc.n, tc.ok)
			}
			if got := isFenceOpen(tc.line); got != tc.isOpe {
				t.Errorf("isFenceOpen(%q) = %v, want %v", tc.line, got, tc.isOpe)
			}
		}
	})

	t.Run("closingFenceIsBare rejects a trailing info string", func(t *testing.T) {
		for _, tc := range []struct {
			line string
			ch   byte
			n    int
			want bool
		}{
			{"```", '`', 3, true},
			{"```  ", '`', 3, true},
			{"`````", '`', 3, true}, // a longer run still closes
			{"```go", '`', 3, false},
		} {
			if got := closingFenceIsBare(tc.line, tc.ch, tc.n); got != tc.want {
				t.Errorf("closingFenceIsBare(%q) = %v, want %v", tc.line, got, tc.want)
			}
		}
	})

	t.Run("isATXHeading", func(t *testing.T) {
		for line, want := range map[string]bool{
			"# h": true, "###### h": true, "#": true, "#hash": false,
			"####### too deep": false, "text": false, "": false, "#\tt": true,
		} {
			if got := isATXHeading(line); got != want {
				t.Errorf("isATXHeading(%q) = %v, want %v", line, got, want)
			}
		}
	})

	t.Run("isRunOf and isThematicBreak", func(t *testing.T) {
		for _, tc := range []struct {
			line     string
			runDash  bool
			breakAst bool
		}{
			{"---", true, false},
			{"-", true, false},
			{"", false, false},
			{"-x-", false, false},
			{"***", false, true},
			{"* * *", false, true}, // interior spaces are allowed
			{"**", false, false},   // fewer than three
			{"*x*", false, false},
		} {
			if got := isRunOf(tc.line, '-'); got != tc.runDash {
				t.Errorf("isRunOf(%q,'-') = %v, want %v", tc.line, got, tc.runDash)
			}
			if got := isThematicBreak(tc.line, '*'); got != tc.breakAst {
				t.Errorf("isThematicBreak(%q,'*') = %v, want %v", tc.line, got, tc.breakAst)
			}
		}
	})

	t.Run("isTableDelimiterRow requires a pipe", func(t *testing.T) {
		for line, want := range map[string]bool{
			"|---|---|":      true,
			"---|---":        true,
			"| :-: | ---: |": true,
			"---":            false, // a thematic break or setext underline, never a table
			"--":             false,
			"| a | b |":      false, // a header row, not a delimiter
			"|":              false,
			"| |":            false, // empty cell, and no dash
			"|---|x|":        false, // one cell is not a run of dashes
			"|---| |":        false, // one cell is empty
			"":               false,
		} {
			if got := isTableDelimiterRow(line); got != want {
				t.Errorf("isTableDelimiterRow(%q) = %v, want %v", line, got, want)
			}
		}
	})

	t.Run("tableCellCount", func(t *testing.T) {
		for line, want := range map[string]int{
			"| a | b |": 2, "a | b": 2, "| a |": 1, "no pipes": 0,
		} {
			if got := tableCellCount(line); got != want {
				t.Errorf("tableCellCount(%q) = %d, want %d", line, got, want)
			}
		}
	})

	t.Run("listMarkerOf", func(t *testing.T) {
		for _, tc := range []struct {
			line       string
			ordered    bool
			contentCol int
			ok         bool
		}{
			{"- item", false, 2, true},
			{"+ item", false, 2, true},
			{"* item", false, 2, true},
			{"1. item", true, 3, true},
			{"12) item", true, 4, true},
			{"-", false, 1, true},          // a marker with no content
			{"-     wide", false, 2, true}, // more than four spaces resets to one
			{"  - nested", false, 4, true}, // indentation is included
			{"-item", false, 0, false},     // no space after the marker
			{"1.item", false, 0, false},    // no space after the number
			{"1x item", false, 0, false},   // not a marker
			{"text", false, 0, false},
			{"", false, 0, false},
			{"   ", false, 0, false}, // whitespace only
		} {
			ordered, col, ok := listMarkerOf(tc.line)
			if ordered != tc.ordered || ok != tc.ok || (ok && col != tc.contentCol) {
				t.Errorf("listMarkerOf(%q) = %v,%d,%v want %v,%d,%v",
					tc.line, ordered, col, ok, tc.ordered, tc.contentCol, tc.ok)
			}
		}
	})

	t.Run("listMarkerNumber", func(t *testing.T) {
		for line, want := range map[string]int{
			"1. a": 1, "7. a": 7, "12) a": 12, "- a": 0,
		} {
			if got := listMarkerNumber(line); got != want {
				t.Errorf("listMarkerNumber(%q) = %d, want %d", line, got, want)
			}
		}
	})

	t.Run("htmlBlockKind classifies the seven types", func(t *testing.T) {
		for _, tc := range []struct {
			line string
			kind uint8
			ok   bool
		}{
			{"<script>", 1, true},
			{"<PRE>", 1, true},
			{"<style>", 1, true},
			{"<textarea>", 1, true},
			{"<!-- comment", 2, true},
			{"<?php", 3, true},
			{"<!DOCTYPE html>", 4, true},
			{"<![CDATA[", 5, true},
			{"<div>", 6, true},
			{"</ul>", 6, true},
			{"<custom-tag>", 7, true},
			{"<1>", 0, false}, // a tag name may not start with a digit
			{"<>", 0, false},  // no tag name at all
			{"</>", 0, false}, // closing marker with no name
			{"text", 0, false},
			{"", 0, false},
		} {
			kind, ok := htmlBlockKind(tc.line)
			if kind != tc.kind || ok != tc.ok {
				t.Errorf("htmlBlockKind(%q) = %d,%v want %d,%v", tc.line, kind, ok, tc.kind, tc.ok)
			}
		}
	})

	t.Run("htmlCloser per type", func(t *testing.T) {
		for kind, want := range map[uint8]string{2: "-->", 3: "?>", 4: ">", 5: "]]>", 1: "</script>"} {
			if got := htmlCloser(kind); got != want {
				t.Errorf("htmlCloser(%d) = %q, want %q", kind, got, want)
			}
		}
	})

	t.Run("isLinkReferenceDefinition", func(t *testing.T) {
		for line, want := range map[string]bool{
			"[ref]: https://example.com": true, "[a]: x": true,
			"[]: x": false, "[ref] not a def": false, "text": false, "": false,
		} {
			if got := isLinkReferenceDefinition(line); got != want {
				t.Errorf("isLinkReferenceDefinition(%q) = %v, want %v", line, got, want)
			}
		}
	})

	t.Run("trimTrailingWhitespaceEnd", func(t *testing.T) {
		for _, tc := range []struct {
			content string
			end     int
			want    int
		}{
			{"abc\n\n", 5, 3}, {"abc", 3, 3}, {"\n\n\n", 3, 0}, {"a \t\n", 4, 1},
		} {
			if got := trimTrailingWhitespaceEnd(tc.content, tc.end); got != tc.want {
				t.Errorf("trimTrailingWhitespaceEnd(%q,%d) = %d, want %d", tc.content, tc.end, got, tc.want)
			}
		}
	})
}

func TestCrossPlatformCoverageMarkdownScanRegionGrouping(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		// wantKinds lists the region kind of each region, in order.
		wantKinds []regionKind
	}{
		{"unterminated fence swallows the tail", "before\n\n```go\nnever closed\nmore",
			[]regionKind{regionParagraph, regionFence}},
		{"indented fence is still a fence", "  ```go\ncode\n  ```",
			[]regionKind{regionFence}},
		{"quote ends at a heading", "> quoted\n# heading",
			[]regionKind{regionQuote, regionLeaf}},
		{"quote absorbs a lazy continuation", "> quoted\nlazy line\n\nafter",
			[]regionKind{regionQuote, regionParagraph}},
		{"setext beats a table delimiter", "heading\n---",
			[]regionKind{regionSetext}},
		{"delimiter row with a mismatched header is not a table", "para\n|---|---|---|",
			[]regionKind{regionParagraph}},
		{"table keeps rows that have no pipes", "| a | b |\n|---|---|\n| 1 | 2 |\nstray",
			[]regionKind{regionTable}},
		{"paragraph before a table is its own region", "intro\n| a | b |\n|---|---|\n| 1 | 2 |",
			[]regionKind{regionParagraph, regionTable}},
		{"list ends at an unindented paragraph", "- item\n\nplain paragraph",
			[]regionKind{regionList, regionParagraph}},
		{"list ends at an adjacent heading with no blank line", "- item\n# heading",
			[]regionKind{regionList, regionLeaf}},
		{"list ends at end of input after a blank line", "- item\n\n",
			[]regionKind{regionList}},
		{"list keeps an indented continuation across a blank line", "- item\n\n  continued\n\nplain",
			[]regionKind{regionList, regionParagraph}},
		{"html type 6 ends at a blank line", "<div>\nbody\n\nafter",
			[]regionKind{regionHTML, regionParagraph}},
		{"html type 1 spans blank lines", "<script>\n\nstill script\n</script>\n\nafter",
			[]regionKind{regionHTML, regionParagraph}},
		{"indented code spans a blank line", "    one\n\n    two\n\nplain",
			[]regionKind{regionIndentedCode, regionParagraph}},
		{"link reference definition is a leaf", "[ref]: https://example.com\n\npara",
			[]regionKind{regionLeaf, regionParagraph}},
		{"dash rule with no paragraph above is a leaf", "---\n\npara",
			[]regionKind{regionLeaf, regionParagraph}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scan := scanMarkdownStructure(tc.content)
			got := make([]regionKind, 0, len(scan.regions))
			for _, r := range scan.regions {
				got = append(got, r.kind)
			}
			if len(got) != len(tc.wantKinds) {
				t.Fatalf("regions = %v, want %v", got, tc.wantKinds)
			}
			for i := range got {
				if got[i] != tc.wantKinds[i] {
					t.Errorf("region %d = %d, want %d (all: %v)", i, got[i], tc.wantKinds[i], got)
				}
			}
			// Every line must belong to exactly the region that claims it, and
			// the regions must tile the non-blank lines in order.
			for i, r := range scan.regions {
				for k := r.firstLine; k <= r.lastLine; k++ {
					if scan.lines[k].region != int32(i) {
						t.Errorf("line %d claims region %d, want %d", k, scan.lines[k].region, i)
					}
				}
			}
		})
	}
}

func TestCrossPlatformCoverageMarkdownScanInterruptsParagraph(t *testing.T) {
	// A construct that cannot interrupt a paragraph must not produce a safe
	// boundary, because splitting there changes how the text above it parses.
	for _, tc := range []struct {
		name       string
		content    string
		limit      int
		wantSafe   bool
		wantDegrad string
	}{
		{"ordered list starting at one interrupts", "para text\n1. item", 10, true, ""},
		// `7.` cannot interrupt a paragraph, so the marker line is paragraph
		// text and the boundary is a paragraph split, not a block boundary.
		{"ordered list starting past one does not", "para text\n7. item", 10, false, "paragraph_split"},
		{"unordered list interrupts", "para text\n- item", 10, true, ""},
		{"heading interrupts", "para text\n## head", 10, true, ""},
		// The limit must leave room for the whole fence in the second chunk;
		// otherwise the fence itself needs splitting and that is a separate case.
		{"fence interrupts", "para text\n```go\nx\n```", 12, true, ""},
		{"quote interrupts", "para text\n> quoted", 10, true, ""},
		{"html type 6 interrupts", "para text\n<div>x</div>", 13, true, ""},
		{"asterisk rule interrupts", "para text\n***", 10, true, ""},
		{"dash rule does not interrupt", "para text\n---", 10, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := SplitMarkdownForAppend(tc.content, tc.limit)
			if tc.wantSafe && plan.Degraded() {
				t.Errorf("expected a clean split, got %v", degradationKinds(plan))
			}
			if tc.wantDegrad != "" {
				if got := strings.Join(degradationKinds(plan), ","); !strings.Contains(got, tc.wantDegrad) {
					t.Errorf("degradations = %q, want to contain %q", got, tc.wantDegrad)
				}
			}
		})
	}
}
