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
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

const (
	CatalogFixtureEnv    = "DWS_CATALOG_FIXTURE"
	CacheDirEnv          = "DWS_CACHE_DIR"
	DefaultMarketBaseURL = "https://mcp.dingtalk.com"

	// defaultDiscoveryTimeout bounds the time spent on live registry discovery.
	defaultDiscoveryTimeout = 10 * time.Second
)

type CatalogLoader interface {
	Load(context.Context) (ir.Catalog, error)
}

type StaticLoader struct {
	Catalog ir.Catalog
}

func (l StaticLoader) Load(_ context.Context) (ir.Catalog, error) {
	return l.Catalog, nil
}

// CatalogLoaderFrom creates a CatalogLoader that returns a
// pre-loaded catalog and error. This allows multiple consumers
// (schema command, MCP command tree) to share one discovery result.
func CatalogLoaderFrom(catalog ir.Catalog, err error) CatalogLoader {
	return &preloadedLoader{catalog: catalog, err: err}
}

type preloadedLoader struct {
	catalog ir.Catalog
	err     error
}

func (l *preloadedLoader) Load(_ context.Context) (ir.Catalog, error) {
	return l.catalog, l.err
}

type FixtureLoader struct {
	Path string
}

func (l FixtureLoader) Load(_ context.Context) (ir.Catalog, error) {
	data, err := os.ReadFile(l.Path)
	if err != nil {
		return ir.Catalog{}, fmt.Errorf("read catalog fixture: %w", err)
	}
	var catalog ir.Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return ir.Catalog{}, fmt.Errorf("decode catalog fixture: %w", err)
	}
	return catalog, nil
}

type EnvironmentLoader struct {
	LookupEnv func(string) (string, bool)
	// CatalogBaseURLOverride allows tests to redirect catalog discovery.
	CatalogBaseURLOverride string
	// DiscoveryTimeout overrides the default timeout for live registry discovery.
	// Zero means use defaultDiscoveryTimeout.
	DiscoveryTimeout time.Duration
	// AuthTokenFunc returns an access token for MCP discovery requests
	// (initialize, tools/list). When nil, discovery runs without auth.
	AuthTokenFunc func(context.Context) string
}

type cachedCatalogState struct {
	Catalog         ir.Catalog
	Registry        cache.RegistrySnapshot
	Available       bool
	NeedsRevalidate bool
}

func NewEnvironmentLoader() EnvironmentLoader {
	return EnvironmentLoader{LookupEnv: os.LookupEnv}
}

