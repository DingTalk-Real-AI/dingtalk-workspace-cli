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

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
)

// packageCLIAssembledDelivery holds the ResolveSchemaBuild dump installed by
// TestMain for package-cli tests (cannot import app).
var packageCLIAssembledDelivery *loadedSchemaCatalog

// TestMain assembles a fresh Catalog through cmd_schema_catalog (ResolveSchemaBuild)
// and installs it as the package-cli delivery source. Package cli cannot import
// internal/app (cycle); the generator subprocess owns the app root factory.
// Production binaries never use this path — only RegisterSchemaSourceRoot from app.
func TestMain(m *testing.M) {
	cleanup, err := installAssembledSchemaDeliveryForPackageCLITests()
	if err != nil {
		fmt.Fprintf(os.Stderr, "schema package-cli TestMain: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func installAssembledSchemaDeliveryForPackageCLITests() (func(), error) {
	noop := func() {}
	repoRoot, err := repoRootFromCLIPackage()
	if err != nil {
		return noop, err
	}
	tmp, err := os.MkdirTemp("", "dws-cli-schema-test-*")
	if err != nil {
		return noop, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	outDir := filepath.Join(tmp, "schema_catalog")
	// On Windows, go build -o without .exe writes foo.exe; exec.Command must
	// use the same path or TestMain fails during go test -list / coverage gate.
	generator := filepath.Join(tmp, "cmd_schema_catalog")
	if runtime.GOOS == "windows" {
		generator += ".exe"
	}
	build := exec.Command("go", "build", "-o", generator, "./internal/generator/cmd_schema_catalog")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		cleanup()
		return noop, fmt.Errorf("build cmd_schema_catalog: %w\n%s", err, out)
	}
	run := exec.Command(generator, "-root", repoRoot, "-output", outDir, "-meta-index", filepath.Join(tmp, "schema_meta_index.gob"))
	run.Dir = repoRoot
	if out, err := run.CombinedOutput(); err != nil {
		cleanup()
		return noop, fmt.Errorf("run cmd_schema_catalog: %w\n%s", err, out)
	}
	envelope, err := os.ReadFile(filepath.Join(outDir, "catalog.json"))
	if err != nil {
		cleanup()
		return noop, err
	}
	snapshot, err := mergeSchemaCatalogDump(envelope, filepath.Join(outDir, "tools"))
	if err != nil {
		cleanup()
		return noop, fmt.Errorf("merge schema catalog dump: %w", err)
	}
	loaded, err := loadSchemaCatalogSnapshot(snapshot)
	if err != nil {
		cleanup()
		return noop, fmt.Errorf("load schema catalog dump: %w", err)
	}
	packageCLIAssembledDelivery = &loaded
	restorePackageCLISchemaDeliveryHook = restorePackageCLISchemaDeliveryForTest
	restorePackageCLISchemaDeliveryForTest()
	return cleanup, nil
}

// mergeSchemaCatalogDump re-merges a cmd_schema_catalog dump (catalog.json
// envelope plus per-product tools shards) into the single-document snapshot
// shape. Test-only: production delivery never reads a dump, and the generator
// owns the shard writer types.
func mergeSchemaCatalogDump(envelopeJSON []byte, toolsDir string) (SchemaCatalogSnapshot, error) {
	var snapshot SchemaCatalogSnapshot
	if err := decodeStrictSchemaJSON(envelopeJSON, &snapshot); err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("decode schema catalog.json: %w", err)
	}
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("read schema catalog tools directory: %w", err)
	}
	tools := make(map[string]map[string]any, len(entries)*8)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(toolsDir, entry.Name()))
		if readErr != nil {
			return SchemaCatalogSnapshot{}, fmt.Errorf("read schema catalog shard %s: %w", entry.Name(), readErr)
		}
		var shard struct {
			Product string                    `json:"product"`
			Tools   map[string]map[string]any `json:"tools"`
		}
		if err := decodeStrictSchemaJSON(data, &shard); err != nil {
			return SchemaCatalogSnapshot{}, fmt.Errorf("decode schema catalog shard %s: %w", entry.Name(), err)
		}
		for canonical, spec := range shard.Tools {
			tools[canonical] = spec
		}
	}
	snapshot.Tools = tools
	return snapshot, nil
}

func restorePackageCLISchemaDeliveryForTest() {
	if packageCLIAssembledDelivery == nil {
		return
	}
	loaded := *packageCLIAssembledDelivery
	storeSchemaSourceRootFn(func() *cobra.Command {
		return &cobra.Command{Use: "dws"}
	})
	assembleDeliverySchemaCatalogFn = func(*cobra.Command) (loadedSchemaCatalog, error) {
		return loaded, nil
	}
	resetDeliverySchemaCatalogStateForTest()
	metaByCLIPathOnce = sync.Once{}
	metaByCLIPath = nil
	runtimeDeliverySchemaMetaIndexErr = nil
	runtimeDeliverySchemaMetaIndexLazyCount.Store(0)
}

func repoRootFromCLIPackage() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("repo root %q: %w", root, err)
	}
	return root, nil
}
