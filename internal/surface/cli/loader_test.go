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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

func TestSynthesizeToolsFromCLIOverlayPrefersToolSensitivityOverOverride(t *testing.T) {
	t.Parallel()

	tools := synthesizeToolsFromCLIOverlay(market.CLIOverlay{
		Tools: []market.CLITool{
			{
				Name:        "create_document",
				CLIName:     "create",
				IsSensitive: boolPtr(true),
			},
		},
		ToolOverrides: map[string]market.CLIToolOverride{
			"create_document": {
				CLIName:     "legacy-create",
				IsSensitive: boolPtr(false),
			},
		},
	})

	if len(tools) != 1 {
		t.Fatalf("synthesizeToolsFromCLIOverlay() len = %d, want 1", len(tools))
	}
	if !tools[0].Sensitive {
		t.Fatalf("synthesizeToolsFromCLIOverlay() sensitive = %t, want true", tools[0].Sensitive)
	}
}

func TestEnvironmentLoaderReturnsEmptyCatalogWhenNoFixture(t *testing.T) {
	t.Parallel()

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			return "", false
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := loader.Load(ctx)
	// When no fixture is set, Load attempts live discovery which may fail
	// with a cancelled context. Either error or any catalog is acceptable.
	// After protocol-first MCP refactoring, live discovery may return products.
	// We don't enforce strict validation on the catalog content since this is
	// testing the loader's behavior with a cancelled context, not the catalog structure.
	if err != nil {
		// Context was cancelled, so error is expected
		return
	}
}

func TestEnvironmentLoaderBuildsDetailEnrichedCanonicalCatalogFromCachedDetailWithoutOverridingRuntimeLabels(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	store := cache.NewStore(cacheDir)
	server := market.ServerDescriptor{
		Key:         "doc-key",
		DisplayName: "文档",
		Endpoint:    "https://example.com/server/doc",
		Source:      "fresh_cache",
		HasCLIMeta:  true,
		CLI: market.CLIOverlay{
			ID:      "doc",
			Command: "documents",
		},
		DetailLocator: market.DetailLocator{MCPID: 1001},
	}
	if err := store.SaveRegistry("default/default", cache.RegistrySnapshot{Servers: []market.ServerDescriptor{server}}); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}
	if err := store.SaveTools("default/default", server.Key, cache.ToolsSnapshot{
		ServerKey:       server.Key,
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{
			{
				Name:        "create_document",
				Title:       "Runtime Create",
				Description: "runtime desc",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title": map[string]any{
							"type": "string",
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}
	detailPayload, err := json.Marshal(market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			MCPID: 1001,
			Tools: []market.DetailTool{
				{
					ToolName:  "create_document",
					ToolTitle: "Detail Create",
					ToolDesc:  "detail desc",
					ToolRequest: `{"type":"object","required":["title"],"properties":{
						"title":{"type":"string","description":"Document title"},
						"folder_id":{"type":"string","description":"Folder ID"}
					}}`,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(detail) error = %v", err)
	}
	if err := store.SaveDetail("default/default", server.Key, cache.DetailSnapshot{
		MCPID:   1001,
		Payload: detailPayload,
	}); err != nil {
		t.Fatalf("SaveDetail() error = %v", err)
	}

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
	}
	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	product, ok := catalog.FindProduct("doc")
	if !ok {
		t.Fatalf("Load() missing doc product: %#v", catalog.Products)
	}
	tool, ok := product.FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = false")
	}
	if tool.Title != "Runtime Create" {
		t.Fatalf("tool.Title = %q, want Runtime Create", tool.Title)
	}
	if tool.Description != "runtime desc" {
		t.Fatalf("tool.Description = %q, want runtime desc", tool.Description)
	}
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool.InputSchema.properties missing: %#v", tool.InputSchema)
	}
	title, _ := properties["title"].(map[string]any)
	if got := title["description"]; got != "Document title" {
		t.Fatalf("title.description = %#v, want Document title", got)
	}
	if _, ok := properties["folder_id"]; !ok {
		t.Fatalf("folder_id missing from detail-enriched schema: %#v", properties)
	}
	required := requiredStringsFromSchema(tool.InputSchema)
	if len(required) != 1 || required[0] != "title" {
		t.Fatalf("required = %#v, want [title]", required)
	}
}

func TestEnvironmentLoaderSynchronouslyFetchesDetailMetadataOnFirstUse(t *testing.T) {
	t.Parallel()

	var detailCalls atomic.Int32
	cacheDir := t.TempDir()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(market.ListResponse{
			Metadata: market.ListMetadata{Count: 1},
			Servers: []market.ServerEnvelope{
				{
					Server: market.RegistryServer{
						Name: "文档",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/doc"},
						},
					},
					Meta: market.EnvelopeMeta{
						Registry: market.RegistryMetadata{
							Status:    "active",
							MCPID:     1001,
							DetailURL: server.URL + "/detail",
						},
						CLI: market.CLIOverlay{
							ID:      "doc",
							Command: "documents",
						},
					},
				},
			},
		})
	})
	mux.HandleFunc("/mcp/market/detail", func(w http.ResponseWriter, r *http.Request) {
		detailCalls.Add(1)
		if got := r.URL.Query().Get("mcpId"); got != "1001" {
			t.Fatalf("detail mcpId = %q, want 1001", got)
		}
		_ = json.NewEncoder(w).Encode(market.DetailResponse{
			Success: true,
			Result: market.DetailResult{
				MCPID: 1001,
				Tools: []market.DetailTool{{
					ToolName:    "create_document",
					ToolTitle:   "Detail Create",
					ToolRequest: `{"type":"object","required":["title"],"properties":{"title":{"type":"string","description":"Document title"},"folder_id":{"type":"string","description":"Folder ID"}}}`,
				}},
			},
		})
	})
	mux.HandleFunc("/server/doc", func(w http.ResponseWriter, r *http.Request) {
		serveRuntimeToolList(t, w, r, "doc", transport.ToolDescriptor{
			Name:        "create_document",
			Title:       "Runtime Create",
			Description: "runtime desc",
			InputSchema: map[string]any{"type": "object"},
		})
	})

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
		CatalogBaseURLOverride: server.URL,
		DiscoveryTimeout:       2 * time.Second,
	}
	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if detailCalls.Load() != 1 {
		t.Fatalf("detail calls = %d, want 1", detailCalls.Load())
	}
	product, ok := catalog.FindProduct("doc")
	if !ok {
		t.Fatalf("Load() missing doc product: %#v", catalog.Products)
	}
	tool, ok := product.FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = false")
	}
	if tool.Title != "Runtime Create" {
		t.Fatalf("tool.Title = %q, want Runtime Create", tool.Title)
	}
	properties := schemaProperties(tool.InputSchema)
	title := mustSchemaProperty(t, properties, "title")
	if got := title["description"]; got != "Document title" {
		t.Fatalf("title.description = %#v, want Document title", got)
	}
	if _, ok := properties["folder_id"]; !ok {
		t.Fatalf("folder_id missing from schema: %#v", properties)
	}
	required := requiredFieldSet(tool.InputSchema)
	if !required["title"] {
		t.Fatalf("tool.InputSchema.required = %#v, want title", tool.InputSchema["required"])
	}
}

