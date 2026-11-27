// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// Execute the prepared shortcut: invalid JSON never calls MCP; semantic names and JSON values stay intact.
func TestCrossPlatformCoverageFillColorShortcutDispatchAndValidation(t *testing.T) {
	rules := []map[string]any{{"type": "row", "formatFieldId": "warning",
		"format":  map[string]any{"color": "red_light"},
		"filters": []any{map[string]any{"fieldId": "gap", "symbol": "GT", "value": 0}}}}
	raw, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{string(raw), "[null]", "{"} {
		caller := &datasourceCoverageCaller{}
		err := runDatasourceShortcutCLI(t, caller, "+view-set-fill-color-rule", "--base-id", "B",
			"--table-id", "T", "--view-id", "V", "--json", input, "--yes")
		if input != string(raw) {
			if err == nil || apperrors.ExitCode(err) != 3 || len(caller.toolLog) != 0 {
				t.Fatalf("invalid JSON reached MCP or lost validation: err=%v calls=%#v", err, caller.toolLog)
			}
			continue
		}
		if err != nil || len(caller.toolLog) != 1 || caller.toolLog[0] != "set_view_fill_color_rule" {
			t.Fatalf("fill dispatch: err=%v calls=%#v", err, caller.toolLog)
		}
		sent := caller.argLog[0]["conditionalFormats"].([]any)[0].(map[string]any)
		if sent["format"].(map[string]any)["color"] != "red_light" {
			t.Fatalf("semantic color changed: %#v", sent)
		}
		filter := sent["filters"].([]any)[0].(map[string]any)
		if filter["fieldId"] != "gap" || filter["symbol"] != "GT" || filter["value"] != float64(0) {
			t.Fatalf("condition changed: %#v", filter)
		}
	}
}
