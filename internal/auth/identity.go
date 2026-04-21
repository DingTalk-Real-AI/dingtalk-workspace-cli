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
// Each agent installation gets a unique agentId (UUID v4) that persists across
// version upgrades but regenerates on reinstall. This identity is transparently
// injected into MCP HTTP headers for gateway-side data collection.
package auth

import (
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	mathrand "math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const identityFile = "identity.json"

// defaultDingTalkSource is the header value for x-dingtalk-source in the
// open-source build. Downstream editions MAY override via edition.Hooks.DingTalkSource.
const defaultDingTalkSource = "github"

// Identity holds the agent instance identification fields.
type Identity struct {
	AgentID string `json:"agentId"` // UUID v4, generated at install time
	Source  string `json:"source"`  // data source, default "dws"
}

// Load reads the identity from <configDir>/identity.json.
// Returns nil if the file does not exist or cannot be parsed.
func Load(configDir string) *Identity {
	path := filepath.Join(configDir, identityFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil
	}
	if id.AgentID == "" {
		return nil
	}
	return &id
}

// EnsureExists loads existing identity or creates a new one if not present.
func EnsureExists(configDir string) *Identity {
	if id := Load(configDir); id != nil {
		return id
	}

	id := &Identity{
		AgentID: generateUUID(),
		Source:  "dws",
	}

	// Best-effort persist — don't fail the CLI if write fails.
	_ = save(configDir, id)
	return id
}

// Headers returns the identity as HTTP header key-value pairs.
func (id *Identity) Headers() map[string]string {
	if id == nil {
		return nil
	}
	h := make(map[string]string, 5)
	if id.AgentID != "" {
		h["x-dws-agent-id"] = id.AgentID
	}
	if id.Source != "" {
		h["x-dws-source"] = id.Source
	}
	scenarioCode := "com.dingtalk.cli"
	if sc := edition.Get().ScenarioCode; sc != "" {
		scenarioCode = sc
	}
	h["x-dingtalk-scenario-code"] = scenarioCode
	source := defaultDingTalkSource
	if ds := edition.Get().DingTalkSource; ds != "" {
		source = ds
	}
	h["x-dingtalk-source"] = source
	return h
}

func save(configDir string, id *Identity) error {
	if err := os.MkdirAll(configDir, config.DirPerm); err != nil {
		return err
	}
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(configDir, identityFile), data, config.FilePerm)
}

// generateUUID produces a UUID v4 string. The primary path uses
// crypto/rand; when the OS entropy source is unavailable, we fall back
// to a time+pid+counter derived pseudo-random seed instead of returning
// a fixed constant, so that agents deployed on broken-entropy hosts
// do not collide on a shared identity.
//
// This function MUST remain non-panicking: identity persistence is a
// best-effort path and a panic here would crash the CLI on any
// otherwise-recoverable request.
func generateUUID() string {
	var u [16]byte
	if _, err := cryptorand.Read(u[:]); err != nil {
		warnEntropyFallbackOnce(err)
		fillUUIDFromFallback(u[:])
	}
	u[6] = (u[6] & 0x0f) | 0x40 // version 4
	u[8] = (u[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

var (
	// entropyWarnOnce ensures the fallback-entropy warning is emitted at
	// most once per process lifetime, even under sustained failure.
	entropyWarnOnce sync.Once

	// fallbackRand is a math/rand source seeded from time+pid on first use.
	// It's guarded by its own mutex because math/rand.Rand is not
	// goroutine-safe.
	fallbackRandMu sync.Mutex
	fallbackRand   *mathrand.Rand
)

// warnEntropyFallbackOnce prints a single stderr warning the first time
// crypto/rand is unavailable in this process. Subsequent fallbacks stay
// silent to avoid log-flooding long-running CLI sessions.
func warnEntropyFallbackOnce(err error) {
	entropyWarnOnce.Do(func() {
		fmt.Fprintf(os.Stderr,
			"dws: crypto/rand unavailable (%v); falling back to time-seeded UUID. "+
				"This indicates a broken entropy source on this host.\n", err)
	})
}

// fillUUIDFromFallback populates buf with pseudo-random bytes sourced
// from a lazily initialised, time+pid seeded math/rand.Rand. Callers
// holding the crypto/rand failure path invoke this to avoid collapsing
// every agent on a broken host onto the same identity.
func fillUUIDFromFallback(buf []byte) {
	fallbackRandMu.Lock()
	defer fallbackRandMu.Unlock()
	if fallbackRand == nil {
		seed := time.Now().UnixNano() ^ int64(os.Getpid())<<32
		fallbackRand = mathrand.New(mathrand.NewSource(seed))
	}
	_, _ = fallbackRand.Read(buf)
}
