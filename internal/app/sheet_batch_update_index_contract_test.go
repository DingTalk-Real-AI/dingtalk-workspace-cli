// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageSheetBatchUpdateIndexGuidanceReachesHelpAndSchema(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Cleanup(CloseFileLogger)
	root := NewRootCommand()
	var help bytes.Buffer
	root.SetOut(&help)
	root.SetErr(&help)
	root.SetArgs([]string{"sheet", "batch-update", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	full := executeShortcutSchemaQuery(t, "--cli-path", "sheet batch-update")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "sheet batch-update", "--compact")
	fullDescription, _ := full["description"].(string)
	compactDescription, _ := compact["description"].(string)
	if fullDescription == "" || fullDescription != compactDescription {
		t.Fatalf("batch update full/compact descriptions differ: %q / %q", fullDescription, compactDescription)
	}
	for label, text := range map[string]string{"help": help.String(), "full schema": fullDescription, "compact schema": compactDescription} {
		for _, required := range []string{
			"index 只能单独更新", "name/title/hidden/", "frozen-row-count/frozen-column-count/tab-color",
			"先更新其他属性，再单独更新 index", "相同、稳定的 sheet-id", "最多 20 条",
			"独立 dws sheet update 仍可组合 index 与其他属性",
		} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s missing index guidance %q: %s", label, required, text)
			}
		}
	}
}

func TestCrossPlatformCoverageSheetBatchUpdateMixedIndexFinalCommandValidation(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Cleanup(CloseFileLogger)
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"sheet", "batch-update", "--node", "node", "--dry-run", "--operations",
		`[{"toolName":"update","input":{"sheet-id":"stable-sheet-id","hidden":false}},{"toolName":"update","input":{"sheet-id":"stable-sheet-id","index":0,"frozen-row-count":2}}]`,
	})
	err := root.Execute()
	requireFinalValidationError(t, "sheet batch-update", err)
	for _, required := range []string{"operations[1] 翻译失败: update:", "先更新其他属性，再单独更新 index", "相同、稳定的 sheet-id"} {
		if !strings.Contains(err.Error(), required) {
			t.Fatalf("final command error %q missing %q", err, required)
		}
	}
}
