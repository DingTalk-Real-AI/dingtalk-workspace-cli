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

// Package editionmerge converts edition.ServerInfo hooks into
// market.ServerDescriptor values and merges them into discovery results.
//
// This package exists so both internal/cli (runtime catalog loader) and
// internal/app (command-tree loader) can apply the edition's
// SupplementServers / FallbackServers hooks consistently against the same
// discovery pipeline, instead of the hooks being wired only at the
// command-tree layer. Keeping the logic here avoids an import cycle
// between internal/cli ↔ internal/app.
package editionmerge

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// MergeSupplement returns servers augmented with the active edition's
// SupplementServers hook. Discovery entries always win on ID collision —
// the supplement only fills gaps that discovery did not cover.
func MergeSupplement(servers []market.ServerDescriptor) []market.ServerDescriptor {
	fn := edition.Get().SupplementServers
	if fn == nil {
		return servers
	}
	existing := make(map[string]bool, len(servers))
	for _, s := range servers {
		if id := s.CLI.ID; id != "" {
			existing[id] = true
		}
		if s.Key != "" {
			existing[s.Key] = true
		}
	}
	for _, sup := range fn() {
		if sup.ID == "" || existing[sup.ID] {
			continue
		}
		servers = append(servers, ToDescriptor(sup, "edition_supplement"))
	}
	return servers
}

// FallbackToDescriptors converts the edition's FallbackServers hook into
// market.ServerDescriptor values. Callers should only invoke this when
// live discovery returned zero servers and the cache is also empty.
func FallbackToDescriptors(servers []edition.ServerInfo) []market.ServerDescriptor {
	out := make([]market.ServerDescriptor, 0, len(servers))
	for _, s := range servers {
		out = append(out, ToDescriptor(s, "edition_fallback"))
	}
	return out
}

// ToDescriptor is the shared conversion from edition.ServerInfo to the
// market descriptor shape expected by downstream consumers.
//
// Source carries the origin tag for diagnostics / metrics. Supplement and
// fallback entries intentionally carry no ToolOverrides — that keeps
// internal/compat.BuildDynamicCommands from materialising parallel
// command trees for products already owned by hardcoded overlays (see
// internal/compat/dynamic_commands.go's CLIOverlay gate).
func ToDescriptor(s edition.ServerInfo, source string) market.ServerDescriptor {
	return market.ServerDescriptor{
		Key:         s.ID,
		DisplayName: s.Name,
		Endpoint:    s.Endpoint,
		Source:      source,
		CLI: market.CLIOverlay{
			ID:       s.ID,
			Command:  s.ID,
			Prefixes: s.Prefixes,
		},
	}
}
