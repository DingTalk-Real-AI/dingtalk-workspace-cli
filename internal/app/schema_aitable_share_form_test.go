// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import "testing"

func TestCrossPlatformCoverageAITableShareFormUpdateConstraints(t *testing.T) {
	want := map[string][][]string{"require_one_of": {{
		"enabled", "auth-type-code", "auth-data", "submit-times-limit", "submit-times-user-limit",
		"form-start-time", "form-end-time", "form-name", "form-desc", "anonymous-submit",
		"load-last-submit", "reply-notice", "share-uid-list",
	}}}
	for _, path := range []string{"aitable form share update", "aitable +form-share-update"} {
		for _, compact := range []bool{false, true} {
			args := []string{"--cli-path", path}
			if compact {
				args = append(args, "--compact")
			}
			tool := executeShortcutSchemaQuery(t, args...)
			if !schemaContractJSONEqual(tool["constraints"], want) {
				t.Fatalf("%s compact=%v constraints=%#v", path, compact, tool["constraints"])
			}
			enabled := tool["parameters"].(map[string]any)["enabled"].(map[string]any)
			if enabled["type"] != "string" || enabled["required"] != false {
				t.Fatalf("%s compact=%v enabled=%#v", path, compact, enabled)
			}
			if _, published := enabled["interface_type"]; published {
				t.Fatalf("%s compact=%v must preserve the historical omitted enabled interface_type", path, compact)
			}
		}
	}
}
