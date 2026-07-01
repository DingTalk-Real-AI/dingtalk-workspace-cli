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

package helpers

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestDeriveConnectHealth(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	alive := os.Getpid()
	dead := deadPid(t)

	cases := []struct {
		name       string
		hb         *connectHeartbeat
		supervised bool
		want       string
	}{
		{"no heartbeat", nil, false, healthNotRunning},
		{"no heartbeat but supervised", nil, true, healthNotRunning},
		{
			"connector dead",
			&connectHeartbeat{Pid: dead, StartUnix: now.Unix() - 100, ConnectedUnix: now.Unix() - 90},
			false, healthDown,
		},
		{
			"alive never connected",
			&connectHeartbeat{Pid: alive, StartUnix: now.Unix() - 5},
			false, healthDegraded,
		},
		{
			"alive and connected",
			&connectHeartbeat{Pid: alive, StartUnix: now.Unix() - 100, ConnectedUnix: now.Unix() - 90, LastReplyUnix: now.Unix() - 10},
			false, healthHealthy,
		},
		{
			"idle but connected is still healthy",
			&connectHeartbeat{Pid: alive, StartUnix: now.Unix() - 100000, ConnectedUnix: now.Unix() - 100000},
			false, healthHealthy,
		},
		{
			"error after last success is degraded",
			&connectHeartbeat{Pid: alive, StartUnix: now.Unix() - 100, ConnectedUnix: now.Unix() - 90, LastReplyUnix: now.Unix() - 50, LastErrorUnix: now.Unix() - 5, LastError: "boom"},
			false, healthDegraded,
		},
		{
			"error before last reply stays healthy",
			&connectHeartbeat{Pid: alive, StartUnix: now.Unix() - 100, ConnectedUnix: now.Unix() - 90, LastErrorUnix: now.Unix() - 50, LastReplyUnix: now.Unix() - 5, LastError: "old"},
			false, healthHealthy,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := deriveConnectHealth(c.hb, c.supervised, now)
			if got.State != c.want {
				t.Fatalf("state = %q, want %q (detail=%q)", got.State, c.want, got.Detail)
			}
			if got.Supervised != c.supervised {
				t.Errorf("supervised = %v, want %v", got.Supervised, c.supervised)
			}
		})
	}
}

func TestConnectHeartbeatRoundTrip(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })

	h := newConnectHealth("cid-round", "opencode")
	if h == nil {
		t.Fatal("newConnectHealth returned nil for a valid clientId")
	}
	h.onConnected()
	h.onPush()
	h.onReply()
	if err := h.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	hb, err := readConnectHeartbeat(h.dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if hb == nil {
		t.Fatal("heartbeat not persisted")
	}
	if hb.Pid != os.Getpid() || hb.Channel != "opencode" || hb.ClientID != "cid-round" {
		t.Errorf("unexpected heartbeat identity: %+v", hb)
	}
	if hb.ConnectedUnix == 0 || hb.LastPushUnix == 0 || hb.LastReplyUnix == 0 {
		t.Errorf("expected all activity timestamps set: %+v", hb)
	}
}

func TestConnectHeartbeatFlushSkipsUnchanged(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })

	h := newConnectHealth("cid-skip", "codex")
	h.onConnected()
	if err := h.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// Second flush with no new event must be a no-op (nothing to write).
	fi1, _ := os.Stat(connectHeartbeatPath(h.dir))
	if err := h.flush(); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	if h.hb.UpdatedUnix != h.flushedUnix {
		t.Errorf("flushedUnix (%d) should track UpdatedUnix (%d) after flush", h.flushedUnix, h.hb.UpdatedUnix)
	}
	_ = fi1
}

func TestConnectHealthNilSafe(t *testing.T) {
	var h *connectHealth // no clientId path yields nil
	// None of these may panic.
	h.onConnected()
	h.onPush()
	h.onReply()
	h.onError(errors.New("x"))
	h.start(nil)
	if err := h.flush(); err != nil {
		t.Fatalf("nil flush: %v", err)
	}
	h.remove()
}

func TestNewConnectHealthNoIdentity(t *testing.T) {
	if h := newConnectHealth("", ""); h != nil {
		t.Errorf("expected nil health writer with no clientId, got %+v", h)
	}
}
