package helpers

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// DefaultMarkdownChunkRunes lives in doc_write_pipeline.go and is the shared
// limit for every chunked markdown write path.

// MarkdownChunkPlan is the result of splitting markdown for append-mode writes.
//
// The contract is deliberately NOT strings.Join(Chunks, "") == content: the
// server's mode=append inserts a brand new structure per call, so keeping a
// table or a fenced code block intact across a boundary requires re-emitting its
// header or its fence. Instead:
//
//   - len(Chunks) >= 1 always, so Chunks[0] is safe to index.
//   - every chunk is at most Limit runes when Limit > 0.
//   - every chunk is a complete, self-contained top-level block sequence: no
//     half table, no unclosed fence, and no partial line unless a hard-split
//     degradation is recorded for that boundary.
//   - every boundary that changes the rendered structure appears in
//     Degradations. An empty Degradations means ExpectedDocument() renders
//     identically to the input.
type MarkdownChunkPlan struct {
	Chunks       []string              `json:"-"`
	Limit        int                   `json:"limit"`
	Degradations []MarkdownDegradation `json:"degradations,omitempty"`
}

// MarkdownDegradation records one boundary that could not be made invisible.
type MarkdownDegradation struct {
	Kind string `json:"kind"`
	Tier string `json:"tier"`
	// ChunkIndex is the chunk *before* the boundary this describes.
	ChunkIndex int    `json:"chunkIndex"`
	Line       int    `json:"line"`
	Detail     string `json:"detail"`
	// InjectedSuffix was appended to Chunks[ChunkIndex] and InjectedPrefix was
	// prepended to Chunks[ChunkIndex+1] — the two halves of one repair, which
	// live in different chunks and are therefore reported separately. They exist
	// so a caller can strip them and recover the original content exactly.
	InjectedSuffix string `json:"injectedSuffix,omitempty"`
	InjectedPrefix string `json:"injectedPrefix,omitempty"`
}

// Degraded reports whether any boundary changed the rendered structure.
func (p MarkdownChunkPlan) Degraded() bool { return len(p.Degradations) > 0 }

// ExpectedDocument is the document the server is expected to hold once every
// chunk has been appended. Readback verification must compare against this
// rather than the original content, because repaired boundaries legitimately
// differ from the input.
//
// It is a method rather than a field so a large document is not duplicated in
// memory unless a caller actually verifies.
func (p MarkdownChunkPlan) ExpectedDocument() string {
	if len(p.Chunks) == 1 {
		return p.Chunks[0]
	}
	return strings.Join(p.Chunks, "\n\n")
}

// Warnings renders one human-readable line per distinct degradation kind,
// aggregated with a count and the affected line numbers.
func (p MarkdownChunkPlan) Warnings() []string {
	if len(p.Degradations) == 0 {
		return nil
	}
	order := make([]string, 0, len(p.Degradations))
	detail := map[string]string{}
	lines := map[string][]int{}
	for _, d := range p.Degradations {
		if _, seen := detail[d.Kind]; !seen {
			order = append(order, d.Kind)
			detail[d.Kind] = d.Detail
		}
		lines[d.Kind] = append(lines[d.Kind], d.Line)
	}
	out := make([]string, 0, len(order))
	for _, kind := range order {
		at := lines[kind]
		if len(at) == 1 {
			out = append(out, fmt.Sprintf("内容过长已分片：%s（第 %d 行）", detail[kind], at[0]))
			continue
		}
		out = append(out, fmt.Sprintf("内容过长已分片：%s（%d 处，首次在第 %d 行）",
			detail[kind], len(at), at[0]))
	}
	return out
}

// Summary is the structured projection for command envelopes.
func (p MarkdownChunkPlan) Summary() map[string]any {
	return map[string]any{
		"chunks":       len(p.Chunks),
		"limit":        p.Limit,
		"degraded":     p.Degraded(),
		"degradations": p.Degradations,
	}
}

// SplitMarkdownForAppend splits content into chunks that are each safe to send
// as an independent update_document mode=append call. See MarkdownChunkPlan for
// the exact contract.
//
// limitRunes <= 0 disables splitting and returns the content as a single chunk,
// which callers use as an explicit "send it in one call whatever the size"
// escape hatch.
func SplitMarkdownForAppend(content string, limitRunes int) MarkdownChunkPlan {
	normalized := normalizeMarkdownNewlines(content)
	plan := MarkdownChunkPlan{Limit: limitRunes}
	if limitRunes <= 0 || utf8.RuneCountInString(normalized) <= limitRunes {
		plan.Chunks = []string{normalized}
		return plan
	}
	scan := scanMarkdownStructure(normalized)
	plan = scan.emit(plan)
	if len(plan.Chunks) > 1 && scan.linkRefs > 0 {
		plan.Degradations = append(plan.Degradations, MarkdownDegradation{
			Kind: "link_reference_split", Tier: "soft", ChunkIndex: 0, Line: 0,
			Detail: "文档含链接引用定义，分片后引用与定义可能不在同一片，链接会失效",
		})
	}
	return plan
}

