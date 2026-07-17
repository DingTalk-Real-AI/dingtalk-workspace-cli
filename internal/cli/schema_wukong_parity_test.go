// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
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
