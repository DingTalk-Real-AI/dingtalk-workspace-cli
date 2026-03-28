package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
)

func TestCacheRefreshProductOnlyClearsSelectedServerCaches(t *testing.T) {
	setupRuntimeCommandTest(t)
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	srv := newCacheRefreshServer(t)
	defer srv.Close()

	selectedServerKey := market.ServerKey(srv.URL + "/doc")
	otherServerKey := market.ServerKey(srv.URL + "/calendar")

	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)
	store := cache.NewStore(cacheDir)

	const partition = "default/default"
	preloadRuntimeCache(t, store, partition, selectedServerKey, "doc")
	preloadRuntimeCache(t, store, partition, otherServerKey, "calendar")

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"cache", "refresh", "--product", "doc"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(cache refresh) error = %v\noutput:\n%s", err, out.String())
	}

	if !strings.Contains(out.String(), "已刷新 1 个服务") {
		t.Fatalf("output = %q, want selected refresh count", out.String())
	}

	if _, _, err := store.LoadTools(partition, selectedServerKey); err != nil {
		t.Fatalf("LoadTools(%s) error = %v", selectedServerKey, err)
	}
	if _, _, err := store.LoadTools(partition, "doc"); !cache.IsNotExist(err) {
		t.Fatalf("LoadTools(doc) error = %v, want not-exist after invalidation", err)
	}
	if _, _, err := store.LoadDetail(partition, selectedServerKey); !cache.IsNotExist(err) {
		t.Fatalf("LoadDetail(%s) error = %v, want not-exist after refresh", selectedServerKey, err)
	}
	if _, _, err := store.LoadDetail(partition, "doc"); !cache.IsNotExist(err) {
		t.Fatalf("LoadDetail(doc) error = %v, want not-exist after refresh", err)
	}

	for _, cacheKey := range []string{otherServerKey, "calendar"} {
		if _, _, err := store.LoadTools(partition, cacheKey); err != nil {
			t.Fatalf("LoadTools(%s) error = %v, want untouched cache", cacheKey, err)
		}
		if _, _, err := store.LoadDetail(partition, cacheKey); err != nil {
			t.Fatalf("LoadDetail(%s) error = %v, want untouched cache", cacheKey, err)
		}
	}
}

func TestBackgroundCacheRefreshPreservesExistingCachesOnRefreshFailure(t *testing.T) {
	setupRuntimeCommandTest(t)
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_BACKGROUND_REFRESH", "1")

	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)
	store := cache.NewStore(cacheDir)

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	selectedServerKey := market.ServerKey(server.URL + "/doc")
	const partition = "default/default"
	preloadRuntimeCache(t, store, partition, selectedServerKey, "doc")

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"metadata": map[string]any{"count": 1, "nextCursor": ""},
			"servers": []map[string]any{
				cacheRefreshServerEnvelope("http://"+r.Host, "Doc Service", "doc", "/doc"),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	mux.HandleFunc("/doc", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "runtime unavailable", http.StatusBadGateway)
	})

	SetDiscoveryBaseURL(server.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"cache", "refresh", "--product", "doc"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(cache refresh background) error = %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "已刷新 0 个服务，失败 1 个") {
		t.Fatalf("output = %q, want degraded refresh failure summary", out.String())
	}

	if _, _, err := store.LoadTools(partition, selectedServerKey); err != nil {
		t.Fatalf("LoadTools(%s) error = %v, want preserved cache on failed background refresh", selectedServerKey, err)
	}
	if _, _, err := store.LoadTools(partition, "doc"); err != nil {
		t.Fatalf("LoadTools(doc) error = %v, want preserved alias cache on failed background refresh", err)
	}
	if _, _, err := store.LoadDetail(partition, selectedServerKey); err != nil {
		t.Fatalf("LoadDetail(%s) error = %v, want preserved detail cache on failed background refresh", selectedServerKey, err)
	}
	if _, _, err := store.LoadDetail(partition, "doc"); err != nil {
		t.Fatalf("LoadDetail(doc) error = %v, want preserved alias detail cache on failed background refresh", err)
	}
}

func preloadRuntimeCache(t *testing.T, store *cache.Store, partition, serverKey, cliID string) {
	t.Helper()

	for _, key := range []string{serverKey, cliID} {
		if err := store.SaveTools(partition, key, cache.ToolsSnapshot{
			ServerKey:       key,
			ProtocolVersion: "2025-03-26",
			Tools: []transport.ToolDescriptor{
				{
					Name:        "search_documents",
					Description: "cached tool",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		}); err != nil {
			t.Fatalf("SaveTools(%s) error = %v", key, err)
		}
		if err := store.SaveDetail(partition, key, cache.DetailSnapshot{
			MCPID:   101,
			Payload: json.RawMessage(`{"success":true,"result":{"mcpId":101,"tools":[]}}`),
		}); err != nil {
			t.Fatalf("SaveDetail(%s) error = %v", key, err)
		}
	}
}

func newCacheRefreshServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		baseURL := "http://" + r.Host
		response := map[string]any{
			"metadata": map[string]any{"count": 2, "nextCursor": ""},
			"servers": []map[string]any{
				cacheRefreshServerEnvelope(baseURL, "Doc Service", "doc", "/doc"),
				cacheRefreshServerEnvelope(baseURL, "Calendar Service", "calendar", "/calendar"),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	mux.HandleFunc("/doc", cacheRefreshMCPHandler("doc"))
	mux.HandleFunc("/calendar", cacheRefreshMCPHandler("calendar"))

	return httptest.NewServer(mux)
}

func cacheRefreshServerEnvelope(baseURL, name, cliID, remotePath string) map[string]any {
	return map[string]any{
		"server": map[string]any{
			"name":        name,
			"description": name,
			"remotes": []map[string]any{
				{
					"type": "streamable-http",
					"url":  baseURL + remotePath,
				},
			},
		},
		"_meta": map[string]any{
			"com.dingtalk.mcp.registry/metadata": map[string]any{
				"mcpId":     0,
				"status":    "active",
				"detailUrl": "",
			},
			"com.dingtalk.mcp.registry/cli": map[string]any{
				"id":          cliID,
				"command":     cliID,
				"description": name,
			},
		},
	}
}

func cacheRefreshMCPHandler(prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json-rpc request", http.StatusBadRequest)
			return
		}
		method, _ := req["method"].(string)
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
					"serverInfo":      map[string]any{"name": prefix, "version": "1.0.0"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        prefix + "_search",
							"description": "test tool",
							"inputSchema": map[string]any{"type": "object"},
						},
					},
				},
			})
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	}
}
