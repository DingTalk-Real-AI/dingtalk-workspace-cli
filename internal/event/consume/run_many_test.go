// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package consume

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

type synchronizedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type manyFakeBus struct {
	client  net.Conn
	server  net.Conn
	hello   chan transport.Hello
	acked   chan struct{}
	ackGate <-chan struct{}
	ack     transport.HelloAck
	send    chan any
}

func newManyFakeBus(busPID int, ackGate <-chan struct{}) *manyFakeBus {
	client, server := net.Pipe()
	f := &manyFakeBus{
		client:  client,
		server:  server,
		hello:   make(chan transport.Hello, 1),
		acked:   make(chan struct{}),
		ackGate: ackGate,
		ack: transport.HelloAck{
			Type:            transport.FrameTypeHelloAck,
			BusPID:          busPID,
			SourceState:     "connected",
			StateSource:     "inferred",
			IdleTimeoutSecs: 300,
		},
		send: make(chan any, 8),
	}
	go f.serve()
	return f
}

func (f *manyFakeBus) serve() {
	r := transport.NewReader(f.server)
	w := transport.NewWriter(f.server)
	var hello transport.Hello
	if err := r.ReadJSON(&hello); err != nil {
		return
	}
	f.hello <- hello
	if f.ackGate != nil {
		select {
		case <-f.ackGate:
		case <-time.After(5 * time.Second):
			return
		}
	}
	if err := w.WriteJSON(f.ack); err != nil {
		return
	}
	close(f.acked)
	go func() {
		for {
			if _, err := r.Read(); err != nil {
				return
			}
		}
	}()
	for frame := range f.send {
		if err := w.WriteJSON(frame); err != nil {
			return
		}
	}
}

func installManyDiscover(t *testing.T, buses ...*manyFakeBus) {
	t.Helper()
	oldDiscover := discoverBus
	index := 0
	discoverBus = func(busctl.DiscoverConfig) (net.Conn, error) {
		if index >= len(buses) {
			return nil, errors.New("unexpected discover call")
		}
		conn := buses[index].client
		index++
		return conn, nil
	}
	t.Cleanup(func() {
		discoverBus = oldDiscover
		for _, bus := range buses {
			_ = bus.client.Close()
			_ = bus.server.Close()
			close(bus.send)
		}
	})
}

func manyTestConfig(stdout, stderr io.Writer) Config {
	return Config{
		WorkDir:     "workdir",
		IPCEndpoint: "endpoint",
		ClientID:    "client",
		Stdout:      stdout,
		Stderr:      stderr,
		Format:      FormatNDJSON,
	}
}

func manyTestSpecs() []ConsumerSpec {
	return []ConsumerSpec{
		{EventKey: "event-a", EventTypes: []string{"event-a"}, SubscribeID: "sub-a", ReadySubscribeID: "sub-a"},
		{EventKey: "event-b", EventTypes: []string{"event-b"}, SubscribeID: "sub-b", ReadySubscribeID: "sub-b"},
	}
}

