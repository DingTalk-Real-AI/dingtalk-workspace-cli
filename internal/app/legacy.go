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
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/compat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func newLegacyPublicCommands(ctx context.Context, runner executor.Runner) []*cobra.Command {
	if fn := edition.Get().StaticServers; fn != nil {
		injectStaticServers(fn())
		// Static servers provided by the edition hook — skip Market discovery
		// entirely. The overlay registers its own product commands via
		// RegisterExtraCommands; we only add the open-source helpers here.
		commands := helpers.NewPublicCommands(runner)
		return mergeTopLevelCommands(commands)
	}

	var commands []*cobra.Command
	if dynamicCmds := loadDynamicCommands(ctx, runner); len(dynamicCmds) > 0 {
		commands = append(commands, dynamicCmds...)
	}
	commands = append(commands, helpers.NewPublicCommands(runner)...)
	return mergeTopLevelCommands(commands)
}

// injectStaticServers converts edition.ServerInfo entries into
// market.ServerDescriptor and feeds them into SetDynamicServers so the
// direct-runtime endpoint resolver can find them.
func injectStaticServers(servers []edition.ServerInfo) {
	descriptors := make([]market.ServerDescriptor, 0, len(servers))
	for _, s := range servers {
		descriptors = append(descriptors, market.ServerDescriptor{
			Key:         s.ID,
			DisplayName: s.Name,
			Endpoint:    s.Endpoint,
			CLI: market.CLIOverlay{
				ID:       s.ID,
				Command:  s.ID,
				Prefixes: s.Prefixes,
			},
		})
	}
	SetDynamicServers(descriptors)
}

// loadDynamicCommands loads the server registry and generates CLI commands
// dynamically from CLIOverlay metadata. It consults the disk cache first.
// Within the short revalidation window it uses the cached registry directly;
// after that it revalidates against the live market registry. Once the hard
// RegistryTTL expires, a successful live registry fetch triggers a full detail
// refresh for every server so command metadata cannot stay pinned to an
// arbitrarily old snapshot. On network failure with a stale cache, it
// gracefully degrades to the cached data so the CLI remains functional
// offline.
//
// Tests may override discoveryBaseURLOverride to redirect to a local server;
// in that case the registry cache is always bypassed.
// editionPartition returns the cache partition for the active edition.
// Each edition gets its own partition to prevent cross-edition data leakage.
func editionPartition() string {
	name := edition.Get().Name
	if name == "" || name == "open" {
		return config.DefaultPartition
	}
	return name + "/default"
}

// discoveryTraceEnabled reports whether the user asked for discovery-path diagnostics.
// loadDynamicCommands runs while building the command tree, before PersistentPreRun
// applies --debug to slog; we also accept argv --debug and DWS_PERF_DEBUG for consistency.
func discoveryTraceEnabled() bool {
	if IsPerfDebugEnabled() {
		return true
	}
	for _, a := range os.Args[1:] {
		if a == "--debug" {
			return true
		}
	}
	return false
}

