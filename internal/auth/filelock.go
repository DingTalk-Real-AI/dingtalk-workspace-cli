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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/config"
)

const (
	lockFileName   = ".data.lock"
	lockRetryDelay = 50 * time.Millisecond
)

// ── Process-level lock ──────────────────────────────────────────────────
// Prevents multiple goroutines within the same process from refreshing
// simultaneously. Uses sync.Map with channel signaling for efficient waiting.

var processLocks sync.Map // map[string]chan struct{}

// processLockKey generates a unique key for process-level locking.
func processLockKey(configDir string) string {
	return "refresh:" + configDir
}

// acquireProcessLock attempts to acquire the process-level lock.
// If another goroutine holds it, this blocks until that goroutine releases.
// Returns a release function that MUST be called when done.
func acquireProcessLock(ctx context.Context, configDir string) (release func(), waited bool, err error) {
	key := processLockKey(configDir)
	done := make(chan struct{})

	for {
		// Try to store our channel; if successful, we own the lock
		if existing, loaded := processLocks.LoadOrStore(key, done); !loaded {
			// We got the lock
			return func() {
				close(done)
				processLocks.Delete(key)
			}, waited, nil
		} else {
			// Another goroutine holds the lock; wait for it
			ch, ok := existing.(chan struct{})
			if !ok {
				// Unexpected type; delete and retry
				processLocks.Delete(key)
				continue
			}
			waited = true
			select {
			case <-ch:
				// Lock released; retry to acquire
				continue
			case <-ctx.Done():
				return nil, waited, ctx.Err()
			}
		}
	}
}

// ── File-level lock ─────────────────────────────────────────────────────
// Prevents multiple CLI processes from refreshing simultaneously.
// Platform support:
//   - Unix/macOS: flock(2) system call
//   - Windows: LockFileEx / UnlockFileEx from kernel32.dll

// tokenFileLock provides cross-process file locking for token operations.
// It prevents concurrent refresh from multiple CLI processes,
// which can corrupt token data when two processes refresh simultaneously.
type tokenFileLock struct {
	path string
	file *os.File
}

// acquireTokenLock acquires an exclusive file lock for token operations.
// It blocks (with timeout) if another process holds the lock.
// The caller MUST call release() when done.
func acquireTokenLock(configDir string) (*tokenFileLock, error) {
	if err := os.MkdirAll(configDir, config.DirPerm); err != nil {
		return nil, fmt.Errorf("creating config dir for lock: %w", err)
	}

	lockPath := filepath.Join(configDir, lockFileName)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, config.FilePerm)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	deadline := time.Now().Add(config.LockTimeout)
	for {
		if err := lockFile(f); err == nil {
			return &tokenFileLock{path: lockPath, file: f}, nil
		}

		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("timeout acquiring token lock after %v (another dws process may be running)", config.LockTimeout)
		}

		time.Sleep(lockRetryDelay)
	}
}

// release releases the file lock.
func (l *tokenFileLock) release() {
	if l.file != nil {
		unlockFile(l.file)
		_ = l.file.Close()
		l.file = nil
	}
}

// ── Dual-layer lock ─────────────────────────────────────────────────────
// Combines process-level and file-level locks for comprehensive protection.

// DualLock holds both process-level and file-level locks.
type DualLock struct {
	processRelease func()
	fileLock       *tokenFileLock
	Waited         bool // true if we waited for another goroutine/process
}

// AcquireDualLock acquires both process-level and file-level locks.
// This provides comprehensive protection against:
// 1. Multiple goroutines in the same process (sync.Map)
// 2. Multiple CLI processes (file lock)
//
// The caller MUST call Release() when done.
func AcquireDualLock(ctx context.Context, configDir string) (*DualLock, error) {
	// 1. Acquire process-level lock first (fast, in-memory)
	processRelease, waited, err := acquireProcessLock(ctx, configDir)
	if err != nil {
		return nil, fmt.Errorf("acquiring process lock: %w", err)
	}

	// 2. Acquire file-level lock (cross-process)
	fileLock, err := acquireTokenLock(configDir)
	if err != nil {
		processRelease() // Release process lock on failure
		return nil, fmt.Errorf("acquiring file lock: %w", err)
	}

	return &DualLock{
		processRelease: processRelease,
		fileLock:       fileLock,
		Waited:         waited,
	}, nil
}

// Release releases both locks in reverse order.
func (d *DualLock) Release() {
	if d.fileLock != nil {
		d.fileLock.release()
		d.fileLock = nil
	}
	if d.processRelease != nil {
		d.processRelease()
		d.processRelease = nil
	}
}
