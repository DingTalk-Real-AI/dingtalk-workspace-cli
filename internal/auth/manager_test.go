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

package auth

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestManagerTokenPriorityAndStatus(t *testing.T) {
	configDir := t.TempDir()
	manager := NewManager(configDir, testLogger())

	if _, _, err := manager.GetToken(); err == nil {
		t.Fatal("GetToken() error = nil, want failure with no auth configured")
	}

	if err := manager.SaveToken("file-token"); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}
	token, source, err := manager.GetToken()
	if err != nil {
		t.Fatalf("GetToken(file) error = %v", err)
	}
	if token != "file-token" || source != "file" {
		t.Fatalf("GetToken(file) = (%q, %q), want (file-token, file)", token, source)
	}

	authenticated, statusSource, masked := manager.Status()
	if !authenticated || statusSource != "file" {
		t.Fatalf("Status() = (%t, %q, %q), want authenticated file source", authenticated, statusSource, masked)
	}
	if masked == "file-token" || masked == "" {
		t.Fatalf("Status() masked token = %q, want redacted token", masked)
	}
}

func TestManagerSaveAndDeleteFiles(t *testing.T) {
	configDir := t.TempDir()
	manager := NewManager(configDir, testLogger())

	if err := manager.SaveMCPURL("https://example.com/server/doc"); err != nil {
		t.Fatalf("SaveMCPURL() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "mcp_url")); err != nil {
		t.Fatalf("Stat(mcp_url) error = %v", err)
	}
	mcpURL, err := manager.GetMCPURL()
	if err != nil {
		t.Fatalf("GetMCPURL() error = %v", err)
	}
	if mcpURL != "https://example.com/server/doc" {
		t.Fatalf("GetMCPURL() = %q, want saved URL", mcpURL)
	}

	if err := manager.SaveToken("temp-token"); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}
	if err := manager.DeleteToken(); err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}
	if manager.IsAuthenticated() {
		t.Fatal("IsAuthenticated() = true after delete, want false")
	}
}
