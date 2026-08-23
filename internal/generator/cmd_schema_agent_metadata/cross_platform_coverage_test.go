// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/agentmetadata"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/outputguard"
)

func TestCrossPlatformCoverageMetadataMainWithHintsProtectedInput(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	hintsDir := filepath.Join(t.TempDir(), "hints")
	if err := os.MkdirAll(hintsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldArgs, oldFlags := os.Args, flag.CommandLine
	originalExit := exitMetadataProcess
	t.Cleanup(func() {
		os.Args, flag.CommandLine = oldArgs, oldFlags
		exitMetadataProcess = originalExit
	})
	exitMetadataProcess = func(int) { panic("exit") }

	flag.CommandLine = flag.NewFlagSet("schema-agent-metadata-hints", flag.ContinueOnError)
	os.Args = []string{"cmd_schema_agent_metadata", "-root", repositoryRoot, "-hints", hintsDir}
	defer func() {
		if r := recover(); r != "exit" {
			t.Fatalf("expected main to fail-closed on retired -hints, recover=%v", r)
		}
	}()
	main()
}

func TestCrossPlatformCoverageMetadataMainWithOutputDir(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "agent-metadata-out")
	oldArgs, oldFlags := os.Args, flag.CommandLine
	originalExit := exitMetadataProcess
	t.Cleanup(func() {
		os.Args, flag.CommandLine = oldArgs, oldFlags
		exitMetadataProcess = originalExit
	})
	exitMetadataProcess = func(code int) {
		if code != 0 {
			panic(fmt.Sprintf("exit:%d", code))
		}
	}
	flag.CommandLine = flag.NewFlagSet("schema-agent-metadata-output-dir", flag.ContinueOnError)
	os.Args = []string{"cmd_schema_agent_metadata", "-root", repositoryRoot, "-output-dir", outputDir}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("main failed with output-dir under temp: %v", r)
		}
	}()
	main()
}

func TestCrossPlatformCoverageMetadataMainValidatesInMemoryWithoutDiskOutput(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	oldArgs, oldFlags := os.Args, flag.CommandLine
	t.Cleanup(func() {
		os.Args, flag.CommandLine = oldArgs, oldFlags
	})
	flag.CommandLine = flag.NewFlagSet("schema-agent-metadata-memory", flag.ContinueOnError)
	os.Args = []string{"cmd_schema_agent_metadata", "-root", repositoryRoot}
	main()
}

func TestCrossPlatformCoverageInterfaceAppliedSummariesNonNil(t *testing.T) {
	if got := interfaceAppliedSummaries(agentmetadata.Stats{
		InterfaceMetadata: &agentmetadata.InterfaceMetadataAudit{AppliedSummaries: 3},
	}); got != 3 {
		t.Fatalf("interfaceAppliedSummaries = %d, want 3", got)
	}
}

func TestCrossPlatformCoverageValidateAgentMetadataOutputIsolationWrapper(t *testing.T) {
	root := t.TempDir()
	hintsDir := filepath.Join(root, "hints")
	if err := os.MkdirAll(hintsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateAgentMetadataOutputIsolation(
		root,
		[]outputguard.Input{{Name: "structured hint input directory", Path: "hints"}},
		"",
		filepath.Join(root, "out"),
		"",
	); err != nil {
		t.Fatalf("validateAgentMetadataOutputIsolation() error = %v", err)
	}
	if err := validateAgentMetadataOutputIsolation(root, nil, "", filepath.Join(root, "out"), ""); err != nil {
		t.Fatalf("validateAgentMetadataOutputIsolation(nil inputs) error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateAgentMetadataOutputAllowlistWrapper(t *testing.T) {
	root := t.TempDir()
	if err := validateAgentMetadataOutputAllowlist(root, "", filepath.Join(root, "internal/cli/schema_agent_metadata"), ""); err != nil {
		t.Fatalf("validateAgentMetadataOutputAllowlist() error = %v", err)
	}
}
