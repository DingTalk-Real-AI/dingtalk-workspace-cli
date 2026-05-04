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

package editionmerge

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func restoreEdition(t *testing.T) {
	t.Helper()
	prev := edition.Get()
	t.Cleanup(func() { edition.Override(prev) })
}

func TestMergeSupplement_DiscoveryWinsOnCollision(t *testing.T) {
	restoreEdition(t)
	edition.Override(&edition.Hooks{
		SupplementServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{
				{ID: "conference", Name: "会议", Endpoint: "https://hardcoded/conference"},
				{ID: "doc", Name: "文档(overridden)", Endpoint: "https://hardcoded/doc"},
			}
		},
	})

	servers := []market.ServerDescriptor{
		{
			Key:         "doc",
			DisplayName: "文档",
			Endpoint:    "https://live/doc",
			CLI:         market.CLIOverlay{ID: "doc", Command: "doc"},
		},
	}

	merged := MergeSupplement(servers)

	if len(merged) != 2 {
		t.Fatalf("merged len = %d, want 2", len(merged))
	}

	byID := make(map[string]market.ServerDescriptor, len(merged))
	for _, m := range merged {
		byID[m.CLI.ID] = m
	}
	if got := byID["doc"].Endpoint; got != "https://live/doc" {
		t.Errorf("doc endpoint = %q, want live endpoint (discovery wins)", got)
	}
	if got := byID["conference"].Endpoint; got != "https://hardcoded/conference" {
		t.Errorf("conference endpoint = %q, want supplement endpoint", got)
	}
	if got := byID["conference"].Source; got != "edition_supplement" {
		t.Errorf("conference Source = %q, want edition_supplement", got)
	}
}

func TestMergeSupplement_NilHookIsNoop(t *testing.T) {
	restoreEdition(t)
	edition.Override(&edition.Hooks{})

	servers := []market.ServerDescriptor{
		{Key: "doc", DisplayName: "文档", Endpoint: "https://live/doc",
			CLI: market.CLIOverlay{ID: "doc", Command: "doc"}},
	}

	merged := MergeSupplement(servers)

	if len(merged) != 1 {
		t.Fatalf("merged len = %d, want 1 (no supplement hook registered)", len(merged))
	}
}

func TestMergeSupplement_EmptyIDSkipped(t *testing.T) {
	restoreEdition(t)
	edition.Override(&edition.Hooks{
		SupplementServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{
				{ID: "", Name: "empty", Endpoint: "https://example.invalid/empty"},
				{ID: "valid", Name: "valid", Endpoint: "https://example.invalid/valid"},
			}
		},
	})

	merged := MergeSupplement(nil)
	if len(merged) != 1 {
		t.Fatalf("merged len = %d, want 1 (empty ID must be skipped)", len(merged))
	}
	if merged[0].CLI.ID != "valid" {
		t.Errorf("merged[0].CLI.ID = %q, want valid", merged[0].CLI.ID)
	}
}

func TestFallbackToDescriptors(t *testing.T) {
	got := FallbackToDescriptors([]edition.ServerInfo{
		{ID: "conference", Name: "会议", Endpoint: "https://example.invalid/conference", Prefixes: []string{"conference", "meeting"}},
	})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	g := got[0]
	if g.CLI.ID != "conference" || g.CLI.Command != "conference" {
		t.Errorf("CLI overlay not wired: %+v", g.CLI)
	}
	if g.Source != "edition_fallback" {
		t.Errorf("Source = %q, want edition_fallback", g.Source)
	}
	if len(g.CLI.ToolOverrides) != 0 {
		t.Errorf("fallback descriptor must not carry ToolOverrides; got %v", g.CLI.ToolOverrides)
	}
}
