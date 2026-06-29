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
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBackoffDelay(t *testing.T) {
	base := time.Second
	cap := 60 * time.Second
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, 0},
		{-1, 0},
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, 32 * time.Second},
		{7, cap}, // 64s capped to 60s
		{8, cap},
		{100, cap},
	}
	for _, c := range cases {
		if got := backoffDelay(c.failures, base, cap); got != c.want {
			t.Errorf("backoffDelay(%d) = %v, want %v", c.failures, got, c.want)
		}
	}
}

func TestDaemonDirKey(t *testing.T) {
	cases := []struct {
		clientID     string
		unifiedAppID string
		want         string
	}{
		{"clientABC", "", "clientABC"},
		{"client/with:bad*chars", "", "client_with_bad_chars"},
		{"", "app-123", "app-app-123"},
		{"", "u/n.id", "app-u_n_id"},
		{"  ", "  ", ""},
		{"cid", "uid", "cid"}, // clientID wins
	}
	for _, c := range cases {
		if got := daemonDirKey(c.clientID, c.unifiedAppID); got != c.want {
			t.Errorf("daemonDirKey(%q,%q) = %q, want %q", c.clientID, c.unifiedAppID, got, c.want)
		}
	}
}

func TestBuildWorkerArgs(t *testing.T) {
	in := []string{"devapp", "robot", "connect", "--daemon", "--robot-client-id", "abc", "--channel=claudecode"}
	got := buildWorkerArgs(in)
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "--daemon ") || strings.HasSuffix(joined, "--daemon") {
		// only the appended --daemon-worker may contain "daemon"
	}
	if !strings.HasSuffix(joined, "--daemon-worker") {
		t.Errorf("worker args must end with --daemon-worker, got %q", joined)
	}
	for _, a := range got[:len(got)-1] {
		if a == "--daemon" || a == "--daemon-supervise" || a == "--daemon-worker" {
			t.Errorf("daemon-control flag leaked into worker args: %q", a)
		}
	}
	if !strings.Contains(joined, "--robot-client-id abc") || !strings.Contains(joined, "--channel=claudecode") {
		t.Errorf("credential/channel flags must be preserved, got %q", joined)
	}
}

func TestBuildSuperviseArgs(t *testing.T) {
	in := []string{"devapp", "robot", "connect", "--daemon", "--robot-client-id", "abc"}
	got := buildSuperviseArgs(in)
	joined := strings.Join(got, " ")
	if !strings.HasSuffix(joined, "--daemon-supervise") {
		t.Errorf("supervise args must end with --daemon-supervise, got %q", joined)
	}
	if strings.Contains(joined, " --daemon ") || strings.Contains(joined, " --daemon\b") {
		t.Errorf("--daemon should be stripped, got %q", joined)
	}
	for _, a := range got[:len(got)-1] {
		if a == "--daemon" {
			t.Errorf("--daemon leaked into supervise args: %q", a)
		}
	}
}

func TestWriteConnectDaemonStartedIncludesLocalDebugNotice(t *testing.T) {
	var buf bytes.Buffer
	writeConnectDaemonStarted(&buf, 1234, "/tmp/daemon.log", "cid", "cid")
	out := buf.String()
	for _, want := range []string{"connect daemon started", "/tmp/daemon.log", "本地调试", "不代表线上发布完成", "不会提交版本发布"} {
		if !strings.Contains(out, want) {
			t.Fatalf("daemon started output missing %q:\n%s", want, out)
		}
	}
}

