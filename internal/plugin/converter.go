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
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
)

const maxPluginCLIOverlayBytes = 4 << 20

// UserContext holds the minimal user identity fields injected into
// stdio plugin subprocesses via environment variables.
type UserContext struct {
	UserID string
	CorpID string
}

// StdioServerClient pairs a transport.StdioClient with its server key.
type StdioServerClient struct {
	Key    string
	Client *transport.StdioClient
}

// StdioClients returns StdioClient instances for all stdio-type MCP
// servers declared by this plugin. uc is the current user's identity;
// if non-nil, DWS_USER_ID and DWS_CORP_ID are injected as environment
// variables so that the subprocess can identify the caller without
// implementing its own auth.
func (p *Plugin) StdioClients(uc *UserContext) []StdioServerClient {
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

		// Inject user identity so the subprocess knows who is calling.
		if uc != nil {
			if uc.UserID != "" {
				env["DWS_USER_ID"] = uc.UserID
			}
			if uc.CorpID != "" {
				env["DWS_CORP_ID"] = uc.CorpID
			}
		}

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
// mcptypes.ServerDescriptor values suitable for SetDynamicServers.
// Only streamable-http servers are converted; stdio servers are
// skipped (they require the stdio transport extension).
func (p *Plugin) ToServerDescriptors() []mcptypes.ServerDescriptor {
	var descriptors []mcptypes.ServerDescriptor
	for key, srv := range p.Manifest.MCPServers {
		if srv.Type != "streamable-http" {
			slog.Debug("plugin: skipping non-http server",
				"plugin", p.Manifest.Name,
				"server", key,
				"type", srv.Type,
			)
			continue
		}

		overlay, ok := p.ResolveCLIOverlay(key)
		if !ok {
			continue
		}

		source := "plugin"

		// Resolve headers: expand environment variable references (e.g. ${DASHSCOPE_API_KEY}).
		var resolvedHeaders map[string]string
		if len(srv.Headers) > 0 {
			resolvedHeaders = make(map[string]string, len(srv.Headers))
			for headerKey, headerVal := range srv.Headers {
				resolvedHeaders[headerKey] = expandPluginVars(headerVal, p.Root)
			}
		}

		descriptors = append(descriptors, mcptypes.ServerDescriptor{
			Key:         key,
			DisplayName: p.Manifest.Name + "/" + key,
			Description: p.Manifest.Description,
			Endpoint:    srv.Endpoint,
			Source:      source,
			CLI:         overlay,
			HasCLIMeta:  len(srv.CLI) > 0,
			AuthHeaders: resolvedHeaders,
		})
	}
	return descriptors
}

// ResolveCLIOverlay resolves inline or external manifest CLI metadata exactly
// once. External files are opened relative to the plugin root with os.Root so
// absolute paths, parent traversal, and escaping symlinks fail closed.
func (p *Plugin) ResolveCLIOverlay(serverKey string) (mcptypes.CLIOverlay, bool) {
	overlay := mcptypes.CLIOverlay{
		ID:      serverKey,
		Command: serverKey,
	}
	server, ok := p.Manifest.MCPServers[serverKey]
	if !ok || len(server.CLI) == 0 {
		return overlay, true
	}

	data := []byte(strings.TrimSpace(string(server.CLI)))
	if len(data) == 0 {
		return overlay, true
	}
	if data[0] == '"' {
		var relativePath string
		if err := json.Unmarshal(data, &relativePath); err != nil ||
			strings.TrimSpace(relativePath) == "" {
			slog.Warn("plugin: invalid external CLI overlay path",
				"plugin", p.Manifest.Name, "server", serverKey, "error", err)
			return mcptypes.CLIOverlay{}, false
		}
		root, err := os.OpenRoot(p.Root)
		if err != nil {
			slog.Warn("plugin: failed to open plugin root",
				"plugin", p.Manifest.Name, "server", serverKey, "error", err)
			return mcptypes.CLIOverlay{}, false
		}
		defer root.Close()
		file, err := root.Open(relativePath)
		if err != nil {
			slog.Warn("plugin: failed to open CLI overlay file",
				"plugin", p.Manifest.Name, "server", serverKey,
				"path", relativePath, "error", err)
			return mcptypes.CLIOverlay{}, false
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, maxPluginCLIOverlayBytes+1))
		if err != nil || len(data) > maxPluginCLIOverlayBytes {
			slog.Warn("plugin: failed to read CLI overlay file",
				"plugin", p.Manifest.Name, "server", serverKey,
				"path", relativePath, "error", err)
			return mcptypes.CLIOverlay{}, false
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&overlay); err != nil {
		slog.Warn("plugin: failed to parse CLI overlay",
			"plugin", p.Manifest.Name, "server", serverKey, "error", err)
		return mcptypes.CLIOverlay{}, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		slog.Warn("plugin: CLI overlay contains trailing JSON",
			"plugin", p.Manifest.Name, "server", serverKey, "error", err)
		return mcptypes.CLIOverlay{}, false
	}
	if strings.TrimSpace(overlay.ID) == "" {
		overlay.ID = serverKey
	}
	if strings.TrimSpace(overlay.Command) == "" {
		overlay.Command = serverKey
	}
	return overlay, true
}
