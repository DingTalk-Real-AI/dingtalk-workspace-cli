// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package app

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCrossPlatformCoverageAITableFieldCreatePublishesPendingReadbackGuidance(t *testing.T) {
	full := executeShortcutSchemaQuery(t, "--cli-path", "aitable +field-create")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "aitable +field-create", "--compact")
	for name, leaf := range map[string]map[string]any{"full": full, "compact": compact} {
		description, _ := leaf["description"].(string)
		if !strings.Contains(description, "CREATE_FIELD_READBACK_PENDING") {
			t.Errorf("%s discovery omits the supported pending-readback receipt code: %q", name, description)
		}
		if description != full["description"] {
			t.Errorf("%s recovery guidance differs from the full leaf", name)
		}
		parameters := schemaContractMap(leaf["parameters"])
		if parameters["resume-field-ids"] == nil {
			t.Errorf("%s discovery omits the same-ID recovery parameter", name)
		}
	}
}

func TestCrossPlatformCoverageAITableWriteResultPublishesReadContract(t *testing.T) {
	leaf := executeShortcutSchemaQuery(t, "--cli-path", "aitable +record-write-result")
	result, _ := leaf["result"].(map[string]any)
	if result == nil {
		t.Fatal("reconciliation is missing its declared result contract")
	}
	dataSchema, _ := result["data_schema"].(map[string]any)
	branches, _ := dataSchema["oneOf"].([]any)
	if len(branches) != 2 {
		t.Fatalf("expected verified result and dry-run branches: %#v", dataSchema)
	}
	verified := branches[0].(map[string]any)
	properties := schemaContractMap(verified["properties"])
	for _, key := range []string{"baseId", "tableId", "clientToken", "state", "recordIds"} {
		if properties[key] == nil {
			t.Errorf("read reconciliation result lacks %s: %#v", key, result)
		}
	}
	if state := properties["state"]; state["const"] != "applied" {
		t.Fatalf("successful reconciliation must only publish applied state: %#v", state)
	}
	compact := executeShortcutSchemaQuery(t, "--cli-path", "aitable +record-write-result", "--compact")
	for name, projection := range map[string]map[string]any{"full": leaf, "compact": compact} {
		guidance := schemaContractString(projection["description"])
		for _, boundary := range []string{"停止后续写入", "下一步仍使用本命令和原 baseId/tableId/clientToken", "不能用全表查询替代"} {
			if !strings.Contains(guidance, boundary) {
				t.Errorf("%s discovery omits original-request reconciliation guidance %q", name, boundary)
			}
		}
		result := projection["result"].(map[string]any)
		branches := result["data_schema"].(map[string]any)["oneOf"].([]any)
		fields := schemaContractMap(branches[0].(map[string]any)["properties"])
		description := schemaContractString(fields["recordIds"]["description"])
		for _, boundary := range []string{"不保证输入顺序", "不保证整批完整", "未返回 ID 的记录仍未核实", "不能按数量差额或输入位置补写"} {
			if !strings.Contains(description, boundary) {
				t.Errorf("%s record IDs omit reconciliation boundary %q: %s", name, boundary, description)
			}
		}
	}
	parameters := schemaContractMap(leaf["parameters"])
	for _, key := range []string{"base-id", "table-id", "client-token"} {
		if parameters[key] == nil {
			t.Errorf("read reconciliation parameter lacks %s", key)
		}
	}
}

func TestCrossPlatformCoverageAITableWriteResultSchemaAcceptsActualDryRun(t *testing.T) {
	args := []string{"aitable", "+record-write-result", "--base-id", "b", "--table-id", "t", "--client-token", "123e4567-e89b-42d3-a456-426614174000", "--dry-run", "--format", "json"}
	testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
	helpers.InitDepsForTest(t, helpers.GetCaller())
	runner := &paramAliasDryRunRejectRunner{}
	testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(*GlobalFlags) executor.Runner { return runner })
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	if _, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args); err != nil {
		t.Fatal(err)
	}
	if err := root.Execute(); err != nil {
		t.Fatalf("dry-run failed: %v; stderr=%s", err, &stderr)
	}
	if len(runner.attempts) != 0 {
		t.Fatalf("dry-run reached remote runner: %#v", runner.attempts)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["ok"] != true || envelope["dry_run"] != true {
		t.Fatalf("unexpected preview: %s", &stdout)
	}
	verified := map[string]any{"baseId": "b", "tableId": "t", "clientToken": args[7], "state": "applied", "recordIds": []any{"r"}}
	for _, compact := range []bool{false, true} {
		query := []string{"--cli-path", "aitable +record-write-result"}
		if compact {
			query = append(query, "--compact")
		}
		leaf := executeShortcutSchemaQuery(t, query...)
		result := leaf["result"].(map[string]any)
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		const resource = "https://dws.test/record-write-result.json"
		if err := compiler.AddResource(resource, result["data_schema"]); err != nil {
			t.Fatal(err)
		}
		schema, err := compiler.Compile(resource)
		if err != nil {
			t.Fatal(err)
		}
		for _, valid := range []any{envelope["data"], verified} {
			if err := schema.Validate(valid); err != nil {
				t.Fatalf("compact=%t valid result rejected: %v", compact, err)
			}
		}
		for _, field := range []string{"baseId", "tableId", "clientToken", "state", "recordIds"} {
			invalid := maps.Clone(verified)
			delete(invalid, field)
			if err := schema.Validate(invalid); err == nil {
				t.Fatalf("compact=%t missing %s accepted", compact, field)
			}
		}
		for _, mutation := range []struct {
			field string
			value any
		}{
			{"state", "unknown"}, {"recordIds", []any{}}, {"recordIds", []any{"r", "r"}}, {"recordIds", []any{42}},
		} {
			invalid := maps.Clone(verified)
			invalid[mutation.field] = mutation.value
			if err := schema.Validate(invalid); err == nil {
				t.Fatalf("compact=%t invalid %s accepted", compact, mutation.field)
			}
		}
	}
}