func discoveryTraceServerIDs(servers []market.ServerDescriptor) []string {
	seen := make(map[string]struct{})
	for _, s := range servers {
		id := strings.TrimSpace(s.CLI.Command)
		if id == "" {
			id = strings.TrimSpace(s.CLI.ID)
		}
		if id == "" {
			continue
		}
		seen[id] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	const maxIDs = 48
	if len(out) > maxIDs {
		out = out[:maxIDs]
	}
	return out
}

func loadDynamicCommands(ctx context.Context, runner executor.Runner) []*cobra.Command {
	store := cacheStoreFromEnv()
	partition := editionPartition()

	// Bypass the registry cache when a fixture override is active.
	// This ensures tests that set DWS_CATALOG_FIXTURE always get fresh
	// data from their local mock server without interference from a
	// stale on-disk cache written by a previous production run.
	useCache := strings.TrimSpace(os.Getenv(cli.CatalogFixtureEnv)) == ""

	// --- Cache-first server registry ---
	cacheLoadStart := time.Now()
	snapshot, freshness, cacheErr := store.LoadRegistry(partition)
	RecordTiming(ctx, "registry_cache", time.Since(cacheLoadStart))

	var servers []market.ServerDescriptor
	now := store.Now().UTC()
	usingCachedRegistry := useCache && cacheErr == nil && len(snapshot.Servers) > 0

	if usingCachedRegistry {
		servers = snapshot.Servers
		// Only trigger async revalidation in production (no URL override).
		// Tests set discoveryBaseURLOverride and control cache expiry directly,
		// so background revalidation would interfere with test expectations.
		if discoveryBaseURLOverride == "" && (freshness == cache.FreshnessStale || cache.ShouldRevalidate(now, snapshot.SavedAt)) {
			go asyncRevalidateRegistry(ctx, store, partition)
		}
	}

	if len(servers) > 0 && discoveryTraceEnabled() {
		slog.Info("loadDynamicCommands: skipping sync discovery fetch, using registry cache",
			"partition", partition,
			"servers", len(servers),
			"registry_freshness", string(freshness))
	}

	// Cache miss or bypassed: fetch from market API synchronously (first run only).
	if len(servers) == 0 {
		if discoveryTraceEnabled() {
			if edURL := strings.TrimSpace(edition.Get().DiscoveryURL); edURL != "" {
				slog.Info("loadDynamicCommands: sync discovery fetch", "partition", partition, "url", edURL)
			} else {
				baseURL := cli.DefaultMarketBaseURL
				if discoveryBaseURLOverride != "" {
					baseURL = discoveryBaseURLOverride
				}
				slog.Info("loadDynamicCommands: sync market catalog fetch", "partition", partition, "base_url", baseURL)
			}
		}
		fetchStart := time.Now()

		resp, fetchErr := fetchRegistryServers(ctx, ipv4OnlyHTTPClient())

		RecordTiming(ctx, "market_fetch", time.Since(fetchStart))
		if fetchErr != nil {
			if discoveryTraceEnabled() {
				slog.Info("loadDynamicCommands: sync discovery fetch failed",
					"partition", partition,
					"error", fetchErr.Error())
			}
			slog.Debug("loadDynamicCommands: market API fetch failed", "error", fetchErr)
			// Degrade to stale cache if available (production only).
			if useCache && cacheErr == nil && len(snapshot.Servers) > 0 {
				slog.Debug("loadDynamicCommands: degrading to stale registry cache", "servers", len(snapshot.Servers))
				servers = snapshot.Servers
			} else {
				// no-op: fall through to FallbackServers check below
			}
		} else {
			servers = market.NormalizeServers(resp, "market")
			if discoveryTraceEnabled() {
				slog.Info("loadDynamicCommands: sync discovery fetch ok",
					"partition", partition,
					"response_servers", len(resp.Servers),
					"metadata_count", resp.Metadata.Count,
					"normalized_servers", len(servers),
					"cli_command_ids", discoveryTraceServerIDs(servers))
			}
			// Persist fresh data (only in non-test mode).
			if useCache {
				saveStart := time.Now()
				if saveErr := store.SaveRegistry(partition, cache.RegistrySnapshot{Servers: servers}); saveErr != nil {
					slog.Debug("loadDynamicCommands: failed to save registry cache", "error", saveErr)
				}
				RecordTiming(ctx, "cache_save", time.Since(saveStart))
			}
		}
	}

	// FallbackServers: safety net when Market discovery + cache both fail.
	if len(servers) == 0 {
		if fn := edition.Get().FallbackServers; fn != nil {
			if fb := fn(); len(fb) > 0 {
				slog.Debug("loadDynamicCommands: using FallbackServers", "count", len(fb))
				descriptors := fallbackToDescriptors(fb)
				descriptors = mergeSupplementServers(descriptors)
				SetDynamicServers(descriptors)
				return nil
			}
		}
		return nil
	}

	// Merge edition-specific supplement servers (not in Market).
	servers = mergeSupplementServers(servers)
	// Inject dynamic server data for endpoint resolution
	SetDynamicServers(servers)

	detailStart := time.Now()
	detailsByID := loadCachedDetailsFast(store, servers)
	RecordTiming(ctx, "tool_metadata", time.Since(detailStart))

	buildStart := time.Now()
	cmds := compat.BuildDynamicCommands(servers, runner, detailsByID)
	RecordTiming(ctx, "build_commands", time.Since(buildStart))

	return cmds
}

// loadCachedDetailsFast reads Detail API tool metadata from disk cache only —
// no network calls. Returns whatever is available (fresh or stale).
func loadCachedDetailsFast(store *cache.Store, servers []market.ServerDescriptor) map[string][]market.DetailTool {
	result := make(map[string][]market.DetailTool)
	if store == nil {
		return result
	}
	partition := editionPartition()
	for _, server := range servers {
		if server.DetailLocator.MCPID <= 0 {
			continue
		}
		serverID := strings.TrimSpace(server.CLI.ID)
		if serverID == "" {
			continue
		}
		snap, _, err := store.LoadDetail(partition, serverID)
		if err != nil {
			continue
		}
		var payload struct {
			Tools []market.DetailTool `json:"tools"`
		}
		if jsonErr := json.Unmarshal(snap.Payload, &payload); jsonErr == nil && len(payload.Tools) > 0 {
			result[serverID] = payload.Tools
		}
	}
	return result
}

// fetchDetailsByServerID fetches MCP Detail API tool metadata for each server
// with a known mcpId. Returns a map from CLI server ID → []DetailTool.
// Results are read from / written to the disk cache (DetailTTL=7d).
// All network fetches run concurrently; best-effort (errors silently skip).
func fetchDetailsByServerID(ctx context.Context, client *market.Client, servers []market.ServerDescriptor, store *cache.Store, forceRefresh bool) map[string][]market.DetailTool {
	if ctx == nil {
		ctx = context.Background()
	}
	partition := editionPartition()
	now := time.Now().UTC()
	if store != nil && store.Now != nil {
		now = store.Now().UTC()
	}

	type entry struct {
		id    string
		tools []market.DetailTool
	}

	results := make(chan entry, len(servers))
	var wg sync.WaitGroup

	for _, server := range servers {
		mcpID := server.DetailLocator.MCPID
		if mcpID <= 0 {
			continue
		}
		serverID := strings.TrimSpace(server.CLI.ID)
		if serverID == "" {
			continue
		}

		wg.Add(1)
		go func(srv market.ServerDescriptor, sID string, mID int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("fetchDetailsByServerID: goroutine panicked", "server", sID, "panic", r)
				}
			}()

			// Cache hit check. Fresh entries within the short revalidation window
			// are returned immediately. Older entries still serve as fallback if
			// the live market detail request fails.
			var cachedTools []market.DetailTool
			haveCachedTools := false
			if store != nil {
				if snap, freshness, err := store.LoadDetail(partition, sID); err == nil {
					var payload struct {
						Tools []market.DetailTool `json:"tools"`
					}
					if jsonErr := json.Unmarshal(snap.Payload, &payload); jsonErr == nil && len(payload.Tools) > 0 {
						cachedTools = payload.Tools
						haveCachedTools = true
					}
					if !forceRefresh && freshness == cache.FreshnessFresh && haveCachedTools && !cache.ShouldRevalidate(now, snap.SavedAt) {
						slog.Debug("fetchDetailsByServerID: using cached detail", "id", sID)
						results <- entry{id: sID, tools: cachedTools}
						return
					}
				}
			}

			// Network fetch with per-server 5s timeout.
			fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			var detail market.DetailResponse
			var fetchErr error
			detailURL := strings.TrimSpace(srv.DetailLocator.DetailURL)
			if detailURL != "" {
				detail, fetchErr = client.FetchDetailByURL(fetchCtx, detailURL)
			} else {
				detail, fetchErr = client.FetchDetail(fetchCtx, mID)
			}
			if fetchErr != nil {
				slog.Debug("fetchDetailsByServerID: skipping server", "id", sID, "mcpId", mID, "error", fetchErr)
				if haveCachedTools {
					results <- entry{id: sID, tools: cachedTools}
				}
				return
			}
			if !detail.Success || len(detail.Result.Tools) == 0 {
				if haveCachedTools {
					results <- entry{id: sID, tools: cachedTools}
				}
				return
			}

			// Persist to cache.
			if store != nil {
				if payload, marshalErr := json.Marshal(map[string]any{"tools": detail.Result.Tools}); marshalErr == nil {
					if saveErr := store.SaveDetail(partition, sID, cache.DetailSnapshot{
						MCPID:   mID,
						Payload: payload,
					}); saveErr != nil {
						slog.Debug("fetchDetailsByServerID: failed to save detail cache", "id", sID, "error", saveErr)
					}
				}
			}

			slog.Debug("fetchDetailsByServerID: got tool details", "id", sID, "tools", len(detail.Result.Tools))
			results <- entry{id: sID, tools: detail.Result.Tools}
		}(server, serverID, mcpID)
	}

	// Close channel after all goroutines finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	result := make(map[string][]market.DetailTool)
	for e := range results {
		result[e.id] = e.tools
	}
	return result
}