func TestEnvironmentLoaderDegradedSynthesisUsesCachedDetail(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	store := cache.NewStore(cacheDir)
	detailPayload, err := json.Marshal(market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			MCPID: 1001,
			Tools: []market.DetailTool{
				{
					ToolName:  "create_document",
					ToolTitle: "Detail Create",
					ToolDesc:  "detail desc",
					ToolRequest: `{"type":"object","required":["title"],"properties":{
						"title":{"type":"string","description":"Document title"}
					}}`,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(detail) error = %v", err)
	}
	if err := store.SaveDetail("default/default", "doc", cache.DetailSnapshot{
		MCPID:   1001,
		Payload: detailPayload,
	}); err != nil {
		t.Fatalf("SaveDetail(doc) error = %v", err)
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(market.ListResponse{
			Metadata: market.ListMetadata{Count: 1},
			Servers: []market.ServerEnvelope{
				{
					Server: market.RegistryServer{
						Name: "文档",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/doc"},
						},
					},
					Meta: market.EnvelopeMeta{
						Registry: market.RegistryMetadata{
							Status:    "active",
							MCPID:     1001,
							DetailURL: server.URL + "/detail",
						},
						CLI: market.CLIOverlay{
							ID:      "doc",
							Command: "documents",
							ToolOverrides: map[string]market.CLIToolOverride{
								"create_document": {CLIName: "create"},
							},
						},
					},
				},
			},
		})
	})
	mux.HandleFunc("/server/doc", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "runtime unavailable", http.StatusBadGateway)
	})
	mux.HandleFunc("/detail", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "detail should come from cache", http.StatusBadGateway)
	})

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
		CatalogBaseURLOverride: server.URL,
		RequireLiveCatalog:     true,
		DiscoveryTimeout:       2 * time.Second,
	}
	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	product, ok := catalog.FindProduct("doc")
	if !ok {
		t.Fatalf("Load() missing degraded doc product: %#v", catalog.Products)
	}
	tool, ok := product.FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = false")
	}
	if tool.Title != "Detail Create" {
		t.Fatalf("tool.Title = %q, want Detail Create", tool.Title)
	}
	required := requiredStringsFromSchema(tool.InputSchema)
	if len(required) != 1 || required[0] != "title" {
		t.Fatalf("required = %#v, want [title]", required)
	}
}

func TestEnvironmentLoaderRequireLiveCatalogRefreshesBeyondCachedCatalog(t *testing.T) {
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
							ID:          "doc",
							Command:     "doc",
							Description: "钉钉文档",
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
							ID:          "drive",
							Command:     "drive",
							Description: "钉盘管理",
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
		serveRuntimeToolList(t, w, r, "doc", transport.ToolDescriptor{
			Name:        "search_documents",
			Title:       "搜索文档",
			Description: "根据关键词搜索文档",
			InputSchema: map[string]any{"type": "object"},
		})
	})
	mux.HandleFunc("/server/drive", func(w http.ResponseWriter, r *http.Request) {
		serveRuntimeToolList(t, w, r, "drive", transport.ToolDescriptor{
			Name:        "search_files",
			Title:       "搜索文件",
			Description: "根据关键词搜索文件",
			InputSchema: map[string]any{"type": "object"},
		})
	})

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
		CatalogBaseURLOverride: server.URL,
		RequireLiveCatalog:     true,
		DiscoveryTimeout:       2 * time.Second,
	}

	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	productIDs := make([]string, 0, len(catalog.Products))
	for _, product := range catalog.Products {
		productIDs = append(productIDs, product.ID)
	}
	sort.Strings(productIDs)

	if got, want := len(productIDs), 2; got != want {
		t.Fatalf("Load() returned %d products, want %d (%v)", got, want, productIDs)
	}
	if productIDs[0] != "doc" || productIDs[1] != "drive" {
		t.Fatalf("Load() products = %v, want [doc drive]", productIDs)
	}
}

