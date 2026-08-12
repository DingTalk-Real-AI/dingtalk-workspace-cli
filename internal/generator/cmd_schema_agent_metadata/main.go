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

var (
	validateMetadataIsolation      = validateAgentMetadataOutputIsolation
	validateMetadataAllowlist      = validateAgentMetadataOutputAllowlist
	loadMetadataRegistryProjection = loadEffectiveCommandRegistryProjection
	validateMetadataSelection      = validateSelectionCoverage
	generateAgentMetadata          = agentmetadata.Generate
	writeMetadataDirectoryOutput   = writeMetadataDirectory
	writeMetadataFileOutput        = writeMetadataFile
	writeMetadataAuditOutput       = writeAuditFile
	exitMetadataProcess            = os.Exit

	makeMetadataDirectory  = os.MkdirAll
	readMetadataDirectory  = os.ReadDir
	removeMetadataFile     = os.Remove
	writeMetadataFileBytes = os.WriteFile
	writeMetadataJSON      = writeJSON

	newMetadataRoot        = app.NewSchemaSourceRootCommand
	buildEffectiveMetadata = cli.BuildEffectiveCommandRegistry
	bindEffectiveMetadata  = cli.BindEffectiveCommandRegistry
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
	// Flag name "-hints" is retained as a fail-closed anti-regression valve
	// (historical CLI compatibility). Non-empty values are rejected below;
	// do not remove the flag or silently ignore it.
	flag.StringVar(&hintsDir, "hints", "", "Retired; rejected when set. Declare ProductDecl/ContractFinal selection instead of schema_hints/")
	flag.StringVar(&interfaceMetadataPath, "interface-metadata", "", "Optional diagnostic MCP dump for fallback Agent summaries (retired pin path refused)")
	flag.StringVar(&outputPath, "output", "", "Optional diagnostic single-file Agent metadata JSON (not a Catalog input)")
	flag.StringVar(&outputDir, "output-dir", "", "Optional diagnostic split Agent metadata directory (not a Catalog input; Catalog injects in-memory)")
	flag.StringVar(&auditOutputPath, "audit-output", "", "Optional output path for build-time source and CommandRegistry diagnostics")
	flag.StringVar(&registryPath, "registry", "", "Retired; rejected when set. Command identity is collected from ContractFinal declarations")
	flag.StringVar(&legacySurfacePath, "surface", "", "Retired alias for --registry; rejected when set")
	flag.IntVar(&maxExamples, "max-examples", 2, "Maximum examples retained per command")
	flag.IntVar(&maxInterfaceSummaryRunes, "max-interface-summary-runes", 120, "Maximum runes retained in an unreviewed MCP-derived Agent summary")
	flag.BoolVar(&validateRegistry, "validate-registry", true, "Retired anti-bypass valve; false is rejected because identity collection cannot be bypassed")
	flag.BoolVar(&legacyValidateSurface, "validate-surface", true, "Deprecated alias; false is rejected because identity collection cannot be bypassed")
	flag.Parse()
	if strings.TrimSpace(hintsDir) != "" {
		fail(fmt.Errorf("-hints is retired; clear the flag and declare ProductDecl/ContractFinal selection instead (got %q)", hintsDir))
	}
	// Flag names "-registry"/"-surface" are retained as fail-closed
	// anti-regression valves (historical CLI compatibility). The reviewed
	// CommandRegistry they pointed at is retired; identity is collected from
	// ContractFinal declarations, so external identity files are rejected.
	if strings.TrimSpace(registryPath) != "" {
		fail(fmt.Errorf("-registry is retired; command identity is collected from ContractFinal declarations (got %q)", registryPath))
	}
	if strings.TrimSpace(legacySurfacePath) != "" {
		fail(fmt.Errorf("-surface is retired; command identity is collected from ContractFinal declarations (got %q)", legacySurfacePath))
	}
	// Disk output is optional and diagnostic only. Catalog generation uses the
	// in-memory agentmetadata pipeline and does not consume schema_agent_metadata/.
	writeRequested := strings.TrimSpace(outputDir) != "" || strings.TrimSpace(outputPath) != ""
	protectedInputs := []outputguard.Input{
		{Name: "canonical main Skill input", Path: "skills/mono/SKILL.md"},
		{Name: "canonical product Skill input directory", Path: "skills/mono/references/products"},
		{Name: "canonical intent guide input", Path: "skills/mono/references/intent-guide.md"},
		{Name: "main Skill input", Path: skillPath},
		{Name: "product Skill input directory", Path: productsDir},
		{Name: "intent guide input", Path: intentGuidePath},
	}
	if strings.TrimSpace(interfaceMetadataPath) != "" {
		if strings.HasSuffix(strings.ReplaceAll(interfaceMetadataPath, "\\", "/"), "internal/cli/schema_mcp_metadata.json") {
			fail(fmt.Errorf("-interface-metadata refuses retired Schema pin internal/cli/schema_mcp_metadata.json"))
		}
		protectedInputs = append(protectedInputs, outputguard.Input{Name: "diagnostic interface metadata input", Path: interfaceMetadataPath})
	}
	if err := validateMetadataIsolation(root, protectedInputs, outputPath, outputDir, auditOutputPath); err != nil {
		fail(err)
	}
	if err := validateMetadataAllowlist(root, outputPath, outputDir, auditOutputPath); err != nil {
		fail(err)
	}
	if !legacyValidateSurface {
		validateRegistry = false
	}
	registry, err := loadMetadataRegistryProjection(validateRegistry)
	if err != nil {
		fail(err)
	}
	if err := validateMetadataSelection(root, hintsDir, registry); err != nil {
		fail(err)
	}

	metadata, stats, err := generateAgentMetadata(agentmetadata.Options{
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
	delivery := "in-memory"
	if strings.TrimSpace(outputDir) != "" {
		if err := writeMetadataDirectoryOutput(outputDir, metadata); err != nil {
			fail(err)
		}
		delivery = outputDir
	} else if strings.TrimSpace(outputPath) != "" {
		if err := writeMetadataFileOutput(outputPath, metadata); err != nil {
			fail(err)
		}
		delivery = outputPath
	} else if !writeRequested {
		_, _ = fmt.Fprintln(os.Stderr, "schema Agent metadata validated in-memory (no --output/--output-dir; Catalog injects Agent metadata without this artifact)")
	}
	if strings.TrimSpace(auditOutputPath) != "" {
		if err := writeMetadataAuditOutput(auditOutputPath, agentmetadata.BuildAudit(metadata, stats)); err != nil {
			fail(err)
		}
	}
	_, _ = fmt.Fprintf(
		os.Stderr,
		"generated schema Agent metadata: output=%s sources=%d products=%d tools=%d summaries=%d interface_summaries=%d intents=%d examples=%d risk_rules=%d unmatched=%d surface_tools=%d\n",
		delivery,
		stats.SourceFiles,
		stats.Products,
		stats.Tools,
		metadata.Coverage.ToolsWithSummary,
		interfaceAppliedSummaries(stats),
		stats.ToolIntents,
		stats.Examples,
		stats.RiskRules,
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
	if err := makeMetadataDirectory(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create audit output directory: %w", err)
	}
	if err := writeMetadataJSON(path, audit); err != nil {
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
	encoded, _ := json.MarshalIndent(metadata, "", "  ")
	if err := makeMetadataDirectory(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := writeMetadataFileBytes(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func writeMetadataDirectory(dir string, metadata agentmetadata.File) error {
	dir = strings.TrimSpace(dir)
	if err := makeMetadataDirectory(dir, 0o755); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
	}
	entries, err := readMetadataDirectory(dir)
	if err != nil {
		return fmt.Errorf("read metadata directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			if err := removeMetadataFile(filepath.Join(dir, entry.Name())); err != nil {
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
	if err := writeMetadataJSON(filepath.Join(dir, "index.json"), index); err != nil {
		return err
	}
	for _, domain := range domains {
		if err := writeMetadataJSON(filepath.Join(dir, domain+".json"), agentMetadataDomain{
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
	if err := writeMetadataFileBytes(path, append(encoded, '\n'), 0o644); err != nil {
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

type commandRegistryProjection = agentmetadata.RegistryProjection

// loadEffectiveCommandRegistryProjection builds the effective command
// identity the same way the Catalog generator does: by collecting
// ContractFinal declarations from the live command tree. There is no
// external identity file to validate or merge.
func loadEffectiveCommandRegistryProjection(validateRegistry bool) (commandRegistryProjection, error) {
	if !validateRegistry {
		return commandRegistryProjection{}, fmt.Errorf("CommandRegistry validation cannot be disabled: Agent metadata identity is collected from ContractFinal declarations")
	}
	commandRoot := newMetadataRoot()
	effective, err := buildEffectiveMetadata(commandRoot)
	if err != nil {
		return commandRegistryProjection{}, fmt.Errorf("build effective CommandRegistry: %w", err)
	}
	bound, err := bindEffectiveMetadata(commandRoot, effective)
	if err != nil {
		return commandRegistryProjection{}, fmt.Errorf("bind effective CommandRegistry: %w", err)
	}
	projection := projectEffectiveCommandRegistry(effective)
	projection.Bound = bound
	return projection, nil
}

// projectEffectiveCommandRegistry deliberately accepts the full effective
// registry. Keeping projection below this boundary prevents a partial
// allowlist from silently dropping effective commands.
func projectEffectiveCommandRegistry(effective cli.EffectiveCommandRegistry) commandRegistryProjection {
	return agentmetadata.ProjectEffectiveRegistry(effective)
}

func validateSelectionCoverage(_ string, _ string, registry commandRegistryProjection) error {
	return agentmetadata.ValidateSelectionCoverage(registry)
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
	// Optional diagnostic writes may still land on the retired on-disk paths,
	// but Catalog generation never reads them.
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
	exitMetadataProcess(1)
}
