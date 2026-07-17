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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Lightweight local retrieval for the Q&A scenario: --knowledge-dir loads
// .md/.txt files, chunks them, and prepends the best-matching chunks to each
// forwarded question. The agent CLI itself keeps running from the clean
// scratch dir — retrieval injects knowledge without the workdir-scan slowdown
// that would miss DingTalk's AI-assistant reply window (the connector's core
// latency constraint, see execForwarder.forward).
const (
	knowledgeTopK           = 3
	knowledgeMaxChunkRunes  = 1200
	knowledgeMaxPromptRunes = 3000
	knowledgeMaxFileBytes   = 1 << 20
)

type knowledgeChunk struct {
	source string
	text   string
	terms  map[string]int
}

type knowledgeBase struct {
	chunks []knowledgeChunk
}

// loadKnowledgeBase walks dir for .md/.txt files (hidden dirs skipped) and
// indexes them into scored-retrieval chunks.
// knowledgeTextExts is the set of plain-text extensions the retriever indexes.
var knowledgeTextExts = []string{".md", ".markdown", ".mdx", ".txt", ".text"}

var knowledgeReadFile = os.ReadFile

// isKnowledgeTextExt reports whether ext is an indexable plain-text extension.
func isKnowledgeTextExt(ext string) bool {
	ext = strings.ToLower(ext)
	for _, e := range knowledgeTextExts {
		if ext == e {
			return true
		}
	}
	return false
}

func loadKnowledgeBase(dir string) (*knowledgeBase, error) {
	kb := &knowledgeBase{}
	root := filepath.Clean(dir)
	indexed, skipped := 0, 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !isKnowledgeTextExt(filepath.Ext(path)) {
			skipped++
			return nil
		}
		if info, ierr := d.Info(); ierr == nil && info.Size() > knowledgeMaxFileBytes {
			fmt.Fprintf(os.Stderr, "[connect][knowledge] 跳过超大文件 %s\n", path)
			return nil
		}
		raw, rerr := knowledgeReadFile(path)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(root, path)
		for _, chunk := range splitKnowledgeChunks(string(raw)) {
			kb.chunks = append(kb.chunks, knowledgeChunk{source: rel, text: chunk, terms: knowledgeTerms(chunk)})
		}
		indexed++
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Tell the operator when files were skipped — a knowledge dir full of .pdf /
	// .docx / .json indexes nothing, which otherwise looks like "knowledge has no
	// effect" (issue #32). Only the .md/.txt text family is indexed.
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "[connect][knowledge] 已索引 %d 个文本文件，跳过 %d 个非文本文件（仅索引 %s）\n",
			indexed, skipped, strings.Join(knowledgeTextExts, "/"))
	}
	if len(kb.chunks) == 0 {
		return nil, fmt.Errorf("知识目录 %s 下没有可用的文本内容（仅索引 %s；pdf/docx/json 等不会被索引）", dir, strings.Join(knowledgeTextExts, "/"))
	}
	return kb, nil
}

// splitKnowledgeChunks cuts a document on markdown headings, merging lines up
// to knowledgeMaxChunkRunes so a chunk carries enough context to answer with.
func splitKnowledgeChunks(doc string) []string {
	var chunks []string
	var cur []string
	curLen := 0
	flush := func() {
		if s := strings.TrimSpace(strings.Join(cur, "\n")); s != "" {
			chunks = append(chunks, s)
		}
		cur = cur[:0]
		curLen = 0
	}
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") && curLen > 0 {
			flush()
		}
		cur = append(cur, line)
		curLen += len([]rune(line))
		if curLen >= knowledgeMaxChunkRunes {
			flush()
		}
	}
	flush()
	return chunks
}

// knowledgeTerms tokenizes for matching: lowercase latin/digit words (len>=2)
// plus CJK bigrams — CJK has no word boundaries and bigrams are the standard
// cheap approximation.
func knowledgeTerms(s string) map[string]int {
	terms := map[string]int{}
	var word []rune
	var prevCJK rune
	flushWord := func() {
		if len(word) >= 2 {
			terms[strings.ToLower(string(word))]++
		}
		word = word[:0]
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9':
			word = append(word, r)
			prevCJK = 0
		case unicode.Is(unicode.Han, r):
			flushWord()
			if prevCJK != 0 {
				terms[string([]rune{prevCJK, r})]++
			}
			prevCJK = r
		default:
			flushWord()
			prevCJK = 0
		}
	}
	flushWord()
	return terms
}

// augment returns the prompt to forward: the original question, or the
// question prefixed with the top-k matching knowledge chunks. No match means
// no prefix — the agent answers from its own knowledge as before.
func (kb *knowledgeBase) augment(question string) string {
	if kb == nil || len(kb.chunks) == 0 {
		return question
	}
	qTerms := knowledgeTerms(question)
	if len(qTerms) == 0 {
		return question
	}
	type scored struct {
		idx  int
		hits int // distinct query terms present
		occ  int // total occurrences, tie-break
	}
	var matches []scored
	for i, c := range kb.chunks {
		hits, occ := 0, 0
		for t := range qTerms {
			if n := c.terms[t]; n > 0 {
				hits++
				occ += n
			}
		}
		if hits > 0 {
			matches = append(matches, scored{i, hits, occ})
		}
	}
	if len(matches) == 0 {
		// Observability for "knowledge has no effect" (issue #32): the KB loaded
		// but nothing in it overlapped this question, so no context was injected.
		fmt.Fprintf(os.Stderr, "[connect][knowledge] 本条未命中知识库（%d 个片段中无相关内容）\n", len(kb.chunks))
		return question
	}
	sort.Slice(matches, func(a, b int) bool {
		if matches[a].hits != matches[b].hits {
			return matches[a].hits > matches[b].hits
		}
		return matches[a].occ > matches[b].occ
	})
	var b strings.Builder
	b.WriteString("以下是本地知识库中可能相关的资料，请优先依据它们回答；与问题无关时忽略：\n")
	used := 0
	for k := 0; k < len(matches) && k < knowledgeTopK; k++ {
		c := kb.chunks[matches[k].idx]
		text := truncateRunes(c.text, knowledgeMaxChunkRunes)
		used += len([]rune(text))
		if k > 0 && used > knowledgeMaxPromptRunes {
			break
		}
		fmt.Fprintf(&b, "\n【资料%d · %s】\n%s\n", k+1, c.source, text)
	}
	b.WriteString("\n用户问题：")
	b.WriteString(question)
	injected := len(matches)
	if injected > knowledgeTopK {
		injected = knowledgeTopK
	}
	fmt.Fprintf(os.Stderr, "[connect][knowledge] 本条命中并注入 %d 个知识片段\n", injected)
	return b.String()
}
