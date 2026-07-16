// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/agentmetadata"
)

func TestCrossPlatformCoverageGenerateProductionAgentMetadataPipeline(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := app.NewRootCommand()
	if _, err := cli.ApplyEmbeddedManualSchemaHints(root); err != nil {
		t.Fatal(err)
	}
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatal(err)
	}
	toolPaths := make(map[string]string, len(effective.ByCLIPath)+len(effective.ByCanonical))
	canonicalToolPaths := make(map[string]string, len(effective.ByCanonical))
	productIDs := map[string]bool{}
	for _, command := range effective.Commands {
		if command.Visibility != cli.SchemaVisibilityPublic {
			continue
		}
		toolPaths[command.PrimaryCLIPath] = command.PrimaryCLIPath
		toolPaths[command.CanonicalPath] = command.PrimaryCLIPath
		canonicalToolPaths[command.CanonicalPath] = command.PrimaryCLIPath
		productIDs[strings.SplitN(command.CanonicalPath, ".", 2)[0]] = true
		for _, alias := range command.Aliases {
			toolPaths[alias] = command.PrimaryCLIPath
		}
	}
	metadata, stats, err := agentmetadata.Generate(agentmetadata.Options{
		Root:                     repositoryRoot,
		SkillPath:                "skills/mono/SKILL.md",
		ProductsDir:              "skills/mono/references/products",
		IntentGuidePath:          "skills/mono/references/intent-guide.md",
		HintsDir:                 "internal/cli/schema_hints",
		InterfaceMetadataPath:    "internal/cli/schema_mcp_metadata.json",
		MaxExamples:              2,
		MaxInterfaceSummaryRunes: 120,
		ToolPaths:                toolPaths,
		CanonicalToolPaths:       canonicalToolPaths,
		BoundCommands:            bound,
		ProductIDs:               productIDs,
		SurfaceHash:              effective.SourceHash(),
		SurfaceToolCount:         len(canonicalToolPaths),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(metadata.Tools) != len(canonicalToolPaths) || stats.Tools == 0 {
		t.Fatalf("generated metadata mismatch: tools=%d registry=%d stats=%d", len(metadata.Tools), len(canonicalToolPaths), stats.Tools)
	}
	if audit := agentmetadata.BuildAudit(metadata, stats); audit.SourceHash == "" {
		t.Fatal("generated audit has an empty source hash")
	}
}
