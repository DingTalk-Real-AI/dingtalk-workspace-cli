package helpers

import (
	"strings"
	"unicode/utf8"
)

// This file implements the structural scan behind SplitMarkdownForAppend.
//
// Why a scan instead of "parse into blocks, then re-join them": the server's
// update_document mode=append always inserts a brand new structure, so a chunk
// boundary can never be continued by the next chunk. Every chunk must therefore
// stand alone as a complete top-level block sequence. The scan records *offsets*
// of positions where cutting is legal, graded by how much damage the cut does,
// and never rebuilds text — which is what makes "no content is lost" a
// structural property rather than something each code path must remember.
//
// Three passes, each O(n):
//
//  1. classifyLines  — fence-aware line classification. Must be exact, because
//     a fence interior may never be read as structure.
//  2. groupRegions   — group non-blank runs into regions, with intra-run
//     lookahead (a table's delimiter row is the *second* line of the region and
//     its header is the line *before* that, so one forward pass cannot see it).
//  3. emitCandidates — derive split candidates from (prev line, line, region).
//
// Invariant: any misclassification must resolve toward the *lower* (more
// conservative) tier. Guessing "safe" corrupts documents; guessing "unsafe"
// only costs an extra chunk.

// splitTier grades a split position by how much it changes the rendered result.
type splitTier uint8

const (
	// tierSafe splits change nothing: no injected characters, and the two
	// halves render exactly as the whole did.
	tierSafe splitTier = iota
	// tierSoft splits inject nothing but do change structure — a paragraph
	// becomes two paragraphs, a list becomes two lists.
	tierSoft
	// tierRepair splits must inject characters to keep each half self-contained
	// (a repeated table header, a closed-and-reopened fence).
	tierRepair
)

// splitTierCount is the number of stored tiers. Hard splits are synthesized on
// demand and never stored, so they are not counted here.
const splitTierCount = 3

// srcSpan is a byte range in the normalized content, excluding any trailing
// newline. Spans keep repair payloads out of memory until one is actually used:
// a 5000-row table stores its header once, not once per candidate row.
type srcSpan struct{ start, end int }

type lineKind uint8

const (
	lineParagraph lineKind = iota
	lineBlank
	lineATXHeading
	lineFenceOpen
	lineFenceBody
	lineFenceClose
	lineSetextUnderline    // a run of '=' only; unambiguous
	lineThematicBreakDash  // a run of '-'; ALSO a setext underline candidate
	lineThematicBreakOther // '*' or '_' form; cannot be a setext underline
	lineTableDelim
	lineListItemStart
	lineQuote
	lineHTMLStart
	lineIndentedCode
	lineLinkRefDef
)

type regionKind uint8

const (
	regionParagraph regionKind = iota
	regionFence
	regionTable
	regionList
	regionQuote
	regionHTML
	regionIndentedCode
	regionSetext
	regionLeaf // a single-line block: heading, thematic break, link ref def
)

type lineInfo struct {
	start     int // byte offset of the first character
	nlEnd     int // byte offset past the trailing '\n' (== end of content if none)
	runeStart int // rune index of start; makes every budget comparison O(1)
	indent    int // leading whitespace in columns, tab = 4
	kind      lineKind
	region    int32 // index into markdownScan.regions
	// fence bookkeeping, set on lineFenceOpen
	fenceChar  byte
	fenceCount int
	// list bookkeeping, set on lineListItemStart
	ordered    bool
	contentCol int // column where the item's content starts
	// html bookkeeping, set on lineHTMLStart
	htmlKind uint8
}

type region struct {
	kind      regionKind
	firstLine int
	lastLine  int
	// regionFence
	fenceOpen   srcSpan // the original opening line, indentation and info string included
	fenceMarker srcSpan // just the run of ` or ~, so a 5-backtick fence closes with 5
	// regionTable
	header srcSpan
	delim  srcSpan
	// regionList
	ordered bool
	// regionHTML repair (only for <pre>/<style>/<textarea>; never <script>)
	htmlRepair      bool
	htmlOpen        srcSpan // the full opening tag, including a multi-line one
	htmlOpenEndLine int     // line index the opening tag's '>' sits on
	htmlCloser      string  // e.g. "</pre>"
}

type candidateKind uint8

const (
	candBlankLine candidateKind = iota
	candBlockStart
	candBlockEnd
	candParagraphLine
	candListItem
	candOrderedListItem
	candQuoteLine
	candTableRow
	candFenceBody
	candRegionBoundary
	candIndentedCodeLine
	candHTMLLine
	candHTMLRepair
	// The hard kinds are synthesized on demand by hardCandidate and never
	// stored in the candidate list.
	candHardWhitespace
	candHardRune
	candHardHTML
)

