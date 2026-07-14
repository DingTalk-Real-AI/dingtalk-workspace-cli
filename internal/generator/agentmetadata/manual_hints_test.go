// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

func TestGenerateUsesReviewedSelectionAsSingleDeliverySource(t *testing.T) {
	root := t.TempDir()
	writeSelectionFixture(t, root, true, `["dws sample item search --query value"]`)
	writeManualFixtureFile(t, root, "skills/mono/SKILL.md", "# Skill\n")
	writeManualFixtureFile(t, root, "skills/mono/references/intent-guide.md", "# Intent\n")
	writeManualFixtureFile(t, root, "skills/mono/references/products/sample.md", "# Sample\n")
	writeManualFixtureFile(t, root, "internal/cli/schema_hints/imported/legacy.json", `{
  "version":1,
  "source":{"kind":"imported","name":"legacy"},
  "tools":{"sample.search_items":{"agent_summary":"legacy A","use_when":["legacy A"],"avoid_when":["legacy A"],"examples":["dws sample item search --legacy-a"]}}
}`)

	selectionPath := filepath.Join(root, "internal/cli/schema_hints/selection/sample.json")
	before, err := os.ReadFile(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	file, _, err := Generate(selectionFixtureOptions(root))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	after, err := os.ReadFile(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("Generate modified the reviewed selection input")
	}

	tool := file.Tools["sample item search"]
	if tool.AgentSummary != "Search existing sample items" || len(tool.UseWhen) != 1 || tool.UseWhen[0] != "An existing item must be found" {
		t.Fatalf("selection was not delivered: %#v", tool)
	}
	product := file.Products["sample"]
	for _, field := range []string{"agent_summary", "use_when", "avoid_when"} {
		provenance := product.FieldProvenance[field]
		if provenance.Precedence != selectionPrecedenceReviewedExplicit || selectedCandidateCount(provenance.Candidates) != 1 {
			t.Fatalf("product %s provenance = %#v", field, provenance)
		}
		if !strings.Contains(provenance.Source, "/selection/") {
			t.Fatalf("product %s source = %q, want selection/", field, provenance.Source)
		}
	}
	for _, field := range []string{"agent_summary", "use_when", "avoid_when", "examples"} {
		provenance := tool.FieldProvenance[field]
		if provenance.Precedence != selectionPrecedenceReviewedExplicit || selectedCandidateCount(provenance.Candidates) != 1 {
			t.Fatalf("%s provenance = %#v", field, provenance)
		}
		for _, candidate := range provenance.Candidates {
			if candidate.Precedence != selectionPrecedenceReviewedExplicit {
				t.Fatalf("legacy prose remained a semantic candidate for %s: %#v", field, provenance.Candidates)
			}
		}
	}
}

func TestGenerateRejectsMaxExamplesThatWouldTruncateReviewedSelection(t *testing.T) {
	root := t.TempDir()
	writeSelectionFixture(t, root, true, `["dws sample item search --query value","dws sample item search --query other"]`)
	writeManualFixtureFile(t, root, "skills/mono/SKILL.md", "# Skill\n")
	writeManualFixtureFile(t, root, "skills/mono/references/intent-guide.md", "# Intent\n")
	if err := os.MkdirAll(filepath.Join(root, "skills/mono/references/products"), 0o755); err != nil {
		t.Fatal(err)
	}
	opts := selectionFixtureOptions(root)
	opts.MaxExamples = 1
	_, _, err := Generate(opts)
	if err == nil || !strings.Contains(err.Error(), "exceeding max-examples=1") {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestGenerateRejectsIncompleteSelectionCoverage(t *testing.T) {
	root := t.TempDir()
	writeSelectionFixture(t, root, false, `[]`)
	writeManualFixtureFile(t, root, "skills/mono/SKILL.md", "# Skill\n")
	writeManualFixtureFile(t, root, "skills/mono/references/intent-guide.md", "# Intent\n")
	if err := os.MkdirAll(filepath.Join(root, "skills/mono/references/products"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := Generate(selectionFixtureOptions(root))
	if err == nil || !strings.Contains(err.Error(), "missing=[sample.search_items]") {
		t.Fatalf("coverage error = %v", err)
	}
}

func TestGenerateRequiresAgentHintDirectory(t *testing.T) {
	_, _, err := Generate(Options{})
	if err == nil || !strings.Contains(err.Error(), "Agent hint directory is required") {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestGenerateRequiresCompleteEffectiveCommandRegistryProjection(t *testing.T) {
	_, _, err := Generate(Options{HintsDir: "internal/cli/schema_hints"})
	if err == nil || !strings.Contains(err.Error(), "complete Effective CommandRegistry projection is required") {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestGenerateValidatesSelectionExamplesAgainstBoundCobra(t *testing.T) {
	root := t.TempDir()
	writeSelectionFixture(t, root, true, `["dws sample item search --invented value"]`)
	writeManualFixtureFile(t, root, "skills/mono/SKILL.md", "# Skill\n")
	writeManualFixtureFile(t, root, "skills/mono/references/intent-guide.md", "# Intent\n")
	if err := os.MkdirAll(filepath.Join(root, "skills/mono/references/products"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := Generate(selectionFixtureOptions(root))
	if err == nil || !strings.Contains(err.Error(), "unknown flag --invented") {
		t.Fatalf("Generate() error = %v", err)
	}
}

func selectionFixtureOptions(root string) Options {
	leaf := &cobra.Command{Use: "search", Run: func(*cobra.Command, []string) {}}
	leaf.Flags().String("query", "", "query")
	boundSpec := cli.BoundCommandSpec{
		CommandSpec:    cli.CommandSpec{CanonicalPath: "sample.search_items", PrimaryCLIPath: "sample item search"},
		PrimaryCommand: leaf,
	}
	return Options{
		Root:               root,
		SkillPath:          "skills/mono/SKILL.md",
		ProductsDir:        "skills/mono/references/products",
		IntentGuidePath:    "skills/mono/references/intent-guide.md",
		HintsDir:           "internal/cli/schema_hints",
		ToolPaths:          map[string]string{"sample.search_items": "sample item search", "sample item search": "sample item search"},
		CanonicalToolPaths: map[string]string{"sample.search_items": "sample item search"},
		BoundCommands: cli.BoundCommandRegistry{
			Commands:    []cli.BoundCommandSpec{boundSpec},
			ByCanonical: map[string]cli.BoundCommandSpec{"sample.search_items": boundSpec},
			ByCLIPath:   map[string]cli.BoundCommandSpec{"sample item search": boundSpec},
		},
		ProductIDs:               map[string]bool{"sample": true},
		SurfaceToolCount:         1,
		MaxExamples:              2,
		InterfaceMetadataPath:    "",
		MaxInterfaceSummaryRunes: 120,
	}
}

func writeSelectionFixture(t *testing.T, root string, includeTool bool, examplesJSON string) {
	t.Helper()
	writeManualFixtureFile(t, root, "internal/cli/schema_hints/index.json", `{
  "version":1,
  "format":"dws-agent-hint-index",
  "source":{"kind":"explicit","name":"fixture"},
  "metadata":{"sample":"metadata/sample.json"},
  "selection":{"sample":"selection/sample.json"}
}`)
	writeManualFixtureFile(t, root, "internal/cli/schema_hints/metadata/sample.json", `{
  "version":1,
  "source":{"kind":"explicit","name":"dws-tool-metadata/sample","reviewed":true},
  "tools":{
    "sample.search_items":{
      "effect":"read",
      "risk":"low",
      "confirmation":"not_required",
      "runtime_gate":"none",
      "reviewed":true,
      "review_reason":"fixture metadata"
    }
  }
}`)
	tool := ""
	if includeTool {
		tool = `"sample.search_items":{
      "agent_summary":"Search existing sample items",
      "use_when":["An existing item must be found"],
      "avoid_when":["A new item must be created"],
      "examples":` + examplesJSON + `,
      "reviewed":true,
      "review_reason":"Reviewed selection guidance",
      "source_refs":["dws sample item search --help"]
    }`
	}
	writeManualFixtureFile(t, root, "internal/cli/schema_hints/selection/sample.json", `{
  "version":1,
  "source":{"kind":"explicit","name":"dws-agent-selection/sample","reviewed":true},
  "products":{
    "sample":{
      "agent_summary":"Manage samples",
      "use_when":["The target is a sample"],
      "avoid_when":["The target is not a sample"],
      "reviewed":true,
      "review_reason":"Reviewed product routing",
      "source_refs":["sample.md"]
    }
  },
  "tools":{`+tool+`}
}`)
}

func writeManualFixtureFile(t *testing.T, root, relative, body string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
