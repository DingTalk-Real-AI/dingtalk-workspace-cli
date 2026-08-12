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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/outputguard"
	"github.com/spf13/cobra"
)

var (
	validateCatalogParameterBindings = cli.ValidateSchemaParameterBindings
	buildCatalogSnapshot             = cli.BuildSchemaCatalogSnapshot
	installCatalogAgentMetadata      = installBuildTimeAgentMetadata
	makeCatalogDirectory             = os.MkdirAll
	writeCatalogFile                 = os.WriteFile
	exitCatalogProcess               = os.Exit
)

func main() {
	var rootPath string
	var surfacePath string
	var outputPath string
	var metaIndexPath string
	flag.StringVar(&rootPath, "root", ".", "Repository root used to protect Schema generator inputs")
	flag.StringVar(&surfacePath, "surface", "", "Retired; rejected when set. Command identity is collected from ContractFinal declarations")
	flag.StringVar(&outputPath, "output", "artifacts/schema_catalog", "Output directory for a CI/local Catalog dump (catalog.json + tools/<product>.json); not a go:generate or production delivery step")
	flag.StringVar(&metaIndexPath, "meta-index", "", "Output path for CommandMeta summary index gob (default: sibling schema_meta_index.gob next to --output)")
	flag.Parse()
	// cmd_schema_catalog is a CI/determinism and policy dump tool. Production
	// Catalog delivery assembles via cli.ResolveSchemaBuild at runtime.
	resolvedSurfacePath := resolveCatalogRootPath(rootPath, surfacePath)
	resolvedMetaIndexPath := resolveSchemaMetaIndexPath(outputPath, metaIndexPath)
	if err := validateCatalogOutputIsolation(rootPath, outputPath, resolvedMetaIndexPath); err != nil {
		fail(err)
	}

	root := app.NewSchemaSourceRootCommand()
	if err := generateSchemaCatalog(rootPath, root, resolvedSurfacePath, outputPath, resolvedMetaIndexPath); err != nil {
		fail(err)
	}
}

func resolveCatalogRootPath(rootPath, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(rootPath, path)
}

func resolveSchemaMetaIndexPath(outputPath, metaIndexPath string) string {
	metaIndexPath = strings.TrimSpace(metaIndexPath)
	if metaIndexPath != "" {
		return metaIndexPath
	}
	return filepath.Join(filepath.Dir(outputPath), "schema_meta_index.gob")
}

func validateCatalogOutputIsolation(rootPath, outputPath, metaIndexPath string) error {
	inputs := []outputguard.Input{
		{Name: "main Skill metadata source", Path: "skills/mono/SKILL.md"},
		{Name: "product Skill metadata source directory", Path: "skills/mono/references/products"},
		{Name: "intent guide metadata source", Path: "skills/mono/references/intent-guide.md"},
		{Name: "reviewed parameter mapping ledger input", Path: "internal/cli/schema_parameter_mapping_ledger.go"},
		{Name: "reviewed command exclusion input", Path: "internal/cli/schema_command_exclusions.go"},
		{Name: "reviewed param concepts input", Path: "internal/cli/param_concepts.json"},
	}
	targets := []outputguard.Target{
		{Name: "--output", Path: outputPath, Directory: true},
		{Name: "--meta-index", Path: metaIndexPath},
	}
	if err := outputguard.Validate(rootPath, inputs, targets); err != nil {
		return err
	}
	if err := outputguard.ValidateRepoTargetAllowlist(rootPath,
		outputguard.Target{Name: "--output", Path: outputPath, Directory: true},
		"artifacts/schema_catalog",
	); err != nil {
		return err
	}
	return outputguard.ValidateRepoTargetAllowlist(rootPath,
		outputguard.Target{Name: "--meta-index", Path: metaIndexPath},
		"artifacts/schema_meta_index.gob",
	)
}

// generateSchemaCatalog consumes the cli package's identity collection API.
// It deliberately does not decode command identity itself: the retired
// --surface flag is rejected, so an external identity file can never enter
// assembly. Agent metadata may be generated in-memory and injected for this
// CI/local dump only; production Agent authority remains ContractFinal /
// ProductDecl. schema_agent_metadata/ is not a delivery artifact.
func generateSchemaCatalog(rootPath string, root *cobra.Command, surfacePath, outputPath, metaIndexPath string) error {
	return generateSchemaCatalogWithResolver(rootPath, root, surfacePath, outputPath, metaIndexPath, cli.ResolveSchemaBuild)
}

type schemaBuildResolver func(*cobra.Command) (cli.ResolvedSchemaBuild, error)

// generateSchemaCatalogWithResolver exists to make the single-resolution
// contract observable in tests. Production passes cli.ResolveSchemaBuild; the
// returned Effective/Bound/SchemaRegistry views then travel together through
// every gate and the final serializer.
func generateSchemaCatalogWithResolver(rootPath string, root *cobra.Command, surfacePath, outputPath, metaIndexPath string, resolve schemaBuildResolver) error {
	if root == nil {
		return fmt.Errorf("schema source root is nil")
	}
	if resolve == nil {
		return fmt.Errorf("schema build resolver is nil")
	}
	metaIndexPath = resolveSchemaMetaIndexPath(outputPath, metaIndexPath)
	if err := validateDeprecatedSurface(surfacePath); err != nil {
		return err
	}
	if err := validateCatalogParameterBindings(); err != nil {
		return fmt.Errorf("validate reviewed parameter binding input: %w", err)
	}
	if err := installCatalogAgentMetadata(rootPath, root); err != nil {
		return err
	}
	defer cli.ClearBuildTimeAgentMetadata()

	resolved, err := resolve(root)
	if err != nil {
		return fmt.Errorf("resolve final Schema build: %w", err)
	}
	snapshot, err := buildCatalogSnapshot(resolved, cli.SchemaCatalogBuildOptions{
		RegistryHash: resolved.RegistryHash(),
	})
	if err != nil {
		return err
	}
	if err := writeSchemaCatalogShards(snapshot, outputPath); err != nil {
		return err
	}
	if err := writeCatalogMetaIndex(snapshot, metaIndexPath); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "generated schema catalog: output=%s meta_index=%s registry_commands=%d tools=%d products=%d registry_hash=%s source_hash=%s\n",
		outputPath, metaIndexPath, resolved.CommandCount(), len(snapshot.Tools), countSchemaCatalogProducts(snapshot), snapshot.SurfaceHash, snapshot.SourceHash)
	return nil
}

