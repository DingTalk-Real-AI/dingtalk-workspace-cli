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

package app

import (
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/spf13/cobra"
)

// A process with runtime plugins mounted must still publish and reuse the
// persisted Schema cache: plugin commands live only in the runtime command
// tree, never in the declaration-only schema source root, so the cache cannot
// misrepresent the builtin schema surface. Regression guard for the removed
// plugin-discovery MarkSchemaCacheRuntimeUncertain call, which made every
// plugin user pay full live assembly forever.
func TestCrossPlatformCoverageSchemaCachePublishesWithRuntimePlugins(t *testing.T) {
	if !schemacache.PersistentBackendEnabled(runtime.GOOS, runtime.GOARCH) {
		t.Skip("persistent cache backend is intentionally disabled on this target")
	}
	isolateSchemaCacheHome(t)
	t.Setenv(schemaCacheTestEnv, "1")
	t.Cleanup(func() { _ = cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{}) })

	previousLoader := rootLoadPlugins
	rootLoadPlugins = func(*cobra.Command, *pipeline.Engine, executor.Runner, string) []*cobra.Command {
		return []*cobra.Command{{
			Use:   "sample-plugin",
			Short: "plugin command mounted at runtime",
			RunE:  func(*cobra.Command, []string) error { return nil },
		}}
	}
	t.Cleanup(func() { rootLoadPlugins = previousLoader })

	// Sibling tests may leave an assembled live catalog registered in-process;
	// ResolveMeta serves that catalog without touching the cache. Reset and
	// re-register so this test exercises the plugin-present publish path.
	cli.RestorePackageCLISchemaDeliveryForTest()
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		return NewSchemaSourceRootCommand()
	})
	applyProductionSchemaCache()

	root := NewRootCommand()
	if root == nil {
		t.Fatal("NewRootCommand returned nil")
	}
	if cmd, _, err := root.Find([]string{"sample-plugin"}); err != nil || cmd == nil || cmd.Name() != "sample-plugin" {
		t.Fatalf("plugin command not mounted: cmd=%v err=%v", cmd, err)
	}

	// First schema use in the plugin-present process must generate AND publish
	// the cache instead of silently staying live-only.
	meta, ok := cli.ResolveMeta("calendar event create")
	if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
		t.Fatalf("plugin-present ResolveMeta = %#v, %v", meta, ok)
	}
	identity, ok := cli.SchemaCacheFastPathIdentity()
	if !ok {
		t.Fatal("plugin-present process did not adopt a generated Schema cache identity")
	}
	cache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := cache.Directory()
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	assertSchemaCacheArtifactsPresent(t, cacheDir, identity)

	// A later plugin-present process reloads the published cache and serves
	// schema reads without live Cobra assembly.
	cli.RestorePackageCLISchemaDeliveryForTest()
	var factoryCalls atomic.Int64
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		factoryCalls.Add(1)
		return NewSchemaSourceRootCommand()
	})
	applyProductionSchemaCache()
	if hit, hitOK := cli.SchemaCacheFastPathIdentity(); !hitOK || hit.BuildID != identity.BuildID {
		t.Fatalf("plugin-present reloaded identity = %#v ready=%v", hit, hitOK)
	}
	meta, ok = cli.ResolveMeta("calendar event create")
	if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
		t.Fatalf("plugin-present cache-hit ResolveMeta = %#v, %v", meta, ok)
	}
	if factoryCalls.Load() != 0 {
		t.Fatalf("plugin-present cache-hit ResolveMeta invoked Cobra factory %d times", factoryCalls.Load())
	}
}
