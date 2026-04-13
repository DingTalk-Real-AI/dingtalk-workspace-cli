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
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// StdioServerClient pairs a transport.StdioClient with its server key.
type StdioServerClient struct {
	Key    string
	Client *transport.StdioClient
}

// StdioClients returns StdioClient instances for all stdio-type MCP
// servers declared by this plugin.
func (p *Plugin) StdioClients() []StdioServerClient {
	var clients []StdioServerClient
	for key, srv := range p.Manifest.MCPServers {
		if srv.Type != "stdio" {
			continue
		}

		command := srv.Command
		if command == "" {
			slog.Warn("plugin: stdio server missing command",
				"plugin", p.Manifest.Name, "server", key)
			continue
		}

		// Expand ${DWS_PLUGIN_ROOT} in command and args.
		command = expandPluginVars(command, p.Root)
		args := make([]string, len(srv.Args))
		for i, a := range srv.Args {
			args[i] = expandPluginVars(a, p.Root)
		}

		env := make(map[string]string)
		for k, v := range srv.Env {
			env[k] = expandPluginVars(v, p.Root)
		}
		env["DWS_PLUGIN_ROOT"] = p.Root
		env["DWS_PLUGIN_DATA"] = filepath.Join(filepath.Dir(filepath.Dir(p.Root)), "data", p.Manifest.Name)

		sc := transport.NewStdioClient(command, args, env)
		clients = append(clients, StdioServerClient{Key: key, Client: sc})
	}
	return clients
}

// expandPluginVars replaces ${DWS_PLUGIN_ROOT} with the actual plugin
// root path and ${DWS_PLUGIN_DATA} with the data directory.
func expandPluginVars(s, root string) string {
	s = strings.ReplaceAll(s, "${DWS_PLUGIN_ROOT}", root)
	dataDir := filepath.Join(filepath.Dir(filepath.Dir(root)), "data")
	s = strings.ReplaceAll(s, "${DWS_PLUGIN_DATA}", dataDir)
	return os.Expand(s, os.Getenv)
}

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
