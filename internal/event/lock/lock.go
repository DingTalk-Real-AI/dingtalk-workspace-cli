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

package lock

import (
	"errors"
	"fmt"
	"os"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// ErrBusy indicates the lock is currently held by another process. Callers
// distinguish "actually busy" (another live bus) from "lock file
// inaccessible" (FS error) by checking for this sentinel.
var ErrBusy = errors.New("lock: file is held by another process")

// File represents a held exclusive file lock. Close releases the lock and
// closes the underlying file handle. A zero File is not usable.
type File struct {
	f *os.File
}

var acquireFileLock = lockFile

// TryAcquire opens path (creating it if absent, mode 0600) and attempts to
// take an exclusive non-blocking lock. Returns ErrBusy when the lock is held
// by another process; any other error wraps the underlying I/O failure.
//
// The directory containing path must already exist; callers should mkdir
// with pkg/config.DirPerm beforehand.
func TryAcquire(path string) (*File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, config.FilePerm)
	if err != nil {
		return nil, fmt.Errorf("lock: open %s: %w", path, err)
	}
	if err := acquireFileLock(f); err != nil {
		_ = f.Close()
		if isBusy(err) {
			return nil, ErrBusy
		}
		return nil, fmt.Errorf("lock: flock %s: %w", path, err)
	}
	return &File{f: f}, nil
}

// File returns the underlying *os.File so callers can Read/Write content
// while holding the lock. The handle MUST NOT be closed by the caller —
// use Close on the lock File instead.
func (l *File) File() *os.File { return l.f }

// Path returns the file path the lock is held on.
func (l *File) Path() string {
	if l == nil || l.f == nil {
		return ""
	}
	return l.f.Name()
}

// Close releases the lock and closes the file handle. Safe to call on a nil
// receiver. Subsequent calls are no-ops.
func (l *File) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	unlockFile(l.f)
	err := l.f.Close()
	l.f = nil
	return err
}
