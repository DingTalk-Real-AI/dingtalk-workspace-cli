package consume

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

type errorFormatter struct{ err error }

func (f errorFormatter) Render(transport.Event) ([]byte, error) { return nil, f.err }

type errorSink struct{ err error }

func (s errorSink) Write(transport.Event, []byte) error { return s.err }
func (s errorSink) Close() error                        { return s.err }

func TestCrossPlatformCoverageFormatterPipelineRouterAndSinkEdges(t *testing.T) {
	wantErr := errors.New("synthetic failure")
	oldMarshal, oldIndent, oldCompact := marshalEvent, marshalEventIndent, marshalCompact
	oldMkdir, oldWrite, oldRename, oldRemove := mkdirSinkDir, writeSinkFile, renameSinkFile, removeSinkFile
	t.Cleanup(func() {
		marshalEvent, marshalEventIndent, marshalCompact = oldMarshal, oldIndent, oldCompact
		mkdirSinkDir, writeSinkFile, renameSinkFile, removeSinkFile = oldMkdir, oldWrite, oldRename, oldRemove
	})
	marshalEvent = func(any) ([]byte, error) { return nil, wantErr }
	if _, err := (ndjsonFormatter{}).Render(transport.Event{}); !errors.Is(err, wantErr) {
		t.Fatalf("ndjson marshal error = %v", err)
	}
	marshalEventIndent = func(any, string, string) ([]byte, error) { return nil, wantErr }
	if _, err := (prettyFormatter{}).Render(transport.Event{}); !errors.Is(err, wantErr) {
		t.Fatalf("pretty marshal error = %v", err)
	}
	marshalCompact = func(any) ([]byte, error) { return nil, wantErr }
	if _, err := (compactFormatter{}).Render(transport.Event{}); !errors.Is(err, wantErr) {
		t.Fatalf("compact marshal error = %v", err)
	}

	pipeline := NewPipeline(errorFormatter{err: wantErr}, errorSink{})
	if err := pipeline.Deliver(transport.Event{}); !errors.Is(err, wantErr) {
		t.Fatalf("formatter pipeline error = %v", err)
	}
	pipeline = NewPipeline(rawFormatter{}, errorSink{err: wantErr})
	if err := pipeline.Deliver(transport.Event{}); !errors.Is(err, wantErr) {
		t.Fatalf("sink pipeline error = %v", err)
	}
	if err := pipeline.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("pipeline close error = %v", err)
	}
	if _, err := BuildPipeline(Format("invalid"), "", nil, nil); err == nil {
		t.Fatal("invalid pipeline format should fail")
	}
	if p, err := BuildPipeline(FormatNDJSON, "", nil, nil); err != nil || p == nil {
		t.Fatalf("nil stdout pipeline = %#v, %v", p, err)
	}
	if (*Router)(nil).Rules() != nil || (*Router)(nil).Match(transport.Event{}) != "" {
		t.Fatal("nil router should have no rules or match")
	}
	if rules := NewRouter([]Route{}).Rules(); rules == nil {
		t.Fatal("non-nil router should return its rule slice")
	}
	if err := ValidateNoOutputConflict(validRunConfig(), "captured.out"); err != nil {
		t.Fatalf("global output without stream sink: %v", err)
	}

	if err := NewStdoutSink(errorWriter{err: testBrokenPipeError()}).Write(transport.Event{}, []byte("x")); !errors.Is(err, ErrPipeClosed) {
		t.Fatalf("broken pipe error = %v", err)
	}
	mkdirSinkDir = func(string, os.FileMode) error { return wantErr }
	if err := NewFileDirSink("dir").Write(transport.Event{}, nil); !errors.Is(err, wantErr) {
		t.Fatalf("sink mkdir error = %v", err)
	}
	mkdirSinkDir = oldMkdir
	writeSinkFile = func(string, []byte, os.FileMode) error { return wantErr }
	if err := atomicWrite("path", nil); !errors.Is(err, wantErr) {
		t.Fatalf("atomic write error = %v", err)
	}
	writeSinkFile = func(string, []byte, os.FileMode) error { return nil }
	renameSinkFile = func(string, string) error { return wantErr }
	removeSinkFile = func(string) error { return nil }
	if err := atomicWrite("path", nil); !errors.Is(err, wantErr) {
		t.Fatalf("atomic rename error = %v", err)
	}
}

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }

func validRunConfig() Config {
	return Config{WorkDir: "work", IPCEndpoint: "endpoint", ClientID: "client", Stdout: io.Discard, Stderr: io.Discard, Format: FormatNDJSON}
}

func TestCrossPlatformCoverageRunSetupAndDiscoveryEdges(t *testing.T) {
	if err := Run(context.Background(), Config{}); err == nil {
		t.Fatal("missing required run config should fail")
	}
	if err := Run(context.Background(), Config{WorkDir: "w", IPCEndpoint: "e", ClientID: "c", Quiet: true, DryRun: true}); err != nil {
		t.Fatalf("defaulted quiet dry run: %v", err)
	}
	cfg := validRunConfig()
	cfg.Format = Format("invalid")
	if err := Run(context.Background(), cfg); err == nil {
		t.Fatal("invalid format should fail before discovery")
	}

	oldDiscover := discoverBus
	t.Cleanup(func() { discoverBus = oldDiscover })
	wantErr := errors.New("discover failed")
	discoverBus = func(busctl.DiscoverConfig) (net.Conn, error) { return nil, wantErr }
	if err := Run(context.Background(), validRunConfig()); !errors.Is(err, wantErr) {
		t.Fatalf("discover error = %v", err)
	}

	discoverBus = func(busctl.DiscoverConfig) (net.Conn, error) { return &faultConn{writeErr: wantErr}, nil }
	if err := Run(context.Background(), validRunConfig()); !errors.Is(err, wantErr) {
		t.Fatalf("hello write error = %v", err)
	}

	discoverBus = pipeDiscover(t, nil, true)
	if err := Run(context.Background(), validRunConfig()); err == nil || !strings.Contains(err.Error(), "hello_ack") {
		t.Fatalf("ack read error = %v", err)
	}
	discoverBus = pipeDiscover(t, [][]byte{mustJSONFrame(t, transport.HelloAck{Type: transport.FrameTypeHeartbeat})}, false)
	if err := Run(context.Background(), validRunConfig()); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("wrong ack error = %v", err)
	}
}

