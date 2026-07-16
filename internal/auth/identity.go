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

// identity.go manages agent instance identification for tracking.
//
// Identity has two granularities, both injected into MCP HTTP headers for
// gateway-side statistics:
//
//   - machineId: a stable per-install UUID v4 (persists across upgrades,
//     regenerates on reinstall). Non-PII.
//   - agentId:   a per-(machine × agentCode) id derived deterministically from
//     machineId + agent_code, so one machine running multiple agent hosts
//     (e.g. claudecode + cursor) yields a distinct, idempotent agentId per
//     agent_code. Computed client-side — no gateway round-trip required.
//
// The agent_code itself is resolved by DetectAgentCode (agent_code_detect.go).
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

var (
	identityReadFile      = os.ReadFile
	identityUnmarshal     = json.Unmarshal
	identityMkdirAll      = os.MkdirAll
	identityMarshalIndent = json.MarshalIndent
	identityWriteFile     = os.WriteFile
	identityRandRead      = rand.Read
)

const identityFile = "identity.json"

// identityVersion is the current on-disk schema version. v1 files (no
// machineId/agents) are migrated transparently on load.
const identityVersion = 2

// AgentEntry records the derived agentId for a single agent_code on this
// machine.
type AgentEntry struct {
	AgentID   string `json:"agentId"`
	FirstSeen string `json:"firstSeen,omitempty"`
	Detect    string `json:"detect,omitempty"` // signal that decided the agent_code
}

// Identity holds the agent instance identification fields.
//
// AgentID is retained for backward compatibility with v1 readers: on a fresh
// install it is written equal to MachineID, and a v1 file's agentId is migrated
// into MachineID on load.
type Identity struct {
	Version   int                    `json:"version,omitempty"`
	AgentID   string                 `json:"agentId"`             // v1 install UUID; == MachineID on v2 installs
	MachineID string                 `json:"machineId,omitempty"` // stable per-install machine seed
	Source    string                 `json:"source"`              // data source, default "dws"
	Agents    map[string]*AgentEntry `json:"agents,omitempty"`    // agent_code -> derived agentId
}

// Load reads the identity from <configDir>/identity.json.
// Returns nil if the file does not exist or cannot be parsed.
// v1 files are migrated in-memory (machineId backfilled from agentId).
func Load(configDir string) *Identity {
	path := filepath.Join(configDir, identityFile)
	data, err := identityReadFile(path)
	if err != nil {
		return nil
	}
	var id Identity
	if err := identityUnmarshal(data, &id); err != nil {
		return nil
	}
	if id.AgentID == "" && id.MachineID == "" {
		return nil
	}
	id.migrate()
	return &id
}

// migrate backfills v2 fields from a v1 file in-memory (does not persist).
func (id *Identity) migrate() {
	if id.MachineID == "" {
		id.MachineID = id.AgentID // v1 install UUID becomes the machine seed
	}
	if id.AgentID == "" {
		id.AgentID = id.MachineID
	}
	if id.Source == "" {
		id.Source = "dws"
	}
	if id.Agents == nil {
		id.Agents = make(map[string]*AgentEntry)
	}
	id.Version = identityVersion
}

// EnsureExists loads existing identity or creates a new one if not present.
func EnsureExists(configDir string) *Identity {
	if id := Load(configDir); id != nil {
		return id
	}

	u := generateUUID()
	id := &Identity{
		Version:   identityVersion,
		AgentID:   u, // kept == MachineID for backward-compat
		MachineID: u,
		Source:    "dws",
		Agents:    make(map[string]*AgentEntry),
	}

	// Best-effort persist — don't fail the CLI if write fails.
	_ = save(configDir, id)
	return id
}

// machineSeed returns the stable seed used to derive per-channel agentIds.
func (id *Identity) machineSeed() string {
	if id.MachineID != "" {
		return id.MachineID
	}
	return id.AgentID
}

