// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package app

import (
	"encoding/json"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAitableNavigationFullAndCompactResult(t *testing.T) {
	paths := []string{"aitable table create", "aitable table get", "aitable +table-get", "aitable +table-bootstrap", "aitable dashboard create", "aitable dashboard get", "aitable +dashboard-get", "aitable +dashboard-create", "aitable field create", "aitable field update", "aitable view create", "aitable view update name", "aitable form create", "aitable form update", "aitable workflow create", "aitable workflow update", "aitable app get", "aitable app update", "aitable app page create", "aitable app page update", "aitable +field-create", "aitable +field-update", "aitable +view-update", "aitable +view-preset-apply", "aitable +form-update", "aitable +workflow-deploy", "aitable +app-page-create", "aitable +form-create"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			full := executeShortcutSchemaQuery(t, "--cli-path", path)
			compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
			root := NewRootCommand()
			leaf, _, findErr := root.Find(strings.Fields(path))
			if findErr != nil {
				t.Fatal(findErr)
			}
			declaration, ok := contractfinal.RuntimeContractFinal(leaf)
			if !ok || declaration.Result == nil {
				t.Fatalf("missing declared Result: %s", path)
			}
			// The current framework deliberately hides Result for legacy output.
			// Keep that boundary; changing a URL must not change the output envelope.
			if !output.UsesUnifiedResult(leaf) {
				if full["result"] != nil || compact["result"] != nil {
					t.Fatalf("legacy Result leaked: %s", path)
				}
				declared, err := json.Marshal(declaration.Result)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(declared), "url") || !strings.Contains(string(declared), "links") {
					t.Fatalf("incomplete declared navigation: %s", declared)
				}
				return
			}
			if full["result"] == nil || !schemaContractJSONEqual(full["result"], compact["result"]) {
				t.Fatalf("missing or divergent Result: %s", path)
			}
			encoded, err := json.Marshal(compact["result"])
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{`"url"`, `"links"`} {
				if !strings.Contains(string(encoded), key) {
					t.Fatalf("%s missing %s: %s", path, key, encoded)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAitableWriteNavigationPublished(t *testing.T) {
	for _, path := range []string{
		"aitable create",
		"aitable base create", "aitable base update", "aitable base copy", "aitable +base-copy",
		"aitable +base-bootstrap", "aitable +table-update", "aitable app widget create", "aitable app widget update",
		"aitable +import-file", "aitable import data",
	} {
		t.Run(path, func(t *testing.T) {
			root := NewRootCommand()
			leaf, _, err := root.Find(strings.Fields(path))
			if err != nil || !output.UsesUnifiedResult(leaf) {
				t.Fatalf("%s has no unified output: %v", path, err)
			}
			full := executeShortcutSchemaQuery(t, "--cli-path", path)
			compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
			if full["result"] == nil || !schemaContractJSONEqual(full["result"], compact["result"]) {
				t.Fatal("Result missing or divergent", full["result"], compact["result"])
			}
			encoded, _ := json.Marshal(compact["result"])
			for _, key := range []string{`"url"`, `"links"`} {
				if !strings.Contains(string(encoded), key) {
					t.Fatalf("missing %s: %s", key, encoded)
				}
			}
			if path == "aitable +base-copy" && !strings.Contains(string(encoded), `"newBaseId"`) {
				t.Fatal("copied Base identity is missing from Result", string(encoded))
			}
			if path == "aitable +table-update" {
				parameter := full["parameters"].(map[string]any)["name"].(map[string]any)
				if parameter["property"] != "name" {
					t.Fatal("public name property changed", parameter)
				}
			}
			if !strings.Contains(leaf.Long, "完整保留 URL 及参数") {
				t.Fatal("navigation guidance missing from Help", leaf.Long)
			}
		})
	}
}
