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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// MetaFileName is the on-disk name of the bus metadata file. It lives
// alongside bus.lock and bus.sock inside the bus working directory.
const MetaFileName = "bus.meta"

// Meta is the JSON document written once at bus startup. Its primary
// purpose is to let `dws event list/status --all` reverse-map directory
// names (clientIDHash hex) back to the human-readable ClientID. It also
// records bus identity for protocol-compatibility diagnostics (a future
// consume client built against bus_version="v2" can refuse to dial a
// bus_version="v1" bus, etc.).
//
// The file is overwritten on each bus startup (so a previous bus's stale
// meta does not persist past a fresh boot) and intentionally NOT deleted
// on Close — keeping it on disk helps `event status` diagnose an orphan
// (bus.lock empty + bus.meta present + PID dead = clean orphan).
type Meta struct {
	ClientID     string              `json:"client_id"`
	Edition      string              `json:"edition"`
	SourceKind   dwsevent.SourceKind `json:"source_kind,omitempty"`
	IdentityHash string              `json:"identity_hash,omitempty"`
	SourceID     string              `json:"source_id,omitempty"`
	StartedAt    time.Time           `json:"started_at"`
	SDKVersion   string              `json:"sdk_version,omitempty"`
	BusVersion   string              `json:"bus_version"`
	BusPID       int                 `json:"bus_pid"`
}

// CurrentBusVersion identifies the bus wire/storage compatibility level.
// Bumped only on breaking changes (IPC protocol, lockfile shape, meta
// schema). v1 is the initial value; the field is parsed defensively by
// readers (older readers tolerate unknown fields via encoding/json).
const CurrentBusVersion = "v1"

var (
	metaMarshalIndent = json.MarshalIndent
	metaWriteFile     = os.WriteFile
	metaRename        = os.Rename
	metaRemove        = os.Remove
)

// WriteMeta atomically writes m to <dir>/bus.meta. Atomic via tmp-file +
// rename. Directory permissions are not changed; caller must mkdir the
// containing directory beforehand with pkg/config.DirPerm.
func WriteMeta(dir string, m Meta) error {
	if m.BusVersion == "" {
		m.BusVersion = CurrentBusVersion
	}
	if m.BusPID == 0 {
		m.BusPID = os.Getpid()
	}
	if m.StartedAt.IsZero() {
		m.StartedAt = time.Now().UTC()
	}
	b, err := metaMarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("bus: marshal meta: %w", err)
	}
	final := filepath.Join(dir, MetaFileName)
	tmp := final + ".tmp"
	if err := metaWriteFile(tmp, b, config.FilePerm); err != nil {
		return fmt.Errorf("bus: write tmp meta: %w", err)
	}
	if err := metaRename(tmp, final); err != nil {
		_ = metaRemove(tmp)
		return fmt.Errorf("bus: rename meta: %w", err)
	}
	return nil
}

// ReadMeta loads and parses <dir>/bus.meta. Returns (nil, error) when the
// file is missing or malformed. Used by `event list/status --all` to
// resolve clientIDHash → original ClientID.
func ReadMeta(dir string) (*Meta, error) {
	path := filepath.Join(dir, MetaFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("bus: parse meta %s: %w", path, err)
	}
	return &m, nil
}
