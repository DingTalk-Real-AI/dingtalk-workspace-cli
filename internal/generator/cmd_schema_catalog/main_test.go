// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

func TestGenerateSchemaCatalogResolvesBuildExactlyOnce(t *testing.T) {
	root := app.NewRootCommand()
	resolveCalls := 0
	resolvedRegistryHash := ""
	resolver := func(candidate *cobra.Command) (cli.ResolvedSchemaBuild, error) {
		resolveCalls++
		if candidate != root {
			t.Fatalf("resolver root = %p, want generator root %p", candidate, root)
		}
		resolved, err := cli.ResolveSchemaBuild(candidate)
		if err == nil {
			resolvedRegistryHash = resolved.RegistryHash()
		}
		return resolved, err
	}
	outputPath := filepath.Join(t.TempDir(), "schema_catalog")
	if err := generateSchemaCatalogWithResolver(root, "", outputPath, resolver); err != nil {
		t.Fatalf("generateSchemaCatalogWithResolver() error = %v", err)
	}
	if resolveCalls != 1 {
		t.Fatalf("Schema build resolver calls = %d, want exactly 1", resolveCalls)
	}
	snapshot := loadSplitSchemaCatalogSnapshot(t, outputPath)
	if snapshot.SurfaceHash != resolvedRegistryHash {
		t.Fatalf("snapshot Registry hash = %q, want once-resolved hash %q", snapshot.SurfaceHash, resolvedRegistryHash)
	}
}

// loadSplitSchemaCatalogSnapshot reassembles the per-product split layout
// (catalog.json + tools/<product>.json) written by the generator back into a
// single SchemaCatalogSnapshot, mirroring the production loader.
func loadSplitSchemaCatalogSnapshot(t *testing.T, dir string) cli.SchemaCatalogSnapshot {
	t.Helper()
	var envelope struct {
		Version     int            `json:"version"`
		SurfaceHash string         `json:"surface_hash,omitempty"`
		SourceHash  string         `json:"source_hash"`
		Catalog     map[string]any `json:"catalog"`
	}
	envData, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatalf("read generated catalog.json: %v", err)
	}
	if err := json.Unmarshal(envData, &envelope); err != nil {
		t.Fatalf("decode generated catalog.json: %v", err)
	}
	snapshot := cli.SchemaCatalogSnapshot{
		Version:     envelope.Version,
		SurfaceHash: envelope.SurfaceHash,
		SourceHash:  envelope.SourceHash,
		Catalog:     envelope.Catalog,
		Tools:       map[string]map[string]any{},
	}
	shards, err := filepath.Glob(filepath.Join(dir, "tools", "*.json"))
	if err != nil {
		t.Fatalf("glob tool shards: %v", err)
	}
	for _, shardPath := range shards {
		data, err := os.ReadFile(shardPath)
		if err != nil {
			t.Fatalf("read tool shard %s: %v", shardPath, err)
		}
		var shard struct {
			Tools map[string]map[string]any `json:"tools"`
		}
		if err := json.Unmarshal(data, &shard); err != nil {
			t.Fatalf("decode tool shard %s: %v", shardPath, err)
		}
		for canonical, spec := range shard.Tools {
			snapshot.Tools[canonical] = spec
		}
	}
	return snapshot
}

func TestValidateDeprecatedSurfaceAcceptsEmbeddedRegistrySource(t *testing.T) {
	// Registry is now per-product shards; merge into a temp file for the
	// deprecated -surface compatibility check (which reads a single JSON).
	merged := mergeRegistryShardsForTest(t, filepath.Join("..", "..", "cli", "schema_command_registry"))
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, merged, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateDeprecatedSurface(path); err != nil {
		t.Fatalf("validateDeprecatedSurface() error = %v", err)
	}
}

