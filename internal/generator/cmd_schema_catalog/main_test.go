// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageMainGeneratesCatalogToTemporaryFile(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	oldArgs, oldFlags := os.Args, flag.CommandLine
	t.Cleanup(func() {
		os.Args, flag.CommandLine = oldArgs, oldFlags
	})
	flag.CommandLine = flag.NewFlagSet("schema-catalog-coverage", flag.ContinueOnError)
	os.Args = []string{"cmd_schema_catalog", "-root", repositoryRoot, "-output", filepath.Join(t.TempDir(), "schema_catalog")}
	main()
}

func TestCrossPlatformCoverageCatalogMainReportsIsolationAndGenerationFailures(t *testing.T) {
	originalArgs, originalFlags := os.Args, flag.CommandLine
	originalValidate := validateCatalogParameterBindings
	originalExit := exitCatalogProcess
	t.Cleanup(func() {
		os.Args, flag.CommandLine = originalArgs, originalFlags
		validateCatalogParameterBindings = originalValidate
		exitCatalogProcess = originalExit
	})
	exitCatalogProcess = func(int) { panic("exit") }
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	invoke := func(args ...string) {
		flag.CommandLine = flag.NewFlagSet("catalog-errors", flag.ContinueOnError)
		os.Args = append([]string{"cmd_schema_catalog"}, args...)
		defer func() {
			if recover() != "exit" {
				t.Fatal("main did not exit")
			}
		}()
		main()
	}
	invoke("-root", repositoryRoot, "-output", filepath.Join(repositoryRoot, "internal/cli/param_concepts.json"))
	validateCatalogParameterBindings = func() error { return errors.New("bindings") }
	invoke("-root", repositoryRoot, "-output", filepath.Join(t.TempDir(), "catalog.json"))
}

func TestCrossPlatformCoverageGenerateSchemaCatalogFailureEdges(t *testing.T) {
	originalValidate := validateCatalogParameterBindings
	originalSnapshot := buildCatalogSnapshot
	originalInstall := installCatalogAgentMetadata
	originalMkdir := makeCatalogDirectory
	originalWrite := writeCatalogFile
	t.Cleanup(func() {
		validateCatalogParameterBindings = originalValidate
		buildCatalogSnapshot = originalSnapshot
		installCatalogAgentMetadata = originalInstall
		makeCatalogDirectory = originalMkdir
		writeCatalogFile = originalWrite
		cli.ClearBuildTimeAgentMetadata()
	})
	installCatalogAgentMetadata = func(string, *cobra.Command) error { return nil }
	root := &cobra.Command{Use: "dws"}
	resolver := func(*cobra.Command) (cli.ResolvedSchemaBuild, error) { return cli.ResolvedSchemaBuild{}, nil }
	output := filepath.Join(t.TempDir(), "catalog.json")

	if err := generateSchemaCatalogWithResolver(".", nil, "", output, "", resolver); err == nil || !strings.Contains(err.Error(), "root is nil") {
		t.Fatalf("nil root error = %v", err)
	}
	if err := generateSchemaCatalogWithResolver(".", root, "", output, "", nil); err == nil || !strings.Contains(err.Error(), "resolver is nil") {
		t.Fatalf("nil resolver error = %v", err)
	}
	if err := generateSchemaCatalogWithResolver(".", root, filepath.Join(t.TempDir(), "missing.json"), output, "", resolver); err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("retired surface error = %v", err)
	}
	validateCatalogParameterBindings = func() error { return errors.New("bindings") }
	if err := generateSchemaCatalogWithResolver(".", root, "", output, "", resolver); err == nil || !strings.Contains(err.Error(), "parameter binding") {
		t.Fatalf("binding error = %v", err)
	}
	validateCatalogParameterBindings = func() error { return nil }
	if err := generateSchemaCatalogWithResolver(".", root, "", output, "", func(*cobra.Command) (cli.ResolvedSchemaBuild, error) {
		return cli.ResolvedSchemaBuild{}, errors.New("resolve")
	}); err == nil || !strings.Contains(err.Error(), "resolve final") {
		t.Fatalf("resolver error = %v", err)
	}
	buildCatalogSnapshot = func(cli.ResolvedSchemaBuild, cli.SchemaCatalogBuildOptions) (cli.SchemaCatalogSnapshot, error) {
		return cli.SchemaCatalogSnapshot{}, errors.New("snapshot")
	}
	if err := generateSchemaCatalogWithResolver(".", root, "", output, "", resolver); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("snapshot error = %v", err)
	}
	buildCatalogSnapshot = func(cli.ResolvedSchemaBuild, cli.SchemaCatalogBuildOptions) (cli.SchemaCatalogSnapshot, error) {
		return cli.SchemaCatalogSnapshot{}, nil
	}
	makeCatalogDirectory = func(string, os.FileMode) error { return errors.New("mkdir") }
	if err := generateSchemaCatalogWithResolver(".", root, "", output, "", resolver); err == nil || !strings.Contains(err.Error(), "create schema catalog") {
		t.Fatalf("mkdir error = %v", err)
	}
	makeCatalogDirectory = func(string, os.FileMode) error { return nil }
	writeCatalogFile = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := generateSchemaCatalogWithResolver(".", root, "", output, "", resolver); err == nil || !strings.Contains(err.Error(), "write schema catalog") {
		t.Fatalf("write error = %v", err)
	}
	writeCatalogFile = func(string, []byte, os.FileMode) error { return nil }
	originalMetaIndex := writeCatalogMetaIndex
	t.Cleanup(func() { writeCatalogMetaIndex = originalMetaIndex })
	writeCatalogMetaIndex = func(cli.SchemaCatalogSnapshot, string) error {
		return errors.New("meta-index")
	}
	if err := generateSchemaCatalogWithResolver(".", root, "", output, "", resolver); err == nil || !strings.Contains(err.Error(), "meta-index") {
		t.Fatalf("meta-index write error = %v", err)
	}

	if got := resolveCatalogRootPath("root", "relative.json"); got != filepath.Join("root", "relative.json") {
		t.Fatalf("resolved path = %q", got)
	}
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCatalogOutputIsolation(repositoryRoot, filepath.Join(t.TempDir(), "catalog.json"), filepath.Join(t.TempDir(), "schema_meta_index.gob")); err != nil {
		t.Fatalf("safe output rejected: %v", err)
	}
}

