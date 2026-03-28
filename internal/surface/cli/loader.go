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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

const (
	CatalogFixtureEnv    = "DWS_CATALOG_FIXTURE"
	CacheDirEnv          = "DWS_CACHE_DIR"
	DefaultMarketBaseURL = "https://mcp.dingtalk.com"

	// defaultDiscoveryTimeout bounds the time spent on live registry discovery.
	defaultDiscoveryTimeout = 10 * time.Second

	backgroundRefreshEnv = "DWS_BACKGROUND_REFRESH"
	discoveryBaseURLEnv  = "DWS_DISCOVERY_BASE_URL"
	goTestRefreshDelay   = 10 * time.Millisecond
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
	// AuthToken optionally overrides the access token used for live runtime
	// discovery. When empty, the loader resolves the locally configured token.
	AuthToken string
	// ServerObserver receives the registry servers used for canonical bootstrap.
	// Callers can use this to publish dynamic endpoint/product state without a
	// secondary compat discovery path.
	ServerObserver func([]market.ServerDescriptor)
	// CatalogBaseURLOverride allows tests to redirect catalog discovery.
	CatalogBaseURLOverride string
	// RequireLiveCatalog forces a live discovery attempt before accepting any
	// cached catalog. This is used by generation flows that must avoid
	// short-circuiting on stale or partial cache snapshots.
	RequireLiveCatalog bool
	// RequireCompleteCatalog enforces that all registered services must be
	// successfully discovered and cached. When enabled, partial discovery
	// results (e.g. some servers failed runtime negotiation) cause Load to
	// return an error instead of silently degrading. This is used for first-
	// run bootstrap and cache warmup flows.
	RequireCompleteCatalog bool
	// ForceRefreshAll bypasses the updatedAt-based change detection and forces
	// every non-skip service through runtime discovery, ignoring cached tools
	// snapshots. This is used by generate-skills to guarantee that the
	// generated artifacts reflect the latest MCP service state.
	ForceRefreshAll bool
	// DiscoveryTimeout overrides the default timeout for live registry discovery.
	// Zero means use defaultDiscoveryTimeout.
	DiscoveryTimeout time.Duration
}

type cachedCatalogState struct {
	Catalog            ir.Catalog
	Registry           cache.RegistrySnapshot
	Available          bool
	NeedsRevalidate    bool
	NeedsDetailRefresh bool
}

func NewEnvironmentLoader() EnvironmentLoader {
	return EnvironmentLoader{LookupEnv: os.LookupEnv}
}

func (l EnvironmentLoader) discoveryTimeout() time.Duration {
	if l.DiscoveryTimeout > 0 {
		return l.DiscoveryTimeout
	}
	return defaultDiscoveryTimeout
}