type splitCandidate struct {
	offset  int
	runeIdx int
	// trimRuneIdx is runeIdx with the trailing whitespace that emit() will trim
	// already discounted. Budgeting against runeIdx would charge the chunk for
	// blank lines it is about to drop, which loses one safe boundary per blank
	// run and makes the greedy choice needlessly early.
	trimRuneIdx int
	line        int32 // 1-based, for diagnostics only
	region      int32 // -1 when no repair is involved
	tier        splitTier
	kind        candidateKind
	// keepTrailing suppresses trailing-whitespace trimming of the emitted
	// chunk. Only set inside a fence, where trailing blank lines are content.
	keepTrailing bool
}

type markdownScan struct {
	content   string
	totalRune int
	lines     []lineInfo
	regions   []region
	cands     []splitCandidate
	byTier    [splitTierCount][]int32 // candidate indices, ascending offset
	linkRefs  int
}

// scanMarkdownStructure runs all three passes over already-normalized content.
func scanMarkdownStructure(content string) *markdownScan {
	s := &markdownScan{content: content, totalRune: utf8.RuneCountInString(content)}
	s.classifyLines()
	s.groupRegions()
	s.emitCandidates()
	return s
}

// runesFrom reports how many runes remain from the given rune index.
func (s *markdownScan) runesFrom(runeIdx int) int { return s.totalRune - runeIdx }

// ── Pass 1: fence-aware line classification ─────────────────────────────────

func (s *markdownScan) classifyLines() {
	var (
		inFence    bool
		fenceChar  byte
		fenceCount int
	)
	offset, runeIdx := 0, 0
	for offset <= len(s.content) {
		nl := strings.IndexByte(s.content[offset:], '\n')
		var raw string
		var nlEnd int
		if nl < 0 {
			raw, nlEnd = s.content[offset:], len(s.content)
		} else {
			raw, nlEnd = s.content[offset:offset+nl], offset+nl+1
		}

		li := lineInfo{start: offset, nlEnd: nlEnd, runeStart: runeIdx, region: -1}
		li.indent = leadingIndent(raw)
		trimmed := strings.TrimSpace(raw)

		switch {
		case inFence:
			if ch, n, ok := fenceMarkerOf(trimmed); ok && li.indent <= 3 &&
				ch == fenceChar && n >= fenceCount && closingFenceIsBare(trimmed, ch, n) {
				li.kind = lineFenceClose
				inFence = false
			} else {
				li.kind = lineFenceBody
			}
		case trimmed == "":
			li.kind = lineBlank
		case li.indent <= 3 && isFenceOpen(trimmed):
			ch, n, _ := fenceMarkerOf(trimmed)
			li.kind, li.fenceChar, li.fenceCount = lineFenceOpen, ch, n
			inFence, fenceChar, fenceCount = true, ch, n
		case li.indent >= 4:
			li.kind = lineIndentedCode
		case isATXHeading(trimmed):
			li.kind = lineATXHeading
		case isRunOf(trimmed, '='):
			li.kind = lineSetextUnderline
		case isRunOf(trimmed, '-'):
			li.kind = lineThematicBreakDash
		case isThematicBreak(trimmed, '*') || isThematicBreak(trimmed, '_'):
			li.kind = lineThematicBreakOther
		case trimmed[0] == '>':
			li.kind = lineQuote
		case isTableDelimiterRow(trimmed):
			li.kind = lineTableDelim
		default:
			if ordered, contentCol, ok := listMarkerOf(raw); ok {
				li.kind, li.ordered, li.contentCol = lineListItemStart, ordered, contentCol
			} else if hk, ok := htmlBlockKind(trimmed); ok {
				li.kind, li.htmlKind = lineHTMLStart, hk
			} else if isLinkReferenceDefinition(trimmed) {
				li.kind = lineLinkRefDef
				s.linkRefs++
			} else {
				li.kind = lineParagraph
			}
		}

		s.lines = append(s.lines, li)
		runeIdx += utf8.RuneCountInString(raw)
		if nl < 0 {
			break
		}
		runeIdx++ // the '\n'
		offset = nlEnd
	}
}

func leadingIndent(line string) int {
	col := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			col++
		case '\t':
			col += 4 - col%4
		default:
			return col
		}
	}
	return col
}

