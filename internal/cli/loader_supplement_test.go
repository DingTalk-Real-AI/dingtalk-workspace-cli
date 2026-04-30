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
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// TestLoadFromCache_SupplementFillsGaps simulates the Wukong gray-release
// scenario: the Portal envelope only carries `live`, but the edition's
// SupplementServers hook ships the hardcoded endpoints for `conference` and
// `todo`. The resulting catalog must expose all three so runtime endpoint
// resolution does not depend on the historical default-partition accident.
func TestLoadFromCache_SupplementFillsGaps(t *testing.T) {
	setEdition(t, &edition.Hooks{
		Name: "wukong",
		SupplementServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{
				{ID: "conference", Name: "会议", Endpoint: "https://example.invalid/conference"},
				{ID: "todo", Name: "待办", Endpoint: "https://example.invalid/todo"},
				// Duplicate of the discovery entry — MUST be overridden by
				// the discovery entry (discovery wins on ID collision).
				{ID: "live", Name: "直播(supplement)", Endpoint: "https://example.invalid/overridden"},
			}
		},
	})

	root := t.TempDir()
	now := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	store := cache.NewStore(root)
	store.Now = func() time.Time { return now }

	liveEndpoint := "https://example.invalid/live"
	seedRegistryCache(t, store, "wukong/default", now, []market.ServerDescriptor{
		{
			Key:         "live",
			DisplayName: "直播",
			Endpoint:    liveEndpoint,
			CLI: market.CLIOverlay{
				ID:      "live",
				Command: "live",
			},
		},
	})

	loader := EnvironmentLoader{}
	state := loader.loadFromCache(store)
	if !state.Available {
		t.Fatalf("expected cached state available; got %+v", state)
	}

	wantIDs := map[string]string{
		"conference": "https://example.invalid/conference",
		"todo":       "https://example.invalid/todo",
		"live":       liveEndpoint, // discovery wins, NOT the supplement's overridden URL
	}
	for id, wantEndpoint := range wantIDs {
		product, ok := state.Catalog.FindProduct(id)
		if !ok {
			t.Errorf("catalog missing product %q; have %v", id, productIDs(state.Catalog.Products))
			continue
		}
		if product.Endpoint != wantEndpoint {
			t.Errorf("product %q endpoint = %q, want %q", id, product.Endpoint, wantEndpoint)
		}
	}
}

// TestLoadFromCache_EmptyRegistry_StillExposesSupplement covers the cold-start
// gray-release case: no cached registry at all, but the edition still knows
// about a set of hardcoded products. Those should be exposed via the catalog
// so `dws foo bar` does not fail with endpoint_not_resolved on first run.
func TestLoadFromCache_EmptyRegistry_StillExposesSupplement(t *testing.T) {
	setEdition(t, &edition.Hooks{
		Name: "wukong",
		SupplementServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{
				{ID: "conference", Name: "会议", Endpoint: "https://example.invalid/conference"},
			}
		},
	})

	root := t.TempDir()
	now := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	store := cache.NewStore(root)
	store.Now = func() time.Time { return now }

	loader := EnvironmentLoader{}
	state := loader.loadFromCache(store)

	if !state.Available {
		t.Fatalf("expected cached state available via supplement; got %+v", state)
	}
	if _, ok := state.Catalog.FindProduct("conference"); !ok {
		t.Fatalf("supplement did not surface conference into catalog; products=%v", productIDs(state.Catalog.Products))
	}
	if !state.NeedsRevalidate {
		t.Errorf("NeedsRevalidate should be true when only supplement is available")
	}
}

// TestFallbackRuntimeServers_UsedWhenDiscoveryFailsWithoutCache exercises
// the worst-case path: no cached registry AND no live discovery (embedded
// scenario where AuthTokenFunc returns empty). FallbackServers must still
// surface a usable catalog.
func TestFallbackRuntimeServers_UsedWhenDiscoveryFailsWithoutCache(t *testing.T) {
	setEdition(t, &edition.Hooks{
		Name: "wukong",
		FallbackServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{
				{ID: "conference", Name: "会议", Endpoint: "https://fallback.invalid/conference"},
			}
		},
		SupplementServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{
				{ID: "extra", Name: "Extra", Endpoint: "https://fallback.invalid/extra"},
			}
		},
	})

	rs := fallbackRuntimeServers()
	if len(rs) != 2 {
		t.Fatalf("fallbackRuntimeServers() len = %d, want 2 (fallback + non-overlapping supplement); got %v", len(rs), rs)
	}

	ids := make(map[string]bool, len(rs))
	for _, r := range rs {
		ids[r.Server.CLI.ID] = true
	}
	for _, want := range []string{"conference", "extra"} {
		if !ids[want] {
			t.Errorf("fallbackRuntimeServers() missing %q; have %v", want, ids)
		}
	}
}
