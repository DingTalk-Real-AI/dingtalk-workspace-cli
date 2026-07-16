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
		canonical    string
		cliPath      string
		risk         string
		confirmation string
		parameters   []string
	}{
		{"drive.permission_apply", "drive permission apply", "medium", "user_required", []string{"node", "role", "users"}},
		{"drive.permission_apply_info", "drive permission apply-info", "low", "not_required", []string{"node"}},
		{"drive.permission_transfer_owner", "drive permission transfer-owner", "high", "user_required", []string{"new-owner", "node", "recursive", "reserve-role", "workspace"}},
		{"drive.star_add", "drive star add", "low", "not_required", []string{"node"}},
		{"drive.star_remove", "drive star remove", "low", "not_required", []string{"node"}},
		{"drive.star_list", "drive star list", "low", "not_required", []string{"content-types", "cursor", "limit", "order-by", "resource-types", "sort"}},
		{"sheet.formula_verify", "sheet formula-verify", "low", "not_required", []string{"exit-on-error", "max-cells", "max-locations-per-error", "node", "range", "sheet-id", "targets"}},
		{"sheet.version_save", "sheet version save", "medium", "not_required", []string{"node"}},
		{"sheet.version_list", "sheet version list", "low", "not_required", []string{"cursor", "limit", "node"}},
		{"sheet.version_revert", "sheet version revert", "medium", "user_required", []string{"node", "version"}},
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
			for _, name := range test.parameters {
				if _, ok := parameters[name]; !ok {
					t.Errorf("missing parameter --%s", name)
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
