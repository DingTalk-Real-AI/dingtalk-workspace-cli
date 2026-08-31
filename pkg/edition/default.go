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

// DefaultOSSClawType is the fixed wire value for request header claw-type in
// the open-source build. It is intentionally independent of caller-provided
// environment variables so PAT and routing behaviour stays predictable.
const DefaultOSSClawType = "openClaw"

// defaultHooks returns the open-source edition defaults.
//
// MergeHeaders is the only hook that ships with behaviour: it supplies
// DefaultOSSClawType so every open-source MCP request has a stable default.
// DWS_AGENT_PRODUCT is sent through a separate observability Header and never
// changes claw-type. All other fields are nil — the internal code interprets
// nil as "use standard open-source behaviour".
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

// openSupplementServers returns explicitly wired MCP endpoints owned by the
// open CLI. They are callable by explicit server ID but are deliberately
// excluded from VisibleProducts, so this hook never generates a top-level
// product command. A public command using one of these endpoints must register
// its Cobra tree explicitly via helpers.RegisterPublic.
func openSupplementServers() []ServerInfo {
	return []ServerInfo{
		{
			ID:       "mcp-meta",
			Name:     "MCP 元服务",
			Endpoint: "https://mcp-gw.dingtalk.com/server/89833ea5debf30c260a07ffcb5127ffa3bf0c830cd76babadb293d9861485d44",
		},
		{
			ID:       "whiteboard",
			Name:     "钉钉白板",
			Endpoint: "https://mcp-gw.dingtalk.com/server/whiteboard",
		},
		{
			ID:       "recruit",
			Name:     "钉钉招聘",
			Endpoint: "https://mcp-gw.dingtalk.com/server/f69b54ada16c57b603c0e5e1c36f464ba73dcee28d64bb701ff2682c259c0cff",
			Prefixes: []string{"recruit", "job"},
		},
		{
			ID:       "contract",
			Name:     "智能合同",
			Endpoint: "https://mcp-gw.dingtalk.com/server/571e843ffa2f7546207fcae482d0d47a43bb7e5317bc717172998e1e00691710",
		},
		{
			ID:       "edu-contact",
			Name:     "家校通讯录",
			Endpoint: "https://mcp-gw.dingtalk.com/server/d24759cc1c6e424e2de4e9901ea0202136e6707991ffc33b473878ec1cd688a2",
			Prefixes: []string{"edu-contact", "edu"},
		},
		{
			ID:       "edu-group",
			Name:     "家校群",
			Endpoint: "https://mcp-gw.dingtalk.com/server/14624b71ac9bc1a03b1b60e5b0403a48b346361f86cc9f555f98f89eb383875a",
			Prefixes: []string{"edu-group"},
		},
		{
			ID:       "edu-app",
			Name:     "家校应用",
			Endpoint: "https://mcp-gw.dingtalk.com/server/905eef591d16e2a1d95b235bcc780ce2fadb6ebe1f25648a279f8a2d97907a1e",
			Prefixes: []string{"edu-app"},
		},
		{
			ID:       "edu-familygroup",
			Name:     "家庭群",
			Endpoint: "https://mcp-gw.dingtalk.com/server/1cd153fb5296df340507c3e9ee20c938f9feeefec3147e9cc32317032f1a2944",
			Prefixes: []string{"edu-familygroup"},
		},
		{
			ID:       "college-contact",
			Name:     "高校通讯录",
			Endpoint: "https://mcp-gw.dingtalk.com/server/45bb310b388b9c39e0b80e08236782880cb51ad536e1292f9a40933c428a7474",
			Prefixes: []string{"college-contact"},
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
