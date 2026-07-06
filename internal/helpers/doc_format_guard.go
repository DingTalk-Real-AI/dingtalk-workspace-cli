package helpers

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// validateDocFormat enforces a case-sensitive whitelist on --content-format.
//
// allowed must include the empty string "" if the flag is optional;
// the first non-empty entry is rendered as "<value> (默认)" in error text.
// commandLabel is the human-readable command path (e.g. "doc create").
// example is a copy-paste-ready invocation shown to the user.
//
// raw=="json" is special-cased: agents pass it because the global persistent
// `-f/--format` (CLI output format, default json) shadows the per-command
// --content-format flag in their mental model. The error names the collision
// instead of just rejecting, so the agent doesn't loop on the same value.
func validateDocFormat(cmd *cobra.Command, allowed []string, commandLabel, example string) error {
	raw, _ := cmd.Flags().GetString("content-format")
	for _, a := range allowed {
		if raw == a {
			return nil
		}
	}
	var msg string
	if raw == "json" {
		msg = fmt.Sprintf(
			"--content-format json 在 %s 上无效。这里的 --content-format 指**文档内容**格式，不是 CLI 输出格式；CLI 输出格式由顶层 -f/--format 控制（默认即为 json，无需重复传）。本命令接受: %s",
			commandLabel, renderAllowedFormats(allowed))
	} else {
		msg = fmt.Sprintf("--content-format 取值 %q 无效，%s 仅接受: %s", raw, commandLabel, renderAllowedFormats(allowed))
	}
	return &CLIError{
		Code:       CodeInvalidParam,
		Message:    msg,
		Suggestion: "示例: " + example,
		Operation:  commandLabel,
	}
}

// renderAllowedFormats turns ["", "markdown", "jsonml"] into "markdown (默认), jsonml".
// When `allowed` contains the empty string "" anywhere, the first non-empty entry
// is rendered with the "(默认)" suffix. Other non-empty entries are rendered
// verbatim. The "" entry itself is never rendered.
func renderAllowedFormats(allowed []string) string {
	hasEmpty := false
	for _, a := range allowed {
		if a == "" {
			hasEmpty = true
			break
		}
	}
	var parts []string
	defaultMarked := false
	for _, a := range allowed {
		if a == "" {
			continue
		}
		if hasEmpty && !defaultMarked {
			parts = append(parts, a+" (默认)")
			defaultMarked = true
			continue
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, ", ")
}

// sniffJsonMLLike is a cheap (no JSON parse) heuristic for "looks like JSONML".
//
// Triggers on:
//   - leading "[" followed (within 64 bytes, after whitespace) by a JSON string literal
//   - leading "{" followed (within 64 bytes) by "\"jsonml\"" key
//
// Does NOT trigger on markdown link "[text](url)" — the first non-whitespace
// after "[" must be a double-quote, not a letter.
func sniffJsonMLLike(content string) bool {
	const lookahead = 64
	s := strings.TrimLeft(content, " \t\r\n")
	if s == "" {
		return false
	}
	scan := s
	if len(scan) > lookahead {
		scan = scan[:lookahead]
	}
	switch s[0] {
	case '[':
		rest := strings.TrimLeft(scan[1:], " \t\r\n")
		// ["tag"...] = single node; [["tag"...], ...] = body-level node list
		return strings.HasPrefix(rest, `"`) || strings.HasPrefix(rest, `[`)
	case '{':
		rest := strings.TrimLeft(scan[1:], " \t\r\n")
		return strings.HasPrefix(rest, `"jsonml"`)
	}
	return false
}