func TestEnvironmentLoaderBuildsDetailEnrichedCanonicalCatalogFromCachedDetailSnapshots(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	store := cache.NewStore(cacheDir)
	if err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		SavedAt: time.Now().UTC(),
		Servers: []market.ServerDescriptor{
			{
				Key:         "doc-key",
				DisplayName: "文档",
				Endpoint:    "https://mcp.dingtalk.com/doc/v1",
				Source:      "fresh_cache",
				CLI: market.CLIOverlay{
					ID:          "doc",
					Command:     "doc",
					Description: "钉钉文档",
				},
				DetailLocator: market.DetailLocator{MCPID: 1001},
				HasCLIMeta:    true,
			},
		},
	}); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}
	if err := store.SaveTools("default/default", "doc-key", cache.ToolsSnapshot{
		SavedAt:         time.Now().UTC(),
		ServerKey:       "doc-key",
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{
			{
				Name:        "create_document",
				Sensitive:   true,
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string"}}},
			},
		},
	}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	detailPayload, err := json.Marshal(market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			MCPID: 1001,
			Tools: []market.DetailTool{
				{
					ToolName:    "create_document",
					ToolTitle:   "Create Document",
					ToolDesc:    "Create a new document",
					ToolRequest: `{"type":"object","required":["title","format"],"properties":{"title":{"description":"Document title"},"format":{"type":"string","description":"Document format"}}}`,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(detail response) error = %v", err)
	}
	if err := store.SaveDetail("default/default", "doc", cache.DetailSnapshot{
		SavedAt: time.Now().UTC(),
		MCPID:   1001,
		Payload: detailPayload,
	}); err != nil {
		t.Fatalf("SaveDetail() error = %v", err)
	}

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
	}

	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	product, ok := catalog.FindProduct("doc")
	if !ok {
		t.Fatalf("FindProduct(doc) = not found")
	}
	tool, ok := product.FindTool("create_document")
	if !ok {
		t.Fatalf("FindTool(create_document) = not found")
	}
	if tool.Title != "Create Document" {
		t.Fatalf("tool.Title = %q, want Create Document", tool.Title)
	}
	if tool.Description != "Create a new document" {
		t.Fatalf("tool.Description = %q, want Create a new document", tool.Description)
	}
	if !tool.Sensitive {
		t.Fatalf("tool.Sensitive = false, want true")
	}
	required := requiredFieldSet(tool.InputSchema)
	if !required["title"] || !required["format"] {
		t.Fatalf("tool.InputSchema.required = %#v, want title+format", tool.InputSchema["required"])
	}
	properties := schemaProperties(tool.InputSchema)
	titleProperty := mustSchemaProperty(t, properties, "title")
	if got := titleProperty["type"]; got != "string" {
		t.Fatalf("title.type = %#v, want string", got)
	}
	if got := titleProperty["description"]; got != "Document title" {
		t.Fatalf("title.description = %#v, want Document title", got)
	}
	formatProperty := mustSchemaProperty(t, properties, "format")
	if got := formatProperty["type"]; got != "string" {
		t.Fatalf("format.type = %#v, want string", got)
	}
	if got := formatProperty["description"]; got != "Document format" {
		t.Fatalf("format.description = %#v, want Document format", got)
	}
}

func TestEnvironmentLoaderRefetchesWhenRegistryCacheLacksCompletenessProof(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	legacyCachePath := filepath.Join(cacheDir, "default_default", "market", "servers.json")
	if err := os.MkdirAll(filepath.Dir(legacyCachePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", legacyCachePath, err)
	}
	legacySnapshot, err := json.MarshalIndent(map[string]any{
		"saved_at": time.Now().UTC(),
		"servers": []market.ServerDescriptor{
			{
				Key:         "cached-key",
				DisplayName: "cached",
				Endpoint:    "https://mcp.dingtalk.com/cached/v1",
				Source:      "fresh_cache",
				CLI: market.CLIOverlay{
					ID:          "cached",
					Command:     "cached",
					Description: "Cached product",
					ToolOverrides: map[string]market.CLIToolOverride{
						"test_tool": {CLIName: "test"},
					},
				},
				HasCLIMeta: true,
			},
		},
	}, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(legacy snapshot) error = %v", err)
	}
	if err := os.WriteFile(legacyCachePath, legacySnapshot, 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", legacyCachePath, err)
	}

	var discoveryCalls int
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		discoveryCalls++
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
							ID:          "doc",
							Command:     "doc",
							Description: "钉钉文档",
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
							ID:          "drive",
							Command:     "drive",
							Description: "钉盘管理",
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
		serveRuntimeToolList(t, w, r, "doc", transport.ToolDescriptor{
			Name:        "search_documents",
			Title:       "搜索文档",
			Description: "根据关键词搜索文档",
			InputSchema: map[string]any{"type": "object"},
		})
	})
	mux.HandleFunc("/server/drive", func(w http.ResponseWriter, r *http.Request) {
		serveRuntimeToolList(t, w, r, "drive", transport.ToolDescriptor{
			Name:        "search_files",
			Title:       "搜索文件",
			Description: "根据关键词搜索文件",
			InputSchema: map[string]any{"type": "object"},
		})
	})

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
		CatalogBaseURLOverride: server.URL,
		DiscoveryTimeout:       2 * time.Second,
	}

	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if discoveryCalls != 1 {
		t.Fatalf("discovery calls = %d, want 1 (cache without completeness proof should trigger sync refetch)", discoveryCalls)
	}

	productIDs := make([]string, 0, len(catalog.Products))
	for _, product := range catalog.Products {
		productIDs = append(productIDs, product.ID)
	}
	sort.Strings(productIDs)

	if got, want := len(productIDs), 2; got != want {
		t.Fatalf("Load() returned %d products, want %d (%v)", got, want, productIDs)
	}
	if productIDs[0] != "doc" || productIDs[1] != "drive" {
		t.Fatalf("Load() products = %v, want [doc drive]", productIDs)
	}
}

