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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

const (
	tenantEnv       = "DWS_TENANT"
	authIdentityEnv = "DWS_AUTH_IDENTITY"
)

var errCLIServerSkipped = errors.New("server marked cli.skip")

type Service struct {
	MarketClient         *market.Client
	Transport            *transport.Client
	Cache                *cache.Store
	Tenant               string
	AuthIdentity         string
	Logger               *slog.Logger
	AllowLiveDetailFetch bool
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

	slog.Debug("DiscoverServers: fetching from market API", "partition", partition)
	response, err := s.MarketClient.FetchServers(ctx, 200)
	if err == nil {
		servers := market.NormalizeServers(response, "live_market")
		slog.Debug("DiscoverServers: fetched from market", "servers", len(servers))
		_ = s.Cache.SaveRegistry(partition, cache.RegistrySnapshot{Servers: servers})
		return servers, nil
	}

	slog.Debug("DiscoverServers: market fetch failed, trying cache", "error", err)
	snapshot, freshness, cacheErr := s.Cache.LoadRegistry(partition)
	if cacheErr == nil && snapshot.Complete && len(snapshot.Servers) > 0 {
		servers := append([]market.ServerDescriptor(nil), snapshot.Servers...)
		for idx := range servers {
			servers[idx].Source = string(freshness) + "_cache"
			servers[idx].Degraded = true
		}
		slog.Debug("DiscoverServers: degraded to cache", "servers", len(servers), "freshness", freshness)
		return servers, nil
	}

	return nil, fmt.Errorf("discover servers: market fetch failed and no cache available: %w", err)
}

