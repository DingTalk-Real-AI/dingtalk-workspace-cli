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
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/process"
)

// LockFileName is the on-disk name of the bus single-instance lock. It lives
// inside the bus working directory
// (<ConfigDir>/events/<edition>/<source_kind>/<identity_hash>/).
const LockFileName = "bus.lock"

// ErrBusy is re-exported from lock for callers that only depend on bus.
var ErrBusy = lock.ErrBusy

// ErrStaleOwnerAlive indicates the PID stored in bus.lock points at a
// live process — there is already a bus running for this ClientID and we
// must not start another one. (This case is hit when the holder is still
// alive but its flock was somehow released; in practice flock + PID always
// agree, so this is mostly defensive.)
var ErrStaleOwnerAlive = errors.New("bus: lock file PID is alive but flock was released; assuming live owner")

// Lock represents a held bus.lock. Close releases the flock and removes the
// PID file, so a subsequent bus can acquire cleanly. A zero Lock is unusable.
type Lock struct {
	inner *lock.File
}

// Acquire takes the bus lock at path and writes our PID into the file body.
//
// If the file already has a PID written by a previous run:
//  1. Try the flock first — if another process holds it, return ErrBusy
//     (a live bus is running, abort).
//  2. flock acquired but file contains a PID → check if that PID is
//     alive via process.Alive(). If alive → return ErrStaleOwnerAlive
//     (defensive; release our flock first). If dead → take over (orphan
//     cleanup) and overwrite PID with our own.
//
// On success the returned Lock owns an exclusive flock and a file body
// containing our PID. Concurrent competing processes will get ErrBusy.
func Acquire(path string) (*Lock, error) {
	l, err := lock.TryAcquire(path)
	if err != nil {
		return nil, err // already wraps ErrBusy when busy
	}

	// flock acquired. Read existing PID (if any).
	f := l.File()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("bus: seek lock: %w", err)
	}
	old, err := io.ReadAll(f)
	if err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("bus: read lock: %w", err)
	}

	if pid := parsePID(old); pid > 0 && process.Alive(pid) {
		// Defensive: flock returned us the lock, but the stored PID is
		// alive. This shouldn't normally happen (the live process holds
		// the flock), but it's possible across odd kernel/FS edge cases
		// (NFS, container restarts). Release and refuse to start.
		_ = l.Close()
		return nil, ErrStaleOwnerAlive
	}

	// Orphan or first-ever acquisition. Rewrite the file with our PID.
	if err := truncateAndWritePID(f, os.Getpid()); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("bus: write PID: %w", err)
	}
	return &Lock{inner: l}, nil
}

// ReadHolderPID returns the PID stored in path, or 0 if the file is missing
// or unreadable. Does NOT attempt to acquire the lock — useful for `event
// status` to display the holder without contention.
func ReadHolderPID(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return parsePID(b)
}

// Close releases the flock and best-effort blanks the PID body so a stale
// reader (e.g. `event status` racing our shutdown) does not see our
// long-dead PID and try to signal it. The lock file itself is NOT removed
// — keeping it on disk avoids a race where a competing bus could acquire
// inode-on-create faster than our truncate.
func (l *Lock) Close() error {
	if l == nil || l.inner == nil {
		return nil
	}
	// Blank the body before releasing the lock.
	f := l.inner.File()
	_ = truncateAndWritePID(f, 0)
	err := l.inner.Close()
	l.inner = nil
	return err
}

// HoldsPath returns the path the lock is held on.
func (l *Lock) HoldsPath() string {
	if l == nil || l.inner == nil {
		return ""
	}
	return l.inner.Path()
}

func truncateAndWritePID(f *os.File, pid int) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if pid > 0 {
		if _, err := fmt.Fprintf(f, "%d\n", pid); err != nil {
			return err
		}
	}
	return f.Sync()
}

func parsePID(b []byte) int {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
