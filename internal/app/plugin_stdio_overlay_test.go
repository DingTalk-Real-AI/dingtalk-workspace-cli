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
	"encoding/json"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/spf13/cobra"
)

// withCleanStdioRegistry snapshots and restores the package-level stdio
// client registry so tests that call RegisterStdioClient don't leak state
// across cases.
func withCleanStdioRegistry(t *testing.T) {
	t.Helper()
	stdioMu.Lock()
	prev := stdioClients
	stdioClients = make(map[string]*transport.StdioClient)
	stdioMu.Unlock()
	t.Cleanup(func() {
		stdioMu.Lock()
		stdioClients = prev
		stdioMu.Unlock()
	})
}

// newOverlayFixture constructs a plugin + stdio entry carrying an inline
// CLIOverlay with the given tool-override map. The stdio client is created
// but never started, since the overlay-first path does not require the
// subprocess to be running for command registration.
func newOverlayFixture(t *testing.T, pluginName, serverKey string, overlay market.CLIOverlay) (*plugin.Plugin, plugin.StdioServerClient) {
	t.Helper()
	raw, err := json.Marshal(overlay)
	if err != nil {
		t.Fatalf("marshal overlay: %v", err)
	}
	p := &plugin.Plugin{
		Manifest: plugin.Manifest{
			Name:        pluginName,
			Version:     "1.0.0",
			Description: pluginName + " plugin",
			MCPServers: map[string]*plugin.MCPServer{
				serverKey: {
					Type:    "stdio",
					Command: "/usr/bin/true", // never executed by overlay-first path
					CLI:     raw,
				},
			},
		},
		Root: t.TempDir(),
	}
	sc := plugin.StdioServerClient{
		Key:    serverKey,
		Client: transport.NewStdioClient("/usr/bin/true", nil, nil),
	}
	return p, sc
}

// TestRegisterStdioServerFromOverlay_NoDiscoveryStillBuildsCommands verifies
// the core promise of the overlay-first path: when overlay.json ships
// ToolOverrides, commands appear immediately — no subprocess probe.
func TestRegisterStdioServerFromOverlay_NoDiscoveryStillBuildsCommands(t *testing.T) {
	withCleanDynamicRegistry(t)
	withCleanStdioRegistry(t)

	overlay := market.CLIOverlay{
		ID:      "conference-local",
		Command: "conference-local",
		Groups: map[string]market.CLIGroupDef{
			"meeting": {Description: "会议控制"},
			"member":  {Description: "成员管理"},
		},
		ToolOverrides: map[string]market.CLIToolOverride{
			"create_meeting": {CLIName: "create", Group: "meeting", Description: "Create a meeting"},
			"end_meeting":    {CLIName: "end", Group: "meeting", Description: "End a meeting"},
			"mute_member":    {CLIName: "mute", Group: "member", Description: "Mute a member"},
		},
	}
	p, sc := newOverlayFixture(t, "conference-local", "conference-local", overlay)

	store := cache.NewStore(t.TempDir())
	cmds, desc, ok := registerStdioServerFromOverlay(p, sc, executor.EchoRunner{}, store)
	if !ok {
		t.Fatal("registerStdioServerFromOverlay returned ok=false, want true")
	}
	if len(cmds) == 0 {
		t.Fatal("registerStdioServerFromOverlay returned 0 commands, want >=1")
	}

	var root *struct{ name, path string }
	_ = root
	found := false
	for _, c := range cmds {
		if c.Name() == "conference-local" {
			found = true
			// Groups must be attached as sub-commands.
			groups := map[string]bool{}
			for _, sub := range c.Commands() {
				groups[sub.Name()] = true
			}
			if !groups["meeting"] {
				t.Errorf("missing 'meeting' group sub-command, children = %v", groups)
			}
			if !groups["member"] {
				t.Errorf("missing 'member' group sub-command, children = %v", groups)
			}
		}
	}
	if !found {
		names := []string{}
		for _, c := range cmds {
			names = append(names, c.Name())
		}
		t.Fatalf("missing top-level 'conference-local' command, got %v", names)
	}

	// AppendDynamicServer registration: product ID should land in
	// DirectRuntimeProductIDs so hideNonDirectRuntimeCommands keeps it
	// visible even under a restrictive VisibleProducts hook.
	if !DirectRuntimeProductIDs()["conference-local"] {
		t.Error("DirectRuntimeProductIDs missing 'conference-local'")
	}

	// RegisterStdioClient side-effect: the runtime must be able to look up
	// the StdioClient when the endpoint is invoked later.
	if _, ok := LookupStdioClient("conference-local/conference-local"); !ok {
		t.Error("LookupStdioClient missing conference-local/conference-local")
	}

	if desc.Endpoint != StdioEndpoint("conference-local", "conference-local") {
		t.Errorf("descriptor.Endpoint = %q, want %q", desc.Endpoint, StdioEndpoint("conference-local", "conference-local"))
	}
}

