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

// Package busctl glues the consume client to the bus daemon. It encapsulates
// the three operations a consumer needs at startup and shutdown:
//
//	discover: find the running bus or fork a fresh one (race-free, plan §12
//	          P3 "try dial → try fork lock → retry dial")
//	spawn:    exec `dws event _bus --client-id <id>` as a detached
//	          background process (stdio detach, setsid, ready pipe handshake)
//	stop:     gracefully terminate the bus daemon (SIGTERM + IPC fallback)
//
// All three operations are short-lived helpers — they own no long-lived
// goroutines and return whole errors to their caller.
package busctl
