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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
)

func realHomeCacheDir(t *testing.T, pattern string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	// Cache bases must live under symlink-free ancestry: the platform walk
	// opens each component with O_NOFOLLOW, and t.TempDir() on macOS sits
	// behind the /var symlink.
	dir, err := os.MkdirTemp(home, pattern)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func seedCorruptSharedCache(t *testing.T, base, editionHex string) string {
	t.Helper()
	sharedV1 := filepath.Join(base, "dws", "schema", editionHex, "v1")
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
	return sharedV1
}

// TestCrossPlatformCoverageRepairFallsBackToUserCacheWhenSharedUnwritable
// covers the root-owned read-only shared cache: corrupted artifacts under a
// base this process cannot lock must not force every process back to live
// assembly. The first repair publishes into the per-user cache, and a later
// process reuses that repair from the per-user cache.
func TestCrossPlatformCoverageRepairFallsBackToUserCacheWhenSharedUnwritable(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()

	edition := "open"
	digest := sha256.Sum256([]byte(edition))
	editionHex := hex.EncodeToString(digest[:])

	sharedBase := realHomeCacheDir(t, ".dws-shared-fallback-")
	sharedV1 := seedCorruptSharedCache(t, sharedBase, editionHex)
	// Root-owned read-only surrogate: lock creation fails with a non-timeout
	// error, which is the production fallback trigger.
	denySharedV1Writes(t, sharedV1)
	t.Setenv("DWS_SCHEMA_CACHE_SHARED_DIR", sharedBase)

	userBase := realHomeCacheDir(t, ".dws-user-fallback-")
	schemacache.UseUserCacheDirForTest(t, userBase)

	goos, goarch := coverageCacheGOOSARCH()
	options := SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: edition, GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true }, Counters: &schemacache.Counters{},
	}
	if err := RegisterSchemaCacheOptions(options); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	runtimeDeliveryLiveCatalog.Store(nil)
	t.Cleanup(func() { runtimeDeliveryLiveCatalog.Store(nil) })

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

// TestCrossPlatformCoverageRepairStaysLiveWhenUserCacheUnavailable covers the
// fallback's failure leg: with the shared cache unwritable AND no usable
// per-user cache, the repair keeps authoritative live-only behavior and never
// writes anything.
func TestCrossPlatformCoverageRepairStaysLiveWhenUserCacheUnavailable(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()

	edition := "open"
	digest := sha256.Sum256([]byte(edition))
	editionHex := hex.EncodeToString(digest[:])

	sharedBase := realHomeCacheDir(t, ".dws-shared-nouser-")
	sharedV1 := seedCorruptSharedCache(t, sharedBase, editionHex)
	denySharedV1Writes(t, sharedV1)
	t.Setenv("DWS_SCHEMA_CACHE_SHARED_DIR", sharedBase)

	// A regular file as the user cache base: the per-user open must fail.
	userBaseFile := filepath.Join(realHomeCacheDir(t, ".dws-user-file-"), "not-a-dir")
	if err := os.WriteFile(userBaseFile, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	schemacache.UseUserCacheDirForTest(t, userBaseFile)

	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: edition, GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true }, Counters: &schemacache.Counters{},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	runtimeDeliveryLiveCatalog.Store(nil)
	t.Cleanup(func() { runtimeDeliveryLiveCatalog.Store(nil) })

	r := activeSchemaCacheRuntime()
	value, _, err := repairSchemaCache(r, func() (any, error) {
		return nil, errors.New("corrupt shared artifacts")
	})
	if err != nil || value != nil {
		t.Fatalf("unusable user cache must keep live-only repair: value=%v err=%v", value, err)
	}
	if repaired, readErr := os.ReadFile(filepath.Join(sharedV1, "meta.cache")); readErr != nil || string(repaired) != "corrupt" {
		t.Fatalf("shared cache must stay untouched: err=%v content=%q", readErr, repaired)
	}
}