// fenceMarkerOf returns the fence character and its run length when the line
// starts with at least three backticks or tildes.
func fenceMarkerOf(trimmed string) (byte, int, bool) {
	if trimmed == "" {
		return 0, 0, false
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return 0, 0, false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == ch {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	return ch, n, true
}

func isFenceOpen(trimmed string) bool {
	ch, n, ok := fenceMarkerOf(trimmed)
	if !ok {
		return false
	}
	// A backtick info string may not contain a backtick.
	return ch != '`' || !strings.Contains(trimmed[n:], "`")
}

// closingFenceIsBare reports whether the line is only the fence marker, which
// CommonMark requires of a closing fence.
func closingFenceIsBare(trimmed string, ch byte, n int) bool {
	rest := trimmed[n:]
	return strings.TrimRight(rest, " \t") == "" || strings.Trim(rest, string(ch)+" \t") == ""
}

func isATXHeading(trimmed string) bool {
	n := 0
	for n < len(trimmed) && trimmed[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return false
	}
	rest := trimmed[n:]
	return rest == "" || rest[0] == ' ' || rest[0] == '\t'
}

// isRunOf reports whether the line consists solely of the given character. The
// empty string is not a run: callers have already excluded blank lines and empty
// table cells.
func isRunOf(trimmed string, ch byte) bool {
	if trimmed == "" {
		return false
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != ch {
			return false
		}
	}
	return true
}

// isThematicBreak reports whether the line is three or more of ch, allowing
// interior spaces.
func isThematicBreak(trimmed string, ch byte) bool {
	n := 0
	for i := 0; i < len(trimmed); i++ {
		switch trimmed[i] {
		case ch:
			n++
		case ' ', '\t':
		default:
			return false
		}
	}
	return n >= 3
}

// isTableDelimiterRow matches a GFM delimiter row such as `|---|:--:|`.
//
// A pipe is required: without one, `---` is a thematic break or a setext
// underline, never a table delimiter.
func isTableDelimiterRow(trimmed string) bool {
	if !strings.ContainsRune(trimmed, '|') {
		return false
	}
	body := strings.TrimSuffix(strings.TrimPrefix(trimmed, "|"), "|")
	if body == "" || !strings.ContainsRune(trimmed, '-') {
		return false
	}
	for _, cell := range strings.Split(body, "|") {
		cell = strings.TrimSpace(cell)
		cell = strings.TrimPrefix(cell, ":")
		cell = strings.TrimSuffix(cell, ":")
		if cell == "" || !isRunOf(cell, '-') {
			return false
		}
	}
	return true
}

// tableCellCount counts the cells of a table row, used to check that a
// delimiter row matches the header above it.
func tableCellCount(trimmed string) int {
	if !strings.ContainsRune(trimmed, '|') {
		return 0
	}
	body := strings.TrimSuffix(strings.TrimPrefix(trimmed, "|"), "|")
	return len(strings.Split(body, "|"))
}

// listMarkerOf recognizes a bullet or ordered list marker and reports the
// column where the item's content begins.
func listMarkerOf(raw string) (ordered bool, contentCol int, ok bool) {
	indent := leadingIndent(raw)
	rest := strings.TrimLeft(raw, " \t")
	if rest == "" {
		return false, 0, false
	}
	width := 0
	switch rest[0] {
	case '-', '+', '*':
		width = 1
	default:
		digits := 0
		for digits < len(rest) && digits < 9 && rest[digits] >= '0' && rest[digits] <= '9' {
			digits++
		}
		if digits == 0 || digits >= len(rest) || (rest[digits] != '.' && rest[digits] != ')') {
			return false, 0, false
		}
		ordered, width = true, digits+1
	}
	after := rest[width:]
	if after != "" && after[0] != ' ' && after[0] != '\t' {
		return false, 0, false
	}
	spaces := 0
	for spaces < len(after) && (after[spaces] == ' ' || after[spaces] == '\t') {
		spaces++
	}
	if spaces > 4 {
		spaces = 1
	}
	return ordered, indent + width + spaces, true
}

// listMarkerNumber returns the leading number of an ordered list marker.
func listMarkerNumber(raw string) int {
	rest := strings.TrimLeft(raw, " \t")
	n := 0
	for i := 0; i < len(rest) && rest[i] >= '0' && rest[i] <= '9'; i++ {
		n = n*10 + int(rest[i]-'0')
	}
	return n
}

// htmlBlockKind classifies an HTML block start into CommonMark's types 1-7.
// Only the terminator behaviour matters here: type 1 spans blank lines and ends
// at its closing tag, types 2-5 have their own terminators, 6-7 end at a blank
// line.
func htmlBlockKind(trimmed string) (uint8, bool) {
	if trimmed == "" || trimmed[0] != '<' {
		return 0, false
	}
	lower := strings.ToLower(trimmed)
	for _, name := range []string{"<script", "<pre", "<style", "<textarea"} {
		if strings.HasPrefix(lower, name) {
			return 1, true
		}
	}
	switch {
	case strings.HasPrefix(trimmed, "<!--"):
		return 2, true
	case strings.HasPrefix(trimmed, "<?"):
		return 3, true
	case strings.HasPrefix(trimmed, "<!") && len(trimmed) > 2 && trimmed[2] >= 'A' && trimmed[2] <= 'Z':
		return 4, true
	case strings.HasPrefix(trimmed, "<![CDATA["):
		return 5, true
	}
	name := strings.TrimLeft(lower[1:], "/")
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return 0, false // a tag name must start with a letter
	}
	end := 0
	for end < len(name) && (name[end] >= 'a' && name[end] <= 'z' || name[end] >= '0' && name[end] <= '9') {
		end++
	}
	if htmlBlockTags[name[:end]] {
		return 6, true
	}
	return 7, true
}

var htmlBlockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "base": true, "blockquote": true,
	"body": true, "caption": true, "center": true, "col": true, "colgroup": true,
	"dd": true, "details": true, "dialog": true, "dir": true, "div": true, "dl": true,
	"dt": true, "fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"form": true, "frame": true, "frameset": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true, "head": true, "header": true, "hr": true,
	"html": true, "iframe": true, "legend": true, "li": true, "link": true, "main": true,
	"menu": true, "menuitem": true, "nav": true, "noframes": true, "ol": true,
	"optgroup": true, "option": true, "p": true, "param": true, "section": true,
	"source": true, "summary": true, "table": true, "tbody": true, "td": true,
	"tfoot": true, "th": true, "thead": true, "title": true, "tr": true, "track": true,
	"ul": true,
}

