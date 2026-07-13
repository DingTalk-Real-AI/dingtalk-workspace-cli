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

// Package transport implements the cross-platform local IPC between the bus
// daemon and consume clients. The wire is \n-delimited JSON frames over a
// Unix domain socket (darwin/linux) or Windows Named Pipe; the abstraction
// keeps both endpoints (bus side / consume side) and both platforms behind
// one Listener/Dialer interface.
//
// Frame protocol (see plan §9):
//
//	consume → bus    hello, bye(reason=client_done), heartbeat, status_req
//	bus → consume    hello_ack, event, source_state, bye(reason=shutdown),
//	                 heartbeat, status_resp
//
// Frames are independent JSON objects; no length prefix, no checksum.
// Maximum single-frame size is enforced by the Reader to bound memory and
// prevent a buggy/malicious peer from causing OOM (MaxFrameBytes).
package transport
