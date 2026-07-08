//go:build goexperiment.jsonv2

package doc

import (
	"fmt"
	"strings"
)

// DiagnoseJSONError uses the Layer 0 Lexer (jsontext.Decoder) to provide
// detailed positional diagnostics when standard json.Unmarshal fails.
// Returns a human-readable multi-line report with line:col information.
// If parsing succeeds (no errors), returns empty string.
//
// This is designed to be called from the pipeline when json.Unmarshal
// reports a generic error like "unexpected end of JSON input" —
// we re-parse with our position-aware lexer to give the user actionable info.
func DiagnoseJSONError(src []byte) string {
	if len(src) == 0 {
		return ""
	}

	// Use Parse to get full diagnostics (Layer 0 + Layer 1)
	doc := Parse(src)
	if len(doc.Diagnostics) == 0 {
		return ""
	}

	// Filter to only error-level diagnostics
	var errs []Diagnostic
	for _, d := range doc.Diagnostics {
		if d.Severity == SeverityError {
			errs = append(errs, d)
		}
	}
	if len(errs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("发现 %d 处语法错误:\n", len(errs)))
	for i, d := range errs {
		if i >= 5 {
			sb.WriteString(fmt.Sprintf("  ... 还有 %d 处错误未显示\n", len(errs)-5))
			break
		}
		sb.WriteString(fmt.Sprintf("  %d. L%d:%d [%s] %s\n",
			i+1, d.Range.Start.Line, d.Range.Start.Col, d.Code, d.Message))
	}

	// Add context snippet around the first error
	if len(errs) > 0 {
		first := errs[0]
		snippet := extractSnippet(src, first.Range.Start.Line, 2)
		if snippet != "" {
			sb.WriteString("\n错误位置附近:\n")
			sb.WriteString(snippet)
		}
	}

	return sb.String()
}

// extractSnippet extracts lines around the given line number (1-based) for context.
// Shows `context` lines before and after the target line.
func extractSnippet(src []byte, targetLine int, context int) string {
	lines := strings.Split(string(src), "\n")
	if targetLine < 1 || targetLine > len(lines) {
		return ""
	}

	startLine := targetLine - context
	if startLine < 1 {
		startLine = 1
	}
	endLine := targetLine + context
	if endLine > len(lines) {
		endLine = len(lines)
	}

	var sb strings.Builder
	for i := startLine; i <= endLine; i++ {
		line := lines[i-1]
		// Truncate long lines
		if len(line) > 120 {
			line = line[:117] + "..."
		}
		marker := "  "
		if i == targetLine {
			marker = "> "
		}
		sb.WriteString(fmt.Sprintf("  %s%4d | %s\n", marker, i, line))
	}
	return sb.String()
}
