package helpers

import (
	"encoding/json"
	"fmt"
	"strings"
)

func printSanitizedMCPText(text string, listKeys ...string) error {
	var payload any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return fmt.Errorf("failed to parse MCP response: %w", err)
	}
	keySet := make(map[string]bool, len(listKeys))
	for _, key := range listKeys {
		keySet[key] = true
	}
	sanitizeEmptyObjectLists(payload, keySet)
	return printFilteredPayload(payload)
}

func sanitizeEmptyObjectLists(value any, listKeys map[string]bool) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if listKeys[key] {
				if arr, ok := child.([]any); ok {
					filtered := arr[:0]
					for _, item := range arr {
						if !isJSONZeroish(item) {
							filtered = append(filtered, item)
						}
					}
					v[key] = filtered
					continue
				}
			}
			sanitizeEmptyObjectLists(child, listKeys)
		}
	case []any:
		for _, child := range v {
			sanitizeEmptyObjectLists(child, listKeys)
		}
	}
}

func isJSONZeroish(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case bool:
		return !v
	case float64:
		return v == 0
	case map[string]any:
		if len(v) == 0 {
			return true
		}
		for _, child := range v {
			if !isJSONZeroish(child) {
				return false
			}
		}
		return true
	case []any:
		if len(v) == 0 {
			return true
		}
		for _, child := range v {
			if !isJSONZeroish(child) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