func waitForBuffer(t *testing.T, b *synchronizedBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(b.String(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("buffer did not contain %q:\n%s", want, b.String())
}

func TestRunManyWaitsForAllConsumersAndStopsOneAtATime(t *testing.T) {
	secondAck := make(chan struct{})
	busA := newManyFakeBus(101, nil)
	busB := newManyFakeBus(101, secondAck)
	installManyDiscover(t, busA, busB)

	var stdout, stderr synchronizedBuffer
	done := make(chan error, 1)
	go func() {
		done <- RunMany(context.Background(), manyTestConfig(&stdout, &stderr), manyTestSpecs())
	}()

	helloA := <-busA.hello
	helloB := <-busB.hello
	if helloA.SubscribeID != "sub-a" || strings.Join(helloA.EventTypes, ",") != "event-a" {
		t.Fatalf("first hello = %#v", helloA)
	}
	if helloB.SubscribeID != "sub-b" || strings.Join(helloB.EventTypes, ",") != "event-b" {
		t.Fatalf("second hello = %#v", helloB)
	}
	select {
	case err := <-done:
		t.Fatalf("RunMany exited before all acknowledgements: %v", err)
	default:
	}
	if strings.Contains(stderr.String(), "[event] ready") || strings.Contains(stderr.String(), "[event] subscription") {
		t.Fatalf("ready output appeared before all acknowledgements:\n%s", stderr.String())
	}

	close(secondAck)
	waitForBuffer(t, &stderr, "[event] ready event_count=2 bus_pid=101")
	if !strings.Contains(stderr.String(), "event_key=event-a subscribe_id=sub-a") ||
		!strings.Contains(stderr.String(), "event_key=event-b subscribe_id=sub-b") {
		t.Fatalf("subscription markers missing:\n%s", stderr.String())
	}

	busA.send <- transport.Event{Type: transport.FrameTypeEvent, EventID: "event-1", EventType: "event-a", SubscribeID: "sub-a", Data: `{}`}
	waitForBuffer(t, &stdout, `"event_id":"event-1"`)
	busA.send <- transport.Bye{Type: transport.FrameTypeBye, Reason: transport.ByeReasonSubscriptionStopped}
	waitForBuffer(t, &stderr, "subscribe_id=sub-a remaining=1")

	busB.send <- transport.Event{Type: transport.FrameTypeEvent, EventID: "event-2", EventType: "event-b", SubscribeID: "sub-b", Data: `{}`}
	waitForBuffer(t, &stdout, `"event_id":"event-2"`)
	busB.send <- transport.Bye{Type: transport.FrameTypeBye, Reason: transport.ByeReasonSubscriptionStopped}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunMany() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunMany did not exit after the last subscription stopped")
	}
	if !strings.Contains(stderr.String(), "subscribe_id=sub-b remaining=0") {
		t.Fatalf("last stop marker missing:\n%s", stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout lines = %d:\n%s", len(lines), stdout.String())
	}
	for i, line := range lines {
		var event transport.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
	}
}

func TestRunManyMaxEventsIsSharedAcrossConsumers(t *testing.T) {
	busA := newManyFakeBus(202, nil)
	busB := newManyFakeBus(202, nil)
	installManyDiscover(t, busA, busB)
	var stdout, stderr synchronizedBuffer
	cfg := manyTestConfig(&stdout, &stderr)
	cfg.MaxEvents = 1
	done := make(chan error, 1)
	go func() { done <- RunMany(context.Background(), cfg, manyTestSpecs()) }()
	<-busA.hello
	<-busB.hello
	<-busA.acked
	<-busB.acked
	busB.send <- transport.Event{Type: transport.FrameTypeEvent, EventID: "only", EventType: "event-b", SubscribeID: "sub-b", Data: `{}`}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunMany() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunMany did not stop at the shared max-events limit")
	}
	if got := strings.Count(strings.TrimSpace(stdout.String()), "\n") + 1; got != 1 {
		t.Fatalf("output event count = %d", got)
	}
}

func TestRunManyRejectsInvalidSpecsAndDifferentBuses(t *testing.T) {
	cfg := manyTestConfig(io.Discard, io.Discard)
	for _, test := range []struct {
		name  string
		specs []ConsumerSpec
	}{
		{name: "one", specs: []ConsumerSpec{{EventKey: "a", SubscribeID: "sub-a"}}},
		{name: "empty event", specs: []ConsumerSpec{{SubscribeID: "sub-a"}, {EventKey: "b", SubscribeID: "sub-b"}}},
		{name: "empty subscription", specs: []ConsumerSpec{{EventKey: "a"}, {EventKey: "b", SubscribeID: "sub-b"}}},
		{name: "duplicate subscription", specs: []ConsumerSpec{{EventKey: "a", SubscribeID: "sub"}, {EventKey: "b", SubscribeID: "sub"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := RunMany(context.Background(), cfg, test.specs); err == nil {
				t.Fatal("RunMany succeeded")
			}
		})
	}

	busA := newManyFakeBus(1, nil)
	busB := newManyFakeBus(2, nil)
	installManyDiscover(t, busA, busB)
	if err := RunMany(context.Background(), cfg, manyTestSpecs()); err == nil || !strings.Contains(err.Error(), "different bus processes") {
		t.Fatalf("different bus error = %v", err)
	}
}

func TestRunManyContextCancellationInterruptsHandshake(t *testing.T) {
	neverAck := make(chan struct{})
	busA := newManyFakeBus(1, neverAck)
	busB := newManyFakeBus(1, nil)
	installManyDiscover(t, busA, busB)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunMany(ctx, manyTestConfig(io.Discard, io.Discard), manyTestSpecs()) }()
	<-busA.hello
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunMany handshake did not unblock on context cancellation")
	}
	close(neverAck)
}

