// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0

package cli

import "testing"

func TestEmbeddedSchemaCatalogIntegrity(t *testing.T) {
	loaded := runtimeEmbeddedSchemaCatalog
	if !embeddedSchemaCatalogAvailable() {
		t.Fatal("embedded schema catalog is unavailable or failed integrity validation")
	}
	if got, want := len(loaded.Snapshot.Tools), 504; got != want {
		t.Fatalf("embedded tools = %d, want %d", got, want)
	}
	if got, want := len(loaded.Products), 21; got != want {
		t.Fatalf("embedded products = %d, want %d", got, want)
	}
	if got := schemaString(loaded.Snapshot.Catalog["source"]); got != "embedded-command-catalog" {
		t.Fatalf("catalog source = %q", got)
	}
}

func TestEmbeddedSchemaCatalogProgressiveQueries(t *testing.T) {
	overview, err := embeddedSchemaPayload(nil)
	if err != nil {
		t.Fatal(err)
	}
	compact := compactSchemaOverviewPayload(overview)
	if got, want := schemaProductToolCount(map[string]any{"tools": compact["products"]}), 21; got != want {
		t.Fatalf("compact product count = %d, want %d", got, want)
	}

	leaf, err := embeddedSchemaPayload([]string{"calendar event create"})
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaString(leaf["canonical_path"]); got != "calendar.create_calendar_event" {
		t.Fatalf("canonical path = %q", got)
	}
	if len(schemaMapSlice(leaf["parameters"])) != 0 {
		t.Fatal("parameters unexpectedly decoded as a list")
	}
	if parameters, ok := leaf["parameters"].(map[string]any); !ok || len(parameters) == 0 {
		t.Fatal("calendar.create_event parameters are empty")
	}

	group, err := embeddedSchemaPayload([]string{"calendar.event"})
	if err != nil {
		t.Fatal(err)
	}
	if schemaProductToolCount(map[string]any{"tools": group["tools"]}) == 0 {
		t.Fatal("calendar.event group is empty")
	}

	alias, err := embeddedSchemaPayload([]string{"aitable record list"})
	if err != nil {
		t.Fatal(err)
	}
	if alias["is_alias"] != true || schemaString(alias["cli_path"]) != "aitable record list" {
		t.Fatalf("alias query did not preserve compatibility path: %#v", alias)
	}
	if schemaString(alias["canonical_path"]) != "aitable.query_records" {
		t.Fatalf("alias canonical path = %q", schemaString(alias["canonical_path"]))
	}
}