// isLinkReferenceDefinition matches `[label]: destination`, whose definition
// must not be separated from the links that use it.
func isLinkReferenceDefinition(trimmed string) bool {
	if trimmed == "" || trimmed[0] != '[' {
		return false
	}
	close := strings.Index(trimmed, "]:")
	return close > 1
}

// ── Pass 2: run grouping and region assignment ───────────────────────────────

func (s *markdownScan) groupRegions() {
	i := 0
	for i < len(s.lines) {
		if s.lines[i].kind == lineBlank {
			i++
			continue
		}
		i = s.groupOneRegion(i)
	}
}

// groupOneRegion assigns the region starting at line i and returns the index of
// the first line after it.
func (s *markdownScan) groupOneRegion(i int) int {
	switch s.lines[i].kind {
	case lineFenceOpen:
		return s.groupFence(i)
	case lineIndentedCode:
		return s.groupIndentedCode(i)
	case lineQuote:
		return s.groupSimple(i, s.quoteEnd(i), region{kind: regionQuote})
	case lineListItemStart:
		return s.groupList(i)
	case lineHTMLStart:
		last := s.htmlEnd(i)
		return s.groupSimple(i, last, s.htmlRegion(i, last))
	case lineATXHeading, lineThematicBreakOther, lineLinkRefDef:
		return s.groupSimple(i, i, region{kind: regionLeaf})
	case lineThematicBreakDash:
		// A dash run is a thematic break only when it cannot be read as a
		// setext underline, i.e. when no paragraph precedes it.
		return s.groupSimple(i, i, region{kind: regionLeaf})
	}
	return s.groupParagraphRun(i)
}

func (s *markdownScan) groupSimple(first, last int, r region) int {
	r.firstLine, r.lastLine = first, last
	s.assign(r)
	return last + 1
}

func (s *markdownScan) assign(r region) {
	idx := int32(len(s.regions))
	s.regions = append(s.regions, r)
	for k := r.firstLine; k <= r.lastLine && k < len(s.lines); k++ {
		s.lines[k].region = idx
	}
}

func (s *markdownScan) groupFence(i int) int {
	last := i
	for last+1 < len(s.lines) && s.lines[last].kind != lineFenceClose {
		last++
		if s.lines[last].kind == lineFenceClose {
			break
		}
	}
	if s.lines[last].kind != lineFenceClose {
		last = len(s.lines) - 1 // unterminated fence swallows the rest
	}
	open := s.lines[i]
	markerStart := open.start
	for markerStart < open.nlEnd && (s.content[markerStart] == ' ' || s.content[markerStart] == '\t') {
		markerStart++
	}
	return s.groupSimple(i, last, region{
		kind:        regionFence,
		fenceOpen:   srcSpan{open.start, s.lineTextEnd(i)},
		fenceMarker: srcSpan{markerStart, markerStart + open.fenceCount},
	})
}

func (s *markdownScan) groupIndentedCode(i int) int {
	last := i
	for k := i + 1; k < len(s.lines); k++ {
		if s.lines[k].kind == lineIndentedCode {
			last = k
			continue
		}
		if s.lines[k].kind == lineBlank {
			continue // a blank line only ends the block if no indented line follows
		}
		break
	}
	return s.groupSimple(i, last, region{kind: regionIndentedCode})
}

