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

package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

const (
	tenantEnv       = "DWS_TENANT"
	authIdentityEnv = "DWS_AUTH_IDENTITY"
)

var errCLIServerSkipped = errors.New("server marked cli.skip")

type Service struct {
	MarketClient     *market.Client
	Transport        *transport.Client
	Cache            *cache.Store
	Tenant           string
	AuthIdentity     string
	Logger           *slog.Logger
	PerServerTimeout time.Duration // overrides perServerDiscoveryTimeout when > 0
}

type RuntimeServer struct {
	Server                    market.ServerDescriptor    `json:"server"`
	NegotiatedProtocolVersion string                     `json:"negotiated_protocol_version"`
	Tools                     []transport.ToolDescriptor `json:"tools"`
	Source                    string                     `json:"source"`
	Degraded                  bool                       `json:"degraded"`
}

type RuntimeFailure struct {
	ServerKey string
	Err       error
}

func NewService(marketClient *market.Client, transportClient *transport.Client, cacheStore *cache.Store) *Service {
	return &Service{
		MarketClient: marketClient,
		Transport:    transportClient,
		Cache:        cacheStore,
		Tenant:       resolveTenant(),
		AuthIdentity: resolveAuthIdentity(),
	}
}

func (s *Service) DiscoverServers(ctx context.Context) ([]market.ServerDescriptor, error) {
	partition := s.partition()

	response, err := s.MarketClient.FetchServers(ctx, 200)
	if err == nil {
		servers := market.NormalizeServers(response, "live_market")
		_ = s.Cache.SaveRegistry(partition, cache.RegistrySnapshot{Servers: servers})
		return servers, nil
	}

	snapshot, freshness, cacheErr := s.Cache.LoadRegistry(partition)
	if cacheErr == nil {
		servers := append([]market.ServerDescriptor(nil), snapshot.Servers...)
		for idx := range servers {
			servers[idx].Source = string(freshness) + "_cache"
			servers[idx].Degraded = true
		}
		return servers, nil
	}

	return nil, fmt.Errorf("discover servers: market fetch failed and no cache available: %w", err)
}

func (s *Service) DiscoverServerRuntime(ctx context.Context, server market.ServerDescriptor) (RuntimeServer, error) {
	if server.CLI.Skip {
		return RuntimeServer{}, errCLIServerSkipped
	}

	partition := s.partition()

	initialize, err := s.Transport.Initialize(ctx, server.Endpoint)
	if err == nil {
		// NotifyInitialized is best-effort; log but do not fail on error.
		if notifyErr := s.Transport.NotifyInitialized(ctx, server.Endpoint); notifyErr != nil && s.Logger != nil {
			s.Logger.Debug("NotifyInitialized failed", "server", server.Key, "error", notifyErr)
		}
		tools, listErr := s.Transport.ListTools(ctx, server.Endpoint)
		if listErr == nil {
			runtimeTools := append([]transport.ToolDescriptor(nil), tools.Tools...)
			var actionVersions map[string]string
			if server.DetailLocator.MCPID > 0 {
				if detail, detailErr := s.DiscoverDetail(ctx, server); detailErr == nil {
					runtimeTools = mergeRuntimeToolsWithDetail(runtimeTools, detail)
					actionVersions = cache.ExtractActionVersions(detail.Result.Tools)
				}
			}
			_ = s.Cache.SaveTools(partition, server.Key, cache.ToolsSnapshot{
				ServerKey:       server.Key,
				ProtocolVersion: initialize.ProtocolVersion,
				Tools:           runtimeTools,
				ActionVersions:  actionVersions,
			})
			server.NegotiatedProtocolVersion = initialize.ProtocolVersion
			server.Source = "live_runtime"
			return RuntimeServer{
				Server:                    server,
				NegotiatedProtocolVersion: initialize.ProtocolVersion,
				Tools:                     runtimeTools,
				Source:                    "live_runtime",
				Degraded:                  false,
			}, nil
		}
		err = listErr
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return RuntimeServer{}, err
	}

	snapshot, freshness, cacheErr := s.Cache.LoadTools(partition, server.Key)
	if cacheErr != nil {
		return RuntimeServer{}, fmt.Errorf("server %s: runtime discovery failed and no cache available: %w", server.Key, err)
	}

	server.NegotiatedProtocolVersion = snapshot.ProtocolVersion
	server.Source = string(freshness) + "_cache"
	server.Degraded = true
	return RuntimeServer{
		Server:                    server,
		NegotiatedProtocolVersion: snapshot.ProtocolVersion,
		Tools:                     snapshot.Tools,
		Source:                    string(freshness) + "_cache",
		Degraded:                  true,
	}, nil
}

