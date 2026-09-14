// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"errors"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// aitableExplicitRetryable reads typed errors and legacy MCP JSON envelopes.
// Explicit false takes precedence over transient-error words in the message.
func aitableExplicitRetryable(err error) (bool, bool) {
	var typed *apperrors.Error
	if errors.As(err, &typed) && typed.RetryableSet {
		return typed.Retryable, true
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		text := current.Error()
		if start := strings.IndexByte(text, '{'); start >= 0 {
			var payload any
			if json.NewDecoder(strings.NewReader(text[start:])).Decode(&payload) == nil {
				if value, found := aitableRetryableFromEnvelope(payload, 0); found {
					return value, true
				}
			}
		}
	}
	return false, false
}

func aitableRetryableFromEnvelope(payload any, depth int) (bool, bool) {
	if depth > 8 {
		return false, false
	}
	foundTrue := false
	children := []any{}
	switch value := payload.(type) {
	case map[string]any:
		if retryable, ok := value["retryable"].(bool); ok {
			if !retryable {
				return false, true
			}
			foundTrue = true
		}
		// Only follow protocol envelopes, never parse arbitrary message prose.
		for _, key := range []string{"error", "result", "data", "content"} {
			children = append(children, value[key])
		}
		if value["type"] == "text" {
			if text, ok := value["text"].(string); ok {
				var nested any
				if json.Unmarshal([]byte(text), &nested) == nil {
					children = append(children, nested)
				}
			}
		}
	case []any:
		children = value
	}
	for _, child := range children {
		if retryable, found := aitableRetryableFromEnvelope(child, depth+1); found {
			if !retryable {
				return false, true
			}
			foundTrue = true
		}
	}
	return foundTrue, foundTrue
}
