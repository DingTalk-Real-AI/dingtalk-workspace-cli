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
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name:         "DWS_CACHE_DIR",
		Category:     configmeta.CategoryCore,
		Description:  "覆盖缓存目录",
		DefaultValue: "~/.dws/cache",
		Example:      "/tmp/dws-cache",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DWS_CATALOG_FIXTURE",
		Category:    configmeta.CategoryDebug,
		Description: "使用本地 JSON 文件替代在线目录发现",
		Example:     "/path/to/catalog.json",
		Hidden:      true,
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:         "DWS_PLUGIN_COLD_TIMEOUT",
		Category:     configmeta.CategoryCore,
		Description:  "插件 MCP 冷启动发现的超时时长（Go duration 格式，如 2s / 1500ms）。设置后同时覆盖 HTTP 与 stdio 插件的冷启动预算；未设置时使用内置默认值（HTTP 无鉴权 1s / 有鉴权 1.5s / stdio 2s）。",
		DefaultValue: "",
		Example:      "3s",
	})
}

// CatalogDegradedReason identifies why catalog discovery returned empty.
type CatalogDegradedReason string

const (
	DegradedUnauthenticated   CatalogDegradedReason = "unauthenticated"
	DegradedMarketUnreachable CatalogDegradedReason = "market_unreachable"
	DegradedRuntimeAllFailed  CatalogDegradedReason = "runtime_all_failed"
)

// CatalogDegraded is returned by EnvironmentLoader.Load when discovery
// fails for a diagnosable reason. Callers that need graceful degradation
// (e.g. the runtime runner) can check errors.As and fall back to an
// empty catalog; callers like the schema command can surface the hint.
type CatalogDegraded struct {
	Reason      CatalogDegradedReason
	Hint        string
	ServerCount int // number of servers discovered (only set for runtime_all_failed)
}

func (e *CatalogDegraded) Error() string { return string(e.Reason) + ": " + e.Hint }

func degradedHint(reason CatalogDegradedReason, serverCount int) string {
	embedded := edition.Get().IsEmbedded
	switch reason {
	case DegradedUnauthenticated:
		if embedded {
			return "未登录，请重新认证"
		}
		return "未登录，无法发现 MCP 服务。请先执行: dws auth login"
	case DegradedMarketUnreachable:
		if embedded {
			return "无法连接 MCP 市场，请检查网络"
		}
		return "无法连接 MCP 市场 (mcp.dingtalk.com)，请检查网络"
	case DegradedRuntimeAllFailed:
		if embedded {
			return fmt.Sprintf("已发现 %d 个服务但连接全部失败，请稍后重试", serverCount)
		}
		return fmt.Sprintf("已发现 %d 个服务但连接全部失败，请稍后重试或执行: dws cache refresh", serverCount)
	default:
		return "MCP 服务发现失败"
	}
}

func newCatalogDegraded(reason CatalogDegradedReason, serverCount int) *CatalogDegraded {
	return &CatalogDegraded{
		Reason:      reason,
		Hint:        degradedHint(reason, serverCount),
		ServerCount: serverCount,
	}
}

const (
	CatalogFixtureEnv    = "DWS_CATALOG_FIXTURE"
	CacheDirEnv          = "DWS_CACHE_DIR"
	PluginColdTimeoutEnv = "DWS_PLUGIN_COLD_TIMEOUT"
	DefaultMarketBaseURL = "https://mcp.dingtalk.com"

	// defaultDiscoveryTimeout bounds the time spent on live registry discovery.
	// Tightened to 4s so a slow/unreachable discovery endpoint cannot block
	// every CLI command invocation. See issue #119.
	defaultDiscoveryTimeout = 4 * time.Second
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
	// LoggerFunc returns a structured logger for discovery diagnostics.
	// Called lazily because the file logger may not be initialized at
	// construction time (it's set up during PersistentPreRunE).
	LoggerFunc func() *slog.Logger
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
	if cached.Available && len(cached.Catalog.Products) > 0 {
		return cached.Catalog, nil
	}

	transportClient := transport.NewClient(nil)
	hasAuth := false
	if l.AuthTokenFunc != nil {
		if token := l.AuthTokenFunc(ctx); token != "" {
			transportClient = transportClient.WithAuth(token, nil)
			hasAuth = true
		}
	}

	if !hasAuth {
		return ir.Catalog{}, newCatalogDegraded(DegradedUnauthenticated, 0)
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
	if l.LoggerFunc != nil {
		service.Logger = l.LoggerFunc()
	}
	response, err := service.MarketClient.FetchServers(discoverCtx, 200)
	if err != nil {
		if cached.Available && len(cached.Catalog.Products) > 0 {
			return cached.Catalog, nil
		}
		return ir.Catalog{}, newCatalogDegraded(DegradedMarketUnreachable, 0)
	}

	// Surface Portal-side merge warnings (dropped envelopes, dangling serverDeps,
	// dangling toolOverrides.*.serverOverride) to stderr so configuration drift
	// is visible at the first `dws cache refresh` / cold start after Portal
	// publishes a broken envelope. Non-fatal: discovery continues with the
	// accepted subset. See plan fix-wukong-discovery-missing-servers Phase 4.3.
	logDiscoveryWarnings(response.Metadata.Warnings)

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
		if cached.Available && len(cached.Catalog.Products) > 0 {
			return cached.Catalog, nil
		}
		return ir.Catalog{}, newCatalogDegraded(DegradedRuntimeAllFailed, len(servers))
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

// logDiscoveryWarnings prints Portal-side merge warnings via slog (stderr).
// No-op when the response carries an empty / nil Warnings slice — which is the
// common case for older Portal builds that don't populate the field.
func logDiscoveryWarnings(warnings []market.ListWarning) {
	if len(warnings) == 0 {
		return
	}
	for _, w := range warnings {
		slog.Warn("discovery: merge warning from Portal",
			"product", w.ProductID,
			"reason", w.Reason,
			"detail", w.Detail,
		)
	}
}
