// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"encoding/json"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// Invalid rule containers fail as validation; the maximum valid batch keeps value types.
func TestCrossPlatformCoverageConditionalFormatsBatchAndJSONTypes(t *testing.T) {
	rules := make([]map[string]any, 40)
	for index := range rules {
		rules[index] = map[string]any{"type": "row", "format": map[string]any{"color": "red_light"},
			"filters": []any{map[string]any{"fieldId": "gap", "symbol": "EQ", "value": index%2 == 0}}}
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := ParseConditionalFormatsJSON(string(raw))
	if err != nil || len(actual) != 40 {
		t.Fatalf("maximum batch: rules=%d error=%v", len(actual), err)
	}
	filter := actual[0].(map[string]any)["filters"].([]any)[0].(map[string]any)
	if value, ok := filter["value"].(bool); !ok || !value {
		t.Fatalf("boolean changed: %#v", filter)
	}
	tooMany, err := json.Marshal(append(rules, rules[0]))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"{", "null", "{}", "[null]", "[1]", string(tooMany)} {
		if _, err := ParseConditionalFormatsJSON(raw); err == nil || apperrors.ExitCode(err) != 3 {
			t.Fatalf("invalid rule container accepted or misclassified: %s error=%v", raw, err)
		}
	}
	empty, err := ParseConditionalFormatsJSON("[]")
	if err != nil || len(empty) != 0 {
		t.Fatalf("clear rules rejected: %v", err)
	}
}
