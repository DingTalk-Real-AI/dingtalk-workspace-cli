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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveAgentID_Format(t *testing.T) {
	id := deriveAgentID("machine-abc", "claudecode")
	if !strings.HasPrefix(id, "dwsa_") {
		t.Fatalf("want dwsa_ prefix, got %q", id)
	}
	if len(id) != len("dwsa_")+12 {
		t.Fatalf("want 12 base62 chars after prefix, got %q (len %d)", id, len(id))
	}
}

func TestDeriveAgentID_Deterministic(t *testing.T) {
	a := deriveAgentID("seed", "claudecode")
	b := deriveAgentID("seed", "claudecode")
	if a != b {
		t.Fatalf("derivation must be deterministic: %q != %q", a, b)
	}
}

func TestDeriveAgentID_DistinctByChannelAndMachine(t *testing.T) {
	m1c1 := deriveAgentID("machine1", "claudecode")
	m1c2 := deriveAgentID("machine1", "cursor")
	m2c1 := deriveAgentID("machine2", "claudecode")
	if m1c1 == m1c2 {
		t.Errorf("same machine, different channel must differ: %q", m1c1)
	}
	if m1c1 == m2c1 {
		t.Errorf("different machine, same channel must differ: %q", m1c1)
	}
}

func TestResolveAgentID_IdempotentAndPersisted(t *testing.T) {
	dir := t.TempDir()
	id := EnsureExists(dir)

	first := id.ResolveAgentID(dir, "claudecode", "sig:CLAUDECODE")
	second := id.ResolveAgentID(dir, "claudecode", "sig:CLAUDECODE")
	if first != second {
		t.Fatalf("ResolveAgentID must be idempotent: %q != %q", first, second)
	}

	// Reload from disk — the channel entry must have persisted.
	reloaded := Load(dir)
	if reloaded == nil {
		t.Fatal("expected identity to persist")
	}
	e, ok := reloaded.Agents["claudecode"]
	if !ok || e.AgentID != first {
		t.Fatalf("persisted agentId mismatch: %+v", reloaded.Agents)
	}
	if e.Detect != "sig:CLAUDECODE" {
		t.Errorf("want detect signal recorded, got %q", e.Detect)
	}
}

func TestResolveAgentID_EmptyAgentCodeGoesCustom(t *testing.T) {
	dir := t.TempDir()
	id := EnsureExists(dir)
	got := id.ResolveAgentID(dir, "", "fallback")
	want := id.ResolveAgentID(dir, AgentCodeCustom, "fallback")
	if got != want {
		t.Fatalf("empty agent_code must map to custom bucket: %q != %q", got, want)
	}
}

// A v1 file ({agentId, source}) must migrate: machineId backfilled from the
// legacy agentId, and per-channel derivation keyed off that stable seed.
func TestLoad_MigratesV1(t *testing.T) {
	dir := t.TempDir()
	v1 := `{"agentId":"504ddd36-3acf-45f6-9c1f-82f99260a419","source":"dws"}`
	if err := os.WriteFile(filepath.Join(dir, identityFile), []byte(v1), 0o600); err != nil {
		t.Fatal(err)
	}

	id := Load(dir)
	if id == nil {
		t.Fatal("v1 file should load")
	}
	if id.MachineID != "504ddd36-3acf-45f6-9c1f-82f99260a419" {
		t.Fatalf("machineId must backfill from legacy agentId, got %q", id.MachineID)
	}
	if id.machineSeed() != id.MachineID {
		t.Fatalf("seed should be machineId, got %q", id.machineSeed())
	}
	// Derivation is stable against the migrated seed.
	want := deriveAgentID(id.MachineID, "claudecode")
	if got := id.ResolveAgentID(dir, "claudecode", "sig:CLAUDECODE"); got != want {
		t.Fatalf("post-migration derivation mismatch: %q != %q", got, want)
	}
}
