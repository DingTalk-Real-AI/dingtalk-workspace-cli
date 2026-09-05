// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestRuntimeRenderingBoundary(t *testing.T) {
	tool := ToolSpec{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "sample",
			Name:           "run",
			CanonicalPath:  "sample.run",
			Path:           "source.run",
			CLIPath:        "sample group run",
			PrimaryCLIPath: "sample group run",
			Aliases:        []string{"sample legacy run"},
		},
		Description: "run sample",
		Parameters: []ParameterSpec{{
			Name: "id", Type: "string", Description: "id", Required: true,
			Property: "request.id", Example: json.RawMessage(`"1"`),
		}},
		Result: &contract.ResultSpec{
			Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
			DataSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"id"}}}`),
		},
	}
	registry, err := SchemaRegistryFromRuntime("runtime-assembled", []ProductSpec{{ID: "sample", Tools: []ToolSpec{tool}}})
	if err != nil {
		t.Fatalf("SchemaRegistryFromRuntime() error = %v", err)
	}
	index, err := registry.Index()
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	hashes := TrustedHashes{CatalogHash: "sha256:catalog", SurfaceHash: "sha256:surface"}
	for name, render := range map[string]func() (map[string]any, error){
		"overview": func() (map[string]any, error) { return RenderOverview(registry, hashes) },
		"all":      func() (map[string]any, error) { return RenderAll(registry, hashes) },
		"catalog":  func() (map[string]any, error) { return RenderCatalog(registry, hashes) },
	} {
		t.Run(name, func(t *testing.T) {
			payload, renderErr := render()
			if renderErr != nil {
				t.Fatalf("render error = %v", renderErr)
			}
			if payload["catalog_hash"] != hashes.CatalogHash || payload["surface_hash"] != hashes.SurfaceHash {
				t.Fatalf("hashes = %#v", payload)
			}
		})
	}

	leaf, err := RenderQuery(registry, index, "sample/legacy/run")
	if err != nil {
		t.Fatalf("RenderQuery(alias) error = %v", err)
	}
	if leaf["cli_path"] != "sample legacy run" || leaf["is_alias"] != true {
		t.Fatalf("alias identity = %#v", leaf)
	}
	if _, exists := leaf["catalog_hash"]; exists {
		t.Fatalf("leaf unexpectedly carries hashes: %#v", leaf)
	}
	group, err := RenderQuery(registry, index, "sample/group")
	if err != nil || group["level"] != "group" {
		t.Fatalf("RenderQuery(group) = %#v, %v", group, err)
	}
	product, err := RenderQuery(registry, index, "sample")
	if err != nil || product["level"] != "product" {
		t.Fatalf("RenderQuery(product) = %#v, %v", product, err)
	}
	for name, payload := range map[string]map[string]any{"leaf": leaf, "group": group, "product": product} {
		if _, exists := payload["catalog_hash"]; exists {
			t.Errorf("%s unexpectedly carries catalog_hash: %#v", name, payload)
		}
		if _, exists := payload["surface_hash"]; exists {
			t.Errorf("%s unexpectedly carries surface_hash: %#v", name, payload)
		}
	}
	_, err = RenderQuery(registry, index, "missing")
	var unknown UnknownPathError
	if !errors.As(err, &unknown) || err.Error() != `unknown runtime schema path "missing"` {
		t.Fatalf("unknown error = %T %v", err, err)
	}

	compact := Compact(leaf)
	if _, exists := compact["property"]; exists {
		t.Fatalf("compact leaf leaked property: %#v", compact)
	}
	parameters := compact["parameters"].(map[string]any)
	parameter := parameters["id"].(map[string]any)
	if _, exists := parameter["property"]; exists {
		t.Fatalf("compact parameter leaked property: %#v", parameter)
	}
	if !reflect.DeepEqual(compact["result"], leaf["result"]) {
		t.Fatalf("compact result changed: got %#v want %#v", compact["result"], leaf["result"])
	}
}

func TestSchemaIndexResolveQueryLocatorParity(t *testing.T) {
	tool := ToolSpec{Identity: contract.ToolIdentitySpec{
		ProductID:       "public",
		SourceProductID: "source",
		Name:            "run",
		CanonicalPath:   "public.run",
		Path:            "contract.run",
		CLIPath:         "public group run",
		PrimaryCLIPath:  "public group run",
		Aliases:         []string{"public legacy run"},
	}}
	registry, err := SchemaRegistryFromRuntime("test", []ProductSpec{{ID: "public", Tools: []ToolSpec{tool}}})
	if err != nil {
		t.Fatal(err)
	}
	index, err := registry.Index()
	if err != nil {
		t.Fatal(err)
	}
	for _, locator := range []string{
		"public.run",
		"contract.run",
		"source.run",
		"public group run",
		"public/group/run",
		"public.group.run",
		"dws  public   group run",
		"public legacy run",
		"public/legacy/run",
		"public.legacy.run",
	} {
		resolved, ok := index.ResolveQuery(locator)
		if !ok || resolved.Identity.CanonicalPath != "public.run" {
			t.Errorf("ResolveQuery(%q) = %#v, %v", locator, resolved.Identity, ok)
		}
	}
}
