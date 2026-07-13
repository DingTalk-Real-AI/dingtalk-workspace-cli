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

package bus

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteRead_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	m := Meta{
		ClientID:   "ding_xyz",
		Edition:    "open",
		StartedAt:  time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
		SDKVersion: "v0.9.1",
		BusPID:     12345,
	}
	if err := WriteMeta(dir, m); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	got, err := ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if got.ClientID != m.ClientID || got.Edition != m.Edition || got.BusPID != m.BusPID {
		t.Errorf("roundtrip mismatch: got %+v want %+v", got, m)
	}
	if got.BusVersion != CurrentBusVersion {
		t.Errorf("BusVersion default not applied: %q", got.BusVersion)
	}
}

func TestWriteMeta_DefaultsPID(t *testing.T) {
	dir := t.TempDir()
	if err := WriteMeta(dir, Meta{ClientID: "x", Edition: "open"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	got, err := ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if got.BusPID != os.Getpid() {
		t.Errorf("BusPID default = %d, want %d", got.BusPID, os.Getpid())
	}
	if got.StartedAt.IsZero() {
		t.Error("StartedAt default not applied")
	}
}

func TestWriteMeta_AtomicNoTmpLeft(t *testing.T) {
	dir := t.TempDir()
	if err := WriteMeta(dir, Meta{ClientID: "x", Edition: "open"}); err != nil {
		t.Fatal(err)
	}
	// .tmp file must not remain after successful rename
	if _, err := os.Stat(filepath.Join(dir, MetaFileName+".tmp")); err == nil {
		t.Fatal(".tmp file leaked after WriteMeta")
	}
}

func TestReadMeta_MissingFileErrors(t *testing.T) {
	if _, err := ReadMeta(t.TempDir()); err == nil {
		t.Fatal("ReadMeta on missing file should error")
	}
}

func TestReadMeta_MalformedErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, MetaFileName), []byte("{garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadMeta(dir); err == nil {
		t.Fatal("ReadMeta on malformed JSON should error")
	}
}
