// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedSchemaCatalogContainsWukongDriveSheetParity(t *testing.T) {
	loaded := embeddedSchemaCatalog()
	if !embeddedSchemaCatalogAvailable() {
		t.Fatalf("embedded schema catalog is unavailable: %v", embeddedSchemaCatalogError())
	}

	cases := []struct {
		canonical           string
		cliPath             string
		risk                string
		confirmation        string
		parameters          []string
		exactParameters     bool
		parameterProperties map[string]string
		forbiddenParameters []string
	}{
		{canonical: "drive.permission_apply", cliPath: "drive permission apply", risk: "medium", confirmation: "user_required", parameters: []string{"node", "role", "users"}},
		{canonical: "drive.permission_apply_info", cliPath: "drive permission apply-info", risk: "low", confirmation: "not_required", parameters: []string{"node"}},
		{canonical: "drive.permission_transfer_owner", cliPath: "drive permission transfer-owner", risk: "high", confirmation: "user_required", parameters: []string{"new-owner", "node", "recursive", "reserve-role", "workspace"}},
		{canonical: "drive.star_add", cliPath: "drive star add", risk: "low", confirmation: "not_required", parameters: []string{"node"}},
		{canonical: "drive.star_remove", cliPath: "drive star remove", risk: "low", confirmation: "not_required", parameters: []string{"node"}},
		{canonical: "drive.star_list", cliPath: "drive star list", risk: "low", confirmation: "not_required", parameters: []string{"content-types", "cursor", "limit", "order-by", "resource-types", "sort"}},
		{canonical: "drive.task_get", cliPath: "drive task get", risk: "low", confirmation: "not_required", parameters: []string{"type", "id"}},
		{canonical: "sheet.formula_verify", cliPath: "sheet formula-verify", risk: "low", confirmation: "not_required", parameters: []string{"exit-on-error", "max-cells", "max-locations-per-error", "node", "range", "sheet-id", "targets"}},
		{canonical: "sheet.version_save", cliPath: "sheet version save", risk: "medium", confirmation: "not_required", parameters: []string{"node"}},
		{canonical: "sheet.version_list", cliPath: "sheet version list", risk: "low", confirmation: "not_required", parameters: []string{"cursor", "limit", "node"}},
		{canonical: "sheet.version_revert", cliPath: "sheet version revert", risk: "medium", confirmation: "user_required", parameters: []string{"node", "version"}},
		{
			canonical:       "doc.export",
			cliPath:         "doc export create",
			risk:            "low",
			confirmation:    "not_required",
			parameters:      []string{"async", "export-format", "node", "output"},
			exactParameters: true,
		},
		{
			canonical:           "doc.query_export_job",
			cliPath:             "doc export get",
			risk:                "low",
			confirmation:        "not_required",
			parameters:          []string{"job-id", "task-id"},
			exactParameters:     true,
			parameterProperties: map[string]string{"job-id": "jobId", "task-id": "jobId"},
		},
		{
			canonical:       "doc.import",
			cliPath:         "doc import create",
			risk:            "medium",
			confirmation:    "not_required",
			parameters:      []string{"async", "file", "folder", "name", "workspace"},
			exactParameters: true,
		},
		{
			canonical:           "doc.import_get",
			cliPath:             "doc import get",
			risk:                "medium",
			confirmation:        "not_required",
			parameters:          []string{"task-id"},
			exactParameters:     true,
			parameterProperties: map[string]string{"task-id": ""},
		},
		{
			canonical:           "sheet.submit_export_job",
			cliPath:             "sheet export",
			risk:                "low",
			confirmation:        "not_required",
			parameters:          []string{"async", "node", "output"},
			exactParameters:     true,
			parameterProperties: map[string]string{"async": "", "node": "nodeId", "output": ""},
		},
		{
			canonical:           "sheet.export_get",
			cliPath:             "sheet export get",
			risk:                "low",
			confirmation:        "not_required",
			parameters:          []string{"task-id"},
			exactParameters:     true,
			parameterProperties: map[string]string{"task-id": "jobId"},
			forbiddenParameters: []string{"job-id"},
		},
	}

	for _, test := range cases {
		t.Run(test.canonical, func(t *testing.T) {
			leaf, ok := loaded.Snapshot.Tools[test.canonical]
			if !ok {
				t.Fatalf("catalog is missing %s", test.canonical)
			}
			if got := schemaString(leaf["cli_path"]); got != test.cliPath {
				t.Fatalf("cli_path = %q, want %q", got, test.cliPath)
			}
			if got := schemaString(leaf["risk"]); got != test.risk {
				t.Fatalf("risk = %q, want %q", got, test.risk)
			}
			if got := schemaString(leaf["confirmation"]); got != test.confirmation {
				t.Fatalf("confirmation = %q, want %q", got, test.confirmation)
			}
			parameters := schemaMap(leaf["parameters"])
			if test.exactParameters && len(parameters) != len(test.parameters) {
				t.Errorf("parameters = %v, want exactly %v", parameters, test.parameters)
			}
			for _, name := range test.parameters {
				if _, ok := parameters[name]; !ok {
					t.Errorf("missing parameter --%s", name)
				}
			}
			for name, property := range test.parameterProperties {
				parameter := parameters[name]
				if got := schemaString(parameter["property"]); got != property {
					t.Errorf("parameter --%s property = %q, want %q", name, got, property)
				}
			}
			for _, name := range test.forbiddenParameters {
				if _, ok := parameters[name]; ok {
					t.Errorf("hidden compatibility parameter --%s leaked into Schema", name)
				}
			}
			if test.confirmation == "user_required" {
				for _, example := range schemaStringSlice(leaf["examples"]) {
					if strings.Contains(example, "--yes") {
						t.Errorf("reviewed example bypasses confirmation: %q", example)
					}
				}
			}
		})
	}

	if _, ok := loaded.Index.Resolve("doc.task_get"); ok {
		t.Fatal("drive.task_get must not publish an alternate doc.task_get command identity")
	}
	if _, ok := loaded.Index.Resolve("doc import"); ok {
		t.Fatal("the runnable historical doc import parent must not be registered as a Schema alias")
	}
	if leaf, ok := loaded.Index.Resolve("sheet export create"); !ok || leaf.Identity.CanonicalPath != "sheet.submit_export_job" {
		t.Fatalf("reviewed sheet export create compatibility alias = (%#v, %t), want sheet.submit_export_job", leaf, ok)
	}

	docExport := loaded.Snapshot.Tools["doc.export"]
	for field, want := range map[string]string{
		"effect":         "read",
		"risk":           "low",
		"confirmation":   "not_required",
		"idempotency":    "idempotent",
		"interface_mode": "composite",
	} {
		if got := schemaString(docExport[field]); got != want {
			t.Errorf("doc.export %s = %q, want %q", field, got, want)
		}
	}
	if _, ok := docExport["interface_ref"]; ok {
		t.Errorf("doc.export must not publish a singular interface_ref: %#v", docExport["interface_ref"])
	}
	docExportParameters := schemaMap(docExport["parameters"])
	for name, property := range map[string]string{"node": "nodeId", "export-format": "exportFormat", "output": "", "async": ""} {
		if got := schemaString(docExportParameters[name]["property"]); got != property {
			t.Errorf("doc.export --%s property = %q, want %q", name, got, property)
		}
	}
	output := docExportParameters["output"]
	if required, _ := output["required"].(bool); required {
		t.Error("doc.export --output must not be unconditionally required")
	}
	if got := schemaString(output["required_when"]); got != "async is false" {
		t.Errorf("doc.export --output required_when = %q, want %q", got, "async is false")
	}
	async := docExportParameters["async"]
	if required, _ := async["required"].(bool); required {
		t.Error("doc.export --async must be optional")
	}
	if got := schemaString(async["type"]); got != "boolean" {
		t.Errorf("doc.export --async type = %q, want boolean", got)
	}

	docQuery := loaded.Snapshot.Tools["doc.query_export_job"]
	for field, want := range map[string]string{
		"effect":         "read",
		"risk":           "low",
		"confirmation":   "not_required",
		"idempotency":    "idempotent",
		"interface_mode": "mcp",
	} {
		if got := schemaString(docQuery[field]); got != want {
			t.Errorf("doc.query_export_job %s = %q, want %q", field, got, want)
		}
	}
	interfaceRef, _ := docQuery["interface_ref"].(map[string]any)
	if got := schemaString(interfaceRef["product_id"]); got != "doc" {
		t.Errorf("doc.query_export_job interface product = %q, want doc", got)
	}
	if got := schemaString(interfaceRef["rpc_name"]); got != "query_export_job" {
		t.Errorf("doc.query_export_job interface RPC = %q, want query_export_job", got)
	}
	docQueryParameters := schemaMap(docQuery["parameters"])
	if required, _ := docQueryParameters["job-id"]["required"].(bool); !required {
		t.Error("doc.query_export_job historical --job-id must remain required")
	}
	if required, _ := docQueryParameters["task-id"]["required"].(bool); required {
		t.Error("doc.query_export_job additive --task-id must remain optional")
	}

	docImport := loaded.Snapshot.Tools["doc.import"]
	for field, want := range map[string]string{
		"effect":         "write",
		"risk":           "medium",
		"confirmation":   "not_required",
		"idempotency":    "non_idempotent",
		"interface_mode": "composite",
	} {
		if got := schemaString(docImport[field]); got != want {
			t.Errorf("doc.import %s = %q, want %q", field, got, want)
		}
	}
	if _, ok := docImport["interface_ref"]; ok {
		t.Errorf("doc.import must not publish a singular interface_ref: %#v", docImport["interface_ref"])
	}
	docImportParameters := schemaMap(docImport["parameters"])
	for name, property := range map[string]string{
		"folder":    "targetFolderId",
		"workspace": "workspaceId",
		"name":      "fileName",
		"file":      "",
		"async":     "",
	} {
		if got := schemaString(docImportParameters[name]["property"]); got != property {
			t.Errorf("doc.import --%s property = %q, want %q", name, got, property)
		}
	}
	for _, name := range []string{"folder", "workspace", "name", "async"} {
		if required, _ := docImportParameters[name]["required"].(bool); required {
			t.Errorf("doc.import --%s must be optional", name)
		}
	}
	if required, _ := docImportParameters["file"]["required"].(bool); !required {
		t.Error("doc.import --file must be required")
	}
	if got := schemaString(docImportParameters["async"]["type"]); got != "boolean" {
		t.Errorf("doc.import --async type = %q, want boolean", got)
	}

	docImportGet := loaded.Snapshot.Tools["doc.import_get"]
	for field, want := range map[string]string{
		"effect":         "write",
		"risk":           "medium",
		"confirmation":   "not_required",
		"idempotency":    "unknown",
		"interface_mode": "composite",
	} {
		if got := schemaString(docImportGet[field]); got != want {
			t.Errorf("doc.import_get %s = %q, want %q", field, got, want)
		}
	}
	if _, ok := docImportGet["interface_ref"]; ok {
		t.Errorf("doc.import_get must remain an unpinned composite query: %#v", docImportGet["interface_ref"])
	}

	sheetExport := loaded.Snapshot.Tools["sheet.submit_export_job"]
	for field, want := range map[string]string{
		"effect":         "read",
		"risk":           "low",
		"confirmation":   "not_required",
		"idempotency":    "idempotent",
		"interface_mode": "mcp",
	} {
		if got := schemaString(sheetExport[field]); got != want {
			t.Errorf("sheet.submit_export_job %s = %q, want %q", field, got, want)
		}
	}
	sheetExportInterface, _ := sheetExport["interface_ref"].(map[string]any)
	if got := schemaString(sheetExportInterface["product_id"]); got != "sheet" {
		t.Errorf("sheet.submit_export_job interface product = %q, want sheet", got)
	}
	if got := schemaString(sheetExportInterface["rpc_name"]); got != "submit_export_job" {
		t.Errorf("sheet.submit_export_job interface RPC = %q, want submit_export_job", got)
	}
	sheetExportParameters := schemaMap(sheetExport["parameters"])
	for _, name := range []string{"async", "output"} {
		if required, _ := sheetExportParameters[name]["required"].(bool); required {
			t.Errorf("sheet.submit_export_job --%s must be optional", name)
		}
		if got := schemaString(sheetExportParameters[name]["required_when"]); got != "" {
			t.Errorf("sheet.submit_export_job --%s required_when = %q, want empty", name, got)
		}
	}
	if got := schemaString(sheetExportParameters["async"]["type"]); got != "boolean" {
		t.Errorf("sheet.submit_export_job --async type = %q, want boolean", got)
	}

	sheetExportGet := loaded.Snapshot.Tools["sheet.export_get"]
	for field, want := range map[string]string{
		"effect":         "read",
		"risk":           "low",
		"confirmation":   "not_required",
		"idempotency":    "idempotent",
		"interface_mode": "mcp",
	} {
		if got := schemaString(sheetExportGet[field]); got != want {
			t.Errorf("sheet.export_get %s = %q, want %q", field, got, want)
		}
	}
	sheetExportGetInterface, _ := sheetExportGet["interface_ref"].(map[string]any)
	if got := schemaString(sheetExportGetInterface["product_id"]); got != "doc" {
		t.Errorf("sheet.export_get interface product = %q, want doc", got)
	}
	if got := schemaString(sheetExportGetInterface["rpc_name"]); got != "query_export_job" {
		t.Errorf("sheet.export_get interface RPC = %q, want query_export_job", got)
	}
	if _, ok := sheetExportGet["source_product_id"]; ok {
		t.Errorf("sheet.export_get must express Doc routing through interface_ref, not source_product_id: %#v", sheetExportGet)
	}
}

