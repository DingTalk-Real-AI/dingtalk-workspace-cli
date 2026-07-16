// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/agentmetadata"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/outputguard"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageMetadataMainReportsEveryStageFailure(t *testing.T) {
	originalArgs, originalFlags := os.Args, flag.CommandLine
	originalIsolation := validateMetadataIsolation
	originalAllowlist := validateMetadataAllowlist
	originalRegistryFile := validateMetadataRegistryFile
	originalLoad := loadMetadataRegistryProjection
	originalSelection := validateMetadataSelection
	originalGenerate := generateAgentMetadata
	originalDir := writeMetadataDirectoryOutput
	originalFile := writeMetadataFileOutput
	originalAudit := writeMetadataAuditOutput
	originalExit := exitMetadataProcess
	t.Cleanup(func() {
		os.Args, flag.CommandLine = originalArgs, originalFlags
		validateMetadataIsolation = originalIsolation
		validateMetadataAllowlist = originalAllowlist
		validateMetadataRegistryFile = originalRegistryFile
		loadMetadataRegistryProjection = originalLoad
		validateMetadataSelection = originalSelection
		generateAgentMetadata = originalGenerate
		writeMetadataDirectoryOutput = originalDir
		writeMetadataFileOutput = originalFile
		writeMetadataAuditOutput = originalAudit
		exitMetadataProcess = originalExit
	})
	exitMetadataProcess = func(int) { panic("exit") }

	reset := func() {
		validateMetadataIsolation = func(string, []outputguard.Input, string, string, string) error { return nil }
		validateMetadataAllowlist = func(string, string, string, string) error { return nil }
		validateMetadataRegistryFile = func(string, string) error { return nil }
		loadMetadataRegistryProjection = func(string, string, bool) (commandRegistryProjection, error) {
			return commandRegistryProjection{}, nil
		}
		validateMetadataSelection = func(string, string, commandRegistryProjection) error { return nil }
		generateAgentMetadata = func(agentmetadata.Options) (agentmetadata.File, agentmetadata.Stats, error) {
			return agentmetadata.File{}, agentmetadata.Stats{}, nil
		}
		writeMetadataDirectoryOutput = func(string, agentmetadata.File) error { return nil }
		writeMetadataFileOutput = func(string, agentmetadata.File) error { return nil }
		writeMetadataAuditOutput = func(string, agentmetadata.Audit) error { return nil }
	}
	invoke := func(args []string, setup func()) {
		reset()
		setup()
		flag.CommandLine = flag.NewFlagSet("metadata-errors", flag.ContinueOnError)
		os.Args = append([]string{"cmd_schema_agent_metadata"}, args...)
		defer func() {
			if recover() != "exit" {
				t.Fatal("main did not exit")
			}
		}()
		main()
	}
	temporaryOutput := filepath.Join(t.TempDir(), "metadata.json")
	temporaryDir := t.TempDir()

	invoke(nil, func() {
		validateMetadataIsolation = func(string, []outputguard.Input, string, string, string) error { return errors.New("isolation") }
	})
	invoke([]string{"-output", temporaryOutput}, func() {
		validateMetadataAllowlist = func(string, string, string, string) error { return errors.New("allowlist") }
	})
	invoke([]string{"-output", temporaryOutput, "-surface", "legacy.json"}, func() {
		validateMetadataRegistryFile = func(string, string) error { return errors.New("legacy") }
	})
	invoke([]string{"-output", temporaryOutput, "-validate-surface=false"}, func() {
		loadMetadataRegistryProjection = func(string, string, bool) (commandRegistryProjection, error) {
			return commandRegistryProjection{}, errors.New("registry disabled")
		}
	})
	invoke([]string{"-output", temporaryOutput}, func() {
		loadMetadataRegistryProjection = func(string, string, bool) (commandRegistryProjection, error) {
			return commandRegistryProjection{}, errors.New("registry")
		}
	})
	invoke([]string{"-output", temporaryOutput}, func() {
		validateMetadataSelection = func(string, string, commandRegistryProjection) error { return errors.New("selection") }
	})
	invoke([]string{"-output", temporaryOutput}, func() {
		generateAgentMetadata = func(agentmetadata.Options) (agentmetadata.File, agentmetadata.Stats, error) {
			return agentmetadata.File{}, agentmetadata.Stats{}, errors.New("generate")
		}
	})
	invoke([]string{"-output-dir", temporaryDir}, func() {
		writeMetadataDirectoryOutput = func(string, agentmetadata.File) error { return errors.New("directory") }
	})
	invoke([]string{"-output", temporaryOutput}, func() {
		writeMetadataFileOutput = func(string, agentmetadata.File) error { return errors.New("file") }
	})
	invoke([]string{"-output", temporaryOutput, "-audit-output", filepath.Join(t.TempDir(), "audit.json")}, func() {
		writeMetadataAuditOutput = func(string, agentmetadata.Audit) error { return errors.New("audit") }
	})
}

