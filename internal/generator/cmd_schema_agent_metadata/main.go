// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/agentmetadata"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/outputguard"
)

func main() {
	var root string
	var skillPath string
	var productsDir string
	var intentGuidePath string
	var hintsDir string
	var interfaceMetadataPath string
	var outputPath string
	var outputDir string
	var auditOutputPath string
	var registryPath string
	var legacySurfacePath string
	var maxExamples int
	var maxInterfaceSummaryRunes int
	var validateRegistry bool
	var legacyValidateSurface bool
	flag.StringVar(&root, "root", ".", "Repository root")
	flag.StringVar(&skillPath, "skill", "skills/mono/SKILL.md", "Main DWS SKILL.md path")
	flag.StringVar(&productsDir, "products", "skills/mono/references/products", "Product skill reference directory")
	flag.StringVar(&intentGuidePath, "intent-guide", "skills/mono/references/intent-guide.md", "Cross-product intent guide path")
	flag.StringVar(&hintsDir, "hints", "internal/cli/schema_hints", "Versioned Agent hint JSON directory (metadata/ + selection/)")
	flag.StringVar(&interfaceMetadataPath, "interface-metadata", "internal/cli/schema_mcp_metadata.json", "Sanitized versioned MCP metadata used only for fallback Agent summaries")
	flag.StringVar(&outputPath, "output", "", "Output embedded Agent metadata JSON file (legacy single-file mode)")
	flag.StringVar(&outputDir, "output-dir", "", "Output directory for split embedded Agent metadata JSON")
	flag.StringVar(&auditOutputPath, "audit-output", "", "Optional output path for build-time source and CommandRegistry diagnostics")
	flag.StringVar(&registryPath, "registry", "internal/cli/schema_command_registry.json", "Reviewed CommandRegistry path, relative to --root; validation-only because the registry is embedded")
	flag.StringVar(&legacySurfacePath, "surface", "", "Deprecated alias for --registry; the file must equal the embedded reviewed CommandRegistry")
	flag.IntVar(&maxExamples, "max-examples", 2, "Maximum examples retained per command")
	flag.IntVar(&maxInterfaceSummaryRunes, "max-interface-summary-runes", 120, "Maximum runes retained in an unreviewed MCP-derived Agent summary")
	flag.BoolVar(&validateRegistry, "validate-registry", true, "Require Agent metadata to use the embedded reviewed CommandRegistry")
	flag.BoolVar(&legacyValidateSurface, "validate-surface", true, "Deprecated alias; false is rejected because Registry validation cannot be bypassed")
	flag.Parse()
	if strings.TrimSpace(outputDir) == "" && strings.TrimSpace(outputPath) == "" {
		outputDir = "internal/cli/schema_agent_metadata"
	}
	protectedInputs := []outputguard.Input{
		{Name: "canonical main Skill input", Path: "skills/mono/SKILL.md"},
		{Name: "canonical product Skill input directory", Path: "skills/mono/references/products"},
		{Name: "canonical intent guide input", Path: "skills/mono/references/intent-guide.md"},
		{Name: "canonical structured hint input directory", Path: "internal/cli/schema_hints"},
		{Name: "canonical reviewed CommandRegistry input", Path: "internal/cli/schema_command_registry.json"},
		{Name: "canonical pinned interface metadata input", Path: "internal/cli/schema_mcp_metadata.json"},
		{Name: "main Skill input", Path: skillPath},
		{Name: "product Skill input directory", Path: productsDir},
		{Name: "intent guide input", Path: intentGuidePath},
		{Name: "structured hint input directory", Path: hintsDir},
		{Name: "reviewed CommandRegistry input", Path: registryPath},
	}
	if strings.TrimSpace(interfaceMetadataPath) != "" {
		protectedInputs = append(protectedInputs, outputguard.Input{Name: "pinned interface metadata input", Path: interfaceMetadataPath})
	}
	if strings.TrimSpace(legacySurfacePath) != "" {
		protectedInputs = append(protectedInputs, outputguard.Input{Name: "deprecated Registry compatibility input", Path: legacySurfacePath})
	}
	if err := validateAgentMetadataOutputIsolation(root, protectedInputs, outputPath, outputDir, auditOutputPath); err != nil {
		fail(err)
	}
	if err := validateAgentMetadataOutputAllowlist(root, outputPath, outputDir, auditOutputPath); err != nil {
		fail(err)
	}
	if !legacyValidateSurface {
		validateRegistry = false
	}
	if strings.TrimSpace(legacySurfacePath) != "" {
		if err := validateCommandRegistryFile(root, legacySurfacePath); err != nil {
			fail(fmt.Errorf("validate deprecated -surface compatibility input: %w", err))
		}
	}
	registry, err := loadEffectiveCommandRegistryProjection(root, registryPath, validateRegistry)
	if err != nil {
		fail(err)
	}
	if err := validateSelectionHintInput(root, hintsDir, registry); err != nil {
		fail(err)
	}

	metadata, stats, err := agentmetadata.Generate(agentmetadata.Options{
		Root:                     root,
		SkillPath:                skillPath,
		ProductsDir:              productsDir,
		IntentGuidePath:          intentGuidePath,
		HintsDir:                 hintsDir,
		InterfaceMetadataPath:    interfaceMetadataPath,
		MaxExamples:              maxExamples,
		MaxInterfaceSummaryRunes: maxInterfaceSummaryRunes,
		ToolPaths:                registry.ToolPaths,
		CanonicalToolPaths:       registry.CanonicalToolPaths,
		BoundCommands:            registry.Bound,
		ProductIDs:               registry.ProductIDs,
		SurfaceHash:              registry.Hash,
		SurfaceToolCount:         registry.ToolCount,
	})
	if err != nil {
		fail(err)
	}
	if strings.TrimSpace(outputDir) != "" {
		if err := writeMetadataDirectory(outputDir, metadata); err != nil {
			fail(err)
		}
		outputPath = outputDir
	} else {
		if strings.TrimSpace(outputPath) == "" {
			outputPath = "internal/cli/schema_agent_metadata.json"
		}
		if err := writeMetadataFile(outputPath, metadata); err != nil {
			fail(err)
		}
	}
	if strings.TrimSpace(auditOutputPath) != "" {
		if err := writeAuditFile(auditOutputPath, agentmetadata.BuildAudit(metadata, stats)); err != nil {
			fail(err)
		}
	}
	_, _ = fmt.Fprintf(
		os.Stderr,
		"generated schema Agent metadata: output=%s sources=%d products=%d tools=%d summaries=%d interface_summaries=%d intents=%d examples=%d risk_rules=%d hint_files=%d hint_tools=%d unmatched=%d surface_tools=%d\n",
		outputPath,
		stats.SourceFiles,
		stats.Products,
		stats.Tools,
		metadata.Coverage.ToolsWithSummary,
		interfaceAppliedSummaries(stats),
		stats.ToolIntents,
		stats.Examples,
		stats.RiskRules,
		stats.HintFiles,
		stats.HintTools,
		stats.UnmatchedTools,
		registry.ToolCount,
	)
}

