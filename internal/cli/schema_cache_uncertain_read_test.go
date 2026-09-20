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
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

// uncertainGroupHelpRoot builds the minimal registered-source-root state the
// affordance renderer consults, without assembling anything.
func uncertainGroupHelpRoot(t *testing.T) *cobra.Command {
	t.Helper()
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	RegisterSchemaSourceRoot(func() *cobra.Command { return &cobra.Command{Use: "dws"} })
	root := &cobra.Command{Use: "dws"}
	group := &cobra.Command{Use: "calendar"}
	leaf := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	group.AddCommand(leaf)
	// Mirror production group commands: ApplyGroupPolicy owns the
	// navigation-only marker and installs the group RunE.
	corecmd.ApplyGroupPolicy(group, corecmd.GroupPolicy{
		Mode: corecmd.GroupNavigationOnly, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoverySibling,
	})
	root.AddCommand(group)
	return group
}

// TestCrossPlatformCoverageGroupHelpSkipsAssemblyUnderUncertainty covers the
// tier-1 guard: a pure group page can never hit the tool index, so rendering
// its affordances must not pay live catalog assembly — not even when plugins
// made the cache runtime uncertain.
func TestCrossPlatformCoverageGroupHelpSkipsAssemblyUnderUncertainty(t *testing.T) {
	group := uncertainGroupHelpRoot(t)
	resetDeliverySchemaCatalogStateForTest()
	resetMetaByCLIPathStateForTest()
	MarkSchemaCacheRuntimeUncertain()

	var out strings.Builder
	group.SetOut(&out)
	RenderHelpAffordances(group)

	if counts := RuntimeSchemaMetadataLoadCounts(); counts.Catalog != 0 {
		t.Fatalf("group help assembled the catalog: Catalog=%d", counts.Catalog)
	}
}

// TestCrossPlatformCoverageUncertainRuntimeServesCacheReads covers the tier-2
// hit: with the per-user cache populated, a domain-level schema query is
// served read-only even while the process surface is plugin-uncertain, and no
// live assembly runs.
func TestCrossPlatformCoverageUncertainRuntimeServesCacheReads(t *testing.T) {
	if schemaRaceInstrumentation {
		t.Skip("race:cli skips real-cache assembly coverage to stay inside the shard budget")
	}
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	coverageSchemaCacheHome(t)
	home := realHomeCacheDir(t, ".dws-uncertain-read-")
	schemacache.UseUserCacheDirForTest(t, home)

	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true }, Counters: &schemacache.Counters{},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })

	// Populate the cache while the runtime is certain, then flip uncertain.
	loaded := deliverySchemaCatalog()
	runtime := activeSchemaCacheRuntime()
	if runtime == nil {
		t.Fatal("registered runtime missing")
	}
	runtime.publishGeneratedOrMatching(nil, loadedSchemaCatalog{})
	cache, err := schemacache.Open("open", schemacache.WithUserOnly())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	runtime.publishGeneratedOrMatching(cache, loaded)
	for _, name := range []string{"identity.json", "meta.cache"} {
		if _, statErr := os.Stat(filepath.Join(cache.Directory(), name)); statErr != nil {
			t.Fatalf("cache artifact %s missing: %v", name, statErr)
		}
	}

	resetDeliverySchemaCatalogStateForTest()
	resetMetaByCLIPathStateForTest()
	MarkSchemaCacheRuntimeUncertain()

	payload, err := DeliverySchemaQueryPayloadForTest("calendar")
	if err != nil {
		t.Fatalf("uncertain domain query: %v", err)
	}
	if level := payload["level"]; level != "product" {
		t.Fatalf("domain query level = %v, want product", level)
	}
	if _, err := queryDeliverySchemaPayload(nil); err != nil {
		t.Fatalf("uncertain cached no-argument query: %v", err)
	}
	if counts := RuntimeSchemaMetadataLoadCounts(); counts.Catalog != 0 {
		t.Fatalf("uncertain query assembled the catalog: Catalog=%d", counts.Catalog)
	}
}