func (l EnvironmentLoader) Load(ctx context.Context) (ir.Catalog, error) {
	if fixturePath, ok := l.lookup(CatalogFixtureEnv); ok {
		l.observeServers(nil)
		return FixtureLoader{Path: fixturePath}.Load(ctx)
	}

	baseURL := DefaultMarketBaseURL
	if l.CatalogBaseURLOverride != "" {
		baseURL = l.CatalogBaseURLOverride
	}

	cacheDir, _ := l.lookup(CacheDirEnv)
	store := cache.NewStore(cacheDir)
	const partition = "default/default"

	cached := l.loadFromCache(store)
	if cached.Available && !l.RequireLiveCatalog && !cached.NeedsRevalidate {
		l.observeServers(cached.Registry.Servers)
		if cached.NeedsDetailRefresh && !backgroundRefreshDisabled() {
			l.asyncRefreshDetail(baseURL, cacheDir, cached.Registry.Servers)
		}
		return cached.Catalog, nil
	}

	transportClient := transport.NewClient(nil)
	transportClient.AuthToken = l.resolveAuthToken(ctx)

	// Use a bounded context so discovery doesn't hang in test or CI environments.
	timeout := l.discoveryTimeout()
	discoverCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	service := discovery.NewService(
		market.NewClient(baseURL, nil),
		transportClient,
		store,
	)
	service.AllowLiveDetailFetch = true
	response, err := service.MarketClient.FetchServers(discoverCtx, 200)
	if err != nil {
		// Graceful degradation: return cached or empty catalog on discovery failure.
		if cached.Available {
			l.observeServers(cached.Registry.Servers)
			return cached.Catalog, nil
		}
		l.observeServers(nil)
		return ir.Catalog{}, nil
	}

	servers := market.NormalizeServers(response, "live_market")
	_ = store.SaveRegistry(partition, cache.RegistrySnapshot{Servers: servers})
	l.observeServers(servers)

	changedKeys := cache.ChangedServerKeysByUpdatedAt(cached.Registry.Servers, servers)
	unchangedRuntime := make(map[string]discovery.RuntimeServer)
	toRefresh := make([]market.ServerDescriptor, 0, len(servers))
	for _, server := range servers {
		// Skip servers marked as cli.skip — no runtime discovery, no CLI support.
		if server.HasCLIMeta && server.CLI.Skip {
			continue
		}
		// When ForceRefreshAll is set, bypass change detection and always
		// refresh every service via live runtime discovery.
		if l.ForceRefreshAll {
			toRefresh = append(toRefresh, server)
			continue
		}
		if changedKeys[server.Key] {
			toRefresh = append(toRefresh, server)
			continue
		}
		toolsSnap, freshness, loadErr := store.LoadTools(partition, server.Key)
		if loadErr != nil || freshness != cache.FreshnessFresh {
			toRefresh = append(toRefresh, server)
			continue
		}
		if requiresSynchronousDetailHydration(server) && !hasFreshCachedDetail(store, partition, server) {
			toRefresh = append(toRefresh, server)
			continue
		}
		unchangedRuntime[server.Key] = enrichRuntimeServerWithCachedDetail(store, partition, discovery.RuntimeServer{
			Server:                    server,
			NegotiatedProtocolVersion: toolsSnap.ProtocolVersion,
			Tools:                     toolsSnap.Tools,
			Source:                    "fresh_cache",
			Degraded:                  false,
		})
	}

	refreshed, failures := service.DiscoverAllRuntime(discoverCtx, toRefresh)
	existingRuntime := append(runtimeServersFromMap(unchangedRuntime), refreshed...)
	recoveredRuntime := recoverRuntimeServersFromDetail(discoverCtx, servers, existingRuntime, store, partition, service.DiscoverDetail)
	degradedRuntime := append([]discovery.RuntimeServer{}, recoveredRuntime...)
	degradedRuntime = append(degradedRuntime, synthesizeRuntimeServersFromRegistryCLI(servers, append(existingRuntime, recoveredRuntime...), store, partition)...)
	if len(unchangedRuntime) == 0 && len(refreshed) == 0 && len(failures) > 0 {
		if l.RequireCompleteCatalog {
			return ir.Catalog{}, fmt.Errorf("complete catalog required but all %d services failed runtime discovery", len(failures))
		}
		if cached.Available {
			return cached.Catalog, nil
		}
		if len(degradedRuntime) > 0 {
			return ir.BuildCatalog(degradedRuntime), nil
		}
		if l.RequireLiveCatalog {
			return ir.Catalog{}, fmt.Errorf("live runtime discovery failed and no cached/live detail recovery is available")
		}
		return ir.Catalog{}, nil
	}

	if l.RequireCompleteCatalog && len(failures) > 0 {
		failedKeys := make([]string, 0, len(failures))
		for _, failure := range failures {
			failedKeys = append(failedKeys, failure.ServerKey)
		}
		return ir.Catalog{}, fmt.Errorf("complete catalog required but %d/%d services failed: %s",
			len(failures), len(toRefresh), strings.Join(failedKeys, ", "))
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
	runtimeServers = append(runtimeServers, recoveredRuntime...)
	runtimeServers = append(runtimeServers, synthesizeRuntimeServersFromRegistryCLI(servers, runtimeServers, store, partition)...)
	if !l.RequireLiveCatalog && len(servers) > 0 && !backgroundRefreshDisabled() {
		l.asyncRefreshDetail(baseURL, cacheDir, servers)
	}
	return ir.BuildCatalog(runtimeServers), nil
}

// loadFromCache builds a catalog from cached registry + tools snapshots.
// Cached registry data is trusted until TTL expiry; regular startup should
// not synchronously revalidate the service list on every invocation.
func (l EnvironmentLoader) loadFromCache(store *cache.Store) cachedCatalogState {
	partition := "default/default"
	regSnap, freshness, err := store.LoadRegistry(partition)
	if err != nil || !regSnap.Complete || len(regSnap.Servers) == 0 {
		return cachedCatalogState{}
	}

	needsRevalidate := freshness == cache.FreshnessStale
	needsDetailRefresh := false
	runtimeServers := make([]discovery.RuntimeServer, 0, len(regSnap.Servers))
	expectedRuntimeServers := 0
	for _, server := range regSnap.Servers {
		// Skip servers marked as cli.skip — no runtime loading from cache either.
		if server.HasCLIMeta && server.CLI.Skip {
			continue
		}
		expectedRuntimeServers++
		toolsSnap, toolsFreshness, toolsErr := store.LoadTools(partition, server.Key)
		if toolsErr != nil || toolsFreshness != cache.FreshnessFresh {
			needsRevalidate = true
			continue
		}
		if requiresSynchronousDetailHydration(server) && !hasFreshCachedDetail(store, partition, server) {
			needsDetailRefresh = true
		}
		runtimeServers = append(runtimeServers, enrichRuntimeServerWithCachedDetail(store, partition, discovery.RuntimeServer{
			Server:                    server,
			NegotiatedProtocolVersion: toolsSnap.ProtocolVersion,
			Tools:                     toolsSnap.Tools,
			Source:                    "fresh_cache",
			Degraded:                  false,
		}))
	}
	incomplete := len(runtimeServers) != expectedRuntimeServers
	if incomplete {
		needsRevalidate = true
	}
	return cachedCatalogState{
		Catalog:            ir.BuildCatalog(runtimeServers),
		Registry:           regSnap,
		Available:          !incomplete,
		NeedsRevalidate:    needsRevalidate,
		NeedsDetailRefresh: needsDetailRefresh,
	}
}

func requiresSynchronousDetailHydration(server market.ServerDescriptor) bool {
	return server.DetailLocator.MCPID > 0 || strings.TrimSpace(server.DetailLocator.DetailURL) != ""
}

func hasFreshCachedDetail(store *cache.Store, partition string, server market.ServerDescriptor) bool {
	if store == nil || !requiresSynchronousDetailHydration(server) {
		return false
	}
	serverKey := strings.TrimSpace(server.Key)
	cliID := strings.TrimSpace(server.CLI.ID)
	for _, cacheKey := range detailSnapshotKeys(serverKey, cliID) {
		usingAlias := cacheKey != serverKey && cacheKey == cliID
		if server.DetailLocator.MCPID <= 0 && usingAlias {
			continue
		}
		snapshot, freshness, err := store.LoadDetail(partition, cacheKey)
		if err != nil || freshness != cache.FreshnessFresh {
			continue
		}
		if server.DetailLocator.MCPID > 0 {
			if usingAlias {
				if snapshot.MCPID != server.DetailLocator.MCPID {
					continue
				}
			} else if snapshot.MCPID != 0 && snapshot.MCPID != server.DetailLocator.MCPID {
				continue
			}
		}
		var detail market.DetailResponse
		if err := json.Unmarshal(snapshot.Payload, &detail); err != nil {
			continue
		}
		if detail.Success && len(detail.Result.Tools) > 0 {
			return true
		}
	}
	return false
}

func detailSnapshotKeys(serverKey, cliID string) []string {
	keys := make([]string, 0, 2)
	if serverKey != "" {
		keys = append(keys, serverKey)
	}
	if cliID != "" && cliID != serverKey {
		keys = append(keys, cliID)
	}
	return keys
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

func (l EnvironmentLoader) resolveAuthToken(ctx context.Context) string {
	if token := strings.TrimSpace(l.AuthToken); token != "" {
		return token
	}
	return resolveCatalogAuthToken(ctx)
}

func resolveCatalogAuthToken(ctx context.Context) string {
	configDir := environmentLoaderConfigDir()
	provider := authpkg.NewOAuthProvider(configDir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	token, tokenErr := provider.GetAccessToken(ctx)
	if tokenErr == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token)
	}
	if tokenErr != nil && errors.Is(tokenErr, authpkg.ErrTokenDecryption) {
		slog.Error(tokenErr.Error())
		return ""
	}

	hasSecureTokenData := false
	if tokenErr != nil {
		if _, loadErr := authpkg.LoadTokenData(configDir); loadErr == nil {
			hasSecureTokenData = true
		}
	}
	if hasSecureTokenData {
		slog.Warn("access token expired and refresh failed, please run: dws auth login", "error", tokenErr)
		return ""
	}

	manager := authpkg.NewManager(configDir, nil)
	if token, _, err := manager.GetToken(); err == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token)
	}
	return ""
}

