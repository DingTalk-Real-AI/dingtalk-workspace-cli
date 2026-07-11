// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import "testing"

func TestSchemaCompatibilityAllowsAdditions(t *testing.T) {
	baseline := schemaContract{Version: 1, Products: map[string]productSchema{
		"doc": {Tools: map[string]toolSchema{
			"doc.create": {Parameters: map[string]string{"title": `"string"`}},
		}},
	}}
	current := cloneContract(baseline)
	current.Products["doc"].Tools["doc.read"] = toolSchema{}
	current.Products["sheet"] = productSchema{Tools: map[string]toolSchema{}}
	if failures := checkCompatibility(baseline, current); len(failures) != 0 {
		t.Fatalf("additions should be compatible: %v", failures)
	}
}

func TestSchemaCompatibilityRejectsRemovedAndNewRequiredInputs(t *testing.T) {
	baseline := schemaContract{Version: 1, Products: map[string]productSchema{
		"doc": {Tools: map[string]toolSchema{
			"doc.create": {Parameters: map[string]string{"title": `"string"`}},
			"doc.read":   {},
		}},
	}}
	current := schemaContract{Version: 1, Products: map[string]productSchema{
		"doc": {Tools: map[string]toolSchema{
			"doc.create": {
				Parameters: map[string]string{"title": `"string"`, "folder": `"string"`},
				Required:   []string{"folder"},
			},
		}},
	}}
	if failures := checkCompatibility(baseline, current); len(failures) != 2 {
		t.Fatalf("got %d failures, want 2: %v", len(failures), failures)
	}
}
