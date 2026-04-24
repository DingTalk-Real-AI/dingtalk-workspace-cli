package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCacheRefreshClearsExistingCachesAndSkipsCLISkippedServers(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)

	var skippedRuntimeCalls atomic.Int32

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cli/discovery/apis":
			_ = json.NewEncoder(w).Encode(market.ListResponse{
				Metadata: market.ListMetadata{Count: 2},
				Servers: []market.ServerEnvelope{
					{
						Server: market.RegistryServer{
							Name: "Active Service",
							Remotes: []market.RegistryRemote{
								{Type: "streamable-http", URL: srv.URL + "/mcp/active"},
							},
						},
						Meta: market.EnvelopeMeta{
							Registry: market.RegistryMetadata{Status: "active"},
							CLI:      market.CLIOverlay{ID: "active", Command: "active"},
						},
					},
					{
						Server: market.RegistryServer{
							Name: "Skipped Service",
							Remotes: []market.RegistryRemote{
								{Type: "streamable-http", URL: srv.URL + "/mcp/skipped"},
							},
						},
						Meta: market.EnvelopeMeta{
							Registry: market.RegistryMetadata{Status: "active"},
							CLI:      market.CLIOverlay{ID: "legacy", Command: "legacy", Skip: true},
						},
					},
				},
			})
		case "/mcp/active":
			http.Error(w, "active runtime unavailable", http.StatusInternalServerError)
		case "/mcp/skipped":
			skippedRuntimeCalls.Add(1)
			http.Error(w, "skipped runtime should not be called", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	store := cache.NewStore(cacheDir)
	const partition = "default/default"
	activeKey := market.ServerKey(srv.URL + "/mcp/active")
	skippedKey := market.ServerKey(srv.URL + "/mcp/skipped")

	saveCachedRuntimeAndDetail(t, store, partition, activeKey)
	saveCachedRuntimeAndDetail(t, store, partition, skippedKey)
	saveCLIIDDetail(t, store, partition, "active")

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	cmd := newCacheCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"refresh"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if _, _, err := store.LoadTools(partition, activeKey); err == nil {
		t.Fatal("LoadTools(active) error = nil, want cache cleared before failed refresh")
	}
	if _, _, err := store.LoadDetail(partition, activeKey); err == nil {
		t.Fatal("LoadDetail(active) error = nil, want detail cache cleared before failed refresh")
	}
	if _, _, err := store.LoadDetail(partition, "active"); err != nil {
		t.Fatalf("LoadDetail(active CLI.ID) error = %v, want CLI metadata preserved on failed refresh", err)
	}
	if _, _, err := store.LoadTools(partition, skippedKey); err == nil {
		t.Fatal("LoadTools(skipped) error = nil, want skipped service cache removed")
	}
	if _, _, err := store.LoadDetail(partition, skippedKey); err == nil {
		t.Fatal("LoadDetail(skipped) error = nil, want skipped service detail cache removed")
	}
	if got := skippedRuntimeCalls.Load(); got != 0 {
		t.Fatalf("skipped runtime calls = %d, want 0", got)
	}
}

// TestCacheRefreshHonorsEditionDiscoveryURL asserts the `dws cache refresh`
// command routes its server-list fetch through edition.Hooks.DiscoveryURL /
// DiscoveryHeaders when they are set, instead of the default Market endpoint.
// Kept deliberately generic (no edition-specific strings) — concrete values
// belong to the overlay repo that installs the hooks, not to this open core.
func TestCacheRefreshHonorsEditionDiscoveryURL(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)

	var (
		editionHits atomic.Int32
		marketHits  atomic.Int32
		gotHeaders  atomic.Value // map[string]string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cli/edition/apis":
			editionHits.Add(1)
			snapshot := map[string]string{
				"x-test-edition": r.Header.Get("x-test-edition"),
				"x-test-client":  r.Header.Get("x-test-client"),
			}
			gotHeaders.Store(snapshot)
			_ = json.NewEncoder(w).Encode(market.ListResponse{
				Metadata: market.ListMetadata{Count: 1},
				Servers: []market.ServerEnvelope{
					{
						Server: market.RegistryServer{
							Name:    "Edition Service",
							Remotes: []market.RegistryRemote{{Type: "streamable-http", URL: "https://example.invalid/mcp"}},
						},
						Meta: market.EnvelopeMeta{
							Registry: market.RegistryMetadata{Status: "active"},
							CLI:      market.CLIOverlay{ID: "edition-service", Command: "edition-service"},
						},
					},
				},
			})
		case "/cli/discovery/apis":
			marketHits.Add(1)
			http.Error(w, "market endpoint must not be called when edition DiscoveryURL is set", http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	edition.Override(&edition.Hooks{
		Name:         "testing",
		DiscoveryURL: srv.URL + "/cli/edition/apis",
		DiscoveryHeaders: func() map[string]string {
			return map[string]string{
				"x-test-edition": "custom",
				"x-test-client":  "cli-refresh",
			}
		},
	})
	t.Cleanup(func() { edition.Override(&edition.Hooks{}) })

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	cmd := newCacheCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"refresh"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := editionHits.Load(); got != 1 {
		t.Fatalf("edition DiscoveryURL hits = %d, want 1", got)
	}
	if got := marketHits.Load(); got != 0 {
		t.Fatalf("market endpoint hits = %d, want 0 (edition DiscoveryURL must take precedence)", got)
	}

	headers, _ := gotHeaders.Load().(map[string]string)
	if headers == nil {
		t.Fatal("captured request headers = nil, want edition DiscoveryHeaders to be applied")
	}
	if headers["x-test-edition"] != "custom" {
		t.Fatalf("x-test-edition header = %q, want %q", headers["x-test-edition"], "custom")
	}
	if headers["x-test-client"] != "cli-refresh" {
		t.Fatalf("x-test-client header = %q, want %q", headers["x-test-client"], "cli-refresh")
	}
}

func saveCLIIDDetail(t *testing.T, store *cache.Store, partition, cliID string) {
	t.Helper()

	payload, err := json.Marshal(market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			Tools: []market.DetailTool{
				{ToolName: "stale_tool", ToolTitle: "Stale Tool"},
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal(cli detail payload) error = %v", err)
	}
	if err := store.SaveDetail(partition, cliID, cache.DetailSnapshot{
		MCPID:   0,
		Payload: payload,
	}); err != nil {
		t.Fatalf("SaveDetail(%s) error = %v", cliID, err)
	}
}

func saveCachedRuntimeAndDetail(t *testing.T, store *cache.Store, partition, serverKey string) {
	t.Helper()

	if err := store.SaveTools(partition, serverKey, cache.ToolsSnapshot{
		ServerKey:       serverKey,
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{
			{Name: "stale_tool", Title: "Stale Tool"},
		},
	}); err != nil {
		t.Fatalf("SaveTools(%s) error = %v", serverKey, err)
	}

	payload, err := json.Marshal(market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			Tools: []market.DetailTool{
				{ToolName: "stale_tool", ToolTitle: "Stale Tool"},
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal(detail payload) error = %v", err)
	}
	if err := store.SaveDetail(partition, serverKey, cache.DetailSnapshot{
		MCPID:   0,
		Payload: payload,
	}); err != nil {
		t.Fatalf("SaveDetail(%s) error = %v", serverKey, err)
	}
}
