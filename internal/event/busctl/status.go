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

package busctl

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/process"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// BusEntryState classifies a discovered bus directory's runtime state.
// Used by `dws event status/list` to render the table and by
// --fail-on-orphan to drive exit code.
type BusEntryState string

const (
	// BusStateRunning: bus.lock holds an alive PID — the daemon is up.
	BusStateRunning BusEntryState = "running"
	// BusStateOrphan: bus.meta exists but bus.lock PID is dead. The user
	// should `dws event stop --client-id <id>` (which detects the dead
	// PID and unblocks fresh starts) or rm -rf the working directory.
	BusStateOrphan BusEntryState = "orphan"
	// BusStateNotRunning: directory exists (e.g. bus.meta retained for
	// historic reasons) but bus.lock is missing or empty. Clean state.
	BusStateNotRunning BusEntryState = "not_running"
)

// BusEntry is one bus working directory found on disk plus its detected
// lifecycle state. EnumerateBuses produces these; the cobra layer joins
// them with QueryStatus output to render the full status view.
type BusEntry struct {
	WorkDir      string              `json:"workdir"`
	Edition      string              `json:"edition"`
	SourceKind   dwsevent.SourceKind `json:"source_kind,omitempty"`
	ClientIDHash string              `json:"client_id_hash"`
	IdentityHash string              `json:"identity_hash,omitempty"`
	HolderPID    int                 `json:"holder_pid"`
	State        BusEntryState       `json:"state"`
	// Meta, if non-nil, lets list/status display the original ClientID
	// (reverse-mapped from the hash) and the bus start time.
	Meta *bus.Meta `json:"meta,omitempty"`
}

// IPCEndpoint returns the IPC endpoint for this entry. Unix uses the
// bus.sock path inside WorkDir; Windows uses the deterministic pipe name
// derived from edition + clientIDHash (same scheme defaultIPCEndpoint
// uses in the cobra layer).
func (e BusEntry) IPCEndpoint() string {
	hash := e.ClientIDHash
	if e.IdentityHash != "" {
		hash = e.IdentityHash
	}
	if runtime.GOOS == "windows" {
		kind := string(e.SourceKind)
		if kind == "" {
			kind = string(dwsevent.SourceKindAppStream)
		}
		return `\\.\pipe\dws-event-` + e.Edition + "-" + kind + "-" + hash
	}
	return filepath.Join(e.WorkDir, "bus.sock")
}

// EnumerateBuses scans <configDir>/events/<editionFilter>/*/ for bus
// working directories. An empty editionFilter scans every edition
// directory found under events/.
//
// Returns a deterministic slice sorted by (edition, source_kind, identity_hash).
// Missing/inaccessible directories are skipped silently — list/status
// commands should still succeed when only some editions have ever run a
// bus.
func EnumerateBuses(configDir string, editionFilter string) ([]BusEntry, error) {
	root := filepath.Join(configDir, "events")
	editions, err := listSubdirs(root)
	if err != nil {
		// events/ might not exist if no bus ever ran — that's fine,
		// surface an empty list rather than an error.
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("busctl: scan events dir: %w", err)
	}

	var out []BusEntry
	for _, ed := range editions {
		if editionFilter != "" && ed != editionFilter {
			continue
		}
		editionDir := filepath.Join(root, ed)
		hashDirs, err := listSubdirs(editionDir)
		if err != nil {
			continue
		}
		for _, h := range hashDirs {
			candidate := filepath.Join(editionDir, h)
			if isSourceKindDir(h) {
				identityDirs, err := listSubdirs(candidate)
				if err != nil {
					continue
				}
				for _, ih := range identityDirs {
					workDir := filepath.Join(candidate, ih)
					out = append(out, inspectEntry(workDir, ed, h, ih))
				}
				continue
			}
			// Legacy v1 app-stream layout: events/<edition>/<client_hash>.
			out = append(out, inspectEntry(candidate, ed, "", h))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Edition != out[j].Edition {
			return out[i].Edition < out[j].Edition
		}
		if out[i].SourceKind != out[j].SourceKind {
			return out[i].SourceKind < out[j].SourceKind
		}
		return out[i].IdentityHash < out[j].IdentityHash
	})
	return out, nil
}

// FindBusByClientID is the "current ClientID" lookup used by `event status`
// (no --all). Returns the entry for the given (edition, clientIDHash) pair
// or nil if no bus has ever started for it. The caller derives the hash
// using event.ClientIDHash.
func FindBusByClientID(configDir, editionName, clientIDHash string) *BusEntry {
	workDir := filepath.Join(configDir, "events", editionName, string(dwsevent.SourceKindAppStream), clientIDHash)
	if _, err := os.Stat(workDir); err != nil {
		legacy := filepath.Join(configDir, "events", editionName, clientIDHash)
		if _, legacyErr := os.Stat(legacy); legacyErr != nil {
			return nil
		}
		workDir = legacy
	}
	e := inspectEntry(workDir, editionName, string(dwsevent.SourceKindAppStream), clientIDHash)
	return &e
}