func TestCrossPlatformCoverageMetadataWriterFailureEdges(t *testing.T) {
	originalMkdir := makeMetadataDirectory
	originalRead := readMetadataDirectory
	originalRemove := removeMetadataFile
	originalWrite := writeMetadataFileBytes
	originalJSON := writeMetadataJSON
	t.Cleanup(func() {
		makeMetadataDirectory = originalMkdir
		readMetadataDirectory = originalRead
		removeMetadataFile = originalRemove
		writeMetadataFileBytes = originalWrite
		writeMetadataJSON = originalJSON
	})
	reset := func() {
		makeMetadataDirectory = originalMkdir
		readMetadataDirectory = originalRead
		removeMetadataFile = originalRemove
		writeMetadataFileBytes = originalWrite
		writeMetadataJSON = originalJSON
	}

	if err := writeAuditFile(" ", agentmetadata.Audit{}); err != nil {
		t.Fatalf("empty audit path error = %v", err)
	}
	makeMetadataDirectory = func(string, os.FileMode) error { return errors.New("mkdir") }
	if err := writeAuditFile("audit.json", agentmetadata.Audit{}); err == nil || !strings.Contains(err.Error(), "create audit") {
		t.Fatalf("audit mkdir error = %v", err)
	}
	reset()
	writeMetadataJSON = func(string, any) error { return errors.New("json") }
	if err := writeAuditFile("audit.json", agentmetadata.Audit{}); err == nil || !strings.Contains(err.Error(), "write audit") {
		t.Fatalf("audit write error = %v", err)
	}

	reset()
	makeMetadataDirectory = func(string, os.FileMode) error { return errors.New("mkdir") }
	if err := writeMetadataFile("metadata.json", agentmetadata.File{}); err == nil || !strings.Contains(err.Error(), "create output") {
		t.Fatalf("metadata mkdir error = %v", err)
	}
	reset()
	writeMetadataFileBytes = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := writeMetadataFile("metadata.json", agentmetadata.File{}); err == nil || !strings.Contains(err.Error(), "write output") {
		t.Fatalf("metadata write error = %v", err)
	}

	reset()
	makeMetadataDirectory = func(string, os.FileMode) error { return errors.New("mkdir") }
	if err := writeMetadataDirectory("metadata", agentmetadata.File{}); err == nil || !strings.Contains(err.Error(), "create metadata") {
		t.Fatalf("directory mkdir error = %v", err)
	}
	reset()
	readMetadataDirectory = func(string) ([]os.DirEntry, error) { return nil, errors.New("read") }
	if err := writeMetadataDirectory(t.TempDir(), agentmetadata.File{}); err == nil || !strings.Contains(err.Error(), "read metadata") {
		t.Fatalf("directory read error = %v", err)
	}
	reset()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stale.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	removeMetadataFile = func(string) error { return errors.New("remove") }
	if err := writeMetadataDirectory(dir, agentmetadata.File{}); err == nil || !strings.Contains(err.Error(), "remove stale") {
		t.Fatalf("remove stale error = %v", err)
	}
	reset()
	writeMetadataJSON = func(string, any) error { return errors.New("index") }
	if err := writeMetadataDirectory(t.TempDir(), agentmetadata.File{}); err == nil || !strings.Contains(err.Error(), "index") {
		t.Fatalf("index write error = %v", err)
	}
	reset()
	writes := 0
	writeMetadataJSON = func(string, any) error {
		writes++
		if writes == 2 {
			return errors.New("domain")
		}
		return nil
	}
	if err := writeMetadataDirectory(t.TempDir(), agentmetadata.File{Tools: map[string]agentmetadata.ToolMetadata{"sample run": {}}}); err == nil || !strings.Contains(err.Error(), "domain") {
		t.Fatalf("domain write error = %v", err)
	}

	reset()
	if err := writeJSON("ignored", func() {}); err == nil || !strings.Contains(err.Error(), "encode") {
		t.Fatalf("JSON encode error = %v", err)
	}
	writeMetadataFileBytes = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := writeJSON("ignored", map[string]any{}); err == nil || !strings.Contains(err.Error(), "write") {
		t.Fatalf("JSON write error = %v", err)
	}
	if firstPathToken(" ") != "" {
		t.Fatal("empty first path token should be empty")
	}
	if interfaceAppliedSummaries(agentmetadata.Stats{}) != 0 {
		t.Fatal("nil interface audit should report zero")
	}

	reset()
	successDir := t.TempDir()
	if err := writeAuditFile(filepath.Join(successDir, "audit.json"), agentmetadata.Audit{}); err != nil {
		t.Fatalf("successful audit write error = %v", err)
	}
	if err := writeMetadataFile(filepath.Join(successDir, "metadata.json"), agentmetadata.File{}); err != nil {
		t.Fatalf("successful metadata write error = %v", err)
	}
	if err := writeMetadataDirectory(filepath.Join(successDir, "split"), agentmetadata.File{
		Tools: map[string]agentmetadata.ToolMetadata{" ": {}},
	}); err != nil {
		t.Fatalf("empty-domain metadata write error = %v", err)
	}
}

