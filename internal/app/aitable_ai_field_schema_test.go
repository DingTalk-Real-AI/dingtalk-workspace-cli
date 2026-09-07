// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAIFieldRunFinalAndCompactSchema(t *testing.T) {
	tool := fullSchemaSnapshotForTest(t).Tools["aitable.shortcut_ai_field_run"]
	if tool == nil {
		t.Fatal("final Schema missing aitable.shortcut_ai_field_run")
	}
	for key, want := range map[string]string{
		"effect": "write", "risk": "high", "confirmation": "user_required", "idempotency": "non_idempotent",
	} {
		if got := schemaContractString(tool[key]); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	parameters := schemaContractMap(tool["parameters"])
	for _, name := range []string{"base-id", "table-id", "field-id", "record-ids"} {
		if required, _ := parameters[name]["required"].(bool); !required {
			t.Fatalf("--%s required = %#v", name, parameters[name]["required"])
		}
	}
	result, ok := tool["result"].(map[string]any)
	if !ok {
		t.Fatalf("result = %#v", tool["result"])
	}
	outcomes, _ := result["outcomes"].([]any)
	if !reflect.DeepEqual(outcomes, []any{"pending", "failure"}) {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	paths, _ := result["sensitive_paths"].([]any)
	if !reflect.DeepEqual(paths, []any{"documentUrl"}) {
		t.Fatalf("sensitive_paths = %#v", paths)
	}
	dryRun, _ := tool["dry_run"].(map[string]any)
	if dryRun["preview_kind"] != "plan" || dryRun["remote_reads"] != true {
		t.Fatalf("dry_run = %#v, want reviewed plan with remote reads", dryRun)
	}
	selectionText := strings.Join(append(schemaContractStrings(tool["use_when"]), schemaContractStrings(tool["avoid_when"])...), " ")
	for _, want := range []string{"钉钉 AI 字段", "不要用于安装", "没有 AI 字段任务状态查询接口"} {
		if !strings.Contains(selectionText, want) {
			t.Fatalf("selection missing %q: %s", want, selectionText)
		}
	}

	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"schema", "--cli-path", "aitable +ai-field-run", "--compact", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("compact Schema: %v; %s", err, stderr.String())
	}
	var compact map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &compact); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(compact["result"], result) {
		t.Fatalf("compact/full Result differs\ncompact=%#v\nfull=%#v", compact["result"], result)
	}
}

func schemaContractStrings(value any) []string {
	switch items := value.(type) {
	case []string:
		return append([]string(nil), items...)
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}
