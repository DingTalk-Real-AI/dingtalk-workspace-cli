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

package consume

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix socket")
	}
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return t.TempDir()
	}
	dir, err := os.MkdirTemp("/tmp", "dws-consume-")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// fakeSource mirrors the one in bus tests; reproduced here to keep the
// integration test self-contained.
type fakeSource struct {
	events  []dwsevent.RawEvent
	trigger <-chan struct{}
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
		time.Sleep(5 * time.Millisecond)
	}
	<-ctx.Done()
	return ctx.Err()
}

// bringUpBus starts a bus.Run in a goroutine and waits for its socket.
// Returns (workDir, sockPath, cancelFunc, runDone, fakeSource trigger).
func bringUpBus(t *testing.T, events []dwsevent.RawEvent) (string, string, context.CancelFunc, <-chan error, chan struct{}) {
	t.Helper()
	dir := shortTempDir(t)
	sock := filepath.Join(dir, "bus.sock")
	ctx, cancel := context.WithCancel(context.Background())
	trigger := make(chan struct{})
	src := &fakeSource{events: events, trigger: trigger}
	done := make(chan error, 1)
	go func() {
		done <- bus.Run(ctx, bus.Config{
			WorkDir:     dir,
			IPCEndpoint: sock,
			ClientID:    "ding_test",
			Edition:     "open",
			Source:      src,
		})
	}()

	// Wait for socket.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
			return dir, sock, cancel, done, trigger
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	t.Fatalf("bus socket did not appear")
	return "", "", nil, nil, nil
}

// dialOnlyDiscover is a Discover-impl-bypass: tests don't want consume.Run
// to exec a real dws binary, so we sidestep by pre-bringing-up the bus and
// letting Discover succeed on its first dial attempt. No Spawn is required.