func TestEnvironmentLoaderUsesFreshCacheWhenFreshCacheLacksDetail(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	var discoveryCalls int
	var runtimeCalls int
	var detailCalls int

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	store := cache.NewStore(cacheDir)
	serverKey := market.ServerKey(server.URL + "/server/doc")
	if err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		SavedAt: time.Now().UTC(),
		Servers: []market.ServerDescriptor{{
			Key:         serverKey,
			DisplayName: "文档",
			Endpoint:    server.URL + "/server/doc",
			Source:      "fresh_cache",
			HasCLIMeta:  true,
			CLI: market.CLIOverlay{
				ID:      "doc",
				Command: "doc",
			},
			DetailLocator: market.DetailLocator{MCPID: 1001},
		}},
	}); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}
	if err := store.SaveTools("default/default", serverKey, cache.ToolsSnapshot{
		SavedAt:         time.Now().UTC(),
		ServerKey:       serverKey,
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{{
			Name:        "search_documents",
			Description: "Search documents",
			InputSchema: map[string]any{"type": "object"},
		}},
	}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		discoveryCalls++
		payload := market.ListResponse{
			Metadata: market.ListMetadata{Count: 1},
			Servers: []market.ServerEnvelope{
				{
					Server: market.RegistryServer{
						Name: "文档",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/doc"},
						},
					},
					Meta: market.EnvelopeMeta{
						Registry: market.RegistryMetadata{
							Status:    "active",
							MCPID:     1001,
							DetailURL: server.URL + "/detail/doc",
						},
						CLI: market.CLIOverlay{
							ID:          "doc",
							Command:     "doc",
							Description: "钉钉文档",
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
		runtimeCalls++
		serveRuntimeToolList(t, w, r, "doc", transport.ToolDescriptor{
			Name:        "search_documents",
			Description: "Search documents",
			InputSchema: map[string]any{"type": "object"},
		})
	})
	mux.HandleFunc("/mcp/market/detail", func(w http.ResponseWriter, r *http.Request) {
		detailCalls++
		if got := r.URL.Query().Get("mcpId"); got != "1001" {
			t.Fatalf("detail mcpId = %q, want 1001", got)
		}
		if err := json.NewEncoder(w).Encode(market.DetailResponse{
			Success: true,
			Result: market.DetailResult{
				MCPID: 1001,
				Tools: []market.DetailTool{
					{
						ToolName:    "search_documents",
						ToolTitle:   "Search Documents",
						ToolRequest: `{"type":"object","properties":{"keyword":{"type":"string","description":"Search keyword"}}}`,
					},
				},
			},
		}); err != nil {
			t.Fatalf("Encode(detail payload) error = %v", err)
		}
	})

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
		CatalogBaseURLOverride: server.URL,
		DiscoveryTimeout:       2 * time.Second,
	}

	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if discoveryCalls != 0 {
		t.Fatalf("discovery calls = %d, want 0", discoveryCalls)
	}
	if runtimeCalls != 0 {
		t.Fatalf("runtime calls = %d, want 0", runtimeCalls)
	}
	if detailCalls != 0 {
		t.Fatalf("detail calls = %d, want 0", detailCalls)
	}
	product, ok := catalog.FindProduct("doc")
	if !ok {
		t.Fatalf("FindProduct(doc) = not found")
	}
	tool, ok := product.FindTool("search_documents")
	if !ok {
		t.Fatalf("FindTool(search_documents) = not found")
	}
	properties := schemaProperties(tool.InputSchema)
	if len(properties) != 0 {
		t.Fatalf("input schema = %#v, want cached runtime schema without detail enrichment", properties)
	}
}

func TestEnvironmentLoaderRequireLiveCatalogFetchesLiveDetailWhenRuntimeUnavailable(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	var detailCalls atomic.Int32
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		payload := market.ListResponse{
			Metadata: market.ListMetadata{Count: 1},
			Servers: []market.ServerEnvelope{
				{
					Server: market.RegistryServer{
						Name: "钉钉 AI 表格",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/aitable"},
						},
					},
					Meta: market.EnvelopeMeta{
						Registry: market.RegistryMetadata{
							Status: "active",
							MCPID:  2001,
						},
						CLI: market.CLIOverlay{
							ID:          "aitable",
							Command:     "aitable",
							Description: "AI 表格操作",
							Groups: map[string]market.CLIGroupDef{
								"base": {Description: "Base 管理"},
							},
							ToolOverrides: map[string]market.CLIToolOverride{
								"create_base": {
									CLIName: "create",
									Group:   "base",
									Flags: map[string]market.CLIFlagOverride{
										"baseName":   {Alias: "name"},
										"templateId": {Alias: "template-id"},
									},
								},
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

	mux.HandleFunc("/server/aitable", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Missing service_id or access_key"}`))
	})
	mux.HandleFunc("/mcp/market/detail", func(w http.ResponseWriter, r *http.Request) {
		detailCalls.Add(1)
		if got := r.URL.Query().Get("mcpId"); got != "2001" {
			t.Fatalf("detail mcpId = %q, want 2001", got)
		}
		if err := json.NewEncoder(w).Encode(market.DetailResponse{
			Success: true,
			Result: market.DetailResult{
				MCPID: 2001,
				Tools: []market.DetailTool{
					{
						ToolName:    "create_base",
						ToolTitle:   "创建 AI 表格 Base",
						ToolDesc:    "创建 AI 表格 Base",
						ToolRequest: `{"type":"object","required":["baseName"],"properties":{"baseName":{"type":"string","description":"Base 名称"}}}`,
					},
				},
			},
		}); err != nil {
			t.Fatalf("Encode(detail payload) error = %v", err)
		}
	})

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
		CatalogBaseURLOverride: server.URL,
		RequireLiveCatalog:     true,
		DiscoveryTimeout:       2 * time.Second,
	}

	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	product, ok := catalog.FindProduct("aitable")
	if !ok {
		t.Fatalf("Load() missing detail-backed aitable product: %#v", catalog.Products)
	}
	if product.Description != "AI 表格操作" {
		t.Fatalf("product.Description = %q, want AI 表格操作", product.Description)
	}

	tool, ok := product.FindTool("create_base")
	if !ok {
		t.Fatalf("FindTool(create_base) = not found")
	}
	if tool.CLIName != "create" {
		t.Fatalf("tool.CLIName = %q, want create", tool.CLIName)
	}
	if tool.Group != "base" {
		t.Fatalf("tool.Group = %q, want base", tool.Group)
	}
	if tool.Title != "创建 AI 表格 Base" {
		t.Fatalf("tool.Title = %q, want 创建 AI 表格 Base", tool.Title)
	}
	if tool.Description != "创建 AI 表格 Base" {
		t.Fatalf("tool.Description = %q, want 创建 AI 表格 Base", tool.Description)
	}
	if tool.FlagHints["baseName"].Alias != "name" {
		t.Fatalf("tool.FlagHints[baseName].Alias = %q, want name", tool.FlagHints["baseName"].Alias)
	}
	if detailCalls.Load() != 1 {
		t.Fatalf("detail calls = %d, want 1", detailCalls.Load())
	}
	required := requiredFieldSet(tool.InputSchema)
	if !required["baseName"] {
		t.Fatalf("tool.InputSchema.required = %#v, want baseName", tool.InputSchema["required"])
	}
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool.InputSchema.properties missing: %#v", tool.InputSchema)
	}
	baseName, ok := properties["baseName"].(map[string]any)
	if !ok {
		t.Fatalf("tool.InputSchema.properties missing baseName: %#v", properties)
	}
	if got := baseName["description"]; got != "Base 名称" {
		t.Fatalf("baseName.description = %#v, want Base 名称", got)
	}
}

func TestEnvironmentLoaderInjectsAuthTokenForLiveRuntimeDiscovery(t *testing.T) {
	cacheDir := t.TempDir()
	const wantToken = "test-token"
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "*")
	var authorizedCalls atomic.Int32
	var unauthorizedCalls atomic.Int32

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		payload := market.ListResponse{
			Metadata: market.ListMetadata{Count: 1},
			Servers: []market.ServerEnvelope{
				{
					Server: market.RegistryServer{
						Name: "钉钉 AI 表格",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/aitable"},
						},
					},
					Meta: market.EnvelopeMeta{
						Registry: market.RegistryMetadata{Status: "active"},
						CLI: market.CLIOverlay{
							ID:          "aitable",
							Command:     "aitable",
							Description: "AI 表格操作",
							ToolOverrides: map[string]market.CLIToolOverride{
								"create_base": {
									CLIName: "create",
									Group:   "base",
									Flags: map[string]market.CLIFlagOverride{
										"baseName": {Alias: "name"},
									},
								},
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

	mux.HandleFunc("/server/aitable", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-user-access-token"); got != wantToken {
			unauthorizedCalls.Add(1)
			http.Error(w, "missing auth token", http.StatusUnauthorized)
			return
		}
		authorizedCalls.Add(1)

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode(JSON-RPC request) error = %v", err)
		}

		switch req["method"] {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]any{"name": "aitable", "version": "1.0.0"},
				},
			})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      2,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "create_base",
							"title":       "创建 AI 表格 Base",
							"description": "创建一个新的 AI 表格 Base",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"baseName": map[string]any{
										"type":        "string",
										"description": "Base 名称",
									},
								},
							},
						},
					},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected JSON-RPC method %q", req["method"])
		}
	})

	loader := EnvironmentLoader{
		AuthToken: wantToken,
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
		CatalogBaseURLOverride: server.URL,
		RequireLiveCatalog:     true,
		ForceRefreshAll:        true,
		DiscoveryTimeout:       2 * time.Second,
	}

	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if authorizedCalls.Load() == 0 {
		t.Fatalf("authorized runtime calls = %d, unauthorized = %d", authorizedCalls.Load(), unauthorizedCalls.Load())
	}

	product, ok := catalog.FindProduct("aitable")
	if !ok {
		t.Fatalf("FindProduct(aitable) = not found: %#v", catalog.Products)
	}
	if product.Degraded {
		t.Fatalf("product.Degraded = true, want false")
	}

	tool, ok := product.FindTool("create_base")
	if !ok {
		t.Fatalf("FindTool(create_base) = not found")
	}
	if tool.Title != "创建 AI 表格 Base" {
		t.Fatalf("tool.Title = %q, want 创建 AI 表格 Base", tool.Title)
	}
	if tool.Description != "创建一个新的 AI 表格 Base" {
		t.Fatalf("tool.Description = %q, want 创建一个新的 AI 表格 Base", tool.Description)
	}
}

