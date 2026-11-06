// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"bufio"
	"bytes"
	"encoding/json"
	stdhtml "html"
	"sort"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const docHighlightColor = "#FFC000"
const docHighlightOpen = `<span style="background-color: #FFC000;">`

var kindDocHighlight = ast.NewNodeKind("DocHighlight")

type docHighlight struct {
	ast.BaseInline
	open, close int
}

func (*docHighlight) Kind() ast.NodeKind              { return kindDocHighlight }
func (n *docHighlight) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// A processor belongs to one parse, so match positions are never shared by
// concurrent verifications. Goldmark keeps escaped punctuation, code and link
// destinations outside this delimiter parser.
type docHighlightParser struct{ open, close int }

func (*docHighlightParser) Trigger() []byte         { return []byte{'='} }
func (*docHighlightParser) IsDelimiter(b byte) bool { return b == '=' }
func (p *docHighlightParser) CanOpenCloser(open, close *parser.Delimiter) bool {
	if open.Char != '=' || close.Char != '=' {
		return false
	}
	p.open, p.close = open.Segment.Stop-2, close.Segment.Start
	return true
}
func (p *docHighlightParser) OnMatch(int) ast.Node {
	return &docHighlight{open: p.open, close: p.close}
}
func (*docHighlightParser) CloseBlock(ast.Node, parser.Context) {}
func (p *docHighlightParser) Parse(_ ast.Node, reader text.Reader, context parser.Context) ast.Node {
	line, segment := reader.PeekLine()
	before := reader.PrecendingCharacter()
	delimiter := parser.ScanDelimiter(line, before, 2, p)
	if delimiter == nil || delimiter.OriginalLength != 2 || before == '=' {
		return nil
	}
	delimiter.Segment = segment.WithStop(segment.Start + 2)
	reader.Advance(2)
	context.PushDelimiter(delimiter)
	return delimiter
}

type docHighlightRange struct {
	start, end int
	color      string
}
type docHighlightText struct {
	text         strings.Builder
	length       int
	ranges       []docHighlightRange
	wordBreaks   []int
	pendingBreak bool
}

func (s *docHighlightText) append(value, color string) {
	for _, char := range value {
		if unicode.IsSpace(char) {
			s.pendingBreak = true
			continue
		}
		if s.pendingBreak && s.length > 0 {
			s.wordBreaks = append(s.wordBreaks, s.length)
		}
		s.text.WriteRune(char)
		if color != "" {
			last := len(s.ranges) - 1
			if !s.pendingBreak && last >= 0 && s.ranges[last].end == s.length && s.ranges[last].color == color {
				s.ranges[last].end++
			} else {
				s.ranges = append(s.ranges, docHighlightRange{s.length, s.length + 1, color})
			}
		}
		s.length++
		s.pendingBreak = false
	}
}

// Only a projection for checking highlight positions. Existing Markdown
// structure/content verification still checks the entire document separately.
func authoredHighlights(source string) (string, docHighlightText) {
	processor := &docHighlightParser{}
	markdown := goldmark.New(goldmark.WithExtensions(extension.Table), goldmark.WithParserOptions(parser.WithInlineParsers(util.Prioritized(processor, 500))))
	raw := []byte(normalizeDocInputLineEndings(source))
	document := markdown.Parser().Parse(text.NewReader(raw))
	var spans []*docHighlight
	var visible docHighlightText
	depth := 0
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if span, ok := node.(*docHighlight); ok {
			if entering {
				spans = append(spans, span)
				depth++
			} else {
				depth--
			}
			return ast.WalkContinue, nil
		}
		if !entering {
			if node.Type() == ast.TypeBlock {
				visible.pendingBreak = true
			}
			return ast.WalkContinue, nil
		}
		color := ""
		if depth > 0 {
			color = docHighlightColor
		}
		switch n := node.(type) {
		case *ast.Text:
			visible.append(markdownHighlightText(n.Value(raw), n.IsRaw()), color)
			if n.SoftLineBreak() || n.HardLineBreak() {
				visible.pendingBreak = true
			}
		case *ast.String:
			visible.append(markdownHighlightText(n.Value, n.IsRaw() || n.IsCode()), color)
		case *ast.AutoLink:
			visible.append(string(n.Label(raw)), color)
		case *ast.CodeSpan:
			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				visible.append(string(child.Text(raw)), color)
			}
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock:
			visible.append(string(n.Lines().Value(raw)), color)
		case *ast.FencedCodeBlock:
			visible.append(string(n.Lines().Value(raw)), color)
		}
		return ast.WalkContinue, nil
	})
	if len(spans) == 0 {
		return source, visible
	}
	type edit struct {
		start int
		value string
	}
	edits := make([]edit, 0, 2*len(spans))
	for _, span := range spans {
		edits = append(edits, edit{span.open, docHighlightOpen}, edit{span.close, "</span>"})
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var canonical strings.Builder
	cursor := 0
	for _, e := range edits {
		canonical.Write(raw[cursor:e.start])
		canonical.WriteString(e.value)
		cursor = e.start + 2
	}
	canonical.Write(raw[cursor:])
	return canonical.String(), visible
}

