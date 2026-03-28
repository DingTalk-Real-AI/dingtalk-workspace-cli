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

package cache

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

func TestRegistrySnapshotFreshness(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC)
	store := NewStore(root)
	store.Now = func() time.Time { return now }

	err := store.SaveRegistry("tenant/default", RegistrySnapshot{
		SavedAt: now,
		Servers: []market.ServerDescriptor{{Key: "doc", DisplayName: "文档"}},
	})
	if err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	_, freshness, err := store.LoadRegistry("tenant/default")
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	if freshness != FreshnessFresh {
		t.Fatalf("LoadRegistry() freshness = %s, want %s", freshness, FreshnessFresh)
	}

	store.Now = func() time.Time { return now.Add(25 * time.Hour) }
	_, freshness, err = store.LoadRegistry("tenant/default")
	if err != nil {
		t.Fatalf("LoadRegistry() stale error = %v", err)
	}
	if freshness != FreshnessStale {
		t.Fatalf("LoadRegistry() stale freshness = %s, want %s", freshness, FreshnessStale)
	}

	if _, err := filepath.Abs(root); err != nil {
		t.Fatalf("unexpected temp dir error: %v", err)
	}
}

func TestToolsSnapshotRoundTrip(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	err := store.SaveTools("tenant/default", "doc", ToolsSnapshot{
		ServerKey:       "doc",
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{
			{Name: "create_document", Title: "创建文档"},
		},
	})
	if err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	snapshot, _, err := store.LoadTools("tenant/default", "doc")
	if err != nil {
		t.Fatalf("LoadTools() error = %v", err)
	}
	if snapshot.ProtocolVersion != "2025-03-26" {
		t.Fatalf("LoadTools() protocol = %q, want 2025-03-26", snapshot.ProtocolVersion)
	}
	if len(snapshot.Tools) != 1 {
		t.Fatalf("LoadTools() len = %d, want 1", len(snapshot.Tools))
	}
}

func TestDetailSnapshotUsesDetailTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir())
	store.Now = func() time.Time { return now }

	if err := store.SaveDetail("tenant/default", "doc", DetailSnapshot{
		SavedAt: now,
		MCPID:   9629,
		Payload: []byte(`{"success":true}`),
	}); err != nil {
		t.Fatalf("SaveDetail() error = %v", err)
	}

	_, freshness, err := store.LoadDetail("tenant/default", "doc")
	if err != nil {
		t.Fatalf("LoadDetail() error = %v", err)
	}
	if freshness != FreshnessFresh {
		t.Fatalf("LoadDetail() freshness = %s, want %s", freshness, FreshnessFresh)
	}

	store.Now = func() time.Time { return now.Add(DetailTTL + time.Hour) }
	_, freshness, err = store.LoadDetail("tenant/default", "doc")
	if err != nil {
		t.Fatalf("LoadDetail() stale error = %v", err)
	}
	if freshness != FreshnessStale {
		t.Fatalf("LoadDetail() stale freshness = %s, want %s", freshness, FreshnessStale)
	}
}

func TestHasActionVersionChanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cached map[string]string
		detail []market.DetailTool
		want   bool
	}{
		{
			name:   "nil cached returns false",
			cached: nil,
			detail: []market.DetailTool{{ToolName: "foo", ActionVersion: "v2"}},
			want:   false,
		},
		{
			name:   "empty cached returns false",
			cached: map[string]string{},
			detail: []market.DetailTool{{ToolName: "foo", ActionVersion: "v2"}},
			want:   false,
		},
		{
			name:   "versions match returns false",
			cached: map[string]string{"foo": "v1"},
			detail: []market.DetailTool{{ToolName: "foo", ActionVersion: "v1"}},
			want:   false,
		},
		{
			name:   "version changed returns true",
			cached: map[string]string{"foo": "v1"},
			detail: []market.DetailTool{{ToolName: "foo", ActionVersion: "v2"}},
			want:   true,
		},
		{
			name:   "new tool not in cache returns false",
			cached: map[string]string{"foo": "v1"},
			detail: []market.DetailTool{
				{ToolName: "foo", ActionVersion: "v1"},
				{ToolName: "bar", ActionVersion: "v1"},
			},
			want: false,
		},
		{
			name:   "detail with empty version skipped",
			cached: map[string]string{"foo": "v1"},
			detail: []market.DetailTool{{ToolName: "foo", ActionVersion: ""}},
			want:   false,
		},
		{
			name:   "detail with empty name skipped",
			cached: map[string]string{"foo": "v1"},
			detail: []market.DetailTool{{ToolName: "", ActionVersion: "v2"}},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := HasActionVersionChanged(tt.cached, tt.detail)
			if got != tt.want {
				t.Fatalf("HasActionVersionChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractActionVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tools  []market.DetailTool
		expect map[string]string
	}{
		{
			name:   "nil tools returns nil",
			tools:  nil,
			expect: nil,
		},
		{
			name:   "empty tools returns nil",
			tools:  []market.DetailTool{},
			expect: nil,
		},
		{
			name: "extracts versions",
			tools: []market.DetailTool{
				{ToolName: "create_doc", ActionVersion: "G-ACT-100"},
				{ToolName: "search_doc", ActionVersion: "G-ACT-101"},
			},
			expect: map[string]string{
				"create_doc": "G-ACT-100",
				"search_doc": "G-ACT-101",
			},
		},
		{
			name: "skips empty version",
			tools: []market.DetailTool{
				{ToolName: "create_doc", ActionVersion: "G-ACT-100"},
				{ToolName: "legacy_tool", ActionVersion: ""},
			},
			expect: map[string]string{
				"create_doc": "G-ACT-100",
			},
		},
		{
			name: "all empty returns nil",
			tools: []market.DetailTool{
				{ToolName: "", ActionVersion: "G-ACT-100"},
				{ToolName: "tool", ActionVersion: ""},
			},
			expect: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractActionVersions(tt.tools)
			if len(got) != len(tt.expect) {
				t.Fatalf("ExtractActionVersions() len = %d, want %d", len(got), len(tt.expect))
			}
			for k, v := range tt.expect {
				if got[k] != v {
					t.Fatalf("ExtractActionVersions()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestToolsSnapshotActionVersionsRoundTrip(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	versions := map[string]string{
		"create_doc": "G-ACT-100",
		"search_doc": "G-ACT-101",
	}
	err := store.SaveTools("tenant/default", "doc", ToolsSnapshot{
		ServerKey:       "doc",
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{
			{Name: "create_doc", Title: "创建文档"},
		},
		ActionVersions: versions,
	})
	if err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	snapshot, _, err := store.LoadTools("tenant/default", "doc")
	if err != nil {
		t.Fatalf("LoadTools() error = %v", err)
	}
	if len(snapshot.ActionVersions) != 2 {
		t.Fatalf("ActionVersions len = %d, want 2", len(snapshot.ActionVersions))
	}
	if snapshot.ActionVersions["create_doc"] != "G-ACT-100" {
		t.Fatalf("ActionVersions[create_doc] = %q, want G-ACT-100", snapshot.ActionVersions["create_doc"])
	}
}

func TestDeleteTools(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	partition := "tenant/default"

	err := store.SaveTools(partition, "doc", ToolsSnapshot{
		ServerKey: "doc",
		Tools:     []transport.ToolDescriptor{{Name: "create_doc"}},
	})
	if err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	if err := store.DeleteTools(partition, "doc"); err != nil {
		t.Fatalf("DeleteTools() error = %v", err)
	}

	_, _, err = store.LoadTools(partition, "doc")
	if err == nil {
		t.Fatal("LoadTools() should fail after DeleteTools()")
	}
}

func TestDeleteToolsNonExistent(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	if err := store.DeleteTools("tenant/default", "nonexistent"); err != nil {
		t.Fatalf("DeleteTools(nonexistent) should not error, got %v", err)
	}
}

func TestListToolsCacheEntries(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir())
	store.Now = func() time.Time { return now }
	partition := "tenant/default"

	// Save two server tools snapshots
	_ = store.SaveTools(partition, "doc", ToolsSnapshot{
		ServerKey:       "doc",
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{
			{Name: "create_doc"},
			{Name: "search_doc"},
		},
	})
	_ = store.SaveTools(partition, "calendar", ToolsSnapshot{
		ServerKey:       "calendar",
		ProtocolVersion: "2025-03-26",
		Tools: []transport.ToolDescriptor{
			{Name: "list_events"},
		},
	})

	entries, err := store.ListToolsCacheEntries(partition)
	if err != nil {
		t.Fatalf("ListToolsCacheEntries() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ListToolsCacheEntries() len = %d, want 2", len(entries))
	}

	byKey := make(map[string]ToolsCacheEntrySummary, len(entries))
	for _, e := range entries {
		byKey[e.ServerKey] = e
	}

	doc, ok := byKey["doc"]
	if !ok {
		t.Fatal("missing 'doc' in ListToolsCacheEntries()")
	}
	if doc.Freshness != FreshnessFresh {
		t.Fatalf("doc freshness = %s, want %s", doc.Freshness, FreshnessFresh)
	}
	if doc.ToolCount != 2 {
		t.Fatalf("doc tool_count = %d, want 2", doc.ToolCount)
	}

	cal, ok := byKey["calendar"]
	if !ok {
		t.Fatal("missing 'calendar' in ListToolsCacheEntries()")
	}
	if cal.ToolCount != 1 {
		t.Fatalf("calendar tool_count = %d, want 1", cal.ToolCount)
	}
}

func TestListToolsCacheEntriesEmpty(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	entries, err := store.ListToolsCacheEntries("nonexistent/partition")
	if err != nil {
		t.Fatalf("ListToolsCacheEntries() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("ListToolsCacheEntries() len = %d, want 0", len(entries))
	}
}
