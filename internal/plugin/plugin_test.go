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
		Root: "/tmp/plugins/conference",
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
	if d.Source != "plugin" {
		t.Errorf("source = %q, want plugin", d.Source)
	}
	if d.CLI.ID != "conference" {
		t.Errorf("cli.id = %q, want conference", d.CLI.ID)
	}
}

func TestPluginToServerDescriptorsWithHeaders(t *testing.T) {
	cliOverlay, _ := json.Marshal(map[string]any{
		"id":      "web-search",
		"command": "web-search",
	})

	// Set an environment variable to test expansion
	t.Setenv("TEST_API_KEY", "sk-test-12345")

	p := &Plugin{
		Manifest: Manifest{
			Name:        "my-plugin",
			Description: "Test plugin with headers",
			MCPServers: map[string]*MCPServer{
				"web-search": {
					Type:     "streamable-http",
					Endpoint: "https://api.example.com/mcp/v1",
					CLI:      cliOverlay,
					Headers: map[string]string{
						"Authorization": "Bearer ${TEST_API_KEY}",
						"X-Custom":      "static-value",
					},
				},
			},
		},
		Root: "/tmp/plugins/my-plugin",
	}

	descriptors := p.ToServerDescriptors()
	if len(descriptors) != 1 {
		t.Fatalf("got %d descriptors, want 1", len(descriptors))
	}

	d := descriptors[0]
	if d.Key != "web-search" {
		t.Errorf("key = %q, want web-search", d.Key)
	}
	if len(d.AuthHeaders) != 2 {
		t.Fatalf("AuthHeaders len = %d, want 2", len(d.AuthHeaders))
	}
	// Environment variable should be expanded
	if d.AuthHeaders["Authorization"] != "Bearer sk-test-12345" {
		t.Errorf("AuthHeaders[Authorization] = %q, want 'Bearer sk-test-12345'", d.AuthHeaders["Authorization"])
	}
	if d.AuthHeaders["X-Custom"] != "static-value" {
		t.Errorf("AuthHeaders[X-Custom] = %q, want static-value", d.AuthHeaders["X-Custom"])
	}
	if d.Source != "plugin" {
		t.Errorf("source = %q, want plugin", d.Source)
	}
}

func TestPluginToServerDescriptorsNoHeaders(t *testing.T) {
	cliOverlay, _ := json.Marshal(map[string]any{
		"id":      "conference",
		"command": "conference",
	})

	p := &Plugin{
		Manifest: Manifest{
			Name: "conference",
			MCPServers: map[string]*MCPServer{
				"conference": {
					Type:     "streamable-http",
					Endpoint: "https://mcp.conference.dingtalk.com",
					CLI:      cliOverlay,
				},
			},
		},
		Root: "/tmp/plugins/conference",
	}

	descriptors := p.ToServerDescriptors()
	if len(descriptors) != 1 {
		t.Fatalf("got %d descriptors, want 1", len(descriptors))
	}
	if descriptors[0].AuthHeaders != nil {
		t.Errorf("AuthHeaders = %v, want nil for server without headers", descriptors[0].AuthHeaders)
	}
}

func TestParseManifestWithHeaders(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "plugin.json")
	content := `{
		"name": "api-plugin",
		"version": "1.0.0",
		"mcpServers": {
			"api-server": {
				"type": "streamable-http",
				"endpoint": "https://api.example.com/mcp",
				"headers": {
					"Authorization": "Bearer ${MY_API_KEY}",
					"X-Custom-Header": "custom-value"
				}
			}
		}
	}`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := ParseManifest(manifestPath)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}

	srv := m.MCPServers["api-server"]
	if srv == nil {
		t.Fatal("api-server not found in MCPServers")
	}
	if len(srv.Headers) != 2 {
		t.Fatalf("Headers len = %d, want 2", len(srv.Headers))
	}
	if srv.Headers["Authorization"] != "Bearer ${MY_API_KEY}" {
		t.Errorf("Headers[Authorization] = %q, want raw template", srv.Headers["Authorization"])
	}
	if srv.Headers["X-Custom-Header"] != "custom-value" {
		t.Errorf("Headers[X-Custom-Header] = %q, want custom-value", srv.Headers["X-Custom-Header"])
	}
}

func TestLoaderScanEmpty(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{
		PluginsDir: dir,
		CLIVersion: "1.0.0",
	}

	user := loader.LoadUser()
	if len(user) != 0 {
		t.Errorf("expected 0 user plugins, got %d", len(user))
	}
}

