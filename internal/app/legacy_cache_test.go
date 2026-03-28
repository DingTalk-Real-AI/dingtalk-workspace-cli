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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/spf13/cobra"
)

// marketListResponse builds a minimal valid FetchServers JSON response.
// The server has a ToolOverride so BuildDynamicCommands emits a command.
func marketListResponse(cliID string) map[string]any {
	return map[string]any{
		"metadata": map[string]any{"count": 1, "nextCursor": ""},
		"servers": []any{
			map[string]any{
				"server": map[string]any{
					"name":        "Test Server",
					"description": "desc",
					"remotes": []any{
						map[string]any{
							"type": "streamable-http",
							"url":  "https://mcp.dingtalk.com/test/v1",
						},
					},
				},
				"_meta": map[string]any{
					"com.dingtalk.mcp.registry/metadata": map[string]any{
						"status": "active", "isLatest": true,
					},
					"com.dingtalk.mcp.registry/cli": map[string]any{
						"id":      cliID,
						"command": cliID,
						"toolOverrides": map[string]any{
							"test_tool": map[string]any{
								"cliName": "test",
								"flags":   map[string]any{},
							},
						},
					},
				},
			},
		},
	}
}

type testCLIServerSpec struct {
	id      string
	command string
	tool    string
	cliName string
}

func marketListResponseForSpecs(specs ...testCLIServerSpec) map[string]any {
	servers := make([]any, 0, len(specs))
	for _, spec := range specs {
		servers = append(servers, map[string]any{
			"server": map[string]any{
				"name":        spec.command,
				"description": spec.command + " desc",
				"remotes": []any{
					map[string]any{
						"type": "streamable-http",
						"url":  "https://mcp.dingtalk.com/" + spec.command + "/v1",
					},
				},
			},
			"_meta": map[string]any{
				"com.dingtalk.mcp.registry/metadata": map[string]any{
					"status": "active", "isLatest": true,
				},
				"com.dingtalk.mcp.registry/cli": map[string]any{
					"id":      spec.id,
					"command": spec.command,
					"toolOverrides": map[string]any{
						spec.tool: map[string]any{
							"cliName": spec.cliName,
							"flags":   map[string]any{},
						},
					},
				},
			},
		})
	}
	return map[string]any{
		"metadata": map[string]any{"count": len(servers), "nextCursor": ""},
		"servers":  servers,
	}
}

// minimalCLIServer returns a ServerDescriptor with ToolOverrides so
// BuildDynamicCommands will emit at least one cobra command.
func minimalCLIServer(id, endpoint string) market.ServerDescriptor {
	return market.ServerDescriptor{
		Key:         id + "-key",
		DisplayName: id,
		Endpoint:    endpoint,
		Source:      "market",
		CLI: market.CLIOverlay{
			ID:      id,
			Command: id,
			ToolOverrides: map[string]market.CLIToolOverride{
				"test_tool": {CLIName: "test"},
			},
		},
		HasCLIMeta: true,
	}
}

// TestLoadDynamicCommandsUsesFreshCacheWithoutNetwork verifies that when a
// fresh registry cache exists, no network request is made.
//
// This test uses an isolated DWS_CACHE_DIR + discoveryBaseURLOverride so that:
// - useCache=true (DWS_CATALOG_FIXTURE is "")
// - The test server records any incoming request; it should NOT be hit when cache is fresh.
func TestLoadDynamicCommandsUsesFreshCacheWithoutNetwork(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")

	requestCount := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		_ = json.NewEncoder(w).Encode(marketListResponse("test-fresh"))
	}))
	defer srv.Close()

	// Isolated cache dir with a FRESH snapshot.
	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)
	store := cache.NewStore(cacheDir)
	err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		SavedAt: time.Now().UTC(), // fresh
		Servers: []market.ServerDescriptor{minimalCLIServer("cached", "https://mcp.dingtalk.com/cached/v1")},
	})
	if err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	// Point discovery to the test server. Since cache is fresh and
	// useCache=true (CATALOG_FIXTURE is ""), the network should not be needed.
	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	cmds := loadDynamicCommands(context.Background(), nil)

	if got := requestCount.Load(); got != 0 {
		t.Errorf("network request count = %d, want 0 (fresh cache should be used)", got)
	}
	if len(cmds) == 0 {
		t.Errorf("loadDynamicCommands() returned 0 commands, want >0 from fresh cache")
	}
}

