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

package plugin

import (
	"encoding/json"
	"log/slog"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
)

// ToServerDescriptors converts a loaded plugin's MCP servers into
// market.ServerDescriptor values suitable for SetDynamicServers.
// Only streamable-http servers are converted; stdio servers are
// skipped (they require the stdio transport extension).
func (p *Plugin) ToServerDescriptors() []market.ServerDescriptor {
	var descriptors []market.ServerDescriptor
	for key, srv := range p.Manifest.MCPServers {
		if srv.Type != "streamable-http" {
			slog.Debug("plugin: skipping non-http server",
				"plugin", p.Manifest.Name,
				"server", key,
				"type", srv.Type,
			)
			continue
		}

		overlay := market.CLIOverlay{}
		if len(srv.CLI) > 0 {
			if err := json.Unmarshal(srv.CLI, &overlay); err != nil {
				slog.Warn("plugin: failed to parse CLIOverlay",
					"plugin", p.Manifest.Name,
					"server", key,
					"error", err,
				)
			}
		}

		// Ensure the overlay has an ID — fall back to server key.
		if overlay.ID == "" {
			overlay.ID = key
		}
		if overlay.Command == "" {
			overlay.Command = key
		}

		source := "plugin"
		if p.IsManaged {
			source = "plugin-managed"
		}

		descriptors = append(descriptors, market.ServerDescriptor{
			Key:         key,
			DisplayName: p.Manifest.Name + "/" + key,
			Description: p.Manifest.Description,
			Endpoint:    srv.Endpoint,
			Source:      source,
			CLI:         overlay,
			HasCLIMeta:  len(srv.CLI) > 0,
		})
	}
	return descriptors
}
