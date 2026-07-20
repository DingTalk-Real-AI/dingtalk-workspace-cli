package bus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	eventlock "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

var errBusInjected = errors.New("injected bus failure")

func TestCrossPlatformCoverageMatcherConsumerAndCounterEdges(t *testing.T) {
	m, err := compileMatcher([]string{"", "*", "im.*", "prefix*", "exact"}, "^im", " sub ")
	if err != nil {
		t.Fatal(err)
	}
	if m.matches(nil) {
		t.Fatal("nil event matched")
	}
	for _, tc := range []struct {
		eventType, subscribe string
		want                 bool
	}{
		{"exact", "sub", false},
		{"im.message", "other", false},
		{"im.message", "sub", true},
		{"prefix-value", "sub", false},
	} {
		if got := m.matches(&dwsevent.RawEvent{EventType: tc.eventType, SubscribeID: tc.subscribe}); got != tc.want {
			t.Fatalf("matches(%q,%q) = %v", tc.eventType, tc.subscribe, got)
		}
	}
	m, _ = compileMatcher([]string{"exact", "pre*"}, "", "")
	if !m.matches(&dwsevent.RawEvent{EventType: "exact"}) || !m.matches(&dwsevent.RawEvent{EventType: "prefix"}) {
		t.Fatal("exact/prefix matcher failed")
	}
	if m.matches(&dwsevent.RawEvent{EventType: "other"}) {
		t.Fatal("unexpected matcher hit")
	}

	re := &RegisterError{Err: errBusInjected}
	if !errors.Is(re, errBusInjected) || re.Error() == "" {
		t.Fatal("register error contract")
	}
	h := NewHub(0)
	h.Deliver(nil)
	c := &Consumer{SendCh: make(chan any), matcher: consumerMatcher{catchAll: true}}
	c.deliver(mkEvent("unbuffered", "1"), h.Counters())
	if c.dropped.Load() != 1 {
		t.Fatalf("unbuffered dropped = %d", c.dropped.Load())
	}
	if ok, evicted := c.tryPushOrDropOldestLocked("x"); ok || evicted {
		t.Fatalf("unbuffered = %v,%v", ok, evicted)
	}
	c.closed = true
	c.deliver(mkEvent("closed", "2"), h.Counters())
	if c.enqueue("x") {
		t.Fatal("closed enqueue succeeded")
	}
	c.closeSend()

	buffered := &Consumer{SendCh: make(chan any, 1)}
	buffered.SendCh <- "old"
	if ok, evicted := buffered.tryPushOrDropOldestLocked("new"); !ok || !evicted {
		t.Fatalf("drop oldest = %v,%v", ok, evicted)
	}
	buffered.closeSend()
	buffered.closeSend()

	counters := NewPerTypeCounters()
	counters.row("zero")
	if got := counters.DropRatePercent("zero"); got != -1 {
		t.Fatalf("zero rate = %d", got)
	}
	origHook := counterRowMissHook
	t.Cleanup(func() { counterRowMissHook = origHook })
	inserted := false
	counterRowMissHook = func() {
		if inserted {
			return
		}
		inserted = true
		counters.mu.Lock()
		counters.m["raced"] = &typeRow{}
		counters.mu.Unlock()
	}
	if counters.row("raced") == nil {
		t.Fatal("raced row missing")
	}
}

func TestCrossPlatformCoverageDropWarningScanEdges(t *testing.T) {
	origInterval := dropWarnTickInterval
	t.Cleanup(func() { dropWarnTickInterval = origInterval })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dropWarnWatcher(ctx, NewPerTypeCounters(), slog.Default(), 0)
	dropWarnWatcher(ctx, NewPerTypeCounters(), slog.Default(), 50)

	c := NewPerTypeCounters()
	c.AddReceived("healthy")
	c.AddReceived("warn")
	c.AddDropped("warn")
	state := map[string]int{"healthy": 90}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	scanDropWarnings(c, logger, 25, state)
	if _, ok := state["healthy"]; ok {
		t.Fatal("healthy warning state retained")
	}
	if state["warn"] == 0 || logs.Len() == 0 {
		t.Fatal("warning not emitted")
	}
	before := logs.Len()
	scanDropWarnings(c, logger, 25, state)
	if logs.Len() != before {
		t.Fatal("warning hysteresis failed")
	}
	for i := 0; i < 10; i++ {
		c.AddDropped("warn")
	}
	scanDropWarnings(c, logger, 25, state)
	if logs.Len() == before {
		t.Fatal("worsened warning not emitted")
	}
	dropWarnTickInterval = time.Millisecond
	ctx, cancel = context.WithTimeout(context.Background(), 4*time.Millisecond)
	defer cancel()
	dropWarnWatcher(ctx, c, logger, 25)
}