// ResolveAgentID returns the per-(machine × agentCode) agentId, deriving and
// persisting it on first sight of an agentCode. Idempotent: the same machine
// and agentCode always yields the same id, which is what makes cumulative
// per-agent_code statistics possible. An empty agentCode has no per-agent
// identity and returns empty.
func (id *Identity) ResolveAgentID(configDir, agentCode, signal string) string {
	if agentCode == "" {
		return ""
	}
	if id.Agents == nil {
		id.Agents = make(map[string]*AgentEntry)
	}
	if e, ok := id.Agents[agentCode]; ok && e.AgentID != "" {
		return e.AgentID
	}
	aid := deriveAgentID(id.machineSeed(), agentCode)
	id.Agents[agentCode] = &AgentEntry{
		AgentID:   aid,
		FirstSeen: time.Now().UTC().Format(time.RFC3339),
		Detect:    signal,
	}
	_ = save(configDir, id) // best-effort cache; recomputable if it fails
	return aid
}

// Headers returns the identity as static HTTP header key-value pairs.
// x-dws-agent-id carries the stable machine-level id (== v1 install UUID), kept
// continuous across versions. The per-(machine × agent_code) instance id is a
// SEPARATE header (x-dws-agent-instance-id) injected by the caller via
// ResolveAgentID — it does not override x-dws-agent-id.
func (id *Identity) Headers() map[string]string {
	if id == nil {
		return nil
	}
	h := make(map[string]string, 5)
	if seed := id.machineSeed(); seed != "" {
		h["x-dws-agent-id"] = seed
	}
	if id.Source != "" {
		h["x-dws-source"] = id.Source
	}
	scenarioCode := "com.dingtalk.cli"
	if sc := edition.Get().ScenarioCode; sc != "" {
		scenarioCode = sc
	}
	h["x-dingtalk-scenario-code"] = scenarioCode
	h["x-dingtalk-source"] = "github"
	return h
}

func save(configDir string, id *Identity) error {
	if err := identityMkdirAll(configDir, config.DirPerm); err != nil {
		return err
	}
	data, err := identityMarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return identityWriteFile(filepath.Join(configDir, identityFile), data, config.FilePerm)
}

// deriveAgentID computes a stable, client-side agentId for a (machine,
// agentCode) pair: dwsa_<12 base62 chars of sha256(seed|agentCode)>.
// Deterministic and idempotent; no gateway allocation needed for statistics.
func deriveAgentID(seed, agentCode string) string {
	sum := sha256.Sum256([]byte(seed + "|" + agentCode))
	return formatDerivedAgentID(base62Encode(sum[:]))
}

func formatDerivedAgentID(enc string) string {
	for len(enc) < 12 {
		enc = "0" + enc
	}
	return "dwsa_" + enc[:12]
}

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func base62Encode(b []byte) string {
	n := new(big.Int).SetBytes(b)
	if n.Sign() == 0 {
		return "0"
	}
	base := big.NewInt(62)
	mod := new(big.Int)
	var out []byte
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		out = append(out, base62Alphabet[mod.Int64()])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

// generateUUID produces a UUID v4 string.
func generateUUID() string {
	var u [16]byte
	if _, err := identityRandRead(u[:]); err != nil {
		// Extremely unlikely; fallback to zero UUID rather than panic.
		return "00000000-0000-4000-8000-000000000000"
	}
	u[6] = (u[6] & 0x0f) | 0x40 // version 4
	u[8] = (u[8] & 0x3f) | 0x80 // variant 10
	return fmtUUID(u)
}

func fmtUUID(u [16]byte) string {
	const hexdig = "0123456789abcdef"
	// 8-4-4-4-12 with dashes => 36 bytes
	buf := make([]byte, 36)
	pos := 0
	for i := 0; i < 16; i++ {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			buf[pos] = '-'
			pos++
		}
		buf[pos] = hexdig[u[i]>>4]
		buf[pos+1] = hexdig[u[i]&0x0f]
		pos += 2
	}
	return string(buf)
}