// TestRegisterStdioServerFromOverlay_WarmCacheEnrichesFlags pre-populates the
// tools cache with a schema-bearing DetailTool and asserts the resulting
// leaf command picks up the typed flag derived from InputSchema.
func TestRegisterStdioServerFromOverlay_WarmCacheEnrichesFlags(t *testing.T) {
	withCleanDynamicRegistry(t)
	withCleanStdioRegistry(t)

	overlay := market.CLIOverlay{
		ID:      "cache-plugin",
		Command: "cache-plugin",
		ToolOverrides: map[string]market.CLIToolOverride{
			"echo": {CLIName: "echo", Description: "Echo input"},
		},
	}
	p, sc := newOverlayFixture(t, "cache-plugin", "cache-plugin", overlay)

	store := cache.NewStore(t.TempDir())
	cacheKey := pluginCacheKey(p.Manifest.Name, sc.Key)
	if err := store.SaveTools(config.DefaultPartition, cacheKey, cache.ToolsSnapshot{
		SavedAt:   time.Now().UTC(),
		ServerKey: cacheKey,
		Tools: []transport.ToolDescriptor{
			{
				Name:        "echo",
				Description: "Echo the input",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"message": map[string]any{"type": "string"},
					},
					"required": []any{"message"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("SaveTools: %v", err)
	}

	cmds, _, ok := registerStdioServerFromOverlay(p, sc, executor.EchoRunner{}, store)
	if !ok || len(cmds) == 0 {
		t.Fatalf("overlay registration failed: ok=%v cmds=%d", ok, len(cmds))
	}

	var echoLeaf *leafMatch
	for _, top := range cmds {
		if top.Name() != "cache-plugin" {
			continue
		}
		for _, sub := range top.Commands() {
			if sub.Name() == "echo" {
				echoLeaf = &leafMatch{name: sub.Name(), hasFlag: sub.Flags().Lookup("message") != nil}
			}
		}
	}
	if echoLeaf == nil {
		t.Fatal("missing 'echo' leaf command under 'cache-plugin'")
	}
	if !echoLeaf.hasFlag {
		t.Error("warm-cache enrichment did not wire --message flag from InputSchema")
	}
}

type leafMatch struct {
	name    string
	hasFlag bool
}

// TestRegisterStdioServerFromOverlay_OverlayWithoutOverridesReturnsFalse
// asserts the fallback contract: when overlay.json is missing toolOverrides,
// the overlay-first path declines so the caller can route the entry through
// the legacy discovery-first registerStdioServer.
func TestRegisterStdioServerFromOverlay_OverlayWithoutOverridesReturnsFalse(t *testing.T) {
	withCleanDynamicRegistry(t)
	withCleanStdioRegistry(t)

	// Overlay with no ToolOverrides (simulates a plugin that relies entirely
	// on runtime discovery for its tool list).
	overlay := market.CLIOverlay{
		ID:      "legacy-plugin",
		Command: "legacy-plugin",
	}
	p, sc := newOverlayFixture(t, "legacy-plugin", "legacy-plugin", overlay)

	store := cache.NewStore(t.TempDir())
	cmds, _, ok := registerStdioServerFromOverlay(p, sc, executor.EchoRunner{}, store)
	if ok {
		t.Errorf("registerStdioServerFromOverlay ok=true for empty toolOverrides; want false")
	}
	if cmds != nil {
		t.Errorf("cmds = %v, want nil", cmds)
	}
	if DirectRuntimeProductIDs()["legacy-plugin"] {
		t.Error("legacy-plugin must NOT be appended to dynamic registry in fallback case")
	}
	if _, found := LookupStdioClient("legacy-plugin/legacy-plugin"); found {
		t.Error("stdio client must NOT be registered in fallback case")
	}
}

// TestRefreshStdioToolsCache_FailurePreservesCache guards against the
// "negative cache poisoning" bug: if discovery fails (subprocess not ready,
// timeout, empty tool list), the existing warm cache must remain intact so
// the next startup still enriches flags from the last good snapshot.
func TestRefreshStdioToolsCache_FailurePreservesCache(t *testing.T) {
	withCleanDynamicRegistry(t)
	withCleanStdioRegistry(t)

	p, sc := newOverlayFixture(t, "refresh-plugin", "refresh-plugin", market.CLIOverlay{
		ID:      "refresh-plugin",
		Command: "refresh-plugin",
	})

	store := cache.NewStore(t.TempDir())
	cacheKey := pluginCacheKey(p.Manifest.Name, sc.Key)
	goodSnapshot := cache.ToolsSnapshot{
		SavedAt:   time.Now().UTC(),
		ServerKey: cacheKey,
		Tools: []transport.ToolDescriptor{
			{
				Name:        "ping",
				Description: "Health check",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}
	if err := store.SaveTools(config.DefaultPartition, cacheKey, goodSnapshot); err != nil {
		t.Fatalf("seed SaveTools: %v", err)
	}

	// /usr/bin/true exits immediately, so Initialize + ListTools will fail
	// (no MCP handshake). discoverStdioTools returns nil → refresh must be
	// a no-op and must NOT overwrite the good cache with a null snapshot.
	refreshStdioToolsCache(p, sc, store, pluginColdTimeouts{stdio: 200 * time.Millisecond})

	got, _, err := store.LoadTools(config.DefaultPartition, cacheKey)
	if err != nil {
		t.Fatalf("LoadTools after failed refresh: %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "ping" {
		t.Errorf("warm cache was overwritten by failed refresh: %+v", got.Tools)
	}
}

// TestLoadPlugins_OverlayFirstVisibleBeforeDiscovery is an integration-style
// test for the loadPlugins split decision: stdio plugins whose overlay ships
// ToolOverrides must have their commands visible on the root immediately,
// WITHOUT waiting on any discovery handshake. It drives the same sequence
// loadPlugins uses (registerStdioServerFromOverlay → root.AddCommand →
// hideNonDirectRuntimeCommands) and asserts the plugin command survives the
// visibility filter even when no discovery has run.
func TestLoadPlugins_OverlayFirstVisibleBeforeDiscovery(t *testing.T) {
	withCleanDynamicRegistry(t)
	withCleanStdioRegistry(t)

	// Simulate a wukong-like edition that declares a static VisibleProducts
	// whitelist NOT containing our plugin. This is the exact scenario where
	// the original bug surfaced.
	overrideVisibleProducts(t, []string{"calendar", "doc"})

	overlay := market.CLIOverlay{
		ID:      "conference-local",
		Command: "conference-local",
		ToolOverrides: map[string]market.CLIToolOverride{
			"create_meeting": {CLIName: "create", Description: "Create a meeting"},
		},
	}
	p, sc := newOverlayFixture(t, "conference-local", "conference-local", overlay)

	// No discovery runs — no cache seeded. This mirrors a cold-start where
	// the subprocess is unavailable (or just slow) yet the user expects
	// `dws --help` to still list the plugin.
	store := cache.NewStore(t.TempDir())
	cmds, _, ok := registerStdioServerFromOverlay(p, sc, executor.EchoRunner{}, store)
	if !ok {
		t.Fatal("registerStdioServerFromOverlay returned ok=false")
	}

	root := &cobra.Command{Use: "dws"}
	// Also add a sibling command that is NOT a registered product so we can
	// prove the visibility filter still hides non-product commands.
	bogus := &cobra.Command{Use: "bogus-not-a-product"}
	root.AddCommand(bogus)
	for _, c := range cmds {
		root.AddCommand(c)
	}

	hideNonDirectRuntimeCommands(root)

	var pluginCmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "conference-local" {
			pluginCmd = c
		}
	}
	if pluginCmd == nil {
		t.Fatal("conference-local missing from root after overlay-first registration")
	}
	if pluginCmd.Hidden {
		t.Error("conference-local must stay visible (Hidden=false) after hideNonDirectRuntimeCommands")
	}
	if !bogus.Hidden {
		t.Error("bogus-not-a-product must be hidden by the visibility filter")
	}

	services := visibleMCPRootCommands(root)
	if !containsCommand(services, "conference-local") {
		t.Errorf("visibleMCPRootCommands missing conference-local: %v", commandNames(services))
	}
}

// TestHasOverlayToolOverrides exercises the split-decision helper used by
// loadPlugins to route stdio entries to overlay-first vs. legacy buckets.
func TestHasOverlayToolOverrides(t *testing.T) {
	cases := []struct {
		name    string
		overlay market.CLIOverlay
		want    bool
	}{
		{
			name:    "empty overlay",
			overlay: market.CLIOverlay{ID: "x", Command: "x"},
			want:    false,
		},
		{
			name: "overlay with overrides",
			overlay: market.CLIOverlay{
				ID:      "x",
				Command: "x",
				ToolOverrides: map[string]market.CLIToolOverride{
					"foo": {CLIName: "foo"},
				},
			},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, sc := newOverlayFixture(t, "x", "x", tc.overlay)
			got := hasOverlayToolOverrides(p, sc)
			if got != tc.want {
				t.Errorf("hasOverlayToolOverrides = %v, want %v", got, tc.want)
			}
		})
	}
}
