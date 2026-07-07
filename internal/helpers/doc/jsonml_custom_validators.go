//go:build goexperiment.jsonv2

package doc

import "fmt"

// CustomValidator is a custom validation function invoked after declarative
// schema validation. It handles cross-field relationships, conditional
// requirements, and other logic that cannot be expressed declaratively.
type CustomValidator func(node *JsonMLNode, attrs map[string]any, schema *TagSchema, emit func(Diagnostic))

// customValidators is the per-tag custom validator registry.
var customValidators = map[string][]CustomValidator{}

// RegisterCustomValidator registers a custom validator for a specific tag.
// Validators are invoked in registration order after standard schema checks.
func RegisterCustomValidator(tag string, v CustomValidator) {
	customValidators[tag] = append(customValidators[tag], v)
}

// p/h1-h6: list.start is only meaningful for ordered lists
func validateListStart(node *JsonMLNode, attrs map[string]any, _ *TagSchema, emit func(Diagnostic)) {
	if attrs == nil {
		return
	}
	listObj, ok := attrs["list"].(map[string]any)
	if !ok {
		return
	}
	if listObj["start"] == nil {
		return
	}
	isOrdered, _ := listObj["isOrdered"].(bool)
	if !isOrdered {
		emit(Diagnostic{
			Range:    node.Range,
			Severity: SeverityWarning,
			Code:     CodeJSONMLSchemaListStartInvalid,
			Source:   SourceJSONMLSchema,
			Message:  `"start" is only meaningful when list.isOrdered is true`,
		})
	}
}

func init() {
	// table: sr != true 时 colsWidth 必填
	// Note: attrs can be typed-nil map when JSON has {}, so we check nil OR missing key
	RegisterCustomValidator("table", func(node *JsonMLNode, attrs map[string]any, _ *TagSchema, emit func(Diagnostic)) {
		sr, _ := attrs["sr"].(bool)
		if sr {
			return // server-rendered table, colsWidth not needed
		}
		if attrs == nil || attrs["colsWidth"] == nil {
			emit(Diagnostic{
				Range:      node.Range,
				Severity:   SeverityWarning,
				Code:       CodeJSONMLSchemaTableMissingColsWidth,
				Source:     SourceJSONMLSchema,
				Message:    `table without sr=true should have "colsWidth" attribute`,
				Suggestion: `"colsWidth": [100, 100, 100]`,
			})
		}
	})

	// table: colsWidth 长度应与 tr 子节点中 tc 数量一致
	RegisterCustomValidator("table", func(node *JsonMLNode, attrs map[string]any, _ *TagSchema, emit func(Diagnostic)) {
		if attrs == nil {
			return
		}
		colsWidth, ok := attrs["colsWidth"].([]any)
		if !ok || len(colsWidth) == 0 {
			return
		}
		// Find first tr child and count tc cells
		for _, child := range node.Children {
			if child.Tag == "tr" {
				cellCount := 0
				for _, cell := range child.Children {
					if cell.Tag == "tc" {
						cellCount++
					}
				}
				if cellCount > 0 && len(colsWidth) != cellCount {
					emit(Diagnostic{
						Range:    node.Range,
						Severity: SeverityWarning,
						Code:     CodeJSONMLSchemaTableColsWidthMismatch,
						Source:   SourceJSONMLSchema,
						Message:  fmt.Sprintf("colsWidth length (%d) doesn't match column count (%d)", len(colsWidth), cellCount),
					})
				}
				break // only check first row
			}
		}
	})

	// p/h1-h6: list validators
	for _, tag := range []string{"p", "h1", "h2", "h3", "h4", "h5", "h6"} {
		RegisterCustomValidator(tag, validateListStart)
	}

	// toc: content must be an array of objects (directory entries)
	RegisterCustomValidator("toc", validateTocContentShape)
}

// validateTocContentShape checks that toc.content is an array where each
// element is an object (a directory entry). Primitives or nested arrays
// are likely malformed.
func validateTocContentShape(node *JsonMLNode, attrs map[string]any, _ *TagSchema, emit func(Diagnostic)) {
	if attrs == nil {
		return
	}
	content, ok := attrs["content"].([]any)
	if !ok {
		return // type mismatch already reported by schema validator
	}
	for i, item := range content {
		if _, isObj := item.(map[string]any); !isObj {
			emit(Diagnostic{
				Range:      node.Range,
				Severity:   SeverityWarning,
				Code:       CodeJSONMLSchemaTocContentShape,
				Source:     SourceJSONMLSchema,
				Message:    fmt.Sprintf("toc.content[%d] should be an object (directory entry), got %T", i, item),
				Suggestion: `{"heading": "Title", "level": 1}`,
			})
			break // report only the first violation to reduce noise
		}
	}
}
