// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/agentmetadata"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageInstallBuildTimeAgentMetadataNilRoot(t *testing.T) {
	t.Cleanup(cli.ClearBuildTimeAgentMetadata)
	if err := installBuildTimeAgentMetadata(".", nil); err == nil {
		t.Fatal("expected nil command root failure")
	}
}

func TestCrossPlatformCoverageInstallBuildTimeAgentMetadataSuccess(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cli.ClearBuildTimeAgentMetadata)
	root := app.NewSchemaSourceRootCommand()
	if err := installBuildTimeAgentMetadata(repositoryRoot, root); err != nil {
		t.Fatalf("installBuildTimeAgentMetadata() error = %v", err)
	}
}

func TestCrossPlatformCoverageResolveSchemaMetaIndexPathDefault(t *testing.T) {
	got := resolveSchemaMetaIndexPath("internal/cli/schema_catalog", "")
	want := filepath.Join("internal/cli", "schema_meta_index.gob")
	if got != want {
		t.Fatalf("default meta index path = %q, want %q", got, want)
	}
	absCatalog := filepath.Join(t.TempDir(), "catalog")
	if got := resolveCatalogRootPath(filepath.Dir(absCatalog), absCatalog); got != absCatalog {
		t.Fatalf("absolute catalog path = %q, want %q", got, absCatalog)
	}
}

func TestCrossPlatformCoverageWriteSchemaMetaIndexFailureEdges(t *testing.T) {
	originalMkdir := makeCatalogDirectory
	originalWrite := writeCatalogFile
	t.Cleanup(func() {
		makeCatalogDirectory = originalMkdir
		writeCatalogFile = originalWrite
	})
	snapshot := cli.SchemaCatalogSnapshot{Version: 1, SourceHash: "hash", Catalog: map[string]any{}, Tools: map[string]map[string]any{}}

	makeCatalogDirectory = func(string, os.FileMode) error { return errors.New("mkdir") }
	if err := writeSchemaMetaIndex(snapshot, "meta.json"); err == nil {
		t.Fatal("expected mkdir failure")
	}
	makeCatalogDirectory = originalMkdir
	writeCatalogFile = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := writeSchemaMetaIndex(snapshot, "meta.json"); err == nil {
		t.Fatal("expected write failure")
	}
}

func TestCrossPlatformCoverageWriteSchemaMetaIndexBuildValidateEncodeFailures(t *testing.T) {
	originalBuild := buildCatalogMetaIndex
	originalValidate := validateCatalogMetaIndex
	originalEncode := encodeCatalogMetaIndex
	originalMkdir := makeCatalogDirectory
	originalWrite := writeCatalogFile
	t.Cleanup(func() {
		buildCatalogMetaIndex = originalBuild
		validateCatalogMetaIndex = originalValidate
		encodeCatalogMetaIndex = originalEncode
		makeCatalogDirectory = originalMkdir
		writeCatalogFile = originalWrite
	})
	snapshot := cli.SchemaCatalogSnapshot{Version: 1, SourceHash: "hash", Catalog: map[string]any{}, Tools: map[string]map[string]any{}}

	buildCatalogMetaIndex = func(cli.SchemaCatalogSnapshot) (cli.SchemaMetaIndexSnapshot, error) {
		return cli.SchemaMetaIndexSnapshot{}, errors.New("build")
	}
	if err := writeSchemaMetaIndex(snapshot, "meta.json"); err == nil || !strings.Contains(err.Error(), "build schema meta index") {
		t.Fatalf("build failure = %v", err)
	}

	buildCatalogMetaIndex = originalBuild
	validateCatalogMetaIndex = func(cli.SchemaMetaIndexSnapshot, cli.SchemaCatalogSnapshot) error {
		return errors.New("validate")
	}
	if err := writeSchemaMetaIndex(snapshot, "meta.json"); err == nil || !strings.Contains(err.Error(), "validate schema meta index") {
		t.Fatalf("validate failure = %v", err)
	}

	validateCatalogMetaIndex = originalValidate
	encodeCatalogMetaIndex = func(cli.SchemaMetaIndexSnapshot) ([]byte, error) {
		return nil, errors.New("encode")
	}
	if err := writeSchemaMetaIndex(snapshot, "meta.json"); err == nil {
		t.Fatal("expected encode failure")
	}
}

func TestCrossPlatformCoverageValidateCatalogOutputIsolationFailure(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCatalogOutputIsolation(repositoryRoot, "skills/mono/SKILL.md", "internal/cli/schema_meta_index.gob"); err == nil {
		t.Fatal("expected output/input overlap failure")
	}
}

func TestCrossPlatformCoverageGenerateSchemaCatalogInstallFailure(t *testing.T) {
	originalInstall := installCatalogAgentMetadata
	t.Cleanup(func() { installCatalogAgentMetadata = originalInstall })
	installCatalogAgentMetadata = func(string, *cobra.Command) error {
		return errors.New("install failed")
	}
	root := app.NewSchemaSourceRootCommand()
	if err := generateSchemaCatalog(".", root, "", t.TempDir(), ""); err == nil || !strings.Contains(err.Error(), "install failed") {
		t.Fatalf("install failure = %v", err)
	}
}

func TestCrossPlatformCoverageInstallBuildTimeAgentMetadataEncodeAndInjectFailures(t *testing.T) {
	originalGenerate := generateCatalogAgentMetadata
	originalEncode := encodeCatalogAgentMetadata
	originalInject := injectCatalogAgentMetadata
	t.Cleanup(func() {
		generateCatalogAgentMetadata = originalGenerate
		encodeCatalogAgentMetadata = originalEncode
		injectCatalogAgentMetadata = originalInject
	})
	root := app.NewSchemaSourceRootCommand()
	generateCatalogAgentMetadata = func(string, *cobra.Command, agentmetadata.Options) (agentmetadata.File, agentmetadata.Stats, agentmetadata.RegistryProjection, error) {
		return agentmetadata.File{}, agentmetadata.Stats{}, agentmetadata.RegistryProjection{}, nil
	}
	encodeCatalogAgentMetadata = func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	if err := installBuildTimeAgentMetadata(".", root); err == nil || !strings.Contains(err.Error(), "encode in-memory Agent metadata") {
		t.Fatalf("marshal failure = %v", err)
	}

	encodeCatalogAgentMetadata = json.Marshal
	injectCatalogAgentMetadata = func([]byte) error { return errors.New("inject failed") }
	if err := installBuildTimeAgentMetadata(".", root); err == nil || !strings.Contains(err.Error(), "inject failed") {
		t.Fatalf("inject failure = %v", err)
	}
}

func TestCrossPlatformCoverageValidateDeprecatedSurfaceReadFailure(t *testing.T) {
	if err := validateDeprecatedSurface("missing-registry.json"); err == nil {
		t.Fatal("expected read failure")
	}
}
