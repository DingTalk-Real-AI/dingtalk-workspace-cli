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

//go:build windows

package lock

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
	// Keep the mandatory Windows lock outside the PID payload at the start
	// of the file. LockFileEx byte-range locks block reads through a second
	// handle, so locking byte zero made bus status unable to read the holder
	// PID while the daemon was running.
	lockfileLockOffset = 1 << 30
)

func lockFile(f *os.File) error {
	handle := windows.Handle(f.Fd())
	ol := &windows.Overlapped{Offset: lockfileLockOffset}
	return windows.LockFileEx(
		handle,
		lockfileExclusiveLock|lockfileFailImmediately,
		0,
		1,
		0,
		(*windows.Overlapped)(unsafe.Pointer(ol)),
	)
}

func unlockFile(f *os.File) {
	handle := windows.Handle(f.Fd())
	ol := &windows.Overlapped{Offset: lockfileLockOffset}
	_ = windows.UnlockFileEx(
		handle,
		0,
		1,
		0,
		(*windows.Overlapped)(unsafe.Pointer(ol)),
	)
}

func isBusy(err error) bool {
	// LockFileEx with LOCKFILE_FAIL_IMMEDIATELY returns ERROR_LOCK_VIOLATION
	// when the region is already locked.
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING)
}
