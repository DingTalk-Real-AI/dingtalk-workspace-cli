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

//go:build darwin

package corecmd

import (
	"os"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openTestPTY allocates a real pty pair so the terminal-only interactive
// prompt branch of ConfirmSafety can run natively on macOS. The slave side is
// a genuine TTY (isatty true); the master side feeds it the scripted answer.
// It reports ok=false instead of failing when the environment cannot allocate
// a pty (e.g. a sandbox without /dev/ptmx), so callers skip gracefully.
func openTestPTY(t *testing.T) (master, slave *os.File, ok bool) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Logf("open /dev/ptmx: %v", err)
		return nil, nil, false
	}
	fd := int(m.Fd())
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		t.Logf("grantpt: %v", err)
		_ = m.Close()
		return nil, nil, false
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		t.Logf("unlockpt: %v", err)
		_ = m.Close()
		return nil, nil, false
	}
	// ptsname(3): TIOCPTYGNAME fills a 128-byte buffer with the slave path.
	buf := make([]byte, 128)
	if _, _, errno := unix.Syscall(
		unix.SYS_IOCTL, uintptr(fd), uintptr(unix.TIOCPTYGNAME), uintptr(unsafe.Pointer(&buf[0])),
	); errno != 0 {
		t.Logf("ptsname ioctl: %v", errno)
		_ = m.Close()
		return nil, nil, false
	}
	name := strings.TrimRight(string(buf), "\x00")
	s, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		t.Logf("open pty slave %s: %v", name, err)
		_ = m.Close()
		return nil, nil, false
	}
	return m, s, true
}