// normalizeMarkdownNewlines converts CRLF and lone CR to LF. Normalizing rather
// than deleting matters: deleting a lone CR silently joins two lines.
//
// This runs unconditionally, including on the single-chunk path, so short and
// long content never disagree about line endings.
func normalizeMarkdownNewlines(content string) string {
	if !strings.ContainsRune(content, '\r') {
		return content
	}
	return strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
}

// emit walks the content, choosing the best available boundary for each window.
//
// A chunk covering [a, b) is rendered as injection(a).prefix + content[a:b] +
// injection(b).suffix, so both halves of a repair are decided by position and a
// hard split can never leave a fence hanging open.
func (s *markdownScan) emit(plan MarkdownChunkPlan) MarkdownChunkPlan {
	limit := plan.Limit
	start, startRune := 0, 0
	skipReported := false
	for {
		prefix, owed, dropped := s.effectiveInjection(start, limit)
		if dropped && !skipReported {
			skipReported = true
			plan.recordKind(s.lineOf(start), "repair_skipped", "soft",
				"该结构的修复文本本身就超过分片上限，已放弃保留其结构")
		}
		prefixRunes := utf8.RuneCountInString(prefix)
		if s.runesFrom(startRune) <= limit-prefixRunes {
			// The tail carries the region's own terminator, so nothing is owed.
			plan.Chunks = append(plan.Chunks, prefix+s.content[start:])
			return plan
		}

		cand, ok := s.pick(start, startRune, limit-prefixRunes, limit)
		if !ok {
			reserve := 0
			if owed != "" {
				reserve = utf8.RuneCountInString(owed) + 1
			}
			cand = s.hardCandidate(start, startRune, limit-prefixRunes-reserve)
		}

		body := s.content[start:cand.offset]
		if !cand.keepTrailing {
			body = strings.TrimRight(body, " \t\n")
		}
		_, suffix, _ := s.effectiveInjection(cand.offset, limit)
		sep := ""
		if suffix != "" && body != "" && !strings.HasSuffix(body, "\n") {
			sep = "\n" // the closing marker must start its own line
		}
		chunk := prefix + body + sep + suffix
		if strings.TrimSpace(chunk) != "" {
			plan.Chunks = append(plan.Chunks, chunk)
			plan.record(s, cand, len(plan.Chunks)-1, suffix, limit)
		}
		start, startRune = cand.offset, cand.runeIdx
	}
}

// effectiveInjection is injectionAt with the repair dropped when it cannot fit,
// which is the only way to keep the per-window budget positive for a structure
// whose own header or fence line is longer than the limit.
func (s *markdownScan) effectiveInjection(offset, limit int) (prefix, suffix string, dropped bool) {
	p, sfx := s.injectionAt(offset)
	if p == "" && sfx == "" {
		return "", "", false
	}
	prefixCost, suffixCost := s.injectionCostAt(offset)
	if prefixCost+suffixCost >= limit {
		return "", "", true
	}
	return p, sfx, false
}

// pick returns the best boundary in [start, start+budget]: the highest tier with
// any candidate, and within that tier the latest one.
//
// Taking the latest candidate within a tier is not just an optimization. Every
// chunk is appended to the same document, so within tierSafe the choice has no
// effect on the final document at all — cutting early only costs extra chunks
// and extra round trips.
func (s *markdownScan) pick(start, startRune, budget, limit int) (splitCandidate, bool) {
	maxRune := startRune + budget
	for tier := splitTier(0); tier < splitTierCount; tier++ {
		list := s.byTier[tier]
		// Candidates are stored in ascending offset, so the binary search finds
		// the first one past the window; everything before it is a candidate to
		// walk back through.
		hi := sort.Search(len(list), func(k int) bool { return s.cands[list[k]].trimRuneIdx > maxRune })
		for j := hi - 1; j >= 0; j-- {
			c := s.cands[list[j]]
			if c.offset <= start {
				break // this tier is exhausted within the window
			}
			prefix, suffix := s.injectionCostAt(c.offset)
			if c.trimRuneIdx+suffix > maxRune {
				continue // the closing injection does not fit
			}
			if prefix+suffix >= limit {
				// The repair would consume the whole next window, so this
				// candidate can never make progress. Rejecting it here is what
				// removes the "repair does not fit" livelock: every accepted
				// candidate leaves the next window at least one rune.
				continue
			}
			return c, true
		}
	}
	return splitCandidate{}, false
}