func TestCrossPlatformCoverageLockFailureInjection(t *testing.T) {
	origTry := busTryAcquire
	origSeek := busSeek
	origRead := busReadAll
	origAlive := busProcessAlive
	origTruncate := busTruncate
	origFprintf := busFprintf
	origSync := busSync
	t.Cleanup(func() {
		busTryAcquire = origTry
		busSeek = origSeek
		busReadAll = origRead
		busProcessAlive = origAlive
		busTruncate = origTruncate
		busFprintf = origFprintf
		busSync = origSync
	})
	busTryAcquire = func(string) (*eventlock.File, error) { return nil, errBusInjected }
	if _, err := Acquire("x"); err == nil {
		t.Fatal("acquire error expected")
	}
	busTryAcquire = origTry

	newPath := func(name string) string { return filepath.Join(t.TempDir(), name) }
	busSeek = func(*os.File, int64, int) (int64, error) { return 0, errBusInjected }
	if _, err := Acquire(newPath("seek")); err == nil {
		t.Fatal("seek error expected")
	}
	busSeek = origSeek
	busReadAll = func(io.Reader) ([]byte, error) { return nil, errBusInjected }
	if _, err := Acquire(newPath("read")); err == nil {
		t.Fatal("read error expected")
	}
	busReadAll = origRead
	path := newPath("alive")
	if err := os.WriteFile(path, []byte("123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	busProcessAlive = func(int) bool { return true }
	if _, err := Acquire(path); !errors.Is(err, ErrStaleOwnerAlive) {
		t.Fatalf("alive error = %v", err)
	}
	busProcessAlive = origAlive
	busTruncate = func(*os.File, int64) error { return errBusInjected }
	if _, err := Acquire(newPath("write")); err == nil {
		t.Fatal("write PID error expected")
	}
	busTruncate = origTruncate

	lock, err := Acquire(newPath("held"))
	if err != nil {
		t.Fatal(err)
	}
	if lock.HoldsPath() == "" {
		t.Fatal("held path empty")
	}
	_ = lock.Close()
	if lock.HoldsPath() != "" {
		t.Fatal("closed path retained")
	}
	var nilLock *Lock
	if nilLock.HoldsPath() != "" || nilLock.Close() != nil {
		t.Fatal("nil lock contract")
	}

	f, err := os.CreateTemp(t.TempDir(), "pid")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	busTruncate = func(*os.File, int64) error { return errBusInjected }
	if truncateAndWritePID(f, 1) == nil {
		t.Fatal("truncate failure expected")
	}
	busTruncate = func(*os.File, int64) error { return nil }
	busSeek = func(*os.File, int64, int) (int64, error) { return 0, errBusInjected }
	if truncateAndWritePID(f, 1) == nil {
		t.Fatal("PID seek failure expected")
	}
	busSeek = origSeek
	busFprintf = func(io.Writer, string, ...any) (int, error) { return 0, errBusInjected }
	if truncateAndWritePID(f, 1) == nil {
		t.Fatal("PID write failure expected")
	}
	busFprintf = origFprintf
	busSync = func(*os.File) error { return errBusInjected }
	if truncateAndWritePID(f, 0) == nil {
		t.Fatal("sync failure expected")
	}
}

func TestCrossPlatformCoverageMetaFailureInjection(t *testing.T) {
	origMarshal := metaMarshalIndent
	origWrite := metaWriteFile
	origRename := metaRename
	origRemove := metaRemove
	t.Cleanup(func() {
		metaMarshalIndent = origMarshal
		metaWriteFile = origWrite
		metaRename = origRename
		metaRemove = origRemove
	})
	metaMarshalIndent = func(any, string, string) ([]byte, error) { return nil, errBusInjected }
	if WriteMeta(t.TempDir(), Meta{}) == nil {
		t.Fatal("marshal error expected")
	}
	metaMarshalIndent = origMarshal
	metaWriteFile = func(string, []byte, os.FileMode) error { return errBusInjected }
	if WriteMeta(t.TempDir(), Meta{}) == nil {
		t.Fatal("write error expected")
	}
	metaWriteFile = origWrite
	removed := false
	metaRename = func(string, string) error { return errBusInjected }
	metaRemove = func(string) error { removed = true; return nil }
	if WriteMeta(t.TempDir(), Meta{}) == nil || !removed {
		t.Fatal("rename cleanup error expected")
	}
}

type edgeSource struct {
	start func(context.Context, dwsevent.EmitFn) error
}

func (s edgeSource) Start(ctx context.Context, emit dwsevent.EmitFn) error { return s.start(ctx, emit) }

func TestCrossPlatformCoverageRunStartupAndSourceEdges(t *testing.T) {
	if err := Run(context.Background(), Config{}); err == nil {
		t.Fatal("source validation expected")
	}
	origMkdir := daemonMkdirAll
	origAcquire := daemonAcquire
	origMeta := daemonWriteMeta
	origListen := daemonListen
	t.Cleanup(func() {
		daemonMkdirAll = origMkdir
		daemonAcquire = origAcquire
		daemonWriteMeta = origMeta
		daemonListen = origListen
	})
	source := edgeSource{start: func(context.Context, dwsevent.EmitFn) error { return errBusInjected }}
	base := Config{WorkDir: t.TempDir(), IPCEndpoint: filepath.Join(t.TempDir(), "bus.sock"), Source: source}
	daemonMkdirAll = func(string, os.FileMode) error { return errBusInjected }
	if err := Run(context.Background(), base); err == nil {
		t.Fatal("mkdir error expected")
	}
	daemonMkdirAll = origMkdir
	daemonAcquire = func(string) (*Lock, error) { return nil, errBusInjected }
	if err := Run(context.Background(), base); err == nil {
		t.Fatal("lock error expected")
	}
	daemonAcquire = origAcquire
	daemonWriteMeta = func(string, Meta) error { return errBusInjected }
	if err := Run(context.Background(), base); err == nil {
		t.Fatal("meta error expected")
	}
	daemonWriteMeta = origMeta
	daemonListen = func(string) (transport.Listener, error) { return nil, errBusInjected }
	if err := Run(context.Background(), base); err == nil {
		t.Fatal("listen error expected")
	}
	daemonListen = origListen

	workDir := shortTempDir(t)
	base.WorkDir = workDir
	base.IPCEndpoint = dwsevent.IPCEndpoint(
		workDir,
		"open",
		dwsevent.SourceKindAppStream,
		dwsevent.IdentityHash(workDir),
	)
	base.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	base.Source = edgeSource{start: func(_ context.Context, emit dwsevent.EmitFn) error {
		emit(nil)
		e := &dwsevent.RawEvent{EventID: "same", EventType: "type", ReceivedAt: time.Now()}
		emit(e)
		emit(e)
		return errBusInjected
	}}
	if err := Run(context.Background(), base); !errors.Is(err, errBusInjected) {
		t.Fatalf("source error = %v", err)
	}
}

type scriptedListener struct {
	accept func() (net.Conn, error)
	closed bool
}

func (l *scriptedListener) Accept() (net.Conn, error) { return l.accept() }
func (l *scriptedListener) Close() error              { l.closed = true; return nil }
func (*scriptedListener) Endpoint() string            { return "edge" }

func TestCrossPlatformCoverageDaemonMethodEdges(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	calls := 0
	l := &scriptedListener{accept: func() (net.Conn, error) {
		calls++
		if calls == 1 {
			return nil, errBusInjected
		}
		return nil, net.ErrClosed
	}}
	d := &daemon{listener: l, log: logger, hub: NewHub(1), idleStop: make(chan struct{})}
	d.acceptLoop(context.Background())
	d.shuttingDown.Store(true)
	d.acceptLoop(context.Background())
	d.triggerShutdown("test")
	if !l.closed {
		t.Fatal("trigger did not close listener")
	}

	d = &daemon{listener: &scriptedListener{accept: func() (net.Conn, error) { return nil, net.ErrClosed }}, log: logger, hub: NewHub(1), idleStop: make(chan struct{})}
	d.idleWatch(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	d.cfg.IdleTimeout = time.Millisecond
	cancel()
	d.idleWatch(ctx)
	d = &daemon{cfg: Config{IdleTimeout: 4 * time.Millisecond}, log: logger, hub: NewHub(1), idleStop: make(chan struct{})}
	consumer, _ := d.hub.Register(transport.Hello{})
	idleDone := make(chan struct{})
	go func() { d.idleWatch(context.Background()); close(idleDone) }()
	time.Sleep(3 * time.Millisecond)
	d.hub.Unregister(consumer.ID)
	select {
	case <-idleDone:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("idle watcher did not stop")
	}

	origTimeout := daemonShutdownTimeout
	t.Cleanup(func() { daemonShutdownTimeout = origTimeout })
	daemonShutdownTimeout = time.Millisecond
	d.listener = &scriptedListener{accept: func() (net.Conn, error) { return nil, net.ErrClosed }}
	d.conns.Store("not-a-conn", struct{}{})
	d.conns.Store(&queryConn{}, struct{}{})
	d.consumerWG.Add(1)
	acceptDone := make(chan struct{})
	close(acceptDone)
	d.shutdown(acceptDone)
	d.consumerWG.Done()
	d.shutdown(acceptDone)

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	want := errBusInjected
	if got := failReady(pw, want); !errors.Is(got, want) {
		t.Fatal(got)
	}
	b := make([]byte, 1)
	_, _ = pr.Read(b)
	_ = pr.Close()
	if b[0] != 'E' {
		t.Fatalf("fail ready byte = %q", b)
	}
	if failReady(nil, want) != want {
		t.Fatal("nil failReady changed error")
	}
}

func TestCrossPlatformCoverageHandleConnectionProtocolEdges(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	newDaemon := func() *daemon {
		return &daemon{
			cfg: Config{ClientID: "client", Edition: "open", IdleTimeout: time.Second},
			log: logger, hub: NewHub(2), started: time.Now(), idleStop: make(chan struct{}),
			listener: &scriptedListener{accept: func() (net.Conn, error) { return nil, net.ErrClosed }},
		}
	}
	run := func(d *daemon, action func(net.Conn)) {
		server, client := net.Pipe()
		done := make(chan struct{})
		go func() { d.handleConnection(context.Background(), server); close(done) }()
		action(client)
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("connection handler did not return")
		}
	}

	run(newDaemon(), func(c net.Conn) {})
	run(newDaemon(), func(c net.Conn) {
		_ = transport.NewWriter(c).WriteJSON(transport.StatusReq{Type: transport.FrameTypeStatusReq})
	})
	run(newDaemon(), func(c net.Conn) {
		_ = transport.NewWriter(c).WriteJSON(transport.Hello{Type: transport.FrameTypeHello, Role: transport.HelloRoleStatus})
	})
	run(newDaemon(), func(c net.Conn) {
		w, r := transport.NewWriter(c), transport.NewReader(c)
		_ = w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, Role: transport.HelloRoleStatus})
		_ = w.WriteJSON(transport.StatusReq{Type: transport.FrameTypeStatusReq})
		var resp transport.StatusResp
		if err := r.ReadJSON(&resp); err != nil {
			t.Fatal(err)
		}
	})
	run(newDaemon(), func(c net.Conn) {
		w, r := transport.NewWriter(c), transport.NewReader(c)
		_ = w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, Role: transport.HelloRoleStop})
		var bye transport.Bye
		if err := r.ReadJSON(&bye); err != nil {
			t.Fatal(err)
		}
	})
	run(newDaemon(), func(c net.Conn) {
		w, r := transport.NewWriter(c), transport.NewReader(c)
		_ = w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, Filter: "["})
		var bye transport.Bye
		if err := r.ReadJSON(&bye); err != nil {
			t.Fatal(err)
		}
	})
	run(newDaemon(), func(c net.Conn) {
		_ = transport.NewWriter(c).WriteJSON(transport.Hello{Type: transport.FrameTypeHello})
		_ = c.Close()
	})
	run(newDaemon(), func(c net.Conn) {
		w, r := transport.NewWriter(c), transport.NewReader(c)
		_ = w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello})
		var ack transport.HelloAck
		if err := r.ReadJSON(&ack); err != nil {
			t.Fatal(err)
		}
		_, _ = c.Write([]byte("{\n"))
		_ = w.WriteJSON(transport.StatusReq{Type: transport.FrameTypeStatusReq})
		_ = w.WriteJSON(transport.Bye{Type: transport.FrameTypeBye})
	})
	run(newDaemon(), func(c net.Conn) {
		w, r := transport.NewWriter(c), transport.NewReader(c)
		_ = w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello})
		var ack transport.HelloAck
		if err := r.ReadJSON(&ack); err != nil {
			t.Fatal(err)
		}
		payload := bytes.Repeat([]byte("x"), transport.MaxFrameBytes+1)
		_ = c.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		_, _ = c.Write(payload)
	})
	d := newDaemon()
	run(d, func(c net.Conn) {
		w, r := transport.NewWriter(c), transport.NewReader(c)
		_ = w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello})
		var ack transport.HelloAck
		if err := r.ReadJSON(&ack); err != nil {
			t.Fatal(err)
		}
		d.hub.Broadcast(transport.Bye{Type: transport.FrameTypeBye})
		time.Sleep(time.Millisecond)
	})
}

type queryConn struct{}

func (*queryConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (*queryConn) Write(p []byte) (int, error)      { return len(p), nil }
func (*queryConn) Close() error                     { return nil }
func (*queryConn) LocalAddr() net.Addr              { return edgeAddr("local") }
func (*queryConn) RemoteAddr() net.Addr             { return edgeAddr("remote") }
func (*queryConn) SetDeadline(time.Time) error      { return nil }
func (*queryConn) SetReadDeadline(time.Time) error  { return nil }
func (*queryConn) SetWriteDeadline(time.Time) error { return nil }

type edgeAddr string

func (a edgeAddr) Network() string { return string(a) }
func (a edgeAddr) String() string  { return string(a) }

var _ = fmt.Sprintf
