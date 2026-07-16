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
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/dedup"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// SourceAdapter is the interface daemon.go uses to talk to the cloud Source
// (in practice internal/event/source.DingtalkSource). Kept abstract so the
// bus daemon can be tested without spinning up the real Stream SDK; the
// integration test substitutes a fake.
type SourceAdapter interface {
	// Start opens the cloud connection and blocks until ctx is cancelled
	// or a fatal error occurs. emit is called for each incoming event.
	Start(ctx context.Context, emit dwsevent.EmitFn) error
}

// Config bundles everything Run needs to start a daemon. All paths and
// identifiers come from busctl/Source so the bus itself stays oblivious
// to ConfigDir / edition rules.
type Config struct {
	// WorkDir is the bus working directory:
	//   <ConfigDir>/events/<edition>/<source_kind>/<identity_hash>/
	// The caller MUST mkdir this with pkg/config.DirPerm before calling Run.
	WorkDir string

	// IPCEndpoint is the Unix socket path or Windows pipe name. Caller
	// computes this from WorkDir (Unix) or edition/clientIDHash (Windows).
	IPCEndpoint string

	// ClientID is the human-readable identifier written into bus.meta and
	// status output. NOT used in any path.
	ClientID string

	// SourceKind/IdentityHash/SourceID are diagnostic identity fields used by
	// list/status. Empty SourceKind is interpreted as app_stream for backward
	// compatibility.
	SourceKind   dwsevent.SourceKind
	IdentityHash string
	SourceID     string

	// Edition is written into bus.meta. Comes from edition.Get().Name with
	// "open" fallback applied by the caller.
	Edition string

	// SDKVersion is recorded in bus.meta for diagnostics.
	SDKVersion string

	// Source is the cloud adapter. Required.
	Source SourceAdapter

	// IdleTimeout: bus self-exits after this long with zero consumers.
	// Zero disables (bus runs until SIGTERM).
	IdleTimeout time.Duration

	// ConsumerBuffer overrides per-consumer sendCh capacity. Zero uses
	// DefaultSendBuffer.
	ConsumerBuffer int

	// DedupCapacity overrides event_id LRU size. Zero uses dedup.DefaultCapacity.
	DedupCapacity int

	// DropWarnPercent is the per-event-type drop-rate threshold (whole
	// percentage points) that triggers a slog WARN in bus.log. Zero or
	// out-of-range values fall back to DefaultDropWarnPercent. Overridable
	// via env DWS_EVENT_DROP_WARN_PCT (read by the cobra layer).
	DropWarnPercent int

	// ReadyPipe receives a single byte ('R' on success, 'E' on failure)
	// once the bus has either come up or failed startup, so the parent
	// process forked by busctl/spawn can stop polling and either dial or
	// surface the error. nil disables (foreground mode).
	ReadyPipe *os.File

	// Logger sink. Nil → slog.Default.
	Logger *slog.Logger
}

var (
	daemonMkdirAll        = os.MkdirAll
	daemonAcquire         = Acquire
	daemonWriteMeta       = WriteMeta
	daemonListen          = transport.Listen
	daemonShutdownTimeout = 2 * time.Second
)