func TestInstallBuildTimeAgentMetadataDoesNotWriteRetiredDirectory(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	retired := filepath.Join(repositoryRoot, "internal/cli/schema_agent_metadata")
	if _, err := os.Stat(retired); !os.IsNotExist(err) {
		t.Fatalf("retired schema_agent_metadata/ should stay absent before inject: %v", err)
	}
	t.Cleanup(cli.ClearBuildTimeAgentMetadata)
	root := app.NewSchemaSourceRootCommand()
	if err := installBuildTimeAgentMetadata(repositoryRoot, root); err != nil {
		t.Fatalf("installBuildTimeAgentMetadata() error = %v", err)
	}
	if _, err := os.Stat(retired); !os.IsNotExist(err) {
		t.Fatalf("inject must not recreate schema_agent_metadata/: %v", err)
	}
}

func TestCrossPlatformCoverageGenerateSchemaCatalogResolvesBuildExactlyOnce(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := app.NewSchemaSourceRootCommand()
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
	t.Cleanup(cli.ClearBuildTimeAgentMetadata)
	if err := generateSchemaCatalogWithResolver(repositoryRoot, root, "", outputPath, "", resolver); err != nil {
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

func TestValidateDeprecatedSurfaceRejectsRetiredIdentitySource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := validateDeprecatedSurface(path)
	if err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("validateDeprecatedSurface() error = %v, want retired-input rejection", err)
	}
}

func TestValidateDeprecatedSurfaceAllowsOmittedCompatibilityFlag(t *testing.T) {
	if err := validateDeprecatedSurface(""); err != nil {
		t.Fatalf("validateDeprecatedSurface() error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateCatalogOutputIsolationProtectsEveryInputLayer(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"skills/mono/SKILL.md",
		"skills/mono/references/intent-guide.md",
		"internal/cli/param_concepts.json",
		"internal/cli/schema_parameter_mapping_ledger.go",
		"internal/cli/schema_command_exclusions.go",
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
	conceptsPath := filepath.Join(root, "internal/cli/param_concepts.json")
	for _, relative := range []string{"skills/mono/references/products"} {
		if err := os.MkdirAll(filepath.Join(root, relative), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name   string
		output string
		want   string
	}{
		{name: "param concepts", output: conceptsPath, want: "reviewed param concepts"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateCatalogOutputIsolation(root, test.output, filepath.Join(t.TempDir(), "schema_meta_index.gob"))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateCatalogOutputIsolation() error = %v, want %q", err, test.want)
			}
		})
	}
	if err := validateCatalogOutputIsolation(root, filepath.Join(root, "artifacts/schema_catalog"), filepath.Join(root, "artifacts/schema_meta_index.gob")); err != nil {
		t.Fatalf("safe artifact output rejected: %v", err)
	}
	if err := validateCatalogOutputIsolation(root, filepath.Join(t.TempDir(), "schema_catalog"), filepath.Join(t.TempDir(), "schema_meta_index.gob")); err != nil {
		t.Fatalf("external temporary output rejected: %v", err)
	}
	if err := validateCatalogOutputIsolation(root, filepath.Join(root, "skills/mono/overwrite.json"), filepath.Join(t.TempDir(), "schema_meta_index.gob")); err == nil || !strings.Contains(err.Error(), "not a canonical generated delivery target") {
		t.Fatalf("non-canonical repository output error = %v", err)
	}
}