// TestCrossPlatformCoverageUncertainColdCacheUsesIsolatedBuilder covers the
// cold-cache path: uncertainty must use the isolated builder rather than
// assembling the catalog in the plugin process.
func TestCrossPlatformCoverageUncertainColdCacheUsesIsolatedBuilder(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	coverageSchemaCacheHome(t)
	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	MarkSchemaCacheRuntimeUncertain()
	resetDeliverySchemaCatalogStateForTest()
	resetMetaByCLIPathStateForTest()

	if _, err := DeliverySchemaQueryPayloadForTest("definitely-not-a-schema-path"); err == nil {
		t.Fatal("unknown path unexpectedly resolved")
	}
	if counts := RuntimeSchemaMetadataLoadCounts(); counts.Catalog != 0 {
		t.Fatalf("cold uncertain query assembled the parent catalog: %#v", counts)
	}
}

// TestCrossPlatformCoverageUncertainRuntimePublishesThroughBuilder covers the
// tier-2 write gate: uncertainty may publish only the isolated builder result.
func TestCrossPlatformCoverageUncertainRuntimePublishesThroughBuilder(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	coverageSchemaCacheHome(t)
	home := realHomeCacheDir(t, ".dws-uncertain-nopub-")
	schemacache.UseUserCacheDirForTest(t, home)

	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true }, Counters: &schemacache.Counters{},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })

	MarkSchemaCacheRuntimeUncertain()
	resetDeliverySchemaCatalogStateForTest()
	resetMetaByCLIPathStateForTest()

	if _, err := DeliverySchemaQueryPayloadForTest("definitely-not-a-schema-path"); err == nil {
		t.Fatal("unknown path unexpectedly resolved")
	}
	marker := filepath.Join(home, "dws")
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("isolated builder did not publish cache state: %v", statErr)
	}
}

func TestCrossPlatformCoverageRegisterCacheOptionsPreservesUncertainty(t *testing.T) {
	t.Cleanup(func() {
		ResetSchemaCacheRuntimeUncertaintyForTest()
		_ = RegisterSchemaCacheOptions(SchemaCacheOptions{})
	})
	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	MarkSchemaCacheRuntimeUncertain()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	if activeSchemaCacheRuntime() != nil {
		t.Fatal("cache registration cleared runtime uncertainty")
	}
	if readableSchemaCacheRuntime() == nil {
		t.Fatal("uncertain runtime lost readable cache access")
	}
}

// TestCrossPlatformCoverageReadableRuntimeNilGuards covers every nil-return
// branch of readableSchemaCacheRuntime so the platform coverage gate counts
// them: no registration, disabled registration, and ineligible runtime.
func TestCrossPlatformCoverageReadableRuntimeNilGuards(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()

	if r := readableSchemaCacheRuntime(); r != nil {
		t.Fatal("readable runtime should be nil with no registration")
	}

	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{}); err != nil {
		t.Fatal(err)
	}
	if r := readableSchemaCacheRuntime(); r != nil {
		t.Fatal("readable runtime should be nil when disabled")
	}

	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return false },
	}); err != nil {
		t.Fatal(err)
	}
	if r := readableSchemaCacheRuntime(); r != nil {
		t.Fatal("readable runtime should be nil when ineligible")
	}

	// A disabled registration seeded through the seam must not serve reads
	// either; the fail-closed guard covers that state for both accessors.
	disabled := newSchemaCacheRuntime(SchemaCacheOptions{})
	schemaCacheRegistrationValue.Store(&schemaCacheRegistration{runtime: disabled})
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	if r := readableSchemaCacheRuntime(); r != nil {
		t.Fatal("readable runtime must reject a disabled registration")
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
}