// Run starts the bus daemon. Lifecycle (plan §4 invariant #6):
//  1. Acquire bus.lock (single-instance enforcement)
//  2. Write bus.meta
//  3. Listen IPC (so consumers can connect before SDK starts pushing)
//  4. Signal readiness via ReadyPipe
//  5. Start the Source (cloud SDK); concurrent with consumer accept loop
//  6. Wait on ctx for shutdown signal
//  7. Graceful: broadcast Bye → close listener → close source → release lock
//
// Run blocks until ctx is cancelled, the Source returns an error, or a fatal
// startup error occurs.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Source == nil {
		return errors.New("bus: Source is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	log := cfg.Logger.With("component", "bus", "client_id", cfg.ClientID, "edition", cfg.Edition)

	// 1. Acquire bus.lock
	if err := daemonMkdirAll(cfg.WorkDir, config.DirPerm); err != nil {
		return failReady(cfg.ReadyPipe, fmt.Errorf("bus: mkdir workdir: %w", err))
	}
	lockPath := filepath.Join(cfg.WorkDir, LockFileName)
	lock, err := daemonAcquire(lockPath)
	if err != nil {
		return failReady(cfg.ReadyPipe, fmt.Errorf("bus: acquire lock: %w", err))
	}
	defer lock.Close()

	// 2. Write bus.meta
	meta := Meta{
		ClientID:     cfg.ClientID,
		Edition:      cfg.Edition,
		SourceKind:   cfg.SourceKind,
		IdentityHash: cfg.IdentityHash,
		SourceID:     cfg.SourceID,
		StartedAt:    time.Now().UTC(),
		SDKVersion:   cfg.SDKVersion,
		BusPID:       os.Getpid(),
	}
	if err := daemonWriteMeta(cfg.WorkDir, meta); err != nil {
		return failReady(cfg.ReadyPipe, fmt.Errorf("bus: write meta: %w", err))
	}

	// 3. IPC listen
	listener, err := daemonListen(cfg.IPCEndpoint)
	if err != nil {
		return failReady(cfg.ReadyPipe, fmt.Errorf("bus: ipc listen: %w", err))
	}
	defer listener.Close()

	hub := NewHub(cfg.ConsumerBuffer)
	dd := dedup.NewWithCapacity(cfg.DedupCapacity)

	d := &daemon{
		cfg:      cfg,
		log:      log,
		lock:     lock,
		listener: listener,
		hub:      hub,
		dedup:    dd,
		started:  time.Now().UTC(),
		idleStop: make(chan struct{}),
	}

	// 4. Signal ready BEFORE accepting consumers (avoids a slow-fork
	// scenario where the parent thinks bus is dead but it's actually mid-
	// startup). Source.Start hasn't yet pulled events from the cloud, but
	// any consumer that connects gets queued for the first events.
	signalReady(cfg.ReadyPipe)

	// runCtx is a child of the caller's ctx that ALL background goroutines
	// (acceptLoop / idleWatch / dropWarnWatcher / source.Start) listen on.
	// On idle-timeout shutdown the parent ctx is never cancelled, so we
	// cancel runCtx ourselves before waiting for the goroutines — otherwise
	// dropWarnWatcher (which only exits on ctx.Done) hangs forever.
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	// 5. Start accept loop and Source concurrently. runCtx cancellation
	// propagates to both.
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		d.acceptLoop(runCtx)
	}()

	idleDone := make(chan struct{})
	go func() {
		defer close(idleDone)
		d.idleWatch(runCtx)
	}()

	// Drop-rate WARN: per-event-type back-pressure monitoring. Runs in
	// the background until runCtx cancellation; emits one WARN per scan
	// when a type's drop rate first crosses the threshold (hysteresis
	// suppresses repeats unless the rate jumps further).
	dropWarnDone := make(chan struct{})
	go func() {
		defer close(dropWarnDone)
		dropWarnWatcher(runCtx, hub.Counters(), log, cfg.DropWarnPercent)
	}()

	srcErr := make(chan error, 1)
	go func() {
		// emit is called from inside the SDK callback goroutine. It MUST
		// NOT block (plan invariant #1) — dedup + Hub.Deliver are both
		// non-blocking by construction.
		emit := func(raw *dwsevent.RawEvent) {
			if raw == nil {
				return
			}
			if dd.Seen(raw.DedupKey()) {
				return // duplicate redelivery from the cloud
			}
			hub.Deliver(raw)
		}
		srcErr <- cfg.Source.Start(runCtx, emit)
	}()

	// 6. Wait for shutdown trigger.
	var exitErr error
	select {
	case <-ctx.Done():
		log.Info("bus: shutdown requested by ctx", "reason", ctx.Err())
	case err := <-srcErr:
		log.Error("bus: source exited", "err", err)
		exitErr = err
	case <-d.idleStop:
		log.Info("bus: idle timeout reached, shutting down")
	}

	// 7. Graceful shutdown — cancel runCtx first so all background
	// goroutines wake up, then close listener / drain consumers.
	cancelRun()
	d.shutdown()
	<-acceptDone
	<-idleDone
	<-dropWarnDone

	return exitErr
}

// daemon is the in-memory state of one bus run. Lifetime equals one Run() call.
type daemon struct {
	cfg      Config
	log      *slog.Logger
	lock     *Lock
	listener transport.Listener
	hub      *Hub
	dedup    *dedup.LRU
	started  time.Time

	consumerWG   sync.WaitGroup // tracks live connection handler goroutines
	conns        sync.Map       // map[net.Conn]struct{} for forced shutdown close
	shutdownMu   sync.Mutex
	shuttingDown atomic.Bool
	idleStop     chan struct{}
}

