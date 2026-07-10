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

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/agentmetadata"
)

func TestLoadCommandSurfaceSnapshotReconcilesAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema_command_surface.json")
	snapshot := commandSurfaceSnapshot{
		Version: commandSurfaceSnapshotVersion,
		Products: []commandSurfaceProduct{{
			ID: "calendar",
			Tools: []commandSurfaceTool{{
				CLIPath: "calendar attendee delete",
				Aliases: []string{"calendar participant delete"},
			}},
		}},
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	surface, err := loadCommandSurfaceSnapshot(path)
	if err != nil {
		t.Fatalf("loadCommandSurfaceSnapshot() error = %v", err)
	}
	if surface.ToolCount != 1 || !surface.ProductIDs["calendar"] {
		t.Fatalf("surface = %#v", surface)
	}
	if got := surface.ToolPaths["calendar participant delete"]; got != "calendar attendee delete" {
		t.Fatalf("alias primary path = %q, want calendar attendee delete", got)
	}
	if surface.Hash == "" {
		t.Fatalf("surface hash is empty: %#v", surface)
	}
}

func TestWriteMetadataDirectorySplitsDomains(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stale.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale) error = %v", err)
	}
	metadata := agentmetadata.File{
		Version:    1,
		SourceHash: "sha256:test",
		Products: map[string]agentmetadata.ProductMetadata{
			"calendar": {UseWhen: []string{"日程"}},
			"contact":  {UseWhen: []string{"联系人"}},
		},
		Tools: map[string]agentmetadata.ToolMetadata{
			"calendar event create": {Effect: "write"},
			"contact user get-self": {Effect: "read"},
		},
	}
	if err := writeMetadataDirectory(dir, metadata); err != nil {
		t.Fatalf("writeMetadataDirectory() error = %v", err)
	}

	indexData, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatalf("ReadFile(index) error = %v", err)
	}
	var index agentMetadataIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatalf("json.Unmarshal(index) error = %v", err)
	}
	if got := index.Domains; len(got) != 2 || got[0] != "calendar" || got[1] != "contact" {
		t.Fatalf("domains = %#v", got)
	}

	calendarData, err := os.ReadFile(filepath.Join(dir, "calendar.json"))
	if err != nil {
		t.Fatalf("ReadFile(calendar) error = %v", err)
	}
	var calendar agentMetadataDomain
	if err := json.Unmarshal(calendarData, &calendar); err != nil {
		t.Fatalf("json.Unmarshal(calendar) error = %v", err)
	}
	if calendar.ProductID != "calendar" || len(calendar.Tools) != 1 || calendar.Tools["calendar event create"].Effect != "write" {
		t.Fatalf("calendar metadata = %#v", calendar)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected stale metadata file: %v", err)
	}
}
