// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import "testing"

func TestCrossPlatformCoverageWikiMemberRemoveFinalSchema(t *testing.T) {
	for _, cliPath := range []string{"wiki member remove", "wiki +member-remove"} {
		t.Run(cliPath, func(t *testing.T) {
			full := executeShortcutSchemaQuery(t, "--cli-path", cliPath)
			compact := executeShortcutSchemaQuery(t, "--cli-path", cliPath, "--compact")
			for field, want := range map[string]string{
				"effect": "write", "risk": "medium", "confirmation": "user_required", "idempotency": "unknown",
			} {
				if got := full[field]; got != want {
					t.Errorf("full %s=%#v, want %q", field, got, want)
				}
				if got := compact[field]; got != want {
					t.Errorf("compact %s=%#v, want %q", field, got, want)
				}
			}
			provenance := schemaContractMap(full["field_provenance"])["confirmation"]
			if provenance["precedence"] != "contract_final" || provenance["value"] != "user_required" {
				t.Fatalf("confirmation delivery/winner mismatch: %#v", provenance)
			}
		})
	}
	for _, cliPath := range []string{"wiki +member-add", "wiki +member-update"} {
		leaf := executeShortcutSchemaQuery(t, "--cli-path", cliPath, "--compact")
		if got := leaf["confirmation"]; got != "not_required" {
			t.Errorf("unrelated %s confirmation=%#v", cliPath, got)
		}
	}
}