func TestSheetExportSkillEvidenceTargetsCreateAndGetLeaves(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	for _, relativePath := range []string{
		"skills/mono/references/products/sheet.md",
		"skills/mono/references/products/sheet/sheet-export.md",
		"skills/multi/dingtalk-sheet/references/sheet.md",
		"skills/multi/dingtalk-sheet/references/sheet/sheet-export.md",
	} {
		body, err := os.ReadFile(filepath.Join(repositoryRoot, relativePath))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		text := string(body)
		for _, fragment := range []string{"dws sheet export create --node", "--async", "TaskResult.id", "dws sheet export get --task-id", "PENDING", "PROCESSING", "SUCCESS", "FAILED", "TIMEOUT"} {
			if !strings.Contains(text, fragment) {
				t.Errorf("%s does not document %q", relativePath, fragment)
			}
		}
	}

	for _, relativePath := range []string{
		"skills/mono/references/url-patterns.md",
		"skills/multi/dingtalk-sheet/references/url-patterns.md",
	} {
		body, err := os.ReadFile(filepath.Join(repositoryRoot, relativePath))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		text := string(body)
		if strings.Contains(text, "开源 dws CLI 暂未暴露在线表格导出能力") {
			t.Errorf("%s still claims Sheet export is unavailable", relativePath)
		}
		if !strings.Contains(text, "dws sheet export create --node") || !strings.Contains(text, "dws sheet export get --task-id") {
			t.Errorf("%s does not route Sheet export create/get", relativePath)
		}
	}

	for _, relativePath := range []string{
		"internal/cli/schema_hints/index.json",
		"internal/cli/schema_hints/reference-review.json",
	} {
		body, err := os.ReadFile(filepath.Join(repositoryRoot, relativePath))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		var document struct {
			ReferenceReview map[string]struct {
				Status string `json:"status"`
				Target string `json:"target"`
			} `json:"reference_review"`
		}
		if err := json.Unmarshal(body, &document); err != nil {
			t.Fatalf("decode %s: %v", relativePath, err)
		}
		if review, ok := document.ReferenceReview["sheet export"]; ok {
			t.Errorf("%s sheet export review = %#v, want historical primary resolved by CommandRegistry", relativePath, review)
		}
	}
}