// TestRemovePluginPurgesSettings verifies RemovePlugin fully purges the
// plugin's settings — both its enabled flag and any pluginConfigs entry —
// so settings.json does not retain dangling state for a plugin that no
// longer exists on disk.
func TestRemovePluginPurgesSettings(t *testing.T) {
	const pkgName = "my-plugin"
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "user", pkgName)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"),
		[]byte(`{"name":"`+pkgName+`","version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	// Seed settings.json with an explicit enabled flag and a
	// pluginConfigs entry to verify both get purged.
	settings := &Settings{
		EnabledPlugins: map[string]bool{pkgName: true, "other-plugin": true},
		PluginConfigs: map[string]map[string]any{
			pkgName:        {"API_KEY": "secret"},
			"other-plugin": {"TOKEN": "keep-me"},
		},
	}
	loader.saveSettings(settings)

	if err := loader.RemovePlugin(pkgName, false); err != nil {
		t.Fatalf("RemovePlugin: %v", err)
	}

	reloaded := loader.loadSettings()
	if _, exists := reloaded.EnabledPlugins[pkgName]; exists {
		t.Errorf("EnabledPlugins should not retain removed plugin %q", pkgName)
	}
	if _, exists := reloaded.PluginConfigs[pkgName]; exists {
		t.Errorf("PluginConfigs should not retain removed plugin %q", pkgName)
	}
	if !reloaded.EnabledPlugins["other-plugin"] {
		t.Error("unrelated EnabledPlugins entry should be preserved")
	}
	if reloaded.PluginConfigs["other-plugin"]["TOKEN"] != "keep-me" {
		t.Error("unrelated PluginConfigs entry should be preserved")
	}
}

func TestIsPluginEnabled(t *testing.T) {
	s := &Settings{
		EnabledPlugins: map[string]bool{
			"my-plugin": true,
			"disabled":  false,
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

func TestParseGitURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantWS   string
		wantRepo string
		wantErr  bool
	}{
		{
			name:     "https with .git",
			url:      "https://github.com/PeterGuy326/hello-plugin.git",
			wantWS:   "PeterGuy326",
			wantRepo: "hello-plugin",
		},
		{
			name:     "https without .git",
			url:      "https://github.com/DingTalk-Real-AI/conference",
			wantWS:   "DingTalk-Real-AI",
			wantRepo: "conference",
		},
		{
			name:     "ssh format",
			url:      "git@github.com:DingTalk-Real-AI/conference.git",
			wantWS:   "DingTalk-Real-AI",
			wantRepo: "conference",
		},
		{
			name:    "invalid - no repo",
			url:     "https://github.com/onlyone",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws, repo, err := parseGitURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGitURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if ws != tt.wantWS {
					t.Errorf("workspace = %q, want %q", ws, tt.wantWS)
				}
				if repo != tt.wantRepo {
					t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
				}
			}
		})
	}
}

func TestDevPluginRegistration(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	// Create a dev plugin directory
	devDir := filepath.Join(t.TempDir(), "my-dev-plugin")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"my-dev-plugin","version":"0.1.0","type":"user"}`
	if err := os.WriteFile(filepath.Join(devDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// Register dev plugin
	if err := loader.RegisterDevPlugin("my-dev-plugin", devDir); err != nil {
		t.Fatalf("RegisterDevPlugin: %v", err)
	}

	// Load dev plugins
	plugins := loader.LoadDev()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 dev plugin, got %d", len(plugins))
	}
	if plugins[0].Manifest.Name != "my-dev-plugin" {
		t.Errorf("name = %q, want my-dev-plugin", plugins[0].Manifest.Name)
	}
	if plugins[0].Root != devDir {
		t.Errorf("root = %q, want %q (should load from source dir, not copy)", plugins[0].Root, devDir)
	}

	// Unregister
	if err := loader.UnregisterDevPlugin("my-dev-plugin"); err != nil {
		t.Fatalf("UnregisterDevPlugin: %v", err)
	}

	// Should be empty now
	plugins = loader.LoadDev()
	if len(plugins) != 0 {
		t.Errorf("expected 0 dev plugins after unregister, got %d", len(plugins))
	}
}

func TestUnregisterDevPluginNotFound(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	err := loader.UnregisterDevPlugin("nonexistent")
	if err == nil {
		t.Error("expected error when unregistering nonexistent dev plugin")
	}
}

func TestSyncSkills(t *testing.T) {
	// Create a plugin with skills
	pluginDir := t.TempDir()
	skillsDir := filepath.Join(pluginDir, "skills", "test-plugin")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillContent := "# Test Plugin Skill"
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &Plugin{
		Manifest: Manifest{
			Name:   "test-plugin",
			Skills: "./skills/test-plugin",
		},
		Root: pluginDir,
	}

	// Create a mock agent directory
	home, _ := os.UserHomeDir()
	agentDir := filepath.Join(home, ".agents", "skills")
	// Only run if .agents exists (don't create in CI)
	if _, err := os.Stat(filepath.Dir(agentDir)); err == nil {
		SyncSkills([]*Plugin{p})

		synced := filepath.Join(agentDir, "dws", "plugins", "test-plugin", "SKILL.md")
		if _, err := os.Stat(synced); err == nil {
			data, _ := os.ReadFile(synced)
			if string(data) != skillContent {
				t.Errorf("synced content = %q, want %q", string(data), skillContent)
			}
			// Cleanup
			os.RemoveAll(filepath.Join(agentDir, "dws", "plugins", "test-plugin"))
		}
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
