// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	goldmarkast "github.com/yuin/goldmark/ast"
	extensionast "github.com/yuin/goldmark/extension/ast"
	goldmarktext "github.com/yuin/goldmark/text"
)

const tableAlignmentReadback = "tableAlignmentJSONML"

type markdownTableLayout struct {
	alignments []extensionast.Alignment
	rows       int
}

func markdownTableLayouts(source string) []markdownTableLayout {
	document := docMarkdown.Parser().Parse(goldmarktext.NewReader([]byte(source)))
	var tables []markdownTableLayout
	_ = goldmarkast.Walk(document, func(node goldmarkast.Node, entering bool) (goldmarkast.WalkStatus, error) {
		if table, ok := node.(*extensionast.Table); ok && entering {
			tables = append(tables, markdownTableLayout{table.Alignments, table.ChildCount()})
		}
		return goldmarkast.WalkContinue, nil
	})
	return tables
}

func hasAuthoredTableAlignment(source string) bool {
	for _, table := range markdownTableLayouts(source) {
		for _, alignment := range table.alignments {
			if alignment != extensionast.AlignNone {
				return true
			}
		}
	}
	return false
}

// Both views are read on each existing verification attempt. JSONML is the
// authority for alignment; the Markdown export remains the content evidence.
func readDocumentContentVerification(rt *shortcut.RuntimeContext, nodeID, expected, mode, format string) (map[string]any, error) {
	needsAlignment := format == "markdown" && hasAuthoredTableAlignment(expected)
	var combined map[string]any
	var jsonmlErr error
	params := map[string]any{"nodeId": nodeID, "format": format}
	value, err := readDocVerification(rt, "get_document_content", params, func(data map[string]any) bool {
		combined = cloneMap(data)
		if needsAlignment {
			var full map[string]any
			full, jsonmlErr = rt.CallMCPData(productDoc, "get_document_content", map[string]any{"nodeId": nodeID, "format": "jsonml"})
			if jsonmlErr != nil {
				return false
			}
			combined[tableAlignmentReadback] = full
		}
		return verifyUpdatedDocumentContent(combined, expected, mode, format)
	})
	if err != nil {
		return nil, err
	}
	if jsonmlErr != nil {
		return nil, jsonmlErr
	}
	if combined != nil {
		return combined, nil
	}
	return value, nil
}

func jsonmlTableElements(value any) ([][]any, bool) {
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
	if root == nil {
		return nil, false
	}
	var tables [][]any
	var walk func([]any)
	walk = func(element []any) {
		if len(element) < 2 {
			return
		}
		if element[0] == "table" {
			tables = append(tables, element)
		}
		for _, child := range element[2:] {
			if nested, ok := child.([]any); ok {
				walk(nested)
			}
		}
	}
	walk(root)
	return tables, true
}

func jsonmlChildren(element []any, tag string) [][]any {
	var children [][]any
	for _, child := range element[2:] {
		if node, ok := child.([]any); ok && len(node) >= 2 && node[0] == tag {
			children = append(children, node)
		}
	}
	return children
}

func tableAlignmentMatches(table []any, expected markdownTableLayout) bool {
	rows := jsonmlChildren(table, "tr")
	if len(rows) != expected.rows {
		return false
	}
	for _, row := range rows {
		cells := jsonmlChildren(row, "tc")
		if len(cells) != len(expected.alignments) {
			return false
		}
		for column, cell := range cells {
			attrs, ok := cell[1].(map[string]any)
			if !ok {
				return false
			}
			for _, dimension := range []string{"rowSpan", "colSpan"} {
				if v, present := attrs[dimension]; present && fmt.Sprint(v) != "1" {
					return false
				}
			}
			paragraphs := jsonmlChildren(cell, "p")
			if len(paragraphs) == 0 {
				return false
			}
			wanted := expected.alignments[column].String()
			if wanted == "none" {
				wanted = "left"
			}
			for _, paragraph := range paragraphs {
				attributes, ok := paragraph[1].(map[string]any)
				if !ok {
					return false
				}
				actual, _ := attributes["jc"].(string)
				if actual == "" {
					actual = "left"
				}
				if actual != wanted {
					return false
				}
			}
		}
	}
	return true
}

