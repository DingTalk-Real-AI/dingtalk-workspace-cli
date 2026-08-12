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
	"strconv"
	"strings"
)

const eventRuntimeDirPrefix = "dws-event-"

func currentUserID() string {
	return strconv.Itoa(os.Geteuid())
}

// MaxUnixSocketPath returns the longest Unix socket path accepted by
// bind/connect on this OS (Go rejects longer names with EINVAL before
// the syscall). sockaddr_un.sun_path is 104 bytes on darwin and the
// BSDs and 108 on Linux; the usable budget is one less.
func MaxUnixSocketPath() int {
	return maxUnixSocketPath(runtime.GOOS)
}

func maxUnixSocketPath(goos string) int {
	if goos == "linux" {
		return 107
	}
	return 103
}

// IPCEndpoint returns the bus IPC endpoint for one identity: a Named Pipe
// name on Windows, otherwise a deterministic Unix socket under a private
// per-user runtime directory.
//
// Unix sockets must live on a local filesystem that supports bind(2).
// Config directories may reside on NFS, CSI, FUSE, or other shared mounts
// that reject Unix socket creation with ENOTSUPP. On Unix, the endpoint uses
// XDG_RUNTIME_DIR when it is absolute and short enough; otherwise it falls
// back to a per-UID directory under os.TempDir. The transport creates and
// validates that directory as owner-only before listening or dialing. The
// socket name is keyed by a hash of workDir so every process (consume parent,
// forked _bus child, status/stop tooling) that derives the endpoint from the
// same workDir agrees on the location. bus.lock / bus.meta / bus.log always
// stay in workDir.
//
// This is the single source of truth for endpoint derivation; the cobra
// layer and busctl must not re-implement the shape.
func IPCEndpoint(workDir, editionName string, sourceKind SourceKind, identityHash string) string {
	return ipcEndpointForOS(runtime.GOOS, workDir, editionName, sourceKind, identityHash)
}

func ipcEndpointForOS(goos, workDir, editionName string, sourceKind SourceKind, identityHash string) string {
	if sourceKind == "" {
		sourceKind = SourceKindAppStream
	}
	if goos == "windows" {
		return `\\.\pipe\dws-event-` + editionName + "-" + string(sourceKind) + "-" + identityHash
	}
	return unixSocketEndpoint(goos, workDir, strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")), os.TempDir())
}

func unixSocketEndpoint(goos, workDir, runtimeDir, tempDir string) string {
	socketName := "dws-evt-" + IdentityHash(workDir) + ".sock"
	userDirName := eventRuntimeDirPrefix + currentUserID()
	if filepath.IsAbs(runtimeDir) {
		candidate := filepath.Join(runtimeDir, userDirName, socketName)
		if len(candidate) <= maxUnixSocketPath(goos) {
			return candidate
		}
	}
	fallback := filepath.Join(tempDir, userDirName, socketName)
	if len(fallback) <= maxUnixSocketPath(goos) {
		return fallback
	}
	return filepath.Join("/tmp", userDirName, socketName)
}