func TestDocImportSkillEvidenceTargetsCreateLeaf(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	docPaths := []string{
		"skills/mono/references/products/doc.md",
		"skills/mono/references/products/doc/doc-import.md",
		"skills/multi/dingtalk-doc/references/doc.md",
		"skills/multi/dingtalk-doc/references/doc/doc-import.md",
	}
	for _, relativePath := range docPaths {
		body, err := os.ReadFile(filepath.Join(repositoryRoot, relativePath))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		text := string(body)
		if !strings.Contains(text, "dws doc import create --file") {
			t.Errorf("%s does not cite the Schema-bindable doc import create leaf", relativePath)
		}
		if !strings.Contains(text, "--async") || !strings.Contains(text, "TaskResult.id") {
			t.Errorf("%s does not document async upload semantics and TaskResult.id", relativePath)
		}
	}
	for _, relativePath := range []string{
		"skills/mono/references/products/doc/doc-import.md",
		"skills/multi/dingtalk-doc/references/doc/doc-import.md",
	} {
		body, err := os.ReadFile(filepath.Join(repositoryRoot, relativePath))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		if strings.Contains(string(body), "taskId") {
			t.Errorf("%s uses taskId for the async field/concept; want TaskResult.id or --task-id", relativePath)
		}
	}

	for _, relativePath := range []string{
		"internal/cli/schema_hints/index.json",
		"internal/cli/schema_hints/reference-review.json",
	} {
		body, err := os.ReadFile(filepath.Join(repositoryRoot, relativePath))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		var document struct {
			ReferenceReview map[string]struct {
				Status string `json:"status"`
				Target string `json:"target"`
			} `json:"reference_review"`
		}
		if err := json.Unmarshal(body, &document); err != nil {
			t.Fatalf("decode %s: %v", relativePath, err)
		}
		review := document.ReferenceReview["doc import"]
		if review.Status != "alias" || review.Target != "doc import create" {
			t.Errorf("%s doc import review = %#v, want alias to doc import create", relativePath, review)
		}
	}
}

func TestWukongParityRuntimeSchemaConstraints(t *testing.T) {
	assertConstraintGroup := func(t *testing.T, groups [][]string, want ...string) {
		t.Helper()
		for _, group := range groups {
			if strings.Join(group, "\x00") == strings.Join(want, "\x00") {
				return
			}
		}
		t.Fatalf("constraint groups %#v do not contain %#v", groups, want)
	}

	if _, ok := runtimeSchemaConstraintsByCanonical["aitable.field_create"]; ok {
		t.Fatal("aitable.field_create must not tighten the existing public Schema contract")
	}

	transferOwner := runtimeSchemaConstraintsByCanonical["drive.permission_transfer_owner"]
	assertConstraintGroup(t, transferOwner.RequireOneOf, "node", "workspace")

	formulaVerify := runtimeSchemaConstraintsByCanonical["sheet.formula_verify"]
	assertConstraintGroup(t, formulaVerify.MutuallyExclusive, "targets", "sheet-id")
	assertConstraintGroup(t, formulaVerify.MutuallyExclusive, "targets", "range")
}