// TestLoadDynamicCommandsUsesStaleCacheOnStartup verifies that when the
// registry cache is stale, startup still returns commands from the cache
// instead of blocking on a synchronous market refresh.
func TestLoadDynamicCommandsUsesStaleCacheOnStartup(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")

	requestCount := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		_ = json.NewEncoder(w).Encode(marketListResponse("network-server"))
	}))
	defer srv.Close()

	// Isolated cache dir with a STALE snapshot.
	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)
	store := cache.NewStore(cacheDir)
	err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		SavedAt: time.Now().UTC().Add(-25 * time.Hour), // older than RegistryTTL=24h
		Servers: []market.ServerDescriptor{minimalCLIServer("stale", "https://mcp.dingtalk.com/stale/v1")},
	})
	if err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	cmds := loadDynamicCommands(context.Background(), nil)

	if len(cmds) == 0 {
		t.Fatalf("loadDynamicCommands() = 0 commands, want >0 from stale cache")
	}
	if got := requestCount.Load(); got != 0 {
		t.Errorf("startup network request count = %d, want 0 (stale cache should not block startup)", got)
	}
}

// TestLoadDynamicCommandsCacheUpdatedAfterFetch verifies the cache is persisted
// after a successful network fetch (useCache=true, isolated cache dir).
func TestLoadDynamicCommandsCacheUpdatedAfterFetch(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(marketListResponse("fresh-server"))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)
	store := cache.NewStore(cacheDir)

	SetDiscoveryBaseURL(srv.URL) // stale/empty cache → network
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	_ = loadDynamicCommands(context.Background(), nil)

	snapshot, freshness, err := store.LoadRegistry("default/default")
	if err != nil {
		t.Fatalf("LoadRegistry() after fetch error = %v", err)
	}
	if freshness != cache.FreshnessFresh {
		t.Errorf("cache freshness = %s, want fresh", freshness)
	}
	if len(snapshot.Servers) == 0 {
		t.Errorf("cache servers = 0, want >0 after network fetch")
	}
}

// TestLoadDynamicCommandsFallsBackToStaleCacheOnNetworkError verifies that
// when the market API is unavailable but a stale cache exists, the CLI
// still generates commands from the stale data (offline degradation).
func TestLoadDynamicCommandsFallsBackToStaleCacheOnNetworkError(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)
	store := cache.NewStore(cacheDir)
	err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		SavedAt: time.Now().UTC().Add(-25 * time.Hour), // stale
		Servers: []market.ServerDescriptor{minimalCLIServer("degraded", "https://mcp.dingtalk.com/degraded/v1")},
	})
	if err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	cmds := loadDynamicCommands(context.Background(), nil)

	if len(cmds) == 0 {
		t.Errorf("loadDynamicCommands() = 0 commands, want >0 (stale fallback on network error)")
	}
}

