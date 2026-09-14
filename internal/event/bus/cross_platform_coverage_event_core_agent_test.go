package bus

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/runtimecred"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func eventCoreDaemon(broker *runtimecred.Broker) *daemon {
	return &daemon{
		cfg:      Config{CredentialBroker: broker, IdleTimeout: time.Second},
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		hub:      NewHub(4),
		started:  time.Now(),
		idleStop: make(chan struct{}),
	}
}

func eventCoreConnection(d *daemon, wrap func(net.Conn) net.Conn) (net.Conn, *transport.Writer, *transport.Reader, <-chan struct{}) {
	server, client := net.Pipe()
	if wrap != nil {
		server = wrap(server)
	}
	done := make(chan struct{})
	go func() {
		d.handleConnection(context.Background(), server)
		close(done)
	}()
	return client, transport.NewWriter(client), transport.NewReader(client), done
}

func eventCoreWaitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("connection handler did not stop")
	}
}

type eventCoreWriteHookConn struct {
	net.Conn
	writes int
	hook   func(int)
}

func (c *eventCoreWriteHookConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.writes++
	if err == nil && c.hook != nil {
		c.hook(c.writes)
	}
	return n, err
}

func TestCrossPlatformCoverageEventCoreDaemonHandshakeEdges(t *testing.T) {
	t.Run("incompatible ack write failure", func(t *testing.T) {
		d := eventCoreDaemon(nil)
		client, w, _, done := eventCoreConnection(d, nil)
		if err := w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, CredentialMode: "unsupported"}); err != nil {
			t.Fatal(err)
		}
		_ = client.Close()
		eventCoreWaitDone(t, done)
	})

	t.Run("runtime ack write failure", func(t *testing.T) {
		d := eventCoreDaemon(runtimecred.New(runtimecred.Config{}))
		client, w, _, done := eventCoreConnection(d, nil)
		if err := w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, CredentialMode: transport.CredentialModeRuntimeToken}); err != nil {
			t.Fatal(err)
		}
		_ = client.Close()
		eventCoreWaitDone(t, done)
	})

	t.Run("terminal runtime hello", func(t *testing.T) {
		d := eventCoreDaemon(runtimecred.New(runtimecred.Config{}))
		d.setTerminalReason(transport.ByeReasonRuntimeTokenRejected)
		client, w, r, done := eventCoreConnection(d, nil)
		defer client.Close()
		if err := w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, CredentialMode: transport.CredentialModeRuntimeToken}); err != nil {
			t.Fatal(err)
		}
		var ack transport.HelloAck
		if err := r.ReadJSON(&ack); err != nil || ack.TerminalReason != transport.ByeReasonRuntimeTokenRejected {
			t.Fatalf("terminal ack = %#v, %v", ack, err)
		}
		eventCoreWaitDone(t, done)
	})

	t.Run("malformed credential update", func(t *testing.T) {
		d := eventCoreDaemon(runtimecred.New(runtimecred.Config{}))
		client, w, r, done := eventCoreConnection(d, nil)
		defer client.Close()
		if err := w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, CredentialMode: transport.CredentialModeRuntimeToken}); err != nil {
			t.Fatal(err)
		}
		var ack transport.HelloAck
		if err := r.ReadJSON(&ack); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Write([]byte("{\n")); err != nil {
			t.Fatal(err)
		}
		eventCoreWaitDone(t, done)
	})

	t.Run("unexpected credential update", func(t *testing.T) {
		d := eventCoreDaemon(runtimecred.New(runtimecred.Config{}))
		client, w, r, done := eventCoreConnection(d, nil)
		defer client.Close()
		if err := w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, CredentialMode: transport.CredentialModeRuntimeToken}); err != nil {
			t.Fatal(err)
		}
		var ack transport.HelloAck
		if err := r.ReadJSON(&ack); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteJSON(transport.Heartbeat{Type: transport.FrameTypeHeartbeat}); err != nil {
			t.Fatal(err)
		}
		var updateAck transport.CredentialUpdateAck
		if err := r.ReadJSON(&updateAck); err != nil || updateAck.ErrorCode != transport.CredentialErrorInvalid {
			t.Fatalf("unexpected-frame ack = %#v, %v", updateAck, err)
		}
		eventCoreWaitDone(t, done)
	})

	t.Run("credential ack write failure", func(t *testing.T) {
		d := eventCoreDaemon(runtimecred.New(runtimecred.Config{}))
		client, w, r, done := eventCoreConnection(d, nil)
		if err := w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, CredentialMode: transport.CredentialModeRuntimeToken}); err != nil {
			t.Fatal(err)
		}
		var ack transport.HelloAck
		if err := r.ReadJSON(&ack); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteJSON(transport.CredentialUpdate{
			Type: transport.FrameTypeCredentialUpdate, ExpectedGeneration: ack.CredentialGeneration, Token: "token",
		}); err != nil {
			t.Fatal(err)
		}
		_ = client.Close()
		eventCoreWaitDone(t, done)
	})

	t.Run("activation conflict", func(t *testing.T) {
		broker := runtimecred.New(runtimecred.Config{RequireSeed: true, RequireActivation: true})
		d := eventCoreDaemon(broker)
		client, w, r, done := eventCoreConnection(d, func(conn net.Conn) net.Conn {
			return &eventCoreWriteHookConn{Conn: conn, hook: func(write int) {
				if write == 2 {
					_, _ = broker.Update(1, "newer-token")
				}
			}}
		})
		defer client.Close()
		if err := w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, CredentialMode: transport.CredentialModeRuntimeToken}); err != nil {
			t.Fatal(err)
		}
		var ack transport.HelloAck
		if err := r.ReadJSON(&ack); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteJSON(transport.CredentialUpdate{Type: transport.FrameTypeCredentialUpdate, Token: "first-token"}); err != nil {
			t.Fatal(err)
		}
		var updateAck transport.CredentialUpdateAck
		if err := r.ReadJSON(&updateAck); err != nil || !updateAck.Accepted {
			t.Fatalf("credential ack = %#v, %v", updateAck, err)
		}
		var bye transport.Bye
		if err := r.ReadJSON(&bye); err != nil || bye.Reason != "runtime_credential_activation_failed" {
			t.Fatalf("activation failure bye = %#v, %v", bye, err)
		}
		eventCoreWaitDone(t, done)
	})
}