func (s *markdownScan) quoteEnd(i int) int {
	last := i
	for k := i + 1; k < len(s.lines); k++ {
		if s.lines[k].kind == lineBlank {
			break
		}
		// Lazy continuation: a plain paragraph line continues the quote.
		if s.lines[k].kind != lineQuote && s.lines[k].kind != lineParagraph {
			break
		}
		last = k
	}
	return last
}

func (s *markdownScan) htmlEnd(i int) int {
	if s.lines[i].htmlKind >= 6 {
		last := i
		for k := i + 1; k < len(s.lines) && s.lines[k].kind != lineBlank; k++ {
			last = k
		}
		return last
	}
	if s.lines[i].htmlKind == 1 {
		// Type 1 covers <script>, <pre>, <style> and <textarea>, each with its own
		// closing tag. CommonMark ends the block on the first line containing any
		// of the four closers, so searching for a single fixed tag (e.g. only
		// </script>) would let a <pre>/<style>/<textarea> block swallow the rest
		// of the document — including a following table, which would then be split
		// without its header re-emitted.
		for k := i; k < len(s.lines); k++ {
			if lineHasType1Closer(strings.ToLower(s.lineText(k))) {
				return k
			}
		}
		return len(s.lines) - 1
	}
	closer := htmlCloser(s.lines[i].htmlKind)
	for k := i; k < len(s.lines); k++ {
		if strings.Contains(strings.ToLower(s.lineText(k)), closer) {
			return k
		}
	}
	return len(s.lines) - 1
}

// lineHasType1Closer reports whether a lowercased line contains any of the four
// CommonMark type-1 HTML closing tags. Type 1 ends on whichever appears first,
// regardless of which opened the block.
func lineHasType1Closer(lower string) bool {
	return strings.Contains(lower, "</script>") ||
		strings.Contains(lower, "</pre>") ||
		strings.Contains(lower, "</style>") ||
		strings.Contains(lower, "</textarea>")
}

// htmlCloser returns the terminator for HTML block types 2-5. Type 1 is handled
// by lineHasType1Closer in htmlEnd (it has four possible closers, not one), and
// types 6-7 end at a blank line; do not route those through here.
func htmlCloser(kind uint8) string {
	switch kind {
	case 2:
		return "-->"
	case 3:
		return "?>"
	case 4:
		return ">"
	case 5:
		return "]]>"
	default:
		return "</script>"
	}
}

// htmlRegion builds a regionHTML, enabling <pre>/<style>/<textarea> repair when
// the block has the canonical shape: a self-contained opening tag, a matching
// closer on the last line, and at least one line of interior between them.
// <script> is deliberately never repaired — splitting JavaScript across <script>
// tags reliably breaks execution. Anything irregular falls back to a plain
// regionHTML (line-boundary split, no reopen).
func (s *markdownScan) htmlRegion(first, last int) region {
	r := region{kind: regionHTML}
	tag, ok := htmlType1RepairTag(strings.ToLower(strings.TrimSpace(s.lineText(first))))
	if !ok {
		return r
	}
	openEndOff, openEndLine, found := s.htmlTagEnd(first)
	if !found || openEndLine >= last {
		return r
	}
	closer := "</" + tag + ">"
	if !strings.Contains(strings.ToLower(s.lineText(last)), closer) {
		return r // not cleanly terminated by the matching closer
	}
	r.htmlRepair = true
	r.htmlOpen = srcSpan{s.lines[first].start, openEndOff}
	r.htmlOpenEndLine = openEndLine
	r.htmlCloser = closer
	return r
}

// htmlType1RepairTag returns the tag name when the line opens a repairable type-1
// block. The tag must be followed by whitespace, '>' or end of line so that e.g.
// "<presentation" is not mistaken for "<pre".
func htmlType1RepairTag(lower string) (string, bool) {
	for _, tag := range []string{"pre", "style", "textarea"} {
		p := "<" + tag
		if !strings.HasPrefix(lower, p) {
			continue
		}
		rest := lower[len(p):]
		if rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '>' {
			return tag, true
		}
	}
	return "", false
}

// htmlTagEnd finds the '>' that closes the tag opening on line `first`, which may
// sit on a later line. It returns the byte offset just past the '>' and its line.
func (s *markdownScan) htmlTagEnd(first int) (offset, line int, ok bool) {
	for k := first; k < len(s.lines); k++ {
		text := s.content[s.lines[k].start:s.lineTextEnd(k)]
		if idx := strings.IndexByte(text, '>'); idx >= 0 {
			return s.lines[k].start + idx + 1, k, true
		}
	}
	return 0, 0, false
}

