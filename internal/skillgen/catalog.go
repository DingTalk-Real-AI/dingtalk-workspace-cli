// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package skillgen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
)

const defaultFixturePath = "test/golden/canonical_cli/testdata/catalog_fixture.json"
const defaultSnapshotPath = generatedDocsSchemaRoot + "/catalog.json"

type CatalogSource string

const (
	CatalogSourceFixture  CatalogSource = "fixture"
	CatalogSourceEnv      CatalogSource = "env"
	CatalogSourceSnapshot CatalogSource = "snapshot"
)

var newEnvironmentLoader = cli.NewEnvironmentLoader

func LoadCatalog(ctx context.Context, fixturePath string) (ir.Catalog, error) {
	return LoadCatalogWithSource(ctx, string(CatalogSourceEnv), fixturePath)
}

func LoadCatalogWithSource(ctx context.Context, source string, catalogPath string) (ir.Catalog, error) {
	switch normalized := normalizeCatalogSource(source); normalized {
	case string(CatalogSourceFixture):
		resolvedPath, err := resolveFixturePath(catalogPath)
		if err != nil {
			return ir.Catalog{}, err
		}
		return cli.FixtureLoader{Path: resolvedPath}.Load(ctx)
	case string(CatalogSourceSnapshot):
		resolvedPath, err := resolveSnapshotPath(catalogPath)
		if err != nil {
			return ir.Catalog{}, err
		}
		return cli.FixtureLoader{Path: resolvedPath}.Load(ctx)
	case string(CatalogSourceEnv):
		loader := newEnvironmentLoader()
		loader.RequireLiveCatalog = true
		loader.ForceRefreshAll = true
		return loader.Load(ctx)
	default:
		return ir.Catalog{}, fmt.Errorf("unsupported catalog source %q: must be one of fixture, env, snapshot", normalized)
	}
}

func normalizeCatalogSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return string(CatalogSourceEnv)
	}
	return source
}

func resolveFixturePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		if envFixture, ok := os.LookupEnv(cli.CatalogFixtureEnv); ok && strings.TrimSpace(envFixture) != "" {
			path = strings.TrimSpace(envFixture)
		} else {
			path = defaultFixturePath
		}
	}
	return resolveCatalogPath(path)
}

func resolveSnapshotPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultSnapshotPath
	}
	return resolveCatalogPath(path)
}

func resolveCatalogPath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return filepath.Join(cwd, path), nil
}
