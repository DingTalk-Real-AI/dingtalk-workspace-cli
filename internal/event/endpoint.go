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

package event

import (
	"os"
	"path/filepath"
	"runtime"
)

// MaxUnixSocketPath returns the longest Unix socket path accepted by
// bind/connect on this OS (Go rejects longer names with EINVAL before
// the syscall). sockaddr_un.sun_path is 104 bytes on darwin and the
// BSDs and 108 on Linux; the usable budget is one less.
func MaxUnixSocketPath() int {
	if runtime.GOOS == "linux" {
		return 107
	}
	return 103
}

// IPCEndpoint returns the bus IPC endpoint for one identity: a Named Pipe
// name on Windows, otherwise bus.sock inside workDir.
//
// The canonical Unix location is <workDir>/bus.sock, but workDir derives
// from the config dir, which can be arbitrarily deep (e.g. dwssb sandboxes
// use ~/.dwssb/sandboxes/<name>/config/...). When the canonical path would
// exceed the OS sun_path limit, the socket falls back to a short
// deterministic path under os.TempDir keyed by a hash of workDir, so every
// process (consume parent, forked _bus child, status/stop tooling) that
// derives the endpoint from the same workDir agrees on the location.
// bus.lock / bus.meta / bus.log always stay in workDir — only the socket
// moves.
//
// This is the single source of truth for endpoint derivation; the cobra
// layer and busctl must not re-implement the shape.
func IPCEndpoint(workDir, editionName string, sourceKind SourceKind, identityHash string) string {
	if sourceKind == "" {
		sourceKind = SourceKindAppStream
	}
	if runtime.GOOS == "windows" {
		return `\\.\pipe\dws-event-` + editionName + "-" + string(sourceKind) + "-" + identityHash
	}
	sock := filepath.Join(workDir, "bus.sock")
	if len(sock) <= MaxUnixSocketPath() {
		return sock
	}
	return filepath.Join(os.TempDir(), "dws-evt-"+IdentityHash(workDir)+".sock")
}