// acceptLoop drives the IPC accept goroutine. Each accepted connection is
// passed to handleConnection in its own goroutine; the accept loop returns
// when the listener Close()s (typically during shutdown).
func (d *daemon) acceptLoop(ctx context.Context) {
	for {
		conn, err := d.listener.Accept()
		if err != nil {
			if d.shuttingDown.Load() {
				return
			}
			// Transient errors: log + continue. Non-transient (listener
			// closed) shows up as net.ErrClosed → also a clean exit.
			if errors.Is(err, net.ErrClosed) {
				return
			}
			d.log.Warn("bus: accept error", "err", err)
			continue
		}
		d.consumerWG.Add(1)
		go func() {
			defer d.consumerWG.Done()
			d.handleConnection(ctx, conn)
		}()
	}
}

// handleConnection processes one IPC connection's full lifecycle: read
// Hello → register with Hub → spawn writer goroutine → read until EOF/Bye.
// Always Unregisters and Closes on exit (plan invariant #5).
func (d *daemon) handleConnection(ctx context.Context, conn net.Conn) {
	d.conns.Store(conn, struct{}{})
	defer func() {
		d.conns.Delete(conn)
		conn.Close()
	}()

	r := transport.NewReader(conn)
	w := transport.NewWriter(conn)

	// Expect Hello first.
	var hello transport.Hello
	if err := r.ReadJSON(&hello); err != nil {
		d.log.Warn("bus: hello read failed", "err", err)
		return
	}
	if hello.Type != transport.FrameTypeHello {
		d.log.Warn("bus: first frame not hello", "type", hello.Type)
		return
	}

	// Ad-hoc tooling (status/list/stop) — short-lived RPC, no Hub register.
	if hello.Role == transport.HelloRoleStatus {
		d.handleStatusRPC(w, r)
		return
	}
	if hello.Role == transport.HelloRoleStop {
		// Signal shutdown by cancelling our parent ctx via shutdown().
		// For now, return after acking — daemon.shutdown is wired through
		// daemon's exit path (the busctl/stop command sends SIGTERM in
		// addition to this RPC for v1).
		_ = w.WriteJSON(transport.Bye{Type: transport.FrameTypeBye, Reason: "stop_request"})
		go d.triggerShutdown("stop_request")
		return
	}

	// Regular consumer registration
	c, err := d.hub.Register(hello)
	if err != nil {
		d.log.Warn("bus: register failed", "err", err, "pid", hello.ConsumerPID)
		_ = w.WriteJSON(transport.Bye{Type: transport.FrameTypeBye, Reason: "register_failed: " + err.Error()})
		return
	}

	// HelloAck — credentials_source fields are filled in by the daemon
	// runner (which knows from the strict resolver) and exposed via the
	// adapter for forward-compat. v1 leaves them empty here; daemon.Run
	// passes them through future config if the caller wishes.
	idleSecs := int(d.cfg.IdleTimeout / time.Second)
	if err := w.WriteJSON(transport.HelloAck{
		Type:            transport.FrameTypeHelloAck,
		BusPID:          os.Getpid(),
		SourceState:     "connected", // best-effort; full state machine pushed via SourceState frames
		StateSource:     "inferred",
		IdleTimeoutSecs: idleSecs,
	}); err != nil {
		d.log.Warn("bus: helloack write failed", "err", err)
		return
	}

	// Writer goroutine pulls from SendCh and writes to the wire.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for frame := range c.SendCh {
			if err := w.WriteJSON(frame); err != nil {
				// Wire error: peer dead. Returning here will let the
				// reader goroutine notice EOF and Unregister.
				return
			}
		}
	}()

	// Reader loop: wait for Bye or EOF. Both trigger Unregister.
	for {
		raw, err := r.Read()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				d.log.Debug("bus: consumer read error", "pid", c.PID, "err", err)
			}
			break
		}
		typ, err := transport.PeekType(raw)
		if err != nil {
			continue
		}
		if typ == transport.FrameTypeBye {
			break
		}
		// Heartbeat / future client→bus frames: ignored for v1.
	}
	// Order matters: Unregister first (closes SendCh), then wait for the
	// writer goroutine to drain. The reverse order would deadlock because
	// the writer loops on `range SendCh` until close, but only the Hub
	// can close that channel via Unregister.
	d.hub.Unregister(c.ID)
	<-writerDone
	_ = ctx // for future use (writer ctx-cancel propagation)
}

