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

//go:build (darwin || linux) && (amd64 || arm64)

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
)

// TestCrossPlatformCoverageRepairFallsBackToUserCacheWhenSharedUnwritable
// covers the root-owned read-only shared cache: corrupted artifacts under a
// base this process cannot lock must not force every process back to live
// assembly. The first repair publishes into the per-user cache, and a later
// process reuses that repair from the per-user cache.
func TestCrossPlatformCoverageRepairFallsBackToUserCacheWhenSharedUnwritable(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	runtimeDeliveryLiveCatalog.Store(nil)
	t.Cleanup(func() { runtimeDeliveryLiveCatalog.Store(nil) })

	edition := "open"
	digest := sha256.Sum256([]byte(edition))
	editionHex := hex.EncodeToString(digest[:])

	realHomeDir := func(pattern string) string {
		t.Helper()
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		// Cache bases must live under symlink-free ancestry: the platform
		// walk opens each component with O_NOFOLLOW, and t.TempDir() on
		// macOS sits behind the /var symlink.
		dir, err := os.MkdirTemp(home, pattern)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		return dir
	}
	sharedBase := realHomeDir(".dws-shared-fallback-")
	sharedV1 := filepath.Join(sharedBase, "dws", "schema", editionHex, "v1")
	if err := os.MkdirAll(sharedV1, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"meta.cache", "registry.shards.cache", "payloads.shards.cache"} {
		if err := os.WriteFile(filepath.Join(sharedV1, name), []byte("corrupt"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sharedV1, "identity.json"), []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Root-owned read-only surrogate: lock creation fails with EACCES — a
	// non-timeout failure, which is the production fallback trigger.
	if err := os.Chmod(sharedV1, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sharedV1, 0o700) })
	t.Setenv("DWS_SCHEMA_CACHE_SHARED_DIR", sharedBase)

	userBase := realHomeDir(".dws-user-fallback-")
	schemacache.UseUserCacheDirForTest(t, userBase)

	goos, goarch := coverageCacheGOOSARCH()
	options := SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: edition, GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}
	if err := RegisterSchemaCacheOptions(options); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })

	// First process: shared verification fails and the shared base cannot be
	// locked, so the repair publishes into the per-user cache instead.
	first := activeSchemaCacheRuntime()
	if first == nil {
		t.Fatal("registered runtime missing")
	}
	value, _, err := repairSchemaCache(first, func() (any, error) {
		return nil, errors.New("corrupt shared artifacts")
	})
	if err != nil {
		t.Fatalf("repair with unwritable shared cache: %v", err)
	}
	if value != nil {
		t.Fatalf("repair with failing recheck must return the live catalog, got %#v", value)
	}
	userV1 := filepath.Join(userBase, "dws", "schema", editionHex, "v1")
	for _, name := range []string{"identity.json", "meta.cache", "registry.shards.cache", "payloads.shards.cache"} {
		if _, statErr := os.Stat(filepath.Join(userV1, name)); statErr != nil {
			t.Fatalf("per-user repair artifact %s missing: %v", name, statErr)
		}
	}
	if repaired, readErr := os.ReadFile(filepath.Join(sharedV1, "meta.cache")); readErr != nil || string(repaired) != "corrupt" {
		t.Fatalf("read-only shared cache must stay untouched: err=%v content=%q", readErr, repaired)
	}

	// Later process: same shared corruption, but it reuses the per-user repair
	// — the recheck succeeds against the user cache without live assembly.
	// Clear the process-global delivery state to model a fresh process.
	runtimeDeliveryLiveCatalog.Store(nil)
	second := newSchemaCacheRuntime(options)
	value, _, err = repairSchemaCache(second, func() (any, error) {
		meta, metaErr := second.readMeta()
		if metaErr != nil {
			return nil, metaErr
		}
		return meta, nil
	})
	if err != nil {
		t.Fatalf("second-process repair: %v", err)
	}
	meta, ok := value.(schemaruntime.DecodedSchemaMeta)
	if !ok || len(meta.LocatorProductByPath) == 0 {
		t.Fatalf("second process did not hit the per-user repair: %#v", value)
	}
}