func TestCrossPlatformCoverageRunManySetupAndHandshakeEdges(t *testing.T) {
	specs := manyTestSpecs()
	if err := RunMany(context.Background(), Config{}, specs); err == nil {
		t.Fatal("missing required config should fail")
	}

	cfg := Config{
		WorkDir: "workdir", IPCEndpoint: "endpoint", ClientID: "client",
		Quiet: true, DryRun: true,
	}
	if err := RunMany(context.Background(), cfg, specs); err != nil {
		t.Fatalf("defaulted quiet dry run: %v", err)
	}

	cfg = manyTestConfig(io.Discard, io.Discard)
	cfg.Format = Format("invalid")
	if err := RunMany(context.Background(), cfg, specs); err == nil {
		t.Fatal("invalid format should fail before discovery")
	}

	oldDiscover := discoverBus
	t.Cleanup(func() { discoverBus = oldDiscover })
	wantErr := errors.New("synthetic discover failure")
	discoverBus = func(busctl.DiscoverConfig) (net.Conn, error) { return nil, wantErr }
	if err := RunMany(context.Background(), manyTestConfig(io.Discard, io.Discard), specs); !errors.Is(err, wantErr) {
		t.Fatalf("discover error = %v", err)
	}

	discoverBus = func(busctl.DiscoverConfig) (net.Conn, error) {
		return &faultConn{writeErr: wantErr}, nil
	}
	if err := RunMany(context.Background(), manyTestConfig(io.Discard, io.Discard), specs); !errors.Is(err, wantErr) {
		t.Fatalf("hello write error = %v", err)
	}

	discoverBus = func(busctl.DiscoverConfig) (net.Conn, error) {
		return &faultConn{readErr: io.EOF}, nil
	}
	if err := RunMany(context.Background(), manyTestConfig(io.Discard, io.Discard), specs); err == nil || !strings.Contains(err.Error(), "hello_ack") {
		t.Fatalf("hello ack read error = %v", err)
	}

	wrongAck := append(mustJSONFrame(t, transport.HelloAck{Type: transport.FrameTypeHeartbeat}), '\n')
	discoverBus = func(busctl.DiscoverConfig) (net.Conn, error) {
		return &faultConn{readData: append([]byte(nil), wrongAck...), readErr: io.EOF}, nil
	}
	if err := RunMany(context.Background(), manyTestConfig(io.Discard, io.Discard), specs); err == nil || !strings.Contains(err.Error(), "unexpected first frame") {
		t.Fatalf("unexpected ack error = %v", err)
	}
}

func TestRunManyDurationAndStdinEOFStopTheWholeGroup(t *testing.T) {
	t.Run("duration", func(t *testing.T) {
		busA := newManyFakeBus(301, nil)
		busB := newManyFakeBus(301, nil)
		installManyDiscover(t, busA, busB)
		cfg := manyTestConfig(io.Discard, io.Discard)
		cfg.Duration = 15 * time.Millisecond
		cfg.Quiet = true
		if err := RunMany(context.Background(), cfg, manyTestSpecs()); err != nil {
			t.Fatalf("duration run: %v", err)
		}
	})

	t.Run("stdin EOF", func(t *testing.T) {
		busA := newManyFakeBus(302, nil)
		busB := newManyFakeBus(302, nil)
		installManyDiscover(t, busA, busB)
		var stderr synchronizedBuffer
		cfg := manyTestConfig(io.Discard, &stderr)
		cfg.Stdin = strings.NewReader("")
		if err := RunMany(context.Background(), cfg, manyTestSpecs()); err != nil {
			t.Fatalf("stdin EOF run: %v", err)
		}
		if !strings.Contains(stderr.String(), "reason: signal") {
			t.Fatalf("stdin exit marker missing:\n%s", stderr.String())
		}
	})
}