// FindBusByIdentity looks up a bus in the source-kind-aware layout.
func FindBusByIdentity(configDir, editionName string, sourceKind dwsevent.SourceKind, identityHash string) *BusEntry {
	if sourceKind == "" {
		sourceKind = dwsevent.SourceKindAppStream
	}
	workDir := filepath.Join(configDir, "events", editionName, string(sourceKind), identityHash)
	if _, err := os.Stat(workDir); err != nil {
		return nil
	}
	e := inspectEntry(workDir, editionName, string(sourceKind), identityHash)
	return &e
}

// listSubdirs returns immediate subdirectories of path, by basename. Ignores
// regular files. Returns the err from ReadDir unchanged (callers handle
// os.IsNotExist).
func listSubdirs(path string) ([]string, error) {
	ents, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// inspectEntry reads bus.meta + bus.lock and derives the lifecycle state.
// Never returns an error: any read failure folds into BusStateNotRunning.
func inspectEntry(workDir, editionName, sourceKindRaw, identityHash string) BusEntry {
	sourceKind := dwsevent.SourceKind(sourceKindRaw)
	if sourceKind == "" {
		sourceKind = dwsevent.SourceKindAppStream
	}
	e := BusEntry{
		WorkDir:      workDir,
		Edition:      editionName,
		SourceKind:   sourceKind,
		ClientIDHash: identityHash,
		IdentityHash: identityHash,
		State:        BusStateNotRunning,
	}
	if m, err := bus.ReadMeta(workDir); err == nil {
		e.Meta = m
		if m.SourceKind != "" {
			e.SourceKind = m.SourceKind
		}
		if m.IdentityHash != "" {
			e.IdentityHash = m.IdentityHash
			e.ClientIDHash = m.IdentityHash
		}
	}
	pid := bus.ReadHolderPID(filepath.Join(workDir, bus.LockFileName))
	e.HolderPID = pid
	switch {
	case pid > 0 && process.Alive(pid):
		e.State = BusStateRunning
	case pid > 0 && !process.Alive(pid):
		e.State = BusStateOrphan
	case pid == 0 && e.Meta != nil:
		// meta retained but lock cleared — bus exited cleanly. Render as
		// not_running (with the historical meta visible if the user asked
		// for --format json).
		e.State = BusStateNotRunning
	}
	return e
}

func isSourceKindDir(name string) bool {
	return name == string(dwsevent.SourceKindAppStream) || name == string(dwsevent.SourceKindPersonalStream)
}

// DefaultStatusRPCTimeout caps how long QueryStatus waits for the bus to
// reply. 2s is generous — the bus's status_resp is a synchronous
// in-memory snapshot, sub-millisecond in practice; the timeout exists
// only to bound pathological cases (bus stuck in shutdown).
const DefaultStatusRPCTimeout = 2 * time.Second

// QueryStatus dials the bus IPC, sends Hello with Role=status, sends a
// StatusReq, reads exactly one StatusResp, and closes. Returns the
// decoded response or an error if any step fails.
//
// Used by `dws event status` and `dws event list` to fetch live
// per-consumer / per-event-type counters. The bus's handleStatusRPC
// path (see internal/event/bus/daemon.go) handles this connection
// without registering with the Hub — ad-hoc tooling does not count as
// a consumer in `status.active_consumers`.
func QueryStatus(endpoint string) (*transport.StatusResp, error) {
	conn, err := transport.Dial(endpoint)
	if err != nil {
		return nil, fmt.Errorf("busctl: dial bus for status: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(DefaultStatusRPCTimeout))

	w := transport.NewWriter(conn)
	r := transport.NewReader(conn)

	if err := w.WriteJSON(transport.Hello{
		Type:        transport.FrameTypeHello,
		ConsumerPID: os.Getpid(),
		Role:        transport.HelloRoleStatus,
	}); err != nil {
		return nil, fmt.Errorf("busctl: write hello: %w", err)
	}
	if err := w.WriteJSON(transport.StatusReq{Type: transport.FrameTypeStatusReq}); err != nil {
		return nil, fmt.Errorf("busctl: write status_req: %w", err)
	}
	var resp transport.StatusResp
	if err := r.ReadJSON(&resp); err != nil {
		return nil, fmt.Errorf("busctl: read status_resp: %w", err)
	}
	return &resp, nil
}

// EntryStatus combines static FS info (BusEntry) with the live RPC
// snapshot (StatusResp). For not_running / orphan entries Live is nil.
type EntryStatus struct {
	Entry BusEntry              `json:"entry"`
	Live  *transport.StatusResp `json:"live,omitempty"`
}

// QueryEntry fetches the live status for one BusEntry. Returns the entry
// wrapped with a nil Live when state != running (or when the dial fails).
// Errors from QueryStatus are folded into Live=nil so the caller's table
// rendering does not need to surface per-bus dial failures (they are
// already conveyed by State).
func QueryEntry(entry BusEntry) EntryStatus {
	out := EntryStatus{Entry: entry}
	if entry.State != BusStateRunning {
		return out
	}
	live, err := QueryStatus(entry.IPCEndpoint())
	if err == nil {
		out.Live = live
	}
	return out
}
