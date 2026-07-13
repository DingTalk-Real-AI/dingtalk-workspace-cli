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

// Package event implements the DingTalk Stream event subscription pipeline for
// dws. The architecture is a single-cloud-connection bus daemon (one per
// ClientID) plus N local consumer processes communicating over Unix socket /
// Windows Named Pipe. The bus keeps one cloud connection per identity while
// exposing observable connection state, per-event-type metrics, and Hello-time
// filter pushdown to local consumers.
//
// Package layout:
//
//	event/             // top-level types (RawEvent, EmitFn, hash helpers)
//	event/dedup/       // event_id LRU dedup
//	event/registry/    // catch-all event types + compact processor registry
//	event/source/      // wrap dingtalk-stream-sdk-go + connection state machine
//	event/bus/         // daemon loop, hub, metrics, lockfile, meta
//	event/transport/   // UDS/Pipe abstraction, frame protocol
//	event/busctl/      // discover, spawn, stop helpers
//	event/consume/     // consumer-side pipeline, formatter, router, sink
//	event/lock/        // cross-platform flock primitive (Unix flock / Windows LockFileEx)
//	event/process/     // cross-platform process-alive check (Unix signal 0 / Windows OpenProcess)
//
// See plans/2026-05-28_event_capability_v1.plan.md for the full design,
// invariants, and protocol.
package event
