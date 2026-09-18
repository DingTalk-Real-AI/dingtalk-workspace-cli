// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Execute the final assembled leaves, not a hand-built preview or a mapper.
// The injected runner rejects any attempt to cross the remote-call boundary.
func TestCrossPlatformCoverageAITableShareFormDryRunResultSchema(t *testing.T) {
	for _, path := range []string{
		"aitable form share get", "aitable +form-share-get",
		"aitable form share update", "aitable +form-share-update",
	} {
		t.Run(path, func(t *testing.T) {
			args := append(strings.Fields(path), "--base-id", "base-test", "--table-id", "table-test", "--view-id", "view-test")
			wantArgs := map[string]any{"baseId": "base-test", "tableId": "table-test", "viewId": "view-test"}
			tool := "get_share_form_config"
			if strings.Contains(path, "update") {
				tool = "update_share_form"
				wantArgs["enabled"] = false
				args = append(args, "--enabled", "false")
			}
			args = append(args, "--dry-run", "--format", "json")
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
				t.Fatalf("execute dry-run: %v; stderr=%s", err, stderr.String())
			}
			if len(runner.attempts) != 0 {
				t.Fatalf("dry-run reached remote runner: %#v", runner.attempts)
			}
			var envelope map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("decode real dry-run envelope: %v; output=%s", err, stdout.String())
			}
			if envelope["ok"] != true || envelope["outcome"] != "success" || envelope["dry_run"] != true || envelope["error"] != nil {
				t.Fatalf("unexpected dry-run envelope: %#v", envelope)
			}
			data := aitableShareSchemaObject(t, envelope["data"], "actual dry-run data")
			if data["tool"] != tool || data["executed"] != false || !reflect.DeepEqual(data["arguments"], wantArgs) {
				t.Fatalf("unexpected request preview: %#v", data)
			}
			for _, field := range []string{"cpSynced", "enabled", "status", "shareFormUuid", "formCover"} {
				if _, exists := data[field]; exists {
					t.Fatalf("dry-run must not claim server state %s: %#v", field, data)
				}
			}
			for _, compact := range []bool{false, true} {
				query := []string{"--cli-path", path}
				if compact {
					query = append(query, "--compact")
				}
				leaf := executeShortcutSchemaQuery(t, query...)
				result := aitableShareSchemaObject(t, leaf["result"], "published result")
				compiler := jsonschema.NewCompiler()
				compiler.DefaultDraft(jsonschema.Draft2020)
				const resource = "https://dws.test/form-share-result.json"
				if err := compiler.AddResource(resource, result["data_schema"]); err != nil {
					t.Fatal(err)
				}
				schema, err := compiler.Compile(resource)
				if err != nil {
					t.Fatalf("compile published schema: %v", err)
				}
				if err := schema.Validate(envelope["data"]); err != nil {
					t.Fatalf("compact=%v actual dry-run data violates published schema: %v; envelope=%s", compact, err, stdout.String())
				}
				// Exercise the published constraints, not just the number of branches.
				for _, field := range []string{"tool", "arguments", "executed"} {
					invalid := maps.Clone(data)
					delete(invalid, field)
					if err := schema.Validate(invalid); err == nil {
						t.Errorf("compact=%v accepted preview missing %s", compact, field)
					}
				}
				for _, mutation := range []struct {
					field string
					value any
				}{
					{"tool", 1}, {"arguments", nil}, {"arguments", "{}"},
					{"executed", true}, {"executed", "false"}, {"dry_run", false},
					{"cpSynced", true},
				} {
					invalid := maps.Clone(data)
					invalid[mutation.field] = mutation.value
					if err := schema.Validate(invalid); err == nil {
						t.Errorf("compact=%v accepted invalid preview %s=%#v", compact, mutation.field, mutation.value)
					}
				}
			}
		})
	}
}
