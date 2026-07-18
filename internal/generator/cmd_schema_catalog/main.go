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

func main() {
	var rootPath string
	var surfacePath string
	var outputPath string
	flag.StringVar(&rootPath, "root", ".", "Repository root used to protect Schema generator inputs")
	flag.StringVar(&surfacePath, "surface", "", "Deprecated compatibility input relative to --root; when set it must equal the embedded reviewed CommandRegistry")
	flag.StringVar(&outputPath, "output", "internal/cli/schema_catalog", "Output directory for the split embedded schema catalog (catalog.json + tools/<product>.json)")
	flag.Parse()
	resolvedSurfacePath := resolveCatalogRootPath(rootPath, surfacePath)
	if err := validateCatalogOutputIsolation(rootPath, outputPath, resolvedSurfacePath); err != nil {
		fail(err)
	}

	root := app.NewRootCommand()
	if err := generateSchemaCatalog(root, resolvedSurfacePath, outputPath); err != nil {
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

func validateCatalogOutputIsolation(rootPath, outputPath, surfacePath string) error {
	inputs := []outputguard.Input{
		{Name: "main Skill metadata source", Path: "skills/mono/SKILL.md"},
		{Name: "product Skill metadata source directory", Path: "skills/mono/references/products"},
		{Name: "intent guide metadata source", Path: "skills/mono/references/intent-guide.md"},
		{Name: "structured metadata source directory", Path: "internal/cli/schema_hints"},
		{Name: "reviewed CommandRegistry input", Path: "internal/cli/schema_command_registry"},
		{Name: "generated Agent metadata input", Path: "internal/cli/schema_agent_metadata"},
		{Name: "pinned MCP metadata input", Path: "internal/cli/schema_mcp_metadata.json"},
		{Name: "reviewed MCP service disposition input", Path: "internal/cli/schema_mcp_service_review.json"},
		{Name: "reviewed parameter binding input", Path: "internal/cli/schema_parameter_bindings.json"},
		{Name: "reviewed command exclusion input", Path: "internal/cli/schema_command_exclusions.json"},
	}
	if strings.TrimSpace(surfacePath) != "" {
		inputs = append(inputs, outputguard.Input{Name: "deprecated Registry compatibility input", Path: surfacePath})
	}
	if err := outputguard.Validate(rootPath, inputs, []outputguard.Target{{Name: "--output", Path: outputPath, Directory: true}}); err != nil {
		return err
	}
	return outputguard.ValidateRepoTargetAllowlist(rootPath,
		outputguard.Target{Name: "--output", Path: outputPath, Directory: true},
		"internal/cli/schema_catalog",
	)
}

// generateSchemaCatalog consumes the cli package's reviewed registry API. It
// deliberately does not decode command identity itself: the compatibility
// --surface flag is validated against the embedded registry and can never
// replace it as an input source.
func generateSchemaCatalog(root *cobra.Command, surfacePath, outputPath string) error {
	return generateSchemaCatalogWithResolver(root, surfacePath, outputPath, cli.ResolveSchemaBuild)
}

type schemaBuildResolver func(*cobra.Command) (cli.ResolvedSchemaBuild, error)

// generateSchemaCatalogWithResolver exists to make the single-resolution
// contract observable in tests. Production passes cli.ResolveSchemaBuild; the
// returned Effective/Bound/SchemaRegistry views then travel together through
// every gate and the final serializer.
func generateSchemaCatalogWithResolver(root *cobra.Command, surfacePath, outputPath string, resolve schemaBuildResolver) error {
	if root == nil {
		return fmt.Errorf("schema source root is nil")
	}
	if resolve == nil {
		return fmt.Errorf("schema build resolver is nil")
	}
	if err := validateDeprecatedSurface(surfacePath); err != nil {
		return err
	}
	if err := cli.ValidateEmbeddedSchemaParameterBindings(); err != nil {
		return fmt.Errorf("validate reviewed parameter binding input: %w", err)
	}

	resolved, err := resolve(root)
	if err != nil {
		return fmt.Errorf("resolve final Schema build: %w", err)
	}
	snapshot, err := cli.BuildSchemaCatalogSnapshot(resolved, cli.SchemaCatalogBuildOptions{
		RegistryHash: resolved.RegistryHash(),
	})
	if err != nil {
		return err
	}
	if err := writeSchemaCatalogShards(snapshot, outputPath); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "generated schema catalog: output=%s registry_commands=%d tools=%d products=%d registry_hash=%s source_hash=%s\n",
		outputPath, resolved.CommandCount(), len(snapshot.Tools), countSchemaCatalogProducts(snapshot), snapshot.SurfaceHash, snapshot.SourceHash)
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
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
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
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

func validateDeprecatedSurface(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read deprecated -surface compatibility input: %w", err)
	}
	if _, err := cli.ValidateCommandRegistrySource(data); err != nil {
		return fmt.Errorf("validate deprecated -surface compatibility input: %w", err)
	}
	return nil
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
