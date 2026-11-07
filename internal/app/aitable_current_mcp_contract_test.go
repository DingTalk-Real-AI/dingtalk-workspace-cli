// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAitableAccelerationScopeOmitsDeferredWrites(t *testing.T) {
	root := NewRootCommand()
	hasLeaf := func(path string) bool {
		current := root
		for _, name := range strings.Fields(path) {
			found := false
			for _, child := range current.Commands() {
				if child.Name() == name {
					current, found = child, true
					break
				}
			}
			if !found {
				return false
			}
		}
		return current.Runnable()
	}
	for _, path := range []string{"aitable chart layout-update", "aitable +chart-layout-update"} {
		if hasLeaf(path) {
			t.Fatalf("deferred write remains executable: %s", path)
		}
	}
	for _, path := range []string{"aitable chart batch-create", "aitable +charts-create", "aitable chart templates", "aitable +dashboard-preview", "aitable +dashboard-diagnose", "aitable +field-wait-ai", "aitable +task-report"} {
		if !hasLeaf(path) {
			t.Fatalf("acceleration command was removed: %s", path)
		}
	}
}

func TestCrossPlatformCoverageAitableCurrentMCPQueryEvidenceReachesCompactSchema(t *testing.T) {
	for _, path := range []string{"aitable field task-status", "aitable +field-wait-ai"} {
		full := executeShortcutSchemaQuery(t, "--cli-path", path)
		compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
		if !schemaContractJSONEqual(full["result"], compact["result"]) {
			t.Fatalf("%s full/compact result differ", path)
		}
		result := compact["result"].(map[string]any)
		schema := result["data_schema"].(map[string]any)
		if branches, ok := schema["oneOf"].([]any); ok {
			schema = branches[0].(map[string]any)
		}
		properties := schema["properties"].(map[string]any)
		for _, name := range []string{"queryComplete", "queryRetryable", "queryBudgetExhausted"} {
			property, ok := properties[name].(map[string]any)
			if !ok || property["type"] != "boolean" || property["description"] == "" {
				t.Fatalf("%s missing published query evidence %s: %#v", path, name, properties)
			}
		}
	}
}

func TestCrossPlatformCoverageAitableGoalClosureCommandsReachCompactSchema(t *testing.T) {
	for path, flags := range map[string][]string{
		"aitable +task-report":        {"checkpoint", "baseline", "model-metrics", "baseline-model-metrics"},
		"aitable +field-wait-ai":      {"checkpoint", "resume"},
		"aitable +dashboard-diagnose": {"expected-layout", "regions"},
	} {
		full := executeShortcutSchemaQuery(t, "--cli-path", path)
		compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
		if !schemaContractJSONEqual(full["result"], compact["result"]) {
			t.Fatalf("%s result differs between projections", path)
		}
		parameters := compact["parameters"].(map[string]any)
		for _, flag := range flags {
			if parameters[flag] == nil {
				t.Fatalf("%s missing %s", path, flag)
			}
		}
	}
}