func (s *markdownScan) groupList(i int) int {
	marker := s.lines[i]
	last := i
	for k := i + 1; k < len(s.lines); k++ {
		li := s.lines[k]
		if li.kind == lineBlank {
			// The list continues only if the next non-blank line is indented
			// into the item's content column or is another marker.
			n := k + 1
			for n < len(s.lines) && s.lines[n].kind == lineBlank {
				n++
			}
			if n >= len(s.lines) {
				break
			}
			nx := s.lines[n]
			if nx.indent >= marker.contentCol ||
				(nx.kind == lineListItemStart && nx.indent == marker.indent) {
				k = n - 1
				continue
			}
			break
		}
		if li.kind == lineListItemStart && li.indent <= marker.indent+3 {
			last = k
			continue
		}
		if li.indent >= marker.contentCol || li.kind == lineParagraph {
			last = k // nested content or lazy continuation
			continue
		}
		break
	}
	return s.groupSimple(i, last, region{kind: regionList, ordered: marker.ordered})
}

// groupParagraphRun handles a maximal non-blank run that starts with paragraph
// text. Within the run, setext underlines and GFM tables take precedence, in
// that order, mirroring the parser phase order (block parsers first, table
// transformation over already-parsed paragraphs second).
func (s *markdownScan) groupParagraphRun(i int) int {
	end := i
	for end+1 < len(s.lines) && s.lines[end+1].kind != lineBlank {
		// interruptsParagraph is the single source of truth for "does this end
		// the paragraph". Notably a `1.` list item interrupts while a `7.` one
		// does not, so the latter stays paragraph text — the same asymmetry that
		// decides whether the boundary is safe or merely soft.
		if s.interruptsParagraph(end + 1) {
			break
		}
		end++
	}

	// Setext: an '=' or '-' run on a later line turns everything above it into
	// a heading, so `para\n---` is NOT a thematic break.
	for j := i + 1; j <= end; j++ {
		if s.lines[j].kind == lineSetextUnderline || s.lines[j].kind == lineThematicBreakDash {
			s.assign(region{kind: regionSetext, firstLine: i, lastLine: j})
			return j + 1
		}
	}

	// Table: the delimiter row is at k, its header at k-1. Everything from the
	// header to the end of the run becomes table rows — including lines with no
	// pipes at all — so the region does NOT stop at the first non-pipe line.
	for k := i + 1; k <= end; k++ {
		if s.lines[k].kind != lineTableDelim {
			continue
		}
		if tableCellCount(strings.TrimSpace(s.lineText(k-1))) != tableCellCount(strings.TrimSpace(s.lineText(k))) {
			continue
		}
		if k-1 > i {
			s.assign(region{kind: regionParagraph, firstLine: i, lastLine: k - 2})
		}
		s.assign(region{
			kind:      regionTable,
			firstLine: k - 1,
			lastLine:  end,
			header:    srcSpan{s.lines[k-1].start, s.lineTextEnd(k - 1)},
			delim:     srcSpan{s.lines[k].start, s.lineTextEnd(k)},
		})
		return end + 1
	}

	s.assign(region{kind: regionParagraph, firstLine: i, lastLine: end})
	return end + 1
}

// lineTextEnd is the byte offset just past the line's text, excluding '\n'.
func (s *markdownScan) lineTextEnd(i int) int {
	li := s.lines[i]
	if li.nlEnd > li.start && li.nlEnd <= len(s.content) && s.content[li.nlEnd-1] == '\n' {
		return li.nlEnd - 1
	}
	return li.nlEnd
}

func (s *markdownScan) lineText(i int) string {
	return s.content[s.lines[i].start:s.lineTextEnd(i)]
}

// ── Pass 3: candidate emission ───────────────────────────────────────────────

func (s *markdownScan) emitCandidates() {
	for i := 1; i < len(s.lines); i++ {
		c, ok := s.candidateAt(i)
		if !ok {
			continue
		}
		c.trimRuneIdx = c.runeIdx
		if !c.keepTrailing {
			c.trimRuneIdx = s.runeIndexOf(trimTrailingWhitespaceEnd(s.content, c.offset))
		}
		idx := int32(len(s.cands))
		s.cands = append(s.cands, c)
		s.byTier[c.tier] = append(s.byTier[c.tier], idx)
	}
}

