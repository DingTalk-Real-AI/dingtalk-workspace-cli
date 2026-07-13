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

package process

import (
	"golang.org/x/sys/windows"
)

// stillActive is the WinAPI sentinel returned by GetExitCodeProcess for
// running processes (STILL_ACTIVE = 259).
const stillActive uint32 = 259

// Alive reports whether a process with the given PID is currently running.
// Returns false for non-positive PIDs without performing any syscall.
//
// On Windows this opens the process with PROCESS_QUERY_LIMITED_INFORMATION
// (least privilege required) and calls GetExitCodeProcess. A return code of
// STILL_ACTIVE means the process is running; any other value means it has
// exited. If OpenProcess fails with ERROR_INVALID_PARAMETER the PID is
// unknown to the system (process never existed or has been recycled).
//
// Any other unexpected error is treated as "alive" defensively, so we never
// steal a lock based on an ambiguous syscall failure.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return false
		}
		// Access denied means the process exists but we can't query it.
		if err == windows.ERROR_ACCESS_DENIED {
			return true
		}
		// Other errors: be conservative, treat as alive.
		return true
	}
	defer windows.CloseHandle(h)

	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return true
	}
	return code == stillActive
}