// discoveryBaseURLOverride allows tests to redirect discovery to a local server.
// Must be empty in production; only set during test execution.
var discoveryBaseURLOverride string

// SetDiscoveryBaseURL sets the base URL used for dynamic server discovery.
// Intended for test use only.
func SetDiscoveryBaseURL(url string) {
	discoveryBaseURLOverride = url
}

// DiscoveryBaseURL returns the effective base URL for discovery —
// discoveryBaseURLOverride if set, otherwise DefaultMarketBaseURL.
func DiscoveryBaseURL() string {
	if discoveryBaseURLOverride != "" {
		return discoveryBaseURLOverride
	}
	return cli.DefaultMarketBaseURL
}

// ipv4HTTPClient returns an HTTP client that forces IPv4 connections with
// the given total request timeout. This avoids IPv6 DNS/connect timeouts on
// hosts without IPv6 networking.
func ipv4HTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp4", addr)
			},
		},
	}
}

// ipv4OnlyHTTPClient returns an IPv4-forcing HTTP client with a short timeout
// suitable for CLI startup network requests.
func ipv4OnlyHTTPClient() *http.Client {
	return ipv4HTTPClient(5 * time.Second)
}

// fetchRegistryServers performs the server-list HTTP fetch honoring the
// active edition's DiscoveryURL override. It is the single source of truth
// for all server-list fetches (startup, async revalidation, explicit
// `cache refresh`); keeping the edition-URL branch in one place prevents
// call sites from drifting out of sync.
func fetchRegistryServers(ctx context.Context, httpClient *http.Client) (market.ListResponse, error) {
	if editionURL := strings.TrimSpace(edition.Get().DiscoveryURL); editionURL != "" {
		client := market.NewClient("", httpClient)
		if fn := edition.Get().DiscoveryHeaders; fn != nil {
			client.Headers = fn()
		}
		return client.FetchServersFromURL(ctx, editionURL)
	}
	client := market.NewClient(DiscoveryBaseURL(), httpClient)
	return client.FetchServers(ctx, config.DefaultFetchServersLimit)
}