const perServerDiscoveryTimeout = 2 * time.Second

func (s *Service) DiscoverAllRuntime(ctx context.Context, servers []market.ServerDescriptor) ([]RuntimeServer, []RuntimeFailure) {
	type discoveryResult struct {
		server  RuntimeServer
		failure *RuntimeFailure
	}

	filtered := make([]market.ServerDescriptor, 0, len(servers))
	for _, srv := range servers {
		if !srv.CLI.Skip {
			filtered = append(filtered, srv)
		}
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	ch := make(chan discoveryResult, len(filtered))
	serverTimeout := s.PerServerTimeout
	if serverTimeout <= 0 {
		serverTimeout = perServerDiscoveryTimeout
	}
	var wg sync.WaitGroup
	for _, srv := range filtered {
		wg.Add(1)
		go func(server market.ServerDescriptor) {
			defer wg.Done()
			serverCtx, cancel := context.WithTimeout(ctx, serverTimeout)
			defer cancel()
			start := time.Now()
			rs, err := s.DiscoverServerRuntime(serverCtx, server)
			elapsed := time.Since(start)
			if err != nil {
				if errors.Is(err, errCLIServerSkipped) {
					return
				}
				if s.Logger != nil {
					s.Logger.Warn("server_discovery_failed",
						slog.String("server_key", server.Key),
						slog.String("duration", elapsed.Truncate(time.Millisecond).String()),
						slog.String("error", err.Error()),
						slog.Bool("is_timeout", errors.Is(err, context.DeadlineExceeded)),
					)
				}
				// Per-server sub-context timed out but parent is still alive:
				// try cache fallback instead of reporting a hard failure.
				if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
					if cached, cacheErr := s.loadServerFromCache(server); cacheErr == nil {
						if s.Logger != nil {
							s.Logger.Info("server_discovery_cache_fallback",
								slog.String("server_key", server.Key),
								slog.String("source", cached.Source),
							)
						}
						ch <- discoveryResult{server: cached}
						return
					}
				}
				ch <- discoveryResult{failure: &RuntimeFailure{ServerKey: server.Key, Err: err}}
				return
			}
			if s.Logger != nil {
				s.Logger.Debug("server_discovery_ok",
					slog.String("server_key", server.Key),
					slog.String("duration", elapsed.Truncate(time.Millisecond).String()),
					slog.String("source", rs.Source),
				)
			}
			ch <- discoveryResult{server: rs}
		}(srv)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	results := make([]RuntimeServer, 0, len(filtered))
	failures := make([]RuntimeFailure, 0)
	for dr := range ch {
		if dr.failure != nil {
			failures = append(failures, *dr.failure)
		} else {
			results = append(results, dr.server)
		}
	}
	return results, failures
}

// loadServerFromCache tries to load a server's tools from cache, returning a
// degraded RuntimeServer. Used as fallback when a per-server discovery timeout
// fires but the parent context is still alive.
func (s *Service) loadServerFromCache(server market.ServerDescriptor) (RuntimeServer, error) {
	partition := s.partition()
	snapshot, freshness, err := s.Cache.LoadTools(partition, server.Key)
	if err != nil {
		return RuntimeServer{}, err
	}
	server.NegotiatedProtocolVersion = snapshot.ProtocolVersion
	server.Source = string(freshness) + "_cache"
	server.Degraded = true
	return RuntimeServer{
		Server:                    server,
		NegotiatedProtocolVersion: snapshot.ProtocolVersion,
		Tools:                     snapshot.Tools,
		Source:                    string(freshness) + "_cache",
		Degraded:                  true,
	}, nil
}

func (s *Service) DiscoverDetail(ctx context.Context, server market.ServerDescriptor) (market.DetailResponse, error) {
	partition := s.partition()
	var fetchErr error

	if detailURL := strings.TrimSpace(server.DetailLocator.DetailURL); detailURL != "" {
		detail, err := s.MarketClient.FetchDetailByURL(ctx, detailURL)
		if err == nil {
			cacheDetailSnapshot(s.Cache, partition, server, server.DetailLocator.MCPID, detail)
			s.invalidateToolsIfVersionChanged(partition, server.Key, detail)
			return detail, nil
		}
		fetchErr = err
	}
	if server.DetailLocator.MCPID > 0 {
		detail, err := s.MarketClient.FetchDetail(ctx, server.DetailLocator.MCPID)
		if err == nil {
			cacheDetailSnapshot(s.Cache, partition, server, server.DetailLocator.MCPID, detail)
			s.invalidateToolsIfVersionChanged(partition, server.Key, detail)
			return detail, nil
		}
		fetchErr = err
	}
	if fetchErr == nil {
		fetchErr = fmt.Errorf("server %s does not expose detail locator", server.Key)
	}

	snapshot, _, cacheErr := s.Cache.LoadDetail(partition, server.Key)
	if cacheErr != nil {
		return market.DetailResponse{}, fetchErr
	}
	if server.DetailLocator.MCPID > 0 && snapshot.MCPID != server.DetailLocator.MCPID {
		return market.DetailResponse{}, fetchErr
	}

	var cached market.DetailResponse
	if unmarshalErr := json.Unmarshal(snapshot.Payload, &cached); unmarshalErr != nil {
		return market.DetailResponse{}, fetchErr
	}
	return cached, nil
}

// invalidateToolsIfVersionChanged checks whether a fresh Detail API response
// contains actionVersion values that differ from those stored in the cached
// tools snapshot. If any tool's version has changed, the tools cache is
// invalidated so the next DiscoverServerRuntime call re-fetches tools/list.
func (s *Service) invalidateToolsIfVersionChanged(partition, serverKey string, detail market.DetailResponse) {
	if !detail.Success || len(detail.Result.Tools) == 0 {
		return
	}
	snapshot, _, err := s.Cache.LoadTools(partition, serverKey)
	if err != nil || len(snapshot.ActionVersions) == 0 {
		return
	}
	if cache.HasActionVersionChanged(snapshot.ActionVersions, detail.Result.Tools) {
		_ = s.Cache.DeleteTools(partition, serverKey)
	}
}

func (s *Service) partition() string {
	return fmt.Sprintf("%s/%s", s.Tenant, s.AuthIdentity)
}

func (s *Service) CachePartition() string {
	return s.partition()
}

func mergeRuntimeToolsWithDetail(tools []transport.ToolDescriptor, detail market.DetailResponse) []transport.ToolDescriptor {
	if !detail.Success || len(detail.Result.Tools) == 0 || len(tools) == 0 {
		return tools
	}

	byName := make(map[string]market.DetailTool, len(detail.Result.Tools))
	for _, tool := range detail.Result.Tools {
		name := strings.TrimSpace(tool.ToolName)
		if name == "" {
			continue
		}
		byName[name] = tool
	}

	out := make([]transport.ToolDescriptor, 0, len(tools))
	for _, tool := range tools {
		merged := tool
		detailTool, ok := byName[strings.TrimSpace(tool.Name)]
		if !ok {
			out = append(out, merged)
			continue
		}
		if title := strings.TrimSpace(detailTool.ToolTitle); title != "" {
			merged.Title = title
		}
		if description := strings.TrimSpace(detailTool.ToolDesc); description != "" {
			merged.Description = description
		}
		merged.Sensitive = detailTool.IsSensitive
		if inputSchema := parseDetailSchema(detailTool.ToolRequest); len(inputSchema) > 0 {
			merged.InputSchema = inputSchema
		}
		if outputSchema := parseDetailSchema(detailTool.ToolResponse); len(outputSchema) > 0 {
			merged.OutputSchema = outputSchema
		}
		out = append(out, merged)
	}
	return out
}

func parseDetailSchema(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	object, ok := parsed.(map[string]any)
	if !ok {
		return nil
	}
	return object
}

func cacheDetailSnapshot(store *cache.Store, partition string, server market.ServerDescriptor, mcpID int, detail market.DetailResponse) {
	if store == nil {
		return
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return
	}
	snapshot := cache.DetailSnapshot{
		MCPID:   mcpID,
		Payload: raw,
	}
	for _, cacheKey := range detailSnapshotKeys(server) {
		_ = store.SaveDetail(partition, cacheKey, snapshot)
	}
}

func detailSnapshotKeys(server market.ServerDescriptor) []string {
	seen := make(map[string]struct{}, 2)
	keys := make([]string, 0, 2)
	for _, candidate := range []string{
		strings.TrimSpace(server.Key),
		strings.TrimSpace(server.CLI.ID),
	} {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		keys = append(keys, candidate)
	}
	return keys
}

func resolveTenant() string {
	value := strings.TrimSpace(os.Getenv(tenantEnv))
	if value == "" {
		return "default"
	}
	return strings.ToLower(value)
}

func resolveAuthIdentity() string {
	if value := strings.TrimSpace(os.Getenv(authIdentityEnv)); value != "" {
		return strings.ToLower(value)
	}
	return "default"
}