func environmentLoaderConfigDir() string {
	if envDir := strings.TrimSpace(os.Getenv("DWS_CONFIG_DIR")); envDir != "" {
		return envDir
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return environmentLoaderExeRelativeConfigDir()
	}
	return filepath.Join(homeDir, ".dws")
}

func environmentLoaderExeRelativeConfigDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return ".dws"
	}
	realPath, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		realPath = exePath
	}
	return filepath.Join(filepath.Dir(realPath), ".dws")
}

func (l EnvironmentLoader) observeServers(servers []market.ServerDescriptor) {
	if l.ServerObserver == nil {
		return
	}
	cloned := append([]market.ServerDescriptor(nil), servers...)
	l.ServerObserver(cloned)
}

func (l EnvironmentLoader) asyncRefreshDetail(baseURL, cacheDir string, servers []market.ServerDescriptor) {
	if backgroundRefreshDisabled() {
		return
	}
	if !runningUnderGoTest() {
		spawnDetachedCacheRefresh(cacheDir, baseURL)
		return
	}
	servers = append([]market.ServerDescriptor(nil), servers...)
	go func() {
		time.Sleep(goTestRefreshDelay)
		store := cache.NewStore(cacheDir)
		service := discovery.NewService(
			market.NewClient(baseURL, nil),
			transport.NewClient(nil),
			store,
		)
		service.AllowLiveDetailFetch = true
		ctx, cancel := context.WithTimeout(context.Background(), l.discoveryTimeout())
		defer cancel()
		for _, server := range servers {
			if server.DetailLocator.MCPID <= 0 && strings.TrimSpace(server.DetailLocator.DetailURL) == "" {
				continue
			}
			_, _ = service.DiscoverDetail(ctx, server)
		}
	}()
}

