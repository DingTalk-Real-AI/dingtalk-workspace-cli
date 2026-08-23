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

func TestCrossPlatformCoverageUnixSocketDirectoryErrorCoverage(t *testing.T) {
	oldLstat, oldRuntimeStat, oldMkdir := lstatSocketPath, statSocketRuntimeRoot, mkdirSocketDir
	t.Cleanup(func() {
		lstatSocketPath, statSocketRuntimeRoot, mkdirSocketDir = oldLstat, oldRuntimeStat, oldMkdir
	})

	wantErr := errors.New("synthetic socket directory failure")
	if err := ensureSocketDir("relative/bus.sock", true); err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("relative socket path error = %v", err)
	}

	root := shortSecureTempDir(t)
	missingRootPath := filepath.Join(root, "missing-root", "dws-event-test", "bus.sock")
	if err := ensureSocketDir(missingRootPath, false); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing runtime root error = %v", err)
	}

	rootInfo, err := oldLstat(root)
	if err != nil {
		t.Fatalf("lstat secure root: %v", err)
	}
	lstatSocketPath = func(path string) (os.FileInfo, error) {
		if path == "/tmp" {
			return fileInfoWithMode{FileInfo: rootInfo, mode: os.ModeSymlink | 0o777}, nil
		}
		return oldLstat(path)
	}
	statSocketRuntimeRoot = func(path string) (os.FileInfo, error) {
		if path == "/tmp" {
			return nil, wantErr
		}
		return oldRuntimeStat(path)
	}
	if err := ensureSocketDir("/tmp/dws-event-coverage/bus.sock", false); !errors.Is(err, wantErr) {
		t.Fatalf("runtime root resolution error = %v", err)
	}
	lstatSocketPath, statSocketRuntimeRoot = oldLstat, oldRuntimeStat

	rootFile := filepath.Join(root, "runtime-root-file")
	if err := os.WriteFile(rootFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write runtime root file: %v", err)
	}
	if err := ensureSocketDir(filepath.Join(rootFile, "dws-event-test", "bus.sock"), false); err == nil || !strings.Contains(err.Error(), "runtime root is not a directory") {
		t.Fatalf("non-directory runtime root error = %v", err)
	}

	mkdirSocketDir = func(string, os.FileMode) error { return wantErr }
	if err := ensureSocketDir(filepath.Join(root, "mkdir-failure", "bus.sock"), true); !errors.Is(err, wantErr) {
		t.Fatalf("socket directory creation error = %v", err)
	}
	mkdirSocketDir = oldMkdir

	if err := ensureSocketDir(filepath.Join(root, "missing-socket-dir", "bus.sock"), false); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing socket directory error = %v", err)
	}

	withoutOwner := fileInfoWithoutOwner{FileInfo: rootInfo}
	if err := validateSocketRuntimeRoot(root, withoutOwner, uint32(os.Geteuid())); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("runtime root owner error = %v", err)
	}
	if err := validatePrivateSocketDir(root, withoutOwner, uint32(os.Geteuid())); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("socket directory owner error = %v", err)
	}
}

type fileInfoWithMode struct {
	os.FileInfo
	mode os.FileMode
}

func (f fileInfoWithMode) Mode() os.FileMode { return f.mode }
func (f fileInfoWithMode) IsDir() bool       { return f.mode.IsDir() }

type fileInfoWithoutOwner struct{ os.FileInfo }

func (fileInfoWithoutOwner) Sys() any { return struct{}{} }

type stubNetListener struct {
	close func() error
}

func (*stubNetListener) Accept() (net.Conn, error) { return nil, io.EOF }
func (l *stubNetListener) Close() error            { return l.close() }
func (*stubNetListener) Addr() net.Addr            { return stubNetAddr{} }

type stubNetAddr struct{}

func (stubNetAddr) Network() string { return "unix" }
func (stubNetAddr) String() string  { return "stub" }