func (s *Service) DiscoverServerRuntime(ctx context.Context, server market.ServerDescriptor) (RuntimeServer, error) {
	if server.CLI.Skip {
		return RuntimeServer{}, errCLIServerSkipped
	}

	partition := s.partition()

	slog.Debug("DiscoverServerRuntime: initializing", "server", server.Key, "endpoint", transport.RedactURL(server.Endpoint))
	initialize, err := s.Transport.Initialize(ctx, server.Endpoint)
	if err == nil {
		slog.Debug("DiscoverServerRuntime: initialized", "server", server.Key, "protocol", initialize.ProtocolVersion)
		// NotifyInitialized is best-effort; log but do not fail on error.
		if notifyErr := s.Transport.NotifyInitialized(ctx, server.Endpoint); notifyErr != nil && s.Logger != nil {
			s.Logger.Debug("NotifyInitialized failed", "server", server.Key, "error", notifyErr)
		}
		slog.Debug("DiscoverServerRuntime: listing tools", "server", server.Key)
		tools, listErr := s.Transport.ListTools(ctx, server.Endpoint)
		if listErr == nil {
			runtimeTools := append([]transport.ToolDescriptor(nil), tools.Tools...)
			var actionVersions map[string]string
			if detail, ok := s.detailForRuntime(ctx, partition, server); ok {
				runtimeTools = mergeRuntimeToolsWithDetail(runtimeTools, detail)
				actionVersions = cache.ExtractActionVersions(detail.Result.Tools)
			}
			slog.Debug("DiscoverServerRuntime: tools discovered", "server", server.Key, "tools", len(runtimeTools))
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

	snapshot, freshness, cacheErr := s.Cache.LoadTools(partition, server.Key)
	if cacheErr != nil {
		if errors.Is(cacheErr, context.Canceled) {
			return RuntimeServer{}, cacheErr
		}
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

func (s *Service) DiscoverAllRuntime(ctx context.Context, servers []market.ServerDescriptor) ([]RuntimeServer, []RuntimeFailure) {
	results := make([]RuntimeServer, 0, len(servers))
	failures := make([]RuntimeFailure, 0)
	for _, server := range servers {
		if server.CLI.Skip {
			continue
		}
		runtimeServer, err := s.DiscoverServerRuntime(ctx, server)
		if err != nil {
			if errors.Is(err, errCLIServerSkipped) {
				continue
			}
			failures = append(failures, RuntimeFailure{
				ServerKey: server.Key,
				Err:       err,
			})
			continue
		}
		results = append(results, runtimeServer)
	}
	return results, failures
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

	cached, ok := LoadCachedDetail(s.Cache, partition, server)
	if !ok {
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

func (s *Service) detailForRuntime(ctx context.Context, partition string, server market.ServerDescriptor) (market.DetailResponse, bool) {
	if s.AllowLiveDetailFetch {
		detail, err := s.DiscoverDetail(ctx, server)
		if err == nil {
			return detail, true
		}
		return market.DetailResponse{}, false
	}
	return LoadCachedDetail(s.Cache, partition, server)
}

func EnrichRuntimeToolsWithDetail(tools []transport.ToolDescriptor, detail market.DetailResponse) []transport.ToolDescriptor {
	return mergeRuntimeToolsWithDetail(tools, detail)
}

func LoadCachedDetail(store *cache.Store, partition string, server market.ServerDescriptor) (market.DetailResponse, bool) {
	if store == nil {
		return market.DetailResponse{}, false
	}
	serverKey := strings.TrimSpace(server.Key)
	cliID := strings.TrimSpace(server.CLI.ID)
	for _, cacheKey := range detailSnapshotKeys(server) {
		usingAlias := cacheKey != serverKey && cacheKey == cliID
		if server.DetailLocator.MCPID <= 0 && usingAlias {
			continue
		}
		snapshot, _, err := store.LoadDetail(partition, cacheKey)
		if err != nil {
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
		var cached market.DetailResponse
		if unmarshalErr := json.Unmarshal(snapshot.Payload, &cached); unmarshalErr != nil {
			continue
		}
		if cached.Success && len(cached.Result.Tools) > 0 {
			return cached, true
		}
	}
	return market.DetailResponse{}, false
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
		canUseDetailTitle := toolTitleCanUseDetail(merged)
		canUseDetailDescription := toolDescriptionCanUseDetail(merged)
		if title := strings.TrimSpace(detailTool.ToolTitle); title != "" && canUseDetailTitle {
			merged.Title = title
		}
		if description := strings.TrimSpace(detailTool.ToolDesc); description != "" && canUseDetailDescription {
			merged.Description = description
		}
		if detailTool.IsSensitive {
			merged.Sensitive = true
		}
		if inputSchema := parseDetailSchema(detailTool.ToolRequest); len(inputSchema) > 0 {
			merged.InputSchema = mergeDetailSchema(merged.InputSchema, inputSchema)
		}
		if outputSchema := parseDetailSchema(detailTool.ToolResponse); len(outputSchema) > 0 {
			merged.OutputSchema = mergeDetailSchema(merged.OutputSchema, outputSchema)
		}
		out = append(out, merged)
	}
	return out
}

func mergeDetailSchema(base, detail map[string]any) map[string]any {
	if len(detail) == 0 {
		return cloneSchemaMap(base)
	}
	if len(base) == 0 {
		return cloneSchemaMap(detail)
	}
	merged := cloneSchemaMap(base)
	if merged == nil {
		merged = map[string]any{}
	}
	return mergeDetailSchemaMaps(merged, detail)
}

func mergeDetailSchemaMaps(dst, src map[string]any) map[string]any {
	for key, srcValue := range src {
		switch key {
		case "required":
			if isEmptySchemaValue(dst[key]) {
				dst[key] = cloneSchemaValue(srcValue)
			}
		case "properties":
			dst[key] = mergeDetailPropertyMaps(dst[key], srcValue)
		default:
			dstMap, dstIsMap := dst[key].(map[string]any)
			srcMap, srcIsMap := srcValue.(map[string]any)
			if dstIsMap && srcIsMap {
				dst[key] = mergeDetailSchemaMaps(dstMap, srcMap)
				continue
			}
			if isEmptySchemaValue(dst[key]) {
				dst[key] = cloneSchemaValue(srcValue)
			}
		}
	}
	return dst
}

func mergeDetailPropertyMaps(dstValue, srcValue any) map[string]any {
	dstProps, _ := dstValue.(map[string]any)
	if dstProps == nil {
		dstProps = map[string]any{}
	}
	srcProps, _ := srcValue.(map[string]any)
	for name, raw := range srcProps {
		srcProp, ok := raw.(map[string]any)
		if !ok {
			if _, exists := dstProps[name]; !exists {
				dstProps[name] = cloneSchemaValue(raw)
			}
			continue
		}
		existing, _ := dstProps[name].(map[string]any)
		if len(existing) == 0 {
			dstProps[name] = cloneSchemaValue(srcProp)
			continue
		}
		dstProps[name] = mergeDetailSchemaMaps(existing, srcProp)
	}
	return dstProps
}

func isEmptySchemaValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func cloneSchemaMap(value map[string]any) map[string]any {
	cloned, _ := cloneSchemaValue(value).(map[string]any)
	return cloned
}

func cloneSchemaValue(value any) any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return value
	}
	return cloned
}

func toolTitleCanUseDetail(tool transport.ToolDescriptor) bool {
	title := strings.TrimSpace(tool.Title)
	if title == "" {
		return true
	}
	name := strings.TrimSpace(tool.Name)
	description := strings.TrimSpace(tool.Description)
	return title == name || (description != "" && description == title)
}

func toolDescriptionCanUseDetail(tool transport.ToolDescriptor) bool {
	description := strings.TrimSpace(tool.Description)
	if description == "" {
		return true
	}
	title := strings.TrimSpace(tool.Title)
	return title != "" && description == title
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