var (
	buildCatalogMetaIndex    = cli.BuildSchemaMetaIndex
	validateCatalogMetaIndex = cli.ValidateSchemaMetaIndexAgainstSnapshot
	encodeCatalogMetaIndex   = cli.EncodeSchemaMetaIndex
	writeCatalogMetaIndex    = writeSchemaMetaIndex
)

func writeSchemaMetaIndex(snapshot cli.SchemaCatalogSnapshot, outputPath string) error {
	index, err := buildCatalogMetaIndex(snapshot)
	if err != nil {
		return fmt.Errorf("build schema meta index: %w", err)
	}
	if err := validateCatalogMetaIndex(index, snapshot); err != nil {
		return fmt.Errorf("validate schema meta index against catalog: %w", err)
	}
	encoded, err := encodeCatalogMetaIndex(index)
	if err != nil {
		return err
	}
	if err := makeCatalogDirectory(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create schema meta index directory: %w", err)
	}
	if err := writeCatalogFile(outputPath, encoded, 0o644); err != nil {
		return fmt.Errorf("write schema meta index: %w", err)
	}
	return nil
}

// schemaCatalogEnvelope is the global half of the split catalog. It carries
// the release envelope (version + integrity hashes) and the Catalog map, whose
// products array and cross-product aggregates do not partition by product.
type schemaCatalogEnvelope struct {
	Version     int            `json:"version"`
	SurfaceHash string         `json:"surface_hash,omitempty"`
	SourceHash  string         `json:"source_hash"`
	Catalog     map[string]any `json:"catalog"`
}

// schemaCatalogToolShard is the per-product half of the split catalog. Each
// product's leaf ToolSpecs live in their own file so concurrent feature PRs
// only rewrite the shard for the product they touch.
type schemaCatalogToolShard struct {
	Product string                    `json:"product"`
	Tools   map[string]map[string]any `json:"tools"`
}

// writeSchemaCatalogShards partitions a validated snapshot into a release
// directory: catalog.json holds the global envelope + Catalog map, and
// tools/<product>.json holds each product's leaf ToolSpecs keyed by canonical
// path. The split is a storage concern only: the loader reassembles the exact
// same SchemaCatalogSnapshot, so source_hash still validates the whole payload.
func writeSchemaCatalogShards(snapshot cli.SchemaCatalogSnapshot, outputDir string) error {
	if err := os.RemoveAll(outputDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear stale schema catalog output: %w", err)
	}
	toolsDir := filepath.Join(outputDir, "tools")
	if err := makeCatalogDirectory(toolsDir, 0o755); err != nil {
		return fmt.Errorf("create schema catalog tools directory: %w", err)
	}

	envelope := schemaCatalogEnvelope{
		Version:     snapshot.Version,
		SurfaceHash: snapshot.SurfaceHash,
		SourceHash:  snapshot.SourceHash,
		Catalog:     snapshot.Catalog,
	}
	if err := writeSchemaCatalogJSON(filepath.Join(outputDir, "catalog.json"), envelope); err != nil {
		return fmt.Errorf("write schema catalog.json: %w", err)
	}

	for product, tools := range partitionSchemaCatalogTools(snapshot.Tools) {
		shard := schemaCatalogToolShard{Product: product, Tools: tools}
		if err := writeSchemaCatalogJSON(filepath.Join(toolsDir, product+".json"), shard); err != nil {
			return fmt.Errorf("write schema catalog tools/%s.json: %w", product, err)
		}
	}
	return nil
}

// partitionSchemaCatalogTools groups leaf ToolSpecs by the product prefix of
// their canonical path (the segment before the first dot). Products are
// returned as a sorted map so generation is deterministic.
func partitionSchemaCatalogTools(tools map[string]map[string]any) map[string]map[string]map[string]any {
	partitioned := map[string]map[string]map[string]any{}
	for canonical, spec := range tools {
		product := canonical
		if idx := strings.IndexByte(canonical, '.'); idx > 0 {
			product = canonical[:idx]
		}
		if partitioned[product] == nil {
			partitioned[product] = map[string]map[string]any{}
		}
		partitioned[product][canonical] = spec
	}
	return partitioned
}

func countSchemaCatalogProducts(snapshot cli.SchemaCatalogSnapshot) int {
	maxProduct := 0
	for range partitionSchemaCatalogTools(snapshot.Tools) {
		maxProduct++
	}
	return maxProduct
}

func writeSchemaCatalogJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	return writeCatalogFile(path, append(encoded, '\n'), 0o644)
}

// validateDeprecatedSurface rejects the retired -surface compatibility input.
// The reviewed CommandRegistry it had to equal was retired when identity
// collection became the single source; an external identity file can never
// re-enter assembly.
func validateDeprecatedSurface(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return fmt.Errorf("deprecated -surface input %q is retired: command identity is collected from ContractFinal declarations", path)
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	exitCatalogProcess(1)
}