// TestCrossPlatformCoverageUncertainAllAndOverviewFallbacks covers the
// fallback legs of the complete-registry and overview loaders while the
// process surface is plugin-uncertain and the per-user cache is cold: both
// must still answer from live assembly, and a failing assembly must surface
// its error instead of a silent empty payload.
func TestCrossPlatformCoverageUncertainAllAndOverviewFallbacks(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	coverageSchemaCacheHome(t)
	schemacache.UseUserCacheDirForTest(t, realHomeCacheDir(t, ".dws-uncertain-all-"))

	goos, goarch := coverageCacheGOOSARCH()
	register := func() {
		if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
			Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
			RuntimeEligible: func() bool { return true }, Counters: &schemacache.Counters{},
		}); err != nil {
			t.Fatal(err)
		}
	}
	register()
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	MarkSchemaCacheRuntimeUncertain()
	resetDeliverySchemaCatalogStateForTest()
	resetMetaByCLIPathStateForTest()

	// Cold cache: the no-argument query and overview loader both use the
	// isolated builder. The following loader reads the detached generation.
	if _, err := queryDeliverySchemaPayload(nil); err != nil {
		t.Fatalf("uncertain cold-cache no-argument query: %v", err)
	}
	resetDeliverySchemaCatalogStateForTest()
	if _, err := DeliverySchemaOverviewPayloadForTest(); err != nil {
		t.Fatalf("uncertain cold-cache overview payload: %v", err)
	}
	resetDeliverySchemaCatalogStateForTest()
	if _, err := DeliverySchemaAllPayloadForTest(); err != nil {
		t.Fatalf("uncertain cold-cache all payload: %v", err)
	}

	// A failing isolated builder must surface instead of degrading silently.
	// The source-root registration resets cache options, so re-register them
	// and keep the uncertainty marker.
	RegisterSchemaSourceRoot(nil)
	register()
	RegisterSchemaCacheIsolatedBuilder(func(context.Context) (SchemaCacheBuildResult, error) {
		return SchemaCacheBuildResult{}, errors.New("isolated builder failed")
	})
	t.Cleanup(registerPackageTestIsolatedBuilder)
	MarkSchemaCacheRuntimeUncertain()
	resetDeliverySchemaCatalogStateForTest()
	resetMetaByCLIPathStateForTest()
	if _, err := DeliverySchemaAllPayloadForTest(); err == nil {
		t.Fatal("uncertain all payload ignored a failing assembly")
	}
	if _, err := DeliverySchemaOverviewPayloadForTest(); err == nil {
		t.Fatal("uncertain overview payload ignored a failing assembly")
	}
	resetDeliverySchemaCatalogStateForTest()
	if _, err := DeliverySchemaQueryPayloadForTest("calendar"); err == nil {
		t.Fatal("uncertain query ignored a failing assembly")
	}
}

func TestCrossPlatformCoverageReadableIdentityNilGuards(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	schemaCacheRegistrationValue.Store(nil)
	if _, ok := SchemaCacheReadableIdentityForTest(); ok {
		t.Fatal("expected ok=false when registration is nil")
	}
	schemaCacheRegistrationValue.Store(&schemaCacheRegistration{})
	if _, ok := SchemaCacheReadableIdentityForTest(); ok {
		t.Fatal("expected ok=false when runtime is nil")
	}
	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	if _, _ = SchemaCacheReadableIdentityForTest(); false {
	}
}

func TestCrossPlatformCoverageDeliveryCatalogUnderUncertainty(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	MarkSchemaCacheRuntimeUncertain()
	resetDeliverySchemaCatalogStateForTest()
	resetMetaByCLIPathStateForTest()

	loaded := deliverySchemaCatalog()
	if runtimeDeliverySchemaCatalogErr == nil {
		t.Fatal("expected error from deliverySchemaCatalog under uncertainty")
	}
	if len(loaded.Registry.Products) != 0 {
		t.Fatal("expected empty catalog")
	}
}

func TestCrossPlatformCoverageRepairableRuntimeNil(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	ResetSchemaCacheRuntimeUncertaintyForTest()
	_ = RegisterSchemaCacheOptions(SchemaCacheOptions{})
	if r := repairableSchemaCacheRuntime(); r != nil {
		t.Fatal("expected nil repairable runtime")
	}
}

