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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
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
	if counts := RuntimeSchemaMetadataLoadCounts(); counts.Catalog != 0 {
		t.Fatalf("uncertain query assembled the catalog: Catalog=%d", counts.Catalog)
	}
}

// TestCrossPlatformCoverageUncertainColdCacheStillAssembles covers the tier-2
// fallback leg: with no populated cache, uncertainty must not wedge schema
// queries — the live catalog still assembles and answers.
func TestCrossPlatformCoverageUncertainColdCacheStillAssembles(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	MarkSchemaCacheRuntimeUncertain()
	resetDeliverySchemaCatalogStateForTest()
	resetMetaByCLIPathStateForTest()

	if _, err := DeliverySchemaQueryPayloadForTest("definitely-not-a-schema-path"); err == nil {
		t.Fatal("unknown path unexpectedly resolved")
	}
	if counts := RuntimeSchemaMetadataLoadCounts(); counts.Catalog == 0 {
		t.Fatal("cold uncertain query never assembled the live catalog")
	}
}

// TestCrossPlatformCoverageUncertainRuntimeNeverPublishes covers the tier-2
// write gate: a query served under uncertainty must not create or rewrite any
// cache artifact, including the per-user fallback publication.
func TestCrossPlatformCoverageUncertainRuntimeNeverPublishes(t *testing.T) {
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
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("uncertain query wrote cache state: %v", statErr)
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
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
}
