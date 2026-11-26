// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package app

import "testing"

func TestCrossPlatformCoverageAitableFormConditionOptionalUpdates(t *testing.T) {
	for _, path := range []string{"aitable form field update", "aitable +form-field-update"} {
		for _, compact := range []bool{false, true} {
			args := []string{"--cli-path", path}
			if compact {
				args = append(args, "--compact")
			}
			schema := executeShortcutSchemaQuery(t, args...)
			params := schema["parameters"].(map[string]any)
			for _, name := range []string{"required", "field-description", "display-condition", "clear-display-condition"} {
				param := params[name].(map[string]any)
				if param["required"] != false {
					t.Fatalf("%s compact=%v: optional update %s marked required: %#v", path, compact, name, param)
				}
			}
		}
	}
}