func TestLoadDynamicCommandsRefreshesRegistryCacheInBackgroundAfterAgedStart(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")

	var phase atomic.Int32
	phase.Store(1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := marketListResponseForSpecs(testCLIServerSpec{
			id:      "doc",
			command: "doc",
			tool:    "create_document",
			cliName: "create-document",
		})
		if phase.Load() == 2 {
			payload = marketListResponseForSpecs(
				testCLIServerSpec{
					id:      "doc",
					command: "doc",
					tool:    "archive_document",
					cliName: "archive-document",
				},
				testCLIServerSpec{
					id:      "drive",
					command: "drive",
					tool:    "list_files",
					cliName: "list-files",
				},
			)
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)
	store := cache.NewStore(cacheDir)

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	cmds := loadDynamicCommands(context.Background(), nil)
	assertDynamicCommandChildren(t, cmds, "doc", []string{"create-document"})

	snapshot, _, err := store.LoadRegistry("default/default")
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	snapshot.SavedAt = time.Now().UTC().Add(-2 * time.Hour)
	if err := store.SaveRegistry("default/default", snapshot); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	phase.Store(2)
	cmds = loadDynamicCommands(context.Background(), nil)
	assertDynamicCommandChildren(t, cmds, "doc", []string{"create-document"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		refreshed, _, err := store.LoadRegistry("default/default")
		if err == nil && len(refreshed.Servers) == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	cmds = loadDynamicCommands(context.Background(), nil)
	assertDynamicCommandChildren(t, cmds, "doc", []string{"archive-document"})
	assertDynamicCommandChildren(t, cmds, "drive", []string{"list-files"})
}

func TestLoadDynamicCommandsDoesNotSynchronouslyFetchDetailMetadata(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")

	var phase atomic.Int32
	docDetailCalls := new(atomic.Int32)
	driveDetailCalls := new(atomic.Int32)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cli/discovery/apis":
			payload := map[string]any{
				"metadata": map[string]any{"count": 2, "nextCursor": ""},
				"servers": []any{
					registryServerEnvelope("doc", "doc", "2026-03-21T02:00:00Z", 1001, "create_document", "create-document"),
					registryServerEnvelope("drive", "drive", "2026-03-21T02:00:00Z", 1002, "list_files", "list-files"),
				},
			}
			if phase.Load() == 1 {
				payload["servers"] = []any{
					registryServerEnvelope("doc", "doc", "2026-03-25T10:00:00Z", 1001, "archive_document", "archive-document"),
					registryServerEnvelope("drive", "drive", "2026-03-21T02:00:00Z", 1002, "list_files", "list-files"),
				}
			}
			_ = json.NewEncoder(w).Encode(payload)
		case "/mcp/market/detail":
			switch r.URL.Query().Get("mcpId") {
			case "1001":
				docDetailCalls.Add(1)
				_ = json.NewEncoder(w).Encode(detailResponse(1001, "archive_document", "Archive Document", "archive desc"))
			case "1002":
				driveDetailCalls.Add(1)
				_ = json.NewEncoder(w).Encode(detailResponse(1002, "list_files", "List Files", "list desc"))
			default:
				http.Error(w, "unknown mcpId", http.StatusNotFound)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	cmds := loadDynamicCommands(context.Background(), nil)
	assertDynamicCommandChildren(t, cmds, "doc", []string{"create-document"})
	assertDynamicCommandChildren(t, cmds, "drive", []string{"list-files"})

	if got := docDetailCalls.Load(); got != 0 {
		t.Fatalf("doc detail calls after startup = %d, want 0", got)
	}
	if got := driveDetailCalls.Load(); got != 0 {
		t.Fatalf("drive detail calls after startup = %d, want 0", got)
	}

	phase.Store(1)
	docDetailCalls.Store(0)
	driveDetailCalls.Store(0)
	ageCacheSnapshotsOnDisk(t, cacheDir, time.Now().UTC().Add(-2*time.Hour))

	cmds = loadDynamicCommands(context.Background(), nil)
	assertDynamicCommandChildren(t, cmds, "doc", []string{"create-document"})
	assertDynamicCommandChildren(t, cmds, "drive", []string{"list-files"})

	if got := docDetailCalls.Load(); got != 0 {
		t.Fatalf("doc detail calls after aged startup = %d, want 0", got)
	}
	if got := driveDetailCalls.Load(); got != 0 {
		t.Fatalf("drive detail calls after aged startup = %d, want 0", got)
	}
}

func TestLoadDynamicCommandsDoesNotSynchronouslyFetchDetailMetadataWhenRegistryTTLExpires(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")

	docDetailCalls := new(atomic.Int32)
	driveDetailCalls := new(atomic.Int32)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cli/discovery/apis":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metadata": map[string]any{"count": 2, "nextCursor": ""},
				"servers": []any{
					registryServerEnvelope("doc", "doc", "2026-03-21T02:00:00Z", 1001, "create_document", "create-document"),
					registryServerEnvelope("drive", "drive", "2026-03-21T02:00:00Z", 1002, "list_files", "list-files"),
				},
			})
		case "/mcp/market/detail":
			switch r.URL.Query().Get("mcpId") {
			case "1001":
				docDetailCalls.Add(1)
				_ = json.NewEncoder(w).Encode(detailResponse(1001, "create_document", "Create Document", "create desc"))
			case "1002":
				driveDetailCalls.Add(1)
				_ = json.NewEncoder(w).Encode(detailResponse(1002, "list_files", "List Files", "list desc"))
			default:
				http.Error(w, "unknown mcpId", http.StatusNotFound)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	cmds := loadDynamicCommands(context.Background(), nil)
	assertDynamicCommandChildren(t, cmds, "doc", []string{"create-document"})
	assertDynamicCommandChildren(t, cmds, "drive", []string{"list-files"})

	if got := docDetailCalls.Load(); got != 0 {
		t.Fatalf("doc detail calls after startup = %d, want 0", got)
	}
	if got := driveDetailCalls.Load(); got != 0 {
		t.Fatalf("drive detail calls after startup = %d, want 0", got)
	}

	docDetailCalls.Store(0)
	driveDetailCalls.Store(0)
	ageCacheSnapshotsOnDisk(t, cacheDir, time.Now().UTC().Add(-25*time.Hour))

	cmds = loadDynamicCommands(context.Background(), nil)
	assertDynamicCommandChildren(t, cmds, "doc", []string{"create-document"})
	assertDynamicCommandChildren(t, cmds, "drive", []string{"list-files"})

	if got := docDetailCalls.Load(); got != 0 {
		t.Fatalf("doc detail calls after registry TTL expiry = %d, want 0", got)
	}
	if got := driveDetailCalls.Load(); got != 0 {
		t.Fatalf("drive detail calls after registry TTL expiry = %d, want 0", got)
	}
}

func TestLoadDynamicCommandsUsesStaleCacheWithoutBlockingRegistryRefresh(t *testing.T) {
	t.Setenv(cli.CatalogFixtureEnv, "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(marketListResponseForSpecs(testCLIServerSpec{
			id:      "doc",
			command: "doc",
			tool:    "archive_document",
			cliName: "archive-document",
		}))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)
	store := cache.NewStore(cacheDir)
	if err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		SavedAt: time.Now().UTC().Add(-25 * time.Hour),
		Servers: []market.ServerDescriptor{
			{
				Key:         "doc-key",
				DisplayName: "doc",
				Endpoint:    "https://mcp.dingtalk.com/doc/v1",
				Source:      "market",
				CLI: market.CLIOverlay{
					ID:      "doc",
					Command: "doc",
					ToolOverrides: map[string]market.CLIToolOverride{
						"create_document": {CLIName: "create-document"},
					},
				},
				HasCLIMeta: true,
			},
		},
	}); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	start := time.Now()
	cmds := loadDynamicCommands(context.Background(), nil)
	if elapsed := time.Since(start); elapsed >= 200*time.Millisecond {
		t.Fatalf("loadDynamicCommands() took %v, want stale cache startup under 200ms", elapsed)
	}

	assertDynamicCommandChildren(t, cmds, "doc", []string{"create-document"})
}