// Use Goldmark's text writer first: Markdown entities require a semicolon,
// and an escaped ampersand must not turn into an entity. Decoding its escaped
// HTML output once gives the same visible text, including literal &copy.
func markdownHighlightText(value []byte, raw bool) string {
	var escaped bytes.Buffer
	writer := bufio.NewWriterSize(&escaped, 64)
	if raw {
		goldmarkhtml.DefaultWriter.RawWrite(writer, value)
	} else {
		goldmarkhtml.DefaultWriter.Write(writer, value)
	}
	_ = writer.Flush()
	return stdhtml.UnescapeString(escaped.String())
}

func hasAuthoredHighlight(source string) bool {
	if len(source) > docMarkdownVerifyMax || !strings.Contains(source, "==") {
		return false
	}
	_, visible := authoredHighlights(source)
	return len(visible.ranges) > 0
}

func jsonmlHighlightText(root []any) (docHighlightText, bool) {
	var visible docHighlightText
	var walk func([]any, string) bool
	walk = func(node []any, color string) bool {
		if len(node) < 2 {
			return false
		}
		if _, ok := node[0].(string); !ok {
			return false
		}
		attrs, ok := node[1].(map[string]any)
		if !ok {
			return false
		}
		if highlight, exists := attrs["highlight"]; exists {
			color, ok = highlight.(string)
			if !ok {
				return false
			}
			color = strings.ToUpper(color)
		}
		for _, child := range node[2:] {
			switch value := child.(type) {
			case string:
				visible.append(value, color)
			case []any:
				if !walk(value, color) {
					return false
				}
			default:
				return false
			}
		}
		switch node[0] {
		case "p", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tc", "codeblock":
			visible.pendingBreak = true
		}
		return true
	}
	ok := walk(root, "")
	return visible, ok
}

func highlightsMatchJSONML(expected docHighlightText, mode string, full any) bool {
	root, ok := documentJSONMLRoot(full)
	if !ok {
		return false
	}
	actual, ok := jsonmlHighlightText(root)
	if !ok || !strings.HasSuffix(actual.text.String(), expected.text.String()) {
		return false
	}
	offset := actual.length - expected.length
	if mode == "overwrite" && offset != 0 {
		return false
	}
	var breaks []int
	for _, at := range actual.wordBreaks {
		if at > offset {
			breaks = append(breaks, at-offset)
		}
	}
	if len(breaks) != len(expected.wordBreaks) {
		return false
	}
	for i := range breaks {
		if breaks[i] != expected.wordBreaks[i] {
			return false
		}
	}
	var ranges []docHighlightRange
	for _, span := range actual.ranges {
		if span.end <= offset {
			continue
		}
		if span.start < offset {
			return false
		}
		ranges = append(ranges, docHighlightRange{span.start - offset, span.end - offset, span.color})
	}
	if len(ranges) != len(expected.ranges) {
		return false
	}
	for i := range ranges {
		if ranges[i] != expected.ranges[i] {
			return false
		}
	}
	return true
}

func documentJSONMLRoot(value any) ([]any, bool) {
	if !completeTableReadback(value) {
		return nil, false
	}
	var root []any
	for _, encoded := range documentContentCandidates(value, "jsonml") {
		var element []any
		if json.Unmarshal([]byte(encoded), &element) == nil && len(element) >= 2 && element[0] == "root" {
			if root != nil {
				return nil, false
			}
			root = element
		}
	}
	return root, root != nil
}
