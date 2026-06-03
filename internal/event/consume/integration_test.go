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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// TestIntegration_HelloPushdownFiltersAtBus verifies the Hello-time
// event_types pushdown contract (plan §4 unsung superpower): a consumer
// subscribing to "im.*" must NOT receive "approval.*" events even when
// bus and source are flowing both. The filter happens at the Hub layer,
// not at the consumer pipeline — saves IPC bytes for narrow consumers.
func TestIntegration_HelloPushdownFiltersAtBus(t *testing.T) {
	skipOnWindows(t)
	events := []dwsevent.RawEvent{
		{EventID: "1", EventType: "im.message.receive_v1", Data: `{}`},
		{EventID: "2", EventType: "approval.task", Data: `{}`},
		{EventID: "3", EventType: "im.message.at_v1", Data: `{}`},
		{EventID: "4", EventType: "approval.instance.status_changed", Data: `{}`},
		{EventID: "5", EventType: "im.chat.member.user.added_v1", Data: `{}`},
	}
	dir, sock, cancel, runDone, trigger := bringUpBus(t, events)
	defer func() { cancel(); <-runDone }()

	var imBuf, approvalBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	// Consumer A: im.* only
	go func() {
		defer wg.Done()
		_ = Run(context.Background(), Config{
			WorkDir:     dir,
			IPCEndpoint: sock,
			ClientID:    "ding_test",
			Stdout:      &imBuf,
			Stderr:      io.Discard,
			EventTypes:  []string{"im.*"},
			MaxEvents:   3, // 3 im events expected
		})
	}()
	// Consumer B: approval.* only
	go func() {
		defer wg.Done()
		_ = Run(context.Background(), Config{
			WorkDir:     dir,
			IPCEndpoint: sock,
			ClientID:    "ding_test",
			Stdout:      &approvalBuf,
			Stderr:      io.Discard,
			EventTypes:  []string{"approval.*"},
			MaxEvents:   2, // 2 approval events expected
		})
	}()

	// Give both consumers time to Hello + register.
	time.Sleep(200 * time.Millisecond)
	close(trigger)
	wg.Wait()

	// Verify consumer A got exactly the 3 im.* events.
	imLines := nonEmptyLines(imBuf.String())
	if len(imLines) != 3 {
		t.Fatalf("im consumer got %d events, want 3:\n%s", len(imLines), imBuf.String())
	}
	for i, line := range imLines {
		var ev transport.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("im[%d] not valid JSON: %v", i, err)
		}
		if !strings.HasPrefix(ev.EventType, "im.") {
			t.Errorf("im consumer got non-im event: %s", ev.EventType)
		}
	}

	// Verify consumer B got exactly the 2 approval.* events.
	apprLines := nonEmptyLines(approvalBuf.String())
	if len(apprLines) != 2 {
		t.Fatalf("approval consumer got %d events, want 2:\n%s", len(apprLines), approvalBuf.String())
	}
	for i, line := range apprLines {
		var ev transport.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("approval[%d] not valid JSON: %v", i, err)
		}
		if !strings.HasPrefix(ev.EventType, "approval.") {
			t.Errorf("approval consumer got non-approval event: %s", ev.EventType)
		}
	}
}

// TestIntegration_FilterRegexInAdditionToEventTypes verifies the
// regex --filter is applied on top of --event-types (logical AND).
// Both narrow the stream; the test confirms only events matching BOTH
// surface to the consumer.
func TestIntegration_FilterRegexNarrowsFurther(t *testing.T) {
	skipOnWindows(t)
	events := []dwsevent.RawEvent{
		{EventID: "1", EventType: "im.message.receive_v1", Data: `{}`},
		{EventID: "2", EventType: "im.message.at_v1", Data: `{}`},
		{EventID: "3", EventType: "im.chat.member.user.added_v1", Data: `{}`},
	}
	dir, sock, cancel, runDone, trigger := bringUpBus(t, events)
	defer func() { cancel(); <-runDone }()

	var buf bytes.Buffer
	consumeDone := make(chan struct{})
	go func() {
		defer close(consumeDone)
		_ = Run(context.Background(), Config{
			WorkDir:     dir,
			IPCEndpoint: sock,
			ClientID:    "ding_test",
			Stdout:      &buf,
			Stderr:      io.Discard,
			EventTypes:  []string{"im.*"},
			Filter:      `\.at_v1$`, // only at_v1 events
			MaxEvents:   1,
		})
	}()
	time.Sleep(150 * time.Millisecond)
	close(trigger)
	<-consumeDone

	lines := nonEmptyLines(buf.String())
	if len(lines) != 1 {
		t.Fatalf("expected 1 event after im.* + .at_v1 regex, got %d:\n%s", len(lines), buf.String())
	}
	var ev transport.Event
	_ = json.Unmarshal([]byte(lines[0]), &ev)
	if ev.EventType != "im.message.at_v1" {
		t.Errorf("got %q, want im.message.at_v1", ev.EventType)
	}
}