// TestFetchDetailsByServerIDRunsConcurrently verifies that detail fetches are
// concurrent, not serial. Uses MCPID path to avoid the localhost SSRF guard.
func TestFetchDetailsByServerIDRunsConcurrently(t *testing.T) {
	const numServers = 4
	const delay = 50 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"mcpId": 1, "name": "test", "description": "test",
				"tools": []any{
					map[string]any{"toolName": "test_tool", "toolTitle": "Test Tool", "toolDesc": "desc"},
				},
			},
		})
	}))
	defer srv.Close()

	servers := make([]market.ServerDescriptor, numServers)
	for i := range servers {
		servers[i] = market.ServerDescriptor{
			DetailLocator: market.DetailLocator{MCPID: i + 1},
			CLI:           market.CLIOverlay{ID: "test-server-" + string(rune('a'+i))},
			HasCLIMeta:    true,
		}
	}

	start := time.Now()
	result := fetchDetailsByServerID(context.TODO(), market.NewClient(srv.URL, nil), servers, cache.NewStore(t.TempDir()), false)
	elapsed := time.Since(start)

	serialBound := time.Duration(numServers) * delay
	if elapsed >= serialBound {
		t.Errorf("elapsed %v >= serial bound %v: requests appear serial, want concurrent", elapsed, serialBound)
	}
	if len(result) == 0 {
		t.Errorf("fetchDetailsByServerID() = empty map, want results")
	}
}