func TestRunManySourceStateAndBusShutdown(t *testing.T) {
	busA := newManyFakeBus(401, nil)
	busB := newManyFakeBus(401, nil)
	installManyDiscover(t, busA, busB)
	var stderr synchronizedBuffer
	done := make(chan error, 1)
	go func() {
		done <- RunMany(context.Background(), manyTestConfig(io.Discard, &stderr), manyTestSpecs())
	}()
	<-busA.hello
	<-busB.hello
	<-busA.acked
	<-busB.acked
	busA.send <- transport.SourceState{Type: transport.FrameTypeSourceState, State: "reconnecting", StateSource: "hook", Attempt: 2}
	waitForBuffer(t, &stderr, "source state: reconnecting")
	busB.send <- transport.Heartbeat{Type: transport.FrameTypeHeartbeat}
	busB.send <- transport.Bye{Type: transport.FrameTypeBye, Reason: "shutdown"}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("bus shutdown run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunMany did not exit on bus shutdown")
	}
	if !strings.Contains(stderr.String(), "bus closing: shutdown") {
		t.Fatalf("bus shutdown marker missing:\n%s", stderr.String())
	}
	if got := firstActiveSession(map[int]struct{}{3: {}, 1: {}, 2: {}}); got != 1 {
		t.Fatalf("firstActiveSession() = %d", got)
	}
}

func TestRunManyOutputAndReadFailures(t *testing.T) {
	t.Run("broken output pipe is graceful", func(t *testing.T) {
		busA := newManyFakeBus(501, nil)
		busB := newManyFakeBus(501, nil)
		installManyDiscover(t, busA, busB)
		cfg := manyTestConfig(errorWriter{err: testBrokenPipeError()}, io.Discard)
		done := make(chan error, 1)
		go func() { done <- RunMany(context.Background(), cfg, manyTestSpecs()) }()
		<-busA.hello
		<-busB.hello
		<-busA.acked
		<-busB.acked
		busA.send <- transport.Event{Type: transport.FrameTypeEvent, EventID: "pipe", EventType: "event-a", SubscribeID: "sub-a", Data: `{}`}
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("broken pipe run: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("RunMany did not exit on broken output pipe")
		}
	})

	t.Run("ordinary output error fails group", func(t *testing.T) {
		busA := newManyFakeBus(502, nil)
		busB := newManyFakeBus(502, nil)
		installManyDiscover(t, busA, busB)
		wantErr := errors.New("output failed")
		cfg := manyTestConfig(errorWriter{err: wantErr}, io.Discard)
		done := make(chan error, 1)
		go func() { done <- RunMany(context.Background(), cfg, manyTestSpecs()) }()
		<-busA.hello
		<-busB.hello
		<-busA.acked
		<-busB.acked
		busB.send <- transport.Event{Type: transport.FrameTypeEvent, EventID: "error", EventType: "event-b", SubscribeID: "sub-b", Data: `{}`}
		select {
		case err := <-done:
			if !errors.Is(err, wantErr) {
				t.Fatalf("output error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("RunMany did not exit on output failure")
		}
	})

	t.Run("unexpected peer close fails group", func(t *testing.T) {
		ack := mustJSONFrame(t, transport.HelloAck{Type: transport.FrameTypeHelloAck, BusPID: 503})
		oldDiscover := discoverBus
		discoverBus = pipeDiscover(t, [][]byte{ack}, true)
		t.Cleanup(func() { discoverBus = oldDiscover })
		if err := RunMany(context.Background(), manyTestConfig(io.Discard, io.Discard), manyTestSpecs()); err == nil || !strings.Contains(err.Error(), "read frame") {
			t.Fatalf("peer close error = %v", err)
		}
	})
}
