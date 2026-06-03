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
	"testing"
	"time"
)

func newMachineAt(now time.Time) *Machine {
	m := NewMachine()
	m.now = func() time.Time { return now }
	return m
}

func TestMachine_InitialState(t *testing.T) {
	m := NewMachine()
	if m.State() != StateDisconnected {
		t.Fatalf("initial state = %s, want %s", m.State(), StateDisconnected)
	}
	snap := m.Snapshot()
	if snap.StateSource != SourceInferred {
		t.Fatalf("v1 must emit StateSource=inferred, got %s", snap.StateSource)
	}
}

func TestMachine_TransitionsHappyPath(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	m := newMachineAt(now)

	m.OnConnecting()
	if m.State() != StateConnecting {
		t.Fatalf("after OnConnecting: %s", m.State())
	}

	m.OnConnected()
	if m.State() != StateConnected {
		t.Fatalf("after OnConnected: %s", m.State())
	}

	m.OnEvent()
	if m.State() != StateConnected {
		t.Fatalf("after OnEvent: %s (should remain connected)", m.State())
	}

	m.OnStopped()
	if m.State() != StateStopped {
		t.Fatalf("after OnStopped: %s", m.State())
	}
}

func TestMachine_IdleAfterQuietWindow(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	m := newMachineAt(now)
	m.OnConnected()
	m.OnEvent() // lastEventAt = now

	// Advance time past IdleAfter
	m.now = func() time.Time { return now.Add(IdleAfter + 5*time.Second) }
	if got := m.State(); got != StateIdle {
		t.Fatalf("state after quiet window: %s, want %s", got, StateIdle)
	}

	// New event resets to connected
	m.OnEvent()
	if got := m.State(); got != StateConnected {
		t.Fatalf("state after new event: %s, want %s", got, StateConnected)
	}
}

func TestMachine_DegradedWhenReconnectsButNoEvents(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	m := newMachineAt(now)
	m.OnConnected()
	m.OnEvent() // baseline

	// Advance + reconnect signal (no event recovery)
	later := now.Add(IdleAfter + 30*time.Second)
	m.now = func() time.Time { return later }
	m.OnReconnect() // sets state=reconnecting, lastReconnectAt=later

	// State while reconnecting
	if got := m.State(); got != StateReconnecting {
		t.Fatalf("after OnReconnect: %s, want %s", got, StateReconnecting)
	}

	// Push state back to connected (as if reconnect succeeded but no traffic)
	m.OnConnected()

	// Now idle window passes with no events but a recent reconnect → degraded
	muchLater := later.Add(IdleAfter + 5*time.Second)
	m.now = func() time.Time { return muchLater }
	if got := m.State(); got != StateDegraded {
		t.Fatalf("state after reconnect-but-no-events: %s, want %s", got, StateDegraded)
	}
}

func TestMachine_ReconnectCount(t *testing.T) {
	m := NewMachine()
	m.OnConnected()
	m.OnReconnect()
	m.OnReconnect()
	m.OnReconnect()
	if snap := m.Snapshot(); snap.ReconnectCount != 3 {
		t.Fatalf("ReconnectCount = %d, want 3", snap.ReconnectCount)
	}
}

func TestMachine_OnEventClearsReconnecting(t *testing.T) {
	m := NewMachine()
	m.OnConnected()
	m.OnReconnect()
	if m.State() != StateReconnecting {
		t.Fatal("expected StateReconnecting after OnReconnect")
	}
	m.OnEvent()
	if m.State() != StateConnected {
		t.Fatalf("OnEvent should clear Reconnecting, got %s", m.State())
	}
}