func TestCrossPlatformCoverageRunFrameAndDeliveryEdges(t *testing.T) {
	oldDiscover := discoverBus
	t.Cleanup(func() { discoverBus = oldDiscover })
	ack := mustJSONFrame(t, transport.HelloAck{Type: transport.FrameTypeHelloAck, BusPID: 1})
	frames := [][]byte{
		ack,
		[]byte("{"),
		[]byte(`{"type":"event","event_id":{}}`),
		mustJSONFrame(t, transport.SourceState{Type: transport.FrameTypeSourceState, State: "running"}),
		mustJSONFrame(t, transport.Heartbeat{Type: transport.FrameTypeHeartbeat}),
		[]byte(`{"type":"future"}`),
		mustJSONFrame(t, transport.Bye{Type: transport.FrameTypeBye, Reason: "done"}),
	}
	discoverBus = pipeDiscover(t, frames, true)
	cfg := validRunConfig()
	cfg.Stderr = &bytes.Buffer{}
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("mixed frames run: %v", err)
	}
	discoverBus = pipeDiscover(t, [][]byte{ack}, true)
	if err := Run(context.Background(), validRunConfig()); err != nil {
		t.Fatalf("clean peer EOF run: %v", err)
	}

	event := mustJSONFrame(t, transport.Event{Type: transport.FrameTypeEvent, EventID: "id", EventType: "type", Data: `{}`})
	for _, tc := range []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{"pipe closed", errorWriter{err: testBrokenPipeError()}, nil},
		{"delivery failure", errorWriter{err: errors.New("output failed")}, errors.New("want error")},
	} {
		discoverBus = pipeDiscover(t, [][]byte{ack, event}, true)
		cfg = validRunConfig()
		cfg.Stdout = tc.writer
		err := Run(context.Background(), cfg)
		if tc.want == nil && err != nil {
			t.Errorf("%s run error = %v", tc.name, err)
		}
		if tc.want != nil && err == nil {
			t.Errorf("%s run should fail", tc.name)
		}
	}

	discoverBus = pipeDiscover(t, [][]byte{ack, event}, true)
	cfg = validRunConfig()
	cfg.MaxEvents = 1
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("max-events run: %v", err)
	}

	readErr := errors.New("read failed")
	discoverBus = func(busctl.DiscoverConfig) (net.Conn, error) {
		return &faultConn{readData: append(append([]byte{}, ack...), '\n'), readErr: readErr}, nil
	}
	if err := Run(context.Background(), validRunConfig()); !errors.Is(err, readErr) {
		t.Fatalf("frame read error = %v", err)
	}

	discoverBus = pipeDiscover(t, [][]byte{ack}, false)
	cfg = validRunConfig()
	cfg.Duration = 10 * time.Millisecond
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("duration cancellation run: %v", err)
	}
}

func testBrokenPipeError() error {
	if runtime.GOOS == "windows" {
		// ERROR_BROKEN_PIPE. Use syscall.Errno so this generic test file
		// stays buildable without importing the Windows-only package.
		return syscall.Errno(109)
	}
	return syscall.EPIPE
}

func mustJSONFrame(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	return raw
}

func pipeDiscover(t *testing.T, frames [][]byte, closeAfter bool) func(busctl.DiscoverConfig) (net.Conn, error) {
	t.Helper()
	return func(busctl.DiscoverConfig) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer func() {
				if closeAfter {
					_ = server.Close()
				}
			}()
			var hello transport.Hello
			if err := transport.NewReader(server).ReadJSON(&hello); err != nil {
				_ = server.Close()
				return
			}
			for _, frame := range frames {
				if _, err := server.Write(append(append([]byte{}, frame...), '\n')); err != nil {
					return
				}
			}
		}()
		return client, nil
	}
}

type faultConn struct {
	readData []byte
	readErr  error
	writeErr error
}

func (c *faultConn) Read(p []byte) (int, error) {
	if len(c.readData) > 0 {
		n := copy(p, c.readData)
		c.readData = c.readData[n:]
		return n, nil
	}
	return 0, c.readErr
}
func (c *faultConn) Write(p []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	return len(p), nil
}
func (*faultConn) Close() error                     { return nil }
func (*faultConn) LocalAddr() net.Addr              { return consumeAddr("local") }
func (*faultConn) RemoteAddr() net.Addr             { return consumeAddr("remote") }
func (*faultConn) SetDeadline(time.Time) error      { return nil }
func (*faultConn) SetReadDeadline(time.Time) error  { return nil }
func (*faultConn) SetWriteDeadline(time.Time) error { return nil }

type consumeAddr string

func (a consumeAddr) Network() string { return "test" }
func (a consumeAddr) String() string  { return string(a) }

func TestCrossPlatformCoverageWatchStdinEOFCancelledAfterData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	watchStdinEOF(ctx, strings.NewReader("data"), io.Discard, func() { called = true })
	if called {
		t.Fatal("watchStdinEOF called shutdown after context cancellation")
	}
}