func assertDynamicCommandChildren(t *testing.T, cmds []*cobra.Command, name string, want []string) {
	t.Helper()

	for _, cmd := range cmds {
		if cmd.Name() != name {
			continue
		}
		got := make([]string, 0)
		for _, child := range cmd.Commands() {
			if child.Name() == "help" {
				continue
			}
			got = append(got, child.Name())
		}
		sort.Strings(got)

		sortedWant := append([]string(nil), want...)
		sort.Strings(sortedWant)
		if len(got) != len(sortedWant) {
			t.Fatalf("command %q children = %#v, want %#v", name, got, sortedWant)
		}
		for idx := range got {
			if got[idx] != sortedWant[idx] {
				t.Fatalf("command %q children = %#v, want %#v", name, got, sortedWant)
			}
		}
		return
	}

	t.Fatalf("command %q not found", name)
}

func registryServerEnvelope(id, command, updatedAt string, mcpID int, toolName, cliName string) map[string]any {
	return map[string]any{
		"server": map[string]any{
			"name":        command,
			"description": command + " desc",
			"remotes": []any{
				map[string]any{
					"type": "streamable-http",
					"url":  "https://mcp.dingtalk.com/" + command + "/v1",
				},
			},
		},
		"_meta": map[string]any{
			"com.dingtalk.mcp.registry/metadata": map[string]any{
				"status":      "active",
				"isLatest":    true,
				"updatedAt":   updatedAt,
				"publishedAt": updatedAt,
				"mcpId":       mcpID,
			},
			"com.dingtalk.mcp.registry/cli": map[string]any{
				"id":      id,
				"command": command,
				"toolOverrides": map[string]any{
					toolName: map[string]any{
						"cliName": cliName,
						"flags":   map[string]any{},
					},
				},
			},
		},
	}
}

func detailResponse(mcpID int, toolName, title, desc string) map[string]any {
	return map[string]any{
		"success": true,
		"result": map[string]any{
			"mcpId":       mcpID,
			"name":        title,
			"description": desc,
			"tools": []any{
				map[string]any{
					"toolName":      toolName,
					"toolTitle":     title,
					"toolDesc":      desc,
					"toolRequest":   `{"type":"object"}`,
					"toolResponse":  `{"type":"object"}`,
					"actionVersion": "v1",
				},
			},
		},
	}
}

func ageCacheSnapshotsOnDisk(t *testing.T, root string, savedAt time.Time) {
	t.Helper()

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil
		}
		if _, ok := payload["saved_at"]; !ok {
			return nil
		}
		payload["saved_at"] = savedAt.Format(time.RFC3339Nano)

		rewritten, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(path, rewritten, 0o644)
	})
	if walkErr != nil {
		t.Fatalf("ageCacheSnapshotsOnDisk() error = %v", walkErr)
	}
}

// TestFetchDetailsByServerIDUsesCacheOnHit verifies that a fresh detail cache
// entry prevents any network request.
func TestFetchDetailsByServerIDUsesCacheOnHit(t *testing.T) {
	requestCount := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{"tools": []any{}}})
	}))
	defer srv.Close()

	store := cache.NewStore(t.TempDir())
	cachedTools := []market.DetailTool{{ToolName: "cached_tool", ToolTitle: "Cached", ToolDesc: "from cache"}}
	cachedJSON, _ := json.Marshal(map[string]any{"tools": cachedTools})
	err := store.SaveDetail("default/default", "test-server", cache.DetailSnapshot{
		SavedAt: time.Now().UTC(),
		MCPID:   42,
		Payload: cachedJSON,
	})
	if err != nil {
		t.Fatalf("SaveDetail() error = %v", err)
	}

	servers := []market.ServerDescriptor{
		{DetailLocator: market.DetailLocator{MCPID: 42}, CLI: market.CLIOverlay{ID: "test-server"}, HasCLIMeta: true},
	}
	result := fetchDetailsByServerID(context.TODO(), market.NewClient(srv.URL, nil), servers, store, false)

	if got := requestCount.Load(); got != 0 {
		t.Errorf("network request count = %d, want 0 (fresh detail cache should be used)", got)
	}
	if len(result) == 0 {
		t.Errorf("fetchDetailsByServerID() returned empty map, want cached tools")
	}
}
