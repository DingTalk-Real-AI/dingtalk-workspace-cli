package chatmsg

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// imageMediaID accepts stored image identifiers, never a URL or local path.
func imageMediaID(destination string) string {
	destination = strings.TrimPrefix(destination, "mediaid:")
	if (strings.HasPrefix(destination, "$") || strings.HasPrefix(destination, "@")) &&
		!strings.ContainsAny(destination, "/\\") {
		return destination
	}
	return ""
}

func markdownImageMediaIDs(source string) []string {
	if !strings.Contains(source, "![") {
		return nil
	}
	document := goldmark.New().Parser().Parse(text.NewReader([]byte(source)))
	var ids []string
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if image, ok := node.(*ast.Image); entering && ok {
			if id := imageMediaID(string(image.Destination)); id != "" {
				ids = append(ids, id)
			}
		}
		return ast.WalkContinue, nil
	})
	return ids
}