// asyncRevalidateRegistry refreshes the registry cache in the background.
// Uses a short timeout derived from the parent context and silently ignores
// errors — the next CLI invocation will pick up the refreshed cache or retry.
func asyncRevalidateRegistry(parent context.Context, store *cache.Store, partition string) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	resp, err := fetchRegistryServers(ctx, ipv4OnlyHTTPClient())
	if err != nil {
		slog.Debug("asyncRevalidateRegistry: fetch failed", "error", err)
		return
	}
	servers := market.NormalizeServers(resp, "market")
	if saveErr := store.SaveRegistry(partition, cache.RegistrySnapshot{Servers: servers}); saveErr != nil {
		slog.Debug("asyncRevalidateRegistry: save failed", "error", saveErr)
	}
}

func newLegacyHiddenCommands(_ executor.Runner) []*cobra.Command {
	return nil
}

func mergeTopLevelCommands(commands []*cobra.Command) []*cobra.Command {
	byName := make(map[string]*cobra.Command, len(commands))
	for _, cmd := range commands {
		if cmd == nil {
			continue
		}
		name := cmd.Name()
		if name == "" {
			continue
		}
		if existing, ok := byName[name]; ok {
			cobracmd.MergeCommandTree(existing, cmd)
			continue
		}
		byName[name] = cmd
	}

	out := make([]*cobra.Command, 0, len(byName))
	for _, cmd := range byName {
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Use < out[j].Use
	})
	return out
}

// mergeSupplementServers appends edition-specific servers (not in Market)
// into the discovery result. Existing IDs from Market/cache take precedence.
func mergeSupplementServers(servers []market.ServerDescriptor) []market.ServerDescriptor {
	fn := edition.Get().SupplementServers
	if fn == nil {
		return servers
	}
	existing := make(map[string]bool, len(servers))
	for _, s := range servers {
		existing[s.CLI.ID] = true
		existing[s.Key] = true
	}
	for _, sup := range fn() {
		if !existing[sup.ID] {
			servers = append(servers, market.ServerDescriptor{
				Key:         sup.ID,
				DisplayName: sup.Name,
				Endpoint:    sup.Endpoint,
				CLI: market.CLIOverlay{
					ID:       sup.ID,
					Command:  sup.ID,
					Prefixes: sup.Prefixes,
				},
			})
		}
	}
	return servers
}

// fallbackToDescriptors converts edition.ServerInfo into market.ServerDescriptor.
func fallbackToDescriptors(servers []edition.ServerInfo) []market.ServerDescriptor {
	descriptors := make([]market.ServerDescriptor, 0, len(servers))
	for _, s := range servers {
		descriptors = append(descriptors, market.ServerDescriptor{
			Key:         s.ID,
			DisplayName: s.Name,
			Endpoint:    s.Endpoint,
			CLI: market.CLIOverlay{
				ID:       s.ID,
				Command:  s.ID,
				Prefixes: s.Prefixes,
			},
		})
	}
	return descriptors
}
