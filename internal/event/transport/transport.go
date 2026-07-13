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

package transport

import "net"

// Listener is the bus-side accept loop. Implementations bind a Unix socket
// (darwin/linux) or Windows Named Pipe at the given Endpoint; calling Close
// removes the underlying socket file on Unix.
type Listener interface {
	// Accept blocks until a peer connects or the listener is closed.
	Accept() (net.Conn, error)
	// Close unbinds the endpoint and unblocks pending Accept calls.
	Close() error
	// Endpoint returns the human-readable address (Unix path or Windows pipe
	// name) — only used in log messages and status output.
	Endpoint() string
}

// Listen binds an IPC endpoint at path. On Unix path is a filesystem socket
// path (caller must ensure the parent directory exists with mode 0700).
// On Windows path is a Named Pipe name like `\\.\pipe\dws-event-<edition>-<hash>`.
//
// Stale Unix sockets (left behind by a crashed bus) are unlinked
// automatically before bind. Caller MUST hold the bus.lock before calling
// Listen so this unlink is race-safe against a still-running bus.
func Listen(path string) (Listener, error) {
	return listen(path)
}

// Dial connects to an IPC endpoint at path. Returns a standard net.Conn
// the caller can wrap with NewReader/NewWriter. Closes are independent —
// closing the conn does not close the listener.
func Dial(path string) (net.Conn, error) {
	return dial(path)
}
