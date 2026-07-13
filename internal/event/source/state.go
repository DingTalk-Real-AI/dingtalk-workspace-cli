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

package source

import (
	"sync"
	"time"
)

// State enumerates the connection states surfaced by `dws event status`.
//
// Stream SDK v0.9.1 does not expose a user-registerable ConnectionState
// hook (verified P0 §1 escape-hatch table row #4), so the state machine
// runs in "inferred" mode: it derives state from observable signals —
// Start() return value, last-event-received timestamp, last-error timestamp,
// SDK reconnect log lines parsed by the source layer. The Source field
// `state_source` (env-style annotation) is emitted alongside the state so
// downstream tools can distinguish authoritative from inferred values when
// SDK hook support arrives in a future release.
type State string

const (
	// StateDisconnected is the zero state before Start has been called.
	StateDisconnected State = "disconnected"
	// StateConnecting is set on entry to Start, before SDK dial completes.
	StateConnecting State = "connecting"
	// StateConnected is set when Start returns nil (dial succeeded).
	StateConnected State = "connected"
	// StateIdle is set when the connection is still up but no event has been
	// observed for a quiet window (default 60s). Distinguishes "everything
	// fine, just no traffic" from "looks connected but broken".
	StateIdle State = "idle"
	// StateReconnecting is set when the SDK has signalled a reconnect (via
	// log line parse or future hook). Cleared on next event received.
	StateReconnecting State = "reconnecting"
	// StateDegraded is set when reconnects keep happening but no events flow.
	// Useful to surface "looks alive, actually broken" cases via status.
	StateDegraded State = "degraded"
	// StateStopped is set after Close() / context cancellation.
	StateStopped State = "stopped"
)

// SourceKind identifies whether the state was reported by an authoritative
// SDK hook or inferred from observable side-channels.
type SourceKind string

const (
	// SourceHook is reserved for future SDK versions that expose
	// ConnectionState / keepalive callbacks. v1 does not emit this.
	SourceHook SourceKind = "hook"
	// SourceInferred is the only value emitted by v1.
	SourceInferred SourceKind = "inferred"
)

// IdleAfter is the duration of no events after which StateConnected
// transitions to StateIdle. Exposed for tests to bypass real-time waits.
var IdleAfter = 60 * time.Second

// Machine tracks the connection state. Methods are safe for concurrent use.
type Machine struct {
	mu              sync.RWMutex
	state           State
	source          SourceKind
	lastEventAt     time.Time
	lastReconnectAt time.Time
	reconnectCount  int
	now             func() time.Time
}

// NewMachine returns a Machine in the StateDisconnected initial state.
func NewMachine() *Machine {
	return &Machine{
		state:  StateDisconnected,
		source: SourceInferred,
		now:    time.Now,
	}
}

// State returns the current effective state, applying time-based idle
// detection on top of the stored state. This is the public read accessor
// used by `event status` formatting.
func (m *Machine) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.effectiveStateLocked()
}

// Snapshot returns the full state view for status rendering: state +
// source + last event/reconnect timestamps + reconnect count.
type Snapshot struct {
	State           State      `json:"state"`
	StateSource     SourceKind `json:"state_source"`
	LastEventAt     time.Time  `json:"last_event_at,omitempty"`
	LastReconnectAt time.Time  `json:"last_reconnect_at,omitempty"`
	ReconnectCount  int        `json:"reconnect_count"`
}

// Snapshot returns the current state view atomically.
func (m *Machine) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Snapshot{
		State:           m.effectiveStateLocked(),
		StateSource:     m.source,
		LastEventAt:     m.lastEventAt,
		LastReconnectAt: m.lastReconnectAt,
		ReconnectCount:  m.reconnectCount,
	}
}

// OnConnecting moves to StateConnecting. Called by Source.Start before the
// SDK dial completes.
func (m *Machine) OnConnecting() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = StateConnecting
}

// OnConnected moves to StateConnected. Called when SDK Start returns nil.
func (m *Machine) OnConnected() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = StateConnected
}

// OnEvent records the timestamp of the most recently received event. Resets
// any Reconnecting or Idle state back to Connected (the connection is
// demonstrably alive).
func (m *Machine) OnEvent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastEventAt = m.now()
	if m.state == StateReconnecting || m.state == StateIdle || m.state == StateDegraded {
		m.state = StateConnected
	}
}

// OnReconnect marks a reconnect signal observed (from SDK log or future
// hook). Increments the counter and stamps the time.
func (m *Machine) OnReconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastReconnectAt = m.now()
	m.reconnectCount++
	m.state = StateReconnecting
}

// OnStopped marks the SDK as closed (graceful exit or Close()).
func (m *Machine) OnStopped() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = StateStopped
}

// effectiveStateLocked applies idle-detection on top of the stored state.
// Caller must hold m.mu (read or write).
func (m *Machine) effectiveStateLocked() State {
	switch m.state {
	case StateConnected:
		// Connected → Idle after IdleAfter with no events. If we've also
		// seen recent reconnects with no recovered events, surface Degraded.
		if !m.lastEventAt.IsZero() && m.now().Sub(m.lastEventAt) >= IdleAfter {
			if m.reconnectCount > 0 && m.lastReconnectAt.After(m.lastEventAt) {
				return StateDegraded
			}
			return StateIdle
		}
	}
	return m.state
}
