// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package bus

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

type sourceStateFailConn struct {
	net.Conn
	writes int
}

func (c *sourceStateFailConn) Write(p []byte) (int, error) {
	c.writes++
	// Complete HelloAck, then fail the next source-state frame.
	if c.writes > 1 {
		return 0, errors.New("state push unavailable")
	}
	return c.Conn.Write(p)
}

func TestCrossPlatformCoverageBusSourceUpdateWriteFailureClosesConnection(t *testing.T) {
	src := &observedSource{status: transport.StatusSource{State: "connecting", Source: "inferred", Observed: true}}
	d := eventCoreDaemon(nil)
	d.cfg.Source = src
	client, w, r, done := eventCoreConnection(d, func(c net.Conn) net.Conn { return &sourceStateFailConn{Conn: c} })
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	if err := w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello}); err != nil {
		t.Fatal(err)
	}
	var ack transport.HelloAck
	if err := r.ReadJSON(&ack); err != nil {
		t.Fatal(err)
	}
	src.set(transport.StatusSource{State: "connected", Source: "inferred", Observed: true})
	if _, err := r.Read(); err == nil {
		t.Fatal("failed source update reported success")
	}
	eventCoreWaitDone(t, done)
}

type unobservedSource struct{}

func (unobservedSource) Start(ctx context.Context, _ dwsevent.EmitFn) error {
	<-ctx.Done()
	return ctx.Err()
}

type observedSource struct {
	unobservedSource
	mu     sync.Mutex
	status transport.StatusSource
}

func (s *observedSource) SourceStatus() transport.StatusSource {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}
func (s *observedSource) set(status transport.StatusSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

func TestCrossPlatformCoverageBusPushesActualSourceTransitions(t *testing.T) {
	dir := shortTempDir(t)
	endpoint := dwsevent.IPCEndpoint(dir, "open", dwsevent.SourceKindPersonalStream, dwsevent.IdentityHash(dir))
	src := &observedSource{status: transport.StatusSource{State: "connecting", Source: "inferred", Observed: true}}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{WorkDir: dir, IPCEndpoint: endpoint, ClientID: "fixture", Edition: "open", Source: src})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("bus 未停止")
		}
	})
	var conn net.Conn
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		conn, err = transport.Dial(endpoint)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if conn == nil {
		t.Fatal("IPC 未就绪")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	w, r := transport.NewWriter(conn), transport.NewReader(conn)
	if err := w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, ConsumerPID: os.Getpid(), EventTypes: []string{"fixture"}}); err != nil {
		t.Fatal(err)
	}
	var ack transport.HelloAck
	if err := r.ReadJSON(&ack); err != nil {
		t.Fatal(err)
	}
	if ack.SourceState != "connecting" || !ack.SourceObserved {
		t.Fatalf("初始状态失真: %+v", ack)
	}
	for _, state := range []string{"connected", "reconnecting", "idle"} {
		want := transport.StatusSource{State: state, Source: "inferred", Observed: true, ReconnectCount: 3, LastEventAtMS: 100, LastReconnectMS: 200}
		src.set(want)
		var update transport.SourceState
		if err := r.ReadJSON(&update); err != nil {
			t.Fatal(err)
		}
		if update.Type != transport.FrameTypeSourceState || update.State != state || !update.Observed || update.Attempt != 3 {
			t.Fatalf("推送失真: %+v", update)
		}
		if got := queryDaemonStatus(t, endpoint).SourceState; got != want {
			t.Fatalf("状态查询丢失观测: got=%+v want=%+v", got, want)
		}
	}
}

func TestCrossPlatformCoverageUnobservedSourceIsNotConnected(t *testing.T) {
	d := &daemon{cfg: Config{Source: unobservedSource{}}}
	if ack := d.helloAck(); ack.SourceState == "connected" {
		t.Fatalf("未观测的上游不能报告已连接: %+v", ack)
	}
}