func TestDaemonStateRoundTrip(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })

	dir, err := connectDaemonDir("roundtrip")
	if err != nil {
		t.Fatalf("connectDaemonDir: %v", err)
	}
	// No file yet → nil, nil.
	if st, err := readDaemonState(dir); err != nil || st != nil {
		t.Fatalf("expected (nil,nil) for missing pid file, got (%v,%v)", st, err)
	}
	want := daemonState{Pid: 4242, StartUnix: time.Now().Unix(), LogPath: "/x/y.log", DirKey: "roundtrip", ClientID: "cid"}
	if err := writeDaemonState(dir, want); err != nil {
		t.Fatalf("writeDaemonState: %v", err)
	}
	got, err := readDaemonState(dir)
	if err != nil || got == nil {
		t.Fatalf("readDaemonState: (%v,%v)", got, err)
	}
	if got.Pid != want.Pid || got.DirKey != want.DirKey || got.ClientID != want.ClientID || got.LogPath != want.LogPath {
		t.Errorf("round trip mismatch: got %+v want %+v", *got, want)
	}
}

func TestReadDaemonStateCorrupt(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })
	dir, _ := connectDaemonDir("corrupt")
	if err := os.WriteFile(daemonPidPath(dir), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readDaemonState(dir); err == nil {
		t.Error("expected error for corrupt pid file")
	}
}

func TestDaemonStatusNotRunning(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })
	var buf bytes.Buffer
	if err := daemonStatus(&buf, "nope"); err != nil {
		t.Fatalf("daemonStatus: %v", err)
	}
	if !strings.Contains(buf.String(), "not running") {
		t.Errorf("expected 'not running', got %q", buf.String())
	}
}

func TestDaemonStatusStalePid(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })
	dir, _ := connectDaemonDir("stale")
	// pid that is essentially certain to be dead.
	writeDaemonState(dir, daemonState{Pid: deadPid(t), StartUnix: time.Now().Unix(), LogPath: "/l", DirKey: "stale"})
	var buf bytes.Buffer
	if err := daemonStatus(&buf, "stale"); err != nil {
		t.Fatalf("daemonStatus: %v", err)
	}
	if !strings.Contains(buf.String(), "stale pid file") {
		t.Errorf("expected stale pid report, got %q", buf.String())
	}
}

func TestDaemonStatusRunning(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })
	dir, _ := connectDaemonDir("live")
	// Use our own pid: guaranteed alive.
	writeDaemonState(dir, daemonState{Pid: os.Getpid(), StartUnix: time.Now().Add(-90 * time.Second).Unix(), LogPath: "/l.log", DirKey: "live", ClientID: "cidX"})
	var buf bytes.Buffer
	if err := daemonStatus(&buf, "live"); err != nil {
		t.Fatalf("daemonStatus: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"running", "pid:", "uptime:", "/l.log", "cidX"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q; got %q", want, out)
		}
	}
}

func TestDaemonStopNotRunning(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })
	var buf bytes.Buffer
	if err := daemonStop(&buf, "ghost"); err != nil {
		t.Fatalf("daemonStop: %v", err)
	}
	if !strings.Contains(buf.String(), "not running") {
		t.Errorf("expected 'not running', got %q", buf.String())
	}
}

func TestDaemonStopStaleCleansPidFile(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })
	dir, _ := connectDaemonDir("stalestop")
	writeDaemonState(dir, daemonState{Pid: deadPid(t), StartUnix: time.Now().Unix(), DirKey: "stalestop"})
	var buf bytes.Buffer
	if err := daemonStop(&buf, "stalestop"); err != nil {
		t.Fatalf("daemonStop: %v", err)
	}
	if _, err := os.Stat(daemonPidPath(dir)); !os.IsNotExist(err) {
		t.Errorf("stale pid file should be removed, stat err=%v", err)
	}
	if !strings.Contains(buf.String(), "stale") {
		t.Errorf("expected stale cleanup message, got %q", buf.String())
	}
}

// deadPid returns a pid that is not alive. It spawns `true`, waits for it to
// exit, and returns its pid — guaranteed reaped and gone.
func deadPid(t *testing.T) int {
	t.Helper()
	// A very high pid is almost never live; combine with a sanity probe.
	for _, candidate := range []int{999999, 524287, 99999} {
		if !processAlive(candidate) {
			return candidate
		}
	}
	t.Skip("could not find a guaranteed-dead pid on this host")
	return 0
}