// TestCrossPlatformCoverageRepairTimeoutKeepsSharedRepairOwner covers the
// timeout leg: while another handle holds the shared rebuild lock, the repair
// stays live-only instead of splitting into the per-user cache.
func TestCrossPlatformCoverageRepairTimeoutKeepsSharedRepairOwner(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()

	edition := "open"
	digest := sha256.Sum256([]byte(edition))
	editionHex := hex.EncodeToString(digest[:])

	sharedBase := realHomeCacheDir(t, ".dws-shared-timeout-")
	seedCorruptSharedCache(t, sharedBase, editionHex)
	t.Setenv("DWS_SCHEMA_CACHE_SHARED_DIR", sharedBase)

	userBase := realHomeCacheDir(t, ".dws-user-timeout-")
	schemacache.UseUserCacheDirForTest(t, userBase)

	holder, err := schemacache.Open(edition)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	lock, err := holder.AcquireLock(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("hold shared lock: %v", err)
	}
	t.Cleanup(func() { _ = lock.Release() })

	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: edition, GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true }, Counters: &schemacache.Counters{},
		LockTimeout: 150 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	runtimeDeliveryLiveCatalog.Store(nil)
	t.Cleanup(func() { runtimeDeliveryLiveCatalog.Store(nil) })

	r := activeSchemaCacheRuntime()
	value, _, err := repairSchemaCache(r, func() (any, error) {
		return nil, errors.New("corrupt shared artifacts")
	})
	if err != nil || value != nil {
		t.Fatalf("lock timeout must keep live-only repair: value=%v err=%v", value, err)
	}
	if _, statErr := os.Stat(filepath.Join(userBase, "dws")); !os.IsNotExist(statErr) {
		t.Fatalf("lock timeout must not fall back to the per-user cache: %v", statErr)
	}
}

// TestCrossPlatformCoverageRepairFallbackUserCacheDirError covers the
// userOnly open leg where the user cache base lookup itself fails.
func TestCrossPlatformCoverageRepairFallbackUserCacheDirError(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()

	edition := "open"
	digest := sha256.Sum256([]byte(edition))
	editionHex := hex.EncodeToString(digest[:])

	sharedBase := realHomeCacheDir(t, ".dws-shared-ucderr-")
	sharedV1 := seedCorruptSharedCache(t, sharedBase, editionHex)
	denySharedV1Writes(t, sharedV1)
	t.Setenv("DWS_SCHEMA_CACHE_SHARED_DIR", sharedBase)
	schemacache.UseUserCacheDirErrorForTest(t)

	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: edition, GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true }, Counters: &schemacache.Counters{},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	runtimeDeliveryLiveCatalog.Store(nil)
	t.Cleanup(func() { runtimeDeliveryLiveCatalog.Store(nil) })

	value, _, err := repairSchemaCache(activeSchemaCacheRuntime(), func() (any, error) {
		return nil, errors.New("corrupt shared artifacts")
	})
	if err != nil || value != nil {
		t.Fatalf("user cache dir failure must keep live-only repair: value=%v err=%v", value, err)
	}
	if repaired, readErr := os.ReadFile(filepath.Join(sharedV1, "meta.cache")); readErr != nil || string(repaired) != "corrupt" {
		t.Fatalf("shared cache must stay untouched: err=%v content=%q", readErr, repaired)
	}
}

// TestCrossPlatformCoverageRepairFallbackSurfacesAssemblyError covers the
// fallback leg where the per-user switch succeeds but live assembly itself
// fails: the assembly error must surface instead of a silent cache hit.
func TestCrossPlatformCoverageRepairFallbackSurfacesAssemblyError(t *testing.T) {
	restorePackageCLISchemaDeliveryForTest()

	edition := "open"
	digest := sha256.Sum256([]byte(edition))
	editionHex := hex.EncodeToString(digest[:])

	sharedBase := realHomeCacheDir(t, ".dws-shared-asmerr-")
	sharedV1 := seedCorruptSharedCache(t, sharedBase, editionHex)
	denySharedV1Writes(t, sharedV1)
	t.Setenv("DWS_SCHEMA_CACHE_SHARED_DIR", sharedBase)
	schemacache.UseUserCacheDirForTest(t, realHomeCacheDir(t, ".dws-user-asmerr-"))

	// Unregister the source root so live assembly fails closed during the
	// fallback; the cleanup restores the package delivery.
	RegisterSchemaSourceRoot(nil)
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)

	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: edition, GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true }, Counters: &schemacache.Counters{},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	runtimeDeliveryLiveCatalog.Store(nil)
	t.Cleanup(func() { runtimeDeliveryLiveCatalog.Store(nil) })

	value, _, err := repairSchemaCache(activeSchemaCacheRuntime(), func() (any, error) {
		return nil, errors.New("corrupt shared artifacts")
	})
	if !errors.Is(err, errSchemaSourceRootNotRegistered) || value != nil {
		t.Fatalf("fallback assembly failure must surface: value=%v err=%v", value, err)
	}
	if repaired, readErr := os.ReadFile(filepath.Join(sharedV1, "meta.cache")); readErr != nil || string(repaired) != "corrupt" {
		t.Fatalf("shared cache must stay untouched: err=%v content=%q", readErr, repaired)
	}
}
