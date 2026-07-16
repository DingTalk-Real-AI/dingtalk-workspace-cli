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

package bus

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// fakeSource is a minimal SourceAdapter that emits a configured slice of
// events then blocks on ctx until cancel. Used by daemon integration tests
// in place of the real Stream SDK.
//
// If trigger is non-nil, fakeSource waits for it to close before emitting.
// Tests use this to register a consumer first (avoids the race where events
// flow before Hello completes and end up dropped by Hub for lack of a
// matching consumer).
type fakeSource struct {
	events  []dwsevent.RawEvent
	delay   time.Duration   // optional delay between emits to let the consumer drain
	trigger <-chan struct{} // optional gate; nil = emit immediately
}

func (f *fakeSource) Start(ctx context.Context, emit dwsevent.EmitFn) error {
	if f.trigger != nil {
		select {
		case <-f.trigger:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for i := range f.events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		ev := f.events[i]
		ev.ReceivedAt = time.Now().UTC()
		emit(&ev)
		if f.delay > 0 {
			time.Sleep(f.delay)
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func skipOnWindows(t *testing.T, reason string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skipf("skip on windows: %s", reason)
	}
}

// shortTempDir returns a temp dir under /tmp so the resulting unix socket
// path stays under the macOS 104-byte sun_path limit. t.TempDir() lives in
// $TMPDIR (/var/folders/.../T/...) which can easily exceed that.
func shortTempDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return t.TempDir()
	}
	dir, err := os.MkdirTemp("/tmp", "dws-bus-")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// waitForFile polls until path exists or timeout elapses.
func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("file %q did not appear within %s", path, timeout)
}

func TestDaemon_RunStartsAndShutsDownCleanly(t *testing.T) {
	skipOnWindows(t, "Unix socket path semantics differ; covered by transport_windows_test")
	workDir := shortTempDir(t)
	sockPath := filepath.Join(workDir, "bus.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := &fakeSource{}
	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(ctx, Config{
			WorkDir:     workDir,
			IPCEndpoint: sockPath,
			ClientID:    "ding_test",
			Edition:     "open",
			Source:      src,
		})
	}()
	waitForFile(t, sockPath, 2*time.Second)

	if pid := ReadHolderPID(filepath.Join(workDir, LockFileName)); pid != os.Getpid() {
		t.Errorf("bus.lock pid = %d, want %d", pid, os.Getpid())
	}
	if _, err := ReadMeta(workDir); err != nil {
		t.Errorf("bus.meta missing: %v", err)
	}

	cancel()
	select {
	case err := <-runDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want nil or canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

func TestDaemon_ConsumerReceivesEvents(t *testing.T) {
	skipOnWindows(t, "uses Unix socket dial")
	workDir := shortTempDir(t)
	sockPath := filepath.Join(workDir, "bus.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trigger := make(chan struct{})
	src := &fakeSource{
		events: []dwsevent.RawEvent{
			{EventID: "1", EventType: "im.message.receive_v1", Data: `{"text":"hi"}`},
			{EventID: "2", EventType: "approval.task", Data: `{"task":"x"}`},
			{EventID: "3", EventType: "im.message.at_v1", Data: `{"at":1}`},
		},
		delay:   5 * time.Millisecond,
		trigger: trigger,
	}
	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(ctx, Config{
			WorkDir:     workDir,
			IPCEndpoint: sockPath,
			ClientID:    "ding_test",
			Edition:     "open",
			Source:      src,
		})
	}()
	waitForFile(t, sockPath, 2*time.Second)

	conn, err := transport.Dial(sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	w := transport.NewWriter(conn)
	r := transport.NewReader(conn)
	if err := w.WriteJSON(transport.Hello{
		Type:        transport.FrameTypeHello,
		ConsumerPID: os.Getpid(),
		EventTypes:  []string{"im.*"},
	}); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	var ack transport.HelloAck
	if err := r.ReadJSON(&ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.Type != transport.FrameTypeHelloAck {
		t.Fatalf("ack type = %s", ack.Type)
	}

	// Consumer is now registered; trigger fakeSource to emit.
	close(trigger)

	received := 0
	deadline := time.After(3 * time.Second)
	for received < 2 {
		select {
		case <-deadline:
			t.Fatalf("only received %d events, want 2", received)
		default:
		}
		raw, err := r.Read()
		if err != nil {
			t.Fatalf("read frame: %v", err)
		}
		typ, _ := transport.PeekType(raw)
		if typ == transport.FrameTypeEvent {
			received++
		}
	}

	cancel()
	<-runDone
}

func TestDaemon_LockBusyOnSecondRun(t *testing.T) {
	skipOnWindows(t, "uses Unix socket / flock semantics")
	workDir := shortTempDir(t)
	sockPath := filepath.Join(workDir, "bus.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = Run(ctx, Config{
			WorkDir:     workDir,
			IPCEndpoint: sockPath,
			ClientID:    "ding_test",
			Edition:     "open",
			Source:      &fakeSource{},
		})
	}()
	waitForFile(t, sockPath, 2*time.Second)

	err := Run(context.Background(), Config{
		WorkDir:     workDir,
		IPCEndpoint: sockPath + ".other",
		ClientID:    "ding_test",
		Edition:     "open",
		Source:      &fakeSource{},
	})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("second Run = %v, want ErrBusy", err)
	}
}

func TestDaemon_IdleTimeoutSelfStops(t *testing.T) {
	skipOnWindows(t, "uses Unix socket")
	workDir := shortTempDir(t)
	sockPath := filepath.Join(workDir, "bus.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(ctx, Config{
			WorkDir:     workDir,
			IPCEndpoint: sockPath,
			ClientID:    "ding_test",
			Edition:     "open",
			Source:      &fakeSource{},
			IdleTimeout: 200 * time.Millisecond,
		})
	}()
	waitForFile(t, sockPath, 2*time.Second)

	select {
	case <-runDone:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("bus did not idle-stop within deadline")
	}
}