func TestCrossPlatformCoverageEventCoreDaemonWriterStopEdges(t *testing.T) {
	originalProcs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(originalProcs) })

	run := func(t *testing.T, queueEvent bool) {
		t.Helper()
		d := eventCoreDaemon(nil)
		client, w, r, done := eventCoreConnection(d, nil)
		defer client.Close()
		if err := w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, SubscribeID: "writer-stop"}); err != nil {
			t.Fatal(err)
		}
		var ack transport.HelloAck
		if err := r.ReadJSON(&ack); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(time.Second)
		for d.hub.Len() != 1 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		time.Sleep(5 * time.Millisecond)
		d.hub.mu.RLock()
		var consumer *Consumer
		for _, candidate := range d.hub.consumers {
			consumer = candidate
		}
		d.hub.mu.RUnlock()
		if consumer == nil {
			t.Fatal("consumer not registered")
		}
		if queueEvent {
			consumer.SendCh <- transport.Heartbeat{Type: transport.FrameTypeHeartbeat}
		}
		consumer.StopCh <- "writer-stop"
		var bye transport.Bye
		if err := r.ReadJSON(&bye); err != nil || bye.Reason != "writer-stop" {
			t.Fatalf("writer stop bye = %#v, %v", bye, err)
		}
		eventCoreWaitDone(t, done)
	}

	t.Run("recheck after event", func(t *testing.T) { run(t, true) })
	t.Run("blocked stop select", func(t *testing.T) { run(t, false) })
}

func TestCrossPlatformCoverageEventCoreDaemonHelpersAndStopAll(t *testing.T) {
	var nilDaemon *daemon
	nilDaemon.setTerminalReason("ignored")
	if nilDaemon.getTerminalReason() != "" {
		t.Fatal("nil daemon returned terminal reason")
	}
	d := eventCoreDaemon(nil)
	d.setTerminalReason("ignored")
	if d.getTerminalReason() != "" {
		t.Fatal("invalid terminal reason was stored")
	}
	d.setTerminalReason(transport.ByeReasonRuntimeTokenRejected)
	if d.getTerminalReason() != transport.ByeReasonRuntimeTokenRejected {
		t.Fatal("terminal reason was not stored")
	}

	if code, _ := classifyCredentialUpdateError(runtimecred.ErrEmptyToken); code != transport.CredentialErrorInvalid {
		t.Fatalf("empty-token classification = %q", code)
	}
	if code, message := classifyCredentialUpdateError(errors.New("internal detail")); code != transport.CredentialErrorInternal || message != "runtime credential update failed" {
		t.Fatalf("internal classification = %q, %q", code, message)
	}

	hub := NewHub(1)
	consumer, err := hub.Register(transport.Hello{})
	if err != nil {
		t.Fatal(err)
	}
	if stopped := hub.StopAll("   "); stopped != 1 {
		t.Fatalf("StopAll = %d", stopped)
	}
	select {
	case reason := <-consumer.StopCh:
		if reason != "shutdown" {
			t.Fatalf("default stop reason = %q", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("default stop reason not delivered")
	}
	hub.Unregister(consumer.ID)
}