func TestValidateDeprecatedSurfaceRejectsDifferentIdentitySource(t *testing.T) {
	merged := mergeRegistryShardsForTest(t, filepath.Join("..", "..", "cli", "schema_command_registry"))
	altered := strings.Replace(string(merged), `"aisearch.search_enterprise_behavior"`, `"aisearch.not_reviewed"`, 1)
	if altered == string(merged) {
		t.Fatal("test fixture did not contain expected canonical path")
	}
	path := filepath.Join(t.TempDir(), "different-registry.json")
	if err := os.WriteFile(path, []byte(altered), 0o600); err != nil {
		t.Fatal(err)
	}

	err := validateDeprecatedSurface(path)
	if err == nil || !strings.Contains(err.Error(), "disagrees with the embedded reviewed registry") {
		t.Fatalf("validateDeprecatedSurface() error = %v, want registry disagreement", err)
	}
}

func TestValidateDeprecatedSurfaceAllowsOmittedCompatibilityFlag(t *testing.T) {
	if err := validateDeprecatedSurface(""); err != nil {
		t.Fatalf("validateDeprecatedSurface() error = %v", err)
	}
}

func TestValidateCatalogOutputIsolationProtectsEveryInputLayer(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"skills/mono/SKILL.md",
		"skills/mono/references/intent-guide.md",
		"internal/cli/schema_command_registry",
		"internal/cli/schema_mcp_metadata.json",
		"internal/cli/schema_mcp_service_review.json",
		"internal/cli/schema_parameter_bindings.json",
		"internal/cli/schema_command_exclusions.json",
	}
	for _, relative := range files {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	metadataDir := filepath.Join(root, "internal/cli/schema_agent_metadata")
	if err := os.MkdirAll(metadataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadataDir, "index.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"skills/mono/references/products", "internal/cli/schema_hints"} {
		if err := os.MkdirAll(filepath.Join(root, relative), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name   string
		output string
		want   string
	}{
		{name: "registry", output: filepath.Join(root, "internal/cli/schema_command_registry"), want: "CommandRegistry"},
		{name: "hints", output: filepath.Join(root, "internal/cli/schema_hints"), want: "structured metadata source directory"},
		{name: "metadata member", output: filepath.Join(metadataDir, "replacement.json"), want: "Agent metadata"},
		{name: "metadata directory", output: metadataDir, want: "Agent metadata"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateCatalogOutputIsolation(root, test.output, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateCatalogOutputIsolation() error = %v, want %q", err, test.want)
			}
		})
	}
	if err := validateCatalogOutputIsolation(root, filepath.Join(root, "internal/cli/schema_catalog"), ""); err != nil {
		t.Fatalf("safe output rejected: %v", err)
	}
	if err := validateCatalogOutputIsolation(root, filepath.Join(t.TempDir(), "schema_catalog"), ""); err != nil {
		t.Fatalf("external temporary output rejected: %v", err)
	}
	if err := validateCatalogOutputIsolation(root, filepath.Join(root, "skills/mono/overwrite.json"), ""); err == nil || !strings.Contains(err.Error(), "not a canonical generated delivery target") {
		t.Fatalf("non-canonical repository output error = %v", err)
	}
}

// mergeRegistryShardsForTest reads per-product registry shards from dir and
// returns a single merged JSON document matching the pre-split layout.
func mergeRegistryShardsForTest(t *testing.T, dir string) []byte {
	t.Helper()
	envelopeData, err := os.ReadFile(filepath.Join(dir, "registry.json"))
	if err != nil {
		t.Fatalf("read registry envelope: %v", err)
	}
	var envelope struct {
		Schema  string `json:"$schema,omitempty"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(envelopeData, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "products"))
	if err != nil {
		t.Fatalf("read products dir: %v", err)
	}
	products := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		shardData, err := os.ReadFile(filepath.Join(dir, "products", entry.Name()))
		if err != nil {
			t.Fatalf("read shard %s: %v", entry.Name(), err)
		}
		products = append(products, json.RawMessage(shardData))
	}
	productsJSON, err := json.Marshal(products)
	if err != nil {
		t.Fatalf("marshal products: %v", err)
	}
	result := struct {
		Schema   string          `json:"$schema,omitempty"`
		Version  int             `json:"version"`
		Products json.RawMessage `json:"products"`
	}{
		Schema:  envelope.Schema,
		Version: envelope.Version,
	}
	result.Products = productsJSON
	merged, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal merged: %v", err)
	}
	return merged
}
