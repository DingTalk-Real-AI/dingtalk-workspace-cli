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

// Package bus implements the daemon side of the dws event subsystem: one
// long-lived process per ClientID that holds the single cloud connection
// and fans out events to N local consumers over IPC.
//
// Files (mirroring plan §5 layout):
//
//	daemon.go    main loop: lock → meta → IPC listen → ready → source.Start
//	hub.go       consumer registry, per-consumer sendCh, drop-oldest backpressure
//	metrics.go   per-event-type + per-consumer received/dropped counters
//	lockfile.go  single bus.lock (flock + PID content + stale recovery)
//	meta.go      bus.meta JSON (clientID/edition/started_at) for list/status reverse mapping
//
// Lifecycle invariants (plan §4 invariants 1–7):
//  1. emit non-blocking (drop-oldest, never block SDK callback)
//  2. dedup on event_id (LRU) to absorb cloud-side redelivery
//  3. single bus per ClientID (bus.lock enforces, all FS paths use clientIDHash)
//  4. upstream always full subscription; consumer filter only affects bus→consume
//  5. dead-consumer auto-reap on socket EOF
//  6. startup order: lock → meta → IPC listen → ready pipe → Source.Start
//  7. stdio detach when fork'd by busctl/spawn
package bus