// handleStatusRPC services a single status_req and returns. The connection
// is closed by the caller's defer.
func (d *daemon) handleStatusRPC(w *transport.Writer, r *transport.Reader) {
	var req transport.StatusReq
	if err := r.ReadJSON(&req); err != nil {
		return
	}
	resp := transport.StatusResp{
		Type: transport.FrameTypeStatusResp,
		Bus: transport.StatusBus{
			PID:            os.Getpid(),
			UptimeSecs:     int64(time.Since(d.started).Seconds()),
			IdleTimeoutSec: int(d.cfg.IdleTimeout / time.Second),
			ClientID:       d.cfg.ClientID,
			Edition:        d.cfg.Edition,
			SourceKind:     d.cfg.SourceKind,
			IdentityHash:   d.cfg.IdentityHash,
			SourceID:       d.cfg.SourceID,
		},
		SourceState: transport.StatusSource{
			State:  "connected", // v1: source state plumbed in P3+
			Source: "inferred",
		},
		Consumers:            d.hub.Snapshot(),
		PerEventTypeCounters: d.hub.Counters().Snapshot(),
	}
	_ = w.WriteJSON(resp)
}

// idleWatch fires d.idleStop when IdleTimeout passes with zero registered
// consumers. Disabled when IdleTimeout <= 0 (returns immediately; idleStop
// is then never closed and Run's select branch on it is effectively dead).
//
// Pre-condition: d.idleStop has already been allocated by Run so the parent
// select never races on a nil channel (which would block forever).
func (d *daemon) idleWatch(ctx context.Context) {
	if d.cfg.IdleTimeout <= 0 {
		return
	}
	tick := time.NewTicker(d.cfg.IdleTimeout / 4)
	defer tick.Stop()
	emptySince := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if d.hub.Len() == 0 {
				if emptySince.IsZero() {
					emptySince = time.Now()
				} else if time.Since(emptySince) >= d.cfg.IdleTimeout {
					close(d.idleStop)
					return
				}
			} else {
				emptySince = time.Time{}
			}
		}
	}
}

// triggerShutdown is called from RPC handlers that want to end the bus.
// It works by closing the listener (which unblocks Run's select via the
// source error path, indirectly). For v1 a full ctx-cancellation hook is
// out of scope; busctl/stop also sends SIGTERM which is the authoritative
// shutdown path.
func (d *daemon) triggerShutdown(reason string) {
	d.log.Info("bus: shutdown triggered via IPC", "reason", reason)
	_ = d.listener.Close() // unblocks Accept(), but doesn't kill Source
	// Best-effort: a future version wires a context.CancelFunc here.
}

// shutdown performs the graceful tear-down sequence:
//  1. mark shuttingDown so acceptLoop exits cleanly
//  2. broadcast Bye to all consumers
//  3. close listener (interrupts pending Accept)
//  4. wait for all per-connection goroutines to drain
//  5. lock + meta cleanup via Run's defers
func (d *daemon) shutdown() {
	d.shutdownMu.Lock()
	defer d.shutdownMu.Unlock()
	if !d.shuttingDown.CompareAndSwap(false, true) {
		return
	}
	d.hub.Broadcast(transport.Bye{Type: transport.FrameTypeBye, Reason: "shutdown"})
	_ = d.listener.Close()
	// Force-close all open IPC connections so any reader goroutine blocked
	// on Read() returns with a network error and exits cleanly. Without
	// this the consumerWG never drains and Run hangs forever.
	d.conns.Range(func(k, _ any) bool {
		if c, ok := k.(net.Conn); ok {
			_ = c.Close()
		}
		return true
	})
	// Give consumers a brief moment to drain final frames before we tear
	// down their channels.
	doneCh := make(chan struct{})
	go func() {
		d.consumerWG.Wait()
		close(doneCh)
	}()
	select {
	case <-doneCh:
	case <-time.After(daemonShutdownTimeout):
		d.log.Warn("bus: shutdown: consumer goroutines did not drain within 2s")
	}
}

// signalReady writes a single 'R' byte to the ready pipe (if provided) and
// closes it. The parent process (busctl/spawn) reads one byte and proceeds.
func signalReady(p *os.File) {
	if p == nil {
		return
	}
	_, _ = p.Write([]byte{'R'})
	_ = p.Close()
}

// failReady writes 'E' to the ready pipe (if provided) and returns err.
// Used by the startup-failure paths so the parent can distinguish "still
// starting up" from "failed to start".
func failReady(p *os.File, err error) error {
	if p != nil {
		_, _ = p.Write([]byte{'E'})
		_ = p.Close()
	}
	return err
}