func interfaceAppliedSummaries(stats agentmetadata.Stats) int {
	if stats.InterfaceMetadata == nil {
		return 0
	}
	return stats.InterfaceMetadata.AppliedSummaries
}

func writeAuditFile(path string, audit agentmetadata.Audit) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create audit output directory: %w", err)
	}
	if err := writeJSON(path, audit); err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	return nil
}

type agentMetadataIndex struct {
	Version     int                                      `json:"version"`
	SourceHash  string                                   `json:"source_hash"`
	SurfaceHash string                                   `json:"surface_hash,omitempty"`
	Coverage    agentmetadata.Coverage                   `json:"coverage"`
	Products    map[string]agentmetadata.ProductMetadata `json:"products"`
	Domains     []string                                 `json:"domains"`
}

type agentMetadataDomain struct {
	ProductID string                                `json:"product_id"`
	Tools     map[string]agentmetadata.ToolMetadata `json:"tools"`
}

func writeMetadataFile(path string, metadata agentmetadata.File) error {
	encoded, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func writeMetadataDirectory(dir string, metadata agentmetadata.File) error {
	dir = strings.TrimSpace(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read metadata directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return fmt.Errorf("remove stale metadata %s: %w", entry.Name(), err)
			}
		}
	}

	byDomain := map[string]map[string]agentmetadata.ToolMetadata{}
	for toolPath, tool := range metadata.Tools {
		domain := firstPathToken(toolPath)
		if domain == "" {
			continue
		}
		if byDomain[domain] == nil {
			byDomain[domain] = map[string]agentmetadata.ToolMetadata{}
		}
		byDomain[domain][toolPath] = tool
	}
	domains := make([]string, 0, len(byDomain))
	for domain := range byDomain {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	index := agentMetadataIndex{
		Version:     metadata.Version,
		SourceHash:  metadata.SourceHash,
		SurfaceHash: metadata.SurfaceHash,
		Coverage:    metadata.Coverage,
		Products:    metadata.Products,
		Domains:     domains,
	}
	if err := writeJSON(filepath.Join(dir, "index.json"), index); err != nil {
		return err
	}
	for _, domain := range domains {
		if err := writeJSON(filepath.Join(dir, domain+".json"), agentMetadataDomain{
			ProductID: domain,
			Tools:     byDomain[domain],
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func firstPathToken(path string) string {
	parts := strings.Fields(strings.TrimSpace(path))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

type commandRegistryProjection struct {
	ToolPaths          map[string]string
	CanonicalToolPaths map[string]string
	ProductIDs         map[string]bool
	Hash               string
	ToolCount          int
	Bound              cli.BoundCommandRegistry
}

// loadEffectiveCommandRegistryProjection consumes the same reviewed registry
// API as the Catalog generator. The registry file argument is validation only
// and can never replace or mutate the embedded identity registry.
func loadEffectiveCommandRegistryProjection(rootPath, registryPath string, validateRegistry bool) (commandRegistryProjection, error) {
	if !validateRegistry {
		return commandRegistryProjection{}, fmt.Errorf("CommandRegistry validation cannot be disabled: Agent metadata must use the reviewed CommandRegistry")
	}
	if err := validateCommandRegistryFile(rootPath, registryPath); err != nil {
		return commandRegistryProjection{}, fmt.Errorf("validate CommandRegistry input: %w", err)
	}
	commandRoot := app.NewRootCommand()
	effective, err := cli.BuildEffectiveCommandRegistry(commandRoot)
	if err != nil {
		return commandRegistryProjection{}, fmt.Errorf("build effective CommandRegistry: %w", err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(commandRoot, effective)
	if err != nil {
		return commandRegistryProjection{}, fmt.Errorf("bind effective CommandRegistry: %w", err)
	}
	projection := projectEffectiveCommandRegistry(effective)
	projection.Bound = bound
	return projection, nil
}

// projectEffectiveCommandRegistry deliberately accepts the post-manual
// registry. Keeping projection below this boundary prevents a base-registry
// allowlist from silently dropping reviewed manual-only commands.
func projectEffectiveCommandRegistry(effective cli.EffectiveCommandRegistry) commandRegistryProjection {
	projection := commandRegistryProjection{
		ToolPaths:          map[string]string{},
		CanonicalToolPaths: map[string]string{},
		ProductIDs:         map[string]bool{},
		Hash:               effective.SourceHash(),
	}
	for _, command := range effective.Commands {
		if command.Visibility != cli.SchemaVisibilityPublic {
			continue
		}
		projection.ToolCount++
		primary := strings.TrimSpace(command.PrimaryCLIPath)
		projection.ToolPaths[primary] = primary
		projection.ToolPaths[command.CanonicalPath] = primary
		projection.CanonicalToolPaths[command.CanonicalPath] = primary
		if productID, _, ok := strings.Cut(command.CanonicalPath, "."); ok && strings.TrimSpace(productID) != "" {
			projection.ProductIDs[strings.TrimSpace(productID)] = true
		}
		for _, alias := range command.Aliases {
			if alias = strings.TrimSpace(alias); alias != "" {
				projection.ToolPaths[alias] = primary
			}
		}
	}
	return projection
}

func validateCommandRegistryFile(rootPath, registryPath string) error {
	if strings.TrimSpace(registryPath) == "" {
		return nil
	}
	data, err := os.ReadFile(resolveRootPath(rootPath, registryPath))
	if err != nil {
		return err
	}
	_, err = cli.ValidateCommandRegistrySource(data)
	return err
}

func validateSelectionHintInput(rootPath, hintsDir string, registry commandRegistryProjection) error {
	hintsRoot := resolveRootPath(rootPath, hintsDir)
	selectionRoot := filepath.Join(hintsRoot, "selection")
	metadataRoot := filepath.Join(hintsRoot, "metadata")
	for _, dir := range []string{selectionRoot, metadataRoot} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return fmt.Errorf("required Agent hint directory missing: %s", dir)
		}
	}
	selectionFS := os.DirFS(selectionRoot)
	agentHints, err := cli.LoadAgentHintsFromSelectionForValidation(selectionFS)
	if err != nil {
		return fmt.Errorf("load selection Agent hints: %w", err)
	}
	expectedTools := make(map[string]bool, len(registry.CanonicalToolPaths))
	for canonical := range registry.CanonicalToolPaths {
		expectedTools[canonical] = true
	}
	if err := cli.ValidateManualAgentHintSet(agentHints, registry.ProductIDs, expectedTools); err != nil {
		return fmt.Errorf("validate selection Agent hints: %w", err)
	}
	if err := cli.ValidateManualAgentHintExamples(registry.Bound, agentHints); err != nil {
		return fmt.Errorf("validate selection Agent hint examples: %w", err)
	}
	if _, err := cli.ValidateManualAgentSelectionContract(registry.Bound, agentHints); err != nil {
		return fmt.Errorf("validate selection Agent selection contract: %w", err)
	}
	return nil
}

func resolveRootPath(root, path string) string {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

// validateManualHintsOutputIsolation protects reviewed human-authored hint
// inputs before the generator writes any output.
func validateManualHintsOutputIsolation(rootPath, hintsDir, outputPath, outputDir, auditOutputPath string) error {
	return validateAgentMetadataOutputIsolation(rootPath,
		[]outputguard.Input{{Name: "structured hint input directory", Path: hintsDir}},
		outputPath, outputDir, auditOutputPath,
	)
}

func validateAgentMetadataOutputIsolation(rootPath string, inputs []outputguard.Input, outputPath, outputDir, auditOutputPath string) error {
	return outputguard.Validate(rootPath,
		inputs,
		[]outputguard.Target{
			{Name: "--output", Path: outputPath},
			{Name: "--output-dir", Path: outputDir, Directory: true},
			{Name: "--audit-output", Path: auditOutputPath},
		},
	)
}

func validateAgentMetadataOutputAllowlist(rootPath, outputPath, outputDir, auditOutputPath string) error {
	for _, target := range []struct {
		target  outputguard.Target
		allowed string
	}{
		{outputguard.Target{Name: "--output", Path: outputPath}, "internal/cli/schema_agent_metadata.json"},
		{outputguard.Target{Name: "--output-dir", Path: outputDir, Directory: true}, "internal/cli/schema_agent_metadata"},
		{outputguard.Target{Name: "--audit-output", Path: auditOutputPath}, "internal/cli/schema_agent_metadata_audit.json"},
	} {
		if err := outputguard.ValidateRepoTargetAllowlist(rootPath, target.target, target.allowed); err != nil {
			return err
		}
	}
	return nil
}

func fail(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "generate-schema-agent-metadata: %v\n", err)
	os.Exit(1)
}
