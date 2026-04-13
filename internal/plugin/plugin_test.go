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
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "plugin.json")
	content := `{
		"name": "conference",
		"version": "1.0.0",
		"description": "音视频会议",
		"type": "managed",
		"minCLIVersion": "0.9.0",
		"mcpServers": {
			"conference": {
				"type": "streamable-http",
				"endpoint": "https://mcp.conference.dingtalk.com"
			},
			"conference-local": {
				"type": "stdio",
				"command": "${DWS_PLUGIN_ROOT}/bin/conference-local",
				"args": ["--mode", "cli"]
			}
		},
		"skills": "./skills/"
	}`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := ParseManifest(manifestPath)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}

	if m.Name != "conference" {
		t.Errorf("name = %q, want conference", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", m.Version)
	}
	if m.Type != "managed" {
		t.Errorf("type = %q, want managed", m.Type)
	}
	if len(m.MCPServers) != 2 {
		t.Errorf("mcpServers count = %d, want 2", len(m.MCPServers))
	}
	if m.MCPServers["conference"].Type != "streamable-http" {
		t.Errorf("conference server type = %q, want streamable-http", m.MCPServers["conference"].Type)
	}
	if m.MCPServers["conference-local"].Type != "stdio" {
		t.Errorf("conference-local server type = %q, want stdio", m.MCPServers["conference-local"].Type)
	}
}

func TestManifestValidate(t *testing.T) {
	tests := []struct {
		name       string
		manifest   Manifest
		cliVersion string
		wantErr    bool
	}{
		{
			name: "valid manifest",
			manifest: Manifest{
				Name:    "conference",
				Version: "1.0.0",
				Type:    "managed",
				MCPServers: map[string]*MCPServer{
					"conference": {Type: "streamable-http", Endpoint: "https://example.com"},
				},
			},
			cliVersion: "1.0.0",
			wantErr:    false,
		},
		{
			name: "invalid name - too short",
			manifest: Manifest{
				Name:    "ab",
				Version: "1.0.0",
			},
			wantErr: true,
		},
		{
			name: "invalid name - uppercase",
			manifest: Manifest{
				Name:    "MyPlugin",
				Version: "1.0.0",
			},
			wantErr: true,
		},
		{
			name: "invalid version",
			manifest: Manifest{
				Name:    "my-plugin",
				Version: "not-semver",
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			manifest: Manifest{
				Name:    "my-plugin",
				Version: "1.0.0",
				Type:    "invalid",
			},
			wantErr: true,
		},
		{
			name: "cli version too low",
			manifest: Manifest{
				Name:          "my-plugin",
				Version:       "1.0.0",
				MinCLIVersion: "2.0.0",
			},
			cliVersion: "1.0.0",
			wantErr:    true,
		},
		{
			name: "streamable-http without endpoint",
			manifest: Manifest{
				Name:    "my-plugin",
				Version: "1.0.0",
				MCPServers: map[string]*MCPServer{
					"srv": {Type: "streamable-http"},
				},
			},
			wantErr: true,
		},
		{
			name: "stdio without command",
			manifest: Manifest{
				Name:    "my-plugin",
				Version: "1.0.0",
				MCPServers: map[string]*MCPServer{
					"srv": {Type: "stdio"},
				},
			},
			wantErr: true,
		},
		{
			name: "unsafe skills path",
			manifest: Manifest{
				Name:    "my-plugin",
				Version: "1.0.0",
				Skills:  "../../../etc/passwd",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate(tt.cliVersion)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPluginToServerDescriptors(t *testing.T) {
	cliOverlay, _ := json.Marshal(map[string]any{
		"id":      "conference",
		"command": "conference",
	})

	p := &Plugin{
		Manifest: Manifest{
			Name:        "conference",
			Description: "音视频会议",
			MCPServers: map[string]*MCPServer{
				"conference": {
					Type:     "streamable-http",
					Endpoint: "https://mcp.conference.dingtalk.com",
					CLI:      cliOverlay,
				},
				"conference-local": {
					Type:    "stdio",
					Command: "/usr/local/bin/conference-local",
				},
			},
		},
		Root:      "/tmp/plugins/conference",
		IsManaged: true,
	}

	descriptors := p.ToServerDescriptors()

	// Only streamable-http should be converted
	if len(descriptors) != 1 {
		t.Fatalf("got %d descriptors, want 1 (stdio should be skipped)", len(descriptors))
	}

	d := descriptors[0]
	if d.Key != "conference" {
		t.Errorf("key = %q, want conference", d.Key)
	}
	if d.Endpoint != "https://mcp.conference.dingtalk.com" {
		t.Errorf("endpoint = %q", d.Endpoint)
	}
	if d.Source != "plugin-managed" {
		t.Errorf("source = %q, want plugin-managed", d.Source)
	}
	if d.CLI.ID != "conference" {
		t.Errorf("cli.id = %q, want conference", d.CLI.ID)
	}
}

func TestLoaderScanEmpty(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{
		PluginsDir: dir,
		CLIVersion: "1.0.0",
	}

	managed := loader.LoadManaged()
	if len(managed) != 0 {
		t.Errorf("expected 0 managed plugins, got %d", len(managed))
	}

	user := loader.LoadUser()
	if len(user) != 0 {
		t.Errorf("expected 0 user plugins, got %d", len(user))
	}
}

func TestLoaderLoadManaged(t *testing.T) {
	dir := t.TempDir()
	managedDir := filepath.Join(dir, "managed", "conference")
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := `{
		"name": "conference",
		"version": "1.0.0",
		"type": "managed",
		"mcpServers": {
			"conference": {
				"type": "streamable-http",
				"endpoint": "https://example.com"
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(managedDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}
	plugins := loader.LoadManaged()

	if len(plugins) != 1 {
		t.Fatalf("expected 1 managed plugin, got %d", len(plugins))
	}
	if plugins[0].Manifest.Name != "conference" {
		t.Errorf("name = %q, want conference", plugins[0].Manifest.Name)
	}
	if !plugins[0].IsManaged {
		t.Error("expected IsManaged = true")
	}
}

func TestRemoveManagedPluginBlocked(t *testing.T) {
	dir := t.TempDir()
	managedDir := filepath.Join(dir, "managed", "conference")
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managedDir, "plugin.json"), []byte(`{"name":"conference","version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}
	err := loader.RemovePlugin("conference", false)
	if err == nil {
		t.Fatal("expected error when removing managed plugin")
	}
	if !contains(err.Error(), "官方插件") {
		t.Errorf("error message should mention 官方插件, got: %v", err)
	}
}

func TestIsPluginEnabled(t *testing.T) {
	s := &Settings{
		EnabledPlugins: map[string]bool{
			"my-plugin":  true,
			"disabled":   false,
		},
	}

	if !isPluginEnabled(s, "my-plugin") {
		t.Error("my-plugin should be enabled")
	}
	if isPluginEnabled(s, "disabled") {
		t.Error("disabled should not be enabled")
	}
	if !isPluginEnabled(s, "not-in-list") {
		t.Error("unlisted plugin should default to enabled")
	}
	if !isPluginEnabled(nil, "anything") {
		t.Error("nil settings should default to enabled")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