// TestIntegration_BusRestartConsumerReconnects verifies bus death + restart
// scenario: a consumer dialing after the first bus died and was replaced
// should connect to the fresh bus and receive new events. This proves
// stale lock cleanup + fresh-bus startup flow work correctly.
func TestIntegration_BusRestartConsumerReconnects(t *testing.T) {
	skipOnWindows(t)
	dir := shortTempDir(t)
	sock := filepath.Join(dir, "bus.sock")
	ctx1, cancel1 := context.WithCancel(context.Background())
	run1Done := make(chan error, 1)
	go func() {
		run1Done <- bus.Run(ctx1, bus.Config{
			WorkDir:     dir,
			IPCEndpoint: sock,
			ClientID:    "ding_test",
			Edition:     "open",
			Source:      &fakeSource{},
		})
	}()
	waitForSock(t, sock, 2*time.Second)
	// Verify first bus is up by dialing it briefly.
	conn, err := transport.Dial(sock)
	if err != nil {
		t.Fatalf("dial first bus: %v", err)
	}
	conn.Close()

	// Kill the first bus.
	cancel1()
	if err := <-run1Done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("first bus exited with: %v", err)
	}

	// Start second bus on same workdir (stale lock should be reclaimed).
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	run2Done := make(chan error, 1)
	events := []dwsevent.RawEvent{
		{EventID: "post_restart", EventType: "im.message.receive_v1", Data: `{}`},
	}
	trigger := make(chan struct{})
	src := &fakeSource{events: events, trigger: trigger}
	go func() {
		run2Done <- bus.Run(ctx2, bus.Config{
			WorkDir:     dir,
			IPCEndpoint: sock,
			ClientID:    "ding_test",
			Edition:     "open",
			Source:      src,
		})
	}()
	waitForSock(t, sock, 2*time.Second)

	// Consumer dials the second bus and receives the new event.
	var buf bytes.Buffer
	consumeDone := make(chan error, 1)
	go func() {
		consumeDone <- Run(context.Background(), Config{
			WorkDir:     dir,
			IPCEndpoint: sock,
			ClientID:    "ding_test",
			Stdout:      &buf,
			Stderr:      io.Discard,
			MaxEvents:   1,
		})
	}()
	time.Sleep(150 * time.Millisecond)
	close(trigger)
	if err := <-consumeDone; err != nil {
		t.Fatalf("consume on restarted bus: %v", err)
	}

	lines := nonEmptyLines(buf.String())
	if len(lines) != 1 {
		t.Fatalf("expected 1 event after restart, got %d", len(lines))
	}
	var ev transport.Event
	_ = json.Unmarshal([]byte(lines[0]), &ev)
	if ev.EventID != "post_restart" {
		t.Errorf("EventID = %q, want post_restart", ev.EventID)
	}

	cancel2()
	<-run2Done
}

// TestIntegration_PipelineWithRouteAndOutputDir is the end-to-end version
// of the unit pipeline tests: a real bus + a real consume.Run process
// configured with --route and --output-dir. Verifies that matched
// events land in route dirs and unmatched events in the fallback dir.
func TestIntegration_PipelineWithRouteAndOutputDir(t *testing.T) {
	skipOnWindows(t)
	events := []dwsevent.RawEvent{
		{EventID: "im1", EventType: "im.message.receive_v1", Data: `{}`},
		{EventID: "ap1", EventType: "approval.task", Data: `{}`},
		{EventID: "im2", EventType: "im.message.at_v1", Data: `{}`},
	}
	dir, sock, cancel, runDone, trigger := bringUpBus(t, events)
	defer func() { cancel(); <-runDone }()

	outputRoot := shortTempDir(t)
	imDir := filepath.Join(outputRoot, "im")
	defaultDir := filepath.Join(outputRoot, "default")
	routes, _ := ParseRoutes([]string{`^im\.=dir:` + imDir})

	consumeDone := make(chan struct{})
	go func() {
		defer close(consumeDone)
		_ = Run(context.Background(), Config{
			WorkDir:     dir,
			IPCEndpoint: sock,
			ClientID:    "ding_test",
			Stdout:      io.Discard,
			Stderr:      io.Discard,
			OutputDir:   defaultDir,
			Routes:      routes,
			MaxEvents:   3,
		})
	}()
	time.Sleep(150 * time.Millisecond)
	close(trigger)
	<-consumeDone

	if entries, _ := os.ReadDir(imDir); len(entries) != 2 {
		t.Errorf("imDir got %d files, want 2 (im1+im2)", len(entries))
	}
	if entries, _ := os.ReadDir(defaultDir); len(entries) != 1 {
		t.Errorf("defaultDir got %d files, want 1 (ap1)", len(entries))
	}
}

func nonEmptyLines(s string) []string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// waitForSock polls for the unix socket file. Used by bus-restart tests
// that bring up bus.Run twice in the same workdir.
func waitForSock(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket %q did not appear within %s", path, timeout)
}
