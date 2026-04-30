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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// setEdition overrides the active edition hooks for the duration of the test.
func setEdition(t *testing.T, h *edition.Hooks) {
	t.Helper()
	prev := edition.Get()
	edition.Override(h)
	t.Cleanup(func() { edition.Override(prev) })
}

func seedRegistryCache(t *testing.T, store *cache.Store, partition string, savedAt time.Time, servers []market.ServerDescriptor) {
	t.Helper()
	if err := store.SaveRegistry(partition, cache.RegistrySnapshot{
		SavedAt: savedAt,
		Servers: servers,
	}); err != nil {
		t.Fatalf("SaveRegistry(%q) error = %v", partition, err)
	}
	for _, server := range servers {
		if err := store.SaveTools(partition, server.Key, cache.ToolsSnapshot{
			SavedAt:   savedAt,
			ServerKey: server.Key,
		}); err != nil {
			t.Fatalf("SaveTools(%q) error = %v", server.Key, err)
		}
	}
}

// TestLoadFromCache_UsesEditionPartition verifies that loadFromCache reads
// from the edition-specific partition (wukong/default) instead of the
// historical hardcoded default/default. Before the fix, an entry written to
// wukong/default was invisible to the runtime catalog loader — which is
// exactly what caused `dws conference meeting create` to report
// endpoint_not_resolved while todo succeeded (the open-source Market cache
// happened to carry todo).
func TestLoadFromCache_UsesEditionPartition(t *testing.T) {
	setEdition(t, &edition.Hooks{Name: "wukong"})

	root := t.TempDir()
	now := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	store := cache.NewStore(root)
	store.Now = func() time.Time { return now }

	seedRegistryCache(t, store, "wukong/default", now, []market.ServerDescriptor{
		{
			Key:         "conference",
			DisplayName: "会议",
			Endpoint:    "https://example.invalid/conference",
			CLI: market.CLIOverlay{
				ID:      "conference",
				Command: "conference",
			},
		},
	})

	loader := EnvironmentLoader{}
	state := loader.loadFromCache(store)

	if !state.Available {
		t.Fatalf("expected cached state available; got %+v", state)
	}
	if _, ok := state.Catalog.FindProduct("conference"); !ok {
		t.Fatalf("conference not in catalog; products=%v", productIDs(state.Catalog.Products))
	}
}

// TestLoadFromCache_IgnoresDefaultPartitionForOverlay asserts the cross-partition
// leak is gone: writing servers under default/default while the edition is
// Wukong must NOT surface in the runtime catalog. Previously this path was
// the accidental fallback that let `dws todo` work on a gray-released host.
func TestLoadFromCache_IgnoresDefaultPartitionForOverlay(t *testing.T) {
	setEdition(t, &edition.Hooks{Name: "wukong"})

	root := t.TempDir()
	now := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	store := cache.NewStore(root)
	store.Now = func() time.Time { return now }

	seedRegistryCache(t, store, "default/default", now, []market.ServerDescriptor{
		{
			Key:         "todo",
			DisplayName: "待办",
			Endpoint:    "https://example.invalid/todo",
			CLI: market.CLIOverlay{
				ID:      "todo",
				Command: "todo",
			},
		},
	})

	loader := EnvironmentLoader{}
	state := loader.loadFromCache(store)

	if state.Available {
		if _, ok := state.Catalog.FindProduct("todo"); ok {
			t.Fatalf("todo leaked from default/default into wukong catalog (partition isolation regressed)")
		}
	}
}

// TestLoadFromCache_OpenEdition_UsesDefaultPartition keeps the open-source
// core behaviour intact: with edition.Name == "" (zero value), loadFromCache
// must still read default/default.
func TestLoadFromCache_OpenEdition_UsesDefaultPartition(t *testing.T) {
	setEdition(t, &edition.Hooks{})

	root := t.TempDir()
	now := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	store := cache.NewStore(root)
	store.Now = func() time.Time { return now }

	seedRegistryCache(t, store, "default/default", now, []market.ServerDescriptor{
		{
			Key:         "doc",
			DisplayName: "文档",
			Endpoint:    "https://example.invalid/doc",
			CLI: market.CLIOverlay{
				ID:      "doc",
				Command: "doc",
			},
		},
	})

	loader := EnvironmentLoader{}
	state := loader.loadFromCache(store)

	if !state.Available {
		t.Fatalf("expected cached state available for open edition; got %+v", state)
	}
	if _, ok := state.Catalog.FindProduct("doc"); !ok {
		t.Fatalf("doc not in catalog; products=%v", productIDs(state.Catalog.Products))
	}
}

func productIDs(products []ir.CanonicalProduct) []string {
	ids := make([]string, 0, len(products))
	for _, p := range products {
		ids = append(ids, p.ID)
	}
	return ids
}