func TestDaemon_ReadyPipeSignalsR(t *testing.T) {
	skipOnWindows(t, "uses Unix socket + pipe")
	workDir := shortTempDir(t)
	sockPath := filepath.Join(workDir, "bus.sock")

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Run(ctx, Config{
		WorkDir:     workDir,
		IPCEndpoint: sockPath,
		ClientID:    "ding_test",
		Edition:     "open",
		Source:      &fakeSource{},
		ReadyPipe:   pw,
	})

	buf := make([]byte, 1)
	pr.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := pr.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("read ready pipe: %v", err)
	}
	if n != 1 || buf[0] != 'R' {
		t.Fatalf("ready byte = %q n=%d, want 'R'", buf, n)
	}
	cancel()
}

func TestDaemon_ConsumerEOFAutoUnregisters(t *testing.T) {
	skipOnWindows(t, "uses Unix socket")
	workDir := shortTempDir(t)
	sockPath := filepath.Join(workDir, "bus.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readyReader.Close()

	go Run(ctx, Config{
		WorkDir:     workDir,
		IPCEndpoint: sockPath,
		ClientID:    "ding_test",
		Edition:     "open",
		Source:      &fakeSource{},
		ReadyPipe:   readyWriter,
	})
	ready := make([]byte, 1)
	if err := readyReader.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if n, err := readyReader.Read(ready); err != nil || n != 1 || ready[0] != 'R' {
		t.Fatalf("ready signal = %q (n=%d), err=%v", ready, n, err)
	}

	conn, err := transport.Dial(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	w := transport.NewWriter(conn)
	r := transport.NewReader(conn)
	_ = w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, ConsumerPID: 12345})
	var ack transport.HelloAck
	_ = r.ReadJSON(&ack)

	// Slam the connection shut without sending Bye.
	conn.Close()

	time.Sleep(200 * time.Millisecond)
	c2, err := transport.Dial(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	w2 := transport.NewWriter(c2)
	r2 := transport.NewReader(c2)
	_ = w2.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, Role: transport.HelloRoleStatus})
	_ = w2.WriteJSON(transport.StatusReq{Type: transport.FrameTypeStatusReq})
	var resp transport.StatusResp
	if err := r2.ReadJSON(&resp); err != nil {
		t.Fatalf("read status: %v", err)
	}
	for _, c := range resp.Consumers {
		if c.PID == 12345 {
			t.Fatalf("dead consumer 12345 still present in status: %+v", resp.Consumers)
		}
	}
}
