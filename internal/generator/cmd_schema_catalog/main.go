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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

type commandSurfaceSnapshot struct {
	Version  int                     `json:"version"`
	Products []commandSurfaceProduct `json:"products"`
}

type commandSurfaceProduct struct {
	ID    string               `json:"id"`
	Tools []commandSurfaceTool `json:"tools"`
}

type commandSurfaceTool struct {
	CanonicalPath   string   `json:"canonical_path"`
	SourceProductID string   `json:"source_product_id,omitempty"`
	CLIPath         string   `json:"cli_path"`
	Aliases         []string `json:"aliases,omitempty"`
}

func main() {
	var surfacePath string
	var outputPath string
	flag.StringVar(&surfacePath, "surface", "internal/cli/schema_command_surface.json", "Reviewed public command-surface snapshot")
	flag.StringVar(&outputPath, "output", "internal/cli/schema_catalog.json", "Output embedded schema catalog")
	flag.Parse()

	surface, allowed, surfaceHash, err := loadSurface(surfacePath)
	if err != nil {
		fail(err)
	}
	root := app.NewRootCommand()
	if err := cli.ValidateEmbeddedRuntimeSchemaCompleteness(root); err != nil {
		fail(fmt.Errorf("validate reverse command-tree completeness: %w", err))
	}
	snapshot, err := cli.BuildSchemaCatalogSnapshot(root, cli.SchemaCatalogBuildOptions{
		AllowedCanonicalPaths: allowed,
		SurfaceHash:           surfaceHash,
	})
	if err != nil {
		fail(err)
	}
	if err := cli.ValidateSchemaCatalogDeliveryCompleteness(root, snapshot); err != nil {
		fail(fmt.Errorf("validate final Catalog delivery completeness: %w", err))
	}
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		fail(fmt.Errorf("encode catalog: %w", err))
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fail(fmt.Errorf("create output directory: %w", err))
	}
	if err := os.WriteFile(outputPath, append(encoded, '\n'), 0o644); err != nil {
		fail(fmt.Errorf("write catalog: %w", err))
	}
	_, _ = fmt.Fprintf(os.Stderr, "generated schema catalog: output=%s products=%d tools=%d surface_hash=%s source_hash=%s\n",
		outputPath, len(surface.Products), len(snapshot.Tools), snapshot.SurfaceHash, snapshot.SourceHash)
}

func loadSurface(path string) (commandSurfaceSnapshot, map[string]bool, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return commandSurfaceSnapshot{}, nil, "", fmt.Errorf("read surface: %w", err)
	}
	var snapshot commandSurfaceSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return commandSurfaceSnapshot{}, nil, "", fmt.Errorf("decode surface: %w", err)
	}
	if snapshot.Version != 1 {
		return commandSurfaceSnapshot{}, nil, "", fmt.Errorf("unsupported surface version %d", snapshot.Version)
	}
	allowed := map[string]bool{}
	rows := make([]string, 0)
	for _, product := range snapshot.Products {
		productID := strings.TrimSpace(product.ID)
		for _, tool := range product.Tools {
			canonical := strings.TrimSpace(tool.CanonicalPath)
			if canonical == "" {
				return commandSurfaceSnapshot{}, nil, "", fmt.Errorf("surface tool %q has no canonical_path", tool.CLIPath)
			}
			if allowed[canonical] {
				return commandSurfaceSnapshot{}, nil, "", fmt.Errorf("duplicate canonical path %s", canonical)
			}
			allowed[canonical] = true
			aliases := append([]string(nil), tool.Aliases...)
			sort.Strings(aliases)
			rows = append(rows, productID+"\x00"+canonical+"\x00"+strings.TrimSpace(tool.SourceProductID)+"\x00"+strings.TrimSpace(tool.CLIPath)+"\x00"+strings.Join(aliases, "\x00"))
		}
	}
	sort.Strings(rows)
	sum := sha256.Sum256([]byte(strings.Join(rows, "\n")))
	return snapshot, allowed, "sha256:" + hex.EncodeToString(sum[:]), nil
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
