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

package app

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
)

// resolveStdioOverlay resolves the CLIOverlay for a stdio plugin server
// from its manifest. It supports two forms:
//
//  1. inline JSON object in manifest.MCPServers[key].CLI
//  2. a relative file path (JSON string) pointing to an external overlay
//     file anchored at the plugin root (e.g. "overlay.json")
//
// When no CLI metadata is present, a minimal overlay keyed by the server
// name is returned so callers can still build an identity descriptor.
func resolveStdioOverlay(p *plugin.Plugin, sc plugin.StdioServerClient) mcptypes.CLIOverlay {
	serverID := sc.Key
	overlay := mcptypes.CLIOverlay{
		ID:      serverID,
		Command: serverID,
	}
	srv, ok := p.Manifest.MCPServers[sc.Key]
	if !ok || len(srv.CLI) == 0 {
		return overlay
	}

	cliData := srv.CLI
	// A JSON string is interpreted as a relative path to an external
	// overlay file (e.g. "overlay.json") anchored at the plugin root.
	if len(cliData) > 0 && cliData[0] == '"' {
		var cliPath string
		if err := json.Unmarshal(cliData, &cliPath); err == nil && cliPath != "" {
			absPath := filepath.Join(p.Root, cliPath)
			if fileData, readErr := os.ReadFile(absPath); readErr == nil {
				cliData = fileData
			} else {
				slog.Warn("plugin: failed to read CLI overlay file",
					"plugin", p.Manifest.Name, "path", absPath, "error", readErr)
			}
		}
	}
	if err := json.Unmarshal(cliData, &overlay); err != nil {
		slog.Warn("plugin: failed to parse CLI overlay for stdio server",
			"plugin", p.Manifest.Name, "server", sc.Key, "error", err)
	}
	if overlay.ID == "" {
		overlay.ID = serverID
	}
	if overlay.Command == "" {
		overlay.Command = serverID
	}
	return overlay
}

// registerStdioServerFromManifest registers an endpoint descriptor and an
// unstarted client from versioned plugin metadata. Tool discovery is not part
// of command-tree construction; execution starts and initializes the client.
func registerStdioServerFromManifest(p *plugin.Plugin, sc plugin.StdioServerClient) mcptypes.ServerDescriptor {
	overlay := resolveStdioOverlay(p, sc)
	descriptor := mcptypes.ServerDescriptor{
		Key:         sc.Key,
		DisplayName: p.Manifest.Name + "/" + sc.Key,
		Description: p.Manifest.Description,
		Endpoint:    StdioEndpoint(p.Manifest.Name, sc.Key),
		Source:      "plugin",
		CLI:         overlay,
		HasCLIMeta:  true,
	}

	AppendDynamicServer(descriptor)
	RegisterStdioClient(p.Manifest.Name+"/"+sc.Key, sc.Client)

	slog.Debug("plugin: stdio server registered from manifest",
		"plugin", p.Manifest.Name, "server", sc.Key,
		"toolOverrides", len(overlay.ToolOverrides))
	return descriptor
}
