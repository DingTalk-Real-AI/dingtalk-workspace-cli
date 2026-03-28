// Package checker provides deep JSON equality comparison for MCP tools/call arguments.
// It normalizes numeric types so that JSON numbers represented as different Go types
// (json.Number, float64, int64) compare equal when they have the same value.
package checker

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Result holds the outcome of a single equivalence check.
type Result struct {
	Equal bool
	// Diff is a human-readable description of the differences, empty when Equal is true.
	Diff string
}

// CompareArguments performs a deep semantic equality comparison of two MCP argument maps.
// Key ordering is ignored; numeric types are normalized before comparison.
func CompareArguments(a, b map[string]any) Result {
	normalizedA := normalizeMap(a)
	normalizedB := normalizeMap(b)
	if reflect.DeepEqual(normalizedA, normalizedB) {
		return Result{Equal: true}
	}
	return Result{Equal: false, Diff: buildDiff(normalizedA, normalizedB)}
}

// normalizeMap recursively normalizes all values in a map.
func normalizeMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = normalizeValue(value)
	}
	return output
}

// normalizeValue converts json.Number and float64 to a canonical int64 or float64
// so that "42" and 42.0 compare equal.
func normalizeValue(value any) any {
	switch typed := value.(type) {
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return integer
		}
		if float, err := typed.Float64(); err == nil {
			return float
		}
		return typed.String()
	case float64:
		// If the float has no fractional part, treat it as int64.
		if typed == float64(int64(typed)) {
			return int64(typed)
		}
		return typed
	case map[string]any:
		return normalizeMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index, element := range typed {
			result[index] = normalizeValue(element)
		}
		return result
	default:
		return value
	}
}

// buildDiff produces a human-readable diff between two normalized argument maps.
// Lines are prefixed with:
//
//	"+" for keys present only in b
//	"-" for keys present only in a
//	"~" for keys present in both but with different values
func buildDiff(a, b map[string]any) string {
	allKeys := make(map[string]struct{}, len(a)+len(b))
	for key := range a {
		allKeys[key] = struct{}{}
	}
	for key := range b {
		allKeys[key] = struct{}{}
	}

	sortedKeys := make([]string, 0, len(allKeys))
	for key := range allKeys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	var lines []string
	for _, key := range sortedKeys {
		aValue, aExists := a[key]
		bValue, bExists := b[key]
		switch {
		case !aExists:
			lines = append(lines, fmt.Sprintf("  + %s: %s", key, marshalJSON(bValue)))
		case !bExists:
			lines = append(lines, fmt.Sprintf("  - %s: %s", key, marshalJSON(aValue)))
		case !reflect.DeepEqual(aValue, bValue):
			lines = append(lines, fmt.Sprintf("  ~ %s: %s != %s", key, marshalJSON(aValue), marshalJSON(bValue)))
		}
	}

	return strings.Join(lines, "\n")
}

func marshalJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}
