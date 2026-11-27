// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Assert final delivery, so help/Schema precedence cannot silently erase the color contract.
func TestCrossPlatformCoverageAitableColorContractDelivery(t *testing.T) {
	for _, compact := range []bool{false, true} {
		for _, tc := range []struct{ path, parameter, text string }{
			{"aitable view update timebar", "color-configs", "SINGLE_OPTION"},
			{"aitable app update", "theme-type", "normal-blue"},
			{"aitable app update", "appearance", "auto"},
			{"aitable app page create", "background", "#FFF1F0"},
			{"aitable app page update", "background", "整体替换"},
		} {
			args := []string{"--cli-path", tc.path}
			if compact {
				args = append(args, "--compact")
			}
			tool := executeShortcutSchemaQuery(t, args...)
			parameter := tool["parameters"].(map[string]any)[tc.parameter].(map[string]any)
			if !strings.Contains(parameter["description"].(string), tc.text) {
				t.Fatalf("color contract missing: %s / %#v", tc.path, parameter)
			}
			if tc.parameter == "appearance" {
				raw, _ := json.Marshal(parameter["enum"])
				if string(raw) != "null" && string(raw) != "[]" {
					t.Fatalf("appearance compatibility was narrowed: %s", raw)
				}
			}
		}
	}
	root := NewRootCommand()
	for _, path := range []string{"aitable view update fill-color-rule", "aitable +view-set-fill-color-rule"} {
		cmd, _, err := root.Find(strings.Fields(path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cmd.Long, "red_light") {
			t.Fatalf("semantic color help missing: %s", path)
		}
		if !strings.Contains(cmd.Long, "formula/lookup/filterUp") || !strings.Contains(cmd.Long, "FILL_COLOR_FIELD_SCHEMA_UNAVAILABLE") {
			t.Fatalf("calculated checkbox compatibility missing: %s", path)
		}
		for _, internal := range []string{"内部代号", "内部色板代号", "FORMAT_COLORS", "历史代号"} {
			if strings.Contains(cmd.Long, internal) {
				t.Fatalf("implementation detail in public help: %s", internal)
			}
		}
	}
}

func TestCrossPlatformCoverageAitableRedWorkflowMatchesIntent(t *testing.T) {
	for _, path := range []string{
		"../../skills/mono/references/products/aitable/aitable-view-extras.md",
		"../../skills/multi/dingtalk-aitable/references/aitable/aitable-view-extras.md",
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		_, workflow, ok := strings.Cut(string(body), `### 配置一个"金额超阈值红色高亮"`)
		if !ok {
			t.Fatalf("red workflow missing: %s", path)
		}
		workflow = strings.SplitN(workflow, "### ", 2)[0]
		if !strings.Contains(workflow, "red_light") || strings.Contains(workflow, "green_strong") {
			t.Fatalf("workflow color contradicts red intent: %s", path)
		}
	}
}
