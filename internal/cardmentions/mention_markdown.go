package cardmentions

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// mentionMarkdown retains byte offsets while hiding Markdown code examples.
func mentionMarkdown(source string) string {
	visible := []byte(source)
	document := goldmark.New().Parser().Parse(text.NewReader([]byte(source)))
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node.(type) {
		case *ast.CodeBlock, *ast.FencedCodeBlock:
			for i := 0; i < node.Lines().Len(); i++ {
				segment := node.Lines().At(i)
				for j := segment.Start; j < segment.Stop; j++ {
					visible[j] = ' '
				}
			}
		case *ast.CodeSpan:
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				if token, ok := child.(*ast.Text); ok {
					for j := token.Segment.Start; j < token.Segment.Stop; j++ {
						visible[j] = ' '
					}
				}
			}
		}
		return ast.WalkContinue, nil
	})
	for i, ch := range visible {
		if ch != '<' {
			continue
		}
		escapes := 0
		for j := i - 1; j >= 0 && visible[j] == '\\'; j-- {
			escapes++
		}
		if escapes%2 != 0 {
			visible[i] = ' '
		}
	}
	return string(visible)
}
