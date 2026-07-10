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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/agentmetadata"
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
	var surfacePath string
	var writeSurfacePath string
	var maxExamples int
	var maxInterfaceSummaryRunes int
	var validateSurface bool
	flag.StringVar(&root, "root", ".", "Repository root")
	flag.StringVar(&skillPath, "skill", "skills/mono/SKILL.md", "Main DWS SKILL.md path")
	flag.StringVar(&productsDir, "products", "skills/mono/references/products", "Product skill reference directory")
	flag.StringVar(&intentGuidePath, "intent-guide", "skills/mono/references/intent-guide.md", "Cross-product intent guide path")
	flag.StringVar(&hintsDir, "hints", "skills/mono/schema-hints", "Versioned Agent hint JSON directory")
	flag.StringVar(&interfaceMetadataPath, "interface-metadata", "internal/cli/schema_mcp_metadata.json", "Sanitized versioned MCP metadata used only for fallback Agent summaries")
	flag.StringVar(&outputPath, "output", "", "Output embedded Agent metadata JSON file (legacy single-file mode)")
	flag.StringVar(&outputDir, "output-dir", "", "Output directory for split embedded Agent metadata JSON")
	flag.StringVar(&auditOutputPath, "audit-output", "", "Optional output path for build-time source and command-surface diagnostics")
	flag.StringVar(&surfacePath, "surface", "", "Versioned command-surface snapshot path, relative to --root")
	flag.StringVar(&writeSurfacePath, "write-surface", "", "Write the current runtime command surface snapshot and exit")
	flag.IntVar(&maxExamples, "max-examples", 2, "Maximum examples retained per command")
	flag.IntVar(&maxInterfaceSummaryRunes, "max-interface-summary-runes", 120, "Maximum runes retained in an unreviewed MCP-derived Agent summary")
	flag.BoolVar(&validateSurface, "validate-surface", true, "Keep only paths present in the command-surface snapshot or current runtime schema")
	flag.Parse()
	if strings.TrimSpace(writeSurfacePath) != "" {
		if err := writeCommandSurfaceSnapshot(resolveRootPath(root, writeSurfacePath)); err != nil {
			fail(fmt.Errorf("write command surface: %w", err))
		}
		return
	}
	if strings.TrimSpace(outputDir) == "" && strings.TrimSpace(outputPath) == "" {
		outputDir = "internal/cli/schema_agent_metadata"
	}
	var surface commandSurface
	if validateSurface {
		var err error
		if strings.TrimSpace(surfacePath) != "" {
			surface, err = loadCommandSurfaceSnapshot(resolveRootPath(root, surfacePath))
		} else {
			surface, err = loadCommandSurface()
		}
		if err != nil {
			fail(fmt.Errorf("load command surface: %w", err))
		}
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
		ToolPaths:                surface.ToolPaths,
		ProductIDs:               surface.ProductIDs,
		SurfaceHash:              surface.Hash,
		SurfaceToolCount:         surface.ToolCount,
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
		surface.ToolCount,
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

type commandSurface struct {
	ToolPaths  map[string]string
	ProductIDs map[string]bool
	Hash       string
	ToolCount  int
}

const commandSurfaceSnapshotVersion = 1

type commandSurfaceSnapshot struct {
	Version  int                     `json:"version"`
	Products []commandSurfaceProduct `json:"products"`
}

type commandSurfaceProduct struct {
	ID    string               `json:"id"`
	Tools []commandSurfaceTool `json:"tools"`
}

type commandSurfaceTool struct {
	CanonicalPath   string   `json:"canonical_path,omitempty"`
	SourceProductID string   `json:"source_product_id,omitempty"`
	CLIPath         string   `json:"cli_path"`
	Aliases         []string `json:"aliases,omitempty"`
}

func loadCommandSurface() (commandSurface, error) {
	snapshot, err := currentCommandSurfaceSnapshot()
	if err != nil {
		return commandSurface{}, err
	}
	return commandSurfaceFromSnapshot(snapshot), nil
}

func currentCommandSurfaceSnapshot() (commandSurfaceSnapshot, error) {
	root := app.NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"schema", "--all", "--format", "json"})
	if err := root.Execute(); err != nil {
		return commandSurfaceSnapshot{}, fmt.Errorf("execute dws schema --all: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var snapshot commandSurfaceSnapshot
	if err := json.Unmarshal(stdout.Bytes(), &snapshot); err != nil {
		return commandSurfaceSnapshot{}, fmt.Errorf("decode schema catalog: %w", err)
	}
	snapshot.Version = commandSurfaceSnapshotVersion
	return normalizeCommandSurfaceSnapshot(snapshot), nil
}

func loadCommandSurfaceSnapshot(path string) (commandSurface, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return commandSurface{}, err
	}
	var snapshot commandSurfaceSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return commandSurface{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if snapshot.Version != commandSurfaceSnapshotVersion {
		return commandSurface{}, fmt.Errorf("unsupported command surface version %d", snapshot.Version)
	}
	return commandSurfaceFromSnapshot(normalizeCommandSurfaceSnapshot(snapshot)), nil
}

func writeCommandSurfaceSnapshot(path string) error {
	snapshot, err := currentCommandSurfaceSnapshot()
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create snapshot directory: %w", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	surface := commandSurfaceFromSnapshot(snapshot)
	_, _ = fmt.Fprintf(os.Stderr, "generated schema command surface: output=%s products=%d tools=%d hash=%s\n", path, len(surface.ProductIDs), surface.ToolCount, surface.Hash)
	return nil
}

func commandSurfaceFromSnapshot(snapshot commandSurfaceSnapshot) commandSurface {
	surface := commandSurface{
		ToolPaths:  map[string]string{},
		ProductIDs: map[string]bool{},
	}
	rows := make([]string, 0)
	seenPrimary := map[string]bool{}
	for _, product := range snapshot.Products {
		productID := strings.TrimSpace(product.ID)
		if productID != "" {
			surface.ProductIDs[productID] = true
		}
		for _, tool := range product.Tools {
			primary := strings.TrimSpace(tool.CLIPath)
			if primary == "" {
				continue
			}
			surface.ToolPaths[primary] = primary
			if canonical := strings.TrimSpace(tool.CanonicalPath); canonical != "" {
				surface.ToolPaths[canonical] = primary
			}
			if !seenPrimary[primary] {
				seenPrimary[primary] = true
				surface.ToolCount++
			}
			aliases := append([]string(nil), tool.Aliases...)
			sort.Strings(aliases)
			for _, alias := range aliases {
				alias = strings.TrimSpace(alias)
				if alias != "" {
					surface.ToolPaths[alias] = primary
				}
			}
			rows = append(rows, productID+"\x00"+strings.TrimSpace(tool.CanonicalPath)+"\x00"+strings.TrimSpace(tool.SourceProductID)+"\x00"+primary+"\x00"+strings.Join(aliases, "\x00"))
		}
	}
	sort.Strings(rows)
	sum := sha256.Sum256([]byte(strings.Join(rows, "\n")))
	surface.Hash = "sha256:" + hex.EncodeToString(sum[:])
	return surface
}

func normalizeCommandSurfaceSnapshot(snapshot commandSurfaceSnapshot) commandSurfaceSnapshot {
	products := make([]commandSurfaceProduct, 0, len(snapshot.Products))
	for _, product := range snapshot.Products {
		product.ID = strings.TrimSpace(product.ID)
		if product.ID == "" {
			continue
		}
		tools := make([]commandSurfaceTool, 0, len(product.Tools))
		for _, tool := range product.Tools {
			tool.CanonicalPath = strings.TrimSpace(tool.CanonicalPath)
			tool.SourceProductID = strings.TrimSpace(tool.SourceProductID)
			tool.CLIPath = strings.TrimSpace(tool.CLIPath)
			if tool.CLIPath == "" {
				continue
			}
			aliases := make([]string, 0, len(tool.Aliases))
			seenAliases := map[string]bool{}
			for _, alias := range tool.Aliases {
				alias = strings.TrimSpace(alias)
				if alias != "" && !seenAliases[alias] {
					seenAliases[alias] = true
					aliases = append(aliases, alias)
				}
			}
			sort.Strings(aliases)
			tools = append(tools, commandSurfaceTool{
				CanonicalPath:   tool.CanonicalPath,
				SourceProductID: tool.SourceProductID,
				CLIPath:         tool.CLIPath,
				Aliases:         aliases,
			})
		}
		sort.Slice(tools, func(i, j int) bool { return tools[i].CLIPath < tools[j].CLIPath })
		products = append(products, commandSurfaceProduct{ID: product.ID, Tools: tools})
	}
	sort.Slice(products, func(i, j int) bool { return products[i].ID < products[j].ID })
	return commandSurfaceSnapshot{Version: commandSurfaceSnapshotVersion, Products: products}
}

func resolveRootPath(root, path string) string {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func fail(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "generate-schema-agent-metadata: %v\n", err)
	os.Exit(1)
}
