package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

func TestDiscoverDetailCachesByServerKeyAndCLIID(t *testing.T) {
	t.Parallel()

	detail := market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			MCPID: 123,
			Tools: []market.DetailTool{
				{
					ToolName:     "search_documents",
					ToolTitle:    "Search Documents",
					ToolDesc:     "Search documents by keyword",
					ToolRequest:  `{"type":"object"}`,
					ToolResponse: `{"type":"object"}`,
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
	}))
	defer server.Close()

	svc := &Service{
		MarketClient: market.NewClient(server.URL, server.Client()),
		Transport:    transport.NewClient(server.Client()),
		Cache:        cache.NewStore(t.TempDir()),
		Tenant:       "tenant",
		AuthIdentity: "identity",
	}

	runtimeServer := market.ServerDescriptor{
		Key: "registry-key",
		CLI: market.CLIOverlay{ID: "cli-id"},
		DetailLocator: market.DetailLocator{
			MCPID:     123,
			DetailURL: server.URL + "/detail",
		},
	}

	if _, err := svc.DiscoverDetail(context.Background(), runtimeServer); err != nil {
		t.Fatalf("DiscoverDetail() error = %v", err)
	}

	for _, cacheKey := range []string{"registry-key", "cli-id"} {
		if _, _, err := svc.Cache.LoadDetail(svc.partition(), cacheKey); err != nil {
			t.Fatalf("LoadDetail(%q) error = %v", cacheKey, err)
		}
	}
}

func TestLoadCachedDetailRejectsCLIIDAliasReuseWithoutMCPID(t *testing.T) {
	t.Parallel()

	store := cache.NewStore(t.TempDir())
	partition := "tenant/identity"
	payload, err := json.Marshal(market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			MCPID: 777,
			Tools: []market.DetailTool{{
				ToolName:  "search_documents",
				ToolTitle: "Search Documents",
			}},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(detail) error = %v", err)
	}
	if err := store.SaveDetail(partition, "cli-id", cache.DetailSnapshot{
		MCPID:   777,
		Payload: payload,
	}); err != nil {
		t.Fatalf("SaveDetail() error = %v", err)
	}

	server := market.ServerDescriptor{
		Key: "registry-key",
		CLI: market.CLIOverlay{ID: "cli-id"},
	}
	if _, ok := LoadCachedDetail(store, partition, server); ok {
		t.Fatal("LoadCachedDetail() = ok, want false when MCPID is absent and only CLI.ID alias exists")
	}

	server.DetailLocator.MCPID = 777
	if _, ok := LoadCachedDetail(store, partition, server); !ok {
		t.Fatal("LoadCachedDetail() = false, want CLI.ID alias reuse when MCPID matches")
	}

	if err := store.SaveDetail(partition, "cli-id-zero", cache.DetailSnapshot{
		MCPID:   0,
		Payload: payload,
	}); err != nil {
		t.Fatalf("SaveDetail(zero) error = %v", err)
	}
	server = market.ServerDescriptor{
		Key:           "registry-key-zero",
		CLI:           market.CLIOverlay{ID: "cli-id-zero"},
		DetailLocator: market.DetailLocator{MCPID: 777},
	}
	if _, ok := LoadCachedDetail(store, partition, server); ok {
		t.Fatal("LoadCachedDetail() = ok, want false when alias-backed snapshot has MCPID 0")
	}
}