func backgroundRefreshDisabled() bool {
	if strings.TrimSpace(os.Getenv(backgroundRefreshEnv)) != "" {
		return true
	}
	if len(os.Args) >= 3 && os.Args[1] == "cache" && os.Args[2] == "refresh" {
		return true
	}
	return false
}

func runningUnderGoTest() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}

func spawnDetachedCacheRefresh(cacheDir, baseURL string) {
	executable, err := os.Executable()
	if err != nil {
		return
	}

	cmd := exec.Command(executable, "cache", "refresh")
	cmd.Env = detachedCacheRefreshEnv(cacheDir, baseURL, os.Environ(), os.Args)
	if devNull, openErr := os.OpenFile(os.DevNull, os.O_WRONLY, 0); openErr == nil {
		cmd.Stdout = devNull
		cmd.Stderr = devNull
	}
	if err := cmd.Start(); err != nil {
		return
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
}

func detachedCacheRefreshEnv(cacheDir, baseURL string, parentEnv, argv []string) []string {
	env := append([]string{}, parentEnv...)
	env = append(env, backgroundRefreshEnv+"=1")
	if strings.TrimSpace(cacheDir) != "" {
		env = append(env, CacheDirEnv+"="+cacheDir)
	}
	if strings.TrimSpace(baseURL) != "" {
		env = append(env, discoveryBaseURLEnv+"="+baseURL)
	}
	return env
}

func enrichRuntimeServerWithCachedDetail(store *cache.Store, partition string, runtimeServer discovery.RuntimeServer) discovery.RuntimeServer {
	if detail, ok := discovery.LoadCachedDetail(store, partition, runtimeServer.Server); ok {
		runtimeServer.Tools = discovery.EnrichRuntimeToolsWithDetail(runtimeServer.Tools, detail)
	}
	return runtimeServer
}

func runtimeServersFromMap(servers map[string]discovery.RuntimeServer) []discovery.RuntimeServer {
	if len(servers) == 0 {
		return nil
	}
	out := make([]discovery.RuntimeServer, 0, len(servers))
	for _, runtimeServer := range servers {
		out = append(out, runtimeServer)
	}
	return out
}

func recoverRuntimeServersFromDetail(
	ctx context.Context,
	servers []market.ServerDescriptor,
	existing []discovery.RuntimeServer,
	store *cache.Store,
	partition string,
	discoverDetail func(context.Context, market.ServerDescriptor) (market.DetailResponse, error),
) []discovery.RuntimeServer {
	existingKeys := make(map[string]struct{}, len(existing))
	for _, runtimeServer := range existing {
		existingKeys[runtimeServer.Server.Key] = struct{}{}
	}

	out := make([]discovery.RuntimeServer, 0, len(servers))
	for _, server := range servers {
		if _, ok := existingKeys[server.Key]; ok {
			continue
		}
		if server.HasCLIMeta && server.CLI.Skip {
			continue
		}
		tools := synthesizeToolsFromCLIOverlay(server.CLI)
		if len(tools) == 0 {
			continue
		}

		detail, ok := discovery.LoadCachedDetail(store, partition, server)
		if !ok && discoverDetail != nil && (server.DetailLocator.MCPID > 0 || strings.TrimSpace(server.DetailLocator.DetailURL) != "") {
			fetched, err := discoverDetail(ctx, server)
			if err == nil {
				detail = fetched
				ok = true
			}
		}
		if !ok {
			continue
		}
		for idx := range tools {
			// CLI overlay only knows flag names. When recovering from detail, let the
			// live/cached detail schema fully define descriptions and required fields.
			tools[idx].InputSchema = nil
			tools[idx].OutputSchema = nil
		}
		tools = discovery.EnrichRuntimeToolsWithDetail(tools, detail)
		if len(tools) == 0 {
			continue
		}
		syntheticServer := server
		syntheticServer.Source = "detail_recovery"
		syntheticServer.Degraded = true
		out = append(out, discovery.RuntimeServer{
			Server:   syntheticServer,
			Tools:    tools,
			Source:   "detail_recovery",
			Degraded: true,
		})
	}
	return out
}

func synthesizeRuntimeServersFromRegistryCLI(servers []market.ServerDescriptor, existing []discovery.RuntimeServer, store *cache.Store, partition string) []discovery.RuntimeServer {
	existingKeys := make(map[string]struct{}, len(existing))
	for _, runtimeServer := range existing {
		existingKeys[runtimeServer.Server.Key] = struct{}{}
	}

	out := make([]discovery.RuntimeServer, 0, len(servers))
	for _, server := range servers {
		if _, ok := existingKeys[server.Key]; ok {
			continue
		}
		if server.HasCLIMeta && server.CLI.Skip {
			continue
		}
		tools := synthesizeToolsFromCLIOverlay(server.CLI)
		if detail, ok := discovery.LoadCachedDetail(store, partition, server); ok {
			tools = discovery.EnrichRuntimeToolsWithDetail(tools, detail)
		}
		if len(tools) == 0 {
			continue
		}
		syntheticServer := server
		syntheticServer.Source = "registry_cli_fallback"
		syntheticServer.Degraded = true
		out = append(out, discovery.RuntimeServer{
			Server:   syntheticServer,
			Tools:    tools,
			Source:   "registry_cli_fallback",
			Degraded: true,
		})
	}
	return out
}

func synthesizeToolsFromCLIOverlay(cli market.CLIOverlay) []transport.ToolDescriptor {
	cliToolsByName := make(map[string]market.CLITool, len(cli.Tools))
	toolNames := make([]string, 0, len(cli.Tools)+len(cli.ToolOverrides))
	seen := make(map[string]struct{}, len(cli.Tools)+len(cli.ToolOverrides))

	for _, cliTool := range cli.Tools {
		name := strings.TrimSpace(cliTool.Name)
		if name == "" {
			continue
		}
		cliToolsByName[name] = cliTool
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		toolNames = append(toolNames, name)
	}
	for name := range cli.ToolOverrides {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		toolNames = append(toolNames, name)
	}
	sort.Strings(toolNames)

	tools := make([]transport.ToolDescriptor, 0, len(toolNames))
	for _, name := range toolNames {
		override := cli.ToolOverrides[name]
		cliTool := cliToolsByName[name]
		title := strings.TrimSpace(cliTool.Title)
		if title == "" {
			title = fallbackToolLabel(override.Group, firstNonEmpty(override.CLIName, cliTool.CLIName, name))
		}
		description := strings.TrimSpace(cliTool.Description)
		if description == "" {
			description = title
		}

		sensitive := false
		if override.IsSensitive != nil {
			sensitive = *override.IsSensitive
		}
		if cliTool.IsSensitive != nil {
			sensitive = *cliTool.IsSensitive
		}

		tools = append(tools, transport.ToolDescriptor{
			Name:        name,
			Title:       title,
			Description: description,
			InputSchema: synthesizeInputSchema(cliTool, override),
			Sensitive:   sensitive,
		})
	}
	return tools
}

func synthesizeInputSchema(cliTool market.CLITool, override market.CLIToolOverride) map[string]any {
	properties := make(map[string]any)

	propNames := make([]string, 0, len(cliTool.Flags)+len(override.Flags))
	seen := make(map[string]struct{}, len(cliTool.Flags)+len(override.Flags))
	for name := range cliTool.Flags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
		propNames = append(propNames, name)
	}
	for name := range override.Flags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		propNames = append(propNames, name)
	}
	sort.Strings(propNames)

	for _, name := range propNames {
		properties[name] = map[string]any{
			"type":        "string",
			"description": name,
		}
	}

	schema := map[string]any{
		"type": "object",
	}
	if len(properties) > 0 {
		schema["properties"] = properties
	}
	return schema
}

func fallbackToolLabel(group, cliName string) string {
	group = strings.TrimSpace(strings.ReplaceAll(group, ".", " "))
	cliName = strings.TrimSpace(cliName)
	switch {
	case group != "" && cliName != "":
		return group + " " + cliName
	case cliName != "":
		return cliName
	default:
		return strings.TrimSpace(group)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