// Restore only alignment tokens proven by the full JSONML. Never copy
// authored alignment into the comparison before checking every target cell.
func markdownMatchesJSONMLTables(actual, expected, mode string, full any) bool {
	actualTables, ok := jsonmlTableElements(full)
	expectedTables := markdownTableLayouts(expected)
	readbackTables := markdownTableLayouts(actual)
	if !ok || len(actualTables) != len(readbackTables) || len(actualTables) < len(expectedTables) {
		return false
	}
	offset := len(actualTables) - len(expectedTables)
	if mode == "overwrite" && offset != 0 {
		return false
	}
	overrides := make([][]extensionast.Alignment, len(actualTables))
	for index, expectedTable := range expectedTables {
		if !tableAlignmentMatches(actualTables[offset+index], expectedTable) {
			return false
		}
		overrides[offset+index] = expectedTable.alignments
	}
	actualHTML, actualHTMLok := markdownRenderedWithTableAlignments(actual, overrides)
	expectedHTML, expectedHTMLok := markdownSemanticFingerprint(expected)
	if actualHTMLok && expectedHTMLok && ((mode == "overwrite" && actualHTML == expectedHTML) ||
		(mode == "append" && strings.HasSuffix(actualHTML, expectedHTML))) {
		return true
	}
	_, actualTokens, actualOK := markdownStructuralFingerprintWithAlignments(actual, overrides)
	_, expectedTokens, expectedOK := markdownStructuralFingerprint(expected)
	if !actualOK || !expectedOK || len(actualTokens) < len(expectedTokens) {
		return false
	}
	if mode == "overwrite" && len(actualTokens) != len(expectedTokens) {
		return false
	}
	return markdownMentionAwareTokensEqual(actualTokens[len(actualTokens)-len(expectedTokens):], expectedTokens)
}

func applyReadbackTableAlignments(document goldmarkast.Node, overrides [][]extensionast.Alignment) {
	index := 0
	_ = goldmarkast.Walk(document, func(node goldmarkast.Node, entering bool) (goldmarkast.WalkStatus, error) {
		table, ok := node.(*extensionast.Table)
		if !ok || !entering {
			return goldmarkast.WalkContinue, nil
		}
		if index < len(overrides) && overrides[index] != nil {
			table.Alignments = overrides[index]
			for row := table.FirstChild(); row != nil; row = row.NextSibling() {
				column := 0
				for child := row.FirstChild(); child != nil; child = child.NextSibling() {
					if cell, ok := child.(*extensionast.TableCell); ok && column < len(table.Alignments) {
						cell.Alignment = table.Alignments[column]
					}
					column++
				}
			}
		}
		index++
		return goldmarkast.WalkContinue, nil
	})
}

// An explicit false completeness marker is never a full readback.
func completeTableReadback(value any) bool {
	if data, ok := value.(map[string]any); ok {
		if data["complete"] == false || data["truncated"] == true {
			return false
		}
		for _, child := range data {
			if nested, ok := child.(map[string]any); ok && !completeTableReadback(nested) {
				return false
			}
		}
	}
	return true
}

func markdownRenderedWithTableAlignments(source string, alignments [][]extensionast.Alignment) (string, bool) {
	if len(source) > docMarkdownVerifyMax {
		return "", false
	}
	raw := []byte(source)
	document := docMarkdown.Parser().Parse(goldmarktext.NewReader(raw))
	applyReadbackTableAlignments(document, alignments)
	var rendered bytes.Buffer
	if err := docMarkdown.Renderer().Render(&rendered, raw, document); err != nil {
		return "", false
	}
	return rendered.String(), true
}