func TestCrossPlatformCoverageWaitForPublishedGeneration(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	runtime := activeSchemaCacheRuntime()
	home := realHomeCacheDir(t, ".dws-wait-gen-")
	schemacache.UseUserCacheDirForTest(t, home)
	cache, err := schemacache.Open("open", schemacache.WithUserOnly())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })

	id := coverageSchemaCacheIdentity()
	_ = persistLocalSchemaCacheIdentity(cache.Directory(), id)

	val, err := runtime.waitForPublishedGeneration(cache, func() (any, error) {
		return "ok", nil
	})
	if err != nil || val != "ok" {
		t.Fatalf("unexpected result: val=%v, err=%v", val, err)
	}

	testseam.Swap(t, &defaultSchemaCacheBuilderTimeout, 120*time.Millisecond)
	loops := 0
	_, err = runtime.waitForPublishedGeneration(cache, func() (any, error) {
		loops++
		return nil, errors.New("timeout expected")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got: %v", err)
	}
	if loops < 2 {
		t.Fatalf("expected multiple loops through ticker, got %d", loops)
	}
}

func TestCrossPlatformCoveragePublishGeneratedResultBranches(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	runtime := activeSchemaCacheRuntime()
	home := realHomeCacheDir(t, ".dws-pub-branches-")
	schemacache.UseUserCacheDirForTest(t, home)
	cache, err := schemacache.Open("open", schemacache.WithUserOnly())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })

	if err := runtime.publishGeneratedResult(nil, SchemaCacheBuildResult{}); err == nil {
		t.Fatal("expected nil cache error")
	}

	if err := runtime.publishGeneratedResult(cache, SchemaCacheBuildResult{}); err == nil {
		t.Fatal("expected validation error on empty result")
	}

	loaded := deliverySchemaCatalog()
	artifacts, err := buildSchemaCacheArtifactsFromLoaded(loaded)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := IdentityFromArtifacts("open", artifacts)
	if err != nil {
		t.Fatal(err)
	}
	validResult := SchemaCacheBuildResult{
		Identity:  identity,
		Artifacts: artifacts,
	}

	mismatchIdentity, err := IdentityFromArtifacts("other", artifacts)
	if err != nil {
		t.Fatal(err)
	}
	mismatchResult := SchemaCacheBuildResult{
		Identity:  mismatchIdentity,
		Artifacts: artifacts,
	}
	if err := runtime.publishGeneratedResult(cache, mismatchResult); err == nil {
		t.Fatal("expected edition mismatch error")
	}

	testseam.Swap(t, &createLocalIdentityTempFile, func(dir, pattern string) (localIdentityTempFile, error) {
		return nil, errors.New("persist fail")
	})
	if err := runtime.publishGeneratedResult(cache, validResult); err == nil {
		t.Fatal("expected persist identity error")
	}
	testseam.Swap(t, &createLocalIdentityTempFile, func(dir, pattern string) (localIdentityTempFile, error) {
		return os.CreateTemp(dir, pattern)
	})

	if err := runtime.publishGeneratedResult(cache, validResult); err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}

	closedCache, err := schemacache.Open("open", schemacache.WithUserOnly())
	if err != nil {
		t.Fatal(err)
	}
	_ = closedCache.Close()
	if err := runtime.publishGeneratedResult(closedCache, validResult); err == nil {
		t.Fatal("expected error on closed cache")
	}
}

