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

package skillgen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
)

func TestLoadCatalogWithSourceRejectsUnknown(t *testing.T) {
	t.Parallel()

	_, err := LoadCatalogWithSource(context.Background(), "unknown-source", "")
	if err == nil {
		t.Fatal("LoadCatalogWithSource() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported catalog source") {
		t.Fatalf("LoadCatalogWithSource() error = %v, want unsupported source", err)
	}
}

func TestNormalizeCatalogSourceDefaultsToEnv(t *testing.T) {
	t.Parallel()

	if got := normalizeCatalogSource(""); got != string(CatalogSourceEnv) {
		t.Fatalf("normalizeCatalogSource(\"\") = %q, want %q", got, CatalogSourceEnv)
	}
	if got := normalizeCatalogSource("   "); got != string(CatalogSourceEnv) {
		t.Fatalf("normalizeCatalogSource(\"   \") = %q, want %q", got, CatalogSourceEnv)
	}
}

func TestResolveSnapshotPathDefaultsToSkillsGeneratedDocs(t *testing.T) {
	t.Parallel()

	resolved, err := resolveSnapshotPath("")
	if err != nil {
		t.Fatalf("resolveSnapshotPath() error = %v", err)
	}
	wantSuffix := filepath.Join("skills", "generated", "docs", "schema", "catalog.json")
	if !strings.HasSuffix(resolved, wantSuffix) {
		t.Fatalf("resolveSnapshotPath() = %q, want suffix %q", resolved, wantSuffix)
	}
}

func TestLoadCatalogWithSourceEnvRequiresLiveCatalog(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	store := cache.NewStore(cacheDir)
	if err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		SavedAt: time.Now().UTC(),
		Servers: []market.ServerDescriptor{
			{
				Key:         "cached-key",
				DisplayName: "cached",
				Endpoint:    "https://mcp.dingtalk.com/cached/v1",
				Source:      "fresh_cache",
				CLI: market.CLIOverlay{
					ID:      "cached",
					Command: "cached",
					ToolOverrides: map[string]market.CLIToolOverride{
						"test_tool": {CLIName: "test"},
					},
				},
				HasCLIMeta: true,
			},
		},
	}); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}
	if err := store.SaveTools("default/default", "cached-key", cache.ToolsSnapshot{
		SavedAt:         time.Now().UTC(),
		ServerKey:       "cached-key",
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{
			{
				Name:        "test_tool",
				Title:       "Cached Tool",
				Description: "Only present in cache",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		payload := market.ListResponse{
			Metadata: market.ListMetadata{Count: 2},
			Servers: []market.ServerEnvelope{
				{
					Server: market.RegistryServer{
						Name: "文档",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/doc"},
						},
					},
					Meta: market.EnvelopeMeta{
						Registry: market.RegistryMetadata{Status: "active"},
						CLI: market.CLIOverlay{
							ID:      "doc",
							Command: "doc",
							ToolOverrides: map[string]market.CLIToolOverride{
								"search_documents": {CLIName: "search-documents"},
							},
						},
					},
				},
				{
					Server: market.RegistryServer{
						Name: "网盘",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/drive"},
						},
					},
					Meta: market.EnvelopeMeta{
						Registry: market.RegistryMetadata{Status: "active"},
						CLI: market.CLIOverlay{
							ID:      "drive",
							Command: "drive",
							ToolOverrides: map[string]market.CLIToolOverride{
								"search_files": {CLIName: "search-files"},
							},
						},
					},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Fatalf("Encode(market payload) error = %v", err)
		}
	})

	mux.HandleFunc("/server/doc", func(w http.ResponseWriter, r *http.Request) {
		serveGeneratorRuntimeToolList(t, w, r, "doc", transport.ToolDescriptor{
			Name:        "search_documents",
			Title:       "搜索文档",
			Description: "根据关键词搜索文档",
			InputSchema: map[string]any{"type": "object"},
		})
	})
	mux.HandleFunc("/server/drive", func(w http.ResponseWriter, r *http.Request) {
		serveGeneratorRuntimeToolList(t, w, r, "drive", transport.ToolDescriptor{
			Name:        "search_files",
			Title:       "搜索文件",
			Description: "根据关键词搜索文件",
			InputSchema: map[string]any{"type": "object"},
		})
	})

	originalFactory := newEnvironmentLoader
	newEnvironmentLoader = func() cli.EnvironmentLoader {
		return cli.EnvironmentLoader{
			LookupEnv: func(key string) (string, bool) {
				if key == cli.CacheDirEnv {
					return cacheDir, true
				}
				return "", false
			},
			CatalogBaseURLOverride: server.URL,
			DiscoveryTimeout:       2 * time.Second,
		}
	}
	defer func() {
		newEnvironmentLoader = originalFactory
	}()

	catalog, err := LoadCatalogWithSource(context.Background(), string(CatalogSourceEnv), "")
	if err != nil {
		t.Fatalf("LoadCatalogWithSource(env) error = %v", err)
	}

	productIDs := make([]string, 0, len(catalog.Products))
	for _, product := range catalog.Products {
		productIDs = append(productIDs, product.ID)
	}
	sort.Strings(productIDs)

	if got, want := len(productIDs), 2; got != want {
		t.Fatalf("LoadCatalogWithSource(env) returned %d products, want %d (%v)", got, want, productIDs)
	}
	if productIDs[0] != "doc" || productIDs[1] != "drive" {
		t.Fatalf("LoadCatalogWithSource(env) products = %v, want [doc drive]", productIDs)
	}
}

func serveGeneratorRuntimeToolList(t *testing.T, w http.ResponseWriter, r *http.Request, serverName string, tools ...transport.ToolDescriptor) {
	t.Helper()

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatalf("Decode(JSON-RPC request) error = %v", err)
	}

	method, _ := req["method"].(string)
	switch method {
	case "initialize":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": serverName, "version": "1.0.0"},
			},
		})
	case "notifications/initialized":
		w.WriteHeader(http.StatusNoContent)
	case "tools/list":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"result": map[string]any{
				"tools": tools,
			},
		})
	default:
		t.Fatalf("unexpected JSON-RPC method %q", method)
	}
}
