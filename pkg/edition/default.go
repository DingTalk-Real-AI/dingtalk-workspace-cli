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

package edition

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/syncdata"

// DefaultOSSClawType is the wire value for request header claw-type in
// the open-source build. It is intentionally hard-wired — the open-source
// CLI does NOT derive claw-type from DINGTALK_AGENT or any other caller
// input, so third-party hosts get a predictable header regardless of
// their environment.
const DefaultOSSClawType = "openClaw"

// defaultHooks returns the open-source edition defaults.
//
// MergeHeaders is the only hook that ships with behaviour: it pins the
// `claw-type` request header to DefaultOSSClawType so every open-source
// MCP request carries the same stable routing tag. All other fields are
// nil — the internal code interprets nil as "use standard open-source
// behaviour".
func defaultHooks() *Hooks {
	return &Hooks{
		Name: "open",
		MergeHeaders: func(base map[string]string) map[string]string {
			if base == nil {
				base = make(map[string]string)
			}
			base["claw-type"] = DefaultOSSClawType
			return base
		},
		StaticServers:     openStaticServers,
		SupplementServers: openSupplementServers,
		VisibleProducts:   openVisibleProducts,
	}
}

// openSupplementServers returns helper-only MCP endpoints owned by the open
// CLI. They are callable by explicit server ID but are deliberately excluded
// from VisibleProducts, so no top-level product command is generated. They
// stay separate from syncdata.StaticServers because only explicitly wired CLI
// helpers may call them; they are not public MCP product surfaces.
func openSupplementServers() []ServerInfo {
	return []ServerInfo{
		{
			ID:       "mcp-meta",
			Name:     "MCP 元服务",
			Endpoint: "https://mcp-gw.dingtalk.com/server/89833ea5debf30c260a07ffcb5127ffa3bf0c830cd76babadb293d9861485d44",
		},
	}
}

func openStaticServers() []ServerInfo {
	raw := syncdata.StaticServers()
	out := make([]ServerInfo, len(raw))
	for i, s := range raw {
		out[i] = ServerInfo{
			ID:       s.ID,
			Name:     s.Name,
			Endpoint: s.Endpoint,
			Prefixes: s.Prefixes,
		}
	}
	return out
}

func openVisibleProducts() []string {
	servers := openStaticServers()
	out := make([]string, 0, len(servers))
	seen := make(map[string]bool, len(servers))
	for _, server := range servers {
		if server.ID == "" || seen[server.ID] {
			continue
		}
		seen[server.ID] = true
		out = append(out, server.ID)
	}
	return out
}
