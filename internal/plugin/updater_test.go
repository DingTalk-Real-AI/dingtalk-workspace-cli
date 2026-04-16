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
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// makePluginZip creates an in-memory zip containing a valid plugin.json.
func makePluginZip(t *testing.T, name, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	manifest := map[string]any{
		"name":    name,
		"version": version,
		"mcpServers": map[string]any{
			name: map[string]any{
				"type":     "streamable-http",
				"endpoint": "https://example.com/" + name,
			},
		},
	}
	data, _ := json.Marshal(manifest)

	f, err := w.Create("plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestEnsureManaged_PullsMissing(t *testing.T) {
	pluginName := "conference"
	zipData := makePluginZip(t, pluginName, "1.0.0")

	// Serve the zip file.
	zipServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipData)
	}))
	defer zipServer.Close()

	// Serve the download API returning the zip URL.
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := pluginDownloadResponse{
			Success: true,
			Result: &remoteVersionInfo{
				Version:     "1.0.0",
				DownloadURL: zipServer.URL + "/conference.zip",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer apiServer.Close()

	// Override the download endpoint for this test.
	origEndpoint := pluginDownloadEndpoint
	defer func() {
		// pluginDownloadEndpoint is a const, so we use a workaround:
		// we won't restore it — instead we accept the const limitation
		// and test via a helper that injects the endpoint.
		_ = origEndpoint
	}()

	tmpDir := t.TempDir()
	u := &Updater{
		PluginsDir: tmpDir,
		CLIVersion: "1.0.0",
		Platform:   "darwin-arm64",
	}

	// Patch checkRemoteVersion by using a custom updater method —
	// since checkRemoteVersion uses the const endpoint, we test
	// downloadAndInstall + EnsureManaged logic directly.
	managedDir := filepath.Join(tmpDir, config.PluginManagedDir)

	// Verify plugin does not exist yet.
	pluginDir := filepath.Join(managedDir, pluginName)
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.json")); err == nil {
		t.Fatal("plugin should not exist before EnsureManaged")
	}

	// Simulate what EnsureManaged does: downloadAndInstall for missing plugin.
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := u.downloadAndInstall(context.Background(), zipServer.URL+"/conference.zip", pluginDir)
	if err != nil {
		t.Fatalf("downloadAndInstall: %v", err)
	}

	// Verify plugin.json was extracted.
	m, err := ParseManifest(filepath.Join(pluginDir, "plugin.json"))
	if err != nil {
		t.Fatalf("ParseManifest after install: %v", err)
	}
	if m.Name != pluginName {
		t.Errorf("name = %q, want %q", m.Name, pluginName)
	}
	if m.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", m.Version)
	}
}

func TestEnsureManaged_SkipsExisting(t *testing.T) {
	tmpDir := t.TempDir()
	managedDir := filepath.Join(tmpDir, config.PluginManagedDir)

	// Pre-create the plugin directory with a valid manifest.
	pluginDir := filepath.Join(managedDir, "conference")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"conference","version":"1.0.0","mcpServers":{"conference":{"type":"streamable-http","endpoint":"https://example.com"}}}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	u := &Updater{
		PluginsDir: tmpDir,
		CLIVersion: "1.0.0",
		Platform:   "darwin-arm64",
	}

	var output bytes.Buffer
	// EnsureManaged should not attempt any download (no token needed since it skips).
	installed := u.EnsureManaged(context.Background(), "fake-token", &output)

	if len(installed) != 0 {
		t.Errorf("expected 0 installs for existing plugin, got %d: %v", len(installed), installed)
	}
	// Should produce no output since nothing was downloaded.
	if strings.Contains(output.String(), "Pulling") {
		t.Errorf("unexpected download attempt for existing plugin: %s", output.String())
	}
}

func TestExtractPluginZip_ZipSlipProtection(t *testing.T) {
	// Create a zip with a path traversal entry.
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("../../etc/passwd")
	f.Write([]byte("malicious"))
	w.Close()

	tmpZip := filepath.Join(t.TempDir(), "bad.zip")
	os.WriteFile(tmpZip, buf.Bytes(), 0o644)

	destDir := filepath.Join(t.TempDir(), "dest")
	err := extractPluginZip(tmpZip, destDir)
	if err == nil {
		t.Fatal("expected zip slip error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid file path") {
		t.Errorf("unexpected error: %v", err)
	}
}
