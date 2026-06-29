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

package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuarantinePartitionNoCacheIsNoop(t *testing.T) {
	s := NewStore(t.TempDir())
	path, err := s.QuarantinePartition("default_default")
	if err != nil {
		t.Fatalf("QuarantinePartition() error = %v", err)
	}
	if path != "" {
		t.Errorf("QuarantinePartition() = %q, want empty path when nothing is cached", path)
	}
}

func TestQuarantinePartitionMovesCacheAside(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)
	if err := s.SaveTools("default_default", "srv", ToolsSnapshot{ServerKey: "srv"}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	path, err := s.QuarantinePartition("default_default")
	if err != nil {
		t.Fatalf("QuarantinePartition() error = %v", err)
	}
	want := filepath.Join(tmp, "default_default.quarantined")
	if path != want {
		t.Errorf("QuarantinePartition() = %q, want %q", path, want)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "default_default")); !os.IsNotExist(statErr) {
		t.Errorf("original partition dir still present after quarantine (stat err = %v)", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(path, "tools", "srv.json")); statErr != nil {
		t.Errorf("quarantined snapshot missing: %v", statErr)
	}
}

func TestQuarantinePartitionReplacesPreviousQuarantine(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)
	if err := s.SaveTools("default_default", "first", ToolsSnapshot{ServerKey: "first"}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}
	if _, err := s.QuarantinePartition("default_default"); err != nil {
		t.Fatalf("first QuarantinePartition() error = %v", err)
	}
	if err := s.SaveTools("default_default", "second", ToolsSnapshot{ServerKey: "second"}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	path, err := s.QuarantinePartition("default_default")
	if err != nil {
		t.Fatalf("second QuarantinePartition() error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(path, "tools", "second.json")); statErr != nil {
		t.Errorf("latest quarantine missing newest snapshot: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(path, "tools", "first.json")); !os.IsNotExist(statErr) {
		t.Errorf("previous quarantine was not replaced (stat err = %v)", statErr)
	}
}

func TestPurgeDiscoveryDataRemovesDiscoveryDirsOnly(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)

	mustWrite := func(parts ...string) {
		t.Helper()
		path := filepath.Join(parts...)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	mustWrite(tmp, "default_default", "market", "servers.json")
	mustWrite(tmp, "default_default", "tools", "srv.json")
	mustWrite(tmp, "default_default", "detail", "srv.json")
	mustWrite(tmp, "wukong_default", "tools", "srv.json")
	// Unrelated data sharing the cache root must survive the purge.
	mustWrite(tmp, "downloads", "dws-1.0.36.tar.gz")

	purged, err := s.PurgeDiscoveryData()
	if err != nil {
		t.Fatalf("PurgeDiscoveryData() error = %v", err)
	}
	if len(purged) != 2 {
		t.Fatalf("PurgeDiscoveryData() purged = %v, want 2 partitions", purged)
	}
	for _, sub := range []string{"market", "tools", "detail"} {
		if _, statErr := os.Stat(filepath.Join(tmp, "default_default", sub)); !os.IsNotExist(statErr) {
			t.Errorf("%s dir survived the purge (stat err = %v)", sub, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "wukong_default", "tools")); !os.IsNotExist(statErr) {
		t.Errorf("second partition tools dir survived the purge (stat err = %v)", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "downloads", "dws-1.0.36.tar.gz")); statErr != nil {
		t.Errorf("unrelated downloads data was removed: %v", statErr)
	}
}

func TestPurgeDiscoveryDataMissingRootIsNoop(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "does-not-exist"))
	purged, err := s.PurgeDiscoveryData()
	if err != nil {
		t.Fatalf("PurgeDiscoveryData() error = %v", err)
	}
	if len(purged) != 0 {
		t.Errorf("PurgeDiscoveryData() purged = %v, want none", purged)
	}
}