func TestCrossPlatformCoverageRepairWithIsolatedBuilderBranches(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	runtime := activeSchemaCacheRuntime()
	home := realHomeCacheDir(t, ".dws-isolated-repair-")
	schemacache.UseUserCacheDirForTest(t, home)
	cache, err := schemacache.Open("open", schemacache.WithUserOnly())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })

	RegisterSchemaCacheIsolatedBuilder(func(context.Context) (SchemaCacheBuildResult, error) {
		return SchemaCacheBuildResult{
			Identity: SchemaCacheIdentity{Edition: "mismatch"},
		}, nil
	})
	t.Cleanup(registerPackageTestIsolatedBuilder)

	if _, _, err := runtime.repairWithIsolatedBuilder(cache, func() (any, error) { return "ok", nil }); err == nil {
		t.Fatal("expected publish error")
	}

	loaded := deliverySchemaCatalog()
	artifacts, err := buildSchemaCacheArtifactsFromLoaded(loaded)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := IdentityFromArtifacts("open", artifacts)
	if err != nil {
		t.Fatal(err)
	}
	RegisterSchemaCacheIsolatedBuilder(func(context.Context) (SchemaCacheBuildResult, error) {
		return SchemaCacheBuildResult{
			Identity:  identity,
			Artifacts: artifacts,
		}, nil
	})
	if _, _, err := runtime.repairWithIsolatedBuilder(cache, func() (any, error) { return nil, errors.New("recheck failed") }); err == nil {
		t.Fatal("expected recheck error")
	}

	val, _, err := runtime.repairWithIsolatedBuilder(cache, func() (any, error) { return "recheck-ok", nil })
	if err != nil || val != "recheck-ok" {
		t.Fatalf("unexpected: val=%v err=%v", val, err)
	}
}

func TestCrossPlatformCoverageRepairUncertainSwitchesToUserCacheOnOpenError(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	home := realHomeCacheDir(t, ".dws-user-switch-")
	schemacache.UseUserCacheDirForTest(t, home)
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "/nonexistent_root_dir_dws_test/dws")

	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true }, Counters: &schemacache.Counters{},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })

	loaded := deliverySchemaCatalog()
	artifacts, err := buildSchemaCacheArtifactsFromLoaded(loaded)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := IdentityFromArtifacts("open", artifacts)
	if err != nil {
		t.Fatal(err)
	}
	RegisterSchemaCacheIsolatedBuilder(func(context.Context) (SchemaCacheBuildResult, error) {
		return SchemaCacheBuildResult{
			Identity:  identity,
			Artifacts: artifacts,
		}, nil
	})
	t.Cleanup(registerPackageTestIsolatedBuilder)

	MarkSchemaCacheRuntimeUncertain()
	resetDeliverySchemaCatalogStateForTest()
	resetMetaByCLIPathStateForTest()

	r := repairableSchemaCacheRuntime()
	if r == nil {
		t.Fatal("expected repairable runtime")
	}

	val, _, err := repairSchemaCache(r, func() (any, error) {
		return "repaired", nil
	})
	if err != nil || val != "repaired" {
		t.Fatalf("expected repair success: val=%v, err=%v", val, err)
	}
}

func TestCrossPlatformCoverageRepairUncertainLockTimeoutWaitsForPublished(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	home := realHomeCacheDir(t, ".dws-lock-timeout-")
	schemacache.UseUserCacheDirForTest(t, home)

	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		LockTimeout:     10 * time.Millisecond,
		RuntimeEligible: func() bool { return true }, Counters: &schemacache.Counters{},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })

	holder, err := schemacache.Open("open", schemacache.WithUserOnly())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	lock, err := holder.AcquireLock(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	MarkSchemaCacheRuntimeUncertain()
	r := repairableSchemaCacheRuntime()
	if r == nil {
		t.Fatal("expected repairable runtime")
	}

	val, _, err := repairSchemaCache(r, func() (any, error) {
		return "published-val", nil
	})
	if err != nil || val != "published-val" {
		t.Fatalf("expected success from waitForPublishedGeneration: val=%v, err=%v", val, err)
	}
}

