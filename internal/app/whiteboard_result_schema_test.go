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

func TestCrossPlatformCoverageWhiteboardTemplateLanguageSchema(t *testing.T) {
	for _, scope := range []string{"personal", "team"} {
		for _, compact := range []bool{false, true} {
			args := []string{"--cli-path", "whiteboard template " + scope + " save"}
			if compact {
				args = append(args, "--compact")
			}
			payload := executeShortcutSchemaQuery(t, args...)
			params := schemaContractMap(payload["parameters"])
			workspace, present := params["template-workspace"]
			if present != (scope == "team") {
				t.Fatalf("%s compact=%v unexpected destination parameter: %#v", scope, compact, workspace)
			}
			if present && workspace["required"] == true {
				t.Fatal("team save destination must remain optional")
			}
			if got := params["language"]["default"]; got != "zh_CN" {
				t.Fatalf("%s compact=%v language=%#v", scope, compact, params["language"])
			}
			if params["language"]["required"] == true {
				t.Fatal("language must remain optional")
			}
		}
	}
}

func TestCrossPlatformCoverageWhiteboardDiffContractDeliveredInFullAndCompactSchema(t *testing.T) {
	want, err := contract.NormalizeResultSpec(whiteboard.Diff.Contract.Result, "whiteboard.shortcut_diff")
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		args := []string{"--cli-path", "whiteboard +diff"}
		if compact {
			args = append(args, "--compact")
		}
		payload := executeShortcutSchemaQuery(t, args...)
		if got := schemaContractString(payload["canonical_path"]); got != "whiteboard.shortcut_diff" {
			t.Fatalf("compact=%v canonical_path=%q", compact, got)
		}
		params := schemaContractMap(payload["parameters"])
		if got := schemaContractString(params["page-id"]["required_when"]); got != "操作独立白板时" {
			t.Fatalf("compact=%v page-id required_when=%q", compact, got)
		}
		if got := params["detail-limit"]["default"]; got != float64(100) && got != "100" {
			t.Fatalf("compact=%v detail-limit default=%#v", compact, got)
		}
		encoded, err := json.Marshal(payload["result"])
		if err != nil {
			t.Fatal(err)
		}
		var gotSpec contract.ResultSpec
		if err := json.Unmarshal(encoded, &gotSpec); err != nil {
			t.Fatal(err)
		}
		got, err := contract.NormalizeResultSpec(&gotSpec, "whiteboard.shortcut_diff")
		if err != nil {
			t.Fatal(err)
		}
		var gotSchema, wantSchema any
		if err := json.Unmarshal(got.DataSchema, &gotSchema); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(want.DataSchema, &wantSchema); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotSchema, wantSchema) || !reflect.DeepEqual(got.Outcomes, want.Outcomes) || !reflect.DeepEqual(got.SensitivePaths, want.SensitivePaths) {
			t.Fatalf("compact=%v lost diff Result contract", compact)
		}
	}
	update := executeShortcutSchemaQuery(t, "--cli-path", "whiteboard +update")
	if got := schemaContractString(schemaContractMap(update["parameters"])["expected-source-digest"]["property"]); got != "expectedSourceDigest" {
		t.Fatalf("+update expected-source-digest property=%q", got)
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
	if got := schemaContractStringSlice(createDataSchema["required"]); !reflect.DeepEqual(got, []string{"requestId", "nodeId", "revision", "sourceDigest"}) {
		t.Fatalf("whiteboard create result required=%v, want stable pre-release projection", got)
	}
	createParams := schemaContractMap(create["parameters"])
	for _, name := range []string{"name", "source", "request-id"} {
		if required, _ := createParams[name]["required"].(bool); !required {
			t.Errorf("whiteboard create-with-content --%s required=%v", name, required)
		}
	}
	fullCreate := executeShortcutSchemaQuery(t, "--cli-path", "whiteboard create-with-content")
	for _, projection := range []map[string]any{create, fullCreate} {
		if got := schemaContractString(projection["confirmation"]); got != "user_required" {
			t.Fatalf("create confirmation=%q, want user_required", got)
		}
	}
	fullParams := schemaContractMap(fullCreate["parameters"])
	if got := fullParams["source"]["property"]; got != "source" {
		t.Fatalf("create source property=%v, want source", got)
	}
	if got := fullParams["source"]["type"]; got != "string" {
		t.Fatalf("create source file flag type=%v, want string", got)
	}
	if got := schemaContractString(fullParams["expected-source-digest"]["property"]); got != "expectedSourceDigest" {
		t.Fatalf("create expected-source-digest property=%q", got)
	}

	render := executeShortcutSchemaQuery(t, "--cli-path", "whiteboard render", "--compact")
	if got := schemaContractString(render["canonical_path"]); got != "whiteboard.render" {
		t.Fatalf("render canonical_path=%q", got)
	}
	if got := schemaContractString(render["effect"]); got != "write" {
		t.Fatalf("render effect=%q", got)
	}
	if got := schemaContractString(render["confirmation"]); got != "user_required" {
		t.Fatalf("render confirmation=%q", got)
	}
	if got := schemaContractString(render["risk"]); got != "medium" {
		t.Fatalf("render risk=%q", got)
	}
	renderResult, _ := render["result"].(map[string]any)
	renderDataSchema, _ := renderResult["data_schema"].(map[string]any)
	properties, _ := renderDataSchema["properties"].(map[string]any)
	nextAction, _ := properties["nextAction"].(map[string]any)
	if nextAction["const"] != "await_user_confirmation" {
		t.Fatalf("render must deliver wait-for-user action: %#v", nextAction)
	}
	if got := schemaContractStringSlice(renderDataSchema["required"]); !containsSchemaContractString(got, "artifactPath") || !containsSchemaContractString(got, "sourceDigest") || !containsSchemaContractString(got, "warnings") {
		t.Fatalf("render required=%v", got)
	}
}

func containsSchemaContractString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestCrossPlatformCoverageWhiteboardExportDryRunContractsDelivered(t *testing.T) {
	for _, path := range []string{"whiteboard export", "whiteboard export-get"} {
		full := executeShortcutSchemaQuery(t, "--cli-path", path)
		compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
		if full["dry_run"] == nil {
			t.Fatalf("%s missing dry-run declaration", path)
		}
		// Export still emits legacy bytes; the assembly intentionally withholds
		// its internal Result declaration until unified output is enabled.
		if full["result"] != nil || compact["result"] != nil {
			t.Fatalf("%s publishes unified Result before runtime rollout", path)
		}
		dryRun, ok := full["dry_run"].(map[string]any)
		if !ok || dryRun["preview_kind"] != "request" || dryRun["remote_reads"] == true {
			t.Fatalf("%s invalid dry-run contract: %#v", path, full["dry_run"])
		}
	}
}

func TestCrossPlatformCoverageWhiteboardMergedCapabilities(t *testing.T) {
	for _, path := range []string{
		"whiteboard render", "whiteboard +diff", "whiteboard +query", "whiteboard +update",
		"whiteboard create-with-content", "whiteboard export", "whiteboard export-get",
		"whiteboard template personal save", "whiteboard template personal list", "whiteboard template personal create",
		"whiteboard template team save", "whiteboard template team list", "whiteboard template team create",
	} {
		t.Run(path, func(t *testing.T) {
			payload := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
			if payload["cli_path"] != path {
				t.Fatalf("capability absent from Schema: %#v", payload)
			}
		})
	}
}
