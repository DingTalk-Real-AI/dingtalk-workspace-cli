package unit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

func TestDiscoverDetailFallsBackToCachedSnapshot(t *testing.T) {
	t.Parallel()

	store := cache.NewStore(t.TempDir())
	store.Now = func() time.Time { return time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC) }
	payload := market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			MCPID: 9629,
			Name:  "钉钉文档",
			Tools: []market.DetailTool{{ToolName: "create_document", ToolTitle: "创建文档"}},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := store.SaveDetail("default/default", "doc", cache.DetailSnapshot{
		MCPID:   9629,
		Payload: raw,
	}); err != nil {
		t.Fatalf("SaveDetail() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	service := discovery.NewService(market.NewClient(server.URL, server.Client()), transport.NewClient(server.Client()), store)
	detail, err := service.DiscoverDetail(context.Background(), market.ServerDescriptor{
		Key: "doc",
		DetailLocator: market.DetailLocator{
			MCPID: 9629,
		},
	})
	if err != nil {
		t.Fatalf("DiscoverDetail() error = %v", err)
	}
	if !detail.Success || detail.Result.MCPID != 9629 {
		t.Fatalf("DiscoverDetail() detail = %#v", detail)
	}
}

func TestDiscoverDetailRejectsCachedSnapshotForDifferentMCPID(t *testing.T) {
	t.Parallel()

	store := cache.NewStore(t.TempDir())
	store.Now = func() time.Time { return time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC) }
	payload := market.DetailResponse{
		Success: true,
		Result: market.DetailResult{
			MCPID: 1047,
			Name:  "钉钉文档（旧）",
			Tools: []market.DetailTool{{ToolName: "search_documents", ToolTitle: "搜索文档"}},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := store.SaveDetail("default/default", "doc", cache.DetailSnapshot{
		MCPID:   1047,
		Payload: raw,
	}); err != nil {
		t.Fatalf("SaveDetail() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	service := discovery.NewService(market.NewClient(server.URL, server.Client()), transport.NewClient(server.Client()), store)
	_, err = service.DiscoverDetail(context.Background(), market.ServerDescriptor{
		Key: "doc",
		DetailLocator: market.DetailLocator{
			MCPID: 9629,
		},
	})
	if err == nil {
		t.Fatal("DiscoverDetail() error = nil, want live fetch failure")
	}
}