func (s *markdownScan) candidateAt(i int) (splitCandidate, bool) {
	li, prev := s.lines[i], s.lines[i-1]
	if li.kind == lineBlank {
		return splitCandidate{}, false // cut before content, never before a blank
	}
	mk := func(tier splitTier, kind candidateKind, regionIdx int32) (splitCandidate, bool) {
		return splitCandidate{
			offset: li.start, runeIdx: li.runeStart, line: int32(i + 1),
			region: regionIdx, tier: tier, kind: kind,
			keepTrailing: kind == candFenceBody || kind == candHTMLRepair,
		}, true
	}

	reg := int32(-1)
	if li.region >= 0 {
		reg = li.region
	}
	inSameRegion := reg >= 0 && prev.region == reg
	startsRegion := reg >= 0 && int(s.regions[reg].firstLine) == i

	// A1 — a blank line at top level separates two independent blocks. This is
	// the strongest guarantee available: nothing renders differently.
	if prev.kind == lineBlank && (prev.region < 0 || !s.regionSpansBlanks(prev.region)) {
		return mk(tierSafe, candBlankLine, -1)
	}

	// A2 — the line starts a construct that provably interrupts a paragraph.
	if startsRegion && s.interruptsParagraph(i) {
		return mk(tierSafe, candBlockStart, -1)
	}

	// A3 — the previous line completed a non-paragraph block and this line
	// starts a new one. Tables are excluded: whether a table interrupts a
	// paragraph is parser-dependent, so splitting there could *create* one.
	if startsRegion && prev.region >= 0 && prev.region != reg &&
		s.regions[prev.region].kind != regionParagraph &&
		s.regions[reg].kind != regionTable {
		return mk(tierSafe, candBlockEnd, -1)
	}

	// A cross-region boundary none of the safe rules accepted: the two halves may
	// parse differently than the whole did — a table header following a paragraph
	// line, or indented code following one. Cutting here is allowed but recorded.
	//
	// Reaching this implies startsRegion: every non-blank line belongs to a
	// region, regions tile contiguous line ranges, and a region that spans blank
	// lines always ends on a non-blank one, so a line whose predecessor sits in a
	// different region can only be its own region's first line.
	if !inSameRegion {
		return mk(tierSoft, candRegionBoundary, -1)
	}

	switch s.regions[reg].kind {
	case regionParagraph:
		// B1 — a soft line break inside a paragraph becomes a paragraph break.
		return mk(tierSoft, candParagraphLine, -1)
	case regionQuote:
		if li.kind == lineQuote && li.indent == s.regions[reg].firstLineIndent(s) {
			return mk(tierSoft, candQuoteLine, -1)
		}
	case regionList:
		if li.kind == lineListItemStart && li.indent == s.lines[s.regions[reg].firstLine].indent {
			if s.regions[reg].ordered {
				// Visible numbering restarts at the server, so this is a
				// heavier change than an unordered split.
				return mk(tierRepair, candOrderedListItem, -1)
			}
			return mk(tierSoft, candListItem, -1)
		}
	case regionTable:
		// B2 — a body row strictly after the delimiter; the next chunk gets the
		// header and delimiter rows re-emitted so it is a table in its own right.
		if i > s.regions[reg].firstLine+1 {
			return mk(tierRepair, candTableRow, reg)
		}
	case regionFence:
		// B2 — close the fence here and reopen it with the original marker and
		// info string on the next chunk.
		if li.kind == lineFenceBody {
			return mk(tierRepair, candFenceBody, reg)
		}
	case regionIndentedCode:
		// Each half stays an indented code block on its own, so no injection is
		// needed — but one code block becomes two.
		if li.kind == lineIndentedCode {
			return mk(tierSoft, candIndentedCodeLine, -1)
		}
	case regionHTML:
		r := &s.regions[reg]
		if r.htmlRepair {
			if i <= r.htmlOpenEndLine {
				// Never cut inside the opening tag (it may span several lines).
				return splitCandidate{}, false
			}
			// B2 — close the tag here and reopen it (with any attributes) on the
			// next chunk, exactly like a code fence.
			return mk(tierRepair, candHTMLRepair, reg)
		}
		// Other tags are not repaired: a line boundary is less destructive than a
		// mid-line cut, but the tags may no longer balance.
		return mk(tierSoft, candHTMLLine, -1)
	}
	return splitCandidate{}, false
}

// firstLineIndent is the indent of the region's first line.
func (r region) firstLineIndent(s *markdownScan) int { return s.lines[r.firstLine].indent }

// regionSpansBlanks reports whether blank lines inside this region belong to it,
// which disqualifies them as safe boundaries.
func (s *markdownScan) regionSpansBlanks(idx int32) bool {
	r := s.regions[idx]
	if r.kind == regionHTML {
		// Type-1 blocks (<script>/<pre>/<style>/<textarea>) run through blank
		// lines, so an interior blank is content, not a safe boundary.
		return s.lines[r.firstLine].htmlKind == 1
	}
	return r.kind == regionFence || r.kind == regionIndentedCode || r.kind == regionList
}

