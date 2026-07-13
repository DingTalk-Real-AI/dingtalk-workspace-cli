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

// Package lock implements a cross-platform exclusive file lock primitive used
// by the bus daemon to enforce the "single bus per ClientID" invariant
// (plan invariant #3). The primitive is intentionally tiny — it acquires and
// releases a non-blocking exclusive lock on an opened file handle, with no
// knowledge of PID files or business semantics. Higher layers (bus/lockfile.go)
// combine this primitive with PID content read/write to provide the full
// single-file bus.lock design.
//
// Unix: syscall.Flock(LOCK_EX|LOCK_NB).
// Windows: windows.LockFileEx with LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY.
package lock
