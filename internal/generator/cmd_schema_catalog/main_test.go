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
	os.Args = []string{"cmd_schema_catalog", "-root", repositoryRoot, "-output", filepath.Join(t.TempDir(), "schema_catalog.json")}
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
	invoke("-root", repositoryRoot, "-output", filepath.Join(repositoryRoot, "internal/cli/schema_command_registry.json"))
	validateCatalogParameterBindings = func() error { return errors.New("bindings") }
	invoke("-root", repositoryRoot, "-output", filepath.Join(t.TempDir(), "catalog.json"))
}

func TestCrossPlatformCoverageGenerateSchemaCatalogFailureEdges(t *testing.T) {
	originalValidate := validateCatalogParameterBindings
	originalSnapshot := buildCatalogSnapshot
	originalMkdir := makeCatalogDirectory
	originalWrite := writeCatalogFile
	t.Cleanup(func() {
		validateCatalogParameterBindings = originalValidate
		buildCatalogSnapshot = originalSnapshot
		makeCatalogDirectory = originalMkdir
		writeCatalogFile = originalWrite
	})
	root := &cobra.Command{Use: "dws"}
	resolver := func(*cobra.Command) (cli.ResolvedSchemaBuild, error) { return cli.ResolvedSchemaBuild{}, nil }
	output := filepath.Join(t.TempDir(), "catalog.json")

	if err := generateSchemaCatalogWithResolver(nil, "", output, resolver); err == nil || !strings.Contains(err.Error(), "root is nil") {
		t.Fatalf("nil root error = %v", err)
	}
	if err := generateSchemaCatalogWithResolver(root, "", output, nil); err == nil || !strings.Contains(err.Error(), "resolver is nil") {
		t.Fatalf("nil resolver error = %v", err)
	}
	if err := generateSchemaCatalogWithResolver(root, filepath.Join(t.TempDir(), "missing.json"), output, resolver); err == nil || !strings.Contains(err.Error(), "read deprecated") {
		t.Fatalf("surface read error = %v", err)
	}
	validateCatalogParameterBindings = func() error { return errors.New("bindings") }
	if err := generateSchemaCatalogWithResolver(root, "", output, resolver); err == nil || !strings.Contains(err.Error(), "parameter binding") {
		t.Fatalf("binding error = %v", err)
	}
	validateCatalogParameterBindings = func() error { return nil }
	if err := generateSchemaCatalogWithResolver(root, "", output, func(*cobra.Command) (cli.ResolvedSchemaBuild, error) {
		return cli.ResolvedSchemaBuild{}, errors.New("resolve")
	}); err == nil || !strings.Contains(err.Error(), "resolve final") {
		t.Fatalf("resolver error = %v", err)
	}
	buildCatalogSnapshot = func(cli.ResolvedSchemaBuild, cli.SchemaCatalogBuildOptions) (cli.SchemaCatalogSnapshot, error) {
		return cli.SchemaCatalogSnapshot{}, errors.New("snapshot")
	}
	if err := generateSchemaCatalogWithResolver(root, "", output, resolver); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("snapshot error = %v", err)
	}
	buildCatalogSnapshot = func(cli.ResolvedSchemaBuild, cli.SchemaCatalogBuildOptions) (cli.SchemaCatalogSnapshot, error) {
		return cli.SchemaCatalogSnapshot{}, nil
	}
	makeCatalogDirectory = func(string, os.FileMode) error { return errors.New("mkdir") }
	if err := generateSchemaCatalogWithResolver(root, "", output, resolver); err == nil || !strings.Contains(err.Error(), "create output") {
		t.Fatalf("mkdir error = %v", err)
	}
	makeCatalogDirectory = func(string, os.FileMode) error { return nil }
	writeCatalogFile = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := generateSchemaCatalogWithResolver(root, "", output, resolver); err == nil || !strings.Contains(err.Error(), "write catalog") {
		t.Fatalf("write error = %v", err)
	}

	if got := resolveCatalogRootPath("root", "relative.json"); got != filepath.Join("root", "relative.json") {
		t.Fatalf("resolved path = %q", got)
	}
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	surface := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(surface, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateCatalogOutputIsolation(repositoryRoot, filepath.Join(t.TempDir(), "catalog.json"), surface); err != nil {
		t.Fatalf("surface isolation input rejected safe output: %v", err)
	}
}

func TestCrossPlatformCoverageGenerateSchemaCatalogResolvesBuildExactlyOnce(t *testing.T) {
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
	outputPath := filepath.Join(t.TempDir(), "schema_catalog.json")
	if err := generateSchemaCatalogWithResolver(root, "", outputPath, resolver); err != nil {
		t.Fatalf("generateSchemaCatalogWithResolver() error = %v", err)
	}
	if resolveCalls != 1 {
		t.Fatalf("Schema build resolver calls = %d, want exactly 1", resolveCalls)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated temporary Catalog: %v", err)
	}
	var snapshot cli.SchemaCatalogSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("decode generated temporary Catalog: %v", err)
	}
	if snapshot.SurfaceHash != resolvedRegistryHash {
		t.Fatalf("snapshot Registry hash = %q, want once-resolved hash %q", snapshot.SurfaceHash, resolvedRegistryHash)
	}
}

func TestCrossPlatformCoverageValidateDeprecatedSurfaceAcceptsEmbeddedRegistrySource(t *testing.T) {
	path := filepath.Join("..", "..", "cli", "schema_command_registry.json")
	if err := validateDeprecatedSurface(path); err != nil {
		t.Fatalf("validateDeprecatedSurface() error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateDeprecatedSurfaceRejectsDifferentIdentitySource(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "cli", "schema_command_registry.json")
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	altered := strings.Replace(string(data), `"canonical_path": "aisearch.search_enterprise_behavior"`, `"canonical_path": "aisearch.not_reviewed"`, 1)
	if altered == string(data) {
		t.Fatal("test fixture did not contain expected canonical path")
	}
	path := filepath.Join(t.TempDir(), "different-registry.json")
	if err := os.WriteFile(path, []byte(altered), 0o600); err != nil {
		t.Fatal(err)
	}

	err = validateDeprecatedSurface(path)
	if err == nil || !strings.Contains(err.Error(), "disagrees with the embedded reviewed registry") {
		t.Fatalf("validateDeprecatedSurface() error = %v, want registry disagreement", err)
	}
}

func TestCrossPlatformCoverageValidateDeprecatedSurfaceAllowsOmittedCompatibilityFlag(t *testing.T) {
	if err := validateDeprecatedSurface(""); err != nil {
		t.Fatalf("validateDeprecatedSurface() error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateCatalogOutputIsolationProtectsEveryInputLayer(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"skills/mono/SKILL.md",
		"skills/mono/references/intent-guide.md",
		"internal/cli/schema_command_registry.json",
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
		{name: "registry", output: filepath.Join(root, "internal/cli/schema_command_registry.json"), want: "CommandRegistry"},
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
	if err := validateCatalogOutputIsolation(root, filepath.Join(root, "internal/cli/schema_catalog.json"), ""); err != nil {
		t.Fatalf("safe output rejected: %v", err)
	}
	if err := validateCatalogOutputIsolation(root, filepath.Join(t.TempDir(), "schema_catalog.json"), ""); err != nil {
		t.Fatalf("external temporary output rejected: %v", err)
	}
	if err := validateCatalogOutputIsolation(root, filepath.Join(root, "skills/mono/overwrite.json"), ""); err == nil || !strings.Contains(err.Error(), "not a canonical generated delivery target") {
		t.Fatalf("non-canonical repository output error = %v", err)
	}
}
