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

//go:build !windows

package process

import (
	"errors"
	"syscall"
)

// Alive reports whether a process with the given PID is currently running.
// Returns false for non-positive PIDs without performing any syscall.
//
// On Unix this uses signal 0 (syscall.Kill(pid, 0)), which performs the
// normal permission/existence check without delivering a signal:
//   - nil error            → process exists and we have permission
//   - syscall.EPERM        → process exists but owned by another user
//     (still counts as alive for our purpose)
//   - syscall.ESRCH        → no such process
//
// Any other error is treated as "alive" defensively, so we never steal a
// lock based on an ambiguous syscall failure.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) {
		// Process exists, we just can't signal it.
		return true
	}
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	// Unknown error: be conservative, treat as alive.
	return true
}
