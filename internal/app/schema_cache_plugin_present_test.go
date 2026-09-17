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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
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

	testseam.Swap(t, &rootLoadPlugins, func(*cobra.Command, *pipeline.Engine, executor.Runner, string) []*cobra.Command {
		return []*cobra.Command{{
			Use:   "sample-plugin",
			Short: "plugin command mounted at runtime",
			RunE:  func(*cobra.Command, []string) error { return nil },
		}}
	})

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

// The safety argument for publishing the Schema cache from a plugin-present
// process is that schema assembly observes none of the process-global state a
// real plugin loader mutates before returning its commands. This test performs
// those real registration side effects — dynamic endpoint descriptors, a
// registered stdio client, and a plugin auth record — and proves the assembled
// artifacts' identity (a digest over the full schema surface) is unchanged.
func TestCrossPlatformCoverageSchemaAssemblyIgnoresPluginRegistrationSideEffects(t *testing.T) {
	isolatePluginRuntime(t)

	assembleIdentity := func() (buildID [32]byte) {
		resolved, err := cli.ResolveSchemaBuild(NewSchemaSourceRootCommand())
		if err != nil {
			t.Fatal(err)
		}
		artifacts, err := cli.BuildSchemaCacheArtifacts(resolved)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := cli.IdentityFromArtifacts("open", artifacts)
		if err != nil {
			t.Fatal(err)
		}
		return identity.BuildID
	}

	clean := assembleIdentity()

	// Replicate what loadPlugins completes before returning plugin commands:
	// an HTTP plugin server descriptor, a stdio overlay server (registered
	// with an unstarted client), and a plugin auth ownership record.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	registerPluginHTTPServer(mcptypes.ServerDescriptor{
		Key:      "side-effect-http",
		Endpoint: server.URL,
		CLI:      mcptypes.CLIOverlay{ID: "side-effect-http", Command: "side-effect-http"},
	})

	pluginRoot := t.TempDir()
	overlay := []byte(`{
		"id":"local",
		"command":"side-effect-plugin",
		"groups":{"health":{"description":"health checks"}},
		"toolOverrides":{"ping":{"cliName":"ping","group":"health"}}
	}`)
	if err := os.WriteFile(filepath.Join(pluginRoot, "overlay.json"), overlay, 0o600); err != nil {
		t.Fatal(err)
	}
	p := &plugin.Plugin{
		Manifest: plugin.Manifest{
			Name:        "side-effect-plugin",
			Description: "side effect fixture",
			MCPServers: map[string]*plugin.MCPServer{
				"local": {Type: "stdio", Command: "unused", CLI: []byte(`"overlay.json"`)},
			},
		},
		Root: pluginRoot,
	}
	client := transport.NewStdioClient("/bin/sh", []string{"-c", ":"}, nil)
	descriptor := registerStdioServerFromManifest(p, plugin.StdioServerClient{Key: "local", Client: client})
	if descriptor.Endpoint == "" {
		t.Fatal("stdio side-effect server was not registered")
	}
	RegisterPluginAuth("side-effect-plugin", &PluginAuth{Token: "opaque"})

	if _, ok := directRuntimeEndpoint("side-effect-http", ""); !ok {
		t.Fatal("HTTP side-effect endpoint missing before reassembly")
	}
	if _, ok := LookupStdioClient("side-effect-plugin/local"); !ok {
		t.Fatal("stdio side-effect client missing before reassembly")
	}
	if _, ok := LookupPluginAuth("side-effect-plugin"); !ok {
		t.Fatal("plugin auth side-effect record missing before reassembly")
	}

	withPlugins := assembleIdentity()
	if clean != withPlugins {
		t.Fatalf("schema assembly identity changed after real plugin registration side effects: clean=%x with-plugins=%x", clean, withPlugins)
	}
}

// Strongest form of the isolation proof: drive the REAL production loader
// (rootLoadPlugins → loadPlugins) against a fully isolated plugin universe —
// an enabled stdio plugin with a CLI overlay plus pluginConfigs entries whose
// env injection (InjectPluginConfigEnv, the loader's first side effect, which
// may set any non-blacklisted variable including DWS_-prefixed ones) actually
// fires — and assert the assembled schema identity is still byte-identical.
func TestCrossPlatformCoverageSchemaAssemblyIgnoresRealPluginLoaderSideEffects(t *testing.T) {
	isolatePluginRuntime(t)

	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv("HOME", t.TempDir())

	const (
		canaryDWS   = "DWS_PLUGIN_LOADER_CANARY"
		canaryPlain = "PLUGIN_LOADER_PLAIN_CANARY"
	)
	t.Cleanup(func() {
		_ = os.Unsetenv(canaryDWS)
		_ = os.Unsetenv(canaryPlain)
	})

	pluginDir := filepath.Join(configDir, "plugins", "user", "loader-side-effect")
	if err := os.MkdirAll(filepath.Join(pluginDir, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
	"name": "loader-side-effect",
	"version": "0.1.0",
	"type": "managed",
	"description": "loader side effect fixture",
	"mcpServers": {
		"local": {
			"name": "local fixture server",
			"description": "stdio fixture",
			"type": "stdio",
			"command": "${DWS_PLUGIN_ROOT}/bin/proxy",
			"args": [],
			"prefixes": ["loader-side-effect"],
			"cli": "overlay.json"
		}
	}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "overlay.json"), []byte(`{
	"id": "local",
	"command": "loader-side-effect",
	"description": "loader side effect fixture",
	"groups": {"health": {"description": "health checks"}},
	"toolOverrides": {"ping": {"cliName": "ping", "group": "health"}}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "bin", "proxy"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{
	"enabledPlugins": {"loader-side-effect": true},
	"pluginConfigs": {"loader-side-effect": {
		"` + canaryDWS + `": "loader-injected",
		"` + canaryPlain + `": "plain-injected"
	}}
}`
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}

	assembleIdentity := func() (buildID [32]byte) {
		resolved, err := cli.ResolveSchemaBuild(NewSchemaSourceRootCommand())
		if err != nil {
			t.Fatal(err)
		}
		artifacts, err := cli.BuildSchemaCacheArtifacts(resolved)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := cli.IdentityFromArtifacts("open", artifacts)
		if err != nil {
			t.Fatal(err)
		}
		return identity.BuildID
	}

	clean := assembleIdentity()

	scratch := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	pluginCmds := rootLoadPlugins(scratch, nil, executor.EchoRunner{}, "")
	if len(pluginCmds) == 0 {
		t.Fatal("real plugin loader returned no commands for the fixture plugin")
	}

	if got := os.Getenv(canaryDWS); got != "loader-injected" {
		t.Fatalf("plugin config env injection did not fire: %s=%q", canaryDWS, got)
	}
	if got := os.Getenv(canaryPlain); got != "plain-injected" {
		t.Fatalf("plugin config env injection did not fire: %s=%q", canaryPlain, got)
	}
	if _, ok := LookupStdioClient("loader-side-effect/local"); !ok {
		t.Fatal("real loader stdio registration side effect missing")
	}

	withLoader := assembleIdentity()
	if clean != withLoader {
		t.Fatalf("schema assembly identity changed after the real plugin loader ran: clean=%x with-loader=%x", clean, withLoader)
	}
}