// interruptsParagraph reports whether the construct starting at line i can
// interrupt a paragraph, i.e. whether cutting immediately before it is safe
// even when the preceding line is paragraph text.
func (s *markdownScan) interruptsParagraph(i int) bool {
	li := s.lines[i]
	switch li.kind {
	case lineATXHeading, lineFenceOpen, lineQuote, lineThematicBreakOther:
		return true
	case lineHTMLStart:
		return li.htmlKind <= 6
	case lineListItemStart:
		// An ordered item can only interrupt a paragraph when it starts at 1.
		return !li.ordered || listMarkerNumber(s.lineText(i)) == 1
	case lineThematicBreakDash:
		// Ambiguous with a setext underline; only safe when no paragraph is open.
		return s.lines[i-1].kind == lineBlank
	default:
		return false
	}
}

// ── Repair payloads ─────────────────────────────────────────────────────────

// injectionAt reports the repair text a boundary at the given byte offset needs:
// prefix is prepended to the chunk that STARTS there, suffix is appended to the
// chunk that ENDS there.
//
// Keying this on the offset rather than on the candidate that produced it is
// what makes the repair correct under hard splits. An earlier version attached
// the injection to the candidate, so a chunk that opened an injected fence but
// was then terminated by a hard split emitted an unbalanced fence. Position is
// the truth: if a boundary sits inside a fence, the chunk before it must close
// and the chunk after it must reopen, no matter which rule chose the boundary.
func (s *markdownScan) injectionAt(offset int) (prefix, suffix string) {
	if offset <= 0 || offset >= len(s.content) {
		return "", ""
	}
	reg := s.lines[s.lineIndexOf(offset)].region
	if reg < 0 {
		return "", ""
	}
	r := &s.regions[reg]
	switch r.kind {
	case regionFence:
		// Everything from the second line of the region onward is fence
		// interior, including the original closing line: a chunk starting at
		// the closing line still needs an opener to be a valid code block.
		if body := r.firstLine + 1; body <= r.lastLine && offset >= s.lines[body].start {
			return s.span(r.fenceOpen) + "\n", s.span(r.fenceMarker)
		}
	case regionTable:
		// Only body rows need the header re-emitted. A boundary inside the
		// header or delimiter row itself cannot be repaired into a table and is
		// only reachable when the limit is smaller than the header.
		if body := r.firstLine + 2; body <= r.lastLine && offset >= s.lines[body].start {
			return s.span(r.header) + "\n" + s.span(r.delim) + "\n", ""
		}
	case regionHTML:
		// <pre>/<style>/<textarea>: reopen the (possibly multi-line) opening tag
		// on the chunk that starts here and close it on the chunk that ends here,
		// exactly like a fence. Only interior lines past the opening tag qualify.
		if r.htmlRepair {
			if body := r.htmlOpenEndLine + 1; body <= r.lastLine && offset >= s.lines[body].start {
				return s.span(r.htmlOpen) + "\n", r.htmlCloser
			}
		}
	}
	return "", ""
}

// injectionCostAt is injectionAt measured in runes, with the suffix budgeting one
// extra rune for the newline that separates it from a body ending mid-line.
func (s *markdownScan) injectionCostAt(offset int) (prefix, suffix int) {
	p, sfx := s.injectionAt(offset)
	prefix = utf8.RuneCountInString(p)
	if sfx != "" {
		suffix = utf8.RuneCountInString(sfx) + 1
	}
	return prefix, suffix
}

// span is only ever called on a span injectionAt has established, so it is
// always non-empty.
func (s *markdownScan) span(sp srcSpan) string {
	return s.content[sp.start:sp.end]
}

// trimTrailingWhitespaceEnd walks back from end over spaces, tabs and newlines.
func trimTrailingWhitespaceEnd(content string, end int) int {
	for end > 0 {
		r, size := utf8.DecodeLastRuneInString(content[:end])
		if r != ' ' && r != '\t' && r != '\n' {
			break
		}
		end -= size
	}
	return end
}

// runeIndexOf converts a byte offset to a rune index, using the per-line rune
// index as the base so only the tail of one line is counted.
func (s *markdownScan) runeIndexOf(offset int) int {
	i := s.lineIndexOf(offset)
	return s.lines[i].runeStart + utf8.RuneCountInString(s.content[s.lines[i].start:offset])
}

// lineIndexOf returns the index of the line containing the given byte offset.
func (s *markdownScan) lineIndexOf(offset int) int {
	lo, hi := 0, len(s.lines)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if s.lines[mid].start <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}
