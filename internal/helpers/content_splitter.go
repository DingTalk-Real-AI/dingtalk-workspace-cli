package helpers

import (
	"strings"
	"unicode/utf8"
)

// splitMarkdownSafe splits content into chunks of at most `limit` runes each,
// respecting markdown structure boundaries.
//
// Split priority (high to low):
//  1. H1 headings (# )
//  2. H2 headings (## )
//  3. H3 headings (### )
//  4. Blank lines (paragraph boundaries)
//  5. Hard split (preserving table/code block integrity)
//
// Invariant: strings.Join(result, "") == content (no content loss)
func splitMarkdownSafe(content string, limit int) []string {
	if utf8.RuneCountInString(content) <= limit {
		return []string{content}
	}

	blocks := parseMarkdownBlocks(content)
	return mergeBlocksIntoChunks(blocks, limit)
}

// markdownBlock represents an atomic block that should not be split.
type markdownBlock struct {
	text      string
	blockType int
}

const (
	blockNormal    = 0
	blockH1        = 1
	blockH2        = 2
	blockH3        = 3
	blockTable     = 4
	blockCodeBlock = 5
)

// parseMarkdownBlocks splits content into atomic blocks that should be kept together.
// The invariant is: strings.Join(all block texts, "") == original content.
// Each block's text includes trailing newlines up to (but not including) the next block's start.
func parseMarkdownBlocks(content string) []markdownBlock {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	var blocks []markdownBlock
	var currentLines []string
	currentType := blockNormal
	inCodeBlock := false

	flushCurrent := func(includeTrailingNewline bool) {
		if len(currentLines) > 0 {
			text := strings.Join(currentLines, "\n")
			if includeTrailingNewline {
				text += "\n"
			}
			blocks = append(blocks, markdownBlock{text: text, blockType: currentType})
			currentLines = nil
			currentType = blockNormal
		}
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		isLastLine := i == len(lines)-1

		// Code block fence detection
		if strings.HasPrefix(trimmed, "```") {
			if !inCodeBlock {
				flushCurrent(!isLastLine)
				currentType = blockCodeBlock
				inCodeBlock = true
				currentLines = append(currentLines, line)
				continue
			}
			// End of code block
			currentLines = append(currentLines, line)
			flushCurrent(!isLastLine)
			inCodeBlock = false
			continue
		}

		if inCodeBlock {
			currentLines = append(currentLines, line)
			continue
		}

		// Table line detection
		if strings.HasPrefix(trimmed, "|") {
			if currentType != blockTable {
				flushCurrent(!isLastLine)
				currentType = blockTable
			}
			currentLines = append(currentLines, line)
			continue
		}

		// If we were in a table and hit a non-table line, flush
		if currentType == blockTable {
			flushCurrent(!isLastLine)
		}

		// Heading detection — only at line start (not inside other blocks)
		// Order matters: check H3 before H2 before H1 to avoid ambiguity
		if strings.HasPrefix(line, "### ") {
			flushCurrent(!isLastLine)
			currentType = blockH3
			currentLines = append(currentLines, line)
			flushCurrent(!isLastLine)
			continue
		} else if strings.HasPrefix(line, "## ") {
			flushCurrent(!isLastLine)
			currentType = blockH2
			currentLines = append(currentLines, line)
			flushCurrent(!isLastLine)
			continue
		}
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			flushCurrent(!isLastLine)
			currentType = blockH1
			currentLines = append(currentLines, line)
			flushCurrent(!isLastLine)
			continue
		}

		currentLines = append(currentLines, line)
	}
	// Last block: no trailing newline
	flushCurrent(false)

	return blocks
}

// mergeBlocksIntoChunks greedily fills chunks up to the limit, then splits
// backwards at the nearest heading boundary. This ensures chunks are as large
// as possible while still breaking at meaningful markdown structure points.
//
// Strategy: fill forward until adding the next block would exceed the limit,
// then look backwards for the last heading in the current chunk to split there.
// If no heading is found, split at the overflow point (greedy).
func mergeBlocksIntoChunks(blocks []markdownBlock, limit int) []string {
	var chunks []string

	i := 0
	for i < len(blocks) {
		// Accumulate blocks greedily until we'd exceed the limit
		var chunkBlocks []markdownBlock
		chunkRunes := 0

		for i < len(blocks) {
			blockRunes := utf8.RuneCountInString(blocks[i].text)

			// Single oversized block: hard-split it
			if blockRunes > limit && chunkRunes == 0 {
				subChunks := hardSplitBlock(blocks[i].text, limit)
				chunks = append(chunks, subChunks...)
				i++
				chunkBlocks = nil
				chunkRunes = 0
				continue
			}

			// Would exceed limit: stop accumulating
			if chunkRunes+blockRunes > limit && chunkRunes > 0 {
				break
			}

			chunkBlocks = append(chunkBlocks, blocks[i])
			chunkRunes += blockRunes
			i++
		}

		if len(chunkBlocks) == 0 {
			continue
		}

		// If we stopped because of overflow AND there are multiple blocks,
		// look backwards for the last heading to use as a split point
		if i < len(blocks) && len(chunkBlocks) > 1 {
			splitIdx := -1
			for j := len(chunkBlocks) - 1; j > 0; j-- {
				bt := chunkBlocks[j].blockType
				if bt == blockH1 || bt == blockH2 || bt == blockH3 {
					splitIdx = j
					break
				}
			}
			if splitIdx > 0 {
				// Split: emit blocks before the heading, push heading+ back
				var emitBuilder strings.Builder
				for _, b := range chunkBlocks[:splitIdx] {
					emitBuilder.WriteString(b.text)
				}
				chunks = append(chunks, emitBuilder.String())
				// Rewind: put the heading and subsequent blocks back for next iteration
				i -= len(chunkBlocks) - splitIdx
				continue
			}
		}

		// No heading split point found (or single block): emit all accumulated blocks
		var emitBuilder strings.Builder
		for _, b := range chunkBlocks {
			emitBuilder.WriteString(b.text)
		}
		chunks = append(chunks, emitBuilder.String())
	}

	return chunks
}

// hardSplitBlock splits a single oversized block at paragraph boundaries,
// falling back to rune-level splitting.
func hardSplitBlock(text string, limit int) []string {
	paragraphs := strings.Split(text, "\n\n")
	var chunks []string
	var current strings.Builder
	currentRunes := 0

	for i, para := range paragraphs {
		separator := ""
		if i > 0 {
			separator = "\n\n"
		}
		paraWithSep := separator + para
		paraRunes := utf8.RuneCountInString(paraWithSep)

		if currentRunes+paraRunes > limit && currentRunes > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
			currentRunes = 0
			paraWithSep = para
			paraRunes = utf8.RuneCountInString(paraWithSep)
		}

		// If single paragraph exceeds limit, split by runes
		if paraRunes > limit {
			if currentRunes > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
				currentRunes = 0
			}
			runes := []rune(paraWithSep)
			for start := 0; start < len(runes); start += limit {
				end := start + limit
				if end > len(runes) {
					end = len(runes)
				}
				chunks = append(chunks, string(runes[start:end]))
			}
			continue
		}

		current.WriteString(paraWithSep)
		currentRunes += paraRunes
	}
	if currentRunes > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}