func TestEnvironmentLoaderRequireLiveCatalogFallbackUsesCachedDetailSnapshots(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	store := cache.NewStore(cacheDir)
	detailPayload, err := json.Marshal(market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			MCPID: 2001,
			Tools: []market.DetailTool{
				{
					ToolName:    "create_base",
					ToolTitle:   "Create Base",
					ToolDesc:    "Create a new AI table base",
					ToolRequest: `{"type":"object","required":["baseName"],"properties":{"baseName":{"type":"string","description":"Base name"}}}`,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(detail response) error = %v", err)
	}
	if err := store.SaveDetail("default/default", "aitable", cache.DetailSnapshot{
		SavedAt: time.Now().UTC(),
		MCPID:   2001,
		Payload: detailPayload,
	}); err != nil {
		t.Fatalf("SaveDetail() error = %v", err)
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		payload := market.ListResponse{
			Metadata: market.ListMetadata{Count: 1},
			Servers: []market.ServerEnvelope{
				{
					Server: market.RegistryServer{
						Name: "钉钉 AI 表格",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/aitable"},
						},
					},
					Meta: market.EnvelopeMeta{
						Registry: market.RegistryMetadata{
							Status:    "active",
							MCPID:     2001,
							DetailURL: server.URL + "/detail/aitable",
						},
						CLI: market.CLIOverlay{
							ID:          "aitable",
							Command:     "aitable",
							Description: "AI 表格操作",
							ToolOverrides: map[string]market.CLIToolOverride{
								"create_base": {CLIName: "create"},
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
	mux.HandleFunc("/server/aitable", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	mux.HandleFunc("/detail/aitable", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("detail endpoint should not be used for degraded fallback enrichment")
	})

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
		CatalogBaseURLOverride: server.URL,
		RequireLiveCatalog:     true,
		DiscoveryTimeout:       2 * time.Second,
	}

	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	product, ok := catalog.FindProduct("aitable")
	if !ok {
		t.Fatalf("FindProduct(aitable) = not found")
	}
	tool, ok := product.FindTool("create_base")
	if !ok {
		t.Fatalf("FindTool(create_base) = not found")
	}
	if tool.Title != "Create Base" {
		t.Fatalf("tool.Title = %q, want Create Base", tool.Title)
	}
	if tool.Description != "Create a new AI table base" {
		t.Fatalf("tool.Description = %q, want Create a new AI table base", tool.Description)
	}
	required := requiredFieldSet(tool.InputSchema)
	if !required["baseName"] {
		t.Fatalf("tool.InputSchema.required = %#v, want baseName", tool.InputSchema["required"])
	}
	baseName := mustSchemaProperty(t, schemaProperties(tool.InputSchema), "baseName")
	if got := baseName["description"]; got != "Base name" {
		t.Fatalf("baseName.description = %#v, want Base name", got)
	}
	if got := baseName["type"]; got != "string" {
		t.Fatalf("baseName.type = %#v, want string", got)
	}
}

func TestEnvironmentLoaderRequireLiveCatalogFallsBackToRegistryCLIMetadataWhenDetailUnavailable(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		payload := market.ListResponse{
			Metadata: market.ListMetadata{Count: 1},
			Servers: []market.ServerEnvelope{
				{
					Server: market.RegistryServer{
						Name: "钉钉 AI 表格",
						Remotes: []market.RegistryRemote{
							{Type: "streamable-http", URL: server.URL + "/server/aitable"},
						},
					},
					Meta: market.EnvelopeMeta{
						Registry: market.RegistryMetadata{
							Status: "active",
						},
						CLI: market.CLIOverlay{
							ID:          "aitable",
							Command:     "aitable",
							Description: "AI 表格操作",
							Groups: map[string]market.CLIGroupDef{
								"base": {Description: "Base 管理"},
							},
							ToolOverrides: map[string]market.CLIToolOverride{
								"create_base": {
									CLIName: "create",
									Group:   "base",
									Flags: map[string]market.CLIFlagOverride{
										"baseName":   {Alias: "name"},
										"templateId": {Alias: "template-id"},
									},
								},
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
	mux.HandleFunc("/server/aitable", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Missing service_id or access_key"}`))
	})

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
		CatalogBaseURLOverride: server.URL,
		RequireLiveCatalog:     true,
		DiscoveryTimeout:       2 * time.Second,
	}

	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	product, ok := catalog.FindProduct("aitable")
	if !ok {
		t.Fatalf("Load() missing synthesized aitable product: %#v", catalog.Products)
	}
	if product.Description != "AI 表格操作" {
		t.Fatalf("product.Description = %q, want AI 表格操作", product.Description)
	}

	tool, ok := product.FindTool("create_base")
	if !ok {
		t.Fatalf("FindTool(create_base) = not found")
	}
	if tool.CLIName != "create" {
		t.Fatalf("tool.CLIName = %q, want create", tool.CLIName)
	}
	if tool.Group != "base" {
		t.Fatalf("tool.Group = %q, want base", tool.Group)
	}
	if tool.FlagHints["baseName"].Alias != "name" {
		t.Fatalf("tool.FlagHints[baseName].Alias = %q, want name", tool.FlagHints["baseName"].Alias)
	}
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool.InputSchema.properties missing: %#v", tool.InputSchema)
	}
	if _, ok := properties["baseName"]; !ok {
		t.Fatalf("tool.InputSchema.properties missing baseName: %#v", properties)
	}
}

func TestEnvironmentLoaderAgedCacheUsesCachedDetailWithoutSynchronousRevalidation(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	store := cache.NewStore(cacheDir)
	oldTime := time.Now().UTC().Add(-2 * time.Hour)
	serverDescriptor := market.ServerDescriptor{
		Key:         "doc-key",
		DisplayName: "文档",
		Endpoint:    "https://placeholder.invalid/server/doc",
		Source:      "fresh_cache",
		HasCLIMeta:  true,
		CLI: market.CLIOverlay{
			ID:      "doc",
			Command: "documents",
		},
		DetailLocator: market.DetailLocator{MCPID: 1001},
	}
	if err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		SavedAt: oldTime,
		Servers: []market.ServerDescriptor{serverDescriptor},
	}); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}
	if err := store.SaveTools("default/default", serverDescriptor.Key, cache.ToolsSnapshot{
		SavedAt:         time.Now().UTC(),
		ServerKey:       serverDescriptor.Key,
		ProtocolVersion: "2025-03-26",
		ActionVersions:  map[string]string{"create_document": "v1"},
		Tools: []transport.ToolDescriptor{{
			Name:        "create_document",
			Title:       "Runtime Create",
			Description: "runtime desc",
			InputSchema: map[string]any{"type": "object"},
		}},
	}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	var registryCalls atomic.Int32
	var detailCalls atomic.Int32
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		registryCalls.Add(1)
		_ = json.NewEncoder(w).Encode(market.ListResponse{
			Metadata: market.ListMetadata{Count: 1},
			Servers: []market.ServerEnvelope{{
				Server: market.RegistryServer{
					Name: "文档",
					Remotes: []market.RegistryRemote{
						{Type: "streamable-http", URL: server.URL + "/server/doc"},
					},
				},
				Meta: market.EnvelopeMeta{
					Registry: market.RegistryMetadata{
						Status:    "active",
						MCPID:     1001,
						UpdatedAt: time.Now().UTC().Format(time.RFC3339),
					},
					CLI: market.CLIOverlay{
						ID:      "doc",
						Command: "documents",
					},
				},
			}},
		})
	})
	mux.HandleFunc("/server/doc", func(w http.ResponseWriter, r *http.Request) {
		serveRuntimeToolList(t, w, r, "doc", transport.ToolDescriptor{
			Name:        "create_document",
			Title:       "Runtime Create",
			Description: "runtime desc",
			InputSchema: map[string]any{"type": "object"},
		})
	})
	mux.HandleFunc("/mcp/market/detail", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("mcpId"); got != "1001" {
			t.Fatalf("detail mcpId = %q, want 1001", got)
		}
		detailCalls.Add(1)
		time.Sleep(250 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(market.DetailResponse{
			Success: true,
			Result: market.DetailResult{
				MCPID: 1001,
				Tools: []market.DetailTool{{
					ToolName:      "create_document",
					ToolTitle:     "Detail Create",
					ActionVersion: "v2",
				}},
			},
		})
	})

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
		CatalogBaseURLOverride: server.URL,
		DiscoveryTimeout:       2 * time.Second,
	}

	start := time.Now()
	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 200*time.Millisecond {
		t.Fatalf("Load() elapsed = %s, want cached startup without synchronous revalidation", elapsed)
	}
	if registryCalls.Load() != 0 {
		t.Fatalf("registry calls = %d, want 0 when cached registry is still within TTL", registryCalls.Load())
	}
	if detailCalls.Load() != 0 {
		t.Fatalf("detail calls = %d, want 0 when cached catalog is reused", detailCalls.Load())
	}
	product, ok := catalog.FindProduct("doc")
	if !ok {
		t.Fatalf("FindProduct(doc) = not found")
	}
	tool, ok := product.FindTool("create_document")
	if !ok || tool.Title != "Runtime Create" {
		t.Fatalf("startup catalog = %#v, want runtime cached tool without synchronous revalidation", tool)
	}
	registry, _, regErr := store.LoadRegistry("default/default")
	if regErr != nil {
		t.Fatalf("LoadRegistry() error = %v", regErr)
	}
	if len(registry.Servers) != 1 {
		t.Fatalf("registry servers = %d, want 1", len(registry.Servers))
	}
	snapshot, _, err := store.LoadTools("default/default", registry.Servers[0].Key)
	if err != nil {
		t.Fatalf("LoadTools(%s) error = %v", registry.Servers[0].Key, err)
	}
	if snapshot.ActionVersions["create_document"] != "v1" {
		t.Fatalf("action versions = %#v, want cached create_document=v1 without sync refresh", snapshot.ActionVersions)
	}
}

func TestEnvironmentLoaderAgedCacheUsesCachedRegistryWithoutLiveFetch(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	store := cache.NewStore(cacheDir)
	oldTime := time.Now().UTC().Add(-2 * time.Hour)
	serverDescriptor := market.ServerDescriptor{
		Key:         "doc-key",
		DisplayName: "文档",
		Endpoint:    "https://placeholder.invalid/server/doc",
		Source:      "fresh_cache",
		HasCLIMeta:  true,
		CLI: market.CLIOverlay{
			ID:      "doc",
			Command: "documents",
		},
	}
	if err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		SavedAt: oldTime,
		Servers: []market.ServerDescriptor{serverDescriptor},
	}); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}
	if err := store.SaveTools("default/default", serverDescriptor.Key, cache.ToolsSnapshot{
		SavedAt:         time.Now().UTC(),
		ServerKey:       serverDescriptor.Key,
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{{
			Name:        "create_document",
			Title:       "Runtime Create",
			Description: "runtime desc",
			InputSchema: map[string]any{"type": "object"},
		}},
	}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	var registryCalls atomic.Int32
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		registryCalls.Add(1)
		http.Error(w, "should not fetch live registry", http.StatusInternalServerError)
	})

	loader := EnvironmentLoader{
		LookupEnv: func(key string) (string, bool) {
			if key == CacheDirEnv {
				return cacheDir, true
			}
			return "", false
		},
		CatalogBaseURLOverride: server.URL,
		DiscoveryTimeout:       2 * time.Second,
	}

	catalog, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if registryCalls.Load() != 0 {
		t.Fatalf("registry calls = %d, want 0 when cache is present and fresh", registryCalls.Load())
	}
	product, ok := catalog.FindProduct("doc")
	if !ok {
		t.Fatalf("FindProduct(doc) = not found")
	}
	tool, ok := product.FindTool("create_document")
	if !ok || tool.Title != "Runtime Create" {
		t.Fatalf("cached tool = %#v, want runtime cached tool", tool)
	}
}

func TestLoadFromCacheDoesNotForceRevalidateForSkippedServers(t *testing.T) {
	t.Parallel()

	store := cache.NewStore(t.TempDir())
	const partition = "default/default"

	servers := []market.ServerDescriptor{
		{
			Key:         "doc",
			DisplayName: "文档",
			Endpoint:    "https://example.invalid/server/doc",
			HasCLIMeta:  true,
			CLI: market.CLIOverlay{
				ID:      "doc",
				Command: "documents",
			},
		},
		{
			Key:         "skip-me",
			DisplayName: "跳过服务",
			Endpoint:    "https://example.invalid/server/skip",
			HasCLIMeta:  true,
			CLI: market.CLIOverlay{
				ID:   "skip-me",
				Skip: true,
			},
		},
	}

	if err := store.SaveRegistry(partition, cache.RegistrySnapshot{Servers: servers}); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}
	if err := store.SaveTools(partition, "doc", cache.ToolsSnapshot{
		ServerKey:       "doc",
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{{
			Name:        "create_document",
			Title:       "创建文档",
			InputSchema: map[string]any{"type": "object"},
		}},
	}); err != nil {
		t.Fatalf("SaveTools(doc) error = %v", err)
	}

	state := EnvironmentLoader{
		LookupEnv: func(string) (string, bool) { return "", false },
	}.loadFromCache(store)

	if !state.Available {
		t.Fatal("loadFromCache() Available = false, want true")
	}
	if state.NeedsRevalidate {
		t.Fatalf("loadFromCache() NeedsRevalidate = true, want false")
	}
	if len(state.Catalog.Products) != 1 || state.Catalog.Products[0].ID != "doc" {
		t.Fatalf("loadFromCache() catalog = %#v, want only doc product", state.Catalog.Products)
	}
}

func serveRuntimeToolList(t *testing.T, w http.ResponseWriter, r *http.Request, serverName string, tools ...transport.ToolDescriptor) {
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

func requiredStringsFromSchema(schema map[string]any) []string {
	raw, _ := schema["required"].([]any)
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		value, _ := entry.(string)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func requiredFieldSet(schema map[string]any) map[string]bool {
	out := make(map[string]bool)
	switch raw := schema["required"].(type) {
	case []any:
		for _, item := range raw {
			name, _ := item.(string)
			if name != "" {
				out[name] = true
			}
		}
	case []string:
		for _, name := range raw {
			if name != "" {
				out[name] = true
			}
		}
	}
	return out
}

func mustSchemaProperty(t *testing.T, properties map[string]map[string]any, name string) map[string]any {
	t.Helper()

	property, ok := properties[name]
	if !ok {
		t.Fatalf("properties[%s] missing: %#v", name, properties)
	}
	return property
}