func (l EnvironmentLoader) Load(ctx context.Context) (ir.Catalog, error) {
	if fixturePath, ok := l.lookup(CatalogFixtureEnv); ok {
		return FixtureLoader{Path: fixturePath}.Load(ctx)
	}

	baseURL := DefaultMarketBaseURL
	if l.CatalogBaseURLOverride != "" {
		baseURL = l.CatalogBaseURLOverride
	}

	cacheDir, _ := l.lookup(CacheDirEnv)
	store := cache.NewStore(cacheDir)
	partition := config.DefaultPartition

	// Cache-first: if a cached catalog is available, use it immediately.
	// Startup command construction should not block on synchronous discovery
	// just because the cache has aged past the short revalidation window.
	cached := l.loadFromCache(store)
	if cached.Available {
		return cached.Catalog, nil
	}

	transportClient := transport.NewClient(nil)
	if l.AuthTokenFunc != nil {
		if token := l.AuthTokenFunc(ctx); token != "" {
			transportClient = transportClient.WithAuth(token, nil)
		}
	}

	// Use a bounded context so discovery doesn't hang in test or CI environments.
	timeout := defaultDiscoveryTimeout
	if l.DiscoveryTimeout > 0 {
		timeout = l.DiscoveryTimeout
	}
	discoverCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	service := discovery.NewService(
		market.NewClient(baseURL, nil),
		transportClient,
		store,
	)
	response, err := service.MarketClient.FetchServers(discoverCtx, 200)
	if err != nil {
		// Graceful degradation: return empty catalog on discovery failure.
		// The runtime runner will fall back to EchoRunner for unknown products.
		if cached.Available {
			return cached.Catalog, nil
		}
		return ir.Catalog{}, nil
	}

	servers := market.NormalizeServers(response, "live_market")
	_ = store.SaveRegistry(partition, cache.RegistrySnapshot{Servers: servers})

	changedKeys := cache.ChangedServerKeysByUpdatedAt(cached.Registry.Servers, servers)
	unchangedRuntime := make(map[string]discovery.RuntimeServer)
	toRefresh := make([]market.ServerDescriptor, 0, len(servers))
	for _, server := range servers {
		if changedKeys[server.Key] {
			toRefresh = append(toRefresh, server)
			continue
		}
		toolsSnap, freshness, loadErr := store.LoadTools(partition, server.Key)
		if loadErr != nil || freshness != cache.FreshnessFresh {
			toRefresh = append(toRefresh, server)
			continue
		}
		unchangedRuntime[server.Key] = discovery.RuntimeServer{
			Server:                    server,
			NegotiatedProtocolVersion: toolsSnap.ProtocolVersion,
			Tools:                     toolsSnap.Tools,
			Source:                    "fresh_cache",
			Degraded:                  false,
		}
	}

	refreshed, failures := service.DiscoverAllRuntime(discoverCtx, toRefresh)
	if len(unchangedRuntime) == 0 && len(refreshed) == 0 && len(failures) > 0 {
		if cached.Available {
			return cached.Catalog, nil
		}
		return ir.Catalog{}, nil
	}

	refreshedByKey := make(map[string]discovery.RuntimeServer, len(refreshed))
	for _, runtimeServer := range refreshed {
		refreshedByKey[runtimeServer.Server.Key] = runtimeServer
	}

	runtimeServers := make([]discovery.RuntimeServer, 0, len(servers))
	for _, server := range servers {
		if runtimeServer, ok := refreshedByKey[server.Key]; ok {
			runtimeServers = append(runtimeServers, runtimeServer)
			continue
		}
		if runtimeServer, ok := unchangedRuntime[server.Key]; ok {
			runtimeServers = append(runtimeServers, runtimeServer)
		}
	}
	return ir.BuildCatalog(runtimeServers), nil
}

// loadFromCache builds a catalog from cached registry + tools snapshots.
// When the cache is still within TTL but older than the short revalidation
// window, the returned state asks the caller to try live discovery before
// trusting the cache as current truth.
func (l EnvironmentLoader) loadFromCache(store *cache.Store) cachedCatalogState {
	partition := config.DefaultPartition
	regSnap, freshness, err := store.LoadRegistry(partition)
	if err != nil || len(regSnap.Servers) == 0 {
		return cachedCatalogState{}
	}

	now := store.Now().UTC()
	needsRevalidate := freshness == cache.FreshnessStale || cache.ShouldRevalidate(now, regSnap.SavedAt)
	runtimeServers := make([]discovery.RuntimeServer, 0, len(regSnap.Servers))
	for _, server := range regSnap.Servers {
		toolsSnap, toolsFreshness, toolsErr := store.LoadTools(partition, server.Key)
		if toolsErr != nil || toolsFreshness != cache.FreshnessFresh {
			needsRevalidate = true
			continue
		}
		runtimeServers = append(runtimeServers, discovery.RuntimeServer{
			Server:                    server,
			NegotiatedProtocolVersion: toolsSnap.ProtocolVersion,
			Tools:                     toolsSnap.Tools,
			Source:                    "fresh_cache",
			Degraded:                  false,
		})
	}
	if len(runtimeServers) != len(regSnap.Servers) {
		needsRevalidate = true
	}
	return cachedCatalogState{
		Catalog:         ir.BuildCatalog(runtimeServers),
		Registry:        regSnap,
		Available:       true,
		NeedsRevalidate: needsRevalidate,
	}
}

func (l EnvironmentLoader) lookup(key string) (string, bool) {
	if l.LookupEnv == nil {
		return "", false
	}
	value, ok := l.LookupEnv(key)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}
