// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/whiteboard"
)

func TestCrossPlatformCoverageWhiteboardReceiptDeliveredInFullAndCompactSchema(t *testing.T) {
	want, err := contract.NormalizeResultSpec(whiteboard.Update.Contract.Result, "whiteboard.shortcut_update")
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		root := NewRootCommand()
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		args := []string{"schema", "--cli-path", "whiteboard +update", "--format", "json"}
		if compact {
			args = append(args, "--compact")
		}
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("schema compact=%v: %v; %s", compact, err, stderr.String())
		}
		var payload struct {
			Result *contract.ResultSpec `json:"result"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		got, err := contract.NormalizeResultSpec(payload.Result, "whiteboard.shortcut_update")
		if err != nil || got == nil {
			t.Fatalf("result missing or invalid, compact=%v: %v", compact, err)
		}
		var gotSchema, wantSchema any
		if err := json.Unmarshal(got.DataSchema, &gotSchema); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(want.DataSchema, &wantSchema); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotSchema, wantSchema) || !reflect.DeepEqual(got.Outcomes, want.Outcomes) || !reflect.DeepEqual(got.SensitivePaths, want.SensitivePaths) {
			t.Fatalf("compact=%v lost or changed receipt branch constraints", compact)
		}
	}
}

func TestCrossPlatformCoverageWhiteboardNativeRoutingAndCreateContractsDelivered(t *testing.T) {
	query := executeShortcutSchemaQuery(t, "--cli-path", "whiteboard query", "--compact")
	queryParams := schemaContractMap(query["parameters"])
	if required, _ := queryParams["part-id"]["required"].(bool); required {
		t.Fatalf("whiteboard query --part-id required=%v, want compatibility routing", required)
	}
	if got := schemaContractString(queryParams["part-id"]["required_when"]); got != "" {
		t.Fatalf("whiteboard query --part-id required_when=%q, want compatibility-safe omission", got)
	}
	if got := schemaContractStringSlice(queryParams["view"]["enum"]); !reflect.DeepEqual(got, []string{"summary", "page", "all"}) {
		t.Fatalf("whiteboard query --view enum=%v", got)
	}
	if got := schemaContractString(queryParams["page-id"]["required_when"]); got != "独立白板且 view=page 时" {
		t.Fatalf("whiteboard query --page-id required_when=%q", got)
	}

	update := executeShortcutSchemaQuery(t, "--cli-path", "whiteboard update")
	updateParams := schemaContractMap(update["parameters"])
	if got := schemaContractString(updateParams["part-id"]["required_when"]); got != "" {
		t.Fatalf("whiteboard update --part-id required_when=%q, want compatibility-safe omission", got)
	}
	for name, property := range map[string]string{
		"expected-revision": "expectedRevision",
		"request-id":        "requestId",
		"page-id":           "pageId",
	} {
		if got := schemaContractString(updateParams[name]["property"]); got != property {
			t.Errorf("whiteboard update --%s property=%q, want %q", name, got, property)
		}
	}

	create := executeShortcutSchemaQuery(t, "--cli-path", "whiteboard create-with-content", "--compact")
	if got := schemaContractString(create["canonical_path"]); got != "whiteboard.create_with_content" {
		t.Fatalf("create canonical_path=%q", got)
	}
	if create["result"] == nil {
		t.Fatal("whiteboard create-with-content compact Schema omitted reviewed Result")
	}
	createResult, _ := create["result"].(map[string]any)
	createDataSchema, _ := createResult["data_schema"].(map[string]any)
	if got := schemaContractStringSlice(createDataSchema["required"]); !reflect.DeepEqual(got, []string{"requestId", "nodeId", "revision"}) {
		t.Fatalf("whiteboard create result required=%v, want stable pre-release projection", got)
	}
	createParams := schemaContractMap(create["parameters"])
	for _, name := range []string{"name", "source", "request-id"} {
		if required, _ := createParams[name]["required"].(bool); !required {
			t.Errorf("whiteboard create-with-content --%s required=%v", name, required)
		}
	}
	fullCreate := executeShortcutSchemaQuery(t, "--cli-path", "whiteboard create-with-content")
	fullParams := schemaContractMap(fullCreate["parameters"])
	if got := fullParams["source"]["property"]; got != "source" {
		t.Fatalf("create source property=%v, want source", got)
	}
	if got := fullParams["source"]["type"]; got != "string" {
		t.Fatalf("create source file flag type=%v, want string", got)
	}
}
