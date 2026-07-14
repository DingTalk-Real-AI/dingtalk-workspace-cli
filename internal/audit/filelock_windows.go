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

package audit

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const lockfileExclusiveLock = 0x00000002

// lockFile takes a non-blocking exclusive lock on the first byte range; returns
// an error when another process holds it so the caller can retry with a timeout.
func lockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		lockfileExclusiveLock|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		(*windows.Overlapped)(unsafe.Pointer(ol)),
	)
}

func unlockFile(f *os.File) {
	ol := new(windows.Overlapped)
	_ = windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0,
		1,
		0,
		(*windows.Overlapped)(unsafe.Pointer(ol)),
	)
}
