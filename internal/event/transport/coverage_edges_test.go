//go:build !windows

package transport

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageFrameCodecCoverageEdges(t *testing.T) {
	if _, err := NewReader(strings.NewReader("partial")).Read(); !errors.Is(err, io.EOF) {
		t.Fatalf("partial frame error = %v, want EOF", err)
	}
	if err := NewReader(strings.NewReader("")).ReadJSON(&map[string]any{}); !errors.Is(err, io.EOF) {
		t.Fatalf("ReadJSON EOF = %v", err)
	}
	if err := NewWriter(io.Discard).WriteJSON(make(chan int)); err == nil {
		t.Fatal("unsupported JSON value should fail")
	}
}

func TestCrossPlatformCoverageUnixListenErrorCoverage(t *testing.T) {
	oldStat, oldRemove, oldListen, oldChmod := statSocket, removeSocket, listenUnix, chmodSocket
	t.Cleanup(func() {
		statSocket, removeSocket, listenUnix, chmodSocket = oldStat, oldRemove, oldListen, oldChmod
	})
	wantErr := errors.New("synthetic failure")
	tempDir, err := os.MkdirTemp("", "dws-et-")
	if err != nil {
		t.Fatalf("create short socket temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	path := filepath.Join(tempDir, "bus.sock")
	if _, err := listen(strings.Repeat("x", 512)); err == nil {
		t.Fatal("overlong socket path should fail")
	}

	statSocket = func(string) (os.FileInfo, error) { return nil, nil }
	removeSocket = func(string) error { return wantErr }
	if _, err := listen(path); !errors.Is(err, wantErr) {
		t.Fatalf("stale remove error = %v", err)
	}

	statSocket = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	listenUnix = func(string, string) (net.Listener, error) { return nil, wantErr }
	if _, err := listen(path); !errors.Is(err, wantErr) {
		t.Fatalf("listen error = %v", err)
	}

	closed := false
	listener := &stubNetListener{close: func() error { closed = true; return nil }}
	listenUnix = func(string, string) (net.Listener, error) { return listener, nil }
	chmodSocket = func(string, os.FileMode) error { return wantErr }
	removeSocket = func(string) error { return nil }
	if _, err := listen(path); !errors.Is(err, wantErr) || !closed {
		t.Fatalf("chmod error = %v, closed=%v", err, closed)
	}

	removeSocket = func(string) error { return wantErr }
	if err := (&unixListener{l: listener, path: path}).Close(); err != nil {
		t.Fatalf("listener close should ignore unlink error: %v", err)
	}

	var buf bytes.Buffer
	if err := NewWriter(&buf).WriteJSON(map[string]any{"ok": true}); err != nil {
		t.Fatalf("sanity frame write: %v", err)
	}
}

type stubNetListener struct {
	close func() error
}

func (*stubNetListener) Accept() (net.Conn, error) { return nil, io.EOF }
func (l *stubNetListener) Close() error            { return l.close() }
func (*stubNetListener) Addr() net.Addr            { return stubNetAddr{} }

type stubNetAddr struct{}

func (stubNetAddr) Network() string { return "unix" }
func (stubNetAddr) String() string  { return "stub" }
