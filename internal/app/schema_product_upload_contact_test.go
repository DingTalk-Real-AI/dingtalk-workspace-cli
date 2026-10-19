// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCrossPlatformCoverageProductUploadAndContactDeliveredContracts(t *testing.T) {
	for _, tc := range []struct {
		canonical, path, flag, effect, resultField string
	}{
		{"oa.image_upload", "oa approval image upload", "file", "write", "mediaIdV2"},
		{"contact.get_open_dingtalk_id", "contact user get-open-dingtalk-id", "keyword", "read", "result"},
	} {
		t.Run(tc.canonical, func(t *testing.T) {
			root := NewRootCommand()
			command := exactCommandForTest(root, tc.path)
			if command == nil || !output.UsesUnifiedResult(command) {
				t.Fatalf("missing executable unified command %q", tc.path)
			}
			full := contactOrgListSchemaLeaf(t, tc.canonical, false)
			compact := contactOrgListSchemaLeaf(t, tc.canonical, true)
			if full["canonical_path"] != tc.canonical || full["primary_cli_path"] != tc.path || full["effect"] != tc.effect {
				t.Fatalf("identity/safety drift: %#v", full)
			}
			params := schemaContractMap(full["parameters"])
			if params[tc.flag]["required"] != true {
				t.Fatalf("required flag missing: %#v", params[tc.flag])
			}
			if problem := schemaHelpFlagCompletenessProblem(tc.canonical, tc.path, command, full); problem != "" {
				t.Fatal(problem)
			}
			result, ok := full["result"].(map[string]any)
			if !ok || !reflect.DeepEqual(result, compact["result"]) {
				t.Fatalf("full/compact Result mismatch: %#v / %#v", result, compact["result"])
			}
			dataSchema, _ := result["data_schema"].(map[string]any)
			branches, _ := dataSchema["oneOf"].([]any)
			if len(branches) != 2 {
				t.Fatalf("missing business/preview variants: %#v", dataSchema)
			}
			business, _ := branches[0].(map[string]any)
			properties, _ := business["properties"].(map[string]any)
			if dataSchema["type"] != "object" || properties[tc.resultField] == nil {
				t.Fatalf("missing reviewed business result: %#v", dataSchema)
			}
		})
	}
	alias := contactOrgListSchemaLeaf(t, "contact user get-open-id", false)
	if alias["canonical_path"] != "contact.get_open_dingtalk_id" || alias["is_alias"] != true {
		t.Fatalf("contact alias did not resolve to the primary identity: %#v", alias)
	}
}

func TestCrossPlatformCoverageChatNoticeOptionsDeliveredMapping(t *testing.T) {
	for _, action := range []string{"create", "edit"} {
		path := "chat group notice " + action
		tool := contactOrgListSchemaLeaf(t, path, false)
		params := schemaContractMap(tool["parameters"])
		for flag, property := range map[string]string{"no-card": "noCard", "must-read": "mustRead"} {
			if params[flag]["property"] != property || params[flag]["required"] != false {
				t.Fatalf("%s --%s mapping: %#v", path, flag, params[flag])
			}
		}
	}
}

func TestCrossPlatformCoverageProductResultSchemasAcceptActualPreview(t *testing.T) {
	imagePath := filepath.Join(t.TempDir(), "evidence.png")
	if err := os.WriteFile(imagePath, []byte("\x89PNG\r\n\x1a\nexample-image"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, canonical string
		args            []string
		receipt         map[string]any
		required        []string
	}{
		{
			name: "contact", canonical: "contact.get_open_dingtalk_id",
			args:     []string{"contact", "user", "get-open-dingtalk-id", "--keyword", "example-user"},
			receipt:  map[string]any{"success": true, "result": map[string]any{"userList": []any{}}},
			required: []string{"success"},
		},
		{
			name: "image", canonical: "oa.image_upload",
			args:     []string{"oa", "approval", "image", "upload", "--file", imagePath},
			receipt:  map[string]any{"mediaIdV2": "example-media", "bizType": "approval_image", "imageType": "png", "size": float64(21), "compressed": false},
			required: []string{"mediaIdV2", "bizType", "imageType", "size", "compressed"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(tc.args, "--dry-run", "--format", "json")
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
				t.Fatalf("preview failed: %v; stderr=%s", err, &stderr)
			}
			if len(runner.attempts) != 0 {
				t.Fatalf("preview reached remote runner: %#v", runner.attempts)
			}
			var envelope map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope["ok"] != true || envelope["dry_run"] != true {
				t.Fatalf("unexpected preview: %s", &stdout)
			}
			for _, compact := range []bool{false, true} {
				leaf := contactOrgListSchemaLeaf(t, tc.canonical, compact)
				result := leaf["result"].(map[string]any)
				compiler := jsonschema.NewCompiler()
				compiler.DefaultDraft(jsonschema.Draft2020)
				const resource = "https://dws.test/product-result.json"
				if err := compiler.AddResource(resource, result["data_schema"]); err != nil {
					t.Fatal(err)
				}
				schema, err := compiler.Compile(resource)
				if err != nil {
					t.Fatal(err)
				}
				for _, valid := range []any{envelope["data"], tc.receipt} {
					if err := schema.Validate(valid); err != nil {
						t.Fatalf("compact=%t valid result rejected: %v", compact, err)
					}
				}
				for _, field := range tc.required {
					invalid := maps.Clone(tc.receipt)
					delete(invalid, field)
					if err := schema.Validate(invalid); err == nil {
						t.Fatalf("compact=%t real result without %s accepted", compact, field)
					}
				}
				for field, value := range map[string]any{"dry_run": false, "executed": true} {
					invalid := maps.Clone(envelope["data"].(map[string]any))
					invalid[field] = value
					if err := schema.Validate(invalid); err == nil {
						t.Fatalf("compact=%t invalid preview %s accepted", compact, field)
					}
				}
			}
		})
	}
}
