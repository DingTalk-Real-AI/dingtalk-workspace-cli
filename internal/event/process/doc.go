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

// Package process provides cross-platform process-existence checks. Used by
// the bus daemon's stale-lock recovery path: bus.lock holds the daemon's PID;
// on startup a competing process reads that PID and calls Alive() to decide
// whether to abort (the holder is still running) or steal the lock (orphan).
//
// On Unix this is the standard signal-0 trick (syscall.Kill(pid, 0)) which
// errors with ESRCH if the process is gone. On Windows it uses OpenProcess
// with PROCESS_QUERY_LIMITED_INFORMATION and GetExitCodeProcess to detect
// STILL_ACTIVE; this avoids the Unix-only assumption in plan §1.
package process