func TestCrossPlatformCoverageMetadataRegistryAndSelectionFailureEdges(t *testing.T) {
	originalRoot := newMetadataRoot
	originalBuild := buildEffectiveMetadata
	originalBind := bindEffectiveMetadata
	originalLoadHints := loadSelectionMetadataHints
	originalValidateSet := validateSelectionMetadataSet
	originalExamples := validateSelectionExamples
	originalContract := validateSelectionContract
	t.Cleanup(func() {
		newMetadataRoot = originalRoot
		buildEffectiveMetadata = originalBuild
		bindEffectiveMetadata = originalBind
		loadSelectionMetadataHints = originalLoadHints
		validateSelectionMetadataSet = originalValidateSet
		validateSelectionExamples = originalExamples
		validateSelectionContract = originalContract
	})

	if err := validateCommandRegistryFile(".", " "); err != nil {
		t.Fatalf("empty registry path error = %v", err)
	}
	if err := validateCommandRegistryFile(t.TempDir(), "missing.json"); err == nil {
		t.Fatal("missing registry file should fail")
	}
	newMetadataRoot = func(...context.Context) *cobra.Command { return &cobra.Command{Use: "dws"} }
	buildEffectiveMetadata = func(*cobra.Command) (cli.EffectiveCommandRegistry, error) {
		return cli.EffectiveCommandRegistry{}, errors.New("build")
	}
	if _, err := loadEffectiveCommandRegistryProjection(".", "", true); err == nil || !strings.Contains(err.Error(), "build effective") {
		t.Fatalf("build projection error = %v", err)
	}
	buildEffectiveMetadata = func(*cobra.Command) (cli.EffectiveCommandRegistry, error) {
		return cli.EffectiveCommandRegistry{}, nil
	}
	bindEffectiveMetadata = func(*cobra.Command, cli.EffectiveCommandRegistry) (cli.BoundCommandRegistry, error) {
		return cli.BoundCommandRegistry{}, errors.New("bind")
	}
	if _, err := loadEffectiveCommandRegistryProjection(".", "", true); err == nil || !strings.Contains(err.Error(), "bind effective") {
		t.Fatalf("bind projection error = %v", err)
	}

	effective := cli.EffectiveCommandRegistry{Commands: []cli.CommandSpec{
		{CanonicalPath: "internal.run", PrimaryCLIPath: "internal run", Visibility: cli.SchemaVisibilityInternal},
		{CanonicalPath: ".run", PrimaryCLIPath: "sample run", Aliases: []string{" ", "sample old"}, Visibility: cli.SchemaVisibilityPublic},
	}}
	projection := projectEffectiveCommandRegistry(effective)
	if projection.ToolCount != 1 || projection.ToolPaths["sample old"] != "sample run" || len(projection.ProductIDs) != 0 {
		t.Fatalf("edge projection = %#v", projection)
	}

	root := t.TempDir()
	selection := filepath.Join(root, "hints", "selection")
	metadata := filepath.Join(root, "hints", "metadata")
	if err := os.MkdirAll(selection, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(metadata, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := commandRegistryProjection{CanonicalToolPaths: map[string]string{"sample.run": "sample run"}}
	loadSelectionMetadataHints = func(fs.FS) (cli.ManualAgentHintSet, error) {
		return cli.ManualAgentHintSet{}, errors.New("load")
	}
	if err := validateSelectionHintInput(root, "hints", registry); err == nil || !strings.Contains(err.Error(), "load selection") {
		t.Fatalf("selection load error = %v", err)
	}
	loadSelectionMetadataHints = func(fs.FS) (cli.ManualAgentHintSet, error) { return cli.ManualAgentHintSet{}, nil }
	validateSelectionMetadataSet = func(cli.ManualAgentHintSet, map[string]bool, map[string]bool) error { return errors.New("set") }
	if err := validateSelectionHintInput(root, "hints", registry); err == nil || !strings.Contains(err.Error(), "validate selection Agent hints") {
		t.Fatalf("selection set error = %v", err)
	}
	validateSelectionMetadataSet = func(cli.ManualAgentHintSet, map[string]bool, map[string]bool) error { return nil }
	validateSelectionExamples = func(cli.BoundCommandRegistry, cli.ManualAgentHintSet) error { return errors.New("examples") }
	if err := validateSelectionHintInput(root, "hints", registry); err == nil || !strings.Contains(err.Error(), "examples") {
		t.Fatalf("selection examples error = %v", err)
	}
	validateSelectionExamples = func(cli.BoundCommandRegistry, cli.ManualAgentHintSet) error { return nil }
	validateSelectionContract = func(cli.BoundCommandRegistry, cli.ManualAgentHintSet) (cli.ManualAgentSelectionReport, error) {
		return cli.ManualAgentSelectionReport{}, errors.New("contract")
	}
	if err := validateSelectionHintInput(root, "hints", registry); err == nil || !strings.Contains(err.Error(), "selection contract") {
		t.Fatalf("selection contract error = %v", err)
	}

	abs := filepath.Join(t.TempDir(), "absolute")
	if resolveRootPath("ignored", abs) != abs {
		t.Fatal("absolute root path should be preserved")
	}
}