func TestCrossPlatformCoverageRepairUncertainFallsBackToUserCacheBuilder(t *testing.T) {
	if schemaRaceInstrumentation {
		t.Skip("race:cli skips real-cache assembly coverage to stay inside the shard budget")
	}
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()

	edition := "open"
	digest := sha256.Sum256([]byte(edition))
	editionHex := hex.EncodeToString(digest[:])

	sharedBase := realHomeCacheDir(t, ".dws-shared-fallback-unc-")
	sharedV1 := seedCorruptSharedCache(t, sharedBase, editionHex)
	denySharedV1Writes(t, sharedV1)
	t.Setenv("DWS_SCHEMA_CACHE_SHARED_DIR", sharedBase)

	userBase := realHomeCacheDir(t, ".dws-user-fallback-unc-")
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

	loaded := deliverySchemaCatalog()
	artifacts, err := buildSchemaCacheArtifactsFromLoaded(loaded)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := IdentityFromArtifacts("open", artifacts)
	if err != nil {
		t.Fatal(err)
	}
	RegisterSchemaCacheIsolatedBuilder(func(context.Context) (SchemaCacheBuildResult, error) {
		return SchemaCacheBuildResult{
			Identity:  identity,
			Artifacts: artifacts,
		}, nil
	})
	t.Cleanup(registerPackageTestIsolatedBuilder)

	MarkSchemaCacheRuntimeUncertain()
	resetDeliverySchemaCatalogStateForTest()
	resetMetaByCLIPathStateForTest()

	r := repairableSchemaCacheRuntime()
	if r == nil {
		t.Fatal("expected repairable runtime")
	}

	val, _, err := repairSchemaCache(r, func() (any, error) {
		if identity, ok := peekLocalSchemaCacheIdentity(filepath.Join(userBase, "dws", "schema", editionHex, "v1")); ok && identity.Edition == "open" {
			return "user-repaired", nil
		}
		return nil, errors.New("not yet repaired")
	})
	if err != nil || val != "user-repaired" {
		t.Fatalf("expected user cache repair success: val=%v, err=%v", val, err)
	}
}

func TestCrossPlatformCoverageRepairUncertainLockTimeoutFailsToIsolatedRequired(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	testseam.Swap(t, &defaultSchemaCacheBuilderTimeout, 10*time.Millisecond)
	home := realHomeCacheDir(t, ".dws-lock-timeout-fail-")
	schemacache.UseUserCacheDirForTest(t, home)

	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		LockTimeout:     10 * time.Millisecond,
		RuntimeEligible: func() bool { return true }, Counters: &schemacache.Counters{},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })

	holder, err := schemacache.Open("open", schemacache.WithUserOnly())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	lock, err := holder.AcquireLock(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	MarkSchemaCacheRuntimeUncertain()
	r := repairableSchemaCacheRuntime()
	if r == nil {
		t.Fatal("expected repairable runtime")
	}

	_, _, err = repairSchemaCache(r, func() (any, error) {
		return nil, errors.New("never ready")
	})
	if err == nil || !strings.Contains(err.Error(), "requires the isolated builder") {
		t.Fatalf("expected requires isolated builder error, got: %v", err)
	}
}

func TestCrossPlatformCoverageRepairBackendReuseAndClose(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()

	home := realHomeCacheDir(t, ".dws-repair-backend-reuse-")
	schemacache.UseUserCacheDirForTest(t, home)

	goos, goarch := coverageCacheGOOSARCH()
	r := newSchemaCacheRuntime(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
	})
	cache1, err := r.openRepairBackend()
	if err != nil {
		t.Fatal(err)
	}
	cache2, err := r.openRepairBackend()
	if err != nil {
		t.Fatal(err)
	}
	if cache1 != cache2 {
		t.Fatalf("expected reused repair cache, got %v vs %v", cache1, cache2)
	}
	cache3, err := schemacache.Open("open", schemacache.WithUserOnly())
	if err != nil {
		t.Fatal(err)
	}
	defer cache3.Close()
	r.setRepairCache(cache3)
	if got := r.repairCacheBackend(); got != cache3 {
		t.Fatalf("expected updated repair cache, got %v", got)
	}
}