// hardCandidate is the last resort, used only when the window holds no
// structural boundary at all — in practice, a single line longer than the limit.
// It prefers the last whitespace within a bounded lookback so words survive,
// and falls back to an exact rune cut.
func (s *markdownScan) hardCandidate(start, startRune, budget int) splitCandidate {
	// budget is always at least 1: effectiveInjection drops any repair whose
	// prefix plus suffix would reach the limit, so the caller's
	// limit-prefix-reserve arithmetic cannot go non-positive.
	const lookback = 64
	off, n := start, 0
	spaceOff, spaceRune := -1, 0
	for off < len(s.content) && n < budget {
		r, size := utf8.DecodeRuneInString(s.content[off:])
		off += size
		n++
		// Deliberately not '\n': a line boundary is a structural decision, and
		// the tiers above already rejected every one in this window. Cutting at
		// a newline here would resurrect exactly the boundaries they refused —
		// a setext underline, for instance.
		if r == ' ' || r == '\t' {
			spaceOff, spaceRune = off, n
		}
	}
	kind := candHardRune
	if spaceOff > start && n-spaceRune <= lookback {
		kind, off, n = candHardWhitespace, spaceOff, spaceRune
	}
	if s.offsetInHTML(start) {
		// A hard cut inside HTML breaks a tag (this is a single oversized HTML
		// line with no safe boundary). Flag it so the caller can point the user
		// at uploading the block as a file instead.
		kind = candHardHTML
	}
	return splitCandidate{
		offset: off, runeIdx: startRune + n, line: int32(s.lineOf(off)),
		region: -1, tier: tierRepair, kind: kind, keepTrailing: true,
	}
}

// offsetInHTML reports whether the byte offset sits inside an HTML region.
func (s *markdownScan) offsetInHTML(offset int) bool {
	r := s.lines[s.lineIndexOf(offset)].region
	return r >= 0 && s.regions[r].kind == regionHTML
}

// lineOf returns the 1-based line number containing the given byte offset. The
// first line always starts at 0, so the search never returns 0.
func (s *markdownScan) lineOf(offset int) int {
	return sort.Search(len(s.lines), func(k int) bool { return s.lines[k].start > offset })
}

// record appends a degradation for the boundary, if it changed anything.
func (p *MarkdownChunkPlan) record(s *markdownScan, c splitCandidate, chunkIndex int, suffix string, limit int) {
	kind, tier, detail := degradationFor(c.kind)
	if kind == "" {
		return
	}
	// Report the injection that was actually used, not the one injectionAt would
	// like to use: when the repair is dropped for not fitting, nothing is added.
	prefix, _, _ := s.effectiveInjection(c.offset, limit)
	p.Degradations = append(p.Degradations, MarkdownDegradation{
		Kind: kind, Tier: tier, ChunkIndex: chunkIndex, Line: int(c.line),
		Detail: detail, InjectedSuffix: suffix, InjectedPrefix: prefix,
	})
}

// recordKind appends a degradation that no candidate describes.
func (p *MarkdownChunkPlan) recordKind(line int, kind, tier, detail string) {
	p.Degradations = append(p.Degradations, MarkdownDegradation{
		Kind: kind, Tier: tier, ChunkIndex: len(p.Chunks), Line: line, Detail: detail,
	})
}

func degradationFor(kind candidateKind) (string, string, string) {
	switch kind {
	case candParagraphLine:
		return "paragraph_split", "soft", "长段落被拆成多个段落"
	case candListItem:
		return "list_split", "soft", "列表被拆成多个列表"
	case candQuoteLine:
		return "blockquote_split", "soft", "引用块被拆成多个引用块"
	case candRegionBoundary:
		return "block_boundary_split", "soft", "在块边界处切分，前后两块的解析结果可能与整体不同"
	case candIndentedCodeLine:
		return "indented_code_split", "soft", "缩进代码块被拆成多个代码块"
	case candHTMLLine:
		return "html_block_split", "soft", "HTML 块被拆开，标签可能不再配对；内容很大时建议存成文件后用 dws doc import 导入"
	case candHTMLRepair:
		return "html_block_split", "repair", "<pre>/<style>/<textarea> 块被拆成多个，每片各自重开并闭合标签；内容很大时可存成文件后用 dws doc import 导入"
	case candOrderedListItem:
		return "ordered_list_split", "repair", "有序列表被拆开，后续分片的编号可能从 1 重新开始"
	case candTableRow:
		return "table_split", "repair", "表格被拆成多个表格，后续分片重复表头行与分隔行"
	case candFenceBody:
		return "code_block_split", "repair", "代码块被拆成多个代码块，每片各自闭合围栏"
	case candHardWhitespace:
		return "hard_split_at_whitespace", "rune", "无结构切分点，已在空白处硬切"
	case candHardRune:
		return "hard_rune_split", "rune", "单行长度超过上限，已按字符硬切"
	case candHardHTML:
		return "html_tag_hard_split", "rune", "HTML 内容无法在不破坏标签的前提下切分，已硬切；建议将内容存成文件后用 dws doc import 导入"
	default:
		return "", "", "" // tierSafe boundaries change nothing
	}
}