// TestCrossPlatformCoverageRepairFallbackErrorPaths closes out the remaining
// fallback branches: counters wiring on the per-user open, a per-user cache
// that cannot be opened at all, and a failing live catalog inside the
// per-user fallback arm (no cache error may mask it).
func TestCrossPlatformCoverageRepairFallbackErrorPaths(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	runtimeDeliveryLiveCatalog.Store(nil)
	t.Cleanup(func() { runtimeDeliveryLiveCatalog.Store(nil) })

	edition := "open"
	goos, goarch := coverageCacheGOOSARCH()
	realHomeDir := func(pattern string) string {
		t.Helper()
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		dir, err := os.MkdirTemp(home, pattern)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		return dir
	}

	t.Run("switch wires counters into the per-user open", func(t *testing.T) {
		userBase := realHomeDir(".dws-user-counters-")
		schemacache.UseUserCacheDirForTest(t, userBase)
		options := SchemaCacheOptions{
			Enabled: true, AllowGenerate: true, Edition: edition, GOOS: goos, GOARCH: goarch,
			RuntimeEligible: func() bool { return true },
			Counters:        &schemacache.Counters{},
		}
		if err := RegisterSchemaCacheOptions(options); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
		if !newSchemaCacheRuntime(options).switchToUserCache() {
			t.Fatal("switch to an openable per-user cache must succeed")
		}
	})

	t.Run("per-user cache unavailable keeps shared handling live-only", func(t *testing.T) {
		// The real os.UserCacheDir seam must fail: neither XDG_CACHE_HOME nor
		// HOME may resolve a base, so the user-only open errors and the
		// switch reports false instead of adopting a broken backend.
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("HOME", "")
		options := SchemaCacheOptions{
			Enabled: true, AllowGenerate: true, Edition: edition, GOOS: goos, GOARCH: goarch,
			RuntimeEligible: func() bool { return true },
		}
		if err := RegisterSchemaCacheOptions(options); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
		r := newSchemaCacheRuntime(options)
		if r.switchToUserCache() {
			t.Fatal("switch must fail when no per-user cache directory resolves")
		}
		if r.userCacheBackend() != nil {
			t.Fatal("failed switch must not adopt a backend")
		}
	})

	t.Run("unusable per-user base fails the open", func(t *testing.T) {
		// A base that is not a directory must fail inside the user-only
		// openCacheDirectory walk, not be silently adopted.
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		notADir := filepath.Join(home, ".dws-user-not-a-dir-")
		if err := os.WriteFile(notADir, []byte("file"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(notADir) })
		schemacache.UseUserCacheDirForTest(t, notADir)
		options := SchemaCacheOptions{
			Enabled: true, AllowGenerate: true, Edition: edition, GOOS: goos, GOARCH: goarch,
			RuntimeEligible: func() bool { return true },
		}
		if err := RegisterSchemaCacheOptions(options); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
		r := newSchemaCacheRuntime(options)
		if r.switchToUserCache() {
			t.Fatal("switch must fail when the per-user base is not a directory")
		}
		if r.userCacheBackend() != nil {
			t.Fatal("failed switch must not adopt a backend")
		}
	})

	t.Run("failing live catalog surfaces from the fallback arm", func(t *testing.T) {
		digest := sha256.Sum256([]byte(edition))
		editionHex := hex.EncodeToString(digest[:])
		sharedBase := realHomeDir(".dws-shared-liveerr-")
		sharedV1 := filepath.Join(sharedBase, "dws", "schema", editionHex, "v1")
		if err := os.MkdirAll(sharedV1, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"meta.cache", "registry.shards.cache", "payloads.shards.cache", "identity.json"} {
			if err := os.WriteFile(filepath.Join(sharedV1, name), []byte("corrupt"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chmod(sharedV1, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(sharedV1, 0o700) })
		t.Setenv("DWS_SCHEMA_CACHE_SHARED_DIR", sharedBase)
		userBase := realHomeDir(".dws-user-liveerr-")
		schemacache.UseUserCacheDirForTest(t, userBase)

		options := SchemaCacheOptions{
			Enabled: true, AllowGenerate: true, Edition: edition, GOOS: goos, GOARCH: goarch,
			RuntimeEligible: func() bool { return true },
		}
		if err := RegisterSchemaCacheOptions(options); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })

		liveErr := errors.New("forced live catalog err")
		previousAssemble := assembleDeliverySchemaCatalogFn
		assembleDeliverySchemaCatalogFn = func(*cobra.Command) (loadedSchemaCatalog, error) {
			return loadedSchemaCatalog{}, liveErr
		}
		t.Cleanup(func() { assembleDeliverySchemaCatalogFn = previousAssemble })
		resetDeliverySchemaCatalogStateForTest()
		t.Cleanup(resetDeliverySchemaCatalogStateForTest)
		runtimeDeliveryLiveCatalog.Store(nil)

		r := activeSchemaCacheRuntime()
		if r == nil {
			t.Fatal("registered runtime missing")
		}
		value, _, err := repairSchemaCache(r, func() (any, error) {
			return nil, errors.New("corrupt shared artifacts")
		})
		if !errors.Is(err, liveErr) {
			t.Fatalf("fallback arm must surface the live catalog error, got %v", err)
		}
		if value != nil {
			t.Fatalf("failing live catalog must not return a value: %#v", value)
		}
	})
}