func TestRun_StdoutNDJSON(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancel, runDone, trigger := bringUpBus(t, []dwsevent.RawEvent{
		{EventID: "1", EventType: "im.message.receive_v1", Data: `{"text":"hi"}`},
		{EventID: "2", EventType: "im.message.at_v1", Data: `{"at":1}`},
	})
	defer func() { cancel(); <-runDone }()

	// Trigger source emission after we've started consume (otherwise events
	// race ahead of consumer registration).
	go func() {
		time.Sleep(150 * time.Millisecond)
		close(trigger)
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ctx, consumeCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer consumeCancel()
	err := Run(ctx, Config{
		WorkDir:          dir,
		IPCEndpoint:      sock,
		ClientID:         "ding_test",
		Stdout:           &stdout,
		Stderr:           &stderr,
		EventTypes:       []string{"im.*"},
		EventKey:         "im.message.receive_v1",
		ReadySubscribeID: "sub-1",
		MaxEvents:        2,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify NDJSON: each non-empty line is a valid Event JSON.
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d:\n%s", len(lines), stdout.String())
	}
	for i, line := range lines {
		var ev transport.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Errorf("line %d not valid JSON: %v\n%s", i, err, line)
		}
		if ev.Type != transport.FrameTypeEvent {
			t.Errorf("line %d type = %s, want event", i, ev.Type)
		}
	}

	// Stderr should carry the standardized ready marker.
	if !strings.Contains(stderr.String(), "[event] ready ") {
		t.Errorf("stderr missing ready marker:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "[event] ready event_key=im.message.receive_v1 bus_pid=") ||
		!strings.Contains(stderr.String(), " subscribe_id=sub-1") {
		t.Errorf("stderr missing ready marker:\n%s", stderr.String())
	}
}

func TestRun_QuietSuppressesStderr(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancel, runDone, trigger := bringUpBus(t, []dwsevent.RawEvent{
		{EventID: "1", EventType: "im.message.receive_v1", Data: `{}`},
	})
	defer func() { cancel(); <-runDone }()
	go func() { time.Sleep(150 * time.Millisecond); close(trigger) }()

	var stdout, stderr bytes.Buffer
	ctx, consumeCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer consumeCancel()
	err := Run(ctx, Config{
		WorkDir:          dir,
		IPCEndpoint:      sock,
		ClientID:         "ding_test",
		Stdout:           &stdout,
		Stderr:           &stderr,
		Quiet:            true,
		EventKey:         "im.message.receive_v1",
		ReadySubscribeID: "sub-1",
		MaxEvents:        1,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("--quiet should suppress all stderr; got: %s", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Error("stdout should still contain the NDJSON event")
	}
}

func TestRun_WritesFirstProjectedEventBeforeConsumeExits(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancelBus, runDone, trigger := bringUpBus(t, []dwsevent.RawEvent{
		{EventID: "1", EventType: "personal.message", Data: `{"content":"hello"}`},
	})
	defer func() { cancelBus(); <-runDone }()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()

	ctx, cancelConsume := context.WithCancel(context.Background())
	consumeDone := make(chan error, 1)
	go func() {
		consumeDone <- Run(ctx, Config{
			WorkDir:     dir,
			IPCEndpoint: sock,
			ClientID:    "ding_test",
			Stdout:      writer,
			Stderr:      io.Discard,
			Projector: func(ev transport.Event) (any, error) {
				return map[string]any{"type": ev.EventType, "content": "hello"}, nil
			},
		})
	}()
	go func() {
		time.Sleep(150 * time.Millisecond)
		close(trigger)
	}()

	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(reader).ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		lineCh <- line
	}()

	select {
	case line := <-lineCh:
		var got map[string]any
		if err := json.Unmarshal([]byte(line), &got); err != nil || got["content"] != "hello" {
			t.Fatalf("first line = %q, err = %v", line, err)
		}
	case err := <-errCh:
		t.Fatalf("read first line: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("first event was not readable while consume was running")
	}

	select {
	case err := <-consumeDone:
		t.Fatalf("consume exited before cancellation: %v", err)
	default:
	}
	cancelConsume()
	select {
	case err := <-consumeDone:
		if err != nil {
			t.Fatalf("Run() after cancellation = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("consume did not exit after cancellation")
	}
}

func TestRun_CtxCancelReturnsCleanly(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancel, runDone, _ := bringUpBus(t, nil)
	defer func() { cancel(); <-runDone }()

	ctx, consumeCancel := context.WithCancel(context.Background())
	consumeDone := make(chan error, 1)
	go func() {
		consumeDone <- Run(ctx, Config{
			WorkDir:     dir,
			IPCEndpoint: sock,
			ClientID:    "ding_test",
			Stdout:      io.Discard,
			Stderr:      io.Discard,
		})
	}()
	// Let consume connect.
	time.Sleep(100 * time.Millisecond)
	consumeCancel()

	select {
	case err := <-consumeDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want nil or canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

func TestRun_MaxEventsZeroIsUnlimited(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancel, runDone, trigger := bringUpBus(t, []dwsevent.RawEvent{
		{EventID: "1", EventType: "x", Data: `{}`},
		{EventID: "2", EventType: "x", Data: `{}`},
		{EventID: "3", EventType: "x", Data: `{}`},
	})
	defer func() { cancel(); <-runDone }()

	var stdout bytes.Buffer
	ctx, consumeCancel := context.WithCancel(context.Background())
	consumeDone := make(chan error, 1)
	go func() {
		consumeDone <- Run(ctx, Config{
			WorkDir:     dir,
			IPCEndpoint: sock,
			ClientID:    "ding_test",
			Stdout:      &stdout,
			Stderr:      io.Discard,
			MaxEvents:   0, // unlimited
		})
	}()
	// Wait for consume to dial + Hello (otherwise events fire before
	// consumer registers and the Hub drops them silently — no consumer
	// to deliver to).
	time.Sleep(150 * time.Millisecond)
	close(trigger)
	// Let all 3 events flow.
	time.Sleep(200 * time.Millisecond)
	consumeCancel()
	<-consumeDone

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 events with MaxEvents=0, got %d:\n%s", len(lines), stdout.String())
	}
}

// TestRun_MultipleConsumersOneBus exercises the daemon's multi-consumer
// fan-out via the real consume.Run path. Both consumers should receive
// every matching event independently.
func TestRun_MultipleConsumersOneBus(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancel, runDone, trigger := bringUpBus(t, []dwsevent.RawEvent{
		{EventID: "1", EventType: "im.message.receive_v1", Data: `{}`},
		{EventID: "2", EventType: "im.message.receive_v1", Data: `{}`},
	})
	defer func() { cancel(); <-runDone }()

	var wg sync.WaitGroup
	bufs := make([]*bytes.Buffer, 2)
	for i := 0; i < 2; i++ {
		i := i
		bufs[i] = &bytes.Buffer{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Run(context.Background(), Config{
				WorkDir:     dir,
				IPCEndpoint: sock,
				ClientID:    "ding_test",
				Stdout:      bufs[i],
				Stderr:      io.Discard,
				MaxEvents:   2,
			})
		}()
	}
	// Both consumers should be Hello'd before trigger.
	time.Sleep(200 * time.Millisecond)
	close(trigger)

	wg.Wait()
	for i, buf := range bufs {
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		if len(lines) != 2 {
			t.Errorf("consumer %d: got %d lines, want 2:\n%s", i, len(lines), buf.String())
		}
	}
}

// --- AI subprocess contract tests (event-subprocess-contract.md) ---

// T1a/T1b: the ready marker carries event_key and precedes the first
// stdout event.
func TestRun_ReadyMarkerContract(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancel, runDone, trigger := bringUpBus(t, []dwsevent.RawEvent{
		{EventID: "1", EventType: "im.message.receive_v1", Data: `{}`},
	})
	defer func() { cancel(); <-runDone }()
	go func() { time.Sleep(150 * time.Millisecond); close(trigger) }()

	var stdout, stderr bytes.Buffer
	ctx, cc := context.WithTimeout(context.Background(), 3*time.Second)
	defer cc()
	if err := Run(ctx, Config{
		WorkDir: dir, IPCEndpoint: sock, ClientID: "ding_test",
		Stdout: &stdout, Stderr: &stderr,
		EventKey: "im.message.receive_v1", MaxEvents: 1,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// T1a: ready line with event_key.
	if !strings.Contains(stderr.String(), "[event] ready event_key=im.message.receive_v1") {
		t.Errorf("missing ready marker with event_key:\n%s", stderr.String())
	}
	// T1b: ready is on stderr, the event is on stdout — the ready line is
	// emitted before Deliver writes the first stdout line. Assert stdout
	// got exactly the event and stderr got ready before exited.
	rIdx := strings.Index(stderr.String(), "[event] ready")
	xIdx := strings.Index(stderr.String(), "[event] exited")
	if rIdx < 0 || xIdx < 0 || rIdx > xIdx {
		t.Errorf("ready must precede exited:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "im.message.receive_v1") {
		t.Errorf("stdout missing the event:\n%s", stdout.String())
	}
}

// T3a: --max-events exit → reason=limit.
func TestRun_ExitReasonLimit(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancel, runDone, trigger := bringUpBus(t, []dwsevent.RawEvent{
		{EventID: "1", EventType: "im.message.receive_v1", Data: `{}`},
	})
	defer func() { cancel(); <-runDone }()
	go func() { time.Sleep(150 * time.Millisecond); close(trigger) }()

	var stdout, stderr bytes.Buffer
	ctx, cc := context.WithTimeout(context.Background(), 3*time.Second)
	defer cc()
	if err := Run(ctx, Config{
		WorkDir: dir, IPCEndpoint: sock, ClientID: "ding_test",
		Stdout: &stdout, Stderr: &stderr, MaxEvents: 1,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(stderr.String(), "reason: limit") {
		t.Errorf("expected reason: limit:\n%s", stderr.String())
	}
}

// T3b: --duration deadline with no events → reason=timeout, exit clean.
func TestRun_ExitReasonTimeout(t *testing.T) {
	skipOnWindows(t)
	// Bring up bus but never trigger emission → consume blocks until the
	// duration deadline fires.
	dir, sock, cancel, runDone, _ := bringUpBus(t, nil)
	defer func() { cancel(); <-runDone }()

	var stdout, stderr bytes.Buffer
	ctx, cc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cc()
	if err := Run(ctx, Config{
		WorkDir: dir, IPCEndpoint: sock, ClientID: "ding_test",
		Stdout: &stdout, Stderr: &stderr, Duration: 300 * time.Millisecond,
	}); err != nil {
		t.Fatalf("Run should exit clean on duration: %v", err)
	}
	if !strings.Contains(stderr.String(), "reason: timeout") {
		t.Errorf("expected reason: timeout:\n%s", stderr.String())
	}
}

// T3c: parent ctx cancel (signal) → reason=signal.
func TestRun_ExitReasonSignal(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancel, runDone, _ := bringUpBus(t, nil)
	defer func() { cancel(); <-runDone }()

	var stdout, stderr bytes.Buffer
	ctx, cc := context.WithCancel(context.Background())
	go func() { time.Sleep(250 * time.Millisecond); cc() }()
	if err := Run(ctx, Config{
		WorkDir: dir, IPCEndpoint: sock, ClientID: "ding_test",
		Stdout: &stdout, Stderr: &stderr,
	}); err != nil {
		t.Fatalf("Run should exit clean on signal: %v", err)
	}
	if !strings.Contains(stderr.String(), "reason: signal") {
		t.Errorf("expected reason: signal:\n%s", stderr.String())
	}
}

// T2c: closing stdin triggers a graceful shutdown (reason=signal), with no
// --max-events / --duration set.
func TestRun_StdinEOFShutsDown(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancel, runDone, _ := bringUpBus(t, nil)
	defer func() { cancel(); <-runDone }()

	pr, pw := io.Pipe()
	go func() { time.Sleep(250 * time.Millisecond); _ = pw.Close() }() // EOF

	var stdout, stderr bytes.Buffer
	ctx, cc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cc()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			WorkDir: dir, IPCEndpoint: sock, ClientID: "ding_test",
			Stdout: &stdout, Stderr: &stderr, Stdin: pr,
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run should exit clean on stdin EOF: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not exit after stdin EOF")
	}
	if !strings.Contains(stderr.String(), "reason: signal") {
		t.Errorf("expected reason: signal on stdin EOF:\n%s", stderr.String())
	}
}

// T3d: a startup failure returns non-nil and emits NO `exited` line (the
// cobra layer prints an Error: line instead).
func TestRun_FailureHasNoExitedLine(t *testing.T) {
	skipOnWindows(t)
	var stderr bytes.Buffer
	// Missing WorkDir/IPCEndpoint/ClientID → immediate config error, before
	// the exit-reason machinery is armed.
	err := Run(context.Background(), Config{Stderr: &stderr})
	if err == nil {
		t.Fatal("expected error for missing required config")
	}
	if strings.Contains(stderr.String(), "[event] exited") {
		t.Errorf("failure must not print an exited line:\n%s", stderr.String())
	}
}
