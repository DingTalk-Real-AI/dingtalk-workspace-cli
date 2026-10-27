// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package interfacesnapshot

import (
	"errors"
	"os"
	"strings"
	"testing"
	"testing/iotest"
)

const enumManifestFixture = `{"version":1,"migrations":[{"kind":"schema_enum_change","command":"dws doc create","schema":{"product_id":"doc","tool_id":"doc.create","parameter":"format"},"before":{"enum":[]},"after":{"enum":["ALL","PARTIAL"]},"state":"pending","reason":"Explicit reviewed domain correction."}]}`

func TestCrossPlatformCoverageSchemaParameterManifestStrictParsing(t *testing.T) {
	if _, err := ReadSchemaParameterMigrationManifest(strings.NewReader(enumManifestFixture)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, body string }{
		{"syntax", "{"},
		{"trailing", enumManifestFixture + "{}"},
		{"trailing-invalid", enumManifestFixture + "!"},
		{"unknown", strings.Replace(enumManifestFixture, `"version":1`, `"extra":1,"version":1`, 1)},
		{"duplicate", strings.Replace(enumManifestFixture, `"version":1`, `"version":1,"version":1`, 1)},
		{"case", strings.Replace(enumManifestFixture, `"version"`, `"Version"`, 1)},
		{"version", strings.Replace(enumManifestFixture, `"version":1`, `"version":2`, 1)},
		{"fractional-version", strings.Replace(enumManifestFixture, `"version":1`, `"version":1.5`, 1)},
		{"null-array", strings.Replace(enumManifestFixture, `"enum":[]`, `"enum":null`, 1)},
		{"missing-array", strings.Replace(enumManifestFixture, `"before":{"enum":[]}`, `"before":{}`, 1)},
		{"null-migrations", `{"version":1,"migrations":null}`},
		{"missing-migrations", `{"version":1}`},
		{"kind", strings.Replace(enumManifestFixture, "schema_enum_change", "flag_rename", 1)},
		{"state", strings.Replace(enumManifestFixture, "pending", "approved", 1)},
		{"command", strings.Replace(enumManifestFixture, "dws doc create", "dws doc *", 1)},
		{"product", strings.Replace(enumManifestFixture, `"product_id":"doc"`, `"product_id":"*"`, 1)},
		{"tool", strings.Replace(enumManifestFixture, "doc.create", "doc/*", 1)},
		{"parameter", strings.Replace(enumManifestFixture, `"parameter":"format"`, `"parameter":"*"`, 1)},
		{"reason", strings.Replace(enumManifestFixture, "Explicit reviewed domain correction.", " ", 1)},
		{"reason-trim", strings.Replace(enumManifestFixture, "Explicit reviewed", " Explicit reviewed", 1)},
		{"unsorted", strings.Replace(enumManifestFixture, `["ALL","PARTIAL"]`, `["PARTIAL","ALL"]`, 1)},
		{"duplicate-value", strings.Replace(enumManifestFixture, `["ALL","PARTIAL"]`, `["ALL","ALL"]`, 1)},
		{"empty-value", strings.Replace(enumManifestFixture, `["ALL","PARTIAL"]`, `[""]`, 1)},
		{"trim-value", strings.Replace(enumManifestFixture, `["ALL","PARTIAL"]`, `[" ALL"]`, 1)},
		{"no-change", strings.Replace(enumManifestFixture, `["ALL","PARTIAL"]`, `[]`, 1)},
		{"same-nonempty", strings.Replace(enumManifestFixture, `"enum":[]`, `"enum":["ALL","PARTIAL"]`, 1)},
		{"wrong-type", strings.Replace(enumManifestFixture, `["ALL","PARTIAL"]`, `[1]`, 1)},
		{"duplicate-entry", strings.Replace(enumManifestFixture, `}]}`, `},`+strings.TrimSuffix(strings.TrimPrefix(enumManifestFixture, `{"version":1,"migrations":[`), `]}`)+`]}`, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ReadSchemaParameterMigrationManifest(strings.NewReader(test.body)); err == nil {
				t.Fatalf("accepted invalid manifest: %s", test.body)
			}
		})
	}
	if _, err := ReadSchemaParameterMigrationManifest(iotest.ErrReader(errors.New("read failure"))); err == nil {
		t.Fatal("accepted reader failure")
	}
}

func TestApprovedSchemaParameterMigrationManifestRemainsValid(t *testing.T) {
	f, err := os.Open("../../scripts/policy/interface-migrations/approved-schema-parameter-migrations-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := ReadSchemaParameterMigrationManifest(f); err != nil {
		t.Fatal(err)
	}
}
