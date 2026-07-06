package doc

import (
	"fmt"
	"strings"
)

// JsonMLValidationResult holds errors and warnings from validation.
type JsonMLValidationResult struct {
	Errors   []string
	Warnings []string
}

// HasErrors returns true if there are blocking errors.
func (r *JsonMLValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// Summary returns a human-readable report.
func (r *JsonMLValidationResult) Summary() string {
	if !r.HasErrors() && len(r.Warnings) == 0 {
		return ""
	}
	var sb strings.Builder
	if len(r.Errors) > 0 {
		sb.WriteString(fmt.Sprintf("JSONML 校验失败（%d 个错误）:\n", len(r.Errors)))
		for i, e := range r.Errors {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, e))
		}
	}
	if len(r.Warnings) > 0 {
		sb.WriteString(fmt.Sprintf("JSONML 校验警告（%d 个）:\n", len(r.Warnings)))
		for i, w := range r.Warnings {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, w))
		}
	}
	return sb.String()
}

func (r *JsonMLValidationResult) addError(path, issue, suggestion string) {
	r.Errors = append(r.Errors, formatDiag(path, issue, suggestion))
}

func (r *JsonMLValidationResult) addWarn(path, issue, suggestion string) {
	r.Warnings = append(r.Warnings, formatDiag(path, issue, suggestion))
}

func formatDiag(path, issue, suggestion string) string {
	issue = strings.TrimRight(issue, ".")
	if suggestion == "" {
		return fmt.Sprintf("%s: %s.", path, issue)
	}
	return fmt.Sprintf("%s: %s. Suggestion: %s", path, issue, suggestion)
}

// toFloat64 attempts to convert a value to float64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// matchesUnion checks if a value matches any of the given type names.
func matchesUnion(value any, types []string) bool {
	for _, t := range types {
		switch t {
		case "string":
			if _, ok := value.(string); ok {
				return true
			}
		case "number":
			if _, ok := toFloat64(value); ok {
				return true
			}
		case "boolean":
			if _, ok := value.(bool); ok {
				return true
			}
		case "array":
			if _, ok := value.([]any); ok {
				return true
			}
		case "object":
			if _, ok := value.(map[string]any); ok {
				return true
			}
		case "null":
			if value == nil {
				return true
			}
		case "any":
			return true
		}
	}
	return false
}
